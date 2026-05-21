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
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
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

func statusRoutesDevStagingProd() map[string]statusRoute {
	return map[string]statusRoute{
		"GET /orgs/acme/projects/triage/agents/order-bot/deployments": {
			status: http.StatusOK,
			body: map[string]any{
				"dev":     map[string]any{"status": "active", "imageId": "img-1", "lastDeployed": "2026-05-21T09:14:00Z", "endpoints": []any{}},
				"staging": map[string]any{"status": "in-progress", "imageId": "img-1", "lastDeployed": "2026-05-21T11:02:11Z", "endpoints": []any{}},
				"prod":    map[string]any{"status": "not-deployed", "imageId": "img-1", "lastDeployed": "0001-01-01T00:00:00Z", "endpoints": []any{}},
			},
		},
		"GET /orgs/acme/projects/triage/deployment-pipeline": {
			status: http.StatusOK,
			body: map[string]any{
				"name": "default", "displayName": "Default", "description": "", "orgName": "acme", "createdAt": "2026-05-21T09:00:00Z",
				"promotionPaths": []any{
					map[string]any{"sourceEnvironmentRef": "dev", "targetEnvironmentRefs": []any{map[string]any{"name": "staging"}}},
					map[string]any{"sourceEnvironmentRef": "staging", "targetEnvironmentRefs": []any{map[string]any{"name": "prod"}}},
				},
			},
		},
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
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		"GET /orgs/acme/projects/triage/agents/order-bot/deployments": {
			status: http.StatusOK,
			body: map[string]any{
				"dev": map[string]any{
					"status":                 "active",
					"imageId":                "img-1",
					"lastDeployed":           "2026-05-21T09:14:00Z",
					"endpoints":              []any{},
					"environmentDisplayName": "Development",
				},
			},
		},
		"GET /orgs/acme/projects/triage/deployment-pipeline": {
			status: http.StatusOK,
			body: map[string]any{
				"name": "default", "displayName": "Default", "description": "", "orgName": "acme", "createdAt": "2026-05-21T09:00:00Z",
				"promotionPaths": []any{},
			},
		},
	})
	defer cleanup()

	err := runStatus(context.Background(), &StatusOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-bot",
	})
	if err != nil {
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
	envs := data["environments"].([]any)
	if len(envs) != 1 {
		t.Fatalf("len(environments) = %d, want 1", len(envs))
	}
	row := envs[0].(map[string]any)
	if _, exists := row["promotionTarget"]; exists {
		t.Fatalf("promotionTarget must be absent in v1: %v", row)
	}
	endpoints, ok := row["endpoints"].([]any)
	if !ok {
		t.Fatalf("endpoints must be array, got %T", row["endpoints"])
	}
	if len(endpoints) != 0 {
		t.Fatalf("endpoints = %v, want []", endpoints)
	}
}

func Test_runStatus_pipelineTransport_failure(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		"GET /orgs/acme/projects/triage/agents/order-bot/deployments": {
			status: http.StatusOK,
			body: map[string]any{
				"dev": map[string]any{
					"status": "active", "imageId": "img-1", "lastDeployed": "2026-05-21T09:14:00Z", "endpoints": []any{},
				},
			},
		},
		"GET /orgs/acme/projects/triage/deployment-pipeline": {transportErr: errors.New("dial tcp 127.0.0.1:443: connect: refused")},
	})
	defer cleanup()

	err := runStatus(context.Background(), &StatusOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-bot",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.Transport {
		t.Fatalf("code = %v, want %s", errBody["code"], clierr.Transport)
	}
}

