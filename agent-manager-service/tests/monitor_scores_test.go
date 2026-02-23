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
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
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

func (s *stubScoreRepo) GetEvaluatorsByRunID(_ uuid.UUID) ([]models.MonitorRunEvaluator, error) {
	return s.evaluators, nil
}

func (s *stubScoreRepo) GetEvaluatorsByMonitorAndRunID(_, _ uuid.UUID) ([]models.MonitorRunEvaluator, error) {
	return s.evaluators, nil
}
func (s *stubScoreRepo) BatchCreateScores(_ []models.Score) error { return nil }
func (s *stubScoreRepo) DeleteStaleScores(_ uuid.UUID, _ []uuid.UUID, _ []string) error {
	return nil
}

func (s *stubScoreRepo) GetScoresByMonitorAndTimeRange(_ uuid.UUID, _, _ time.Time, _ repositories.ScoreFilters) ([]repositories.ScoreWithEvaluator, error) {
	return nil, nil
}

func (s *stubScoreRepo) GetMonitorScoresAggregated(_ uuid.UUID, _, _ time.Time, _ repositories.ScoreFilters) ([]repositories.EvaluatorAggregation, error) {
	return nil, nil
}

func (s *stubScoreRepo) GetEvaluatorTimeSeriesAggregated(_ uuid.UUID, _ string, _, _ time.Time, _ string) ([]repositories.TimeBucketAggregation, error) {
	return nil, nil
}

func (s *stubScoreRepo) GetScoresByTraceID(_ string, _, _, _ string) ([]repositories.ScoreWithMonitor, error) {
	return nil, nil
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
	mux.HandleFunc("GET "+base+"/scores/timeseries", ctrl.GetScoresTimeSeries)
	mux.HandleFunc("GET "+agentBase+"/traces/{traceId}/scores", ctrl.GetTraceScores)

	return mux
}

// -----------------------------------------------------------------------------
// CalculateGranularity
// -----------------------------------------------------------------------------

func TestCalculateGranularity(t *testing.T) {
	cases := []struct {
		name     string
		duration time.Duration
		want     string
	}{
		{"exactly 24h", 24 * time.Hour, "hour"},
		{"1 day", 24 * time.Hour, "hour"},
		{"exactly 2 days", 2 * 24 * time.Hour, "hour"},
		{"2 days + 1 sec (crosses into day bucket)", 2*24*time.Hour + time.Second, "day"},
		{"7 days", 7 * 24 * time.Hour, "day"},
		{"exactly 28 days", 28 * 24 * time.Hour, "day"},
		{"28 days + 1 sec (crosses into week bucket)", 28*24*time.Hour + time.Second, "week"},
		{"60 days", 60 * 24 * time.Hour, "week"},
		{"100 days", 100 * 24 * time.Hour, "week"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, controllers.CalculateGranularity(tc.duration))
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
	for _, level := range []string{"trace", "agent", "span"} {
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

// TestGetScoresTimeSeries_GranularityBoundaries verifies that valid ranges
// pass all validation checks (not 400).
func TestGetScoresTimeSeries_GranularityBoundaries(t *testing.T) {
	handler := newScoresHandler()
	base := "/orgs/org1/projects/proj1/agents/agent1/monitors/mon1/scores/timeseries"

	now := time.Now().UTC()

	cases := []struct {
		name     string
		duration time.Duration
	}{
		{"24h boundary (hour granularity)", 24 * time.Hour},
		{"2 days (hour granularity)", 2 * 24 * time.Hour},
		{"3 days (day granularity)", 3 * 24 * time.Hour},
		{"28 days (day granularity)", 28 * 24 * time.Hour},
		{"29 days (week granularity)", 29 * 24 * time.Hour},
		{"100 days (week granularity, max allowed)", 100 * 24 * time.Hour},
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
