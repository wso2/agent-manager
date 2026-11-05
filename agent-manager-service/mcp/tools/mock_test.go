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

package tools

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MockToolsetHandler implements every toolset interface for use in tool-package tests.
type MockToolsetHandler struct {
	calls          map[string][]interface{}
	createAgentErr error
	attachErr      error
	environments   []models.GatewayEnvironmentResponse
	// platformAgent makes AttachLLMProvider answer as the service does for a
	// platform agent: no proxy URL and no API key value.
	platformAgent bool
}

func NewMockToolsetHandler() *MockToolsetHandler {
	return &MockToolsetHandler{
		calls: make(map[string][]interface{}),
	}
}

func (m *MockToolsetHandler) recordCall(method string, args ...interface{}) {
	m.calls[method] = append(m.calls[method], args)
}

// Project Toolset Handler

func (m *MockToolsetHandler) ListProjects(
	ctx context.Context, ouID string, limit int, offset int,
) ([]*models.ProjectResponse, int32, error) {
	m.recordCall("ListProjects", ouID, limit, offset)
	return []*models.ProjectResponse{
		{Name: "test-project", CreatedAt: time.Time{}},
	}, 1, nil
}

func (m *MockToolsetHandler) CreateProject(
	ctx context.Context, ouID string, payload spec.CreateProjectRequest,
) (*models.ProjectResponse, error) {
	m.recordCall("CreateProject", ouID, payload)
	return &models.ProjectResponse{Name: payload.Name, CreatedAt: time.Time{}}, nil
}

// Agent Toolset Handler

func (m *MockToolsetHandler) ListAgents(
	ctx context.Context, ouID string, projName string, limit int32, offset int32,
) ([]*models.AgentResponse, int32, error) {
	m.recordCall("ListAgents", ouID, projName, limit, offset)
	return []*models.AgentResponse{
		{Name: "test-agent"},
	}, 1, nil
}

func (m *MockToolsetHandler) GenerateToken(
	ctx context.Context, ouID string, projectName string, agentName string,
	environment string, expiresIn string,
) (*spec.TokenResponse, error) {
	m.recordCall("GenerateToken", ouID, projectName, agentName, environment, expiresIn)
	return &spec.TokenResponse{
		Token:     "test-token",
		ExpiresAt: 0,
	}, nil
}

func (m *MockToolsetHandler) CreateAgent(
	ctx context.Context, ouID string, projectName string, req *spec.CreateAgentRequest,
) error {
	m.recordCall("CreateAgent", ouID, projectName, req)
	return m.createAgentErr
}

// Build Toolset Handler

func (m *MockToolsetHandler) ListAgentBuilds(
	ctx context.Context, ouID string, projectName string, agentName string,
	limit int32, offset int32,
) ([]*models.BuildResponse, int32, error) {
	m.recordCall("ListAgentBuilds", ouID, projectName, agentName, limit, offset)
	return []*models.BuildResponse{
		{Name: "test-build", Status: "BuildSucceeded"},
	}, 1, nil
}

func (m *MockToolsetHandler) GetBuild(
	ctx context.Context, ouID string, projectName string, agentName string, buildName string,
) (*models.BuildDetailsResponse, error) {
	m.recordCall("GetBuild", ouID, projectName, agentName, buildName)
	return &models.BuildDetailsResponse{
		BuildResponse: models.BuildResponse{
			Name:   buildName,
			Status: "BuildSucceeded",
		},
	}, nil
}

func (m *MockToolsetHandler) BuildAgent(
	ctx context.Context, ouID string, projectName string, agentName string, commitId string,
) (*models.BuildResponse, error) {
	m.recordCall("BuildAgent", ouID, projectName, agentName, commitId)
	return &models.BuildResponse{Name: "test-build", Status: "BuildInitiated"}, nil
}

// Deployment Toolset Handler

func (m *MockToolsetHandler) GetAgentDeployments(
	ctx context.Context, ouID string, projectName string, agentName string,
) ([]*models.DeploymentResponse, error) {
	m.recordCall("GetAgentDeployments", ouID, projectName, agentName)
	return []*models.DeploymentResponse{}, nil
}

func (m *MockToolsetHandler) DeployAgent(
	ctx context.Context, ouID string, projectName string, agentName string,
	req *spec.DeployAgentRequest,
) (string, error) {
	m.recordCall("DeployAgent", ouID, projectName, agentName, req)
	return testEnvName, nil
}

