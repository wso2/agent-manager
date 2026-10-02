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
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

var testAlertKey = []byte("0123456789abcdef0123456789abcdef")

type fakeAlertSender struct {
	code    int
	err     error
	urlErr  error
	sent    []string
	targets []AlertTarget
}

func (f *fakeAlertSender) Send(_ context.Context, target AlertTarget, eventID, _ string, _ []byte) (int, error) {
	f.sent = append(f.sent, eventID)
	f.targets = append(f.targets, target)
	return f.code, f.err
}

func (f *fakeAlertSender) ValidateURL(context.Context, string) error { return f.urlErr }

type alertTestEnv struct {
	repo       *repomocks.AlertRepositoryMock
	scoreRepo  *repomocks.ScoreRepositoryMock
	enqueued   []*models.AlertDelivery
	stateCount int
	stateAt    *time.Time
	stateCalls int
}

func newAlertTestEnv(cfg *models.MonitorAlertConfig, endpointEnabled bool, runEvaluators []models.MonitorRunEvaluator) *alertTestEnv {
	env := &alertTestEnv{}
	env.repo = &repomocks.AlertRepositoryMock{
		GetMonitorAlertConfigFunc: func(_ context.Context, _ uuid.UUID) (*models.MonitorAlertConfig, error) {
			if cfg == nil {
				return nil, gorm.ErrRecordNotFound
			}
			c := *cfg
			return &c, nil
		},
		GetEndpointFunc: func(_ context.Context, ouID string) (*models.AlertEndpoint, error) {
			return &models.AlertEndpoint{OUID: ouID, Enabled: endpointEnabled}, nil
		},
		EnqueueDeliveryFunc: func(_ context.Context, d *models.AlertDelivery) (bool, error) {
			env.enqueued = append(env.enqueued, d)
			return true, nil
		},
		UpdateMonitorAlertStateFunc: func(_ context.Context, _ uuid.UUID, count int, at *time.Time) error {
			env.stateCalls++
			env.stateCount = count
			env.stateAt = at
			return nil
		},
	}
	env.scoreRepo = &repomocks.ScoreRepositoryMock{
		GetEvaluatorsByMonitorAndRunIDFunc: func(_, _ uuid.UUID) ([]models.MonitorRunEvaluator, error) {
			return runEvaluators, nil
		},
	}
	return env
}

func (e *alertTestEnv) service() *alertingService {
	return newAlertingService(e.repo, &repomocks.MonitorRepositoryMock{}, e.scoreRepo, &fakeAlertSender{}, testAlertKey, discardLogger())
}

func testMonitorAndRun() (*models.Monitor, *models.MonitorRun) {
	monitor := &models.Monitor{
		ID: uuid.New(), Name: "prod-quality", Type: models.MonitorTypeFuture, OUID: "org-1", ProjectName: "p", AgentName: "a", EnvironmentName: "prod",
		Evaluators: []models.MonitorEvaluator{{Identifier: "relevancy", DisplayName: "Relevancy"}},
	}
	return monitor, &models.MonitorRun{ID: uuid.New(), MonitorID: monitor.ID}
}

func relevancyRule(value float64) models.MonitorAlertThreshold {
	return models.MonitorAlertThreshold{Evaluator: "Relevancy", Aggregation: "mean", Operator: models.AlertOperatorLT, Value: value}
}

func relevancyScore(mean float64, count int) []models.MonitorRunEvaluator {
	return []models.MonitorRunEvaluator{{EvaluatorName: "Relevancy", Count: count, Aggregations: map[string]interface{}{"mean": mean}}}
}

func TestFindThresholdBreaches(t *testing.T) {
	rules := []models.MonitorAlertThreshold{
		relevancyRule(0.7),
		{Evaluator: "Relevancy", Aggregation: "min", Operator: models.AlertOperatorLTE, Value: 0.2},
		{Evaluator: "Relevancy", Aggregation: "p90", Operator: models.AlertOperatorLT, Value: 0.9},
		{Evaluator: "Missing", Aggregation: "mean", Operator: models.AlertOperatorLT, Value: 0.9},
	}
	evals := []models.MonitorRunEvaluator{{
		EvaluatorName: "Relevancy", Count: 10,
		Aggregations: map[string]interface{}{"mean": 0.65, "min": 0.2},
	}}

	breaches := findThresholdBreaches(rules, evals)

	require.Len(t, breaches, 2, "unreported aggregations and evaluators must not count as breaches")
	assert.Equal(t, "mean", breaches[0].Aggregation)
	assert.InDelta(t, 0.65, breaches[0].Value, 1e-9)
	assert.Equal(t, 10, breaches[0].SampleCount)
	assert.Equal(t, "min", breaches[1].Aggregation, "lte must include the boundary")
}

