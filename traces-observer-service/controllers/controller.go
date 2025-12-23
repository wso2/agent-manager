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
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/wso2/ai-agent-management-platform/traces-observer-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/opensearch"
)

// ErrTraceNotFound is returned when a trace is not found
var ErrTraceNotFound = errors.New("trace not found")

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
	log := logger.GetLogger(ctx)
	log.Info("Getting trace overviews",
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid, "startTime", params.StartTime, "endTime", params.EndTime)

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

	// Generate indices based on time range
	indices, err := opensearch.GetIndicesForTimeRange(params.StartTime, params.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}
	log.Debug("Searching indices", "indices", indices)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
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
		var rootSpan *opensearch.Span

		for i := range traceSpans {
			if traceSpans[i].ParentSpanID == "" {
				rootSpan = &traceSpans[i]
				break
			}
		}

		// Skip this trace if no root span found
		if rootSpan == nil {
			logger.GetLogger(ctx).Warn("No root span found for trace", "traceId", traceID)
			continue
		}

		// Extract token usage from GenAI spans
		tokenUsage := opensearch.ExtractTokenUsage(traceSpans)

		// Extract trace status and error information
		traceStatus := opensearch.ExtractTraceStatus(traceSpans)

		// Extract input and output from root span
		// Check if this is a CrewAI workflow span and delegate to CrewAI processor
		var input, output interface{}
		if opensearch.IsCrewAISpan(rootSpan.Attributes) {
			input, output = opensearch.ExtractCrewAIRootSpanInputOutput(rootSpan)
		} else {
			input, output = opensearch.ExtractRootSpanInputOutput(rootSpan)
		}

		// Add to overviews
		allOverviews = append(allOverviews, opensearch.TraceOverview{
			TraceID:         traceID,
			RootSpanID:      rootSpan.SpanID,
			RootSpanName:    rootSpan.Name,
			RootSpanKind:    string(opensearch.DetermineSpanType(*rootSpan)),
			StartTime:       rootSpan.StartTime.Format(time.RFC3339Nano),
			EndTime:         rootSpan.EndTime.Format(time.RFC3339Nano),
			DurationInNanos: rootSpan.DurationInNanos,
			SpanCount:       len(traceSpans),
			TokenUsage:      tokenUsage,
			Status:          traceStatus,
			Input:           input,
			Output:          output,
		})
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

	log.Info("Retrieved trace overviews",
		"unique_traces", len(allOverviews),
		"total_spans", len(spans),
		"showing_start", start,
		"showing_end", end,
		"total_count", totalCount)

	return &opensearch.TraceOverviewResponse{
		Traces:     paginatedOverviews,
		TotalCount: totalCount,
	}, nil
}

// GetTraceByIdAndService retrieves spans for a specific trace ID and component UID
func (s *TracingController) GetTraceByIdAndService(ctx context.Context, params opensearch.TraceByIdAndServiceParams) (*opensearch.TraceResponse, error) {
	log := logger.GetLogger(ctx)
	log.Info("Getting trace by ID",
		"traceId", params.TraceID,
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid)

	// Build query
	query := opensearch.BuildTraceByIdAndServiceQuery(params)

	// For trace by ID queries, we need to search across a broader time range
	// Use current day and previous 7 days as default
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -7)
	indices, err := opensearch.GetIndicesForTimeRange(
		startTime.Format(time.RFC3339),
		endTime.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}
	log.Debug("Searching indices for trace ID", "indices", indices)

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search traces: %w", err)
	}

	// Parse spans
	spans := opensearch.ParseSpans(response)

	if len(spans) == 0 {
		log.Warn("No spans found for trace",
			"traceId", params.TraceID,
			"component", params.ComponentUid,
			"environment", params.EnvironmentUid)
		return nil, ErrTraceNotFound
	}

	// Extract token usage from GenAI spans
	tokenUsage := opensearch.ExtractTokenUsage(spans)

	// Extract trace status and error information
	traceStatus := opensearch.ExtractTraceStatus(spans)

	log.Info("Retrieved trace spans",
		"span_count", len(spans),
		"traceId", params.TraceID,
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid)

	return &opensearch.TraceResponse{
		Spans:      spans,
		TotalCount: len(spans),
		TokenUsage: tokenUsage,
		Status:     traceStatus,
	}, nil
}

// HealthCheck checks if the service is healthy
func (s *TracingController) HealthCheck(ctx context.Context) error {
	return s.osClient.HealthCheck(ctx)
}
