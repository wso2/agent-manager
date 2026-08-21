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

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type InfraResourceManager interface {
	ListOrgEnvironments(ctx context.Context, ouID string) ([]*models.EnvironmentResponse, error)
	GetProjectDeploymentPipeline(ctx context.Context, ouID string, projectName string) (*models.DeploymentPipelineResponse, error)
	CreateOrgDeploymentPipeline(ctx context.Context, ouID string, displayName string, description *string, projectName *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error)
	UpdateOrgDeploymentPipeline(ctx context.Context, ouID string, pipelineName string, displayName *string, description *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error)
	ListOrganizations(ctx context.Context, limit int, offset int) ([]*models.OrganizationResponse, int32, error)
	GetOrganization(ctx context.Context, ouID string) (*models.OrganizationResponse, error)
	ListProjects(ctx context.Context, ouID string, limit int, offset int) ([]*models.ProjectResponse, int32, error)
	GetProject(ctx context.Context, ouID string, projectName string) (*models.ProjectResponse, error)
	CreateProject(ctx context.Context, ouID string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error)
	UpdateProject(ctx context.Context, ouID string, projectName string, payload spec.UpdateProjectRequest) (*models.ProjectResponse, error)
	DeleteProject(ctx context.Context, ouID string, projectName string) error
	DeleteOrgDeploymentPipeline(ctx context.Context, ouID, pipelineName string) error
	ListOrgDeploymentPipelines(ctx context.Context, ouID string, limit int, offset int) ([]*models.DeploymentPipelineResponse, int, error)
	GetDataplanes(ctx context.Context, ouID string) ([]*models.DataPlaneResponse, error)
}

type infraResourceManager struct {
	ocClient client.OpenChoreoClient
	logger   *slog.Logger
}

func NewInfraResourceManager(
	openChoreoClient client.OpenChoreoClient,
	logger *slog.Logger,
) InfraResourceManager {
	return &infraResourceManager{
		ocClient: openChoreoClient,
		logger:   logger,
	}
}

func (s *infraResourceManager) ListOrganizations(ctx context.Context, limit int, offset int) ([]*models.OrganizationResponse, int32, error) {
	s.logger.Debug("ListOrganizations called", "limit", limit, "offset", offset)

	// Fetch organizations from OpenChoreo
	orgs, err := s.ocClient.ListOrganizations(ctx)
	if err != nil {
		s.logger.Warn("Failed to list organizations from openchoreo", "error", err)
		return nil, 0, fmt.Errorf("failed to list organizations from OpenChoreo: %w", err)
	}
	s.logger.Debug("Retrieved organizations from openchoreo", "total_count", len(orgs))

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

func (s *infraResourceManager) GetOrganization(ctx context.Context, ouID string) (*models.OrganizationResponse, error) {
	s.logger.Debug("GetOrganization called", "ou_id", ouID)

	org, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to get organization from OpenChoreo", "ou_id", ouID, "error", err)
		return nil, err
	}

	s.logger.Info("Fetched organization successfully", "ou_id", ouID)
	return org, nil
}

func (s *infraResourceManager) CreateProject(ctx context.Context, ouID string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error) {
	s.logger.Debug("CreateProject called", "ou_id", ouID, "project_name", payload.Name, "deployment_pipeline", payload.DeploymentPipeline)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to get organization", "ou_id", ouID, "error", err)
		return nil, err
	}

	// Create project in OpenChoreo
	req := client.CreateProjectRequest{
		Name:               payload.Name,
		DisplayName:        payload.DisplayName,
		Description:        utils.StrPointerAsStr(payload.Description, ""),
		DeploymentPipeline: payload.DeploymentPipeline,
	}

	if err := s.ocClient.CreateProject(ctx, ouID, req); err != nil {
		s.logger.Warn("Failed to create project in OpenChoreo", "ou_id", ouID, "project_name", payload.Name, "error", err)
		return nil, err
	}
	s.logger.Info("Project created successfully", "ou_id", ouID, "project_name", payload.Name)

	// Provision the cell namespace for every environment the project can reach.
	// Best effort: the project itself exists and is usable, and the deploy and
	// promote paths ensure the binding for the environment they target, so a
	// failure here is recoverable rather than a reason to fail the request.
	s.ensureProjectReleaseBindings(ctx, ouID, payload.Name)

	return &models.ProjectResponse{
		Name:               payload.Name,
		OrgName:            ouID,
		DisplayName:        payload.DisplayName,
		Description:        utils.StrPointerAsStr(payload.Description, ""),
		CreatedAt:          time.Now(),
		DeploymentPipeline: payload.DeploymentPipeline,
	}, nil
}

