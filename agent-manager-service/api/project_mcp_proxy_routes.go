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

// RegisterProjectMCPProxyRoutes registers the project-scoped MCP proxy routes.
//
// Every route is registered through RouteRegistrar, which builds the one middleware chain
// (path-param validation, permission, org match, audit; see RouteRegistrar.register).
func RegisterProjectMCPProxyRoutes(rr *middleware.RouteRegistrar, ctrl controllers.ProjectMCPProxyController) {
	const base = "/orgs/{orgName}/projects/{projName}/mcp-proxies"

	// PUT is gated on update; the controller additionally requires MCPServerCreate when the
	// upsert would insert.
	rr.HandleFuncWithValidationAndAuthz("PUT "+base+"/{proxyId}", rbac.MCPServerUpdate, ctrl.UpsertProjectMCPProxy)
	rr.HandleFuncWithValidationAndAuthz("GET "+base, rbac.MCPServerRead, ctrl.ListProjectMCPProxies)
	rr.HandleFuncWithValidationAndAuthz("GET "+base+"/{proxyId}", rbac.MCPServerRead, ctrl.GetProjectMCPProxy)
	rr.HandleFuncWithValidationAndAuthz("DELETE "+base+"/{proxyId}", rbac.MCPServerDelete, ctrl.DeleteProjectMCPProxy)
}
