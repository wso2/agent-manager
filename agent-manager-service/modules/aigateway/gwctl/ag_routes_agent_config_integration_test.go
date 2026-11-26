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

// agConfigDoc is a config document of kind in the test agent's project.
func agConfigDoc(kind, name string, spec map[string]any) map[string]any {
	return map[string]any{
		"apiVersion": "ai-control-plane.choreolabs.com/v1",
		"kind":       kind,
		"metadata":   map[string]any{"name": name, "project": agProject},
		"spec":       spec,
	}
}

// agModelConfigBody is an AgentModelConfig document binding provider.
func agModelConfigBody(name, provider string) map[string]any {
	return agConfigDoc("AgentModelConfig", name, map[string]any{
		"description": "config " + name, "providerName": provider, "configuration": map[string]any{},
	})
}

// agMCPConfigBody is an AgentMcpConfig document binding proxy.
func agMCPConfigBody(name, proxy string) map[string]any {
	return agConfigDoc("AgentMcpConfig", name, map[string]any{"proxyName": proxy, "configuration": map[string]any{}})
}

// agUID is a config document's metadata.uid.
func agUID(t *testing.T, cfg map[string]any) string {
	t.Helper()
	uid, _ := agPart(t, cfg, "metadata")["uid"].(string)
	require.NotEmpty(t, uid, "metadata.uid: %v", cfg)
	return uid
}

// agPart is one top-level object of a document: metadata, spec or status.
func agPart(t *testing.T, cfg map[string]any, part string) map[string]any {
	t.Helper()
	m, ok := cfg[part].(map[string]any)
	require.True(t, ok, "%s: %v", part, cfg)
	return m
}

// agList is the items of a list response.
func agList(t *testing.T, list map[string]any) []any {
	t.Helper()
	items, ok := list["list"].([]any)
	require.True(t, ok, "list: %v", list)
	return items
}

