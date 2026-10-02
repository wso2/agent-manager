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
	"cmp"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
)

const (
	// MaxSpansPerRequest is the hard cap on spans fetched per trace (used in export).
	MaxSpansPerRequest = 10000
	// maxConcurrentFetches limits concurrent GetSpanDetails calls to the Observer.
	maxConcurrentFetches = 50
	// maxConcurrentTraces limits concurrent per-trace goroutines in ExportTraces.
	maxConcurrentTraces = 10
	// maxLLMLeavesPerTrace caps the number of leaf LLM spans fetched per trace
	// when the trace-list view falls back to leaf aggregation for Input/Output
	// preview and token totals (OpenAI Agents SDK / pure-OTel agents). Realistic
	// multi-turn agents stay under this; traces beyond the cap get TokenUsage.Partial=true.
	maxLLMLeavesPerTrace = 50
	// skipLeafAggregationSpanCountThreshold short-circuits leaf aggregation
	// entirely when the trace's total span count exceeds this — a rough proxy
	// for "way more than 50 LLM leaves" that keeps the worst-case list-endpoint
	// cost bounded.
	skipLeafAggregationSpanCountThreshold = 100
	// lookBackBatchSize is a filtered list request's first fetch size and the
	// number of traces it enriches at a time.
	lookBackBatchSize = 50
	// maxExaminedTraces caps the traces one filtered list request examines,
	// bounding its enrichment calls when a filter rarely matches.
	maxExaminedTraces = 500
	// maxCursorDepth caps the upstream fetch limit of a list request, which
	// a deep cursor grows. Each trace bucket carries about six
	// sub-aggregations, so this stays well under OpenSearch's default
	// search.max_buckets of 65535.
	maxCursorDepth = 5000
)

// TracingController provides tracing functionality via the observer service.
type TracingController struct {
	observerClient observer.Client
	// enrichAll turns off early filter rejection; tests compare against it.
	enrichAll bool
	// perSpanDetails turns off reusing the span list's inline attributes; tests compare against it.
	perSpanDetails bool
	// fullChunks turns off sizing summary-only chunks to the page; tests compare against it.
	fullChunks bool
}

// NewTracingController creates a new tracing controller.
func NewTracingController(observerClient observer.Client) *TracingController {
	return &TracingController{observerClient: observerClient}
}

// TraceQueryParams holds parameters for trace queries.
type TraceQueryParams struct {
	Organization string
	Project      *string
	Agent        *string
	Environment  *string
	StartTime    time.Time
	EndTime      time.Time
	Limit        int
	SortOrder    string
	Include      Include
	Filters      TraceFilters
	// Cursor continues from a previous page of the same window, sort order and filters.
	Cursor *TraceCursor
}

// Include is the set of opt-in span-derived fields for a trace list.
// The API exposes it as the comma-separated include query parameter.
// Each one is off by default because it costs extra upstream reads.
type Include struct {
	Models bool
}

// SpanSummary is a lightweight span summary for the span list endpoint.
type SpanSummary struct {
	SpanID        string    `json:"spanId"`
	SpanName      string    `json:"spanName"`
	SpanKind      string    `json:"spanKind,omitempty"`
	ParentSpanID  string    `json:"parentSpanId,omitempty"`
	StartTime     time.Time `json:"startTime"`
	EndTime       time.Time `json:"endTime"`
	DurationNs    int64     `json:"durationNs"`
	Error         bool      `json:"error,omitempty"`
	StatusMessage string    `json:"statusMessage,omitempty"`
}

// returns (e.g. "Error"), which is enough for the trace tree to flag failed spans.
func spanStatusIsError(s *observer.SpanStatus) bool {
	return s != nil && strings.EqualFold(s.Code, "error")
}

// SpanListResponse is the response for GET /api/v1/traces/{traceId}/spans.
type SpanListResponse struct {
	Spans      []SpanSummary `json:"spans"`
	TotalCount int           `json:"totalCount"`
}

// GetTraceOverviews fetches a page of traces with root-span enrichment (input, output, tokenUsage).
// With no filter and no cursor it calls QueryTraces once and fetches root span details in parallel.
// With a filter it looks back through the window in batches until the page fills,
// the window runs out, or maxExaminedTraces traces have been examined.
// A cursor continues from an earlier page over the same whole window.
func (c *TracingController) GetTraceOverviews(ctx context.Context, params TraceQueryParams) (*opensearch.TraceOverviewResponse, error) {
	// A model filter needs Models filled.
	if params.Filters.Model != "" {
		params.Include.Models = true
	}

	var resp *opensearch.TraceOverviewResponse
	var examined int
	var err error
	if params.Filters.IsZero() {
		resp, examined, err = c.traceOverviewPage(ctx, params)
	} else {
		resp, examined, err = c.lookBackForMatches(ctx, params)
	}
	if err != nil {
		return nil, err
	}

	logger.GetLogger(ctx).Info("Retrieved trace overviews",
		"organization", params.Organization,
		"filters", params.Filters,
		"cursor", params.Cursor != nil,
		"examined", examined,
		"matched", len(resp.Traces),
		"truncated", resp.Truncated)

	return resp, nil
}

