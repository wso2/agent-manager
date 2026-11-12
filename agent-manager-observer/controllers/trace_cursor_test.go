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
	"encoding/base64"
	"fmt"
	"math"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
	"github.com/wso2/agent-manager/agent-manager-observer/store/openchoreo"
)

// overlappingFake is lookBackFake with 5 s traces. Traces 0-3 end after the
// window, so their roots are outside it.
func overlappingFake(n, matchEvery int) *fakeObserverClient {
	fake := lookBackFake(n, matchEvery)
	for i := range fake.traces {
		fake.traces[i].EndTime = fake.traces[i].StartTime.Add(5 * time.Second)
	}
	return fake
}

type cursorPage struct {
	resp  *opensearch.TraceOverviewResponse
	calls int32
}

// pageAll follows nextCursor from params until it is absent.
func pageAll(t *testing.T, c *TracingController, fake *fakeObserverClient, params TraceQueryParams) []cursorPage {
	t.Helper()
	var pages []cursorPage
	for len(pages) < 200 {
		before := atomic.LoadInt32(&fake.queryTracesCalls)
		resp, err := c.GetTraceOverviews(context.Background(), params)
		if err != nil {
			t.Fatalf("page %d: GetTraceOverviews returned error: %v", len(pages), err)
		}
		pages = append(pages, cursorPage{resp: resp, calls: atomic.LoadInt32(&fake.queryTracesCalls) - before})
		if resp.NextCursor == "" {
			return pages
		}
		if params.Cursor, err = DecodeTraceCursor(resp.NextCursor); err != nil {
			t.Fatalf("page %d: nextCursor does not decode: %v", len(pages)-1, err)
		}
	}
	t.Fatalf("still paging after %d pages", len(pages))
	return nil
}

// unionIDs de-duplicates the pages' traces by ID, as a client does.
func unionIDs(pages []cursorPage) map[string]bool {
	ids := map[string]bool{}
	for _, p := range pages {
		for _, tr := range p.resp.Traces {
			ids[tr.TraceID] = true
		}
	}
	return ids
}

// wantIDs is every matchEvery-th trace ID in [lo, hi).
func wantIDs(lo, hi, matchEvery int) map[string]bool {
	ids := map[string]bool{}
	for i := lo; i < hi; i++ {
		if i%matchEvery == 0 {
			ids[fmt.Sprintf("trace-%04d", i)] = true
		}
	}
	return ids
}

// assertIDs reports missing and extra trace IDs.
func assertIDs(t *testing.T, got, want map[string]bool) {
	t.Helper()
	var missing, extra []string
	for id := range want {
		if !got[id] {
			missing = append(missing, id)
		}
	}
	for id := range got {
		if !want[id] {
			extra = append(extra, id)
		}
	}
	if len(missing) > 0 || len(extra) > 0 {
		slices.Sort(missing)
		slices.Sort(extra)
		t.Errorf("got %d traces, want %d; missing %v, extra %v", len(got), len(want), missing, extra)
	}
}

// cursorParams builds look-back params with the given sort order, filtered or not.
func cursorParams(limit int, sortOrder string, filtered bool) TraceQueryParams {
	params := lookBackParams(limit)
	params.SortOrder = sortOrder
	if !filtered {
		params.Filters = TraceFilters{}
	}
	return params
}

