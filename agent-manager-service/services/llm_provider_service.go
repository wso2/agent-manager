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
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// LLMProviderService handles LLM provider business logic
type LLMProviderService struct {
	providerRepo repositories.LLMProviderRepository
	templateRepo repositories.LLMProviderTemplateRepository
}

// NewLLMProviderService creates a new LLM provider service
func NewLLMProviderService(
	providerRepo repositories.LLMProviderRepository,
	templateRepo repositories.LLMProviderTemplateRepository,
) *LLMProviderService {
	return &LLMProviderService{
		providerRepo: providerRepo,
		templateRepo: templateRepo,
	}
}

// Create creates a new LLM provider
// TODO: Implement using providerRepo.Create() and providerRepo.GetByID()
func (s *LLMProviderService) Create(orgID, createdBy string, provider *models.LLMProvider) (*models.LLMProvider, error) {
	return nil, utils.ErrNotImplemented
}

// List lists all LLM providers for an organization
// TODO: Implement using providerRepo.List() and providerRepo.Count()
func (s *LLMProviderService) List(orgID string, limit, offset int) ([]*models.LLMProvider, int, error) {
	return nil, 0, utils.ErrNotImplemented
}

// Get retrieves an LLM provider by ID
// TODO: Implement using providerRepo.GetByID()
func (s *LLMProviderService) Get(providerID, orgID string) (*models.LLMProvider, error) {
	return nil, utils.ErrNotImplemented
}

// Update updates an existing LLM provider
// TODO: Implement using providerRepo.Update() and providerRepo.GetByID()
func (s *LLMProviderService) Update(providerID, orgID string, updates *models.LLMProvider) (*models.LLMProvider, error) {
	return nil, utils.ErrNotImplemented
}

// Delete deletes an LLM provider
// TODO: Implement using providerRepo.Delete()
func (s *LLMProviderService) Delete(providerID, orgID string) error {
	return utils.ErrNotImplemented
}

// ListProxiesByProvider lists all LLM proxies for a provider
// TODO: Implement - requires proxy repository injection
func (s *LLMProviderService) ListProxiesByProvider(providerID, orgID string) ([]*models.LLMProxy, error) {
	return nil, utils.ErrNotImplemented
}
