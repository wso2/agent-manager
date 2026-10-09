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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/agentmanager"
	"github.com/wso2/agent-manager/agent-manager-observer/middleware"
)

func f64(v float64) *float64 { return &v }

// scoreOf is a fixture trace's score, by its index. With no evaluator: 0.4 for
// every 20th trace, no scores for every 7th, only skipped scores (nil) for
// every 11th, and 0.9 for the rest. Helpfulness scores every 3rd trace 0.2
// and the rest 0.8; other evaluators score nothing.
func scoreOf(id, evaluator string) (score *float64, ok bool) {
	i, err := strconv.Atoi(strings.TrimPrefix(id, "trace-"))
	switch {
	case err != nil:
		return nil, false
	case evaluator == "Helpfulness" && i%3 == 0:
		return f64(0.2), true
	case evaluator == "Helpfulness":
		return f64(0.8), true
	case evaluator != "":
		return nil, false
	case i%20 == 0:
		return f64(0.4), true
	case i%7 == 0:
		return nil, false
	case i%11 == 0:
		return nil, true
	}
	return f64(0.9), true
}

// scorePasses reports whether trace i's fixture score passes f's score filter.
func scorePasses(i int, f TraceFilters) bool {
	score, _ := scoreOf(fmt.Sprintf("trace-%04d", i), f.Evaluator)
	return matchesScore(score, f)
}

// scoreLookup is one TraceScores call.
type scoreLookup struct {
	org, project, agent string
	start, end          time.Time
	ids                 []string
	evaluator           string
}

// fakeScoreClient answers from scoreOf and records each lookup.
type fakeScoreClient struct {
	// fail, when set, fails lookup n (from 1) with its error.
	fail func(n int) error
	// onCall runs with lookup n's ctx before it is answered.
	onCall func(ctx context.Context, n int)

	mu      sync.Mutex
	lookups []scoreLookup
}

