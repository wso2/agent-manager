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
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// LLMProviderRepository defines the interface for LLM provider persistence
type LLMProviderRepository interface {
	Create(tx *gorm.DB, p *models.LLMProvider, handle, name, version string, orgUUID uuid.UUID) error
	GetByUUID(providerID, orgUUID string) (*models.LLMProvider, error)
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
	slog.Info("LLMProviderRepo.Create: starting", "handle", handle, "name", name, "version", version, "orgUUID", orgUUID)

	// Generate UUID if not set
	if p.UUID == uuid.Nil {
		p.UUID = uuid.New()
		slog.Info("LLMProviderRepo.Create: generated new UUID", "handle", handle, "uuid", p.UUID)
	}
	now := time.Now()

	// Insert into artifacts table first
	slog.Info("LLMProviderRepo.Create: creating artifact", "handle", handle, "uuid", p.UUID, "kind", models.KindLLMAPI)
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
		slog.Error("LLMProviderRepo.Create: failed to create artifact", "handle", handle, "uuid", p.UUID, "error", err)
		return fmt.Errorf("failed to create artifact: %w", err)
	}

	// Insert into llm_providers table
	slog.Info("LLMProviderRepo.Create: inserting into llm_providers table", "handle", handle, "uuid", p.UUID)
	if err := tx.Create(p).Error; err != nil {
		slog.Error("LLMProviderRepo.Create: failed to insert provider", "handle", handle, "uuid", p.UUID, "error", err)
		return err
	}

	slog.Info("LLMProviderRepo.Create: completed successfully", "handle", handle, "uuid", p.UUID)
	return nil
}

// GetByID retrieves an LLM provider by ID (handle)
func (r *LLMProviderRepo) GetByUUID(providerID, orgUUID string) (*models.LLMProvider, error) {
	slog.Info("LLMProviderRepo.GetByID: starting", "providerID", providerID, "orgUUID", orgUUID)

	var provider models.LLMProvider
	err := r.db.
		Preload("Artifact").
		Joins("JOIN artifacts a ON llm_providers.uuid = a.uuid").
		Where("a.uuid = ? AND a.organization_uuid = ? AND a.kind = ?", providerID, orgUUID, models.KindLLMAPI).
		First(&provider).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderRepo.GetByID: provider not found", "providerID", providerID, "orgUUID", orgUUID)
			return nil, err
		}
		slog.Error("LLMProviderRepo.GetByID: query failed", "providerID", providerID, "orgUUID", orgUUID, "error", err)
		return nil, err
	}

	slog.Info("LLMProviderRepo.GetByID: completed successfully", "providerID", providerID, "orgUUID", orgUUID, "uuid", provider.UUID)
	return &provider, nil
}

// List retrieves LLM providers with pagination
func (r *LLMProviderRepo) List(orgUUID string, limit, offset int) ([]*models.LLMProvider, error) {
	slog.Info("LLMProviderRepo.List: starting", "orgUUID", orgUUID, "limit", limit, "offset", offset)

	var providers []*models.LLMProvider
	err := r.db.
		Preload("Artifact").
		Joins("JOIN artifacts a ON llm_providers.uuid = a.uuid").
		Where("a.organization_uuid = ? AND a.kind = ?", orgUUID, models.KindLLMAPI).
		Order("a.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&providers).Error
	if err != nil {
		slog.Error("LLMProviderRepo.List: query failed", "orgUUID", orgUUID, "error", err)
		return providers, err
	}

	slog.Info("LLMProviderRepo.List: completed successfully", "orgUUID", orgUUID, "count", len(providers))
	return providers, nil
}

// Count counts LLM providers for an organization
func (r *LLMProviderRepo) Count(orgUUID string) (int, error) {
	return r.artifactRepo.CountByKindAndOrg(models.KindLLMAPI, orgUUID)
}

// Update modifies an existing LLM provider
func (r *LLMProviderRepo) Update(p *models.LLMProvider, handle string, orgUUID uuid.UUID) error {
	slog.Info("LLMProviderRepo.Update: starting", "handle", handle, "orgUUID", orgUUID)

	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()

		// Get the provider UUID from handle
		slog.Info("LLMProviderRepo.Update: resolving handle to UUID", "handle", handle, "orgUUID", orgUUID)
		var artifact struct{ UUID uuid.UUID }
		result := tx.Table("artifacts").
			Select("uuid").
			Where("handle = ? AND organization_uuid = ? AND kind = ?", handle, orgUUID, models.KindLLMAPI).
			Take(&artifact)
		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				slog.Warn("LLMProviderRepo.Update: provider not found", "handle", handle, "orgUUID", orgUUID)
				return gorm.ErrRecordNotFound
			}
			slog.Error("LLMProviderRepo.Update: failed to resolve handle", "handle", handle, "orgUUID", orgUUID, "error", result.Error)
			return result.Error
		}
		providerUUID := artifact.UUID

		slog.Info("LLMProviderRepo.Update: resolved UUID", "handle", handle, "uuid", providerUUID)

		// Update artifacts table
		slog.Info("LLMProviderRepo.Update: updating artifact", "handle", handle, "uuid", providerUUID)
		if err := r.artifactRepo.Update(tx, &models.Artifact{
			UUID:             providerUUID,
			OrganizationUUID: orgUUID,
			UpdatedAt:        now,
		}); err != nil {
			slog.Error("LLMProviderRepo.Update: failed to update artifact", "handle", handle, "uuid", providerUUID, "error", err)
			return fmt.Errorf("failed to update artifact: %w", err)
		}

		// Update llm_providers table
		slog.Info("LLMProviderRepo.Update: updating provider fields", "handle", handle, "uuid", providerUUID)
		result = tx.Model(&models.LLMProvider{}).
			Where("uuid = ?", providerUUID).
			Updates(map[string]any{
				"description":   p.Description,
				"template_uuid": p.TemplateUUID,
				"openapi_spec":  p.OpenAPISpec,
				"model_list":    p.ModelList,
				"status":        p.Status,
				"configuration": p.Configuration,
			})

		if result.Error != nil {
			slog.Error("LLMProviderRepo.Update: failed to update provider", "handle", handle, "uuid", providerUUID, "error", result.Error)
			return fmt.Errorf("failed to update provider: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			slog.Warn("LLMProviderRepo.Update: no rows affected", "handle", handle, "uuid", providerUUID)
			return gorm.ErrRecordNotFound
		}

		slog.Info("LLMProviderRepo.Update: completed successfully", "handle", handle, "uuid", providerUUID, "rowsAffected", result.RowsAffected)
		return nil
	})
}

