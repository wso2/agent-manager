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

package repositories

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// MonitorAlertRepository persists shared monitor alert settings and durable delivery jobs.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/monitor_alert_repository_mock.go . MonitorAlertRepository:MonitorAlertRepositoryMock
type MonitorAlertRepository interface {
	// WithAlert evaluates a current watch under a row lock. All writes through
	// txRepo commit atomically with the evaluation. A deleted watch is skipped.
	WithAlert(ctx context.Context, id uuid.UUID, evaluate func(w *models.MonitorAlert, txRepo MonitorAlertRepository) error) error
	// GetAlert returns shared alert settings on a monitor, or gorm.ErrRecordNotFound.
	GetAlert(ctx context.Context, monitorID uuid.UUID) (*models.MonitorAlert, error)
	// SaveAlert creates or replaces shared alert settings on a monitor.
	SaveAlert(ctx context.Context, watch *models.MonitorAlert) error
	DeleteAlert(ctx context.Context, monitorID uuid.UUID) error
	// ListAlerts returns the shared alert configuration for a monitor.
	ListAlerts(ctx context.Context, monitorID uuid.UUID) ([]models.MonitorAlert, error)
	// UpdateAlertBreached records the thresholds a run breached, unless a
	// newer run was already evaluated.
	UpdateAlertBreached(ctx context.Context, watchID uuid.UUID, breached []string, evaluatedAt time.Time) error

	// QueueAlerts records delivery jobs in the same transaction as threshold state.
	QueueAlerts(ctx context.Context, watch *models.MonitorAlert, notifications []models.Notification, event events.Event) error
	// AlertDeliveries returns durable jobs for a source run event, including after redelivery.
	AlertDeliveries(ctx context.Context, sourceEventID string) ([]models.EventDelivery, error)
}

type monitorAlertRepository struct {
	db *gorm.DB
}

// NewMonitorAlertRepository creates a MonitorAlertRepository.
func NewMonitorAlertRepository(db *gorm.DB) MonitorAlertRepository {
	return &monitorAlertRepository{db: db}
}

func (r *monitorAlertRepository) WithAlert(ctx context.Context, id uuid.UUID, evaluate func(*models.MonitorAlert, MonitorAlertRepository) error) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var w models.MonitorAlert
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", id).First(&w).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		return evaluate(&w, &monitorAlertRepository{db: tx})
	})
}

func (r *monitorAlertRepository) GetAlert(ctx context.Context, monitorID uuid.UUID) (*models.MonitorAlert, error) {
	var w models.MonitorAlert
	err := r.db.WithContext(ctx).Where("monitor_id = ?", monitorID).First(&w).Error
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *monitorAlertRepository) SaveAlert(ctx context.Context, w *models.MonitorAlert) error {
	var saved models.MonitorAlert
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Insert only when absent, then lock the persisted row. ON CONFLICT
		// also serializes two first-time saves for the same monitor.
		initial := *w
		initial.ID = uuid.New()
		initial.CreatedAt = time.Now().UTC().Truncate(time.Microsecond)
		initial.UpdatedAt = initial.CreatedAt
		initial.Breached = []string{}
		initial.LastEvaluatedAt = nil
		if err := tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "monitor_id"}},
			DoNothing: true,
		}).Create(&initial).Error; err != nil {
			return err
		}
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("monitor_id = ? AND ou_id = ?", w.MonitorID, w.OUID).
			First(&saved).Error; err != nil {
			return err
		}
		// The service's snapshot may precede an evaluation. Preserve the
		// latest committed breaches only for thresholds that still exist.
		keys := make(map[string]bool, len(w.Thresholds))
		for _, threshold := range w.Thresholds {
			keys[threshold.Key()] = true
		}
		breached := []string{}
		for _, key := range saved.Breached {
			if keys[key] {
				breached = append(breached, key)
			}
		}
		saved.NotifyOnFailure, saved.NotifyOnSuccess = w.NotifyOnFailure, w.NotifyOnSuccess
		saved.Thresholds, saved.Breached = w.Thresholds, breached
		saved.DestinationIDs = w.DestinationIDs
		saved.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		return tx.Model(&models.MonitorAlert{}).Where("id = ?", saved.ID).
			Select("notify_on_failure", "notify_on_success", "thresholds", "breached", "destination_ids", "updated_at").
			UpdateColumns(&saved).Error
	})
	if err == nil {
		*w = saved
	}
	return err
}

