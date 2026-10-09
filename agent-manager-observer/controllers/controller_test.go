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
	"fmt"
	"math/rand/v2"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
)

// fakeObserverClient is a minimal observer.Client implementation that the
// cascade tests use to script trace-overview enrichment scenarios. Each test
// installs the spans for a single trace; getSpanDetailsCalls counts how many
// GetSpanDetails calls the cascade made so we can verify the cost guards.
type fakeObserverClient struct {
	// rootSpan is returned by GetSpanDetails when called for the root span.
	rootSpan *observer.SpanDetailsResponse
	// traces is the list returned by QueryTraces (export path).
	traces []observer.TraceInfo
	// spans is the QueryTraceSpans result; attributes are stripped unless requested.
	spans []observer.SpanInfo
	// spansByTrace, when set, gives each trace its own QueryTraceSpans result.
	spansByTrace map[string][]observer.SpanInfo
	// spanDetails maps spanID → detail response for GetSpanDetails lookups
	// (excluding root, which is rootSpan).
	spanDetails map[string]*observer.SpanDetailsResponse

	// mu guards lastSpansReq, spansTraceIDs, attrSpansTraceIDs and detailSpanIDs.
	mu sync.Mutex
	// lastSpansReq records the request passed to the most recent
	// QueryTraceSpans call so export tests can assert IncludeAttributes.
	lastSpansReq observer.TracesQueryRequest
	// spansTraceIDs records the trace ID of every QueryTraceSpans call.
	spansTraceIDs []string
	// attrSpansTraceIDs records the trace ID of every QueryTraceSpans call with IncludeAttributes.
	attrSpansTraceIDs []string
	// detailSpanIDs records the span ID of every GetSpanDetails call.
	detailSpanIDs []string

	// windowed makes QueryTraces apply the request's window, sort order and
	// limit the way the upstream Observer does.
	windowed bool
	// shuffleTies makes QueryTraces return traces with the same start time in
	// a different order on every call, before the limit cuts them.
	shuffleTies bool
	// onQueryTraces runs at the start of each QueryTraces call.
	onQueryTraces func()
	// onGetSpanDetails runs at the start of each GetSpanDetails call, which
	// enrichment makes concurrently.
	onGetSpanDetails func(ctx context.Context, spanID string)
	// onCall runs with the ctx of every QueryTraces, QueryTraceSpans and GetSpanDetails call.
	onCall func(ctx context.Context)
	// failCall runs after each GetSpanDetails call (with its span ID) and
	// QueryTraceSpans call (with its trace ID) is recorded; an error fails it.
	failCall func(id string) error
	// failOnDone fails each GetSpanDetails and QueryTraceSpans call whose ctx
	// is done by the time it returns, as the HTTP client does.
	failOnDone bool
	// tracesReqs records every QueryTraces request.
	tracesReqs []observer.TracesQueryRequest

	getSpanDetailsCalls  int32
	queryTraceSpansCalls int32
	queryTracesCalls     int32
	// attrSpansCalls counts QueryTraceSpans calls with IncludeAttributes.
	attrSpansCalls int32

	// defaultNamespace is returned by NamespaceFor, mirroring the real client.
	defaultNamespace string
}

// QueryTraces records the request and returns the configured traces, windowed like upstream when windowed is set.
func (f *fakeObserverClient) QueryTraces(ctx context.Context, req observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	if f.onCall != nil {
		f.onCall(ctx)
	}
	calls := atomic.AddInt32(&f.queryTracesCalls, 1)
	f.tracesReqs = append(f.tracesReqs, req)
	if f.onQueryTraces != nil {
		f.onQueryTraces()
	}
	if !f.windowed {
		return &observer.TracesQueryResponse{Traces: f.traces, Total: len(f.traces)}, nil
	}

	// Each trace is a root span [StartTime, EndTime] and a zero-length child
	// span at StartTime. As upstream, a span is in the window when it starts
	// at or after req.StartTime and ends at or before req.EndTime; the limit
	// counts traces with any span in the window, and traces whose root is
	// outside it are dropped afterwards.
	inWindow := func(start, end time.Time) bool {
		return !start.Before(req.StartTime) && !end.After(req.EndTime)
	}
	type bucket struct {
		info    observer.TraceInfo
		hasRoot bool
	}
	buckets := make([]bucket, 0, len(f.traces))
	total := 0
	for _, t := range f.traces {
		hasRoot := inWindow(t.StartTime, t.EndTime)
		if hasRoot || inWindow(t.StartTime, t.StartTime) {
			buckets = append(buckets, bucket{info: t, hasRoot: hasRoot})
		}
		if hasRoot {
			total++
		}
	}
	asc := req.SortOrder != nil && *req.SortOrder == "asc"
	sort.SliceStable(buckets, func(i, j int) bool {
		a, b := buckets[i].info, buckets[j].info
		if !a.StartTime.Equal(b.StartTime) {
			return a.StartTime.Before(b.StartTime) == asc
		}
		return a.TraceID < b.TraceID
	})
	if f.shuffleTies {
		rng := rand.New(rand.NewPCG(uint64(calls), 0))
		for i := 0; i < len(buckets); {
			j := i + 1
			for j < len(buckets) && buckets[j].info.StartTime.Equal(buckets[i].info.StartTime) {
				j++
			}
			rng.Shuffle(j-i, func(a, b int) { buckets[i+a], buckets[i+b] = buckets[i+b], buckets[i+a] })
			i = j
		}
	}
	if req.Limit != nil && len(buckets) > *req.Limit {
		buckets = buckets[:*req.Limit]
	}
	traces := make([]observer.TraceInfo, 0, len(buckets))
	for _, b := range buckets {
		if b.hasRoot {
			traces = append(traces, b.info)
		}
	}
	return &observer.TracesQueryResponse{Traces: traces, Total: total}, nil
}