// traceOverviewPage serves an unfiltered list. Without a cursor it calls
// QueryTraces once. With one it fetches the whole window, skips traces before
// the cursor, and fetches more only when the response comes back short.
func (c *TracingController) traceOverviewPage(ctx context.Context, params TraceQueryParams) (*opensearch.TraceOverviewResponse, int, error) {
	cur := params.Cursor
	asc := params.SortOrder == "asc"
	fetchLimit := params.Limit
	if cur != nil {
		fetchLimit = min(cur.Rank+params.Limit, maxCursorDepth)
	}
	for {
		tracesResp, err := c.observerClient.QueryTraces(ctx, c.traceListRequest(params, fetchLimit))
		if err != nil {
			return nil, 0, err
		}
		traces := tracesResp.Traces

		// Traces at the cursor time were on the previous page. They come back
		// but don't count toward the limit, so a page always moves past ties.
		page := make([]observer.TraceInfo, 0, min(len(traces), params.Limit))
		passed, counted := 0, 0
		last := walkStart(params)
		for _, t := range traces {
			if counted == params.Limit {
				break
			}
			passed++
			if beforeCursor(t.StartTime, cur, asc) {
				continue
			}
			page = append(page, t)
			last = t.StartTime
			if !atCursor(t.StartTime, cur) {
				counted++
			}
		}
		full := counted == params.Limit
		exhausted := passed == len(traces) && len(traces) >= tracesResp.Total
		if cur != nil && !full && !exhausted && fetchLimit < maxCursorDepth {
			fetchLimit = min(2*fetchLimit, maxCursorDepth)
			continue
		}

		resp := &opensearch.TraceOverviewResponse{
			Traces:       c.enrichTraces(ctx, params, page),
			TotalCount:   tracesResp.Total,
			LookedBackTo: formatCursor(last),
		}
		switch {
		case exhausted:
			resp.LookedBackTo = formatCursor(windowEdge(params))
		case full || cur == nil:
			rank := passed + rootlessSlots(fetchLimit, len(traces), tracesResp.Total)
			resp.NextCursor = TraceCursor{Rank: rank, Time: last}.Encode()
		default:
			// The cursor is too deep to fetch past.
			resp.Truncated = true
		}
		return resp, len(page), nil
	}
}

// lookBackForMatches walks the window in sort order, keeping traces that
// match params.Filters. It returns the matches and the number of traces
// examined.
//
// Each fetch covers the whole window with a doubled limit and skips trace IDs
// already seen. The window is never narrowed: the Observer bounds each span's
// end time, so a narrowed end would drop traces that overlap it. A cursor
// skips traces before it by time; they are not enriched or examined.
func (c *TracingController) lookBackForMatches(ctx context.Context, params TraceQueryParams) (*opensearch.TraceOverviewResponse, int, error) {
	cur := params.Cursor
	asc := params.SortOrder == "asc"
	summaryOnly := params.Filters.SummaryOnly() && !c.fullChunks
	matched := make([]opensearch.TraceOverview, 0, params.Limit)
	seen := make(map[string]struct{})
	// counted is the matches past the cursor time; rootless is the latest
	// fetch's slots spent on traces rooted outside the window.
	examined, skipped, counted, rootless := 0, 0, 0, 0
	// last is the last trace examined.
	last := walkStart(params)

	done := func(lookedBackTo time.Time, truncated, more bool) (*opensearch.TraceOverviewResponse, int, error) {
		resp := &opensearch.TraceOverviewResponse{
			Traces:       matched,
			TotalCount:   len(matched),
			LookedBackTo: formatCursor(lookedBackTo),
			Truncated:    truncated,
		}
		if more {
			resp.NextCursor = TraceCursor{Rank: skipped + examined + rootless, Time: last}.Encode()
		}
		return resp, examined, nil
	}

	fetchLimit := lookBackBatchSize
	if cur != nil {
		fetchLimit = min(cur.Rank+lookBackBatchSize, maxCursorDepth)
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, examined, fmt.Errorf("controllers.GetTraceOverviews: %w", err)
		}
		tracesResp, err := c.observerClient.QueryTraces(ctx, c.traceListRequest(params, fetchLimit))
		if err != nil {
			return nil, examined, fmt.Errorf("controllers.GetTraceOverviews: %w", err)
		}
		rootless = rootlessSlots(fetchLimit, len(tracesResp.Traces), tracesResp.Total)

		fresh := make([]observer.TraceInfo, 0, len(tracesResp.Traces))
		for _, t := range tracesResp.Traces {
			if _, ok := seen[t.TraceID]; ok {
				continue
			}
			seen[t.TraceID] = struct{}{}
			if beforeCursor(t.StartTime, cur, asc) {
				skipped++
				continue
			}
			fresh = append(fresh, t)
		}

		// Enrich in chunks and walk in sort order, so a full page stops at
		// its last match and the cursor never passes an unreturned match.
		for len(fresh) > 0 && examined < maxExaminedTraces {
			chunk := fresh[:min(lookBackBatchSize, len(fresh), maxExaminedTraces-examined)]
			// Summary-only survivors all match, so enrich no more than the page still needs.
			if summaryOnly {
				chunk = chunk[:summaryChunkLen(chunk, params.Filters, cur, params.Limit-counted)]
			}
			fresh = fresh[len(chunk):]
			byID := make(map[string]opensearch.TraceOverview, len(chunk))
			for _, ov := range c.enrichTraces(ctx, params, filterTraceInfos(chunk, params.Filters)) {
				byID[ov.TraceID] = ov
			}
			for i, t := range chunk {
				examined++
				last = t.StartTime
				ov, ok := byID[t.TraceID]
				if !ok || !matchesFilters(ov, params.Filters) {
					continue
				}
				matched = append(matched, ov)
				// Matches at the cursor time were on the previous page.
				if !atCursor(t.StartTime, cur) {
					counted++
				}
				if counted == params.Limit {
					if i == len(chunk)-1 && len(fresh) == 0 && len(seen) >= tracesResp.Total {
						return done(windowEdge(params), false, false)
					}
					return done(t.StartTime, false, true)
				}
			}
		}

		// Total counts every trace in the window. A short response does not
		// mean the window ran out: the limit also counts traces whose root
		// span lies outside the window, which the Observer then drops.
		exhausted := len(fresh) == 0 && len(seen) >= tracesResp.Total
		// A fetch past skipped+maxExaminedTraces would only add traces the cap rules out.
		next := min(2*fetchLimit, skipped+maxExaminedTraces, maxCursorDepth)
		switch {
		case exhausted:
			return done(windowEdge(params), false, false)
		case examined >= maxExaminedTraces || (next <= fetchLimit && fetchLimit < maxCursorDepth):
			return done(last, true, true)
		case next <= fetchLimit:
			// The cursor is too deep to fetch past.
			return done(last, true, false)
		}
		fetchLimit = next
	}
}