// TestAGRoutesModelConfigLifecycle creates an external agent's LLM
// configuration, reads it back, renames it and deletes it.
func TestAGRoutesModelConfigLifecycle(t *testing.T) {
	a, gw := agSetup(t)
	providerUID := agCreateProvider(t, a, "agprov", gw)

	created := expect(t, a.do(http.MethodPost, agAgentPath("/model-configs"), agModelConfigBody("chat", "agprov")), http.StatusCreated)
	id := agUID(t, created)
	assert.Equal(t, "AgentModelConfig", created["kind"])
	assert.Equal(t, "chat", agPart(t, created, "metadata")["name"])
	assert.Equal(t, agProject, agPart(t, created, "metadata")["project"])
	spec := agPart(t, created, "spec")
	assert.Equal(t, "agprov", spec["providerName"])
	assert.Len(t, spec["environmentVariables"], 2)
	status := agPart(t, created, "status")
	assert.Equal(t, providerUID, status["providerUuid"])
	assert.Contains(t, status["url"], "https://gw.example.com/", "the external agent is handed the gateway's public URL")

	got := expect(t, a.do(http.MethodGet, agAgentPath("/model-configs/"+id), nil), http.StatusOK)
	assert.Equal(t, "config chat", agPart(t, got, "spec")["description"])
	assert.Equal(t, "agprov", agPart(t, got, "spec")["providerName"])

	list := expect(t, a.do(http.MethodGet, agAgentPath("/model-configs?limit=0&offset=-5"), nil), http.StatusOK)
	items := agList(t, list)
	require.Len(t, items, 1)
	assert.EqualValues(t, 1, list["count"])
	assert.Equal(t, id, agUID(t, items[0].(map[string]any)))
	assert.Equal(t, "agprov", agPart(t, items[0].(map[string]any), "spec")["providerName"], "a list item carries the bound provider")
	assert.EqualValues(t, 20, list["pagination"].(map[string]any)["limit"], "an out-of-range limit falls back to the default")
	capped := expect(t, a.do(http.MethodGet, agAgentPath("/model-configs?limit=1000"), nil), http.StatusOK)
	assert.EqualValues(t, 100, capped["pagination"].(map[string]any)["limit"])

	renamed := agModelConfigBody("chat2", "agprov")
	renamed["spec"].(map[string]any)["description"] = "renamed"
	updated := expect(t, a.do(http.MethodPut, agAgentPath("/model-configs/"+id), renamed), http.StatusOK)
	assert.Equal(t, "renamed", agPart(t, updated, "spec")["description"])
	assert.Equal(t, "chat2", agPart(t, updated, "metadata")["name"])
	got = expect(t, a.do(http.MethodGet, agAgentPath("/model-configs/"+id), nil), http.StatusOK)
	assert.Equal(t, "renamed", agPart(t, got, "spec")["description"])

	// An absent description keeps the stored one.
	kept := agConfigDoc("AgentModelConfig", "chat2", map[string]any{"providerName": "agprov"})
	expect(t, a.do(http.MethodPut, agAgentPath("/model-configs/"+id), kept), http.StatusOK)
	got = expect(t, a.do(http.MethodGet, agAgentPath("/model-configs/"+id), nil), http.StatusOK)
	assert.Equal(t, "renamed", agPart(t, got, "spec")["description"])

	mcpList := expect(t, a.do(http.MethodGet, agAgentPath("/mcp-configs"), nil), http.StatusOK)
	assert.Empty(t, agList(t, mcpList), "an LLM configuration is not listed as an MCP one")

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

	noProvider := agConfigDoc("AgentModelConfig", "x", map[string]any{"configuration": map[string]any{}})
	wrongKind := agModelConfigBody("x", "agprov")
	wrongKind["kind"] = "AgentMcpConfig"
	noSpec := agModelConfigBody("x", "agprov")
	delete(noSpec, "spec")
	otherProject := agModelConfigBody("x", "agprov")
	otherProject["metadata"].(map[string]any)["project"] = "other"

	tests := []struct {
		name    string
		path    string
		body    any
		status  int
		message string
	}{
		{"malformed json", agAgentPath("/model-configs"), "{", http.StatusBadRequest, "Invalid request body"},
		{"no project", "/agents/" + agAgent + "/model-configs", agModelConfigBody("x", "agprov"), http.StatusBadRequest, "project query parameter is required"},
		{"other project", agAgentPath("/model-configs"), otherProject, http.StatusBadRequest, "metadata.project must match"},
		{"invalid name", agAgentPath("/model-configs"), agModelConfigBody("bad/name", "agprov"), http.StatusBadRequest, "invalid characters"},
		{"wrong kind", agAgentPath("/model-configs"), wrongKind, http.StatusBadRequest, `kind must be "AgentModelConfig"`},
		{"no provider", agAgentPath("/model-configs"), noProvider, http.StatusBadRequest, "spec.providerName is required"},
		{"no spec", agAgentPath("/model-configs"), noSpec, http.StatusBadRequest, "spec is required"},
		{"unknown provider", agAgentPath("/model-configs"), agModelConfigBody("x", "ghost"), http.StatusNotFound, "LLM provider not found"},
		{"provider not in the environment", agAgentPath("/model-configs"), agModelConfigBody("x", "elsewhere"), http.StatusBadRequest, "not deployed to a"},
		{"unknown agent", agentPathIn(agProject, "ghost", "/model-configs"), agModelConfigBody("x", "agprov"), http.StatusNotFound, "Agent not found"},
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
	assert.Len(t, agList(t, list), 1, "no refused create left a configuration")
}

// TestAGRoutesModelConfigByIDRejections pins the 400s and 404s of the
// single-configuration LLM routes, including an MCP configuration's id.
func TestAGRoutesModelConfigByIDRejections(t *testing.T) {
	a, _ := agSetup(t)
	agCreateMCPProxy(t, a, "tools")
	mcpCfg := expect(t, a.do(http.MethodPost, agAgentPath("/mcp-configs"), agMCPConfigBody("tools", "tools")), http.StatusCreated)
	mcpID := agUID(t, mcpCfg)
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
			rec := a.do(method, agAgentPath("/model-configs/"+tc.id), agModelConfigBody("d", "agprov"))
			require.Equal(t, tc.status, rec.Code, "%s %s: %s", method, tc.id, rec.Body.String())
			assert.Equal(t, tc.message, errorMessage(t, rec))
		}
	}
	rec := a.do(http.MethodPut, agAgentPath("/model-configs/"+missing), "{")
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Equal(t, "Invalid request body", errorMessage(t, rec))
	rec = a.do(http.MethodPut, agAgentPath("/model-configs/"+missing), agModelConfigBody("bad/name", "agprov"))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	rec = a.do(http.MethodPut, agAgentPath("/model-configs/"+missing), agConfigDoc("AgentModelConfig", "d", map[string]any{}))
	require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
	assert.Contains(t, errorMessage(t, rec), "spec.providerName is required")
	expect(t, a.do(http.MethodGet, agAgentPath("/mcp-configs/"+mcpID), nil), http.StatusOK)
}

