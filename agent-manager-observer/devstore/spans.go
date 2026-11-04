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

//go:build devstore

// Package devstore backs the dev observer with SQLite instead of OpenSearch.
package devstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

var ErrSpanNotFound = errors.New("span not found")

const schema = `
CREATE TABLE IF NOT EXISTS spans (
	trace_id TEXT NOT NULL,
	span_id TEXT NOT NULL,
	parent_span_id TEXT NOT NULL,
	name TEXT NOT NULL,
	kind TEXT NOT NULL,
	start_ns INTEGER NOT NULL,
	end_ns INTEGER NOT NULL,
	status_code TEXT NOT NULL,
	status_message TEXT NOT NULL,
	attributes TEXT NOT NULL,
	resource_attributes TEXT NOT NULL,
	namespace TEXT NOT NULL,
	project_uid TEXT NOT NULL,
	component_uid TEXT NOT NULL,
	environment_uid TEXT NOT NULL,
	PRIMARY KEY (trace_id, span_id)
);
CREATE INDEX IF NOT EXISTS spans_scope_start ON spans (component_uid, environment_uid, start_ns);
`

type Span struct {
	TraceID, SpanID, ParentSpanID, Name, Kind, StatusCode, StatusMessage string
	StartNs, EndNs                                                       int64
	Attributes                                                           map[string]any
	ResourceAttributes                                                   map[string]string
	Namespace, ProjectUID, ComponentUID, EnvironmentUID                  string
}

// Scope's empty fields are not filtered on.
type Scope struct {
	Namespace, ProjectUID, ComponentUID, EnvironmentUID string
}

type Window struct {
	StartNs, EndNs int64
	Limit          int
	Desc           bool
}

type SpanStore struct {
	db *sql.DB
}

func OpenSpanStore(path string) (*SpanStore, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("create span store directory: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=synchronous(OFF)")
	if err != nil {
		return nil, fmt.Errorf("open span store: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create span schema: %w", err)
	}
	return &SpanStore{db: db}, nil
}

func (s *SpanStore) Close() error { return s.db.Close() }

