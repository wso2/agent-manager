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
	"fmt"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// APIPublicationRepository defines operations for API publication tracking
type APIPublicationRepository interface {
	// Basic CRUD operations
	Create(publication *models.APIPublication) error
	GetByAPIAndDevPortal(apiUUID, devPortalUUID, orgUUID string) (*models.APIPublication, error)
	GetByAPIUUID(apiUUID, orgUUID string) ([]*models.APIPublication, error)
	Update(publication *models.APIPublication) error
	Delete(apiUUID, devPortalUUID, orgUUID string) error
	UpsertPublication(publication *models.APIPublication) error
	GetAPIDevPortalsWithDetails(apiUUID, orgUUID string) ([]*models.APIDevPortalWithDetails, error)
}

// APIPublicationRepo implements the APIPublicationRepository interface using GORM
type APIPublicationRepo struct {
	db *gorm.DB
}

// NewAPIPublicationRepository creates a new API publication repository
func NewAPIPublicationRepository(db *gorm.DB) APIPublicationRepository {
	return &APIPublicationRepo{db: db}
}

// UpsertPublication creates or updates a publication record
func (r *APIPublicationRepo) UpsertPublication(publication *models.APIPublication) error {
	publication.UpdatedAt = time.Now()
	if publication.CreatedAt.IsZero() {
		publication.CreatedAt = time.Now()
	}

	// Use GORM's Clauses for upsert (ON CONFLICT)
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "api_uuid"}, {Name: "devportal_uuid"}, {Name: "organization_uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "api_version", "devportal_ref_id",
			"sandbox_endpoint_url", "production_endpoint_url", "updated_at",
		}),
	}).Create(publication).Error
}

// Create creates a new API publication record
func (r *APIPublicationRepo) Create(publication *models.APIPublication) error {
	publication.CreatedAt = time.Now()
	publication.UpdatedAt = time.Now()
	return r.db.Create(publication).Error
}

// GetByAPIAndDevPortal retrieves an API publication by API and DevPortal UUIDs
func (r *APIPublicationRepo) GetByAPIAndDevPortal(apiUUID, devPortalUUID, orgUUID string) (*models.APIPublication, error) {
	var publication models.APIPublication
	err := r.db.Where("api_uuid = ? AND devportal_uuid = ? AND organization_uuid = ?",
		apiUUID, devPortalUUID, orgUUID).
		First(&publication).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("API publication not found")
		}
		return nil, fmt.Errorf("failed to get API publication: %w", err)
	}

	return &publication, nil
}

// GetByAPIUUID retrieves all API publications for a specific API and organization
func (r *APIPublicationRepo) GetByAPIUUID(apiUUID, orgUUID string) ([]*models.APIPublication, error) {
	var publications []*models.APIPublication
	err := r.db.Where("api_uuid = ? AND organization_uuid = ?", apiUUID, orgUUID).
		Order("created_at DESC").
		Find(&publications).Error
	return publications, err
}

// Update updates an existing API publication
func (r *APIPublicationRepo) Update(publication *models.APIPublication) error {
	publication.UpdatedAt = time.Now()

	result := r.db.Model(&models.APIPublication{}).
		Where("api_uuid = ? AND devportal_uuid = ? AND organization_uuid = ?",
			publication.APIUUID, publication.DevPortalUUID, publication.OrganizationUUID).
		Updates(map[string]interface{}{
			"status":                  publication.Status,
			"api_version":             publication.APIVersion,
			"devportal_ref_id":        publication.DevPortalRefID,
			"sandbox_endpoint_url":    publication.SandboxEndpointURL,
			"production_endpoint_url": publication.ProductionEndpointURL,
			"updated_at":              publication.UpdatedAt,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update API publication: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		// Verify if the row still exists (RowsAffected can be 0 for no-op updates)
		var exists bool
		err := r.db.Model(&models.APIPublication{}).
			Select("1").
			Where("api_uuid = ? AND devportal_uuid = ? AND organization_uuid = ?",
				publication.APIUUID, publication.DevPortalUUID, publication.OrganizationUUID).
			Limit(1).
			Find(&exists).Error

		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return errors.New("API publication not found")
			}
			return fmt.Errorf("failed to verify API publication existence: %w", err)
		}
		// Row exists but no changes were made - this is OK (idempotent update)
	}

	return nil
}

// Delete removes a publication record
func (r *APIPublicationRepo) Delete(apiUUID, devPortalUUID, orgUUID string) error {
	result := r.db.Where("api_uuid = ? AND devportal_uuid = ? AND organization_uuid = ?",
		apiUUID, devPortalUUID, orgUUID).
		Delete(&models.APIPublication{})

	if result.Error != nil {
		return fmt.Errorf("failed to delete API publication: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		return errors.New("API publication not found")
	}

	return nil
}

// GetAPIDevPortalsWithDetails retrieves all DevPortals associated with an API including publication details
func (r *APIPublicationRepo) GetAPIDevPortalsWithDetails(apiUUID, orgUUID string) ([]*models.APIDevPortalWithDetails, error) {
	var devPortals []*models.APIDevPortalWithDetails
	err := r.db.Table("association_mappings aa").
		Select("d.uuid, d.organization_uuid, d.name, d.identifier, d.api_url, d.hostname, "+
			"d.is_active, d.is_enabled, d.is_default, d.visibility, d.description, "+
			"d.created_at, d.updated_at, "+
			"aa.created_at as associated_at, aa.updated_at as association_updated_at, "+
			"CASE WHEN ap.api_uuid IS NOT NULL THEN 1 ELSE 0 END as is_published, "+
			"ap.status as publication_status, ap.api_version, ap.devportal_ref_id, "+
			"ap.sandbox_endpoint_url, ap.production_endpoint_url, "+
			"ap.created_at as published_at, ap.updated_at as publication_updated_at").
		Joins("INNER JOIN devportals d ON aa.resource_uuid = d.uuid").
		Joins("LEFT JOIN publication_mappings ap ON ap.api_uuid = aa.artifact_uuid "+
			"AND ap.devportal_uuid = aa.resource_uuid "+
			"AND ap.organization_uuid = aa.organization_uuid").
		Where("aa.artifact_uuid = ? AND aa.organization_uuid = ? AND aa.association_type = ?",
			apiUUID, orgUUID, "dev_portal").
		Order("aa.created_at DESC").
		Scan(&devPortals).Error

	return devPortals, err
}
