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
	"net/url"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/tracesv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store/clickhouse"
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
		{"filter", "project=shop&agent=bot&model=gpt-4o&" + testWindow, http.StatusOK},
		{"cursor the store didn't issue", "project=shop&agent=bot&cursor=x&" + testWindow, http.StatusBadRequest},
		{"malformed filter", "project=shop&agent=bot&model=%zz&" + testWindow, http.StatusBadRequest},
	} {
		if got := serve(t, h.listTraces, "org-a", "/?"+tc.query); got != tc.want {
			t.Errorf("%s: listTraces = %d, want %d", tc.name, got, tc.want)
		}
		if got := serve(t, h.exportTraces, "org-a", "/?"+tc.query); got != tc.want {
			t.Errorf("%s: exportTraces = %d, want %d", tc.name, got, tc.want)
		}
	}
	if got := serve(t, h.listSpans, "org-a", "/?project=shop&agent=bot&model=%zz&"+testWindow); got != http.StatusBadRequest {
		t.Errorf("malformed query: listSpans = %d, want 400", got)
	}
}

func TestListTracesPage(t *testing.T) {
	traces := memory.New()
	scope := store.Scope{
		OrgID: "org-a", ProjectID: "p1", AgentID: "a1",
		Names: store.Names{Org: "acme", Project: "shop", Agent: "bot"},
	}
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	for i := range 3 {
		traces.Add(scope, tracesv1.Trace{TraceID: fmt.Sprint(i), StartTime: at.Add(time.Duration(i) * time.Second)})
	}
	h := handlers{core: fakeCore{}, traces: guarded{traces: traces}, log: slog.New(slog.DiscardHandler)}

	r := httptest.NewRequest(http.MethodGet, "/?project=shop&agent=bot&limit=2&"+testWindow, nil)
	w := httptest.NewRecorder()
	h.listTraces(w, r.WithContext(modkit.WithOrgID(r.Context(), "org-a")))
	if w.Code != http.StatusOK {
		t.Fatalf("listTraces = %d %s, want 200", w.Code, w.Body.String())
	}
	var got TraceListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Traces) != 2 || got.Traces[0].TraceID != "2" {
		t.Fatalf("listed %+v, want the 2 newest", got.Traces)
	}
	if got.Pagination.NextCursor == "" || got.Pagination.Total == nil || *got.Pagination.Total != 3 {
		t.Fatalf("pagination = %+v, want a cursor and total 3", got.Pagination)
	}

	r = httptest.NewRequest(http.MethodGet, "/?project=shop&agent=bot&limit=2&cursor="+url.QueryEscape(got.Pagination.NextCursor)+"&"+testWindow, nil)
	w = httptest.NewRecorder()
	h.listTraces(w, r.WithContext(modkit.WithOrgID(r.Context(), "org-a")))
	var next TraceListResponse
	if err := json.Unmarshal(w.Body.Bytes(), &next); err != nil || w.Code != http.StatusOK {
		t.Fatalf("second page = %d %s", w.Code, w.Body.String())
	}
	if len(next.Traces) != 1 || next.Traces[0].TraceID != "0" {
		t.Fatalf("second page listed %+v, want the oldest", next.Traces)
	}
	if got, want := rawPagination(t, w.Body.Bytes()), `{"limit":2,"total":3}`; got != want {
		t.Errorf("last page pagination = %s, want %s: no cursor, not truncated", got, want)
	}
}

// The observer's filter checks answer 400 with its messages, on the list and the export.
func TestTraceFilterErrors(t *testing.T) {
	h := newHandlers()
	long := url.QueryEscape(strings.Repeat("é", maxFilterValueLen+1))
	for _, tc := range []struct{ query, msg string }{
		{"status=failed", "status must be 'error' or 'ok'"},
		{"status=ERROR", "status must be 'error' or 'ok'"},
		{"minDurationMs=1.5", "minDurationMs must be a non-negative integer"},
		{"minTokens=-1", "minTokens must be a non-negative integer"},
		{"minSpanCount=lots", "minSpanCount must be a non-negative integer"},
		{"model=" + long, "model must be at most 256 characters"},
		{"conversationId=" + long, "conversationId must be at most 256 characters"},
		{"tool=" + long, "tool must be at most 256 characters"},
		{"mcpServer=" + long, "mcpServer must be at most 256 characters"},
		{"toolError=TRUE", "toolError must be 'true' or 'false'"},
		{"toolError=1", "toolError must be 'true' or 'false'"},
		{"missing=model", `invalid missing value "model": must be 'conversationId'`},
		{"missing=conversationId&conversationId=c1", "conversationId and missing=conversationId can't be combined"},
	} {
		for name, serveFn := range map[string]http.HandlerFunc{"list": h.listTraces, "export": h.exportTraces} {
			r := httptest.NewRequest(http.MethodGet, "/?project=shop&agent=bot&"+testWindow+"&"+tc.query, nil)
			w := httptest.NewRecorder()
			serveFn(w, r.WithContext(modkit.WithOrgID(r.Context(), "org-a")))
			var body ErrorResponse
			_ = json.Unmarshal(w.Body.Bytes(), &body)
			if w.Code != http.StatusBadRequest || body.Message != tc.msg {
				t.Errorf("%s %s = %d %q, want 400 %q", name, tc.query, w.Code, body.Message, tc.msg)
			}
		}
	}
}

