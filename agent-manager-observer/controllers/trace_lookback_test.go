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
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"reflect"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/store/openchoreo"
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

// lookBackParams builds a 24h window filtered on conv-match.
func lookBackParams(limit int) TraceQueryParams {
	params := baseParams()
	params.StartTime = lookBackWindowEnd.Add(-24 * time.Hour)
	params.EndTime = lookBackWindowEnd
	params.Limit = limit
	params.Filters.ConversationID = "conv-match"
	return params
}

// assertNoDuplicates fails if a trace ID appears twice.
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

// traceIDs fetches one page and returns its IDs, lookedBackTo and truncated.
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

// No filter: one QueryTraces call for the requested limit plus one, which
// shows the page's last trace is settled.
func TestGetTraceOverviews_NoFilterQueriesOnce(t *testing.T) {
	fake := lookBackFake(200, 5)
	c := NewTracingController(openchoreo.New(fake))
	params := lookBackParams(20)
	params.Filters = TraceFilters{}

	ids, lookedBackTo, truncated := traceIDs(c, t, params)

	if got := atomic.LoadInt32(&fake.queryTracesCalls); got != 1 {
		t.Fatalf("QueryTraces calls = %d, want 1", got)
	}
	if got := *fake.tracesReqs[0].Limit; got != 21 {
		t.Errorf("QueryTraces limit = %d, want 21", got)
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
	c := NewTracingController(openchoreo.New(fake))

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
	// Each fetch asks for one more trace than it needs.
	wantLimits := []int{51, 101, 201}
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
	c := NewTracingController(openchoreo.New(fake))
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
			c := NewTracingController(openchoreo.New(fake))
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
	c := NewTracingController(openchoreo.New(fake))
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
	c := NewTracingController(openchoreo.New(fake))

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
	c := NewTracingController(openchoreo.New(fake))

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
			c := NewTracingController(openchoreo.New(fake))
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
	c := NewTracingController(openchoreo.New(fake))
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
	c := NewTracingController(openchoreo.New(fake))

	_, err := c.GetTraceOverviews(ctx, lookBackParams(25))

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if got := atomic.LoadInt32(&fake.queryTracesCalls); got != 1 {
		t.Errorf("QueryTraces calls = %d, want 1", got)
	}
}

// fakeClock is a controller clock that tests move by hand. Moving it past a
// look-back budget ends that budget's context.
type fakeClock struct {
	mu      sync.Mutex
	elapsed time.Duration
	budgets []fakeBudget
}

// fakeBudget is a look-back budget on a fakeClock.
type fakeBudget struct {
	end    time.Duration
	cancel context.CancelCauseFunc
}

// budget starts a budget of d.
func (c *fakeClock) budget(ctx context.Context, d time.Duration) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(ctx)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.budgets = append(c.budgets, fakeBudget{end: c.elapsed + d, cancel: cancel})
	return ctx, func() { cancel(context.Canceled) }
}

// advance moves the clock forward by d, ending the budgets it reaches.
func (c *fakeClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.elapsed += d
	for _, b := range c.budgets {
		if c.elapsed >= b.end {
			b.cancel(context.DeadlineExceeded)
		}
	}
}

// withClock gives c a fake clock that stands still until advanced.
func withClock(c *TracingController) *fakeClock {
	clock := &fakeClock{}
	c.budget = clock.budget
	return clock
}

// advanceOnRoot moves clock by d when rootID is fetched, during that trace's chunk.
func advanceOnRoot(fake *fakeObserverClient, clock *fakeClock, rootID string, d time.Duration) {
	var once sync.Once
	fake.onGetSpanDetails = func(_ context.Context, spanID string) {
		if spanID == rootID {
			once.Do(func() { clock.advance(d) })
		}
	}
}

// logContext returns a context whose logger writes JSON lines to the buffer.
func logContext() (context.Context, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return logger.WithLogger(context.Background(), slog.New(slog.NewJSONHandler(buf, nil))), buf
}

