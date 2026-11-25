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
	"reflect"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func setupWellKnownMux() *http.ServeMux {
	mux := http.NewServeMux()
	registerWellKnownRoutes(mux, nil)
	return mux
}

func withWellKnownConfig(t *testing.T, publicURL string, authServers []string, scopes []string) {
	t.Helper()
	cfg := config.GetConfig()
	origURL := cfg.ServerPublicURL
	origServers := cfg.OAuthAuthorizationServers
	origScopes := cfg.OAuthScopesSupported
	t.Cleanup(func() {
		cfg.ServerPublicURL = origURL
		cfg.OAuthAuthorizationServers = origServers
		cfg.OAuthScopesSupported = origScopes
	})
	cfg.ServerPublicURL = publicURL
	cfg.OAuthAuthorizationServers = authServers
	cfg.OAuthScopesSupported = scopes
}

func TestWellKnownOAuthProtectedResource_HappyPath(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com", []string{"https://idp.example.com"}, []string{"org:view", "project:read"})

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
	if expected := []string{"org:view", "project:read"}; !reflect.DeepEqual(body.ScopesSupported, expected) {
		t.Errorf("expected API/CLI scopes %v, got %v", expected, body.ScopesSupported)
	}
}

func TestWellKnownOAuthProtectedResource_MCPMetadata(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com/", []string{"https://idp.example.com"}, []string{"api:scope"})

	mux := setupWellKnownMux()
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
	if body.Resource != "https://am.example.com/mcp" {
		t.Errorf("expected MCP resource %q, got %q", "https://am.example.com/mcp", body.Resource)
	}
	if expected := rbac.MainMCPScopes(); !reflect.DeepEqual(body.ScopesSupported, expected) {
		t.Errorf("expected MCP scopes %v, got %v", expected, body.ScopesSupported)
	}
}

func TestWellKnownOAuthProtectedResource_MultipleAuthorizationServers(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com", []string{"https://idp1.example.com", "https://idp2.example.com"}, []string{"org:view", "project:read"})

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

func TestWellKnownOAuthProtectedResource_TrailingSlashNormalized(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com/", []string{"https://idp.example.com/"}, nil)

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
	// The API/CLI resource is the normalized public service URL. MCP metadata is
	// served separately at /.well-known/oauth-protected-resource/mcp.
	if body.Resource != "https://am.example.com" {
		t.Errorf("expected resource with trailing slash normalized away, got %q", body.Resource)
	}
	// authorization_servers is passed through verbatim — untouched by resource normalization.
	if len(body.AuthorizationServers) != 1 || body.AuthorizationServers[0] != "https://idp.example.com/" {
		t.Errorf("expected authorization_servers to preserve trailing slash, got %v", body.AuthorizationServers)
	}
}

func TestWellKnownOAuthProtectedResource_MissingPublicURL(t *testing.T) {
	withWellKnownConfig(t, "", []string{"https://idp.example.com"}, nil)

	mux := setupWellKnownMux()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when SERVER_PUBLIC_URL is empty, got %d", rec.Code)
	}
}

func TestWellKnownOAuthProtectedResource_MissingAuthorizationServers(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com", nil, nil)

	mux := setupWellKnownMux()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when OAUTH_AUTHORIZATION_SERVERS is empty, got %d", rec.Code)
	}
}

func TestWellKnownOAuthProtectedResource_MethodNotAllowed(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com", []string{"https://idp.example.com"}, nil)

	mux := setupWellKnownMux()
	req := httptest.NewRequest(http.MethodPost, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405 for POST, got %d", rec.Code)
	}
}

func TestWellKnownOAuthProtectedResource_NoAuthRequired(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com", []string{"https://idp.example.com"}, nil)

	mux := setupWellKnownMux()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected 200 without Authorization header, got %d", rec.Code)
	}
}

// toolModules is a module set whose only surface is its MCP tools.
type toolModules struct {
	Modules
	tools []modhost.MCPTool
}

func (m toolModules) MCPTools() []modhost.MCPTool { return m.tools }

// The MCP metadata advertises the scopes of the active modules' tools after
// the core's, once each.
func TestWellKnownMCPMetadataAddsModuleToolScopes(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com", []string{"https://idp.example.com"}, nil)
	mux := http.NewServeMux()
	registerWellKnownRoutes(mux, toolModules{tools: []modhost.MCPTool{
		{Permission: "mod-x:things.read"}, {Permission: "mod-x:things.read"}, {Permission: rbac.AgentRead},
	}})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/oauth-protected-resource/mcp", nil))

	var body protectedResourceMetadata
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	want := append(rbac.MainMCPScopes(), rbac.Permission("mod-x:things.read").Scope())
	if !reflect.DeepEqual(body.ScopesSupported, want) {
		t.Errorf("expected MCP scopes %v, got %v", want, body.ScopesSupported)
	}
}

// With OAUTH_SCOPES_SUPPORTED unset, the API advertises every scope of the
// permission catalog, so a CLI that asks for them gets a token its role
// allows without a deployment file listing them.
func TestWellKnownOAuthProtectedResource_DefaultsToCatalogScopes(t *testing.T) {
	withWellKnownConfig(t, "https://am.example.com", []string{"https://idp.example.com"}, nil)

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
	var want []string
	for _, p := range rbac.Current().Permissions() {
		want = append(want, p.Scope())
	}
	if len(want) == 0 || !reflect.DeepEqual(body.ScopesSupported, want) {
		t.Errorf("expected the catalog's %d scopes, got %v", len(want), body.ScopesSupported)
	}
}
