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

package websocket

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Manager handles the lifecycle of gateway WebSocket connections.
// It maintains an in-memory registry of active connections, manages heartbeats,
// and handles graceful/ungraceful disconnections.
type Manager struct {
	// connections maps gatewayID -> []*Connection
	// Supports multiple connections per gateway ID for clustering scenarios
	connections sync.Map

	// mu protects the connectionCount and maxConnections fields
	mu sync.RWMutex

	// connectionCount tracks the total number of active connections across all gateways
	connectionCount int

	// maxConnections enforces a limit on concurrent connections (default 1000)
	maxConnections int

	// heartbeatInterval specifies how often to send ping frames (default 20s)
	heartbeatInterval time.Duration

	// heartbeatTimeout specifies when to consider a connection dead (default 30s)
	heartbeatTimeout time.Duration

	// shutdownCtx is used to signal graceful shutdown to all connection goroutines
	shutdownCtx context.Context
	shutdownFn  context.CancelFunc

	// wg tracks active connection handler goroutines for graceful shutdown
	wg sync.WaitGroup
}

// ManagerConfig contains configuration parameters for the connection manager
type ManagerConfig struct {
	MaxConnections    int           // Maximum concurrent connections (default 1000)
	HeartbeatInterval time.Duration // Ping interval (default 20s)
	HeartbeatTimeout  time.Duration // Pong timeout (default 30s)
}

// DefaultManagerConfig returns sensible default configuration values
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		MaxConnections:    1000,
		HeartbeatInterval: 20 * time.Second,
		HeartbeatTimeout:  30 * time.Second,
	}
}

// NewManager creates a new connection manager with the provided configuration
func NewManager(config ManagerConfig) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		connections:       sync.Map{},
		connectionCount:   0,
		maxConnections:    config.MaxConnections,
		heartbeatInterval: config.HeartbeatInterval,
		heartbeatTimeout:  config.HeartbeatTimeout,
		shutdownCtx:       ctx,
		shutdownFn:        cancel,
	}
}

// Register adds a new connection to the registry and starts heartbeat monitoring.
// Returns an error if the maximum connection limit is reached.
func (m *Manager) Register(gatewayID string, transport Transport, authToken string) (*Connection, error) {
	// Check connection limit
	m.mu.Lock()
	if m.connectionCount >= m.maxConnections {
		m.mu.Unlock()
		return nil, fmt.Errorf("maximum connection limit reached (%d)", m.maxConnections)
	}
	m.connectionCount++
	m.mu.Unlock()

	// Create connection with unique connection ID
	connectionID := uuid.New().String()
	conn := NewConnection(gatewayID, connectionID, transport, authToken)

	// Add connection to registry
	connsInterface, _ := m.connections.LoadOrStore(gatewayID, []*Connection{})
	conns := connsInterface.([]*Connection)
	conns = append(conns, conn)
	m.connections.Store(gatewayID, conns)

	// Start heartbeat monitoring in background
	m.wg.Add(1)
	go m.monitorHeartbeat(conn)

	log.Printf("[INFO] Gateway connected: gatewayID=%s connectionID=%s totalConnections=%d",
		gatewayID, connectionID, m.GetConnectionCount())

	return conn, nil
}

