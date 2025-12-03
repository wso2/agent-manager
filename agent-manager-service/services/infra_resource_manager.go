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
	"gorm.io/gorm"

	clients "github.com/wso2-enterprise/agent-management-platform/agent-manager-service/clients/openchoreosvc"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/db"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/models"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/spec"
	"github.com/wso2-enterprise/agent-management-platform/agent-manager-service/utils"
)

type InfraResourceManager interface {
	GetOrgEnvironments(ctx context.Context, userIdpId uuid.UUID, orgName string) ([]*models.EnvironmentResponse, error)
	GetProjectDeploymentPipeline(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string) (*models.DeploymentPipelineResponse, error)
	CreateOrganization(ctx context.Context, userIdpId uuid.UUID, payload spec.CreateOrganizationRequest) (string, error)
	ListOrganizations(ctx context.Context, userIdpId uuid.UUID, limit int, offset int) ([]*models.OrganizationResponse, int32, error)
	GetOrganization(ctx context.Context, userIdpId uuid.UUID, orgName string) (*models.OrganizationResponse, error)
	ListProjects(ctx context.Context, userIdpId uuid.UUID, orgName string, limit int, offset int) ([]*models.ProjectResponse, int32, error)
	GetProject(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string) (*models.ProjectResponse, error)
	CreateProject(ctx context.Context, userIdpId uuid.UUID, orgName string, payload spec.CreateProjectRequest) error
	DeleteProject(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string) error
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
	orgs, err := s.OrganizationRepository.GetOrganizationsByUserIdpID(ctx, userIdpId)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list organizations for user %s: %w", userIdpId, err)
	}
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
	valid, err := s.validateOrganization(ctx, userIdpId, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	if !valid {
		return nil, utils.ErrOrganizationNotFound
	}
	org, err := s.OpenChoreoSvcClient.GetOrganization(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization %s from OpenChoreo: %w", orgName, err)
	}

	s.logger.Info("Fetched organization successfully", "orgName", orgName)
	return org, nil
}

func (s *infraResourceManager) CreateProject(ctx context.Context, userIdpId uuid.UUID, orgName string, payload spec.CreateProjectRequest) error {
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return utils.ErrOrganizationNotFound
		}
		return fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	proj, err := s.ProjectRepository.GetProjectByName(ctx, org.ID, payload.Name)
	if err != nil && !db.IsRecordNotFoundError(err) {
		return fmt.Errorf("failed to check existing projects: %w", err)
	}
	if proj != nil {
		s.logger.Warn("Project already exists in organization", "orgName", orgName, "projectName", payload.Name)
		return utils.ErrProjectAlreadyExists
	}

	deploymentPipelines, err := s.OpenChoreoSvcClient.GetDeploymentPipelinesForOrganization(ctx, orgName)
	if err != nil {
		return fmt.Errorf("failed to get deployment pipelines for organization %s: %w", orgName, err)
	}

	// Check if deployment pipeline exists
	pipelineExists := false
	for _, pipeline := range deploymentPipelines {
		if pipeline.Name == payload.DeploymentPipeline {
			pipelineExists = true
			break
		}
	}
	if !pipelineExists {
		return fmt.Errorf("deployment pipeline %s does not exist in organization %s", payload.DeploymentPipeline, orgName)
	}

	project := &models.Project{
		ID:                uuid.New(),
		OrgID:             org.ID,
		Name:              payload.Name,
		DisplayName:       payload.DisplayName,
		OpenChoreoProject: payload.Name,
	}

	// Save project in database first
	if err := s.ProjectRepository.CreateProject(ctx, project); err != nil {
		return fmt.Errorf("failed to save project in repository: %w", err)
	}

	// Create project in OpenChoreo after successful database transaction
	if err := s.OpenChoreoSvcClient.CreateProject(ctx, orgName, payload.Name, payload.DeploymentPipeline, payload.DisplayName); err != nil {
		// OpenChoreo creation failed, rollback database changes
		deleteErr := db.DB(ctx).Transaction(func(tx *gorm.DB) error {
			txCtx := db.CtxWithTx(ctx, tx)
			if deleteErr := db.DB(txCtx).Where("id = ?", project.ID).Delete(&models.Project{}).Error; deleteErr != nil {
				s.logger.Error("Failed to rollback project creation from database", "projectId", project.ID, "error", deleteErr)
				return deleteErr
			}
			return nil
		})
		if deleteErr != nil {
			s.logger.Error("Critical: Project exists in database but not in OpenChoreo, manual cleanup required",
				"projectId", project.ID, "projectName", payload.Name, "orgName", orgName)
		}
		return fmt.Errorf("failed to create project in OpenChoreo: %w", err)
	}

	s.logger.Info("Project created successfully", "orgName", orgName, "projectName", payload.Name)
	return nil
}

