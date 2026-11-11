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

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type InfraResourceManager interface {
	ListOrgEnvironments(ctx context.Context, ouID string) ([]*models.EnvironmentResponse, error)
	ListOrganizations(ctx context.Context, limit int, offset int) ([]*models.OrganizationResponse, int32, error)
	GetOrganization(ctx context.Context, ouID string) (*models.OrganizationResponse, error)
	ListProjects(ctx context.Context, ouID string, limit int, offset int) ([]*models.ProjectResponse, int32, error)
	GetProject(ctx context.Context, ouID string, projectName string) (*models.ProjectResponse, error)
	CreateProject(ctx context.Context, ouID string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error)
	UpdateProject(ctx context.Context, ouID string, projectName string, payload spec.UpdateProjectRequest) (*models.ProjectResponse, error)
	DeleteProject(ctx context.Context, ouID string, projectName string) error
}

type infraResourceManager struct {
	platform repositories.PlatformRepository
	logger   *slog.Logger
}

func NewInfraResourceManager(
	platform repositories.PlatformRepository,
	logger *slog.Logger,
) InfraResourceManager {
	return &infraResourceManager{
		platform: platform,
		logger:   logger,
	}
}

func (s *infraResourceManager) ListOrganizations(ctx context.Context, limit int, offset int) ([]*models.OrganizationResponse, int32, error) {
	s.logger.Debug("ListOrganizations called", "limit", limit, "offset", offset)

	// Fetch organizations
	orgs, err := s.platform.ListOrganizations(ctx)
	if err != nil {
		s.logger.Error("Failed to list organizations", "error", err)
		return nil, 0, fmt.Errorf("failed to list organizations: %w", err)
	}
	s.logger.Debug("Retrieved organizations", "totalCount", len(orgs))

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
	s.logger.Debug("GetOrganization called", "ouID", ouID)

	org, err := s.platform.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to get organization", "ouID", ouID, "error", err)
		return nil, err
	}

	s.logger.Info("Fetched organization successfully", "ouID", ouID)
	return org, nil
}

// CreateProject creates a project. The deploymentPipeline field is accepted
// and ignored: every project deploys to the organization's default environment.
func (s *infraResourceManager) CreateProject(ctx context.Context, ouID string, payload spec.CreateProjectRequest) (*models.ProjectResponse, error) {
	in := repositories.CreateProjectInput{
		Name:        payload.Name,
		DisplayName: payload.DisplayName,
		Description: utils.StrPointerAsStr(payload.Description, ""),
	}
	if claims := jwtassertion.GetTokenClaims(ctx); claims != nil {
		in.CreatedBy = claims.Sub
	}
	project, err := s.platform.CreateProject(ctx, ouID, in)
	if err != nil {
		return nil, err
	}
	s.logger.Info("Project created", "ouID", ouID, "projectName", payload.Name)
	return project, nil
}

func (s *infraResourceManager) UpdateProject(ctx context.Context, ouID string, projectName string, payload spec.UpdateProjectRequest) (*models.ProjectResponse, error) {
	if err := s.platform.PatchProject(ctx, ouID, projectName, &payload.DisplayName, &payload.Description); err != nil {
		return nil, err
	}
	return s.platform.GetProject(ctx, ouID, projectName)
}

func (s *infraResourceManager) ListProjects(ctx context.Context, ouID string, limit int, offset int) ([]*models.ProjectResponse, int32, error) {
	s.logger.Debug("ListProjects called", "ouID", ouID, "limit", limit, "offset", offset)

	// Validate organization exists
	_, err := s.platform.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to get organization", "ouID", ouID, "error", err)
		return nil, 0, err
	}

	projects, err := s.platform.ListProjects(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to list projects", "ouID", ouID, "error", err)
		return nil, 0, fmt.Errorf("failed to list projects for organization %s: %w", ouID, err)
	}
	s.logger.Debug("Retrieved projects", "ouID", ouID, "totalCount", len(projects))

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

	s.logger.Info("Fetched projects successfully", "ouID", ouID, "count", len(paginatedProjects), "total", total)
	return paginatedProjects, int32(total), nil
}

func (s *infraResourceManager) DeleteProject(ctx context.Context, ouID string, projectName string) error {
	s.logger.Debug("DeleteProject called", "ouID", ouID, "projectName", projectName)

	// Validate organization exists
	_, err := s.platform.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to get organization", "ouID", ouID, "error", err)
		return err
	}
	// Refuse to delete a project that still holds agents.
	s.logger.Debug("Checking for components in the project", "ouID", ouID, "projectName", projectName)
	componentCount, err := s.platform.CountProjectAgents(ctx, ouID, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			s.logger.Warn("Project not found while listing components; delete is idempotent", "ouID", ouID, "projectName", projectName)
			return nil
		}
		s.logger.Error("Failed to count components for project", "ouID", ouID, "projectName", projectName, "error", err)
		return err
	}
	if componentCount > 0 {
		s.logger.Warn("Cannot delete project that still has components", "ouID", ouID, "projectName", projectName, "componentCount", componentCount)
		return utils.ErrProjectHasAssociatedAgents
	}
	s.logger.Debug("No components found, proceeding with deletion", "ouID", ouID, "projectName", projectName)

	// Delete project
	deleteAttempt, auditErr := audit.Begin(
		ctx, audit.ActionProjectDelete,
		audit.Org(ouID),
		audit.ResourceNamed("project", projectName, projectName),
		audit.Project(projectName),
		audit.Detail("projectName", projectName),
	)
	if auditErr != nil {
		s.logger.Error("Refusing to delete project: audit record could not be written",
			"projectName", projectName, "error", auditErr)
		return auditErr
	}

	err = s.platform.DeleteProject(ctx, ouID, projectName)
	deleteAttempt.Complete(ctx, err)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			s.logger.Warn("Project not found during deletion, delete is idempotent", "ouID", ouID, "projectName", projectName)
			return nil
		}
		s.logger.Error("Failed to delete project", "ouID", ouID, "projectName", projectName, "error", err)
		return err
	}
	s.logger.Info("Project deleted successfully", "ouID", ouID, "projectName", projectName)
	return nil
}

func (s *infraResourceManager) GetProject(ctx context.Context, ouID string, projectName string) (*models.ProjectResponse, error) {
	s.logger.Debug("GetProject called", "ouID", ouID, "projectName", projectName)

	// Validate organization exists
	_, err := s.platform.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to get organization", "ouID", ouID, "error", err)
		return nil, err
	}

	project, err := s.platform.GetProject(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to get project", "ouID", ouID, "projectName", projectName, "error", err)
		return nil, err
	}

	s.logger.Info("Fetched project successfully", "ouID", ouID, "projectName", projectName)
	return project, nil
}

func (s *infraResourceManager) ListOrgEnvironments(ctx context.Context, ouID string) ([]*models.EnvironmentResponse, error) {
	s.logger.Debug("ListOrgEnvironments called", "ouID", ouID)

	// Validate organization exists
	_, err := s.platform.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to get organization", "ouID", ouID, "error", err)
		return nil, err
	}
	s.logger.Debug("Fetching environments", "ouID", ouID)
	environments, err := s.platform.ListEnvironments(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to get environments", "ouID", ouID, "error", err)
		return nil, err
	}

	s.logger.Info("Fetched environments successfully", "ouID", ouID, "count", len(environments))
	return environments, nil
}
