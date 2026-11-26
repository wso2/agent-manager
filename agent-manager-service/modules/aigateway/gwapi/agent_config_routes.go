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

package gwapi

import (
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

// RegisterAgentConfigRoutes registers all agent configuration routes
func RegisterAgentConfigRoutes(rr Router, ctrl gwctl.AgentConfigurationController) {
	rr.HandleFuncWithValidationAndAuthz(
		"POST /agents/{agentName}/model-configs",
		gwperm.AgentUpdate, inProject(rr.Track("amp.agent-development.bind-model-config", actionDims("created-model-config-binding"), ctrl.CreateAgentModelConfig)),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"GET /agents/{agentName}/model-configs",
		gwperm.AgentRead, inProject(ctrl.ListAgentModelConfigs),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"GET /agents/{agentName}/model-configs/{configId}",
		gwperm.AgentRead, inProject(ctrl.GetAgentModelConfig),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"PUT /agents/{agentName}/model-configs/{configId}",
		gwperm.AgentUpdate, inProject(rr.Track("amp.agent-development.bind-model-config", actionDims("updated-model-config-binding"), ctrl.UpdateAgentModelConfig)),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"DELETE /agents/{agentName}/model-configs/{configId}",
		gwperm.AgentDelete, inProject(rr.Track("amp.agent-development.bind-model-config", actionDims("deleted-model-config-binding"), ctrl.DeleteAgentModelConfig)),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"POST /agents/{agentName}/mcp-configs",
		gwperm.AgentUpdate, inProject(rr.Track("amp.agent-development.bind-mcp-config", actionDims("created-mcp-config-binding"), ctrl.CreateAgentMCPConfig)),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"GET /agents/{agentName}/mcp-configs",
		gwperm.AgentRead, inProject(ctrl.ListAgentMCPConfigs),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"GET /agents/{agentName}/mcp-configs/{configId}",
		gwperm.AgentRead, inProject(ctrl.GetAgentMCPConfig),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"PUT /agents/{agentName}/mcp-configs/{configId}",
		gwperm.AgentUpdate, inProject(rr.Track("amp.agent-development.bind-mcp-config", actionDims("updated-mcp-config-binding"), ctrl.UpdateAgentMCPConfig)),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"DELETE /agents/{agentName}/mcp-configs/{configId}",
		gwperm.AgentDelete, inProject(rr.Track("amp.agent-development.bind-mcp-config", actionDims("deleted-mcp-config-binding"), ctrl.DeleteAgentMCPConfig)),
	)

	// Per-config MCP API keys (external agents): the key an agent uses to call its
	// MCP server through the gateway, scoped to a configuration.
	rr.HandleFuncWithValidationAndAuthz(
		"GET /agents/{agentName}/mcp-configs/{configId}/api-keys",
		gwperm.AgentRead, inProject(ctrl.ListMCPConfigAPIKeys),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"POST /agents/{agentName}/mcp-configs/{configId}/api-keys",
		gwperm.AgentAPIKeyManage, inProject(rr.Track("amp.security-access.config-scoped-api-key", configScopedAPIKeyDims("mcp-config", "issued-config-scoped-api-key"), ctrl.CreateMCPConfigAPIKey)),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"PUT /agents/{agentName}/mcp-configs/{configId}/api-keys/{keyName}",
		gwperm.AgentAPIKeyManage, inProject(rr.Track("amp.security-access.config-scoped-api-key", configScopedAPIKeyDims("mcp-config", "rotated-config-scoped-api-key"), ctrl.RotateMCPConfigAPIKey)),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"DELETE /agents/{agentName}/mcp-configs/{configId}/api-keys/{keyName}",
		gwperm.AgentAPIKeyManage, inProject(rr.Track("amp.security-access.config-scoped-api-key", configScopedAPIKeyDims("mcp-config", "revoked-config-scoped-api-key"), ctrl.RevokeMCPConfigAPIKey)),
	)

	// Per-config LLM API keys (external agents): the key an agent uses to call its
	// LLM provider through the gateway, scoped to a configuration.
	rr.HandleFuncWithValidationAndAuthz(
		"GET /agents/{agentName}/model-configs/{configId}/api-keys",
		gwperm.AgentRead, inProject(ctrl.ListLLMConfigAPIKeys),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"POST /agents/{agentName}/model-configs/{configId}/api-keys",
		gwperm.AgentAPIKeyManage, inProject(rr.Track("amp.security-access.config-scoped-api-key", configScopedAPIKeyDims("model-config", "issued-config-scoped-api-key"), ctrl.CreateLLMConfigAPIKey)),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"PUT /agents/{agentName}/model-configs/{configId}/api-keys/{keyName}",
		gwperm.AgentAPIKeyManage, inProject(rr.Track("amp.security-access.config-scoped-api-key", configScopedAPIKeyDims("model-config", "rotated-config-scoped-api-key"), ctrl.RotateLLMConfigAPIKey)),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"DELETE /agents/{agentName}/model-configs/{configId}/api-keys/{keyName}",
		gwperm.AgentAPIKeyManage, inProject(rr.Track("amp.security-access.config-scoped-api-key", configScopedAPIKeyDims("model-config", "revoked-config-scoped-api-key"), ctrl.RevokeLLMConfigAPIKey)),
	)
}

// configScopedAPIKeyDims builds the growth-analytics dimensions for
// "amp.security-access.config-scoped-api-key", tagging which kind of binding
// the key is scoped to and which CRUD action this route performs on it.
func configScopedAPIKeyDims(configType, action string) map[string]any {
	return map[string]any{"config_type": configType, "action": action}
}

// actionDims builds a growth-analytics dimensions map tagging which action a
// route performs, for any taxonomy feature whose declared dimensions are just
// an "action" enum (e.g. bind-model-config/bind-mcp-config's create/update/
// delete, deploy-llm-provider's deploy/undeploy/restore, llm-provider-api-key's
// create/rotate/revoke).
func actionDims(action string) map[string]any {
	return map[string]any{"action": action}
}
