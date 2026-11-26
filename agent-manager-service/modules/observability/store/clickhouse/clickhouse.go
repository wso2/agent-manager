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
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
)

// Table is the span table, the exporter's default traces_table_name.
const Table = "otel_traces"

// DefaultDatabase is the database the table is in unless configured, the one users.xml grants on.
const DefaultDatabase = "otel"

// Resource attributes on each span; the table materializes org_id,
// project_id and agent_id from the agent-manager/* ones.
const (
	AttrOrgID         = "agent-manager/org-id"
	AttrProjectID     = "agent-manager/project-uid"
	AttrAgentID       = "agent-manager/agent-uid"
	AttrEnvironmentID = "agent-manager/environment-uid"
)

// ErrMissingAgent is returned by DeleteAgent when it is not given an agent.
var ErrMissingAgent = errors.New("clickhouse: no agent to delete")

// Config locates ClickHouse and the user that reads the span table.
type Config struct {
	URL      string // HTTP interface, e.g. http://clickhouse:8123
	Database string // "" is DefaultDatabase
	User     string
	Password string
}

// Store reads, and deletes, spans of one organization at a time.
type Store struct {
	c *client
}

var _ store.TraceStore = (*Store)(nil)

// New returns a Store for cfg. It does not connect.
func New(cfg Config) (*Store, error) {
	c, err := newClient(cfg.URL, databaseOr(cfg.Database), cfg.User, cfg.Password)
	if err != nil {
		return nil, err
	}
	return &Store{c: c}, nil
}