// Paging by cursor returns exactly the traces rooted in the window, with
// overlapping traces, in both sort orders, filtered and not. Pages don't overlap.
func TestGetTraceOverviews_CursorPagesWholeWindow(t *testing.T) {
	tests := []struct {
		name       string
		filtered   bool
		summary    bool
		matchEvery int
	}{
		{name: "unfiltered", matchEvery: 1},
		{name: "filter matches all", filtered: true, matchEvery: 1},
		{name: "filter matches 1 in 3", filtered: true, matchEvery: 3},
		{name: "summary filter matches 1 in 3", filtered: true, summary: true, matchEvery: 3},
	}
	for _, sortOrder := range []string{"desc", "asc"} {
		for _, tt := range tests {
			t.Run(sortOrder+"/"+tt.name, func(t *testing.T) {
				fake := overlappingFake(300, tt.matchEvery)
				c := NewTracingController(openchoreo.New(fake))
				params := cursorParams(20, sortOrder, tt.filtered)
				if tt.summary {
					longEvery(fake, func(i int) bool { return i%tt.matchEvery == 0 })
					params.Filters = TraceFilters{MinDurationMs: ptr(1000)}
				}

				pages := pageAll(t, c, fake, params)

				assertIDs(t, unionIDs(pages), wantIDs(4, 300, tt.matchEvery))
				assertNoDuplicates(t, pageIDs(pages))
				for i, p := range pages {
					if p.resp.Truncated {
						t.Errorf("page %d truncated", i)
					}
					// Unfiltered pages, first and cursor alike, make one list call.
					if !tt.filtered && p.calls != 1 {
						t.Errorf("page %d made %d QueryTraces calls, want 1", i, p.calls)
					}
				}
				for i, req := range fake.tracesReqs {
					if !req.StartTime.Equal(params.StartTime) || !req.EndTime.Equal(params.EndTime) {
						t.Fatalf("fetch %d window = %s..%s, want the request window", i, req.StartTime, req.EndTime)
					}
				}
			})
		}
	}
}

// Traces added or removed before the cursor between pages cause no loss,
// and an added one is not returned.
func TestGetTraceOverviews_CursorChangeBeforeCursor(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*fakeObserverClient)
	}{
		{name: "trace added", mutate: func(fake *fakeObserverClient) {
			late := baseTraceInfo(2)
			late.TraceID, late.RootSpanID = "trace-late", "root-late"
			late.StartTime = fake.traces[10].StartTime.Add(500 * time.Millisecond)
			late.EndTime = late.StartTime.Add(5 * time.Second)
			fake.traces = append(fake.traces, late)
			attrs := completeRootAttrs()
			attrs["gen_ai.conversation.id"] = "conv-match"
			fake.spanDetails[late.RootSpanID] = &observer.SpanDetailsResponse{
				SpanID: late.RootSpanID, SpanName: "invoke_agent LangGraph", Attributes: attrs,
			}
		}},
		{name: "trace removed", mutate: func(fake *fakeObserverClient) {
			fake.traces = slices.Delete(fake.traces, 10, 11)
		}},
	}
	for _, filtered := range []bool{false, true} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%s/filtered=%t", tt.name, filtered), func(t *testing.T) {
				fake := overlappingFake(300, 1)
				c := NewTracingController(openchoreo.New(fake))
				params := cursorParams(20, "desc", filtered)

				first, err := c.GetTraceOverviews(context.Background(), params)
				if err != nil || first.NextCursor == "" {
					t.Fatalf("first page: err %v, nextCursor %q; want a cursor", err, first.NextCursor)
				}
				tt.mutate(fake)
				if params.Cursor, err = DecodeTraceCursor(first.NextCursor); err != nil {
					t.Fatalf("nextCursor does not decode: %v", err)
				}
				rest := pageAll(t, c, fake, params)

				if unionIDs(rest)["trace-late"] {
					t.Error("trace added before the cursor was returned")
				}
				assertIDs(t, unionIDs(append([]cursorPage{{resp: first}}, rest...)), wantIDs(4, 300, 1))
			})
		}
	}
}

// More traces at one timestamp than fit on a page still make progress, pages
// don't overlap, and each page holds limit traces until the last.
func TestGetTraceOverviews_CursorTiesMakeProgress(t *testing.T) {
	tests := []struct {
		name    string
		filters *TraceFilters
	}{
		{name: "unfiltered", filters: &TraceFilters{}},
		{name: "filtered"},
		{name: "summary filter", filters: &TraceFilters{MinSpanCount: ptr(2)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := lookBackFake(60, 1)
			for i := 10; i < 40; i++ {
				fake.traces[i].StartTime = fake.traces[10].StartTime
				fake.traces[i].EndTime = fake.traces[10].StartTime
			}
			c := NewTracingController(openchoreo.New(fake))
			params := cursorParams(10, "desc", true)
			if tt.filters != nil {
				params.Filters = *tt.filters
			}

			pages := pageAll(t, c, fake, params)

			assertIDs(t, unionIDs(pages), wantIDs(0, 60, 1))
			assertNoDuplicates(t, pageIDs(pages))
			for i, p := range pages[:len(pages)-1] {
				if len(p.resp.Traces) != params.Limit {
					t.Errorf("page %d holds %d traces, want %d", i, len(p.resp.Traces), params.Limit)
				}
			}
		})
	}
}

