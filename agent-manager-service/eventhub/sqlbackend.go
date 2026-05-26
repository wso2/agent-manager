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
	db     *sql.DB
	logger *slog.Logger
	config SQLBackendConfig

	registry *gatewayRegistry

	stmtMu                   sync.RWMutex
	insertEventStmt          *sql.Stmt
	updateGatewayVersionStmt *sql.Stmt
	upsertGatewayStmt        *sql.Stmt
	getGatewayStateStmt      *sql.Stmt
	getGatewayStatesPageStmt *sql.Stmt
	getEventsStmt            *sql.Stmt // cold start: all events for a gateway
	getEventsAfterCursorStmt *sql.Stmt // resume: (processed_timestamp, event_id) > (?, ?)
	getEventByIDStmt         *sql.Stmt
	cleanupEventsStmt        *sql.Stmt

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

var _ EventHub = (*SQLBackend)(nil)

// NewSQLBackend creates a new SQL-backed EventHub for PostgreSQL.
func NewSQLBackend(db *sql.DB, logger *slog.Logger, config SQLBackendConfig) *SQLBackend {
	ctx, cancel := context.WithCancel(context.Background())
	return &SQLBackend{
		db:       db,
		logger:   logger,
		config:   config,
		registry: newGatewayRegistry(),
		ctx:      ctx,
		cancel:   cancel,
	}
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
		b.insertEventStmt,
		b.updateGatewayVersionStmt,
		b.upsertGatewayStmt,
		b.getGatewayStateStmt,
		b.getGatewayStatesPageStmt,
		b.getEventsStmt,
		b.getEventsAfterCursorStmt,
		b.getEventByIDStmt,
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

	b.insertEventStmt, err = b.db.Prepare(rebind(`
		INSERT INTO eventhub_events (gateway_id, processed_timestamp, originated_timestamp, entity_type, action, entity_id, event_id, event_data)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`))
	if err != nil {
		return fmt.Errorf("prepare insert event: %w", err)
	}

	b.updateGatewayVersionStmt, err = b.db.Prepare(rebind(`
		UPDATE eventhub_gateway_states SET version_id = ?, updated_at = CURRENT_TIMESTAMP WHERE gateway_id = ?`))
	if err != nil {
		return fmt.Errorf("prepare update gateway version: %w", err)
	}

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
		SELECT gateway_id, processed_timestamp, originated_timestamp, entity_type, action, entity_id, event_id, event_data
		FROM eventhub_events
		WHERE gateway_id = ?
		ORDER BY processed_timestamp ASC, event_id ASC`))
	if err != nil {
		return fmt.Errorf("prepare get events: %w", err)
	}

	b.getEventsAfterCursorStmt, err = b.db.Prepare(rebind(`
		SELECT gateway_id, processed_timestamp, originated_timestamp, entity_type, action, entity_id, event_id, event_data
		FROM eventhub_events
		WHERE gateway_id = ? AND (processed_timestamp, event_id) > (?, ?)
		ORDER BY processed_timestamp ASC, event_id ASC`))
	if err != nil {
		return fmt.Errorf("prepare get events after cursor: %w", err)
	}

	b.getEventByIDStmt, err = b.db.Prepare(rebind(`
		SELECT event_id FROM eventhub_events WHERE event_id = ?`))
	if err != nil {
		return fmt.Errorf("prepare get event by ID: %w", err)
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

// PublishEvent publishes an event atomically (insert event + bump gateway version).
func (b *SQLBackend) PublishEvent(gatewayID string, event Event) error {
	var err error
	if gatewayID, err = normalizeGatewayID(gatewayID); err != nil {
		return err
	}
	newVersion := uuid.New().String()
	eventData := strings.TrimSpace(event.EventData)
	if eventData == "" {
		eventData = EmptyEventData
	}
	eventID := strings.TrimSpace(event.EventID)
	if eventID == "" {
		event.EventID = uuid.New().String()
		eventID = event.EventID
	}

	tx, err := b.db.BeginTx(b.ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Ensure the gateway row exists. This handles gateways that existed before
	// the EventHub was introduced and gateways that publish events before their
	// first WebSocket connection (e.g. API key provisioning at agent creation).
	if _, err = tx.Stmt(b.upsertGatewayStmt).Exec(gatewayID); err != nil {
		return fmt.Errorf("failed to ensure gateway registered: %w", err)
	}

	_, err = tx.Stmt(b.insertEventStmt).Exec(
		gatewayID,
		time.Now(),
		event.OriginatedTimestamp,
		string(event.EventType),
		event.Action,
		event.EntityID,
		eventID,
		eventData,
	)
	if err != nil {
		insertErr := err
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			return fmt.Errorf("failed to rollback after insert failure: %w", rollbackErr)
		}
		err = nil

		exists, checkErr := b.eventExists(eventID)
		if checkErr != nil {
			return fmt.Errorf("failed to check event existence after insert failure: %w", checkErr)
		}
		if exists {
			b.logger.Info("Duplicate event, skipping publish",
				slog.String("gateway_id", gatewayID),
				slog.String("event_id", eventID))
			return nil
		}
		return fmt.Errorf("failed to insert event: %w", insertErr)
	}

	result, err := tx.Stmt(b.updateGatewayVersionStmt).Exec(newVersion, gatewayID)
	if err != nil {
		return fmt.Errorf("failed to update gateway version: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		err = fmt.Errorf("gateway %q is not registered", gatewayID)
		return err
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit event publish: %w", err)
	}

	b.logger.Info("Event published to EventHub",
		slog.String("gateway_id", gatewayID),
		slog.String("event_type", string(event.EventType)),
		slog.String("action", event.Action),
		slog.String("event_id", eventID))

	return nil
}

func (b *SQLBackend) eventExists(eventID string) (bool, error) {
	var id string
	err := b.getEventByIDStmt.QueryRow(eventID).Scan(&id)
	if err == nil {
		return true, nil
	}
	if err == sql.ErrNoRows {
		return false, nil
	}
	return false, err
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
	lastPolledTime := gw.lastPolledTime
	lastPolledEventID := gw.lastPolledEventID
	b.registry.mu.RUnlock()

	if state.VersionID == knownVersion || state.VersionID == "" {
		return nil
	}

	// Choose query based on whether a delivery cursor exists.
	// Cold start (lastPolledTime.IsZero()): fetch all events for the gateway.
	// Resume: use composite (processed_timestamp, event_id) > (?, ?) so events
	// sharing a timestamp do not replay — event_id is the stable tie-breaker.
	var rows *sql.Rows
	var err error
	resuming := !lastPolledTime.IsZero()
	if resuming {
		rows, err = b.getEventsAfterCursorStmt.Query(gw.id, lastPolledTime, lastPolledEventID)
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
		); err != nil {
			return fmt.Errorf("failed to scan event row: %w", err)
		}
		evt.EventType = EventType(eventType)
		events = append(events, evt)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating event rows: %w", err)
	}

	// On cold start, filter to current desired state: skip API key events
	// (gateway bulk-syncs those on reconnect) and keep only the latest event
	// per entity_id for all other types.
	if !resuming {
		events = deduplicateLatestPerEntity(events)
	}

	// Hold the read lock across all channel sends so that Unsubscribe/UnsubscribeAll
	// (which need the write lock) cannot close a channel while a send is in flight.
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
			gw.lastPolledTime = lastDelivered.ProcessedTimestamp
			gw.lastPolledEventID = lastDelivered.EventID
		}
		// else: nothing delivered this cycle; cursor unchanged.
	} else {
		gw.knownVersion = state.VersionID
		if lastDelivered != nil {
			gw.lastPolledTime = lastDelivered.ProcessedTimestamp
			gw.lastPolledEventID = lastDelivered.EventID
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
// by processed_timestamp ASC; the last entry per entity_id wins.
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