func TestTraceFilterValues(t *testing.T) {
	at256 := strings.Repeat("é", maxFilterValueLen)
	for _, tc := range []struct {
		query string
		want  tracesv1.TraceFilters
	}{
		{"", tracesv1.TraceFilters{}},
		{"status=ok&toolError=false", tracesv1.TraceFilters{Status: "ok"}},
		{"minTokens=0", tracesv1.TraceFilters{MinTokens: ptr(int64(0))}},
		{"minDurationMs=5000&minSpanCount=20", tracesv1.TraceFilters{MinDurationMs: ptr(int64(5000)), MinSpanCount: ptr(int64(20))}},
		{"model=" + at256, tracesv1.TraceFilters{Model: at256}},
		{"tool=search&toolError=true&mcpServer=github", tracesv1.TraceFilters{Tool: "search", ToolError: true, MCPServer: "github"}},
		{"missing=%20conversationId,&missing=conversationId", tracesv1.TraceFilters{MissingConversationID: true}},
	} {
		query, _ := url.ParseQuery(tc.query)
		got, err := parseTraceFilters(query)
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("parseTraceFilters(%q) = %+v, %v; want %+v", tc.query, got, err, tc.want)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// queryStore records the query it is asked and answers with page.
type queryStore struct {
	store.TraceStore
	page *tracesv1.TracePage
	got  tracesv1.TraceQuery
}

func (s *queryStore) QueryTraces(_ context.Context, _ store.Scope, q tracesv1.TraceQuery) (*tracesv1.TracePage, error) {
	s.got = q
	return s.page, nil
}

func TestListTracesPassesFiltersAndCursor(t *testing.T) {
	traces := &queryStore{page: &tracesv1.TracePage{}}
	h := handlers{core: fakeCore{}, traces: guarded{traces: traces}, log: slog.New(slog.DiscardHandler)}
	serve(t, h.listTraces, "org-a", "/?project=shop&agent=bot&status=error&tool=search&cursor=c1&"+testWindow)
	want := tracesv1.TraceFilters{Status: "error", Tool: "search"}
	if traces.got.Filters != want || traces.got.Cursor != "c1" {
		t.Errorf("store got %+v, want filters %+v and cursor c1", traces.got, want)
	}
	if got := serve(t, h.exportTraces, "org-a", "/?project=shop&agent=bot&cursor=c1&"+testWindow); got != http.StatusBadRequest {
		t.Errorf("export with a cursor = %d, want 400", got)
	}
}

// A store that stopped early reports it; the pagination says how far it looked.
func TestListTracesTruncatedPage(t *testing.T) {
	at := time.Date(2026, 11, 1, 9, 40, 0, 0, time.UTC)
	traces := &queryStore{page: &tracesv1.TracePage{NextCursor: "c2", Truncated: true, LookedBackTo: at}}
	h := handlers{core: fakeCore{}, traces: guarded{traces: traces}, log: slog.New(slog.DiscardHandler)}
	r := httptest.NewRequest(http.MethodGet, "/?project=shop&agent=bot&limit=5&"+testWindow, nil)
	w := httptest.NewRecorder()
	h.listTraces(w, r.WithContext(modkit.WithOrgID(r.Context(), "org-a")))
	want := `{"limit":5,"nextCursor":"c2","lookedBackTo":"2026-11-01T09:40:00Z","truncated":true}`
	if got := rawPagination(t, w.Body.Bytes()); got != want {
		t.Errorf("pagination = %s, want %s", got, want)
	}
}

// rawPagination is a response body's pagination as sent.
func rawPagination(t *testing.T, body []byte) string {
	t.Helper()
	var raw struct{ Pagination json.RawMessage }
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	return string(raw.Pagination)
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
	late := tracesv1.SpanDetails{SpanID: "late", StartTime: at.Add(24 * time.Hour)}
	traces.Add(scope, tracesv1.Trace{TraceID: "small", StartTime: at.Add(time.Second)}, spans[0], late)
	h := handlers{core: fakeCore{}, traces: guarded{traces: traces}, log: slog.New(slog.DiscardHandler)}

	r := httptest.NewRequest(http.MethodGet, "/?project=shop&agent=bot&"+testWindow, nil)
	w := httptest.NewRecorder()
	h.exportTraces(w, r.WithContext(modkit.WithOrgID(r.Context(), "org-a")))
	if w.Code != http.StatusOK {
		t.Fatalf("exportTraces = %d %s, want 200", w.Code, w.Body.String())
	}
	if n := traces.Queries(); n != 2 {
		t.Errorf("export ran %d store queries, want 2: the list and one for every trace's spans", n)
	}
	var got TraceExportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Traces) != 2 || got.Pagination.Total == nil || *got.Pagination.Total != 2 || got.Traces[0].TraceID != "small" {
		t.Fatalf("exported %+v, want small then big", got)
	}
	if n := len(got.Traces[1].Spans); n != maxExportSpans {
		t.Errorf("big trace has %d spans, want the cap %d", n, maxExportSpans)
	}
	if p, want := rawPagination(t, w.Body.Bytes()), `{"limit":100,"total":2}`; p != want {
		t.Errorf("pagination = %s, want %s: no cursor, not truncated", p, want)
	}
	if !got.SpansTruncated {
		t.Error("spansTruncated = false, want true")
	}
	if n := len(got.Traces[0].Spans); n != 2 {
		t.Errorf("small trace has %d spans, want 2, the one after the window included", n)
	}
	if got.Traces[0].Spans[0].Attributes["i"] != float64(0) {
		t.Errorf("exported span attributes = %v, want i=0", got.Traces[0].Spans[0].Attributes)
	}
}

// overviewKeys are the overview fields a trace sends only when it has them.
var overviewKeys = []string{"status", "tokenUsage", "conversationId", "input", "output"}

// rawTraces is each listed or exported trace's overview fields, as sent.
func rawTraces(t *testing.T, body []byte) map[string]map[string]json.RawMessage {
	t.Helper()
	var raw struct{ Traces []map[string]json.RawMessage }
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatal(err)
	}
	out := map[string]map[string]json.RawMessage{}
	for _, tr := range raw.Traces {
		var id string
		if err := json.Unmarshal(tr["traceId"], &id); err != nil {
			t.Fatal(err)
		}
		fields := map[string]json.RawMessage{}
		for _, k := range overviewKeys {
			if v, ok := tr[k]; ok {
				fields[k] = v
			}
		}
		out[id] = fields
	}
	return out
}

