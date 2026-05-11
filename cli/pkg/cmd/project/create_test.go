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

package project

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/config"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func TestCreate_Success(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusAccepted, amsvc.ProjectResponse{
		Name:               "alpha",
		DisplayName:        "Alpha Project",
		OrgName:            "acme",
		DeploymentPipeline: "default",
		Description:        "a test project",
	})
	defer closeFn()

	desc := "a test project"
	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
		Name: "alpha", DisplayName: "Alpha Project", Description: desc,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !captured.called {
		t.Fatal("server should have been called")
	}
	if captured.method != "POST" {
		t.Errorf("method = %q, want POST", captured.method)
	}
	if captured.path != "/orgs/acme/projects" {
		t.Errorf("path = %q, want /orgs/acme/projects", captured.path)
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	if data["name"] != "alpha" {
		t.Errorf("name = %v, want alpha", data["name"])
	}
}

func TestCreate_Conflict(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, _, closeFn := newTestClient(t, http.StatusConflict, amsvc.ErrorResponse{
		Code:    "PROJECT_ALREADY_EXISTS",
		Message: "project 'alpha' already exists",
	})
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
		Name: "alpha", DisplayName: "Alpha",
	})
	if err == nil {
		t.Fatal("expected error for 409")
	}
	env := decodeEnvelope(t, out.String())
	errBody := env["error"].(map[string]any)
	if errBody["code"] != "PROJECT_ALREADY_EXISTS" {
		t.Errorf("code = %v, want PROJECT_ALREADY_EXISTS", errBody["code"])
	}
}

func TestCreate_NoDescription(t *testing.T) {
	io, out, _ := newTestIO(true)
	clientFn, _, closeFn := newTestClient(t, http.StatusAccepted, amsvc.ProjectResponse{
		Name:               "alpha",
		DisplayName:        "Alpha",
		OrgName:            "acme",
		DeploymentPipeline: "default",
	})
	defer closeFn()

	err := runCreate(context.Background(), &CreateOptions{
		IO: io, Client: clientFn, Org: "acme", Scope: baseScope(),
		Name: "alpha", DisplayName: "Alpha",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	env := decodeEnvelope(t, out.String())
	if _, ok := env["error"]; ok {
		t.Fatal("should succeed without description")
	}
}

func testProjectCreateCmd(t *testing.T, ios *iostreams.IOStreams, clientFn func(context.Context) (*amsvc.ClientWithResponses, error)) *cobra.Command {
	t.Helper()
	f := &cmdutil.Factory{
		IOStreams:     ios,
		AgentManager: clientFn,
		Config: func() (*config.Config, error) {
			return &config.Config{
				CurrentInstance: "default",
				Instances: map[string]config.Instance{
					"default": {URL: "http://test", CurrentOrg: "acme"},
				},
			}, nil
		},
	}
	root := &cobra.Command{Use: "amctl", SilenceErrors: true, SilenceUsage: true}
	cmdutil.EnableOrgOverride(root, f)

	projectCmd := NewProjectCmd(f)
	root.AddCommand(projectCmd)
	return root
}

func TestCreate_MissingNameAndDisplayName(t *testing.T) {
	ios, out, _ := newTestIO(true)
	cmd := testProjectCreateCmd(t, ios, nil)
	cmd.SetArgs([]string{"project", "create"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	env := decodeEnvelope(t, out.String())
	errMap := env["error"].(map[string]any)
	if errMap["code"] != clierr.InvalidFlag {
		t.Errorf("code = %v, want %s", errMap["code"], clierr.InvalidFlag)
	}
	additional := errMap["additionalData"].(map[string]any)
	details, ok := additional["details"].([]any)
	if !ok {
		t.Fatalf("details type = %T", additional["details"])
	}
	foundName := false
	foundDisplayName := false
	for _, d := range details {
		s := d.(string)
		if strings.Contains(s, "name argument is required") {
			foundName = true
		}
		if strings.Contains(s, "--display-name is required") {
			foundDisplayName = true
		}
	}
	if !foundName {
		t.Errorf("expected 'name argument is required' in details, got %v", details)
	}
	if !foundDisplayName {
		t.Errorf("expected '--display-name is required' in details, got %v", details)
	}
}

func TestCreate_NameWithSlash(t *testing.T) {
	ios, out, _ := newTestIO(true)
	cmd := testProjectCreateCmd(t, ios, nil)
	cmd.SetArgs([]string{"project", "create", "bad/name", "--display-name", "Bad"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	env := decodeEnvelope(t, out.String())
	errMap := env["error"].(map[string]any)
	additional := errMap["additionalData"].(map[string]any)
	details := additional["details"].([]any)
	found := false
	for _, d := range details {
		if strings.Contains(d.(string), "name must not contain '/'") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected slash violation in details, got %v", details)
	}
}

func TestCreate_PositionalName(t *testing.T) {
	ios, out, _ := newTestIO(true)
	clientFn, captured, closeFn := newTestClient(t, http.StatusAccepted, amsvc.ProjectResponse{
		Name:               "alpha",
		DisplayName:        "Alpha Project",
		OrgName:            "acme",
		DeploymentPipeline: "default",
	})
	defer closeFn()

	cmd := testProjectCreateCmd(t, ios, clientFn)
	cmd.SetArgs([]string{"project", "create", "alpha", "--display-name", "Alpha Project"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !captured.called {
		t.Fatal("server should have been called")
	}
	env := decodeEnvelope(t, out.String())
	data := env["data"].(map[string]any)
	if data["name"] != "alpha" {
		t.Errorf("name = %v, want alpha", data["name"])
	}
}

func TestCreate_MissingDisplayNameOnly(t *testing.T) {
	ios, out, _ := newTestIO(true)
	cmd := testProjectCreateCmd(t, ios, nil)
	cmd.SetArgs([]string{"project", "create", "alpha"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}

	env := decodeEnvelope(t, out.String())
	errMap := env["error"].(map[string]any)
	additional := errMap["additionalData"].(map[string]any)
	details := additional["details"].([]any)
	if len(details) != 1 {
		t.Errorf("expected 1 violation, got %d: %v", len(details), details)
	}
	if !strings.Contains(details[0].(string), "--display-name is required") {
		t.Errorf("expected '--display-name is required', got %v", details[0])
	}
}
