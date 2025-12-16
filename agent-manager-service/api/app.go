// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

// MakeHTTPHandler creates the top-level HTTP handler that routes public and internal API requests and applies the appropriate middleware.
// It registers a health check and mounts API v1 handlers under /api/v1 (agent, infra, and observability routes) with middleware for authentication, correlation IDs, request logging, CORS, and panic recovery.
// It also mounts internal routes under /internal with API-key enforcement, correlation IDs, request logging, and panic recovery.
// The configured http.ServeMux is returned as the HTTP handler.
func MakeHTTPHandler(params *wiring.AppParams) http.Handler {
	mux := http.NewServeMux()

	// Register health check
	registerHealthCheck(mux)

	// Create a sub-mux for API v1 routes
	apiMux := http.NewServeMux()
	registerAgentRoutes(apiMux, params.AgentController)
	registerInfraRoutes(apiMux, params.InfraResourceController)
	registerObservabilityRoutes(apiMux, params.ObservabilityController)

	// Apply middleware in reverse order (last middleware is applied first)
	apiHandler := http.Handler(apiMux)
	apiHandler = params.AuthMiddleware(apiHandler)
	apiHandler = middleware.AddCorrelationID()(apiHandler)
	apiHandler = logger.RequestLogger()(apiHandler)
	apiHandler = middleware.CORS(config.GetConfig().CORSAllowedOrigin)(apiHandler)
	apiHandler = middleware.RecovererOnPanic()(apiHandler)

	// Create a mux for internal API routes
	internalApiMux := http.NewServeMux()
	registerInternalRoutes(internalApiMux, params.BuildCIController)
	internalApiHandler := http.Handler(internalApiMux)
	internalApiHandler = middleware.APIKeyMiddleware()(internalApiHandler) // Add API key middleware for internal routes
	internalApiHandler = middleware.AddCorrelationID()(internalApiHandler)
	internalApiHandler = logger.RequestLogger()(internalApiHandler)
	internalApiHandler = middleware.RecovererOnPanic()(internalApiHandler)

	mux.Handle("/api/v1/", http.StripPrefix("/api/v1", apiHandler))
	mux.Handle("/internal/", http.StripPrefix("/internal", internalApiHandler))

	return mux
}