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

// GetTraceByIdV2 retrieves spans for a specific trace.
// When params.ParentSpan is true, only the root span (parentSpanId == "") is returned.
func (s *TracingController) GetTraceByIdV2(ctx context.Context, params opensearch.V2TraceByIdParams) (*opensearch.TraceResponse, error) {
	log := logger.GetLogger(ctx)
	log.Info("Getting trace by ID (v2)",
		"traceIds", params.TraceIDs,
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid,
		"parentSpan", params.ParentSpan)

	// Build query
	query := opensearch.BuildV2TraceByIdsQuery(params)

	// Search across last 7 days
	endTime := time.Now()
	startTime := endTime.AddDate(0, 0, -7)
	indices, err := opensearch.GetIndicesForTimeRange(
		startTime.Format(time.RFC3339),
		endTime.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search traces: %w", err)
	}

	// Parse spans
	spans := opensearch.ParseSpans(response)

	if len(spans) == 0 {
		log.Warn("No spans found for trace (v2)",
			"traceIds", params.TraceIDs,
			"component", params.ComponentUid,
			"environment", params.EnvironmentUid)
		return nil, ErrTraceNotFound
	}

	// Extract token usage and status from returned spans
	tokenUsage := opensearch.ExtractTokenUsage(spans)
	traceStatus := opensearch.ExtractTraceStatus(spans)

	log.Info("Retrieved trace spans (v2)",
		"span_count", len(spans),
		"traceIds", params.TraceIDs,
		"parentSpan", params.ParentSpan)

	return &opensearch.TraceResponse{
		Spans:      spans,
		TotalCount: len(spans),
		TokenUsage: tokenUsage,
		Status:     traceStatus,
	}, nil
}

