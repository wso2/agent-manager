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
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
)

// Manager handles the lifecycle of gateway WebSocket connections.
// It maintains an in-memory registry of active connections, manages heartbeats,
// and handles graceful/ungraceful disconnections.
//
// When an EventHub is configured, Manager subscribes to it on Register and
// forwards received events to the local WebSocket connection. This allows any
// service instance to deliver events to gateways connected to a different pod.
type Manager struct {
	// connections maps gatewayID -> []*Connection
	connections sync.Map

	// registryMu serializes all map+count mutation paths (Register, Unregister) so that
	// LoadAndDelete/Store and the accompanying connectionCount adjustment are one atomic
	// unit. Without this, parallel reconnects for the same gateway can interleave their
	// map reads and count decrements, causing count drift or double-decrement.
	registryMu sync.Mutex

	// mu protects connectionCount
	mu sync.RWMutex

	connectionCount   int
	maxConnections    int
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration

	hub eventhub.EventHub

	shutdownCtx context.Context
	shutdownFn  context.CancelFunc

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
func NewManager(config ManagerConfig, hub eventhub.EventHub) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		maxConnections:    config.MaxConnections,
		heartbeatInterval: config.HeartbeatInterval,
		heartbeatTimeout:  config.HeartbeatTimeout,
		hub:               hub,
		shutdownCtx:       ctx,
		shutdownFn:        cancel,
	}
}

// Register adds a new connection to the registry and starts heartbeat monitoring.
// Returns an error if the maximum connection limit is reached.
//
// Any existing connections for the same gateway are evicted first (singleton per
// gateway). When kgateway reloads config it reconnects immediately; without eviction
// the stale upstream lingers and receives no heartbeat pings.
func (m *Manager) Register(gatewayID string, transport Transport, authToken string) (*Connection, error) {
	// registryMu ensures the LoadAndDelete → count update → Store sequence is atomic
	// against concurrent Register/Unregister calls for the same gateway.
	m.registryMu.Lock()

	// Snapshot existing connections for eviction. We close them after releasing the
	// lock so we don't hold registryMu during I/O.
	var evicted []*Connection
	if connsInterface, loaded := m.connections.LoadAndDelete(gatewayID); loaded {
		evicted = connsInterface.([]*Connection)
	}

	// Adjust the connection count: subtract evicted, add the new one, then range-check.
	m.mu.Lock()
	projected := m.connectionCount - len(evicted)
	if projected >= m.maxConnections {
		m.mu.Unlock()
		m.registryMu.Unlock()
		return nil, fmt.Errorf("maximum connection limit reached (%d)", m.maxConnections)
	}
	m.connectionCount = projected + 1
	m.mu.Unlock()

	connectionID := uuid.New().String()
	conn := NewConnection(gatewayID, connectionID, transport, authToken)

	// Store as the sole active connection for this gateway (singleton per gateway).
	m.connections.Store(gatewayID, []*Connection{conn})

	m.registryMu.Unlock()

	// Unsubscribe and close evicted connections outside the lock.
	for _, old := range evicted {
		if m.hub != nil && old.eventSub != nil {
			if err := m.hub.Unsubscribe(gatewayID, old.eventSub); err != nil {
				slog.Error("Failed to unsubscribe evicted connection from EventHub",
					"gatewayID", gatewayID, "connectionID", old.ConnectionID, "error", err)
			}
		}
		_ = old.Close(1000, "superseded by new connection")
		log.Printf("[INFO] Evicted superseded connection: gatewayID=%s connectionID=%s",
			gatewayID, old.ConnectionID)
	}

	m.wg.Add(1)
	go m.monitorHeartbeat(conn)

	// Subscribe to the EventHub and forward events to this connection.
	if m.hub != nil {
		ch, err := m.hub.Subscribe(gatewayID)
		if err != nil {
			slog.Error("Failed to subscribe to EventHub for gateway",
				"gatewayID", gatewayID, "connectionID", connectionID, "error", err)
			m.Unregister(gatewayID, connectionID)
			return nil, fmt.Errorf("failed to subscribe to EventHub for gateway %s: %w", gatewayID, err)
		}
		conn.eventSub = ch
		m.wg.Add(1)
		go m.forwardEvents(conn, ch)
	}

	if len(evicted) > 0 {
		log.Printf("[INFO] Gateway reconnected (evicted %d stale connection(s)): gatewayID=%s connectionID=%s totalConnections=%d",
			len(evicted), gatewayID, connectionID, m.GetConnectionCount())
	} else {
		log.Printf("[INFO] Gateway connected: gatewayID=%s connectionID=%s totalConnections=%d",
			gatewayID, connectionID, m.GetConnectionCount())
	}

	return conn, nil
}

