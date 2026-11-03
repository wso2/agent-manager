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

package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations"
)

const (
	eventFanoutConsumer   = events.EventFanoutConsumer
	eventDeliveryConsumer = "event-delivery"
	eventFanoutRetryDelay = 5 * time.Second
	eventDeliveryWorkers  = 8
	eventPruneInterval    = time.Hour
	// eventDeliveryClaim is how long a worker holds a delivery while sending
	// it. It outlasts a driver's send timeout, so a job the bus redelivers
	// meanwhile waits instead of sending the event a second time.
	eventDeliveryClaim = 2 * time.Minute
)

// eventRetryBackoff is the delay before each retry; attempts beyond its
// length reuse the last delay.
var eventRetryBackoff = []time.Duration{
	10 * time.Second,
	1 * time.Minute,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
}

// eventDeliveryJob is one message on the deliveries queue: an event bound
// for one destination. The subscriptions that send it there are recorded on
// the delivery's row.
type eventDeliveryJob struct {
	DestinationID string          `json:"destinationId"`
	Event         json.RawMessage `json:"event"`
}

// EventDispatcherService consumes published events and delivers each one to
// the destinations of the subscriptions it matches.
//
// It runs in two stages so one slow or failing destination does not hold
// up, or cause duplicates at, the others. The fan-out stage reads each
// event, finds the subscriptions it matches and queues one delivery per
// destination, however many of them send there. The delivery stage hands
// each one to the destination kind's driver, retrying with backoff until it
// succeeds or runs out of attempts. The work lives on the events.Bus, so
// every replica can run the dispatcher: each message goes to one of them.
type EventDispatcherService interface {
	Start(ctx context.Context) error
	Stop() error
}

type eventDispatcher struct {
	bus      events.Bus
	repo     repositories.EventRepository
	resolver DestinationResolver
	cfg      config.EventsConfig
	logger   *slog.Logger
	now      func() time.Time

	subs    []events.Subscription
	workers chan struct{}
	wg      sync.WaitGroup
	cancel  context.CancelFunc
}

// NewEventDispatcherService creates the dispatcher. bus may be nil (events
// off), in which case Start does nothing.
func NewEventDispatcherService(
	bus events.Bus,
	repo repositories.EventRepository,
	resolver DestinationResolver,
	cfg config.Config,
	logger *slog.Logger,
) EventDispatcherService {
	return &eventDispatcher{
		bus:      bus,
		repo:     repo,
		resolver: resolver,
		cfg:      cfg.Events,
		logger:   logger.With("component", "event-dispatcher"),
		now:      time.Now,
		workers:  make(chan struct{}, eventDeliveryWorkers),
	}
}

