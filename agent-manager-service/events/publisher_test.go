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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type publisherTestBus struct {
	Bus
	publish func(context.Context, Event, []byte) error
}

func (b publisherTestBus) PublishEvent(ctx context.Context, e Event, body []byte) error {
	return b.publish(ctx, e, body)
}

type publisherTestOutbox struct {
	put      func(context.Context, Event) error
	claim    func(context.Context) ([]PendingEvent, error)
	complete func(context.Context, PendingEvent) error
	retry    func(context.Context, PendingEvent) error
}

func (o publisherTestOutbox) Put(ctx context.Context, e Event) error { return o.put(ctx, e) }
func (o publisherTestOutbox) Claim(ctx context.Context) ([]PendingEvent, error) {
	return o.claim(ctx)
}

func (o publisherTestOutbox) Complete(ctx context.Context, e PendingEvent) error {
	return o.complete(ctx, e)
}

func (o publisherTestOutbox) Retry(ctx context.Context, e PendingEvent) error {
	return o.retry(ctx, e)
}

func publisherTestEvent() Event {
	return New("evt-durable", TypeMonitorRunSucceeded, ScopeAgent, "org", "project", "agent", time.Now())
}

func TestAsyncPublisher_PersistsBeforeStart(t *testing.T) {
	evt := publisherTestEvent()
	var stored Event
	outbox := publisherTestOutbox{put: func(ctx context.Context, e Event) error {
		require.NoError(t, ctx.Err())
		stored = e
		return nil
	}}
	// Neither a worker nor a reachable bus is needed to accept an event.
	p := NewAsyncPublisher(nil, outbox, slog.New(slog.DiscardHandler))
	require.NoError(t, p.Publish(context.Background(), evt))
	assert.Equal(t, evt, stored)
	p.Stop()
}

func TestAsyncPublisher_PersistenceFailureReachesCaller(t *testing.T) {
	storeErr := errors.New("database unavailable")
	outbox := publisherTestOutbox{put: func(context.Context, Event) error { return storeErr }}
	p := NewAsyncPublisher(nil, outbox, slog.New(slog.DiscardHandler))
	err := p.Publish(context.Background(), publisherTestEvent())
	assert.ErrorIs(t, err, storeErr)
	assert.ErrorContains(t, err, "evt-durable")
}

func TestAsyncPublisher_CallerCancellationDoesNotDiscardEvent(t *testing.T) {
	type contextKey struct{}
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), contextKey{}, "correlation"))
	cancel()
	persisted := false
	outbox := publisherTestOutbox{put: func(saveCtx context.Context, _ Event) error {
		assert.NoError(t, saveCtx.Err())
		assert.Equal(t, "correlation", saveCtx.Value(contextKey{}))
		deadline, bounded := saveCtx.Deadline()
		assert.True(t, bounded)
		assert.LessOrEqual(t, time.Until(deadline), publishTimeout)
		persisted = true
		return nil
	}}
	p := NewAsyncPublisher(nil, outbox, slog.New(slog.DiscardHandler))
	require.NoError(t, p.Publish(ctx, publisherTestEvent()))
	assert.True(t, persisted)
}

func TestAsyncPublisher_RestartRetriesUnfinishedPublication(t *testing.T) {
	for _, failure := range []string{"publish", "complete"} {
		t.Run(failure, func(t *testing.T) {
			evt := publisherTestEvent()
			body, err := json.Marshal(evt)
			require.NoError(t, err)
			pending := []PendingEvent{{ID: evt.ID, Body: body, Lease: "lease"}}
			fail := true
			publishes, releases := 0, 0
			outbox := publisherTestOutbox{
				claim: func(context.Context) ([]PendingEvent, error) { return pending, nil },
				complete: func(_ context.Context, e PendingEvent) error {
					assert.Equal(t, pending[0], e)
					if fail && failure == "complete" {
						return errors.New("database unavailable")
					}
					pending = nil
					return nil
				},
				retry: func(_ context.Context, e PendingEvent) error {
					assert.Equal(t, pending[0], e)
					releases++
					return nil
				},
			}
			bus := publisherTestBus{publish: func(_ context.Context, e Event, raw []byte) error {
				publishes++
				assert.Equal(t, evt.ID, e.ID, "retries retain the deduplication key")
				assert.JSONEq(t, string(body), string(raw))
				if fail && failure == "publish" {
					return errors.New("NATS unavailable")
				}
				return nil
			}}
			p := NewAsyncPublisher(bus, outbox, slog.New(slog.DiscardHandler))
			p.drain(context.Background())
			require.Len(t, pending, 1, "an unconfirmed publication must remain durable")
			if failure == "publish" {
				assert.Equal(t, 1, releases)
			} else {
				assert.Zero(t, releases)
			}
			// A replacement process can claim the same durable item after its
			// lease is available; lease expiry itself is an outbox concern.
			fail = false
			restarted := NewAsyncPublisher(bus, outbox, slog.New(slog.DiscardHandler))
			restarted.drain(context.Background())
			assert.Empty(t, pending)
			assert.Equal(t, 2, publishes)
		})
	}
}

