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
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
	"github.com/wso2/agent-manager/agent-manager-observer/store"
	"github.com/wso2/agent-manager/agent-manager-observer/store/openchoreo"
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

// noRootAttrs adds no root attributes.
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

// upstreamCalls counts every upstream call the fake received.
func upstreamCalls(f *fakeObserverClient) int32 {
	return atomic.LoadInt32(&f.queryTracesCalls) + atomic.LoadInt32(&f.getSpanDetailsCalls) + atomic.LoadInt32(&f.queryTraceSpansCalls)
}

// wantPage is one expected trace-list page.
type wantPage struct {
	ids          []string
	lookedBackTo time.Time
	more         bool
	truncated    bool
}

// wantPages is up to n pages of fake's traces, where match(i) picks trace i.
// A page walks the traces in page order from just after the previous page's
// last trace, and stops at params.Limit matches or maxExaminedTraces examined
// traces.
func wantPages(fake *fakeObserverClient, params TraceQueryParams, n int, match func(i int) bool) []wantPage {
	asc := params.SortOrder == "asc"
	order := make([]int, len(fake.traces))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool {
		return compareTraces(store.Trace(fake.traces[order[a]]), store.Trace(fake.traces[order[b]]), asc) < 0
	})
	edge := params.StartTime
	if asc {
		edge = params.EndTime
	}
	var pages []wantPage
	start := 0
	for len(pages) < n {
		p := wantPage{lookedBackTo: edge}
		k := start
		for ; k < len(order) && k-start < maxExaminedTraces; k++ {
			if !match(order[k]) {
				continue
			}
			p.ids = append(p.ids, fake.traces[order[k]].TraceID)
			if len(p.ids) == params.Limit {
				break
			}
		}
		switch {
		case len(p.ids) == params.Limit && k < len(order)-1:
			p.lookedBackTo, p.more = fake.traces[order[k]].StartTime, true
		case len(p.ids) < params.Limit && k < len(order):
			k--
			p.lookedBackTo, p.more, p.truncated = fake.traces[order[k]].StartTime, true, true
		}
		pages = append(pages, p)
		if !p.more {
			return pages
		}
		// The next page starts just after the cursor trace.
		start = k + 1
	}
	return pages
}

// assertPages checks each page's trace IDs, cursor, truncation and lookedBackTo.
func assertPages(t *testing.T, got []*opensearch.TraceOverviewResponse, want []wantPage) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d pages, want %d", len(got), len(want))
	}
	for i, w := range want {
		ids := make([]string, len(got[i].Traces))
		for j, ov := range got[i].Traces {
			ids[j] = ov.TraceID
		}
		if !slices.Equal(ids, w.ids) {
			t.Errorf("page %d traces = %v, want %v", i, ids, w.ids)
		}
		if more := got[i].NextCursor != ""; more != w.more {
			t.Errorf("page %d has nextCursor %v, want %v", i, more, w.more)
		}
		if got[i].Truncated != w.truncated {
			t.Errorf("page %d truncated = %v, want %v", i, got[i].Truncated, w.truncated)
		}
		if lookedBackTo := formatCursor(w.lookedBackTo); got[i].LookedBackTo != lookedBackTo {
			t.Errorf("page %d lookedBackTo = %s, want %s", i, got[i].LookedBackTo, lookedBackTo)
		}
	}
}

// traceIndex is the fixture index in a langGraphFake trace or span ID.
func traceIndex(t *testing.T, id string) int {
	t.Helper()
	i, err := strconv.Atoi(id[len(id)-4:])
	if err != nil {
		t.Fatalf("no trace index in %q", id)
	}
	return i
}

// assertLangGraphRows checks each row's enriched fields against its langGraphFake trace.
func assertLangGraphRows(t *testing.T, fake *fakeObserverClient, pages []*opensearch.TraceOverviewResponse) {
	t.Helper()
	for _, page := range pages {
		for _, ov := range page.Traces {
			i := traceIndex(t, ov.TraceID)
			spans := fake.spansByTrace[ov.TraceID]
			root := spans[len(spans)-1].Attributes
			if ov.Input != fmt.Sprintf(`"in %d"`, i) || ov.Output != fmt.Sprintf("out %d", i) {
				t.Errorf("%s input/output = %v/%v, want the chain span's", ov.TraceID, ov.Input, ov.Output)
			}
			wantErrors := 0
			if _, ok := root["error.type"]; ok {
				wantErrors = 1
			}
			if ov.Status == nil || ov.Status.ErrorCount != wantErrors {
				t.Errorf("%s status = %+v, want errorCount %d", ov.TraceID, ov.Status, wantErrors)
			}
			if conv, _ := root["gen_ai.conversation.id"].(string); ov.ConversationID != conv {
				t.Errorf("%s conversationId = %q, want %q", ov.TraceID, ov.ConversationID, conv)
			}
			// Over the span threshold the leaves are never read.
			if fake.traces[i].SpanCount > skipLeafAggregationSpanCountThreshold {
				if ov.TokenUsage != nil || ov.Models != nil {
					t.Errorf("%s tokens/models = %+v/%v, want none", ov.TraceID, ov.TokenUsage, ov.Models)
				}
				continue
			}
			if ov.TokenUsage == nil || ov.TokenUsage.TotalTokens != 12+i%40 || ov.TokenUsage.Partial {
				t.Errorf("%s tokenUsage = %+v, want total %d from the leaves", ov.TraceID, ov.TokenUsage, 12+i%40)
			}
			model, _ := spans[1].Attributes["gen_ai.response.model"].(string)
			if !slices.Equal(ov.Models, []string{model}) {
				t.Errorf("%s models = %v, want [%s]", ov.TraceID, ov.Models, model)
			}
		}
	}
}

