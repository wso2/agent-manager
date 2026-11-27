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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/observability/store"
)

// A Scope with no organization is refused before any query: the store
// points at nothing listening, so a query would fail with another error.
func TestNoOrgRunsNoQuery(t *testing.T) {
	s, err := New(Config{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for name, sc := range map[string]store.Scope{
		"no org id":   {Names: store.Names{Org: "acme"}, AgentID: "a"},
		"no org name": {OrgID: "org-a", AgentID: "a"},
		"blank":       {OrgID: " ", Names: store.Names{Org: "acme"}},
	} {
		if _, err := s.QueryTraces(ctx, sc, store.TraceQuery{}); !errors.Is(err, store.ErrMissingOrg) {
			t.Errorf("%s: QueryTraces error = %v, want ErrMissingOrg", name, err)
		}
		if _, err := s.QueryTraceSpans(ctx, sc, "t", store.SpanQuery{}); !errors.Is(err, store.ErrMissingOrg) {
			t.Errorf("%s: QueryTraceSpans error = %v, want ErrMissingOrg", name, err)
		}
		if _, err := s.GetSpanDetails(ctx, sc, "t", "s"); !errors.Is(err, store.ErrMissingOrg) {
			t.Errorf("%s: GetSpanDetails error = %v, want ErrMissingOrg", name, err)
		}
	}
	if err := s.DeleteAgent(ctx, "", "a"); !errors.Is(err, store.ErrMissingOrg) {
		t.Errorf("DeleteAgent with no org = %v, want ErrMissingOrg", err)
	}
	if err := s.DeleteAgent(ctx, "org-a", ""); !errors.Is(err, ErrMissingAgent) {
		t.Errorf("DeleteAgent with no agent = %v, want ErrMissingAgent", err)
	}
}

func TestNewRejectsBadConfig(t *testing.T) {
	for _, cfg := range []Config{
		{URL: ""},
		{URL: "tcp://clickhouse:9000"},
	} {
		if _, err := New(cfg); err == nil {
			t.Errorf("New(%+v) succeeded, want an error", cfg)
		}
	}
}

// A URL with a login is refused, and no refused URL's password is in the error.
func TestNewRejectsURLWithLogin(t *testing.T) {
	for _, raw := range []string{"http://amp:s3cret@clickhouse:8123", "clickhouse://amp:s3cret@clickhouse:9000"} {
		_, err := New(Config{URL: raw, User: "amp"})
		if err == nil || strings.Contains(err.Error(), "s3cret") || !strings.Contains(err.Error(), "MODULE_OBSERVABILITY_CLICKHOUSE_URL") {
			t.Errorf("New(%q) error = %v, want one naming the setting without the password", raw, err)
		}
	}
}

// A bound past every storable timestamp is dropped, one that leaves no
// storable timestamp empties the window, and others are kept as given.
func TestWindowFilterBounds(t *testing.T) {
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	farPast := time.Date(1000, 1, 1, 0, 0, 0, 0, time.UTC)
	farFuture := time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)
	for name, tc := range map[string]struct {
		from, to  time.Time
		wantEmpty bool
		wantKeys  []string
	}{
		"open":                 {wantKeys: nil},
		"ordinary":             {from: from, to: to, wantKeys: []string{"from", "to"}},
		"far past start":       {from: farPast, to: to, wantKeys: []string{"to"}},
		"far future end":       {from: from, to: farFuture, wantKeys: []string{"from"}},
		"both far":             {from: farPast, to: farFuture, wantKeys: nil},
		"start after storable": {from: farFuture, wantEmpty: true},
		"end before storable":  {to: farPast, wantEmpty: true},
	} {
		params := map[string]string{}
		_, empty := windowFilter(tc.from, tc.to, params)
		if empty != tc.wantEmpty || len(params) != len(tc.wantKeys) {
			t.Errorf("%s: empty = %v, params = %v; want empty %v, params %v", name, empty, params, tc.wantEmpty, tc.wantKeys)
			continue
		}
		for _, k := range tc.wantKeys {
			if _, ok := params[k]; !ok {
				t.Errorf("%s: params = %v, want %s", name, params, k)
			}
		}
	}
	got := map[string]string{}
	windowFilter(from, to, got)
	if got["from"] != strconv.FormatInt(from.UnixNano(), 10) || got["to"] != strconv.FormatInt(to.UnixNano(), 10) {
		t.Errorf("ordinary params = %v, want the bounds in nanoseconds", got)
	}
}

// A window no stored span can be in returns an empty page without querying:
// the store points at nothing listening.
func TestEmptyWindowRunsNoQuery(t *testing.T) {
	s, err := New(Config{URL: "http://127.0.0.1:1"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	sc := store.Scope{OrgID: "org-a", Names: store.Names{Org: "acme"}}
	future := time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)
	if page, err := s.QueryTraces(ctx, sc, store.TraceQuery{StartTime: future}); err != nil || len(page.Traces) != 0 {
		t.Errorf("QueryTraces = %+v, %v; want an empty page", page, err)
	}
	if page, err := s.QueryTraceSpans(ctx, sc, "t", store.SpanQuery{StartTime: future}); err != nil || len(page.Spans) != 0 {
		t.Errorf("QueryTraceSpans = %+v, %v; want an empty page", page, err)
	}
}

// A trace's spans are read from clockSkew before the window to traceSpread
// after it; an open bound stays open.
func TestTraceBounds(t *testing.T) {
	from := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	to := from.Add(time.Hour)
	if f, e := traceBounds(from, to); !f.Equal(from.Add(-time.Minute)) || !e.Equal(to.Add(24*time.Hour)) {
		t.Errorf("traceBounds(%v, %v) = %v, %v; want a minute before and a day after", from, to, f, e)
	}
	if f, e := traceBounds(time.Time{}, time.Time{}); !f.IsZero() || !e.IsZero() {
		t.Errorf("traceBounds of an open window = %v, %v; want it open", f, e)
	}
	params := map[string]string{}
	far := time.Date(3000, 1, 1, 0, 0, 0, 0, time.UTC)
	f, e := traceBounds(from, far)
	if where, empty := timeFilter(f, e, "trace_from", "trace_to", params); empty || len(params) != 1 ||
		params["trace_from"] != strconv.FormatInt(f.UnixNano(), 10) || where != " AND Timestamp >= fromUnixTimestamp64Nano({trace_from:Int64})" {
		t.Errorf("timeFilter of a far end = %q, %v, %v; want only the start bound, as trace_from", where, empty, params)
	}
}

// The reader, cleaner and schema clients all use the configured database.
func TestConfiguredDatabase(t *testing.T) {
	var mu sync.Mutex
	var dbs []string
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		defer mu.Unlock()
		q := r.URL.Query()
		if db := q.Get("database"); db != "" {
			dbs = append(dbs, db)
		} else {
			dbs = append(dbs, q.Get("param_db"))
		}
		bodies = append(bodies, string(body))
		_, _ = io.WriteString(w, `{"n":3}`+"\n")
	}))
	defer srv.Close()
	ctx := context.Background()

	s, err := New(Config{URL: srv.URL, Database: "traces", User: "amp_reader"})
	if err != nil {
		t.Fatal(err)
	}
	sc := store.Scope{OrgID: "org-a", Names: store.Names{Org: "acme"}}
	if _, err := s.QueryTraces(ctx, sc, store.TraceQuery{}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteAgent(ctx, "org-a", "a"); err != nil {
		t.Fatal(err)
	}
	schema, err := NewSchema(SchemaConfig{URL: srv.URL, Database: "traces", User: "amp_migrator"}, Migrations())
	if err != nil {
		t.Fatal(err)
	}
	if err := schema.Apply(ctx, "001_test", "sum", []byte("SELECT 1")); err != nil {
		t.Fatal(err)
	}
	if _, err := schema.Applied(ctx); err != nil {
		t.Fatal(err)
	}
	for i, db := range dbs {
		if db != "traces" && !strings.HasPrefix(bodies[i], "CREATE DATABASE IF NOT EXISTS traces") {
			t.Errorf("request %q went to database %q, want traces", bodies[i], db)
		}
	}
	if !slices.Contains(bodies, "CREATE DATABASE IF NOT EXISTS traces") {
		t.Errorf("requests %q create no database traces", bodies)
	}

	for _, bad := range []string{"otel; DROP TABLE x", "1db", "my-db"} {
		if _, err := New(Config{URL: srv.URL, Database: bad, User: "amp_reader"}); err == nil || !strings.Contains(err.Error(), "MODULE_OBSERVABILITY_CLICKHOUSE_DATABASE") {
			t.Errorf("New with database %q = %v, want a database error", bad, err)
		}
		if _, err := NewSchema(SchemaConfig{URL: srv.URL, Database: bad, User: "amp_migrator"}, Migrations()); err == nil {
			t.Errorf("NewSchema with database %q succeeded, want an error", bad)
		}
	}
}

// A query that fails after its first rows is an error, not a short result.
func TestQueryFailingMidStream(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"n":1}`+"\n"+`{"exception": "Code: 159. DB::Exception: Timeout exceeded"}`+"\n")
	}))
	defer srv.Close()
	c, err := newClient(srv.URL, "", "amp_reader", "")
	if err != nil {
		t.Fatal(err)
	}
	var rows []string
	err = c.query(context.Background(), "SELECT 1", nil, func(line []byte) error {
		rows = append(rows, string(line))
		return nil
	})
	var chErr *Error
	if !errors.As(err, &chErr) || !strings.Contains(chErr.Message, "Timeout exceeded") {
		t.Fatalf("query error = %v; want the streamed exception", err)
	}
	if len(rows) != 1 {
		t.Errorf("rows = %q; want only the row before the exception", rows)
	}
}