// logField returns field from the first log line with message msg.
func logField(t *testing.T, buf *bytes.Buffer, msg, field string) any {
	t.Helper()
	for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
		var rec map[string]any
		if json.Unmarshal(line, &rec) == nil && rec["msg"] == msg {
			return rec[field]
		}
	}
	t.Fatalf("no %q log line", msg)
	return nil
}

// warnings lists the messages of the WARN log lines.
func warnings(buf *bytes.Buffer) []string {
	var msgs []string
	for _, line := range bytes.Split(buf.Bytes(), []byte("\n")) {
		var rec map[string]any
		if json.Unmarshal(line, &rec) == nil && rec["level"] == "WARN" {
			msgs = append(msgs, fmt.Sprint(rec["msg"]))
		}
	}
	return msgs
}

// pageIDs lists the pages' trace IDs in order.
func pageIDs(pages []cursorPage) []string {
	var ids []string
	for _, p := range pages {
		for _, tr := range p.resp.Traces {
			ids = append(ids, tr.TraceID)
		}
	}
	return ids
}

// A sparse filter that runs out of time stops before the chunk it was
// reading, with a cursor, and paging on returns what a run with no budget returns.
func TestGetTraceOverviews_LookBackStopsAtTimeBudget(t *testing.T) {
	tests := []struct {
		name string
		// passOn is the root fetched when the clock passes the budget.
		passOn       string
		wantExamined int
		wantFetches  int32
	}{
		// The second fetch starts chunk 2, which the budget cuts.
		{name: "second chunk", passOn: "root-0050", wantExamined: 50, wantFetches: 2},
		// The third fetch starts chunk 3, which the budget cuts.
		{name: "third chunk", passOn: "root-0100", wantExamined: 100, wantFetches: 3},
	}
	for _, matchEvery := range []int{1000, 45} {
		for _, tt := range tests {
			t.Run(fmt.Sprintf("%s/1 in %d", tt.name, matchEvery), func(t *testing.T) {
				fake := lookBackFake(600, matchEvery)
				c := NewTracingController(openchoreo.New(fake))
				advanceOnRoot(fake, withClock(c), tt.passOn, listLookBackBudget)
				params := lookBackParams(10)
				ctx, logs := logContext()

				first, err := c.GetTraceOverviews(ctx, params)
				if err != nil {
					t.Fatalf("GetTraceOverviews returned error: %v", err)
				}

				if !first.Truncated || first.NextCursor == "" {
					t.Fatalf("truncated %v, nextCursor %q; want truncated with a cursor", first.Truncated, first.NextCursor)
				}
				if got := logField(t, logs, "Retrieved trace overviews", "examined"); got != float64(tt.wantExamined) {
					t.Errorf("examined logged as %v, want %d", got, tt.wantExamined)
				}
				if got := atomic.LoadInt32(&fake.queryTracesCalls); got != tt.wantFetches {
					t.Errorf("QueryTraces calls = %d, want %d", got, tt.wantFetches)
				}
				if want := formatCursor(fake.traces[tt.wantExamined-1].StartTime); first.LookedBackTo != want {
					t.Errorf("lookedBackTo = %s, want the last examined trace %s", first.LookedBackTo, want)
				}
				if got := logField(t, logs, "Retrieved trace overviews", "budgetExceeded"); got != true {
					t.Errorf("budgetExceeded logged as %v, want true", got)
				}

				if params.Cursor, err = DecodeTraceCursor(first.NextCursor); err != nil {
					t.Fatalf("nextCursor does not decode: %v", err)
				}
				got := pageIDs(append([]cursorPage{{resp: first}}, pageAll(t, c, fake, params)...))
				assertNoDuplicates(t, got)

				unbudgetedFake := lookBackFake(600, matchEvery)
				unbudgeted := NewTracingController(openchoreo.New(unbudgetedFake))
				withClock(unbudgeted)
				if want := pageIDs(pageAll(t, unbudgeted, unbudgetedFake, lookBackParams(10))); !reflect.DeepEqual(got, want) {
					t.Errorf("paged %v, want %v as with no budget", got, want)
				}
			})
		}
	}
}

