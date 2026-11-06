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
	"fmt"
	"log/slog"
	"sync"
	"time"
)

const publishTimeout = 5 * time.Second

// Publisher durably accepts an event or returns an error. Callers with a
// replayable source must not mark the source complete until this succeeds.
type Publisher interface {
	Publish(ctx context.Context, e Event) error
}

type NoopPublisher struct{}

func (NoopPublisher) Publish(context.Context, Event) error { return nil }

// AsyncPublisher stores events synchronously and drains the durable outbox in
// the background. Bus outages and restarts do not discard accepted events.
type AsyncPublisher struct {
	bus     Bus
	outbox  Outbox
	logger  *slog.Logger
	wg      sync.WaitGroup
	cancel  context.CancelFunc
	ctx     context.Context
	mu      sync.Mutex
	started bool
	stopped bool
}

func NewAsyncPublisher(bus Bus, outbox Outbox, logger *slog.Logger) *AsyncPublisher {
	ctx, cancel := context.WithCancel(context.Background())
	return &AsyncPublisher{bus: bus, outbox: outbox, logger: logger, ctx: ctx, cancel: cancel}
}

func (p *AsyncPublisher) Start() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started || p.stopped {
		return
	}
	p.started = true
	ctx := p.ctx
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			// A full batch published cleanly means more may be waiting: keep
			// going rather than draining one batch per tick.
			for more := true; more; {
				more = p.drain(ctx)
			}
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

func (p *AsyncPublisher) Stop() {
	p.mu.Lock()
	p.stopped = true
	p.cancel()
	p.mu.Unlock()
	p.wg.Wait()
}

func (p *AsyncPublisher) Publish(ctx context.Context, e Event) error {
	// An HTTP client disconnect must not cancel persistence of its completed
	// operation. Bound the detached write, rather than queueing in memory.
	saveCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), publishTimeout)
	defer cancel()
	if err := p.outbox.Put(saveCtx, e); err != nil {
		return fmt.Errorf("failed to persist event %s: %w", e.ID, err)
	}
	return nil
}

// drain publishes one claimed batch. It reports whether the batch was full
// and published without failures, so another batch is worth claiming now.
func (p *AsyncPublisher) drain(ctx context.Context) bool {
	pending, err := p.outbox.Claim(ctx)
	if err != nil {
		if ctx.Err() == nil {
			p.logger.Error("Failed to claim event outbox", "error", err)
		}
		return false
	}
	more := len(pending) >= OutboxBatchSize
	for _, item := range pending {
		if ctx.Err() != nil {
			return false
		}
		var e Event
		if err := json.Unmarshal(item.Body, &e); err != nil {
			p.logger.Error("Invalid event in outbox", "eventId", item.ID, "error", err)
			_ = p.outbox.Retry(ctx, item)
			more = false
			continue
		}
		publishCtx, cancel := context.WithTimeout(ctx, publishTimeout)
		err := p.bus.PublishEvent(publishCtx, e, item.Body)
		cancel()
		if err != nil {
			p.logger.Warn("Event publication failed; retained for retry", "eventId", item.ID, "error", err)
			if err := p.outbox.Retry(ctx, item); err != nil {
				p.logger.Error("Failed to release event claim", "eventId", item.ID, "error", err)
			}
			more = false
			continue
		}
		if err := p.outbox.Complete(ctx, item); err != nil {
			p.logger.Error("Failed to complete event publication; will retry", "eventId", item.ID, "error", err)
			more = false
		}
	}
	return more
}
