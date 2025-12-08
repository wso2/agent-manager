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
	testDeleteOrgId       = uuid.New()
	testDeleteProjId      = uuid.New()
	testDeleteUserIdpId   = uuid.New()
	testDeleteOrgName     = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])
	testDeleteProjName    = fmt.Sprintf("test-project-%s", uuid.New().String()[:5])
	testDeleteAgentName   = fmt.Sprintf("test-agent-%s", uuid.New().String()[:5])
	testExternalAgentName = fmt.Sprintf("test-external-%s", uuid.New().String()[:5])
	testFailingAgentName  = fmt.Sprintf("failing-agent-%s", uuid.New().String()[:5])
)

func createMockOpenChoreoClientForDelete() *clientmocks.OpenChoreoSvcClientMock {
	return &clientmocks.OpenChoreoSvcClientMock{
		DeleteAgentComponentFunc: func(ctx context.Context, orgName string, projName string, agentName string) error {
			return nil
		},
	}
}

func TestDeleteAgent(t *testing.T) {
	setUpDeleteTest(t)
	authMiddleware := jwtassertion.NewMockMiddleware(t, testDeleteOrgId, testDeleteUserIdpId)

	t.Run("Deleting an internal agent should return 204", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForDelete()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the delete request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", testDeleteOrgName, testDeleteProjName, testDeleteAgentName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusNoContent, rr.Code)

		// Validate service calls
		require.Len(t, openChoreoClient.DeleteAgentComponentCalls(), 1)

		// Validate call parameters
		deleteCall := openChoreoClient.DeleteAgentComponentCalls()[0]
		require.Equal(t, testDeleteOrgName, deleteCall.OrgName)
		require.Equal(t, testDeleteProjName, deleteCall.ProjName)
		require.Equal(t, testDeleteAgentName, deleteCall.AgentName)
	})

	t.Run("Deleting an external agent should return 204", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForDelete()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Send the delete request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", testDeleteOrgName, testDeleteProjName, testExternalAgentName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusNoContent, rr.Code)

		// Validate that DeleteAgentComponent was NOT called for external agents
		require.Len(t, openChoreoClient.DeleteAgentComponentCalls(), 0)
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
			url:            fmt.Sprintf("/api/v1/orgs/nonexistent-org/projects/%s/agents/%s", testDeleteProjName, testDeleteAgentName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForDelete()
			},
			setupData: func(t *testing.T) {
				// No data setup needed
			},
		},
		{
			name:           "return 404 on project not found",
			authMiddleware: authMiddleware,
			wantStatus:     404,
			wantErrMsg:     "Project not found",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/nonexistent-project/agents/%s", testDeleteOrgName, testDeleteAgentName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForDelete()
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
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", testDeleteOrgName, testDeleteProjName, testDeleteAgentName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForDelete()
			},
			setupData: func(t *testing.T) {
				// No data setup needed
			},
		},
		{
			name:           "return 500 on OpenChoreo delete failure for internal agent",
			authMiddleware: authMiddleware,
			wantStatus:     500,
			wantErrMsg:     "Failed to delete agent",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", testDeleteOrgName, testDeleteProjName, testFailingAgentName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClientForDelete()
				mock.DeleteAgentComponentFunc = func(ctx context.Context, orgName string, projName string, agentName string) error {
					return fmt.Errorf("OpenChoreo service error")
				}
				return mock
			},
			setupData: func(t *testing.T) {
				// Create an internal agent that will fail to delete from OpenChoreo
				_ = apitestutils.CreateAgent(t, uuid.New(), testDeleteOrgId, testDeleteProjId, testFailingAgentName, string(utils.InternalAgent))
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

func TestDeleteAgentIdempotency(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t, testDeleteOrgId, testDeleteUserIdpId)

	t.Run("Multiple deletes of same agent should be handled gracefully", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForDelete()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create an agent to delete
		agentName := fmt.Sprintf("new-agent-%s", uuid.New().String()[:7])
		_ = apitestutils.CreateAgent(t, uuid.New(), testDeleteOrgId, testDeleteProjId, agentName, string(utils.InternalAgent))

		// Make multiple delete requests
		numRequests := 2
		responses := make([]*httptest.ResponseRecorder, numRequests)

		for i := 0; i < numRequests; i++ {
			responses[i] = httptest.NewRecorder()
			url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", testDeleteOrgName, testDeleteProjName, agentName)
			req := httptest.NewRequest(http.MethodDelete, url, nil)

			// Execute request
			app.ServeHTTP(responses[i], req)
		}

		// All responses should be successful (204 No Content) due to idempotent nature
		for i, rr := range responses {
			require.Equal(t, http.StatusNoContent, rr.Code, "Request %d should succeed", i)
		}

		// OpenChoreo delete should be called at least once (but may be called multiple times due to race conditions)
		require.GreaterOrEqual(t, len(openChoreoClient.DeleteAgentComponentCalls()), 1)
	})
}

func setUpDeleteTest(t *testing.T) {
	_ = apitestutils.CreateOrganization(t, testDeleteOrgId, testDeleteUserIdpId, testDeleteOrgName)
	_ = apitestutils.CreateProject(t, testDeleteProjId, testDeleteOrgId, testDeleteProjName)
	_ = apitestutils.CreateAgent(t, uuid.New(), testDeleteOrgId, testDeleteProjId, testDeleteAgentName, string(utils.InternalAgent))
	_ = apitestutils.CreateAgent(t, uuid.New(), testDeleteOrgId, testDeleteProjId, testExternalAgentName, string(utils.ExternalAgent))
}
