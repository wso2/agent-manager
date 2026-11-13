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
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"gorm.io/gorm"
)

const (
	defaultGatewayStatePageSize = 200
)

// SQLBackendConfig holds configuration for the SQL backend
type SQLBackendConfig struct {
	PollInterval         time.Duration
	CleanupInterval      time.Duration
	RetentionPeriod      time.Duration
	GatewayStatePageSize int
}

// DefaultSQLBackendConfig returns a SQLBackendConfig with sensible defaults
func DefaultSQLBackendConfig() SQLBackendConfig {
	return SQLBackendConfig{
		PollInterval:         2 * time.Second,
		CleanupInterval:      5 * time.Minute,
		RetentionPeriod:      1 * time.Hour,
		GatewayStatePageSize: defaultGatewayStatePageSize,
	}
}

// SQLBackend implements EventHub by polling the gateway_event_states and gateway_events tables.
type SQLBackend struct {
	gormDB *gorm.DB
	logger *slog.Logger
	config SQLBackendConfig
	clock  func() time.Time

	registry *gatewayRegistry

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

var _ EventHub = (*SQLBackend)(nil)

// NewSQLBackend creates a new SQL-backed EventHub. Publishes run on db, so a caller's GORM transaction can carry them.
func NewSQLBackend(db *gorm.DB, logger *slog.Logger, config SQLBackendConfig) (*SQLBackend, error) {
	ctx, cancel := context.WithCancel(context.Background())
	return &SQLBackend{
		gormDB:   db,
		logger:   logger,
		config:   config,
		clock:    time.Now,
		registry: newGatewayRegistry(),
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// eventStateRow is a gateway's row in gateway_event_states.
type eventStateRow struct {
	GatewayID string    `gorm:"column:gateway_id;primaryKey"`
	OrgID     string    `gorm:"column:org_id"`
	VersionID *string   `gorm:"column:version_id"`
	Seq       int64     `gorm:"column:seq"`
	UpdatedAt time.Time `gorm:"column:updated_at"`
}

func (eventStateRow) TableName() string { return "gateway_event_states" }

// eventRow is one row of gateway_events.
type eventRow struct {
	ID           string    `gorm:"column:id;primaryKey"`
	OrgID        string    `gorm:"column:org_id"`
	GatewayID    string    `gorm:"column:gateway_id"`
	Seq          int64     `gorm:"column:seq"`
	EntityType   string    `gorm:"column:entity_type"`
	Action       string    `gorm:"column:action"`
	EntityID     string    `gorm:"column:entity_id"`
	EventData    string    `gorm:"column:event_data"`
	OriginatedAt time.Time `gorm:"column:originated_at"`
	ProcessedAt  time.Time `gorm:"column:processed_at"`
}

func (eventRow) TableName() string { return "gateway_events" }

// ensureGatewayState creates the gateway's state row if it has none, taking its organization from the gateway.
func ensureGatewayState(tx *gorm.DB, gatewayID string, now time.Time) (string, error) {
	var gw struct{ OrgID string }
	if err := tx.Table("gateways").Select("org_id").Where("id = ?", gatewayID).Take(&gw).Error; err != nil {
		return "", fmt.Errorf("gateway %q: %w", gatewayID, err)
	}
	if _, err := dbx.InsertIfAbsent(tx, &eventStateRow{GatewayID: gatewayID, OrgID: gw.OrgID, UpdatedAt: now}); err != nil {
		return "", err
	}
	return gw.OrgID, nil
}

// eventUUID is the event's id as stored: the id itself when it is a UUID, else a UUID derived from it, so a
// re-published event still deduplicates.
func eventUUID(eventID string) string {
	if id, err := uuid.Parse(eventID); err == nil {
		return id.String()
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(eventID)).String()
}

// Initialize prepares statements and starts background goroutines.
func (b *SQLBackend) Initialize() error {
	if b.config.PollInterval <= 0 {
		return fmt.Errorf("invalid config: poll_interval must be > 0, got %s", b.config.PollInterval)
	}
	if b.config.CleanupInterval <= 0 {
		return fmt.Errorf("invalid config: cleanup_interval must be > 0, got %s", b.config.CleanupInterval)
	}
	if b.config.RetentionPeriod <= 0 {
		return fmt.Errorf("invalid config: retention_period must be > 0, got %s", b.config.RetentionPeriod)
	}

	b.wg.Add(2)
	go b.pollLoop()
	go b.cleanupLoop()

	b.logger.Info("EventHub initialized",
		slog.String("poll_interval", b.config.PollInterval.String()),
		slog.String("cleanup_interval", b.config.CleanupInterval.String()),
		slog.String("retention_period", b.config.RetentionPeriod.String()))

	return nil
}

// normalizeGatewayID trims whitespace and returns an error if the result is empty.
func normalizeGatewayID(gatewayID string) (string, error) {
	gatewayID = strings.TrimSpace(gatewayID)
	if gatewayID == "" {
		return "", fmt.Errorf("gateway_id cannot be empty")
	}
	return gatewayID, nil
}

// RegisterGateway registers a new gateway for event tracking.
func (b *SQLBackend) RegisterGateway(gatewayID string) error {
	var err error
	if gatewayID, err = normalizeGatewayID(gatewayID); err != nil {
		return err
	}

	if _, err := ensureGatewayState(b.gormDB.WithContext(b.ctx), gatewayID, dbx.Now()); err != nil {
		return fmt.Errorf("failed to register gateway: %w", err)
	}

	if regErr := b.registry.register(gatewayID); regErr != nil && !errors.Is(regErr, ErrGatewayAlreadyExists) {
		return fmt.Errorf("failed to register gateway in registry: %w", regErr)
	}

	b.logger.Info("Gateway registered for event tracking", slog.String("gateway_id", gatewayID))
	return nil
}

// PublishEvent is PublishEventTx in a transaction of its own.
func (b *SQLBackend) PublishEvent(ctx context.Context, gatewayID string, event Event) error {
	return b.gormDB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return b.PublishEventTx(ctx, tx, gatewayID, event)
	})
}

// PublishEventTx writes the event in the caller's transaction, so it is delivered only if that transaction commits.
// It isn't tied to the hub's lifetime: it runs in the caller's transaction, governed by the caller's ctx.
//
// The gateway-state row lock is held until the caller commits, and every other publish to the gateway waits on it:
// publish last in the transaction, and lock several gateways in gateway_id order.
func (b *SQLBackend) PublishEventTx(ctx context.Context, tx *gorm.DB, gatewayID string, event Event) error {
	if err := event.Err(); err != nil {
		return err
	}
	var err error
	if gatewayID, err = normalizeGatewayID(gatewayID); err != nil {
		return err
	}
	eventData := strings.TrimSpace(event.EventData)
	if eventData == "" {
		eventData = EmptyEventData
	}
	eventID := strings.TrimSpace(event.EventID)
	if eventID == "" {
		eventID = uuid.New().String()
	}
	tx = tx.WithContext(ctx)

	now := dbx.Now()

	// Gateways that predate the event hub, or that get events before their first connection (API keys provisioned
	// at agent creation), have no state row yet.
	orgID, err := ensureGatewayState(tx, gatewayID, now)
	if err != nil {
		return fmt.Errorf("failed to ensure gateway registered: %w", err)
	}

	// The sequence number is taken under the row lock that this update holds until the caller commits, so the
	// poller's seq cursor follows commit order whatever the clocks say.
	bump := tx.Model(&eventStateRow{}).Where("gateway_id = ?", gatewayID).Updates(map[string]any{
		"seq": gorm.Expr("seq + 1"), "version_id": uuid.New().String(), "updated_at": now,
	})
	if bump.Error != nil {
		return fmt.Errorf("failed to update gateway version: %w", bump.Error)
	}
	if bump.RowsAffected == 0 {
		return fmt.Errorf("gateway %q is not registered", gatewayID)
	}
	var state eventStateRow
	if err := tx.Select("seq").Where("gateway_id = ?", gatewayID).Take(&state).Error; err != nil {
		return fmt.Errorf("failed to read gateway sequence: %w", err)
	}

	inserted, err := dbx.InsertIfAbsent(tx, &eventRow{
		ID: eventUUID(eventID), OrgID: orgID, GatewayID: gatewayID, Seq: state.Seq,
		EntityType: string(event.EventType), Action: event.Action, EntityID: event.EntityID, EventData: eventData,
		OriginatedAt: event.OriginatedTimestamp.UTC(), ProcessedAt: b.clock().UTC(),
	})
	if err != nil {
		return fmt.Errorf("failed to insert event: %w", err)
	}
	if !inserted {
		b.logger.Info("Duplicate event, skipping publish",
			slog.String("gateway_id", gatewayID),
			slog.String("event_id", eventID))
		return nil
	}

	b.logger.Info("Event published to EventHub",
		slog.String("gateway_id", gatewayID),
		slog.String("event_type", string(event.EventType)),
		slog.String("action", event.Action),
		slog.String("event_id", eventID))
	return nil
}

// Subscribe subscribes to events for a gateway.
func (b *SQLBackend) Subscribe(gatewayID string) (<-chan Event, error) {
	var err error
	if gatewayID, err = normalizeGatewayID(gatewayID); err != nil {
		return nil, err
	}
	ch := make(chan Event, 100)
	if err := b.registry.addSubscriber(gatewayID, ch); err != nil {
		close(ch)
		return nil, fmt.Errorf("failed to subscribe to gateway %s: %w", gatewayID, err)
	}
	b.logger.Info("Subscribed to gateway events", slog.String("gateway_id", gatewayID))
	return ch, nil
}

// Unsubscribe removes a specific subscription for a gateway.
func (b *SQLBackend) Unsubscribe(gatewayID string, subscriber <-chan Event) error {
	var err error
	if gatewayID, err = normalizeGatewayID(gatewayID); err != nil {
		return err
	}
	ch, err := b.registry.removeSubscriber(gatewayID, subscriber)
	if err != nil {
		return fmt.Errorf("failed to unsubscribe from gateway %s: %w", gatewayID, err)
	}
	close(ch)
	b.logger.Info("Unsubscribed from gateway events", slog.String("gateway_id", gatewayID))
	return nil
}

// UnsubscribeAll removes all subscriptions for a gateway.
func (b *SQLBackend) UnsubscribeAll(gatewayID string) error {
	var err error
	if gatewayID, err = normalizeGatewayID(gatewayID); err != nil {
		return err
	}
	subscribers, err := b.registry.removeAllSubscribers(gatewayID)
	if err != nil {
		return fmt.Errorf("failed to unsubscribe all for gateway %s: %w", gatewayID, err)
	}
	for _, ch := range subscribers {
		close(ch)
	}
	b.logger.Info("Unsubscribed all gateway events", slog.String("gateway_id", gatewayID))
	return nil
}

func (b *SQLBackend) pollLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.config.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.pollGateways()
		}
	}
}

