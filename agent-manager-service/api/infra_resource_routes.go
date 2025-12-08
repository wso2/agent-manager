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

func registerInfraRoutes(mux *http.ServeMux, ctrl controllers.InfraResourceController) {
	// All routes now use HandleFuncWithValidation which automatically
	// extracts path parameters from the pattern and validates them
	middleware.HandleFuncWithValidation(mux, "POST /orgs", ctrl.CreateOrganization)
	middleware.HandleFuncWithValidation(mux, "GET /orgs", ctrl.ListOrganizations)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}", ctrl.GetOrganization)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects", ctrl.ListProjects)
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/projects", ctrl.CreateProject)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects/{projName}", ctrl.GetProject)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/environments", ctrl.GetOrgEnvironments)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/projects/{projName}/deployment-pipelines", ctrl.GetProjectDeploymentPipeline)
	middleware.HandleFuncWithValidation(mux, "DELETE /orgs/{orgName}/projects/{projName}", ctrl.DeleteProject)
}
