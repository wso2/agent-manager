package project

import (
	"context"
	"net/http"
	"testing"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/clierr"
)

func TestGet_Success(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, amsvc.ProjectResponse{
		Name:               "alpha",
		DisplayName:        "Alpha",
		OrgName:            "acme",
		DeploymentPipeline: "default",
	})
	defer closeFn()

	err := runGet(context.Background(), &GetOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), ProjectName: "alpha",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !captured.called {
		t.Fatal("server should have been called")
	}
	if captured.path != "/orgs/acme/projects/alpha" {
		t.Errorf("path = %q, want /orgs/acme/projects/alpha", captured.path)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	if data["name"] != "alpha" {
		t.Errorf("name = %v, want alpha", data["name"])
	}
}

func TestGet_NotFound(t *testing.T) {
	io, out, _ := newTestIO(true)
	reason := "not found"
	clientFn, _, closeFn := newTestClient(t, http.StatusNotFound, amsvc.ErrorResponse{
		Code:    "PROJECT_NOT_FOUND",
		Message: "Project 'nope' not found",
		Reason:  &reason,
	})
	defer closeFn()

	err := runGet(context.Background(), &GetOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(), ProjectName: "nope",
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

func TestGet_EmptyName(t *testing.T) {
	io, out, _ := newTestIO(true)

	err := runGet(context.Background(), &GetOptions{
		IO: io, Client: unreachableClient, Org: "acme", Scope: baseScope(), ProjectName: "",
	})
	if err == nil {
		t.Fatal("expected error for empty name")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.InvalidFlag)
	}
}
