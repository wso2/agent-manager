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
	"gorm.io/gorm"

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
	s.logger.Info("Getting agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "userIdpId", userIdpId)
	// Validate organization exists
	_, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Error("Organization not found", "orgName", orgName, "userIdpId", userIdpId)
			return nil, utils.ErrOrganizationNotFound
		}
		s.logger.Error("Failed to find organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	ocAgentComponent, err := s.OpenChoreoSvcClient.GetAgentComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to fetch agent from oc: %w", err)
	}
	if ocAgentComponent.Provisioning.Type == string(utils.ExternalAgent) {
		s.logger.Info("Fetched external agent successfully", "agentName", ocAgentComponent.Name, "orgName", orgName, "projectName", projectName, "provisioningType", ocAgentComponent.Provisioning.Type)
		return s.convertExternalAgentToAgentResponse(ocAgentComponent), nil
	}
	s.logger.Info("Fetched agent successfully from oc", "agentName", ocAgentComponent.Name, "orgName", orgName, "projectName", projectName, "provisioningType", string(utils.InternalAgent))
	return s.convertManagedAgentToAgentResponse(ocAgentComponent), nil
}

func (s *agentManagerService) ListAgents(ctx context.Context, userIdpId uuid.UUID, orgName string, projName string, limit int32, offset int32) ([]*models.AgentResponse, int32, error) {
	s.logger.Info("Listing agents", "orgName", orgName, "projectName", projName, "limit", limit, "offset", offset, "userIdpId", userIdpId)
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil, 0, utils.ErrOrganizationNotFound
		}
		return nil, 0, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Error("Project not found", "projectName", projName, "orgId", org.ID)
			return nil, 0, utils.ErrProjectNotFound
		}
		s.logger.Error("Failed to find project", "projectName", projName, "orgId", org.ID, "error", err)
		return nil, 0, fmt.Errorf("failed to find project %s: %w", projName, err)
	}
	// Fetch all agents from the database
	agents, err := s.OpenChoreoSvcClient.ListAgentComponents(ctx, orgName, projName)
	if err != nil {
		s.logger.Error("Failed to list agents from repository", "orgId", org.ID, "projectId", project.ID, "error", err)
		return nil, 0, fmt.Errorf("failed to list external agents: %w", err)
	}
	var allAgents []*models.AgentResponse
	for _, agent := range agents {
		allAgents = append(allAgents, s.convertToAgentListItem(agent))
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
	s.logger.Info("Creating agent", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "provisioningType", req.Provisioning.Type, "userIdpId", userIdpId)
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		if db.IsRecordNotFoundError(err) {
			return utils.ErrOrganizationNotFound
		}
		return fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	// Validate project exists in OpenChoreo
	_, err = s.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgId", org.ID, "error", err)
		return err
	}
	// Check if agent already exists
	agent, err := s.OpenChoreoSvcClient.GetAgentComponent(ctx, orgName, projectName, req.Name)
	if err != nil && err != utils.ErrAgentNotFound {
		s.logger.Error("Failed to check existing agents", "agentName", req.Name, "orgId", org.ID, "project", projectName, "error", err)
		return fmt.Errorf("failed to check existing agents: %w", err)
	}
	if agent != nil {
		s.logger.Warn("Agent already exists", "agentName", req.Name, "orgId", org.ID, "project", projectName)
		return utils.ErrAgentAlreadyExists
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgId", org.ID, "error", err)
		if db.IsRecordNotFoundError(err) {
			return utils.ErrProjectNotFound
		}
		return err
	}
	// Save agent record in database first
	err = s.saveAgentRecord(ctx, org.ID, project.ID, req)
	if err != nil {
		s.logger.Error("Failed to save agent record", "agentName", req.Name, "error", err)
		return err
	}
	err = s.createOpenChoreoAgentComponent(ctx, orgName, projectName, req)
	if err != nil {
		s.logger.Error("OpenChoreo creation failed, initiating rollback", "agentName", req.Name, "error", err)
		// OpenChoreo creation failed, rollback database record
		if deleteErr := s.deleteAgentRecord(ctx, org.ID, project.ID, req.Name, false); deleteErr != nil {
			s.logger.Error("Critical: Agent exists in database but not in OpenChoreo, manual cleanup required",
				"agentName", req.Name, "orgName", orgName, "projectName", projectName, "error", deleteErr)
		}
		return err
	}

	s.logger.Info("Agent created successfully", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "provisioningType", req.Provisioning.Type)
	return nil
}

