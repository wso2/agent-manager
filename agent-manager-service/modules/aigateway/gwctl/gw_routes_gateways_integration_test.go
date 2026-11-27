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

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// gwDoc is a Gateway document registering a self-hosted gateway named name.
func gwDoc(name string, spec map[string]any) map[string]any {
	s := map[string]any{"displayName": "Gateway " + name, "endpoint": "https://" + name + ".example.com"}
	for k, v := range spec {
		s[k] = v
	}
	return map[string]any{
		"apiVersion": "ai-control-plane.choreolabs.com/v1", "kind": "Gateway",
		"metadata": map[string]any{"name": name}, "spec": s,
	}
}

// gwRegister registers a self-hosted gateway through the API and returns its document.
func gwRegister(t *testing.T, a *routeAPI, name string) gwspec.GatewayDocument {
	t.Helper()
	return expectInto[gwspec.GatewayDocument](t, a.do(http.MethodPost, "/gateways", gwDoc(name, nil)), http.StatusCreated)
}

// gwRegisterUID registers a self-hosted gateway and returns its UUID.
func gwRegisterUID(t *testing.T, a *routeAPI, name string) string {
	t.Helper()
	doc := gwRegister(t, a, name)
	return doc.Metadata.GetUid()
}

// gwMappedTo returns the environment a gateway is mapped to, read from the database.
func gwMappedTo(t *testing.T, a *routeAPI, gateway string) []string {
	t.Helper()
	var envs []string
	require.NoError(t, a.db.Raw(`SELECT environment_id FROM mod_ai_gateway_gateway_environments WHERE gateway_id = ?`, gateway).Scan(&envs).Error)
	return envs
}

