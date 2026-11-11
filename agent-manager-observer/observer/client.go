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

// Package observer reads traces from the OpenSearch span index the OTel
// collector writes to.
package observer

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
)

// ErrSpanNotFound is returned by GetSpanDetails when no span matches.
var ErrSpanNotFound = errors.New("span not found")

const (
	// defaultSpanLimit applies when a spans query sets no limit.
	defaultSpanLimit = 1000
	// maxResultWindow is OpenSearch's default index.max_result_window.
	maxResultWindow = 10000
	// totalPrecision is the trace count up to which Total is exact.
	totalPrecision = 40000
)

// Client is the interface for querying trace data.
type Client interface {
	// QueryTraces fetches the traces whose root span starts in the window.
	QueryTraces(ctx context.Context, req TracesQueryRequest) (*TracesQueryResponse, error)

	// QueryTraceSpans fetches the spans of a specific trace.
	QueryTraceSpans(ctx context.Context, traceID string, req TracesQueryRequest) (*TraceSpansQueryResponse, error)

	// GetSpanDetails fetches a single span with its attributes.
	GetSpanDetails(ctx context.Context, traceID, spanID string) (*SpanDetailsResponse, error)
}

type clientImpl struct {
	baseURL    string
	index      string
	username   string
	password   string
	httpClient *http.Client
}

// NewClient creates an OpenSearch-backed trace client.
func NewClient(cfg config.OpenSearchConfig) Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.InsecureSkipVerify {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // opt-in for self-signed in-cluster OpenSearch
	}
	return &clientImpl{
		baseURL:    strings.TrimRight(cfg.Address, "/"),
		index:      cfg.TraceIndex,
		username:   cfg.Username,
		password:   cfg.Password,
		httpClient: &http.Client{Timeout: 30 * time.Second, Transport: transport},
	}
}

// spanDoc is a span document as the OTel collector's OpenSearch exporter writes it.
type spanDoc struct {
	TraceID         string                 `json:"traceId"`
	SpanID          string                 `json:"spanId"`
	ParentSpanID    string                 `json:"parentSpanId"`
	Name            string                 `json:"name"`
	Kind            string                 `json:"kind"`
	StartTime       time.Time              `json:"startTime"`
	EndTime         time.Time              `json:"endTime"`
	DurationInNanos float64                `json:"durationInNanos"`
	Status          *SpanStatus            `json:"status"`
	Attributes      map[string]interface{} `json:"attributes"`
	Resource        map[string]interface{} `json:"resource"`
}

func (d spanDoc) durationNs() int64 {
	if d.DurationInNanos > 0 {
		return int64(d.DurationInNanos)
	}
	return d.EndTime.Sub(d.StartTime).Nanoseconds()
}

