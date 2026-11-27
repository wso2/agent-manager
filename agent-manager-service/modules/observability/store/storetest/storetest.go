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

// Package storetest is the test suite every store.TraceStore backend must pass.
package storetest

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
)

// Seed is one trace and its spans, stored under Scope.
type Seed struct {
	Scope store.Scope
	Trace store.Trace
	Spans []store.SpanDetails
}

// NewStore returns a backend holding exactly seeds.
type NewStore func(t *testing.T, seeds []Seed) store.TraceStore

// Run runs the suite against the backend newStore builds.
func Run(t *testing.T, newStore NewStore) {
	t.Run("OrgsWithCollidingIDsAreIsolated", func(t *testing.T) { testOrgIsolation(t, newStore) })
	t.Run("EmptyOrgIsRejected", func(t *testing.T) { testEmptyOrgRejected(t, newStore) })
	t.Run("ScopeIDsFilterWithoutNames", func(t *testing.T) { testScopeIDsFilter(t, newStore) })
	t.Run("ListAndSpansAgreeOnATrace", func(t *testing.T) { testListAndSpansAgree(t, newStore) })
	t.Run("SpansByTrace", func(t *testing.T) { testSpansByTrace(t, newStore) })
}

// span is a span of the given name starting at at.
func span(id, parent string, at time.Time) store.SpanDetails {
	return store.SpanDetails{SpanID: id, SpanName: id, ParentSpanID: parent, StartTime: at, EndTime: at.Add(time.Second), DurationNs: time.Second.Nanoseconds()}
}

// testListAndSpansAgree lists a trace that starts just before the window ends
// and runs past it: its span list, by itself or with others, holds exactly
// the spans the trace list counts.
func testListAndSpansAgree(t *testing.T, newStore NewStore) {
	ctx := context.Background()
	sc := scopeFor("org-a", "acme")
	root := windowEnd.Add(-time.Minute)
	s := newStore(t, []Seed{{
		Scope: sc,
		Trace: store.Trace{TraceID: "late", RootSpanID: "root", StartTime: root},
		Spans: []store.SpanDetails{
			span("root", "", root),
			span("after-window", "root", windowEnd.Add(time.Hour)),
			span("past-spread", "root", windowEnd.Add(store.TraceSpread+time.Hour)),
		},
	}})

	page, err := s.QueryTraces(ctx, sc, store.TraceQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 10})
	if err != nil {
		t.Fatalf("QueryTraces: %v", err)
	}
	if len(page.Traces) != 1 {
		t.Fatalf("QueryTraces = %+v, want the late trace", page)
	}
	q := store.SpanQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 10, SortOrder: "asc"}
	spans, err := s.QueryTraceSpans(ctx, sc, "late", q)
	if err != nil {
		t.Fatalf("QueryTraceSpans: %v", err)
	}
	if got := spanIDs(spans); len(got) != page.Traces[0].SpanCount || len(got) != 2 || got[0] != "root" || got[1] != "after-window" {
		t.Errorf("QueryTraceSpans = %v, list counts %d; want root and after-window", got, page.Traces[0].SpanCount)
	}
	byTrace, err := s.QuerySpansByTrace(ctx, sc, []string{"late"}, q)
	if err != nil {
		t.Fatalf("QuerySpansByTrace: %v", err)
	}
	if got := spanIDs(byTrace["late"]); len(got) != 2 || got[0] != "root" || got[1] != "after-window" {
		t.Errorf("QuerySpansByTrace = %v, want root and after-window", got)
	}

	// A window that ends later counts, and lists, the third span too.
	laterEnd := windowEnd.Add(2 * time.Hour)
	page, err = s.QueryTraces(ctx, sc, store.TraceQuery{StartTime: windowStart, EndTime: laterEnd, Limit: 10})
	if err != nil {
		t.Fatalf("QueryTraces, later window: %v", err)
	}
	spans, err = s.QueryTraceSpans(ctx, sc, "late", store.SpanQuery{StartTime: windowStart, EndTime: laterEnd, Limit: 10})
	if err != nil {
		t.Fatalf("QueryTraceSpans, later window: %v", err)
	}
	if len(page.Traces) != 1 || page.Traces[0].SpanCount != 3 || len(spans.Spans) != 3 {
		t.Errorf("later window: list counts %+v, span list has %d; want 3 in both", page.Traces, len(spans.Spans))
	}
}

