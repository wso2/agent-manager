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
	"errors"
	"fmt"
	"sort"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// pagedEnvMock serves `envs` a page at a time and reports `total`, which a
// misbehaving upstream may set higher than what it actually returns.
type pagedEnvMock struct {
	*MockToolsetHandler
	envs  []string
	total int32
	pages int
}

func (m *pagedEnvMock) ListEnvironments(ctx context.Context, ouID string, limit int32, offset int32) (*models.EnvironmentListResponse, error) {
	m.pages++
	start, end := int(offset), int(offset)+int(limit)
	if start > len(m.envs) {
		start = len(m.envs)
	}
	if end > len(m.envs) {
		end = len(m.envs)
	}
	page := []models.GatewayEnvironmentResponse{}
	for _, n := range m.envs[start:end] {
		page = append(page, models.GatewayEnvironmentResponse{Name: n, UUID: "uuid-" + n})
	}
	return &models.EnvironmentListResponse{Environments: page, Total: m.total, Limit: limit, Offset: offset}, nil
}

func TestResolveEnvironmentNamesReadsEveryPage(t *testing.T) {
	envs := make([]string, 250)
	for i := range envs {
		envs[i] = fmt.Sprintf("env-%d", i)
	}
	m := &pagedEnvMock{MockToolsetHandler: NewMockToolsetHandler(), envs: envs, total: int32(len(envs))}

	got, err := resolveEnvironmentNames(context.Background(), "test_tool", m, "ou", nil)
	if err != nil {
		t.Fatalf("resolveEnvironmentNames: %v", err)
	}
	if len(got) != len(envs) || got[0] != envs[0] || got[len(got)-1] != envs[len(envs)-1] {
		t.Fatalf("got %d environments (%v ... %v), want all %d", len(got), got[0], got[len(got)-1], len(envs))
	}
	if m.pages != 3 {
		t.Errorf("listed %d pages, want 3", m.pages)
	}
}

func TestResolveEnvironmentNamesStopsOnShortUpstream(t *testing.T) {
	// Total claims more than the upstream ever returns; the loop must end on
	// the empty page instead of spinning.
	m := &pagedEnvMock{MockToolsetHandler: NewMockToolsetHandler(), envs: []string{"dev", "prod"}, total: 500}

	got, err := resolveEnvironmentNames(context.Background(), "test_tool", m, "ou", nil)
	if err != nil {
		t.Fatalf("resolveEnvironmentNames: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %v, want [dev prod]", got)
	}
}

func TestResolveEnvironmentNamesErrorsWhenNoneConfigured(t *testing.T) {
	m := &pagedEnvMock{MockToolsetHandler: NewMockToolsetHandler()}

	if _, err := resolveEnvironmentNames(context.Background(), "test_tool", m, "ou", nil); err == nil {
		t.Fatal("want an error when the organization has no environments")
	}
}

// providerEnvsMock reports the provider usable in envs only, as for a provider
// deployed to some of the org's environments.
type providerEnvsMock struct {
	*MockToolsetHandler
	envs []string
}

func (m providerEnvsMock) ProviderEnvironments(ctx context.Context, ouID, projectName, agentName, providerHandle string) ([]string, error) {
	m.recordCall("ProviderEnvironments", ouID, projectName, agentName, providerHandle)
	return m.envs, nil
}

func callAttach(t *testing.T, mock providerEnvsMock, tool string, args map[string]any) (bool, string) {
	t.Helper()
	session := setupTestServerWithToolsets(t, &Toolsets{AgentConfigToolset: mock, EnvironmentToolset: mock, LLMProviderToolset: mock})
	args["project_name"], args["agent_name"] = testProjectName, testAgentName
	res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	return res.IsError, res.Content[0].(*gomcp.TextContent).Text
}

func mappedEnvs(t *testing.T, m *MockToolsetHandler) []string {
	t.Helper()
	calls := m.calls["CreateAgentConfig"]
	if len(calls) != 1 {
		t.Fatalf("CreateAgentConfig called %d times, want 1", len(calls))
	}
	req := calls[0].([]interface{})[3].(models.CreateAgentModelConfigRequest)
	envs := make([]string, 0, len(req.EnvMappings))
	for env := range req.EnvMappings {
		envs = append(envs, env)
	}
	sort.Strings(envs)
	return envs
}