// A budget already spent when the first fetch returns still examines one chunk.
func TestGetTraceOverviews_LookBackBudgetSpentOnFirstFetch(t *testing.T) {
	fake := lookBackFake(600, 1000)
	c := NewTracingController(openchoreo.New(fake))
	clock := withClock(c)
	fake.onQueryTraces = func() { clock.advance(listLookBackBudget) }

	resp, err := c.GetTraceOverviews(context.Background(), lookBackParams(10))
	if err != nil {
		t.Fatalf("GetTraceOverviews returned error: %v", err)
	}

	if len(resp.Traces) != 1 || resp.Traces[0].TraceID != "trace-0000" {
		t.Errorf("got %d traces, want trace-0000 alone", len(resp.Traces))
	}
	if !resp.Truncated || resp.NextCursor == "" {
		t.Fatalf("truncated %v, nextCursor %q; want truncated with a cursor", resp.Truncated, resp.NextCursor)
	}
	if got := atomic.LoadInt32(&fake.queryTracesCalls); got != 1 {
		t.Errorf("QueryTraces calls = %d, want 1", got)
	}
	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != lookBackBatchSize {
		t.Errorf("GetSpanDetails calls = %d, want %d (one chunk)", got, lookBackBatchSize)
	}
	if want := formatCursor(fake.traces[lookBackBatchSize-1].StartTime); resp.LookedBackTo != want {
		t.Errorf("lookedBackTo = %s, want %s", resp.LookedBackTo, want)
	}
}

// The budget cancels a fetch still in flight without a warning, and the walk
// stops before its chunk.
func TestGetTraceOverviews_LookBackBudgetCancelsInFlightFetch(t *testing.T) {
	fake := lookBackFake(600, 1000)
	fake.failOnDone = true
	c := NewTracingController(openchoreo.New(fake))
	clock := withClock(c)
	var cancelled atomic.Bool
	fake.onGetSpanDetails = func(ctx context.Context, spanID string) {
		if spanID != "root-0050" {
			return
		}
		clock.advance(listLookBackBudget)
		select {
		case <-ctx.Done():
			cancelled.Store(true)
		case <-time.After(5 * time.Second):
		}
	}
	ctx, logs := logContext()

	resp, err := c.GetTraceOverviews(ctx, lookBackParams(10))
	if err != nil {
		t.Fatalf("GetTraceOverviews returned error: %v", err)
	}

	if !cancelled.Load() {
		t.Fatal("the fetch in flight was not cancelled")
	}
	if !resp.Truncated || resp.NextCursor == "" {
		t.Fatalf("truncated %v, nextCursor %q; want truncated with a cursor", resp.Truncated, resp.NextCursor)
	}
	if want := formatCursor(fake.traces[49].StartTime); resp.LookedBackTo != want {
		t.Errorf("lookedBackTo = %s, want the last trace before the chunk %s", resp.LookedBackTo, want)
	}
	if got := warnings(logs); len(got) != 0 {
		t.Errorf("logged warnings %v, want none", got)
	}
	if got := logField(t, logs, "Retrieved trace overviews", "budgetExceeded"); got != true {
		t.Errorf("budgetExceeded logged as %v, want true", got)
	}
}

