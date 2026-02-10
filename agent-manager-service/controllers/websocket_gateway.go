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

package controllers

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
	ws "github.com/wso2/ai-agent-management-platform/agent-manager-service/websocket"

	"github.com/gorilla/websocket"
)

// WebSocketGatewayController defines the interface for WebSocket gateway handlers
type WebSocketGatewayController interface {
	Connect(w http.ResponseWriter, r *http.Request)
}

type websocketGatewayController struct {
	gatewayService services.GatewayService
	wsManager      *ws.Manager
	upgrader       websocket.Upgrader
	rateLimitMu    sync.RWMutex
	rateLimitMap   map[string][]time.Time
	rateLimitCount int
	logger         *slog.Logger
}

// NewWebSocketGatewayController creates a new WebSocket gateway controller
func NewWebSocketGatewayController(gatewayService services.GatewayService, wsManager *ws.Manager, rateLimitCount int, logger *slog.Logger) WebSocketGatewayController {
	return &websocketGatewayController{
		gatewayService: gatewayService,
		wsManager:      wsManager,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			HandshakeTimeout: 10 * time.Second,
		},
		rateLimitMap:   make(map[string][]time.Time),
		rateLimitCount: rateLimitCount,
		logger:         logger,
	}
}

// Connect handles WebSocket upgrade and connection for gateways
func (h *websocketGatewayController) Connect(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	clientIP := r.RemoteAddr

	// Check rate limit
	if !h.checkRateLimit(clientIP) {
		log.Warn("Rate limit exceeded", "address", clientIP)
		utils.WriteErrorResponse(w, http.StatusTooManyRequests, "Connection rate limit exceeded. Please try again later.")
		return
	}

	// Get API key from header
	apiKey := r.Header.Get("api-key")
	if apiKey == "" {
		log.Warn("WebSocket connection attempt without API key", "address", clientIP)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "API key is required. Provide 'api-key' header.")
		return
	}

	// Verify API key and get gateway
	gateway, err := h.gatewayService.VerifyToken(r.Context(), apiKey)
	if err != nil {
		log.Warn("WebSocket authentication failed", "address", clientIP, "error", err)
		if errors.Is(err, utils.ErrGatewayNotFound) {
			utils.WriteErrorResponse(w, http.StatusUnauthorized, "Invalid API key")
		} else {
			utils.WriteErrorResponse(w, http.StatusUnauthorized, "Authentication failed")
		}
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("WebSocket upgrade failed", "gatewayId", gateway.UUID, "error", err)
		return
	}

	// Create transport and register connection
	transport := ws.NewWebSocketTransport(conn)
	connection, err := h.wsManager.Register(gateway.UUID.String(), transport, apiKey)
	if err != nil {
		log.Error("Connection registration failed", "gatewayId", gateway.UUID, "error", err)
		errorMsg := map[string]string{
			"type":    "error",
			"message": err.Error(),
		}
		if jsonErr, _ := json.Marshal(errorMsg); jsonErr != nil {
			if writeErr := conn.WriteMessage(websocket.TextMessage, jsonErr); writeErr != nil {
				log.Error("Failed to send error message", "gatewayId", gateway.UUID, "error", writeErr)
			}
		}
		if closeErr := conn.Close(); closeErr != nil {
			log.Error("Failed to close connection", "gatewayId", gateway.UUID, "error", closeErr)
		}
		return
	}

	// Send connection acknowledgment
	ack := models.ConnectionAckDTO{
		Type:         "connection.ack",
		GatewayID:    gateway.UUID.String(),
		ConnectionID: connection.ConnectionID,
		Timestamp:    time.Now().Format(time.RFC3339),
	}

	ackJSON, err := json.Marshal(ack)
	if err != nil {
		log.Error("Failed to marshal connection ACK", "gatewayId", gateway.UUID, "error", err)
	} else {
		if err := connection.Send(ackJSON); err != nil {
			log.Error("Failed to send connection ACK",
				"gatewayId", gateway.UUID,
				"connectionId", connection.ConnectionID,
				"error", err)
		}
	}

	log.Info("WebSocket connection established",
		"gatewayId", gateway.UUID,
		"connectionId", connection.ConnectionID,
		"address", clientIP,
	)

	// Update gateway active status
	if err := h.gatewayService.UpdateGatewayActiveStatus(r.Context(), gateway.UUID.String(), true); err != nil {
		log.Error("Failed to update gateway active status",
			"gatewayId", gateway.UUID,
			"status", "active",
			"error", err)
	}

	// Run read loop (blocks until connection closes)
	h.readLoop(connection)

	log.Info("WebSocket connection closed",
		"gatewayId", gateway.UUID,
		"connectionId", connection.ConnectionID,
	)

	// Unregister connection
	h.wsManager.Unregister(gateway.UUID.String(), connection.ConnectionID)

	// Update gateway active status
	if err := h.gatewayService.UpdateGatewayActiveStatus(r.Context(), gateway.UUID.String(), false); err != nil {
		log.Error("Failed to update gateway active status",
			"gatewayId", gateway.UUID,
			"status", "inactive",
			"error", err)
	}
}

// readLoop reads messages from the WebSocket connection
func (h *websocketGatewayController) readLoop(conn *ws.Connection) {
	defer func() {
		if r := recover(); r != nil {
			h.logger.Error("Panic in WebSocket read loop",
				"gatewayId", conn.GatewayID,
				"connectionId", conn.ConnectionID,
				"panic", r,
			)
		}
	}()

	for {
		if conn.IsClosed() {
			return
		}

		wsTransport, ok := conn.Transport.(*ws.WebSocketTransport)
		if !ok {
			h.logger.Error("Invalid transport type for connection",
				"gatewayId", conn.GatewayID,
				"connectionId", conn.ConnectionID,
			)
			return
		}

		_, _, err := wsTransport.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				h.logger.Error("WebSocket read error",
					"gatewayId", conn.GatewayID,
					"connectionId", conn.ConnectionID,
					"error", err,
				)
			}
			return
		}
	}
}

// checkRateLimit implements a sliding window rate limit per IP address
func (h *websocketGatewayController) checkRateLimit(clientIP string) bool {
	h.rateLimitMu.Lock()
	defer h.rateLimitMu.Unlock()

	now := time.Now()
	oneMinuteAgo := now.Add(-1 * time.Minute)

	attempts, exists := h.rateLimitMap[clientIP]
	if !exists {
		attempts = []time.Time{}
	}

	var recentAttempts []time.Time
	for _, t := range attempts {
		if t.After(oneMinuteAgo) {
			recentAttempts = append(recentAttempts, t)
		}
	}

	if len(recentAttempts) >= h.rateLimitCount {
		return false
	}

	recentAttempts = append(recentAttempts, now)
	h.rateLimitMap[clientIP] = recentAttempts

	return true
}
