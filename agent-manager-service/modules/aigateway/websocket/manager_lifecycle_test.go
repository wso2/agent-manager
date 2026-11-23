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
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	gorilla "github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/eventhub"
)

const lifecycleWait = 2 * time.Second

// wsPair is a real WebSocket: the server side wrapped as a Transport and the client side a gateway would hold.
type wsPair struct {
	server *WebSocketTransport
	client *gorilla.Conn
}

// newWSPair upgrades one connection on an httptest server. The server side keeps reading
// so control frames (pongs, close) are processed, as the controller's read loop does.
func newWSPair(t *testing.T) wsPair {
	t.Helper()
	got := make(chan *WebSocketTransport, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&gorilla.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		tr := NewWebSocketTransport(conn).(*WebSocketTransport)
		got <- tr
		for {
			if _, _, err := tr.ReadMessage(); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)

	client, resp, err := gorilla.DefaultDialer.Dial("ws"+strings.TrimPrefix(srv.URL, "http"), nil)
	require.NoError(t, err)
	require.NoError(t, resp.Body.Close())
	t.Cleanup(func() { _ = client.Close() })

	select {
	case tr := <-got:
		return wsPair{server: tr, client: client}
	case <-time.After(lifecycleWait):
		t.Fatal("server never upgraded the connection")
		return wsPair{}
	}
}

// readText reads the next data frame the gateway receives.
func readText(t *testing.T, c *gorilla.Conn) string {
	t.Helper()
	require.NoError(t, c.SetReadDeadline(time.Now().Add(lifecycleWait)))
	_, msg, err := c.ReadMessage()
	require.NoError(t, err)
	return string(msg)
}

func newTestManager(maxConns int, hub eventhub.EventHub) *Manager {
	return NewManager(ManagerConfig{MaxConnections: maxConns, HeartbeatInterval: time.Hour, HeartbeatTimeout: time.Hour}, hub)
}

// Events published for a gateway reach its socket: the gateway envelope as is, or the event itself when it has none.
func TestManagerForwardsHubEventsToTheGatewaySocket(t *testing.T) {
	hub := &chanHub{subs: map[string]chan eventhub.Event{}}
	m := newTestManager(10, hub)
	t.Cleanup(m.Shutdown)
	p := newWSPair(t)

	conn, err := m.Register("gw-1", p.server, "token")
	require.NoError(t, err)

	hub.send("gw-1", eventhub.Event{EventType: "llmprovider.deployed", EventData: `{"type":"llmprovider.deployed"}`})
	assert.JSONEq(t, `{"type":"llmprovider.deployed"}`, readText(t, p.client))

	hub.send("gw-1", eventhub.Event{EventType: "agent.undeployed", EventID: "evt-2", EntityID: "agent-1"})
	var raw eventhub.Event
	require.NoError(t, json.Unmarshal([]byte(readText(t, p.client)), &raw))
	assert.Equal(t, eventhub.EventType("agent.undeployed"), raw.EventType)
	assert.Equal(t, "evt-2", raw.EventID)
	assert.Equal(t, "agent-1", raw.EntityID)

	assert.Eventually(t, func() bool { return conn.DeliveryStats.GetTotalEventsSent() == 2 }, lifecycleWait, 5*time.Millisecond)
	assert.Zero(t, conn.DeliveryStats.GetFailedDeliveries())
}

// Unregister sends the gateway a normal-closure frame and forgets the connection; repeating it is harmless.
func TestManagerUnregisterClosesTheSocket(t *testing.T) {
	hub := &chanHub{subs: map[string]chan eventhub.Event{}}
	m := newTestManager(10, hub)
	t.Cleanup(m.Shutdown)
	p := newWSPair(t)
	conn, err := m.Register("gw-1", p.server, "token")
	require.NoError(t, err)
	require.Equal(t, []*Connection{conn}, m.GetConnections("gw-1"))

	m.Unregister("gw-1", conn.ConnectionID)

	require.NoError(t, p.client.SetReadDeadline(time.Now().Add(lifecycleWait)))
	_, _, err = p.client.ReadMessage()
	var closeErr *gorilla.CloseError
	require.ErrorAs(t, err, &closeErr)
	assert.Equal(t, gorilla.CloseNormalClosure, closeErr.Code)
	assert.Equal(t, "normal closure", closeErr.Text)
	assert.True(t, conn.IsClosed())
	assert.Empty(t, m.GetConnections("gw-1"))
	assert.Empty(t, m.GetAllConnections())
	assert.Zero(t, m.GetConnectionCount())
	hub.mu.Lock()
	assert.Empty(t, hub.subs, "the hub subscription is released")
	hub.mu.Unlock()

	m.Unregister("gw-1", conn.ConnectionID)
	m.Unregister("gw-unknown", "c")
	assert.Zero(t, m.GetConnectionCount())
}

// The manager pings each connection and a gateway's pong keeps it alive.
func TestManagerHeartbeatPongUpdatesLastHeartbeat(t *testing.T) {
	m := NewManager(ManagerConfig{MaxConnections: 10, HeartbeatInterval: 10 * time.Millisecond, HeartbeatTimeout: time.Hour}, nil)
	t.Cleanup(m.Shutdown)
	p := newWSPair(t)
	pinged := make(chan struct{}, 1)
	p.client.SetPingHandler(func(data string) error {
		select {
		case pinged <- struct{}{}:
		default:
		}
		return p.client.WriteControl(gorilla.PongMessage, []byte(data), time.Now().Add(time.Second))
	})
	go func() {
		for {
			if _, _, err := p.client.ReadMessage(); err != nil {
				return
			}
		}
	}()

	conn, err := m.Register("gw-1", p.server, "token")
	require.NoError(t, err)
	registeredAt := conn.GetLastHeartbeat()

	select {
	case <-pinged:
	case <-time.After(lifecycleWait):
		t.Fatal("gateway was never pinged")
	}
	assert.Eventually(t, func() bool { return conn.GetLastHeartbeat().After(registeredAt) }, lifecycleWait, 5*time.Millisecond)
	assert.Equal(t, 1, m.GetConnectionCount())
}

// pingTransport fails pings on demand.
type pingTransport struct {
	recordingTransport
	pingErr error
}

func (t *pingTransport) SendPing() error { return t.pingErr }

// failingSendTransport fails every send.
type failingSendTransport struct{ recordingTransport }

func (*failingSendTransport) Send([]byte) error { return errors.New("broken pipe") }

// A connection whose heartbeat lapsed, or whose ping fails, is dropped and closed.
func TestManagerDropsDeadConnections(t *testing.T) {
	for name, tc := range map[string]struct {
		cfg ManagerConfig
		tr  *pingTransport
	}{
		"heartbeat timed out": {
			ManagerConfig{MaxConnections: 5, HeartbeatInterval: 5 * time.Millisecond, HeartbeatTimeout: time.Nanosecond},
			&pingTransport{},
		},
		"ping failed": {
			ManagerConfig{MaxConnections: 5, HeartbeatInterval: 5 * time.Millisecond, HeartbeatTimeout: time.Hour},
			&pingTransport{pingErr: errors.New("write: broken pipe")},
		},
	} {
		t.Run(name, func(t *testing.T) {
			m := NewManager(tc.cfg, nil)
			t.Cleanup(m.Shutdown)

			conn, err := m.Register("gw-1", tc.tr, "token")
			require.NoError(t, err)

			assert.Eventually(t, tc.tr.isClosed, lifecycleWait, 5*time.Millisecond)
			assert.Eventually(t, func() bool { return m.GetConnectionCount() == 0 }, lifecycleWait, 5*time.Millisecond)
			assert.True(t, conn.IsClosed())
			assert.Empty(t, m.GetConnections("gw-1"))
		})
	}
}

// A failed forward is counted against the connection with its reason, and the connection stays up.
func TestManagerCountsFailedForwards(t *testing.T) {
	hub := &chanHub{subs: map[string]chan eventhub.Event{}}
	m := newTestManager(10, hub)
	t.Cleanup(m.Shutdown)
	conn, err := m.Register("gw-1", &failingSendTransport{}, "token")
	require.NoError(t, err)

	hub.send("gw-1", eventhub.Event{EventType: "x", EventData: "{}"})

	assert.Eventually(t, func() bool { return conn.DeliveryStats.GetFailedDeliveries() == 1 }, lifecycleWait, 5*time.Millisecond)
	_, reason := conn.DeliveryStats.GetLastFailure()
	assert.Equal(t, "forward error: broken pipe", reason)
	assert.Zero(t, conn.DeliveryStats.GetTotalEventsSent())
	assert.False(t, conn.IsClosed())
}

// The connection limit counts distinct gateways; a gateway reconnecting replaces its old socket instead.
func TestManagerConnectionLimitAndReconnectEviction(t *testing.T) {
	hub := &chanHub{subs: map[string]chan eventhub.Event{}}
	m := newTestManager(1, hub)
	t.Cleanup(m.Shutdown)
	first := &recordingTransport{}
	old, err := m.Register("gw-1", first, "token")
	require.NoError(t, err)

	conn, err := m.Register("gw-2", &recordingTransport{}, "token")
	require.EqualError(t, err, "maximum connection limit reached (1)")
	assert.Nil(t, conn)
	assert.Equal(t, 1, m.GetConnectionCount())

	second := &recordingTransport{}
	replacement, err := m.Register("gw-1", second, "token")
	require.NoError(t, err)

	assert.True(t, first.isClosed(), "the superseded socket is closed")
	assert.True(t, old.IsClosed())
	assert.False(t, second.isClosed())
	assert.NotEqual(t, old.ConnectionID, replacement.ConnectionID)
	assert.Equal(t, []*Connection{replacement}, m.GetConnections("gw-1"))
	assert.Equal(t, map[string][]*Connection{"gw-1": {replacement}}, m.GetAllConnections())
	assert.Equal(t, 1, m.GetConnectionCount())

	hub.send("gw-1", eventhub.Event{EventType: "x", EventData: "{}"})
	assert.Eventually(t, func() bool { return second.sentCount() == 1 }, lifecycleWait, 5*time.Millisecond)
	assert.Zero(t, first.sentCount(), "events go to the new socket only")

	m.Unregister("gw-1", old.ConnectionID)
	assert.Equal(t, 1, m.GetConnectionCount(), "unregistering the evicted connection does not drop the new one")
}

// failingHub refuses subscriptions.
type failingHub struct{ eventhub.EventHub }

func (failingHub) Subscribe(string) (<-chan eventhub.Event, error) {
	return nil, errors.New("hub down")
}

// A gateway whose event subscription fails is refused and leaves no connection behind.
func TestManagerRegisterFailsWhenHubSubscribeFails(t *testing.T) {
	m := newTestManager(10, failingHub{})
	t.Cleanup(m.Shutdown)
	tr := &recordingTransport{}

	conn, err := m.Register("gw-1", tr, "token")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to subscribe to EventHub for gateway gw-1")
	assert.Contains(t, err.Error(), "hub down")
	assert.Nil(t, conn)
	assert.True(t, tr.isClosed())
	assert.Zero(t, m.GetConnectionCount())
	assert.Empty(t, m.GetConnections("gw-1"))
}

// Shutdown closes every connection with a server-shutdown frame and returns once its goroutines exit.
func TestManagerShutdownClosesEveryConnection(t *testing.T) {
	hub := &chanHub{subs: map[string]chan eventhub.Event{}}
	m := newTestManager(10, hub)
	p := newWSPair(t)
	_, err := m.Register("gw-1", p.server, "token")
	require.NoError(t, err)
	other := &recordingTransport{}
	_, err = m.Register("gw-2", other, "token")
	require.NoError(t, err)

	done := make(chan struct{})
	go func() { m.Shutdown(); close(done) }()
	select {
	case <-done:
	case <-time.After(lifecycleWait):
		t.Fatal("Shutdown did not return")
	}

	assert.True(t, other.isClosed())
	require.NoError(t, p.client.SetReadDeadline(time.Now().Add(lifecycleWait)))
	_, _, err = p.client.ReadMessage()
	var closeErr *gorilla.CloseError
	require.ErrorAs(t, err, &closeErr)
	assert.Equal(t, gorilla.CloseNormalClosure, closeErr.Code)
	assert.Equal(t, "server shutdown", closeErr.Text)
}

// A closed connection refuses sends and pings, closes only once, and reports itself closed.
func TestConnectionAfterClose(t *testing.T) {
	tr := &recordingTransport{}
	c := NewConnection("gw-1", "c-1", tr, "token")

	require.NoError(t, c.Send([]byte("hi")))
	require.NoError(t, c.SendPing())
	assert.Equal(t, "connected", c.GetStatus(time.Hour).Status)

	require.NoError(t, c.Close(1000, "bye"))
	require.NoError(t, c.Close(1000, "again"))

	assert.ErrorIs(t, c.Send([]byte("late")), ErrConnectionClosed)
	assert.ErrorIs(t, c.SendPing(), ErrConnectionClosed)
	assert.Equal(t, "connection is closed", ErrConnectionClosed.Error())
	assert.Equal(t, 1, tr.sentCount())
	st := c.GetStatus(time.Hour)
	assert.Equal(t, "closed", st.Status)
	assert.Equal(t, "gw-1", st.GatewayID)
	assert.Equal(t, "c-1", st.ConnectionID)
	assert.Equal(t, c.ConnectedAt, st.ConnectedAt)
}

// A connection whose last heartbeat is older than the timeout is reported stale until the next pong.
func TestConnectionStatusStaleUntilHeartbeat(t *testing.T) {
	c := NewConnection("gw-1", "c-1", &recordingTransport{}, "token")
	c.LastHeartbeat = time.Now().Add(-time.Minute)

	assert.Equal(t, "stale", c.GetStatus(time.Second).Status)

	c.UpdateHeartbeat()

	assert.Equal(t, "connected", c.GetStatus(time.Second).Status)
	assert.WithinDuration(t, time.Now(), c.GetLastHeartbeat(), time.Second)
}

// Delivery stats report the success rate and the latest failure.
func TestStatsSuccessRateAndLastFailure(t *testing.T) {
	s := NewStats()
	assert.InDelta(t, 100.0, s.GetSuccessRate(), 0.001, "no traffic counts as fully successful")
	when, reason := s.GetLastFailure()
	assert.True(t, when.IsZero())
	assert.Empty(t, reason)

	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() { defer wg.Done(); s.IncrementTotalSent() }()
	}
	wg.Wait()
	s.IncrementFailed("first")
	s.IncrementFailed("timeout")

	assert.Equal(t, uint64(4), s.GetTotalEventsSent())
	assert.Equal(t, uint64(2), s.GetFailedDeliveries())
	assert.InDelta(t, 50.0, s.GetSuccessRate(), 0.001)
	when, reason = s.GetLastFailure()
	assert.WithinDuration(t, time.Now(), when, time.Second)
	assert.Equal(t, "timeout", reason)
}

