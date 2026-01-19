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

const (
	// MaxSpansPerRequest is the maximum number of spans that can be fetched in a single query
	MaxSpansPerRequest = 10000
	// MaxTracesPerRequest is the maximum number of traces that can be requested at once
	MaxTracesPerRequest = 1000
	// DefaultTracesLimit is the default number of traces to return when no limit is specified
	DefaultTracesLimit = 10
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

// retrieveAndGroupTraces is a shared helper that fetches spans and groups them into traces
func (s *TracingController) retrieveAndGroupTraces(ctx context.Context, params opensearch.TraceQueryParams) ([]map[string]interface{}, int, error) {
	log := logger.GetLogger(ctx)

	// Set defaults for limit and offset
	if params.Limit == 0 {
		params.Limit = DefaultTracesLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	// Store original pagination params before modifying
	originalLimit := params.Limit
	originalOffset := params.Offset

	// Fetch spans with multiplier to ensure we get complete traces
	params.Limit = originalLimit * 100
	if params.Limit > MaxSpansPerRequest {
		params.Limit = MaxSpansPerRequest
	}
	params.Offset = 0

	log.Debug("Fetching spans for traces",
		"originalLimit", originalLimit,
		"originalOffset", originalOffset,
		"spanFetchLimit", params.Limit)

	// Build query
	query := opensearch.BuildTraceQuery(params)
	log.Info("Built query", "query", query)

	// Generate indices based on time range
	indices, err := opensearch.GetIndicesForTimeRange(params.StartTime, params.EndTime)
	if err != nil {
		log.Error("Failed to generate indices for time range",
			"startTime", params.StartTime,
			"endTime", params.EndTime,
			"error", err)
		return nil, 0, fmt.Errorf("failed to generate indices: %w", err)
	}
	log.Debug("Searching indices", "indices", indices, "indexCount", len(indices))

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		log.Error("OpenSearch query failed",
			"indices", indices,
			"component", params.ComponentUid,
			"environment", params.EnvironmentUid,
			"error", err)
		return nil, 0, fmt.Errorf("failed to search traces: %w", err)
	}

	// Parse all spans
	spans := opensearch.ParseSpans(response)
	log.Debug("Parsed spans from OpenSearch", "spanCount", len(spans))

	if len(spans) == 0 {
		log.Warn("No spans found for query",
			"component", params.ComponentUid,
			"environment", params.EnvironmentUid,
			"startTime", params.StartTime,
			"endTime", params.EndTime)
		return []map[string]interface{}{}, 0, nil
	}

	// Group spans by traceId
	traceMap := make(map[string][]opensearch.Span)
	for _, span := range spans {
		traceMap[span.TraceID] = append(traceMap[span.TraceID], span)
	}
	log.Debug("Grouped spans into traces", "uniqueTraceCount", len(traceMap))

	// Process each trace to extract metadata
	allTraces := []map[string]interface{}{}
	skippedTraces := 0
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
			log.Warn("No root span found for trace, skipping",
				"traceId", traceID,
				"spanCount", len(traceSpans))
			skippedTraces++
			continue
		}

		// Extract token usage from GenAI spans
		tokenUsage := opensearch.ExtractTokenUsage(traceSpans)

		// Extract trace status and error information
		traceStatus := opensearch.ExtractTraceStatus(traceSpans)

		// Extract input and output from root span
		var input, output interface{}
		if opensearch.IsCrewAISpan(rootSpan.Attributes) {
			input, output = opensearch.ExtractCrewAIRootSpanInputOutput(rootSpan)
		} else {
			input, output = opensearch.ExtractRootSpanInputOutput(rootSpan)
		}

		// Store trace data as a map with all necessary fields
		traceData := map[string]interface{}{
			"traceID":         traceID,
			"rootSpanID":      rootSpan.SpanID,
			"spans":           traceSpans,
			"tokenUsage":      tokenUsage,
			"status":          traceStatus,
			"input":           input,
			"output":          output,
			"rootSpanName":    rootSpan.Name,
			"rootSpanKind":    string(opensearch.DetermineSpanType(*rootSpan)),
			"startTime":       rootSpan.StartTime.Format(time.RFC3339Nano),
			"endTime":         rootSpan.EndTime.Format(time.RFC3339Nano),
			"durationInNanos": rootSpan.DurationInNanos,
			"spanCount":       len(traceSpans),
			"originalLimit":   originalLimit,
			"originalOffset":  originalOffset,
		}

		allTraces = append(allTraces, traceData)
	}

	if skippedTraces > 0 {
		log.Warn("Skipped traces due to missing root spans",
			"skippedCount", skippedTraces,
			"totalTraces", len(traceMap))
	}

	// Sort by StartTime (descending) for consistent pagination
	sort.Slice(allTraces, func(i, j int) bool {
		return allTraces[i]["startTime"].(string) > allTraces[j]["startTime"].(string)
	})

	log.Info("Successfully retrieved and grouped traces",
		"uniqueTraces", len(allTraces),
		"totalSpans", len(spans),
		"skippedTraces", skippedTraces,
		"requestedLimit", originalLimit,
		"requestedOffset", originalOffset)

	return allTraces, len(spans), nil
}

