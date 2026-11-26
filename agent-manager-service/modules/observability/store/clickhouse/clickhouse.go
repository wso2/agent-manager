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

// Package clickhouse is a store.TraceStore over the span table the OTel
// collector's clickhouse exporter writes, read through ClickHouse's HTTP
// interface. Every query filters on the table's org_id column.
package clickhouse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
)

// Table is the span table, the exporter's default traces_table_name.
const Table = "otel_traces"

// DefaultDatabase is the database the table is in when none is configured.
const DefaultDatabase = "otel"

// Resource attributes the collector sets from the gateway's headers; the
// table materializes them as org_id, project_id and agent_id.
const (
	AttrOrgID         = "agent-manager/org-id"
	AttrProjectID     = "agent-manager/project-uid"
	AttrAgentID       = "agent-manager/agent-uid"
	AttrEnvironmentID = "agent-manager/environment-uid"
)

// ErrMissingAgent is returned by DeleteAgent when it is not given an agent.
var ErrMissingAgent = errors.New("clickhouse: no agent to delete")

// Config locates the span table and the user that reads it.
type Config struct {
	URL      string // HTTP interface, e.g. http://clickhouse:8123
	Database string // DefaultDatabase when empty
	User     string
	Password string
}

// Store reads, and deletes, spans of one organization at a time.
type Store struct {
	c *client
}

var _ store.TraceStore = (*Store)(nil)

var identifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// New returns a Store for cfg. It does not connect.
func New(cfg Config) (*Store, error) {
	if cfg.Database == "" {
		cfg.Database = DefaultDatabase
	}
	if !identifier.MatchString(cfg.Database) {
		return nil, fmt.Errorf("clickhouse: invalid database name %q", cfg.Database)
	}
	c, err := newClient(cfg.URL, cfg.Database, cfg.User, cfg.Password)
	if err != nil {
		return nil, err
	}
	return &Store{c: c}, nil
}

// Ping checks that the span table is readable.
func (s *Store) Ping(ctx context.Context) error {
	return s.c.query(ctx, "SELECT 1 FROM "+Table+" LIMIT 1", nil, func([]byte) error { return nil })
}

// scopeFilter is the WHERE clause for a Scope: always the org, and the
// project, agent and environment when set.
func scopeFilter(sc store.Scope, params map[string]string) string {
	params["org"] = sc.OrgID
	where := "org_id = {org:String}"
	if sc.ProjectID != "" {
		params["project"] = sc.ProjectID
		where += " AND project_id = {project:String}"
	}
	if sc.AgentID != "" {
		params["agent"] = sc.AgentID
		where += " AND agent_id = {agent:String}"
	}
	if sc.EnvironmentID != "" {
		params["env"] = sc.EnvironmentID
		where += " AND ResourceAttributes['" + AttrEnvironmentID + "'] = {env:String}"
	}
	return where
}

// windowFilter narrows to [from, to]; a zero bound is open.
func windowFilter(from, to time.Time, params map[string]string) string {
	where := ""
	if !from.IsZero() {
		params["from"] = strconv.FormatInt(from.UnixNano(), 10)
		where += " AND Timestamp >= fromUnixTimestamp64Nano({from:Int64})"
	}
	if !to.IsZero() {
		params["to"] = strconv.FormatInt(to.UnixNano(), 10)
		where += " AND Timestamp <= fromUnixTimestamp64Nano({to:Int64})"
	}
	return where
}

// limitClause bounds a list; zero or less lists everything.
func limitClause(limit int, params map[string]string) string {
	if limit <= 0 {
		return ""
	}
	params["limit"] = strconv.Itoa(limit)
	return " LIMIT {limit:UInt32}"
}

func direction(sort, fallback string) string {
	if sort == "" {
		sort = fallback
	}
	if sort == "asc" {
		return "ASC"
	}
	return "DESC"
}

