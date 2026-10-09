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
	"time"
)

// QueueWebhookDeliveries is the work queue holding one item per (event,
// webhook) pair, created by the dispatcher's fan-out.
const QueueWebhookDeliveries = "webhook-deliveries"

// Bus is the event transport. The rest of the service depends on this
// interface only; NATSBus implements it for a NATS server and for the
// embedded in-memory server, and other systems (Kafka, say) can be added as
// further implementations without touching publishers or the dispatcher.
//
// A Bus offers two kinds of channel:
//   - the event stream: every published event, read by each subscriber
//     (each consumer name gets every event once, shared across replicas);
//   - work queues: each item goes to one consumer, which acknowledges it,
//     asks for it again later, or drops it.
type Bus interface {
	// Setup creates whatever the transport needs (streams, topics). It is
	// idempotent and may be retried until the transport is reachable.
	Setup(ctx context.Context) error

	// PublishEvent writes an event to the event stream. Publishing the same
	// event ID twice within a short window delivers it once.
	PublishEvent(ctx context.Context, e Event, body []byte) error
	// SubscribeEvents delivers events published from now on to handle. Every
	// replica subscribing with the same consumer name shares the stream, so
	// each event is handled by one of them.
	SubscribeEvents(ctx context.Context, consumer string, handle func(Message)) (Subscription, error)

	// Enqueue adds an item to a work queue. Enqueueing the same key twice
	// within a short window adds it once.
	Enqueue(ctx context.Context, queue, key string, body []byte) error
	// Consume delivers a work queue's items to handle, shared across every
	// replica consuming with the same consumer name.
	Consume(ctx context.Context, queue, consumer string, handle func(Message)) (Subscription, error)

	// Close releases the transport.
	Close()
}

// Message is one event or work item being handled. Exactly one of Ack, Retry
// or Drop should be called; an item left unanswered is redelivered after the
// transport's own timeout.
type Message interface {
	Data() []byte
	// Attempt is the 1-based delivery count of this item.
	Attempt() int
	// Ack marks the item done.
	Ack() error
	// Retry asks for the item to be delivered again after the delay.
	Retry(after time.Duration) error
	// Drop gives up on the item; it is not delivered again.
	Drop() error
}

// Subscription is a running consumer.
type Subscription interface {
	Stop()
}
