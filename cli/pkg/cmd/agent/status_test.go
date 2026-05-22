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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

const (
	depsPath = "GET /orgs/acme/projects/triage/agents/order-bot/deployments"
	pipePath = "GET /orgs/acme/projects/triage/deployment-pipeline"
)

type statusRoute struct {
	status       int
	body         any
	delay        time.Duration
	transportErr error
}

func newStatusClient(t *testing.T, routes map[string]statusRoute) (func(context.Context) (*amsvc.ClientWithResponses, error), func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		route, ok := routes[key]
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "NOT_STUBBED", "message": key})
			return
		}
		if route.transportErr != nil {
			hj, ok := w.(http.Hijacker)
			if !ok {
				t.Fatalf("response writer does not support hijacking")
			}
			conn, _, err := hj.Hijack()
			if err != nil {
				t.Fatalf("hijack: %v", err)
			}
			_ = conn.Close()
			return
		}
		if route.delay > 0 {
			time.Sleep(route.delay)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(route.status)
		if route.body != nil {
			_ = json.NewEncoder(w).Encode(route.body)
		}
	}))
	client, err := amsvc.NewClientWithResponses(srv.URL)
	if err != nil {
		srv.Close()
		t.Fatalf("new client: %v", err)
	}
	return func(context.Context) (*amsvc.ClientWithResponses, error) { return client, nil }, srv.Close
}

func okJSON(body any) statusRoute { return statusRoute{status: http.StatusOK, body: body} }

func deployment(status, lastDeployed string, endpoints ...any) map[string]any {
	if endpoints == nil {
		endpoints = []any{}
	}
	return map[string]any{
		"status": status, "imageId": "img-1", "lastDeployed": lastDeployed, "endpoints": endpoints,
	}
}

func pipeline(paths ...any) map[string]any {
	if paths == nil {
		paths = []any{}
	}
	return map[string]any{
		"name": "default", "displayName": "Default", "description": "",
		"orgName": "acme", "createdAt": "2026-05-21T09:00:00Z", "promotionPaths": paths,
	}
}

func promo(src string, targets ...string) map[string]any {
	refs := make([]any, len(targets))
	for i, t := range targets {
		refs[i] = map[string]any{"name": t}
	}
	return map[string]any{"sourceEnvironmentRef": src, "targetEnvironmentRefs": refs}
}

func endpoint(name, url, vis string) map[string]any {
	return map[string]any{"name": name, "url": url, "visibility": vis}
}

func newStatusOpts(io *iostreams.IOStreams, client func(context.Context) (*amsvc.ClientWithResponses, error)) *StatusOptions {
	return &StatusOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-bot",
	}
}

func decodeEnvs(t *testing.T, out string) []map[string]any {
	t.Helper()
	raw := decodeEnvelope(t, out)["data"].(map[string]any)["environments"].([]any)
	envs := make([]map[string]any, len(raw))
	for i, e := range raw {
		envs[i] = e.(map[string]any)
	}
	return envs
}

func envNames(envs []map[string]any) []string {
	names := make([]string, len(envs))
	for i, e := range envs {
		names[i] = e["name"].(string)
	}
	return names
}

func devStagingProdRoutes() map[string]statusRoute {
	return map[string]statusRoute{
		depsPath: okJSON(map[string]any{
			"dev":     deployment("active", "2026-05-21T09:14:00Z"),
			"staging": deployment("in-progress", "2026-05-21T11:02:11Z"),
			"prod":    deployment("not-deployed", "0001-01-01T00:00:00Z"),
		}),
		pipePath: okJSON(pipeline(promo("dev", "staging"), promo("staging", "prod"))),
	}
}

