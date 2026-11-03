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
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
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

// SQLBackend implements EventHub using SQL polling over PostgreSQL.
type SQLBackend struct {
	gormDB *gorm.DB
	db     *sql.DB
	logger *slog.Logger
	config SQLBackendConfig
	clock  func() time.Time

	registry *gatewayRegistry

	stmtMu                   sync.RWMutex
	upsertGatewayStmt        *sql.Stmt
	getGatewayStateStmt      *sql.Stmt
	getGatewayStatesPageStmt *sql.Stmt
	getEventsStmt            *sql.Stmt // cold start: all events for a gateway
	getEventsAfterCursorStmt *sql.Stmt // resume: seq > ?
	cleanupEventsStmt        *sql.Stmt

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

var _ EventHub = (*SQLBackend)(nil)

// NewSQLBackend creates a new SQL-backed EventHub for PostgreSQL. Publishes run on db, so a caller's GORM
// transaction can carry them.
func NewSQLBackend(db *gorm.DB, logger *slog.Logger, config SQLBackendConfig) (*SQLBackend, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get the event hub's sql.DB: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &SQLBackend{
		gormDB:   db,
		db:       sqlDB,
		logger:   logger,
		config:   config,
		clock:    time.Now,
		registry: newGatewayRegistry(),
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

// rebind replaces ? placeholders with $1, $2, ... for PostgreSQL.
func rebind(query string) string {
	var buf strings.Builder
	n := 1
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			fmt.Fprintf(&buf, "$%d", n)
			n++
		} else {
			buf.WriteByte(query[i])
		}
	}
	return buf.String()
}

func (b *SQLBackend) closeStatements() {
	for _, stmt := range []*sql.Stmt{
		b.upsertGatewayStmt,
		b.getGatewayStateStmt,
		b.getGatewayStatesPageStmt,
		b.getEventsStmt,
		b.getEventsAfterCursorStmt,
		b.cleanupEventsStmt,
	} {
		if stmt != nil {
			_ = stmt.Close()
		}
	}
}

func (b *SQLBackend) prepareStatements() (err error) {
	defer func() {
		if err != nil {
			b.closeStatements()
		}
	}()

	// upsertGatewayStmt ensures the gateway row exists before inserting an event.
	// ON CONFLICT DO NOTHING is safe: if the row already exists we leave version_id
	// untouched; the subsequent UPDATE will bump it.
	b.upsertGatewayStmt, err = b.db.Prepare(rebind(`
		INSERT INTO eventhub_gateway_states (gateway_id, version_id) VALUES (?, '')
		ON CONFLICT (gateway_id) DO NOTHING`))
	if err != nil {
		return fmt.Errorf("prepare upsert gateway: %w", err)
	}

	b.getGatewayStateStmt, err = b.db.Prepare(rebind(`
		SELECT gateway_id, version_id, updated_at FROM eventhub_gateway_states WHERE gateway_id = ?`))
	if err != nil {
		return fmt.Errorf("prepare get gateway state: %w", err)
	}

	b.getGatewayStatesPageStmt, err = b.db.Prepare(rebind(`
		SELECT gateway_id, version_id, updated_at
		FROM eventhub_gateway_states
		WHERE gateway_id > ?
		ORDER BY gateway_id ASC
		LIMIT ?`))
	if err != nil {
		return fmt.Errorf("prepare get gateway states page: %w", err)
	}

	b.getEventsStmt, err = b.db.Prepare(rebind(`
		SELECT gateway_id, processed_timestamp, originated_timestamp, entity_type, action, entity_id, event_id, event_data, seq
		FROM eventhub_events
		WHERE gateway_id = ?
		ORDER BY seq`))
	if err != nil {
		return fmt.Errorf("prepare get events: %w", err)
	}

	b.getEventsAfterCursorStmt, err = b.db.Prepare(rebind(`
		SELECT gateway_id, processed_timestamp, originated_timestamp, entity_type, action, entity_id, event_id, event_data, seq
		FROM eventhub_events
		WHERE gateway_id = ? AND seq > ?
		ORDER BY seq`))
	if err != nil {
		return fmt.Errorf("prepare get events after cursor: %w", err)
	}

	b.cleanupEventsStmt, err = b.db.Prepare(rebind(`
		DELETE FROM eventhub_events WHERE processed_timestamp < ?`))
	if err != nil {
		return fmt.Errorf("prepare cleanup events: %w", err)
	}

	return nil
}

