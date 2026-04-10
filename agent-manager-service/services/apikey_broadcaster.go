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
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// apiKeyBroadcaster encapsulates the shared create/revoke/rotate broadcast pattern
// used by both LLMProviderAPIKeyService and LLMProxyAPIKeyService.
type apiKeyBroadcaster struct {
	gatewayRepo    repositories.GatewayRepository
	gatewayService *GatewayEventsService
	apiKeyRepo     repositories.APIKeyRepository
}

// broadcastCreate generates an API key, persists it, and broadcasts to all gateways.
// apiID is the identifier sent to the gateway (UUID for providers, handle for proxies).
// artifactUUID is the DB UUID for persistence (always a valid UUID).
func (b *apiKeyBroadcaster) broadcastCreate(orgID, apiID, artifactUUID string, req *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
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

	keyUUID := uuid.Must(uuid.NewV7())
	nowTime := time.Now().UTC()
	now := nowTime.Format(time.RFC3339)
	apiKeyHash := hashAPIKeySHA256(apiKey)

	// Parse artifact UUID for storage
	parsedArtifactUUID, err := uuid.Parse(artifactUUID)
	if err != nil {
		return nil, fmt.Errorf("invalid artifact UUID: %w", err)
	}

	// Parse optional expiry
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("invalid expiresAt format, expected RFC3339: %w", err)
		}
		expiresAt = &t
	}

	// Persist API key for bulk-sync
	if b.apiKeyRepo != nil {
		storedKey := &models.StoredAPIKey{
			UUID:             keyUUID,
			Name:             keyName,
			ArtifactUUID:     parsedArtifactUUID,
			OrganizationName: orgID,
			APIKeyHash:       apiKeyHash,
			MaskedAPIKey:     maskAPIKey(apiKey),
			Status:           "active",
			CreatedAt:        nowTime,
			UpdatedAt:        nowTime,
			ExpiresAt:        expiresAt,
		}
		if err := b.apiKeyRepo.Upsert(storedKey); err != nil {
			return nil, fmt.Errorf("failed to persist API key: %w", err)
		}
	}

	event := &models.APIKeyCreatedEvent{
		UUID:         keyUUID.String(),
		APIID:        apiID,
		Name:         keyName,
		DisplayName:  displayName,
		ApiKeyHashes: hashAPIKeyToJSON(apiKey),
		MaskedApiKey: maskAPIKey(apiKey),
		Operations:   "[\"*\"]",
		ExpiresAt:    req.ExpiresAt,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	for _, gateway := range gateways {
		if err := b.gatewayService.BroadcastAPIKeyCreatedEvent(gateway.UUID.String(), event); err != nil {
			return nil, fmt.Errorf("failed to deliver API key to gateway %s: %w", gateway.UUID, err)
		}
	}

	return &models.CreateAPIKeyResponse{
		Status:  "success",
		Message: fmt.Sprintf("API key created and broadcasted to %d gateway(s)", len(gateways)),
		KeyID:   keyName,
		APIKey:  apiKey,
	}, nil
}

func (b *apiKeyBroadcaster) broadcastRevoke(orgID, apiID, artifactUUID, keyName string) error {
	gateways, err := b.gatewayRepo.GetByOrganizationID(orgID)
	if err != nil {
		return fmt.Errorf("failed to get gateways: %w", err)
	}
	if len(gateways) == 0 {
		return utils.ErrGatewayNotFound
	}

	// Remove from persistent store
	if b.apiKeyRepo != nil {
		if err := b.apiKeyRepo.Delete(artifactUUID, keyName); err != nil {
			return fmt.Errorf("failed to delete API key from store: %w", err)
		}
	}

	event := &models.APIKeyRevokedEvent{
		APIID:   apiID,
		KeyName: keyName,
	}

	var errs []error
	for _, gateway := range gateways {
		if err := b.gatewayService.BroadcastAPIKeyRevokedEvent(gateway.UUID.String(), event); err != nil {
			errs = append(errs, fmt.Errorf("gateway %s: %w", gateway.UUID, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

func (b *apiKeyBroadcaster) broadcastRotate(orgID, apiID, artifactUUID, keyName string, req *models.RotateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
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

	nowTime := time.Now().UTC()

	// Parse and validate artifact UUID
	parsedArtifactUUID, parseErr := uuid.Parse(artifactUUID)
	if parseErr != nil {
		return nil, fmt.Errorf("invalid artifact UUID: %w", parseErr)
	}

	// Parse optional expiry
	var expiresAt *time.Time
	if req.ExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.ExpiresAt)
		if err != nil {
			return nil, fmt.Errorf("invalid expiresAt format, expected RFC3339: %w", err)
		}
		expiresAt = &t
	}

	// Update key in persistent store
	if b.apiKeyRepo != nil {
		storedKey := &models.StoredAPIKey{
			UUID:             uuid.Must(uuid.NewV7()),
			Name:             keyName,
			ArtifactUUID:     parsedArtifactUUID,
			OrganizationName: orgID,
			APIKeyHash:       hashAPIKeySHA256(newAPIKey),
			MaskedAPIKey:     maskAPIKey(newAPIKey),
			Status:           "active",
			CreatedAt:        nowTime,
			UpdatedAt:        nowTime,
			ExpiresAt:        expiresAt,
		}
		if err := b.apiKeyRepo.Upsert(storedKey); err != nil {
			return nil, fmt.Errorf("failed to persist rotated API key: %w", err)
		}
	}

	event := &models.APIKeyUpdatedEvent{
		APIID:        apiID,
		KeyName:      keyName,
		ApiKeyHashes: hashAPIKeyToJSON(newAPIKey),
		MaskedApiKey: maskAPIKey(newAPIKey),
		UpdatedAt:    nowTime.Format(time.RFC3339),
	}
	if req.DisplayName != nil {
		event.DisplayName = *req.DisplayName
	}
	if req.ExpiresAt != nil {
		event.ExpiresAt = req.ExpiresAt
	}

	for _, gateway := range gateways {
		if err := b.gatewayService.BroadcastAPIKeyUpdatedEvent(gateway.UUID.String(), event); err != nil {
			return nil, fmt.Errorf("failed to deliver API key rotation to gateway %s: %w", gateway.UUID, err)
		}
	}

	return &models.CreateAPIKeyResponse{
		Status:  "success",
		Message: fmt.Sprintf("API key rotated and broadcasted to %d gateway(s)", len(gateways)),
		KeyID:   keyName,
		APIKey:  newAPIKey,
	}, nil
}

// hashAPIKeySHA256 computes a SHA-256 hash of the plain API key and returns the hex-encoded hash.
func hashAPIKeySHA256(plainKey string) string {
	h := sha256.Sum256([]byte(plainKey))
	return hex.EncodeToString(h[:])
}

// hashAPIKeyToJSON computes a SHA-256 hash of the plain API key and returns
// a JSON string in the format expected by the gateway: {"sha256": "<hex_hash>"}
func hashAPIKeyToJSON(plainKey string) string {
	return fmt.Sprintf(`{"sha256":"%s"}`, hashAPIKeySHA256(plainKey))
}

// maskAPIKey returns a masked version of the API key showing only the last 4 characters.
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 4 {
		return "****"
	}
	return "****" + apiKey[len(apiKey)-4:]
}