func TestFindThresholdBreaches_SkipsEvaluatorWithNoScores(t *testing.T) {
	breaches := findThresholdBreaches([]models.MonitorAlertThreshold{relevancyRule(0.7)}, relevancyScore(0, 0))
	assert.Empty(t, breaches)
}

func TestOnMonitorRunFinished_BreachQueuesAlert(t *testing.T) {
	monitor, run := testMonitorAndRun()
	cfg := &models.MonitorAlertConfig{MonitorID: monitor.ID, Enabled: true, Thresholds: []models.MonitorAlertThreshold{relevancyRule(0.7)}, ConsecutiveBreaches: 1, CooldownMinutes: 60}
	env := newAlertTestEnv(cfg, true, relevancyScore(0.5, 4))

	env.service().OnMonitorRunFinished(context.Background(), monitor, run, models.RunStatusSuccess, "")

	require.Len(t, env.enqueued, 1)
	d := env.enqueued[0]
	assert.Equal(t, "mon-breach-"+run.ID.String(), d.EventID, "event IDs are per run so re-evaluation cannot duplicate")
	assert.Equal(t, models.AlertEventMonitorThresholdBreached, d.EventType)
	var event models.AlertEvent
	require.NoError(t, json.Unmarshal([]byte(d.Payload), &event))
	require.Len(t, event.Breaches, 1)
	assert.Equal(t, models.AlertSeverityWarning, event.Severity)
	assert.Equal(t, "prod", event.Environment)
	assert.Equal(t, 1, env.stateCount)
	assert.NotNil(t, env.stateAt)
}

func TestOnMonitorRunFinished_WaitsForConsecutiveBreaches(t *testing.T) {
	monitor, run := testMonitorAndRun()
	cfg := &models.MonitorAlertConfig{MonitorID: monitor.ID, Enabled: true, Thresholds: []models.MonitorAlertThreshold{relevancyRule(0.7)}, ConsecutiveBreaches: 2}
	env := newAlertTestEnv(cfg, true, relevancyScore(0.5, 4))

	env.service().OnMonitorRunFinished(context.Background(), monitor, run, models.RunStatusSuccess, "")

	assert.Empty(t, env.enqueued)
	assert.Equal(t, 1, env.stateCount)
	assert.Nil(t, env.stateAt)
}

func TestOnMonitorRunFinished_CooldownSuppressesAlert(t *testing.T) {
	monitor, run := testMonitorAndRun()
	recent := time.Now().Add(-5 * time.Minute)
	cfg := &models.MonitorAlertConfig{MonitorID: monitor.ID, Enabled: true, AlertOnRunFailure: true, Thresholds: []models.MonitorAlertThreshold{relevancyRule(0.7)}, ConsecutiveBreaches: 1, CooldownMinutes: 60, LastAlertedAt: &recent}
	env := newAlertTestEnv(cfg, true, relevancyScore(0.5, 4))
	svc := env.service()

	svc.OnMonitorRunFinished(context.Background(), monitor, run, models.RunStatusSuccess, "")
	svc.OnMonitorRunFinished(context.Background(), monitor, run, models.RunStatusFailed, "boom")

	assert.Empty(t, env.enqueued)
}

func TestOnMonitorRunFinished_HealthyRunResetsCounter(t *testing.T) {
	monitor, run := testMonitorAndRun()
	cfg := &models.MonitorAlertConfig{MonitorID: monitor.ID, Enabled: true, Thresholds: []models.MonitorAlertThreshold{relevancyRule(0.7)}, ConsecutiveBreaches: 3, ConsecutiveBreachCount: 2}
	env := newAlertTestEnv(cfg, true, relevancyScore(0.9, 4))

	env.service().OnMonitorRunFinished(context.Background(), monitor, run, models.RunStatusSuccess, "")

	assert.Empty(t, env.enqueued, "successes are never alerted")
	assert.Equal(t, 1, env.stateCalls)
	assert.Equal(t, 0, env.stateCount)
}

