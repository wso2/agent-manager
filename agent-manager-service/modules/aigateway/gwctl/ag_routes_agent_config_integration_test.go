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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// agModelConfigBody is a create request for an LLM configuration on provider
// in the default environment.
func agModelConfigBody(name, provider string) map[string]any {
	return map[string]any{
		"name": name, "type": "llm", "description": "config " + name,
		"envMappings": map[string]any{"default": map[string]any{"providerName": provider, "configuration": map[string]any{}}},
	}
}

// agMCPConfigBody is a create request for an MCP configuration on proxy in the
// default environment.
func agMCPConfigBody(name, proxy string) map[string]any {
	return map[string]any{
		"name":        name,
		"envMappings": map[string]any{"default": map[string]any{"mcpProxyName": proxy, "configuration": map[string]any{}}},
	}
}

// agDefaultMapping is the default environment's mapping in a configuration response.
func agDefaultMapping(t *testing.T, cfg map[string]any) map[string]any {
	t.Helper()
	mappings, ok := cfg["envMappings"].(map[string]any)
	require.True(t, ok, "envMappings: %v", cfg)
	m, ok := mappings["default"].(map[string]any)
	require.True(t, ok, "default mapping: %v", mappings)
	return m["configuration"].(map[string]any)
}

// TestAGRoutesModelConfigLifecycle creates an external agent's LLM
// configuration, reads it back, renames it and deletes it.
func TestAGRoutesModelConfigLifecycle(t *testing.T) {
	a, gw := agSetup(t)
	providerUID := agCreateProvider(t, a, "agprov", gw)

	created := expect(t, a.do(http.MethodPost, agAgentPath("/model-configs"), agModelConfigBody("chat", "agprov")), http.StatusCreated)
	id, _ := created["uuid"].(string)
	require.NotEmpty(t, id)
	assert.Equal(t, "chat", created["name"])
	assert.Equal(t, "llm", created["type"])
	assert.Equal(t, agAgent, created["agentId"])
	mapping := agDefaultMapping(t, created)
	assert.Equal(t, "agprov", mapping["providerName"])
	assert.Equal(t, providerUID, mapping["providerUuid"])
	assert.Contains(t, mapping["url"], "https://gw.example.com/", "the external agent is handed the gateway's public URL")
	assert.Len(t, created["environmentVariables"], 2)

	got := expect(t, a.do(http.MethodGet, agAgentPath("/model-configs/"+id), nil), http.StatusOK)
	assert.Equal(t, "config chat", got["description"])
	assert.Equal(t, "agprov", agDefaultMapping(t, got)["providerName"])

	list := expect(t, a.do(http.MethodGet, agAgentPath("/model-configs?limit=0&offset=-5"), nil), http.StatusOK)
	configs := list["configs"].([]any)
	require.Len(t, configs, 1)
	assert.Equal(t, id, configs[0].(map[string]any)["uuid"])
	assert.EqualValues(t, 20, list["pagination"].(map[string]any)["limit"], "an out-of-range limit falls back to the default")
	capped := expect(t, a.do(http.MethodGet, agAgentPath("/model-configs?limit=1000"), nil), http.StatusOK)
	assert.EqualValues(t, 100, capped["pagination"].(map[string]any)["limit"])

	updated := expect(t, a.do(http.MethodPut, agAgentPath("/model-configs/"+id),
		map[string]any{"name": "chat2", "description": "renamed"}), http.StatusOK)
	assert.Equal(t, "renamed", updated["description"])
	got = expect(t, a.do(http.MethodGet, agAgentPath("/model-configs/"+id), nil), http.StatusOK)
	assert.Equal(t, "renamed", got["description"])

	mcpList := expect(t, a.do(http.MethodGet, agAgentPath("/mcp-configs"), nil), http.StatusOK)
	assert.Empty(t, mcpList["configs"], "an LLM configuration is not listed as an MCP one")

	rec := a.do(http.MethodDelete, agAgentPath("/model-configs/"+id), nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, agAgentPath("/model-configs/"+id), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, "Configuration not found", errorMessage(t, rec))
}

