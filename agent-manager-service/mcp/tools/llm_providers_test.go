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
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

func callTool(t *testing.T, session *gomcp.ClientSession, name string, args map[string]any) (*gomcp.CallToolResult, string) {
	t.Helper()
	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool %s failed: %v", name, err)
	}
	if len(result.Content) == 0 {
		t.Fatalf("%s returned no content", name)
	}
	text, ok := result.Content[0].(*gomcp.TextContent)
	if !ok {
		t.Fatalf("%s: expected *gomcp.TextContent, got %T", name, result.Content[0])
	}
	return result, text.Text
}

func lastAttachRequest(t *testing.T, mock *MockToolsetHandler) models.CreateAgentModelConfigRequest {
	t.Helper()
	calls := mock.calls["AttachLLMProvider"]
	if len(calls) == 0 {
		t.Fatal("AttachLLMProvider was not called")
	}
	args, ok := calls[len(calls)-1].([]interface{})
	if !ok {
		t.Fatalf("recorded args have unexpected type %T", calls[len(calls)-1])
	}
	req, ok := args[3].(models.CreateAgentModelConfigRequest)
	if !ok {
		t.Fatalf("request has unexpected type %T", args[3])
	}
	return req
}

func attachArgs(extra map[string]any) map[string]any {
	args := map[string]any{
		"project_name": testProjectName,
		"agent_name":   testAgentName,
		"provider_id":  testProviderID,
	}
	for k, v := range extra {
		args[k] = v
	}
	return args
}

func TestListLLMProvidersOmitsUpstreamCredentials(t *testing.T) {
	session, _ := setupTestServer(t)
	result, text := callTool(t, session, "list_llm_providers", map[string]any{})
	if result.IsError {
		t.Fatalf("expected success, got %s", text)
	}
	if strings.Contains(text, testUpstreamSecret) || strings.Contains(text, "api.openai.example") {
		t.Fatalf("list_llm_providers leaked upstream configuration: %s", text)
	}
	var out listLLMProvidersOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	want := llmProviderItem{ID: testProviderID, Name: "Test Provider", Template: "openai", Status: "deployed", InCatalog: true}
	if out.Total != 1 || len(out.Providers) != 1 || out.Providers[0] != want {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestAddLLMProviderToCatalogReportsChange(t *testing.T) {
	session, _ := setupTestServer(t)
	_, text := callTool(t, session, "add_llm_provider_to_catalog", map[string]any{"provider_id": testProviderID})
	var out addLLMProviderToCatalogOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("invalid JSON: %v (%s)", err, text)
	}
	if out.ID != testProviderID || !out.InCatalog || !out.Changed {
		t.Fatalf("unexpected output: %+v", out)
	}
}

func TestAddLLMProviderToCatalogUnknownProvider(t *testing.T) {
	session, _ := setupTestServer(t)
	result, text := callTool(t, session, "add_llm_provider_to_catalog", map[string]any{"provider_id": "nope"})
	if !result.IsError {
		t.Fatalf("expected error result, got %s", text)
	}
	if !strings.Contains(text, "list_llm_providers") {
		t.Errorf("error should point to list_llm_providers: %s", text)
	}
}

func TestAttachLLMProviderDefaults(t *testing.T) {
	session, mock := setupTestServer(t)
	result, text := callTool(t, session, "attach_llm_provider", attachArgs(nil))
	if result.IsError {
		t.Fatalf("expected success, got %s", text)
	}
	req := lastAttachRequest(t, mock)
	if req.Name != testProviderID {
		t.Errorf("config name: got %q, want provider id %q", req.Name, testProviderID)
	}
	if len(req.EnvMappings) != 1 {
		t.Errorf("env mappings: got %v, want only %q", req.EnvMappings, testEnvName)
	}
	if req.EnvironmentVariables != nil {
		t.Errorf("environment variables: got %v, want none", req.EnvironmentVariables)
	}
}

func TestAttachLLMProviderNeverReturnsAPIKey(t *testing.T) {
	session, _ := setupTestServer(t)
	_, text := callTool(t, session, "attach_llm_provider", attachArgs(nil))
	if strings.Contains(text, testProxyAPIKey) {
		t.Fatalf("attach_llm_provider leaked the proxy API key: %s", text)
	}
	var out attachLLMProviderOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(out.Environments) != 1 || !out.Environments[0].APIKeyIssued || out.Environments[0].ProxyURL == "" {
		t.Fatalf("unexpected environments: %+v", out.Environments)
	}
	if out.Note == "" {
		t.Error("expected a note that tells the user how to get an API key")
	}
}

func TestAttachLLMProviderPlatformAgentKeyIsInjected(t *testing.T) {
	session, mock := setupTestServer(t)
	mock.platformAgent = true
	_, text := callTool(t, session, "attach_llm_provider", attachArgs(nil))
	var out attachLLMProviderOutput
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatalf("invalid JSON: %v (%s)", err, text)
	}
	if len(out.Environments) != 1 || !out.Environments[0].APIKeyIssued {
		t.Fatalf("a platform agent gets a key in every environment: %+v", out.Environments)
	}
	if out.Note != "" {
		t.Errorf("a platform agent's key is injected, so no note is expected: %q", out.Note)
	}
}

