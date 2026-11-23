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

package observability

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/tracesv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store/memory"
)

// fakeCore is one organization with one project and one agent.
type fakeCore struct{}

func (fakeCore) GetOrg(_ context.Context, orgID string) (modkit.Org, error) {
	if orgID != "org-a" {
		return modkit.Org{}, modkit.ErrNotFound
	}
	return modkit.Org{ID: orgID, Handle: "acme"}, nil
}

func (fakeCore) ListProjects(context.Context, string) ([]modkit.Project, error) {
	return nil, nil
}

func (fakeCore) GetProject(_ context.Context, orgID, projectID string) (modkit.Project, error) {
	if projectID != "p1" {
		return modkit.Project{}, modkit.ErrNotFound
	}
	return modkit.Project{ID: projectID, OrgID: orgID, Name: "shop"}, nil
}

func (fakeCore) ListAgents(context.Context, string, string) ([]modkit.Agent, error) {
	return nil, nil
}

func (fakeCore) GetAgent(_ context.Context, orgID, agentID string) (modkit.Agent, error) {
	if agentID != "a1" {
		return modkit.Agent{}, modkit.ErrNotFound
	}
	return modkit.Agent{ID: agentID, OrgID: orgID, ProjectID: "p1", ProjectName: "shop", Name: "bot"}, nil
}

func (fakeCore) ListEnvironments(_ context.Context, orgID string) ([]modkit.Environment, error) {
	return []modkit.Environment{{ID: "e1", OrgID: orgID, Name: "dev"}}, nil
}

func (fakeCore) GetProjectByName(_ context.Context, orgID, name string) (modkit.Project, error) {
	if name != "shop" {
		return modkit.Project{}, modkit.ErrNotFound
	}
	return modkit.Project{ID: "p1", OrgID: orgID, Name: name}, nil
}

func (fakeCore) GetAgentByName(_ context.Context, orgID, projectName, name string) (modkit.Agent, error) {
	if projectName != "shop" || name != "bot" {
		return modkit.Agent{}, modkit.ErrNotFound
	}
	return modkit.Agent{ID: "a1", OrgID: orgID, ProjectID: "p1", ProjectName: projectName, Name: name}, nil
}

func (fakeCore) GetEnvironmentByName(context.Context, string, string) (modkit.Environment, error) {
	return modkit.Environment{}, modkit.ErrNotFound
}

func (fakeCore) LockAgent(context.Context, *gorm.DB, string, string) (modkit.Agent, error) {
	return modkit.Agent{}, modkit.ErrNotFound
}

func (fakeCore) EnsureOrg(context.Context, string) (modkit.Org, error) {
	return modkit.Org{}, modkit.ErrNotImplemented
}

func newHandlers() handlers {
	return handlers{core: fakeCore{}, traces: guarded{traces: memory.New()}, log: slog.New(slog.DiscardHandler)}
}

func serve(t *testing.T, h http.HandlerFunc, org, target string) int {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, target, nil)
	if org != "" {
		r = r.WithContext(modkit.WithOrgID(r.Context(), org))
	}
	w := httptest.NewRecorder()
	h(w, r)
	return w.Code
}

const testWindow = "startTime=2026-11-01T00:00:00Z&endTime=2026-11-02T00:00:00Z"

func TestRoutesNeedAnOrganization(t *testing.T) {
	h := newHandlers()
	if got := serve(t, h.listTraces, "", "/?project=shop&agent=bot&"+testWindow); got != http.StatusUnauthorized {
		t.Errorf("listTraces without an org = %d, want 401", got)
	}
	if got := serve(t, h.listSpans, "", "/?"+testWindow); got != http.StatusUnauthorized {
		t.Errorf("listSpans without an org = %d, want 401", got)
	}
	if got := serve(t, h.getSpan, "", "/"); got != http.StatusUnauthorized {
		t.Errorf("getSpan without an org = %d, want 401", got)
	}
}

