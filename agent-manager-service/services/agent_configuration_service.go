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
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/secretmanagersvc"
	appConfig "github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
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
	secretClient              secretmanagersvc.SecretManagementClient
}

// rollbackResource tracks a proxy, its deployment, and API keys for cleanup
type rollbackResource struct {
	proxyHandle        string
	deploymentID       uuid.UUID
	proxyAPIKeyID      string    // API key created for the proxy
	providerAPIKeyID   string    // API key name created for the provider
	providerUUID       string    // UUID of the provider (needed to revoke the provider API key)
	mappingID          uint      // ID of the env mapping to revert (HIGH-4, Scenario A only)
	oldProxyUUID       uuid.UUID // old proxy UUID to restore in the mapping on rollback (HIGH-4, Scenario A only)
	providerSecretPath string    // KV path for provider API key secret
	proxySecretPath    string    // KV path for proxy API key secret
	secretRefName      string    // Name of the SecretReference CR to delete on rollback (internal agents only)
}

// nonK8sNameChar matches any character not valid in a Kubernetes resource name segment.
var nonK8sNameChar = regexp.MustCompile(`[^a-z0-9-]`)

// multiHyphenRe matches two or more consecutive hyphens.
var multiHyphenRe = regexp.MustCompile(`-{2,}`)

// sanitizeForK8sName converts a string to a valid Kubernetes resource name segment.
// It lowercases the input, replaces spaces and underscores with hyphens, strips
// remaining invalid characters, collapses consecutive hyphens, trims leading/trailing
// hyphens, and caps the result at 63 characters.
func sanitizeForK8sName(s string) string {
	s = strings.ToLower(s)
	s = strings.NewReplacer(" ", "-", "_", "-").Replace(s)
	s = nonK8sNameChar.ReplaceAllString(s, "")
	s = multiHyphenRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = strings.TrimRight(s[:63], "-")
	}
	return s
}

// buildSecretRefName constructs the SecretReference CR name for an agent config + environment pair.
// The combined name is truncated to 63 characters (Kubernetes resource name limit) with trailing
// hyphens trimmed after truncation.
func buildSecretRefName(configName, envName string) string {
	name := fmt.Sprintf("%s-%s-secrets", sanitizeForK8sName(configName), sanitizeForK8sName(envName))
	if len(name) > 63 {
		name = strings.TrimRight(name[:63], "-")
	}
	return name
}

// buildProxyURL constructs the proxy base URL from a gateway vhost and an optional context path.

func buildProxyURL(vhost string, contextPath *string) string {
	if contextPath != nil {
		return fmt.Sprintf("%s%s", vhost, *contextPath)
	}
	return vhost
}

// buildLLMEnvVars constructs the two env vars (URL and API key) from the env config templates.
func buildLLMEnvVars(templates []EnvConfigTemplate, proxyURL, secretRefName string) []client.EnvVar {
	var urlTemplate, apiKeyTemplate EnvConfigTemplate
	for _, t := range templates {
		switch t.Key {
		case "url":
			urlTemplate = t
		case "apikey":
			apiKeyTemplate = t
		}
	}
	return []client.EnvVar{
		{Key: urlTemplate.Name, Value: proxyURL},
		{
			Key: apiKeyTemplate.Name,
			ValueFrom: &client.EnvVarValueFrom{
				SecretKeyRef: &client.SecretKeyRef{
					Name: secretRefName,
					Key:  secretmanagersvc.SecretKeyAPIKey,
				},
			},
		},
	}
}

