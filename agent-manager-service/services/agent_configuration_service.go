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
	Get(ctx context.Context, configUUID uuid.UUID, orgName string) (*models.AgentModelConfigResponse, error)
	GetByAgent(ctx context.Context, agentID, orgName string) (*models.AgentModelConfigResponse, error)
	List(ctx context.Context, orgName string, limit, offset int) (*models.AgentModelConfigListResponse, error)
	Update(ctx context.Context, configUUID uuid.UUID, orgName string,
		req models.UpdateAgentModelConfigRequest) (*models.AgentModelConfigResponse, error)
	Delete(ctx context.Context, configUUID uuid.UUID, orgName string) error
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
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	// Check for duplicate configuration for this agent
	existingConfig, err := s.agentConfigRepo.GetByAgentID(ctx, agentID, orgName)
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing config: %w", err)
	}
	if existingConfig != nil && existingConfig.UUID != uuid.Nil {
		return nil, utils.ErrAgentConfigAlreadyExists
	}

	// Validate all providers exist and are in catalog
	for envName, envMapping := range req.EnvMappings {
		provider, err := s.llmProviderRepo.GetByUUID(envMapping.ProviderUUID.String(), orgName)
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil, fmt.Errorf("provider not found for environment %s: %w", envName, err)
			}
			return nil, fmt.Errorf("failed to validate provider for environment %s: %w", envName, err)
		}
		if !provider.InCatalog {
			return nil, fmt.Errorf("provider %s must be in catalog for environment %s", envMapping.ProviderUUID, envName)
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
			return nil, fmt.Errorf("environment not found: %s", envName)
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
	var createdProxies []string
	var createdDeployments []uuid.UUID

	err = s.db.Transaction(func(tx *gorm.DB) error {
		// Create agent configuration
		if err := s.agentConfigRepo.Create(ctx, tx, config); err != nil {
			return fmt.Errorf("failed to create configuration: %w", err)
		}

		// For each environment, create proxy, deploy, and create mappings
		for envName, envMapping := range req.EnvMappings {
			envUUID, _ := uuid.Parse(envName)
			env := envMap[envName]

			// Resolve gateway for environment (AI-first preference)
			gateway, err := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
			if err != nil {
				return fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
			}

			// Build LLM proxy configuration
			proxyConfig, err := s.buildLLMProxyConfig(ctx, config, envMapping, gateway)
			if err != nil {
				return fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
			}

			// Create LLM proxy
			proxy, err := s.llmProxyService.Create(orgName, createdBy, proxyConfig)
			if err != nil {
				return fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
			}
			createdProxies = append(createdProxies, proxy.Handle)

			// Deploy proxy
			deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
				Name:      fmt.Sprintf("%s-%s-deployment", config.Name, env.Name),
				Base:      envName,
				GatewayID: gateway.UUID.String(),
			}, orgName)
			if err != nil {
				return fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
			}
			createdDeployments = append(createdDeployments, deployment.DeploymentID)

			// Generate API key for proxy
			_, err = s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, "system", &models.CreateAPIKeyRequest{
				Name: fmt.Sprintf("%s-%s-key", config.Name, env.Name),
			})
			if err != nil {
				return fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
			}

			// Create environment mapping
			mapping := &models.EnvAgentModelMapping{
				ConfigUUID:      config.UUID,
				EnvironmentUUID: envUUID,
				LLMProxyUUID:    proxy.UUID,
			}
			if err := s.envMappingRepo.Create(ctx, tx, mapping); err != nil {
				return fmt.Errorf("failed to create environment mapping for %s: %w", envName, err)
			}

			// Build environment variables
			varNames := s.buildEnvironmentVariables(config.Name)
			proxyURL := fmt.Sprintf("%s%s", gateway.Vhost, *proxy.Configuration.Context)

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
		// Rollback: clean up created proxies and deployments
		s.logger.Error("Transaction failed, rolling back resources", "error", err)
		s.rollbackProxies(ctx, createdProxies, createdDeployments, orgName)
		return nil, err
	}

	// Return created configuration
	return s.Get(ctx, config.UUID, orgName)
}

// Get retrieves a configuration by UUID
func (s *agentConfigurationService) Get(ctx context.Context, configUUID uuid.UUID, orgName string) (*models.AgentModelConfigResponse, error) {
	config, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrAgentConfigNotFound
		}
		return nil, fmt.Errorf("failed to get configuration: %w", err)
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

