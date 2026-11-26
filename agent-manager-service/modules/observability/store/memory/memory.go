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

// Package memory is an in-process store.TraceStore.
package memory

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
)

type entry struct {
	scope store.Scope
	trace store.Trace
	spans []store.SpanDetails
}

// Store holds traces in memory and filters them on the Scope.
type Store struct {
	mu      sync.Mutex
	entries []entry
	queries int
}

var _ store.TraceStore = (*Store)(nil)

// New returns an empty Store.
func New() *Store {
	return &Store{}
}

// Add stores trace and its spans under scope.
func (m *Store) Add(scope store.Scope, trace store.Trace, spans ...store.SpanDetails) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.entries = append(m.entries, entry{scope: scope, trace: trace, spans: spans})
}

// Queries counts the lookups the Store has run.
func (m *Store) Queries() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.queries
}

// DeleteAgent drops an agent's traces in an organization.
func (m *Store) DeleteAgent(_ context.Context, orgID, agentID string) error {
	if orgID == "" {
		return store.ErrMissingOrg
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	kept := m.entries[:0]
	for _, e := range m.entries {
		if e.scope.OrgID != orgID || e.scope.AgentID != agentID {
			kept = append(kept, e)
		}
	}
	clear(m.entries[len(kept):])
	m.entries = kept
	return nil
}

// cursorTag starts the memory store's cursors: the backend and the cursor version.
const cursorTag = "mem1"

// QueryTraces lists the Scope's traces in the window that pass the filters,
// one page at a time. A trace stored with spans is counted and filtered within
// store.TraceBounds of the window, as its span list is.
func (m *Store) QueryTraces(_ context.Context, s store.Scope, q store.TraceQuery) (*store.TracePage, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	order := "desc"
	if q.SortOrder == "asc" {
		order = "asc"
	}
	var after *store.CursorKey
	if q.Cursor != "" {
		k, err := store.DecodeCursor(cursorTag, q.Cursor, order)
		if err != nil {
			return nil, err
		}
		after = &k
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries++

	from, to := store.TraceBounds(q.StartTime, q.EndTime)
	traces := []store.Trace{}
	for _, e := range m.entries {
		if !inScope(e.scope, s) || !inWindow(e.trace.StartTime, q.StartTime, q.EndTime) {
			continue
		}
		tr, spans := e.trace, e.spans
		if len(e.spans) > 0 {
			spans = spansIn(e.spans, from, to)
			tr.SpanCount = len(spans)
		}
		if store.MatchTrace(q.Filters, tr, spans) {
			traces = append(traces, tr)
		}
	}
	sort.SliceStable(traces, func(i, j int) bool {
		a, b := traces[i], traces[j]
		k := store.CursorKey{Order: order, StartNs: a.StartTime.UnixNano(), TraceID: a.TraceID}
		return k.After(b.StartTime.UnixNano(), b.TraceID)
	})
	total := len(traces)
	if after != nil {
		i := sort.Search(len(traces), func(i int) bool {
			return after.After(traces[i].StartTime.UnixNano(), traces[i].TraceID)
		})
		traces = traces[i:]
	}
	page := &store.TracePage{Traces: traces, Total: &total}
	if q.Limit > 0 && len(traces) > q.Limit {
		page.Traces = traces[:q.Limit]
		last := page.Traces[q.Limit-1]
		page.NextCursor = store.EncodeCursor(cursorTag, store.CursorKey{
			Order: order, StartNs: last.StartTime.UnixNano(), TraceID: last.TraceID,
		})
	}
	return page, nil
}

// QueryTraceSpans lists one of the Scope's traces' spans, within
// store.TraceBounds of the window.
func (m *Store) QueryTraceSpans(_ context.Context, s store.Scope, traceID string, q store.SpanQuery) (*store.SpanPage, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries++

	e, ok := m.find(s, traceID)
	if !ok {
		return &store.SpanPage{Spans: []store.Span{}}, nil
	}
	return spansOf(e, q), nil
}

// QuerySpansByTrace lists several of the Scope's traces' spans in one lookup.
func (m *Store) QuerySpansByTrace(_ context.Context, s store.Scope, traceIDs []string, q store.SpanQuery) (map[string]*store.SpanPage, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries++

	pages := map[string]*store.SpanPage{}
	for _, id := range traceIDs {
		if e, ok := m.find(s, id); ok {
			if p := spansOf(e, q); len(p.Spans) > 0 {
				pages[id] = p
			}
		}
	}
	return pages, nil
}

// spansOf is a trace's spans within store.TraceBounds of q's window.
// spansIn is the spans that start between from and to.
func spansIn(spans []store.SpanDetails, from, to time.Time) []store.SpanDetails {
	out := make([]store.SpanDetails, 0, len(spans))
	for _, d := range spans {
		if inWindow(d.StartTime, from, to) {
			out = append(out, d)
		}
	}
	return out
}

func spansOf(e entry, q store.SpanQuery) *store.SpanPage {
	from, to := store.TraceBounds(q.StartTime, q.EndTime)
	spans := make([]store.Span, 0, len(e.spans))
	for _, d := range e.spans {
		if !inWindow(d.StartTime, from, to) {
			continue
		}
		sp := store.Span{
			SpanID: d.SpanID, SpanName: d.SpanName, ParentSpanID: d.ParentSpanID,
			StartTime: d.StartTime, EndTime: d.EndTime, DurationNs: d.DurationNs,
			Kind: d.Kind, Status: d.Status,
		}
		if q.IncludeAttributes {
			sp.Attributes, sp.ResourceAttributes = d.Attributes, d.ResourceAttributes
		}
		spans = append(spans, sp)
	}
	sort.SliceStable(spans, func(i, j int) bool {
		if q.SortOrder == "desc" {
			return spans[i].StartTime.After(spans[j].StartTime)
		}
		return spans[i].StartTime.Before(spans[j].StartTime)
	})
	total := len(spans)
	if q.Limit > 0 && len(spans) > q.Limit {
		spans = spans[:q.Limit]
	}
	return &store.SpanPage{Spans: spans, Total: total}
}

// GetSpanDetails reads one span of the Scope's trace.
func (m *Store) GetSpanDetails(_ context.Context, s store.Scope, traceID, spanID string) (*store.SpanDetails, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries++

	e, ok := m.find(s, traceID)
	if !ok {
		return nil, store.ErrNotFound
	}
	for _, d := range e.spans {
		if d.SpanID == spanID {
			return &d, nil
		}
	}
	return nil, store.ErrNotFound
}

// find returns the Scope's trace with traceID.
func (m *Store) find(s store.Scope, traceID string) (entry, bool) {
	for _, e := range m.entries {
		if e.trace.TraceID == traceID && inScope(e.scope, s) {
			return e, true
		}
	}
	return entry{}, false
}

// inScope reports whether a trace stored under have is visible to want; the
// org must match on both ID and name, and an empty one matches nothing. Each
// set project, agent and environment ID or name must match too.
func inScope(have, want store.Scope) bool {
	h, w := have.Names, want.Names
	return have.OrgID != "" && have.OrgID == want.OrgID &&
		h.Org != "" && h.Org == w.Org &&
		matches(have.ProjectID, want.ProjectID) && matches(h.Project, w.Project) &&
		matches(have.AgentID, want.AgentID) && matches(h.Agent, w.Agent) &&
		matches(have.EnvironmentID, want.EnvironmentID) && matches(h.Environment, w.Environment)
}

// matches treats an empty filter as any value.
func matches(have, want string) bool {
	return want == "" || have == want
}

// inWindow reports whether at is within [from, to]; a zero bound is open.
func inWindow(at, from, to time.Time) bool {
	if !from.IsZero() && at.Before(from) {
		return false
	}
	return to.IsZero() || !at.After(to)
}
