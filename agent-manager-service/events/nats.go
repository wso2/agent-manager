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
	"os"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// JetStream layout used by NATSBus.
const (
	// EventsStream holds every published event. Subjects are
	// amp.events.<ouId>.<event type>.
	EventsStream  = "AMP_EVENTS"
	EventsSubject = "amp.events"

	// EventFanoutConsumer is the durable consumer the event dispatcher reads.
	EventFanoutConsumer = "event-fanout"
	// MonitorAlertFanoutConsumer is the durable consumer the monitor alert
	// processor reads.
	MonitorAlertFanoutConsumer = "notification-fanout"

	streamMaxAge = 72 * time.Hour
	// natsAckWait is how long a handler has before an unanswered message is
	// redelivered; it must outlast a delivery to a destination.
	natsAckWait = 30 * time.Second
	// natsPullBuffer bounds the messages fetched ahead of the handler. The
	// ack timer starts at fetch, so a large buffer behind a few slow
	// deliveries would time out and be redelivered, sending them twice.
	natsPullBuffer = 16
	// natsQueueMaxAckPending bounds the work items a queue consumer holds
	// unacknowledged. An item retried with a delay stays pending until the
	// delay runs out, so the server's default of 1000 would let one failing
	// destination's backlog of retries stop every delivery for hours.
	natsQueueMaxAckPending = 50_000
	// Bound poison-event retries so one bad event cannot occupy the consumer forever.
	natsEventMaxDeliver = EventMaxDeliver
)

// EventMaxDeliver is how many times an event consumer is handed one event
// before the bus gives up on it.
const EventMaxDeliver = 20

// provisionedConsumers are the durable event consumers Setup creates before
// publishing starts, so their workers can attach without losing events. A
// consumer is listed together with the worker that reads it: one created
// with no reader would hold every event until that worker ships, then
// deliver up to streamMaxAge of them at once.
var provisionedConsumers = []string{EventFanoutConsumer, MonitorAlertFanoutConsumer}

// natsQueues maps work queue names to their JetStream streams and subjects.
// A queue's subjects must stay outside amp.events.>, which AMP_EVENTS
// captures: JetStream refuses streams with overlapping subjects.
var natsQueues = map[string]struct{ stream, subject string }{
	QueueEventDeliveries: {stream: "AMP_EVENT_DELIVERIES", subject: "amp.event-deliveries"},
}

// Subject is the NATS subject an event is published on.
func Subject(e Event) string {
	return fmt.Sprintf("%s.%s.%s", EventsSubject, e.OrgID, e.Name())
}

// NATSBus is a Bus on NATS JetStream: a NATS server (streams on disk) or an
// embedded in-process server (streams in memory).
type NATSBus struct {
	nc *nats.Conn
	js jetstream.JetStream
	// storage is where the streams keep messages.
	storage jetstream.StorageType
	// embedded is set when the server runs in this process.
	embedded *server.Server
	storeDir string
}

var _ Bus = (*NATSBus)(nil)

// Connect dials a NATS server, authenticating with token when it is set. It
// keeps retrying in the background when the server is not up yet, so a slow
// NATS start does not stop the service.
func Connect(url, token, name string, logger *slog.Logger) (*NATSBus, error) {
	nc, err := nats.Connect(url,
		nats.Name(name),
		nats.Token(token),
		nats.RetryOnFailedConnect(true),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			if err != nil {
				logger.Warn("Disconnected from NATS", "error", err)
			}
		}),
		nats.ReconnectHandler(func(*nats.Conn) { logger.Info("Reconnected to NATS") }),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		return nil, fmt.Errorf("failed to open JetStream: %w", err)
	}
	return &NATSBus{nc: nc, js: js, storage: jetstream.FileStorage}, nil
}

