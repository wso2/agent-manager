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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/tests/apitestutils"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
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
			SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
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
		require.Equal(t, testProjName, createComponentCall.ProjectName)
		require.Equal(t, testAgentNameOne, createComponentCall.Req.Name)
		require.Equal(t, "Test Agent Description", createComponentCall.Req.Description)
	})

	t.Run("Creating an agent with ballerina language should return 202", func(t *testing.T) {
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
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
			SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
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
			SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
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
		require.Equal(t, testProjName, createComponentCall.ProjectName)
		require.Equal(t, testAgentNameDocker, createComponentCall.Req.Name)
		require.Equal(t, "Test Docker Agent Description", createComponentCall.Req.Description)
		require.Equal(t, "docker", createComponentCall.Req.Build.Type)
		require.Equal(t, "/Dockerfile", createComponentCall.Req.Build.Docker.DockerfilePath)

		// Validate that all traits were attached in a single call
		attachTraitsCalls := openChoreoClient.AttachTraitsCalls()
		require.Len(t, attachTraitsCalls, 1, "Should have called AttachTraits once with all traits")

		attachCall := attachTraitsCalls[0]
		require.Equal(t, testProjName, attachCall.ProjectName)
		require.Equal(t, testAgentNameDocker, attachCall.ComponentName)
		require.Len(t, attachCall.TraitRequests, 2)
		require.Equal(t, client.TraitEnvInjection, attachCall.TraitRequests[0].TraitType, "Should attach env injection trait")
		require.Equal(t, client.TraitAPIManagement, attachCall.TraitRequests[1].TraitType, "Should attach api-configuration trait")
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
			SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
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
			wantErrMsg: "Agent name cannot be empty",
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
			wantErrMsg: "Agent name must contain only lowercase alphanumeric characters or '-'",
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
			wantErrMsg: "Internal agents require either a repository or an agentKind",
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
			wantErrMsg: "Invalid repository URL format. Please use: https://github.com/owner/repo",
			url:        fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName),
			setupMock: func() *clientmocks.OpenChoreoClientMock {
				return apitestutils.CreateMockOpenChoreoClient()
			},
		},
		{
			name:           "return 404 on organization not found",
			authMiddleware: jwtassertion.NewMockMiddlewareWithOUID(t, "nonexistent-org"),
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
			wantErrMsg: "The selected programming language is not supported",
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
			wantErrMsg: "The selected language version is not supported",
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
			wantErrMsg: "Please select a programming language",
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
			wantErrMsg: "Please specify a language version",
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
				SecretMgmtClient: apitestutils.CreateMockSecretManagementClient(),
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

// TestCreateAgentFromKind_SecretConfigDefaults covers agent creation from a kind whose
// config schema has a secret item: a secret's default value is never returned to a
// client, so the server applies it itself when the create-agent request leaves the
// field untouched, rather than relying on the client to round-trip a value it was
// never given.
func TestCreateAgentFromKind_SecretConfigDefaults(t *testing.T) {
	authMiddleware := jwtassertion.NewMockMiddleware(t)

	// Seeds an Agent Kind version whose config schema declares one mandatory secret
	// item with a default value.
	seedKindVersion := func(t *testing.T) (kindName, version string) {
		gdb := db.DB(context.Background())
		kindName = fmt.Sprintf("test-kind-%s", uuid.New().String()[:5])
		sourceAgentName := fmt.Sprintf("source-agent-%s", uuid.New().String()[:5])
		kind := &models.AgentKind{
			ID:          uuid.New(),
			Name:        kindName,
			DisplayName: "Test Kind",
			OUID:        jwtassertion.MockOUID,
			ProjectName: testProjName,
			AgentName:   sourceAgentName,
			Labels:      map[string]string{},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, gdb.Create(kind).Error)
		t.Cleanup(func() { gdb.Delete(kind) })

		version = "v1"
		kindVersion := &models.AgentKindVersion{
			ID:          uuid.New(),
			AgentKindID: kind.ID,
			Version:     version,
			ImageId:     "sha256:test-kind-image",
			ConfigSchema: []models.KindConfigSchemaItem{
				{Name: "OPENAI_API_KEY", IsSecret: true, IsMandatory: true, DefaultValue: strPtr("sk-kind-default-secret")},
			},
			CreatedAt: time.Now(),
		}
		require.NoError(t, gdb.Create(kindVersion).Error)
		t.Cleanup(func() { gdb.Delete(kindVersion) })

		return kindName, version
	}

	t.Run("omitting a mandatory secret field applies the kind's default server-side, never exposing it", func(t *testing.T) {
		kindName, version := seedKindVersion(t)
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		// Kind-based creation calls CreateInternalAgentFromKindWorkload (not
		// TriggerBuild) since the image is already built; the shared mock has no
		// default for it.
		openChoreoClient.CreateInternalAgentFromKindWorkloadFunc = func(ctx context.Context, ouID, projectName, componentName string, req client.InternalAgentFromKindWorkloadRequest) error {
			return nil
		}
		// Kind creation now also cuts the ComponentRelease and binds it to the first
		// environment, which is where its configuration lives; the shared mock has no
		// default for it either.
		openChoreoClient.EnsureReleaseAndBindingFunc = func(ctx context.Context, ouID, projectName, componentName, environment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar) error {
			return nil
		}
		secretMgmtClient := apitestutils.CreateMockSecretManagementClient()
		testClients := wiring.TestClients{
			OpenChoreoClient: openChoreoClient,
			SecretMgmtClient: secretMgmtClient,
		}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		agentName := fmt.Sprintf("test-agent-%s", uuid.New().String()[:5])
		reqBody := new(bytes.Buffer)
		require.NoError(t, json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"name":        agentName,
			"displayName": "Kind Agent",
			"provisioning": map[string]interface{}{
				"type": "internal",
				"agentKind": map[string]interface{}{
					"name":    kindName,
					"version": version,
				},
			},
			// No configurations.env at all: the mandatory secret field is left
			// untouched, exactly like a user accepting the kind's default without
			// typing anything (the frontend never even receives a value to submit).
		}))

		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName)
		req := httptest.NewRequest(http.MethodPost, url, reqBody)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusAccepted, rr.Code, rr.Body.String())

		require.NotContains(t, rr.Body.String(), "sk-kind-default-secret",
			"the 202 body must not echo the kind's stored secret default, which the caller never supplied and is not entitled to")

		require.Len(t, secretMgmtClient.CreateSecretCalls(), 1)
		require.Equal(t, "sk-kind-default-secret", secretMgmtClient.CreateSecretCalls()[0].Data["OPENAI_API_KEY"])

		require.Len(t, openChoreoClient.CreateComponentCalls(), 1)
		envVars := openChoreoClient.CreateComponentCalls()[0].Req.Configurations.Env
		require.Len(t, envVars, 1)
		require.Empty(t, envVars[0].Value, "the OpenChoreo-bound request must never carry the plaintext secret")
		require.NotNil(t, envVars[0].ValueFrom)
		require.NotNil(t, envVars[0].ValueFrom.SecretKeyRef)
	})

	t.Run("an explicit override value wins over the kind's default and is still treated as sensitive", func(t *testing.T) {
		kindName, version := seedKindVersion(t)
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		// Kind-based creation calls CreateInternalAgentFromKindWorkload (not
		// TriggerBuild) since the image is already built; the shared mock has no
		// default for it.
		openChoreoClient.CreateInternalAgentFromKindWorkloadFunc = func(ctx context.Context, ouID, projectName, componentName string, req client.InternalAgentFromKindWorkloadRequest) error {
			return nil
		}
		// Kind creation now also cuts the ComponentRelease and binds it to the first
		// environment, which is where its configuration lives; the shared mock has no
		// default for it either.
		openChoreoClient.EnsureReleaseAndBindingFunc = func(ctx context.Context, ouID, projectName, componentName, environment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar) error {
			return nil
		}
		secretMgmtClient := apitestutils.CreateMockSecretManagementClient()
		testClients := wiring.TestClients{
			OpenChoreoClient: openChoreoClient,
			SecretMgmtClient: secretMgmtClient,
		}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		agentName := fmt.Sprintf("test-agent-%s", uuid.New().String()[:5])
		reqBody := new(bytes.Buffer)
		require.NoError(t, json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"name":        agentName,
			"displayName": "Kind Agent Override",
			"provisioning": map[string]interface{}{
				"type": "internal",
				"agentKind": map[string]interface{}{
					"name":    kindName,
					"version": version,
				},
			},
			"configurations": map[string]interface{}{
				"env": []map[string]interface{}{
					// isSensitive deliberately false: the server must force it true
					// for any key matching a secret schema item, regardless of what
					// the client claims.
					{"key": "OPENAI_API_KEY", "value": "my-own-api-key", "isSensitive": false},
				},
			},
		}))

		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName)
		req := httptest.NewRequest(http.MethodPost, url, reqBody)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusAccepted, rr.Code, rr.Body.String())

		require.NotContains(t, rr.Body.String(), "my-own-api-key",
			"the 202 body must not echo a submitted secret value back to the caller")

		require.Len(t, secretMgmtClient.CreateSecretCalls(), 1)
		require.Equal(t, "my-own-api-key", secretMgmtClient.CreateSecretCalls()[0].Data["OPENAI_API_KEY"])

		require.Len(t, openChoreoClient.CreateComponentCalls(), 1)
		envVars := openChoreoClient.CreateComponentCalls()[0].Req.Configurations.Env
		require.Len(t, envVars, 1)
		require.Empty(t, envVars[0].Value, "a kind-declared secret must never be emitted as plaintext, even if the client claimed isSensitive=false")
		require.NotNil(t, envVars[0].ValueFrom)
		require.NotNil(t, envVars[0].ValueFrom.SecretKeyRef)
	})

	t.Run("rejects a duplicate env var key instead of silently picking one", func(t *testing.T) {
		kindName, version := seedKindVersion(t)
		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		secretMgmtClient := apitestutils.CreateMockSecretManagementClient()
		testClients := wiring.TestClients{
			OpenChoreoClient: openChoreoClient,
			SecretMgmtClient: secretMgmtClient,
		}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		agentName := fmt.Sprintf("test-agent-%s", uuid.New().String()[:5])
		reqBody := new(bytes.Buffer)
		require.NoError(t, json.NewEncoder(reqBody).Encode(map[string]interface{}{
			"name":        agentName,
			"displayName": "Kind Agent Duplicate Key",
			"provisioning": map[string]interface{}{
				"type": "internal",
				"agentKind": map[string]interface{}{
					"name":    kindName,
					"version": version,
				},
			},
			"configurations": map[string]interface{}{
				"env": []map[string]interface{}{
					// The same key submitted twice, once as a plaintext, unmarked value —
					// exactly the shape that would otherwise let an earlier duplicate slip
					// through unsecreted while only the later one gets forced sensitive.
					{"key": "OPENAI_API_KEY", "value": "sneaked-in-plaintext", "isSensitive": false},
					{"key": "OPENAI_API_KEY", "value": "", "isSensitive": true},
				},
			},
		}))

		url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName)
		req := httptest.NewRequest(http.MethodPost, url, reqBody)
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		app.ServeHTTP(rr, req)

		require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
		require.Contains(t, rr.Body.String(), "duplicate environment variable key")
		require.Empty(t, secretMgmtClient.CreateSecretCalls(), "no secret must be stored once the request is rejected")
		require.Empty(t, openChoreoClient.CreateComponentCalls(), "no component must be created once the request is rejected")
	})

	t.Run("a mandatory secret with no kind default must be supplied by the caller", func(t *testing.T) {
		gdb := db.DB(context.Background())
		kindName := fmt.Sprintf("test-kind-%s", uuid.New().String()[:5])
		sourceAgentName := fmt.Sprintf("source-agent-%s", uuid.New().String()[:5])
		kind := &models.AgentKind{
			ID:          uuid.New(),
			Name:        kindName,
			DisplayName: "Test Kind No Default",
			OUID:        jwtassertion.MockOUID,
			ProjectName: testProjName,
			AgentName:   sourceAgentName,
			Labels:      map[string]string{},
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		require.NoError(t, gdb.Create(kind).Error)
		t.Cleanup(func() { gdb.Delete(kind) })

		version := "v1"
		kindVersion := &models.AgentKindVersion{
			ID:          uuid.New(),
			AgentKindID: kind.ID,
			Version:     version,
			ImageId:     "sha256:test-kind-image",
			// No DefaultValue: the kind author never baked one in, so whoever creates
			// an agent from this kind must supply their own value.
			ConfigSchema: []models.KindConfigSchemaItem{
				{Name: "OPENAI_API_KEY", IsSecret: true, IsMandatory: true},
			},
			CreatedAt: time.Now(),
		}
		require.NoError(t, gdb.Create(kindVersion).Error)
		t.Cleanup(func() { gdb.Delete(kindVersion) })

		openChoreoClient := apitestutils.CreateMockOpenChoreoClient()
		secretMgmtClient := apitestutils.CreateMockSecretManagementClient()
		testClients := wiring.TestClients{
			OpenChoreoClient: openChoreoClient,
			SecretMgmtClient: secretMgmtClient,
		}
		app := apitestutils.MakeAppClientWithDeps(t, testClients, authMiddleware)

		buildReqBody := func(env []map[string]interface{}) *bytes.Buffer {
			body := map[string]interface{}{
				"name":        fmt.Sprintf("test-agent-%s", uuid.New().String()[:5]),
				"displayName": "Kind Agent No Default",
				"provisioning": map[string]interface{}{
					"type": "internal",
					"agentKind": map[string]interface{}{
						"name":    kindName,
						"version": version,
					},
				},
			}
			if env != nil {
				body["configurations"] = map[string]interface{}{"env": env}
			}
			reqBody := new(bytes.Buffer)
			require.NoError(t, json.NewEncoder(reqBody).Encode(body))
			return reqBody
		}

		t.Run("omitted entirely: rejected, not silently created with a blank secret", func(t *testing.T) {
			url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName)
			req := httptest.NewRequest(http.MethodPost, url, buildReqBody(nil))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			require.Equal(t, http.StatusBadRequest, rr.Code, rr.Body.String())
			require.Contains(t, rr.Body.String(), "missing required configuration value")
			require.Empty(t, secretMgmtClient.CreateSecretCalls())
		})

		t.Run("supplied by the caller: creation succeeds with that value", func(t *testing.T) {
			openChoreoClient.CreateInternalAgentFromKindWorkloadFunc = func(ctx context.Context, ouID, projectName, componentName string, req client.InternalAgentFromKindWorkloadRequest) error {
				return nil
			}
			// Kind creation now also cuts the ComponentRelease and binds it to the first
			// environment, which is where its configuration lives; the shared mock has no
			// default for it either.
			openChoreoClient.EnsureReleaseAndBindingFunc = func(ctx context.Context, ouID, projectName, componentName, environment string, envOverrides []client.EnvVar, fileOverrides []client.FileVar) error {
				return nil
			}
			env := []map[string]interface{}{
				{"key": "OPENAI_API_KEY", "value": "caller-supplied-key", "isSensitive": false},
			}
			url := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", testOrgName, testProjName)
			req := httptest.NewRequest(http.MethodPost, url, buildReqBody(env))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			app.ServeHTTP(rr, req)

			require.Equal(t, http.StatusAccepted, rr.Code, rr.Body.String())
			require.Len(t, secretMgmtClient.CreateSecretCalls(), 1)
			require.Equal(t, "caller-supplied-key", secretMgmtClient.CreateSecretCalls()[0].Data["OPENAI_API_KEY"])
		})
	})
}
