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

// Package memory is an in-memory store.TraceStore for tests.
package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/wso2/agent-manager/agent-manager-observer/store"
)

type entry struct {
	scope store.Scope
	trace store.Trace
	spans []store.SpanDetails
}

// Store holds traces in memory and filters them on the Scope's IDs.
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

// QueryTraces lists the Scope's traces in the window.
func (m *Store) QueryTraces(_ context.Context, s store.Scope, q store.TraceQuery) (*store.TracePage, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries++

	var traces []store.Trace
	for _, e := range m.entries {
		if inScope(e.scope, s) && inWindow(e.trace, q) {
			traces = append(traces, e.trace)
		}
	}
	sort.SliceStable(traces, func(i, j int) bool {
		if q.SortOrder == "asc" {
			return traces[i].StartTime.Before(traces[j].StartTime)
		}
		return traces[i].StartTime.After(traces[j].StartTime)
	})
	total := len(traces)
	if q.Limit > 0 && len(traces) > q.Limit {
		traces = traces[:q.Limit]
	}
	return &store.TracePage{Traces: traces, Total: total}, nil
}

// QueryTraceSpans lists one of the Scope's traces' spans.
func (m *Store) QueryTraceSpans(_ context.Context, s store.Scope, traceID string, q store.SpanQuery) (*store.SpanPage, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries++

	e, ok := m.find(s, traceID)
	if !ok {
		return &store.SpanPage{}, nil
	}
	spans := make([]store.Span, 0, len(e.spans))
	for _, d := range e.spans {
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
	total := len(spans)
	if q.Limit > 0 && len(spans) > q.Limit {
		spans = spans[:q.Limit]
	}
	return &store.SpanPage{Spans: spans, Total: total}, nil
}

// GetSpanDetails reads one span of the Scope's trace.
func (m *Store) GetSpanDetails(_ context.Context, s store.Scope, traceID, spanID string) (*store.SpanDetails, error) {
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

// find returns the Scope's trace with traceID; a Scope with no org matches on the ID alone.
func (m *Store) find(s store.Scope, traceID string) (entry, bool) {
	for _, e := range m.entries {
		if e.trace.TraceID != traceID {
			continue
		}
		if s.OrgID == "" && s.Names.Org == "" || inScope(e.scope, s) {
			return e, true
		}
	}
	return entry{}, false
}

// inScope reports whether a trace stored under have is visible to want.
func inScope(have, want store.Scope) bool {
	return have.OrgID == want.OrgID &&
		matches(have.ProjectID, want.ProjectID) &&
		matches(have.AgentID, want.AgentID) &&
		matches(have.EnvironmentID, want.EnvironmentID)
}

// matches treats an empty filter as any value.
func matches(have, want string) bool {
	return want == "" || have == want
}

func inWindow(t store.Trace, q store.TraceQuery) bool {
	if !q.StartTime.IsZero() && t.StartTime.Before(q.StartTime) {
		return false
	}
	return q.EndTime.IsZero() || !t.StartTime.After(q.EndTime)
}
