//go:build integration

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

package gwctl_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

// agCreateToolProxy creates an MCP proxy whose endpoint discovered tools, with
// identity security when identity is set.
func agCreateToolProxy(t *testing.T, a *routeAPI, handle string, identity bool, tools ...string) {
	t.Helper()
	body := agMCPProxyBody(handle, a.defaultEnv().String())
	ep := body["endpoints"].([]any)[0].(map[string]any)
	discovered := make([]any, 0, len(tools))
	for _, tl := range tools {
		discovered = append(discovered, map[string]any{"name": tl})
	}
	ep["capabilities"] = map[string]any{"tools": discovered}
	if identity {
		ep["security"] = map[string]any{"enabled": true, "identity": map[string]any{"enabled": true}}
	}
	expect(t, a.do(http.MethodPost, "/orgs/{org}/mcp-proxies", body), http.StatusCreated)
}

// TestAGRoutesMCPProxyScopeLifecycle creates, lists, updates and deletes a
// proxy's scope, reading each change back.
func TestAGRoutesMCPProxyScopeLifecycle(t *testing.T) {
	a, _ := agSetup(t)
	agCreateToolProxy(t, a, "forecast", false, "get_forecast", "get_alerts")
	base := "/orgs/{org}/mcp-proxies/forecast/scopes"

	empty := expectInto[gwspec.MCPProxyScopeListResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	assert.Empty(t, empty.Scopes)

	created := expectInto[gwspec.MCPProxyScopeResponse](t, a.do(http.MethodPost, base, map[string]any{
		"action": "read", "description": "read the forecast", "tools": []string{"get_forecast"},
	}), http.StatusCreated)
	assert.Equal(t, "read", created.Action)
	assert.Equal(t, "forecast:read", created.Scope)
	assert.Equal(t, "read the forecast", *created.Description)
	assert.Equal(t, []string{"get_forecast"}, created.Tools)

	updated := expectInto[gwspec.MCPProxyScopeResponse](t, a.do(http.MethodPut, base+"/read", map[string]any{
		"description": "read everything", "tools": []string{"get_forecast", "get_alerts"},
	}), http.StatusOK)
	assert.Equal(t, "read everything", *updated.Description)
	assert.Equal(t, []string{"get_alerts", "get_forecast"}, updated.Tools, "tools are stored sorted")

	list := expectInto[gwspec.MCPProxyScopeListResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.Len(t, list.Scopes, 1)
	assert.Equal(t, "forecast:read", list.Scopes[0].Scope)
	assert.Equal(t, "read everything", *list.Scopes[0].Description)

	rec := a.do(http.MethodDelete, base+"/read", nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	list = expectInto[gwspec.MCPProxyScopeListResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	assert.Empty(t, list.Scopes)
}

// TestAGRoutesMCPProxyScopeRejections pins the 400, 404, 409 and 403 answers of
// the scope routes.
func TestAGRoutesMCPProxyScopeRejections(t *testing.T) {
	a, _ := agSetup(t)
	agCreateToolProxy(t, a, "forecast", false, "get_forecast")
	base := "/orgs/{org}/mcp-proxies/forecast/scopes"
	expect(t, a.do(http.MethodPost, base, map[string]any{"action": "read"}), http.StatusCreated)

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		status  int
		message string
	}{
		{"create malformed json", http.MethodPost, base, "{", http.StatusBadRequest, "Invalid request body"},
		{"create bad action", http.MethodPost, base, map[string]any{"action": "no spaces"}, http.StatusBadRequest, "action must match"},
		{"create undiscovered tool", http.MethodPost, base, map[string]any{"action": "write", "tools": []string{"drop_db"}}, http.StatusBadRequest, `tool "drop_db" is not among`},
		{"create duplicate tool", http.MethodPost, base, map[string]any{"action": "write", "tools": []string{"get_forecast", "get_forecast"}}, http.StatusBadRequest, "duplicate tool"},
		{"create existing action", http.MethodPost, base, map[string]any{"action": "read"}, http.StatusConflict, `scope action "read" already exists`},
		{"create on unknown proxy", http.MethodPost, "/orgs/{org}/mcp-proxies/ghost/scopes", map[string]any{"action": "read"}, http.StatusNotFound, "MCP proxy not found"},
		{"list unknown proxy", http.MethodGet, "/orgs/{org}/mcp-proxies/ghost/scopes", nil, http.StatusNotFound, "MCP proxy not found"},
		{"update malformed json", http.MethodPut, base + "/read", "{", http.StatusBadRequest, "Invalid request body"},
		{"update undiscovered tool", http.MethodPut, base + "/read", map[string]any{"tools": []string{"drop_db"}}, http.StatusBadRequest, "not among the proxy's discovered tools"},
		{"update unknown action", http.MethodPut, base + "/write", map[string]any{"description": "d"}, http.StatusNotFound, "MCP proxy or scope not found"},
		{"delete unknown action", http.MethodDelete, base + "/write", nil, http.StatusNotFound, "MCP proxy or scope not found"},
		{"delete on unknown proxy", http.MethodDelete, "/orgs/{org}/mcp-proxies/ghost/scopes/read", nil, http.StatusNotFound, "MCP proxy or scope not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(tc.method, tc.path, tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Contains(t, errorMessage(t, rec), tc.message)
		})
	}

	for _, r := range []struct{ method, path, perm string }{
		{http.MethodGet, base, gwperm.ScopeCreate},
		{http.MethodPost, base, gwperm.ScopeRead},
		{http.MethodPut, base + "/read", gwperm.ScopeRead},
		{http.MethodDelete, base + "/read", gwperm.ScopeRead},
	} {
		rec := a.do(r.method, r.path, map[string]any{"action": "x"}, withScopes(scope(r.perm)))
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", r.method, r.path, rec.Body.String())
	}
	list := expectInto[gwspec.MCPProxyScopeListResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.Len(t, list.Scopes, 1, "the refused calls changed nothing")
}

// TestAGRoutesAgentIdentityScopes pins the environment's grantable scopes: only
// identity-secured proxies deployed there contribute, named by proxy.
func TestAGRoutesAgentIdentityScopes(t *testing.T) {
	a, _ := agSetup(t)
	agCreateToolProxy(t, a, "secured", true, "get_forecast")
	agCreateToolProxy(t, a, "keyed", false, "get_forecast")
	expect(t, a.do(http.MethodPost, "/orgs/{org}/mcp-proxies/secured/scopes",
		map[string]any{"action": "read", "description": "read it"}), http.StatusCreated)
	expect(t, a.do(http.MethodPost, "/orgs/{org}/mcp-proxies/keyed/scopes", map[string]any{"action": "read"}), http.StatusCreated)

	got := expectInto[gwspec.AgentIdentityScopeListResponse](t,
		a.do(http.MethodGet, "/orgs/{org}/environments/default/agent-identities/scopes", nil), http.StatusOK)
	require.Len(t, got.Scopes, 1, "only the identity-secured proxy's scope is grantable")
	assert.Equal(t, "secured:read", got.Scopes[0].Scope)
	assert.Equal(t, "read it", *got.Scopes[0].Description)
	assert.Equal(t, "MCP secured", *got.Scopes[0].McpProxyName)

	// Every environment a route names is read as the organization's only one.
	other := expectInto[gwspec.AgentIdentityScopeListResponse](t,
		a.do(http.MethodGet, "/orgs/{org}/environments/elsewhere/agent-identities/scopes", nil), http.StatusOK)
	assert.Equal(t, got.Scopes, other.Scopes)
	rec := a.do(http.MethodGet, "/orgs/{org}/environments/default/agent-identities/scopes", nil,
		withScopes(scope(gwperm.ScopeRead)))
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}