// GetTraceOverviewsV2 retrieves trace overviews using OpenSearch aggregations for
// trace-level grouping and pagination, then enriches with root span data.
func (s *TracingController) GetTraceOverviewsV2(ctx context.Context, params opensearch.TraceQueryParams) (*opensearch.TraceOverviewResponse, error) {
	log := logger.GetLogger(ctx)
	log.Info("Getting trace overviews (v2)",
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid,
		"startTime", params.StartTime,
		"endTime", params.EndTime,
		"limit", params.Limit,
		"offset", params.Offset)

	// Set defaults
	if params.Limit == 0 {
		params.Limit = DefaultTracesLimit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	// Phase 1: Aggregation to discover trace IDs with pagination
	aggQuery := opensearch.BuildTraceAggregationQuery(params)

	indices, err := opensearch.GetIndicesForTimeRange(params.StartTime, params.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	aggResponse, err := s.osClient.SearchWithAggregation(ctx, indices, aggQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute aggregation: %w", err)
	}

	totalCount := aggResponse.Aggregations.TotalTraces.Value
	buckets := aggResponse.Aggregations.Traces.Buckets

	// Apply pagination: skip first `offset` buckets, take `limit`
	start := params.Offset
	end := params.Offset + params.Limit
	if start >= len(buckets) {
		return &opensearch.TraceOverviewResponse{
			Traces:     []opensearch.TraceOverview{},
			TotalCount: totalCount,
		}, nil
	}
	if end > len(buckets) {
		end = len(buckets)
	}
	paginatedBuckets := buckets[start:end]

	if len(paginatedBuckets) == 0 {
		return &opensearch.TraceOverviewResponse{
			Traces:     []opensearch.TraceOverview{},
			TotalCount: totalCount,
		}, nil
	}

	// Collect trace IDs and span counts from aggregation
	traceIDs := make([]string, len(paginatedBuckets))
	spanCountMap := make(map[string]int, len(paginatedBuckets))
	for i, bucket := range paginatedBuckets {
		traceIDs[i] = bucket.Key
		spanCountMap[bucket.Key] = bucket.DocCount
	}

	// Phase 2: Fetch root spans for enrichment
	rootSpanParams := opensearch.V2TraceByIdParams{
		TraceIDs:       traceIDs,
		ComponentUid:   params.ComponentUid,
		EnvironmentUid: params.EnvironmentUid,
		ParentSpan:     true,
		Limit:          len(traceIDs), // One root span per trace
	}

	rootSpanQuery := opensearch.BuildV2TraceByIdsQuery(rootSpanParams)
	rootSpanResponse, err := s.osClient.Search(ctx, indices, rootSpanQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch root spans: %w", err)
	}

	rootSpans := opensearch.ParseSpans(rootSpanResponse)

	// Index root spans by traceId
	rootSpanMap := make(map[string]*opensearch.Span, len(rootSpans))
	for i := range rootSpans {
		rootSpanMap[rootSpans[i].TraceID] = &rootSpans[i]
	}

	// Build trace overviews in aggregation order (preserves sort)
	overviews := make([]opensearch.TraceOverview, 0, len(paginatedBuckets))
	for _, bucket := range paginatedBuckets {
		rootSpan, hasRoot := rootSpanMap[bucket.Key]
		if !hasRoot {
			log.Warn("No root span found for trace, skipping",
				"traceId", bucket.Key)
			continue
		}

		// Extract input/output from root span
		var input, output interface{}
		if opensearch.IsCrewAISpan(rootSpan.Attributes) {
			input, output = opensearch.ExtractCrewAIRootSpanInputOutput(rootSpan)
		} else {
			input, output = opensearch.ExtractRootSpanInputOutput(rootSpan)
		}

		// Extract token usage from root span's traceloop.entity.output,
		// falling back to gen_ai.usage.* attributes
		tokenUsage := opensearch.ExtractTokenUsageFromEntityOutput(rootSpan)
		if tokenUsage == nil {
			tokenUsage = opensearch.ExtractTokenUsage([]opensearch.Span{*rootSpan})
		}
		traceStatus := opensearch.ExtractTraceStatus([]opensearch.Span{*rootSpan})

		overviews = append(overviews, opensearch.TraceOverview{
			TraceID:         bucket.Key,
			RootSpanID:      rootSpan.SpanID,
			RootSpanName:    rootSpan.Name,
			RootSpanKind:    string(opensearch.DetermineSpanType(*rootSpan)),
			StartTime:       rootSpan.StartTime.Format(time.RFC3339Nano),
			EndTime:         rootSpan.EndTime.Format(time.RFC3339Nano),
			DurationInNanos: rootSpan.DurationInNanos,
			SpanCount:       spanCountMap[bucket.Key],
			TokenUsage:      tokenUsage,
			Status:          traceStatus,
			Input:           input,
			Output:          output,
		})
	}

	log.Info("Retrieved trace overviews (v2)",
		"totalCount", totalCount,
		"returned", len(overviews),
		"offset", params.Offset,
		"limit", params.Limit)

	return &opensearch.TraceOverviewResponse{
		Traces:     overviews,
		TotalCount: totalCount,
	}, nil
}

// ExportTracesV2 retrieves complete trace objects with all spans for export.
// Uses aggregation for trace discovery with pagination, then fetches all spans per trace.
func (s *TracingController) ExportTracesV2(ctx context.Context, params opensearch.TraceQueryParams) (*opensearch.TraceExportResponse, error) {
	log := logger.GetLogger(ctx)
	log.Info("Starting trace export (v2)",
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid,
		"startTime", params.StartTime,
		"endTime", params.EndTime,
		"limit", params.Limit,
		"offset", params.Offset)

	// Set defaults
	if params.Limit == 0 {
		params.Limit = DefaultTracesLimit
	}
	if params.Limit > MaxTracesPerRequest {
		params.Limit = MaxTracesPerRequest
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	// Phase 1: Aggregation to discover trace IDs with pagination
	aggQuery := opensearch.BuildTraceAggregationQuery(params)

	indices, err := opensearch.GetIndicesForTimeRange(params.StartTime, params.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	aggResponse, err := s.osClient.SearchWithAggregation(ctx, indices, aggQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute aggregation: %w", err)
	}

	totalCount := aggResponse.Aggregations.TotalTraces.Value
	buckets := aggResponse.Aggregations.Traces.Buckets

	// Apply pagination
	start := params.Offset
	end := params.Offset + params.Limit
	if start >= len(buckets) {
		return &opensearch.TraceExportResponse{
			Traces:     []opensearch.FullTrace{},
			TotalCount: totalCount,
		}, nil
	}
	if end > len(buckets) {
		end = len(buckets)
	}
	paginatedBuckets := buckets[start:end]

	if len(paginatedBuckets) == 0 {
		return &opensearch.TraceExportResponse{
			Traces:     []opensearch.FullTrace{},
			TotalCount: totalCount,
		}, nil
	}

	// Collect trace IDs and span counts, sum total spans for fetch limit
	traceIDs := make([]string, len(paginatedBuckets))
	spanCountMap := make(map[string]int, len(paginatedBuckets))
	totalSpanCount := 0
	for i, bucket := range paginatedBuckets {
		traceIDs[i] = bucket.Key
		spanCountMap[bucket.Key] = bucket.DocCount
		totalSpanCount += bucket.DocCount
	}

	// Cap at OpenSearch max_result_window default
	if totalSpanCount > MaxSpansPerRequest {
		totalSpanCount = MaxSpansPerRequest
	}

	// Phase 2: Fetch ALL spans for each trace (no parentSpan filter)
	// Use exact span count from aggregation as limit to avoid truncation
	allSpansParams := opensearch.V2TraceByIdParams{
		TraceIDs:       traceIDs,
		ComponentUid:   params.ComponentUid,
		EnvironmentUid: params.EnvironmentUid,
		ParentSpan:     false,
		Limit:          totalSpanCount,
	}

	allSpansQuery := opensearch.BuildV2TraceByIdsQuery(allSpansParams)
	allSpansResponse, err := s.osClient.Search(ctx, indices, allSpansQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch spans for export: %w", err)
	}

	allSpans := opensearch.ParseSpans(allSpansResponse)

	// Group spans by traceId
	spansByTrace := make(map[string][]opensearch.Span, len(traceIDs))
	for _, span := range allSpans {
		spansByTrace[span.TraceID] = append(spansByTrace[span.TraceID], span)
	}

	// Build FullTrace objects in aggregation order
	fullTraces := make([]opensearch.FullTrace, 0, len(paginatedBuckets))
	for _, bucket := range paginatedBuckets {
		traceSpans, ok := spansByTrace[bucket.Key]
		if !ok || len(traceSpans) == 0 {
			log.Warn("No spans found for trace in export, skipping",
				"traceId", bucket.Key)
			continue
		}

		// Sort spans by start time
		sort.Slice(traceSpans, func(i, j int) bool {
			return traceSpans[i].StartTime.Before(traceSpans[j].StartTime)
		})

		// Find root span
		var rootSpan *opensearch.Span
		for i := range traceSpans {
			if traceSpans[i].ParentSpanID == "" {
				rootSpan = &traceSpans[i]
				break
			}
		}

		if rootSpan == nil {
			log.Warn("No root span found for trace in export, skipping",
				"traceId", bucket.Key)
			continue
		}

		// Extract input/output from root span
		var input, output interface{}
		if opensearch.IsCrewAISpan(rootSpan.Attributes) {
			input, output = opensearch.ExtractCrewAIRootSpanInputOutput(rootSpan)
		} else {
			input, output = opensearch.ExtractRootSpanInputOutput(rootSpan)
		}

		// Token usage and status from all spans (same as v1 export)
		tokenUsage := opensearch.ExtractTokenUsage(traceSpans)
		traceStatus := opensearch.ExtractTraceStatus(traceSpans)

		// Extract task.id and trial.id from baggage attributes
		var taskId, trialId string
		if rootSpan.Attributes != nil {
			if v, ok := rootSpan.Attributes["task.id"].(string); ok {
				taskId = v
			}
			if v, ok := rootSpan.Attributes["trial.id"].(string); ok {
				trialId = v
			}
		}

		fullTraces = append(fullTraces, opensearch.FullTrace{
			TraceID:         bucket.Key,
			RootSpanID:      rootSpan.SpanID,
			RootSpanName:    rootSpan.Name,
			RootSpanKind:    string(opensearch.DetermineSpanType(*rootSpan)),
			StartTime:       rootSpan.StartTime.Format(time.RFC3339Nano),
			EndTime:         rootSpan.EndTime.Format(time.RFC3339Nano),
			DurationInNanos: rootSpan.DurationInNanos,
			SpanCount:       spanCountMap[bucket.Key],
			TokenUsage:      tokenUsage,
			Status:          traceStatus,
			Input:           input,
			Output:          output,
			TaskId:          taskId,
			TrialId:         trialId,
			Spans:           traceSpans,
		})
	}

	log.Info("Successfully completed trace export (v2)",
		"exportedTraces", len(fullTraces),
		"totalCount", totalCount,
		"offset", params.Offset,
		"limit", params.Limit)

	return &opensearch.TraceExportResponse{
		Traces:     fullTraces,
		TotalCount: totalCount,
	}, nil
}

// HealthCheck checks if the service is healthy
func (s *TracingController) HealthCheck(ctx context.Context) error {
	return s.osClient.HealthCheck(ctx)
}
