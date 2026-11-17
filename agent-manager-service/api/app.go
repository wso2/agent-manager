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
	"github.com/wso2/agent-manager/agent-manager-service/builtin"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/mcp"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/webconsole"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// registerAPIRoutes registers every authenticated /api/v1 route.
//
// It is separated from MakeHTTPHandler so the audit coverage test can drive
// registration with a bare registrar and assert that no mutating route shipped
// unaudited. Keeping the list in one function is what makes that check total
// rather than a sample.
func registerAPIRoutes(rr *middleware.RouteRegistrar, params *wiring.AppParams, bundles builtin.Built) {
	registerAgentRoutes(rr, params.AgentController)
	registerAgentTokenRoutes(rr, params.AgentTokenController)
	registerInfraRoutes(rr, params.InfraResourceController)
	registerEnvironmentRoutes(rr, params.EnvironmentController, bundles.EnvironmentReaders())
	registerEventsRoutes(rr, params.EventsController)
	registerIdentityRoutes(rr, params.IdentityController)
	registerTelemetryRoutes(rr)
	bundles.Routes(rr)
}

// MakeHTTPHandler creates a new HTTP handler with middleware and routes.
// extraAPIRoutes, if non-nil, is called to register additional routes onto the
// authenticated /api/v1 sub-mux before middleware is applied.
func MakeHTTPHandler(params *wiring.AppParams, extraAPIRoutes func(*http.ServeMux, *wiring.AppParams)) http.Handler {
	return MakeHTTPHandlerWithMiddleware(params, extraAPIRoutes, nil)
}

// MakeHTTPHandlerWithMiddleware is MakeHTTPHandler with apiMiddleware, if non-nil,
// wrapping the authenticated API mux before dispatch.
func MakeHTTPHandlerWithMiddleware(params *wiring.AppParams, extraAPIRoutes func(*http.ServeMux, *wiring.AppParams), apiMiddleware func(*http.ServeMux) http.Handler) http.Handler {
	return MakeHTTPHandlerWithModules(params, extraAPIRoutes, apiMiddleware, nil, nil)
}

// Modules serves the routes of the active modules.
type Modules interface {
	// ServeAPI serves /api/v1/m/<module>/..., behind sign-in.
	ServeAPI() http.Handler
	// ServeInternal serves /api/internal/v1/m/<module>/... for module workloads.
	ServeInternal() http.Handler
	// ServeTokenExchange serves POST /api/internal/v1/workload-tokens.
	ServeTokenExchange() http.Handler
}

// ModuleMounter registers a module's public route behind the same middleware
// as the core's routes: org resolution, permission check, audit and events.
func ModuleMounter(params *wiring.AppParams) func(*http.ServeMux, modhost.Route) {
	return func(mux *http.ServeMux, r modhost.Route) {
		rr := middleware.NewRouteRegistrar(mux, params.OrgResolver, params.AuditRecorder)
		rr.SetEventPublisher(params.EventPublisher)
		rr.HandleModuleRoute(r.Pattern(), r.Permission, r.Handler)
	}
}

