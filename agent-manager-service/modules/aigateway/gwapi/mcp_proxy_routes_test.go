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

package gwapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

// These cover the MCP proxy ROUTES: mounting, permissions and org resolution. gwconv covers the documents.

// proxyProbe records which handler ran and for which org; any other method panics on the nil embed.
type proxyProbe struct {
	gwctl.MCPProxyController
	gotOrg      string
	gotProxy    string
	ran         string
	gotProject  string
	createRan   bool
	listRan     bool
	policiesRan bool
	fetchRan    bool
}

func (c *proxyProbe) CreateMCPProxy(w http.ResponseWriter, r *http.Request) {
	c.ran, c.createRan = "create", true
	c.gotOrg = modkit.OrgID(r.Context())
	w.WriteHeader(http.StatusCreated)
}

func (c *proxyProbe) ListMCPProxies(w http.ResponseWriter, r *http.Request) {
	c.ran, c.listRan = "list", true
	c.gotOrg = modkit.OrgID(r.Context())
	c.gotProject = r.URL.Query().Get("project")
	w.WriteHeader(http.StatusOK)
}

func (c *proxyProbe) ListAvailableMCPPolicies(w http.ResponseWriter, r *http.Request) {
	c.ran, c.policiesRan = "policies", true
	c.gotOrg = modkit.OrgID(r.Context())
	w.WriteHeader(http.StatusOK)
}

func (c *proxyProbe) GetMCPProxy(w http.ResponseWriter, r *http.Request) {
	c.ran = "get"
	c.gotOrg = modkit.OrgID(r.Context())
	c.gotProxy = r.PathValue("proxyId")
	w.WriteHeader(http.StatusOK)
}

func (c *proxyProbe) UpdateMCPProxy(w http.ResponseWriter, r *http.Request) {
	c.ran = "update"
	c.gotOrg = modkit.OrgID(r.Context())
	c.gotProxy = r.PathValue("proxyId")
	w.WriteHeader(http.StatusOK)
}

func (c *proxyProbe) DeleteMCPProxy(w http.ResponseWriter, r *http.Request) {
	c.ran = "delete"
	c.gotOrg = modkit.OrgID(r.Context())
	c.gotProxy = r.PathValue("proxyId")
	w.WriteHeader(http.StatusNoContent)
}

func (c *proxyProbe) FetchServerInfo(w http.ResponseWriter, r *http.Request) {
	c.ran, c.fetchRan = "fetch", true
	c.gotOrg = modkit.OrgID(r.Context())
	w.WriteHeader(http.StatusOK)
}

// scopeProbe is the same for the per-proxy scope routes.
type scopeProbe struct {
	gwctl.MCPProxyScopeController
	gotOrg    string
	gotProxy  string
	gotAction string
	ran       string
}

func (c *scopeProbe) ListMCPProxyScopes(w http.ResponseWriter, r *http.Request) {
	c.record(r, "list")
	w.WriteHeader(http.StatusOK)
}

func (c *scopeProbe) CreateMCPProxyScope(w http.ResponseWriter, r *http.Request) {
	c.record(r, "create")
	w.WriteHeader(http.StatusCreated)
}

func (c *scopeProbe) UpdateMCPProxyScope(w http.ResponseWriter, r *http.Request) {
	c.record(r, "update")
	w.WriteHeader(http.StatusOK)
}

func (c *scopeProbe) DeleteMCPProxyScope(w http.ResponseWriter, r *http.Request) {
	c.record(r, "delete")
	w.WriteHeader(http.StatusNoContent)
}

func (c *scopeProbe) record(r *http.Request, ran string) {
	c.ran = ran
	c.gotOrg = modkit.OrgID(r.Context())
	c.gotProxy = r.PathValue("proxyId")
	c.gotAction = r.PathValue("scopeAction")
}

func mountProxies(t *testing.T, ctrl gwctl.MCPProxyController) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	RegisterMCPProxyRoutes(moduleRoute{coreRouter{middleware.NewRouteRegistrar(mux, nil, audit.NewNoopRecorder())}}, ctrl)
	return mux
}

