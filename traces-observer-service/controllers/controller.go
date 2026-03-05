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

const (
	// compositeAggBatchSize is the number of buckets to fetch per composite aggregation request
	compositeAggBatchSize = 1000
)

// traceBucketWithMetadata holds trace bucket data with metadata for sorting
type traceBucketWithMetadata struct {
	TraceID       string
	DocCount      int
	EarliestStart float64 // Unix timestamp in milliseconds
	SpanCount     int
}

// collectAllTraceBuckets fetches all trace buckets using composite aggregation,
// iterating with the after key until all buckets are collected.
func (s *TracingController) collectAllTraceBuckets(
	ctx context.Context,
	params opensearch.TraceQueryParams,
	indices []string,
) ([]traceBucketWithMetadata, error) {
	log := logger.GetLogger(ctx)

	var allBuckets []traceBucketWithMetadata
	var afterKey *opensearch.CompositeAfterKey
	requestCount := 0

	for {
		requestCount++
		query := opensearch.BuildCompositeTraceAggregationQuery(params, afterKey, compositeAggBatchSize)

		response, err := s.osClient.SearchWithCompositeAggregation(ctx, indices, query)
		if err != nil {
			return nil, fmt.Errorf("failed to execute composite aggregation (request %d): %w", requestCount, err)
		}

		buckets := response.Aggregations.TraceComposite.Buckets
		if len(buckets) == 0 {
			break
		}

		for _, bucket := range buckets {
			if bucket.RootSpanCount.DocCount == 0 {
				continue
			}

			allBuckets = append(allBuckets, traceBucketWithMetadata{
				TraceID:       bucket.Key.TraceID,
				DocCount:      bucket.DocCount,
				EarliestStart: bucket.EarliestStart.Value,
				SpanCount:     bucket.SpanCount.Value,
			})
		}

		if response.Aggregations.TraceComposite.AfterKey == nil {
			break
		}
		afterKey = response.Aggregations.TraceComposite.AfterKey

		log.Debug("Composite aggregation pagination",
			"request", requestCount,
			"buckets_this_batch", len(buckets),
			"total_collected", len(allBuckets))
	}

	log.Info("Collected all trace buckets",
		"total_traces", len(allBuckets),
		"requests", requestCount)

	return allBuckets, nil
}

// sortAndPaginateTraceBuckets sorts buckets by earliest_start and applies offset/limit.
func sortAndPaginateTraceBuckets(
	buckets []traceBucketWithMetadata,
	sortOrder string,
	offset int,
	limit int,
) []traceBucketWithMetadata {
	sort.Slice(buckets, func(i, j int) bool {
		if sortOrder == "asc" {
			return buckets[i].EarliestStart < buckets[j].EarliestStart
		}
		return buckets[i].EarliestStart > buckets[j].EarliestStart
	})

	start := offset
	if start >= len(buckets) {
		return []traceBucketWithMetadata{}
	}

	end := offset + limit
	if end > len(buckets) {
		end = len(buckets)
	}

	return buckets[start:end]
}

