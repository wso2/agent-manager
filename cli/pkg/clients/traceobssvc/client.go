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

type RequestEditorFn func(ctx context.Context, req *http.Request) error

type Client struct {
	baseURL    string
	httpClient *http.Client
	editor     RequestEditorFn
}

type Option func(*Client)

func WithHTTPClient(h *http.Client) Option {
	return func(c *Client) {
		if h != nil {
			c.httpClient = h
		}
	}
}

func WithRequestEditor(fn RequestEditorFn) Option {
	return func(c *Client) { c.editor = fn }
}

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

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(resp.Body)
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
		return fmt.Errorf("traceobssvc: decode response: %w", err)
	}
	return nil
}

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
}

func setTimeRange(q url.Values, start, end time.Time) {
	if !start.IsZero() {
		q.Set("startTime", start.Format(time.RFC3339))
	}
	if !end.IsZero() {
		q.Set("endTime", end.Format(time.RFC3339))
	}
}

func (c *Client) ListTraces(ctx context.Context, p *ListTracesParams) (*TraceOverviewResponse, error) {
	q := url.Values{}
	setCommonParams(q, p)
	var out TraceOverviewResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/traces", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) ExportTraces(ctx context.Context, p *ExportTracesParams) (*TraceExportResponse, error) {
	q := url.Values{}
	setCommonParams(q, p)
	var out TraceExportResponse
	if err := c.do(ctx, http.MethodGet, "/api/v1/traces/export", q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

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

func (c *Client) GetSpanDetail(ctx context.Context, traceID, spanID string) (*Span, error) {
	var out Span
	path := "/api/v1/traces/" + url.PathEscape(traceID) + "/spans/" + url.PathEscape(spanID)
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func setOptionalString(q url.Values, key string, val *string) {
	if val != nil && *val != "" {
		q.Set(key, *val)
	}
}