// TestAGRoutesModelConfigCreateRejections pins the 400, 404 and 409 answers of
// an LLM configuration create.
func TestAGRoutesModelConfigCreateRejections(t *testing.T) {
	a, gw := agSetup(t)
	agCreateProvider(t, a, "agprov", gw)
	expect(t, a.do(http.MethodPost, agAgentPath("/model-configs"), agModelConfigBody("taken", "agprov")), http.StatusCreated)
	// A catalog provider deployed only to a gateway of another environment.
	otherEnv := uuid.New()
	elsewhere := gwtest.GatewayIn(t, a.db, a.org, gwmodels.GatewayRoleBoth, otherEnv)
	agCreateProvider(t, a, "elsewhere", elsewhere)

	noProvider := agModelConfigBody("x", "agprov")
	noProvider["envMappings"] = map[string]any{"default": map[string]any{"configuration": map[string]any{}}}
	wrongType := agModelConfigBody("x", "agprov")
	wrongType["type"] = "mcp"
	noMappings := agModelConfigBody("x", "agprov")
	noMappings["envMappings"] = map[string]any{}

	tests := []struct {
		name    string
		path    string
		body    any
		status  int
		message string
	}{
		{"malformed json", agAgentPath("/model-configs"), "{", http.StatusBadRequest, "Invalid request body"},
		{"invalid name", agAgentPath("/model-configs"), agModelConfigBody("bad/name", "agprov"), http.StatusBadRequest, "invalid characters"},
		{"wrong type", agAgentPath("/model-configs"), wrongType, http.StatusBadRequest, "Type must be llm"},
		{"mapping without provider", agAgentPath("/model-configs"), noProvider, http.StatusBadRequest, "providerName or proxyName is required for environment default"},
		{"no mappings", agAgentPath("/model-configs"), noMappings, http.StatusBadRequest, "at least one environment mapping is required"},
		{"unknown provider", agAgentPath("/model-configs"), agModelConfigBody("x", "ghost"), http.StatusNotFound, "LLM provider not found"},
		{"provider not in the environment", agAgentPath("/model-configs"), agModelConfigBody("x", "elsewhere"), http.StatusBadRequest, "not deployed to a"},
		{"unknown agent", "/orgs/{org}/projects/" + agProject + "/agents/ghost/model-configs", agModelConfigBody("x", "agprov"), http.StatusNotFound, "Agent not found"},
		{"name taken", agAgentPath("/model-configs"), agModelConfigBody("taken", "agprov"), http.StatusConflict, "Agent configuration already exists"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPost, tc.path, tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Contains(t, errorMessage(t, rec), tc.message)
		})
	}
	list := expect(t, a.do(http.MethodGet, agAgentPath("/model-configs"), nil), http.StatusOK)
	assert.Len(t, list["configs"], 1, "no refused create left a configuration")
}