// A leaf fetch the budget cuts logs no warning; one that fails while the walk
// runs does.
func TestGetTraceOverviews_LookBackLeafFetchWarnings(t *testing.T) {
	const leafWarning = "aggregateFromLeafLLMSpans: GetSpanDetails failed for leaf"
	// No trace reaches minTokens, so every leaf is fetched and the walk reaches chunk 2.
	minTokens := int64(1000)
	params := lookBackParams(10)
	params.Filters = TraceFilters{MinTokens: &minTokens}

	t.Run("cut by budget", func(t *testing.T) {
		fake := langGraphFake(200, noRootAttrs)
		fake.failOnDone = true
		c := NewTracingController(openchoreo.New(fake))
		clock := withClock(c)
		var once sync.Once
		fake.onGetSpanDetails = func(_ context.Context, spanID string) {
			if spanID == "leaf-a-0050" {
				once.Do(func() { clock.advance(listLookBackBudget) })
			}
		}
		ctx, logs := logContext()

		resp, err := c.GetTraceOverviews(ctx, params)
		if err != nil {
			t.Fatalf("GetTraceOverviews returned error: %v", err)
		}

		if want := formatCursor(fake.traces[49].StartTime); !resp.Truncated || resp.LookedBackTo != want {
			t.Errorf("truncated %v, lookedBackTo %s; want true, %s", resp.Truncated, resp.LookedBackTo, want)
		}
		if got := warnings(logs); len(got) != 0 {
			t.Errorf("logged warnings %v, want none", got)
		}
		if got := logField(t, logs, "Retrieved trace overviews", "budgetExceeded"); got != true {
			t.Errorf("budgetExceeded logged as %v, want true", got)
		}
	})

	t.Run("fails live", func(t *testing.T) {
		fake := langGraphFake(200, noRootAttrs)
		fake.failCall = failTimes(map[string]int{"leaf-a-0050": -1})
		ctx, logs := logContext()

		if _, err := NewTracingController(openchoreo.New(fake)).GetTraceOverviews(ctx, params); err != nil {
			t.Fatalf("GetTraceOverviews returned error: %v", err)
		}

		if got := warnings(logs); !slices.Contains(got, leafWarning) {
			t.Errorf("logged warnings %v, want %q", got, leafWarning)
		}
	})
}

// A chunk's failed traces aren't retried once the budget is spent, and the
// walk stops before the chunk rather than listing them as failed.
func TestGetTraceOverviews_LookBackNoRetryPastBudget(t *testing.T) {
	fake := lookBackFake(600, 1000)
	c := NewTracingController(openchoreo.New(fake))
	clock := withClock(c)
	var calls atomic.Int32
	fake.onGetSpanDetails = func(_ context.Context, spanID string) {
		if spanID == "root-0050" && calls.Add(1) == 1 {
			clock.advance(listLookBackBudget)
		}
	}
	fake.failCall = failTimes(map[string]int{"root-0050": 1})
	ctx, logs := logContext()

	resp, err := c.GetTraceOverviews(ctx, lookBackParams(10))
	if err != nil {
		t.Fatalf("GetTraceOverviews returned error: %v", err)
	}

	if got := calls.Load(); got != 1 {
		t.Errorf("root-0050 fetched %d times, want 1", got)
	}
	if want := formatCursor(fake.traces[49].StartTime); !resp.Truncated || resp.LookedBackTo != want {
		t.Errorf("truncated %v, lookedBackTo %s; want true, %s", resp.Truncated, resp.LookedBackTo, want)
	}
	if got := logField(t, logs, "Retrieved trace overviews", "failed"); got != float64(0) {
		t.Errorf("failed logged as %v, want 0", got)
	}
}

// Every upstream call a trace list or export makes ends by requestTimeout,
// the first chunk's and the export's span fetches included.
func TestRequestTimeoutBoundsEveryCall(t *testing.T) {
	ctx := context.Background()
	tests := map[string]struct {
		fake *fakeObserverClient
		run  func(*TracingController) error
	}{
		"filtered list": {fake: lookBackFake(200, 5), run: func(c *TracingController) error {
			_, err := c.GetTraceOverviews(ctx, lookBackParams(10))
			return err
		}},
		"unfiltered list": {fake: lookBackFake(200, 5), run: func(c *TracingController) error {
			params := lookBackParams(10)
			params.Filters = TraceFilters{}
			_, err := c.GetTraceOverviews(ctx, params)
			return err
		}},
		"filtered export": {fake: langGraphFake(60, errorEvery(10)), run: func(c *TracingController) error {
			_, err := c.ExportTraces(ctx, exportParams(100, TraceFilters{Status: TraceStatusError}))
			return err
		}},
		"unfiltered export": {fake: langGraphFake(60, errorEvery(10)), run: func(c *TracingController) error {
			_, err := c.ExportTraces(ctx, exportParams(100, TraceFilters{}))
			return err
		}},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var calls, unbounded atomic.Int32
			tt.fake.onCall = func(ctx context.Context) {
				calls.Add(1)
				if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > requestTimeout {
					unbounded.Add(1)
				}
			}

			if err := tt.run(NewTracingController(openchoreo.New(tt.fake))); err != nil {
				t.Fatalf("returned error: %v", err)
			}

			if calls.Load() == 0 || unbounded.Load() != 0 {
				t.Errorf("%d of %d upstream calls without the request deadline", unbounded.Load(), calls.Load())
			}
		})
	}
}