// A cursor inside a tie group bigger than the page returns and enriches only
// the page, not every trace at the cursor time.
func TestGetTraceOverviews_CursorInsideLargeTieGroup(t *testing.T) {
	// Traces 10-899 share one start time, and the deepest fetch sees past them.
	fake := lookBackFake(maxCursorDepth-50, 1)
	for i := 10; i < maxCursorDepth-100; i++ {
		fake.traces[i].StartTime = fake.traces[10].StartTime
		fake.traces[i].EndTime = fake.traces[10].StartTime
	}
	c := NewTracingController(openchoreo.New(fake))
	params := cursorParams(20, "desc", false)

	var ids []string
	for page := 0; page < 5; page++ {
		before := rootFetches(fake)
		resp, err := c.GetTraceOverviews(context.Background(), params)
		if err != nil {
			t.Fatalf("page %d: GetTraceOverviews returned error: %v", page, err)
		}
		if got := rootFetches(fake) - before; got > params.Limit {
			t.Errorf("page %d made %d root GetSpanDetails calls, want at most %d", page, got, params.Limit)
		}
		if len(resp.Traces) != params.Limit || resp.NextCursor == "" {
			t.Fatalf("page %d: %d traces, nextCursor %q; want %d and a cursor", page, len(resp.Traces), resp.NextCursor, params.Limit)
		}
		for _, tr := range resp.Traces {
			ids = append(ids, tr.TraceID)
		}
		if params.Cursor, err = DecodeTraceCursor(resp.NextCursor); err != nil {
			t.Fatalf("page %d: nextCursor does not decode: %v", page, err)
		}
	}

	// Ties are in trace ID order, so five pages are trace-0000 to trace-0099.
	want := make([]string, 0, 100)
	for i := 0; i < 100; i++ {
		want = append(want, fmt.Sprintf("trace-%04d", i))
	}
	if !slices.Equal(ids, want) {
		t.Errorf("paged %v, want %v", ids, want)
	}
}

// A filter that matches none of a tie group bigger than the examine cap
// pages through the group to the match after it, and every page moves on.
func TestGetTraceOverviews_CursorPastNonMatchingTies(t *testing.T) {
	for _, sortOrder := range []string{"desc", "asc"} {
		t.Run(sortOrder, func(t *testing.T) {
			// Traces 0-689 share one start time and match nothing; trace-0690 matches.
			fake := lookBackFake(700, 690)
			fake.spanDetails["root-0000"].Attributes["gen_ai.conversation.id"] = "conv-other"
			for i := 0; i < 690; i++ {
				fake.traces[i].StartTime = fake.traces[0].StartTime
				fake.traces[i].EndTime = fake.traces[0].StartTime
			}
			c := NewTracingController(openchoreo.New(fake))

			pages := pageAll(t, c, fake, cursorParams(20, sortOrder, true))

			assertIDs(t, unionIDs(pages), map[string]bool{"trace-0690": true})
			cursors := map[string]bool{}
			for i, p := range pages {
				if cursors[p.resp.NextCursor] {
					t.Fatalf("page %d repeats an earlier nextCursor", i)
				}
				cursors[p.resp.NextCursor] = true
			}
		})
	}
}

