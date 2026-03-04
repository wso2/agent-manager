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
	"fmt"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// apiKeyBroadcaster encapsulates the shared create/revoke/rotate broadcast pattern
// used by both LLMProviderAPIKeyService and LLMProxyAPIKeyService.
type apiKeyBroadcaster struct {
	gatewayRepo    repositories.GatewayRepository
	gatewayService *GatewayEventsService
}

func (b *apiKeyBroadcaster) broadcastCreate(orgID, apiID string, req *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	apiKey, err := utils.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

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

	gateways, err := b.gatewayRepo.GetByOrganizationID(orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateways: %w", err)
	}
	if len(gateways) == 0 {
		return nil, utils.ErrGatewayNotFound
	}

	event := &models.APIKeyCreatedEvent{
		APIID:       apiID,
		Name:        keyName,
		DisplayName: displayName,
		APIKey:      apiKey,
		Operations:  "[\"*\"]",
		ExpiresAt:   req.ExpiresAt,
	}

	successCount := 0
	var lastError error
	for _, gateway := range gateways {
		if err := b.gatewayService.BroadcastAPIKeyCreatedEvent(gateway.UUID.String(), event); err != nil {
			lastError = err
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

func (b *apiKeyBroadcaster) broadcastRevoke(orgID, apiID, keyName string) error {
	gateways, err := b.gatewayRepo.GetByOrganizationID(orgID)
	if err != nil {
		return fmt.Errorf("failed to get gateways: %w", err)
	}
	if len(gateways) == 0 {
		return utils.ErrGatewayNotFound
	}

	event := &models.APIKeyRevokedEvent{
		APIID:   apiID,
		KeyName: keyName,
	}

	var lastError error
	for _, gateway := range gateways {
		if err := b.gatewayService.BroadcastAPIKeyRevokedEvent(gateway.UUID.String(), event); err != nil {
			lastError = err
		}
	}

	if lastError != nil {
		return fmt.Errorf("failed to deliver API key revocation to all gateways: %w", lastError)
	}
	return nil
}

func (b *apiKeyBroadcaster) broadcastRotate(orgID, apiID, keyName string, req *models.RotateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	newAPIKey, err := utils.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	gateways, err := b.gatewayRepo.GetByOrganizationID(orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateways: %w", err)
	}
	if len(gateways) == 0 {
		return nil, utils.ErrGatewayNotFound
	}

	event := &models.APIKeyUpdatedEvent{
		APIID:   apiID,
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
		if err := b.gatewayService.BroadcastAPIKeyUpdatedEvent(gateway.UUID.String(), event); err != nil {
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
