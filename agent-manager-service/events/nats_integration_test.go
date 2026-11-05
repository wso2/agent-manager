//go:build integration

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
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedEventStreamEvictsOldEvents(t *testing.T) {
	bus, err := StartEmbedded(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer bus.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, bus.Setup(ctx))
	stream, err := bus.js.Stream(ctx, EventsStream)
	require.NoError(t, err)
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, embeddedMaxMemory/2, info.Config.MaxBytes)
	require.Equal(t, jetstream.DiscardOld, info.Config.Discard)
	// Exercise eviction using a tiny limit rather than allocating the full budget.
	cfg := info.Config
	cfg.MaxBytes = 1024
	_, err = bus.js.UpdateStream(ctx, cfg)
	require.NoError(t, err)
	for i := range 4 {
		e := New(fmt.Sprint(i), "agent.created", ScopeAgent, "org", "project", "agent", time.Now())
		require.NoError(t, bus.PublishEvent(ctx, e, make([]byte, 512)))
	}
	info, err = stream.Info(ctx)
	require.NoError(t, err)
	require.Positive(t, info.State.Msgs)
	require.Less(t, info.State.Msgs, uint64(4))
	require.LessOrEqual(t, info.State.Bytes, uint64(1024))
}

func TestEventConsumerBoundsRetries(t *testing.T) {
	bus, err := StartEmbedded(slog.New(slog.NewTextHandler(io.Discard, nil)))
	require.NoError(t, err)
	defer bus.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, bus.Setup(ctx))
	sub, err := bus.SubscribeEvents(ctx, "bounded-retries", func(msg Message) { _ = msg.Drop() })
	require.NoError(t, err)
	defer sub.Stop()
	consumer, err := bus.js.Consumer(ctx, EventsStream, "bounded-retries")
	require.NoError(t, err)
	info, err := consumer.Info(ctx)
	require.NoError(t, err)
	require.Equal(t, natsEventMaxDeliver, info.Config.MaxDeliver)
}