// Create refuses an LLM config on any environment where the provider is not
// placed, so the default must be the provider's environments, not the org's.
func TestAttachLLMProviderDefaultsToProviderEnvironments(t *testing.T) {
	mock := providerEnvsMock{MockToolsetHandler: NewMockToolsetHandler(), envs: []string{"dev"}}
	failed, text := callAttach(t, mock, "attach_llm_provider_to_agent", map[string]any{"provider_id": testProviderID})
	if failed {
		t.Fatalf("tool failed: %s", text)
	}
	// The org's environments may still be listed, for the production tier
	// check; what matters is that only the provider's are mapped.
	if got := mappedEnvs(t, mock.MockToolsetHandler); len(got) != 1 || got[0] != "dev" {
		t.Errorf("mapped environments %v, want [dev]", got)
	}
}

func TestAttachLLMProviderFailsUpFrontWhenProviderIsNowhere(t *testing.T) {
	mock := providerEnvsMock{MockToolsetHandler: NewMockToolsetHandler(), envs: []string{}}
	failed, text := callAttach(t, mock, "attach_llm_provider_to_agent", map[string]any{"provider_id": testProviderID})
	if !failed {
		t.Fatalf("want an error for a provider deployed nowhere the agent can use, got %s", text)
	}
	if _, created := mock.calls["CreateAgentConfig"]; created {
		t.Error("created a config for a provider with no usable environment")
	}
}

func TestAttachLLMProviderUsesExplicitEnvironments(t *testing.T) {
	mock := providerEnvsMock{MockToolsetHandler: NewMockToolsetHandler(), envs: []string{"dev"}}
	failed, text := callAttach(t, mock, "attach_llm_provider_to_agent", map[string]any{"provider_id": testProviderID, "environments": []any{testEnvName}})
	if failed {
		t.Fatalf("tool failed: %s", text)
	}
	if got := mappedEnvs(t, mock.MockToolsetHandler); len(got) != 1 || got[0] != testEnvName {
		t.Errorf("mapped environments %v, want [%s]", got, testEnvName)
	}
	if _, asked := mock.calls["ProviderEnvironments"]; asked {
		t.Error("explicit environments were replaced by the provider's")
	}
}

// The MCP proxy create tolerates environments the proxy is not deployed to,
// so its default stays every environment.
func TestAttachMCPProxyStillDefaultsToEveryEnvironment(t *testing.T) {
	mock := providerEnvsMock{MockToolsetHandler: NewMockToolsetHandler(), envs: []string{"dev"}}
	failed, text := callAttach(t, mock, "attach_mcp_proxy_to_agent", map[string]any{"proxy_id": testProxyID})
	if failed {
		t.Fatalf("tool failed: %s", text)
	}
	if got := mappedEnvs(t, mock.MockToolsetHandler); len(got) != 1 || got[0] != testEnvName {
		t.Errorf("mapped environments %v, want every org environment [%s]", got, testEnvName)
	}
	if _, asked := mock.calls["ProviderEnvironments"]; asked {
		t.Error("MCP proxy attach used the LLM provider placement default")
	}
}

// The response lists the guardrails as they were saved, so names that were
// trimmed before saving come back trimmed.
func TestAttachLLMProviderEchoesSavedGuardrailNames(t *testing.T) {
	mock := providerEnvsMock{MockToolsetHandler: NewMockToolsetHandler(), envs: []string{"dev"}}
	failed, text := callAttach(t, mock, "attach_llm_provider_to_agent", map[string]any{
		"provider_id": testProviderID,
		"guardrails":  []any{map[string]any{"name": "  " + testGuardrailName + " ", "version": "v1"}},
	})
	if failed {
		t.Fatalf("tool failed: %s", text)
	}
	if want := `"guardrails":["` + testGuardrailName + `"]`; !strings.Contains(text, want) {
		t.Errorf("response should list the saved name %s: %s", want, text)
	}
}

// get_mcp_proxy and get_llm_provider name environments through this map, so
// environments past the first page must be named too.
func TestEnvironmentNamesByUUIDReadsEveryPage(t *testing.T) {
	envs := make([]string, 150)
	for i := range envs {
		envs[i] = fmt.Sprintf("env-%d", i)
	}
	m := &pagedEnvMock{MockToolsetHandler: NewMockToolsetHandler(), envs: envs, total: int32(len(envs))}

	names := environmentNamesByUUID(context.Background(), m, "ou")

	if len(names) != len(envs) || names["uuid-env-149"] != "env-149" {
		t.Fatalf("named %d of %d environments (uuid-env-149 -> %q)", len(names), len(envs), names["uuid-env-149"])
	}
}

