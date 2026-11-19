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
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// registerAgentGatewayRoutes registers the agent and environment routes that
// only exist with a gateway.
func registerAgentGatewayRoutes(rr Router, ctrl *gwctl.AgentGatewayController, envCtrl *gwctl.EnvironmentGatewayController) {
	// Both of these rewrite a running deployment's env vars, secret references
	// and file mounts, so they reach an environment the same way a deploy does
	// and carry the same two axes: agent:update is the capability, and the tier
	// is where it lands. The environment comes from the request body, so
	// requireEnvTier in the service layer is what decides production; declaring
	// the floor here is the early deny and the floor half of that check.
	rr.HandleFuncWithValidationAndAllAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/deploy-settings",
		rr.Track("amp.agent-development.update-agent", updateTargetDims("deploy-settings"), ctrl.UpdateAgentDeploySettings),
		rbac.AgentUpdate, gwperm.DeployNonProduction)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments", rbac.AgentRead, ctrl.GetAgentDeployments)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/configurations", rbac.AgentRead, ctrl.GetAgentConfigurations)
	rr.HandleFuncWithValidationAndAnyAuthz("GET /orgs/{orgName}/environments/{envID}/gateways", envCtrl.GetEnvironmentGateways,
		rbac.EnvironmentRead, gwperm.Read)
}

// updateTargetDims builds the growth-analytics dimensions for the shared
// "amp.agent-development.update-agent" feature, tagging which part of the
// agent's configuration this specific route updates.
func updateTargetDims(target string) map[string]any {
	return map[string]any{"update_target": target}
}
