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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// AgentConfigurationService interface defines agent configuration business logic
type AgentConfigurationService interface {
	Create(ctx context.Context, orgName, projectName, agentID string,
		req models.CreateAgentModelConfigRequest, createdBy string) (*models.AgentModelConfigResponse, error)
	Get(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string) (*models.AgentModelConfigResponse, error)
	GetByAgent(ctx context.Context, agentID, orgName string) (*models.AgentModelConfigResponse, error)
	List(ctx context.Context, orgName, projectName, agentName string, limit, offset int) (*models.AgentModelConfigListResponse, error)
	Update(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string,
		req models.UpdateAgentModelConfigRequest) (*models.AgentModelConfigResponse, error)
	Delete(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string) error
}

type EnvConfigTemplate struct {
	Key             string `json:"key"`
	Name            string `json:"name"`
	IsSecret        bool   `json:"isSecret"`
	Value           string `json:"value"`
	SecretReference string `json:"secretReference"`
}

type agentConfigurationService struct {
	db                        *gorm.DB
	agentConfigRepo           repositories.AgentConfigurationRepository
	envMappingRepo            repositories.EnvAgentModelMappingRepository
	envVariableRepo           repositories.AgentEnvConfigVariableRepository
	llmProviderRepo           repositories.LLMProviderRepository
	gatewayRepo               repositories.GatewayRepository
	llmProxyService           *LLMProxyService
	llmProxyDeploymentService *LLMProxyDeploymentService
	llmProxyAPIKeyService     *LLMProxyAPIKeyService
	llmProviderAPIKeyService  *LLMProviderAPIKeyService
	infraResourceManager      InfraResourceManager
	ocClient                  client.OpenChoreoClient
	logger                    *slog.Logger
}

// rollbackResource tracks a proxy, its deployment, and API keys for cleanup
type rollbackResource struct {
	proxyHandle      string
	deploymentID     uuid.UUID
	proxyAPIKeyID    string    // API key created for the proxy
	providerAPIKeyID string    // API key name created for the provider
	providerUUID     string    // UUID of the provider (needed to revoke the provider API key)
	mappingID        uint      // ID of the env mapping to revert (HIGH-4, Scenario A only)
	oldProxyUUID     uuid.UUID // old proxy UUID to restore in the mapping on rollback (HIGH-4, Scenario A only)
}

// envCredentialData tracks proxy credentials for external agents
type envCredentialData struct {
	apiKey   string
	proxyURL string
}

// NewAgentConfigurationService creates a new agent configuration service
func NewAgentConfigurationService(
	db *gorm.DB,
	agentConfigRepo repositories.AgentConfigurationRepository,
	envMappingRepo repositories.EnvAgentModelMappingRepository,
	envVariableRepo repositories.AgentEnvConfigVariableRepository,
	llmProviderRepo repositories.LLMProviderRepository,
	gatewayRepo repositories.GatewayRepository,
	llmProxyService *LLMProxyService,
	llmProxyDeploymentService *LLMProxyDeploymentService,
	llmProxyAPIKeyService *LLMProxyAPIKeyService,
	infraResourceManager InfraResourceManager,
	ocClient client.OpenChoreoClient,
	llmProviderAPIKeyService *LLMProviderAPIKeyService,
	logger *slog.Logger,
) AgentConfigurationService {
	return &agentConfigurationService{
		db:                        db,
		agentConfigRepo:           agentConfigRepo,
		envMappingRepo:            envMappingRepo,
		envVariableRepo:           envVariableRepo,
		llmProviderRepo:           llmProviderRepo,
		gatewayRepo:               gatewayRepo,
		llmProxyService:           llmProxyService,
		llmProxyDeploymentService: llmProxyDeploymentService,
		llmProxyAPIKeyService:     llmProxyAPIKeyService,
		infraResourceManager:      infraResourceManager,
		ocClient:                  ocClient,
		llmProviderAPIKeyService:  llmProviderAPIKeyService,
		logger:                    logger,
	}
}

// compensatingDeleteConfig performs a best-effort DELETE of the config row committed in Phase 1,
// when a later phase fails. CASCADE on EnvMappings/EnvVariables removes any partially-written rows.
func (s *agentConfigurationService) compensatingDeleteConfig(ctx context.Context, configUUID uuid.UUID, orgName string) {
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.agentConfigRepo.Delete(ctx, tx, configUUID, orgName)
	}); err != nil {
		s.logger.Error("CRITICAL: Failed to compensate config creation - orphaned config record",
			"configUUID", configUUID, "orgName", orgName, "error", err, "action", "manual cleanup required")
	} else {
		s.logger.Info("Compensating delete of config record succeeded", "configUUID", configUUID)
	}
}

