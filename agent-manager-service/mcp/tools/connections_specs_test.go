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
package tools

import (
	"context"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// Returns the test specs for tools registered by registerLLMProviderTools.
func llmProviderToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_llm_provider_templates",
			toolset:             "llm-provider",
			permissions:         []rbac.Permission{rbac.LLMProviderTemplateRead},
			descriptionKeywords: []string{"list", "template"},
			descriptionMinLen:   20,
			optionalParams:      []string{"limit", "offset"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListLLMProviderTemplates",
			validateCall: func(t *testing.T, args []interface{}) {
				if got := args[0]; got != testOrgName {
					t.Errorf("ouID: got %v, want %q", got, testOrgName)
				}
			},
		},
		{
			name:                "list_llm_providers",
			toolset:             "llm-provider",
			permissions:         []rbac.Permission{rbac.LLMProviderRead},
			descriptionKeywords: []string{"list", "provider"},
			descriptionMinLen:   20,
			optionalParams:      []string{"limit", "offset"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListLLMProviders",
			validateCall: func(t *testing.T, args []interface{}) {
				if got := args[0]; got != testOrgName {
					t.Errorf("ouID: got %v, want %q", got, testOrgName)
				}
			},
		},
		{
			name:                "get_llm_provider",
			toolset:             "llm-provider",
			permissions:         []rbac.Permission{rbac.LLMProviderRead},
			descriptionKeywords: []string{"provider", "guardrail", "deployed"},
			descriptionMinLen:   20,
			requiredParams:      []string{"provider_id"},
			testArgs:            map[string]any{"provider_id": testProviderID},
			expectedMethod:      "GetLLMProvider",
			validateCall: func(t *testing.T, args []interface{}) {
				if got := args[1]; got != testProviderID {
					t.Errorf("providerID: got %v, want %q", got, testProviderID)
				}
			},
		},
		{
			name:                "add_llm_provider_guardrails",
			toolset:             "llm-provider",
			permissions:         []rbac.Permission{rbac.LLMProviderUpdate},
			descriptionKeywords: []string{"guardrail", "provider", "redeployed"},
			descriptionMinLen:   20,
			requiredParams:      []string{"provider_id", "guardrails"},
			testArgs: map[string]any{
				"provider_id": testProviderID,
				"guardrails": []any{
					map[string]any{"name": testGuardrailName, "version": "v2", "params": map[string]any{"mask": false}},
					map[string]any{"name": "word-count", "version": "v1"},
				},
			},
			expectedMethod: "SetLLMProviderGuardrails",
			validateCall: func(t *testing.T, args []interface{}) {
				got, ok := args[2].([]models.LLMPolicy)
				if !ok {
					t.Fatalf("guardrails: got %T", args[2])
				}
				// The provider already has pii-masking v1: it is replaced in place,
				// and word-count is appended.
				if len(got) != 2 || got[0].Name != testGuardrailName || got[0].Version != "v2" || got[1].Name != "word-count" {
					t.Fatalf("guardrails: got %+v", got)
				}
				if got[0].Paths[0].Params["mask"] != false || got[0].Paths[0].Path != guardrailPathAll {
					t.Errorf("replaced guardrail params/path: got %+v", got[0].Paths)
				}
			},
		},
		{
			name:                "remove_llm_provider_guardrail",
			toolset:             "llm-provider",
			permissions:         []rbac.Permission{rbac.LLMProviderUpdate},
			descriptionKeywords: []string{"remove", "guardrail", "redeployed"},
			descriptionMinLen:   20,
			requiredParams:      []string{"provider_id", "name"},
			testArgs:            map[string]any{"provider_id": testProviderID, "name": testGuardrailName},
			expectedMethod:      "SetLLMProviderGuardrails",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, ok := args[2].([]models.LLMPolicy); !ok || len(got) != 0 {
					t.Fatalf("guardrails after removal: got %v", args[2])
				}
			},
		},
		{
			name:                "list_llm_guardrails",
			toolset:             "llm-provider",
			permissions:         []rbac.Permission{rbac.LLMProviderRead},
			descriptionKeywords: []string{"guardrail"},
			descriptionMinLen:   20,
			optionalParams:      []string{"provider_id", "name"},
			testArgs:            map[string]any{"provider_id": testProviderID},
			expectedMethod:      "ListLLMGuardrails",
			validateCall: func(t *testing.T, args []interface{}) {
				if got := args[1]; got != testProviderID {
					t.Errorf("providerID: got %v, want %q", got, testProviderID)
				}
			},
		},
		{
			name:                "create_llm_provider",
			toolset:             "llm-provider",
			permissions:         []rbac.Permission{rbac.LLMProviderCreate},
			descriptionKeywords: []string{"create", "provider", "guardrail"},
			descriptionMinLen:   20,
			requiredParams:      []string{"name", "template"},
			optionalParams:      []string{"api_key", "upstream_url", "description", "version", "gateway_ids", "guardrails"},
			testArgs: map[string]any{
				"name":        "My OpenAI",
				"template":    testTemplateID,
				"api_key":     "sk-test",
				"gateway_ids": []any{testGatewayID},
				"guardrails":  []any{map[string]any{"name": testGuardrailName, "version": "v1", "params": map[string]any{"mask": true}}},
			},
			expectedMethod: "CreateLLMProvider",
			validateCall: func(t *testing.T, args []interface{}) {
				req, ok := args[1].(*spec.CreateLLMProviderRequest)
				if !ok {
					t.Fatalf("request: got %T", args[1])
				}
				if req.Id != "my-openai" || req.Context != "/my-openai" || req.Version != defaultLLMProviderVersion {
					t.Errorf("handle/context/version: got %q %q %q", req.Id, req.Context, req.Version)
				}
				if req.Upstream.Main == nil || req.Upstream.Main.Url == nil || *req.Upstream.Main.Url != "https://api.openai.com" {
					t.Errorf("upstream url not defaulted from template: %+v", req.Upstream.Main)
				}
				if a := req.Upstream.Main.Auth; a == nil || a.Value == nil || *a.Value != "Bearer sk-test" || a.Header == nil || *a.Header != "Authorization" {
					t.Errorf("upstream auth not built from template: %+v", a)
				}
				if len(req.Gateways) != 1 || req.Gateways[0] != testGatewayID {
					t.Errorf("gateways: got %v", req.Gateways)
				}
				if len(req.Policies) != 1 || req.Policies[0].Name != testGuardrailName || req.Policies[0].Paths[0].Path != guardrailPathAll {
					t.Errorf("guardrails: got %+v", req.Policies)
				}
			},
		},
	}
}

