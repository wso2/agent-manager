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
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// testKeyTTL is the validity window for a console-issued test API key.
// The console refreshes the key at staleTime well before this elapses.
const testKeyTTL = 10 * time.Minute

// AgentAPIKeyServiceInterface defines the contract for agent API key operations
type AgentAPIKeyServiceInterface interface {
	CreateAPIKey(ctx context.Context, orgName, projectName, agentName string, req *models.CreateAPIKeyRequest) (*models.CreateAPIKeyResponse, error)
	RevokeAPIKey(ctx context.Context, orgName, projectName, agentName, keyName string) error
	RotateAPIKey(ctx context.Context, orgName, projectName, agentName, keyName string, req *models.RotateAPIKeyRequest) (*models.CreateAPIKeyResponse, error)
	ListAPIKeys(ctx context.Context, orgName, projectName, agentName string) ([]models.StoredAPIKey, error)
	IssueTestAPIKey(ctx context.Context, orgName, projectName, agentName string) (*models.IssueTestAPIKeyResponse, error)
}

// AgentAPIKeyService handles API key management for agents
type AgentAPIKeyService struct {
	artifactRepo repositories.ArtifactRepository
	apiKeyRepo   repositories.APIKeyRepository
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
		apiKeyRepo:   apiKeyRepo,
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
	if req != nil && req.Name == models.APIKeyTestKeyName {
		return nil, fmt.Errorf("%w: %q is reserved for console test keys", utils.ErrBadRequest, models.APIKeyTestKeyName)
	}
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

// ListAPIKeys returns API keys for the given agent (masked values only).
func (s *AgentAPIKeyService) ListAPIKeys(
	ctx context.Context,
	orgName, projectName, agentName string,
) ([]models.StoredAPIKey, error) {
	handle := projectName + "/" + agentName
	artifact, err := s.artifactRepo.GetByHandle(handle, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent artifact: %w", err)
	}
	if artifact.Kind != models.KindAgent {
		return nil, utils.ErrArtifactNotFound
	}
	all, err := s.apiKeyRepo.ListPermanentByArtifactKind(orgName, models.KindAgent)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	var result []models.StoredAPIKey
	for _, k := range all {
		if k.ArtifactUUID == artifact.UUID {
			result = append(result, k)
		}
	}
	return result, nil
}

// IssueTestAPIKey issues (or rotates) the single short-lived test API key
// associated with an agent. Used by the console Try-It flow. The key is
// scoped by APIKeyTestKeyName and never appears in the user-facing list.
func (s *AgentAPIKeyService) IssueTestAPIKey(
	ctx context.Context,
	orgName, projectName, agentName string,
) (*models.IssueTestAPIKeyResponse, error) {
	handle := projectName + "/" + agentName
	artifact, err := s.artifactRepo.GetByHandle(handle, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get agent artifact: %w", err)
	}
	if artifact.Kind != models.KindAgent {
		return nil, utils.ErrArtifactNotFound
	}
	artifactUUID := artifact.UUID.String()

	expiresAt := time.Now().UTC().Add(testKeyTTL).Format(time.RFC3339)

	existing, err := s.apiKeyRepo.GetByArtifactAndName(artifactUUID, models.APIKeyTestKeyName)
	if err != nil {
		return nil, fmt.Errorf("failed to look up existing test key: %w", err)
	}

	var resp *models.CreateAPIKeyResponse
	if existing != nil {
		if existing.Purpose != models.APIKeyPurposeTest {
			return nil, fmt.Errorf("%w: %q is reserved for console test keys", utils.ErrBadRequest, models.APIKeyTestKeyName)
		}
		// Same DB row, new hash + expiry; purpose is preserved (Upsert.DoUpdates excludes it).
		resp, err = s.broadcaster.broadcastRotate(orgName, artifactUUID, artifactUUID, models.APIKeyTestKeyName,
			&models.RotateAPIKeyRequest{ExpiresAt: &expiresAt})
	} else {
		resp, err = s.broadcaster.broadcastCreate(orgName, artifactUUID, artifactUUID,
			&models.CreateAPIKeyRequest{
				Name:        models.APIKeyTestKeyName,
				DisplayName: "Console Try-It",
				Purpose:     models.APIKeyPurposeTest,
				ExpiresAt:   &expiresAt,
			})
	}
	if err != nil {
		return nil, err
	}

	return &models.IssueTestAPIKeyResponse{
		Status:    resp.Status,
		Message:   resp.Message,
		KeyID:     resp.KeyID,
		APIKey:    resp.APIKey,
		ExpiresAt: expiresAt,
	}, nil
}