// traceListRequest builds the QueryTraces request for params' window.
func (c *TracingController) traceListRequest(params TraceQueryParams, limit int) observer.TracesQueryRequest {
	sortOrder := params.SortOrder
	return observer.TracesQueryRequest{
		StartTime: params.StartTime,
		EndTime:   params.EndTime,
		Limit:     &limit,
		SortOrder: &sortOrder,
		SearchScope: observer.ComponentSearchScope{
			Namespace:   c.observerClient.NamespaceFor(params.Organization),
			Project:     params.Project,
			Component:   params.Agent,
			Environment: params.Environment,
		},
	}
}

// walkStart is where a walk begins: the cursor time, or the window's near edge.
func walkStart(params TraceQueryParams) time.Time {
	switch {
	case params.Cursor != nil:
		return params.Cursor.Time
	case params.SortOrder == "asc":
		return params.StartTime
	default:
		return params.EndTime
	}
}

// windowEdge is the end of the window in paging direction.
func windowEdge(params TraceQueryParams) time.Time {
	if params.SortOrder == "asc" {
		return params.EndTime
	}
	return params.StartTime
}

// formatCursor keeps nanoseconds so the cursor pages without gaps.
func formatCursor(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

// enrichTraces fetches root spans and enriches traces in parallel, returning
// overviews in input order. Traces whose root fetch fails are skipped, and so
// are traces whose root fails matchesRootFilters or whose models fail the model
// filter, before the rest of the cascade. When listSpansFirst holds, the root
// comes from the trace's attribute span list instead of its own fetch.
func (c *TracingController) enrichTraces(ctx context.Context, params TraceQueryParams, traces []observer.TraceInfo) []opensearch.TraceOverview {
	log := logger.GetLogger(ctx)

	// outerSem caps how many traces are being enriched at once; innerSem caps
	// the total observer round-trips across all enrichments in flight (root
	// fetches plus anything enrichTraceOverview fetches inside).
	// Two pools avoid the deadlock a single shared pool would hit when every
	// outer slot is held by a goroutine waiting on a fetch slot.
	type result struct {
		span           *opensearch.Span
		input          interface{}
		output         interface{}
		tokenUsage     *opensearch.TokenUsage
		models         []string
		status         *opensearch.TraceStatus
		conversationID string
		// rejected marks a trace the root or model filters ruled out early.
		rejected bool
		err      error
	}
	results := make([]result, len(traces))
	outerSem := make(chan struct{}, maxConcurrentTraces)
	innerSem := make(chan struct{}, maxConcurrentFetches)
	var wg sync.WaitGroup

	for i, t := range traces {
		if t.RootSpanID == "" {
			log.Warn("trace has no rootSpanId, skipping", "traceId", t.TraceID)
			continue
		}
		wg.Add(1)
		go func(idx int, t observer.TraceInfo) {
			defer wg.Done()
			outerSem <- struct{}{}
			defer func() { <-outerSem }()

			var spans []observer.SpanInfo
			var root *opensearch.Span
			if c.listSpansFirst(params, t) {
				spans, root = c.rootFromSpanList(ctx, params, t, innerSem)
			}
			if root == nil {
				innerSem <- struct{}{}
				details, err := c.observerClient.GetSpanDetails(ctx, t.TraceID, t.RootSpanID)
				<-innerSem
				if err != nil {
					results[idx] = result{err: err}
					return
				}
				enriched := opensearch.ProcessSpan(observer.ConvertSpanDetailsToSpan(t.TraceID, details))
				root = &enriched
			}
			// Status is root-only by design.
			status := opensearch.ExtractTraceStatus([]opensearch.Span{*root})
			conversationID := opensearch.ExtractConversationID(root)
			if !c.enrichAll && !matchesRootFilters(status, conversationID, params.Filters) {
				results[idx] = result{rejected: true}
				return
			}
			input, output, tokens, models, rejected := c.enrichTraceOverview(ctx, params, t, root, spans, innerSem)
			if rejected {
				results[idx] = result{rejected: true}
				return
			}
			results[idx] = result{
				span:           root,
				input:          input,
				output:         output,
				tokenUsage:     tokens,
				models:         models,
				status:         status,
				conversationID: conversationID,
			}
		}(i, t)
	}
	wg.Wait()

	overviews := make([]opensearch.TraceOverview, 0, len(traces))
	for i, t := range traces {
		res := results[i]
		if res.err != nil {
			log.Warn("failed to fetch root span details, skipping trace",
				"traceId", t.TraceID, "err", res.err)
			continue
		}
		if res.rejected || res.span == nil {
			continue
		}
		rootSpan := res.span

		overviews = append(overviews, opensearch.TraceOverview{
			TraceID:         t.TraceID,
			RootSpanID:      t.RootSpanID,
			RootSpanName:    t.RootSpanName,
			RootSpanKind:    string(opensearch.DetermineSpanType(*rootSpan)),
			StartTime:       t.StartTime.Format(time.RFC3339Nano),
			EndTime:         t.EndTime.Format(time.RFC3339Nano),
			DurationInNanos: t.DurationNs,
			SpanCount:       t.SpanCount,
			TokenUsage:      res.tokenUsage,
			Status:          res.status,
			Input:           res.input,
			Output:          res.output,
			Models:          res.models,
			ConversationID:  res.conversationID,
		})
	}
	return overviews
}

// listSpansFirst reports whether t's span list is fetched with attributes
// whatever the root holds, so it can supply the root. Root filters keep the
// root fetch first, so a trace they reject never downloads its list.
func (c *TracingController) listSpansFirst(params TraceQueryParams, t observer.TraceInfo) bool {
	return !c.perSpanDetails && params.Include.Models &&
		t.SpanCount <= skipLeafAggregationSpanCountThreshold &&
		params.Filters.Status == TraceStatusAny && params.Filters.ConversationID == ""
}

// rootFromSpanList fetches t's attribute span list and builds the root from it.
// root is nil when the root isn't in the list; spans is nil when the fetch fails.
func (c *TracingController) rootFromSpanList(
	ctx context.Context,
	params TraceQueryParams,
	t observer.TraceInfo,
	fetchSem chan struct{},
) (spans []observer.SpanInfo, root *opensearch.Span) {
	spans, ok := c.fetchTraceSpanSummaries(ctx, params, t, fetchSem, true)
	if !ok {
		return nil, nil
	}
	if spans == nil {
		spans = []observer.SpanInfo{}
	}
	for _, s := range spans {
		if s.SpanID == t.RootSpanID {
			span := opensearch.ProcessSpan(observer.ConvertSpanInfoToSpan(t.TraceID, s))
			return spans, &span
		}
	}
	return spans, nil
}

// enrichTraceOverview computes Input/Output/Tokens/Models for one trace-list
// row, cascading through three sources in order of cost. enrichTraces calls it
// only for traces whose root passes matchesRootFilters (status and
// conversationId), so a rejected trace costs just its root fetch:
//
//  1. The root span's own attributes (older Traceloop entity.input/output
//     and CrewAI roll-up). Free — root span is already fetched.
//  2. The immediate child of the root, typically a chain span like
//     LangGraph.workflow that Traceloop's LangChain instrumentation still
//     decorates with traceloop.entity.input/output. Costs +1 GetSpanDetails.
//  3. Leaf LLM spans (anything ending in ".chat"). Used for OpenAI Agents
//     SDK / pure-OTel agents where neither the root nor any chain span
//     carries the conversation. Bounded: skipped entirely when the trace's
//     total span count exceeds skipLeafAggregationSpanCountThreshold; up to
//     maxLLMLeavesPerTrace leaves are aggregated, fetched in parallel;
//     TokenUsage.Partial is set true when the cap truncates the aggregation
//     or a leaf fetch fails.
//
// Each step only fills fields the earlier step left nil.
//
// Models come from step 3's leaves, or from the span list's inline attributes
// when params.Include.Models is set (see modelsFromSpanList). With a model
// filter, rejected is true once the models rule the trace out, before steps 2
// and 3; traces over the span threshold have no models and are rejected first.
//
// When the span list carries attributes, steps 2 and 3 read their spans from
// it instead of fetching them. spans is the list when the caller already
// fetched it with attributes, or nil.
//
// Token usage from traceloop.entity.output is used only when no step finds a
// gen_ai.usage.* report.
//
// fetchSem bounds the total observer fetches across all concurrent
// enrichments — callers pass a shared semaphore so a 50-trace page can't
// fan out into thousands of in-flight requests.
func (c *TracingController) enrichTraceOverview(
	ctx context.Context,
	params TraceQueryParams,
	traceInfo observer.TraceInfo,
	rootSpan *opensearch.Span,
	spans []observer.SpanInfo,
	fetchSem chan struct{},
) (input interface{}, output interface{}, tokenUsage *opensearch.TokenUsage, models []string, rejected bool) {
	// Step 1: root span attributes.
	if opensearch.IsCrewAISpan(rootSpan.Attributes) {
		input, output = opensearch.ExtractCrewAIRootSpanInputOutput(rootSpan)
		tokenUsage = opensearch.ExtractCrewAITraceTokenUsage(rootSpan)
	} else {
		input, output = opensearch.ExtractRootSpanInputOutput(rootSpan)
	}
	if tokenUsage == nil {
		tokenUsage = opensearch.ExtractTokenUsage([]opensearch.Span{*rootSpan})
	}
	entityTokens := opensearch.ExtractTokenUsageFromEntityOutput(rootSpan)

	rootComplete := input != nil && output != nil && tokenUsage != nil
	aggregateLeaves := traceInfo.SpanCount <= skipLeafAggregationSpanCountThreshold
	modelsFromList := params.Include.Models && aggregateLeaves
	rejectOnModel := !c.enrichAll && params.Filters.Model != ""

	// Without leaf aggregation the trace has no models.
	if rejectOnModel && !aggregateLeaves {
		return nil, nil, nil, nil, true
	}

	// Nothing left to fetch.
	if rootComplete && !modelsFromList {
		return input, output, tokenUsage, nil, false
	}

	// Steps 2 and 3 and inline models all need the span list. Fetch it once.
	if spans == nil {
		var ok bool
		spans, ok = c.fetchTraceSpanSummaries(ctx, params, traceInfo, fetchSem, modelsFromList)
		if !ok {
			return input, output, cmp.Or(tokenUsage, entityTokens), nil, false
		}
	}
	// The list carries attributes exactly when it was fetched for models.
	inline := modelsFromList && !c.perSpanDetails
	if modelsFromList {
		models = modelsFromSpanList(traceInfo.TraceID, spans)
		if rejectOnModel && !matchesModel(models, params.Filters) {
			return nil, nil, nil, nil, true
		}
	}

	// Step 2: immediate child of the root (Traceloop chain span path).
	if !rootComplete {
		if childInput, childOutput, childTokens, childEntityTokens, ok := c.tryChildChainSpan(ctx, traceInfo.TraceID, rootSpan.SpanID, spans, inline, fetchSem); ok {
			if input == nil {
				input = childInput
			}
			if output == nil {
				output = childOutput
			}
			if tokenUsage == nil {
				tokenUsage = childTokens
			}
			if entityTokens == nil {
				entityTokens = childEntityTokens
			}
		}
	}

	// Step 3: leaf LLM aggregation (OpenAI Agents SDK / pure-OTel path).
	stillMissing := input == nil || output == nil || tokenUsage == nil
	if stillMissing {
		if !aggregateLeaves {
			logger.GetLogger(ctx).Debug("skipping leaf-LLM aggregation: trace exceeds spanCount threshold",
				"traceId", traceInfo.TraceID,
				"spanCount", traceInfo.SpanCount,
				"threshold", skipLeafAggregationSpanCountThreshold)
		} else {
			leafInput, leafOutput, leafTokens, leafModels := c.aggregateFromLeafLLMSpans(ctx, traceInfo.TraceID, spans, inline, fetchSem)
			if models == nil {
				models = leafModels
			}
			if input == nil {
				input = leafInput
			}
			if output == nil {
				output = leafOutput
			}
			if tokenUsage == nil {
				tokenUsage = leafTokens
			}
		}
	}

	return input, output, cmp.Or(tokenUsage, entityTokens), models, false
}

// fetchTraceSpanSummaries calls QueryTraceSpans for one trace and returns
// the span-summary list, mirroring the request shape used elsewhere in this
// controller, with inline attributes when includeAttributes is set. Acquires a
// slot on fetchSem for the duration of the call so the call counts against the
// shared cross-trace fetch budget. Returns ok=false on error (logged as a
// warning); callers fall back gracefully to whatever they already extracted.
func (c *TracingController) fetchTraceSpanSummaries(
	ctx context.Context,
	params TraceQueryParams,
	traceInfo observer.TraceInfo,
	fetchSem chan struct{},
	includeAttributes bool,
) ([]observer.SpanInfo, bool) {
	log := logger.GetLogger(ctx)

	spanLimit := traceInfo.SpanCount
	if spanLimit <= 0 || spanLimit > MaxSpansPerRequest {
		spanLimit = MaxSpansPerRequest
	}
	fetchSem <- struct{}{}
	spansResp, err := c.observerClient.QueryTraceSpans(ctx, traceInfo.TraceID, observer.TracesQueryRequest{
		StartTime:         params.StartTime,
		EndTime:           params.EndTime,
		Limit:             &spanLimit,
		IncludeAttributes: includeAttributes,
		SearchScope: observer.ComponentSearchScope{
			Namespace:   c.observerClient.NamespaceFor(params.Organization),
			Project:     params.Project,
			Component:   params.Agent,
			Environment: params.Environment,
		},
	})
	<-fetchSem
	if err != nil {
		log.Warn("enrichTraceOverview: QueryTraceSpans failed, skipping enrichment",
			"traceId", traceInfo.TraceID, "err", err)
		return nil, false
	}
	return spansResp.Spans, true
}

// modelsFromSpanList returns the distinct model names across the leaf LLM
// spans of a span list fetched with inline attributes, in start-time order.
func modelsFromSpanList(traceID string, spans []observer.SpanInfo) []string {
	leaves := make([]opensearch.Span, 0)
	for _, s := range spans {
		if opensearch.IsLLMLeafSpan(s.SpanName) {
			leaves = append(leaves, opensearch.ProcessSpan(observer.ConvertSpanInfoToSpan(traceID, s)))
		}
	}
	if len(leaves) == 0 {
		return nil
	}
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].StartTime.Before(leaves[j].StartTime) })
	return opensearch.ExtractModels(leaves)
}