// TraceScores records the lookup and returns the fixture scores of ids.
func (f *fakeScoreClient) TraceScores(ctx context.Context, org, project, agent string, start, end time.Time, ids []string, evaluator string) (map[string]*float64, error) {
	f.mu.Lock()
	f.lookups = append(f.lookups, scoreLookup{org: org, project: project, agent: agent, start: start, end: end, ids: slices.Clone(ids), evaluator: evaluator})
	n := len(f.lookups)
	f.mu.Unlock()
	if f.onCall != nil {
		f.onCall(ctx, n)
	}
	if f.fail != nil {
		if err := f.fail(n); err != nil {
			return nil, err
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	scores := make(map[string]*float64, len(ids))
	for _, id := range ids {
		if score, ok := scoreOf(id, evaluator); ok {
			scores[id] = score
		}
	}
	return scores, nil
}

// counts is the lookups made and the trace IDs they sent.
func (f *fakeScoreClient) counts() scoreCounts {
	f.mu.Lock()
	defer f.mu.Unlock()
	c := scoreCounts{Calls: len(f.lookups)}
	for _, l := range f.lookups {
		c.IDs += len(l.ids)
	}
	return c
}

// sentIDs lists every trace ID sent, in order.
func (f *fakeScoreClient) sentIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var ids []string
	for _, l := range f.lookups {
		ids = append(ids, l.ids...)
	}
	return ids
}

// withScores gives c a fakeScoreClient.
func withScores(c *TracingController) (*TracingController, *fakeScoreClient) {
	scores := &fakeScoreClient{}
	return c.WithScoreClient(scores), scores
}

// traceRange is trace-lo up to trace-hi, every step-th, as IDs.
func traceRange(lo, hi, step int) []string {
	var ids []string
	for i := lo; i < hi; i += step {
		ids = append(ids, fmt.Sprintf("trace-%04d", i))
	}
	return ids
}

// A trace matches when its score is within both bounds, inclusive. A trace with
// no score never matches a score filter.
func TestMatchesScore(t *testing.T) {
	tests := []struct {
		name  string
		f     TraceFilters
		score *float64
		want  bool
	}{
		{name: "no filter, no score", want: true},
		{name: "no filter, a score", score: f64(0.1), want: true},
		{name: "below maxScore", f: TraceFilters{MaxScore: f64(0.5)}, score: f64(0.4), want: true},
		{name: "on maxScore", f: TraceFilters{MaxScore: f64(0.5)}, score: f64(0.5), want: true},
		{name: "above maxScore", f: TraceFilters{MaxScore: f64(0.5)}, score: f64(0.5001), want: false},
		{name: "below minScore", f: TraceFilters{MinScore: f64(0.5)}, score: f64(0.4999), want: false},
		{name: "on minScore", f: TraceFilters{MinScore: f64(0.5)}, score: f64(0.5), want: true},
		{name: "above minScore", f: TraceFilters{MinScore: f64(0.5)}, score: f64(1), want: true},
		{name: "inside both", f: TraceFilters{MinScore: f64(0.3), MaxScore: f64(0.5)}, score: f64(0.4), want: true},
		{name: "on the lower of both", f: TraceFilters{MinScore: f64(0.3), MaxScore: f64(0.5)}, score: f64(0.3), want: true},
		{name: "below both", f: TraceFilters{MinScore: f64(0.3), MaxScore: f64(0.5)}, score: f64(0.2), want: false},
		{name: "above both", f: TraceFilters{MinScore: f64(0.3), MaxScore: f64(0.5)}, score: f64(0.6), want: false},
		{name: "equal bounds", f: TraceFilters{MinScore: f64(0.5), MaxScore: f64(0.5)}, score: f64(0.5), want: true},
		{name: "minScore 0 and a zero score", f: TraceFilters{MinScore: f64(0)}, score: f64(0), want: true},
		{name: "minScore 0 and no score", f: TraceFilters{MinScore: f64(0)}, want: false},
		{name: "maxScore 1 and no score", f: TraceFilters{MaxScore: f64(1)}, want: false},
		{name: "evaluator with a bound and no score", f: TraceFilters{MaxScore: f64(1), Evaluator: "Helpfulness"}, want: false},
		{name: "evaluator alone and a score", f: TraceFilters{Evaluator: "Helpfulness"}, score: f64(0), want: true},
		{name: "evaluator alone and no score", f: TraceFilters{Evaluator: "Helpfulness"}, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := matchesScore(tt.score, tt.f); got != tt.want {
				t.Errorf("matchesScore = %v, want %v", got, tt.want)
			}
		})
	}
}

// A score filter counts as a filter, isn't summary-only, and is logged.
func TestTraceFilters_ScoreFields(t *testing.T) {
	for _, f := range []TraceFilters{{MinScore: f64(0)}, {MaxScore: f64(1)}, {MaxScore: f64(1), Evaluator: "e"}, {MinDurationMs: ptr(1), MaxScore: f64(0.5)}} {
		if f.IsZero() || f.SummaryOnly() || !f.HasScoreFilter() {
			t.Errorf("%+v: IsZero = %t, SummaryOnly = %t, HasScoreFilter = %t", f, f.IsZero(), f.SummaryOnly(), f.HasScoreFilter())
		}
	}
	for _, f := range []TraceFilters{{}, {MinDurationMs: ptr(1)}, {Status: TraceStatusError, Tool: "a", MCPServer: "m", Model: "g"}} {
		if f.HasScoreFilter() {
			t.Errorf("%+v reads as a score filter", f)
		}
	}
	got := TraceFilters{MinScore: f64(0.25), MaxScore: f64(0.5), Evaluator: "Helpfulness"}.LogValue()
	want := slog.GroupValue(slog.Float64("minScore", 0.25), slog.Float64("maxScore", 0.5), slog.String("evaluator", "Helpfulness"))
	if !got.Equal(want) {
		t.Errorf("LogValue = %v, want %v", got, want)
	}
}

