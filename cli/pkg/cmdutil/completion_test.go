// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// Licensed under the Apache License, Version 2.0.
package cmdutil

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/config"
)

func TestCompleteInstances_SortedNames(t *testing.T) {
	cfg := &config.Config{
		Instances: map[string]config.Instance{
			"prod":    {URL: "https://prod"},
			"dev":     {URL: "https://dev"},
			"staging": {URL: "https://staging"},
		},
	}
	f := &Factory{Config: func() (*config.Config, error) { return cfg, nil }}

	got := CompleteInstances(f)
	want := []string{"dev", "prod", "staging"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CompleteInstances = %v, want %v", got, want)
	}
}

func TestCompleteInstances_EmptyConfig(t *testing.T) {
	cfg := &config.Config{Instances: map[string]config.Instance{}}
	f := &Factory{Config: func() (*config.Config, error) { return cfg, nil }}

	got := CompleteInstances(f)
	if len(got) != 0 {
		t.Errorf("CompleteInstances = %v, want empty slice", got)
	}
}

func TestCompleteInstances_ConfigError(t *testing.T) {
	f := &Factory{Config: func() (*config.Config, error) { return nil, errors.New("boom") }}

	got := CompleteInstances(f)
	if got != nil {
		t.Errorf("CompleteInstances = %v, want nil on config error", got)
	}
}

// Helper that swaps out userCacheDir for the duration of the test, restoring
// it on cleanup. Returns the temp cache dir to assert against.
func withTempCacheDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	prev := userCacheDir
	userCacheDir = func() (string, error) { return dir, nil }
	t.Cleanup(func() { userCacheDir = prev })
	return dir
}

func TestLogCompletionErr_DisabledByDefault(t *testing.T) {
	t.Setenv("AMCTL_COMPLETION_DEBUG", "")
	cacheDir := withTempCacheDir(t)

	logCompletionErr("CompleteOrgs", map[string]string{"org": "x"}, errors.New("boom"))

	logPath := filepath.Join(cacheDir, "amctl", "completion.log")
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Errorf("log file exists when AMCTL_COMPLETION_DEBUG unset: err=%v", err)
	}
}

func TestLogCompletionErr_EnabledWritesLine(t *testing.T) {
	t.Setenv("AMCTL_COMPLETION_DEBUG", "1")
	cacheDir := withTempCacheDir(t)

	logCompletionErr("CompleteProjects", map[string]string{"org": "acme"}, errors.New("transport: nope"))

	logPath := filepath.Join(cacheDir, "amctl", "completion.log")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	line := string(data)
	if !strings.Contains(line, "CompleteProjects") {
		t.Errorf("log line missing op: %q", line)
	}
	if !strings.Contains(line, "org=acme") {
		t.Errorf("log line missing kv: %q", line)
	}
	if !strings.Contains(line, "transport: nope") {
		t.Errorf("log line missing err: %q", line)
	}
}

func TestLogCompletionErr_NilErrIsNoop(t *testing.T) {
	t.Setenv("AMCTL_COMPLETION_DEBUG", "1")
	cacheDir := withTempCacheDir(t)

	logCompletionErr("CompleteOrgs", nil, nil)

	logPath := filepath.Join(cacheDir, "amctl", "completion.log")
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Errorf("log file exists when err is nil: err=%v", err)
	}
}

func newOrgServer(t *testing.T, status int, names []string) (*Factory, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			items := make([]amsvc.OrganizationListItem, 0, len(names))
			for _, n := range names {
				items = append(items, amsvc.OrganizationListItem{Name: n, CreatedAt: time.Now().UTC()})
			}
			_ = json.NewEncoder(w).Encode(amsvc.OrganizationListResponse{
				Organizations: items, Limit: 20, Offset: 0, Total: len(items),
			})
		}
	}))
	client, err := amsvc.NewClientWithResponses(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("new client: %v", err)
	}
	f := &Factory{
		AgentManager: func(context.Context) (*amsvc.ClientWithResponses, error) {
			return client, nil
		},
	}
	return f, server.Close
}

