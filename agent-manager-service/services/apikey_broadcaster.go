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

// broadcastCreate generates an API key, persists it, and broadcasts to all gateways for the org.
// apiID is the identifier sent to the gateway (UUID for providers, handle for proxies).
// artifactUUID is the DB UUID for persistence (always a valid UUID).
func (b *apiKeyBroadcaster) broadcastCreate(ctx context.Context, orgID, apiID, artifactUUID string, req *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	gateways, err := b.gatewayRepo.GetByOrganizationID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateways: %w", err)
	}
	if len(gateways) == 0 {
		return nil, utils.ErrGatewayNotFound
	}
	return b.broadcastCreateToGateways(gateways, orgID, apiID, artifactUUID, req)
}

// broadcastCreateToGateways generates an API key, persists it, and broadcasts to the given gateways only.
func (b *apiKeyBroadcaster) broadcastCreateToGateways(gateways []*models.Gateway, orgID, apiID, artifactUUID string, req *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
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

	purpose := req.Purpose
	if purpose == 0 {
		purpose = models.APIKeyPurposeUserManaged
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
			UUID:         keyUUID,
			Name:         keyName,
			DisplayName:  displayName,
			ArtifactUUID: parsedArtifactUUID,
			OUID:         orgID,
			APIKeyHash:   apiKeyHash,
			MaskedAPIKey: maskAPIKey(apiKey),
			Status:       "active",
			Purpose:      purpose,
			CreatedAt:    nowTime,
			UpdatedAt:    nowTime,
			ExpiresAt:    expiresAt,
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

func (b *apiKeyBroadcaster) broadcastRevoke(ctx context.Context, orgID, apiID, artifactUUID, keyName string) error {
	gateways, err := b.gatewayRepo.GetByOrganizationID(ctx, orgID)
	if err != nil {
		return fmt.Errorf("failed to get gateways: %w", err)
	}
	if len(gateways) == 0 {
		return utils.ErrGatewayNotFound
	}
	return b.broadcastRevokeToGateways(ctx, gateways, apiID, artifactUUID, keyName)
}

// broadcastRevokeToGateways removes a key from the store and broadcasts revocation to the given gateways only.
func (b *apiKeyBroadcaster) broadcastRevokeToGateways(ctx context.Context, gateways []*models.Gateway, apiID, artifactUUID, keyName string) error {
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

// broadcastRevokeUserManaged deletes and broadcasts revocation for every user-managed
// API key belonging to an artifact. Console-managed and test keys are left untouched —
// they are owned by internal provisioning flows, not the user-facing security toggle.
// apiID is the identifier sent to gateways (UUID for providers, handle/UUID for proxies);
// artifactUUID is the DB UUID used for lookup and deletion. Best-effort across keys: errors
// are collected and joined so one failure does not abort the rest. A missing gateway set is
// not an error — the stored keys are still deleted.
func (b *apiKeyBroadcaster) broadcastRevokeUserManaged(ctx context.Context, orgID, apiID, artifactUUID string) error {
	if b.apiKeyRepo == nil {
		return nil
	}
	stored, err := b.apiKeyRepo.ListByArtifact(ctx, artifactUUID)
	if err != nil {
		return fmt.Errorf("failed to list API keys: %w", err)
	}
	gateways, err := b.gatewayRepo.GetByOrganizationID(ctx, orgID)
	if err != nil {
		return fmt.Errorf("failed to get gateways: %w", err)
	}

	var errs []error
	for _, k := range stored {
		if k.Purpose != models.APIKeyPurposeUserManaged {
			continue
		}
		if err := b.broadcastRevokeToGateways(ctx, gateways, apiID, artifactUUID, k.Name); err != nil {
			errs = append(errs, fmt.Errorf("key %s: %w", k.Name, err))
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// isAPIKeyAuthEnabled reports whether API key authentication is enabled in a security
// config. Delegates to models.SecurityConfig.RequiresAPIKey so this package agrees with
// the deployment translators on what "enabled" means: an absent security.enabled used to
// count as enabled here while emitting no gateway policy, so a proxy could be described
// as needing a credential the gateway never asked for.
func isAPIKeyAuthEnabled(sec *models.SecurityConfig) bool {
	return sec.RequiresAPIKey()
}

func (b *apiKeyBroadcaster) broadcastRotate(ctx context.Context, orgID, apiID, artifactUUID, keyName string, req *models.RotateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	gateways, err := b.gatewayRepo.GetByOrganizationID(ctx, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateways: %w", err)
	}
	if len(gateways) == 0 {
		return nil, utils.ErrGatewayNotFound
	}
	return b.broadcastRotateToGateways(gateways, orgID, apiID, artifactUUID, keyName, req)
}

// broadcastRotateToGateways generates a new key value, updates the store, and broadcasts to the given gateways only.
func (b *apiKeyBroadcaster) broadcastRotateToGateways(gateways []*models.Gateway, orgID, apiID, artifactUUID, keyName string, req *models.RotateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	newAPIKey, err := utils.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
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
			UUID:         uuid.Must(uuid.NewV7()),
			Name:         keyName,
			ArtifactUUID: parsedArtifactUUID,
			OUID:         orgID,
			APIKeyHash:   hashAPIKeySHA256(newAPIKey),
			MaskedAPIKey: maskAPIKey(newAPIKey),
			Status:       "active",
			CreatedAt:    nowTime,
			UpdatedAt:    nowTime,
			ExpiresAt:    expiresAt,
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
