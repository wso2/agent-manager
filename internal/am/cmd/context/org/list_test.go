package org

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	amsvc "github.com/wso2/agent-manager/internal/am/clients/amsvc/gen"
	"github.com/wso2/agent-manager/internal/am/clierr"
	"github.com/wso2/agent-manager/internal/am/config"
	"github.com/wso2/agent-manager/internal/am/iostreams"
)

func newTestIO() (*iostreams.IOStreams, *bytes.Buffer) {
	io, _, out, _ := iostreams.Test()
	return io, out
}

func decodeEnvelope(t *testing.T, raw string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("decode envelope: %v\nbody=%q", err, raw)
	}
	return m
}

func writeConfig(t *testing.T, cfg *config.Config) func() (*config.Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config")
	cfg.Path = path
	if err := cfg.Save(); err != nil {
		t.Fatalf("save config: %v", err)
	}
	return func() (*config.Config, error) { return config.Load(path) }
}

func newTestClient(t *testing.T, status int, body any) (func(context.Context) (*amsvc.ClientWithResponses, error), func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	return func(context.Context) (*amsvc.ClientWithResponses, error) { return client, nil }, server.Close
}

func TestList_Success(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "prod",
		Instances: map[string]config.Instance{
			"prod": {URL: "https://prod.example.com"},
		},
	})
	clientFn, closeFn := newTestClient(t, http.StatusOK, amsvc.OrganizationListResponse{
		Limit:  20,
		Offset: 0,
		Total:  2,
		Organizations: []amsvc.OrganizationListItem{
			{Name: "acme"},
			{Name: "globex"},
		},
	})
	defer closeFn()

	err := runList(context.Background(), &ListOptions{IO: io, Config: cfgFn, Client: clientFn})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	orgs := data["organizations"].([]any)
	if len(orgs) != 2 {
		t.Errorf("len(organizations) = %d, want 2", len(orgs))
	}
	if env["instance"] != "prod" {
		t.Errorf("instance = %v, want prod", env["instance"])
	}
}

func TestList_ServerError(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{
		CurrentInstance: "prod",
		Instances: map[string]config.Instance{
			"prod": {URL: "https://prod.example.com"},
		},
	})
	clientFn, closeFn := newTestClient(t, http.StatusInternalServerError, amsvc.ErrorResponse{
		Code:    "INTERNAL",
		Message: "something broke",
	})
	defer closeFn()

	err := runList(context.Background(), &ListOptions{IO: io, Config: cfgFn, Client: clientFn})
	if err == nil {
		t.Fatal("expected error for 500")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "INTERNAL" {
		t.Errorf("code = %v, want INTERNAL", errBody["code"])
	}
}

func TestList_NoInstance(t *testing.T) {
	io, out := newTestIO()
	cfgFn := writeConfig(t, &config.Config{})
	clientFn, closeFn := newTestClient(t, http.StatusOK, nil)
	defer closeFn()

	err := runList(context.Background(), &ListOptions{IO: io, Config: cfgFn, Client: clientFn})
	if err == nil {
		t.Fatal("expected error when no instance configured")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != clierr.NoInstance {
		t.Errorf("code = %v, want %s", errBody["code"], clierr.NoInstance)
	}
}
