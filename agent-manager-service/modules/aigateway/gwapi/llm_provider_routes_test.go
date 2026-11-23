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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

// These cover the provider ROUTES: mounting, permissions and org resolution. gwconv covers the documents.

// providerProbe records which handler ran and for which org; any other method panics on the nil embed.
type providerProbe struct {
	gwctl.LLMController
	gotOrg    string
	gotPath   string
	gotLimit  string
	listRan   bool
	createRan bool
	updateRan bool
	deleteRan bool
}

func (c *providerProbe) UpdateLLMProvider(w http.ResponseWriter, r *http.Request) {
	c.updateRan = true
	c.gotOrg = modkit.OrgID(r.Context())
	c.gotPath = r.PathValue("providerId")
	w.WriteHeader(http.StatusOK)
}

func (c *providerProbe) DeleteLLMProvider(w http.ResponseWriter, r *http.Request) {
	c.deleteRan = true
	c.gotOrg = modkit.OrgID(r.Context())
	c.gotPath = r.PathValue("providerId")
	w.WriteHeader(http.StatusNoContent)
}

func (c *providerProbe) CreateLLMProvider(w http.ResponseWriter, r *http.Request) {
	c.createRan = true
	c.gotOrg = modkit.OrgID(r.Context())
	w.WriteHeader(http.StatusCreated)
}

func (c *providerProbe) ListLLMProviders(w http.ResponseWriter, r *http.Request) {
	c.listRan = true
	c.gotOrg = modkit.OrgID(r.Context())
	c.gotLimit = r.URL.Query().Get("limit")
	w.WriteHeader(http.StatusOK)
}

func (c *providerProbe) GetLLMProvider(w http.ResponseWriter, r *http.Request) {
	c.gotOrg = modkit.OrgID(r.Context())
	c.gotPath = r.PathValue("providerId")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]string{"ok": "true"})
}

func mountProviders(t *testing.T, ctrl gwctl.LLMController) *http.ServeMux {
	t.Helper()
	mux := http.NewServeMux()
	registerLLMProviderRoutes(moduleRoute{coreRouter{middleware.NewRouteRegistrar(mux, nil, audit.NewNoopRecorder())}}, ctrl)
	return mux
}

