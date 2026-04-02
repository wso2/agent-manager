// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package observabilitysvc

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/observabilitysvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/requests"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Build log constants
const (
	BuildLogLevelInfo = "INFO"
	BuildLogTypeBuild = "BUILD"
)

//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/observability_client_fake.go . ObservabilitySvcClient:ObservabilitySvcClientMock

// BuildLogsParams holds the context information needed for fetching build logs
type BuildLogsParams struct {
	NamespaceName      string
	ProjectName        string
	AgentComponentName string
	BuildName          string
}

// ComponentMetricsParams holds the component context information needed for fetching metrics
type ComponentMetricsParams struct {
	AgentComponentId string
	EnvId            string
	ProjectId        string
	NamespaceName    string
	ProjectName      string
	ComponentName    string
	EnvironmentName  string
}

// ComponentLogsParams holds the component context information needed for fetching logs
type ComponentLogsParams struct {
	AgentComponentId string
	EnvId            string
	NamespaceName    string
	ComponentName    string
	ProjectName      string
	EnvironmentName  string
}

type ObservabilitySvcClient interface {
	GetBuildLogs(ctx context.Context, params BuildLogsParams) (*models.LogsResponse, error)
	GetWorkflowRunLogs(ctx context.Context, workflowRunName string, namespaceName string) (*models.LogsResponse, error)
	GetComponentMetrics(ctx context.Context, params ComponentMetricsParams, payload spec.MetricsFilterRequest) (*models.MetricsResponse, error)
	GetComponentLogs(ctx context.Context, params ComponentLogsParams, payload spec.LogFilterRequest) (*models.LogsResponse, error)
}

// Config contains configuration for the observability service client
type Config struct {
	BaseURL      string
	AuthProvider client.AuthProvider
	RetryConfig  requests.RequestRetryConfig
}

type observabilitySvcClient struct {
	baseURL        string
	observerClient *gen.ClientWithResponses
	httpClient     requests.HttpClient
	authProvider   client.AuthProvider
}

func NewObservabilitySvcClient(cfg *Config) (ObservabilitySvcClient, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("base URL is required")
	}
	if cfg.AuthProvider == nil {
		return nil, fmt.Errorf("auth provider is required")
	}

	// Configure retry behavior to handle 401 Unauthorized by invalidating the token
	retryConfig := cfg.RetryConfig
	if retryConfig.RetryOnStatus == nil {
		// Custom retry logic that includes 401 handling + default transient errors
		retryConfig.RetryOnStatus = func(statusCode int) bool {
			// Handle 401 by invalidating cached token and retrying
			if statusCode == http.StatusUnauthorized {
				slog.Info("Received 401 Unauthorized, invalidating cached token")
				cfg.AuthProvider.InvalidateToken()
				return true
			}
			return slices.Contains(requests.TransientHTTPErrorCodes, statusCode)
		}
	}

	httpClient := requests.NewRetryableHTTPClient(&http.Client{}, retryConfig)

	// Auth editor function - called before every request
	authEditor := func(ctx context.Context, req *http.Request) error {
		slog.Debug("Adding auth token to observer request")
		token, err := cfg.AuthProvider.GetToken(ctx)
		if err != nil {
			return fmt.Errorf("failed to get auth token: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		return nil
	}

	// Create the generated observer client with auth and retries
	observerClient, err := gen.NewClientWithResponses(
		cfg.BaseURL,
		gen.WithHTTPClient(httpClient),
		gen.WithRequestEditorFn(authEditor),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create observer client: %w", err)
	}

	return &observabilitySvcClient{
		baseURL:        cfg.BaseURL,
		observerClient: observerClient,
	}, nil
}

// GetBuildLogs retrieves build logs for a specific agent build from the observer service
func (o *observabilitySvcClient) GetBuildLogs(ctx context.Context, params BuildLogsParams) (*models.LogsResponse, error) {
	// Calculate time range: 30 days ago to now
	endTime := time.Now()
	startTime := endTime.Add(-30 * 24 * time.Hour)

	sortOrder := gen.LogsQueryRequestSortOrderAsc
	var searchScope gen.LogsQueryRequest_SearchScope
	if err := searchScope.FromWorkflowSearchScope(gen.WorkflowSearchScope{
		Namespace:       params.NamespaceName,
		WorkflowRunName: &params.BuildName,
	}); err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetBuildLogs: failed to create search scope: %w", err)
	}

	requestBody := gen.LogsQueryRequest{
		StartTime:   startTime,
		EndTime:     endTime,
		Limit:       utils.IntAsIntPointer(1000),
		SortOrder:   &sortOrder,
		SearchScope: searchScope,
	}

	resp, err := o.observerClient.QueryLogsWithResponse(ctx, requestBody)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetBuildLogs: request failed: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("observabilitysvc.GetBuildLogs: failed with status code %d [%s]", resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("observabilitysvc.GetBuildLogs: empty response body")
	}

	return convertLogsQueryResponseToLogsResponse(resp.JSON200), nil
}