// GetTraceOverviews retrieves unique trace IDs with root span information
func (s *TracingController) GetTraceOverviews(ctx context.Context, params opensearch.TraceQueryParams) (*opensearch.TraceOverviewResponse, error) {
	log := logger.GetLogger(ctx)
	log.Info("Getting trace overviews",
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid,
		"startTime", params.StartTime,
		"endTime", params.EndTime)

	// Retrieve and group traces using shared function
	// Use 100x multiplier to ensure we discover all traces
	allTraces, totalSpans, err := s.retrieveAndGroupTraces(ctx, params)
	if err != nil {
		return nil, err
	}

	// Convert trace data maps to TraceOverview structs
	allOverviews := make([]opensearch.TraceOverview, 0, len(allTraces))
	for _, traceData := range allTraces {
		allOverviews = append(allOverviews, opensearch.TraceOverview{
			TraceID:         traceData["traceID"].(string),
			RootSpanID:      traceData["rootSpanID"].(string), // Use stored ID instead of pointer
			RootSpanName:    traceData["rootSpanName"].(string),
			RootSpanKind:    traceData["rootSpanKind"].(string),
			StartTime:       traceData["startTime"].(string),
			EndTime:         traceData["endTime"].(string),
			DurationInNanos: traceData["durationInNanos"].(int64),
			SpanCount:       traceData["spanCount"].(int),
			TokenUsage:      traceData["tokenUsage"].(*opensearch.TokenUsage),
			Status:          traceData["status"].(*opensearch.TraceStatus),
			Input:           traceData["input"],
			Output:          traceData["output"],
		})
	}

	// Apply pagination to the trace overviews
	totalCount := len(allOverviews)

	// Get pagination params from first trace (they're all the same)
	var originalLimit, originalOffset int
	if len(allTraces) > 0 {
		originalLimit = allTraces[0]["originalLimit"].(int)
		originalOffset = allTraces[0]["originalOffset"].(int)
	}

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
		"total_spans", totalSpans,
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

// ExportTraces retrieves complete trace objects with all spans for export
func (s *TracingController) ExportTraces(ctx context.Context, params opensearch.TraceQueryParams) (*opensearch.TraceExportResponse, error) {
	log := logger.GetLogger(ctx)
	log.Info("Starting trace export",
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid,
		"startTime", params.StartTime,
		"endTime", params.EndTime)

	// For export, ignore pagination params and set a high limit to get all traces
	// Cap at MaxTracesPerRequest for safety to prevent overwhelming the system
	params.Limit = MaxTracesPerRequest
	params.Offset = 0

	// Retrieve and group traces using shared function
	allTraces, totalSpans, err := s.retrieveAndGroupTraces(ctx, params)
	if err != nil {
		log.Error("Failed to retrieve traces for export",
			"component", params.ComponentUid,
			"environment", params.EnvironmentUid,
			"error", err)
		return nil, err
	}

	if len(allTraces) == 0 {
		log.Warn("No traces found to export",
			"component", params.ComponentUid,
			"environment", params.EnvironmentUid,
			"startTime", params.StartTime,
			"endTime", params.EndTime)
	}

	// Convert trace data maps to FullTrace structs
	fullTraces := make([]opensearch.FullTrace, 0, len(allTraces))
	for _, traceData := range allTraces {
		// Get spans and sort by start time for consistent ordering
		traceSpans := traceData["spans"].([]opensearch.Span)
		sort.Slice(traceSpans, func(i, j int) bool {
			return traceSpans[i].StartTime.Before(traceSpans[j].StartTime)
		})

		fullTraces = append(fullTraces, opensearch.FullTrace{
			TraceID:         traceData["traceID"].(string),
			RootSpanID:      traceData["rootSpanID"].(string),
			RootSpanName:    traceData["rootSpanName"].(string),
			RootSpanKind:    traceData["rootSpanKind"].(string),
			StartTime:       traceData["startTime"].(string),
			EndTime:         traceData["endTime"].(string),
			DurationInNanos: traceData["durationInNanos"].(int64),
			SpanCount:       traceData["spanCount"].(int),
			TokenUsage:      traceData["tokenUsage"].(*opensearch.TokenUsage),
			Status:          traceData["status"].(*opensearch.TraceStatus),
			Input:           traceData["input"],
			Output:          traceData["output"],
			Spans:           traceSpans, // Include all spans with full details
		})
	}

	log.Info("Successfully completed trace export",
		"exportedTraces", len(fullTraces),
		"totalSpans", totalSpans,
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid)

	return &opensearch.TraceExportResponse{
		Traces:     fullTraces, // Return ALL traces, no pagination
		TotalCount: len(fullTraces),
	}, nil
}

// HealthCheck checks if the service is healthy
func (s *TracingController) HealthCheck(ctx context.Context) error {
	return s.osClient.HealthCheck(ctx)
}
