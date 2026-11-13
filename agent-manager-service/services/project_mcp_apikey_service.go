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
	"strings"
	"time"
	"unicode/utf8"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// A project MCP proxy binding's API keys are the keys its callers present to the gateway in that environment. Each
// is issued against the binding's gateway artifact, which is both the API the gateway checks the key against and the
// row the key is stored under:
//   - the key is sent to every gateway of the organization when it is issued, so whichever gateway serves the binding
//     already holds it, and a gateway that connects later receives it in its key sync (KindMCPMapping keys);
//   - the key goes with its binding: removing the binding (or the proxy) deletes its artifact, and the api_keys rows
//     cascade with it. A later binding in the same environment gets a new artifact and starts with no keys.
//
// Keys are user-managed. The agent keys AMS issues for AM agents using the proxy are stored under each agent's own
// mapping artifact, so they never show up here and are never touched by these operations.

// bindingKeyTarget is a binding resolved for its API keys: the proxy, the binding's endpoint and its gateway artifact.
type bindingKeyTarget struct {
	proxy    *models.MCPProxy
	endpoint *models.MCPProxyEndpoint
	artifact string
}

// resolveBindingKeyTarget finds the project proxy's binding in envName.
func (s *ProjectMCPProxyService) resolveBindingKeyTarget(
	ctx context.Context, ouID, projectUUID, handle, envName string,
) (*bindingKeyTarget, error) {
	if s.base.apiKeyBroadcaster.apiKeyRepo == nil || s.base.apiKeyBroadcaster.gatewayRepo == nil {
		return nil, fmt.Errorf("API key service is not configured")
	}
	proxy, err := s.getInProject(ctx, ouID, projectUUID, handle)
	if err != nil {
		return nil, err
	}
	endpoint, ee := bindingByEnvironmentName(proxy, strings.TrimSpace(envName))
	if ee == nil {
		return nil, utils.ErrMCPProxyEnvironmentNotBound
	}
	return &bindingKeyTarget{proxy: proxy, endpoint: endpoint, artifact: ee.ArtifactUUID.String()}, nil
}

func (t *bindingKeyTarget) auditTarget(ouID, projectName, envName, keyName string) apiKeyAuditTarget {
	return apiKeyAuditTarget{
		OUID:        ouID,
		OwnerType:   audit.APIKeyOwnerMCPProxy,
		OwnerID:     t.proxy.UUID.String(),
		OwnerName:   proxyHandleOf(t.proxy),
		KeyName:     keyName,
		Project:     projectName,
		Environment: envName,
	}
}

// requireAPIKeySecurity refuses a key for a binding the gateway does not check keys on: one with security off.
func (t *bindingKeyTarget) requireAPIKeySecurity() error {
	if !t.endpoint.Configuration.Security.RequiresAPIKey() {
		return utils.ErrMCPAPIKeySecurityDisabled
	}
	return nil
}

// existingKey returns the binding's user-managed key named keyName, or ErrMCPAPIKeyNotFound. Agent and console keys
// are stored elsewhere, and the binding's test keys do not match.
func (s *ProjectMCPProxyService) existingKey(t *bindingKeyTarget, keyName string) (*models.StoredAPIKey, error) {
	key, err := s.base.apiKeyBroadcaster.apiKeyRepo.GetByArtifactAndName(t.artifact, keyName)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && key == nil):
		return nil, utils.ErrMCPAPIKeyNotFound
	case err != nil:
		return nil, fmt.Errorf("failed to read API key: %w", err)
	case key.Purpose != models.APIKeyPurposeUserManaged:
		// A test key is not one of the binding's user keys: it is issued and replaced through the test key route.
		return nil, utils.ErrMCPAPIKeyNotFound
	}
	return key, nil
}

// organizationGateways is every gateway of the organization: the binding's keys go to all of them, as an agent's MCP
// keys do. None is not an error; the key is stored and reaches a gateway in its key sync when one connects.
func (s *ProjectMCPProxyService) organizationGateways(ctx context.Context, ouID string) ([]*models.Gateway, error) {
	gateways, err := s.base.apiKeyBroadcaster.gatewayRepo.GetByOrganizationID(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateways: %w", err)
	}
	return gateways, nil
}

