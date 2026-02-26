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
	proxyAPIKeyID    string // API key created for the proxy
	providerAPIKeyID string // API key created for the provider
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

// Create creates a new agent model configuration
func (s *agentConfigurationService) Create(ctx context.Context, orgName, projectName, agentID string,
	req models.CreateAgentModelConfigRequest, createdBy string,
) (*models.AgentModelConfigResponse, error) {
	// Validate agent exists
	_, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentID)
	if err != nil {
		// Check if it's a 404 error (agent not found) vs other errors
		if errors.Is(err, utils.ErrAgentNotFound) {
			return nil, utils.ErrAgentNotFound
		}
		// For other errors (unauthorized, internal, etc), return as-is
		return nil, fmt.Errorf("failed to validate agent: %w", err)
	}

	// Note: Duplicate check removed - database unique constraint handles it
	// The constraint on (agent_id, organization_name) prevents race conditions

	// Validate all providers exist and are in catalog
	for envName, envMapping := range req.EnvMappings {
		provider, err := s.llmProviderRepo.GetByUUID(envMapping.ProviderUUID.String(), orgName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("%w for environment %s: %w", utils.ErrLLMProviderNotFound, envName, err)
			}
			return nil, fmt.Errorf("failed to validate provider for environment %s: %w", envName, err)
		}
		if !provider.InCatalog {
			return nil, fmt.Errorf("%w: provider %s must be in catalog for environment %s", utils.ErrInvalidInput, envMapping.ProviderUUID, envName)
		}
	}

	// Validate environment UUIDs exist
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range envs {
		envMap[env.UUID] = env
	}

	for envName := range req.EnvMappings {
		if _, exists := envMap[envName]; !exists {
			return nil, fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
		}
	}

	// Create configuration in transaction
	config := &models.AgentConfiguration{
		Name:             req.Name,
		Description:      req.Description,
		AgentID:          agentID,
		Type:             req.Type,
		OrganizationName: orgName,
		ProjectName:      projectName,
	}

	// Track created resources for rollback
	var rollbackResources []rollbackResource

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Create agent configuration
		if err := s.agentConfigRepo.Create(ctx, tx, config); err != nil {
			// Check if it's a duplicate error from the database constraint
			if errors.Is(err, utils.ErrAgentConfigAlreadyExists) {
				return err // Propagate to outer error handler
			}
			return fmt.Errorf("failed to create configuration: %w", err)
		}

		// For each environment, create proxy, deploy, and create mappings
		for envName, envMapping := range req.EnvMappings {
			// Check if context is cancelled
			select {
			case <-ctx.Done():
				return fmt.Errorf("operation cancelled: %w", ctx.Err())
			default:
			}

			envUUID, err := uuid.Parse(envName)
			if err != nil {
				return fmt.Errorf("invalid environment id %q: %w", envName, err)
			}
			env := envMap[envName]

			// Resolve gateway for environment (AI-first preference)
			gateway, gatewayErr := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
			if gatewayErr != nil {
				return fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, gatewayErr)
			}

			// Build LLM proxy configuration
			proxyConfig, providerAPIKeyID, err := s.buildLLMProxyConfig(ctx, config, envMapping, gateway)
			if err != nil {
				return fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
			}

			// Create LLM proxy
			proxy, err := s.llmProxyService.Create(orgName, createdBy, proxyConfig)
			if err != nil {
				return fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
			}

			// Deploy proxy
			deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
				Name:      fmt.Sprintf("%s-%s-deployment", config.Name, env.Name),
				Base:      "current",
				GatewayID: gateway.UUID.String(),
			}, orgName)
			if err != nil {
				return fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
			}

			// Track resource for rollback (including provider API key)
			rollbackResources = append(rollbackResources, rollbackResource{
				proxyHandle:      proxy.Handle,
				deploymentID:     deployment.DeploymentID,
				providerAPIKeyID: providerAPIKeyID,
			})

			// Generate API key for proxy
			proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, models.UserRoleSystem, &models.CreateAPIKeyRequest{
				Name: fmt.Sprintf("%s-%s-key", config.Name, env.Name),
			})
			if err != nil {
				return fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
			}

			// Update rollback resource with API key ID
			rollbackResources[len(rollbackResources)-1].proxyAPIKeyID = proxyAPIKey.KeyID

			// Create environment mapping
			mapping := &models.EnvAgentModelMapping{
				ConfigUUID:      config.UUID,
				EnvironmentUUID: envUUID,
				LLMProxyUUID:    proxy.UUID,
			}
			if err := s.envMappingRepo.Create(ctx, tx, mapping); err != nil {
				return fmt.Errorf("failed to create environment mapping for %s: %w", envName, err)
			}

			// Build and validate environment variables
			varNames, err := s.buildEnvironmentVariables(config.Name)
			if err != nil {
				return fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
			}

			// Build proxy URL with nil-safe context access
			var proxyURL string
			if proxy != nil && proxy.Configuration.Context != nil {
				proxyURL = fmt.Sprintf("%s%s", gateway.Vhost, *proxy.Configuration.Context)
			} else {
				proxyURL = gateway.Vhost
			}

			// Create environment variable records (secret references, not actual secrets)
			variables := []models.AgentEnvConfigVariable{
				{
					ConfigUUID:      config.UUID,
					EnvironmentUUID: envUUID,
					VariableName:    varNames[0], // URL variable
					SecretReference: s.buildSecretReference(config.Name, env.Name, "url"),
				},
				{
					ConfigUUID:      config.UUID,
					EnvironmentUUID: envUUID,
					VariableName:    varNames[1], // API_KEY variable
					SecretReference: s.buildSecretReference(config.Name, env.Name, "apikey"),
				},
			}
			if err := s.envVariableRepo.CreateBatch(ctx, tx, variables); err != nil {
				return fmt.Errorf("failed to create environment variables for %s: %w", envName, err)
			}

			s.logger.Info("Created proxy and deployment for environment",
				"environment", envName,
				"proxyURL", proxyURL,
				"proxyUUID", proxy.UUID,
			)
		}

		return nil
	})
	if err != nil {
		// Check if it's a duplicate error - no need to rollback proxies
		if errors.Is(err, utils.ErrAgentConfigAlreadyExists) {
			return nil, utils.ErrAgentConfigAlreadyExists
		}
		// Rollback: clean up created proxies and deployments
		s.logger.Error("Transaction failed, rolling back resources", "error", err)
		s.rollbackProxies(ctx, rollbackResources, orgName)
		return nil, err
	}

	// Audit log for configuration creation
	s.logger.Info("Agent configuration created successfully",
		"configUUID", config.UUID,
		"configName", config.Name,
		"agentID", agentID,
		"orgName", orgName,
		"projectName", projectName,
		"createdBy", createdBy,
		"environmentCount", len(req.EnvMappings),
	)

	// Return created configuration
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

	return s.buildConfigResponse(ctx, config)
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

	return s.buildConfigResponse(ctx, config)
}

