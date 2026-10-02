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
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// leafPathFake scripts an OpenAI Agents-shaped trace: an empty root and n LLM
// leaves carrying messages, tokens and a model, the same in list and details.
func leafPathFake(n int) *fakeObserverClient {
	start := time.Now().Add(-1 * time.Hour)
	spans := make([]observer.SpanInfo, 0, n)
	details := make(map[string]*observer.SpanDetailsResponse, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("leaf-%02d", i)
		attrs := map[string]interface{}{
			"gen_ai.response.model":      fmt.Sprintf("gpt-4o-%d", i%2),
			"gen_ai.input.messages":      fmt.Sprintf(`[{"role":"user","parts":[{"type":"text","content":"user %d"}]}]`, i),
			"gen_ai.output.messages":     fmt.Sprintf(`[{"role":"assistant","parts":[{"type":"text","content":"assistant %d"}]}]`, i),
			"gen_ai.usage.input_tokens":  float64(10 + i),
			"gen_ai.usage.output_tokens": float64(2),
		}
		spans = append(spans, observer.SpanInfo{
			SpanID: id, SpanName: "openai.chat", ParentSpanID: "root", Kind: "CLIENT",
			Status: &observer.SpanStatus{Code: "ok"}, StartTime: start.Add(time.Duration(i) * time.Second), Attributes: attrs,
		})
		details[id] = &observer.SpanDetailsResponse{
			SpanID: id, SpanName: "openai.chat", ParentSpanID: "root", Kind: "CLIENT",
			Status: &observer.SpanStatus{Code: "ok"}, StartTime: start.Add(time.Duration(i) * time.Second), Attributes: attrs,
		}
	}
	return overviewFake(map[string]interface{}{"gen_ai.operation.name": "invoke_agent"}, spans, details)
}

// With include=models, the chain and leaf paths read every span from the
// attribute list and return the row the per-span fetches did.
func TestGetTraceOverviews_InlineSpansMatchPerSpanFetches(t *testing.T) {
	tests := []struct {
		name        string
		fake        func() *fakeObserverClient
		wantPartial bool
	}{
		{name: "chain span", fake: chainSpanFake},
		{name: "leaf path", fake: func() *fakeObserverClient { return leafPathFake(3) }},
		// The leaf cap still applies to aggregation.
		{name: "leaf path over cap", fake: func() *fakeObserverClient { return leafPathFake(maxLLMLeavesPerTrace + 5) }, wantPartial: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := baseParams()
			params.Include.Models = true
			fake, perSpanFake, noModelsFake := tt.fake(), tt.fake(), tt.fake()
			// Same trace times across the three.
			perSpanFake.traces, noModelsFake.traces = fake.traces, fake.traces

			got := singleOverview(t, NewTracingController(fake), params)
			want := singleOverview(t, &TracingController{observerClient: perSpanFake, perSpanDetails: true}, params)
			noModelsParams := params
			noModelsParams.Include.Models = false
			noModels := singleOverview(t, NewTracingController(noModelsFake), noModelsParams)

			if !reflect.DeepEqual(got, want) {
				t.Errorf("overview differs from per-span fetches:\n got %+v\nwant %+v", got, want)
			}
			if got.Input == nil || got.Output == nil || got.TokenUsage == nil || len(got.Models) == 0 {
				t.Fatalf("overview = %+v, want input, output, tokens and models", got)
			}
			if got.TokenUsage.Partial != tt.wantPartial {
				t.Errorf("Partial = %t, want %t", got.TokenUsage.Partial, tt.wantPartial)
			}
			// The row is the same without include=models, bar the models.
			got.Models, noModels.Models = nil, nil
			if !reflect.DeepEqual(got, noModels) {
				t.Errorf("overview differs from include=models off:\n got %+v\nwant %+v", got, noModels)
			}
			if n := atomic.LoadInt32(&fake.getSpanDetailsCalls); n != 0 {
				t.Errorf("GetSpanDetails calls = %d, want 0", n)
			}
			if n := atomic.LoadInt32(&fake.attrSpansCalls); n != 1 {
				t.Errorf("QueryTraceSpans calls with attributes = %d, want 1", n)
			}
		})
	}
}

