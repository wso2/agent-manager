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
	"sync"
	"time"
)

// Connection represents an active gateway connection with metadata and lifecycle management.
// This wrapper decouples connection management logic from the underlying transport protocol.
type Connection struct {
	// GatewayID identifies the gateway instance (UUID from gateway registration)
	GatewayID string

	// ConnectionID provides a unique identifier for this specific connection instance.
	// Used to distinguish between multiple connections from the same gateway (clustering).
	ConnectionID string

	// ConnectedAt records when the connection was established
	ConnectedAt time.Time

	// LastHeartbeat records the timestamp of the most recent heartbeat (pong) received.
	// Updated automatically by the pong handler to track connection liveness.
	LastHeartbeat time.Time

	// Transport provides the underlying protocol implementation for message delivery.
	// Abstraction allows swapping WebSocket for other protocols without changing business logic.
	Transport Transport

	// AuthToken stores the API key used to authenticate this connection.
	// Can be used for re-validation or audit logging.
	AuthToken string

	// DeliveryStats tracks event delivery statistics for this connection
	DeliveryStats *Stats

	// mu protects concurrent access to mutable fields (LastHeartbeat, closed state)
	mu sync.RWMutex

	// closed tracks whether the connection has been terminated
	closed bool
}

// NewConnection creates a new Connection wrapper with the provided parameters
func NewConnection(gatewayID, connectionID string, transport Transport, authToken string) *Connection {
	now := time.Now()
	return &Connection{
		GatewayID:     gatewayID,
		ConnectionID:  connectionID,
		ConnectedAt:   now,
		LastHeartbeat: now,
		Transport:     transport,
		AuthToken:     authToken,
		DeliveryStats: NewStats(),
		closed:        false,
	}
}

// Send delivers a message to the gateway through the underlying transport.
// This method is thread-safe and can be called concurrently.
func (c *Connection) Send(message []byte) error {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.closed {
		return ErrConnectionClosed
	}

	return c.Transport.Send(message)
}

// Close terminates the connection gracefully with a close code and reason.
// This method is idempotent - calling it multiple times is safe.
func (c *Connection) Close(code int, reason string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil // Already closed, no-op
	}

	c.closed = true
	return c.Transport.Close(code, reason)
}

// IsClosed returns true if the connection has been explicitly closed.
// Thread-safe for concurrent access.
func (c *Connection) IsClosed() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.closed
}

// UpdateHeartbeat records the current time as the last heartbeat timestamp.
// Called automatically by the pong handler when heartbeat frames are received.
func (c *Connection) UpdateHeartbeat() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.LastHeartbeat = time.Now()
}

// GetLastHeartbeat returns the timestamp of the most recent heartbeat.
// Used by the connection manager to detect stale/dead connections.
func (c *Connection) GetLastHeartbeat() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.LastHeartbeat
}

// ConnectionStatus represents the current state of a connection for monitoring
type ConnectionStatus struct {
	GatewayID     string    `json:"gatewayId"`
	ConnectionID  string    `json:"connectionId"`
	ConnectedAt   time.Time `json:"connectedAt"`
	LastHeartbeat time.Time `json:"lastHeartbeat"`
	Status        string    `json:"status"` // "connected", "stale", "closed"
}

// GetStatus returns the current connection status for monitoring and stats API
func (c *Connection) GetStatus(heartbeatTimeout time.Duration) ConnectionStatus {
	c.mu.RLock()
	defer c.mu.RUnlock()

	status := "connected"
	if c.closed {
		status = "closed"
	} else if time.Since(c.LastHeartbeat) > heartbeatTimeout {
		status = "stale"
	}

	return ConnectionStatus{
		GatewayID:     c.GatewayID,
		ConnectionID:  c.ConnectionID,
		ConnectedAt:   c.ConnectedAt,
		LastHeartbeat: c.LastHeartbeat,
		Status:        status,
	}
}

// Common connection errors
var (
	// ErrConnectionClosed is returned when attempting to send on a closed connection
	ErrConnectionClosed = &ConnectionError{Message: "connection is closed"}
)

// ConnectionError represents connection-specific errors
type ConnectionError struct {
	Message string
}

func (e *ConnectionError) Error() string {
	return e.Message
}