// forwardEvents reads events from the EventHub subscription channel and sends
// them to the WebSocket connection. It exits when the channel is closed or
// the connection is closed.
func (m *Manager) forwardEvents(conn *Connection, ch <-chan eventhub.Event) {
	defer m.wg.Done()
	for {
		select {
		case evt, ok := <-ch:
			if !ok {
				return
			}
			if conn.IsClosed() {
				return
			}
			// EventData already contains the full GatewayEventDTO JSON serialized by
			// GatewayEventsService.broadcastEvent — send it directly.
			payload := []byte(evt.EventData)
			if len(payload) == 0 {
				payload, _ = json.Marshal(evt)
			}
			if err := conn.Send(payload); err != nil {
				slog.Error("Failed to forward EventHub event to gateway",
					"gatewayID", conn.GatewayID,
					"connectionID", conn.ConnectionID,
					"event_type", string(evt.EventType),
					"error", err)
				conn.DeliveryStats.IncrementFailed(fmt.Sprintf("forward error: %v", err))
			} else {
				slog.Debug("Forwarded event to gateway WebSocket",
					"gatewayID", conn.GatewayID,
					"connectionID", conn.ConnectionID,
					"event_type", string(evt.EventType),
					"event_id", evt.EventID)
				conn.DeliveryStats.IncrementTotalSent()
			}
		case <-m.shutdownCtx.Done():
			return
		}
	}
}

// Unregister removes a connection from the registry and closes it gracefully.
// This method is idempotent - calling it multiple times is safe.
func (m *Manager) Unregister(gatewayID, connectionID string) {
	m.registryMu.Lock()

	connsInterface, ok := m.connections.Load(gatewayID)
	if !ok {
		m.registryMu.Unlock()
		return // Gateway not found
	}

	conns := connsInterface.([]*Connection)
	var updatedConns []*Connection
	var removed *Connection

	for _, conn := range conns {
		if conn.ConnectionID == connectionID {
			removed = conn
		} else {
			updatedConns = append(updatedConns, conn)
		}
	}

	if removed == nil {
		m.registryMu.Unlock()
		return // Connection not found
	}

	if len(updatedConns) == 0 {
		m.connections.Delete(gatewayID)
	} else {
		m.connections.Store(gatewayID, updatedConns)
	}

	m.mu.Lock()
	m.connectionCount--
	m.mu.Unlock()

	m.registryMu.Unlock()

	// Unsubscribe from the EventHub before closing the connection so the forwardEvents
	// goroutine exits cleanly (channel close signals it) rather than leaking until shutdown.
	if m.hub != nil && removed.eventSub != nil {
		if err := m.hub.Unsubscribe(gatewayID, removed.eventSub); err != nil {
			slog.Error("Failed to unsubscribe from EventHub",
				"gatewayID", gatewayID, "connectionID", connectionID, "error", err)
		}
	}

	// Close the connection outside the lock — Close involves I/O and must not
	// block other Register/Unregister callers.
	if err := removed.Close(1000, "normal closure"); err != nil {
		log.Printf("[DEBUG] Connection close returned error: gatewayID=%s connectionID=%s error=%v",
			gatewayID, connectionID, err)
	}

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
		return true
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

	conn.Transport.EnablePongHandler(func(appData string) error {
		conn.UpdateHeartbeat()
		return nil
	})

	for {
		select {
		case <-m.shutdownCtx.Done():
			return

		case <-ticker.C:
			if conn.IsClosed() {
				return
			}

			if time.Since(conn.GetLastHeartbeat()) > m.heartbeatTimeout {
				log.Printf("[WARN] Heartbeat timeout detected: gatewayID=%s connectionID=%s lastHeartbeat=%v",
					conn.GatewayID, conn.ConnectionID, conn.GetLastHeartbeat())
				m.Unregister(conn.GatewayID, conn.ConnectionID)
				return
			}

			if err := conn.SendPing(); err != nil {
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

	m.shutdownFn()

	m.connections.Range(func(key, value interface{}) bool {
		gatewayID := key.(string)
		conns := value.([]*Connection)
		for _, conn := range conns {
			if err := conn.Close(1000, "server shutdown"); err != nil {
				log.Printf("[DEBUG] Connection close returned error during shutdown: gatewayID=%s connectionID=%s error=%v",
					gatewayID, conn.ConnectionID, err)
			}
		}
		return true
	})

	m.wg.Wait()

	log.Println("[INFO] WebSocket manager shutdown complete")
}
