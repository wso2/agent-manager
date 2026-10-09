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
	"log/slog"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
)

// matchesTools judges tool and toolError on tools and failedTools.
func TestMatchesTools(t *testing.T) {
	tests := []struct {
		name   string
		filter TraceFilters
		tools  []string
		failed []string
		want   bool
	}{
		{name: "no filter", tools: []string{"a"}, want: true},
		{name: "no filter, no tools", want: true},
		{name: "tool exact", filter: TraceFilters{Tool: "search_web"}, tools: []string{"search_web"}, want: true},
		{name: "tool substring", filter: TraceFilters{Tool: "search"}, tools: []string{"web_search_v2"}, want: true},
		{name: "tool ignores case", filter: TraceFilters{Tool: "SEARCH"}, tools: []string{"web_search"}, want: true},
		{name: "tool in a later entry", filter: TraceFilters{Tool: "calc"}, tools: []string{"search", "calculator"}, want: true},
		{name: "tool absent", filter: TraceFilters{Tool: "calc"}, tools: []string{"search"}},
		{name: "tool with nil tools", filter: TraceFilters{Tool: "calc"}},
		{name: "toolError with a failed tool", filter: TraceFilters{ToolError: true}, tools: []string{"a", "b"}, failed: []string{"b"}, want: true},
		{name: "toolError with none failed", filter: TraceFilters{ToolError: true}, tools: []string{"a"}},
		{name: "toolError with nil tools", filter: TraceFilters{ToolError: true}},
		{name: "pair: the failed tool matches", filter: TraceFilters{Tool: "search", ToolError: true},
			tools: []string{"search", "calc"}, failed: []string{"search"}, want: true},
		{name: "pair: another tool failed", filter: TraceFilters{Tool: "search", ToolError: true},
			tools: []string{"search", "calc"}, failed: []string{"calc"}},
		{name: "pair: matching tool ok, none failed", filter: TraceFilters{Tool: "search", ToolError: true}, tools: []string{"search"}},
		{name: "pair ignores case", filter: TraceFilters{Tool: "SEARCH", ToolError: true},
			tools: []string{"web_search"}, failed: []string{"web_search"}, want: true},
		{name: "pair with nil tools", filter: TraceFilters{Tool: "search", ToolError: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesTools(tt.tools, tt.failed, tt.filter); got != tt.want {
				t.Errorf("matchesTools(%v, %v, %+v) = %t, want %t", tt.tools, tt.failed, tt.filter, got, tt.want)
			}
		})
	}
}

// matchesFilters ANDs the tool filters with the others.
func TestMatchesFilters_Tools(t *testing.T) {
	ov := filterOverview()
	ov.Tools, ov.FailedTools = []string{"search", "calc"}, []string{"calc"}
	tests := []struct {
		name   string
		filter TraceFilters
		want   bool
	}{
		{name: "tool", filter: TraceFilters{Tool: "search"}, want: true},
		{name: "toolError", filter: TraceFilters{ToolError: true}, want: true},
		{name: "pair on the failed tool", filter: TraceFilters{Tool: "calc", ToolError: true}, want: true},
		{name: "pair on another tool", filter: TraceFilters{Tool: "search", ToolError: true}},
		{name: "tool and the status", filter: TraceFilters{Tool: "search", Status: TraceStatusError}, want: true},
		{name: "tool and another status", filter: TraceFilters{Tool: "search", Status: TraceStatusOK}},
		{name: "tool and the span count", filter: TraceFilters{Tool: "search", MinSpanCount: ptr(int64(ov.SpanCount))}, want: true},
		{name: "tool over the span cap", filter: TraceFilters{Tool: "search", MinSpanCount: ptr(int64(ov.SpanCount + 1))}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesFilters(ov, tt.filter); got != tt.want {
				t.Errorf("matchesFilters(%+v) = %t, want %t", tt.filter, got, tt.want)
			}
		})
	}
	ov.SpanCount = maxToolListSpans + 1
	if matchesFilters(ov, TraceFilters{Tool: "search"}) {
		t.Error("an overview over the span cap matched a tool filter")
	}
}

