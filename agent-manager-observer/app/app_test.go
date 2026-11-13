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
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
)

func newTestHandler(t *testing.T, openSearchURL string) http.Handler {
	t.Helper()
	cfg := &config.Config{
		OpenSearch: config.OpenSearchConfig{Address: openSearchURL, TraceIndex: "otel-traces-*"},
		Auth:       config.AuthConfig{IsLocalDevEnv: true},
	}
	return newHandler(cfg)
}

func scopedToken(t *testing.T, scope string) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":   "test-user",
		"scope": scope,
		"exp":   time.Now().Add(time.Hour).Unix(),
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("k"))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestNewHandler_Health(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler(t, "http://unused").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestNewHandler_RejectsMissingToken(t *testing.T) {
	rec := httptest.NewRecorder()
	newTestHandler(t, "http://unused").ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/traces/abc/spans", nil))

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestNewHandler_QueriesOpenSearch(t *testing.T) {
	var gotPath string
	store := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"hits":{"total":{"value":0},"hits":[]}}`))
	}))
	defer store.Close()

	r := httptest.NewRequest(http.MethodGet,
		"/api/v1/traces/abc/spans?organization=default&startTime=2026-01-01T00:00:00Z&endTime=2026-01-02T00:00:00Z", nil)
	r.Header.Set("Authorization", "Bearer "+scopedToken(t, rbac.TraceRead.Scope()))
	rec := httptest.NewRecorder()
	newTestHandler(t, store.URL).ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if gotPath != "/otel-traces-*/_search" {
		t.Errorf("expected a search on the trace index, got %q", gotPath)
	}
}

func TestNewHandler_LogsNotImplemented(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet,
		"/api/v1/logs?organization=o&project=p&agent=a&startTime=2026-01-01T00:00:00Z&endTime=2026-01-02T00:00:00Z", nil)
	r.Header.Set("Authorization", "Bearer "+scopedToken(t, rbac.LogRead.Scope()))
	rec := httptest.NewRecorder()
	newTestHandler(t, "http://unused").ServeHTTP(rec, r)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d (body: %s)", rec.Code, rec.Body.String())
	}
}

func TestIsSpanDetailPath(t *testing.T) {
	tests := map[string]bool{
		"/api/v1/traces/abc/spans":      false,
		"/api/v1/traces/abc/spans/":     false,
		"/api/v1/traces/abc/spans/span": true,
	}
	for path, want := range tests {
		if got := isSpanDetailPath(path); got != want {
			t.Errorf("isSpanDetailPath(%q) = %v, want %v", path, got, want)
		}
	}
}

func TestRun_ReturnsListenError(t *testing.T) {
	ln, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()

	cfg := &config.Config{
		Server:     config.ServerConfig{Port: ln.Addr().(*net.TCPAddr).Port},
		OpenSearch: config.OpenSearchConfig{Address: "http://unused", TraceIndex: "otel-traces-*"},
		Auth:       config.AuthConfig{IsLocalDevEnv: true},
	}
	done := make(chan error, 1)
	go func() { done <- Run(cfg, Options{}) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error for a port already in use, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after failing to bind")
	}
}
