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

package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	observabilitysvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/observabilitysvc"
	clients "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

type AgentManagerService interface {
	ListAgents(ctx context.Context, userIdpId uuid.UUID, orgName string, projName string, limit int32, offset int32) ([]*models.AgentResponse, int32, error)
	CreateAgent(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, req *spec.CreateAgentRequest) error
	BuildAgent(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, commitId string) (*models.BuildResponse, error)
	DeleteAgent(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string) error
	DeployAgent(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, req *spec.DeployAgentRequest) (string, error)
	GetAgent(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string) (*models.AgentResponse, error)
	ListAgentBuilds(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error)
	GetBuild(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, buildName string) (*models.BuildDetailsResponse, error)
	GetAgentDeployments(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string) ([]*models.DeploymentResponse, error)
	GetAgentEndpoints(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, environmentName string) (map[string]models.EndpointsResponse, error)
	GetAgentConfigurations(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, environment string) ([]models.EnvVars, error)
	GetBuildLogs(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, buildName string) (*models.BuildLogsResponse, error)
	GenerateName(ctx context.Context, userIdpId uuid.UUID, orgName string, payload spec.ResourceNameRequest) (string, error)
}

type agentManagerService struct {
	OrganizationRepository  repositories.OrganizationRepository
	ProjectRepository       repositories.ProjectRepository
	AgentRepository         repositories.AgentRepository
	InternalAgentRepository repositories.InternalAgentRepository
	OpenChoreoSvcClient     clients.OpenChoreoSvcClient
	ObservabilitySvcClient  observabilitysvc.ObservabilitySvcClient
	logger                  *slog.Logger
}

func NewAgentManagerService(
	orgRepo repositories.OrganizationRepository,
	projRepo repositories.ProjectRepository,
	agentRepo repositories.AgentRepository,
	internalAgentRepo repositories.InternalAgentRepository,
	openChoreoSvcClient clients.OpenChoreoSvcClient,
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient,
	logger *slog.Logger,
) AgentManagerService {
	return &agentManagerService{
		OrganizationRepository:  orgRepo,
		ProjectRepository:       projRepo,
		AgentRepository:         agentRepo,
		InternalAgentRepository: internalAgentRepo,
		OpenChoreoSvcClient:     openChoreoSvcClient,
		ObservabilitySvcClient:  observabilitySvcClient,
		logger:                  logger,
	}
}

func (s *agentManagerService) GetAgent(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string) (*models.AgentResponse, error) {
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	s.logger.Info("Fetching agent", "agentName", agentName, "orgName", org.ID, "projectName", project.ID)
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to fetch agent: %w", err)
	}
	// If the agent is external, return the agent record directly
	if agent.ProvisioningType == string(utils.ExternalAgent) {
		s.logger.Info("Fetched external agent successfully", "agentName", agent.Name, "orgName", orgName, "projectName", projectName)
		return s.convertExternalAgentToAgentResponse(agent, project.Name), nil
	}
	// If the agent is managed by Open Choreo, fetch the agent component from OpenChoreo
	ocAgentComponent, err := s.OpenChoreoSvcClient.GetAgentComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch agent from oc: %w", err)
	}
	s.logger.Info("Fetched agent successfully", "agentName", ocAgentComponent.Name, "orgName", orgName, "projectName", projectName)
	return s.convertManagedAgentToAgentResponse(ocAgentComponent, agent), nil
}

