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

	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/clients/clientmocks"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/clients/openchoreosvc"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/models"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/spec"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/utils"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/wiring"
)

var (
	deployTestOrgId     = uuid.New()
	deployTestUserIdpId = uuid.New()
	deployTestProjId    = uuid.New()
	deployTestOrgName   = fmt.Sprintf("deploy-test-org-%s", uuid.New().String()[:5])
	deployTestProjName  = fmt.Sprintf("deploy-test-project-%s", uuid.New().String()[:5])
	deployTestAgentName = fmt.Sprintf("deploy-test-agent-%s", uuid.New().String()[:5])
)

func createMockOpenChoreoClientForDeploy() *clientmocks.OpenChoreoSvcClientMock {
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
		GetAgentComponentFunc: func(ctx context.Context, orgName string, projName string, agentName string) (*openchoreosvc.AgentComponent, error) {
			return &openchoreosvc.AgentComponent{
				Name:        agentName,
				ProjectName: projName,
				CreatedAt:   time.Now(),
			}, nil
		},
		DeployAgentComponentFunc: func(ctx context.Context, orgName string, projName string, componentName string, language string, req *spec.DeployAgentRequest) error {
			return nil
		},
	}
}

func TestDeployAgent(t *testing.T) {
	setUpDeployTest(t)
	authMiddleware := jwtassertion.NewMockMiddleware(t, deployTestOrgId, deployTestUserIdpId)

	t.Run("Deploying agent with valid imageId should return 202", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForDeploy()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create the request body
		reqBody := new(bytes.Buffer)
		err := json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"imageId": "registry.example.com/myapp:v1.0.0",
		})
		require.NoError(t, err)

		// Send the request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
			deployTestOrgName, deployTestProjName, deployTestAgentName)
		req := httptest.NewRequest(http.MethodPost, url, reqBody)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusAccepted, rr.Code)

		// Read and validate response body
		b, err := io.ReadAll(rr.Body)
		require.NoError(t, err)
		t.Logf("response body: %s", string(b))

		var response spec.DeploymentResponse
		require.NoError(t, json.Unmarshal(b, &response))

		// Validate response fields
		require.Equal(t, deployTestAgentName, response.AgentName)
		require.Equal(t, deployTestProjName, response.ProjectName)
		require.Equal(t, "registry.example.com/myapp:v1.0.0", response.ImageId)
		require.Equal(t, "Development", response.Environment)

		// Validate service calls
		require.Len(t, openChoreoClient.DeployAgentComponentCalls(), 1)

		// Validate call parameters
		deployCall := openChoreoClient.DeployAgentComponentCalls()[0]
		require.Equal(t, deployTestOrgName, deployCall.OrgName)
		require.Equal(t, deployTestProjName, deployCall.ProjName)
		require.Equal(t, deployTestAgentName, deployCall.ComponentName)
		require.Equal(t, "registry.example.com/myapp:v1.0.0", deployCall.Req.ImageId)
		require.Empty(t, deployCall.Req.Env) // No env vars provided
	})

	t.Run("Deploying agent with imageId and environment variables should return 202", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClientForDeploy()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create the request body with environment variables
		reqBody := new(bytes.Buffer)
		err := json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"imageId": "registry.example.com/myapp:v1.2.0",
			"env": []map[string]interface{}{
				{
					"key":   "DATABASE_URL",
					"value": "postgresql://localhost:5432/mydb",
				},
				{
					"key":   "API_KEY",
					"value": "secret-api-key",
				},
				{
					"key":   "LOG_LEVEL",
					"value": "INFO",
				},
			},
		})
		require.NoError(t, err)

		// Send the request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
			deployTestOrgName, deployTestProjName, deployTestAgentName)
		req := httptest.NewRequest(http.MethodPost, url, reqBody)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusAccepted, rr.Code)

		// Read and validate response body
		b, err := io.ReadAll(rr.Body)
		require.NoError(t, err)
		t.Logf("response body: %s", string(b))

		var response spec.DeploymentResponse
		require.NoError(t, json.Unmarshal(b, &response))

		// Validate response fields
		require.Equal(t, deployTestAgentName, response.AgentName)
		require.Equal(t, deployTestProjName, response.ProjectName)
		require.Equal(t, "registry.example.com/myapp:v1.2.0", response.ImageId)
		require.Equal(t, "Development", response.Environment)

		// Validate service calls
		require.Len(t, openChoreoClient.DeployAgentComponentCalls(), 1)

		// Validate call parameters
		deployCall := openChoreoClient.DeployAgentComponentCalls()[0]
		require.Equal(t, deployTestOrgName, deployCall.OrgName)
		require.Equal(t, deployTestProjName, deployCall.ProjName)
		require.Equal(t, deployTestAgentName, deployCall.ComponentName)
		require.Equal(t, "registry.example.com/myapp:v1.2.0", deployCall.Req.ImageId)

		// Validate environment variables
		require.Len(t, deployCall.Req.Env, 3)
		require.Equal(t, "DATABASE_URL", deployCall.Req.Env[0].Key)
		require.Equal(t, "postgresql://localhost:5432/mydb", deployCall.Req.Env[0].Value)
		require.Equal(t, "API_KEY", deployCall.Req.Env[1].Key)
		require.Equal(t, "secret-api-key", deployCall.Req.Env[1].Value)
		require.Equal(t, "LOG_LEVEL", deployCall.Req.Env[2].Key)
		require.Equal(t, "INFO", deployCall.Req.Env[2].Value)
	})

	validationTests := []struct {
		name           string
		authMiddleware jwtassertion.Middleware
		orgName        string
		projName       string
		agentName      string
		payload        map[string]interface{}
		wantStatus     int
		wantErrMsg     string
		setupMock      func() *clientmocks.OpenChoreoSvcClientMock
	}{
		{
			name:           "return 400 on missing imageId",
			authMiddleware: authMiddleware,
			orgName:        deployTestOrgName,
			projName:       deployTestProjName,
			agentName:      deployTestAgentName,
			payload: map[string]interface{}{
				"env": []map[string]interface{}{
					{
						"key":   "TEST_VAR",
						"value": "test-value",
					},
				},
				// Missing imageId
			},
			wantStatus: 400,
			wantErrMsg: "Invalid request body",
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForDeploy()
			},
		},
		{
			name:           "return 400 on empty imageId",
			authMiddleware: authMiddleware,
			orgName:        deployTestOrgName,
			projName:       deployTestProjName,
			agentName:      deployTestAgentName,
			payload: map[string]interface{}{
				"imageId": "", // Empty imageId
			},
			wantStatus: 400,
			wantErrMsg: "Invalid request body",
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForDeploy()
			},
		},
		{
			name:           "return 404 on organization not found",
			authMiddleware: authMiddleware,
			orgName:        deployTestOrgName,
			projName:       deployTestProjName,
			agentName:      deployTestAgentName,
			payload: map[string]interface{}{
				"imageId": "registry.example.com/myapp:v1.0.0",
			},
			wantStatus: 404,
			wantErrMsg: "Organization not found",
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClientForDeploy()
				mock.DeployAgentComponentFunc = func(ctx context.Context, orgName string, projName string, componentName string, language string, req *spec.DeployAgentRequest) error {
					return utils.ErrOrganizationNotFound
				}
				return mock
			},
		},
		{
			name:           "return 404 on project not found",
			authMiddleware: authMiddleware,
			orgName:        deployTestOrgName,
			projName:       deployTestProjName,
			agentName:      deployTestAgentName,
			payload: map[string]interface{}{
				"imageId": "registry.example.com/myapp:v1.0.0",
			},
			wantStatus: 404,
			wantErrMsg: "Project not found",
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClientForDeploy()
				mock.DeployAgentComponentFunc = func(ctx context.Context, orgName string, projName string, componentName string, language string, req *spec.DeployAgentRequest) error {
					return utils.ErrProjectNotFound
				}
				return mock
			},
		},
		{
			name:           "return 404 on agent not found",
			authMiddleware: authMiddleware,
			orgName:        deployTestOrgName,
			projName:       deployTestProjName,
			agentName:      deployTestAgentName,
			payload: map[string]interface{}{
				"imageId": "registry.example.com/myapp:v1.0.0",
			},
			wantStatus: 404,
			wantErrMsg: "Agent not found",
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClientForDeploy()
				mock.DeployAgentComponentFunc = func(ctx context.Context, orgName string, projName string, componentName string, language string, req *spec.DeployAgentRequest) error {
					return utils.ErrAgentNotFound
				}
				return mock
			},
		},
		{
			name:           "return 500 on service error",
			authMiddleware: authMiddleware,
			orgName:        deployTestOrgName,
			projName:       deployTestProjName,
			agentName:      deployTestAgentName,
			payload: map[string]interface{}{
				"imageId": "registry.example.com/myapp:v1.0.0",
			},
			wantStatus: 500,
			wantErrMsg: "Failed to deploy agent",
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClientForDeploy()
				mock.DeployAgentComponentFunc = func(ctx context.Context, orgName string, projName string, componentName string, language string, req *spec.DeployAgentRequest) error {
					return fmt.Errorf("internal service error")
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
			orgName:   deployTestOrgName,
			projName:  deployTestProjName,
			agentName: deployTestAgentName,
			payload: map[string]interface{}{
				"imageId": "registry.example.com/myapp:v1.0.0",
			},
			wantStatus: 401,
			wantErrMsg: "missing header: Authorization",
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClientForDeploy()
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

			// Prepare request body
			var reqBody *bytes.Buffer
			if tt.payload != nil {
				reqBody = new(bytes.Buffer)
				err := json.NewEncoder(reqBody).Encode(tt.payload)
				require.NoError(t, err)
			} else {
				reqBody = bytes.NewBuffer([]byte("invalid json"))
			}

			// Build URL
			url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
				tt.orgName, tt.projName, tt.agentName)

			req := httptest.NewRequest(http.MethodPost, url, reqBody)
			if tt.payload != nil {
				req.Header.Set("Content-Type", "application/json")
			}

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

func setUpDeployTest(t *testing.T) {
	_ = apitestutils.CreateOrganization(t, deployTestOrgId, deployTestUserIdpId, deployTestOrgName)
	_ = apitestutils.CreateProject(t, deployTestProjId, deployTestOrgId, deployTestProjName)
	_ = apitestutils.CreateAgent(t, uuid.New(), deployTestOrgId, deployTestProjId, deployTestAgentName, string(utils.InternalAgent))
}
