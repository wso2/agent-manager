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

package memory_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store/memory"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store/storetest"
)

func TestSuite(t *testing.T) {
	storetest.Run(t, func(_ *testing.T, seeds []storetest.Seed) store.TraceStore {
		m := memory.New()
		for _, s := range seeds {
			m.Add(s.Scope, s.Trace, s.Spans...)
		}
		return m
	})
}

func TestEmptyOrgRunsNoQuery(t *testing.T) {
	m := memory.New()
	m.Add(store.Scope{OrgID: "org-a", Names: store.Names{Org: "acme"}}, store.Trace{TraceID: "trace-a"})

	for _, sc := range []store.Scope{{OrgID: "org-a"}, {Names: store.Names{Org: "acme"}}} {
		if _, err := m.QueryTraces(context.Background(), sc, store.TraceQuery{}); !errors.Is(err, store.ErrMissingOrg) {
			t.Fatalf("QueryTraces(%+v) error = %v, want ErrMissingOrg", sc, err)
		}
		if _, err := m.QueryTraceSpans(context.Background(), sc, "trace-a", store.SpanQuery{}); !errors.Is(err, store.ErrMissingOrg) {
			t.Fatalf("QueryTraceSpans(%+v) error = %v, want ErrMissingOrg", sc, err)
		}
		if _, err := m.GetSpanDetails(context.Background(), sc, "trace-a", "span-a"); !errors.Is(err, store.ErrMissingOrg) {
			t.Fatalf("GetSpanDetails(%+v) error = %v, want ErrMissingOrg", sc, err)
		}
	}
	if n := m.Queries(); n != 0 {
		t.Errorf("Queries() = %d, want 0", n)
	}
}

func TestDeleteAgentWithNoOrgDeletesNothing(t *testing.T) {
	ctx := context.Background()
	m := memory.New()
	sc := store.Scope{OrgID: "org-a", AgentID: "agent-1", Names: store.Names{Org: "acme"}}
	m.Add(sc, store.Trace{TraceID: "trace-a"})

	if err := m.DeleteAgent(ctx, "", "agent-1"); !errors.Is(err, store.ErrMissingOrg) {
		t.Fatalf("DeleteAgent with no org error = %v, want ErrMissingOrg", err)
	}
	page, err := m.QueryTraces(ctx, sc, store.TraceQuery{})
	if err != nil || page.Total == nil || *page.Total != 1 {
		t.Errorf("after DeleteAgent with no org: %v, %v; want the trace kept", page, err)
	}
}

func TestQueryTraceSpans_WindowAndOrder(t *testing.T) {
	base := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	sc := store.Scope{OrgID: "org-a", Names: store.Names{Org: "acme"}}
	m := memory.New()
	m.Add(sc, store.Trace{TraceID: "t1"},
		store.SpanDetails{SpanID: "early", StartTime: base},
		store.SpanDetails{SpanID: "a", StartTime: base.Add(2 * time.Hour)},
		store.SpanDetails{SpanID: "b", StartTime: base.Add(3 * time.Hour)},
	)

	page, err := m.QueryTraceSpans(context.Background(), sc, "t1", store.SpanQuery{
		StartTime: base.Add(time.Hour), EndTime: base.Add(4 * time.Hour), SortOrder: "desc", Limit: 1,
	})
	if err != nil {
		t.Fatalf("QueryTraceSpans: %v", err)
	}
	if page.Total != 2 || len(page.Spans) != 1 || page.Spans[0].SpanID != "b" {
		t.Errorf("QueryTraceSpans = %+v, want total 2 and only span b", page)
	}
}

func TestOrgMatchesOnIDAndName(t *testing.T) {
	m := memory.New()
	m.Add(store.Scope{OrgID: "org-a", Names: store.Names{Org: "acme"}}, store.Trace{TraceID: "t1"}, store.SpanDetails{SpanID: "s1"})

	for _, sc := range []store.Scope{
		{OrgID: "org-a", Names: store.Names{Org: "globex"}},
		{OrgID: "org-b", Names: store.Names{Org: "acme"}},
	} {
		page, err := m.QueryTraces(context.Background(), sc, store.TraceQuery{})
		if err != nil {
			t.Fatalf("QueryTraces: %v", err)
		}
		if len(page.Traces) != 0 {
			t.Errorf("QueryTraces(%+v) leaked %+v", sc, page.Traces)
		}
		if _, err := m.GetSpanDetails(context.Background(), sc, "t1", "s1"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("GetSpanDetails(%+v) error = %v, want ErrNotFound", sc, err)
		}
	}
}
