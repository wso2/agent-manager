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

package controllers

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"github.com/wso2-enterprise/agent-management-platform/traces-observer-service/opensearch"
)

// TracingController provides tracing functionality
type TracingController struct {
	osClient *opensearch.Client
}

// NewTracingController creates a new tracing service
func NewTracingController(osClient *opensearch.Client) *TracingController {
	return &TracingController{
		osClient: osClient,
	}
}

// GetTraceOverviews retrieves unique trace IDs with root span information
func (s *TracingController) GetTraceOverviews(ctx context.Context, params opensearch.TraceQueryParams) (*opensearch.TraceOverviewResponse, error) {
	log.Printf("Getting trace overviews for service: %s", params.ServiceName)

	// Set defaults
	if params.Limit == 0 {
		params.Limit = 10
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	// For trace overview, we need to fetch more spans to ensure we get complete traces
	// Multiply limit by a factor to get enough spans (each trace typically has multiple spans)
	originalLimit := params.Limit
	originalOffset := params.Offset
	params.Limit = params.Limit * 50 // Fetch more spans to capture complete traces
	params.Offset = 0                // Start from beginning for grouping

	// Use the existing BuildTraceQuery
	query := opensearch.BuildTraceQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search trace overviews: %w", err)
	}

	// Parse all spans
	spans := opensearch.ParseSpans(response)

	// Group spans by traceId and find root spans
	traceMap := make(map[string][]opensearch.Span)
	for _, span := range spans {
		traceMap[span.TraceID] = append(traceMap[span.TraceID], span)
	}

	// Process each trace to find root span
	allOverviews := []opensearch.TraceOverview{}
	for traceID, traceSpans := range traceMap {
		// Find root span (span with no parentSpanId)
		var rootSpanID, rootSpanName, startTime, endTime string
		var durationInNanos int64

		for _, span := range traceSpans {
			if span.ParentSpanID == "" {
				rootSpanID = span.SpanID
				rootSpanName = span.Name
				startTime = span.StartTime.Format(time.RFC3339Nano)
				endTime = span.EndTime.Format(time.RFC3339Nano)
				durationInNanos = span.DurationInNanos
				break
			}
		}

		// Add to overviews if we found a root span
		if rootSpanID != "" {
			allOverviews = append(allOverviews, opensearch.TraceOverview{
				TraceID:         traceID,
				RootSpanID:      rootSpanID,
				RootSpanName:    rootSpanName,
				StartTime:       startTime,
				EndTime:         endTime,
				DurationInNanos: durationInNanos,
				SpanCount:       len(traceSpans),
			})
		}
	}

	
	// Sort by StartTime (descending) for consistent pagination
	sort.Slice(allOverviews, func(i, j int) bool {
		return allOverviews[i].StartTime > allOverviews[j].StartTime
	})

	// Apply pagination to the trace overviews
	totalCount := len(allOverviews)
	start := originalOffset
	end := originalOffset + originalLimit

	if start >= len(allOverviews) {
		start = len(allOverviews)
	}
	if end > len(allOverviews) {
		end = len(allOverviews)
	}

	paginatedOverviews := allOverviews[start:end]

	log.Printf("Retrieved %d unique traces from %d spans (showing %d-%d of %d)",
		len(allOverviews), len(spans), start, end, totalCount)

	return &opensearch.TraceOverviewResponse{
		Traces:     paginatedOverviews,
		TotalCount: totalCount,
	}, nil
}

// GetTraceByIdAndService retrieves spans for a specific trace ID and service name
func (s *TracingController) GetTraceByIdAndService(ctx context.Context, params opensearch.TraceByIdAndServiceParams) (*opensearch.TraceResponse, error) {
	log.Printf("Getting trace for traceID: %s and service: %s", params.TraceID, params.ServiceName)

	// Build query
	query := opensearch.BuildTraceByIdAndServiceQuery(params)

	// Execute search
	response, err := s.osClient.Search(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search traces: %w", err)
	}

	// Parse spans
	spans := opensearch.ParseSpans(response)

	if len(spans) == 0 {
		return nil, fmt.Errorf("no spans found for traceID: %s and service: %s", params.TraceID, params.ServiceName)
	}

	log.Printf("Retrieved %d spans for traceID: %s and service: %s", len(spans), params.TraceID, params.ServiceName)

	return &opensearch.TraceResponse{
		Spans:      spans,
		TotalCount: len(spans),
	}, nil
}

// HealthCheck checks if the service is healthy
func (s *TracingController) HealthCheck(ctx context.Context) error {
	return s.osClient.HealthCheck(ctx)
}