// List lists all configurations for an organization, project, and agent
func (s *agentConfigurationService) List(ctx context.Context, orgName, projectName, agentName string, limit, offset int) (*models.AgentModelConfigListResponse, error) {
	// List all configurations for the organization
	configs, err := s.agentConfigRepo.List(ctx, orgName, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list configurations: %w", err)
	}

	// Filter by projectName and agentName
	filteredConfigs := make([]models.AgentConfiguration, 0, len(configs))
	for _, cfg := range configs {
		if cfg.ProjectName == projectName && cfg.AgentID == agentName {
			filteredConfigs = append(filteredConfigs, cfg)
		}
	}

	// Count is based on filtered results
	count := int64(len(filteredConfigs))

	items := make([]models.AgentModelConfigListItem, len(filteredConfigs))
	for i, cfg := range filteredConfigs {
		items[i] = models.AgentModelConfigListItem{
			UUID:             cfg.UUID.String(),
			Name:             cfg.Name,
			Description:      cfg.Description,
			AgentID:          cfg.AgentID,
			Type:             cfg.Type,
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

// Update updates an existing configuration with project and agent scoping validation
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

	// Build map of existing environment mappings for comparison
	existingEnvMap := make(map[string]*models.EnvAgentModelMapping)
	for i := range existingConfig.EnvMappings {
		envUUID := existingConfig.EnvMappings[i].EnvironmentUUID.String()
		existingEnvMap[envUUID] = &existingConfig.EnvMappings[i]
	}

	// Validate all providers exist and are in catalog (if envMappings provided)
	if req.EnvMappings != nil {
		for envName, envMapping := range req.EnvMappings {
			provider, err := s.llmProviderRepo.GetByUUID(envMapping.ProviderUUID.String(), orgName)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, fmt.Errorf("%w for environment %s: %w", utils.ErrLLMProviderNotFound, envName, err)
				}
				return nil, fmt.Errorf("failed to validate provider for environment %s: %w", envName, err)
			}
			if !provider.InCatalog {
				return nil, fmt.Errorf("%w: provider %s must be in catalog for environment %s", utils.ErrInvalidInput, envMapping.ProviderUUID, envName)
			}
		}
	}

	// Validate environment UUIDs exist
	envs, err := s.infraResourceManager.ListOrgEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}
	envMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range envs {
		envMap[env.UUID] = env
	}

	// Track resources for rollback
	var rollbackResources []rollbackResource
	var deletedProxies []string

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Update basic fields if provided
		if req.Name != "" {
			existingConfig.Name = req.Name
		}
		if req.Description != "" {
			existingConfig.Description = req.Description
		}

		// Save updated config
		if req.Name != "" || req.Description != "" {
			if err := s.agentConfigRepo.Update(ctx, tx, existingConfig); err != nil {
				return fmt.Errorf("failed to update configuration: %w", err)
			}
		}

		// If no envMappings provided, just update basic fields and return
		if req.EnvMappings == nil {
			return nil
		}

		// Process environment mappings updates
		for envName, envMapping := range req.EnvMappings {
			// Check if context is cancelled
			select {
			case <-ctx.Done():
				return fmt.Errorf("operation cancelled: %w", ctx.Err())
			default:
			}

			envUUID, err := uuid.Parse(envName)
			if err != nil {
				return fmt.Errorf("invalid environment id %q: %w", envName, err)
			}
			env, exists := envMap[envName]
			if !exists {
				return fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envName)
			}

			existingMapping, hasExisting := existingEnvMap[envName]

			if hasExisting {
				// Environment exists - check if provider changed
				// Compare the provider UUID stored in the proxy's configuration with the new provider UUID
				providerChanged := existingMapping.LLMProxy != nil && existingMapping.LLMProxy.Configuration.Provider != envMapping.ProviderUUID.String()

				if providerChanged {
					// Provider changed - need to create new proxy and delete old one
					s.logger.Info("Provider changed for environment, recreating proxy",
						"environment", envName,
						"oldProviderUUID", existingMapping.LLMProxy.Configuration.Provider,
						"newProviderUUID", envMapping.ProviderUUID)

					// Resolve gateway for environment
					gateway, err := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
					if err != nil {
						return fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
					}

					// Build new proxy configuration
					proxyConfig, providerAPIKeyID, err := s.buildLLMProxyConfig(ctx, existingConfig, envMapping, gateway)
					if err != nil {
						return fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
					}

					// Create new LLM proxy
					proxy, err := s.llmProxyService.Create(orgName, models.UserRoleSystem, proxyConfig)
					if err != nil {
						return fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
					}

					// Deploy new proxy
					deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
						Name:      fmt.Sprintf("%s-%s-deployment", existingConfig.Name, env.Name),
						Base:      "current",
						GatewayID: gateway.UUID.String(),
					}, orgName)
					if err != nil {
						return fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
					}

					// Generate API key for new proxy
					proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, models.UserRoleSystem, &models.CreateAPIKeyRequest{
						Name: fmt.Sprintf("%s-%s-key", existingConfig.Name, env.Name),
					})
					if err != nil {
						return fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
					}

					// Track resource for rollback (including provider API key)
					rollbackResources = append(rollbackResources, rollbackResource{
						proxyHandle:      proxy.Handle,
						deploymentID:     deployment.DeploymentID,
						proxyAPIKeyID:    proxyAPIKey.KeyID,
						providerAPIKeyID: providerAPIKeyID,
					})

					// Track old proxy for deletion
					if existingMapping.LLMProxy != nil {
						deletedProxies = append(deletedProxies, existingMapping.LLMProxy.Handle)
					}

					// Update the mapping to point to new proxy
					existingMapping.LLMProxyUUID = proxy.UUID
					if err := s.envMappingRepo.Update(ctx, tx, existingMapping); err != nil {
						return fmt.Errorf("failed to update environment mapping for %s: %w", envName, err)
					}

					// Update environment variables with new secret references
					if err := s.envVariableRepo.DeleteByConfigAndEnv(ctx, tx, configUUID, envUUID); err != nil {
						return fmt.Errorf("failed to delete old environment variables for %s: %w", envName, err)
					}

					varNames, err := s.buildEnvironmentVariables(existingConfig.Name)
					if err != nil {
						return fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
					}
					variables := []models.AgentEnvConfigVariable{
						{
							ConfigUUID:      configUUID,
							EnvironmentUUID: envUUID,
							VariableName:    varNames[0],
							SecretReference: s.buildSecretReference(existingConfig.Name, env.Name, "url"),
						},
						{
							ConfigUUID:      configUUID,
							EnvironmentUUID: envUUID,
							VariableName:    varNames[1],
							SecretReference: s.buildSecretReference(existingConfig.Name, env.Name, "apikey"),
						},
					}
					if err := s.envVariableRepo.CreateBatch(ctx, tx, variables); err != nil {
						return fmt.Errorf("failed to create environment variables for %s: %w", envName, err)
					}
				} else {
					// Provider hasn't changed - update proxy configuration and redeploy
					// This handles policy updates and other configuration changes
					s.logger.Info("Updating proxy configuration for environment",
						"environment", envName,
						"providerUUID", envMapping.ProviderUUID)

					if existingMapping.LLMProxy == nil {
						return fmt.Errorf("existing proxy not found for environment %s", envName)
					}

					// Resolve gateway for environment
					gateway, err := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
					if err != nil {
						return fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
					}

					// Build updated proxy configuration
					proxyConfig, providerAPIKeyID, err := s.buildLLMProxyConfig(ctx, existingConfig, envMapping, gateway)
					if err != nil {
						return fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
					}

					// Update the existing proxy with new configuration
					// Note: Provider UUID and upstream auth are preserved from proxyConfig
					proxyConfig.UUID = existingMapping.LLMProxy.UUID
					proxyConfig.Handle = existingMapping.LLMProxy.Handle
					proxyConfig.CreatedBy = existingMapping.LLMProxy.CreatedBy
					proxyConfig.Status = existingMapping.LLMProxy.Status
					proxyConfig.ProjectUUID = existingMapping.LLMProxy.ProjectUUID

					updatedProxy, err := s.llmProxyService.Update(existingMapping.LLMProxy.Handle, orgName, proxyConfig)
					if err != nil {
						return fmt.Errorf("failed to update proxy for environment %s: %w", envName, err)
					}

					gatewayId := gateway.UUID.String()

					// Get existing deployments for this proxy and gateway
					deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(updatedProxy.Handle, orgName, &gatewayId, nil)
					if err != nil {
						return fmt.Errorf("failed to get deployments for environment %s: %w", envName, err)
					}

					// Find the deployed deployment
					var existingDeployment *models.Deployment
					for _, dep := range deployments {
						if dep.Status != nil && *dep.Status == models.DeploymentStatusDeployed {
							existingDeployment = dep
							break
						}
					}

					// Redeploy with updated configuration
					deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(updatedProxy.Handle, &models.DeployAPIRequest{
						Name:      fmt.Sprintf("%s-%s-deployment", existingConfig.Name, env.Name),
						Base:      existingDeployment.DeploymentID.String(), // Use current proxy configuration
						GatewayID: gateway.UUID.String(),
					}, orgName)
					if err != nil {
						return fmt.Errorf("failed to redeploy proxy for environment %s: %w", envName, err)
					}

					s.logger.Info("Proxy configuration updated and redeployed",
						"environment", envName,
						"proxyHandle", updatedProxy.Handle,
						"newDeploymentID", deployment.DeploymentID)

					// If there was an existing deployment, undeploy it (after successful new deployment)
					if existingDeployment != nil && existingDeployment.DeploymentID != deployment.DeploymentID {
						if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(updatedProxy.Handle, existingDeployment.DeploymentID.String(), orgName); err != nil {
							s.logger.Warn("Failed to clean up old deployment after redeployment",
								"environment", envName,
								"oldDeploymentID", existingDeployment.DeploymentID,
								"error", err)
							// Don't fail the operation - new deployment is already active
						}
					}

					// Track provider API key for rollback if a new one was created
					if providerAPIKeyID != "" {
						rollbackResources = append(rollbackResources, rollbackResource{
							providerAPIKeyID: providerAPIKeyID,
						})
					}
				}
				// Mark as processed
				delete(existingEnvMap, envName)
			} else {
				// New environment - create proxy and mapping
				s.logger.Info("Adding new environment to configuration",
					"environment", envName,
					"providerUUID", envMapping.ProviderUUID)

				// Resolve gateway for environment
				gateway, err := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
				if err != nil {
					return fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
				}

				// Build proxy configuration
				proxyConfig, providerAPIKeyID, err := s.buildLLMProxyConfig(ctx, existingConfig, envMapping, gateway)
				if err != nil {
					return fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
				}

				// Create LLM proxy
				proxy, err := s.llmProxyService.Create(orgName, models.UserRoleSystem, proxyConfig)
				if err != nil {
					return fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
				}

				// Deploy proxy
				deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
					Name:      fmt.Sprintf("%s-%s-deployment", existingConfig.Name, env.Name),
					Base:      "current",
					GatewayID: gateway.UUID.String(),
				}, orgName)
				if err != nil {
					return fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
				}

				// Generate API key
				proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, models.UserRoleSystem, &models.CreateAPIKeyRequest{
					Name: fmt.Sprintf("%s-%s-key", existingConfig.Name, env.Name),
				})
				if err != nil {
					return fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
				}

				// Track resource for rollback (including provider API key)
				rollbackResources = append(rollbackResources, rollbackResource{
					proxyHandle:      proxy.Handle,
					deploymentID:     deployment.DeploymentID,
					proxyAPIKeyID:    proxyAPIKey.KeyID,
					providerAPIKeyID: providerAPIKeyID,
				})

				// Create environment mapping
				mapping := &models.EnvAgentModelMapping{
					ConfigUUID:      configUUID,
					EnvironmentUUID: envUUID,
					LLMProxyUUID:    proxy.UUID,
				}
				if err := s.envMappingRepo.Create(ctx, tx, mapping); err != nil {
					return fmt.Errorf("failed to create environment mapping for %s: %w", envName, err)
				}

				// Create environment variables
				varNames, err := s.buildEnvironmentVariables(existingConfig.Name)
				if err != nil {
					return fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
				}
				variables := []models.AgentEnvConfigVariable{
					{
						ConfigUUID:      configUUID,
						EnvironmentUUID: envUUID,
						VariableName:    varNames[0],
						SecretReference: s.buildSecretReference(existingConfig.Name, env.Name, "url"),
					},
					{
						ConfigUUID:      configUUID,
						EnvironmentUUID: envUUID,
						VariableName:    varNames[1],
						SecretReference: s.buildSecretReference(existingConfig.Name, env.Name, "apikey"),
					},
				}
				if err := s.envVariableRepo.CreateBatch(ctx, tx, variables); err != nil {
					return fmt.Errorf("failed to create environment variables for %s: %w", envName, err)
				}
			}
		}

		// Delete environments that were not in the request (removed environments)
		for envUUID, mapping := range existingEnvMap {
			proxyHandle := "<nil>"
			if mapping.LLMProxy != nil {
				proxyHandle = mapping.LLMProxy.Handle
			}
			s.logger.Info("Removing environment from configuration",
				"environment", envUUID,
				"proxyHandle", proxyHandle)

			// Track proxy for deletion
			if mapping.LLMProxy != nil {
				deletedProxies = append(deletedProxies, mapping.LLMProxy.Handle)
			}

			// Delete environment variables
			envUUIDParsed, parseErr := uuid.Parse(envUUID)
			if parseErr != nil {
				return fmt.Errorf("invalid environment UUID %q: %w", envUUID, parseErr)
			}
			if err := s.envVariableRepo.DeleteByConfigAndEnv(ctx, tx, configUUID, envUUIDParsed); err != nil {
				return fmt.Errorf("failed to delete environment variables for %s: %w", envUUID, err)
			}

			// Delete environment mapping
			if err := s.envMappingRepo.Delete(ctx, tx, mapping.ID); err != nil {
				return fmt.Errorf("failed to delete environment mapping for %s: %w", envUUID, err)
			}
		}

		return nil
	})
	if err != nil {
		// Rollback: clean up created proxies and deployments
		s.logger.Error("Update transaction failed, rolling back resources", "error", err)
		s.rollbackProxies(ctx, rollbackResources, orgName)
		return nil, err
	}

	// Clean up old proxies (outside transaction, best effort with proper logging)
	cleanupErrors := 0
	for _, proxyHandle := range deletedProxies {
		s.logger.Info("Cleaning up replaced proxy", "proxyHandle", proxyHandle)

		// Get deployments for this proxy
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

		// Delete proxy
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
			"totalProxies", len(deletedProxies),
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

	// Delete in transaction
	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Delete configuration (cascades to mappings and variables)
		if err := s.agentConfigRepo.Delete(ctx, tx, configUUID, orgName); err != nil {
			return fmt.Errorf("failed to delete configuration: %w", err)
		}

		// Clean up proxies (best effort with proper logging)
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

		return nil
	})

	// Audit log for configuration deletion (after successful deletion)
	if err == nil {
		s.logger.Info("Agent configuration deleted successfully",
			"configUUID", configUUID,
			"configName", existingConfig.Name,
			"orgName", orgName,
			"environmentCount", len(mappings),
		)
	}

	return err
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

