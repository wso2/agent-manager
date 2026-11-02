package clickhouse

import (
	"strings"
	"testing"
	"time"
)

func TestBuildTraceListQuery(t *testing.T) {
	w := traceWindow{Start: time.Unix(1000, 0), End: time.Unix(2000, 0)}
	q, args := buildTraceListQuery("otel.otel_traces_tenant", tenantTable,
		ScopeUIDs{Namespace: "default", Component: "abc"}, w, true, 10)

	if !strings.Contains(q, "ComponentUid = ?") {
		t.Error("component filter missing")
	}
	if strings.Contains(q, "abc") {
		t.Error("request values must never appear inside the SQL text")
	}
	if !strings.Contains(q, "ORDER BY start_time DESC, TraceId ASC") {
		t.Error("wrong sort order")
	}
	if len(args) != 6 { // namespace, component, three times, limit
		t.Errorf("expected 6 values, got %d", len(args))
	}
}

func TestBuildTraceSpansQuery(t *testing.T) {
	w := traceWindow{Start: time.Unix(1000, 0), End: time.Unix(2000, 0)}

	q, args := buildTraceSpansQuery("otel_traces", exporterTable, "trace-1",
		ScopeUIDs{Namespace: "default", Component: "abc"}, w, false, false, 50)

	if strings.Contains(q, "trace-1") || strings.Contains(q, "abc") {
		t.Error("request values must never appear inside the SQL text")
	}
	if strings.Contains(q, "SpanAttributes") {
		t.Error("attributes must not be selected unless asked for")
	}
	if len(args) != 7 { // trace id, namespace, component, three times, limit
		t.Errorf("expected 7 values, got %d", len(args))
	}

	q, _ = buildTraceSpansQuery("otel_traces", exporterTable, "trace-1", ScopeUIDs{}, w, true, true, 50)
	if !strings.Contains(q, "SpanAttributes, ResourceAttributes") || !strings.Contains(q, "Timestamp DESC") {
		t.Error("attributes or sort order missing")
	}
}

func TestBuildSpanDetailQuery(t *testing.T) {
	q, args := buildSpanDetailQuery("otel_traces", exporterTable, "default", "trace-1", "span-1")
	if strings.Contains(q, "trace-1") || strings.Contains(q, "span-1") {
		t.Error("request values must never appear inside the SQL text")
	}
	if !strings.Contains(q, "ResourceAttributes['openchoreo.dev/namespace'] = ?") {
		t.Error("namespace condition missing")
	}
	want := []any{"trace-1", "span-1", "default", "trace-1", "trace-1"}
	if len(args) != len(want) {
		t.Fatalf("got %d values, want %d", len(args), len(want))
	}
	for i := range want {
		if args[i] != want[i] {
			t.Errorf("value %d: got %v, want %v", i, args[i], want[i])
		}
	}
}