// A span list without the root: the root is fetched and the list is reused.
func TestGetTraceOverviews_RootMissingFromSpanList(t *testing.T) {
	fake := chainSpanFake()
	fake.spans = fake.spans[:len(fake.spans)-1]
	params := baseParams()
	params.Include.Models = true

	ov := singleOverview(t, NewTracingController(fake), params)

	assertModels(t, ov.Models, []string{"gpt-4o", "claude-sonnet-4-5"})
	if ov.Output != "chain out" || ov.TokenUsage == nil || ov.TokenUsage.TotalTokens != 25 {
		t.Errorf("output/tokens = %v/%+v, want the chain span's", ov.Output, ov.TokenUsage)
	}
	if n := atomic.LoadInt32(&fake.getSpanDetailsCalls); n != 1 {
		t.Errorf("GetSpanDetails calls = %d, want 1 (root)", n)
	}
	if n := atomic.LoadInt32(&fake.queryTraceSpansCalls); n != 1 {
		t.Errorf("QueryTraceSpans calls = %d, want 1", n)
	}
}

// Root filters keep the root fetch first, so a rejected trace never downloads its list.
func TestGetTraceOverviews_RootFilterSkipsListFirst(t *testing.T) {
	fake := langGraphFake(120, noRootAttrs)
	params := lookBackParams(10)
	params.Filters = TraceFilters{Status: TraceStatusError}
	params.Include.Models = true

	ids, _, _ := traceIDs(NewTracingController(fake), t, params)

	if len(ids) != 0 {
		t.Fatalf("traces = %v, want none", ids)
	}
	if n := atomic.LoadInt32(&fake.getSpanDetailsCalls); n != 120 {
		t.Errorf("GetSpanDetails calls = %d, want 120 (one root per examined trace)", n)
	}
	if n := atomic.LoadInt32(&fake.queryTraceSpansCalls); n != 0 {
		t.Errorf("QueryTraceSpans calls = %d, want 0", n)
	}
}

// Reading spans from the attribute list returns the same pages as per-span
// fetches, with fewer upstream calls.
func TestGetTraceOverviews_InlineSpansKeepResults(t *testing.T) {
	// conv-match on every 3rd root, an error on every 7th, rare-model on
	// every 9th trace, and every 5th trace over the span threshold.
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
	newFake := func() *fakeObserverClient {
		return overThreshold(withModel(langGraphFake(600, rootAttrs), 9, "rare-model"), 5)
	}
	tests := []struct {
		name    string
		filters TraceFilters
	}{
		{name: "none", filters: TraceFilters{}},
		{name: "model", filters: TraceFilters{Model: "claude"}},
		{name: "rare model", filters: TraceFilters{Model: "rare"}},
		{name: "model and status", filters: TraceFilters{Model: "gpt", Status: TraceStatusError}},
		{name: "model and minTokens", filters: TraceFilters{Model: "claude", MinTokens: ptr(47)}},
		{name: "status", filters: TraceFilters{Status: TraceStatusOK}},
		{name: "conversationId", filters: TraceFilters{ConversationID: "conv-match"}},
		{name: "minTokens", filters: TraceFilters{MinTokens: ptr(47)}},
	}
	for _, tt := range tests {
		for _, order := range []string{"desc", "asc"} {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				params := lookBackParams(10)
				params.Filters = tt.filters
				params.SortOrder = order
				params.Include.Models = true
				fake, perSpanFake := newFake(), newFake()

				got := pagesOf(t, NewTracingController(fake), params, 3)
				want := pagesOf(t, &TracingController{observerClient: perSpanFake, perSpanDetails: true}, params, 3)

				if len(got) != len(want) {
					t.Fatalf("got %d pages, want %d", len(got), len(want))
				}
				for i := range want {
					if !reflect.DeepEqual(got[i], want[i]) {
						t.Errorf("page %d differs:\n got %+v\nwant %+v", i, got[i], want[i])
					}
				}
				if calls, perSpanCalls := upstreamCalls(fake), upstreamCalls(perSpanFake); calls >= perSpanCalls {
					t.Errorf("upstream calls = %d, want fewer than %d", calls, perSpanCalls)
				}
			})
		}
	}
}
