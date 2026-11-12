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

// Package tracesv1 is the traces.query/v1 capability: how other modules read traces in process.
package tracesv1

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-observer/store"
)

// Capability names this version of the trace query capability.
const Capability = "traces.query/v1"

// Query is the same three reads as store.TraceStore; a breaking change ships as tracesv2.
type Query interface {
	// QueryTraces lists the traces in the Scope and time window.
	QueryTraces(ctx context.Context, s Scope, q TraceQuery) (*TracePage, error)
	// QueryTraceSpans lists one trace's spans in the Scope.
	QueryTraceSpans(ctx context.Context, s Scope, traceID string, q SpanQuery) (*SpanPage, error)
	// GetSpanDetails reads one span with its attributes.
	GetSpanDetails(ctx context.Context, s Scope, traceID, spanID string) (*SpanDetails, error)
}

// The capability's types are the store's.
type (
	Scope       = store.Scope
	Names       = store.Names
	TraceQuery  = store.TraceQuery
	SpanQuery   = store.SpanQuery
	TracePage   = store.TracePage
	Trace       = store.Trace
	SpanPage    = store.SpanPage
	Span        = store.Span
	SpanStatus  = store.SpanStatus
	SpanDetails = store.SpanDetails
)

// Every TraceStore serves Query.
var _ Query = store.TraceStore(nil)
