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
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gatewaytoken"
)

// chanHub hands each subscription a channel the test feeds.
type chanHub struct {
	eventhub.EventHub
	mu   sync.Mutex
	subs map[string]chan eventhub.Event
}

func (h *chanHub) Subscribe(gatewayID string) (<-chan eventhub.Event, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch := make(chan eventhub.Event, 4)
	h.subs[gatewayID] = ch
	return ch, nil
}

func (h *chanHub) Unsubscribe(gatewayID string, _ <-chan eventhub.Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.subs[gatewayID]; ok {
		close(ch)
		delete(h.subs, gatewayID)
	}
	return nil
}

func (h *chanHub) send(gatewayID string, e eventhub.Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subs[gatewayID] <- e
}

// recordingTransport records what the manager sends and whether it closed.
type recordingTransport struct {
	mu     sync.Mutex
	sent   [][]byte
	closed bool
}

func (t *recordingTransport) Send(m []byte) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.sent = append(t.sent, m)
	return nil
}

func (t *recordingTransport) Close(int, string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.closed = true
	return nil
}

func (t *recordingTransport) isClosed() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.closed
}

func (t *recordingTransport) sentCount() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.sent)
}

func (*recordingTransport) SetReadDeadline(time.Time) error      { return nil }
func (*recordingTransport) SetWriteDeadline(time.Time) error     { return nil }
func (*recordingTransport) EnablePongHandler(func(string) error) {}
func (*recordingTransport) SendPing() error                      { return nil }

// Revoking a gateway's token closes the socket that authenticated with it,
// and is never forwarded to the controller; revoking another token of the
// same gateway leaves the socket open.
func TestRevokedTokenClosesTheSocket(t *testing.T) {
	hub := &chanHub{subs: map[string]chan eventhub.Event{}}
	m := NewManager(ManagerConfig{MaxConnections: 10, HeartbeatInterval: time.Hour, HeartbeatTimeout: time.Hour}, hub)
	t.Cleanup(m.Shutdown)
	token, err := gatewaytoken.New()
	require.NoError(t, err)
	other, err := gatewaytoken.New()
	require.NoError(t, err)
	prefix, err := gatewaytoken.Prefix(token)
	require.NoError(t, err)
	otherPrefix, err := gatewaytoken.Prefix(other)
	require.NoError(t, err)

	tr := &recordingTransport{}
	_, err = m.Register("gw-1", tr, token)
	require.NoError(t, err)

	hub.send("gw-1", eventhub.Event{EventType: TokenRevokedEvent, EntityID: otherPrefix})
	time.Sleep(50 * time.Millisecond)
	assert.False(t, tr.isClosed(), "another token's revocation leaves the socket open")
	assert.Equal(t, 1, m.GetConnectionCount())

	hub.send("gw-1", eventhub.Event{EventType: TokenRevokedEvent, EntityID: prefix})
	assert.Eventually(t, tr.isClosed, time.Second, 10*time.Millisecond)
	assert.Eventually(t, func() bool { return m.GetConnectionCount() == 0 }, time.Second, 10*time.Millisecond)
	assert.Zero(t, tr.sentCount(), "a revocation is never sent to the controller")
}
