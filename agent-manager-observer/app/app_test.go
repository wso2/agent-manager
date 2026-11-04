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
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/middleware"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
)

type requestTokenProvider struct{}

func (requestTokenProvider) GetToken(ctx context.Context) (string, error) {
	if token := middleware.BearerTokenFromContext(ctx); token != "" {
		return token, nil
	}
	return "", errors.New("no bearer token in request context")
}

func (requestTokenProvider) InvalidateToken() {}

func newTestHandler(t *testing.T, upstreamURL string) http.Handler {
	t.Helper()
	cfg := &config.Config{
		Observer: config.ObserverConfig{BaseURL: upstreamURL, DefaultNamespace: "default"},
		Auth:     config.AuthConfig{IsLocalDevEnv: true},
	}
	return newHandler(cfg, observer.NewClient(upstreamURL, requestTokenProvider{}, cfg.Observer.DefaultNamespace))
}

func scopedToken(t *testing.T) string {
	t.Helper()
	claims := jwt.MapClaims{
		"sub":   "test-user",
		"scope": rbac.TraceRead.Scope(),
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

func TestNewHandler_ForwardsInjectedProviderToken(t *testing.T) {
	var gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"spans": []}`))
	}))
	defer upstream.Close()

	token := scopedToken(t)
	r := httptest.NewRequest(http.MethodGet,
		"/api/v1/traces/abc/spans?organization=default&startTime=2026-01-01T00:00:00Z&endTime=2026-01-02T00:00:00Z", nil)
	r.Header.Set("Authorization", "Bearer "+token)
	rec := httptest.NewRecorder()
	newTestHandler(t, upstream.URL).ServeHTTP(rec, r)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rec.Code, rec.Body.String())
	}
	if gotAuth != "Bearer "+token {
		t.Errorf("expected upstream Authorization to carry the caller token, got %q", gotAuth)
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
		Server:   config.ServerConfig{Port: ln.Addr().(*net.TCPAddr).Port},
		Observer: config.ObserverConfig{BaseURL: "http://unused"},
		Auth:     config.AuthConfig{IsLocalDevEnv: true},
	}
	done := make(chan error, 1)
	go func() { done <- Run(cfg, requestTokenProvider{}, Options{}) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error for a port already in use, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after failing to bind")
	}
}

func TestRun_ReturnsIngestListenError(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = held.Close() }()

	cfg := &config.Config{
		Server:   config.ServerConfig{Port: freePort(t)},
		Observer: config.ObserverConfig{BaseURL: "http://unused"},
		Auth:     config.AuthConfig{IsLocalDevEnv: true},
	}
	opts := Options{Ingest: http.NotFoundHandler(), IngestAddr: held.Addr().String()}
	done := make(chan error, 1)
	go func() { done <- Run(cfg, requestTokenProvider{}, opts) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected error for an ingest port already in use, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after failing to bind the ingest listener")
	}
}

func TestRun_CallsCloseOnShutdown(t *testing.T) {
	cfg := &config.Config{
		Server:   config.ServerConfig{Port: freePort(t)},
		Observer: config.ObserverConfig{BaseURL: "http://unused"},
		Auth:     config.AuthConfig{IsLocalDevEnv: true},
	}
	var closed atomic.Int32
	opts := Options{Close: func() error { closed.Add(1); return nil }}
	done := make(chan error, 1)
	go func() { done <- Run(cfg, requestTokenProvider{}, opts) }()

	waitForHealth(t, cfg.Server.Port)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGTERM); err != nil {
		t.Fatalf("signal: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after SIGTERM")
	}
	if got := closed.Load(); got != 1 {
		t.Fatalf("expected Close called once, got %d", got)
	}
}

func TestRun_CallsCloseWhenServerFails(t *testing.T) {
	held, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = held.Close() }()

	cfg := &config.Config{
		Server:   config.ServerConfig{Port: freePort(t)},
		Observer: config.ObserverConfig{BaseURL: "http://unused"},
		Auth:     config.AuthConfig{IsLocalDevEnv: true},
	}
	var closed atomic.Int32
	opts := Options{
		Ingest:     http.NotFoundHandler(),
		IngestAddr: held.Addr().String(),
		Close:      func() error { closed.Add(1); return nil },
	}
	done := make(chan error, 1)
	go func() { done <- Run(cfg, requestTokenProvider{}, opts) }()

	select {
	case err := <-done:
		if err == nil {
			t.Fatal("expected a server failure, got nil")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after a server failed")
	}
	if got := closed.Load(); got != 1 {
		t.Fatalf("expected Close called once, got %d", got)
	}
	apiAddr := fmt.Sprintf("127.0.0.1:%d", cfg.Server.Port)
	if conn, err := net.DialTimeout("tcp", apiAddr, time.Second); err == nil {
		_ = conn.Close()
		t.Fatal("API server still accepts connections after Run returned")
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = ln.Close() }()
	return ln.Addr().(*net.TCPAddr).Port
}

func waitForHealth(t *testing.T, port int) {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:%d/health", port)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(url)
		if err == nil {
			_ = resp.Body.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("server did not become healthy")
}
