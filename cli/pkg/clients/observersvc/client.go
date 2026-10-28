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

package observersvc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type RequestEditorFn func(ctx context.Context, req *http.Request) error

type Client struct {
	baseURL    string
	httpClient *http.Client
	editor     RequestEditorFn
}

type Option func(*Client)

// WithHTTPClient sets the HTTP client the Client uses.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithRequestEditor adds fn to edit each request before it's sent.
func WithRequestEditor(fn RequestEditorFn) Option {
	return func(c *Client) { c.editor = fn }
}

// NewClient returns a Client for the observer at baseURL.
func NewClient(baseURL string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("observersvc: baseURL is required")
	}
	c := &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: http.DefaultClient,
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// URL returns the observer's base URL.
func (c *Client) URL() string { return c.baseURL }

// do sends a request and decodes the JSON response into out.
func (c *Client) do(ctx context.Context, method, path string, q url.Values, out any) error {
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	if c.editor != nil {
		if err := c.editor(ctx, req); err != nil {
			return err
		}
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		herr := &HTTPError{StatusCode: resp.StatusCode, RawBody: body}
		if ct := resp.Header.Get("Content-Type"); strings.Contains(ct, "application/json") {
			var er ErrorResponse
			if jerr := json.Unmarshal(body, &er); jerr == nil {
				herr.Body = &er
			}
		}
		return herr
	}
	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("observersvc: decode response: %w", err)
	}
	return nil
}

// setCommonParams sets the scope, paging and filter params ListTraces and ExportTraces share.
func setCommonParams(q url.Values, p *ListTracesParams) {
	if p.Organization != "" {
		q.Set("organization", p.Organization)
	}
	if p.Project != "" {
		q.Set("project", p.Project)
	}
	if p.Agent != "" {
		q.Set("agent", p.Agent)
	}
	if p.Environment != "" {
		q.Set("environment", p.Environment)
	}
	setTimeRange(q, p.StartTime, p.EndTime)
	if p.Limit != nil {
		q.Set("limit", strconv.Itoa(*p.Limit))
	}
	if p.SortOrder != nil && *p.SortOrder != "" {
		q.Set("sortOrder", *p.SortOrder)
	}
	if p.Status != "" {
		q.Set("status", p.Status)
	}
	setOptionalInt64(q, "minDurationMs", p.MinDurationMs)
	setOptionalInt64(q, "minTokens", p.MinTokens)
	setOptionalInt64(q, "minSpanCount", p.MinSpanCount)
	if p.Model != "" {
		q.Set("model", p.Model)
	}
	if p.ConversationID != "" {
		q.Set("conversationId", p.ConversationID)
	}
	if len(p.Missing) > 0 {
		q.Set("missing", strings.Join(p.Missing, ","))
	}
	if p.Tool != "" {
		q.Set("tool", p.Tool)
	}
	if p.ToolError {
		q.Set("toolError", "true")
	}
	if p.MCPServer != "" {
		q.Set("mcpServer", p.MCPServer)
	}
}

// setOptionalInt64 sets key only when val is non-nil.
func setOptionalInt64(q url.Values, key string, val *int64) {
	if val != nil {
		q.Set(key, strconv.FormatInt(*val, 10))
	}
}

// setTimeRange sets startTime and endTime on q when they're set.
func setTimeRange(q url.Values, start, end time.Time) {
	if !start.IsZero() {
		q.Set("startTime", start.Format(time.RFC3339))
	}
	if !end.IsZero() {
		q.Set("endTime", end.Format(time.RFC3339))
	}
}

// ListTraces lists trace overviews for p.
func (c *Client) ListTraces(ctx context.Context, p *ListTracesParams) (*TraceOverviewResponse, error) {
	q := url.Values{}
	setCommonParams(q, p)
	if include := includeValues(p); len(include) > 0 {
		q.Set("include", strings.Join(include, ","))
	}
	if p.Cursor != "" {
		q.Set("cursor", p.Cursor)
	}
	var out TraceOverviewResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/traces", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// includeValues lists the include values p asks for.
func includeValues(p *ListTracesParams) []string {
	var include []string
	if p.IncludeModels {
		include = append(include, "models")
	}
	if p.IncludeTools {
		include = append(include, "tools")
	}
	if p.IncludeMCPServers {
		include = append(include, "mcpServers")
	}
	return include
}

// ExportTraces exports full traces for p.
func (c *Client) ExportTraces(ctx context.Context, p *ExportTracesParams) (*TraceExportResponse, error) {
	q := url.Values{}
	setCommonParams(q, p)
	var out TraceExportResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/traces/export", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTraceSpans lists a trace's spans.
func (c *Client) GetTraceSpans(ctx context.Context, traceID string, p *GetTraceSpansParams) (*SpanListResponse, error) {
	q := url.Values{}
	if p.Organization != "" {
		q.Set("organization", p.Organization)
	}
	setOptionalString(q, "project", p.Project)
	setOptionalString(q, "agent", p.Agent)
	setOptionalString(q, "environment", p.Environment)
	setTimeRange(q, p.StartTime, p.EndTime)
	if p.Limit != nil {
		q.Set("limit", strconv.Itoa(*p.Limit))
	}
	setOptionalString(q, "sortOrder", p.SortOrder)
	var out SpanListResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/traces/"+url.PathEscape(traceID)+"/spans", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSpanDetail returns one span of a trace.
func (c *Client) GetSpanDetail(ctx context.Context, traceID, spanID string) (*Span, error) {
	var out Span
	path := "/api/v1/traces/" + url.PathEscape(traceID) + "/spans/" + url.PathEscape(spanID)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// setOptionalString sets key on q when val is set.
func setOptionalString(q url.Values, key string, val *string) {
	if val != nil && *val != "" {
		q.Set(key, *val)
	}
}

// GetRuntimeLogs returns an agent's runtime logs.
func (c *Client) GetRuntimeLogs(ctx context.Context, p RuntimeLogsParams) (*LogsResponse, error) {
	q := url.Values{}
	q.Set("organization", p.Organization)
	q.Set("project", p.Project)
	q.Set("agent", p.Agent)
	q.Set("environment", p.Environment)
	if p.StartTime != "" {
		q.Set("startTime", p.StartTime)
	}
	if p.EndTime != "" {
		q.Set("endTime", p.EndTime)
	}
	if p.SearchPhrase != "" {
		q.Set("searchPhrase", p.SearchPhrase)
	}
	if len(p.LogLevels) > 0 {
		q.Set("logLevels", strings.Join(p.LogLevels, ","))
	}
	if p.Limit != nil {
		q.Set("limit", strconv.Itoa(*p.Limit))
	}
	if p.SortOrder != "" {
		q.Set("sortOrder", p.SortOrder)
	}
	var out LogsResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/logs", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetBuildLogs returns a build's logs.
func (c *Client) GetBuildLogs(ctx context.Context, p BuildLogsParams) (*LogsResponse, error) {
	q := url.Values{}
	q.Set("organization", p.Organization)
	q.Set("buildName", p.BuildName)
	var out LogsResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/build-logs", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetMetrics returns an agent's resource metrics.
func (c *Client) GetMetrics(ctx context.Context, p MetricsParams) (*MetricsResponse, error) {
	q := url.Values{}
	q.Set("organization", p.Organization)
	q.Set("project", p.Project)
	q.Set("agent", p.Agent)
	q.Set("environment", p.Environment)
	if p.StartTime != "" {
		q.Set("startTime", p.StartTime)
	}
	if p.EndTime != "" {
		q.Set("endTime", p.EndTime)
	}
	var out MetricsResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/metrics", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