func callGet(t *testing.T, mux *http.ServeMux, target string, scope string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req = req.WithContext(jwtassertion.ContextWithTokenClaimsAndScope(req.Context(),
		&jwtassertion.TokenClaims{Sub: "user-a", OuId: "ou-a", OuHandle: "handle-a", Scope: scope}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// A path with no {orgName} still reaches its handler, with the org taken from the token.
func TestProviderRouteMountsWithoutOrgNameInThePath(t *testing.T) {
	ctrl := &providerProbe{}
	rec := callGet(t, mountProviders(t, ctrl), "/llm-providers/openai-provider", gwperm.Scope(gwperm.LLMProviderRead))

	require.Equal(t, http.StatusOK, rec.Code, "the v2 path must reach its handler")
	assert.Equal(t, "ou-a", ctrl.gotOrg, "the organization comes from the token, not the path")
	assert.Equal(t, "openai-provider", ctrl.gotPath, "the path parameter still binds")
}

// The routes keep the permissions v1 used, so no operator has to re-grant roles.
func TestProviderRouteKeepsItsPermission(t *testing.T) {
	ctrl := &providerProbe{}
	mux := mountProviders(t, ctrl)

	rec := callGet(t, mux, "/llm-providers/p", gwperm.Scope(gwperm.LLMProviderRead))
	assert.Equal(t, http.StatusOK, rec.Code, "the read permission admits the route")

	ctrl2 := &providerProbe{}
	rec = callGet(t, mountProviders(t, ctrl2), "/llm-providers/p", gwperm.Scope(gwperm.LLMProviderDelete))
	assert.Equal(t, http.StatusForbidden, rec.Code, "an unrelated permission does not")
	assert.Empty(t, ctrl2.gotOrg, "the handler must not run for a refused request")
}

// Dropping {orgName} must not drop the tenancy guard: a token with no org is refused.
func TestProviderRouteRefusesATokenWithNoOrg(t *testing.T) {
	ctrl := &providerProbe{}
	mux := mountProviders(t, ctrl)

	req := httptest.NewRequest(http.MethodGet, "/llm-providers/p", nil)
	req = req.WithContext(jwtassertion.ContextWithTokenClaimsAndScope(req.Context(),
		&jwtassertion.TokenClaims{Sub: "user-a", Scope: gwperm.Scope(gwperm.LLMProviderRead)}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusForbidden, rec.Code)
	assert.Empty(t, ctrl.gotOrg, "the handler must not run without a resolved organization")
}

// The collection route must not be shadowed by the item route beside it.
func TestProviderListRouteMounts(t *testing.T) {
	ctrl := &providerProbe{}
	rec := callGet(t, mountProviders(t, ctrl), "/llm-providers?limit=5", gwperm.Scope(gwperm.LLMProviderRead))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, ctrl.listRan, "the collection route reached the list handler, not the item one")
	assert.Equal(t, "ou-a", ctrl.gotOrg, "the organization comes from the token")
	assert.Equal(t, "5", ctrl.gotLimit, "query parameters survive the chain")
	assert.Empty(t, ctrl.gotPath, "the item handler must not have run")
}

// callPost drives a write route.
func callPost(t *testing.T, mux *http.ServeMux, target, body, scope string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req = req.WithContext(jwtassertion.ContextWithTokenClaimsAndScope(req.Context(),
		&jwtassertion.TokenClaims{Sub: "user-a", OuId: "ou-a", OuHandle: "handle-a", Scope: scope}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

// Create is gated on create, not read; a copy-pasted read permission would let any viewer create.
func TestProviderCreateRouteNeedsTheCreatePermission(t *testing.T) {
	ctrl := &providerProbe{}
	rec := callPost(t, mountProviders(t, ctrl), "/llm-providers", `{}`, gwperm.Scope(gwperm.LLMProviderCreate))
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.True(t, ctrl.createRan)
	assert.Equal(t, "ou-a", ctrl.gotOrg)

	readOnly := &providerProbe{}
	rec = callPost(t, mountProviders(t, readOnly), "/llm-providers", `{}`, gwperm.Scope(gwperm.LLMProviderRead))
	assert.Equal(t, http.StatusForbidden, rec.Code, "read must not admit a create")
	assert.False(t, readOnly.createRan, "the handler must not run for a refused request")
}

// POST and GET share a path; each must reach its own handler.
func TestProviderCreateDoesNotCollideWithTheList(t *testing.T) {
	ctrl := &providerProbe{}
	rec := callPost(t, mountProviders(t, ctrl), "/llm-providers", `{}`, gwperm.Scope(gwperm.LLMProviderCreate))
	require.Equal(t, http.StatusCreated, rec.Code)
	assert.True(t, ctrl.createRan)
	assert.False(t, ctrl.listRan, "a POST must not reach the list handler")
}

// callMethod issues a request with no body on the given method.
func callMethod(t *testing.T, mux *http.ServeMux, method, target, scope string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, target, nil)
	req = req.WithContext(jwtassertion.ContextWithTokenClaimsAndScope(req.Context(),
		&jwtassertion.TokenClaims{Sub: "user-a", OuId: "ou-a", OuHandle: "handle-a", Scope: scope}))
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func TestProviderUpdateRouteNeedsTheUpdatePermission(t *testing.T) {
	ctrl := &providerProbe{}
	rec := callMethod(t, mountProviders(t, ctrl), http.MethodPut, "/llm-providers/openai", gwperm.Scope(gwperm.LLMProviderUpdate))
	require.Equal(t, http.StatusOK, rec.Code)
	assert.True(t, ctrl.updateRan)
	assert.Equal(t, "ou-a", ctrl.gotOrg)
	assert.Equal(t, "openai", ctrl.gotPath)

	readOnly := &providerProbe{}
	rec = callMethod(t, mountProviders(t, readOnly), http.MethodPut, "/llm-providers/openai", gwperm.Scope(gwperm.LLMProviderRead))
	assert.Equal(t, http.StatusForbidden, rec.Code, "read must not admit an update")
	assert.False(t, readOnly.updateRan)
}

func TestProviderDeleteRouteNeedsTheDeletePermission(t *testing.T) {
	ctrl := &providerProbe{}
	rec := callMethod(t, mountProviders(t, ctrl), http.MethodDelete, "/llm-providers/openai", gwperm.Scope(gwperm.LLMProviderDelete))
	require.Equal(t, http.StatusNoContent, rec.Code)
	assert.True(t, ctrl.deleteRan)
	assert.Equal(t, "openai", ctrl.gotPath)

	updateOnly := &providerProbe{}
	rec = callMethod(t, mountProviders(t, updateOnly), http.MethodDelete, "/llm-providers/openai", gwperm.Scope(gwperm.LLMProviderUpdate))
	assert.Equal(t, http.StatusForbidden, rec.Code, "update must not admit a delete")
	assert.False(t, updateOnly.deleteRan)
}

// The five provider routes share two paths; each method must reach its own handler.
func TestProviderMethodsDoNotCollide(t *testing.T) {
	all := gwperm.Scope(gwperm.LLMProviderRead) + " " + gwperm.Scope(gwperm.LLMProviderUpdate) +
		" " + gwperm.Scope(gwperm.LLMProviderDelete)

	ctrl := &providerProbe{}
	require.Equal(t, http.StatusOK, callMethod(t, mountProviders(t, ctrl), http.MethodPut, "/llm-providers/p", all).Code)
	assert.True(t, ctrl.updateRan)
	assert.False(t, ctrl.deleteRan)
	assert.False(t, ctrl.createRan)

	ctrl2 := &providerProbe{}
	require.Equal(t, http.StatusNoContent, callMethod(t, mountProviders(t, ctrl2), http.MethodDelete, "/llm-providers/p", all).Code)
	assert.True(t, ctrl2.deleteRan)
	assert.False(t, ctrl2.updateRan)

	ctrl3 := &providerProbe{}
	require.Equal(t, http.StatusOK, callGet(t, mountProviders(t, ctrl3), "/llm-providers/p", all).Code)
	assert.False(t, ctrl3.updateRan, "a GET must not reach the update handler")
	assert.False(t, ctrl3.deleteRan)
}
