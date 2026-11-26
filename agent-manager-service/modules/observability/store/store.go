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

// Package store is the observability module's trace data access layer: one
// interface, many backends.
package store

import (
	"context"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/tracesv1"
)

var (
	// ErrMissingOrg is returned when a query's Scope names no organization.
	ErrMissingOrg = tracesv1.ErrMissingOrg
	// ErrNotFound is returned when a trace or span is not in the Scope.
	ErrNotFound = tracesv1.ErrNotFound
	// ErrInvalidCursor is returned for a cursor the store did not issue.
	ErrInvalidCursor = tracesv1.ErrInvalidCursor
)

// TraceStore reads traces for one Scope at a time. A trace list's span counts
// and a span list's spans are both read within TraceBounds of the window, so
// a trace shows the same spans in both.
type TraceStore interface {
	// QueryTraces lists the traces whose root span starts in the window.
	QueryTraces(ctx context.Context, s Scope, q TraceQuery) (*TracePage, error)
	// QueryTraceSpans lists one trace's spans in the Scope.
	QueryTraceSpans(ctx context.Context, s Scope, traceID string, q SpanQuery) (*SpanPage, error)
	// QuerySpansByTrace lists the spans of several traces, at most q.Limit
	// each, by trace ID; a trace with no spans is left out.
	QuerySpansByTrace(ctx context.Context, s Scope, traceIDs []string, q SpanQuery) (map[string]*SpanPage, error)
	// GetSpanDetails reads one span with its attributes.
	GetSpanDetails(ctx context.Context, s Scope, traceID, spanID string) (*SpanDetails, error)
}

// ClockSkew is how far before its root a child span may be stamped.
const ClockSkew = time.Minute

// TraceSpread is how long after the window a listed trace's spans are read.
const TraceSpread = 24 * time.Hour

// TraceBounds widens a window to where its traces' spans are read: ClockSkew
// before it and TraceSpread after it. A zero bound stays open.
func TraceBounds(from, to time.Time) (time.Time, time.Time) {
	if !from.IsZero() {
		from = from.Add(-ClockSkew)
	}
	if !to.IsZero() {
		to = to.Add(TraceSpread)
	}
	return from, to
}

// Names are the names a Scope's IDs resolve to.
type Names struct {
	Org         string
	Project     string
	Agent       string
	Environment string
}

// Scope is who a query is for: the IDs and the names they resolve to.
type Scope struct {
	OrgID         string
	ProjectID     string
	AgentID       string
	EnvironmentID string
	Names         Names
}

// Validate rejects a Scope with no organization ID or name.
func (s Scope) Validate() error {
	if strings.TrimSpace(s.OrgID) == "" || strings.TrimSpace(s.Names.Org) == "" {
		return ErrMissingOrg
	}
	return nil
}

// The store's other types are the capability's.
type (
	TraceQuery   = tracesv1.TraceQuery
	TraceFilters = tracesv1.TraceFilters
	SpanQuery    = tracesv1.SpanQuery
	TracePage    = tracesv1.TracePage
	Trace        = tracesv1.Trace
	SpanPage     = tracesv1.SpanPage
	Span         = tracesv1.Span
	SpanStatus   = tracesv1.SpanStatus
	SpanDetails  = tracesv1.SpanDetails
)
