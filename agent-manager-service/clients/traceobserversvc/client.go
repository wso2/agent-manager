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
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/requests"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
)

// TraceObserverClient interface defines methods for interacting with the traces-observer-service
type TraceObserverClient interface {
	ListTraces(ctx context.Context, params ListTracesParams) (*TraceOverviewResponse, error)
	TraceDetailsById(ctx context.Context, params TraceDetailsByIdParams) (*TraceResponse, error)
}

type traceObserverClient struct {
	httpClient requests.HttpClient
}

// NewTraceObserverClient creates a new trace observer client
func NewTraceObserverClient() TraceObserverClient {
	httpClient := &http.Client{
		Timeout: time.Second * 15,
	}
	return &traceObserverClient{
		httpClient: httpClient,
	}
}

// ListTraces retrieves trace overviews from the traces-observer-service
func (c *traceObserverClient) ListTraces(ctx context.Context, params ListTracesParams) (*TraceOverviewResponse, error) {
	baseURL := config.GetConfig().TraceObserver.URL
	tracesURL := fmt.Sprintf("%s/api/traces", baseURL)

	// Build query parameters
	queryParams := url.Values{}
	queryParams.Set("serviceName", params.ServiceName)

	if params.StartTime != "" {
		queryParams.Set("startTime", params.StartTime)
	}
	if params.EndTime != "" {
		queryParams.Set("endTime", params.EndTime)
	}
	if params.Limit > 0 {
		queryParams.Set("limit", strconv.Itoa(params.Limit))
	}
	if params.Offset > 0 {
		queryParams.Set("offset", strconv.Itoa(params.Offset))
	}
	if params.SortOrder != "" {
		queryParams.Set("sortOrder", params.SortOrder)
	}

	fullURL := fmt.Sprintf("%s?%s", tracesURL, queryParams.Encode())

	req := &requests.HttpRequest{
		Name:   "traceobserver.ListTraces",
		URL:    fullURL,
		Method: http.MethodGet,
	}
	req.SetHeader("Accept", "application/json")

	var response TraceOverviewResponse
	if err := requests.SendRequest(ctx, c.httpClient, req).ScanResponse(&response, http.StatusOK); err != nil {
		return nil, fmt.Errorf("traceobserver.ListTraces: %w", err)
	}

	return &response, nil
}

// TraceDetailsById retrieves detailed trace spans for a specific trace ID from the traces-observer-service
func (c *traceObserverClient) TraceDetailsById(ctx context.Context, params TraceDetailsByIdParams) (*TraceResponse, error) {
	baseURL := config.GetConfig().TraceObserver.URL
	traceURL := fmt.Sprintf("%s/api/trace", baseURL)

	// Build query parameters
	queryParams := url.Values{}
	queryParams.Set("traceId", params.TraceID)
	queryParams.Set("serviceName", params.ServiceName)

	fullURL := fmt.Sprintf("%s?%s", traceURL, queryParams.Encode())

	req := &requests.HttpRequest{
		Name:   "traceobserver.TraceDetailsById",
		URL:    fullURL,
		Method: http.MethodGet,
	}
	req.SetHeader("Accept", "application/json")

	var response TraceResponse
	if err := requests.SendRequest(ctx, c.httpClient, req).ScanResponse(&response, http.StatusOK); err != nil {
		return nil, fmt.Errorf("traceobserver.TraceDetailsById: %w", err)
	}

	return &response, nil
}
