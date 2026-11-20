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
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/tracesv1"
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

func (fakeCore) GetProject(context.Context, string, string) (modkit.Project, error) {
	return modkit.Project{}, modkit.ErrNotFound
}

func (fakeCore) ListAgents(context.Context, string, string) ([]modkit.Agent, error) {
	return nil, nil
}

func (fakeCore) GetAgent(context.Context, string, string) (modkit.Agent, error) {
	return modkit.Agent{}, modkit.ErrNotFound
}

func (fakeCore) ListEnvironments(context.Context, string) ([]modkit.Environment, error) {
	return nil, nil
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
	return modkit.Agent{ID: "a1", OrgID: orgID, ProjectID: "p1", Name: name}, nil
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
	}
}

func TestUnencodableSpanIsA500(t *testing.T) {
	traces := memory.New()
	traces.Add(tracesv1.Scope{OrgID: "org-a", Names: tracesv1.Names{Org: "acme"}},
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
	q := guarded{traces: memory.New()}
	ctx := modkit.WithOrgID(context.Background(), "org-a")
	foreign := tracesv1.Scope{OrgID: "org-b", Names: tracesv1.Names{Org: "globex"}}
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
	own := tracesv1.Scope{OrgID: "org-a", Names: tracesv1.Names{Org: "acme"}}
	if _, err := q.QueryTraces(ctx, own, tracesv1.TraceQuery{}); err != nil {
		t.Errorf("QueryTraces of the caller's org = %v, want nil", err)
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
	traces, cleaner, err = openStores(configRuntime{cfg: ch})
	if _, ok := cleaner.(*clickhouse.Store); err != nil || !ok || cleaner == agentCleaner(traces.(*clickhouse.Store)) {
		t.Errorf("ClickHouse with a cleaner: openStores = %T, %T, %v; want a separate cleaner store", traces, cleaner, err)
	}
	if _, _, err := openStores(configRuntime{cfg: map[string]string{"clickhouse.url": "clickhouse:8123"}}); err == nil {
		t.Error("openStores with a bad URL succeeded, want an error")
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
		if _, err := (module{}).ExternalSchema(cfg(ch), true); err == nil || errors.Is(err, modkit.ErrNotConfigured) {
			t.Errorf("ttl-days %q: %v, want a config error", bad, err)
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
		m.Add(tracesv1.Scope{OrgID: org, AgentID: agent, Names: tracesv1.Names{Org: org, Agent: agent}},
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
		page, err := m.QueryTraces(ctx, tracesv1.Scope{OrgID: tc.org, AgentID: tc.agent, Names: tracesv1.Names{Org: tc.org, Agent: tc.agent}}, tracesv1.TraceQuery{})
		if err != nil || page.Total != tc.want {
			t.Errorf("%s/%s: %d traces, %v; want %d", tc.org, tc.agent, page.Total, err, tc.want)
		}
	}
}