func (b *SQLBackend) pollGateways() {
	// Snapshot gateway IDs under the lock; do not retain pointers outside it.
	var gatewayIDs []string
	b.registry.forEach(func(gw *gateway) {
		gatewayIDs = append(gatewayIDs, gw.id)
	})
	if len(gatewayIDs) == 0 {
		return
	}
	gatewayByID := make(map[string]struct{}, len(gatewayIDs))
	for _, id := range gatewayIDs {
		gatewayByID[id] = struct{}{}
	}

	pageSize := b.config.GatewayStatePageSize
	if pageSize <= 0 {
		pageSize = defaultGatewayStatePageSize
	}

	cursor := ""
	for {
		states, nextCursor, err := b.getGatewayStatesPage(cursor, pageSize)
		if err != nil {
			b.logger.Warn("Failed to poll gateway states page",
				slog.String("cursor", cursor),
				slog.Any("error", err))
			return
		}
		if len(states) == 0 {
			return
		}

		for _, state := range states {
			if _, ok := gatewayByID[state.GatewayID]; !ok {
				continue
			}
			b.registry.mu.RLock()
			gw := b.registry.get(state.GatewayID)
			b.registry.mu.RUnlock()
			if gw == nil {
				continue
			}
			if err := b.pollGatewayWithState(gw, state); err != nil {
				b.logger.Warn("Failed to poll gateway",
					slog.String("gateway_id", state.GatewayID),
					slog.Any("error", err))
			}
		}

		if len(states) < pageSize {
			return
		}
		cursor = nextCursor
	}
}

