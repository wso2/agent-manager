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

	"github.com/gorilla/websocket"
)

// writeWait bounds a single write to the gateway. Writes are serialized, so without a
// deadline one write to a stalled gateway would also block every other writer, including
// the heartbeat that detects dead connections. It must stay below the heartbeat timeout.
var writeWait = 10 * time.Second

// WebSocketTransport implements the Transport interface using gorilla/websocket.
// This provides the concrete WebSocket protocol implementation while isolating
// WebSocket-specific code from business logic.
type WebSocketTransport struct {
	conn *websocket.Conn

	// writeMu serializes writes. gorilla/websocket supports one concurrent writer and
	// panics on overlapping writes, while event forwarding, the connect ack and the
	// heartbeat all write to the same connection from different goroutines.
	writeMu sync.Mutex
}

// NewWebSocketTransport creates a new WebSocket transport wrapper
func NewWebSocketTransport(conn *websocket.Conn) Transport {
	return &WebSocketTransport{
		conn: conn,
	}
}

// write sends one frame while holding writeMu, with a per-write deadline.
func (t *WebSocketTransport) write(messageType int, data []byte) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if err := t.conn.SetWriteDeadline(time.Now().Add(writeWait)); err != nil {
		return err
	}
	return t.conn.WriteMessage(messageType, data)
}

// Send delivers a message to the WebSocket client as a text frame
func (t *WebSocketTransport) Send(message []byte) error {
	return t.write(websocket.TextMessage, message)
}

// Close terminates the WebSocket connection with a close frame
func (t *WebSocketTransport) Close(code int, reason string) error {
	closeMessage := websocket.FormatCloseMessage(code, reason)
	if err := t.write(websocket.CloseMessage, closeMessage); err != nil {
		// "close sent" means a close frame was already written — the connection
		// is already shutting down, so we just need to close the underlying conn.
		if !websocket.IsCloseError(err) && err.Error() != "websocket: close sent" {
			_ = t.conn.Close()
			return err
		}
	}
	return t.conn.Close()
}

// SetReadDeadline sets the deadline for read operations
func (t *WebSocketTransport) SetReadDeadline(deadline time.Time) error {
	return t.conn.SetReadDeadline(deadline)
}

// SetWriteDeadline sets the deadline for write operations. gorilla/websocket counts it
// as a write method, so it takes writeMu too. Each write sets its own deadline.
func (t *WebSocketTransport) SetWriteDeadline(deadline time.Time) error {
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	return t.conn.SetWriteDeadline(deadline)
}

// EnablePongHandler configures the automatic pong frame handler
func (t *WebSocketTransport) EnablePongHandler(handler func(string) error) {
	t.conn.SetPongHandler(handler)
}

// SendPing sends a WebSocket ping frame to test connection liveness
func (t *WebSocketTransport) SendPing() error {
	return t.write(websocket.PingMessage, []byte{})
}

// ReadMessage reads the next message from the WebSocket connection
func (t *WebSocketTransport) ReadMessage() (messageType int, payload []byte, err error) {
	return t.conn.ReadMessage()
}
