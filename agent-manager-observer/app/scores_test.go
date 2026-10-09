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

package app

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
)

// traceListQuery lists one agent's traces over a day.
const traceListQuery = "/api/v1/traces?organization=default&project=p&agent=a&environment=dev" +
	"&startTime=2026-01-01T00:00:00Z&endTime=2026-01-02T00:00:00Z"

// upstreamWithOneTrace stands in for the upstream observer with one trace, t1.
func upstreamWithOneTrace(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1alpha1/traces/query" {
			_, _ = w.Write([]byte(`{"traces":[{"traceId":"t1","rootSpanId":"r1","startTime":"2026-01-01T01:00:00Z","endTime":"2026-01-01T01:00:01Z"}],"total":1}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// getTraces sends path with a trace-read token through the full handler.
func getTraces(t *testing.T, h http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, path, nil)
	r.Header.Set("Authorization", "Bearer "+scopedToken(t))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

// Without AGENT_MANAGER_SERVICE_URL a score filter is a 503 and other requests are served.
func TestNewHandler_ScoreFilterWithoutServiceURL(t *testing.T) {
	h := newTestHandler(t, upstreamWithOneTrace(t).URL)

	if rec := getTraces(t, h, traceListQuery+"&maxScore=0.5"); rec.Code != http.StatusServiceUnavailable {
		t.Errorf("maxScore: expected 503, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if rec := getTraces(t, h, traceListQuery); rec.Code != http.StatusOK {
		t.Errorf("no filter: expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

// With the URL set, a score filter asks the service with the caller's token,
// and other requests never call it.
func TestNewHandler_ScoreFilterCallsService(t *testing.T) {
	var calls atomic.Int32
	var gotAuth atomic.Value
	service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		gotAuth.Store(r.Header.Get("Authorization"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"traces":[{"traceId":"t1","score":0.4,"totalCount":1,"skippedCount":0}],"totalCount":1}`))
	}))
	t.Cleanup(service.Close)
	cfg := &config.Config{
		Observer:     config.ObserverConfig{BaseURL: upstreamWithOneTrace(t).URL, DefaultNamespace: "default"},
		Auth:         config.AuthConfig{IsLocalDevEnv: true},
		AgentManager: config.AgentManagerConfig{BaseURL: service.URL},
	}
	h := newHandler(cfg, requestTokenProvider{})

	for _, path := range []string{traceListQuery, traceListQuery + "&status=error", traceListQuery + "&minDurationMs=0"} {
		if rec := getTraces(t, h, path); rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, rec.Code)
		}
	}
	if calls.Load() != 0 {
		t.Fatalf("service called %d times without a score filter, want 0", calls.Load())
	}

	rec := getTraces(t, h, traceListQuery+"&maxScore=0.5")

	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"t1"`) {
		t.Fatalf("expected 200 with t1, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if calls.Load() != 1 {
		t.Errorf("service called %d times, want 1", calls.Load())
	}
	if auth, _ := gotAuth.Load().(string); !strings.HasPrefix(auth, "Bearer ") || auth == "Bearer " {
		t.Errorf("service Authorization = %q, want the caller's token", auth)
	}
}