// validateKeyExpiry refuses, as a 400, an expiresAt the broadcaster could not parse, and one that is not in the
// future: the gateway would drop such a key as soon as it arrived, so it could never be used.
func validateKeyExpiry(expiresAt *string, now time.Time) error {
	if expiresAt == nil {
		return nil
	}
	t, err := time.Parse(time.RFC3339, *expiresAt)
	if err != nil {
		return fmt.Errorf("%w: expiresAt must be an RFC 3339 time, e.g. 2026-12-31T23:59:59Z", utils.ErrBadRequest)
	}
	if !t.After(now) {
		return fmt.Errorf("%w: expiresAt must be in the future", utils.ErrBadRequest)
	}
	return nil
}

// keepKeyExpiry is req with the key's stored expiry filled in when req gives none. The broadcaster stores and sends
// exactly the request's expiresAt, so without this a rotation with no expiresAt would drop the key's expiry and the
// new value would never expire. An expired key is refused: its new value would be dead on arrival, so the caller
// has to give a new expiresAt.
func keepKeyExpiry(req *models.RotateAPIKeyRequest, existing *models.StoredAPIKey, now time.Time) (*models.RotateAPIKeyRequest, error) {
	if req.ExpiresAt != nil || existing.ExpiresAt == nil {
		return req, nil
	}
	if !existing.ExpiresAt.After(now) {
		return nil, fmt.Errorf("%w: the API key has expired; give a new expiresAt to rotate it", utils.ErrBadRequest)
	}
	kept := *req
	at := existing.ExpiresAt.UTC().Format(time.RFC3339)
	kept.ExpiresAt = &at
	return &kept, nil
}

// ListEnvironmentAPIKeys returns the binding's API keys, masked.
func (s *ProjectMCPProxyService) ListEnvironmentAPIKeys(
	ctx context.Context, ouID, projectUUID, handle, envName string,
) (*models.ListAPIKeysResponse, error) {
	t, err := s.resolveBindingKeyTarget(ctx, ouID, projectUUID, handle, envName)
	if err != nil {
		return nil, err
	}
	stored, err := s.base.apiKeyBroadcaster.apiKeyRepo.ListByArtifact(ctx, t.artifact)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	return &models.ListAPIKeysResponse{Keys: models.ToUserManagedAPIKeyInfos(stored)}, nil
}

