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

package clickhouse

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store/storetest"
)

// These tests run against the ClickHouse at CLICKHOUSE_TEST_URL, started
// with users.xml mounted and its passwords set (see the service's CI
// workflow), and skip when it is unset. They own its otel database.
type testEnv struct {
	url                                      string
	admin, migrator, ingest, reader, cleaner *client
}

func clickhouseEnv(tb testing.TB) testEnv {
	tb.Helper()
	url := os.Getenv("CLICKHOUSE_TEST_URL")
	if url == "" {
		tb.Skip("CLICKHOUSE_TEST_URL is not set")
	}
	mk := func(user, password string) *client {
		c, err := newClient(url, DefaultDatabase, user, password)
		if err != nil {
			tb.Fatal(err)
		}
		return c
	}
	e := testEnv{
		url:      url,
		admin:    mk(os.Getenv("CLICKHOUSE_TEST_ADMIN_USER"), os.Getenv("CLICKHOUSE_TEST_ADMIN_PASSWORD")),
		migrator: mk("amp_migrator", os.Getenv("AMP_CLICKHOUSE_MIGRATOR_PASSWORD")),
		ingest:   mk("amp_ingest", os.Getenv("AMP_CLICKHOUSE_INGEST_PASSWORD")),
		reader:   mk("amp_reader", os.Getenv("AMP_CLICKHOUSE_READER_PASSWORD")),
		cleaner:  mk("amp_cleaner", os.Getenv("AMP_CLICKHOUSE_CLEANER_PASSWORD")),
	}
	e.applySchema(tb)
	if err := e.admin.exec(context.Background(), "TRUNCATE TABLE "+Table, nil); err != nil {
		tb.Fatal(err)
	}
	return e
}

// schema returns the Schema as user logs in.
func (e testEnv) schema(tb testing.TB, user, password string, ttlDays int) *Schema {
	tb.Helper()
	s, err := NewSchema(SchemaConfig{URL: e.url, User: user, Password: password, TTLDays: ttlDays}, Migrations())
	if err != nil {
		tb.Fatal(err)
	}
	return s
}

func (e testEnv) migratorSchema(tb testing.TB, ttlDays int) *Schema {
	return e.schema(tb, "amp_migrator", os.Getenv("AMP_CLICKHOUSE_MIGRATOR_PASSWORD"), ttlDays)
}

// applySchema applies the schema files not yet recorded, as amp migrate does.
func (e testEnv) applySchema(tb testing.TB) {
	tb.Helper()
	ctx := context.Background()
	s := e.migratorSchema(tb, 0)
	applied, err := s.Applied(ctx)
	if err != nil {
		tb.Fatal(err)
	}
	files, err := fs.Glob(Migrations(), "*.sql")
	if err != nil {
		tb.Fatal(err)
	}
	for _, f := range files {
		version := strings.TrimSuffix(f, ".sql")
		if _, ok := applied[version]; ok {
			continue
		}
		body, err := fs.ReadFile(Migrations(), f)
		if err != nil {
			tb.Fatal(err)
		}
		sum := sha256.Sum256(body)
		if err := s.Apply(ctx, version, hex.EncodeToString(sum[:]), body); err != nil {
			tb.Fatalf("%s: %v", f, err)
		}
	}
}

func (e testEnv) store(tb testing.TB, user, password string) *Store {
	tb.Helper()
	s, err := New(Config{URL: e.url, User: user, Password: password})
	if err != nil {
		tb.Fatal(err)
	}
	return s
}

func (e testEnv) readerStore(tb testing.TB) *Store {
	return e.store(tb, "amp_reader", os.Getenv("AMP_CLICKHOUSE_READER_PASSWORD"))
}

// spanRecord is a row as the collector's exporter writes it.
type spanRecord struct {
	Timestamp          string            `json:"Timestamp"`
	TraceID            string            `json:"TraceId"`
	SpanID             string            `json:"SpanId"`
	ParentSpanID       string            `json:"ParentSpanId"`
	SpanName           string            `json:"SpanName"`
	SpanKind           string            `json:"SpanKind"`
	ServiceName        string            `json:"ServiceName"`
	ResourceAttributes map[string]string `json:"ResourceAttributes"`
	SpanAttributes     map[string]string `json:"SpanAttributes"`
	Duration           int64             `json:"Duration"`
	StatusCode         string            `json:"StatusCode"`
	StatusMessage      string            `json:"StatusMessage"`
}