func TestAttachLLMProviderExplicitInputs(t *testing.T) {
	session, mock := setupTestServer(t)
	result, text := callTool(t, session, "attach_llm_provider", attachArgs(map[string]any{
		"environments":    []any{"prod", "dev", "prod"},
		"config_name":     "Main LLM",
		"url_env_var":     "OPENAI_BASE_URL",
		"api_key_env_var": "OPENAI_API_KEY",
	}))
	if result.IsError {
		t.Fatalf("expected success, got %s", text)
	}
	req := lastAttachRequest(t, mock)
	if req.Name != "Main LLM" {
		t.Errorf("config name: got %q", req.Name)
	}
	envs := make([]string, 0, len(req.EnvMappings))
	for name := range req.EnvMappings {
		envs = append(envs, name)
	}
	slices.Sort(envs)
	if !slices.Equal(envs, []string{"dev", "prod"}) {
		t.Errorf("env mappings: got %v, want [dev prod]", envs)
	}
	want := []models.EnvironmentVariableConfig{{Key: "url", Name: "OPENAI_BASE_URL"}, {Key: "apikey", Name: "OPENAI_API_KEY"}}
	if !slices.Equal(req.EnvironmentVariables, want) {
		t.Errorf("environment variables: got %v, want %v", req.EnvironmentVariables, want)
	}
}

func TestAttachLLMProviderRequiresEnvironmentWhenSeveral(t *testing.T) {
	session, mock := setupTestServer(t)
	mock.environments = []models.GatewayEnvironmentResponse{{Name: "dev"}, {Name: "prod"}}
	result, text := callTool(t, session, "attach_llm_provider", attachArgs(nil))
	if !result.IsError {
		t.Fatalf("expected error result, got %s", text)
	}
	if !strings.Contains(text, "environments is required") {
		t.Errorf("error should name the environments argument: %s", text)
	}
	if calls := mock.calls["AttachLLMProvider"]; len(calls) != 0 {
		t.Fatalf("AttachLLMProvider was invoked despite missing environments")
	}
}

func TestAttachLLMProviderRejectsInvalidConfigName(t *testing.T) {
	session, mock := setupTestServer(t)
	result, text := callTool(t, session, "attach_llm_provider", attachArgs(map[string]any{"config_name": "bad/name"}))
	if !result.IsError {
		t.Fatalf("expected error result, got %s", text)
	}
	if calls := mock.calls["AttachLLMProvider"]; len(calls) != 0 {
		t.Fatalf("AttachLLMProvider was invoked despite an invalid config name")
	}
}

func TestLLMProviderToolAnnotations(t *testing.T) {
	session, _ := setupTestServer(t)
	tools, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	want := map[string]gomcp.ToolAnnotations{
		"list_llm_providers":          {ReadOnlyHint: true, IdempotentHint: true},
		"add_llm_provider_to_catalog": {IdempotentHint: true},
		"attach_llm_provider":         {},
	}
	for _, tool := range tools.Tools {
		w, ok := want[tool.Name]
		if !ok {
			continue
		}
		delete(want, tool.Name)
		a := tool.Annotations
		if a == nil || tool.Title == "" {
			t.Errorf("%s: missing title or annotations", tool.Name)
			continue
		}
		if a.ReadOnlyHint != w.ReadOnlyHint || a.IdempotentHint != w.IdempotentHint {
			t.Errorf("%s: annotations %+v, want readOnly=%v idempotent=%v", tool.Name, a, w.ReadOnlyHint, w.IdempotentHint)
		}
		if a.DestructiveHint == nil || *a.DestructiveHint || a.OpenWorldHint == nil || *a.OpenWorldHint {
			t.Errorf("%s: destructiveHint and openWorldHint must be explicitly false", tool.Name)
		}
	}
	if len(want) != 0 {
		t.Errorf("tools not registered: %v", want)
	}
}

// Every scope a tool requires must be advertised in the MCP resource
// metadata, or MCP clients never request it.
func TestMCPScopesCoverToolPermissions(t *testing.T) {
	toolsets, _ := newFullToolsets()
	reg := toolsets.register(gomcp.NewServer(&gomcp.Implementation{Name: "t", Version: "0"}, nil))
	advertised := rbac.MainMCPScopes()
	for tool, perms := range reg.permissions {
		for _, perm := range perms {
			if !slices.Contains(advertised, perm.Scope()) {
				t.Errorf("tool %q requires %s, which rbac.MainMCPScopes does not list", tool, perm.Scope())
			}
		}
	}
}

func TestAttachLLMProviderNotInCatalogPointsToCatalogTool(t *testing.T) {
	session, mock := setupTestServer(t)
	mock.attachErr = fmt.Errorf("%w: provider %s must be in catalog", utils.ErrInvalidInput, testProviderID)
	result, text := callTool(t, session, "attach_llm_provider", attachArgs(nil))
	if !result.IsError || !strings.Contains(text, "add_llm_provider_to_catalog") {
		t.Fatalf("expected an error that points to add_llm_provider_to_catalog, got %s", text)
	}
}
