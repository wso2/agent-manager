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

// Package tracesv1 is the "traces.query/v1" capability: reading an
// organization's traces in process.
package tracesv1

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ID is the capability id.
const ID = "traces.query/v1"

var (
	// ErrMissingOrg is returned when a Scope names no organization.
	ErrMissingOrg = errors.New("tracesv1: scope has no organization")
	// ErrForeignOrg is returned when a Scope names another organization than the caller's.
	ErrForeignOrg = errors.New("tracesv1: scope is not the caller's organization")
	// ErrNotFound is returned when a trace or span is not in the Scope, or
	// the Scope's project, agent or environment does not exist.
	ErrNotFound = errors.New("tracesv1: not found")
)

// Query reads traces for one Scope at a time. Every call refuses a Scope
// whose OrgID is not the organization of ctx (modkit.OrgID).
type Query interface {
	// QueryTraces lists the traces in the Scope and time window.
	QueryTraces(ctx context.Context, s Scope, q TraceQuery) (*TracePage, error)
	// QueryTraceSpans lists one trace's spans in the Scope.
	QueryTraceSpans(ctx context.Context, s Scope, traceID string, q SpanQuery) (*SpanPage, error)
	// GetSpanDetails reads one span with its attributes.
	GetSpanDetails(ctx context.Context, s Scope, traceID, spanID string) (*SpanDetails, error)
}

// Scope is who a query is for, by ID. OrgID is required; an empty
// ProjectID, AgentID or EnvironmentID matches any.
type Scope struct {
	OrgID         string
	ProjectID     string
	AgentID       string
	EnvironmentID string
}

// Validate rejects a Scope with no organization.
func (s Scope) Validate() error {
	if strings.TrimSpace(s.OrgID) == "" {
		return ErrMissingOrg
	}
	return nil
}

// TraceQuery is the time window and paging of a trace list.
type TraceQuery struct {
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	// SortOrder is "asc" or "desc" by start time.
	SortOrder string
}

// SpanQuery is the time window and paging of a span list. The window is the
// trace list's: a trace's spans are read as that list counts them. A zero
// StartTime or EndTime leaves that side open.
type SpanQuery struct {
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	// SortOrder is "asc" or "desc"; empty leaves the backend's order.
	SortOrder string
	// IncludeAttributes fills each span's Attributes and ResourceAttributes.
	IncludeAttributes bool
}

// TracePage is one page of traces, with the window's total.
type TracePage struct {
	Traces []Trace
	Total  int
}

// Trace is one trace in a list.
type Trace struct {
	TraceID      string
	TraceName    string
	SpanCount    int
	RootSpanID   string
	RootSpanName string
	RootSpanKind string
	StartTime    time.Time
	EndTime      time.Time
	DurationNs   int64
}

// SpanPage is one page of a trace's spans, with the trace's total.
type SpanPage struct {
	Spans []Span
	Total int
}

// Span is one span in a span list.
type Span struct {
	SpanID             string
	SpanName           string
	ParentSpanID       string
	StartTime          time.Time
	EndTime            time.Time
	DurationNs         int64
	Kind               string
	Status             *SpanStatus
	Attributes         map[string]any
	ResourceAttributes map[string]any
}

// SpanStatus is a span's OTel status.
type SpanStatus struct {
	Code    string
	Message string
}

// SpanDetails is one span with its attributes.
type SpanDetails struct {
	SpanID             string
	SpanName           string
	ParentSpanID       string
	StartTime          time.Time
	EndTime            time.Time
	DurationNs         int64
	Kind               string
	Status             *SpanStatus
	Attributes         map[string]any
	ResourceAttributes map[string]any
}