// maxScore over costFake: one lookup per chunk examined, sent before any
// upstream call. Only the 10 matches fetch their root, span list, chain and leaves.
func TestGetTraceOverviews_MaxScoreCallCounts(t *testing.T) {
	fake := costFake(true)
	c, scores := withScores(NewTracingController(fake))
	params := lookBackParams(10)
	params.Filters = TraceFilters{MaxScore: f64(0.5)}

	ids, _, truncated := traceIDs(c, t, params)

	want := traceRange(0, 200, 20)
	if !slices.Equal(ids, want) || truncated {
		t.Fatalf("traces = %v, truncated %v; want %v, false", ids, truncated, want)
	}
	// The 10th match is in the 4th chunk of 50; each chunk is sent whole, once.
	if got := scores.counts(); got != (scoreCounts{Calls: 4, IDs: 200}) {
		t.Errorf("lookups = %+v, want 4 lookups of 50 IDs, one per chunk", got)
	}
	if sent := scores.sentIDs(); !slices.Equal(sent, traceRange(0, 200, 1)) {
		t.Errorf("sent %d IDs, want trace-0000 to trace-0199 in page order", len(sent))
	}
	lists, roots, others := fetchedTraces(t, fake)
	if len(roots) != 10 || len(lists) != 10 || len(others) != 30 {
		t.Errorf("roots, span lists, other spans = %d, %d, %d; want 10, 10, 30, the matches' cascade only", len(roots), len(lists), len(others))
	}
	for _, i := range slices.Concat(lists, roots, others) {
		if i%20 != 0 {
			t.Errorf("trace-%04d is out of score range but was fetched", i)
		}
	}
}

// With status set the score check still comes first: only traces in score
// range fetch their root.
func TestGetTraceOverviews_ScoreBeforeRoot(t *testing.T) {
	for _, f := range []TraceFilters{
		{MaxScore: f64(0.5), Status: TraceStatusError},
		{MinScore: f64(0.5), Status: TraceStatusError},
		{MaxScore: f64(0.5), ConversationID: "conv-01"},
	} {
		t.Run(fmt.Sprint(f.LogValue()), func(t *testing.T) {
			fake := costFake(true)
			c, _ := withScores(NewTracingController(fake))
			params := lookBackParams(10)
			params.Filters = f

			if _, err := c.GetTraceOverviews(t.Context(), params); err != nil {
				t.Fatalf("GetTraceOverviews: %v", err)
			}
			lists, roots, others := fetchedTraces(t, fake)
			if len(roots) == 0 {
				t.Fatal("no root fetched")
			}
			for _, i := range slices.Concat(lists, roots, others) {
				if !scorePasses(i, f) {
					t.Errorf("trace-%04d is out of score range but was fetched", i)
				}
			}
		})
	}
}

// Traces the summary rejects are never sent to the service.
func TestGetTraceOverviews_ScoreSendsOnlySummarySurvivors(t *testing.T) {
	fake := costFake(true)
	c, scores := withScores(NewTracingController(fake))
	params := lookBackParams(10)
	params.Filters = TraceFilters{MaxScore: f64(0.5), MinDurationMs: ptr(800)}

	ids, _, truncated := traceIDs(c, t, params)

	// Every 20th trace lasts 0 ms, so nothing matches and the walk hits the cap.
	if len(ids) != 0 || !truncated {
		t.Fatalf("got %d traces, truncated %v; want 0, true", len(ids), truncated)
	}
	if got := scores.counts(); got != (scoreCounts{Calls: 10, IDs: 100}) {
		t.Errorf("lookups = %+v, want 10 lookups of 10 IDs, one per chunk", got)
	}
	for _, id := range scores.sentIDs() {
		if i := traceIndex(t, id); i%10 < 8 {
			t.Errorf("%s lasts %d ms but was sent", id, i%10*100)
		}
	}
	if n := upstreamCalls(fake) - atomic.LoadInt32(&fake.queryTracesCalls); n != 0 {
		t.Errorf("per-trace upstream calls = %d, want 0", n)
	}
}

// A chunk whose traces all fail the summary or the score makes no upstream call
// for them, and a chunk with no summary survivors makes no lookup.
func TestGetTraceOverviews_ScoreNoSurvivorsNoLookup(t *testing.T) {
	fake := costFake(true)
	c, scores := withScores(NewTracingController(fake))
	params := lookBackParams(10)
	params.Filters = TraceFilters{MaxScore: f64(0.5), MinDurationMs: ptr(10_000)}

	if _, err := c.GetTraceOverviews(t.Context(), params); err != nil {
		t.Fatalf("GetTraceOverviews: %v", err)
	}
	if got := scores.counts(); got.Calls != 0 {
		t.Errorf("lookups = %+v, want none", got)
	}
}