// Create creates a new agent model configuration
func (s *agentConfigurationService) Create(ctx context.Context, orgName, projectName, agentID string,
	req models.CreateAgentModelConfigRequest, createdBy string,
) (*models.AgentModelConfigResponse, error) {
	// Validate agent exists and determine type
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentID)
	if err != nil {
		// Check if it's a 404 error (agent not found) vs other errors
		if errors.Is(err, utils.ErrAgentNotFound) {
			return nil, utils.ErrAgentNotFound
		}
		// For other errors (unauthorized, internal, etc), return as-is
		return nil, fmt.Errorf("failed to validate agent: %w", err)
	}

	// Determine if this is an external agent
	isExternalAgent := agent.Provisioning.Type == string(utils.ExternalAgent)

	// Validate that at least one environment mapping is provided (CRIT-5).
	// The binding:"required,min=1" tag on the DTO is ignored by net/http + json.NewDecoder,
	// so we enforce it explicitly here.
	if len(req.EnvMappings) == 0 {
		return nil, fmt.Errorf("%w: at least one environment mapping is required", utils.ErrInvalidInput)
	}

	// Fail fast: validate env var names before any I/O.
	// If the config name would generate a reserved env var prefix the error is returned here,
	// before any gateway/proxy/deployment resources have been created.
	// The returned slice is intentionally discarded; it is rebuilt at deployment time.
	if _, err := s.buildEnvironmentVariables(req.Name); err != nil {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
	}

	// Validate all providers exist and are in catalog
	for envName, envMapping := range req.EnvMappings {
		provider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, orgName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				s.logger.Warn("Provider not found", "env", envName, "error", err)
				return nil, fmt.Errorf("provider for environment %s not found: %w", envName, utils.ErrLLMProviderNotFound)
			}
			return nil, fmt.Errorf("failed to validate provider for environment %s: %w", envName, err)
		}
		if !provider.InCatalog {
			return nil, fmt.Errorf("%w: provider %s must be in catalog for environment %s", utils.ErrInvalidInput, envMapping.ProviderName, envName)
		}
	}

	// Validate environment UUIDs exist
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range envs {
		envMap[env.Name] = env
	}

	for envName := range req.EnvMappings {
		if _, exists := envMap[envName]; !exists {
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}
	}

	// Build config struct (UUID assigned on Create)
	config := &models.AgentConfiguration{
		Name:             req.Name,
		Description:      req.Description,
		AgentID:          agentID,
		TypeID:           models.AgentConfigTypeToID(req.Type),
		OrganizationName: orgName,
		ProjectName:      projectName,
	}

	// Phase 1 — Short TX: persist config row only.
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.agentConfigRepo.Create(ctx, tx, config); err != nil {
			if errors.Is(err, utils.ErrAgentConfigAlreadyExists) {
				return err
			}
			return fmt.Errorf("failed to create configuration: %w", err)
		}
		return nil
	}); err != nil {
		if errors.Is(err, utils.ErrAgentConfigAlreadyExists) {
			return nil, utils.ErrAgentConfigAlreadyExists
		}
		return nil, err
	}

	// Track created resources for rollback across all environments.
	var rollbackResources []rollbackResource

	// Track credentials for external agents.
	var envCredentials map[string]envCredentialData
	if isExternalAgent {
		envCredentials = make(map[string]envCredentialData)
	}

	// Phase 2 — Loop over environments: external ops first, then short per-env TX.
	// NOTE: map iteration order is non-deterministic; partial failures leave a random subset processed.
	for envName, envMapping := range req.EnvMappings {
		// Context cancellation check before each env.
		select {
		case <-ctx.Done():
			// Use a fresh context for cleanup so cancelled ctx doesn't prevent rollback (CRIT-2).
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			s.processRollBack(cleanupCtx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("operation cancelled: %w", ctx.Err())
		default:
		}

		env, exists := envMap[envName]
		if !exists {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}

		envUUID, err := uuid.Parse(env.UUID)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("invalid environment id %q: %w", envName, err)
		}

		// External ops — no transaction held.
		gateway, err := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
		}

		proxyConfig, providerAPIKeyID, providerUUID, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
		}

		proxy, err := s.llmProxyService.Create(orgName, createdBy, proxyConfig)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
		}
		// Track proxy immediately so it is cleaned up on any subsequent failure.
		rollbackResources = append(rollbackResources, rollbackResource{
			proxyHandle:      proxy.Handle,
			providerAPIKeyID: providerAPIKeyID,
			providerUUID:     providerUUID,
		})

		deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
			Name:      fmt.Sprintf("%s-%s-deployment", strings.ToLower(strings.ReplaceAll(config.Name, " ", "-")), strings.ToLower(strings.ReplaceAll(env.Name, " ", "-"))),
			Base:      "current",
			GatewayID: gateway.UUID.String(),
		}, orgName)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
		}
		rollbackResources[len(rollbackResources)-1].deploymentID = deployment.DeploymentID

		proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, &models.CreateAPIKeyRequest{
			Name: fmt.Sprintf("%s-%s-key", strings.ToLower(strings.ReplaceAll(config.Name, " ", "-")), strings.ToLower(strings.ReplaceAll(env.Name, " ", "-"))),
		})
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			s.compensatingDeleteConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
		}
		rollbackResources[len(rollbackResources)-1].proxyAPIKeyID = proxyAPIKey.KeyID

		// Build proxy URL with nil-safe context access.
		var proxyURL string
		if proxy != nil && proxy.Configuration.Context != nil {
			proxyURL = fmt.Sprintf("%s%s", gateway.Vhost, *proxy.Configuration.Context)
		} else {
			proxyURL = gateway.Vhost
		}

		// Capture credentials for external agents.
		if isExternalAgent {
			envCredentials[envUUID.String()] = envCredentialData{
				apiKey:   proxyAPIKey.APIKey,
				proxyURL: proxyURL,
			}
		}

		// Build environment variables (pure computation, no I/O).
		envConfigTemplates, err := s.buildEnvironmentVariables(config.Name)
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			s.compensatingDeleteConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
		}
		variables := []models.AgentEnvConfigVariable{}
		for _, envConfigTemplate := range envConfigTemplates {
			secretReference := ""
			if envConfigTemplate.IsSecret {
				secretReference = s.buildSecretReference(config.Name, env.Name, envConfigTemplate.Key)
			}
			variables = append(variables, models.AgentEnvConfigVariable{
				ConfigUUID:      config.UUID,
				EnvironmentUUID: envUUID,
				VariableName:    envConfigTemplate.Name,
				VariableKey:     envConfigTemplate.Key,
				SecretReference: secretReference,
			})
		}

		// Short per-env TX: DB writes only.
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			mapping := &models.EnvAgentModelMapping{
				ConfigUUID:          config.UUID,
				EnvironmentUUID:     envUUID,
				LLMProxyUUID:        proxy.UUID,
				PolicyConfiguration: models.LLMPolicies(envMapping.Configuration.Policies),
			}
			if err := s.envMappingRepo.Create(ctx, tx, mapping); err != nil {
				return fmt.Errorf("failed to create environment mapping for %s: %w", envName, err)
			}
			if err := s.envVariableRepo.CreateBatch(ctx, tx, variables); err != nil {
				return fmt.Errorf("failed to create environment variables for %s: %w", envName, err)
			}
			return nil
		}); err != nil {
			// CASCADE on config row will clean up any mappings/variables written for earlier envs.
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, err
		}

		s.logger.Info("Created proxy and deployment for environment",
			"environment", envName,
			"proxyURL", proxyURL,
			"proxyUUID", proxy.UUID,
		)
	}

	// Phase 3 — Success.
	s.logger.Info("Agent configuration created successfully",
		"configUUID", config.UUID,
		"configName", config.Name,
		"agentID", agentID,
		"orgName", orgName,
		"projectName", projectName,
		"createdBy", createdBy,
		"environmentCount", len(req.EnvMappings),
	)

	// Return created configuration with credentials for external agents
	if isExternalAgent {
		return s.buildExternalAgentConfigResponse(ctx, config, envCredentials)
	}
	return s.Get(ctx, config.UUID, orgName, projectName, agentID)
}

// Get retrieves a configuration by UUID with project and agent scoping validation
func (s *agentConfigurationService) Get(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string) (*models.AgentModelConfigResponse, error) {
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	// Validate project and agent scoping
	if config.ProjectName != projectName || config.AgentID != agentName {
		return nil, utils.ErrAgentConfigNotFound
	}

	// Check if agent is external
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		// If we can't determine agent type, assume internal (safer default)
		s.logger.Warn("Failed to get agent type, assuming internal", "error", err)
		return s.buildConfigResponse(ctx, config, false)
	}
	isExternal := agent.Provisioning.Type == string(utils.ExternalAgent)

	return s.buildConfigResponse(ctx, config, isExternal)
}

