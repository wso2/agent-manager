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
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type publisherTestBus struct {
	Bus
	count atomic.Int64
}

func (b *publisherTestBus) PublishEvent(context.Context, Event, []byte) error {
	b.count.Add(1)
	return nil
}

func TestPublisherLifecycle(t *testing.T) {
	bus := &publisherTestBus{}
	p := NewAsyncPublisher(bus, slog.New(slog.NewTextHandler(io.Discard, nil)))
	p.Publish(context.Background(), Event{ID: "queued"})
	p.Start()
	p.Start()
	p.Stop()
	p.Publish(context.Background(), Event{ID: "stopped"})
	p.Start()
	p.Stop()
	if got := bus.count.Load(); got != 1 {
		t.Fatalf("published %d events, want only the queued event", got)
	}
}

func TestPublisherConcurrentStartStopPublish(t *testing.T) {
	for range 100 {
		bus := &publisherTestBus{}
		p := NewAsyncPublisher(bus, slog.New(slog.NewTextHandler(io.Discard, nil)))
		var wg sync.WaitGroup
		wg.Go(p.Start)
		wg.Go(p.Stop)
		wg.Go(func() {
			for range 10 {
				p.Publish(context.Background(), Event{ID: "concurrent"})
			}
		})
		wg.Wait()
		p.Stop()
	}
}

type unavailablePublisherBus struct {
	Bus
	attempted chan struct{}
	once      sync.Once
}

func (b *unavailablePublisherBus) PublishEvent(ctx context.Context, _ Event, _ []byte) error {
	b.once.Do(func() { close(b.attempted) })
	<-ctx.Done()
	return ctx.Err()
}

func TestPublisherShutdownBoundsUnavailableBus(t *testing.T) {
	bus := &unavailablePublisherBus{attempted: make(chan struct{})}
	p := NewAsyncPublisher(bus, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for range 10 {
		p.Publish(context.Background(), Event{ID: "queued"})
	}
	p.Start()
	<-bus.attempted
	done := make(chan struct{})
	go func() { p.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(shutdownTimeout + 3*time.Second):
		p.cancel()
		t.Fatal("shutdown did not cancel the unavailable transport")
	}
}
