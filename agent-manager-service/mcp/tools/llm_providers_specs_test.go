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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// Returns the test specs for tools registered by registerLLMProviderTools.
// New tools added to llm_providers.go must have a spec here — registration_test.go fails the build otherwise.
func llmProviderToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "list_llm_providers",
			toolset:             "llm-provider",
			permissions:         []rbac.Permission{rbac.LLMProviderRead},
			descriptionKeywords: []string{"list", "llm provider", "catalog"},
			descriptionMinLen:   20,
			requiredParams:      nil,
			optionalParams:      []string{"limit", "offset"},
			testArgs:            map[string]any{},
			expectedMethod:      "ListLLMProviders",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("ouID: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "add_llm_provider_to_catalog",
			toolset:             "llm-provider",
			permissions:         []rbac.Permission{rbac.LLMProviderUpdate},
			descriptionKeywords: []string{"llm provider", "catalog"},
			descriptionMinLen:   20,
			requiredParams:      []string{"provider_id"},
			optionalParams:      nil,
			testArgs:            map[string]any{"provider_id": testProviderID},
			expectedMethod:      "AddLLMProviderToCatalog",
			validateCall: func(t *testing.T, args []interface{}) {
				if got, want := args[0], testOrgName; got != want {
					t.Errorf("ouID: got %v, want %q", got, want)
				}
				if got, want := args[1], testProviderID; got != want {
					t.Errorf("providerID: got %v, want %q", got, want)
				}
			},
		},
		{
			name:                "attach_llm_provider",
			toolset:             "llm-provider",
			permissions:         []rbac.Permission{rbac.AgentUpdate},
			descriptionKeywords: []string{"attach", "llm provider", "agent"},
			descriptionMinLen:   20,
			requiredParams:      []string{"project_name", "agent_name", "provider_id"},
			optionalParams: []string{
				"environments", "config_name", "description", "url_env_var", "api_key_env_var",
			},
			testArgs: map[string]any{
				"project_name": testProjectName,
				"agent_name":   testAgentName,
				"provider_id":  testProviderID,
			},
			expectedMethod: "AttachLLMProvider",
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
				req, ok := args[3].(models.CreateAgentModelConfigRequest)
				if !ok {
					t.Fatalf("request has unexpected type %T", args[3])
				}
				if req.Type != models.AgentConfigTypeLLM {
					t.Errorf("type: got %q, want %q", req.Type, models.AgentConfigTypeLLM)
				}
				if got := req.EnvMappings[testEnvName].ProviderName; got != testProviderID {
					t.Errorf("env mapping provider: got %q, want %q", got, testProviderID)
				}
			},
		},
	}
}