// GetByAgent retrieves configuration by agent ID
func (s *agentConfigurationService) GetByAgent(ctx context.Context, agentID, orgName string) (*models.AgentModelConfigResponse, error) {
	config, err := s.agentConfigRepo.GetByAgentID(ctx, agentID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	// Check if agent is external
	agent, err := s.ocClient.GetComponent(ctx, orgName, config.ProjectName, agentID)
	if err != nil {
		// If we can't determine agent type, assume internal (safer default)
		s.logger.Warn("Failed to get agent type, assuming internal", "error", err)
		return s.buildConfigResponse(ctx, config, false)
	}
	isExternal := agent.Provisioning.Type == string(utils.ExternalAgent)

	return s.buildConfigResponse(ctx, config, isExternal)
}

// List lists all configurations for an organization, project, and agent
func (s *agentConfigurationService) List(ctx context.Context, orgName, projectName, agentName string, limit, offset int) (*models.AgentModelConfigListResponse, error) {
	configs, err := s.agentConfigRepo.ListByAgent(ctx, orgName, projectName, agentName, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list configurations: %w", err)
	}

	count, err := s.agentConfigRepo.CountByAgent(ctx, orgName, projectName, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to count configurations: %w", err)
	}

	items := make([]models.AgentModelConfigListItem, len(configs))
	for i, cfg := range configs {
		items[i] = models.AgentModelConfigListItem{
			UUID:             cfg.UUID.String(),
			Name:             cfg.Name,
			Description:      cfg.Description,
			AgentID:          cfg.AgentID,
			Type:             models.AgentConfigTypeFromID(cfg.TypeID),
			OrganizationName: cfg.OrganizationName,
			ProjectName:      cfg.ProjectName,
			CreatedAt:        cfg.CreatedAt,
		}
	}

	return &models.AgentModelConfigListResponse{
		Configs: items,
		Pagination: models.PaginationInfo{
			Count:  int(count),
			Offset: offset,
			Limit:  limit,
		},
	}, nil
}

// processEnvProviderChange handles Scenario A: provider changed for an existing environment.
// External ops run outside any transaction; a short per-env TX follows.
// Returns the old proxy handle (for later cleanup) and the rollback resource for the new proxy.
func (s *agentConfigurationService) processEnvProviderChange(
	ctx context.Context,
	configUUID uuid.UUID,
	config *models.AgentConfiguration,
	env *models.EnvironmentResponse,
	envUUID uuid.UUID,
	envName string,
	envMapping models.EnvModelConfigRequest,
	existingMapping *models.EnvAgentModelMapping,
	orgName string,
) (oldProxyHandle string, rbRes rollbackResource, err error) {
	s.logger.Info("Provider changed for environment, recreating proxy",
		"environment", envName,
		"oldProviderUUID", existingMapping.LLMProxy.Configuration.Provider,
		"newProviderName", envMapping.ProviderName)

	gateway, err := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
	if err != nil {
		return "", rollbackResource{}, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
	}

	proxyConfig, providerAPIKeyID, providerUUID, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
	if err != nil {
		return "", rollbackResource{}, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
	}

	proxy, err := s.llmProxyService.Create(orgName, models.UserRoleSystem, proxyConfig)
	if err != nil {
		return "", rollbackResource{}, fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
	}
	// Store mapping ID and old proxy UUID so rollback can revert the DB mapping (HIGH-4).
	rbRes = rollbackResource{
		proxyHandle:      proxy.Handle,
		providerAPIKeyID: providerAPIKeyID,
		providerUUID:     providerUUID,
		mappingID:        existingMapping.ID,
		oldProxyUUID:     existingMapping.LLMProxyUUID,
	}

	deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
		Name:      fmt.Sprintf("%s-%s-deployment", strings.ToLower(strings.ReplaceAll(config.Name, " ", "-")), strings.ToLower(strings.ReplaceAll(env.Name, " ", "-"))),
		Base:      "current",
		GatewayID: gateway.UUID.String(),
	}, orgName)
	if err != nil {
		return "", rbRes, fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
	}
	rbRes.deploymentID = deployment.DeploymentID

	proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, &models.CreateAPIKeyRequest{
		Name: fmt.Sprintf("%s-%s-key", strings.ToLower(strings.ReplaceAll(config.Name, " ", "-")), strings.ToLower(strings.ReplaceAll(env.Name, " ", "-"))),
	})
	if err != nil {
		return "", rbRes, fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
	}
	rbRes.proxyAPIKeyID = proxyAPIKey.KeyID

	envConfigTemplates, err := s.buildEnvironmentVariables(config.Name)
	if err != nil {
		return "", rbRes, fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
	}
	variables := []models.AgentEnvConfigVariable{}
	for _, envConfigTemplate := range envConfigTemplates {
		secretReference := ""
		if envConfigTemplate.IsSecret {
			secretReference = s.buildSecretReference(config.Name, env.Name, envConfigTemplate.Key)
		}
		variables = append(variables, models.AgentEnvConfigVariable{
			ConfigUUID:      config.UUID,
			EnvironmentUUID: envUUID,
			VariableName:    envConfigTemplate.Name,
			VariableKey:     envConfigTemplate.Key,
			SecretReference: secretReference,
		})
	}

	// Short per-env TX: DB writes only.
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		existingMapping.LLMProxyUUID = proxy.UUID
		if err := s.envMappingRepo.Update(ctx, tx, existingMapping); err != nil {
			return fmt.Errorf("failed to update environment mapping for %s: %w", envName, err)
		}
		if err := s.envVariableRepo.DeleteByConfigAndEnv(ctx, tx, configUUID, envUUID); err != nil {
			return fmt.Errorf("failed to delete old environment variables for %s: %w", envName, err)
		}
		if err := s.envVariableRepo.CreateBatch(ctx, tx, variables); err != nil {
			return fmt.Errorf("failed to create environment variables for %s: %w", envName, err)
		}
		return nil
	}); err != nil {
		return "", rbRes, err
	}

	if existingMapping.LLMProxy != nil {
		oldProxyHandle = existingMapping.LLMProxy.Handle
	}
	return oldProxyHandle, rbRes, nil
}

