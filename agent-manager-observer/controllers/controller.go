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
	"slices"
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
	// maxToolListSpans is the most spans a trace can have and still report tools.
	maxToolListSpans = 200
	// maxMCPHandshakesPerTrace caps the MCP initialize spans fetched per trace, one per tool, in start order.
	maxMCPHandshakesPerTrace = 5
	// maxMCPHandshakesPerRequest ends a filtered walk before its next chunk once it has fetched this many handshakes.
	maxMCPHandshakesPerRequest = 100
	// lookBackBatchSize is a filtered list request's first fetch size and the
	// number of traces it enriches at a time.
	lookBackBatchSize = 50
	// maxExaminedTraces caps the traces one filtered list request examines,
	// bounding its enrichment calls when a filter rarely matches.
	maxExaminedTraces = 500
	// requestTimeout ends a trace list or export inside the server's 30 s WriteTimeout.
	requestTimeout = 25 * time.Second
	// listLookBackBudget ends a filtered list's walk well inside requestTimeout.
	listLookBackBudget = 20 * time.Second
	// exportLookBackBudget leaves part of requestTimeout for a filtered export's span fetches.
	exportLookBackBudget = 10 * time.Second
	// maxCursorDepth caps the upstream fetch limit of a list request, which
	// a deep cursor or a large tie group grows. The upstream Observer rejects
	// a trace query limit above 1000.
	maxCursorDepth = 1000
)

// TracingController provides tracing functionality via the observer service.
type TracingController struct {
	observerClient observer.Client
	// budget bounds a look-back walk's context.
	budget func(context.Context, time.Duration) (context.Context, context.CancelFunc)
	// scores serves score filters; nil turns them off.
	scores ScoreClient
}

// NewTracingController creates a new tracing controller.
func NewTracingController(observerClient observer.Client) *TracingController {
	return &TracingController{observerClient: observerClient, budget: context.WithTimeout}
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
	Cursor       *TraceCursor
}

