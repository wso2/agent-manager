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
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// MockToolsetHandler implements every toolset interface for use in tool-package tests.
type MockToolsetHandler struct {
	calls map[string][]interface{}
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
	ctx context.Context, orgName string, limit int, offset int,
) ([]*models.ProjectResponse, int32, error) {
	m.recordCall("ListProjects", orgName, limit, offset)
	return []*models.ProjectResponse{
		{Name: "test-project", CreatedAt: time.Time{}},
	}, 1, nil
}

func (m *MockToolsetHandler) CreateProject(
	ctx context.Context, orgName string, payload spec.CreateProjectRequest,
) (*models.ProjectResponse, error) {
	m.recordCall("CreateProject", orgName, payload)
	return &models.ProjectResponse{Name: payload.Name, CreatedAt: time.Time{}}, nil
}

// Agent Toolset Handler

func (m *MockToolsetHandler) ListAgents(
	ctx context.Context, orgName string, projName string, limit int32, offset int32,
) ([]*models.AgentResponse, int32, error) {
	m.recordCall("ListAgents", orgName, projName, limit, offset)
	return []*models.AgentResponse{
		{Name: "test-agent"},
	}, 1, nil
}

func (m *MockToolsetHandler) GenerateToken(
	ctx context.Context, orgName string, projectName string, agentName string,
	environment string, expiresIn string,
) (*spec.TokenResponse, error) {
	m.recordCall("GenerateToken", orgName, projectName, agentName, environment, expiresIn)
	return &spec.TokenResponse{
		Token:     "test-token",
		ExpiresAt: 0,
	}, nil
}

func (m *MockToolsetHandler) CreateAgent(
	ctx context.Context, orgName string, projectName string, req *spec.CreateAgentRequest,
) error {
	m.recordCall("CreateAgent", orgName, projectName, req)
	return nil
}

func (m *MockToolsetHandler) GetAgent(
	ctx context.Context, orgName string, projectName string, agentName string,
) (*models.AgentResponse, error) {
	m.recordCall("GetAgent", orgName, projectName, agentName)
	return &models.AgentResponse{Name: agentName}, nil
}

// Build Toolset Handler

func (m *MockToolsetHandler) ListAgentBuilds(
	ctx context.Context, orgName string, projectName string, agentName string,
	limit int32, offset int32,
) ([]*models.BuildResponse, int32, error) {
	m.recordCall("ListAgentBuilds", orgName, projectName, agentName, limit, offset)
	return []*models.BuildResponse{
		{Name: "test-build", Status: "BuildSucceeded"},
	}, 1, nil
}

func (m *MockToolsetHandler) GetBuild(
	ctx context.Context, orgName string, projectName string, agentName string, buildName string,
) (*models.BuildDetailsResponse, error) {
	m.recordCall("GetBuild", orgName, projectName, agentName, buildName)
	return &models.BuildDetailsResponse{
		BuildResponse: models.BuildResponse{
			Name:   buildName,
			Status: "BuildSucceeded",
		},
	}, nil
}

func (m *MockToolsetHandler) BuildAgent(
	ctx context.Context, orgName string, projectName string, agentName string, commitId string,
) (*models.BuildResponse, error) {
	m.recordCall("BuildAgent", orgName, projectName, agentName, commitId)
	return &models.BuildResponse{Name: "test-build", Status: "BuildInitiated"}, nil
}

func (m *MockToolsetHandler) GetBuildLogs(
	ctx context.Context, orgName string, projectName string, agentName string, buildName string,
) (*models.LogsResponse, error) {
	m.recordCall("GetBuildLogs", orgName, projectName, agentName, buildName)
	return &models.LogsResponse{
		Logs:       []models.LogEntry{},
		TotalCount: 0,
	}, nil
}

// Deployment Toolset Handler

func (m *MockToolsetHandler) GetAgentDeployments(
	ctx context.Context, orgName string, projectName string, agentName string,
) ([]*models.DeploymentResponse, error) {
	m.recordCall("GetAgentDeployments", orgName, projectName, agentName)
	return []*models.DeploymentResponse{}, nil
}

func (m *MockToolsetHandler) DeployAgent(
	ctx context.Context, orgName string, projectName string, agentName string,
	req *spec.DeployAgentRequest,
) (string, error) {
	m.recordCall("DeployAgent", orgName, projectName, agentName, req)
	return testEnvName, nil
}

func (m *MockToolsetHandler) UpdateDeploymentState(
	ctx context.Context, orgName string, projectName string, agentName string,
	environment string, state string,
) error {
	m.recordCall("UpdateDeploymentState", orgName, projectName, agentName, environment, state)
	return nil
}