// TestGWRoutesGatewayLifecycle registers, reads, updates and deletes a self-hosted gateway, reading each write back.
func TestGWRoutesGatewayLifecycle(t *testing.T) {
	a := newRouteAPI(t)
	name := unique("gw")

	created := expectInto[gwspec.GatewayDocument](t, a.do(http.MethodPost, "/gateways", gwDoc(name, map[string]any{
		"description": "edge", "isCritical": true, "properties": map[string]any{"site": "berlin"},
	})), http.StatusCreated)
	require.Equal(t, "Gateway", created.Kind)
	require.Equal(t, name, created.Metadata.Name)
	require.NotEmpty(t, created.Metadata.GetUid())
	require.Equal(t, gwspec.GATEWAYHOSTING_SELF_HOSTED, created.Spec.GetHosting())
	require.Equal(t, gwspec.GATEWAYTYPE_EGRESS, created.Spec.GetType())
	require.Equal(t, "https://"+name+".example.com", created.Spec.GetEndpoint())
	require.True(t, created.Spec.GetIsCritical())
	require.Equal(t, "edge", created.Spec.GetDescription())
	require.Equal(t, map[string]any{"site": "berlin"}, created.Spec.Properties)
	require.Nil(t, created.Spec.RuntimeUrl)
	require.Equal(t, gwspec.GATEWAYSTATUS_INACTIVE, created.Status.GetState(), "a new gateway has not connected")
	uid := created.Metadata.GetUid()
	require.Equal(t, []string{a.defaultEnv().String()}, gwMappedTo(t, a, uid), "it serves the default environment")

	// Read back by id and by name.
	for _, id := range []string{uid, name} {
		got := expectInto[gwspec.GatewayDocument](t, a.do(http.MethodGet, "/gateways/"+id, nil), http.StatusOK)
		require.Equal(t, uid, got.Metadata.GetUid())
	}

	// A PUT replaces the editable fields; the round-tripped document carries the fixed ones unchanged.
	created.Spec.DisplayName = "Renamed"
	created.Spec.Description = nil
	created.Spec.Properties = nil
	created.Spec.IsCritical = gwspec.PtrBool(false)
	upd := expectInto[gwspec.GatewayDocument](t, a.do(http.MethodPut, "/gateways/"+name, created), http.StatusOK)
	require.Equal(t, "Renamed", upd.Spec.DisplayName)
	require.False(t, upd.Spec.GetIsCritical())
	require.Nil(t, upd.Spec.Description, "an absent description is cleared")
	require.Empty(t, upd.Spec.Properties, "absent properties are cleared")

	got := expectInto[gwspec.GatewayDocument](t, a.do(http.MethodGet, "/gateways/"+uid, nil), http.StatusOK)
	require.Equal(t, "Renamed", got.Spec.DisplayName)

	// Delete, then the gateway is gone.
	rec := a.do(http.MethodDelete, "/gateways/"+name, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, "/gateways/"+uid, nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "Gateway not found", errorMessage(t, rec))
	rec = a.do(http.MethodDelete, "/gateways/"+uid, nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// TestGWRoutesRegisterGatewayRejections pins the status and message of each refused registration.
func TestGWRoutesRegisterGatewayRejections(t *testing.T) {
	a := newRouteAPI(t)
	taken := unique("gw")
	gwRegister(t, a, taken)

	endpointMsg := "The gateway needs a base http or https URL, such as https://gateway.example.com, with no path, query or fragment"
	cases := []struct {
		name    string
		body    any
		status  int
		message string
	}{
		{"malformed body", "{bad", http.StatusBadRequest, "Invalid request body"},
		{"duplicate name", gwDoc(taken, nil), http.StatusConflict, "A gateway with this name already exists"},
		{"short name", gwDoc("ab", nil), http.StatusBadRequest, ""},
		{"uppercase name", gwDoc("Bad-Name", nil), http.StatusBadRequest, ""},
		{"empty display name", gwDoc(unique("gw"), map[string]any{"displayName": "   "}), http.StatusBadRequest, "invalid document: spec.displayName is required"},
		{"managed", gwDoc(unique("gw"), map[string]any{"hosting": "MANAGED"}), http.StatusBadRequest, ""},
		{"ingress", gwDoc(unique("gw"), map[string]any{"type": "INGRESS"}), http.StatusBadRequest, ""},
		{"no endpoint", gwDoc(unique("gw"), map[string]any{"endpoint": nil}), http.StatusBadRequest, "invalid document: spec.endpoint is required"},
		{"endpoint with path", gwDoc(unique("gw"), map[string]any{"endpoint": "https://gw.example.com/api"}), http.StatusBadRequest, endpointMsg},
		{"endpoint with query", gwDoc(unique("gw"), map[string]any{"endpoint": "https://gw.example.com?x=1"}), http.StatusBadRequest, endpointMsg},
		{"non-http endpoint", gwDoc(unique("gw"), map[string]any{"endpoint": "ftp://gw.example.com"}), http.StatusBadRequest, endpointMsg},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPost, "/gateways", tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			if tc.message != "" {
				require.Equal(t, tc.message, errorMessage(t, rec))
			} else {
				require.NotEmpty(t, errorMessage(t, rec))
			}
		})
	}

	list := expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/gateways", nil), http.StatusOK)
	require.EqualValues(t, 1, list.GetCount(), "nothing refused was stored")
}

