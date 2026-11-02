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
	watch := &models.MonitorAlert{
		OUID: monitor.OUID, MonitorID: monitor.ID, DestinationIDs: []string{},
		Thresholds: []models.ScoreThreshold{{Evaluator: "quality", Aggregation: "mean", Operator: "below", Value: 0.7}},
		Breached:   []string{},
	}
	t.Cleanup(func() {
		assert.NoError(t, conn.Where("ou_id = ?", watch.OUID).Delete(&models.EventDelivery{}).Error)
		assert.NoError(t, conn.Delete(monitor).Error)
	})
	require.NoError(t, repo.SaveAlert(context.Background(), watch))
	return repo, watch
}

func TestMonitorAlertWithAlertRollsBackAllWrites(t *testing.T) {
	repo, watch := monitorAlertFixture(t)
	ctx := context.Background()
	failure := errors.New("evaluation aborted")
	destination := monitorAlertDestinationFixture(t, watch.OUID)
	watch.DestinationIDs = []string{destination.ID.String()}
	require.NoError(t, repo.SaveAlert(ctx, watch))
	event := events.New("evt-"+uuid.NewString(), events.TypeMonitorRunSucceeded, events.ScopeAgent, watch.OUID, "project", "agent", time.Now())
	err := repo.WithAlert(ctx, watch.ID, func(w *models.MonitorAlert, tx MonitorAlertRepository) error {
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
	current, err := repo.GetAlert(ctx, watch.MonitorID)
	require.NoError(t, err)
	assert.Empty(t, current.Breached)
	assert.Nil(t, current.LastEvaluatedAt)
	deliveries, err := repo.AlertDeliveries(ctx, event.ID)
	require.NoError(t, err)
	assert.Empty(t, deliveries)
}

func TestMonitorAlertWithAlertSerializesEvaluations(t *testing.T) {
	repo, watch := monitorAlertFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	locked := make(chan struct{})
	release := make(chan struct{})
	firstDone := make(chan error, 1)
	go func() {
		firstDone <- repo.WithAlert(ctx, watch.ID, func(w *models.MonitorAlert, tx MonitorAlertRepository) error {
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
		secondDone <- repo.WithAlert(ctx, watch.ID, func(w *models.MonitorAlert, _ MonitorAlertRepository) error {
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
		assert.Equal(t, []string{watch.Thresholds[0].Key()}, breached)
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func TestMonitorAlertSaveAlertUsesCurrentBreaches(t *testing.T) {
	repo, watch := monitorAlertFixture(t)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(time.Microsecond)
	require.NoError(t, repo.UpdateAlertBreached(ctx, watch.ID, []string{watch.Thresholds[0].Key(), "removed-threshold"}, at))
	// The caller read before the evaluation and sends an empty state. Only
	// settings are accepted; evaluation state comes from the locked row.
	watch.NotifyOnSuccess = true
	watch.Breached = []string{}
	originalID, createdAt := watch.ID, watch.CreatedAt
	require.NoError(t, repo.SaveAlert(ctx, watch))
	assert.Equal(t, originalID, watch.ID)
	assert.True(t, createdAt.Equal(watch.CreatedAt))
	assert.Equal(t, []string{watch.Thresholds[0].Key()}, watch.Breached)
	require.NotNil(t, watch.LastEvaluatedAt)
	assert.True(t, at.Equal(*watch.LastEvaluatedAt))
	stored, err := repo.GetAlert(ctx, watch.MonitorID)
	require.NoError(t, err)
	assert.Equal(t, watch.ID, stored.ID)
	assert.Equal(t, watch.Breached, stored.Breached)
	assert.Equal(t, watch.Thresholds, stored.Thresholds)
	assert.True(t, stored.NotifyOnSuccess)
	assert.True(t, watch.UpdatedAt.Equal(stored.UpdatedAt))
	assert.True(t, watch.CreatedAt.Equal(stored.CreatedAt))
	assert.True(t, watch.LastEvaluatedAt.Equal(*stored.LastEvaluatedAt))
}

func monitorAlertDestinationFixture(t *testing.T, org string) *models.EventDestination {
	t.Helper()
	d := &models.EventDestination{ID: uuid.New(), OUID: org, Name: "alerts", Kind: models.DestinationKindWebhook, Config: json.RawMessage(`{"url":"https://example.com"}`), Availability: models.DestinationAvailableAll, Projects: []string{}}
	repo := NewEventRepository(db.GetDB())
	require.NoError(t, repo.CreateDestination(context.Background(), d))
	t.Cleanup(func() { _, err := repo.DeleteDestination(context.Background(), org, d.ID); assert.NoError(t, err) })
	return d
}

func TestMonitorAlertQueueAlertsPersistsAndDeduplicates(t *testing.T) {
	repo, watch := monitorAlertFixture(t)
	ctx := context.Background()
	destination := monitorAlertDestinationFixture(t, watch.OUID)
	watch.DestinationIDs = []string{destination.ID.String()}
	require.NoError(t, repo.SaveAlert(ctx, watch))
	event := events.New("evt-"+uuid.NewString(), events.TypeMonitorRunFailed, events.ScopeAgent, watch.OUID, "project", "agent", time.Now())
	alerts := []models.Notification{{OUID: watch.OUID, Kind: models.NotificationMonitorRunFailed, Title: "Run failed"}}
	for range 2 {
		require.NoError(t, repo.QueueAlerts(ctx, watch, alerts, event))
	}
	deliveries, err := repo.AlertDeliveries(ctx, event.ID)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, destination.ID, deliveries[0].DestinationID)
	assert.Equal(t, watch.ID, *deliveries[0].MonitorAlertID)
	assert.Equal(t, event.ID, deliveries[0].SourceEventID)
	var payload events.Event
	require.NoError(t, json.Unmarshal(deliveries[0].Payload, &payload))
	assert.Equal(t, watch.OUID, payload.OrgID)
	assert.Equal(t, "monitor.alert."+models.NotificationMonitorRunFailed, payload.Name())
	assert.Equal(t, models.EventDeliveryPending, deliveries[0].Status)
}

func TestMonitorAlertQueueAlertsRejectsOtherOrganization(t *testing.T) {
	repo, watch := monitorAlertFixture(t)
	event := events.New("evt-"+uuid.NewString(), events.TypeMonitorRunFailed, events.ScopeAgent, "other", "project", "agent", time.Now())
	err := repo.QueueAlerts(context.Background(), watch, []models.Notification{{Kind: models.NotificationMonitorRunFailed}}, event)
	require.Error(t, err)
	deliveries, err := repo.AlertDeliveries(context.Background(), event.ID)
	require.NoError(t, err)
	assert.Empty(t, deliveries)
}
