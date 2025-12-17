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
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware"
)

func registerAgentRoutes(mux *http.ServeMux, ctrl controllers.AgentController) {
	// All routes now use HandleFuncWithValidation which automatically
	// extracts path parameters from the pattern and validates them

	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/projects/{projName}/agents", ctrl.CreateAgent)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects/{projName}/agents", ctrl.ListAgents)
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/utils/generate-name", ctrl.GenerateName)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}", ctrl.GetAgent)
	middleware.HandleFuncWithValidation(mux, "DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}", ctrl.DeleteAgent)
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds", ctrl.BuildAgent)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds", ctrl.ListAgentBuilds)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds/{buildName}", ctrl.GetBuild)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds/{buildName}/build-logs", ctrl.GetBuildLogs)
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments", ctrl.DeployAgent)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments", ctrl.GetAgentDeployments)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/endpoints", ctrl.GetAgentEndpoints)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/configurations", ctrl.GetAgentConfigurations)
}
