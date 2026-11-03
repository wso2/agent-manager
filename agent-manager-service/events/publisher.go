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
	"log/slog"
	"sync"
	"time"
)

const (
	publishTimeout  = 5 * time.Second
	publishBuffer   = 4096
	publishRetries  = 3
	shutdownTimeout = 10 * time.Second
)

// Publisher publishes events. Publish never blocks or fails the caller: an
// event that cannot be published is logged and dropped.
type Publisher interface {
	Publish(ctx context.Context, e Event)
}

// NoopPublisher discards events. Used when events are off.
type NoopPublisher struct{}

func (NoopPublisher) Publish(context.Context, Event) {}

// AsyncPublisher queues events in memory and publishes them to a Bus from a
// single goroutine, so a request never waits on the transport.
type AsyncPublisher struct {
	bus     Bus
	logger  *slog.Logger
	queue   chan Event
	wg      sync.WaitGroup
	mu      sync.Mutex
	started bool
	stopped bool
	ctx     context.Context
	cancel  context.CancelFunc
}

// NewAsyncPublisher creates a publisher; call Start before use.
func NewAsyncPublisher(bus Bus, logger *slog.Logger) *AsyncPublisher {
	ctx, cancel := context.WithCancel(context.Background())
	return &AsyncPublisher{bus: bus, logger: logger, queue: make(chan Event, publishBuffer), ctx: ctx, cancel: cancel}
}

// Start runs the publishing loop until Stop.
func (p *AsyncPublisher) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || p.stopped {
		return
	}
	p.started = true
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for e := range p.queue {
			p.publish(e)
		}
	}()
}

// Stop drains queued events for up to shutdownTimeout, then cancels publishing.
func (p *AsyncPublisher) Stop() {
	p.mu.Lock()
	if !p.stopped {
		p.stopped = true
		close(p.queue)
	}
	p.mu.Unlock()
	timer := time.AfterFunc(shutdownTimeout, p.cancel)
	p.wg.Wait()
	timer.Stop()
	p.cancel()
}

// Publish queues an event. When the queue is full the event is dropped.
func (p *AsyncPublisher) Publish(_ context.Context, e Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		p.logger.Warn("Event dropped: publisher stopped", "type", e.Type, "id", e.ID)
		return
	}
	select {
	case p.queue <- e:
	default:
		p.logger.Error("Event dropped: publish queue full", "type", e.Type, "id", e.ID)
	}
}

func (p *AsyncPublisher) publish(e Event) {
	if p.ctx.Err() != nil {
		p.logger.Warn("Event dropped: publisher shutdown timed out", "type", e.Type, "id", e.ID)
		return
	}
	body, err := json.Marshal(e)
	if err != nil {
		p.logger.Error("Failed to encode event", "type", e.Type, "error", err)
		return
	}
	var lastErr error
	for attempt := range publishRetries {
		ctx, cancel := context.WithTimeout(p.ctx, publishTimeout)
		lastErr = p.bus.PublishEvent(ctx, e, body)
		cancel()
		if lastErr == nil {
			return
		}
		if attempt < publishRetries-1 {
			select {
			case <-p.ctx.Done():
				return
			case <-time.After(time.Duration(attempt+1) * time.Second):
			}
		}
	}
	p.logger.Error("Failed to publish event", "type", e.Type, "id", e.ID, "error", lastErr)
}
