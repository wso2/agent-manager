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

func registerMCPProxyScopeRoutes(rr *middleware.RouteRegistrar, ctrl controllers.MCPProxyScopeController) {
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/mcp-proxies/{proxyId}/scopes", rbac.ScopeRead, ctrl.ListMCPProxyScopes)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/mcp-proxies/{proxyId}/scopes", rbac.ScopeCreate,
		growthanalytics.Track("amp.connections.manage-mcp-scope", actionDims("created-mcp-scope"), ctrl.CreateMCPProxyScope))
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/mcp-proxies/{proxyId}/scopes/{scopeAction}", rbac.ScopeUpdate,
		growthanalytics.Track("amp.connections.manage-mcp-scope", actionDims("updated-mcp-scope"), ctrl.UpdateMCPProxyScope))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/mcp-proxies/{proxyId}/scopes/{scopeAction}", rbac.ScopeDelete,
		growthanalytics.Track("amp.connections.manage-mcp-scope", actionDims("deleted-mcp-scope"), ctrl.DeleteMCPProxyScope))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/environments/{envName}/agent-identities/scopes", rbac.AgentIdentityRead, ctrl.ListAgentIdentityScopes)
}