// The WebSocket transport delivers text frames, honours deadlines and tolerates a second close.
func TestWebSocketTransportSendDeadlinesAndClose(t *testing.T) {
	p := newWSPair(t)

	require.NoError(t, p.server.SetWriteDeadline(time.Now().Add(lifecycleWait)))
	require.NoError(t, p.server.Send([]byte(`{"type":"ping"}`)))
	require.NoError(t, p.client.SetReadDeadline(time.Now().Add(lifecycleWait)))
	mt, msg, err := p.client.ReadMessage()
	require.NoError(t, err)
	assert.Equal(t, gorilla.TextMessage, mt)
	assert.JSONEq(t, `{"type":"ping"}`, string(msg))

	require.NoError(t, p.server.SetReadDeadline(time.Now().Add(lifecycleWait)))
	require.NoError(t, p.server.Close(gorilla.CloseGoingAway, "restarting"))
	_, _, err = p.client.ReadMessage()
	var closeErr *gorilla.CloseError
	require.ErrorAs(t, err, &closeErr)
	assert.Equal(t, gorilla.CloseGoingAway, closeErr.Code)
	assert.Equal(t, "restarting", closeErr.Text)

	assert.Error(t, p.server.Close(gorilla.CloseNormalClosure, "again"), "closing a closed socket reports the underlying error")
}
