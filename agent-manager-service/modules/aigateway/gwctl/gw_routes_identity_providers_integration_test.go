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
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// gwIdPBody is an upsert request for an identity provider at issuer.
func gwIdPBody(issuer string) map[string]any {
	return map[string]any{
		"issuer": issuer, "jwksUri": issuer + "/jwks", "skipTlsVerify": true, "description": "idp at " + issuer,
	}
}

// gwIdPNames is the names of a listed set of identity providers.
func gwIdPNames(list gwspec.IdentityProviderListResponse) []string {
	out := make([]string, 0, len(list.List))
	for _, p := range list.List {
		out = append(out, p.Name)
	}
	return out
}

// TestGWRoutesGatewayIdentityProviders upserts, lists and deletes a gateway's identity providers.
func TestGWRoutesGatewayIdentityProviders(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleIngress)
	base := "/orgs/{org}/gateways/" + gw.UUID.String() + "/identity-providers"

	p := expectInto[gwspec.IdentityProvider](t, a.do(http.MethodPut, base+"/corp-idp", gwIdPBody("https://idp.example.com")), http.StatusOK)
	require.Equal(t, "corp-idp", p.Name)
	require.Equal(t, "https://idp.example.com", *p.Issuer)
	require.Equal(t, "https://idp.example.com/jwks", *p.JwksUri)
	require.True(t, *p.SkipTlsVerify)
	require.Equal(t, gwmodels.IdentityProviderTypeCustom, *p.Type)
	require.Equal(t, "idp at https://idp.example.com", *p.Description)

	// Upserting again replaces it; the gateway can be named instead of its id.
	p = expectInto[gwspec.IdentityProvider](t, a.do(http.MethodPut, "/orgs/{org}/gateways/"+gw.Name+"/identity-providers/corp-idp",
		map[string]any{"issuer": "https://idp2.example.com", "jwksUri": "https://idp2.example.com/keys"}), http.StatusOK)
	require.Equal(t, "https://idp2.example.com", *p.Issuer)
	require.False(t, *p.SkipTlsVerify)
	require.Nil(t, p.Description)

	// A well-known name is a system provider.
	sys := expectInto[gwspec.IdentityProvider](t, a.do(http.MethodPut, base+"/ThunderKeyManager", gwIdPBody("https://thunder.example.com")), http.StatusOK)
	require.Equal(t, gwmodels.IdentityProviderTypeSystem, *sys.Type)

	list := expectInto[gwspec.IdentityProviderListResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.EqualValues(t, 2, list.Count)
	require.ElementsMatch(t, []string{"corp-idp", "ThunderKeyManager"}, gwIdPNames(list))
	for _, item := range list.List {
		if item.Name == "corp-idp" {
			require.Equal(t, "https://idp2.example.com", *item.Issuer, "the upsert replaced the provider")
		}
	}

	// The organization-wide list names the gateway and the environment.
	org := expectInto[gwspec.IdentityProviderListResponse](t, a.do(http.MethodGet, "/orgs/{org}/identity-providers", nil), http.StatusOK)
	require.EqualValues(t, 2, org.Count)
	for _, item := range org.List {
		require.Equal(t, gw.UUID.String(), *item.GatewayId)
		require.Equal(t, gw.Name, *item.GatewayName)
		require.NotNil(t, item.EnvironmentName)
		require.Equal(t, gwmodels.DefaultEnvironmentName, *item.EnvironmentName)
	}

	// The environment's providers are those of its gateways.
	env := expectInto[gwspec.IdentityProviderListResponse](t,
		a.do(http.MethodGet, "/orgs/{org}/environments/default/identity-providers", nil), http.StatusOK)
	require.ElementsMatch(t, []string{"corp-idp", "ThunderKeyManager"}, gwIdPNames(env))

	// A system provider cannot be deleted.
	rec := a.do(http.MethodDelete, base+"/ThunderKeyManager", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Equal(t, "Invalid input", errorMessage(t, rec))

	rec = a.do(http.MethodDelete, base+"/corp-idp", nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	list = expectInto[gwspec.IdentityProviderListResponse](t, a.do(http.MethodGet, base, nil), http.StatusOK)
	require.Equal(t, []string{"ThunderKeyManager"}, gwIdPNames(list))

	// Deleting it again finds nothing.
	rec = a.do(http.MethodDelete, base+"/corp-idp", nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TestGWRoutesIdentityProviderRejections pins the refused identity provider requests.
func TestGWRoutesIdentityProviderRejections(t *testing.T) {
	a := newRouteAPI(t)
	gw := gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleIngress)
	base := "/orgs/{org}/gateways/" + gw.UUID.String() + "/identity-providers"

	rec := a.do(http.MethodPut, base+"/corp-idp", "{bad")
	require.Equal(t, http.StatusBadRequest, rec.Code)
	require.Equal(t, "Invalid request body", errorMessage(t, rec))

	// The internal provider is reserved.
	rec = a.do(http.MethodPut, base+"/"+gwmodels.ReservedIdentityProviderName, gwIdPBody("https://idp.example.com"))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Equal(t, "Invalid input", errorMessage(t, rec))
	rec = a.do(http.MethodDelete, base+"/"+gwmodels.ReservedIdentityProviderName, nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

	// Unknown, foreign and self-hosted gateways are all not found.
	other := newRouteAPI(t)
	foreign := gwtest.Gateway(t, other.db, other.org, gwmodels.GatewayRoleIngress)
	self := gwRegisterSelfHosted(t, a, unique("sh"))
	for _, id := range []string{uuid.NewString(), "no-such-gateway", foreign.UUID.String(), self.Uuid} {
		path := "/orgs/{org}/gateways/" + id + "/identity-providers"
		rec = a.do(http.MethodPut, path+"/corp-idp", gwIdPBody("https://idp.example.com"))
		require.Equal(t, http.StatusNotFound, rec.Code, "%s: %s", id, rec.Body.String())
		require.Equal(t, "Gateway not found", errorMessage(t, rec))
		rec = a.do(http.MethodDelete, path+"/corp-idp", nil)
		require.Equal(t, http.StatusNotFound, rec.Code, "%s: %s", id, rec.Body.String())
		if id != self.Uuid {
			rec = a.do(http.MethodGet, path, nil)
			require.Equal(t, http.StatusNotFound, rec.Code, "%s: %s", id, rec.Body.String())
		}
	}

	// The self-hosted gateway's providers can be listed, though not written.
	list := expectInto[gwspec.IdentityProviderListResponse](t,
		a.do(http.MethodGet, "/orgs/{org}/gateways/"+self.Uuid+"/identity-providers", nil), http.StatusOK)
	require.EqualValues(t, 0, list.Count)

	// An organization with no providers lists none.
	org := expectInto[gwspec.IdentityProviderListResponse](t, a.do(http.MethodGet, "/orgs/{org}/identity-providers", nil), http.StatusOK)
	require.EqualValues(t, 0, org.Count)
	require.Empty(t, org.List)
}

// TestGWRoutesDiscoverOidcConfiguration refuses a missing or unsafe discovery URL without fetching it.
func TestGWRoutesDiscoverOidcConfiguration(t *testing.T) {
	a := newRouteAPI(t)

	rec := a.do(http.MethodGet, "/orgs/{org}/identity-providers/discover", nil)
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Equal(t, "Query parameter 'url' is required", errorMessage(t, rec))

	for _, u := range []string{"not-a-url", "http://127.0.0.1:9000", "http://169.254.169.254/latest", "http://localhost/realms/x"} {
		rec = a.do(http.MethodGet, "/orgs/{org}/identity-providers/discover?url="+u, nil)
		require.Equal(t, http.StatusBadRequest, rec.Code, "%s: %s", u, rec.Body.String())
		require.Equal(t, "Could not discover OIDC configuration from the provided URL", errorMessage(t, rec))
	}
}