// Ties that come back in a different order on every fetch, cut anywhere by
// the fetch limit, page to exactly the window with no repeats.
func TestGetTraceOverviews_CursorTiesInAnyOrder(t *testing.T) {
	tests := []struct {
		name       string
		filtered   bool
		matchEvery int
	}{
		{name: "unfiltered", matchEvery: 1},
		{name: "filtered", filtered: true, matchEvery: 3},
	}
	for _, sortOrder := range []string{"desc", "asc"} {
		for _, tt := range tests {
			// Groups of 7 straddle pages; groups of 61 are bigger than the first fetch.
			for _, group := range []int{7, 61} {
				t.Run(fmt.Sprintf("%s/%s/groups of %d", sortOrder, tt.name, group), func(t *testing.T) {
					fake := lookBackFake(300, tt.matchEvery)
					fake.shuffleTies = true
					for i := range fake.traces {
						fake.traces[i].StartTime = fake.traces[i-i%group].StartTime
						fake.traces[i].EndTime = fake.traces[i].StartTime
					}
					c := NewTracingController(openchoreo.New(fake))

					pages := pageAll(t, c, fake, cursorParams(10, sortOrder, tt.filtered))

					assertNoDuplicates(t, pageIDs(pages))
					assertIDs(t, unionIDs(pages), wantIDs(0, 300, tt.matchEvery))
				})
			}
		}
	}
}

// A tie group bigger than the deepest fetch stops truncated with no cursor.
func TestGetTraceOverviews_TieGroupPastDepthCap(t *testing.T) {
	for _, filtered := range []bool{false, true} {
		t.Run(fmt.Sprintf("filtered=%t", filtered), func(t *testing.T) {
			fake := lookBackFake(maxCursorDepth+100, 1)
			tie := fake.traces[0].StartTime
			for i := range fake.traces {
				fake.traces[i].StartTime = tie
				fake.traces[i].EndTime = tie
			}
			c := NewTracingController(openchoreo.New(fake))

			resp, err := c.GetTraceOverviews(context.Background(), cursorParams(20, "desc", filtered))
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}

			if !resp.Truncated || resp.NextCursor != "" {
				t.Errorf("truncated %v, nextCursor %q; want true and none", resp.Truncated, resp.NextCursor)
			}
			// No fetch sees past the group, so no trace is settled. Unfiltered,
			// no cursor follows, so the page still fills; the filtered walk
			// holds the unsettled group back.
			want := 20
			if filtered {
				want = 0
			}
			if len(resp.Traces) != want {
				t.Errorf("got %d traces, want %d", len(resp.Traces), want)
			}
			var ids []string
			for _, tr := range resp.Traces {
				ids = append(ids, tr.TraceID)
				if tr.StartTime != tie.Format(time.RFC3339Nano) {
					t.Errorf("%s startTime = %q, want the tie time", tr.TraceID, tr.StartTime)
				}
			}
			if !slices.IsSorted(ids) {
				t.Errorf("page %v not in trace ID order", ids)
			}
			for i, req := range fake.tracesReqs {
				if *req.Limit > maxCursorDepth {
					t.Errorf("fetch %d limit = %d, want at most %d", i, *req.Limit, maxCursorDepth)
				}
			}
		})
	}
}

// A cursor without a trace ID still decodes and pages. It keeps every trace
// at its time, so the trace it was issued at comes back.
func TestGetTraceOverviews_CursorWithoutID(t *testing.T) {
	for _, filtered := range []bool{false, true} {
		t.Run(fmt.Sprintf("filtered=%t", filtered), func(t *testing.T) {
			fake := lookBackFake(60, 1)
			c := NewTracingController(openchoreo.New(fake))
			params := cursorParams(10, "desc", filtered)
			raw := fmt.Sprintf(`{"r":10,"t":%q}`, fake.traces[9].StartTime.Format(time.RFC3339Nano))
			cur, err := DecodeTraceCursor(base64.RawURLEncoding.EncodeToString([]byte(raw)))
			if err != nil {
				t.Fatalf("DecodeTraceCursor returned error: %v", err)
			}
			if cur.ID != "" {
				t.Fatalf("cursor ID = %q, want empty", cur.ID)
			}
			params.Cursor = cur

			pages := pageAll(t, c, fake, params)

			assertNoDuplicates(t, pageIDs(pages))
			assertIDs(t, unionIDs(pages), wantIDs(9, 60, 1))
		})
	}
}

