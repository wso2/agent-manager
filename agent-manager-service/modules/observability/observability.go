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
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"slices"

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
	// TracesRead is PermTracesRead's full name, as the host names it.
	TracesRead = "observability:traces-read"
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
			"AI Lead":             {PermTracesRead},
			"Platform Engineer":   {PermTracesRead},
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
	g := guarded{traces: traces}
	rt.Provide(tracesv1.ID, capability{core: rt.Core(), traces: g})
	h := handlers{core: rt.Core(), traces: g, log: rt.Logger()}
	rt.Handle(http.MethodGet, "/api/v1/traces", PermTracesRead, h.listTraces)
	rt.Handle(http.MethodGet, "/api/v1/traces/export", PermTracesRead, h.exportTraces)
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

// guarded is the store behind the routes and the capability: it refuses a
// Scope of another organization than the caller's.
type guarded struct {
	traces store.TraceStore
}

var _ store.TraceStore = guarded{}

func (g guarded) check(ctx context.Context, s store.Scope) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.OrgID != modkit.OrgID(ctx) {
		return tracesv1.ErrForeignOrg
	}
	return nil
}

func (g guarded) QueryTraces(ctx context.Context, s store.Scope, q store.TraceQuery) (*store.TracePage, error) {
	if err := g.check(ctx, s); err != nil {
		return nil, err
	}
	return g.traces.QueryTraces(ctx, s, q)
}

func (g guarded) QueryTraceSpans(ctx context.Context, s store.Scope, traceID string, q store.SpanQuery) (*store.SpanPage, error) {
	if err := g.check(ctx, s); err != nil {
		return nil, err
	}
	return g.traces.QueryTraceSpans(ctx, s, traceID, q)
}

func (g guarded) GetSpanDetails(ctx context.Context, s store.Scope, traceID, spanID string) (*store.SpanDetails, error) {
	if err := g.check(ctx, s); err != nil {
		return nil, err
	}
	return g.traces.GetSpanDetails(ctx, s, traceID, spanID)
}

// capability is the traces.query/v1 implementation: it looks up the names of
// a Scope's IDs and reads through guarded.
type capability struct {
	core   modkit.CoreAPI
	traces guarded
}

var _ tracesv1.Query = capability{}

// resolve returns the store Scope of s, with the names of its IDs.
func (c capability) resolve(ctx context.Context, s tracesv1.Scope) (store.Scope, error) {
	if err := s.Validate(); err != nil {
		return store.Scope{}, err
	}
	if s.OrgID != modkit.OrgID(ctx) {
		return store.Scope{}, tracesv1.ErrForeignOrg
	}
	org, err := c.core.GetOrg(ctx, s.OrgID)
	if err != nil {
		return store.Scope{}, lookupErr("organization", err)
	}
	r := store.Scope{
		OrgID: s.OrgID, ProjectID: s.ProjectID, AgentID: s.AgentID, EnvironmentID: s.EnvironmentID,
		Names: store.Names{Org: org.Handle},
	}
	switch {
	case s.AgentID != "":
		a, err := c.core.GetAgent(ctx, s.OrgID, s.AgentID)
		if err != nil {
			return store.Scope{}, lookupErr("agent", err)
		}
		if s.ProjectID != "" && a.ProjectID != s.ProjectID {
			return store.Scope{}, tracesv1.ErrNotFound
		}
		r.ProjectID, r.Names.Project, r.Names.Agent = a.ProjectID, a.ProjectName, a.Name
	case s.ProjectID != "":
		p, err := c.core.GetProject(ctx, s.OrgID, s.ProjectID)
		if err != nil {
			return store.Scope{}, lookupErr("project", err)
		}
		r.Names.Project = p.Name
	}
	if s.EnvironmentID != "" {
		envs, err := c.core.ListEnvironments(ctx, s.OrgID)
		if err != nil {
			return store.Scope{}, lookupErr("environments", err)
		}
		i := slices.IndexFunc(envs, func(e modkit.Environment) bool { return e.ID == s.EnvironmentID })
		if i < 0 {
			return store.Scope{}, tracesv1.ErrNotFound
		}
		r.Names.Environment = envs[i].Name
	}
	return r, nil
}

// lookupErr maps the core's ErrNotFound to the capability's.
func lookupErr(what string, err error) error {
	if errors.Is(err, modkit.ErrNotFound) {
		return fmt.Errorf("%s: %w", what, tracesv1.ErrNotFound)
	}
	return fmt.Errorf("get %s: %w", what, err)
}

func (c capability) QueryTraces(ctx context.Context, s tracesv1.Scope, q tracesv1.TraceQuery) (*tracesv1.TracePage, error) {
	r, err := c.resolve(ctx, s)
	if err != nil {
		return nil, err
	}
	return c.traces.QueryTraces(ctx, r, q)
}

func (c capability) QueryTraceSpans(ctx context.Context, s tracesv1.Scope, traceID string, q tracesv1.SpanQuery) (*tracesv1.SpanPage, error) {
	r, err := c.resolve(ctx, s)
	if err != nil {
		return nil, err
	}
	return c.traces.QueryTraceSpans(ctx, r, traceID, q)
}

func (c capability) GetSpanDetails(ctx context.Context, s tracesv1.Scope, traceID, spanID string) (*tracesv1.SpanDetails, error) {
	r, err := c.resolve(ctx, s)
	if err != nil {
		return nil, err
	}
	return c.traces.GetSpanDetails(ctx, r, traceID, spanID)
}
