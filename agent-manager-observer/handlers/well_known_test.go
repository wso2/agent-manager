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

package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
)

func setupWellKnownMux(cfg config.AuthConfig) *http.ServeMux {
	mux := http.NewServeMux()
	RegisterWellKnownRoutes(mux, cfg)
	return mux
}

func TestWellKnownOAuthProtectedResource_HappyPath(t *testing.T) {
	cfg := config.AuthConfig{
		ServerPublicURL:      "https://traces.amp.example.com",
		AuthorizationServers: []string{"https://thunder.example.com"},
		ScopesSupported:      []string{"amp:observability:log-read", "amp:observability:trace-read"},
	}

	mux := setupWellKnownMux(cfg)
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
	if body.Resource != ampAPIResourceIdentifier {
		t.Errorf("expected REST API resource %q, got %q", ampAPIResourceIdentifier, body.Resource)
	}
	if len(body.AuthorizationServers) != 1 || body.AuthorizationServers[0] != "https://thunder.example.com" {
		t.Errorf("expected authorization_servers [https://thunder.example.com], got %v", body.AuthorizationServers)
	}
	if len(body.BearerMethodsSupported) != 1 || body.BearerMethodsSupported[0] != "header" {
		t.Errorf("expected bearer_methods_supported [header], got %v", body.BearerMethodsSupported)
	}
	if !reflect.DeepEqual(body.ScopesSupported, cfg.ScopesSupported) {
		t.Errorf("expected scopes %v, got %v", cfg.ScopesSupported, body.ScopesSupported)
	}
}

func TestWellKnownOAuthProtectedResource_MCPPath(t *testing.T) {
	cfg := config.AuthConfig{
		ServerPublicURL:      "https://traces.amp.example.com/",
		AuthorizationServers: []string{"https://thunder.example.com"},
		ScopesSupported:      []string{"amp:observability:trace-read"},
	}

	mux := setupWellKnownMux(cfg)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var body protectedResourceMetadata
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body.Resource != "https://traces.amp.example.com/mcp" {
		t.Fatalf("expected normalized MCP resource, got %q", body.Resource)
	}
	if body.Resource == ampAPIResourceIdentifier {
		t.Fatal("expected MCP metadata to remain distinct from REST API metadata")
	}
}

func TestWellKnownOAuthProtectedResource_MissingPublicURL(t *testing.T) {
	cfg := config.AuthConfig{
		AuthorizationServers: []string{"https://thunder.example.com"},
	}

	mux := setupWellKnownMux(cfg)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when SERVER_PUBLIC_URL is empty, got %d", rec.Code)
	}
}

func TestWellKnownOAuthProtectedResource_MissingAuthorizationServers(t *testing.T) {
	cfg := config.AuthConfig{
		ServerPublicURL: "https://traces.amp.example.com",
	}

	mux := setupWellKnownMux(cfg)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when OAUTH_AUTHORIZATION_SERVERS is empty, got %d", rec.Code)
	}
}

func TestWellKnownOAuthProtectedResource_MethodNotAllowed(t *testing.T) {
	cfg := config.AuthConfig{
		ServerPublicURL:      "https://traces.amp.example.com",
		AuthorizationServers: []string{"https://thunder.example.com"},
	}

	mux := setupWellKnownMux(cfg)
	req := httptest.NewRequest(http.MethodPost, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", rec.Code)
	}
}

func TestWellKnownOAuthProtectedResource_NoAuthRequired(t *testing.T) {
	cfg := config.AuthConfig{
		ServerPublicURL:      "https://traces.amp.example.com",
		AuthorizationServers: []string{"https://thunder.example.com"},
	}

	mux := setupWellKnownMux(cfg)
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 without Authorization header, got %d", rec.Code)
	}
}