// processEnvProxyUpdate handles Scenario B: same provider, update proxy config and redeploy.
// No DB TX needed — mapping already points to the same proxy UUID.
// Returns a non-nil rollback resource only if a new providerAPIKeyID was created.
func (s *agentConfigurationService) processEnvProxyUpdate(
	ctx context.Context,
	config *models.AgentConfiguration,
	env *models.EnvironmentResponse,
	envUUID uuid.UUID,
	envName string,
	envMapping models.EnvModelConfigRequest,
	existingMapping *models.EnvAgentModelMapping,
	orgName string,
) (rollbackResource, error) {
	s.logger.Info("Updating proxy configuration for environment",
		"environment", envName,
		"providerName", envMapping.ProviderName)

	if existingMapping.LLMProxy == nil {
		return rollbackResource{}, fmt.Errorf("existing proxy not found for environment %s", envName)
	}

	gateway, err := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
	}

	proxyConfig, providerAPIKeyID, providerUUID, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
	}

	// LLMProxy.Handle is gorm:"-" and not populated by GORM Preload.
	// The handle is the proxy's Configuration.Name (set by buildLLMProxyConfig).
	proxyHandle := proxyConfig.Configuration.Name
	proxyConfig.UUID = existingMapping.LLMProxy.UUID
	proxyConfig.Handle = proxyHandle
	proxyConfig.CreatedBy = existingMapping.LLMProxy.CreatedBy
	proxyConfig.Status = existingMapping.LLMProxy.Status
	proxyConfig.ProjectUUID = existingMapping.LLMProxy.ProjectUUID

	updatedProxy, err := s.llmProxyService.Update(proxyHandle, orgName, proxyConfig)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to update proxy for environment %s: %w", envName, err)
	}

	gatewayID := gateway.UUID.String()
	deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(updatedProxy.Handle, orgName, &gatewayID, nil)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to get deployments for environment %s: %w", envName, err)
	}

	var existingDeployment *models.Deployment
	for _, dep := range deployments {
		if dep.Status != nil && *dep.Status == models.DeploymentStatusDeployed {
			existingDeployment = dep
			break
		}
	}

	deployBase := "current"
	if existingDeployment != nil {
		deployBase = existingDeployment.DeploymentID.String()
	}
	newDeployment, err := s.llmProxyDeploymentService.DeployLLMProxy(updatedProxy.Handle, &models.DeployAPIRequest{
		Name:      fmt.Sprintf("%s-%s-deployment", strings.ToLower(strings.ReplaceAll(config.Name, " ", "-")), strings.ToLower(strings.ReplaceAll(env.Name, " ", "-"))),
		Base:      deployBase,
		GatewayID: gateway.UUID.String(),
	}, orgName)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to redeploy proxy for environment %s: %w", envName, err)
	}

	s.logger.Info("Proxy configuration updated and redeployed",
		"environment", envName,
		"proxyHandle", updatedProxy.Handle,
		"newDeploymentID", newDeployment.DeploymentID)

	// Persist updated PolicyConfiguration to DB.
	existingMapping.PolicyConfiguration = models.LLMPolicies(envMapping.Configuration.Policies)
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		return s.envMappingRepo.Update(ctx, tx, existingMapping)
	}); err != nil {
		// Return zero-value struct; providerAPIKeyID cleanup handled separately below if needed (LOW-2).
		return rollbackResource{}, fmt.Errorf("failed to update policy configuration for environment %s: %w", envName, err)
	}

	if existingDeployment != nil && existingDeployment.DeploymentID != newDeployment.DeploymentID {
		if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(updatedProxy.Handle, existingDeployment.DeploymentID.String(), orgName); err != nil {
			s.logger.Warn("Failed to clean up old deployment after redeployment",
				"environment", envName,
				"oldDeploymentID", existingDeployment.DeploymentID,
				"error", err)
		}
	}

	return rollbackResource{providerAPIKeyID: providerAPIKeyID, providerUUID: providerUUID}, nil
}

// processNewEnv handles Scenario C: new environment added during update.
// Mirrors Create() per-env logic: external ops then a short per-env TX.
func (s *agentConfigurationService) processNewEnv(
	ctx context.Context,
	configUUID uuid.UUID,
	config *models.AgentConfiguration,
	env *models.EnvironmentResponse,
	envUUID uuid.UUID,
	envName string,
	envMapping models.EnvModelConfigRequest,
	orgName string,
) (rollbackResource, error) {
	s.logger.Info("Adding new environment to configuration",
		"environment", envName,
		"providerName", envMapping.ProviderName)

	gateway, err := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
	}

	proxyConfig, providerAPIKeyID, providerUUID, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
	}

	proxy, err := s.llmProxyService.Create(orgName, models.UserRoleSystem, proxyConfig)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
	}
	rbRes := rollbackResource{proxyHandle: proxy.Handle, providerAPIKeyID: providerAPIKeyID, providerUUID: providerUUID}

	deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
		Name:      fmt.Sprintf("%s-%s-deployment", strings.ToLower(strings.ReplaceAll(config.Name, " ", "-")), strings.ToLower(strings.ReplaceAll(env.Name, " ", "-"))),
		Base:      "current",
		GatewayID: gateway.UUID.String(),
	}, orgName)
	if err != nil {
		return rbRes, fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
	}
	rbRes.deploymentID = deployment.DeploymentID

	proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, &models.CreateAPIKeyRequest{
		Name: fmt.Sprintf("%s-%s-key", strings.ToLower(strings.ReplaceAll(config.Name, " ", "-")), strings.ToLower(strings.ReplaceAll(env.Name, " ", "-"))),
	})
	if err != nil {
		return rbRes, fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
	}
	rbRes.proxyAPIKeyID = proxyAPIKey.KeyID

	envConfigTemplates, err := s.buildEnvironmentVariables(config.Name)
	if err != nil {
		return rbRes, fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
	}
	variables := []models.AgentEnvConfigVariable{}
	for _, envConfigTemplate := range envConfigTemplates {
		secretReference := ""
		if envConfigTemplate.IsSecret {
			secretReference = s.buildSecretReference(config.Name, env.Name, envConfigTemplate.Key)
		}
		variables = append(variables, models.AgentEnvConfigVariable{
			ConfigUUID:      config.UUID,
			EnvironmentUUID: envUUID,
			VariableName:    envConfigTemplate.Name,
			VariableKey:     envConfigTemplate.Key,
			SecretReference: secretReference,
		})
	}

	// Short per-env TX: DB writes only.
	if err := s.db.Transaction(func(tx *gorm.DB) error {
		mapping := &models.EnvAgentModelMapping{
			ConfigUUID:      configUUID,
			EnvironmentUUID: envUUID,
			LLMProxyUUID:    proxy.UUID,
		}
		if err := s.envMappingRepo.Create(ctx, tx, mapping); err != nil {
			return fmt.Errorf("failed to create environment mapping for %s: %w", envName, err)
		}
		if err := s.envVariableRepo.CreateBatch(ctx, tx, variables); err != nil {
			return fmt.Errorf("failed to create environment variables for %s: %w", envName, err)
		}
		return nil
	}); err != nil {
		return rbRes, err
	}

	return rbRes, nil
}

// processEnvRemoval handles Scenario D: environment removed from the request.
// No external calls; short TX to delete mapping and variables.
func (s *agentConfigurationService) processEnvRemoval(
	ctx context.Context,
	configUUID uuid.UUID,
	envUUIDStr string,
	mapping *models.EnvAgentModelMapping,
) error {
	proxyHandle := "<nil>"
	if mapping.LLMProxy != nil {
		proxyHandle = mapping.LLMProxy.Handle
	}
	s.logger.Info("Removing environment from configuration",
		"environment", envUUIDStr,
		"proxyHandle", proxyHandle)

	envUUIDParsed, err := uuid.Parse(envUUIDStr)
	if err != nil {
		return fmt.Errorf("invalid environment UUID %q: %w", envUUIDStr, err)
	}

	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := s.envVariableRepo.DeleteByConfigAndEnv(ctx, tx, configUUID, envUUIDParsed); err != nil {
			return fmt.Errorf("failed to delete environment variables for %s: %w", envUUIDStr, err)
		}
		if err := s.envMappingRepo.Delete(ctx, tx, mapping.ID); err != nil {
			return fmt.Errorf("failed to delete environment mapping for %s: %w", envUUIDStr, err)
		}
		return nil
	})
}