func newCmdWithCtx(ctx context.Context) *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.SetContext(ctx)
	return cmd
}

func TestCompleteOrgs_ReturnsSortedNames(t *testing.T) {
	f, cleanup := newOrgServer(t, http.StatusOK, []string{"prod", "acme", "labs"})
	defer cleanup()

	got := CompleteOrgs(newCmdWithCtx(context.Background()), f)
	want := []string{"acme", "labs", "prod"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CompleteOrgs = %v, want %v", got, want)
	}
}

func TestCompleteOrgs_ServerErrorReturnsNil(t *testing.T) {
	f, cleanup := newOrgServer(t, http.StatusInternalServerError, nil)
	defer cleanup()

	got := CompleteOrgs(newCmdWithCtx(context.Background()), f)
	if got != nil {
		t.Errorf("CompleteOrgs on 500 = %v, want nil", got)
	}
}

func TestCompleteOrgs_ClientFactoryErrorReturnsNil(t *testing.T) {
	f := &Factory{
		AgentManager: func(context.Context) (*amsvc.ClientWithResponses, error) {
			return nil, errors.New("no instance")
		},
	}
	got := CompleteOrgs(newCmdWithCtx(context.Background()), f)
	if got != nil {
		t.Errorf("CompleteOrgs with bad factory = %v, want nil", got)
	}
}

func newProjectServer(t *testing.T, expectedOrg string, status int, names []string) (*Factory, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// The organization comes from the token, so it is never in the path.
		if r.URL.Path != "/projects" {
			t.Errorf("path = %q, want /projects", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			items := make([]amsvc.ProjectDocument, 0, len(names))
			for _, n := range names {
				items = append(items, amsvc.ProjectDocument{Metadata: amsvc.DocumentMetadata{Name: n}, Spec: amsvc.ProjectSpec{DisplayName: n}})
			}
			_ = json.NewEncoder(w).Encode(amsvc.ProjectListResponse{Count: int32(len(items)), List: items})
		}
	}))
	client, err := amsvc.NewClientWithResponses(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("new client: %v", err)
	}
	cfg := &config.Config{
		CurrentInstance: "default",
		Instances: map[string]config.Instance{
			"default": {URL: server.URL, CurrentOrg: expectedOrg},
		},
	}
	f := &Factory{
		Config: func() (*config.Config, error) { return cfg, nil },
		AgentManager: func(context.Context) (*amsvc.ClientWithResponses, error) {
			return client, nil
		},
	}
	return f, server.Close
}

func TestCompleteProjects_UsesCurrentOrg(t *testing.T) {
	f, cleanup := newProjectServer(t, "acme", http.StatusOK, []string{"triage", "billing", "ops"})
	defer cleanup()

	cmd := newCmdWithCtx(context.Background())
	cmd.Flags().String("org", "", "")
	cmd.Flags().String("project", "", "")

	got := CompleteProjects(cmd, f)
	want := []string{"billing", "ops", "triage"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CompleteProjects = %v, want %v", got, want)
	}
}

func TestCompleteProjects_OrgFlagOverridesCurrentOrg(t *testing.T) {
	f, cleanup := newProjectServer(t, "labs", http.StatusOK, []string{"x", "y"})
	defer cleanup()

	// Override CurrentOrg to "acme"; --org=labs should win.
	cfg, _ := f.Config()
	inst := cfg.Instances["default"]
	inst.CurrentOrg = "acme"
	cfg.Instances["default"] = inst

	cmd := newCmdWithCtx(context.Background())
	cmd.Flags().String("org", "", "")
	cmd.Flags().String("project", "", "")
	if err := cmd.Flags().Set("org", "labs"); err != nil {
		t.Fatalf("set org flag: %v", err)
	}

	got := CompleteProjects(cmd, f)
	want := []string{"x", "y"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CompleteProjects = %v, want %v", got, want)
	}
}

