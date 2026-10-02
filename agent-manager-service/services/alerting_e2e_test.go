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

package services

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// A breached run travels the whole path: evaluation queues a delivery, the
// dispatcher POSTs it with the configured headers and a verifiable signature,
// and the outbox records it as sent.
func TestAlerting_EndToEnd_ThresholdBreachIsDelivered(t *testing.T) {
	type received struct {
		header http.Header
		body   []byte
	}
	var mu sync.Mutex
	var got []received
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, received{header: r.Header.Clone(), body: body})
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	defer receiver.Close()

	gdb := db.GetDB()
	alertRepo := repositories.NewAlertRepository(gdb)
	monitorRepo := repositories.NewMonitorRepo(gdb)
	scoreRepo := repositories.NewScoreRepo(gdb)
	sender := NewAlertSender(config.Config{Alerting: config.AlertingConfig{AllowPrivateEndpoints: true}})
	svc := NewAlertingService(alertRepo, monitorRepo, scoreRepo, sender, testAlertKey, discardLogger())
	dispatcher := NewAlertDispatcherService(alertRepo, sender, testAlertKey, discardLogger())
	ctx := auditableCtx(t)

	ouID := "ou-" + uuid.NewString()[:8]
	interval := 60
	monitor := &models.Monitor{
		Name: "e2e-" + uuid.NewString()[:8], Type: models.MonitorTypeFuture, OUID: ouID,
		ProjectName: "p", AgentName: "a", AgentID: "a", EnvironmentName: "prod", EnvironmentID: "prod",
		Evaluators:      []models.MonitorEvaluator{{Identifier: "relevancy", DisplayName: "Relevancy"}},
		IntervalMinutes: &interval, SamplingRate: 1,
	}
	require.NoError(t, monitorRepo.CreateMonitor(monitor))
	t.Cleanup(func() {
		_ = monitorRepo.DeleteMonitor(monitor)
		_, _ = alertRepo.DeleteEndpoint(ctx, ouID)
		gdb.Exec("DELETE FROM alert_deliveries WHERE ou_id = ?", ouID)
	})

	_, secret, err := svc.UpsertEndpoint(ctx, ouID, UpsertAlertEndpointInput{
		URL: receiver.URL + "/hook", Enabled: true, Headers: map[string]string{"Authorization": "Bearer t0ken"},
	})
	require.NoError(t, err)

	_, configured, err := svc.UpdateMonitorAlertConfig(ctx, ouID, "p", "a", monitor.Name, models.MonitorAlertConfig{
		Enabled: true, AlertOnRunFailure: true, CooldownMinutes: 60, ConsecutiveBreaches: 1,
		Thresholds: []models.MonitorAlertThreshold{{Evaluator: "Relevancy", Aggregation: "mean", Value: 0.7}},
	})
	require.NoError(t, err)
	assert.True(t, configured)

	run := &models.MonitorRun{
		MonitorID: monitor.ID, Name: "run-" + uuid.NewString()[:8], Evaluators: monitor.Evaluators,
		TraceStart: time.Now().Add(-time.Hour), TraceEnd: time.Now(), Status: models.RunStatusSuccess,
	}
	require.NoError(t, monitorRepo.CreateMonitorRun(run))
	require.NoError(t, scoreRepo.UpsertMonitorRunEvaluators([]models.MonitorRunEvaluator{{
		MonitorRunID: run.ID, MonitorID: monitor.ID, Identifier: "relevancy", EvaluatorName: "Relevancy",
		Level: "trace", Aggregations: map[string]interface{}{"mean": 0.42}, Count: 12,
	}}))

	svc.OnMonitorRunFinished(ctx, monitor, run, models.RunStatusSuccess, "")
	// A second evaluation of the same run must not queue a duplicate.
	svc.OnMonitorRunFinished(ctx, monitor, run, models.RunStatusSuccess, "")
	dispatcher.RunOnce(ctx)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 1)
	req := got[0]
	assert.Equal(t, "Bearer t0ken", req.header.Get("Authorization"))
	assert.Equal(t, models.AlertEventMonitorThresholdBreached, req.header.Get(AlertHeaderEventType))

	// Verify the signature the way a receiver would.
	sig := req.header.Get(AlertHeaderSignature)
	parts := strings.SplitN(sig, ",", 2)
	require.Len(t, parts, 2)
	ts := strings.TrimPrefix(parts[0], "t=")
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(ts + "."))
	mac.Write(req.body)
	assert.Equal(t, "v1="+hex.EncodeToString(mac.Sum(nil)), parts[1])

	var event models.AlertEvent
	require.NoError(t, json.Unmarshal(req.body, &event))
	require.Len(t, event.Breaches, 1)
	assert.InDelta(t, 0.42, event.Breaches[0].Value, 1e-9)
	assert.Equal(t, 12, event.Breaches[0].SampleCount)
	assert.Equal(t, monitor.Name, event.Monitor.Name)

	deliveries, err := svc.ListDeliveries(ctx, ouID, 10)
	require.NoError(t, err)
	require.Len(t, deliveries, 1)
	assert.Equal(t, models.AlertDeliveryStatusSent, deliveries[0].Status)

	endpoint, err := svc.GetEndpoint(ctx, ouID)
	require.NoError(t, err)
	assert.NotNil(t, endpoint.LastSuccessAt)
}
