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
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/models"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/spec"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/utils"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/wiring"
)

var (
	testOrgId        = uuid.New()
	testProjId       = uuid.New()
	testUserIdpId    = uuid.New()
	testOrgName      = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])
	testProjName     = fmt.Sprintf("test-project-%s", uuid.New().String()[:5])
	testAgentNameOne = fmt.Sprintf("test-agent-%s", uuid.New().String()[:5])
	testAgentNameTwo = fmt.Sprintf("test-agent-%s", uuid.New().String()[:5])
)

func createMockOpenChoreoClient() *clientmocks.OpenChoreoSvcClientMock {
	return &clientmocks.OpenChoreoSvcClientMock{
		GetProjectFunc: func(ctx context.Context, projectName string, orgName string) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{
				Name:               projectName,
				DisplayName:        projectName,
				OrgName:            orgName,
				DeploymentPipeline: "test-pipeline",
				CreatedAt:          time.Now(),
			}, nil
		},
		IsAgentComponentExistsFunc: func(ctx context.Context, orgName string, projName string, agentName string) (bool, error) {
			return false, nil
		},
		CreateAgentComponentFunc: func(ctx context.Context, orgName string, projName string, req *spec.CreateAgentRequest) error {
			return nil
		},
		TriggerBuildFunc: func(ctx context.Context, orgName string, projName string, agentName string, commitId string) (*models.BuildResponse, error) {
			return &models.BuildResponse{
				UUID:        uuid.New().String(),
				Name:        fmt.Sprintf("%s-build-1", agentName),
				AgentName:   agentName,
				ProjectName: projName,
				CommitID:    "abc123def",
				Status:      "Running",
				StartedAt:   time.Now(),
			}, nil
		},
		SetupDeploymentFunc: func(ctx context.Context, orgName string, projName string, req *spec.CreateAgentRequest, envVars []spec.EnvironmentVariable) error {
			return nil
		},
		GetDeploymentPipelineFunc: func(ctx context.Context, orgName string, deploymentPipelineName string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{
				Name:        deploymentPipelineName,
				DisplayName: deploymentPipelineName,
				Description: "Test deployment pipeline",
				OrgName:     orgName,
				CreatedAt:   time.Now(),
				PromotionPaths: []models.PromotionPath{
					{
						SourceEnvironmentRef: "Development",
					},
				},
			}, nil
		},
	}
}