// TestAGRoutesModelConfigByIDRejections pins the 400s and 404s of the
// single-configuration LLM routes, including an MCP configuration's id.
func TestAGRoutesModelConfigByIDRejections(t *testing.T) {
	a, _ := agSetup(t)
	agCreateMCPProxy(t, a, "tools")
	mcpCfg := expect(t, a.do(http.MethodPost, agAgentPath("/mcp-configs"), agMCPConfigBody("tools", "tools")), http.StatusCreated)
	mcpID := mcpCfg["uuid"].(string)
	missing := uuid.NewString()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		for _, tc := range []struct {
			id      string
			status  int
			message string
		}{
			{"not-a-uuid", http.StatusBadRequest, "Invalid configuration ID"},
			{missing, http.StatusNotFound, "Configuration not found"},
			{mcpID, http.StatusNotFound, "Configuration not found"},
		} {
			rec := a.do(method, agAgentPath("/model-configs/"+tc.id), map[string]any{"description": "d"})
			require.Equal(t, tc.status, rec.Code, "%s %s: %s", method, tc.id, rec.Body.String())
			assert.Equal(t, tc.message, errorMessage(t, rec))
		}
	}
	rec := a.do(http.MethodPut, agAgentPath("/model-configs/"+missing), "{")
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "Invalid request body", errorMessage(t, rec))
	rec = a.do(http.MethodPut, agAgentPath("/model-configs/"+missing), map[string]any{"name": "bad/name"})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	rec = a.do(http.MethodPut, agAgentPath("/model-configs/"+missing),
		map[string]any{"envMappings": map[string]any{"default": map[string]any{"configuration": map[string]any{}}}})
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, errorMessage(t, rec), "providerName or proxyName is required")
	expect(t, a.do(http.MethodGet, agAgentPath("/mcp-configs/"+mcpID), nil), http.StatusOK)
}

// TestAGRoutesModelConfigUpdateSwitchesProvider pins that an update moving the
// environment to another provider re-points the mapping, read back after.
func TestAGRoutesModelConfigUpdateSwitchesProvider(t *testing.T) {
	a, gw := agSetup(t)
	agCreateProvider(t, a, "first", gw)
	second := agCreateProvider(t, a, "second", gw)
	created := expect(t, a.do(http.MethodPost, agAgentPath("/model-configs"), agModelConfigBody("chat", "first")), http.StatusCreated)
	id := created["uuid"].(string)

	rec := a.do(http.MethodPut, agAgentPath("/model-configs/"+id), map[string]any{
		"envMappings": map[string]any{"default": map[string]any{"providerName": "ghost", "configuration": map[string]any{}}},
	})
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, "LLM provider not found", errorMessage(t, rec))

	expect(t, a.do(http.MethodPut, agAgentPath("/model-configs/"+id), map[string]any{
		"envMappings": map[string]any{"default": map[string]any{"providerName": "second", "configuration": map[string]any{}}},
	}), http.StatusOK)
	got := expect(t, a.do(http.MethodGet, agAgentPath("/model-configs/"+id), nil), http.StatusOK)
	assert.Equal(t, second, agDefaultMapping(t, got)["providerUuid"])
}

