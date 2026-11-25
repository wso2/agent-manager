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

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// gwSelfHostedBody is a valid self-hosted registration in the default environment.
func gwSelfHostedBody(handle string) map[string]any {
	return map[string]any{
		"displayName": "Self " + handle, "handle": handle, "environment": gwmodels.DefaultEnvironmentName,
		"endpoints": []string{"https://" + handle + ".example.com"},
	}
}

// gwRegisterSelfHosted registers a self-hosted gateway through the API.
func gwRegisterSelfHosted(t *testing.T, a *routeAPI, handle string) gwspec.GatewayResponse {
	t.Helper()
	return expectInto[gwspec.GatewayResponse](t,
		a.do(http.MethodPost, "/orgs/{org}/self-hosted-gateways", gwSelfHostedBody(handle)), http.StatusCreated)
}

// TestGWRoutesSelfHostedGatewayLifecycle registers, lists, reads, updates and deletes a self-hosted gateway.
func TestGWRoutesSelfHostedGatewayLifecycle(t *testing.T) {
	a := newRouteAPI(t)
	env := a.defaultEnv()
	managed := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleIngress)
	handle := unique("sh")

	body := gwSelfHostedBody(handle)
	body["description"] = "runs on prem"
	body["isCritical"] = true
	body["functionalityType"] = "AI"
	body["version"] = "1.2"
	body["properties"] = map[string]any{"region": "eu"}
	created := expectInto[gwspec.GatewayResponse](t, a.do(http.MethodPost, "/orgs/{org}/self-hosted-gateways", body), http.StatusCreated)
	require.Equal(t, handle, created.Name)
	require.Equal(t, gwspec.GATEWAYHOSTING_SELF_HOSTED, created.Hosting)
	require.Equal(t, gwspec.GATEWAYTYPE_EGRESS, created.GatewayType)
	require.Equal(t, "https://"+handle+".example.com", created.Vhost)
	require.True(t, created.IsCritical)
	require.Len(t, created.Environments, 1)
	require.Equal(t, env.String(), created.Environments[0].Id)

	// Only self-hosted gateways are listed.
	list := expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/orgs/{org}/self-hosted-gateways?limit=10", nil), http.StatusOK)
	require.EqualValues(t, 1, list.Total)
	require.EqualValues(t, 10, list.Limit)
	require.Equal(t, created.Uuid, list.Gateways[0].Uuid)
	require.Len(t, list.Gateways[0].Environments, 1)

	got := expectInto[gwspec.GatewayResponse](t, a.do(http.MethodGet, "/orgs/{org}/self-hosted-gateways/"+created.Uuid, nil), http.StatusOK)
	require.Equal(t, "Self "+handle, got.DisplayName)

	upd := expectInto[gwspec.GatewayResponse](t, a.do(http.MethodPut, "/orgs/{org}/self-hosted-gateways/"+created.Uuid, map[string]any{
		"displayName": "Renamed", "description": "moved", "isCritical": false, "properties": map[string]any{"region": "us"},
	}), http.StatusOK)
	require.Equal(t, "Renamed", upd.DisplayName)
	require.False(t, upd.IsCritical)
	require.Len(t, upd.Environments, 1)
	var stored gwmodels.Gateway
	require.NoError(t, a.db.Where("id = ?", created.Uuid).Take(&stored).Error)
	require.Equal(t, "moved", stored.Description)
	require.Equal(t, "us", stored.Properties["region"])

	// The self-hosted routes do not reach a managed gateway, nor an unknown one.
	for _, id := range []string{managed.UUID.String(), uuid.NewString(), "not-a-uuid"} {
		for _, tc := range []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, "/orgs/{org}/self-hosted-gateways/" + id, nil},
			{http.MethodPut, "/orgs/{org}/self-hosted-gateways/" + id, map[string]any{"displayName": "x"}},
			{http.MethodDelete, "/orgs/{org}/self-hosted-gateways/" + id, nil},
			{http.MethodGet, "/orgs/{org}/self-hosted-gateways/" + id + "/tokens", nil},
		} {
			rec := a.do(tc.method, tc.path, tc.body)
			require.Equal(t, http.StatusNotFound, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
			require.Equal(t, "Gateway not found", errorMessage(t, rec))
		}
	}
	got = expectInto[gwspec.GatewayResponse](t, a.do(http.MethodGet, "/orgs/{org}/gateways/"+managed.UUID.String(), nil), http.StatusOK)
	require.Equal(t, "Test gateway", got.DisplayName, "the managed gateway is untouched")

	// A self-hosted gateway stays in the environment it was registered for.
	rec := a.do(http.MethodDelete, "/orgs/{org}/gateways/"+created.Uuid+"/environments/default", nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	require.Equal(t, gwerr.ErrCodeGatewayEnvironmentFixed, gwErrCode(t, rec))
	expect(t, a.do(http.MethodPost, "/orgs/{org}/gateways/"+created.Uuid+"/environments/default", nil), http.StatusCreated)

	// It has no runtime URL.
	rec = a.do(http.MethodPut, "/orgs/{org}/gateways/"+created.Uuid, map[string]any{"runtimeUrl": "http://gw.amp.svc.cluster.local:8080"})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Equal(t, "A self-hosted gateway has no runtime URL", errorMessage(t, rec))

	rec = a.do(http.MethodPut, "/orgs/{org}/self-hosted-gateways/"+created.Uuid, "{bad")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "Invalid request body", errorMessage(t, rec))
	rec = a.do(http.MethodPut, "/orgs/{org}/self-hosted-gateways/"+created.Uuid, map[string]any{"displayName": " "})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	rec = a.do(http.MethodDelete, "/orgs/{org}/self-hosted-gateways/"+created.Uuid, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, "/orgs/{org}/self-hosted-gateways/"+created.Uuid, nil)
	require.Equal(t, http.StatusNotFound, rec.Code)
	list = expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/orgs/{org}/self-hosted-gateways", nil), http.StatusOK)
	require.EqualValues(t, 0, list.Total)
}

