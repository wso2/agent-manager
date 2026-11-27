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

// Package gwapi registers the AI Gateway's routes: its public API at
// /m/ai-gateway and the controller-facing routes below /api/internal/v1.
package gwapi

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwwire"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// RegisterRoutes registers the gateway's public routes.
func RegisterRoutes(rr Router, gw *gwwire.Gateway) {
	RegisterGatewayRoutes(rr, gw.GatewayController)
	RegisterPlatformGatewayHealthRoutes(rr, gw.GatewayController)
	registerAgentGatewayRoutes(rr, gw.AgentGatewayController)
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
	RegisterAgentConfigRoutes(rr, gw.AgentConfigurationController)
	registerMCPProxyScopeRoutes(rr, gw.MCPProxyScopeController)
	registerAgentIdentityRoutes(rr, gw.AgentIdentityController)
}

// RegisterInternalRoutes registers the gateway-facing internal routes: the
// gateway internal API and the gateway WebSocket.
func RegisterInternalRoutes(rr InternalRouter, gw *gwwire.Gateway) {
	RegisterGatewayInternalRoutes(rr, gw.GatewayInternalController)
	RegisterWebSocketRoutes(rr, gw.WebSocketController)
}

// notImplemented answers 501 for a route this version does not serve.
func notImplemented(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteErrorResponse(w, http.StatusNotImplemented, "This operation is not available in this version")
}
