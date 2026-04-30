package agent

import (
	"context"
	"testing"

	"github.com/wso2/agent-manager/internal/amctl/clierr"
)

func TestGet_RejectsEmptyName(t *testing.T) {
	io, out, _ := newTestIO(true)

	err := runGet(context.Background(), &GetOptions{
		IO: io, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "",
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

func TestGet_RejectsSlashInName(t *testing.T) {
	io, out, _ := newTestIO(true)

	err := runGet(context.Background(), &GetOptions{
		IO: io, Client: unreachableClient, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "foo/bar",
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
