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
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/opensearch"
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

func cursorParams(limit int, sortOrder string, filtered bool) TraceQueryParams {
	params := lookBackParams(limit)
	params.SortOrder = sortOrder
	if !filtered {
		params.Filters = TraceFilters{}
	}
	return params
}

// Paging by cursor returns exactly the traces rooted in the window, with
// overlapping traces, in both sort orders, filtered and not.
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
				c := NewTracingController(fake)
				params := cursorParams(20, sortOrder, tt.filtered)
				if tt.summary {
					longEvery(fake, func(i int) bool { return i%tt.matchEvery == 0 })
					params.Filters = TraceFilters{MinDurationMs: ptr(1000)}
				}

				pages := pageAll(t, c, fake, params)

				assertIDs(t, unionIDs(pages), wantIDs(4, 300, tt.matchEvery))
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
				c := NewTracingController(fake)
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

// More traces at one timestamp than fit on a page still make progress.
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
			c := NewTracingController(fake)
			params := cursorParams(10, "desc", true)
			if tt.filters != nil {
				params.Filters = *tt.filters
			}

			pages := pageAll(t, c, fake, params)

			assertIDs(t, unionIDs(pages), wantIDs(0, 60, 1))
		})
	}
}

// A filtered page stopped by the examine cap carries a cursor, and the next
// page picks up where it stopped.
func TestGetTraceOverviews_CursorContinuesPastExamineCap(t *testing.T) {
	fake := lookBackFake(1000, 200)
	c := NewTracingController(fake)

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
			c := NewTracingController(fake)
			params := cursorParams(20, "desc", filtered)
			params.Cursor = &TraceCursor{Rank: maxCursorDepth - 10, Time: fake.traces[maxCursorDepth-11].StartTime}

			resp, err := c.GetTraceOverviews(context.Background(), params)
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}

			if !resp.Truncated || resp.NextCursor != "" {
				t.Errorf("truncated %v, nextCursor %q; want true and none", resp.Truncated, resp.NextCursor)
			}
			// The trace at the cursor time plus the 10 the depth still reaches.
			if len(resp.Traces) != 11 {
				t.Errorf("got %d traces, want 11", len(resp.Traces))
			}
			for i, req := range fake.tracesReqs {
				if *req.Limit > maxCursorDepth {
					t.Errorf("fetch %d limit = %d, want at most %d", i, *req.Limit, maxCursorDepth)
				}
			}
		})
	}
}

func TestTraceCursor_RoundTrip(t *testing.T) {
	want := TraceCursor{Rank: 42, Time: time.Date(2026, 9, 1, 11, 59, 1, 123456789, time.UTC)}

	got, err := DecodeTraceCursor(want.Encode())

	if err != nil {
		t.Fatalf("DecodeTraceCursor returned error: %v", err)
	}
	if got.Rank != want.Rank || !got.Time.Equal(want.Time) {
		t.Errorf("got %+v, want %+v", *got, want)
	}
}

func TestDecodeTraceCursor_Rejects(t *testing.T) {
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	tests := map[string]string{
		"not base64":    "not base64!",
		"not JSON":      enc("nope"),
		"negative rank": enc(`{"r":-1,"t":"2026-09-01T00:00:00Z"}`),
		"missing time":  enc(`{"r":1}`),
		"bad time":      enc(`{"r":1,"t":"yesterday"}`),
	}
	for name, raw := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := DecodeTraceCursor(raw); err == nil {
				t.Errorf("DecodeTraceCursor(%q) succeeded, want an error", raw)
			}
		})
	}
}