// QueryTraceSpans returns the configured spans for traceID and records the call.
func (f *fakeObserverClient) QueryTraceSpans(ctx context.Context, traceID string, req observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	if f.onCall != nil {
		f.onCall(ctx)
	}
	atomic.AddInt32(&f.queryTraceSpansCalls, 1)
	if req.IncludeAttributes {
		atomic.AddInt32(&f.attrSpansCalls, 1)
	}
	f.mu.Lock()
	f.lastSpansReq = req
	f.spansTraceIDs = append(f.spansTraceIDs, traceID)
	if req.IncludeAttributes {
		f.attrSpansTraceIDs = append(f.attrSpansTraceIDs, traceID)
	}
	f.mu.Unlock()
	if f.failOnDone && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if f.failCall != nil {
		if err := f.failCall(traceID); err != nil {
			return nil, err
		}
	}
	spans := f.spans
	if f.spansByTrace != nil {
		spans = f.spansByTrace[traceID]
	}
	if !req.IncludeAttributes {
		all := spans
		spans = make([]observer.SpanInfo, len(all))
		for i, s := range all {
			s.Attributes = nil
			s.ResourceAttributes = nil
			spans[i] = s
		}
	}
	return &observer.TraceSpansQueryResponse{Spans: spans, Total: len(spans)}, nil
}

func (f *fakeObserverClient) NamespaceFor(_ string) string {
	return f.defaultNamespace
}

func (f *fakeObserverClient) QueryLogs(_ context.Context, _ observer.LogsQueryRequest) (*observer.LogsQueryResponse, error) {
	return &observer.LogsQueryResponse{}, nil
}

func (f *fakeObserverClient) QueryMetrics(_ context.Context, _ observer.MetricsQueryRequest) (*observer.ResourceMetricsTimeSeries, error) {
	return &observer.ResourceMetricsTimeSeries{}, nil
}

// GetSpanDetails returns the configured details for spanID and records the call.
func (f *fakeObserverClient) GetSpanDetails(ctx context.Context, _, spanID string) (*observer.SpanDetailsResponse, error) {
	if f.onCall != nil {
		f.onCall(ctx)
	}
	atomic.AddInt32(&f.getSpanDetailsCalls, 1)
	if f.onGetSpanDetails != nil {
		f.onGetSpanDetails(ctx, spanID)
	}
	f.mu.Lock()
	f.detailSpanIDs = append(f.detailSpanIDs, spanID)
	f.mu.Unlock()
	if f.failOnDone && ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if f.failCall != nil {
		if err := f.failCall(spanID); err != nil {
			return nil, err
		}
	}
	if f.rootSpan != nil && spanID == f.rootSpan.SpanID {
		return f.rootSpan, nil
	}
	if d, ok := f.spanDetails[spanID]; ok {
		return d, nil
	}
	return nil, fmt.Errorf("fake: span %s not found", spanID)
}

// makeRootSpan constructs an opensearch.Span suitable for passing to
// enrichTraceOverview as the rootSpan argument.
func makeRootSpan(spanID string, attrs map[string]interface{}) *opensearch.Span {
	return &opensearch.Span{
		SpanID:     spanID,
		Name:       "invoke_agent LangGraph",
		Attributes: attrs,
	}
}

// baseTraceInfo returns a TraceInfo with sane defaults; tests override
// SpanCount where they need to exercise the cost guard.
func baseTraceInfo(spanCount int) observer.TraceInfo {
	return observer.TraceInfo{
		TraceID:    "trace-1",
		RootSpanID: "root",
		StartTime:  time.Now().Add(-1 * time.Hour),
		EndTime:    time.Now(),
		SpanCount:  spanCount,
	}
}

func baseParams() TraceQueryParams {
	project := "proj"
	component := "comp"
	environment := "env"
	return TraceQueryParams{
		Organization: "ns",
		Project:      &project,
		Agent:        &component,
		Environment:  &environment,
		StartTime:    time.Now().Add(-1 * time.Hour),
		EndTime:      time.Now(),
		Limit:        50,
		SortOrder:    "desc",
	}
}

// testFetchSem returns a fresh fetch-budget semaphore sized for tests. The
// cascade's per-trace fetches share this channel in production; tests give
// each call its own channel for isolation.
func testFetchSem() chan struct{} { return make(chan struct{}, maxConcurrentFetches) }

// Root span carries entity.input/output and its own gen_ai.usage.*
// report → the cascade short-circuits at step 1 with no extra fetches.
func TestEnrichTraceOverview_RootHasEntityAndUsageShortCircuits(t *testing.T) {
	root := makeRootSpan("root", map[string]interface{}{
		"traceloop.entity.input":     `{"inputs":"hello there"}`,
		"traceloop.entity.output":    `{"outputs":{"messages":[{"kwargs":{"content":"hi back"}}]}}`,
		"gen_ai.usage.input_tokens":  float64(10),
		"gen_ai.usage.output_tokens": float64(3),
	})
	fake := &fakeObserverClient{rootSpan: &observer.SpanDetailsResponse{SpanID: "root"}}
	c := NewTracingController(fake)

	input, output, tokens, _, _, _, _, _ := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(5), root, nil, testFetchSem())

	if input == nil || output == nil {
		t.Errorf("expected input/output from root, got input=%v output=%v", input, output)
	}
	if tokens == nil || tokens.TotalTokens != 13 {
		t.Errorf("expected tokens from root, got %+v", tokens)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("expected no extra GetSpanDetails calls, got %d", got)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 0 {
		t.Errorf("expected no QueryTraceSpans calls, got %d", got)
	}
}

