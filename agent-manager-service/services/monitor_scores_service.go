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

package services

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// MonitorScoresService handles evaluation score business logic
type MonitorScoresService struct {
	repo   repositories.ScoreRepository
	logger *slog.Logger
}

// NewMonitorScoresService creates a new monitor scores service
func NewMonitorScoresService(
	repo repositories.ScoreRepository,
	logger *slog.Logger,
) *MonitorScoresService {
	return &MonitorScoresService{
		repo:   repo,
		logger: logger,
	}
}

// PublishScores stores evaluation scores and aggregates for a monitor run
func (s *MonitorScoresService) PublishScores(
	monitorID, runID uuid.UUID,
	req *models.PublishScoresRequest,
) error {
	// Step 1: Upsert monitor_run_evaluators
	runEvaluators := make([]models.MonitorRunEvaluator, len(req.AggregatedScores))
	for i, item := range req.AggregatedScores {
		runEvaluators[i] = models.MonitorRunEvaluator{
			ID:            uuid.New(),
			MonitorRunID:  runID,
			MonitorID:     monitorID,
			EvaluatorName: item.Identifier,
			DisplayName:   item.DisplayName,
			Level:         item.Level,
			Aggregations:  item.Aggregations,
			Count:         item.Count,
			ErrorCount:    item.ErrorCount,
		}
	}

	// Step 2: Build evaluator displayName → ID map for score insertion
	evaluatorMap := make(map[string]uuid.UUID)
	for _, re := range runEvaluators {
		evaluatorMap[re.DisplayName] = re.ID
	}

	// Step 3: Convert score items to DB models
	scores := make([]models.Score, len(req.IndividualScores))
	for i, item := range req.IndividualScores {
		runEvaluatorID, exists := evaluatorMap[item.DisplayName]
		if !exists {
			return fmt.Errorf("evaluator %s not found in evaluators list", item.DisplayName)
		}

		// TraceTimestamp is required
		if item.TraceTimestamp == nil {
			return fmt.Errorf("TraceTimestamp is required for score with traceID %s", item.TraceID)
		}
		traceTimestamp := *item.TraceTimestamp

		scores[i] = models.Score{
			ID:             uuid.New(),
			RunEvaluatorID: runEvaluatorID,
			MonitorID:      monitorID,
			TraceID:        item.TraceID,
			SpanID:         item.SpanID,
			Score:          item.Score,
			Explanation:    item.Explanation,
			TraceTimestamp: traceTimestamp,
			Metadata:       item.Metadata,
			Error:          item.Error,
		}
	}

	// Step 4: Upsert evaluators and scores atomically
	if err := s.repo.RunInTransaction(func(txRepo repositories.ScoreRepository) error {
		if err := txRepo.UpsertMonitorRunEvaluators(runEvaluators); err != nil {
			return fmt.Errorf("failed to upsert run evaluators: %w", err)
		}

		if err := txRepo.BatchCreateScores(scores); err != nil {
			return fmt.Errorf("failed to create scores: %w", err)
		}

		return nil
	}); err != nil {
		return err
	}

	s.logger.Info("Published evaluation scores",
		"monitorID", monitorID,
		"runID", runID,
		"scoreCount", len(scores),
		"evaluatorCount", len(runEvaluators))

	return nil
}

// GetMonitorScores returns aggregated scores for a monitor within a time range
func (s *MonitorScoresService) GetMonitorScores(
	monitorID uuid.UUID,
	monitorName string,
	startTime, endTime time.Time,
	filters repositories.ScoreFilters,
) (*models.MonitorScoresResponse, error) {
	// Use SQL aggregation query instead of fetching all rows
	aggregations, err := s.repo.GetMonitorScoresAggregated(monitorID, startTime, endTime, filters)
	if err != nil {
		return nil, fmt.Errorf("failed to get aggregated scores: %w", err)
	}

	// Convert aggregations to response format
	evaluators := make([]models.EvaluatorScoreSummary, len(aggregations))
	for i, agg := range aggregations {
		aggregationMap := make(map[string]interface{})
		if agg.MeanScore != nil {
			aggregationMap["mean"] = *agg.MeanScore
		}

		evaluators[i] = models.EvaluatorScoreSummary{
			EvaluatorName: agg.EvaluatorName,
			Level:         agg.Level,
			Count:         agg.SuccessCount + agg.ErrorCount,
			ErrorCount:    agg.ErrorCount,
			Aggregations:  aggregationMap,
		}
	}

	return &models.MonitorScoresResponse{
		MonitorName: monitorName,
		TimeRange: models.TimeRange{
			Start: startTime,
			End:   endTime,
		},
		Evaluators: evaluators,
	}, nil
}

