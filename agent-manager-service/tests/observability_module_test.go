//go:build integration

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

package tests

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/api"
	"github.com/wso2/agent-manager/agent-manager-service/app"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/internal/modhost"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store/memory"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

const tracesReadScope = "amp:observability:traces-read"

// TestObservabilityModule drives the observability module over the HTTP API:
// install and disable, its permission, and organization isolation.
func TestObservabilityModule(t *testing.T) {
	// The collector the module forwards agents' exports to.
	type export struct {
		header http.Header
		body   []byte
	}
	exports := make(chan export, 16)
	collector := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		exports <- export{header: r.Header.Clone(), body: body}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer collector.Close()
	t.Setenv("MODULE_OBSERVABILITY_COLLECTOR_URL", collector.URL)

	ctx, cancel := context.WithCancel(context.Background())
	database := db.DB(context.Background())
	traces := memory.New()
	host := modhost.NewWith(database, config.GetConfig().Database.Driver, slog.New(slog.DiscardHandler),
		[]modkit.Module{observability.New(traces)})
	host.PollInterval = 50 * time.Millisecond
	_, err := host.Migrate(ctx)
	require.NoError(t, err)

	params := func(auth jwtassertion.Middleware) *wiring.AppParams {
		p, err := wiring.InitializeTestAppParamsWithClientMocks(config.GetConfig(), database, auth,
			wiring.TestClients{PlatformRepository: repositories.NewPlatformRepo(database)})
		require.NoError(t, err)
		return p
	}
	type org struct {
		id, handle         string
		projectID, agentID string
		full, limited      http.Handler
	}
	newOrg := func() org {
		id := uuid.Must(uuid.NewV7()).String()
		o := org{id: id, handle: "obs-" + id[len(id)-8:]}
		o.full = api.MakeHTTPHandlerWithModules(params(moduleAuth(o.id, o.handle, tracesReadScope)), nil, nil, host)
		o.limited = api.MakeHTTPHandlerWithModules(params(moduleAuth(o.id, o.handle)), nil, nil, host)
		return o
	}
	a, b := newOrg(), newOrg()

	stop, err := app.StartModules(ctx, host, database, params(moduleAuth(a.id, a.handle, tracesReadScope)))
	require.NoError(t, err)
	t.Cleanup(func() {
		stop()
		cancel()
		coreext.Set(nil)
		rbac.SetCurrent(rbac.NewRegistry())
		_ = host.Uninstall(context.Background(), observability.Name, false)
	})

	call := func(h http.Handler, path string) (int, []byte) {
		return moduleWorld{}.call(t, h, http.MethodGet, path, nil)
	}
	// Both organizations have a project and an agent of the same names.
	const project, agent = "obsproj", "obsagent"
	for _, o := range []*org{&a, &b} {
		code, body := moduleWorld{}.call(t, o.full, http.MethodPost, "/api/v1/orgs/"+o.handle+"/projects",
			map[string]any{"name": project, "displayName": project})
		require.Equal(t, http.StatusAccepted, code, string(body))
		code, body = moduleWorld{}.call(t, o.full, http.MethodPost, "/api/v1/orgs/"+o.handle+"/projects/"+project+"/agents",
			map[string]any{
				"name": agent, "displayName": agent,
				"provisioning": map[string]any{"type": "external"},
				"agentType":    map[string]any{"type": "external-agent-api"},
			})
		require.Equal(t, http.StatusAccepted, code, string(body))
		o.projectID, o.agentID = resolveIDs(t, database, o.id, project, agent)
	}

	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	end := start.Add(2 * time.Second)
	traces.Add(store.Scope{
		OrgID: a.id, ProjectID: a.projectID, AgentID: a.agentID,
		Names: store.Names{Org: a.handle, Project: project, Agent: agent},
	},
		store.Trace{
			TraceID: "trace-a", SpanCount: 2, RootSpanID: "root", RootSpanName: "invoke_agent support",
			StartTime: start, EndTime: end, DurationNs: end.Sub(start).Nanoseconds(),
		},
		store.SpanDetails{
			SpanID: "root", SpanName: "invoke_agent support", StartTime: start, EndTime: end,
			DurationNs: end.Sub(start).Nanoseconds(), Status: &store.SpanStatus{Code: "Ok"},
			ResourceAttributes: map[string]any{"agent-manager/agent": agent},
		},
		store.SpanDetails{
			SpanID: "llm", ParentSpanID: "root", SpanName: "chat gpt-4o", StartTime: start, EndTime: end,
			Status: &store.SpanStatus{Code: "Error", Message: "rate limited"}, Attributes: map[string]any{"gen_ai.request.model": "gpt-4o"},
		},
	)

	window := url.Values{
		"startTime": {start.Add(-time.Minute).Format(time.RFC3339)},
		"endTime":   {end.Add(time.Minute).Format(time.RFC3339)},
	}
	listPath := func(extra url.Values) string {
		q := url.Values{"project": {project}, "agent": {agent}}
		for k, v := range window {
			q[k] = v
		}
		for k, v := range extra {
			q[k] = v
		}
		return "/m/observability/api/v1/traces?" + q.Encode()
	}
	exportPath := strings.Replace(listPath(nil), "/traces?", "/traces/export?", 1)
	spansPath := "/m/observability/api/v1/traces/trace-a/spans?" + window.Encode() + "&project=" + project + "&agent=" + agent
	spanPath := "/m/observability/api/v1/traces/trace-a/spans/llm"

	// keyOf signs an agent key for o's agent, with over replacing claims.
	keyOf := func(o org, over jwt.MapClaims) string {
		c := jwt.MapClaims{
			"iss": config.GetConfig().JWTSigning.Issuer, "sub": agent, "jti": uuid.NewString(),
			"org_id": o.id, "organization": o.handle, "project_uid": o.projectID, "project": project,
			"component_uid": o.agentID, "agent": agent, "environment_uid": "env-1", "exp": time.Now().Add(time.Hour).Unix(),
		}
		for k, v := range over {
			c[k] = v
		}
		return signLikeTheCore(t, c)
	}
	// A span whose resource claims org B.
	span := map[string]any{"resourceSpans": []any{map[string]any{
		"resource": map[string]any{"attributes": []any{
			map[string]any{"key": "agent-manager/org-id", "value": map[string]any{"stringValue": b.id}},
		}},
	}}}
	ingest := func(h http.Handler, header ...string) (int, []byte) {
		return moduleWorld{}.call(t, h, http.MethodPost, "/m/observability"+observability.IngestPath, span, header...)
	}

	t.Run("routes are not found before install", func(t *testing.T) {
		for _, p := range []string{listPath(nil), exportPath, spansPath, spanPath} {
			code, _ := call(a.full, p)
			assert.Equal(t, http.StatusNotFound, code, p)
		}
		code, _ := ingest(a.full, modkit.AgentKeyHeader, keyOf(a, nil))
		assert.Equal(t, http.StatusNotFound, code, "nor is trace ingest")
	})

	_, err = host.Install(ctx, observability.Name, false)
	require.NoError(t, err)

	t.Run("the caller's organization reads its traces", func(t *testing.T) {
		code, body := call(a.full, listPath(nil))
		require.Equal(t, http.StatusOK, code, string(body))
		var list observability.TraceOverviewResponse
		require.NoError(t, json.Unmarshal(body, &list))
		require.Len(t, list.Traces, 1)
		assert.Equal(t, 1, list.TotalCount)
		assert.Equal(t, "trace-a", list.Traces[0].TraceID)
		assert.Equal(t, "agent", list.Traces[0].RootSpanKind)

		code, body = call(a.full, spansPath)
		require.Equal(t, http.StatusOK, code, string(body))
		var spans observability.SpanListResponse
		require.NoError(t, json.Unmarshal(body, &spans))
		require.Len(t, spans.Spans, 2)
		assert.Equal(t, "llm", spans.Spans[1].SpanKind)
		assert.True(t, spans.Spans[1].Error)

		code, body = call(a.full, spanPath)
		require.Equal(t, http.StatusOK, code, string(body))
		var span observability.Span
		require.NoError(t, json.Unmarshal(body, &span))
		assert.Equal(t, "trace-a", span.TraceID)
		assert.Equal(t, "error", span.Status)
		require.NotNil(t, span.AmpAttributes)
		assert.Equal(t, "llm", span.AmpAttributes.Kind)
		assert.Equal(t, "gpt-4o", span.Attributes["gen_ai.request.model"])

		code, body = call(a.full, exportPath)
		require.Equal(t, http.StatusOK, code, string(body))
		var export observability.TraceExportResponse
		require.NoError(t, json.Unmarshal(body, &export))
		require.Len(t, export.Traces, 1)
		assert.Equal(t, 1, export.TotalCount)
		assert.Equal(t, "trace-a", export.Traces[0].TraceID)
		require.Len(t, export.Traces[0].Spans, 2)
		assert.Equal(t, "gpt-4o", export.Traces[0].Spans[1].Attributes["gen_ai.request.model"])
	})

	t.Run("routes need traces-read", func(t *testing.T) {
		for _, p := range []string{listPath(nil), exportPath, spansPath, spanPath} {
			code, _ := call(a.limited, p)
			assert.Equal(t, http.StatusForbidden, code, p)
		}
	})

	t.Run("another organization sees nothing", func(t *testing.T) {
		// The organization in the query is ignored, as is the observer's.
		code, body := call(b.full, listPath(url.Values{"organization": {a.handle}}))
		require.Equal(t, http.StatusOK, code, string(body))
		assert.JSONEq(t, `[]`, string(mustField(t, body, "traces")))

		code, body = call(b.full, exportPath)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.JSONEq(t, `[]`, string(mustField(t, body, "traces")))

		code, body = call(b.full, spansPath)
		require.Equal(t, http.StatusOK, code, string(body))
		assert.JSONEq(t, `[]`, string(mustField(t, body, "spans")))

		code, _ = call(b.full, spanPath)
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("unknown names are not found", func(t *testing.T) {
		code, _ := call(a.full, listPath(url.Values{"project": {"nosuch"}}))
		assert.Equal(t, http.StatusNotFound, code)
		code, _ = call(a.full, listPath(url.Values{"agent": {"nosuch"}}))
		assert.Equal(t, http.StatusNotFound, code)
	})

	t.Run("an agent's traces go to the collector under its key's org", func(t *testing.T) {
		// Sent through org B's handler, with org B's headers: only the key counts.
		code, body := ingest(b.full, modkit.AgentKeyHeader, keyOf(a, nil),
			"X-Org-Id", b.id, "X-User-Component", b.agentID, "X-User-Agent-Name", "spoofed")
		require.Equal(t, http.StatusOK, code, string(body))
		e := <-exports
		want := map[string]string{
			"X-Org-Id": a.id, "X-User-Project": a.projectID, "X-User-Component": a.agentID, "X-User-Environment": "env-1",
			"X-User-Organization-Name": a.handle, "X-User-Project-Name": project, "X-User-Agent-Name": agent,
		}
		for h, v := range want {
			assert.Equal(t, []string{v}, e.header.Values(h), h)
		}
		assert.Empty(t, e.header.Get(modkit.AgentKeyHeader), "the key never reaches the collector")
		sent, err := json.Marshal(span)
		require.NoError(t, err)
		assert.Equal(t, sent, e.body, "the body is forwarded unchanged; the collector replaces its identity attributes")
	})

	t.Run("trace ingest needs a valid agent key", func(t *testing.T) {
		login := signLikeTheCore(t, jwt.MapClaims{
			"iss": "amp-test-login", "sub": "tester", "ouId": a.id, "ouHandle": a.handle, "exp": time.Now().Add(time.Minute).Unix(),
		})
		refused := map[string][]string{
			"no key":                    nil,
			"a sign-in token":           {"Authorization", "Bearer " + login},
			"a sign-in token as key":    {modkit.AgentKeyHeader, login},
			"an expired key":            {modkit.AgentKeyHeader, keyOf(a, jwt.MapClaims{"exp": time.Now().Add(-time.Minute).Unix()})},
			"another issuer's key":      {modkit.AgentKeyHeader, keyOf(a, jwt.MapClaims{"iss": "someone-else"})},
			"a key for no agent":        {modkit.AgentKeyHeader, keyOf(a, jwt.MapClaims{"component_uid": uuid.NewString()})},
			"a key for another's agent": {modkit.AgentKeyHeader, keyOf(a, jwt.MapClaims{"component_uid": b.agentID})},
		}
		for name, header := range refused {
			code, body := ingest(a.full, header...)
			assert.Equal(t, http.StatusUnauthorized, code, "%s: %s", name, body)
		}
		assert.Empty(t, exports, "nothing refused reaches the collector")
	})

	t.Run("disabling the module removes its routes", func(t *testing.T) {
		require.NoError(t, host.Disable(ctx, observability.Name))
		for _, p := range []string{listPath(nil), exportPath, spansPath, spanPath} {
			code, _ := call(a.full, p)
			assert.Equal(t, http.StatusNotFound, code, p)
		}
		code, _ := ingest(a.full, modkit.AgentKeyHeader, keyOf(a, nil))
		assert.Equal(t, http.StatusNotFound, code, "an agent's export gets 404")
	})
}

// resolveIDs returns the project and agent IDs the module's routes resolve.
func resolveIDs(t *testing.T, database *gorm.DB, orgID, project, agent string) (string, string) {
	t.Helper()
	core := repositories.NewCoreReadAPI(database)
	projects, err := core.ListProjects(context.Background(), orgID)
	require.NoError(t, err)
	for _, p := range projects {
		if p.Name != project {
			continue
		}
		agents, err := core.ListAgents(context.Background(), orgID, p.ID)
		require.NoError(t, err)
		for _, a := range agents {
			if a.Name == agent {
				return p.ID, a.ID
			}
		}
	}
	t.Fatalf("no agent %s/%s in org %s", project, agent, orgID)
	return "", ""
}

func mustField(t *testing.T, body []byte, field string) json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &m))
	return m[field]
}
