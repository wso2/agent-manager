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
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// stubScoreRepo is a minimal ScoreRepository that returns "not found" for monitor lookups.
type stubScoreRepo struct {
	evaluators []models.MonitorRunEvaluator
}

func (s *stubScoreRepo) WithTx(_ *gorm.DB) repositories.ScoreRepository { return s }
func (s *stubScoreRepo) RunInTransaction(fn func(txRepo repositories.ScoreRepository) error) error {
	return fn(s)
}

func (s *stubScoreRepo) UpsertMonitorRunEvaluators(evals []models.MonitorRunEvaluator) error {
	s.evaluators = evals
	return nil
}

func (s *stubScoreRepo) GetEvaluatorsByMonitorAndRunID(_, _ uuid.UUID) ([]models.MonitorRunEvaluator, error) {
	return s.evaluators, nil
}
func (s *stubScoreRepo) BatchCreateScores(_ []models.Score) error { return nil }
func (s *stubScoreRepo) DeleteStaleScores(_ uuid.UUID, _ []uuid.UUID, _ []string) error {
	return nil
}

func (s *stubScoreRepo) GetMonitorScoresAggregated(_ uuid.UUID, _, _ time.Time, _ repositories.ScoreFilters) ([]repositories.EvaluatorAggregation, error) {
	return nil, nil
}

func (s *stubScoreRepo) GetEvaluatorTimeSeriesAggregated(_ uuid.UUID, _ string, _, _ time.Time, _ string) ([]repositories.TimeBucketAggregation, error) {
	return nil, nil
}

func (s *stubScoreRepo) GetEvaluatorTraceAggregated(_ uuid.UUID, _ string, _, _ time.Time, _ int) ([]repositories.TraceAggregation, error) {
	return nil, nil
}

func (s *stubScoreRepo) GetScoresGroupedByLabel(_ uuid.UUID, _, _ time.Time, _ string) ([]repositories.LabelAggregation, error) {
	return nil, nil
}

func (s *stubScoreRepo) GetScoresByTraceID(_ string, _, _, _ string) ([]repositories.ScoreWithMonitor, error) {
	return nil, nil
}

func (s *stubScoreRepo) GetAgentTraceScores(_, _, _ string, _, _ time.Time, _, _ int) ([]repositories.TraceAggregation, int, error) {
	return nil, 0, nil
}

func (s *stubScoreRepo) GetMonitorID(_, _, _, _ string) (uuid.UUID, error) {
	return uuid.Nil, gorm.ErrRecordNotFound
}

// newScoresHandler builds a minimal ServeMux wired to a scores controller backed by
// a stub repository that returns "not found" for all monitor lookups.
func newScoresHandler() http.Handler {
	mux := http.NewServeMux()
	svc := services.NewMonitorScoresService(&stubScoreRepo{}, slog.Default())
	ctrl := controllers.NewMonitorScoresController(svc)

	base := "/orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}"
	agentBase := "/orgs/{orgName}/projects/{projName}/agents/{agentName}"

	mux.HandleFunc("GET "+base+"/scores", ctrl.GetMonitorScores)
	mux.HandleFunc("GET "+base+"/scores/breakdown", ctrl.GetGroupedScores)
	mux.HandleFunc("GET "+base+"/scores/timeseries", ctrl.GetScoresTimeSeries)
	mux.HandleFunc("GET "+agentBase+"/traces/{traceId}/scores", ctrl.GetTraceScores)

	return mux
}

// -----------------------------------------------------------------------------
// CalculateAdaptiveGranularity
// -----------------------------------------------------------------------------