// fetchedTraces splits fake's per-trace calls into span lists, root fetches
// and other span fetches, as trace indexes.
func fetchedTraces(t *testing.T, fake *fakeObserverClient) (lists, roots, others []int) {
	t.Helper()
	fake.mu.Lock()
	defer fake.mu.Unlock()
	for _, id := range fake.spansTraceIDs {
		lists = append(lists, traceIndex(t, id))
	}
	for _, id := range fake.detailSpanIDs {
		if strings.HasPrefix(id, "root-") {
			roots = append(roots, traceIndex(t, id))
		} else {
			others = append(others, traceIndex(t, id))
		}
	}
	return lists, roots, others
}

// status=error over ok roots: one root fetch per examined trace, no span lists.
func TestGetTraceOverviews_StatusRejectsAtRoot(t *testing.T) {
	fake := langGraphFake(120, noRootAttrs)
	c := NewTracingController(openchoreo.New(fake))
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
	c := NewTracingController(openchoreo.New(fake))
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

// Root filters page through the matches, and a trace they reject costs only its root fetch.
func TestGetTraceOverviews_RootFilterPages(t *testing.T) {
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
	all := func(int) bool { return true }
	failed := func(i int) bool { return i%7 == 0 }
	ok := func(i int) bool { return i%7 != 0 }
	convMatch := func(i int) bool { return i%3 == 0 }
	tests := []struct {
		name    string
		filters TraceFilters
		// match picks the traces returned; root picks those the root filters pass.
		match, root func(i int) bool
	}{
		{name: "none", filters: TraceFilters{}, match: all, root: all},
		{name: "status error", filters: TraceFilters{Status: TraceStatusError}, match: failed, root: failed},
		{name: "status ok", filters: TraceFilters{Status: TraceStatusOK}, match: ok, root: ok},
		{name: "conversationId", filters: TraceFilters{ConversationID: "conv-match"}, match: convMatch, root: convMatch},
		// Matches nothing, so the walk stops at the examine cap.
		{name: "conversationId at cap", filters: TraceFilters{ConversationID: "conv-none"},
			match: func(int) bool { return false }, root: func(int) bool { return false }},
		{name: "status and minTokens", filters: TraceFilters{Status: TraceStatusError, MinTokens: ptr(47)},
			match: func(i int) bool { return failed(i) && 12+i%40 >= 47 }, root: failed},
		{name: "status and model", filters: TraceFilters{Status: TraceStatusOK, Model: "claude"},
			match: func(i int) bool { return ok(i) && i%2 == 1 }, root: ok},
	}
	for _, tt := range tests {
		for _, order := range []string{"desc", "asc"} {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				params := lookBackParams(10)
				params.Filters = tt.filters
				params.SortOrder = order
				fake := langGraphFake(600, rootAttrs)

				pages := pagesOf(t, NewTracingController(openchoreo.New(fake)), params, 3)

				assertPages(t, pages, wantPages(fake, params, 3, tt.match))
				assertLangGraphRows(t, fake, pages)
				lists, _, others := fetchedTraces(t, fake)
				for _, i := range append(lists, others...) {
					if !tt.root(i) {
						t.Fatalf("trace-%04d failed the root filters but was fetched past its root", i)
					}
				}
			})
		}
	}
}

// status=error matching 5% of traces: every examined trace costs its root
// fetch, and only the 10 matches get a span list.
func TestGetTraceOverviews_StatusErrorFivePercentCallCounts(t *testing.T) {
	params := lookBackParams(10)
	params.Filters = TraceFilters{Status: TraceStatusError}
	want := make([]string, 0, 10)
	for i := 0; i < 200; i += 20 {
		want = append(want, fmt.Sprintf("trace-%04d", i))
	}

	fake := langGraphFake(200, errorEvery(20))
	ids, _, truncated := traceIDs(NewTracingController(openchoreo.New(fake)), t, params)

	if !slices.Equal(ids, want) || truncated {
		t.Fatalf("traces = %v, truncated %v; want %v, false", ids, truncated, want)
	}
	// The 10th match is in the 4th chunk of 50, so all 200 roots are fetched.
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 200+10*3 {
		t.Errorf("GetSpanDetails calls = %d, want %d (200 roots, chain and two leaves per match)", got, 200+10*3)
	}
	if listed := slices.Sorted(slices.Values(fake.spansTraceIDs)); !slices.Equal(listed, want) {
		t.Errorf("QueryTraceSpans traces = %v, want the matches %v", listed, want)
	}
}
