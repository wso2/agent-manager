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
package openchoreo

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// Reader reads traces from the OpenChoreo observer API.
type Reader interface {
	// QueryTraces lists the traces matching req.
	QueryTraces(ctx context.Context, req TracesQueryRequest) (*observer.TracesQueryResponse, error)
	// QueryTraceSpans lists the spans of one trace.
	QueryTraceSpans(ctx context.Context, traceID string, req TracesQueryRequest) (*observer.TraceSpansQueryResponse, error)
	// GetSpanDetails reads one span with its attributes.
	GetSpanDetails(ctx context.Context, traceID, spanID string) (*observer.SpanDetailsResponse, error)
	// NamespaceFor returns the namespace an organization's traces are in.
	NamespaceFor(organization string) string
}

// SearchScope is the namespace a query runs in, with optional filters.
type SearchScope struct {
	Namespace   string  `json:"namespace"`
	Project     *string `json:"project,omitempty"`
	Component   *string `json:"component,omitempty"`
	Environment *string `json:"environment,omitempty"`
}

// TracesQueryRequest is the body of a trace list or trace spans query.
type TracesQueryRequest struct {
	StartTime         time.Time   `json:"startTime"`
	EndTime           time.Time   `json:"endTime"`
	Limit             *int        `json:"limit,omitempty"`
	SortOrder         *string     `json:"sortOrder,omitempty"`
	SearchScope       SearchScope `json:"searchScope"`
	IncludeAttributes bool        `json:"includeAttributes,omitempty"`
}

// TokenProvider supplies the bearer token sent to the OpenChoreo observer.
type TokenProvider interface {
	GetToken(ctx context.Context) (string, error)
	InvalidateToken()
}

var _ TokenProvider = (*AuthProvider)(nil)

type client struct {
	baseURL          string
	authProvider     TokenProvider
	httpClient       *http.Client
	defaultNamespace string
}

// NewClient returns a Reader for the OpenChoreo observer at baseURL.
func NewClient(baseURL string, auth TokenProvider, defaultNamespace string) Reader {
	return &client{
		baseURL:          baseURL,
		authProvider:     auth,
		httpClient:       &http.Client{Timeout: 30 * time.Second},
		defaultNamespace: defaultNamespace,
	}
}

func (c *client) NamespaceFor(_ string) string {
	return c.defaultNamespace
}

func (c *client) QueryTraces(ctx context.Context, req TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	var result observer.TracesQueryResponse
	if err := c.doPost(ctx, "/api/v1alpha1/traces/query", req, &result); err != nil {
		return nil, fmt.Errorf("openchoreo.QueryTraces: %w", err)
	}
	return &result, nil
}

func (c *client) QueryTraceSpans(ctx context.Context, traceID string, req TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	var result observer.TraceSpansQueryResponse
	path := "/api/v1alpha1/traces/" + url.PathEscape(traceID) + "/spans/query"
	if err := c.doPost(ctx, path, req, &result); err != nil {
		return nil, fmt.Errorf("openchoreo.QueryTraceSpans: %w", err)
	}
	return &result, nil
}

func (c *client) GetSpanDetails(ctx context.Context, traceID, spanID string) (*observer.SpanDetailsResponse, error) {
	var result observer.SpanDetailsResponse
	path := "/api/v1alpha1/traces/" + url.PathEscape(traceID) + "/spans/" + url.PathEscape(spanID)
	if err := c.doGet(ctx, path, &result); err != nil {
		return nil, fmt.Errorf("openchoreo.GetSpanDetails: %w", err)
	}
	return &result, nil
}

func (c *client) doPost(ctx context.Context, path string, body any, out any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal request body: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+path, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("build POST request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return c.doWithAuth(ctx, req, out)
}

func (c *client) doGet(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build GET request: %w", err)
	}
	return c.doWithAuth(ctx, req, out)
}

// doWithAuth sends req with a bearer token, refreshing the token and retrying once on 401.
func (c *client) doWithAuth(ctx context.Context, req *http.Request, out any) error {
	for attempt := 1; attempt <= 2; attempt++ {
		token, err := c.authProvider.GetToken(ctx)
		if err != nil {
			return fmt.Errorf("get auth token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		if attempt > 1 && req.GetBody != nil {
			body, err := req.GetBody()
			if err != nil {
				return fmt.Errorf("re-read request body for retry: %w", err)
			}
			req.Body = body
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("execute request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return fmt.Errorf("read response body: %w", err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 1 {
			slog.Info("openchoreo client: received 401, invalidating token and retrying")
			c.authProvider.InvalidateToken()
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
		}

		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		return nil
	}
	return fmt.Errorf("request failed after token refresh")
}
