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
	"fmt"
	"strings"
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
	// ListLLMProviders lists comprehensive LLM provider catalog entries with dynamic filtering
	ListLLMProviders(filters *models.CatalogListFilters) ([]models.CatalogLLMProviderEntry, int64, error)
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

// ListLLMProviders lists comprehensive LLM provider catalog entries with dynamic filtering
// Uses a single optimized query joining llm_providers, artifacts, and deployment_status tables
func (r *CatalogRepo) ListLLMProviders(filters *models.CatalogListFilters) ([]models.CatalogLLMProviderEntry, int64, error) {
	if filters == nil {
		return nil, 0, fmt.Errorf("filters cannot be nil")
	}

	if err := filters.Validate(); err != nil {
		return nil, 0, fmt.Errorf("invalid filters: %w", err)
	}

	var total int64

	// Build base query with common joins and conditions
	baseQuery := r.db.Model(&models.LLMProvider{}).
		Joins("JOIN artifacts a ON llm_providers.uuid = a.uuid").
		Where("a.organization_uuid = ? AND a.kind = ? AND a.in_catalog = ?",
			filters.OrganizationUUID, models.KindLLMProvider, true)

	// Apply environment filter if provided (join with deployment_status)
	if filters.HasEnvironmentFilter() {
		// Join with deployment_status and gateway_environment_mappings
		baseQuery = baseQuery.
			Joins("JOIN deployment_status ds ON llm_providers.uuid = ds.artifact_uuid AND ds.organization_uuid = a.organization_uuid").
			Joins("JOIN gateway_environment_mappings gem ON ds.gateway_uuid = gem.gateway_uuid").
			Where("gem.environment_uuid = ? AND ds.status = ?",
					filters.EnvironmentUUID, models.DeploymentStatusDeployed).
			Distinct() // Ensure unique providers (one provider can have multiple deployments)
	}

	// Apply name filter if provided (partial match, case-insensitive)
	if filters.HasNameFilter() {
		// Escape LIKE wildcards to prevent SQL injection
		escapedName := escapeLikeWildcards(filters.Name)
		baseQuery = baseQuery.Where("LOWER(a.name) LIKE LOWER(?) ESCAPE '\\'", "%"+escapedName+"%")
	}

	// Count total matching records
	countQuery := baseQuery.Session(&gorm.Session{}) // Clone to avoid mutation
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to count catalog entries: %w", err)
	}

	// Build query for retrieving ALL provider data (configuration only, not deployments yet)
	// We'll fetch deployments separately and aggregate them
	query := r.db.
		Select(`
			a.uuid, a.handle, a.name, a.version, a.kind, a.in_catalog, a.created_at,
			llm_providers.description, llm_providers.created_by, llm_providers.status,
			llm_providers.configuration, llm_providers.model_list,
			a.organization_uuid
		`).
		Table("llm_providers").
		Joins("JOIN artifacts a ON llm_providers.uuid = a.uuid").
		Where("a.organization_uuid = ? AND a.kind = ? AND a.in_catalog = ?",
			filters.OrganizationUUID, models.KindLLMProvider, true)

	// Apply same filters to data query
	if filters.HasEnvironmentFilter() {
		query = query.
			Joins("JOIN deployment_status ds ON llm_providers.uuid = ds.artifact_uuid AND ds.organization_uuid = a.organization_uuid").
			Joins("JOIN gateway_environment_mappings gem ON ds.gateway_uuid = gem.gateway_uuid").
			Where("gem.environment_uuid = ? AND ds.status = ?",
				filters.EnvironmentUUID, models.DeploymentStatusDeployed)
	}

	if filters.HasNameFilter() {
		// Escape LIKE wildcards to prevent SQL injection
		escapedName := escapeLikeWildcards(filters.Name)
		query = query.Where("LOWER(a.name) LIKE LOWER(?) ESCAPE '\\'", "%"+escapedName+"%")
	}

	// Apply ordering and pagination
	query = query.
		Order("a.created_at DESC").
		Limit(filters.Limit).
		Offset(filters.Offset)

	// Execute query - fetch complete provider data
	type ProviderRow struct {
		UUID             string
		Handle           string
		Name             string
		Version          string
		Kind             string
		InCatalog        bool
		CreatedAt        time.Time
		Description      string
		CreatedBy        string
		Status           string
		Configuration    string // Full JSON configuration
		ModelList        string // Full JSON model list
		OrganizationUUID string
	}

	var rows []ProviderRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to query catalog entries: %w", err)
	}

	// Convert to entries with FULL configuration data
	entries := make([]models.CatalogLLMProviderEntry, 0, len(rows))
	for _, row := range rows {
		providerUUID, err := uuid.Parse(row.UUID)
		if err != nil {
			// Database UUIDs should always be valid - this indicates data corruption
			r.db.Logger.Error(context.Background(),
				"Database integrity error: invalid UUID in artifacts table - possible data corruption (uuid=%s, handle=%s, orgUUID=%s): %v",
				row.UUID, row.Handle, row.OrganizationUUID, err)
			return nil, 0, fmt.Errorf("database integrity error: invalid UUID %q in artifacts table (handle=%s): %w",
				row.UUID, row.Handle, err)
		}

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

		// Parse FULL configuration JSON and populate ALL fields
		if row.Configuration != "" {
			var config models.LLMProviderConfig
			if err := json.Unmarshal([]byte(row.Configuration), &config); err == nil {
				// Basic configuration
				entry.Template = config.Template
				entry.Context = config.Context
				entry.VHost = config.VHost

				// Security summary
				if config.Security != nil {
					entry.Security = &models.SecuritySummary{
						Enabled:       config.Security.Enabled != nil && *config.Security.Enabled,
						APIKeyEnabled: config.Security.APIKey != nil && config.Security.APIKey.Enabled != nil && *config.Security.APIKey.Enabled,
					}
					if config.Security.APIKey != nil {
						entry.Security.APIKeyIn = config.Security.APIKey.In
					}
				}

				// Rate limiting summary
				if config.RateLimiting != nil {
					entry.RateLimiting = &models.RateLimitingSummary{}

					if config.RateLimiting.ProviderLevel != nil {
						entry.RateLimiting.ProviderLevel = extractRateLimitingScopeFromConfig(config.RateLimiting.ProviderLevel)
					}

					if config.RateLimiting.ConsumerLevel != nil {
						entry.RateLimiting.ConsumerLevel = extractRateLimitingScopeFromConfig(config.RateLimiting.ConsumerLevel)
					}
				}
			}
		}

		// Parse model list JSON
		if row.ModelList != "" {
			var modelProviders []models.LLMModelProvider
			if err := json.Unmarshal([]byte(row.ModelList), &modelProviders); err == nil {
				entry.ModelProviders = modelProviders
			}
		}

		entries = append(entries, entry)
	}

	// If we have entries, fetch ALL deployment data in a single query
	if len(entries) > 0 {
		if err := r.populateDeploymentsInBatch(entries, filters.OrganizationUUID); err != nil {
			// Use GORM's logger to log the error with context
			// Deployments are optional enrichment, so we don't fail the entire operation
			r.db.Logger.Error(context.Background(),
				"Failed to populate deployments for catalog entries, returning entries without deployment info: %v (orgUUID=%s, entryCount=%d)",
				err, filters.OrganizationUUID, len(entries))
		}
	}

	return entries, total, nil
}

