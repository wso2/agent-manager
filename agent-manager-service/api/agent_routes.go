// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

func registerAgentRoutes(rr *middleware.RouteRegistrar, ctrl controllers.AgentController) {
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents", rbac.AgentCreate,
		growthanalytics.Track("amp.agent-development.create-agent", actionDims("created-agent"), ctrl.CreateAgent))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents", rbac.AgentRead, ctrl.ListAgents)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/agents", rbac.AgentRead, ctrl.ListOrgAgents)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/utils/generate-name", rbac.AgentCreate, ctrl.GenerateName)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}", rbac.AgentRead, ctrl.GetAgent)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}", rbac.AgentUpdate,
		growthanalytics.Track("amp.agent-development.update-agent", updateTargetDims("basic-info"), ctrl.UpdateAgentBasicInfo))
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/build-parameters", rbac.AgentUpdate,
		growthanalytics.Track("amp.agent-development.update-agent", updateTargetDims("build-parameters"), ctrl.UpdateAgentBuildParameters))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/resource-configs", rbac.AgentRead, ctrl.GetAgentResourceConfigs)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/resource-configs", rbac.AgentUpdate,
		growthanalytics.Track("amp.agent-development.update-agent", updateTargetDims("resource-configs"), ctrl.UpdateAgentResourceConfigs))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}", rbac.AgentDelete,
		growthanalytics.Track("amp.agent-development.delete-agent", actionDims("deleted-agent"), ctrl.DeleteAgent))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds", rbac.AgentBuild,
		growthanalytics.Track("amp.agent-development.build-agent", map[string]interface{}{
			"action":  "built-agent",
			"outcome": growthanalytics.DynamicOutcome,
		}, ctrl.BuildAgent))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds", rbac.AgentRead, ctrl.ListAgentBuilds)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds/{buildName}", rbac.AgentRead, ctrl.GetBuild)
	// Deploy and promote declare only the tier floor, and have no capability
	// scope of their own: within an environment the tier scope is the whole
	// grant. The environment that decides the real answer is not known here —
	// deploy derives it from the pipeline, promote takes it from the body — so
	// requireEnvTier in the service layer is the guarantee, and this declaration
	// is a cheap early deny plus the floor half of a production check.
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments", rbac.AgentEnvNonProduction,
		growthanalytics.Track("amp.deployment-ops.deploy-agent", actionDims("deployed-agent"), ctrl.DeployAgent))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/promote", rbac.AgentEnvNonProduction,
		growthanalytics.Track("amp.deployment-ops.deploy-agent.promote", actionDims("promoted-agent-deployment"), ctrl.PromoteAgent))
	// Both of these rewrite a running deployment's env vars, secret references
	// and file mounts, so they reach an environment the same way a deploy does
	// and carry the same two axes: agent:update is the capability, and the tier
	// is where it lands. The environment comes from the request body, so
	// requireEnvTier in the service layer is what decides production; declaring
	// the floor here is the early deny and the floor half of that check.
	rr.HandleFuncWithValidationAndAllAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/deploy-settings",
		growthanalytics.Track("amp.agent-development.update-agent", updateTargetDims("deploy-settings"), ctrl.UpdateAgentDeploySettings),
		rbac.AgentUpdate, rbac.AgentEnvNonProduction)
	rr.HandleFuncWithValidationAndAllAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/configurations",
		growthanalytics.Track("amp.agent-development.update-agent", updateTargetDims("configurations"), ctrl.UpdateAgentConfigurations),
		rbac.AgentUpdate, rbac.AgentEnvNonProduction)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/tracing-token/regenerate", rbac.AgentTokenManage,
		growthanalytics.Track("amp.security-access.agent-token", tokenTypeDims("tracing-token", "regenerated-tracing-token"), ctrl.RegenerateTracingToken))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments", rbac.AgentRead, ctrl.GetAgentDeployments)
	// Two independent axes: the capability to change deployment state, and the
	// tier of the environment it is changed in.
	rr.HandleFuncWithValidationAndAllAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments/state", ctrl.UpdateDeploymentState, rbac.AgentSuspend, rbac.AgentEnvNonProduction)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/endpoints", rbac.AgentRead, ctrl.GetAgentEndpoints)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/configurations", rbac.AgentRead, ctrl.GetAgentConfigurations)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/publish-kind", rbac.AgentKindCreate,
		growthanalytics.Track("amp.agent-development.publish-kind", actionDims("published-agent-as-kind"), ctrl.PublishKind))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities", rbac.AgentUpdate, ctrl.GetAgentIdentity)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities", rbac.AgentUpdate,
		growthanalytics.Track("amp.security-access.agent-identity", actionDims("provisioned-agent-identity"), ctrl.ProvisionAgentIdentity))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities", rbac.AgentUpdate,
		growthanalytics.Track("amp.security-access.agent-identity", actionDims("regenerated-agent-identity-secret"), ctrl.RegenerateAgentIdentitySecret))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities", rbac.AgentUpdate,
		growthanalytics.Track("amp.security-access.agent-identity", actionDims("revoked-agent-identity-secret"), ctrl.RevokeAgentIdentitySecret))
	// RetryAgentIdentityProvisioning didn't exist yet when the growth-analytics
	// taxonomy for this feature was defined upstream, so it's intentionally left
	// untracked here rather than guessing a new action value for it.
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities/retry", rbac.AgentUpdate, ctrl.RetryAgentIdentityProvisioning)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/roles", rbac.AgentUpdate, ctrl.GetAgentRoles)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/groups", rbac.AgentUpdate, ctrl.GetAgentGroups)
}

// updateTargetDims builds the growth-analytics dimensions for the shared
// "amp.agent-development.update-agent" feature, tagging which part of the
// agent's configuration this specific route updates (the taxonomy's
// update_target dimension).
func updateTargetDims(target string) map[string]interface{} {
	return map[string]interface{}{"update_target": target}
}
