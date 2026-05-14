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

package traceobssvc

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

// RequestEditorFn lets callers mutate each outgoing request (e.g. to inject
// an Authorization header).
type RequestEditorFn func(ctx context.Context, req *http.Request) error

// Client is a small HTTP client for the traces-observer-service.
type Client struct {
	baseURL    string
	httpClient *http.Client
	editor     RequestEditorFn
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient overrides the default *http.Client.
func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.httpClient = h
		}
	}
}

// WithRequestEditor registers a single request editor invoked before every
// request is sent.
func WithRequestEditor(fn RequestEditorFn) Option {
	return func(c *Client) { c.editor = fn }
}

// NewClient returns a Client rooted at baseURL.
func NewClient(baseURL string, opts ...Option) (*Client, error) {
	if baseURL == "" {
		return nil, fmt.Errorf("traceobssvc: baseURL is required")
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

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
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
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("traceobssvc: decode response: %w", err)
	}
	return nil
}

func addCommon(q url.Values, organization, project, agent, environment string, startTime, endTime time.Time, limit *int, sortOrder *string) {
	if organization != "" {
		q.Set("organization", organization)
	}
	if project != "" {
		q.Set("project", project)
	}
	if agent != "" {
		q.Set("agent", agent)
	}
	if environment != "" {
		q.Set("environment", environment)
	}
	if !startTime.IsZero() {
		q.Set("startTime", startTime.Format(time.RFC3339))
	}
	if !endTime.IsZero() {
		q.Set("endTime", endTime.Format(time.RFC3339))
	}
	if limit != nil {
		q.Set("limit", strconv.Itoa(*limit))
	}
	if sortOrder != nil && *sortOrder != "" {
		q.Set("sortOrder", *sortOrder)
	}
}

// ListTraces calls GET /api/v1/traces.
func (c *Client) ListTraces(ctx context.Context, p *ListTracesParams) (*TraceOverviewResponse, error) {
	q := url.Values{}
	addCommon(q, p.Organization, p.Project, p.Agent, p.Environment, p.StartTime, p.EndTime, p.Limit, p.SortOrder)
	var out TraceOverviewResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/traces", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ExportTraces calls GET /api/v1/traces/export.
func (c *Client) ExportTraces(ctx context.Context, p *ExportTracesParams) (*TraceExportResponse, error) {
	q := url.Values{}
	addCommon(q, p.Organization, p.Project, p.Agent, p.Environment, p.StartTime, p.EndTime, p.Limit, p.SortOrder)
	var out TraceExportResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/traces/export", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetTraceSpans calls GET /api/v1/traces/{traceId}/spans.
func (c *Client) GetTraceSpans(ctx context.Context, traceID string, p *GetTraceSpansParams) (*SpanListResponse, error) {
	q := url.Values{}
	if p.Organization != "" {
		q.Set("organization", p.Organization)
	}
	if p.Project != nil && *p.Project != "" {
		q.Set("project", *p.Project)
	}
	if p.Agent != nil && *p.Agent != "" {
		q.Set("agent", *p.Agent)
	}
	if p.Environment != nil && *p.Environment != "" {
		q.Set("environment", *p.Environment)
	}
	if !p.StartTime.IsZero() {
		q.Set("startTime", p.StartTime.Format(time.RFC3339))
	}
	if !p.EndTime.IsZero() {
		q.Set("endTime", p.EndTime.Format(time.RFC3339))
	}
	if p.Limit != nil {
		q.Set("limit", strconv.Itoa(*p.Limit))
	}
	if p.SortOrder != nil && *p.SortOrder != "" {
		q.Set("sortOrder", *p.SortOrder)
	}
	var out SpanListResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/traces/"+url.PathEscape(traceID)+"/spans", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetSpanDetail calls GET /api/v1/traces/{traceId}/spans/{spanId}.
func (c *Client) GetSpanDetail(ctx context.Context, traceID, spanID string) (*Span, error) {
	var out Span
	path := "/api/v1/traces/" + url.PathEscape(traceID) + "/spans/" + url.PathEscape(spanID)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