// Delete removes an LLM provider
func (r *LLMProviderRepo) Delete(providerID, orgUUID string) error {
	slog.Info("LLMProviderRepo.Delete: starting", "providerID", providerID, "orgUUID", orgUUID)

	return r.db.Transaction(func(tx *gorm.DB) error {
		// Get the provider UUID from handle
		slog.Info("LLMProviderRepo.Delete: resolving handle to UUID", "providerID", providerID, "orgUUID", orgUUID)
		var artifact struct{ UUID uuid.UUID }
		result := tx.Table("artifacts").
			Select("uuid").
			Where("handle = ? AND organization_uuid = ? AND kind = ?", providerID, orgUUID, models.KindLLMAPI).
			Take(&artifact)
		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				slog.Warn("LLMProviderRepo.Delete: provider not found", "providerID", providerID, "orgUUID", orgUUID)
				return gorm.ErrRecordNotFound
			}
			slog.Error("LLMProviderRepo.Delete: failed to resolve handle", "providerID", providerID, "orgUUID", orgUUID, "error", result.Error)
			return result.Error
		}
		providerUUID := artifact.UUID

		slog.Info("LLMProviderRepo.Delete: resolved UUID", "providerID", providerID, "uuid", providerUUID)

		// Delete from llm_providers first
		slog.Info("LLMProviderRepo.Delete: deleting from llm_providers table", "providerID", providerID, "uuid", providerUUID)
		if err := tx.Where("uuid = ?", providerUUID).Delete(&models.LLMProvider{}).Error; err != nil {
			slog.Error("LLMProviderRepo.Delete: failed to delete provider", "providerID", providerID, "uuid", providerUUID, "error", err)
			return err
		}

		// Delete from artifacts
		slog.Info("LLMProviderRepo.Delete: deleting from artifacts table", "providerID", providerID, "uuid", providerUUID)
		if err := r.artifactRepo.Delete(tx, providerUUID.String()); err != nil {
			slog.Error("LLMProviderRepo.Delete: failed to delete artifact", "providerID", providerID, "uuid", providerUUID, "error", err)
			return err
		}

		slog.Info("LLMProviderRepo.Delete: completed successfully", "providerID", providerID, "uuid", providerUUID)
		return nil
	})
}

// Exists checks if an LLM provider exists
func (r *LLMProviderRepo) Exists(providerID, orgUUID string) (bool, error) {
	slog.Info("LLMProviderRepo.Exists: checking", "providerID", providerID, "orgUUID", orgUUID)

	exists, err := r.artifactRepo.Exists(models.KindLLMAPI, providerID, orgUUID)
	if err != nil {
		slog.Error("LLMProviderRepo.Exists: check failed", "providerID", providerID, "orgUUID", orgUUID, "error", err)
		return false, err
	}

	slog.Info("LLMProviderRepo.Exists: completed", "providerID", providerID, "orgUUID", orgUUID, "exists", exists)
	return exists, nil
}
