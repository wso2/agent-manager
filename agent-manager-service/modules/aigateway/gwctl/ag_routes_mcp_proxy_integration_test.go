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

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

// TestAGRoutesMCPProxyLifecycle creates, reads, lists, updates and deletes an
// org-level MCP proxy through its routes.
func TestAGRoutesMCPProxyLifecycle(t *testing.T) {
	a, gw := agSetup(t)
	env := a.defaultEnv().String()

	created := expectInto[gwmodels.MCPProxyDTO](t,
		a.do(http.MethodPost, "/orgs/{org}/mcp-proxies", agMCPProxyBody("weather", env)), http.StatusCreated)
	assert.Equal(t, "weather", created.ID)
	assert.Equal(t, "MCP weather", created.Name)
	assert.True(t, created.InCatalog)
	require.Len(t, created.Endpoints, 1)
	assert.Equal(t, agPublicURL, created.Endpoints[0].Upstream.Main.URL)
	require.NotNil(t, created.Endpoints[0].Security, "the default security is applied")
	assert.True(t, *created.Endpoints[0].Security.APIKey.Enabled)

	got := expectInto[gwmodels.MCPProxyDTO](t, a.do(http.MethodGet, "/orgs/{org}/mcp-proxies/weather", nil), http.StatusOK)
	assert.Equal(t, []string{gw.UUID.String()}, got.Gateways, "the endpoint is placed on the environment's gateway")
	require.Len(t, got.Endpoints[0].Environments, 1)
	assert.Equal(t, env, got.Endpoints[0].Environments[0].EnvironmentUUID)
	assert.Equal(t, gw.UUID.String(), *got.Endpoints[0].Environments[0].GatewayID)

	list := expect(t, a.do(http.MethodGet, "/orgs/{org}/mcp-proxies?limit=500&offset=-1", nil), http.StatusOK)
	assert.EqualValues(t, 1, list["count"])
	items := list["list"].([]any)
	require.Len(t, items, 1)
	assert.Equal(t, "weather", items[0].(map[string]any)["id"])

	update := agMCPProxyBody("weather", env)
	update["name"] = "Weather v2"
	update["description"] = "renamed"
	updated := expectInto[gwmodels.MCPProxyDTO](t, a.do(http.MethodPut, "/orgs/{org}/mcp-proxies/weather", update), http.StatusOK)
	assert.Equal(t, "Weather v2", updated.Name)
	got = expectInto[gwmodels.MCPProxyDTO](t, a.do(http.MethodGet, "/orgs/{org}/mcp-proxies/weather", nil), http.StatusOK)
	assert.Equal(t, "Weather v2", got.Name)
	assert.Equal(t, "renamed", *got.Description)

	rec := a.do(http.MethodDelete, "/orgs/{org}/mcp-proxies/weather", nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, "/orgs/{org}/mcp-proxies/weather", nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, "MCP proxy not found", errorMessage(t, rec))
}

// TestAGRoutesMCPProxyCreateRejectsBadRequests pins the 400s and 409s a create
// answers, each with the reason the caller sees.
func TestAGRoutesMCPProxyCreateRejectsBadRequests(t *testing.T) {
	a, _ := agSetup(t)
	env := a.defaultEnv().String()
	agCreateMCPProxy(t, a, "taken")

	withEndpoint := func(mutate func(ep map[string]any)) map[string]any {
		body := agMCPProxyBody("fresh", env)
		mutate(body["endpoints"].([]any)[0].(map[string]any))
		return body
	}
	tests := []struct {
		name    string
		body    any
		status  int
		message string
		reason  string
	}{
		{"malformed json", "{", http.StatusBadRequest, "Invalid request body", ""},
		{"handle not kebab-case", agMCPProxyBody("Not_Kebab", env), http.StatusBadRequest, "Bad request", "kebab-case"},
		{"no endpoints", map[string]any{"id": "fresh", "name": "n", "version": "v1"}, http.StatusBadRequest, "Bad request", "at least one endpoint"},
		{"no upstream url", withEndpoint(func(ep map[string]any) { ep["upstream"] = map[string]any{} }), http.StatusBadRequest, "Bad request", "missing an upstream url"},
		{"no environments", withEndpoint(func(ep map[string]any) { delete(ep, "environments") }), http.StatusBadRequest, "Bad request", "at least one environment"},
		{"invalid environment id", withEndpoint(func(ep map[string]any) {
			ep["environments"] = []any{map[string]any{"environmentUuid": "not-a-uuid"}}
		}), http.StatusBadRequest, "Bad request", "invalid environment id"},
		{"already exists", agMCPProxyBody("taken", env), http.StatusConflict, `MCP proxy "taken" already exists`, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPost, "/orgs/{org}/mcp-proxies", tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Equal(t, tc.message, errorMessage(t, rec))
			if tc.reason != "" {
				assert.Contains(t, agReason(t, rec), tc.reason)
			}
		})
	}
	list := expect(t, a.do(http.MethodGet, "/orgs/{org}/mcp-proxies", nil), http.StatusOK)
	assert.EqualValues(t, 1, list["count"], "no rejected create left a proxy")
}