// createSecretReference creates a SecretReference CR that mounts the proxy API key as a K8s Secret.
// proxyKVPath is the full KV path (including the key segment); the method strips the key before
// passing the path to the CR so that spec.kvPath points to the secret object, not the key.
func (s *agentConfigurationService) createSecretReference(ctx context.Context, orgName, secretRefName, projectName, componentName, proxyKVPath string) error {
	_, err := s.ocClient.CreateSecretReference(ctx, orgName, client.CreateSecretReferenceRequest{
		Name:            secretRefName,
		ProjectName:     projectName,
		ComponentName:   componentName,
		KVPath:          proxyKVPath,
		SecretKeys:      []string{secretmanagersvc.SecretKeyAPIKey},
		RefreshInterval: appConfig.GetConfig().SecretManager.RefreshInterval,
	})
	return err
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
	secretClient secretmanagersvc.SecretManagementClient,
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
		secretClient:              secretClient,
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
	if _, err := s.buildEnvironmentVariables(req.Name, req.EnvironmentVariables); err != nil {
		return nil, errors.Join(utils.ErrInvalidInput, err)
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

	// Resolve first/dev environment name for ReleaseBinding patch (internal agents only).
	firstEnvName := ""
	if !isExternalAgent {
		pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
		if pipelineErr != nil {
			s.logger.Warn("failed to get deployment pipeline; ReleaseBinding patch will be skipped", "err", pipelineErr)
		} else if pipeline != nil {
			firstEnvName = client.FindFirstEnvironment(pipeline.PromotionPaths)
		}
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

		proxyConfig, providerAPIKeyID, providerUUID, providerSecretPath, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
		}
		// Track provider credentials immediately so they are cleaned up even if proxy creation fails.
		rollbackResources = append(rollbackResources, rollbackResource{
			providerAPIKeyID:   providerAPIKeyID,
			providerUUID:       providerUUID,
			providerSecretPath: providerSecretPath,
		})
		// Capture index immediately after append to avoid fragile len(slice)-1 indexing below.
		rbIdx := len(rollbackResources) - 1

		proxy, err := s.llmProxyService.Create(orgName, createdBy, proxyConfig)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
		}
		// Update the rollback entry with the proxy handle now that it was created.
		rollbackResources[rbIdx].proxyHandle = proxy.Handle

		deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
			Name:      fmt.Sprintf("%s-%s-deployment", sanitizeForK8sName(config.Name), sanitizeForK8sName(env.Name)),
			Base:      "current",
			GatewayID: gateway.UUID.String(),
		}, orgName)
		if err != nil {
			s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
			return nil, fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
		}
		rollbackResources[rbIdx].deploymentID = deployment.DeploymentID

		proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, &models.CreateAPIKeyRequest{
			Name: fmt.Sprintf("%s-%s-key", sanitizeForK8sName(config.Name), sanitizeForK8sName(env.Name)),
		})
		s.logger.Info("Created proxy API key", "proxyHandle", proxy.Handle, "proxyKeyName", proxyAPIKey.KeyID, "name", fmt.Sprintf("%s-%s-key", sanitizeForK8sName(config.Name), sanitizeForK8sName(env.Name)))
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			s.compensatingDeleteConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
		}
		rollbackResources[rbIdx].proxyAPIKeyID = proxyAPIKey.KeyID

		// Store proxy API key in OpenBao KV
		proxySecretLoc := secretmanagersvc.SecretLocation{
			OrgName:         orgName,
			ProjectName:     projectName,
			AgentName:       agentID,
			EnvironmentName: env.Name,
			ConfigName:      config.Name,
			EntityName:      proxy.Handle,
			SecretKey:       secretmanagersvc.SecretKeyAPIKey,
		}
		proxyKVPath, err := s.secretClient.CreateSecret(ctx, proxySecretLoc,
			map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			s.compensatingDeleteConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to store proxy API key in KV for environment %s: %w", envName, err)
		}
		rollbackResources[rbIdx].proxySecretPath = proxyKVPath

		// Build proxy URL with nil-safe context access.
		var proxyContext *string
		if proxy != nil {
			proxyContext = proxy.Configuration.Context
		}
		proxyURL := buildProxyURL(gateway.Vhost, proxyContext)

		// Capture credentials for external agents.
		if isExternalAgent {
			envCredentials[envUUID.String()] = envCredentialData{
				apiKey:   proxyAPIKey.APIKey,
				proxyURL: proxyURL,
			}
		}

		// Build environment variables (pure computation, no I/O).
		envConfigTemplates, err := s.buildEnvironmentVariables(config.Name, req.EnvironmentVariables)
		if err != nil {
			s.rollbackProxies(ctx, rollbackResources, orgName)
			s.compensatingDeleteConfig(ctx, config.UUID, orgName)
			return nil, fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
		}
		variables := []models.AgentEnvConfigVariable{}
		for _, envConfigTemplate := range envConfigTemplates {
			secretReference := ""
			if envConfigTemplate.IsSecret {
				secretReference = proxyKVPath
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

		// Internal-agent only: create SecretReference and inject per-env vars into ReleaseBinding.
		// The Component CR (global, shared across envs) is updated once after the loop using the
		// first-environment's vars to avoid last-write-wins clobbering (HIGH-3).
		if !isExternalAgent {
			secretRefName := buildSecretRefName(config.Name, envName)

			// Step 1: Create SecretReference CR so the proxy API key is available as a K8s Secret.
			if err := s.createSecretReference(ctx, orgName, secretRefName, projectName, agentID, proxyKVPath); err != nil {
				s.processRollBack(ctx, rollbackResources, orgName, config.UUID)
				return nil, fmt.Errorf("failed to create SecretReference for environment %s: %w", envName, err)
			}
			rollbackResources[rbIdx].secretRefName = secretRefName

			// Step 2: Build the two env vars (URL plain, API key via secretKeyRef).
			envVarsToInject := buildLLMEnvVars(envConfigTemplates, proxyURL, secretRefName)

			// Step 3: Inject per-environment URL and API key ref into the ReleaseBinding.
			// Each environment gets its own ReleaseBinding with the correct per-env proxy URL,
			// avoiding last-write-wins clobbering in the global Component CR.
			if err := s.ocClient.UpdateReleaseBindingEnvVars(ctx, orgName, projectName, agentID, envName, envVarsToInject); err != nil {
				s.logger.Warn("failed to patch ReleaseBinding for env var injection (will apply on next deploy)",
					"environment", envName, "err", err)
			}

			// Step 4: For the first/dev environment, also update the Component CR once as a bootstrap
			// default so agents with no ReleaseBinding yet have a working config.
			if firstEnvName != "" && envName == firstEnvName {
				if err := s.ocClient.UpdateComponentEnvVars(ctx, orgName, projectName, agentID, envVarsToInject); err != nil {
					s.logger.Error("failed to update Component CR env vars for internal agent — Component CR in inconsistent state",
						"environment", envName, "err", err)
				}
			}
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
	existingVarNames map[string]string,
	isExternalAgent bool,
	firstEnvName string,
) (oldProxyHandle string, rbRes rollbackResource, err error) {
	s.logger.Info("Provider changed for environment, recreating proxy",
		"environment", envName,
		"oldProviderUUID", existingMapping.LLMProxy.Configuration.Provider,
		"newProviderName", envMapping.ProviderName)

	gateway, err := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
	if err != nil {
		return "", rollbackResource{}, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
	}

	proxyConfig, providerAPIKeyID, providerUUID, providerSecretPath, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
	if err != nil {
		return "", rollbackResource{}, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
	}
	// Register provider credentials immediately so they are cleaned up on any subsequent failure.
	rbRes = rollbackResource{
		providerAPIKeyID:   providerAPIKeyID,
		providerUUID:       providerUUID,
		providerSecretPath: providerSecretPath,
		mappingID:          existingMapping.ID,
		oldProxyUUID:       existingMapping.LLMProxyUUID,
	}

	proxy, err := s.llmProxyService.Create(orgName, models.UserRoleSystem, proxyConfig)
	if err != nil {
		return "", rbRes, fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
	}
	rbRes.proxyHandle = proxy.Handle

	deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
		Name:      fmt.Sprintf("%s-%s-deployment", sanitizeForK8sName(config.Name), sanitizeForK8sName(env.Name)),
		Base:      "current",
		GatewayID: gateway.UUID.String(),
	}, orgName)
	if err != nil {
		return "", rbRes, fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
	}
	rbRes.deploymentID = deployment.DeploymentID

	proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, &models.CreateAPIKeyRequest{
		Name: fmt.Sprintf("%s-%s-key", sanitizeForK8sName(config.Name), sanitizeForK8sName(env.Name)),
	})
	if err != nil {
		return "", rbRes, fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
	}
	rbRes.proxyAPIKeyID = proxyAPIKey.KeyID

	// Store proxy API key in OpenBao KV
	proxySecretLoc := secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     config.ProjectName,
		AgentName:       config.AgentID,
		EnvironmentName: env.Name,
		ConfigName:      config.Name,
		EntityName:      proxy.Handle,
		SecretKey:       secretmanagersvc.SecretKeyAPIKey,
	}
	proxyKVPath, err := s.secretClient.CreateSecret(ctx, proxySecretLoc,
		map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, orgName)
		return "", rollbackResource{}, fmt.Errorf("processEnvProviderChange: failed to store proxy API key in KV for environment %s: %w", envName, err)
	}
	rbRes.proxySecretPath = proxyKVPath

	envConfigTemplates, err := s.buildEnvironmentVariables(config.Name, varNamesToOverrides(existingVarNames))
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, orgName)
		return "", rollbackResource{}, fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
	}
	variables := []models.AgentEnvConfigVariable{}
	for _, envConfigTemplate := range envConfigTemplates {
		secretReference := ""
		if envConfigTemplate.IsSecret {
			secretReference = proxyKVPath
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

	// Internal-agent only: upsert SecretReference (delete best-effort, then re-create), inject env vars.
	if !isExternalAgent {
		// The SecretReference name is derived from config.Name + envName. Since config.Name does not
		// change within an update and only the KVPath changes (new provider), this is effectively an
		// upsert: delete the old one (best-effort) and create a fresh one.
		secretRefName := buildSecretRefName(config.Name, envName)
		_ = s.ocClient.DeleteSecretReference(ctx, orgName, secretRefName) // best-effort delete before re-create
		if crErr := s.createSecretReference(ctx, orgName, secretRefName, config.ProjectName, config.AgentID, proxyKVPath); crErr != nil {
			return oldProxyHandle, rbRes, fmt.Errorf("failed to create SecretReference for %s/%s: %w", config.Name, envName, crErr)
		}
		rbRes.secretRefName = secretRefName

		proxyURL := buildProxyURL(gateway.Vhost, proxy.Configuration.Context)
		envVarsToInject := buildLLMEnvVars(envConfigTemplates, proxyURL, secretRefName)
		if uvErr := s.ocClient.UpdateComponentEnvVars(ctx, orgName, config.ProjectName, config.AgentID, envVarsToInject); uvErr != nil {
			s.logger.Error("failed to update Component CR env vars in Scenario A — Component CR in inconsistent state", "env", envName, "err", uvErr)
		}
		if firstEnvName != "" && envName == firstEnvName {
			if rbErr := s.ocClient.UpdateReleaseBindingEnvVars(ctx, orgName, config.ProjectName, config.AgentID, firstEnvName, envVarsToInject); rbErr != nil {
				s.logger.Warn("failed to patch ReleaseBinding in Scenario A", "env", envName, "err", rbErr)
			}
		}
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
	existingVarNames map[string]string,
	isExternalAgent bool,
	firstEnvName string,
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

	proxyConfig, providerUUID, err := s.buildLLMProxyUpdateConfig(config, envMapping, existingMapping.LLMProxy)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
	}

	// LLMProxy.Handle is gorm:"-" and not populated by GORM Preload.
	// Use the existing proxy's handle (Configuration.Name) rather than recomputing it,
	// so the proxy identity is preserved exactly as created.
	proxyHandle := existingMapping.LLMProxy.Configuration.Name
	proxyConfig.UUID = existingMapping.LLMProxy.UUID
	proxyConfig.Handle = proxyHandle
	proxyConfig.CreatedBy = existingMapping.LLMProxy.CreatedBy
	proxyConfig.Status = existingMapping.LLMProxy.Status

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
	newDeployment, err := s.llmProxyDeploymentService.DeployLLMProxy(updatedProxy.Handle, &models.DeployAPIRequest{
		Name:      fmt.Sprintf("%s-%s-deployment", sanitizeForK8sName(config.Name), sanitizeForK8sName(env.Name)),
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

	// Internal-agent only: update Component CR env vars (proxy URL may have changed).
	// The SecretReference is NOT updated here: the proxy handle is unchanged in Scenario B,
	// so the KVPath is identical and the existing SecretReference already points to the correct secret.
	if !isExternalAgent {
		secretRefName := buildSecretRefName(config.Name, envName)
		proxyURL := buildProxyURL(gateway.Vhost, updatedProxy.Configuration.Context)
		envConfigTemplates, err := s.buildEnvironmentVariables(config.Name, varNamesToOverrides(existingVarNames))
		if err != nil {
			s.logger.Warn("failed to build env config templates in Scenario B", "err", err)
		}
		envVarsToInject := buildLLMEnvVars(envConfigTemplates, proxyURL, secretRefName)
		if uvErr := s.ocClient.UpdateComponentEnvVars(ctx, orgName, config.ProjectName, config.AgentID, envVarsToInject); uvErr != nil {
			s.logger.Error("failed to update Component CR env vars in Scenario B — Component CR in inconsistent state", "env", envName, "err", uvErr)
		}
		if firstEnvName != "" && envName == firstEnvName {
			if rbErr := s.ocClient.UpdateReleaseBindingEnvVars(ctx, orgName, config.ProjectName, config.AgentID, firstEnvName, envVarsToInject); rbErr != nil {
				s.logger.Warn("failed to patch ReleaseBinding in Scenario B", "env", envName, "err", rbErr)
			}
		}
	}

	return rollbackResource{providerUUID: providerUUID}, nil
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
	existingVarNames map[string]string,
	isExternalAgent bool,
	firstEnvName string,
) (rollbackResource, error) {
	s.logger.Info("Adding new environment to configuration",
		"environment", envName,
		"providerName", envMapping.ProviderName)

	gateway, err := s.resolveGatewayForEnvironment(ctx, envUUID, orgName)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to resolve gateway for environment %s: %w", envName, err)
	}

	proxyConfig, providerAPIKeyID, providerUUID, providerSecretPath, err := s.buildLLMProxyConfig(ctx, config, env.Name, envMapping)
	if err != nil {
		return rollbackResource{}, fmt.Errorf("failed to build proxy config for environment %s: %w", envName, err)
	}
	// Register provider credentials immediately so they are cleaned up on any subsequent failure.
	rbRes := rollbackResource{providerAPIKeyID: providerAPIKeyID, providerUUID: providerUUID, providerSecretPath: providerSecretPath}

	proxy, err := s.llmProxyService.Create(orgName, models.UserRoleSystem, proxyConfig)
	if err != nil {
		return rbRes, fmt.Errorf("failed to create proxy for environment %s: %w", envName, err)
	}
	rbRes.proxyHandle = proxy.Handle

	deployment, err := s.llmProxyDeploymentService.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{
		Name:      fmt.Sprintf("%s-%s-deployment", sanitizeForK8sName(config.Name), sanitizeForK8sName(env.Name)),
		Base:      "current",
		GatewayID: gateway.UUID.String(),
	}, orgName)
	if err != nil {
		return rbRes, fmt.Errorf("failed to deploy proxy for environment %s: %w", envName, err)
	}
	rbRes.deploymentID = deployment.DeploymentID

	proxyAPIKey, err := s.llmProxyAPIKeyService.CreateAPIKey(ctx, orgName, proxy.Handle, &models.CreateAPIKeyRequest{
		Name: fmt.Sprintf("%s-%s-key", sanitizeForK8sName(config.Name), sanitizeForK8sName(env.Name)),
	})
	if err != nil {
		return rbRes, fmt.Errorf("failed to generate API key for environment %s: %w", envName, err)
	}
	rbRes.proxyAPIKeyID = proxyAPIKey.KeyID

	// Store proxy API key in OpenBao KV
	proxySecretLoc := secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     config.ProjectName,
		AgentName:       config.AgentID,
		EnvironmentName: env.Name,
		ConfigName:      config.Name,
		EntityName:      proxy.Handle,
		SecretKey:       secretmanagersvc.SecretKeyAPIKey,
	}
	proxyKVPath, err := s.secretClient.CreateSecret(ctx, proxySecretLoc,
		map[string]string{secretmanagersvc.SecretKeyAPIKey: proxyAPIKey.APIKey})
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, orgName)
		return rollbackResource{}, fmt.Errorf("processNewEnv: failed to store proxy API key in KV for environment %s: %w", envName, err)
	}
	rbRes.proxySecretPath = proxyKVPath

	envConfigTemplates, err := s.buildEnvironmentVariables(config.Name, varNamesToOverrides(existingVarNames))
	if err != nil {
		s.rollbackProxies(ctx, []rollbackResource{rbRes}, orgName)
		return rollbackResource{}, fmt.Errorf("failed to build environment variables for %s: %w", envName, err)
	}
	variables := []models.AgentEnvConfigVariable{}
	for _, envConfigTemplate := range envConfigTemplates {
		secretReference := ""
		if envConfigTemplate.IsSecret {
			secretReference = proxyKVPath
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

	// Internal-agent only: create SecretReference and inject per-env vars into ReleaseBinding.
	// The Component CR (global) is updated only for the first/dev environment to avoid
	// last-write-wins clobbering across multiple environments (HIGH-3).
	if !isExternalAgent {
		secretRefName := buildSecretRefName(config.Name, envName)
		if crErr := s.createSecretReference(ctx, orgName, secretRefName, config.ProjectName, config.AgentID, proxyKVPath); crErr != nil {
			return rbRes, fmt.Errorf("failed to create SecretReference for %s/%s: %w", config.Name, envName, crErr)
		}
		rbRes.secretRefName = secretRefName

		proxyURL := ""
		if scGateway, gwErr := s.resolveGatewayForEnvironment(ctx, envUUID, orgName); gwErr != nil {
			s.logger.Warn("failed to resolve gateway in Scenario C, skipping env var injection", "err", gwErr)
		} else {
			proxyURL = buildProxyURL(scGateway.Vhost, proxy.Configuration.Context)
		}
		envVarsToInject := buildLLMEnvVars(envConfigTemplates, proxyURL, secretRefName)
		// Inject per-env URL into the ReleaseBinding for this specific environment.
		if rbErr := s.ocClient.UpdateReleaseBindingEnvVars(ctx, orgName, config.ProjectName, config.AgentID, envName, envVarsToInject); rbErr != nil {
			s.logger.Warn("failed to patch ReleaseBinding in Scenario C", "env", envName, "err", rbErr)
		}
		// Update Component CR only for the first/dev environment as a bootstrap default.
		if firstEnvName != "" && envName == firstEnvName {
			if uvErr := s.ocClient.UpdateComponentEnvVars(ctx, orgName, config.ProjectName, config.AgentID, envVarsToInject); uvErr != nil {
				s.logger.Error("failed to update Component CR env vars in Scenario C — Component CR in inconsistent state", "env", envName, "err", uvErr)
			}
		}
	}

	return rbRes, nil
}

// processEnvRemoval handles Scenario D: environment removed from the request.
// Removes env vars from the ReleaseBinding (and Component CR if this is the
// last remaining environment) before deleting DB records.
func (s *agentConfigurationService) processEnvRemoval(
	ctx context.Context,
	configUUID uuid.UUID,
	envUUIDStr string,
	mapping *models.EnvAgentModelMapping,
	configName string,
	envName string,
	orgName string,
	projectName string,
	agentName string,
	isExternalAgent bool,
	existingVarNames map[string]string,
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

	// Internal-agent only: remove env vars from Component CR and the removed environment's ReleaseBinding.
	if !isExternalAgent && envName != "" {
		// Delete SecretReference CR for this environment (best-effort).
		secretRefName := buildSecretRefName(configName, envName)
		if delErr := s.ocClient.DeleteSecretReference(ctx, orgName, secretRefName); delErr != nil {
			s.logger.Warn("failed to delete SecretReference in Scenario D", "name", secretRefName, "err", delErr)
		}

		// Build the list of env var keys from DB-persisted names so user-overridden names are respected.
		envConfigTemplates, buildErr := s.buildEnvironmentVariables(configName, varNamesToOverrides(existingVarNames))
		if buildErr != nil {
			s.logger.Warn("failed to build env var keys for Scenario D cleanup, skipping env var removal", "err", buildErr)
		} else {
			keysToRemove := make([]string, 0, len(envConfigTemplates))
			for _, t := range envConfigTemplates {
				keysToRemove = append(keysToRemove, t.Name)
			}
			// Remove from the removed environment's ReleaseBinding.
			if rbErr := s.ocClient.RemoveReleaseBindingEnvVars(ctx, orgName, projectName, agentName, envName, keysToRemove); rbErr != nil {
				s.logger.Warn("failed to remove env vars from ReleaseBinding in Scenario D", "environment", envName, "err", rbErr)
			}
			// Remove from the Component CR (global default) — best-effort.
			if compErr := s.ocClient.RemoveComponentEnvironmentVariables(ctx, orgName, projectName, agentName, keysToRemove); compErr != nil {
				s.logger.Warn("failed to remove env vars from Component CR in Scenario D", "environment", envName, "err", compErr)
			}
		}
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

	// Phase 1b — Update env var names if provided (global rename across all environments).
	// Read, validate, and write happen inside a single transaction with a row-level lock to
	// prevent concurrent rename requests from bypassing uniqueness checks.
	if len(req.EnvironmentVariables) > 0 {
		// Capture the old names before the rename so we can remove them from CRs afterwards.
		oldVarNames, oldNamesErr := s.loadExistingVarNames(ctx, configUUID)

		if err := s.db.Transaction(func(tx *gorm.DB) error {
			// Lock the rows so concurrent renames on the same config are serialised.
			vars, err := s.envVariableRepo.ListByConfigForUpdate(ctx, tx, configUUID)
			if err != nil {
				return fmt.Errorf("failed to load existing variable names: %w", err)
			}
			// Build key→name map from locked rows (first-occurrence wins per key).
			persistedVarNames := make(map[string]string)
			for _, v := range vars {
				if _, already := persistedVarNames[v.VariableKey]; !already {
					persistedVarNames[v.VariableKey] = v.VariableName
				}
			}
			// Merge requested renames over persisted names.
			mergedOverrides := make([]models.EnvironmentVariableConfig, 0, len(persistedVarNames))
			for key, name := range persistedVarNames {
				mergedOverrides = append(mergedOverrides, models.EnvironmentVariableConfig{Key: key, Name: name})
			}
			for _, ev := range req.EnvironmentVariables {
				found := false
				for i, mo := range mergedOverrides {
					if mo.Key == ev.Key {
						mergedOverrides[i].Name = ev.Name
						found = true
						break
					}
				}
				if !found {
					mergedOverrides = append(mergedOverrides, ev)
				}
			}
			// Validate using the merged result (catches uniqueness and format errors against locked names).
			if _, err := s.buildEnvironmentVariables(existingConfig.Name, mergedOverrides); err != nil {
				return errors.Join(utils.ErrInvalidInput, err)
			}
			keyNameMap := make(map[string]string, len(req.EnvironmentVariables))
			for _, ev := range req.EnvironmentVariables {
				keyNameMap[ev.Key] = ev.Name
			}
			return s.envVariableRepo.UpdateVariableNames(ctx, tx, configUUID, keyNameMap)
		}); err != nil {
			return nil, fmt.Errorf("failed to update environment variable names: %w", err)
		}

		// For internal agents: remove old env var names from the Component CR and all
		// per-environment ReleaseBindings so stale variables don't linger after a rename.
		// Only runs when at least one name actually changed; skipped entirely if nothing differed.
		// Best-effort — failures are logged but do not abort the update.
		if oldNamesErr == nil && len(oldVarNames) > 0 {
			// Collect names that were actually renamed (old name != new name).
			changedOldKeys := make([]string, 0, len(req.EnvironmentVariables))
			for _, ev := range req.EnvironmentVariables {
				if existing, ok := oldVarNames[ev.Key]; ok && existing != ev.Name {
					changedOldKeys = append(changedOldKeys, existing)
				}
			}
			if len(changedOldKeys) > 0 {
				agentComp, compErr := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
				if compErr != nil {
					s.logger.Warn("Phase 1b: failed to determine agent type for env var cleanup", "err", compErr)
				} else if agentComp.Provisioning.Type != string(utils.ExternalAgent) {
					// Remove old names from Component CR.
					if rmErr := s.ocClient.RemoveComponentEnvironmentVariables(ctx, orgName, projectName, agentName, changedOldKeys); rmErr != nil {
						s.logger.Warn("Phase 1b: failed to remove old env vars from Component CR", "err", rmErr)
					}
					// Remove old names from each environment's ReleaseBinding.
					for i := range existingConfig.EnvMappings {
						envUUID := existingConfig.EnvMappings[i].EnvironmentUUID.String()
						envName := uuidToEnvName[envUUID]
						if envName == "" {
							continue
						}
						if rmErr := s.ocClient.RemoveReleaseBindingEnvVars(ctx, orgName, projectName, agentName, envName, changedOldKeys); rmErr != nil {
							s.logger.Warn("Phase 1b: failed to remove old env vars from ReleaseBinding",
								"environment", envName, "err", rmErr)
						}
					}
				}
			}
		}
	}

	// If no envMappings provided, return the updated config immediately.
	if req.EnvMappings == nil {
		return s.Get(ctx, configUUID, orgName, projectName, agentName)
	}

	// Load existing variable names so new/replaced envs get consistent names.
	existingVarNames, err := s.loadExistingVarNames(ctx, configUUID)
	if err != nil {
		return nil, err
	}

	// Determine agent type and first env for internal-agent env var injection.
	// Fail closed: if GetComponent errors, return rather than defaulting to internal (which could corrupt CRs).
	agentComp, agentErr := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if agentErr != nil {
		return nil, fmt.Errorf("failed to determine agent type: %w", agentErr)
	}
	isExternalAgent := agentComp.Provisioning.Type == string(utils.ExternalAgent)
	firstEnvName := ""
	if !isExternalAgent {
		if pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName); pipelineErr == nil && pipeline != nil {
			firstEnvName = client.FindFirstEnvironment(pipeline.PromotionPaths)
		}
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
					ctx, configUUID, existingConfig, env, envUUID, envName, envMapping, existingMapping, orgName, existingVarNames, isExternalAgent, firstEnvName)
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
					ctx, existingConfig, env, envUUID, envName, envMapping, existingMapping, orgName, existingVarNames, isExternalAgent, firstEnvName)
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
				ctx, configUUID, existingConfig, env, envUUID, envName, envMapping, orgName, existingVarNames, isExternalAgent, firstEnvName)
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
		removedEnvName := uuidToEnvName[mapping.EnvironmentUUID.String()]
		if err := s.processEnvRemoval(ctx, configUUID, mapping.EnvironmentUUID.String(), mapping, existingConfig.Name, removedEnvName, orgName, projectName, agentName, isExternalAgent, existingVarNames); err != nil {
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

	// Determine agent type for internal-agent cleanup decisions.
	// Fail closed: if GetComponent errors, return rather than defaulting to internal (which could corrupt CRs).
	agentComp, agentErr := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if agentErr != nil {
		return fmt.Errorf("failed to determine agent type: %w", agentErr)
	}
	isExternalAgent := agentComp.Provisioning.Type == string(utils.ExternalAgent)

	s.logger.Info("Deleting agent configuration", "configUUID", existingConfig.UUID, "name", existingConfig.Name)

	// Get all environment mappings
	mappings, err := s.envMappingRepo.ListByConfig(ctx, configUUID)
	if err != nil {
		return fmt.Errorf("failed to list environment mappings: %w", err)
	}

	environments, err := s.ocClient.ListEnvironments(ctx, orgName)
	if err != nil {
		return fmt.Errorf("error while list environments from open choreo. %w", err)
	}

	envIDNameMap := make(map[string]string)

	for _, env := range environments {
		envIDNameMap[env.UUID] = env.Name
	}

	// Steps 1-4: Per-mapping cleanup in strict order before DB deletion.
	// External resources are cleaned up before DB deletion so that if any step fails,
	// the DB row remains and the caller can retry. On retry, already-deleted external
	// resources are skipped gracefully.
	// Order matters: revoke API keys (1) before undeploying (2) so the gateway still has
	// the proxy config when it processes the revocation event.
	//
	// Key names mirror the naming convention used during Create/buildLLMProxyConfig:
	//   proxyHandle       = "{sanitizedConfigName}-{sanitizedEnvName}-proxy"  (= Configuration.Name)
	//   proxy API key     = "{sanitizedConfigName}-{sanitizedEnvName}-key"
	//   provider API key  = "{sanitizedConfigName}-{sanitizedEnvName}-proxy"  (= proxyHandle)
	for _, mapping := range mappings {
		if mapping.LLMProxy == nil {
			continue
		}
		env, ok := envIDNameMap[mapping.EnvironmentUUID.String()]
		if !ok {
			s.logger.Warn("environment is not available in openchoreo")
			continue
		}

		// Configuration.Name = proxyHandle = "{sanitizedConfigName}-{sanitizedEnvName}-proxy".
		// Use it directly as the proxy handle (Handle field is gorm:"-" and not populated by Preload).
		proxyHandle := mapping.LLMProxy.Configuration.Name

		// Step 1: Revoke API keys (must happen before undeployment so the gateway still has
		// the proxy config when it processes the revocation event).
		proxyKeyName := fmt.Sprintf("%s-key", strings.TrimSuffix(proxyHandle, "-proxy"))
		providerKeyName := proxyHandle

		s.logger.Info("Revoking API keys", "proxyHandle", proxyHandle, "proxyKeyName", proxyKeyName, "providerKeyName", providerKeyName)

		if err := s.llmProxyAPIKeyService.RevokeAPIKey(ctx, orgName, proxyHandle, proxyKeyName); err != nil {
			s.logger.Warn("Failed to revoke proxy API key during deletion (best-effort)",
				"proxyHandle", proxyHandle,
				"keyName", proxyKeyName,
				"error", err,
			)
		}

		// Revoke provider API key (only if provider auth was configured).
		if mapping.LLMProxy.Configuration.UpstreamAuth != nil {
			providerUUID := mapping.LLMProxy.ProviderUUID.String()
			if err := s.llmProviderAPIKeyService.RevokeAPIKey(ctx, orgName, providerUUID, providerKeyName); err != nil {
				s.logger.Warn("Failed to revoke provider API key during deletion (best-effort)",
					"providerUUID", providerUUID,
					"keyName", providerKeyName,
					"error", err,
				)
			}
		}

		// Step 1b: Delete SecretReference CR (internal agents only, best-effort).
		if !isExternalAgent {
			secretRefName := buildSecretRefName(existingConfig.Name, env)
			if err := s.ocClient.DeleteSecretReference(ctx, orgName, secretRefName); err != nil {
				s.logger.Warn("failed to delete SecretReference on config delete",
					"name", secretRefName, "err", err)
			}
		}

		// Step 2: Undeploy proxy deployments.
		s.logger.Info("Cleaning up proxy deployments for deleted config",
			"configUUID", configUUID,
			"proxyHandle", proxyHandle,
		)

		deployments, err := s.llmProxyDeploymentService.GetLLMProxyDeployments(proxyHandle, orgName, nil, nil)
		if err != nil {
			if errors.Is(err, utils.ErrLLMProxyNotFound) {
				// Proxy already gone — skip deployment cleanup for this mapping.
				s.logger.Info("Proxy already deleted, skipping deployment cleanup",
					"proxyHandle", proxyHandle,
				)
			} else {
				return fmt.Errorf("failed to get deployments for proxy %q: %w", proxyHandle, err)
			}
		} else {
			for _, dep := range deployments {
				if _, err := s.llmProxyDeploymentService.UndeployLLMProxyDeployment(proxyHandle, dep.DeploymentID.String(), dep.GatewayUUID.String(), orgName); err != nil {
					s.logger.Error("Failed to undeploy deployment during cleanup",
						"proxyHandle", proxyHandle,
						"deploymentID", dep.DeploymentID,
						"gatewayID", dep.GatewayUUID,
						"error", err,
					)
				}
			}
		}

		// Step 3: Delete proxy record.
		if err := s.llmProxyService.Delete(proxyHandle, orgName); err != nil {
			// ErrLLMProxyNotFound means already deleted — treat as success.
			if !errors.Is(err, utils.ErrLLMProxyNotFound) {
				return fmt.Errorf("failed to delete proxy %q: %w", proxyHandle, err)
			}
			s.logger.Info("Proxy already deleted, skipping", "proxyHandle", proxyHandle)
		}

		// Step 4: Delete KV secrets.
		if mapping.LLMProxy.Configuration.UpstreamAuth != nil &&
			mapping.LLMProxy.Configuration.UpstreamAuth.SecretRef != nil {
			if err := s.secretClient.DeleteSecretByPath(ctx,
				*mapping.LLMProxy.Configuration.UpstreamAuth.SecretRef); err != nil {
				return fmt.Errorf("failed to delete provider API key from KV for proxy %q: %w",
					proxyHandle, err)
			}
		}

		proxySecretLoc := secretmanagersvc.SecretLocation{
			OrgName:         existingConfig.OrganizationName,
			ProjectName:     existingConfig.ProjectName,
			AgentName:       existingConfig.AgentID,
			EnvironmentName: env,
			ConfigName:      existingConfig.Name,
			EntityName:      proxyHandle,
			SecretKey:       secretmanagersvc.SecretKeyAPIKey,
		}
		proxyKVPath, pathErr := proxySecretLoc.KVPath()
		if pathErr == nil {
			if err := s.secretClient.DeleteSecretByPath(ctx, proxyKVPath); err != nil {
				return fmt.Errorf("failed to delete proxy API key from KV for proxy %q: %w",
					proxyHandle, err)
			}
		}
	}

	// Step 4b: Remove env vars from Component CR and all ReleaseBindings (internal agents only, best-effort).
	// Must use names from DB (not auto-generated) to handle user-overridden names correctly.
	if !isExternalAgent {
		existingVarNames, varErr := s.loadExistingVarNames(ctx, configUUID)
		if varErr != nil {
			s.logger.Warn("failed to load var names for cleanup, skipping env var removal", "err", varErr)
		} else {
			envConfigTemplates, _ := s.buildEnvironmentVariables(existingConfig.Name, varNamesToOverrides(existingVarNames))
			keysToRemove := make([]string, 0, len(envConfigTemplates))
			for _, t := range envConfigTemplates {
				keysToRemove = append(keysToRemove, t.Name)
			}
			// Remove from Component CR.
			if err := s.ocClient.RemoveComponentEnvironmentVariables(ctx, orgName, projectName, agentName, keysToRemove); err != nil {
				s.logger.Warn("failed to remove env vars from Component CR on config delete", "err", err)
			}
			// Remove from each environment's ReleaseBinding.
			for _, mapping := range mappings {
				env, ok := envIDNameMap[mapping.EnvironmentUUID.String()]
				if !ok {
					continue
				}
				if err := s.ocClient.RemoveReleaseBindingEnvVars(ctx, orgName, projectName, agentName, env, keysToRemove); err != nil {
					s.logger.Warn("failed to remove env vars from ReleaseBinding on config delete",
						"environment", env, "err", err)
				}
			}
		}
	}

	// Step 5: Delete DB records only after all external resources are confirmed cleaned up.
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
// Returns the proxy config, provider API key ID, provider UUID, provider secret KV path, and any error.
// The provider UUID is needed by rollbackProxies to revoke the provider API key on failure.
func (s *agentConfigurationService) buildLLMProxyConfig(
	ctx context.Context,
	config *models.AgentConfiguration,
	envName string,
	envMapping models.EnvModelConfigRequest,
) (*models.LLMProxy, string, string, string, error) {
	sanitizedConfigName := sanitizeForK8sName(config.Name)
	sanitizedEnvName := sanitizeForK8sName(envName)
	proxyName := fmt.Sprintf("%s-%s-proxy", sanitizedConfigName, sanitizedEnvName)
	contextUuid := uuid.New()
	contextPath := fmt.Sprintf("/%s", contextUuid)

	project, err := s.ocClient.GetProject(ctx, config.OrganizationName, config.ProjectName)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("failed to get project from openchoreo: %w", err)
	}

	// Get provider details
	provider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, config.OrganizationName)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("failed to get provider: %w", err)
	}

	apiKeyId := ""
	providerUUID := provider.UUID.String()
	providerSecretPath := ""

	// Parse project UUID
	projectUUID, err := uuid.Parse(project.UUID)
	if err != nil {
		return nil, "", "", "", fmt.Errorf("invalid project UUID from openchoreo: %w", err)
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
	if providerSecurityConfig != nil && providerSecurityConfig.Enabled != nil && *providerSecurityConfig.Enabled {
		// Provider is secured.
		providerApiKeyConfig := providerSecurityConfig.APIKey

		if providerApiKeyConfig != nil && providerApiKeyConfig.Enabled != nil && *providerApiKeyConfig.Enabled {
			// Provider api key security is enabled.
			apiKey, err := s.llmProviderAPIKeyService.CreateAPIKey(ctx, config.OrganizationName, provider.UUID.String(), &models.CreateAPIKeyRequest{
				Name:        proxyName,
				DisplayName: proxyName,
			})
			s.logger.Info("Created provider API key", "providerUUID", provider.UUID.String(), "providerKeyName", proxyName)
			if err != nil {
				return nil, "", "", "", fmt.Errorf("failed to create api key for provider: %w", err)
			}

			apiKeyId = apiKey.KeyID

			// Resolve artifact handle for SecretLocation.EntityName used by KVPath/CreateSecret.
			// EntityName must be non-empty or SecretLocation.KVPath() will fail.
			artifactHandle := ""
			if provider.Artifact != nil {
				artifactHandle = provider.Artifact.Handle
			}
			if artifactHandle == "" {
				return nil, "", "", "", fmt.Errorf("provider %s has no artifact handle; cannot derive SecretLocation.EntityName for KV secret storage", provider.UUID.String())
			}
			kvPath, err := s.storeSecret(ctx, config.OrganizationName, config.ProjectName, config.AgentID, envName, config.Name, artifactHandle, secretmanagersvc.SecretKeyAPIKey, apiKey.APIKey)
			if err != nil {
				// revoke created api key
				if err := s.llmProviderAPIKeyService.RevokeAPIKey(ctx, config.OrganizationName, provider.UUID.String(), proxyName); err != nil {
					s.logger.Error("Failed to revoke provider API key during creation",
						"providerUUID", provider.UUID.String(),
						"providerKeyName", proxyName,
						"error", err,
					)
				}
				return nil, "", "", "", fmt.Errorf("failed to store provider API key in KV: %w", err)
			}
			providerSecretPath = kvPath

			upstreamAuthConfig.Type = utils.StrAsStrPointer(models.AuthTypeAPIKey)
			upstreamAuthConfig.Header = utils.StrAsStrPointer(providerApiKeyConfig.Key)
			upstreamAuthConfig.SecretRef = &kvPath // Store KV path instead of plaintext
			upstreamAuthConfig.Value = nil         // No plaintext in DB
			proxyConfig.Configuration.UpstreamAuth = &upstreamAuthConfig
		}
	}

	return proxyConfig, apiKeyId, providerUUID, providerSecretPath, nil
}

