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

package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// These tests install the process-wide OrgOverrideResolver; none runs in parallel.

const (
	overridePattern = "PUT /orgs/{orgName}/projects/{projName}/mcp-proxies/{proxyId}"
	plainPattern    = "PUT /orgs/{orgName}/mcp-proxies/{proxyId}"
)

type registrarCall struct {
	status   int
	ran      bool
	resolved ResolvedOrg
	body     string
}

func newOverrideRegistrar(t *testing.T) (*http.ServeMux, *registrarCall) {
	t.Helper()
	mux := http.NewServeMux()
	rr := NewRouteRegistrar(mux, stubResolver{}, audit.NewNoopRecorder())
	call := &registrarCall{}
	handler := func(w http.ResponseWriter, r *http.Request) {
		call.ran = true
		call.resolved, _ = GetResolvedOrg(r.Context())
		w.WriteHeader(http.StatusOK)
	}
	lookup := func(_ context.Context, ouID string) (string, error) { return "handle-of-" + ouID, nil }
	rr.HandleFuncWithValidationAndAuthzAndOrgOverride(overridePattern, rbac.MCPServerUpdate, ApplyOrgOverride(lookup), handler)
	rr.HandleFuncWithValidationAndAuthz(plainPattern, rbac.MCPServerUpdate, handler)
	return mux, call
}

func serveRegistrar(t *testing.T, mux *http.ServeMux, call *registrarCall, path string, scopes string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPut, path, nil)
	req.Header.Set(occlient.HeaderImpersonateOrg, overrideTargetOUID)
	claims := &jwtassertion.TokenClaims{
		ClientID: overrideClientID, OuId: overrideTokenOUID, OuHandle: overrideTokenHandle, Scope: scopes,
	}
	req = req.WithContext(jwtassertion.ContextWithTokenClaimsAndScope(req.Context(), claims))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	call.status = w.Code
	call.body = w.Body.String()
}

func TestRegistrarOrgOverride_AppliedOnlyWhereComposed(t *testing.T) {
	installOrgOverrideResolver(t, trustingResolver)
	mux, call := newOverrideRegistrar(t)
	scope := rbac.MCPServerUpdate.Scope()

	serveRegistrar(t, mux, call, "/orgs/handle-of-"+overrideTargetOUID+"/projects/p1/mcp-proxies/h1", scope)
	require.Equal(t, http.StatusOK, call.status, call.body)
	assert.Equal(t, ResolvedOrg{OUID: overrideTargetOUID, OuHandle: "handle-of-" + overrideTargetOUID}, call.resolved)

	*call = registrarCall{}
	serveRegistrar(t, mux, call, "/orgs/x/mcp-proxies/h1", scope)
	require.Equal(t, http.StatusOK, call.status, call.body)
	assert.Equal(t, ResolvedOrg{OUID: overrideTokenOUID, OuHandle: overrideTokenHandle}, call.resolved,
		"a route registered without the override keeps the token's org whatever the header says")
}

// The override runs before the permission check: an impersonation the
// deployment does not permit is refused as such, whatever scopes the caller
// holds — and a permitted one still has to pass the scope check.
func TestRegistrarOrgOverride_OrderedBetweenOrgMatchAndAuthz(t *testing.T) {
	installOrgOverrideResolver(t, nil)
	mux, call := newOverrideRegistrar(t)
	serveRegistrar(t, mux, call, "/orgs/x/projects/p1/mcp-proxies/h1", "")
	assert.Equal(t, http.StatusForbidden, call.status)
	assert.Contains(t, call.body, "org impersonation not permitted")
	assert.False(t, call.ran)

	installOrgOverrideResolver(t, trustingResolver)
	*call = registrarCall{}
	serveRegistrar(t, mux, call, "/orgs/handle-of-"+overrideTargetOUID+"/projects/p1/mcp-proxies/h1", "")
	assert.Equal(t, http.StatusForbidden, call.status)
	assert.Contains(t, call.body, "insufficient permissions")
	assert.False(t, call.ran)
}

func TestRegistrarOrgOverride_RejectsMisuse(t *testing.T) {
	rr := NewRouteRegistrar(http.NewServeMux(), stubResolver{}, audit.NewNoopRecorder())
	noop := func(http.ResponseWriter, *http.Request) {}
	override := ApplyOrgOverride(func(context.Context, string) (string, error) { return "h", nil })

	assert.Panics(t, func() {
		rr.HandleFuncWithValidationAndAuthzAndOrgOverride("PUT /platform/x", rbac.MCPServerUpdate, override, noop)
	}, "an override needs the org resolved first, so the route must be org-scoped")
	assert.Panics(t, func() {
		rr.HandleFuncWithValidationAndAuthzAndOrgOverride(overridePattern, rbac.MCPServerUpdate, nil, noop)
	})
}
