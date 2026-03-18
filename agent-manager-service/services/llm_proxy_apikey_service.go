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

// LLMProxyAPIKeyService handles API key management for LLM proxies
type LLMProxyAPIKeyService struct {
	proxyRepo   repositories.LLMProxyRepository
	broadcaster apiKeyBroadcaster
}

// NewLLMProxyAPIKeyService creates a new LLM proxy API key service instance
func NewLLMProxyAPIKeyService(
	proxyRepo repositories.LLMProxyRepository,
	gatewayRepo repositories.GatewayRepository,
	gatewayService *GatewayEventsService,
) *LLMProxyAPIKeyService {
	return &LLMProxyAPIKeyService{
		proxyRepo: proxyRepo,
		broadcaster: apiKeyBroadcaster{
			gatewayRepo:    gatewayRepo,
			gatewayService: gatewayService,
		},
	}
}

// CreateAPIKey generates an API key for an LLM proxy and broadcasts it to all gateways
func (s *LLMProxyAPIKeyService) CreateAPIKey(
	ctx context.Context,
	orgID, proxyID string,
	req *models.CreateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	proxy, err := s.proxyRepo.GetByID(proxyID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}
	return s.broadcaster.broadcastCreate(orgID, proxy.Handle, req)
}

// RevokeAPIKey broadcasts an API key revocation event to all gateways for this organization.
func (s *LLMProxyAPIKeyService) RevokeAPIKey(
	ctx context.Context,
	orgID, proxyID, keyName string,
) error {
	proxy, err := s.proxyRepo.GetByID(proxyID, orgID)
	if err != nil {
		return fmt.Errorf("failed to get LLM proxy: %w", err)
	}
	if proxy == nil {
		return utils.ErrLLMProxyNotFound
	}
	return s.broadcaster.broadcastRevoke(orgID, proxy.Handle, keyName)
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
		return nil, fmt.Errorf("failed to get LLM proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}
	return s.broadcaster.broadcastRotate(orgID, proxy.Handle, keyName, req)
}
