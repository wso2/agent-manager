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

package tools

import (
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// returns the test specs for tools registered by registerProjectTools.
// New tools added to projects.go must have a spechere — registration_test.go fails the build otherwise.
func projectToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_projects",
			toolset:             "project",
			descriptionKeywords: []string{"list", "project"},
			descriptionMinLen:   20,
			requiredParams:      nil,
			optionalParams:      []string{"org_name", "limit", "offset"},
			testArgs:            map[string]any{"org_name": testOrgName},
			expectedMethod:      "ListProjects",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "create_project",
			toolset:             "project",
			descriptionKeywords: []string{"create", "project"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "display_name"},
			optionalParams:      []string{"org_name", "description"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"display_name": testDisplayName,
			},
			expectedMethod: "CreateProject",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				req, ok := args[1].(spec.CreateProjectRequest)
				if !ok {
					t.Fatalf("args[1] is not spec.CreateProjectRequest: %T", args[1])
				}
				if got, want := req.DisplayName, testDisplayName; got != want {
					t.Errorf("CreateProjectRequest.DisplayName: got %q, want %q", got, want)
				}
				if got, want := req.Name, testProjectName; got != want {
					t.Errorf("CreateProjectRequest.Name: got %q, want %q", got, want)
				}
			},
		},
	}
}