func mountProxyScopes(t *testing.T, ctrl gwctl.MCPProxyScopeController) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	registerMCPProxyScopeDocumentRoutes(moduleRoute{coreRouter{middleware.NewRouteRegistrar(mux, nil, audit.NewNoopRecorder())}}, ctrl)
	return mux
}

// A path with no {orgName} still reaches its handler, with the org taken from the token.
func TestMCPProxyRouteMountsWithoutOrgNameInThePath(t *testing.T) {
	ctrl := &proxyProbe{}
	rec := callGet(t, mountProxies(t, ctrl), "/mcp-proxies/weather", gwperm.Scope(gwperm.MCPServerRead))

	require.Equal(t, http.StatusOK, rec.Code, "the document path must reach its handler")
	assert.Equal(t, "get", ctrl.ran)
	assert.Equal(t, "01900000-0000-7000-8000-0000000000a1", ctrl.gotOrg, "the organization comes from the token, not the path")
	assert.Equal(t, "weather", ctrl.gotProxy, "the path parameter still binds")
}

// Dropping {orgName} must not drop the tenancy guard: a token with no org is refused.
func TestMCPProxyRouteRefusesATokenWithNoOrg(t *testing.T) {
	ctrl := &proxyProbe{}
	req := httptest.NewRequest(http.MethodGet, "/mcp-proxies/weather", nil)
	req = req.WithContext(jwtassertion.ContextWithTokenClaimsAndScope(req.Context(),
		&jwtassertion.TokenClaims{Sub: "user-a", Scope: gwperm.Scope(gwperm.MCPServerRead)}))
	rec := httptest.NewRecorder()
	mountProxies(t, ctrl).ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, ctrl.gotOrg, "the handler must not run without a resolved organization")
}

// The literal sub-paths sit beside /mcp-proxies/{proxyId}; neither may be swallowed by it.
func TestMCPProxyLiteralPathsAreNotShadowed(t *testing.T) {
	ctrl := &proxyProbe{}
	rec := callGet(t, mountProxies(t, ctrl), "/mcp-proxies/policies", gwperm.Scope(gwperm.MCPServerRead))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, ctrl.policiesRan, "/policies reached the policy handler, not the item one")
	assert.Empty(t, ctrl.gotProxy)

	probe := &proxyProbe{}
	rec = callPost(t, mountProxies(t, probe), "/mcp-proxies/fetch-server-info", `{}`, gwperm.Scope(gwperm.MCPServerConnect))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, probe.fetchRan, "/fetch-server-info reached the probe handler")
	assert.Equal(t, "01900000-0000-7000-8000-0000000000a1", probe.gotOrg)
}

// The collection route must not be shadowed by the item route beside it, and its query survives.
func TestMCPProxyListRouteMounts(t *testing.T) {
	ctrl := &proxyProbe{}
	rec := callGet(t, mountProxies(t, ctrl), "/mcp-proxies?project=shop", gwperm.Scope(gwperm.MCPServerRead))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, ctrl.listRan, "the collection route reached the list handler, not the item one")
	assert.Equal(t, "shop", ctrl.gotProject, "query parameters survive the chain")
	assert.Empty(t, ctrl.gotProxy, "the item handler must not have run")
}

// POST and GET share a path; each must reach its own handler.
func TestMCPProxyCreateDoesNotCollideWithTheList(t *testing.T) {
	ctrl := &proxyProbe{}
	rec := callPost(t, mountProxies(t, ctrl), "/mcp-proxies", `{}`, gwperm.Scope(gwperm.MCPServerCreate))

	require.Equal(t, http.StatusCreated, rec.Code)
	assert.True(t, ctrl.createRan)
	assert.False(t, ctrl.listRan, "a POST must not reach the list handler")
}

