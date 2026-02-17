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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/clientmocks"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/tests/apitestutils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/wiring"
)

var (
	testOrgName      = fmt.Sprintf("test-org-%s", uuid.New().String()[:5])
	testProjName     = fmt.Sprintf("test-project-%s", uuid.New().String()[:5])
	testAgentNameOne = fmt.Sprintf("nonexistent-agent-%s", uuid.New().String()[:5])
	testAgentNameTwo = fmt.Sprintf("nonexistent-agent-%s", uuid.New().String()[:5])
)

func TestCreateAgent(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	t.Run("Creating an agent with default interface should return 202", func(t *testing.T) {
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()

		// Override GetComponentFunc to return valid component for token generation
		openChoreoClient.GetComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				UUID:        uuid.New().String(),
				Name:        componentName,
				ProjectName: projectName,
				Provisioning: models.Provisioning{
					Type: "internal",
				},
				CreatedAt: time.Now(),
			}, nil
		}

		testClients := wiring.TestClients{
			OpenChoreoClient: openChoreoClient,
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
					"appPath": "/agent-sample",
				},
			},
			"agentType": map[string]interface{}{
				"type":    "agent-api",
				"subType": "chat-api",
			},
			"build": map[string]interface{}{
				"type": "buildpack",
				"buildpack": map[string]interface{}{
					"language":        "python",
					"languageVersion": "3.11",
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
				},
			},
			"inputInterface": map[string]interface{}{
				"type": "HTTP",
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
		require.Len(t, openChoreoClient.CreateComponentCalls(), 1)
		require.Len(t, openChoreoClient.TriggerBuildCalls(), 1)

		// Validate call parameters
		createComponentCall := openChoreoClient.CreateComponentCalls()[0]
		require.Equal(t, testOrgName, createComponentCall.NamespaceName)
		require.Equal(t, testProjName, createComponentCall.ProjectName)
		require.Equal(t, testAgentNameOne, createComponentCall.Req.Name)
		require.Equal(t, "Test Agent Description", createComponentCall.Req.Description)
	})

	t.Run("Creating an agent with ballerina language should return 202", func(t *testing.T) {
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		testClients := wiring.TestClients{
			OpenChoreoClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create the request body for Ballerina agent (no language version or run command)
		testAgentNameBallerina := fmt.Sprintf("nonexistent-agent-%s", uuid.New().String()[:5])
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
					"appPath": "/ballerina-agent",
				},
			},
			"build": map[string]interface{}{
				"type": "buildpack",
				"buildpack": map[string]interface{}{
					"language": "ballerina",
					// No languageVersion or runCommand for Ballerina
				},
			},
			"agentType": map[string]interface{}{
				"type":    "agent-api",
				"subType": "chat-api",
			},
			"inputInterface": map[string]interface{}{
				"type": "HTTP",
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
		require.Len(t, openChoreoClient.CreateComponentCalls(), 1)
		require.Len(t, openChoreoClient.TriggerBuildCalls(), 1)

		// Validate call parameters
		createComponentCall := openChoreoClient.CreateComponentCalls()[0]
		require.Equal(t, testOrgName, createComponentCall.NamespaceName)
		require.Equal(t, testProjName, createComponentCall.ProjectName)
		require.Equal(t, testAgentNameBallerina, createComponentCall.Req.Name)
		require.Equal(t, "Test Ballerina Agent Description", createComponentCall.Req.Description)
		require.Equal(t, "ballerina", createComponentCall.Req.Build.Buildpack.Language)
	})

	t.Run("Creating an agent with docker build should return 202", func(t *testing.T) {
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()

		// Override GetComponentFunc to return valid component for token generation
		testAgentNameDocker := fmt.Sprintf("docker-agent-%s", uuid.New().String()[:5])
		openChoreoClient.GetComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				UUID:        uuid.New().String(),
				Name:        componentName,
				ProjectName: projectName,
				Provisioning: models.Provisioning{
					Type: "internal",
				},
				CreatedAt: time.Now(),
			}, nil
		}

		testClients := wiring.TestClients{
			OpenChoreoClient: openChoreoClient,
		}

		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		// Create the request body for Docker-based agent
		reqBody := new(bytes.Buffer)
		err := json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"name":        testAgentNameDocker,
			"displayName": "Test Docker Agent",
			"description": "Test Docker Agent Description",
			"provisioning": map[string]interface{}{
				"type": "internal",
				"repository": map[string]interface{}{
					"url":     "https://github.com/test/test-docker-repo",
					"branch":  "main",
					"appPath": "/docker-agent",
				},
			},
			"build": map[string]interface{}{
				"type": "docker",
				"docker": map[string]interface{}{
					"dockerfilePath": "/Dockerfile",
				},
			},
			"agentType": map[string]interface{}{
				"type":    "agent-api",
				"subType": "chat-api",
			},
			"inputInterface": map[string]interface{}{
				"type": "HTTP",
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
		require.Equal(t, testAgentNameDocker, payload.Name)
		require.Equal(t, "Test Docker Agent Description", payload.Description)
		require.Equal(t, testProjName, payload.ProjectName)
		require.NotZero(t, payload.CreatedAt)

		// Validate service calls
		require.Len(t, openChoreoClient.CreateComponentCalls(), 1)
		require.Len(t, openChoreoClient.TriggerBuildCalls(), 1)

		// Validate call parameters
		createComponentCall := openChoreoClient.CreateComponentCalls()[0]
		require.Equal(t, testOrgName, createComponentCall.NamespaceName)
		require.Equal(t, testProjName, createComponentCall.ProjectName)
		require.Equal(t, testAgentNameDocker, createComponentCall.Req.Name)
		require.Equal(t, "Test Docker Agent Description", createComponentCall.Req.Description)
		require.Equal(t, "docker", createComponentCall.Req.Build.Type)
		require.Equal(t, "/Dockerfile", createComponentCall.Req.Build.Docker.DockerfilePath)

		// Validate that tracing environment variables were injected via UpdateComponentEnvironmentVariables
		updateEnvVarsCalls := openChoreoClient.UpdateComponentEnvironmentVariablesCalls()
		require.Len(t, updateEnvVarsCalls, 1, "Should have called UpdateComponentEnvironmentVariables once")

		updateCall := updateEnvVarsCalls[0]
		require.Equal(t, testOrgName, updateCall.NamespaceName)
		require.Equal(t, testProjName, updateCall.ProjectName)
		require.Equal(t, testAgentNameDocker, updateCall.ComponentName)
		require.Len(t, updateCall.EnvVars, 2, "Should have 2 tracing env vars injected")

		// Verify tracing env vars are present
		envVarMap := make(map[string]string)
		for _, env := range updateCall.EnvVars {
			envVarMap[env.Key] = env.Value
		}

		require.Contains(t, envVarMap, client.EnvVarOTELEndpoint)
		require.NotEmpty(t, envVarMap[client.EnvVarOTELEndpoint])

		require.Contains(t, envVarMap, client.EnvVarAgentAPIKey)
		require.NotEmpty(t, envVarMap[client.EnvVarAgentAPIKey])
	})

	t.Run("Creating an agent with custom interface should return 202", func(t *testing.T) {
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()

		// Override GetComponentFunc to return valid component for token generation
		openChoreoClient.GetComponentFunc = func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				UUID:        uuid.New().String(),
				Name:        componentName,
				ProjectName: projectName,
				Provisioning: models.Provisioning{
					Type: "internal",
				},
				CreatedAt: time.Now(),
			}, nil
		}

		testClients := wiring.TestClients{
			OpenChoreoClient: openChoreoClient,
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
					"appPath": "/agent-sample",
				},
			},
			"build": map[string]interface{}{
				"type": "buildpack",
				"buildpack": map[string]interface{}{
					"language":        "python",
					"languageVersion": "3.11",
					"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
				},
			},
			"configurations": map[string]interface{}{
				"env": []map[string]interface{}{
					{
						"key":   "DB_HOST",
						"value": "aiven",
					},
				},
			},
			"agentType": map[string]interface{}{
				"type":    "agent-api",
				"subType": "custom-api",
			},
			"inputInterface": map[string]interface{}{
				"type":     "HTTP",
				"port":     5000,
				"basePath": "/reading-list",
				"schema": map[string]interface{}{
					"path": "/openapi.yaml",
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
		require.Len(t, openChoreoClient.CreateComponentCalls(), 1)
		require.Len(t, openChoreoClient.TriggerBuildCalls(), 1)

		// Validate call parameters
		createComponentCall := openChoreoClient.CreateComponentCalls()[0]
		require.Equal(t, testOrgName, createComponentCall.NamespaceName)
		require.Equal(t, testProjName, createComponentCall.ProjectName)
		require.Equal(t, testAgentNameTwo, createComponentCall.Req.Name)
		require.Equal(t, "Test Agent Description", createComponentCall.Req.Description)

		// Validate build configs
		require.Equal(t, "uvicorn app:app --host 0.0.0.0 --port 8000", createComponentCall.Req.Build.Buildpack.RunCommand)
		require.Equal(t, "3.11", createComponentCall.Req.Build.Buildpack.LanguageVersion)
	})

	validationTests := []struct {
		name           string
		authMiddleware jwtassertion.Middleware
		payload        map[string]interface{}
		wantStatus     int
		wantErrMsg     string
		url            string
		setupMock      func() *clientmocks.OpenChoreoClientMock
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
						"appPath": "/agent-sample",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "python",
						"languageVersion": "3.11",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid agent name: agent name cannot be empty",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				return apitestutils.CreateMockOpenChoreoClient()
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
						"appPath": "/agent-sample",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "python",
						"languageVersion": "3.11",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid agent name: agent name must contain only lowercase alphanumeric characters or '-'",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				return apitestutils.CreateMockOpenChoreoClient()
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
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "python",
						"languageVersion": "3.11",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid repository details: repository details are required for internal agents",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				return apitestutils.CreateMockOpenChoreoClient()
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
						"appPath": "/sample-agent",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "python",
						"languageVersion": "3.11",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid repository details: invalid GitHub repository format (expected: https://github.com/owner/repo)",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				return apitestutils.CreateMockOpenChoreoClient()
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
						"appPath": "/sample-agent",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "python",
						"languageVersion": "3.11",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 404,
			wantErrMsg: "Organization not found",
			url:        fmt.Sprintf("/api/v1/orgs/nonexistent-org/projects/%s/agents", testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				mock := apitestutils.CreateMockOpenChoreoClient()
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
						"appPath": "/sample-agent",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "python",
						"languageVersion": "3.11",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 404,
			wantErrMsg: "Project not found",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/nonexistent-project/agents", testOrgName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				mock := apitestutils.CreateMockOpenChoreoClient()
				mock.CreateComponentFunc = func(ctx context.Context, namespaceName string, projectName string, req client.CreateComponentRequest) error {
					if projectName == "nonexistent-project" {
						return utils.ErrProjectNotFound
					}
					return nil
				}
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
						"appPath": "/sample-agent",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "python",
						"languageVersion": "3.11",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 409,
			wantErrMsg: "Agent already exists",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				mock := apitestutils.CreateMockOpenChoreoClient()
				mock.CreateComponentFunc = func(ctx context.Context, namespaceName string, projectName string, req client.CreateComponentRequest) error {
					// Return error to simulate agent already exists
					return utils.ErrAgentAlreadyExists
				}
				return mock
			},
		},
		{
			name:           "return 500 on service error",
			authMiddleware: authMiddleware,
			payload: map[string]interface{}{
				"name":        fmt.Sprintf("nonexistent-agent-%s", uuid.New().String()[:5]),
				"displayName": "Test Agent",
				"description": "Test description",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"repository": map[string]interface{}{
						"url":     "https://github.com/test/test-repo",
						"branch":  "main",
						"appPath": "/sample-agent",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "python",
						"languageVersion": "3.11",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 500,
			wantErrMsg: "Failed to create agent",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				mock := apitestutils.CreateMockOpenChoreoClient()
				mock.CreateComponentFunc = func(ctx context.Context, namespaceName string, projectName string, req client.CreateComponentRequest) error {
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
						"appPath": "/sample-agent",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "python",
						"languageVersion": "3.11",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 401,
			wantErrMsg: "missing header: Authorization",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				return apitestutils.CreateMockOpenChoreoClient()
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
						"appPath": "/agent-sample",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "rust", // Invalid language
						"languageVersion": "1.70",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid language: unsupported language 'rust'",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				return apitestutils.CreateMockOpenChoreoClient()
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
						"appPath": "/agent-sample",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":        "python",
						"languageVersion": "2.7", // Invalid version for python
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid language: unsupported language version '2.7' for language 'python'",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				return apitestutils.CreateMockOpenChoreoClient()
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
						"appPath": "/agent-sample",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"languageVersion": "3.11",
						"runCommand":      "uvicorn app:app --host 0.0.0.0 --port 8000",
						// Missing "language" field
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 400,
			wantErrMsg: "language cannot be empty",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				return apitestutils.CreateMockOpenChoreoClient()
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
						"appPath": "/agent-sample",
					},
				},
				"build": map[string]interface{}{
					"type": "buildpack",
					"buildpack": map[string]interface{}{
						"language":   "python",
						"runCommand": "uvicorn app:app --host 0.0.0.0 --port 8000",
						// Missing "languageVersion" field
					},
				},
				"agentType": map[string]interface{}{
					"type":    "agent-api",
					"subType": "chat-api",
				},
				"inputInterface": map[string]interface{}{
					"type": "HTTP",
				},
			},
			wantStatus: 400,
			wantErrMsg: "invalid language: language version cannot be empty",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				return apitestutils.CreateMockOpenChoreoClient()
			},
		},
	}

	for _, tt := range validationTests {
		t.Run(tt.name, func(t *testing.T) {
			openChoreoClient := tt.setupMock()
			testClients := wiring.TestClients{
				OpenChoreoClient: openChoreoClient,
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