func TestCompleteProjects_NoOrgReturnsNil(t *testing.T) {
	f := &Factory{
		Config: func() (*config.Config, error) {
			return &config.Config{Instances: map[string]config.Instance{}}, nil
		},
	}
	cmd := newCmdWithCtx(context.Background())
	cmd.Flags().String("org", "", "")
	cmd.Flags().String("project", "", "")

	got := CompleteProjects(cmd, f)
	if got != nil {
		t.Errorf("CompleteProjects with no org = %v, want nil", got)
	}
}

func newAgentServer(t *testing.T, expectedOrg, expectedProj string, status int, names []string) (*Factory, func()) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("project"); got != expectedProj {
			t.Errorf("project = %q, want %q", got, expectedProj)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if status == http.StatusOK {
			agents := make([]amsvc.AgentDocument, 0, len(names))
			for _, n := range names {
				created := time.Now().UTC()
				agents = append(agents, amsvc.AgentDocument{
					Metadata: amsvc.ProjectScopedDocumentMetadata{Name: n, Project: expectedProj, CreationTimestamp: &created},
					Spec:     amsvc.AgentSpec{DisplayName: n},
				})
			}
			_ = json.NewEncoder(w).Encode(amsvc.AgentListResponse{Count: int32(len(agents)), List: agents})
		}
	}))
	client, err := amsvc.NewClientWithResponses(server.URL)
	if err != nil {
		server.Close()
		t.Fatalf("new client: %v", err)
	}
	cfg := &config.Config{
		CurrentInstance: "default",
		Instances: map[string]config.Instance{
			"default": {URL: server.URL, CurrentOrg: expectedOrg},
		},
	}
	f := &Factory{
		Config: func() (*config.Config, error) { return cfg, nil },
		AgentManager: func(context.Context) (*amsvc.ClientWithResponses, error) {
			return client, nil
		},
	}
	return f, server.Close
}

func TestCompleteAgents_UsesResolvedOrgAndProject(t *testing.T) {
	f, cleanup := newAgentServer(t, "acme", "triage", http.StatusOK, []string{"order-bot", "alpha-bot"})
	defer cleanup()

	cmd := newCmdWithCtx(context.Background())
	cmd.Flags().String("org", "", "")
	cmd.Flags().String("project", "", "")
	if err := cmd.Flags().Set("project", "triage"); err != nil {
		t.Fatalf("set project flag: %v", err)
	}

	got := CompleteAgents(cmd, f)
	want := []string{"alpha-bot", "order-bot"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("CompleteAgents = %v, want %v", got, want)
	}
}

func TestCompleteAgents_MissingProjectReturnsNil(t *testing.T) {
	f, cleanup := newAgentServer(t, "acme", "triage", http.StatusOK, []string{"x"})
	defer cleanup()

	cmd := newCmdWithCtx(context.Background())
	cmd.Flags().String("org", "", "")
	cmd.Flags().String("project", "", "")
	// no project flag set

	got := CompleteAgents(cmd, f)
	if got != nil {
		t.Errorf("CompleteAgents without project = %v, want nil", got)
	}
}

func TestIsRuntimeManaged_InternalIsTrue(t *testing.T) {
	agent := amsvc.AgentDocument{
		Metadata: amsvc.ProjectScopedDocumentMetadata{Name: "my-agent"},
		Spec:     amsvc.AgentSpec{Provisioning: amsvc.AgentProvisioning{Type: amsvc.AgentProvisioningKind(amsvc.ProvisioningTypeInternal)}},
	}
	if !IsRuntimeManaged(agent) {
		t.Error("IsRuntimeManaged = false for internal agent, want true")
	}
}

func TestIsRuntimeManaged_ExternalIsFalse(t *testing.T) {
	agent := amsvc.AgentDocument{
		Metadata: amsvc.ProjectScopedDocumentMetadata{Name: "ext-agent"},
		Spec:     amsvc.AgentSpec{Provisioning: amsvc.AgentProvisioning{Type: amsvc.AgentProvisioningKindExternal}},
	}
	if IsRuntimeManaged(agent) {
		t.Error("IsRuntimeManaged = true for external agent, want false")
	}
}
