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

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

// LLMProxyAPIKeyService handles API key management for LLM proxies
type LLMProxyAPIKeyService struct {
	proxyRepo   repositories.LLMProxyRepository
	apiKeyRepo  repositories.APIKeyRepository
	ocClient    repositories.PlatformRepository
	broadcaster apiKeyBroadcaster
}

// NewLLMProxyAPIKeyService creates a new LLM proxy API key service instance
func NewLLMProxyAPIKeyService(
	proxyRepo repositories.LLMProxyRepository,
	gatewayRepo repositories.GatewayRepository,
	gatewayService *GatewayEventsService,
	apiKeyRepo repositories.APIKeyRepository,
	ocClient repositories.PlatformRepository,
) *LLMProxyAPIKeyService {
	return &LLMProxyAPIKeyService{
		proxyRepo:  proxyRepo,
		apiKeyRepo: apiKeyRepo,
		ocClient:   ocClient,
		broadcaster: apiKeyBroadcaster{
			gatewayRepo:    gatewayRepo,
			gatewayService: gatewayService,
			apiKeyRepo:     apiKeyRepo,
		},
	}
}

// resolveProjectScopedProxy looks up an LLM proxy and validates that it belongs
// to the caller's project. The project name is resolved to its UUID via
// the platform repository, then the proxy lookup is scoped to org + project + proxy so a
// proxy owned by a different project is reported as not found rather than being
// treated as a wildcard match.
func (s *LLMProxyAPIKeyService) resolveProjectScopedProxy(
	ctx context.Context,
	orgID, projName, proxyID string,
) (*models.LLMProxy, error) {
	project, err := s.ocClient.GetProject(ctx, orgID, projName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			return nil, utils.ErrProjectNotFound
		}
		return nil, fmt.Errorf("failed to resolve project: %w", err)
	}
	if project == nil {
		return nil, utils.ErrProjectNotFound
	}

	proxy, err := s.proxyRepo.GetByIDAndProject(proxyID, orgID, project.UUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get LLM proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}
	return proxy, nil
}

// ListAPIKeys returns the masked, user-managed (permanent) API keys for an LLM proxy.
// The plain key value is never returned — only the masked representation.
func (s *LLMProxyAPIKeyService) ListAPIKeys(
	ctx context.Context,
	orgID, projName, proxyID string,
) (*models.ListAPIKeysResponse, error) {
	proxy, err := s.resolveProjectScopedProxy(ctx, orgID, projName, proxyID)
	if err != nil {
		return nil, err
	}

	stored, err := s.apiKeyRepo.ListByArtifact(ctx, proxy.UUID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}

	return &models.ListAPIKeysResponse{Keys: models.ToUserManagedAPIKeyInfos(stored)}, nil
}

// CreateAPIKey generates an API key for an LLM proxy and broadcasts it to all gateways
func (s *LLMProxyAPIKeyService) CreateAPIKey(
	ctx context.Context,
	orgID, proxyID string,
	req *models.CreateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	proxy, err := s.proxyRepo.GetByID(proxyID, orgID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get LLM proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}

	attempt, err := beginAPIKeyAudit(ctx, audit.ActionAPIKeyCreate, apiKeyAuditTarget{
		OUID:      orgID,
		OwnerType: audit.APIKeyOwnerLLMProxy,
		OwnerID:   proxy.UUID.String(),
		OwnerName: proxy.Name,
		KeyName:   keyNameOf(req),
	})
	if err != nil {
		return nil, err
	}

	resp, err := s.broadcaster.broadcastCreate(ctx, orgID, proxyID, proxy.UUID.String(), req)
	attempt.Complete(ctx, err)
	return resp, err
}

// RevokeAPIKey broadcasts an API key revocation event to all gateways for this organization.
func (s *LLMProxyAPIKeyService) RevokeAPIKey(
	ctx context.Context,
	orgID, proxyID, keyName string,
) error {
	proxy, err := s.proxyRepo.GetByID(proxyID, orgID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrLLMProxyNotFound
		}
		return fmt.Errorf("failed to get LLM proxy: %w", err)
	}
	if proxy == nil {
		return utils.ErrLLMProxyNotFound
	}

	attempt, err := beginAPIKeyAudit(ctx, audit.ActionAPIKeyRevoke, apiKeyAuditTarget{
		OUID:      orgID,
		OwnerType: audit.APIKeyOwnerLLMProxy,
		OwnerID:   proxy.UUID.String(),
		OwnerName: proxy.Name,
		KeyName:   keyName,
	})
	if err != nil {
		return err
	}

	err = s.broadcaster.broadcastRevoke(ctx, orgID, proxyID, proxy.UUID.String(), keyName)
	attempt.Complete(ctx, err)
	return err
}

// RotateAPIKey generates a new API key value and broadcasts the update to all gateways.
// Returns the new API key (shown once) and its identifier.
func (s *LLMProxyAPIKeyService) RotateAPIKey(
	ctx context.Context,
	orgID, proxyID, keyName string,
	req *models.RotateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	proxy, err := s.proxyRepo.GetByID(proxyID, orgID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get LLM proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}

	attempt, err := beginAPIKeyAudit(ctx, audit.ActionAPIKeyRotate, apiKeyAuditTarget{
		OUID:      orgID,
		OwnerType: audit.APIKeyOwnerLLMProxy,
		OwnerID:   proxy.UUID.String(),
		OwnerName: proxy.Name,
		KeyName:   keyName,
	})
	if err != nil {
		return nil, err
	}

	resp, err := s.broadcaster.broadcastRotate(ctx, orgID, proxyID, proxy.UUID.String(), keyName, req)
	attempt.Complete(ctx, err)
	return resp, err
}
