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
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	ws "github.com/wso2/ai-agent-management-platform/agent-manager-service/websocket"
)

// WebSocketController defines interface for WebSocket HTTP handlers
type WebSocketController interface {
	Connect(w http.ResponseWriter, r *http.Request)
}

type websocketController struct {
	manager        *ws.Manager
	gatewayService *services.PlatformGatewayService
	upgrader       websocket.Upgrader

	// Rate limiting: track connection attempts per IP
	rateLimitMu    sync.RWMutex
	rateLimitMap   map[string][]time.Time
	rateLimitCount int
}

// ConnectionAckDTO represents the acknowledgment message sent when a gateway connects
type ConnectionAckDTO struct {
	Type         string `json:"type"`
	GatewayID    string `json:"gatewayId"`
	ConnectionID string `json:"connectionId"`
	Timestamp    string `json:"timestamp"`
}

// NewWebSocketController creates a new WebSocket controller
func NewWebSocketController(
	manager *ws.Manager,
	gatewayService *services.PlatformGatewayService,
	rateLimitCount int,
) WebSocketController {
	ctrl := &websocketController{
		manager:        manager,
		gatewayService: gatewayService,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// TODO: Implement proper origin checking in production
				return true
			},
			HandshakeTimeout: 10 * time.Second,
		},
		rateLimitMap:   make(map[string][]time.Time),
		rateLimitCount: rateLimitCount,
	}

	// Start periodic cleanup goroutine to prevent memory leak
	// Cleans up rate limit entries for IPs that haven't connected recently
	go ctrl.cleanupRateLimitMap()

	return ctrl
}

// Connect handles WebSocket upgrade requests
// This is the entry point for gateway connections
func (c *websocketController) Connect(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract client IP for rate limiting
	clientIP := getClientIP(r)

	// Check rate limit
	if !c.checkRateLimit(clientIP) {
		log.Warn("Rate limit exceeded", "ip", clientIP)
		http.Error(w, "Connection rate limit exceeded. Please try again later.", http.StatusTooManyRequests)
		return
	}

	// Extract and validate API key from header
	apiKey := r.Header.Get("api-key")
	if apiKey == "" {
		log.Warn("WebSocket connection attempt without API key", "ip", clientIP)
		http.Error(w, "API key is required. Provide 'api-key' header.", http.StatusUnauthorized)
		return
	}

	// Authenticate gateway using API key
	gateway, err := c.gatewayService.VerifyToken(apiKey)
	if err != nil {
		log.Warn("WebSocket authentication failed", "ip", clientIP, "error", err)
		http.Error(w, "Invalid or expired API key", http.StatusUnauthorized)
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := c.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("WebSocket upgrade failed", "gatewayID", gateway.UUID.String(), "error", err)
		// Upgrade error is already sent by upgrader
		return
	}

	// Create WebSocket transport
	transport := ws.NewWebSocketTransport(conn)

	// Register connection with manager
	connection, err := c.manager.Register(gateway.UUID.String(), transport, apiKey)
	if err != nil {
		log.Error("Connection registration failed", "gatewayID", gateway.UUID.String(), "error", err)
		// Send error message before closing
		errorMsg := map[string]string{
			"type":    "error",
			"message": err.Error(),
		}
		if jsonErr, _ := json.Marshal(errorMsg); jsonErr != nil {
			if err := conn.WriteMessage(websocket.TextMessage, jsonErr); err != nil {
				log.Error("Failed to send error message", "gatewayID", gateway.UUID.String(), "error", err)
			}
		}
		if err := conn.Close(); err != nil {
			log.Error("Failed to close connection", "gatewayID", gateway.UUID.String(), "error", err)
		}
		return
	}

	// Send connection acknowledgment
	ack := ConnectionAckDTO{
		Type:         "connection.ack",
		GatewayID:    gateway.UUID.String(),
		ConnectionID: connection.ConnectionID,
		Timestamp:    time.Now().Format(time.RFC3339),
	}

	ackJSON, err := json.Marshal(ack)
	if err != nil {
		log.Error("Failed to marshal connection ACK", "gatewayID", gateway.UUID.String(), "error", err)
	} else {
		if err := connection.Send(ackJSON); err != nil {
			log.Error("Failed to send connection ACK", "gatewayID", gateway.UUID.String(), "connectionID", connection.ConnectionID, "error", err)
		}
	}

	log.Info("WebSocket connection established", "gatewayID", gateway.UUID.String(), "connectionID", connection.ConnectionID)

	// Update gateway active status to true when connection is established
	if err := c.gatewayService.UpdateGatewayActiveStatus(gateway.UUID.String(), true); err != nil {
		log.Error("Failed to update gateway active status to true", "gatewayID", gateway.UUID.String(), "error", err)
	}

	// Start reading messages (blocks until connection closes)
	// This keeps the handler goroutine alive to maintain the connection
	c.readLoop(connection)

	// Connection closed - cleanup
	log.Info("WebSocket connection closed", "gatewayID", gateway.UUID.String(), "connectionID", connection.ConnectionID)
	c.manager.Unregister(gateway.UUID.String(), connection.ConnectionID)

	// Update gateway active status to false when connection is disconnected
	if err := c.gatewayService.UpdateGatewayActiveStatus(gateway.UUID.String(), false); err != nil {
		log.Error("Failed to update gateway active status to false", "gatewayID", gateway.UUID.String(), "error", err)
	}
}