func TestNewAgentCmd_RegistersStatusAfterDeploy(t *testing.T) {
	wasSorting := cobra.EnableCommandSorting
	cobra.EnableCommandSorting = false
	defer func() { cobra.EnableCommandSorting = wasSorting }()

	ios, _, _, _ := iostreams.Test()
	cmd := NewAgentCmd(&cmdutil.Factory{IOStreams: ios})

	idxDeploy, idxStatus := -1, -1
	for i, c := range cmd.Commands() {
		switch c.Name() {
		case "deploy":
			idxDeploy = i
		case "status":
			idxStatus = i
		}
	}
	if idxDeploy == -1 {
		t.Fatal("deploy command not registered")
	}
	if idxStatus != idxDeploy+1 {
		t.Fatalf("status index = %d, want %d", idxStatus, idxDeploy+1)
	}
}

func Test_runStatus_json_envelope_and_shape(t *testing.T) {
	io, out, _ := newTestIO(true)
	dev := deployment("active", "2026-05-21T09:14:00Z")
	dev["environmentDisplayName"] = "Development"
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		depsPath: okJSON(map[string]any{"dev": dev}),
		pipePath: okJSON(pipeline()),
	})
	defer cleanup()

	if err := runStatus(context.Background(), newStatusOpts(io, client)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	if env["instance"] != "default" || env["org"] != "acme" || env["project"] != "triage" {
		t.Fatalf("unexpected envelope scope: %v", env)
	}
	data := env["data"].(map[string]any)
	if data["agent"] != "order-bot" {
		t.Fatalf("agent = %v, want order-bot", data["agent"])
	}
	envs := decodeEnvs(t, out.String())
	if len(envs) != 1 {
		t.Fatalf("len(environments) = %d, want 1", len(envs))
	}
	if _, exists := envs[0]["promotionTarget"]; exists {
		t.Fatalf("promotionTarget must be absent in v1: %v", envs[0])
	}
	endpoints, ok := envs[0]["endpoints"].([]any)
	if !ok {
		t.Fatalf("endpoints must be array, got %T", envs[0]["endpoints"])
	}
	if len(endpoints) != 0 {
		t.Fatalf("endpoints = %v, want []", endpoints)
	}
}

func Test_runStatus_serverErrors(t *testing.T) {
	cases := []struct {
		name       string
		deps       statusRoute
		pipe       statusRoute
		wantCode   string
		wantStatus float64
	}{
		{
			name:       "agentNotFound",
			deps:       statusRoute{status: http.StatusNotFound, body: map[string]any{"code": "AGENT_NOT_FOUND", "message": "Agent 'order-bot' not found", "reason": "not found"}},
			pipe:       okJSON(pipeline()),
			wantCode:   "AGENT_NOT_FOUND",
			wantStatus: 404,
		},
		{
			name:       "pipelineNotFound",
			deps:       okJSON(map[string]any{"dev": deployment("active", "2026-05-21T09:14:00Z")}),
			pipe:       statusRoute{status: http.StatusNotFound, body: map[string]any{"code": "PIPELINE_NOT_FOUND", "message": "Deployment pipeline not found for project 'triage'", "reason": "not found"}},
			wantCode:   "PIPELINE_NOT_FOUND",
			wantStatus: 404,
		},
		{
			name:     "pipelineTransportFailure",
			deps:     okJSON(map[string]any{"dev": deployment("active", "2026-05-21T09:14:00Z")}),
			pipe:     statusRoute{transportErr: errors.New("dial tcp 127.0.0.1:443: connect: refused")},
			wantCode: clierr.Transport,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			io, out, _ := newTestIO(true)
			client, cleanup := newStatusClient(t, map[string]statusRoute{depsPath: tc.deps, pipePath: tc.pipe})
			defer cleanup()
			if err := runStatus(context.Background(), newStatusOpts(io, client)); err == nil {
				t.Fatal("expected error")
			}
			errBody := decodeEnvelope(t, out.String())["error"].(map[string]any)
			if errBody["code"] != tc.wantCode {
				t.Fatalf("code = %v, want %s", errBody["code"], tc.wantCode)
			}
			if tc.wantStatus != 0 && errBody["status"].(float64) != tc.wantStatus {
				t.Fatalf("status = %v, want %v", errBody["status"], tc.wantStatus)
			}
		})
	}
}

