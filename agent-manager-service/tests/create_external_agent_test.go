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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

var (
	testExternalOrgName      = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])
	testExternalProjName     = fmt.Sprintf("test-project-%s", uuid.New().String()[:5])
	testExternalAgentNameOne = fmt.Sprintf("nonexistent-agent-%s", uuid.New().String()[:5])
)

func TestCreateExternalAgent(t *testing.T) {
	seedIngressGatewayForDefaultEnvironment(t, jwtassertion.MockOUID)
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Creating an external agent should return 202", func(t *testing.T) {
		platformRepo := apitestutils.CreateMockPlatformRepository()
		testClients := wiring.TestClients{
			PlatformRepository: platformRepo,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create the request body for external agent
		reqBody := new(bytes.Buffer)
		err := json.NewEncoder(reqBody).Encode(agentDocument(testExternalProjName, map[string]interface{}{
			"name":        testExternalAgentNameOne,
			"displayName": "Test External Agent",
			"description": "Test External Agent Description",
			"provisioning": map[string]interface{}{
				"type": "external",
			},
			"agentType": map[string]interface{}{
				"type": "external-agent-api",
			},
		}))
		require.NoError(t, err)

		// Send the request
		req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", reqBody)
		req.Header.Set("Content-Type", "application/json")

		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		// Assert response
		require.Equal(t, http.StatusAccepted, rr.Code)

		// Read and validate response body
		b, err := io.ReadAll(rr.Body)
		require.NoError(t, err)
		t.Logf("response body: %s", string(b))

		var payload spec.AgentDocument
		require.NoError(t, json.Unmarshal(b, &payload))

		// Validate response fields
		require.Equal(t, testExternalAgentNameOne, payload.Metadata.Name)
		require.Equal(t, "Test External Agent Description", payload.Spec.GetDescription())
		require.Equal(t, testExternalProjName, payload.Metadata.Project)
		require.Equal(t, "external", payload.Spec.Provisioning.Type)
		require.NotZero(t, payload.Metadata.GetCreationTimestamp())
	})

	externalAgentValidationTests := []struct {
		name           string
		authMiddleware jwtassertion.Middleware
		payload        map[string]interface{}
		wantStatus     int
		wantErrMsg     string
		project        string
		setupMock      func() *repomocks.PlatformRepositoryMock
	}{
		{
			name:           "return 400 on missing agent name for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
				"agentType": map[string]interface{}{
					"type": "external-agent-api",
				},
			},
			wantStatus: 400,
			wantErrMsg: "metadata.name is required",
			project:    testExternalProjName,
			setupMock: func() *repomocks.PlatformRepositoryMock {
				return apitestutils.CreateMockPlatformRepository()
			},
		},
		{
			name:           "return 400 on invalid agent name for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        "Invalid Agent Name!", // Invalid characters
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
				"agentType": map[string]interface{}{
					"type": "external-agent-api",
				},
			},
			wantStatus: 400,
			wantErrMsg: "Agent name must contain only lowercase alphanumeric characters or '-'",
			project:    testExternalProjName,
			setupMock: func() *repomocks.PlatformRepositoryMock {
				return apitestutils.CreateMockPlatformRepository()
			},
		},
		{
			name:           "return 400 on missing display name for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5]),
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
				"agentType": map[string]interface{}{
					"type": "external-agent-api",
				},
			},
			wantStatus: 400,
			wantErrMsg: "Agent display name cannot be empty",
			project:    testExternalProjName,
			setupMock: func() *repomocks.PlatformRepositoryMock {
				return apitestutils.CreateMockPlatformRepository()
			},
		},
		{
			name:           "return 400 on missing provisioning type for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":         fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5]),
				"displayName":  "Test External Agent",
				"description":  "Test description",
				"provisioning": map[string]interface{}{
					// Missing "type" field
				},
				"agentType": map[string]interface{}{
					"type": "external-agent-api",
				},
			},
			wantStatus: 400,
			wantErrMsg: "provisioning type must be 'external'",
			project:    testExternalProjName,
			setupMock: func() *repomocks.PlatformRepositoryMock {
				return apitestutils.CreateMockPlatformRepository()
			},
		},
		{
			name:           "return 400 on invalid provisioning type for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "managed", // Invalid type
				},
				"agentType": map[string]interface{}{
					"type": "external-agent-api",
				},
			},
			wantStatus: 400,
			wantErrMsg: "provisioning type must be 'external'",
			project:    testExternalProjName,
			setupMock: func() *repomocks.PlatformRepositoryMock {
				return apitestutils.CreateMockPlatformRepository()
			},
		},
		{
			name:           "return 404 on project not found for external agent",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
				"agentType": map[string]interface{}{
					"type": "external-agent-api",
				},
			},
			wantStatus: 404,
			wantErrMsg: "Project not found",
			project:    "nonexistent-project",
			setupMock: func() *repomocks.PlatformRepositoryMock {
				mock := apitestutils.CreateMockPlatformRepository()
				mock.CreateAgentFunc = func(ctx context.Context, ouID, projectName string, in repositories.CreateAgentInput) (*models.AgentResponse, error) {
					if projectName == "nonexistent-project" {
						return nil, utils.ErrProjectNotFound
					}
					return &models.AgentResponse{Name: in.Name, ProjectName: projectName}, nil
				}
				return mock
			},
		},
		{
			name:           "return 409 on external agent already exists",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        testExternalAgentNameOne, // Use existing agent name
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
				"agentType": map[string]interface{}{
					"type": "external-agent-api",
				},
			},
			wantStatus: 409,
			wantErrMsg: "Agent already exists",
			project:    testExternalProjName,
			setupMock: func() *repomocks.PlatformRepositoryMock {
				mock := apitestutils.CreateMockPlatformRepository()
				// Override CreateAgentFunc to return error indicating agent already exists
				mock.CreateAgentFunc = func(ctx context.Context, ouID, projectName string, in repositories.CreateAgentInput) (*models.AgentResponse, error) {
					return nil, utils.ErrAgentAlreadyExists
				}
				return mock
			},
		},
		{
			name: "return 401 on missing authentication for external agent",
			authMiddleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					httpx.WriteErrorResponse(w, http.StatusUnauthorized, "missing header: Authorization")
				})
			},
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-external-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test External Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "external",
				},
				"agentType": map[string]interface{}{
					"type": "external-agent-api",
				},
			},
			wantStatus: 401,
			wantErrMsg: "missing header: Authorization",
			project:    testExternalProjName,
			setupMock: func() *repomocks.PlatformRepositoryMock {
				return apitestutils.CreateMockPlatformRepository()
			},
		},
	}

	for _, tt := range externalAgentValidationTests {
		t.Run(tt.name, func(t *testing.T) {
			platformRepo := tt.setupMock()
			testClients := wiring.TestClients{
				PlatformRepository: platformRepo,
			}

			app := apitestutils.MakeAppClientWithDeps(t, testClients, tt.authMiddleware)

			reqBody := new(bytes.Buffer)
			err := json.NewEncoder(reqBody).Encode(agentDocument(tt.project, tt.payload))
			require.NoError(t, err)

			// Send the request
			req := httptest.NewRequest(http.MethodPost, "/api/v1/agents", reqBody)
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
			} else if tt.wantStatus == 202 {
				// For success responses, validate the response structure
				var payload spec.AgentDocument
				require.NoError(t, json.Unmarshal(body, &payload))
				require.Equal(t, "external", payload.Spec.Provisioning.Type)
			}
		})
	}
}