// readLoop reads messages from the WebSocket connection.
// This is primarily for handling control frames (ping/pong) and detecting disconnections.
// Gateways are not expected to send application messages to the platform.
func (c *websocketController) readLoop(conn *ws.Connection) {
	defer func() {
		if r := recover(); r != nil {
			logger.GetLogger(context.TODO()).Error("Panic in WebSocket read loop", "gatewayID", conn.GatewayID, "connectionID", conn.ConnectionID, "panic", r)
		}
	}()

	// Read messages until connection closes
	// The gorilla/websocket library handles ping/pong automatically via SetPongHandler
	for {
		// Check if connection is closed
		if conn.IsClosed() {
			return
		}

		// Read next message (blocks until message or error)
		// We don't expect gateways to send messages, but we need to read
		// to detect disconnections and handle control frames
		wsTransport, ok := conn.Transport.(*ws.WebSocketTransport)
		if !ok {
			logger.GetLogger(context.TODO()).Error("Invalid transport type for connection", "gatewayID", conn.GatewayID, "connectionID", conn.ConnectionID)
			return
		}

		_, _, err := wsTransport.ReadMessage()
		if err != nil {
			// Connection closed or error occurred
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				logger.GetLogger(context.TODO()).Error("WebSocket read error", "gatewayID", conn.GatewayID, "connectionID", conn.ConnectionID, "error", err)
			}
			return
		}

		// If gateway sends messages, we can handle them here in future iterations
		// For now, we just ignore any messages from the gateway
	}
}

// checkRateLimit verifies if the client IP is within rate limits.
// Returns true if connection is allowed, false if rate limit exceeded.
//
// Rate limit: rateLimitCount connections per minute per IP
func (c *websocketController) checkRateLimit(clientIP string) bool {
	c.rateLimitMu.Lock()
	defer c.rateLimitMu.Unlock()

	now := time.Now()
	oneMinuteAgo := now.Add(-1 * time.Minute)

	// Get recent connection attempts for this IP
	attempts, exists := c.rateLimitMap[clientIP]
	if !exists {
		attempts = []time.Time{}
	}

	// Filter out attempts older than 1 minute
	var recentAttempts []time.Time
	for _, t := range attempts {
		if t.After(oneMinuteAgo) {
			recentAttempts = append(recentAttempts, t)
		}
	}

	// Check if rate limit exceeded
	if len(recentAttempts) >= c.rateLimitCount {
		return false // Rate limit exceeded
	}

	// Add current attempt
	recentAttempts = append(recentAttempts, now)
	c.rateLimitMap[clientIP] = recentAttempts

	return true // Connection allowed
}

// cleanupRateLimitMap periodically removes stale entries from the rate limit map
// to prevent memory leaks from IPs that never reconnect.
// Runs every 5 minutes and removes entries with no recent activity (>1 minute old).
func (c *websocketController) cleanupRateLimitMap() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.rateLimitMu.Lock()

		cutoff := time.Now().Add(-1 * time.Minute)
		cleanedCount := 0

		for ip, attempts := range c.rateLimitMap {
			// Filter attempts to keep only recent ones
			var recent []time.Time
			for _, t := range attempts {
				if t.After(cutoff) {
					recent = append(recent, t)
				}
			}

			// If no recent attempts, remove the entry entirely
			if len(recent) == 0 {
				delete(c.rateLimitMap, ip)
				cleanedCount++
			} else if len(recent) < len(attempts) {
				// Update with only recent attempts if we filtered some out
				c.rateLimitMap[ip] = recent
			}
		}

		if cleanedCount > 0 {
			slog.Info("cleaned up stale rate limit entries",
				"removedCount", cleanedCount,
				"remainingCount", len(c.rateLimitMap))
		}

		c.rateLimitMu.Unlock()
	}
}

// getClientIP extracts the client IP address from the request
// Properly parses X-Forwarded-For to extract only the first (leftmost) IP
// to prevent rate limit bypass via header manipulation
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (parse only first IP)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// X-Forwarded-For format: "client, proxy1, proxy2"
		// Only trust the leftmost IP (actual client)
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	// Fall back to RemoteAddr (strip port if present)
	if idx := strings.LastIndex(r.RemoteAddr, ":"); idx != -1 {
		return r.RemoteAddr[:idx]
	}
	return r.RemoteAddr
}
