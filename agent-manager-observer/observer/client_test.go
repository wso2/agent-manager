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

package observer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
)

// fakeOpenSearch serves canned _search responses and records request bodies.
type fakeOpenSearch struct {
	t        *testing.T
	bodies   []string
	respond  func(body string) string
	status   int
	username string
	password string
	path     string
}

func (f *fakeOpenSearch) start() *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		f.bodies = append(f.bodies, string(data))
		f.path = r.URL.Path
		f.username, f.password, _ = r.BasicAuth()
		if f.status != 0 {
			w.WriteHeader(f.status)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		_, _ = w.Write([]byte(f.respond(string(data))))
	}))
	f.t.Cleanup(srv.Close)
	return srv
}

func newTestClient(url string) Client {
	return NewClient(config.OpenSearchConfig{Address: url, Username: "admin", Password: "pw", TraceIndex: "otel-traces-*"})
}

func TestQueryTraces_RootSpansWithStats(t *testing.T) {
	f := &fakeOpenSearch{t: t, respond: func(body string) string {
		if strings.Contains(body, `"collapse"`) {
			return `{"hits":{"total":{"value":1},"hits":[{"_source":{
				"traceId":"t1","spanId":"root","name":"invoke_agent","kind":"Server",
				"startTime":"2026-01-01T10:00:00Z","endTime":"2026-01-01T10:00:01Z","durationInNanos":1000000000}}]},
				"aggregations":{"traces":{"value":7}}}`
		}
		end := float64(time.Date(2026, 1, 1, 10, 0, 3, 0, time.UTC).UnixMilli())
		b, _ := json.Marshal(map[string]any{"hits": map[string]any{"hits": []any{}}, "aggregations": map[string]any{
			"by_trace": map[string]any{"buckets": []any{map[string]any{"key": "t1", "spans": map[string]any{"value": 4}, "lastEnd": map[string]any{"value": end}}}},
		}})
		return string(b)
	}}
	srv := f.start()

	project, agent, limit, order := "proj", "agent-a", 10, "desc"
	resp, err := newTestClient(srv.URL).QueryTraces(context.Background(), TracesQueryRequest{
		StartTime:   time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		EndTime:     time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
		Limit:       &limit,
		SortOrder:   &order,
		SearchScope: TraceScope{Organization: "org", Project: &project, Agent: &agent},
	})
	if err != nil {
		t.Fatalf("QueryTraces: %v", err)
	}
	if f.path != "/otel-traces-*/_search" {
		t.Errorf("unexpected path %q", f.path)
	}
	if f.username != "admin" || f.password != "pw" {
		t.Errorf("basic auth not sent: %q/%q", f.username, f.password)
	}
	if len(f.bodies) != 2 {
		t.Fatalf("expected list + stats queries, got %d", len(f.bodies))
	}
	for _, want := range []string{`"resource.agent-manager/organization":"org"`, `"resource.agent-manager/project":"proj"`, `"resource.agent-manager/agent":"agent-a"`, `"parentSpanId":""`} {
		if !strings.Contains(f.bodies[0], want) {
			t.Errorf("list query missing %s: %s", want, f.bodies[0])
		}
	}
	if resp.Total != 7 || len(resp.Traces) != 1 {
		t.Fatalf("unexpected response: %+v", resp)
	}
	got := resp.Traces[0]
	if got.TraceID != "t1" || got.RootSpanID != "root" || got.TraceName != "invoke_agent" || got.SpanCount != 4 {
		t.Errorf("unexpected trace: %+v", got)
	}
	if got.DurationNs != int64(3*time.Second) {
		t.Errorf("expected duration extended to the last span, got %d", got.DurationNs)
	}
}

func TestQueryTraces_EmptySkipsStats(t *testing.T) {
	f := &fakeOpenSearch{t: t, respond: func(string) string { return `{"hits":{"total":{"value":0},"hits":[]}}` }}
	srv := f.start()
	resp, err := newTestClient(srv.URL).QueryTraces(context.Background(), TracesQueryRequest{SearchScope: TraceScope{Organization: "org"}})
	if err != nil {
		t.Fatalf("QueryTraces: %v", err)
	}
	if len(resp.Traces) != 0 || len(f.bodies) != 1 {
		t.Errorf("expected one query and no traces, got %d queries, %+v", len(f.bodies), resp)
	}
}

func TestQueryTraceSpans(t *testing.T) {
	f := &fakeOpenSearch{t: t, respond: func(string) string {
		return `{"hits":{"total":{"value":3},"hits":[
			{"_source":{"traceId":"t1","spanId":"a","name":"root","startTime":"2026-01-01T10:00:00Z","endTime":"2026-01-01T10:00:01Z","status":{"code":"Error","message":"boom"}}},
			{"_source":{"traceId":"t1","spanId":"a","name":"root"}},
			{"_source":{"traceId":"t1","spanId":"b","parentSpanId":"a","name":"chat gpt-4o","attributes":{"gen_ai.system":"openai"}}}]}}`
	}}
	srv := f.start()
	c := newTestClient(srv.URL)

	resp, err := c.QueryTraceSpans(context.Background(), "t1", TracesQueryRequest{SearchScope: TraceScope{Organization: "org"}})
	if err != nil {
		t.Fatalf("QueryTraceSpans: %v", err)
	}
	if !strings.Contains(f.bodies[0], `"excludes":["attributes","resource"]`) {
		t.Errorf("expected attributes excluded: %s", f.bodies[0])
	}
	if len(resp.Spans) != 2 || resp.Total != 3 {
		t.Fatalf("expected duplicate dropped, got %+v", resp)
	}
	if resp.Spans[0].Status == nil || resp.Spans[0].Status.Message != "boom" || resp.Spans[0].DurationNs != int64(time.Second) {
		t.Errorf("unexpected root span: %+v", resp.Spans[0])
	}

	if _, err := c.QueryTraceSpans(context.Background(), "t1", TracesQueryRequest{IncludeAttributes: true}); err != nil {
		t.Fatalf("QueryTraceSpans: %v", err)
	}
	if strings.Contains(f.bodies[1], "excludes") {
		t.Errorf("expected attributes included: %s", f.bodies[1])
	}
}

func TestGetSpanDetails_NotFound(t *testing.T) {
	f := &fakeOpenSearch{t: t, respond: func(string) string { return `{"hits":{"total":{"value":0},"hits":[]}}` }}
	srv := f.start()
	_, err := newTestClient(srv.URL).GetSpanDetails(context.Background(), "t1", "s1")
	if !errors.Is(err, ErrSpanNotFound) {
		t.Fatalf("expected ErrSpanNotFound, got %v", err)
	}
}

func TestSearch_ErrorStatus(t *testing.T) {
	f := &fakeOpenSearch{t: t, status: http.StatusInternalServerError}
	srv := f.start()
	_, err := newTestClient(srv.URL).GetSpanDetails(context.Background(), "t1", "s1")
	if err == nil || !strings.Contains(err.Error(), "status 500") {
		t.Fatalf("expected status error, got %v", err)
	}
}