func Test_runStatus_ordering(t *testing.T) {
	cases := []struct {
		name      string
		deps      map[string]any
		paths     []any
		wantOrder []string
	}{
		{
			name: "followsPipeline",
			deps: map[string]any{
				"prod":    deployment("not-deployed", "0001-01-01T00:00:00Z"),
				"dev":     deployment("active", "2026-05-21T09:14:00Z"),
				"staging": deployment("in-progress", "2026-05-21T10:14:00Z"),
			},
			paths:     []any{promo("dev", "staging"), promo("staging", "prod")},
			wantOrder: []string{"dev", "staging", "prod"},
		},
		{
			name: "envInDeploymentsButNotPipeline",
			deps: map[string]any{
				"dev":     deployment("active", "2026-05-21T09:14:00Z"),
				"sandbox": deployment("failed", "2026-05-21T09:20:00Z"),
				"qa":      deployment("suspended", "2026-05-21T09:18:00Z"),
			},
			paths:     []any{promo("dev")},
			wantOrder: []string{"dev", "qa", "sandbox"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			io, out, _ := newTestIO(true)
			client, cleanup := newStatusClient(t, map[string]statusRoute{
				depsPath: okJSON(tc.deps),
				pipePath: okJSON(pipeline(tc.paths...)),
			})
			defer cleanup()
			if err := runStatus(context.Background(), newStatusOpts(io, client)); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got := envNames(decodeEnvs(t, out.String())); !reflect.DeepEqual(got, tc.wantOrder) {
				t.Fatalf("order = %v, want %v", got, tc.wantOrder)
			}
		})
	}
}

func Test_runStatus_envFilter(t *testing.T) {
	t.Run("match", func(t *testing.T) {
		io, out, _ := newTestIO(true)
		client, cleanup := newStatusClient(t, devStagingProdRoutes())
		defer cleanup()
		opts := newStatusOpts(io, client)
		opts.Env = "staging"
		if err := runStatus(context.Background(), opts); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		envs := decodeEnvs(t, out.String())
		if len(envs) != 1 || envs[0]["name"] != "staging" {
			t.Fatalf("envs = %v, want [staging]", envs)
		}
	})
	t.Run("miss", func(t *testing.T) {
		io, out, _ := newTestIO(true)
		client, cleanup := newStatusClient(t, devStagingProdRoutes())
		defer cleanup()
		opts := newStatusOpts(io, client)
		opts.Env = "nope"
		if err := runStatus(context.Background(), opts); err == nil {
			t.Fatal("expected error")
		}
		errBody := decodeEnvelope(t, out.String())["error"].(map[string]any)
		if errBody["code"] != clierr.NotFound {
			t.Fatalf("code = %v, want %s", errBody["code"], clierr.NotFound)
		}
		msg := errBody["message"].(string)
		for _, want := range []string{"nope", "dev", "staging", "prod"} {
			if !strings.Contains(msg, want) {
				t.Fatalf("message %q missing %q", msg, want)
			}
		}
	})
}

