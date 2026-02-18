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

package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
)

// CatalogService defines the interface for catalog operations
type CatalogService interface {
	ListCatalog(ctx context.Context, orgUUID string, kind string, limit, offset int) ([]models.CatalogEntry, int64, error)
	ListLLMProviders(ctx context.Context, orgUUID string, environmentName *string, limit, offset int) ([]models.CatalogLLMProviderEntry, int64, error)
}

type catalogService struct {
	logger           *slog.Logger
	catalogRepo      repositories.CatalogRepository
	deploymentRepo   repositories.DeploymentRepository
	gatewayRepo      repositories.GatewayRepository
	db               *gorm.DB
	openChoreoClient client.OpenChoreoClient
}

// NewCatalogService creates a new catalog service
func NewCatalogService(
	logger *slog.Logger,
	catalogRepo repositories.CatalogRepository,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	db *gorm.DB,
	openChoreoClient client.OpenChoreoClient,
) CatalogService {
	return &catalogService{
		logger:           logger,
		catalogRepo:      catalogRepo,
		deploymentRepo:   deploymentRepo,
		gatewayRepo:      gatewayRepo,
		db:               db,
		openChoreoClient: openChoreoClient,
	}
}

// ListCatalog retrieves catalog entries filtered by kind and organization
func (s *catalogService) ListCatalog(ctx context.Context, orgUUID string, kind string, limit, offset int) ([]models.CatalogEntry, int64, error) {
	s.logger.Info("Listing catalog entries",
		"orgUUID", orgUUID,
		"kind", kind,
		"limit", limit,
		"offset", offset)

	// Validate orgUUID
	if _, err := uuid.Parse(orgUUID); err != nil {
		s.logger.Error("Invalid organization UUID", "orgUUID", orgUUID, "error", err)
		return nil, 0, fmt.Errorf("invalid organization UUID: %w", err)
	}

	var entries []models.CatalogEntry
	var total int64
	var err error

	// Query based on kind filter
	if kind == "" {
		// No kind filter - return all catalog entries
		entries, total, err = s.catalogRepo.ListAll(orgUUID, limit, offset)
	} else {
		// Filter by specific kind
		entries, total, err = s.catalogRepo.ListByKind(orgUUID, kind, limit, offset)
	}

	if err != nil {
		s.logger.Error("Failed to list catalog entries",
			"orgUUID", orgUUID,
			"kind", kind,
			"error", err)
		return nil, 0, fmt.Errorf("failed to list catalog entries: %w", err)
	}

	s.logger.Info("Successfully listed catalog entries",
		"count", len(entries),
		"total", total)

	return entries, total, nil
}

