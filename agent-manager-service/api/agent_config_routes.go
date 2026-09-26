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

package api

import (
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// RegisterAgentConfigRoutes registers all agent configuration routes
func RegisterAgentConfigRoutes(rr *middleware.RouteRegistrar, ctrl controllers.AgentConfigurationController) {
	rr.HandleFuncWithValidationAndAuthz(
		"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs",
		rbac.AgentUpdate, growthanalytics.Track("amp.agent-development.bind-model-config", actionDims("created-model-config-binding"), ctrl.CreateAgentModelConfig),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs",
		rbac.AgentRead, ctrl.ListAgentModelConfigs,
	)

	rr.HandleFuncWithValidationAndAuthz(
		"GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}",
		rbac.AgentRead, ctrl.GetAgentModelConfig,
	)

	rr.HandleFuncWithValidationAndAuthz(
		"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}",
		rbac.AgentUpdate, growthanalytics.Track("amp.agent-development.bind-model-config", actionDims("updated-model-config-binding"), ctrl.UpdateAgentModelConfig),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}",
		rbac.AgentDelete, growthanalytics.Track("amp.agent-development.bind-model-config", actionDims("deleted-model-config-binding"), ctrl.DeleteAgentModelConfig),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs",
		rbac.AgentUpdate, growthanalytics.Track("amp.agent-development.bind-mcp-config", actionDims("created-mcp-config-binding"), ctrl.CreateAgentMCPConfig),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs",
		rbac.AgentRead, ctrl.ListAgentMCPConfigs,
	)

	rr.HandleFuncWithValidationAndAuthz(
		"GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}",
		rbac.AgentRead, ctrl.GetAgentMCPConfig,
	)

	rr.HandleFuncWithValidationAndAuthz(
		"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}",
		rbac.AgentUpdate, growthanalytics.Track("amp.agent-development.bind-mcp-config", actionDims("updated-mcp-config-binding"), ctrl.UpdateAgentMCPConfig),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}",
		rbac.AgentDelete, growthanalytics.Track("amp.agent-development.bind-mcp-config", actionDims("deleted-mcp-config-binding"), ctrl.DeleteAgentMCPConfig),
	)

	// Per-config MCP API keys (external agents): the key an agent uses to call its
	// MCP server through the gateway, scoped to a configuration + environment.
	rr.HandleFuncWithValidationAndAuthz(
		"GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}/environments/{envName}/api-keys",
		rbac.AgentRead, ctrl.ListMCPConfigAPIKeys,
	)

	rr.HandleFuncWithValidationAndAuthz(
		"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}/environments/{envName}/api-keys",
		rbac.AgentAPIKeyManage, growthanalytics.Track("amp.security-access.config-scoped-api-key", configScopedAPIKeyDims("mcp-config", "issued-config-scoped-api-key"), ctrl.CreateMCPConfigAPIKey),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}/environments/{envName}/api-keys/{keyName}",
		rbac.AgentAPIKeyManage, growthanalytics.Track("amp.security-access.config-scoped-api-key", configScopedAPIKeyDims("mcp-config", "rotated-config-scoped-api-key"), ctrl.RotateMCPConfigAPIKey),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}/environments/{envName}/api-keys/{keyName}",
		rbac.AgentAPIKeyManage, growthanalytics.Track("amp.security-access.config-scoped-api-key", configScopedAPIKeyDims("mcp-config", "revoked-config-scoped-api-key"), ctrl.RevokeMCPConfigAPIKey),
	)

	// Per-config LLM API keys (external agents): the key an agent uses to call its
	// LLM provider through the gateway, scoped to a configuration + environment.
	rr.HandleFuncWithValidationAndAuthz(
		"GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}/environments/{envName}/api-keys",
		rbac.AgentRead, ctrl.ListLLMConfigAPIKeys,
	)

	rr.HandleFuncWithValidationAndAuthz(
		"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}/environments/{envName}/api-keys",
		rbac.AgentAPIKeyManage, growthanalytics.Track("amp.security-access.config-scoped-api-key", configScopedAPIKeyDims("model-config", "issued-config-scoped-api-key"), ctrl.CreateLLMConfigAPIKey),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}/environments/{envName}/api-keys/{keyName}",
		rbac.AgentAPIKeyManage, growthanalytics.Track("amp.security-access.config-scoped-api-key", configScopedAPIKeyDims("model-config", "rotated-config-scoped-api-key"), ctrl.RotateLLMConfigAPIKey),
	)

	rr.HandleFuncWithValidationAndAuthz(
		"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}/environments/{envName}/api-keys/{keyName}",
		rbac.AgentAPIKeyManage, growthanalytics.Track("amp.security-access.config-scoped-api-key", configScopedAPIKeyDims("model-config", "revoked-config-scoped-api-key"), ctrl.RevokeLLMConfigAPIKey),
	)
}

// configScopedAPIKeyDims builds the growth-analytics dimensions for
// "amp.security-access.config-scoped-api-key", tagging which kind of binding
// the key is scoped to and which CRUD action this route performs on it.
func configScopedAPIKeyDims(configType, action string) map[string]interface{} {
	return map[string]interface{}{"config_type": configType, "action": action}
}

// actionDims builds a growth-analytics dimensions map tagging which action a
// route performs, for any taxonomy feature whose declared dimensions are just
// an "action" enum (e.g. bind-model-config/bind-mcp-config's create/update/
// delete, deploy-llm-provider's deploy/undeploy/restore, llm-provider-api-key's
// create/rotate/revoke).
func actionDims(action string) map[string]interface{} {
	return map[string]interface{}{"action": action}
}
