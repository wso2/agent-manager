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

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// callListProjects invokes list_projects over the in-memory client session and
// returns the result. list_projects requires rbac.ProjectRead.
func callListProjects(t *testing.T, session *gomcp.ClientSession) *gomcp.CallToolResult {
	t.Helper()
	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name:      "list_projects",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	return result
}

func newFullToolsets() (*Toolsets, *MockToolsetHandler) {
	mock := NewMockToolsetHandler()
	return &Toolsets{
		ProjectToolset:     mock,
		AgentToolset:       mock,
		BuildToolset:       mock,
		DeploymentToolset:  mock,
		EnvironmentToolset: mock,
	}, mock
}

func TestE2EToolAllowedWithScope(t *testing.T) {
	setRBACEnabled(t, true)
	toolsets, _ := newFullToolsets()
	session := setupTestServerWithClaims(t, toolsets, &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.ProjectRead.Scope(),
	})
	result := callListProjects(t, session)
	if result.IsError {
		t.Fatalf("expected success, got error result: %+v", result.Content)
	}
}

func TestE2EToolDeniedWithoutScope(t *testing.T) {
	setRBACEnabled(t, true)
	toolsets, mock := newFullToolsets()
	session := setupTestServerWithClaims(t, toolsets, &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.AgentRead.Scope(), // wrong scope for list_projects
	})
	result := callListProjects(t, session)
	if !result.IsError {
		t.Fatal("expected denial, got success")
	}
	text, ok := result.Content[0].(*gomcp.TextContent)
	if !ok {
		t.Fatalf("content is %T, want *gomcp.TextContent", result.Content[0])
	}
	if want := "insufficient permissions: this tool requires the amp:project:read scope"; text.Text != want {
		t.Fatalf("denial text = %q, want %q", text.Text, want)
	}
	if calls := mock.calls["ListProjects"]; len(calls) != 0 {
		t.Fatalf("handler was invoked %d times despite denial", len(calls))
	}
}

func TestE2EToolAllowedWhenRBACDisabled(t *testing.T) {
	setRBACEnabled(t, false)
	toolsets, _ := newFullToolsets()
	session := setupTestServerWithClaims(t, toolsets, &jwtassertion.TokenClaims{
		OuId: testOrgName, // no scopes at all
	})
	result := callListProjects(t, session)
	if result.IsError {
		t.Fatalf("expected success with RBAC disabled, got error result: %+v", result.Content)
	}
}

func TestE2ERogueToolFailsClosed(t *testing.T) {
	setRBACEnabled(t, false) // fail-closed must hold even with RBAC disabled
	toolsets, _ := newFullToolsets()

	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "test-agent-manager-mcp",
		Version: "0.0.1",
	}, nil)
	toolsets.Register(server)

	// Bypass addTool deliberately: register a tool directly with the SDK.
	gomcp.AddTool(server, &gomcp.Tool{
		Name:        "rogue_tool",
		Description: "registered without going through addTool",
		InputSchema: createSchema(map[string]any{}, nil),
	}, func(context.Context, *gomcp.CallToolRequest, struct{}) (*gomcp.CallToolResult, any, error) {
		return &gomcp.CallToolResult{
			Content: []gomcp.Content{&gomcp.TextContent{Text: "should never run"}},
		}, nil, nil
	})

	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: unionScopes(),
	})
	clientTransport, serverTransport := gomcp.NewInMemoryTransports()
	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("failed to connect server: %v", err)
	}
	client := gomcp.NewClient(&gomcp.Implementation{Name: "test-mcp-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}
	t.Cleanup(func() { _ = session.Close() })

	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name:      "rogue_tool",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatal("rogue tool executed — fail-closed middleware did not deny it")
	}
	text, ok := result.Content[0].(*gomcp.TextContent)
	if !ok {
		t.Fatalf("content is %T, want *gomcp.TextContent", result.Content[0])
	}
	if !strings.Contains(text.Text, `tool "rogue_tool" has no registered permissions`) {
		t.Fatalf("unexpected denial text: %q", text.Text)
	}
}

func TestE2EMultiPermissionToolDeniedWithPartialScopes(t *testing.T) {
	setRBACEnabled(t, true)
	toolsets, mock := newFullToolsets()
	// create_external_agent requires AgentCreate AND AgentTokenManage.
	session := setupTestServerWithClaims(t, toolsets, &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: rbac.AgentCreate.Scope(),
	})
	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name: "create_external_agent",
		Arguments: map[string]any{
			"project_name": testProjectName,
			"agent_name":   testAgentName,
			"display_name": testDisplayName,
			"language":     "python",
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected denial with only one of two required scopes")
	}
	if calls := mock.calls["CreateAgent"]; len(calls) != 0 {
		t.Fatalf("handler was invoked %d times despite denial", len(calls))
	}
}
