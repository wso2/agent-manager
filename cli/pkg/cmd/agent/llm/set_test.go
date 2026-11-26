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
	"net/http"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func newSetOpts(io *iostreams.IOStreams, client clientFn) *SetOptions {
	return &SetOptions{IO: io, Client: client, Scope: baseScope(), Org: "acme", Proj: "triage", AgentName: "order-bot", Name: "primary"}
}

// specOf returns the spec of a captured request document.
func specOf(body map[string]any) map[string]any {
	spec, _ := body["spec"].(map[string]any)
	return spec
}

func Test_runSet_createsWhenAbsent(t *testing.T) {
	io, _, errOut := newTestIO(false)
	io.SetTerminal(true, true, true)
	var posted map[string]any
	client, cleanup := newClient(t, map[string]route{
		listPath: okJSON(listResponse(llmItem("other", getUUID))), // name not present
		postPath: {status: http.StatusCreated, body: getResponseFixture(), capture: &posted},
	})
	defer cleanup()

	opts := newSetOpts(io, client)
	opts.Provider = "openai"
	if err := runSet(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if meta, _ := posted["metadata"].(map[string]any); meta["name"] != "primary" || meta["project"] != "triage" {
		t.Errorf("posted metadata = %v, want primary in triage", posted["metadata"])
	}
	if posted["kind"] != "AgentModelConfig" {
		t.Errorf("posted kind = %v", posted["kind"])
	}
	if spec := specOf(posted); spec["providerName"] != "openai" {
		t.Errorf("spec.providerName = %v, want openai", spec["providerName"])
	}
	if !strings.Contains(errOut.String(), "created") {
		t.Errorf("expected 'created' confirmation, got %q", errOut.String())
	}
}

// An update PUTs the stored document back: the new binding replaces the old one, and what the command does not set is kept.
func Test_runSet_updateKeepsStoredFields(t *testing.T) {
	io, _, _ := newTestIO(false)
	io.SetTerminal(true, true, true)
	var put map[string]any
	client, cleanup := newClient(t, map[string]route{
		listPath:                   okJSON(listResponse(llmItem("primary", getUUID))),
		configPath("GET", getUUID): okJSON(getResponseFixture()),
		configPath("PUT", getUUID): {status: http.StatusOK, body: getResponseFixture(), capture: &put},
	})
	defer cleanup()

	opts := newSetOpts(io, client)
	opts.Provider = "cohere"
	if err := runSet(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	spec := specOf(put)
	if spec["providerName"] != "cohere" {
		t.Errorf("spec.providerName = %v, want cohere", spec["providerName"])
	}
	if spec["description"] != "primary openai binding" {
		t.Errorf("description = %v, want the stored one", spec["description"])
	}
	if cfg, _ := spec["configuration"].(map[string]any); cfg == nil || len(cfg["policies"].([]any)) != 1 {
		t.Errorf("configuration = %v, want the stored policies", spec["configuration"])
	}
	if _, ok := put["status"]; ok {
		t.Errorf("status was sent back: %v", put["status"])
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
	opts.Provider = "openai"
	opts.URLEnv, opts.APIKeyEnv = "LLM_URL", "LLM_KEY"
	if err := runSet(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	evs, ok := specOf(posted)["environmentVariables"].([]any)
	if !ok || len(evs) != 2 {
		t.Fatalf("environmentVariables = %v, want 2 entries", specOf(posted)["environmentVariables"])
	}
	byKey := map[string]string{}
	for _, e := range evs {
		m := e.(map[string]any)
		byKey[m["key"].(string)] = m["name"].(string)
	}
	if byKey["url"] != "LLM_URL" || byKey["apikey"] != "LLM_KEY" {
		t.Errorf("env vars = %v, want url=LLM_URL apikey=LLM_KEY", byKey)
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
			opts.Provider = "openai"
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
	opts.Provider = "openai"
	if err := runSet(context.Background(), opts); err == nil {
		t.Fatal("expected error")
	}
	if status := decodeEnvelope(t, out.String())["error"].(map[string]any)["status"]; status.(float64) != 409 {
		t.Fatalf("status = %v, want 409", status)
	}
}