// TestGWRoutesUpdateGatewayRejections refuses a change to a fixed field, and any change to a managed gateway.
func TestGWRoutesUpdateGatewayRejections(t *testing.T) {
	a := newRouteAPI(t)
	name := unique("gw")
	gwRegister(t, a, name)
	managed := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)

	for _, tc := range []struct {
		name, path string
		body       any
		status     int
	}{
		{"bad body", "/gateways/" + name, "{bad", http.StatusBadRequest},
		{"other name", "/gateways/" + name, gwDoc(unique("gw"), nil), http.StatusBadRequest},
		{"endpoint", "/gateways/" + name, gwDoc(name, map[string]any{"endpoint": "https://other.example.com"}), http.StatusBadRequest},
		{"type", "/gateways/" + name, gwDoc(name, map[string]any{"type": "BOTH"}), http.StatusBadRequest},
		{"hosting", "/gateways/" + name, gwDoc(name, map[string]any{"hosting": "MANAGED"}), http.StatusBadRequest},
		{"managed update", "/gateways/" + managed.UUID.String(), gwDoc(managed.Name, nil), http.StatusForbidden},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPut, tc.path, tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
		})
	}

	rec := a.do(http.MethodDelete, "/gateways/"+managed.UUID.String(), nil)
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Equal(t, "A managed gateway is read-only", errorMessage(t, rec))

	got := expectInto[gwspec.GatewayDocument](t, a.do(http.MethodGet, "/gateways/"+managed.UUID.String(), nil), http.StatusOK)
	require.Equal(t, "Test gateway", got.Spec.DisplayName, "the managed gateway is untouched")
	require.Equal(t, gwspec.GATEWAYHOSTING_MANAGED, got.Spec.GetHosting())
}

// TestGWRoutesGatewayNotFound pins the 404s of the single-gateway routes.
func TestGWRoutesGatewayNotFound(t *testing.T) {
	a := newRouteAPI(t)
	missing := uuid.NewString()

	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/gateways/" + missing, nil},
		{http.MethodGet, "/gateways/no-such-gateway", nil},
		{http.MethodPut, "/gateways/" + missing, gwDoc("x-gateway", nil)},
		{http.MethodDelete, "/gateways/" + missing, nil},
		{http.MethodGet, "/gateways/" + missing + "/health", nil},
		{http.MethodGet, "/gateways/" + missing + "/tokens", nil},
		{http.MethodPost, "/gateways/" + missing + "/tokens", nil},
		{http.MethodDelete, "/gateways/" + missing + "/tokens/" + uuid.NewString(), nil},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := a.do(tc.method, tc.path, tc.body)
			require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
			require.Contains(t, []string{"Gateway not found", "Resource not found"}, errorMessage(t, rec))
		})
	}

	// Another organization's gateway is as unknown as a missing one.
	other := newRouteAPI(t)
	foreign := gwtest.Gateway(t, other.db, other.org, gwmodels.GatewayRoleEgress)
	rec := a.do(http.MethodGet, "/gateways/"+foreign.UUID.String(), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TestGWRoutesOldGatewayPathsAreGone pins that the organization-scoped and self-hosted routes are no longer served.
func TestGWRoutesOldGatewayPathsAreGone(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress).UUID.String()
	for _, path := range []string{
		"/orgs/{org}/gateways",
		"/orgs/{org}/gateways/" + gw,
		"/orgs/{org}/gateways/" + gw + "/environments",
		"/orgs/{org}/self-hosted-gateways",
		"/orgs/{org}/identity-providers",
		"/orgs/{org}/environments/default/gateways",
		"/orgs/{org}/environments/default/identity-providers",
	} {
		rec := a.do(http.MethodGet, path, nil)
		require.Equal(t, http.StatusNotFound, rec.Code, "%s: %s", path, rec.Body.String())
	}
}

