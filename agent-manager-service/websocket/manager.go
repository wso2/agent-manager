/*
 *  Copyright (c) 2026, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package websocket

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"
)

// Manager manages WebSocket connections and message broadcasting
type Manager struct {
	connections map[string]*Connection // gatewayID -> Connection
	mu          sync.RWMutex
	logger      *slog.Logger
	transport   Transport
}

// Transport defines the interface for WebSocket transport layer
type Transport interface {
	Dial(ctx context.Context, url string) (*Connection, error)
	Close(conn *Connection) error
}

// Message represents a WebSocket message
type Message struct {
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
	Timestamp time.Time       `json:"timestamp"`
}

// NewManager creates a new WebSocket manager
func NewManager(logger *slog.Logger, transport Transport) *Manager {
	return &Manager{
		connections: make(map[string]*Connection),
		logger:      logger,
		transport:   transport,
	}
}

// Connect establishes a WebSocket connection to a gateway
func (m *Manager) Connect(ctx context.Context, gatewayID, url string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if connection already exists
	if _, exists := m.connections[gatewayID]; exists {
		m.logger.Info("Connection already exists for gateway", "gatewayID", gatewayID)
		return nil
	}

	// Establish new connection
	conn, err := m.transport.Dial(ctx, url)
	if err != nil {
		return err
	}

	m.connections[gatewayID] = conn
	m.logger.Info("WebSocket connection established", "gatewayID", gatewayID)

	// Start reading messages in background
	go m.readMessages(gatewayID, conn)

	return nil
}

// Disconnect closes the WebSocket connection to a gateway
func (m *Manager) Disconnect(gatewayID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	conn, exists := m.connections[gatewayID]
	if !exists {
		m.logger.Warn("Connection not found for gateway", "gatewayID", gatewayID)
		return nil
	}

	if err := m.transport.Close(conn); err != nil {
		m.logger.Error("Failed to close connection", "gatewayID", gatewayID, "error", err)
		return err
	}

	delete(m.connections, gatewayID)
	m.logger.Info("WebSocket connection closed", "gatewayID", gatewayID)

	return nil
}

// SendMessage sends a message to a specific gateway
func (m *Manager) SendMessage(gatewayID string, msg *Message) error {
	m.mu.RLock()
	conn, exists := m.connections[gatewayID]
	m.mu.RUnlock()

	if !exists {
		m.logger.Warn("Connection not found for gateway", "gatewayID", gatewayID)
		return nil
	}

	return conn.WriteMessage(msg)
}

// BroadcastMessage sends a message to all connected gateways
func (m *Manager) BroadcastMessage(msg *Message) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for gatewayID, conn := range m.connections {
		if err := conn.WriteMessage(msg); err != nil {
			m.logger.Error("Failed to send message to gateway", "gatewayID", gatewayID, "error", err)
		}
	}
}

// readMessages reads messages from a WebSocket connection
func (m *Manager) readMessages(gatewayID string, conn *Connection) {
	for {
		msg, err := conn.ReadMessage()
		if err != nil {
			m.logger.Error("Failed to read message", "gatewayID", gatewayID, "error", err)
			m.Disconnect(gatewayID)
			return
		}

		m.logger.Debug("Received message from gateway", "gatewayID", gatewayID, "messageType", msg.Type)

		// Handle message based on type
		m.handleMessage(gatewayID, msg)
	}
}

// handleMessage processes incoming messages from gateways
func (m *Manager) handleMessage(gatewayID string, msg *Message) {
	switch msg.Type {
	case "heartbeat":
		m.handleHeartbeat(gatewayID)
	case "event":
		m.handleEvent(gatewayID, msg.Payload)
	default:
		m.logger.Warn("Unknown message type", "gatewayID", gatewayID, "type", msg.Type)
	}
}

// handleHeartbeat processes heartbeat messages
func (m *Manager) handleHeartbeat(gatewayID string) {
	m.logger.Debug("Received heartbeat", "gatewayID", gatewayID)
	// Send heartbeat response
	m.SendMessage(gatewayID, &Message{
		Type:      "heartbeat_ack",
		Timestamp: time.Now(),
	})
}

// handleEvent processes event messages
func (m *Manager) handleEvent(gatewayID string, payload json.RawMessage) {
	m.logger.Info("Received event from gateway", "gatewayID", gatewayID)
	// TODO: Process event payload
}

// GetConnection retrieves a WebSocket connection by gateway ID
func (m *Manager) GetConnection(gatewayID string) (*Connection, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	conn, exists := m.connections[gatewayID]
	return conn, exists
}

// Close closes all WebSocket connections
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for gatewayID, conn := range m.connections {
		if err := m.transport.Close(conn); err != nil {
			m.logger.Error("Failed to close connection", "gatewayID", gatewayID, "error", err)
		}
	}

	m.connections = make(map[string]*Connection)
	return nil
}
