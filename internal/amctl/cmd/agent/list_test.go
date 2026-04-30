package agent

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	amsvc "github.com/wso2/agent-manager/internal/amctl/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/amctl/iostreams"
)

func TestList_TextOutput(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.JSON = false
	status := "active"
	client, _, closeFn := newTestClient(t, http.StatusOK, amsvc.AgentListResponse{
		Agents: []amsvc.AgentResponse{
			{Name: "order-triage", DisplayName: "Order Triage", Status: &status, CreatedAt: time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)},
		},
		Limit: 20, Offset: 0, Total: 1,
	})
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "order-triage") {
		t.Errorf("output should contain agent name, got %q", got)
	}
	if !strings.Contains(got, "Order Triage") {
		t.Errorf("output should contain display name, got %q", got)
	}
	if !strings.Contains(got, "active") {
		t.Errorf("output should contain status, got %q", got)
	}
}

func TestList_JSONOutput(t *testing.T) {
	io, out, _ := newTestIO(true)
	status := "active"
	client, _, closeFn := newTestClient(t, http.StatusOK, amsvc.AgentListResponse{
		Agents: []amsvc.AgentResponse{
			{Name: "order-triage", DisplayName: "Order Triage", Status: &status, CreatedAt: time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)},
		},
		Limit: 20, Offset: 0, Total: 1,
	})
	defer closeFn()

	err := runList(context.Background(), &ListOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	if _, ok := env["data"]; !ok {
		t.Fatal("expected data key in JSON envelope")
	}
}
