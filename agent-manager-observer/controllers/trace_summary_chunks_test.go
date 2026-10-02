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
	"slices"
	"strings"
	"testing"
	"time"
)

// longEvery gives trace i a 2 s duration when long(i), and 0 otherwise.
func longEvery(fake *fakeObserverClient, long func(i int) bool) *fakeObserverClient {
	for i := range fake.traces {
		fake.traces[i].DurationNs = 0
		if long(i) {
			fake.traces[i].DurationNs = int64(2 * time.Second)
		}
	}
	return fake
}

// sixtyPercent passes 3 of every 5 traces.
func sixtyPercent(i int) bool { return i%5 < 3 }

// rootFetches counts GetSpanDetails calls for root spans.
func rootFetches(f *fakeObserverClient) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for _, id := range f.detailSpanIDs {
		if strings.HasPrefix(id, "root-") {
			n++
		}
	}
	return n
}

// longIDs is the first n trace IDs that long passes.
func longIDs(n int, long func(i int) bool) []string {
	ids := make([]string, 0, n)
	for i := 0; len(ids) < n; i++ {
		if long(i) {
			ids = append(ids, fmt.Sprintf("trace-%04d", i))
		}
	}
	return ids
}

func TestTraceFilters_SummaryOnly(t *testing.T) {
	tests := []struct {
		name    string
		filters TraceFilters
		want    bool
	}{
		{name: "none", filters: TraceFilters{}, want: false},
		{name: "minDurationMs", filters: TraceFilters{MinDurationMs: ptr(0)}, want: true},
		{name: "minSpanCount", filters: TraceFilters{MinSpanCount: ptr(3)}, want: true},
		{name: "both", filters: TraceFilters{MinDurationMs: ptr(1), MinSpanCount: ptr(3)}, want: true},
		{name: "with minTokens", filters: TraceFilters{MinDurationMs: ptr(1), MinTokens: ptr(3)}, want: false},
		{name: "with status", filters: TraceFilters{MinSpanCount: ptr(1), Status: TraceStatusOK}, want: false},
		{name: "with model", filters: TraceFilters{MinSpanCount: ptr(1), Model: "gpt"}, want: false},
		{name: "with conversationId", filters: TraceFilters{MinSpanCount: ptr(1), ConversationID: "c"}, want: false},
		{name: "minTokens only", filters: TraceFilters{MinTokens: ptr(3)}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.filters.SummaryOnly(); got != tt.want {
				t.Errorf("SummaryOnly() = %v, want %v", got, tt.want)
			}
		})
	}
}

// minDurationMs passing 60%: only the 10 traces the page returns are enriched.
// The full LangGraph cascade drops from 151 upstream calls to 51.
func TestGetTraceOverviews_SummaryOnlyEnrichesPage(t *testing.T) {
	params := lookBackParams(10)
	params.Filters = TraceFilters{MinDurationMs: ptr(1000)}
	want := longIDs(10, sixtyPercent)

	fullFake := longEvery(langGraphFake(200, noRootAttrs), sixtyPercent)
	fullIDs, _, _ := traceIDs(&TracingController{observerClient: fullFake, fullChunks: true}, t, params)
	fake := longEvery(langGraphFake(200, noRootAttrs), sixtyPercent)
	ids, _, truncated := traceIDs(NewTracingController(fake), t, params)

	if !slices.Equal(ids, want) || !slices.Equal(fullIDs, want) || truncated {
		t.Fatalf("traces = %v (full chunks %v), truncated %v; want %v, false", ids, fullIDs, truncated, want)
	}
	if got := rootFetches(fake); got != 10 {
		t.Errorf("root GetSpanDetails calls = %d, want 10", got)
	}
	// One list query, then root, span list, chain and two leaves per match.
	if got := upstreamCalls(fake); got != 1+10*5 {
		t.Errorf("upstream calls = %d, want %d", got, 1+10*5)
	}
	// The first chunk of 50 holds 30 survivors, all enriched.
	if got := upstreamCalls(fullFake); got != 1+30*5 {
		t.Errorf("full chunks upstream calls = %d, want %d", got, 1+30*5)
	}
}

