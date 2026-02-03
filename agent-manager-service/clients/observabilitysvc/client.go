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
	"net/http"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/idp"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/requests"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
)

// Build log constants
const (
	BuildLogLevelInfo = "INFO"
	BuildLogTypeBuild = "BUILD"
)

//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/observability_client_fake.go . ObservabilitySvcClient:ObservabilitySvcClientMock

type ObservabilitySvcClient interface {
	GetBuildLogs(ctx context.Context, buildName string) (*models.LogsResponse, error)
	GetComponentMetrics(ctx context.Context, agentComponentId string, envId string, projectId string, payload spec.MetricsFilterRequest) (*models.MetricsResponse, error)
	GetComponentLogs(ctx context.Context, agentComponentId string, envId string, payload spec.LogFilterRequest) (*models.LogsResponse, error)
}

type observabilitySvcClient struct {
	httpClient    requests.HttpClient
	tokenProvider idp.TokenProvider
}

func NewObservabilitySvcClient() ObservabilitySvcClient {
	cfg := config.GetConfig()
	return &observabilitySvcClient{
		httpClient: &http.Client{
			Timeout: time.Second * 15,
		},
		tokenProvider: idp.NewTokenProvider(cfg.IDP),
	}
}

// GetBuildLogs retrieves build logs for a specific agent build from the observer service
func (o *observabilitySvcClient) GetBuildLogs(ctx context.Context, buildName string) (*models.LogsResponse, error) {
	// temporary use config to get observer URL since the observer url in dataplane is cluster svc name which is not accessible outside the cluster,
	// so we need to portforward the observer svc and use localhost:port to access the observer service
	baseURL := config.GetConfig().Observer.URL
	logsURL := fmt.Sprintf("%s/api/logs/build/%s", baseURL, buildName)

	token, err := o.tokenProvider.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetBuildLogs: failed to get token: %w", err)
	}

	// Calculate time range: 30 days ago to now
	endTime := time.Now()
	startTime := endTime.Add(-30 * 24 * time.Hour)

	requestBody := map[string]interface{}{
		"startTime": startTime.Format(time.RFC3339),
		"endTime":   endTime.Format(time.RFC3339),
		"limit":     1000,
		"sortOrder": "asc",
	}

	req := &requests.HttpRequest{
		Name:   "observabilitysvc.GetBuildLogs",
		URL:    logsURL,
		Method: http.MethodPost,
	}
	req.SetHeader("Accept", "application/json")
	req.SetHeader("Authorization", fmt.Sprintf("Bearer %s", token))
	req.SetJson(requestBody)

	var logsResponse models.LogsResponse
	if err := requests.SendRequest(ctx, o.httpClient, req).ScanResponse(&logsResponse, http.StatusOK); err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetBuildLogs: %w", err)
	}

	return &logsResponse, nil
}

func (o *observabilitySvcClient) GetComponentMetrics(ctx context.Context, agentComponentId string, envId string, projectId string, payload spec.MetricsFilterRequest) (*models.MetricsResponse, error) {
	baseURL := config.GetConfig().Observer.URL
	metricsURL := fmt.Sprintf("%s/api/metrics/component/usage", baseURL)
	token, err := o.tokenProvider.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetBuildLogs: failed to get token: %w", err)
	}

	requestBody := map[string]interface{}{
		"componentId":   agentComponentId,
		"startTime":     payload.StartTime,
		"endTime":       payload.EndTime,
		"environmentId": envId,
		"projectId":     projectId,
	}

	req := &requests.HttpRequest{
		Name:   "observabilitysvc.GetComponentMetrics",
		URL:    metricsURL,
		Method: http.MethodPost,
	}
	req.SetHeader("Accept", "application/json")
	req.SetHeader("Authorization", fmt.Sprintf("Bearer %s", token))
	req.SetJson(requestBody)

	var metricsResponse models.MetricsResponse
	if err := requests.SendRequest(ctx, o.httpClient, req).ScanResponse(&metricsResponse, http.StatusOK); err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetComponentMetrics: %w", err)
	}

	return &metricsResponse, nil
}

func (o *observabilitySvcClient) GetComponentLogs(ctx context.Context, agentComponentId string, envId string, payload spec.LogFilterRequest) (*models.LogsResponse, error) {
	baseURL := config.GetConfig().Observer.URL
	logsURL := fmt.Sprintf("%s/api/logs/component/%s", baseURL, agentComponentId)
	token, err := o.tokenProvider.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetBuildLogs: failed to get token: %w", err)
	}

	requestBody := map[string]interface{}{
		"startTime":     payload.StartTime,
		"endTime":       payload.EndTime,
		"searchPhrase":  payload.SearchPhrase,
		"logLevels":     payload.LogLevels,
		"limit":         payload.Limit,
		"sortOrder":     payload.SortOrder,
		"componentId":   agentComponentId,
		"environmentId": envId,
		"logType":       "RUNTIME",
	}

	req := &requests.HttpRequest{
		Name:   "observabilitysvc.GetApplicationLogs",
		URL:    logsURL,
		Method: http.MethodPost,
	}
	req.SetHeader("Accept", "application/json")
	req.SetHeader("Authorization", fmt.Sprintf("Bearer %s", token))
	req.SetJson(requestBody)

	var logsResponse models.LogsResponse
	if err := requests.SendRequest(ctx, o.httpClient, req).ScanResponse(&logsResponse, http.StatusOK); err != nil {
		return nil, fmt.Errorf("observabilitysvc.GetApplicationLogs: %w", err)
	}

	return &logsResponse, nil
}