// Root entity.output carries token_usage but the trace has no leaf LLM
// spans → the entity.output usage is used as the fallback.
func TestEnrichTraceOverview_RootEntityTokensUsedWhenNoLeaves(t *testing.T) {
	root := makeRootSpan("root", map[string]interface{}{
		"traceloop.entity.input":  `{"inputs":"hello there"}`,
		"traceloop.entity.output": `{"outputs":{"messages":[{"kwargs":{"content":"hi back","response_metadata":{"token_usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}}}]}}`,
	})
	fake := &fakeObserverClient{rootSpan: &observer.SpanDetailsResponse{SpanID: "root"}}
	c := NewTracingController(fake)

	_, _, tokens, _, _, _, _, _ := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(5), root, nil, testFetchSem())

	if tokens == nil || tokens.TotalTokens != 13 {
		t.Errorf("expected entity.output fallback tokens, got %+v", tokens)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("expected no GetSpanDetails calls (no child, no leaves), got %d", got)
	}
}

// Root span empty, but the immediate child chain span carries
// traceloop.entity.input/output (LangGraph pattern). Step 2 must fill it in.
func TestEnrichTraceOverview_FallsBackToChildChainSpan(t *testing.T) {
	root := makeRootSpan("root", map[string]interface{}{})
	// Two children of root: a non-chain span starting later, and the chain
	// span starting first. tryChildChainSpan picks the earliest.
	startEarly := time.Now().Add(-10 * time.Minute)
	startLate := time.Now().Add(-5 * time.Minute)
	fake := &fakeObserverClient{
		spans: []observer.SpanInfo{
			{SpanID: "chain-1", SpanName: "LangGraph.workflow", ParentSpanID: "root", StartTime: startEarly},
			{SpanID: "other", SpanName: "execute_task something", ParentSpanID: "root", StartTime: startLate},
		},
		spanDetails: map[string]*observer.SpanDetailsResponse{
			"chain-1": {
				SpanID: "chain-1", SpanName: "LangGraph.workflow",
				Attributes: map[string]interface{}{
					"traceloop.entity.input":  `{"inputs":"chain in"}`,
					"traceloop.entity.output": `{"outputs":{"messages":[{"kwargs":{"content":"chain out","response_metadata":{"token_usage":{"prompt_tokens":20,"completion_tokens":5,"total_tokens":25}}}}]}}`,
				},
			},
		},
	}
	c := NewTracingController(fake)

	input, output, tokens, _, _, _, _, _ := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(5), root, nil, testFetchSem())

	if input == nil || output == nil {
		t.Errorf("expected input/output from chain child, got input=%v output=%v", input, output)
	}
	if tokens == nil || tokens.TotalTokens != 25 {
		t.Errorf("expected tokens from chain child, got %+v", tokens)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 1 {
		t.Errorf("expected exactly 1 GetSpanDetails (child), got %d", got)
	}
}

// Root and child empty, but leaf LLM spans carry gen_ai.input.messages /
// gen_ai.output.messages / gen_ai.usage.* — the OpenAI Agents SDK case. Step
// 3 aggregates: token sum across all leaves, first-leaf input, last-leaf output.
func TestEnrichTraceOverview_AggregatesFromLeafLLMSpans(t *testing.T) {
	root := makeRootSpan("root", map[string]interface{}{})
	start := time.Now().Add(-10 * time.Minute)
	fake := &fakeObserverClient{
		spans: []observer.SpanInfo{
			{SpanID: "leaf-1", SpanName: "openai.chat", ParentSpanID: "root", StartTime: start},
			{SpanID: "leaf-2", SpanName: "openai.chat", ParentSpanID: "root", StartTime: start.Add(1 * time.Minute)},
		},
		spanDetails: map[string]*observer.SpanDetailsResponse{
			"leaf-1": {
				SpanID: "leaf-1", SpanName: "openai.chat",
				Attributes: map[string]interface{}{
					"gen_ai.input.messages":      `[{"role":"user","parts":[{"type":"text","content":"first user msg"}]}]`,
					"gen_ai.output.messages":     `[{"role":"assistant","parts":[{"type":"text","content":"first assistant"}]}]`,
					"gen_ai.usage.input_tokens":  float64(100),
					"gen_ai.usage.output_tokens": float64(20),
				},
			},
			"leaf-2": {
				SpanID: "leaf-2", SpanName: "openai.chat",
				Attributes: map[string]interface{}{
					"gen_ai.input.messages":      `[{"role":"user","parts":[{"type":"text","content":"second user msg"}]}]`,
					"gen_ai.output.messages":     `[{"role":"assistant","parts":[{"type":"text","content":"final answer"}]}]`,
					"gen_ai.usage.input_tokens":  float64(50),
					"gen_ai.usage.output_tokens": float64(10),
				},
			},
		},
	}
	c := NewTracingController(fake)

	input, output, tokens, _, _, _, _, _ := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(5), root, nil, testFetchSem())

	if input != "first user msg" {
		t.Errorf("input = %v, want first user msg", input)
	}
	if output != "final answer" {
		t.Errorf("output = %v, want last assistant msg", output)
	}
	if tokens == nil || tokens.InputTokens != 150 || tokens.OutputTokens != 30 || tokens.TotalTokens != 180 {
		t.Errorf("tokens = %+v, want sum across leaves (150/30/180)", tokens)
	}
	if tokens.Partial {
		t.Errorf("expected Partial=false (under cap), got true")
	}
}