// QueryTraces lists the Scope's traces with a span in the window, each
// summarised from all its spans, in one query.
func (s *Store) QueryTraces(ctx context.Context, sc store.Scope, q store.TraceQuery) (*store.TracePage, error) {
	if err := sc.Validate(); err != nil {
		return nil, err
	}
	params := map[string]string{}
	scope := scopeFilter(sc, params)
	where := scope + " AND TraceId IN (SELECT TraceId FROM " + Table + " WHERE " + scope +
		windowFilter(q.StartTime, q.EndTime, params) + ")"
	dir := direction(q.SortOrder, "desc")
	sql := `SELECT
  TraceId AS trace_id,
  count() AS span_count,
  countIf(ParentSpanId = '') > 0 AS has_root,
  if(has_root, argMinIf(SpanId, Timestamp, ParentSpanId = ''), argMin(SpanId, Timestamp)) AS root_span_id,
  if(has_root, argMinIf(SpanName, Timestamp, ParentSpanId = ''), argMin(SpanName, Timestamp)) AS root_span_name,
  if(has_root, argMinIf(SpanKind, Timestamp, ParentSpanId = ''), argMin(SpanKind, Timestamp)) AS root_span_kind,
  toUnixTimestamp64Nano(min(Timestamp)) AS start_ns,
  max(toUnixTimestamp64Nano(Timestamp) + toInt64(Duration)) AS end_ns,
  count() OVER () AS total
FROM ` + Table + `
WHERE ` + where + `
GROUP BY TraceId
ORDER BY start_ns ` + dir + `, trace_id ` + dir + limitClause(q.Limit, params)

	page := &store.TracePage{Traces: []store.Trace{}}
	err := s.c.query(ctx, sql, params, func(line []byte) error {
		var r struct {
			TraceID      string `json:"trace_id"`
			SpanCount    int    `json:"span_count"`
			RootSpanID   string `json:"root_span_id"`
			RootSpanName string `json:"root_span_name"`
			RootSpanKind string `json:"root_span_kind"`
			StartNs      int64  `json:"start_ns"`
			EndNs        int64  `json:"end_ns"`
			Total        int    `json:"total"`
		}
		if err := json.Unmarshal(line, &r); err != nil {
			return err
		}
		page.Total = r.Total
		page.Traces = append(page.Traces, store.Trace{
			TraceID: r.TraceID, TraceName: r.RootSpanName, SpanCount: r.SpanCount,
			RootSpanID: r.RootSpanID, RootSpanName: r.RootSpanName, RootSpanKind: r.RootSpanKind,
			StartTime: time.Unix(0, r.StartNs).UTC(), EndTime: time.Unix(0, r.EndNs).UTC(),
			DurationNs: r.EndNs - r.StartNs,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return page, nil
}

// spanRow is one span as the queries below select it.
type spanRow struct {
	SpanID        string            `json:"SpanId"`
	SpanName      string            `json:"SpanName"`
	ParentSpanID  string            `json:"ParentSpanId"`
	StartNs       int64             `json:"start_ns"`
	Duration      int64             `json:"Duration"`
	SpanKind      string            `json:"SpanKind"`
	StatusCode    string            `json:"StatusCode"`
	StatusMessage string            `json:"StatusMessage"`
	SpanAttrs     map[string]string `json:"SpanAttributes"`
	ResourceAttrs map[string]string `json:"ResourceAttributes"`
	Total         int               `json:"total"`
}

func (r spanRow) details() store.SpanDetails {
	start := time.Unix(0, r.StartNs).UTC()
	d := store.SpanDetails{
		SpanID: r.SpanID, SpanName: r.SpanName, ParentSpanID: r.ParentSpanID,
		StartTime: start, EndTime: start.Add(time.Duration(r.Duration)), DurationNs: r.Duration,
		Kind: r.SpanKind,
	}
	if r.StatusCode != "" {
		d.Status = &store.SpanStatus{Code: r.StatusCode, Message: r.StatusMessage}
	}
	if r.SpanAttrs != nil {
		d.Attributes = toAny(r.SpanAttrs)
	}
	if r.ResourceAttrs != nil {
		d.ResourceAttributes = toAny(r.ResourceAttrs)
	}
	return d
}

const spanColumns = `SpanId, SpanName, ParentSpanId, toUnixTimestamp64Nano(Timestamp) AS start_ns,
  Duration, SpanKind, StatusCode, StatusMessage`

// QueryTraceSpans lists one of the Scope's traces' spans.
func (s *Store) QueryTraceSpans(ctx context.Context, sc store.Scope, traceID string, q store.SpanQuery) (*store.SpanPage, error) {
	if err := sc.Validate(); err != nil {
		return nil, err
	}
	params := map[string]string{"trace": traceID}
	where := scopeFilter(sc, params) + " AND TraceId = {trace:String}" + windowFilter(q.StartTime, q.EndTime, params)
	cols := spanColumns
	if q.IncludeAttributes {
		cols += ", SpanAttributes, ResourceAttributes"
	}
	dir := direction(q.SortOrder, "asc")
	sql := "SELECT " + cols + ", count() OVER () AS total FROM " + Table +
		" WHERE " + where + " ORDER BY Timestamp " + dir + ", SpanId " + dir + limitClause(q.Limit, params)

	page := &store.SpanPage{Spans: []store.Span{}}
	err := s.c.query(ctx, sql, params, func(line []byte) error {
		var r spanRow
		if err := json.Unmarshal(line, &r); err != nil {
			return err
		}
		page.Total = r.Total
		d := r.details()
		page.Spans = append(page.Spans, store.Span(d))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return page, nil
}

// GetSpanDetails reads one span of the Scope's trace.
func (s *Store) GetSpanDetails(ctx context.Context, sc store.Scope, traceID, spanID string) (*store.SpanDetails, error) {
	if err := sc.Validate(); err != nil {
		return nil, err
	}
	params := map[string]string{"trace": traceID, "span": spanID}
	where := scopeFilter(sc, params) + " AND TraceId = {trace:String} AND SpanId = {span:String}"
	sql := "SELECT " + spanColumns + ", SpanAttributes, ResourceAttributes FROM " + Table +
		" WHERE " + where + " LIMIT 1"

	var found *store.SpanDetails
	err := s.c.query(ctx, sql, params, func(line []byte) error {
		var r spanRow
		if err := json.Unmarshal(line, &r); err != nil {
			return err
		}
		d := r.details()
		found = &d
		return nil
	})
	if err != nil {
		return nil, err
	}
	if found == nil {
		return nil, store.ErrNotFound
	}
	return found, nil
}

// DeleteAgent deletes an agent's spans in an organization. Deleting spans
// that are already gone is not an error, so it is safe to retry.
func (s *Store) DeleteAgent(ctx context.Context, orgID, agentID string) error {
	if strings.TrimSpace(orgID) == "" {
		return store.ErrMissingOrg
	}
	if strings.TrimSpace(agentID) == "" {
		return ErrMissingAgent
	}
	return s.c.exec(ctx, "DELETE FROM "+Table+" WHERE org_id = {org:String} AND agent_id = {agent:String}",
		map[string]string{"org": orgID, "agent": agentID})
}

func toAny(m map[string]string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}
