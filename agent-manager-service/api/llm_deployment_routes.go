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

// RegisterLLMDeploymentRoutes registers all LLM deployment-related routes
func RegisterLLMDeploymentRoutes(rr *middleware.RouteRegistrar, ctrl controllers.LLMDeploymentController) {
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/llm-providers/{providerId}/deployments", rbac.LLMProviderDeploy,
		growthanalytics.Track("amp.connections.deploy-llm-provider", actionDims("deployed-llm-provider"), ctrl.DeployLLMProvider))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/llm-providers/{providerId}/deployments/undeploy", rbac.LLMProviderDeploy,
		growthanalytics.Track("amp.connections.deploy-llm-provider", actionDims("undeployed-llm-provider"), ctrl.UndeployLLMProviderDeployment))
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/llm-providers/{providerId}/deployments/restore", rbac.LLMProviderDeploy,
		growthanalytics.Track("amp.connections.deploy-llm-provider", actionDims("restored-llm-provider-deployment"), ctrl.RestoreLLMProviderDeployment))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-providers/{providerId}/deployments", rbac.LLMProviderRead, ctrl.GetLLMProviderDeployments)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-providers/{providerId}/deployments/{deploymentId}", rbac.LLMProviderRead, ctrl.GetLLMProviderDeployment)
	// DeleteLLMProviderDeployment removes a deployment record outright; the
	// taxonomy's action enum has no "delete" value for this feature, so it's
	// reported as "undeployed-llm-provider" — the closest existing
	// lifecycle-terminal value.
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/llm-providers/{providerId}/deployments/{deploymentId}", rbac.LLMProviderDeploy,
		growthanalytics.Track("amp.connections.deploy-llm-provider", actionDims("undeployed-llm-provider"), ctrl.DeleteLLMProviderDeployment))
}
