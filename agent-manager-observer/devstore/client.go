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

//go:build devstore

package devstore

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

const (
	defaultTraceLimit = 20
	defaultLogLimit   = 100
	sortAscending     = "asc"
)

var (
	dbPath     = "/tmp/amp-observer/traces.db"
	ingestAddr = ":4318"
)

type logQuerier interface {
	Query(ctx context.Context, req observer.LogsQueryRequest, limit int, desc bool) (*observer.LogsQueryResponse, error)
}

// Client serves observer queries from the local SQLite span store and the Kubernetes API.
type Client struct {
	spans            *SpanStore
	resolver         *Resolver
	logs             logQuerier
	defaultNamespace string
}

var _ observer.Client = (*Client)(nil)

// New opens the span store and Kubernetes clients, returning the client, the OTLP ingest handler
// with its listen address, and the function that closes the store.
func New(defaultNamespace string) (*Client, http.Handler, string, func() error, error) {
	store, err := OpenSpanStore(dbPath)
	if err != nil {
		return nil, nil, "", nil, err
	}
	dyn, typed, err := newKubeClients()
	if err != nil {
		_ = store.Close()
		return nil, nil, "", nil, err
	}
	client := &Client{
		spans:            store,
		resolver:         NewResolver(dyn, DefaultResolverTTL, time.Now),
		logs:             NewLogReader(typed),
		defaultNamespace: defaultNamespace,
	}
	return client, NewIngestHandler(store, DefaultMaxIngestBytes), ingestAddr, store.Close, nil
}

func (c *Client) NamespaceFor(string) string { return c.defaultNamespace }

func (c *Client) QueryTraces(ctx context.Context, req observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	scope, ok, err := c.resolver.Scope(ctx, req.SearchScope)
	if err != nil {
		return nil, fmt.Errorf("resolve trace scope: %w", err)
	}
	if !ok {
		return &observer.TracesQueryResponse{Traces: []observer.TraceInfo{}}, nil
	}
	traces, total, err := c.spans.QueryTraces(ctx, scope, windowOf(req))
	if err != nil {
		return nil, err
	}
	return &observer.TracesQueryResponse{Traces: traces, Total: total}, nil
}

func (c *Client) QueryTraceSpans(ctx context.Context, traceID string, req observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	scope, ok, err := c.resolver.Scope(ctx, req.SearchScope)
	if err != nil {
		return nil, fmt.Errorf("resolve trace scope: %w", err)
	}
	if !ok {
		return &observer.TraceSpansQueryResponse{Spans: []observer.SpanInfo{}}, nil
	}
	spans, total, err := c.spans.TraceSpans(ctx, traceID, scope, windowOf(req), req.IncludeAttributes)
	if err != nil {
		return nil, err
	}
	return &observer.TraceSpansQueryResponse{Spans: spans, Total: total}, nil
}

func (c *Client) GetSpanDetails(ctx context.Context, traceID, spanID string) (*observer.SpanDetailsResponse, error) {
	details, err := c.spans.SpanDetails(ctx, traceID, spanID)
	if errors.Is(err, ErrSpanNotFound) {
		return nil, fmt.Errorf("span %s/%s: %w", traceID, spanID, err)
	}
	return details, err
}

func (c *Client) QueryLogs(ctx context.Context, req observer.LogsQueryRequest) (*observer.LogsQueryResponse, error) {
	return c.logs.Query(ctx, req, limitOr(req.Limit, defaultLogLimit), descending(req.SortOrder))
}

func (c *Client) QueryMetrics(context.Context, observer.MetricsQueryRequest) (*observer.ResourceMetricsTimeSeries, error) {
	return &observer.ResourceMetricsTimeSeries{}, nil
}

func windowOf(req observer.TracesQueryRequest) Window {
	return Window{
		StartNs: req.StartTime.UnixNano(),
		EndNs:   req.EndTime.UnixNano(),
		Limit:   limitOr(req.Limit, defaultTraceLimit),
		Desc:    descending(req.SortOrder),
	}
}

func limitOr(limit *int, fallback int) int {
	if limit == nil {
		return fallback
	}
	return *limit
}

func descending(sortOrder *string) bool {
	return sortOrder == nil || *sortOrder != sortAscending
}
