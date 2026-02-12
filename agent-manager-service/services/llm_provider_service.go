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

package services

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
	"gorm.io/gorm"
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
	if provider == nil {
		return nil, utils.ErrInvalidInput
	}

	// Extract handle, name, and version from configuration
	handle := provider.Artifact.Handle
	name := provider.Configuration.Name
	version := provider.Configuration.Version

	if handle == "" || name == "" || version == "" {
		return nil, utils.ErrInvalidInput
	}

	// Validate template exists
	template := provider.Configuration.Template
	if template == "" {
		return nil, utils.ErrInvalidInput
	}

	templateExists, err := s.templateRepo.Exists(template, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate template: %w", err)
	}
	if !templateExists {
		return nil, utils.ErrLLMProviderTemplateNotFound
	}

	// Check if provider already exists
	exists, err := s.providerRepo.Exists(handle, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to check provider exists: %w", err)
	}
	if exists {
		return nil, utils.ErrLLMProviderExists
	}

	// Parse organization UUID
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Set default values
	provider.CreatedBy = createdBy
	provider.Status = llmStatusPending
	if provider.Configuration.Context == nil {
		defaultContext := "/"
		provider.Configuration.Context = &defaultContext
	}

	// Serialize model providers to ModelList
	if len(provider.ModelProviders) > 0 {
		modelListBytes, err := json.Marshal(provider.ModelProviders)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize model providers: %w", err)
		}
		provider.ModelList = string(modelListBytes)
	}

	// Create provider in transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		return s.providerRepo.Create(tx, provider, handle, name, version, orgUUID)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	// Fetch created provider
	created, err := s.providerRepo.GetByID(handle, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch created provider: %w", err)
	}

	// Parse model providers from ModelList
	if created.ModelList != "" {
		if err := json.Unmarshal([]byte(created.ModelList), &created.ModelProviders); err != nil {
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	return created, nil
}

// List lists all LLM providers for an organization
func (s *LLMProviderService) List(orgID string, limit, offset int) ([]*models.LLMProvider, int, error) {
	providers, err := s.providerRepo.List(orgID, limit, offset)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list providers: %w", err)
	}

	// Parse model providers for each provider
	for _, p := range providers {
		if p.ModelList != "" {
			if err := json.Unmarshal([]byte(p.ModelList), &p.ModelProviders); err != nil {
				return nil, 0, fmt.Errorf("failed to parse model providers: %w", err)
			}
		}
	}

	totalCount, err := s.providerRepo.Count(orgID)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to count providers: %w", err)
	}

	return providers, totalCount, nil
}

// Get retrieves an LLM provider by ID
func (s *LLMProviderService) Get(providerID, orgID string) (*models.LLMProvider, error) {
	if providerID == "" {
		return nil, utils.ErrInvalidInput
	}

	provider, err := s.providerRepo.GetByID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}

	// Parse model providers from ModelList
	if provider.ModelList != "" {
		if err := json.Unmarshal([]byte(provider.ModelList), &provider.ModelProviders); err != nil {
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	return provider, nil
}

// Update updates an existing LLM provider
func (s *LLMProviderService) Update(providerID, orgID string, updates *models.LLMProvider) (*models.LLMProvider, error) {
	if providerID == "" || updates == nil {
		return nil, utils.ErrInvalidInput
	}

	// Validate template exists
	template := updates.Configuration.Template
	if template != "" {
		templateExists, err := s.templateRepo.Exists(template, orgID)
		if err != nil {
			return nil, fmt.Errorf("failed to validate template: %w", err)
		}
		if !templateExists {
			return nil, utils.ErrLLMProviderTemplateNotFound
		}
	}

	// Parse organization UUID
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Serialize model providers to ModelList
	if len(updates.ModelProviders) > 0 {
		modelListBytes, err := json.Marshal(updates.ModelProviders)
		if err != nil {
			return nil, fmt.Errorf("failed to serialize model providers: %w", err)
		}
		updates.ModelList = string(modelListBytes)
	}

	// Update provider
	if err := s.providerRepo.Update(updates, providerID, orgUUID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProviderNotFound
		}
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	// Fetch updated provider
	updated, err := s.providerRepo.GetByID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated provider: %w", err)
	}
	if updated == nil {
		return nil, utils.ErrLLMProviderNotFound
	}

	// Parse model providers from ModelList
	if updated.ModelList != "" {
		if err := json.Unmarshal([]byte(updated.ModelList), &updated.ModelProviders); err != nil {
			return nil, fmt.Errorf("failed to parse model providers: %w", err)
		}
	}

	return updated, nil
}

// Delete deletes an LLM provider
func (s *LLMProviderService) Delete(providerID, orgID string) error {
	if providerID == "" {
		return utils.ErrInvalidInput
	}

	if err := s.providerRepo.Delete(providerID, orgID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrLLMProviderNotFound
		}
		return fmt.Errorf("failed to delete provider: %w", err)
	}

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
