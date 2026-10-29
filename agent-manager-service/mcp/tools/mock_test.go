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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MockToolsetHandler implements every toolset interface for use in tool-package tests.
type MockToolsetHandler struct {
	calls          map[string][]interface{}
	createAgentErr error
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
	return &models.EnvironmentListResponse{
		Environments: []models.GatewayEnvironmentResponse{
			{Name: testEnvName, DisplayName: "Default Env", IsProduction: false},
		},
		Total:  1,
		Limit:  limit,
		Offset: offset,
	}, nil
}

func (m *MockToolsetHandler) GetEnvironment(
	ctx context.Context, ouID string, envName string,
) (*models.GatewayEnvironmentResponse, error) {
	m.recordCall("GetEnvironment", ouID, envName)
	if envName != testEnvName {
		return nil, utils.ErrEnvironmentNotFound
	}
	return &models.GatewayEnvironmentResponse{UUID: testEnvUUID, Name: testEnvName, DisplayName: "Default Env", IsProduction: false}, nil
}

// LLM Provider Toolset Handler

func (m *MockToolsetHandler) ListLLMProviderTemplates(
	ctx context.Context, ouID string, limit int, offset int,
) ([]*models.LLMProviderTemplate, int, error) {
	m.recordCall("ListLLMProviderTemplates", ouID, limit, offset)
	return []*models.LLMProviderTemplate{{
		Handle: testTemplateID,
		Name:   "OpenAI",
		Metadata: &models.LLMProviderTemplateMetadata{
			EndpointURL: "https://api.openai.com",
			Auth:        &models.LLMProviderTemplateAuth{Type: "api-key", Header: "Authorization", ValuePrefix: "Bearer "},
		},
	}}, 1, nil
}

func (m *MockToolsetHandler) ListLLMProviders(
	ctx context.Context, ouID string, limit int, offset int,
) ([]*models.LLMProvider, int, error) {
	m.recordCall("ListLLMProviders", ouID, limit, offset)
	p := &models.LLMProvider{
		TemplateHandle: testTemplateID, Status: "deployed",
		Artifact: &models.Artifact{Handle: testProviderID, Name: "Test Provider"},
	}
	return []*models.LLMProvider{p}, 1, nil
}

func (m *MockToolsetHandler) ListLLMGuardrails(
	ctx context.Context, ouID string, providerID string,
) (*models.LLMPolicyAvailabilityResponse, error) {
	m.recordCall("ListLLMGuardrails", ouID, providerID)
	return &models.LLMPolicyAvailabilityResponse{Count: 1, List: []models.LLMPolicyDefinition{
		{Name: testGuardrailName, Version: "v1"},
	}}, nil
}

func (m *MockToolsetHandler) CreateLLMProvider(
	ctx context.Context, ouID string, req *spec.CreateLLMProviderRequest,
) (*models.LLMProvider, []services.DeploymentResult, error) {
	m.recordCall("CreateLLMProvider", ouID, req)
	p := &models.LLMProvider{
		TemplateHandle: req.Template, Status: "created",
		Artifact: &models.Artifact{Handle: req.Id, Name: req.Name},
	}
	deployments := make([]services.DeploymentResult, 0, len(req.Gateways))
	for _, gw := range req.Gateways {
		deployments = append(deployments, services.DeploymentResult{GatewayID: gw, Success: true})
	}
	return p, deployments, nil
}

// Gateway Toolset Handler

func (m *MockToolsetHandler) ListGateways(
	ctx context.Context, ouID string, environmentID string, limit int, offset int,
) (*services.GatewayListResponse, error) {
	m.recordCall("ListGateways", ouID, environmentID, limit, offset)
	return &services.GatewayListResponse{
		Count:      1,
		List:       []services.GatewayResponse{{ID: testGatewayID, Name: "default-gateway", IsActive: true}},
		Pagination: services.Pagination{Total: 1, Limit: limit, Offset: offset},
	}, nil
}

// MCP Proxy Toolset Handler

func (m *MockToolsetHandler) ListMCPProxies(
	ctx context.Context, ouID string, limit int, offset int,
) (*models.MCPProxyListResponse, error) {
	m.recordCall("ListMCPProxies", ouID, limit, offset)
	id, name := testProxyID, "Test Proxy"
	return &models.MCPProxyListResponse{
		Count:      1,
		List:       []models.MCPProxyListItem{{ID: &id, Name: &name}},
		Pagination: models.PaginationInfo{Count: 1, Limit: limit, Offset: offset},
	}, nil
}

func (m *MockToolsetHandler) FetchMCPServerInfo(
	ctx context.Context, req *models.MCPServerInfoFetchRequest,
) (*models.MCPServerInfoFetchResponse, error) {
	m.recordCall("FetchMCPServerInfo", req)
	tools := []map[string]interface{}{{"name": "search"}}
	return &models.MCPServerInfoFetchResponse{Tools: &tools}, nil
}

func (m *MockToolsetHandler) CreateMCPProxy(
	ctx context.Context, ouID string, req *models.MCPProxyDTO,
) (*models.MCPProxyDTO, error) {
	m.recordCall("CreateMCPProxy", ouID, req)
	return req, nil
}

// Agent Config Toolset Handler

func (m *MockToolsetHandler) CreateAgentConfig(
	ctx context.Context, ouID string, projectName string, agentName string, req models.CreateAgentModelConfigRequest,
) (*models.AgentModelConfigResponse, error) {
	m.recordCall("CreateAgentConfig", ouID, projectName, agentName, req)
	return &models.AgentModelConfigResponse{
		UUID: "config-uuid",
		Name: req.Name,
		Type: req.Type,
		EnvironmentVariables: []models.EnvironmentVariableConfig{
			{Name: "OPENAI_URL", Key: "url"},
			{Name: "OPENAI_API_KEY", Key: "apikey"},
		},
	}, nil
}
