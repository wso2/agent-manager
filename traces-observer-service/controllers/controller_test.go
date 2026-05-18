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
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/ai-agent-management-platform/traces-observer-service/observer"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/opensearch"
)

// fakeObserverClient is a minimal observer.Client implementation that the
// cascade tests use to script trace-overview enrichment scenarios. Each test
// installs the spans for a single trace; getSpanDetailsCalls counts how many
// GetSpanDetails calls the cascade made so we can verify the cost guards.
type fakeObserverClient struct {
	// rootSpan is returned by GetSpanDetails when called for the root span.
	rootSpan *observer.SpanDetailsResponse
	// spans is the list returned by QueryTraceSpans.
	spans []observer.SpanInfo
	// spanDetails maps spanID → detail response for GetSpanDetails lookups
	// (excluding root, which is rootSpan).
	spanDetails map[string]*observer.SpanDetailsResponse

	getSpanDetailsCalls  int32
	queryTraceSpansCalls int32
}

func (f *fakeObserverClient) QueryTraces(_ context.Context, _ observer.TracesQueryRequest) (*observer.TracesQueryResponse, error) {
	return nil, fmt.Errorf("not used by enrichTraceOverview tests")
}

func (f *fakeObserverClient) QueryTraceSpans(_ context.Context, _ string, _ observer.TracesQueryRequest) (*observer.TraceSpansQueryResponse, error) {
	atomic.AddInt32(&f.queryTraceSpansCalls, 1)
	return &observer.TraceSpansQueryResponse{Spans: f.spans, Total: len(f.spans)}, nil
}

func (f *fakeObserverClient) GetSpanDetails(_ context.Context, _, spanID string) (*observer.SpanDetailsResponse, error) {
	atomic.AddInt32(&f.getSpanDetailsCalls, 1)
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

// (a) Root span carries entity.input/output and entity.output → root-derived
// token usage. The cascade must short-circuit at step 1 — no extra fetches.
func TestEnrichTraceOverview_RootHasEntityShortCircuits(t *testing.T) {
	root := makeRootSpan("root", map[string]interface{}{
		"traceloop.entity.input":  `{"inputs":"hello there"}`,
		"traceloop.entity.output": `{"outputs":{"messages":[{"kwargs":{"content":"hi back","response_metadata":{"token_usage":{"prompt_tokens":10,"completion_tokens":3,"total_tokens":13}}}}]}}`,
	})
	fake := &fakeObserverClient{rootSpan: &observer.SpanDetailsResponse{SpanID: "root"}}
	c := NewTracingController(fake)

	input, output, tokens := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(5), root, testFetchSem())

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

// (b) Root span empty, but the immediate child chain span carries
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

	input, output, tokens := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(5), root, testFetchSem())

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

// (c) Root and child empty, but leaf LLM spans carry gen_ai.input.messages /
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

	input, output, tokens := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(5), root, testFetchSem())

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

// (d) Everything empty — degenerate case. No extra fetches beyond QueryTraceSpans.
// All three return nil; trace overview row simply renders "-" in the UI.
func TestEnrichTraceOverview_AllEmptyReturnsNil(t *testing.T) {
	root := makeRootSpan("root", map[string]interface{}{})
	fake := &fakeObserverClient{spans: []observer.SpanInfo{}}
	c := NewTracingController(fake)

	input, output, tokens := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(1), root, testFetchSem())

	if input != nil || output != nil || tokens != nil {
		t.Errorf("expected all nil, got input=%v output=%v tokens=%+v", input, output, tokens)
	}
}

// (e) Leaf cap honoured. With > maxLLMLeavesPerTrace leaves, only the cap is
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
	_, _, tokens := c.enrichTraceOverview(context.Background(), baseParams(), baseTraceInfo(skipLeafAggregationSpanCountThreshold-1), root, testFetchSem())

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
	input, output, tokens := c.enrichTraceOverview(context.Background(), baseParams(), hugeTrace, root, testFetchSem())

	if input != nil || output != nil || tokens != nil {
		t.Errorf("expected nil for huge trace, got input=%v output=%v tokens=%+v", input, output, tokens)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("expected no GetSpanDetails calls (skip), got %d", got)
	}
}
