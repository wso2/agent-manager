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
)

// Transport defines an abstraction layer for protocol-independent message delivery.
// This interface allows the system to switch between different transport mechanisms
// (WebSocket, Server-Sent Events, gRPC, etc.) without modifying business logic.
type Transport interface {
	// Send delivers a message to the connected client
	Send(message []byte) error

	// Close terminates the transport connection gracefully
	Close(code int, reason string) error

	// SetReadDeadline sets the deadline for reading from the transport
	SetReadDeadline(deadline time.Time) error

	// SetWriteDeadline sets the deadline for writing to the transport
	SetWriteDeadline(deadline time.Time) error

	// EnablePongHandler configures automatic handling of pong frames for heartbeat
	EnablePongHandler(handler func(string) error)

	// SendPing sends a ping frame to test connection liveness
	SendPing() error
}

// DeliveryStats tracks event delivery statistics for a connection
type DeliveryStats struct {
	MessagesSent     int64
	MessagesReceived int64
	BytesSent        int64
	BytesReceived    int64
}
