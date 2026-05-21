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

package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func setupConfigMux() *http.ServeMux {
	mux := http.NewServeMux()
	registerConfigRoutes(mux)
	return mux
}

func withTraceObserverURL(t *testing.T, url string) {
	t.Helper()
	cfg := config.GetConfig()
	orig := cfg.TraceObserver.URL
	t.Cleanup(func() {
		cfg.TraceObserver.URL = orig
	})
	cfg.TraceObserver.URL = url
}

func TestConfigEndpoint_HappyPath(t *testing.T) {
	withTraceObserverURL(t, "https://observer.example.com")

	mux := setupConfigMux()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var body spec.ConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TraceObserverBaseUrl != "https://observer.example.com" {
		t.Errorf("traceObserverBaseUrl = %q, want %q", body.TraceObserverBaseUrl, "https://observer.example.com")
	}
}

func TestConfigEndpoint_EmptyURLStillReturns200(t *testing.T) {
	withTraceObserverURL(t, "")

	mux := setupConfigMux()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body spec.ConfigResponse
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.TraceObserverBaseUrl != "" {
		t.Errorf("traceObserverBaseUrl = %q, want empty", body.TraceObserverBaseUrl)
	}
}

func TestConfigEndpoint_FieldNameIsCamelCase(t *testing.T) {
	withTraceObserverURL(t, "https://observer.example.com")

	mux := setupConfigMux()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	var raw map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := raw["traceObserverBaseUrl"]; !ok {
		t.Errorf("expected camelCase field traceObserverBaseUrl, got %v", raw)
	}
}

func TestConfigEndpoint_MethodNotAllowed(t *testing.T) {
	withTraceObserverURL(t, "https://observer.example.com")

	mux := setupConfigMux()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", rec.Code)
	}
}

func TestConfigEndpoint_NoAuthRequired(t *testing.T) {
	withTraceObserverURL(t, "https://observer.example.com")

	mux := setupConfigMux()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 without Authorization header, got %d", rec.Code)
	}
}
