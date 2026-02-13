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

// OrganizationRepository defines the interface for organization data access
type OrganizationRepository interface {
	CreateOrganization(org *models.Organization) error
	GetOrganizationByIdOrHandle(id, handle string) (*models.Organization, error)
	GetOrganizationByUUID(orgId string) (*models.Organization, error)
	GetOrganizationByHandle(handle string) (*models.Organization, error)
	UpdateOrganization(org *models.Organization) error
	DeleteOrganization(orgId string) error
	ListOrganizations(limit, offset int) ([]*models.Organization, error)
}

// OrganizationRepo implements OrganizationRepository using GORM
type OrganizationRepo struct {
	db *gorm.DB
}

// NewOrganizationRepo creates a new organization repository
func NewOrganizationRepo(db *gorm.DB) OrganizationRepository {
	return &OrganizationRepo{db: db}
}

// CreateOrganization inserts a new organization
func (r *OrganizationRepo) CreateOrganization(org *models.Organization) error {
	now := time.Now()
	org.CreatedAt = now
	org.UpdatedAt = now
	return r.db.Create(org).Error
}

// GetOrganizationByIdOrHandle retrieves an organization by id or handle
func (r *OrganizationRepo) GetOrganizationByIdOrHandle(id, handle string) (*models.Organization, error) {
	var org models.Organization
	err := r.db.Where("uuid = ? OR handle = ?", id, handle).First(&org).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &org, nil
}

// GetOrganizationByUUID retrieves an organization by ID
func (r *OrganizationRepo) GetOrganizationByUUID(orgId string) (*models.Organization, error) {
	var org models.Organization
	err := r.db.Where("uuid = ?", orgId).First(&org).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &org, nil
}

// GetOrganizationByHandle retrieves an organization by handle
func (r *OrganizationRepo) GetOrganizationByHandle(handle string) (*models.Organization, error) {
	var org models.Organization
	err := r.db.Where("handle = ?", handle).First(&org).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &org, nil
}

// UpdateOrganization modifies an existing organization
func (r *OrganizationRepo) UpdateOrganization(org *models.Organization) error {
	org.UpdatedAt = time.Now()
	return r.db.Model(&models.Organization{}).
		Where("uuid = ?", org.UUID).
		Updates(map[string]interface{}{
			"handle":     org.Handle,
			"name":       org.Name,
			"region":     org.Region,
			"updated_at": org.UpdatedAt,
		}).Error
}

// DeleteOrganization removes an organization
func (r *OrganizationRepo) DeleteOrganization(orgId string) error {
	return r.db.Where("uuid = ?", orgId).Delete(&models.Organization{}).Error
}

// ListOrganizations retrieves organizations with pagination
func (r *OrganizationRepo) ListOrganizations(limit, offset int) ([]*models.Organization, error) {
	var organizations []*models.Organization
	err := r.db.Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&organizations).Error
	return organizations, err
}