// An unfiltered page whose ctx ends during enrichment fails instead of leaving traces out.
func TestGetTraceOverviews_PageFailsWhenCtxEndsDuringEnrichment(t *testing.T) {
	fake := lookBackFake(200, 1000)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake.onGetSpanDetails = func(_ context.Context, spanID string) {
		if spanID == "root-0005" {
			cancel()
		}
	}
	params := lookBackParams(20)
	params.Filters = TraceFilters{}

	resp, err := NewTracingController(openchoreo.New(fake)).GetTraceOverviews(ctx, params)

	if !errors.Is(err, context.Canceled) || resp != nil {
		t.Fatalf("err = %v, response %v; want context.Canceled and no response", err, resp != nil)
	}
}

// A clock short of the list's budget leaves the walk to the examine cap.
func TestGetTraceOverviews_LookBackWithinBudget(t *testing.T) {
	fake := lookBackFake(600, 1000)
	c := NewTracingController(openchoreo.New(fake))
	advanceOnRoot(fake, withClock(c), "root-0050", listLookBackBudget-time.Nanosecond)
	ctx, logs := logContext()

	resp, err := c.GetTraceOverviews(ctx, lookBackParams(10))
	if err != nil {
		t.Fatalf("GetTraceOverviews returned error: %v", err)
	}

	if got := atomic.LoadInt32(&fake.getSpanDetailsCalls); got != maxExaminedTraces {
		t.Errorf("GetSpanDetails calls = %d, want %d", got, maxExaminedTraces)
	}
	if !resp.Truncated || resp.NextCursor == "" {
		t.Errorf("truncated %v, nextCursor %q; want truncated with a cursor", resp.Truncated, resp.NextCursor)
	}
	if got := logField(t, logs, "Retrieved trace overviews", "budgetExceeded"); got != false {
		t.Errorf("budgetExceeded logged as %v, want false", got)
	}
}

// A filtered list retries a root that fails once, and leaves out one that
// keeps failing with no error and no new response field.
func TestGetTraceOverviews_LookBackRootFetchFails(t *testing.T) {
	// matchesSkipping is the first 10 matches of a 1-in-5 filter without skip.
	matchesSkipping := func(skip int) []string {
		var ids []string
		for i := 0; len(ids) < 10; i += 5 {
			if i != skip {
				ids = append(ids, fmt.Sprintf("trace-%04d", i))
			}
		}
		return ids
	}
	tests := []struct {
		name       string
		times      int
		want       []string
		wantFailed float64
	}{
		{name: "fails once", times: 1, want: matchesSkipping(-1)},
		{name: "keeps failing", times: -1, want: matchesSkipping(5), wantFailed: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := lookBackFake(200, 5)
			fake.failCall = failTimes(map[string]int{"root-0005": tt.times})
			ctx, logs := logContext()

			resp, err := NewTracingController(openchoreo.New(fake)).GetTraceOverviews(ctx, lookBackParams(10))
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}

			ids := make([]string, 0, len(resp.Traces))
			for _, tr := range resp.Traces {
				ids = append(ids, tr.TraceID)
			}
			if !reflect.DeepEqual(ids, tt.want) || resp.Truncated || resp.NextCursor == "" {
				t.Fatalf("got %v, truncated %v, cursor %q; want %v, false, a cursor", ids, resp.Truncated, resp.NextCursor, tt.want)
			}
			if got := logField(t, logs, "Retrieved trace overviews", "failed"); got != tt.wantFailed {
				t.Errorf("failed logged as %v, want %v", got, tt.wantFailed)
			}
			if got := warnings(logs); !slices.Contains(got, "failed to fetch root span details, skipping trace") {
				t.Errorf("logged warnings %v, want the root fetch's", got)
			}
			b, err := json.Marshal(resp)
			if err != nil {
				t.Fatalf("marshal: %v", err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(b, &fields); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			want := []string{"lookedBackTo", "nextCursor", "totalCount", "traces", "truncated"}
			if got := slices.Sorted(maps.Keys(fields)); !reflect.DeepEqual(got, want) {
				t.Errorf("response fields = %v, want %v", got, want)
			}
		})
	}
}

