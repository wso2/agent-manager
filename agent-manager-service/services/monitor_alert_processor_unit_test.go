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
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

var testRunTime = time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)

// monitorRunEventBytes builds a published monitor run event.
func monitorRunEventBytes(t *testing.T, name string, monitorID uuid.UUID, data map[string]any) []byte {
	t.Helper()
	e := events.New("evt_monrun_1", name, events.ScopeAgent, "ou-1", "proj", "agent", testRunTime)
	e.Environment = "dev"
	monitor := map[string]any{"id": monitorID.String(), "name": "quality", "displayName": "Quality"}
	payload := map[string]any{"monitor": monitor}
	for k, v := range data {
		payload[k] = v
	}
	e.Data = payload
	raw, err := json.Marshal(e)
	require.NoError(t, err)
	return raw
}

func scoresData(evaluator string, mean float64) map[string]any {
	return map[string]any{
		"run": map[string]any{"id": "run-1", "status": "success"},
		"scores": []map[string]any{{
			"evaluator": evaluator, "level": "trace", "aggregations": map[string]any{"mean": mean},
		}},
	}
}

func newTestMonitorAlerts(repo *repomocks.MonitorAlertRepositoryMock) *monitorAlertProcessor {
	if repo.WithAlertFunc == nil {
		repo.WithAlertFunc = func(ctx context.Context, id uuid.UUID, evaluate func(*models.MonitorAlert, repositories.MonitorAlertRepository) error) error {
			watches, err := repo.ListAlerts(ctx, uuid.Nil)
			if err != nil {
				return err
			}
			for i := range watches {
				if watches[i].ID == id {
					return evaluate(&watches[i], repo)
				}
			}
			return nil
		}
	}
	if repo.AlertDeliveriesFunc == nil {
		repo.AlertDeliveriesFunc = func(context.Context, string) ([]models.EventDelivery, error) { return []models.EventDelivery{}, nil }
	}
	return NewMonitorAlertProcessor(nil, repo, discardSlog()).(*monitorAlertProcessor)
}

func TestMonitorAlerts_IgnoresOtherEvents(t *testing.T) {
	// A nil ListAlertsFunc panics if called: other events must not reach the
	// repository.
	center := newTestMonitorAlerts(&repomocks.MonitorAlertRepositoryMock{})
	e := events.New("evt_1", "project.created", events.ScopeProject, "ou-1", "proj", "", testRunTime)
	raw, err := json.Marshal(e)
	require.NoError(t, err)

	msg := &answeredMessage{data: raw}
	center.handleEvent(context.Background(), msg)
	assert.Equal(t, "ack", msg.outcome)
}

func TestMonitorAlerts_QueuesAlertsAndRecordsBreaches(t *testing.T) {
	monitorID := uuid.New()
	threshold := models.ScoreThreshold{
		Evaluator: "Helpfulness", Aggregation: "mean", Operator: models.ThresholdBelow, Value: 0.7,
	}
	watcher := models.MonitorAlert{
		ID: uuid.New(), OUID: "ou-1", MonitorID: monitorID,
		Thresholds: []models.ScoreThreshold{threshold},
	}
	// Alert settings from another org on the same monitor ID is never notified.
	stranger := models.MonitorAlert{
		ID: uuid.New(), OUID: "ou-2", MonitorID: monitorID, NotifyOnSuccess: true,
	}

	var created []models.Notification
	var breached []string
	var evaluatedAt time.Time
	repo := &repomocks.MonitorAlertRepositoryMock{
		ListAlertsFunc: func(_ context.Context, id uuid.UUID) ([]models.MonitorAlert, error) {
			assert.True(t, id == monitorID || id == uuid.Nil)
			return []models.MonitorAlert{watcher, stranger}, nil
		},
		QueueAlertsFunc: func(_ context.Context, _ *models.MonitorAlert, n []models.Notification, _ events.Event) error {
			created = append(created, n...)
			return nil
		},
		UpdateAlertBreachedFunc: func(_ context.Context, id uuid.UUID, keys []string, at time.Time) error {
			assert.Equal(t, watcher.ID, id)
			breached, evaluatedAt = keys, at
			return nil
		},
	}
	center := newTestMonitorAlerts(repo)

	msg := &answeredMessage{data: monitorRunEventBytes(t, events.TypeMonitorRunSucceeded, monitorID, scoresData("Helpfulness", 0.62))}
	center.handleEvent(context.Background(), msg)

	assert.Equal(t, "ack", msg.outcome)
	require.Len(t, created, 1)
	assert.Equal(t, models.NotificationScoreBreached, created[0].Kind)
	assert.Equal(t, "evt_monrun_1", created[0].EventID)
	assert.Equal(t, "run-1", created[0].Target.RunID)
	assert.Equal(t, "dev", created[0].Target.EnvironmentName)
	assert.Equal(t, []string{threshold.Key()}, breached)
	assert.True(t, evaluatedAt.Equal(testRunTime))
}

