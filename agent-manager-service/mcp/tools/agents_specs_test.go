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

// Returns the test specs for tools registered by registerAgentTools.
// New tools added to agents.go must have a spec here — registration_test.go fails the build otherwise.
func agentToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_agents",
			toolset:             "agent",
			descriptionKeywords: []string{"list", "agent"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name"},
			optionalParams:      []string{"org_name", "limit", "offset"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
			},
			expectedMethod: "ListAgents",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "list_project_agent_pairs",
			toolset:             "agent",
			descriptionKeywords: []string{"project", "agent"},
			descriptionMinLen:   20,
			requiredParams:      nil,
			optionalParams: []string{
				"org_name", "project_search", "agent_search",
				"project_limit", "project_offset",
				"agent_limit", "agent_offset",
			},
			testArgs:       map[string]any{"org_name": testOrgName},
			expectedMethod: "ListProjects",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "create_external_agent",
			toolset:             "agent",
			descriptionKeywords: []string{"external", "agent"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name", "display_name", "language"},
			optionalParams:      []string{"org_name", "description"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
				"display_name": testDisplayName,
				"language":     "python",
			},
			expectedMethod: "CreateAgent",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("orgName: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
				req, ok := args[2].(*spec.CreateAgentRequest)
				if !ok {
					t.Fatalf("args[2] is not *spec.CreateAgentRequest: %T", args[2])
				}
				if got, want := req.Name, testAgentName; got != want {
					t.Errorf("CreateAgentRequest.Name: got %q, want %q", got, want)
				}
				if got, want := req.DisplayName, testDisplayName; got != want {
					t.Errorf("CreateAgentRequest.DisplayName: got %q, want %q", got, want)
				}
			},
		},
		{
			name:                "create_internal_agent_python",
			toolset:             "agent",
			descriptionKeywords: []string{"internal", "python", "agent"},
			descriptionMinLen:   20,
			requiredParams: []string{
				"project_name", "agent_name", "display_name", "repository_url",
				"branch", "app_path", "interface_type", "env",
			},
			optionalParams: []string{
				"org_name", "description", "language_version",
				"run_command", "port", "base_path", "openapi_path",
				"enable_auto_instrumentation",
			},
			// Skipping invocation for this tool: the schema requires `env`
			// (an array of {key,value} objects) and the production handler
			// has additional cross-field validation around interface_type.
			// Tier 3 covers it with a dedicated test.
			expectedMethod: "",
			validateCall:   func(t *testing.T, args []interface{}) {},
		},
	}
}