// TestAGRoutesModelConfigUpdateSwitchesProvider pins that an update moving the
// binding to another provider re-points it, read back after.
func TestAGRoutesModelConfigUpdateSwitchesProvider(t *testing.T) {
	a, gw := agSetup(t)
	agCreateProvider(t, a, "first", gw)
	second := agCreateProvider(t, a, "second", gw)
	created := expect(t, a.do(http.MethodPost, agAgentPath("/model-configs"), agModelConfigBody("chat", "first")), http.StatusCreated)
	id := agUID(t, created)

	rec := a.do(http.MethodPut, agAgentPath("/model-configs/"+id), agModelConfigBody("chat", "ghost"))
	require.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
	assert.Equal(t, "LLM provider not found", errorMessage(t, rec))

	expect(t, a.do(http.MethodPut, agAgentPath("/model-configs/"+id), agModelConfigBody("chat", "second")), http.StatusOK)
	got := expect(t, a.do(http.MethodGet, agAgentPath("/model-configs/"+id), nil), http.StatusOK)
	assert.Equal(t, second, agPart(t, got, "status")["providerUuid"])
	assert.Equal(t, "second", agPart(t, got, "spec")["providerName"])
}

// TestAGRoutesMCPConfigLifecycle creates an external agent's MCP configuration,
// reads it back, updates it and deletes it.
func TestAGRoutesMCPConfigLifecycle(t *testing.T) {
	a, _ := agSetup(t)
	agCreateMCPProxy(t, a, "tools")

	created := expect(t, a.do(http.MethodPost, agAgentPath("/mcp-configs"), agMCPConfigBody("tools", "tools")), http.StatusCreated)
	id := agUID(t, created)
	assert.Equal(t, "AgentMcpConfig", created["kind"])
	assert.Equal(t, "tools", agPart(t, created, "spec")["proxyName"])
	status := agPart(t, created, "status")
	assert.Equal(t, "https://gw.example.com/tools/mcp", status["url"])
	auth := status["authInfo"].(map[string]any)
	assert.Equal(t, "X-API-Key", auth["name"])
	assert.NotEmpty(t, auth["value"], "the external agent is handed its key once")

	got := expect(t, a.do(http.MethodGet, agAgentPath("/mcp-configs/"+id), nil), http.StatusOK)
	assert.Equal(t, "tools", agPart(t, got, "spec")["proxyName"])
	list := expect(t, a.do(http.MethodGet, agAgentPath("/mcp-configs?limit=-1&offset=-1"), nil), http.StatusOK)
	require.Len(t, agList(t, list), 1)
	models := expect(t, a.do(http.MethodGet, agAgentPath("/model-configs"), nil), http.StatusOK)
	assert.Empty(t, agList(t, models), "an MCP configuration is not listed as an LLM one")

	described := agMCPConfigBody("tools", "tools")
	described["spec"].(map[string]any)["description"] = "my tools"
	expect(t, a.do(http.MethodPut, agAgentPath("/mcp-configs/"+id), described), http.StatusOK)
	got = expect(t, a.do(http.MethodGet, agAgentPath("/mcp-configs/"+id), nil), http.StatusOK)
	assert.Equal(t, "my tools", agPart(t, got, "spec")["description"])

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
	noProxy := agConfigDoc("AgentMcpConfig", "x", map[string]any{"configuration": map[string]any{}})

	creates := []struct {
		name    string
		path    string
		body    any
		status  int
		message string
	}{
		{"malformed json", agAgentPath("/mcp-configs"), "{", http.StatusBadRequest, "Invalid request body"},
		{"invalid name", agAgentPath("/mcp-configs"), agMCPConfigBody("bad/name", "tools"), http.StatusBadRequest, "invalid characters"},
		{"wrong kind", agAgentPath("/mcp-configs"), agModelConfigBody("x", "tools"), http.StatusBadRequest, `kind must be "AgentMcpConfig"`},
		{"no proxy", agAgentPath("/mcp-configs"), noProxy, http.StatusBadRequest, "spec.proxyName is required"},
		{"unknown proxy", agAgentPath("/mcp-configs"), agMCPConfigBody("x", "ghost"), http.StatusNotFound, "MCP proxy not found"},
		{"unknown agent", agentPathIn(agProject, "ghost", "/mcp-configs"), agMCPConfigBody("x", "tools"), http.StatusNotFound, "Agent not found"},
		{"name taken", agAgentPath("/mcp-configs"), agMCPConfigBody("taken", "tools"), http.StatusConflict, "Agent MCP configuration already exists"},
	}
	for _, tc := range creates {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPost, tc.path, tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Contains(t, errorMessage(t, rec), tc.message)
		})
	}

	llmID := agUID(t, llm)
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
			rec := a.do(method, agAgentPath("/mcp-configs/"+tc.id), agMCPConfigBody("d", "tools"))
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
		{http.MethodPost, agAgentPath("/model-configs/" + id + "/api-keys"), readOnly},
		{http.MethodDelete, agAgentPath("/mcp-configs/" + id + "/api-keys/k"), readOnly},
		{http.MethodGet, agAgentPath("/mcp-configs/" + id + "/api-keys"), withScopes(scope(gwperm.AgentAPIKeyManage))},
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
		{"llm", "/model-configs/" + agUID(t, llm) + "/api-keys"},
		{"mcp", "/mcp-configs/" + agUID(t, mcp) + "/api-keys"},
	} {
		t.Run(kind.name, func(t *testing.T) {
			keys := func(p string) string { return agAgentPath(kind.base + p) }
			before := expect(t, a.do(http.MethodGet, keys(""), nil), http.StatusOK)
			keysBefore, _ := before["keys"].([]any)

			created := expect(t, a.do(http.MethodPost, keys(""), map[string]any{"name": "ci-key"}), http.StatusCreated)
			assert.Equal(t, "ci-key", created["keyId"])
			first, _ := created["apiKey"].(string)
			require.NotEmpty(t, first)

			listed := expect(t, a.do(http.MethodGet, keys(""), nil), http.StatusOK)
			require.Len(t, listed["keys"].([]any), len(keysBefore)+1)

			rotated := expect(t, a.do(http.MethodPut, keys("/ci-key"), nil), http.StatusOK)
			assert.NotEqual(t, first, rotated["apiKey"], "a rotation issues a new value")

			rec := a.do(http.MethodPut, keys("/ci-key"), "{")
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Equal(t, "Invalid request body", errorMessage(t, rec))
			rec = a.do(http.MethodPost, keys(""), map[string]any{})
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
			assert.Equal(t, "At least one of 'name' or 'displayName' must be provided", errorMessage(t, rec))
			rec = a.do(http.MethodPost, keys(""), "{")
			require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())

			rec = a.do(http.MethodDelete, keys("/ci-key"), nil)
			require.Equal(t, http.StatusNoContent, rec.Code, rec.Body.String())
			listed = expect(t, a.do(http.MethodGet, keys(""), nil), http.StatusOK)
			assert.Len(t, listed["keys"], len(keysBefore), "the revoked key is gone")
		})
	}

	missing := uuid.NewString()
	for _, base := range []string{"/model-configs/" + missing + "/api-keys", "/mcp-configs/" + missing + "/api-keys"} {
		for _, r := range []struct {
			method, path string
			body         any
		}{
			{http.MethodGet, agAgentPath(base), nil},
			{http.MethodPost, agAgentPath(base), map[string]any{"name": "k1"}},
			{http.MethodPut, agAgentPath(base + "/k1"), nil},
			{http.MethodDelete, agAgentPath(base + "/k1"), nil},
		} {
			rec := a.do(r.method, r.path, r.body)
			require.Equal(t, http.StatusNotFound, rec.Code, "%s %s: %s", r.method, r.path, rec.Body.String())
			assert.Equal(t, "Configuration not found", errorMessage(t, rec))
		}
		rec := a.do(http.MethodGet, agAgentPath("/model-configs/nope/api-keys"), nil)
		require.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
		assert.Equal(t, "Invalid configuration ID", errorMessage(t, rec))
	}
}

