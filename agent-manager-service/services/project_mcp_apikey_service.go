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

// existingKey returns the binding's stored key named keyName, or ErrMCPAPIKeyNotFound. Agent and console keys are
// stored elsewhere, so only a user-managed key of this binding can match.
func (s *ProjectMCPProxyService) existingKey(t *bindingKeyTarget, keyName string) (*models.StoredAPIKey, error) {
	key, err := s.base.apiKeyBroadcaster.apiKeyRepo.GetByArtifactAndName(t.artifact, keyName)
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && key == nil):
		return nil, utils.ErrMCPAPIKeyNotFound
	case err != nil:
		return nil, fmt.Errorf("failed to read API key: %w", err)
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

// validateKeyExpiry refuses an expiresAt the broadcaster could not parse, as a 400 rather than its plain error.
func validateKeyExpiry(expiresAt *string) error {
	if expiresAt == nil {
		return nil
	}
	if _, err := time.Parse(time.RFC3339, *expiresAt); err != nil {
		return fmt.Errorf("%w: expiresAt must be an RFC 3339 time, e.g. 2026-12-31T23:59:59Z", utils.ErrBadRequest)
	}
	return nil
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
	if err := validateKeyExpiry(req.ExpiresAt); err != nil {
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
	resp, err := s.base.apiKeyBroadcaster.broadcastCreateToGateways(gateways, ouID, t.artifact, t.artifact, &issue)
	attempt.Complete(ctx, err)
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
	if err := validateKeyExpiry(req.ExpiresAt); err != nil {
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
	gateways, err := s.organizationGateways(ctx, ouID)
	if err != nil {
		return nil, err
	}

	attempt, err := beginAPIKeyAudit(ctx, audit.ActionAPIKeyRotate, t.auditTarget(ouID, projectName, envName, keyName))
	if err != nil {
		return nil, err
	}
	resp, err := s.base.apiKeyBroadcaster.broadcastRotateToGateways(gateways, ouID, t.artifact, t.artifact, existing.Name, req)
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