// List lists all configurations for an organization
func (s *agentConfigurationService) List(ctx context.Context, orgName string, limit, offset int) (*models.AgentModelConfigListResponse, error) {
	configs, err := s.agentConfigRepo.List(ctx, orgName, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to list configurations: %w", err)
	}

	count, err := s.agentConfigRepo.Count(ctx, orgName)
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

// Update updates an existing configuration
func (s *agentConfigurationService) Update(ctx context.Context, configUUID uuid.UUID, orgName string,
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
					return nil, fmt.Errorf("provider not found for environment %s: %w", envName, err)
				}
				return nil, fmt.Errorf("failed to validate provider for environment %s: %w", envName, err)
			}
			if !provider.InCatalog {
				return nil, fmt.Errorf("provider %s must be in catalog for environment %s", envMapping.ProviderUUID, envName)
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
	var createdProxies []string
	var createdDeployments []uuid.UUID
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
			envUUID, _ := uuid.Parse(envName)
			env, exists := envMap[envName]
			if !exists {
				return fmt.Errorf("environment not found: %s", envName)
			}

			existingMapping, hasExisting := existingEnvMap[envName]

			if hasExisting {
				// Environment exists - check if provider changed
				if existingMapping.LLMProxyUUID.String() != envMapping.ProviderUUID.String() {
					// Provider changed - need to create new proxy and delete old one
					s.logger.Info("Provider changed for environment, recreating proxy",
						"environment", envName,
						"oldProxyUUID", existingMapping.LLMProxyUUID,
						"newProviderUUID", envMapping.ProviderUUID)

					// Resolve gateway for environment
					gateway, err := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
					if err != nil {
						return fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
					}

					// Build new proxy configuration
					proxyConfig, err := s.buildLLMProxyConfig(ctx, existingConfig, envMapping, gateway)
					if err != nil {
						return fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
					}

					// Create new LLM proxy
					proxy, err := s.llmProxyService.Create(orgName, "system", proxyConfig)
					if err != nil {
						return fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
					}
					createdProxies = append(createdProxies, proxy.Handle)

					// Deploy new proxy
					deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
						Name:      fmt.Sprintf("%s-%s-deployment", existingConfig.Name, env.Name),
						Base:      envName,
						GatewayID: gateway.UUID.String(),
					}, orgName)
					if err != nil {
						return fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
					}
					createdDeployments = append(createdDeployments, deployment.DeploymentID)

					// Generate API key for new proxy
					_, err = s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, "system", &models.CreateAPIKeyRequest{
						Name: fmt.Sprintf("%s-%s-key", existingConfig.Name, env.Name),
					})
					if err != nil {
						return fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
					}

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

					varNames := s.buildEnvironmentVariables(existingConfig.Name)
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
				// If provider hasn't changed, no action needed
				delete(existingEnvMap, envName) // Mark as processed
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
				proxyConfig, err := s.buildLLMProxyConfig(ctx, existingConfig, envMapping, gateway)
				if err != nil {
					return fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
				}

				// Create LLM proxy
				proxy, err := s.llmProxyService.Create(orgName, "system", proxyConfig)
				if err != nil {
					return fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
				}
				createdProxies = append(createdProxies, proxy.Handle)

				// Deploy proxy
				deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
					Name:      fmt.Sprintf("%s-%s-deployment", existingConfig.Name, env.Name),
					Base:      envName,
					GatewayID: gateway.UUID.String(),
				}, orgName)
				if err != nil {
					return fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
				}
				createdDeployments = append(createdDeployments, deployment.DeploymentID)

				// Generate API key
				_, err = s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, "system", &models.CreateAPIKeyRequest{
					Name: fmt.Sprintf("%s-%s-key", existingConfig.Name, env.Name),
				})
				if err != nil {
					return fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
				}

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
				varNames := s.buildEnvironmentVariables(existingConfig.Name)
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
			s.logger.Info("Removing environment from configuration",
				"environment", envUUID,
				"proxyHandle", mapping.LLMProxy.Handle)

			// Track proxy for deletion
			if mapping.LLMProxy != nil {
				deletedProxies = append(deletedProxies, mapping.LLMProxy.Handle)
			}

			// Delete environment variables
			envUUIDParsed, _ := uuid.Parse(envUUID)
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
		s.rollbackProxies(ctx, createdProxies, createdDeployments, orgName)
		return nil, err
	}

	// Clean up old proxies (outside transaction, best effort)
	for _, proxyHandle := range deletedProxies {
		s.logger.Info("Cleaning up replaced proxy", "proxyHandle", proxyHandle)

		// Get deployments for this proxy
		deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(proxyHandle, orgName, nil, nil)
		if err == nil {
			for _, dep := range deployments {
				_ = s.llmProxyDeploymentService.DeleteLLMProxyDeployment(proxyHandle, dep.DeploymentID.String(), orgName)
			}
		}

		// Delete proxy
		_ = s.llmProxyService.Delete(proxyHandle, orgName)
	}

	// Return updated configuration
	return s.Get(ctx, configUUID, orgName)
}