// TestAGRoutesMCPConfigUpdate pins the update's 400s and 404, and that an
// update moving the binding to another proxy is read back.
func TestAGRoutesMCPConfigUpdate(t *testing.T) {
	a, _ := agSetup(t)
	agCreateMCPProxy(t, a, "first")
	agCreateMCPProxy(t, a, "second")
	created := expect(t, a.do(http.MethodPost, agAgentPath("/mcp-configs"), agMCPConfigBody("tools", "first")), http.StatusCreated)
	path := agAgentPath("/mcp-configs/" + agUID(t, created))
	otherProject := agMCPConfigBody("tools", "first")
	otherProject["metadata"].(map[string]any)["project"] = "other"

	tests := []struct {
		name    string
		body    any
		status  int
		message string
	}{
		{"malformed json", "{", http.StatusBadRequest, "Invalid request body"},
		{"invalid name", agMCPConfigBody("bad/name", "first"), http.StatusBadRequest, "invalid characters"},
		{"other project", otherProject, http.StatusBadRequest, "metadata.project must match"},
		{"no proxy", agConfigDoc("AgentMcpConfig", "tools", map[string]any{"configuration": map[string]any{}}), http.StatusBadRequest, "spec.proxyName is required"},
		{"unknown proxy", agMCPConfigBody("tools", "ghost"), http.StatusNotFound, "MCP proxy not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := a.do(http.MethodPut, path, tc.body)
			require.Equal(t, tc.status, rec.Code, rec.Body.String())
			assert.Contains(t, errorMessage(t, rec), tc.message)
		})
	}
	got := expect(t, a.do(http.MethodGet, path, nil), http.StatusOK)
	assert.Equal(t, "first", agPart(t, got, "spec")["proxyName"], "the refused updates changed nothing")

	expect(t, a.do(http.MethodPut, path, agMCPConfigBody("tools", "second")), http.StatusOK)
	got = expect(t, a.do(http.MethodGet, path, nil), http.StatusOK)
	assert.Equal(t, "second", agPart(t, got, "spec")["proxyName"])
}