// populateDeploymentsInBatch fetches deployment data for all providers in a single query
func (r *CatalogRepo) populateDeploymentsInBatch(entries []models.CatalogLLMProviderEntry, orgUUID string) error {
	// Extract all provider UUIDs
	providerUUIDs := make([]string, len(entries))
	for i, entry := range entries {
		providerUUIDs[i] = entry.UUID.String()
	}

	// Single query to fetch ALL deployments with gateway and environment info
	type DeploymentRow struct {
		ArtifactUUID        string
		GatewayUUID         string
		GatewayName         string
		GatewayDisplayName  string
		GatewayVHost        string
		EnvironmentUUID     string
		DeploymentStatus    string
		DeploymentUpdatedAt *time.Time
	}

	var deploymentRows []DeploymentRow

	// Fetch all deployments for these providers in ONE query
	query := r.db.
		Table("deployment_status ds").
		Select(`
			ds.artifact_uuid,
			ds.gateway_uuid,
			g.name as gateway_name,
			g.display_name as gateway_display_name,
			g.vhost as gateway_vhost,
			COALESCE(gem.environment_uuid::text, '') as environment_uuid,
			ds.status as deployment_status,
			ds.updated_at as deployment_updated_at
		`).
		Joins("JOIN gateways g ON ds.gateway_uuid = g.uuid").
		Joins("LEFT JOIN gateway_environment_mappings gem ON ds.gateway_uuid = gem.gateway_uuid").
		Where("ds.artifact_uuid IN ? AND ds.organization_uuid = ? AND ds.status = ?",
			providerUUIDs, orgUUID, models.DeploymentStatusDeployed)

	if err := query.Scan(&deploymentRows).Error; err != nil {
		return fmt.Errorf("failed to fetch deployments: %w", err)
	}

	// Group deployments by provider UUID
	deploymentMap := make(map[string][]DeploymentRow)
	for _, row := range deploymentRows {
		deploymentMap[row.ArtifactUUID] = append(deploymentMap[row.ArtifactUUID], row)
	}

	// Track metrics for skipped deployments due to data integrity issues
	skippedCount := 0

	// Populate deployment summaries for each entry
	for i := range entries {
		providerUUIDStr := entries[i].UUID.String()
		deploymentRows, ok := deploymentMap[providerUUIDStr]

		if !ok || len(deploymentRows) == 0 {
			entries[i].Deployments = []models.DeploymentSummary{}
			continue
		}

		deployments := make([]models.DeploymentSummary, 0, len(deploymentRows))
		for _, row := range deploymentRows {
			gatewayUUID, err := uuid.Parse(row.GatewayUUID)
			if err != nil {
				// Log invalid gateway UUID but skip this deployment instead of failing entire operation
				// This indicates data corruption in deployment_status or gateways table
				r.db.Logger.Warn(context.Background(),
					"Invalid gateway UUID in deployment_status, skipping deployment (gatewayUUID=%s, artifactUUID=%s): %v",
					row.GatewayUUID, row.ArtifactUUID, err)
				skippedCount++
				continue
			}

			var status models.DeploymentStatus
			switch row.DeploymentStatus {
			case string(models.DeploymentStatusDeployed):
				status = models.DeploymentStatusDeployed
			case string(models.DeploymentStatusUndeployed):
				status = models.DeploymentStatusUndeployed
			case string(models.DeploymentStatusArchived):
				status = models.DeploymentStatusArchived
			default:
				// Log unknown deployment status
				r.db.Logger.Warn(context.Background(),
					"Unknown deployment status encountered, defaulting to deployed (status=%s, artifactUUID=%s, gatewayUUID=%s)",
					row.DeploymentStatus, row.ArtifactUUID, row.GatewayUUID)
				status = models.DeploymentStatusDeployed
			}

			deployment := models.DeploymentSummary{
				GatewayID:       gatewayUUID,
				GatewayName:     row.GatewayName,
				EnvironmentName: row.EnvironmentUUID,
				Status:          status,
				VHost:           row.GatewayVHost,
			}
			deployments = append(deployments, deployment)
		}

		entries[i].Deployments = deployments
	}

	// Log summary if any deployments were skipped due to data integrity issues
	if skippedCount > 0 {
		r.db.Logger.Warn(context.Background(),
			"Skipped %d deployments due to invalid gateway UUIDs. This may indicate database corruption. (orgUUID=%s)",
			skippedCount, orgUUID)
	}

	return nil
}

