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
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

const depsPath = "GET /orgs/acme/projects/triage/agents/order-bot/deployments"

type statusRoute struct {
	status       int
	body         any
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

func endpoint(name, url, vis string) map[string]any {
	return map[string]any{"name": name, "url": url, "visibility": vis}
}

func newStatusOpts(io *iostreams.IOStreams, client func(context.Context) (*amsvc.ClientWithResponses, error)) *StatusOptions {
	return &StatusOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-bot",
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

func decodeStatus(t *testing.T, out string) map[string]any {
	t.Helper()
	return decodeEnvelope(t, out)["data"].(map[string]any)
}

func Test_runStatus_json_envelope_and_shape(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		depsPath: okJSON(map[string]any{"default": deployment("active", "2026-05-21T09:14:00Z")}),
	})
	defer cleanup()

	if err := runStatus(context.Background(), newStatusOpts(io, client)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	if env["instance"] != "default" || env["org"] != "acme" || env["project"] != "triage" {
		t.Fatalf("unexpected envelope scope: %v", env)
	}
	data := decodeStatus(t, out.String())
	if data["agent"] != "order-bot" || data["status"] != "active" {
		t.Fatalf("data = %v, want agent order-bot, status active", data)
	}
	if _, exists := data["environments"]; exists {
		t.Fatalf("environments must be absent: %v", data)
	}
	endpoints, ok := data["endpoints"].([]any)
	if !ok || len(endpoints) != 0 {
		t.Fatalf("endpoints = %v, want []", data["endpoints"])
	}
}

func Test_runStatus_serverErrors(t *testing.T) {
	cases := []struct {
		name       string
		deps       statusRoute
		wantCode   string
		wantStatus float64
	}{
		{
			name:       "agentNotFound",
			deps:       statusRoute{status: http.StatusNotFound, body: map[string]any{"code": "AGENT_NOT_FOUND", "message": "Agent 'order-bot' not found", "reason": "not found"}},
			wantCode:   "AGENT_NOT_FOUND",
			wantStatus: 404,
		},
		{
			name:     "transportFailure",
			deps:     statusRoute{transportErr: errors.New("dial tcp 127.0.0.1:443: connect: refused")},
			wantCode: clierr.Transport,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			io, out, _ := newTestIO(true)
			client, cleanup := newStatusClient(t, map[string]statusRoute{depsPath: tc.deps})
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

func Test_runStatus_humanTable(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.SetTerminal(true, true, true)
	io.JSON = false
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		depsPath: okJSON(map[string]any{
			"default": deployment("active", "2026-05-21T09:14:00Z", endpoint("public", "https://agent.example.com", "Public")),
		}),
	})
	defer cleanup()

	if err := runStatus(context.Background(), newStatusOpts(io, client)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"STATUS", "LAST DEPLOYED", "ENDPOINTS", "https://agent.example.com", "m" + "active" + "\x1b[0m"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q in %q", want, got)
		}
	}
	if strings.Contains(got, "ENV") {
		t.Errorf("output must not show an environment column: %q", got)
	}
}

func Test_runStatus_noDeployment_returnsSuccess(t *testing.T) {
	io, out, errOut := newTestIO(true)
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		depsPath: okJSON(map[string]any{}),
	})
	defer cleanup()

	if err := runStatus(context.Background(), newStatusOpts(io, client)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(errOut.String(), "No deployment found for this agent.") {
		t.Fatalf("missing empty-state message: %q", errOut.String())
	}
	if data := decodeStatus(t, out.String()); data["status"] != "not-deployed" {
		t.Fatalf("status = %v, want not-deployed", data["status"])
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
