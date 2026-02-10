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
	"time"

	"github.com/gorilla/websocket"
)

// WebSocketTransport implements the Transport interface for WebSocket connections
type WebSocketTransport struct {
	conn *websocket.Conn
}

// NewWebSocketTransport creates a new WebSocket transport from a gorilla WebSocket connection
func NewWebSocketTransport(conn *websocket.Conn) Transport {
	return &WebSocketTransport{conn: conn}
}

// Send sends a text message through the WebSocket connection
func (t *WebSocketTransport) Send(message []byte) error {
	return t.conn.WriteMessage(websocket.TextMessage, message)
}

// Close closes the WebSocket connection with a status code and reason
func (t *WebSocketTransport) Close(code int, reason string) error {
	closeMessage := websocket.FormatCloseMessage(code, reason)
	err := t.conn.WriteMessage(websocket.CloseMessage, closeMessage)
	if err != nil {
		return err
	}
	return t.conn.Close()
}

// SetReadDeadline sets the read deadline for the WebSocket connection
func (t *WebSocketTransport) SetReadDeadline(deadline time.Time) error {
	return t.conn.SetReadDeadline(deadline)
}

// SetWriteDeadline sets the write deadline for the WebSocket connection
func (t *WebSocketTransport) SetWriteDeadline(deadline time.Time) error {
	return t.conn.SetWriteDeadline(deadline)
}

// EnablePongHandler sets the handler for pong messages
func (t *WebSocketTransport) EnablePongHandler(handler func(string) error) {
	t.conn.SetPongHandler(handler)
}

// SendPing sends a ping message through the WebSocket connection
func (t *WebSocketTransport) SendPing() error {
	return t.conn.WriteMessage(websocket.PingMessage, []byte{})
}

// ReadMessage reads a message from the WebSocket connection
func (t *WebSocketTransport) ReadMessage() (int, []byte, error) {
	return t.conn.ReadMessage()
}