func TestCalculateAdaptiveGranularity(t *testing.T) {
	cases := []struct {
		name     string
		duration time.Duration
		count    int64
		want     string
	}{
		// Sparse data (count <= 50) → trace-level aggregation regardless of duration
		{"0 points, 7 days", 7 * 24 * time.Hour, 0, "trace"},
		{"1 point, 7 days", 7 * 24 * time.Hour, 1, "trace"},
		{"50 points, 7 days", 7 * 24 * time.Hour, 50, "trace"},
		{"50 points, 1 hour", time.Hour, 50, "trace"},

		// Dense data (count > 50) → time-bucket granularity based on duration
		{"51 points, 3 hours → minute", 3 * time.Hour, 51, "minute"},
		{"51 points, exactly 6 hours → minute", 6 * time.Hour, 51, "minute"},
		{"51 points, 6h + 1s → hour", 6*time.Hour + time.Second, 51, "hour"},
		{"51 points, 3 days → hour", 3 * 24 * time.Hour, 51, "hour"},
		{"51 points, exactly 7 days → hour", 7 * 24 * time.Hour, 51, "hour"},
		{"51 points, 7 days + 1 sec → day", 7*24*time.Hour + time.Second, 51, "day"},
		{"51 points, 14 days → day", 14 * 24 * time.Hour, 51, "day"},
		{"51 points, exactly 28 days → day", 28 * 24 * time.Hour, 51, "day"},
		{"51 points, 28 days + 1 sec → week", 28*24*time.Hour + time.Second, 51, "week"},
		{"51 points, 60 days → week", 60 * 24 * time.Hour, 51, "week"},
		{"51 points, 100 days → week", 100 * 24 * time.Hour, 51, "week"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, utils.CalculateAdaptiveGranularity(tc.duration, tc.count))
		})
	}
}

// -----------------------------------------------------------------------------
// GET /scores — validation
// -----------------------------------------------------------------------------

func TestGetMonitorScores_Validation(t *testing.T) {
	handler := newScoresHandler()
	base := "/orgs/org1/projects/proj1/agents/agent1/monitors/mon1/scores"

	now := time.Now().UTC()
	validStart := now.Add(-48 * time.Hour).Format(time.RFC3339)
	validEnd := now.Format(time.RFC3339)

	cases := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{
			name:       "missing startTime and endTime",
			query:      "",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing endTime",
			query:      "?startTime=" + validStart,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing startTime",
			query:      "?endTime=" + validEnd,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid startTime format",
			query:      "?startTime=not-a-date&endTime=" + validEnd,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid endTime format",
			query:      "?startTime=" + validStart + "&endTime=not-a-date",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "endTime before startTime",
			query:      "?startTime=" + validEnd + "&endTime=" + validStart,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid level value",
			query:      "?startTime=" + validStart + "&endTime=" + validEnd + "&level=invalid",
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, base+tc.query, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.Equal(t, tc.wantStatus, w.Code)
		})
	}
}

func TestGetMonitorScores_ValidLevel(t *testing.T) {
	handler := newScoresHandler()
	base := "/orgs/org1/projects/proj1/agents/agent1/monitors/mon1/scores"

	now := time.Now().UTC()
	validStart := now.Add(-48 * time.Hour).Format(time.RFC3339)
	validEnd := now.Format(time.RFC3339)

	// Valid level values must pass validation (will 404 from DB, not 400)
	for _, level := range []string{"trace", "agent", "llm"} {
		t.Run("level="+level, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet,
				base+"?startTime="+validStart+"&endTime="+validEnd+"&level="+level, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.NotEqual(t, http.StatusBadRequest, w.Code)
		})
	}
}

// -----------------------------------------------------------------------------
// GET /scores/timeseries — validation + granularity selection
// -----------------------------------------------------------------------------

func TestGetScoresTimeSeries_Validation(t *testing.T) {
	handler := newScoresHandler()
	base := "/orgs/org1/projects/proj1/agents/agent1/monitors/mon1/scores/timeseries"

	now := time.Now().UTC()
	validStart := now.Add(-48 * time.Hour).Format(time.RFC3339)
	validEnd := now.Format(time.RFC3339)

	cases := []struct {
		name       string
		query      string
		wantStatus int
	}{
		{
			name:       "missing startTime and endTime",
			query:      "?evaluator=latency",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "missing evaluator",
			query:      "?startTime=" + validStart + "&endTime=" + validEnd,
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid startTime format",
			query:      "?startTime=bad&endTime=" + validEnd + "&evaluator=latency",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "invalid endTime format",
			query:      "?startTime=" + validStart + "&endTime=bad&evaluator=latency",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "endTime before startTime",
			query:      "?startTime=" + validEnd + "&endTime=" + validStart + "&evaluator=latency",
			wantStatus: http.StatusBadRequest,
		},
		{
			name: "duration exceeds 100 days",
			query: func() string {
				s := now.Add(-101 * 24 * time.Hour).Format(time.RFC3339)
				e := now.Format(time.RFC3339)
				return "?startTime=" + s + "&endTime=" + e + "&evaluator=latency"
			}(),
			wantStatus: http.StatusBadRequest,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, base+tc.query, nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			assert.Equal(t, tc.wantStatus, w.Code)
		})
	}
}