// ListLLMProviders retrieves comprehensive LLM provider catalog entries with deployment details
func (s *catalogService) ListLLMProviders(ctx context.Context, orgUUID string, environmentName *string, limit, offset int) ([]models.CatalogLLMProviderEntry, int64, error) {
	s.logger.Info("Listing LLM provider catalog entries",
		"orgUUID", orgUUID,
		"environmentName", environmentName,
		"limit", limit,
		"offset", offset)

	// Validate orgUUID
	orgParsed, err := uuid.Parse(orgUUID)
	if err != nil {
		s.logger.Error("Invalid organization UUID", "orgUUID", orgUUID, "error", err)
		return nil, 0, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Get ALL catalog providers from repository (filtering happens in service layer)
	entries, total, err := s.catalogRepo.ListLLMProviders(orgUUID, environmentName, limit, offset)
	if err != nil {
		s.logger.Error("Failed to list LLM providers from repository",
			"orgUUID", orgUUID,
			"error", err)
		return nil, 0, fmt.Errorf("failed to list LLM providers: %w", err)
	}

	// Enrich each entry with deployment details, configuration, and summaries
	for i := range entries {
		entry := &entries[i]

		// Parse and populate configuration details
		if err := s.populateConfiguration(entry); err != nil {
			s.logger.Warn("Failed to populate configuration",
				"providerUUID", entry.UUID,
				"error", err)
		}

		// Populate deployment information (includes environment lookup)
		if err := s.populateDeployments(entry, orgParsed); err != nil {
			s.logger.Warn("Failed to populate deployments",
				"providerUUID", entry.UUID,
				"error", err)
		}
	}

	// Filter by environment if specified (done after enrichment)
	if environmentName != nil && *environmentName != "" {
		filteredEntries := make([]models.CatalogLLMProviderEntry, 0)
		for _, entry := range entries {
			// Check if any deployment is in the requested environment
			for _, deployment := range entry.Deployments {
				if deployment.EnvironmentName == *environmentName && deployment.Status == models.DeploymentStatusDeployed {
					filteredEntries = append(filteredEntries, entry)
					break
				}
			}
		}
		entries = filteredEntries
		total = int64(len(filteredEntries))
	}

	s.logger.Info("Successfully listed LLM provider catalog entries",
		"count", len(entries),
		"total", total)

	return entries, total, nil
}

// populateConfiguration parses configuration and populates summary fields
func (s *catalogService) populateConfiguration(entry *models.CatalogLLMProviderEntry) error {
	// Query the provider to get configuration as raw JSON
	var provider models.LLMProvider
	if err := s.db.
		Table("llm_providers").
		Where("uuid = ?", entry.UUID).
		First(&provider).Error; err != nil {
		return fmt.Errorf("failed to get provider configuration: %w", err)
	}

	// Populate basic config fields
	entry.Template = provider.Configuration.Template
	entry.Context = provider.Configuration.Context
	entry.VHost = provider.Configuration.VHost

	// Parse model providers
	if provider.ModelList != "" {
		var modelProviders []models.LLMModelProvider
		if err := json.Unmarshal([]byte(provider.ModelList), &modelProviders); err == nil {
			entry.ModelProviders = modelProviders
		}
	}

	// Populate security summary
	if provider.Configuration.Security != nil {
		entry.Security = &models.SecuritySummary{
			Enabled:       provider.Configuration.Security.Enabled != nil && *provider.Configuration.Security.Enabled,
			APIKeyEnabled: provider.Configuration.Security.APIKey != nil && provider.Configuration.Security.APIKey.Enabled != nil && *provider.Configuration.Security.APIKey.Enabled,
		}
		if provider.Configuration.Security.APIKey != nil {
			entry.Security.APIKeyIn = provider.Configuration.Security.APIKey.In
		}
	}

	// Populate rate limiting summary
	if provider.Configuration.RateLimiting != nil {
		entry.RateLimiting = &models.RateLimitingSummary{}

		if provider.Configuration.RateLimiting.ProviderLevel != nil {
			entry.RateLimiting.ProviderLevel = extractRateLimitingScope(provider.Configuration.RateLimiting.ProviderLevel)
		}

		if provider.Configuration.RateLimiting.ConsumerLevel != nil {
			entry.RateLimiting.ConsumerLevel = extractRateLimitingScope(provider.Configuration.RateLimiting.ConsumerLevel)
		}
	}

	return nil
}

// extractRateLimitingScope extracts rate limiting scope summary
func extractRateLimitingScope(scopeConfig *models.RateLimitingScopeConfig) *models.RateLimitingScope {
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

// populateDeployments fetches and populates deployment information for a provider
func (s *catalogService) populateDeployments(entry *models.CatalogLLMProviderEntry, orgUUID uuid.UUID) error {
	// Get deployed gateways for this provider
	gatewayIDs, err := s.deploymentRepo.GetDeployedGatewaysByProvider(entry.UUID, orgUUID)
	if err != nil {
		return fmt.Errorf("failed to get deployed gateways: %w", err)
	}

	if len(gatewayIDs) == 0 {
		entry.Deployments = []models.DeploymentSummary{}
		return nil
	}

	// Build map of gateway UUID to environment UUID from gateway_environment_mappings
	gatewayEnvMap := make(map[string]string) // gatewayUUID -> environmentUUID
	var mappings []struct {
		GatewayUUID     string `gorm:"column:gateway_uuid"`
		EnvironmentUUID string `gorm:"column:environment_uuid"`
	}

	if err := s.db.
		Table("gateway_environment_mappings").
		Select("gateway_uuid, environment_uuid").
		Where("gateway_uuid IN ?", gatewayIDs).
		Scan(&mappings).Error; err != nil {
		s.logger.Warn("Failed to get gateway-environment mappings", "error", err)
	}

	for _, mapping := range mappings {
		gatewayEnvMap[mapping.GatewayUUID] = mapping.EnvironmentUUID
	}

	// Fetch deployment details for each gateway
	deployments := make([]models.DeploymentSummary, 0, len(gatewayIDs))
	for _, gatewayID := range gatewayIDs {
		gatewayUUID, err := uuid.Parse(gatewayID)
		if err != nil {
			s.logger.Warn("Invalid gateway UUID", "gatewayID", gatewayID, "error", err)
			continue
		}

		// Get gateway details
		gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
		if err != nil {
			s.logger.Warn("Failed to get gateway details", "gatewayID", gatewayID, "error", err)
			continue
		}

		// Get deployment status
		var deployment struct {
			Status    models.DeploymentStatus `gorm:"column:status"`
			UpdatedAt *time.Time              `gorm:"column:updated_at"`
		}

		if err := s.db.
			Table("deployment_status").
			Select("status, updated_at").
			Where("artifact_uuid = ? AND gateway_uuid = ? AND organization_uuid = ?", entry.UUID, gatewayUUID, orgUUID).
			Scan(&deployment).Error; err != nil {
			s.logger.Warn("Failed to get deployment status", "gatewayID", gatewayID, "error", err)
			continue
		}

		// Get environment name - fallback to gateway name if no mapping
		environmentName := gateway.Name
		if envUUID, ok := gatewayEnvMap[gatewayID]; ok && envUUID != "" {
			// Try to get environment name from OpenChoreo (this could be optimized with caching)
			// For now, use the environment UUID as name - service layer can enhance this
			environmentName = envUUID
		}

		// Create deployment summary
		deploymentSummary := models.DeploymentSummary{
			GatewayID:          gatewayUUID,
			GatewayName:        gateway.Name,
			GatewayDisplayName: gateway.DisplayName,
			EnvironmentName:    environmentName,
			Status:             deployment.Status,
			DeployedAt:         deployment.UpdatedAt,
			VHost:              gateway.Vhost,
		}

		deployments = append(deployments, deploymentSummary)
	}

	entry.Deployments = deployments
	return nil
}
