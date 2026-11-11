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

package controllers

import (
	"context"
	"errors"
	"time"
)

// ErrNotImplemented is returned for runtime logs, build logs and metrics,
// which only existed for platform-hosted agents.
var ErrNotImplemented = errors.New("not implemented: runtime logs, build logs and metrics are not available for external agents")

// ObservabilityController serves log and metrics queries.
type ObservabilityController struct{}

// NewObservabilityController creates a new observability controller.
func NewObservabilityController() *ObservabilityController {
	return &ObservabilityController{}
}

// LogsQueryParams holds parameters for component log queries.
type LogsQueryParams struct {
	Organization string
	Project      string
	Agent        string
	StartTime    time.Time
	EndTime      time.Time
	SearchPhrase string
	LogLevels    []string
	Limit        *int
	SortOrder    string
}

// MetricsQueryParams holds parameters for resource metrics queries.
type MetricsQueryParams struct {
	Organization string
	Project      string
	Agent        string
	StartTime    time.Time
	EndTime      time.Time
}

// LogEntry is a single log line in a LogsResponse.
type LogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Log       string    `json:"log"`
	LogLevel  string    `json:"logLevel"`
}

// LogsResponse is the response for log query endpoints.
type LogsResponse struct {
	Logs       []LogEntry `json:"logs"`
	TotalCount int32      `json:"totalCount"`
	TookMs     float32    `json:"tookMs"`
}

// MetricDataPoint is a single timestamped value in a metrics time series.
type MetricDataPoint struct {
	Time  string  `json:"time"`
	Value float64 `json:"value"`
}

// MetricsResponse is the response for the resource metrics endpoint.
type MetricsResponse struct {
	CpuUsage       []MetricDataPoint `json:"cpuUsage"`
	CpuRequests    []MetricDataPoint `json:"cpuRequests"`
	CpuLimits      []MetricDataPoint `json:"cpuLimits"`
	Memory         []MetricDataPoint `json:"memory"`
	MemoryRequests []MetricDataPoint `json:"memoryRequests"`
	MemoryLimits   []MetricDataPoint `json:"memoryLimits"`
}

// GetLogs returns ErrNotImplemented.
func (c *ObservabilityController) GetLogs(_ context.Context, _ LogsQueryParams) (*LogsResponse, error) {
	return nil, ErrNotImplemented
}

// GetBuildLogs returns ErrNotImplemented.
func (c *ObservabilityController) GetBuildLogs(_ context.Context, _, _ string) (*LogsResponse, error) {
	return nil, ErrNotImplemented
}

// GetMetrics returns ErrNotImplemented.
func (c *ObservabilityController) GetMetrics(_ context.Context, _ MetricsQueryParams) (*MetricsResponse, error) {
	return nil, ErrNotImplemented
}