func (s *agentManagerService) GenerateName(ctx context.Context, userIdpId uuid.UUID, orgName string, payload spec.ResourceNameRequest) (string, error) {
	s.logger.Info("Generating resource name", "resourceType", payload.ResourceType, "displayName", payload.DisplayName, "orgName", orgName, "userIdpId", userIdpId)
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Error("Organization not found", "orgName", orgName, "userIdpId", userIdpId)
			return "", utils.ErrOrganizationNotFound
		}
		s.logger.Error("Failed to find organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		return "", fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	// Generate candidate name from display name
	candidateName := utils.GenerateCandidateName(payload.DisplayName)
	s.logger.Debug("Generated candidate name", "candidateName", candidateName, "displayName", payload.DisplayName)

	if payload.ResourceType == string(utils.ResourceTypeAgent) {
		projectName := utils.StrPointerAsStr(payload.ProjectName, "")
		// Validates the project name by checking its existence
		project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
		if err != nil {
			s.logger.Error("Failed to find project", "projectName", projectName, "orgId", org.ID, "error", err)
			if db.IsRecordNotFoundError(err) {
				return "", utils.ErrProjectNotFound
			}
			return "", fmt.Errorf("failed to find project %s: %w", projectName, err)
		}

		// Check if candidate name is available
		_, err = s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, candidateName)
		if err != nil && db.IsRecordNotFoundError(err) {
			// Name is available, return it
			s.logger.Info("Generated unique agent name from display name", "agentName", candidateName, "orgName", orgName, "projectName", projectName)
			return candidateName, nil
		}
		if err != nil {
			s.logger.Error("Failed to check agent name availability", "name", candidateName, "orgId", org.ID, "projectId", project.ID, "error", err)
			return "", fmt.Errorf("failed to check agent name availability: %w", err)
		}

		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueAgentName(ctx, org.ID, project.ID, candidateName)
		if err != nil {
			s.logger.Error("Failed to generate unique agent name", "baseName", candidateName, "orgId", org.ID, "projectId", project.ID, "error", err)
			return "", fmt.Errorf("failed to generate unique agent name: %w", err)
		}
		s.logger.Info("Generated unique agent name", "agentName", uniqueName, "orgName", orgName, "projectName", projectName)
		return uniqueName, nil
	}
	if payload.ResourceType == string(utils.ResourceTypeProject) {
		// Check if candidate name is available
		_, err = s.ProjectRepository.GetProjectByName(ctx, org.ID, candidateName)
		if err != nil && db.IsRecordNotFoundError(err) {
			// Name is available, return it
			s.logger.Info("Generated unique project name", "projectName", candidateName, "orgName", orgName)
			return candidateName, nil
		}
		if err != nil {
			s.logger.Error("Failed to check project name availability", "name", candidateName, "orgId", org.ID, "error", err)
			return "", fmt.Errorf("failed to check project name availability: %w", err)
		}
		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueProjectName(ctx, org.ID, candidateName)
		if err != nil {
			s.logger.Error("Failed to generate unique project name", "baseName", candidateName, "orgId", org.ID, "error", err)
			return "", fmt.Errorf("failed to generate unique project name: %w", err)
		}
		s.logger.Info("Generated unique project name", "projectName", uniqueName, "orgName", orgName)
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
			s.logger.Error("Failed to check project name availability", "name", name, "orgId", orgId, "error", err)
			return false, fmt.Errorf("failed to check project name availability: %w", err)
		}
		// Name is taken
		return false, nil
	}

	// Use the common unique name generation logic from utils
	uniqueName, err := utils.GenerateUniqueNameWithSuffix(baseName, nameChecker)
	if err != nil {
		s.logger.Error("Failed to generate unique project name", "baseName", baseName, "orgId", orgId, "error", err)
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
	agentId := uuid.New()

	// Execute database operations in a transaction
	return db.DB(ctx).Transaction(func(tx *gorm.DB) error {
		txCtx := db.CtxWithTx(ctx, tx)

		// Create agent record in the database
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
		if err := s.AgentRepository.CreateAgent(txCtx, newAgent); err != nil {
			s.logger.Error("Failed to create agent record in database", "agentName", req.Name, "agentId", agentId, "error", err)
			return fmt.Errorf("failed to create agent record: %w", err)
		}

		// If agent type is internal, also create internal agent record
		if req.Provisioning.Type == string(utils.InternalAgent) {
			// Build workload spec from request
			workloadSpec, err := buildWorkloadSpec(req)
			if err != nil {
				s.logger.Error("Failed to build workload spec", "agentName", req.Name, "error", err)
				return fmt.Errorf("failed to build workload spec: %w", err)
			}

			internalAgent := &models.InternalAgent{
				ID:           agentId,
				WorkloadSpec: workloadSpec,
			}

			if err := s.InternalAgentRepository.CreateInternalAgent(txCtx, internalAgent); err != nil {
				s.logger.Error("Failed to create internal agent record", "agentName", req.Name, "agentId", agentId, "error", err)
				return fmt.Errorf("failed to create internal agent record: %w", err)
			}
		}

		return nil
	})
}