// TestGWRoutesRegisterSelfHostedRejections pins the status and message of each refused self-hosted registration.
func TestGWRoutesRegisterSelfHostedRejections(t *testing.T) {
	a := newRouteAPI(t)
	taken := unique("sh")
	gwRegisterSelfHosted(t, a, taken)

	with := func(k string, v any) map[string]any {
		b := gwSelfHostedBody(unique("sh"))
		b[k] = v
		return b
	}
	endpointMsg := "The gateway needs a base http or https URL, such as https://gateway.example.com, with no path, query or fragment"
	cases := []struct {
		name    string
		body    any
		status  int
		message string
	}{
		{"malformed body", "{bad", http.StatusBadRequest, "Invalid request body"},
		{"unknown environment", with("environment", "nowhere"), http.StatusNotFound, "Environment not found"},
		{"duplicate handle", gwSelfHostedBody(taken), http.StatusConflict, "A gateway with this name already exists"},
		{"non-AI type", with("functionalityType", "regular"), http.StatusBadRequest, "Self-hosted gateways support only the AI functionality type"},
		{"no endpoints", with("endpoints", []string{}), http.StatusBadRequest, endpointMsg},
		{"endpoint with path", with("endpoints", []string{"https://gw.example.com/api"}), http.StatusBadRequest, endpointMsg},
		{"endpoint with query", with("endpoints", []string{"https://gw.example.com?x=1"}), http.StatusBadRequest, endpointMsg},
		{"non-http endpoint", with("endpoints", []string{"ftp://gw.example.com"}), http.StatusBadRequest, endpointMsg},
		{"bad handle", with("handle", "Bad_Handle"), http.StatusBadRequest, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPost, "/orgs/{org}/self-hosted-gateways", tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			if tc.message != "" {
				require.Equal(t, tc.message, errorMessage(t, rec))
			}
		})
	}

	list := expectInto[gwspec.GatewayListResponse](t, a.do(http.MethodGet, "/orgs/{org}/self-hosted-gateways", nil), http.StatusOK)
	require.EqualValues(t, 1, list.Total, "nothing refused was stored")
}

// TestGWRoutesSelfHostedGatewayTokens rotates, lists and revokes a self-hosted gateway's tokens.
func TestGWRoutesSelfHostedGatewayTokens(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwRegisterSelfHosted(t, a, unique("sh"))
	base := "/orgs/{org}/self-hosted-gateways/" + gw.Uuid + "/tokens"

	// Rotation on the self-hosted route answers 201.
	tok := expectInto[gwspec.GatewayTokenResponse](t, a.do(http.MethodPost, base, nil), http.StatusCreated)
	require.Equal(t, gw.Uuid, tok.GatewayId)
	require.NotEmpty(t, tok.Token)

	list := expectInto[gwspec.GatewayTokenListResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.EqualValues(t, 1, list.Count)
	require.Equal(t, tok.TokenId, list.List[0].Id)

	rec := a.do(http.MethodDelete, base+"/"+tok.TokenId, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	list = expectInto[gwspec.GatewayTokenListResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.EqualValues(t, 0, list.Count)

	rec = a.do(http.MethodDelete, base+"/"+uuid.NewString(), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "Token not found", errorMessage(t, rec))

	// A managed gateway's tokens are not reachable from the self-hosted routes.
	managed := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress).UUID.String()
	rec = a.do(http.MethodPost, "/orgs/{org}/self-hosted-gateways/"+managed+"/tokens", nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	rec = a.do(http.MethodDelete, "/orgs/{org}/self-hosted-gateways/"+managed+"/tokens/"+uuid.NewString(), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())

	rec = a.do(http.MethodPost, base, nil, withScopes(scope(gwperm.Read)))
	require.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}