// testSpansByTrace reads several traces' spans at once: at most the limit of
// each, with each trace's total, none of another org's, and none for a trace
// that is not there.
func testSpansByTrace(t *testing.T, newStore NewStore) {
	ctx := context.Background()
	orgA, orgB := scopeFor("org-a", "acme"), scopeFor("org-b", "globex")
	at := windowStart.Add(time.Hour)
	quoted := `trace-'q\`
	s := newStore(t, []Seed{
		{
			Scope: orgA, Trace: store.Trace{TraceID: "trace-1", StartTime: at},
			Spans: []store.SpanDetails{span("1a", "", at), span("1b", "1a", at.Add(time.Second))},
		},
		{Scope: orgA, Trace: store.Trace{TraceID: quoted, StartTime: at}, Spans: []store.SpanDetails{span("qa", "", at)}},
		{Scope: orgB, Trace: store.Trace{TraceID: "trace-2", StartTime: at}, Spans: []store.SpanDetails{span("2a", "", at)}},
	})

	pages, err := s.QuerySpansByTrace(ctx, orgA, []string{"trace-1", quoted, "trace-2", "missing"},
		store.SpanQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 1, SortOrder: "asc"})
	if err != nil {
		t.Fatalf("QuerySpansByTrace: %v", err)
	}
	if len(pages) != 2 || pages["trace-1"] == nil || pages[quoted] == nil {
		t.Fatalf("QuerySpansByTrace = %v, want only org-a's trace-1 and %s", pages, quoted)
	}
	if got := spanIDs(pages["trace-1"]); len(got) != 1 || got[0] != "1a" || pages["trace-1"].Total != 2 {
		t.Errorf("trace-1 = %v of %d, want the first of 2", got, pages["trace-1"].Total)
	}
	if got := spanIDs(pages[quoted]); len(got) != 1 || got[0] != "qa" {
		t.Errorf("%s = %v, want qa", quoted, got)
	}
	noOrg := orgA
	noOrg.Names.Org = ""
	if _, err := s.QuerySpansByTrace(ctx, noOrg, []string{"trace-1"}, store.SpanQuery{}); !errors.Is(err, store.ErrMissingOrg) {
		t.Errorf("QuerySpansByTrace with no org = %v, want ErrMissingOrg", err)
	}
}

func spanIDs(p *store.SpanPage) []string {
	if p == nil {
		return nil
	}
	out := make([]string, 0, len(p.Spans))
	for _, sp := range p.Spans {
		out = append(out, sp.SpanID)
	}
	return out
}

var (
	windowStart = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	windowEnd   = windowStart.Add(24 * time.Hour)
)

// scopeFor returns a Scope whose project, agent and environment IDs and names
// are the same for every org, so only the org tells two scopes apart.
func scopeFor(orgID, orgName string) store.Scope {
	return store.Scope{
		OrgID:         orgID,
		ProjectID:     "project-1",
		AgentID:       "agent-1",
		EnvironmentID: "env-1",
		Names:         store.Names{Org: orgName, Project: "shop", Agent: "support-bot", Environment: "dev"},
	}
}

// seedFor returns one trace of one span, named after the scope's org.
func seedFor(sc store.Scope, traceID, spanID string) Seed {
	start := windowStart.Add(time.Hour)
	end := start.Add(time.Second)
	name := "invoke_agent " + sc.Names.Org
	return Seed{
		Scope: sc,
		Trace: store.Trace{
			TraceID: traceID, TraceName: name, SpanCount: 1,
			RootSpanID: spanID, RootSpanName: name,
			StartTime: start, EndTime: end, DurationNs: end.Sub(start).Nanoseconds(),
		},
		Spans: []store.SpanDetails{{
			SpanID: spanID, SpanName: name,
			StartTime: start, EndTime: end, DurationNs: end.Sub(start).Nanoseconds(),
			Attributes: map[string]any{"org": sc.OrgID},
		}},
	}
}

// testOrgIsolation stores the same trace and span IDs in two orgs; each org
// must read only its own payload.
func testOrgIsolation(t *testing.T, newStore NewStore) {
	ctx := context.Background()
	orgA := scopeFor("org-a", "acme")
	orgB := scopeFor("org-b", "globex")
	s := newStore(t, []Seed{
		seedFor(orgA, "trace-1", "span-1"),
		seedFor(orgB, "trace-1", "span-1"),
	})

	for _, sc := range []store.Scope{orgA, orgB} {
		org, want := sc.Names.Org, "invoke_agent "+sc.Names.Org

		page, err := s.QueryTraces(ctx, sc, store.TraceQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 10, SortOrder: "desc"})
		if err != nil {
			t.Fatalf("%s: QueryTraces: %v", org, err)
		}
		if len(page.Traces) != 1 || page.Total != 1 || page.Traces[0].RootSpanName != want {
			t.Errorf("%s: QueryTraces = %+v, want only its own trace with total 1", org, page)
		}

		spans, err := s.QueryTraceSpans(ctx, sc, "trace-1", store.SpanQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 10})
		if err != nil {
			t.Fatalf("%s: QueryTraceSpans: %v", org, err)
		}
		if len(spans.Spans) != 1 || spans.Spans[0].SpanName != want {
			t.Errorf("%s: QueryTraceSpans = %+v, want only its own span", org, spans)
		}

		d, err := s.GetSpanDetails(ctx, sc, "trace-1", "span-1")
		if err != nil {
			t.Fatalf("%s: GetSpanDetails: %v", org, err)
		}
		if d.SpanName != want || d.Attributes["org"] != sc.OrgID {
			t.Errorf("%s: GetSpanDetails = %s %v, want its own span", org, d.SpanName, d.Attributes)
		}
	}
}

// testScopeIDsFilter queries by agent ID alone; another agent's trace must
// not match.
func testScopeIDsFilter(t *testing.T, newStore NewStore) {
	ctx := context.Background()
	agent1 := scopeFor("org-a", "acme")
	agent2 := agent1
	agent2.AgentID, agent2.Names.Agent = "agent-2", "billing-bot"
	s := newStore(t, []Seed{
		seedFor(agent1, "trace-1", "span-1"),
		seedFor(agent2, "trace-2", "span-2"),
	})

	byID := store.Scope{OrgID: "org-a", ProjectID: "project-1", AgentID: "agent-2", Names: store.Names{Org: "acme"}}
	page, err := s.QueryTraces(ctx, byID, store.TraceQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 10})
	if err != nil {
		t.Fatalf("QueryTraces: %v", err)
	}
	if len(page.Traces) != 1 || page.Traces[0].TraceID != "trace-2" {
		t.Errorf("QueryTraces by agent ID = %+v, want only trace-2", page)
	}
}

func testEmptyOrgRejected(t *testing.T, newStore NewStore) {
	ctx := context.Background()
	orgA := scopeFor("org-a", "acme")
	s := newStore(t, []Seed{seedFor(orgA, "trace-a", "span-a")})

	noOrg := orgA
	noOrg.Names.Org = ""

	if page, err := s.QueryTraces(ctx, noOrg, store.TraceQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 10}); !errors.Is(err, store.ErrMissingOrg) {
		t.Errorf("QueryTraces with no org = %+v, %v; want ErrMissingOrg", page, err)
	}
	if page, err := s.QueryTraceSpans(ctx, noOrg, "trace-a", store.SpanQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 10}); !errors.Is(err, store.ErrMissingOrg) {
		t.Errorf("QueryTraceSpans with no org = %+v, %v; want ErrMissingOrg", page, err)
	}
	if d, err := s.GetSpanDetails(ctx, noOrg, "trace-a", "span-a"); !errors.Is(err, store.ErrMissingOrg) {
		t.Errorf("GetSpanDetails with no org = %+v, %v; want ErrMissingOrg", d, err)
	}
}