// Update updates an existing configuration with project and agent scoping validation.
// External network calls (proxy create/update/deploy, API key generation) are performed outside
// transactions. Only pure DB writes use short, focused transactions.
//
// NOTE: Partial failure across multiple environments is an accepted limitation (see SAGA.md).
// On failure in env N, envs 1..N-1 may already be updated. Retry is possible but not idempotent.
func (s *agentConfigurationService) Update(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string,
	req models.UpdateAgentModelConfigRequest,
) (*models.AgentModelConfigResponse, error) {
	// Get existing configuration with all mappings
	existingConfig, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get configuration: %w", err)
	}

	// Validate project and agent scoping
	if existingConfig.ProjectName != projectName || existingConfig.AgentID != agentName {
		return nil, utils.ErrAgentConfigNotFound
	}

	// Load environments once; used to key existingEnvMap by name and to validate request envs.
	allEnvs, err := s.infraResourceManager.ListOrgEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]*models.EnvironmentResponse, len(allEnvs))
	uuidToEnvName := make(map[string]string, len(allEnvs))
	for _, e := range allEnvs {
		envMap[e.Name] = e
		uuidToEnvName[e.UUID] = e.Name
	}

	// Build map of existing environment mappings for comparison, keyed by environment name.
	// The request uses env names, so we must match by name (not UUID).
	existingEnvMap := make(map[string]*models.EnvAgentModelMapping, len(existingConfig.EnvMappings))
	for i := range existingConfig.EnvMappings {
		envUUID := existingConfig.EnvMappings[i].EnvironmentUUID.String()
		name := uuidToEnvName[envUUID]
		if name == "" {
			name = envUUID // fall back to UUID if env was deleted
		}
		existingEnvMap[name] = &existingConfig.EnvMappings[i]
	}

	// Validate all providers exist and are in catalog (if envMappings provided)
	if req.EnvMappings != nil {
		for envName, envMapping := range req.EnvMappings {
			provider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, orgName)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					s.logger.Warn("Provider not found", "env", envName, "error", err)
					return nil, fmt.Errorf("provider for environment %s not found: %w", envName, utils.ErrLLMProviderNotFound)
				}
				return nil, fmt.Errorf("failed to validate provider for environment %s: %w", envName, err)
			}
			if !provider.InCatalog {
				return nil, fmt.Errorf("%w: provider %s must be in catalog for environment %s", utils.ErrInvalidInput, envMapping.ProviderName, envName)
			}
		}
	}

	// Phase 1 — Short TX: update name/description only.
	if req.Name != "" {
		existingConfig.Name = req.Name
	}
	if req.Description != "" {
		existingConfig.Description = req.Description
	}
	if req.Name != "" || req.Description != "" {
		if err := s.db.Transaction(func(tx *gorm.DB) error {
			return s.agentConfigRepo.Update(ctx, tx, existingConfig)
		}); err != nil {
			return nil, fmt.Errorf("failed to update configuration: %w", err)
		}
	}

	// If no envMappings provided, return the updated config immediately.
	if req.EnvMappings == nil {
		return s.Get(ctx, configUUID, orgName, projectName, agentName)
	}

	// Track resources for rollback and old proxies to clean up post-success.
	var rollbackResources []rollbackResource
	var proxiesToDelete []string

	// Phase 2/3 — Loop over requested environments, calling scenario helpers.
	// NOTE: map iteration order is non-deterministic; partial failures leave a random subset processed.
	for envName, envMapping := range req.EnvMappings {
		select {
		case <-ctx.Done():
			// Use a fresh context for cleanup so cancelled ctx doesn't prevent rollback (CRIT-2).
			cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cleanupCancel()
			s.rollbackProxies(cleanupCtx, rollbackResources, orgName)
			return nil, fmt.Errorf("operation cancelled: %w", ctx.Err())
		default:
		}

		env, exists := envMap[envName]
		if !exists {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}

		envUUID, err := uuid.Parse(env.UUID)
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			return nil, fmt.Errorf("invalid environment id %q: %w", envName, err)
		}

		existingMapping, hasExisting := existingEnvMap[envName]

		if hasExisting {
			var newProviderUUID string
			if existingMapping.LLMProxy != nil {
				newProvider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, orgName)
				if err == nil {
					newProviderUUID = newProvider.UUID.String()
				}
			}
			providerChanged := existingMapping.LLMProxy != nil &&
				existingMapping.LLMProxy.Configuration.Provider != newProviderUUID

			if providerChanged {
				// Scenario A: provider changed — create new proxy, update mapping, schedule old proxy for cleanup.
				oldHandle, rbRes, err := s.processEnvProviderChange(
					ctx, configUUID, existingConfig, env, envUUID, envName, envMapping, existingMapping, orgName)
				if err != nil {
					s.rollbackProxies(ctx, rollbackResources, orgName)
					return nil, err
				}
				rollbackResources = append(rollbackResources, rbRes)
				if oldHandle != "" {
					proxiesToDelete = append(proxiesToDelete, oldHandle)
				}
			} else {
				// Scenario B: same provider — update proxy config and redeploy. No DB TX needed.
				rbRes, err := s.processEnvProxyUpdate(
					ctx, existingConfig, env, envUUID, envName, envMapping, existingMapping, orgName)
				if err != nil {
					s.rollbackProxies(ctx, rollbackResources, orgName)
					return nil, err
				}
				if rbRes.providerAPIKeyID != "" {
					rollbackResources = append(rollbackResources, rbRes)
				}
			}
			delete(existingEnvMap, envName)
		} else {
			// Scenario C: new environment — create proxy and mapping.
			rbRes, err := s.processNewEnv(
				ctx, configUUID, existingConfig, env, envUUID, envName, envMapping, orgName)
			if err != nil {
				s.rollbackProxies(ctx, rollbackResources, orgName)
				return nil, err
			}
			rollbackResources = append(rollbackResources, rbRes)
		}
	}

	// Phase 4 — Remove environments not in the request (Scenario D).
	for _, mapping := range existingEnvMap {
		if mapping.LLMProxy != nil {
			proxiesToDelete = append(proxiesToDelete, mapping.LLMProxy.Handle)
		}
		if err := s.processEnvRemoval(ctx, configUUID, mapping.EnvironmentUUID.String(), mapping); err != nil {
			// HIGH-6: Phase 2-3 DB changes are already committed. Log enough information for manual reconciliation.
			s.logger.Error("Partial update failure — manual reconciliation required",
				"configUUID", configUUID,
				"action", "manual_cleanup_required",
				"failedAtEnv", mapping.EnvironmentUUID.String(),
				"error", err,
			)
			s.rollbackProxies(ctx, rollbackResources, orgName)
			return nil, err
		}
	}

	// Phase 5 — Post-success proxy cleanup (outside any transaction, best effort).
	cleanupErrors := 0
	for _, proxyHandle := range proxiesToDelete {
		s.logger.Info("Cleaning up replaced proxy", "proxyHandle", proxyHandle)

		deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(proxyHandle, orgName, nil, nil)
		if err != nil {
			s.logger.Error("Failed to get deployments for proxy cleanup",
				"proxyHandle", proxyHandle,
				"error", err,
			)
			cleanupErrors++
		} else {
			for _, dep := range deployments {
				if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(proxyHandle, dep.DeploymentID.String(), orgName); err != nil {
					s.logger.Error("Failed to delete deployment during cleanup",
						"proxyHandle", proxyHandle,
						"deploymentID", dep.DeploymentID,
						"error", err,
					)
					cleanupErrors++
				}
			}
		}

		if err := s.llmProxyService.Delete(proxyHandle, orgName); err != nil {
			s.logger.Error("Failed to delete proxy during cleanup",
				"proxyHandle", proxyHandle,
				"error", err,
			)
			cleanupErrors++
		}
	}

	if cleanupErrors > 0 {
		s.logger.Warn("Cleanup completed with errors",
			"totalProxies", len(proxiesToDelete),
			"errors", cleanupErrors,
		)
	}

	// Audit log for configuration update
	s.logger.Info("Agent configuration updated successfully",
		"configUUID", configUUID,
		"orgName", orgName,
		"updatedFields", func() []string {
			fields := []string{}
			if req.Name != "" {
				fields = append(fields, "name")
			}
			if req.Description != "" {
				fields = append(fields, "description")
			}
			if req.EnvMappings != nil {
				fields = append(fields, "envMappings")
			}
			return fields
		}(),
	)

	// Return updated configuration
	return s.Get(ctx, configUUID, orgName, projectName, agentName)
}

