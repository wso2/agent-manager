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
	"log/slog"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
		id, handle    string
		full, limited http.Handler
	}
	newOrg := func() org {
		id := uuid.Must(uuid.NewV7()).String()
		o := org{id: id, handle: "obs-" + id[len(id)-8:]}
		o.full = api.MakeHTTPHandlerWithModules(params(moduleAuth(o.id, o.handle, tracesReadScope)), nil, nil, host, nil)
		o.limited = api.MakeHTTPHandlerWithModules(params(moduleAuth(o.id, o.handle)), nil, nil, host, nil)
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
		_ = host.Uninstall(context.Background(), observability.Name)
	})

	call := func(h http.Handler, path string) (int, []byte) {
		return moduleWorld{}.call(t, h, http.MethodGet, path, nil)
	}
	// Both organizations have a project and an agent of the same names.
	const project, agent = "obsproj", "obsagent"
	for _, o := range []org{a, b} {
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
	}

	start := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	end := start.Add(2 * time.Second)
	traces.Add(store.Scope{OrgID: a.id, Names: store.Names{Org: a.handle, Project: project, Agent: agent}},
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
		return "/api/v1/m/observability/api/v1/traces?" + q.Encode()
	}
	spansPath := "/api/v1/m/observability/api/v1/traces/trace-a/spans?" + window.Encode() + "&project=" + project + "&agent=" + agent
	spanPath := "/api/v1/m/observability/api/v1/traces/trace-a/spans/llm"

	t.Run("routes are not found before install", func(t *testing.T) {
		for _, p := range []string{listPath(nil), spansPath, spanPath} {
			code, _ := call(a.full, p)
			assert.Equal(t, http.StatusNotFound, code, p)
		}
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
	})

	t.Run("routes need traces-read", func(t *testing.T) {
		for _, p := range []string{listPath(nil), spansPath, spanPath} {
			code, _ := call(a.limited, p)
			assert.Equal(t, http.StatusForbidden, code, p)
		}
	})

	t.Run("another organization sees nothing", func(t *testing.T) {
		// The organization in the query is ignored, as is the observer's.
		code, body := call(b.full, listPath(url.Values{"organization": {a.handle}}))
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

	t.Run("disabling the module removes its routes", func(t *testing.T) {
		require.NoError(t, host.Disable(ctx, observability.Name))
		for _, p := range []string{listPath(nil), spansPath, spanPath} {
			code, _ := call(a.full, p)
			assert.Equal(t, http.StatusNotFound, code, p)
		}
	})
}

func mustField(t *testing.T, body []byte, field string) json.RawMessage {
	t.Helper()
	var m map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &m))
	return m[field]
}
