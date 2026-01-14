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

package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/clientmocks"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/wiring"
)

var (
	logsOrgName   = fmt.Sprintf("logs-org-%s", uuid.New().String()[:5])
	logsProjName  = fmt.Sprintf("logs-project-%s", uuid.New().String()[:5])
	logsAgentName = fmt.Sprintf("logs-agent-%s", uuid.New().String()[:5])
)

// createMockObservabilityClient creates a mock observability client for testing
func createMockObservabilityClient() *clientmocks.ObservabilitySvcClientMock {
	return &clientmocks.ObservabilitySvcClientMock{
		GetComponentLogsFunc: func(ctx context.Context, agentComponentId string, envId string, payload spec.LogFilterRequest) (*models.LogsResponse, error) {
			return &models.LogsResponse{
				Logs: []models.LogEntry{
					{
						Timestamp:     time.Now().Add(-30 * time.Minute),
						Log:           "Application started successfully",
						LogLevel:      "INFO",
						ComponentId:   agentComponentId,
						EnvironmentId: envId,
						ProjectId:     "project-123",
						Version:       "1.0.0",
						VersionId:     "version-123",
						Namespace:     "default",
						PodId:         "pod-abc-123",
						ContainerName: "agent-container",
					},
					{
						Timestamp:     time.Now().Add(-15 * time.Minute),
						Log:           "Processing request from user",
						LogLevel:      "DEBUG",
						ComponentId:   agentComponentId,
						EnvironmentId: envId,
						ProjectId:     "project-123",
						Version:       "1.0.0",
						VersionId:     "version-123",
						Namespace:     "default",
						PodId:         "pod-abc-123",
						ContainerName: "agent-container",
					},
					{
						Timestamp:     time.Now().Add(-5 * time.Minute),
						Log:           "Request completed successfully",
						LogLevel:      "INFO",
						ComponentId:   agentComponentId,
						EnvironmentId: envId,
						ProjectId:     "project-123",
						Version:       "1.0.0",
						VersionId:     "version-123",
						Namespace:     "default",
						PodId:         "pod-abc-123",
						ContainerName: "agent-container",
					},
				},
				TotalCount: 3,
				TookMs:     15.5,
			}, nil
		},
	}
}

