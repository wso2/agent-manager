// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package services

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MonitorRef names a monitor by its place in the org.
type MonitorRef struct {
	OUID        string
	ProjectName string
	AgentName   string
	MonitorName string
}

// MonitorAlertSettings configures shared monitor alerts sent to organization destinations.
type MonitorAlertSettings struct {
	DestinationIDs  []string
	NotifyOnFailure bool
	NotifyOnSuccess bool
	Thresholds      []models.ScoreThreshold
}

// MonitorAlertService manages one shared alert configuration per monitor.
type MonitorAlertService interface {
	GetAlert(ctx context.Context, ref MonitorRef) (alert *models.MonitorAlert, found bool, err error)
	SaveAlert(ctx context.Context, ref MonitorRef, settings MonitorAlertSettings) (*models.MonitorAlert, error)
	DeleteAlert(ctx context.Context, ref MonitorRef) error
}

type monitorAlertService struct {
	repo        repositories.MonitorAlertRepository
	monitorRepo repositories.MonitorRepository
	eventRepo   repositories.EventRepository
}

// NewMonitorAlertService creates a MonitorAlertService.
func NewMonitorAlertService(
	repo repositories.MonitorAlertRepository,
	monitorRepo repositories.MonitorRepository,
	eventRepo repositories.EventRepository,
) MonitorAlertService {
	return &monitorAlertService{repo: repo, monitorRepo: monitorRepo, eventRepo: eventRepo}
}

func (s *monitorAlertService) monitor(ref MonitorRef) (*models.Monitor, error) {
	m, err := s.monitorRepo.GetMonitorByName(ref.OUID, ref.ProjectName, ref.AgentName, ref.MonitorName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}
	if ref.OUID == "" || m.OUID != ref.OUID || m.ProjectName != ref.ProjectName || m.AgentName != ref.AgentName || m.Name != ref.MonitorName {
		return nil, utils.ErrMonitorNotFound
	}
	return m, nil
}

func (s *monitorAlertService) GetAlert(
	ctx context.Context, ref MonitorRef,
) (*models.MonitorAlert, bool, error) {
	m, err := s.monitor(ref)
	if err != nil {
		return nil, false, err
	}
	w, err := s.repo.GetAlert(ctx, m.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get monitor alert settings: %w", err)
	}
	return w, true, nil
}

func (s *monitorAlertService) SaveAlert(
	ctx context.Context, ref MonitorRef, settings MonitorAlertSettings,
) (*models.MonitorAlert, error) {
	m, err := s.monitor(ref)
	if err != nil {
		return nil, err
	}
	if err := validateThresholds(m, settings.Thresholds); err != nil {
		return nil, err
	}
	if !settings.NotifyOnFailure && !settings.NotifyOnSuccess && len(settings.Thresholds) == 0 {
		return nil, fmt.Errorf("%w: choose at least one alert condition", utils.ErrInvalidInput)
	}
	if len(settings.DestinationIDs) < 1 || len(settings.DestinationIDs) > 20 {
		return nil, fmt.Errorf("%w: choose between 1 and 20 destinations", utils.ErrInvalidInput)
	}
	seen := make(map[uuid.UUID]bool, len(settings.DestinationIDs))
	for _, raw := range settings.DestinationIDs {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil || seen[id] {
			return nil, fmt.Errorf("%w: destination IDs must be valid and unique", utils.ErrInvalidInput)
		}
		seen[id] = true
		destination, err := s.eventRepo.GetDestination(ctx, ref.OUID, id)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("%w: destination is unavailable", utils.ErrInvalidInput)
			}
			return nil, fmt.Errorf("failed to get alert destination: %w", err)
		}
		if destination.OUID != ref.OUID || !destination.UsableBy(models.EventScopeAgent, ref.ProjectName) {
			return nil, fmt.Errorf("%w: destination is unavailable to this monitor", utils.ErrInvalidInput)
		}
	}
	// The repository preserves breach state under the same row lock as the save.
	breached := []string{}
	thresholds := settings.Thresholds
	if thresholds == nil {
		thresholds = []models.ScoreThreshold{}
	}
	w := &models.MonitorAlert{
		OUID:            m.OUID,
		MonitorID:       m.ID,
		DestinationIDs:  append([]string(nil), settings.DestinationIDs...),
		NotifyOnFailure: settings.NotifyOnFailure,
		NotifyOnSuccess: settings.NotifyOnSuccess,
		Thresholds:      thresholds,
		Breached:        breached,
	}
	if err := s.repo.SaveAlert(ctx, w); err != nil {
		return nil, fmt.Errorf("failed to save monitor alert settings: %w", err)
	}
	return w, nil
}

// validateThresholds checks each threshold names one of the monitor's
// evaluators and a supported comparison, and that none repeats another.
func validateThresholds(m *models.Monitor, thresholds []models.ScoreThreshold) error {
	if len(thresholds) > models.MaxMonitorThresholds {
		return fmt.Errorf("%w: at most %d thresholds are allowed", utils.ErrInvalidInput, models.MaxMonitorThresholds)
	}
	evaluators := make(map[string]bool, len(m.Evaluators))
	for _, e := range m.Evaluators {
		evaluators[e.DisplayName] = true
	}
	seen := make(map[string]bool, len(thresholds))
	for i, t := range thresholds {
		switch {
		case !evaluators[t.Evaluator]:
			return fmt.Errorf("%w: thresholds[%d]: %q is not an evaluator of this monitor", utils.ErrInvalidInput, i, t.Evaluator)
		case t.Aggregation != models.ThresholdAggregationMean:
			return fmt.Errorf("%w: thresholds[%d]: aggregation must be mean", utils.ErrInvalidInput, i)
		case t.Operator != models.ThresholdBelow && t.Operator != models.ThresholdAbove:
			return fmt.Errorf("%w: thresholds[%d]: operator must be below or above", utils.ErrInvalidInput, i)
		case math.IsNaN(t.Value) || math.IsInf(t.Value, 0) || t.Value < 0 || t.Value > 1:
			return fmt.Errorf("%w: thresholds[%d]: value must be between 0 and 1", utils.ErrInvalidInput, i)
		}
		key := t.Evaluator + "|" + t.Aggregation + "|" + t.Operator
		if seen[key] {
			return fmt.Errorf("%w: thresholds[%d]: repeats another threshold", utils.ErrInvalidInput, i)
		}
		seen[key] = true
	}
	return nil
}

func (s *monitorAlertService) DeleteAlert(ctx context.Context, ref MonitorRef) error {
	m, err := s.monitor(ref)
	if err != nil {
		return err
	}
	if err := s.repo.DeleteAlert(ctx, m.ID); err != nil {
		return fmt.Errorf("failed to delete monitor alert settings: %w", err)
	}
	return nil
}