// Delete deletes a configuration and all associated resources
func (s *agentConfigurationService) Delete(ctx context.Context, configUUID uuid.UUID, orgName string) error {
	// Get configuration and mappings
	existingConfig, err := s.agentConfigRepo.GetByUUID(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrAgentConfigNotFound
		}
		return fmt.Errorf("failed to get configuration: %w", err)
	}

	s.logger.Info("Deleting agent configuration", "configUUID", existingConfig.UUID, "name", existingConfig.Name)

	// Get all environment mappings
	mappings, err := s.envMappingRepo.ListByConfig(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("failed to list environment mappings: %w", err)
	}

	// Delete in transaction
	return s.db.Transaction(func(tx *gorm.DB) error {
		// Delete configuration (cascades to mappings and variables)
		if err := s.agentConfigRepo.Delete(ctx, tx, configUUID, orgName); err != nil {
			return fmt.Errorf("failed to delete configuration: %w", err)
		}

		// Clean up proxies (best effort)
		for _, mapping := range mappings {
			if mapping.LLMProxy != nil {
				// Undeploy and delete proxy
				s.logger.Info("Cleaning up proxy for deleted config",
					"configUUID", configUUID,
					"proxyHandle", mapping.LLMProxy.Handle,
				)

				// Get deployments for this proxy
				deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(mapping.LLMProxy.Handle, orgName, nil, nil)
				if err == nil {
					for _, dep := range deployments {
						_ = s.llmProxyDeploymentService.DeleteLLMProxyDeployment(mapping.LLMProxy.Handle, dep.DeploymentID.String(), orgName)
					}
				}

				// Delete proxy
				_ = s.llmProxyService.Delete(mapping.LLMProxy.Handle, orgName)
			}
		}

		return nil
	})
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
func (s *agentConfigurationService) buildLLMProxyConfig(
	ctx context.Context,
	config *models.AgentConfiguration,
	envMapping models.EnvModelConfigRequest,
	gateway *models.Gateway,
) (*models.LLMProxy, error) {
	proxyName := fmt.Sprintf("%s-proxy", config.Name)
	context := fmt.Sprintf("/%s", strings.ToLower(strings.ReplaceAll(config.Name, " ", "-")))
	vhost := gateway.Vhost

	// Get provider details
	provider, err := s.llmProviderRepo.GetByUUID(envMapping.ProviderUUID.String(), config.OrganizationName)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	apiKey, err := s.llmProviderAPIKeyService.CreateAPIKey(ctx, config.OrganizationName, provider.UUID.String(), "system", &models.CreateAPIKeyRequest{
		Name:        proxyName,
		DisplayName: proxyName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create api key for provider: %w", err)
	}

	upstreamAuthType := "api-key"
	upstreamAuthHeader := "api-key"

	// Build proxy configuration
	proxyConfig := &models.LLMProxy{
		Description: fmt.Sprintf("LLM proxy for agent %s", config.AgentID),
		Configuration: models.LLMProxyConfig{
			Name:     proxyName,
			Version:  "1.0.0",
			Context:  &context,
			Vhost:    &vhost,
			Provider: provider.UUID.String(),
			UpstreamAuth: &models.UpstreamAuth{
				Type:   &upstreamAuthType,
				Value:  &apiKey.APIKey,
				Header: &upstreamAuthHeader,
			},
			Policies: s.convertPolicies(envMapping.Configuration.Policies),
		},
	}

	return proxyConfig, nil
}

// convertPolicies converts policy maps to LLMPolicy structs
func (s *agentConfigurationService) convertPolicies(policies []map[string]interface{}) []models.LLMPolicy {
	result := make([]models.LLMPolicy, 0, len(policies))
	for _, p := range policies {
		// Convert map to LLMPolicy - this is a simplified version
		// Real implementation would need proper type conversion
		if name, ok := p["name"].(string); ok {
			result = append(result, models.LLMPolicy{
				Name: name,
				// Add other fields as needed
			})
		}
	}
	return result
}

// buildEnvironmentVariables generates variable names from config name
func (s *agentConfigurationService) buildEnvironmentVariables(configName string) []string {
	// Convert to uppercase and replace spaces with underscores
	prefix := strings.ToUpper(strings.ReplaceAll(configName, " ", "_"))
	return []string{
		fmt.Sprintf("%s_URL", prefix),
		fmt.Sprintf("%s_API_KEY", prefix),
	}
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

// rollbackProxies cleans up created proxies on failure
func (s *agentConfigurationService) rollbackProxies(ctx context.Context, proxyHandles []string, deploymentUUIDs []uuid.UUID, orgName string) {
	s.logger.Warn("Rolling back created proxies", "count", len(proxyHandles))

	// Undeploy all created deployments
	for _, depUUID := range deploymentUUIDs {
		// Find which proxy this deployment belongs to
		for _, handle := range proxyHandles {
			if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(handle, depUUID.String(), orgName); err != nil {
				s.logger.Error("Failed to undeploy proxy during rollback",
					"handle", handle,
					"deploymentUUID", depUUID,
					"error", err,
				)
			}
		}
	}

	// Delete all created proxies
	for _, handle := range proxyHandles {
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
		var proxyInfo *models.LLMProxyInfo
		if mapping.LLMProxy != nil {
			proxyInfo = &models.LLMProxyInfo{
				ProxyUUID: mapping.LLMProxy.UUID.String(),
				ProxyName: mapping.LLMProxy.Name,
				Context:   *mapping.LLMProxy.Configuration.Context,
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