// LangGraph trace with three LLM calls: tokens are the sum of the leaf LLM
// spans, not the usage in LangGraph.workflow's entity.output.
func TestEnrichTraceOverview_LangGraphSumsLeavesOverEntityOutput(t *testing.T) {
	root := makeRootSpan("root", map[string]interface{}{})
	start := time.Now().Add(-10 * time.Minute)
	aiMsg := func(in, out int) string {
		return fmt.Sprintf(`{"kwargs":{"type":"ai","content":"a","usage_metadata":{"input_tokens":%d,"output_tokens":%d,"total_tokens":%d}}}`, in, out, in+out)
	}
	workflowOutput := `{"outputs":{"messages":[` +
		`{"kwargs":{"type":"human","content":"earlier turn"}},` + aiMsg(2933, 26) + `,` +
		`{"kwargs":{"type":"human","content":"reset my password"}},` +
		aiMsg(2982, 31) + `,` + aiMsg(3086, 92) + `,` + aiMsg(3285, 63) + `]}}`

	leafUsage := [][2]int{{2982, 31}, {3086, 92}, {3285, 63}}
	spans := []observer.SpanInfo{
		{SpanID: "workflow", SpanName: "LangGraph.workflow", ParentSpanID: "root", StartTime: start},
	}
	details := map[string]*observer.SpanDetailsResponse{
		"workflow": {
			SpanID: "workflow", SpanName: "LangGraph.workflow",
			Attributes: map[string]interface{}{
				"traceloop.entity.input":  `{"inputs":"reset my password"}`,
				"traceloop.entity.output": workflowOutput,
			},
		},
	}
	for i, u := range leafUsage {
		id := fmt.Sprintf("chat-%d", i)
		seq := fmt.Sprintf("seq-%d", i)
		spans = append(spans, observer.SpanInfo{
			SpanID: id, SpanName: "ChatOpenAI.chat", ParentSpanID: seq,
			StartTime: start.Add(time.Duration(i+1) * time.Second),
		})
		details[id] = &observer.SpanDetailsResponse{
			SpanID: id, SpanName: "ChatOpenAI.chat", ParentSpanID: seq,
			Attributes: map[string]interface{}{
				"gen_ai.usage.input_tokens":  float64(u[0]),
				"gen_ai.usage.output_tokens": float64(u[1]),
			},
		}
	}
	fake := &fakeObserverClient{spans: spans, spanDetails: details}
	c := NewTracingController(fake)

	input, output, tokens, _, _, _, _, _ := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(24), root, nil, testFetchSem())

	if input == nil || output == nil {
		t.Errorf("expected input/output from workflow span, got input=%v output=%v", input, output)
	}
	if tokens == nil || tokens.InputTokens != 9353 || tokens.OutputTokens != 186 || tokens.TotalTokens != 9539 {
		t.Errorf("tokens = %+v, want sum of the three LLM calls (9353/186/9539)", tokens)
	}
}

// Everything empty — degenerate case. No extra fetches beyond QueryTraceSpans.
// All three return nil; trace overview row simply renders "-" in the UI.
func TestEnrichTraceOverview_AllEmptyReturnsNil(t *testing.T) {
	root := makeRootSpan("root", map[string]interface{}{})
	fake := &fakeObserverClient{spans: []observer.SpanInfo{}}
	c := NewTracingController(fake)

	input, output, tokens, _, _, _, _, _ := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(1), root, nil, testFetchSem())

	if input != nil || output != nil || tokens != nil {
		t.Errorf("expected all nil, got input=%v output=%v tokens=%+v", input, output, tokens)
	}
}

// Leaf cap honoured. With > maxLLMLeavesPerTrace leaves, only the cap is
// fetched and TokenUsage.Partial is true.
func TestEnrichTraceOverview_LeafCapHonoredAndPartialFlagged(t *testing.T) {
	root := makeRootSpan("root", map[string]interface{}{})
	const totalLeaves = maxLLMLeavesPerTrace + 5

	spans := make([]observer.SpanInfo, 0, totalLeaves)
	details := make(map[string]*observer.SpanDetailsResponse, totalLeaves)
	start := time.Now().Add(-1 * time.Hour)
	for i := 0; i < totalLeaves; i++ {
		id := fmt.Sprintf("leaf-%02d", i)
		spans = append(spans, observer.SpanInfo{
			SpanID: id, SpanName: "openai.chat", ParentSpanID: "root",
			StartTime: start.Add(time.Duration(i) * time.Second),
		})
		details[id] = &observer.SpanDetailsResponse{
			SpanID: id, SpanName: "openai.chat",
			Attributes: map[string]interface{}{
				"gen_ai.input.messages":      `[{"role":"user","content":"u"}]`,
				"gen_ai.output.messages":     `[{"role":"assistant","content":"a"}]`,
				"gen_ai.usage.input_tokens":  float64(1),
				"gen_ai.usage.output_tokens": float64(1),
			},
		}
	}
	fake := &fakeObserverClient{spans: spans, spanDetails: details}
	c := NewTracingController(fake)

	// SpanCount stays clearly under the skip threshold so step 3 runs; the
	// cap (maxLLMLeavesPerTrace) is what should trigger Partial. Using
	// threshold-1 expresses "below the skip threshold" independently of
	// whether the guard ever tightens from > to >=.
	_, _, tokens, _, _, _, _, _ := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(skipLeafAggregationSpanCountThreshold-1), root, nil, testFetchSem())

	if tokens == nil {
		t.Fatalf("expected tokens, got nil")
	}
	if !tokens.Partial {
		t.Errorf("expected Partial=true when leaves exceed cap")
	}
	if tokens.TotalTokens != maxLLMLeavesPerTrace*2 {
		t.Errorf("expected sum from %d leaves (cap), got TotalTokens=%d", maxLLMLeavesPerTrace, tokens.TotalTokens)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != int32(maxLLMLeavesPerTrace) {
		t.Errorf("expected exactly %d GetSpanDetails fetches, got %d", maxLLMLeavesPerTrace, got)
	}
}