func records(seeds []storetest.Seed) []spanRecord {
	var out []spanRecord
	for _, sd := range seeds {
		res := map[string]string{
			AttrOrgID: sd.Scope.OrgID, AttrProjectID: sd.Scope.ProjectID,
			AttrAgentID: sd.Scope.AgentID, AttrEnvironmentID: sd.Scope.EnvironmentID,
			"agent-manager/organization": sd.Scope.Names.Org, "agent-manager/agent": sd.Scope.Names.Agent,
		}
		for _, d := range sd.Spans {
			r := spanRecord{
				Timestamp: d.StartTime.UTC().Format("2006-01-02 15:04:05.000000000"),
				TraceID:   sd.Trace.TraceID, SpanID: d.SpanID, ParentSpanID: d.ParentSpanID,
				SpanName: d.SpanName, SpanKind: d.Kind, ServiceName: sd.Scope.Names.Agent,
				ResourceAttributes: res, SpanAttributes: map[string]string{}, Duration: d.DurationNs,
			}
			for k, v := range d.Attributes {
				r.SpanAttributes[k] = fmt.Sprint(v)
			}
			if d.Status != nil {
				r.StatusCode, r.StatusMessage = d.Status.Code, d.Status.Message
			}
			out = append(out, r)
		}
	}
	return out
}

