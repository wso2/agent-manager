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

package observer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// Client is the interface for querying trace data from the observer service.
type Client interface {
	// QueryTraces fetches a paginated list of traces matching the given request.
	QueryTraces(ctx context.Context, req TracesQueryRequest) (*TracesQueryResponse, error)

	// QueryTraceSpans fetches span summaries for a specific trace.
	QueryTraceSpans(ctx context.Context, traceID string, req TracesQueryRequest) (*TraceSpansQueryResponse, error)

	// GetSpanDetails fetches the full details (including OTEL attributes) for a single span.
	GetSpanDetails(ctx context.Context, traceID, spanID string) (*SpanDetailsResponse, error)
}

type clientImpl struct {
	baseURL      string
	authProvider *AuthProvider
	httpClient   *http.Client
}

// NewClient creates a new observer service client.
func NewClient(baseURL string, auth *AuthProvider) Client {
	return &clientImpl{
		baseURL:      baseURL,
		authProvider: auth,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *clientImpl) QueryTraces(ctx context.Context, req TracesQueryRequest) (*TracesQueryResponse, error) {
	var result TracesQueryResponse
	if err := c.doPost(ctx, "/api/v1alpha1/traces/query", req, &result); err != nil {
		return nil, fmt.Errorf("observer.QueryTraces: %w", err)
	}
	return &result, nil
}

func (c *clientImpl) QueryTraceSpans(ctx context.Context, traceID string, req TracesQueryRequest) (*TraceSpansQueryResponse, error) {
	var result TraceSpansQueryResponse
	path := fmt.Sprintf("/api/v1alpha1/traces/%s/spans/query", traceID)
	if err := c.doPost(ctx, path, req, &result); err != nil {
		return nil, fmt.Errorf("observer.QueryTraceSpans: %w", err)
	}
	return &result, nil
}

func (c *clientImpl) GetSpanDetails(ctx context.Context, traceID, spanID string) (*SpanDetailsResponse, error) {
	var result SpanDetailsResponse
	path := fmt.Sprintf("/api/v1alpha1/traces/%s/spans/%s", traceID, spanID)
	if err := c.doGet(ctx, path, &result); err != nil {
		return nil, fmt.Errorf("observer.GetSpanDetails: %w", err)
	}
	return &result, nil
}

func (c *clientImpl) doPost(ctx context.Context, path string, body any, out any) error {
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

func (c *clientImpl) doGet(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return fmt.Errorf("build GET request: %w", err)
	}
	return c.doWithAuth(ctx, req, out)
}

// doWithAuth executes the request with a Bearer token. On a 401 response it
// invalidates the cached token and retries once.
func (c *clientImpl) doWithAuth(ctx context.Context, req *http.Request, out any) error {
	for attempt := 1; attempt <= 2; attempt++ {
		token, err := c.authProvider.GetToken(ctx)
		if err != nil {
			return fmt.Errorf("get auth token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)

		// Clone the body reader for retry (POST bodies need to be re-readable).
		// For GET requests the body is nil so this is a no-op.
		if attempt > 1 {
			if req.GetBody != nil {
				body, err := req.GetBody()
				if err != nil {
					return fmt.Errorf("re-read request body for retry: %w", err)
				}
				req.Body = body
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("execute request: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return fmt.Errorf("read response body: %w", err)
		}

		if resp.StatusCode == http.StatusUnauthorized && attempt == 1 {
			slog.Info("observer client: received 401, invalidating token and retrying")
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
