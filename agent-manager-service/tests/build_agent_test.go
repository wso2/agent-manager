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

	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/clients/clientmocks"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/models"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/utils"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/wiring"
)

var (
	buildTestOrgId     = uuid.New()
	buildTestUserIdpId = uuid.New()
	buildTestProjId    = uuid.New()
	buildTestOrgName   = fmt.Sprintf("build-test-org-%s", uuid.New().String()[:5])
	buildTestProjName  = fmt.Sprintf("build-test-project-%s", uuid.New().String()[:5])
	buildTestAgentName = fmt.Sprintf("build-test-agent-%s", uuid.New().String()[:5])
)

func createMockOpenChoreoClientForBuild() *clientmocks.OpenChoreoSvcClientMock {
	return &clientmocks.OpenChoreoSvcClientMock{
		GetProjectFunc: func(ctx context.Context, projectName string, orgName string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{
				Name:        projectName,
				DisplayName: projectName,
				OrgName:     orgName,
				CreatedAt:   time.Now(),
			}, nil
		},
		IsAgentComponentExistsFunc: func(ctx context.Context, orgName string, projName string, agentName string) (bool, error) {
			return true, nil
		},
		TriggerBuildFunc: func(ctx context.Context, orgName string, projName string, agentName string, commitId string) (*models.BuildResponse, error) {
			return &models.BuildResponse{
				UUID:        uuid.New().String(),
				Name:        fmt.Sprintf("%s-build-%s", agentName, uuid.New().String()[:8]),
				AgentName:   agentName,
				ProjectName: projName,
				CommitID:    commitId,
				Status:      "BuildInitiated",
				StartedAt:   time.Now(),
				Branch:      "main",
			}, nil
		},
	}
}

