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

// constants for testing
const (
	testOrgName     = "default-org"
	testProjectName = "default-project"
	testAgentName   = "default-agent"
	testBuildName   = "default-build"
	testEnvName     = "default-env"
	testDisplayName = "Default Display Name"

	testEnvUUID       = "00000000-0000-0000-0000-0000000000e1"
	testTemplateID    = "openai"
	testProviderID    = "test-provider"
	testGatewayID     = "00000000-0000-0000-0000-0000000000a1"
	testProxyID       = "test-proxy"
	testGuardrailName = "pii-masking"
	// testSecret stands in for an upstream credential; no tool output may contain it.
	testSecret = "Bearer sk-must-never-be-returned"
)

// Creates an MCP server with all toolsets backed by the same mock handler,
// connects an in-memory client, and returns both for assertions.
func setupTestServer(t *testing.T) (*gomcp.ClientSession, *MockToolsetHandler) {
	t.Helper()

	// records every method call so tests can verify wiring after a tool invocation.
	mock := NewMockToolsetHandler()
	toolsets := &Toolsets{
		ProjectToolset:     mock,
		AgentToolset:       mock,
		BuildToolset:       mock,
		DeploymentToolset:  mock,
		EnvironmentToolset: mock,
		LLMProviderToolset: mock,
		GatewayToolset:     mock,
		MCPProxyToolset:    mock,
		AgentConfigToolset: mock,
	}
	return setupTestServerWithToolsets(t, toolsets), mock
}

// lower-level helper used when a test needs to register only a subset of toolsets
func setupTestServerWithToolsets(t *testing.T, toolsets *Toolsets) *gomcp.ClientSession {
	t.Helper()
	return setupTestServerWithClaims(t, toolsets, &jwtassertion.TokenClaims{
		OuId:  testOrgName,
		Scope: unionScopes(),
	})
}

// lowest-level helper: lets authz tests pin the exact token claims (and
// therefore scopes) the session context carries.
func setupTestServerWithClaims(t *testing.T, toolsets *Toolsets, claims *jwtassertion.TokenClaims) *gomcp.ClientSession {
	t.Helper()

	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "test-agent-manager-mcp",
		Version: "0.0.1",
	}, nil)

	toolsets.Register(server)

	// Claims + scope string on the connection context, mirroring how the
	// assertion middleware injects them in prod (HasAllScopes reads the
	// scope key, not the claims struct).
	ctx := jwtassertion.ContextWithTokenClaimsAndScope(context.Background(), claims)
	clientTransport, serverTransport := gomcp.NewInMemoryTransports()

	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("failed to connect server: %v", err)
	}

	client := gomcp.NewClient(&gomcp.Implementation{
		Name:    "test-mcp-client",
		Version: "0.0.1",
	}, nil)

	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("failed to connect client: %v", err)
	}

	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

// describes everything tests need to know about a single MCP tool
type toolTestSpec struct {
	name string

	// Toolset group names, used by partial-registration tests
	toolset string // "project", "agent", "build", "deployment", "environment", "llm-provider", "gateway", "mcp-proxy", "agent-config"

	// Permissions the tool must declare via addTool. Required: the
	// registration test fails any tool whose spec leaves this empty.
	permissions []rbac.Permission

	// Description validation.
	descriptionKeywords []string
	descriptionMinLen   int

	// Schema validation.
	requiredParams []string
	optionalParams []string

	// Parameter wiring test.
	testArgs       map[string]any
	expectedMethod string
	validateCall   func(t *testing.T, args []interface{})
}

// aggregates specs from every per-toolset spec file.
var allToolSpecs = func() []toolTestSpec {
	specs := make([]toolTestSpec, 0)
	specs = append(specs, projectToolSpecs()...)
	specs = append(specs, agentToolSpecs()...)
	specs = append(specs, buildToolSpecs()...)
	specs = append(specs, deploymentToolSpecs()...)
	specs = append(specs, environmentToolSpecs()...)
	specs = append(specs, llmProviderToolSpecs()...)
	specs = append(specs, gatewayToolSpecs()...)
	specs = append(specs, mcpProxyToolSpecs()...)
	specs = append(specs, agentConfigToolSpecs()...)
	return specs
}()

// unionScopes returns a space-separated scope string covering every
// permission any tool declares, so wiring tests are never blocked by authz.
func unionScopes() string {
	seen := make(map[string]struct{})
	scopes := make([]string, 0)
	for _, spec := range allToolSpecs {
		for _, perm := range spec.permissions {
			scope := perm.Scope()
			if _, ok := seen[scope]; ok {
				continue
			}
			seen[scope] = struct{}{}
			scopes = append(scopes, scope)
		}
	}
	return strings.Join(scopes, " ")
}
