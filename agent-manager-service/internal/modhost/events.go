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

package modhost

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// Module events live in module_events, with one module_event_deliveries row
// per subscribing module, written in the transaction that raises the event.
// A drainer on every replica claims the deliveries of the modules it runs,
// calls their handlers off any request with a deadline, and deletes a
// delivery once its module has handled it. A failed delivery is retried with
// backoff, so each subscriber gets each event at least once. A handler that
// overruns the deadline is abandoned, not stopped: it may still be running
// when the retry starts, here or on another replica, so handlers must
// tolerate running concurrently and more than once. Deliveries expire after
// EventRetention, handled or not, and an uninstalled module's are dropped.
//
// Times are each replica's clock (dbx.Now), as everywhere else in the
// schema; replicas are expected to keep their clocks in sync.

var coreEvents = []string{modkit.EventAgentCreated, modkit.EventAgentDeleted, modkit.EventEnvironmentCreated}

const (
	// DefaultEventDeliveryTimeout bounds one delivery to one module, all its
	// handlers for the event together.
	DefaultEventDeliveryTimeout = 30 * time.Second
	// eventClaimLease is how long a claimed delivery stays with its replica,
	// four times the delivery deadline, so a delivery is not handed to a
	// second replica while the first is still within its deadline.
	eventClaimLease = 2 * time.Minute
	// EventRetention is how long a delivery is kept, pending or failed.
	EventRetention = 7 * 24 * time.Hour
	// eventSweepInterval is how often a replica drops expired deliveries.
	eventSweepInterval = time.Hour
	// eventMaxAttempts is how often a delivery is tried before it is kept as failed.
	eventMaxAttempts = 10
	// eventWorkers is how many deliveries a replica runs at once.
	eventWorkers = 8
	// DefaultEventRetryBase is the first retry delay; it doubles each attempt.
	DefaultEventRetryBase = 2 * time.Second
	maxEventRetry         = 5 * time.Minute
)

// knownEvent reports whether the core or a compiled-in module's manifest
// declares an event type.
func (h *Host) knownEvent(t string) bool {
	if slices.Contains(coreEvents, t) {
		return true
	}
	for _, mf := range h.manifests {
		if _, ok := mf.Event(t); ok {
			return true
		}
	}
	return false
}

// subscribers lists the modules of v that handle an event type, sorted.
func (v *view) subscribers(eventType string) []string {
	var out []string
	for _, s := range v.subs[eventType] {
		if !slices.Contains(out, s.module) {
			out = append(out, s.module)
		}
	}
	sort.Strings(out)
	return out
}