func (s *agentManagerService) ListAgents(ctx context.Context, userIdpId uuid.UUID, orgName string, projName string, limit int32, offset int32) ([]*models.AgentResponse, int32, error) {
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, 0, utils.ErrOrganizationNotFound
		}
		return nil, 0, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, 0, utils.ErrProjectNotFound
		}
		return nil, 0, fmt.Errorf("failed to find project %s: %w", projName, err)
	}
	// Fetch all agents from the database
	agents, err := s.AgentRepository.ListAgents(ctx, org.ID, project.ID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list external agents: %w", err)
	}
	var allAgents []*models.AgentResponse
	for _, agent := range agents {
		allAgents = append(allAgents, s.convertToAgentListItem(agent, project.Name))
	}

	// Calculate total count
	total := int32(len(allAgents))

	// Apply pagination
	var paginatedAgents []*models.AgentResponse
	if offset >= total {
		// If offset is beyond available data, return empty slice
		paginatedAgents = []*models.AgentResponse{}
	} else {
		endIndex := offset + limit
		if endIndex > total {
			endIndex = total
		}
		paginatedAgents = allAgents[offset:endIndex]
	}
	s.logger.Info("Listed agents successfully", "orgName", orgName, "projName", projName, "totalAgents", total, "returnedAgents", len(paginatedAgents))
	return paginatedAgents, total, nil
}

func (s *agentManagerService) CreateAgent(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, req *spec.CreateAgentRequest) error {
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return utils.ErrOrganizationNotFound
		}
		return fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	// Validates the project name by checking its existence
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return utils.ErrProjectNotFound
		}
		return fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	// Check if agent already exists
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, req.Name)
	if err != nil && !db.IsRecordNotFoundError(err) {
		return fmt.Errorf("failed to check existing agents: %w", err)
	}
	if agent != nil {
		return utils.ErrAgentAlreadyExists
	}
	// Save agent record in database first
	err = s.saveAgentRecord(ctx, org.ID, project.ID, req)
	if err != nil {
		return err
	}
	if req.Provisioning.Type == string(utils.InternalAgent) {
		err = s.createOpenChoreoAgentComponent(ctx, orgName, projectName, req)
		if err != nil {
			// OpenChoreo creation failed, rollback database record
			if deleteErr := s.deleteAgentRecord(ctx, org.ID, project.ID, req.Name, false); deleteErr != nil {
				s.logger.Error("Critical: Agent exists in database but not in OpenChoreo, manual cleanup required",
					"agentName", req.Name, "orgName", orgName, "projectName", projectName, "error", deleteErr)
			}
			return err
		}
	}

	return nil
}

func (s *agentManagerService) GenerateName(ctx context.Context, userIdpId uuid.UUID, orgName string, payload spec.ResourceNameRequest) (string, error) {
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return "", utils.ErrOrganizationNotFound
		}
		return "", fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	// Generate candidate name from display name
	candidateName := utils.GenerateCandidateName(payload.DisplayName)

	if payload.ResourceType == string(utils.ResourceTypeAgent) {
		projectName := utils.StrPointerAsStr(payload.ProjectName, "")
		// Validates the project name by checking its existence
		project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
		if err != nil {
			if db.IsRecordNotFoundError(err) {
				return "", utils.ErrProjectNotFound
			}
			return "", fmt.Errorf("failed to find project %s: %w", projectName, err)
		}

		// Check if candidate name is available
		_, err = s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, candidateName)
		if err != nil && db.IsRecordNotFoundError(err) {
			// Name is available, return it
			return candidateName, nil
		}
		if err != nil {
			return "", fmt.Errorf("failed to check agent name availability: %w", err)
		}

		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueAgentName(ctx, org.ID, project.ID, candidateName)
		if err != nil {
			return "", fmt.Errorf("failed to generate unique agent name: %w", err)
		}
		return uniqueName, nil
	}
	if payload.ResourceType == string(utils.ResourceTypeProject) {
		// Check if candidate name is available
		_, err = s.ProjectRepository.GetProjectByName(ctx, org.ID, candidateName)
		if err != nil && db.IsRecordNotFoundError(err) {
			// Name is available, return it
			return candidateName, nil
		}
		if err != nil {
			return "", fmt.Errorf("failed to check project name availability: %w", err)
		}
		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueProjectName(ctx, org.ID, candidateName)
		if err != nil {
			return "", fmt.Errorf("failed to generate unique project name: %w", err)
		}
		return uniqueName, nil
	}
	return "", errors.New("invalid resource type for name generation")
}

