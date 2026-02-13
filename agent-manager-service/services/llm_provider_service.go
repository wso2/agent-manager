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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

const (
	llmStatusPending  = "pending"
	llmStatusDeployed = "deployed"
	llmStatusFailed   = "failed"
)

// LLMProviderService handles LLM provider business logic
type LLMProviderService struct {
	db           *gorm.DB
	providerRepo repositories.LLMProviderRepository
	templateRepo repositories.LLMProviderTemplateRepository
	proxyRepo    repositories.LLMProxyRepository
}

// NewLLMProviderService creates a new LLM provider service
func NewLLMProviderService(
	db *gorm.DB,
	providerRepo repositories.LLMProviderRepository,
	templateRepo repositories.LLMProviderTemplateRepository,
	proxyRepo repositories.LLMProxyRepository,
) *LLMProviderService {
	return &LLMProviderService{
		db:           db,
		providerRepo: providerRepo,
		templateRepo: templateRepo,
		proxyRepo:    proxyRepo,
	}
}

// Create creates a new LLM provider
func (s *LLMProviderService) Create(orgID, createdBy string, provider *models.LLMProvider) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.Create: starting", "orgID", orgID, "createdBy", createdBy)

	if provider == nil {
		slog.Error("LLMProviderService.Create: provider is nil", "orgID", orgID)
		return nil, utils.ErrInvalidInput
	}

	// Extract handle, name, and version from configuration
	// Note: handle is not in Configuration, so we use name as handle
	name := provider.Configuration.Name
	version := provider.Configuration.Version

	// Use name as handle (artifact identifier)
	handle := name

	slog.Info("LLMProviderService.Create: extracted configuration", "orgID", orgID, "handle", handle, "name", name, "version", version)

	if handle == "" || name == "" || version == "" {
		slog.Error("LLMProviderService.Create: missing required fields", "orgID", orgID, "handle", handle, "name", name, "version", version)
		return nil, utils.ErrInvalidInput
	}

	// Validate template exists
	template := provider.Configuration.Template
	if template == "" {
		slog.Error("LLMProviderService.Create: template not specified", "orgID", orgID, "handle", handle)
		return nil, utils.ErrInvalidInput
	}

	slog.Info("LLMProviderService.Create: validating template", "orgID", orgID, "handle", handle, "template", template)
	templateExists, err := s.templateRepo.Exists(template, orgID)
	if err != nil {
		slog.Error("LLMProviderService.Create: failed to validate template", "orgID", orgID, "handle", handle, "template", template, "error", err)
		return nil, fmt.Errorf("failed to validate template: %w", err)
	}
	if !templateExists {
		slog.Warn("LLMProviderService.Create: template not found", "orgID", orgID, "handle", handle, "template", template)
		return nil, utils.ErrLLMProviderTemplateNotFound
	}

	// Check if provider already exists
	slog.Info("LLMProviderService.Create: checking if provider exists", "orgID", orgID, "handle", handle)
	exists, err := s.providerRepo.Exists(handle, orgID)
	if err != nil {
		slog.Error("LLMProviderService.Create: failed to check provider exists", "orgID", orgID, "handle", handle, "error", err)
		return nil, fmt.Errorf("failed to check provider exists: %w", err)
	}
	if exists {
		slog.Warn("LLMProviderService.Create: provider already exists", "orgID", orgID, "handle", handle)
		return nil, utils.ErrLLMProviderExists
	}

	// Parse organization UUID
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		slog.Error("LLMProviderService.Create: invalid organization UUID", "orgID", orgID, "error", err)
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Set default values
	provider.CreatedBy = createdBy
	provider.Status = llmStatusPending
	if provider.Configuration.Context == nil {
		defaultContext := "/"
		provider.Configuration.Context = &defaultContext
	}

	slog.Info("LLMProviderService.Create: set default values", "orgID", orgID, "handle", handle, "status", provider.Status, "context", *provider.Configuration.Context)

	// Serialize model providers to ModelList
	if len(provider.ModelProviders) > 0 {
		slog.Info("LLMProviderService.Create: serializing model providers", "orgID", orgID, "handle", handle, "count", len(provider.ModelProviders))
		modelListBytes, err := json.Marshal(provider.ModelProviders)
		if err != nil {
			slog.Error("LLMProviderService.Create: failed to serialize model providers", "orgID", orgID, "handle", handle, "error", err)
			return nil, fmt.Errorf("failed to serialize model providers: %w", err)
		}
		provider.ModelList = string(modelListBytes)
	}

	// Create provider in transaction
	slog.Info("LLMProviderService.Create: creating provider in database", "orgID", orgID, "handle", handle, "name", name, "version", version)
	err = s.db.Transaction(func(tx *gorm.DB) error {
		return s.providerRepo.Create(tx, provider, handle, name, version, orgUUID)
	})
	if err != nil {
		slog.Error("LLMProviderService.Create: failed to create provider", "orgID", orgID, "handle", handle, "error", err)
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	slog.Info("LLMProviderService.Create: provider created, fetching details", "orgID", orgID, "handle", handle)

	// Fetch created provider
	created, err := s.providerRepo.GetByID(handle, orgID)
	if err != nil {
		slog.Error("LLMProviderService.Create: failed to fetch created provider", "orgID", orgID, "handle", handle, "error", err)
		return nil, fmt.Errorf("failed to fetch created provider: %w", err)
	}

	// Parse model providers from ModelList
	if created.ModelList != "" {
		slog.Info("LLMProviderService.Create: parsing model providers from ModelList", "orgID", orgID, "handle", handle)
		if err := json.Unmarshal([]byte(created.ModelList), &created.ModelProviders); err != nil {
			slog.Error("LLMProviderService.Create: failed to parse model providers", "orgID", orgID, "handle", handle, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	slog.Info("LLMProviderService.Create: completed successfully", "orgID", orgID, "handle", handle, "providerUUID", created.UUID)
	return created, nil
}

// List lists all LLM providers for an organization
func (s *LLMProviderService) List(orgID string, limit, offset int) ([]*models.LLMProvider, int, error) {
	slog.Info("LLMProviderService.List: starting", "orgID", orgID, "limit", limit, "offset", offset)

	providers, err := s.providerRepo.List(orgID, limit, offset)
	if err != nil {
		slog.Error("LLMProviderService.List: failed to list providers", "orgID", orgID, "error", err)
		return nil, 0, fmt.Errorf("failed to list providers: %w", err)
	}

	slog.Info("LLMProviderService.List: providers retrieved from repository", "orgID", orgID, "count", len(providers))

	// Parse model providers for each provider
	for i, p := range providers {
		if p.ModelList != "" {
			if err := json.Unmarshal([]byte(p.ModelList), &p.ModelProviders); err != nil {
				slog.Error("LLMProviderService.List: failed to parse model providers", "orgID", orgID, "providerIndex", i, "providerUUID", p.UUID, "error", err)
				return nil, 0, fmt.Errorf("failed to parse model providers: %w", err)
			}
		}
	}

	totalCount, err := s.providerRepo.Count(orgID)
	if err != nil {
		slog.Error("LLMProviderService.List: failed to count providers", "orgID", orgID, "error", err)
		return nil, 0, fmt.Errorf("failed to count providers: %w", err)
	}

	slog.Info("LLMProviderService.List: completed successfully", "orgID", orgID, "count", len(providers), "total", totalCount)
	return providers, totalCount, nil
}

// Get retrieves an LLM provider by ID
func (s *LLMProviderService) Get(providerID, orgID string) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.Get: starting", "orgID", orgID, "providerID", providerID)

	if providerID == "" {
		slog.Error("LLMProviderService.Get: providerID is empty", "orgID", orgID)
		return nil, utils.ErrInvalidInput
	}

	provider, err := s.providerRepo.GetByID(providerID, orgID)
	if err != nil {
		slog.Error("LLMProviderService.Get: failed to get provider", "orgID", orgID, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		slog.Warn("LLMProviderService.Get: provider not found", "orgID", orgID, "providerID", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Parse model providers from ModelList
	if provider.ModelList != "" {
		slog.Info("LLMProviderService.Get: parsing model providers", "orgID", orgID, "providerID", providerID, "providerUUID", provider.UUID)
		if err := json.Unmarshal([]byte(provider.ModelList), &provider.ModelProviders); err != nil {
			slog.Error("LLMProviderService.Get: failed to parse model providers", "orgID", orgID, "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	slog.Info("LLMProviderService.Get: completed successfully", "orgID", orgID, "providerID", providerID, "providerUUID", provider.UUID)
	return provider, nil
}

// Update updates an existing LLM provider
func (s *LLMProviderService) Update(providerID, orgID string, updates *models.LLMProvider) (*models.LLMProvider, error) {
	slog.Info("LLMProviderService.Update: starting", "orgID", orgID, "providerID", providerID)

	if providerID == "" || updates == nil {
		slog.Error("LLMProviderService.Update: invalid input", "orgID", orgID, "providerID", providerID, "updatesIsNil", updates == nil)
		return nil, utils.ErrInvalidInput
	}

	// Validate template exists
	template := updates.Configuration.Template
	if template != "" {
		slog.Info("LLMProviderService.Update: validating template", "orgID", orgID, "providerID", providerID, "template", template)
		templateExists, err := s.templateRepo.Exists(template, orgID)
		if err != nil {
			slog.Error("LLMProviderService.Update: failed to validate template", "orgID", orgID, "providerID", providerID, "template", template, "error", err)
			return nil, fmt.Errorf("failed to validate template: %w", err)
		}
		if !templateExists {
			slog.Warn("LLMProviderService.Update: template not found", "orgID", orgID, "providerID", providerID, "template", template)
			return nil, utils.ErrLLMProviderTemplateNotFound
		}
	}

	// Parse organization UUID
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		slog.Error("LLMProviderService.Update: invalid organization UUID", "orgID", orgID, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Serialize model providers to ModelList
	if len(updates.ModelProviders) > 0 {
		slog.Info("LLMProviderService.Update: serializing model providers", "orgID", orgID, "providerID", providerID, "count", len(updates.ModelProviders))
		modelListBytes, err := json.Marshal(updates.ModelProviders)
		if err != nil {
			slog.Error("LLMProviderService.Update: failed to serialize model providers", "orgID", orgID, "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to serialize model providers: %w", err)
		}
		updates.ModelList = string(modelListBytes)
	}

	// Update provider
	slog.Info("LLMProviderService.Update: updating provider in database", "orgID", orgID, "providerID", providerID)
	if err := s.providerRepo.Update(updates, providerID, orgUUID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Update: provider not found", "orgID", orgID, "providerID", providerID)
			return nil, utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Update: failed to update provider", "orgID", orgID, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	// Fetch updated provider
	slog.Info("LLMProviderService.Update: fetching updated provider", "orgID", orgID, "providerID", providerID)
	updated, err := s.providerRepo.GetByID(providerID, orgID)
	if err != nil {
		slog.Error("LLMProviderService.Update: failed to fetch updated provider", "orgID", orgID, "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to fetch updated provider: %w", err)
	}
	if updated == nil {
		slog.Warn("LLMProviderService.Update: updated provider not found", "orgID", orgID, "providerID", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Parse model providers from ModelList
	if updated.ModelList != "" {
		slog.Info("LLMProviderService.Update: parsing model providers", "orgID", orgID, "providerID", providerID)
		if err := json.Unmarshal([]byte(updated.ModelList), &updated.ModelProviders); err != nil {
			slog.Error("LLMProviderService.Update: failed to parse model providers", "orgID", orgID, "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	slog.Info("LLMProviderService.Update: completed successfully", "orgID", orgID, "providerID", providerID, "providerUUID", updated.UUID)
	return updated, nil
}

// Delete deletes an LLM provider
func (s *LLMProviderService) Delete(providerID, orgID string) error {
	slog.Info("LLMProviderService.Delete: starting", "orgID", orgID, "providerID", providerID)

	if providerID == "" {
		slog.Error("LLMProviderService.Delete: providerID is empty", "orgID", orgID)
		return utils.ErrInvalidInput
	}

	slog.Info("LLMProviderService.Delete: deleting provider from database", "orgID", orgID, "providerID", providerID)
	if err := s.providerRepo.Delete(providerID, orgID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			slog.Warn("LLMProviderService.Delete: provider not found", "orgID", orgID, "providerID", providerID)
			return utils.ErrLLMProviderNotFound
		}
		slog.Error("LLMProviderService.Delete: failed to delete provider", "orgID", orgID, "providerID", providerID, "error", err)
		return fmt.Errorf("failed to delete provider: %w", err)
	}

	slog.Info("LLMProviderService.Delete: completed successfully", "orgID", orgID, "providerID", providerID)
	return nil
}

// ListProxiesByProvider lists all LLM proxies for a provider
func (s *LLMProviderService) ListProxiesByProvider(providerID, orgID string, limit, offset int) ([]*models.LLMProxy, int, error) {
	if providerID == "" {
		return nil, 0, utils.ErrInvalidInput
	}

	// Get provider to get its UUID
	provider, err := s.providerRepo.GetByID(providerID, orgID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return nil, 0, utils.ErrLLMProviderNotFound
	}

	// List proxies by provider UUID
	proxies, err := s.proxyRepo.ListByProvider(orgID, provider.UUID.String(), limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list proxies by provider: %w", err)
	}

	totalCount, err := s.proxyRepo.CountByProvider(orgID, provider.UUID.String())
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count proxies by provider: %w", err)
	}

	return proxies, totalCount, nil
}
