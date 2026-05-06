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
	"strings"
	"testing"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

type capturedRequest struct {
	called bool
	method string
	path   string
}

func newTestClient(t *testing.T, status int, body any) (func(context.Context) (*amsvc.ClientWithResponses, error), *capturedRequest, func()) {
	t.Helper()
	captured := &capturedRequest{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		captured.called = true
		captured.method = r.Method
		captured.path = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			if err := json.NewEncoder(w).Encode(body); err != nil {
				t.Errorf("encode response: %v", err)
			}
		}
	}))
	client, err := amsvc.NewClientWithResponses(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("new client: %v", err)
	}
	return func(context.Context) (*amsvc.ClientWithResponses, error) { return client, nil }, captured, server.Close
}

func unreachableClient(context.Context) (*amsvc.ClientWithResponses, error) {
	return nil, errors.New("client should not be constructed")
}

type fakePrompter struct {
	confirmDeletionErr error
	confirmDeletionArg string
	calls              int
}

func (p *fakePrompter) ConfirmDeletion(required string) error {
	p.calls++
	p.confirmDeletionArg = required
	return p.confirmDeletionErr
}

func newTestIO(canPrompt bool) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	io, _, out, errOut := iostreams.Test()
	io.SetTerminal(canPrompt, canPrompt, canPrompt)
	io.JSON = true
	return io, out, errOut
}

func decodeEnvelope(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decode envelope: %v\nbody=%q", err, raw)
	}
	return m
}

func baseScope() render.Scope {
	return render.Scope{Instance: "default", Org: "acme", Project: "triage"}
}

func TestDelete_NonTTYWithoutYes(t *testing.T) {
	io, out, _ := newTestIO(false)
	prompter := &fakePrompter{}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage", Yes: false,
	})
	if err == nil {
		t.Fatal("expected error for non-TTY without --yes")
	}
	env := decodeEnvelope(t, out.String())
	errBody, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error key, got %v", env)
	}
	if errBody["code"] != clierr.ConfirmationRequired {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.ConfirmationRequired)
	}
}

func TestDelete_MismatchedTypedName(t *testing.T) {
	io, out, _ := newTestIO(true)
	prompter := &fakePrompter{confirmDeletionErr: errors.New("confirmation \"oops\" did not match \"order-triage\"")}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage", Yes: false,
	})
	if err == nil {
		t.Fatal("expected error from prompter mismatch")
	}
	if prompter.calls != 1 {
		t.Errorf("prompter calls = %d, want 1", prompter.calls)
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.ConfirmationRequired {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.ConfirmationRequired)
	}
}

func TestDelete_Success204(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, captured, closeFn := newTestClient(t, http.StatusNoContent, nil)
	defer closeFn()
	prompter := &fakePrompter{}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage", Yes: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !captured.called {
		t.Fatal("server should have been called")
	}
	if captured.method != "DELETE" {
		t.Errorf("method = %q, want DELETE", captured.method)
	}
	if captured.path != "/orgs/acme/projects/triage/agents/order-triage" {
		t.Errorf("path = %q, want /orgs/acme/projects/triage/agents/order-triage", captured.path)
	}
	if prompter.calls != 1 {
		t.Errorf("prompter calls = %d, want 1", prompter.calls)
	}
	if prompter.confirmDeletionArg != "order-triage" {
		t.Errorf("confirmation arg = %q, want %q", prompter.confirmDeletionArg, "order-triage")
	}
	env := decodeEnvelope(t, out.String())
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data key in success envelope: %v", env)
	}
	if data["name"] != "order-triage" || data["deleted"] != true {
		t.Errorf("data = %v, want {name=order-triage, deleted=true}", data)
	}
}

func TestDelete_Server404(t *testing.T) {
	io, out, _ := newTestIO(true)
	reason := "not found"
	body := amsvc.ErrorResponse{
		Code:    "AGENT_NOT_FOUND",
		Message: "Agent 'order-triage' not found",
		Reason:  &reason,
	}
	client, _, closeFn := newTestClient(t, http.StatusNotFound, body)
	defer closeFn()
	prompter := &fakePrompter{}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage", Yes: true,
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	if prompter.calls != 0 {
		t.Errorf("prompter should not be called with --yes (calls=%d)", prompter.calls)
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "AGENT_NOT_FOUND" {
		t.Errorf("code = %v, want AGENT_NOT_FOUND", errBody["code"])
	}
	if errBody["status"].(float64) != 404 {
		t.Errorf("status = %v, want 404", errBody["status"])
	}
}

func TestDelete_RejectsEmptyName(t *testing.T) {
	io, out, _ := newTestIO(true)

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: &fakePrompter{}, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "", Yes: true,
	})
	if err == nil {
		t.Fatal("expected error for empty agent name")
	}
	env := decodeEnvelope(t, out.String())
	errBody, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error key, got %v", env)
	}
	if errBody["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.InvalidFlag)
	}
}

func TestDelete_RejectsSlashInName(t *testing.T) {
	io, out, _ := newTestIO(true)

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: &fakePrompter{}, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "foo/bar", Yes: true,
	})
	if err == nil {
		t.Fatal("expected error for slash in agent name")
	}
	env := decodeEnvelope(t, out.String())
	errBody, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error key, got %v", env)
	}
	if errBody["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.InvalidFlag)
	}
}

func TestDelete_YesSkipsPrompt(t *testing.T) {
	io, _, _ := newTestIO(false)
	client, captured, closeFn := newTestClient(t, http.StatusNoContent, nil)
	defer closeFn()
	prompter := &fakePrompter{}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage", Yes: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompter.calls != 0 {
		t.Errorf("prompter calls = %d, want 0 with --yes", prompter.calls)
	}
	if !captured.called {
		t.Fatal("server should have been called")
	}
}

func TestDelete_TextSuccess(t *testing.T) {
	io, _, out, errOut := iostreams.Test()
	io.JSON = false
	io.SetTerminal(true, true, true)
	client, _, closeFn := newTestClient(t, http.StatusNoContent, nil)
	defer closeFn()

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: &fakePrompter{}, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage", Yes: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty in text mode, got %q", out.String())
	}
	got := errOut.String()
	if !strings.Contains(got, "Deleted agent order-triage") {
		t.Errorf("stderr = %q, want it to contain success message", got)
	}
}

func TestDelete_TextError(t *testing.T) {
	io, _, out, errOut := iostreams.Test()
	io.JSON = false

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: &fakePrompter{}, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "", Yes: true,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if out.Len() != 0 {
		t.Errorf("stdout should be empty in text error mode, got %q", out.String())
	}
	if errOut.Len() == 0 {
		t.Fatal("expected error message on stderr")
	}
}
