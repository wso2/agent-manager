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
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/wiring"
)

var (
	buildLogsOrgName   = fmt.Sprintf("build-org-%s", uuid.New().String()[:5])
	buildLogsProjName  = fmt.Sprintf("build-project-%s", uuid.New().String()[:5])
	buildLogsAgentName = fmt.Sprintf("build-agent-%s", uuid.New().String()[:5])
	buildLogsBuildName = fmt.Sprintf("build-%s", uuid.New().String()[:5])
)

// createMockObservabilityClientForBuildLogs creates a mock observability client for build logs testing
func createMockObservabilityClientForBuildLogs() *clientmocks.ObservabilitySvcClientMock {
	return &clientmocks.ObservabilitySvcClientMock{
		GetBuildLogsFunc: func(ctx context.Context, buildName string) (*models.LogsResponse, error) {
			return &models.LogsResponse{
				Logs: []models.LogEntry{
					{
						Timestamp: time.Now().Add(-10 * time.Minute),
						Log:       "Starting build process",
						LogLevel:  "INFO",
					},
					{
						Timestamp: time.Now().Add(-8 * time.Minute),
						Log:       "Cloning repository",
						LogLevel:  "INFO",
					},
					{
						Timestamp: time.Now().Add(-5 * time.Minute),
						Log:       "Installing dependencies",
						LogLevel:  "INFO",
					},
					{
						Timestamp: time.Now().Add(-2 * time.Minute),
						Log:       "Building application",
						LogLevel:  "INFO",
					},
					{
						Timestamp: time.Now().Add(-1 * time.Minute),
						Log:       "Build completed successfully",
						LogLevel:  "INFO",
					},
				},
				TotalCount: 5,
				TookMs:     25.3,
			}, nil
		},
	}
}

