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

package models

import (
	"time"

	"github.com/google/uuid"
)

// ============================================================================
// GORM Models
// ============================================================================

// MonitorRunEvaluator is the junction table linking runs to evaluators with aggregations
type MonitorRunEvaluator struct {
	ID            uuid.UUID              `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	MonitorRunID  uuid.UUID              `gorm:"column:monitor_run_id;not null"`
	MonitorID     uuid.UUID              `gorm:"column:monitor_id;not null"`
	EvaluatorName string                 `gorm:"column:evaluator_name;not null"`
	DisplayName   string                 `gorm:"column:display_name;not null"`
	Level         string                 `gorm:"column:level;not null"`
	Aggregations  map[string]interface{} `gorm:"column:aggregations;type:jsonb;serializer:json;default:'{}'"`
	Count         int                    `gorm:"column:count;not null;default:0"`
	SkippedCount  int                    `gorm:"column:skipped_count;not null;default:0"`
	CreatedAt     time.Time              `gorm:"column:created_at;not null;default:NOW()"`
}

func (MonitorRunEvaluator) TableName() string { return "monitor_run_evaluators" }

// Score is the individual evaluation result
type Score struct {
	ID             uuid.UUID              `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	RunEvaluatorID uuid.UUID              `gorm:"column:run_evaluator_id;not null"`
	MonitorID      uuid.UUID              `gorm:"column:monitor_id;not null"`
	TraceID        string                 `gorm:"column:trace_id;not null"`
	SpanID         *string                `gorm:"column:span_id"`
	Score          *float64               `gorm:"column:score"`
	Explanation    *string                `gorm:"column:explanation"`
	TraceTimestamp time.Time              `gorm:"column:trace_timestamp;not null"`
	Metadata       map[string]interface{} `gorm:"column:metadata;type:jsonb;serializer:json;default:'{}'"`
	SkipReason     *string                `gorm:"column:skip_reason"`
	CreatedAt      time.Time              `gorm:"column:created_at;not null;default:NOW()"`
}

func (Score) TableName() string { return "scores" }

// ============================================================================
// Request Types
// ============================================================================

// PublishScoresRequest is the batch publish request from eval job
type PublishScoresRequest struct {
	IndividualScores []PublishScoreItem     `json:"individualScores" validate:"required,min=1,dive"`
	AggregatedScores []PublishAggregateItem `json:"aggregatedScores" validate:"required,min=1,dive"`
}

// PublishScoreItem is an individual score in publish request
type PublishScoreItem struct {
	DisplayName    string                 `json:"displayName" validate:"required"`
	Level          string                 `json:"level" validate:"required"`
	TraceID        string                 `json:"traceId" validate:"required"`
	SpanID         *string                `json:"spanId,omitempty"`
	Score          *float64               `json:"score,omitempty" validate:"omitempty,min=0,max=1"`
	Explanation    *string                `json:"explanation,omitempty"`
	TraceTimestamp *time.Time             `json:"traceTimestamp,omitempty"`
	Metadata       map[string]interface{} `json:"metadata,omitempty"`
	SkipReason     *string                `json:"skipReason,omitempty"`
}

// PublishAggregateItem is evaluator info + aggregations in publish request
type PublishAggregateItem struct {
	Identifier   string                 `json:"identifier" validate:"required"`
	DisplayName  string                 `json:"displayName" validate:"required"`
	Level        string                 `json:"level" validate:"required,oneof=trace agent span"`
	Aggregations map[string]interface{} `json:"aggregations" validate:"required"`
	Count        int                    `json:"count"`
	SkippedCount int                    `json:"skippedCount"`
}

// ============================================================================
// Response Types
// ============================================================================

// MonitorScoresResponse is the response for GET /monitors/{monitorName}/scores
type MonitorScoresResponse struct {
	MonitorName string                  `json:"monitorName"`
	TimeRange   TimeRange               `json:"timeRange"`
	Evaluators  []EvaluatorScoreSummary `json:"evaluators"`
}

// TimeRange represents a time window
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// EvaluatorScoreSummary is aggregated scores for one evaluator
type EvaluatorScoreSummary struct {
	EvaluatorName string                 `json:"evaluatorName"`
	Level         string                 `json:"level"`
	Count         int                    `json:"count"`
	SkippedCount  int                    `json:"skippedCount"`
	Aggregations  map[string]interface{} `json:"aggregations"`
}

// TimeSeriesResponse is the response for time series data
type TimeSeriesResponse struct {
	MonitorName   string            `json:"monitorName"`
	EvaluatorName string            `json:"evaluatorName"`
	Granularity   string            `json:"granularity"`
	Points        []TimeSeriesPoint `json:"points"`
}

// TimeSeriesPoint is a single data point in time series
type TimeSeriesPoint struct {
	Timestamp    time.Time              `json:"timestamp"`
	Count        int                    `json:"count"`
	SkippedCount int                    `json:"skippedCount"`
	Aggregations map[string]interface{} `json:"aggregations"`
}

// MonitorRunScoresResponse is the response for GET .../runs/{runId}/scores
type MonitorRunScoresResponse struct {
	RunID       string                  `json:"runId"`
	MonitorName string                  `json:"monitorName"`
	Evaluators  []EvaluatorScoreSummary `json:"evaluators"`
}

// TraceScoresResponse is the response for GET /traces/{traceId}/scores
type TraceScoresResponse struct {
	TraceID  string              `json:"traceId"`
	Monitors []MonitorTraceGroup `json:"monitors"`
}

// MonitorTraceGroup groups scores by monitor
type MonitorTraceGroup struct {
	MonitorName string                `json:"monitorName"`
	MonitorID   string                `json:"monitorId"`
	RunID       string                `json:"runId"`
	Evaluators  []EvaluatorTraceGroup `json:"evaluators"`
}

// EvaluatorTraceGroup groups scores by evaluator within a monitor
type EvaluatorTraceGroup struct {
	EvaluatorName string      `json:"evaluatorName"`
	Level         string      `json:"level"`
	Scores        []ScoreItem `json:"scores"`
}

// ScoreItem is an individual score in trace response
type ScoreItem struct {
	SpanID      *string                `json:"spanId,omitempty"`
	Score       *float64               `json:"score,omitempty"`
	Explanation *string                `json:"explanation,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	SkipReason  *string                `json:"skipReason,omitempty"`
}