// CreateEnvironmentAPIKey issues an API key for the binding and sends it to the organization's gateways. The key
// value is returned once. A binding not secured with an API key is refused (ErrMCPAPIKeySecurityDisabled), as is a
// name the binding already has a key for (ErrMCPAPIKeyExists): rotate that key instead.
func (s *ProjectMCPProxyService) CreateEnvironmentAPIKey(
	ctx context.Context, ouID, projectName, projectUUID, handle, envName string, req *models.CreateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request body is required", utils.ErrBadRequest)
	}
	if err := validateKeyExpiry(req.ExpiresAt, time.Now().UTC()); err != nil {
		return nil, err
	}
	t, err := s.resolveBindingKeyTarget(ctx, ouID, projectUUID, handle, envName)
	if err != nil {
		return nil, err
	}
	if err := t.requireAPIKeySecurity(); err != nil {
		return nil, err
	}
	// Name the key here, as the broadcaster would, so a duplicate is refused before anything is written: storing it
	// would replace the existing key's value without its holders knowing.
	keyName := strings.TrimSpace(req.Name)
	if keyName == "" {
		keyName, err = utils.GenerateHandle(req.DisplayName)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to generate an API key name: %w", utils.ErrBadRequest, err)
		}
	}
	if strings.HasPrefix(keyName, models.APIKeyTestKeyPrefix) {
		return nil, fmt.Errorf("%w: names starting with %q are reserved for test keys",
			utils.ErrBadRequest, models.APIKeyTestKeyPrefix)
	}
	if utf8.RuneCountInString(keyName) < models.MinAPIKeyNameLength {
		return nil, fmt.Errorf("%w: API key name %q must be at least %d characters",
			utils.ErrBadRequest, keyName, models.MinAPIKeyNameLength)
	}
	if _, err := s.existingKey(t, keyName); err == nil {
		return nil, fmt.Errorf("%w: %q", utils.ErrMCPAPIKeyExists, keyName)
	} else if !errors.Is(err, utils.ErrMCPAPIKeyNotFound) {
		return nil, err
	}
	gateways, err := s.organizationGateways(ctx, ouID)
	if err != nil {
		return nil, err
	}

	attempt, err := beginAPIKeyAudit(ctx, audit.ActionAPIKeyCreate, t.auditTarget(ouID, projectName, envName, keyName))
	if err != nil {
		return nil, err
	}
	issue := *req
	issue.Name = keyName
	issue.Purpose = models.APIKeyPurposeUserManaged
	// Stored only as a new row: a create for the same name that ran between the check above and here wins, and this
	// one is refused rather than replacing the key that request handed out.
	resp, err := s.base.apiKeyBroadcaster.broadcastCreateNewToGateways(gateways, ouID, t.artifact, t.artifact, &issue)
	attempt.Complete(ctx, err)
	if errors.Is(err, utils.ErrAPIKeyNameTaken) {
		return nil, fmt.Errorf("%w: %q", utils.ErrMCPAPIKeyExists, keyName)
	}
	return resp, err
}