// GetWorkflowRunLogs retrieves workflow run logs for a specific workflow execution from the observer service
func (o *observabilitySvcClient) GetWorkflowRunLogs(ctx context.Context, workflowRunName string, namespaceName string) (*models.LogsResponse, error) {
	// Calculate time range: 30 days ago to now
	endTime := time.Now()
	startTime := endTime.Add(-30 * 24 * time.Hour)

	sortOrder := gen.LogsQueryRequestSortOrderAsc
	var searchScope gen.LogsQueryRequest_SearchScope
	if err := searchScope.FromWorkflowSearchScope(gen.WorkflowSearchScope{
		Namespace:       namespaceName,
		WorkflowRunName: &workflowRunName,
	}); err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetWorkflowRunLogs: failed to create search scope: %w", err)
	}

	requestBody := gen.LogsQueryRequest{
		StartTime:   startTime,
		EndTime:     endTime,
		Limit:       utils.IntAsIntPointer(1000),
		SortOrder:   &sortOrder,
		SearchScope: searchScope,
	}

	resp, err := o.observerClient.QueryLogsWithResponse(ctx, requestBody)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetWorkflowRunLogs: request failed: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("observabilitysvc.GetWorkflowRunLogs: failed with status code %d [%s]", resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("observabilitysvc.GetWorkflowRunLogs: empty response body")
	}

	return convertLogsQueryResponseToLogsResponse(resp.JSON200), nil
}

func (o *observabilitySvcClient) GetComponentMetrics(ctx context.Context, params ComponentMetricsParams, payload spec.MetricsFilterRequest) (*models.MetricsResponse, error) {
	startTime, err := time.Parse(time.RFC3339, payload.StartTime)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentMetrics: invalid startTime: %w", err)
	}

	endTime, err := time.Parse(time.RFC3339, payload.EndTime)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentMetrics: invalid endTime: %w", err)
	}

	requestBody := gen.MetricsQueryRequest{
		StartTime: startTime,
		EndTime:   endTime,
		Metric:    gen.Resource,
		SearchScope: gen.ComponentSearchScope{
			Namespace:   params.NamespaceName,
			Project:     &params.ProjectName,
			Component:   &params.ComponentName,
			Environment: &params.EnvironmentName,
		},
	}

	resp, err := o.observerClient.QueryMetricsWithResponse(ctx, requestBody)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentMetrics: request failed: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("observabilitysvc.GetComponentMetrics: failed with status code %d [%s]", resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentMetrics: empty response body")
	}

	return convertMetricsQueryResponseToMetricsResponse(resp.JSON200)
}

func (o *observabilitySvcClient) GetComponentLogs(ctx context.Context, params ComponentLogsParams, payload spec.LogFilterRequest) (*models.LogsResponse, error) {
	startTime, err := time.Parse(time.RFC3339, payload.StartTime)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentLogs: invalid startTime: %w", err)
	}

	endTime, err := time.Parse(time.RFC3339, payload.EndTime)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentLogs: invalid endTime: %w", err)
	}

	var searchScope gen.LogsQueryRequest_SearchScope
	if err := searchScope.FromComponentSearchScope(gen.ComponentSearchScope{
		Namespace:   params.NamespaceName,
		Project:     &params.ProjectName,
		Component:   &params.ComponentName,
		Environment: &params.EnvironmentName,
	}); err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentLogs: failed to create search scope: %w", err)
	}

	requestBody := gen.LogsQueryRequest{
		StartTime:    startTime,
		EndTime:      endTime,
		SearchPhrase: payload.SearchPhrase,
		LogLevels:    convertLogLevels(payload.LogLevels),
		Limit:        convertInt32PtrToIntPtr(payload.Limit),
		SortOrder:    convertSortOrder(payload.SortOrder),
		SearchScope:  searchScope,
	}

	resp, err := o.observerClient.QueryLogsWithResponse(ctx, requestBody)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentLogs: request failed: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("observabilitysvc.GetComponentLogs: failed with status code %d [%s]", resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentLogs: empty response body")
	}

	return convertLogsQueryResponseToLogsResponse(resp.JSON200), nil
}

func convertLogsQueryResponseToLogsResponse(resp *gen.LogsQueryResponse) *models.LogsResponse {
	result := &models.LogsResponse{
		Logs:       make([]models.LogEntry, 0),
		TotalCount: 0,
		TookMs:     0,
	}

	if resp.Total != nil {
		result.TotalCount = int32(*resp.Total)
	}

	if resp.TookMs != nil {
		result.TookMs = float32(*resp.TookMs)
	}

	if resp.Logs == nil {
		return result
	}

	// Try to parse as component logs first
	componentLogs, err := resp.Logs.AsLogsQueryResponseLogs0()
	if err == nil && len(componentLogs) > 0 {
		for _, log := range componentLogs {
			entry := convertComponentLogEntry(&log)
			result.Logs = append(result.Logs, entry)
		}
		return result
	}

	// Try to parse as workflow logs
	workflowLogs, err := resp.Logs.AsLogsQueryResponseLogs1()
	if err == nil && len(workflowLogs) > 0 {
		for _, log := range workflowLogs {
			entry := convertWorkflowLogEntry(&log)
			result.Logs = append(result.Logs, entry)
		}
	}

	return result
}

