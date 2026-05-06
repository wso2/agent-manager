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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// AgentAPIKeyService handles API key management for agents
type AgentAPIKeyService struct {
	artifactRepo repositories.ArtifactRepository
	broadcaster  apiKeyBroadcaster
}

// NewAgentAPIKeyService creates a new agent API key service instance
func NewAgentAPIKeyService(
	artifactRepo repositories.ArtifactRepository,
	gatewayRepo repositories.GatewayRepository,
	gatewayService *GatewayEventsService,
	apiKeyRepo repositories.APIKeyRepository,
) *AgentAPIKeyService {
	return &AgentAPIKeyService{
		artifactRepo: artifactRepo,
		broadcaster: apiKeyBroadcaster{
			gatewayRepo:    gatewayRepo,
			gatewayService: gatewayService,
			apiKeyRepo:     apiKeyRepo,
		},
	}
}

// CreateAPIKey generates an API key for an agent and broadcasts it to all gateways
func (s *AgentAPIKeyService) CreateAPIKey(
	ctx context.Context,
	orgName, projectName, agentName string,
	req *models.CreateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	handle := projectName + "/" + agentName
	artifact, err := s.artifactRepo.GetByHandle(handle, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent artifact: %w", err)
	}
	if artifact.Kind != models.KindAgent {
		return nil, utils.ErrArtifactNotFound
	}
	artifactUUID := artifact.UUID.String()
	return s.broadcaster.broadcastCreate(orgName, artifactUUID, artifactUUID, req)
}

// RevokeAPIKey broadcasts an API key revocation event to all gateways for this organization.
func (s *AgentAPIKeyService) RevokeAPIKey(
	ctx context.Context,
	orgName, projectName, agentName, keyName string,
) error {
	handle := projectName + "/" + agentName
	artifact, err := s.artifactRepo.GetByHandle(handle, orgName)
	if err != nil {
		return fmt.Errorf("failed to get agent artifact: %w", err)
	}
	if artifact.Kind != models.KindAgent {
		return utils.ErrArtifactNotFound
	}
	artifactUUID := artifact.UUID.String()
	return s.broadcaster.broadcastRevoke(orgName, artifactUUID, artifactUUID, keyName)
}

// RotateAPIKey generates a new API key value and broadcasts the update to all gateways.
// Returns the new API key (shown once) and its identifier.
func (s *AgentAPIKeyService) RotateAPIKey(
	ctx context.Context,
	orgName, projectName, agentName, keyName string,
	req *models.RotateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	handle := projectName + "/" + agentName
	artifact, err := s.artifactRepo.GetByHandle(handle, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent artifact: %w", err)
	}
	if artifact.Kind != models.KindAgent {
		return nil, utils.ErrArtifactNotFound
	}
	artifactUUID := artifact.UUID.String()
	return s.broadcaster.broadcastRotate(orgName, artifactUUID, artifactUUID, keyName, req)
}