// TestAGRoutesMCPProxyMissingAndForbidden pins the 404s for an unknown proxy and
// the 403 for a token without the route's scope.
func TestAGRoutesMCPProxyMissingAndForbidden(t *testing.T) {
	a, _ := agSetup(t)
	env := a.defaultEnv().String()

	rec := a.do(http.MethodPut, "/orgs/{org}/mcp-proxies/ghost", agMCPProxyBody("ghost", env))
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, "MCP proxy not found", errorMessage(t, rec))
	rec = a.do(http.MethodDelete, "/orgs/{org}/mcp-proxies/ghost", nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	rec = a.do(http.MethodPut, "/orgs/{org}/mcp-proxies/ghost", "{")
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "Invalid request body", errorMessage(t, rec))
	rec = a.do(http.MethodGet, "/orgs/{org}/mcp-proxies?project=no-such-project", nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, "Project not found", errorMessage(t, rec))

	readOnly := withScopes(scope(gwperm.MCPServerRead))
	for _, r := range []struct{ method, path string }{
		{http.MethodPost, "/orgs/{org}/mcp-proxies"},
		{http.MethodPut, "/orgs/{org}/mcp-proxies/ghost"},
		{http.MethodDelete, "/orgs/{org}/mcp-proxies/ghost"},
		{http.MethodPost, "/orgs/{org}/mcp-proxies/fetch-server-info"},
	} {
		rec := a.do(r.method, r.path, agMCPProxyBody("ghost", env), readOnly)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", r.method, r.path, rec.Body.String())
	}
	rec = a.do(http.MethodGet, "/orgs/{org}/mcp-proxies", nil, withScopes(scope(gwperm.MCPServerCreate)))
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

// TestAGRoutesMCPProxyProjectFilterAndOwnership pins that ?project= adds the
// project's proxies to the listing, and that the org routes refuse to change a
// project-owned proxy.
func TestAGRoutesMCPProxyProjectFilterAndOwnership(t *testing.T) {
	a, _ := agSetup(t)
	env := a.defaultEnv().String()
	agCreateMCPProxy(t, a, "org-proxy")
	agCreateMCPProxy(t, a, "proj-proxy")
	var projectID string
	require.NoError(t, a.db.Raw(`SELECT id FROM projects WHERE org_id = ? AND name = ?`, a.org, agProject).Scan(&projectID).Error)
	require.NoError(t, a.db.Exec(`UPDATE mod_ai_gateway_mcp_proxies SET project_id = ?
		WHERE id = (SELECT id FROM mod_ai_gateway_artifacts WHERE handle = ? AND org_id = ?)`,
		projectID, "proj-proxy", a.org).Error)

	orgOnly := expect(t, a.do(http.MethodGet, "/orgs/{org}/mcp-proxies", nil), http.StatusOK)
	assert.EqualValues(t, 1, orgOnly["count"], "the org listing leaves out project proxies")
	withProject := expect(t, a.do(http.MethodGet, "/orgs/{org}/mcp-proxies?project="+agProject, nil), http.StatusOK)
	assert.EqualValues(t, 2, withProject["count"])

	rec := a.do(http.MethodPut, "/orgs/{org}/mcp-proxies/proj-proxy", agMCPProxyBody("proj-proxy", env))
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Equal(t, "MCP proxy belongs to a project", errorMessage(t, rec))
	rec = a.do(http.MethodDelete, "/orgs/{org}/mcp-proxies/proj-proxy", nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Equal(t, "MCP proxy belongs to a project", errorMessage(t, rec))
	rec = a.do(http.MethodPost, "/orgs/{org}/mcp-proxies/proj-proxy/scopes", map[string]any{"action": "read"})
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Equal(t, "MCP proxy belongs to a project", errorMessage(t, rec))
}

// TestAGRoutesMCPProxyDeleteRefusedWhileBound pins that a proxy an agent's MCP
// configuration uses cannot be deleted until the configuration is.
func TestAGRoutesMCPProxyDeleteRefusedWhileBound(t *testing.T) {
	a, _ := agSetup(t)
	agCreateMCPProxy(t, a, "bound")
	cfg := expect(t, a.do(http.MethodPost, agAgentPath("/mcp-configs"), agMCPConfigBody("tools", "bound")), http.StatusCreated)

	rec := a.do(http.MethodDelete, "/orgs/{org}/mcp-proxies/bound", nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, errorMessage(t, rec), "associated MCP proxy mappings")
	expect(t, a.do(http.MethodGet, "/orgs/{org}/mcp-proxies/bound", nil), http.StatusOK)

	rec = a.do(http.MethodDelete, agAgentPath("/mcp-configs/"+cfg["uuid"].(string)), nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodDelete, "/orgs/{org}/mcp-proxies/bound", nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
}

// TestAGRoutesMCPProxyPoliciesAndProbe pins the policy listing of a gateway
// that has reported no policies, and the probe's refusals of URLs it must not
// call.
func TestAGRoutesMCPProxyPoliciesAndProbe(t *testing.T) {
	a, _ := agSetup(t)

	policies := expect(t, a.do(http.MethodGet, "/orgs/{org}/mcp-proxies/policies", nil), http.StatusOK)
	assert.EqualValues(t, 0, policies["count"])

	tests := []struct {
		name    string
		body    any
		message string
	}{
		{"malformed json", "{", "Invalid request body"},
		{"no url", map[string]any{}, "Bad request"},
		{"refresh by proxy id", map[string]any{"url": agPublicURL, "proxyId": "p"}, "Bad request"},
		{"loopback url", map[string]any{"url": "http://127.0.0.1:9/mcp"}, "Bad request"},
		{"not a url", map[string]any{"url": "ftp://example"}, "Bad request"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPost, "/orgs/{org}/mcp-proxies/fetch-server-info", tc.body)
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Equal(t, tc.message, errorMessage(t, rec))
		})
	}
}