// buildLLMProxyUpdateConfig builds a proxy config for the Update flow (Scenario B).
// It preserves the existing proxy's Name, Context, Security, and ProjectUUID —
// only mutable fields (Provider, UpstreamAuth, Policies) are updated.
func (s *agentConfigurationService) buildLLMProxyUpdateConfig(
	config *models.AgentConfiguration,
	envMapping models.EnvModelConfigRequest,
	existingProxy *models.LLMProxy,
) (*models.LLMProxy, string, error) {
	provider, err := s.llmProviderRepo.GetByHandle(envMapping.ProviderName, config.OrganizationName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get provider: %w", err)
	}
	providerUUID := provider.UUID.String()

	proxyConfig := &models.LLMProxy{
		Description: fmt.Sprintf("LLM proxy for agent %s", config.AgentID),
		ProjectUUID: existingProxy.ProjectUUID,
		Configuration: models.LLMProxyConfig{
			Name:         existingProxy.Configuration.Name,
			Version:      models.DefaultProxyVersion,
			Context:      existingProxy.Configuration.Context,
			Provider:     provider.UUID.String(),
			Security:     existingProxy.Configuration.Security,
			Policies:     envMapping.Configuration.Policies,
			UpstreamAuth: existingProxy.Configuration.UpstreamAuth,
		},
	}

	return proxyConfig, providerUUID, nil
}

