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
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func boolPtr(b bool) *bool { return &b }

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

func TestFindLowestEnvironment(t *testing.T) {
	cases := []struct {
		name  string
		paths []gen.PromotionPath
		want  string
	}{
		{
			name: "linear dev->staging->prod, dev is entry",
			paths: []gen.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "staging"}}},
				{SourceEnvironmentRef: "staging", TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "prod"}}},
			},
			want: "dev",
		},
		{
			name:  "empty pipeline",
			paths: nil,
			want:  "",
		},
		{
			name: "single path dev->prod",
			paths: []gen.PromotionPath{
				{SourceEnvironmentRef: "dev", TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "prod"}}},
			},
			want: "dev",
		},
		{
			name: "every source is also a target (cycle) -> empty",
			paths: []gen.PromotionPath{
				{SourceEnvironmentRef: "a", TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "b"}}},
				{SourceEnvironmentRef: "b", TargetEnvironmentRefs: []gen.TargetEnvironmentRef{{Name: "a"}}},
			},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := findLowestEnvironment(tc.paths)
			if got != tc.want {
				t.Errorf("findLowestEnvironment = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestMergeEnv(t *testing.T) {
	type result struct {
		final     map[string]string
		conflicts []string
	}
	cases := []struct {
		name    string
		current []gen.ConfigurationItem
		cli     map[string]string
		want    result
	}{
		{
			name:    "no current, no cli",
			current: nil, cli: nil,
			want: result{final: map[string]string{}, conflicts: nil},
		},
		{
			name: "preserve current when cli absent",
			current: []gen.ConfigurationItem{
				{Key: "A", Value: "1"},
				{Key: "B", Value: "2"},
			},
			cli:  nil,
			want: result{final: map[string]string{"A": "1", "B": "2"}, conflicts: nil},
		},
		{
			name:    "add new cli key",
			current: []gen.ConfigurationItem{{Key: "A", Value: "1"}},
			cli:     map[string]string{"B": "2"},
			want:    result{final: map[string]string{"A": "1", "B": "2"}, conflicts: nil},
		},
		{
			name:    "same value is not a conflict",
			current: []gen.ConfigurationItem{{Key: "A", Value: "1"}},
			cli:     map[string]string{"A": "1"},
			want:    result{final: map[string]string{"A": "1"}, conflicts: nil},
		},
		{
			name:    "different value is a conflict",
			current: []gen.ConfigurationItem{{Key: "A", Value: "1"}},
			cli:     map[string]string{"A": "2"},
			want:    result{final: map[string]string{"A": "2"}, conflicts: []string{"A"}},
		},
		{
			name: "sensitive current key always conflicts when cli sets it",
			current: []gen.ConfigurationItem{
				{Key: "SECRET", Value: "", IsSensitive: boolPtr(true)},
			},
			cli:  map[string]string{"SECRET": "new"},
			want: result{final: map[string]string{"SECRET": "new"}, conflicts: []string{"SECRET"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			final, conflicts := mergeEnv(tc.current, tc.cli)

			gotFinal := map[string]string{}
			for _, ev := range final {
				if ev.Value == nil {
					gotFinal[ev.Key] = ""
				} else {
					gotFinal[ev.Key] = *ev.Value
				}
			}
			if len(gotFinal) != len(tc.want.final) {
				t.Errorf("final size = %d, want %d (%v vs %v)", len(gotFinal), len(tc.want.final), gotFinal, tc.want.final)
			}
			for k, v := range tc.want.final {
				if gotFinal[k] != v {
					t.Errorf("final[%q] = %q, want %q", k, gotFinal[k], v)
				}
			}

			gotConflicts := make([]string, 0, len(conflicts))
			for _, c := range conflicts {
				gotConflicts = append(gotConflicts, c.Key)
			}
			sort.Strings(gotConflicts)
			wantConflicts := append([]string{}, tc.want.conflicts...)
			sort.Strings(wantConflicts)
			if len(gotConflicts) != len(wantConflicts) {
				t.Fatalf("conflicts = %v, want %v", gotConflicts, wantConflicts)
			}
			for i := range gotConflicts {
				if gotConflicts[i] != wantConflicts[i] {
					t.Errorf("conflicts[%d] = %q, want %q", i, gotConflicts[i], wantConflicts[i])
				}
			}
		})
	}
}

func TestRenderConflictTable_PlainOnly(t *testing.T) {
	io, _, _, errOut := iostreams.Test()
	io.SetTerminal(true, true, true)
	conflicts := []envConflict{
		{Key: "OPENAI_MODEL", CurrentValue: "gpt-4o-mini", NewValue: "gpt-4o", CurrentSensitive: false},
	}
	renderConflictTable(io, conflicts)
	out := errOut.String()
	if strings.Contains(out, "secret") {
		t.Errorf("plain-only render should not mention secrets, got: %q", out)
	}
	for _, want := range []string{"OPENAI_MODEL", "gpt-4o-mini", "gpt-4o"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %q", want, out)
		}
	}
}

func TestRenderConflictTable_SensitiveBanner(t *testing.T) {
	io, _, _, errOut := iostreams.Test()
	io.SetTerminal(true, true, true)
	conflicts := []envConflict{
		{Key: "PINECONE_API_KEY", CurrentValue: "", NewValue: "new-secret-value", CurrentSensitive: true},
	}
	renderConflictTable(io, conflicts)
	out := errOut.String()
	if !strings.Contains(out, "PINECONE_API_KEY") {
		t.Errorf("banner should name affected key, got: %q", out)
	}
	if !strings.Contains(out, "secret") {
		t.Errorf("banner should mention secret demotion, got: %q", out)
	}
	if !strings.Contains(out, "(secret)") {
		t.Errorf("table should render current sensitive value as (secret), got: %q", out)
	}
	if !strings.Contains(out, "***") {
		t.Errorf("table should render incoming sensitive value as ***, got: %q", out)
	}
	if strings.Contains(out, "new-secret-value") {
		t.Errorf("table must NOT echo incoming CLI value for sensitive key, got: %q", out)
	}
}

// fakeDeployPrompter records Confirm calls and returns a canned answer.
type fakeDeployPrompter struct {
	confirmCalls  int
	confirmAnswer bool
	confirmErr    error
	confirmPrompt string
}

func (p *fakeDeployPrompter) ConfirmDeletion(required string) error { return nil }
func (p *fakeDeployPrompter) Confirm(prompt string) (bool, error) {
	p.confirmCalls++
	p.confirmPrompt = prompt
	return p.confirmAnswer, p.confirmErr
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
		"name":        "order-bot",
		"displayName": "Order Bot",
		"description": "",
		"projectName": "triage",
		"createdAt":   "2026-05-13T10:00:00Z",
		"provisioning": map[string]any{"type": "internal"},
		"agentType":   map[string]any{"type": "chat"},
		"uuid":        "00000000-0000-0000-0000-000000000001",
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

func stubPipelineDevToProd() map[string]any {
	return map[string]any{
		"name":        "default-pipeline",
		"displayName": "Default",
		"description": "",
		"orgName":     "acme",
		"createdAt":   "2026-05-13T09:00:00Z",
		"promotionPaths": []any{
			map[string]any{
				"sourceEnvironmentRef":  "dev",
				"targetEnvironmentRefs": []any{map[string]any{"name": "prod"}},
			},
		},
	}
}

func stubConfigurations(items []map[string]any) map[string]any {
	if items == nil {
		items = []map[string]any{}
	}
	return map[string]any{
		"agentName":      "order-bot",
		"projectName":    "triage",
		"environment":    "dev",
		"configurations": items,
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
	prompter := &fakeDeployPrompter{}
	requests := []recordedRequest{}

	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":                {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":         {200, stubBuildsListLatest("b1", "image-sha-123", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":             {200, stubPipelineDevToProd()},
		"GET /orgs/acme/projects/triage/agents/order-bot/configurations": {200, stubConfigurations(nil)},
		"POST /orgs/acme/projects/triage/agents/order-bot/deployments":   {202, stubDeployAccepted("image-sha-123")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	opts := &DeployOptions{
		IO:        io,
		Prompter:  prompter,
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
	if prompter.confirmCalls != 0 {
		t.Errorf("confirm should not be called for no-conflict deploy; got %d", prompter.confirmCalls)
	}
}

func TestDeploy_NoBuilds_ReturnsBuildNotDeployable(t *testing.T) {
	io, out, _ := newTestDeployIO(true, true)
	prompter := &fakeDeployPrompter{}
	requests := []recordedRequest{}

	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":        {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds": {200, map[string]any{"builds": []any{}}},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: prompter,
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
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
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
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

func TestDeploy_EmptyPipeline_ReturnsInternal(t *testing.T) {
	io, out, _ := newTestDeployIO(true, true)
	requests := []recordedRequest{}

	emptyPipeline := stubPipelineDevToProd()
	emptyPipeline["promotionPaths"] = []any{}

	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":        {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds": {200, stubBuildsListLatest("b1", "image-sha-123", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":     {200, emptyPipeline},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
	})
	if err == nil {
		t.Fatal("expected error")
	}

	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.Internal {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.Internal)
	}
	if !strings.Contains(errBody["message"].(string), "no entry environment") {
		t.Errorf("message = %v, want 'no entry environment' substring", errBody["message"])
	}
}

// stubBuildDetails is the response shape for GetBuild (single build).
func stubBuildDetails(name, imageID, status string) map[string]any {
	out := map[string]any{
		"agentName":       "order-bot",
		"projectName":     "triage",
		"buildName":       name,
		"startedAt":       "2026-05-13T11:00:00Z",
		"buildParameters": map[string]any{},
	}
	if imageID != "" {
		out["imageId"] = imageID
	}
	if status != "" {
		out["status"] = status
	}
	return out
}

// stubConfigsWithKVs builds a configurations response from (key, value) pairs.
func stubConfigsWithKVs(kvs ...[2]string) map[string]any {
	items := make([]map[string]any, 0, len(kvs))
	for _, kv := range kvs {
		items = append(items, map[string]any{"key": kv[0], "value": kv[1]})
	}
	return stubConfigurations(items)
}

func TestDeploy_BuildNameSuccess_PreservesExistingEnv(t *testing.T) {
	io, _, _ := newTestDeployIO(true, false)
	requests := []recordedRequest{}

	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":                   {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds/specific-b": {200, stubBuildDetails("specific-b", "image-X", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":                {200, stubPipelineDevToProd()},
		"GET /orgs/acme/projects/triage/agents/order-bot/configurations":    {200, stubConfigsWithKVs([2]string{"FOO", "1"}, [2]string{"BAR", "2"})},
		"POST /orgs/acme/projects/triage/agents/order-bot/deployments":      {202, stubDeployAccepted("image-X")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
		BuildName: "specific-b",
	})
	if err != nil {
		t.Fatalf("runDeploy: %v", err)
	}

	var post []byte
	for _, r := range requests {
		if r.method == "POST" && strings.HasSuffix(r.path, "/deployments") {
			post = r.body
		}
	}
	var sent map[string]any
	_ = json.Unmarshal(post, &sent)
	if sent["imageId"] != "image-X" {
		t.Errorf("imageId = %v, want image-X", sent["imageId"])
	}
	envArr, _ := sent["env"].([]any)
	gotKeys := map[string]string{}
	for _, e := range envArr {
		m := e.(map[string]any)
		if v, ok := m["value"].(string); ok {
			gotKeys[m["key"].(string)] = v
		}
	}
	if gotKeys["FOO"] != "1" || gotKeys["BAR"] != "2" {
		t.Errorf("env should preserve FOO=1, BAR=2; got %v", gotKeys)
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
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
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
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
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

func TestDeploy_EnvNewKey_NoConflict(t *testing.T) {
	io, _, _ := newTestDeployIO(true, false)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":                {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":         {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":             {200, stubPipelineDevToProd()},
		"GET /orgs/acme/projects/triage/agents/order-bot/configurations": {200, stubConfigsWithKVs([2]string{"EXISTING", "1"})},
		"POST /orgs/acme/projects/triage/agents/order-bot/deployments":   {202, stubDeployAccepted("img1")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	prompter := &fakeDeployPrompter{}
	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: prompter,
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags:  []string{"NEW_KEY=hello"},
	})
	if err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	got := envFromPost(t, captureDeployPost(requests))
	if got["EXISTING"] != "1" || got["NEW_KEY"] != "hello" {
		t.Errorf("env = %v, want EXISTING=1, NEW_KEY=hello", got)
	}
	if prompter.confirmCalls != 0 {
		t.Errorf("no prompt expected when no conflict")
	}
}

func TestDeploy_EnvConflict_YesFlagBypassesPrompt(t *testing.T) {
	io, _, _ := newTestDeployIO(true, false)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":                {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":         {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":             {200, stubPipelineDevToProd()},
		"GET /orgs/acme/projects/triage/agents/order-bot/configurations": {200, stubConfigsWithKVs([2]string{"A", "1"})},
		"POST /orgs/acme/projects/triage/agents/order-bot/deployments":   {202, stubDeployAccepted("img1")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	prompter := &fakeDeployPrompter{}
	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: prompter,
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags:  []string{"A=2"},
		Yes:       true,
	})
	if err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	got := envFromPost(t, captureDeployPost(requests))
	if got["A"] != "2" {
		t.Errorf("env[A] = %q, want 2", got["A"])
	}
	if prompter.confirmCalls != 0 {
		t.Errorf("--yes should bypass prompt; got %d calls", prompter.confirmCalls)
	}
}

func TestDeploy_EnvConflict_PromptAccepted(t *testing.T) {
	io, _, _ := newTestDeployIO(true, false)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":                {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":         {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":             {200, stubPipelineDevToProd()},
		"GET /orgs/acme/projects/triage/agents/order-bot/configurations": {200, stubConfigsWithKVs([2]string{"A", "1"})},
		"POST /orgs/acme/projects/triage/agents/order-bot/deployments":   {202, stubDeployAccepted("img1")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	prompter := &fakeDeployPrompter{confirmAnswer: true}
	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: prompter,
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags:  []string{"A=2"},
	})
	if err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	if prompter.confirmCalls != 1 {
		t.Errorf("expected 1 confirm call, got %d", prompter.confirmCalls)
	}
	if captureDeployPost(requests) == nil {
		t.Fatal("expected POST to be sent after accepted prompt")
	}
}

func TestDeploy_EnvConflict_PromptDeclined(t *testing.T) {
	io, _, _ := newTestDeployIO(true, false)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":                {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":         {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":             {200, stubPipelineDevToProd()},
		"GET /orgs/acme/projects/triage/agents/order-bot/configurations": {200, stubConfigsWithKVs([2]string{"A", "1"})},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	prompter := &fakeDeployPrompter{confirmAnswer: false}
	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: prompter,
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags:  []string{"A=2"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if captureDeployPost(requests) != nil {
		t.Errorf("POST should not be sent when prompt is declined")
	}
	var cliErr clierr.CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != clierr.ConfirmationRequired {
		t.Errorf("err = %v, want ConfirmationRequired", err)
	}
	if !strings.Contains(cliErr.Message, "deploy cancelled") {
		t.Errorf("message = %q, want 'deploy cancelled'", cliErr.Message)
	}
}

func TestDeploy_EnvConflict_JSONModeWithoutYes_Errors(t *testing.T) {
	io, out, _ := newTestDeployIO(true, true)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":                {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":         {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":             {200, stubPipelineDevToProd()},
		"GET /orgs/acme/projects/triage/agents/order-bot/configurations": {200, stubConfigsWithKVs([2]string{"A", "1"})},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags:  []string{"A=2"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.ConfirmationRequired {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.ConfirmationRequired)
	}
	if !strings.Contains(errBody["message"].(string), "--json") {
		t.Errorf("message should mention --json: %v", errBody["message"])
	}
}

func TestDeploy_EnvConflict_NonTTYWithoutYes_Errors(t *testing.T) {
	io, _, _ := newTestDeployIO(false, false)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":                {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":         {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":             {200, stubPipelineDevToProd()},
		"GET /orgs/acme/projects/triage/agents/order-bot/configurations": {200, stubConfigsWithKVs([2]string{"A", "1"})},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags:  []string{"A=2"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	var cliErr clierr.CLIError
	if !errors.As(err, &cliErr) || cliErr.Code != clierr.ConfirmationRequired {
		t.Errorf("code = %v, want ConfirmationRequired", err)
	}
	if !strings.Contains(cliErr.Message, "stdin is not a terminal") {
		t.Errorf("message should mention stdin: %q", cliErr.Message)
	}
}

func sensitiveConfigsRoute() (string, stubResponse) {
	return "GET /orgs/acme/projects/triage/agents/order-bot/configurations",
		stubResponse{200, map[string]any{
			"agentName":   "order-bot",
			"projectName": "triage",
			"environment": "dev",
			"configurations": []any{
				map[string]any{"key": "PINECONE_API_KEY", "value": "", "isSensitive": true},
			},
		}}
}

func TestDeploy_EnvSensitiveConflict_WithYes_JSONReportsKeyOnly(t *testing.T) {
	io, out, _ := newTestDeployIO(true, true)
	requests := []recordedRequest{}
	sk, sv := sensitiveConfigsRoute()
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":              {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":       {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":           {200, stubPipelineDevToProd()},
		sk: sv,
		"POST /orgs/acme/projects/triage/agents/order-bot/deployments": {202, stubDeployAccepted("img1")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags:  []string{"PINECONE_API_KEY=newval"},
		Yes:       true,
	})
	if err != nil {
		t.Fatalf("runDeploy: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	resolved, ok := data["conflictsResolved"].([]any)
	if !ok || len(resolved) != 1 || resolved[0] != "PINECONE_API_KEY" {
		t.Errorf("conflictsResolved = %v, want [PINECONE_API_KEY]", resolved)
	}
	body := captureDeployPost(requests)
	if !strings.Contains(string(body), "newval") {
		t.Errorf("POST should still send the new value")
	}
	if strings.Contains(out.String(), "newval") {
		t.Errorf("envelope must NOT contain the new sensitive value: %s", out.String())
	}
}

func TestDeploy_EnvSensitiveConflict_JSONWithoutYes_ErrorsKeyOnly(t *testing.T) {
	io, out, _ := newTestDeployIO(true, true)
	requests := []recordedRequest{}
	sk, sv := sensitiveConfigsRoute()
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":        {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds": {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":     {200, stubPipelineDevToProd()},
		sk: sv,
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags:  []string{"PINECONE_API_KEY=newval"},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if captureDeployPost(requests) != nil {
		t.Errorf("no POST should be issued before confirmation")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.ConfirmationRequired {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.ConfirmationRequired)
	}
	if strings.Contains(out.String(), "newval") {
		t.Errorf("error envelope must NOT contain the new sensitive value: %s", out.String())
	}
}

func TestDeploy_EnvFlag_MalformedEmptyKey(t *testing.T) {
	io, out, _ := newTestDeployIO(true, true)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":                {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":         {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":             {200, stubPipelineDevToProd()},
		"GET /orgs/acme/projects/triage/agents/order-bot/configurations": {200, stubConfigurations(nil)},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags:  []string{"=foo"},
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
		"GET /orgs/acme/projects/triage/agents/order-bot":                {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":         {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":             {200, stubPipelineDevToProd()},
		"GET /orgs/acme/projects/triage/agents/order-bot/configurations": {200, stubConfigurations(nil)},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
		EnvFlags:  []string{"FOO"},
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
// advertised BuildCompleted/BuildInProgress/BuildTriggered enum. See
// agent-manager-service/clients/openchoreosvc/client/builds.go:677-693.
// Both "Completed" (workload CR updated) and "Succeeded" (image pushed,
// workload CR not yet updated) have an imageId populated and are deployable.

func TestDeploy_StatusCompleted_IsDeployable(t *testing.T) {
	io, _, _ := newTestDeployIO(true, false)
	requests := []recordedRequest{}
	routes := map[string]stubResponse{
		"GET /orgs/acme/projects/triage/agents/order-bot":                {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":         {200, stubBuildsListLatest("b1", "img1", "Completed")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":             {200, stubPipelineDevToProd()},
		"GET /orgs/acme/projects/triage/agents/order-bot/configurations": {200, stubConfigurations(nil)},
		"POST /orgs/acme/projects/triage/agents/order-bot/deployments":   {202, stubDeployAccepted("img1")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
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
		"GET /orgs/acme/projects/triage/agents/order-bot":                {200, stubBuildableAgent()},
		"GET /orgs/acme/projects/triage/agents/order-bot/builds":         {200, stubBuildsListLatest("b1", "img1", "Succeeded")},
		"GET /orgs/acme/projects/triage/deployment-pipeline":             {200, stubPipelineDevToProd()},
		"GET /orgs/acme/projects/triage/agents/order-bot/configurations": {200, stubConfigurations(nil)},
		"POST /orgs/acme/projects/triage/agents/order-bot/deployments":   {202, stubDeployAccepted("img1")},
	}
	srv := newStubServer(t, routes, &requests)
	defer srv.Close()
	client, _ := gen.NewClientWithResponses(srv.URL)

	err := runDeploy(context.Background(), &DeployOptions{
		IO: io, Prompter: &fakeDeployPrompter{},
		Client:    func(context.Context) (*gen.ClientWithResponses, error) { return client, nil },
		Scope:     baseScope(),
		Org:       "acme", Proj: "triage", AgentName: "order-bot",
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
