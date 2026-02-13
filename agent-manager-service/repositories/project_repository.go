/*
 *  Copyright (c) 2025, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package repositories

import (
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// ProjectRepository defines the interface for project data access
type ProjectRepository interface {
	CreateProject(project *models.Project) error
	GetProjectByUUID(projectId string) (*models.Project, error)
	GetProjectByNameAndOrgID(name, orgID string) (*models.Project, error)
	GetProjectsByOrganizationID(orgID string) ([]*models.Project, error)
	UpdateProject(project *models.Project) error
	DeleteProject(projectId string) error
	ListProjects(orgID string, limit, offset int) ([]*models.Project, error)
}

// ProjectRepo implements ProjectRepository using GORM
type ProjectRepo struct {
	db *gorm.DB
}

// NewProjectRepo creates a new project repository
func NewProjectRepo(db *gorm.DB) ProjectRepository {
	return &ProjectRepo{db: db}
}

// CreateProject inserts a new project
func (r *ProjectRepo) CreateProject(project *models.Project) error {
	project.CreatedAt = time.Now()
	project.UpdatedAt = time.Now()
	return r.db.Create(project).Error
}

// GetProjectByUUID retrieves a project by ID
func (r *ProjectRepo) GetProjectByUUID(projectId string) (*models.Project, error) {
	var project models.Project
	err := r.db.Where("uuid = ?", projectId).First(&project).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &project, nil
}

// GetProjectByNameAndOrgID retrieves a project by name within an organization
func (r *ProjectRepo) GetProjectByNameAndOrgID(name, orgID string) (*models.Project, error) {
	var project models.Project
	err := r.db.Where("name = ? AND organization_uuid = ?", name, orgID).First(&project).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &project, nil
}

// GetProjectsByOrganizationID retrieves all projects for an organization
func (r *ProjectRepo) GetProjectsByOrganizationID(orgID string) ([]*models.Project, error) {
	var projects []*models.Project
	err := r.db.Where("organization_uuid = ?", orgID).
		Order("created_at DESC").
		Find(&projects).Error
	return projects, err
}

// UpdateProject modifies an existing project
func (r *ProjectRepo) UpdateProject(project *models.Project) error {
	project.UpdatedAt = time.Now()
	return r.db.Model(&models.Project{}).
		Where("uuid = ?", project.ID).
		Updates(map[string]interface{}{
			"name":        project.Name,
			"description": project.Description,
			"updated_at":  project.UpdatedAt,
		}).Error
}

// DeleteProject removes a project
func (r *ProjectRepo) DeleteProject(projectId string) error {
	return r.db.Where("uuid = ?", projectId).Delete(&models.Project{}).Error
}

// ListProjects retrieves projects with pagination
func (r *ProjectRepo) ListProjects(orgID string, limit, offset int) ([]*models.Project, error) {
	var projects []*models.Project
	err := r.db.Where("organization_uuid = ?", orgID).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&projects).Error
	return projects, err
}
