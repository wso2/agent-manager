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
