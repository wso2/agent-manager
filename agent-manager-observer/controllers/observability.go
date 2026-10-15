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
	"encoding/json"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/observer"
)

const (
	// buildLogsWindow is the fixed lookback window GetBuildLogs queries over.
	buildLogsWindow = 30 * 24 * time.Hour
	// buildLogsLimit is the fixed max number of log lines GetBuildLogs fetches.
	buildLogsLimit = 1000
	// workflowLogsSortOrder is the sort order every workflow-log query
	// requests (oldest first).
	workflowLogsSortOrder = "asc"
	// maxMetricsPoints caps how many points a metrics series may have; the
	// step is the smallest entry of metricsSteps that stays within it.
	maxMetricsPoints = 200
)

// DefaultWorkflowLogsWindow is the lookback a caller falls back to when it
// cannot supply an explicit range for GetMonitorRunLogs — an MCP client has no
// access to the monitor_runs table and so cannot know a run's timestamps.
const DefaultWorkflowLogsWindow = buildLogsWindow

// metricsSteps are the query resolutions GetMetrics chooses between.
var metricsSteps = []time.Duration{
	30 * time.Second, time.Minute, 2 * time.Minute, 5 * time.Minute,
	10 * time.Minute, 15 * time.Minute, 30 * time.Minute, time.Hour,
	2 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour,
}

// ObservabilityController provides log and metrics functionality via the observer service.
type ObservabilityController struct {
	observerClient observer.Client
}

// NewObservabilityController creates a new observability controller.
func NewObservabilityController(observerClient observer.Client) *ObservabilityController {
	return &ObservabilityController{observerClient: observerClient}
}

