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

	"github.com/wso2/agent-manager/agent-manager-observer/store"
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

func seedFor(sc store.Scope, traceID, spanID string) Seed {
	start := windowStart.Add(time.Hour)
	end := start.Add(time.Second)
	return Seed{
		Scope: sc,
		Trace: store.Trace{
			TraceID: traceID, TraceName: "invoke_agent", SpanCount: 1,
			RootSpanID: spanID, RootSpanName: "invoke_agent",
			StartTime: start, EndTime: end, DurationNs: end.Sub(start).Nanoseconds(),
		},
		Spans: []store.SpanDetails{{
			SpanID: spanID, SpanName: "invoke_agent",
			StartTime: start, EndTime: end, DurationNs: end.Sub(start).Nanoseconds(),
			Attributes: map[string]interface{}{"org": sc.OrgID},
		}},
	}
}

func testOrgIsolation(t *testing.T, newStore NewStore) {
	ctx := context.Background()
	orgA := scopeFor("org-a", "acme")
	orgB := scopeFor("org-b", "globex")
	s := newStore(t, []Seed{
		seedFor(orgA, "trace-a", "span-a"),
		seedFor(orgB, "trace-b", "span-b"),
	})

	for _, tc := range []struct {
		scope                 store.Scope
		ownTrace, ownSpan     string
		otherTrace, otherSpan string
	}{
		{orgA, "trace-a", "span-a", "trace-b", "span-b"},
		{orgB, "trace-b", "span-b", "trace-a", "span-a"},
	} {
		org := tc.scope.Names.Org

		page, err := s.QueryTraces(ctx, tc.scope, store.TraceQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 10, SortOrder: "desc"})
		if err != nil {
			t.Fatalf("%s: QueryTraces: %v", org, err)
		}
		if len(page.Traces) != 1 || page.Traces[0].TraceID != tc.ownTrace || page.Total != 1 {
			t.Errorf("%s: QueryTraces = %+v, want only %s with total 1", org, page, tc.ownTrace)
		}

		spans, err := s.QueryTraceSpans(ctx, tc.scope, tc.ownTrace, store.SpanQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 10})
		if err != nil {
			t.Fatalf("%s: QueryTraceSpans(own): %v", org, err)
		}
		if len(spans.Spans) != 1 || spans.Spans[0].SpanID != tc.ownSpan {
			t.Errorf("%s: QueryTraceSpans(own) = %+v, want only %s", org, spans, tc.ownSpan)
		}

		spans, err = s.QueryTraceSpans(ctx, tc.scope, tc.otherTrace, store.SpanQuery{StartTime: windowStart, EndTime: windowEnd, Limit: 10})
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("%s: QueryTraceSpans(other) error = %v, want nil or ErrNotFound", org, err)
		}
		if err == nil && len(spans.Spans) != 0 {
			t.Errorf("%s: QueryTraceSpans(other) leaked %+v", org, spans.Spans)
		}

		d, err := s.GetSpanDetails(ctx, tc.scope, tc.ownTrace, tc.ownSpan)
		if err != nil {
			t.Fatalf("%s: GetSpanDetails(own): %v", org, err)
		}
		if d.SpanID != tc.ownSpan {
			t.Errorf("%s: GetSpanDetails(own) = %s, want %s", org, d.SpanID, tc.ownSpan)
		}

		if d, err := s.GetSpanDetails(ctx, tc.scope, tc.otherTrace, tc.otherSpan); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("%s: GetSpanDetails(other) = %+v, %v; want ErrNotFound", org, d, err)
		}
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
}
