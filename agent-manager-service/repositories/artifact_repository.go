// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
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
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// ArtifactRepository defines the interface for artifact data access
type ArtifactRepository interface {
	Create(tx *gorm.DB, artifact *models.Artifact) error
	Delete(tx *gorm.DB, uuid string) error
	Update(tx *gorm.DB, artifact *models.Artifact) error
	UpdateCatalogStatus(tx *gorm.DB, uuid, organizationUUID string, inCatalog bool) error
	Exists(kind, handle, orgUUID string) (bool, error)
	GetByHandle(handle, orgUUID string) (*models.Artifact, error)
	CountByKindAndOrg(kind, orgUUID string) (int, error)
}

// ArtifactRepo implements ArtifactRepository using GORM
type ArtifactRepo struct {
	db *gorm.DB
}

// NewArtifactRepo creates a new artifact repository
func NewArtifactRepo(db *gorm.DB) ArtifactRepository {
	return &ArtifactRepo{db: db}
}

// Create inserts a new artifact within a transaction
func (r *ArtifactRepo) Create(tx *gorm.DB, artifact *models.Artifact) error {
	now := time.Now()
	artifact.CreatedAt = now
	artifact.UpdatedAt = now
	return tx.Create(artifact).Error
}

// Delete removes an artifact within a transaction
func (r *ArtifactRepo) Delete(tx *gorm.DB, uuid string) error {
	result := tx.Where("uuid = ?", uuid).Delete(&models.Artifact{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Update modifies an artifact within a transaction
func (r *ArtifactRepo) Update(tx *gorm.DB, artifact *models.Artifact) error {
	artifact.UpdatedAt = time.Now()
	result := tx.Model(&models.Artifact{}).
		Where("uuid = ? AND organization_name = ?", artifact.UUID, artifact.OrganizationName).
		Updates(map[string]interface{}{
			"name":       artifact.Name,
			"version":    artifact.Version,
			"updated_at": artifact.UpdatedAt,
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

// Exists checks if an artifact exists with the given kind, handle and organization
func (r *ArtifactRepo) Exists(kind, handle, orgUUID string) (bool, error) {
	var count int64
	err := r.db.Model(&models.Artifact{}).
		Where("kind = ? AND handle = ? AND organization_name = ?", kind, handle, orgUUID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetByHandle retrieves an artifact by handle and organization
func (r *ArtifactRepo) GetByHandle(handle, orgUUID string) (*models.Artifact, error) {
	var artifact models.Artifact
	err := r.db.Where("handle = ? AND organization_name = ?", handle, orgUUID).
		First(&artifact).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrArtifactNotFound
		}
		return nil, err
	}
	return &artifact, nil
}

// CountByKindAndOrg counts artifacts by kind and organization
func (r *ArtifactRepo) CountByKindAndOrg(kind, orgUUID string) (int, error) {
	var count int64
	err := r.db.Model(&models.Artifact{}).
		Where("kind = ? AND organization_name = ?", kind, orgUUID).
		Count(&count).Error
	return int(count), err
}

// UpdateCatalogStatus updates the in_catalog field for an artifact
func (r *ArtifactRepo) UpdateCatalogStatus(tx *gorm.DB, uuid, organizationUUID string, inCatalog bool) error {
	result := tx.Model(&models.Artifact{}).
		Where("uuid = ? AND organization_name = ?", uuid, organizationUUID).
		Updates(map[string]interface{}{
			"in_catalog": inCatalog,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}