func TestBuildAgent(t *testing.T) {
	setUpBuildTest(t)
	authMiddleware := jwtassertion.NewMockMiddleware(t, buildTestOrgId, buildTestUserIdpId)

	t.Run("Triggering build with commitId should return 202", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForBuild()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the request with commitId query parameter
		commitId := "328efd0dc93c4a184be3967a6e7307c982836ea7"
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds?commitId=%s",
			buildTestOrgName, buildTestProjName, buildTestAgentName, commitId)
		req := httptest.NewRequest(http.MethodPost, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusAccepted, rr.Code)

		// Read and validate response body
		b, err := io.ReadAll(rr.Body)
		require.NoError(t, err)
		t.Logf("response body: %s", string(b))

		var build models.BuildResponse
		require.NoError(t, json.Unmarshal(b, &build))

		// Validate response fields
		require.Equal(t, buildTestAgentName, build.AgentName)
		require.Equal(t, buildTestProjName, build.ProjectName)
		require.Equal(t, commitId, build.CommitID)
		require.Equal(t, "BuildInitiated", build.Status)
		require.NotEmpty(t, build.Name)
		require.NotEmpty(t, build.UUID)
		require.NotZero(t, build.StartedAt)

		// Validate service calls
		require.Len(t, openChoreoClient.TriggerBuildCalls(), 1)

		// Validate call parameters
		triggerBuildCall := openChoreoClient.TriggerBuildCalls()[0]
		require.Equal(t, buildTestOrgName, triggerBuildCall.OrgName)
		require.Equal(t, buildTestProjName, triggerBuildCall.ProjName)
		require.Equal(t, buildTestAgentName, triggerBuildCall.AgentName)
		require.Equal(t, commitId, triggerBuildCall.CommitId)
	})

	t.Run("Triggering build without commitId should return 202", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForBuild()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the request without commitId query parameter
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds",
			buildTestOrgName, buildTestProjName, buildTestAgentName)
		req := httptest.NewRequest(http.MethodPost, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusAccepted, rr.Code)

		// Read and validate response body
		b, err := io.ReadAll(rr.Body)
		require.NoError(t, err)
		t.Logf("response body: %s", string(b))

		var build models.BuildResponse
		require.NoError(t, json.Unmarshal(b, &build))

		// Validate response fields
		require.Equal(t, buildTestAgentName, build.AgentName)
		require.Equal(t, buildTestProjName, build.ProjectName)
		require.Equal(t, "BuildInitiated", build.Status)
		require.NotEmpty(t, build.Name)
		require.NotEmpty(t, build.UUID)
		require.NotZero(t, build.StartedAt)

		// Validate service calls
		require.Len(t, openChoreoClient.TriggerBuildCalls(), 1)

		// Validate call parameters - commitId is empty when not provided
		triggerBuildCall := openChoreoClient.TriggerBuildCalls()[0]
		require.Equal(t, buildTestOrgName, triggerBuildCall.OrgName)
		require.Equal(t, buildTestProjName, triggerBuildCall.ProjName)
		require.Equal(t, buildTestAgentName, triggerBuildCall.AgentName)
		require.Equal(t, "", triggerBuildCall.CommitId)
	})

	validationTests := []struct {
		name           string
		authMiddleware jwtassertion.Middleware
		orgName        string
		projName       string
		agentName      string
		commitId       string
		url            string
		wantStatus     int
		wantErrMsg     string
		setupMock      func() *clientmocks.OpenChoreoSvcClientMock
	}{
		{
			name:           "return 404 on organization not found",
			authMiddleware: authMiddleware,
			orgName:        "nonexistent-org",
			projName:       buildTestProjName,
			agentName:      buildTestAgentName,
			commitId:       "abc123",
			url:            fmt.Sprintf("/api/v1/orgs/nonexistent-org/projects/%s/agents/%s/builds?commitId=abc123", buildTestProjName, buildTestAgentName),
			wantStatus:     404,
			wantErrMsg:     "Organization not found",
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClientForBuild()
				mock.GetProjectFunc = func(ctx context.Context, projectName string, orgName string) (*models.ProjectResponse, error) {
					return nil, utils.ErrOrganizationNotFound
				}
				return mock
			},
		},
		{
			name:           "return 404 on project not found",
			authMiddleware: authMiddleware,
			orgName:        buildTestOrgName,
			projName:       "nonexistent-project",
			agentName:      buildTestAgentName,
			commitId:       "abc123",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/nonexistent-project/agents/%s/builds?commitId=abc123", buildTestOrgName, buildTestAgentName),
			wantStatus:     404,
			wantErrMsg:     "Project not found",
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClientForBuild()
				mock.GetProjectFunc = func(ctx context.Context, projectName string, orgName string) (*models.ProjectResponse, error) {
					return nil, utils.ErrProjectNotFound
				}
				return mock
			},
		},
		{
			name:           "return 404 on agent not found",
			authMiddleware: authMiddleware,
			orgName:        buildTestOrgName,
			projName:       buildTestProjName,
			agentName:      "nonexistent-agent",
			commitId:       "abc123",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/nonexistent-agent/builds?commitId=abc123", buildTestOrgName, buildTestProjName),
			wantStatus:     404,
			wantErrMsg:     "Agent not found",
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClientForBuild()
				mock.IsAgentComponentExistsFunc = func(ctx context.Context, orgName string, projName string, agentName string) (bool, error) {
					return false, nil
				}
				return mock
			},
		},
		{
			name:           "return 500 on service error",
			authMiddleware: authMiddleware,
			orgName:        buildTestOrgName,
			projName:       buildTestProjName,
			agentName:      buildTestAgentName, // Use existing agent to reach the TriggerBuild call
			commitId:       "abc123",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds?commitId=abc123", buildTestOrgName, buildTestProjName, buildTestAgentName),
			wantStatus:     500,
			wantErrMsg:     "Failed to build agent",
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClientForBuild()
				mock.TriggerBuildFunc = func(ctx context.Context, orgName string, projName string, agentName string, commitId string) (*models.BuildResponse, error) {
					return nil, fmt.Errorf("internal service error")
				}
				return mock
			},
		},
		{
			name: "return 401 on missing authentication",
			authMiddleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					utils.WriteErrorResponse(w, http.StatusUnauthorized, "missing header: Authorization")
				})
			},
			orgName:    buildTestOrgName,
			projName:   buildTestProjName,
			agentName:  buildTestAgentName,
			commitId:   "abc123",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds?commitId=abc123", buildTestOrgName, buildTestProjName, buildTestAgentName),
			wantStatus: 401,
			wantErrMsg: "missing header: Authorization",
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForBuild()
			},
		},
	}

	for _, tt := range validationTests {
		t.Run(tt.name, func(t *testing.T) {
			openChoreoClient := tt.setupMock()
			testClients := wiring.TestClients{
				OpenChoreoSvcClient: openChoreoClient,
			}

			app := apitestutils.MakeAppClientWithDeps(t, testClients, tt.authMiddleware)

			req := httptest.NewRequest(http.MethodPost, tt.url, nil)
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

func setUpBuildTest(t *testing.T) {
	_ = apitestutils.CreateOrganization(t, buildTestOrgId, buildTestUserIdpId, buildTestOrgName)
	_ = apitestutils.CreateProject(t, buildTestProjId, buildTestOrgId, buildTestProjName)
	_ = apitestutils.CreateAgent(t, uuid.New(), buildTestOrgId, buildTestProjId, buildTestAgentName, string(utils.InternalAgent))
}
