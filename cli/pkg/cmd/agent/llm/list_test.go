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

package llm

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

func mixedListResponse() aigateway.AgentModelConfigListResponse {
	desc := "primary openai binding"
	return aigateway.AgentModelConfigListResponse{Configs: []aigateway.AgentModelConfigListItem{
		{Name: "primary", Type: "llm", Description: &desc, Uuid: mustUUID("11111111-1111-1111-1111-111111111111")},
		{Name: "secondary", Type: "llm", Uuid: mustUUID("22222222-2222-2222-2222-222222222222")},
		{Name: "tool-proxy", Type: "mcp", Uuid: mustUUID("33333333-3333-3333-3333-333333333333")},
	}}
}

func Test_runList_humanTable_llmOnly(t *testing.T) {
	io, out, _ := newTestIO(false)
	io.SetTerminal(true, true, true)
	client, cleanup := newClient(t, map[string]route{listPath: okJSON(mixedListResponse())})
	defer cleanup()

	opts := &ListOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot"}
	if err := runList(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	for _, want := range []string{"NAME", "DESCRIPTION", "primary", "secondary", "primary openai binding"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "tool-proxy") {
		t.Errorf("mcp config must be filtered out of llm list, got:\n%s", got)
	}
}

func Test_runList_json_onlyLLM(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newClient(t, map[string]route{listPath: okJSON(mixedListResponse())})
	defer cleanup()

	opts := &ListOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot"}
	if err := runList(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, out.String())["data"].(map[string]any)
	configs, ok := data["configs"].([]any)
	if !ok {
		t.Fatalf("data.configs not an array: %v", data["configs"])
	}
	if len(configs) != 2 {
		t.Fatalf("len(configs) = %d, want 2 (llm only)", len(configs))
	}
	for _, c := range configs {
		if c.(map[string]any)["type"] != "llm" {
			t.Errorf("non-llm config leaked into list: %v", c)
		}
	}
}

func Test_runList_serverError(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newClient(t, map[string]route{
		listPath: {status: http.StatusInternalServerError, body: map[string]any{"code": "INTERNAL", "message": "boom"}},
	})
	defer cleanup()

	opts := &ListOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot"}
	if err := runList(context.Background(), opts); err == nil {
		t.Fatal("expected error")
	}
	if code := decodeEnvelope(t, out.String())["error"].(map[string]any)["status"]; code.(float64) != 500 {
		t.Fatalf("status = %v, want 500", code)
	}
}

func Test_runList_transportError(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newClient(t, map[string]route{listPath: {transportErr: errors.New("refused")}})
	defer cleanup()

	opts := &ListOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot"}
	if err := runList(context.Background(), opts); err == nil {
		t.Fatal("expected error")
	}
	if code := decodeEnvelope(t, out.String())["error"].(map[string]any)["code"]; code != clierr.Transport {
		t.Fatalf("code = %v, want %s", code, clierr.Transport)
	}
}
