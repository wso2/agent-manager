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
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const (
	webhookFanoutConsumer   = "webhook-fanout"
	webhookDeliveryConsumer = "webhook-delivery"
	webhookFanoutRetryDelay = 5 * time.Second
	webhookDeliveryWorkers  = 8
	webhookPruneInterval    = time.Hour
)

// webhookRetryBackoff is the delay before each retry; attempts beyond its
// length reuse the last delay.
var webhookRetryBackoff = []time.Duration{
	10 * time.Second,
	1 * time.Minute,
	5 * time.Minute,
	30 * time.Minute,
	2 * time.Hour,
}

// webhookDelivery is one message on the deliveries stream: an event bound for
// one endpoint.
type webhookDelivery struct {
	EndpointID string          `json:"endpointId"`
	Event      json.RawMessage `json:"event"`
}

// WebhookDispatcherService consumes published events and delivers them to
// the matching webhook endpoints.
//
// It runs in two stages so one slow or failing endpoint does not hold up, or
// cause duplicates at, the others. The fan-out stage reads each event, finds
// the endpoints subscribed to it and queues one delivery per endpoint. The
// delivery stage sends each one, retrying with backoff until it succeeds or
// runs out of attempts. The work lives on the events.Bus, so the dispatcher
// works with any transport that implements it, and every replica can run a
// dispatcher: each message goes to one of them.
type WebhookDispatcherService interface {
	Start(ctx context.Context) error
	Stop()
}

type webhookDispatcher struct {
	bus           events.Bus
	repo          repositories.WebhookRepository
	sender        WebhookSender
	encryptionKey []byte
	cfg           config.WebhooksConfig
	logger        *slog.Logger
	now           func() time.Time

	subs    []events.Subscription
	workers chan struct{}
	wg      sync.WaitGroup
	cancel  context.CancelFunc
}

// NewWebhookDispatcherService creates the dispatcher. bus may be nil, in which
// case Start does nothing.
func NewWebhookDispatcherService(
	bus events.Bus,
	repo repositories.WebhookRepository,
	sender WebhookSender,
	encryptionKey []byte,
	cfg config.Config,
	logger *slog.Logger,
) WebhookDispatcherService {
	return &webhookDispatcher{
		bus:           bus,
		repo:          repo,
		sender:        sender,
		encryptionKey: encryptionKey,
		cfg:           cfg.Webhooks,
		logger:        logger.With("component", "webhook-dispatcher"),
		now:           time.Now,
		workers:       make(chan struct{}, webhookDeliveryWorkers),
	}
}

func (d *webhookDispatcher) Start(ctx context.Context) error {
	if d.bus == nil {
		d.logger.Info("Webhook dispatcher not started: events are not configured")
		return nil
	}
	if !d.cfg.DispatcherEnabled {
		d.logger.Info("Webhook dispatcher disabled on this instance")
		return nil
	}
	runCtx, cancel := context.WithCancel(ctx)
	d.cancel = cancel

	fanout, err := d.bus.SubscribeEvents(runCtx, webhookFanoutConsumer, func(msg events.Message) {
		d.handleEvent(runCtx, msg)
	})
	if err != nil {
		cancel()
		return fmt.Errorf("failed to consume events: %w", err)
	}
	delivery, err := d.bus.Consume(runCtx, events.QueueWebhookDeliveries, webhookDeliveryConsumer,
		func(msg events.Message) {
			// Sends run on a bounded pool so one slow endpoint does not stall
			// the rest; the pool blocks the consumer when every worker is busy.
			d.workers <- struct{}{}
			d.wg.Add(1)
			go func() {
				defer func() { <-d.workers; d.wg.Done() }()
				d.handleDelivery(runCtx, msg)
			}()
		})
	if err != nil {
		fanout.Stop()
		cancel()
		return fmt.Errorf("failed to consume deliveries: %w", err)
	}
	d.subs = []events.Subscription{fanout, delivery}

	d.wg.Add(1)
	go d.pruneLoop(runCtx)

	d.logger.Info("Webhook dispatcher started", "maxAttempts", d.cfg.MaxAttempts)
	return nil
}

func (d *webhookDispatcher) Stop() {
	for _, sub := range d.subs {
		sub.Stop()
	}
	if d.cancel != nil {
		d.cancel()
	}
	d.wg.Wait()
}

// handleEvent fans one event out to its endpoints.
func (d *webhookDispatcher) handleEvent(ctx context.Context, msg events.Message) {
	var e events.Event
	if err := json.Unmarshal(msg.Data(), &e); err != nil || e.ID == "" || e.OrgID == "" {
		d.logger.Error("Dropping malformed event", "error", err)
		_ = msg.Drop()
		return
	}
	log := d.logger.With("eventId", e.ID, "type", e.Name())

	endpoints, err := d.repo.MatchingEndpoints(ctx, e.OrgID, string(e.Scope), e.Project, e.Agent, e.Environment)
	if err != nil {
		log.Error("Failed to find webhook endpoints; will retry", "error", err)
		_ = msg.Retry(webhookFanoutRetryDelay)
		return
	}
	for i := range endpoints {
		ep := &endpoints[i]
		if !ep.Subscribes(e.Name()) {
			continue
		}
		// Recorded before it is queued: a delivery worker can finish before
		// this handler continues, and its outcome must not be overwritten.
		if err := d.repo.RecordQueuedDelivery(ctx, &models.WebhookDelivery{
			EndpointID: ep.ID, OUID: ep.OUID, EventID: e.ID, EventType: e.Name(),
			Status: models.WebhookDeliveryPending,
		}); err != nil {
			log.Error("Failed to record webhook delivery; will retry", "endpointId", ep.ID, "error", err)
			_ = msg.Retry(webhookFanoutRetryDelay)
			return
		}
		body, _ := json.Marshal(webhookDelivery{EndpointID: ep.ID.String(), Event: msg.Data()})
		// The key makes a repeated fan-out of the same event (after a crash
		// before the ack) a no-op for endpoints already queued.
		if err := d.bus.Enqueue(ctx, events.QueueWebhookDeliveries, e.ID+":"+ep.ID.String(), body); err != nil {
			log.Error("Failed to queue webhook delivery; will retry", "endpointId", ep.ID, "error", err)
			_ = msg.Retry(webhookFanoutRetryDelay)
			return
		}
	}

	_ = msg.Ack()
}