// TestGetApplicationLogs tests the application logs endpoint
func TestGetApplicationLogs(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Getting application logs with valid parameters should return 200", func(t *testing.T) {
		observabilityClient := createMockObservabilityClient()
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient:    openChoreoClient,
			ObservabilitySvcClient: observabilityClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create request body
		startTime := time.Now().Add(-1 * time.Hour)
		endTime := time.Now()
		limit := int32(100)
		sortOrder := "desc"

		reqBody := new(bytes.Buffer)
		err := json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"environmentName": "Development",
			"startTime":       startTime.Format(time.RFC3339),
			"endTime":         endTime.Format(time.RFC3339),
			"limit":           limit,
			"sortOrder":       sortOrder,
		})
		require.NoError(t, err)

		// Send the request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName)
		req := httptest.NewRequest(http.MethodPost, url, reqBody)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusOK, rr.Code)

		// Read and validate response body
		b, err := io.ReadAll(rr.Body)
		require.NoError(t, err)
		t.Logf("response body: %s", string(b))

		var response models.LogsResponse
		require.NoError(t, json.Unmarshal(b, &response))

		// Validate response fields
		require.Equal(t, int32(3), response.TotalCount)
		require.Len(t, response.Logs, 3)

		// Validate first log entry
		log1 := response.Logs[0]
		require.Equal(t, "Application started successfully", log1.Log)
		require.Equal(t, "INFO", log1.LogLevel)

		// Validate service calls
		require.Len(t, observabilityClient.GetComponentLogsCalls(), 1)

		// Validate call parameters
		getLogsCall := observabilityClient.GetComponentLogsCalls()[0]
		require.Equal(t, "component-uid-123", getLogsCall.AgentComponentId)
		require.Equal(t, "environment-uid-123", getLogsCall.EnvId)
		require.Equal(t, "Development", getLogsCall.Payload.EnvironmentName)
	})

	t.Run("Getting application logs with log level filter should return 200", func(t *testing.T) {
		observabilityClient := createMockObservabilityClient()
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		openChoreoClient.GetAgentComponentFunc = func(ctx context.Context, orgName, projectName, agentName string) (*openchoreosvc.AgentComponent, error) {
			return &openchoreosvc.AgentComponent{
				UUID: "component-uid-123",
			}, nil
		}
		testClients := wiring.TestClients{
			OpenChoreoSvcClient:    openChoreoClient,
			ObservabilitySvcClient: observabilityClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create request body with log level filter
		startTime := time.Now().Add(-1 * time.Hour)
		endTime := time.Now()

		reqBody := new(bytes.Buffer)
		err := json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"environmentName": "Development",
			"startTime":       startTime.Format(time.RFC3339),
			"endTime":         endTime.Format(time.RFC3339),
			"logLevels":       []string{"INFO", "ERROR"},
		})
		require.NoError(t, err)

		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName)
		req := httptest.NewRequest(http.MethodPost, url, reqBody)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		// Validate service calls include log levels
		require.Len(t, observabilityClient.GetComponentLogsCalls(), 1)
		getLogsCall := observabilityClient.GetComponentLogsCalls()[0]
		require.Equal(t, []string{"INFO", "ERROR"}, getLogsCall.Payload.LogLevels)
	})

	t.Run("Getting application logs with search phrase should return 200", func(t *testing.T) {
		observabilityClient := createMockObservabilityClient()
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		openChoreoClient.GetAgentComponentFunc = func(ctx context.Context, orgName, projectName, agentName string) (*openchoreosvc.AgentComponent, error) {
			return &openchoreosvc.AgentComponent{
				UUID: "component-uid-123",
			}, nil
		}
		testClients := wiring.TestClients{
			OpenChoreoSvcClient:    openChoreoClient,
			ObservabilitySvcClient: observabilityClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create request body with search phrase
		startTime := time.Now().Add(-1 * time.Hour)
		endTime := time.Now()

		reqBody := new(bytes.Buffer)
		err := json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"environmentName": "Development",
			"startTime":       startTime.Format(time.RFC3339),
			"endTime":         endTime.Format(time.RFC3339),
			"searchPhrase":    "error",
		})
		require.NoError(t, err)

		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName)
		req := httptest.NewRequest(http.MethodPost, url, reqBody)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusOK, rr.Code)

		// Validate service calls include search phrase
		require.Len(t, observabilityClient.GetComponentLogsCalls(), 1)
		getLogsCall := observabilityClient.GetComponentLogsCalls()[0]
		require.NotNil(t, getLogsCall.Payload.SearchPhrase)
		require.Equal(t, "error", *getLogsCall.Payload.SearchPhrase)
	})

	getLogsTests := []struct {
		name           string
		authMiddleware jwtassertion.Middleware
		payload        map[string]interface{}
		wantStatus     int
		wantErrMsg     string
		url            string
		setupMock      func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock)
	}{
		{
			name:           "return 400 on invalid request body",
			authMiddleware: authMiddleware,
			payload:        map[string]interface{}{
				// Invalid JSON
			},
			wantStatus: 400,
			wantErrMsg: "environment is required",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				return createMockObservabilityClient(), apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 404 on non-existent agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"environmentName": "Development",
				"startTime":       time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				"endTime":         time.Now().Format(time.RFC3339),
			},
			wantStatus: 404,
			wantErrMsg: "Agent not found",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/nonexistent-agent/application-logs", logsOrgName, logsProjName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				obsClient := createMockObservabilityClient()
				openClient := apitestutils.CreateMockOpenChoreoClient()
				return obsClient, openClient
			},
		},
		{
			name:           "return 400 on missing environment name",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				// Missing environmentName
				"startTime": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				"endTime":   time.Now().Format(time.RFC3339),
			},
			wantStatus: 400,
			wantErrMsg: "environment",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				return createMockObservabilityClient(), apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on missing start time",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"environmentName": "Development",
				// Missing startTime
				"endTime": time.Now().Format(time.RFC3339),
			},
			wantStatus: 400,
			wantErrMsg: "startTime",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				return createMockObservabilityClient(), apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on missing end time",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"environmentName": "Development",
				"startTime":       time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				// Missing endTime
			},
			wantStatus: 400,
			wantErrMsg: "endTime",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				return createMockObservabilityClient(), apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on end time before start time",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"environmentName": "Development",
				"startTime":       time.Now().Format(time.RFC3339),
				"endTime":         time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
			},
			wantStatus: 400,
			wantErrMsg: "must be after startTime",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				return createMockObservabilityClient(), apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on start time in the future",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"environmentName": "Development",
				"startTime":       time.Now().Add(24 * time.Hour).Format(time.RFC3339),
				"endTime":         time.Now().Add(25 * time.Hour).Format(time.RFC3339),
			},
			wantStatus: 400,
			wantErrMsg: "startTime cannot be in the future",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				return createMockObservabilityClient(), apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on time range exceeding maximum days",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"environmentName": "Development",
				"startTime":       time.Now().Add(-20 * 24 * time.Hour).Format(time.RFC3339), // 20 days ago
				"endTime":         time.Now().Format(time.RFC3339),
			},
			wantStatus: 400,
			wantErrMsg: "time range cannot exceed",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				return createMockObservabilityClient(), apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on invalid limit (negative)",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"environmentName": "Development",
				"startTime":       time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				"endTime":         time.Now().Format(time.RFC3339),
				"limit":           -1,
			},
			wantStatus: 400,
			wantErrMsg: "limit must be between",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				return createMockObservabilityClient(), apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on invalid limit (exceeds maximum)",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"environmentName": "Development",
				"startTime":       time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				"endTime":         time.Now().Format(time.RFC3339),
				"limit":           utils.MaxLogLimit + 1,
			},
			wantStatus: 400,
			wantErrMsg: "limit must be between",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				return createMockObservabilityClient(), apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on invalid sort order",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"environmentName": "Development",
				"startTime":       time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				"endTime":         time.Now().Format(time.RFC3339),
				"sortOrder":       "ascending",
			},
			wantStatus: 400,
			wantErrMsg: "sortOrder must be",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				return createMockObservabilityClient(), apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name: "return 401 on missing authentication",
			authMiddleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					utils.WriteErrorResponse(w, http.StatusUnauthorized, "missing header: Authorization")
				})
			},
			payload: map[string]interface{}{
				"environmentName": "Development",
				"startTime":       time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				"endTime":         time.Now().Format(time.RFC3339),
			},
			wantStatus: 401,
			wantErrMsg: "missing header: Authorization",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/application-logs", logsOrgName, logsProjName, logsAgentName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				return createMockObservabilityClient(), apitestutils.CreateMockOpenChoreoClient()
			},
		},
	}

	for _, tt := range getLogsTests {
		t.Run(tt.name, func(t *testing.T) {
			obsClient, openClient := tt.setupMock()
			testClients := wiring.TestClients{
				OpenChoreoSvcClient:    openClient,
				ObservabilitySvcClient: obsClient,
			}

			app := apitestutils.MakeAppClientWithDeps(t, testClients, tt.authMiddleware)

			reqBody := new(bytes.Buffer)
			err := json.NewEncoder(reqBody).Encode(tt.payload)
			require.NoError(t, err)

			// Send the request
			req := httptest.NewRequest(http.MethodPost, tt.url, reqBody)
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			// Assert response
			require.Equal(t, tt.wantStatus, rr.Code)

			// Read response body and check error message
			body, err := io.ReadAll(rr.Body)
			require.NoError(t, err)

			if tt.wantStatus >= 400 {
				// For error responses, check that the error message is contained in the response
				bodyStr := string(body)
				require.Contains(t, bodyStr, tt.wantErrMsg)
			}
		})
	}
}
