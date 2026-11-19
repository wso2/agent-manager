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

// Package observability is an installable module that serves an
// organization's traces: the observer's trace routes under
// /m/observability, and the traces.query/v1 capability.
package observability

import (
	"context"
	_ "embed"
	"io/fs"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/tracesv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store/clickhouse"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store/memory"
)

const (
	// Name is the module's name.
	Name = "observability"
	// PermTracesRead lets a caller read the organization's traces.
	PermTracesRead = "traces-read"
)

func init() {
	modkit.Register(module{})
}

//go:embed api/openapi.yaml
var openAPI []byte

// New returns the module serving traces from traces; nil picks the store from
// config each time the module activates.
func New(traces store.TraceStore) modkit.Module {
	return module{traces: traces}
}

type module struct {
	traces store.TraceStore
}

func (module) Manifest() modkit.Manifest {
	return modkit.Manifest{
		Name:        Name,
		DisplayName: "Observability",
		Version:     "1.0.0",
		Description: "Traces of the organization's agents",
		CoreAPI:     modkit.CoreAPIVersion,
		Provides:    []string{tracesv1.ID},
		Permissions: []modkit.Permission{
			{Name: PermTracesRead, DisplayName: "Read Traces", Description: "Read the organization's traces"},
		},
		RoleGrants: map[string][]string{
			"Agent Manager Admin": {PermTracesRead},
			"Developer":           {PermTracesRead},
		},
		UI: &modkit.UI{Pages: []modkit.Page{
			{ID: "ai-observability/traces", Permission: PermTracesRead},
		}},
	}
}

func (module) Migrations() fs.FS { return nil }
func (module) UI() fs.FS         { return nil }

// OpenAPI describes the routes the module mounts.
func (module) OpenAPI() []byte { return openAPI }

func (m module) Activate(_ context.Context, rt modkit.Runtime) error {
	traces := m.traces
	if traces == nil {
		var err error
		if traces, err = openStore(rt); err != nil {
			return err
		}
	}
	q := guarded{traces: traces}
	rt.Provide(tracesv1.ID, q)
	h := handlers{core: rt.Core(), traces: q, log: rt.Logger()}
	rt.Handle(http.MethodGet, "/api/v1/traces", PermTracesRead, h.listTraces)
	rt.Handle(http.MethodGet, "/api/v1/traces/{traceId}/spans", PermTracesRead, h.listSpans)
	rt.Handle(http.MethodGet, "/api/v1/traces/{traceId}/spans/{spanId}", PermTracesRead, h.getSpan)
	return nil
}

// openStore returns the ClickHouse store when clickhouse.url is set, else an
// empty in-memory store. It does not connect.
func openStore(rt modkit.Runtime) (store.TraceStore, error) {
	url := rt.Config("clickhouse.url")
	if url == "" {
		rt.Logger().Warn("observability: no ClickHouse configured (MODULE_OBSERVABILITY_CLICKHOUSE_URL); serving traces from an empty in-memory store")
		return memory.New(), nil
	}
	s, err := clickhouse.New(clickhouse.Config{
		URL:      url,
		Database: rt.Config("clickhouse.database"),
		User:     rt.Config("clickhouse.user"),
		Password: rt.Config("clickhouse.password"),
	})
	if err != nil {
		return nil, err
	}
	rt.Logger().Info("observability: reading traces from ClickHouse", "url", url)
	return s, nil
}

// guarded is the traces.query/v1 implementation: it refuses a Scope of
// another organization than the caller's.
type guarded struct {
	traces store.TraceStore
}

var _ tracesv1.Query = guarded{}

func (g guarded) check(ctx context.Context, s tracesv1.Scope) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.OrgID != modkit.OrgID(ctx) {
		return tracesv1.ErrForeignOrg
	}
	return nil
}

func (g guarded) QueryTraces(ctx context.Context, s tracesv1.Scope, q tracesv1.TraceQuery) (*tracesv1.TracePage, error) {
	if err := g.check(ctx, s); err != nil {
		return nil, err
	}
	return g.traces.QueryTraces(ctx, s, q)
}

func (g guarded) QueryTraceSpans(ctx context.Context, s tracesv1.Scope, traceID string, q tracesv1.SpanQuery) (*tracesv1.SpanPage, error) {
	if err := g.check(ctx, s); err != nil {
		return nil, err
	}
	return g.traces.QueryTraceSpans(ctx, s, traceID, q)
}

func (g guarded) GetSpanDetails(ctx context.Context, s tracesv1.Scope, traceID, spanID string) (*tracesv1.SpanDetails, error) {
	if err := g.check(ctx, s); err != nil {
		return nil, err
	}
	return g.traces.GetSpanDetails(ctx, s, traceID, spanID)
}
