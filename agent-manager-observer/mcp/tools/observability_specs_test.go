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
	"testing"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

// Returns the test specs for tools registered by registerObservabilityTools.
// New tools added to observability.go must have a spec here — registration_test.go fails otherwise.
func observabilityToolSpecs() []toolTestSpec {
	return []toolTestSpec{
		{
			name:                "get_runtime_logs",
			descriptionKeywords: []string{"log"},
			descriptionMinLen:   20,
			requiredParams:      []string{"organization", "project", "agent", "environment"},
			optionalParams:      []string{"start_time", "end_time", "limit", "sort_order", "log_levels", "search_phrase"},
			testArgs: map[string]any{
				"organization": testOrgName,
				"project":      testProjectName,
				"agent":        testAgentName,
				"environment":  testEnvName,
			},
		},
		{
			name:                "get_build_logs",
			descriptionKeywords: []string{"build"},
			descriptionMinLen:   20,
			requiredParams:      []string{"organization", "build_name"},
			testArgs: map[string]any{
				"organization": testOrgName,
				"build_name":   testBuildName,
			},
		},
		{
			name:                "get_metrics",
			descriptionKeywords: []string{"metric"},
			descriptionMinLen:   20,
			requiredParams:      []string{"organization", "project", "agent", "environment"},
			optionalParams:      []string{"start_time", "end_time"},
			testArgs: map[string]any{
				"organization": testOrgName,
				"project":      testProjectName,
				"agent":        testAgentName,
				"environment":  testEnvName,
			},
		},
	}
}

// Verifies that log_levels is upper-cased and passed through to the
// observer client, and that an unrecognized level is rejected as a tool
// error rather than silently forwarded upstream.
func TestLogLevelNormalization(t *testing.T) {
	t.Run("lowercase levels are normalized", func(t *testing.T) {
		clientSession, fake := setupTestServer(t)
		ctx := context.Background()

		result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
			Name: "get_runtime_logs",
			Arguments: map[string]any{
				"organization": testOrgName,
				"project":      testProjectName,
				"agent":        testAgentName,
				"environment":  testEnvName,
				"log_levels":   []string{"debug", "Error"},
			},
		})
		if err != nil {
			t.Fatalf("CallTool failed: %v", err)
		}
		if result.IsError {
			t.Fatalf("expected success, got tool error: %+v", result.Content)
		}

		calls := fake.calls["QueryLogs"]
		if len(calls) != 1 {
			t.Fatalf("expected exactly one QueryLogs call, got %d", len(calls))
		}
		args, ok := calls[0].([]any)
		if !ok || len(args) != 1 {
			t.Fatalf("unexpected recorded args: %v", calls[0])
		}
		req, ok := args[0].(observer.LogsQueryRequest)
		if !ok {
			t.Fatalf("recorded arg has unexpected type %T", args[0])
		}
		want := []string{"DEBUG", "ERROR"}
		if len(req.LogLevels) != len(want) {
			t.Fatalf("LogLevels: got %v, want %v", req.LogLevels, want)
		}
		for i, lvl := range want {
			if req.LogLevels[i] != lvl {
				t.Errorf("LogLevels[%d]: got %q, want %q", i, req.LogLevels[i], lvl)
			}
		}
	})

	t.Run("unrecognized level is a tool error", func(t *testing.T) {
		clientSession, _ := setupTestServer(t)
		ctx := context.Background()

		result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
			Name: "get_runtime_logs",
			Arguments: map[string]any{
				"organization": testOrgName,
				"project":      testProjectName,
				"agent":        testAgentName,
				"environment":  testEnvName,
				"log_levels":   []string{"bogus"},
			},
		})
		switch {
		case err != nil:
			// Protocol-level rejection — fine.
		case result != nil && result.IsError:
			// Handler-level tool error — fine.
		default:
			t.Error("expected error for invalid log level; got success")
		}
	})
}

// Verifies that get_runtime_logs enforces the locked 14-day cap on the
// start_time..end_time span (resolveTimeWindow).
func TestLogsTimeWindowCap(t *testing.T) {
	end := time.Now().UTC()

	t.Run("15 days is rejected", func(t *testing.T) {
		clientSession, fake := setupTestServer(t)
		ctx := context.Background()

		start := end.Add(-15 * 24 * time.Hour)
		result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
			Name: "get_runtime_logs",
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
			t.Error("expected error for a 15-day window; got success")
		}
		if calls := fake.calls["QueryLogs"]; len(calls) != 0 {
			t.Errorf("expected no QueryLogs call for a rejected request, got %d", len(calls))
		}
	})

	t.Run("13 days succeeds", func(t *testing.T) {
		clientSession, fake := setupTestServer(t)
		ctx := context.Background()

		start := end.Add(-13 * 24 * time.Hour)
		result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
			Name: "get_runtime_logs",
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
		if calls := fake.calls["QueryLogs"]; len(calls) != 1 {
			t.Errorf("expected exactly one QueryLogs call, got %d", len(calls))
		}
	})
}

// Verifies that get_runtime_logs rejects a start_time in the future,
// mirroring validateLogTimeRange in handlers/handlers.go (GetLogs).
func TestGetRuntimeLogsFutureStartTimeRejected(t *testing.T) {
	clientSession, fake := setupTestServer(t)
	ctx := context.Background()

	now := time.Now().UTC()
	start := now.Add(1 * time.Hour)
	end := now.Add(2 * time.Hour)

	result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
		Name: "get_runtime_logs",
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
		t.Error("expected error for a future start_time; got success")
	}
	if calls := fake.calls["QueryLogs"]; len(calls) != 0 {
		t.Errorf("expected no QueryLogs call for a rejected request, got %d", len(calls))
	}
}

// Verifies that get_metrics does NOT reject a future start_time: REST's
// GetMetrics handler (handlers/handlers.go) never calls validateLogTimeRange
// — that check is unique to GetLogs — so get_metrics must not inherit it via
// the resolveTimeWindow helper it shares with get_runtime_logs.
func TestGetMetricsAllowsFutureStartTime(t *testing.T) {
	clientSession, fake := setupTestServer(t)
	ctx := context.Background()

	now := time.Now().UTC()
	start := now.Add(1 * time.Hour)
	end := now.Add(2 * time.Hour)

	result, err := clientSession.CallTool(ctx, &gomcp.CallToolParams{
		Name: "get_metrics",
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
		t.Fatalf("expected success for a future start_time on get_metrics, got tool error: %+v", result.Content)
	}
	if len(fake.calls["QueryMetrics"]) == 0 {
		t.Error("expected the future window to reach upstream QueryMetrics")
	}
}
