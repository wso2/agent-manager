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
	orgID, providerID, userID string,
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
		APIID:       providerID,
		Name:        keyName,
		DisplayName: displayName,
		APIKey:      apiKey,
		Operations:  []string{"*"}, // All operations
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