func Test_runStatus_callsDeploymentAndPipelineInParallel(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = true

	var inflight, maxInflight atomic.Int32
	allArrived := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := inflight.Add(1)
		for {
			cur := maxInflight.Load()
			if n <= cur || maxInflight.CompareAndSwap(cur, n) {
				break
			}
		}
		if n >= 2 {
			close(allArrived)
		}
		select {
		case <-allArrived:
		case <-time.After(2 * time.Second):
			t.Errorf("handler for %s did not see second concurrent request within 2s", r.URL.Path)
		}
		defer inflight.Add(-1)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		switch r.URL.Path {
		case "/orgs/acme/projects/triage/agents/order-bot/deployments":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"dev": map[string]any{"status": "active", "imageId": "img-1", "lastDeployed": "2026-05-21T09:14:00Z", "endpoints": []any{}},
			})
		case "/orgs/acme/projects/triage/deployment-pipeline":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"name": "default", "displayName": "Default", "description": "", "orgName": "acme", "createdAt": "2026-05-21T09:00:00Z", "promotionPaths": []any{},
			})
		default:
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
	}))
	defer srv.Close()

	client, err := amsvc.NewClientWithResponses(srv.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	clientFn := func(context.Context) (*amsvc.ClientWithResponses, error) { return client, nil }

	if err := runStatus(context.Background(), &StatusOptions{
		IO: ios, Client: clientFn, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-bot",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := maxInflight.Load(); got < 2 {
		t.Fatalf("maxInflight = %d, want >= 2 (calls were not parallel)", got)
	}
	_ = out.String()
}

func Test_runStatus_agentNotFound(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		"GET /orgs/acme/projects/triage/agents/order-bot/deployments": {
			status: http.StatusNotFound,
			body:   map[string]any{"code": "AGENT_NOT_FOUND", "message": "Agent 'order-bot' not found", "reason": "not found"},
		},
		"GET /orgs/acme/projects/triage/deployment-pipeline": {
			status: http.StatusOK,
			body:   map[string]any{"name": "default", "displayName": "Default", "description": "", "orgName": "acme", "createdAt": "2026-05-21T09:00:00Z", "promotionPaths": []any{}},
		},
	})
	defer cleanup()

	err := runStatus(context.Background(), &StatusOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot"})
	if err == nil {
		t.Fatal("expected error")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "AGENT_NOT_FOUND" {
		t.Fatalf("code = %v, want AGENT_NOT_FOUND", errBody["code"])
	}
	if errBody["status"].(float64) != 404 {
		t.Fatalf("status = %v, want 404", errBody["status"])
	}
}

func Test_runStatus_pipelineNotFound(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		"GET /orgs/acme/projects/triage/agents/order-bot/deployments": {
			status: http.StatusOK,
			body: map[string]any{
				"dev": map[string]any{"status": "active", "imageId": "img-1", "lastDeployed": "2026-05-21T09:14:00Z", "endpoints": []any{}},
			},
		},
		"GET /orgs/acme/projects/triage/deployment-pipeline": {
			status: http.StatusNotFound,
			body:   map[string]any{"code": "PIPELINE_NOT_FOUND", "message": "Deployment pipeline not found for project 'triage'", "reason": "not found"},
		},
	})
	defer cleanup()

	err := runStatus(context.Background(), &StatusOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot"})
	if err == nil {
		t.Fatal("expected error")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "PIPELINE_NOT_FOUND" {
		t.Fatalf("code = %v, want PIPELINE_NOT_FOUND", errBody["code"])
	}
	if errBody["status"].(float64) != 404 {
		t.Fatalf("status = %v, want 404", errBody["status"])
	}
}

func Test_runStatus_ordering_followsPipeline(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		"GET /orgs/acme/projects/triage/agents/order-bot/deployments": {
			status: http.StatusOK,
			body: map[string]any{
				"prod":    map[string]any{"status": "not-deployed", "imageId": "img-1", "lastDeployed": "0001-01-01T00:00:00Z", "endpoints": []any{}},
				"dev":     map[string]any{"status": "active", "imageId": "img-1", "lastDeployed": "2026-05-21T09:14:00Z", "endpoints": []any{}},
				"staging": map[string]any{"status": "in-progress", "imageId": "img-1", "lastDeployed": "2026-05-21T10:14:00Z", "endpoints": []any{}},
			},
		},
		"GET /orgs/acme/projects/triage/deployment-pipeline": {
			status: http.StatusOK,
			body: map[string]any{
				"name": "default", "displayName": "Default", "description": "", "orgName": "acme", "createdAt": "2026-05-21T09:00:00Z",
				"promotionPaths": []any{
					map[string]any{"sourceEnvironmentRef": "dev", "targetEnvironmentRefs": []any{map[string]any{"name": "staging"}}},
					map[string]any{"sourceEnvironmentRef": "staging", "targetEnvironmentRefs": []any{map[string]any{"name": "prod"}}},
				},
			},
		},
	})
	defer cleanup()

	err := runStatus(context.Background(), &StatusOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	envs := env["data"].(map[string]any)["environments"].([]any)
	got := []string{envs[0].(map[string]any)["name"].(string), envs[1].(map[string]any)["name"].(string), envs[2].(map[string]any)["name"].(string)}
	want := []string{"dev", "staging", "prod"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func Test_runStatus_ordering_envInDeploymentsButNotPipeline(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		"GET /orgs/acme/projects/triage/agents/order-bot/deployments": {
			status: http.StatusOK,
			body: map[string]any{
				"dev":     map[string]any{"status": "active", "imageId": "img-1", "lastDeployed": "2026-05-21T09:14:00Z", "endpoints": []any{}},
				"sandbox": map[string]any{"status": "failed", "imageId": "img-1", "lastDeployed": "2026-05-21T09:20:00Z", "endpoints": []any{}},
				"qa":      map[string]any{"status": "suspended", "imageId": "img-1", "lastDeployed": "2026-05-21T09:18:00Z", "endpoints": []any{}},
			},
		},
		"GET /orgs/acme/projects/triage/deployment-pipeline": {
			status: http.StatusOK,
			body: map[string]any{
				"name": "default", "displayName": "Default", "description": "", "orgName": "acme", "createdAt": "2026-05-21T09:00:00Z",
				"promotionPaths": []any{map[string]any{"sourceEnvironmentRef": "dev", "targetEnvironmentRefs": []any{}}},
			},
		},
	})
	defer cleanup()

	err := runStatus(context.Background(), &StatusOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	envs := env["data"].(map[string]any)["environments"].([]any)
	got := []string{envs[0].(map[string]any)["name"].(string), envs[1].(map[string]any)["name"].(string), envs[2].(map[string]any)["name"].(string)}
	want := []string{"dev", "qa", "sandbox"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func Test_runStatus_envFilter_match(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newStatusClient(t, statusRoutesDevStagingProd())
	defer cleanup()

	err := runStatus(context.Background(), &StatusOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-bot", Env: "staging",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	envs := env["data"].(map[string]any)["environments"].([]any)
	if len(envs) != 1 {
		t.Fatalf("len(environments) = %d, want 1", len(envs))
	}
	if envs[0].(map[string]any)["name"] != "staging" {
		t.Fatalf("name = %v, want staging", envs[0].(map[string]any)["name"])
	}
}

func Test_runStatus_envFilter_miss(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newStatusClient(t, statusRoutesDevStagingProd())
	defer cleanup()

	err := runStatus(context.Background(), &StatusOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-bot", Env: "nope",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.NotFound {
		t.Fatalf("code = %v, want %s", errBody["code"], clierr.NotFound)
	}
	msg := errBody["message"].(string)
	for _, want := range []string{"nope", "dev", "staging", "prod"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("message %q missing %q", msg, want)
		}
	}
}

func Test_runStatus_humanTable_multiEnv(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.SetTerminal(true, true, true)
	io.JSON = false
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		"GET /orgs/acme/projects/triage/agents/order-bot/deployments": {
			status: http.StatusOK,
			body: map[string]any{
				"dev":     map[string]any{"status": "active", "imageId": "img-1", "lastDeployed": "2026-05-21T09:14:00Z", "endpoints": []any{map[string]any{"name": "public", "url": "https://dev.example.com", "visibility": "Public"}}},
				"staging": map[string]any{"status": "in-progress", "imageId": "img-1", "lastDeployed": "2026-05-21T11:02:11Z", "endpoints": []any{}},
				"prod":    map[string]any{"status": "not-deployed", "imageId": "img-1", "lastDeployed": "0001-01-01T00:00:00Z", "endpoints": []any{}},
				"archive": map[string]any{"status": "suspended", "imageId": "img-1", "lastDeployed": "2026-04-30T08:00:00Z", "endpoints": []any{}},
				"qa":      map[string]any{"status": "mystery", "imageId": "img-1", "lastDeployed": "2026-05-21T12:00:00Z", "endpoints": []any{}},
			},
		},
		"GET /orgs/acme/projects/triage/deployment-pipeline": {
			status: http.StatusOK,
			body: map[string]any{"name": "default", "displayName": "Default", "description": "", "orgName": "acme", "createdAt": "2026-05-21T09:00:00Z", "promotionPaths": []any{
				map[string]any{"sourceEnvironmentRef": "dev", "targetEnvironmentRefs": []any{map[string]any{"name": "staging"}}},
				map[string]any{"sourceEnvironmentRef": "staging", "targetEnvironmentRefs": []any{map[string]any{"name": "prod"}}},
				map[string]any{"sourceEnvironmentRef": "prod", "targetEnvironmentRefs": []any{map[string]any{"name": "archive"}}},
			}},
		},
	})
	defer cleanup()

	err := runStatus(context.Background(), &StatusOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot"})
	if err != nil {
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
	if !strings.Contains(got, "\x1b[32mactive\x1b[0m") {
		t.Fatalf("active status must be green: %q", got)
	}
	if !strings.Contains(got, "\x1b[33min-progress\x1b[0m") {
		t.Fatalf("in-progress status must be yellow: %q", got)
	}
	if !strings.Contains(got, "\x1b[90mnot-deployed\x1b[0m") {
		t.Fatalf("not-deployed status must be gray: %q", got)
	}
	if !strings.Contains(got, "\x1b[90msuspended\x1b[0m") {
		t.Fatalf("suspended status must be gray: %q", got)
	}
	if strings.Contains(got, "\x1b[31mmystery\x1b[0m") || strings.Contains(got, "\x1b[32mmystery\x1b[0m") || strings.Contains(got, "\x1b[33mmystery\x1b[0m") || strings.Contains(got, "\x1b[90mmystery\x1b[0m") {
		t.Fatalf("unknown status must be uncolored: %q", got)
	}

	prodLine := ""
	for _, line := range strings.Split(got, "\n") {
		if strings.Contains(line, "prod") {
			prodLine = line
			break
		}
	}
	if prodLine == "" {
		t.Fatalf("prod row not found in:\n%s", got)
	}
	if !strings.Contains(prodLine, "-") {
		t.Fatalf("prod row should render zero-time lastDeployed as '-': %q", prodLine)
	}
}

func Test_runStatus_humanTable_endpointsTruncation(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.SetTerminal(true, true, true)
	io.JSON = false
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		"GET /orgs/acme/projects/triage/agents/order-bot/deployments": {
			status: http.StatusOK,
			body: map[string]any{
				"dev": map[string]any{
					"status":       "active",
					"imageId":      "img-1",
					"lastDeployed": "2026-05-21T09:14:00Z",
					"endpoints": []any{
						map[string]any{"name": "public", "url": "https://dev.example.com", "visibility": "Public"},
						map[string]any{"name": "internal", "url": "https://dev.internal", "visibility": "Internal"},
						map[string]any{"name": "private", "url": "https://dev.private", "visibility": "Private"},
					},
				},
			},
		},
		"GET /orgs/acme/projects/triage/deployment-pipeline": {
			status: http.StatusOK,
			body:   map[string]any{"name": "default", "displayName": "Default", "description": "", "orgName": "acme", "createdAt": "2026-05-21T09:00:00Z", "promotionPaths": []any{map[string]any{"sourceEnvironmentRef": "dev", "targetEnvironmentRefs": []any{}}}},
		},
	})
	defer cleanup()

	err := runStatus(context.Background(), &StatusOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "https://dev.example.com (+2)") {
		t.Fatalf("expected first endpoint + suffix, got %q", got)
	}
	if strings.Contains(got, "https://dev.internal") || strings.Contains(got, "https://dev.private") {
		t.Fatalf("only first endpoint URL should be shown, got %q", got)
	}
}

func Test_runStatus_humanTable_singleEnvPipeline(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.SetTerminal(true, true, true)
	io.JSON = false
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		"GET /orgs/acme/projects/triage/agents/order-bot/deployments": {
			status: http.StatusOK,
			body: map[string]any{
				"dev": map[string]any{"status": "active", "imageId": "img-1", "lastDeployed": "2026-05-21T09:14:00Z", "endpoints": []any{}},
			},
		},
		"GET /orgs/acme/projects/triage/deployment-pipeline": {
			status: http.StatusOK,
			body: map[string]any{
				"name": "default", "displayName": "Default", "description": "", "orgName": "acme", "createdAt": "2026-05-21T09:00:00Z",
				"promotionPaths": []any{map[string]any{"sourceEnvironmentRef": "dev", "targetEnvironmentRefs": []any{}}},
			},
		},
	})
	defer cleanup()

	err := runStatus(context.Background(), &StatusOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "ENV") || !strings.Contains(got, "dev") {
		t.Fatalf("expected one-row table, got %q", got)
	}
}

func Test_runStatus_emptyDeployments_returnsSuccess(t *testing.T) {
	io, out, errOut := newTestIO(true)
	client, cleanup := newStatusClient(t, map[string]statusRoute{
		"GET /orgs/acme/projects/triage/agents/order-bot/deployments": {
			status: http.StatusOK,
			body:   map[string]any{},
		},
		"GET /orgs/acme/projects/triage/deployment-pipeline": {
			status: http.StatusOK,
			body:   map[string]any{"name": "default", "displayName": "Default", "description": "", "orgName": "acme", "createdAt": "2026-05-21T09:00:00Z", "promotionPaths": []any{}},
		},
	})
	defer cleanup()

	err := runStatus(context.Background(), &StatusOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(errOut.String(), "No environments configured for this project's deployment pipeline.") {
		t.Fatalf("missing empty-state message: %q", errOut.String())
	}
	env := decodeEnvelope(t, out.String())
	envs := env["data"].(map[string]any)["environments"].([]any)
	if len(envs) != 0 {
		t.Fatalf("len(environments) = %d, want 0", len(envs))
	}
}
