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
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// LLMProviderRepository defines the interface for LLM provider persistence
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/llm_provider_repository_mock.go . LLMProviderRepository:LLMProviderRepositoryMock
type LLMProviderRepository interface {
	Create(tx *gorm.DB, p *models.LLMProvider, handle, name, version string, orgUUID string) error
	GetByUUID(providerID, orgUUID string) (*models.LLMProvider, error)
	GetByHandle(handle, orgUUID string) (*models.LLMProvider, error)
	List(orgUUID string, limit, offset int) ([]*models.LLMProvider, error)
	Count(orgUUID string) (int, error)
	Update(p *models.LLMProvider, providerID string, orgUUID string) error
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
func (r *LLMProviderRepo) Create(tx *gorm.DB, p *models.LLMProvider, handle, name, version string, orgUUID string) error {
	slog.Info("LLMProviderRepo.Create: starting", "handle", handle, "name", name, "version", version, "org_uuid", orgUUID)

	// Generate UUID if not set
	if p.UUID == uuid.Nil {
		p.UUID = uuid.New()
		slog.Info("LLMProviderRepo.Create: generated new UUID", "handle", handle, "uuid", p.UUID)
	}
	now := time.Now()

	// Insert into artifacts table first
	slog.Info("LLMProviderRepo.Create: creating artifact", "handle", handle, "uuid", p.UUID, "kind", models.KindLLMProvider)
	if err := r.artifactRepo.Create(tx, &models.Artifact{
		UUID:      p.UUID,
		Handle:    handle,
		Name:      name,
		Version:   version,
		Kind:      models.KindLLMProvider,
		OUID:      orgUUID,
		CreatedAt: now,
		UpdatedAt: now,
		InCatalog: true,
	}); err != nil {
		slog.Error("LLMProviderRepo.Create: failed to create artifact", "handle", handle, "uuid", p.UUID, "error", err)
		return fmt.Errorf("failed to create artifact: %w", err)
	}

	// Insert into llm_providers table
	slog.Info("LLMProviderRepo.Create: inserting into llm_providers table", "handle", handle, "uuid", p.UUID)
	if err := tx.Omit("status").Create(p).Error; err != nil {
		slog.Error("LLMProviderRepo.Create: failed to insert provider", "handle", handle, "uuid", p.UUID, "error", err)
		return err
	}

	slog.Info("LLMProviderRepo.Create: completed successfully", "handle", handle, "uuid", p.UUID)
	return nil
}

// GetByID retrieves an LLM provider by ID (handle)
func (r *LLMProviderRepo) GetByUUID(providerID, orgUUID string) (*models.LLMProvider, error) {
	slog.Info("LLMProviderRepo.GetByID: starting", "provider_id", providerID, "org_uuid", orgUUID)

	var provider models.LLMProvider
	err := r.db.
		Preload("Artifact").
		Joins("JOIN artifacts a ON llm_providers.uuid = a.uuid").
		Where("a.uuid = ? AND a.ou_id = ? AND a.kind = ?", providerID, orgUUID, models.KindLLMProvider).
		First(&provider).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderRepo.GetByID: provider not found", "provider_id", providerID, "org_uuid", orgUUID)
			return nil, err
		}
		slog.Error("LLMProviderRepo.GetByID: query failed", "provider_id", providerID, "org_uuid", orgUUID, "error", err)
		return nil, err
	}

	// Populate InCatalog from preloaded Artifact
	if provider.Artifact != nil {
		provider.InCatalog = provider.Artifact.InCatalog
	}

	slog.Info("LLMProviderRepo.GetByID: completed successfully", "provider_id", providerID, "org_uuid", orgUUID, "uuid", provider.UUID)
	return &provider, nil
}

// GetByHandle retrieves an LLM provider by artifact handle
func (r *LLMProviderRepo) GetByHandle(handle, orgUUID string) (*models.LLMProvider, error) {
	slog.Info("LLMProviderRepo.GetByHandle: starting", "handle", handle, "org_uuid", orgUUID)

	var provider models.LLMProvider
	err := r.db.
		Preload("Artifact").
		Joins("JOIN artifacts a ON llm_providers.uuid = a.uuid").
		Where("a.handle = ? AND a.ou_id = ? AND a.kind = ?", handle, orgUUID, models.KindLLMProvider).
		First(&provider).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderRepo.GetByHandle: provider not found", "handle", handle, "org_uuid", orgUUID)
			return nil, err
		}
		slog.Error("LLMProviderRepo.GetByHandle: query failed", "handle", handle, "org_uuid", orgUUID, "error", err)
		return nil, err
	}

	if provider.Artifact != nil {
		provider.InCatalog = provider.Artifact.InCatalog
	}

	slog.Info("LLMProviderRepo.GetByHandle: completed successfully", "handle", handle, "org_uuid", orgUUID, "uuid", provider.UUID)
	return &provider, nil
}