// A tool filter counts as a filter, isn't summary-only, and is logged.
func TestTraceFilters_ToolFields(t *testing.T) {
	for _, f := range []TraceFilters{{Tool: "a"}, {ToolError: true}, {Tool: "a", ToolError: true}} {
		if f.IsZero() || f.SummaryOnly() || !f.hasToolFilter() {
			t.Errorf("%+v: IsZero = %t, SummaryOnly = %t, hasToolFilter = %t", f, f.IsZero(), f.SummaryOnly(), f.hasToolFilter())
		}
	}
	if (TraceFilters{MinDurationMs: ptr(1)}).hasToolFilter() || !(TraceFilters{MinDurationMs: ptr(1)}).SummaryOnly() {
		t.Error("a summary filter reads as a tool filter or stopped being summary-only")
	}
	got := TraceFilters{Tool: "search", ToolError: true}.LogValue()
	want := slog.GroupValue(slog.String("tool", "search"), slog.Bool("toolError", true))
	if !got.Equal(want) {
		t.Errorf("LogValue = %v, want %v", got, want)
	}
	if got := (TraceFilters{}).LogValue(); len(got.Group()) != 0 {
		t.Errorf("LogValue of no filters = %v, want an empty group", got)
	}
}

// A filter implies its include and never turns one off.
func TestImpliedInclude(t *testing.T) {
	tests := []struct {
		name    string
		include Include
		filters TraceFilters
		want    Include
	}{
		{name: "nothing", want: Include{}},
		{name: "model", filters: TraceFilters{Model: "gpt"}, want: Include{Models: true}},
		{name: "tool", filters: TraceFilters{Tool: "a"}, want: Include{Tools: true}},
		{name: "toolError", filters: TraceFilters{ToolError: true}, want: Include{Tools: true}},
		{name: "model and tool", filters: TraceFilters{Model: "gpt", Tool: "a"}, want: Include{Models: true, Tools: true}},
		{name: "kept", include: Include{Models: true, Tools: true}, filters: TraceFilters{Status: TraceStatusError}, want: Include{Models: true, Tools: true}},
		{name: "other filters", filters: TraceFilters{Status: TraceStatusOK, MinTokens: ptr(1), ConversationID: "c"}, want: Include{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := impliedInclude(tt.include, tt.filters); got != tt.want {
				t.Errorf("impliedInclude(%+v, %+v) = %+v, want %+v", tt.include, tt.filters, got, tt.want)
			}
		})
	}
}

// Over maxToolListSpans a trace fails the summary check under a tool filter, and
// only under one.
func TestMatchesSummary_ToolSpanCap(t *testing.T) {
	over, at := maxToolListSpans+1, maxToolListSpans
	tests := []struct {
		name      string
		spanCount int
		filter    TraceFilters
		want      bool
	}{
		{name: "tool at the cap", spanCount: at, filter: TraceFilters{Tool: "a"}, want: true},
		{name: "tool over the cap", spanCount: over, filter: TraceFilters{Tool: "a"}},
		{name: "toolError over the cap", spanCount: over, filter: TraceFilters{ToolError: true}},
		{name: "toolError at the cap", spanCount: at, filter: TraceFilters{ToolError: true}, want: true},
		{name: "no filter over the cap", spanCount: over, want: true},
		{name: "model over the cap", spanCount: over, filter: TraceFilters{Model: "gpt"}, want: true},
		{name: "status over the cap", spanCount: over, filter: TraceFilters{Status: TraceStatusError}, want: true},
		{name: "minSpanCount over the cap", spanCount: over, filter: TraceFilters{MinSpanCount: ptr(1)}, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesSummary(0, tt.spanCount, tt.filter); got != tt.want {
				t.Errorf("matchesSummary(%d spans, %+v) = %t, want %t", tt.spanCount, tt.filter, got, tt.want)
			}
		})
	}
}

