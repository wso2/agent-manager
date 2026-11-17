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

package legacyapi

import (
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// registerHostedRoutes registers the routes of hosted agents (builds, deploy
// and promote, deployment pipelines, data planes), which this version does
// not serve: every one answers 501.
func registerHostedRoutes(rr *middleware.RouteRegistrar) {
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/build-parameters", rbac.AgentUpdate,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/resource-configs", rbac.AgentRead, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/resource-configs", rbac.AgentUpdate,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds", rbac.AgentBuild,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds", rbac.AgentRead, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/builds/{buildName}", rbac.AgentRead, notImplemented)
	// Deploy and promote declare only the tier floor, and have no capability
	// scope of their own: within an environment the tier scope is the whole
	// grant. The environment that decides the real answer is not known here —
	// deploy derives it from the pipeline, promote takes it from the body — so
	// requireEnvTier in the service layer is the guarantee, and this declaration
	// is a cheap early deny plus the floor half of a production check.
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments", rbac.AgentEnvNonProduction,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/promote", rbac.AgentEnvNonProduction,
		notImplemented)
	rr.HandleFuncWithValidationAndAllAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/configurations",
		notImplemented,
		rbac.AgentUpdate, rbac.AgentEnvNonProduction)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/tracing-token/regenerate", rbac.AgentTokenManage,
		notImplemented)
	// Two independent axes: the capability to change deployment state, and the
	// tier of the environment it is changed in.
	rr.HandleFuncWithValidationAndAllAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments/state", notImplemented, rbac.AgentSuspend, rbac.AgentEnvNonProduction)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/endpoints", rbac.AgentRead, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/publish-kind", rbac.AgentKindCreate,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/data-planes", rbac.DataPlaneRead, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/deployment-pipelines", rbac.DeploymentPipelineRead, notImplemented)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/deployment-pipelines", rbac.DeploymentPipelineCreate,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/deployment-pipelines/{pipelineName}", rbac.DeploymentPipelineUpdate,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/deployment-pipelines/{pipelineName}", rbac.DeploymentPipelineDelete,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/deployment-pipeline", rbac.DeploymentPipelineRead, notImplemented)
}
