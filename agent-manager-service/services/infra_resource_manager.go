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
	"time"

	clients "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

type InfraResourceManager interface {
	ListOrgEnvironments(ctx context.Context, orgName string) ([]*models.EnvironmentResponse, error)
	GetProjectDeploymentPipeline(ctx context.Context, orgName string, projectName string) (*models.DeploymentPipelineResponse, error)
	ListOrganizations(ctx context.Context, limit int, offset int) ([]*models.OrganizationResponse, int32, error)
	GetOrganization(ctx context.Context, orgName string) (*models.OrganizationResponse, error)
	ListProjects(ctx context.Context, orgName string, limit int, offset int) ([]*models.ProjectResponse, int32, error)
	GetProject(ctx context.Context, orgName string, projectName string) (*models.ProjectResponse, error)
	CreateProject(ctx context.Context, orgName string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error)
	DeleteProject(ctx context.Context, orgName string, projectName string) error
	ListOrgDeploymentPipelines(ctx context.Context, orgName string, limit int, offset int) ([]*models.DeploymentPipelineResponse, int, error)
	GetDataplanes(ctx context.Context, orgName string) ([]*models.DataPlaneResponse, error)
}

type infraResourceManager struct {
	AgentRepository     repositories.AgentRepository
	OpenChoreoSvcClient clients.OpenChoreoSvcClient
	logger              *slog.Logger
}

func NewInfraResourceManager(
	agentRepo repositories.AgentRepository,
	openChoreoSvcClient clients.OpenChoreoSvcClient,
	logger *slog.Logger,
) InfraResourceManager {
	return &infraResourceManager{
		AgentRepository:     agentRepo,
		OpenChoreoSvcClient: openChoreoSvcClient,
		logger:              logger,
	}
}

func (s *infraResourceManager) ListOrganizations(ctx context.Context, limit int, offset int) ([]*models.OrganizationResponse, int32, error) {
	s.logger.Debug("ListOrganizations called", "limit", limit, "offset", offset)
	orgs, err := s.OpenChoreoSvcClient.ListOrganizations(ctx)
	if err != nil {
		s.logger.Error("Failed to list organizations from openchoreo", "error", err)
		return nil, 0, fmt.Errorf("failed to list organizations: %w", err)
	}
	s.logger.Debug("Retrieved organizations from openchoreo", "totalCount", len(orgs))
	total := len(orgs)
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
	return paginatedOrgs, int32(total), nil
}

