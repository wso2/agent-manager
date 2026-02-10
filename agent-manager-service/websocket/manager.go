package websocket

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Manager manages WebSocket connections for gateways
type Manager struct {
	connections       sync.Map
	mu                sync.RWMutex
	connectionCount   int
	maxConnections    int
	heartbeatInterval time.Duration
	heartbeatTimeout  time.Duration
	shutdownCtx       context.Context
	shutdownFn        context.CancelFunc
	wg                sync.WaitGroup
	logger            *slog.Logger
}

// ManagerConfig holds configuration for the WebSocket manager
type ManagerConfig struct {
	MaxConnections    int
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
}

// DefaultManagerConfig returns the default manager configuration
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		MaxConnections:    1000,
		HeartbeatInterval: 20 * time.Second,
		HeartbeatTimeout:  30 * time.Second,
	}
}

// NewManager creates a new WebSocket manager
func NewManager(config ManagerConfig, logger *slog.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		connections:       sync.Map{},
		connectionCount:   0,
		maxConnections:    config.MaxConnections,
		heartbeatInterval: config.HeartbeatInterval,
		heartbeatTimeout:  config.HeartbeatTimeout,
		shutdownCtx:       ctx,
		shutdownFn:        cancel,
		logger:            logger,
	}
}

// Register registers a new WebSocket connection for a gateway
func (m *Manager) Register(gatewayID string, transport Transport, authToken string) (*Connection, error) {
	m.mu.Lock()
	if m.connectionCount >= m.maxConnections {
		m.mu.Unlock()
		return nil, fmt.Errorf("maximum connection limit reached (%d)", m.maxConnections)
	}
	m.connectionCount++
	m.mu.Unlock()

	connectionID := uuid.New().String()
	conn := NewConnection(gatewayID, connectionID, transport, authToken)

	// Load existing connections for this gateway
	value, _ := m.connections.LoadOrStore(gatewayID, []*Connection{})
	conns := value.([]*Connection)

	// Add new connection (requires locking the slice)
	m.mu.Lock()
	conns = append(conns, conn)
	m.connections.Store(gatewayID, conns)
	m.mu.Unlock()

	// Start heartbeat monitoring
	m.wg.Add(1)
	go m.monitorHeartbeat(conn)

	m.logger.Info("Gateway connected",
		"gatewayId", gatewayID,
		"connectionId", connectionID,
		"totalConnections", m.GetConnectionCount(),
	)

	return conn, nil
}

// Unregister unregisters a WebSocket connection
func (m *Manager) Unregister(gatewayID, connectionID string) {
	value, ok := m.connections.Load(gatewayID)
	if !ok {
		return
	}

	conns := value.([]*Connection)
	var updatedConns []*Connection
	var removed *Connection

	m.mu.Lock()
	for _, conn := range conns {
		if conn.ConnectionID == connectionID {
			removed = conn
		} else {
			updatedConns = append(updatedConns, conn)
		}
	}
	m.mu.Unlock()

	if removed == nil {
		return
	}

	// Update the connections map
	if len(updatedConns) == 0 {
		m.connections.Delete(gatewayID)
	} else {
		m.connections.Store(gatewayID, updatedConns)
	}

	// Close the connection
	if err := removed.Close(1000, "normal closure"); err != nil {
		m.logger.Error("Failed to close connection",
			"gatewayId", gatewayID,
			"connectionId", connectionID,
			"error", err,
		)
	}

	m.mu.Lock()
	m.connectionCount--
	m.mu.Unlock()

	m.logger.Info("Gateway disconnected",
		"gatewayId", gatewayID,
		"connectionId", connectionID,
		"totalConnections", m.GetConnectionCount(),
	)
}

// GetConnections returns all active connections for a gateway
func (m *Manager) GetConnections(gatewayID string) []*Connection {
	value, ok := m.connections.Load(gatewayID)
	if !ok {
		return []*Connection{}
	}
	return value.([]*Connection)
}

// GetConnectionCount returns the total number of active connections
func (m *Manager) GetConnectionCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.connectionCount
}

// GetAllGatewayIDs returns all gateway IDs with active connections
func (m *Manager) GetAllGatewayIDs() []string {
	var gatewayIDs []string
	m.connections.Range(func(key, value interface{}) bool {
		gatewayIDs = append(gatewayIDs, key.(string))
		return true
	})
	return gatewayIDs
}

// monitorHeartbeat monitors the heartbeat for a connection
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

			lastHeartbeat := conn.GetLastHeartbeat()
			timeSinceHeartbeat := time.Since(lastHeartbeat)

			if timeSinceHeartbeat > m.heartbeatTimeout {
				m.logger.Warn("Heartbeat timeout",
					"gatewayId", conn.GatewayID,
					"connectionId", conn.ConnectionID,
					"elapsed", timeSinceHeartbeat.Seconds(),
				)
				m.Unregister(conn.GatewayID, conn.ConnectionID)
				return
			}

			if err := conn.Transport.SendPing(); err != nil {
				m.logger.Error("Failed to send ping",
					"gatewayId", conn.GatewayID,
					"connectionId", conn.ConnectionID,
					"error", err,
				)
				m.Unregister(conn.GatewayID, conn.ConnectionID)
				return
			}
		}
	}
}

// Shutdown gracefully shuts down the WebSocket manager
func (m *Manager) Shutdown() {
	m.logger.Info("Shutting down WebSocket manager", "activeConnections", m.GetConnectionCount())
	m.shutdownFn()

	// Close all connections
	m.connections.Range(func(key, value interface{}) bool {
		gatewayID := key.(string)
		conns := value.([]*Connection)
		for _, conn := range conns {
			if err := conn.Close(1000, "server shutdown"); err != nil {
				m.logger.Error("Failed to close connection during shutdown",
					"gatewayId", gatewayID,
					"connectionId", conn.ConnectionID,
					"error", err,
				)
			}
		}
		return true
	})

	// Wait for all goroutines to finish
	m.wg.Wait()

	m.logger.Info("WebSocket manager shutdown complete")
}

// GetStats returns aggregate statistics for all connections
func (m *Manager) GetStats() map[string]interface{} {
	totalConnections := m.GetConnectionCount()
	totalGateways := 0
	var totalSent, totalFailed int64

	m.connections.Range(func(key, value interface{}) bool {
		totalGateways++
		conns := value.([]*Connection)
		for _, conn := range conns {
			sent, failed, _, _ := conn.GetStats()
			totalSent += sent
			totalFailed += failed
		}
		return true
	})

	return map[string]interface{}{
		"totalConnections":  totalConnections,
		"totalGateways":     totalGateways,
		"totalEventsSent":   totalSent,
		"totalFailedEvents": totalFailed,
	}
}