// RotateEnvironmentAPIKey gives the binding's key keyName a new value, returned once, and sends it to the
// organization's gateways. The old value stops working.
func (s *ProjectMCPProxyService) RotateEnvironmentAPIKey(
	ctx context.Context, ouID, projectName, projectUUID, handle, envName, keyName string, req *models.RotateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	if req == nil {
		req = &models.RotateAPIKeyRequest{}
	}
	if err := validateKeyExpiry(req.ExpiresAt, time.Now().UTC()); err != nil {
		return nil, err
	}
	t, err := s.resolveBindingKeyTarget(ctx, ouID, projectUUID, handle, envName)
	if err != nil {
		return nil, err
	}
	if err := t.requireAPIKeySecurity(); err != nil {
		return nil, err
	}
	existing, err := s.existingKey(t, keyName)
	if err != nil {
		return nil, err
	}
	req, err = keepKeyExpiry(req, existing, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	gateways, err := s.organizationGateways(ctx, ouID)
	if err != nil {
		return nil, err
	}

	attempt, err := beginAPIKeyAudit(ctx, audit.ActionAPIKeyRotate, t.auditTarget(ouID, projectName, envName, keyName))
	if err != nil {
		return nil, err
	}
	// Stored only while the key still has the value read above: a rotate or revoke that ran in between wins, and this
	// one is refused (ErrAPIKeyChanged) rather than overwriting that value or bringing a revoked key back.
	resp, err := s.base.apiKeyBroadcaster.broadcastRotateIfUnchangedToGateways(gateways, ouID, t.artifact, t.artifact,
		existing.Name, existing.APIKeyHash, req)
	attempt.Complete(ctx, err)
	return resp, err
}

// RevokeEnvironmentAPIKey deletes the binding's key keyName and revokes it on the organization's gateways. It works
// whatever the binding's security, so a key issued before security was turned off can still be removed.
func (s *ProjectMCPProxyService) RevokeEnvironmentAPIKey(
	ctx context.Context, ouID, projectName, projectUUID, handle, envName, keyName string,
) error {
	t, err := s.resolveBindingKeyTarget(ctx, ouID, projectUUID, handle, envName)
	if err != nil {
		return err
	}
	existing, err := s.existingKey(t, keyName)
	if err != nil {
		return err
	}
	gateways, err := s.organizationGateways(ctx, ouID)
	if err != nil {
		return err
	}

	attempt, err := beginAPIKeyAudit(ctx, audit.ActionAPIKeyRevoke, t.auditTarget(ouID, projectName, envName, keyName))
	if err != nil {
		return err
	}
	err = s.base.apiKeyBroadcaster.broadcastRevokeToGateways(ctx, gateways, t.artifact, t.artifact, existing.Name)
	attempt.Complete(ctx, err)
	return err
}

const (
	// bindingTestKeyTTL is how long a binding test key lives when the caller gives no expiresAt.
	bindingTestKeyTTL = time.Hour
	// bindingTestKeyMaxTTL is the latest a caller may ask a binding test key to expire.
	bindingTestKeyMaxTTL = 24 * time.Hour
	// bindingTestKeySlots is how many test keys one user holds per binding at a time.
	bindingTestKeySlots = 3
	// bindingTestKeyAttempts is how many times a test key request picks a slot again after another request for the
	// same user took the one it picked.
	bindingTestKeyAttempts = 5
	// maxTestKeyUserLength bounds forUser, which only names the key.
	maxTestKeyUserLength = 256
)

// bindingTestKeyNames are the names of forUser's test key slots: the console test key prefix, a hash of the user and
// the slot number. The user is not stored, only the hash in the name.
func bindingTestKeyNames(forUser string) []string {
	sum := sha256.Sum256([]byte(forUser))
	base := models.APIKeyTestKeyPrefix + hex.EncodeToString(sum[:])[:12]
	names := make([]string, bindingTestKeySlots)
	for i := range names {
		names[i] = fmt.Sprintf("%s-%d", base, i+1)
	}
	return names
}

// testKeyExpiry is the expiry of a test key: expiresAt if given, which must be in the future and no later than
// bindingTestKeyMaxTTL from now, else bindingTestKeyTTL from now.
func testKeyExpiry(expiresAt *string, now time.Time) (string, error) {
	if expiresAt == nil || strings.TrimSpace(*expiresAt) == "" {
		return now.Add(bindingTestKeyTTL).Format(time.RFC3339), nil
	}
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(*expiresAt))
	if err != nil {
		return "", fmt.Errorf("%w: expiresAt must be an RFC 3339 time, e.g. 2026-12-31T23:59:59Z", utils.ErrBadRequest)
	}
	if !t.After(now) {
		return "", fmt.Errorf("%w: expiresAt must be in the future", utils.ErrBadRequest)
	}
	if t.After(now.Add(bindingTestKeyMaxTTL)) {
		return "", fmt.Errorf("%w: expiresAt must be no more than %s from now", utils.ErrBadRequest, bindingTestKeyMaxTTL)
	}
	return t.UTC().Format(time.RFC3339), nil
}

// claimTestKeySlot picks forUser's slot and issues the test key in it: an insert for a free slot, a rotate for the
// oldest one only while it still holds the value read. It returns utils.ErrAPIKeyNameTaken or utils.ErrAPIKeyChanged
// when a concurrent request took the slot first, and nothing was written or sent.
func (s *ProjectMCPProxyService) claimTestKeySlot(
	ctx context.Context, t *bindingKeyTarget, gateways []*models.Gateway, ouID, projectName, envName, forUser, expiresAt string,
) (*models.CreateAPIKeyResponse, error) {
	slot, oldest, err := s.pickTestKeySlot(t, forUser)
	if err != nil {
		return nil, err
	}
	rotated := oldest != nil
	attempt, err := beginAPIKeyAudit(ctx, audit.ActionAPIKeyIssueTest, t.auditTarget(ouID, projectName, envName, slot),
		audit.Detail("expiresAt", expiresAt), audit.Detail("rotated", rotated))
	if err != nil {
		return nil, err
	}
	var resp *models.CreateAPIKeyResponse
	if rotated {
		resp, err = s.base.apiKeyBroadcaster.broadcastRotateIfUnchangedToGateways(gateways, ouID, t.artifact,
			t.artifact, slot, oldest.APIKeyHash, &models.RotateAPIKeyRequest{ExpiresAt: &expiresAt})
	} else {
		resp, err = s.base.apiKeyBroadcaster.broadcastCreateNewToGateways(gateways, ouID, t.artifact, t.artifact,
			&models.CreateAPIKeyRequest{
				Name:        slot,
				DisplayName: "Test key",
				Purpose:     models.APIKeyPurposeTest,
				ExpiresAt:   &expiresAt,
			})
	}
	attempt.Complete(ctx, err)
	return resp, err
}