// Delete deletes a configuration and all associated resources with project and agent scoping validation
func (s *agentConfigurationService) Delete(ctx context.Context, configUUID uuid.UUID, orgName, projectName, agentName string) error {
	// Get configuration and mappings
	existingConfig, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrAgentConfigNotFound
		}
		return fmt.Errorf("failed to get configuration: %w", err)
	}

	// Validate project and agent scoping
	if existingConfig.ProjectName != projectName || existingConfig.AgentID != agentName {
		return utils.ErrAgentConfigNotFound
	}

	s.logger.Info("Deleting agent configuration", "configUUID", existingConfig.UUID, "name", existingConfig.Name)

	// Get all environment mappings
	mappings, err := s.envMappingRepo.ListByConfig(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("failed to list environment mappings: %w", err)
	}

	// Delete in transaction (DB records only)
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Delete configuration (cascades to mappings and variables)
		if err := s.agentConfigRepo.Delete(ctx, tx, configUUID, orgName); err != nil {
			return fmt.Errorf("failed to delete configuration: %w", err)
		}
		return nil
	})
	if err != nil {
		return err
	}

	// Clean up proxies outside transaction (best effort with proper logging)
	cleanupErrors := 0
	for _, mapping := range mappings {
		if mapping.LLMProxy != nil {
			// Undeploy and delete proxy
			s.logger.Info("Cleaning up proxy for deleted config",
				"configUUID", configUUID,
				"proxyHandle", mapping.LLMProxy.Handle,
			)

			// Get deployments for this proxy
			deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(mapping.LLMProxy.Handle, orgName, nil, nil)
			if err != nil {
				s.logger.Error("Failed to get deployments during config deletion",
					"proxyHandle", mapping.LLMProxy.Handle,
					"error", err,
				)
				cleanupErrors++
			} else {
				for _, dep := range deployments {
					if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(mapping.LLMProxy.Handle, dep.DeploymentID.String(), orgName); err != nil {
						s.logger.Error("Failed to delete deployment during config deletion",
							"proxyHandle", mapping.LLMProxy.Handle,
							"deploymentID", dep.DeploymentID,
							"error", err,
						)
						cleanupErrors++
					}
				}
			}

			// Delete proxy
			if err := s.llmProxyService.Delete(mapping.LLMProxy.Handle, orgName); err != nil {
				s.logger.Error("Failed to delete proxy during config deletion",
					"proxyHandle", mapping.LLMProxy.Handle,
					"error", err,
				)
				cleanupErrors++
			}
		}
	}

	if cleanupErrors > 0 {
		s.logger.Warn("Configuration deleted but proxy cleanup had errors",
			"configUUID", configUUID,
			"errors", cleanupErrors,
		)
	}

	// Audit log for configuration deletion
	s.logger.Info("Agent configuration deleted successfully",
		"configUUID", configUUID,
		"configName", existingConfig.Name,
		"orgName", orgName,
		"environmentCount", len(mappings),
	)

	return nil
}

// Helper methods

// resolveGatewayForEnvironment selects gateway with AI-first preference
func (s *agentConfigurationService) resolveGatewayForEnvironment(ctx context.Context, envUUID uuid.UUID, orgName string) (*models.Gateway, error) {
	envIDStr := envUUID.String()
	aiType := "ai"
	activeStatus := true

	// Try AI gateway first
	gateways, err := s.gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID:    orgName,
		FunctionalityType: &aiType,
		Status:            &activeStatus,
		EnvironmentID:     &envIDStr,
		Limit:             1,
	})
	if err == nil && len(gateways) > 0 {
		return gateways[0], nil
	}

	// Fallback to any active gateway
	gateways, err = s.gatewayRepo.ListWithFilters(repositories.GatewayFilterOptions{
		OrganizationID: orgName,
		Status:         &activeStatus,
		EnvironmentID:  &envIDStr,
		Limit:          1,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to find gateway: %w", err)
	}
	if len(gateways) == 0 {
		return nil, errors.New("no active gateway found for environment")
	}

	return gateways[0], nil
}

