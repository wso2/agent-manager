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
	"testing"

	amsvc "github.com/wso2/agent-manager/internal/amctl/clients/amsvc/gen"
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
