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
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/mcp"
	"github.com/wso2/agent-manager/agent-manager-service/mcp/tools"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
	"github.com/wso2/agent-manager/agent-manager-service/webconsole"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// registerAPIRoutes registers every authenticated /api/v1 route.
//
// It is separated from MakeHTTPHandler so the audit coverage test can drive
// registration with a bare registrar and assert that no mutating route shipped
// unaudited. Keeping the list in one function is what makes that check total
// rather than a sample.
func registerAPIRoutes(rr *middleware.RouteRegistrar, params *wiring.AppParams) {
	registerAgentRoutes(rr, params.AgentController)
	registerAgentTokenRoutes(rr, params.AgentTokenController)
	registerInfraRoutes(rr, params.InfraResourceController)
	registerEnvironmentRoutes(rr, params.EnvironmentController)
	registerEventsRoutes(rr, params.EventsController)
	registerIdentityRoutes(rr, params.IdentityController)
	registerTelemetryRoutes(rr)
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
	return MakeHTTPHandlerWithModules(params, extraAPIRoutes, apiMiddleware, nil)
}

// Modules serves the routes of the active modules.
type Modules interface {
	// ServeAPI serves /m/<module>/... on the API port, behind sign-in.
	ServeAPI() http.Handler
	// ServeInternal serves /m/<module>/internal/... on the internal server,
	// for module workloads.
	ServeInternal() http.Handler
	// ServeHeartbeat serves POST /api/internal/v1/workloads/heartbeat.
	ServeHeartbeat() http.Handler
	// ServeInternalAt serves the routes modules mount at fixed paths below
	// /api/internal/v1.
	ServeInternalAt() http.Handler
	// MCPTools lists the active modules' MCP tools.
	MCPTools() []modhost.MCPTool
}

// ModuleMounter registers a module's public route behind the same middleware
// as the core's routes: org resolution, permission check, audit and events.
func ModuleMounter(params *wiring.AppParams) func(*http.ServeMux, modhost.Route) {
	return func(mux *http.ServeMux, r modhost.Route) {
		rr := middleware.NewRouteRegistrar(mux, params.OrgResolver, params.AuditRecorder)
		rr.SetEventPublisher(params.EventPublisher)
		rr.SetModule(r.Module)
		mountModuleRoute(rr, r, params.OUHandleLookup)
	}
}

// ModuleInternalMounter registers a module's route on the internal server
// that authenticates its own callers (HandleInternalAt, HandleWebhook) behind
// the internal server's audit middleware.
func ModuleInternalMounter(params *wiring.AppParams) func(*http.ServeMux, modhost.Route) {
	return func(mux *http.ServeMux, r modhost.Route) {
		rr := middleware.NewInternalRouteRegistrar(mux, params.AuditRecorder)
		rr.SetModule(r.Module)
		rr.HandleModuleInternalRoute(r.Pattern(), modulePolicy(r), r.Handler)
	}
}

func mountModuleRoute(rr *middleware.RouteRegistrar, r modhost.Route, lookup middleware.OUHandleLookup) {
	policy := modulePolicy(r)
	if r.AllowOrgOverride {
		policy.OrgOverride = middleware.ApplyOrgOverride(lookup)
	}
	rr.HandleModuleRoute(r.Pattern(), r.Permission, policy, r.Handler)
}

// modulePolicy is what a module route declared for the audit trail and
// webhooks.
func modulePolicy(r modhost.Route) middleware.ModuleRoutePolicy {
	return middleware.ModuleRoutePolicy{
		Audit: audit.RoutePolicy{
			Action: audit.Action(r.AuditAction), Read: r.AuditRead, Coalesce: r.AuditCoalesce, FailClosed: r.AuditFailClosed,
		},
		Event:            r.Event,
		EventScope:       events.Scope(r.EventScope),
		EventWithoutData: r.EventWithoutData,
		AlsoRequires:     r.AlsoRequires,
		AnyOf:            r.AnyOf,
		PlatformAdmin:    r.PlatformAdmin,
	}
}

// MakeHTTPHandlerWithModules is MakeHTTPHandlerWithMiddleware that also serves
// the active modules' routes; modules may be nil.
func MakeHTTPHandlerWithModules(params *wiring.AppParams, extraAPIRoutes func(*http.ServeMux, *wiring.AppParams), apiMiddleware func(*http.ServeMux) http.Handler, modules Modules) http.Handler {
	mux := http.NewServeMux()

	// Register health check
	registerHealthCheck(mux, params.SecretVaults)

	// Register JWKS endpoint at root level (no authentication required)
	registerJWKSRoute(mux, params.AgentTokenController)

	// Register OAuth 2.0 Protected Resource Metadata (RFC 9728) at root level (no authentication required)
	registerWellKnownRoutes(mux, modules)

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
	mcpDeps := mcp.Dependencies{
		InfraResourceManager:     params.InfraResourceManager,
		AgentManagerService:      params.AgentManagerService,
		AgentTokenManagerService: params.AgentTokenManagerService,
		EnvironmentService:       params.EnvironmentService,
	}
	if modules != nil {
		mcpDeps.ModuleTools = moduleTools{modules}
	}
	mcp.RegisterRoute(mux, mcpDeps, mcpMiddleware)

	// Authentication is rejected before route matching, so the audit middleware
	// installed per route never sees it. This hook is how a failed token
	// reaches the trail; it is set once because the auth middleware is global.
	jwtassertion.SetAuthFailureHook(audit.AuthFailureRecorder(params.AuditRecorder))

	// Create a sub-mux for API v1 routes (JWT-authenticated)
	apiMux := http.NewServeMux()
	rr := middleware.NewRouteRegistrar(apiMux, params.OrgResolver, params.AuditRecorder)
	rr.SetEventPublisher(params.EventPublisher)
	registerAPIRoutes(rr, params)
	var ui UISource
	if src, ok := modules.(UISource); ok {
		ui = src
	}
	registerUIManifestRoute(rr, ui)
	if extraAPIRoutes != nil {
		extraAPIRoutes(apiMux, params)
	}

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", signedIn(params, apiMux, apiMiddleware)))
	if modules != nil {
		// Module APIs live beside the core's, at /m/<module>, behind the same
		// chain. The module is looked up on every request, so enabling or
		// disabling one changes these routes without a restart.
		moduleMux := http.NewServeMux()
		moduleMux.Handle("/m/", modules.ServeAPI())
		mux.Handle("/m/", signedIn(params, moduleMux, apiMiddleware))
	}

	// The console, at the root: its files, its runtime config and, for any
	// other path, its index page. Every API path above is more specific.
	if config.GetConfig().ConsoleEnabled {
		mux.Handle("/", middleware.RecovererOnPanic()(webconsole.Handler()))
	}

	return mux
}

