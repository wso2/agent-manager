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
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

var lookBackWindowEnd = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// lookBackFake scripts n zero-length traces one second apart, newest first,
// inside a one-day window. Every matchEvery-th trace (from 0) carries
// conversation ID conv-match.
func lookBackFake(n, matchEvery int) *fakeObserverClient {
	fake := &fakeObserverClient{windowed: true, spanDetails: map[string]*observer.SpanDetailsResponse{}}
	for i := 0; i < n; i++ {
		info := baseTraceInfo(2)
		info.TraceID = fmt.Sprintf("trace-%04d", i)
		info.RootSpanID = fmt.Sprintf("root-%04d", i)
		info.StartTime = lookBackWindowEnd.Add(-time.Duration(i+1) * time.Second)
		info.EndTime = info.StartTime
		fake.traces = append(fake.traces, info)

		attrs := completeRootAttrs()
		attrs["gen_ai.conversation.id"] = "conv-other"
		if i%matchEvery == 0 {
			attrs["gen_ai.conversation.id"] = "conv-match"
		}
		fake.spanDetails[info.RootSpanID] = &observer.SpanDetailsResponse{
			SpanID: info.RootSpanID, SpanName: "invoke_agent LangGraph", Attributes: attrs,
		}
	}
	return fake
}

func lookBackParams(limit int) TraceQueryParams {
	params := baseParams()
	params.StartTime = lookBackWindowEnd.Add(-24 * time.Hour)
	params.EndTime = lookBackWindowEnd
	params.Limit = limit
	params.Filters.ConversationID = "conv-match"
	return params
}

func assertNoDuplicates(t *testing.T, ids []string) {
	t.Helper()
	seen := make(map[string]bool, len(ids))
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("trace %s returned twice", id)
		}
		seen[id] = true
	}
}

func traceIDs(c *TracingController, t *testing.T, params TraceQueryParams) ([]string, string, bool) {
	t.Helper()
	resp, err := c.GetTraceOverviews(context.Background(), params)
	if err != nil {
		t.Fatalf("GetTraceOverviews returned error: %v", err)
	}
	if !params.Filters.IsZero() && resp.TotalCount != len(resp.Traces) {
		t.Errorf("totalCount = %d, want %d", resp.TotalCount, len(resp.Traces))
	}
	ids := make([]string, len(resp.Traces))
	for i, tr := range resp.Traces {
		ids[i] = tr.TraceID
	}
	return ids, resp.LookedBackTo, resp.Truncated
}

// No filter: one QueryTraces call for the requested limit.
func TestGetTraceOverviews_NoFilterQueriesOnce(t *testing.T) {
	fake := lookBackFake(200, 5)
	c := NewTracingController(fake)
	params := lookBackParams(20)
	params.Filters = TraceFilters{}

	ids, lookedBackTo, truncated := traceIDs(c, t, params)

	if got := atomic.LoadInt32(&fake.queryTracesCalls); got != 1 {
		t.Fatalf("QueryTraces calls = %d, want 1", got)
	}
	if got := *fake.tracesReqs[0].Limit; got != 20 {
		t.Errorf("QueryTraces limit = %d, want 20", got)
	}
	if len(ids) != 20 || truncated {
		t.Errorf("got %d traces, truncated %v; want 20, false", len(ids), truncated)
	}
	if want := formatCursor(fake.traces[19].StartTime); lookedBackTo != want {
		t.Errorf("lookedBackTo = %s, want %s", lookedBackTo, want)
	}
}

