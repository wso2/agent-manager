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

package apitestutils

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/clientmocks"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// CreateMockOpenChoreoClient creates a mock OpenChoreo client with default behavior for testing
func CreateMockOpenChoreoClient() *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(ctx context.Context, orgName string) (*models.OrganizationResponse, error) {
			if orgName == "nonexistent-org" {
				return nil, utils.ErrOrganizationNotFound
			}
			return &models.OrganizationResponse{
				Name:        orgName,
				DisplayName: orgName,
				CreatedAt:   time.Now(),
				Status:      "ACTIVE",
			}, nil
		},
		GetProjectFunc: func(ctx context.Context, namespaceName string, projectName string) (*models.ProjectResponse, error) {
			if strings.Contains(projectName, "nonexistent-proj") {
				return nil, utils.ErrProjectNotFound
			}
			return &models.ProjectResponse{
				Name:               projectName,
				DisplayName:        projectName,
				OrgName:            namespaceName,
				DeploymentPipeline: "test-pipeline",
				CreatedAt:          time.Now(),
			}, nil
		},
		ComponentExistsFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, verifyProject bool) (bool, error) {
			return false, nil
		},
		CreateComponentFunc: func(ctx context.Context, namespaceName string, projectName string, req client.CreateComponentRequest) error {
			return nil
		},
		AttachTraitFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, traitType client.TraitType, agentApiKey ...string) error {
			return nil
		},
		UpdateComponentEnvironmentVariablesFunc: func(ctx context.Context, namespaceName, projectName, componentName string, envVars []client.EnvVar) error {
			return nil
		},
		TriggerBuildFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, commitID string) (*models.BuildResponse, error) {
			return &models.BuildResponse{
				UUID:        uuid.New().String(),
				Name:        fmt.Sprintf("%s-build-1", componentName),
				AgentName:   componentName,
				ProjectName: projectName,
				Status:      "BuildInitiated",
				StartedAt:   time.Now(),
				BuildParameters: models.BuildParameters{
					CommitID: commitID,
				},
			}, nil
		},
		GetProjectDeploymentPipelineFunc: func(ctx context.Context, namespaceName string, projectName string) (*models.DeploymentPipelineResponse, error) {
			return &models.DeploymentPipelineResponse{
				Name:        "test-pipeline",
				DisplayName: "test-pipeline",
				Description: "Test deployment pipeline",
				OrgName:     namespaceName,
				CreatedAt:   time.Now(),
				PromotionPaths: []models.PromotionPath{
					{
						SourceEnvironmentRef: "Development",
					},
				},
			}, nil
		},
		GetComponentFunc: func(ctx context.Context, namespaceName, projectName, componentName string) (*models.AgentResponse, error) {
			if strings.Contains(componentName, "nonexistent-agent") {
				return nil, utils.ErrAgentNotFound
			}
			return &models.AgentResponse{
				UUID:        "component-uid-123",
				Name:        componentName,
				ProjectName: projectName,
				Provisioning: models.Provisioning{
					Type: "internal",
				},
			}, nil
		},
		GetEnvironmentFunc: func(ctx context.Context, namespaceName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{
				UUID: "environment-uid-123",
				Name: environmentName,
			}, nil
		},
		DeleteComponentFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string) error {
			return nil
		},
		ListComponentsFunc: func(ctx context.Context, namespaceName string, projectName string) ([]*models.AgentResponse, error) {
			return []*models.AgentResponse{}, nil
		},
		DeleteProjectFunc: func(ctx context.Context, namespaceName string, projectName string) error {
			return nil
		},
		ListDeploymentPipelinesFunc: func(ctx context.Context, namespaceName string) ([]*models.DeploymentPipelineResponse, error) {
			return []*models.DeploymentPipelineResponse{
				{
					Name:        "default",
					DisplayName: "Default Pipeline",
					OrgName:     namespaceName,
				},
			}, nil
		},
		CreateProjectFunc: func(ctx context.Context, namespaceName string, req client.CreateProjectRequest) error {
			return nil
		},
		DeployFunc: func(ctx context.Context, namespaceName string, projectName string, componentName string, req client.DeployRequest) error {
			return nil
		},
	}
}
