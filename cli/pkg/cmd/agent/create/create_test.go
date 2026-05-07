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
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

// --- test helpers ---

type capturedRequest struct {
	method string
	path   string
	body   []byte
}

func newTestClient(t *testing.T, status int, respBody any) (func(context.Context) (*amsvc.ClientWithResponses, error), *capturedRequest, func()) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.method = r.Method
		captured.path = r.URL.Path
		captured.body, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if respBody != nil {
			json.NewEncoder(w).Encode(respBody)
		}
	}))
	client, err := amsvc.NewClientWithResponses(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("new client: %v", err)
	}
	return func(context.Context) (*amsvc.ClientWithResponses, error) { return client, nil }, captured, server.Close
}

func newTestIO(jsonMode bool) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	ios, _, out, errOut := iostreams.Test()
	ios.JSON = jsonMode
	return ios, out, errOut
}

func testCreateCmd(t *testing.T, ios *iostreams.IOStreams, clientFn func(context.Context) (*amsvc.ClientWithResponses, error)) *cobra.Command {
	t.Helper()
	f := &cmdutil.Factory{
		IOStreams:     ios,
		AgentManager: clientFn,
		Config: func() (*config.Config, error) {
			return &config.Config{
				CurrentInstance: "default",
				Instances: map[string]config.Instance{
					"default": {URL: "http://test", CurrentOrg: "acme"},
				},
			}, nil
		},
	}
	root := &cobra.Command{Use: "amctl", SilenceErrors: true, SilenceUsage: true}
	cmdutil.EnableOrgOverride(root, f)

	agentCmd := &cobra.Command{Use: "agent"}
	cmdutil.EnableProjectOverride(agentCmd, f)

	createCmd := NewCreateCmd(f)
	agentCmd.AddCommand(createCmd)
	root.AddCommand(agentCmd)
	return root
}

func decodeEnvelope(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decode envelope: %v\nbody=%q", err, raw)
	}
	return m
}

func agentResponse() amsvc.AgentResponse {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	return amsvc.AgentResponse{
		Name:        "my-agent",
		DisplayName: "My Agent",
		Description: "test",
		AgentType:   amsvc.AgentType{Type: "agent-api"},
		Provisioning: amsvc.Provisioning{
			Type: "internal",
		},
		ProjectName: "triage",
		Uuid:        "uuid-123",
		CreatedAt:   now,
	}
}

func buildpackArgs() []string {
	return []string{
		"agent", "create",
		"--project", "triage",
		"--name", "my-agent",
		"--display-name", "My Agent",
		"--type", "agent-api",
		"--repo-url", "https://github.com/example/repo",
		"--repo-branch", "main",
		"--repo-path", "/",
		"--build-type", "buildpack",
		"--language", "go",
	}
}

func dockerArgs() []string {
	return []string{
		"agent", "create",
		"--project", "triage",
		"--name", "my-agent",
		"--display-name", "My Agent",
		"--type", "agent-api",
		"--repo-url", "https://github.com/example/repo",
		"--repo-branch", "main",
		"--repo-path", "/",
		"--build-type", "docker",
		"--dockerfile", "Dockerfile",
	}
}

// --- tests ---

func TestCreate_Buildpack_JSON(t *testing.T) {
	ios, out, _ := newTestIO(true)
	clientFn, captured, cleanup := newTestClient(t, 202, agentResponse())
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn)
	cmd.SetArgs(buildpackArgs())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if captured.method != http.MethodPost {
		t.Errorf("method = %q, want POST", captured.method)
	}
	if !strings.HasSuffix(captured.path, "/agents") {
		t.Errorf("path = %q, want suffix /agents", captured.path)
	}

	var reqBody amsvc.CreateAgentRequest
	if err := json.Unmarshal(captured.body, &reqBody); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	if reqBody.Name != "my-agent" {
		t.Errorf("request Name = %q", reqBody.Name)
	}
	if reqBody.AgentType.Type != "agent-api" {
		t.Errorf("request AgentType.Type = %q", reqBody.AgentType.Type)
	}

	env := decodeEnvelope(t, out.String())
	if env["org"] != "acme" || env["project"] != "triage" {
		t.Errorf("scope = org:%v project:%v", env["org"], env["project"])
	}
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data key, got %v", env)
	}
	if data["name"] != "my-agent" {
		t.Errorf("data.name = %v", data["name"])
	}
}

func TestCreate_Docker_JSON(t *testing.T) {
	ios, out, _ := newTestIO(true)
	clientFn, captured, cleanup := newTestClient(t, 202, agentResponse())
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn)
	cmd.SetArgs(dockerArgs())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]any
	json.Unmarshal(captured.body, &reqBody)
	build := reqBody["build"].(map[string]any)
	if build["type"] != "docker" {
		t.Errorf("build.type = %v", build["type"])
	}

	env := decodeEnvelope(t, out.String())
	if _, ok := env["data"]; !ok {
		t.Error("missing data key in response")
	}
}

func TestCreate_TextMode(t *testing.T) {
	ios, _, errOut := newTestIO(false)
	clientFn, _, cleanup := newTestClient(t, 202, agentResponse())
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn)
	cmd.SetArgs(buildpackArgs())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := errOut.String()
	if !strings.Contains(output, "Created agent my-agent") {
		t.Errorf("output missing success line, got:\n%s", output)
	}
	if !strings.Contains(output, "agent-api") {
		t.Errorf("output missing type, got:\n%s", output)
	}
}