// A 1-in-5 filter keeps looking back until the page is full.
func TestGetTraceOverviews_LookBackFillsPage(t *testing.T) {
	fake := lookBackFake(400, 5)
	c := NewTracingController(fake)

	ids, lookedBackTo, truncated := traceIDs(c, t, lookBackParams(25))

	if len(ids) != 25 || truncated {
		t.Fatalf("got %d traces, truncated %v; want 25, false", len(ids), truncated)
	}
	for i, id := range ids {
		if want := fmt.Sprintf("trace-%04d", i*5); id != want {
			t.Fatalf("traces[%d] = %s, want %s", i, id, want)
		}
	}
	// The 25th match is trace 120, in the third fetch.
	if want := formatCursor(fake.traces[120].StartTime); lookedBackTo != want {
		t.Errorf("lookedBackTo = %s, want %s", lookedBackTo, want)
	}
	wantLimits := []int{50, 100, 200}
	if len(fake.tracesReqs) != len(wantLimits) {
		t.Fatalf("QueryTraces calls = %d, want %d", len(fake.tracesReqs), len(wantLimits))
	}
	for i, req := range fake.tracesReqs {
		if *req.Limit != wantLimits[i] {
			t.Errorf("fetch %d limit = %d, want %d", i, *req.Limit, wantLimits[i])
		}
		if !req.StartTime.Equal(fake.tracesReqs[0].StartTime) || !req.EndTime.Equal(fake.tracesReqs[0].EndTime) {
			t.Errorf("fetch %d window = %s..%s, want the request window", i, req.StartTime, req.EndTime)
		}
	}
}

// A filter that never matches stops at the examine cap.
func TestGetTraceOverviews_LookBackStopsAtCap(t *testing.T) {
	fake := lookBackFake(1000, 5)
	c := NewTracingController(fake)
	params := lookBackParams(10)
	params.Filters.ConversationID = "conv-none"

	ids, lookedBackTo, truncated := traceIDs(c, t, params)

	if len(ids) != 0 || !truncated {
		t.Fatalf("got %d traces, truncated %v; want 0, true", len(ids), truncated)
	}
	if want := formatCursor(fake.traces[maxExaminedTraces-1].StartTime); lookedBackTo != want {
		t.Errorf("lookedBackTo = %s, want %s", lookedBackTo, want)
	}
	// Each examined trace costs one root fetch, so the cap bounds enrichment.
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != maxExaminedTraces {
		t.Errorf("GetSpanDetails calls = %d, want %d", got, maxExaminedTraces)
	}
}

// Running out of window before the cap is not truncation.
func TestGetTraceOverviews_LookBackWindowExhausted(t *testing.T) {
	tests := []struct {
		name      string
		sortOrder string
		wantEdge  func(TraceQueryParams) time.Time
	}{
		{name: "desc", sortOrder: "desc", wantEdge: func(p TraceQueryParams) time.Time { return p.StartTime }},
		{name: "asc", sortOrder: "asc", wantEdge: func(p TraceQueryParams) time.Time { return p.EndTime }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := lookBackFake(120, 5)
			c := NewTracingController(fake)
			params := lookBackParams(100)
			params.SortOrder = tt.sortOrder

			ids, lookedBackTo, truncated := traceIDs(c, t, params)

			if len(ids) != 24 || truncated {
				t.Fatalf("got %d traces, truncated %v; want 24, false", len(ids), truncated)
			}
			if want := formatCursor(tt.wantEdge(params)); lookedBackTo != want {
				t.Errorf("lookedBackTo = %s, want %s", lookedBackTo, want)
			}
		})
	}
}

// Ascending order pages forward, so lookedBackTo is the newest trace examined.
func TestGetTraceOverviews_LookBackAscending(t *testing.T) {
	fake := lookBackFake(400, 5)
	c := NewTracingController(fake)
	params := lookBackParams(25)
	params.SortOrder = "asc"

	ids, lookedBackTo, _ := traceIDs(c, t, params)

	if len(ids) != 25 || ids[0] != "trace-0395" {
		t.Fatalf("got %d traces starting at %v; want 25 starting at trace-0395", len(ids), ids)
	}
	// Oldest-first matches are 395, 390, ...; the 25th is trace 275.
	if want := formatCursor(fake.traces[275].StartTime); lookedBackTo != want {
		t.Errorf("lookedBackTo = %s, want %s", lookedBackTo, want)
	}
}

