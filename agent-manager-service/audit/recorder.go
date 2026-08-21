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

package audit

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Recorder persists audit events.
//
// Record is asynchronous and fail-open: it never blocks a request and never
// returns an error, because an audit backlog must not take the control plane
// down. RecordSync is synchronous and fail-closed, for the operations where an
// unrecordable action should not happen at all.
type Recorder interface {
	// Record queues an event. Safe from any goroutine; a no-op after Close.
	Record(ctx context.Context, e Event)
	// RecordSync writes an event before returning, reporting failure to the
	// caller so it can refuse the operation.
	RecordSync(ctx context.Context, e Event) error
	// Close drains buffered events and flushes sinks.
	Close(ctx context.Context) error
}

// Config tunes the buffered recorder.
type Config struct {
	// BufferSize bounds queued events. Overflow drops rather than blocking.
	BufferSize int
	// BatchSize is the maximum number of events written in one sink call.
	BatchSize int
	// FlushInterval bounds how long an event waits before being written.
	FlushInterval time.Duration
}

// DefaultConfig returns the tuning used when configuration omits values.
//
// A 4096-event buffer flushed every second at 200 per batch absorbs roughly
// twenty seconds of a sustained 200 rps mutation burst before it has to drop.
func DefaultConfig() Config {
	return Config{
		BufferSize:    4096,
		BatchSize:     200,
		FlushInterval: time.Second,
	}
}

type bufferedRecorder struct {
	sink   Sink
	logger *slog.Logger
	cfg    Config

	ch chan Event
	// stopping is closed by Close. The event channel itself is never closed:
	// a producer cannot check a flag and send atomically, so closing the
	// channel a request goroutine may be sending on is a send-on-closed panic
	// waiting to happen. Signalling through a second channel removes the race
	// rather than narrowing it — servers can still be draining requests when
	// Close runs, and background workers are not joined at all.
	stopping chan struct{}
	closed   atomic.Bool
	done     chan struct{}
	once     sync.Once

	dropped  atomic.Uint64
	lastWarn atomic.Int64
}

// NewRecorder returns a buffered recorder writing to sink.
func NewRecorder(sink Sink, logger *slog.Logger, cfg Config) Recorder {
	if logger == nil {
		logger = slog.Default()
	}
	def := DefaultConfig()
	if cfg.BufferSize <= 0 {
		cfg.BufferSize = def.BufferSize
	}
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = def.BatchSize
	}
	if cfg.FlushInterval <= 0 {
		cfg.FlushInterval = def.FlushInterval
	}
	r := &bufferedRecorder{
		sink:     sink,
		logger:   logger,
		cfg:      cfg,
		ch:       make(chan Event, cfg.BufferSize),
		stopping: make(chan struct{}),
		done:     make(chan struct{}),
	}
	go r.run()
	return r
}

// Record queues an event, dropping it if the buffer is full.
//
// Dropping is the correct failure mode here: the alternative is blocking a
// request goroutine on the audit path, which turns a slow sink into an outage.
// Drops are counted and reported so the loss is visible rather than silent.
func (r *bufferedRecorder) Record(_ context.Context, e Event) {
	if r.closed.Load() {
		// Best-effort early exit once shutdown has begun. Correctness does not
		// depend on it: the send below is safe whether or not this races.
		return
	}
	prepare(&e)
	select {
	case r.ch <- e:
	default:
		r.noteDrop(e)
	}
}

// RecordSync writes immediately and reports failure.
//
// Used for operations where proceeding without a record is worse than failing:
// the caller refuses the operation when this returns an error. Note the honest
// limit of that guarantee with a stdout sink — a successful write means the
// record reached the process's output, not that it reached durable storage.
func (r *bufferedRecorder) RecordSync(ctx context.Context, e Event) error {
	prepare(&e)
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()
	return r.sink.Write(writeCtx, []Event{e})
}

// Close drains the buffer and stops the worker.
func (r *bufferedRecorder) Close(ctx context.Context) error {
	var err error
	r.once.Do(func() {
		r.closed.Store(true)
		close(r.stopping)
		select {
		case <-r.done:
		case <-ctx.Done():
			err = ctx.Err()
		}
		if dropped := r.dropped.Load(); dropped > 0 {
			r.logger.Error("audit events were dropped during this process lifetime",
				"dropped", dropped, "sink", r.sink.Name())
		}
	})
	return err
}

