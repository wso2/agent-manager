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

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// RegisterProjectMCPProxyRoutes registers the project-scoped MCP proxy routes.
//
// Every route is registered through RouteRegistrar, which builds the one middleware chain
// (see RouteRegistrar.register). These routes additionally compose orgOverride
// (middleware.ApplyOrgOverride) between org resolution and the permission check, because
// the Integration Platform drives them for a tenant with a service token from the platform's
// root OU plus X-Impersonate-Org. No other route takes the override.
func RegisterProjectMCPProxyRoutes(
	rr *middleware.RouteRegistrar,
	orgOverride func(http.HandlerFunc) http.HandlerFunc,
) {
	const base = "/orgs/{orgName}/projects/{projName}/mcp-proxies"

	// PUT is gated on update; the controller additionally requires MCPServerCreate when the
	// upsert would insert.
	rr.HandleFuncWithValidationAndAuthzAndOrgOverride("PUT "+base+"/{proxyId}", rbac.MCPServerUpdate, orgOverride, notImplemented)
	rr.HandleFuncWithValidationAndAuthzAndOrgOverride("GET "+base, rbac.MCPServerRead, orgOverride, notImplemented)
	rr.HandleFuncWithValidationAndAuthzAndOrgOverride("GET "+base+"/{proxyId}", rbac.MCPServerRead, orgOverride, notImplemented)
	rr.HandleFuncWithValidationAndAuthzAndOrgOverride("DELETE "+base+"/{proxyId}", rbac.MCPServerDelete, orgOverride, notImplemented)

	// Environment bindings: binding and unbinding change the proxy, so both are gated on update.
	const binding = base + "/{proxyId}/environments/{envName}"
	rr.HandleFuncWithValidationAndAuthzAndOrgOverride("PUT "+binding, rbac.MCPServerUpdate, orgOverride, notImplemented)
	rr.HandleFuncWithValidationAndAuthzAndOrgOverride("GET "+binding, rbac.MCPServerRead, orgOverride, notImplemented)
	rr.HandleFuncWithValidationAndAuthzAndOrgOverride("DELETE "+binding, rbac.MCPServerUpdate, orgOverride, notImplemented)
}
