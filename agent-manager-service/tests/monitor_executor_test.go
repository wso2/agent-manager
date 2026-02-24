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

package tests

import (
	"context"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/clientmocks"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
)

// realEvaluators returns a realistic set of evaluators spanning all levels (trace, agent, span)
// with varied config shapes, including arrays and nested booleans.
func realEvaluators() []models.MonitorEvaluator {
	return []models.MonitorEvaluator{
		{Identifier: "latency", DisplayName: "Latency Check", Config: map[string]interface{}{"level": "trace", "max_latency_ms": float64(3000), "use_task_constraint": false}},
		{Identifier: "iteration_count", DisplayName: "Iteration Count", Config: map[string]interface{}{"level": "trace", "max_iterations": float64(5), "use_context_constraint": false}},
		{Identifier: "token_efficiency", DisplayName: "Token Efficiency", Config: map[string]interface{}{"level": "trace", "max_tokens": float64(4000), "use_context_constraint": false}},
		{Identifier: "answer_relevancy", DisplayName: "Answer Relevancy", Config: map[string]interface{}{"level": "trace", "min_overlap_ratio": 0.2}},
		{Identifier: "prohibited_content", DisplayName: "Prohibited Content", Config: map[string]interface{}{
			"level":                  "trace",
			"case_sensitive":         false,
			"prohibited_strings":     []interface{}{"internal error", "stack trace", "debug:", "hotels"},
			"use_context_prohibited": false,
		}},
		{Identifier: "answer_length", DisplayName: "Answer Length", Config: map[string]interface{}{"level": "trace", "max_length": float64(5000), "min_length": float64(10)}},
		{Identifier: "latency", DisplayName: "Agent Latency", Config: map[string]interface{}{"level": "agent", "max_latency_ms": float64(5000), "use_task_constraint": true}},
		{Identifier: "latency", DisplayName: "Span Latency", Config: map[string]interface{}{"level": "span", "max_latency_ms": float64(1000), "use_task_constraint": true}},
	}
}

// seedMonitor creates a monitor row in the DB that satisfies FK constraints for monitor_runs.
func seedMonitor(t *testing.T) *models.Monitor {
	t.Helper()
	gdb := db.DB(context.Background())

	evaluators := realEvaluators()
	monitor := &models.Monitor{
		ID:              uuid.New(),
		Name:            "exec-test-" + uuid.New().String()[:8],
		DisplayName:     "Executor Test Monitor",
		Type:            models.MonitorTypePast,
		OrgName:         "test-org",
		ProjectName:     "test-project",
		AgentName:       "test-agent",
		AgentID:         "agent-uuid-123",
		EnvironmentName: "default",
		EnvironmentID:   "env-uuid-456",
		Evaluators:      evaluators,
		SamplingRate:    0.8,
	}
	require.NoError(t, gdb.Create(monitor).Error)
	// GORM's serializer:json may mutate the slice in-place after Create; restore with a fresh copy
	monitor.Evaluators = realEvaluators()

	t.Cleanup(func() {
		gdb.Where("monitor_id = ?", monitor.ID).Delete(&models.MonitorRun{})
		gdb.Delete(monitor)
	})

	return monitor
}

