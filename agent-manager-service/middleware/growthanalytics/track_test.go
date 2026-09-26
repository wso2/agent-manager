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

package growthanalytics

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/moesifcollector"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
)

// callerToken is the fake caller JWT tests attach to requests, standing in
// for the real bearer token an authenticated caller would carry — Track
// forwards whatever is on the request, never a config value, so tests must
// supply one for the send path to run at all.
const callerToken = "test-caller-jwt"

// authedRequest builds a request as Track would see it after the JWT
// assertion middleware has run: carrying a caller bearer token on its
// context, per jwtassertion.GetJWTFromContext.
func authedRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	return req.WithContext(jwtassertion.ContextWithJWT(req.Context(), callerToken))
}

// withGrowthAnalyticsConfig sets the collector fields on the process-wide
// config for the duration of the test, restoring the originals on cleanup —
// same pattern used elsewhere in this repo (see api/well_known_routes_test.go)
// since config.GetConfig() returns a pointer to the single package-level
// Config. IsOnPremDeployment is deliberately not touched here: Track doesn't
// consult it — see TestTrack_ActivatesRegardlessOfIsOnPremDeployment.
func withGrowthAnalyticsConfig(t *testing.T, baseURL string) {
	t.Helper()
	withGrowthAnalyticsConfigFull(t, config.GrowthAnalyticsConfig{
		Enabled:                true,
		MoesifCollectorBaseURL: baseURL,
		DeploymentModel:        "saas",
		Environment:            "development",
	})
}

// withGrowthAnalyticsConfigFull is withGrowthAnalyticsConfig for the tests
// that need to vary a field the common helper fixes — the kill switch and
// the reported deployment model.
func withGrowthAnalyticsConfigFull(t *testing.T, ga config.GrowthAnalyticsConfig) {
	t.Helper()
	cfg := config.GetConfig()
	origGA := cfg.GrowthAnalytics
	cfg.GrowthAnalytics = ga
	t.Cleanup(func() {
		cfg.GrowthAnalytics = origGA
	})
}

// fakeSender substitutes for the real moesifcollector.Client, so tests never
// make a network call. Each SendEvent also publishes on calls, since Track
// dispatches the send on a detached goroutine — tests need a way to wait
// deterministically for it rather than racing a background send. It also
// records the token newSender was built with, so tests can assert Track
// forwards the caller's own JWT rather than a config credential.
type fakeSender struct {
	mu    sync.Mutex
	sent  []moesifcollector.Event
	err   error
	calls chan moesifcollector.Event
}

func newFakeSender() *fakeSender {
	return &fakeSender{calls: make(chan moesifcollector.Event, 8)}
}

func (f *fakeSender) SendEvent(_ context.Context, evt moesifcollector.Event) error {
	f.mu.Lock()
	f.sent = append(f.sent, evt)
	f.mu.Unlock()
	f.calls <- evt
	return f.err
}

// withFakeSender substitutes newSender for the duration of the test,
// restoring the original on cleanup, and captures every token newSender was
// called with.
func withFakeSender(t *testing.T) (*fakeSender, *[]string) {
	t.Helper()
	fs := newFakeSender()
	var tokens []string
	orig := newSender
	newSender = func(_ config.GrowthAnalyticsConfig, token string) eventSender {
		tokens = append(tokens, token)
		return fs
	}
	t.Cleanup(func() { newSender = orig })
	return fs, &tokens
}

func waitForEvent(t *testing.T, fs *fakeSender) moesifcollector.Event {
	t.Helper()
	select {
	case evt := <-fs.calls:
		return evt
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for event to be sent to the collector")
		return moesifcollector.Event{}
	}
}

