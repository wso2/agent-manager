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
	"fmt"
	"sort"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/models"
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
	session := setupTestServerWithToolsets(t, &Toolsets{AgentConfigToolset: mock, EnvironmentToolset: mock})
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
	if got := mappedEnvs(t, mock.MockToolsetHandler); len(got) != 1 || got[0] != "dev" {
		t.Errorf("mapped environments %v, want [dev]", got)
	}
	if _, listed := mock.calls["ListEnvironments"]; listed {
		t.Error("listed every org environment instead of the provider's")
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
	if len(res.Tools) < 20 {
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
