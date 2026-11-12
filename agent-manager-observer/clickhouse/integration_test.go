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
	"os"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/config"
	"github.com/wso2/agent-manager/agent-manager-observer/observer"
	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

// Talks to a real ClickHouse. Skipped unless CLICKHOUSE_TEST_ADDR is set.
func newIntegrationReader(t *testing.T, layout, table string) *TraceReaderImpl {
	t.Helper()
	addr := os.Getenv("CLICKHOUSE_TEST_ADDR")
	if addr == "" {
		t.Skip("CLICKHOUSE_TEST_ADDR not set")
	}
	db, err := Open(config.TraceReaderConfig{
		Addr: addr, Database: "otel",
		User: os.Getenv("CLICKHOUSE_TEST_USER"), Password: os.Getenv("CLICKHOUSE_TEST_PASSWORD"),
		MaxOpenConns: 2, MaxIdleConns: 1, ConnMaxLifetime: time.Minute, DialTimeout: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	resolver := scope.StaticResolver{
		Projects:     map[string]string{"default/default": "55f125d0-35fc-4b66-af37-50b9fc97e0a0"},
		Components:   map[string]string{"default/it-support": "f41255dd-7647-4401-895e-a55d629dbf5e"},
		Environments: map[string]string{"default/default": "c7acefa5-b717-40f6-b056-2c05a739420c"},
	}
	r, err := NewTraceReader(db, Options{Table: table, Layout: layout, DefaultNamespace: "default", Resolver: resolver})
	if err != nil {
		t.Fatalf("new reader: %v", err)
	}
	return r
}

func integrationRequest(limit int) observer.TracesQueryRequest {
	str := func(s string) *string { return &s }
	order := "desc"
	return observer.TracesQueryRequest{
		StartTime: time.Date(2026, 10, 29, 0, 0, 0, 0, time.UTC),
		EndTime:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		Limit:     &limit,
		SortOrder: &order,
		SearchScope: observer.ComponentSearchScope{
			Namespace: "default", Project: str("default"),
			Component: str("it-support"), Environment: str("default"),
		},
	}
}

func TestIntegration_QueryTraces(t *testing.T) {
	str := func(s string) *string { return &s }
	limit := 10

	for _, tc := range []struct{ layout, table string }{
		{config.TraceLayoutExporter, "otel_traces"},
		{config.TraceLayoutTenant, "otel_traces_tenant"},
	} {
		t.Run(tc.layout, func(t *testing.T) {
			r := newIntegrationReader(t, tc.layout, tc.table)

			req := integrationRequest(limit)

			resp, err := r.QueryTraces(context.Background(), req)
			if err != nil {
				t.Fatalf("QueryTraces: %v", err)
			}
			if len(resp.Traces) != 10 || resp.Total < 10 {
				t.Fatalf("got %d traces, total %d", len(resp.Traces), resp.Total)
			}
			for i, tr := range resp.Traces {
				if tr.SpanCount != 5 || tr.RootSpanName != "lets-go" || tr.DurationNs <= 0 {
					t.Errorf("trace %d looks wrong: %+v", i, tr)
				}
				if tr.StartTime.Location() != time.UTC {
					t.Errorf("trace %d start time is not UTC", i)
				}
				if i > 0 && tr.StartTime.After(resp.Traces[i-1].StartTime) {
					t.Errorf("traces are not newest first at %d", i)
				}
			}

			// A name that does not exist must be refused, never treated as "no filter".
			req.SearchScope.Component = str("no-such-agent")
			if _, err := r.QueryTraces(context.Background(), req); !errors.Is(err, scope.ErrUnknownScope) {
				t.Errorf("unknown agent must fail with ErrUnknownScope, got %v", err)
			}
		})
	}
}

func TestIntegration_Spans(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct{ layout, table string }{
		{config.TraceLayoutExporter, "otel_traces"},
		{config.TraceLayoutTenant, "otel_traces_tenant"},
	} {
		t.Run(tc.layout, func(t *testing.T) {
			r := newIntegrationReader(t, tc.layout, tc.table)

			list, err := r.QueryTraces(ctx, integrationRequest(1))
			if err != nil || len(list.Traces) != 1 {
				t.Fatalf("need one trace to start from: %v", err)
			}
			trace := list.Traces[0]

			req := integrationRequest(1000)
			req.SortOrder = nil // oldest first
			req.IncludeAttributes = true
			spans, err := r.QueryTraceSpans(ctx, trace.TraceID, req)
			if err != nil {
				t.Fatalf("QueryTraceSpans: %v", err)
			}
			if len(spans.Spans) != trace.SpanCount {
				t.Fatalf("got %d spans, the list said %d", len(spans.Spans), trace.SpanCount)
			}
			roots := 0
			for _, s := range spans.Spans {
				if s.ParentSpanID == "" {
					roots++
					if s.SpanID != trace.RootSpanID {
						t.Errorf("root span %s differs from the list's %s", s.SpanID, trace.RootSpanID)
					}
				}
				if !s.EndTime.After(s.StartTime) || s.Status == nil || s.Attributes == nil {
					t.Errorf("span looks wrong: %+v", s)
				}
			}
			if roots != 1 {
				t.Errorf("expected exactly one root span, got %d", roots)
			}

			req.IncludeAttributes = false
			plain, err := r.QueryTraceSpans(ctx, trace.TraceID, req)
			if err != nil || len(plain.Spans) == 0 || plain.Spans[0].Attributes != nil {
				t.Errorf("attributes must be left out when not asked for: %v", err)
			}

			detail, err := r.GetSpanDetails(ctx, trace.TraceID, trace.RootSpanID)
			if err != nil {
				t.Fatalf("GetSpanDetails: %v", err)
			}
			if detail.SpanName != "lets-go" ||
				detail.ResourceAttributes["openchoreo.dev/component-uid"] != "f41255dd-7647-4401-895e-a55d629dbf5e" {
				t.Errorf("unexpected details: %+v", detail)
			}

			if _, err := r.GetSpanDetails(ctx, trace.TraceID, "0000000000000000"); !errors.Is(err, ErrSpanNotFound) {
				t.Errorf("unknown span should be ErrSpanNotFound, got %v", err)
			}
			if _, err := r.GetSpanDetails(ctx, "00000000000000000000000000000000", trace.RootSpanID); !errors.Is(err, ErrSpanNotFound) {
				t.Errorf("unknown trace should be ErrSpanNotFound, got %v", err)
			}

			other, err := NewTraceReader(r.db, Options{Table: r.table, Layout: tc.layout,
				DefaultNamespace: "other", Resolver: scope.StaticResolver{}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := other.GetSpanDetails(ctx, trace.TraceID, trace.RootSpanID); !errors.Is(err, ErrSpanNotFound) {
				t.Errorf("a span from another namespace must not be found, got %v", err)
			}

			// Batch span query test
			batchRes, err := r.QueryTraceSpansBatch(ctx, []string{trace.TraceID}, req)
			if err != nil {
				t.Fatalf("QueryTraceSpansBatch: %v", err)
			}
			if len(batchRes[trace.TraceID]) != trace.SpanCount {
				t.Errorf("expected %d spans in batch result, got %d", trace.SpanCount, len(batchRes[trace.TraceID]))
			}
		})
	}
}