func TestListTracesRequests(t *testing.T) {
	h := newHandlers()
	for _, tc := range []struct {
		name, query string
		want        int
	}{
		{"resolves names", "project=shop&agent=bot&" + testWindow, http.StatusOK},
		{"unknown project", "project=nosuch&agent=bot&" + testWindow, http.StatusNotFound},
		{"unknown agent", "project=shop&agent=nosuch&" + testWindow, http.StatusNotFound},
		{"no agent", "project=shop&" + testWindow, http.StatusBadRequest},
		{"no window", "project=shop&agent=bot", http.StatusBadRequest},
		{"unsupported filter", "project=shop&agent=bot&model=gpt-4o&" + testWindow, http.StatusBadRequest},
		{"cursor", "project=shop&agent=bot&cursor=x&" + testWindow, http.StatusBadRequest},
		{"malformed unsupported filter", "project=shop&agent=bot&model=%zz&" + testWindow, http.StatusBadRequest},
	} {
		if got := serve(t, h.listTraces, "org-a", "/?"+tc.query); got != tc.want {
			t.Errorf("%s: listTraces = %d, want %d", tc.name, got, tc.want)
		}
		if got := serve(t, h.exportTraces, "org-a", "/?"+tc.query); got != tc.want {
			t.Errorf("%s: exportTraces = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestExportTraces(t *testing.T) {
	traces := memory.New()
	scope := store.Scope{
		OrgID: "org-a", ProjectID: "p1", AgentID: "a1",
		Names: store.Names{Org: "acme", Project: "shop", Agent: "bot"},
	}
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	spans := make([]tracesv1.SpanDetails, maxExportSpans+1)
	for i := range spans {
		spans[i] = tracesv1.SpanDetails{SpanID: fmt.Sprint(i), StartTime: at, Attributes: map[string]any{"i": i}}
	}
	traces.Add(scope, tracesv1.Trace{TraceID: "big", StartTime: at}, spans...)
	traces.Add(scope, tracesv1.Trace{TraceID: "small", StartTime: at.Add(time.Second)}, spans[0])
	h := handlers{core: fakeCore{}, traces: guarded{traces: traces}, log: slog.New(slog.DiscardHandler)}

	r := httptest.NewRequest(http.MethodGet, "/?project=shop&agent=bot&"+testWindow, nil)
	w := httptest.NewRecorder()
	h.exportTraces(w, r.WithContext(modkit.WithOrgID(r.Context(), "org-a")))
	if w.Code != http.StatusOK {
		t.Fatalf("exportTraces = %d %s, want 200", w.Code, w.Body.String())
	}
	var got TraceExportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Traces) != 2 || got.TotalCount != 2 || got.Traces[0].TraceID != "small" {
		t.Fatalf("exported %+v, want small then big", got)
	}
	if n := len(got.Traces[1].Spans); n != maxExportSpans {
		t.Errorf("big trace has %d spans, want the cap %d", n, maxExportSpans)
	}
	if !got.SpansTruncated || !got.Truncated {
		t.Errorf("spansTruncated = %v, truncated = %v; want both true", got.SpansTruncated, got.Truncated)
	}
	if got.Traces[0].Spans[0].Attributes["i"] != float64(0) {
		t.Errorf("exported span attributes = %v, want i=0", got.Traces[0].Spans[0].Attributes)
	}
}

func TestUnencodableSpanIsA500(t *testing.T) {
	traces := memory.New()
	traces.Add(store.Scope{OrgID: "org-a", Names: store.Names{Org: "acme"}},
		tracesv1.Trace{TraceID: "t"},
		tracesv1.SpanDetails{SpanID: "s", Attributes: map[string]any{"score": math.NaN()}})
	h := handlers{core: fakeCore{}, traces: guarded{traces: traces}, log: slog.New(slog.DiscardHandler)}
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.SetPathValue("traceId", "t")
	r.SetPathValue("spanId", "s")
	w := httptest.NewRecorder()
	h.getSpan(w, r.WithContext(modkit.WithOrgID(r.Context(), "org-a")))
	if w.Code != http.StatusInternalServerError || !json.Valid(w.Body.Bytes()) {
		t.Errorf("getSpan of an unencodable span = %d %q, want a JSON 500", w.Code, w.Body.String())
	}
}

func TestCapabilityRefusesAnotherOrganization(t *testing.T) {
	q := capability{core: fakeCore{}, traces: guarded{traces: memory.New()}}
	ctx := modkit.WithOrgID(context.Background(), "org-a")
	foreign := tracesv1.Scope{OrgID: "org-b"}
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	if _, err := q.QueryTraces(ctx, foreign, tracesv1.TraceQuery{StartTime: day}); !errors.Is(err, tracesv1.ErrForeignOrg) {
		t.Errorf("QueryTraces = %v, want ErrForeignOrg", err)
	}
	if _, err := q.QueryTraceSpans(ctx, foreign, "t", tracesv1.SpanQuery{}); !errors.Is(err, tracesv1.ErrForeignOrg) {
		t.Errorf("QueryTraceSpans = %v, want ErrForeignOrg", err)
	}
	if _, err := q.GetSpanDetails(ctx, foreign, "t", "s"); !errors.Is(err, tracesv1.ErrForeignOrg) {
		t.Errorf("GetSpanDetails = %v, want ErrForeignOrg", err)
	}
	if _, err := q.QueryTraces(context.Background(), foreign, tracesv1.TraceQuery{}); !errors.Is(err, tracesv1.ErrForeignOrg) {
		t.Errorf("QueryTraces with no org on ctx = %v, want ErrForeignOrg", err)
	}
	own := tracesv1.Scope{OrgID: "org-a"}
	if _, err := q.QueryTraces(ctx, own, tracesv1.TraceQuery{}); err != nil {
		t.Errorf("QueryTraces of the caller's org = %v, want nil", err)
	}
}

func TestCapabilityResolvesIDsToNames(t *testing.T) {
	traces := memory.New()
	named := store.Scope{
		OrgID: "org-a", ProjectID: "p1", AgentID: "a1", EnvironmentID: "e1",
		Names: store.Names{Org: "acme", Project: "shop", Agent: "bot", Environment: "dev"},
	}
	traces.Add(named, tracesv1.Trace{TraceID: "t"}, tracesv1.SpanDetails{SpanID: "s"})
	q := capability{core: fakeCore{}, traces: guarded{traces: traces}}
	ctx := modkit.WithOrgID(context.Background(), "org-a")

	for _, s := range []tracesv1.Scope{
		{OrgID: "org-a"},
		{OrgID: "org-a", ProjectID: "p1"},
		{OrgID: "org-a", AgentID: "a1"},
		{OrgID: "org-a", ProjectID: "p1", AgentID: "a1", EnvironmentID: "e1"},
	} {
		page, err := q.QueryTraces(ctx, s, tracesv1.TraceQuery{})
		if err != nil || len(page.Traces) != 1 {
			t.Errorf("QueryTraces(%+v) = %v, %v; want the trace", s, page, err)
		}
	}
	if _, err := q.GetSpanDetails(ctx, tracesv1.Scope{OrgID: "org-a", AgentID: "a1"}, "t", "s"); err != nil {
		t.Errorf("GetSpanDetails by agent ID = %v, want nil", err)
	}
	for _, s := range []tracesv1.Scope{
		{OrgID: "org-a", ProjectID: "nosuch"},
		{OrgID: "org-a", AgentID: "nosuch"},
		{OrgID: "org-a", ProjectID: "p2", AgentID: "a1"},
		{OrgID: "org-a", EnvironmentID: "nosuch"},
	} {
		if _, err := q.QueryTraces(ctx, s, tracesv1.TraceQuery{}); !errors.Is(err, tracesv1.ErrNotFound) {
			t.Errorf("QueryTraces(%+v) = %v, want ErrNotFound", s, err)
		}
	}
}