func TestTrack_NoOp_NoBaseURL(t *testing.T) {
	withGrowthAnalyticsConfig(t, "")

	orig := newSender
	newSender = func(config.GrowthAnalyticsConfig, string) eventSender {
		t.Fatal("newSender must not be called when growth analytics is disabled")
		return nil
	}
	t.Cleanup(func() { newSender = orig })

	ran := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		ran = true
		w.WriteHeader(http.StatusCreated)
	}

	wrapped := Track("amp.agent-development.create-agent", map[string]interface{}{"creation_method": "platform-hosted"}, handler)

	rec := httptest.NewRecorder()
	wrapped(rec, authedRequest(http.MethodPost, "/agents"))

	if !ran {
		t.Error("expected the wrapped handler to run even when tracking is disabled")
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

// TestTrack_NoOp_WhenDisabled covers the kill switch: a fully configured
// collector URL is not on its own permission to report. MOESIF_ENABLED=false
// turns export off while leaving the rest of the configuration in place,
// which is how an environment stops reporting without deleting config.
func TestTrack_NoOp_WhenDisabled(t *testing.T) {
	withGrowthAnalyticsConfigFull(t, config.GrowthAnalyticsConfig{
		Enabled:                false,
		MoesifCollectorBaseURL: "http://localhost:18080/moesif-collector",
		DeploymentModel:        "saas",
	})

	orig := newSender
	newSender = func(config.GrowthAnalyticsConfig, string) eventSender {
		t.Fatal("newSender must not be called when growth analytics is switched off")
		return nil
	}
	t.Cleanup(func() { newSender = orig })

	ran := false
	handler := func(w http.ResponseWriter, r *http.Request) {
		ran = true
		w.WriteHeader(http.StatusCreated)
	}

	wrapped := Track("amp.agent-development.create-agent", nil, handler)

	rec := httptest.NewRecorder()
	wrapped(rec, authedRequest(http.MethodPost, "/agents"))

	if !ran {
		t.Error("expected the wrapped handler to run even when tracking is switched off")
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

// TestTrack_ReportsConfiguredDeploymentModel verifies the event's
// deployment_model comes from config rather than a compiled-in "saas", so a
// deployment that is not the cloud one labels its events honestly.
func TestTrack_ReportsConfiguredDeploymentModel(t *testing.T) {
	withGrowthAnalyticsConfigFull(t, config.GrowthAnalyticsConfig{
		Enabled:                true,
		MoesifCollectorBaseURL: "http://localhost:18080/moesif-collector",
		DeploymentModel:        "vm",
	})
	fs, _ := withFakeSender(t)

	tracked := Track("amp.agent-development.create-agent", nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	tracked(httptest.NewRecorder(), authedRequest(http.MethodPost, "/agents"))

	evt := waitForEvent(t, fs)
	if got := evt.Metadata["deployment_model"]; got != "vm" {
		t.Errorf("metadata[deployment_model] = %v, want %q", got, "vm")
	}
}

// TestTrack_ReportsConfiguredEnvironment verifies each event carries the
// environment it came from, which is what keeps dev/stage/prod usage
// separable inside the single shared Moesif application.
func TestTrack_ReportsConfiguredEnvironment(t *testing.T) {
	withGrowthAnalyticsConfigFull(t, config.GrowthAnalyticsConfig{
		Enabled:                true,
		MoesifCollectorBaseURL: "http://localhost:18080/moesif-collector",
		DeploymentModel:        "saas",
		Environment:            "production",
	})
	fs, _ := withFakeSender(t)

	tracked := Track("amp.agent-development.create-agent", nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	tracked(httptest.NewRecorder(), authedRequest(http.MethodPost, "/agents"))

	evt := waitForEvent(t, fs)
	if got := evt.Metadata["environment"]; got != "production" {
		t.Errorf("metadata[environment] = %v, want %q", got, "production")
	}
}

// TestTrack_ActivatesRegardlessOfIsOnPremDeployment locks in a deliberate
// design choice: Track does not consult IsOnPremDeployment. Enabled plus
// MoesifCollectorBaseURL are the only gates (both off by default), and
// deployment_model records whether the reporting install is on-prem or SaaS.
func TestTrack_ActivatesRegardlessOfIsOnPremDeployment(t *testing.T) {
	cfg := config.GetConfig()
	origOnPrem := cfg.IsOnPremDeployment
	cfg.IsOnPremDeployment = true
	t.Cleanup(func() { cfg.IsOnPremDeployment = origOnPrem })

	withGrowthAnalyticsConfig(t, "http://localhost:18080/moesif-collector")
	fs, _ := withFakeSender(t)

	tracked := Track("amp.agent-development.create-agent", nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	tracked(httptest.NewRecorder(), authedRequest(http.MethodPost, "/agents"))

	waitForEvent(t, fs)
}

// TestTrack_ForwardsCallerJWT_NotAConfigToken verifies the event is
// authenticated to the collector proxy with the bearer token carried on the
// request being tracked — never a static credential from config, which no
// longer even has one.
func TestTrack_ForwardsCallerJWT_NotAConfigToken(t *testing.T) {
	withGrowthAnalyticsConfig(t, "http://localhost:18080/moesif-collector")
	fs, tokens := withFakeSender(t)

	tracked := Track("amp.agent-development.create-agent", nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})

	tracked(httptest.NewRecorder(), authedRequest(http.MethodPost, "/agents"))
	waitForEvent(t, fs)

	if len(*tokens) != 1 || (*tokens)[0] != callerToken {
		t.Errorf("newSender token = %v, want [%q] (the caller's own request token)", *tokens, callerToken)
	}
}

// TestTrack_NoCallerJWT_DropsEventWithoutSending verifies that a request
// reaching Track with no caller JWT on its context (every route Track wraps
// requires authentication, so this would be a bug upstream, not a normal
// case) never reaches the sender — sending unauthenticated to the proxy
// would just get a 401 — and, critically, never affects the response
// already written to the real caller.
func TestTrack_NoCallerJWT_DropsEventWithoutSending(t *testing.T) {
	withGrowthAnalyticsConfig(t, "http://localhost:18080/moesif-collector")

	orig := newSender
	newSender = func(config.GrowthAnalyticsConfig, string) eventSender {
		t.Fatal("newSender must not be called when the request has no caller JWT")
		return nil
	}
	t.Cleanup(func() { newSender = orig })

	handlerRan := false
	tracked := Track("amp.agent-development.create-agent", nil, func(w http.ResponseWriter, r *http.Request) {
		handlerRan = true
		w.WriteHeader(http.StatusCreated)
	})

	rec := httptest.NewRecorder()
	tracked(rec, httptest.NewRequest(http.MethodPost, "/agents", nil)) // no caller JWT attached

	if !handlerRan {
		t.Fatal("handler did not run")
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusCreated)
	}
}

func TestTrack_ReportsCorrectFeatureCodeAndDimensionsPerRoute(t *testing.T) {
	withGrowthAnalyticsConfig(t, "http://localhost:18080/moesif-collector")
	fs, _ := withFakeSender(t)

	var createRan, updateRan bool
	createHandler := func(w http.ResponseWriter, r *http.Request) { createRan = true; w.WriteHeader(http.StatusCreated) }
	updateHandler := func(w http.ResponseWriter, r *http.Request) { updateRan = true; w.WriteHeader(http.StatusOK) }

	trackedCreate := Track("amp.agent-development.create-agent",
		map[string]interface{}{"creation_method": "platform-hosted"}, createHandler)
	trackedUpdate := Track("amp.agent-development.update-agent",
		map[string]interface{}{"update_target": "basic-info"}, updateHandler)

	trackedCreate(httptest.NewRecorder(), authedRequest(http.MethodPost, "/agents"))
	if !createRan {
		t.Fatal("create handler did not run")
	}
	evt := waitForEvent(t, fs)
	if got := evt.Metadata["growth_action"]; got != "amp.agent-development.create-agent" {
		t.Errorf("growth_action = %v, want amp.agent-development.create-agent", got)
	}
	if got := evt.Metadata["creation_method"]; got != "platform-hosted" {
		t.Errorf("creation_method = %v, want platform-hosted", got)
	}
	if evt.Response.Status != http.StatusCreated {
		t.Errorf("response.status = %d, want %d", evt.Response.Status, http.StatusCreated)
	}
	if evt.Request.Verb != http.MethodPost {
		t.Errorf("request.verb = %q, want POST", evt.Request.Verb)
	}

	trackedUpdate(httptest.NewRecorder(), authedRequest(http.MethodPut, "/agents/x"))
	if !updateRan {
		t.Fatal("update handler did not run")
	}
	evt = waitForEvent(t, fs)
	if got := evt.Metadata["growth_action"]; got != "amp.agent-development.update-agent" {
		t.Errorf("growth_action = %v, want amp.agent-development.update-agent", got)
	}
	if got := evt.Metadata["update_target"]; got != "basic-info" {
		t.Errorf("update_target = %v, want basic-info", got)
	}
}

func TestTrack_SetDimension_ReportsValueDiscoveredInsideTheHandler(t *testing.T) {
	withGrowthAnalyticsConfig(t, "http://localhost:18080/moesif-collector")
	fs, _ := withFakeSender(t)

	// Simulates create-agent: the route has no static creation_method dimension
	// (nil), because it isn't known until the controller parses the request
	// body — the controller calls SetDimension itself once it knows.
	handler := func(w http.ResponseWriter, r *http.Request) {
		SetDimension(r.Context(), "creation_method", "external")
		w.WriteHeader(http.StatusAccepted)
	}
	tracked := Track("amp.agent-development.create-agent", nil, handler)

	tracked(httptest.NewRecorder(), authedRequest(http.MethodPost, "/agents"))

	evt := waitForEvent(t, fs)
	if got := evt.Metadata["creation_method"]; got != "external" {
		t.Errorf("creation_method = %v, want external", got)
	}
	if got := evt.Metadata["growth_action"]; got != "amp.agent-development.create-agent" {
		t.Errorf("growth_action = %v, want amp.agent-development.create-agent", got)
	}
}

func TestTrack_SetDimension_OverridesAStaticDimensionOfTheSameName(t *testing.T) {
	withGrowthAnalyticsConfig(t, "http://localhost:18080/moesif-collector")
	fs, _ := withFakeSender(t)

	handler := func(w http.ResponseWriter, r *http.Request) {
		SetDimension(r.Context(), "creation_method", "from-kind")
	}
	// A wrong static value, same as the bug being fixed — SetDimension must win.
	tracked := Track("amp.agent-development.create-agent",
		map[string]interface{}{"creation_method": "platform-hosted"}, handler)

	tracked(httptest.NewRecorder(), authedRequest(http.MethodPost, "/agents"))

	evt := waitForEvent(t, fs)
	if got := evt.Metadata["creation_method"]; got != "from-kind" {
		t.Errorf("creation_method = %v, want from-kind (SetDimension should override the static value)", got)
	}
}

func TestSetDimension_NoOpOutsideATrackedRequest(t *testing.T) {
	// A unit test calling a controller directly (no Track wrapper, no
	// request context set up by it) must not panic.
	defer func() {
		if rec := recover(); rec != nil {
			t.Fatalf("SetDimension panicked outside a tracked request: %v", rec)
		}
	}()
	SetDimension(context.Background(), "creation_method", "external")
}

func TestTrack_DynamicOutcome_ReflectsRealResponseStatus(t *testing.T) {
	tests := []struct {
		name        string
		statusCode  int
		wantOutcome string
	}{
		{"2xx is success", http.StatusCreated, "success"},
		{"4xx is failure", http.StatusBadRequest, "failure"},
		{"5xx is failure", http.StatusInternalServerError, "failure"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			withGrowthAnalyticsConfig(t, "http://localhost:18080/moesif-collector")
			fs, _ := withFakeSender(t)

			handler := func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(tt.statusCode) }
			tracked := Track("amp.agent-development.build-agent",
				map[string]interface{}{"outcome": DynamicOutcome}, handler)

			tracked(httptest.NewRecorder(), authedRequest(http.MethodPost, "/agents/x/builds"))

			evt := waitForEvent(t, fs)
			if got := evt.Metadata["outcome"]; got != tt.wantOutcome {
				t.Errorf("outcome = %v, want %v", got, tt.wantOutcome)
			}
			if evt.Response.Status != tt.statusCode {
				t.Errorf("response.status = %d, want %d", evt.Response.Status, tt.statusCode)
			}
		})
	}
}

// TestTrack_HandlerPanicPropagatesNormally verifies that Track no longer
// (unlike the old SDK-wrapped implementation) recovers a panic raised by
// the wrapped business handler itself — that's the router's concern, not
// growthanalytics's. Only the tracking code that runs after the handler
// returns is guarded by a recover().
func TestTrack_HandlerPanicPropagatesNormally(t *testing.T) {
	withGrowthAnalyticsConfig(t, "http://localhost:18080/moesif-collector")
	withFakeSender(t)

	tracked := Track("amp.agent-development.create-agent", nil, func(w http.ResponseWriter, r *http.Request) {
		panic("business handler failure")
	})

	defer func() {
		if rec := recover(); rec == nil {
			t.Fatal("expected the handler's panic to propagate out of Track, not be swallowed")
		}
	}()
	tracked(httptest.NewRecorder(), authedRequest(http.MethodPost, "/agents"))
}

// panicSender simulates a broken collector client (e.g. a bug in
// moesifcollector.Client) to prove a panic there can never crash the
// process or affect the response already sent to the caller.
type panicSender struct{ started chan struct{} }

func (p panicSender) SendEvent(context.Context, moesifcollector.Event) error {
	close(p.started)
	panic("simulated collector client failure")
}

func TestTrack_PanicSendingEventDoesNotCrashTheProcessOrAffectTheResponse(t *testing.T) {
	withGrowthAnalyticsConfig(t, "http://localhost:18080/moesif-collector")

	ps := panicSender{started: make(chan struct{})}
	orig := newSender
	newSender = func(config.GrowthAnalyticsConfig, string) eventSender { return ps }
	t.Cleanup(func() { newSender = orig })

	handlerRan := false
	tracked := Track("amp.agent-development.create-agent", nil, func(w http.ResponseWriter, r *http.Request) {
		handlerRan = true
		w.WriteHeader(http.StatusCreated)
	})

	rec := httptest.NewRecorder()
	tracked(rec, authedRequest(http.MethodPost, "/agents"))

	if !handlerRan {
		t.Fatal("handler did not run")
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("status = %d, want %d (must be unaffected by a failure sending the event)", rec.Code, http.StatusCreated)
	}

	select {
	case <-ps.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the panicking sender to be invoked")
	}
	// Give the detached goroutine's recover() a moment to run. If it
	// doesn't, the panic crashes the whole test binary — there's nothing
	// else to assert on for that case.
	time.Sleep(50 * time.Millisecond)
}

func TestBuildMetadata(t *testing.T) {
	tests := []struct {
		name        string
		feature     string
		dimensions  map[string]interface{}
		version     string
		deployment  string
		environment string
		want        map[string]interface{}
	}{
		{
			name:        "no dimensions",
			feature:     "amp.agent-development.delete-agent",
			version:     "1.2.3",
			deployment:  "saas",
			environment: "development",
			want: map[string]interface{}{
				"platform":         "Agent Manager",
				"source":           SourceAPI,
				"growth_action":    "amp.agent-development.delete-agent",
				"product_version":  "1.2.3",
				"deployment_model": "saas",
				"environment":      "development",
			},
		},
		{
			name:        "dimensions are merged in",
			feature:     "amp.agent-development.update-agent",
			dimensions:  map[string]interface{}{"update_target": "configurations"},
			version:     "1.2.3",
			deployment:  "saas",
			environment: "development",
			want: map[string]interface{}{
				"platform":         "Agent Manager",
				"source":           SourceAPI,
				"growth_action":    "amp.agent-development.update-agent",
				"product_version":  "1.2.3",
				"deployment_model": "saas",
				"environment":      "development",
				"update_target":    "configurations",
			},
		},
		{
			name:        "deployment model is reported from config, not hardcoded",
			feature:     "amp.agent-development.create-agent",
			version:     "1.2.3",
			deployment:  "vm",
			environment: "development",
			want: map[string]interface{}{
				"platform":         "Agent Manager",
				"source":           SourceAPI,
				"growth_action":    "amp.agent-development.create-agent",
				"product_version":  "1.2.3",
				"deployment_model": "vm",
				"environment":      "development",
			},
		},
		{
			// A blank environment must omit the field rather than report
			// environment:"" — an empty bucket in Moesif is worse than no
			// field at all, since it looks like a real environment.
			name:        "empty environment omits the field entirely",
			feature:     "amp.agent-development.create-agent",
			version:     "1.2.3",
			deployment:  "saas",
			environment: "",
			want: map[string]interface{}{
				"platform":         "Agent Manager",
				"source":           SourceAPI,
				"growth_action":    "amp.agent-development.create-agent",
				"product_version":  "1.2.3",
				"deployment_model": "saas",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildMetadata(tt.feature, tt.dimensions, tt.version, tt.deployment, tt.environment, SourceAPI)
			if len(got) != len(tt.want) {
				t.Fatalf("buildMetadata() = %v, want %v", got, tt.want)
			}
			for k, wantV := range tt.want {
				if got[k] != wantV {
					t.Errorf("buildMetadata()[%q] = %v, want %v", k, got[k], wantV)
				}
			}
		})
	}
}

func TestResolveDimensions(t *testing.T) {
	t.Run("nil dimensions stay nil", func(t *testing.T) {
		if got := resolveDimensions(nil, &statusHolder{code: http.StatusOK}); got != nil {
			t.Errorf("resolveDimensions(nil, ...) = %v, want nil", got)
		}
	})

	t.Run("does not mutate the input map", func(t *testing.T) {
		input := map[string]interface{}{"outcome": DynamicOutcome}
		_ = resolveDimensions(input, &statusHolder{code: http.StatusInternalServerError})
		if input["outcome"] != DynamicOutcome {
			t.Error("resolveDimensions mutated its input map")
		}
	})

	t.Run("non-outcome dimensions pass through unchanged", func(t *testing.T) {
		got := resolveDimensions(map[string]interface{}{"action": "create"}, nil)
		if got["action"] != "create" {
			t.Errorf(`got["action"] = %v, want "create"`, got["action"])
		}
	})

	t.Run("nil status holder defaults to success", func(t *testing.T) {
		got := resolveDimensions(map[string]interface{}{"outcome": DynamicOutcome}, nil)
		if got["outcome"] != "success" {
			t.Errorf(`got["outcome"] = %v, want "success"`, got["outcome"])
		}
	})
}

func TestOutcomeFromStatus(t *testing.T) {
	tests := []struct {
		status int
		want   string
	}{
		{http.StatusOK, "success"},
		{http.StatusCreated, "success"},
		{http.StatusMultipleChoices, "failure"},
		{http.StatusBadRequest, "failure"},
		{http.StatusInternalServerError, "failure"},
	}
	for _, tt := range tests {
		if got := outcomeFromStatus(tt.status); got != tt.want {
			t.Errorf("outcomeFromStatus(%d) = %q, want %q", tt.status, got, tt.want)
		}
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name       string
		remoteAddr string
		forwardFor string
		want       string
	}{
		{"no forwarded header uses remote addr host", "203.0.113.9:44321", "", "203.0.113.9"},
		{"forwarded-for wins over remote addr", "203.0.113.9:44321", "198.51.100.4", "198.51.100.4"},
		{"first entry of a comma-separated forwarded-for", "203.0.113.9:44321", "198.51.100.4, 10.0.0.1", "198.51.100.4"},
		{"malformed remote addr falls back to raw value", "not-a-host-port", "", "not-a-host-port"},
		{"non-IP forwarded-for falls back to the peer", "203.0.113.9:44321", "evil<script>", "203.0.113.9"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.RemoteAddr = tt.remoteAddr
			if tt.forwardFor != "" {
				r.Header.Set("X-Forwarded-For", tt.forwardFor)
			}
			if got := ClientIP(r); got != tt.want {
				t.Errorf("ClientIP() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestTrack_DropsEventWhenSendPoolFull: a slow collector must cost a fixed
// number of goroutines. With every slot taken, the event is dropped before a
// sender is built and the caller's response is untouched.
func TestTrack_DropsEventWhenSendPoolFull(t *testing.T) {
	withGrowthAnalyticsConfig(t, "http://localhost:18080/moesif-collector")

	orig, origSlots := newSender, eventSendSlots
	full := make(chan struct{}, 1)
	full <- struct{}{}
	eventSendSlots = full
	newSender = func(config.GrowthAnalyticsConfig, string) eventSender {
		t.Fatal("newSender must not be called when the send pool is full")
		return nil
	}
	t.Cleanup(func() { newSender, eventSendSlots = orig, origSlots })

	tracked := Track("amp.agent-development.create-agent", nil, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
	})
	w := httptest.NewRecorder()
	tracked(w, authedRequest(http.MethodPost, "/agents"))

	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestRequestURI_StripsQueryAndIgnoresBogusProto(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "http://api.example.com/orgs/o1/agents?search=secret#frag", nil)
	r.Header.Set("X-Forwarded-Proto", "javascript")

	if got, want := requestURI(r), "http://api.example.com/orgs/o1/agents"; got != want {
		t.Errorf("requestURI() = %q, want %q", got, want)
	}

	r.Header.Set("X-Forwarded-Proto", "https")
	if got, want := requestURI(r), "https://api.example.com/orgs/o1/agents"; got != want {
		t.Errorf("requestURI() = %q, want %q", got, want)
	}
}