// Returns the test specs for tools registered by registerGatewayTools.
func gatewayToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_gateways",
			toolset:             "gateway",
			permissions:         []rbac.Permission{rbac.GatewayRead},
			descriptionKeywords: []string{"list", "gateway"},
			descriptionMinLen:   20,
			optionalParams:      []string{"environment", "limit", "offset"},
			testArgs:            map[string]any{"environment": testEnvName},
			expectedMethod:      "ListGateways",
			validateCall: func(t *testing.T, args []interface{}) {
				if got := args[1]; got != testEnvUUID {
					t.Errorf("environmentID: got %v, want %q", got, testEnvUUID)
				}
			},
		},
	}
}

// Returns the test specs for tools registered by registerMCPProxyTools.
func mcpProxyToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_mcp_proxies",
			toolset:             "mcp-proxy",
			permissions:         []rbac.Permission{rbac.MCPServerRead},
			descriptionKeywords: []string{"list", "mcp proxies"},
			descriptionMinLen:   20,
			optionalParams:      []string{"limit", "offset"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListMCPProxies",
			validateCall: func(t *testing.T, args []interface{}) {
				if got := args[0]; got != testOrgName {
					t.Errorf("ouID: got %v, want %q", got, testOrgName)
				}
			},
		},
		{
			name:                "get_mcp_proxy",
			toolset:             "mcp-proxy",
			permissions:         []rbac.Permission{rbac.MCPServerRead},
			descriptionKeywords: []string{"mcp proxy", "tools", "deployed"},
			descriptionMinLen:   20,
			requiredParams:      []string{"proxy_id"},
			testArgs:            map[string]any{"proxy_id": testProxyID},
			expectedMethod:      "GetMCPProxy",
			validateCall: func(t *testing.T, args []interface{}) {
				if got := args[1]; got != testProxyID {
					t.Errorf("proxyID: got %v, want %q", got, testProxyID)
				}
			},
		},
		{
			name:                "create_mcp_proxy",
			toolset:             "mcp-proxy",
			permissions:         []rbac.Permission{rbac.MCPServerCreate, rbac.MCPServerConnect},
			descriptionKeywords: []string{"mcp", "proxy", "gateway"},
			descriptionMinLen:   20,
			requiredParams:      []string{"name", "url", "environments"},
			optionalParams:      []string{"auth_header", "auth_value", "gateway_id", "description", "version"},
			testArgs: map[string]any{
				"name":         "Docs Search",
				"url":          "https://mcp.example.com/mcp",
				"environments": []any{testEnvName},
				"auth_header":  "Authorization",
				"auth_value":   "Bearer t",
			},
			expectedMethod: "CreateMCPProxy",
			validateCall: func(t *testing.T, args []interface{}) {
				dto, ok := args[1].(*models.MCPProxyDTO)
				if !ok {
					t.Fatalf("request: got %T", args[1])
				}
				if dto.ID != "docs-search" || dto.Version != defaultMCPProxyVersion || len(dto.Endpoints) != 1 {
					t.Fatalf("proxy: got id=%q version=%q endpoints=%d", dto.ID, dto.Version, len(dto.Endpoints))
				}
				ep := dto.Endpoints[0]
				if ep.Upstream.Main == nil || ep.Upstream.Main.URL != "https://mcp.example.com/mcp" || ep.Upstream.Main.Auth == nil {
					t.Errorf("upstream: got %+v", ep.Upstream.Main)
				}
				if len(ep.Environments) != 1 || ep.Environments[0].EnvironmentUUID != testEnvUUID {
					t.Errorf("environments: got %+v", ep.Environments)
				}
				if ep.Capabilities == nil || ep.Capabilities.Tools == nil || len(*ep.Capabilities.Tools) != 1 {
					t.Errorf("capabilities not taken from the server probe: %+v", ep.Capabilities)
				}
			},
		},
	}
}

