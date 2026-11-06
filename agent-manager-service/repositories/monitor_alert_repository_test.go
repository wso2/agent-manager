//go:build integration

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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func monitorAlertFixture(t *testing.T) (MonitorAlertRepository, *models.MonitorAlert) {
	t.Helper()
	conn := db.GetDB()
	monitor := &models.Monitor{
		ID: uuid.New(), Name: "alert-" + uuid.NewString(), OUID: "ou-" + uuid.NewString(),
		ProjectName: "project", AgentName: "agent", Type: models.MonitorTypeFuture,
		EnvironmentName: "development",
	}
	require.NoError(t, conn.Create(monitor).Error)
	repo := NewMonitorAlertRepository(conn)
	alert := &models.MonitorAlert{
		OUID: monitor.OUID, MonitorID: monitor.ID, DestinationIDs: []string{},
		Thresholds: []models.ScoreThreshold{{Evaluator: "quality", Aggregation: "mean", Operator: "below", Value: 0.7}},
		Breached:   []string{},
	}
	t.Cleanup(func() {
		assert.NoError(t, conn.Where("ou_id = ?", alert.OUID).Delete(&models.EventDelivery{}).Error)
		assert.NoError(t, conn.Delete(monitor).Error)
	})
	require.NoError(t, repo.SaveAlert(context.Background(), alert))
	return repo, alert
}

func TestMonitorAlertWithAlertRollsBackAllWrites(t *testing.T) {
	repo, alert := monitorAlertFixture(t)
	ctx := context.Background()
	failure := errors.New("evaluation aborted")
	destination := monitorAlertDestinationFixture(t, alert.OUID)
	alert.DestinationIDs = []string{destination.ID.String()}
	require.NoError(t, repo.SaveAlert(ctx, alert))
	event := events.New("evt-"+uuid.NewString(), events.TypeMonitorRunSucceeded, events.ScopeAgent, alert.OUID, "project", "agent", time.Now())
	err := repo.WithAlert(ctx, alert.ID, func(w *models.MonitorAlert, tx MonitorAlertRepository) error {
		if err := tx.QueueAlerts(ctx, w, []models.Notification{{
			OUID: w.OUID, Kind: models.NotificationScoreBreached, Title: "breached",
		}}, event); err != nil {
			return err
		}
		if err := tx.UpdateAlertBreached(ctx, w.ID, []string{w.Thresholds[0].Key()}, time.Now()); err != nil {
			return err
		}
		return failure
	})
	require.ErrorIs(t, err, failure)
	current, err := repo.GetAlert(ctx, alert.MonitorID)
	require.NoError(t, err)
	assert.Empty(t, current.Breached)
	assert.Nil(t, current.LastEvaluatedAt)
	deliveries, err := repo.AlertDeliveries(ctx, event.ID)
	require.NoError(t, err)
	assert.Empty(t, deliveries)
}