func TestAsyncPublisher_CancelledDrainLeavesClaimedEvents(t *testing.T) {
	evt := publisherTestEvent()
	body, err := json.Marshal(evt)
	require.NoError(t, err)
	pending := []PendingEvent{{ID: evt.ID, Body: body, Lease: "lease"}}
	ctx, cancel := context.WithCancel(context.Background())
	outbox := publisherTestOutbox{claim: func(context.Context) ([]PendingEvent, error) {
		cancel()
		return pending, nil
	}}
	// Any publication or completion would panic: none is configured.
	p := NewAsyncPublisher(nil, outbox, slog.New(slog.DiscardHandler))
	p.drain(ctx)
	assert.Len(t, pending, 1)
}

func TestAsyncPublisher_StopCancelsBlockedPublication(t *testing.T) {
	evt := publisherTestEvent()
	body, err := json.Marshal(evt)
	require.NoError(t, err)
	pending := []PendingEvent{{ID: evt.ID, Body: body, Lease: "lease"}}
	entered := make(chan struct{})
	outbox := publisherTestOutbox{
		claim: func(context.Context) ([]PendingEvent, error) { return pending, nil },
		retry: func(ctx context.Context, _ PendingEvent) error { return ctx.Err() },
	}
	bus := publisherTestBus{publish: func(ctx context.Context, _ Event, _ []byte) error {
		close(entered)
		<-ctx.Done()
		return ctx.Err()
	}}
	p := NewAsyncPublisher(bus, outbox, slog.New(slog.DiscardHandler))
	p.Start()
	t.Cleanup(p.Stop)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("publisher did not start draining")
	}
	stopped := make(chan struct{})
	go func() {
		p.Stop()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop waited for publish timeout instead of cancelling")
	}
	assert.Len(t, pending, 1, "cancelled publication survives even if claim release fails")
}

func TestAsyncPublisher_DrainsBacklogWithoutWaitingForTicks(t *testing.T) {
	var queue []PendingEvent
	for i := range 3*OutboxBatchSize + 5 {
		e := New(fmt.Sprintf("evt-%d", i), TypeMonitorRunSucceeded, ScopeAgent, "org", "project", "agent", time.Now())
		body, err := json.Marshal(e)
		require.NoError(t, err)
		queue = append(queue, PendingEvent{ID: e.ID, Body: body, Lease: "lease"})
	}
	var mu sync.Mutex
	outbox := publisherTestOutbox{
		claim: func(context.Context) ([]PendingEvent, error) {
			mu.Lock()
			defer mu.Unlock()
			n := min(OutboxBatchSize, len(queue))
			batch := queue[:n]
			queue = queue[n:]
			return batch, nil
		},
		complete: func(context.Context, PendingEvent) error { return nil },
	}
	published := make(chan struct{}, len(queue))
	bus := publisherTestBus{publish: func(context.Context, Event, []byte) error {
		published <- struct{}{}
		return nil
	}}
	total := len(queue)
	p := NewAsyncPublisher(bus, outbox, slog.New(slog.DiscardHandler))
	p.Start()
	t.Cleanup(p.Stop)
	// The first tick is a second away: the whole backlog must go before it.
	deadline := time.After(500 * time.Millisecond)
	for range total {
		select {
		case <-published:
		case <-deadline:
			t.Fatal("the backlog was not drained in one pass")
		}
	}
}

func TestPublisherConcurrentStartStopPublish(t *testing.T) {
	for range 100 {
		outbox := publisherTestOutbox{
			put:   func(context.Context, Event) error { return nil },
			claim: func(context.Context) ([]PendingEvent, error) { return []PendingEvent{}, nil },
		}
		p := NewAsyncPublisher(nil, outbox, slog.New(slog.DiscardHandler))
		var wg sync.WaitGroup
		wg.Go(p.Start)
		wg.Go(p.Start)
		wg.Go(p.Stop)
		wg.Go(func() { _ = p.Publish(context.Background(), publisherTestEvent()) })
		wg.Wait()
		p.Start()
		p.Stop()
	}
}