func Test_runStatus_callsDeploymentAndPipelineInParallel(t *testing.T) {
	ios, _, _, _ := iostreams.Test()
	ios.JSON = true

	// Both handlers block until both have arrived. If runStatus called the
	// endpoints serially, the second never arrives and the timeout fires.
	var wg sync.WaitGroup
	wg.Add(2)
	barrier := make(chan struct{})
	go func() { wg.Wait(); close(barrier) }()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wg.Done()
		select {
		case <-barrier:
		case <-time.After(2 * time.Second):
			t.Errorf("handler for %s did not see concurrent request within 2s", r.URL.Path)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/orgs/acme/projects/triage/agents/order-bot/deployments":
			_ = json.NewEncoder(w).Encode(map[string]any{"dev": deployment("active", "2026-05-21T09:14:00Z")})
		case "/orgs/acme/projects/triage/deployment-pipeline":
			_ = json.NewEncoder(w).Encode(pipeline())
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	client, err := amsvc.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	clientFn := func(context.Context) (*amsvc.ClientWithResponses, error) { return client, nil }

	if err := runStatus(context.Background(), newStatusOpts(ios, clientFn)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func Test_runStatus_humanTable_multiEnv(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.SetTerminal(true, true, true)
	io.JSON = false
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		depsPath: okJSON(map[string]any{
			"dev":     deployment("active", "2026-05-21T09:14:00Z", endpoint("public", "https://dev.example.com", "Public")),
			"staging": deployment("in-progress", "2026-05-21T11:02:11Z"),
			"prod":    deployment("not-deployed", "0001-01-01T00:00:00Z"),
			"archive": deployment("suspended", "2026-04-30T08:00:00Z"),
			"qa":      deployment("mystery", "2026-05-21T12:00:00Z"),
		}),
		pipePath: okJSON(pipeline(promo("dev", "staging"), promo("staging", "prod"), promo("prod", "archive"))),
	})
	defer cleanup()

	if err := runStatus(context.Background(), newStatusOpts(io, client)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"ENV", "STATUS", "LAST DEPLOYED", "ENDPOINTS", "https://dev.example.com"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q in %q", want, got)
		}
	}
	for _, name := range []string{"dev", "staging", "prod", "archive", "qa"} {
		if !strings.Contains(got, name) {
			t.Fatalf("output missing env %q in %q", name, got)
		}
	}
	// Known statuses are wrapped in some SGR escape sequence; unknown is not.
	// The byte before the status is the SGR terminator 'm' only when colorized.
	for _, s := range []string{"active", "in-progress", "not-deployed", "suspended"} {
		if !strings.Contains(got, "m"+s+"\x1b[0m") {
			t.Errorf("status %q should be colorized in %q", s, got)
		}
	}
	if strings.Contains(got, "mmystery\x1b[0m") {
		t.Errorf("unknown status must be uncolored: %q", got)
	}

	prodLine := ""
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "prod") {
			prodLine = line
			break
		}
	}
	if prodLine == "" || !strings.Contains(prodLine, "-") {
		t.Fatalf("prod row should render zero-time lastDeployed as '-': %q", prodLine)
	}
}

func Test_runStatus_emptyDeployments_returnsSuccess(t *testing.T) {
	io, out, errOut := newTestIO(true)
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		depsPath: okJSON(map[string]any{}),
		pipePath: okJSON(pipeline()),
	})
	defer cleanup()

	if err := runStatus(context.Background(), newStatusOpts(io, client)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(errOut.String(), "No deployments found for this agent.") {
		t.Fatalf("missing empty-state message: %q", errOut.String())
	}
	if envs := decodeEnvs(t, out.String()); len(envs) != 0 {
		t.Fatalf("len(environments) = %d, want 0", len(envs))
	}
}

func TestFormatLastDeployed(t *testing.T) {
	if got := formatLastDeployed(time.Time{}); got != "-" {
		t.Errorf("zero time = %q, want %q", got, "-")
	}
	ts := time.Date(2026, 5, 21, 9, 14, 0, 0, time.UTC)
	if got := formatLastDeployed(ts); got != "2026-05-21T09:14:00Z" {
		t.Errorf("got %q, want RFC3339", got)
	}
}

func TestSummarizeEndpoints(t *testing.T) {
	cases := []struct {
		name string
		in   []EndpointRef
		want string
	}{
		{"none", nil, "-"},
		{"one", []EndpointRef{{URL: "https://a"}}, "https://a"},
		{"many", []EndpointRef{{URL: "https://a"}, {URL: "https://b"}, {URL: "https://c"}}, "https://a (+2)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := summarizeEndpoints(tc.in); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusColorFunc(t *testing.T) {
	cs := &iostreams.ColorScheme{Enabled: true}
	for _, s := range []string{"active", "in-progress", "failed", "not-deployed", "suspended"} {
		if statusColorFunc(cs, s) == nil {
			t.Errorf("statusColorFunc(%q) = nil, want non-nil", s)
		}
	}
	if statusColorFunc(cs, "mystery") != nil {
		t.Error("statusColorFunc(\"mystery\") != nil, want nil")
	}
}
