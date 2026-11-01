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
	"fmt"
	"time"

	"github.com/google/uuid"
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
	ListExisting(ctx context.Context, ouID string, uuids []uuid.UUID, llmProxyHandles []string) ([]models.ArtifactRef, error)
	GetByHandle(handle, orgUUID string) (*models.Artifact, error)
	GetByUUID(artifactUUID, orgUUID string) (*models.Artifact, error)
	ListByHandlePrefix(ctx context.Context, kinds []string, handlePrefix, orgUUID string) ([]*models.Artifact, error)
	CountByKindAndOrg(kind, orgUUID string) (int, error)
	// WithTx returns the repository bound to tx.
	WithTx(tx *gorm.DB) ArtifactRepository
}

// ArtifactRepo implements ArtifactRepository using GORM
type ArtifactRepo struct {
	db *gorm.DB
}

// NewArtifactRepo creates a new artifact repository
func NewArtifactRepo(db *gorm.DB) ArtifactRepository {
	return &ArtifactRepo{db: db}
}

// WithTx returns the repository bound to tx.
func (r *ArtifactRepo) WithTx(tx *gorm.DB) ArtifactRepository {
	return &ArtifactRepo{db: tx}
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
	if artifact.Kind != "" {
		updates["kind"] = artifact.Kind
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

// ListExisting returns the artifacts of ouID among uuids, and the LLM proxies among llmProxyHandles.
func (r *ArtifactRepo) ListExisting(
	ctx context.Context, ouID string, uuids []uuid.UUID, llmProxyHandles []string,
) ([]models.ArtifactRef, error) {
	if len(uuids) == 0 && len(llmProxyHandles) == 0 {
		return nil, nil
	}
	if uuids == nil {
		uuids = []uuid.UUID{}
	}
	if llmProxyHandles == nil {
		llmProxyHandles = []string{}
	}
	var artifacts []models.Artifact
	err := r.db.WithContext(ctx).
		Select("uuid", "ou_id", "kind", "handle").
		Where("ou_id = ? AND (uuid IN ? OR (kind = ? AND handle IN ?))", ouID, uuids, models.KindLLMProxy, llmProxyHandles).
		Find(&artifacts).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list existing artifacts: %w", err)
	}
	refs := make([]models.ArtifactRef, 0, len(artifacts))
	for _, a := range artifacts {
		refs = append(refs, models.ArtifactRef{OUID: a.OUID, ArtifactUUID: a.UUID, Kind: a.Kind, Handle: a.Handle})
	}
	return refs, nil
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

// ListByHandlePrefix returns the org's artifacts of any of the kinds whose handle starts with handlePrefix.
func (r *ArtifactRepo) ListByHandlePrefix(ctx context.Context, kinds []string, handlePrefix, orgUUID string) ([]*models.Artifact, error) {
	var artifacts []*models.Artifact
	err := r.db.WithContext(ctx).
		Where(`kind IN ? AND ou_id = ? AND handle LIKE ? ESCAPE '\'`, kinds, orgUUID, escapeLikeWildcards(handlePrefix)+"%").
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