// generateUniqueProjectName creates a unique name by appending a random suffix
func (s *agentManagerService) generateUniqueProjectName(ctx context.Context, orgId uuid.UUID, baseName string) (string, error) {
	// Create a name availability checker function that uses the project repository
	nameChecker := func(name string) (bool, error) {
		_, err := s.ProjectRepository.GetProjectByName(ctx, orgId, name)
		if err != nil && db.IsRecordNotFoundError(err) {
			// Name is available
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("failed to check project name availability: %w", err)
		}
		// Name is taken
		return false, nil
	}

	// Use the common unique name generation logic from utils
	uniqueName, err := utils.GenerateUniqueNameWithSuffix(baseName, nameChecker)
	if err != nil {
		return "", fmt.Errorf("failed to generate unique project name: %w", err)
	}

	return uniqueName, nil
}

// generateUniqueAgentName creates a unique name by appending a random suffix
func (s *agentManagerService) generateUniqueAgentName(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, baseName string) (string, error) {
	// Create a name availability checker function that uses the agent repository
	nameChecker := func(name string) (bool, error) {
		_, err := s.AgentRepository.GetAgentByName(ctx, orgId, projectId, name)
		if err != nil && db.IsRecordNotFoundError(err) {
			// Name is available
			return true, nil
		}
		if err != nil {
			return false, fmt.Errorf("failed to check agent name availability: %w", err)
		}
		// Name is taken
		return false, nil
	}

	// Use the common unique name generation logic from utils
	uniqueName, err := utils.GenerateUniqueNameWithSuffix(baseName, nameChecker)
	if err != nil {
		return "", fmt.Errorf("failed to generate unique agent name: %w", err)
	}

	return uniqueName, nil
}

func (s *agentManagerService) saveAgentRecord(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, req *spec.CreateAgentRequest) error {
	// Create agent record in the database
	agentId := uuid.New()
	newAgent := &models.Agent{
		ID:               agentId,
		Name:             req.Name,
		ProvisioningType: req.Provisioning.Type,
		DisplayName:      req.DisplayName,
		Description:      utils.StrPointerAsStr(req.Description, ""),
		ProjectId:        projectId,
		OrgID:            orgId,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
	if err := s.AgentRepository.CreateAgent(ctx, newAgent); err != nil {
		return fmt.Errorf("failed to create agent record: %w", err)
	}

	// If agent type is internal, also create internal agent record
	if req.Provisioning.Type == string(utils.InternalAgent) {
		// Build workload spec from request
		workloadSpec := buildWorkloadSpec(req)

		internalAgent := &models.InternalAgent{
			ID:           agentId,
			AgentType:    req.AgentType.Type,
			AgentSubType: req.AgentType.SubType,
			Language:     req.RuntimeConfigs.Language,
			WorkloadSpec: workloadSpec,
		}

		if err := s.InternalAgentRepository.CreateInternalAgent(ctx, internalAgent); err != nil {
			// Rollback the agent record if internal agent creation fails
			if deleteErr := s.deleteAgentRecord(ctx, orgId, projectId, req.Name, false); deleteErr != nil {
				s.logger.Error("Failed to rollback agent record after internal agent creation failure",
					"agentName", req.Name, "error", deleteErr)
			}
			return fmt.Errorf("failed to create internal agent record: %w", err)
		}
	}

	return nil
}

// createOpenChoreoAgentComponent handles the creation of a managed agent
func (s *agentManagerService) createOpenChoreoAgentComponent(ctx context.Context, orgName, projectName string, req *spec.CreateAgentRequest) error {
	// Create agent component in Open Choreo
	if err := s.OpenChoreoSvcClient.CreateAgentComponent(ctx, orgName, projectName, req); err != nil {
		return fmt.Errorf("failed to create agent component: agentName %s, error: %w", req.Name, err)
	}
	// Trigger build in Open Choreo with the latest commit
	build, err := s.OpenChoreoSvcClient.TriggerBuild(ctx, orgName, projectName, req.Name, "")
	if err != nil {
		// Clean up the component if build trigger fails
		s.logger.Info("Cleaning up component after build trigger failure", "agentName", req.Name)
		if deleteErr := s.OpenChoreoSvcClient.DeleteAgentComponent(ctx, orgName, projectName, req.Name); deleteErr != nil {
			s.logger.Error("Failed to clean up component after build trigger failure", "agentName", req.Name, "deleteError", deleteErr)
		}
		return fmt.Errorf("failed to trigger build: agentName %s, error: %w", req.Name, err)
	}
	s.logger.Info("Agent created successfully", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "buildName", build.Name)
	return nil
}

func (s *agentManagerService) DeleteAgent(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string) error {
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return utils.ErrOrganizationNotFound
		}
		return fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return utils.ErrProjectNotFound
		}
		return fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	// Check if agent exists in the database
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		// DELETE is idempotent
		if db.IsRecordNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("failed to check existing agents: %w", err)
	}
	// Soft deletion
	err = s.deleteAgentRecord(ctx, org.ID, project.ID, agentName, true)
	if err != nil {
		return err
	}
	if agent.ProvisioningType == string(utils.InternalAgent) {
		// Remove open Choreo managed resources
		err = s.deleteManagedAgent(ctx, orgName, projectName, agentName)
		if err != nil {
			return err
		}
	}
	// Delete Agent record from table
	return s.deleteAgentRecord(ctx, org.ID, project.ID, agentName, false)
}

