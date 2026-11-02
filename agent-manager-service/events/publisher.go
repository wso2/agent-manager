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
	publishTimeout = 5 * time.Second
	publishBuffer  = 4096
	publishRetries = 3
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
	bus    Bus
	logger *slog.Logger
	queue  chan Event
	wg     sync.WaitGroup
	once   sync.Once
}

// NewAsyncPublisher creates a publisher; call Start before use.
func NewAsyncPublisher(bus Bus, logger *slog.Logger) *AsyncPublisher {
	return &AsyncPublisher{bus: bus, logger: logger, queue: make(chan Event, publishBuffer)}
}

// Start runs the publishing loop until Stop.
func (p *AsyncPublisher) Start() {
	p.wg.Add(1)
	go func() {
		defer p.wg.Done()
		for e := range p.queue {
			p.publish(e)
		}
	}()
}

// Stop publishes what is queued and returns.
func (p *AsyncPublisher) Stop() {
	p.once.Do(func() { close(p.queue) })
	p.wg.Wait()
}

// Publish queues an event. When the queue is full the event is dropped.
func (p *AsyncPublisher) Publish(_ context.Context, e Event) {
	defer func() {
		// Publishing after Stop sends on a closed channel; drop instead.
		if recover() != nil {
			p.logger.Warn("Event dropped: publisher stopped", "type", e.Type, "id", e.ID)
		}
	}()
	select {
	case p.queue <- e:
	default:
		p.logger.Error("Event dropped: publish queue full", "type", e.Type, "id", e.ID)
	}
}

func (p *AsyncPublisher) publish(e Event) {
	body, err := json.Marshal(e)
	if err != nil {
		p.logger.Error("Failed to encode event", "type", e.Type, "error", err)
		return
	}
	var lastErr error
	for attempt := range publishRetries {
		ctx, cancel := context.WithTimeout(context.Background(), publishTimeout)
		lastErr = p.bus.PublishEvent(ctx, e, body)
		cancel()
		if lastErr == nil {
			return
		}
		if attempt < publishRetries-1 {
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}
	p.logger.Error("Failed to publish event", "type", e.Type, "id", e.ID, "error", lastErr)
}
