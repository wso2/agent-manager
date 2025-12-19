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
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	clients "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

type InfraResourceManager interface {
	ListOrgEnvironments(ctx context.Context, userIdpId uuid.UUID, orgName string) ([]*models.EnvironmentResponse, error)
	GetProjectDeploymentPipeline(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string) (*models.DeploymentPipelineResponse, error)
	ListOrganizations(ctx context.Context, userIdpId uuid.UUID, limit int, offset int) ([]*models.OrganizationResponse, int32, error)
	GetOrganization(ctx context.Context, userIdpId uuid.UUID, orgName string) (*models.OrganizationResponse, error)
	ListProjects(ctx context.Context, userIdpId uuid.UUID, orgName string, limit int, offset int) ([]*models.ProjectResponse, int32, error)
	GetProject(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string) (*models.ProjectResponse, error)
	CreateProject(ctx context.Context, userIdpId uuid.UUID, orgName string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error)
	DeleteProject(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string) error
	ListOrgDeploymentPipelines(ctx context.Context, userIdpId uuid.UUID, orgName string, limit int, offset int) ([]*models.DeploymentPipelineResponse, int, error)
	GetDataplanes(ctx context.Context, userIdpId uuid.UUID, orgName string) ([]*models.DataPlaneResponse, error)
}

type infraResourceManager struct {
	OrganizationRepository repositories.OrganizationRepository
	AgentRepository        repositories.AgentRepository
	ProjectRepository      repositories.ProjectRepository
	OpenChoreoSvcClient    clients.OpenChoreoSvcClient
	logger                 *slog.Logger
}

func NewInfraResourceManager(
	orgRepo repositories.OrganizationRepository,
	projectRepo repositories.ProjectRepository,
	agentRepo repositories.AgentRepository,
	openChoreoSvcClient clients.OpenChoreoSvcClient,
	logger *slog.Logger,
) InfraResourceManager {
	return &infraResourceManager{
		OrganizationRepository: orgRepo,
		ProjectRepository:      projectRepo,
		AgentRepository:        agentRepo,
		OpenChoreoSvcClient:    openChoreoSvcClient,
		logger:                 logger,
	}
}

func (s *infraResourceManager) ListOrganizations(ctx context.Context, userIdpId uuid.UUID, limit int, offset int) ([]*models.OrganizationResponse, int32, error) {
	s.logger.Debug("ListOrganizations called", "userIdpId", userIdpId, "limit", limit, "offset", offset)

	orgs, err := s.OrganizationRepository.GetOrganizationsByUserIdpID(ctx, userIdpId)
	if err != nil {
		s.logger.Error("Failed to get organizations from repository", "userIdpId", userIdpId, "error", err)
		return nil, 0, fmt.Errorf("failed to list organizations for user %s: %w", userIdpId, err)
	}
	s.logger.Debug("Retrieved organizations from repository", "userIdpId", userIdpId, "totalCount", len(orgs))

	total := int32(len(orgs))
	// Apply pagination
	start := offset
	if start > len(orgs) {
		start = len(orgs)
	}
	end := offset + limit
	if end > len(orgs) {
		end = len(orgs)
	}
	paginatedOrgs := orgs[start:end]

	// Convert Organization models to OrganizationResponse DTOs
	var orgResponses []*models.OrganizationResponse
	for _, org := range paginatedOrgs {
		orgResponse := &models.OrganizationResponse{
			Name:      org.OpenChoreoOrgName,
			CreatedAt: org.CreatedAt,
		}
		orgResponses = append(orgResponses, orgResponse)
	}

	s.logger.Info("Fetched organizations successfully", "count", len(orgResponses))
	return orgResponses, total, nil
}

func (s *infraResourceManager) GetOrganization(ctx context.Context, userIdpId uuid.UUID, orgName string) (*models.OrganizationResponse, error) {
	s.logger.Debug("GetOrganization called", "userIdpId", userIdpId, "orgName", orgName)

	_, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Debug("Organization not found in repository", "userIdpId", userIdpId, "orgName", orgName)
			return nil, utils.ErrOrganizationNotFound
		}
		s.logger.Error("Failed to get organization from repository", "userIdpId", userIdpId, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	s.logger.Debug("Organization found in repository, fetching from OpenChoreo", "orgName", orgName)

	org, err := s.OpenChoreoSvcClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization from OpenChoreo", "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to get organization %s from OpenChoreo: %w", orgName, err)
	}

	s.logger.Info("Fetched organization successfully", "orgName", orgName)
	return org, nil
}