func (s *agentManagerService) deleteManagedAgent(ctx context.Context, orgName, projectName, agentName string) error {
	// Delete agent component in Open Choreo
	if err := s.OpenChoreoSvcClient.DeleteAgentComponent(ctx, orgName, projectName, agentName); err != nil {
		return fmt.Errorf("failed to delete agent component: agentName %s, error: %w", agentName, err)
	}
	s.logger.Info("Managed agent deleted successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	return nil
}

func (s *agentManagerService) deleteAgentRecord(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, agentName string, isSoftDelete bool) error {
	// Delete agent record from the database
	if isSoftDelete {
		if err := s.AgentRepository.SoftDeleteAgentByName(ctx, orgId, projectId, agentName); err != nil {
			return fmt.Errorf("failed to delete agent record: agentName %s, error: %w", agentName, err)
		}
	} else {
		if err := s.AgentRepository.HardDeleteAgentByName(ctx, orgId, projectId, agentName); err != nil {
			return fmt.Errorf("failed to hard delete agent record: agentName %s, error: %w", agentName, err)
		}
	}
	s.logger.Info("Agent record deleted successfully", "agentName", agentName, "orgId", orgId, "projectId", projectId)
	return nil
}

// BuildAgent triggers a build for an agent.
func (s *agentManagerService) BuildAgent(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, commitId string) (*models.BuildResponse, error) {
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to fetch agent: %w", err)
	}
	if agent.ProvisioningType != string(utils.InternalAgent) {
		return nil, fmt.Errorf("build operation is not supported for agent type: '%s'", agent.ProvisioningType)
	}
	// Trigger build in Open Choreo
	build, err := s.OpenChoreoSvcClient.TriggerBuild(ctx, orgName, projectName, agentName, commitId)
	if err != nil {
		if errors.Is(err, utils.ErrAgentNotFound) {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to trigger build: agentName %s, error: %w", agentName, err)
	}
	err = s.AgentRepository.UpdateAgentTimestamp(ctx, org.ID, project.ID, agentName)
	if err != nil {
		s.logger.Error("Failed to update agent timestamp after successfully triggering the build", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
	}
	s.logger.Info("Build triggered successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "buildName", build.Name)
	return build, nil
}

// DeployAgent deploys an agent.
func (s *agentManagerService) DeployAgent(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, req *spec.DeployAgentRequest) (string, error) {
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return "", utils.ErrOrganizationNotFound
		}
		return "", fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return "", utils.ErrProjectNotFound
		}
		return "", fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return "", utils.ErrAgentNotFound
		}
		return "", fmt.Errorf("failed to fetch agent: %w", err)
	}
	if agent.ProvisioningType != string(utils.InternalAgent) {
		return "", fmt.Errorf("deploy operation is not supported for agent type: '%s'", agent.ProvisioningType)
	}

	// Create a new request with the combined environment variables
	deployReq := &spec.DeployAgentRequest{
		ImageId: req.ImageId,
		Env:     req.Env,
	}

	// Deploy agent component in Open Choreo
	if err := s.OpenChoreoSvcClient.DeployAgentComponent(ctx, orgName, projectName, agentName, deployReq); err != nil {
		return "", fmt.Errorf("failed to deploy agent component: agentName %s, error: %w", agentName, err)
	}
	openChoreoProject, err := s.OpenChoreoSvcClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		return "", fmt.Errorf("failed to fetch openchoreo project: %w", err)
	}

	pipelineName := openChoreoProject.DeploymentPipeline
	if pipelineName == "" {
		return "", fmt.Errorf("project has no deployment pipeline configured")
	}
	pipeline, err := s.OpenChoreoSvcClient.GetDeploymentPipeline(ctx, orgName, pipelineName)
	if err != nil {
		return "", fmt.Errorf("failed to fetch deployment pipeline: %w", err)
	}
	err = s.AgentRepository.UpdateAgentTimestamp(ctx, org.ID, project.ID, agentName)
	if err != nil {
		s.logger.Error("Failed to update agent timestamp after successful deployment", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
	}
	lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
	s.logger.Info("Agent deployed successfully to "+lowestEnv, "agentName", agentName, "orgName", orgName, "projectName", projectName)
	return lowestEnv, nil
}