// record writes an event and a delivery for each module, through db.
func record(db *gorm.DB, e modkit.Event, modules []string) error {
	if len(modules) == 0 {
		return nil
	}
	if e.ID == "" {
		e.ID = uuid.Must(uuid.NewV7()).String()
	}
	org, err := uuid.Parse(e.OrgID)
	if err != nil {
		return fmt.Errorf("modkit: event %s names no valid organization", e.Type)
	}
	data := e.Data
	if len(data) == 0 {
		data = json.RawMessage(`null`)
	}
	var subject any
	if e.Subject != "" {
		subject = e.Subject
	}
	now := dbx.Now()
	// One transaction (a savepoint inside the caller's): the sweep drops
	// events without deliveries, so an event committed on its own can vanish
	// before its deliveries are written.
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(`INSERT INTO module_events (id, org_id, type, source, subject, data, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			e.ID, org, e.Type, e.Source, subject, string(data), now).Error; err != nil {
			return err
		}
		for _, m := range modules {
			if err := tx.Exec(`INSERT INTO module_event_deliveries (event_id, module, attempts, available_at) VALUES (?, ?, 0, ?)`,
				e.ID, m, now).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// Emit records a core event for the modules subscribed to it in the
// operation's snapshot, inside tx.
func (op *operation) Emit(ctx context.Context, tx *gorm.DB, e modkit.Event) error {
	e.Source = "core"
	db := tx
	if db == nil {
		db = op.h.db
	}
	return record(db.WithContext(ctx), e, op.v.subscribers(e.Type))
}

func (h *Host) publish(ctx context.Context, tx *gorm.DB, module, eventType string, data any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("modkit: event %s: %w", eventType, err)
	}
	org := modkit.OrgID(ctx)
	if org == "" {
		var probe struct {
			OrgID string `json:"orgId"`
		}
		_ = json.Unmarshal(raw, &probe)
		org = probe.OrgID
	}
	if org == "" {
		return fmt.Errorf("modkit: event %s names no organization", eventType)
	}
	if tx == nil {
		hook, inHook := hookTx(ctx)
		if inHook && hook == nil {
			return errReadOnlyHook
		}
		tx = hook
	}
	if tx == nil {
		tx = h.db
	}
	e := modkit.Event{Type: eventType, Source: module, OrgID: org, Data: raw}
	mt, _ := h.manifests[module].Event(eventType)
	if mt.Scope == "" {
		return record(tx.WithContext(ctx), e, h.current().subscribers(eventType))
	}
	// A scoped event is also a webhook event: it goes to the core's outbox in
	// the same transaction, so subscriptions get it only if the caller commits.
	return tx.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := record(tx, e, h.current().subscribers(eventType)); err != nil {
			return err
		}
		return repositories.NewEventOutboxRepository(tx).Put(ctx, webhookEvent(e, events.Scope(mt.Scope)))
	})
}

// webhookEvent is the webhook form of a scoped module event. Its project,
// agent, environment and subject come from the data's "project", "agent",
// "environment" and "subject" fields, and its organization handle from
// "orgName".
func webhookEvent(e modkit.Event, scope events.Scope) events.Event {
	var about struct {
		OrgName     string `json:"orgName"`
		Project     string `json:"project"`
		Agent       string `json:"agent"`
		Environment string `json:"environment"`
		Subject     string `json:"subject"`
	}
	_ = json.Unmarshal(e.Data, &about)
	out := events.New("evt_"+uuid.NewString(), e.Type, scope, e.OrgID, about.Project, about.Agent, dbx.Now())
	out.OrgHandle = about.OrgName
	out.Environment = about.Environment
	out.Subject = about.Subject
	if len(e.Data) > 0 {
		var data map[string]any
		if json.Unmarshal(e.Data, &data) == nil {
			out.Data = data
		}
	}
	return out
}

type deliveryRow struct {
	EventID  string
	Module   string
	Attempts int
}

type eventRow struct {
	ID        string
	OrgID     string
	Type      string
	Source    string
	Subject   *string
	Data      string
	CreatedAt time.Time
}

// drain delivers module events until ctx ends. Each claimed delivery runs on
// its own worker, so a slow module holds up only its own deliveries.
func (h *Host) drain(ctx context.Context) {
	workers := make(chan struct{}, eventWorkers)
	var wg sync.WaitGroup
	defer wg.Wait()
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	sweep := time.NewTicker(eventSweepInterval)
	defer sweep.Stop()
	h.ExpireEvents(ctx, "")
	for {
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			h.ExpireEvents(ctx, "")
		case <-tick.C:
		}
		for free := eventWorkers - len(workers); free > 0 && ctx.Err() == nil; free = eventWorkers - len(workers) {
			v, lease, claimed := h.claim(ctx, free)
			for _, d := range claimed {
				workers <- struct{}{}
				wg.Go(func() {
					defer func() { <-workers }()
					h.deliver(ctx, v, d, lease)
				})
			}
			if len(claimed) < free {
				break
			}
		}
	}
}

// claim claims up to n due deliveries for the modules this replica runs.
func (h *Host) claim(ctx context.Context, n int) (*view, string, []deliveryRow) {
	v := h.current()
	var modules []string
	for _, subs := range v.subs {
		for _, s := range subs {
			if !slices.Contains(modules, s.module) {
				modules = append(modules, s.module)
			}
		}
	}
	if len(modules) == 0 {
		return v, "", nil
	}
	db := h.db.WithContext(ctx)
	now := dbx.Now()
	var due []deliveryRow
	if err := db.Raw(`SELECT event_id, module, attempts FROM module_event_deliveries
		WHERE available_at <= ? AND failed_at IS NULL AND module IN ? ORDER BY available_at LIMIT ?`,
		now, modules, n).Scan(&due).Error; err != nil {
		if ctx.Err() == nil {
			h.log.Error("modhost:reading module event deliveries failed", "error", err)
		}
		return v, "", nil
	}
	lease := uuid.NewString()
	var claimed []deliveryRow
	for _, d := range due {
		r := db.Exec(`UPDATE module_event_deliveries SET claimed_by = ?, available_at = ?
			WHERE event_id = ? AND module = ? AND available_at <= ? AND failed_at IS NULL`,
			lease, now.Add(eventClaimLease), d.EventID, d.Module, now)
		if r.Error == nil && r.RowsAffected == 1 {
			claimed = append(claimed, d)
		}
	}
	return v, lease, claimed
}

func (h *Host) deliveryTimeout() time.Duration {
	if h.EventDeliveryTimeout > 0 {
		return min(h.EventDeliveryTimeout, eventClaimLease/4)
	}
	return DefaultEventDeliveryTimeout
}

// deliver runs one module's handlers for one event, under one deadline, and
// records the outcome.
func (h *Host) deliver(ctx context.Context, v *view, d deliveryRow, lease string) {
	db := h.db.WithContext(context.WithoutCancel(ctx))
	var rows []eventRow
	if err := db.Raw(`SELECT id, org_id, type, source, subject, data, created_at FROM module_events WHERE id = ?`, d.EventID).
		Scan(&rows).Error; err != nil || len(rows) == 0 {
		h.finish(db, d, lease, err)
		return
	}
	er := rows[0]
	e := modkit.Event{ID: er.ID, Type: er.Type, Source: er.Source, OrgID: er.OrgID, Time: er.CreatedAt, Data: json.RawMessage(er.Data)}
	if er.Subject != nil {
		e.Subject = *er.Subject
	}
	var handlers []modkit.EventHandler
	for _, s := range v.subs[e.Type] {
		if s.module == d.Module {
			handlers = append(handlers, s.fn)
		}
	}
	err := h.runHandlers(ctx, handlers, e)
	if ctx.Err() != nil {
		// Shutting down: give the delivery back without counting an attempt.
		_ = db.Exec(`UPDATE module_event_deliveries SET available_at = ?, claimed_by = NULL
			WHERE event_id = ? AND module = ? AND claimed_by = ?`, dbx.Now(), d.EventID, d.Module, lease).Error
		return
	}
	h.finish(db, d, lease, err)
}

// runHandlers calls a module's handlers in turn, all under one deadline. A
// handler that overruns is abandoned and the delivery counts as failed.
func (h *Host) runHandlers(ctx context.Context, handlers []modkit.EventHandler, e modkit.Event) error {
	timeout := h.deliveryTimeout()
	hctx, cancel := context.WithTimeout(modkit.WithOrgID(ctx, e.OrgID), timeout)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		defer func() {
			if p := recover(); p != nil {
				done <- fmt.Errorf("panic: %v", p)
			}
		}()
		for _, fn := range handlers {
			if err := fn(hctx, e); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	select {
	case err := <-done:
		return err
	case <-hctx.Done():
		return fmt.Errorf("did not finish within %s", timeout)
	}
}

// ExpireEvents drops deliveries older than EventRetention, pending or failed,
// for one module or (module "") all, and the events no delivery needs.
func (h *Host) ExpireEvents(ctx context.Context, module string) {
	if err := dropDeliveries(h.db.WithContext(ctx), module, dbx.Now().Add(-EventRetention)); err != nil && ctx.Err() == nil {
		h.log.Error("modhost:expiring module event deliveries failed", "error", err)
	}
}

// dropDeliveries deletes the deliveries of events created before cutoff (all
// of them when cutoff is zero), for one module or all, then orphaned events.
func dropDeliveries(db *gorm.DB, module string, cutoff time.Time) error {
	q := `DELETE FROM module_event_deliveries WHERE 1 = 1`
	var args []any
	if module != "" {
		q += ` AND module = ?`
		args = append(args, module)
	}
	if !cutoff.IsZero() {
		q += ` AND event_id IN (SELECT id FROM module_events WHERE created_at < ?)`
		args = append(args, cutoff)
	}
	if err := db.Exec(q, args...).Error; err != nil {
		return err
	}
	return db.Exec(`DELETE FROM module_events WHERE NOT EXISTS
		(SELECT 1 FROM module_event_deliveries WHERE module_event_deliveries.event_id = module_events.id)`).Error
}

func (h *Host) retryBase() time.Duration {
	if h.EventRetryBase > 0 {
		return h.EventRetryBase
	}
	return DefaultEventRetryBase
}

// finish deletes a handled delivery (and its event once no module is left),
// or schedules a failed one for retry, until its attempts run out.
func (h *Host) finish(db *gorm.DB, d deliveryRow, lease string, failure error) {
	if failure == nil {
		err := db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Exec(`DELETE FROM module_event_deliveries WHERE event_id = ? AND module = ? AND claimed_by = ?`,
				d.EventID, d.Module, lease).Error; err != nil {
				return err
			}
			return tx.Exec(`DELETE FROM module_events WHERE id = ? AND NOT EXISTS
				(SELECT 1 FROM module_event_deliveries WHERE event_id = ?)`, d.EventID, d.EventID).Error
		})
		if err != nil {
			h.log.Error("modhost:completing a module event delivery failed; it will be delivered again", "eventId", d.EventID, "module", d.Module, "error", err)
		}
		return
	}
	attempts := d.Attempts + 1
	msg := failure.Error()
	if len(msg) > 1000 {
		msg = msg[:1000]
	}
	now := dbx.Now()
	if attempts >= eventMaxAttempts {
		h.log.Error("modhost:module event delivery failed for good", "eventId", d.EventID, "module", d.Module, "attempts", attempts, "error", failure)
		_ = db.Exec(`UPDATE module_event_deliveries SET attempts = ?, last_error = ?, failed_at = ?, claimed_by = NULL
			WHERE event_id = ? AND module = ? AND claimed_by = ?`, attempts, msg, now, d.EventID, d.Module, lease).Error
		return
	}
	delay := min(h.retryBase()<<(attempts-1), maxEventRetry)
	h.log.Warn("modhost:module event delivery failed; retrying", "eventId", d.EventID, "module", d.Module, "attempts", attempts, "retryIn", delay, "error", failure)
	_ = db.Exec(`UPDATE module_event_deliveries SET attempts = ?, last_error = ?, available_at = ?, claimed_by = NULL
		WHERE event_id = ? AND module = ? AND claimed_by = ?`, attempts, msg, now.Add(delay), d.EventID, d.Module, lease).Error
}