// TestGWRoutesGatewayScopes refuses each gateway route to a token without its permission.
func TestGWRoutesGatewayScopes(t *testing.T) {
	a := newRouteAPI(t)
	name := unique("gw")
	gw := gwRegisterUID(t, a, name)
	readOnly := withScopes(scope(gwperm.Read))

	for _, tc := range []struct {
		method, path string
		body         any
		opt          reqOpt
	}{
		{http.MethodPost, "/gateways", gwDoc(unique("gw"), nil), readOnly},
		{http.MethodGet, "/gateways", nil, withScopes(scope(gwperm.Update))},
		{http.MethodGet, "/gateways/" + gw, nil, withScopes(scope(gwperm.Update))},
		{http.MethodPut, "/gateways/" + gw, gwDoc(name, map[string]any{"displayName": "x"}), readOnly},
		{http.MethodDelete, "/gateways/" + gw, nil, readOnly},
		{http.MethodGet, "/gateways/" + gw + "/tokens", nil, readOnly},
		{http.MethodPost, "/gateways/" + gw + "/tokens", nil, readOnly},
		{http.MethodGet, "/gateways/status", nil, withScopes(scope(gwperm.Update))},
		{http.MethodGet, "/identity-providers", nil, withScopes(scope(gwperm.Update))},
		{http.MethodGet, "/identity-providers/discover?url=https://example.com", nil, readOnly},
		{http.MethodPut, "/gateways/" + gw + "/identity-providers/idp", map[string]any{}, readOnly},
		{http.MethodGet, "/orgs/{org}/catalog", nil, readOnly},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := a.do(tc.method, tc.path, tc.body, tc.opt)
			require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		})
	}

	// The gateway is untouched by the refused writes.
	got := expectInto[gwspec.GatewayDocument](t, a.do(http.MethodGet, "/gateways/"+gw, nil), http.StatusOK)
	require.Equal(t, "Gateway "+name, got.Spec.DisplayName)
}