// evaluatorScores answers each evaluator's lookups from its own scores by trace ID.
type evaluatorScores map[string]map[string]*float64

// TraceScores returns evaluator's scores for ids; a trace it never scored is absent.
func (e evaluatorScores) TraceScores(_ context.Context, _, _, _ string, _, _ time.Time, ids []string, evaluator string) (map[string]*float64, error) {
	scores := map[string]*float64{}
	for _, id := range ids {
		if score, ok := e[evaluator][id]; ok {
			scores[id] = score
		}
	}
	return scores, nil
}

// A lone evaluator keeps traces with a non-skipped score from it, whatever the
// score, and drops traces it never scored or only skipped, before fetching them.
func TestGetTraceOverviews_LoneEvaluator(t *testing.T) {
	fake := costFake(true)
	c := NewTracingController(fake).WithScoreClient(evaluatorScores{
		"Helpfulness": {"trace-0000": f64(0), "trace-0002": f64(0.9), "trace-0003": nil},
		"Toxicity":    {"trace-0001": f64(0.5)},
	})
	params := lookBackParams(10)
	params.Filters = TraceFilters{Evaluator: "Helpfulness"}

	ids, _, truncated := traceIDs(c, t, params)

	if want := []string{"trace-0000", "trace-0002"}; !slices.Equal(ids, want) || !truncated {
		t.Fatalf("traces = %v, truncated %v; want %v, true", ids, truncated, want)
	}
	lists, roots, others := fetchedTraces(t, fake)
	for _, i := range slices.Concat(lists, roots, others) {
		if i != 0 && i != 2 {
			t.Errorf("trace-%04d has no Helpfulness score but was fetched", i)
		}
	}
}

// A score filter returns what the filter keeps of the fully enriched traces:
// the same pages, cursors, lookedBackTo and truncated, and the same rows, as if
// scores were checked last. A trace out of score range is never fetched.
func TestGetTraceOverviews_ScoreFiltersMatchFullEnrichment(t *testing.T) {
	tests := []struct {
		name    string
		filters TraceFilters
	}{
		{name: "maxScore", filters: TraceFilters{MaxScore: f64(0.5)}},
		{name: "minScore", filters: TraceFilters{MinScore: f64(0.5)}},
		{name: "minScore 0", filters: TraceFilters{MinScore: f64(0)}},
		{name: "both bounds", filters: TraceFilters{MinScore: f64(0.3), MaxScore: f64(0.5)}},
		{name: "a bound nothing meets", filters: TraceFilters{MinScore: f64(0.95)}},
		{name: "evaluator", filters: TraceFilters{MaxScore: f64(0.5), Evaluator: "Helpfulness"}},
		{name: "evaluator nobody runs", filters: TraceFilters{MinScore: f64(0), Evaluator: "Toxicity"}},
		{name: "evaluator alone", filters: TraceFilters{Evaluator: "Helpfulness"}},
		{name: "evaluator alone nobody runs", filters: TraceFilters{Evaluator: "Toxicity"}},
		{name: "maxScore and status", filters: TraceFilters{MaxScore: f64(0.5), Status: TraceStatusError}},
		{name: "minScore and status ok", filters: TraceFilters{MinScore: f64(0.5), Status: TraceStatusOK}},
		{name: "maxScore and toolError", filters: TraceFilters{MaxScore: f64(0.5), ToolError: true}},
		{name: "minScore and tool", filters: TraceFilters{MinScore: f64(0.5), Tool: "search_issues"}},
		{name: "minScore and model", filters: TraceFilters{MinScore: f64(0.5), Model: "claude"}},
		{name: "minScore and minTokens", filters: TraceFilters{MinScore: f64(0.5), MinTokens: ptr(40)}},
		{name: "minScore and minDurationMs", filters: TraceFilters{MinScore: f64(0.5), MinDurationMs: ptr(300)}},
		{name: "minScore and conversationId", filters: TraceFilters{MinScore: f64(0.5), ConversationID: "conv-01"}},
		{name: "evaluator, tool and model", filters: TraceFilters{MaxScore: f64(0.5), Evaluator: "Helpfulness", Tool: "issues", Model: "gpt"}},
	}
	for _, tt := range tests {
		for _, order := range []string{"desc", "asc"} {
			t.Run(tt.name+"/"+order, func(t *testing.T) {
				include := impliedInclude(Include{}, tt.filters)
				ref := referenceOverviews(t, toolFilterFake(), include)
				match := func(i int) bool {
					ov, ok := ref[i]
					return ok && matchesFilters(ov, tt.filters) && scorePasses(i, tt.filters)
				}
				params := lookBackParams(10)
				params.Filters = tt.filters
				params.SortOrder = order
				fake := toolFilterFake()
				c, scores := withScores(NewTracingController(fake))

				pages := pagesListedOnce(t, c, fake, params, 3)

				assertPages(t, pages, wantPages(fake, params, 3, match))
				for _, page := range pages {
					for _, ov := range page.Traces {
						if want := ref[traceIndex(t, ov.TraceID)]; !reflect.DeepEqual(ov, want) {
							t.Errorf("%s row = %+v, want the fully enriched %+v", ov.TraceID, ov, want)
						}
					}
				}
				lists, roots, others := fetchedTraces(t, fake)
				for _, i := range slices.Concat(lists, roots, others) {
					if !scorePasses(i, tt.filters) {
						t.Errorf("trace-%04d is out of score range but was fetched", i)
					}
				}
				for _, l := range scores.lookups {
					if l.evaluator != tt.filters.Evaluator || len(l.ids) > lookBackBatchSize {
						t.Errorf("lookup sent evaluator %q and %d IDs, want %q and at most %d", l.evaluator, len(l.ids), tt.filters.Evaluator, lookBackBatchSize)
					}
				}
			})
		}
	}
}