// Include is the set of opt-in span-derived fields for a trace list.
// The API exposes it as the comma-separated include query parameter.
// Each one is off by default because it costs extra upstream reads.
type Include struct {
	Models     bool
	Tools      bool
	MCPServers bool
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

// walkStats describes how a page's walk went, beside the page itself.
type walkStats struct {
	examined int
	// read counts the examined traces enrichment read, kept or ruled out.
	read           int
	budgetExceeded bool
	// handshakeCapReached is set when maxMCPHandshakesPerRequest stopped the walk.
	handshakeCapReached bool
	// failed lists the examined traces that couldn't be read, in page order.
	failed []string
}

// GetTraceOverviews fetches a page of traces with root-span enrichment (input, output, tokenUsage).
// With no filter and no cursor it calls QueryTraces once, unless traces share a
// start time at the page's end, and fetches root span details in parallel.
// With a filter it looks back through the window in batches until the page fills,
// the window runs out, maxExaminedTraces traces have been examined,
// maxMCPHandshakesPerRequest handshakes have been fetched, or listLookBackBudget
// has passed.
// A cursor continues from an earlier page over the same whole window.
// A trace that can't be read is left out of the page.
// The request fails if it runs past requestTimeout, or if it could read none
// of the traces it examined.
func (c *TracingController) GetTraceOverviews(ctx context.Context, params TraceQueryParams) (*opensearch.TraceOverviewResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	var resp *opensearch.TraceOverviewResponse
	var stats walkStats
	var err error
	if params.Filters.IsZero() {
		resp, stats, err = c.traceOverviewPage(ctx, params)
	} else {
		resp, stats, err = c.lookBackForMatches(ctx, params, listLookBackBudget)
	}
	if err != nil {
		return nil, err
	}
	// Nothing could be read, which usually means the upstream is down.
	if stats.read == 0 && len(stats.failed) > 0 {
		return nil, fmt.Errorf("controllers.GetTraceOverviews: none of %d traces could be read", len(stats.failed))
	}

	logger.GetLogger(ctx).Info("Retrieved trace overviews",
		"organization", params.Organization,
		"filters", params.Filters,
		"cursor", params.Cursor != nil,
		"examined", stats.examined,
		"matched", len(resp.Traces),
		"truncated", resp.Truncated,
		"budgetExceeded", stats.budgetExceeded,
		"handshakeCapReached", stats.handshakeCapReached,
		"failed", len(stats.failed))

	return resp, nil
}

// traceOverviewPage serves an unfiltered list. It fetches the window up to
// the page's end plus one trace, which shows whether the page's last trace is
// settled, and skips traces not past the cursor. It fetches more only when
// the page is short or ends on an unsettled trace.
func (c *TracingController) traceOverviewPage(ctx context.Context, params TraceQueryParams) (*opensearch.TraceOverviewResponse, walkStats, error) {
	cur := params.Cursor
	asc := params.SortOrder == "asc"
	size := params.Limit
	if cur != nil {
		size = cur.Rank + params.Limit
	}
	for {
		fetchLimit := fetchSize(size)
		tracesResp, err := c.observerClient.QueryTraces(ctx, c.traceListRequest(params, fetchLimit))
		if err != nil {
			return nil, walkStats{}, err
		}
		traces := sortedTraces(tracesResp.Traces, asc)
		complete := len(traces) >= tracesResp.Total
		settled := settledLen(traces, complete)

		start := 0
		for start < len(traces) && !pastCursor(traces[start], cur, asc) {
			start++
		}
		end := min(start+params.Limit, len(traces))
		full := end-start == params.Limit
		// Without a cursor a short response still makes a page, so the first
		// page stays one call when traces rooted outside the window take slots.
		if cur == nil && !full && !complete {
			end = settled
		}

		resp := &opensearch.TraceOverviewResponse{TotalCount: tracesResp.Total}
		switch {
		case complete && end == len(traces):
			resp.LookedBackTo = formatCursor(windowEdge(params))
		case end > start && end <= settled && (full || cur == nil):
			last := traces[end-1]
			rank := end + rootlessSlots(fetchLimit, len(traces), tracesResp.Total)
			resp.LookedBackTo = formatCursor(last.StartTime)
			resp.NextCursor = TraceCursor{Rank: rank, Time: last.StartTime, ID: last.TraceID}.Encode()
		case fetchLimit < maxCursorDepth:
			size = 2 * fetchLimit
			continue
		default:
			// No cursor follows, so the page may end inside an unsettled group.
			end = min(start+params.Limit, len(traces))
			resp.LookedBackTo = formatCursor(walkStart(params))
			if end > start {
				resp.LookedBackTo = formatCursor(traces[end-1].StartTime)
			}
			resp.Truncated = true
		}
		page := traces[start:end]
		var failed []string
		resp.Traces, failed = c.enrichTraces(ctx, params, page, new(atomic.Int32))
		// A done ctx would leave out every trace it was reading.
		if err := ctx.Err(); err != nil {
			return nil, walkStats{}, fmt.Errorf("controllers.GetTraceOverviews: %w", err)
		}
		return resp, walkStats{examined: len(page), read: len(resp.Traces), failed: failed}, nil
	}
}

// lookBackForMatches walks the window in page order, keeping traces that
// match params.Filters. Beside the matches it reports the traces examined and
// read, whether budget stopped the walk, and the examined traces it couldn't read.
//
// Each fetch covers the whole window with a doubled limit and skips trace IDs
// already seen. The window is never narrowed: the Observer bounds each span's
// end time, so a narrowed end would drop traces that overlap it. A cursor
// skips traces not past it; they are not enriched or examined. Each fetch
// holds back its unsettled tail, so the next, bigger fetch returns those
// traces whole and in order, and the walk stops only on a settled trace.
//
// A chunk's traces that can't be read are enriched once more before the walk
// reaches them. Those that fail again are walked past like non-matches.
//
// Past budget the walk stops as the examine cap stops it, once a chunk has
// been examined. After the first chunk the walk's fetches run under budget,
// which cancels those in flight without a warning; the walk then stops before
// the chunk they belong to.
//
// Past maxMCPHandshakesPerRequest handshake fetches it stops the same way, before the next chunk and without a retry.
//
// With a score filter, each chunk's summary survivors are scored in one
// lookup before any upstream call, and those out of range are walked past
// like summary rejections. A failed lookup fails the walk, unless the budget
// cut it. Without a score client the walk fails before it starts.
func (c *TracingController) lookBackForMatches(ctx context.Context, params TraceQueryParams, budget time.Duration) (*opensearch.TraceOverviewResponse, walkStats, error) {
	if params.Filters.HasScoreFilter() && c.scores == nil {
		return nil, walkStats{}, fmt.Errorf("controllers.GetTraceOverviews: %w", ErrScoresNotConfigured)
	}
	walkCtx, cancel := c.budget(ctx, budget)
	defer cancel()

	params.Include = impliedInclude(params.Include, params.Filters)
	cur := params.Cursor
	asc := params.SortOrder == "asc"
	summaryOnly := params.Filters.SummaryOnly()
	matched := make([]opensearch.TraceOverview, 0, params.Limit)
	seen := make(map[string]struct{})
	// rootless is the latest fetch's slots spent on traces rooted outside the window.
	examined, read, skipped, rootless := 0, 0, 0, 0
	// last is the last trace examined, or where the walk began.
	last := observer.TraceInfo{StartTime: walkStart(params)}
	if cur != nil {
		last.TraceID = cur.ID
	}
	budgetExceeded := false
	handshakeCapReached := false
	// handshakes counts the walk's MCP handshake fetches.
	handshakes := new(atomic.Int32)
	var failed []string

	done := func(lookedBackTo time.Time, truncated, more bool) (*opensearch.TraceOverviewResponse, walkStats, error) {
		resp := &opensearch.TraceOverviewResponse{
			Traces:       matched,
			TotalCount:   len(matched),
			LookedBackTo: formatCursor(lookedBackTo),
			Truncated:    truncated,
		}
		if more {
			// A window that changed between fetches can push the count past the cap.
			rank := min(skipped+examined+rootless, maxCursorDepth)
			resp.NextCursor = TraceCursor{Rank: rank, Time: last.StartTime, ID: last.TraceID}.Encode()
		}
		return resp, walkStats{examined: examined, read: read, budgetExceeded: budgetExceeded,
			handshakeCapReached: handshakeCapReached, failed: failed}, nil
	}
	// pastBudget reports whether the budget is spent, never before the first chunk.
	pastBudget := func() bool {
		return examined > 0 && walkCtx.Err() != nil
	}
	// stopAtBudget ends the walk after the last examined trace.
	stopAtBudget := func() (*opensearch.TraceOverviewResponse, walkStats, error) {
		budgetExceeded = true
		return done(last.StartTime, true, true)
	}
	// pastHandshakeCap reports whether the walk has fetched maxMCPHandshakesPerRequest handshakes.
	pastHandshakeCap := func() bool {
		return handshakes.Load() >= maxMCPHandshakesPerRequest
	}
	// stopAtHandshakeCap ends the walk after the last examined trace.
	stopAtHandshakeCap := func() (*opensearch.TraceOverviewResponse, walkStats, error) {
		handshakeCapReached = true
		return done(last.StartTime, true, true)
	}
	// fetchCtx is ctx until the first chunk is examined, then walkCtx.
	fetchCtx := func() context.Context {
		if examined == 0 {
			return ctx
		}
		return walkCtx
	}

	// size is the traces the next fetch needs; it asks for one more.
	size := lookBackBatchSize
	if cur != nil {
		size = cur.Rank + lookBackBatchSize
	}
	for {
		if err := ctx.Err(); err != nil {
			return nil, walkStats{}, fmt.Errorf("controllers.GetTraceOverviews: %w", err)
		}
		if pastBudget() {
			return stopAtBudget()
		}
		if pastHandshakeCap() {
			return stopAtHandshakeCap()
		}
		fetchLimit := fetchSize(size)
		tracesResp, err := c.observerClient.QueryTraces(fetchCtx(), c.traceListRequest(params, fetchLimit))
		if err != nil {
			if ctx.Err() == nil && pastBudget() {
				return stopAtBudget()
			}
			return nil, walkStats{}, fmt.Errorf("controllers.GetTraceOverviews: %w", err)
		}
		rootless = rootlessSlots(fetchLimit, len(tracesResp.Traces), tracesResp.Total)

		// Total counts every trace in the window. A short response does not
		// mean the window ran out: the limit also counts traces whose root
		// span lies outside the window, which the Observer then drops.
		traces := sortedTraces(tracesResp.Traces, asc)
		complete := len(traces) >= tracesResp.Total
		settled := settledLen(traces, complete)
		fresh := make([]observer.TraceInfo, 0, settled)
		for _, t := range traces[:settled] {
			if _, ok := seen[t.TraceID]; ok {
				continue
			}
			seen[t.TraceID] = struct{}{}
			if !pastCursor(t, cur, asc) {
				skipped++
				continue
			}
			fresh = append(fresh, t)
		}

		// Enrich in chunks and walk in page order, so a full page stops at
		// its last match and the cursor never passes an unreturned match.
		for len(fresh) > 0 && examined < maxExaminedTraces {
			if err := ctx.Err(); err != nil {
				return nil, walkStats{}, fmt.Errorf("controllers.GetTraceOverviews: %w", err)
			}
			if pastBudget() {
				return stopAtBudget()
			}
			if pastHandshakeCap() {
				return stopAtHandshakeCap()
			}
			chunk := fresh[:min(lookBackBatchSize, len(fresh), maxExaminedTraces-examined)]
			// Summary-only survivors all match, so enrich no more than the page still needs.
			if summaryOnly {
				chunk = chunk[:summaryChunkLen(chunk, params.Filters, params.Limit-len(matched))]
			}
			fresh = fresh[len(chunk):]
			survivors := filterTraceInfos(chunk, params.Filters)
			chunkCtx := fetchCtx()
			var outOfRange map[string]bool
			if params.Filters.HasScoreFilter() {
				survivors, outOfRange, err = c.filterByScore(chunkCtx, params, survivors)
				if err != nil {
					if ctx.Err() != nil {
						return nil, walkStats{}, fmt.Errorf("controllers.GetTraceOverviews: %w", ctx.Err())
					}
					if pastBudget() {
						return stopAtBudget()
					}
					return nil, walkStats{}, fmt.Errorf("controllers.GetTraceOverviews: %w", err)
				}
			}
			overviews, chunkFailed := c.enrichTraces(chunkCtx, params, survivors, handshakes)
			if len(chunkFailed) > 0 && !pastBudget() && !pastHandshakeCap() {
				var retried []opensearch.TraceOverview
				retried, chunkFailed = c.enrichTraces(chunkCtx, params, tracesWithIDs(survivors, chunkFailed), handshakes)
				overviews = append(overviews, retried...)
			}
			// A cancelled request would report every trace it was reading as unread.
			if err := ctx.Err(); err != nil {
				return nil, walkStats{}, fmt.Errorf("controllers.GetTraceOverviews: %w", err)
			}
			// The budget may have cut the chunk's reads, so the walk stops before it.
			if pastBudget() {
				return stopAtBudget()
			}
			byID := make(map[string]opensearch.TraceOverview, len(overviews))
			for _, ov := range overviews {
				byID[ov.TraceID] = ov
			}
			for i, t := range chunk {
				examined++
				last = t
				ov, ok := byID[t.TraceID]
				switch {
				case ok:
					read++
				case slices.Contains(chunkFailed, t.TraceID):
					failed = append(failed, t.TraceID)
				case outOfRange[t.TraceID]:
					// Ruled out by score before any read.
				case matchesSummary(t.DurationNs, t.SpanCount, params.Filters):
					// Enrichment read it and ruled it out.
					read++
				}
				if !ok || !matchesFilters(ov, params.Filters) {
					continue
				}
				matched = append(matched, ov)
				if len(matched) == params.Limit {
					if i == len(chunk)-1 && len(fresh) == 0 && complete {
						return done(windowEdge(params), false, false)
					}
					return done(t.StartTime, false, true)
				}
			}
		}

		exhausted := len(fresh) == 0 && complete
		next := 2 * size
		// A fetch past skipped+maxExaminedTraces would only add traces the cap
		// rules out, unless an unsettled tail still needs a bigger fetch.
		if settled == len(traces) {
			next = min(next, skipped+maxExaminedTraces)
		}
		switch {
		case exhausted:
			return done(windowEdge(params), false, false)
		case examined >= maxExaminedTraces || (fetchSize(next) <= fetchLimit && fetchLimit < maxCursorDepth):
			return done(last.StartTime, true, true)
		case fetchSize(next) <= fetchLimit:
			// The cursor is too deep to fetch past.
			return done(last.StartTime, true, false)
		}
		size = next
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

// tracesWithIDs keeps the traces whose IDs are in ids, in order.
func tracesWithIDs(traces []observer.TraceInfo, ids []string) []observer.TraceInfo {
	kept := make([]observer.TraceInfo, 0, len(ids))
	for _, t := range traces {
		if slices.Contains(ids, t.TraceID) {
			kept = append(kept, t)
		}
	}
	return kept
}

// enrichVerdict says whether enrichment kept a trace, ruled it out, or couldn't read it.
type enrichVerdict int

const (
	enrichKept enrichVerdict = iota
	enrichRejected
	enrichFailed
)

// enrichTraces fetches root spans and enriches traces in parallel, returning
// overviews in input order. Traces whose root fails matchesStatus, or whose
// conversation ID, tools or models fail their filter, are skipped before the
// rest of the cascade.
// Traces that couldn't be read are skipped too and returned as failed: no
// root span ID, a failed root fetch, or a failed fetch a model, tool, minTokens
// or conversationId filter needs.
// When listSpansFirst holds, the root comes from the trace's attribute span
// list instead of its own fetch. With a tool or MCP server filter and no root
// filter, the span list comes before the root and a trace whose tools fail the
// filter never fetches its root. Without a model check pending, the MCP
// handshakes come before the root too. handshakes counts the handshake fetches.
func (c *TracingController) enrichTraces(
	ctx context.Context,
	params TraceQueryParams,
	traces []observer.TraceInfo,
	handshakes *atomic.Int32,
) (overviews []opensearch.TraceOverview, failed []string) {
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
		tools          []string
		failedTools    []string
		mcpServers     []string
		verdict        enrichVerdict
	}
	results := make([]result, len(traces))
	outerSem := make(chan struct{}, maxConcurrentTraces)
	innerSem := make(chan struct{}, maxConcurrentFetches)
	var wg sync.WaitGroup

	for i, t := range traces {
		if t.RootSpanID == "" {
			log.Warn("trace has no rootSpanId, skipping", "traceId", t.TraceID)
			results[i] = result{verdict: enrichFailed}
			continue
		}
		wg.Add(1)
		go func(idx int, t observer.TraceInfo) {
			defer wg.Done()
			outerSem <- struct{}{}
			defer func() { <-outerSem }()

			var spans []observer.SpanInfo
			var root *opensearch.Span
			mcp := mcpLookup{handshakes: handshakes}
			toolsFirst := c.toolListFirst(params, t)
			attrsFirst := c.listSpansFirst(params, t)
			switch {
			case attrsFirst:
				spans, root = c.rootFromSpanList(ctx, params, t, innerSem)
			case toolsFirst:
				spans = c.plainSpanList(ctx, params, t, innerSem)
			}
			if toolsFirst {
				// A nil list is a failed fetch, already warned about.
				if spans == nil {
					results[idx] = result{verdict: enrichFailed}
					return
				}
				if tools, failedTools := toolsFromSpanList(spans); !matchesTools(tools, failedTools, params.Filters) {
					results[idx] = result{verdict: enrichRejected}
					return
				}
				// An attribute list leaves a model check, which comes first.
				if params.Include.MCPServers && !attrsFirst {
					if verdict := c.readMCPServers(ctx, params, t, spans, innerSem, &mcp); verdict != enrichKept {
						results[idx] = result{verdict: verdict}
						return
					}
				}
			}
			if root == nil {
				innerSem <- struct{}{}
				details, err := c.observerClient.GetSpanDetails(ctx, t.TraceID, t.RootSpanID)
				<-innerSem
				if err != nil {
					// A fetch a done ctx cut short isn't warned about.
					if ctx.Err() == nil {
						log.Warn("failed to fetch root span details, skipping trace",
							"traceId", t.TraceID, "err", err)
					}
					results[idx] = result{verdict: enrichFailed}
					return
				}
				enriched := opensearch.ProcessSpan(observer.ConvertSpanDetailsToSpan(t.TraceID, details))
				root = &enriched
			}
			// Status is root-only by design.
			status := opensearch.ExtractTraceStatus([]opensearch.Span{*root})
			if !matchesStatus(status, params.Filters) {
				results[idx] = result{verdict: enrichRejected}
				return
			}
			input, output, tokens, models, conversationID, tools, failedTools, verdict := c.enrichTrace(ctx, params, t, root, spans, innerSem, &mcp)
			if verdict != enrichKept {
				results[idx] = result{verdict: verdict}
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
				tools:          tools,
				failedTools:    failedTools,
				mcpServers:     mcp.servers,
			}
		}(i, t)
	}
	wg.Wait()

	overviews = make([]opensearch.TraceOverview, 0, len(traces))
	for i, t := range traces {
		res := results[i]
		if res.verdict == enrichFailed {
			failed = append(failed, t.TraceID)
		}
		if res.verdict != enrichKept || res.span == nil {
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
			Tools:           res.tools,
			FailedTools:     res.failedTools,
			MCPServers:      res.mcpServers,
		})
	}
	return overviews, failed
}

// listSpansFirst reports whether t's span list is fetched with attributes
// whatever the root holds, so it can supply the root. Root filters keep the
// root fetch first, so a trace they reject never downloads its list.
func (c *TracingController) listSpansFirst(params TraceQueryParams, t observer.TraceInfo) bool {
	return params.Include.Models &&
		t.SpanCount <= skipLeafAggregationSpanCountThreshold &&
		params.Filters.Status == TraceStatusAny && params.Filters.ConversationID == ""
}

// toolListFirst reports whether a tool or MCP server filter judges t on its span
// list before its root is fetched. Root filters keep the root first, as for
// listSpansFirst, and so does a model filter that rejects t at its root for its
// span count.
func (c *TracingController) toolListFirst(params TraceQueryParams, t observer.TraceInfo) bool {
	return params.Filters.hasSpanListFilter() &&
		t.SpanCount <= maxToolListSpans &&
		params.Filters.Status == TraceStatusAny && params.Filters.ConversationID == "" &&
		(params.Filters.Model == "" || t.SpanCount <= skipLeafAggregationSpanCountThreshold)
}

// plainSpanList fetches t's span list without attributes. It is nil when the
// fetch fails.
func (c *TracingController) plainSpanList(
	ctx context.Context,
	params TraceQueryParams,
	t observer.TraceInfo,
	fetchSem chan struct{},
) []observer.SpanInfo {
	spans, ok := c.fetchTraceSpanSummaries(ctx, params, t, fetchSem, false)
	switch {
	case !ok:
		return nil
	case spans == nil:
		return []observer.SpanInfo{}
	}
	return spans
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

// enrichTraceOverview is enrichTrace without the MCP servers.
func (c *TracingController) enrichTraceOverview(
	ctx context.Context,
	params TraceQueryParams,
	traceInfo observer.TraceInfo,
	rootSpan *opensearch.Span,
	spans []observer.SpanInfo,
	fetchSem chan struct{},
) (input interface{}, output interface{}, tokenUsage *opensearch.TokenUsage, models []string, conversationID string, tools, failedTools []string, verdict enrichVerdict) {
	return c.enrichTrace(ctx, params, traceInfo, rootSpan, spans, fetchSem, &mcpLookup{handshakes: new(atomic.Int32)})
}

// enrichTrace computes Input/Output/Tokens/Models/ConversationID/Tools
// for one trace-list row, cascading through three sources in order of cost.
// enrichTraces calls it only for traces whose root passes matchesStatus, so a
// rejected trace costs just its root fetch:
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
// filter, the verdict is enrichRejected once the models rule the trace out,
// before steps 2 and 3; traces over the span threshold have no models and are
// rejected first. It is enrichFailed when a model or minTokens filter is set
// and the span list can't be fetched, or when the token count falls short of
// minTokens with leaves unread, since the filter can't judge the trace.
//
// The conversation ID is the first one found on the root, the step 2 child,
// then the step 3 leaves in start order. Only spans a step reads anyway are
// checked, so it costs no fetch. With a conversationId filter, a root without
// an ID doesn't end the cascade, even a complete one: steps 2 and 3 run to
// find it. The verdict is enrichRejected once an ID is found that doesn't
// match: on the root before any fetch, or on the child before step 3. A trace
// with no ID is kept here and ruled out by matchesFilters. It is enrichFailed
// when a span the ID may sit on goes unread: a failed span list, child or leaf
// ahead of the first ID, leaves skipped over the span threshold, or leaves cut
// off by the cap.
//
// When the span list carries attributes, steps 2 and 3 read their spans from
// it instead of fetching them. spans is the list when the caller already
// fetched it with attributes, or nil.
//
// With params.Include.Tools, tools and failedTools come from the span list's
// names and statuses (toolsFromSpanList), for traces of at most
// maxToolListSpans spans. They reuse the list the cascade reads; a trace with
// nothing left to fetch fetches it without attributes. A failed fetch leaves
// them nil and keeps the row, unless a tool filter is set: then the verdict is
// enrichFailed. With a tool filter, the verdict is enrichRejected as soon as the
// tools rule the trace out, right after the list and before the model check
// and steps 2 and 3.
//
// With params.Include.MCPServers, readMCPServers fills mcp from the same list,
// once, after the tool and model checks and before steps 2 and 3. A caller
// that already read them sets mcp.read.
//
// Token usage from traceloop.entity.output is used only when no step finds a
// gen_ai.usage.* report.
//
// fetchSem bounds the total observer fetches across all concurrent
// enrichments — callers pass a shared semaphore so a 50-trace page can't
// fan out into thousands of in-flight requests.
func (c *TracingController) enrichTrace(
	ctx context.Context,
	params TraceQueryParams,
	traceInfo observer.TraceInfo,
	rootSpan *opensearch.Span,
	spans []observer.SpanInfo,
	fetchSem chan struct{},
	mcp *mcpLookup,
) (input interface{}, output interface{}, tokenUsage *opensearch.TokenUsage, models []string, conversationID string, tools, failedTools []string, verdict enrichVerdict) {
	// Step 1: root span attributes.
	conversationID = opensearch.ExtractConversationID(rootSpan)
	if conversationRulesOut(conversationID, params.Filters) {
		return nil, nil, nil, nil, "", nil, nil, enrichRejected
	}
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
	toolsFromList := params.Include.Tools && traceInfo.SpanCount <= maxToolListSpans
	mcpFromList := params.Include.MCPServers && traceInfo.SpanCount <= maxToolListSpans
	rejectOnModel := params.Filters.Model != ""

	// Without leaf aggregation the trace has no models.
	if rejectOnModel && !aggregateLeaves {
		return nil, nil, nil, nil, "", nil, nil, enrichRejected
	}

	// Nothing left to fetch, unless tools or MCP servers need the list.
	if rootComplete && !modelsFromList && !conversationPending(conversationID, params.Filters) {
		if (toolsFromList || mcpFromList) && spans == nil {
			var ok bool
			if spans, ok = c.fetchTraceSpanSummaries(ctx, params, traceInfo, fetchSem, false); !ok && params.Filters.hasSpanListFilter() {
				return nil, nil, nil, nil, "", nil, nil, enrichFailed
			}
		}
		if toolsFromList {
			tools, failedTools = toolsFromSpanList(spans)
			if !matchesTools(tools, failedTools, params.Filters) {
				return nil, nil, nil, nil, "", nil, nil, enrichRejected
			}
		}
		if mcpFromList {
			if verdict := c.readMCPServers(ctx, params, traceInfo, spans, fetchSem, mcp); verdict != enrichKept {
				return nil, nil, nil, nil, "", nil, nil, verdict
			}
		}
		return input, output, tokenUsage, nil, conversationID, tools, failedTools, enrichKept
	}

	// Steps 2 and 3, inline models and tools all need the span list. Fetch it once.
	if spans == nil {
		var ok bool
		spans, ok = c.fetchTraceSpanSummaries(ctx, params, traceInfo, fetchSem, modelsFromList)
		if !ok {
			if rejectOnModel || params.Filters.hasSpanListFilter() || params.Filters.MinTokens != nil || conversationPending(conversationID, params.Filters) {
				return nil, nil, nil, nil, "", nil, nil, enrichFailed
			}
			return input, output, cmp.Or(tokenUsage, entityTokens), nil, conversationID, nil, nil, enrichKept
		}
	}
	if toolsFromList {
		tools, failedTools = toolsFromSpanList(spans)
		if !matchesTools(tools, failedTools, params.Filters) {
			return nil, nil, nil, nil, "", nil, nil, enrichRejected
		}
	}
	// The list carries attributes exactly when modelsFromList holds.
	if modelsFromList {
		models = modelsFromSpanList(traceInfo.TraceID, spans)
		if rejectOnModel && !matchesModel(models, params.Filters) {
			return nil, nil, nil, nil, "", nil, nil, enrichRejected
		}
	}
	if mcpFromList {
		if verdict := c.readMCPServers(ctx, params, traceInfo, spans, fetchSem, mcp); verdict != enrichKept {
			return nil, nil, nil, nil, "", nil, nil, verdict
		}
	}

	// Step 2: immediate child of the root (Traceloop chain span path).
	if !rootComplete || conversationPending(conversationID, params.Filters) {
		childInput, childOutput, childTokens, childEntityTokens, childConversationID, ok, childUnread := c.tryChildChainSpan(ctx, traceInfo.TraceID, rootSpan.SpanID, spans, modelsFromList, fetchSem)
		if ok {
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
			conversationID = cmp.Or(conversationID, childConversationID)
		}
		if conversationRulesOut(conversationID, params.Filters) {
			return nil, nil, nil, nil, "", nil, nil, enrichRejected
		}
		// The child that couldn't be read may hold the ID.
		if childUnread && conversationPending(conversationID, params.Filters) {
			return nil, nil, nil, nil, "", nil, nil, enrichFailed
		}
	}

	// Step 3: leaf LLM aggregation (OpenAI Agents SDK / pure-OTel path).
	stillMissing := input == nil || output == nil || tokenUsage == nil
	tokensUnread := false
	if stillMissing || conversationPending(conversationID, params.Filters) {
		if !aggregateLeaves {
			logger.GetLogger(ctx).Debug("skipping leaf-LLM aggregation: trace exceeds spanCount threshold",
				"traceId", traceInfo.TraceID,
				"spanCount", traceInfo.SpanCount,
				"threshold", skipLeafAggregationSpanCountThreshold)
			// The skipped leaves may hold the ID.
			if conversationPending(conversationID, params.Filters) {
				return nil, nil, nil, nil, "", nil, nil, enrichFailed
			}
		} else {
			leafInput, leafOutput, leafTokens, leafModels, leafConversationID, leavesUnread, leafIDUnread := c.aggregateFromLeafLLMSpans(ctx, traceInfo.TraceID, spans, modelsFromList, fetchSem)
			if models == nil {
				models = leafModels
			}
			// A leaf that couldn't be read ahead of the first ID may have held another.
			if leafIDUnread && conversationPending(conversationID, params.Filters) {
				return nil, nil, nil, nil, "", nil, nil, enrichFailed
			}
			conversationID = cmp.Or(conversationID, leafConversationID)
			if input == nil {
				input = leafInput
			}
			if output == nil {
				output = leafOutput
			}
			if tokenUsage == nil {
				tokenUsage = leafTokens
				tokensUnread = leavesUnread
			}
		}
	}

	tokenUsage = cmp.Or(tokenUsage, entityTokens)
	if tokensUnread && !matchesMinTokens(tokenUsage, params.Filters) {
		return nil, nil, nil, nil, "", nil, nil, enrichFailed
	}
	return input, output, tokenUsage, models, conversationID, tools, failedTools, enrichKept
}

// fetchTraceSpanSummaries calls QueryTraceSpans for one trace and returns
// the span-summary list, mirroring the request shape used elsewhere in this
// controller, with inline attributes when includeAttributes is set. Acquires a
// slot on fetchSem for the duration of the call so the call counts against the
// shared cross-trace fetch budget. Returns ok=false on error (logged as a
// warning unless ctx is done); callers fall back gracefully to whatever they
// already extracted.
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
		if ctx.Err() == nil {
			log.Warn("enrichTraceOverview: QueryTraceSpans failed, skipping enrichment",
				"traceId", traceInfo.TraceID, "err", err)
		}
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

// toolsFromSpanList returns the distinct non-empty tool names in a span list,
// in start-time order, and those with any span whose status is error. It reads
// span names and statuses only, even when the list carries attributes.
func toolsFromSpanList(spans []observer.SpanInfo) (tools, failedTools []string) {
	type toolCall struct {
		name   string
		start  time.Time
		failed bool
	}
	calls := make([]toolCall, 0)
	for _, s := range spans {
		if name, ok := opensearch.ToolNameFromSpanName(s.SpanName); ok && name != "" {
			calls = append(calls, toolCall{name: name, start: s.StartTime, failed: spanStatusIsError(s.Status)})
		}
	}
	sort.SliceStable(calls, func(i, j int) bool { return calls[i].start.Before(calls[j].start) })
	failed := make(map[string]bool, len(calls))
	for _, call := range calls {
		if _, seen := failed[call.name]; !seen {
			tools = append(tools, call.name)
		}
		failed[call.name] = failed[call.name] || call.failed
	}
	for _, name := range tools {
		if failed[name] {
			failedTools = append(failedTools, name)
		}
	}
	return tools, failedTools
}

// mcpLookup holds a trace's MCP servers once readMCPServers has read them.
type mcpLookup struct {
	servers []string
	read    bool
	// handshakes counts the request's handshake fetches.
	handshakes *atomic.Int32
}

// readMCPServers fills mcp from t's handshake spans unless it is already read.
// The verdict is enrichRejected when the servers fail the mcpServer filter, and
// enrichFailed when a handshake fetch fails under it. A trace with no handshake
// span costs no call.
func (c *TracingController) readMCPServers(
	ctx context.Context,
	params TraceQueryParams,
	t observer.TraceInfo,
	spans []observer.SpanInfo,
	fetchSem chan struct{},
	mcp *mcpLookup,
) enrichVerdict {
	if mcp.read {
		return enrichKept
	}
	servers, ok := c.mcpServersFromSpanList(ctx, t.TraceID, spans, fetchSem, mcp.handshakes)
	mcp.servers, mcp.read = servers, true
	switch {
	case !ok && params.Filters.MCPServer != "":
		return enrichFailed
	case !matchesMCPServer(servers, params.Filters):
		return enrichRejected
	}
	return enrichKept
}

// mcpServersFromSpanList returns the distinct servers the handshakesToFetch spans name; ok is false when a fetch fails.
func (c *TracingController) mcpServersFromSpanList(
	ctx context.Context,
	traceID string,
	spans []observer.SpanInfo,
	fetchSem chan struct{},
	fetched *atomic.Int32,
) (servers []string, ok bool) {
	handshakes := handshakesToFetch(spans)
	fetched.Add(int32(len(handshakes)))

	names := make([]string, len(handshakes))
	failed := make([]bool, len(handshakes))
	var wg sync.WaitGroup
	for i, s := range handshakes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fetchSem <- struct{}{}
			details, err := c.observerClient.GetSpanDetails(ctx, traceID, s.SpanID)
			<-fetchSem
			if err != nil {
				if ctx.Err() == nil {
					logger.GetLogger(ctx).Warn("failed to fetch MCP handshake span details",
						"traceId", traceID, "spanId", s.SpanID, "err", err)
				}
				failed[i] = true
				return
			}
			names[i] = opensearch.MCPServerFromHandshake(details.Attributes)
		}()
	}
	wg.Wait()

	for _, name := range names {
		if name != "" && !slices.Contains(servers, name) {
			servers = append(servers, name)
		}
	}
	return servers, !slices.Contains(failed, true)
}

