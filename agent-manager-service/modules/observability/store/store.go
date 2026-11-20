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

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/tracesv1"
)

var (
	// ErrMissingOrg is returned when a query's Scope names no organization.
	ErrMissingOrg = tracesv1.ErrMissingOrg
	// ErrNotFound is returned when a trace or span is not in the Scope.
	ErrNotFound = tracesv1.ErrNotFound
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

// The store's types are the capability's.
type (
	Scope       = tracesv1.Scope
	Names       = tracesv1.Names
	TraceQuery  = tracesv1.TraceQuery
	SpanQuery   = tracesv1.SpanQuery
	TracePage   = tracesv1.TracePage
	Trace       = tracesv1.Trace
	SpanPage    = tracesv1.SpanPage
	Span        = tracesv1.Span
	SpanStatus  = tracesv1.SpanStatus
	SpanDetails = tracesv1.SpanDetails
)