// TestGWRoutesListGatewaysFiltersAndPages filters by type, status and hosting, and pages the result.
func TestGWRoutesListGatewaysFiltersAndPages(t *testing.T) {
	a := newRouteAPI(t)
	env := a.defaultEnv()

	ingress := gwtest.GatewayIn(t, a.db, a.org, gwmodels.GatewayRoleIngress, env)
	egress := gwtest.GatewayIn(t, a.db, a.org, gwmodels.GatewayRoleEgress, env)
	selfHosted := gwRegisterUID(t, a, unique("gw"))

	ids := func(q string) []string {
		t.Helper()
		list := expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/gateways"+q, nil), http.StatusOK)
		out := make([]string, 0, len(list.List))
		for _, g := range list.List {
			out = append(out, g.Metadata.GetUid())
		}
		return out
	}

	all := []string{ingress.UUID.String(), egress.UUID.String(), selfHosted}
	require.ElementsMatch(t, all, ids(""))
	require.ElementsMatch(t, []string{ingress.UUID.String()}, ids("?type=INGRESS"))
	require.ElementsMatch(t, []string{egress.UUID.String(), selfHosted}, ids("?type=EGRESS"))
	require.ElementsMatch(t, []string{selfHosted}, ids("?status=INACTIVE"))
	require.ElementsMatch(t, []string{ingress.UUID.String(), egress.UUID.String()}, ids("?status=ACTIVE"))
	require.ElementsMatch(t, []string{selfHosted}, ids("?hosting=SELF_HOSTED"))
	require.ElementsMatch(t, []string{ingress.UUID.String(), egress.UUID.String()}, ids("?hosting=MANAGED"))

	rec := a.do(http.MethodGet, "/gateways?hosting=cloud", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	page := expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/gateways?limit=2&offset=1", nil), http.StatusOK)
	require.EqualValues(t, 3, page.Pagination.GetTotal())
	require.EqualValues(t, 2, page.Pagination.GetLimit())
	require.EqualValues(t, 1, page.Pagination.GetOffset())
	require.EqualValues(t, 2, page.GetCount())
	require.Len(t, page.List, 2)

	// Negative values fall back to the defaults; an oversized page is capped.
	page = expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/gateways?limit=-1&offset=-5", nil), http.StatusOK)
	require.EqualValues(t, 100, page.Pagination.GetLimit())
	require.EqualValues(t, 0, page.Pagination.GetOffset())
	page = expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/gateways?limit=5000", nil), http.StatusOK)
	require.EqualValues(t, 1000, page.Pagination.GetLimit())
}

// TestGWRoutesGatewayHealthAndStatus reports a gateway's health and the organization's gateway statuses.
func TestGWRoutesGatewayHealthAndStatus(t *testing.T) {
	a := newRouteAPI(t)
	active := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	inactive := gwtest.GatewayIn(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	require.NoError(t, a.db.Model(&gwmodels.Gateway{}).Where("id = ?", inactive.UUID).Update("is_active", false).Error)

	h := expect(t, a.do(http.MethodGet, "/gateways/"+active.UUID.String()+"/health", nil), http.StatusOK)
	require.Equal(t, "healthy", h["status"])
	require.Equal(t, active.UUID.String(), h["gatewayId"])
	require.NotEmpty(t, h["checkedAt"])
	h = expect(t, a.do(http.MethodGet, "/gateways/"+inactive.UUID.String()+"/health", nil), http.StatusOK)
	require.Equal(t, "unhealthy", h["status"])

	type status struct {
		Count int `json:"count"`
		List  []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			IsActive bool   `json:"isActive"`
		} `json:"list"`
	}
	all := expectInto[status](t, a.do(http.MethodGet, "/gateways/status", nil), http.StatusOK)
	require.Equal(t, 2, all.Count)
	byID := map[string]bool{}
	for _, s := range all.List {
		byID[s.ID] = s.IsActive
	}
	require.Equal(t, map[string]bool{active.UUID.String(): true, inactive.UUID.String(): false}, byID)

	one := expectInto[status](t, a.do(http.MethodGet, "/gateways/status?gatewayId="+inactive.UUID.String(), nil), http.StatusOK)
	require.Len(t, one.List, 1)
	require.Equal(t, inactive.Name, one.List[0].Name)
	require.False(t, one.List[0].IsActive)

	rec := a.do(http.MethodGet, "/gateways/status?gatewayId="+uuid.NewString(), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TestGWRoutesGatewayTokens rotates, lists and revokes a gateway's tokens, and caps the active ones.
func TestGWRoutesGatewayTokens(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress).UUID.String()
	base := "/gateways/" + gw + "/tokens"

	first := expectInto[gwspec.GatewayTokenResponse](t, a.do(http.MethodPost, base, nil), http.StatusCreated)
	require.Equal(t, gw, first.GatewayId)
	require.NotEmpty(t, first.Token)
	require.NotEmpty(t, first.TokenId)

	second := expectInto[gwspec.GatewayTokenResponse](t,
		a.do(http.MethodPost, base, map[string]any{"orgId": a.org}), http.StatusCreated)
	require.NotEqual(t, first.Token, second.Token)

	// Two active tokens is the cap.
	rec := a.do(http.MethodPost, base, nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())

	rec = a.do(http.MethodPost, base, map[string]any{"orgId": uuid.NewString()})
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
	require.Equal(t, "orgId does not match the caller's organization", errorMessage(t, rec))
	rec = a.do(http.MethodPost, base, "{bad")
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Equal(t, "Invalid request body", errorMessage(t, rec))

	list := expectInto[gwspec.GatewayTokenListResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.EqualValues(t, 2, list.Count)
	for _, tok := range list.List {
		require.Equal(t, "active", tok.Status)
		require.False(t, tok.RevokedAt.IsSet() && tok.RevokedAt.Get() != nil)
	}

	rec = a.do(http.MethodDelete, base+"/"+first.TokenId, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodDelete, base+"/"+first.TokenId, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, "revoking a revoked token is idempotent: %s", rec.Body.String())
	rec = a.do(http.MethodDelete, base+"/"+uuid.NewString(), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "Token not found", errorMessage(t, rec))

	// Another gateway's token is not this gateway's to revoke.
	otherGW := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress).UUID.String()
	rec = a.do(http.MethodDelete, "/gateways/"+otherGW+"/tokens/"+second.TokenId, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "Token not found", errorMessage(t, rec))

	// Only active tokens are listed.
	list = expectInto[gwspec.GatewayTokenListResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.EqualValues(t, 1, list.Count)
	require.Equal(t, second.TokenId, list.List[0].Id)

	// With one revoked, a new token can be rotated in.
	expect(t, a.do(http.MethodPost, base, nil), http.StatusCreated)
}
