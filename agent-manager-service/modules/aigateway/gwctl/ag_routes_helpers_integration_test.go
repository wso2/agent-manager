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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// Shared fixtures for the agent-facing routes: an external agent, a gateway
// serving both roles in the default environment, and the providers and MCP
// proxies the agent's configurations bind to.

const (
	agProject = "ag-proj"
	agAgent   = "ag-agent"
	// agPublicURL is a public address the URL checks accept without a DNS lookup.
	agPublicURL = "https://1.1.1.1/mcp"
)

// agAgentPath is path under the test agent's routes, in its project.
func agAgentPath(path string) string { return agentPathIn(agProject, agAgent, path) }

// agentPathIn is path under agent's routes with the project in the query.
func agentPathIn(project, agent, path string) string {
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	return "/agents/" + agent + path + sep + "project=" + project
}

// agSetup serves the gateway for a fresh organization with the test agent and
// one gateway in the default environment, and returns the gateway.
func agSetup(t *testing.T) (*routeAPI, *gwmodels.Gateway) {
	t.Helper()
	a := newRouteAPI(t)
	a.agent(agProject, agAgent)
	return a, gwtest.Gateway(t, a.db, a.org, gwmodels.GatewayRoleBoth)
}

// agCreateProvider creates a catalog LLM provider deployed to gateway.
func agCreateProvider(t *testing.T, a *routeAPI, handle string, gateway *gwmodels.Gateway) string {
	t.Helper()
	doc := map[string]any{
		"apiVersion": "ai-control-plane.choreolabs.com/v1",
		"kind":       "LlmProvider",
		"metadata":   map[string]any{"name": handle},
		"spec": map[string]any{
			"displayName": "Provider " + handle, "version": "v1.0", "context": "/" + handle,
			"template": "openai", "inCatalog": true, "gateways": []string{gateway.UUID.String()},
			"upstream": map[string]any{"main": map[string]any{
				"url":  "https://api.openai.com/v1",
				"auth": map[string]any{"type": "api-key", "header": "Authorization", "value": "Bearer sk-test"},
			}},
		},
	}
	body := expect(t, a.do(http.MethodPost, "/llm-providers", doc), http.StatusCreated)
	uid, _ := body["metadata"].(map[string]any)["uid"].(string)
	require.NotEmpty(t, uid)
	return uid
}

// agMCPProxyBody is an MCP proxy document with one endpoint bound to env.
func agMCPProxyBody(handle, env string) map[string]any {
	return map[string]any{
		"apiVersion": "ai-control-plane.choreolabs.com/v1",
		"kind":       "McpProxy",
		"metadata":   map[string]any{"name": handle},
		"spec": map[string]any{
			"displayName": "MCP " + handle, "version": "v1.0", "context": "/" + handle,
			"description": "proxy " + handle,
			"endpoints": []any{map[string]any{
				"id":           "primary",
				"upstream":     map[string]any{"main": map[string]any{"url": agPublicURL}},
				"environments": []any{map[string]any{"environmentUuid": env}},
			}},
		},
	}
}

// agCreateMCPProxy creates an org-level MCP proxy bound to the default environment.
func agCreateMCPProxy(t *testing.T, a *routeAPI, handle string) {
	t.Helper()
	expect(t, a.do(http.MethodPost, "/mcp-proxies", agMCPProxyBody(handle, a.defaultEnv().String())), http.StatusCreated)
}

// agReason is the technical reason of an error response.
func agReason(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Reason string `json:"reason"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body), "body: %s", rec.Body.String())
	return body.Reason
}
