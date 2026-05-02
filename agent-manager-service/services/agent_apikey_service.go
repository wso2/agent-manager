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
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	gatewaymgmtsvc "github.com/wso2/agent-manager/agent-manager-service/clients/gatewaymgmtsvc"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	gatewayMgmtURLProperty      = "managementAPIURL"
	gatewayMgmtUsernameProperty = "managementUsername"
	gatewayMgmtPasswordProperty = "managementPassword"
)

// GatewayManagementClientFactory creates a gateway management client for one gateway.
type GatewayManagementClientFactory func(managementURL, username, password string) (gatewaymgmtsvc.Client, error)

// AgentAPIKeyService manages gateway-native API keys for agent RestApis.
type AgentAPIKeyService struct {
	ocClient                 occlient.OpenChoreoClient
	gatewayRepo              repositories.GatewayRepository
	apiKeyRepo               repositories.APIKeyRepository
	logger                   *slog.Logger
	gatewayManagementFactory GatewayManagementClientFactory
}

// NewAgentAPIKeyService creates a new agent API key service.
func NewAgentAPIKeyService(
	ocClient occlient.OpenChoreoClient,
	gatewayRepo repositories.GatewayRepository,
	apiKeyRepo repositories.APIKeyRepository,
	logger *slog.Logger,
	gatewayManagementFactory GatewayManagementClientFactory,
) *AgentAPIKeyService {
	return &AgentAPIKeyService{
		ocClient:                 ocClient,
		gatewayRepo:              gatewayRepo,
		apiKeyRepo:               apiKeyRepo,
		logger:                   logger,
		gatewayManagementFactory: gatewayManagementFactory,
	}
}

// CreateAPIKey creates a gateway-native API key and persists a masked local record for UI listing.
func (s *AgentAPIKeyService) CreateAPIKey(
	ctx context.Context,
	orgName, projectName, agentName string,
	req *models.CreateAPIKeyRequest,
) (*models.CreateAPIKeyResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}

	keyName, err := apiKeyName(req)
	if err != nil {
		return nil, err
	}

	artifactID, gateway, err := s.resolveAgentGateway(ctx, orgName, projectName, agentName)
	if err != nil {
		return nil, err
	}

	client, restAPIID, err := s.resolveGatewayRestAPI(ctx, gateway, artifactID)
	if err != nil {
		return nil, err
	}

	plainKey, gatewayExpiresAt, err := client.CreateAPIKey(ctx, restAPIID, keyName, req.ExpiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to create gateway API key: %w", err)
	}

	if err := s.storeAPIKey(orgName, artifactID, keyName, plainKey, req.ExpiresAt, gatewayExpiresAt); err != nil {
		return nil, err
	}

	return &models.CreateAPIKeyResponse{
		Status:  "success",
		Message: "API key created",
		KeyID:   keyName,
		APIKey:  plainKey,
	}, nil
}

// ListAPIKeys lists locally stored masked API keys for the agent.
func (s *AgentAPIKeyService) ListAPIKeys(ctx context.Context, orgName, projectName, agentName string) ([]models.StoredAPIKey, error) {
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrAgentNotFound) {
			return nil, utils.ErrAgentNotFound
		}
		return nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil || strings.TrimSpace(agent.UUID) == "" {
		return nil, utils.ErrAgentNotFound
	}

	keys, err := s.apiKeyRepo.ListByArtifact(orgName, agent.UUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list API keys: %w", err)
	}
	return keys, nil
}

// RevokeAPIKey revokes a gateway-native API key and removes the local listing record.
func (s *AgentAPIKeyService) RevokeAPIKey(ctx context.Context, orgName, projectName, agentName, keyName string) error {
	keyName = strings.TrimSpace(keyName)
	if keyName == "" {
		return fmt.Errorf("key name is required")
	}

	artifactID, gateway, err := s.resolveAgentGateway(ctx, orgName, projectName, agentName)
	if err != nil {
		return err
	}

	client, restAPIID, err := s.resolveGatewayRestAPI(ctx, gateway, artifactID)
	if err != nil {
		return err
	}

	if err := client.RevokeAPIKey(ctx, restAPIID, keyName); err != nil {
		return fmt.Errorf("failed to revoke gateway API key: %w", err)
	}

	if err := s.apiKeyRepo.Delete(artifactID, keyName); err != nil {
		return fmt.Errorf("failed to delete local API key record: %w", err)
	}
	return nil
}