// Returns the test specs for tools registered by registerAgentConfigTools.
func agentConfigToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "attach_llm_provider_to_agent",
			toolset:             "agent-config",
			permissions:         []rbac.Permission{rbac.AgentUpdate},
			descriptionKeywords: []string{"agent", "llm provider", "guardrail"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name", "provider"},
			optionalParams:      []string{"config_name", "environments", "guardrails"},
			testArgs: map[string]any{
				"project_name": testProjectName,
				"agent_name":   testAgentName,
				"provider":     testProviderID,
				"guardrails":   []any{map[string]any{"name": testGuardrailName, "version": "v1"}},
			},
			expectedMethod: "CreateAgentConfig",
			validateCall: func(t *testing.T, args []interface{}) {
				if args[1] != testProjectName || args[2] != testAgentName {
					t.Errorf("project/agent: got %v/%v", args[1], args[2])
				}
				req, ok := args[3].(models.CreateAgentModelConfigRequest)
				if !ok {
					t.Fatalf("request: got %T", args[3])
				}
				if req.Type != models.AgentConfigTypeLLM || req.Name != testProviderID {
					t.Errorf("type/name: got %q/%q", req.Type, req.Name)
				}
				m, ok := req.EnvMappings[testEnvName]
				if !ok || m.ProviderName != testProviderID {
					t.Fatalf("env mapping not defaulted to every environment: %+v", req.EnvMappings)
				}
				if len(m.Configuration.Policies) != 1 || m.Configuration.Policies[0].Name != testGuardrailName {
					t.Errorf("guardrails: got %+v", m.Configuration.Policies)
				}
			},
		},
		{
			name:                "attach_mcp_proxy_to_agent",
			toolset:             "agent-config",
			permissions:         []rbac.Permission{rbac.AgentUpdate},
			descriptionKeywords: []string{"agent", "mcp proxy"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name", "proxy"},
			optionalParams:      []string{"config_name", "environments"},
			testArgs: map[string]any{
				"project_name": testProjectName,
				"agent_name":   testAgentName,
				"proxy":        testProxyID,
				"environments": []any{testEnvName},
			},
			expectedMethod: "CreateAgentConfig",
			validateCall: func(t *testing.T, args []interface{}) {
				req, ok := args[3].(models.CreateAgentModelConfigRequest)
				if !ok {
					t.Fatalf("request: got %T", args[3])
				}
				if req.Type != models.AgentConfigTypeMCP || req.EnvMappings[testEnvName].ProviderName != testProxyID {
					t.Errorf("mcp config: got %+v", req)
				}
			},
		},
	}
}

