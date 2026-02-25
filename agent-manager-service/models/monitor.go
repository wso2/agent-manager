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

// Monitor types
const (
	MonitorTypeFuture = "future"
	MonitorTypePast   = "past"
)

// MonitorStatus represents the status of a monitor
type MonitorStatus string

const (
	MonitorStatusActive    MonitorStatus = "Active"
	MonitorStatusSuspended MonitorStatus = "Suspended"
	MonitorStatusFailed    MonitorStatus = "Failed"
	MonitorStatusUnknown   MonitorStatus = "Unknown"
)

// Run status constants
const (
	RunStatusPending = "pending"
	RunStatusRunning = "running"
	RunStatusSuccess = "success"
	RunStatusFailed  = "failed"
)

// MonitorEvaluator represents an evaluator with its configuration
type MonitorEvaluator struct {
	Identifier  string                 `json:"identifier" validate:"required,min=1"`
	DisplayName string                 `json:"displayName" validate:"required,min=1"`
	Config      map[string]interface{} `json:"config,omitempty"`
}

// MonitorLLMProviderConfig holds a single env-var credential for an LLM provider.
// The platform sets EnvVar=Value on the evaluation job process at runtime;
// LiteLLM reads these natively — no changes to evaluator code needed.
type MonitorLLMProviderConfig struct {
	ProviderName string `json:"providerName"` // e.g. "openai" — from catalog LLMProviderEntry.Name
	EnvVar       string `json:"envVar"`       // e.g. "OPENAI_API_KEY" — from catalog LLMConfigField.EnvVar
	Value        string `json:"value"`        // API key value
}

// redactLLMProviderConfigs returns a copy with secret values cleared.
func redactLLMProviderConfigs(configs []MonitorLLMProviderConfig) []MonitorLLMProviderConfig {
	redacted := make([]MonitorLLMProviderConfig, len(configs))
	for i, c := range configs {
		redacted[i] = MonitorLLMProviderConfig{ProviderName: c.ProviderName, EnvVar: c.EnvVar, Value: "****"}
	}
	return redacted
}

// Monitor is the GORM model for the monitors table
type Monitor struct {
	ID                 uuid.UUID                  `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	Name               string                     `gorm:"column:name;not null"`
	DisplayName        string                     `gorm:"column:display_name;not null;default:''"`
	Type               string                     `gorm:"column:type;not null"`
	OrgName            string                     `gorm:"column:org_name;not null"`
	ProjectName        string                     `gorm:"column:project_name;not null"`
	AgentName          string                     `gorm:"column:agent_name;not null"`
	AgentID            string                     `gorm:"column:agent_id;not null"`
	EnvironmentName    string                     `gorm:"column:environment_name;not null"`
	EnvironmentID      string                     `gorm:"column:environment_id;not null"`
	Evaluators         []MonitorEvaluator         `gorm:"column:evaluators;type:jsonb;serializer:json;not null;default:'[]'"`
	LLMProviderConfigs []MonitorLLMProviderConfig `gorm:"column:llm_provider_configs;type:jsonb;serializer:json;not null;default:'[]'"`
	IntervalMinutes    *int                       `gorm:"column:interval_minutes"`
	NextRunTime        *time.Time                 `gorm:"column:next_run_time"`
	TraceStart         *time.Time                 `gorm:"column:trace_start"`
	TraceEnd           *time.Time                 `gorm:"column:trace_end"`
	SamplingRate       float64                    `gorm:"column:sampling_rate;not null;default:1.00"`
	CreatedAt          time.Time                  `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt          time.Time                  `gorm:"column:updated_at;not null;default:NOW()"`
}

func (Monitor) TableName() string { return "monitors" }

// ToResponse converts a Monitor DB record to MonitorResponse
func (m *Monitor) ToResponse(status MonitorStatus, latestRun *MonitorRunResponse) *MonitorResponse {
	return &MonitorResponse{
		ID:                 m.ID.String(),
		Name:               m.Name,
		DisplayName:        m.DisplayName,
		Type:               m.Type,
		OrgName:            m.OrgName,
		ProjectName:        m.ProjectName,
		AgentName:          m.AgentName,
		AgentID:            m.AgentID,
		EnvironmentName:    m.EnvironmentName,
		EnvironmentID:      m.EnvironmentID,
		Evaluators:         m.Evaluators,
		LLMProviderConfigs: redactLLMProviderConfigs(m.LLMProviderConfigs),
		IntervalMinutes:    m.IntervalMinutes,
		NextRunTime:        m.NextRunTime,
		TraceStart:         m.TraceStart,
		TraceEnd:           m.TraceEnd,
		SamplingRate:       m.SamplingRate,
		Status:             status,
		LatestRun:          latestRun,
		CreatedAt:          m.CreatedAt,
	}
}