func (s *infraResourceManager) CreateProject(ctx context.Context, userIdpId uuid.UUID, orgName string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error) {
	s.logger.Debug("CreateProject called", "userIdpId", userIdpId, "orgName", orgName, "projectName", payload.Name, "deploymentPipeline", payload.DeploymentPipeline)

	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Debug("Organization not found", "userIdpId", userIdpId, "orgName", orgName)
			return nil, utils.ErrOrganizationNotFound
		}
		s.logger.Error("Failed to get organization from repository", "userIdpId", userIdpId, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	proj, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, payload.Name)
	if err != nil && !db.IsRecordNotFoundError(err) {
		s.logger.Error("Failed to check existing projects", "orgId", org.ID, "projectName", payload.Name, "error", err)
		return nil, fmt.Errorf("failed to check existing projects: %w", err)
	}
	if proj != nil {
		s.logger.Warn("Project already exists in organization", "orgName", orgName, "projectName", payload.Name, "projectId", proj.ID)
		return nil, utils.ErrProjectAlreadyExists
	}
	s.logger.Debug("Verified project does not exist", "orgName", orgName, "projectName", payload.Name)

	s.logger.Debug("Fetching deployment pipelines from OpenChoreo", "orgName", orgName)
	deploymentPipelines, err := s.OpenChoreoSvcClient.GetDeploymentPipelinesForOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get deployment pipelines from OpenChoreo", "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to get deployment pipelines for organization %s: %w", orgName, err)
	}
	s.logger.Debug("Retrieved deployment pipelines", "orgName", orgName, "pipelineCount", len(deploymentPipelines))

	// Check if deployment pipeline exists
	pipelineExists := false
	for _, pipeline := range deploymentPipelines {
		if pipeline.Name == payload.DeploymentPipeline {
			pipelineExists = true
			break
		}
	}
	if !pipelineExists {
		s.logger.Warn("Deployment pipeline not found", "orgName", orgName, "requestedPipeline", payload.DeploymentPipeline)
		return nil, utils.ErrDeploymentPipelineNotFound
	}

	project := &models.Project{
		ID:                uuid.New(),
		OrgID:             org.ID,
		Name:              payload.Name,
		DisplayName:       payload.DisplayName,
		Description:       utils.StrPointerAsStr(payload.Description, ""),
		OpenChoreoProject: payload.Name,
	}

	// Save project in database first
	if err := s.ProjectRepository.CreateProject(ctx, project); err != nil {
		s.logger.Error("Failed to save project in repository", "projectId", project.ID, "projectName", payload.Name, "error", err)
		return nil, fmt.Errorf("failed to save project in repository: %w", err)
	}
	s.logger.Debug("Project saved to database successfully", "projectId", project.ID, "projectName", payload.Name)

	// Create project in OpenChoreo after successful database transaction
	if err := s.OpenChoreoSvcClient.CreateProject(ctx, orgName, payload.Name, payload.DeploymentPipeline, payload.DisplayName, utils.StrPointerAsStr(payload.Description, "")); err != nil {
		s.logger.Error("Failed to create project in OpenChoreo, initiating rollback", "orgName", orgName, "projectName", payload.Name, "error", err)
		// OpenChoreo creation failed, rollback database changes
		deleteErr := s.ProjectRepository.HardDeleteProject(ctx, org.ID, project.ID)
		if deleteErr != nil {
			s.logger.Error("Critical: Project exists in database but not in OpenChoreo, manual cleanup required",
				"projectId", project.ID, "projectName", payload.Name, "orgName", orgName)
		} else {
			s.logger.Debug("Successfully rolled back database changes", "projectId", project.ID, "projectName", payload.Name)
		}
		return nil, fmt.Errorf("failed to create project in OpenChoreo: %w", err)
	}
	s.logger.Info("Project created successfully", "orgName", orgName, "projectName", payload.Name, "projectId", project.ID)

	return &models.ProjectResponse{
		Name:               project.Name,
		OrgName:            orgName,
		DisplayName:        project.DisplayName,
		Description:        project.Description,
		CreatedAt:          project.CreatedAt,
		DeploymentPipeline: payload.DeploymentPipeline,
	}, nil
}

