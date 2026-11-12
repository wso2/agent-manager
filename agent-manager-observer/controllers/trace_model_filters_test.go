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
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
	"github.com/wso2/agent-manager/agent-manager-observer/store/openchoreo"
)

// withModel sets the model on both leaves of every nth trace (from 0).
func withModel(fake *fakeObserverClient, n int, model string) *fakeObserverClient {
	for i, info := range fake.traces {
		if i%n != 0 {
			continue
		}
		// Span list and span details share each attribute map.
		for _, s := range fake.spansByTrace[info.TraceID] {
			if opensearch.IsLLMLeafSpan(s.SpanName) {
				s.Attributes["gen_ai.response.model"] = model
			}
		}
	}
	return fake
}

// overThreshold gives every nth trace (from 0) more spans than leaf aggregation allows.
func overThreshold(fake *fakeObserverClient, n int) *fakeObserverClient {
	for i := range fake.traces {
		if i%n == 0 {
			fake.traces[i].SpanCount = skipLeafAggregationSpanCountThreshold + 1
		}
	}
	return fake
}

// A model no trace has: one attribute list per examined trace, which also
// supplies the root.
func TestGetTraceOverviews_ModelRejectsAfterSpanList(t *testing.T) {
	fake := langGraphFake(120, noRootAttrs)
	params := lookBackParams(10)
	params.Filters = TraceFilters{Model: "llama"}

	ids, _, truncated := traceIDs(NewTracingController(openchoreo.New(fake)), t, params)

	if len(ids) != 0 || truncated {
		t.Fatalf("got %d traces, truncated %v; want 0, false", len(ids), truncated)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("GetSpanDetails calls = %d, want 0", got)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 120 {
		t.Errorf("QueryTraceSpans calls = %d, want 120", got)
	}
	if got := atomic.LoadInt32(&fake.attrSpansCalls); got != 120 {
		t.Errorf("QueryTraceSpans calls with attributes = %d, want 120", got)
	}
}

// Traces over the span threshold have no models, so a model filter rejects
// them after the root fetch, without a span list.
func TestGetTraceOverviews_ModelRejectsOverThresholdAtRoot(t *testing.T) {
	fake := overThreshold(langGraphFake(20, noRootAttrs), 1)
	params := lookBackParams(10)
	params.Filters = TraceFilters{Model: "gpt"}

	ids, _, _ := traceIDs(NewTracingController(openchoreo.New(fake)), t, params)

	if len(ids) != 0 {
		t.Fatalf("traces = %v, want none", ids)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 20 {
		t.Errorf("GetSpanDetails calls = %d, want 20 (roots only)", got)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 0 {
		t.Errorf("QueryTraceSpans calls = %d, want 0", got)
	}
}

// A matching trace still gets input, output and tokens from the full cascade,
// read from its attribute list.
func TestGetTraceOverviews_ModelMatchRunsFullCascade(t *testing.T) {
	fake := langGraphFake(100, noRootAttrs)
	params := lookBackParams(5)
	params.Filters = TraceFilters{Model: "claude"}

	resp, err := NewTracingController(openchoreo.New(fake)).GetTraceOverviews(context.Background(), params)
	if err != nil {
		t.Fatalf("GetTraceOverviews returned error: %v", err)
	}

	want := []string{"trace-0001", "trace-0003", "trace-0005", "trace-0007", "trace-0009"}
	got := make([]string, len(resp.Traces))
	for i, ov := range resp.Traces {
		got[i] = ov.TraceID
		n := 2*i + 1
		if ov.Output != fmt.Sprintf("out %d", n) || ov.Input == nil {
			t.Errorf("%s input/output = %v/%v, want the chain span's", ov.TraceID, ov.Input, ov.Output)
		}
		if ov.TokenUsage == nil || ov.TokenUsage.TotalTokens != 12+n%40 {
			t.Errorf("%s tokenUsage = %+v, want total %d from the leaves", ov.TraceID, ov.TokenUsage, 12+n%40)
		}
		if !slices.Equal(ov.Models, []string{"claude-sonnet-4-5"}) {
			t.Errorf("%s models = %v, want [claude-sonnet-4-5]", ov.TraceID, ov.Models)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("traces = %v, want %v", got, want)
	}
	// The first chunk of 50 fills the page: 50 lists and no span details,
	// down from 50 roots and lists plus chain and two leaves for 25 matches.
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("GetSpanDetails calls = %d, want 0", got)
	}
	if got := atomic.LoadInt32(&fake.attrSpansCalls); got != 50 {
		t.Errorf("QueryTraceSpans calls with attributes = %d, want 50", got)
	}
}

// Model filters page through the matches. A trace the model rules out costs
// only the call that supplies its root: its span list, or its root fetch when
// it is over the span threshold or a status filter is set.
func TestGetTraceOverviews_ModelFilterPages(t *testing.T) {
	over := func(i int) bool { return i%5 == 0 }
	// model is trace i's model, or "" over the span threshold.
	model := func(i int) string {
		switch {
		case over(i):
			return ""
		case i%9 == 0:
			return "rare-model"
		case i%2 == 1:
			return "claude-sonnet-4-5"
		}
		return "gpt-4o-mini"
	}
	claude := func(i int) bool { return strings.HasPrefix(model(i), "claude") }
	all := func(int) bool { return true }
	tests := []struct {
		name    string
		filters TraceFilters
		// match picks the traces returned; root picks those the status filter passes.
		match, root func(i int) bool
	}{
		{name: "model", filters: TraceFilters{Model: "claude"}, match: claude, root: all},
		{name: "rare model", filters: TraceFilters{Model: "rare"},
			match: func(i int) bool { return model(i) == "rare-model" }, root: all},
		// Matches nothing, so the walk stops at the examine cap.
		{name: "model at cap", filters: TraceFilters{Model: "llama"}, match: func(int) bool { return false }, root: all},
		{name: "model and status", filters: TraceFilters{Model: "gpt", Status: TraceStatusError},
			match: func(i int) bool { return strings.HasPrefix(model(i), "gpt") && i%7 == 0 },
			root:  func(i int) bool { return i%7 == 0 }},
		{name: "model and minTokens", filters: TraceFilters{Model: "claude", MinTokens: ptr(47)},
			match: func(i int) bool { return claude(i) && 12+i%40 >= 47 }, root: all},
	}
	for _, tt := range tests {
		for _, order := range []string{"desc", "asc"} {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				params := lookBackParams(10)
				params.Filters = tt.filters
				params.SortOrder = order
				// rare-model on every 9th trace, an error on every 7th root, and
				// every 5th trace over the span threshold.
				fake := overThreshold(withModel(langGraphFake(600, errorEvery(7)), 9, "rare-model"), 5)

				pages := pagesOf(t, NewTracingController(openchoreo.New(fake)), params, 3)

				assertPages(t, pages, wantPages(fake, params, 3, tt.match))
				assertLangGraphRows(t, fake, pages)
				lists, roots, others := fetchedTraces(t, fake)
				if len(others) != 0 {
					t.Errorf("chain and leaf fetches for traces %v, want none", others)
				}
				for _, i := range lists {
					if over(i) || !tt.root(i) {
						t.Fatalf("trace-%04d got a span list", i)
					}
				}
				for _, i := range roots {
					if !over(i) && tt.filters.Status == TraceStatusAny {
						t.Fatalf("trace-%04d fetched its root, want it from the span list", i)
					}
				}
			})
		}
	}
}

// include=models without a filter: a page of 10 costs the trace list and an
// attribute list per row, which also supplies the root.
func TestGetTraceOverviews_IncludeModelsWithoutFilterCallCounts(t *testing.T) {
	params := lookBackParams(10)
	params.Filters = TraceFilters{}
	params.Include.Models = true
	fake := langGraphFake(30, noRootAttrs)

	pages := pagesOf(t, NewTracingController(openchoreo.New(fake)), params, 1)

	want := make([]string, 0, 10)
	for i := 0; i < 10; i++ {
		want = append(want, fmt.Sprintf("trace-%04d", i))
	}
	assertPages(t, pages, []wantPage{{ids: want, lookedBackTo: fake.traces[9].StartTime, more: true}})
	assertLangGraphRows(t, fake, pages)
	if got := atomic.LoadInt32(&fake.queryTracesCalls); got != 1 {
		t.Errorf("QueryTraces calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("GetSpanDetails calls = %d, want 0", got)
	}
	if got := atomic.LoadInt32(&fake.attrSpansCalls); got != 10 {
		t.Errorf("QueryTraceSpans calls with attributes = %d, want 10", got)
	}
}

// A model on 2% of traces, at the examine cap: one attribute list per examined
// trace and no span details.
func TestGetTraceOverviews_ModelTwoPercentCallCounts(t *testing.T) {
	params := lookBackParams(10)
	params.Filters = TraceFilters{Model: "rare"}
	want := make([]string, 0, 10)
	for i := 0; i < 500; i += 50 {
		want = append(want, fmt.Sprintf("trace-%04d", i))
	}

	fake := withModel(langGraphFake(600, noRootAttrs), 50, "rare-model")
	ids, _, _ := traceIDs(NewTracingController(openchoreo.New(fake)), t, params)

	if !slices.Equal(ids, want) {
		t.Fatalf("traces = %v, want %v", ids, want)
	}
	// The 10th match is in the 10th chunk of 50, so all 500 lists are fetched.
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("GetSpanDetails calls = %d, want 0", got)
	}
	if got := atomic.LoadInt32(&fake.attrSpansCalls); got != 500 {
		t.Errorf("QueryTraceSpans calls with attributes = %d, want 500", got)
	}
}