func findLowestEnvironment(promotionPaths []models.PromotionPath) string {
	if len(promotionPaths) == 0 {
		return ""
	}

	// Collect all target environments
	targets := make(map[string]bool)
	for _, path := range promotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}

	// Find a source environment that is not a target
	for _, path := range promotionPaths {
		if !targets[path.SourceEnvironmentRef] {
			return path.SourceEnvironmentRef
		}
	}
	return ""
}

func (s *agentManagerService) GetBuildLogs(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, buildName string) (*models.BuildLogsResponse, error) {
	// Validate organization exists
	valid, err := s.validateOrganization(ctx, userIdpId, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	if !valid {
		return nil, utils.ErrOrganizationNotFound
	}

	// Validates the project name by checking its existence
	_, err = s.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		return nil, err
	}

	// Check if component already exists
	_, err = s.OpenChoreoSvcClient.GetAgentComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentNotFound) {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to check component existence: %w", err)
	}

	// Check if build exists
	build, err := s.OpenChoreoSvcClient.GetComponentWorkflow(ctx, orgName, projectName, agentName, buildName)
	if err != nil {
		if errors.Is(err, utils.ErrBuildNotFound) {
			return nil, utils.ErrBuildNotFound
		}
		return nil, fmt.Errorf("failed to get build %s for agent %s: %w", buildName, agentName, err)
	}

	// Fetch the build logs from Observability service
	buildLogs, err := s.ObservabilitySvcClient.GetBuildLogs(ctx, build.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch build logs: %w", err)
	}
	s.logger.Info("Fetched build logs successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "buildName", buildName, "logCount", len(buildLogs.Logs))
	return buildLogs, nil
}

func (s *agentManagerService) validateOrganization(ctx context.Context, userIdpID uuid.UUID, orgName string) (bool, error) {
	orgs, err := s.OrganizationRepository.GetOrganizationsByUserIdpID(ctx, userIdpID)
	if err != nil {
		return false, err
	}
	if len(orgs) == 0 {
		s.logger.Warn("No organizations found for user", "userIdpID", userIdpID)
		return false, nil
	}
	for _, org := range orgs {
		if org.OpenChoreoOrgName == orgName {
			return true, nil
		}
	}
	s.logger.Warn("No matching organization found for user", "userIdpID", userIdpID, "orgName", orgName)
	return false, nil
}

