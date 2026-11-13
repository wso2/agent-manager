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

	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// Returns the test specs for tools registered by registerDeploymentTools.
// New tools added to deployments.go must have a spec here — registration_test.go fails the build otherwise.
func deploymentToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_deployments",
			toolset:             "deployment",
			permissions:         []rbac.Permission{rbac.AgentRead},
			descriptionKeywords: []string{"deployment"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name"},
			optionalParams:      nil,
			testArgs: map[string]any{
				"project_name": testProjectName,
				"agent_name":   testAgentName,
			},
			expectedMethod: "GetAgentDeployments",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("ouID: got %v, want %q", got, want)
				}
				if got, want := args[1], testProjectName; got != want {
					t.Errorf("projectName: got %v, want %q", got, want)
				}
				if got, want := args[2], testAgentName; got != want {
					t.Errorf("agentName: got %v, want %q", got, want)
				}
			},
		},
	}
}
