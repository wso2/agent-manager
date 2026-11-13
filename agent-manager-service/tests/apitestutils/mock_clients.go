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
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// DefaultEnvironmentUUID is the id the default mock reports for the default
// environment; a test that creates an API agent seeds an ingress gateway under it.
const DefaultEnvironmentUUID = "6f1c2f64-3f7e-4a52-9c1d-0b7a8e2d4c11"

// CreateMockPlatformRepository creates a platform repository mock with default
// behavior for testing.
func CreateMockPlatformRepository() *repomocks.PlatformRepositoryMock {
	defaultEnv := &models.EnvironmentResponse{UUID: DefaultEnvironmentUUID, Name: models.DefaultEnvironmentName, DisplayName: "Default"}
	return &repomocks.PlatformRepositoryMock{
		ListOrganizationsFunc: func(_ context.Context) ([]*models.OrganizationResponse, error) {
			return []*models.OrganizationResponse{{Name: "default", Handle: "default"}}, nil
		},
		GetOrganizationFunc: func(_ context.Context, ouID string) (*models.OrganizationResponse, error) {
			if ouID == "nonexistent-org" {
				return nil, utils.ErrOrganizationNotFound
			}
			return &models.OrganizationResponse{Name: ouID, DisplayName: ouID, Handle: ouID, CreatedAt: time.Now()}, nil
		},
		GetProjectFunc: func(_ context.Context, ouID, projectName string) (*models.ProjectResponse, error) {
			if strings.Contains(projectName, "nonexistent-proj") {
				return nil, utils.ErrProjectNotFound
			}
			return &models.ProjectResponse{UUID: uuid.NewString(), Name: projectName, DisplayName: projectName, OrgName: ouID, CreatedAt: time.Now()}, nil
		},
		CreateProjectFunc: func(_ context.Context, ouID string, in repositories.CreateProjectInput) (*models.ProjectResponse, error) {
			return &models.ProjectResponse{UUID: uuid.NewString(), Name: in.Name, DisplayName: in.DisplayName, OrgName: ouID, CreatedAt: time.Now()}, nil
		},
		PatchProjectFunc:  func(context.Context, string, string, *string, *string) error { return nil },
		DeleteProjectFunc: func(context.Context, string, string) error { return nil },
		ListProjectsFunc: func(context.Context, string) ([]*models.ProjectResponse, error) {
			return []*models.ProjectResponse{}, nil
		},
		CreateAgentFunc: func(_ context.Context, _, projectName string, in repositories.CreateAgentInput) (*models.AgentResponse, error) {
			return &models.AgentResponse{
				UUID: uuid.NewString(), Name: in.Name, DisplayName: in.DisplayName, ProjectName: projectName,
				Provisioning: models.Provisioning{Type: in.ProvisioningType},
				Type:         models.AgentType{Type: string(utils.AgentTypeForProvisioning(in.ProvisioningType))},
				Labels:       in.Labels,
			}, nil
		},
		GetAgentFunc: func(_ context.Context, _, projectName, agentName string) (*models.AgentResponse, error) {
			if strings.Contains(agentName, "nonexistent-agent") {
				return nil, utils.ErrAgentNotFound
			}
			return &models.AgentResponse{
				UUID: "agent-uid-123", Name: agentName, ProjectName: projectName,
				Provisioning: models.Provisioning{Type: "external"},
			}, nil
		},
		UpdateAgentBasicInfoFunc: func(context.Context, string, string, string, string, string, *map[string]string) error { return nil },
		DeleteAgentFunc:          func(context.Context, string, string, string) error { return nil },
		ListAgentsFunc: func(context.Context, string, string) ([]*models.AgentResponse, error) {
			return []*models.AgentResponse{}, nil
		},
		CountProjectAgentsFunc: func(context.Context, string, string) (int, error) { return 0, nil },
		AgentExistsFunc:        func(context.Context, string, string, string) (bool, error) { return false, nil },
		GetEnvironmentFunc: func(context.Context, string, string) (*models.EnvironmentResponse, error) {
			return defaultEnv, nil
		},
		ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{defaultEnv}, nil
		},
	}
}

// CreateMockSecretManagementClient creates a mock SecretManagementClient with default behavior for testing.
func CreateMockSecretManagementClient() *clientmocks.SecretManagementClientMock {
	return &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(ctx context.Context, location secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			return location.KVPath()
		},
		GetSecretFunc: func(ctx context.Context, kvPath string) (*secretmanagersvc.SecretInfo, error) {
			return nil, secretmanagersvc.ErrSecretNotFound
		},
		DeleteSecretFunc: func(ctx context.Context, location secretmanagersvc.SecretLocation, secretRefName string) error {
			return nil
		},
		PatchSecretFunc: func(ctx context.Context, location secretmanagersvc.SecretLocation, data map[string]string, keysToDelete []string) (string, error) {
			return "", nil
		},
	}
}