func TestOnMonitorRunFinished_RunFailure(t *testing.T) {
	monitor, run := testMonitorAndRun()

	t.Run("alerts when enabled", func(t *testing.T) {
		cfg := &models.MonitorAlertConfig{MonitorID: monitor.ID, Enabled: true, AlertOnRunFailure: true}
		env := newAlertTestEnv(cfg, true, nil)
		env.service().OnMonitorRunFinished(context.Background(), monitor, run, models.RunStatusFailed, "workflow completed with failure")
		require.Len(t, env.enqueued, 1)
		assert.Equal(t, models.AlertEventMonitorRunFailed, env.enqueued[0].EventType)
		assert.Contains(t, env.enqueued[0].Payload, "workflow completed with failure")
	})

	t.Run("silent when run-failure alerts are off", func(t *testing.T) {
		cfg := &models.MonitorAlertConfig{MonitorID: monitor.ID, Enabled: true, AlertOnRunFailure: false, Thresholds: []models.MonitorAlertThreshold{relevancyRule(0.7)}}
		env := newAlertTestEnv(cfg, true, nil)
		env.service().OnMonitorRunFinished(context.Background(), monitor, run, models.RunStatusFailed, "x")
		assert.Empty(t, env.enqueued)
	})

	t.Run("silent when org endpoint is disabled", func(t *testing.T) {
		cfg := &models.MonitorAlertConfig{MonitorID: monitor.ID, Enabled: true, AlertOnRunFailure: true}
		env := newAlertTestEnv(cfg, false, nil)
		env.service().OnMonitorRunFinished(context.Background(), monitor, run, models.RunStatusFailed, "x")
		assert.Empty(t, env.enqueued)
	})

	t.Run("silent when monitor has no config", func(t *testing.T) {
		env := newAlertTestEnv(nil, true, nil)
		env.service().OnMonitorRunFinished(context.Background(), monitor, run, models.RunStatusFailed, "x")
		assert.Empty(t, env.enqueued)
	})
}

func TestValidateMonitorAlertConfig(t *testing.T) {
	monitor, _ := testMonitorAndRun()

	cfg := models.MonitorAlertConfig{Enabled: true, Thresholds: []models.MonitorAlertThreshold{{Evaluator: "Relevancy", Aggregation: "mean", Value: 0.7}}}
	require.NoError(t, validateMonitorAlertConfig(monitor, &cfg))
	assert.Equal(t, models.AlertOperatorLT, cfg.Thresholds[0].Operator, "operator defaults to lt")
	assert.Equal(t, models.DefaultAlertConsecutiveBreaches, cfg.ConsecutiveBreaches)

	cases := map[string]models.MonitorAlertConfig{
		"unknown evaluator":   {Thresholds: []models.MonitorAlertThreshold{{Evaluator: "Nope", Aggregation: "mean", Value: 0.5}}},
		"value out of range":  {Thresholds: []models.MonitorAlertThreshold{{Evaluator: "Relevancy", Aggregation: "mean", Value: 1.5}}},
		"bad operator":        {Thresholds: []models.MonitorAlertThreshold{{Evaluator: "Relevancy", Aggregation: "mean", Operator: "gt", Value: 0.5}}},
		"missing aggregation": {Thresholds: []models.MonitorAlertThreshold{{Evaluator: "Relevancy", Value: 0.5}}},
		"duplicate rule": {Thresholds: []models.MonitorAlertThreshold{
			{Evaluator: "Relevancy", Aggregation: "mean", Value: 0.5}, {Evaluator: "Relevancy", Aggregation: "mean", Value: 0.6},
		}},
		"enabled with nothing to alert on": {Enabled: true, AlertOnRunFailure: false},
		"negative cooldown":                {CooldownMinutes: -1},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			c := c
			assert.ErrorIs(t, validateMonitorAlertConfig(monitor, &c), utils.ErrInvalidInput)
		})
	}
}