// overCap gives every nth trace (from 0) more spans than a tool filter reads.
func overCap(fake *fakeObserverClient, n int) *fakeObserverClient {
	for i := range fake.traces {
		if i%n == 0 {
			fake.traces[i].SpanCount = maxToolListSpans + 1
		}
	}
	return fake
}

// toolFilterFake is costFake with every 7th trace over the leaf threshold and
// every 11th over the tool span cap.
func toolFilterFake() *fakeObserverClient {
	return overCap(overThreshold(costFake(true), 7), 11)
}

// referenceOverviews enriches every trace of fake with no filter and the given
// include, by trace index. It is what a filtered request must agree with.
func referenceOverviews(t *testing.T, fake *fakeObserverClient, include Include) map[int]opensearch.TraceOverview {
	t.Helper()
	params := lookBackParams(len(fake.traces))
	params.Filters = TraceFilters{}
	params.Include = include
	resp, err := NewTracingController(fake).GetTraceOverviews(t.Context(), params)
	if err != nil {
		t.Fatalf("reference GetTraceOverviews: %v", err)
	}
	if len(resp.Traces) != len(fake.traces) {
		t.Fatalf("reference has %d traces, want %d", len(resp.Traces), len(fake.traces))
	}
	ref := make(map[int]opensearch.TraceOverview, len(resp.Traces))
	for _, ov := range resp.Traces {
		ref[traceIndex(t, ov.TraceID)] = ov
	}
	return ref
}

// A tool filter returns what the filter keeps of the fully enriched traces: the
// same pages, cursors, lookedBackTo and truncated, and the same rows, as if no
// trace had been rejected early. Whatever the filter rejected before its root
// or beyond its list was never fetched that far.
func TestGetTraceOverviews_ToolFiltersMatchFullEnrichment(t *testing.T) {
	tests := []struct {
		name    string
		filters TraceFilters
	}{
		{name: "tool", filters: TraceFilters{Tool: "search_issues"}},
		{name: "tool substring", filters: TraceFilters{Tool: "ISSUES"}},
		{name: "tool nobody calls", filters: TraceFilters{Tool: "send_email"}},
		{name: "toolError", filters: TraceFilters{ToolError: true}},
		{name: "tool and toolError", filters: TraceFilters{Tool: "search_issues", ToolError: true}},
		{name: "toolError for a tool nobody calls", filters: TraceFilters{Tool: "send_email", ToolError: true}},
		{name: "tool and status", filters: TraceFilters{Tool: "search_issues", Status: TraceStatusError}},
		{name: "toolError and status ok", filters: TraceFilters{ToolError: true, Status: TraceStatusOK}},
		{name: "tool and model", filters: TraceFilters{Tool: "search_issues", Model: "claude"}},
		{name: "toolError and model", filters: TraceFilters{ToolError: true, Model: "gpt"}},
		{name: "tool and minTokens", filters: TraceFilters{Tool: "search_issues", MinTokens: ptr(40)}},
		{name: "tool and conversationId", filters: TraceFilters{Tool: "search_issues", ConversationID: "conv-01"}},
		{name: "toolError and minDurationMs", filters: TraceFilters{ToolError: true, MinDurationMs: ptr(300)}},
		{name: "tool, status and model", filters: TraceFilters{Tool: "issues", Status: TraceStatusOK, Model: "gpt"}},
	}
	for _, tt := range tests {
		for _, order := range []string{"desc", "asc"} {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				include := impliedInclude(Include{}, tt.filters)
				ref := referenceOverviews(t, toolFilterFake(), include)
				match := func(i int) bool {
					ov, ok := ref[i]
					return ok && matchesFilters(ov, tt.filters)
				}
				params := lookBackParams(10)
				params.Filters = tt.filters
				params.SortOrder = order
				fake := toolFilterFake()

				pages := pagesListedOnce(t, NewTracingController(fake), fake, params, 3)

				assertPages(t, pages, wantPages(fake, params, 3, match))
				for _, page := range pages {
					for _, ov := range page.Traces {
						if want := ref[traceIndex(t, ov.TraceID)]; !reflect.DeepEqual(ov, want) {
							t.Errorf("%s row = %+v, want the fully enriched %+v", ov.TraceID, ov, want)
						}
					}
				}

				lists, roots, others := fetchedTraces(t, fake)
				toolsPass := func(i int) bool {
					ov := ref[i]
					return matchesTools(ov.Tools, ov.FailedTools, tt.filters)
				}
				rootFirst := tt.filters.Status != TraceStatusAny || tt.filters.ConversationID != ""
				for _, i := range others {
					if !toolsPass(i) {
						t.Errorf("trace-%04d failed the tool filter but was fetched past its list", i)
					}
				}
				for _, i := range roots {
					// A model filter reads an over-threshold trace's root before its list.
					modelAtRoot := tt.filters.Model != "" && fake.traces[i].SpanCount > skipLeafAggregationSpanCountThreshold
					if !rootFirst && !toolsPass(i) && !modelAtRoot {
						t.Errorf("trace-%04d failed the tool filter but fetched its root", i)
					}
				}
				for _, i := range lists {
					if fake.traces[i].SpanCount > maxToolListSpans {
						t.Errorf("trace-%04d is over the tool span cap but got a span list", i)
					}
				}
			})
		}
	}
}