// LogsQueryParams holds parameters for component log queries.
type LogsQueryParams struct {
	Organization string
	Project      string
	Agent        string
	Environment  string
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
	Environment  string
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

// GetLogs fetches component log entries matching the given params.
func (c *ObservabilityController) GetLogs(ctx context.Context, params LogsQueryParams) (*LogsResponse, error) {
	req := observer.LogsQueryRequest{
		StartTime: params.StartTime,
		EndTime:   params.EndTime,
		Limit:     params.Limit,
		LogLevels: params.LogLevels,
		SearchScope: observer.ComponentSearchScope{
			Namespace:   c.observerClient.NamespaceFor(params.Organization),
			Project:     &params.Project,
			Component:   &params.Agent,
			Environment: &params.Environment,
		},
	}
	if params.SearchPhrase != "" {
		req.SearchPhrase = &params.SearchPhrase
	}
	if params.SortOrder != "" {
		req.SortOrder = &params.SortOrder
	}

	resp, err := c.observerClient.QueryLogs(ctx, req)
	if err != nil {
		return nil, err
	}

	return convertLogsQueryResponse(resp), nil
}

// queryWorkflowLogs fetches log entries for a single Argo workflow run over an
// explicit window, oldest-first.
func (c *ObservabilityController) queryWorkflowLogs(
	ctx context.Context, organization, runName string,
	startTime, endTime time.Time, limit int,
) (*LogsResponse, error) {
	sortOrder := workflowLogsSortOrder

	req := observer.LogsQueryRequest{
		StartTime: startTime,
		EndTime:   endTime,
		Limit:     &limit,
		SortOrder: &sortOrder,
		SearchScope: observer.WorkflowSearchScope{
			Namespace:       c.observerClient.NamespaceFor(organization),
			WorkflowRunName: &runName,
		},
	}

	resp, err := c.observerClient.QueryLogs(ctx, req)
	if err != nil {
		return nil, err
	}

	return convertLogsQueryResponse(resp), nil
}

// GetBuildLogs fetches workflow log entries for an Argo build run, over a
// fixed 30-day window, oldest-first, capped at 1000 lines.
func (c *ObservabilityController) GetBuildLogs(ctx context.Context, organization, buildName string) (*LogsResponse, error) {
	endTime := time.Now()
	return c.queryWorkflowLogs(ctx, organization, buildName, endTime.Add(-buildLogsWindow), endTime, buildLogsLimit)
}

// GetMonitorRunLogs fetches workflow log entries for a monitor run over the
// window the caller asked for. Unlike build runs, a monitor run can be
// arbitrarily old, so the window is the caller's to choose.
func (c *ObservabilityController) GetMonitorRunLogs(
	ctx context.Context, organization, runName string,
	startTime, endTime time.Time, limit int,
) (*LogsResponse, error) {
	return c.queryWorkflowLogs(ctx, organization, runName, startTime, endTime, limit)
}

// GetMetrics fetches a resource metrics time series matching the given params.
// The window is snapped to multiples of the step so repeated queries over a
// moving window evaluate at the same timestamps and past points stay put.
func (c *ObservabilityController) GetMetrics(ctx context.Context, params MetricsQueryParams) (*MetricsResponse, error) {
	step := metricsStep(params.EndTime.Sub(params.StartTime))
	req := observer.MetricsQueryRequest{
		StartTime: params.StartTime.Truncate(step),
		EndTime:   params.EndTime.Truncate(step),
		Metric:    "resource",
		Step:      step.String(),
		SearchScope: observer.ComponentSearchScope{
			Namespace:   c.observerClient.NamespaceFor(params.Organization),
			Project:     &params.Project,
			Component:   &params.Agent,
			Environment: &params.Environment,
		},
	}

	resp, err := c.observerClient.QueryMetrics(ctx, req)
	if err != nil {
		return nil, err
	}

	return &MetricsResponse{
		CpuUsage:       convertTimeSeries(resp.CpuUsage),
		CpuRequests:    convertTimeSeries(resp.CpuRequests),
		CpuLimits:      convertTimeSeries(resp.CpuLimits),
		Memory:         convertTimeSeries(resp.MemoryUsage),
		MemoryRequests: convertTimeSeries(resp.MemoryRequests),
		MemoryLimits:   convertTimeSeries(resp.MemoryLimits),
	}, nil
}

// metricsStep returns the smallest step that keeps a window within
// maxMetricsPoints points, or the largest step for longer windows.
func metricsStep(window time.Duration) time.Duration {
	for _, step := range metricsSteps {
		if window/step <= maxMetricsPoints {
			return step
		}
	}
	return metricsSteps[len(metricsSteps)-1]
}

// convertLogsQueryResponse converts an upstream LogsQueryResponse into the
// wire LogsResponse. TotalCount falls back to len(logs) when the upstream
// Total is nil; TookMs falls back to 0 when the upstream TookMs is nil.
func convertLogsQueryResponse(resp *observer.LogsQueryResponse) *LogsResponse {
	logs := make([]LogEntry, 0, len(resp.Logs))
	for _, l := range resp.Logs {
		logs = append(logs, convertLogEntry(l))
	}

	result := &LogsResponse{Logs: logs}
	if resp.Total != nil {
		result.TotalCount = int32(*resp.Total)
	} else {
		result.TotalCount = int32(len(logs))
	}
	if resp.TookMs != nil {
		result.TookMs = float32(*resp.TookMs)
	}
	return result
}

// convertLogEntry ports convertComponentLogEntry from
// agent-manager-service/clients/observabilitysvc/client.go: copy the
// timestamp/log/level, then if the log line is JSON-formatted, extract the
// "msg" field (non-empty string replaces Log), and fall back to the parsed
// "level" (uppercased) or "time" (RFC3339, then "2006-01-02T15:04:05") only
// when the corresponding field wasn't already set upstream.
func convertLogEntry(l observer.LogEntry) LogEntry {
	entry := LogEntry{}
	if l.Timestamp != nil {
		entry.Timestamp = *l.Timestamp
	}
	if l.Log != nil {
		entry.Log = *l.Log
	}
	if l.Level != nil {
		entry.LogLevel = *l.Level
	}

	if l.Log == nil || !strings.HasPrefix(strings.TrimSpace(*l.Log), "{") {
		return entry
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal([]byte(*l.Log), &parsed); err != nil {
		return entry
	}

	if msg, ok := parsed["msg"]; ok {
		if msgStr, ok := msg.(string); ok && msgStr != "" {
			entry.Log = msgStr
		}
	}
	if entry.LogLevel == "" {
		if lvl, ok := parsed["level"]; ok {
			if lvlStr, ok := lvl.(string); ok {
				entry.LogLevel = strings.ToUpper(lvlStr)
			}
		}
	}
	if entry.Timestamp.IsZero() {
		if ts, ok := parsed["time"]; ok {
			if tsStr, ok := ts.(string); ok {
				if t, err := time.Parse(time.RFC3339, tsStr); err == nil {
					entry.Timestamp = t
				} else if t, err := time.Parse("2006-01-02T15:04:05", tsStr); err == nil {
					entry.Timestamp = t
				}
			}
		}
	}

	return entry
}

// convertTimeSeries ports convertTimeSeriesData: a nil upstream series
// converts to an empty (never nil) slice; each point's timestamp is
// formatted as RFC3339.
func convertTimeSeries(data *[]observer.MetricsTimeSeriesItem) []MetricDataPoint {
	if data == nil {
		return []MetricDataPoint{}
	}

	result := make([]MetricDataPoint, 0, len(*data))
	for _, point := range *data {
		var timeStr string
		if point.Timestamp != nil {
			timeStr = point.Timestamp.Format(time.RFC3339)
		}
		var value float64
		if point.Value != nil {
			value = *point.Value
		}
		result = append(result, MetricDataPoint{Time: timeStr, Value: value})
	}
	return result
}
