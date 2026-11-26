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
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
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

// agentServer answers GET /agents/{name} with status and body, and records the project query.
func agentServer(t *testing.T, status int, body any) (func(context.Context) (*amsvc.ClientWithResponses, error), *string) {
	t.Helper()
	project := new(string)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*project = r.URL.Query().Get("project")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(server.Close)
	client, err := amsvc.NewClientWithResponses(server.URL)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	return func(context.Context) (*amsvc.ClientWithResponses, error) { return client, nil }, project
}

func fullAgent() amsvc.AgentDocument {
	created := time.Date(2025, 3, 15, 0, 0, 0, 0, time.UTC)
	desc, sub, state := "Sorts orders", "chat-api", "active"
	return amsvc.AgentDocument{
		Metadata: amsvc.ProjectScopedDocumentMetadata{Name: "order-triage", Project: "triage", CreationTimestamp: &created},
		Spec: amsvc.AgentSpec{
			DisplayName: "Order Triage",
			Description: &desc,
			AgentType:   amsvc.AgentType{Type: "agent-api", SubType: &sub},
		},
		Status: &amsvc.AgentStatus{State: &state},
	}
}

func TestGet_TextOutput(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.JSON = false
	client, project := agentServer(t, http.StatusOK, fullAgent())

	err := runGet(context.Background(), &GetOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if *project != "triage" {
		t.Errorf("project query = %q, want triage", *project)
	}
	got := out.String()
	for _, want := range []string{"order-triage", "Order Triage", "Sorts orders", "agent-api / chat-api", "active", "triage", "2025-03-15T00:00:00Z"} {
		if !strings.Contains(got, want) {
			t.Errorf("output missing %q, got %q", want, got)
		}
	}
}

func TestGet_TextOutputWithoutOptionalFields(t *testing.T) {
	io, _, out, _ := iostreams.Test()
	io.JSON = false
	client, _ := agentServer(t, http.StatusOK, amsvc.AgentDocument{
		Metadata: amsvc.ProjectScopedDocumentMetadata{Name: "bare", Project: "triage"},
		Spec:     amsvc.AgentSpec{DisplayName: "Bare", AgentType: amsvc.AgentType{Type: "agent-api"}},
	})

	if err := runGet(context.Background(), &GetOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "bare",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "status:        -") || !strings.Contains(got, "created:       -") {
		t.Errorf("missing status and created should print -, got %q", got)
	}
	if strings.Contains(got, " / ") {
		t.Errorf("type without a sub-type should print alone, got %q", got)
	}
}

func TestGet_JSONOutput(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, _ := agentServer(t, http.StatusOK, fullAgent())

	if err := runGet(context.Background(), &GetOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage",
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	data, ok := env["data"].(map[string]any)
	if !ok {
		t.Fatalf("missing data key, got %v", env)
	}
	if md, _ := data["metadata"].(map[string]any); md["name"] != "order-triage" {
		t.Errorf("metadata = %v, want name order-triage", data["metadata"])
	}
}

func TestGet_NotFound(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, _ := agentServer(t, http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "agent not found"})

	err := runGet(context.Background(), &GetOptions{
		IO: io, Client: client, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "missing",
	})
	if err == nil {
		t.Fatal("expected error for a missing agent")
	}
	env := decodeEnvelope(t, out.String())
	errBody, ok := env["error"].(map[string]any)
	if !ok {
		t.Fatalf("missing error key, got %v", env)
	}
	if errBody["code"] != clierr.NotFound {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.NotFound)
	}
}

func TestGet_ClientError(t *testing.T) {
	io, out, _ := newTestIO(true)
	failing := func(context.Context) (*amsvc.ClientWithResponses, error) { return nil, errors.New("no credentials") }

	if err := runGet(context.Background(), &GetOptions{
		IO: io, Client: failing, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage",
	}); err == nil {
		t.Fatal("expected error when the client cannot be built")
	}
	if _, ok := decodeEnvelope(t, out.String())["error"]; !ok {
		t.Errorf("expected error envelope, got %q", out.String())
	}
}

func TestGet_TransportError(t *testing.T) {
	io, out, _ := newTestIO(true)
	client, err := amsvc.NewClientWithResponses("http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	if err := runGet(context.Background(), &GetOptions{
		IO: io, Client: func(context.Context) (*amsvc.ClientWithResponses, error) { return client, nil }, Scope: baseScope(),
		Org: "acme", Proj: "triage", AgentName: "order-triage",
	}); err == nil {
		t.Fatal("expected a transport error")
	}
	errBody, _ := decodeEnvelope(t, out.String())["error"].(map[string]any)
	if errBody["code"] != clierr.Transport {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.Transport)
	}
}
