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

func setupWellKnownMux() *http.ServeMux {
	mux := http.NewServeMux()
	registerWellKnownRoutes(mux)
	return mux
}

func withWellKnownConfig(t *testing.T, publicURL string, authServers []string) {
	t.Helper()
	cfg := config.GetConfig()
	origURL := cfg.ServerPublicURL
	origServers := cfg.OAuthAuthorizationServers
	t.Cleanup(func() {
		cfg.ServerPublicURL = origURL
		cfg.OAuthAuthorizationServers = origServers
	})
	cfg.ServerPublicURL = publicURL
	cfg.OAuthAuthorizationServers = authServers
}

func TestWellKnownOAuthProtectedResource_HappyPath(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com", []string{"https://idp.example.com"})

	mux := setupWellKnownMux()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}
	if cc := rec.Header().Get("Cache-Control"); cc != "public, max-age=3600" {
		t.Errorf("expected Cache-Control public, max-age=3600, got %q", cc)
	}

	var body protectedResourceMetadata
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Resource != "https://am.example.com" {
		t.Errorf("expected resource %q, got %q", "https://am.example.com", body.Resource)
	}
	if len(body.AuthorizationServers) != 1 || body.AuthorizationServers[0] != "https://idp.example.com" {
		t.Errorf("expected authorization_servers [https://idp.example.com], got %v", body.AuthorizationServers)
	}
	if len(body.BearerMethodsSupported) != 1 || body.BearerMethodsSupported[0] != "header" {
		t.Errorf("expected bearer_methods_supported [header], got %v", body.BearerMethodsSupported)
	}
}

func TestWellKnownOAuthProtectedResource_MultipleAuthorizationServers(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com", []string{"https://idp1.example.com", "https://idp2.example.com"})

	mux := setupWellKnownMux()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body protectedResourceMetadata
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(body.AuthorizationServers) != 2 {
		t.Fatalf("expected 2 authorization_servers, got %d", len(body.AuthorizationServers))
	}
	if body.AuthorizationServers[0] != "https://idp1.example.com" || body.AuthorizationServers[1] != "https://idp2.example.com" {
		t.Errorf("expected [https://idp1.example.com, https://idp2.example.com], got %v", body.AuthorizationServers)
	}
}

func TestWellKnownOAuthProtectedResource_TrailingSlashPreserved(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com/", []string{"https://idp.example.com/"})

	mux := setupWellKnownMux()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var body protectedResourceMetadata
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Resource != "https://am.example.com/" {
		t.Errorf("expected resource to preserve trailing slash, got %q", body.Resource)
	}
	if len(body.AuthorizationServers) != 1 || body.AuthorizationServers[0] != "https://idp.example.com/" {
		t.Errorf("expected authorization_servers to preserve trailing slash, got %v", body.AuthorizationServers)
	}
}

func TestWellKnownOAuthProtectedResource_MissingPublicURL(t *testing.T) {
	withWellKnownConfig(t, "", []string{"https://idp.example.com"})

	mux := setupWellKnownMux()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when SERVER_PUBLIC_URL is empty, got %d", rec.Code)
	}
}

func TestWellKnownOAuthProtectedResource_MissingAuthorizationServers(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com", nil)

	mux := setupWellKnownMux()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when OAUTH_AUTHORIZATION_SERVERS is empty, got %d", rec.Code)
	}
}

func TestWellKnownOAuthProtectedResource_MethodNotAllowed(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com", []string{"https://idp.example.com"})

	mux := setupWellKnownMux()
	req := httptest.NewRequest(http.MethodPost, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", rec.Code)
	}
}

func TestWellKnownOAuthProtectedResource_NoAuthRequired(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com", []string{"https://idp.example.com"})

	mux := setupWellKnownMux()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 without Authorization header, got %d", rec.Code)
	}
}
