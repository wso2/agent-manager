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
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ArtifactRepository defines the interface for artifact data access
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/artifact_repository_mock.go . ArtifactRepository:ArtifactRepositoryMock
type ArtifactRepository interface {
	Create(tx *gorm.DB, artifact *models.Artifact) error
	Delete(tx *gorm.DB, uuid string) error
	Update(tx *gorm.DB, artifact *models.Artifact) error
	UpdateCatalogStatus(tx *gorm.DB, uuid, organizationUUID string, inCatalog bool) error
	Exists(kind, handle, orgUUID string) (bool, error)
	GetByHandle(handle, orgUUID string) (*models.Artifact, error)
	GetByUUID(artifactUUID, orgUUID string) (*models.Artifact, error)
	ListByHandlePrefix(ctx context.Context, kind, handlePrefix, orgUUID string) ([]*models.Artifact, error)
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
	updates := map[string]interface{}{
		"updated_at": artifact.UpdatedAt,
	}
	if artifact.Name != "" {
		updates["name"] = artifact.Name
	}
	if artifact.Version != "" {
		updates["version"] = artifact.Version
	}

	result := tx.Model(&models.Artifact{}).
		Where("uuid = ? AND ou_id = ?", artifact.UUID, artifact.OUID).
		Updates(updates)

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
		Where("kind = ? AND handle = ? AND ou_id = ?", kind, handle, orgUUID).
		Count(&count).Error
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetByHandle retrieves an artifact by handle and organization
func (r *ArtifactRepo) GetByHandle(handle, orgUUID string) (*models.Artifact, error) {
	var artifact models.Artifact
	err := r.db.Where("handle = ? AND ou_id = ?", handle, orgUUID).
		First(&artifact).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrArtifactNotFound
		}
		return nil, err
	}
	return &artifact, nil
}

// GetByUUID retrieves an artifact by its stable UUID and organization. Returns
// utils.ErrArtifactNotFound when no matching row exists.
func (r *ArtifactRepo) GetByUUID(artifactUUID, orgUUID string) (*models.Artifact, error) {
	var artifact models.Artifact
	err := r.db.Where("uuid = ? AND ou_id = ?", artifactUUID, orgUUID).
		First(&artifact).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrArtifactNotFound
		}
		return nil, err
	}
	return &artifact, nil
}

// ListByHandlePrefix returns the org's artifacts of a kind whose handle starts with handlePrefix.
func (r *ArtifactRepo) ListByHandlePrefix(ctx context.Context, kind, handlePrefix, orgUUID string) ([]*models.Artifact, error) {
	var artifacts []*models.Artifact
	err := r.db.WithContext(ctx).
		Where(`kind = ? AND ou_id = ? AND handle LIKE ? ESCAPE '\'`, kind, orgUUID, escapeLikeWildcards(handlePrefix)+"%").
		Find(&artifacts).Error
	return artifacts, err
}

// CountByKindAndOrg counts artifacts by kind and organization
func (r *ArtifactRepo) CountByKindAndOrg(kind, orgUUID string) (int, error) {
	var count int64
	err := r.db.Model(&models.Artifact{}).
		Where("kind = ? AND ou_id = ?", kind, orgUUID).
		Count(&count).Error
	return int(count), err
}

// UpdateCatalogStatus updates the in_catalog field for an artifact
func (r *ArtifactRepo) UpdateCatalogStatus(tx *gorm.DB, uuid, organizationUUID string, inCatalog bool) error {
	result := tx.Model(&models.Artifact{}).
		Where("uuid = ? AND ou_id = ?", uuid, organizationUUID).
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