func (s *AgentAPIKeyService) resolveAgentGateway(ctx context.Context, orgName, projectName, agentName string) (string, *models.Gateway, error) {
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrAgentNotFound) {
			return "", nil, utils.ErrAgentNotFound
		}
		return "", nil, fmt.Errorf("failed to get agent: %w", err)
	}
	if agent == nil || strings.TrimSpace(agent.UUID) == "" {
		return "", nil, utils.ErrAgentNotFound
	}

	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrProjectNotFound) {
			return "", nil, utils.ErrProjectNotFound
		}
		return "", nil, fmt.Errorf("failed to get project deployment pipeline: %w", err)
	}

	environmentName := findLowestEnvironment(pipeline.PromotionPaths)
	if environmentName == "" {
		return "", nil, fmt.Errorf("project deployment pipeline does not contain a source environment")
	}

	environment, err := s.ocClient.GetEnvironment(ctx, orgName, environmentName)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) || errors.Is(err, utils.ErrEnvironmentNotFound) {
			return "", nil, utils.ErrEnvironmentNotFound
		}
		return "", nil, fmt.Errorf("failed to get environment: %w", err)
	}
	if environment == nil || strings.TrimSpace(environment.UUID) == "" {
		return "", nil, utils.ErrEnvironmentNotFound
	}

	mappings, err := s.gatewayRepo.GetEnvironmentMappingsByEnvironmentID(environment.UUID)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get environment gateway mappings: %w", err)
	}
	if len(mappings) == 0 {
		return "", nil, utils.ErrGatewayNotFound
	}
	if len(mappings) > 1 {
		return "", nil, fmt.Errorf("agent API key creation requires exactly one gateway mapped to environment %q; found %d", environmentName, len(mappings))
	}

	gateway, err := s.gatewayRepo.GetByUUID(mappings[0].GatewayUUID.String())
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, utils.ErrGatewayNotFound) {
			return "", nil, utils.ErrGatewayNotFound
		}
		return "", nil, fmt.Errorf("failed to get gateway: %w", err)
	}
	if gateway == nil {
		return "", nil, utils.ErrGatewayNotFound
	}

	return agent.UUID, gateway, nil
}

func (s *AgentAPIKeyService) resolveGatewayRestAPI(ctx context.Context, gateway *models.Gateway, artifactID string) (gatewaymgmtsvc.Client, string, error) {
	managementURL := gatewayProperty(gateway, gatewayMgmtURLProperty)
	username := gatewayProperty(gateway, gatewayMgmtUsernameProperty)
	password := gatewayProperty(gateway, gatewayMgmtPasswordProperty)
	if managementURL == "" || username == "" {
		return nil, "", fmt.Errorf("gateway %s is missing management API properties", gateway.UUID)
	}

	client, err := s.gatewayManagementFactory(managementURL, username, password)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create gateway management client: %w", err)
	}

	restAPIID, err := client.FindRestAPIByArtifactID(ctx, artifactID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to find gateway RestAPI by artifact ID: %w", err)
	}
	return client, restAPIID, nil
}

func (s *AgentAPIKeyService) storeAPIKey(orgName, artifactID, keyName, plainKey string, expiresAtValue *string, gatewayExpiresAt *time.Time) error {
	artifactUUID, err := uuid.Parse(artifactID)
	if err != nil {
		return fmt.Errorf("invalid agent artifact UUID: %w", err)
	}

	expiresAt := gatewayExpiresAt
	if expiresAt == nil && expiresAtValue != nil {
		t, err := time.Parse(time.RFC3339, *expiresAtValue)
		if err != nil {
			return fmt.Errorf("invalid expiresAt format, expected RFC3339: %w", err)
		}
		expiresAt = &t
	}

	now := time.Now().UTC()
	if err := s.apiKeyRepo.EnsureArtifactReference(&models.Artifact{
		UUID:             artifactUUID,
		Handle:           artifactID,
		Name:             artifactID,
		Version:          "v1",
		Kind:             models.KindAgent,
		OrganizationName: orgName,
		CreatedAt:        now,
		UpdatedAt:        now,
	}); err != nil {
		return fmt.Errorf("failed to ensure agent API key artifact reference: %w", err)
	}

	storedKey := &models.StoredAPIKey{
		UUID:             uuid.Must(uuid.NewV7()),
		Name:             keyName,
		ArtifactUUID:     artifactUUID,
		OrganizationName: orgName,
		APIKeyHash:       hashAPIKeySHA256(plainKey),
		MaskedAPIKey:     maskAPIKey(plainKey),
		Status:           "active",
		CreatedAt:        now,
		UpdatedAt:        now,
		ExpiresAt:        expiresAt,
	}
	if err := s.apiKeyRepo.Upsert(storedKey); err != nil {
		return fmt.Errorf("failed to persist API key: %w", err)
	}
	return nil
}

func apiKeyName(req *models.CreateAPIKeyRequest) (string, error) {
	if strings.TrimSpace(req.Name) != "" {
		return strings.TrimSpace(req.Name), nil
	}
	if strings.TrimSpace(req.DisplayName) == "" {
		return "", fmt.Errorf("at least one of name or displayName must be provided")
	}
	name, err := utils.GenerateHandle(req.DisplayName)
	if err != nil {
		return "", fmt.Errorf("failed to generate API key name: %w", err)
	}
	return name, nil
}

func gatewayProperty(gateway *models.Gateway, key string) string {
	if gateway == nil || gateway.Properties == nil {
		return ""
	}
	value, ok := gateway.Properties[key]
	if !ok || value == nil {
		return ""
	}
	if stringValue, ok := value.(string); ok {
		return strings.TrimSpace(stringValue)
	}
	return strings.TrimSpace(fmt.Sprint(value))
}
