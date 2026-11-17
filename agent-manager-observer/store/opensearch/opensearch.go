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
// Package opensearch reads traces from the OpenSearch span index.
package opensearch

import (
	"context"
	"errors"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	ospkg "github.com/wso2/agent-manager/agent-manager-observer/opensearch"
	"github.com/wso2/agent-manager/agent-manager-observer/store"
)

// Store is a store.TraceStore backed by the OpenSearch trace client.
type Store struct {
	client observer.Client
}

var _ store.TraceStore = (*Store)(nil)

// New returns a Store that reads through client.
func New(client observer.Client) *Store {
	return &Store{client: client}
}

// QueryTraces lists traces of the Scope's organization, project and agent.
func (s *Store) QueryTraces(ctx context.Context, sc store.Scope, q store.TraceQuery) (*store.TracePage, error) {
	if err := sc.Validate(); err != nil {
		return nil, err
	}
	limit, sortOrder := q.Limit, q.SortOrder
	resp, err := s.client.QueryTraces(ctx, observer.TracesQueryRequest{
		StartTime:   q.StartTime,
		EndTime:     q.EndTime,
		Limit:       &limit,
		SortOrder:   &sortOrder,
		SearchScope: traceScope(sc),
	})
	if err != nil || resp == nil {
		return nil, err
	}
	page := &store.TracePage{Traces: make([]store.Trace, len(resp.Traces)), Total: resp.Total}
	for i, t := range resp.Traces {
		page.Traces[i] = store.Trace(t)
	}
	return page, nil
}

// QueryTraceSpans lists one trace's spans in the Scope.
func (s *Store) QueryTraceSpans(ctx context.Context, sc store.Scope, traceID string, q store.SpanQuery) (*store.SpanPage, error) {
	if err := sc.Validate(); err != nil {
		return nil, err
	}
	limit := q.Limit
	resp, err := s.client.QueryTraceSpans(ctx, traceID, observer.TracesQueryRequest{
		StartTime:         q.StartTime,
		EndTime:           q.EndTime,
		Limit:             &limit,
		SortOrder:         optional(q.SortOrder),
		SearchScope:       traceScope(sc),
		IncludeAttributes: q.IncludeAttributes,
	})
	if err != nil || resp == nil {
		return nil, err
	}
	page := &store.SpanPage{Spans: make([]store.Span, len(resp.Spans)), Total: resp.Total}
	for i, sp := range resp.Spans {
		page.Spans[i] = store.Span{
			SpanID:             sp.SpanID,
			SpanName:           sp.SpanName,
			ParentSpanID:       sp.ParentSpanID,
			StartTime:          sp.StartTime,
			EndTime:            sp.EndTime,
			DurationNs:         sp.DurationNs,
			Kind:               sp.Kind,
			Status:             toStatus(sp.Status),
			Attributes:         sp.Attributes,
			ResourceAttributes: sp.ResourceAttributes,
		}
	}
	return page, nil
}

// GetSpanDetails reads one span; a Scope with an organization only sees that organization's spans.
func (s *Store) GetSpanDetails(ctx context.Context, sc store.Scope, traceID, spanID string) (*store.SpanDetails, error) {
	d, err := s.client.GetSpanDetails(ctx, traceID, spanID)
	if errors.Is(err, observer.ErrSpanNotFound) {
		return nil, store.ErrNotFound
	}
	if err != nil || d == nil {
		return nil, err
	}
	if org := sc.Names.Org; org != "" && d.ResourceAttributes[ospkg.ResourceOrganization] != org {
		return nil, store.ErrNotFound
	}
	return &store.SpanDetails{
		SpanID:             d.SpanID,
		SpanName:           d.SpanName,
		ParentSpanID:       d.ParentSpanID,
		StartTime:          d.StartTime,
		EndTime:            d.EndTime,
		DurationNs:         d.DurationNs,
		Kind:               d.Kind,
		Status:             toStatus(d.Status),
		Attributes:         d.Attributes,
		ResourceAttributes: d.ResourceAttributes,
	}, nil
}

// traceScope maps the Scope's names to the span resource attributes the client filters on.
func traceScope(sc store.Scope) observer.TraceScope {
	return observer.TraceScope{
		Organization: sc.Names.Org,
		Project:      optional(sc.Names.Project),
		Agent:        optional(sc.Names.Agent),
	}
}

func optional(v string) *string {
	if v == "" {
		return nil
	}
	return &v
}

func toStatus(st *observer.SpanStatus) *store.SpanStatus {
	if st == nil {
		return nil
	}
	return &store.SpanStatus{Code: st.Code, Message: st.Message}
}
