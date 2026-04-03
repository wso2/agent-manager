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

package controllers

import (
	"context"
	"sync"
	"time"

	"github.com/wso2/ai-agent-management-platform/traces-observer-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/observer"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/opensearch"
)

// V2TracingController provides tracing functionality via the observer service.
type V2TracingController struct {
	observerClient observer.Client
}

// NewV2TracingController creates a new v2 tracing controller.
func NewV2TracingController(observerClient observer.Client) *V2TracingController {
	return &V2TracingController{observerClient: observerClient}
}

// V2TraceQueryParams holds parameters for v2 trace queries.
type V2TraceQueryParams struct {
	Namespace   string
	Project     *string
	Component   *string
	Environment *string
	StartTime   time.Time
	EndTime     time.Time
	Limit       int
	SortOrder   string
}

// V2SpanSummary is a lightweight span summary for the span list endpoint.
type V2SpanSummary struct {
	SpanID       string    `json:"spanId"`
	SpanName     string    `json:"spanName"`
	ParentSpanID string    `json:"parentSpanId,omitempty"`
	StartTime    time.Time `json:"startTime"`
	EndTime      time.Time `json:"endTime"`
	DurationNs   int64     `json:"durationNs"`
}

// V2SpanListResponse is the response for GET /api/v2/traces/{traceId}/spans.
type V2SpanListResponse struct {
	Spans      []V2SpanSummary `json:"spans"`
	TotalCount int             `json:"totalCount"`
}

// GetTraceOverviews fetches a page of traces with root-span enrichment (input, output, tokenUsage).
// It calls QueryTraces once, then fetches root span details in parallel (one per trace in the page).
func (c *V2TracingController) GetTraceOverviews(ctx context.Context, params V2TraceQueryParams) (*opensearch.TraceOverviewResponse, error) {
	log := logger.GetLogger(ctx)

	sortOrder := params.SortOrder
	req := observer.TracesQueryRequest{
		StartTime: params.StartTime,
		EndTime:   params.EndTime,
		Limit:     &params.Limit,
		SortOrder: &sortOrder,
		SearchScope: observer.ComponentSearchScope{
			Namespace:   params.Namespace,
			Project:     params.Project,
			Component:   params.Component,
			Environment: params.Environment,
		},
	}

	tracesResp, err := c.observerClient.QueryTraces(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(tracesResp.Traces) == 0 {
		return &opensearch.TraceOverviewResponse{
			Traces:     []opensearch.TraceOverview{},
			TotalCount: tracesResp.Total,
		}, nil
	}

	// Fetch root span details in parallel for each trace in the page.
	type result struct {
		idx  int
		span *opensearch.Span
		err  error
	}
	results := make([]result, len(tracesResp.Traces))
	var wg sync.WaitGroup

	for i, t := range tracesResp.Traces {
		if t.RootSpanID == "" {
			log.Warn("trace has no rootSpanId, skipping", "traceId", t.TraceID)
			continue
		}
		wg.Add(1)
		go func(idx int, traceID, rootSpanID string) {
			defer wg.Done()
			details, err := c.observerClient.GetSpanDetails(ctx, traceID, rootSpanID)
			if err != nil {
				results[idx] = result{idx: idx, err: err}
				return
			}
			span := observer.ConvertSpanDetailsToSpan(traceID, details)
			enriched := opensearch.ProcessSpan(span)
			results[idx] = result{idx: idx, span: &enriched}
		}(i, t.TraceID, t.RootSpanID)
	}
	wg.Wait()

	overviews := make([]opensearch.TraceOverview, 0, len(tracesResp.Traces))
	for i, t := range tracesResp.Traces {
		res := results[i]
		if res.err != nil {
			log.Warn("failed to fetch root span details, skipping trace",
				"traceId", t.TraceID, "err", res.err)
			continue
		}
		if res.span == nil {
			continue
		}
		rootSpan := res.span

		// Extract input/output — same logic as controller.go lines 315-321.
		var input, output interface{}
		if opensearch.IsCrewAISpan(rootSpan.Attributes) {
			input, output = opensearch.ExtractCrewAIRootSpanInputOutput(rootSpan)
		} else {
			input, output = opensearch.ExtractRootSpanInputOutput(rootSpan)
		}

		// Extract token usage — same fallback chain as controller.go lines 323-335.
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
			TraceID:         t.TraceID,
			RootSpanID:      t.RootSpanID,
			RootSpanName:    t.RootSpanName,
			RootSpanKind:    string(opensearch.DetermineSpanType(*rootSpan)),
			StartTime:       t.StartTime.Format(time.RFC3339Nano),
			EndTime:         t.EndTime.Format(time.RFC3339Nano),
			DurationInNanos: t.DurationNs,
			SpanCount:       t.SpanCount,
			TokenUsage:      tokenUsage,
			Status:          traceStatus,
			Input:           input,
			Output:          output,
		})
	}

	log.Info("Retrieved v2 trace overviews",
		"totalCount", tracesResp.Total,
		"returned", len(overviews))

	return &opensearch.TraceOverviewResponse{
		Traces:     overviews,
		TotalCount: tracesResp.Total,
	}, nil
}

// GetTraceSpans fetches span summaries for a specific trace (no attributes).
func (c *V2TracingController) GetTraceSpans(ctx context.Context, traceID string, params V2TraceQueryParams) (*V2SpanListResponse, error) {
	log := logger.GetLogger(ctx)

	sortOrder := params.SortOrder
	req := observer.TracesQueryRequest{
		StartTime: params.StartTime,
		EndTime:   params.EndTime,
		Limit:     &params.Limit,
		SortOrder: &sortOrder,
		SearchScope: observer.ComponentSearchScope{
			Namespace:   params.Namespace,
			Project:     params.Project,
			Component:   params.Component,
			Environment: params.Environment,
		},
	}

	spansResp, err := c.observerClient.QueryTraceSpans(ctx, traceID, req)
	if err != nil {
		return nil, err
	}

	summaries := make([]V2SpanSummary, 0, len(spansResp.Spans))
	for _, s := range spansResp.Spans {
		summaries = append(summaries, V2SpanSummary{
			SpanID:       s.SpanID,
			SpanName:     s.SpanName,
			ParentSpanID: s.ParentSpanID,
			StartTime:    s.StartTime,
			EndTime:      s.EndTime,
			DurationNs:   s.DurationNs,
		})
	}

	log.Info("Retrieved v2 trace spans",
		"traceId", traceID,
		"totalCount", spansResp.Total,
		"returned", len(summaries))

	return &V2SpanListResponse{
		Spans:      summaries,
		TotalCount: spansResp.Total,
	}, nil
}

// GetSpanDetail fetches full span details including enriched AmpAttributes.
func (c *V2TracingController) GetSpanDetail(ctx context.Context, traceID, spanID string) (*opensearch.Span, error) {
	details, err := c.observerClient.GetSpanDetails(ctx, traceID, spanID)
	if err != nil {
		return nil, err
	}

	span := observer.ConvertSpanDetailsToSpan(traceID, details)
	enriched := opensearch.ProcessSpan(span)
	return &enriched, nil
}