// A filtered page stopped by the examine cap carries a cursor, and the next
// page picks up where it stopped.
func TestGetTraceOverviews_CursorContinuesPastExamineCap(t *testing.T) {
	fake := lookBackFake(1000, 200)
	c := NewTracingController(openchoreo.New(fake))

	pages := pageAll(t, c, fake, lookBackParams(10))

	assertIDs(t, unionIDs(pages), wantIDs(0, 1000, 200))
	if !pages[0].resp.Truncated || len(pages) < 2 {
		t.Errorf("first page truncated %v over %d pages; want truncated with a later page", pages[0].resp.Truncated, len(pages))
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got > int32(len(pages)*maxExaminedTraces) {
		t.Errorf("GetSpanDetails calls = %d, want at most %d per page", got, maxExaminedTraces)
	}
}

// A cursor too deep to fetch past returns what it found, truncated, with no cursor.
func TestGetTraceOverviews_CursorDepthCap(t *testing.T) {
	for _, filtered := range []bool{false, true} {
		t.Run(fmt.Sprintf("filtered=%t", filtered), func(t *testing.T) {
			fake := lookBackFake(maxCursorDepth+1000, 1)
			c := NewTracingController(openchoreo.New(fake))
			params := cursorParams(20, "desc", filtered)
			at := fake.traces[maxCursorDepth-11]
			params.Cursor = &TraceCursor{Rank: maxCursorDepth - 10, Time: at.StartTime, ID: at.TraceID}

			resp, err := c.GetTraceOverviews(context.Background(), params)
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}

			if !resp.Truncated || resp.NextCursor != "" {
				t.Errorf("truncated %v, nextCursor %q; want true and none", resp.Truncated, resp.NextCursor)
			}
			// The depth still reaches 10 traces. The last may have ties past
			// the fetch: unfiltered, no cursor follows, so it is kept; the
			// filtered walk leaves it out.
			want := 10
			if filtered {
				want = 9
			}
			if len(resp.Traces) != want {
				t.Errorf("got %d traces, want %d", len(resp.Traces), want)
			}
			for i, req := range fake.tracesReqs {
				if *req.Limit > maxCursorDepth {
					t.Errorf("fetch %d limit = %d, want at most %d", i, *req.Limit, maxCursorDepth)
				}
			}
		})
	}
}

// Unfiltered paging over unique start times reaches the depth cap: the first
// maxCursorDepth traces in page order, at any limit, ending truncated with no cursor.
func TestGetTraceOverviews_UnfilteredPagesToDepthCap(t *testing.T) {
	for _, sortOrder := range []string{"desc", "asc"} {
		for _, limit := range []int{10, 500, 999, 1000} {
			t.Run(fmt.Sprintf("%s/limit=%d", sortOrder, limit), func(t *testing.T) {
				fake := lookBackFake(1500, 1)
				c := NewTracingController(openchoreo.New(fake))

				pages := pageAll(t, c, fake, cursorParams(limit, sortOrder, false))

				want := make([]string, 0, maxCursorDepth)
				for i := 0; i < maxCursorDepth; i++ {
					n := i
					if sortOrder == "asc" {
						n = len(fake.traces) - 1 - i
					}
					want = append(want, fmt.Sprintf("trace-%04d", n))
				}
				got := pageIDs(pages)
				assertNoDuplicates(t, got)
				if !slices.Equal(got, want) {
					t.Errorf("paged %d traces, want the first %d in page order", len(got), len(want))
				}
				if last := pages[len(pages)-1].resp; !last.Truncated || last.NextCursor != "" {
					t.Errorf("last page truncated %v, nextCursor %q; want true and none", last.Truncated, last.NextCursor)
				}
				if limit == maxCursorDepth && (len(pages) != 1 || pages[0].calls != 1) {
					t.Errorf("%d pages, first made %d QueryTraces calls; want 1 page from 1 call", len(pages), pages[0].calls)
				}
			})
		}
	}
}

