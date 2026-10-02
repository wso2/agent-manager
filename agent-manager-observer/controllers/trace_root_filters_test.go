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
	"maps"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
)

// langGraphFake scripts n LangGraph-shaped traces one second apart, newest
// first, like lookBackFake. Each has an empty root, a chain span with input
// and output, and two LLM leaves carrying tokens (12 + i%40) and the model
// (gpt-4o-mini for even i, claude-sonnet-4-5 for odd). The full cascade is 5
// calls: root, span list, chain, two leaves. The span list ends with the root,
// as upstream's does. rootAttrs adds root attributes.
func langGraphFake(n int, rootAttrs func(i int) map[string]interface{}) *fakeObserverClient {
	fake := &fakeObserverClient{
		windowed:     true,
		spanDetails:  map[string]*observer.SpanDetailsResponse{},
		spansByTrace: map[string][]observer.SpanInfo{},
	}
	for i := 0; i < n; i++ {
		info := baseTraceInfo(4)
		info.TraceID = fmt.Sprintf("trace-%04d", i)
		info.RootSpanID = fmt.Sprintf("root-%04d", i)
		info.StartTime = lookBackWindowEnd.Add(-time.Duration(i+1) * time.Second)
		info.EndTime = info.StartTime
		fake.traces = append(fake.traces, info)

		root := map[string]interface{}{"gen_ai.operation.name": "invoke_agent"}
		maps.Copy(root, rootAttrs(i))
		model := "gpt-4o-mini"
		if i%2 == 1 {
			model = "claude-sonnet-4-5"
		}
		chainID, leafA, leafB := fmt.Sprintf("chain-%04d", i), fmt.Sprintf("leaf-a-%04d", i), fmt.Sprintf("leaf-b-%04d", i)
		spans := []observer.SpanInfo{
			{SpanID: chainID, SpanName: "LangGraph.workflow", ParentSpanID: info.RootSpanID, StartTime: info.StartTime,
				Attributes: map[string]interface{}{
					"traceloop.entity.input":  fmt.Sprintf(`{"inputs":"in %d"}`, i),
					"traceloop.entity.output": fmt.Sprintf(`{"outputs":{"messages":[{"kwargs":{"content":"out %d"}}]}}`, i),
				}},
			{SpanID: leafA, SpanName: "ChatOpenAI.chat", ParentSpanID: chainID, StartTime: info.StartTime,
				Attributes: map[string]interface{}{
					"gen_ai.response.model":      model,
					"gen_ai.usage.input_tokens":  float64(i % 40),
					"gen_ai.usage.output_tokens": float64(1),
				}},
			{SpanID: leafB, SpanName: "ChatOpenAI.chat", ParentSpanID: chainID, StartTime: info.StartTime,
				Attributes: map[string]interface{}{
					"gen_ai.response.model":      model,
					"gen_ai.usage.input_tokens":  float64(10),
					"gen_ai.usage.output_tokens": float64(1),
				}},
		}
		spans = append(spans, observer.SpanInfo{SpanID: info.RootSpanID, SpanName: "invoke_agent LangGraph", Attributes: root})
		fake.spansByTrace[info.TraceID] = spans
		for _, s := range spans {
			fake.spanDetails[s.SpanID] = &observer.SpanDetailsResponse{
				SpanID: s.SpanID, SpanName: s.SpanName, ParentSpanID: s.ParentSpanID, Attributes: s.Attributes,
			}
		}
	}
	return fake
}

func noRootAttrs(int) map[string]interface{} { return nil }

// errorEvery marks every nth root (from 0) as failed.
func errorEvery(n int) func(int) map[string]interface{} {
	return func(i int) map[string]interface{} {
		if i%n == 0 {
			return map[string]interface{}{"error.type": "RuntimeError"}
		}
		return nil
	}
}