// MakeHTTPHandlerWithModules is MakeHTTPHandlerWithMiddleware that also serves
// the active modules' routes; modules may be nil.
func MakeHTTPHandlerWithModules(params *wiring.AppParams, extraAPIRoutes func(*http.ServeMux, *wiring.AppParams), apiMiddleware func(*http.ServeMux) http.Handler, modules Modules, bundles builtin.Built) http.Handler {
	mux := http.NewServeMux()

	// Register health check
	registerHealthCheck(mux)

	// Register JWKS endpoint at root level (no authentication required)
	registerJWKSRoute(mux, params.AgentTokenController)

	// Register OAuth 2.0 Protected Resource Metadata (RFC 9728) at root level (no authentication required)
	registerWellKnownRoutes(mux)

	// Register the file-based login endpoint at root level. It is where a
	// caller goes to obtain a token, so it cannot itself require one.
	registerFileAuthRoutes(mux, params.FileAuthService, params.AuditRecorder, config.GetConfig().CORSAllowedOrigin)

	// Register service-configuration discovery endpoint at root level (no authentication required)
	registerConfigRoutes(mux)

	// Register MCP at root level, with the core's tools and those the
	// built-in features add.
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
	}, mcpMiddleware, bundles.MCPTools()...)

	// Other routes of built-in bundles that live at the root authenticate
	// themselves.
	bundles.RootRoutes(mux, params)

	// Authentication is rejected before route matching, so the audit middleware
	// installed per route never sees it. This hook is how a failed token
	// reaches the trail; it is set once because the auth middleware is global.
	jwtassertion.SetAuthFailureHook(audit.AuthFailureRecorder(params.AuditRecorder))

	// Create a sub-mux for API v1 routes (JWT-authenticated)
	apiMux := http.NewServeMux()
	rr := middleware.NewRouteRegistrar(apiMux, params.OrgResolver, params.AuditRecorder)
	rr.SetEventPublisher(params.EventPublisher)
	registerAPIRoutes(rr, params, bundles)
	var ui UISource
	if src, ok := modules.(UISource); ok {
		ui = src
	}
	registerUIManifestRoute(rr, ui)
	if modules != nil {
		// The module is looked up on every request, so enabling or disabling
		// one changes these routes without a restart.
		apiMux.Handle("/m/", modules.ServeAPI())
	}
	if extraAPIRoutes != nil {
		extraAPIRoutes(apiMux, params)
	}

	// Apply middleware in reverse order (last middleware is applied first)
	apiHandler := http.Handler(apiMux)
	if apiMiddleware != nil {
		apiHandler = apiMiddleware(apiMux)
	}
	apiHandler = params.AuthMiddleware(apiHandler)
	apiHandler = logger.RequestLogger()(apiHandler)
	apiHandler = middleware.AddCorrelationID()(apiHandler)
	apiHandler = middleware.CORS(config.GetConfig().CORSAllowedOrigin)(apiHandler)
	apiHandler = middleware.RecovererOnPanic()(apiHandler)

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiHandler))

	// The console, at the root: its files, its runtime config and, for any
	// other path, its index page. Every API path above is more specific.
	if config.GetConfig().ConsoleEnabled {
		mux.Handle("/", middleware.RecovererOnPanic()(webconsole.Handler()))
	}

	return mux
}

// MakeInternalHTTPHandler creates the internal HTTPS server handler
// This server hosts WebSocket connections and gateway internal APIs without JWT middleware
func MakeInternalHTTPHandler(params *wiring.AppParams) http.Handler {
	return MakeInternalHTTPHandlerWithModules(params, nil, nil)
}

// MakeInternalHTTPHandlerWithModules is MakeInternalHTTPHandler that also serves
// module workload routes and the workload token exchange; modules may be nil.
func MakeInternalHTTPHandlerWithModules(params *wiring.AppParams, modules Modules, bundles builtin.Built) http.Handler {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			logger.GetLogger(r.Context()).Error("Failed to write health check response", "error", err)
		}
	})

	// Create internal mux for the routes of module workloads and built-in
	// bundles, such as the gateway internal API and WebSocket (NO JWT
	// middleware); these authenticate themselves
	internalMux := http.NewServeMux()
	internalRR := middleware.NewInternalRouteRegistrar(internalMux, params.AuditRecorder)
	bundles.InternalRoutes(internalRR, internalMux)
	if modules != nil {
		internalMux.Handle("/m/", modules.ServeInternal())
		internalMux.Handle("POST /workload-tokens", modules.ServeTokenExchange())
	}

	// Apply basic middleware (no JWT auth)
	internalHandler := http.Handler(internalMux)
	// The registrar above audits the routes it owns. This installs the recorder
	// for everything else on the internal chain — the WebSocket upgrade, and the
	// handler-level emits for api-key rejections, which happen before any route
	// wrapper could see them.
	internalHandler = middleware.WithAuditRecorder(params.AuditRecorder, audit.SurfaceInternal)(internalHandler)
	internalHandler = logger.RequestLogger()(internalHandler)
	internalHandler = middleware.AddCorrelationID()(internalHandler)
	internalHandler = middleware.CORS(config.GetConfig().CORSAllowedOrigin)(internalHandler)
	internalHandler = middleware.RecovererOnPanic()(internalHandler)

	mux.Handle("/api/internal/v1/", http.StripPrefix("/api/internal/v1", internalHandler))

	return mux
}