// Initialize prepares statements and starts background goroutines.
func (b *SQLBackend) Initialize() error {
	if err := b.prepareStatements(); err != nil {
		return fmt.Errorf("failed to prepare statements: %w", err)
	}

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

	if _, err := b.upsertGatewayStmt.Exec(gatewayID); err != nil {
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

	// Gateways that predate the event hub, or that get events before their first connection (API keys provisioned
	// at agent creation), have no state row yet.
	if err := tx.Exec(`INSERT INTO eventhub_gateway_states (gateway_id, version_id) VALUES (?, '')
		ON CONFLICT (gateway_id) DO NOTHING`, gatewayID).Error; err != nil {
		return fmt.Errorf("failed to ensure gateway registered: %w", err)
	}

	// The sequence number is taken under the row lock that orders this gateway's commits, so the poller's seq
	// cursor follows commit order whatever the clocks say.
	var seq int64
	bump := tx.Raw(`UPDATE eventhub_gateway_states SET seq = seq + 1, version_id = ?, updated_at = CURRENT_TIMESTAMP
		WHERE gateway_id = ? RETURNING seq`, uuid.New().String(), gatewayID).Scan(&seq)
	if bump.Error != nil {
		return fmt.Errorf("failed to update gateway version: %w", bump.Error)
	}
	if bump.RowsAffected == 0 {
		return fmt.Errorf("gateway %q is not registered", gatewayID)
	}

	insert := tx.Exec(`INSERT INTO eventhub_events
		(gateway_id, processed_timestamp, originated_timestamp, entity_type, action, entity_id, event_id, event_data, seq)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (event_id) DO NOTHING`,
		gatewayID, b.clock(), event.OriginatedTimestamp, string(event.EventType), event.Action, event.EntityID,
		eventID, eventData, seq)
	if insert.Error != nil {
		return fmt.Errorf("failed to insert event: %w", insert.Error)
	}
	if insert.RowsAffected == 0 {
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
	rows, err := b.getGatewayStatesPageStmt.Query(cursor, limit)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query gateway states page: %w", err)
	}
	defer func() { _ = rows.Close() }()

	states := make([]GatewayState, 0, limit)
	nextCursor := ""
	for rows.Next() {
		var state GatewayState
		if err := rows.Scan(&state.GatewayID, &state.VersionID, &state.UpdatedAt); err != nil {
			return nil, "", fmt.Errorf("failed to scan gateway state row: %w", err)
		}
		states = append(states, state)
		nextCursor = state.GatewayID
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("error iterating gateway state rows: %w", err)
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
	var rows *sql.Rows
	var err error
	resuming := lastSeq > 0
	if resuming {
		rows, err = b.getEventsAfterCursorStmt.Query(gw.id, lastSeq)
	} else {
		rows, err = b.getEventsStmt.Query(gw.id)
	}
	if err != nil {
		return fmt.Errorf("failed to query events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var events []Event
	for rows.Next() {
		var evt Event
		var eventType string
		if err := rows.Scan(
			&evt.GatewayID,
			&evt.ProcessedTimestamp,
			&evt.OriginatedTimestamp,
			&eventType,
			&evt.Action,
			&evt.EntityID,
			&evt.EventID,
			&evt.EventData,
			&evt.Seq,
		); err != nil {
			return fmt.Errorf("failed to scan event row: %w", err)
		}
		evt.EventType = EventType(eventType)
		events = append(events, evt)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating event rows: %w", err)
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
	result, err := b.cleanupEventsStmt.Exec(cutoff)
	if err != nil {
		return fmt.Errorf("failed to clean up events: %w", err)
	}
	if affected, _ := result.RowsAffected(); affected > 0 {
		b.logger.Info("Cleaned up old events", slog.Int64("deleted_count", affected))
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

	b.stmtMu.Lock()
	defer b.stmtMu.Unlock()
	b.closeStatements()

	b.logger.Info("EventHub closed")
	return nil
}