// A failed root fetch costs one more enrichment, and the page still fills.
func TestGetTraceOverviews_SummaryOnlyRootFetchFails(t *testing.T) {
	params := lookBackParams(10)
	params.Filters = TraceFilters{MinDurationMs: ptr(1000)}
	newFake := func() *fakeObserverClient {
		fake := longEvery(langGraphFake(200, noRootAttrs), sixtyPercent)
		delete(fake.spanDetails, "root-0002")
		return fake
	}
	want := slices.DeleteFunc(longIDs(11, sixtyPercent), func(id string) bool { return id == "trace-0002" })

	fullResp := pagesOf(t, &TracingController{observerClient: newFake(), fullChunks: true}, params, 1)[0]
	fake := newFake()
	resp := pagesOf(t, NewTracingController(fake), params, 1)[0]

	ids := make([]string, len(resp.Traces))
	for i, ov := range resp.Traces {
		ids[i] = ov.TraceID
	}
	if !slices.Equal(ids, want) {
		t.Fatalf("traces = %v, want %v", ids, want)
	}
	if !reflect.DeepEqual(resp, fullResp) {
		t.Errorf("response differs from full chunks:\n got %+v\nwant %+v", resp, fullResp)
	}
	if got := rootFetches(fake); got != 11 {
		t.Errorf("root GetSpanDetails calls = %d, want 11", got)
	}
}

// Page-sized chunks return the same pages as full chunks, and make fewer
// upstream calls whenever the page fills.
func TestGetTraceOverviews_SummaryOnlyKeepsResults(t *testing.T) {
	// Spans per trace: 4, or 6 on every 4th.
	moreSpans := func(fake *fakeObserverClient) {
		for i := range fake.traces {
			if i%4 == 0 {
				fake.traces[i].SpanCount = 6
			}
		}
	}
	// Traces 10-39 share one start time, so cursor pages repeat ties.
	ties := func(fake *fakeObserverClient) {
		moreSpans(fake)
		for i := 10; i < 40; i++ {
			fake.traces[i].StartTime = fake.traces[10].StartTime
			fake.traces[i].EndTime = fake.traces[10].StartTime
		}
	}
	tests := []struct {
		name    string
		filters TraceFilters
		models  bool
		mutate  func(*fakeObserverClient)
		// fills is false when no page fills, so the calls are equal.
		fills bool
	}{
		{name: "minDurationMs", filters: TraceFilters{MinDurationMs: ptr(1000)}, mutate: moreSpans, fills: true},
		{name: "minSpanCount", filters: TraceFilters{MinSpanCount: ptr(5)}, mutate: moreSpans, fills: true},
		{name: "both", filters: TraceFilters{MinDurationMs: ptr(1000), MinSpanCount: ptr(5)}, mutate: moreSpans, fills: true},
		{name: "includeModels", filters: TraceFilters{MinDurationMs: ptr(1000)}, models: true, mutate: moreSpans, fills: true},
		{name: "ties at the cursor", filters: TraceFilters{MinSpanCount: ptr(4)}, mutate: ties, fills: true},
		// Matches nothing, so the walk stops at the examine cap.
		{name: "at cap", filters: TraceFilters{MinSpanCount: ptr(100)}, mutate: moreSpans},
	}
	for _, tt := range tests {
		for _, order := range []string{"desc", "asc"} {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				params := lookBackParams(10)
				params.Filters = tt.filters
				params.Include.Models = tt.models
				params.SortOrder = order
				newFake := func() *fakeObserverClient {
					fake := longEvery(langGraphFake(600, noRootAttrs), sixtyPercent)
					tt.mutate(fake)
					return fake
				}
				fake, fullFake := newFake(), newFake()

				got := pagesOf(t, NewTracingController(fake), params, 3)
				want := pagesOf(t, &TracingController{observerClient: fullFake, fullChunks: true}, params, 3)

				if len(got) != len(want) {
					t.Fatalf("got %d pages, want %d", len(got), len(want))
				}
				for i := range want {
					if !reflect.DeepEqual(got[i], want[i]) {
						t.Errorf("page %d differs:\n got %+v\nwant %+v", i, got[i], want[i])
					}
				}
				calls, fullCalls := upstreamCalls(fake), upstreamCalls(fullFake)
				if tt.fills && calls >= fullCalls {
					t.Errorf("upstream calls = %d, want fewer than %d", calls, fullCalls)
				}
				if !tt.fills && calls != fullCalls {
					t.Errorf("upstream calls = %d, want %d", calls, fullCalls)
				}
			})
		}
	}
}
