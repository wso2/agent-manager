package project

import (
	"context"
	"net/http"
	"testing"

	amsvc "github.com/wso2/agent-manager/internal/amctl/clients/amsvc/gen"
)

func TestList_Success(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusOK, amsvc.ProjectListResponse{
		Limit:  20,
		Offset: 0,
		Total:  2,
		Projects: []amsvc.ProjectListItem{
			{Name: "alpha", DisplayName: "Alpha", OrgName: "acme"},
			{Name: "beta", DisplayName: "Beta", OrgName: "acme"},
		},
	})
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !captured.called {
		t.Fatal("server should have been called")
	}
	if captured.path != "/orgs/acme/projects" {
		t.Errorf("path = %q, want /orgs/acme/projects", captured.path)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	projects := data["projects"].([]any)
	if len(projects) != 2 {
		t.Errorf("len(projects) = %d, want 2", len(projects))
	}
}

func TestList_ServerError(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, _, closeFn := newTestClient(t, http.StatusInternalServerError, amsvc.ErrorResponse{
		Code:    "INTERNAL",
		Message: "something broke",
	})
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
	})
	if err == nil {
		t.Fatal("expected error for 500")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "INTERNAL" {
		t.Errorf("code = %v, want INTERNAL", errBody["code"])
	}
}