// A leaf whose details fetch fails is left out of the sum, and the usage is
// flagged Partial because it no longer covers every selected leaf.
func TestEnrichTraceOverview_FailedLeafFetchFlagsPartial(t *testing.T) {
	root := makeRootSpan("root", map[string]interface{}{})
	start := time.Now().Add(-10 * time.Minute)
	fake := &fakeObserverClient{
		spans: []observer.SpanInfo{
			{SpanID: "leaf-ok", SpanName: "openai.chat", ParentSpanID: "root", StartTime: start},
			{SpanID: "leaf-missing", SpanName: "openai.chat", ParentSpanID: "root", StartTime: start.Add(1 * time.Minute)},
		},
		spanDetails: map[string]*observer.SpanDetailsResponse{
			"leaf-ok": {
				SpanID: "leaf-ok", SpanName: "openai.chat",
				Attributes: map[string]interface{}{
					"gen_ai.usage.input_tokens":  float64(100),
					"gen_ai.usage.output_tokens": float64(20),
				},
			},
		},
	}
	c := NewTracingController(fake)

	_, _, tokens, _, _, _, _, _ := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(5), root, nil, testFetchSem())

	if tokens == nil {
		t.Fatalf("expected tokens, got nil")
	}
	if tokens.InputTokens != 100 || tokens.OutputTokens != 20 || tokens.TotalTokens != 120 {
		t.Errorf("tokens = %+v, want usage from the fetched leaf only (100/20/120)", tokens)
	}
	if !tokens.Partial {
		t.Errorf("expected Partial=true when a leaf fetch fails")
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 2 {
		t.Errorf("expected 2 GetSpanDetails fetches, got %d", got)
	}
}

// Cost guard: when the trace's total span count exceeds the skip threshold,
// step 3 is bypassed entirely — no leaf fetches happen even if leaves exist.
func TestEnrichTraceOverview_SkipsLeafAggregationForHugeTraces(t *testing.T) {
	root := makeRootSpan("root", map[string]interface{}{})
	fake := &fakeObserverClient{
		spans: []observer.SpanInfo{
			{SpanID: "leaf-1", SpanName: "openai.chat", ParentSpanID: "root", StartTime: time.Now()},
		},
		spanDetails: map[string]*observer.SpanDetailsResponse{
			"leaf-1": {SpanID: "leaf-1", Attributes: map[string]interface{}{"gen_ai.usage.input_tokens": float64(1)}},
		},
	}
	c := NewTracingController(fake)

	hugeTrace := baseTraceInfo(skipLeafAggregationSpanCountThreshold + 1)
	input, output, tokens, _, _, _, _, _ := c.enrichTraceOverview(context.Background(), baseParams(), hugeTrace, root, nil, testFetchSem())

	if input != nil || output != nil || tokens != nil {
		t.Errorf("expected nil for huge trace, got input=%v output=%v tokens=%+v", input, output, tokens)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("expected no GetSpanDetails calls (skip), got %d", got)
	}
}

// TestExportTraces_UsesBulkAttributes verifies the export path fetches span
// attributes inline via QueryTraceSpans(includeAttributes=true) and makes zero
// GetSpanDetails calls, while preserving span kind/status/attributes.
func TestExportTraces_UsesBulkAttributes(t *testing.T) {
	now := time.Now()
	rootAttrs := map[string]interface{}{
		"traceloop.entity.input":  `{"inputs":"hello"}`,
		"traceloop.entity.output": `{"outputs":{"messages":[{"kwargs":{"content":"hi"}}]}}`,
	}
	fake := &fakeObserverClient{
		traces: []observer.TraceInfo{{
			TraceID:    "trace-1",
			RootSpanID: "root",
			StartTime:  now.Add(-time.Minute),
			EndTime:    now,
			SpanCount:  2,
		}},
		spans: []observer.SpanInfo{
			{SpanID: "root", SpanName: "invoke_agent", Kind: "SERVER", Status: &observer.SpanStatus{Code: "ok"}, StartTime: now.Add(-time.Minute), EndTime: now, Attributes: rootAttrs},
			{SpanID: "child", SpanName: "llm", ParentSpanID: "root", Kind: "INTERNAL", Status: &observer.SpanStatus{Code: "ok"}, StartTime: now.Add(-30 * time.Second), EndTime: now},
		},
	}
	c := NewTracingController(fake)

	resp, err := c.ExportTraces(context.Background(), baseParams())
	if err != nil {
		t.Fatalf("ExportTraces returned error: %v", err)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("expected 0 GetSpanDetails calls, got %d", got)
	}
	if !fake.lastSpansReq.IncludeAttributes {
		t.Error("expected QueryTraceSpans to request IncludeAttributes=true")
	}
	if len(resp.Traces) != 1 {
		t.Fatalf("expected 1 exported trace, got %d", len(resp.Traces))
	}
	tr := resp.Traces[0]
	if len(tr.Spans) != 2 {
		t.Fatalf("expected 2 spans, got %d", len(tr.Spans))
	}
	var root *opensearch.Span
	for i := range tr.Spans {
		if tr.Spans[i].SpanID == "root" {
			root = &tr.Spans[i]
		}
	}
	if root == nil {
		t.Fatal("root span missing from export")
	}
	if root.Kind != "SERVER" {
		t.Errorf("expected root Kind SERVER, got %q", root.Kind)
	}
	if _, ok := root.Attributes["traceloop.entity.input"]; !ok {
		t.Error("expected root span to retain bulk-fetched attributes")
	}
}