// insert writes rows as the collector's user.
func (e testEnv) insert(tb testing.TB, rows []spanRecord) {
	tb.Helper()
	var b strings.Builder
	b.WriteString("INSERT INTO " + Table + " FORMAT JSONEachRow\n")
	for _, r := range rows {
		line, err := json.Marshal(r)
		if err != nil {
			tb.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := e.ingest.exec(context.Background(), b.String(), nil); err != nil {
		tb.Fatal(err)
	}
}

func TestStoreSuite(t *testing.T) {
	storetest.Run(t, func(t *testing.T, seeds []storetest.Seed) store.TraceStore {
		e := clickhouseEnv(t)
		e.insert(t, records(seeds))
		return e.readerStore(t)
	})
}

func TestUserGrants(t *testing.T) {
	e := clickhouseEnv(t)
	ctx := context.Background()
	e.insert(t, records([]storetest.Seed{seed("org-a", "agent-1", "trace-1")}))

	refused := func(who string, c *client, sql string) {
		t.Helper()
		var ce *Error
		if err := c.exec(ctx, sql, nil); !errors.As(err, &ce) || !strings.Contains(ce.Message, "Not enough privileges") {
			t.Errorf("%s: %q = %v, want refused for privileges", who, sql, err)
		}
	}
	insert := "INSERT INTO " + Table + " (TraceId, SpanId) VALUES ('x', 'y')"
	for _, sql := range []string{
		insert,
		"DROP TABLE " + Table,
		"TRUNCATE TABLE " + Table,
		"DELETE FROM " + Table + " WHERE 1",
		"ALTER TABLE " + Table + " DELETE WHERE 1",
		"ALTER TABLE " + Table + " ADD COLUMN x String",
		"CREATE TABLE reader_t (x String) ENGINE = Memory",
		"GRANT INSERT ON otel.otel_traces TO amp_reader",
	} {
		refused("reader", e.reader, sql)
	}
	for _, sql := range []string{"SELECT count() FROM " + Table, "DROP TABLE " + Table, "DELETE FROM " + Table + " WHERE 1"} {
		refused("ingest", e.ingest, sql)
	}
	for _, sql := range []string{"SELECT count() FROM " + Table, insert, "DELETE FROM " + Table + " WHERE 1"} {
		refused("migrator", e.migrator, sql)
	}
	for _, sql := range []string{
		"SELECT count() FROM " + Table,
		insert,
		"DROP TABLE " + Table,
		"ALTER TABLE " + Table + " UPDATE SpanName = 'x' WHERE 1",
		"ALTER TABLE " + Table + " MODIFY TTL toDateTime(Timestamp) + toIntervalDay(1)",
	} {
		refused("cleaner", e.cleaner, sql)
	}
	historyInsert := "INSERT INTO " + HistoryTable + " (version, checksum) VALUES ('999_x', 'x')"
	refused("reader", e.reader, historyInsert)
	refused("reader", e.reader, "DROP TABLE "+HistoryTable)

	page, err := e.readerStore(t).QueryTraces(ctx, seed("org-a", "agent-1", "").Scope, store.TraceQuery{})
	if err != nil || page.Total != 1 {
		t.Errorf("reader QueryTraces = %+v, %v; want the one trace", page, err)
	}
}

func TestCrossOrgReadsSeeNothing(t *testing.T) {
	e := clickhouseEnv(t)
	ctx := context.Background()
	e.insert(t, records([]storetest.Seed{
		seed("org-a", "agent-1", "trace-a"),
		seed("org-b", "agent-1", "trace-b"),
	}))
	s := e.readerStore(t)
	other := seed("org-c", "agent-1", "").Scope

	if page, err := s.QueryTraces(ctx, other, store.TraceQuery{}); err != nil || page.Total != 0 || len(page.Traces) != 0 {
		t.Errorf("QueryTraces(org-c) = %+v, %v; want nothing", page, err)
	}
	for _, tr := range []string{"trace-a", "trace-b"} {
		if page, err := s.QueryTraceSpans(ctx, other, tr, store.SpanQuery{}); err != nil || len(page.Spans) != 0 {
			t.Errorf("QueryTraceSpans(org-c, %s) = %+v, %v; want nothing", tr, page, err)
		}
		if d, err := s.GetSpanDetails(ctx, other, tr, "root"); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("GetSpanDetails(org-c, %s) = %+v, %v; want ErrNotFound", tr, d, err)
		}
	}
	// An org-wide scope of org-a sees only org-a's trace.
	orgA := store.Scope{OrgID: "org-a", Names: store.Names{Org: "org-a"}}
	if page, err := s.QueryTraces(ctx, orgA, store.TraceQuery{}); err != nil || page.Total != 1 || page.Traces[0].TraceID != "trace-a" {
		t.Errorf("QueryTraces(org-a) = %+v, %v; want only trace-a", page, err)
	}
}

func TestDeleteAgent(t *testing.T) {
	e := clickhouseEnv(t)
	ctx := context.Background()
	e.insert(t, records([]storetest.Seed{
		seed("org-a", "agent-1", "trace-a1"),
		seed("org-a", "agent-2", "trace-a2"),
		seed("org-b", "agent-1", "trace-b1"),
	}))
	deleter := e.store(t, "amp_cleaner", os.Getenv("AMP_CLICKHOUSE_CLEANER_PASSWORD"))

	for range 2 { // the second delete finds nothing and succeeds
		if err := deleter.DeleteAgent(ctx, "org-a", "agent-1"); err != nil {
			t.Fatalf("DeleteAgent: %v", err)
		}
	}

	s := e.readerStore(t)
	for _, tc := range []struct {
		org, agent string
		want       int
	}{
		{"org-a", "agent-1", 0},
		{"org-a", "agent-2", 1},
		{"org-b", "agent-1", 1},
	} {
		page, err := s.QueryTraces(ctx, seed(tc.org, tc.agent, "").Scope, store.TraceQuery{})
		if err != nil {
			t.Fatal(err)
		}
		if page.Total != tc.want {
			t.Errorf("%s/%s: %d traces after deleting org-a/agent-1, want %d", tc.org, tc.agent, page.Total, tc.want)
		}
	}
}

func TestSchemaHistory(t *testing.T) {
	e := clickhouseEnv(t)
	ctx := context.Background()

	// The migrator recorded every file; the reader reads the same record.
	want, err := e.migratorSchema(t, 0).Applied(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := want["001_otel_traces"]; !ok {
		t.Fatalf("migrator Applied = %v, want 001_otel_traces recorded", want)
	}
	got, err := e.schema(t, "amp_reader", os.Getenv("AMP_CLICKHOUSE_READER_PASSWORD"), 0).Applied(ctx)
	if err != nil || len(got) != len(want) || got["001_otel_traces"] != want["001_otel_traces"] {
		t.Errorf("reader Applied = %v, %v; want %v", got, err, want)
	}

	// Applying a file again is safe, and its record is not duplicated.
	body, err := fs.ReadFile(Migrations(), "001_otel_traces.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.migratorSchema(t, 0).Apply(ctx, "001_otel_traces", want["001_otel_traces"], body); err != nil {
		t.Fatalf("re-applying 001: %v", err)
	}
	again, err := e.migratorSchema(t, 0).Applied(ctx)
	if err != nil || len(again) != len(want) {
		t.Errorf("Applied after re-applying = %v, %v; want %v", again, err, want)
	}
}

func TestSettleSetsTheTTL(t *testing.T) {
	e := clickhouseEnv(t)
	ctx := context.Background()
	engine := func() string {
		var out string
		if err := e.admin.query(ctx, "SELECT engine_full FROM system.tables WHERE database = 'otel' AND name = '"+Table+"'", nil,
			func(line []byte) error {
				var r struct {
					E string `json:"engine_full"`
				}
				err := json.Unmarshal(line, &r)
				out = r.E
				return err
			}); err != nil {
			t.Fatal(err)
		}
		return out
	}
	// Far-off TTLs, so the other tests' November 2026 spans never expire.
	for _, days := range []int{3651, 3650, 3650} {
		if err := e.migratorSchema(t, days).Settle(ctx); err != nil {
			t.Fatalf("Settle(%d): %v", days, err)
		}
		if want := fmt.Sprintf("TTL toDateTime(Timestamp) + toIntervalDay(%d) ", days); !strings.Contains(engine(), want) {
			t.Errorf("after Settle(%d) the table is %q, want %q", days, engine(), want)
		}
	}
	// Zero leaves it as it is.
	if err := e.migratorSchema(t, 0).Settle(ctx); err != nil || !strings.Contains(engine(), "toIntervalDay(3650)") {
		t.Errorf("Settle(0) = %v, table %q; want the TTL unchanged", err, engine())
	}
}

// seed is a one-span trace of agent in org; traceID may be empty for a Scope.
func seed(org, agent, traceID string) storetest.Seed {
	start := time.Date(2026, 11, 1, 1, 0, 0, 0, time.UTC)
	return storetest.Seed{
		Scope: store.Scope{OrgID: org, ProjectID: "project-1", AgentID: agent, Names: store.Names{Org: org, Agent: agent}},
		Trace: store.Trace{TraceID: traceID},
		Spans: []store.SpanDetails{{SpanID: "root", SpanName: "invoke_agent", StartTime: start, DurationNs: int64(time.Second)}},
	}
}

// BenchmarkQueryTracesOneAgentDay lists the first page of one agent's traces
// over a day holding a million of its spans, next to a million spans of
// other agents and orgs.
func BenchmarkQueryTracesOneAgentDay(b *testing.B) {
	e := clickhouseEnv(b)
	ctx := context.Background()
	day := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	fill := `INSERT INTO ` + Table + ` (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, SpanKind, ResourceAttributes, Duration)
SELECT
  fromUnixTimestamp64Nano(toInt64({start:Int64}) + intDiv(number, 10) * 860000000 + (number % 10) * 1000000),
  concat({prefix:String}, toString(intDiv(number, 10))),
  toString(number),
  if(number % 10 = 0, '', toString(number - number % 10)),
  'span', 'Internal',
  map('agent-manager/org-id', {org:String}, 'agent-manager/agent-uid', {agent:String}, 'agent-manager/project-uid', 'project-1'),
  1000000
FROM numbers(1000000)`
	for _, p := range []map[string]string{
		{"prefix": "t-", "org": "org-a", "agent": "agent-1"},
		{"prefix": "o-", "org": "org-a", "agent": "agent-2"},
		{"prefix": "x-", "org": "org-b", "agent": "agent-1"},
	} {
		p["start"] = fmt.Sprint(day.UnixNano())
		if err := e.admin.exec(ctx, fill, p); err != nil {
			b.Fatal(err)
		}
	}
	s := e.readerStore(b)
	sc := seed("org-a", "agent-1", "").Scope
	q := store.TraceQuery{StartTime: day, EndTime: day.Add(24 * time.Hour), Limit: 20, SortOrder: "desc"}
	b.ResetTimer()
	for range b.N {
		page, err := s.QueryTraces(ctx, sc, q)
		if err != nil {
			b.Fatal(err)
		}
		if page.Total != 100000 || len(page.Traces) != 20 {
			b.Fatalf("got %d of %d traces, want 20 of 100000", len(page.Traces), page.Total)
		}
	}
}