func (s *infraResourceManager) ListProjects(ctx context.Context, userIdpId uuid.UUID, orgName string, limit int, offset int) ([]*models.ProjectResponse, int32, error) {
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, 0, utils.ErrOrganizationNotFound
		}
		return nil, 0, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	projects, err := s.ProjectRepository.ListProjects(ctx, org.ID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list projects for organization %s: %w", orgName, err)
	}

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
			Name:        project.Name,
			OrgName:     orgName,
			DisplayName: project.DisplayName,
			Description: project.Description,
			CreatedAt:   project.CreatedAt,
		}
		projectResponses = append(projectResponses, projectResponse)
	}

	s.logger.Info("Fetched projects successfully", "orgName", orgName)
	return projectResponses, int32(total), nil
}

func (s *infraResourceManager) DeleteProject(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string) error {
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
		// DELETE is idempotent
		if db.IsRecordNotFoundError(err) {
			return nil
		}
		return fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	// Check agents exist for the project
	agents, err := s.AgentRepository.ListAgents(ctx, org.ID, project.ID)
	if err != nil {
		return fmt.Errorf("failed to list agents for project %s: %w", projectName, err)
	}
	if len(agents) > 0 {
		return fmt.Errorf("cannot delete project %s: project has %d associated agent(s)", projectName, len(agents))
	}
	// Soft delete the project from the database
	if err := s.ProjectRepository.SoftDeleteProject(ctx, org.ID, project.ID); err != nil {
		return fmt.Errorf("failed to delete project %s from repository: %w", projectName, err)
	}

	// Delete project from OpenChoreo
	if err := s.OpenChoreoSvcClient.DeleteProject(ctx, orgName, projectName); err != nil {
		return fmt.Errorf("failed to delete project %s from OpenChoreo: %w", projectName, err)
	}

	// Hard delete the project from the database
	if err := s.ProjectRepository.HardDeleteProject(ctx, org.ID, project.ID); err != nil {
		return fmt.Errorf("failed to hard delete project %s from repository: %w", projectName, err)
	}
	return nil
}

func (s *infraResourceManager) GetProject(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string) (*models.ProjectResponse, error) {
	// Validate organization exists
	org, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, orgName)
	if err != nil {
		if db.IsRecordNotFoundError(err) {
			return nil, utils.ErrOrganizationNotFound
		}
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	_, err = s.ProjectRepository.GetProjectByName(ctx, org.ID, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to find project %s: %w", projectName, err)
	}

	openChoreoProject, err := s.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project %s for organization %s: %w", projectName, orgName, err)
	}

	s.logger.Info("Fetched project successfully", "orgName", orgName, "projectName", projectName)
	return openChoreoProject, nil
}

