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
	rr.HandleFuncWithValidationAndAuthz("POST /agents", rbac.AgentCreate,
		growthanalytics.Track("amp.agent-development.create-agent", actionDims("created-agent"), ctrl.CreateAgent))
	rr.HandleFuncWithValidationAndAuthz("GET /agents", rbac.AgentRead, ctrl.ListAgents)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/utils/generate-name", rbac.AgentCreate, ctrl.GenerateName)
	rr.HandleFuncWithValidationAndAuthz("GET /agents/{agentName}", rbac.AgentRead, ctrl.GetAgent)
	rr.HandleFuncWithValidationAndAuthz("PUT /agents/{agentName}", rbac.AgentUpdate,
		growthanalytics.Track("amp.agent-development.update-agent", updateTargetDims("basic-info"), ctrl.UpdateAgentBasicInfo))
	rr.HandleFuncWithValidationAndAuthz("DELETE /agents/{agentName}", rbac.AgentDelete,
		growthanalytics.Track("amp.agent-development.delete-agent", actionDims("deleted-agent"), ctrl.DeleteAgent))
	rr.HandleFuncWithValidationAndAuthz("GET /agents/{agentName}/identities", rbac.AgentUpdate, ctrl.GetAgentIdentity)
	rr.HandleFuncWithValidationAndAuthz("PUT /agents/{agentName}/identities", rbac.AgentUpdate,
		growthanalytics.Track("amp.security-access.agent-identity", actionDims("provisioned-agent-identity"), ctrl.ProvisionAgentIdentity))
	rr.HandleFuncWithValidationAndAuthz("POST /agents/{agentName}/identities", rbac.AgentUpdate,
		growthanalytics.Track("amp.security-access.agent-identity", actionDims("regenerated-agent-identity-secret"), ctrl.RegenerateAgentIdentitySecret))
	rr.HandleFuncWithValidationAndAuthz("DELETE /agents/{agentName}/identities", rbac.AgentUpdate,
		growthanalytics.Track("amp.security-access.agent-identity", actionDims("revoked-agent-identity-secret"), ctrl.RevokeAgentIdentitySecret))
	// RetryAgentIdentityProvisioning didn't exist yet when the growth-analytics
	// taxonomy for this feature was defined upstream, so it's intentionally left
	// untracked here rather than guessing a new action value for it.
	rr.HandleFuncWithValidationAndAuthz("POST /agents/{agentName}/identities/retry", rbac.AgentUpdate, ctrl.RetryAgentIdentityProvisioning)
	rr.HandleFuncWithValidationAndAuthz("GET /agents/{agentName}/roles", rbac.AgentUpdate, ctrl.GetAgentRoles)
	rr.HandleFuncWithValidationAndAuthz("GET /agents/{agentName}/groups", rbac.AgentUpdate, ctrl.GetAgentGroups)
}

// updateTargetDims builds the growth-analytics dimensions for the shared
// "amp.agent-development.update-agent" feature, tagging which part of the
// agent's configuration this specific route updates (the taxonomy's
// update_target dimension).
func updateTargetDims(target string) map[string]interface{} {
	return map[string]interface{}{"update_target": target}
}
