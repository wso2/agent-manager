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
	providerRepo   repositories.LLMProviderRepository
	gatewayRepo    repositories.GatewayRepository
	gatewayService *GatewayEventsService
}

// NewLLMProviderAPIKeyService creates a new LLM provider API key service instance
func NewLLMProviderAPIKeyService(
	providerRepo repositories.LLMProviderRepository,
	gatewayRepo repositories.GatewayRepository,
	gatewayService *GatewayEventsService,
) *LLMProviderAPIKeyService {
	return &LLMProviderAPIKeyService{
		providerRepo:   providerRepo,
		gatewayRepo:    gatewayRepo,
		gatewayService: gatewayService,
	}
}

// CreateAPIKey generates an API key for an LLM provider and broadcasts it to all gateways
func (s *LLMProviderAPIKeyService) CreateAPIKey(
	ctx context.Context,
	orgID, providerID string,
	req *models.CreateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	// Validate provider exists
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}

	// Generate API key
	apiKey, err := utils.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Determine key name and display name
	var keyName string
	if req.Name != "" {
		keyName = req.Name
	} else {
		keyName, err = utils.GenerateHandle(req.DisplayName)
		if err != nil {
			return nil, fmt.Errorf("failed to generate API key name: %w", err)
		}
	}

	displayName := req.DisplayName
	if displayName == "" {
		displayName = keyName
	}

	// Get all gateways for this organization
	gateways, err := s.gatewayRepo.GetByOrganizationID(orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateways: %w", err)
	}

	if len(gateways) == 0 {
		return nil, utils.ErrGatewayNotFound
	}

	// Create API key event
	event := &models.APIKeyCreatedEvent{
		APIID:       provider.Artifact.Name,
		Name:        keyName,
		DisplayName: displayName,
		APIKey:      apiKey,
		Operations:  "[\"*\"]", // All operations
		ExpiresAt:   req.ExpiresAt,
	}

	// Broadcast to all gateways
	successCount := 0
	var lastError error
	for _, gateway := range gateways {
		if err := s.gatewayService.BroadcastAPIKeyCreatedEvent(gateway.UUID.String(), event); err != nil {
			lastError = err
			// Log error but continue to try other gateways
		} else {
			successCount++
		}
	}

	if successCount == 0 && lastError != nil {
		return nil, fmt.Errorf("failed to deliver API key to any gateway: %w", lastError)
	}

	return &models.CreateAPIKeyResponse{
		Status:  "success",
		Message: fmt.Sprintf("API key created and broadcasted to %d gateway(s)", successCount),
		KeyID:   keyName,
		APIKey:  apiKey,
	}, nil
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

	gateways, err := s.gatewayRepo.GetByOrganizationID(orgID)
	if err != nil {
		return fmt.Errorf("failed to get gateways: %w", err)
	}
	if len(gateways) == 0 {
		return utils.ErrGatewayNotFound
	}

	event := &models.APIKeyRevokedEvent{
		APIID:   provider.Artifact.Name,
		KeyName: keyName,
	}

	successCount := 0
	var lastError error
	for _, gateway := range gateways {
		if err := s.gatewayService.BroadcastAPIKeyRevokedEvent(gateway.UUID.String(), event); err != nil {
			lastError = err
		} else {
			successCount++
		}
	}

	if successCount == 0 && lastError != nil {
		return fmt.Errorf("failed to deliver API key revocation to any gateway: %w", lastError)
	}
	return nil
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

	newAPIKey, err := utils.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	gateways, err := s.gatewayRepo.GetByOrganizationID(orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateways: %w", err)
	}
	if len(gateways) == 0 {
		return nil, utils.ErrGatewayNotFound
	}

	event := &models.APIKeyUpdatedEvent{
		APIID:   provider.Artifact.Name,
		KeyName: keyName,
		APIKey:  newAPIKey,
	}
	if req.DisplayName != nil {
		event.DisplayName = *req.DisplayName
	}
	if req.ExpiresAt != nil {
		event.ExpiresAt = req.ExpiresAt
	}

	successCount := 0
	var lastError error
	for _, gateway := range gateways {
		if err := s.gatewayService.BroadcastAPIKeyUpdatedEvent(gateway.UUID.String(), event); err != nil {
			lastError = err
		} else {
			successCount++
		}
	}

	if successCount == 0 && lastError != nil {
		return nil, fmt.Errorf("failed to deliver API key rotation to any gateway: %w", lastError)
	}

	return &models.CreateAPIKeyResponse{
		Status:  "success",
		Message: fmt.Sprintf("API key rotated and broadcasted to %d gateway(s)", successCount),
		KeyID:   keyName,
		APIKey:  newAPIKey,
	}, nil
}