// TestGetBuildLogs tests the build logs endpoint
func TestGetBuildLogs(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Getting build logs with valid parameters should return 200", func(t *testing.T) {
		observabilityClient := createMockObservabilityClientForBuildLogs()
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		// Override to return existing build
		openChoreoClient.GetComponentWorkflowFunc = func(ctx context.Context, orgName, projName, componentName, buildName string) (*models.BuildDetailsResponse, error) {
			return &models.BuildDetailsResponse{
				BuildResponse: models.BuildResponse{
					UUID:        "build-uid-456",
					Name:        buildName,
					AgentName:   componentName,
					ProjectName: projName,
				},
			}, nil
		}
		testClients := wiring.TestClients{
			OpenChoreoSvcClient:    openChoreoClient,
			ObservabilitySvcClient: observabilityClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds/%s/build-logs", buildLogsOrgName, buildLogsProjName, buildLogsAgentName, buildLogsBuildName)
		req := httptest.NewRequest(http.MethodGet, url, nil)

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
		require.Equal(t, int32(5), response.TotalCount)
		require.Len(t, response.Logs, 5)

		// Validate first log entry
		log1 := response.Logs[0]
		require.Equal(t, "Starting build process", log1.Log)
		require.Equal(t, "INFO", log1.LogLevel)

		// Validate last log entry
		log5 := response.Logs[4]
		require.Equal(t, "Build completed successfully", log5.Log)
		require.Equal(t, "INFO", log5.LogLevel)

		// Validate service calls
		require.Len(t, observabilityClient.GetBuildLogsCalls(), 1)
		require.Len(t, openChoreoClient.GetAgentComponentCalls(), 1)
		require.Len(t, openChoreoClient.GetComponentWorkflowCalls(), 1)

		// Validate call parameters
		getBuildLogsCall := observabilityClient.GetBuildLogsCalls()[0]
		require.Equal(t, buildLogsBuildName, getBuildLogsCall.BuildName)

		getComponentCall := openChoreoClient.GetAgentComponentCalls()[0]
		require.Equal(t, buildLogsOrgName, getComponentCall.OrgName)
		require.Equal(t, buildLogsProjName, getComponentCall.ProjName)
		require.Equal(t, buildLogsAgentName, getComponentCall.AgentName)

		getWorkflowCall := openChoreoClient.GetComponentWorkflowCalls()[0]
		require.Equal(t, buildLogsOrgName, getWorkflowCall.OrgName)
		require.Equal(t, buildLogsProjName, getWorkflowCall.ProjName)
		require.Equal(t, buildLogsAgentName, getWorkflowCall.ComponentName)
		require.Equal(t, buildLogsBuildName, getWorkflowCall.BuildName)
	})

	getBuildLogsTests := []struct {
		name           string
		authMiddleware jwtassertion.Middleware
		wantStatus     int
		wantErrMsg     string
		url            string
		setupMock      func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock)
	}{
		{
			name:           "return 404 on non-existent organization",
			authMiddleware: authMiddleware,
			wantStatus:     404,
			wantErrMsg:     "Organization not found",
			url:            fmt.Sprintf("/api/v1/orgs/nonexistent-org/projects/%s/agents/%s/builds/%s/build-logs", buildLogsProjName, buildLogsAgentName, buildLogsBuildName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				obsClient := createMockObservabilityClientForBuildLogs()
				openClient := apitestutils.CreateMockOpenChoreoClient()
				// Override to return organization not found - already handled by default mock
				return obsClient, openClient
			},
		},
		{
			name:           "return 404 on non-existent project",
			authMiddleware: authMiddleware,
			wantStatus:     404,
			wantErrMsg:     "Project not found",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/nonexistent-project/agents/%s/builds/%s/build-logs", buildLogsOrgName, buildLogsAgentName, buildLogsBuildName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				obsClient := createMockObservabilityClientForBuildLogs()
				openClient := apitestutils.CreateMockOpenChoreoClient()
				openClient.GetComponentWorkflowFunc = func(ctx context.Context, orgName, projName, componentName, buildName string) (*models.BuildDetailsResponse, error) {
					return &models.BuildDetailsResponse{
						BuildResponse: models.BuildResponse{
							UUID:        "build-uid-456",
							Name:        buildName,
							AgentName:   componentName,
							ProjectName: projName,
						},
					}, nil
				}
				return obsClient, openClient
			},
		},
		{
			name:           "return 404 on non-existent agent",
			authMiddleware: authMiddleware,
			wantStatus:     404,
			wantErrMsg:     "Agent not found",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/nonexistent-agent/builds/%s/build-logs", buildLogsOrgName, buildLogsProjName, buildLogsBuildName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				obsClient := createMockObservabilityClientForBuildLogs()
				openClient := apitestutils.CreateMockOpenChoreoClient()
				return obsClient, openClient
			},
		},
		{
			name:           "return 404 on non-existent build",
			authMiddleware: authMiddleware,
			wantStatus:     404,
			wantErrMsg:     "Build not found",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds/nonexistent-build/build-logs", buildLogsOrgName, buildLogsProjName, buildLogsAgentName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				obsClient := createMockObservabilityClientForBuildLogs()
				openClient := apitestutils.CreateMockOpenChoreoClient()
				openClient.GetComponentWorkflowFunc = func(ctx context.Context, orgName, projName, componentName, buildName string) (*models.BuildDetailsResponse, error) {
					return nil, utils.ErrBuildNotFound
				}
				return obsClient, openClient
			},
		},
		{
			name:           "return 500 on observability service error",
			authMiddleware: authMiddleware,
			wantStatus:     500,
			wantErrMsg:     "Failed to get build logs",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds/%s/build-logs", buildLogsOrgName, buildLogsProjName, buildLogsAgentName, buildLogsBuildName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				obsClient := createMockObservabilityClientForBuildLogs()
				openClient := apitestutils.CreateMockOpenChoreoClient()
				openClient.GetComponentWorkflowFunc = func(ctx context.Context, orgName, projName, componentName, buildName string) (*models.BuildDetailsResponse, error) {
					return &models.BuildDetailsResponse{
						BuildResponse: models.BuildResponse{
							UUID:        "build-uid-456",
							Name:        buildName,
							AgentName:   componentName,
							ProjectName: projName,
						},
					}, nil
				}
				obsClient.GetBuildLogsFunc = func(ctx context.Context, buildName string) (*models.LogsResponse, error) {
					return nil, fmt.Errorf("observability service error")
				}
				return obsClient, openClient
			},
		},
		{
			name: "return 401 on missing authentication",
			authMiddleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					utils.WriteErrorResponse(w, http.StatusUnauthorized, "missing header: Authorization")
				})
			},
			wantStatus: 401,
			wantErrMsg: "missing header: Authorization",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds/%s/build-logs", buildLogsOrgName, buildLogsProjName, buildLogsAgentName, buildLogsBuildName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				return createMockObservabilityClientForBuildLogs(), apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 500 on workflow fetch error",
			authMiddleware: authMiddleware,
			wantStatus:     500,
			wantErrMsg:     "Failed to get build logs",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds/%s/build-logs", buildLogsOrgName, buildLogsProjName, buildLogsAgentName, buildLogsBuildName),
			setupMock: func() (*clientmocks.ObservabilitySvcClientMock, *clientmocks.OpenChoreoSvcClientMock) {
				obsClient := createMockObservabilityClientForBuildLogs()
				openClient := apitestutils.CreateMockOpenChoreoClient()
				openClient.GetComponentWorkflowFunc = func(ctx context.Context, orgName, projName, componentName, buildName string) (*models.BuildDetailsResponse, error) {
					return nil, fmt.Errorf("workflow service error")
				}
				return obsClient, openClient
			},
		},
	}

	for _, tt := range getBuildLogsTests {
		t.Run(tt.name, func(t *testing.T) {
			obsClient, openClient := tt.setupMock()
			testClients := wiring.TestClients{
				OpenChoreoSvcClient:    openClient,
				ObservabilitySvcClient: obsClient,
			}

			app := apitestutils.MakeAppClientWithDeps(t, testClients, tt.authMiddleware)

			// Send the request
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)

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
