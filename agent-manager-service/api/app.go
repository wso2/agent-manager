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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/wiring"
)

// MakeHTTPHandler creates a new HTTP handler with middleware and routes
func MakeHTTPHandler(params *wiring.AppParams) http.Handler {
	mux := http.NewServeMux()

	// Register health check
	registerHealthCheck(mux)

	// Register JWKS endpoint at root level (no authentication required)
	registerJWKSRoute(mux, params.AgentTokenController)

	// Create a sub-mux for API v1 routes (JWT-authenticated)
	apiMux := http.NewServeMux()
	registerAgentRoutes(apiMux, params.AgentController)
	registerAgentTokenRoutes(apiMux, params.AgentTokenController)
	registerInfraRoutes(apiMux, params.InfraResourceController)
	registerObservabilityRoutes(apiMux, params.ObservabilityController)
	registerRepositoryRoutes(apiMux, params.RepositoryController)
	registerEnvironmentRoutes(apiMux, params.EnvironmentController)
	RegisterGatewayRoutes(apiMux, params.GatewayController)
	registerMonitorRoutes(apiMux, params.MonitorController)
	registerEvaluatorRoutes(apiMux, params.EvaluatorController)
	RegisterLLMRoutes(apiMux, params.LLMController)
	RegisterLLMDeploymentRoutes(apiMux, params.LLMDeploymentController)

	// Apply middleware in reverse order (last middleware is applied first)
	apiHandler := http.Handler(apiMux)
	apiHandler = params.AuthMiddleware(apiHandler)
	apiHandler = middleware.AddCorrelationID()(apiHandler)
	apiHandler = logger.RequestLogger()(apiHandler)
	apiHandler = middleware.CORS(config.GetConfig().CORSAllowedOrigin)(apiHandler)
	apiHandler = middleware.RecovererOnPanic()(apiHandler)

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
	RegisterGatewayInternalRoutes(internalMux, params.GatewayInternalController)
	RegisterWebSocketRoutes(internalMux, params.WebSocketController)

	// Apply basic middleware (no JWT auth)
	internalHandler := http.Handler(internalMux)
	internalHandler = middleware.AddCorrelationID()(internalHandler)
	internalHandler = logger.RequestLogger()(internalHandler)
	internalHandler = middleware.CORS(config.GetConfig().CORSAllowedOrigin)(internalHandler)
	internalHandler = middleware.RecovererOnPanic()(internalHandler)

	mux.Handle("/api/internal/v1/", http.StripPrefix("/api/internal/v1", internalHandler))

	return mux
}
