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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func newTestAlertDelivery(ouID string) *models.AlertDelivery {
	now := time.Now().Add(-time.Second)
	return &models.AlertDelivery{
		OUID:          ouID,
		EventID:       "evt-" + uuid.NewString(),
		EventType:     models.AlertEventMonitorRunFailed,
		MonitorName:   "m",
		Payload:       `{"type":"monitor.run_failed"}`,
		Status:        models.AlertDeliveryStatusPending,
		NextAttemptAt: &now,
	}
}

func TestAlertRepository_EndpointUpsertAndOutcome(t *testing.T) {
	repo := NewAlertRepository(db.GetDB())
	ctx := context.Background()
	ouID := "ou-" + uuid.NewString()[:8]
	t.Cleanup(func() { _, _ = repo.DeleteEndpoint(ctx, ouID) })

	require.NoError(t, repo.UpsertEndpoint(ctx, &models.AlertEndpoint{
		OUID: ouID, URL: "https://a.example.com", Enabled: true,
		HeaderNames: []string{"Authorization"}, HeadersEncrypted: []byte{1, 2, 3},
		SigningSecretEncrypted: []byte{4, 5, 6},
	}))
	require.NoError(t, repo.UpsertEndpoint(ctx, &models.AlertEndpoint{
		OUID: ouID, URL: "https://b.example.com", Enabled: false,
		HeaderNames: []string{}, SigningSecretEncrypted: []byte{7},
	}))
	require.NoError(t, repo.RecordEndpointOutcome(ctx, ouID, false, time.Now()))
	require.NoError(t, repo.RecordEndpointOutcome(ctx, ouID, false, time.Now()))

	got, err := repo.GetEndpoint(ctx, ouID)
	require.NoError(t, err)
	assert.Equal(t, "https://b.example.com", got.URL, "upsert replaces the org's single endpoint")
	assert.False(t, got.Enabled)
	assert.Empty(t, got.HeaderNames)
	assert.Equal(t, []byte{7}, got.SigningSecretEncrypted)
	assert.Equal(t, 2, got.ConsecutiveFailures)

	require.NoError(t, repo.RecordEndpointOutcome(ctx, ouID, true, time.Now()))
	got, err = repo.GetEndpoint(ctx, ouID)
	require.NoError(t, err)
	assert.Equal(t, 0, got.ConsecutiveFailures)
	assert.NotNil(t, got.LastSuccessAt)

	deleted, err := repo.DeleteEndpoint(ctx, ouID)
	require.NoError(t, err)
	assert.True(t, deleted)
	deleted, err = repo.DeleteEndpoint(ctx, ouID)
	require.NoError(t, err)
	assert.False(t, deleted)
}

func TestAlertRepository_DeliveryQueue(t *testing.T) {
	repo := NewAlertRepository(db.GetDB())
	ctx := context.Background()
	ouID := "ou-" + uuid.NewString()[:8]

	d := newTestAlertDelivery(ouID)
	created, err := repo.EnqueueDelivery(ctx, d)
	require.NoError(t, err)
	assert.True(t, created)

	dup := newTestAlertDelivery(ouID)
	dup.EventID = d.EventID
	created, err = repo.EnqueueDelivery(ctx, dup)
	require.NoError(t, err)
	assert.False(t, created, "the same event is queued once")

	claimed, err := repo.ClaimDueDeliveries(ctx, time.Now(), time.Minute, 500)
	require.NoError(t, err)
	var mine *models.AlertDelivery
	for i := range claimed {
		if claimed[i].EventID == d.EventID {
			mine = &claimed[i]
		}
	}
	require.NotNil(t, mine)
	assert.JSONEq(t, `{"type":"monitor.run_failed"}`, mine.Payload)

	again, err := repo.ClaimDueDeliveries(ctx, time.Now(), time.Minute, 500)
	require.NoError(t, err)
	for _, row := range again {
		assert.NotEqual(t, d.EventID, row.EventID, "a claimed delivery is leased, not re-claimed")
	}

	code := 503
	require.NoError(t, repo.MarkDeliveryRetry(ctx, mine.ID, 1, &code, "unavailable", time.Now().Add(time.Hour)))
	require.NoError(t, repo.MarkDeliverySent(ctx, mine.ID, 2, 200, time.Now()))

	list, err := repo.ListDeliveries(ctx, ouID, 10)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, models.AlertDeliveryStatusSent, list[0].Status)
	assert.Equal(t, 2, list[0].Attempts)
	require.NotNil(t, list[0].LastResponseCode)
	assert.Equal(t, 200, *list[0].LastResponseCode)
	assert.Nil(t, list[0].NextAttemptAt)
}

func TestAlertRepository_MonitorConfig(t *testing.T) {
	gdb := db.GetDB()
	repo := NewAlertRepository(gdb)
	monitorRepo := NewMonitorRepo(gdb)
	ctx := context.Background()

	interval := 60
	monitor := &models.Monitor{
		Name: "alert-" + uuid.NewString()[:8], Type: models.MonitorTypeFuture, OUID: "ou-" + uuid.NewString()[:8],
		ProjectName: "p", AgentName: "a", AgentID: "a", EnvironmentName: "dev", EnvironmentID: "dev",
		Evaluators: []models.MonitorEvaluator{}, IntervalMinutes: &interval, SamplingRate: 1,
	}
	require.NoError(t, monitorRepo.CreateMonitor(monitor))
	t.Cleanup(func() { _ = monitorRepo.DeleteMonitor(monitor) })

	cfg := &models.MonitorAlertConfig{
		MonitorID: monitor.ID, Enabled: true, AlertOnRunFailure: true,
		Thresholds:      []models.MonitorAlertThreshold{{Evaluator: "Relevancy", Aggregation: "mean", Operator: "lt", Value: 0.7}},
		CooldownMinutes: 30, ConsecutiveBreaches: 2,
	}
	require.NoError(t, repo.UpsertMonitorAlertConfig(ctx, cfg))
	now := time.Now()
	require.NoError(t, repo.UpdateMonitorAlertState(ctx, monitor.ID, 3, &now))

	got, err := repo.GetMonitorAlertConfig(ctx, monitor.ID)
	require.NoError(t, err)
	assert.Equal(t, 3, got.ConsecutiveBreachCount)
	require.Len(t, got.Thresholds, 1)
	assert.InDelta(t, 0.7, got.Thresholds[0].Value, 1e-9)

	cfg.CooldownMinutes = 5
	require.NoError(t, repo.UpsertMonitorAlertConfig(ctx, cfg))
	got, err = repo.GetMonitorAlertConfig(ctx, monitor.ID)
	require.NoError(t, err)
	assert.Equal(t, 5, got.CooldownMinutes)
	assert.Equal(t, 0, got.ConsecutiveBreachCount, "changing the rules resets the breach counter")
	assert.NotNil(t, got.LastAlertedAt, "the cooldown clock survives a rule change")
}