// Traces sharing a timestamp at a fetch boundary are neither lost nor repeated.
func TestGetTraceOverviews_LookBackSharedBoundaryTimestamp(t *testing.T) {
	fake := lookBackFake(120, 1)
	// Traces 45-54 straddle the first fetch boundary at one timestamp.
	for i := 45; i < 55; i++ {
		fake.traces[i].StartTime = fake.traces[45].StartTime
	}
	c := NewTracingController(fake)

	ids, _, _ := traceIDs(c, t, lookBackParams(200))

	assertNoDuplicates(t, ids)
	if len(ids) != 120 {
		t.Errorf("got %d traces, want all 120", len(ids))
	}
}

// More than a fetch at one timestamp returns each trace once.
func TestGetTraceOverviews_LookBackBatchAtOneTimestamp(t *testing.T) {
	fake := lookBackFake(80, 1)
	for i := range fake.traces {
		fake.traces[i].StartTime = fake.traces[0].StartTime
		fake.traces[i].EndTime = fake.traces[0].StartTime
	}
	c := NewTracingController(fake)

	ids, _, truncated := traceIDs(c, t, lookBackParams(200))

	assertNoDuplicates(t, ids)
	if len(ids) != 80 || truncated {
		t.Errorf("got %d traces, truncated %v; want 80, false", len(ids), truncated)
	}
	if got := atomic.LoadInt32(&fake.queryTracesCalls); got != 2 {
		t.Errorf("QueryTraces calls = %d, want 2", got)
	}
}

// Traces that overlap a fetch boundary are kept, and traces whose root ends
// after the window, which still take up upstream limit slots, do not end the
// look-back early.
func TestGetTraceOverviews_LookBackOverlappingTraces(t *testing.T) {
	for _, sortOrder := range []string{"desc", "asc"} {
		t.Run(sortOrder, func(t *testing.T) {
			fake := lookBackFake(300, 1)
			for i := range fake.traces {
				fake.traces[i].EndTime = fake.traces[i].StartTime.Add(5 * time.Second)
			}
			c := NewTracingController(fake)
			params := lookBackParams(1000)
			params.SortOrder = sortOrder

			ids, lookedBackTo, truncated := traceIDs(c, t, params)

			assertNoDuplicates(t, ids)
			// Traces 0-3 end after the window, so their roots are out of it.
			if len(ids) != 296 || truncated {
				t.Fatalf("got %d traces, truncated %v; want 296, false", len(ids), truncated)
			}
			if want := formatCursor(windowEdge(params)); lookedBackTo != want {
				t.Errorf("lookedBackTo = %s, want %s", lookedBackTo, want)
			}
		})
	}
}

// Summary filters drop traces before enrichment but still count as examined.
func TestGetTraceOverviews_LookBackPreFilterCountsAsExamined(t *testing.T) {
	fake := lookBackFake(1000, 1)
	c := NewTracingController(fake)
	params := lookBackParams(10)
	params.Filters = TraceFilters{MinSpanCount: ptr(100)}

	ids, _, truncated := traceIDs(c, t, params)

	if len(ids) != 0 || !truncated {
		t.Fatalf("got %d traces, truncated %v; want 0, true", len(ids), truncated)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != 0 {
		t.Errorf("GetSpanDetails calls = %d, want 0", got)
	}
}

// A cancelled context stops the loop between batches.
func TestGetTraceOverviews_LookBackHonoursCancel(t *testing.T) {
	fake := lookBackFake(400, 50)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake.onQueryTraces = cancel
	c := NewTracingController(fake)

	_, err := c.GetTraceOverviews(ctx, lookBackParams(25))

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got := atomic.LoadInt32(&fake.queryTracesCalls); got != 1 {
		t.Errorf("QueryTraces calls = %d, want 1", got)
	}
}
