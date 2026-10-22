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
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

const a2aCatalogAgentBase = "/orgs/{orgName}/projects/{projName}/agents/{agentName}"

// registerA2ACatalogRoutes registers three GETs, none in sensitiveReadPaths, so not audited.
func registerA2ACatalogRoutes(rr *middleware.RouteRegistrar, ctrl controllers.A2ACatalogController) {
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/a2a-agents", rbac.AgentRead, ctrl.ListA2AAgents)
	rr.HandleFuncWithValidationAndAuthz("GET "+a2aCatalogAgentBase+"/a2a-catalog-entry", rbac.AgentRead,
		ctrl.GetA2ACatalogEntry)
	rr.HandleFuncWithValidationAndAuthz("GET "+a2aCatalogAgentBase+"/environments/{envID}/a2a-connect", rbac.AgentRead,
		ctrl.GetA2AConnect)
}
