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

package tools

import (
	"context"
	"slices"
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// traceListOptionalParams are the optional inputs list_traces and get_traces share.
var traceListOptionalParams = []string{
	"start_time", "end_time", "limit", "sort_order",
	"status", "min_duration_ms", "min_tokens", "min_span_count", "model", "conversation_id",
	"tool", "tool_error", "mcp_server", "missing",
}

// Returns the test specs for tools registered by registerTraceTools.
// New tools added to traces.go must have a spec here — registration_test.go fails otherwise.
func tracesToolSpecs() []toolTestSpec {
	baseTraceArgs := map[string]any{
		"organization": testOrgName,
		"project":      testProjectName,
		"agent":        testAgentName,
		"environment":  testEnvName,
	}

	return []toolTestSpec{
		{
			name:                "list_traces",
			descriptionKeywords: []string{"trace"},
			descriptionMinLen:   20,
			requiredParams:      []string{"organization", "project", "agent", "environment"},
			optionalParams:      append(slices.Clone(traceListOptionalParams), "include_models", "include_tools", "include_mcp_servers", "cursor"),
			testArgs:            baseTraceArgs,
		},
		{
			name:                "get_traces",
			descriptionKeywords: []string{"trace", "span"},
			descriptionMinLen:   20,
			requiredParams:      []string{"organization", "project", "agent", "environment"},
			optionalParams:      traceListOptionalParams,
			testArgs:            baseTraceArgs,
		},
		{
			// Mirrors REST GetTraceSpans: only organization + trace_id are
			// required; project/agent/environment are optional scoping filters.
			name:                "get_trace_details",
			descriptionKeywords: []string{"trace", "span"},
			descriptionMinLen:   20,
			requiredParams:      []string{"organization", "trace_id"},
			optionalParams:      []string{"project", "agent", "environment", "start_time", "end_time"},
			testArgs: map[string]any{
				"organization": testOrgName,
				"trace_id":     testTraceID,
			},
		},
		{
			name:                "get_span_details",
			descriptionKeywords: []string{"span"},
			descriptionMinLen:   20,
			requiredParams:      []string{"trace_id", "span_id"},
			testArgs: map[string]any{
				"trace_id": testTraceID,
				"span_id":  testSpanID,
			},
		},
	}
}

// Verifies that a malformed start_time is rejected as a tool error rather
// than reaching the observer client.
func TestBadRFC3339Rejected(t *testing.T) {
	clientSession, fake := setupTestServer(t)
	ctx := context.Background()

	result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
		Name: "list_traces",
		Arguments: map[string]any{
			"organization": testOrgName,
			"project":      testProjectName,
			"agent":        testAgentName,
			"environment":  testEnvName,
			"start_time":   "not-a-timestamp",
			"end_time":     "2026-01-02T00:00:00Z",
		},
	})
	switch {
	case err != nil:
		// Protocol-level rejection — fine.
	case result != nil && result.IsError:
		// Handler-level tool error — fine.
	default:
		t.Error("expected error for malformed start_time; got success")
	}

	if calls := fake.calls["QueryTraces"]; len(calls) != 0 {
		t.Errorf("expected no QueryTraces call for a rejected request, got %d", len(calls))
	}
}

// Verifies that list_traces rejects a non-positive limit but clamps an
// over-max limit down to maxTraceListLimit, mirroring parseLimit in
// handlers/handlers.go (used by GetTraceOverviews/ExportTraces/GetTraceSpans):
// a non-positive value is a caller error, but an over-max value is silently
// clamped rather than rejected.
func TestTraceListLimitBounds(t *testing.T) {
	rejectedCases := []struct {
		name  string
		limit int
	}{
		{"zero", 0},
		{"negative", -1},
	}

	for _, tc := range rejectedCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			clientSession, fake := setupTestServer(t)
			ctx := context.Background()

			result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
				Name: "list_traces",
				Arguments: map[string]any{
					"organization": testOrgName,
					"project":      testProjectName,
					"agent":        testAgentName,
					"environment":  testEnvName,
					"limit":        tc.limit,
				},
			})
			switch {
			case err != nil:
				// Protocol-level rejection — fine.
			case result != nil && result.IsError:
				// Handler-level tool error — fine.
			default:
				t.Errorf("expected error for limit=%d; got success", tc.limit)
			}

			if calls := fake.calls["QueryTraces"]; len(calls) != 0 {
				t.Errorf("expected no QueryTraces call for a rejected request, got %d", len(calls))
			}
		})
	}

	t.Run("within bounds succeeds", func(t *testing.T) {
		clientSession, fake := setupTestServer(t)
		ctx := context.Background()

		result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
			Name: "list_traces",
			Arguments: map[string]any{
				"organization": testOrgName,
				"project":      testProjectName,
				"agent":        testAgentName,
				"environment":  testEnvName,
				"limit":        maxTraceListLimit,
			},
		})
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected success, got tool error: %+v", result.Content)
		}
		if calls := fake.calls["QueryTraces"]; len(calls) != 1 {
			t.Errorf("expected exactly one QueryTraces call, got %d", len(calls))
		}
	})

	t.Run("over max is clamped, not rejected", func(t *testing.T) {
		clientSession, fake := setupTestServer(t)
		ctx := context.Background()

		result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
			Name: "list_traces",
			Arguments: map[string]any{
				"organization": testOrgName,
				"project":      testProjectName,
				"agent":        testAgentName,
				"environment":  testEnvName,
				"limit":        maxTraceListLimit + 1,
			},
		})
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected success (clamped limit), got tool error: %+v", result.Content)
		}
		calls := fake.calls["QueryTraces"]
		if len(calls) != 1 {
			t.Fatalf("expected exactly one QueryTraces call, got %d", len(calls))
		}
		args, ok := calls[0].([]any)
		if !ok || len(args) != 1 {
			t.Fatalf("unexpected recorded args: %v", calls[0])
		}
		req, ok := args[0].(observer.TracesQueryRequest)
		if !ok {
			t.Fatalf("recorded arg has unexpected type %T", args[0])
		}
		if req.Limit == nil || *req.Limit != maxTraceListLimit {
			t.Errorf("Limit: got %v, want %d (clamped)", req.Limit, maxTraceListLimit)
		}
	})
}

