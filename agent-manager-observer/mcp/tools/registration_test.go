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
	"slices"
	"strings"
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Verifies that every tool described by allToolSpecs is actually registered
// on a fully-wired MCP server, and that there are exactly eight of them.
func TestToolRegistration(t *testing.T) {
	clientSession, _ := setupTestServer(t)

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	if got, want := len(toolsResult.Tools), 8; got != want {
		t.Errorf("registered tool count: got %d, want %d", got, want)
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

// Verifies that every tool has a description that is at least the spec's
// minimum length and contains every required keyword.
func TestToolDescriptions(t *testing.T) {
	clientSession, _ := setupTestServer(t)

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	toolsByName := make(map[string]*gomcp.Tool, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		toolsByName[tool.Name] = tool
	}

	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			tool, exists := toolsByName[spec.name]
			if !exists {
				t.Fatalf("tool %q not registered", spec.name)
			}

			desc := strings.ToLower(tool.Description)
			if len(desc) < spec.descriptionMinLen {
				t.Errorf("description too short: got %d chars, want >= %d", len(desc), spec.descriptionMinLen)
			}
			for _, kw := range spec.descriptionKeywords {
				if !strings.Contains(desc, strings.ToLower(kw)) {
					t.Errorf("description missing keyword %q. description was: %s", kw, tool.Description)
				}
			}
		})
	}
}

// Verifies that the (auto-inferred) JSON schema of each tool declares
// exactly the required and optional params named in its spec.
func TestToolSchemas(t *testing.T) {
	clientSession, _ := setupTestServer(t)

	ctx := context.Background()
	toolsResult, err := clientSession.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}

	toolsByName := make(map[string]*gomcp.Tool, len(toolsResult.Tools))
	for _, tool := range toolsResult.Tools {
		toolsByName[tool.Name] = tool
	}

	for _, spec := range allToolSpecs {
		t.Run(spec.name, func(t *testing.T) {
			tool, exists := toolsByName[spec.name]
			if !exists {
				t.Fatalf("tool %q not registered", spec.name)
			}
			if tool.InputSchema == nil {
				t.Fatal("InputSchema is nil")
			}

			schema, ok := tool.InputSchema.(map[string]any)
			if !ok {
				t.Fatalf("InputSchema is not map[string]any (got %T)", tool.InputSchema)
			}

			if got, _ := schema["type"].(string); got != "object" {
				t.Errorf("schema.type: got %v, want \"object\"", schema["type"])
			}

			requiredInSchema := map[string]bool{}
			if reqList, ok := schema["required"].([]interface{}); ok {
				for _, r := range reqList {
					if s, ok := r.(string); ok {
						requiredInSchema[s] = true
					}
				}
			}
			for _, want := range spec.requiredParams {
				if !requiredInSchema[want] {
					t.Errorf("required param %q missing from schema.required", want)
				}
			}
			if got, want := len(requiredInSchema), len(spec.requiredParams); got != want {
				t.Errorf("schema.required has %d entries %v, want %d %v", got, requiredInSchema, want, spec.requiredParams)
			}

			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatal("schema.properties is not a map")
			}
			var wantProps []string
			wantProps = append(wantProps, spec.requiredParams...)
			wantProps = append(wantProps, spec.optionalParams...)
			for _, want := range wantProps {
				if _, ok := properties[want]; !ok {
					t.Errorf("param %q missing from schema.properties", want)
				}
			}
			if got, want := len(properties), len(wantProps); got != want {
				t.Errorf("schema.properties has %d entries, want %d (%v)", got, want, wantProps)
			}
		})
	}
}

// Smoke-tests that every tool accepts a minimal valid argument set and
// returns a JSON-encoded, non-error result.
func TestToolSmokeCall(t *testing.T) {
	for _, spec := range allToolSpecs {
		spec := spec
		t.Run(spec.name, func(t *testing.T) {
			clientSession, _ := setupTestServer(t)
			ctx := context.Background()

			result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
				Name:      spec.name,
				Arguments: spec.testArgs,
			})
			if err != nil {
				t.Fatalf("CallTool failed: %v", err)
			}
			if result.IsError {
				t.Fatalf("expected success, got tool error: %+v", result.Content)
			}
			if len(result.Content) == 0 {
				t.Fatal("expected non-empty result content")
			}

			textContent, ok := result.Content[0].(*gomcp.TextContent)
			if !ok {
				t.Fatalf("expected *gomcp.TextContent, got %T", result.Content[0])
			}
			var data interface{}
			if err := json.Unmarshal([]byte(textContent.Text), &data); err != nil {
				t.Errorf("response is not valid JSON: %v\nresponse: %s", err, textContent.Text)
			}
		})
	}
}

// Verifies that every tool declaring "organization" as required rejects a
// call that omits it — either at the protocol level (schema validation,
// before the handler runs) or as a tool-level error result.
func TestMissingOrganizationRejected(t *testing.T) {
	for _, spec := range allToolSpecs {
		if !slices.Contains(spec.requiredParams, "organization") {
			continue
		}
		spec := spec
		t.Run(spec.name, func(t *testing.T) {
			clientSession, _ := setupTestServer(t)
			ctx := context.Background()

			args := make(map[string]any, len(spec.testArgs))
			for k, v := range spec.testArgs {
				if k == "organization" {
					continue
				}
				args[k] = v
			}

			result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
				Name:      spec.name,
				Arguments: args,
			})
			switch {
			case err != nil:
				// Protocol-level rejection by the SDK — fine.
			case result != nil && result.IsError:
				// Handler-level tool error — fine.
			default:
				t.Errorf("expected error for %q called without organization; got success", spec.name)
			}
		})
	}
}
