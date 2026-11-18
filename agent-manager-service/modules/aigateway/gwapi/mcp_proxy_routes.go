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

package gwapi

import (
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

// RegisterMCPProxyRoutes registers MCP proxy routes.
func RegisterMCPProxyRoutes(rr Router, ctrl gwctl.MCPProxyController) {
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/mcp-proxies/fetch-server-info", gwperm.MCPServerConnect,
		growthanalytics.Track("amp.connections.create-mcp-proxy", actionDims("probed-mcp-server"), ctrl.FetchServerInfo))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/mcp-proxies/policies", gwperm.MCPServerRead, ctrl.ListAvailableMCPPolicies)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/mcp-proxies", gwperm.MCPServerCreate,
		growthanalytics.Track("amp.connections.create-mcp-proxy", actionDims("created-mcp-proxy"), ctrl.CreateMCPProxy))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/mcp-proxies", gwperm.MCPServerRead, ctrl.ListMCPProxies)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/mcp-proxies/{proxyId}", gwperm.MCPServerRead, ctrl.GetMCPProxy)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/mcp-proxies/{proxyId}", gwperm.MCPServerUpdate,
		growthanalytics.Track("amp.connections.create-mcp-proxy", actionDims("updated-mcp-proxy"), ctrl.UpdateMCPProxy))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/mcp-proxies/{proxyId}", gwperm.MCPServerDelete,
		growthanalytics.Track("amp.connections.create-mcp-proxy", actionDims("deleted-mcp-proxy"), ctrl.DeleteMCPProxy))
}
