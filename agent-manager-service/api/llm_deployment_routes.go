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
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware"
)

// RegisterLLMDeploymentRoutes registers all LLM deployment-related routes
func RegisterLLMDeploymentRoutes(mux *http.ServeMux, ctrl controllers.LLMDeploymentController) {
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/llm-providers/{id}/deployments", ctrl.DeployLLMProvider)
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/llm-providers/{id}/deployments/undeploy", ctrl.UndeployLLMProviderDeployment)
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/llm-providers/{id}/deployments/restore", ctrl.RestoreLLMProviderDeployment)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/llm-providers/{id}/deployments", ctrl.GetLLMProviderDeployments)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/llm-providers/{id}/deployments/{deploymentId}", ctrl.GetLLMProviderDeployment)
	middleware.HandleFuncWithValidation(mux, "DELETE /orgs/{orgName}/llm-providers/{id}/deployments/{deploymentId}", ctrl.DeleteLLMProviderDeployment)
}