// An argument a tool does not declare, misspelled or under an old name, is an
// error the model can correct rather than dropped for the default to apply.
func TestAttachRejectsUndeclaredArguments(t *testing.T) {
	for name, args := range map[string]map[string]any{
		"misspelled environments": {"provider_id": testProviderID, "environment": testEnvName},
		"old provider name":       {"provider": testProviderID},
	} {
		t.Run(name, func(t *testing.T) {
			mock := providerEnvsMock{MockToolsetHandler: NewMockToolsetHandler(), envs: []string{"dev"}}
			failed, text := callAttach(t, mock, "attach_llm_provider_to_agent", args)
			if !failed {
				t.Fatalf("an undeclared argument was accepted: %s", text)
			}
			if _, created := mock.calls["CreateAgentConfig"]; created {
				t.Error("created a config despite the undeclared argument")
			}
		})
	}
}

// Every tool's input schema refuses properties it does not declare.
func TestEveryToolSchemaRefusesUndeclaredProperties(t *testing.T) {
	session, _ := setupTestServer(t)
	res, err := session.ListTools(context.Background(), &gomcp.ListToolsParams{})
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(res.Tools) < 15 {
		t.Fatalf("listed %d tools, want every registered tool", len(res.Tools))
	}
	for _, tool := range res.Tools {
		schema, ok := tool.InputSchema.(map[string]any)
		if !ok {
			t.Errorf("%s: input schema is %T", tool.Name, tool.InputSchema)
			continue
		}
		if schema["additionalProperties"] != false {
			t.Errorf("%s: input schema allows undeclared properties", tool.Name)
		}
	}
}

// agentKindMock serves the agent as the given provisioning type, or fails the lookup.
type agentKindMock struct {
	providerEnvsMock
	provisioning string
	err          error
}

func (m agentKindMock) GetAgent(_ context.Context, _, _, agentName string) (*models.AgentResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &models.AgentResponse{Name: agentName, Provisioning: models.Provisioning{Type: m.provisioning}}, nil
}

func callAttachAs(t *testing.T, mock agentKindMock, tool string, args map[string]any) (bool, string) {
	t.Helper()
	session := setupTestServerWithToolsets(t, &Toolsets{AgentConfigToolset: mock, EnvironmentToolset: mock, AgentToolset: mock})
	args["project_name"], args["agent_name"] = testProjectName, testAgentName
	res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	return res.IsError, res.Content[0].(*gomcp.TextContent).Text
}

// An external agent has nothing to build or deploy, and its URL and key are
// withheld, so its note says where in the console to get them.
func TestAttachNoteFollowsTheAgentKind(t *testing.T) {
	newMock := func(provisioning string) agentKindMock {
		return agentKindMock{providerEnvsMock: providerEnvsMock{MockToolsetHandler: NewMockToolsetHandler(), envs: []string{"dev"}}, provisioning: provisioning}
	}
	llm := map[string]any{"provider_id": testProviderID}
	mcp := map[string]any{"proxy_id": testProxyID}
	cases := []struct {
		name, tool, provisioning string
		args                     map[string]any
		want, notWant            []string
	}{
		{
			"platform agent", "attach_llm_provider_to_agent", string(utils.InternalAgent), llm,
			[]string{"restart it"},
			[]string{"external agent", "deploy_agent"},
		},
		{
			"external agent, LLM", "attach_llm_provider_to_agent", string(utils.ExternalAgent), llm,
			[]string{"external agent", `Connect to LLM Provider`, "revoked", "generates a key"},
			[]string{"deploy_agent", "build_agent"},
		},
		{
			"external agent, MCP", "attach_mcp_proxy_to_agent", string(utils.ExternalAgent), mcp,
			[]string{"external agent", `Connect to MCP Server`, "API key security"},
			[]string{"deploy_agent", "build_agent"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			failed, text := callAttachAs(t, newMock(tc.provisioning), tc.tool, tc.args)
			if failed {
				t.Fatalf("tool failed: %s", text)
			}
			for _, w := range tc.want {
				if !strings.Contains(text, w) {
					t.Errorf("note missing %q: %s", w, text)
				}
			}
			for _, nw := range tc.notWant {
				if strings.Contains(text, nw) {
					t.Errorf("note should not mention %q: %s", nw, text)
				}
			}
		})
	}
}