// Paging by cursor through a window whose traces overlap returns each trace in
// score range exactly once, in both sort orders.
func TestGetTraceOverviews_ScoreFilterCursorPagesWholeWindow(t *testing.T) {
	for _, order := range []string{"desc", "asc"} {
		t.Run(order, func(t *testing.T) {
			fake := costFake(true)
			fake.traces = fake.traces[:300]
			for i := range fake.traces {
				fake.traces[i].EndTime = fake.traces[i].StartTime.Add(5 * time.Second)
			}
			c, _ := withScores(NewTracingController(fake))
			params := lookBackParams(3)
			params.Filters = TraceFilters{MaxScore: f64(0.5)}
			params.SortOrder = order

			pages := pageAll(t, c, fake, params)

			// Traces 0-3 end after the window, so their roots are outside it.
			want := map[string]bool{}
			for _, id := range traceRange(20, 300, 20) {
				want[id] = true
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

// Export with maxScore fetches full spans only for the matches, and selects the
// same traces as the list's first page.
func TestExportTraces_MaxScoreFetchesSpansOnlyForMatches(t *testing.T) {
	fake := costFake(true)
	c, scores := withScores(NewTracingController(fake))
	params := exportParams(10, TraceFilters{MaxScore: f64(0.5)})

	resp := mustExport(t, c, params)

	want := traceRange(0, 200, 20)
	if got := exportedIDs(resp); !slices.Equal(got, want) {
		t.Fatalf("exported %v, want %v", got, want)
	}
	if got := atomic.LoadInt32(&fake.attrSpansCalls); got != 10 {
		t.Errorf("full span fetches = %d, want 10, one per match", got)
	}
	if got := scores.counts(); got.Calls != 4 {
		t.Errorf("lookups = %+v, want 4", got)
	}
	listC, _ := withScores(NewTracingController(costFake(true)))
	ids, _, _ := traceIDs(listC, t, params)
	if !slices.Equal(ids, want) {
		t.Errorf("list selected %v, want the export's %v", ids, want)
	}
}

// Without a score client a score filter fails before any call, on the list and
// the export; other filters don't need one.
func TestScoreFilterWithoutClient(t *testing.T) {
	fake := costFake(true)
	c := NewTracingController(fake)
	params := lookBackParams(10)
	params.Filters = TraceFilters{MaxScore: f64(0.5)}

	_, listErr := c.GetTraceOverviews(t.Context(), params)
	_, exportErr := c.ExportTraces(t.Context(), params)

	for name, err := range map[string]error{"list": listErr, "export": exportErr} {
		if !errors.Is(err, ErrScoresNotConfigured) {
			t.Errorf("%s err = %v, want ErrScoresNotConfigured", name, err)
		}
	}
	if n := upstreamCalls(fake); n != 0 {
		t.Errorf("upstream calls = %d, want 0", n)
	}
	params.Filters = TraceFilters{ToolError: true}
	if _, err := c.GetTraceOverviews(t.Context(), params); err != nil {
		t.Errorf("toolError without a score client: %v", err)
	}
}

// A failed lookup fails the list and the export, wrapping ErrScoreLookup and
// the cause, and the chunk it was for is never fetched.
func TestScoreLookupFails(t *testing.T) {
	cause := errors.New("agent-manager-service: status 500")
	for _, export := range []bool{false, true} {
		t.Run(fmt.Sprintf("export=%v", export), func(t *testing.T) {
			fake := costFake(true)
			scores := &fakeScoreClient{fail: func(n int) error {
				if n == 2 {
					return cause
				}
				return nil
			}}
			c := NewTracingController(fake).WithScoreClient(scores)
			params := lookBackParams(10)
			params.Filters = TraceFilters{MaxScore: f64(0.5)}

			var err error
			if export {
				_, err = c.ExportTraces(t.Context(), params)
			} else {
				_, err = c.GetTraceOverviews(t.Context(), params)
			}

			if !errors.Is(err, ErrScoreLookup) || !errors.Is(err, cause) {
				t.Fatalf("err = %v, want ErrScoreLookup wrapping the cause", err)
			}
			if got := scores.counts().Calls; got != 2 {
				t.Errorf("lookups = %d, want 2 (no retry, no later chunk)", got)
			}
			lists, roots, others := fetchedTraces(t, fake)
			for _, i := range slices.Concat(lists, roots, others) {
				if i >= 50 {
					t.Errorf("trace-%04d of the failed chunk was fetched", i)
				}
			}
		})
	}
}

// A lookup the budget cuts stops the walk before its chunk, as a cut fetch
// does: truncated, with a cursor after the last examined trace.
func TestScoreLookupCutByBudget(t *testing.T) {
	fake := costFake(true)
	c := NewTracingController(fake)
	clock := withClock(c)
	scores := &fakeScoreClient{onCall: func(_ context.Context, n int) {
		if n == 2 {
			clock.advance(listLookBackBudget)
		}
	}}
	c.WithScoreClient(scores)
	params := lookBackParams(10)
	params.Filters = TraceFilters{MaxScore: f64(0.5)}

	resp, err := c.GetTraceOverviews(t.Context(), params)
	if err != nil {
		t.Fatalf("GetTraceOverviews: %v", err)
	}

	ids := make([]string, len(resp.Traces))
	for i, ov := range resp.Traces {
		ids[i] = ov.TraceID
	}
	if want := traceRange(0, 50, 20); !slices.Equal(ids, want) || !resp.Truncated {
		t.Fatalf("traces = %v, truncated %v; want %v, true", ids, resp.Truncated, want)
	}
	cur, err := DecodeTraceCursor(resp.NextCursor)
	if err != nil {
		t.Fatalf("nextCursor: %v", err)
	}
	if cur.ID != "trace-0049" {
		t.Errorf("cursor at %s, want trace-0049, the last examined trace", cur.ID)
	}
}

// A cancelled request stops the walk at the lookup it was in.
func TestScoreLookupCancelled(t *testing.T) {
	fake := costFake(true)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	scores := &fakeScoreClient{onCall: func(_ context.Context, n int) {
		if n == 2 {
			cancel()
		}
	}}
	c := NewTracingController(fake).WithScoreClient(scores)
	params := lookBackParams(10)
	params.Filters = TraceFilters{MaxScore: f64(0.5)}

	_, err := c.GetTraceOverviews(ctx, params)

	if !errors.Is(err, context.Canceled) || errors.Is(err, ErrScoreLookup) {
		t.Fatalf("err = %v, want context.Canceled and not ErrScoreLookup", err)
	}
	if got := scores.counts().Calls; got != 2 {
		t.Errorf("lookups = %d, want 2", got)
	}
}

// A trace ruled out by score is examined but not read: when every trace in
// range fails to read, the list fails as if nothing could be read.
func TestScoreRejectedTracesAreNotRead(t *testing.T) {
	fake := costFake(true)
	fake.failCall = func(id string) error {
		if strings.HasPrefix(id, "root-") || strings.HasPrefix(id, "trace-") {
			return failEvery(id)
		}
		return nil
	}
	c, _ := withScores(NewTracingController(fake))
	params := lookBackParams(10)
	params.Filters = TraceFilters{MaxScore: f64(0.5)}

	_, err := c.GetTraceOverviews(t.Context(), params)

	if err == nil || !strings.Contains(err.Error(), "none of 25 traces could be read") {
		t.Errorf("err = %v, want none of the 25 in-range traces read", err)
	}
}

// Against an HTTP stand-in for the service: every lookup forwards the caller's
// token, names the agent, sends one chunk's IDs and the window padded by 1 s,
// and all of them share one connection.
func TestScoreLookupOverHTTP(t *testing.T) {
	type request struct {
		path, auth, start, end, ids string
	}
	var (
		mu       sync.Mutex
		requests []request
		conns    atomic.Int32
	)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		mu.Lock()
		requests = append(requests, request{path: r.URL.EscapedPath(), auth: r.Header.Get("Authorization"),
			start: q.Get("startTime"), end: q.Get("endTime"), ids: q.Get("traceIds")})
		mu.Unlock()
		type row struct {
			TraceID string   `json:"traceId"`
			Score   *float64 `json:"score"`
		}
		rows := []row{}
		for _, id := range strings.Split(q.Get("traceIds"), ",") {
			if score, ok := scoreOf(id, q.Get("evaluator")); ok {
				rows = append(rows, row{TraceID: id, Score: score})
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"traces": rows, "totalCount": len(rows)})
	}))
	srv.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			conns.Add(1)
		}
	}
	srv.Start()
	t.Cleanup(srv.Close)

	c := NewTracingController(costFake(true)).WithScoreClient(agentmanager.NewClient(srv.URL))
	params := lookBackParams(10)
	params.Filters = TraceFilters{MaxScore: f64(0.5)}
	ctx := middleware.ContextWithBearerToken(t.Context(), "caller-token")

	resp, err := c.GetTraceOverviews(ctx, params)
	if err != nil {
		t.Fatalf("GetTraceOverviews: %v", err)
	}

	if len(resp.Traces) != 10 {
		t.Errorf("got %d traces, want 10", len(resp.Traces))
	}
	if len(requests) != 4 {
		t.Fatalf("lookups = %d, want 4, one per chunk", len(requests))
	}
	wantPath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/scores", params.Organization, *params.Project, *params.Agent)
	wantStart := params.StartTime.Add(-time.Second).UTC().Format(time.RFC3339Nano)
	wantEnd := params.EndTime.Add(time.Second).UTC().Format(time.RFC3339Nano)
	var sent []string
	for i, r := range requests {
		if r.path != wantPath || r.auth != "Bearer caller-token" || r.start != wantStart || r.end != wantEnd {
			t.Errorf("lookup %d = %+v, want path %s, the caller's token and window %s..%s", i, r, wantPath, wantStart, wantEnd)
		}
		sent = append(sent, strings.Split(r.ids, ",")...)
	}
	if !slices.Equal(sent, traceRange(0, 200, 1)) {
		t.Errorf("sent %d IDs, want trace-0000 to trace-0199 in page order", len(sent))
	}
	if got := conns.Load(); got != 1 {
		t.Errorf("opened %d connections, want 1 reused across chunks", got)
	}
}