// TestAGRoutesModelConfigPolicies pins that a binding's policies, with their
// paths, are stored, read back and kept by an update that omits them.
func TestAGRoutesModelConfigPolicies(t *testing.T) {
	a, gw := agSetup(t)
	agCreateProvider(t, a, "agprov", gw)
	body := agModelConfigBody("guarded", "agprov")
	body["spec"].(map[string]any)["configuration"] = map[string]any{
		"policies": []any{map[string]any{"name": "word-count-guardrail", "version": "v0", "paths": []any{
			map[string]any{"path": "/chat/completions", "methods": []string{"POST"}, "params": map[string]any{"max": 100}},
		}}},
	}
	created := expect(t, a.do(http.MethodPost, agAgentPath("/model-configs"), body), http.StatusCreated)
	path := agAgentPath("/model-configs/" + agUID(t, created))

	assertPolicy := func(got map[string]any) {
		t.Helper()
		cfg := agPart(t, got, "spec")["configuration"].(map[string]any)
		policies := cfg["policies"].([]any)
		require.Len(t, policies, 1)
		policy := policies[0].(map[string]any)
		assert.Equal(t, "word-count-guardrail", policy["name"])
		assert.Equal(t, []any{map[string]any{"path": "/chat/completions", "methods": []any{"POST"}, "params": map[string]any{"max": float64(100)}}}, policy["paths"])
	}
	assertPolicy(expect(t, a.do(http.MethodGet, path, nil), http.StatusOK))

	// An absent configuration keeps the stored policies.
	expect(t, a.do(http.MethodPut, path, agConfigDoc("AgentModelConfig", "guarded", map[string]any{"providerName": "agprov"})), http.StatusOK)
	assertPolicy(expect(t, a.do(http.MethodGet, path, nil), http.StatusOK))
}
