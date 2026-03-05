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
	Identifier    string                 `gorm:"column:identifier;not null"`
	EvaluatorName string                 `gorm:"column:evaluator_name;not null"`
	Level         string                 `gorm:"column:level;not null"`
	Aggregations  map[string]interface{} `gorm:"column:aggregations;type:jsonb;serializer:json;default:'{}'"`
	Count         int                    `gorm:"column:count;not null;default:0"`
	SkippedCount  int                    `gorm:"column:skipped_count;not null;default:0"`
	CreatedAt     time.Time              `gorm:"column:created_at;not null;default:NOW()"`
}

func (MonitorRunEvaluator) TableName() string { return "monitor_run_evaluators" }

// Score is the individual evaluation result
type Score struct {
	ID             uuid.UUID `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	RunEvaluatorID uuid.UUID `gorm:"column:run_evaluator_id;not null"`
	MonitorID      uuid.UUID `gorm:"column:monitor_id;not null"`
	TraceID        string    `gorm:"column:trace_id;not null"`
	SpanID         *string   `gorm:"column:span_id"`
	Score          *float64  `gorm:"column:score"`
	Explanation    *string   `gorm:"column:explanation"`
	TraceStartTime time.Time `gorm:"column:trace_start_time;not null"`
	SkipReason     *string   `gorm:"column:skip_reason"`
	SpanLabel      string    `gorm:"column:span_label;default:''"`
	CreatedAt      time.Time `gorm:"column:created_at;not null;default:NOW()"`
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

// PublishSpanContext carries span identity from the evaluation framework
type PublishSpanContext struct {
	SpanID    *string `json:"spanId,omitempty"`
	AgentName *string `json:"agentName,omitempty"`
	Model     *string `json:"model,omitempty"`
	Vendor    *string `json:"vendor,omitempty"`
}

// PublishScoreItem is an individual score in publish request
type PublishScoreItem struct {
	EvaluatorName  string              `json:"evaluatorName" validate:"required"`
	Level          string              `json:"level" validate:"required"`
	TraceID        string              `json:"traceId" validate:"required"`
	Score          *float64            `json:"score,omitempty" validate:"omitempty,min=0,max=1"`
	Explanation    *string             `json:"explanation,omitempty"`
	TraceStartTime *time.Time          `json:"traceStartTime,omitempty"`
	SkipReason     *string             `json:"skipReason,omitempty"`
	SpanContext    *PublishSpanContext `json:"spanContext,omitempty"`
}

// PublishAggregateItem is evaluator info + aggregations in publish request
type PublishAggregateItem struct {
	Identifier    string                 `json:"identifier" validate:"required"`
	EvaluatorName string                 `json:"evaluatorName" validate:"required"`
	Level         string                 `json:"level" validate:"required,oneof=trace agent llm"`
	Aggregations  map[string]interface{} `json:"aggregations" validate:"required"`
	Count         int                    `json:"count"`
	SkippedCount  int                    `json:"skippedCount"`
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
	Monitors []TraceMonitorGroup `json:"monitors"`
}

// TraceMonitorGroup groups trace-level and span-level scores by monitor
type TraceMonitorGroup struct {
	MonitorName string                `json:"monitorName"`
	Evaluators  []TraceEvaluatorScore `json:"evaluators"`
	Spans       []TraceSpanGroup      `json:"spans"`
}

// TraceEvaluatorScore is a single evaluator score
type TraceEvaluatorScore struct {
	EvaluatorName string   `json:"evaluatorName"`
	Score         *float64 `json:"score,omitempty"`
	Explanation   *string  `json:"explanation,omitempty"`
	SkipReason    *string  `json:"skipReason,omitempty"`
}

// TraceSpanGroup groups evaluator scores by span
type TraceSpanGroup struct {
	SpanID     string                `json:"spanId"`
	SpanLabel  string                `json:"spanLabel,omitempty"`
	Evaluators []TraceEvaluatorScore `json:"evaluators"`
}

// AgentTraceScoresResponse is the response for GET /agents/{agentName}/scores
type AgentTraceScoresResponse struct {
	Traces     []TraceScoreSummary `json:"traces"`
	TotalCount int                 `json:"totalCount"`
}

// TraceScoreSummary is a single trace with its aggregated score
type TraceScoreSummary struct {
	TraceID      string   `json:"traceId"`
	Score        *float64 `json:"score,omitempty"`
	TotalCount   int      `json:"totalCount"`
	SkippedCount int      `json:"skippedCount"`
}

// GroupedScoresResponse is the response for GET /monitors/{monitorName}/scores/breakdown
type GroupedScoresResponse struct {
	MonitorName string            `json:"monitorName"`
	Level       string            `json:"level"`
	TimeRange   TimeRange         `json:"timeRange"`
	Groups      []ScoreLabelGroup `json:"groups"`
}

// ScoreLabelGroup groups evaluator scores under a span label (agent name or model name)
type ScoreLabelGroup struct {
	Label      string                  `json:"label"`
	Evaluators []LabelEvaluatorSummary `json:"evaluators"`
}

// LabelEvaluatorSummary is aggregated scores for one evaluator within a label group
type LabelEvaluatorSummary struct {
	EvaluatorName string   `json:"evaluatorName"`
	Mean          *float64 `json:"mean"`
	Count         int      `json:"count"`
	SkippedCount  int      `json:"skippedCount"`
}