// signedIn wraps an API mux in the chain every signed-in API route gets.
// Middleware is applied in reverse order (the last one runs first).
func signedIn(params *wiring.AppParams, apiMux *http.ServeMux, apiMiddleware func(*http.ServeMux) http.Handler) http.Handler {
	h := http.Handler(apiMux)
	if apiMiddleware != nil {
		h = apiMiddleware(apiMux)
	}
	// After authentication, so an unauthenticated request still gets 401 first.
	h = middleware.RequireIdentityProvider(config.GetConfig().AuthMode)(h)
	h = params.AuthMiddleware(h)
	h = logger.RequestLogger()(h)
	h = middleware.AddCorrelationID()(h)
	h = middleware.CORS(config.GetConfig().CORSAllowedOrigin)(h)
	return middleware.RecovererOnPanic()(h)
}

// internalChain wraps an internal-server mux: no JWT, the routes authenticate
// themselves. The audit recorder covers what the route registrar does not —
// the WebSocket upgrade, and the handler-level emits for api-key rejections,
// which happen before any route wrapper could see them.
func internalChain(params *wiring.AppParams, internalMux *http.ServeMux) http.Handler {
	h := middleware.WithAuditRecorder(params.AuditRecorder, audit.SurfaceInternal)(internalMux)
	h = logger.RequestLogger()(h)
	h = middleware.AddCorrelationID()(h)
	h = middleware.CORS(config.GetConfig().CORSAllowedOrigin)(h)
	return middleware.RecovererOnPanic()(h)
}

// MakeInternalHTTPHandler creates the internal HTTPS server handler
// This server hosts WebSocket connections and gateway internal APIs without JWT middleware
func MakeInternalHTTPHandler(params *wiring.AppParams) http.Handler {
	return MakeInternalHTTPHandlerWithModules(params, nil)
}

// MakeInternalHTTPHandlerWithModules is MakeInternalHTTPHandler that also serves
// module workload routes and workload heartbeats; modules may be nil.
func MakeInternalHTTPHandlerWithModules(params *wiring.AppParams, modules Modules) http.Handler {
	mux := http.NewServeMux()

	// Health check endpoint
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte(`{"status":"ok"}`)); err != nil {
			logger.GetLogger(r.Context()).Error("Failed to write health check response", "error", err)
		}
	})

	// The internal API (NO JWT middleware): workload heartbeats and the
	// routes modules mount at fixed paths, such as the AI Gateway's
	// controller API and WebSocket, which authenticate themselves.
	internalMux := http.NewServeMux()
	if modules != nil {
		internalMux.Handle("POST /workloads/heartbeat", modules.ServeHeartbeat())
		// Routes modules mount at fixed paths (HandleInternalAt); every
		// path the core serves here is more specific.
		internalMux.Handle("/", modules.ServeInternalAt())
	}
	mux.Handle("/api/internal/v1/", http.StripPrefix("/api/internal/v1", internalChain(params, internalMux)))
	if modules != nil {
		// Module workload routes and webhooks, at /m/<module>/internal/... and
		// /m/<module>/webhooks/...
		moduleMux := http.NewServeMux()
		moduleMux.Handle("/m/", modules.ServeInternal())
		mux.Handle("/m/", internalChain(params, moduleMux))
	}

	return mux
}

// moduleTools hands the active modules' MCP tools to the MCP server.
type moduleTools struct{ modules Modules }

func (m moduleTools) ModuleTools() []tools.ModuleTool {
	listed := m.modules.MCPTools()
	out := make([]tools.ModuleTool, 0, len(listed))
	for _, t := range listed {
		out = append(out, tools.ModuleTool{
			Module: t.Module, Name: t.Tool.Name, Title: t.Tool.Title, Description: t.Tool.Description,
			InputSchema: t.Tool.InputSchema, ReadOnly: t.Tool.ReadOnly, Destructive: t.Tool.Destructive,
			Permission: t.Permission, Action: audit.Action(t.Action), AuditRead: t.AuditRead, Handler: t.Handler,
		})
	}
	return out
}