func (s *agentConfigurationService) storeSecret(ctx context.Context, orgName, projectName, agentName, envName, configName, entityName, secretKey, secretValue string) (string, error) {
	// Store provider API key in OpenBao KV
	secretLoc := secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     projectName,
		AgentName:       agentName,
		EnvironmentName: envName,
		EntityName:      entityName,
		ConfigName:      configName,
		SecretKey:       secretKey,
	}
	kvPath, err := s.secretClient.CreateSecret(ctx, secretLoc,
		map[string]string{secretKey: secretValue})
	if err != nil {
		return "", fmt.Errorf("failed to store provider API key in KV: %w", err)
	}
	return kvPath, nil
}

// buildEnvironmentVariables generates environment variable templates from config name.
// If overrides are provided, user-supplied names take precedence over auto-generated ones.
// Validates all names using ValidateEnvironmentVariableName.
func (s *agentConfigurationService) buildEnvironmentVariables(configName string, overrides []models.EnvironmentVariableConfig) ([]EnvConfigTemplate, error) {
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

	// Known keys with their secrets flag and auto-generated name
	type keyMeta struct {
		isSecret bool
		autoName string
	}
	knownKeys := map[string]keyMeta{
		"url":    {isSecret: false, autoName: fmt.Sprintf("%s_URL", prefix)},
		"apikey": {isSecret: true, autoName: fmt.Sprintf("%s_API_KEY", prefix)},
	}

	// Build override map from user input; reject unknown keys
	overrideMap := make(map[string]string)
	seen := make(map[string]bool)
	for _, ov := range overrides {
		if _, known := knownKeys[ov.Key]; !known {
			return nil, fmt.Errorf("unknown environment variable key %q: must be one of url, apikey", ov.Key)
		}
		if seen[ov.Key] {
			return nil, fmt.Errorf("duplicate environment variable key %q", ov.Key)
		}
		seen[ov.Key] = true
		overrideMap[ov.Key] = ov.Name
	}

	// Determine final name for each key (override wins, then auto-generate).
	// Iterate in a fixed order so the returned slice is deterministic.
	keyOrder := []string{"url", "apikey"}
	envConfigTemplates := make([]EnvConfigTemplate, 0, len(knownKeys))
	usedNames := make(map[string]string) // name -> key, for duplicate detection
	for _, key := range keyOrder {
		meta := knownKeys[key]
		name := meta.autoName
		if customName, ok := overrideMap[key]; ok {
			name = customName
		}
		if err := utils.ValidateEnvironmentVariableName(name); err != nil {
			return nil, fmt.Errorf("invalid environment variable name %q for key %q: %w", name, key, err)
		}
		if conflictKey, exists := usedNames[name]; exists {
			return nil, fmt.Errorf("duplicate environment variable name %q for keys %q and %q", name, conflictKey, key)
		}
		usedNames[name] = key
		envConfigTemplates = append(envConfigTemplates, EnvConfigTemplate{
			Key:             key,
			Name:            name,
			IsSecret:        meta.isSecret,
			Value:           "",
			SecretReference: "",
		})
	}

	return envConfigTemplates, nil
}