// pickTestKeySlot is the slot forUser's next test key goes in: the first free one, else the one issued longest ago,
// returned as oldest.
func (s *ProjectMCPProxyService) pickTestKeySlot(t *bindingKeyTarget, forUser string) (string, *models.StoredAPIKey, error) {
	var oldest *models.StoredAPIKey
	for _, name := range bindingTestKeyNames(forUser) {
		key, err := s.base.apiKeyBroadcaster.apiKeyRepo.GetByArtifactAndName(t.artifact, name)
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && key == nil):
			return name, nil, nil
		case err != nil:
			return "", nil, fmt.Errorf("failed to read API key: %w", err)
		case key.Purpose != models.APIKeyPurposeTest:
			return "", nil, fmt.Errorf("%w: %q is reserved for test keys", utils.ErrBadRequest, name)
		case oldest == nil || key.UpdatedAt.Before(oldest.UpdatedAt):
			oldest = key
		}
	}
	return oldest.Name, oldest, nil
}

// IssueEnvironmentTestAPIKey issues a short-lived test key for the binding on behalf of forUser, for a caller that
// acts for its own users. Each user has bindingTestKeySlots slots: a free slot takes the new key, else the slot
// issued longest ago is rotated, so its old value stops working. Test keys are not listed, and the user key routes
// cannot rotate or revoke them; they go with the binding like its other keys.
func (s *ProjectMCPProxyService) IssueEnvironmentTestAPIKey(
	ctx context.Context, ouID, projectName, projectUUID, handle, envName string, req *models.IssueBindingTestAPIKeyRequest,
) (*models.IssueTestAPIKeyResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("%w: request body is required", utils.ErrBadRequest)
	}
	forUser := strings.TrimSpace(req.ForUser)
	if forUser == "" {
		return nil, fmt.Errorf("%w: forUser is required", utils.ErrBadRequest)
	}
	if utf8.RuneCountInString(forUser) > maxTestKeyUserLength {
		return nil, fmt.Errorf("%w: forUser must be at most %d characters", utils.ErrBadRequest, maxTestKeyUserLength)
	}
	expiresAt, err := testKeyExpiry(req.ExpiresAt, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	t, err := s.resolveBindingKeyTarget(ctx, ouID, projectUUID, handle, envName)
	if err != nil {
		return nil, err
	}
	if err := t.requireAPIKeySecurity(); err != nil {
		return nil, err
	}

	gateways, err := s.organizationGateways(ctx, ouID)
	if err != nil {
		return nil, err
	}

	// Concurrent requests for the same user can pick the same slot. Each claims it with a write that fails if
	// another request got there first (an insert for a free slot, a rotate only while the slot holds the value read
	// for an occupied one), and the loser picks again, so no request hands out a key another one overwrote.
	var resp *models.CreateAPIKeyResponse
	for range bindingTestKeyAttempts {
		resp, err = s.claimTestKeySlot(ctx, t, gateways, ouID, projectName, envName, forUser, expiresAt)
		if errors.Is(err, utils.ErrAPIKeyNameTaken) || errors.Is(err, utils.ErrAPIKeyChanged) {
			continue
		}
		if err != nil {
			return nil, err
		}
		break
	}
	if resp == nil {
		return nil, fmt.Errorf("%w: other test key requests for this user kept taking the slots", utils.ErrAPIKeyChanged)
	}
	return &models.IssueTestAPIKeyResponse{
		Status:    resp.Status,
		Message:   resp.Message,
		KeyID:     resp.KeyID,
		APIKey:    resp.APIKey,
		ExpiresAt: expiresAt,
	}, nil
}