// The agent is read before the create, so a failed lookup leaves no config.
func TestAttachFailsBeforeCreateWhenTheAgentLookupFails(t *testing.T) {
	mock := agentKindMock{
		providerEnvsMock: providerEnvsMock{MockToolsetHandler: NewMockToolsetHandler(), envs: []string{"dev"}},
		err:              utils.ErrAgentNotFound,
	}
	failed, text := callAttachAs(t, mock, "attach_llm_provider_to_agent", map[string]any{"provider_id": testProviderID})
	if !failed {
		t.Fatalf("want an error for an agent that cannot be read, got: %s", text)
	}
	if _, created := mock.calls["CreateAgentConfig"]; created {
		t.Error("created a config for an agent that could not be read")
	}
}

// tierMock has a non-production dev and a production prod, with the provider
// placed in both.
type tierMock struct{ *MockToolsetHandler }

func (tierMock) ListEnvironments(_ context.Context, _ string, _, _ int32) (*models.EnvironmentListResponse, error) {
	return &models.EnvironmentListResponse{Environments: []models.GatewayEnvironmentResponse{
		{Name: "dev"}, {Name: "prod", IsProduction: true},
	}, Total: 2}, nil
}

func (m tierMock) ProviderEnvironments(context.Context, string, string, string, string) ([]string, error) {
	return []string{"dev", "prod"}, nil
}

// callAttachWithScopes runs an attach with a token holding scopes beyond the
// tools' own, as the console agent's token does.
func callAttachWithScopes(t *testing.T, mock tierMock, extraScopes string, args map[string]any) (bool, string) {
	t.Helper()
	return callToolWithScopes(t, mock, "attach_llm_provider_to_agent", extraScopes, args)
}

func callToolWithScopes(t *testing.T, mock tierMock, tool, extraScopes string, args map[string]any) (bool, string) {
	t.Helper()
	session := setupTestServerWithClaims(t, &Toolsets{AgentConfigToolset: mock, EnvironmentToolset: mock}, &jwtassertion.TokenClaims{
		OuId: testOrgName, Scope: strings.TrimSpace(unionScopes() + " " + extraScopes),
	})
	args["project_name"], args["agent_name"] = testProjectName, testAgentName
	res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	return res.IsError, res.Content[0].(*gomcp.TextContent).Text
}

// An attach that reaches a production environment needs the same
// amp:agent:env-production scope the deploy tools need there.
func TestAttachProductionEnvironmentNeedsTheProductionScope(t *testing.T) {
	prodScope := rbac.AgentEnvProduction.Scope()

	t.Run("named production environment is refused without the scope", func(t *testing.T) {
		mock := tierMock{NewMockToolsetHandler()}
		failed, text := callAttachWithScopes(t, mock, "", map[string]any{"provider_id": testProviderID, "environments": []any{"dev", "prod"}})
		if !failed || !strings.Contains(text, prodScope) || !strings.Contains(text, "prod") {
			t.Fatalf("want a refusal naming %s and prod, got failed=%v: %s", prodScope, failed, text)
		}
		if _, created := mock.calls["CreateAgentConfig"]; created {
			t.Error("created a config in production without the production scope")
		}
	})

	t.Run("default selection leaves production out without the scope", func(t *testing.T) {
		mock := tierMock{NewMockToolsetHandler()}
		failed, text := callAttachWithScopes(t, mock, "", map[string]any{"provider_id": testProviderID})
		if failed {
			t.Fatalf("tool failed: %s", text)
		}
		if got := mappedEnvs(t, mock.MockToolsetHandler); len(got) != 1 || got[0] != "dev" {
			t.Errorf("mapped %v, want [dev]", got)
		}
		if !strings.Contains(text, `"skipped_production_environments":["prod"]`) {
			t.Errorf("output should say prod was left out: %s", text)
		}
	})

	t.Run("production is allowed with the scope", func(t *testing.T) {
		mock := tierMock{NewMockToolsetHandler()}
		failed, text := callAttachWithScopes(t, mock, prodScope, map[string]any{"provider_id": testProviderID, "environments": []any{"dev", "prod"}})
		if failed {
			t.Fatalf("tool failed: %s", text)
		}
		if got := mappedEnvs(t, mock.MockToolsetHandler); len(got) != 2 {
			t.Errorf("mapped %v, want [dev prod]", got)
		}
	})
}