// ensureProjectReleaseBindings creates a ProjectReleaseBinding for the project in
// every environment of its deployment pipeline, so the cell namespace exists
// before anything is deployed there. Failures are logged, not returned — see the
// call site for why.
func (s *infraResourceManager) ensureProjectReleaseBindings(ctx context.Context, ouID, projectName string) {
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to resolve deployment pipeline for project release bindings",
			"ou_id", ouID, "project_name", projectName, "error", err)
		return
	}

	for _, envName := range pipelineEnvironments(pipeline) {
		if err := s.ocClient.EnsureProjectReleaseBinding(ctx, ouID, projectName, envName); err != nil {
			s.logger.Error("Failed to ensure project release binding",
				"ou_id", ouID, "project_name", projectName, "environment", envName, "error", err)
			continue
		}
		s.logger.Debug("Ensured project release binding",
			"ou_id", ouID, "project_name", projectName, "environment", envName)
	}
}

// pipelineEnvironments returns every environment named by a pipeline's promotion
// paths — sources and targets alike — in a stable order and without duplicates.
func pipelineEnvironments(pipeline *models.DeploymentPipelineResponse) []string {
	if pipeline == nil {
		return nil
	}

	seen := make(map[string]struct{})
	envNames := make([]string, 0, len(pipeline.PromotionPaths))
	add := func(name string) {
		if name == "" {
			return
		}
		if _, ok := seen[name]; ok {
			return
		}
		seen[name] = struct{}{}
		envNames = append(envNames, name)
	}

	for _, path := range pipeline.PromotionPaths {
		add(path.SourceEnvironmentRef)
		for _, target := range path.TargetEnvironmentRefs {
			add(target.Name)
		}
	}
	return envNames
}

func (s *infraResourceManager) UpdateProject(ctx context.Context, ouID string, projectName string, payload spec.UpdateProjectRequest) (*models.ProjectResponse, error) {
	s.logger.Info("Updating project", "ou_id", ouID, "project_name", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to get organization", "ou_id", ouID, "error", err)
		return nil, err
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, ouID, projectName)
	if err != nil {
		s.logger.Warn("Failed to get project", "project_name", projectName, "ou_id", ouID, "error", err)
		return nil, err
	}
	// Todo: verify existence of deployment pipeline if deployment pipeline is being updated

	// Update project in OpenChoreo using PatchProject
	patchReq := client.PatchProjectRequest{
		DisplayName:        payload.DisplayName,
		Description:        payload.Description,
		DeploymentPipeline: payload.DeploymentPipeline,
	}
	if err := s.ocClient.PatchProject(ctx, ouID, projectName, patchReq); err != nil {
		s.logger.Warn("Failed to update project in OpenChoreo", "project_name", projectName, "ou_id", ouID, "error", err)
		return nil, fmt.Errorf("failed to update project: %w", err)
	}

	// Fetch updated project
	updatedProject, err := s.ocClient.GetProject(ctx, ouID, projectName)
	if err != nil {
		s.logger.Warn("Failed to fetch updated project", "project_name", projectName, "ou_id", ouID, "error", err)
		return nil, err
	}

	s.logger.Info("Project updated successfully", "ou_id", ouID, "project_name", projectName)

	return updatedProject, nil
}

func (s *infraResourceManager) ListProjects(ctx context.Context, ouID string, limit int, offset int) ([]*models.ProjectResponse, int32, error) {
	s.logger.Debug("ListProjects called", "ou_id", ouID, "limit", limit, "offset", offset)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to get organization", "ou_id", ouID, "error", err)
		return nil, 0, err
	}

	projects, err := s.ocClient.ListProjects(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to list projects", "ou_id", ouID, "error", err)
		return nil, 0, fmt.Errorf("failed to list projects for organization %s: %w", ouID, err)
	}
	s.logger.Debug("Retrieved projects", "ou_id", ouID, "total_count", len(projects))

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

	s.logger.Info("Fetched projects successfully", "ou_id", ouID, "count", len(paginatedProjects), "total", total)
	return paginatedProjects, int32(total), nil
}

