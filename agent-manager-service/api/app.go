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

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"

	"github.com/wso2/agent-manager/agent-manager-service/mcp"
)

// registerAPIRoutes registers every authenticated /api/v1 route.
//
// It is separated from MakeHTTPHandler so the audit coverage test can drive
// registration with a bare registrar and assert that no mutating route shipped
// unaudited. Keeping the list in one function is what makes that check total
// rather than a sample.
func registerAPIRoutes(rr *middleware.RouteRegistrar, params *wiring.AppParams) {
	registerAgentRoutes(rr, params.AgentController)
	registerAgentKindRoutes(rr, params.AgentKindController)
	registerAgentTokenRoutes(rr, params.AgentTokenController)
	registerInfraRoutes(rr, params.InfraResourceController)
	registerRepositoryRoutes(rr, params.RepositoryController)
	registerEnvironmentRoutes(rr, params.EnvironmentController)
	RegisterGatewayRoutes(rr, params.GatewayController)
	registerMonitorRoutes(rr, params.MonitorController)
	registerMonitorScoreRoutes(rr, params.MonitorScoresController)
	registerEvaluatorRoutes(rr, params.EvaluatorController)
	registerCatalogRoutes(rr, params.CatalogController)
	registerAgentBuildOptionsRoutes(rr, params.AgentBuildOptionsController)
	RegisterLLMRoutes(rr, params.LLMController)
	RegisterLLMDeploymentRoutes(rr, params.LLMDeploymentController)
	RegisterLLMProviderAPIKeyRoutes(rr, params.LLMProviderAPIKeyController)
	RegisterLLMProxyAPIKeyRoutes(rr, params.LLMProxyAPIKeyController)
	RegisterAgentAPIKeyRoutes(rr, params.AgentAPIKeyController)
	RegisterLLMProxyDeploymentRoutes(rr, params.LLMProxyDeploymentController)
	RegisterMCPProxyRoutes(rr, params.MCPProxyController)
	RegisterAgentConfigRoutes(rr, params.AgentConfigurationController)
	RegisterMonitorPublisherRoutes(rr, params.MonitorScoresPublisherController)
	RegisterGitSecretRoutes(rr, params.GitSecretController)
	registerIdentityRoutes(rr, params.IdentityController)
	registerMCPProxyScopeRoutes(rr, params.MCPProxyScopeController)
	registerAgentIdentityRoutes(rr, params.AgentIdentityController)
}

// MakeHTTPHandler creates a new HTTP handler with middleware and routes.
// extraAPIRoutes, if non-nil, is called to register additional routes onto the
// authenticated /api/v1 sub-mux before middleware is applied.
func MakeHTTPHandler(params *wiring.AppParams, extraAPIRoutes func(*http.ServeMux, *wiring.AppParams)) http.Handler {
	mux := http.NewServeMux()

	// Register health check
	registerHealthCheck(mux)

	// Register JWKS endpoint at root level (no authentication required)
	registerJWKSRoute(mux, params.AgentTokenController)

	// Register OAuth 2.0 Protected Resource Metadata (RFC 9728) at root level (no authentication required)
	registerWellKnownRoutes(mux)

	// Register service-configuration discovery endpoint at root level (no authentication required)
	registerConfigRoutes(mux)

	// Register Caddy's on-demand TLS ask endpoint at root level (no authentication
	// required — see registerThunderAskRoute's doc comment)
	registerThunderAskRoute(mux, params.EnvironmentService)

	// Register MCP at root level.
	//
	// MCP does not go through the route registrar, so the recorder is installed
	// by composing it into the middleware MCP is given. Without it, services
	// that refuse to act when they cannot be audited would fail every MCP call
	// that reaches them. Auth stays outermost so claims are present by the time
	// a handler emits.
	mcpMiddleware := func(next http.Handler) http.Handler {
		return params.AuthMiddleware(
			middleware.WithAuditRecorder(params.AuditRecorder, audit.SurfaceMCP)(next),
		)
	}
	mcp.RegisterRoute(mux, mcp.Dependencies{
		InfraResourceManager:     params.InfraResourceManager,
		AgentManagerService:      params.AgentManagerService,
		AgentTokenManagerService: params.AgentTokenManagerService,
		EnvironmentService:       params.EnvironmentService,
	}, mcpMiddleware)

	// Authentication is rejected before route matching, so the audit middleware
	// installed per route never sees it. This hook is how a failed token
	// reaches the trail; it is set once because the auth middleware is global.
	jwtassertion.SetAuthFailureHook(audit.AuthFailureRecorder(params.AuditRecorder))

	// Create a sub-mux for API v1 routes (JWT-authenticated)
	apiMux := http.NewServeMux()
	rr := middleware.NewRouteRegistrar(apiMux, params.OrgResolver, params.AuditRecorder)
	registerAPIRoutes(rr, params)
	if extraAPIRoutes != nil {
		extraAPIRoutes(apiMux, params)
	}

	// Apply middleware in reverse order (last middleware is applied first)
	apiHandler := http.Handler(apiMux)
	apiHandler = params.AuthMiddleware(apiHandler)
	// Applied innermost-first. RecovererOnPanic sits *inside* AddCorrelationID
	// and RequestLogger so the record it writes carries the correlation ID and
	// the request line; outside them it could only ever log "unknown". The
	// three middleware now outside it do no caller-driven work.
	apiHandler = middleware.RecovererOnPanic()(apiHandler)
	apiHandler = logger.RequestLogger()(apiHandler)
	apiHandler = middleware.AddCorrelationID()(apiHandler)
	apiHandler = middleware.CORS(config.GetConfig().CORSAllowedOrigin)(apiHandler)

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiHandler))

	return mux
}

// MakeInternalHTTPHandler creates the internal HTTPS server handler
// This server hosts WebSocket connections and gateway internal APIs without JWT middleware
func MakeInternalHTTPHandler(params *wiring.AppParams) http.Handler {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			logger.GetLogger(r.Context()).Error("Failed to write health check response", "error", err)
		}
	})

	// Create internal mux for gateway internal and WebSocket routes (NO JWT middleware)
	// These routes use api-key header authentication instead
	internalMux := http.NewServeMux()
	internalRR := middleware.NewInternalRouteRegistrar(internalMux, params.AuditRecorder)
	RegisterGatewayInternalRoutes(internalRR, params.GatewayInternalController)
	RegisterWebSocketRoutes(internalMux, params.WebSocketController)

	// Apply basic middleware (no JWT auth)
	internalHandler := http.Handler(internalMux)
	// The registrar above audits the routes it owns. This installs the recorder
	// for everything else on the internal chain — the WebSocket upgrade, and the
	// handler-level emits for api-key rejections, which happen before any route
	// wrapper could see them.
	internalHandler = middleware.WithAuditRecorder(params.AuditRecorder, audit.SurfaceInternal)(internalHandler)
	internalHandler = middleware.RecovererOnPanic()(internalHandler)
	internalHandler = logger.RequestLogger()(internalHandler)
	internalHandler = middleware.AddCorrelationID()(internalHandler)
	internalHandler = middleware.CORS(config.GetConfig().CORSAllowedOrigin)(internalHandler)

	mux.Handle("/api/internal/v1/", http.StripPrefix("/api/internal/v1", internalHandler))

	return mux
}