// TestAGRoutesMCPConfigLifecycle creates an external agent's MCP configuration,
// reads it back, updates it and deletes it.
func TestAGRoutesMCPConfigLifecycle(t *testing.T) {
	a, _ := agSetup(t)
	agCreateMCPProxy(t, a, "tools")

	created := expect(t, a.do(http.MethodPost, agAgentPath("/mcp-configs"), agMCPConfigBody("tools", "tools")), http.StatusCreated)
	id := created["uuid"].(string)
	assert.Equal(t, "mcp", created["type"])
	mapping := agDefaultMapping(t, created)
	assert.Equal(t, "tools", mapping["mcpProxyName"])
	assert.Equal(t, "https://gw.example.com/tools/mcp", mapping["url"])
	auth := mapping["authInfo"].(map[string]any)
	assert.Equal(t, "X-API-Key", auth["name"])
	assert.NotEmpty(t, auth["value"], "the external agent is handed its key once")

	got := expect(t, a.do(http.MethodGet, agAgentPath("/mcp-configs/"+id), nil), http.StatusOK)
	assert.Equal(t, "tools", agDefaultMapping(t, got)["mcpProxyName"])
	list := expect(t, a.do(http.MethodGet, agAgentPath("/mcp-configs?limit=-1&offset=-1"), nil), http.StatusOK)
	require.Len(t, list["configs"], 1)
	models := expect(t, a.do(http.MethodGet, agAgentPath("/model-configs"), nil), http.StatusOK)
	assert.Empty(t, models["configs"], "an MCP configuration is not listed as an LLM one")

	expect(t, a.do(http.MethodPut, agAgentPath("/mcp-configs/"+id), map[string]any{"description": "my tools"}), http.StatusOK)
	got = expect(t, a.do(http.MethodGet, agAgentPath("/mcp-configs/"+id), nil), http.StatusOK)
	assert.Equal(t, "my tools", got["description"])

	rec := a.do(http.MethodDelete, agAgentPath("/mcp-configs/"+id), nil)
	require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
	rec = a.do(http.MethodGet, agAgentPath("/mcp-configs/"+id), nil)
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// TestAGRoutesMCPConfigRejections pins the 400, 404 and 409 answers of the MCP
// configuration routes.
func TestAGRoutesMCPConfigRejections(t *testing.T) {
	a, gw := agSetup(t)
	agCreateMCPProxy(t, a, "tools")
	agCreateProvider(t, a, "agprov", gw)
	llm := expect(t, a.do(http.MethodPost, agAgentPath("/model-configs"), agModelConfigBody("chat", "agprov")), http.StatusCreated)
	expect(t, a.do(http.MethodPost, agAgentPath("/mcp-configs"), agMCPConfigBody("taken", "tools")), http.StatusCreated)
	noProxy := agMCPConfigBody("x", "tools")
	noProxy["envMappings"] = map[string]any{"default": map[string]any{"configuration": map[string]any{}}}

	creates := []struct {
		name    string
		path    string
		body    any
		status  int
		message string
	}{
		{"malformed json", agAgentPath("/mcp-configs"), "{", http.StatusBadRequest, "Invalid request body"},
		{"invalid name", agAgentPath("/mcp-configs"), agMCPConfigBody("bad/name", "tools"), http.StatusBadRequest, "invalid characters"},
		{"mapping without proxy", agAgentPath("/mcp-configs"), noProxy, http.StatusBadRequest, "is required for environment default"},
		{"unknown proxy", agAgentPath("/mcp-configs"), agMCPConfigBody("x", "ghost"), http.StatusNotFound, "MCP proxy not found"},
		{"unknown agent", "/orgs/{org}/projects/" + agProject + "/agents/ghost/mcp-configs", agMCPConfigBody("x", "tools"), http.StatusNotFound, "Agent not found"},
		{"name taken", agAgentPath("/mcp-configs"), agMCPConfigBody("taken", "tools"), http.StatusConflict, "Agent MCP configuration already exists"},
	}
	for _, tc := range creates {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPost, tc.path, tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Contains(t, errorMessage(t, rec), tc.message)
		})
	}

	llmID := llm["uuid"].(string)
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		for _, tc := range []struct {
			id      string
			status  int
			message string
		}{
			{"not-a-uuid", http.StatusBadRequest, "Invalid configuration ID"},
			{uuid.NewString(), http.StatusNotFound, "Configuration not found"},
			{llmID, http.StatusNotFound, "Configuration not found"},
		} {
			rec := a.do(method, agAgentPath("/mcp-configs/"+tc.id), map[string]any{"description": "d"})
			require.Equal(t, tc.status, rec.Code, "%s %s: %s", method, tc.id, rec.Body.String())
			assert.Equal(t, tc.message, errorMessage(t, rec))
		}
	}
}