func (s *agentManagerService) ListAgentBuilds(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error) {
	// Validate organization exists
	valid, err := s.validateOrganization(ctx, userIdpId, orgName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	if !valid {
		return nil, 0, utils.ErrOrganizationNotFound
	}

	// Validates the project name by checking its existence
	_, err = s.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		return nil, 0, err
	}

	// Check if component already exists
	_, err = s.OpenChoreoSvcClient.GetAgentComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentNotFound) {
			return nil, 0, utils.ErrAgentNotFound
		}
		return nil, 0, fmt.Errorf("failed to check component existence: %w", err)
	}

	// Fetch all builds from Open Choreo first
	allBuilds, err := s.OpenChoreoSvcClient.ListComponentWorkflows(ctx, orgName, projectName, agentName)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list builds for agent %s: %w", agentName, err)
	}

	// Calculate total count
	total := int32(len(allBuilds))

	// Apply pagination
	var paginatedBuilds []*models.BuildResponse
	if offset >= total {
		// If offset is beyond available data, return empty slice
		paginatedBuilds = []*models.BuildResponse{}
	} else {
		endIndex := offset + limit
		if endIndex > total {
			endIndex = total
		}
		paginatedBuilds = allBuilds[offset:endIndex]
	}

	s.logger.Info("Listed builds successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "totalBuilds", total, "returnedBuilds", len(paginatedBuilds))
	return paginatedBuilds, total, nil
}

func (s *agentManagerService) GetBuild(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, buildName string) (*models.BuildDetailsResponse, error) {
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to fetch agent: %w", err)
	}
	if agent.ProvisioningType != string(utils.InternalAgent) {
		return nil, fmt.Errorf("build operation is not supported for agent type: '%s'", agent.ProvisioningType)
	}
	// Fetch the build from Open Choreo
	build, err := s.OpenChoreoSvcClient.GetComponentWorkflow(ctx, orgName, projectName, agentName, buildName)
	if err != nil {
		if errors.Is(err, utils.ErrBuildNotFound) {
			return nil, utils.ErrBuildNotFound
		}
		return nil, fmt.Errorf("failed to get build %s for agent %s: %w", buildName, agentName, err)
	}

	s.logger.Info("Fetched build successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "buildName", build.Name)
	return build, nil
}

func (s *agentManagerService) GetAgentDeployments(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string) ([]*models.DeploymentResponse, error) {
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to fetch agent: %w", err)
	}
	if agent.ProvisioningType != string(utils.InternalAgent) {
		return nil, fmt.Errorf("deployment operation is not supported for agent type: '%s'", agent.ProvisioningType)
	}
	// Fetch OC project details
	openChoreoProject, err := s.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		return nil, err
	}
	pipelineName := openChoreoProject.DeploymentPipeline
	deployments, err := s.OpenChoreoSvcClient.GetAgentDeployments(ctx, orgName, pipelineName, projectName, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployments for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched deployments successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	return deployments, nil
}

func (s *agentManagerService) GetAgentEndpoints(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, environmentName string) (map[string]models.EndpointsResponse, error) {
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to fetch agent: %w", err)
	}
	if agent.ProvisioningType != string(utils.InternalAgent) {
		return nil, fmt.Errorf("endpoints are not supported for agent type: '%s'", agent.ProvisioningType)
	}
	// Check if environment exists
	_, err = s.OpenChoreoSvcClient.GetEnvironment(ctx, orgName, environmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
	}

	endpoints, err := s.OpenChoreoSvcClient.GetAgentEndpoints(ctx, orgName, projectName, agentName, environmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoints for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched endpoints successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	return endpoints, nil
}