// tryChildChainSpan fetches the earliest immediate child of the root span
// and runs the same entity.input/output / entity.output token extractors
// against it. Covers LangChain / LangGraph agents where Traceloop emits the
// conversation summary on a chain span (e.g. LangGraph.workflow) right under
// an attribute-empty `invoke_agent` root.
//
// tokens comes from the span's gen_ai.usage.* attributes; entityTokens from
// its traceloop.entity.output. With inline set, the child comes from spans
// instead of a fetch.
//
// Returns ok=false when no immediate child is found or the fetch fails.
func (c *TracingController) tryChildChainSpan(
	ctx context.Context,
	traceID string,
	rootSpanID string,
	spans []observer.SpanInfo,
	inline bool,
	fetchSem chan struct{},
) (input interface{}, output interface{}, tokens, entityTokens *opensearch.TokenUsage, ok bool) {
	log := logger.GetLogger(ctx)

	// Pick the earliest-started direct child of the root, skipping leaf LLM
	// spans. We're looking for a chain/agent wrapper (e.g. LangGraph.workflow)
	// that summarizes the whole conversation, not a single LLM call — those
	// belong to step 3's full-trace aggregation, not a per-span lookup.
	child := -1
	for i, s := range spans {
		if s.ParentSpanID != rootSpanID {
			continue
		}
		if opensearch.IsLLMLeafSpan(s.SpanName) {
			continue
		}
		if child < 0 || s.StartTime.Before(spans[child].StartTime) {
			child = i
		}
	}
	if child < 0 {
		return nil, nil, nil, nil, false
	}

	var childSpan opensearch.Span
	if inline {
		childSpan = opensearch.ProcessSpan(observer.ConvertSpanInfoToSpan(traceID, spans[child]))
	} else {
		childID := spans[child].SpanID
		fetchSem <- struct{}{}
		details, err := c.observerClient.GetSpanDetails(ctx, traceID, childID)
		<-fetchSem
		if err != nil {
			log.Warn("tryChildChainSpan: GetSpanDetails failed",
				"traceId", traceID, "childSpanId", childID, "err", err)
			return nil, nil, nil, nil, false
		}
		childSpan = opensearch.ProcessSpan(observer.ConvertSpanDetailsToSpan(traceID, details))
	}

	input, output = opensearch.ExtractRootSpanInputOutput(&childSpan)
	tokens = opensearch.ExtractTokenUsage([]opensearch.Span{childSpan})
	entityTokens = opensearch.ExtractTokenUsageFromEntityOutput(&childSpan)
	return input, output, tokens, entityTokens, true
}

