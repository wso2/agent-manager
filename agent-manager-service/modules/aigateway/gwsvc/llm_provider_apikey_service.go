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

package gwsvc

import (
	"context"
	"errors"
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
)

// LLMProviderAPIKeyService handles API key management for LLM providers
type LLMProviderAPIKeyService struct {
	providerRepo gwrepo.LLMProviderRepository
	apiKeyRepo   gwrepo.APIKeyRepository
	broadcaster  apiKeyBroadcaster
}

// NewLLMProviderAPIKeyService creates a new LLM provider API key service instance
func NewLLMProviderAPIKeyService(
	providerRepo gwrepo.LLMProviderRepository,
	gatewayRepo gwrepo.GatewayRepository,
	gatewayService *GatewayEventsService,
	apiKeyRepo gwrepo.APIKeyRepository,
) *LLMProviderAPIKeyService {
	return &LLMProviderAPIKeyService{
		providerRepo: providerRepo,
		apiKeyRepo:   apiKeyRepo,
		broadcaster: apiKeyBroadcaster{
			gatewayRepo:    gatewayRepo,
			gatewayService: gatewayService,
			apiKeyRepo:     apiKeyRepo,
		},
	}
}

// ListAPIKeys returns the masked, user-managed (permanent) API keys for an LLM provider.
// The plain key value is never returned — only the masked representation.
func (s *LLMProviderAPIKeyService) ListAPIKeys(
	ctx context.Context,
	orgID, providerID string,
) (*gwmodels.ListAPIKeysResponse, error) {
	provider, err := lookupLLMProvider(s.providerRepo, providerID, orgID)
	if err != nil {
		if errors.Is(err, gwerr.ErrLLMProviderNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	providerID = provider.UUID.String()

	stored, err := s.apiKeyRepo.ListByArtifact(ctx, providerID)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	return &gwmodels.ListAPIKeysResponse{Keys: gwmodels.ToUserManagedAPIKeyInfos(stored)}, nil
}

// CreateAPIKey generates an API key for an LLM provider and broadcasts it to all gateways
func (s *LLMProviderAPIKeyService) CreateAPIKey(
	ctx context.Context,
	orgID, providerID string,
	req *gwmodels.CreateAPIKeyRequest,
) (*gwmodels.CreateAPIKeyResponse, error) {
	provider, err := lookupLLMProvider(s.providerRepo, providerID, orgID)
	if err != nil {
		if errors.Is(err, gwerr.ErrLLMProviderNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	providerID = provider.UUID.String()

	annotateAPIKey(ctx, apiKeyAuditTarget{
		OwnerType: gwaudit.APIKeyOwnerLLMProvider,
		OwnerID:   providerID,
		OwnerName: llmProviderName(provider),
		KeyName:   keyNameOf(req),
	})

	resp, err := s.broadcaster.broadcastCreate(ctx, orgID, providerID, providerID, req)
	return resp, err
}

// RevokeAPIKey broadcasts an API key revocation event to all gateways for this organization.
func (s *LLMProviderAPIKeyService) RevokeAPIKey(
	ctx context.Context,
	orgID, providerID, keyName string,
) error {
	provider, err := lookupLLMProvider(s.providerRepo, providerID, orgID)
	if err != nil {
		if errors.Is(err, gwerr.ErrLLMProviderNotFound) {
			return err
		}
		return fmt.Errorf("failed to get LLM provider: %w", err)
	}
	providerID = provider.UUID.String()

	annotateAPIKey(ctx, apiKeyAuditTarget{
		OwnerType: gwaudit.APIKeyOwnerLLMProvider,
		OwnerID:   providerID,
		OwnerName: llmProviderName(provider),
		KeyName:   keyName,
	})

	err = s.broadcaster.broadcastRevoke(ctx, orgID, providerID, providerID, keyName)
	return err
}

// RevokeAllUserManagedKeys revokes every user-managed API key for an LLM provider and
// broadcasts the revocations to all gateways. Used when API key security is disabled.
func (s *LLMProviderAPIKeyService) RevokeAllUserManagedKeys(
	ctx context.Context,
	orgID, providerID string,
) error {
	provider, err := lookupLLMProvider(s.providerRepo, providerID, orgID)
	if err != nil {
		if errors.Is(err, gwerr.ErrLLMProviderNotFound) {
			return err
		}
		return fmt.Errorf("failed to get LLM provider: %w", err)
	}
	providerID = provider.UUID.String()

	// A bulk revocation, triggered when API-key security is turned off. Recorded
	// as one event naming the provider rather than one per key: the operation
	// the operator performed was "disable key security on this provider".
	annotateAPIKey(ctx, apiKeyAuditTarget{
		OwnerType: gwaudit.APIKeyOwnerLLMProvider,
		OwnerID:   providerID,
		OwnerName: llmProviderName(provider),
		KeyName:   "*",
	})

	err = s.broadcaster.broadcastRevokeUserManaged(ctx, orgID, providerID, providerID)
	return err
}

// RotateAPIKey generates a new API key value and broadcasts the update to all gateways.
// Returns the new API key (shown once) and its identifier.
func (s *LLMProviderAPIKeyService) RotateAPIKey(
	ctx context.Context,
	orgID, providerID, keyName string,
	req *gwmodels.RotateAPIKeyRequest,
) (*gwmodels.CreateAPIKeyResponse, error) {
	provider, err := lookupLLMProvider(s.providerRepo, providerID, orgID)
	if err != nil {
		if errors.Is(err, gwerr.ErrLLMProviderNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	providerID = provider.UUID.String()

	annotateAPIKey(ctx, apiKeyAuditTarget{
		OwnerType: gwaudit.APIKeyOwnerLLMProvider,
		OwnerID:   providerID,
		OwnerName: llmProviderName(provider),
		KeyName:   keyName,
	})

	resp, err := s.broadcaster.broadcastRotate(ctx, orgID, providerID, providerID, keyName, req)
	return resp, err
}

// llmProviderName returns the provider's display name for an audit record.
// The Artifact relation carries the name and is preloaded by the repository,
// but it is nil-guarded here so a missing join degrades the record rather than
// panicking on the credential path.
func llmProviderName(provider *gwmodels.LLMProvider) string {
	if provider == nil || provider.Artifact == nil {
		return ""
	}
	return provider.Artifact.Name
}