// Traces that land between a filtered walk's fetches can carry its rank past
// the depth cap. The issued cursor is clamped there, so it still decodes.
func TestGetTraceOverviews_CursorRankClampedAtDepthCap(t *testing.T) {
	// The cursor rank is 500, so the walk fetches 551 traces and then 1000.
	at := maxCursorDepth - maxExaminedTraces - 1
	// Only trace-0000 matches, and it is before the cursor.
	fake := lookBackFake(maxCursorDepth+1000, maxCursorDepth+1000)
	cursorTime := fake.traces[at].StartTime
	add := func(id string, start time.Time) {
		info := baseTraceInfo(2)
		info.TraceID, info.RootSpanID = id, "root-"+id
		info.StartTime, info.EndTime = start, start
		fake.traces = append(fake.traces, info)
		fake.spanDetails[info.RootSpanID] = &observer.SpanDetailsResponse{
			SpanID: info.RootSpanID, SpanName: "invoke_agent LangGraph", Attributes: completeRootAttrs(),
		}
	}
	calls := 0
	fake.onQueryTraces = func() {
		if calls++; calls != 2 {
			return
		}
		// 50 traces land before the cursor and 500 just past it, which push
		// the traces the first fetch examined out of the second.
		for i := 1; i <= 50; i++ {
			add(fmt.Sprintf("late-before-%03d", i), cursorTime.Add(time.Duration(i)*time.Millisecond))
		}
		for i := 1; i <= 500; i++ {
			add(fmt.Sprintf("late-after-%03d", i), cursorTime.Add(-time.Duration(i)*time.Millisecond))
		}
	}
	c := NewTracingController(openchoreo.New(fake))
	params := lookBackParams(10)
	params.Cursor = &TraceCursor{Rank: at + 1, Time: cursorTime}

	resp, err := c.GetTraceOverviews(context.Background(), params)
	if err != nil {
		t.Fatalf("GetTraceOverviews returned error: %v", err)
	}

	if resp.NextCursor == "" {
		t.Fatal("no nextCursor, want one")
	}
	next, err := DecodeTraceCursor(resp.NextCursor)
	if err != nil {
		t.Fatalf("nextCursor does not decode: %v", err)
	}
	if next.Rank != maxCursorDepth {
		t.Errorf("nextCursor rank = %d, want %d", next.Rank, maxCursorDepth)
	}
}

// A cursor decodes to the rank, time and trace ID it was encoded with.
func TestTraceCursor_RoundTrip(t *testing.T) {
	want := TraceCursor{Rank: 42, Time: time.Date(2026, 9, 1, 11, 59, 1, 123456789, time.UTC), ID: "trace-0042"}

	got, err := DecodeTraceCursor(want.Encode())

	if err != nil {
		t.Fatalf("DecodeTraceCursor returned error: %v", err)
	}
	if got.Rank != want.Rank || !got.Time.Equal(want.Time) || got.ID != want.ID {
		t.Errorf("got %+v, want %+v", *got, want)
	}
}

// A rank equal to the depth cap is accepted.
func TestDecodeTraceCursor_AcceptsDepthCap(t *testing.T) {
	want := TraceCursor{Rank: maxCursorDepth, Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}

	got, err := DecodeTraceCursor(want.Encode())

	if err != nil {
		t.Fatalf("DecodeTraceCursor returned error: %v", err)
	}
	if got.Rank != maxCursorDepth {
		t.Errorf("rank = %d, want %d", got.Rank, maxCursorDepth)
	}
}

// Malformed or out-of-range cursors are rejected.
func TestDecodeTraceCursor_Rejects(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	tests := map[string]string{
		"not base64":          "not base64!",
		"not JSON":            enc("nope"),
		"negative rank":       enc(`{"r":-1,"t":"2026-09-01T00:00:00Z"}`),
		"rank past depth cap": enc(fmt.Sprintf(`{"r":%d,"t":"2026-09-01T00:00:00Z"}`, maxCursorDepth+1)),
		"rank near max int":   enc(fmt.Sprintf(`{"r":%d,"t":"2026-09-01T00:00:00Z"}`, math.MaxInt)),
		"missing time":        enc(`{"r":1}`),
		"bad time":            enc(`{"r":1,"t":"yesterday"}`),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeTraceCursor(raw); err == nil {
				t.Errorf("DecodeTraceCursor(%q) succeeded, want an error", raw)
			}
		})
	}
}