// Setup creates or updates the streams and durable event consumers. Every
// replica provisions the same consumers before publishing, so DeliverNewPolicy
// starts at initial setup rather than whenever the first worker attaches.
func (b *NATSBus) Setup(ctx context.Context) error {
	eventsCfg := jetstream.StreamConfig{
		Name:       EventsStream,
		Subjects:   []string{EventsSubject + ".>"},
		Storage:    b.storage,
		Retention:  jetstream.LimitsPolicy,
		MaxAge:     streamMaxAge,
		Duplicates: 2 * time.Minute,
	}
	if b.storage == jetstream.MemoryStorage {
		eventsCfg.MaxBytes = embeddedMaxMemory / 2
		eventsCfg.Discard = jetstream.DiscardOld
	}
	if _, err := b.js.CreateOrUpdateStream(ctx, eventsCfg); err != nil {
		return fmt.Errorf("failed to set up stream %s: %w", EventsStream, err)
	}
	for _, name := range provisionedConsumers {
		if _, err := b.js.CreateOrUpdateConsumer(ctx, EventsStream, eventConsumerConfig(name)); err != nil {
			return fmt.Errorf("failed to set up consumer %s: %w", name, err)
		}
	}
	for _, q := range natsQueues {
		queueCfg := jetstream.StreamConfig{
			Name:       q.stream,
			Subjects:   []string{q.subject + ".>"},
			Storage:    b.storage,
			Retention:  jetstream.WorkQueuePolicy,
			MaxAge:     streamMaxAge,
			Duplicates: 10 * time.Minute,
		}
		if b.storage == jetstream.MemoryStorage {
			// The queues share what the event stream leaves, with headroom.
			// A full queue refuses new items rather than dropping queued
			// ones: the fan-out then retries the event later.
			queueCfg.MaxBytes = embeddedMaxMemory / 4 / int64(len(natsQueues))
			queueCfg.Discard = jetstream.DiscardNew
		}
		if _, err := b.js.CreateOrUpdateStream(ctx, queueCfg); err != nil {
			return fmt.Errorf("failed to set up stream %s: %w", q.stream, err)
		}
	}
	return nil
}

func (b *NATSBus) PublishEvent(ctx context.Context, e Event, body []byte) error {
	_, err := b.js.Publish(ctx, Subject(e), body, jetstream.WithMsgID(e.ID))
	return err
}

func (b *NATSBus) SubscribeEvents(ctx context.Context, consumer string, handle func(Message)) (Subscription, error) {
	return b.consume(ctx, EventsStream, eventConsumerConfig(consumer), handle)
}

func eventConsumerConfig(consumer string) jetstream.ConsumerConfig {
	return jetstream.ConsumerConfig{
		Durable:       consumer,
		MaxDeliver:    natsEventMaxDeliver,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       natsAckWait,
		DeliverPolicy: jetstream.DeliverNewPolicy,
		FilterSubject: EventsSubject + ".>",
	}
}

func (b *NATSBus) Enqueue(ctx context.Context, queue, key string, body []byte) error {
	q, ok := natsQueues[queue]
	if !ok {
		return fmt.Errorf("unknown queue %q", queue)
	}
	_, err := b.js.Publish(ctx, q.subject+".item", body, jetstream.WithMsgID(key))
	return err
}

func (b *NATSBus) Consume(ctx context.Context, queue, consumer string, handle func(Message)) (Subscription, error) {
	q, ok := natsQueues[queue]
	if !ok {
		return nil, fmt.Errorf("unknown queue %q", queue)
	}
	return b.consume(ctx, q.stream, jetstream.ConsumerConfig{
		Durable:       consumer,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       natsAckWait,
		MaxAckPending: natsQueueMaxAckPending,
	}, handle)
}

func (b *NATSBus) consume(
	ctx context.Context, stream string, cfg jetstream.ConsumerConfig, handle func(Message),
) (Subscription, error) {
	c, err := b.js.CreateOrUpdateConsumer(ctx, stream, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer %s: %w", cfg.Durable, err)
	}
	cc, err := c.Consume(func(m jetstream.Msg) { handle(natsMessage{m}) }, jetstream.PullMaxMessages(natsPullBuffer))
	if err != nil {
		return nil, fmt.Errorf("failed to consume %s: %w", stream, err)
	}
	return &natsSubscription{ConsumeContext: cc}, nil
}

// natsSubscription joins the consumer callback before Stop returns, so callers
// may safely wait for workers after stopping intake.
type natsSubscription struct{ jetstream.ConsumeContext }

func (s *natsSubscription) Stop() {
	s.ConsumeContext.Stop()
	<-s.Closed()
}

// Close drains the connection, and stops the embedded server if there is one.
func (b *NATSBus) Close() {
	if b == nil {
		return
	}
	if b.nc != nil {
		if b.embedded != nil {
			// Drain is asynchronous; the in-process server must outlive it.
			b.nc.Close()
		} else {
			_ = b.nc.Drain()
		}
	}
	if b.embedded != nil {
		b.embedded.Shutdown()
		b.embedded.WaitForShutdown()
		_ = os.RemoveAll(b.storeDir)
	}
}

// natsMessage adapts a JetStream message to Message.
type natsMessage struct{ m jetstream.Msg }

func (n natsMessage) Data() []byte { return n.m.Data() }

func (n natsMessage) Attempt() int {
	if meta, err := n.m.Metadata(); err == nil {
		return int(meta.NumDelivered)
	}
	return 1
}

func (n natsMessage) Ack() error                      { return n.m.Ack() }
func (n natsMessage) Retry(after time.Duration) error { return n.m.NakWithDelay(after) }
func (n natsMessage) Drop() error                     { return n.m.Term() }