// TestGetScoresTimeSeries_ValidRanges verifies that valid time ranges
// pass all validation checks (not 400). Granularity is now determined
// adaptively by the backend — no client-provided granularity parameter.
func TestGetScoresTimeSeries_ValidRanges(t *testing.T) {
	handler := newScoresHandler()
	base := "/orgs/org1/projects/proj1/agents/agent1/monitors/mon1/scores/timeseries"

	now := time.Now().UTC()

	cases := []struct {
		name     string
		duration time.Duration
	}{
		{"24h", 24 * time.Hour},
		{"2 days", 2 * 24 * time.Hour},
		{"3 days", 3 * 24 * time.Hour},
		{"28 days", 28 * 24 * time.Hour},
		{"29 days", 29 * 24 * time.Hour},
		{"100 days (max allowed)", 100 * 24 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start := now.Add(-tc.duration).Format(time.RFC3339)
			end := now.Format(time.RFC3339)
			req := httptest.NewRequest(http.MethodGet,
				base+"?startTime="+start+"&endTime="+end+"&evaluator=latency", nil)
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)
			// Validation should pass — response will be 404 (no monitor in DB), not 400
			assert.NotEqual(t, http.StatusBadRequest, w.Code,
				"expected valid range to pass validation")
		})
	}
}

// -----------------------------------------------------------------------------
// Service-level adaptive granularity routing
// -----------------------------------------------------------------------------

// configurableScoreRepo extends stubScoreRepo with configurable return values
// for the adaptive granularity and trace scores methods.
type configurableScoreRepo struct {
	stubScoreRepo
	traceAggs       []repositories.TraceAggregation
	timeBucketAggs  []repositories.TimeBucketAggregation
	lastGranularity string // captures the granularity passed to GetEvaluatorTimeSeriesAggregated
	traceScores     []repositories.ScoreWithMonitor
	agentTraceAggs  []repositories.TraceAggregation
}

func (c *configurableScoreRepo) GetEvaluatorTraceAggregated(_ uuid.UUID, _ string, _, _ time.Time, limit int) ([]repositories.TraceAggregation, error) {
	if limit > 0 && len(c.traceAggs) > limit {
		return c.traceAggs[:limit], nil
	}
	return c.traceAggs, nil
}

func (c *configurableScoreRepo) GetEvaluatorTimeSeriesAggregated(_ uuid.UUID, _ string, _, _ time.Time, granularity string) ([]repositories.TimeBucketAggregation, error) {
	c.lastGranularity = granularity
	return c.timeBucketAggs, nil
}

func (c *configurableScoreRepo) GetMonitorID(_, _, _, _ string) (uuid.UUID, error) {
	return uuid.New(), nil // return a valid ID so the service proceeds
}

func (c *configurableScoreRepo) GetScoresByTraceID(_ string, _, _, _ string) ([]repositories.ScoreWithMonitor, error) {
	return c.traceScores, nil
}

func (c *configurableScoreRepo) GetAgentTraceScores(_, _, _ string, _, _ time.Time, _, _ int) ([]repositories.TraceAggregation, int, error) {
	return c.agentTraceAggs, len(c.agentTraceAggs), nil
}

// makeDenseTraceAggs generates n dummy TraceAggregation entries to simulate dense data.
func makeDenseTraceAggs(n int, baseTime time.Time) []repositories.TraceAggregation {
	score := 0.5
	aggs := make([]repositories.TraceAggregation, n)
	for i := range n {
		aggs[i] = repositories.TraceAggregation{
			TraceID:        fmt.Sprintf("dense-t%d", i),
			TraceStartTime: baseTime.Add(time.Duration(i) * time.Minute),
			TotalCount:     1,
			MeanScore:      &score,
		}
	}
	return aggs
}