func (b *SQLBackend) getGatewayStatesPage(cursor string, limit int) ([]GatewayState, string, error) {
	q := b.gormDB.WithContext(b.ctx).Model(&eventStateRow{})
	if cursor != "" {
		q = q.Where("gateway_id > ?", cursor)
	}
	var rows []eventStateRow
	if err := q.Order("gateway_id ASC").Limit(limit).Find(&rows).Error; err != nil {
		return nil, "", fmt.Errorf("failed to query gateway states page: %w", err)
	}
	states := make([]GatewayState, 0, len(rows))
	nextCursor := ""
	for _, r := range rows {
		version := ""
		if r.VersionID != nil {
			version = *r.VersionID
		}
		states = append(states, GatewayState{GatewayID: r.GatewayID, VersionID: version, UpdatedAt: r.UpdatedAt})
		nextCursor = r.GatewayID
	}
	return states, nextCursor, nil
}

func subscriberChannelsAvailable(subscribers []chan Event) bool {
	for _, ch := range subscribers {
		if len(ch) == cap(ch) {
			return false
		}
	}
	return true
}

func (b *SQLBackend) pollGatewayWithState(gw *gateway, state GatewayState) error {
	b.registry.mu.RLock()
	knownVersion := gw.knownVersion
	lastSeq := gw.lastSeq
	b.registry.mu.RUnlock()

	if state.VersionID == knownVersion || state.VersionID == "" {
		return nil
	}

	// Choose query based on whether a delivery cursor exists.
	// Cold start (lastSeq == 0): fetch all events for the gateway.
	// Resume: seq > lastSeq. Sequence numbers are taken under the gateway-state
	// row lock, so they follow commit order and a later commit never sorts before
	// the cursor.
	resuming := lastSeq > 0
	q := b.gormDB.WithContext(b.ctx).Where("gateway_id = ?", gw.id)
	if resuming {
		q = q.Where("seq > ?", lastSeq)
	}
	var rows []eventRow
	if err := q.Order("seq").Find(&rows).Error; err != nil {
		return fmt.Errorf("failed to query events: %w", err)
	}
	events := make([]Event, 0, len(rows))
	for _, r := range rows {
		events = append(events, Event{
			GatewayID: r.GatewayID, ProcessedTimestamp: r.ProcessedAt, OriginatedTimestamp: r.OriginatedAt,
			EventType: EventType(r.EntityType), Action: r.Action, EntityID: r.EntityID, EventID: r.ID,
			EventData: r.EventData, Seq: r.Seq,
		})
	}

	// Snapshot subscriber count under the read lock so the deduplication decision
	// and the no-subscriber early-return below are both based on consistent state.
	// Dropping and re-acquiring the lock between the check and deduplicateLatestPerEntity
	// would allow a concurrent Subscribe (write lock) to race in and add a subscriber,
	// causing a newly-connected gateway to receive deduplicated catch-up events instead
	// of the full live stream.
	b.registry.mu.RLock()
	subscriberCountAtCheck := len(gw.subscribers)
	b.registry.mu.RUnlock()

	// On cold start with no active subscribers at the time of the check, filter to
	// current desired state: skip API key events (gateway bulk-syncs those on reconnect)
	// and keep only the latest event per entity_id for all other types. When subscribers
	// ARE present we deliver all events verbatim — deduplication must not drop live
	// apikey events that arrived while the gateway was connected.
	//
	// Also track the last raw event before dedup so we can advance the cursor even
	// when all events were skipped (e.g. only API key events). Without this,
	// lastSeq stays zero and every future poll also takes the cold-start path,
	// silencing all subsequent API key events.
	var coldStartCursorEvent *Event
	if !resuming && subscriberCountAtCheck == 0 {
		if len(events) > 0 {
			coldStartCursorEvent = &events[len(events)-1]
		}
		events = deduplicateLatestPerEntity(events)
	}

	// Re-acquire the read lock and hold it across all channel sends so that
	// Unsubscribe/UnsubscribeAll (which need the write lock) cannot close a
	// channel while a send is in flight.
	b.registry.mu.RLock()

	if len(gw.subscribers) == 0 {
		shouldLog := len(events) > 0 && gw.queuedLoggedAt == 0
		if shouldLog {
			gw.queuedLoggedAt = time.Now().UnixNano()
		}
		b.registry.mu.RUnlock()
		if shouldLog {
			b.logger.Info("Gateway disconnected with pending events, waiting for reconnect",
				slog.String("gateway_id", gw.id),
				slog.Int("queued_event_count", len(events)))
		}
		return nil
	}
	// Subscriber present — clear the queued flag so it fires again after the next disconnect.
	gw.queuedLoggedAt = 0

	subscriberCount := len(gw.subscribers)
	var lastDelivered *Event
	deliveredCount := 0
	deliveryBlocked := false
	for i := range events {
		if !subscriberChannelsAvailable(gw.subscribers) {
			deliveryBlocked = true
			b.logger.Warn("Subscriber channel full, deferring event delivery",
				slog.String("gateway_id", gw.id),
				slog.String("entity_id", events[i].EntityID))
			break
		}
		for _, ch := range gw.subscribers {
			ch <- events[i]
		}
		lastDelivered = &events[i]
		deliveredCount++
	}

	b.registry.mu.RUnlock()

	b.registry.mu.Lock()
	if deliveryBlocked {
		if lastDelivered != nil {
			gw.lastSeq = lastDelivered.Seq
		}
		// else: nothing delivered this cycle; cursor unchanged.
	} else {
		gw.knownVersion = state.VersionID
		if lastDelivered != nil {
			gw.lastSeq = lastDelivered.Seq
		} else if coldStartCursorEvent != nil {
			// Cold start where all events were skipped by dedup (e.g. only API
			// key events): advance the cursor past those events so future polls
			// use the resuming path and deliver new API key events individually.
			gw.lastSeq = coldStartCursorEvent.Seq
		}
	}
	b.registry.mu.Unlock()

	if deliveredCount > 0 {
		catchUp := knownVersion == ""
		b.logger.Info("Delivered events to gateway subscribers",
			slog.String("gateway_id", gw.id),
			slog.Int("event_count", deliveredCount),
			slog.Int("subscriber_count", subscriberCount),
			slog.Bool("catch_up", catchUp))
	}

	return nil
}

