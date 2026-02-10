package websocket

import (
	"fmt"
	"sync"
	"time"
)

// Connection wraps a WebSocket connection with metadata and statistics
type Connection struct {
	GatewayID     string
	ConnectionID  string
	ConnectedAt   time.Time
	LastHeartbeat time.Time
	Transport     Transport
	AuthToken     string
	DeliveryStats *DeliveryStats
	mu            sync.RWMutex
	closed        bool
}

// NewConnection creates a new WebSocket connection wrapper
func NewConnection(gatewayID, connectionID string, transport Transport, authToken string) *Connection {
	now := time.Now()
	return &Connection{
		GatewayID:     gatewayID,
		ConnectionID:  connectionID,
		ConnectedAt:   now,
		LastHeartbeat: now,
		Transport:     transport,
		AuthToken:     authToken,
		DeliveryStats: &DeliveryStats{},
		closed:        false,
	}
}

// Send sends a message through the WebSocket connection
func (c *Connection) Send(message []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.closed {
		return ErrConnectionClosed
	}
	return c.Transport.Send(message)
}

// Close closes the WebSocket connection with a status code and reason
func (c *Connection) Close(code int, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return nil
	}
	c.closed = true
	return c.Transport.Close(code, reason)
}

// IsClosed returns whether the connection is closed
func (c *Connection) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// UpdateHeartbeat updates the last heartbeat timestamp
func (c *Connection) UpdateHeartbeat() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastHeartbeat = time.Now()
}

// GetLastHeartbeat returns the last heartbeat timestamp
func (c *Connection) GetLastHeartbeat() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.LastHeartbeat
}

// ErrConnectionClosed is returned when attempting to operate on a closed connection
var ErrConnectionClosed = &ConnectionError{Message: "connection is closed"}

// ConnectionError represents a connection-level error
type ConnectionError struct {
	Message string
}

func (e *ConnectionError) Error() string {
	return e.Message
}

// IsConnectionError checks if an error is a ConnectionError
func IsConnectionError(err error) bool {
	_, ok := err.(*ConnectionError)
	return ok
}

// GetConnectionInfo returns connection information as a map for logging
func (c *Connection) GetConnectionInfo() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return map[string]interface{}{
		"gatewayId":     c.GatewayID,
		"connectionId":  c.ConnectionID,
		"connectedAt":   c.ConnectedAt.Format(time.RFC3339),
		"lastHeartbeat": c.LastHeartbeat.Format(time.RFC3339),
		"closed":        c.closed,
	}
}

// GetStats returns current delivery statistics
func (c *Connection) GetStats() (totalSent, failed int64, lastFailureTime time.Time, lastFailureReason string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	totalSent = c.DeliveryStats.GetTotalSent()
	failed = c.DeliveryStats.GetFailedDeliveries()
	lastFailureTime, lastFailureReason = c.DeliveryStats.GetLastFailure()
	return
}

// String returns a string representation of the connection
func (c *Connection) String() string {
	return fmt.Sprintf("Connection{gatewayId=%s, connectionId=%s, closed=%v}", c.GatewayID, c.ConnectionID, c.closed)
}