// pagesOf follows nextCursor for up to n pages.
func pagesOf(t *testing.T, c *TracingController, params TraceQueryParams, n int) []*opensearch.TraceOverviewResponse {
	t.Helper()
	var pages []*opensearch.TraceOverviewResponse
	for len(pages) < n {
		resp, err := c.GetTraceOverviews(context.Background(), params)
		if err != nil {
			t.Fatalf("GetTraceOverviews returned error: %v", err)
		}
		pages = append(pages, resp)
		if resp.NextCursor == "" {
			break
		}
		cur, err := DecodeTraceCursor(resp.NextCursor)
		if err != nil {
			t.Fatalf("DecodeTraceCursor: %v", err)
		}
		params.Cursor = cur
	}
	return pages
}

func upstreamCalls(f *fakeObserverClient) int32 {
	return atomic.LoadInt32(&f.queryTracesCalls) + atomic.LoadInt32(&f.getSpanDetailsCalls) + atomic.LoadInt32(&f.queryTraceSpansCalls)
}

// status=error over ok roots: one root fetch per examined trace, no span lists.
func TestGetTraceOverviews_StatusRejectsAtRoot(t *testing.T) {
	fake := langGraphFake(120, noRootAttrs)
	c := NewTracingController(fake)
	params := lookBackParams(10)
	params.Filters = TraceFilters{Status: TraceStatusError}

	ids, _, truncated := traceIDs(c, t, params)

	if len(ids) != 0 || truncated {
		t.Fatalf("got %d traces, truncated %v; want 0, false", len(ids), truncated)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 120 {
		t.Errorf("GetSpanDetails calls = %d, want 120 (one root per examined trace)", got)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 0 {
		t.Errorf("QueryTraceSpans calls = %d, want 0", got)
	}
}

// conversationId: a non-matching trace costs its root fetch, a match the full cascade.
func TestGetTraceOverviews_ConversationIDRejectsAtRoot(t *testing.T) {
	fake := langGraphFake(100, func(i int) map[string]interface{} {
		if i%10 == 0 {
			return map[string]interface{}{"gen_ai.conversation.id": "conv-match"}
		}
		return map[string]interface{}{"gen_ai.conversation.id": "conv-other"}
	})
	c := NewTracingController(fake)
	params := lookBackParams(5)

	resp, err := c.GetTraceOverviews(context.Background(), params)
	if err != nil {
		t.Fatalf("GetTraceOverviews returned error: %v", err)
	}

	want := []string{"trace-0000", "trace-0010", "trace-0020", "trace-0030", "trace-0040"}
	got := make([]string, len(resp.Traces))
	for i, ov := range resp.Traces {
		got[i] = ov.TraceID
		n := i * 10
		if ov.Output != fmt.Sprintf("out %d", n) || ov.Input == nil {
			t.Errorf("%s input/output = %v/%v, want the chain span's", ov.TraceID, ov.Input, ov.Output)
		}
		if ov.TokenUsage == nil || ov.TokenUsage.TotalTokens != 12+n%40 {
			t.Errorf("%s tokenUsage = %+v, want total %d from the leaves", ov.TraceID, ov.TokenUsage, 12+n%40)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("traces = %v, want %v", got, want)
	}
	// The first chunk of 50 fills the page: 50 roots, plus chain and two leaves per match.
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 50+5*3 {
		t.Errorf("GetSpanDetails calls = %d, want %d", got, 50+5*3)
	}
	listed := slices.Sorted(slices.Values(fake.spansTraceIDs))
	if !slices.Equal(listed, want) {
		t.Errorf("QueryTraceSpans traces = %v, want the matches %v", listed, want)
	}
}

// Rejecting at the root returns the same pages as matchesFilters over fully
// enriched overviews, and makes fewer upstream calls.
func TestGetTraceOverviews_RootRejectionKeepsResults(t *testing.T) {
	// conv-match on every 3rd root, an error on every 7th.
	rootAttrs := func(i int) map[string]interface{} {
		attrs := map[string]interface{}{"gen_ai.conversation.id": "conv-other"}
		if i%3 == 0 {
			attrs["gen_ai.conversation.id"] = "conv-match"
		}
		if i%7 == 0 {
			attrs["error.type"] = "RuntimeError"
		}
		return attrs
	}
	tests := []struct {
		name    string
		filters TraceFilters
	}{
		{name: "none", filters: TraceFilters{}},
		{name: "status error", filters: TraceFilters{Status: TraceStatusError}},
		{name: "status ok", filters: TraceFilters{Status: TraceStatusOK}},
		{name: "conversationId", filters: TraceFilters{ConversationID: "conv-match"}},
		// Matches nothing, so the walk stops at the examine cap.
		{name: "conversationId at cap", filters: TraceFilters{ConversationID: "conv-none"}},
		{name: "status and minTokens", filters: TraceFilters{Status: TraceStatusError, MinTokens: ptr(47)}},
		{name: "status and model", filters: TraceFilters{Status: TraceStatusOK, Model: "claude"}},
	}
	for _, tt := range tests {
		for _, order := range []string{"desc", "asc"} {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				params := lookBackParams(10)
				params.Filters = tt.filters
				params.SortOrder = order
				earlyFake, fullFake := langGraphFake(600, rootAttrs), langGraphFake(600, rootAttrs)

				early := pagesOf(t, NewTracingController(earlyFake), params, 3)
				full := pagesOf(t, &TracingController{observerClient: fullFake, enrichAll: true}, params, 3)

				if len(early) != len(full) {
					t.Fatalf("got %d pages, want %d", len(early), len(full))
				}
				for i := range full {
					if !reflect.DeepEqual(early[i], full[i]) {
						t.Errorf("page %d differs:\n got %+v\nwant %+v", i, early[i], full[i])
					}
				}
				earlyCalls, fullCalls := upstreamCalls(earlyFake), upstreamCalls(fullFake)
				if tt.filters.IsZero() && earlyCalls != fullCalls {
					t.Errorf("unfiltered calls = %d, want %d", earlyCalls, fullCalls)
				}
				if !tt.filters.IsZero() && earlyCalls >= fullCalls {
					t.Errorf("upstream calls = %d, want fewer than %d", earlyCalls, fullCalls)
				}
			})
		}
	}
}

// status=error matching 5% of traces: only the 10 matches get a span list.
// Enrichment calls drop from 1,000 (5 per examined trace) to 240.
func TestGetTraceOverviews_StatusErrorFivePercentCallCounts(t *testing.T) {
	params := lookBackParams(10)
	params.Filters = TraceFilters{Status: TraceStatusError}
	want := make([]string, 0, 10)
	for i := 0; i < 200; i += 20 {
		want = append(want, fmt.Sprintf("trace-%04d", i))
	}

	fullFake := langGraphFake(200, errorEvery(20))
	fullIDs, _, _ := traceIDs(&TracingController{observerClient: fullFake, enrichAll: true}, t, params)
	fake := langGraphFake(200, errorEvery(20))
	ids, _, truncated := traceIDs(NewTracingController(fake), t, params)

	if !slices.Equal(ids, want) || !slices.Equal(fullIDs, want) || truncated {
		t.Fatalf("traces = %v (full cascade %v), truncated %v; want %v, false", ids, fullIDs, truncated, want)
	}
	// The 10th match is in the 4th chunk of 50, so all 200 roots are fetched.
	if got := atomic.LoadInt32(&fullFake.getSpanDetailsCalls); got != 200*4 {
		t.Errorf("full cascade GetSpanDetails calls = %d, want %d", got, 200*4)
	}
	if got := atomic.LoadInt32(&fullFake.queryTraceSpansCalls); got != 200 {
		t.Errorf("full cascade QueryTraceSpans calls = %d, want 200", got)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 200+10*3 {
		t.Errorf("GetSpanDetails calls = %d, want %d (200 roots, chain and two leaves per match)", got, 200+10*3)
	}
	if listed := slices.Sorted(slices.Values(fake.spansTraceIDs)); !slices.Equal(listed, want) {
		t.Errorf("QueryTraceSpans traces = %v, want the matches %v", listed, want)
	}
	t.Logf("upstream calls: %d with the full cascade, %d with root rejection", upstreamCalls(fullFake), upstreamCalls(fake))
}