// Verifies that get_traces (ExportTraces) also clamps an over-max limit down
// to maxTraceExportLimit rather than rejecting it, mirroring parseLimit in
// handlers/handlers.go.
func TestGetTracesLimitClamped(t *testing.T) {
	clientSession, fake := setupTestServer(t)
	ctx := context.Background()

	result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
		Name: "get_traces",
		Arguments: map[string]any{
			"organization": testOrgName,
			"project":      testProjectName,
			"agent":        testAgentName,
			"environment":  testEnvName,
			"limit":        maxTraceExportLimit + 1,
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success (clamped limit), got tool error: %+v", result.Content)
	}
	calls := fake.calls["QueryTraces"]
	if len(calls) != 1 {
		t.Fatalf("expected exactly one QueryTraces call, got %d", len(calls))
	}
	args, ok := calls[0].([]any)
	if !ok || len(args) != 1 {
		t.Fatalf("unexpected recorded args: %v", calls[0])
	}
	req, ok := args[0].(observer.TracesQueryRequest)
	if !ok {
		t.Fatalf("recorded arg has unexpected type %T", args[0])
	}
	if req.Limit == nil || *req.Limit != maxTraceExportLimit {
		t.Errorf("Limit: got %v, want %d (clamped)", req.Limit, maxTraceExportLimit)
	}
}

// Verifies that get_trace_details accepts only organization + trace_id (no
// project/agent/environment), mirroring REST GetTraceSpans, and forwards the
// omitted scope filters to the observer as nil rather than empty strings.
func TestGetTraceDetailsOptionalScope(t *testing.T) {
	clientSession, fake := setupTestServer(t)
	ctx := context.Background()

	result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
		Name: "get_trace_details",
		Arguments: map[string]any{
			"organization": testOrgName,
			"trace_id":     testTraceID,
		},
	})
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success with only organization + trace_id, got tool error: %+v", result.Content)
	}

	calls := fake.calls["QueryTraceSpans"]
	if len(calls) != 1 {
		t.Fatalf("expected exactly one QueryTraceSpans call, got %d", len(calls))
	}
	args, ok := calls[0].([]any)
	if !ok || len(args) != 2 {
		t.Fatalf("unexpected recorded args: %v", calls[0])
	}
	req, ok := args[1].(observer.TracesQueryRequest)
	if !ok {
		t.Fatalf("recorded arg has unexpected type %T", args[1])
	}
	scope := req.SearchScope
	if scope.Project != nil || scope.Component != nil || scope.Environment != nil {
		t.Errorf("expected nil project/agent/environment, got project=%v agent=%v environment=%v",
			scope.Project, scope.Component, scope.Environment)
	}
}

// Verifies that list_traces enforces the locked 30-day cap on the
// start_time..end_time span (resolveTraceTimeWindow).
func TestTraceTimeWindowCap(t *testing.T) {
	end := time.Now().UTC()

	t.Run("31 days is rejected", func(t *testing.T) {
		clientSession, fake := setupTestServer(t)
		ctx := context.Background()

		start := end.Add(-31 * 24 * time.Hour)
		result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
			Name: "list_traces",
			Arguments: map[string]any{
				"organization": testOrgName,
				"project":      testProjectName,
				"agent":        testAgentName,
				"environment":  testEnvName,
				"start_time":   start.Format(time.RFC3339),
				"end_time":     end.Format(time.RFC3339),
			},
		})
		switch {
		case err != nil:
			// Protocol-level rejection — fine.
		case result != nil && result.IsError:
			// Handler-level tool error — fine.
		default:
			t.Error("expected error for a 31-day window; got success")
		}
		if calls := fake.calls["QueryTraces"]; len(calls) != 0 {
			t.Errorf("expected no QueryTraces call for a rejected request, got %d", len(calls))
		}
	})

	t.Run("29 days succeeds", func(t *testing.T) {
		clientSession, fake := setupTestServer(t)
		ctx := context.Background()

		start := end.Add(-29 * 24 * time.Hour)
		result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
			Name: "list_traces",
			Arguments: map[string]any{
				"organization": testOrgName,
				"project":      testProjectName,
				"agent":        testAgentName,
				"environment":  testEnvName,
				"start_time":   start.Format(time.RFC3339),
				"end_time":     end.Format(time.RFC3339),
			},
		})
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected success, got tool error: %+v", result.Content)
		}
		if calls := fake.calls["QueryTraces"]; len(calls) != 1 {
			t.Errorf("expected exactly one QueryTraces call, got %d", len(calls))
		}
	})
}
