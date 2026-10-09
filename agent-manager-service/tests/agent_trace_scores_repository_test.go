//go:build integration

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

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const (
	traceScoresOrg     = "test-org"
	traceScoresProject = "test-project"
	latencyEval        = "Latency Check"
	hallucinationEval  = "Hallucination"
)

var traceScoresBase = time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

// wantTraceScore is one expected row of GetAgentTraceScores.
type wantTraceScore struct {
	traceID      string
	mean         *float64
	totalCount   int
	skippedCount int
}

// seedAgentTraceScores seeds one monitor for a fresh agent with two evaluators and returns the agent name:
//
//	trace-A at +0m:  Latency 0.8, Hallucination 0.4
//	trace-B at +10m: Latency 0.6, Hallucination skipped
//	trace-C at +20m: Latency 0.2
//	trace-D at +30m: Hallucination 1.0
func seedAgentTraceScores(t *testing.T) string {
	t.Helper()
	gdb := db.DB(context.Background())
	agentName := "trace-scores-agent-" + uuid.New().String()[:8]

	monitor := &models.Monitor{
		ID:              uuid.New(),
		Name:            "trace-scores-" + uuid.New().String()[:8],
		DisplayName:     "Trace Scores Monitor",
		Type:            models.MonitorTypePast,
		OUID:            traceScoresOrg,
		ProjectName:     traceScoresProject,
		AgentName:       agentName,
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
		Name:       "trace-scores-run-" + uuid.New().String()[:8],
		Evaluators: []models.MonitorEvaluator{},
		TraceStart: traceScoresBase.Add(-time.Hour),
		TraceEnd:   traceScoresBase.Add(time.Hour),
		Status:     models.RunStatusPending,
	}
	require.NoError(t, gdb.Create(run).Error)

	evaluatorIDs := map[string]uuid.UUID{}
	for _, name := range []string{latencyEval, hallucinationEval} {
		evaluator := &models.MonitorRunEvaluator{
			ID:            uuid.New(),
			MonitorRunID:  run.ID,
			MonitorID:     monitor.ID,
			Identifier:    name,
			EvaluatorName: name,
			Level:         "trace",
			Aggregations:  map[string]interface{}{},
		}
		require.NoError(t, gdb.Create(evaluator).Error)
		evaluatorIDs[name] = evaluator.ID
	}

	score := func(evaluator, traceID string, minutes int, value *float64, skipReason *string) models.Score {
		return models.Score{
			ID:             uuid.New(),
			RunEvaluatorID: evaluatorIDs[evaluator],
			MonitorID:      monitor.ID,
			TraceID:        traceID,
			Score:          value,
			SkipReason:     skipReason,
			TraceStartTime: traceScoresBase.Add(time.Duration(minutes) * time.Minute),
		}
	}
	repo := repositories.NewScoreRepo(gdb)
	require.NoError(t, repo.BatchCreateScores([]models.Score{
		score(latencyEval, "trace-A", 0, float64Ptr(0.8), nil),
		score(hallucinationEval, "trace-A", 0, float64Ptr(0.4), nil),
		score(latencyEval, "trace-B", 10, float64Ptr(0.6), nil),
		score(hallucinationEval, "trace-B", 10, nil, strPtr("no output")),
		score(latencyEval, "trace-C", 20, float64Ptr(0.2), nil),
		score(hallucinationEval, "trace-D", 30, float64Ptr(1.0), nil),
	}))

	t.Cleanup(func() {
		gdb.Where("monitor_id = ?", monitor.ID).Delete(&models.Score{})
		gdb.Where("monitor_id = ?", monitor.ID).Delete(&models.MonitorRunEvaluator{})
		gdb.Delete(run)
		gdb.Delete(monitor)
	})
	return agentName
}

// getAgentTraceScores runs the repository query over the seeded window.
func getAgentTraceScores(
	t *testing.T, agentName string, limit, offset int, sortOrder string, filters repositories.AgentTraceScoreFilters,
) ([]repositories.TraceAggregation, int) {
	t.Helper()
	repo := repositories.NewScoreRepo(db.DB(context.Background()))
	rows, total, err := repo.GetAgentTraceScores(traceScoresOrg, traceScoresProject, agentName,
		traceScoresBase.Add(-time.Hour), traceScoresBase.Add(time.Hour), limit, offset, sortOrder, filters)
	require.NoError(t, err)
	return rows, total
}

func assertTraceScores(t *testing.T, want []wantTraceScore, got []repositories.TraceAggregation) {
	t.Helper()
	require.Len(t, got, len(want))
	for i, w := range want {
		assert.Equal(t, w.traceID, got[i].TraceID, "row %d", i)
		assert.Equal(t, w.totalCount, got[i].TotalCount, "totalCount of %s", w.traceID)
		assert.Equal(t, w.skippedCount, got[i].SkippedCount, "skippedCount of %s", w.traceID)
		if w.mean == nil {
			assert.Nil(t, got[i].MeanScore, "mean of %s", w.traceID)
			continue
		}
		require.NotNil(t, got[i].MeanScore, "mean of %s", w.traceID)
		assert.InDelta(t, *w.mean, *got[i].MeanScore, 1e-9, "mean of %s", w.traceID)
	}
}