func TestMonitorAlertWithAlertSerializesEvaluations(t *testing.T) {
	repo, alert := monitorAlertFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	locked := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- repo.WithAlert(ctx, alert.ID, func(w *models.MonitorAlert, tx MonitorAlertRepository) error {
			close(locked)
			select {
			case <-release:
			case <-ctx.Done():
				return ctx.Err()
			}
			return tx.UpdateAlertBreached(ctx, w.ID, []string{w.Thresholds[0].Key()}, time.Now())
		})
	}()
	select {
	case <-locked:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	secondEntered := make(chan []string, 1)
	secondDone := make(chan error, 1)
	go func() {
		secondDone <- repo.WithAlert(ctx, alert.ID, func(w *models.MonitorAlert, _ MonitorAlertRepository) error {
			secondEntered <- w.Breached
			return nil
		})
	}()
	select {
	case <-secondEntered:
		t.Error("second evaluation bypassed first alert lock")
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-firstDone)
	require.NoError(t, <-secondDone)
	select {
	case breached := <-secondEntered:
		assert.Equal(t, []string{alert.Thresholds[0].Key()}, breached)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestMonitorAlertSaveAlertUsesCurrentBreaches(t *testing.T) {
	repo, alert := monitorAlertFixture(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, repo.UpdateAlertBreached(ctx, alert.ID, []string{alert.Thresholds[0].Key(), "removed-threshold"}, at))
	// The caller read before the evaluation and sends an empty state. Only
	// settings are accepted; evaluation state comes from the locked row.
	alert.NotifyOnSuccess = true
	alert.Breached = []string{}
	originalID, createdAt := alert.ID, alert.CreatedAt
	require.NoError(t, repo.SaveAlert(ctx, alert))
	assert.Equal(t, originalID, alert.ID)
	assert.True(t, createdAt.Equal(alert.CreatedAt))
	assert.Equal(t, []string{alert.Thresholds[0].Key()}, alert.Breached)
	require.NotNil(t, alert.LastEvaluatedAt)
	assert.True(t, at.Equal(*alert.LastEvaluatedAt))
	stored, err := repo.GetAlert(ctx, alert.MonitorID)
	require.NoError(t, err)
	assert.Equal(t, alert.ID, stored.ID)
	assert.Equal(t, alert.Breached, stored.Breached)
	assert.Equal(t, alert.Thresholds, stored.Thresholds)
	assert.True(t, stored.NotifyOnSuccess)
	assert.True(t, alert.UpdatedAt.Equal(stored.UpdatedAt))
	assert.True(t, alert.CreatedAt.Equal(stored.CreatedAt))
	assert.True(t, alert.LastEvaluatedAt.Equal(*stored.LastEvaluatedAt))
}

func monitorAlertDestinationFixture(t *testing.T, org string) *models.EventDestination {
	t.Helper()
	d := &models.EventDestination{ID: uuid.New(), OUID: org, Name: "alerts-" + uuid.NewString(), Kind: models.DestinationKindWebhook, Config: json.RawMessage(`{"url":"https://example.com"}`), Availability: models.DestinationAvailableAll, Projects: []string{}}
	repo := NewEventRepository(db.GetDB())
	require.NoError(t, repo.CreateDestination(context.Background(), d))
	t.Cleanup(func() {
		assert.NoError(t, db.GetDB().Where("ou_id = ?", org).Delete(&models.MonitorAlert{}).Error)
		assert.NoError(t, db.GetDB().Where("ou_id = ?", org).Delete(&models.EventDelivery{}).Error)
		_, err := repo.DeleteDestination(context.Background(), org, d.ID)
		assert.NoError(t, err)
	})
	return d
}

func TestMonitorAlertQueueAlertsPersistsAndDeduplicates(t *testing.T) {
	repo, alert := monitorAlertFixture(t)
	ctx := context.Background()
	destination := monitorAlertDestinationFixture(t, alert.OUID)
	alert.DestinationIDs = []string{destination.ID.String()}
	require.NoError(t, repo.SaveAlert(ctx, alert))
	event := events.New("evt-"+uuid.NewString(), events.TypeMonitorRunFailed, events.ScopeAgent, alert.OUID, "project", "agent", time.Now())
	alerts := []models.Notification{{OUID: alert.OUID, Kind: models.NotificationMonitorRunFailed, Title: "Run failed"}}
	for range 2 {
		require.NoError(t, repo.QueueAlerts(ctx, alert, alerts, event))
	}
	deliveries, err := repo.AlertDeliveries(ctx, event.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, destination.ID, deliveries[0].DestinationID)
	assert.Equal(t, alert.ID, *deliveries[0].MonitorAlertID)
	assert.Equal(t, event.ID, deliveries[0].SourceEventID)
	var payload events.Event
	require.NoError(t, json.Unmarshal(deliveries[0].Payload, &payload))
	assert.Equal(t, alert.OUID, payload.OrgID)
	assert.Equal(t, "monitor.alert."+models.NotificationMonitorRunFailed, payload.Name())
	assert.Equal(t, models.EventDeliveryPending, deliveries[0].Status)
}

func TestMonitorAlertQueueAlertsRejectsOtherOrganization(t *testing.T) {
	repo, alert := monitorAlertFixture(t)
	event := events.New("evt-"+uuid.NewString(), events.TypeMonitorRunFailed, events.ScopeAgent, "other", "project", "agent", time.Now())
	err := repo.QueueAlerts(context.Background(), alert, []models.Notification{{Kind: models.NotificationMonitorRunFailed}}, event)
	require.Error(t, err)
	deliveries, err := repo.AlertDeliveries(context.Background(), event.ID)
	require.NoError(t, err)
	assert.Empty(t, deliveries)
}

func TestMonitorAlertProtectsDestinationDeletionAndAvailability(t *testing.T) {
	alerts, alert := monitorAlertFixture(t)
	ctx := context.Background()
	destination := monitorAlertDestinationFixture(t, alert.OUID)
	alert.DestinationIDs = []string{destination.ID.String()}
	require.NoError(t, alerts.SaveAlert(ctx, alert))
	repo := NewEventRepository(db.GetDB())
	deleted, err := repo.DeleteDestination(ctx, alert.OUID, destination.ID)
	require.ErrorIs(t, err, utils.ErrEventDestinationInUse)
	assert.False(t, deleted)
	destination.Availability = models.DestinationAvailableOrg
	require.ErrorIs(t, repo.UpdateDestination(ctx, destination, nil), utils.ErrEventDestinationInUse)
	destination.Availability = models.DestinationAvailableProjects
	destination.Projects = []string{"other"}
	require.ErrorIs(t, repo.UpdateDestination(ctx, destination, nil), utils.ErrEventDestinationInUse)
	destination.Projects = []string{"project"}
	require.NoError(t, repo.UpdateDestination(ctx, destination, nil))
	require.NoError(t, alerts.DeleteAlert(ctx, alert.MonitorID))
	deleted, err = repo.DeleteDestination(ctx, alert.OUID, destination.ID)
	require.NoError(t, err)
	assert.True(t, deleted)
	// A save validated before the delete must not recreate a dangling reference.
	require.ErrorIs(t, alerts.SaveAlert(ctx, alert), utils.ErrInvalidInput)
}

func TestMonitorAlertMissingDestinationDoesNotBlockOtherDeliveries(t *testing.T) {
	repo, alert := monitorAlertFixture(t)
	ctx := context.Background()
	missing := monitorAlertDestinationFixture(t, alert.OUID)
	active := monitorAlertDestinationFixture(t, alert.OUID)
	alert.DestinationIDs = []string{missing.ID.String(), active.ID.String()}
	require.NoError(t, repo.SaveAlert(ctx, alert))
	// Simulate data created before the destination-use guard was introduced.
	require.NoError(t, db.GetDB().Delete(missing).Error)
	event := events.New("evt-"+uuid.NewString(), events.TypeMonitorRunSucceeded, events.ScopeAgent, alert.OUID, "project", "agent", time.Now())
	require.NoError(t, repo.WithAlert(ctx, alert.ID, func(current *models.MonitorAlert, tx MonitorAlertRepository) error {
		if err := tx.QueueAlerts(ctx, current, []models.Notification{{Kind: models.NotificationScoreBreached, Title: "breached"}}, event); err != nil {
			return err
		}
		return tx.UpdateAlertBreached(ctx, current.ID, []string{current.Thresholds[0].Key()}, event.Time)
	}))
	deliveries, err := repo.AlertDeliveries(ctx, event.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, active.ID, deliveries[0].DestinationID)
	stored, err := repo.GetAlert(ctx, alert.MonitorID)
	require.NoError(t, err)
	require.NotNil(t, stored.LastEvaluatedAt, "missing destination must not roll back evaluation")
}

// An evaluator removed from the monitor is never scored again, so its
// threshold and breach state go with it; the others are kept.
func TestMonitorAlertKeepThresholdsForRemovedEvaluators(t *testing.T) {
	repo, alert := monitorAlertFixture(t)
	ctx := context.Background()
	latency := models.ScoreThreshold{Evaluator: "latency", Aggregation: "mean", Operator: "above", Value: 0.9}
	alert.Thresholds = append(alert.Thresholds, latency)
	require.NoError(t, repo.SaveAlert(ctx, alert))
	quality := alert.Thresholds[0]
	require.NoError(t, repo.UpdateAlertBreached(ctx, alert.ID, []string{quality.Key(), latency.Key()}, time.Now()))

	require.NoError(t, repo.KeepThresholdsFor(ctx, alert.MonitorID, []string{"latency"}))
	current, err := repo.GetAlert(ctx, alert.MonitorID)
	require.NoError(t, err)
	assert.Equal(t, []models.ScoreThreshold{latency}, current.Thresholds)
	assert.Equal(t, []string{latency.Key()}, current.Breached)

	// Keeping every evaluator changes nothing, and a monitor without alert
	// settings is left alone.
	require.NoError(t, repo.KeepThresholdsFor(ctx, alert.MonitorID, []string{"latency"}))
	unchanged, err := repo.GetAlert(ctx, alert.MonitorID)
	require.NoError(t, err)
	assert.True(t, unchanged.UpdatedAt.Equal(current.UpdatedAt))
	require.NoError(t, repo.KeepThresholdsFor(ctx, uuid.New(), nil))
}