func (r *monitorAlertRepository) DeleteAlert(ctx context.Context, monitorID uuid.UUID) error {
	return r.db.WithContext(ctx).Where("monitor_id = ?", monitorID).
		Delete(&models.MonitorAlert{}).Error
}

func (r *monitorAlertRepository) ListAlerts(ctx context.Context, monitorID uuid.UUID) ([]models.MonitorAlert, error) {
	var out []models.MonitorAlert
	if err := r.db.WithContext(ctx).Where("monitor_id = ?", monitorID).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *monitorAlertRepository) UpdateAlertBreached(
	ctx context.Context, watchID uuid.UUID, breached []string, evaluatedAt time.Time,
) error {
	// Update from the struct so breached goes through its JSON serializer.
	return r.db.WithContext(ctx).Model(&models.MonitorAlert{}).
		Where("id = ? AND (last_evaluated_at IS NULL OR last_evaluated_at < ?)", watchID, evaluatedAt).
		Select("breached", "last_evaluated_at").
		Updates(&models.MonitorAlert{Breached: breached, LastEvaluatedAt: &evaluatedAt}).Error
}

func (r *monitorAlertRepository) QueueAlerts(ctx context.Context, watch *models.MonitorAlert, notifications []models.Notification, original events.Event) error {
	if watch.OUID != original.OrgID {
		return fmt.Errorf("alert organization does not match source event")
	}
	deliveries := &eventRepository{db: r.db}
	for _, notification := range notifications {
		event := events.New(original.ID+":"+watch.ID.String()+":"+notification.Kind, "monitor.alert."+notification.Kind, events.ScopeAgent, original.OrgID, original.Project, original.Agent, original.Time)
		event.Environment = original.Environment
		event.OrgHandle = original.OrgHandle
		event.Data = map[string]any{"title": notification.Title, "body": notification.Body, "monitor": notification.Target}
		payload, err := json.Marshal(event)
		if err != nil {
			return fmt.Errorf("encode monitor alert: %w", err)
		}
		for _, rawID := range watch.DestinationIDs {
			destinationID, err := uuid.Parse(rawID)
			if err != nil {
				return fmt.Errorf("parse monitor alert destination: %w", err)
			}
			delivery := &models.EventDelivery{
				OUID: watch.OUID, DestinationID: destinationID, SubscriptionIDs: []string{},
				MonitorAlertID: &watch.ID, SourceEventID: original.ID, Payload: payload,
				EventID: event.ID, EventType: event.Name(), EventScope: string(event.Scope),
				ProjectName: event.Project, AgentName: event.Agent, Environment: event.Environment,
				Status: models.EventDeliveryPending,
			}
			if err := deliveries.RecordQueuedDelivery(ctx, delivery); err != nil {
				return fmt.Errorf("queue monitor alert delivery: %w", err)
			}
		}
	}
	return nil
}

func (r *monitorAlertRepository) AlertDeliveries(ctx context.Context, sourceEventID string) ([]models.EventDelivery, error) {
	var deliveries []models.EventDelivery
	err := r.db.WithContext(ctx).Where("monitor_alert_id IS NOT NULL AND source_event_id = ?", sourceEventID).Find(&deliveries).Error
	if err != nil {
		return nil, fmt.Errorf("load monitor alert deliveries: %w", err)
	}
	return deliveries, nil
}
