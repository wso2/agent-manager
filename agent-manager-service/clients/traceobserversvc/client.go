// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package traceobserversvc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
)

// TraceObserverClient is the interface for interacting with the trace observer service
type TraceObserverClient interface {
	ListTraces(ctx context.Context, params ListTracesParams) (*TraceOverviewResponse, error)
	TraceDetailsById(ctx context.Context, params TraceDetailsByIdParams) (*TraceResponse, error)
}

type traceObserverClient struct {
	baseURL    string
	httpClient *http.Client
}

// NewTraceObserverClient creates a new TraceObserverClient instance
func NewTraceObserverClient() TraceObserverClient {
	cfg := config.GetConfig()
	return &traceObserverClient{
		baseURL: cfg.TraceObserver.URL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// ListTraces retrieves trace overviews from the trace observer service
func (c *traceObserverClient) ListTraces(ctx context.Context, params ListTracesParams) (*TraceOverviewResponse, error) {
	// Build query parameters
	queryParams := url.Values{}
	queryParams.Add("componentUid", params.ComponentUid)
	if params.EnvironmentUid != "" {
		queryParams.Add("environmentUid", params.EnvironmentUid)
	}
	if params.StartTime != "" {
		queryParams.Add("startTime", params.StartTime)
	}
	if params.EndTime != "" {
		queryParams.Add("endTime", params.EndTime)
	}
	queryParams.Add("limit", strconv.Itoa(params.Limit))
	queryParams.Add("offset", strconv.Itoa(params.Offset))
	queryParams.Add("sortOrder", params.SortOrder)

	// Build URL - endpoint is /api/v1/traces
	requestURL := fmt.Sprintf("%s/api/v1/traces?%s", c.baseURL, queryParams.Encode())

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("trace observer returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response TraceOverviewResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}

// TraceDetailsById retrieves detailed trace information by trace ID
func (c *traceObserverClient) TraceDetailsById(ctx context.Context, params TraceDetailsByIdParams) (*TraceResponse, error) {
	// Build query parameters - traceId is also a query param, not path param
	queryParams := url.Values{}
	queryParams.Add("traceId", params.TraceID)
	queryParams.Add("componentUid", params.ComponentUid)
	if params.EnvironmentUid != "" {
		queryParams.Add("environmentUid", params.EnvironmentUid)
	}

	// Build URL - endpoint is /api/v1/trace (singular, not plural)
	requestURL := fmt.Sprintf("%s/api/v1/trace?%s", c.baseURL, queryParams.Encode())

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("trace observer returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response TraceResponse
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &response, nil
}