// failEvery fails every call.
func failEvery(id string) error { return fmt.Errorf("fake: %s unavailable", id) }

// A filtered list that can read none of the traces it examines fails, after
// fetching each root twice.
func TestGetTraceOverviews_LookBackEveryRootFetchFails(t *testing.T) {
	fake := lookBackFake(200, 5)
	fake.failCall = failEvery

	resp, err := NewTracingController(openchoreo.New(fake)).GetTraceOverviews(context.Background(), lookBackParams(10))

	if err == nil || resp != nil {
		t.Fatalf("err = %v, response %v; want an error and no response", err, resp != nil)
	}
	want := make(map[string]int, len(fake.traces))
	for _, tr := range fake.traces {
		want[tr.RootSpanID] = 2
	}
	got := make(map[string]int)
	for _, id := range fake.detailSpanIDs {
		got[id]++
	}
	if !maps.Equal(got, want) {
		t.Errorf("root fetches = %v, want each of the %d roots twice", got, len(want))
	}
}

// An unfiltered page fails when it can read none of its traces, and
// otherwise leaves out the ones it can't read.
func TestGetTraceOverviews_PageRootFetchFails(t *testing.T) {
	tests := []struct {
		name    string
		fail    func(string) error
		wantErr bool
		want    int
	}{
		{name: "every root", fail: failEvery, wantErr: true},
		{name: "one root", fail: failTimes(map[string]int{"root-0003": -1}), want: 19},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := lookBackFake(200, 5)
			fake.failCall = tt.fail
			params := lookBackParams(20)
			params.Filters = TraceFilters{}

			resp, err := NewTracingController(openchoreo.New(fake)).GetTraceOverviews(context.Background(), params)

			if tt.wantErr {
				if err == nil || resp != nil {
					t.Fatalf("err = %v, response %v; want an error and no response", err, resp != nil)
				}
				return
			}
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}
			if len(resp.Traces) != tt.want {
				t.Errorf("got %d traces, want %d", len(resp.Traces), tt.want)
			}
		})
	}
}

// A filtered list that reads one trace that doesn't match, and none of the
// rest, returns an empty page and logs the rest as failed.
func TestGetTraceOverviews_LookBackOneTraceReads(t *testing.T) {
	tests := []struct {
		name    string
		filters TraceFilters
	}{
		{name: "ruled out by its root", filters: TraceFilters{ConversationID: "conv-match"}},
		{name: "kept, short of minTokens", filters: TraceFilters{MinTokens: ptr(100)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := lookBackFake(200, 5)
			// Only trace-0001 reads, and it matches neither filter.
			fake.failCall = func(id string) error {
				if id == "root-0001" || id == "trace-0001" {
					return nil
				}
				return failEvery(id)
			}
			params := lookBackParams(10)
			params.Filters = tt.filters
			ctx, logs := logContext()

			resp, err := NewTracingController(openchoreo.New(fake)).GetTraceOverviews(ctx, params)
			if err != nil {
				t.Fatalf("GetTraceOverviews returned error: %v", err)
			}

			if len(resp.Traces) != 0 {
				t.Errorf("got %d traces, want none", len(resp.Traces))
			}
			if got := logField(t, logs, "Retrieved trace overviews", "failed"); got != float64(199) {
				t.Errorf("failed logged as %v, want 199", got)
			}
		})
	}
}
