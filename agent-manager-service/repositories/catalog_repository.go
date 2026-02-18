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
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// CatalogRepository defines the interface for catalog data access
type CatalogRepository interface {
	// ListByKind lists catalog entries filtered by kind with pagination
	ListByKind(orgUUID, kind string, limit, offset int) ([]models.CatalogEntry, int64, error)
	// ListAll lists all catalog entries with pagination
	ListAll(orgUUID string, limit, offset int) ([]models.CatalogEntry, int64, error)
	// ListLLMProviders lists comprehensive LLM provider catalog entries with optional environment filter
	ListLLMProviders(orgUUID string, environmentName *string, limit, offset int) ([]models.CatalogLLMProviderEntry, int64, error)
}

// CatalogRepo implements CatalogRepository using GORM
type CatalogRepo struct {
	db *gorm.DB
}

// NewCatalogRepo creates a new catalog repository
func NewCatalogRepo(db *gorm.DB) CatalogRepository {
	return &CatalogRepo{db: db}
}

// GetDB returns the underlying database connection
func (r *CatalogRepo) GetDB() *gorm.DB {
	return r.db
}

// ListByKind lists catalog entries filtered by kind with pagination
func (r *CatalogRepo) ListByKind(orgUUID, kind string, limit, offset int) ([]models.CatalogEntry, int64, error) {
	var entries []models.CatalogEntry
	var total int64

	// Execute count and fetch within a read-only transaction for consistency
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Count total matching records
		if err := tx.Model(&models.CatalogEntry{}).
			Where("organization_uuid = ? AND kind = ? AND in_catalog = ?", orgUUID, kind, true).
			Count(&total).Error; err != nil {
			return err
		}

		// Retrieve paginated results
		if err := tx.
			Where("organization_uuid = ? AND kind = ? AND in_catalog = ?", orgUUID, kind, true).
			Order("created_at DESC").
			Limit(limit).
			Offset(offset).
			Find(&entries).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return entries, total, nil
}

// ListAll lists all catalog entries with pagination
func (r *CatalogRepo) ListAll(orgUUID string, limit, offset int) ([]models.CatalogEntry, int64, error) {
	var entries []models.CatalogEntry
	var total int64

	// Execute count and fetch within a read-only transaction for consistency
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Count total matching records
		if err := tx.Model(&models.CatalogEntry{}).
			Where("organization_uuid = ? AND in_catalog = ?", orgUUID, true).
			Count(&total).Error; err != nil {
			return err
		}

		// Retrieve paginated results
		if err := tx.
			Where("organization_uuid = ? AND in_catalog = ?", orgUUID, true).
			Order("created_at DESC").
			Limit(limit).
			Offset(offset).
			Find(&entries).Error; err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return nil, 0, err
	}

	return entries, total, nil
}

// ListLLMProviders lists comprehensive LLM provider catalog entries
// Note: Environment filtering is done in the service layer using OpenChoreo data
func (r *CatalogRepo) ListLLMProviders(orgUUID string, environmentName *string, limit, offset int) ([]models.CatalogLLMProviderEntry, int64, error) {
	var total int64

	// Build base query for counting - get ALL catalog LLM providers
	countQuery := r.db.Model(&models.LLMProvider{}).
		Joins("JOIN artifacts a ON llm_providers.uuid = a.uuid").
		Where("a.organization_uuid = ? AND a.kind = ? AND a.in_catalog = ?", orgUUID, models.KindLLMProvider, true)

	// Count total matching records
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// Build query for retrieving results - get ALL catalog LLM providers
	query := r.db.
		Select(`
			a.uuid, a.handle, a.name, a.version, a.kind, a.in_catalog, a.created_at,
			llm_providers.description, llm_providers.created_by, llm_providers.status,
			llm_providers.configuration, llm_providers.model_list
		`).
		Table("llm_providers").
		Joins("JOIN artifacts a ON llm_providers.uuid = a.uuid").
		Where("a.organization_uuid = ? AND a.kind = ? AND a.in_catalog = ?", orgUUID, models.KindLLMProvider, true)

	// Apply ordering and pagination
	query = query.
		Order("a.created_at DESC").
		Limit(limit).
		Offset(offset)

	// Execute query - retrieve providers first
	type ProviderRow struct {
		UUID          string
		Handle        string
		Name          string
		Version       string
		Kind          string
		InCatalog     bool
		CreatedAt     time.Time
		Description   string
		CreatedBy     string
		Status        string
		Configuration string
		ModelList     string
	}

	var rows []ProviderRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, 0, err
	}

	// Convert to entries (deployments will be populated by service layer)
	entries := make([]models.CatalogLLMProviderEntry, 0, len(rows))
	for _, row := range rows {
		providerUUID, _ := uuid.Parse(row.UUID)
		entry := models.CatalogLLMProviderEntry{
			UUID:        providerUUID,
			Handle:      row.Handle,
			Name:        row.Name,
			Version:     row.Version,
			Kind:        row.Kind,
			InCatalog:   row.InCatalog,
			CreatedAt:   row.CreatedAt,
			Description: row.Description,
			CreatedBy:   row.CreatedBy,
			Status:      row.Status,
		}
		entries = append(entries, entry)
	}

	return entries, total, nil
}