// GetEvaluatorTimeSeries returns time-series data for a specific evaluator
func (s *MonitorScoresService) GetEvaluatorTimeSeries(
	monitorID uuid.UUID,
	monitorName, displayName string,
	startTime, endTime time.Time,
	granularity string,
) (*models.TimeSeriesResponse, error) {
	// Use SQL aggregation query instead of fetching all rows
	aggregations, err := s.repo.GetEvaluatorTimeSeriesAggregated(monitorID, displayName, startTime, endTime, granularity)
	if err != nil {
		return nil, fmt.Errorf("failed to get time series aggregations: %w", err)
	}

	// Convert aggregations to response format
	points := make([]models.TimeSeriesPoint, len(aggregations))
	for i, agg := range aggregations {
		aggregationMap := make(map[string]interface{})
		if agg.MeanScore != nil {
			aggregationMap["mean"] = *agg.MeanScore
		}

		points[i] = models.TimeSeriesPoint{
			Timestamp:    agg.TimeBucket,
			Count:        agg.SuccessCount + agg.ErrorCount,
			ErrorCount:   agg.ErrorCount,
			Aggregations: aggregationMap,
		}
	}

	return &models.TimeSeriesResponse{
		MonitorName:   monitorName,
		EvaluatorName: displayName,
		Granularity:   granularity,
		Points:        points,
	}, nil
}

// GetTraceScores returns all evaluation scores for a specific trace across all monitors
func (s *MonitorScoresService) GetTraceScores(
	traceID, orgName, projName, agentName string,
) (*models.TraceScoresResponse, error) {
	scores, err := s.repo.GetScoresByTraceID(traceID, orgName, projName, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get trace scores: %w", err)
	}

	// Group by monitor → evaluator
	type monitorGroup struct {
		monitorID   uuid.UUID
		monitorName string
		runID       uuid.UUID
		evaluators  map[string]*models.EvaluatorTraceGroup
		evalOrder   []string
	}

	monitorMap := make(map[uuid.UUID]*monitorGroup)
	var monitorOrder []uuid.UUID

	for _, score := range scores {
		mg, exists := monitorMap[score.MonitorID]
		if !exists {
			mg = &monitorGroup{
				monitorID:   score.MonitorID,
				monitorName: score.MonitorName,
				runID:       score.RunID,
				evaluators:  make(map[string]*models.EvaluatorTraceGroup),
				evalOrder:   []string{},
			}
			monitorMap[score.MonitorID] = mg
			monitorOrder = append(monitorOrder, score.MonitorID)
		}

		eg, exists := mg.evaluators[score.EvaluatorName]
		if !exists {
			eg = &models.EvaluatorTraceGroup{
				EvaluatorName: score.EvaluatorName,
				Level:         score.Level,
				Scores:        []models.ScoreItem{},
			}
			mg.evaluators[score.EvaluatorName] = eg
			mg.evalOrder = append(mg.evalOrder, score.EvaluatorName)
		}

		eg.Scores = append(eg.Scores, models.ScoreItem{
			SpanID:      score.SpanID,
			Score:       score.Score,
			Explanation: score.Explanation,
			Metadata:    score.Metadata,
			Error:       score.Error,
		})
	}

	// Build response
	monitors := make([]models.MonitorTraceGroup, len(monitorOrder))
	for i, monitorID := range monitorOrder {
		mg := monitorMap[monitorID]

		evaluators := make([]models.EvaluatorTraceGroup, len(mg.evalOrder))
		for j, evalName := range mg.evalOrder {
			evaluators[j] = *mg.evaluators[evalName]
		}

		monitors[i] = models.MonitorTraceGroup{
			MonitorName: mg.monitorName,
			MonitorID:   mg.monitorID.String(),
			RunID:       mg.runID.String(),
			Evaluators:  evaluators,
		}
	}

	return &models.TraceScoresResponse{
		TraceID:  traceID,
		Monitors: monitors,
	}, nil
}

// GetMonitorID resolves monitor name to monitor ID
func (s *MonitorScoresService) GetMonitorID(orgName, projName, agentName, monitorName string) (uuid.UUID, error) {
	id, err := s.repo.GetMonitorID(orgName, projName, agentName, monitorName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return uuid.Nil, fmt.Errorf("monitor %q not found: %w", monitorName, utils.ErrMonitorNotFound)
		}
		return uuid.Nil, fmt.Errorf("failed to query monitor: %w", err)
	}
	return id, nil
}
