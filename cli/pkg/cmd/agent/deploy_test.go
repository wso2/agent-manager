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

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func TestParseEnvFlag(t *testing.T) {
	cases := []struct {
		name    string
		inputs  []string
		want    map[string]string
		wantErr string
	}{
		{"single pair", []string{"A=1"}, map[string]string{"A": "1"}, ""},
		{"value with equals", []string{"URL=k=v"}, map[string]string{"URL": "k=v"}, ""},
		{"empty value", []string{"A="}, map[string]string{"A": ""}, ""},
		{"multiple", []string{"A=1", "B=2"}, map[string]string{"A": "1", "B": "2"}, ""},
		{"duplicate last-wins", []string{"A=1", "A=2"}, map[string]string{"A": "2"}, ""},
		{"empty key", []string{"=foo"}, nil, `invalid --env "=foo": empty key`},
		{"no equals", []string{"FOO"}, nil, `invalid --env "FOO": expected KEY=VALUE`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEnvFlag(tc.inputs)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want contains %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (got %v)", len(got), len(tc.want), got)
			}
			for k, v := range tc.want {
				if got[k] != v {
					t.Errorf("got[%q] = %q, want %q", k, got[k], v)
				}
			}
		})
	}
}

// recordedRequest captures a single inbound request body for assertions.
type recordedRequest struct {
	method string
	path   string
	body   []byte
}

type stubResponse struct {
	status int
	body   any
}

func newStubServer(t *testing.T, routes map[string]stubResponse, recorder *[]recordedRequest) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := readAllBody(r)
		*recorder = append(*recorder, recordedRequest{method: r.Method, path: r.URL.Path, body: body})
		key := r.Method + " " + r.URL.Path
		resp, ok := routes[key]
		if !ok {
			w.WriteHeader(500)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "NOT_STUBBED", "message": key})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(resp.status)
		if resp.body != nil {
			_ = json.NewEncoder(w).Encode(resp.body)
		}
	}))
}

func readAllBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	buf := &bytes.Buffer{}
	if _, err := buf.ReadFrom(r.Body); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func newTestDeployIO(canPrompt, jsonMode bool) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	io, _, out, errOut := iostreams.Test()
	io.SetTerminal(canPrompt, canPrompt, canPrompt)
	io.JSON = jsonMode
	return io, out, errOut
}

func stubBuildableAgent() map[string]any {
	return map[string]any{
		"name":         "order-bot",
		"displayName":  "Order Bot",
		"description":  "",
		"projectName":  "triage",
		"createdAt":    "2026-05-13T10:00:00Z",
		"provisioning": map[string]any{"type": "internal"},
		"agentType":    map[string]any{"type": "chat"},
		"uuid":         "00000000-0000-0000-0000-000000000001",
	}
}

func stubBuildsListLatest(name, imageID, status string) map[string]any {
	return map[string]any{
		"builds": []any{
			map[string]any{
				"agentName":       "order-bot",
				"projectName":     "triage",
				"buildName":       name,
				"imageId":         imageID,
				"startedAt":       "2026-05-13T11:00:00Z",
				"status":          status,
				"buildParameters": map[string]any{},
			},
		},
	}
}

func stubDeployAccepted(imageID string) map[string]any {
	return map[string]any{
		"agentName":   "order-bot",
		"projectName": "triage",
		"imageId":     imageID,
		"environment": "dev",
	}
}

