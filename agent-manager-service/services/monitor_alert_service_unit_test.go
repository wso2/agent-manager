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
	"math"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

var testMonitorRef = MonitorRef{OUID: "ou-1", ProjectName: "proj", AgentName: "agent", MonitorName: "quality"}

func monitorRepoWith(m *models.Monitor, err error) *repomocks.MonitorRepositoryMock {
	return &repomocks.MonitorRepositoryMock{
		GetMonitorByNameFunc: func(string, string, string, string) (*models.Monitor, error) {
			return m, err
		},
	}
}

func testMonitor() *models.Monitor {
	return &models.Monitor{
		ID: uuid.New(), OUID: "ou-1", Name: "quality", ProjectName: "proj", AgentName: "agent",
		Evaluators: []models.MonitorEvaluator{{Identifier: "helpfulness", DisplayName: "Helpfulness"}},
	}
}

func TestMonitorAlertService_MonitorLookupErrors(t *testing.T) {
	t.Run("missing monitor is not found", func(t *testing.T) {
		svc := NewMonitorAlertService(&repomocks.MonitorAlertRepositoryMock{}, monitorRepoWith(nil, gorm.ErrRecordNotFound), &repomocks.EventRepositoryMock{})
		_, _, err := svc.GetAlert(context.Background(), testMonitorRef)
		assert.ErrorIs(t, err, utils.ErrMonitorNotFound)
	})
	t.Run("a database error is not masked as not found", func(t *testing.T) {
		svc := NewMonitorAlertService(&repomocks.MonitorAlertRepositoryMock{}, monitorRepoWith(nil, errors.New("db down")), &repomocks.EventRepositoryMock{})
		err := svc.DeleteAlert(context.Background(), testMonitorRef)
		require.Error(t, err)
		assert.NotErrorIs(t, err, utils.ErrMonitorNotFound)
	})
}