// overviewFake scripts one trace for GetTraceOverviews tests. Its span list
// ends with the root, as upstream's does.
func overviewFake(rootAttrs map[string]interface{}, spans []observer.SpanInfo, details map[string]*observer.SpanDetailsResponse) *fakeObserverClient {
	spans = append(spans, observer.SpanInfo{SpanID: "root", SpanName: "invoke_agent LangGraph", Attributes: rootAttrs})
	return &fakeObserverClient{
		traces:      []observer.TraceInfo{baseTraceInfo(len(spans))},
		rootSpan:    &observer.SpanDetailsResponse{SpanID: "root", SpanName: "invoke_agent LangGraph", Attributes: rootAttrs},
		spans:       spans,
		spanDetails: details,
	}
}

// singleOverview fetches the trace list and expects exactly one trace.
func singleOverview(t *testing.T, c *TracingController, params TraceQueryParams) opensearch.TraceOverview {
	t.Helper()
	resp, err := c.GetTraceOverviews(context.Background(), params)
	if err != nil {
		t.Fatalf("GetTraceOverviews returned error: %v", err)
	}
	if len(resp.Traces) != 1 {
		t.Fatalf("expected 1 trace overview, got %d", len(resp.Traces))
	}
	return resp.Traces[0]
}

// assertModels checks the models list, in order.
func assertModels(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("models = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("models = %v, want %v", got, want)
		}
	}
}

// Empty root and no chain span: models come from the fetched leaves.
func TestGetTraceOverviews_ModelsFromLeafLLMSpans(t *testing.T) {
	start := time.Now().Add(-10 * time.Minute)
	fake := overviewFake(
		map[string]interface{}{
			"gen_ai.operation.name":  "invoke_agent",
			"gen_ai.conversation.id": "conv-42",
		},
		[]observer.SpanInfo{
			{SpanID: "leaf-1", SpanName: "openai.chat", ParentSpanID: "root", StartTime: start},
			{SpanID: "leaf-2", SpanName: "openai.chat", ParentSpanID: "root", StartTime: start.Add(1 * time.Minute)},
			{SpanID: "leaf-3", SpanName: "openai.chat", ParentSpanID: "root", StartTime: start.Add(2 * time.Minute)},
		},
		map[string]*observer.SpanDetailsResponse{
			"leaf-1": {
				SpanID: "leaf-1", SpanName: "openai.chat",
				Attributes: map[string]interface{}{
					"gen_ai.request.model":       "gpt-4o",
					"gen_ai.response.model":      "gpt-4o-2024-08-06",
					"gen_ai.input.messages":      `[{"role":"user","parts":[{"type":"text","content":"hello"}]}]`,
					"gen_ai.output.messages":     `[{"role":"assistant","parts":[{"type":"text","content":"hi"}]}]`,
					"gen_ai.usage.input_tokens":  float64(10),
					"gen_ai.usage.output_tokens": float64(2),
				},
			},
			"leaf-2": {
				SpanID: "leaf-2", SpanName: "openai.chat",
				Attributes: map[string]interface{}{
					"gen_ai.request.model":       "gpt-4o-mini",
					"gen_ai.usage.input_tokens":  float64(5),
					"gen_ai.usage.output_tokens": float64(1),
				},
			},
			"leaf-3": {
				SpanID: "leaf-3", SpanName: "openai.chat",
				Attributes: map[string]interface{}{
					"gen_ai.response.model":      "gpt-4o-2024-08-06",
					"gen_ai.output.messages":     `[{"role":"assistant","parts":[{"type":"text","content":"bye"}]}]`,
					"gen_ai.usage.input_tokens":  float64(5),
					"gen_ai.usage.output_tokens": float64(1),
				},
			},
		},
	)
	c := NewTracingController(fake)

	ov := singleOverview(t, c, baseParams())

	assertModels(t, ov.Models, []string{"gpt-4o-2024-08-06", "gpt-4o-mini"})
	if ov.ConversationID != "conv-42" {
		t.Errorf("conversationId = %q, want conv-42", ov.ConversationID)
	}
	if ov.Input != "hello" || ov.Output != "bye" {
		t.Errorf("input/output = %v/%v, want hello/bye", ov.Input, ov.Output)
	}
	if ov.TokenUsage == nil || ov.TokenUsage.TotalTokens != 24 {
		t.Errorf("tokenUsage = %+v, want total 24", ov.TokenUsage)
	}
	// root + 3 leaves, each fetched once.
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 4 {
		t.Errorf("expected 4 GetSpanDetails calls (root + 3 leaves), got %d", got)
	}
}

// Chain span fills step 2; Include.Models adds Models from the span list and
// takes the root and chain span from it too.
func TestGetTraceOverviews_ChainSpanShortCircuit_IncludeModels(t *testing.T) {
	for _, includeModels := range []bool{true, false} {
		t.Run(fmt.Sprintf("include.models=%t", includeModels), func(t *testing.T) {
			fake := chainSpanFake()
			c := NewTracingController(fake)
			params := baseParams()
			params.Include.Models = includeModels

			ov := singleOverview(t, c, params)

			if includeModels {
				assertModels(t, ov.Models, []string{"gpt-4o", "claude-sonnet-4-5"})
			} else if ov.Models != nil {
				t.Errorf("models = %v, want nil without Include.Models", ov.Models)
			}
			if ov.ConversationID != "" {
				t.Errorf("conversationId = %q, want empty (root has none)", ov.ConversationID)
			}
			if ov.TokenUsage == nil || ov.TokenUsage.TotalTokens != 25 {
				t.Errorf("tokenUsage = %+v, want the chain span's 25, not the leaf sum", ov.TokenUsage)
			}
			if ov.Output != "chain out" {
				t.Errorf("output = %v, want the chain span's output", ov.Output)
			}
			// root + chain without the flag; with it, 0: both come from the
			// attribute list.
			wantDetails := int32(2)
			if includeModels {
				wantDetails = 0
			}
			if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != wantDetails {
				t.Errorf("expected %d GetSpanDetails calls, got %d", wantDetails, got)
			}
			if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 1 {
				t.Errorf("expected exactly 1 QueryTraceSpans call, got %d", got)
			}
			if fake.lastSpansReq.IncludeAttributes != includeModels {
				t.Errorf("QueryTraceSpans IncludeAttributes = %t, want %t", fake.lastSpansReq.IncludeAttributes, includeModels)
			}
		})
	}
}