// TestAGRoutesAgentConfigForbidden pins the scopes the configuration routes
// require.
func TestAGRoutesAgentConfigForbidden(t *testing.T) {
	a, _ := agSetup(t)
	id := uuid.NewString()
	readOnly := withScopes(scope(gwperm.AgentRead))
	for _, r := range []struct {
		method, path string
		opt          reqOpt
	}{
		{http.MethodPost, agAgentPath("/model-configs"), readOnly},
		{http.MethodPut, agAgentPath("/model-configs/" + id), readOnly},
		{http.MethodDelete, agAgentPath("/model-configs/" + id), withScopes(scope(gwperm.AgentUpdate))},
		{http.MethodGet, agAgentPath("/model-configs"), withScopes(scope(gwperm.AgentUpdate))},
		{http.MethodPost, agAgentPath("/mcp-configs"), readOnly},
		{http.MethodGet, agAgentPath("/mcp-configs/" + id), withScopes(scope(gwperm.AgentUpdate))},
		{http.MethodPost, agAgentPath("/model-configs/" + id + "/environments/default/api-keys"), readOnly},
		{http.MethodDelete, agAgentPath("/mcp-configs/" + id + "/environments/default/api-keys/k"), readOnly},
		{http.MethodGet, agAgentPath("/mcp-configs/" + id + "/environments/default/api-keys"), withScopes(scope(gwperm.AgentAPIKeyManage))},
	} {
		rec := a.do(r.method, r.path, map[string]any{"name": "x"}, r.opt)
		assert.Equal(t, http.StatusForbidden, rec.Code, "%s %s: %s", r.method, r.path, rec.Body.String())
	}
}

// TestAGRoutesConfigAPIKeys issues, lists, rotates and revokes the per-config
// keys of an LLM and an MCP configuration.
func TestAGRoutesConfigAPIKeys(t *testing.T) {
	a, gw := agSetup(t)
	agCreateProvider(t, a, "agprov", gw)
	agCreateMCPProxy(t, a, "tools")
	llm := expect(t, a.do(http.MethodPost, agAgentPath("/model-configs"), agModelConfigBody("chat", "agprov")), http.StatusCreated)
	mcp := expect(t, a.do(http.MethodPost, agAgentPath("/mcp-configs"), agMCPConfigBody("tools", "tools")), http.StatusCreated)

	for _, kind := range []struct{ name, base string }{
		{"llm", agAgentPath("/model-configs/" + llm["uuid"].(string) + "/environments/default/api-keys")},
		{"mcp", agAgentPath("/mcp-configs/" + mcp["uuid"].(string) + "/environments/default/api-keys")},
	} {
		t.Run(kind.name, func(t *testing.T) {
			before := expect(t, a.do(http.MethodGet, kind.base, nil), http.StatusOK)
			keysBefore, _ := before["keys"].([]any)

			created := expect(t, a.do(http.MethodPost, kind.base, map[string]any{"name": "ci-key"}), http.StatusCreated)
			assert.Equal(t, "ci-key", created["keyId"])
			first, _ := created["apiKey"].(string)
			require.NotEmpty(t, first)

			listed := expect(t, a.do(http.MethodGet, kind.base, nil), http.StatusOK)
			keys := listed["keys"].([]any)
			require.Len(t, keys, len(keysBefore)+1)

			rotated := expect(t, a.do(http.MethodPut, kind.base+"/ci-key", nil), http.StatusOK)
			assert.NotEqual(t, first, rotated["apiKey"], "a rotation issues a new value")

			rec := a.do(http.MethodPut, kind.base+"/ci-key", "{")
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Equal(t, "Invalid request body", errorMessage(t, rec))
			rec = a.do(http.MethodPost, kind.base, map[string]any{})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Equal(t, "At least one of 'name' or 'displayName' must be provided", errorMessage(t, rec))
			rec = a.do(http.MethodPost, kind.base, "{")
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

			rec = a.do(http.MethodDelete, kind.base+"/ci-key", nil)
			require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
			listed = expect(t, a.do(http.MethodGet, kind.base, nil), http.StatusOK)
			assert.Len(t, listed["keys"], len(keysBefore), "the revoked key is gone")
		})
	}

	missing := uuid.NewString()
	for _, base := range []string{
		agAgentPath("/model-configs/" + missing + "/environments/default/api-keys"),
		agAgentPath("/mcp-configs/" + missing + "/environments/default/api-keys"),
	} {
		for _, r := range []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, base, nil},
			{http.MethodPost, base, map[string]any{"name": "k1"}},
			{http.MethodPut, base + "/k1", nil},
			{http.MethodDelete, base + "/k1", nil},
		} {
			rec := a.do(r.method, r.path, r.body)
			require.Equal(t, http.StatusNotFound, rec.Code, "%s %s: %s", r.method, r.path, rec.Body.String())
			assert.Equal(t, "Configuration not found", errorMessage(t, rec))
		}
		rec := a.do(http.MethodGet, agAgentPath("/model-configs/nope/environments/default/api-keys"), nil)
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "Invalid configuration ID", errorMessage(t, rec))
	}
}

