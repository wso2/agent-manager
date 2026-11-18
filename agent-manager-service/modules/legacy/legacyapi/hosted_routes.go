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
	// The environment-tier permissions belong to the AI Gateway module; these
	// stubs answer 501 and need only the agent permission.
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments", rbac.AgentUpdate,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/promote", rbac.AgentUpdate,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/configurations", rbac.AgentUpdate,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/tracing-token/regenerate", rbac.AgentTokenManage,
		notImplemented)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments/state", rbac.AgentSuspend,
		notImplemented)
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
