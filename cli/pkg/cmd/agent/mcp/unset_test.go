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
	"errors"
	"net/http"
	"strings"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmd/agent/modelconfig"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func newUnsetOpts(io *iostreams.IOStreams, client clientFn) *UnsetOptions {
	return &UnsetOptions{
		IO: io, Prompter: &fakePrompter{}, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-bot", Name: "primary",
	}
}

func Test_runUnset_deletesWholeConfig(t *testing.T) {
	io, _, errOut := newTestIO(false)
	io.SetTerminal(true, true, true)
	prompter := &fakePrompter{}
	client, cleanup := newClient(t, map[string]route{
		listPath:                      okJSON(listResponse(mcpItem("primary", getUUID))),
		configPath("DELETE", getUUID): {status: http.StatusNoContent},
	})
	defer cleanup()

	opts := newUnsetOpts(io, client)
	opts.Prompter = prompter
	if err := runUnset(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompter.calls != 1 {
		t.Errorf("prompter calls = %d, want 1", prompter.calls)
	}
	if prompter.confirmDeletionArg != "primary" {
		t.Errorf("confirmation arg = %q, want primary", prompter.confirmDeletionArg)
	}
	if !strings.Contains(errOut.String(), "deleted") {
		t.Errorf("expected 'deleted' confirmation, got %q", errOut.String())
	}
}

func Test_runUnset_deleteJSON(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newClient(t, map[string]route{
		listPath:                      okJSON(listResponse(mcpItem("primary", getUUID))),
		configPath("DELETE", getUUID): {status: http.StatusNoContent},
	})
	defer cleanup()

	opts := newUnsetOpts(io, client)
	opts.Yes = true
	if err := runUnset(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data := decodeEnvelope(t, out.String())["data"].(map[string]any)
	if data["deleted"] != true {
		t.Errorf("data.deleted = %v, want true", data["deleted"])
	}
}

func Test_runUnset_deleteYesSkipsPrompt(t *testing.T) {
	io, _, _ := newTestIO(false)
	prompter := &fakePrompter{}
	client, cleanup := newClient(t, map[string]route{
		listPath:                      okJSON(listResponse(mcpItem("primary", getUUID))),
		configPath("DELETE", getUUID): {status: http.StatusNoContent},
	})
	defer cleanup()

	opts := newUnsetOpts(io, client)
	opts.Prompter = prompter
	opts.Yes = true
	if err := runUnset(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompter.calls != 0 {
		t.Errorf("prompter calls = %d, want 0 with --yes", prompter.calls)
	}
}

func Test_runUnset_deleteNonTTYWithoutYes(t *testing.T) {
	io, out, _ := newTestIO(true)
	called := false
	client := func(context.Context) (*amsvc.ClientWithResponses, error) {
		called = true
		return nil, errors.New("client should not be constructed")
	}

	if err := runUnset(context.Background(), newUnsetOpts(io, client)); err == nil {
		t.Fatal("expected error for non-TTY without --yes")
	}
	if called {
		t.Fatal("client should not be constructed before confirmation")
	}
	errBody := decodeEnvelope(t, out.String())["error"].(map[string]any)
	if errBody["code"] != clierr.ConfirmationRequired {
		t.Fatalf("code = %v, want %s", errBody["code"], clierr.ConfirmationRequired)
	}
}

func Test_runUnset_deleteConfirmationMismatch(t *testing.T) {
	io, out, _ := newTestIO(true)
	io.SetTerminal(true, true, true)
	prompter := &fakePrompter{confirmDeletionErr: errors.New("confirmation mismatch")}
	opts := newUnsetOpts(io, unreachableClient)
	opts.Prompter = prompter

	if err := runUnset(context.Background(), opts); err == nil {
		t.Fatal("expected error from confirmation mismatch")
	}
	errBody := decodeEnvelope(t, out.String())["error"].(map[string]any)
	if errBody["code"] != clierr.ConfirmationRequired {
		t.Fatalf("code = %v, want %s", errBody["code"], clierr.ConfirmationRequired)
	}
}

func Test_runUnset_envRemovesOnePreservesOthers(t *testing.T) {
	io, _, _ := newTestIO(false)
	io.SetTerminal(true, true, true)
	var put map[string]any
	client, cleanup := newClient(t, map[string]route{
		listPath:                   okJSON(listResponse(mcpItem("primary", getUUID))),
		configPath("GET", getUUID): okJSON(getResponseFixture()), // dev + prod
		configPath("PUT", getUUID): {status: http.StatusOK, body: getResponseFixture(), capture: &put},
	})
	defer cleanup()

	opts := newUnsetOpts(io, client)
	opts.Env = "prod"
	if err := runUnset(context.Background(), opts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	em := envMapKeys(put)
	if _, ok := em["prod"]; ok {
		t.Errorf("prod should have been removed from envMappings: %v", em)
	}
	if _, ok := em["dev"]; !ok {
		t.Errorf("dev should be preserved in envMappings: %v", em)
	}
}

func Test_runUnset_envNotBound(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newClient(t, map[string]route{
		listPath:                   okJSON(listResponse(mcpItem("primary", getUUID))),
		configPath("GET", getUUID): okJSON(getResponseFixture()), // dev + prod only
	})
	defer cleanup()

	opts := newUnsetOpts(io, client)
	opts.Env = "staging"
	if err := runUnset(context.Background(), opts); err == nil {
		t.Fatal("expected error")
	}
	if code := decodeEnvelope(t, out.String())["error"].(map[string]any)["code"]; code != clierr.NotFound {
		t.Fatalf("code = %v, want %s", code, clierr.NotFound)
	}
}

func Test_runUnset_envLastRemainingRejected(t *testing.T) {
	io, out, _ := newTestIO(true)
	single := getResponseFixture()
	single.EnvMappings = map[string]amsvc.EnvProviderConfigMappings{
		"dev": {EnvironmentName: "dev", Configuration: &amsvc.ProviderConfig{ProviderName: "github", ProxyName: modelconfig.StringPtr("github")}},
	}
	client, cleanup := newClient(t, map[string]route{
		listPath:                   okJSON(listResponse(mcpItem("primary", getUUID))),
		configPath("GET", getUUID): okJSON(single),
	})
	defer cleanup()

	opts := newUnsetOpts(io, client)
	opts.Env = "dev"
	if err := runUnset(context.Background(), opts); err == nil {
		t.Fatal("expected error")
	}
	errBody := decodeEnvelope(t, out.String())["error"].(map[string]any)
	if errBody["code"] != clierr.InvalidFlag {
		t.Fatalf("code = %v, want %s", errBody["code"], clierr.InvalidFlag)
	}
	// Must guide the user to a full unset.
	if msg := errBody["message"].(string); !strings.Contains(msg, "--env") {
		t.Errorf("message should mention dropping --env for full delete, got %q", msg)
	}
}

func Test_runUnset_nameNotFound(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newClient(t, map[string]route{
		listPath: okJSON(listResponse(mcpItem("other", getUUID))),
	})
	defer cleanup()

	opts := newUnsetOpts(io, client)
	opts.Yes = true
	if err := runUnset(context.Background(), opts); err == nil {
		t.Fatal("expected error")
	}
	if code := decodeEnvelope(t, out.String())["error"].(map[string]any)["code"]; code != clierr.NotFound {
		t.Fatalf("code = %v, want %s", code, clierr.NotFound)
	}
}

func Test_runUnset_deleteServerError(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, cleanup := newClient(t, map[string]route{
		listPath:                      okJSON(listResponse(mcpItem("primary", getUUID))),
		configPath("DELETE", getUUID): {status: http.StatusNotFound, body: map[string]any{"code": "MODEL_CONFIG_NOT_FOUND", "message": "gone"}},
	})
	defer cleanup()

	opts := newUnsetOpts(io, client)
	opts.Yes = true
	if err := runUnset(context.Background(), opts); err == nil {
		t.Fatal("expected error")
	}
	if status := decodeEnvelope(t, out.String())["error"].(map[string]any)["status"]; status.(float64) != 404 {
		t.Fatalf("status = %v, want 404", status)
	}
}