func (s *infraResourceManager) DeleteProject(ctx context.Context, ouID string, projectName string) error {
	s.logger.Debug("DeleteProject called", "ou_id", ouID, "project_name", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to get organization", "ou_id", ouID, "error", err)
		return err
	}
	// Check agents exist for the project
	s.logger.Debug("Checking for associated agents", "project_name", projectName)
	agents, err := s.ocClient.ListComponents(ctx, ouID, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			s.logger.Warn("Project not found while listing components; delete is idempotent", "ou_id", ouID, "project_name", projectName)
			return nil
		}
		s.logger.Warn("Failed to list agents for project", "project_name", projectName, "error", err)
		return err
	}
	if len(agents) > 0 {
		s.logger.Warn("Cannot delete project with associated agents", "ou_id", ouID, "project_name", projectName, "agent_count", len(agents))
		return utils.ErrProjectHasAssociatedAgents
	}
	s.logger.Debug("No associated agents found, proceeding with deletion", "project_name", projectName)

	// Delete project from OpenChoreo
	deleteAttempt, auditErr := audit.Begin(
		ctx, audit.ActionProjectDelete,
		audit.Org(ouID),
		audit.ResourceNamed("project", projectName, projectName),
		audit.Project(projectName),
		audit.Detail("projectName", projectName),
	)
	if auditErr != nil {
		s.logger.Error("Refusing to delete project: audit record could not be written",
			"project_name", projectName, "error", auditErr)
		return auditErr
	}

	err = s.ocClient.DeleteProject(ctx, ouID, projectName)
	deleteAttempt.Complete(ctx, err)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			s.logger.Warn("Project not found during deletion, delete is idempotent", "ou_id", ouID, "project_name", projectName)
			return nil
		}
		s.logger.Warn("Failed to delete project from OpenChoreo", "ou_id", ouID, "project_name", projectName, "error", err)
		return err
	}
	s.logger.Info("Project deleted successfully", "ou_id", ouID, "project_name", projectName)
	return nil
}

func (s *infraResourceManager) GetProject(ctx context.Context, ouID string, projectName string) (*models.ProjectResponse, error) {
	s.logger.Debug("GetProject called", "ou_id", ouID, "project_name", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to get organization", "ou_id", ouID, "error", err)
		return nil, err
	}

	project, err := s.ocClient.GetProject(ctx, ouID, projectName)
	if err != nil {
		s.logger.Warn("Failed to get project from OpenChoreo", "ou_id", ouID, "project_name", projectName, "error", err)
		return nil, err
	}

	s.logger.Info("Fetched project successfully", "ou_id", ouID, "project_name", projectName)
	return project, nil
}

func (s *infraResourceManager) ListOrgDeploymentPipelines(ctx context.Context, ouID string, limit int, offset int) ([]*models.DeploymentPipelineResponse, int, error) {
	s.logger.Debug("ListOrgDeploymentPipelines called", "ou_id", ouID)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to get organization", "ou_id", ouID, "error", err)
		return nil, 0, err
	}

	s.logger.Debug("Fetching deployment pipelines from OpenChoreo", "ou_id", ouID)
	deploymentPipelines, err := s.ocClient.ListDeploymentPipelines(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to get deployment pipelines from OpenChoreo", "ou_id", ouID, "error", err)
		return nil, 0, fmt.Errorf("failed to get deployment pipelines for organization %s: %w", ouID, err)
	}

	s.logger.Info("Fetched deployment pipelines successfully", "ou_id", ouID, "count", len(deploymentPipelines))
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

func (s *infraResourceManager) ListOrgEnvironments(ctx context.Context, ouID string) ([]*models.EnvironmentResponse, error) {
	s.logger.Debug("ListOrgEnvironments called", "ou_id", ouID)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to get organization from OpenChoreo", "ou_id", ouID, "error", err)
		return nil, err
	}
	s.logger.Debug("Fetching environments from OpenChoreo", "ou_id", ouID)
	environments, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to get environments from OpenChoreo", "ou_id", ouID, "error", err)
		return nil, err
	}

	s.logger.Info("Fetched environments successfully", "ou_id", ouID, "count", len(environments))
	return environments, nil
}

func (s *infraResourceManager) GetProjectDeploymentPipeline(ctx context.Context, ouID string, projectName string) (*models.DeploymentPipelineResponse, error) {
	s.logger.Debug("GetProjectDeploymentPipeline called", "ou_id", ouID, "project_name", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to get organization", "ou_id", ouID, "error", err)
		return nil, err
	}

	s.logger.Debug("Fetching deployment pipeline from OpenChoreo", "ou_id", ouID, "project_name", projectName)
	deploymentPipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if err != nil {
		s.logger.Warn("Failed to get deployment pipeline from OpenChoreo", "ou_id", ouID, "project_name", projectName, "error", err)
		return nil, err
	}

	s.logger.Info("Fetched deployment pipeline successfully", "ou_id", ouID, "project_name", projectName)

	return deploymentPipeline, nil
}

