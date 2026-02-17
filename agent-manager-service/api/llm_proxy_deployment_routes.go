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

// RegisterLLMProxyDeploymentRoutes registers all LLM proxy deployment-related routes
func RegisterLLMProxyDeploymentRoutes(mux *http.ServeMux, ctrl controllers.LLMProxyDeploymentController) {
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments", ctrl.DeployLLMProxy)
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments/undeploy", ctrl.UndeployLLMProxyDeployment)
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments/restore", ctrl.RestoreLLMProxyDeployment)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments", ctrl.GetLLMProxyDeployments)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments/{deploymentId}", ctrl.GetLLMProxyDeployment)
	middleware.HandleFuncWithValidation(mux, "DELETE /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments/{deploymentId}", ctrl.DeleteLLMProxyDeployment)
}