// pagesListedOnce follows nextCursor for up to n pages, failing if a page lists
// a trace's spans twice.
func pagesListedOnce(t *testing.T, c *TracingController, fake *fakeObserverClient, params TraceQueryParams, n int) []*opensearch.TraceOverviewResponse {
	t.Helper()
	var pages []*opensearch.TraceOverviewResponse
	for len(pages) < n {
		fake.mu.Lock()
		from := len(fake.spansTraceIDs)
		fake.mu.Unlock()
		resp, err := c.GetTraceOverviews(t.Context(), params)
		if err != nil {
			t.Fatalf("GetTraceOverviews: %v", err)
		}
		fake.mu.Lock()
		listed := slices.Clone(fake.spansTraceIDs[from:])
		fake.mu.Unlock()
		seen := map[string]bool{}
		for _, id := range listed {
			if seen[id] {
				t.Errorf("page %d listed %s's spans twice", len(pages), id)
			}
			seen[id] = true
		}
		pages = append(pages, resp)
		if resp.NextCursor == "" {
			break
		}
		if params.Cursor, err = DecodeTraceCursor(resp.NextCursor); err != nil {
			t.Fatalf("DecodeTraceCursor: %v", err)
		}
	}
	return pages
}

// duplicate returns an index that appears twice, or -1.
func duplicate(indexes []int) int {
	seen := map[int]bool{}
	for _, i := range indexes {
		if seen[i] {
			return i
		}
		seen[i] = true
	}
	return -1
}

// toolError over costFake: every examined trace costs one span list without
// attributes and no span details. Only the 10 matches fetch their root, chain and leaves.
func TestGetTraceOverviews_ToolErrorCallCounts(t *testing.T) {
	fake := costFake(true)
	params := lookBackParams(10)
	params.Filters = TraceFilters{ToolError: true}

	ids, _, truncated := traceIDs(NewTracingController(fake), t, params)

	want := make([]string, 0, 10)
	for i := 0; i < 300; i += 30 {
		want = append(want, fmt.Sprintf("trace-%04d", i))
	}
	if !slices.Equal(ids, want) || truncated {
		t.Fatalf("traces = %v, truncated %v; want %v, false", ids, truncated, want)
	}
	if got := atomic.LoadInt32(&fake.attrSpansCalls); got != 0 {
		t.Errorf("QueryTraceSpans calls with attributes = %d, want 0", got)
	}
	lists, roots, others := fetchedTraces(t, fake)
	if dup := duplicate(lists); dup >= 0 {
		t.Errorf("trace-%04d got more than one span list", dup)
	}
	// The 10th match is in the 6th chunk of 50.
	if len(lists) != 300 {
		t.Errorf("span lists = %d, want 300, one per examined trace", len(lists))
	}
	if len(roots) != 10 || len(others) != 30 {
		t.Errorf("root fetches = %d, other span fetches = %d; want 10 and 30, the matches' cascade only", len(roots), len(others))
	}
	for _, i := range append(roots, others...) {
		if i%30 != 0 {
			t.Errorf("trace-%04d is not a match but was fetched past its list", i)
		}
	}
}

