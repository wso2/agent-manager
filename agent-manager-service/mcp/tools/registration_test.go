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
	"os"
	"slices"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/auth"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// Verifies that every tool described by allToolSpecs is actually registered on a fully-wired MCP server.
func TestToolRegistration(t *testing.T) {
	clientSession, _ := setupTestServer(t)

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	registered := make(map[string]bool, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		registered[tool.Name] = true
	}

	for _, spec := range allToolSpecs {
		if !registered[spec.name] {
			t.Errorf("expected tool %q not registered", spec.name)
		}
	}
}

// Verifies every tool spec declares permissions and that the registry built by
// registration matches the specs exactly — a tool added without declaring its
// permission fails here (and addTool panics before this test even runs).
func TestToolPermissionsMatchSpecs(t *testing.T) {
	reg := registerMockToolsets()

	for _, spec := range allToolSpecs {
		if len(spec.permissions) == 0 {
			t.Errorf("tool spec %q declares no permissions", spec.name)
			continue
		}
		got := reg.permissions[spec.name]
		if !slices.Equal(got, spec.permissions) {
			t.Errorf("tool %q: registered permissions %v, spec expects %v", spec.name, got, spec.permissions)
		}
	}
	if len(reg.permissions) != len(allToolSpecs) {
		t.Errorf("registry has %d tools, specs describe %d", len(reg.permissions), len(allToolSpecs))
	}
}

func TestMutatingToolsHaveGrowthAnalytics(t *testing.T) {
	reg := registerMockToolsets()
	for name, action := range reg.actions {
		_, tracked := trackedTools[name]
		if tracked != (action.Class() != audit.ClassRead) {
			t.Errorf("tool %q with audit class %s has unexpected analytics tracking: %t", name, action.Class(), tracked)
		}
	}
	for name := range trackedTools {
		if _, registered := reg.actions[name]; !registered {
			t.Errorf("analytics mapping for unregistered tool %q", name)
		}
	}
}

// Verify the metadata the tools actually submit, including the deployment
// actions computed at runtime.
func TestToolTelemetryMatchesTaxonomy(t *testing.T) {
	type feature struct {
		Code     string
		Status   string
		Tracking struct {
			Dimensions []struct {
				Name     string
				Type     string
				Values   []string
				Required bool
			}
		}
		SubFeatures []feature `yaml:"sub_features"`
	}
	var taxonomy struct {
		Features []feature
	}
	data, err := os.ReadFile("../../middleware/growthanalytics/taxonomy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(data, &taxonomy); err != nil {
		t.Fatal(err)
	}
	features := make(map[string]feature)
	var indexFeatures func([]feature)
	indexFeatures = func(entries []feature) {
		for _, entry := range entries {
			features[entry.Code] = entry
			indexFeatures(entry.SubFeatures)
		}
	}
	indexFeatures(taxonomy.Features)

	for name := range trackedTools {
		t.Run(name, func(t *testing.T) {
			calls := 0
			report := growthanalytics.MCPReporter(func(_ context.Context, code string, metadata map[string]any, _ bool) {
				calls++
				feature, ok := features[code]
				if !ok || feature.Status == "deprecated" {
					t.Fatalf("tool emits undeclared or deprecated code %q", code)
				}
				for _, dim := range feature.Tracking.Dimensions {
					value, sent := metadata[dim.Name]
					if !sent {
						if dim.Required {
							t.Errorf("%s is missing required dimension %q", code, dim.Name)
						}
						continue
					}
					delete(metadata, dim.Name)
					if dim.Type != "enum" {
						continue
					}
					if dim.Name == "outcome" && value == growthanalytics.DynamicOutcome {
						if !slices.Contains(dim.Values, "success") || !slices.Contains(dim.Values, "failure") {
							t.Errorf("%s must declare both dynamic outcome values", code)
						}
					} else if enum, ok := value.(string); !ok || !slices.Contains(dim.Values, enum) {
						t.Errorf("%s sends %s=%v, want one of %v", code, dim.Name, value, dim.Values)
					}
				}
				delete(metadata, "channel") // Shared event metadata.
				if len(metadata) > 0 {
					t.Errorf("%s sends undeclared dimensions: %v", code, metadata)
				}
			})
			request := &gomcp.CallToolRequest{Extra: &gomcp.RequestExtra{
				TokenInfo: &auth.TokenInfo{Extra: map[string]any{growthanalytics.MCPReporterKey: report}},
			}}
			inputs := []any{nil}
			if name == "update_deployment_state" {
				inputs = append(inputs, updateDeploymentStateInput{State: "redeploy"}, updateDeploymentStateInput{State: "undeploy"})
			}
			for _, input := range inputs {
				reportToolUsage(t.Context(), request, name, input, true)
			}
			if calls != len(inputs) {
				t.Errorf("got %d events for %d tool invocations", calls, len(inputs))
			}
		})
	}
}

func registerMockToolsets() *toolRegistry {
	mock := NewMockToolsetHandler()
	toolsets := &Toolsets{
		ProjectToolset:     mock,
		AgentToolset:       mock,
		BuildToolset:       mock,
		DeploymentToolset:  mock,
		EnvironmentToolset: mock,
		LLMProviderToolset: mock,
		MCPProxyToolset:    mock,
		AgentConfigToolset: mock,
	}
	server := gomcp.NewServer(&gomcp.Implementation{Name: "t", Version: "0"}, nil)
	return toolsets.register(server)
}

// Verifies every permission a tool declares is advertised in the MCP
// endpoint's scopes_supported. A missing scope is never requested at sign-in,
// so the tool would be denied for every user even though it is registered.
func TestToolPermissionsAreAdvertised(t *testing.T) {
	advertised := make(map[string]bool)
	for _, scope := range rbac.MainMCPScopes() {
		advertised[scope] = true
	}
	for _, spec := range allToolSpecs {
		for _, perm := range spec.permissions {
			if !advertised[perm.Scope()] {
				t.Errorf("tool %q requires %s, which rbac.MainMCPScopes does not advertise", spec.name, perm.Scope())
			}
		}
	}
}

// TestNilToolsetsInstallsFailClosedMiddleware verifies that a server built
// from a nil *Toolsets still gets the authz middleware installed: any
// tools/call — even one bypassing addTool entirely — is denied.
func TestNilToolsetsInstallsFailClosedMiddleware(t *testing.T) {
	server := gomcp.NewServer(&gomcp.Implementation{Name: "test-nil-toolsets", Version: "0.0.1"}, nil)
	var toolsets *Toolsets
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

	ctx := context.Background()
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
		t.Fatal("rogue tool executed on a nil-Toolsets server — fail-closed middleware did not deny it")
	}
}