func (m *MockToolsetHandler) UpdateDeploymentState(
	ctx context.Context, ouID string, projectName string, agentName string,
	environment string, state string,
) error {
	m.recordCall("UpdateDeploymentState", ouID, projectName, agentName, environment, state)
	return nil
}

// Environment Toolset Handler

func (m *MockToolsetHandler) ListEnvironments(
	ctx context.Context, ouID string, limit int32, offset int32,
) (*models.EnvironmentListResponse, error) {
	m.recordCall("ListEnvironments", ouID, limit, offset)
	envs := m.environments
	if envs == nil {
		envs = []models.GatewayEnvironmentResponse{
			{Name: testEnvName, DisplayName: "Default Env", IsProduction: false},
		}
	}
	return &models.EnvironmentListResponse{
		Environments: envs,
		Total:        int32(len(envs)),
		Limit:        limit,
		Offset:       offset,
	}, nil
}

func (m *MockToolsetHandler) GetEnvironment(
	ctx context.Context, ouID string, envName string,
) (*models.GatewayEnvironmentResponse, error) {
	m.recordCall("GetEnvironment", ouID, envName)
	if envName != testEnvName {
		return nil, utils.ErrEnvironmentNotFound
	}
	return &models.GatewayEnvironmentResponse{Name: testEnvName, DisplayName: "Default Env", IsProduction: false}, nil
}

// LLM Provider Toolset Handler

func testLLMProvider(inCatalog bool) *models.LLMProvider {
	return &models.LLMProvider{
		UUID:           uuid.MustParse("11111111-1111-1111-1111-111111111111"),
		TemplateHandle: "openai",
		Status:         "deployed",
		InCatalog:      inCatalog,
		Artifact:       &models.Artifact{Handle: testProviderID, Name: "Test Provider", InCatalog: inCatalog},
		Configuration: models.LLMProviderConfig{
			Upstream: &models.UpstreamConfig{Main: &models.UpstreamEndpoint{
				URL:  "https://api.openai.example",
				Auth: &models.UpstreamAuth{Value: utils.StrAsStrPointer(testUpstreamSecret)},
			}},
		},
	}
}

func (m *MockToolsetHandler) ListLLMProviders(
	ctx context.Context, ouID string, limit int, offset int,
) ([]*models.LLMProvider, int, error) {
	m.recordCall("ListLLMProviders", ouID, limit, offset)
	return []*models.LLMProvider{testLLMProvider(true)}, 1, nil
}

func (m *MockToolsetHandler) AddLLMProviderToCatalog(
	ctx context.Context, ouID string, providerID string,
) (*models.LLMProvider, bool, error) {
	m.recordCall("AddLLMProviderToCatalog", ouID, providerID)
	if providerID != testProviderID {
		return nil, false, utils.ErrLLMProviderNotFound
	}
	return testLLMProvider(true), true, nil
}

func (m *MockToolsetHandler) AttachLLMProvider(
	ctx context.Context, ouID string, projectName string, agentName string,
	req models.CreateAgentModelConfigRequest,
) (*models.AgentModelConfigResponse, error) {
	m.recordCall("AttachLLMProvider", ouID, projectName, agentName, req)
	if m.attachErr != nil {
		return nil, m.attachErr
	}
	envs := make(map[string]models.EnvModelConfigResponse, len(req.EnvMappings))
	for name, mapping := range req.EnvMappings {
		proxy := &models.LLMProxyInfo{ProviderName: utils.StrAsStrPointer(mapping.ProviderName)}
		if !m.platformAgent {
			proxy.URL = utils.StrAsStrPointer("https://gw.example/" + name)
			proxy.APIKey = utils.StrAsStrPointer(testProxyAPIKey)
		}
		envs[name] = models.EnvModelConfigResponse{EnvironmentName: name, LLMProxy: proxy}
	}
	return &models.AgentModelConfigResponse{
		UUID:                 "22222222-2222-2222-2222-222222222222",
		Name:                 req.Name,
		Type:                 req.Type,
		ProjectName:          projectName,
		EnvModelConfig:       envs,
		EnvironmentVariables: req.EnvironmentVariables,
	}, nil
}