// extractRateLimitingScopeFromConfig extracts rate limiting scope summary from configuration
func extractRateLimitingScopeFromConfig(scopeConfig *models.RateLimitingScopeConfig) *models.RateLimitingScope {
	scope := &models.RateLimitingScope{
		GlobalEnabled:       scopeConfig.Global != nil,
		ResourceWiseEnabled: scopeConfig.ResourceWise != nil,
	}

	// Extract global limits if present
	if scopeConfig.Global != nil {
		if scopeConfig.Global.Request != nil && scopeConfig.Global.Request.Enabled {
			scope.RequestLimitCount = &scopeConfig.Global.Request.Count
		}
		if scopeConfig.Global.Token != nil && scopeConfig.Global.Token.Enabled {
			scope.TokenLimitCount = &scopeConfig.Global.Token.Count
		}
		if scopeConfig.Global.Cost != nil && scopeConfig.Global.Cost.Enabled {
			scope.CostLimitAmount = &scopeConfig.Global.Cost.Amount
		}
	}

	return scope
}

// escapeLikeWildcards escapes LIKE pattern wildcards (% and _) to prevent SQL injection
// when using user input in LIKE queries
func escapeLikeWildcards(input string) string {
	// Escape backslash first to avoid double-escaping
	escaped := strings.ReplaceAll(input, "\\", "\\\\")
	// Then escape LIKE wildcards
	escaped = strings.ReplaceAll(escaped, "%", "\\%")
	escaped = strings.ReplaceAll(escaped, "_", "\\_")
	return escaped
}