// buildLLMProxyConfig constructs proxy configuration from request.
// Returns the proxy config, provider API key ID, provider UUID, and any error.
// The provider UUID is needed by rollbackProxies to revoke the provider API key on failure.
func (s *agentConfigurationService) buildLLMProxyConfig(
	ctx context.Context,
	config *models.AgentConfiguration,
	envName string,
	envMapping models.EnvModelConfigRequest,
) (*models.LLMProxy, string, string, error) {
	sanitizedConfigName := strings.ToLower(strings.ReplaceAll(config.Name, " ", "-"))
	sanitizedEnvName := strings.ToLower(strings.ReplaceAll(envName, " ", "-"))
	proxyName := fmt.Sprintf("%s-%s-proxy", sanitizedConfigName, sanitizedEnvName)
	contextPath := fmt.Sprintf("/%s-%s", sanitizedConfigName, sanitizedEnvName)

	project, err := s.ocClient.GetProject(ctx, config.OrganizationName, config.ProjectName)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get project from openchoreo: %w", err)
	}

	// Get provider details
	provider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, config.OrganizationName)
	if err != nil {
		return nil, "", "", fmt.Errorf("failed to get provider: %w", err)
	}

	apiKeyId := ""
	providerUUID := provider.UUID.String()

	// Parse project UUID
	projectUUID, err := uuid.Parse(project.UUID)
	if err != nil {
		return nil, "", "", fmt.Errorf("invalid project UUID from openchoreo: %w", err)
	}

	enabled := true
	// Build proxy configuration
	proxyConfig := &models.LLMProxy{
		Description: fmt.Sprintf("LLM proxy for agent %s", config.AgentID),
		ProjectUUID: projectUUID,
		Configuration: models.LLMProxyConfig{
			Name:     proxyName,
			Version:  models.DefaultProxyVersion,
			Context:  &contextPath,
			Provider: provider.UUID.String(),
			Security: &models.SecurityConfig{
				Enabled: &enabled,
				APIKey: &models.APIKeySecurity{
					Enabled: &enabled,
					Key:     "API-Key",
					In:      "header",
				},
			},
			Policies: envMapping.Configuration.Policies,
		},
	}

	var upstreamAuthConfig models.UpstreamAuth

	providerSecurityConfig := provider.Configuration.Security
	if providerSecurityConfig.Enabled != nil && *providerSecurityConfig.Enabled {
		// Provider is secured.
		providerApiKeyConfig := providerSecurityConfig.APIKey

		if providerApiKeyConfig != nil && providerApiKeyConfig.Enabled != nil && *providerApiKeyConfig.Enabled {
			// Provider api key security is enabled.
			apiKey, err := s.llmProviderAPIKeyService.CreateAPIKey(ctx, config.OrganizationName, provider.UUID.String(), &models.CreateAPIKeyRequest{
				Name:        proxyName,
				DisplayName: proxyName,
			})
			if err != nil {
				return nil, "", "", fmt.Errorf("failed to create api key for provider: %w", err)
			}

			apiKeyId = apiKey.KeyID

			upstreamAuthConfig.Type = utils.StrAsStrPointer(models.AuthTypeAPIKey)
			upstreamAuthConfig.Header = utils.StrAsStrPointer(providerApiKeyConfig.Key)
			upstreamAuthConfig.Value = utils.StrAsStrPointer(apiKey.APIKey)
			proxyConfig.Configuration.UpstreamAuth = &upstreamAuthConfig
		}
	}

	return proxyConfig, apiKeyId, providerUUID, nil
}

// buildEnvironmentVariables generates variable names from config name
// Returns error if generated names conflict with system variables
func (s *agentConfigurationService) buildEnvironmentVariables(configName string) ([]EnvConfigTemplate, error) {
	// Sanitize: Replace any character not in A-Za-z0-9_ with '_'
	prefix := strings.Map(func(r rune) rune {
		if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return '_'
	}, configName)

	// Convert to uppercase
	prefix = strings.ToUpper(prefix)

	// If prefix starts with a digit, prepend underscore
	if len(prefix) > 0 && prefix[0] >= '0' && prefix[0] <= '9' {
		prefix = "_" + prefix
	}

	envConfigTemplates := []EnvConfigTemplate{
		{
			Key:             "url",
			Name:            fmt.Sprintf("%s_URL", prefix),
			IsSecret:        false,
			Value:           "",
			SecretReference: "",
		},
		{
			Key:             "apikey",
			Name:            fmt.Sprintf("%s_API_KEY", prefix),
			IsSecret:        true,
			Value:           "",
			SecretReference: "",
		},
	}

	// Validate each generated variable name (not the constant key)
	for _, tmpl := range envConfigTemplates {
		if err := utils.ValidateEnvironmentVariableName(tmpl.Name); err != nil {
			return nil, fmt.Errorf("invalid generated environment variable name %q: %w", tmpl.Name, err)
		}
	}

	return envConfigTemplates, nil
}

// buildSecretReference constructs OpenChoreo secret reference.
// Uses the same comprehensive sanitizer as buildEnvironmentVariables to ensure
// the generated path is valid for config names with special characters.
func (s *agentConfigurationService) buildSecretReference(configName, envName, secretType string) string {
	// Format: choreo:///default/secret/{config-name}-{env-name}-{type}
	secretName := fmt.Sprintf("%s-%s-%s", utils.SanitizeString(configName), utils.SanitizeString(envName), secretType)
	return fmt.Sprintf("choreo:///default/secret/%s", secretName)
}

// rollbackProxies cleans up created proxies, deployments, and API keys on failure
func (s *agentConfigurationService) rollbackProxies(ctx context.Context, resources []rollbackResource, orgName string) {
	s.logger.Warn("Rolling back created proxies and API keys", "count", len(resources))

	// Track unique proxies to delete
	proxyHandles := make(map[string]bool)

	// Clean up each resource
	for _, res := range resources {
		// Revoke the proxy API key if one was created
		if res.proxyAPIKeyID != "" {
			if err := s.llmProxyAPIKeyService.RevokeAPIKey(ctx, orgName, res.proxyHandle, res.proxyAPIKeyID); err != nil {
				s.logger.Error("Failed to revoke proxy API key during rollback",
					"proxyHandle", res.proxyHandle,
					"apiKeyID", res.proxyAPIKeyID,
					"error", err,
				)
			} else {
				s.logger.Info("Revoked proxy API key during rollback",
					"proxyHandle", res.proxyHandle,
					"apiKeyID", res.proxyAPIKeyID,
				)
			}
		}

		// Undeploy deployment
		if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(res.proxyHandle, res.deploymentID.String(), orgName); err != nil {
			s.logger.Error("Failed to undeploy proxy during rollback",
				"handle", res.proxyHandle,
				"deploymentID", res.deploymentID,
				"error", err,
			)
		}

		// Revoke provider API key if one was created (CRIT-3).
		if res.providerAPIKeyID != "" && res.providerUUID != "" {
			if err := s.llmProviderAPIKeyService.RevokeAPIKey(ctx, orgName, res.providerUUID, res.providerAPIKeyID); err != nil {
				s.logger.Error("Failed to revoke provider API key during rollback",
					"providerAPIKeyID", res.providerAPIKeyID,
					"providerUUID", res.providerUUID,
					"error", err,
				)
			} else {
				s.logger.Info("Revoked provider API key during rollback",
					"providerAPIKeyID", res.providerAPIKeyID,
				)
			}
		}

		proxyHandles[res.proxyHandle] = true
	}

	// Delete all unique proxies
	for handle := range proxyHandles {
		if err := s.llmProxyService.Delete(handle, orgName); err != nil {
			s.logger.Error("Failed to delete proxy during rollback",
				"handle", handle,
				"error", err,
			)
		}
	}

	// Revert DB mappings for Scenario A: restore old proxy UUID so the mapping is not left dangling (HIGH-4).
	for _, res := range resources {
		if res.mappingID != 0 && res.oldProxyUUID != uuid.Nil {
			revertErr := s.db.Transaction(func(tx *gorm.DB) error {
				return tx.Model(&models.EnvAgentModelMapping{}).
					Where("id = ?", res.mappingID).
					Update("llm_proxy_uuid", res.oldProxyUUID).Error
			})
			if revertErr != nil {
				s.logger.Error("Failed to revert DB mapping to old proxy UUID during rollback — mapping may be dangling",
					"mappingID", res.mappingID,
					"oldProxyUUID", res.oldProxyUUID,
					"error", revertErr,
				)
			}
		}
	}
}

