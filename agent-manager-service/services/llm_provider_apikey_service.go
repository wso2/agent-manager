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
	"fmt"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// LLMProviderAPIKeyService handles API key management for LLM providers
type LLMProviderAPIKeyService struct {
	providerRepo repositories.LLMProviderRepository
	broadcaster  apiKeyBroadcaster
}

// NewLLMProviderAPIKeyService creates a new LLM provider API key service instance
func NewLLMProviderAPIKeyService(
	providerRepo repositories.LLMProviderRepository,
	gatewayRepo repositories.GatewayRepository,
	gatewayService *GatewayEventsService,
) *LLMProviderAPIKeyService {
	return &LLMProviderAPIKeyService{
		providerRepo: providerRepo,
		broadcaster: apiKeyBroadcaster{
			gatewayRepo:    gatewayRepo,
			gatewayService: gatewayService,
		},
	}
}

// CreateAPIKey generates an API key for an LLM provider and broadcasts it to all gateways
func (s *LLMProviderAPIKeyService) CreateAPIKey(
	ctx context.Context,
	orgID, providerID string,
	req *models.CreateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}
	return s.broadcaster.broadcastCreate(orgID, provider.Artifact.Handle, req)
}

// RevokeAPIKey broadcasts an API key revocation event to all gateways for this organization.
func (s *LLMProviderAPIKeyService) RevokeAPIKey(
	ctx context.Context,
	orgID, providerID, keyName string,
) error {
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return utils.ErrLLMProviderNotFound
	}
	return s.broadcaster.broadcastRevoke(orgID, provider.Artifact.Handle, keyName)
}

// RotateAPIKey generates a new API key value and broadcasts the update to all gateways.
// Returns the new API key (shown once) and its identifier.
func (s *LLMProviderAPIKeyService) RotateAPIKey(
	ctx context.Context,
	orgID, providerID, keyName string,
	req *models.RotateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}
	return s.broadcaster.broadcastRotate(orgID, provider.Artifact.Handle, keyName, req)
}
