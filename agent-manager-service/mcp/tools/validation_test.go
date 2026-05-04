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
)

// Verifies that every tool has a description that is: at least the spec's minimum length and contains every required keyword.
// Also checks that no two tools share the exact same description
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
			// ensure description is at least the minimum length specified.
			if len(desc) < spec.descriptionMinLen {
				t.Errorf("description too short: got %d chars, want >= %d",
					len(desc), spec.descriptionMinLen)
			}
			// ensure descriptions contains every required keyword.
			for _, kw := range spec.descriptionKeywords {
				if !strings.Contains(desc, strings.ToLower(kw)) {
					t.Errorf("description missing keyword %q. description was: %s",
						kw, tool.Description)
				}
			}
		})
	}

	// Ensure descriptions are unique.
	seen := make(map[string]string)
	for _, tool := range toolsResult.Tools {
		if other, dup := seen[tool.Description]; dup {
			t.Errorf("duplicate description shared by %q and %q: %s",
				tool.Name, other, tool.Description)
		}
		seen[tool.Description] = tool.Name
	}
}

// Verifies that the JSON schema of each tool declares the required and optional params named in its spec.
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

			// Schema must be an object.
			if got, _ := schema["type"].(string); got != "object" {
				t.Errorf("schema.type: got %v, want \"object\"", schema["type"])
			}

			// Required params must appear in schema.required.
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

			// All params (required + optional) must appear in schema.properties.
			properties, ok := schema["properties"].(map[string]any)
			if !ok {
				t.Fatal("schema.properties is not a map")
			}
			for _, want := range append([]string(nil), spec.requiredParams...) {
				if _, ok := properties[want]; !ok {
					t.Errorf("required param %q missing from schema.properties", want)
				}
			}
			for _, want := range spec.optionalParams {
				if _, ok := properties[want]; !ok {
					t.Errorf("optional param %q missing from schema.properties", want)
				}
			}
		})
	}
}