func (s *infraResourceManager) CreateOrgDeploymentPipeline(ctx context.Context, ouID string, displayName string, description *string, projectName *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error) {
	s.logger.Info("Creating deployment pipeline", "ou_id", ouID, "display_name", displayName)

	pipelineName := slugify(displayName) // slugify is defined in evaluator_manager.go
	if pipelineName == "" {
		return nil, fmt.Errorf("invalid display name: cannot derive a valid pipeline name")
	}

	created, err := s.ocClient.CreateDeploymentPipeline(ctx, ouID, pipelineName, &displayName, description, promotionPaths)
	if err != nil {
		s.logger.Warn("Failed to create deployment pipeline", "ou_id", ouID, "error", err)
		return nil, err
	}

	// If a projectName was provided, link the newly created pipeline as the project's deploymentPipelineRef.
	// OpenChoreo's DeploymentPipeline model has no projectName; the project↔pipeline link is represented
	// via Project.spec.deploymentPipelineRef and must be set separately.
	if projectName != nil && *projectName != "" {
		project, getErr := s.ocClient.GetProject(ctx, ouID, *projectName)
		if getErr != nil {
			s.logger.Error("Failed to fetch project for pipeline linkage", "ou_id", ouID, "project_name", *projectName, "error", getErr)
			return nil, fmt.Errorf("failed to link deployment pipeline to project: %w", getErr)
		}
		if patchErr := s.ocClient.PatchProject(ctx, ouID, *projectName, client.PatchProjectRequest{
			DisplayName:        project.DisplayName,
			Description:        project.Description,
			DeploymentPipeline: pipelineName,
		}); patchErr != nil {
			s.logger.Error("Failed to patch project with deployment pipeline ref", "ou_id", ouID, "project_name", *projectName, "pipeline_name", pipelineName, "error", patchErr)
			return nil, fmt.Errorf("failed to link deployment pipeline to project: %w", patchErr)
		}
	}

	s.logger.Info("Deployment pipeline created successfully", "ou_id", ouID, "pipeline_name", pipelineName)
	return created, nil
}

func (s *infraResourceManager) UpdateOrgDeploymentPipeline(ctx context.Context, ouID string, pipelineName string, displayName *string, description *string, promotionPaths []models.PromotionPath) (*models.DeploymentPipelineResponse, error) {
	s.logger.Info("Updating deployment pipeline", "ou_id", ouID, "pipeline_name", pipelineName)
	updated, err := s.ocClient.UpdateDeploymentPipeline(ctx, ouID, pipelineName, displayName, description, promotionPaths)
	if err != nil {
		s.logger.Warn("Failed to update deployment pipeline", "ou_id", ouID, "pipeline_name", pipelineName, "error", err)
		return nil, err
	}
	s.logger.Info("Deployment pipeline updated successfully", "ou_id", ouID, "pipeline_name", pipelineName)
	return updated, nil
}

func (s *infraResourceManager) DeleteOrgDeploymentPipeline(ctx context.Context, ouID string, pipelineName string) error {
	s.logger.Info("Deleting deployment pipeline", "ou_id", ouID, "pipeline_name", pipelineName)

	// Block deletion if any project still references this deployment pipeline.
	s.logger.Debug("Checking for projects referencing the deployment pipeline", "ou_id", ouID, "pipeline_name", pipelineName)
	projects, err := s.ocClient.ListProjects(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to list projects while checking deployment pipeline references", "ou_id", ouID, "pipeline_name", pipelineName, "error", err)
		return fmt.Errorf("failed to verify deployment pipeline references: %w", err)
	}
	var referencingProjects []string
	for _, project := range projects {
		if project != nil && project.DeploymentPipeline == pipelineName {
			referencingProjects = append(referencingProjects, project.Name)
		}
	}
	if len(referencingProjects) > 0 {
		s.logger.Warn("Cannot delete deployment pipeline referenced by projects", "ou_id", ouID, "pipeline_name", pipelineName, "projects", referencingProjects)
		return fmt.Errorf("%w: %v", utils.ErrDeploymentPipelineInUse, referencingProjects)
	}

	if err := s.ocClient.DeleteOrgDeploymentPipeline(ctx, ouID, pipelineName); err != nil {
		s.logger.Warn("Failed to delete deployment pipeline", "ou_id", ouID, "pipeline_name", pipelineName, "error", err)
		return fmt.Errorf("failed to delete deployment pipeline: %w", err)
	}

	s.logger.Info("Deployment pipeline deleted successfully", "ou_id", ouID, "pipeline_name", pipelineName)
	return nil
}

func (s *infraResourceManager) GetDataplanes(ctx context.Context, ouID string) ([]*models.DataPlaneResponse, error) {
	s.logger.Debug("GetDataplanes called", "ou_id", ouID)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to get organization", "ou_id", ouID, "error", err)
		return nil, err
	}

	s.logger.Debug("Fetching dataplanes from OpenChoreo")
	dataplanes, err := s.ocClient.ListDataPlanes(ctx)
	if err != nil {
		s.logger.Warn("Failed to get dataplanes from OpenChoreo", "error", err)
		return nil, err
	}

	s.logger.Info("Fetched dataplanes successfully", "count", len(dataplanes))
	return dataplanes, nil
}