// run batches events by size and by time until the channel closes.
func (r *bufferedRecorder) run() {
	defer close(r.done)

	ticker := time.NewTicker(r.cfg.FlushInterval)
	defer ticker.Stop()

	batch := make([]Event, 0, r.cfg.BatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), writeTimeout)
		if err := r.sink.Write(ctx, batch); err != nil {
			r.logger.Error("audit sink write failed; events lost",
				"error", err, "sink", r.sink.Name(), "count", len(batch))
		}
		cancel()
		batch = batch[:0]
	}

	for {
		select {
		case e := <-r.ch:
			batch = append(batch, e)
			if len(batch) >= r.cfg.BatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		case <-r.stopping:
			// Drain whatever is already queued, then stop. Anything a late
			// producer sends after this is left in the buffer and lost, which
			// is the same outcome as dropping on a full buffer — and far
			// better than panicking on a closed channel mid-request.
			for {
				select {
				case e := <-r.ch:
					batch = append(batch, e)
					if len(batch) >= r.cfg.BatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}

// noteDrop counts a dropped event and warns at most once every 30 seconds, so
// that a sustained overflow does not itself become a log flood.
func (r *bufferedRecorder) noteDrop(e Event) {
	total := r.dropped.Add(1)
	now := time.Now().Unix()
	last := r.lastWarn.Load()
	if now-last < 30 {
		return
	}
	if !r.lastWarn.CompareAndSwap(last, now) {
		return
	}
	r.logger.Error("audit buffer full; event dropped",
		"action", string(e.Action),
		"actor_id", e.ActorID,
		"dropped_total", total,
		"sink", r.sink.Name())
}

// prepare stamps identity and applies redaction. Every path into a sink goes
// through here, so no sink can receive an unredacted event.
func prepare(e *Event) {
	if e.EventID == uuid.Nil {
		e.EventID = uuid.New()
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	}
	e.SchemaVersion = SchemaVersion
	if e.ActionClass == "" {
		e.ActionClass = e.Action.Class()
	}
	if e.Severity == 0 {
		e.Severity = e.Action.Severity()
	}
	if e.Outcome == "" {
		e.Outcome = OutcomeSuccess
	}
	if e.ActorType == "" {
		e.ActorType = ActorAnonymous
	}
	if e.Surface == "" {
		e.Surface = SurfaceSystem
	}
	redact(e)
}

// noopRecorder discards events. Used when auditing is disabled by configuration.
type noopRecorder struct{}

// NewNoopRecorder returns a recorder that discards events, for deployments that
// have deliberately turned auditing off.
func NewNoopRecorder() Recorder { return noopRecorder{} }

func (noopRecorder) Record(context.Context, Event)           {}
func (noopRecorder) RecordSync(context.Context, Event) error { return nil }
func (noopRecorder) Close(context.Context) error             { return nil }

// uninstalledRecorder is what FromContext returns when no recorder was
// installed on the context.
//
// This is the failure mode of a context-carried recorder: a missing
// installation would otherwise silently produce an empty audit trail, which is
// indistinguishable from "nothing happened". Complaining loudly converts that
// into an obvious defect. Unlike noopRecorder it also fails RecordSync, so a
// fail-closed emit site refuses rather than proceeding unrecorded.
type uninstalledRecorder struct {
	warned atomic.Bool
}

var uninstalled = &uninstalledRecorder{}

func (u *uninstalledRecorder) warn(action Action) {
	if u.warned.Swap(true) {
		return
	}
	//nolint:forbidigo // reports a missing recorder, so the context it was handed cannot be trusted to carry a logger either
	slog.Error("audit recorder not installed on context; events are being lost",
		"action", string(action),
		"hint", "install one with audit.WithRecorder in the surface's middleware")
}

func (u *uninstalledRecorder) Record(_ context.Context, e Event) { u.warn(e.Action) }

func (u *uninstalledRecorder) RecordSync(_ context.Context, e Event) error {
	u.warn(e.Action)
	return ErrRecorderUnavailable
}

func (u *uninstalledRecorder) Close(context.Context) error { return nil }
