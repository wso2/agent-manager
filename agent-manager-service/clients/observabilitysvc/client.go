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
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/observabilitysvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/requests"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
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

	sortOrder := gen.BuildLogsRequestSortOrderAsc
	requestBody := gen.BuildLogsRequest{
		ComponentName: params.AgentComponentName,
		NamespaceName: params.NamespaceName,
		ProjectName:   params.ProjectName,
		StartTime:     startTime,
		EndTime:       endTime,
		Limit:         utils.IntAsIntPointer(1000),
		SortOrder:     &sortOrder,
	}

	resp, err := o.observerClient.GetBuildLogsWithResponse(ctx, params.BuildName, requestBody)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetBuildLogs: request failed: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("observabilitysvc.GetBuildLogs: failed with status code %d [%s]", resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("observabilitysvc.GetBuildLogs: empty response body")
	}

	return convertToLogsResponse(resp.JSON200), nil
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

	requestBody := gen.MetricsRequest{
		NamespaceName:   params.NamespaceName,
		ProjectName:     params.ProjectName,
		ComponentName:   params.ComponentName,
		EnvironmentName: params.EnvironmentName,
		ComponentId:     params.AgentComponentId,
		EnvironmentId:   params.EnvId,
		ProjectId:       params.ProjectId,
		StartTime:       &startTime,
		EndTime:         &endTime,
	}

	resp, err := o.observerClient.GetComponentResourceMetricsWithResponse(ctx, requestBody)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentMetrics: request failed: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("observabilitysvc.GetComponentMetrics: failed with status code %d [%s]", resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentMetrics: empty response body")
	}

	return convertToMetricsResponse(resp.JSON200), nil
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

	logType := gen.ComponentLogsRequestLogTypeRuntime
	requestBody := gen.ComponentLogsRequest{
		NamespaceName:   params.NamespaceName,
		ComponentName:   params.ComponentName,
		ProjectName:     params.ProjectName,
		EnvironmentId:   params.EnvId,
		EnvironmentName: params.EnvironmentName,
		StartTime:       startTime,
		EndTime:         endTime,
		SearchPhrase:    payload.SearchPhrase,
		LogLevels:       &payload.LogLevels,
		Limit:           convertInt32PtrToIntPtr(payload.Limit),
		SortOrder:       (*gen.ComponentLogsRequestSortOrder)(payload.SortOrder),
		LogType:         &logType,
	}

	resp, err := o.observerClient.GetComponentLogsWithResponse(ctx, params.AgentComponentId, requestBody)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentLogs: request failed: %w", err)
	}

	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("observabilitysvc.GetComponentLogs: failed with status code %d [%s]", resp.StatusCode(), string(resp.Body))
	}

	if resp.JSON200 == nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentLogs: empty response body")
	}

	return convertToLogsResponse(resp.JSON200), nil
}

func convertToLogsResponse(resp *gen.LogResponse) *models.LogsResponse {
	result := &models.LogsResponse{
		Logs:       make([]models.LogEntry, 0),
		TotalCount: 0,
		TookMs:     0,
	}

	if resp.TotalCount != nil {
		result.TotalCount = int32(*resp.TotalCount)
	}

	if resp.TookMs != nil {
		result.TookMs = float32(*resp.TookMs)
	}

	if resp.Logs != nil {
		for _, log := range *resp.Logs {
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
			result.Logs = append(result.Logs, entry)
		}
	}

	return result
}

func convertInt32PtrToIntPtr(val *int32) *int {
	if val == nil {
		return nil
	}
	intVal := int(*val)
	return &intVal
}

func convertToMetricsResponse(resp *gen.ResourceMetricsTimeSeries) *models.MetricsResponse {
	result := &models.MetricsResponse{
		CpuUsage:       convertTimeSeriesData(resp.CpuUsage),
		CpuRequests:    convertTimeSeriesData(resp.CpuRequests),
		CpuLimits:      convertTimeSeriesData(resp.CpuLimits),
		Memory:         convertTimeSeriesData(resp.Memory),
		MemoryRequests: convertTimeSeriesData(resp.MemoryRequests),
		MemoryLimits:   convertTimeSeriesData(resp.MemoryLimits),
	}
	return result
}

func convertTimeSeriesData(data *[]gen.TimeValuePoint) []models.TimeValuePoint {
	if data == nil {
		return []models.TimeValuePoint{}
	}

	result := make([]models.TimeValuePoint, 0, len(*data))
	for _, point := range *data {
		timeStr := ""
		if point.Time != nil {
			timeStr = point.Time.Format(time.RFC3339)
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
