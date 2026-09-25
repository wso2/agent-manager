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
)

// RegisterGatewayInternalRoutes registers all gateway internal API routes.
//
// These use api-key authentication (checked inside each handler) rather than
// JWT, so the registrar applies no authz here. It is still the registrar and
// not a bare mux, because that is what puts these routes in the audit ledger
// and under the same coverage test as the public API — the bulk-sync endpoints
// below hand real key material to a gateway.
func RegisterGatewayInternalRoutes(rr *middleware.RouteRegistrar, ctrl controllers.GatewayInternalController) {
	// API key bulk-sync endpoints (must be registered before {id} catch-all routes)
	rr.HandleFuncWithValidation("GET /llm-providers/api-keys", ctrl.GetLLMProviderAPIKeys)
	rr.HandleFuncWithValidation("GET /llm-proxies/api-keys", ctrl.GetLLMProxyAPIKeys)
	rr.HandleFuncWithValidation("GET /apis/api-keys", ctrl.GetAPIKeys)

	// Subscription plans endpoint
	rr.HandleFuncWithValidation("GET /subscription-plans", ctrl.GetSubscriptionPlans)

	// Deployment sync endpoint (gateway-controller reconciles its local cache
	// against this on connect and periodically).
	rr.HandleFuncWithValidation("GET /deployments", ctrl.GetDeployments)

	// AI applications endpoint (bulk-sync for per-consumer rate limiting)
	rr.HandleFuncWithValidation("GET /applications", ctrl.GetApplications)

	// Gateway manifest endpoint
	rr.HandleFuncWithValidation("POST /gateways/{gatewayId}/manifest", ctrl.PushGatewayManifest)

	// LLM Provider endpoints
	rr.HandleFuncWithValidation("GET /llm-providers/{providerId}", ctrl.GetLLMProvider)

	// LLM Proxy endpoints
	rr.HandleFuncWithValidation("GET /llm-proxies/{proxyId}", ctrl.GetLLMProxy)

	// MCP Proxy endpoints
	rr.HandleFuncWithValidation("GET /mcp-proxies/{proxyId}", ctrl.GetMCPProxy)

	// A2A Agent endpoints. The gateway fetches this after an agent.deployed
	// event and expects a ZIP containing the Agent YAML.
	rr.HandleFuncWithValidation("GET /agents/{agentId}", ctrl.GetAgent)
}