// buildLLMProxyConfig constructs proxy configuration from request
// Returns the proxy config and the provider API key ID for rollback tracking
func (s *agentConfigurationService) buildLLMProxyConfig(
	ctx context.Context,
	config *models.AgentConfiguration,
	envMapping models.EnvModelConfigRequest,
	gateway *models.Gateway,
) (*models.LLMProxy, string, error) {
	proxyName := fmt.Sprintf("%s-proxy", config.Name)
	context := fmt.Sprintf("/%s", strings.ToLower(strings.ReplaceAll(config.Name, " ", "-")))
	vhost := gateway.Vhost

	project, err := s.ocClient.GetProject(ctx, config.OrganizationName, config.ProjectName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get project from openchoreo: %w", err)
	}

	// Get provider details
	provider, err := s.llmProviderRepo.GetByUUID(envMapping.ProviderUUID.String(), config.OrganizationName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get provider: %w", err)
	}

	apiKey, err := s.llmProviderAPIKeyService.CreateAPIKey(ctx, config.OrganizationName, provider.UUID.String(), models.UserRoleSystem, &models.CreateAPIKeyRequest{
		Name:        proxyName,
		DisplayName: proxyName,
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to create api key for provider: %w", err)
	}

	upstreamAuthType := models.AuthTypeAPIKey
	upstreamAuthHeader := "Authorization"

	// Convert policies
	policies, err := s.convertPolicies(envMapping.Configuration.Policies)
	if err != nil {
		return nil, "", fmt.Errorf("failed to convert policies: %w", err)
	}

	// Parse project UUID
	projectUUID, err := uuid.Parse(project.UUID)
	if err != nil {
		return nil, "", fmt.Errorf("invalid project UUID from openchoreo: %w", err)
	}

	// Build proxy configuration
	proxyConfig := &models.LLMProxy{
		Description: fmt.Sprintf("LLM proxy for agent %s", config.AgentID),
		ProjectUUID: projectUUID,
		Configuration: models.LLMProxyConfig{
			Name:     proxyName,
			Version:  models.DefaultProxyVersion,
			Context:  &context,
			Vhost:    &vhost,
			Provider: provider.UUID.String(),
			UpstreamAuth: &models.UpstreamAuth{
				Type:   &upstreamAuthType,
				Value:  &apiKey.APIKey,
				Header: &upstreamAuthHeader,
			},
			Policies: policies,
		},
	}

	return proxyConfig, apiKey.KeyID, nil
}

// convertPolicies converts policy maps to LLMPolicy structs
// Returns error if conversion fails to prevent silent data loss
func (s *agentConfigurationService) convertPolicies(policies []map[string]any) ([]models.LLMPolicy, error) {
	if len(policies) == 0 {
		return []models.LLMPolicy{}, nil
	}

	result := make([]models.LLMPolicy, 0, len(policies))
	for i, p := range policies {
		name, ok := p["name"].(string)
		if !ok {
			return nil, fmt.Errorf("policy at index %d missing required 'name' field", i)
		}

		policy := models.LLMPolicy{
			Name: name,
		}

		// Convert optional version field
		if version, ok := p["version"].(string); ok {
			policy.Version = version
		}

		// Convert paths field
		if pathsRaw, ok := p["paths"].([]any); ok {
			paths := make([]models.LLMPolicyPath, 0, len(pathsRaw))
			for j, pathRaw := range pathsRaw {
				pathMap, ok := pathRaw.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("policy %d path at index %d is not a valid object", i, j)
				}

				path := models.LLMPolicyPath{}
				if pathStr, ok := pathMap["path"].(string); ok {
					path.Path = pathStr
				}

				if methodsRaw, ok := pathMap["methods"].([]any); ok {
					methods := make([]string, 0, len(methodsRaw))
					for _, m := range methodsRaw {
						if methodStr, ok := m.(string); ok {
							methods = append(methods, methodStr)
						}
					}
					path.Methods = methods
				}

				if params, ok := pathMap["params"].(map[string]any); ok {
					path.Params = params
				}

				paths = append(paths, path)
			}
			policy.Paths = paths
		}

		result = append(result, policy)
	}
	return result, nil
}

