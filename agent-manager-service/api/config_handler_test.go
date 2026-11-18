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
)

func setupConfigMux() *http.ServeMux {
	mux := http.NewServeMux()
	registerConfigRoutes(mux)
	return mux
}

func TestConfigEndpoint_HappyPath(t *testing.T) {
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

	var raw map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, ok := raw["eventsEnabled"].(bool); !ok {
		t.Errorf("expected a boolean eventsEnabled in %v", raw)
	}
	if _, ok := raw["observerBaseUrl"]; ok {
		t.Errorf("observerBaseUrl is no longer served, got %v", raw)
	}
}

func TestConfigEndpoint_MethodNotAllowed(t *testing.T) {
	mux := setupConfigMux()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", rec.Code)
	}
}

func TestConfigEndpoint_NoAuthRequired(t *testing.T) {
	mux := setupConfigMux()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 without Authorization header, got %d", rec.Code)
	}
}

func TestConfigEndpoint_CORSHeadersPresentForCrossOriginRequest(t *testing.T) {
	origin := config.GetConfig().CORSAllowedOrigin
	mux := setupConfigMux()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/config", nil)
	req.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, origin)
	}
}

func TestConfigEndpoint_PreflightOptionsSucceeds(t *testing.T) {
	origin := config.GetConfig().CORSAllowedOrigin
	mux := setupConfigMux()
	req := httptest.NewRequest(http.MethodOptions, "/api/v1/config", nil)
	req.Header.Set("Origin", origin)
	req.Header.Set("Access-Control-Request-Method", http.MethodGet)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code < 200 || rec.Code >= 300 {
		t.Fatalf("expected 2xx for OPTIONS preflight, got %d", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
		t.Errorf("Access-Control-Allow-Origin = %q, want %q", got, origin)
	}
}