// chainSpanFake scripts a LangGraph-shaped trace: empty root, chain span, two LLM leaves.
func chainSpanFake() *fakeObserverClient {
	start := time.Now().Add(-10 * time.Minute)
	chainAttrs := map[string]interface{}{
		"traceloop.entity.input":     `{"inputs":"chain in"}`,
		"traceloop.entity.output":    `{"outputs":{"messages":[{"kwargs":{"content":"chain out"}}]}}`,
		"gen_ai.usage.input_tokens":  float64(20),
		"gen_ai.usage.output_tokens": float64(5),
	}
	leaf1Attrs := map[string]interface{}{
		"gen_ai.response.model":      "gpt-4o",
		"gen_ai.input.messages":      `[{"role":"user","parts":[{"type":"text","content":"leaf in"}]}]`,
		"gen_ai.output.messages":     `[{"role":"assistant","parts":[{"type":"text","content":"leaf out"}]}]`,
		"gen_ai.usage.input_tokens":  float64(999),
		"gen_ai.usage.output_tokens": float64(999),
	}
	leaf2Attrs := map[string]interface{}{
		"gen_ai.request.model": "claude-sonnet-4-5",
	}
	return overviewFake(
		map[string]interface{}{},
		[]observer.SpanInfo{
			{SpanID: "chain-1", SpanName: "LangGraph.workflow", ParentSpanID: "root", StartTime: start, Attributes: chainAttrs},
			{SpanID: "leaf-1", SpanName: "ChatOpenAI.chat", ParentSpanID: "chain-1", StartTime: start.Add(1 * time.Minute), Attributes: leaf1Attrs},
			{SpanID: "leaf-2", SpanName: "ChatAnthropic.chat", ParentSpanID: "chain-1", StartTime: start.Add(2 * time.Minute), Attributes: leaf2Attrs},
		},
		map[string]*observer.SpanDetailsResponse{
			"chain-1": {SpanID: "chain-1", SpanName: "LangGraph.workflow", Attributes: chainAttrs},
			"leaf-1":  {SpanID: "leaf-1", SpanName: "ChatOpenAI.chat", ParentSpanID: "chain-1", Attributes: leaf1Attrs},
			"leaf-2":  {SpanID: "leaf-2", SpanName: "ChatAnthropic.chat", ParentSpanID: "chain-1", Attributes: leaf2Attrs},
		},
	)
}

// completeRootAttrs is a root span that fills input, output and tokens by
// itself, so the cascade has nothing left to fetch.
func completeRootAttrs() map[string]interface{} {
	return map[string]interface{}{
		"gen_ai.operation.name":      "invoke_agent",
		"gen_ai.conversation.id":     "conv-9",
		"traceloop.entity.input":     `{"inputs":"hello there"}`,
		"traceloop.entity.output":    `{"outputs":{"messages":[{"kwargs":{"content":"hi back"}}]}}`,
		"gen_ai.usage.input_tokens":  float64(10),
		"gen_ai.usage.output_tokens": float64(3),
	}
}

// Root-complete trace: one call either way. Without Include.Models it is the
// root fetch; with it, the span list, which also supplies the root.
func TestGetTraceOverviews_RootComplete_IncludeModels(t *testing.T) {
	for _, includeModels := range []bool{true, false} {
		t.Run(fmt.Sprintf("include.models=%t", includeModels), func(t *testing.T) {
			start := time.Now().Add(-10 * time.Minute)
			fake := overviewFake(
				completeRootAttrs(),
				[]observer.SpanInfo{
					{SpanID: "leaf-1", SpanName: "openai.chat", ParentSpanID: "root", StartTime: start,
						Attributes: map[string]interface{}{"gen_ai.response.model": "gpt-4o", "gen_ai.usage.input_tokens": float64(999)}},
					{SpanID: "leaf-2", SpanName: "openai.chat", ParentSpanID: "root", StartTime: start.Add(1 * time.Minute),
						Attributes: map[string]interface{}{"gen_ai.request.model": "gpt-4o-mini"}},
				},
				nil,
			)
			c := NewTracingController(fake)
			params := baseParams()
			params.Include.Models = includeModels

			ov := singleOverview(t, c, params)

			if includeModels {
				assertModels(t, ov.Models, []string{"gpt-4o", "gpt-4o-mini"})
			} else if ov.Models != nil {
				t.Errorf("models = %v, want nil without Include.Models", ov.Models)
			}
			if ov.ConversationID != "conv-9" {
				t.Errorf("conversationId = %q, want conv-9", ov.ConversationID)
			}
			if ov.TokenUsage == nil || ov.TokenUsage.TotalTokens != 13 {
				t.Errorf("tokenUsage = %+v, want the root's 13, not the leaf usage", ov.TokenUsage)
			}
			if ov.Input == nil || ov.Output == nil {
				t.Errorf("input/output = %v/%v, want the root's values", ov.Input, ov.Output)
			}
			wantDetails, wantLists := int32(1), int32(0)
			if includeModels {
				wantDetails, wantLists = 0, 1
			}
			if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != wantDetails {
				t.Errorf("expected %d GetSpanDetails calls, got %d", wantDetails, got)
			}
			if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != wantLists {
				t.Errorf("expected %d QueryTraceSpans calls, got %d", wantLists, got)
			}
			if includeModels && !fake.lastSpansReq.IncludeAttributes {
				t.Error("expected QueryTraceSpans to request IncludeAttributes=true")
			}
		})
	}
}