func (s *infraResourceManager) ListProjects(ctx context.Context, userIdpId uuid.UUID, orgName string, limit int, offset int) ([]*models.ProjectResponse, int32, error) {
	s.logger.Debug("ListProjects called", "userIdpId", userIdpId, "orgName", orgName, "limit", limit, "offset", offset)

	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Debug("Organization not found", "userIdpId", userIdpId, "orgName", orgName)
			return nil, 0, utils.ErrOrganizationNotFound
		}
		s.logger.Error("Failed to get organization from repository", "userIdpId", userIdpId, "orgName", orgName, "error", err)
		return nil, 0, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	projects, err := s.OpenChoreoSvcClient.ListProjects(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to list projects from repository", "orgId", org.ID, "orgName", orgName, "error", err)
		return nil, 0, fmt.Errorf("failed to list projects for organization %s: %w", orgName, err)
	}
	s.logger.Debug("Retrieved projects from repository", "orgName", orgName, "totalCount", len(projects))

	total := len(projects)
	// Apply pagination
	start := offset
	if start > len(projects) {
		start = len(projects)
	}
	end := offset + limit
	if end > len(projects) {
		end = len(projects)
	}
	paginatedProjects := projects[start:end]

	// Convert Project models to ProjectResponse DTOs
	var projectResponses []*models.ProjectResponse
	for _, project := range paginatedProjects {
		projectResponse := &models.ProjectResponse{
			UUID:  project.UUID,
			Name:        project.Name,
			OrgName:     orgName,
			DisplayName: project.DisplayName,
			Description: project.Description,
			CreatedAt:   project.CreatedAt,
		}
		projectResponses = append(projectResponses, projectResponse)
	}

	s.logger.Info("Fetched projects successfully", "orgName", orgName, "count", len(projectResponses), "total", total)
	return projectResponses, int32(total), nil
}

func (s *infraResourceManager) DeleteProject(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string) error {
	s.logger.Debug("DeleteProject called", "userIdpId", userIdpId, "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Debug("Organization not found", "userIdpId", userIdpId, "orgName", orgName)
			return utils.ErrOrganizationNotFound
		}
		s.logger.Error("Failed to get organization from repository", "userIdpId", userIdpId, "orgName", orgName, "error", err)
		return fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		// DELETE is idempotent
		if db.IsRecordNotFoundError(err) {
			s.logger.Debug("Project not found, treating as successful delete (idempotent)", "orgName", orgName, "projectName", projectName)
			return nil
		}
		s.logger.Error("Failed to get project from repository", "orgId", org.ID, "projectName", projectName, "error", err)
		return fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	s.logger.Debug("Project found", "orgName", orgName, "projectName", projectName, "projectId", project.ID)

	// Check agents exist for the project
	s.logger.Debug("Checking for associated agents", "projectId", project.ID, "projectName", projectName)
	agents, err := s.AgentRepository.ListAgents(ctx, org.ID, project.ID)
	if err != nil {
		s.logger.Error("Failed to list agents for project", "projectId", project.ID, "projectName", projectName, "error", err)
		return fmt.Errorf("failed to list agents for project %s: %w", projectName, err)
	}
	if len(agents) > 0 {
		s.logger.Warn("Cannot delete project with associated agents", "orgName", orgName, "projectName", projectName, "agentCount", len(agents))
		return utils.ErrProjectHasAssociatedAgents
	}
	s.logger.Debug("No associated agents found, proceeding with deletion", "projectName", projectName)
	err = s.handleProjectDeletion(ctx, org.ID, project.ID, orgName, projectName)
	if err != nil {
		return err
	}
	s.logger.Info("Project deleted successfully", "orgName", orgName, "projectName", projectName, "projectId", project.ID)
	return nil
}