// The routes keep the permissions v1 used, so no operator has to re-grant roles.
func TestMCPProxyRoutesKeepTheirPermissions(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		target  string
		granted string
		refused string
		status  int
	}{
		{"create", http.MethodPost, "/mcp-proxies", gwperm.MCPServerCreate, gwperm.MCPServerRead, http.StatusCreated},
		{"update", http.MethodPut, "/mcp-proxies/p", gwperm.MCPServerUpdate, gwperm.MCPServerRead, http.StatusOK},
		{"delete", http.MethodDelete, "/mcp-proxies/p", gwperm.MCPServerDelete, gwperm.MCPServerUpdate, http.StatusNoContent},
		{"probe", http.MethodPost, "/mcp-proxies/fetch-server-info", gwperm.MCPServerConnect, gwperm.MCPServerRead, http.StatusOK},
		{"list", http.MethodGet, "/mcp-proxies", gwperm.MCPServerRead, gwperm.MCPServerCreate, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := &proxyProbe{}
			rec := callMethod(t, mountProxies(t, ctrl), tc.method, tc.target, gwperm.Scope(tc.granted))
			require.Equal(t, tc.status, rec.Code, "the route's own permission admits it")
			assert.Equal(t, "01900000-0000-7000-8000-0000000000a1", ctrl.gotOrg)

			refused := &proxyProbe{}
			rec = callMethod(t, mountProxies(t, refused), tc.method, tc.target, gwperm.Scope(tc.refused))
			assert.Equal(t, http.StatusForbidden, rec.Code, "an unrelated permission does not")
			assert.Empty(t, refused.ran, "the handler must not run for a refused request")
		})
	}
}

// The scope routes nest under the proxy; both path parameters must bind, and each method must
// reach its own handler on the two shared paths.
func TestMCPProxyScopeRoutesBindBothPathParameters(t *testing.T) {
	ctrl := &scopeProbe{}
	rec := callGet(t, mountProxyScopes(t, ctrl), "/mcp-proxies/weather/scopes", gwperm.Scope(gwperm.ScopeRead))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "list", ctrl.ran)
	assert.Equal(t, "01900000-0000-7000-8000-0000000000a1", ctrl.gotOrg)
	assert.Equal(t, "weather", ctrl.gotProxy)
	assert.Empty(t, ctrl.gotAction)

	created := &scopeProbe{}
	rec = callPost(t, mountProxyScopes(t, created), "/mcp-proxies/weather/scopes", `{}`, gwperm.Scope(gwperm.ScopeCreate))
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.Equal(t, "create", created.ran, "a POST must not reach the list handler")

	updated := &scopeProbe{}
	rec = callMethod(t, mountProxyScopes(t, updated), http.MethodPut, "/mcp-proxies/weather/scopes/read", gwperm.Scope(gwperm.ScopeUpdate))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "update", updated.ran)
	assert.Equal(t, "weather", updated.gotProxy)
	assert.Equal(t, "read", updated.gotAction)

	deleted := &scopeProbe{}
	rec = callMethod(t, mountProxyScopes(t, deleted), http.MethodDelete, "/mcp-proxies/weather/scopes/read", gwperm.Scope(gwperm.ScopeDelete))
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "delete", deleted.ran, "a DELETE must not reach the update handler")
	assert.Equal(t, "read", deleted.gotAction)
}

// Scope writes are gated on their own permissions: a copy-pasted read would let any viewer grant.
func TestMCPProxyScopeRoutesKeepTheirPermissions(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		target  string
		granted string
		refused string
		status  int
	}{
		{"create", http.MethodPost, "/mcp-proxies/p/scopes", gwperm.ScopeCreate, gwperm.ScopeRead, http.StatusCreated},
		{"update", http.MethodPut, "/mcp-proxies/p/scopes/read", gwperm.ScopeUpdate, gwperm.ScopeRead, http.StatusOK},
		{"delete", http.MethodDelete, "/mcp-proxies/p/scopes/read", gwperm.ScopeDelete, gwperm.ScopeRead, http.StatusNoContent},
		{"list", http.MethodGet, "/mcp-proxies/p/scopes", gwperm.ScopeRead, gwperm.ScopeCreate, http.StatusOK},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := &scopeProbe{}
			rec := callMethod(t, mountProxyScopes(t, ctrl), tc.method, tc.target, gwperm.Scope(tc.granted))
			require.Equal(t, tc.status, rec.Code, "the route's own permission admits it")

			refused := &scopeProbe{}
			rec = callMethod(t, mountProxyScopes(t, refused), tc.method, tc.target, gwperm.Scope(tc.refused))
			assert.Equal(t, http.StatusForbidden, rec.Code, "an unrelated permission does not")
			assert.Empty(t, refused.ran, "the handler must not run for a refused request")
		})
	}
}
