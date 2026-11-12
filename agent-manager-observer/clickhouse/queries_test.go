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
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/scope"
)

func TestBuildTraceListQuery(t *testing.T) {
	w := traceWindow{Start: time.Unix(1000, 0), End: time.Unix(2000, 0)}
	q, args := buildTraceListQuery("otel.otel_traces_tenant", tenantTable,
		scope.UIDs{Namespace: "default", Component: "abc"}, w, true, 10)

	if !strings.Contains(q, "ComponentUid = ?") {
		t.Error("component filter missing")
	}
	if strings.Contains(q, "abc") {
		t.Error("request values must never appear inside the SQL text")
	}
	if !strings.Contains(q, "ORDER BY start_time DESC, TraceId ASC") {
		t.Error("wrong sort order")
	}
	if len(args) != 16 { // namespace, component, three times, limit
		t.Errorf("expected 16 values, got %d", len(args))
	}
}

func TestBuildTraceSpansQuery(t *testing.T) {
	w := traceWindow{Start: time.Unix(1000, 0), End: time.Unix(2000, 0)}

	q, args := buildTraceSpansQuery("otel_traces", exporterTable, "trace-1",
		scope.UIDs{Namespace: "default", Component: "abc"}, w, false, false, 50)

	if strings.Contains(q, "trace-1") || strings.Contains(q, "abc") {
		t.Error("request values must never appear inside the SQL text")
	}
	if strings.Contains(q, "SpanAttributes") {
		t.Error("attributes must not be selected unless asked for")
	}
	if len(args) != 7 { // trace id, namespace, component, three times, limit
		t.Errorf("expected 7 values, got %d", len(args))
	}

	q, _ = buildTraceSpansQuery("otel_traces", exporterTable, "trace-1", scope.UIDs{}, w, true, true, 50)
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

func TestBuildSpansBatchQuery(t *testing.T) {
	w := traceWindow{Start: time.Unix(1000, 0), End: time.Unix(2000, 0)}
	traceIDs := []string{"trace-a", "trace-b"}

	q, args := buildSpansBatchQuery("otel_traces", exporterTable, traceIDs,
		scope.UIDs{Namespace: "default", Component: "abc"}, w, 500)

	if strings.Contains(q, "trace-a") || strings.Contains(q, "trace-b") {
		t.Error("request values must never appear inside the SQL text")
	}
	if !strings.Contains(q, "TraceId IN (?,?)") {
		t.Errorf("expected TraceId IN (?,?), query was: %s", q)
	}
	if !strings.Contains(q, "ResourceAttributes['openchoreo.dev/component-uid'] = ?") {
		t.Error("component scope condition missing")
	}
	if !strings.Contains(q, "LIMIT ?") {
		t.Error("limit parameter missing")
	}
	// args: trace-a, trace-b, namespace, component, 3x timestamps, maxRows = 2 + 2 + 3 + 1 = 8
	if len(args) != 8 {
		t.Errorf("expected 8 args, got %d (%v)", len(args), args)
	}
	if args[0] != "trace-a" || args[1] != "trace-b" {
		t.Errorf("expected trace IDs at the beginning of args, got %v", args[:2])
	}
	if args[len(args)-1] != 500 {
		t.Errorf("expected maxRows at the end of args, got %v", args[len(args)-1])
	}
}

