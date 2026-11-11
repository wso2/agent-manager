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
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/agent_kind_repository_mock.go . AgentKindRepository:AgentKindRepositoryMock
type AgentKindRepository interface {
	CreateKind(ctx context.Context, kind *models.AgentKind) error
	GetKind(ctx context.Context, ouID, kindName string) (*models.AgentKind, error)
	ListKinds(ctx context.Context, ouID string, labelFilter map[string]string, limit, offset int) ([]models.AgentKind, int64, error)
	UpdateKind(ctx context.Context, kind *models.AgentKind) error
	DeleteKind(ctx context.Context, ouID, kindName string) error

	// ExistsBySourceAgent reports whether any agent kind is published from the
	// given source agent.
	//
	// NOTE: this is intentionally a plain existence check, not lock-protected.
	// There's a check-then-act gap between this and whatever the caller does next
	// (e.g. deleting the agent), but closing it fully would mean
	// holding a DB transaction open across platform/secret-manager network
	// calls — a worse trade than the risk it prevents. That risk also requires
	// two admins racing the exact same agent within milliseconds, and is
	// independently mitigated by publishVersion verifying the source agent still
	// exists before it ever creates a kind (see agent_kind_service.go). Treat a
	// false negative here as acceptable; do not "fix" this with a lock without
	// re-reading that reasoning.
	ExistsBySourceAgent(ctx context.Context, ouID, projectName, agentName string) (bool, error)

	CreateVersion(ctx context.Context, version *models.AgentKindVersion) error
	GetVersion(ctx context.Context, kindID uuid.UUID, versionTag string) (*models.AgentKindVersion, error)
	GetVersionByImageID(ctx context.Context, kindID uuid.UUID, imageID string) (*models.AgentKindVersion, error)
	FindVersionByImageIDInOrg(ctx context.Context, ouID, imageID string) (*models.AgentKindVersion, error)
	ListVersions(ctx context.Context, kindID uuid.UUID) ([]models.AgentKindVersion, error)
	DeleteVersion(ctx context.Context, kindID uuid.UUID, versionTag string) error
}

type agentKindRepo struct {
	db *gorm.DB
}

func NewAgentKindRepo(db *gorm.DB) AgentKindRepository {
	return &agentKindRepo{db: db}
}

func (r *agentKindRepo) CreateKind(ctx context.Context, kind *models.AgentKind) error {
	if kind.ID == uuid.Nil {
		kind.ID = uuid.New()
	}
	if kind.Labels == nil {
		kind.Labels = map[string]string{}
	}
	result := r.db.WithContext(ctx).Create(kind)
	return result.Error
}

func (r *agentKindRepo) ExistsBySourceAgent(ctx context.Context, ouID, projectName, agentName string) (bool, error) {
	var count int64
	result := r.db.WithContext(ctx).Model(&models.AgentKind{}).
		Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		Count(&count)
	return count > 0, result.Error
}

func (r *agentKindRepo) GetKind(ctx context.Context, ouID, kindName string) (*models.AgentKind, error) {
	var kind models.AgentKind
	result := r.db.WithContext(ctx).
		Preload("Versions", func(db *gorm.DB) *gorm.DB { return db.Order("created_at DESC") }).
		Where("ou_id = ? AND name = ?", ouID, kindName).
		First(&kind)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	return &kind, result.Error
}

func (r *agentKindRepo) ListKinds(ctx context.Context, ouID string, labelFilter map[string]string, limit, offset int) ([]models.AgentKind, int64, error) {
	var kinds []models.AgentKind
	var total int64

	query := r.db.WithContext(ctx).Model(&models.AgentKind{}).
		Where("ou_id = ?", ouID)

	if len(labelFilter) > 0 {
		filterJSON, err := json.Marshal(labelFilter)
		if err != nil {
			return nil, 0, err
		}
		query = query.Where("labels @> ?::jsonb", string(filterJSON))
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	result := query.
		Preload("Versions", func(db *gorm.DB) *gorm.DB { return db.Order("created_at DESC") }).
		Limit(limit).
		Offset(offset).
		Order("created_at DESC").
		Find(&kinds)
	return kinds, total, result.Error
}

func (r *agentKindRepo) UpdateKind(ctx context.Context, kind *models.AgentKind) error {
	// Updates(map) bypasses GORM's serializer:json tag (same caveat as
	// agent_config_repository.go), so labels must be pre-serialized and cast
	// to jsonb explicitly. A nil map marshals to SQL null — normalize first.
	labels := kind.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	labelsJSON, err := json.Marshal(labels)
	if err != nil {
		return err
	}
	result := r.db.WithContext(ctx).
		Model(kind).
		Where("id = ?", kind.ID).
		Updates(map[string]interface{}{
			"display_name": kind.DisplayName,
			"description":  kind.Description,
			"labels":       gorm.Expr("?::jsonb", string(labelsJSON)),
			"updated_at":   kind.UpdatedAt,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func (r *agentKindRepo) DeleteKind(ctx context.Context, ouID, kindName string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Fetch the kind first so we can delete its versions before the parent row.
		var kind models.AgentKind
		if err := tx.Where("ou_id = ? AND name = ?", ouID, kindName).
			First(&kind).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}

		// Delete all versions belonging to this kind.
		if err := tx.Where("agent_kind_id = ?", kind.ID).
			Delete(&models.AgentKindVersion{}).Error; err != nil {
			return err
		}

		// Delete the kind itself.
		result := tx.Delete(&kind)
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return result.Error
	})
}

func (r *agentKindRepo) CreateVersion(ctx context.Context, version *models.AgentKindVersion) error {
	if version.ID == uuid.Nil {
		version.ID = uuid.New()
	}
	return r.db.WithContext(ctx).Create(version).Error
}

func (r *agentKindRepo) GetVersion(ctx context.Context, kindID uuid.UUID, versionTag string) (*models.AgentKindVersion, error) {
	var v models.AgentKindVersion
	result := r.db.WithContext(ctx).
		Preload("Kind").
		Where("agent_kind_id = ? AND version = ?", kindID, versionTag).
		First(&v)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	return &v, result.Error
}

func (r *agentKindRepo) GetVersionByImageID(ctx context.Context, kindID uuid.UUID, imageID string) (*models.AgentKindVersion, error) {
	var v models.AgentKindVersion
	result := r.db.WithContext(ctx).
		Where("agent_kind_id = ? AND image_id = ?", kindID, imageID).
		First(&v)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	return &v, result.Error
}

func (r *agentKindRepo) FindVersionByImageIDInOrg(ctx context.Context, ouID, imageID string) (*models.AgentKindVersion, error) {
	var v models.AgentKindVersion
	result := r.db.WithContext(ctx).
		Joins("JOIN agent_kinds ON agent_kinds.id = agent_kind_versions.agent_kind_id").
		Where("agent_kinds.ou_id = ? AND agent_kind_versions.image_id = ?", ouID, imageID).
		Preload("Kind").
		First(&v)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	return &v, result.Error
}

func (r *agentKindRepo) ListVersions(ctx context.Context, kindID uuid.UUID) ([]models.AgentKindVersion, error) {
	var versions []models.AgentKindVersion
	result := r.db.WithContext(ctx).
		Where("agent_kind_id = ?", kindID).
		Order("created_at DESC").
		Find(&versions)
	return versions, result.Error
}

func (r *agentKindRepo) DeleteVersion(ctx context.Context, kindID uuid.UUID, versionTag string) error {
	result := r.db.WithContext(ctx).
		Where("agent_kind_id = ? AND version = ?", kindID, versionTag).
		Delete(&models.AgentKindVersion{})
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return result.Error
}
