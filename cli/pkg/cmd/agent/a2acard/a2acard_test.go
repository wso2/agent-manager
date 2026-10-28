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

package a2acard

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

const cardURL = "https://a.example/.well-known/agent-card.json"

var (
	target    = Target{Org: "acme", Proj: "triage", Agent: "order-bot", Env: "dev"}
	sourceKey = "PUT " + envPath("dev", "agent-card/source")
	proxyGet  = "GET " + envPath("dev", "a2a-proxy")
	proxyPut  = "PUT " + envPath("dev", "a2a-proxy")
)

func TestCheckProxyableURL(t *testing.T) {
	const m1 = "--url must be an http(s) URL"
	const m2 = "--url must not have a query or fragment to use --proxy"
	const m3 = "--url must end in /.well-known/agent-card.json to use --proxy"
	const m4 = "--url must have a canonical path (no \"//\", \"..\", trailing \"/\" or escaped characters before /.well-known/agent-card.json) to use --proxy"
	tests := []struct{ name, url, want string }{
		{"ok", cardURL, ""},
		{"double slash", "https://a.example//.well-known/agent-card.json", m4},
		{"dot dot", "https://a.example/a/../b/.well-known/agent-card.json", m4},
		{"escaped", "https://a.example/a%2Fb/.well-known/agent-card.json", m4},
		{"nested path", "https://a.example/agents/x/.well-known/agent-card.json", ""},
		{"wrong path", "https://a.example/card.json", m3},
		{"query", cardURL + "?x=1", m2},
		{"fragment", cardURL + "#f", m2},
		{"scheme", "ftp://a.example/.well-known/agent-card.json", m1},
		{"unparseable", "::bad", m1},
		{"no host", "https:///.well-known/agent-card.json", m1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := CheckProxyableURL("--url", tt.url)
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var fe *cmdutil.FlagError
			if !errors.As(err, &fe) {
				t.Fatalf("want FlagError, got %T %v", err, err)
			}
			var ce clierr.CLIError
			if !errors.As(err, &ce) || ce.Message != tt.want {
				t.Fatalf("message = %q, want %q", ce.Message, tt.want)
			}
		})
	}
}

func TestApplySource_direct(t *testing.T) {
	c := newClient(t, map[string]route{
		sourceKey: {status: http.StatusAccepted},
		proxyGet:  {status: http.StatusNotFound, body: map[string]any{"code": "NOT_FOUND", "message": "none"}},
	})
	got, err := ApplySource(context.Background(), c, target, cardURL, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := (SourceResult{Environment: "dev", URL: cardURL, Routing: RoutingDirect}); got != want {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}

func TestApplySource_proxyFromDirect(t *testing.T) {
	var body map[string]any
	c := newClient(t, map[string]route{
		sourceKey: {status: http.StatusAccepted},
		proxyGet:  {status: http.StatusNotFound, body: map[string]any{"code": "NOT_FOUND", "message": "none"}},
		proxyPut:  {status: http.StatusAccepted, capture: &body},
	})
	got, err := ApplySource(context.Background(), c, target, cardURL, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Routing != RoutingProxied {
		t.Fatalf("routing = %q", got.Routing)
	}
	if body == nil {
		t.Fatal("proxy PUT body not captured")
	}
	if _, ok := body["upstreamAuth"]; ok || len(body) != 0 {
		t.Fatalf("body should be empty, got %v", body)
	}
}

func TestApplySource_alreadyProxiedSendsNoPut(t *testing.T) {
	c := newClient(t, map[string]route{
		sourceKey: {status: http.StatusAccepted},
		proxyGet: {status: http.StatusOK, body: amsvc.A2AProxyResponse{
			Status: "published", UpstreamAuthHeader: "Authorization",
		}},
	})
	got, err := ApplySource(context.Background(), c, target, cardURL, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Routing != RoutingProxied {
		t.Fatalf("routing = %q", got.Routing)
	}
}

func TestApplySource_proxyFailsAfterSource(t *testing.T) {
	c := newClient(t, map[string]route{
		sourceKey: {status: http.StatusAccepted},
		proxyGet:  {status: http.StatusNotFound, body: map[string]any{"code": "NOT_FOUND", "message": "none"}},
		proxyPut:  {status: http.StatusBadRequest, body: map[string]any{"code": "BAD_REQUEST", "message": "boom"}},
	})
	got, err := ApplySource(context.Background(), c, target, cardURL, true)
	var ce clierr.CLIError
	if !errors.As(err, &ce) {
		t.Fatalf("want CLIError, got %T %v", err, err)
	}
	if !strings.HasPrefix(ce.Message, "card URL saved for dev, but ") {
		t.Fatalf("message = %q", ce.Message)
	}
	if ce.Code != "BAD_REQUEST" || ce.Status != http.StatusBadRequest {
		t.Fatalf("code/status = %q/%d", ce.Code, ce.Status)
	}
	if ce.AdditionalData["urlSaved"] != true || ce.AdditionalData["routing"] != RoutingDirect ||
		ce.AdditionalData["url"] != cardURL || ce.AdditionalData["environment"] != "dev" {
		t.Fatalf("additionalData = %v", ce.AdditionalData)
	}
	if got.Routing != RoutingDirect {
		t.Fatalf("routing = %q", got.Routing)
	}
}

func TestApplySource_proxyLookupFailsAfterSource(t *testing.T) {
	c := newClient(t, map[string]route{
		sourceKey: {status: http.StatusAccepted},
		proxyGet:  {status: http.StatusInternalServerError, body: map[string]any{"code": "INTERNAL", "message": "down"}},
	})
	got, err := ApplySource(context.Background(), c, target, cardURL, false)
	var ce clierr.CLIError
	if !errors.As(err, &ce) || ce.AdditionalData["urlSaved"] != true || ce.AdditionalData["routing"] != "" {
		t.Fatalf("err = %v", err)
	}
	if got.Routing != "" {
		t.Fatalf("routing = %q", got.Routing)
	}
}

func TestApplySource_privateHostHint(t *testing.T) {
	c := newClient(t, map[string]route{
		sourceKey: {status: http.StatusBadRequest, body: map[string]any{
			"code": "BAD_REQUEST", "message": "invalid URL: url host must be publicly reachable",
		}},
	})
	_, err := ApplySource(context.Background(), c, target, "http://localhost/card", false)
	var ce clierr.CLIError
	if !errors.As(err, &ce) || !strings.Contains(ce.Message, "expose the agent on a public URL") {
		t.Fatalf("err = %v", err)
	}
}

func TestApplySource_409Hint(t *testing.T) {
	c := newClient(t, map[string]route{
		sourceKey: {status: http.StatusConflict, body: map[string]any{"code": "CONFLICT", "message": "managed"}},
	})
	_, err := ApplySource(context.Background(), c, target, cardURL, false)
	var ce clierr.CLIError
	if !errors.As(err, &ce) || !strings.HasSuffix(ce.Message, "(only the console can clear it)") {
		t.Fatalf("err = %v", err)
	}
}
