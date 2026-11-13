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

package mcp

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func newSetOpts(io *iostreams.IOStreams, client clientFn) *SetOptions {
	return &SetOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot", Name: "primary"}
}

// envMapKeys returns the env names present in a captured request body's envMappings.
func envMapKeys(body map[string]any) map[string]any {
	em, _ := body["envMappings"].(map[string]any)
	return em
}

func Test_runSet_createsWhenAbsent(t *testing.T) {
	io, _, errOut := newTestIO(false)
	io.SetTerminal(true, true, true)
	var posted map[string]any
	client, cleanup := newClient(t, map[string]route{
		listPath: okJSON(listResponse(mcpItem("other", getUUID))), // name not present
		postPath: {status: http.StatusCreated, body: getResponseFixture(), capture: &posted},
	})
	defer cleanup()

	opts := newSetOpts(io, client)
	opts.Proxy = "github"
	if err := runSet(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if posted["name"] != "primary" {
		t.Errorf("posted name = %v, want primary", posted["name"])
	}
	if posted["type"] != "mcp" {
		t.Errorf("posted type = %v, want mcp", posted["type"])
	}
	em := envMapKeys(posted)
	dev, ok := em["default"].(map[string]any)
	if !ok {
		t.Fatalf("envMappings.default missing: %v", em)
	}
	if dev["proxyName"] != "github" {
		t.Errorf("dev proxyName = %v, want github", dev["proxyName"])
	}
	if !strings.Contains(errOut.String(), "created") {
		t.Errorf("expected 'created' confirmation, got %q", errOut.String())
	}
}

// The critical read-merge-write guarantee: updating one env must NOT drop the
// others, since PUT replaces the entire envMappings set.
func Test_runSet_updatePreservesSiblingEnvs(t *testing.T) {
	io, _, _ := newTestIO(false)
	io.SetTerminal(true, true, true)
	var put map[string]any
	client, cleanup := newClient(t, map[string]route{
		listPath:                   okJSON(listResponse(mcpItem("primary", getUUID))),
		configPath("GET", getUUID): okJSON(getResponseFixture()), // has dev + prod
		configPath("PUT", getUUID): {status: http.StatusOK, body: getResponseFixture(), capture: &put},
	})
	defer cleanup()

	opts := newSetOpts(io, client)
	opts.Proxy = "bitbucket"
	if err := runSet(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	em := envMapKeys(put)
	for _, env := range []string{"dev", "prod", "default"} {
		if _, ok := em[env]; !ok {
			t.Errorf("PUT envMappings missing %q (read-merge-write dropped a sibling): %v", env, em)
		}
	}
	if staging := em["default"].(map[string]any); staging["proxyName"] != "bitbucket" {
		t.Errorf("staging proxyName = %v, want bitbucket", staging["proxyName"])
	}
	// Sibling proxy must be carried over from the GET response untouched.
	if dev := em["dev"].(map[string]any); dev["proxyName"] != "github" {
		t.Errorf("dev proxyName = %v, want github (preserved)", dev["proxyName"])
	}
}

func Test_runSet_createCarriesEnvVars(t *testing.T) {
	io, _, _ := newTestIO(false)
	io.SetTerminal(true, true, true)
	var posted map[string]any
	client, cleanup := newClient(t, map[string]route{
		listPath: okJSON(listResponse()),
		postPath: {status: http.StatusCreated, body: getResponseFixture(), capture: &posted},
	})
	defer cleanup()

	opts := newSetOpts(io, client)
	opts.Proxy = "github"
	opts.URLEnv, opts.APIKeyEnv = "MCP_URL", "MCP_KEY"
	if err := runSet(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	evs, ok := posted["environmentVariables"].([]any)
	if !ok || len(evs) != 2 {
		t.Fatalf("environmentVariables = %v, want 2 entries", posted["environmentVariables"])
	}
	byKey := map[string]string{}
	for _, e := range evs {
		m := e.(map[string]any)
		byKey[m["key"].(string)] = m["name"].(string)
	}
	if byKey["url"] != "MCP_URL" || byKey["apikey"] != "MCP_KEY" {
		t.Errorf("env vars = %v, want url=MCP_URL apikey=MCP_KEY", byKey)
	}
}

// Invalid env-var names must be rejected client-side, before any API call, so
// the user gets early feedback instead of a server rejection. Empty routes mean
// any HTTP request would 500 with NOT_STUBBED — reaching one is a test failure.
func Test_runSet_rejectsInvalidEnvVarName(t *testing.T) {
	for _, tc := range []struct {
		name    string
		set     func(*SetOptions)
		wantArg string
	}{
		{name: "url-env", set: func(o *SetOptions) { o.URLEnv = "1BAD" }, wantArg: "--url-env"},
		{name: "apikey-env", set: func(o *SetOptions) { o.APIKeyEnv = "BAD-KEY" }, wantArg: "--apikey-env"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			io, out, _ := newTestIO(true)
			client, cleanup := newClient(t, map[string]route{})
			defer cleanup()

			opts := newSetOpts(io, client)
			opts.Proxy = "github"
			tc.set(opts)
			if err := runSet(context.Background(), opts); err == nil {
				t.Fatalf("expected error for invalid %s", tc.wantArg)
			}
			if !strings.Contains(out.String(), tc.wantArg) {
				t.Errorf("error output %q does not mention %s", out.String(), tc.wantArg)
			}
		})
	}
}

func Test_runSet_createServerError(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newClient(t, map[string]route{
		listPath: okJSON(listResponse()),
		postPath: {status: http.StatusConflict, body: map[string]any{"code": "CONFLICT", "message": "exists"}},
	})
	defer cleanup()

	opts := newSetOpts(io, client)
	opts.Proxy = "github"
	if err := runSet(context.Background(), opts); err == nil {
		t.Fatal("expected error")
	}
	if status := decodeEnvelope(t, out.String())["error"].(map[string]any)["status"]; status.(float64) != 409 {
		t.Fatalf("status = %v, want 409", status)
	}
}
