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

// Returns the test specs for tools registered by registerDeploymentTools.
// New tools added to deployments.go must have a spec here — registration_test.go fails the build otherwise.
func deploymentToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_deployments",
			toolset:             "deployment",
			descriptionKeywords: []string{"deployment"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name"},
			optionalParams:      []string{"org_name"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
			},
			expectedMethod: "GetAgentDeployments",
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
			name:                "deploy_agent",
			toolset:             "deployment",
			descriptionKeywords: []string{"deploy"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name", "image_id"},
			optionalParams: []string{
				"org_name", "enable_auto_instrumentation", "env",
			},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
				"image_id":     "test-image:v1",
			},
			expectedMethod: "DeployAgent",
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
				// args[3] is *spec.DeployAgentRequest; tier 3 verifies its
				// fields (image_id propagation, env mapping, etc.)
			},
		},
		{
			name:                "update_deployment_state",
			toolset:             "deployment",
			descriptionKeywords: []string{"deployment", "state"},
			descriptionMinLen:   20,
			requiredParams: []string{
				"project_name", "agent_name", "environment", "state",
			},
			optionalParams: []string{"org_name"},
			testArgs: map[string]any{
				"org_name":     testOrgName,
				"project_name": testProjectName,
				"agent_name":   testAgentName,
				"environment":  testEnvName,
				"state":        "redeploy",
			},
			expectedMethod: "UpdateDeploymentState",
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
				if got, want := args[3], testEnvName; got != want {
					t.Errorf("environment: got %v, want %q", got, want)
				}
				// `redeploy` is mapped to "Active" before reaching the
				// service. Tier 3 owns that specific assertion.
			},
		},
	}
}
