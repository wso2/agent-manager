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
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clients/observersvc"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

// --- test helpers ---

type capturedRequest struct {
	method string
	path   string
	query  string
	body   []byte
	count  int
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

type routeResponse struct {
	Status int
	Body   any
}

// newTestRouter mounts multiple routes on one httptest server. Each captured
// request is keyed by route pattern. Patterns are matched with strings.HasPrefix
// against r.URL.Path, longest-first, so a more specific route (e.g.
// /agents/foo/token) wins over its prefix (/agents).
func newTestRouter(t *testing.T, routes map[string]routeResponse) (func(context.Context) (*amsvc.ClientWithResponses, error), map[string]*capturedRequest, func()) {
	t.Helper()
	keys := make([]string, 0, len(routes))
	for k := range routes {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })

	captured := map[string]*capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for _, pattern := range keys {
			if !strings.HasPrefix(r.URL.Path, pattern) {
				continue
			}
			route := routes[pattern]
			cap := &capturedRequest{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery}
			cap.body, _ = io.ReadAll(r.Body)
			if prev := captured[pattern]; prev != nil {
				cap.count = prev.count
			}
			cap.count++
			captured[pattern] = cap
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(route.Status)
			if route.Body != nil {
				_ = json.NewEncoder(w).Encode(route.Body)
			}
			return
		}
		// External agent creation resolves the lowest environment from the
		// deployment pipeline before minting a token. Tests that don't exercise
		// that resolution get a default single-environment pipeline so the token
		// flow still runs; tests that care route it explicitly above.
		if strings.HasSuffix(r.URL.Path, "/deployment-pipeline") {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(amsvc.DeploymentPipelineResponse{
				Name:           "default",
				PromotionPaths: []amsvc.PromotionPath{{SourceEnvironmentRef: "development"}},
			})
			return
		}
		t.Errorf("unrouted request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
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

func testCreateCmd(t *testing.T, ios *iostreams.IOStreams, clientFn func(context.Context) (*amsvc.ClientWithResponses, error), observerURL string) *cobra.Command {
	t.Helper()
	f := &cmdutil.Factory{
		IOStreams:    ios,
		AgentManager: clientFn,
		Observer: func(context.Context) (*observersvc.Client, error) {
			if observerURL == "" {
				return nil, nil
			}
			return observersvc.NewClient(observerURL)
		},
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
	// Mirror the production root's persistent --json flag (pkg/cmd/root.go) so
	// mode gating that checks Changed("json") is exercisable. Output JSON mode
	// is still driven by newTestIO, not this flag.
	root.PersistentFlags().Bool("json", false, "Output as JSON envelopes")
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
		"agent", "create", "my-agent",
		"--project", "triage",
		"--display-name", "My Agent",
		"--subtype", "chat-api",
		"--repo-url", "https://github.com/example/repo",
		"--repo-branch", "main",
		"--repo-path", "/",
		"--build-type", "buildpack",
		"--language", "go",
		"--language-version", "1.22",
		"--run-command", "go run .",
	}
}

func dockerArgs() []string {
	return []string{
		"agent", "create", "my-agent",
		"--project", "triage",
		"--display-name", "My Agent",
		"--subtype", "chat-api",
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

	cmd := testCreateCmd(t, ios, clientFn, "")
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
		t.Errorf("request AgentType.Type = %q, want agent-api (derived from internal provisioning)", reqBody.AgentType.Type)
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

	cmd := testCreateCmd(t, ios, clientFn, "")
	cmd.SetArgs(dockerArgs())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]any
	if err := json.Unmarshal(captured.body, &reqBody); err != nil {
		t.Fatalf("decode request: %v", err)
	}
	build, ok := reqBody["build"].(map[string]any)
	if !ok {
		t.Fatalf("request missing build object: %v", reqBody)
	}
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

	cmd := testCreateCmd(t, ios, clientFn, "")
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

	cmd := testCreateCmd(t, ios, clientFn, "")
	cmd.SetArgs([]string{
		"agent", "create", "chat-bot",
		"--project", "triage",
		"--display-name", "Chat Bot",
		"--subtype", "chat-api",
		"--repo-url", "https://github.com/example/repo",
		"--repo-branch", "main",
		"--repo-path", "/",
		"--build-type", "buildpack",
		"--language", "python",
		"--language-version", "3.12",
		"--run-command", "python main.py",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var reqBody map[string]any
	if err := json.Unmarshal(captured.body, &reqBody); err != nil {
		t.Fatalf("decode request: %v", err)
	}

	at := reqBody["agentType"].(map[string]any)
	if at["type"] != "agent-api" {
		t.Errorf("agentType.type = %v, want agent-api (derived from internal provisioning)", at["type"])
	}
	if at["subType"] != "chat-api" {
		t.Errorf("agentType.subType = %v, want chat-api", at["subType"])
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

	cmd := testCreateCmd(t, ios, clientFn, "")
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

	cmd := testCreateCmd(t, ios, clientFn, "")
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

	cmd := testCreateCmd(t, ios, clientFn, "")
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
	cmd := testCreateCmd(t, ios, nil, "")
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
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{"agent", "create", "--project", "triage"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	output := errOut.String()
	if !strings.Contains(output, "invalid agent spec") {
		t.Errorf("output missing 'invalid agent spec', got:\n%s", output)
	}
	if !strings.Contains(output, "spec.name is required") {
		t.Errorf("output missing 'spec.name is required', got:\n%s", output)
	}
}

func TestCreate_UnknownProvisioning(t *testing.T) {
	ios, out, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{
		"agent", "create", "foo",
		"--project", "triage",
		"--display-name", "Foo",
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

func TestCreate_MissingName_BatchedError(t *testing.T) {
	ios, out, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil, "")
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
	foundName := false
	foundDisplayName := false
	for _, d := range details {
		s := d.(string)
		if strings.Contains(s, "spec.name is required") {
			foundName = true
		}
		if strings.Contains(s, "spec.displayName is required") {
			foundDisplayName = true
		}
	}
	if !foundName {
		t.Errorf("expected 'spec.name is required' in details, got %v", details)
	}
	if !foundDisplayName {
		t.Errorf("expected 'spec.displayName is required' in details, got %v", details)
	}
}

func TestCreate_External_Text(t *testing.T) {
	ios, _, errOut := newTestIO(false)
	routes := map[string]routeResponse{
		"/orgs/acme/projects/triage/agents/testing/token": {
			Status: 200,
			Body: amsvc.TokenResponse{
				Token:     "tok-abc",
				ExpiresAt: 1700000000,
				IssuedAt:  1690000000,
				TokenType: "Bearer",
			},
		},
		"/orgs/acme/projects/triage/agents": {
			Status: 202,
			Body: amsvc.AgentResponse{
				Name:         "testing",
				DisplayName:  "Testing",
				Description:  "dssdf",
				AgentType:    amsvc.AgentType{Type: "external-agent-api"},
				Provisioning: amsvc.Provisioning{Type: amsvc.ProvisioningTypeExternal},
				ProjectName:  "triage",
				Uuid:         "uuid-ext",
				CreatedAt:    time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC),
			},
		},
	}
	clientFn, captured, cleanup := newTestRouter(t, routes)
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn, "https://otel.example")
	cmd.SetArgs([]string{
		"agent", "create", "testing",
		"--project", "triage",
		"--display-name", "Testing",
		"--description", "dssdf",
		"--provisioning", "external",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	createReq, ok := captured["/orgs/acme/projects/triage/agents"]
	if !ok {
		t.Fatal("no create request captured")
	}
	var body map[string]any
	if err := json.Unmarshal(createReq.body, &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	prov := body["provisioning"].(map[string]any)
	if prov["type"] != "external" {
		t.Errorf("provisioning.type = %v, want external", prov["type"])
	}
	if _, ok := prov["repository"]; ok {
		t.Errorf("provisioning.repository should be absent, got %v", prov["repository"])
	}
	for _, k := range []string{"build", "inputInterface", "configurations"} {
		if _, ok := body[k]; ok {
			t.Errorf("body.%s should be absent for external, got %v", k, body[k])
		}
	}

	if _, ok := captured["/orgs/acme/projects/triage/agents/testing/token"]; !ok {
		t.Error("no token request captured")
	}

	out := errOut.String()
	for _, sub := range []string{
		"Created agent testing",
		"Provisioning: external",
		`export AMP_OTEL_ENDPOINT="https://otel.example/v1/traces"`,
		`export AMP_AGENT_API_KEY="tok-abc"`,
		"amp-instrument <your_existing_start_command>",
	} {
		if !strings.Contains(out, sub) {
			t.Errorf("output missing %q\n---\n%s", sub, out)
		}
	}
}

func TestCreate_External_JSON(t *testing.T) {
	ios, out, _ := newTestIO(true)
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
			},
		},
	}
	clientFn, _, cleanup := newTestRouter(t, routes)
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn, "https://otel.example")
	cmd.SetArgs([]string{
		"agent", "create", "testing",
		"--project", "triage",
		"--display-name", "Testing",
		"--provisioning", "external",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	env := decodeEnvelope(t, out.String())
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data: %v", env)
	}
	if data["token"] != "tok-abc" {
		t.Errorf("data.token = %v, want tok-abc", data["token"])
	}
	if data["otelEndpoint"] != "https://otel.example/v1/traces" {
		t.Errorf("data.otelEndpoint = %v", data["otelEndpoint"])
	}
	if _, ok := data["instrumentationInstructions"].(string); !ok {
		t.Errorf("data.instrumentationInstructions missing or not a string: %v", data["instrumentationInstructions"])
	}
}

// External agent creation must resolve the lowest (entry) environment of the
// project's deployment pipeline and send it on the token request, mirroring how
// internal agent deploys pick their target environment. Without it the server
// has no environment to resolve for the token claims and fails.
func TestCreate_External_ResolvesLowestEnvironment(t *testing.T) {
	ios, _, _ := newTestIO(false)
	routes := map[string]routeResponse{
		"/orgs/acme/projects/triage/agents/testing/token": {
			Status: 200,
			Body:   amsvc.TokenResponse{Token: "tok-abc", ExpiresAt: 1700000000, IssuedAt: 1690000000, TokenType: "Bearer"},
		},
		"/orgs/acme/projects/triage/deployment-pipeline": {
			Status: 200,
			Body: amsvc.DeploymentPipelineResponse{
				Name: "default",
				PromotionPaths: []amsvc.PromotionPath{
					{
						SourceEnvironmentRef:  "development",
						TargetEnvironmentRefs: []amsvc.TargetEnvironmentRef{{Name: "production"}},
					},
				},
			},
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
			},
		},
	}
	clientFn, captured, cleanup := newTestRouter(t, routes)
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn, "https://otel.example")
	cmd.SetArgs([]string{
		"agent", "create", "testing",
		"--project", "triage",
		"--display-name", "Testing",
		"--provisioning", "external",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := captured["/orgs/acme/projects/triage/deployment-pipeline"]; !ok {
		t.Fatal("deployment pipeline was not fetched to resolve the environment")
	}
	tokenReq, ok := captured["/orgs/acme/projects/triage/agents/testing/token"]
	if !ok {
		t.Fatal("no token request captured")
	}
	if got := tokenReq.query; got != "environment=development" {
		t.Errorf("token request query = %q, want environment=development", got)
	}
}

// Agent creation succeeds server-side; the token mint fails. The CLI must
// exit 0 with a warning so the user does not retry into a 409.
func TestCreate_External_TokenFailure_WarnsAndSucceeds(t *testing.T) {
	ios, out, errOut := newTestIO(true)
	routes := map[string]routeResponse{
		"/orgs/acme/projects/triage/agents/testing/token": {
			Status: 500,
			Body:   amsvc.ErrorResponse{Code: "INTERNAL", Message: "boom"},
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
			},
		},
	}
	clientFn, _, cleanup := newTestRouter(t, routes)
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn, "https://otel.example")
	cmd.SetArgs([]string{
		"agent", "create", "testing",
		"--project", "triage",
		"--display-name", "Testing",
		"--provisioning", "external",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error (post-create failure is a warning), got %v", err)
	}

	if !strings.Contains(errOut.String(), "warning:") {
		t.Errorf("stderr missing warning prefix: %q", errOut.String())
	}

	env := decodeEnvelope(t, out.String())
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data envelope: %v", env)
	}
	agent, ok := data["agent"].(map[string]any)
	if !ok {
		t.Fatalf("data.agent missing or wrong shape (success and failure envelopes must share the .data.agent selector): %v", data)
	}
	if agent["name"] != "testing" {
		t.Errorf("data.agent.name = %v, want testing", agent["name"])
	}
	if _, hasToken := data["token"]; hasToken {
		t.Errorf("data.token should be absent on failure fallback: %v", data["token"])
	}
}

func TestCreate_External_TokenFailure_TextMode(t *testing.T) {
	ios, _, errOut := newTestIO(false)
	routes := map[string]routeResponse{
		"/orgs/acme/projects/triage/agents/testing/token": {
			Status: 500,
			Body:   amsvc.ErrorResponse{Code: "INTERNAL", Message: "boom"},
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
			},
		},
	}
	clientFn, _, cleanup := newTestRouter(t, routes)
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn, "https://otel.example")
	cmd.SetArgs([]string{
		"agent", "create", "testing",
		"--project", "triage",
		"--display-name", "Testing",
		"--provisioning", "external",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error (post-create failure is a warning), got %v", err)
	}

	stderr := errOut.String()
	if !strings.Contains(stderr, "Created agent testing") {
		t.Errorf("stderr missing success line: %q", stderr)
	}
	if !strings.Contains(stderr, "warning:") {
		t.Errorf("stderr missing warning prefix: %q", stderr)
	}
	if strings.Contains(stderr, "amp-instrument") {
		t.Errorf("stderr should not contain instrumentation block when token mint failed: %q", stderr)
	}
}

func externalA2AAgentRoutes() map[string]routeResponse {
	return map[string]routeResponse{
		"/orgs/acme/projects/triage/agents/testing/token": {
			Status: 200,
			Body:   amsvc.TokenResponse{Token: "tok-abc", ExpiresAt: 1700000000, IssuedAt: 1690000000, TokenType: "Bearer"},
		},
		"/orgs/acme/projects/triage/agents": {
			Status: 202,
			Body: amsvc.AgentResponse{
				Name:         "testing",
				DisplayName:  "Testing",
				AgentType:    amsvc.AgentType{Type: "external-agent-api", SubType: ptr("a2a-agent")},
				Provisioning: amsvc.Provisioning{Type: amsvc.ProvisioningTypeExternal},
				ProjectName:  "triage",
				Uuid:         "u",
			},
		},
	}
}

func ptr[T any](v T) *T { return &v }

const testCardURL = "https://a.example/.well-known/agent-card.json"

func TestCreate_External_A2ASubtype(t *testing.T) {
	ios, _, _ := newTestIO(false)
	clientFn, captured, cleanup := newTestRouter(t, externalA2AAgentRoutes())
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn, "https://otel.example")
	cmd.SetArgs([]string{
		"agent", "create", "testing", "--project", "triage", "--display-name", "Testing",
		"--provisioning", "external", "--subtype", "a2a-agent",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var body struct {
		AgentType struct {
			SubType string `json:"subType"`
		} `json:"agentType"`
	}
	if err := json.Unmarshal(captured["/orgs/acme/projects/triage/agents"].body, &body); err != nil {
		t.Fatal(err)
	}
	if body.AgentType.SubType != "a2a-agent" {
		t.Errorf("subType = %q, want a2a-agent", body.AgentType.SubType)
	}
}

func TestCreate_External_CardURL_JSON(t *testing.T) {
	ios, out, _ := newTestIO(true)
	routes := externalA2AAgentRoutes()
	routes["/orgs/acme/projects/triage/agents/testing/environments/development/agent-card/source"] = routeResponse{Status: 202, Body: map[string]string{}}
	routes["/orgs/acme/projects/triage/agents/testing/environments/development/a2a-proxy"] = routeResponse{Status: 404, Body: amsvc.ErrorResponse{Code: "NOT_FOUND", Message: "none"}}
	routes["/orgs/acme/projects/triage/deployment-pipeline"] = routeResponse{
		Status: 200,
		Body: amsvc.DeploymentPipelineResponse{
			Name:           "default",
			PromotionPaths: []amsvc.PromotionPath{{SourceEnvironmentRef: "development"}},
		},
	}
	clientFn, captured, cleanup := newTestRouter(t, routes)
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn, "https://otel.example")
	cmd.SetArgs([]string{
		"agent", "create", "testing", "--project", "triage", "--display-name", "Testing",
		"--provisioning", "external", "--subtype", "a2a-agent", "--card-url", testCardURL, "--json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	src := captured["/orgs/acme/projects/triage/agents/testing/environments/development/agent-card/source"]
	if src == nil {
		t.Fatal("card source was not set")
	}
	if want := `{"url":"` + testCardURL + `"}`; strings.TrimSpace(string(src.body)) != want {
		t.Errorf("source body = %s, want %s", src.body, want)
	}
	if n := captured["/orgs/acme/projects/triage/deployment-pipeline"].count; n != 1 {
		t.Errorf("pipeline requested %d times, want 1", n)
	}
	data := decodeEnvelope(t, out.String())["data"].(map[string]any)
	card, _ := data["card"].(map[string]any)
	if card["environment"] != "development" || card["url"] != testCardURL || card["routing"] != "direct" {
		t.Errorf("data.card = %v", data["card"])
	}
}

func cardFailureRoutes() map[string]routeResponse {
	routes := externalA2AAgentRoutes()
	routes["/orgs/acme/projects/triage/agents/testing/environments/development/agent-card/source"] = routeResponse{
		Status: 400,
		Body:   map[string]string{"message": "invalid URL: url host must be publicly reachable"},
	}
	return routes
}

func TestCreate_External_CardURLFailure_JSON(t *testing.T) {
	ios, out, _ := newTestIO(true)
	clientFn, _, cleanup := newTestRouter(t, cardFailureRoutes())
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn, "https://otel.example")
	cmd.SetArgs([]string{
		"agent", "create", "testing", "--project", "triage", "--display-name", "Testing",
		"--provisioning", "external", "--subtype", "a2a-agent", "--card-url", testCardURL, "--json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	data := decodeEnvelope(t, out.String())["data"].(map[string]any)
	card, _ := data["card"].(map[string]any)
	if msg, _ := card["error"].(string); !strings.Contains(msg, "publicly reachable") {
		t.Errorf("data.card.error = %v", card["error"])
	}
	if data["token"] != "tok-abc" {
		t.Errorf("data.token = %v, want tok-abc", data["token"])
	}
}

func TestCreate_External_CardURLFailure_Text(t *testing.T) {
	ios, _, errOut := newTestIO(false)
	clientFn, _, cleanup := newTestRouter(t, cardFailureRoutes())
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn, "https://otel.example")
	cmd.SetArgs([]string{
		"agent", "create", "testing", "--project", "triage", "--display-name", "Testing",
		"--provisioning", "external", "--subtype", "a2a-agent", "--card-url", testCardURL,
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	stderr := errOut.String()
	for _, want := range []string{"warning: agent created but card setup failed", "amctl agent card set testing --env development --url '" + testCardURL + "'"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr missing %q: %q", want, stderr)
		}
	}
}

func TestCreate_FileMode_RejectsCardURL(t *testing.T) {
	ios, _, _ := newTestIO(true)
	cmd := testCreateCmd(t, ios, nil, "")
	cmd.SetArgs([]string{"agent", "create", "-f", "x.yaml", "--project", "triage", "--card-url", testCardURL})
	details := mustFlagDetails(t, cmd.Execute())
	assertContains(t, details, "--card-url is not allowed with --file")
}

func TestCreate_External_EnvResolutionFailure_CardURL_JSON(t *testing.T) {
	ios, out, errOut := newTestIO(true)
	routes := externalA2AAgentRoutes()
	routes["/orgs/acme/projects/triage/deployment-pipeline"] = routeResponse{Status: 500, Body: amsvc.ErrorResponse{Code: "INTERNAL", Message: "boom"}}
	clientFn, captured, cleanup := newTestRouter(t, routes)
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn, "https://otel.example")
	cmd.SetArgs([]string{
		"agent", "create", "testing", "--project", "triage", "--display-name", "Testing",
		"--provisioning", "external", "--subtype", "a2a-agent", "--card-url", testCardURL, "--json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !strings.Contains(errOut.String(), "could not resolve the entry environment (token and card setup skipped)") {
		t.Errorf("stderr = %q", errOut.String())
	}
	data := decodeEnvelope(t, out.String())["data"].(map[string]any)
	if data["agent"] == nil {
		t.Errorf("data.agent missing: %v", data)
	}
	card, _ := data["card"].(map[string]any)
	if msg, _ := card["error"].(string); msg == "" {
		t.Errorf("data.card.error empty: %v", data["card"])
	}
	if _, ok := data["token"]; ok {
		t.Errorf("data.token present: %v", data["token"])
	}
	if captured["/orgs/acme/projects/triage/agents/testing/environments/development/agent-card/source"] != nil {
		t.Error("card source was requested")
	}
}

func TestCreate_External_CardURL_TokenFailure_JSON(t *testing.T) {
	ios, out, _ := newTestIO(true)
	routes := externalA2AAgentRoutes()
	routes["/orgs/acme/projects/triage/agents/testing/token"] = routeResponse{Status: 500, Body: amsvc.ErrorResponse{Code: "INTERNAL", Message: "boom"}}
	routes["/orgs/acme/projects/triage/agents/testing/environments/development/agent-card/source"] = routeResponse{Status: 202, Body: map[string]string{}}
	routes["/orgs/acme/projects/triage/agents/testing/environments/development/a2a-proxy"] = routeResponse{Status: 404, Body: amsvc.ErrorResponse{Code: "NOT_FOUND", Message: "none"}}
	clientFn, _, cleanup := newTestRouter(t, routes)
	defer cleanup()

	cmd := testCreateCmd(t, ios, clientFn, "https://otel.example")
	cmd.SetArgs([]string{
		"agent", "create", "testing", "--project", "triage", "--display-name", "Testing",
		"--provisioning", "external", "--subtype", "a2a-agent", "--card-url", testCardURL, "--json",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	data := decodeEnvelope(t, out.String())["data"].(map[string]any)
	if data["agent"] == nil {
		t.Errorf("data.agent missing: %v", data)
	}
	card, _ := data["card"].(map[string]any)
	if card["routing"] != "direct" {
		t.Errorf("data.card.routing = %v, want direct", card["routing"])
	}
}