func TestDeploy_LatestBuild_HappyPath(t *testing.T) {
	io, _, _ := newTestDeployIO(true, false)
	requests := []recordedRequest{}

	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":              {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":       {200, stubBuildsListLatest("b1", "image-sha-123", "Completed")},
		"POST /orgs/acme/projects/triage/agents/order-bot/deployments": {202, stubDeployAccepted("image-sha-123")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	opts := &DeployOptions{
		IO:        io,
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme",
		Proj:      "triage",
		AgentName: "order-bot",
	}
	if err := runDeploy(context.Background(), opts); err != nil {
		t.Fatalf("runDeploy: %v", err)
	}

	var deployBody []byte
	for _, r := range requests {
		if r.method == "POST" && strings.HasSuffix(r.path, "/deployments") {
			deployBody = r.body
		}
	}
	if deployBody == nil {
		t.Fatalf("POST /deployments not called; requests: %v", requests)
	}
	var sent map[string]any
	if err := json.Unmarshal(deployBody, &sent); err != nil {
		t.Fatalf("decode POST body: %v", err)
	}
	if sent["imageId"] != "image-sha-123" {
		t.Errorf("imageId = %v, want image-sha-123", sent["imageId"])
	}
	if envField, ok := sent["env"]; ok && envField != nil {
		if arr, isArr := envField.([]any); !isArr || len(arr) != 0 {
			t.Errorf("env = %v, want absent or empty", envField)
		}
	}
	if _, hasInstr := sent["enableAutoInstrumentation"]; hasInstr {
		t.Errorf("enableAutoInstrumentation must not be sent; server reads from DB")
	}
}

func TestDeploy_NoBuilds_ReturnsBuildNotDeployable(t *testing.T) {
	io, out, _ := newTestDeployIO(true, true)
	requests := []recordedRequest{}

	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":        {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds": {200, map[string]any{"builds": []any{}}},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO:     io,
		Client: func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:  baseScope(),
		Org:    "acme", Proj: "triage", AgentName: "order-bot",
	})
	if err == nil {
		t.Fatal("expected error")
	}

	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.BuildNotDeployable {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.BuildNotDeployable)
	}
	if !strings.Contains(errBody["message"].(string), "no builds found") {
		t.Errorf("message = %v, want 'no builds found' substring", errBody["message"])
	}
}

func TestDeploy_NotBuildable_PassthroughError(t *testing.T) {
	io, out, _ := newTestDeployIO(true, true)
	requests := []recordedRequest{}

	externalAgent := stubBuildableAgent()
	externalAgent["provisioning"] = map[string]any{"type": "external"}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot": {200, externalAgent},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO:     io,
		Client: func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:  baseScope(),
		Org:    "acme", Proj: "triage", AgentName: "order-bot",
	})
	if err == nil {
		t.Fatal("expected error")
	}

	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.Validation {
		t.Errorf("code = %v, want %s (ValidateBuildable returns Validation)", errBody["code"], clierr.Validation)
	}
}

func deployStatusErrorTest(t *testing.T, status, wantSubstr string) {
	t.Helper()
	io, out, _ := newTestDeployIO(true, true)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":        {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds": {200, stubBuildsListLatest("b1", "image-X", status)},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO:     io,
		Client: func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:  baseScope(),
		Org:    "acme", Proj: "triage", AgentName: "order-bot",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.BuildNotDeployable {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.BuildNotDeployable)
	}
	if !strings.Contains(errBody["message"].(string), wantSubstr) {
		t.Errorf("message = %v, want %q substring", errBody["message"], wantSubstr)
	}
}

func TestDeploy_BuildInProgress_Errors(t *testing.T) {
	deployStatusErrorTest(t, "BuildInProgress", "status=BuildInProgress")
}

func TestDeploy_BuildTriggered_Errors(t *testing.T) {
	deployStatusErrorTest(t, "BuildTriggered", "status=BuildTriggered")
}

func TestDeploy_BuildCompletedButNoImageID_Errors(t *testing.T) {
	io, out, _ := newTestDeployIO(true, true)
	requests := []recordedRequest{}
	noImg := map[string]any{
		"builds": []any{
			map[string]any{
				"agentName":       "order-bot",
				"projectName":     "triage",
				"buildName":       "b1",
				"status":          "Completed",
				"startedAt":       "2026-05-13T11:00:00Z",
				"buildParameters": map[string]any{},
				// imageId omitted
			},
		},
	}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":        {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds": {200, noImg},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO:     io,
		Client: func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:  baseScope(),
		Org:    "acme", Proj: "triage", AgentName: "order-bot",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.BuildNotDeployable {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.BuildNotDeployable)
	}
}

