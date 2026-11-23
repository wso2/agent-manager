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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// gwRegister registers a managed gateway through the API and returns the response.
func gwRegister(t *testing.T, a *routeAPI, body map[string]any) gwspec.GatewayResponse {
	t.Helper()
	return expectInto[gwspec.GatewayResponse](t, a.do(http.MethodPost, "/orgs/{org}/gateways", body), http.StatusCreated)
}

// gwBody is a valid registration body for a gateway named name.
func gwBody(name, gatewayType string) map[string]any {
	return map[string]any{"name": name, "displayName": "Gateway " + name, "gatewayType": gatewayType, "vhost": name + ".example.com"}
}

// TestGWRoutesGatewayLifecycle registers, reads, updates and deletes a gateway, reading each write back.
func TestGWRoutesGatewayLifecycle(t *testing.T) {
	a := newRouteAPI(t)
	env := a.defaultEnv()
	name := unique("gw")

	body := gwBody(name, "EGRESS")
	body["environmentIds"] = []string{env.String()}
	body["isCritical"] = true
	body["runtimeUrl"] = "http://gw.amp.svc.cluster.local:8080"
	body["orgId"] = a.org
	created := gwRegister(t, a, body)
	require.Equal(t, name, created.Name)
	require.Equal(t, gwspec.GATEWAYTYPE_EGRESS, created.GatewayType)
	require.Equal(t, gwspec.GATEWAYHOSTING_MANAGED, created.Hosting)
	require.Equal(t, name+".example.com", created.Vhost)
	require.True(t, created.IsCritical)
	require.NotNil(t, created.RuntimeUrl)
	require.Equal(t, "http://gw.amp.svc.cluster.local:8080", *created.RuntimeUrl)
	require.Len(t, created.Environments, 1)
	require.Equal(t, env.String(), created.Environments[0].Id)
	require.Equal(t, gwmodels.DefaultEnvironmentName, created.Environments[0].Name)

	// Read back by id and by name.
	for _, id := range []string{created.Uuid, name} {
		got := expectInto[gwspec.GatewayResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways/"+id, nil), http.StatusOK)
		require.Equal(t, created.Uuid, got.Uuid)
		require.Len(t, got.Environments, 1)
	}

	// Update display name, critical flag and runtime URL.
	upd := expectInto[gwspec.GatewayResponse](t, a.do(http.MethodPut, "/orgs/{org}/gateways/"+created.Uuid, map[string]any{
		"displayName": "Renamed", "isCritical": false, "runtimeUrl": "",
	}), http.StatusOK)
	require.Equal(t, "Renamed", upd.DisplayName)
	require.False(t, upd.IsCritical)
	require.Nil(t, upd.RuntimeUrl, "an empty runtime URL clears it")
	require.Len(t, upd.Environments, 1)

	got := expectInto[gwspec.GatewayResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways/"+created.Uuid, nil), http.StatusOK)
	require.Equal(t, "Renamed", got.DisplayName)
	require.False(t, got.IsCritical)

	// Delete, then the gateway is gone.
	rec := a.do(http.MethodDelete, "/orgs/{org}/gateways/"+created.Uuid, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, "/orgs/{org}/gateways/"+created.Uuid, nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	require.Equal(t, "Gateway not found", errorMessage(t, rec))
	rec = a.do(http.MethodDelete, "/orgs/{org}/gateways/"+created.Uuid, nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
}

// TestGWRoutesRegisterGatewayRejections pins the status and message of each refused registration.
func TestGWRoutesRegisterGatewayRejections(t *testing.T) {
	a := newRouteAPI(t)
	env := a.defaultEnv()
	taken := unique("gw")
	gwRegister(t, a, gwBody(taken, "EGRESS"))

	withEnvs := func(b map[string]any, envs ...string) map[string]any {
		b["environmentIds"] = envs
		return b
	}
	withField := func(b map[string]any, k string, v any) map[string]any {
		b[k] = v
		return b
	}

	cases := []struct {
		name    string
		body    any
		status  int
		message string
	}{
		{"malformed body", "{not json", http.StatusBadRequest, ""},
		{"duplicate name", gwBody(taken, "EGRESS"), http.StatusConflict, "A gateway with this name already exists"},
		{"short name", gwBody("ab", "EGRESS"), http.StatusBadRequest, ""},
		{"uppercase name", gwBody("Bad-Name", "EGRESS"), http.StatusBadRequest, ""},
		{"leading hyphen", gwBody("-bad-name", "EGRESS"), http.StatusBadRequest, ""},
		{"empty display name", withField(gwBody(unique("gw"), "EGRESS"), "displayName", "   "), http.StatusBadRequest, ""},
		{"empty vhost", withField(gwBody(unique("gw"), "EGRESS"), "vhost", " "), http.StatusBadRequest, ""},
		{"unknown environment", withEnvs(gwBody(unique("gw"), "EGRESS"), uuid.NewString()), http.StatusBadRequest, "environment validation failed"},
		{"foreign body org", withField(gwBody(unique("gw"), "EGRESS"), "orgId", uuid.NewString()), http.StatusForbidden, "orgId does not match the caller's organization"},
		{"public runtime URL", withField(gwBody(unique("gw"), "EGRESS"), "runtimeUrl", "https://example.com"), http.StatusBadRequest, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPost, "/orgs/{org}/gateways", tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			if tc.message != "" {
				require.Equal(t, tc.message, errorMessage(t, rec))
			} else {
				require.NotEmpty(t, errorMessage(t, rec))
			}
		})
	}

	// Two environments is refused: a gateway serves exactly one.
	other := uuid.Must(uuid.NewV7())
	dbtest.EnvironmentWithID(t, a.db, a.org, other)
	rec := a.do(http.MethodPost, "/orgs/{org}/gateways", withEnvs(gwBody(unique("gw"), "EGRESS"), env.String(), other.String()))
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Contains(t, errorMessage(t, rec), "exactly one environment")

	// A second ingress gateway in the same environment exceeds the cap.
	gwRegister(t, a, withEnvs(gwBody(unique("gw"), "INGRESS"), env.String()))
	rec = a.do(http.MethodPost, "/orgs/{org}/gateways", withEnvs(gwBody(unique("gw"), "BOTH"), env.String()))
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Contains(t, errorMessage(t, rec), "already has an ingress gateway")

	// Nothing refused was stored.
	list := expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways", nil), http.StatusOK)
	require.EqualValues(t, 2, list.Total)
}

// TestGWRoutesGatewayNotFoundAndBadInput pins the 404s and 400s of the single-gateway routes.
func TestGWRoutesGatewayNotFoundAndBadInput(t *testing.T) {
	a := newRouteAPI(t)
	missing := uuid.NewString()

	for _, tc := range []struct {
		method, path string
		body         any
	}{
		{http.MethodGet, "/orgs/{org}/gateways/" + missing, nil},
		{http.MethodGet, "/orgs/{org}/gateways/no-such-gateway", nil},
		{http.MethodPut, "/orgs/{org}/gateways/" + missing, map[string]any{"displayName": "x"}},
		{http.MethodDelete, "/orgs/{org}/gateways/" + missing, nil},
		{http.MethodGet, "/orgs/{org}/gateways/" + missing + "/health", nil},
		{http.MethodGet, "/orgs/{org}/gateways/" + missing + "/tokens", nil},
		{http.MethodPost, "/orgs/{org}/gateways/" + missing + "/tokens", nil},
		{http.MethodDelete, "/orgs/{org}/gateways/" + missing + "/tokens/" + uuid.NewString(), nil},
		{http.MethodPost, "/orgs/{org}/gateways/" + missing + "/environments/default", nil},
		{http.MethodDelete, "/orgs/{org}/gateways/" + missing + "/environments/default", nil},
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
	rec := a.do(http.MethodGet, "/orgs/{org}/gateways/"+foreign.UUID.String(), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	rec = a.do(http.MethodPut, "/orgs/{org}/gateways/"+gw.UUID.String(), "{bad")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "Invalid request body", errorMessage(t, rec))

	rec = a.do(http.MethodPut, "/orgs/{org}/gateways/"+gw.UUID.String(), map[string]any{"displayName": "  "})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

// TestGWRoutesGatewayScopes refuses each gateway route to a token without its permission.
func TestGWRoutesGatewayScopes(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress).UUID.String()
	readOnly := withScopes(scope(gwperm.Read))

	for _, tc := range []struct {
		method, path string
		body         any
		opt          reqOpt
	}{
		{http.MethodPost, "/orgs/{org}/gateways", gwBody(unique("gw"), "EGRESS"), readOnly},
		{http.MethodGet, "/orgs/{org}/gateways/" + gw, nil, withScopes(scope(gwperm.Update))},
		{http.MethodPut, "/orgs/{org}/gateways/" + gw, map[string]any{"displayName": "x"}, readOnly},
		{http.MethodDelete, "/orgs/{org}/gateways/" + gw, nil, readOnly},
		{http.MethodPost, "/orgs/{org}/gateways/" + gw + "/environments/default", nil, readOnly},
		{http.MethodDelete, "/orgs/{org}/gateways/" + gw + "/environments/default", nil, readOnly},
		{http.MethodGet, "/orgs/{org}/gateways/" + gw + "/tokens", nil, readOnly},
		{http.MethodPost, "/orgs/{org}/gateways/" + gw + "/tokens", nil, readOnly},
		{http.MethodGet, "/orgs/{org}/gateways/status", nil, withScopes(scope(gwperm.Update))},
		{http.MethodGet, "/orgs/{org}/self-hosted-gateways", nil, withScopes(scope(gwperm.Update))},
		{http.MethodPost, "/orgs/{org}/self-hosted-gateways", map[string]any{}, readOnly},
		{http.MethodGet, "/orgs/{org}/identity-providers", nil, withScopes(scope(gwperm.Update))},
		{http.MethodGet, "/orgs/{org}/identity-providers/discover?url=https://example.com", nil, readOnly},
		{http.MethodPut, "/orgs/{org}/gateways/" + gw + "/identity-providers/idp", map[string]any{}, readOnly},
		{http.MethodGet, "/orgs/{org}/catalog", nil, readOnly},
	} {
		t.Run(tc.method+" "+tc.path, func(t *testing.T) {
			rec := a.do(tc.method, tc.path, tc.body, tc.opt)
			require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
		})
	}

	// The gateway is untouched by the refused writes.
	got := expectInto[gwspec.GatewayResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways/"+gw, nil), http.StatusOK)
	require.Equal(t, "Test gateway", got.DisplayName)
}

// TestGWRoutesListGatewaysFiltersAndPages filters by type, status and environment, and pages the result.
func TestGWRoutesListGatewaysFiltersAndPages(t *testing.T) {
	a := newRouteAPI(t)
	env := a.defaultEnv()

	ingress := gwtest.GatewayIn(t, a.db, a.org, gwmodels.GatewayRoleIngress, env)
	egress := gwtest.GatewayIn(t, a.db, a.org, gwmodels.GatewayRoleEgress, env)
	inactive := gwtest.GatewayIn(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	require.NoError(t, a.db.Model(&gwmodels.Gateway{}).Where("id = ?", inactive.UUID).Update("is_active", false).Error)

	ids := func(q string) []string {
		t.Helper()
		list := expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways"+q, nil), http.StatusOK)
		out := make([]string, 0, len(list.Gateways))
		for _, g := range list.Gateways {
			out = append(out, g.Uuid)
		}
		return out
	}

	require.ElementsMatch(t, []string{ingress.UUID.String(), egress.UUID.String(), inactive.UUID.String()}, ids(""))
	require.ElementsMatch(t, []string{ingress.UUID.String()}, ids("?type=INGRESS"))
	require.ElementsMatch(t, []string{egress.UUID.String(), inactive.UUID.String()}, ids("?type=EGRESS"))
	require.ElementsMatch(t, []string{inactive.UUID.String()}, ids("?status=INACTIVE"))
	require.ElementsMatch(t, []string{ingress.UUID.String(), egress.UUID.String()}, ids("?status=ACTIVE"))
	// Any environment a request names is the organization's only one, so the
	// filter drops the unmapped gateway whatever it is given.
	require.ElementsMatch(t, []string{ingress.UUID.String(), egress.UUID.String()}, ids("?environment=default"))
	require.ElementsMatch(t, []string{ingress.UUID.String(), egress.UUID.String()}, ids("?environment=anything"))

	// A listed gateway carries its environment; an unmapped one carries none.
	list := expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways", nil), http.StatusOK)
	for _, g := range list.Gateways {
		if g.Uuid == inactive.UUID.String() {
			require.Empty(t, g.Environments)
			require.Equal(t, gwspec.GatewayStatus("INACTIVE"), g.Status)
			continue
		}
		require.Len(t, g.Environments, 1)
		require.Equal(t, env.String(), g.Environments[0].Id)
		require.Equal(t, gwspec.GatewayStatus("ACTIVE"), g.Status)
	}

	page := expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways?limit=2&offset=1", nil), http.StatusOK)
	require.EqualValues(t, 3, page.Total)
	require.EqualValues(t, 2, page.Limit)
	require.EqualValues(t, 1, page.Offset)
	require.Len(t, page.Gateways, 2)

	// Negative values fall back to the defaults; an oversized page is capped.
	page = expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways?limit=-1&offset=-5", nil), http.StatusOK)
	require.EqualValues(t, 100, page.Limit)
	require.EqualValues(t, 0, page.Offset)
	page = expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways?limit=5000", nil), http.StatusOK)
	require.EqualValues(t, 1000, page.Limit)
	page = expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways?limit=abc", nil), http.StatusOK)
	require.EqualValues(t, 100, page.Limit)
}

// TestGWRoutesGatewayEnvironmentMapping maps a gateway to the environment, reads it back, and unmaps it.
func TestGWRoutesGatewayEnvironmentMapping(t *testing.T) {
	a := newRouteAPI(t)
	env := a.defaultEnv()
	created := gwRegister(t, a, gwBody(unique("gw"), "EGRESS"))
	require.Empty(t, created.Environments)

	envs := func(gw string) gwspec.GetGatewayEnvironments200Response {
		t.Helper()
		return expectInto[gwspec.GetGatewayEnvironments200Response](t,
			a.do(http.MethodGet, "/orgs/{org}/gateways/"+gw+"/environments", nil), http.StatusOK)
	}
	require.Empty(t, envs(created.Uuid).Environments)

	// Unmapping an environment the gateway is not in is a 404.
	rec := a.do(http.MethodDelete, "/orgs/{org}/gateways/"+created.Uuid+"/environments/default", nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "Gateway-environment mapping not found", errorMessage(t, rec))

	// Assign; re-assigning is idempotent.
	out := expect(t, a.do(http.MethodPost, "/orgs/{org}/gateways/"+created.Uuid+"/environments/default", nil), http.StatusCreated)
	require.Equal(t, "Gateway assigned successfully", out["message"])
	expect(t, a.do(http.MethodPost, "/orgs/{org}/gateways/"+created.Uuid+"/environments/"+env.String(), nil), http.StatusCreated)

	got := envs(created.Uuid)
	require.Len(t, got.Environments, 1)
	require.Equal(t, env.String(), got.Environments[0].Id)
	require.Equal(t, gwmodels.DefaultEnvironmentName, got.Environments[0].Name)
	byName := expectInto[gwspec.GatewayResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways/"+created.Name, nil), http.StatusOK)
	require.Len(t, byName.Environments, 1, "a lookup by name finds the mapping too")

	// Assigning by gateway name is a bad request: mapping needs the gateway's id.
	rec = a.do(http.MethodPost, "/orgs/{org}/gateways/"+created.Name+"/environments/default", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	// A second ingress-capable gateway is refused by the environment's ingress cap.
	gwRegister(t, a, gwBody(unique("gw"), "INGRESS"))
	second := gwRegister(t, a, gwBody(unique("gw"), "BOTH"))
	expect(t, a.do(http.MethodPost, "/orgs/{org}/gateways/"+second.Uuid+"/environments/default", nil), http.StatusCreated)
	third := gwRegister(t, a, gwBody(unique("gw"), "INGRESS"))
	rec = a.do(http.MethodPost, "/orgs/{org}/gateways/"+third.Uuid+"/environments/default", nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Contains(t, errorMessage(t, rec), "already has an ingress gateway")
	require.Empty(t, envs(third.Uuid).Environments)

	// A gateway already in another environment is refused a new one.
	other := uuid.Must(uuid.NewV7())
	elsewhere := gwtest.GatewayIn(t, a.db, a.org, gwmodels.GatewayRoleEgress, other)
	rec = a.do(http.MethodPost, "/orgs/{org}/gateways/"+elsewhere.UUID.String()+"/environments/default", nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, gwerr.ErrCodeGatewayEnvironmentFixed, gwErrCode(t, rec))

	rec = a.do(http.MethodDelete, "/orgs/{org}/gateways/"+created.Uuid+"/environments/default", nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	require.Empty(t, envs(created.Uuid).Environments)

	// An unknown gateway id lists no environments.
	require.Empty(t, envs(uuid.NewString()).Environments)
}

// gwErrCode is the code of an error response.
func gwErrCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `json:"code"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "body: %s", rec.Body.String())
	return body.Code
}

// TestGWRoutesGatewayHealthAndStatus reports a gateway's health and the organization's gateway statuses.
func TestGWRoutesGatewayHealthAndStatus(t *testing.T) {
	a := newRouteAPI(t)
	active := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	inactive := gwtest.GatewayIn(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	require.NoError(t, a.db.Model(&gwmodels.Gateway{}).Where("id = ?", inactive.UUID).Update("is_active", false).Error)

	h := expect(t, a.do(http.MethodGet, "/orgs/{org}/gateways/"+active.UUID.String()+"/health", nil), http.StatusOK)
	require.Equal(t, "healthy", h["status"])
	require.Equal(t, active.UUID.String(), h["gatewayId"])
	require.NotEmpty(t, h["checkedAt"])
	h = expect(t, a.do(http.MethodGet, "/orgs/{org}/gateways/"+inactive.UUID.String()+"/health", nil), http.StatusOK)
	require.Equal(t, "unhealthy", h["status"])

	type status struct {
		Count int `json:"count"`
		List  []struct {
			ID       string `json:"id"`
			Name     string `json:"name"`
			IsActive bool   `json:"isActive"`
		} `json:"list"`
	}
	all := expectInto[status](t, a.do(http.MethodGet, "/orgs/{org}/gateways/status", nil), http.StatusOK)
	require.Equal(t, 2, all.Count)
	byID := map[string]bool{}
	for _, s := range all.List {
		byID[s.ID] = s.IsActive
	}
	require.Equal(t, map[string]bool{active.UUID.String(): true, inactive.UUID.String(): false}, byID)

	one := expectInto[status](t, a.do(http.MethodGet, "/orgs/{org}/gateways/status?gatewayId="+inactive.UUID.String(), nil), http.StatusOK)
	require.Len(t, one.List, 1)
	require.Equal(t, inactive.Name, one.List[0].Name)
	require.False(t, one.List[0].IsActive)

	rec := a.do(http.MethodGet, "/orgs/{org}/gateways/status?gatewayId="+uuid.NewString(), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TestGWRoutesGatewayTokens rotates, lists and revokes a gateway's tokens, and caps the active ones.
func TestGWRoutesGatewayTokens(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress).UUID.String()
	base := "/orgs/{org}/gateways/" + gw + "/tokens"

	first := expectInto[gwspec.GatewayTokenResponse](t, a.do(http.MethodPost, base, nil), http.StatusOK)
	require.Equal(t, gw, first.GatewayId)
	require.NotEmpty(t, first.Token)
	require.NotEmpty(t, first.TokenId)

	second := expectInto[gwspec.GatewayTokenResponse](t,
		a.do(http.MethodPost, base, map[string]any{"orgId": a.org}), http.StatusOK)
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
	rec = a.do(http.MethodDelete, "/orgs/{org}/gateways/"+otherGW+"/tokens/"+second.TokenId, nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "Token not found", errorMessage(t, rec))

	// Only active tokens are listed.
	list = expectInto[gwspec.GatewayTokenListResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.EqualValues(t, 1, list.Count)
	require.Equal(t, second.TokenId, list.List[0].Id)

	// With one revoked, a new token can be rotated in.
	expect(t, a.do(http.MethodPost, base, nil), http.StatusOK)
}