func (s *infraResourceManager) CreateOrganization(ctx context.Context, userIdpId uuid.UUID, payload spec.CreateOrganizationRequest) (string, error) {
	// Check if organization already exists
	orgRepo, err := s.OrganizationRepository.GetOrganizationByOrgName(ctx, userIdpId, payload.Name)
	if err != nil && !db.IsRecordNotFoundError(err) {
		return "", fmt.Errorf("failed to check existing organizations: %w", err)
	}
	if orgRepo != nil {
		s.logger.Warn("Organization already exists for user", "userIdpId", userIdpId)
		return "", utils.ErrOrganizationAlreadyExists
	}
	s.logger.Info("Creating organization", "userIdpId", userIdpId, "orgName", payload.Name)

	orgName := payload.Name

	// Cleanup function for when organization creation fails
	cleanup := func() {
		s.logger.Warn("Organization creation failed, cleaning up resources", "orgName", orgName)
		if cleanupErr := s.OpenChoreoSvcClient.CleanupOrganizationResources(ctx, orgName); cleanupErr != nil {
			s.logger.Error("Failed to cleanup organization resources", "orgName", orgName, "error", cleanupErr)
		}
	}

	// Create namespace in OpenChoreo
	if err := s.OpenChoreoSvcClient.CreateNamespaceForOrganization(ctx, orgName); err != nil {
		cleanup()
		return "", fmt.Errorf("failed to create namespace in OpenChoreo: %w", err)
	}

	// Create organization in OpenChoreo
	if err := s.OpenChoreoSvcClient.CreateOrganization(ctx, orgName, orgName, orgName); err != nil {
		cleanup()
		return "", fmt.Errorf("failed to create organization in OpenChoreo: %w", err)
	}

	// Create default build plane for organization
	buildPlaneName := orgName
	if err := s.OpenChoreoSvcClient.CreateBuildPlaneForOrganization(ctx, orgName, buildPlaneName); err != nil {
		cleanup()
		return "", fmt.Errorf("failed to create build plane in OpenChoreo: %w", err)
	}

	// Create default data plane for organization
	dataPlaneName := clients.DefaultName
	if err := s.OpenChoreoSvcClient.CreateDataPlaneForOrganization(ctx, orgName, dataPlaneName); err != nil {
		cleanup()
		return "", fmt.Errorf("failed to create data plane in OpenChoreo: %w", err)
	}

	// Create a service class for python, in addition to the default service class
	serviceClassName := clients.ObservabilityEnabledServiceClassName
	if err := s.OpenChoreoSvcClient.CreateObservabilityEnabledServiceClassForPython(ctx, orgName, serviceClassName); err != nil {
		cleanup()
		return "", fmt.Errorf("failed to create service class in OpenChoreo: %w", err)
	}

	// Create default API class for organization with CORS settings
	apiClassName := clients.DefaultAPIClassNameWithCORS
	if err := s.OpenChoreoSvcClient.CreateAPIClassDefaultWithCORS(ctx, orgName, apiClassName); err != nil {
		cleanup()
		return "", fmt.Errorf("failed to create API class in OpenChoreo: %w", err)
	}

	// Create dev environment for organization
	environmentName := clients.DevEnvironmentName
	envDisplayName := clients.DevEnvironmentDisplayName
	isProduction := false // Development environment should not be production
	dnsPrefix := "dev"
	if err := s.OpenChoreoSvcClient.CreateEnvironments(ctx, orgName, environmentName, envDisplayName, dataPlaneName, isProduction, dnsPrefix); err != nil {
		cleanup()
		return "", fmt.Errorf("failed to create environment in OpenChoreo: %w", err)
	}

	// Create default deployment pipeline for organization
	pipelineName := clients.DefaultName
	promotionPaths := []models.PromotionPath{
		{
			SourceEnvironmentRef: clients.DevEnvironmentName,
		},
	}
	if err := s.OpenChoreoSvcClient.CreateDeploymentPipeline(ctx, orgName, pipelineName, promotionPaths); err != nil {
		cleanup()
		return "", fmt.Errorf("failed to create deployment pipeline in OpenChoreo: %w", err)
	}

	// Create default project for organization
	projectName := clients.DefaultName
	projectDisplayName := clients.DefaultDisplayName
	deploymentPipelineRef := pipelineName
	if err := s.OpenChoreoSvcClient.CreateProject(ctx, orgName, projectName, deploymentPipelineRef, projectDisplayName); err != nil {
		cleanup()
		return "", fmt.Errorf("failed to create project in OpenChoreo: %w", err)
	}

	orgId := uuid.New()
	org := &models.Organization{
		ID:                orgId,
		UserIdpId:         userIdpId,
		OpenChoreoOrgName: orgName,
	}

	project := &models.Project{
		ID:                uuid.New(),
		OrgID:             orgId,
		Name:              projectName,
		DisplayName:       projectDisplayName,
		OpenChoreoProject: projectName,
	}

	// Execute database operations in a transaction
	err = db.DB(ctx).Transaction(func(tx *gorm.DB) error {
		ctx := db.CtxWithTx(ctx, tx)

		if err := s.OrganizationRepository.CreateOrganization(ctx, org); err != nil {
			return fmt.Errorf("failed to save organization in repository: %w", err)
		}

		if err := s.ProjectRepository.CreateProject(ctx, project); err != nil {
			return fmt.Errorf("failed to save project in repository: %w", err)
		}

		return nil
	})
	if err != nil {
		cleanup()
		return "", err
	}

	s.logger.Info("Organization and default project created successfully", "userIdpId", userIdpId, "orgName", orgName)
	return orgName, nil
}

func (s *infraResourceManager) GetOrgEnvironments(ctx context.Context, userIdpId uuid.UUID, orgName string) ([]*models.EnvironmentResponse, error) {
	// Validate organization exists
	valid, err := s.validateOrganization(ctx, userIdpId, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}
	if !valid {
		return nil, utils.ErrOrganizationNotFound
	}

	environments, err := s.OpenChoreoSvcClient.GetOrgEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
	}

	s.logger.Info("Fetched environments successfully", "orgName", orgName)
	return environments, nil
}

func (s *infraResourceManager) GetProjectDeploymentPipeline(ctx context.Context, userIdpId uuid.UUID, orgName string, projectName string) (*models.DeploymentPipelineResponse, error) {
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
		return nil, fmt.Errorf("failed to find project %s in organization %s: %w", projectName, orgName, err)
	}

	openChoreoProject, err := s.OpenChoreoSvcClient.GetProject(ctx, project.OpenChoreoProject, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get project %s from OpenChoreo: %w", projectName, err)
	}

	pipelineName := "default"
	if openChoreoProject.DeploymentPipeline != "" {
		// Project has an explicit deployment pipeline reference
		pipelineName = openChoreoProject.DeploymentPipeline
		s.logger.Debug("Using explicit deployment pipeline reference", "pipeline", pipelineName)
	}

	deploymentPipeline, err := s.OpenChoreoSvcClient.GetDeploymentPipeline(ctx, orgName, pipelineName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline for project %s: %w", projectName, err)
	}

	s.logger.Info("Fetched deployment pipeline successfully", "orgName", orgName, "projectName", projectName)

	return deploymentPipeline, nil
}

// Todo: implement validateOrganization as a common utility function to be used across services
func (s *infraResourceManager) validateOrganization(ctx context.Context, userIdpID uuid.UUID, orgName string) (bool, error) {
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