// handleDelivery sends one event to one endpoint.
func (d *webhookDispatcher) handleDelivery(ctx context.Context, msg events.Message) {
	var del webhookDelivery
	if err := json.Unmarshal(msg.Data(), &del); err != nil {
		d.logger.Error("Dropping malformed delivery", "error", err)
		_ = msg.Drop()
		return
	}
	endpointID, err := uuid.Parse(del.EndpointID)
	if err != nil {
		d.logger.Error("Dropping delivery with an invalid endpoint ID", "endpointId", del.EndpointID)
		_ = msg.Drop()
		return
	}
	var head events.Event
	_ = json.Unmarshal(del.Event, &head)
	// The delivery log keeps the short catalog name; the request carries the
	// full CloudEvents type.
	eventName := head.Name()

	ep, err := d.repo.GetEndpointByID(ctx, endpointID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			_ = msg.Drop() // endpoint deleted since the event was queued
			return
		}
		d.logger.Error("Failed to load webhook endpoint; will retry", "endpointId", endpointID, "error", err)
		_ = msg.Retry(webhookFanoutRetryDelay)
		return
	}
	if !ep.Enabled {
		d.record(ctx, ep, head.ID, eventName, models.WebhookDeliveryFailed, 0, nil, "endpoint disabled", nil)
		_ = msg.Drop()
		return
	}
	target, err := webhookTargetFor(ep, d.encryptionKey)
	if err != nil {
		d.record(ctx, ep, head.ID, eventName, models.WebhookDeliveryFailed, 0, nil, "signing secret unreadable", nil)
		_ = msg.Drop()
		return
	}

	attempt := msg.Attempt()
	code, sendErr := d.sender.Send(ctx, target, head.ID, head.Type, del.Event)
	var codePtr *int
	if code != 0 {
		codePtr = &code
	}
	if sendErr == nil {
		now := d.now()
		d.record(ctx, ep, head.ID, eventName, models.WebhookDeliveryDelivered, attempt, codePtr, "", &now)
		_ = msg.Ack()
		return
	}

	if attempt >= d.maxAttempts() || !retryableStatus(code) {
		d.record(ctx, ep, head.ID, eventName, models.WebhookDeliveryFailed, attempt, codePtr, sendErr.Error(), nil)
		d.logger.Warn("Webhook delivery failed", "endpointId", ep.ID, "eventId", head.ID, "attempts", attempt, "error", sendErr)
		_ = msg.Drop()
		return
	}
	d.record(ctx, ep, head.ID, eventName, models.WebhookDeliveryPending, attempt, codePtr, sendErr.Error(), nil)
	_ = msg.Retry(webhookBackoff(attempt))
}

func (d *webhookDispatcher) maxAttempts() int {
	if d.cfg.MaxAttempts < 1 {
		return 1
	}
	return d.cfg.MaxAttempts
}

// retryableStatus reports whether a failed response is worth retrying. A
// network error (code 0), a 5xx, 408, 425 or 429 is; other 4xx answers mean
// the request itself was refused and will be refused again.
func retryableStatus(code int) bool {
	if code == 0 || code >= 500 {
		return true
	}
	return code == 408 || code == 425 || code == 429
}

func webhookBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	if attempt > len(webhookRetryBackoff) {
		return webhookRetryBackoff[len(webhookRetryBackoff)-1]
	}
	return webhookRetryBackoff[attempt-1]
}

func (d *webhookDispatcher) record(
	ctx context.Context, ep *models.WebhookEndpoint, eventID, eventType, status string,
	attempts int, code *int, lastErr string, deliveredAt *time.Time,
) {
	err := d.repo.RecordDelivery(ctx, &models.WebhookDelivery{
		EndpointID:   ep.ID,
		OUID:         ep.OUID,
		EventID:      eventID,
		EventType:    eventType,
		Status:       status,
		Attempts:     attempts,
		ResponseCode: code,
		LastError:    lastErr,
		DeliveredAt:  deliveredAt,
	})
	if err != nil {
		d.logger.Error("Failed to record webhook delivery", "endpointId", ep.ID, "eventId", eventID, "error", err)
	}
}

func (d *webhookDispatcher) pruneLoop(ctx context.Context) {
	defer d.wg.Done()
	ticker := time.NewTicker(webhookPruneInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			days := max(d.cfg.DeliveryRetentionDays, 1)
			cutoff := d.now().Add(-time.Duration(days) * 24 * time.Hour)
			if n, err := d.repo.PruneDeliveries(ctx, cutoff); err != nil {
				d.logger.Error("Failed to prune webhook deliveries", "error", err)
			} else if n > 0 {
				d.logger.Info("Pruned webhook deliveries", "count", n)
			}
		}
	}
}
