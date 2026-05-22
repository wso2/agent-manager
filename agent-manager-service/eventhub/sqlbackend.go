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
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	initialPollSkewWindow       = 120 * time.Second
	defaultGatewayStatePageSize = 200

	unixMillisThreshold = int64(100_000_000_000)
	unixMicrosThreshold = int64(100_000_000_000_000)
	unixNanosThreshold  = int64(100_000_000_000_000_000)
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
	getEventsStmt            *sql.Stmt
	getEventByIDStmt         *sql.Stmt
	cleanupEventsStmt        *sql.Stmt

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

var _ EventHub = (*SQLBackend)(nil)

func unixTimestampToTime(ts int64) time.Time {
	switch {
	case ts >= unixNanosThreshold:
		return time.Unix(0, ts)
	case ts >= unixMicrosThreshold:
		return time.UnixMicro(ts)
	case ts >= unixMillisThreshold:
		return time.UnixMilli(ts)
	default:
		return time.Unix(ts, 0)
	}
}

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
		b.getEventByIDStmt,
		b.cleanupEventsStmt,
	} {
		if stmt != nil {
			stmt.Close()
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
		WHERE gateway_id = ? AND processed_timestamp >= ?
		ORDER BY processed_timestamp ASC`))
	if err != nil {
		return fmt.Errorf("prepare get events: %w", err)
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

	b.wg.Add(2)
	go b.pollLoop()
	go b.cleanupLoop()

	b.logger.Info("EventHub initialized",
		slog.Duration("poll_interval", b.config.PollInterval),
		slog.Duration("cleanup_interval", b.config.CleanupInterval),
		slog.Duration("retention_period", b.config.RetentionPeriod))

	return nil
}

// RegisterGateway registers a new gateway for event tracking.
func (b *SQLBackend) RegisterGateway(gatewayID string) error {
	gatewayID = strings.TrimSpace(gatewayID)
	if gatewayID == "" {
		return fmt.Errorf("gateway_id cannot be empty")
	}

	if _, err := b.upsertGatewayStmt.Exec(gatewayID); err != nil {
		return fmt.Errorf("failed to register gateway: %w", err)
	}

	if regErr := b.registry.register(gatewayID); regErr != nil && regErr != ErrGatewayAlreadyExists {
		return fmt.Errorf("failed to register gateway in registry: %w", regErr)
	}

	b.logger.Info("Gateway registered for event tracking", slog.String("gateway_id", gatewayID))
	return nil
}

// PublishEvent publishes an event atomically (insert event + bump gateway version).
func (b *SQLBackend) PublishEvent(gatewayID string, event Event) error {
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
			tx.Rollback()
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
		if rollbackErr := tx.Rollback(); rollbackErr != nil && rollbackErr != sql.ErrTxDone {
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

	b.logger.Debug("Event published",
		slog.String("gateway_id", gatewayID),
		slog.String("event_type", string(event.EventType)),
		slog.String("action", event.Action),
		slog.String("entity_id", event.EntityID))

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
	gatewayByID := make(map[string]*gateway)
	for _, gw := range b.registry.getAll() {
		gatewayByID[gw.id] = gw
	}
	if len(gatewayByID) == 0 {
		return
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
			gw, ok := gatewayByID[state.GatewayID]
			if !ok {
				continue
			}
			if err := b.pollGatewayWithState(gw, state); err != nil {
				b.logger.Warn("Failed to poll gateway",
					slog.String("gateway_id", gw.id),
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
	defer rows.Close()

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
	lastPolled := gw.lastPolled
	b.registry.mu.RUnlock()

	if state.VersionID == knownVersion || state.VersionID == "" {
		return nil
	}

	var lastPolledTime time.Time
	resumingFromLastPolled := lastPolled > 0
	if lastPolled > 0 {
		lastPolledTime = unixTimestampToTime(lastPolled)
	} else {
		lastPolledTime = time.Now().Add(-initialPollSkewWindow)
	}

	rows, err := b.getEventsStmt.Query(gw.id, lastPolledTime)
	if err != nil {
		return fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

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

	events = trimSingleBoundaryReplay(events, lastPolledTime, resumingFromLastPolled)

	// Snapshot subscribers under the lock so we don't hold it during channel sends.
	b.registry.mu.RLock()
	subscribers := make([]chan Event, len(gw.subscribers))
	copy(subscribers, gw.subscribers)
	b.registry.mu.RUnlock()

	var latestDeliveredTimestamp time.Time
	deliveredCount := 0
	deliveryBlocked := false
	for _, evt := range events {
		if !subscriberChannelsAvailable(subscribers) {
			deliveryBlocked = true
			b.logger.Warn("Subscriber channel full, deferring event delivery",
				slog.String("gateway_id", gw.id),
				slog.String("entity_id", evt.EntityID))
			break
		}
		for _, ch := range subscribers {
			ch <- evt
		}
		latestDeliveredTimestamp = evt.ProcessedTimestamp
		deliveredCount++
	}

	b.registry.mu.Lock()
	if deliveryBlocked {
		if !latestDeliveredTimestamp.IsZero() {
			gw.lastPolled = latestDeliveredTimestamp.UnixNano()
		} else {
			gw.lastPolled = lastPolledTime.UnixNano()
		}
	} else {
		gw.knownVersion = state.VersionID
		if !latestDeliveredTimestamp.IsZero() {
			gw.lastPolled = latestDeliveredTimestamp.UnixNano()
		}
	}
	b.registry.mu.Unlock()

	if deliveredCount > 0 {
		b.logger.Debug("Delivered events to gateway subscribers",
			slog.String("gateway_id", gw.id),
			slog.Int("event_count", deliveredCount),
			slog.Int("subscriber_count", len(subscribers)))
	}

	return nil
}

// trimSingleBoundaryReplay drops the single boundary event that was already
// delivered on the previous poll (the >= query re-fetches it).
func trimSingleBoundaryReplay(events []Event, boundary time.Time, enabled bool) []Event {
	if !enabled || len(events) == 0 || !events[0].ProcessedTimestamp.Equal(boundary) {
		return events
	}
	if len(events) == 1 || !events[1].ProcessedTimestamp.Equal(boundary) {
		return events[1:]
	}
	return events
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

	for _, gw := range b.registry.getAll() {
		b.registry.mu.Lock()
		for _, ch := range gw.subscribers {
			close(ch)
		}
		gw.subscribers = nil
		b.registry.mu.Unlock()
	}

	b.stmtMu.Lock()
	defer b.stmtMu.Unlock()
	b.closeStatements()

	b.logger.Info("EventHub closed")
	return nil
}