func TestGetEvaluatorTimeSeries_SparseData_UsesTraceMode(t *testing.T) {
	baseTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	meanScore := 0.85

	repo := &configurableScoreRepo{
		traceAggs: []repositories.TraceAggregation{
			{TraceID: "t1", TraceStartTime: baseTime, TotalCount: 1, SkippedCount: 0, MeanScore: &meanScore},
			{TraceID: "t2", TraceStartTime: baseTime.Add(30 * time.Minute), TotalCount: 1, SkippedCount: 0, MeanScore: &meanScore},
		},
	}
	svc := services.NewMonitorScoresService(repo, slog.Default())

	result, err := svc.GetEvaluatorTimeSeries(
		uuid.New(), "test-monitor", "Latency Check",
		baseTime.Add(-time.Hour), baseTime.Add(7*24*time.Hour),
	)
	require.NoError(t, err)
	assert.Equal(t, "trace", result.Granularity)
	assert.Len(t, result.Points, 2)
	assert.Equal(t, baseTime, result.Points[0].Timestamp)
	assert.Equal(t, 1, result.Points[0].Count)
	assert.InDelta(t, 0.85, result.Points[0].Aggregations["mean"], 1e-9)
}

func TestGetEvaluatorTimeSeries_DenseData_ShortRange_UsesMinute(t *testing.T) {
	baseTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	meanScore := 0.7

	repo := &configurableScoreRepo{
		traceAggs: makeDenseTraceAggs(100, baseTime), // dense: > 50
		timeBucketAggs: []repositories.TimeBucketAggregation{
			{TimeBucket: baseTime, TotalCount: 5, SkippedCount: 0, MeanScore: &meanScore},
		},
	}
	svc := services.NewMonitorScoresService(repo, slog.Default())

	// Time range <= 6 hours → should select "minute"
	result, err := svc.GetEvaluatorTimeSeries(
		uuid.New(), "test-monitor", "Latency Check",
		baseTime, baseTime.Add(4*time.Hour),
	)
	require.NoError(t, err)
	assert.Equal(t, "minute", result.Granularity)
	assert.Equal(t, "minute", repo.lastGranularity)
	assert.Len(t, result.Points, 1)
}

func TestGetEvaluatorTimeSeries_DenseData_MediumRange_UsesHour(t *testing.T) {
	baseTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	meanScore := 0.6

	repo := &configurableScoreRepo{
		traceAggs: makeDenseTraceAggs(200, baseTime), // dense
		timeBucketAggs: []repositories.TimeBucketAggregation{
			{TimeBucket: baseTime, TotalCount: 10, SkippedCount: 1, MeanScore: &meanScore},
		},
	}
	svc := services.NewMonitorScoresService(repo, slog.Default())

	// Time range 3 days (1-7 days) → should select "hour"
	result, err := svc.GetEvaluatorTimeSeries(
		uuid.New(), "test-monitor", "Latency Check",
		baseTime, baseTime.Add(3*24*time.Hour),
	)
	require.NoError(t, err)
	assert.Equal(t, "hour", result.Granularity)
	assert.Equal(t, "hour", repo.lastGranularity)
}

func TestGetEvaluatorTimeSeries_DenseData_LongRange_UsesDay(t *testing.T) {
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	meanScore := 0.5

	repo := &configurableScoreRepo{
		traceAggs: makeDenseTraceAggs(500, baseTime), // dense
		timeBucketAggs: []repositories.TimeBucketAggregation{
			{TimeBucket: baseTime, TotalCount: 50, SkippedCount: 0, MeanScore: &meanScore},
		},
	}
	svc := services.NewMonitorScoresService(repo, slog.Default())

	// Time range 14 days (7-28 days) → should select "day"
	result, err := svc.GetEvaluatorTimeSeries(
		uuid.New(), "test-monitor", "Latency Check",
		baseTime, baseTime.Add(14*24*time.Hour),
	)
	require.NoError(t, err)
	assert.Equal(t, "day", result.Granularity)
	assert.Equal(t, "day", repo.lastGranularity)
}

