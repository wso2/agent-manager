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

// Package gwapi registers the AI Gateway's routes on the core's routers.
package gwapi

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/mcp/tools"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway/gwmcp"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway/gwwire"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// RegisterRoutes registers the gateway's routes on the authenticated /api/v1
// router.
func RegisterRoutes(rr *middleware.RouteRegistrar, gw *gwwire.Gateway, core *wiring.AppParams) {
	RegisterGatewayRoutes(rr, gw.GatewayController)
	RegisterPlatformGatewayHealthRoutes(rr, gw.GatewayController)
	registerAgentGatewayRoutes(rr, gw.AgentGatewayController, gw.EnvironmentGatewayController)
	registerCatalogRoutes(rr, gw.CatalogController)
	RegisterLLMRoutes(rr, gw.LLMController)
	RegisterLLMDeploymentRoutes(rr, gw.LLMDeploymentController)
	RegisterLLMProviderAPIKeyRoutes(rr, gw.LLMProviderAPIKeyController)
	RegisterLLMProxyAPIKeyRoutes(rr, gw.LLMProxyAPIKeyController)
	RegisterAgentAPIKeyRoutes(rr, gw.AgentAPIKeyController)
	registerA2AAgentCardRoutes(rr, gw.A2AAgentCardController)
	registerA2AProxyRoutes(rr, gw.A2AProxyController)
	RegisterLLMProxyDeploymentRoutes(rr, gw.LLMProxyDeploymentController)
	RegisterMCPProxyRoutes(rr, gw.MCPProxyController)
	RegisterProjectMCPProxyRoutes(rr, middleware.ApplyOrgOverride(core.OUHandleLookup))
	RegisterAgentConfigRoutes(rr, gw.AgentConfigurationController)
	registerMCPProxyScopeRoutes(rr, gw.MCPProxyScopeController)
	registerAgentIdentityRoutes(rr, gw.AgentIdentityController)
}

// RegisterInternalRoutes registers the gateway-facing internal routes: the
// gateway internal API and the gateway WebSocket.
func RegisterInternalRoutes(rr *middleware.RouteRegistrar, mux *http.ServeMux, gw *gwwire.Gateway) {
	RegisterGatewayInternalRoutes(rr, gw.GatewayInternalController)
	RegisterWebSocketRoutes(mux, gw.WebSocketController)
}

// MCPTools adds the gateway's toolsets to the core's MCP server: LLM
// providers, MCP proxies, agent configurations and agent deployments.
func MCPTools(gw *gwwire.Gateway) func(*tools.Toolsets) {
	return func(ts *tools.Toolsets) {
		ts.DeploymentToolset = gwmcp.NewDeploymentHandler(gw.AgentGateway)
		ts.LLMProviderToolset = gwmcp.NewLLMProviderHandler(gw.LLMProviderService, gw.LLMProviderDeploymentService, gw.PlatformGatewayService)
		ts.MCPProxyToolset = gwmcp.NewMCPProxyHandler(gw.MCPProxyService)
		ts.AgentConfigToolset = gwmcp.NewAgentConfigHandler(gw.AgentConfigurationService)
	}
}

// notImplemented answers 501 for a route this version does not serve.
func notImplemented(w http.ResponseWriter, _ *http.Request) {
	utils.WriteErrorResponse(w, http.StatusNotImplemented, "This operation is not available in this version")
}
