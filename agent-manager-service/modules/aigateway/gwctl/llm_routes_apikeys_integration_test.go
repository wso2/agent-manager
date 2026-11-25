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

	"github.com/wso2/agent-manager/agent-manager-service/db/dbtest"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// llmKeyNames is the names of the keys a list response holds.
func llmKeyNames(t *testing.T, list map[string]any) []string {
	t.Helper()
	keys, _ := list["keys"].([]any)
	names := make([]string, 0, len(keys))
	for _, k := range keys {
		names = append(names, k.(map[string]any)["name"].(string))
	}
	return names
}

// llmAPIKeyLifecycle issues, lists, rotates and revokes a key under base, the owner's api-keys collection.
func llmAPIKeyLifecycle(t *testing.T, a *routeAPI, base string) {
	t.Helper()
	require.Empty(t, llmKeyNames(t, expect(t, a.do(http.MethodGet, base, nil), http.StatusOK)))

	created := expect(t, a.do(http.MethodPost, base, map[string]any{"displayName": "CI key"}), http.StatusCreated)
	require.NotEmpty(t, created["apiKey"], "the key value is returned once")
	require.NotEmpty(t, created["keyId"])

	names := llmKeyNames(t, expect(t, a.do(http.MethodGet, base, nil), http.StatusOK))
	require.Len(t, names, 1)
	name := names[0]

	rotated := expect(t, a.do(http.MethodPut, base+"/"+name, map[string]any{"displayName": "CI key 2"}), http.StatusOK)
	require.NotEmpty(t, rotated["apiKey"])
	require.NotEqual(t, created["apiKey"], rotated["apiKey"], "a rotation issues a new value")

	rec := a.do(http.MethodDelete, base+"/"+name, nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	list := expect(t, a.do(http.MethodGet, base, nil), http.StatusOK)
	for _, k := range list["keys"].([]any) {
		require.NotEqual(t, "active", k.(map[string]any)["status"], "the revoked key is no longer active")
	}
}

// A provider's API keys are issued, listed, rotated and revoked.
func TestLLMRoutesProviderAPIKeyLifecycle(t *testing.T) {
	a := newRouteAPI(t)
	gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	uid := *llmCreateProvider(t, a, unique("keys")).Metadata.Uid
	llmAPIKeyLifecycle(t, a, "/orgs/{org}/llm-providers/"+uid+"/api-keys")
}

// A proxy's API keys are issued, listed, rotated and revoked.
func TestLLMRoutesProxyAPIKeyLifecycle(t *testing.T) {
	a := newRouteAPI(t)
	gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	project := unique("proj")
	dbtest.EnsureProject(t, a.db, a.org, project)
	providerUID := *llmCreateProvider(t, a, unique("prov")).Metadata.Uid
	proxy := llmCreateProxy(t, a, project, providerUID, unique("px"))
	llmAPIKeyLifecycle(t, a, llmProxiesPath(project)+"/"+*proxy.Configuration.Name+"/api-keys")
}

// API key routes refuse a bad body or name, an unknown owner, and an organization with no gateway.
func TestLLMRoutesAPIKeyErrors(t *testing.T) {
	a := newRouteAPI(t)
	project := unique("proj")
	dbtest.EnsureProject(t, a.db, a.org, project)
	providerHandle := unique("prov")
	providerUID := *llmCreateProvider(t, a, providerHandle).Metadata.Uid
	proxy := llmCreateProxy(t, a, project, providerUID, unique("px"))
	providerKeys := "/orgs/{org}/llm-providers/" + providerUID + "/api-keys"
	proxyKeys := llmProxiesPath(project) + "/" + *proxy.Configuration.Name + "/api-keys"

	type keyCase struct {
		name    string
		method  string
		path    string
		body    any
		status  int
		message string
	}
	tests := []keyCase{
		{"provider create malformed body", http.MethodPost, providerKeys, "{", http.StatusBadRequest, "Invalid request body"},
		{
			"provider create without a name", http.MethodPost, providerKeys,
			map[string]any{},
			http.StatusBadRequest,
			"At least one of 'name' or 'displayName' must be provided",
		},
		{
			"provider create with no gateway", http.MethodPost, providerKeys,
			map[string]any{"name": "ci-key"},
			http.StatusServiceUnavailable,
			"No gateway connections available",
		},
		{
			"provider revoke with no gateway", http.MethodDelete, providerKeys + "/ci-key", nil, http.StatusServiceUnavailable,
			"No gateway connections available",
		},
		{
			"provider rotate with no gateway", http.MethodPut, providerKeys + "/ci-key", nil, http.StatusServiceUnavailable,
			"No gateway connections available",
		},
		{"proxy create malformed body", http.MethodPost, proxyKeys, "{", http.StatusBadRequest, "Invalid request body"},
		{
			"proxy create without a name", http.MethodPost, proxyKeys,
			map[string]any{"name": ""},
			http.StatusBadRequest,
			"At least one of 'name' or 'displayName' must be provided",
		},
		{
			"proxy create with no gateway", http.MethodPost, proxyKeys,
			map[string]any{"name": "ci-key"},
			http.StatusServiceUnavailable,
			"No gateway connections available",
		},
		{
			"proxy revoke with no gateway", http.MethodDelete, proxyKeys + "/ci-key", nil, http.StatusServiceUnavailable,
			"No gateway connections available",
		},
		{
			"proxy rotate with no gateway", http.MethodPut, proxyKeys + "/ci-key", nil, http.StatusServiceUnavailable,
			"No gateway connections available",
		},
		{
			"proxy list in an unknown project", http.MethodGet, llmProxiesPath(unique("noproj")) + "/" + *proxy.Configuration.Name + "/api-keys", nil,
			http.StatusNotFound, "Project not found",
		},
	}
	// An unknown provider is a 404 whether it is named by UUID or by handle.
	for _, id := range []string{uuid.NewString(), unique("missing")} {
		keys := "/orgs/{org}/llm-providers/" + id + "/api-keys"
		tests = append(tests,
			keyCase{"list on unknown provider " + id, http.MethodGet, keys, nil, http.StatusNotFound, "LLM provider not found"},
			keyCase{"create on unknown provider " + id, http.MethodPost, keys, map[string]any{"name": "ci-key"}, http.StatusNotFound, "LLM provider not found"},
			keyCase{"revoke on unknown provider " + id, http.MethodDelete, keys + "/ci-key", nil, http.StatusNotFound, "LLM provider not found"},
			keyCase{"rotate on unknown provider " + id, http.MethodPut, keys + "/ci-key", nil, http.StatusNotFound, "LLM provider not found"},
		)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := a.do(tt.method, tt.path, tt.body)
			require.Equal(t, tt.status, rec.Code, rec.Body.String())
			require.Equal(t, tt.message, errorMessage(t, rec))
		})
	}

	// The provider's handle names it as well as its UUID.
	rec := a.do(http.MethodGet, "/orgs/{org}/llm-providers/"+providerHandle+"/api-keys", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleEgress)
	rec = a.do(http.MethodPost, providerKeys, map[string]any{"name": "ab"})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, errorMessage(t, rec), "must be at least")
	rec = a.do(http.MethodPost, proxyKeys, map[string]any{"name": "ab"})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	require.Contains(t, errorMessage(t, rec), "must be at least")

	// Rotating replaces an existing key; it does not create one.
	for _, keys := range []string{providerKeys, proxyKeys} {
		rec = a.do(http.MethodPut, keys+"/never-created", nil)
		require.Equal(t, http.StatusNotFound, rec.Code, "%s: %s", keys, rec.Body.String())
		require.Equal(t, "API key not found", errorMessage(t, rec))
		list := expect(t, a.do(http.MethodGet, keys, nil), http.StatusOK)
		require.NotContains(t, llmKeyNames(t, list), "never-created")
	}
}

