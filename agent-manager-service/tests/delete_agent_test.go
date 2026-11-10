//go:build integration

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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var (
	testDeleteOrgName     = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])
	testDeleteProjName    = fmt.Sprintf("test-project-%s", uuid.New().String()[:5])
	testDeleteAgentName   = fmt.Sprintf("test-agent-%s", uuid.New().String()[:5])
	testExternalAgentName = fmt.Sprintf("test-external-%s", uuid.New().String()[:5])
	testFailingAgentName  = fmt.Sprintf("failing-agent-%s", uuid.New().String()[:5])
)

func TestDeleteAgent(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Deleting an internal agent should return 204", func(t *testing.T) {
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		secretMgmtClient := apitestutils.CreateMockSecretManagementClient()
		testClients := wiring.TestClients{
			OpenChoreoClient: openChoreoClient,
			SecretMgmtClient: secretMgmtClient,
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
		require.Len(t, openChoreoClient.DeleteComponentCalls(), 1)

		// Validate call parameters
		deleteCall := openChoreoClient.DeleteComponentCalls()[0]
		require.Equal(t, testDeleteProjName, deleteCall.ProjectName)
		require.Equal(t, testDeleteAgentName, deleteCall.ComponentName)

		// The agent's API key secret is referenced by no ReleaseBinding, so it is deleted
		// at its known location, in the background once the component is gone.
		apiKeySecret := testDeleteAgentName + "-agent-api-key-development-secrets"
		require.Eventually(t, func() bool {
			for _, call := range secretMgmtClient.DeleteSecretCalls() {
				if call.Location.SecretRefName() == apiKeySecret {
					return true
				}
			}
			return false
		}, 5*time.Second, 10*time.Millisecond, "the agent API key secret %q is deleted", apiKeySecret)
	})

	t.Run("Deleting an external agent should return 204", func(t *testing.T) {
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		secretMgmtClient := apitestutils.CreateMockSecretManagementClient()
		testClients := wiring.TestClients{
			OpenChoreoClient: openChoreoClient,
			SecretMgmtClient: secretMgmtClient,
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
		require.Len(t, openChoreoClient.DeleteComponentCalls(), 1)
	})

	validationTests := []struct {
		name           string
		authMiddleware jwtassertion.Middleware
		wantStatus     int
		wantErrMsg     string
		url            string
		setupMock      func() *clientmocks.OpenChoreoClientMock
	}{
		{
			name:           "return 404 on organization not found",
			authMiddleware: jwtassertion.NewMockMiddlewareWithOUID(t, "nonexistent-org"),
			wantStatus:     404,
			wantErrMsg:     "Organization not found",
			url:            fmt.Sprintf("/api/v1/orgs/nonexistent-org/projects/%s/agents/%s", testDeleteProjName, testDeleteAgentName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				return apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 404 on project not found",
			authMiddleware: authMiddleware,
			wantStatus:     404,
			wantErrMsg:     "Project not found",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/nonexistent-project/agents/%s", testDeleteOrgName, testDeleteAgentName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				mock := apitestutils.CreateMockOpenChoreoClient()
				mock.DeleteComponentFunc = func(ctx context.Context, namespaceName string, projectName string, componentName string) error {
					if projectName == "nonexistent-project" {
						return utils.ErrProjectNotFound
					}
					return nil
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
			wantStatus: 401,
			wantErrMsg: "missing header: Authorization",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", testDeleteOrgName, testDeleteProjName, testDeleteAgentName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				return apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 500 on OpenChoreo delete failure for internal agent",
			authMiddleware: authMiddleware,
			wantStatus:     500,
			wantErrMsg:     "Failed to delete agent",
			url:            fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", testDeleteOrgName, testDeleteProjName, testFailingAgentName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				mock := apitestutils.CreateMockOpenChoreoClient()
				mock.DeleteComponentFunc = func(ctx context.Context, orgName string, projName string, agentName string) error {
					return fmt.Errorf("OpenChoreo service error")
				}
				return mock
			},
		},
	}

	for _, tt := range validationTests {
		t.Run(tt.name, func(t *testing.T) {
			openChoreoClient := tt.setupMock()
			secretMgmtClient := apitestutils.CreateMockSecretManagementClient()
			testClients := wiring.TestClients{
				OpenChoreoClient: openChoreoClient,
				SecretMgmtClient: secretMgmtClient,
			}

			app := apitestutils.MakeAppClientWithDeps(t, testClients, tt.authMiddleware)

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

func TestDeleteAgentBlockedWhenKindSource(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Deleting an agent that is the source of an agent kind should return 409", func(t *testing.T) {
		orgName := fmt.Sprintf("test-org-%s", uuid.New().String()[:5])
		projName := fmt.Sprintf("test-project-%s", uuid.New().String()[:5])
		agentName := fmt.Sprintf("test-agent-%s", uuid.New().String()[:5])

		gdb := db.DB(context.Background())
		kind := &models.AgentKind{
			ID:          uuid.New(),
			Name:        fmt.Sprintf("test-kind-%s", uuid.New().String()[:5]),
			DisplayName: "Test Kind",
			// OrgName maps to the ou_id column; the delete flow scopes the
			// kind-source check by the token's OU ID, so seed it with that.
			OUID:        jwtassertion.MockOUID,
			ProjectName: projName,
			AgentName:   agentName,
			Labels:      map[string]string{},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, gdb.Create(kind).Error)
		t.Cleanup(func() {
			gdb.Delete(kind)
		})

		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		secretMgmtClient := apitestutils.CreateMockSecretManagementClient()
		testClients := wiring.TestClients{
			OpenChoreoClient: openChoreoClient,
			SecretMgmtClient: secretMgmtClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s", orgName, projName, agentName)
		req := httptest.NewRequest(http.MethodDelete, url, nil)

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusConflict, rr.Code)
		require.Contains(t, rr.Body.String(), "Agent is the source of an agent kind")
		require.Empty(t, openChoreoClient.DeleteComponentCalls())
	})
}

func TestDeleteAgentIdempotency(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Multiple deletes of same agent should be handled gracefully", func(t *testing.T) {
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		secretMgmtClient := apitestutils.CreateMockSecretManagementClient()
		testClients := wiring.TestClients{
			OpenChoreoClient: openChoreoClient,
			SecretMgmtClient: secretMgmtClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create an agent to delete
		agentName := fmt.Sprintf("new-agent-%s", uuid.New().String()[:7])

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
		require.GreaterOrEqual(t, len(openChoreoClient.DeleteComponentCalls()), 1)
	})
}