// apiKeySyncEventTypes lists event types the gateway reconciles via bulk-sync on
// reconnect. These are skipped during catch-up replay to avoid replaying stale
// individual key operations that are already reflected in the bulk-sync response.
var apiKeySyncEventTypes = map[string]bool{
	"apikey.created": true,
	"apikey.revoked": true,
	"apikey.updated": true,
}

// deduplicateLatestPerEntity filters a catch-up event list to current desired
// state: skips API key events (covered by bulk-sync) and keeps only the most
// recent event per entity_id for all other event types. Input must be ordered
// by seq; the last entry per entity_id wins.
func deduplicateLatestPerEntity(events []Event) []Event {
	latest := make(map[string]int, len(events))
	for i, evt := range events {
		if apiKeySyncEventTypes[string(evt.EventType)] {
			continue
		}
		latest[evt.EntityID] = i
	}
	if len(latest) == 0 {
		return nil
	}
	result := make([]Event, 0, len(latest))
	for i, evt := range events {
		if apiKeySyncEventTypes[string(evt.EventType)] {
			continue
		}
		if latest[evt.EntityID] == i {
			result = append(result, evt)
		}
	}
	return result
}

func (b *SQLBackend) cleanupLoop() {
	defer b.wg.Done()
	ticker := time.NewTicker(b.config.CleanupInterval)
	defer ticker.Stop()
	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			if err := b.CleanUpEvents(); err != nil {
				b.logger.Warn("Failed to clean up events", slog.Any("error", err))
			}
		}
	}
}

// CleanUpEvents removes events older than the retention period.
func (b *SQLBackend) CleanUpEvents() error {
	cutoff := time.Now().Add(-b.config.RetentionPeriod)
	result := b.gormDB.WithContext(b.ctx).Where("processed_at < ?", cutoff.UTC()).Delete(&eventRow{})
	if result.Error != nil {
		return fmt.Errorf("failed to clean up events: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		b.logger.Info("Cleaned up old events", slog.Int64("deleted_count", result.RowsAffected))
	}
	return nil
}

// Close gracefully shuts down the backend.
func (b *SQLBackend) Close() error {
	b.cancel()
	b.wg.Wait()

	b.registry.mu.Lock()
	for _, gw := range b.registry.gateways {
		for _, ch := range gw.subscribers {
			close(ch)
		}
		gw.subscribers = nil
	}
	b.registry.mu.Unlock()

	b.logger.Info("EventHub closed")
	return nil
}