func (s *infraResourceManager) handleProjectDeletion(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, orgName string, projectName string) error {
	// Soft delete project from database
	s.logger.Debug("Handling project deletion", "orgName", orgName, "projectName", projectName)
	if err := s.ProjectRepository.SoftDeleteProject(ctx, orgId, projectId); err != nil {
		s.logger.Error("Critical: Failed to soft delete project from database",
			"projectId", projectId, "projectName", projectName, "orgName", orgName, "error", err)
		return fmt.Errorf("failed to delete project %s from repository: %w", projectName, err)
	}
	// Delete project from OpenChoreo
	err := s.OpenChoreoSvcClient.DeleteProject(ctx, orgName, projectName)
	if err != nil {
		// Delete project from OpenChoreo failed, rollback database changes
		err := s.ProjectRepository.RollbackSoftDeleteProject(ctx, orgId, projectId)
		if err != nil {
			s.logger.Error("Critical: Project exists in database but not in OpenChoreo, manual cleanup required",
				"projectId", projectId, "projectName", projectName, "orgName", orgName, "error", err)
		}
		return fmt.Errorf("failed to delete project %s from OpenChoreo: %w", projectName, err)
	}
	s.logger.Debug("Project deleted from OpenChoreo successfully", "orgName", orgName, "projectName", projectName)
	// Delete project from database
	s.logger.Debug("Deleting project from database", "projectId", projectId, "projectName", projectName)
	if err := s.ProjectRepository.HardDeleteProject(ctx, orgId, projectId); err != nil {
		s.logger.Error("Critical: Project deleted from OpenChoreo but DB deletion failed, retry required",
			"projectId", projectId, "projectName", projectName, "orgName", orgName, "error", err)
		return fmt.Errorf("failed to delete project %s from repository: %w", projectName, err)
	}
	return nil
}

func (s *infraResourceManager) GetProject(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string) (*models.ProjectResponse, error) {
	s.logger.Debug("GetProject called", "userIdpId", userIdpId, "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Debug("Organization not found", "userIdpId", userIdpId, "orgName", orgName)
			return nil, utils.ErrOrganizationNotFound
		}
		s.logger.Error("Failed to get organization from repository", "userIdpId", userIdpId, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Debug("Project not found in repository", "orgId", org.ID, "projectName", projectName)
			return nil, utils.ErrProjectNotFound
		}
		s.logger.Error("Failed to get project from repository", "orgId", org.ID, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to find project %s in organization %s: %w", projectName, orgName, err)
	}
	s.logger.Debug("Project found in repository, fetching from OpenChoreo", "projectName", projectName, "projectId", project.ID)

	openChoreoProject, err := s.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		s.logger.Error("Failed to get project from OpenChoreo", "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get project %s for organization %s: %w", projectName, orgName, err)
	}

	s.logger.Info("Fetched project successfully", "orgName", orgName, "projectName", projectName)
	return openChoreoProject, nil
}

func (s *infraResourceManager) ListOrgDeploymentPipelines(ctx context.Context, userIdpId uuid.UUID, orgName string, limit int, offset int) ([]*models.DeploymentPipelineResponse, int, error) {
	s.logger.Debug("ListOrgDeploymentPipelines called", "userIdpId", userIdpId, "orgName", orgName)

	// Validate organization exists
	_, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Debug("Organization not found", "userIdpId", userIdpId, "orgName", orgName)
			return nil, 0, utils.ErrOrganizationNotFound
		}
		s.logger.Error("Failed to get organization from repository", "userIdpId", userIdpId, "orgName", orgName, "error", err)
		return nil, 0, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	s.logger.Debug("Fetching deployment pipelines from OpenChoreo", "orgName", orgName)
	deploymentPipelines, err := s.OpenChoreoSvcClient.GetDeploymentPipelinesForOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get deployment pipelines from OpenChoreo", "orgName", orgName, "error", err)
		return nil, 0, fmt.Errorf("failed to get deployment pipelines for organization %s: %w", orgName, err)
	}

	s.logger.Info("Fetched deployment pipelines successfully", "orgName", orgName, "count", len(deploymentPipelines))
	total := len(deploymentPipelines)
	// Apply pagination
	start := offset
	if start > len(deploymentPipelines) {
		start = len(deploymentPipelines)
	}
	end := offset + limit
	if end > len(deploymentPipelines) {
		end = len(deploymentPipelines)
	}
	paginatedDeploymentPipelines := deploymentPipelines[start:end]

	return paginatedDeploymentPipelines, total, nil
}