// Same scenario through enrichTraceOverview directly: the models path makes
// no GetSpanDetails call at all.
func TestEnrichTraceOverview_IncludeModelsFetchesNoSpanDetails(t *testing.T) {
	root := makeRootSpan("root", completeRootAttrs())
	fake := &fakeObserverClient{
		spans: []observer.SpanInfo{
			{SpanID: "leaf-1", SpanName: "openai.chat", ParentSpanID: "root", StartTime: time.Now(),
				Attributes: map[string]interface{}{"gen_ai.response.model": "gpt-4o"}},
		},
	}
	c := NewTracingController(fake)
	params := baseParams()
	params.Include.Models = true

	_, _, _, models, _, _, _, _ := c.enrichTraceOverview(context.Background(), params, baseTraceInfo(2), root, nil, testFetchSem())

	assertModels(t, models, []string{"gpt-4o"})
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("expected 0 GetSpanDetails calls, got %d", got)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 1 {
		t.Errorf("expected exactly 1 QueryTraceSpans call, got %d", got)
	}
}

// Models read from the span list are not subject to maxLLMLeavesPerTrace:
// every leaf's model is reported from the one span-list call.
func TestGetTraceOverviews_IncludeModelsNotCappedAtLeafLimit(t *testing.T) {
	const totalLeaves = maxLLMLeavesPerTrace + 5
	start := time.Now().Add(-1 * time.Hour)
	spans := make([]observer.SpanInfo, 0, totalLeaves)
	want := make([]string, 0, totalLeaves)
	for i := 0; i < totalLeaves; i++ {
		model := fmt.Sprintf("model-%02d", i)
		want = append(want, model)
		spans = append(spans, observer.SpanInfo{
			SpanID: fmt.Sprintf("leaf-%02d", i), SpanName: "openai.chat", ParentSpanID: "root",
			StartTime:  start.Add(time.Duration(i) * time.Second),
			Attributes: map[string]interface{}{"gen_ai.response.model": model},
		})
	}
	fake := overviewFake(completeRootAttrs(), spans, nil)
	c := NewTracingController(fake)
	params := baseParams()
	params.Include.Models = true

	ov := singleOverview(t, c, params)

	assertModels(t, ov.Models, want)
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("expected 0 GetSpanDetails calls, got %d", got)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 1 {
		t.Errorf("expected exactly 1 QueryTraceSpans call, got %d", got)
	}
}

// The span-count threshold still gates the models path: a root-complete
// trace above it is not listed at all, flag or no flag.
func TestGetTraceOverviews_IncludeModelsSkipsHugeTraces(t *testing.T) {
	fake := overviewFake(
		completeRootAttrs(),
		[]observer.SpanInfo{
			{SpanID: "leaf-1", SpanName: "openai.chat", ParentSpanID: "root", StartTime: time.Now(),
				Attributes: map[string]interface{}{"gen_ai.response.model": "gpt-4o"}},
		},
		nil,
	)
	fake.traces[0].SpanCount = skipLeafAggregationSpanCountThreshold + 1
	c := NewTracingController(fake)
	params := baseParams()
	params.Include.Models = true

	ov := singleOverview(t, c, params)

	if ov.Models != nil {
		t.Errorf("models = %v, want nil above the span-count threshold", ov.Models)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 0 {
		t.Errorf("expected 0 QueryTraceSpans calls, got %d", got)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 1 {
		t.Errorf("expected exactly 1 GetSpanDetails call (root only), got %d", got)
	}
}

// No LLM spans with Include.Models: Models is nil, ConversationID still comes
// from the root, and the span list, which supplies the root, is the only call.
func TestGetTraceOverviews_NoLLMSpansLeavesModelsEmpty(t *testing.T) {
	start := time.Now().Add(-10 * time.Minute)
	fake := overviewFake(
		map[string]interface{}{
			"gen_ai.operation.name":      "invoke_agent",
			"gen_ai.conversation.id":     "conv-7",
			"traceloop.entity.input":     `{"inputs":"hello there"}`,
			"traceloop.entity.output":    `{"outputs":{"messages":[{"kwargs":{"content":"hi back"}}]}}`,
			"gen_ai.usage.input_tokens":  float64(10),
			"gen_ai.usage.output_tokens": float64(3),
		},
		[]observer.SpanInfo{
			{SpanID: "tool-1", SpanName: "search_web.tool", ParentSpanID: "root", StartTime: start},
		},
		map[string]*observer.SpanDetailsResponse{
			"tool-1": {SpanID: "tool-1", SpanName: "search_web.tool", Attributes: map[string]interface{}{"gen_ai.tool.name": "search_web"}},
		},
	)
	c := NewTracingController(fake)
	params := baseParams()
	params.Include.Models = true

	ov := singleOverview(t, c, params)

	if ov.Models != nil {
		t.Errorf("models = %v, want nil", ov.Models)
	}
	if ov.ConversationID != "conv-7" {
		t.Errorf("conversationId = %q, want conv-7", ov.ConversationID)
	}
	if ov.TokenUsage == nil || ov.TokenUsage.TotalTokens != 13 {
		t.Errorf("tokenUsage = %+v, want root's 13", ov.TokenUsage)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("expected 0 GetSpanDetails calls, got %d", got)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 1 {
		t.Errorf("expected exactly 1 QueryTraceSpans call, got %d", got)
	}
}