// List retrieves LLM providers with pagination
func (r *LLMProviderRepo) List(orgUUID string, limit, offset int) ([]*models.LLMProvider, error) {
	slog.Info("LLMProviderRepo.List: starting", "org_uuid", orgUUID, "limit", limit, "offset", offset)

	var providers []*models.LLMProvider
	err := r.db.
		Preload("Artifact").
		Joins("JOIN artifacts a ON llm_providers.uuid = a.uuid").
		Where("a.ou_id = ? AND a.kind = ?", orgUUID, models.KindLLMProvider).
		Order("a.created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&providers).Error
	if err != nil {
		slog.Error("LLMProviderRepo.List: query failed", "org_uuid", orgUUID, "error", err)
		return providers, err
	}

	// Populate InCatalog from preloaded Artifact for each provider
	for _, provider := range providers {
		if provider.Artifact != nil {
			provider.InCatalog = provider.Artifact.InCatalog
		}
	}

	slog.Info("LLMProviderRepo.List: completed successfully", "org_uuid", orgUUID, "count", len(providers))
	return providers, nil
}

// Count counts LLM providers for an organization
func (r *LLMProviderRepo) Count(orgUUID string) (int, error) {
	return r.artifactRepo.CountByKindAndOrg(models.KindLLMProvider, orgUUID)
}

// Update modifies an existing LLM provider
func (r *LLMProviderRepo) Update(p *models.LLMProvider, providerID string, orgUUID string) error {
	slog.Info("LLMProviderRepo.Update: starting", "provider_id", providerID, "org_uuid", orgUUID)

	return r.db.Transaction(func(tx *gorm.DB) error {
		slog.Info("LLMProviderRepo.Update: resolved UUID", "provider_id", providerID)

		_, err := uuid.Parse(providerID)
		if err != nil {
			return fmt.Errorf("error parsing provider id: %s, error: %w", providerID, err)
		}

		// Update llm_providers table
		slog.Info("LLMProviderRepo.Update: updating provider fields", "handle", providerID)
		result := tx.Model(&models.LLMProvider{}).
			Where("uuid = ?", providerID).
			Updates(map[string]any{
				"description":     p.Description,
				"template_handle": p.TemplateHandle,
				"openapi_spec":    p.OpenAPISpec,
				"model_list":      p.ModelList,
				"configuration":   p.Configuration,
			})

		if result.Error != nil {
			slog.Error("LLMProviderRepo.Update: failed to update provider", "handle", providerID, "error", result.Error)
			return fmt.Errorf("failed to update provider: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			slog.Warn("LLMProviderRepo.Update: no rows affected", "handle", providerID)
			return gorm.ErrRecordNotFound
		}

		slog.Info("LLMProviderRepo.Update: completed successfully", "handle", providerID, "rows_affected", result.RowsAffected)
		return nil
	})
}

// Delete removes an LLM provider
func (r *LLMProviderRepo) Delete(providerID, orgUUID string) error {
	slog.Info("LLMProviderRepo.Delete: starting", "provider_id", providerID, "org_uuid", orgUUID)

	// Parse providerID as UUID
	providerUUID, err := uuid.Parse(providerID)
	if err != nil {
		slog.Error("LLMProviderRepo.Delete: invalid provider UUID", "provider_id", providerID, "error", err)
		return fmt.Errorf("invalid provider UUID: %w", err)
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		// Verify the provider exists and belongs to the organization
		slog.Info("LLMProviderRepo.Delete: verifying provider exists", "provider_id", providerID, "uuid", providerUUID, "org_uuid", orgUUID)
		var artifact struct{ UUID uuid.UUID }
		result := tx.Table("artifacts").
			Select("uuid").
			Where("uuid = ? AND ou_id = ? AND kind = ?", providerUUID, orgUUID, models.KindLLMProvider).
			Take(&artifact)
		if result.Error != nil {
			if errors.Is(result.Error, gorm.ErrRecordNotFound) {
				slog.Warn("LLMProviderRepo.Delete: provider not found", "provider_id", providerID, "uuid", providerUUID, "org_uuid", orgUUID)
				return gorm.ErrRecordNotFound
			}
			slog.Error("LLMProviderRepo.Delete: failed to verify provider", "provider_id", providerID, "uuid", providerUUID, "org_uuid", orgUUID, "error", result.Error)
			return result.Error
		}

		slog.Info("LLMProviderRepo.Delete: provider verified", "provider_id", providerID, "uuid", providerUUID)

		slog.Info("LLMProviderRepo.Delete: deleting from llm_providers table", "provider_id", providerID, "uuid", providerUUID)
		if err := tx.Where("uuid = ?", providerUUID).Delete(&models.LLMProvider{}).Error; err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == "23503" {
				slog.Warn("LLMProviderRepo.Delete: provider has associated proxies", "provider_id", providerID, "uuid", providerUUID, "constraint", pgErr.ConstraintName)
				return utils.ErrLLMProviderHasProxies
			}
			slog.Error("LLMProviderRepo.Delete: failed to delete provider", "provider_id", providerID, "uuid", providerUUID, "error", err)
			return err
		}

		// Delete from artifacts
		slog.Info("LLMProviderRepo.Delete: deleting from artifacts table", "provider_id", providerID, "uuid", providerUUID)
		if err := r.artifactRepo.Delete(tx, providerUUID.String()); err != nil {
			slog.Error("LLMProviderRepo.Delete: failed to delete artifact", "provider_id", providerID, "uuid", providerUUID, "error", err)
			return err
		}

		slog.Info("LLMProviderRepo.Delete: completed successfully", "provider_id", providerID, "uuid", providerUUID)
		return nil
	})
}

// Exists checks if an LLM provider exists
func (r *LLMProviderRepo) Exists(providerID, orgUUID string) (bool, error) {
	slog.Info("LLMProviderRepo.Exists: checking", "provider_id", providerID, "org_uuid", orgUUID)

	exists, err := r.artifactRepo.Exists(models.KindLLMProvider, providerID, orgUUID)
	if err != nil {
		slog.Error("LLMProviderRepo.Exists: check failed", "provider_id", providerID, "org_uuid", orgUUID, "error", err)
		return false, err
	}

	slog.Info("LLMProviderRepo.Exists: completed", "provider_id", providerID, "org_uuid", orgUUID, "exists", exists)
	return exists, nil
}