// createOpenChoreoAgentComponent handles the creation of a managed agent
func (s *agentManagerService) createOpenChoreoAgentComponent(ctx context.Context, orgName, projectName string, req *spec.CreateAgentRequest) error {
	// Create agent component in Open Choreo
	s.logger.Debug("Creating agent component in OpenChoreo", "agentName", req.Name, "orgName", orgName, "projectName", projectName)
	if err := s.OpenChoreoSvcClient.CreateAgentComponent(ctx, orgName, projectName, req); err != nil {
		s.logger.Error("Failed to create agent component in OpenChoreo", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "error", err)
		return fmt.Errorf("failed to create agent component: agentName %s, error: %w", req.Name, err)
	}
	if req.Provisioning.Type == string(utils.ExternalAgent) {
		s.logger.Info("External agent component created successfully in OpenChoreo", "agentName", req.Name, "orgName", orgName, "projectName", projectName)
		return nil
	}
	// For internal agents, trigger build after creation
	s.logger.Debug("Agent component created, triggering build", "agentName", req.Name, "orgName", orgName, "projectName", projectName)
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
	s.logger.Info("Agent component created and build triggered successfully", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "buildName", build.Name)
	return nil
}

func (s *agentManagerService) DeleteAgent(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string) error {
	s.logger.Info("Deleting agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "userIdpId", userIdpId)
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		if db.IsRecordNotFoundError(err) {
			return utils.ErrOrganizationNotFound
		}
		return fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgId", org.ID, "error", err)
		if db.IsRecordNotFoundError(err) {
			return utils.ErrProjectNotFound
		}
		return fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	// Check if agent exists in the database
	_, err = s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		// DELETE is idempotent
		s.logger.Error("Failed to check existing agents", "agentName", agentName, "orgId", org.ID, "projectId", project.ID, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("failed to check existing agents: %w", err)
	}
	err = s.handleAgentDeletion(ctx, org.ID, project.ID, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to delete oc agent", "agentName", agentName, "error", err)
		return err
	}
	return nil
}

func (s *agentManagerService) handleAgentDeletion(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, orgName string, projectName string, agentName string) error {
	// Soft delete agent from database
	s.logger.Debug("Handling project deletion", "orgName", orgName, "projectName", projectName)
	if err := s.AgentRepository.SoftDeleteAgentByName(ctx, orgId, projectId, agentName); err != nil {
		s.logger.Error("Failed to soft delete agent from repository", "agentName", agentName, "orgId", orgId, "projectId", projectId, "error", err)
		return fmt.Errorf("failed to delete agent %s from repository: %w", projectName, err)
	}
	// Delete agent from OpenChoreo
	err := s.OpenChoreoSvcClient.DeleteAgentComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		// Delete agent from OpenChoreo failed, rollback database changes
		err := s.AgentRepository.RollbackSoftDeleteAgent(ctx, orgId, projectId, agentName)
		if err != nil {
			s.logger.Error("Critical: Agent exists in database but not in OpenChoreo, manual cleanup required",
				"projectId", projectId, "projectName", projectName, "orgName", orgName, "error", err)
		}
		return fmt.Errorf("failed to delete agent %s from OpenChoreo and database: %w", agentName, err)
	}
	s.logger.Debug("Agent deleted from OpenChoreo successfully", "orgName", orgName, "agentName", agentName)
	// Delete agent from database
	if err := s.AgentRepository.HardDeleteAgentByName(ctx, orgId, projectId, agentName); err != nil {
		s.logger.Error("Critical: Agent deleted from OpenChoreo but DB deletion failed, retry required",
			"projectId", projectId, "projectName", projectName, "orgName", orgName, "error", err)
		return fmt.Errorf("failed to delete agent %s from repository: %w", agentName, err)
	}
	return nil
}