func (s *infraResourceManager) GetOrganization(ctx context.Context, orgName string) (*models.OrganizationResponse, error) {
	s.logger.Debug("GetOrganization called", "orgName", orgName)

	_, err := s.OpenChoreoSvcClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization from repository", "orgName", orgName, "error", err)
		return nil, err
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

func (s *infraResourceManager) CreateProject(ctx context.Context, orgName string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error) {
	s.logger.Debug("CreateProject called", "orgName", orgName, "projectName", payload.Name, "deploymentPipeline", payload.DeploymentPipeline)

	// Validate organization exists
	_, err := s.OpenChoreoSvcClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization from repository", "orgName", orgName, "error", err)
		return nil, err
	}
	_, err = s.OpenChoreoSvcClient.GetProject(ctx, payload.Name, orgName)
	if err == nil {
		// Project already exists
		s.logger.Error("Project already exists", "orgName", orgName, "projectName", payload.Name)
		return nil, utils.ErrProjectAlreadyExists
	} else if err != utils.ErrProjectNotFound {
		// Some other error occurred
		s.logger.Error("Failed to check existing projects", "orgName", orgName, "projectName", payload.Name, "error", err)
		return nil, fmt.Errorf("failed to check existing projects: %w", err)
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

	// Create project in OpenChoreo
	if err := s.OpenChoreoSvcClient.CreateProject(ctx, orgName, payload.Name, payload.DeploymentPipeline, payload.DisplayName, utils.StrPointerAsStr(payload.Description, "")); err != nil {
		return nil, fmt.Errorf("failed to create project in OpenChoreo: %w", err)
	}
	s.logger.Info("Project created successfully", "orgName", orgName, "projectName", payload.Name)

	return &models.ProjectResponse{
		Name:               payload.Name,
		OrgName:            orgName,
		DisplayName:        payload.DisplayName,
		Description:        utils.StrPointerAsStr(payload.Description, ""),
		CreatedAt:          time.Now(),
		DeploymentPipeline: payload.DeploymentPipeline,
	}, nil
}

func (s *infraResourceManager) ListProjects(ctx context.Context, orgName string, limit int, offset int) ([]*models.ProjectResponse, int32, error) {
	s.logger.Debug("ListProjects called", "orgName", orgName, "limit", limit, "offset", offset)

	// Validate organization exists
	_, err := s.OpenChoreoSvcClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization from repository", "orgName", orgName, "error", err)
		return nil, 0, err
	}

	projects, err := s.OpenChoreoSvcClient.ListProjects(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to list projects from repository", "orgName", orgName, "error", err)
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
			UUID:        project.UUID,
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

func (s *infraResourceManager) DeleteProject(ctx context.Context, orgName string, projectName string) error {
	s.logger.Debug("DeleteProject called", "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	_, err := s.OpenChoreoSvcClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization from repository", "orgName", orgName, "error", err)
		return err
	}
	_, err = s.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		// DELETE is idempotent
		if err == utils.ErrProjectNotFound {
			s.logger.Debug("Project not found, treating as successful delete (idempotent)", "orgName", orgName, "projectName", projectName)
			return nil
		}
		s.logger.Error("Failed to get project", "orgName", orgName, "projectName", projectName, "error", err)
		return fmt.Errorf("failed to find project %s: %w", projectName, err)
	}
	s.logger.Debug("Project found", "orgName", orgName, "projectName", projectName)

	// Check agents exist for the project
	s.logger.Debug("Checking for associated agents", "projectName", projectName)
	agents, err := s.OpenChoreoSvcClient.ListAgentComponents(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to list agents for project", "projectName", projectName, "error", err)
		return fmt.Errorf("failed to list agents for project %s: %w", projectName, err)
	}
	if len(agents) > 0 {
		s.logger.Warn("Cannot delete project with associated agents", "orgName", orgName, "projectName", projectName, "agentCount", len(agents))
		return utils.ErrProjectHasAssociatedAgents
	}
	s.logger.Debug("No associated agents found, proceeding with deletion", "projectName", projectName)
	// Delete project from OpenChoreo
	err = s.OpenChoreoSvcClient.DeleteProject(ctx, orgName, projectName)
	if err != nil {
		return err
	}
	s.logger.Info("Project deleted successfully", "orgName", orgName, "projectName", projectName)
	return nil
}

func (s *infraResourceManager) GetProject(ctx context.Context, orgName string, projectName string) (*models.ProjectResponse, error) {
	s.logger.Debug("GetProject called", "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	_, err := s.OpenChoreoSvcClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization from repository", "orgName", orgName, "error", err)
		return nil, err
	}
	openChoreoProject, err := s.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		s.logger.Error("Failed to get project from OpenChoreo", "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get project %s for organization %s: %w", projectName, orgName, err)
	}

	s.logger.Info("Fetched project successfully", "orgName", orgName, "projectName", projectName)
	return openChoreoProject, nil
}

func (s *infraResourceManager) ListOrgDeploymentPipelines(ctx context.Context, orgName string, limit int, offset int) ([]*models.DeploymentPipelineResponse, int, error) {
	s.logger.Debug("ListOrgDeploymentPipelines called", "orgName", orgName)

	// Validate organization exists
	_, err := s.OpenChoreoSvcClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization from repository", "orgName", orgName, "error", err)
		return nil, 0, err
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

func (s *infraResourceManager) ListOrgEnvironments(ctx context.Context, orgName string) ([]*models.EnvironmentResponse, error) {
	s.logger.Debug("ListOrgEnvironments called", "orgName", orgName)

	// Validate organization exists
	_, err := s.OpenChoreoSvcClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization from repository", "orgName", orgName, "error", err)
		return nil, err
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

func (s *infraResourceManager) GetProjectDeploymentPipeline(ctx context.Context, orgName string, projectName string) (*models.DeploymentPipelineResponse, error) {
	s.logger.Debug("GetProjectDeploymentPipeline called", "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	_, err := s.OpenChoreoSvcClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization from repository", "orgName", orgName, "error", err)
		return nil, err
	}
	openChoreoProject, err := s.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		s.logger.Error("Failed to get project from OpenChoreo", "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
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

func (s *infraResourceManager) GetDataplanes(ctx context.Context, orgName string) ([]*models.DataPlaneResponse, error) {
	s.logger.Debug("GetDataplanes called", "orgName", orgName)

	// Validate organization exists
	_, err := s.OpenChoreoSvcClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to get organization from repository", "orgName", orgName, "error", err)
		return nil, err
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