// The list and the export send each trace's status, tokens, conversation,
// input and output, and leave out what a trace doesn't have.
func TestTraceOverviewJSON(t *testing.T) {
	traces := memory.New()
	scope := store.Scope{
		OrgID: "org-a", ProjectID: "p1", AgentID: "a1",
		Names: store.Names{Org: "acme", Project: "shop", Agent: "bot"},
	}
	at := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	traces.Add(scope, tracesv1.Trace{TraceID: "full", StartTime: at, SpanCount: 2},
		tracesv1.SpanDetails{
			SpanID: "r", SpanName: "invoke_agent bot", StartTime: at, Status: &tracesv1.SpanStatus{Code: "Error"},
			Attributes: map[string]any{"gen_ai.conversation.id": "conv-1", "traceloop.entity.input": "hi", "traceloop.entity.output": "hello"},
		},
		tracesv1.SpanDetails{
			SpanID: "c", ParentSpanID: "r", SpanName: "chat gpt-4o", StartTime: at,
			Attributes: map[string]any{"gen_ai.usage.input_tokens": 10, "gen_ai.usage.output_tokens": 5},
		})
	traces.Add(scope, tracesv1.Trace{TraceID: "bare", StartTime: at.Add(time.Second), SpanCount: 1},
		tracesv1.SpanDetails{SpanID: "r", SpanName: "invoke_agent bot", StartTime: at.Add(time.Second)})
	traces.Add(scope, tracesv1.Trace{TraceID: "no-spans", StartTime: at.Add(2 * time.Second)})
	h := handlers{core: fakeCore{}, traces: guarded{traces: traces}, log: slog.New(slog.DiscardHandler)}

	want := map[string]map[string]string{
		"full": {
			"status": `{"errorCount":1}`, "tokenUsage": `{"inputTokens":10,"outputTokens":5,"totalTokens":15}`,
			"conversationId": `"conv-1"`, "input": `"hi"`, "output": `"hello"`,
		},
		"bare":     {"status": `{"errorCount":0}`},
		"no-spans": {},
	}
	for name, serve := range map[string]http.HandlerFunc{"list": h.listTraces, "export": h.exportTraces} {
		r := httptest.NewRequest(http.MethodGet, "/?project=shop&agent=bot&"+testWindow, nil)
		w := httptest.NewRecorder()
		serve(w, r.WithContext(modkit.WithOrgID(r.Context(), "org-a")))
		if w.Code != http.StatusOK {
			t.Fatalf("%s = %d %s, want 200", name, w.Code, w.Body.String())
		}
		got := rawTraces(t, w.Body.Bytes())
		for id, fields := range want {
			sent := map[string]string{}
			for k, v := range got[id] {
				sent[k] = string(v)
			}
			if !reflect.DeepEqual(sent, fields) {
				t.Errorf("%s: %s sent %v, want %v", name, id, sent, fields)
			}
		}
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

// configRuntime answers Config from a map; nothing else is used.
type configRuntime struct {
	modkit.Runtime
	cfg map[string]string
}

func (r configRuntime) Config(key string) string { return r.cfg[key] }
func (configRuntime) Logger() *slog.Logger       { return slog.New(slog.DiscardHandler) }

func TestOpenStores(t *testing.T) {
	traces, cleaner, err := openStores(configRuntime{cfg: map[string]string{}})
	if m, ok := traces.(*memory.Store); err != nil || !ok || cleaner != agentCleaner(m) {
		t.Errorf("no ClickHouse configured: openStores = %T, %T, %v; want the memory store for both", traces, cleaner, err)
	}
	ch := map[string]string{"clickhouse.url": "http://clickhouse:8123", "clickhouse.user": "amp_reader"}
	traces, cleaner, err = openStores(configRuntime{cfg: ch})
	if _, ok := traces.(*clickhouse.Store); err != nil || !ok || cleaner != nil {
		t.Errorf("ClickHouse with no cleaner: openStores = %T, %T, %v; want the ClickHouse store and no cleaner", traces, cleaner, err)
	}
	ch["clickhouse.cleaner-user"] = "amp_cleaner"
	if _, _, err := openStores(configRuntime{cfg: ch}); !errors.Is(err, errHalfCleaner) {
		t.Errorf("cleaner login without a password: openStores = %v, want errHalfCleaner", err)
	}
	ch["clickhouse.cleaner-password"] = "secret"
	traces, cleaner, err = openStores(configRuntime{cfg: ch})
	if _, ok := cleaner.(*clickhouse.Store); err != nil || !ok || cleaner == agentCleaner(traces.(*clickhouse.Store)) {
		t.Errorf("ClickHouse with a cleaner: openStores = %T, %T, %v; want a separate cleaner store", traces, cleaner, err)
	}
	delete(ch, "clickhouse.cleaner-user")
	if _, _, err := openStores(configRuntime{cfg: ch}); !errors.Is(err, errHalfCleaner) {
		t.Errorf("cleaner password without a login: openStores = %v, want errHalfCleaner", err)
	}
	if _, _, err := openStores(configRuntime{cfg: map[string]string{"clickhouse.url": "clickhouse:8123", "clickhouse.user": "amp_reader"}}); err == nil || errors.Is(err, errNoReader) {
		t.Errorf("openStores with a bad URL = %v, want a URL error", err)
	}
	if _, _, err := openStores(configRuntime{cfg: map[string]string{"clickhouse.url": "http://clickhouse:8123"}}); !errors.Is(err, errNoReader) {
		t.Errorf("openStores with no reader login = %v, want errNoReader", err)
	}
}

func TestExternalSchemaConfig(t *testing.T) {
	cfg := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	for _, apply := range []bool{true, false} {
		if _, err := (module{}).ExternalSchema(cfg(nil), apply); !errors.Is(err, modkit.ErrNotConfigured) {
			t.Errorf("apply=%v with no ClickHouse: %v, want ErrNotConfigured", apply, err)
		}
	}
	ch := map[string]string{"clickhouse.url": "http://clickhouse:8123", "clickhouse.user": "amp_reader"}
	if _, err := (module{}).ExternalSchema(cfg(ch), true); !errors.Is(err, modkit.ErrNotConfigured) {
		t.Errorf("migrating with no migrator login: %v, want ErrNotConfigured", err)
	}
	if s, err := (module{}).ExternalSchema(cfg(ch), false); err != nil || s.Store() != "clickhouse" {
		t.Errorf("checking with the reader login = %v, %v; want the ClickHouse schema", s, err)
	}
	ch["clickhouse.migrator-user"] = "amp_migrator"
	if s, err := (module{}).ExternalSchema(cfg(ch), true); err != nil || s.Store() != "clickhouse" {
		t.Errorf("migrating with the migrator login = %v, %v; want the ClickHouse schema", s, err)
	}
	for _, bad := range []string{"0", "-1", "thirty"} {
		ch["clickhouse.ttl-days"] = bad
		for _, apply := range []bool{true, false} {
			_, err := (module{}).ExternalSchema(cfg(ch), apply)
			if err == nil || errors.Is(err, modkit.ErrNotConfigured) || !strings.Contains(err.Error(), "MODULE_OBSERVABILITY_CLICKHOUSE_TTL_DAYS") {
				t.Errorf("ttl-days %q, apply=%v: %v, want a config error naming the setting", bad, apply, err)
			}
		}
	}
}

// recordingCleaner records the agents it was asked to delete.
type recordingCleaner struct {
	deleted []string
	err     error
}

func (c *recordingCleaner) DeleteAgent(_ context.Context, orgID, agentID string) error {
	c.deleted = append(c.deleted, orgID+"/"+agentID)
	return c.err
}

func TestAgentDeletedDeletesThatAgentsSpans(t *testing.T) {
	ctx := context.Background()
	log := slog.New(slog.DiscardHandler)
	ev := func(org, data string) modkit.Event {
		return modkit.Event{ID: "e1", Type: modkit.EventAgentDeleted, OrgID: org, Data: []byte(data)}
	}

	c := &recordingCleaner{}
	h := cleanup{cleaner: c, log: log}
	if err := h.agentDeleted(ctx, ev("org-a", `{"id":"agent-1","orgId":"org-a","name":"bot"}`)); err != nil {
		t.Fatal(err)
	}
	if err := h.agentDeleted(ctx, ev("", `{"id":"agent-2","orgId":"org-b"}`)); err != nil {
		t.Fatal(err)
	}
	if want := []string{"org-a/agent-1", "org-b/agent-2"}; !slices.Equal(c.deleted, want) {
		t.Errorf("deleted %v, want %v", c.deleted, want)
	}

	// An event it can never act on is dropped, not retried.
	for _, data := range []string{`not json`, `{"orgId":"org-a"}`, `{"id":"agent-1"}`} {
		if err := h.agentDeleted(ctx, ev("", data)); err != nil {
			t.Errorf("%s: %v, want nil", data, err)
		}
	}
	if len(c.deleted) != 2 {
		t.Errorf("deleted %v after unusable events", c.deleted)
	}

	// A failed delete is returned, so the event is retried.
	c.err = errors.New("clickhouse down")
	if err := h.agentDeleted(ctx, ev("org-a", `{"id":"agent-1"}`)); err == nil {
		t.Error("a failed delete returned nil, want the error")
	}

	// With no cleaner login, the spans are left to expire.
	if err := (cleanup{log: log}).agentDeleted(ctx, ev("org-a", `{"id":"agent-1"}`)); err != nil {
		t.Errorf("no cleaner: %v, want nil", err)
	}
}

func TestMemoryStoreDeletesOnlyThatAgent(t *testing.T) {
	ctx := context.Background()
	m := memory.New()
	day := time.Date(2026, 11, 1, 1, 0, 0, 0, time.UTC)
	add := func(org, agent, trace string) {
		m.Add(store.Scope{OrgID: org, AgentID: agent, Names: store.Names{Org: org, Agent: agent}},
			tracesv1.Trace{TraceID: trace, StartTime: day})
	}
	add("org-a", "agent-1", "a1")
	add("org-a", "agent-2", "a2")
	add("org-b", "agent-1", "b1")
	if err := (cleanup{cleaner: m, log: slog.New(slog.DiscardHandler)}).agentDeleted(ctx,
		modkit.Event{OrgID: "org-a", Data: []byte(`{"id":"agent-1"}`)}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		org, agent string
		want       int
	}{{"org-a", "agent-1", 0}, {"org-a", "agent-2", 1}, {"org-b", "agent-1", 1}} {
		page, err := m.QueryTraces(ctx, store.Scope{OrgID: tc.org, AgentID: tc.agent, Names: store.Names{Org: tc.org, Agent: tc.agent}}, tracesv1.TraceQuery{})
		if err != nil || page.Total == nil || *page.Total != tc.want {
			t.Errorf("%s/%s: %+v, %v; want %d traces", tc.org, tc.agent, page, err, tc.want)
		}
	}
}
