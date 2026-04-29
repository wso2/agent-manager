package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"

	"github.com/wso2/agent-manager/internal/am/clierr"
	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/iostreams"
	"github.com/wso2/agent-manager/internal/am/render"
)

type fakeDeleter struct {
	resp   *amsvc.DeleteAgentResp
	err    error
	called bool

	gotOrg, gotProj, gotAgent string
}

func (f *fakeDeleter) DeleteAgentWithResponse(ctx context.Context, orgName, projName, agentName string, _ ...amsvc.RequestEditorFn) (*amsvc.DeleteAgentResp, error) {
	f.called = true
	f.gotOrg, f.gotProj, f.gotAgent = orgName, projName, agentName
	return f.resp, f.err
}

type fakePrompter struct {
	confirmDeletionErr error
	confirmDeletionArg string
	calls              int
}

func (p *fakePrompter) Confirm(string, bool) (bool, error) { return false, nil }
func (p *fakePrompter) ConfirmDeletion(required string) error {
	p.calls++
	p.confirmDeletionArg = required
	return p.confirmDeletionErr
}

func newTestIO(canPrompt bool) (*iostreams.IOStreams, *bytes.Buffer, *bytes.Buffer) {
	io, _, out, errOut := iostreams.Test()
	io.SetTerminal(canPrompt, canPrompt)
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

func clientThunk(d *fakeDeleter) func(context.Context) (agentDeleter, error) {
	return func(context.Context) (agentDeleter, error) { return d, nil }
}

func TestDelete_NonTTYWithoutYes(t *testing.T) {
	io, out, _ := newTestIO(false)
	deleter := &fakeDeleter{}
	prompter := &fakePrompter{}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: clientThunk(deleter), Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage", Yes: false,
	})
	if err == nil {
		t.Fatal("expected error for non-TTY without --yes")
	}
	if deleter.called {
		t.Fatal("client should not be called when confirmation is required")
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
	deleter := &fakeDeleter{}
	prompter := &fakePrompter{confirmDeletionErr: errors.New("confirmation \"oops\" did not match \"order-triage\"")}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: clientThunk(deleter), Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage", Yes: false,
	})
	if err == nil {
		t.Fatal("expected error from prompter mismatch")
	}
	if deleter.called {
		t.Fatal("client should not be called when confirmation fails")
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
	deleter := &fakeDeleter{
		resp: &amsvc.DeleteAgentResp{
			HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
		},
	}
	prompter := &fakePrompter{}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: clientThunk(deleter), Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage", Yes: false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !deleter.called {
		t.Fatal("client should have been called")
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
	deleter := &fakeDeleter{
		resp: &amsvc.DeleteAgentResp{
			HTTPResponse: &http.Response{StatusCode: http.StatusNotFound},
			JSON404: &amsvc.ErrorResponse{
				Code:    "AGENT_NOT_FOUND",
				Message: "Agent 'order-triage' not found",
				Reason:  &reason,
			},
		},
	}
	prompter := &fakePrompter{}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: clientThunk(deleter), Scope: baseScope(),
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

func TestDelete_YesSkipsPrompt(t *testing.T) {
	io, _, _ := newTestIO(false)
	deleter := &fakeDeleter{
		resp: &amsvc.DeleteAgentResp{
			HTTPResponse: &http.Response{StatusCode: http.StatusNoContent},
		},
	}
	prompter := &fakePrompter{}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: clientThunk(deleter), Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage", Yes: true,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if prompter.calls != 0 {
		t.Errorf("prompter calls = %d, want 0 with --yes", prompter.calls)
	}
	if !deleter.called {
		t.Fatal("client should have been called")
	}
}
