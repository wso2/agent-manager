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

// ─── adaptive time series tests ─────────────────────────────────────────────

// seedScores inserts N scores for the given evaluator with timestamps spread
// across the time range. Returns the time range [start, end] that covers all scores.
func seedScores(t *testing.T, runEvaluatorID, monitorID uuid.UUID, count int, baseTime time.Time, interval time.Duration) {
	t.Helper()
	repo := repositories.NewScoreRepo(db.DB(context.Background()))
	scores := make([]models.Score, count)
	for i := range count {
		scores[i] = models.Score{
			ID:             uuid.New(),
			RunEvaluatorID: runEvaluatorID,
			MonitorID:      monitorID,
			TraceID:        "trace-ts-" + uuid.New().String()[:12],
			Score:          float64Ptr(float64(i%10) / 10.0),
			TraceTimestamp: baseTime.Add(time.Duration(i) * interval),
		}
	}
	require.NoError(t, repo.BatchCreateScores(scores))
}

// TestGetEvaluatorScoreCount verifies the lightweight COUNT query
func TestGetEvaluatorScoreCount(t *testing.T) {
	runEvaluatorID, monitorID := seedRunEvaluator(t)
	repo := repositories.NewScoreRepo(db.DB(context.Background()))

	baseTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	seedScores(t, runEvaluatorID, monitorID, 5, baseTime, 10*time.Minute)

	// Count within the full range
	count, err := repo.GetEvaluatorScoreCount(monitorID, "Latency Check",
		baseTime.Add(-time.Hour), baseTime.Add(2*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(5), count)

	// Count with a narrower range that excludes some scores
	count, err = repo.GetEvaluatorScoreCount(monitorID, "Latency Check",
		baseTime, baseTime.Add(25*time.Minute))
	require.NoError(t, err)
	assert.Equal(t, int64(3), count, "only scores at +0m, +10m, +20m should be in range")

	// Count with a different evaluator name returns 0
	count, err = repo.GetEvaluatorScoreCount(monitorID, "Nonexistent Evaluator",
		baseTime.Add(-time.Hour), baseTime.Add(2*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, int64(0), count)
}

// TestGetEvaluatorTraceAggregated verifies per-trace aggregation
func TestGetEvaluatorTraceAggregated(t *testing.T) {
	runEvaluatorID, monitorID := seedRunEvaluator(t)
	repo := repositories.NewScoreRepo(db.DB(context.Background()))

	baseTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	// Insert 3 scores for 3 different traces
	scores := []models.Score{
		{
			ID:             uuid.New(),
			RunEvaluatorID: runEvaluatorID,
			MonitorID:      monitorID,
			TraceID:        "trace-agg-A",
			Score:          float64Ptr(0.8),
			TraceTimestamp: baseTime,
		},
		{
			ID:             uuid.New(),
			RunEvaluatorID: runEvaluatorID,
			MonitorID:      monitorID,
			TraceID:        "trace-agg-B",
			Score:          float64Ptr(0.6),
			TraceTimestamp: baseTime.Add(30 * time.Minute),
		},
		{
			ID:             uuid.New(),
			RunEvaluatorID: runEvaluatorID,
			MonitorID:      monitorID,
			TraceID:        "trace-agg-C",
			Score:          nil,
			SkipReason:     strPtr("skipped"),
			TraceTimestamp: baseTime.Add(1 * time.Hour),
		},
	}
	require.NoError(t, repo.BatchCreateScores(scores))

	results, err := repo.GetEvaluatorTraceAggregated(monitorID, "Latency Check",
		baseTime.Add(-time.Hour), baseTime.Add(2*time.Hour))
	require.NoError(t, err)
	require.Len(t, results, 3, "should have one result per trace")

	// Results ordered by trace_timestamp
	assert.Equal(t, "trace-agg-A", results[0].TraceID)
	assert.InDelta(t, 0.8, *results[0].MeanScore, 1e-9)
	assert.Equal(t, 1, results[0].TotalCount)
	assert.Equal(t, 0, results[0].SkippedCount)

	assert.Equal(t, "trace-agg-B", results[1].TraceID)
	assert.InDelta(t, 0.6, *results[1].MeanScore, 1e-9)

	// Skipped trace: mean should be NULL, skippedCount = 1
	assert.Equal(t, "trace-agg-C", results[2].TraceID)
	assert.Nil(t, results[2].MeanScore)
	assert.Equal(t, 1, results[2].SkippedCount)
}

// TestGetEvaluatorTimeSeriesAggregated_Minute verifies minute-level bucketing via date_trunc
func TestGetEvaluatorTimeSeriesAggregated_Minute(t *testing.T) {
	runEvaluatorID, monitorID := seedRunEvaluator(t)
	repo := repositories.NewScoreRepo(db.DB(context.Background()))

	// Create scores at: 10:03:10, 10:03:40, 10:05:20
	// Expected minute buckets: [10:03, 10:05]
	baseTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	scores := []models.Score{
		{
			ID: uuid.New(), RunEvaluatorID: runEvaluatorID, MonitorID: monitorID,
			TraceID: "t1", Score: float64Ptr(0.8), TraceTimestamp: baseTime.Add(3*time.Minute + 10*time.Second),
		},
		{
			ID: uuid.New(), RunEvaluatorID: runEvaluatorID, MonitorID: monitorID,
			TraceID: "t2", Score: float64Ptr(0.6), TraceTimestamp: baseTime.Add(3*time.Minute + 40*time.Second),
		},
		{
			ID: uuid.New(), RunEvaluatorID: runEvaluatorID, MonitorID: monitorID,
			TraceID: "t3", Score: float64Ptr(0.9), TraceTimestamp: baseTime.Add(5*time.Minute + 20*time.Second),
		},
	}
	require.NoError(t, repo.BatchCreateScores(scores))

	results, err := repo.GetEvaluatorTimeSeriesAggregated(monitorID, "Latency Check",
		baseTime, baseTime.Add(1*time.Hour), "minute")
	require.NoError(t, err)
	require.Len(t, results, 2, "should have 2 non-empty minute buckets")

	// Bucket 10:03: scores at 10:03:10 (0.8) and 10:03:40 (0.6), mean = 0.7
	assert.True(t, baseTime.Add(3*time.Minute).Equal(results[0].TimeBucket), "bucket 0 time: expected %v, got %v", baseTime.Add(3*time.Minute), results[0].TimeBucket)
	assert.Equal(t, 2, results[0].TotalCount)
	assert.InDelta(t, 0.7, *results[0].MeanScore, 1e-9)

	// Bucket 10:05: score at 10:05:20 (0.9), mean = 0.9
	assert.True(t, baseTime.Add(5*time.Minute).Equal(results[1].TimeBucket), "bucket 1 time: expected %v, got %v", baseTime.Add(5*time.Minute), results[1].TimeBucket)
	assert.Equal(t, 1, results[1].TotalCount)
	assert.InDelta(t, 0.9, *results[1].MeanScore, 1e-9)
}

// TestGetEvaluatorTimeSeriesAggregated_Hour verifies hourly bucketing still works
func TestGetEvaluatorTimeSeriesAggregated_Hour(t *testing.T) {
	runEvaluatorID, monitorID := seedRunEvaluator(t)
	repo := repositories.NewScoreRepo(db.DB(context.Background()))

	// Scores across 3 hours
	baseTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	scores := []models.Score{
		{
			ID: uuid.New(), RunEvaluatorID: runEvaluatorID, MonitorID: monitorID,
			TraceID: "th1", Score: float64Ptr(0.5), TraceTimestamp: baseTime.Add(15 * time.Minute),
		},
		{
			ID: uuid.New(), RunEvaluatorID: runEvaluatorID, MonitorID: monitorID,
			TraceID: "th2", Score: float64Ptr(0.7), TraceTimestamp: baseTime.Add(45 * time.Minute),
		},
		{
			ID: uuid.New(), RunEvaluatorID: runEvaluatorID, MonitorID: monitorID,
			TraceID: "th3", Score: float64Ptr(0.9), TraceTimestamp: baseTime.Add(90 * time.Minute),
		},
	}
	require.NoError(t, repo.BatchCreateScores(scores))

	results, err := repo.GetEvaluatorTimeSeriesAggregated(monitorID, "Latency Check",
		baseTime, baseTime.Add(3*time.Hour), "hour")
	require.NoError(t, err)
	require.Len(t, results, 2, "should have 2 non-empty hour buckets")

	// Hour 10:00: 2 scores, mean = 0.6
	assert.True(t, baseTime.Equal(results[0].TimeBucket), "bucket 0 time: expected %v, got %v", baseTime, results[0].TimeBucket)
	assert.Equal(t, 2, results[0].TotalCount)
	assert.InDelta(t, 0.6, *results[0].MeanScore, 1e-9)

	// Hour 11:00: 1 score, mean = 0.9
	assert.True(t, baseTime.Add(1*time.Hour).Equal(results[1].TimeBucket), "bucket 1 time: expected %v, got %v", baseTime.Add(1*time.Hour), results[1].TimeBucket)
	assert.Equal(t, 1, results[1].TotalCount)
	assert.InDelta(t, 0.9, *results[1].MeanScore, 1e-9)
}
