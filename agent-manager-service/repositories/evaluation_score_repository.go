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

package repositories

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// ScoreRepository defines the interface for score data access
type ScoreRepository interface {
	// Transaction support
	WithTx(tx *gorm.DB) ScoreRepository
	RunInTransaction(fn func(txRepo ScoreRepository) error) error

	// MonitorRunEvaluator operations
	UpsertMonitorRunEvaluators(evaluators []models.MonitorRunEvaluator) error
	GetEvaluatorsByRunID(runID uuid.UUID) ([]models.MonitorRunEvaluator, error)

	// Score publishing
	BatchCreateScores(scores []models.Score) error
	DeleteStaleScores(monitorID uuid.UUID, currentRunEvaluatorIDs []uuid.UUID, traceIDs []string) error

	// Monitor-level queries (time-based)
	GetScoresByMonitorAndTimeRange(monitorID uuid.UUID, startTime, endTime time.Time, filters ScoreFilters) ([]ScoreWithEvaluator, error)

	// Aggregated queries (SQL-based aggregations)
	GetMonitorScoresAggregated(monitorID uuid.UUID, startTime, endTime time.Time, filters ScoreFilters) ([]EvaluatorAggregation, error)
	GetEvaluatorTimeSeriesAggregated(monitorID uuid.UUID, displayName string, startTime, endTime time.Time, granularity string) ([]TimeBucketAggregation, error)

	// Trace-level queries (cross-monitor)
	GetScoresByTraceID(traceID string, orgName, projName, agentName string) ([]ScoreWithMonitor, error)

	// Monitor lookup
	GetMonitorID(orgName, projName, agentName, monitorName string) (uuid.UUID, error)
}

// ScoreFilters contains optional filters for querying scores
type ScoreFilters struct {
	EvaluatorName string
	Level         string
}

// EvaluatorAggregation is the result of aggregated scores per evaluator (from SQL GROUP BY)
type EvaluatorAggregation struct {
	EvaluatorName string   `gorm:"column:display_name"`
	Level         string   `gorm:"column:level"`
	SuccessCount  int      `gorm:"column:success_count"`
	ErrorCount    int      `gorm:"column:error_count"`
	MeanScore     *float64 `gorm:"column:mean_score"` // NULL if no successful scores
}

// TimeBucketAggregation is the result of aggregated scores per time bucket (from SQL GROUP BY)
type TimeBucketAggregation struct {
	TimeBucket   time.Time `gorm:"column:time_bucket"`
	SuccessCount int       `gorm:"column:success_count"`
	ErrorCount   int       `gorm:"column:error_count"`
	MeanScore    *float64  `gorm:"column:mean_score"` // NULL if no successful scores
}

// ScoreWithEvaluator is a score joined with its evaluator info (flattened for GORM scanning)
type ScoreWithEvaluator struct {
	// Score fields
	ID             uuid.UUID              `gorm:"column:id"`
	RunEvaluatorID uuid.UUID              `gorm:"column:run_evaluator_id"`
	MonitorID      uuid.UUID              `gorm:"column:monitor_id"`
	TraceID        string                 `gorm:"column:trace_id"`
	SpanID         *string                `gorm:"column:span_id"`
	Score          *float64               `gorm:"column:score"`
	Explanation    *string                `gorm:"column:explanation"`
	TraceTimestamp time.Time              `gorm:"column:trace_timestamp"`
	Metadata       map[string]interface{} `gorm:"column:metadata;type:jsonb;serializer:json"`
	Error          *string                `gorm:"column:error"`
	CreatedAt      time.Time              `gorm:"column:created_at"`
	// Evaluator info from join
	EvaluatorName string `gorm:"column:display_name"`
	Level         string `gorm:"column:level"`
}