func (s *agentManagerService) GetAgentConfigurations(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, environment string) ([]models.EnvVars, error) {
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to fetch agent: %w", err)
	}
	if agent.ProvisioningType != string(utils.InternalAgent) {
		return nil, fmt.Errorf("configuration operation is not supported for agent type: '%s'", agent.ProvisioningType)
	}
	// Check if environment exists
	_, err = s.OpenChoreoSvcClient.GetEnvironment(ctx, orgName, environment)
	if err != nil {
		return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
	}

	configurations, err := s.OpenChoreoSvcClient.GetAgentConfigurations(ctx, orgName, projectName, agentName, environment)
	if err != nil {
		return nil, fmt.Errorf("failed to get configurations for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched configurations successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	return configurations, nil
}

func (s *agentManagerService) convertToAgentListItem(agent *models.Agent, projName string) *models.AgentResponse {
	response := &models.AgentResponse{
		Name:        agent.Name,
		DisplayName: agent.DisplayName,
		Description: agent.Description,
		ProjectName: projName,
		Provisioning: models.Provisioning{
			Type: agent.ProvisioningType,
		},
		CreatedAt: agent.CreatedAt,
	}
	if agent.AgentDetails != nil {
		response.AgentType = models.AgentType{
			Type:    agent.AgentDetails.AgentType,
			SubType: agent.AgentDetails.AgentSubType,
		}
		response.Language = agent.AgentDetails.Language
	}
	return response
}

// convertToExternalAgentResponse converts a database Agent model to AgentResponse for external agents
func (s *agentManagerService) convertExternalAgentToAgentResponse(agent *models.Agent, projName string) *models.AgentResponse {
	return &models.AgentResponse{
		Name:        agent.Name,
		DisplayName: agent.DisplayName,
		Description: agent.Description,
		ProjectName: projName,
		Provisioning: models.Provisioning{
			Type: string(utils.ExternalAgent),
		},
		CreatedAt: agent.CreatedAt,
	}
}

// convertToManagedAgentResponse converts an OpenChoreo AgentComponent to AgentResponse for managed agents
func (s *agentManagerService) convertManagedAgentToAgentResponse(ocAgentComponent *clients.AgentComponent, agent *models.Agent) *models.AgentResponse {
	return &models.AgentResponse{
		Name:        ocAgentComponent.Name,
		DisplayName: ocAgentComponent.DisplayName,
		Description: ocAgentComponent.Description,
		ProjectName: ocAgentComponent.ProjectName,
		Provisioning: models.Provisioning{
			Type: string(utils.InternalAgent),
			Repository: models.Repository{
				Url:     ocAgentComponent.Repository.RepoURL,
				Branch:  ocAgentComponent.Repository.Branch,
				AppPath: ocAgentComponent.Repository.AppPath,
			},
		},
		AgentType: models.AgentType{
			Type:    agent.AgentDetails.AgentType,
			SubType: agent.AgentDetails.AgentSubType,
		},
		Language:  agent.AgentDetails.Language,
		CreatedAt: ocAgentComponent.CreatedAt,
	}
}


// buildWorkloadSpec constructs the workload specification from the create agent request
func buildWorkloadSpec(req *spec.CreateAgentRequest) map[string]interface{} {
	workloadSpec := make(map[string]interface{})

	workloadSpec["envVars"] = req.RuntimeConfigs.Env

	if req.AgentType.Type == string(utils.AgentTypeAPI) && req.AgentType.SubType == string(utils.AgentSubTypeChatAPI) {
		// Read OpenAPI schema from file
		schemaContent, err := utils.ReadChatAPISchema()
		if err != nil {
			// Log error but continue - workflow will handle missing schema
			fmt.Printf("Warning: failed to read chat API schema: %v\n", err)
		}

		endpoints := []map[string]interface{}{
			{
				"name":          fmt.Sprintf("%s-endpoint", req.Name),
				"port":          config.GetConfig().DefaultChatAPI.DefaultHTTPPort,
				"type":          string(utils.InputInterfaceTypeHTTP),
				"schemaContent": schemaContent,
			},
		}
		workloadSpec["endpoints"] = endpoints
	}

	// Handle Custom API - use schema path from request
	if req.AgentType.Type == string(utils.AgentTypeAPI) && req.AgentType.SubType == string(utils.AgentSubTypeCustomAPI) {
		endpoints := []map[string]interface{}{
			{
				"name":       fmt.Sprintf("%s-endpoint", req.Name),
				"port":       req.InputInterface.Port,
				"type":       string(req.InputInterface.Type),
				"schemaPath": req.InputInterface.Schema.Path,
			},
		}
		workloadSpec["endpoints"] = endpoints
	}

	return workloadSpec
}
