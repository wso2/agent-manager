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

package traces

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/wso2/agent-manager/cli/pkg/clients/traceobssvc"
	"github.com/wso2/agent-manager/cli/pkg/iostreams"
)

func TestListTraces_TextOutput(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = false
	client, closeFn := newTraceTestClient(t, http.StatusOK, traceobssvc.TraceOverviewResponse{
		TotalCount: 1,
		Traces: []traceobssvc.TraceOverview{
			{
				TraceID:      "abc123",
				RootSpanName: "handle_request",
				SpanCount:    8,
			},
		},
	})
	defer closeFn()

	err := runListTraces(context.Background(), &ListTracesOptions{
		IO: ios, TraceClient: client, Scope: traceBaseScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", Env: "dev",
		StartTime: "2026-05-12T00:00:00Z", EndTime: "2026-05-13T00:00:00Z",
		Limit: 10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "abc123") {
		t.Errorf("output should contain trace ID, got %q", got)
	}
}

func TestRunFilteredTraces_SameColumnsAsList(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = false
	startTime := time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339Nano)
	client, closeFn := newTraceTestClient(t, http.StatusOK, traceobssvc.TraceOverviewResponse{
		TotalCount: 1,
		Traces: []traceobssvc.TraceOverview{
			{
				TraceID:         "abc123",
				RootSpanID:      "rootspanidshouldnotshow",
				RootSpanName:    "handle_request",
				SpanCount:       100,
				StartTime:       startTime,
				DurationInNanos: 60_000_000_000,
			},
		},
	})
	defer closeFn()

	err := runFilteredTraces(context.Background(), &ListTracesOptions{
		IO: ios, TraceClient: client, Scope: traceBaseScope(),
		Org: "acme", Proj: "triage", AgentName: "my-agent", Env: "dev",
		StartTime: "2026-05-12T00:00:00Z", EndTime: "2026-05-13T00:00:00Z",
		Limit:     10,
		Condition: conditionExcessiveSteps,
		MaxSpans:  10,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "handle_request") {
		t.Errorf("filtered output should show RootSpanName (handle_request), got %q", got)
	}
	if strings.Contains(got, "rootspanidshould") {
		t.Errorf("filtered output should NOT show RootSpanID, got %q", got)
	}
	if !strings.Contains(got, "ago") {
		t.Errorf("filtered output should include the 'started' column (relative time), got %q", got)
	}
}

func TestValidateCondition(t *testing.T) {
	for _, c := range []string{
		"",
		conditionErrorStatus,
		conditionHighLatency,
		conditionHighTokenUsage,
		conditionToolCallFails,
		conditionExcessiveSteps,
	} {
		if err := validateCondition(c); err != nil {
			t.Errorf("validateCondition(%q) returned %v, want nil", c, err)
		}
	}
	err := validateCondition("bogus_condition")
	if err == nil {
		t.Fatal("validateCondition(\"bogus_condition\") returned nil, want error")
	}
	if !strings.Contains(err.Error(), "bogus_condition") {
		t.Errorf("error should mention the invalid value, got %q", err.Error())
	}
}

func TestRenderFilteredOverview_JSONShapeStableWhenEmpty(t *testing.T) {
	ios, _, out, _ := iostreams.Test()
	ios.JSON = true

	err := renderFilteredOverview(&ListTracesOptions{IO: ios, Scope: traceBaseScope()}, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var envelope struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(out.Bytes(), &envelope); err != nil {
		t.Fatalf("decode: %v\nbody: %s", err, out.String())
	}
	traces, ok := envelope.Data["traces"]
	if !ok {
		t.Fatalf("data missing \"traces\" key: %s", out.String())
	}
	if traces == nil {
		t.Errorf("traces should be [], not null: %s", out.String())
	}
	if arr, ok := traces.([]any); !ok || len(arr) != 0 {
		t.Errorf("traces should be empty array, got %T %v", traces, traces)
	}
	count, ok := envelope.Data["count"]
	if !ok {
		t.Fatalf("data missing \"count\" key (non-empty branch uses \"count\"): %s", out.String())
	}
	if n, _ := count.(float64); n != 0 {
		t.Errorf("count = %v, want 0", count)
	}
}