func TestGetEvaluatorTimeSeries_DenseData_VeryLongRange_UsesWeek(t *testing.T) {
	baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	meanScore := 0.9

	repo := &configurableScoreRepo{
		traceAggs: makeDenseTraceAggs(1000, baseTime), // dense
		timeBucketAggs: []repositories.TimeBucketAggregation{
			{TimeBucket: baseTime, TotalCount: 100, SkippedCount: 5, MeanScore: &meanScore},
		},
	}
	svc := services.NewMonitorScoresService(repo, slog.Default())

	// Time range 60 days (> 28 days) → should select "week"
	result, err := svc.GetEvaluatorTimeSeries(
		uuid.New(), "test-monitor", "Latency Check",
		baseTime, baseTime.Add(60*24*time.Hour),
	)
	require.NoError(t, err)
	assert.Equal(t, "week", result.Granularity)
	assert.Equal(t, "week", repo.lastGranularity)
}

func TestGetEvaluatorTimeSeries_BoundaryAt50(t *testing.T) {
	baseTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	meanScore := 0.75

	// Exactly 50 traces → trace mode (probe returns 50, which is <= RawThreshold)
	repo50 := &configurableScoreRepo{
		traceAggs: makeDenseTraceAggs(50, baseTime),
	}
	// Override first entry with a known score for assertion
	repo50.traceAggs[0] = repositories.TraceAggregation{
		TraceID: "t1", TraceStartTime: baseTime, TotalCount: 1, MeanScore: &meanScore,
	}
	svc50 := services.NewMonitorScoresService(repo50, slog.Default())
	result, err := svc50.GetEvaluatorTimeSeries(uuid.New(), "m", "e", baseTime, baseTime.Add(3*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, "trace", result.Granularity)

	// 51 traces → time-bucket mode (probe returns 51, which is > RawThreshold)
	repo51 := &configurableScoreRepo{
		traceAggs:      makeDenseTraceAggs(51, baseTime),
		timeBucketAggs: []repositories.TimeBucketAggregation{},
	}
	svc51 := services.NewMonitorScoresService(repo51, slog.Default())
	result, err = svc51.GetEvaluatorTimeSeries(uuid.New(), "m", "e", baseTime, baseTime.Add(3*24*time.Hour))
	require.NoError(t, err)
	assert.Equal(t, "hour", result.Granularity)
}

// -----------------------------------------------------------------------------
// GET /traces/{traceId}/scores — validation
// -----------------------------------------------------------------------------

func TestGetTraceScores_EmptyTraceID(t *testing.T) {
	// Call the handler directly with an explicitly empty traceId path value.
	// The router would never produce this (unmatched route → 404), but the
	// handler has an explicit guard that must return 400 for empty traceId.
	ctrl := controllers.NewMonitorScoresController(nil)

	req := httptest.NewRequest(http.MethodGet,
		"/orgs/org1/projects/proj1/agents/agent1/traces//scores", nil)
	req.SetPathValue("orgName", "org1")
	req.SetPathValue("agentName", "agent1")
	req.SetPathValue("traceId", "") // explicitly empty
	w := httptest.NewRecorder()

	ctrl.GetTraceScores(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestGetTraceScores_GroupsTraceAndSpanScores(t *testing.T) {
	monitorID := uuid.New()
	explanation := "good answer"
	spanID1 := "span-abc"
	spanID2 := "span-def"

	repo := &configurableScoreRepo{
		traceScores: []repositories.ScoreWithMonitor{
			// Trace-level score (span_id is nil)
			{
				MonitorID:     monitorID,
				TraceID:       "trace-1",
				SpanID:        nil,
				Score:         ptrFloat64(0.85),
				Explanation:   &explanation,
				MonitorName:   "quality-check",
				EvaluatorName: "Faithfulness",
			},
			// Agent-level score (span-level)
			{
				MonitorID:     monitorID,
				TraceID:       "trace-1",
				SpanID:        &spanID1,
				Score:         ptrFloat64(0.9),
				SpanLabel:     "PlanAgent",
				MonitorName:   "quality-check",
				EvaluatorName: "Agent Latency",
			},
			// LLM-level score (span-level, different span)
			{
				MonitorID:     monitorID,
				TraceID:       "trace-1",
				SpanID:        &spanID2,
				Score:         ptrFloat64(0.6),
				SpanLabel:     "openai/gpt-4",
				MonitorName:   "quality-check",
				EvaluatorName: "LLM Latency",
			},
		},
	}

	svc := services.NewMonitorScoresService(repo, slog.Default())
	result, err := svc.GetTraceScores("trace-1", "org1", "proj1", "agent1")
	require.NoError(t, err)

	assert.Equal(t, "trace-1", result.TraceID)
	require.Len(t, result.Monitors, 1)

	mon := result.Monitors[0]
	assert.Equal(t, "quality-check", mon.MonitorName)

	// Trace-level evaluators
	require.Len(t, mon.Evaluators, 1)
	assert.Equal(t, "Faithfulness", mon.Evaluators[0].EvaluatorName)
	assert.InDelta(t, 0.85, *mon.Evaluators[0].Score, 1e-9)
	assert.Equal(t, "good answer", *mon.Evaluators[0].Explanation)

	// Span-level groups
	require.Len(t, mon.Spans, 2)

	assert.Equal(t, "span-abc", mon.Spans[0].SpanID)
	assert.Equal(t, "PlanAgent", mon.Spans[0].SpanLabel)
	require.Len(t, mon.Spans[0].Evaluators, 1)
	assert.Equal(t, "Agent Latency", mon.Spans[0].Evaluators[0].EvaluatorName)
	assert.InDelta(t, 0.9, *mon.Spans[0].Evaluators[0].Score, 1e-9)

	assert.Equal(t, "span-def", mon.Spans[1].SpanID)
	assert.Equal(t, "openai/gpt-4", mon.Spans[1].SpanLabel)
	require.Len(t, mon.Spans[1].Evaluators, 1)
	assert.Equal(t, "LLM Latency", mon.Spans[1].Evaluators[0].EvaluatorName)
	assert.InDelta(t, 0.6, *mon.Spans[1].Evaluators[0].Score, 1e-9)
}

func TestGetTraceScores_MultipleMonitors(t *testing.T) {
	monitorID1 := uuid.New()
	monitorID2 := uuid.New()

	repo := &configurableScoreRepo{
		traceScores: []repositories.ScoreWithMonitor{
			{
				MonitorID:     monitorID1,
				TraceID:       "trace-1",
				Score:         ptrFloat64(0.8),
				MonitorName:   "monitor-a",
				EvaluatorName: "Latency",
			},
			{
				MonitorID:     monitorID2,
				TraceID:       "trace-1",
				Score:         ptrFloat64(0.7),
				MonitorName:   "monitor-b",
				EvaluatorName: "Latency",
			},
		},
	}

	svc := services.NewMonitorScoresService(repo, slog.Default())
	result, err := svc.GetTraceScores("trace-1", "org1", "proj1", "agent1")
	require.NoError(t, err)

	require.Len(t, result.Monitors, 2)
	assert.Equal(t, "monitor-a", result.Monitors[0].MonitorName)
	assert.Equal(t, "monitor-b", result.Monitors[1].MonitorName)
}

func TestGetTraceScores_SkippedScore(t *testing.T) {
	skipReason := "no input data"

	repo := &configurableScoreRepo{
		traceScores: []repositories.ScoreWithMonitor{
			{
				MonitorID:     uuid.New(),
				TraceID:       "trace-1",
				Score:         nil, // skipped
				SkipReason:    &skipReason,
				MonitorName:   "quality-check",
				EvaluatorName: "Faithfulness",
			},
		},
	}

	svc := services.NewMonitorScoresService(repo, slog.Default())
	result, err := svc.GetTraceScores("trace-1", "org1", "proj1", "agent1")
	require.NoError(t, err)

	require.Len(t, result.Monitors, 1)
	require.Len(t, result.Monitors[0].Evaluators, 1)

	eval := result.Monitors[0].Evaluators[0]
	assert.Nil(t, eval.Score)
	assert.Equal(t, "no input data", *eval.SkipReason)
}

func TestGetTraceScores_MultipleEvalsPerSpan(t *testing.T) {
	monitorID := uuid.New()
	spanID := "span-1"

	repo := &configurableScoreRepo{
		traceScores: []repositories.ScoreWithMonitor{
			{
				MonitorID:     monitorID,
				TraceID:       "trace-1",
				SpanID:        &spanID,
				Score:         ptrFloat64(0.9),
				SpanLabel:     "PlanAgent",
				MonitorName:   "quality-check",
				EvaluatorName: "Agent Latency",
			},
			{
				MonitorID:     monitorID,
				TraceID:       "trace-1",
				SpanID:        &spanID,
				Score:         ptrFloat64(0.7),
				SpanLabel:     "PlanAgent",
				MonitorName:   "quality-check",
				EvaluatorName: "Tool Accuracy",
			},
		},
	}

	svc := services.NewMonitorScoresService(repo, slog.Default())
	result, err := svc.GetTraceScores("trace-1", "org1", "proj1", "agent1")
	require.NoError(t, err)

	require.Len(t, result.Monitors, 1)
	require.Len(t, result.Monitors[0].Spans, 1)

	span := result.Monitors[0].Spans[0]
	assert.Equal(t, "span-1", span.SpanID)
	assert.Equal(t, "PlanAgent", span.SpanLabel)
	require.Len(t, span.Evaluators, 2)
	assert.Equal(t, "Agent Latency", span.Evaluators[0].EvaluatorName)
	assert.Equal(t, "Tool Accuracy", span.Evaluators[1].EvaluatorName)
}

func TestGetTraceScores_EmptyResult(t *testing.T) {
	repo := &configurableScoreRepo{
		traceScores: []repositories.ScoreWithMonitor{},
	}

	svc := services.NewMonitorScoresService(repo, slog.Default())
	result, err := svc.GetTraceScores("trace-1", "org1", "proj1", "agent1")
	require.NoError(t, err)

	assert.Equal(t, "trace-1", result.TraceID)
	assert.Empty(t, result.Monitors)
}

// -----------------------------------------------------------------------------
// GetAgentTraceScores — service-level tests
// -----------------------------------------------------------------------------

func TestGetAgentTraceScores_MultipleTraces(t *testing.T) {
	score1 := 0.85
	score2 := 0.60

	repo := &configurableScoreRepo{
		agentTraceAggs: []repositories.TraceAggregation{
			{TraceID: "trace-1", TotalCount: 5, SkippedCount: 1, MeanScore: &score1},
			{TraceID: "trace-2", TotalCount: 3, SkippedCount: 0, MeanScore: &score2},
		},
	}

	svc := services.NewMonitorScoresService(repo, slog.Default())
	result, err := svc.GetAgentTraceScores("org1", "proj1", "agent1", time.Now().Add(-24*time.Hour), time.Now(), 100, 0)
	require.NoError(t, err)

	require.Len(t, result.Traces, 2)

	assert.Equal(t, "trace-1", result.Traces[0].TraceID)
	assert.Equal(t, 0.85, *result.Traces[0].Score)
	assert.Equal(t, 5, result.Traces[0].TotalCount)
	assert.Equal(t, 1, result.Traces[0].SkippedCount)

	assert.Equal(t, "trace-2", result.Traces[1].TraceID)
	assert.Equal(t, 0.60, *result.Traces[1].Score)
	assert.Equal(t, 3, result.Traces[1].TotalCount)
	assert.Equal(t, 0, result.Traces[1].SkippedCount)
}

func TestGetAgentTraceScores_AllSkipped(t *testing.T) {
	repo := &configurableScoreRepo{
		agentTraceAggs: []repositories.TraceAggregation{
			{TraceID: "trace-1", TotalCount: 4, SkippedCount: 4, MeanScore: nil},
		},
	}

	svc := services.NewMonitorScoresService(repo, slog.Default())
	result, err := svc.GetAgentTraceScores("org1", "proj1", "agent1", time.Now().Add(-24*time.Hour), time.Now(), 100, 0)
	require.NoError(t, err)

	require.Len(t, result.Traces, 1)
	assert.Equal(t, "trace-1", result.Traces[0].TraceID)
	assert.Nil(t, result.Traces[0].Score)
	assert.Equal(t, 4, result.Traces[0].TotalCount)
	assert.Equal(t, 4, result.Traces[0].SkippedCount)
}

func TestGetAgentTraceScores_EmptyResult(t *testing.T) {
	repo := &configurableScoreRepo{
		agentTraceAggs: []repositories.TraceAggregation{},
	}

	svc := services.NewMonitorScoresService(repo, slog.Default())
	result, err := svc.GetAgentTraceScores("org1", "proj1", "agent1", time.Now().Add(-24*time.Hour), time.Now(), 100, 0)
	require.NoError(t, err)

	assert.Empty(t, result.Traces)
}

func ptrFloat64(v float64) *float64 {
	return &v
}
