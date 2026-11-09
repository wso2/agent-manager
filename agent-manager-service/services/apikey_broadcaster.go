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
	"slices"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"gorm.io/gorm"

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
	return b.createToGateways(gateways, orgID, apiID, artifactUUID, req,
		func(repo repositories.APIKeyRepository, key *models.StoredAPIKey) error { return repo.Upsert(key) })
}

// broadcastCreateNewToGateways is broadcastCreateToGateways for a name the caller read as free: the key is stored
// only as a new row, so if another request took the name first it returns utils.ErrAPIKeyNameTaken and sends
// nothing, instead of replacing that request's key.
func (b *apiKeyBroadcaster) broadcastCreateNewToGateways(gateways []*models.Gateway, orgID, apiID, artifactUUID string, req *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	return b.createToGateways(gateways, orgID, apiID, artifactUUID, req,
		func(repo repositories.APIKeyRepository, key *models.StoredAPIKey) error { return repo.Insert(key) })
}

// createToGateways generates an API key, stores it with persist, and broadcasts it to the given gateways.
func (b *apiKeyBroadcaster) createToGateways(
	gateways []*models.Gateway, orgID, apiID, artifactUUID string, req *models.CreateAPIKeyRequest,
	persist func(repositories.APIKeyRepository, *models.StoredAPIKey) error,
) (*models.CreateAPIKeyResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	keyName := req.Name
	if keyName == "" {
		var err error
		keyName, err = utils.GenerateHandle(req.DisplayName)
		if err != nil {
			return nil, fmt.Errorf("failed to generate API key name: %w", err)
		}
	}
	if utf8.RuneCountInString(keyName) < models.MinAPIKeyNameLength {
		return nil, fmt.Errorf("%w: API key name %q must be at least %d characters",
			utils.ErrBadRequest, keyName, models.MinAPIKeyNameLength)
	}

	apiKey, err := utils.GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
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

	// Persisted for bulk-sync.
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

	err = b.commitKeyChange(gateways,
		func(repo repositories.APIKeyRepository) error {
			if err := persist(repo, storedKey); err != nil {
				return fmt.Errorf("failed to persist API key: %w", err)
			}
			return nil
		},
		func(tx *gorm.DB, gatewayID string) error {
			var err error
			if tx == nil {
				err = b.gatewayService.BroadcastAPIKeyCreatedEvent(gatewayID, event)
			} else {
				err = b.gatewayService.PublishAPIKeyCreatedTx(context.Background(), tx, gatewayID, event)
			}
			if err != nil {
				return fmt.Errorf("failed to deliver API key to gateway %s: %w", gatewayID, err)
			}
			return nil
		})
	if err != nil {
		return nil, err
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
	event := &models.APIKeyRevokedEvent{
		APIID:   apiID,
		KeyName: keyName,
	}

	return b.commitKeyChange(gateways,
		func(repo repositories.APIKeyRepository) error {
			if err := repo.Delete(artifactUUID, keyName); err != nil {
				return fmt.Errorf("failed to delete API key from store: %w", err)
			}
			return nil
		},
		func(tx *gorm.DB, gatewayID string) error {
			var err error
			if tx == nil {
				err = b.gatewayService.BroadcastAPIKeyRevokedEvent(gatewayID, event)
			} else {
				err = b.gatewayService.PublishAPIKeyRevokedTx(ctx, tx, gatewayID, event)
			}
			if err != nil {
				return fmt.Errorf("gateway %s: %w", gatewayID, err)
			}
			return nil
		})
}

// publishArtifactKeysRevokedTx writes, in tx, a revocation of every API key stored for the artifact to each of the
// organization's gateways: the gateways every key of it was sent to. It is for a caller that deletes the artifact in
// tx, which takes the key rows with it (api_keys.artifact_uuid cascades), so no key outlives its artifact at a
// gateway. apiID is the artifact UUID, which is the API id the keys were sent under.
func (b *apiKeyBroadcaster) publishArtifactKeysRevokedTx(ctx context.Context, tx *gorm.DB, orgID string, artifactUUID uuid.UUID) error {
	if b.apiKeyRepo == nil || b.gatewayService == nil || b.gatewayRepo == nil {
		return nil
	}
	keys, err := b.apiKeyRepo.ListByArtifact(ctx, artifactUUID.String())
	if err != nil {
		return fmt.Errorf("failed to list the artifact's API keys: %w", err)
	}
	if len(keys) == 0 {
		return nil
	}
	gateways, err := b.gatewayRepo.GetByOrganizationID(ctx, orgID)
	if err != nil {
		return fmt.Errorf("failed to get gateways: %w", err)
	}
	for _, key := range keys {
		event := &models.APIKeyRevokedEvent{APIID: artifactUUID.String(), KeyName: key.Name}
		for _, gateway := range gateways {
			if err := b.gatewayService.PublishAPIKeyRevokedTx(ctx, tx, gateway.UUID.String(), event); err != nil {
				return fmt.Errorf("failed to revoke API key %s on gateway %s: %w", key.Name, gateway.UUID, err)
			}
		}
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
	return b.rotateToGateways(gateways, orgID, apiID, artifactUUID, keyName, req,
		func(repo repositories.APIKeyRepository, key *models.StoredAPIKey) error { return repo.Upsert(key) })
}

// broadcastRotateIfUnchangedToGateways is broadcastRotateToGateways for a key the caller read with hash prevHash:
// the new value is stored only while the key still has that hash, so if another request rotated or removed it first
// it returns utils.ErrAPIKeyChanged and sends nothing, instead of overwriting that request's value or bringing a
// revoked key back.
func (b *apiKeyBroadcaster) broadcastRotateIfUnchangedToGateways(gateways []*models.Gateway, orgID, apiID, artifactUUID, keyName, prevHash string, req *models.RotateAPIKeyRequest) (*models.CreateAPIKeyResponse, error) {
	return b.rotateToGateways(gateways, orgID, apiID, artifactUUID, keyName, req,
		func(repo repositories.APIKeyRepository, key *models.StoredAPIKey) error {
			return repo.ReplaceIfUnchanged(key, prevHash)
		})
}

// rotateToGateways generates a new value for the key, stores it with persist, and broadcasts it to the given gateways.
func (b *apiKeyBroadcaster) rotateToGateways(
	gateways []*models.Gateway, orgID, apiID, artifactUUID, keyName string, req *models.RotateAPIKeyRequest,
	persist func(repositories.APIKeyRepository, *models.StoredAPIKey) error,
) (*models.CreateAPIKeyResponse, error) {
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

	err = b.commitKeyChange(gateways,
		func(repo repositories.APIKeyRepository) error {
			if err := persist(repo, storedKey); err != nil {
				return fmt.Errorf("failed to persist rotated API key: %w", err)
			}
			return nil
		},
		func(tx *gorm.DB, gatewayID string) error {
			var err error
			if tx == nil {
				err = b.gatewayService.BroadcastAPIKeyUpdatedEvent(gatewayID, event)
			} else {
				err = b.gatewayService.PublishAPIKeyUpdatedTx(context.Background(), tx, gatewayID, event)
			}
			if err != nil {
				return fmt.Errorf("failed to deliver API key rotation to gateway %s: %w", gatewayID, err)
			}
			return nil
		})
	if err != nil {
		return nil, err
	}

	return &models.CreateAPIKeyResponse{
		Status:  "success",
		Message: fmt.Sprintf("API key rotated and broadcasted to %d gateway(s)", len(gateways)),
		KeyID:   keyName,
		APIKey:  newAPIKey,
	}, nil
}

// commitKeyChange applies a key write and publishes its event to each gateway in one transaction, so the gateways get
// key events in the order the writes were made. The write locks the key's row until commit, so a concurrent change to
// the same key waits for this one's events to be written before it writes its own, and the event hub numbers each
// gateway's events in commit order: an earlier rotation's event can no longer reach a gateway after a later one's.
// Gateways are published to in gateway ID order, as the event hub requires of a transaction that publishes to several.
// If the write or any publish fails, nothing is stored or sent.
//
// Without a key store (b.apiKeyRepo nil) there is nothing to write, and publish is called with a nil tx for each
// gateway.
func (b *apiKeyBroadcaster) commitKeyChange(
	gateways []*models.Gateway,
	write func(repositories.APIKeyRepository) error,
	publish func(tx *gorm.DB, gatewayID string) error,
) error {
	ids := make([]string, 0, len(gateways))
	for _, gateway := range gateways {
		ids = append(ids, gateway.UUID.String())
	}
	slices.Sort(ids)
	publishAll := func(tx *gorm.DB) error {
		for _, id := range ids {
			if err := publish(tx, id); err != nil {
				return err
			}
		}
		return nil
	}
	if b.apiKeyRepo == nil {
		return publishAll(nil)
	}
	return b.apiKeyRepo.Transaction(context.Background(), func(repo repositories.APIKeyRepository, tx *gorm.DB) error {
		if err := write(repo); err != nil {
			return err
		}
		return publishAll(tx)
	})
}

// hashAPIKeySHA256 computes a SHA-256 hash of the plain API key and returns the hex-encoded hash.
func hashAPIKeySHA256(plainKey string) string {
	h := sha256.Sum256([]byte(plainKey))
	return hex.EncodeToString(h[:])
}

// hashAPIKeyToJSON computes a SHA-256 hash of the plain API key and returns
// a JSON string in the format expected by the gateway: {"sha256": "<hex_hash>"}
func hashAPIKeyToJSON(plainKey string) string {
	return apiKeyHashesJSON(hashAPIKeySHA256(plainKey))
}

// apiKeyHashesJSON wraps an already hex-encoded SHA-256 in the gateway's ApiKeyHashes format.
func apiKeyHashesJSON(hexHash string) string {
	return fmt.Sprintf(`{"sha256":"%s"}`, hexHash)
}

// maskAPIKey returns a masked version of the API key showing only the last 4 characters.
func maskAPIKey(apiKey string) string {
	if len(apiKey) <= 4 {
		return "****"
	}
	return "****" + apiKey[len(apiKey)-4:]
}
