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
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/clientmocks"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/wiring"
)

var (
	testDeleteProjectOrgId     = uuid.New()
	testDeleteProjectProjId    = uuid.New()
	testDeleteProjectUserIdpId = uuid.New()
	testDeleteProjectOrgName   = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])
	testDeleteProjectProjName  = fmt.Sprintf("test-project-%s", uuid.New().String()[:5])
	testProjectWithAgents      = fmt.Sprintf("project-with-agents-%s", uuid.New().String()[:5])
	testFailingProjectName     = fmt.Sprintf("failing-project-%s", uuid.New().String()[:5])
	projectWithAgentsId        = uuid.New()
)

func createMockOpenChoreoClientForProjectDelete() *clientmocks.OpenChoreoSvcClientMock {
	return &clientmocks.OpenChoreoSvcClientMock{
		DeleteProjectFunc: func(ctx context.Context, orgName string, projectName string) error {
			return nil
		},
	}
}

func TestDeleteProject(t *testing.T) {
	setUpDeleteProjectTest(t)
	authMiddleware := jwtassertion.NewMockMiddleware(t, testDeleteProjectOrgId, testDeleteProjectUserIdpId)

	t.Run("Deleting an empty project should return 204", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForProjectDelete()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the delete request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", testDeleteProjectOrgName, testDeleteProjectProjName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusNoContent, rr.Code)

		// Validate service calls
		require.Len(t, openChoreoClient.DeleteProjectCalls(), 1)

		// Validate call parameters
		deleteCall := openChoreoClient.DeleteProjectCalls()[0]
		require.Equal(t, testDeleteProjectOrgName, deleteCall.OrgName)
		require.Equal(t, testDeleteProjectProjName, deleteCall.ProjectName)
	})

	t.Run("Deleting a project with agents should return 409", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForProjectDelete()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the delete request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", testDeleteProjectOrgName, testProjectWithAgents)
		req := httptest.NewRequest(http.MethodDelete, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusConflict, rr.Code)

		// Validate that OpenChoreo delete was NOT called
		require.Len(t, openChoreoClient.DeleteProjectCalls(), 0)
	})

	t.Run("Deleting non-existent project should return 204 (idempotent)", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForProjectDelete()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the delete request for non-existent project
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/non-existent-project", testDeleteProjectOrgName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response - DELETE should be idempotent
		require.Equal(t, http.StatusNoContent, rr.Code)

		// Validate that OpenChoreo delete was NOT called for non-existent project
		require.Len(t, openChoreoClient.DeleteProjectCalls(), 0)
	})

	validationTests := []struct {
		name           string
		authMiddleware jwtassertion.Middleware
		wantStatus     int
		wantErrMsg     string
		url            string
		setupMock      func() *clientmocks.OpenChoreoSvcClientMock
		setupData      func(t *testing.T) // Function to set up test data if needed
	}{
		{
			name:           "return 404 on organization not found",
			authMiddleware: authMiddleware,
			wantStatus:     404,
			wantErrMsg:     "Organization not found",
			url:            fmt.Sprintf("/api/v1/orgs/nonexistent-org/projects/%s", testDeleteProjectProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForProjectDelete()
			},
			setupData: func(t *testing.T) {
				// No data setup needed
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
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s", testDeleteProjectOrgName, testDeleteProjectProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForProjectDelete()
			},
			setupData: func(t *testing.T) {
				// No data setup needed
			},
		},
		{
			name:           "return 500 on OpenChoreo delete failure",
			authMiddleware: authMiddleware,
			wantStatus:     500,
			wantErrMsg:     "Failed to delete project",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/%s", testDeleteProjectOrgName, testFailingProjectName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClientForProjectDelete()
				mock.DeleteProjectFunc = func(ctx context.Context, orgName string, projectName string) error {
					return fmt.Errorf("OpenChoreo service error")
				}
				return mock
			},
			setupData: func(t *testing.T) {
				// Create a project that will fail to delete from OpenChoreo
				failingProjectId := uuid.New()
				_ = apitestutils.CreateProject(t, failingProjectId, testDeleteProjectOrgId, testFailingProjectName)
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

			// Setup test data if needed
			tt.setupData(t)

			// Send the delete request
			req := httptest.NewRequest(http.MethodDelete, tt.url, nil)

			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			// Assert response
			require.Equal(t, tt.wantStatus, rr.Code)

			// Check error message for error responses
			if tt.wantStatus >= 400 {
				body := rr.Body.String()
				require.Contains(t, body, tt.wantErrMsg)
			}
		})
	}
}

func TestDeleteProjectIdempotency(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t, testDeleteProjectOrgId, testDeleteProjectUserIdpId)

	t.Run("Multiple deletes of same project should be handled gracefully", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForProjectDelete()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create a project to delete
		projectName := fmt.Sprintf("new-project-%s", uuid.New().String()[:7])
		projectId := uuid.New()
		_ = apitestutils.CreateProject(t, projectId, testDeleteProjectOrgId, projectName)

		// Make multiple delete requests
		numRequests := 2
		responses := make([]*httptest.ResponseRecorder, numRequests)

		for i := 0; i < numRequests; i++ {
			responses[i] = httptest.NewRecorder()
			url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", testDeleteProjectOrgName, projectName)
			req := httptest.NewRequest(http.MethodDelete, url, nil)

			// Execute request
			app.ServeHTTP(responses[i], req)
		}

		// All responses should be successful (204 No Content) due to idempotent nature
		for i, rr := range responses {
			require.Equal(t, http.StatusNoContent, rr.Code, "Request %d should succeed", i)
		}

		// OpenChoreo delete should be called at least once (but may be called multiple times due to race conditions)
		require.GreaterOrEqual(t, len(openChoreoClient.DeleteProjectCalls()), 1)
	})
}

func setUpDeleteProjectTest(t *testing.T) {
	_ = apitestutils.CreateOrganization(t, testDeleteProjectOrgId, testDeleteProjectUserIdpId, testDeleteProjectOrgName)
	_ = apitestutils.CreateProject(t, testDeleteProjectProjId, testDeleteProjectOrgId, testDeleteProjectProjName)
	_ = apitestutils.CreateProject(t, projectWithAgentsId, testDeleteProjectOrgId, testProjectWithAgents)
	_ = apitestutils.CreateAgent(t, uuid.New(), testDeleteProjectOrgId, projectWithAgentsId, "test-agent-1", string(utils.InternalAgent))
}
