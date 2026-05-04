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
	"testing"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

// Calls each tool through the in-memory MCP client and verifies that the expected handler
// method on the mock was invoked with the expected arguments.
func TestToolParameterWiring(t *testing.T) {
	for _, spec := range allToolSpecs {
		spec := spec // capture for closure
		t.Run(spec.name, func(t *testing.T) {
			if spec.expectedMethod == "" {
				t.Skipf("spec for %q skips wiring check (multi-step or complex)", spec.name)
			}

			clientSession, mock := setupTestServer(t)
			ctx := context.Background()

			result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
				Name:      spec.name,
				Arguments: spec.testArgs,
			})
			if err != nil {
				t.Fatalf("CallTool failed: %v", err)
			}
			if len(result.Content) == 0 {
				t.Fatal("expected non-empty result content")
			}

			calls, ok := mock.calls[spec.expectedMethod]
			if !ok || len(calls) == 0 {
				t.Fatalf("expected method %q was not called. recorded calls: %v",
					spec.expectedMethod, mock.calls)
			}

			args, ok := calls[0].([]interface{})
			if !ok {
				t.Fatalf("recorded args have unexpected type %T", calls[0])
			}
			spec.validateCall(t, args)
		})
	}
}

// Verifies that tool results are valid JSON.
func TestToolResponseFormat(t *testing.T) {
	clientSession, _ := setupTestServer(t)
	ctx := context.Background()

	result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
		Name:      "list_projects",
		Arguments: map[string]any{"org_name": testOrgName},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
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
		t.Errorf("response is not valid JSON: %v\nresponse: %s",
			err, textContent.Text)
	}
}

// Verifies that calls missing required parameters are rejected — either by the MCP SDK
// before the handler runs or by the handler itself returning a tool-level error result.
func TestToolErrorHandling(t *testing.T) {
	var sample toolTestSpec
	for _, spec := range allToolSpecs {
		if len(spec.requiredParams) > 0 {
			sample = spec
			break
		}
	}
	if sample.name == "" {
		t.Fatal("no spec with required params; cannot exercise error path")
	}

	clientSession, _ := setupTestServer(t)
	ctx := context.Background()

	result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
		Name:      sample.name,
		Arguments: map[string]any{}, // missing all required params
	})

	switch {
	case err != nil:
		// Protocol-level rejection by the SDK — fine.
	case result != nil && result.IsError:
		// Handler returned a tool-level error result — fine.
	default:
		t.Errorf("expected error for %q called with empty args; got success", sample.name)
	}
}