// databaseOr is db, or DefaultDatabase when db is "".
func databaseOr(db string) string {
	if db == "" {
		return DefaultDatabase
	}
	return db
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

// The range of a stored Timestamp, nanoseconds since the epoch in an Int64.
var (
	minStored = time.Unix(0, math.MinInt64)
	maxStored = time.Unix(0, math.MaxInt64)
)

// windowFilter narrows to [from, to]. A zero bound, or one no stored span is
// outside of, is open; empty reports a window no stored span can be in.
func windowFilter(from, to time.Time, params map[string]string) (where string, empty bool) {
	return timeFilter(from, to, "from", "to", params)
}

// timeFilter is windowFilter with the bounds bound as fromKey and toKey.
func timeFilter(from, to time.Time, fromKey, toKey string, params map[string]string) (where string, empty bool) {
	if (!from.IsZero() && from.After(maxStored)) || (!to.IsZero() && to.Before(minStored)) {
		return "", true
	}
	if !from.IsZero() && from.After(minStored) {
		params[fromKey] = strconv.FormatInt(from.UnixNano(), 10)
		where += " AND Timestamp >= fromUnixTimestamp64Nano({" + fromKey + ":Int64})"
	}
	if !to.IsZero() && to.Before(maxStored) {
		params[toKey] = strconv.FormatInt(to.UnixNano(), 10)
		where += " AND Timestamp <= fromUnixTimestamp64Nano({" + toKey + ":Int64})"
	}
	return where, false
}

// spanWindow narrows to store.TraceBounds of a trace list's window, where the
// list counts its traces' spans; empty reports a window no trace is listed in.
func spanWindow(from, to time.Time, params map[string]string) (where string, empty bool) {
	if _, empty := windowFilter(from, to, map[string]string{}); empty {
		return "", true
	}
	// Widening a window that is not empty never empties it.
	from, to = store.TraceBounds(from, to)
	return windowFilter(from, to, params)
}

// arrayParam is vs as an Array(String) query parameter.
func arrayParam(vs []string) string {
	quote := strings.NewReplacer(`\`, `\\`, `'`, `\'`)
	var b strings.Builder
	b.WriteByte('[')
	for i, v := range vs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString("'" + quote.Replace(v) + "'")
	}
	b.WriteByte(']')
	return b.String()
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

// cursorTag starts this store's cursors: the backend and the cursor version.
const cursorTag = "ch1"

// QueryTraces lists the Scope's traces whose root span starts in the window
// and that pass the filters, one page at a time, each summarised from its
// spans within store.TraceBounds of the window. The filters and the total run
// in one GROUP BY; the page starts after the cursor.
func (s *Store) QueryTraces(ctx context.Context, sc store.Scope, q store.TraceQuery) (*store.TracePage, error) {
	if err := sc.Validate(); err != nil {
		return nil, err
	}
	order := "desc"
	if q.SortOrder == "asc" {
		order = "asc"
	}
	params := map[string]string{}
	page := ""
	if q.Cursor != "" {
		k, err := store.DecodeCursor(cursorTag, q.Cursor, order)
		if err != nil {
			return nil, err
		}
		params["after_ns"], params["after_id"] = strconv.FormatInt(k.StartNs, 10), k.TraceID
		cmp := "<"
		if order == "asc" {
			cmp = ">"
		}
		page = "\nWHERE (start_ns, trace_id) " + cmp + " ({after_ns:Int64}, {after_id:String})"
	}
	scope := scopeFilter(sc, params)
	window, empty := windowFilter(q.StartTime, q.EndTime, params)
	if empty {
		out := &store.TracePage{Traces: []store.Trace{}}
		if q.Cursor == "" {
			zero := 0
			out.Total = &zero
		}
		return out, nil
	}
	// Widening a window that is not empty never empties it.
	traceFrom, traceTo := store.TraceBounds(q.StartTime, q.EndTime)
	spread, _ := timeFilter(traceFrom, traceTo, "trace_from", "trace_to", params)
	where := scope + spread + " AND TraceId IN (SELECT TraceId FROM " + Table + " WHERE " + scope +
		" AND ParentSpanId = ''" + window + ")"
	columns, having := traceFilters(q.Filters, params)
	havingClause := ""
	if len(having) > 0 {
		havingClause = "\nHAVING " + strings.Join(having, "\n  AND ")
	}
	extra := ""
	for _, c := range columns {
		extra += "  " + c + ",\n"
	}
	dir := direction(order, "desc")
	sql := `SELECT trace_id, span_count, root_span_id, root_span_name, root_span_kind, start_ns, end_ns, total
FROM (
SELECT
  TraceId AS trace_id,
  count() AS span_count,
  countIf(ParentSpanId = '') > 0 AS has_root,
  if(has_root, argMinIf(SpanId, Timestamp, ParentSpanId = ''), argMin(SpanId, Timestamp)) AS root_span_id,
  if(has_root, argMinIf(SpanName, Timestamp, ParentSpanId = ''), argMin(SpanName, Timestamp)) AS root_span_name,
  if(has_root, argMinIf(SpanKind, Timestamp, ParentSpanId = ''), argMin(SpanKind, Timestamp)) AS root_span_kind,
  toUnixTimestamp64Nano(min(Timestamp)) AS start_ns,
  max(toUnixTimestamp64Nano(Timestamp) + toInt64(Duration)) AS end_ns,
` + extra + `  count() OVER () AS total
FROM ` + Table + `
WHERE ` + where + `
GROUP BY TraceId` + havingClause + `
)` + page + `
ORDER BY start_ns ` + dir + `, trace_id ` + dir + limitClause(nextLimit(q.Limit), params)

	out := &store.TracePage{Traces: []store.Trace{}}
	var total *int
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
		total = &r.Total
		out.Traces = append(out.Traces, store.Trace{
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
	// With no row, the total is known only on the first page.
	if total == nil && q.Cursor == "" {
		zero := 0
		total = &zero
	}
	out.Total = total
	if q.Limit > 0 && len(out.Traces) > q.Limit {
		out.Traces = out.Traces[:q.Limit]
		last := out.Traces[q.Limit-1]
		out.NextCursor = store.EncodeCursor(cursorTag, store.CursorKey{
			Order: order, StartNs: last.StartTime.UnixNano(), TraceID: last.TraceID,
		})
	}
	return out, nil
}

// nextLimit is one row past the page, to tell whether another page follows.
func nextLimit(limit int) int {
	if limit <= 0 {
		return 0
	}
	return limit + 1
}

// spanRow is one span as the queries below select it.
type spanRow struct {
	TraceID       string            `json:"TraceId"`
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

// QueryTraceSpans lists one of the Scope's traces' spans, within
// store.TraceBounds of the window as QueryTraces counts them.
func (s *Store) QueryTraceSpans(ctx context.Context, sc store.Scope, traceID string, q store.SpanQuery) (*store.SpanPage, error) {
	if err := sc.Validate(); err != nil {
		return nil, err
	}
	params := map[string]string{"trace": traceID}
	window, empty := spanWindow(q.StartTime, q.EndTime, params)
	if empty {
		return &store.SpanPage{Spans: []store.Span{}}, nil
	}
	where := scopeFilter(sc, params) + " AND TraceId = {trace:String}" + window
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

// QuerySpansByTrace lists several of the Scope's traces' spans in one query,
// at most q.Limit each, within store.TraceBounds of the window.
func (s *Store) QuerySpansByTrace(ctx context.Context, sc store.Scope, traceIDs []string, q store.SpanQuery) (map[string]*store.SpanPage, error) {
	if err := sc.Validate(); err != nil {
		return nil, err
	}
	pages := map[string]*store.SpanPage{}
	if len(traceIDs) == 0 {
		return pages, nil
	}
	params := map[string]string{"traces": arrayParam(traceIDs)}
	window, empty := spanWindow(q.StartTime, q.EndTime, params)
	if empty {
		return pages, nil
	}
	where := scopeFilter(sc, params) + " AND TraceId IN {traces:Array(String)}" + window
	cols := "TraceId, " + spanColumns
	if q.IncludeAttributes {
		cols += ", SpanAttributes, ResourceAttributes"
	}
	dir := direction(q.SortOrder, "asc")
	limit := ""
	if q.Limit > 0 {
		params["limit"] = strconv.Itoa(q.Limit)
		limit = " LIMIT {limit:UInt32} BY TraceId"
	}
	sql := "SELECT " + cols + ", count() OVER (PARTITION BY TraceId) AS total FROM " + Table +
		" WHERE " + where + " ORDER BY TraceId, Timestamp " + dir + ", SpanId " + dir + limit

	err := s.c.query(ctx, sql, params, func(line []byte) error {
		var r spanRow
		if err := json.Unmarshal(line, &r); err != nil {
			return err
		}
		page := pages[r.TraceID]
		if page == nil {
			page = &store.SpanPage{Spans: []store.Span{}, Total: r.Total}
			pages[r.TraceID] = page
		}
		page.Spans = append(page.Spans, store.Span(r.details()))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return pages, nil
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