// API key routes need the API-key permission of their owner.
func TestLLMRoutesAPIKeyNeedsScope(t *testing.T) {
	a := newRouteAPI(t)
	providerKeys := "/orgs/{org}/llm-providers/x/api-keys"
	proxyKeys := llmProxiesPath("p") + "/x/api-keys"
	for _, tc := range []struct {
		method, path string
		opt          reqOpt
	}{
		{http.MethodGet, providerKeys, withScopes(scope(gwperm.LLMProxyAPIKeyManage))},
		{http.MethodPost, providerKeys, withScopes(scope(gwperm.LLMProviderRead))},
		{http.MethodPut, providerKeys + "/k", withScopes(scope(gwperm.LLMProviderRead))},
		{http.MethodDelete, providerKeys + "/k", withScopes(scope(gwperm.LLMProviderRead))},
		{http.MethodGet, proxyKeys, withScopes(scope(gwperm.LLMProviderAPIKeyManage))},
		{http.MethodPost, proxyKeys, withScopes(scope(gwperm.LLMProxyRead))},
		{http.MethodPut, proxyKeys + "/k", withScopes(scope(gwperm.LLMProxyRead))},
		{http.MethodDelete, proxyKeys + "/k", withScopes(scope(gwperm.LLMProxyRead))},
	} {
		rec := a.do(tc.method, tc.path, nil, tc.opt)
		require.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", tc.method, tc.path, rec.Body.String())
	}
}

// Listing the keys of an unknown proxy is a 404.
func TestLLMRoutesAPIKeyUnknownProxy(t *testing.T) {
	a := newRouteAPI(t)
	project := unique("proj")
	dbtest.EnsureProject(t, a.db, a.org, project)

	rec := a.do(http.MethodGet, llmProxiesPath(project)+"/"+unique("missing")+"/api-keys", nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	require.Equal(t, "LLM proxy not found", errorMessage(t, rec))
}