func TestCreateAgent(t *testing.T) {
	setUpTest(t)
	authMiddleware := jwtassertion.NewMockMiddleware(t, testOrgId, testUserIdpId)

	t.Run("Creating an agent with default interface should return 202", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClient()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create the request body
		reqBody := new(bytes.Buffer)
		err := json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"name":        testAgentNameOne,
			"displayName": "Test Agent",
			"description": "Test Agent Description",
			"provisioning": map[string]interface{}{
				"type": "internal",
				"repository": map[string]interface{}{
					"url":     "https://github.com/test/test-repo",
					"branch":  "main",
					"appPath": "agent-sample",
				},
			},
			"runtimeConfigs": map[string]interface{}{
				"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
				"language":        "python",
				"languageVersion": "3.11",
			},
			"inputInterface": map[string]interface{}{
				"type": "DEFAULT",
			},
		})
		require.NoError(t, err)

		// Send the request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName)
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

		var payload spec.AgentResponse
		require.NoError(t, json.Unmarshal(b, &payload))

		// Validate response fields
		require.Equal(t, testAgentNameOne, payload.Name)
		require.Equal(t, "Test Agent Description", payload.Description)
		require.Equal(t, testProjName, payload.ProjectName)
		require.NotZero(t, payload.CreatedAt)

		// Validate service calls
		require.Len(t, openChoreoClient.GetProjectCalls(), 1)
		require.Len(t, openChoreoClient.CreateAgentComponentCalls(), 1)
		require.Len(t, openChoreoClient.TriggerBuildCalls(), 1)
		require.Len(t, openChoreoClient.SetupDeploymentCalls(), 1)

		// Validate call parameters
		getProjectCall := openChoreoClient.GetProjectCalls()[0]
		require.Equal(t, testProjName, getProjectCall.ProjectName)
		require.Equal(t, testOrgName, getProjectCall.OrgName)

		createComponentCall := openChoreoClient.CreateAgentComponentCalls()[0]
		require.Equal(t, testOrgName, createComponentCall.OrgName)
		require.Equal(t, testProjName, createComponentCall.ProjName)
		require.Equal(t, testAgentNameOne, createComponentCall.Req.Name)
		require.Equal(t, "Test Agent Description", *createComponentCall.Req.Description)

		// Validate SetupDeployment call and environment variables
		setupDeploymentCall := openChoreoClient.SetupDeploymentCalls()[0]
		require.Equal(t, testOrgName, setupDeploymentCall.OrgName)
		require.Equal(t, testProjName, setupDeploymentCall.ProjName)
		require.Equal(t, testAgentNameOne, setupDeploymentCall.Req.Name)

		// Validate that system environment variables are included
		envVars := setupDeploymentCall.EnvVars
		require.GreaterOrEqual(t, len(envVars), 4, "Should have at least 4 system environment variables")

		// Check for system environment variables
		envMap := make(map[string]string)
		for _, env := range envVars {
			envMap[env.Key] = env.Value
		}
		require.Equal(t, testAgentNameOne, envMap["AMP_COMPONENT_ID"])
		require.Equal(t, testAgentNameOne, envMap["AMP_APP_NAME"])
		require.Equal(t, "1.0.0", envMap["AMP_APP_VERSION"])
		require.Equal(t, "Development", envMap["AMP_ENV"])
	})

	t.Run("Creating an agent with ballerina language should return 202", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClient()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create the request body for Ballerina agent (no language version or run command)
		testAgentNameBallerina := fmt.Sprintf("test-agent-%s", uuid.New().String()[:5])
		reqBody := new(bytes.Buffer)
		err := json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"name":        testAgentNameBallerina,
			"displayName": "Test Ballerina Agent",
			"description": "Test Ballerina Agent Description",
			"provisioning": map[string]interface{}{
				"type": "internal",
				"repository": map[string]interface{}{
					"url":     "https://github.com/test/test-ballerina-repo",
					"branch":  "main",
					"appPath": "ballerina-agent",
				},
			},
			"runtimeConfigs": map[string]interface{}{
				"language": "ballerina",
				// No languageVersion or runCommand for Ballerina
			},
			"inputInterface": map[string]interface{}{
				"type": "DEFAULT",
			},
		})
		require.NoError(t, err)

		// Send the request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName)
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

		var payload spec.AgentResponse
		require.NoError(t, json.Unmarshal(b, &payload))

		// Validate response fields
		require.Equal(t, testAgentNameBallerina, payload.Name)
		require.Equal(t, "Test Ballerina Agent Description", payload.Description)
		require.Equal(t, testProjName, payload.ProjectName)
		require.NotZero(t, payload.CreatedAt)

		// Validate service calls
		require.Len(t, openChoreoClient.GetProjectCalls(), 1)
		require.Len(t, openChoreoClient.CreateAgentComponentCalls(), 1)
		require.Len(t, openChoreoClient.TriggerBuildCalls(), 1)
		require.Len(t, openChoreoClient.SetupDeploymentCalls(), 1)

		// Validate call parameters
		getProjectCall := openChoreoClient.GetProjectCalls()[0]
		require.Equal(t, testProjName, getProjectCall.ProjectName)
		require.Equal(t, testOrgName, getProjectCall.OrgName)

		createComponentCall := openChoreoClient.CreateAgentComponentCalls()[0]
		require.Equal(t, testOrgName, createComponentCall.OrgName)
		require.Equal(t, testProjName, createComponentCall.ProjName)
		require.Equal(t, testAgentNameBallerina, createComponentCall.Req.Name)
		require.Equal(t, "Test Ballerina Agent Description", *createComponentCall.Req.Description)

		// Validate SetupDeployment call and environment variables
		setupDeploymentCall := openChoreoClient.SetupDeploymentCalls()[0]
		require.Equal(t, testOrgName, setupDeploymentCall.OrgName)
		require.Equal(t, testProjName, setupDeploymentCall.ProjName)
		require.Equal(t, testAgentNameBallerina, setupDeploymentCall.Req.Name)

		// Validate that system environment variables are 0 for Ballerina
		envVars := setupDeploymentCall.EnvVars
		require.Equal(t, 0, len(envVars), "Should have 0 system environment variables for Ballerina")

		// Validate runtime configs
		require.Equal(t, "ballerina", createComponentCall.Req.RuntimeConfigs.Language)
		require.Nil(t, createComponentCall.Req.RuntimeConfigs.LanguageVersion)
		require.Nil(t, createComponentCall.Req.RuntimeConfigs.RunCommand)
	})

	t.Run("Creating an agent with custom interface should return 202", func(t *testing.T) {
		openChoreoClient := createMockOpenChoreoClient()
		testClients := wiring.TestClients{
			OpenChoreoSvcClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create the request body with custom interface
		reqBody := new(bytes.Buffer)
		err := json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"name":        testAgentNameTwo,
			"displayName": "Test Agent",
			"description": "Test Agent Description",
			"provisioning": map[string]interface{}{
				"type": "internal",
				"repository": map[string]interface{}{
					"url":     "https://github.com/test/test-repo",
					"branch":  "main",
					"appPath": "agent-sample",
				},
			},
			"runtimeConfigs": map[string]interface{}{
				"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
				"language":        "python",
				"languageVersion": "3.11",
				"env": []map[string]interface{}{
					{
						"key":   "DB_HOST",
						"value": "aiven",
					},
				},
			},
			"inputInterface": map[string]interface{}{
				"type": "CUSTOM",
				"customOpenAPISpec": map[string]interface{}{
					"port":     5000,
					"basePath": "/reading-list",
					"schema": map[string]interface{}{
						"content": "openapi: 3.0.3\ninfo:\n  title: Basic API\n  version: 1.0.0\n\npaths:\n  /hello:\n    get:\n      summary: Returns a greeting\n      responses:\n        \"200\":\n          description: Successful response\n          content:\n            application/json:\n              schema:\n                type: object\n                properties:\n                  message:\n                    type: string\n                    example: Hello world",
					},
				},
			},
		})
		require.NoError(t, err)

		// Send the request
		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName)
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

		var payload spec.AgentResponse
		require.NoError(t, json.Unmarshal(b, &payload))

		// Validate response fields
		require.Equal(t, testAgentNameTwo, payload.Name)
		require.Equal(t, "Test Agent Description", payload.Description)
		require.Equal(t, testProjName, payload.ProjectName)
		require.NotZero(t, payload.CreatedAt)

		// Validate service calls
		require.Len(t, openChoreoClient.GetProjectCalls(), 1)
		require.Len(t, openChoreoClient.CreateAgentComponentCalls(), 1)
		require.Len(t, openChoreoClient.TriggerBuildCalls(), 1)
		require.Len(t, openChoreoClient.SetupDeploymentCalls(), 1)

		// Validate call parameters
		getProjectCall := openChoreoClient.GetProjectCalls()[0]
		require.Equal(t, testProjName, getProjectCall.ProjectName)
		require.Equal(t, testOrgName, getProjectCall.OrgName)

		createComponentCall := openChoreoClient.CreateAgentComponentCalls()[0]
		require.Equal(t, testOrgName, createComponentCall.OrgName)
		require.Equal(t, testProjName, createComponentCall.ProjName)
		require.Equal(t, testAgentNameTwo, createComponentCall.Req.Name)
		require.Equal(t, "Test Agent Description", *createComponentCall.Req.Description)

		// Validate SetupDeployment call and environment variables
		setupDeploymentCall := openChoreoClient.SetupDeploymentCalls()[0]
		require.Equal(t, testOrgName, setupDeploymentCall.OrgName)
		require.Equal(t, testProjName, setupDeploymentCall.ProjName)
		require.Equal(t, testAgentNameTwo, setupDeploymentCall.Req.Name)

		// Validate that system and user environment variables are merged correctly
		envVars := setupDeploymentCall.EnvVars
		require.GreaterOrEqual(t, len(envVars), 5, "Should have at least 5 environment variables (4 system + 1 user)")

		// Check for both system and user environment variables
		envMap := make(map[string]string)
		for _, env := range envVars {
			envMap[env.Key] = env.Value
		}

		// System environment variables
		require.Equal(t, testAgentNameTwo, envMap["AMP_COMPONENT_ID"])
		require.Equal(t, testAgentNameTwo, envMap["AMP_APP_NAME"])
		require.Equal(t, "1.0.0", envMap["AMP_APP_VERSION"])
		require.Equal(t, "Development", envMap["AMP_ENV"])

		// User environment variables from request
		require.Equal(t, "aiven", envMap["DB_HOST"])

		// Validate custom interface specific fields
		require.Equal(t, "CUSTOM", createComponentCall.Req.InputInterface.Type)
		require.NotNil(t, createComponentCall.Req.InputInterface.CustomOpenAPISpec)
		require.Equal(t, int32(5000), createComponentCall.Req.InputInterface.CustomOpenAPISpec.Port)
		require.Equal(t, "/reading-list", createComponentCall.Req.InputInterface.CustomOpenAPISpec.BasePath)
		require.Contains(t, createComponentCall.Req.InputInterface.CustomOpenAPISpec.Schema.Content, "openapi: 3.0.3")

		// Validate runtime configs
		require.Equal(t, "uvicorn app:app --host 0.0.0.0 --port 8000", *createComponentCall.Req.RuntimeConfigs.RunCommand)
		require.Equal(t, "3.11", *createComponentCall.Req.RuntimeConfigs.LanguageVersion)
		require.Len(t, createComponentCall.Req.RuntimeConfigs.Env, 1)
		require.Equal(t, "DB_HOST", createComponentCall.Req.RuntimeConfigs.Env[0].Key)
		require.Equal(t, "aiven", createComponentCall.Req.RuntimeConfigs.Env[0].Value)
	})

	validationTests := []struct {
		name           string
		authMiddleware jwtassertion.Middleware
		payload        map[string]interface{}
		wantStatus     int
		wantErrMsg     string
		url            string
		setupMock      func() *clientmocks.OpenChoreoSvcClientMock
	}{
		{
			name:           "return 400 on missing agent name",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "agent-sample",
					},
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					"language":        "python",
					"languageVersion": "3.11",
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid agent name: agent name cannot be empty",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on invalid agent name",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        "Invalid Agent Name!", // Invalid characters
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "agent-sample",
					},
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					"language":        "python",
					"languageVersion": "3.11",
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid agent name: agent name must contain only lowercase alphanumeric characters or '-'",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on missing repository",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					"language":        "python",
					"languageVersion": "3.11",
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid repository details: repository details are required for internal agents",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on invalid repository URL",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/invalid",
						"branch":  "main",
						"appPath": "sample-agent",
					},
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					"language":        "python",
					"languageVersion": "3.11",
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid repository details: invalid GitHub repository format (expected: https://github.com/owner/repo)",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClient()
			},
		},
		{
			name:           "return 404 on organization not found",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "sample-agent",
					},
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					"language":        "python",
					"languageVersion": "3.11",
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 404,
			wantErrMsg: "Organization not found",
			url:        fmt.Sprintf("/api/v1/orgs/nonexistent-org/projects/%s/agents", testProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClient()
				return mock
			},
		},
		{
			name:           "return 404 on project not found",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "sample-agent",
					},
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					"language":        "python",
					"languageVersion": "3.11",
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 404,
			wantErrMsg: "Project not found",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/nonexistent-project/agents", testOrgName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClient()
				return mock
			},
		},
		{
			name:           "return 409 on agent already exists",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        testAgentNameOne, // Use testAgentNameOne since this test specifically wants to test existing agent
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "sample-agent",
					},
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					"language":        "python",
					"languageVersion": "3.11",
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 409,
			wantErrMsg: "Agent already exists",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClient()
				mock.IsAgentComponentExistsFunc = func(ctx context.Context, orgName string, projName string, agentName string) (bool, error) {
					return true, nil
				}
				mock.CreateAgentComponentFunc = func(ctx context.Context, orgName string, projName string, req *spec.CreateAgentRequest) error {
					return utils.ErrAgentAlreadyExists
				}
				return mock
			},
		},
		{
			name:           "return 500 on service error",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "sample-agent",
					},
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					"language":        "python",
					"languageVersion": "3.11",
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 500,
			wantErrMsg: "Failed to create agent",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				mock := createMockOpenChoreoClient()
				mock.CreateAgentComponentFunc = func(ctx context.Context, orgName string, projName string, req *spec.CreateAgentRequest) error {
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
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "sample-agent",
					},
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					"language":        "python",
					"languageVersion": "3.11",
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 401,
			wantErrMsg: "missing header: Authorization",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on invalid language",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "agent-sample",
					},
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					"language":        "rust", // Invalid language
					"languageVersion": "1.70",
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid language: unsupported language 'rust'",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on invalid language version for python",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "agent-sample",
					},
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					"language":        "python",
					"languageVersion": "2.7", // Invalid version for python
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid language: unsupported language version '2.7' for language 'python'",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on missing language",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "agent-sample",
					},
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					"languageVersion": "3.11",
					// Missing "language" field
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 400,
			wantErrMsg: "language cannot be empty",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClient()
			},
		},
		{
			name:           "return 400 on missing language version",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "agent-sample",
					},
				},
				"runtimeConfigs": map[string]interface{}{
					"runCommand": "uvicorn app:app --host 0.0.0.0 --port 8000",
					"language":   "python",
					// Missing "languageVersion" field
				},
				"inputInterface": map[string]interface{}{
					"type": "DEFAULT",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid language: language version cannot be empty",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoSvcClientMock {
				return createMockOpenChoreoClient()
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

func setUpTest(t *testing.T) {
	_ = apitestutils.CreateOrganization(t, testOrgId, testUserIdpId, testOrgName)
	_ = apitestutils.CreateProject(t, testProjId, testOrgId, testProjName)
}
