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
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// newTestConnection upgrades a loopback WebSocket and wraps the server side in
// a Connection, the way the gateway WebSocket controller does. The returned
// client connection is the gateway's end.
func newTestConnection(t *testing.T) (*Connection, *websocket.Conn) {
	t.Helper()
	serverConns := make(chan *websocket.Conn, 1)
	upgrader := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Errorf("upgrade: %v", err)
			return
		}
		serverConns <- c
	}))
	t.Cleanup(srv.Close)

	client, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	select {
	case server := <-serverConns:
		conn := NewConnection("gateway-1", "connection-1", NewWebSocketTransport(server), "")
		t.Cleanup(func() { _ = conn.Close(websocket.CloseNormalClosure, "test done") })
		// Cleanups run last-registered first: close the raw socket before conn.Close, so a
		// write still blocked on a stalled gateway returns instead of hanging the cleanup.
		t.Cleanup(func() { _ = server.Close() })
		return conn, client
	case <-time.After(5 * time.Second):
		t.Fatal("server side of the WebSocket never connected")
		return nil, nil
	}
}

// TestConnection_ConcurrentSendAndPing covers the gateway connection's real
// writers running at once: event forwarding and the connect ack (Send) and the
// heartbeat (SendPing). gorilla/websocket allows one concurrent writer; two
// overlapping writes panic with "concurrent write to websocket connection",
// which crashes agent-manager-service because those goroutines do not recover.
func TestConnection_ConcurrentSendAndPing(t *testing.T) {
	conn, client := newTestConnection(t)

	const senders, perSender = 8, 200
	var received atomic.Int64
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		for received.Load() < senders*perSender {
			msgType, _, err := client.ReadMessage()
			if err != nil {
				return
			}
			if msgType == websocket.TextMessage {
				received.Add(1)
			}
		}
	}()

	var (
		wg       sync.WaitGroup
		panicsMu sync.Mutex
		panics   []string
		writeErr atomic.Value
	)
	guard := func(fn func()) {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				panicsMu.Lock()
				panics = append(panics, fmt.Sprint(r))
				panicsMu.Unlock()
			}
		}()
		fn()
	}

	stopPings := make(chan struct{})
	wg.Add(1)
	go guard(func() {
		for {
			select {
			case <-stopPings:
				return
			default:
			}
			if err := conn.SendPing(); err != nil {
				writeErr.Store(fmt.Errorf("ping: %w", err))
				return
			}
		}
	})

	var sendWG sync.WaitGroup
	for i := 0; i < senders; i++ {
		wg.Add(1)
		sendWG.Add(1)
		go guard(func() {
			defer sendWG.Done()
			for j := 0; j < perSender; j++ {
				if err := conn.Send([]byte(`{"type":"event"}`)); err != nil {
					writeErr.Store(fmt.Errorf("send: %w", err))
					return
				}
			}
		})
	}
	sendWG.Wait()
	close(stopPings)
	wg.Wait()

	if len(panics) > 0 {
		t.Fatalf("concurrent writers panicked %d time(s); first: %s", len(panics), panics[0])
	}
	if err, _ := writeErr.Load().(error); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	select {
	case <-readDone:
	case <-time.After(10 * time.Second):
	}
	if got := received.Load(); got != senders*perSender {
		t.Fatalf("gateway received %d of %d messages", got, senders*perSender)
	}
}

// TestConnection_StalledGatewayWritesTimeOut covers why each write carries a deadline:
// writes are serialized, so a write blocked on a gateway that stopped reading would
// otherwise hold every other writer, including the heartbeat that detects the dead
// connection. Writes to a stalled gateway must fail instead of blocking forever.
func TestConnection_StalledGatewayWritesTimeOut(t *testing.T) {
	previous := writeWait
	writeWait = 200 * time.Millisecond
	t.Cleanup(func() { writeWait = previous })

	conn, _ := newTestConnection(t) // the gateway end never reads

	payload := make([]byte, 1<<20)
	failed := make(chan error, 1)
	go func() {
		for {
			if err := conn.Send(payload); err != nil {
				failed <- err
				return
			}
		}
	}()

	select {
	case err := <-failed:
		if !strings.Contains(err.Error(), "timeout") {
			t.Fatalf("expected a write timeout, got: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Send to a stalled gateway blocked instead of timing out")
	}

	pingDone := make(chan error, 1)
	go func() { pingDone <- conn.SendPing() }()
	select {
	case <-pingDone:
	case <-time.After(5 * time.Second):
		t.Fatal("SendPing stayed blocked behind a stalled write")
	}
}