func TestCreate_ChatAPI_RequestBody(t *testing.T) {
	ios, _, _ := newTestIO(true)
	clientFn, captured, cleanup := newTestClient(t, 202, agentResponse())
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn)
	cmd.SetArgs([]string{
		"agent", "create",
		"--project", "triage",
		"--name", "chat-bot",
		"--display-name", "Chat Bot",
		"--type", "chat-api",
		"--repo-url", "https://github.com/example/repo",
		"--repo-branch", "main",
		"--repo-path", "/",
		"--build-type", "buildpack",
		"--language", "python",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]any
	if err := json.Unmarshal(captured.body, &reqBody); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	iface := reqBody["inputInterface"].(map[string]any)
	if iface["type"] != "HTTP" {
		t.Errorf("inputInterface.type = %v, want HTTP", iface["type"])
	}
	if iface["port"].(float64) != 8000 {
		t.Errorf("inputInterface.port = %v, want 8000", iface["port"])
	}
	if _, ok := iface["basePath"]; ok {
		t.Errorf("inputInterface.basePath should be absent for chat-api, got %v", iface["basePath"])
	}
	if _, ok := iface["schema"]; ok {
		t.Errorf("inputInterface.schema should be absent for chat-api, got %v", iface["schema"])
	}
}

func TestCreate_400Error(t *testing.T) {
	ios, out, _ := newTestIO(true)
	errBody := amsvc.ErrorResponse{
		Code:    "VALIDATION_ERROR",
		Message: "name already exists",
	}
	clientFn, _, cleanup := newTestClient(t, 400, errBody)
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn)
	cmd.SetArgs(buildpackArgs())
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	env := decodeEnvelope(t, out.String())
	errMap := env["error"].(map[string]any)
	if errMap["code"] != "VALIDATION_ERROR" {
		t.Errorf("code = %v", errMap["code"])
	}
}

func TestCreate_409Conflict(t *testing.T) {
	ios, out, _ := newTestIO(true)
	errBody := amsvc.ErrorResponse{
		Code:    "CONFLICT",
		Message: "agent already exists",
	}
	clientFn, _, cleanup := newTestClient(t, 409, errBody)
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn)
	cmd.SetArgs(buildpackArgs())
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	env := decodeEnvelope(t, out.String())
	errMap := env["error"].(map[string]any)
	if errMap["code"] != "CONFLICT" {
		t.Errorf("code = %v", errMap["code"])
	}
}

func TestCreate_500Error(t *testing.T) {
	ios, out, _ := newTestIO(true)
	errBody := amsvc.ErrorResponse{
		Code:    "INTERNAL_ERROR",
		Message: "something went wrong",
	}
	clientFn, _, cleanup := newTestClient(t, 500, errBody)
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn)
	cmd.SetArgs(buildpackArgs())
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	env := decodeEnvelope(t, out.String())
	errMap := env["error"].(map[string]any)
	if errMap["code"] != "INTERNAL_ERROR" {
		t.Errorf("code = %v", errMap["code"])
	}
}

func TestCreate_ValidationError_JSON(t *testing.T) {
	ios, out, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil)
	cmd.SetArgs([]string{"agent", "create", "--project", "triage"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	env := decodeEnvelope(t, out.String())
	errMap := env["error"].(map[string]any)
	if errMap["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v, want %s", errMap["code"], clierr.InvalidFlag)
	}
	additional := errMap["additionalData"].(map[string]any)
	details, ok := additional["details"].([]any)
	if !ok {
		t.Fatalf("details type = %T", additional["details"])
	}
	if len(details) < 3 {
		t.Errorf("expected at least 3 violations, got %d: %v", len(details), details)
	}
}

func TestCreate_ValidationError_Text(t *testing.T) {
	ios, _, errOut := newTestIO(false)
	cmd := testCreateCmd(t, ios, nil)
	cmd.SetArgs([]string{"agent", "create", "--project", "triage"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	output := errOut.String()
	if !strings.Contains(output, "invalid flags") {
		t.Errorf("output missing 'invalid flags', got:\n%s", output)
	}
	if !strings.Contains(output, "--name is required") {
		t.Errorf("output missing '--name is required', got:\n%s", output)
	}
}

func TestCreate_ExternalProvisioning(t *testing.T) {
	ios, out, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil)
	cmd.SetArgs([]string{
		"agent", "create",
		"--project", "triage",
		"--name", "foo",
		"--display-name", "Foo",
		"--type", "agent-api",
		"--provisioning", "external",
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	env := decodeEnvelope(t, out.String())
	errMap := env["error"].(map[string]any)
	if errMap["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v", errMap["code"])
	}
	msg := errMap["message"].(string)
	if !strings.Contains(msg, "not yet supported") {
		t.Errorf("message = %q, want 'not yet supported'", msg)
	}
}

func TestCreate_UnknownProvisioning(t *testing.T) {
	ios, out, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil)
	cmd.SetArgs([]string{
		"agent", "create",
		"--project", "triage",
		"--name", "foo",
		"--display-name", "Foo",
		"--type", "agent-api",
		"--provisioning", "cloud",
	})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	env := decodeEnvelope(t, out.String())
	errMap := env["error"].(map[string]any)
	additional := errMap["additionalData"].(map[string]any)
	details := additional["details"].([]any)
	found := false
	for _, d := range details {
		if strings.Contains(d.(string), `"cloud"`) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected violation mentioning 'cloud', got %v", details)
	}
}