// The output names each environment's per-agent proxy, never its key, so a
// client can point the user at the proxy whose key to manage.
func TestAttachLLMProviderListsItsProxies(t *testing.T) {
	mock := providerEnvsMock{MockToolsetHandler: NewMockToolsetHandler(), envs: []string{"dev"}}
	failed, text := callAttach(t, mock, "attach_llm_provider_to_agent", map[string]any{"provider_id": testProviderID})
	if failed {
		t.Fatalf("tool failed: %s", text)
	}
	if want := `"proxies":[{"environment":"dev","proxy_id":"` + testProviderID + `-dev-proxy"}]`; !strings.Contains(text, want) {
		t.Errorf("output does not name the proxy\n got: %s\nwant: %s", text, want)
	}
}

// A credential in guardrail params would pass through the model, so it is
// refused before anything is saved; ordinary parameters pass.
func TestGuardrailCredentialParamsAreRefused(t *testing.T) {
	for name, params := range map[string]map[string]any{
		"api key":           {"apiKey": "sk-123"},
		"nested secret":     {"request": map[string]any{"clientSecret": "s"}},
		"subscription key":  {"azure_subscription_key": "k"},
		"bare token":        {"token": "t"},
		"key in list items": {"rules": []any{map[string]any{"password": "p"}}},
	} {
		t.Run(name, func(t *testing.T) {
			if credentialParam(params, "") == "" {
				t.Errorf("%v was not recognised as a credential", params)
			}
		})
	}
	for name, params := range map[string]map[string]any{
		"thresholds":      {"request": map[string]any{"jsonPath": "", "hateSeverityThreshold": 4}},
		"keywords":        {"keywords": []any{"a"}, "maxTokens": 100, "tokenLimit": 5},
		"non-secret keys": {"cacheKey": "x", "sortKey": "y"},
		"masking":         {"mask": true},
		"no params":       nil,
	} {
		t.Run("allowed "+name, func(t *testing.T) {
			if p := credentialParam(params, ""); p != "" {
				t.Errorf("%q was refused as a credential", p)
			}
		})
	}

	failed, text, saved := callGuardrailTool(t, nil, "add_llm_provider_guardrails", map[string]any{
		"guardrails": []any{map[string]any{"name": "vendor-check", "version": "v1", "params": map[string]any{"apiKey": "sk-123"}}},
	})
	if !failed || saved != nil || !strings.Contains(text, `"apiKey"`) || strings.Contains(text, "sk-123") {
		t.Fatalf("want a refusal naming the parameter but not its value, and no save; got failed=%v saved=%v: %s", failed, saved, text)
	}

	mock := providerEnvsMock{MockToolsetHandler: NewMockToolsetHandler(), envs: []string{"dev"}}
	failed, text = callAttach(t, mock, "attach_llm_provider_to_agent", map[string]any{
		"provider_id": testProviderID,
		"guardrails":  []any{map[string]any{"name": "vendor-check", "version": "v1", "params": map[string]any{"password": "p"}}},
	})
	if !failed {
		t.Fatalf("attach accepted a credential guardrail parameter: %s", text)
	}
	if _, created := mock.calls["CreateAgentConfig"]; created {
		t.Error("created a config with a credential guardrail parameter")
	}
}

// failingRevokeMock fails to revoke the key in one environment.
type failingRevokeMock struct {
	agentKindMock
	failEnv string
}

func (m failingRevokeMock) RevokeConfigAPIKeys(ctx context.Context, ouID, projectName, agentName, configType, configID, envName string) error {
	m.recordCall("RevokeConfigAPIKeys", ouID, projectName, agentName, configType, configID, envName)
	if envName == m.failEnv {
		return errors.New("gateway unreachable")
	}
	return nil
}