// The detail tools read records that hold upstream credentials. They must
// report that auth is configured without ever returning the credential.
func TestDetailToolsNeverReturnCredentials(t *testing.T) {
	session, _ := setupTestServer(t)
	cases := []struct {
		tool string
		args map[string]any
		want []string
	}{
		{"get_llm_provider", map[string]any{"provider_id": testProviderID},
			[]string{`"upstream_url":"https://api.openai.com/v1"`, `"upstream_auth_configured":true`, testGuardrailName, `"status":"DEPLOYED"`}},
		{"get_mcp_proxy", map[string]any{"proxy_id": testProxyID},
			[]string{`"upstream_auth_configured":true`, `"tools":["search"]`, `"name":"` + testEnvName + `"`}},
	}
	for _, tc := range cases {
		t.Run(tc.tool, func(t *testing.T) {
			res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: tc.tool, Arguments: tc.args})
			if err != nil {
				t.Fatalf("CallTool: %v", err)
			}
			if res.IsError {
				t.Fatalf("tool returned an error: %+v", res.Content)
			}
			text := res.Content[0].(*gomcp.TextContent).Text
			if strings.Contains(text, "ARCHIVED") {
				t.Errorf("output lists archived deployments: %s", text)
			}
			if strings.Contains(text, "sk-must-never-be-returned") {
				t.Fatalf("output leaks the upstream credential: %s", text)
			}
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("output missing %s: %s", w, text)
				}
			}
		})
	}
}

// Removing a guardrail the provider does not have is an error, not a silent
// no-op redeploy.
func TestRemoveMissingGuardrailIsAnError(t *testing.T) {
	session, mock := setupTestServer(t)
	res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name:      "remove_llm_provider_guardrail",
		Arguments: map[string]any{"provider_id": testProviderID, "name": "not-there"},
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if !res.IsError {
		t.Fatal("expected an error for a guardrail the provider does not have")
	}
	if _, called := mock.calls["SetLLMProviderGuardrails"]; called {
		t.Error("provider was saved and redeployed for a no-op removal")
	}
}

// The guardrail list hides gateway policies that are not guardrails and leaves
// out parameter schemas, which together run past what clients pass to the
// model; a lookup by name returns the one schema.
func TestListLLMGuardrailsFiltersAndReturnsSchemaByName(t *testing.T) {
	session, _ := setupTestServer(t)
	call := func(args map[string]any) (bool, string) {
		t.Helper()
		res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: "list_llm_guardrails", Arguments: args})
		if err != nil {
			t.Fatalf("CallTool: %v", err)
		}
		return res.IsError, res.Content[0].(*gomcp.TextContent).Text
	}

	isErr, text := call(map[string]any{})
	if isErr || !strings.Contains(text, testGuardrailName) {
		t.Fatalf("list: got %s", text)
	}
	if strings.Contains(text, `"cors"`) {
		t.Errorf("list includes a non-guardrail policy: %s", text)
	}
	if strings.Contains(text, `"parameters"`) {
		t.Errorf("list includes parameter schemas: %s", text)
	}

	isErr, text = call(map[string]any{"name": testGuardrailName})
	if isErr || !strings.Contains(text, `"parameters"`) || !strings.Contains(text, `"mask"`) {
		t.Errorf("lookup by name: got %s", text)
	}

	if isErr, _ = call(map[string]any{"name": "cors"}); !isErr {
		t.Error("lookup of a non-guardrail policy by name succeeded")
	}
}
