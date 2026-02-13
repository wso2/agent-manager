/*
 *  Copyright (c) 2026, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// LLMProxyRepository defines the interface for LLM proxy persistence
type LLMProxyRepository interface {
	Create(p *models.LLMProxy, handle, name, version string, orgUUID uuid.UUID) error
	GetByID(proxyID, orgUUID string) (*models.LLMProxy, error)
	List(orgUUID string, limit, offset int) ([]*models.LLMProxy, error)
	ListByProject(orgUUID, projectUUID string, limit, offset int) ([]*models.LLMProxy, error)
	ListByProvider(orgUUID, providerUUID string, limit, offset int) ([]*models.LLMProxy, error)
	Count(orgUUID string) (int, error)
	CountByProject(orgUUID, projectUUID string) (int, error)
	CountByProvider(orgUUID, providerUUID string) (int, error)
	Update(p *models.LLMProxy, handle string, orgUUID uuid.UUID) error
	Delete(proxyID, orgUUID string) error
	Exists(proxyID, orgUUID string) (bool, error)
}

// LLMProxyRepo implements LLMProxyRepository using GORM
type LLMProxyRepo struct {
	db           *gorm.DB
	artifactRepo ArtifactRepository
}

// NewLLMProxyRepo creates a new LLM proxy repository
func NewLLMProxyRepo(db *gorm.DB) LLMProxyRepository {
	return &LLMProxyRepo{
		db:           db,
		artifactRepo: NewArtifactRepo(db),
	}
}

// Create inserts a new LLM proxy
func (r *LLMProxyRepo) Create(p *models.LLMProxy, handle, name, version string, orgUUID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		if p.UUID == uuid.Nil {
			p.UUID = uuid.New()
		}
		now := time.Now()

		// Insert into artifacts table first
		if err := r.artifactRepo.Create(tx, &models.Artifact{
			UUID:             p.UUID,
			Handle:           handle,
			Name:             name,
			Version:          version,
			Kind:             models.KindLLMAPI,
			OrganizationUUID: orgUUID,
			CreatedAt:        now,
			UpdatedAt:        now,
		}); err != nil {
			return fmt.Errorf("failed to create artifact: %w", err)
		}

		// Insert into llm_proxies table
		return tx.Create(p).Error
	})
}

// GetByID retrieves an LLM proxy by ID (handle)
func (r *LLMProxyRepo) GetByID(proxyID, orgUUID string) (*models.LLMProxy, error) {
	var proxy models.LLMProxy
	err := r.db.
		Preload("Artifact").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.handle = ? AND a.organization_uuid = ? AND a.kind = ?", proxyID, orgUUID, models.KindLLMAPI).
		First(&proxy).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &proxy, nil
}

// List retrieves LLM proxies with pagination
func (r *LLMProxyRepo) List(orgUUID string, limit, offset int) ([]*models.LLMProxy, error) {
	var proxies []*models.LLMProxy
	err := r.db.
		Preload("Artifact").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.organization_uuid = ? AND a.kind = ?", orgUUID, models.KindLLMAPI).
		Order("a.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&proxies).Error
	return proxies, err
}

// ListByProject retrieves LLM proxies for a specific project with pagination
func (r *LLMProxyRepo) ListByProject(orgUUID, projectUUID string, limit, offset int) ([]*models.LLMProxy, error) {
	var proxies []*models.LLMProxy
	err := r.db.
		Preload("Artifact").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.organization_uuid = ? AND llm_proxies.project_uuid = ? AND a.kind = ?", orgUUID, projectUUID, models.KindLLMAPI).
		Order("a.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&proxies).Error
	return proxies, err
}

// ListByProvider retrieves LLM proxies for a specific provider with pagination
func (r *LLMProxyRepo) ListByProvider(orgUUID, providerUUID string, limit, offset int) ([]*models.LLMProxy, error) {
	var proxies []*models.LLMProxy
	err := r.db.
		Preload("Artifact").
		Joins("JOIN artifacts a ON llm_proxies.uuid = a.uuid").
		Where("a.organization_uuid = ? AND llm_proxies.provider_uuid = ? AND a.kind = ?", orgUUID, providerUUID, models.KindLLMAPI).
		Order("a.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&proxies).Error
	return proxies, err
}

// Count counts LLM proxies for an organization
func (r *LLMProxyRepo) Count(orgUUID string) (int, error) {
	return r.artifactRepo.CountByKindAndOrg(models.KindLLMAPI, orgUUID)
}

// CountByProject counts LLM proxies for a specific project
func (r *LLMProxyRepo) CountByProject(orgUUID, projectUUID string) (int, error) {
	var count int64
	err := r.db.Table("artifacts a").
		Joins("JOIN llm_proxies p ON a.uuid = p.uuid").
		Where("a.organization_uuid = ? AND p.project_uuid = ? AND a.kind = ?", orgUUID, projectUUID, models.KindLLMAPI).
		Count(&count).Error
	return int(count), err
}

// CountByProvider counts LLM proxies for a specific provider
func (r *LLMProxyRepo) CountByProvider(orgUUID, providerUUID string) (int, error) {
	var count int64
	err := r.db.Table("artifacts a").
		Joins("JOIN llm_proxies p ON a.uuid = p.uuid").
		Where("a.organization_uuid = ? AND p.provider_uuid = ? AND a.kind = ?", orgUUID, providerUUID, models.KindLLMAPI).
		Count(&count).Error
	return int(count), err
}

// Update modifies an existing LLM proxy
func (r *LLMProxyRepo) Update(p *models.LLMProxy, handle string, orgUUID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		// Get the proxy UUID from handle
		var proxyUUID uuid.UUID
		if err := tx.Table("artifacts").
			Select("uuid").
			Where("handle = ? AND organization_uuid = ? AND kind = ?", handle, orgUUID, models.KindLLMAPI).
			Scan(&proxyUUID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}

		// Update artifacts table
		if err := r.artifactRepo.Update(tx, &models.Artifact{
			UUID:             proxyUUID,
			OrganizationUUID: orgUUID,
			UpdatedAt:        now,
		}); err != nil {
			return fmt.Errorf("failed to update artifact: %w", err)
		}

		// Update llm_proxies table
		result := tx.Model(&models.LLMProxy{}).
			Where("uuid = ?", proxyUUID).
			Updates(map[string]interface{}{
				"description":   p.Description,
				"provider_uuid": p.ProviderUUID,
				"openapi_spec":  p.OpenAPISpec,
				"status":        p.Status,
				"configuration": p.Configuration,
			})

		if result.Error != nil {
			return fmt.Errorf("failed to update proxy: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// Delete removes an LLM proxy
func (r *LLMProxyRepo) Delete(proxyID, orgUUID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Get the proxy UUID from handle
		var proxyUUID uuid.UUID
		if err := tx.Table("artifacts").
			Select("uuid").
			Where("handle = ? AND organization_uuid = ? AND a.kind = ?", proxyID, orgUUID, models.KindLLMAPI).
			Scan(&proxyUUID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}

		// Delete from llm_proxies first
		if err := tx.Where("uuid = ?", proxyUUID).Delete(&models.LLMProxy{}).Error; err != nil {
			return err
		}

		// Delete from artifacts
		return r.artifactRepo.Delete(tx, proxyUUID.String())
	})
}

// Exists checks if an LLM proxy exists
func (r *LLMProxyRepo) Exists(proxyID, orgUUID string) (bool, error) {
	return r.artifactRepo.Exists(models.KindLLMAPI, proxyID, orgUUID)
}
