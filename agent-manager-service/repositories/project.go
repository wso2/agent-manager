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

package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

type ProjectRepository interface {
	ListProjects(ctx context.Context, orgId uuid.UUID) ([]models.Project, error)
	GetProjectByName(ctx context.Context, orgId uuid.UUID, projectName string) (*models.Project, error)
	CreateProject(ctx context.Context, project *models.Project) error
	SoftDeleteProject(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID) error
	HardDeleteProject(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID) error
}

type projectRepository struct{}

func NewProjectRepository() ProjectRepository {
	return &projectRepository{}
}

func (r *projectRepository) ListProjects(ctx context.Context, orgId uuid.UUID) ([]models.Project, error) {
	var projects []models.Project
	if err := db.DB(ctx).Where("org_id = ?", orgId).Order("created_at DESC").Find(&projects).Error; err != nil {
		return nil, fmt.Errorf("projectRepository.ListProjects: %w", err)
	}

	return projects, nil
}

func (r *projectRepository) GetProjectByName(ctx context.Context, orgId uuid.UUID, projectName string) (*models.Project, error) {
	var project models.Project
	if err := db.DB(ctx).Where("org_id = ? AND name = ?", orgId, projectName).First(&project).Error; err != nil {
		return nil, fmt.Errorf("projectRepository.GetProjectByName: %w", err)
	}
	return &project, nil
}

func (r *projectRepository) CreateProject(ctx context.Context, project *models.Project) error {
	if err := db.DB(ctx).Create(project).Error; err != nil {
		return fmt.Errorf("projectRepository.CreateProject: %w", err)
	}
	return nil
}

func (r *projectRepository) SoftDeleteProject(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID) error {
	if err := db.DB(ctx).Where("id = ? AND org_id = ?", projectId, orgId).Delete(&models.Project{}).Error; err != nil {
		return fmt.Errorf("projectRepository.DeleteProject: %w", err)
	}
	return nil
}

func (r *projectRepository) HardDeleteProject(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID) error {
	if err := db.DB(ctx).Unscoped().Where("id = ? AND org_id = ?", projectId, orgId).Delete(&models.Project{}).Error; err != nil {
		return fmt.Errorf("projectRepository.HardDeleteProject: %w", err)
	}
	return nil
}
