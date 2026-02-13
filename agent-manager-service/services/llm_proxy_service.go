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
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// LLMProxyService handles LLM proxy business logic
type LLMProxyService struct {
	proxyRepo    repositories.LLMProxyRepository
	providerRepo repositories.LLMProviderRepository
}

// NewLLMProxyService creates a new LLM proxy service
func NewLLMProxyService(
	proxyRepo repositories.LLMProxyRepository,
	providerRepo repositories.LLMProviderRepository,
) *LLMProxyService {
	return &LLMProxyService{
		proxyRepo:    proxyRepo,
		providerRepo: providerRepo,
	}
}

// Create creates a new LLM proxy
func (s *LLMProxyService) Create(orgID, createdBy string, proxy *models.LLMProxy) (*models.LLMProxy, error) {
	if proxy == nil {
		return nil, utils.ErrInvalidInput
	}

	// Extract handle, name, and version from configuration
	// Note: handle is not in Configuration, so we use name as handle
	name := proxy.Configuration.Name
	version := proxy.Configuration.Version
	provider := proxy.Configuration.Provider

	// Use name as handle (artifact identifier)
	handle := name

	if handle == "" || name == "" || version == "" || provider == "" {
		return nil, utils.ErrInvalidInput
	}

	if proxy.ProjectUUID == uuid.Nil {
		return nil, utils.ErrInvalidInput
	}
	// Validate provider exists
	providerModel, err := s.providerRepo.GetByUUID(provider, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to validate provider: %w", err)
	}
	if providerModel == nil {
		return nil, utils.ErrLLMProviderNotFound
	}

	// Check if proxy already exists
	exists, err := s.proxyRepo.Exists(handle, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to check proxy exists: %w", err)
	}
	if exists {
		return nil, utils.ErrLLMProxyExists
	}

	// Parse organization UUID
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Set default values
	proxy.CreatedBy = createdBy
	proxy.ProviderUUID = providerModel.UUID
	proxy.Status = llmStatusPending
	if proxy.Configuration.Context == nil {
		defaultContext := "/"
		proxy.Configuration.Context = &defaultContext
	}

	// Create proxy
	if err := s.proxyRepo.Create(proxy, handle, name, version, orgUUID); err != nil {
		return nil, fmt.Errorf("failed to create proxy: %w", err)
	}

	// Fetch created proxy
	created, err := s.proxyRepo.GetByID(handle, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch created proxy: %w", err)
	}

	return created, nil
}

// List lists all LLM proxies for an organization
func (s *LLMProxyService) List(orgID string, projectID *string, limit, offset int) ([]*models.LLMProxy, int, error) {
	var proxies []*models.LLMProxy
	var totalCount int
	var err error

	if projectID != nil && *projectID != "" {
		proxies, err = s.proxyRepo.ListByProject(orgID, *projectID, limit, offset)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to list proxies by project: %w", err)
		}

		totalCount, err = s.proxyRepo.CountByProject(orgID, *projectID)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to count proxies by project: %w", err)
		}
	} else {
		proxies, err = s.proxyRepo.List(orgID, limit, offset)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to list proxies: %w", err)
		}

		totalCount, err = s.proxyRepo.Count(orgID)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to count proxies: %w", err)
		}
	}

	return proxies, totalCount, nil
}

// Get retrieves an LLM proxy by ID
func (s *LLMProxyService) Get(proxyID, orgID string) (*models.LLMProxy, error) {
	if proxyID == "" {
		return nil, utils.ErrInvalidInput
	}

	proxy, err := s.proxyRepo.GetByID(proxyID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}

	return proxy, nil
}

// Update updates an existing LLM proxy
func (s *LLMProxyService) Update(proxyID, orgID string, updates *models.LLMProxy) (*models.LLMProxy, error) {
	if proxyID == "" || updates == nil {
		return nil, utils.ErrInvalidInput
	}

	// Validate provider if specified
	provider := updates.Configuration.Provider
	if provider != "" {
		providerModel, err := s.providerRepo.GetByUUID(provider, orgID)
		if err != nil {
			return nil, fmt.Errorf("failed to validate provider: %w", err)
		}
		if providerModel == nil {
			return nil, utils.ErrLLMProviderNotFound
		}
		updates.ProviderUUID = providerModel.UUID
	}

	// Parse organization UUID
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Update proxy
	if err := s.proxyRepo.Update(updates, proxyID, orgUUID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to update proxy: %w", err)
	}

	// Fetch updated proxy
	updated, err := s.proxyRepo.GetByID(proxyID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch updated proxy: %w", err)
	}
	if updated == nil {
		return nil, utils.ErrLLMProxyNotFound
	}

	return updated, nil
}

// Delete deletes an LLM proxy
func (s *LLMProxyService) Delete(proxyID, orgID string) error {
	if proxyID == "" {
		return utils.ErrInvalidInput
	}

	if err := s.proxyRepo.Delete(proxyID, orgID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrLLMProxyNotFound
		}
		return fmt.Errorf("failed to delete proxy: %w", err)
	}

	return nil
}

// ListByProvider lists all proxies for a specific provider
func (s *LLMProxyService) ListByProvider(orgID, providerID string, limit, offset int) ([]*models.LLMProxy, int, error) {
	if providerID == "" {
		return nil, 0, utils.ErrInvalidInput
	}

	// Get provider to get its UUID
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
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
