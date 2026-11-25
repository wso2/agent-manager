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
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// newUnitBackend is a backend with no database: enough for everything that never reaches SQL.
func newUnitBackend(t *testing.T, cfg SQLBackendConfig) *SQLBackend {
	t.Helper()
	b, err := NewSQLBackend(nil, slog.New(slog.NewTextHandler(io.Discard, nil)), cfg)
	require.NoError(t, err)
	return b
}

// Catch-up replay keeps only the latest event per entity, in order, and drops API key events entirely.
func TestDeduplicateLatestPerEntity(t *testing.T) {
	ev := func(typ, entity, id string) Event {
		return Event{EventType: EventType(typ), EntityID: entity, EventID: id}
	}
	for name, tc := range map[string]struct {
		in      []Event
		wantIDs []string
	}{
		"empty":         {nil, nil},
		"only API keys": {[]Event{ev("apikey.created", "k1", "1"), ev("apikey.revoked", "k1", "2"), ev("apikey.updated", "k2", "3")}, nil},
		"latest per entity wins, order kept": {
			[]Event{
				ev("llmprovider.deployed", "p1", "1"), ev("agent.deployed", "a1", "2"),
				ev("apikey.created", "k1", "3"), ev("llmprovider.undeployed", "p1", "4"),
			},
			[]string{"2", "4"},
		},
	} {
		t.Run(name, func(t *testing.T) {
			got := deduplicateLatestPerEntity(tc.in)

			var ids []string
			for _, e := range got {
				ids = append(ids, e.EventID)
			}
			assert.Equal(t, tc.wantIDs, ids)
		})
	}
}

// The registry refuses a second registration and subscriptions to unknown gateways.
func TestGatewayRegistryErrors(t *testing.T) {
	r := newGatewayRegistry()
	require.NoError(t, r.register("gw"))

	assert.ErrorIs(t, r.register("gw"), ErrGatewayAlreadyExists)
	assert.ErrorIs(t, r.addSubscriber("other", make(chan Event)), ErrGatewayNotFound)
	_, err := r.removeSubscriber("other", make(chan Event))
	require.ErrorIs(t, err, ErrGatewayNotFound)
	_, err = r.removeSubscriber("gw", make(chan Event))
	require.ErrorIs(t, err, ErrSubscriberNotFound)
	_, err = r.removeAllSubscribers("other")
	require.ErrorIs(t, err, ErrGatewayNotFound)
}

// Removing one subscriber leaves the others registered.
func TestGatewayRegistryRemoveSubscriberKeepsTheRest(t *testing.T) {
	r := newGatewayRegistry()
	require.NoError(t, r.register("gw"))
	a, b, c := make(chan Event), make(chan Event), make(chan Event)
	for _, ch := range []chan Event{a, b, c} {
		require.NoError(t, r.addSubscriber("gw", ch))
	}

	removed, err := r.removeSubscriber("gw", a)

	require.NoError(t, err)
	assert.Equal(t, a, removed)
	assert.ElementsMatch(t, []chan Event{b, c}, r.get("gw").subscribers)
	all, err := r.removeAllSubscribers("gw")
	require.NoError(t, err)
	assert.ElementsMatch(t, []chan Event{b, c}, all)
	assert.Empty(t, r.get("gw").subscribers)
}

// Delivery is held back when any subscriber's buffer is full.
func TestSubscriberChannelsAvailable(t *testing.T) {
	full := make(chan Event, 1)
	full <- Event{}

	assert.True(t, subscriberChannelsAvailable(nil))
	assert.True(t, subscriberChannelsAvailable([]chan Event{make(chan Event, 1)}))
	assert.False(t, subscriberChannelsAvailable([]chan Event{make(chan Event, 1), full}))
}

// A UUID event id is stored as is; any other id maps to a stable UUID so a re-publish still deduplicates.
func TestEventUUID(t *testing.T) {
	id := uuid.New()
	assert.Equal(t, id.String(), eventUUID(id.String()))
	assert.Equal(t, id.String(), eventUUID(strings.ToUpper(id.String())))

	derived := eventUUID("deploy-42")
	_, err := uuid.Parse(derived)
	require.NoError(t, err)
	assert.Equal(t, derived, eventUUID("deploy-42"))
	assert.NotEqual(t, derived, eventUUID("deploy-43"))
}