// handshakesToFetch picks the first handshake per parent tool in start order, up to maxMCPHandshakesPerTrace.
func handshakesToFetch(spans []observer.SpanInfo) []observer.SpanInfo {
	var handshakes []observer.SpanInfo
	for _, s := range spans {
		if opensearch.IsMCPHandshakeSpan(s.SpanName) {
			handshakes = append(handshakes, s)
		}
	}
	if len(handshakes) == 0 {
		return nil
	}
	sort.SliceStable(handshakes, func(i, j int) bool { return handshakes[i].StartTime.Before(handshakes[j].StartTime) })

	names := make(map[string]string, len(spans))
	for _, s := range spans {
		names[s.SpanID] = s.SpanName
	}
	type key struct{ tool, span string }
	seen := make(map[key]bool, len(handshakes))
	picked := make([]observer.SpanInfo, 0, min(len(handshakes), maxMCPHandshakesPerTrace))
	for _, h := range handshakes {
		k := key{span: h.SpanID}
		if parent, ok := names[h.ParentSpanID]; ok {
			if tool, ok := opensearch.ToolNameFromSpanName(parent); ok && tool != "" {
				k = key{tool: tool}
			}
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		picked = append(picked, h)
		if len(picked) == maxMCPHandshakesPerTrace {
			break
		}
	}
	return picked
}

// tryChildChainSpan fetches the earliest immediate child of the root span
// and runs the same entity.input/output / entity.output token extractors
// against it. Covers LangChain / LangGraph agents where Traceloop emits the
// conversation summary on a chain span (e.g. LangGraph.workflow) right under
// an attribute-empty `invoke_agent` root.
//
// tokens comes from the span's gen_ai.usage.* attributes; entityTokens from
// its traceloop.entity.output; conversationID from ExtractConversationID.
// With inline set, the child comes from spans instead of a fetch.
//
// Returns ok=false when no immediate child is found or the fetch fails, and
// unread=true when the fetch fails.
func (c *TracingController) tryChildChainSpan(
	ctx context.Context,
	traceID string,
	rootSpanID string,
	spans []observer.SpanInfo,
	inline bool,
	fetchSem chan struct{},
) (input interface{}, output interface{}, tokens, entityTokens *opensearch.TokenUsage, conversationID string, ok, unread bool) {
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
		return nil, nil, nil, nil, "", false, false
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
			if ctx.Err() == nil {
				log.Warn("tryChildChainSpan: GetSpanDetails failed",
					"traceId", traceID, "childSpanId", childID, "err", err)
			}
			return nil, nil, nil, nil, "", false, true
		}
		childSpan = opensearch.ProcessSpan(observer.ConvertSpanDetailsToSpan(traceID, details))
	}

	input, output = opensearch.ExtractRootSpanInputOutput(&childSpan)
	tokens = opensearch.ExtractTokenUsage([]opensearch.Span{childSpan})
	entityTokens = opensearch.ExtractTokenUsageFromEntityOutput(&childSpan)
	return input, output, tokens, entityTokens, opensearch.ExtractConversationID(&childSpan), true, false
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
// marker. unread reports leaves left out, by the cap or a failed fetch.
// conversationID is the first one a leaf carries, in start order. idUnread
// reports a leaf that failed to fetch ahead of it, or, when there is none, any
// leaf that failed or was cut off by the cap.
func (c *TracingController) aggregateFromLeafLLMSpans(
	ctx context.Context,
	traceID string,
	spans []observer.SpanInfo,
	inline bool,
	fetchSem chan struct{},
) (input interface{}, output interface{}, tokens *opensearch.TokenUsage, models []string, conversationID string, unread, idUnread bool) {
	log := logger.GetLogger(ctx)

	// Filter to leaf LLM spans, ordered by start time.
	leaves := make([]observer.SpanInfo, 0)
	for _, s := range spans {
		if opensearch.IsLLMLeafSpan(s.SpanName) {
			leaves = append(leaves, s)
		}
	}
	if len(leaves) == 0 {
		return nil, nil, nil, nil, "", false, false
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
				if ctx.Err() == nil {
					log.Warn("aggregateFromLeafLLMSpans: GetSpanDetails failed for leaf",
						"traceId", traceID, "spanId", spanID, "err", err)
				}
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
	unread = len(validLeaves) < totalLeaves
	for i := range fetched {
		if fetched[i].SpanID == "" {
			idUnread = true
			continue
		}
		if conversationID = opensearch.ExtractConversationID(&fetched[i]); conversationID != "" {
			break
		}
	}
	// A leaf past the cap may hold the ID.
	if partial && conversationID == "" {
		idUnread = true
	}
	if len(validLeaves) == 0 {
		return nil, nil, nil, nil, "", unread, idUnread
	}

	models = opensearch.ExtractModels(validLeaves)
	tokens = opensearch.ExtractTokenUsage(validLeaves)
	if tokens != nil && (partial || unread) {
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
	return input, output, tokens, models, conversationID, unread, idUnread
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
// The traces come from selectExportTraces; each then costs one QueryTraceSpans
// (spans carry attributes inline via includeAttributes). Concurrency is bounded
// by maxConcurrentTraces outer goroutines. A failed QueryTraceSpans is retried
// once; a trace that still can't be read, or has no root span, is left out and
// listed in FailedTraceIDs. The export fails when ctx is done, it runs past
// requestTimeout, or no selected trace could be read.
func (c *TracingController) ExportTraces(ctx context.Context, params TraceQueryParams) (*opensearch.TraceExportResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	log := logger.GetLogger(ctx)

	traces, resp, err := c.selectExportTraces(ctx, params)
	if err != nil {
		return nil, err
	}

	if len(traces) == 0 {
		resp.Traces = []opensearch.FullTrace{}
		return resp, nil
	}

	type traceResult struct {
		idx       int
		fullTrace *opensearch.FullTrace
		err       error
	}

	results := make([]traceResult, len(traces))
	var truncated atomic.Bool

	outerSem := make(chan struct{}, maxConcurrentTraces)
	var wg sync.WaitGroup

	for i, t := range traces {
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

			spansReq := observer.TracesQueryRequest{
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
			}
			spansResp, err := c.observerClient.QueryTraceSpans(ctx, traceInfo.TraceID, spansReq)
			if err != nil && ctx.Err() == nil {
				spansResp, err = c.observerClient.QueryTraceSpans(ctx, traceInfo.TraceID, spansReq)
			}
			if err != nil {
				results[idx] = traceResult{idx: idx, err: fmt.Errorf("trace %s: query spans: %w", traceInfo.TraceID, err)}
				return
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
			// A retry would return the same spans.
			if rootSpan == nil {
				results[idx] = traceResult{idx: idx, err: fmt.Errorf("trace %s: no root span found", traceInfo.TraceID)}
				return
			}

			if traceInfo.SpanCount > MaxSpansPerRequest {
				truncated.Store(true)
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

	// A cancelled export fails rather than listing every trace it was reading.
	if err := ctx.Err(); err != nil {
		return nil, fmt.Errorf("controllers.ExportTraces: %w", err)
	}

	fullTraces := make([]opensearch.FullTrace, 0, len(results))
	var firstErr error
	for i, r := range results {
		if r.err != nil {
			log.Warn("failed to read trace for export, leaving it out", "traceId", traces[i].TraceID, "err", r.err)
			resp.FailedTraceIDs = append(resp.FailedTraceIDs, traces[i].TraceID)
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		if r.fullTrace != nil {
			fullTraces = append(fullTraces, *r.fullTrace)
		}
	}
	// Nothing could be read, which usually means the upstream is down.
	if len(fullTraces) == 0 && firstErr != nil {
		return nil, firstErr
	}

	resp.Traces = fullTraces
	// A filtered export counts what it exported.
	if !params.Filters.IsZero() {
		resp.TotalCount = len(fullTraces)
	}
	resp.SpansTruncated = truncated.Load()
	resp.Truncated = resp.Truncated || resp.SpansTruncated

	log.Info("Completed trace export",
		"organization", params.Organization,
		"filters", params.Filters,
		"totalCount", resp.TotalCount,
		"exported", len(fullTraces),
		"truncated", resp.Truncated,
		"spansTruncated", resp.SpansTruncated,
		"failed", len(resp.FailedTraceIDs))

	return resp, nil
}

// selectExportTraces picks the traces to export. Without a filter it calls
// QueryTraces once. With one it selects matches the way a filtered list does,
// from the start of the window, so the examine cap, maxMCPHandshakesPerRequest
// and exportLookBackBudget apply, and lists the examined traces it couldn't read
// in FailedTraceIDs.
func (c *TracingController) selectExportTraces(ctx context.Context, params TraceQueryParams) ([]observer.TraceInfo, *opensearch.TraceExportResponse, error) {
	if params.Filters.IsZero() {
		tracesResp, err := c.observerClient.QueryTraces(ctx, c.traceListRequest(params, params.Limit))
		if err != nil {
			return nil, nil, err
		}
		return tracesResp.Traces, &opensearch.TraceExportResponse{TotalCount: tracesResp.Total}, nil
	}

	params.Cursor = nil
	page, stats, err := c.lookBackForMatches(ctx, params, exportLookBackBudget)
	if err != nil {
		return nil, nil, fmt.Errorf("controllers.ExportTraces: %w", err)
	}
	traces := make([]observer.TraceInfo, 0, len(page.Traces))
	for _, ov := range page.Traces {
		t, err := traceInfoOf(ov)
		if err != nil {
			return nil, nil, fmt.Errorf("controllers.ExportTraces: %w", err)
		}
		traces = append(traces, t)
	}
	logger.GetLogger(ctx).Info("Selected traces for export",
		"organization", params.Organization,
		"filters", params.Filters,
		"examined", stats.examined,
		"matched", len(traces),
		"truncated", page.Truncated,
		"budgetExceeded", stats.budgetExceeded,
		"handshakeCapReached", stats.handshakeCapReached,
		"failed", len(stats.failed))
	return traces, &opensearch.TraceExportResponse{
		TotalCount:     page.TotalCount,
		LookedBackTo:   page.LookedBackTo,
		Truncated:      page.Truncated,
		FailedTraceIDs: stats.failed,
	}, nil
}

// traceInfoOf rebuilds the trace-list fields export needs from an overview.
func traceInfoOf(ov opensearch.TraceOverview) (observer.TraceInfo, error) {
	start, err := time.Parse(time.RFC3339Nano, ov.StartTime)
	if err != nil {
		return observer.TraceInfo{}, fmt.Errorf("trace %s: start time: %w", ov.TraceID, err)
	}
	end, err := time.Parse(time.RFC3339Nano, ov.EndTime)
	if err != nil {
		return observer.TraceInfo{}, fmt.Errorf("trace %s: end time: %w", ov.TraceID, err)
	}
	return observer.TraceInfo{
		TraceID:      ov.TraceID,
		RootSpanID:   ov.RootSpanID,
		RootSpanName: ov.RootSpanName,
		StartTime:    start,
		EndTime:      end,
		DurationNs:   ov.DurationInNanos,
		SpanCount:    ov.SpanCount,
	}, nil
}