// ScoreWithMonitor is a score joined with monitor and run info (flattened for GORM scanning)
type ScoreWithMonitor struct {
	// Score fields
	ID             uuid.UUID              `gorm:"column:id"`
	RunEvaluatorID uuid.UUID              `gorm:"column:run_evaluator_id"`
	MonitorID      uuid.UUID              `gorm:"column:monitor_id"`
	TraceID        string                 `gorm:"column:trace_id"`
	SpanID         *string                `gorm:"column:span_id"`
	Score          *float64               `gorm:"column:score"`
	Explanation    *string                `gorm:"column:explanation"`
	TraceTimestamp time.Time              `gorm:"column:trace_timestamp"`
	Metadata       map[string]interface{} `gorm:"column:metadata;type:jsonb;serializer:json"`
	Error          *string                `gorm:"column:error"`
	CreatedAt      time.Time              `gorm:"column:created_at"`
	// Evaluator and monitor info from join
	EvaluatorName string    `gorm:"column:display_name"`
	Level         string    `gorm:"column:level"`
	MonitorName   string    `gorm:"column:monitor_name"`
	RunID         uuid.UUID `gorm:"column:run_id"`
}

// ScoreRepo implements ScoreRepository using GORM
type ScoreRepo struct {
	db *gorm.DB
}

// NewScoreRepo creates a new score repository
func NewScoreRepo(db *gorm.DB) ScoreRepository {
	return &ScoreRepo{db: db}
}

// WithTx returns a new ScoreRepository backed by the given transaction
func (r *ScoreRepo) WithTx(tx *gorm.DB) ScoreRepository {
	return &ScoreRepo{db: tx}
}

// RunInTransaction executes fn within a database transaction, providing a transaction-bound repository
func (r *ScoreRepo) RunInTransaction(fn func(txRepo ScoreRepository) error) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		return fn(r.WithTx(tx))
	})
}

// UpsertMonitorRunEvaluators creates or updates evaluator records for a run
func (r *ScoreRepo) UpsertMonitorRunEvaluators(evaluators []models.MonitorRunEvaluator) error {
	if len(evaluators) == 0 {
		return nil
	}

	// Use ON CONFLICT to handle upserts
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "monitor_run_id"}, {Name: "display_name"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"monitor_id", "evaluator_name", "level", "aggregations", "count", "error_count",
		}),
	}).Create(&evaluators).Error
}

// GetEvaluatorsByRunID fetches all evaluators for a specific run
func (r *ScoreRepo) GetEvaluatorsByRunID(runID uuid.UUID) ([]models.MonitorRunEvaluator, error) {
	var evaluators []models.MonitorRunEvaluator
	err := r.db.Where("monitor_run_id = ?", runID).Find(&evaluators).Error
	return evaluators, err
}

// BatchCreateScores creates scores in batches with upsert logic
func (r *ScoreRepo) BatchCreateScores(scores []models.Score) error {
	if len(scores) == 0 {
		return nil
	}

	// Use ON CONFLICT to handle upserts (replaces existing scores on rerun)
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "run_evaluator_id"},
			{Name: "trace_id"},
			{Name: "span_id"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"score", "explanation", "trace_timestamp", "metadata", "error",
		}),
	}).CreateInBatches(scores, 100).Error
}

// DeleteStaleScores removes scores from previous runs for the same monitor and traces,
// keeping only scores belonging to the current run's evaluators.
// This is called during publish to ensure reruns replace old scores.
func (r *ScoreRepo) DeleteStaleScores(monitorID uuid.UUID, currentRunEvaluatorIDs []uuid.UUID, traceIDs []string) error {
	if len(currentRunEvaluatorIDs) == 0 || len(traceIDs) == 0 {
		return nil
	}

	return r.db.Where("monitor_id = ? AND trace_id IN ? AND run_evaluator_id NOT IN ?",
		monitorID, traceIDs, currentRunEvaluatorIDs).
		Delete(&models.Score{}).Error
}

// GetScoresByMonitorAndTimeRange fetches scores for a monitor within a time window
func (r *ScoreRepo) GetScoresByMonitorAndTimeRange(
	monitorID uuid.UUID,
	startTime, endTime time.Time,
	filters ScoreFilters,
) ([]ScoreWithEvaluator, error) {
	var results []ScoreWithEvaluator

	query := r.db.Table("scores s").
		Select("s.*, mre.display_name, mre.level").
		Joins("JOIN monitor_run_evaluators mre ON s.run_evaluator_id = mre.id").
		Where("s.monitor_id = ?", monitorID).
		Where("s.trace_timestamp BETWEEN ? AND ?", startTime, endTime)

	if filters.EvaluatorName != "" {
		query = query.Where("mre.display_name = ?", filters.EvaluatorName)
	}
	if filters.Level != "" {
		query = query.Where("mre.level = ?", filters.Level)
	}

	err := query.Order("s.trace_timestamp ASC").Find(&results).Error
	return results, err
}