// aggregateFromLeafLLMSpans fetches up to maxLLMLeavesPerTrace leaf LLM spans
// (name matches IsLLMLeafSpan) and aggregates token usage across them plus
// first/last message previews and distinct model names. Used when neither
// the root nor a child chain span carries the data (OpenAI Agents SDK /
// pure-OTel agents).
//
// Fetches share fetchSem with every other enrichment in flight, so leaf fan-
// out across many concurrent traces doesn't flood the upstream observer. With
// inline set, the leaves come from spans instead and nothing is fetched; the
// cap still applies, so a row is the same with or without include=models.
//
// If the trace has more LLM leaves than the cap, or any leaf fetch fails, the
// returned TokenUsage has Partial=true so the UI can render an "approximate"
// marker.
func (c *TracingController) aggregateFromLeafLLMSpans(
	ctx context.Context,
	traceID string,
	spans []observer.SpanInfo,
	inline bool,
	fetchSem chan struct{},
) (input interface{}, output interface{}, tokens *opensearch.TokenUsage, models []string) {
	log := logger.GetLogger(ctx)

	// Filter to leaf LLM spans, ordered by start time.
	leaves := make([]observer.SpanInfo, 0)
	for _, s := range spans {
		if opensearch.IsLLMLeafSpan(s.SpanName) {
			leaves = append(leaves, s)
		}
	}
	if len(leaves) == 0 {
		return nil, nil, nil, nil
	}
	sort.Slice(leaves, func(i, j int) bool { return leaves[i].StartTime.Before(leaves[j].StartTime) })

	totalLeaves := len(leaves)
	partial := false
	if totalLeaves > maxLLMLeavesPerTrace {
		leaves = leaves[:maxLLMLeavesPerTrace]
		partial = true
		log.Debug("aggregateFromLeafLLMSpans: capping leaves",
			"traceId", traceID, "totalLeaves", totalLeaves, "cap", maxLLMLeavesPerTrace)
	}

	fetched := make([]opensearch.Span, len(leaves))
	var wg sync.WaitGroup
	for i, leaf := range leaves {
		if inline {
			fetched[i] = opensearch.ProcessSpan(observer.ConvertSpanInfoToSpan(traceID, leaf))
			continue
		}
		wg.Add(1)
		go func(idx int, spanID string) {
			defer wg.Done()
			fetchSem <- struct{}{}
			defer func() { <-fetchSem }()

			details, err := c.observerClient.GetSpanDetails(ctx, traceID, spanID)
			if err != nil {
				log.Warn("aggregateFromLeafLLMSpans: GetSpanDetails failed for leaf",
					"traceId", traceID, "spanId", spanID, "err", err)
				return
			}
			fetched[idx] = opensearch.ProcessSpan(observer.ConvertSpanDetailsToSpan(traceID, details))
		}(i, leaf.SpanID)
	}
	wg.Wait()

	// Trim out any leaves that failed to fetch (zero-value Span).
	validLeaves := make([]opensearch.Span, 0, len(fetched))
	for _, s := range fetched {
		if s.SpanID != "" {
			validLeaves = append(validLeaves, s)
		}
	}
	if len(validLeaves) == 0 {
		return nil, nil, nil, nil
	}

	models = opensearch.ExtractModels(validLeaves)
	tokens = opensearch.ExtractTokenUsage(validLeaves)
	if tokens != nil && (partial || len(validLeaves) < len(leaves)) {
		tokens.Partial = true
	}

	// Input preview from the first leaf that actually carries messages. A
	// framework that wraps the provider call in its own LLM span (Strands opens
	// "chat" around "openai.chat") puts a content-free span first, and reading
	// only validLeaves[0] leaves the trace list's Input column blank.
	for i := range validLeaves {
		if v := opensearch.ExtractInputPreviewFromLeaf(&validLeaves[i]); v != nil {
			input = v
			break
		}
	}
	// Output stays pinned to the last leaf: it is the turn's final model call,
	// and walking backwards would surface an earlier turn's answer instead.
	output = opensearch.ExtractOutputPreviewFromLeaf(&validLeaves[len(validLeaves)-1])
	return input, output, tokens, models
}