func TestMonitorAlerts_RetriesOnStoreErrors(t *testing.T) {
	monitorID := uuid.New()
	watch := models.MonitorAlert{ID: uuid.New(), OUID: "ou-1", MonitorID: monitorID, NotifyOnFailure: true}
	failed := monitorRunEventBytes(t, events.TypeMonitorRunFailed, monitorID,
		map[string]any{"run": map[string]any{"id": "run-1", "status": "failed", "error": "boom"}})

	t.Run("loading alert settings", func(t *testing.T) {
		repo := &repomocks.MonitorAlertRepositoryMock{
			ListAlertsFunc: func(context.Context, uuid.UUID) ([]models.MonitorAlert, error) {
				return nil, errors.New("db down")
			},
		}
		msg := &answeredMessage{data: failed}
		newTestMonitorAlerts(repo).handleEvent(context.Background(), msg)
		assert.Equal(t, "retry", msg.outcome)
	})

	t.Run("queueing alerts", func(t *testing.T) {
		repo := &repomocks.MonitorAlertRepositoryMock{
			ListAlertsFunc: func(context.Context, uuid.UUID) ([]models.MonitorAlert, error) {
				return []models.MonitorAlert{watch}, nil
			},
			QueueAlertsFunc: func(context.Context, *models.MonitorAlert, []models.Notification, events.Event) error {
				return errors.New("db down")
			},
		}
		msg := &answeredMessage{data: failed}
		newTestMonitorAlerts(repo).handleEvent(context.Background(), msg)
		assert.Equal(t, "retry", msg.outcome)
	})
}

func TestMonitorAlerts_DropsMalformedEvents(t *testing.T) {
	msg := &answeredMessage{data: []byte("not json")}
	newTestMonitorAlerts(&repomocks.MonitorAlertRepositoryMock{}).handleEvent(context.Background(), msg)
	assert.Equal(t, "drop", msg.outcome)
}

// alertRetryBus fails the first enqueue to reproduce a crash/transport outage
// after the outcome and threshold state have already committed.
type alertRetryBus struct {
	queueBus
	fail     bool
	attempts int
}

func (b *alertRetryBus) Enqueue(ctx context.Context, queue, key string, body []byte) error {
	b.attempts++
	if b.fail {
		b.fail = false
		return errors.New("queue unavailable")
	}
	return b.queueBus.Enqueue(ctx, queue, key, body)
}

func TestMonitorAlerts_RedeliveryRequeuesDurableAlert(t *testing.T) {
	monitorID, destinationID := uuid.New(), uuid.New()
	threshold := models.ScoreThreshold{Evaluator: "Helpfulness", Aggregation: "mean", Operator: models.ThresholdBelow, Value: 0.7}
	watch := models.MonitorAlert{ID: uuid.New(), OUID: "ou-1", MonitorID: monitorID, DestinationIDs: []string{destinationID.String()}, Thresholds: []models.ScoreThreshold{threshold}}
	var durable []models.EventDelivery
	outcomes := 0
	repo := &repomocks.MonitorAlertRepositoryMock{
		ListAlertsFunc: func(context.Context, uuid.UUID) ([]models.MonitorAlert, error) {
			return []models.MonitorAlert{watch}, nil
		},
		QueueAlertsFunc: func(_ context.Context, _ *models.MonitorAlert, alerts []models.Notification, e events.Event) error {
			outcomes += len(alerts)
			if len(alerts) > 0 {
				payload, err := json.Marshal(e)
				if err != nil {
					return err
				}
				durable = append(durable, models.EventDelivery{EventID: "derived-alert", DestinationID: destinationID, Payload: payload})
			}
			return nil
		},
		UpdateAlertBreachedFunc: func(_ context.Context, _ uuid.UUID, keys []string, at time.Time) error {
			watch.Breached = keys
			watch.LastEvaluatedAt = &at
			return nil
		},
		AlertDeliveriesFunc: func(_ context.Context, source string) ([]models.EventDelivery, error) {
			assert.Equal(t, "evt_monrun_1", source)
			return durable, nil
		},
	}
	repo.WithAlertFunc = func(_ context.Context, _ uuid.UUID, evaluate func(*models.MonitorAlert, repositories.MonitorAlertRepository) error) error {
		return evaluate(&watch, repo)
	}
	bus := &alertRetryBus{queueBus: queueBus{enqueued: make(map[string][]byte)}, fail: true}
	center := NewMonitorAlertProcessor(bus, repo, discardSlog()).(*monitorAlertProcessor)
	raw := monitorRunEventBytes(t, events.TypeMonitorRunSucceeded, monitorID, scoresData("Helpfulness", 0.62))
	first := &answeredMessage{data: raw}
	center.handleEvent(context.Background(), first)
	assert.Equal(t, "retry", first.outcome)
	require.Len(t, watch.Breached, 1)
	second := &answeredMessage{data: raw}
	center.handleEvent(context.Background(), second)
	assert.Equal(t, "ack", second.outcome)
	assert.Equal(t, 1, outcomes, "redelivery must not create another threshold crossing")
	assert.Equal(t, 2, bus.attempts)
	require.Len(t, bus.enqueued, 1)
	var job eventDeliveryJob
	require.NoError(t, json.Unmarshal(bus.enqueued["derived-alert:"+destinationID.String()], &job))
	assert.Equal(t, destinationID.String(), job.DestinationID)
}

func TestMonitorAlerts_RechecksOrganizationUnderLock(t *testing.T) {
	monitorID := uuid.New()
	repo := &repomocks.MonitorAlertRepositoryMock{
		ListAlertsFunc: func(context.Context, uuid.UUID) ([]models.MonitorAlert, error) {
			return []models.MonitorAlert{{ID: uuid.New(), OUID: "ou-1", MonitorID: monitorID}}, nil
		},
		WithAlertFunc: func(_ context.Context, _ uuid.UUID, evaluate func(*models.MonitorAlert, repositories.MonitorAlertRepository) error) error {
			return evaluate(&models.MonitorAlert{OUID: "other", NotifyOnFailure: true}, &repomocks.MonitorAlertRepositoryMock{})
		},
	}
	msg := &answeredMessage{data: monitorRunEventBytes(t, events.TypeMonitorRunFailed, monitorID, map[string]any{"run": map[string]any{"id": "run-1", "status": "failed"}})}
	newTestMonitorAlerts(repo).handleEvent(context.Background(), msg)
	assert.Equal(t, "ack", msg.outcome)
}