// buildConfigResponse builds the full configuration response
func (s *agentConfigurationService) buildConfigResponse(ctx context.Context, config *models.AgentConfiguration, includeProxyURL bool) (*models.AgentModelConfigResponse, error) {
	// Get environment names from OpenChoreo
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, config.OrganizationName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]string)
	for _, env := range envs {
		envMap[env.UUID] = env.Name
	}

	s.logger.Info("Building config response", "configUUID", config.UUID, "envCount", len(envs))

	// Build environment model config map
	envModelConfig := make(map[string]models.EnvModelConfigResponse)
	for _, mapping := range config.EnvMappings {
		envName := envMap[mapping.EnvironmentUUID.String()]
		// Fall back to UUID if environment was deleted
		if envName == "" {
			envName = mapping.EnvironmentUUID.String()
		}

		var proxyInfo *models.LLMProxyInfo = nil
		if mapping.LLMProxy != nil {
			proxyInfo = &models.LLMProxyInfo{
				ProxyUUID: utils.StrAsStrPointer(mapping.LLMProxy.UUID.String()),
				Policies:  mapping.PolicyConfiguration,
			}
			if provider, err := s.llmProviderRepo.GetByUUID(mapping.LLMProxy.ProviderUUID.String(), config.OrganizationName); err == nil && provider.Artifact != nil {
				proxyInfo.ProviderName = utils.StrAsStrPointer(provider.Artifact.Handle)
			}

			// Add proxy URL for external agents (subsequent GET calls)
			if includeProxyURL {
				gateway, err := s.resolveGatewayForEnvironment(ctx, mapping.EnvironmentUUID, config.OrganizationName)
				if err == nil && mapping.LLMProxy.Configuration.Context != nil {
					url := fmt.Sprintf("%s%s", gateway.Vhost, *mapping.LLMProxy.Configuration.Context)
					proxyInfo.URL = &url
				} else if err == nil {
					// If no context, just use gateway vhost
					url := gateway.Vhost
					proxyInfo.URL = &url
				}
			}
		}

		envModelConfig[envName] = models.EnvModelConfigResponse{
			EnvironmentName: envName,
			LLMProxy:        proxyInfo,
		}
	}

	// Build environment variables list (only variable names, not secrets)
	envVars := make([]models.EnvironmentVariableConfig, len(config.EnvVariables))
	for i, v := range config.EnvVariables {
		envVars[i] = models.EnvironmentVariableConfig{
			Name: v.VariableName,
			Key:  v.VariableKey,
		}
	}

	return &models.AgentModelConfigResponse{
		UUID:                 config.UUID.String(),
		Name:                 config.Name,
		Description:          config.Description,
		AgentID:              config.AgentID,
		Type:                 models.AgentConfigTypeFromID(config.TypeID),
		OrganizationName:     config.OrganizationName,
		ProjectName:          config.ProjectName,
		EnvModelConfig:       envModelConfig,
		EnvironmentVariables: envVars,
		CreatedAt:            config.CreatedAt,
		UpdatedAt:            config.UpdatedAt,
	}, nil
}

// envCredentialKeys returns the keys (environment UUIDs) of the credential map, for safe logging.
func envCredentialKeys(m map[string]envCredentialData) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// buildExternalAgentConfigResponse builds response with one-time credentials for external agents
func (s *agentConfigurationService) buildExternalAgentConfigResponse(
	ctx context.Context,
	config *models.AgentConfiguration,
	envCredentials map[string]envCredentialData,
) (*models.AgentModelConfigResponse, error) {
	// Reload configuration with relationships (EnvMappings, LLMProxy, etc.)
	reloadedConfig, err := s.agentConfigRepo.GetByUUID(ctx, config.UUID, config.OrganizationName)
	if err != nil {
		return nil, fmt.Errorf("failed to reload configuration: %w", err)
	}

	s.logger.Info("Building external agent config response",
		"configUUID", config.UUID,
		"envMappingCount", len(reloadedConfig.EnvMappings),
		"envCredentialCount", len(envCredentials),
	)

	// Get environment names
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, config.OrganizationName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]string)
	for _, env := range envs {
		envMap[env.UUID] = env.Name
	}

	// Build environment model config map WITH credentials
	envModelConfig := make(map[string]models.EnvModelConfigResponse)
	for _, mapping := range reloadedConfig.EnvMappings {
		envUUID := mapping.EnvironmentUUID.String()
		envName := envMap[envUUID]
		if envName == "" {
			envName = envUUID
		}

		var proxyInfo *models.LLMProxyInfo
		if mapping.LLMProxy != nil {
			proxyInfo = &models.LLMProxyInfo{
				ProxyUUID: utils.StrAsStrPointer(mapping.LLMProxy.UUID.String()),
				Policies:  mapping.PolicyConfiguration,
			}
			if provider, err := s.llmProviderRepo.GetByUUID(mapping.LLMProxy.ProviderUUID.String(), config.OrganizationName); err == nil && provider.Artifact != nil {
				proxyInfo.ProviderName = utils.StrAsStrPointer(provider.Artifact.Handle)
			}

			// Add credentials for external agents
			if creds, ok := envCredentials[envUUID]; ok {
				proxyInfo.URL = &creds.proxyURL
				proxyInfo.APIKey = &creds.apiKey
				s.logger.Info("Added credentials for external agent",
					"envUUID", envUUID,
					"hasProxyURL", creds.proxyURL != "",
					"hasAPIKey", creds.apiKey != "",
				)
			} else {
				s.logger.Warn("No credentials found for environment",
					"envUUID", envUUID,
					"availableEnvUUIDs", envCredentialKeys(envCredentials),
				)
			}
		}

		envModelConfig[envName] = models.EnvModelConfigResponse{
			EnvironmentName: envName,
			LLMProxy:        proxyInfo,
		}
	}

	// Build environment variables list
	envVars := make([]models.EnvironmentVariableConfig, len(reloadedConfig.EnvVariables))
	for i, v := range reloadedConfig.EnvVariables {
		envVars[i] = models.EnvironmentVariableConfig{
			Name: v.VariableName,
			Key:  v.VariableKey,
		}
	}

	return &models.AgentModelConfigResponse{
		UUID:                 reloadedConfig.UUID.String(),
		Name:                 reloadedConfig.Name,
		Description:          reloadedConfig.Description,
		AgentID:              reloadedConfig.AgentID,
		Type:                 models.AgentConfigTypeFromID(reloadedConfig.TypeID),
		OrganizationName:     reloadedConfig.OrganizationName,
		ProjectName:          reloadedConfig.ProjectName,
		EnvModelConfig:       envModelConfig,
		EnvironmentVariables: envVars,
		CreatedAt:            reloadedConfig.CreatedAt,
		UpdatedAt:            reloadedConfig.UpdatedAt,
	}, nil
}

func (s *agentConfigurationService) processRollBack(ctx context.Context, rollbackResources []rollbackResource, orgName string, configUUID uuid.UUID) {
	s.logger.Error("Rolling back created proxies and API keys", "count", len(rollbackResources))
	s.rollbackProxies(ctx, rollbackResources, orgName)
	s.compensatingDeleteConfig(ctx, configUUID, orgName)
	s.logger.Error("Rolled back created proxies and API keys", "count", len(rollbackResources))
}