// GetTraceById retrieves spans for a specific trace.
// When params.ParentSpan is true, only the root span (parentSpanId == "") is returned.
func (s *TracingController) GetTraceById(ctx context.Context, params opensearch.TraceByIdParams) (*opensearch.TraceResponse, error) {
	log := logger.GetLogger(ctx)
	log.Info("Getting trace by ID",
		"traceIds", params.TraceIDs,
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid,
		"parentSpan", params.ParentSpan)

	// Build query
	query := opensearch.BuildTraceByIdsQuery(params)

	// Resolve indices from time range, or search all if no time range provided
	var indices []string
	var err error
	if params.StartTime != "" && params.EndTime != "" {
		indices, err = opensearch.GetIndicesForTimeRange(params.StartTime, params.EndTime)
		if err != nil {
			return nil, fmt.Errorf("failed to generate indices: %w", err)
		}
	} else {
		indices = opensearch.GetAllTraceIndices()
	}

	// Execute search
	response, err := s.osClient.Search(ctx, indices, query)
	if err != nil {
		return nil, fmt.Errorf("failed to search traces: %w", err)
	}

	// Parse spans
	spans := opensearch.ParseSpans(response)

	if len(spans) == 0 {
		log.Warn("No spans found for trace",
			"traceIds", params.TraceIDs,
			"component", params.ComponentUid,
			"environment", params.EnvironmentUid)
		return nil, ErrTraceNotFound
	}

	// Extract token usage and status from returned spans
	tokenUsage := opensearch.ExtractTokenUsage(spans)
	traceStatus := opensearch.ExtractTraceStatus(spans)

	log.Info("Retrieved trace spans",
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

// GetTraceOverviews retrieves trace overviews using OpenSearch aggregations for
// trace-level grouping and pagination, then enriches with root span data.
func (s *TracingController) GetTraceOverviews(ctx context.Context, params opensearch.TraceQueryParams) (*opensearch.TraceOverviewResponse, error) {
	log := logger.GetLogger(ctx)
	log.Info("Getting trace overviews",
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid,
		"startTime", params.StartTime,
		"endTime", params.EndTime,
		"limit", params.Limit,
		"offset", params.Offset)

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = DefaultTracesLimit
	}
	if params.Limit > MaxTracesPerRequest {
		params.Limit = MaxTracesPerRequest
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	// Phase 1: Composite aggregation to discover ALL trace IDs with exact pagination
	var indices []string
	var err error

	if params.StartTime != "" && params.EndTime != "" {
		indices, err = opensearch.GetIndicesForTimeRange(params.StartTime, params.EndTime)
		if err != nil {
			return nil, fmt.Errorf("failed to generate indices: %w", err)
		}
	} else {
		indices = opensearch.GetAllTraceIndices()
	}

	allBuckets, err := s.collectAllTraceBuckets(ctx, params, indices)
	if err != nil {
		return nil, fmt.Errorf("failed to collect trace buckets: %w", err)
	}

	totalCount := len(allBuckets)
	paginatedBuckets := sortAndPaginateTraceBuckets(allBuckets, params.SortOrder, params.Offset, params.Limit)

	if len(paginatedBuckets) == 0 {
		return &opensearch.TraceOverviewResponse{
			Traces:     []opensearch.TraceOverview{},
			TotalCount: totalCount,
		}, nil
	}

	// Collect trace IDs and span counts from paginated buckets
	traceIDs := make([]string, len(paginatedBuckets))
	spanCountMap := make(map[string]int, len(paginatedBuckets))
	for i, bucket := range paginatedBuckets {
		traceIDs[i] = bucket.TraceID
		spanCountMap[bucket.TraceID] = bucket.SpanCount
	}

	// Phase 2: Fetch root spans for enrichment
	rootSpanParams := opensearch.TraceByIdParams{
		TraceIDs:       traceIDs,
		ComponentUid:   params.ComponentUid,
		EnvironmentUid: params.EnvironmentUid,
		ParentSpan:     true,
		Limit:          len(traceIDs), // One root span per trace
	}

	rootSpanQuery := opensearch.BuildTraceByIdsQuery(rootSpanParams)
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
		rootSpan, hasRoot := rootSpanMap[bucket.TraceID]
		if !hasRoot {
			log.Warn("No root span found for trace, skipping",
				"traceId", bucket.TraceID)
			continue
		}

		// Extract input/output from root span
		var input, output interface{}
		if opensearch.IsCrewAISpan(rootSpan.Attributes) {
			input, output = opensearch.ExtractCrewAIRootSpanInputOutput(rootSpan)
		} else {
			input, output = opensearch.ExtractRootSpanInputOutput(rootSpan)
		}

		// Extract token usage from root span.
		// For CrewAI traces, read from crewai.crew.token_usage on the workflow span.
		// Otherwise try traceloop.entity.output, then fall back to gen_ai.usage.* attributes.
		var tokenUsage *opensearch.TokenUsage
		if opensearch.IsCrewAISpan(rootSpan.Attributes) {
			tokenUsage = opensearch.ExtractCrewAITraceTokenUsage(rootSpan)
		}
		if tokenUsage == nil {
			tokenUsage = opensearch.ExtractTokenUsageFromEntityOutput(rootSpan)
		}
		if tokenUsage == nil {
			tokenUsage = opensearch.ExtractTokenUsage([]opensearch.Span{*rootSpan})
		}
		traceStatus := opensearch.ExtractTraceStatus([]opensearch.Span{*rootSpan})

		overviews = append(overviews, opensearch.TraceOverview{
			TraceID:         bucket.TraceID,
			RootSpanID:      rootSpan.SpanID,
			RootSpanName:    rootSpan.Name,
			RootSpanKind:    string(opensearch.DetermineSpanType(*rootSpan)),
			StartTime:       rootSpan.StartTime.Format(time.RFC3339Nano),
			EndTime:         rootSpan.EndTime.Format(time.RFC3339Nano),
			DurationInNanos: rootSpan.DurationInNanos,
			SpanCount:       spanCountMap[bucket.TraceID],
			TokenUsage:      tokenUsage,
			Status:          traceStatus,
			Input:           input,
			Output:          output,
		})
	}

	log.Info("Retrieved trace overviews",
		"totalCount", totalCount,
		"returned", len(overviews),
		"offset", params.Offset,
		"limit", params.Limit)

	return &opensearch.TraceOverviewResponse{
		Traces:     overviews,
		TotalCount: totalCount,
	}, nil
}

// ExportTraces retrieves complete trace objects with all spans for export.
// Uses aggregation for trace discovery with pagination, then fetches all spans per trace.
func (s *TracingController) ExportTraces(ctx context.Context, params opensearch.TraceQueryParams) (*opensearch.TraceExportResponse, error) {
	log := logger.GetLogger(ctx)
	log.Info("Starting trace export",
		"component", params.ComponentUid,
		"environment", params.EnvironmentUid,
		"startTime", params.StartTime,
		"endTime", params.EndTime,
		"limit", params.Limit,
		"offset", params.Offset)

	// Set defaults
	if params.Limit <= 0 {
		params.Limit = DefaultTracesLimit
	}
	if params.Limit > MaxTracesPerRequest {
		params.Limit = MaxTracesPerRequest
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	// Phase 1: Composite aggregation to discover ALL trace IDs with exact pagination
	indices, err := opensearch.GetIndicesForTimeRange(params.StartTime, params.EndTime)
	if err != nil {
		return nil, fmt.Errorf("failed to generate indices: %w", err)
	}

	allBuckets, err := s.collectAllTraceBuckets(ctx, params, indices)
	if err != nil {
		return nil, fmt.Errorf("failed to collect trace buckets: %w", err)
	}

	totalCount := len(allBuckets)
	paginatedBuckets := sortAndPaginateTraceBuckets(allBuckets, params.SortOrder, params.Offset, params.Limit)

	if len(paginatedBuckets) == 0 {
		return &opensearch.TraceExportResponse{
			Traces:     []opensearch.FullTrace{},
			TotalCount: totalCount,
		}, nil
	}

	// Collect trace IDs and unique span counts for response.
	// Use raw doc_count for fetch sizing to avoid under-fetch when duplicates exist.
	traceIDs := make([]string, len(paginatedBuckets))
	spanCountMap := make(map[string]int, len(paginatedBuckets))
	totalSpanCount := 0
	for i, bucket := range paginatedBuckets {
		traceIDs[i] = bucket.TraceID
		spanCountMap[bucket.TraceID] = bucket.SpanCount
		totalSpanCount += bucket.DocCount
	}

	// Cap at OpenSearch max_result_window default
	truncated := false
	if totalSpanCount > MaxSpansPerRequest {
		log.Warn("Span count exceeds maximum, export will be truncated",
			"requestedSpans", totalSpanCount,
			"maxSpans", MaxSpansPerRequest)
		totalSpanCount = MaxSpansPerRequest
		truncated = true
	}

	// Phase 2: Fetch ALL spans for each trace (no parentSpan filter)
	// Use exact span count from aggregation as limit to avoid truncation
	allSpansParams := opensearch.TraceByIdParams{
		TraceIDs:       traceIDs,
		ComponentUid:   params.ComponentUid,
		EnvironmentUid: params.EnvironmentUid,
		ParentSpan:     false,
		Limit:          totalSpanCount,
	}

	allSpansQuery := opensearch.BuildTraceByIdsQuery(allSpansParams)
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
		traceSpans, ok := spansByTrace[bucket.TraceID]
		if !ok || len(traceSpans) == 0 {
			log.Warn("No spans found for trace in export, skipping",
				"traceId", bucket.TraceID)
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
				"traceId", bucket.TraceID)
			continue
		}

		// Extract input/output from root span
		var input, output interface{}
		if opensearch.IsCrewAISpan(rootSpan.Attributes) {
			input, output = opensearch.ExtractCrewAIRootSpanInputOutput(rootSpan)
		} else {
			input, output = opensearch.ExtractRootSpanInputOutput(rootSpan)
		}

		// Token usage and status from all spans
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
			TraceID:         bucket.TraceID,
			RootSpanID:      rootSpan.SpanID,
			RootSpanName:    rootSpan.Name,
			RootSpanKind:    string(opensearch.DetermineSpanType(*rootSpan)),
			StartTime:       rootSpan.StartTime.Format(time.RFC3339Nano),
			EndTime:         rootSpan.EndTime.Format(time.RFC3339Nano),
			DurationInNanos: rootSpan.DurationInNanos,
			SpanCount:       spanCountMap[bucket.TraceID],
			TokenUsage:      tokenUsage,
			Status:          traceStatus,
			Input:           input,
			Output:          output,
			TaskId:          taskId,
			TrialId:         trialId,
			Spans:           traceSpans,
		})
	}

	log.Info("Successfully completed trace export",
		"exportedTraces", len(fullTraces),
		"totalCount", totalCount,
		"offset", params.Offset,
		"limit", params.Limit)

	return &opensearch.TraceExportResponse{
		Traces:     fullTraces,
		TotalCount: totalCount,
		Truncated:  truncated,
	}, nil
}

// HealthCheck checks if the service is healthy
func (s *TracingController) HealthCheck(ctx context.Context) error {
	return s.osClient.HealthCheck(ctx)
}
