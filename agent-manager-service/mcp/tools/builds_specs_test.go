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

import "testing"

// Returns the test specs for tools registered by registerBuildTools.
// New tools added to builds.go must have a spec here — registration_test.go fails the build otherwise.
func buildToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_builds",
			toolset:             "build",
			descriptionKeywords: []string{"list", "build"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name"},
			optionalParams:      []string{"org_name", "limit", "offset"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
			},
			expectedMethod: "ListAgentBuilds",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
				if got, want := args[2], testAgentName; got != want {
					t.Errorf("agentName: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "get_build_details",
			toolset:             "build",
			descriptionKeywords: []string{"build"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name", "build_name"},
			optionalParams:      []string{"org_name"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
				"build_name":   testBuildName,
			},
			expectedMethod: "GetBuild",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
				if got, want := args[2], testAgentName; got != want {
					t.Errorf("agentName: got %v, want %q", got, want)
				}
				if got, want := args[3], testBuildName; got != want {
					t.Errorf("buildName: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "build_agent",
			toolset:             "build",
			descriptionKeywords: []string{"build"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name"},
			optionalParams:      []string{"org_name", "commit_id"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
			},
			expectedMethod: "BuildAgent",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
				if got, want := args[2], testAgentName; got != want {
					t.Errorf("agentName: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "get_build_logs",
			toolset:             "build",
			descriptionKeywords: []string{"build", "log"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name", "build_name"},
			optionalParams:      []string{"org_name"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
				"build_name":   testBuildName,
			},
			expectedMethod: "GetBuildLogs",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
				if got, want := args[2], testAgentName; got != want {
					t.Errorf("agentName: got %v, want %q", got, want)
				}
				if got, want := args[3], testBuildName; got != want {
					t.Errorf("buildName: got %v, want %q", got, want)
				}
			},
		},
	}
}