// varNamesToOverrides converts a key→name map to a slice of EnvironmentVariableConfig.
// Used when passing existing DB names as overrides to buildEnvironmentVariables.
func varNamesToOverrides(names map[string]string) []models.EnvironmentVariableConfig {
	if len(names) == 0 {
		return nil
	}
	overrides := make([]models.EnvironmentVariableConfig, 0, len(names))
	for key, name := range names {
		overrides = append(overrides, models.EnvironmentVariableConfig{Key: key, Name: name})
	}
	return overrides
}

// loadExistingVarNames loads the variable key→name mapping from DB for a config.
// Names are config-level (identical across all environments). The first occurrence per key
// is used; a warning is logged if divergence is detected (indicates a data integrity problem).
func (s *agentConfigurationService) loadExistingVarNames(ctx context.Context, configUUID uuid.UUID) (map[string]string, error) {
	vars, err := s.envVariableRepo.ListByConfig(ctx, configUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to load existing variable names: %w", err)
	}
	result := make(map[string]string)
	for _, v := range vars {
		if existing, already := result[v.VariableKey]; already {
			if existing != v.VariableName {
				s.logger.Warn("environment variable name diverged across environments — using first-occurrence value",
					"configUUID", configUUID,
					"key", v.VariableKey,
					"firstValue", existing,
					"divergedValue", v.VariableName,
				)
			}
		} else {
			result[v.VariableKey] = v.VariableName
		}
	}
	return result, nil
}