// With a status filter the root comes first, so only root survivors get a list.
func TestGetTraceOverviews_ToolWithStatusReadsRootFirst(t *testing.T) {
	fake := costFake(true)
	params := lookBackParams(5)
	params.Filters = TraceFilters{Tool: "search_issues", Status: TraceStatusError}

	ids, _, _ := traceIDs(NewTracingController(fake), t, params)

	// Roots fail on every 20th trace and tools run on every 3rd: every 60th matches.
	want := []string{"trace-0000", "trace-0060", "trace-0120", "trace-0180", "trace-0240"}
	if !slices.Equal(ids, want) {
		t.Fatalf("traces = %v, want %v", ids, want)
	}
	lists, roots, _ := fetchedTraces(t, fake)
	// The 5th match is in the 5th chunk of 50.
	if len(roots) != 250 {
		t.Errorf("root fetches = %d, want 250, one per examined trace", len(roots))
	}
	for _, i := range lists {
		if i%20 != 0 {
			t.Errorf("trace-%04d got a span list but its root passes no status filter", i)
		}
	}
}

// A tool and a model filter read one attribute list per trace, which also
// supplies the root: no second list and no span details.
func TestGetTraceOverviews_ToolWithModelOneAttributeList(t *testing.T) {
	fake := costFake(true)
	params := lookBackParams(5)
	params.Filters = TraceFilters{Tool: "search_issues", Model: "gpt"}

	ids, _, _ := traceIDs(NewTracingController(fake), t, params)

	// Tools run on every 3rd trace and gpt-4o-mini on the even ones, except
	// every 50th, which has rare-model.
	want := []string{"trace-0006", "trace-0012", "trace-0018", "trace-0024", "trace-0030"}
	if !slices.Equal(ids, want) {
		t.Fatalf("traces = %v, want %v", ids, want)
	}
	lists, _, _ := fetchedTraces(t, fake)
	if dup := duplicate(lists); dup >= 0 {
		t.Errorf("trace-%04d got more than one span list", dup)
	}
	if got, all := atomic.LoadInt32(&fake.attrSpansCalls), atomic.LoadInt32(&fake.queryTraceSpansCalls); got != all || int(all) != len(lists) {
		t.Errorf("attribute lists = %d of %d span lists, want every list to carry attributes", got, all)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("GetSpanDetails calls = %d, want 0", got)
	}
}

// A trace over the tool span cap is rejected from the trace list: no span list
// and no span details.
func TestGetTraceOverviews_ToolFilterOverCapCostsNothing(t *testing.T) {
	for _, filters := range []TraceFilters{{Tool: "search_issues"}, {ToolError: true}, {Tool: "search_issues", ToolError: true}} {
		fake := costOverCapFake()
		params := lookBackParams(10)
		params.Filters = filters

		ids, _, truncated := traceIDs(NewTracingController(fake), t, params)

		if len(ids) != 0 || !truncated {
			t.Errorf("%+v: got %d traces, truncated %v; want 0, true", filters, len(ids), truncated)
		}
		if got := atomic.LoadInt32(&fake.queryTraceSpansCalls) + atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
			t.Errorf("%+v: per-trace calls = %d, want 0", filters, got)
		}
	}
}

