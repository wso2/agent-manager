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
	"encoding/json"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Connection wraps a WebSocket connection with additional metadata
type Connection struct {
	conn      *websocket.Conn
	mu        sync.Mutex
	lastPing  time.Time
	lastPong  time.Time
	connected bool
}

// NewConnection creates a new WebSocket connection wrapper
func NewConnection(conn *websocket.Conn) *Connection {
	now := time.Now()
	return &Connection{
		conn:      conn,
		lastPing:  now,
		lastPong:  now,
		connected: true,
	}
}

// WriteMessage writes a message to the WebSocket connection
func (c *Connection) WriteMessage(msg *Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return websocket.ErrCloseSent
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	return c.conn.WriteMessage(websocket.TextMessage, data)
}

// ReadMessage reads a message from the WebSocket connection
func (c *Connection) ReadMessage() (*Message, error) {
	_, data, err := c.conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, err
	}

	return &msg, nil
}

// Close closes the WebSocket connection
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		return nil
	}

	c.connected = false
	return c.conn.Close()
}

// IsConnected returns whether the connection is active
func (c *Connection) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// UpdateLastPing updates the last ping timestamp
func (c *Connection) UpdateLastPing() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastPing = time.Now()
}

// UpdateLastPong updates the last pong timestamp
func (c *Connection) UpdateLastPong() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastPong = time.Now()
}

// GetLastPong returns the last pong timestamp
func (c *Connection) GetLastPong() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.lastPong
}
