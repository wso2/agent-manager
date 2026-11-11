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

	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func callCreateInternalAgentPython(t *testing.T, session *gomcp.ClientSession, args map[string]any) *gomcp.CallToolResult {
	t.Helper()
	base := map[string]any{
		"project_name":   testProjectName,
		"agent_name":     testAgentName,
		"display_name":   testDisplayName,
		"repository_url": "https://github.com/acme/private-agent",
		"branch":         "dev",
		"app_path":       "/agents/support",
		"interface_type": "DEFAULT",
		"env":            []map[string]any{},
	}
	for k, v := range args {
		base[k] = v
	}
	result, err := session.CallTool(context.Background(), &gomcp.CallToolParams{
		Name:      "create_internal_agent_python",
		Arguments: base,
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	return result
}

func lastCreateAgentRequest(t *testing.T, mock *MockToolsetHandler) *spec.CreateAgentRequest {
	t.Helper()
	calls := mock.calls["CreateAgent"]
	if len(calls) == 0 {
		t.Fatal("CreateAgent was not called")
	}
	args, ok := calls[len(calls)-1].([]interface{})
	if !ok {
		t.Fatalf("recorded args have unexpected type %T", calls[len(calls)-1])
	}
	req, ok := args[2].(*spec.CreateAgentRequest)
	if !ok {
		t.Fatalf("request arg has unexpected type %T", args[2])
	}
	return req
}

func resultText(t *testing.T, result *gomcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) != 1 {
		t.Fatalf("expected one content item, got %d", len(result.Content))
	}
	text, ok := result.Content[0].(*gomcp.TextContent)
	if !ok {
		t.Fatalf("content has unexpected type %T", result.Content[0])
	}
	return text.Text
}

func TestCreateInternalAgentPythonSetsGitHubAppSource(t *testing.T) {
	session, mock := setupTestServer(t)
	result := callCreateInternalAgentPython(t, session, map[string]any{"github_installation_id": 4242})
	if result.IsError {
		t.Fatalf("expected success, got error result: %s", resultText(t, result))
	}
	got := lastCreateAgentRequest(t, mock).GithubApp
	if got == nil {
		t.Fatal("GithubApp is nil")
	}
	if got.InstallationId != 4242 || got.Owner != "acme" || got.Repo != "private-agent" {
		t.Errorf("GithubApp identity: got %d %q/%q", got.InstallationId, got.Owner, got.Repo)
	}
	if got.GetBranch() != "dev" || got.GetAppPath() != "/agents/support" ||
		got.GetRepositoryUrl() != "https://github.com/acme/private-agent" {
		t.Errorf("GithubApp source: got branch %q, app path %q, url %q",
			got.GetBranch(), got.GetAppPath(), got.GetRepositoryUrl())
	}
}

func TestCreateInternalAgentPythonOmitsGitHubAppSourceForPublicRepository(t *testing.T) {
	session, mock := setupTestServer(t)
	result := callCreateInternalAgentPython(t, session, nil)
	if result.IsError {
		t.Fatalf("expected success, got error result: %s", resultText(t, result))
	}
	if got := lastCreateAgentRequest(t, mock).GithubApp; got != nil {
		t.Errorf("GithubApp: got %+v, want nil", got)
	}
}

func TestCreateInternalAgentPythonRejectsInvalidInstallationBeforeCreation(t *testing.T) {
	cases := map[string]map[string]any{
		"non-GitHub URL": {
			"github_installation_id": 4242,
			"repository_url":         "https://gitlab.com/acme/private-agent",
		},
		"zero id":     {"github_installation_id": 0},
		"negative id": {"github_installation_id": -1},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			session, mock := setupTestServer(t)
			result := callCreateInternalAgentPython(t, session, args)
			if !result.IsError {
				t.Fatal("expected error result")
			}
			if text := resultText(t, result); !strings.Contains(text, "github_installation_id") {
				t.Errorf("error text %q does not name github_installation_id", text)
			}
			if calls := mock.calls["CreateAgent"]; len(calls) != 0 {
				t.Fatalf("CreateAgent was invoked %d times despite invalid input", len(calls))
			}
		})
	}
}
