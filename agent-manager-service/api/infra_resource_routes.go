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

func registerInfraRoutes(rr *middleware.RouteRegistrar, ctrl controllers.InfraResourceController) {
	rr.HandleFuncWithValidationAndAuthz("GET /orgs", rbac.OrgView, ctrl.ListOrganizations)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}", rbac.OrgView, ctrl.GetOrganization)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/data-planes", rbac.DataPlaneRead, ctrl.GetDataplanes)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/deployment-pipelines", rbac.DeploymentPipelineRead, ctrl.ListOrgDeploymentPipelines)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/deployment-pipelines", rbac.DeploymentPipelineCreate,
		growthanalytics.Track("amp.deployment-ops.deployment-pipeline", actionDims("created-deployment-pipeline"), ctrl.CreateOrgDeploymentPipeline))
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/deployment-pipelines/{pipelineName}", rbac.DeploymentPipelineUpdate,
		growthanalytics.Track("amp.deployment-ops.deployment-pipeline", actionDims("updated-deployment-pipeline"), ctrl.UpdateOrgDeploymentPipeline))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/deployment-pipelines/{pipelineName}", rbac.DeploymentPipelineDelete,
		growthanalytics.Track("amp.deployment-ops.deployment-pipeline", actionDims("deleted-deployment-pipeline"), ctrl.DeleteOrgDeploymentPipeline))

	// NOTE: /orgs/{orgName}/environments routes are now registered in environment_routes.go
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects", rbac.ProjectRead, ctrl.ListProjects)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects", rbac.ProjectCreate,
		growthanalytics.Track("amp.deployment-ops.create-project", actionDims("created-project"), ctrl.CreateProject))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}", rbac.ProjectRead, ctrl.GetProject)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}", rbac.ProjectUpdate,
		growthanalytics.Track("amp.deployment-ops.create-project", actionDims("updated-project"), ctrl.UpdateProject))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/deployment-pipeline", rbac.DeploymentPipelineRead, ctrl.GetProjectDeploymentPipeline)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/projects/{projName}", rbac.ProjectDelete,
		growthanalytics.Track("amp.deployment-ops.create-project", actionDims("deleted-project"), ctrl.DeleteProject))
}
