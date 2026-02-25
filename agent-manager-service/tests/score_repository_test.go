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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
)

// ─── helpers ──────────────────────────────────────────────────────────────────

func float64Ptr(f float64) *float64 { return &f }
func strPtr(s string) *string       { return &s }

// seedRunEvaluator creates the monitor → monitor_run → monitor_run_evaluator chain
// needed as a foreign-key prerequisite for scores. It registers cleanup automatically.
func seedRunEvaluator(t *testing.T) (runEvaluatorID, monitorID uuid.UUID) {
	t.Helper()
	gdb := db.DB(context.Background())

	monitor := &models.Monitor{
		ID:              uuid.New(),
		Name:            "score-repo-test-" + uuid.New().String()[:8],
		DisplayName:     "Score Repo Test Monitor",
		Type:            models.MonitorTypePast,
		OrgName:         "test-org",
		ProjectName:     "test-project",
		AgentName:       "test-agent",
		AgentID:         uuid.New().String(),
		EnvironmentName: "default",
		EnvironmentID:   uuid.New().String(),
		Evaluators:      []models.MonitorEvaluator{},
		SamplingRate:    1.0,
	}
	require.NoError(t, gdb.Create(monitor).Error)

	run := &models.MonitorRun{
		ID:         uuid.New(),
		MonitorID:  monitor.ID,
		Name:       "test-run-" + uuid.New().String()[:8],
		Evaluators: []models.MonitorEvaluator{},
		TraceStart: time.Now().Add(-1 * time.Hour),
		TraceEnd:   time.Now(),
		Status:     models.RunStatusPending,
	}
	require.NoError(t, gdb.Create(run).Error)

	evaluator := &models.MonitorRunEvaluator{
		ID:            uuid.New(),
		MonitorRunID:  run.ID,
		MonitorID:     monitor.ID,
		EvaluatorName: "latency",
		DisplayName:   "Latency Check",
		Level:         "trace",
		Aggregations:  map[string]interface{}{},
	}
	require.NoError(t, gdb.Create(evaluator).Error)

	t.Cleanup(func() {
		gdb.Where("run_evaluator_id = ?", evaluator.ID).Delete(&models.Score{})
		gdb.Delete(evaluator)
		gdb.Delete(run)
		gdb.Delete(monitor)
	})

	return evaluator.ID, monitor.ID
}

// ─── tests ────────────────────────────────────────────────────────────────────

// TestBatchCreateScores_NullSpanID verifies that trace-level scores (span_id = NULL)
// are correctly upserted via the uq_score_per_item NULLS NOT DISTINCT unique index and
// that re-inserting the same (run_evaluator_id, trace_id) updates the row instead of
// producing a duplicate or a constraint error.
func TestBatchCreateScores_NullSpanID(t *testing.T) {
	runEvaluatorID, monitorID := seedRunEvaluator(t)
	repo := repositories.NewScoreRepo(db.DB(context.Background()))

	traceID := "trace-" + uuid.New().String()[:16]
	ts := time.Now().Truncate(time.Millisecond)

	initial := []models.Score{{
		ID:             uuid.New(),
		RunEvaluatorID: runEvaluatorID,
		MonitorID:      monitorID,
		TraceID:        traceID,
		SpanID:         nil,
		Score:          float64Ptr(0.8),
		Explanation:    strPtr("initial"),
		TraceTimestamp: ts,
	}}

	require.NoError(t, repo.BatchCreateScores(initial), "insert with null span_id should succeed")

	var got models.Score
	require.NoError(t, db.DB(context.Background()).
		Where("run_evaluator_id = ? AND trace_id = ?", runEvaluatorID, traceID).
		First(&got).Error)
	assert.Nil(t, got.SpanID)
	assert.InDelta(t, 0.8, *got.Score, 1e-9)

	// Re-insert same key with a new score — must update, not error or duplicate.
	initial[0].Score = float64Ptr(0.5)
	initial[0].Explanation = strPtr("updated")
	require.NoError(t, repo.BatchCreateScores(initial), "upsert with null span_id should succeed")

	require.NoError(t, db.DB(context.Background()).
		Where("run_evaluator_id = ? AND trace_id = ?", runEvaluatorID, traceID).
		First(&got).Error)
	assert.InDelta(t, 0.5, *got.Score, 1e-9)

	var count int64
	db.DB(context.Background()).Model(&models.Score{}).
		Where("run_evaluator_id = ? AND trace_id = ?", runEvaluatorID, traceID).
		Count(&count)
	assert.Equal(t, int64(1), count, "upsert must not produce duplicate rows")
}

