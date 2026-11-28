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

package create

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
)

// --- template mode ---

// --template writes exactly the manifest to stdout (clean for `>` redirects),
// makes no API call (nil client would panic), and resolves no scope.
func TestCreate_TemplateMode(t *testing.T) {
	ios, out, errOut := newTestIO(false)
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{"agent", "create", "--template"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.String() != agentTemplate {
		t.Errorf("stdout is not exactly the template:\n%s", out.String())
	}
	if errOut.String() != "" {
		t.Errorf("stderr should be empty, got %q", errOut.String())
	}
}

func TestCreate_TemplateMode_RejectsNameArg(t *testing.T) {
	ios, _, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{"agent", "create", "my-agent", "--template"})
	err := cmd.Execute()
	details := mustFlagDetails(t, err)
	assertContains(t, details, "name argument is not allowed with --template")
}

func TestCreate_TemplateMode_RejectsContentFlags(t *testing.T) {
	ios, _, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{"agent", "create", "--template", "--display-name", "X", "--repo-url", "https://x"})
	err := cmd.Execute()
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--display-name is not allowed with --template")
	assertContains(t, details, "--repo-url is not allowed with --template")
}

// --template prints a YAML document; --json promises a JSON envelope on
// stdout, so the combination is rejected rather than silently emitting YAML.
func TestCreate_TemplateMode_RejectsJSON(t *testing.T) {
	ios, _, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{"agent", "create", "--template", "--json"})
	err := cmd.Execute()
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--json is not allowed with --template")
}

func TestCreate_TemplateMode_RejectsFile(t *testing.T) {
	ios, _, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{"agent", "create", "--template", "-f", "agent.yaml"})
	err := cmd.Execute()
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--file is not allowed with --template")
}

// --- file mode ---

func TestCreate_FileMode_Internal(t *testing.T) {
	ios, _, _ := newTestIO(true)
	clientFn, captured, cleanup := newTestClient(t, 202, agentResponse())
	defer cleanup()

	path := writeManifest(t, internalManifest)
	cmd := testCreateCmd(t, ios, clientFn, "")
	cmd.SetArgs([]string{"agent", "create", "-f", path, "--project", "triage"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if captured.method != http.MethodPost {
		t.Errorf("method = %q, want POST", captured.method)
	}
	if !strings.HasSuffix(captured.path, "/orgs/acme/projects/triage/agents") {
		t.Errorf("path = %q", captured.path)
	}

	var body map[string]any
	if err := json.Unmarshal(captured.body, &body); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if body["name"] != "my-agent" || body["displayName"] != "My Agent" {
		t.Errorf("body name/displayName = %v/%v", body["name"], body["displayName"])
	}
	build := body["build"].(map[string]any)
	if build["type"] != "buildpack" {
		t.Errorf("build.type = %v", build["type"])
	}
	bp := build["buildpack"].(map[string]any)
	if bp["language"] != "python" {
		t.Errorf("buildpack.language = %v", bp["language"])
	}
}

// File-mode external agents get the token + instrumentation flow: the
// post-create branch keys off the request body, not the --provisioning flag.
func TestCreate_FileMode_External_TokenFlow(t *testing.T) {
	ios, _, errOut := newTestIO(false)
	routes := map[string]routeResponse{
		"/orgs/acme/projects/triage/agents/testing/token": {
			Status: 200,
			Body:   amsvc.TokenResponse{Token: "tok-abc", ExpiresAt: 1700000000, IssuedAt: 1690000000, TokenType: "Bearer"},
		},
		"/orgs/acme/projects/triage/agents": {
			Status: 202,
			Body: amsvc.AgentResponse{
				Name:         "testing",
				DisplayName:  "Testing",
				AgentType:    amsvc.AgentType{Type: "external-agent-api"},
				Provisioning: amsvc.Provisioning{Type: amsvc.ProvisioningTypeExternal},
				ProjectName:  "triage",
				Uuid:         "u",
				CreatedAt:    time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	clientFn, captured, cleanup := newTestRouter(t, routes)
	defer cleanup()

	path := writeManifest(t, `
apiVersion: agent-manager.wso2.com/v1alpha1
kind: Agent
spec:
  name: testing
  displayName: Testing
  agentType:
    type: external-agent-api
  provisioning:
    type: external
`)
	cmd := testCreateCmd(t, ios, clientFn, "https://otel.example")
	cmd.SetArgs([]string{"agent", "create", "-f", path, "--project", "triage"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := captured["/orgs/acme/projects/triage/agents/testing/token"]; !ok {
		t.Error("no token request captured — external flow must run for file mode")
	}
	if !strings.Contains(errOut.String(), `export AMP_AGENT_API_KEY="tok-abc"`) {
		t.Errorf("missing instrumentation block:\n%s", errOut.String())
	}
}

func TestCreate_FileMode_RejectsNameArg(t *testing.T) {
	ios, _, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{"agent", "create", "my-agent", "-f", "agent.yaml", "--project", "triage"})
	err := cmd.Execute()
	details := mustFlagDetails(t, err)
	assertContains(t, details, "name argument is not allowed with --file")
}

func TestCreate_FileMode_RejectsContentFlags(t *testing.T) {
	ios, _, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{
		"agent", "create", "-f", "agent.yaml", "--project", "triage",
		"--display-name", "X", "--env", "A=1", "--port", "9000",
	})
	err := cmd.Execute()
	details := mustFlagDetails(t, err)
	assertContains(t, details, "--display-name is not allowed with --file")
	assertContains(t, details, "--env is not allowed with --file")
	assertContains(t, details, "--port is not allowed with --file")
}

func TestCreate_FileMode_InvalidSpecAggregated(t *testing.T) {
	ios, _, _ := newTestIO(true)
	path := writeManifest(t, `
apiVersion: agent-manager.wso2.com/v1alpha1
kind: Agent
spec:
  name: my-agent
  provisioning:
    type: internal
    repository:
      url: https://github.com/example/repo
`)
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{"agent", "create", "-f", path, "--project", "triage"})
	err := cmd.Execute()
	details := mustFlagDetails(t, err)
	assertContains(t, details, "spec.displayName is required")
	assertContains(t, details, "spec.provisioning.repository.branch is required")
	assertContains(t, details, "spec.provisioning.repository.appPath is required")
	assertContains(t, details, "spec.build is required for internal provisioning")
	assertContains(t, details, "spec.agentType.type is required for internal provisioning")
	assertContains(t, details, "spec.inputInterface is required for internal provisioning")
	if !strings.Contains(err.Error(), "invalid agent spec") {
		t.Errorf("error %q missing 'invalid agent spec' header", err.Error())
	}
}

func TestCreate_FileMode_EnvelopeMismatch(t *testing.T) {
	ios, _, _ := newTestIO(true)
	path := writeManifest(t, `
apiVersion: agent-manager.wso2.com/v1alpha1
kind: Project
spec:
  name: x
`)
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{"agent", "create", "-f", path, "--project", "triage"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "kind") {
		t.Fatalf("expected kind mismatch error, got %v", err)
	}
}

func TestCreate_FileMode_FileNotFound(t *testing.T) {
	ios, _, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{"agent", "create", "-f", "/nonexistent/agent.yaml", "--project", "triage"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error for missing manifest file")
	}
}
