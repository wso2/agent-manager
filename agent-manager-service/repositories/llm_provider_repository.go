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
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"gorm.io/gorm"
)

// LLMProviderRepository defines the interface for LLM provider persistence
type LLMProviderRepository interface {
	Create(tx *gorm.DB, p *models.LLMProvider, handle, name, version string, orgUUID uuid.UUID) error
	GetByID(providerID, orgUUID string) (*models.LLMProvider, error)
	List(orgUUID string, limit, offset int) ([]*models.LLMProvider, error)
	Count(orgUUID string) (int, error)
	Update(p *models.LLMProvider, handle string, orgUUID uuid.UUID) error
	Delete(providerID, orgUUID string) error
	Exists(providerID, orgUUID string) (bool, error)
}

// LLMProviderRepo implements LLMProviderRepository using GORM
type LLMProviderRepo struct {
	db           *gorm.DB
	artifactRepo ArtifactRepository
}

// NewLLMProviderRepo creates a new LLM provider repository
func NewLLMProviderRepo(db *gorm.DB) LLMProviderRepository {
	return &LLMProviderRepo{
		db:           db,
		artifactRepo: NewArtifactRepo(db),
	}
}

// Create inserts a new LLM provider
func (r *LLMProviderRepo) Create(tx *gorm.DB, p *models.LLMProvider, handle, name, version string, orgUUID uuid.UUID) error {
	// Generate UUID if not set
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

	// Insert into llm_providers table
	return tx.Create(p).Error
}

// GetByID retrieves an LLM provider by ID (handle)
func (r *LLMProviderRepo) GetByID(providerID, orgUUID string) (*models.LLMProvider, error) {
	var provider models.LLMProvider
	err := r.db.Table("llm_providers p").
		Select("p.*").
		Joins("JOIN artifacts a ON p.uuid = a.uuid").
		Where("a.handle = ? AND a.organization_uuid = ? AND a.kind = ?", providerID, orgUUID, models.KindLLMAPI).
		Scan(&provider).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return &provider, nil
}

// List retrieves LLM providers with pagination
func (r *LLMProviderRepo) List(orgUUID string, limit, offset int) ([]*models.LLMProvider, error) {
	var providers []*models.LLMProvider
	err := r.db.Table("llm_providers p").
		Select("p.*").
		Joins("JOIN artifacts a ON p.uuid = a.uuid").
		Where("a.organization_uuid = ? AND a.kind = ?", orgUUID, models.KindLLMAPI).
		Order("a.created_at DESC").
		Limit(limit).
		Offset(offset).
		Scan(&providers).Error
	return providers, err
}

// Count counts LLM providers for an organization
func (r *LLMProviderRepo) Count(orgUUID string) (int, error) {
	return r.artifactRepo.CountByKindAndOrg(models.KindLLMAPI, orgUUID)
}

// Update modifies an existing LLM provider
func (r *LLMProviderRepo) Update(p *models.LLMProvider, handle string, orgUUID uuid.UUID) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		// Get the provider UUID from handle
		var providerUUID uuid.UUID
		if err := tx.Table("artifacts").
			Select("uuid").
			Where("handle = ? AND organization_uuid = ? AND kind = ?", handle, orgUUID, models.KindLLMAPI).
			Scan(&providerUUID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}

		// Update artifacts table
		if err := r.artifactRepo.Update(tx, &models.Artifact{
			UUID:             providerUUID,
			OrganizationUUID: orgUUID,
			UpdatedAt:        now,
		}); err != nil {
			return fmt.Errorf("failed to update artifact: %w", err)
		}

		// Update llm_providers table
		result := tx.Model(&models.LLMProvider{}).
			Where("uuid = ?", providerUUID).
			Updates(map[string]interface{}{
				"description":   p.Description,
				"template_uuid": p.TemplateUUID,
				"openapi_spec":  p.OpenAPISpec,
				"model_list":    p.ModelList,
				"status":        p.Status,
				"configuration": p.Configuration,
			})

		if result.Error != nil {
			return fmt.Errorf("failed to update provider: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
}

// Delete removes an LLM provider
func (r *LLMProviderRepo) Delete(providerID, orgUUID string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Get the provider UUID from handle
		var providerUUID uuid.UUID
		if err := tx.Table("artifacts").
			Select("uuid").
			Where("handle = ? AND organization_uuid = ? AND kind = ?", providerID, orgUUID, models.KindLLMAPI).
			Scan(&providerUUID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gorm.ErrRecordNotFound
			}
			return err
		}

		// Delete from llm_providers first
		if err := tx.Where("uuid = ?", providerUUID).Delete(&models.LLMProvider{}).Error; err != nil {
			return err
		}

		// Delete from artifacts
		return r.artifactRepo.Delete(tx, providerUUID.String())
	})
}

// Exists checks if an LLM provider exists
func (r *LLMProviderRepo) Exists(providerID, orgUUID string) (bool, error) {
	return r.artifactRepo.Exists(models.KindLLMAPI, providerID, orgUUID)
}
