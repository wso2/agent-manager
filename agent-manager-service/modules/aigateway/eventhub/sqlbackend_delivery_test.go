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

package eventhub

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
)

func newConfiguredBackend(t *testing.T, cfg SQLBackendConfig) *SQLBackend {
	t.Helper()
	backend, err := NewSQLBackend(db.GetDB(), slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
	require.NoError(t, err)
	return backend
}

// A delivered event carries what was published, with the gateway, its sequence number and a "{}" default payload.
func TestPollerDeliversThePublishedEventFields(t *testing.T) {
	backend := newTestBackend(t)
	gatewayID := newTestGateway(t, backend)
	ch, err := backend.Subscribe(gatewayID)
	require.NoError(t, err)
	eventID := "deploy-" + uuid.NewString()
	originated := time.Now().Add(-time.Minute).UTC().Truncate(time.Second)

	require.NoError(t, backend.PublishEvent(context.Background(), gatewayID, Event{
		EventType: "agent.deployed", Action: "UPDATE", EntityID: "agent-1", EventID: eventID,
		OriginatedTimestamp: originated,
	}))
	backend.pollGateways()

	require.Len(t, ch, 1)
	got := <-ch
	assert.Equal(t, gatewayID, got.GatewayID)
	assert.Equal(t, EventType("agent.deployed"), got.EventType)
	assert.Equal(t, "UPDATE", got.Action)
	assert.Equal(t, "agent-1", got.EntityID)
	assert.Equal(t, eventUUID(eventID), got.EventID)
	assert.Equal(t, EmptyEventData, got.EventData)
	assert.EqualValues(t, 1, got.Seq)
	assert.WithinDuration(t, originated, got.OriginatedTimestamp, time.Second)
	assert.False(t, got.ProcessedTimestamp.IsZero())
}

// Each subscriber of a gateway gets every event; once one unsubscribes only the others do. A later
// poll delivers only what is new.
func TestPollerFansOutToEverySubscriber(t *testing.T) {
	backend := newTestBackend(t)
	gatewayID := newTestGateway(t, backend)
	a, err := backend.Subscribe(gatewayID)
	require.NoError(t, err)
	b, err := backend.Subscribe(gatewayID)
	require.NoError(t, err)

	require.NoError(t, backend.PublishEvent(context.Background(), gatewayID, testEvent("one")))
	backend.pollGateways()
	assert.Equal(t, []string{"one"}, drain(a))
	assert.Equal(t, []string{"one"}, drain(b))

	backend.pollGateways()
	assert.Empty(t, drain(a), "nothing new, nothing delivered")

	require.NoError(t, backend.Unsubscribe(gatewayID, a))
	require.NoError(t, backend.PublishEvent(context.Background(), gatewayID, testEvent("two")))
	backend.pollGateways()
	assert.Equal(t, []string{"two"}, drain(b))
}

// Events are kept for a gateway with no subscriber and delivered, in full, once it subscribes.
func TestPollerQueuesEventsUntilTheGatewaySubscribes(t *testing.T) {
	backend := newTestBackend(t)
	gatewayID := newTestGateway(t, backend)
	other := newTestGateway(t, backend)

	for _, id := range []string{"p1", "p1", "p2"} {
		require.NoError(t, backend.PublishEvent(context.Background(), gatewayID, testEvent(id)))
	}
	backend.pollGateways()

	ch, err := backend.Subscribe(gatewayID)
	require.NoError(t, err)
	otherCh, err := backend.Subscribe(other)
	require.NoError(t, err)
	backend.pollGateways()

	assert.Equal(t, []string{"p1", "p1", "p2"}, drain(ch))
	assert.Empty(t, drain(otherCh), "a gateway gets only its own events")
}

// A subscriber whose buffer fills gets the rest on a later poll, with nothing lost or repeated.
func TestPollerDefersDeliveryWhileASubscriberIsFull(t *testing.T) {
	backend := newTestBackend(t)
	gatewayID := newTestGateway(t, backend)
	ch, err := backend.Subscribe(gatewayID)
	require.NoError(t, err)
	total := cap(ch) + 2
	for range total {
		require.NoError(t, backend.PublishEvent(context.Background(), gatewayID, testEvent("e")))
	}

	backend.pollGateways()
	var seqs []int64
	for len(ch) > 0 {
		seqs = append(seqs, (<-ch).Seq)
	}
	require.Len(t, seqs, cap(ch))

	backend.pollGateways()
	for len(ch) > 0 {
		seqs = append(seqs, (<-ch).Seq)
	}

	want := make([]int64, total)
	for i := range want {
		want[i] = int64(i + 1)
	}
	assert.Equal(t, want, seqs)
}

// The poller pages through gateway states and still reaches every registered gateway.
func TestPollerPagesThroughGatewayStates(t *testing.T) {
	cfg := DefaultSQLBackendConfig()
	cfg.GatewayStatePageSize = 1
	backend := newConfiguredBackend(t, cfg)
	gateways := []string{newTestGateway(t, backend), newTestGateway(t, backend), newTestGateway(t, backend)}
	subs := make([]<-chan Event, len(gateways))
	for i, gw := range gateways {
		ch, err := backend.Subscribe(gw)
		require.NoError(t, err)
		subs[i] = ch
		require.NoError(t, backend.PublishEvent(context.Background(), gw, testEvent(gw)))
	}

	backend.pollGateways()

	for i, gw := range gateways {
		assert.Equal(t, []string{gw}, drain(subs[i]))
	}
}

// Registering an unknown or blank gateway fails; registering a known one twice is fine.
func TestRegisterGateway(t *testing.T) {
	backend := newTestBackend(t)

	err := backend.RegisterGateway(uuid.NewString())
	require.ErrorContains(t, err, "failed to register gateway")

	gatewayID := newTestGateway(t, backend)
	require.NoError(t, backend.RegisterGateway(gatewayID))
	_, err = backend.Subscribe(gatewayID)
	require.NoError(t, err)
}

// Publishing to a gateway that does not exist fails and stores nothing.
func TestPublishEventToAnUnknownGatewayFails(t *testing.T) {
	backend := newTestBackend(t)
	gatewayID := uuid.NewString()

	err := backend.PublishEvent(context.Background(), gatewayID, testEvent("e"))

	require.ErrorContains(t, err, "failed to ensure gateway registered")
	assert.Empty(t, eventSeqs(t, gatewayID))
}

// Cleanup removes events processed before the retention window and keeps newer ones.
func TestCleanUpEventsRemovesOnlyExpiredEvents(t *testing.T) {
	backend := newTestBackend(t)
	gatewayID := newTestGateway(t, backend)
	old := newTestBackend(t)
	old.clock = func() time.Time { return time.Now().Add(-2 * backend.config.RetentionPeriod) }

	require.NoError(t, old.PublishEvent(context.Background(), gatewayID, testEvent("old")))
	require.NoError(t, backend.PublishEvent(context.Background(), gatewayID, testEvent("new")))

	require.NoError(t, backend.CleanUpEvents())

	assert.Equal(t, []int64{2}, eventSeqs(t, gatewayID))
}

// With its loops running, the hub delivers published events by itself and Close ends the subscription.
func TestInitializedBackendDeliversAndCloses(t *testing.T) {
	cfg := DefaultSQLBackendConfig()
	cfg.PollInterval = 10 * time.Millisecond
	cfg.CleanupInterval = 10 * time.Millisecond
	backend := newConfiguredBackend(t, cfg)
	gatewayID := newTestGateway(t, backend)
	ch, err := backend.Subscribe(gatewayID)
	require.NoError(t, err)
	require.NoError(t, backend.Initialize())

	require.NoError(t, backend.PublishEvent(context.Background(), gatewayID, testEvent("live")))

	select {
	case evt := <-ch:
		assert.Equal(t, "live", evt.EntityID)
	case <-time.After(5 * time.Second):
		t.Fatal("the poll loop never delivered the event")
	}
	require.NoError(t, backend.Close())
	_, open := <-ch
	assert.False(t, open)
	assert.Equal(t, []int64{1}, eventSeqs(t, gatewayID), "cleanup keeps events inside the retention window")
}