func (d *eventDispatcher) Start(ctx context.Context) error {
	if d.bus == nil {
		d.logger.Info("Event dispatcher not started: events are off")
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	fanout, err := d.bus.SubscribeEvents(runCtx, eventFanoutConsumer, func(msg events.Message) {
		d.recovering(msg, eventFanoutRetryDelay, func() { d.handleEvent(runCtx, msg) })
	})
	if err != nil {
		cancel()
		return fmt.Errorf("failed to consume events: %w", err)
	}
	delivery, err := d.bus.Consume(runCtx, events.QueueEventDeliveries, eventDeliveryConsumer,
		func(msg events.Message) {
			// Deliveries run on a bounded pool so one slow destination does
			// not stall the rest; the pool blocks the consumer when every
			// worker is busy.
			select {
			case d.workers <- struct{}{}:
			case <-runCtx.Done():
				return
			}
			d.wg.Add(1)
			go func() {
				defer func() { <-d.workers; d.wg.Done() }()
				d.recovering(msg, eventBackoff(len(eventRetryBackoff)), func() { d.handleDelivery(runCtx, msg) })
			}()
		})
	if err != nil {
		cancel()
		fanout.Stop()
		return fmt.Errorf("failed to consume event deliveries: %w", err)
	}
	d.subs = []events.Subscription{fanout, delivery}

	d.wg.Add(1)
	go d.pruneLoop(runCtx)

	d.logger.Info("Event dispatcher started", "maxAttempts", d.maxAttempts())
	return nil
}

func (d *eventDispatcher) Stop() error {
	if d.cancel != nil {
		d.cancel()
	}
	for _, sub := range d.subs {
		sub.Stop()
	}
	d.wg.Wait()
	return nil
}

// recovering runs handle, turning a panic into a retry of msg after delay
// rather than a crash: the bus would hand the same message to the next
// replica, and crash that one too.
func (d *eventDispatcher) recovering(msg events.Message, delay time.Duration, handle func()) {
	defer func() {
		if r := recover(); r != nil {
			d.logger.Error("Event handler panicked; will retry", "panic", r, "stack", string(debug.Stack()))
			_ = msg.Retry(delay)
		}
	}()
	handle()
}

// handleEvent fans one event out to the destinations of the subscriptions it
// matches.
func (d *eventDispatcher) handleEvent(ctx context.Context, msg events.Message) {
	var e events.Event
	if err := json.Unmarshal(msg.Data(), &e); err != nil || e.ID == "" || e.OrgID == "" {
		d.logger.Error("Dropping malformed event", "error", err)
		_ = msg.Drop()
		return
	}
	log := d.logger.With("eventId", e.ID, "type", e.Name())

	candidates, err := d.repo.CandidateSubscriptions(ctx, e.OrgID, e.Project, e.Agent)
	if err != nil {
		log.Error("Failed to find event subscriptions; will retry", "error", err)
		_ = msg.Retry(eventFanoutRetryDelay)
		return
	}
	wanted := map[uuid.UUID]bool{}
	for i := range candidates {
		if subscriptionWants(&candidates[i], e) {
			wanted[candidates[i].DestinationID] = true
		}
	}
	if len(wanted) == 0 {
		_ = msg.Ack()
		return
	}
	ids := make([]uuid.UUID, 0, len(wanted))
	for id := range wanted {
		ids = append(ids, id)
	}
	found, err := d.repo.DestinationsByID(ctx, e.OrgID, ids)
	if err != nil {
		log.Error("Failed to load event destinations; will retry", "error", err)
		_ = msg.Retry(eventFanoutRetryDelay)
		return
	}
	destinations := make(map[uuid.UUID]*models.EventDestination, len(found))
	for i := range found {
		destinations[found[i].ID] = &found[i]
	}

	for _, g := range groupByDestination(candidates, destinations, e) {
		// Recorded before it is queued: a delivery worker can finish before
		// this handler continues, and its outcome must not be overwritten.
		if err := d.repo.RecordQueuedDelivery(ctx, &models.EventDelivery{
			OUID:            e.OrgID,
			DestinationID:   g.Destination.ID,
			SubscriptionIDs: g.SubscriptionIDs,
			EventID:         e.ID,
			EventType:       e.Name(),
			EventScope:      string(e.Scope),
			ProjectName:     e.Project,
			AgentName:       e.Agent,
			Environment:     e.Environment,
			Status:          models.EventDeliveryPending,
		}); err != nil {
			log.Error("Failed to record event delivery; will retry", "destinationId", g.Destination.ID, "error", err)
			_ = msg.Retry(eventFanoutRetryDelay)
			return
		}
		body, err := json.Marshal(eventDeliveryJob{DestinationID: g.Destination.ID.String(), Event: msg.Data()})
		if err != nil {
			log.Error("Failed to encode event delivery", "destinationId", g.Destination.ID, "error", err)
			continue
		}
		// The key makes a repeated fan-out of the same event (after a crash
		// before the ack) a no-op for destinations already queued.
		if err := d.bus.Enqueue(ctx, events.QueueEventDeliveries, e.ID+":"+g.Destination.ID.String(), body); err != nil {
			log.Error("Failed to queue event delivery; will retry", "destinationId", g.Destination.ID, "error", err)
			_ = msg.Retry(eventFanoutRetryDelay)
			return
		}
	}
	_ = msg.Ack()
}

// handleDelivery delivers one event to one destination.
func (d *eventDispatcher) handleDelivery(ctx context.Context, msg events.Message) {
	var job eventDeliveryJob
	if err := json.Unmarshal(msg.Data(), &job); err != nil {
		d.logger.Error("Dropping malformed event delivery", "error", err)
		_ = msg.Drop()
		return
	}
	destinationID, err := uuid.Parse(job.DestinationID)
	if err != nil {
		d.logger.Error("Dropping event delivery with an invalid destination ID", "destinationId", job.DestinationID)
		_ = msg.Drop()
		return
	}
	var e events.Event
	_ = json.Unmarshal(job.Event, &e)
	log := d.logger.With("destinationId", destinationID, "eventId", e.ID)

	delivery, err := d.repo.GetDelivery(ctx, destinationID, e.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = msg.Drop() // the destination was deleted, and its log with it
			return
		}
		log.Error("Failed to load event delivery; will retry", "error", err)
		_ = msg.Retry(eventFanoutRetryDelay)
		return
	}
	// A terminal delivery is not processed again if its acknowledgement or
	// termination was lost. Failed rows cannot be claimed for another send.
	if delivery.Status == models.EventDeliveryDelivered || delivery.Status == models.EventDeliverySkipped || delivery.Status == models.EventDeliveryFailed {
		_ = msg.Ack()
		return
	}
	dest, err := d.repo.GetDestination(ctx, delivery.OUID, destinationID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = msg.Drop()
			return
		}
		log.Error("Failed to load event destination; will retry", "error", err)
		_ = msg.Retry(eventFanoutRetryDelay)
		return
	}
	// The subscriptions or monitor alert settings may have been disabled,
	// deleted or cut off from the destination since; a retry hours later
	// must not send what nobody wants any more.
	stillWanted, err := d.stillWanted(ctx, delivery, dest, e)
	if err != nil {
		log.Error("Failed to check event delivery eligibility; will retry", "error", err)
		_ = msg.Retry(eventFanoutRetryDelay)
		return
	}
	if !stillWanted {
		if err := d.record(ctx, delivery, models.EventDeliverySkipped, delivery.Attempts, nil, "no active subscription or monitor alert wants this event any more", nil); err != nil {
			_ = msg.Retry(eventFanoutRetryDelay)
			return
		}
		_ = msg.Drop()
		return
	}
	driver, target, err := d.resolver.Resolve(dest)
	if err != nil {
		if err := d.record(ctx, delivery, models.EventDeliveryFailed, delivery.Attempts, nil, "destination settings unreadable", nil); err != nil {
			_ = msg.Retry(eventFanoutRetryDelay)
			return
		}
		log.Error("Cannot deliver to event destination", "error", err)
		_ = msg.Drop()
		return
	}

	// The bus redelivers a job whose acknowledgement is late, so two workers
	// can hold the same job. Only the one holding the claim sends; the other
	// comes back after the claim, when the delivery is finished or its
	// worker has died.
	claimed, err := d.repo.ClaimDelivery(ctx, delivery.ID, eventDeliveryClaim)
	if err != nil {
		log.Error("Failed to claim event delivery; will retry", "error", err)
		_ = msg.Retry(eventFanoutRetryDelay)
		return
	}
	if !claimed {
		_ = msg.Retry(eventDeliveryClaim)
		return
	}

	// Bus redeliveries include database failures before a send. Only actual
	// destination attempts consume the send budget or advance its backoff.
	attempt := delivery.Attempts + 1
	res := driver.Deliver(ctx, target, eventdestinations.Message{ID: e.ID, Type: e.Type, Body: job.Event})
	var code *int
	if res.StatusCode != 0 {
		code = &res.StatusCode
	}
	if res.Err == nil {
		now := d.now()
		if err := d.record(ctx, delivery, models.EventDeliveryDelivered, attempt, code, "", &now); err != nil {
			// The event was sent. Retrying the job would send it again, which
			// pages people twice; a log row left pending is the lesser harm.
			log.Error("Event delivered but its outcome could not be recorded", "error", err)
		}
		_ = msg.Ack()
		return
	}
	if attempt >= d.maxAttempts() || !res.Retryable {
		if err := d.record(ctx, delivery, models.EventDeliveryFailed, attempt, code, res.Err.Error(), nil); err != nil {
			_ = msg.Retry(eventFanoutRetryDelay)
			return
		}
		log.Warn("Event delivery failed", "kind", dest.Kind, "attempts", attempt, "error", res.Err)
		_ = msg.Drop()
		return
	}
	if err := d.record(ctx, delivery, models.EventDeliveryPending, attempt, code, res.Err.Error(), nil); err != nil {
		_ = msg.Retry(eventFanoutRetryDelay)
		return
	}
	_ = msg.Retry(eventBackoff(attempt))
}