// GetTraceSpans fetches span summaries for a specific trace (no attributes).
func (c *TracingController) GetTraceSpans(ctx context.Context, traceID string, params TraceQueryParams) (*SpanListResponse, error) {
	log := logger.GetLogger(ctx)

	sortOrder := params.SortOrder
	req := observer.TracesQueryRequest{
		StartTime: params.StartTime,
		EndTime:   params.EndTime,
		Limit:     &params.Limit,
		SortOrder: &sortOrder,
		SearchScope: observer.ComponentSearchScope{
			Namespace:   c.observerClient.NamespaceFor(params.Organization),
			Project:     params.Project,
			Component:   params.Agent,
			Environment: params.Environment,
		},
	}

	spansResp, err := c.observerClient.QueryTraceSpans(ctx, traceID, req)
	if err != nil {
		return nil, err
	}

	summaries := make([]SpanSummary, 0, len(spansResp.Spans))
	for _, s := range spansResp.Spans {
		summary := SpanSummary{
			SpanID:       s.SpanID,
			SpanName:     s.SpanName,
			SpanKind:     string(opensearch.DetermineSpanKindFromName(s.SpanName)),
			ParentSpanID: s.ParentSpanID,
			StartTime:    s.StartTime,
			EndTime:      s.EndTime,
			DurationNs:   s.DurationNs,
			Error:        spanStatusIsError(s.Status),
		}
		if s.Status != nil {
			summary.StatusMessage = s.Status.Message
		}
		summaries = append(summaries, summary)
	}

	log.Info("Retrieved trace spans",
		"traceId", traceID,
		"totalCount", spansResp.Total,
		"returned", len(summaries))

	return &SpanListResponse{
		Spans:      summaries,
		TotalCount: spansResp.Total,
	}, nil
}

