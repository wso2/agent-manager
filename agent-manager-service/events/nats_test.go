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

package events

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNATSSetupPreservesEventsBeforeWorkersStart(t *testing.T) {
	for _, name := range []string{EventFanoutConsumer, MonitorAlertFanoutConsumer} {
		t.Run(name, func(t *testing.T) {
			bus, err := StartEmbedded(slog.New(slog.DiscardHandler))
			require.NoError(t, err)
			t.Cleanup(bus.Close)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			t.Cleanup(cancel)
			require.NoError(t, bus.Setup(ctx))

			first := New("first", TypeMonitorRunSucceeded, ScopeAgent, "org", "project", "agent", time.Now())
			require.NoError(t, bus.PublishEvent(ctx, first, []byte("first")))
			// Another replica's setup must not move the durable's start point
			// past events published while workers were starting.
			require.NoError(t, bus.Setup(ctx))
			received := make(chan Message, 4)
			sub, err := bus.SubscribeEvents(ctx, name, func(msg Message) { received <- msg })
			require.NoError(t, err)
			t.Cleanup(sub.Stop)
			select {
			case msg := <-received:
				assert.Equal(t, "first", string(msg.Data()))
				require.NoError(t, msg.Ack())
			case <-ctx.Done():
				t.Fatal("event published before worker startup was lost")
			}
			consumer, err := bus.js.Consumer(ctx, EventsStream, name)
			require.NoError(t, err)
			require.Eventually(t, func() bool {
				info, infoErr := consumer.Info(ctx)
				return infoErr == nil && info.AckFloor.Stream == 1
			}, time.Second, 10*time.Millisecond)
			sub.Stop()
			select {
			case <-sub.(jetstream.ConsumeContext).Closed():
			case <-ctx.Done():
				t.Fatal("original consumer did not stop")
			}
			require.NoError(t, bus.nc.Flush())

			second := New("second", TypeMonitorRunSucceeded, ScopeAgent, "org", "project", "agent", time.Now())
			require.NoError(t, bus.PublishEvent(ctx, second, []byte("second")))
			require.NoError(t, bus.Setup(ctx))
			sub, err = bus.SubscribeEvents(ctx, name, func(msg Message) { received <- msg })
			require.NoError(t, err)
			t.Cleanup(sub.Stop)
			select {
			case msg := <-received:
				assert.Equal(t, "second", string(msg.Data()), "existing durable must resume without replaying acknowledged events")
				require.NoError(t, msg.Ack())
			case <-ctx.Done():
				t.Fatal("existing durable did not resume")
			}
		})
	}
}

// Items waiting out a retry delay stay unacknowledged; a backlog of them
// larger than the server's default ack-pending limit must not stop the queue.
func TestNATSQueueKeepsDeliveringBehindDelayedRetries(t *testing.T) {
	bus, err := StartEmbedded(slog.New(slog.DiscardHandler))
	require.NoError(t, err)
	t.Cleanup(bus.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, bus.Setup(ctx))

	const backlog = 1100
	for i := range backlog {
		require.NoError(t, bus.Enqueue(ctx, QueueEventDeliveries, fmt.Sprintf("retry-%d", i), []byte("retry")))
	}
	fresh := make(chan struct{}, 1)
	sub, err := bus.Consume(ctx, QueueEventDeliveries, "delivery-test", func(msg Message) {
		if string(msg.Data()) == "fresh" {
			_ = msg.Ack()
			fresh <- struct{}{}
			return
		}
		_ = msg.Retry(time.Hour)
	})
	require.NoError(t, err)
	t.Cleanup(sub.Stop)

	consumer, err := bus.js.Consumer(ctx, "AMP_EVENT_DELIVERIES", "delivery-test")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		info, infoErr := consumer.Info(ctx)
		return infoErr == nil && info.NumAckPending == backlog
	}, 20*time.Second, 50*time.Millisecond, "every item is waiting out its retry")

	require.NoError(t, bus.Enqueue(ctx, QueueEventDeliveries, "fresh", []byte("fresh")))
	select {
	case <-fresh:
	case <-time.After(5 * time.Second):
		t.Fatal("a new item was held back by other items' retries")
	}
}

func TestNATSConnectAuthenticatesWithToken(t *testing.T) {
	srv, err := server.NewServer(&server.Options{
		Host: "127.0.0.1", Port: -1, JetStream: true, StoreDir: t.TempDir(),
		Authorization: "bus-token", NoSigs: true,
	})
	require.NoError(t, err)
	go srv.Start()
	require.True(t, srv.ReadyForConnections(10*time.Second))
	t.Cleanup(srv.Shutdown)
	logger := slog.New(slog.DiscardHandler)

	bus, err := Connect(srv.ClientURL(), "bus-token", "token-test", logger)
	require.NoError(t, err)
	t.Cleanup(bus.Close)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	require.NoError(t, bus.Setup(ctx))

	anonymous, err := Connect(srv.ClientURL(), "", "token-test", logger)
	require.NoError(t, err, "connection is retried in the background")
	t.Cleanup(anonymous.Close)
	short, cancelShort := context.WithTimeout(context.Background(), time.Second)
	t.Cleanup(cancelShort)
	assert.Error(t, anonymous.Setup(short), "a client without the token is refused")
}