func TestGetAgentTraceScores_NoFiltersReturnsEveryScoredTrace(t *testing.T) {
	agentName := seedAgentTraceScores(t)

	rows, total := getAgentTraceScores(t, agentName, 100, 0, "desc", repositories.AgentTraceScoreFilters{})

	assert.Equal(t, 4, total)
	assertTraceScores(t, []wantTraceScore{
		{traceID: "trace-D", mean: float64Ptr(1.0), totalCount: 1},
		{traceID: "trace-C", mean: float64Ptr(0.2), totalCount: 1},
		{traceID: "trace-B", mean: float64Ptr(0.6), totalCount: 2, skippedCount: 1},
		{traceID: "trace-A", mean: float64Ptr(0.6), totalCount: 2},
	}, rows)
}

func TestGetAgentTraceScores_TraceIDsLimitRows(t *testing.T) {
	agentName := seedAgentTraceScores(t)

	rows, total := getAgentTraceScores(t, agentName, 100, 0, "desc",
		repositories.AgentTraceScoreFilters{TraceIDs: []string{"trace-A", "trace-C", "trace-unknown"}})

	assert.Equal(t, 2, total, "an unknown ID is not counted")
	assertTraceScores(t, []wantTraceScore{
		{traceID: "trace-C", mean: float64Ptr(0.2), totalCount: 1},
		{traceID: "trace-A", mean: float64Ptr(0.6), totalCount: 2},
	}, rows)
}

func TestGetAgentTraceScores_TraceIDsPageWithLimitAndOffset(t *testing.T) {
	agentName := seedAgentTraceScores(t)

	rows, total := getAgentTraceScores(t, agentName, 1, 1, "asc",
		repositories.AgentTraceScoreFilters{TraceIDs: []string{"trace-A", "trace-B", "trace-C"}})

	assert.Equal(t, 3, total)
	assertTraceScores(t, []wantTraceScore{
		{traceID: "trace-B", mean: float64Ptr(0.6), totalCount: 2, skippedCount: 1},
	}, rows)
}

func TestGetAgentTraceScores_EvaluatorRestrictsMean(t *testing.T) {
	agentName := seedAgentTraceScores(t)

	rows, total := getAgentTraceScores(t, agentName, 100, 0, "desc",
		repositories.AgentTraceScoreFilters{EvaluatorName: hallucinationEval})

	assert.Equal(t, 3, total, "trace-C has no Hallucination rows")
	assertTraceScores(t, []wantTraceScore{
		{traceID: "trace-D", mean: float64Ptr(1.0), totalCount: 1},
		{traceID: "trace-B", mean: nil, totalCount: 1, skippedCount: 1},
		{traceID: "trace-A", mean: float64Ptr(0.4), totalCount: 1},
	}, rows)
}

func TestGetAgentTraceScores_SkippedRowsCountedButNotAveraged(t *testing.T) {
	agentName := seedAgentTraceScores(t)

	rows, total := getAgentTraceScores(t, agentName, 100, 0, "desc",
		repositories.AgentTraceScoreFilters{TraceIDs: []string{"trace-B"}})
	assert.Equal(t, 1, total)
	assertTraceScores(t, []wantTraceScore{
		{traceID: "trace-B", mean: float64Ptr(0.6), totalCount: 2, skippedCount: 1},
	}, rows)
}

func TestGetAgentTraceScores_TraceIDsAndEvaluatorCombine(t *testing.T) {
	agentName := seedAgentTraceScores(t)

	rows, total := getAgentTraceScores(t, agentName, 100, 0, "desc",
		repositories.AgentTraceScoreFilters{TraceIDs: []string{"trace-B", "trace-C", "trace-D"}, EvaluatorName: latencyEval})

	assert.Equal(t, 2, total, "trace-D has no Latency Check rows")
	assertTraceScores(t, []wantTraceScore{
		{traceID: "trace-C", mean: float64Ptr(0.2), totalCount: 1},
		{traceID: "trace-B", mean: float64Ptr(0.6), totalCount: 1},
	}, rows)
}

func TestGetAgentTraceScores_UnknownEvaluatorReturnsNothing(t *testing.T) {
	agentName := seedAgentTraceScores(t)

	rows, total := getAgentTraceScores(t, agentName, 100, 0, "desc",
		repositories.AgentTraceScoreFilters{EvaluatorName: "No Such Evaluator"})

	assert.Equal(t, 0, total)
	assert.Empty(t, rows)
}
