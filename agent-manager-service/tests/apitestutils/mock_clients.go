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
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// CreateMockOpenChoreoClient creates a mock OpenChoreo client with default behavior for testing
func CreateMockOpenChoreoClient() *clientmocks.OpenChoreoSvcClientMock {
	return &clientmocks.OpenChoreoSvcClientMock{
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
		GetProjectFunc: func(ctx context.Context, projectName string, orgName string) (*models.ProjectResponse, error) {
			if strings.Contains(projectName, "nonexistent-proj") {
				return nil, utils.ErrProjectNotFound
			}
			return &models.ProjectResponse{
				Name:               projectName,
				DisplayName:        projectName,
				OrgName:            orgName,
				DeploymentPipeline: "test-pipeline",
				CreatedAt:          time.Now(),
			}, nil
		},
		IsAgentComponentExistsFunc: func(ctx context.Context, orgName string, projName string, agentName string, verifyProject bool) (bool, error) {
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
				CommitID:    commitId,
				Status:      "BuildInitiated",
				StartedAt:   time.Now(),
			}, nil
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
		GetAgentComponentFunc: func(ctx context.Context, orgName, projectName, agentName string) (*openchoreosvc.AgentComponent, error) {
			if strings.Contains(agentName, "nonexistent-agent") {
				return nil, utils.ErrAgentNotFound
			}
			return &openchoreosvc.AgentComponent{
				UUID:        "component-uid-123",
				Name:        agentName,
				ProjectName: projectName,
			}, nil
		},
		GetEnvironmentFunc: func(ctx context.Context, orgName, environmentName string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{
				UUID: "environment-uid-123",
			}, nil
		},
		DeleteAgentComponentFunc: func(ctx context.Context, orgName string, projName string, agentName string) error {
			return nil
		},
		ListAgentComponentsFunc: func(ctx context.Context, orgName string, projectName string) ([]*openchoreosvc.AgentComponent, error) {
			return []*openchoreosvc.AgentComponent{}, nil
		},
		DeleteProjectFunc: func(ctx context.Context, orgName string, projectName string) error {
			return nil
		},
		GetDeploymentPipelinesForOrganizationFunc: func(ctx context.Context, orgName string) ([]*models.DeploymentPipelineResponse, error) {
			return []*models.DeploymentPipelineResponse{
				{
					Name:        "default",
					DisplayName: "Default Pipeline",
					OrgName:     orgName,
				},
			}, nil
		},
		CreateProjectFunc: func(ctx context.Context, orgName, projectName, deploymentPipeline, displayName, description string) error {
			return nil
		},
		DeployAgentComponentFunc: func(ctx context.Context, orgName string, projName string, componentName string, req *spec.DeployAgentRequest) error {
			return nil
		},
	}
}