// The key the create issues for an external agent is withheld from the chat,
// so it is revoked instead of being left valid with nobody holding it.
func TestAttachRevokesTheWithheldKeyOfAnExternalAgent(t *testing.T) {
	newMock := func(provisioning string, envs ...string) agentKindMock {
		return agentKindMock{providerEnvsMock: providerEnvsMock{MockToolsetHandler: NewMockToolsetHandler(), envs: envs}, provisioning: provisioning}
	}

	t.Run("external agent: every environment's key is revoked", func(t *testing.T) {
		mock := newMock(string(utils.ExternalAgent), "dev", "staging")
		if failed, text := callAttachAs(t, mock, "attach_llm_provider_to_agent", map[string]any{"provider_id": testProviderID}); failed {
			t.Fatalf("tool failed: %s", text)
		}
		var revoked []string
		for _, call := range mock.calls["RevokeConfigAPIKeys"] {
			args := call.([]interface{})
			// ouID, project, agent, config type, config id, environment
			if args[3] != models.AgentConfigTypeLLM || args[4] != "config-uuid" {
				t.Errorf("revoked the wrong config: %v", args)
			}
			revoked = append(revoked, args[5].(string))
		}
		sort.Strings(revoked)
		if strings.Join(revoked, ",") != "dev,staging" {
			t.Errorf("revoked keys in %v, want [dev staging]", revoked)
		}
	})

	t.Run("platform agent: nothing is revoked", func(t *testing.T) {
		mock := newMock(string(utils.InternalAgent), "dev")
		if failed, text := callAttachAs(t, mock, "attach_llm_provider_to_agent", map[string]any{"provider_id": testProviderID}); failed {
			t.Fatalf("tool failed: %s", text)
		}
		if _, revoked := mock.calls["RevokeConfigAPIKeys"]; revoked {
			t.Error("revoked the injected key of a platform-hosted agent")
		}
	})

	t.Run("a failed revoke is reported, not fatal", func(t *testing.T) {
		mock := failingRevokeMock{agentKindMock: newMock(string(utils.ExternalAgent), "dev", "staging"), failEnv: "staging"}
		session := setupTestServerWithToolsets(t, &Toolsets{AgentConfigToolset: mock, EnvironmentToolset: mock, AgentToolset: mock})
		res, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: "attach_llm_provider_to_agent", Arguments: map[string]any{
			"project_name": testProjectName, "agent_name": testAgentName, "provider_id": testProviderID,
		}})
		if err != nil || res.IsError {
			t.Fatalf("the config was created, so the attach should succeed: err=%v %+v", err, res.Content)
		}
		text := res.Content[0].(*gomcp.TextContent).Text
		if !strings.Contains(text, `"unrevoked_key_environments":["staging"]`) || !strings.Contains(text, "regenerate it there") {
			t.Errorf("output should name staging as unrevoked and say to regenerate: %s", text)
		}
	})
}

// The attach's guardrails are checked against the provider's gateways too,
// before the config is created.
func TestAttachGuardrailsMustBeOnesTheGatewaysReport(t *testing.T) {
	mock := providerEnvsMock{MockToolsetHandler: NewMockToolsetHandler(), envs: []string{"dev"}}
	failed, text := callAttach(t, mock, "attach_llm_provider_to_agent", map[string]any{
		"provider_id": testProviderID,
		"guardrails":  []any{map[string]any{"name": "cors", "version": "v1"}},
	})
	if !failed || !strings.Contains(text, "not a guardrail") {
		t.Fatalf("want cors refused as not a guardrail, got failed=%v: %s", failed, text)
	}
	if _, created := mock.calls["CreateAgentConfig"]; created {
		t.Error("created a config with a policy that is not a guardrail")
	}
}

// attach_mcp_proxy_to_agent shares the tier: its default, every environment,
// leaves production out, and a named production environment is refused.
func TestAttachMCPProxyProductionEnvironmentNeedsTheProductionScope(t *testing.T) {
	mock := tierMock{NewMockToolsetHandler()}
	failed, text := callToolWithScopes(t, mock, "attach_mcp_proxy_to_agent", "", map[string]any{"proxy_id": testProxyID})
	if failed {
		t.Fatalf("tool failed: %s", text)
	}
	if got := mappedEnvs(t, mock.MockToolsetHandler); len(got) != 1 || got[0] != "dev" {
		t.Errorf("mapped %v, want [dev]", got)
	}
	if !strings.Contains(text, `"skipped_production_environments":["prod"]`) {
		t.Errorf("output should say prod was left out: %s", text)
	}

	mock = tierMock{NewMockToolsetHandler()}
	failed, text = callToolWithScopes(t, mock, "attach_mcp_proxy_to_agent", "", map[string]any{"proxy_id": testProxyID, "environments": []any{"prod"}})
	if !failed || !strings.Contains(text, rbac.AgentEnvProduction.Scope()) {
		t.Fatalf("want a refusal naming %s, got failed=%v: %s", rbac.AgentEnvProduction.Scope(), failed, text)
	}
	if _, created := mock.calls["CreateAgentConfig"]; created {
		t.Error("created an MCP config in production without the production scope")
	}
}