func (s *agentManagerService) deleteAgentRecord(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, agentName string, isSoftDelete bool) error {
	// Delete agent record from the database
	if isSoftDelete {
		if err := s.AgentRepository.SoftDeleteAgentByName(ctx, orgId, projectId, agentName); err != nil {
			s.logger.Error("Failed to soft delete agent record", "agentName", agentName, "orgId", orgId, "projectId", projectId, "error", err)
			return fmt.Errorf("failed to delete agent record: agentName %s, error: %w", agentName, err)
		}
	} else {
		if err := s.AgentRepository.HardDeleteAgentByName(ctx, orgId, projectId, agentName); err != nil {
			s.logger.Error("Failed to hard delete agent record", "agentName", agentName, "orgId", orgId, "projectId", projectId, "error", err)
			return fmt.Errorf("failed to hard delete agent record: agentName %s, error: %w", agentName, err)
		}
	}
	s.logger.Info("Agent record deleted successfully", "agentName", agentName, "orgId", orgId, "projectId", projectId, "isSoftDelete", isSoftDelete)
	return nil
}

// BuildAgent triggers a build for an agent.
func (s *agentManagerService) BuildAgent(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, commitId string) (*models.BuildResponse, error) {
	s.logger.Info("Building agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "commitId", commitId, "userIdpId", userIdpId)
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgId", org.ID, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from repository", "agentName", agentName, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to fetch agent: %w", err)
	}
	if agent.ProvisioningType != string(utils.InternalAgent) {
		return nil, fmt.Errorf("build operation is not supported for agent type: '%s'", agent.ProvisioningType)
	}
	// Trigger build in Open Choreo
	s.logger.Debug("Triggering build in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "commitId", commitId)
	build, err := s.OpenChoreoSvcClient.TriggerBuild(ctx, orgName, projectName, agentName, commitId)
	if err != nil {
		s.logger.Error("Failed to trigger build in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
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
	s.logger.Info("Deploying agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "imageId", req.ImageId, "userIdpId", userIdpId)
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		if db.IsRecordNotFoundError(err) {
			return "", utils.ErrOrganizationNotFound
		}
		return "", fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgId", org.ID, "error", err)
		if db.IsRecordNotFoundError(err) {
			return "", utils.ErrProjectNotFound
		}
		return "", fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from repository", "agentName", agentName, "error", err)
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
	s.logger.Debug("Deploying agent component in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "imageId", req.ImageId)
	if err := s.OpenChoreoSvcClient.DeployAgentComponent(ctx, orgName, projectName, agentName, deployReq); err != nil {
		s.logger.Error("Failed to deploy agent component in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return "", fmt.Errorf("failed to deploy agent component: agentName %s, error: %w", agentName, err)
	}
	openChoreoProject, err := s.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		s.logger.Error("Failed to fetch OpenChoreo project", "orgName", orgName, "projectName", projectName, "error", err)
		return "", fmt.Errorf("failed to fetch openchoreo project: %w", err)
	}

	pipelineName := openChoreoProject.DeploymentPipeline
	if pipelineName == "" {
		s.logger.Error("Project has no deployment pipeline configured", "orgName", orgName, "projectName", projectName)
		return "", fmt.Errorf("project has no deployment pipeline configured")
	}
	pipeline, err := s.OpenChoreoSvcClient.GetDeploymentPipeline(ctx, orgName, pipelineName)
	if err != nil {
		s.logger.Error("Failed to fetch deployment pipeline", "orgName", orgName, "pipelineName", pipelineName, "error", err)
		return "", fmt.Errorf("failed to fetch deployment pipeline: %w", err)
	}
	err = s.AgentRepository.UpdateAgentTimestamp(ctx, org.ID, project.ID, agentName)
	if err != nil {
		s.logger.Error("Failed to update agent timestamp after successful deployment", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
	}
	lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
	s.logger.Info("Agent deployed successfully to "+lowestEnv, "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", lowestEnv)
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
	s.logger.Info("Getting build logs", "agentName", agentName, "buildName", buildName, "orgName", orgName, "projectName", projectName, "userIdpId", userIdpId)
	// Validate organization exists
	valid, err := s.validateOrganization(ctx, userIdpId, orgName)
	if err != nil {
		s.logger.Error("Failed to validate organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	if !valid {
		return nil, utils.ErrOrganizationNotFound
	}

	// Validates the project name by checking its existence
	_, err = s.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		s.logger.Error("Failed to get OpenChoreo project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}

	// Check if component already exists
	_, err = s.OpenChoreoSvcClient.GetAgentComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentNotFound) {
			s.logger.Warn("Agent component not found in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName)
			return nil, utils.ErrAgentNotFound
		}
		s.logger.Error("Failed to check component existence", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to check component existence: %w", err)
	}

	// Check if build exists
	build, err := s.OpenChoreoSvcClient.GetComponentWorkflow(ctx, orgName, projectName, agentName, buildName)
	if err != nil {
		if errors.Is(err, utils.ErrBuildNotFound) {
			s.logger.Warn("Build not found", "buildName", buildName, "agentName", agentName, "orgName", orgName, "projectName", projectName)
			return nil, utils.ErrBuildNotFound
		}
		s.logger.Error("Failed to get build", "buildName", buildName, "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get build %s for agent %s: %w", buildName, agentName, err)
	}

	// Fetch the build logs from Observability service
	buildLogs, err := s.ObservabilitySvcClient.GetBuildLogs(ctx, build.Name)
	if err != nil {
		s.logger.Error("Failed to fetch build logs from observability service", "buildName", build.Name, "error", err)
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
	return false, nil
}

func (s *agentManagerService) ListAgentBuilds(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error) {
	s.logger.Info("Listing agent builds", "agentName", agentName, "orgName", orgName, "projectName", projectName, "limit", limit, "offset", offset, "userIdpId", userIdpId)
	// Validate organization exists
	valid, err := s.validateOrganization(ctx, userIdpId, orgName)
	if err != nil {
		s.logger.Error("Failed to validate organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		return nil, 0, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	if !valid {
		s.logger.Warn("Organization not found", "orgName", orgName, "userIdpId", userIdpId)
		return nil, 0, utils.ErrOrganizationNotFound
	}

	// Validates the project name by checking its existence
	_, err = s.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		s.logger.Error("Failed to get OpenChoreo project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, 0, err
	}

	// Check if component already exists
	_, err = s.OpenChoreoSvcClient.GetAgentComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentNotFound) {
			s.logger.Warn("Agent component not found in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName)
			return nil, 0, utils.ErrAgentNotFound
		}
		s.logger.Error("Failed to check component existence", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, 0, fmt.Errorf("failed to check component existence: %w", err)
	}

	// Fetch all builds from Open Choreo first
	allBuilds, err := s.OpenChoreoSvcClient.ListComponentWorkflows(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to list builds from OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
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
	s.logger.Info("Getting build details", "agentName", agentName, "buildName", buildName, "orgName", orgName, "projectName", projectName, "userIdpId", userIdpId)
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgId", org.ID, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from repository", "agentName", agentName, "error", err)
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
		s.logger.Error("Failed to get build from OpenChoreo", "buildName", buildName, "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		if errors.Is(err, utils.ErrBuildNotFound) {
			return nil, utils.ErrBuildNotFound
		}
		return nil, fmt.Errorf("failed to get build %s for agent %s: %w", buildName, agentName, err)
	}

	s.logger.Info("Fetched build successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "buildName", build.Name)
	return build, nil
}

func (s *agentManagerService) GetAgentDeployments(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string) ([]*models.DeploymentResponse, error) {
	s.logger.Info("Getting agent deployments", "agentName", agentName, "orgName", orgName, "projectName", projectName, "userIdpId", userIdpId)
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgId", org.ID, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from repository", "agentName", agentName, "error", err)
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
		s.logger.Error("Failed to fetch OpenChoreo project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}
	pipelineName := openChoreoProject.DeploymentPipeline
	deployments, err := s.OpenChoreoSvcClient.GetAgentDeployments(ctx, orgName, pipelineName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to get deployments from OpenChoreo", "agentName", agentName, "pipelineName", pipelineName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get deployments for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched deployments successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "deploymentCount", len(deployments))
	return deployments, nil
}

func (s *agentManagerService) GetAgentEndpoints(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, environmentName string) (map[string]models.EndpointsResponse, error) {
	s.logger.Info("Getting agent endpoints", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environmentName, "userIdpId", userIdpId)
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgName", orgName, "error", err)

		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "projectName", projectName, "orgName", orgName, "error", err)
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
		s.logger.Error("Failed to validate environment", "environment", environmentName, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
	}
	s.logger.Debug("Fetching agent endpoints from OpenChoreo", "agentName", agentName, "environment", environmentName, "orgName", orgName, "projectName", projectName)
	endpoints, err := s.OpenChoreoSvcClient.GetAgentEndpoints(ctx, orgName, projectName, agentName, environmentName)
	if err != nil {
		s.logger.Error("Failed to fetch endpoints", "agentName", agentName, "environment", environmentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get endpoints for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched endpoints successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environmentName, "endpointCount", len(endpoints))
	return endpoints, nil
}

func (s *agentManagerService) GetAgentConfigurations(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string, agentName string, environment string) ([]models.EnvVars, error) {
	s.logger.Info("Getting agent configurations", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment, "userIdpId", userIdpId)
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "userIdpId", userIdpId, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgName", orgName, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	agent, err := s.AgentRepository.GetAgentByName(ctx, org.ID, project.ID, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "projectName", projectName, "orgName", orgName, "error", err)
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to fetch agent: %w", err)
	}
	if agent.ProvisioningType != string(utils.InternalAgent) {
		s.logger.Warn("Configuration operation not supported for agent type", "agentName", agentName, "provisioningType", agent.ProvisioningType, "orgName", orgName, "projectName", projectName)
		return nil, fmt.Errorf("configuration operation is not supported for agent type: '%s'", agent.ProvisioningType)
	}
	// Check if environment exists
	_, err = s.OpenChoreoSvcClient.GetEnvironment(ctx, orgName, environment)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environment, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
	}

	s.logger.Debug("Fetching agent configurations from OpenChoreo", "agentName", agentName, "environment", environment, "orgName", orgName, "projectName", projectName)
	configurations, err := s.OpenChoreoSvcClient.GetAgentConfigurations(ctx, orgName, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch configurations", "agentName", agentName, "environment", environment, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get configurations for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched configurations successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment, "configCount", len(configurations))
	return configurations, nil
}

func (s *agentManagerService) convertToAgentListItem(agent *clients.AgentComponent) *models.AgentResponse {
	response := &models.AgentResponse{
		UUID: agent.UUID,
		Name:        agent.Name,
		DisplayName: agent.DisplayName,
		Description: agent.Description,
		ProjectName: agent.ProjectName,
		Provisioning: models.Provisioning{
			Type: agent.Provisioning.Type,
		},
		Type: models.AgentType{
			Type:    agent.Type.Type,
			SubType: agent.Type.SubType,
		},
		Language:  agent.Language,
		CreatedAt: agent.CreatedAt,
	}
	return response
}

// convertToExternalAgentResponse converts a database Agent model to AgentResponse for external agents
func (s *agentManagerService) convertExternalAgentToAgentResponse(ocAgentComponent *clients.AgentComponent) *models.AgentResponse {
	return &models.AgentResponse{
		UUID: ocAgentComponent.UUID,
		Name:        ocAgentComponent.Name,
		DisplayName: ocAgentComponent.DisplayName,
		Description: ocAgentComponent.Description,
		ProjectName: ocAgentComponent.ProjectName,
		Provisioning: models.Provisioning{
			Type: ocAgentComponent.Provisioning.Type,
		},
		Type: models.AgentType{
			Type: ocAgentComponent.Type.Type,
		},
		CreatedAt: ocAgentComponent.CreatedAt,
	}
}

// convertToManagedAgentResponse converts an OpenChoreo AgentComponent to AgentResponse for managed agents
func (s *agentManagerService) convertManagedAgentToAgentResponse(ocAgentComponent *clients.AgentComponent) *models.AgentResponse {
	return &models.AgentResponse{
		UUID: ocAgentComponent.UUID,
		Name:        ocAgentComponent.Name,
		DisplayName: ocAgentComponent.DisplayName,
		Description: ocAgentComponent.Description,
		ProjectName: ocAgentComponent.ProjectName,
		Provisioning: models.Provisioning{
			Type: ocAgentComponent.Provisioning.Type,
			Repository: models.Repository{
				Url:     ocAgentComponent.Provisioning.Repository.RepoURL,
				Branch:  ocAgentComponent.Provisioning.Repository.Branch,
				AppPath: ocAgentComponent.Provisioning.Repository.AppPath,
			},
		},
		Type: models.AgentType{
			Type:    ocAgentComponent.Type.Type,
			SubType: ocAgentComponent.Type.SubType,
		},
		Language:  ocAgentComponent.Language,
		CreatedAt: ocAgentComponent.CreatedAt,
	}
}

// buildWorkloadSpec constructs the workload specification from the create agent request
func buildWorkloadSpec(req *spec.CreateAgentRequest) (map[string]interface{}, error) {
	workloadSpec := make(map[string]interface{})

	workloadSpec["envVars"] = req.RuntimeConfigs.Env

	if req.AgentType.Type == string(utils.AgentTypeAPI) &&
		utils.StrPointerAsStr(req.AgentType.SubType, "") == string(utils.AgentSubTypeChatAPI) {
		// Read OpenAPI schema from embedded file
		schemaContent, err := clients.GetDefaultChatAPISchema()
		if err != nil {
			return nil, fmt.Errorf("failed to read Chat API schema: %w", err)
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
	if req.AgentType.Type == string(utils.AgentTypeAPI) && utils.StrPointerAsStr(req.AgentType.SubType, "") == string(utils.AgentSubTypeCustomAPI) {
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

	return workloadSpec, nil
}