type searchResponse struct {
	Hits struct {
		Total struct {
			Value int `json:"value"`
		} `json:"total"`
		Hits []struct {
			Source spanDoc `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
	Aggregations json.RawMessage `json:"aggregations"`
}

func (c *clientImpl) QueryTraces(ctx context.Context, req TracesQueryRequest) (*TracesQueryResponse, error) {
	limit := defaultSpanLimit
	if req.Limit != nil && *req.Limit > 0 {
		limit = min(*req.Limit, maxResultWindow)
	}
	order := sortOrder(req.SortOrder, "desc")
	filters := append(scopeFilters(req.SearchScope), rootSpanFilter(), map[string]any{
		"range": map[string]any{"startTime": map[string]any{
			"gte": req.StartTime.UTC().Format(time.RFC3339Nano),
			"lte": req.EndTime.UTC().Format(time.RFC3339Nano),
		}},
	})
	body := map[string]any{
		"size":     limit,
		"query":    map[string]any{"bool": map[string]any{"filter": filters}},
		"sort":     []any{map[string]any{"startTime": order}, map[string]any{"traceId": order}},
		"collapse": map[string]any{"field": "traceId"},
		"_source":  []string{"traceId", "spanId", "name", "kind", "startTime", "endTime", "durationInNanos"},
		"aggs": map[string]any{"traces": map[string]any{"cardinality": map[string]any{
			"field": "traceId", "precision_threshold": totalPrecision,
		}}},
	}
	var resp searchResponse
	if err := c.search(ctx, body, &resp); err != nil {
		return nil, fmt.Errorf("observer.QueryTraces: %w", err)
	}
	var aggs struct {
		Traces struct {
			Value int `json:"value"`
		} `json:"traces"`
	}
	if len(resp.Aggregations) > 0 {
		if err := json.Unmarshal(resp.Aggregations, &aggs); err != nil {
			return nil, fmt.Errorf("observer.QueryTraces: decode aggregations: %w", err)
		}
	}

	traces := make([]TraceInfo, 0, len(resp.Hits.Hits))
	ids := make([]string, 0, len(resp.Hits.Hits))
	for _, h := range resp.Hits.Hits {
		root := h.Source
		traces = append(traces, TraceInfo{
			TraceID:      root.TraceID,
			TraceName:    root.Name,
			RootSpanID:   root.SpanID,
			RootSpanName: root.Name,
			RootSpanKind: root.Kind,
			StartTime:    root.StartTime,
			EndTime:      root.EndTime,
			DurationNs:   root.durationNs(),
		})
		ids = append(ids, root.TraceID)
	}
	if err := c.addTraceStats(ctx, req.SearchScope, ids, traces); err != nil {
		return nil, fmt.Errorf("observer.QueryTraces: %w", err)
	}
	return &TracesQueryResponse{Traces: traces, Total: max(aggs.Traces.Value, len(traces))}, nil
}

// addTraceStats fills in each trace's span count and extends its end time to
// its last span.
func (c *clientImpl) addTraceStats(ctx context.Context, scope TraceScope, ids []string, traces []TraceInfo) error {
	if len(ids) == 0 {
		return nil
	}
	filters := append(scopeFilters(scope), map[string]any{"terms": map[string]any{"traceId": ids}})
	body := map[string]any{
		"size":  0,
		"query": map[string]any{"bool": map[string]any{"filter": filters}},
		"aggs": map[string]any{"by_trace": map[string]any{
			"terms": map[string]any{"field": "traceId", "size": len(ids)},
			"aggs": map[string]any{
				"spans":   map[string]any{"cardinality": map[string]any{"field": "spanId"}},
				"lastEnd": map[string]any{"max": map[string]any{"field": "endTime"}},
			},
		}},
	}
	var resp searchResponse
	if err := c.search(ctx, body, &resp); err != nil {
		return fmt.Errorf("trace stats: %w", err)
	}
	var aggs struct {
		ByTrace struct {
			Buckets []struct {
				Key   string `json:"key"`
				Spans struct {
					Value int `json:"value"`
				} `json:"spans"`
				LastEnd struct {
					Value *float64 `json:"value"`
				} `json:"lastEnd"`
			} `json:"buckets"`
		} `json:"by_trace"`
	}
	if len(resp.Aggregations) > 0 {
		if err := json.Unmarshal(resp.Aggregations, &aggs); err != nil {
			return fmt.Errorf("decode trace stats: %w", err)
		}
	}
	byID := make(map[string]int, len(traces))
	for i, t := range traces {
		byID[t.TraceID] = i
	}
	for _, b := range aggs.ByTrace.Buckets {
		i, ok := byID[b.Key]
		if !ok {
			continue
		}
		t := &traces[i]
		t.SpanCount = b.Spans.Value
		if b.LastEnd.Value != nil {
			if end := time.UnixMilli(int64(*b.LastEnd.Value)).UTC(); end.After(t.EndTime) {
				t.EndTime = end
				t.DurationNs = end.Sub(t.StartTime).Nanoseconds()
			}
		}
	}
	return nil
}

func (c *clientImpl) QueryTraceSpans(ctx context.Context, traceID string, req TracesQueryRequest) (*TraceSpansQueryResponse, error) {
	limit := defaultSpanLimit
	if req.Limit != nil && *req.Limit > 0 {
		limit = min(*req.Limit, maxResultWindow)
	}
	filters := append(scopeFilters(req.SearchScope), map[string]any{"term": map[string]any{"traceId": traceID}})
	body := map[string]any{
		"size":             limit,
		"track_total_hits": true,
		"query":            map[string]any{"bool": map[string]any{"filter": filters}},
		"sort":             []any{map[string]any{"startTime": sortOrder(req.SortOrder, "asc")}},
	}
	if !req.IncludeAttributes {
		body["_source"] = map[string]any{"excludes": []string{"attributes", "resource"}}
	}
	var resp searchResponse
	if err := c.search(ctx, body, &resp); err != nil {
		return nil, fmt.Errorf("observer.QueryTraceSpans: %w", err)
	}
	spans := make([]SpanInfo, 0, len(resp.Hits.Hits))
	seen := make(map[string]struct{}, len(resp.Hits.Hits))
	for _, h := range resp.Hits.Hits {
		d := h.Source
		// Retried exporter writes can duplicate a span document.
		if _, dup := seen[d.SpanID]; dup {
			continue
		}
		seen[d.SpanID] = struct{}{}
		spans = append(spans, SpanInfo{
			SpanID:             d.SpanID,
			SpanName:           d.Name,
			ParentSpanID:       d.ParentSpanID,
			StartTime:          d.StartTime,
			EndTime:            d.EndTime,
			DurationNs:         d.durationNs(),
			Kind:               d.Kind,
			Status:             d.Status,
			Attributes:         d.Attributes,
			ResourceAttributes: d.Resource,
		})
	}
	return &TraceSpansQueryResponse{Spans: spans, Total: max(resp.Hits.Total.Value, len(spans))}, nil
}

func (c *clientImpl) GetSpanDetails(ctx context.Context, traceID, spanID string) (*SpanDetailsResponse, error) {
	body := map[string]any{
		"size": 1,
		"query": map[string]any{"bool": map[string]any{"filter": []any{
			map[string]any{"term": map[string]any{"traceId": traceID}},
			map[string]any{"term": map[string]any{"spanId": spanID}},
		}}},
	}
	var resp searchResponse
	if err := c.search(ctx, body, &resp); err != nil {
		return nil, fmt.Errorf("observer.GetSpanDetails: %w", err)
	}
	if len(resp.Hits.Hits) == 0 {
		return nil, fmt.Errorf("observer.GetSpanDetails: trace %s span %s: %w", traceID, spanID, ErrSpanNotFound)
	}
	d := resp.Hits.Hits[0].Source
	return &SpanDetailsResponse{
		SpanID:             d.SpanID,
		SpanName:           d.Name,
		ParentSpanID:       d.ParentSpanID,
		StartTime:          d.StartTime,
		EndTime:            d.EndTime,
		DurationNs:         d.durationNs(),
		Kind:               d.Kind,
		Status:             d.Status,
		Attributes:         d.Attributes,
		ResourceAttributes: d.Resource,
	}, nil
}

func (c *clientImpl) search(ctx context.Context, body map[string]any, out *searchResponse) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal query: %w", err)
	}
	url := c.baseURL + "/" + c.index + "/_search?ignore_unavailable=true&allow_no_indices=true"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build search request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("execute search: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read search response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("search returned status %d: %s", resp.StatusCode, truncate(string(data), 512))
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode search response: %w", err)
	}
	return nil
}

// scopeFilters matches the organization, project and agent names the collector
// stamps on every span.
func scopeFilters(s TraceScope) []any {
	filters := make([]any, 0, 3)
	add := func(key, value string) {
		if value != "" {
			filters = append(filters, exactMatch("resource."+key, value))
		}
	}
	add(opensearch.ResourceOrganization, s.Organization)
	if s.Project != nil {
		add(opensearch.ResourceProject, *s.Project)
	}
	if s.Agent != nil {
		add(opensearch.ResourceAgent, *s.Agent)
	}
	return filters
}

// exactMatch matches a string field whether it is mapped as keyword or as
// text with a keyword subfield.
func exactMatch(field, value string) map[string]any {
	return map[string]any{"bool": map[string]any{
		"should": []any{
			map[string]any{"term": map[string]any{field: value}},
			map[string]any{"term": map[string]any{field + ".keyword": value}},
		},
		"minimum_should_match": 1,
	}}
}

// rootSpanFilter matches spans with an empty or absent parentSpanId.
func rootSpanFilter() map[string]any {
	return map[string]any{"bool": map[string]any{
		"should": []any{
			map[string]any{"term": map[string]any{"parentSpanId": ""}},
			map[string]any{"bool": map[string]any{"must_not": map[string]any{"exists": map[string]any{"field": "parentSpanId"}}}},
		},
		"minimum_should_match": 1,
	}}
}

func sortOrder(s *string, def string) string {
	if s != nil && (*s == "asc" || *s == "desc") {
		return *s
	}
	return def
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