func (s *infraResourceManager) ListOrgEnvironments(ctx context.Context, userIdpId uuid.UUID, orgName string) ([]*models.EnvironmentResponse, error) {
	s.logger.Debug("ListOrgEnvironments called", "userIdpId", userIdpId, "orgName", orgName)

	// Validate organization exists
	_, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Debug("Organization not found", "userIdpId", userIdpId, "orgName", orgName)
			return nil, utils.ErrOrganizationNotFound
		}
		s.logger.Error("Failed to get organization from repository", "userIdpId", userIdpId, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	s.logger.Debug("Fetching environments from OpenChoreo", "orgName", orgName)
	environments, err := s.OpenChoreoSvcClient.ListOrgEnvironments(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get environments from OpenChoreo", "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
	}

	s.logger.Info("Fetched environments successfully", "orgName", orgName, "count", len(environments))
	return environments, nil
}

func (s *infraResourceManager) GetProjectDeploymentPipeline(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string) (*models.DeploymentPipelineResponse, error) {
	s.logger.Debug("GetProjectDeploymentPipeline called", "userIdpId", userIdpId, "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Debug("Organization not found", "userIdpId", userIdpId, "orgName", orgName)
			return nil, utils.ErrOrganizationNotFound
		}
		s.logger.Error("Failed to get organization from repository", "userIdpId", userIdpId, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	project, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Debug("Project not found in repository", "orgId", org.ID, "projectName", projectName)
			return nil, utils.ErrProjectNotFound
		}
		s.logger.Error("Failed to get project from repository", "orgId", org.ID, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to find project %s in organization %s: %w", projectName, orgName, err)
	}
	s.logger.Debug("Project found in repository, fetching from OpenChoreo", "projectName", projectName, "projectId", project.ID, "openChoreoProject", project.OpenChoreoProject)

	openChoreoProject, err := s.OpenChoreoSvcClient.GetProject(ctx, project.OpenChoreoProject, orgName)
	if err != nil {
		s.logger.Error("Failed to get project from OpenChoreo", "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get project %s from OpenChoreo: %w", projectName, err)
	}

	pipelineName := openChoreoProject.DeploymentPipeline
	s.logger.Debug("Fetching deployment pipeline from OpenChoreo", "orgName", orgName, "pipelineName", pipelineName)
	deploymentPipeline, err := s.OpenChoreoSvcClient.GetDeploymentPipeline(ctx, orgName, pipelineName)
	if err != nil {
		s.logger.Error("Failed to get deployment pipeline from OpenChoreo", "orgName", orgName, "pipelineName", pipelineName, "error", err)
		return nil, fmt.Errorf("failed to get deployment pipeline for project %s: %w", projectName, err)
	}

	s.logger.Info("Fetched deployment pipeline successfully", "orgName", orgName, "projectName", projectName, "pipelineName", pipelineName)

	return deploymentPipeline, nil
}

func (s *infraResourceManager) GetDataplanes(ctx context.Context, userIdpId uuid.UUID, orgName string) ([]*models.DataPlaneResponse, error) {
	s.logger.Debug("GetDataplanes called", "userIdpId", userIdpId, "orgName", orgName)

	// Validate organization exists
	_, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			s.logger.Debug("Organization not found", "userIdpId", userIdpId, "orgName", orgName)
			return nil, utils.ErrOrganizationNotFound
		}
		s.logger.Error("Failed to get organization from repository", "userIdpId", userIdpId, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	s.logger.Debug("Fetching dataplanes from OpenChoreo", "orgName", orgName)
	dataplanes, err := s.OpenChoreoSvcClient.GetDataplanesForOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get dataplanes from OpenChoreo", "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to get dataplanes for organization %s: %w", orgName, err)
	}

	s.logger.Info("Fetched dataplanes successfully", "orgName", orgName, "count", len(dataplanes))
	return dataplanes, nil
}