// MonitorRun is the GORM model for the monitor_runs table
type MonitorRun struct {
	ID                 uuid.UUID                  `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	MonitorID          uuid.UUID                  `gorm:"column:monitor_id;not null"`
	Name               string                     `gorm:"column:name;not null"` // WorkflowRun name for querying OpenChoreo API
	Evaluators         []MonitorEvaluator         `gorm:"column:evaluators;type:jsonb;serializer:json;not null;default:'[]'"`
	LLMProviderConfigs []MonitorLLMProviderConfig `gorm:"column:llm_provider_configs;type:jsonb;serializer:json;not null;default:'[]'"`
	TraceStart         time.Time                  `gorm:"column:trace_start;not null"`
	TraceEnd           time.Time                  `gorm:"column:trace_end;not null"`
	StartedAt          *time.Time                 `gorm:"column:started_at"`
	CompletedAt        *time.Time                 `gorm:"column:completed_at"`
	Status             string                     `gorm:"column:status;not null;default:'pending'"`
	ErrorMessage       *string                    `gorm:"column:error_message"`
	CreatedAt          time.Time                  `gorm:"column:created_at;not null;default:NOW()"`
}

func (MonitorRun) TableName() string { return "monitor_runs" }

// ToResponse converts a MonitorRun DB record to MonitorRunResponse
func (r *MonitorRun) ToResponse() *MonitorRunResponse {
	return &MonitorRunResponse{
		ID:                 r.ID.String(),
		MonitorName:        "",
		Evaluators:         r.Evaluators,
		LLMProviderConfigs: redactLLMProviderConfigs(r.LLMProviderConfigs),
		TraceStart:         r.TraceStart,
		TraceEnd:           r.TraceEnd,
		StartedAt:          r.StartedAt,
		CompletedAt:        r.CompletedAt,
		Status:             r.Status,
		ErrorMessage:       r.ErrorMessage,
	}
}

// CreateMonitorRequest is the request body for creating a monitor
type CreateMonitorRequest struct {
	Name               string                     `json:"name" validate:"required,min=1,max=63"`
	DisplayName        string                     `json:"displayName" validate:"required,min=1,max=128"`
	ProjectName        string                     `json:"projectName" validate:"required,min=1,max=63"`
	AgentName          string                     `json:"agentName" validate:"required,min=1,max=63"`
	EnvironmentName    string                     `json:"environmentName" validate:"required,min=1,max=63"`
	Evaluators         []MonitorEvaluator         `json:"evaluators" validate:"required,min=1,dive"`
	LLMProviderConfigs []MonitorLLMProviderConfig `json:"llmProviderConfigs,omitempty"`
	Type               string                     `json:"type" validate:"required,oneof=future past"`
	// Future monitor fields
	IntervalMinutes *int `json:"intervalMinutes,omitempty"`
	// Past monitor fields
	TraceStart *time.Time `json:"traceStart,omitempty"`
	TraceEnd   *time.Time `json:"traceEnd,omitempty"`
	// Common
	SamplingRate *float64 `json:"samplingRate,omitempty" validate:"omitempty,gt=0,lte=1"`
}

// UpdateMonitorRequest is the request body for updating a monitor
type UpdateMonitorRequest struct {
	DisplayName        *string                     `json:"displayName,omitempty" validate:"omitempty,min=1,max=128"`
	Evaluators         *[]MonitorEvaluator         `json:"evaluators,omitempty" validate:"omitempty,min=1,dive"`
	LLMProviderConfigs *[]MonitorLLMProviderConfig `json:"llmProviderConfigs,omitempty"`
	IntervalMinutes    *int                        `json:"intervalMinutes,omitempty"`
	TraceStart         *time.Time                  `json:"traceStart,omitempty"`
	TraceEnd           *time.Time                  `json:"traceEnd,omitempty"`
	SamplingRate       *float64                    `json:"samplingRate,omitempty" validate:"omitempty,gt=0,lte=1"`
}

// MonitorResponse is the API response for a monitor
type MonitorResponse struct {
	ID                 string                     `json:"id"`
	Name               string                     `json:"name"`
	DisplayName        string                     `json:"displayName"`
	Type               string                     `json:"type"`
	OrgName            string                     `json:"orgName"`
	ProjectName        string                     `json:"projectName"`
	AgentName          string                     `json:"agentName"`
	AgentID            string                     `json:"agentId"`
	EnvironmentID      string                     `json:"environmentId"`
	EnvironmentName    string                     `json:"environmentName"`
	Evaluators         []MonitorEvaluator         `json:"evaluators"`
	LLMProviderConfigs []MonitorLLMProviderConfig `json:"llmProviderConfigs,omitempty"`
	IntervalMinutes    *int                       `json:"intervalMinutes,omitempty"`
	NextRunTime        *time.Time                 `json:"nextRunTime,omitempty"`
	TraceStart         *time.Time                 `json:"traceStart,omitempty"`
	TraceEnd           *time.Time                 `json:"traceEnd,omitempty"`
	SamplingRate       float64                    `json:"samplingRate"`
	Status             MonitorStatus              `json:"status"`
	LatestRun          *MonitorRunResponse        `json:"latestRun,omitempty"`
	CreatedAt          time.Time                  `json:"createdAt"`
}

// MonitorListResponse is the API response for listing monitors
type MonitorListResponse struct {
	Monitors []MonitorResponse `json:"monitors"`
	Total    int               `json:"total"`
}

// MonitorRunResponse represents a single monitor execution run
type MonitorRunResponse struct {
	ID                 string                     `json:"id"`
	MonitorName        string                     `json:"monitorName,omitempty"`
	Evaluators         []MonitorEvaluator         `json:"evaluators"`
	LLMProviderConfigs []MonitorLLMProviderConfig `json:"llmProviderConfigs,omitempty"`
	TraceStart         time.Time                  `json:"traceStart"`
	TraceEnd           time.Time                  `json:"traceEnd"`
	StartedAt          *time.Time                 `json:"startedAt,omitempty"`
	CompletedAt        *time.Time                 `json:"completedAt,omitempty"`
	Status             string                     `json:"status"`
	ErrorMessage       *string                    `json:"errorMessage,omitempty"`
}

// MonitorRunsListResponse is the API response for listing monitor runs
type MonitorRunsListResponse struct {
	Runs  []MonitorRunResponse `json:"runs"`
	Total int                  `json:"total"`
}

// Default values
const (
	DefaultIntervalMinutes = 60
	MinIntervalMinutes     = 5
	DefaultSamplingRate    = 1.0
	SafetyDeltaPercent     = 0.05 // 5% of interval
	MonitorWorkflowName    = "monitor-evaluation-workflow"
)