func TestMonitorAlertService_GetAlertWhenDisabled(t *testing.T) {
	repo := &repomocks.MonitorAlertRepositoryMock{
		GetAlertFunc: func(context.Context, uuid.UUID) (*models.MonitorAlert, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := NewMonitorAlertService(repo, monitorRepoWith(testMonitor(), nil), &repomocks.EventRepositoryMock{})
	w, found, err := svc.GetAlert(context.Background(), testMonitorRef)
	require.NoError(t, err)
	assert.False(t, found)
	assert.Nil(t, w)
}

func TestMonitorAlertService_SaveAlertValidatesThresholds(t *testing.T) {
	valid := models.ScoreThreshold{Evaluator: "Helpfulness", Aggregation: "mean", Operator: "below", Value: 0.7}
	tests := []struct {
		name      string
		threshold models.ScoreThreshold
	}{
		{"unknown evaluator", models.ScoreThreshold{Evaluator: "Nope", Aggregation: "mean", Operator: "below", Value: 0.5}},
		{"unknown aggregation", models.ScoreThreshold{Evaluator: "Helpfulness", Aggregation: "p99", Operator: "below", Value: 0.5}},
		{"unknown operator", models.ScoreThreshold{Evaluator: "Helpfulness", Aggregation: "mean", Operator: "equals", Value: 0.5}},
		{"value out of range", models.ScoreThreshold{Evaluator: "Helpfulness", Aggregation: "mean", Operator: "below", Value: 1.5}},
		{"repeats another threshold", models.ScoreThreshold{Evaluator: "Helpfulness", Aggregation: "mean", Operator: "below", Value: 0.2}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// SaveAlertFunc is nil: an invalid alert must not be stored.
			svc := NewMonitorAlertService(&repomocks.MonitorAlertRepositoryMock{}, monitorRepoWith(testMonitor(), nil), &repomocks.EventRepositoryMock{})
			_, err := svc.SaveAlert(context.Background(), testMonitorRef, MonitorAlertSettings{
				Thresholds: []models.ScoreThreshold{valid, tt.threshold},
			})
			assert.ErrorIs(t, err, utils.ErrInvalidInput)
		})
	}
}

func TestMonitorAlertService_MonitorScope(t *testing.T) {
	for _, field := range []string{"org", "project", "agent", "name"} {
		t.Run(field, func(t *testing.T) {
			m := testMonitor()
			switch field {
			case "org":
				m.OUID = "another"
			case "project":
				m.ProjectName = "another"
			case "agent":
				m.AgentName = "another"
			case "name":
				m.Name = "another"
			}
			svc := NewMonitorAlertService(&repomocks.MonitorAlertRepositoryMock{}, monitorRepoWith(m, nil), &repomocks.EventRepositoryMock{})
			_, _, err := svc.GetAlert(context.Background(), testMonitorRef)
			assert.ErrorIs(t, err, utils.ErrMonitorNotFound)
		})
	}
}

func TestMonitorAlertService_SaveAlertDestinations(t *testing.T) {
	id := uuid.New()
	dbErr := errors.New("database unavailable")
	tests := []struct {
		name        string
		ids         []string
		destination *models.EventDestination
		lookupErr   error
		wantErr     error
	}{
		{name: "no destinations", wantErr: utils.ErrInvalidInput},
		{name: "invalid UUID", ids: []string{"bad"}, wantErr: utils.ErrInvalidInput},
		{name: "zero UUID", ids: []string{uuid.Nil.String()}, wantErr: utils.ErrInvalidInput},
		{name: "duplicate", ids: []string{id.String(), id.String()}, destination: &models.EventDestination{OUID: "ou-1", Availability: models.DestinationAvailableAll}, wantErr: utils.ErrInvalidInput},
		{name: "too many", ids: make([]string, 21), wantErr: utils.ErrInvalidInput},
		{name: "missing destination", ids: []string{id.String()}, lookupErr: gorm.ErrRecordNotFound, wantErr: utils.ErrInvalidInput},
		{name: "lookup failure", ids: []string{id.String()}, lookupErr: dbErr, wantErr: dbErr},
		{name: "other org", ids: []string{id.String()}, destination: &models.EventDestination{OUID: "other", Availability: models.DestinationAvailableAll}, wantErr: utils.ErrInvalidInput},
		{name: "org only", ids: []string{id.String()}, destination: &models.EventDestination{OUID: "ou-1"}, wantErr: utils.ErrInvalidInput},
		{name: "other project", ids: []string{id.String()}, destination: &models.EventDestination{OUID: "ou-1", Availability: models.DestinationAvailableProjects, Projects: []string{"other"}}, wantErr: utils.ErrInvalidInput},
		{name: "all projects", ids: []string{id.String()}, destination: &models.EventDestination{OUID: "ou-1", Availability: models.DestinationAvailableAll}},
		{name: "selected project", ids: []string{id.String()}, destination: &models.EventDestination{OUID: "ou-1", Availability: models.DestinationAvailableProjects, Projects: []string{"proj"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var saved *models.MonitorAlert
			repo := &repomocks.MonitorAlertRepositoryMock{}
			if tt.wantErr == nil {
				repo.SaveAlertFunc = func(_ context.Context, w *models.MonitorAlert) error { saved = w; return nil }
			}
			eventRepo := &repomocks.EventRepositoryMock{GetDestinationFunc: func(_ context.Context, org string, got uuid.UUID) (*models.EventDestination, error) {
				assert.Equal(t, "ou-1", org)
				assert.Equal(t, id, got)
				return tt.destination, tt.lookupErr
			}}
			m := testMonitor()
			svc := NewMonitorAlertService(repo, monitorRepoWith(m, nil), eventRepo)
			result, err := svc.SaveAlert(context.Background(), testMonitorRef, MonitorAlertSettings{NotifyOnFailure: true, DestinationIDs: tt.ids})
			if tt.wantErr != nil {
				assert.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, saved)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, saved)
			assert.Same(t, saved, result)
			assert.Equal(t, m.ID, saved.MonitorID)
			assert.Equal(t, m.OUID, saved.OUID)
			assert.Equal(t, tt.ids, saved.DestinationIDs)
		})
	}
}

func TestMonitorAlertService_SaveAlertRequiresCondition(t *testing.T) {
	svc := NewMonitorAlertService(&repomocks.MonitorAlertRepositoryMock{}, monitorRepoWith(testMonitor(), nil), &repomocks.EventRepositoryMock{})
	_, err := svc.SaveAlert(context.Background(), testMonitorRef, MonitorAlertSettings{DestinationIDs: []string{uuid.NewString()}})
	assert.ErrorIs(t, err, utils.ErrInvalidInput)
}

func TestMonitorAlertService_RejectsNonfiniteThreshold(t *testing.T) {
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		svc := NewMonitorAlertService(&repomocks.MonitorAlertRepositoryMock{}, monitorRepoWith(testMonitor(), nil), &repomocks.EventRepositoryMock{})
		_, err := svc.SaveAlert(context.Background(), testMonitorRef, MonitorAlertSettings{Thresholds: []models.ScoreThreshold{{Evaluator: "Helpfulness", Aggregation: "mean", Operator: "below", Value: value}}})
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	}
}

func TestMonitorAlertService_DeleteSharedSettings(t *testing.T) {
	m := testMonitor()
	called := false
	repo := &repomocks.MonitorAlertRepositoryMock{DeleteAlertFunc: func(_ context.Context, id uuid.UUID) error { called = true; assert.Equal(t, m.ID, id); return nil }}
	svc := NewMonitorAlertService(repo, monitorRepoWith(m, nil), &repomocks.EventRepositoryMock{})
	require.NoError(t, svc.DeleteAlert(context.Background(), testMonitorRef))
	assert.True(t, called)
}