// stillWanted checks that the subscription or monitor alert that queued a
// delivery still exists, is enabled and may still use the destination.
func (d *eventDispatcher) stillWanted(
	ctx context.Context, delivery *models.EventDelivery, dest *models.EventDestination, e events.Event,
) (bool, error) {
	ids := make([]uuid.UUID, 0, len(delivery.SubscriptionIDs))
	for _, raw := range delivery.SubscriptionIDs {
		if id, err := uuid.Parse(raw); err == nil {
			ids = append(ids, id)
		}
	}
	subs, err := d.repo.SubscriptionsByID(ctx, delivery.OUID, ids)
	if err != nil {
		return false, err
	}
	for i := range subs {
		if subscriptionWants(&subs[i], e) && subs[i].DestinationID == dest.ID && dest.UsableBy(subs[i].Scope, subs[i].ProjectName) {
			return true, nil
		}
	}
	return false, nil
}

func (d *eventDispatcher) maxAttempts() int {
	return max(d.cfg.DeliveryMaxAttempts, 1)
}

func eventBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(eventRetryBackoff) {
		return eventRetryBackoff[len(eventRetryBackoff)-1]
	}
	return eventRetryBackoff[attempt-1]
}

func (d *eventDispatcher) record(
	ctx context.Context, delivery *models.EventDelivery, status string,
	attempts int, code *int, lastErr string, deliveredAt *time.Time,
) error {
	update := *delivery
	// A fresh row ID: the upsert matches the existing row by destination
	// and event.
	update.ID = uuid.Nil
	update.Status = status
	update.Attempts = attempts
	update.ResponseCode = code
	update.LastError = lastErr
	update.DeliveredAt = deliveredAt
	if err := d.repo.RecordDelivery(ctx, &update); err != nil {
		d.logger.Error("Failed to record event delivery", "destinationId", delivery.DestinationID,
			"eventId", delivery.EventID, "error", err)
		return fmt.Errorf("failed to record event delivery: %w", err)
	}
	return nil
}

func (d *eventDispatcher) pruneLoop(ctx context.Context) {
	defer d.wg.Done()
	ticker := time.NewTicker(eventPruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			days := max(d.cfg.DeliveryRetentionDays, 1)
			cutoff := d.now().Add(-time.Duration(days) * 24 * time.Hour)
			if n, err := d.repo.PruneDeliveries(ctx, cutoff); err != nil {
				d.logger.Error("Failed to prune event deliveries", "error", err)
			} else if n > 0 {
				d.logger.Info("Pruned event deliveries", "count", n)
			}
		}
	}
}