// buildEnvironmentVariables generates variable names from config name
// Returns error if generated names conflict with system variables
func (s *agentConfigurationService) buildEnvironmentVariables(configName string) ([]string, error) {
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

	varNames := []string{
		fmt.Sprintf("%s_URL", prefix),
		fmt.Sprintf("%s_API_KEY", prefix),
	}

	// Validate each variable name
	for _, varName := range varNames {
		if err := utils.ValidateEnvironmentVariableName(varName); err != nil {
			return nil, fmt.Errorf("invalid environment variable name %s: %w", varName, err)
		}
	}

	return varNames, nil
}

// buildSecretReference constructs OpenChoreo secret reference
func (s *agentConfigurationService) buildSecretReference(configName, envName, secretType string) string {
	// Format: choreo:///default/secret/{config-name}-{env-name}-{type}
	secretName := fmt.Sprintf("%s-%s-%s",
		strings.ToLower(strings.ReplaceAll(configName, " ", "-")),
		strings.ToLower(envName),
		secretType,
	)
	return fmt.Sprintf("choreo:///default/secret/%s", secretName)
}

// rollbackProxies cleans up created proxies, deployments, and API keys on failure
func (s *agentConfigurationService) rollbackProxies(ctx context.Context, resources []rollbackResource, orgName string) {
	s.logger.Warn("Rolling back created proxies and API keys", "count", len(resources))

	// Track unique proxies to delete
	proxyHandles := make(map[string]bool)

	// Clean up each resource
	for _, res := range resources {
		// Log proxy API key for manual cleanup
		// TODO: Implement API key revocation when gateway supports it
		if res.proxyAPIKeyID != "" {
			s.logger.Warn("API key created during failed transaction - manual revocation may be needed",
				"proxyHandle", res.proxyHandle,
				"apiKeyID", res.proxyAPIKeyID,
			)
		}

		// Undeploy deployment
		if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(res.proxyHandle, res.deploymentID.String(), orgName); err != nil {
			s.logger.Error("Failed to undeploy proxy during rollback",
				"handle", res.proxyHandle,
				"deploymentID", res.deploymentID,
				"error", err,
			)
		}

		// Delete provider API key if created
		if res.providerAPIKeyID != "" {
			// Provider API key deletion requires provider UUID - we'll log and skip for now
			// This is tracked separately in buildLLMProxyConfig rollback
			s.logger.Warn("Provider API key cleanup needed",
				"providerAPIKeyID", res.providerAPIKeyID,
			)
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
}

// buildConfigResponse builds the full configuration response
func (s *agentConfigurationService) buildConfigResponse(ctx context.Context, config *models.AgentConfiguration) (*models.AgentModelConfigResponse, error) {
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

		var proxyInfo *models.LLMProxyInfo
		if mapping.LLMProxy != nil {
			var contextVal string
			if mapping.LLMProxy.Configuration.Context != nil {
				contextVal = *mapping.LLMProxy.Configuration.Context
			}
			proxyInfo = &models.LLMProxyInfo{
				ProxyUUID: mapping.LLMProxy.UUID.String(),
				ProxyName: mapping.LLMProxy.Name,
				Context:   contextVal,
				Status:    mapping.LLMProxy.Status,
			}
		}

		envModelConfig[envName] = models.EnvModelConfigResponse{
			EnvironmentUUID: mapping.EnvironmentUUID.String(),
			EnvironmentName: envName,
			LLMProxy:        proxyInfo,
		}
	}

	// Build environment variables list (only variable names, not secrets)
	envVars := make([]models.EnvironmentVariableConfig, len(config.EnvVariables))
	for i, v := range config.EnvVariables {
		envVars[i] = models.EnvironmentVariableConfig{
			Name: v.VariableName,
		}
	}

	return &models.AgentModelConfigResponse{
		UUID:                 config.UUID.String(),
		Name:                 config.Name,
		Description:          config.Description,
		AgentID:              config.AgentID,
		Type:                 config.Type,
		OrganizationName:     config.OrganizationName,
		ProjectName:          config.ProjectName,
		EnvModelConfig:       envModelConfig,
		EnvironmentVariables: envVars,
		CreatedAt:            config.CreatedAt,
		UpdatedAt:            config.UpdatedAt,
	}, nil
}