// A tool and a model filter on a trace over the leaf threshold: its root
// rejects it on the model, with no span list.
func TestGetTraceOverviews_ToolWithModelOverThresholdRejectsAtRoot(t *testing.T) {
	fake := overThreshold(costFake(true), 1)
	params := lookBackParams(10)
	params.Filters = TraceFilters{Tool: "search_issues", Model: "gpt"}

	ids, _, _ := traceIDs(NewTracingController(fake), t, params)

	if len(ids) != 0 {
		t.Fatalf("traces = %v, want none", ids)
	}
	if got := atomic.LoadInt32(&fake.queryTraceSpansCalls); got != 0 {
		t.Errorf("QueryTraceSpans calls = %d, want 0", got)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != maxExaminedTraces {
		t.Errorf("GetSpanDetails calls = %d, want %d, one root per examined trace", got, maxExaminedTraces)
	}
}

// An export with toolError selects as the list does and fetches full spans for
// the matches only.
func TestExportTraces_ToolErrorFetchesSpansOnlyForMatches(t *testing.T) {
	fake := costFake(true)

	resp := mustExport(t, NewTracingController(fake), exportParams(10, TraceFilters{ToolError: true}))

	want := make([]string, 0, 10)
	for i := 0; i < 300; i += 30 {
		want = append(want, fmt.Sprintf("trace-%04d", i))
	}
	if got := exportedIDs(resp); !slices.Equal(got, want) {
		t.Fatalf("exported %v, want %v", got, want)
	}
	if got := atomic.LoadInt32(&fake.attrSpansCalls); got != 10 {
		t.Errorf("full span fetches = %d, want 10, one per match", got)
	}
	page, err := NewTracingController(costFake(true)).GetTraceOverviews(t.Context(), exportParams(10, TraceFilters{ToolError: true}))
	if err != nil {
		t.Fatalf("GetTraceOverviews: %v", err)
	}
	listed := make([]string, len(page.Traces))
	for i, ov := range page.Traces {
		listed[i] = ov.TraceID
	}
	if !slices.Equal(listed, want) {
		t.Errorf("list selected %v, want the export's %v", listed, want)
	}
}

// Paging by cursor through a window whose traces overlap returns each toolError
// match exactly once, in both sort orders.
func TestGetTraceOverviews_ToolErrorCursorPagesWholeWindow(t *testing.T) {
	for _, order := range []string{"desc", "asc"} {
		t.Run(order, func(t *testing.T) {
			fake := costFake(true)
			fake.traces = fake.traces[:300]
			for i := range fake.traces {
				fake.traces[i].EndTime = fake.traces[i].StartTime.Add(5 * time.Second)
			}
			params := lookBackParams(3)
			params.Filters = TraceFilters{ToolError: true}
			params.SortOrder = order

			pages := pageAll(t, NewTracingController(fake), fake, params)

			// Traces 0-3 end after the window, so their roots are outside it.
			want := map[string]bool{}
			for i := 4; i < 300; i++ {
				if i%30 == 0 {
					want[fmt.Sprintf("trace-%04d", i)] = true
				}
			}
			assertIDs(t, unionIDs(pages), want)
			assertNoDuplicates(t, pageIDs(pages))
			for i, p := range pages {
				if p.resp.Truncated {
					t.Errorf("page %d truncated", i)
				}
			}
		})
	}
}

// A trace whose span list can't be read can't be judged by a tool filter. It is
// listed as failed, the list-fetch warning is logged and the root-fetch one never is.
func TestGetTraceOverviews_ToolFilterSpanListFails(t *testing.T) {
	tests := []struct {
		name    string
		fixture func() *fakeObserverClient
		filters TraceFilters
	}{
		{name: "tool", fixture: func() *fakeObserverClient { return costFake(true) }, filters: TraceFilters{ToolError: true}},
		{name: "tool and status", fixture: func() *fakeObserverClient { return costFake(true) },
			filters: TraceFilters{ToolError: true, Status: TraceStatusError}},
		{name: "tool and model", fixture: func() *fakeObserverClient { return costFake(true) },
			filters: TraceFilters{ToolError: true, Model: "gpt"}},
		{name: "root-complete", fixture: costRootCompleteFake, filters: TraceFilters{ToolError: true}},
		{name: "root-complete with status", fixture: costRootCompleteFake,
			filters: TraceFilters{ToolError: true, Status: TraceStatusError}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := tt.fixture()
			fake.failCall = failTimes(map[string]int{"trace-0000": -1})
			params := lookBackParams(3)
			params.Filters = tt.filters
			ctx, logs := logContext()

			resp, err := NewTracingController(fake).GetTraceOverviews(ctx, params)
			if err != nil {
				t.Fatalf("GetTraceOverviews: %v", err)
			}

			// trace-0000 would match; it is left out, and the walk moves on to 30, 60 and 90.
			want := []string{"trace-0030", "trace-0060", "trace-0090"}
			if tt.filters.Status != TraceStatusAny {
				want = []string{"trace-0060", "trace-0120", "trace-0180"}
			}
			got := make([]string, len(resp.Traces))
			for i, ov := range resp.Traces {
				got[i] = ov.TraceID
			}
			if !slices.Equal(got, want) {
				t.Errorf("traces = %v, want %v", got, want)
			}
			if got := logField(t, logs, "Retrieved trace overviews", "failed"); got != float64(1) {
				t.Errorf("failed logged as %v, want 1", got)
			}
			// One warning per attempt, and the walk retries a failed trace once.
			wantWarns := []string{
				"enrichTraceOverview: QueryTraceSpans failed, skipping enrichment",
				"enrichTraceOverview: QueryTraceSpans failed, skipping enrichment",
			}
			if got := warnings(logs); !slices.Equal(got, wantWarns) {
				t.Errorf("warnings = %q, want %q", got, wantWarns)
			}
			if n := spanFetches(fake, "trace-0000"); n != 2 {
				t.Errorf("span list calls for trace-0000 = %d, want 2, the attempt and its retry", n)
			}
		})
	}
}