func (s *SpanStore) Insert(ctx context.Context, spans []Span) (err error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin insert: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	stmt, err := tx.PrepareContext(ctx, `INSERT OR REPLACE INTO spans
		(trace_id, span_id, parent_span_id, name, kind, start_ns, end_ns, status_code, status_message,
		 attributes, resource_attributes, namespace, project_uid, component_uid, environment_uid)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare insert: %w", err)
	}
	defer func() { _ = stmt.Close() }()
	for _, sp := range spans {
		attrs, err := json.Marshal(orEmpty(sp.Attributes))
		if err != nil {
			return fmt.Errorf("encode attributes: %w", err)
		}
		resAttrs, err := json.Marshal(orEmpty(sp.ResourceAttributes))
		if err != nil {
			return fmt.Errorf("encode resource attributes: %w", err)
		}
		if _, err := stmt.ExecContext(ctx, sp.TraceID, sp.SpanID, sp.ParentSpanID, sp.Name, sp.Kind,
			sp.StartNs, sp.EndNs, sp.StatusCode, sp.StatusMessage, string(attrs), string(resAttrs),
			sp.Namespace, sp.ProjectUID, sp.ComponentUID, sp.EnvironmentUID); err != nil {
			return fmt.Errorf("insert span: %w", err)
		}
	}
	return tx.Commit()
}

func orEmpty[V any](m map[string]V) map[string]V {
	if m == nil {
		return map[string]V{}
	}
	return m
}

type whereClause struct {
	sql  string
	args []any
}

func (w *whereClause) add(cond string, arg any) {
	if w.sql != "" {
		w.sql += " AND "
	}
	w.sql += cond
	w.args = append(w.args, arg)
}

func windowedScope(sc Scope, win Window) whereClause {
	var w whereClause
	for _, f := range []struct{ column, value string }{
		{"namespace", sc.Namespace},
		{"project_uid", sc.ProjectUID},
		{"component_uid", sc.ComponentUID},
		{"environment_uid", sc.EnvironmentUID},
	} {
		if f.value != "" {
			w.add(f.column+" = ?", f.value)
		}
	}
	w.add("start_ns >= ?", win.StartNs)
	w.add("end_ns <= ?", win.EndNs)
	return w
}

func sortDirection(desc bool) string {
	if desc {
		return "DESC"
	}
	return "ASC"
}

func limitOrAll(limit int) int {
	if limit <= 0 {
		return -1
	}
	return limit
}

func (s *SpanStore) QueryTraces(ctx context.Context, sc Scope, win Window) ([]observer.TraceInfo, int, error) {
	where := windowedScope(sc, win)
	dir := sortDirection(win.Desc)
	query := fmt.Sprintf(`
WITH in_window AS (
	SELECT trace_id, span_id, parent_span_id, name, kind, start_ns, end_ns
	FROM spans WHERE %s
), ranked AS (
	SELECT *,
		ROW_NUMBER() OVER (PARTITION BY trace_id ORDER BY (parent_span_id <> ''), start_ns, span_id) AS rn,
		COUNT(*) OVER (PARTITION BY trace_id) AS span_count,
		MIN(start_ns) OVER (PARTITION BY trace_id) AS first_start,
		MAX(end_ns) OVER (PARTITION BY trace_id) AS last_end
	FROM in_window
), rooted AS (
	SELECT * FROM ranked WHERE rn = 1 AND parent_span_id = ''
)
SELECT trace_id, name, kind, span_id, span_count, first_start, last_end, COUNT(*) OVER () AS total
FROM rooted ORDER BY first_start %s, trace_id LIMIT ?`, where.sql, dir)

	rows, err := s.db.QueryContext(ctx, query, append(where.args, limitOrAll(win.Limit))...)
	if err != nil {
		return nil, 0, fmt.Errorf("query traces: %w", err)
	}
	defer func() { _ = rows.Close() }()

	traces := []observer.TraceInfo{}
	total := 0
	for rows.Next() {
		var t observer.TraceInfo
		var startNs, endNs int64
		if err := rows.Scan(&t.TraceID, &t.RootSpanName, &t.RootSpanKind, &t.RootSpanID,
			&t.SpanCount, &startNs, &endNs, &total); err != nil {
			return nil, 0, fmt.Errorf("scan trace: %w", err)
		}
		t.TraceName = t.RootSpanName
		t.StartTime, t.EndTime = time.Unix(0, startNs).UTC(), time.Unix(0, endNs).UTC()
		t.DurationNs = endNs - startNs
		traces = append(traces, t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read traces: %w", err)
	}
	return traces, total, nil
}

func (s *SpanStore) TraceSpans(ctx context.Context, traceID string, sc Scope, win Window, withAttrs bool) ([]observer.SpanInfo, int, error) {
	where := windowedScope(sc, win)
	where.add("trace_id = ?", traceID)
	query := fmt.Sprintf(`
SELECT span_id, name, parent_span_id, start_ns, end_ns, kind, status_code, status_message,
	attributes, resource_attributes, COUNT(*) OVER () AS total
FROM spans WHERE %s ORDER BY start_ns %s, span_id LIMIT ?`, where.sql, sortDirection(win.Desc))

	rows, err := s.db.QueryContext(ctx, query, append(where.args, limitOrAll(win.Limit))...)
	if err != nil {
		return nil, 0, fmt.Errorf("query spans: %w", err)
	}
	defer func() { _ = rows.Close() }()

	spans := []observer.SpanInfo{}
	total := 0
	for rows.Next() {
		var sp observer.SpanInfo
		var startNs, endNs int64
		var statusCode, statusMessage, attrs, resAttrs string
		if err := rows.Scan(&sp.SpanID, &sp.SpanName, &sp.ParentSpanID, &startNs, &endNs, &sp.Kind,
			&statusCode, &statusMessage, &attrs, &resAttrs, &total); err != nil {
			return nil, 0, fmt.Errorf("scan span: %w", err)
		}
		sp.StartTime, sp.EndTime = time.Unix(0, startNs).UTC(), time.Unix(0, endNs).UTC()
		sp.DurationNs = endNs - startNs
		sp.Status = spanStatus(statusCode, statusMessage)
		if withAttrs {
			if sp.Attributes, sp.ResourceAttributes, err = decodeAttributes(attrs, resAttrs); err != nil {
				return nil, 0, err
			}
		}
		spans = append(spans, sp)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("read spans: %w", err)
	}
	return spans, total, nil
}

func (s *SpanStore) SpanDetails(ctx context.Context, traceID, spanID string) (*observer.SpanDetailsResponse, error) {
	var d observer.SpanDetailsResponse
	var startNs, endNs int64
	var statusCode, statusMessage, attrs, resAttrs string
	err := s.db.QueryRowContext(ctx, `
SELECT name, parent_span_id, start_ns, end_ns, kind, status_code, status_message, attributes, resource_attributes
FROM spans WHERE trace_id = ? AND span_id = ?`, traceID, spanID).
		Scan(&d.SpanName, &d.ParentSpanID, &startNs, &endNs, &d.Kind, &statusCode, &statusMessage, &attrs, &resAttrs)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrSpanNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("query span details: %w", err)
	}
	d.SpanID = spanID
	d.StartTime, d.EndTime = time.Unix(0, startNs).UTC(), time.Unix(0, endNs).UTC()
	d.DurationNs = endNs - startNs
	d.Status = spanStatus(statusCode, statusMessage)
	if d.Attributes, d.ResourceAttributes, err = decodeAttributes(attrs, resAttrs); err != nil {
		return nil, err
	}
	return &d, nil
}

func spanStatus(code, message string) *observer.SpanStatus {
	if code == "" {
		code = "unset"
	}
	return &observer.SpanStatus{Code: code, Message: message}
}

func decodeAttributes(attrs, resAttrs string) (map[string]interface{}, map[string]interface{}, error) {
	var a, r map[string]interface{}
	if err := json.NewDecoder(strings.NewReader(attrs)).Decode(&a); err != nil {
		return nil, nil, fmt.Errorf("decode attributes: %w", err)
	}
	if err := json.NewDecoder(strings.NewReader(resAttrs)).Decode(&r); err != nil {
		return nil, nil, fmt.Errorf("decode resource attributes: %w", err)
	}
	return a, r, nil
}
