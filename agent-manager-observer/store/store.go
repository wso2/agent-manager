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

// Package store is the trace data access layer: one interface, many backends.
package store

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	// ErrMissingOrg is returned when a query's Scope names no organization.
	ErrMissingOrg = errors.New("store: scope has no organization")
	// ErrDisabled is returned by every call when traces are turned off.
	ErrDisabled = errors.New("store: traces are disabled")
	// ErrNotFound is returned when a trace or span is not in the Scope.
	ErrNotFound = errors.New("store: not found")
)

// TraceStore reads traces for one Scope at a time.
type TraceStore interface {
	// QueryTraces lists the traces in the Scope and time window.
	QueryTraces(ctx context.Context, s Scope, q TraceQuery) (*TracePage, error)
	// QueryTraceSpans lists one trace's spans in the Scope.
	QueryTraceSpans(ctx context.Context, s Scope, traceID string, q SpanQuery) (*SpanPage, error)
	// GetSpanDetails reads one span with its attributes.
	GetSpanDetails(ctx context.Context, s Scope, traceID, spanID string) (*SpanDetails, error)
}

// Names are the scope names as the caller sent them.
type Names struct {
	Org         string
	Project     string
	Agent       string
	Environment string
}

// Scope is who a query is for: the caller's names, and the IDs they resolve to.
type Scope struct {
	OrgID         string
	ProjectID     string
	AgentID       string
	EnvironmentID string
	Names         Names
}

// Validate rejects a Scope with no organization.
func (s Scope) Validate() error {
	if strings.TrimSpace(s.Names.Org) == "" {
		return ErrMissingOrg
	}
	return nil
}

// TraceQuery is the time window and paging of a trace list.
type TraceQuery struct {
	StartTime time.Time
	EndTime   time.Time
	Limit     int
	SortOrder string
}

// SpanQuery is the time window and paging of a span list.
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
	Attributes         map[string]interface{}
	ResourceAttributes map[string]interface{}
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
	Attributes         map[string]interface{}
	ResourceAttributes map[string]interface{}
}