func TestGetEndpoint_ErrorMapping(t *testing.T) {
	repo := &repomocks.AlertRepositoryMock{
		GetEndpointFunc: func(context.Context, string) (*models.AlertEndpoint, error) { return nil, gorm.ErrRecordNotFound },
	}
	svc := newAlertingService(repo, &repomocks.MonitorRepositoryMock{}, &repomocks.ScoreRepositoryMock{}, &fakeAlertSender{}, testAlertKey, discardLogger())
	_, err := svc.GetEndpoint(context.Background(), "org-1")
	assert.ErrorIs(t, err, utils.ErrAlertEndpointNotFound)

	dbErr := errors.New("connection refused")
	repo.GetEndpointFunc = func(context.Context, string) (*models.AlertEndpoint, error) { return nil, dbErr }
	_, err = svc.GetEndpoint(context.Background(), "org-1")
	assert.ErrorIs(t, err, dbErr)
	assert.NotErrorIs(t, err, utils.ErrAlertEndpointNotFound, "a real error must not be masked as not-found")
}

func TestUpsertEndpoint(t *testing.T) {
	var saved *models.AlertEndpoint
	repo := &repomocks.AlertRepositoryMock{
		GetEndpointFunc: func(context.Context, string) (*models.AlertEndpoint, error) {
			if saved == nil {
				return nil, gorm.ErrRecordNotFound
			}
			c := *saved
			return &c, nil
		},
		UpsertEndpointFunc: func(_ context.Context, e *models.AlertEndpoint) error {
			c := *e
			saved = &c
			return nil
		},
	}
	svc := newAlertingService(repo, &repomocks.MonitorRepositoryMock{}, &repomocks.ScoreRepositoryMock{}, &fakeAlertSender{}, testAlertKey, discardLogger())
	ctx := auditableCtx(t)

	_, _, err := svc.UpsertEndpoint(context.Background(), "org-1", UpsertAlertEndpointInput{URL: "https://alerts.example.com/hook", Enabled: true})
	require.ErrorIs(t, err, audit.ErrRecorderUnavailable, "the endpoint is not changed without an audit record")
	assert.Nil(t, saved)

	endpoint, secret, err := svc.UpsertEndpoint(ctx, "org-1", UpsertAlertEndpointInput{
		URL: "https://alerts.example.com/hook", Enabled: true, Headers: map[string]string{"Authorization": "Bearer abc"},
	})
	require.NoError(t, err)
	assert.NotEmpty(t, secret, "first save generates a signing secret")
	assert.Equal(t, []string{"Authorization"}, endpoint.HeaderNames)
	assert.NotContains(t, string(saved.HeadersEncrypted), "Bearer abc", "header values are stored encrypted")

	target, err := resolveAlertTarget(saved, testAlertKey)
	require.NoError(t, err)
	assert.Equal(t, "Bearer abc", target.Headers["Authorization"])
	assert.Equal(t, secret, target.SigningSecret)

	// Updating without headers keeps them and keeps the secret.
	_, secret2, err := svc.UpsertEndpoint(ctx, "org-1", UpsertAlertEndpointInput{URL: "https://alerts.example.com/v2", Enabled: false})
	require.NoError(t, err)
	assert.Empty(t, secret2)
	assert.Equal(t, []string{"Authorization"}, saved.HeaderNames)

	// Moving to another host without re-entering headers is refused, so the
	// stored Authorization value never reaches the new destination.
	_, _, err = svc.UpsertEndpoint(ctx, "org-1", UpsertAlertEndpointInput{URL: "https://other.example.net/hook", Enabled: true})
	require.ErrorIs(t, err, utils.ErrInvalidInput)
	assert.Equal(t, "https://alerts.example.com/v2", saved.URL, "the rejected update is not saved")

	// An explicit empty object clears the credentials and allows the move.
	_, _, err = svc.UpsertEndpoint(ctx, "org-1", UpsertAlertEndpointInput{URL: "https://other.example.net/hook", Enabled: true, Headers: map[string]string{}})
	require.NoError(t, err)
	assert.Empty(t, saved.HeaderNames)
	assert.Empty(t, saved.HeadersEncrypted)

	_, _, err = svc.UpsertEndpoint(ctx, "org-1", UpsertAlertEndpointInput{URL: "https://x", Headers: map[string]string{"X-AMP-Signature": "forged"}})
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "reserved headers are rejected")
}

