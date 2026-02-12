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
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketTransport implements the Transport interface using gorilla/websocket.
// This provides the concrete WebSocket protocol implementation while isolating
// WebSocket-specific code from business logic.
type WebSocketTransport struct {
	conn *websocket.Conn
}

// NewWebSocketTransport creates a new WebSocket transport wrapper
func NewWebSocketTransport(conn *websocket.Conn) Transport {
	return &WebSocketTransport{
		conn: conn,
	}
}

// Send delivers a message to the WebSocket client as a text frame
func (t *WebSocketTransport) Send(message []byte) error {
	return t.conn.WriteMessage(websocket.TextMessage, message)
}

// Close terminates the WebSocket connection with a close frame
func (t *WebSocketTransport) Close(code int, reason string) error {
	closeMessage := websocket.FormatCloseMessage(code, reason)
	err := t.conn.WriteMessage(websocket.CloseMessage, closeMessage)
	if err != nil {
		return err
	}
	// Close the underlying connection
	return t.conn.Close()
}

// SetReadDeadline sets the deadline for read operations
func (t *WebSocketTransport) SetReadDeadline(deadline time.Time) error {
	return t.conn.SetReadDeadline(deadline)
}

// SetWriteDeadline sets the deadline for write operations
func (t *WebSocketTransport) SetWriteDeadline(deadline time.Time) error {
	return t.conn.SetWriteDeadline(deadline)
}

// EnablePongHandler configures the automatic pong frame handler
func (t *WebSocketTransport) EnablePongHandler(handler func(string) error) {
	t.conn.SetPongHandler(handler)
}

// SendPing sends a WebSocket ping frame to test connection liveness
func (t *WebSocketTransport) SendPing() error {
	return t.conn.WriteMessage(websocket.PingMessage, []byte{})
}

// ReadMessage reads the next message from the WebSocket connection
func (t *WebSocketTransport) ReadMessage() (messageType int, payload []byte, err error) {
	return t.conn.ReadMessage()
}
