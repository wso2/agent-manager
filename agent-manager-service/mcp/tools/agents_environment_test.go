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

func callCreateExternalAgent(t *testing.T, session *gomcp.ClientSession, args map[string]any) *gomcp.CallToolResult {
	t.Helper()
	base := map[string]any{
		"project_name": testProjectName,
		"agent_name":   testAgentName,
		"display_name": testDisplayName,
	}
	for k, v := range args {
		base[k] = v
	}
	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name:      "create_external_agent",
		Arguments: base,
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	return result
}

// lastGenerateTokenEnv returns the environment argument of the most recent
// GenerateToken call recorded on the mock.
func lastGenerateTokenEnv(t *testing.T, mock *MockToolsetHandler) string {
	t.Helper()
	calls := mock.calls["GenerateToken"]
	if len(calls) == 0 {
		t.Fatal("GenerateToken was not called")
	}
	args, ok := calls[len(calls)-1].([]interface{})
	if !ok {
		t.Fatalf("recorded args have unexpected type %T", calls[len(calls)-1])
	}
	env, ok := args[3].(string)
	if !ok {
		t.Fatalf("environment arg has unexpected type %T", args[3])
	}
	return env
}

func TestCreateExternalAgentPassesEnvironmentToTokenGeneration(t *testing.T) {
	session, mock := setupTestServer(t)
	result := callCreateExternalAgent(t, session, map[string]any{"environment": testEnvName})
	if result.IsError {
		t.Fatalf("expected success, got error result: %+v", result.Content)
	}
	if got := lastGenerateTokenEnv(t, mock); got != testEnvName {
		t.Errorf("GenerateToken environment: got %q, want %q", got, testEnvName)
	}
}

func TestCreateExternalAgentDefaultsToOnlyEnvironment(t *testing.T) {
	session, mock := setupTestServer(t)
	result := callCreateExternalAgent(t, session, nil)
	if result.IsError {
		t.Fatalf("expected success, got error result: %+v", result.Content)
	}
	// the mock exposes a single environment, so the omitted input must resolve to it
	if got := lastGenerateTokenEnv(t, mock); got != testEnvName {
		t.Errorf("GenerateToken environment: got %q, want %q", got, testEnvName)
	}
}

func TestCreateExternalAgentRejectsUnknownEnvironmentBeforeCreation(t *testing.T) {
	session, mock := setupTestServer(t)
	result := callCreateExternalAgent(t, session, map[string]any{"environment": "no-such-env"})
	if !result.IsError {
		t.Fatal("expected error result for unknown environment")
	}
	if calls := mock.calls["CreateAgent"]; len(calls) != 0 {
		t.Fatalf("CreateAgent was invoked %d times despite invalid environment", len(calls))
	}
	if calls := mock.calls["GenerateToken"]; len(calls) != 0 {
		t.Fatalf("GenerateToken was invoked %d times despite invalid environment", len(calls))
	}
}
