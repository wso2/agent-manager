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
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
	"github.com/wso2/agent-manager/agent-manager-observer/store/openchoreo"
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
// attribute list. The row is the same without include=models, bar the models.
func TestGetTraceOverviews_InlineSpans(t *testing.T) {
	// Leaf i of leafPathFake has 10+i input and 2 output tokens.
	tests := []struct {
		name          string
		fake          func() *fakeObserverClient
		input, output string
		tokens        opensearch.TokenUsage
		models        []string
	}{
		{name: "chain span", fake: chainSpanFake, input: `"chain in"`, output: "chain out",
			tokens: opensearch.TokenUsage{InputTokens: 20, OutputTokens: 5, TotalTokens: 25},
			models: []string{"gpt-4o", "claude-sonnet-4-5"}},
		{name: "leaf path", fake: func() *fakeObserverClient { return leafPathFake(3) }, input: "user 0", output: "assistant 2",
			tokens: opensearch.TokenUsage{InputTokens: 33, OutputTokens: 6, TotalTokens: 39},
			models: []string{"gpt-4o-0", "gpt-4o-1"}},
		// The leaf cap still applies to aggregation: only leaves 0-49 count.
		{name: "leaf path over cap", fake: func() *fakeObserverClient { return leafPathFake(maxLLMLeavesPerTrace + 5) },
			input: "user 0", output: "assistant 49",
			tokens: opensearch.TokenUsage{InputTokens: 1725, OutputTokens: 100, TotalTokens: 1825, Partial: true},
			models: []string{"gpt-4o-0", "gpt-4o-1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := baseParams()
			params.Include.Models = true
			fake, noModelsFake := tt.fake(), tt.fake()
			noModelsParams := params
			noModelsParams.Include.Models = false

			got := singleOverview(t, NewTracingController(openchoreo.New(fake)), params)
			noModels := singleOverview(t, NewTracingController(openchoreo.New(noModelsFake)), noModelsParams)

			for _, ov := range []opensearch.TraceOverview{got, noModels} {
				if ov.Input != tt.input || ov.Output != tt.output {
					t.Errorf("input/output = %v/%v, want %s/%s", ov.Input, ov.Output, tt.input, tt.output)
				}
				if ov.TokenUsage == nil || *ov.TokenUsage != tt.tokens {
					t.Errorf("tokenUsage = %+v, want %+v", ov.TokenUsage, tt.tokens)
				}
			}
			assertModels(t, got.Models, tt.models)
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

	ov := singleOverview(t, NewTracingController(openchoreo.New(fake)), params)

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

	ids, _, _ := traceIDs(NewTracingController(openchoreo.New(fake)), t, params)

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

// include=models pages through the matches, reading the spans of every trace
// under the span threshold from its attribute list.
func TestGetTraceOverviews_InlineSpansPages(t *testing.T) {
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
	over := func(i int) bool { return i%5 == 0 }
	tests := []struct {
		name    string
		filters TraceFilters
		match   func(i int) bool
	}{
		{name: "none", filters: TraceFilters{}, match: func(int) bool { return true }},
		{name: "status", filters: TraceFilters{Status: TraceStatusOK}, match: func(i int) bool { return i%7 != 0 }},
		{name: "conversationId", filters: TraceFilters{ConversationID: "conv-match"}, match: func(i int) bool { return i%3 == 0 }},
		// Over-threshold traces have no tokens.
		{name: "minTokens", filters: TraceFilters{MinTokens: ptr(47)}, match: func(i int) bool { return !over(i) && 12+i%40 >= 47 }},
	}
	for _, tt := range tests {
		for _, order := range []string{"desc", "asc"} {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				params := lookBackParams(10)
				params.Filters = tt.filters
				params.SortOrder = order
				params.Include.Models = true
				fake := overThreshold(withModel(langGraphFake(600, rootAttrs), 9, "rare-model"), 5)

				pages := pagesOf(t, NewTracingController(openchoreo.New(fake)), params, 3)

				assertPages(t, pages, wantPages(fake, params, 3, tt.match))
				assertLangGraphRows(t, fake, pages)
				// Root filters keep the root fetch first.
				rootFilters := tt.filters.Status != TraceStatusAny || tt.filters.ConversationID != ""
				_, roots, others := fetchedTraces(t, fake)
				for _, i := range roots {
					if !over(i) && !rootFilters {
						t.Fatalf("trace-%04d fetched its root, want it from the span list", i)
					}
				}
				for _, i := range others {
					if !over(i) {
						t.Fatalf("trace-%04d fetched a chain or leaf span, want it from the span list", i)
					}
				}
			})
		}
	}
}