// TestAGRoutesMCPConfigUpdate pins the update's 400s and 404, and that an
// update moving the environment to another proxy is read back.
func TestAGRoutesMCPConfigUpdate(t *testing.T) {
	a, _ := agSetup(t)
	agCreateMCPProxy(t, a, "first")
	agCreateMCPProxy(t, a, "second")
	created := expect(t, a.do(http.MethodPost, agAgentPath("/mcp-configs"), agMCPConfigBody("tools", "first")), http.StatusCreated)
	path := agAgentPath("/mcp-configs/" + created["uuid"].(string))

	tests := []struct {
		name    string
		body    any
		status  int
		message string
	}{
		{"malformed json", "{", http.StatusBadRequest, "Invalid request body"},
		{"invalid name", map[string]any{"name": "bad/name"}, http.StatusBadRequest, "invalid characters"},
		{"mapping without proxy", map[string]any{"envMappings": map[string]any{"default": map[string]any{"configuration": map[string]any{}}}}, http.StatusBadRequest, "is required for environment default"},
		{"unknown proxy", map[string]any{"envMappings": map[string]any{"default": map[string]any{"mcpProxyName": "ghost", "configuration": map[string]any{}}}}, http.StatusNotFound, "MCP proxy not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPut, path, tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Contains(t, errorMessage(t, rec), tc.message)
		})
	}
	got := expect(t, a.do(http.MethodGet, path, nil), http.StatusOK)
	assert.Equal(t, "first", agDefaultMapping(t, got)["mcpProxyName"], "the refused updates changed nothing")

	expect(t, a.do(http.MethodPut, path, map[string]any{
		"envMappings": map[string]any{"default": map[string]any{"mcpProxyId": "second", "configuration": map[string]any{}}},
	}), http.StatusOK)
	got = expect(t, a.do(http.MethodGet, path, nil), http.StatusOK)
	assert.Equal(t, "second", agDefaultMapping(t, got)["mcpProxyName"])
}

// TestAGRoutesModelConfigPolicies pins that an environment's policies, with
// their paths, are stored and read back.
func TestAGRoutesModelConfigPolicies(t *testing.T) {
	a, gw := agSetup(t)
	agCreateProvider(t, a, "agprov", gw)
	body := agModelConfigBody("guarded", "agprov")
	body["envMappings"] = map[string]any{"default": map[string]any{"proxyName": "agprov", "configuration": map[string]any{
		"policies": []any{map[string]any{"name": "word-count-guardrail", "version": "v0", "paths": []any{
			map[string]any{"path": "/chat/completions", "methods": []string{"POST"}, "params": map[string]any{"max": 100}},
		}}},
	}}}
	created := expect(t, a.do(http.MethodPost, agAgentPath("/model-configs"), body), http.StatusCreated)

	got := expect(t, a.do(http.MethodGet, agAgentPath("/model-configs/"+created["uuid"].(string)), nil), http.StatusOK)
	policies := agDefaultMapping(t, got)["policies"].([]any)
	require.Len(t, policies, 1)
	policy := policies[0].(map[string]any)
	assert.Equal(t, "word-count-guardrail", policy["name"])
	assert.Equal(t, []any{map[string]any{"path": "/chat/completions", "methods": []any{"POST"}, "params": map[string]any{"max": float64(100)}}}, policy["paths"])
}
