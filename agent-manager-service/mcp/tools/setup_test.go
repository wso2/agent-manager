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
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// constants for testing
const (
	testOrgName     = "default-org"
	testProjectName = "default-project"
	testAgentName   = "default-agent"
	testBuildName   = "default-build"
	testEnvName     = "default-env"
	testDisplayName = "Default Display Name"
)

// Creates an MCP server with all toolsets backed by the same mock handler,
// connects an in-memory client, and returns both for assertions.
func setupTestServer(t *testing.T) (*gomcp.ClientSession, *MockToolsetHandler) {
	t.Helper()

	// records every method call so tests can verify wiring after a tool invocation.
	mock := NewMockToolsetHandler()
	toolsets := &Toolsets{
		ProjectToolset:    mock,
		AgentToolset:      mock,
		BuildToolset:      mock,
		DeploymentToolset: mock,
	}
	return setupTestServerWithToolsets(t, toolsets), mock
}

// lower-level helper used when a test needs to register only a subset of toolsets
func setupTestServerWithToolsets(t *testing.T, toolsets *Toolsets) *gomcp.ClientSession {
	t.Helper()

	// create an in-memory MCP server with all toolsets registered to the same mock handler
	server := gomcp.NewServer(&gomcp.Implementation{
		Name:    "test-agent-manager-mcp",
		Version: "0.0.1",
	}, nil)

	toolsets.Register(server)

	ctx := context.Background()
	clientTransport, serverTransport := gomcp.NewInMemoryTransports()

	if _, err := server.Connect(ctx, serverTransport, nil); err != nil {
		t.Fatalf("failed to connect server: %v", err)
	}

	// create an in-memory client and connect to the in-memory MCP server
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
	toolset string // "project", "agent", "build", "deployment"

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
	return specs
}()
