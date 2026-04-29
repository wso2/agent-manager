package project

import (
	"context"
	"errors"
	"net/http"
	"testing"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/clierr"
)

func TestDelete_Success(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusNoContent, nil)
	defer closeFn()
	prompter := &fakePrompter{}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: clientFn, Scope: baseScope(),
		Org: "acme", ProjectName: "alpha", Yes: false,
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
	if captured.path != "/orgs/acme/projects/alpha" {
		t.Errorf("path = %q, want /orgs/acme/projects/alpha", captured.path)
	}
	if prompter.calls != 1 {
		t.Errorf("prompter calls = %d, want 1", prompter.calls)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	if data["name"] != "alpha" || data["deleted"] != true {
		t.Errorf("data = %v, want {name=alpha, deleted=true}", data)
	}
}

func TestDelete_YesSkipsPrompt(t *testing.T) {
	io, _, _ := newTestIO(false)
	clientFn, captured, closeFn := newTestClient(t, http.StatusNoContent, nil)
	defer closeFn()
	prompter := &fakePrompter{}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: clientFn, Scope: baseScope(),
		Org: "acme", ProjectName: "alpha", Yes: true,
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

func TestDelete_NonTTYWithoutYes(t *testing.T) {
	io, out, _ := newTestIO(false)

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: &fakePrompter{}, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", ProjectName: "alpha", Yes: false,
	})
	if err == nil {
		t.Fatal("expected error for non-TTY without --yes")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.ConfirmationRequired {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.ConfirmationRequired)
	}
}

func TestDelete_NotFound(t *testing.T) {
	io, out, _ := newTestIO(true)
	reason := "not found"
	clientFn, _, closeFn := newTestClient(t, http.StatusNotFound, amsvc.ErrorResponse{
		Code:    "PROJECT_NOT_FOUND",
		Message: "Project 'alpha' not found",
		Reason:  &reason,
	})
	defer closeFn()

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: &fakePrompter{}, Client: clientFn, Scope: baseScope(),
		Org: "acme", ProjectName: "alpha", Yes: true,
	})
	if err == nil {
		t.Fatal("expected error for 404")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "PROJECT_NOT_FOUND" {
		t.Errorf("code = %v, want PROJECT_NOT_FOUND", errBody["code"])
	}
}

func TestDelete_ConfirmationMismatch(t *testing.T) {
	io, out, _ := newTestIO(true)
	prompter := &fakePrompter{confirmDeletionErr: errors.New("confirmation mismatch")}

	err := runDelete(context.Background(), &DeleteOptions{
		IO: io, Prompter: prompter, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", ProjectName: "alpha", Yes: false,
	})
	if err == nil {
		t.Fatal("expected error from confirmation mismatch")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.ConfirmationRequired {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.ConfirmationRequired)
	}
}