// rollbackProxies cleans up created proxies, deployments, and API keys on failure
func (s *agentConfigurationService) rollbackProxies(ctx context.Context, resources []rollbackResource, orgName string) {
	s.logger.Warn("Rolling back created proxies and API keys", "count", len(resources))

	// Track unique proxies to delete
	proxyHandles := make(map[string]bool)

	// Clean up each resource
	for _, res := range resources {
		// Delete provider API key from KV
		if res.providerSecretPath != "" {
			if err := s.secretClient.DeleteSecretByPath(ctx, res.providerSecretPath); err != nil {
				s.logger.Error("Failed to delete provider API key from KV during rollback",
					"kvPath", res.providerSecretPath, "error", err)
			}
		}
		// Delete proxy API key from KV
		if res.proxySecretPath != "" {
			if err := s.secretClient.DeleteSecretByPath(ctx, res.proxySecretPath); err != nil {
				s.logger.Error("Failed to delete proxy API key from KV during rollback",
					"kvPath", res.proxySecretPath, "error", err)
			}
		}
		// Delete SecretReference CR (internal agents only)
		if res.secretRefName != "" {
			if err := s.ocClient.DeleteSecretReference(ctx, orgName, res.secretRefName); err != nil {
				s.logger.Warn("failed to delete SecretReference on rollback",
					"name", res.secretRefName, "err", err)
			}
		}

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

		// Undeploy deployment — only if a deployment was actually created.
		if res.proxyHandle != "" && res.deploymentID != uuid.Nil {
			if err := s.llmProxyDeploymentService.DeleteLLMProxyDeployment(res.proxyHandle, res.deploymentID.String(), orgName); err != nil {
				s.logger.Error("Failed to undeploy proxy during rollback",
					"handle", res.proxyHandle,
					"deploymentID", res.deploymentID,
					"error", err,
				)
			}
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

		if res.proxyHandle != "" {
			proxyHandles[res.proxyHandle] = true
		}
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