// TestBatchCreateScores_NonNullSpanID verifies that span-level scores (span_id != NULL)
// are correctly upserted via the uq_score_per_item NULLS NOT DISTINCT unique index.
func TestBatchCreateScores_NonNullSpanID(t *testing.T) {
	runEvaluatorID, monitorID := seedRunEvaluator(t)
	repo := repositories.NewScoreRepo(db.DB(context.Background()))

	traceID := "trace-" + uuid.New().String()[:16]
	spanID := "span-abc-001"
	ts := time.Now().Truncate(time.Millisecond)

	initial := []models.Score{{
		ID:             uuid.New(),
		RunEvaluatorID: runEvaluatorID,
		MonitorID:      monitorID,
		TraceID:        traceID,
		SpanID:         strPtr(spanID),
		Score:          float64Ptr(1.0),
		Explanation:    strPtr("span score"),
		TraceTimestamp: ts,
	}}

	require.NoError(t, repo.BatchCreateScores(initial), "insert with non-null span_id should succeed")

	var got models.Score
	require.NoError(t, db.DB(context.Background()).
		Where("run_evaluator_id = ? AND trace_id = ? AND span_id = ?", runEvaluatorID, traceID, spanID).
		First(&got).Error)
	assert.Equal(t, spanID, *got.SpanID)
	assert.InDelta(t, 1.0, *got.Score, 1e-9)

	// Upsert: same (run_evaluator_id, trace_id, span_id) → update.
	initial[0].Score = float64Ptr(0.7)
	require.NoError(t, repo.BatchCreateScores(initial), "upsert with non-null span_id should succeed")

	require.NoError(t, db.DB(context.Background()).
		Where("run_evaluator_id = ? AND trace_id = ? AND span_id = ?", runEvaluatorID, traceID, spanID).
		First(&got).Error)
	assert.InDelta(t, 0.7, *got.Score, 1e-9)

	var count int64
	db.DB(context.Background()).Model(&models.Score{}).
		Where("run_evaluator_id = ? AND trace_id = ?", runEvaluatorID, traceID).
		Count(&count)
	assert.Equal(t, int64(1), count, "upsert must not produce duplicate rows")
}

// TestBatchCreateScores_Mixed verifies that a single batch containing both NULL and
// non-NULL span_ids inserts all rows without constraint errors.
func TestBatchCreateScores_Mixed(t *testing.T) {
	runEvaluatorID, monitorID := seedRunEvaluator(t)
	repo := repositories.NewScoreRepo(db.DB(context.Background()))

	traceID1 := "trace-" + uuid.New().String()[:16]
	traceID2 := "trace-" + uuid.New().String()[:16]
	spanID := "span-xyz-002"
	ts := time.Now().Truncate(time.Millisecond)

	mixed := []models.Score{
		{
			ID:             uuid.New(),
			RunEvaluatorID: runEvaluatorID,
			MonitorID:      monitorID,
			TraceID:        traceID1,
			SpanID:         nil, // trace-level
			Score:          float64Ptr(0.9),
			TraceTimestamp: ts,
		},
		{
			ID:             uuid.New(),
			RunEvaluatorID: runEvaluatorID,
			MonitorID:      monitorID,
			TraceID:        traceID2,
			SpanID:         strPtr(spanID), // span-level
			Score:          float64Ptr(0.6),
			TraceTimestamp: ts,
		},
	}

	require.NoError(t, repo.BatchCreateScores(mixed), "mixed batch insert should succeed")

	var count int64
	db.DB(context.Background()).Model(&models.Score{}).
		Where("run_evaluator_id = ?", runEvaluatorID).
		Count(&count)
	assert.Equal(t, int64(2), count)
}

// TestBatchCreateScores_SkippedScore verifies that a score with no numeric value
// (skipped case, score = NULL) can be inserted alongside normal scores.
func TestBatchCreateScores_SkippedScore(t *testing.T) {
	runEvaluatorID, monitorID := seedRunEvaluator(t)
	repo := repositories.NewScoreRepo(db.DB(context.Background()))

	traceID := "trace-" + uuid.New().String()[:16]
	ts := time.Now().Truncate(time.Millisecond)
	errMsg := "evaluation skipped: missing data"

	scores := []models.Score{{
		ID:             uuid.New(),
		RunEvaluatorID: runEvaluatorID,
		MonitorID:      monitorID,
		TraceID:        traceID,
		SpanID:         nil,
		Score:          nil, // NULL score for skipped case
		SkipReason:     strPtr(errMsg),
		TraceTimestamp: ts,
	}}

	require.NoError(t, repo.BatchCreateScores(scores), "skipped score with null score value should succeed")

	var got models.Score
	require.NoError(t, db.DB(context.Background()).
		Where("run_evaluator_id = ? AND trace_id = ?", runEvaluatorID, traceID).
		First(&got).Error)
	assert.Nil(t, got.Score)
	assert.Equal(t, errMsg, *got.SkipReason)
}
