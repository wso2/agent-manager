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
	"crypto/tls"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// GorillaTransport implements the Transport interface using gorilla/websocket
type GorillaTransport struct {
	dialer *websocket.Dialer
}

// NewGorillaTransport creates a new Gorilla WebSocket transport
func NewGorillaTransport() *GorillaTransport {
	return &GorillaTransport{
		dialer: &websocket.Dialer{
			HandshakeTimeout: 45 * time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: false,
			},
		},
	}
}

// Dial establishes a WebSocket connection to the specified URL
func (t *GorillaTransport) Dial(ctx context.Context, url string) (*Connection, error) {
	headers := http.Header{}
	headers.Add("User-Agent", "agent-manager-service/1.0")

	conn, _, err := t.dialer.DialContext(ctx, url, headers)
	if err != nil {
		return nil, err
	}

	// Set up ping/pong handlers
	conn.SetPingHandler(func(appData string) error {
		return conn.WriteControl(websocket.PongMessage, []byte(appData), time.Now().Add(10*time.Second))
	})

	return NewConnection(conn), nil
}

// Close closes a WebSocket connection
func (t *GorillaTransport) Close(conn *Connection) error {
	if conn == nil {
		return nil
	}
	return conn.Close()
}

// WithTLSConfig sets a custom TLS configuration
func (t *GorillaTransport) WithTLSConfig(config *tls.Config) *GorillaTransport {
	t.dialer.TLSClientConfig = config
	return t
}

// WithHandshakeTimeout sets a custom handshake timeout
func (t *GorillaTransport) WithHandshakeTimeout(timeout time.Duration) *GorillaTransport {
	t.dialer.HandshakeTimeout = timeout
	return t
}
