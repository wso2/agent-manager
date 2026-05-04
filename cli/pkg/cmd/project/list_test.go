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

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
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