// GetScoresByTraceID fetches all scores for a specific trace across all monitors
func (r *ScoreRepo) GetScoresByTraceID(traceID string, orgName, projName, agentName string) ([]ScoreWithMonitor, error) {
	var results []ScoreWithMonitor

	err := r.db.Table("scores s").
		Select("s.*, mre.display_name, mre.level, m.name as monitor_name, m.id as monitor_id, mr.id as run_id").
		Joins("JOIN monitor_run_evaluators mre ON s.run_evaluator_id = mre.id").
		Joins("JOIN monitor_runs mr ON mre.monitor_run_id = mr.id").
		Joins("JOIN monitors m ON mr.monitor_id = m.id").
		Where("s.trace_id = ?", traceID).
		Where("m.org_name = ? AND m.project_name = ? AND m.agent_name = ?", orgName, projName, agentName).
		Order("m.name, mre.display_name, s.created_at").
		Find(&results).Error

	return results, err
}

// GetMonitorID resolves monitor name to monitor ID
func (r *ScoreRepo) GetMonitorID(orgName, projName, agentName, monitorName string) (uuid.UUID, error) {
	var monitor models.Monitor
	if err := r.db.Where(
		"name = ? AND org_name = ? AND project_name = ? AND agent_name = ?",
		monitorName, orgName, projName, agentName,
	).Select("id").First(&monitor).Error; err != nil {
		return uuid.Nil, err
	}
	return monitor.ID, nil
}

// GetMonitorScoresAggregated returns pre-aggregated scores per evaluator using SQL GROUP BY
func (r *ScoreRepo) GetMonitorScoresAggregated(
	monitorID uuid.UUID,
	startTime, endTime time.Time,
	filters ScoreFilters,
) ([]EvaluatorAggregation, error) {
	var results []EvaluatorAggregation

	query := r.db.Table("scores s").
		Select(`
			mre.display_name,
			mre.level,
			COUNT(CASE WHEN s.error IS NULL THEN 1 END) as success_count,
			COUNT(CASE WHEN s.error IS NOT NULL THEN 1 END) as error_count,
			AVG(CASE WHEN s.error IS NULL THEN s.score END) as mean_score
		`).
		Joins("JOIN monitor_run_evaluators mre ON s.run_evaluator_id = mre.id").
		Where("s.monitor_id = ?", monitorID).
		Where("s.trace_timestamp BETWEEN ? AND ?", startTime, endTime).
		Group("mre.display_name, mre.level").
		Order("mre.display_name")

	if filters.EvaluatorName != "" {
		query = query.Where("mre.display_name = ?", filters.EvaluatorName)
	}
	if filters.Level != "" {
		query = query.Where("mre.level = ?", filters.Level)
	}

	err := query.Find(&results).Error
	return results, err
}

// GetEvaluatorTimeSeriesAggregated returns pre-aggregated scores per time bucket using SQL GROUP BY
func (r *ScoreRepo) GetEvaluatorTimeSeriesAggregated(
	monitorID uuid.UUID,
	displayName string,
	startTime, endTime time.Time,
	granularity string,
) ([]TimeBucketAggregation, error) {
	var results []TimeBucketAggregation

	// Map granularity to PostgreSQL date_trunc argument
	var truncArg string
	switch granularity {
	case "day":
		truncArg = "day"
	case "week":
		truncArg = "week"
	case "hour":
		fallthrough
	default:
		truncArg = "hour"
	}

	query := r.db.Table("scores s").
		Select(`
			date_trunc(?, s.trace_timestamp) as time_bucket,
			COUNT(CASE WHEN s.error IS NULL THEN 1 END) as success_count,
			COUNT(CASE WHEN s.error IS NOT NULL THEN 1 END) as error_count,
			AVG(CASE WHEN s.error IS NULL THEN s.score END) as mean_score
		`, truncArg).
		Joins("JOIN monitor_run_evaluators mre ON s.run_evaluator_id = mre.id").
		Where("s.monitor_id = ?", monitorID).
		Where("s.trace_timestamp BETWEEN ? AND ?", startTime, endTime).
		Where("mre.display_name = ?", displayName).
		Group("time_bucket").
		Order("time_bucket")

	err := query.Find(&results).Error
	return results, err
}