// An export lists the trace whose span list failed in failedTraceIds and leaves it out.
func TestExportTraces_ToolFilterSpanListFailsInSelection(t *testing.T) {
	fake := costFake(true)
	fake.failCall = failTimes(map[string]int{"trace-0000": -1})

	resp := mustExport(t, NewTracingController(fake), exportParams(3, TraceFilters{ToolError: true}))

	if got, want := exportedIDs(resp), []string{"trace-0030", "trace-0060", "trace-0090"}; !slices.Equal(got, want) {
		t.Fatalf("exported %v, want %v", got, want)
	}
	if want := []string{"trace-0000"}; !slices.Equal(resp.FailedTraceIDs, want) {
		t.Errorf("failedTraceIds = %v, want %v", resp.FailedTraceIDs, want)
	}
}

// A trace with no spans has no tools: a tool filter rules it out, it isn't listed as failed.
func TestGetTraceOverviews_ToolFilterEmptySpanList(t *testing.T) {
	fake := costFake(true)
	for _, info := range fake.traces {
		fake.spansByTrace[info.TraceID] = []observer.SpanInfo{}
	}
	params := lookBackParams(3)
	params.Filters = TraceFilters{ToolError: true}
	ctx, logs := logContext()

	resp, err := NewTracingController(fake).GetTraceOverviews(ctx, params)
	if err != nil {
		t.Fatalf("GetTraceOverviews: %v", err)
	}

	if len(resp.Traces) != 0 || !resp.Truncated {
		t.Errorf("got %d traces, truncated %v; want 0, true", len(resp.Traces), resp.Truncated)
	}
	if got := logField(t, logs, "Retrieved trace overviews", "failed"); got != float64(0) {
		t.Errorf("failed logged as %v, want 0", got)
	}
	lists, roots, _ := fetchedTraces(t, fake)
	if len(roots) != 0 || duplicate(lists) >= 0 {
		t.Errorf("root fetches = %d, a trace listed twice = %t; want none", len(roots), duplicate(lists) >= 0)
	}
}