// Gateway ids are trimmed and a blank one is rejected.
func TestNormalizeGatewayID(t *testing.T) {
	got, err := normalizeGatewayID("  gw-1 \n")
	require.NoError(t, err)
	assert.Equal(t, "gw-1", got)

	_, err = normalizeGatewayID("   ")
	require.EqualError(t, err, "gateway_id cannot be empty")
}

// The defaults are the documented intervals.
func TestDefaultConfigs(t *testing.T) {
	assert.Equal(t, Config{PollInterval: 3 * time.Second, CleanupInterval: 10 * time.Minute, RetentionPeriod: time.Hour}, DefaultConfig())
	assert.Equal(t, SQLBackendConfig{
		PollInterval: 2 * time.Second, CleanupInterval: 5 * time.Minute, RetentionPeriod: time.Hour, GatewayStatePageSize: 200,
	}, DefaultSQLBackendConfig())
}

// Initialize refuses a non-positive interval or retention, naming the bad setting.
func TestSQLBackendInitializeValidatesConfig(t *testing.T) {
	for name, tc := range map[string]struct {
		mutate  func(*SQLBackendConfig)
		wantErr string
	}{
		"poll interval":    {func(c *SQLBackendConfig) { c.PollInterval = 0 }, "invalid config: poll_interval must be > 0, got 0s"},
		"cleanup interval": {func(c *SQLBackendConfig) { c.CleanupInterval = -time.Second }, "invalid config: cleanup_interval must be > 0, got -1s"},
		"retention":        {func(c *SQLBackendConfig) { c.RetentionPeriod = 0 }, "invalid config: retention_period must be > 0, got 0s"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := DefaultSQLBackendConfig()
			tc.mutate(&cfg)

			require.EqualError(t, newUnitBackend(t, cfg).Initialize(), tc.wantErr)
		})
	}
}

// Subscriptions need a registered gateway and a non-blank id; unsubscribing closes the channel.
func TestSQLBackendSubscribeAndUnsubscribe(t *testing.T) {
	b := newUnitBackend(t, DefaultSQLBackendConfig())
	require.NoError(t, b.registry.register("gw-1"))

	_, err := b.Subscribe("gw-unknown")
	require.ErrorIs(t, err, ErrGatewayNotFound)
	assert.Contains(t, err.Error(), "failed to subscribe to gateway gw-unknown")
	_, err = b.Subscribe(" ")
	require.Error(t, err)

	first, err := b.Subscribe(" gw-1 ")
	require.NoError(t, err)
	second, err := b.Subscribe("gw-1")
	require.NoError(t, err)

	require.NoError(t, b.Unsubscribe("gw-1", first))
	_, open := <-first
	assert.False(t, open, "an unsubscribed channel is closed")
	err = b.Unsubscribe("gw-1", first)
	require.ErrorIs(t, err, ErrSubscriberNotFound)
	require.Error(t, b.Unsubscribe("", second))

	require.NoError(t, b.UnsubscribeAll("gw-1"))
	_, open = <-second
	assert.False(t, open)
	require.ErrorIs(t, b.UnsubscribeAll("gw-unknown"), ErrGatewayNotFound)
	require.Error(t, b.UnsubscribeAll(""))
}

// A publish of an event that failed to build, or to a blank gateway, fails before touching the database.
func TestSQLBackendPublishEventTxRejectsBadInput(t *testing.T) {
	b := newUnitBackend(t, DefaultSQLBackendConfig())

	err := b.PublishEventTx(context.Background(), nil, "gw", NewGatewayEvent("x", "CREATE", "id", make(chan int)))
	require.ErrorContains(t, err, "failed to serialize x event")

	err = b.PublishEventTx(context.Background(), nil, "  ", Event{})
	require.EqualError(t, err, "gateway_id cannot be empty")

	require.EqualError(t, b.RegisterGateway(""), "gateway_id cannot be empty")
}

// Close stops the background loops and closes every subscriber channel.
func TestSQLBackendCloseClosesSubscribers(t *testing.T) {
	cfg := DefaultSQLBackendConfig()
	cfg.PollInterval = time.Millisecond
	b := newUnitBackend(t, cfg)
	require.NoError(t, b.Initialize())
	require.NoError(t, b.registry.register("gw-1"))
	ch, err := b.Subscribe("gw-1")
	require.NoError(t, err)

	done := make(chan error, 1)
	go func() { done <- b.Close() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return")
	}

	_, open := <-ch
	assert.False(t, open)
	assert.Empty(t, b.registry.get("gw-1").subscribers)
}