func TestSignAlertPayload(t *testing.T) {
	at := time.Unix(1700000000, 0)
	sig := SignAlertPayload("whsec_test", at, []byte(`{"a":1}`))
	assert.Equal(t, sig, SignAlertPayload("whsec_test", at, []byte(`{"a":1}`)))
	assert.NotEqual(t, sig, SignAlertPayload("whsec_other", at, []byte(`{"a":1}`)))
	assert.Contains(t, sig, "t=1700000000,v1=")
}

func TestAlertSender_ValidateURL_RequiresHTTPS(t *testing.T) {
	strict := NewAlertSender(config.Config{})
	err := strict.ValidateURL(context.Background(), "http://alerts.example.com/hook")
	assert.ErrorIs(t, err, utils.ErrInvalidInput, "plain http is rejected when private endpoints are not allowed")

	local := NewAlertSender(config.Config{Alerting: config.AlertingConfig{AllowPrivateEndpoints: true}})
	assert.NoError(t, local.ValidateURL(context.Background(), "http://localhost:8787/hook"),
		"local development keeps accepting http")
}

func TestAlertEndpointOriginChanged(t *testing.T) {
	assert.False(t, alertEndpointOriginChanged("https://a.example.com/v1", "https://A.example.com/v2"), "path change keeps the origin")
	assert.True(t, alertEndpointOriginChanged("https://a.example.com/x", "https://b.example.com/x"))
	assert.True(t, alertEndpointOriginChanged("https://a.example.com/x", "http://a.example.com/x"))
	assert.True(t, alertEndpointOriginChanged("https://a.example.com/x", "https://a.example.com:8443/x"))
}

func TestOnMonitorRunFinished_PastMonitorRerunIgnoresCooldown(t *testing.T) {
	monitor, run := testMonitorAndRun()
	monitor.Type = models.MonitorTypePast
	recent := time.Now().Add(-5 * time.Minute)
	cfg := &models.MonitorAlertConfig{MonitorID: monitor.ID, Enabled: true, AlertOnRunFailure: true, CooldownMinutes: 60, LastAlertedAt: &recent}
	env := newAlertTestEnv(cfg, true, nil)

	env.service().OnMonitorRunFinished(context.Background(), monitor, run, models.RunStatusFailed, "boom")

	require.Len(t, env.enqueued, 1, "a past monitor's rerun alerts even inside the cooldown")
}

func TestAlertSender_RejectsNonHTTPSRedirect(t *testing.T) {
	sender, ok := NewAlertSender(config.Config{}).(*httpAlertSender)
	require.True(t, ok)
	req, err := http.NewRequest(http.MethodPost, "http://alerts.example.com/hook", nil)
	require.NoError(t, err)

	err = sender.client.CheckRedirect(req, nil)

	assert.ErrorIs(t, err, utils.ErrInvalidURL, "an https endpoint must not be followed to plain http")
}

func TestAlertSender_RejectsCrossHostRedirect(t *testing.T) {
	sender, ok := NewAlertSender(config.Config{}).(*httpAlertSender)
	require.True(t, ok)
	orig, err := http.NewRequest(http.MethodPost, "https://alerts.example.com/hook", nil)
	require.NoError(t, err)
	next, err := http.NewRequest(http.MethodPost, "https://attacker.example.net/collect", nil)
	require.NoError(t, err)

	err = sender.client.CheckRedirect(next, []*http.Request{orig})

	assert.ErrorIs(t, err, utils.ErrInvalidURL, "custom headers must not follow a redirect to another host")
}

func TestAlertSender_SendErrorOmitsURL(t *testing.T) {
	sender := &httpAlertSender{client: &http.Client{Timeout: time.Second}, now: time.Now}
	// Nothing listens on port 1, so the request fails with a *url.Error.
	_, err := sender.Send(context.Background(), AlertTarget{URL: "http://127.0.0.1:1/services/T000/B000/SECRETTOKEN"}, "e", "t", []byte("{}"))

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "SECRETTOKEN", "the webhook path must not reach logs or delivery rows")
}

func TestAlertEndpointOrigin(t *testing.T) {
	assert.Equal(t, "https://hooks.example.com", alertEndpointOrigin("https://hooks.example.com/services/T/B/SECRET?x=1"))
	assert.Empty(t, alertEndpointOrigin("::bad"))
}