// TestExecuteMonitorRun_CRStructure verifies that the WorkflowRun CR submitted to
// ApplyResource has the correct structure, metadata, labels, and parameters.
func TestExecuteMonitorRun_CRStructure(t *testing.T) {
	monitor := seedMonitor(t)

	var capturedCR map[string]interface{}
	mockClient := &clientmocks.OpenChoreoClientMock{
		ApplyResourceFunc: func(ctx context.Context, body map[string]interface{}) error {
			capturedCR = body
			return nil
		},
		DeleteResourceFunc: func(ctx context.Context, body map[string]interface{}) error {
			return nil
		},
	}

	executor := services.NewMonitorExecutor(mockClient, slog.Default(), repositories.NewMonitorRepo(db.GetDB()))

	startTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	endTime := time.Date(2026, 1, 15, 11, 0, 0, 0, time.UTC)

	result, err := executor.ExecuteMonitorRun(context.Background(), services.ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  startTime,
		EndTime:    endTime,
		Evaluators: monitor.Evaluators,
	})
	require.NoError(t, err)
	require.NotNil(t, result)
	require.NotNil(t, capturedCR)

	// --- Top-level fields ---
	assert.Equal(t, "openchoreo.dev/v1alpha1", capturedCR["apiVersion"])
	assert.Equal(t, "WorkflowRun", capturedCR["kind"])

	// --- Metadata ---
	metadata := capturedCR["metadata"].(map[string]interface{})
	assert.Equal(t, monitor.OrgName, metadata["namespace"])
	assert.NotEmpty(t, metadata["name"])

	labels := metadata["labels"].(map[string]interface{})
	assert.Equal(t, "monitor", labels["amp.wso2.com/resource-type"])
	assert.Equal(t, monitor.AgentName, labels["amp.wso2.com/agent-name"])

	annotations := metadata["annotations"].(map[string]interface{})
	assert.Equal(t, monitor.DisplayName, annotations["amp.wso2.com/display-name"])

	// --- Spec / Workflow ---
	spec := capturedCR["spec"].(map[string]interface{})
	workflow := spec["workflow"].(map[string]interface{})
	assert.Equal(t, "monitor-evaluation-workflow", workflow["name"])

	params := workflow["parameters"].(map[string]interface{})

	// Monitor params
	monitorParams := params["monitor"].(map[string]interface{})
	assert.Equal(t, monitor.Name, monitorParams["name"])
	assert.Equal(t, monitor.DisplayName, monitorParams["displayName"])

	// Agent params
	agentParams := params["agent"].(map[string]interface{})
	assert.Equal(t, monitor.AgentID, agentParams["id"])

	// Environment params
	envParams := params["environment"].(map[string]interface{})
	assert.Equal(t, monitor.EnvironmentID, envParams["id"])

	// Evaluation params
	evalParams := params["evaluation"].(map[string]interface{})
	assert.Equal(t, monitor.SamplingRate, evalParams["samplingRate"])
	assert.Equal(t, "2026-01-15T10:00:00Z", evalParams["traceStart"])
	assert.Equal(t, "2026-01-15T11:00:00Z", evalParams["traceEnd"])

	// Publishing params
	pubParams := params["publishing"].(map[string]interface{})
	assert.Equal(t, monitor.ID.String(), pubParams["monitorId"])
	assert.NotEmpty(t, pubParams["runId"])

	// Verify the run ID in publishing matches the DB record
	assert.Equal(t, result.Run.ID.String(), pubParams["runId"])
}