// envFromPost decodes the env field of a captured deploy POST body into key->value.
func envFromPost(t *testing.T, body []byte) map[string]string {
	t.Helper()
	var sent map[string]any
	if err := json.Unmarshal(body, &sent); err != nil {
		t.Fatalf("decode POST: %v", err)
	}
	out := map[string]string{}
	arr, _ := sent["env"].([]any)
	for _, e := range arr {
		m := e.(map[string]any)
		v := ""
		if val, ok := m["value"].(string); ok {
			v = val
		}
		out[m["key"].(string)] = v
	}
	return out
}

func captureDeployPost(requests []recordedRequest) []byte {
	for _, r := range requests {
		if r.method == "POST" && strings.HasSuffix(r.path, "/deployments") {
			return r.body
		}
	}
	return nil
}

func TestDeploy_EnvFlagsSentSorted(t *testing.T) {
	io, _, _ := newTestDeployIO(true, false)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":              {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":       {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"POST /orgs/acme/projects/triage/agents/order-bot/deployments": {202, stubDeployAccepted("img1")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO:     io,
		Client: func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:  baseScope(),
		Org:    "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags: []string{"B=2", "A=1"},
	})
	if err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	body := captureDeployPost(requests)
	got := envFromPost(t, body)
	if len(got) != 2 || got["A"] != "1" || got["B"] != "2" {
		t.Errorf("env = %v, want A=1, B=2", got)
	}
	if !bytes.Contains(body, []byte(`"key":"A"`)) || bytes.Index(body, []byte(`"key":"A"`)) > bytes.Index(body, []byte(`"key":"B"`)) {
		t.Errorf("env not sorted by key: %s", body)
	}
}

func TestDeploy_EnvFlag_MalformedEmptyKey(t *testing.T) {
	io, out, _ := newTestDeployIO(true, true)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":        {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds": {200, stubBuildsListLatest("b1", "img1", "Completed")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO:     io,
		Client: func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:  baseScope(),
		Org:    "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags: []string{"=foo"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.InvalidFlag)
	}
}

func TestDeploy_EnvFlag_MalformedNoEquals(t *testing.T) {
	io, out, _ := newTestDeployIO(true, true)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":        {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds": {200, stubBuildsListLatest("b1", "img1", "Completed")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO:     io,
		Client: func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:  baseScope(),
		Org:    "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags: []string{"FOO"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.InvalidFlag)
	}
}

// The server emits raw WorkflowRun phase strings ("Completed", "Succeeded",
// "Running", "Failed", "Pending") in BuildResponse.Status, not the spec's
// advertised BuildCompleted/BuildInProgress/BuildTriggered enum.
// Both "Completed" (workload CR updated) and "Succeeded" (image pushed,
// workload CR not yet updated) have an imageId populated and are deployable.

func TestDeploy_StatusCompleted_IsDeployable(t *testing.T) {
	io, _, _ := newTestDeployIO(true, false)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":              {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":       {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"POST /orgs/acme/projects/triage/agents/order-bot/deployments": {202, stubDeployAccepted("img1")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO:     io,
		Client: func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:  baseScope(),
		Org:    "acme", Proj: "triage", AgentName: "order-bot",
	})
	if err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	if captureDeployPost(requests) == nil {
		t.Fatal("expected POST /deployments to be issued for status=Completed")
	}
}

func TestDeploy_StatusSucceeded_IsDeployable(t *testing.T) {
	io, _, _ := newTestDeployIO(true, false)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":              {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":       {200, stubBuildsListLatest("b1", "img1", "Succeeded")},
		"POST /orgs/acme/projects/triage/agents/order-bot/deployments": {202, stubDeployAccepted("img1")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO:     io,
		Client: func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:  baseScope(),
		Org:    "acme", Proj: "triage", AgentName: "order-bot",
	})
	if err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	if captureDeployPost(requests) == nil {
		t.Fatal("expected POST /deployments to be issued for status=Succeeded")
	}
}

func TestDeploy_StatusRunning_Errors(t *testing.T) {
	deployStatusErrorTest(t, "Running", "status=Running")
}

func TestDeploy_StatusFailed_Errors(t *testing.T) {
	deployStatusErrorTest(t, "Failed", "status=Failed")
}
