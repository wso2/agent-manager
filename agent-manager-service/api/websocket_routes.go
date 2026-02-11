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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
)

// registerWebSocketRoutes registers WebSocket gateway routes
// These routes are registered at the internal API level and use API key authentication
func registerWebSocketRoutes(mux *http.ServeMux, wsCtrl controllers.WebSocketGatewayController, internalCtrl controllers.GatewayInternalController) {
	// Internal WebSocket API for gateway connections
	// These routes use API key authentication instead of JWT
	internalMux := http.NewServeMux()
	middleware.HandleFuncWithValidation(internalMux, "GET /ws/gateways/connect", wsCtrl.Connect)

	// Internal API routes for gateway communication (same as api-platform)
	// These endpoints MUST match api-platform's internal API exactly
	middleware.HandleFuncWithValidation(internalMux, "GET /apis/{apiId}", internalCtrl.GetAPI)
	middleware.HandleFuncWithValidation(internalMux, "POST /apis/{apiId}/gateway-deployments", internalCtrl.CreateGatewayDeployment)

	// Apply middleware in reverse order (no auth middleware - WebSocket uses API key)
	wsHandler := http.Handler(internalMux)
	wsHandler = middleware.AddCorrelationID()(wsHandler)
	wsHandler = logger.RequestLogger()(wsHandler)
	wsHandler = middleware.CORS("*")(wsHandler) // Allow all origins for WebSocket connections
	wsHandler = middleware.RecovererOnPanic()(wsHandler)

	mux.Handle("/api/internal/v1/", http.StripPrefix("/api/internal/v1", wsHandler))
}