func convertComponentLogEntry(log *gen.ComponentLogEntry) models.LogEntry {
	entry := models.LogEntry{}
	if log.Timestamp != nil {
		entry.Timestamp = *log.Timestamp
	}
	if log.Log != nil {
		entry.Log = *log.Log
	}
	if log.Level != nil {
		entry.LogLevel = *log.Level
	}

	// Try to parse JSON-formatted log lines and extract the message.
	// Many containers (e.g. evaluation jobs) emit structured JSON logs like:
	//   {"time": "...", "level": "INFO", "msg": "actual message", "logger": "..."}
	// We extract the "msg" field so the frontend shows the human-readable message
	// instead of the raw JSON string.
	if log.Log != nil && strings.HasPrefix(strings.TrimSpace(*log.Log), "{") {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(*log.Log), &parsed); err == nil {
			if msg, ok := parsed["msg"]; ok {
				if msgStr, ok := msg.(string); ok && msgStr != "" {
					entry.Log = msgStr
				}
			}
			// Use parsed level/time as fallback when the observer didn't extract them
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
		}
	}

	return entry
}

func convertWorkflowLogEntry(log *gen.WorkflowLogEntry) models.LogEntry {
	entry := models.LogEntry{}
	if log.Timestamp != nil {
		entry.Timestamp = *log.Timestamp
	}
	if log.Log != nil {
		entry.Log = *log.Log
	}

	// Try to parse JSON-formatted log lines and extract the message
	if log.Log != nil && strings.HasPrefix(strings.TrimSpace(*log.Log), "{") {
		var parsed map[string]interface{}
		if err := json.Unmarshal([]byte(*log.Log), &parsed); err == nil {
			if msg, ok := parsed["msg"]; ok {
				if msgStr, ok := msg.(string); ok && msgStr != "" {
					entry.Log = msgStr
				}
			}
			if lvl, ok := parsed["level"]; ok {
				if lvlStr, ok := lvl.(string); ok {
					entry.LogLevel = strings.ToUpper(lvlStr)
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
		}
	}

	return entry
}

func convertInt32PtrToIntPtr(val *int32) *int {
	if val == nil {
		return nil
	}
	intVal := int(*val)
	return &intVal
}

func convertMetricsQueryResponseToMetricsResponse(resp *gen.MetricsQueryResponse) (*models.MetricsResponse, error) {
	resourceMetrics, err := resp.AsResourceMetricsTimeSeries()
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource metrics: %w", err)
	}

	result := &models.MetricsResponse{
		CpuUsage:       convertTimeSeriesData(resourceMetrics.CpuUsage),
		CpuRequests:    convertTimeSeriesData(resourceMetrics.CpuRequests),
		CpuLimits:      convertTimeSeriesData(resourceMetrics.CpuLimits),
		Memory:         convertTimeSeriesData(resourceMetrics.MemoryUsage),
		MemoryRequests: convertTimeSeriesData(resourceMetrics.MemoryRequests),
		MemoryLimits:   convertTimeSeriesData(resourceMetrics.MemoryLimits),
	}
	return result, nil
}

func convertTimeSeriesData(data *[]gen.MetricsTimeSeriesItem) []models.TimeValuePoint {
	if data == nil {
		return []models.TimeValuePoint{}
	}

	result := make([]models.TimeValuePoint, 0, len(*data))
	for _, point := range *data {
		timeStr := ""
		if point.Timestamp != nil {
			timeStr = point.Timestamp.Format(time.RFC3339)
		}
		value := 0.0
		if point.Value != nil {
			value = *point.Value
		}
		result = append(result, models.TimeValuePoint{
			Time:  timeStr,
			Value: value,
		})
	}
	return result
}

func convertLogLevels(levels []string) *[]gen.LogsQueryRequestLogLevels {
	if len(levels) == 0 {
		return nil
	}
	result := make([]gen.LogsQueryRequestLogLevels, 0, len(levels))
	for _, level := range levels {
		result = append(result, gen.LogsQueryRequestLogLevels(level))
	}
	return &result
}

func convertSortOrder(sortOrder *string) *gen.LogsQueryRequestSortOrder {
	if sortOrder == nil {
		return nil
	}
	order := gen.LogsQueryRequestSortOrder(*sortOrder)
	return &order
}