// GetSpanDetail fetches full span details including enriched AmpAttributes.
func (c *TracingController) GetSpanDetail(ctx context.Context, traceID, spanID string) (*opensearch.Span, error) {
	details, err := c.observerClient.GetSpanDetails(ctx, traceID, spanID)
	if err != nil {
		return nil, err
	}

	span := observer.ConvertSpanDetailsToSpan(traceID, details)
	enriched := opensearch.ProcessSpan(span)
	return &enriched, nil
}

// ExportTraces fetches complete traces with all spans fully enriched for export.
// Observer calls: 1 QueryTraces + N QueryTraceSpans (spans carry attributes
// inline via includeAttributes). Concurrency is bounded by maxConcurrentTraces
// outer goroutines. Any single failure aborts the entire export.
func (c *TracingController) ExportTraces(ctx context.Context, params TraceQueryParams) (*opensearch.TraceExportResponse, error) {
	log := logger.GetLogger(ctx)

	sortOrder := params.SortOrder
	req := observer.TracesQueryRequest{
		StartTime: params.StartTime,
		EndTime:   params.EndTime,
		Limit:     &params.Limit,
		SortOrder: &sortOrder,
		SearchScope: observer.ComponentSearchScope{
			Namespace:   c.observerClient.NamespaceFor(params.Organization),
			Project:     params.Project,
			Component:   params.Agent,
			Environment: params.Environment,
		},
	}

	tracesResp, err := c.observerClient.QueryTraces(ctx, req)
	if err != nil {
		return nil, err
	}

	if len(tracesResp.Traces) == 0 {
		return &opensearch.TraceExportResponse{
			Traces:     []opensearch.FullTrace{},
			TotalCount: tracesResp.Total,
		}, nil
	}

	type traceResult struct {
		idx       int
		fullTrace *opensearch.FullTrace
	}

	results := make([]traceResult, len(tracesResp.Traces))
	var truncated atomic.Bool

	// Fail-fast: first error cancels all in-flight requests.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var firstErr error
	var errOnce sync.Once

	outerSem := make(chan struct{}, maxConcurrentTraces)
	var wg sync.WaitGroup

	for i, t := range tracesResp.Traces {
		wg.Add(1)
		go func(idx int, traceInfo observer.TraceInfo) {
			defer wg.Done()

			outerSem <- struct{}{}
			defer func() { <-outerSem }()

			if ctx.Err() != nil {
				return
			}

			spanLimit := traceInfo.SpanCount
			if spanLimit <= 0 || spanLimit > MaxSpansPerRequest {
				spanLimit = MaxSpansPerRequest
			}

			spansResp, err := c.observerClient.QueryTraceSpans(ctx, traceInfo.TraceID, observer.TracesQueryRequest{
				StartTime:         params.StartTime,
				EndTime:           params.EndTime,
				Limit:             &spanLimit,
				IncludeAttributes: true,
				SearchScope: observer.ComponentSearchScope{
					Namespace:   c.observerClient.NamespaceFor(params.Organization),
					Project:     params.Project,
					Component:   params.Agent,
					Environment: params.Environment,
				},
			})
			if err != nil {
				errOnce.Do(func() {
					firstErr = fmt.Errorf("trace %s: query spans: %w", traceInfo.TraceID, err)
					cancel()
				})
				return
			}

			if traceInfo.SpanCount > MaxSpansPerRequest {
				truncated.Store(true)
			}

			// Spans already carry attributes/kind/status from the bulk
			// QueryTraceSpans call (includeAttributes=true), so convert them
			// directly — no per-span GetSpanDetails round-trips.
			spans := make([]opensearch.Span, 0, len(spansResp.Spans))
			for _, s := range spansResp.Spans {
				enriched := opensearch.ProcessSpan(observer.ConvertSpanInfoToSpan(traceInfo.TraceID, s))
				spans = append(spans, enriched)
			}
			sort.Slice(spans, func(i, j int) bool {
				return spans[i].StartTime.Before(spans[j].StartTime)
			})

			// Find root span by the RootSpanID the Observer already identified.
			// Fallback: also accept a span with an empty or all-zero parentSpanId,
			// since some OTEL exporters use "0000000000000000" instead of "".
			var rootSpan *opensearch.Span
			for k := range spans {
				if spans[k].SpanID == traceInfo.RootSpanID {
					rootSpan = &spans[k]
					break
				}
			}
			if rootSpan == nil {
				// Fallback for traces where the Observer RootSpanID is absent.
				for k := range spans {
					p := spans[k].ParentSpanID
					if p == "" || p == "0000000000000000" {
						rootSpan = &spans[k]
						break
					}
				}
			}
			if rootSpan == nil {
				errOnce.Do(func() {
					firstErr = fmt.Errorf("trace %s: no root span found", traceInfo.TraceID)
					cancel()
				})
				return
			}

			// Extract input/output. Agent-rooted traces (e.g. a LangGraph
			// invoke_agent wrapper) carry no traceloop.entity.output on the
			// root, so fall back through the child chain span and leaf-LLM
			// spans — otherwise the exported trace has a blank response and the
			// LLM judge scores nothing. All spans are already in memory here.
			input, output := opensearch.ExtractTraceInputOutputWithFallback(rootSpan, spans)

			tokenUsage := opensearch.ExtractTokenUsage(spans)
			traceStatus := opensearch.ExtractTraceStatus(spans)

			// Extract taskId / trialId from root span baggage attributes.
			var taskID, trialID string
			if rootSpan.Attributes != nil {
				if v, ok := rootSpan.Attributes["task.id"].(string); ok {
					taskID = v
				}
				if v, ok := rootSpan.Attributes["trial.id"].(string); ok {
					trialID = v
				}
			}

			results[idx] = traceResult{
				idx: idx,
				fullTrace: &opensearch.FullTrace{
					TraceID:         traceInfo.TraceID,
					RootSpanID:      rootSpan.SpanID,
					RootSpanName:    rootSpan.Name,
					RootSpanKind:    string(opensearch.DetermineSpanType(*rootSpan)),
					StartTime:       traceInfo.StartTime.Format(time.RFC3339Nano),
					EndTime:         traceInfo.EndTime.Format(time.RFC3339Nano),
					DurationInNanos: traceInfo.DurationNs,
					SpanCount:       traceInfo.SpanCount,
					TokenUsage:      tokenUsage,
					Status:          traceStatus,
					Input:           input,
					Output:          output,
					TaskId:          taskID,
					TrialId:         trialID,
					Spans:           spans,
				},
			}
		}(i, t)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	fullTraces := make([]opensearch.FullTrace, 0, len(results))
	for _, r := range results {
		if r.fullTrace != nil {
			fullTraces = append(fullTraces, *r.fullTrace)
		}
	}

	log.Info("Completed trace export",
		"totalCount", tracesResp.Total,
		"exported", len(fullTraces),
		"truncated", truncated.Load())

	return &opensearch.TraceExportResponse{
		Traces:     fullTraces,
		TotalCount: tracesResp.Total,
		Truncated:  truncated.Load(),
	}, nil
}
