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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

type AgentKindRepository interface {
	CreateKind(ctx context.Context, kind *models.AgentKind) error
	GetKind(ctx context.Context, orgName, kindName string) (*models.AgentKind, error)
	ListKinds(ctx context.Context, orgName string, limit, offset int) ([]models.AgentKind, int64, error)
	UpdateKind(ctx context.Context, kind *models.AgentKind) error
	DeleteKind(ctx context.Context, orgName, kindName string) error

	CreateVersion(ctx context.Context, version *models.AgentKindVersion) error
	GetVersion(ctx context.Context, kindID uuid.UUID, versionTag string) (*models.AgentKindVersion, error)
	GetVersionByImageID(ctx context.Context, kindID uuid.UUID, imageID string) (*models.AgentKindVersion, error)
	FindVersionByImageIDInOrg(ctx context.Context, orgName, imageID string) (*models.AgentKindVersion, error)
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
	result := r.db.WithContext(ctx).Create(kind)
	return result.Error
}

func (r *agentKindRepo) GetKind(ctx context.Context, orgName, kindName string) (*models.AgentKind, error) {
	var kind models.AgentKind
	result := r.db.WithContext(ctx).
		Preload("Versions", func(db *gorm.DB) *gorm.DB { return db.Order("created_at DESC") }).
		Where("org_name = ? AND name = ?", orgName, kindName).
		First(&kind)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return nil, gorm.ErrRecordNotFound
	}
	return &kind, result.Error
}

func (r *agentKindRepo) ListKinds(ctx context.Context, orgName string, limit, offset int) ([]models.AgentKind, int64, error) {
	var kinds []models.AgentKind
	var total int64

	query := r.db.WithContext(ctx).Model(&models.AgentKind{}).
		Where("org_name = ?", orgName)

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
	result := r.db.WithContext(ctx).
		Model(kind).
		Where("id = ?", kind.ID).
		Updates(map[string]interface{}{
			"display_name": kind.DisplayName,
			"description":  kind.Description,
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

func (r *agentKindRepo) DeleteKind(ctx context.Context, orgName, kindName string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Fetch the kind first so we can delete its versions before the parent row.
		var kind models.AgentKind
		if err := tx.Where("org_name = ? AND name = ?", orgName, kindName).
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

func (r *agentKindRepo) FindVersionByImageIDInOrg(ctx context.Context, orgName, imageID string) (*models.AgentKindVersion, error) {
	var v models.AgentKindVersion
	result := r.db.WithContext(ctx).
		Joins("JOIN agent_kinds ON agent_kinds.id = agent_kind_versions.agent_kind_id").
		Where("agent_kinds.org_name = ? AND agent_kind_versions.image_id = ?", orgName, imageID).
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