// TestExecuteMonitorRun_EvaluatorsJSON verifies that the evaluators are serialized as a
// JSON string in the CR and that the full evaluator data (identifiers, display names,
// levels, and configs including arrays) round-trips correctly.
func TestExecuteMonitorRun_EvaluatorsJSON(t *testing.T) {
	monitor := seedMonitor(t)

	var capturedCR map[string]interface{}
	mockClient := &clientmocks.OpenChoreoClientMock{
		ApplyResourceFunc: func(ctx context.Context, body map[string]interface{}) error {
			capturedCR = body
			return nil
		},
		DeleteResourceFunc: func(ctx context.Context, body map[string]interface{}) error {
			return nil
		},
	}

	executor := services.NewMonitorExecutor(mockClient, slog.Default(), repositories.NewMonitorRepo(db.GetDB()))

	result, err := executor.ExecuteMonitorRun(context.Background(), services.ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  time.Now().Add(-1 * time.Hour),
		EndTime:    time.Now(),
		Evaluators: monitor.Evaluators,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Extract the evaluators JSON string from the CR
	spec := capturedCR["spec"].(map[string]interface{})
	workflow := spec["workflow"].(map[string]interface{})
	params := workflow["parameters"].(map[string]interface{})
	evalParams := params["evaluation"].(map[string]interface{})

	evaluatorsStr, ok := evalParams["evaluators"].(string)
	require.True(t, ok, "evaluators should be a JSON string")

	// The serialized format merges "level" into config for the amp-evaluation SDK.
	// Deserialize into the eval-job format (no top-level Level field).
	type EvalJobEvaluator struct {
		Identifier  string                 `json:"identifier"`
		DisplayName string                 `json:"displayName"`
		Config      map[string]interface{} `json:"config"`
	}

	var evaluators []EvalJobEvaluator
	require.NoError(t, json.Unmarshal([]byte(evaluatorsStr), &evaluators))
	require.Len(t, evaluators, 8)

	// Verify all levels are represented (level is inside config)
	levels := map[string]int{}
	for _, e := range evaluators {
		lvl, ok := e.Config["level"].(string)
		require.True(t, ok, "config.level should be a string for %s", e.DisplayName)
		levels[lvl]++
	}
	assert.Equal(t, 6, levels["trace"])
	assert.Equal(t, 1, levels["agent"])
	assert.Equal(t, 1, levels["span"])

	// Verify a specific evaluator with simple config
	latencyCheck := evaluators[0]
	assert.Equal(t, "latency", latencyCheck.Identifier)
	assert.Equal(t, "Latency Check", latencyCheck.DisplayName)
	assert.Equal(t, "trace", latencyCheck.Config["level"])
	assert.Equal(t, float64(3000), latencyCheck.Config["max_latency_ms"])
	assert.Equal(t, false, latencyCheck.Config["use_task_constraint"])

	// Verify evaluator with array config (prohibited_content)
	prohibitedContent := evaluators[4]
	assert.Equal(t, "prohibited_content", prohibitedContent.Identifier)
	assert.Equal(t, "Prohibited Content", prohibitedContent.DisplayName)
	assert.Equal(t, "trace", prohibitedContent.Config["level"])
	prohibitedStrings, ok := prohibitedContent.Config["prohibited_strings"].([]interface{})
	require.True(t, ok, "prohibited_strings should be an array")
	assert.Len(t, prohibitedStrings, 4)
	assert.Contains(t, prohibitedStrings, "internal error")
	assert.Contains(t, prohibitedStrings, "stack trace")
	assert.Contains(t, prohibitedStrings, "debug:")
	assert.Contains(t, prohibitedStrings, "hotels")

	// Verify same identifier with different display names across levels
	agentLatency := evaluators[6]
	assert.Equal(t, "latency", agentLatency.Identifier)
	assert.Equal(t, "Agent Latency", agentLatency.DisplayName)
	assert.Equal(t, "agent", agentLatency.Config["level"])
	assert.Equal(t, float64(5000), agentLatency.Config["max_latency_ms"])

	spanLatency := evaluators[7]
	assert.Equal(t, "latency", spanLatency.Identifier)
	assert.Equal(t, "Span Latency", spanLatency.DisplayName)
	assert.Equal(t, "span", spanLatency.Config["level"])
	assert.Equal(t, float64(1000), spanLatency.Config["max_latency_ms"])
}

// TestExecuteMonitorRun_DBRecordCreated verifies that a monitor_runs row is created
// in the database with the correct evaluator snapshot and time range.
func TestExecuteMonitorRun_DBRecordCreated(t *testing.T) {
	monitor := seedMonitor(t)

	mockClient := &clientmocks.OpenChoreoClientMock{
		ApplyResourceFunc: func(ctx context.Context, body map[string]interface{}) error {
			return nil
		},
		DeleteResourceFunc: func(ctx context.Context, body map[string]interface{}) error {
			return nil
		},
	}

	executor := services.NewMonitorExecutor(mockClient, slog.Default(), repositories.NewMonitorRepo(db.GetDB()))

	startTime := time.Now().Add(-2 * time.Hour).Truncate(time.Millisecond)
	endTime := time.Now().Add(-1 * time.Hour).Truncate(time.Millisecond)

	result, err := executor.ExecuteMonitorRun(context.Background(), services.ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  startTime,
		EndTime:    endTime,
		Evaluators: monitor.Evaluators,
	})
	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify DB record
	var run models.MonitorRun
	require.NoError(t, db.DB(context.Background()).
		Where("id = ?", result.Run.ID).
		First(&run).Error)

	assert.Equal(t, monitor.ID, run.MonitorID)
	assert.Equal(t, models.RunStatusPending, run.Status)
	assert.WithinDuration(t, startTime, run.TraceStart, time.Second)
	assert.WithinDuration(t, endTime, run.TraceEnd, time.Second)

	// Verify evaluators are snapshotted in the run
	require.Len(t, run.Evaluators, 8)
	assert.Equal(t, "Latency Check", run.Evaluators[0].DisplayName)
	assert.Equal(t, "Span Latency", run.Evaluators[7].DisplayName)
}

// TestExecuteMonitorRun_NilEvaluatorsReturnsError verifies that calling ExecuteMonitorRun
// with nil evaluators returns an error immediately.
func TestExecuteMonitorRun_NilEvaluatorsReturnsError(t *testing.T) {
	monitor := seedMonitor(t)

	mockClient := &clientmocks.OpenChoreoClientMock{
		ApplyResourceFunc: func(ctx context.Context, body map[string]interface{}) error {
			t.Fatal("ApplyResource should not be called with nil evaluators")
			return nil
		},
	}

	executor := services.NewMonitorExecutor(mockClient, slog.Default(), repositories.NewMonitorRepo(db.GetDB()))

	_, err := executor.ExecuteMonitorRun(context.Background(), services.ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  time.Now().Add(-1 * time.Hour),
		EndTime:    time.Now(),
		Evaluators: nil,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "evaluators must not be empty")
}