// Unregister removes a connection from the registry and closes it gracefully.
// This method is idempotent - calling it multiple times is safe.
func (m *Manager) Unregister(gatewayID, connectionID string) {
	connsInterface, ok := m.connections.Load(gatewayID)
	if !ok {
		return // Gateway not found
	}

	conns := connsInterface.([]*Connection)
	var updatedConns []*Connection
	var removed *Connection

	// Filter out the connection to remove
	for _, conn := range conns {
		if conn.ConnectionID == connectionID {
			removed = conn
		} else {
			updatedConns = append(updatedConns, conn)
		}
	}

	if removed == nil {
		return // Connection not found
	}

	// Update or delete the gateway entry
	if len(updatedConns) == 0 {
		m.connections.Delete(gatewayID)
	} else {
		m.connections.Store(gatewayID, updatedConns)
	}

	// Close the connection gracefully
	if err := removed.Close(1000, "normal closure"); err != nil {
		log.Printf("[ERROR] Failed to close connection: gatewayID=%s connectionID=%s error=%v",
			gatewayID, connectionID, err)
	}

	// Decrement connection count
	m.mu.Lock()
	m.connectionCount--
	m.mu.Unlock()

	log.Printf("[INFO] Gateway disconnected: gatewayID=%s connectionID=%s totalConnections=%d",
		gatewayID, connectionID, m.GetConnectionCount())
}

// GetConnections retrieves all connections for a specific gateway ID.
// Returns an empty slice if the gateway has no active connections.
func (m *Manager) GetConnections(gatewayID string) []*Connection {
	connsInterface, ok := m.connections.Load(gatewayID)
	if !ok {
		return []*Connection{}
	}
	return connsInterface.([]*Connection)
}

// GetAllConnections returns all active connections across all gateways.
// Used by the stats API to provide operational visibility.
func (m *Manager) GetAllConnections() map[string][]*Connection {
	result := make(map[string][]*Connection)
	m.connections.Range(func(key, value interface{}) bool {
		gatewayID := key.(string)
		conns := value.([]*Connection)
		result[gatewayID] = conns
		return true // Continue iteration
	})
	return result
}

// GetConnectionCount returns the total number of active connections
func (m *Manager) GetConnectionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connectionCount
}

// monitorHeartbeat periodically sends ping frames and detects connection death.
// Runs in a background goroutine for each connection.
func (m *Manager) monitorHeartbeat(conn *Connection) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.heartbeatInterval)
	defer ticker.Stop()

	// Configure pong handler to update heartbeat timestamp
	conn.Transport.EnablePongHandler(func(appData string) error {
		conn.UpdateHeartbeat()
		return nil
	})

	for {
		select {
		case <-m.shutdownCtx.Done():
			// Graceful shutdown triggered
			return

		case <-ticker.C:
			// Check if connection is already closed
			if conn.IsClosed() {
				return
			}

			// Check for heartbeat timeout
			if time.Since(conn.GetLastHeartbeat()) > m.heartbeatTimeout {
				log.Printf("[WARN] Heartbeat timeout detected: gatewayID=%s connectionID=%s lastHeartbeat=%v",
					conn.GatewayID, conn.ConnectionID, conn.GetLastHeartbeat())
				m.Unregister(conn.GatewayID, conn.ConnectionID)
				return
			}

			// Send ping frame
			if err := conn.Transport.SendPing(); err != nil {
				log.Printf("[ERROR] Failed to send ping: gatewayID=%s connectionID=%s error=%v",
					conn.GatewayID, conn.ConnectionID, err)
				m.Unregister(conn.GatewayID, conn.ConnectionID)
				return
			}
		}
	}
}

// Shutdown gracefully closes all connections and stops heartbeat monitoring.
// Waits for all connection handler goroutines to exit before returning.
func (m *Manager) Shutdown() {
	log.Println("[INFO] Shutting down WebSocket manager...")

	// Signal shutdown to all monitoring goroutines
	m.shutdownFn()

	// Close all connections
	m.connections.Range(func(key, value interface{}) bool {
		gatewayID := key.(string)
		conns := value.([]*Connection)
		for _, conn := range conns {
			if err := conn.Close(1000, "server shutdown"); err != nil {
				log.Printf("[ERROR] Failed to close connection during shutdown: gatewayID=%s connectionID=%s error=%v",
					gatewayID, conn.ConnectionID, err)
			}
		}
		return true // Continue iteration
	})

	// Wait for all goroutines to exit
	m.wg.Wait()

	log.Println("[INFO] WebSocket manager shutdown complete")
}
