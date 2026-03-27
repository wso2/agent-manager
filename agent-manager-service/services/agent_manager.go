// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

	observabilitysvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/observabilitysvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

type AgentManagerService interface {
	ListAgents(ctx context.Context, orgName string, projName string, limit int32, offset int32) ([]*models.AgentResponse, int32, error)
	CreateAgent(ctx context.Context, orgName string, projectName string, req *spec.CreateAgentRequest) error
	UpdateAgentBasicInfo(ctx context.Context, orgName string, projectName string, agentName string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error)
	UpdateAgentBuildParameters(ctx context.Context, orgName string, projectName string, agentName string, req *spec.UpdateAgentBuildParametersRequest) (*models.AgentResponse, error)
	BuildAgent(ctx context.Context, orgName string, projectName string, agentName string, commitId string) (*models.BuildResponse, error)
	DeleteAgent(ctx context.Context, orgName string, projectName string, agentName string) error
	DeployAgent(ctx context.Context, orgName string, projectName string, agentName string, req *spec.DeployAgentRequest) (string, error)
	GetAgent(ctx context.Context, orgName string, projectName string, agentName string) (*models.AgentResponse, error)
	ListAgentBuilds(ctx context.Context, orgName string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error)
	GetBuild(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.BuildDetailsResponse, error)
	GetAgentDeployments(ctx context.Context, orgName string, projectName string, agentName string) ([]*models.DeploymentResponse, error)
	UpdateAgentDeploymentState(ctx context.Context, orgName string, projectName string, agentName string, environment string, state string) error
	GetAgentEndpoints(ctx context.Context, orgName string, projectName string, agentName string, environmentName string) (map[string]models.EndpointsResponse, error)
	GetAgentConfigurations(ctx context.Context, orgName string, projectName string, agentName string, environment string) ([]models.EnvVars, error)
	GetBuildLogs(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.LogsResponse, error)
	GenerateName(ctx context.Context, orgName string, payload spec.ResourceNameRequest) (string, error)
	GetAgentMetrics(ctx context.Context, orgName string, projectName string, agentName string, payload spec.MetricsFilterRequest) (*spec.MetricsResponse, error)
	GetAgentRuntimeLogs(ctx context.Context, orgName string, projectName string, agentName string, payload spec.LogFilterRequest) (*models.LogsResponse, error)
	GetAgentResourceConfigs(ctx context.Context, orgName string, projectName string, agentName string, environment string) (*spec.AgentResourceConfigsResponse, error)
	UpdateAgentResourceConfigs(ctx context.Context, orgName string, projectName string, agentName string, environment string, req *spec.UpdateAgentResourceConfigsRequest) (*spec.AgentResourceConfigsResponse, error)
}

type agentManagerService struct {
	ocClient                  client.OpenChoreoClient
	observabilitySvcClient    observabilitysvc.ObservabilitySvcClient
	secretMgmtClient          secretmanagersvc.SecretManagementClient
	gitRepositoryService      RepositoryService
	tokenManagerService       AgentTokenManagerService
	agentConfigRepo           repositories.AgentConfigRepository
	agentConfigurationService AgentConfigurationService
	logger                    *slog.Logger
}

func NewAgentManagerService(
	OpenChoreoClient client.OpenChoreoClient,
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient,
	secretMgmtClient secretmanagersvc.SecretManagementClient,
	gitRepositoryService RepositoryService,
	tokenManagerService AgentTokenManagerService,
	agentConfigRepo repositories.AgentConfigRepository,
	agentConfigurationService AgentConfigurationService,
	logger *slog.Logger,
) AgentManagerService {
	return &agentManagerService{
		ocClient:                  OpenChoreoClient,
		observabilitySvcClient:    observabilitySvcClient,
		secretMgmtClient:          secretMgmtClient,
		gitRepositoryService:      gitRepositoryService,
		tokenManagerService:       tokenManagerService,
		agentConfigRepo:           agentConfigRepo,
		agentConfigurationService: agentConfigurationService,
		logger:                    logger,
	}
}

// -----------------------------------------------------------------------------
// Error Translation Helpers
// -----------------------------------------------------------------------------

// translateOrgError translates a generic ErrNotFound to ErrOrganizationNotFound
func translateOrgError(err error) error {
	if err != nil && errors.Is(err, utils.ErrNotFound) {
		return utils.ErrOrganizationNotFound
	}
	return err
}

// translateProjectError translates a generic ErrNotFound to ErrProjectNotFound
func translateProjectError(err error) error {
	if err != nil && errors.Is(err, utils.ErrNotFound) {
		return utils.ErrProjectNotFound
	}
	return err
}

// translateAgentError translates a generic ErrNotFound to ErrAgentNotFound
func translateAgentError(err error) error {
	if err != nil && errors.Is(err, utils.ErrNotFound) {
		return utils.ErrAgentNotFound
	}
	return err
}

// translateBuildError translates a generic ErrNotFound to ErrBuildNotFound
func translateBuildError(err error) error {
	if err != nil && errors.Is(err, utils.ErrNotFound) {
		return utils.ErrBuildNotFound
	}
	return err
}

// translateEnvironmentError translates a generic ErrNotFound to ErrEnvironmentNotFound
func translateEnvironmentError(err error) error {
	if err != nil && errors.Is(err, utils.ErrNotFound) {
		return utils.ErrEnvironmentNotFound
	}
	return err
}

// translatePipelineError translates a generic ErrNotFound to ErrDeploymentPipelineNotFound
func translatePipelineError(err error) error {
	if err != nil && errors.Is(err, utils.ErrNotFound) {
		return utils.ErrDeploymentPipelineNotFound
	}
	return err
}

// validateGitSecretExists checks if the specified git secret exists in the organization
func (s *agentManagerService) validateGitSecretExists(ctx context.Context, orgName string, secretRef string) error {
	if secretRef == "" {
		return fmt.Errorf("git secret reference is empty")
	}

	secrets, err := s.ocClient.ListGitSecrets(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to list git secrets for validation", "orgName", orgName, "error", err)
		return fmt.Errorf("failed to validate git secret: %w", err)
	}

	for _, secret := range secrets {
		if secret.Name == secretRef {
			return nil
		}
	}

	s.logger.Error("Git secret not found", "orgName", orgName, "secretRef", secretRef)
	return utils.ErrGitSecretNotFound
}

// Build type constants
const (
	BuildTypeBuildpack = "buildpack"
	BuildTypeDocker    = "docker"
)

// -----------------------------------------------------------------------------
// Mapping Helper Functions
// -----------------------------------------------------------------------------

// mapBuildConfig converts spec.Build to client.BuildConfig
func mapBuildConfig(specBuild *spec.Build) *client.BuildConfig {
	if specBuild == nil {
		return nil
	}

	if specBuild.BuildpackBuild != nil {
		return &client.BuildConfig{
			Type: BuildTypeBuildpack,
			Buildpack: &client.BuildpackConfig{
				Language:        specBuild.BuildpackBuild.Buildpack.Language,
				LanguageVersion: utils.StrPointerAsStr(specBuild.BuildpackBuild.Buildpack.LanguageVersion, ""),
				RunCommand:      utils.StrPointerAsStr(specBuild.BuildpackBuild.Buildpack.RunCommand, ""),
			},
		}
	}

	if specBuild.DockerBuild != nil {
		return &client.BuildConfig{
			Type: BuildTypeDocker,
			Docker: &client.DockerConfig{
				DockerfilePath: specBuild.DockerBuild.Docker.DockerfilePath,
			},
		}
	}

	return nil
}

// mapConfigurationsWithSecrets converts spec.Configurations to client.Configurations
// handling secret env vars by using secretKeyRef pointing to the K8s Secret created by SecretReference
func mapConfigurationsWithSecrets(specConfigs *spec.Configurations, componentName string) *client.Configurations {
	if specConfigs == nil || len(specConfigs.Env) == 0 {
		return nil
	}

	// K8s Secret name created by SecretReference
	secretName := utils.BuildSecretRefName(componentName)

	configs := &client.Configurations{
		Env: make([]client.EnvVar, len(specConfigs.Env)),
	}

	for i, env := range specConfigs.Env {
		if env.GetIsSensitive() {
			// Use secretKeyRef pointing to the K8s Secret
			// Name = K8s Secret name (created by SecretReference)
			// Key = key within the K8s Secret
			configs.Env[i] = client.EnvVar{
				Key: env.Key,
				ValueFrom: &client.EnvVarValueFrom{
					SecretKeyRef: &client.SecretKeyRef{
						Name: secretName, // K8s Secret name (e.g., "component-secrets")
						Key:  env.Key,    // Key within the secret
					},
				},
			}
		} else {
			configs.Env[i] = client.EnvVar{Key: env.Key, Value: env.GetValue()}
		}
	}

	return configs
}

// mapRepository converts spec.RepositoryConfig to client.RepositoryConfig
func mapRepository(specRepo *spec.RepositoryConfig) *client.RepositoryConfig {
	if specRepo == nil {
		return nil
	}
	repo := &client.RepositoryConfig{
		URL:     specRepo.Url,
		Branch:  specRepo.Branch,
		AppPath: specRepo.AppPath,
	}
	if specRepo.SecretRef.Get() != nil {
		repo.SecretRef = *specRepo.SecretRef.Get()
	}
	return repo
}

// mapInputInterface converts spec.InputInterface to client.InputInterfaceConfig
func mapInputInterface(specInterface *spec.InputInterface) *client.InputInterfaceConfig {
	if specInterface == nil {
		return nil
	}

	config := &client.InputInterfaceConfig{
		Type: specInterface.Type,
	}

	if specInterface.Port != nil {
		config.Port = *specInterface.Port
	}
	if specInterface.BasePath != nil {
		config.BasePath = *specInterface.BasePath
	}
	if specInterface.Schema != nil {
		config.SchemaPath = specInterface.Schema.Path
	}

	return config
}

// buildCreateTraitRequests collects all traits needed during agent creation into a single
// list so they can be attached in one GET-UPDATE cycle, avoiding resource version conflicts.
func (s *agentManagerService) buildCreateTraitRequests(ctx context.Context, orgName, projectName string, req *spec.CreateAgentRequest) ([]client.TraitRequest, error) {
	var traits []client.TraitRequest

	// Determine instrumentation trait
	autoInstrumentation := req.Configurations == nil || req.Configurations.EnableAutoInstrumentation == nil || *req.Configurations.EnableAutoInstrumentation
	isAPIAgent := req.AgentType.Type == string(utils.AgentTypeAPI)
	isPythonBuildpack := req.Build != nil && req.Build.BuildpackBuild != nil && req.Build.BuildpackBuild.Buildpack.Language == string(utils.LanguagePython)
	isDocker := req.Build != nil && req.Build.DockerBuild != nil

	// Only generate API key when an instrumentation trait is needed
	needsOTEL := isAPIAgent && autoInstrumentation && isPythonBuildpack
	needsEnvInjection := isAPIAgent && (isDocker || (!autoInstrumentation && isPythonBuildpack))

	if needsOTEL || needsEnvInjection {
		apiKey, err := s.generateAgentAPIKey(ctx, orgName, projectName, req.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to generate agent API key: %w", err)
		}

		if needsOTEL {
			traits = append(traits, client.TraitRequest{TraitType: client.TraitOTELInstrumentation, Opts: []client.TraitOption{client.WithAgentApiKey(apiKey)}})
		} else {
			traits = append(traits, client.TraitRequest{TraitType: client.TraitEnvInjection, Opts: []client.TraitOption{client.WithAgentApiKey(apiKey)}})
		}
	}

	// API configuration trait (only for chat and custom API agents)
	if isAPIAgent {
		var traitOpts []client.TraitOption
		if req.InputInterface != nil && req.InputInterface.HasPort() && req.InputInterface.GetPort() > 0 {
			traitOpts = append(traitOpts, client.WithUpstreamPort(req.InputInterface.GetPort()))
		} else {
			traitOpts = append(traitOpts, client.WithUpstreamPort(config.GetConfig().DefaultChatAPI.DefaultHTTPPort))
		}
		if req.InputInterface != nil && req.InputInterface.HasBasePath() {
			traitOpts = append(traitOpts, client.WithUpstreamBasePath(req.InputInterface.GetBasePath()))
		} else {
			traitOpts = append(traitOpts, client.WithUpstreamBasePath(config.GetConfig().DefaultChatAPI.DefaultBasePath))
		}
		traits = append(traits, client.TraitRequest{TraitType: client.TraitAPIManagement, Opts: traitOpts})
	}

	return traits, nil
}

// attachOTELInstrumentationTrait attaches OTEL instrumentation trait to the agent
// The trait handles injection of OTEL configuration including the agent API key
func (s *agentManagerService) attachOTELInstrumentationTrait(ctx context.Context, orgName, projectName, agentName string) error {
	// Generate agent API key for the trait parameters
	apiKey, err := s.generateAgentAPIKey(ctx, orgName, projectName, agentName)
	if err != nil {
		return fmt.Errorf("failed to generate agent API key: %w", err)
	}

	if err := s.ocClient.AttachTraits(ctx, orgName, projectName, agentName, []client.TraitRequest{
		{TraitType: client.TraitOTELInstrumentation, Opts: []client.TraitOption{client.WithAgentApiKey(apiKey)}},
	}); err != nil {
		return fmt.Errorf("error attaching OTEL instrumentation trait: %w", err)
	}

	s.logger.Info("Enabled instrumentation for buildpack agent", "agentName", agentName)
	return nil
}

// detachOTELInstrumentationTrait removes the OTEL instrumentation trait from the agent
func (s *agentManagerService) detachOTELInstrumentationTrait(ctx context.Context, orgName, projectName, agentName string) error {
	if err := s.ocClient.DetachTrait(ctx, orgName, projectName, agentName, client.TraitOTELInstrumentation); err != nil {
		return fmt.Errorf("error detaching OTEL instrumentation trait: %w", err)
	}

	s.logger.Info("Disabled instrumentation for buildpack agent", "agentName", agentName)
	return nil
}

// attachEnvInjectionTrait attaches the env injection trait to inject AMP_OTEL_ENDPOINT
// and AMP_AGENT_API_KEY environment variables. Used for Docker builds and buildpack
// builds when auto-instrumentation is disabled.
func (s *agentManagerService) attachEnvInjectionTrait(ctx context.Context, orgName, projectName, agentName string) error {
	// Generate agent API key for the trait parameters
	apiKey, err := s.generateAgentAPIKey(ctx, orgName, projectName, agentName)
	if err != nil {
		return fmt.Errorf("failed to generate agent API key: %w", err)
	}

	if err := s.ocClient.AttachTraits(ctx, orgName, projectName, agentName, []client.TraitRequest{
		{TraitType: client.TraitEnvInjection, Opts: []client.TraitOption{client.WithAgentApiKey(apiKey)}},
	}); err != nil {
		return fmt.Errorf("error attaching env injection trait: %w", err)
	}

	s.logger.Info("Attached env injection trait", "agentName", agentName)
	return nil
}

// detachEnvInjectionTrait removes the env injection trait from the agent
func (s *agentManagerService) detachEnvInjectionTrait(ctx context.Context, orgName, projectName, agentName string) error {
	if err := s.ocClient.DetachTrait(ctx, orgName, projectName, agentName, client.TraitEnvInjection); err != nil {
		return fmt.Errorf("error detaching env injection trait: %w", err)
	}

	s.logger.Info("Detached env injection trait", "agentName", agentName)
	return nil
}

// persistInstrumentationConfig saves the instrumentation config to the database
func (s *agentManagerService) persistInstrumentationConfig(ctx context.Context, orgName, projectName, agentName string, enableAutoInstrumentation bool) {
	// Get the first/lowest environment
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		s.logger.Warn("Failed to get deployment pipeline for config persistence", "agentName", agentName, "error", err)
		return
	}

	lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
	if lowestEnv == "" {
		s.logger.Warn("No environment found for config persistence", "agentName", agentName)
		return
	}

	targetEnv, err := s.ocClient.GetEnvironment(ctx, orgName, lowestEnv)
	if err != nil {
		s.logger.Warn("Failed to get environment details for config persistence", "agentName", agentName, "environment", lowestEnv, "error", err)
		return
	}

	agentConfig := &models.AgentConfig{
		OrgName:                   orgName,
		ProjectName:               projectName,
		AgentName:                 agentName,
		EnvironmentName:           targetEnv.Name,
		EnableAutoInstrumentation: enableAutoInstrumentation,
	}

	if err := s.agentConfigRepo.Upsert(agentConfig); err != nil {
		s.logger.Warn("Failed to persist instrumentation config to database", "agentName", agentName, "error", err)
	} else {
		s.logger.Debug("Persisted instrumentation config to database", "agentName", agentName, "environment", lowestEnv, "enableAutoInstrumentation", enableAutoInstrumentation)
	}
}

// generateAgentAPIKey generates an agent API key (JWT token) for the agent
// This is a common utility used by both buildpack and docker agent instrumentation
func (s *agentManagerService) generateAgentAPIKey(ctx context.Context, orgName, projectName, agentName string) (string, error) {
	// Get the deployment pipeline to find the first environment
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get deployment pipeline for token generation", "projectName", projectName, "error", err)
		return "", translatePipelineError(err)
	}
	firstEnvName := findLowestEnvironment(pipeline.PromotionPaths)

	// Generate agent API key using token manager service with 1 year expiry
	tokenReq := GenerateTokenRequest{
		OrgName:     orgName,
		ProjectName: projectName,
		AgentName:   agentName,
		Environment: firstEnvName,
		ExpiresIn:   "8760h", // 1 year (365 days * 24 hours)
	}
	tokenResp, err := s.tokenManagerService.GenerateToken(ctx, tokenReq)
	if err != nil {
		s.logger.Error("Failed to generate agent API key", "agentName", agentName, "error", err)
		return "", fmt.Errorf("failed to generate agent API key: %w", err)
	}

	s.logger.Debug("Generated agent API key", "agentName", agentName)
	return tokenResp.Token, nil
}

// generateTracingEnvVars generates tracing-related environment variables (OTEL endpoint and
// agent API key) for the named agent. Returns the env vars without persisting them.
func (s *agentManagerService) generateTracingEnvVars(ctx context.Context, orgName, projectName, agentName string) ([]client.EnvVar, error) {
	s.logger.Debug("Generating tracing environment variables", "agentName", agentName)

	// Generate agent API key
	apiKey, err := s.generateAgentAPIKey(ctx, orgName, projectName, agentName)
	if err != nil {
		return nil, err
	}

	// Get OTEL exporter endpoint from config
	cfg := config.GetConfig()
	otelEndpoint := cfg.OTEL.ExporterEndpoint

	// Prepare tracing environment variables
	tracingEnvVars := []client.EnvVar{
		{
			Key:   client.EnvVarOTELEndpoint,
			Value: otelEndpoint,
		},
		{
			Key:   client.EnvVarAgentAPIKey,
			Value: apiKey,
		},
	}

	return tracingEnvVars, nil
}

// injectTracingEnvVarsByName injects tracing-related environment variables (OTEL endpoint and
// agent API key) for the named agent into the Component CR. This is used during agent creation
// for docker and Python buildpack agents (the latter when auto-instrumentation is disabled).
func (s *agentManagerService) injectTracingEnvVarsByName(ctx context.Context, orgName, projectName, agentName string) error {
	s.logger.Debug("Injecting tracing environment variables", "agentName", agentName)

	tracingEnvVars, err := s.generateTracingEnvVars(ctx, orgName, projectName, agentName)
	if err != nil {
		return err
	}

	// Update component configurations with tracing environment variables (for persistence)
	if err := s.updateComponentEnvVars(ctx, orgName, projectName, agentName, tracingEnvVars); err != nil {
		s.logger.Error("Failed to update component with tracing env vars", "agentName", agentName, "error", err)
		return fmt.Errorf("failed to update component env vars: %w", err)
	}

	s.logger.Info("Injected tracing environment variables",
		"agentName", agentName,
		"envVarCount", len(tracingEnvVars),
	)

	return nil
}

// updateComponentEnvVars updates the component's workflow parameters with new environment variables
func (s *agentManagerService) updateComponentEnvVars(ctx context.Context, orgName, projectName, componentName string, newEnvVars []client.EnvVar) error {
	s.logger.Debug("Updating component environment variables", "componentName", componentName, "newEnvCount", len(newEnvVars))

	if err := s.ocClient.UpdateComponentEnvVars(ctx, orgName, projectName, componentName, newEnvVars); err != nil {
		s.logger.Error("Failed to update component environment variables", "componentName", componentName, "error", err)
		return fmt.Errorf("failed to update component environment variables: %w", err)
	}

	s.logger.Info("Successfully updated component environment variables",
		"componentName", componentName,
		"envVarCount", len(newEnvVars),
	)

	return nil
}

func (s *agentManagerService) GetAgent(ctx context.Context, orgName string, projectName string, agentName string) (*models.AgentResponse, error) {
	s.logger.Info("Getting agent", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	// Populate enableAutoInstrumentation from database
	// Get the first/lowest environment to read the config
	pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if pipelineErr == nil && len(pipeline.PromotionPaths) > 0 {
		lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
		if lowestEnv != "" {
			agentConfig, configErr := s.agentConfigRepo.Get(orgName, projectName, agentName, lowestEnv)
			if errors.Is(configErr, repositories.ErrAgentConfigNotFound) {
				// No config in DB - default to true for display purposes
				defaultEnabled := true
				agent.Configurations = &models.Configurations{
					EnableAutoInstrumentation: &defaultEnabled,
				}
				s.logger.Debug("No agent config in database, defaulting to enabled", "agentName", agentName)
			} else if configErr != nil {
				s.logger.Warn("Failed to read agent config from database", "agentName", agentName, "environment", lowestEnv, "error", configErr)
			} else {
				agent.Configurations = &models.Configurations{
					EnableAutoInstrumentation: &agentConfig.EnableAutoInstrumentation,
				}
				s.logger.Debug("Populated enableAutoInstrumentation from database", "agentName", agentName, "environment", lowestEnv, "enableAutoInstrumentation", agentConfig.EnableAutoInstrumentation)
			}
		}
	}

	s.logger.Info("Fetched agent successfully from oc", "agentName", agent.Name, "orgName", orgName, "projectName", projectName, "provisioningType", agent.Provisioning.Type)
	return agent, nil
}

func (s *agentManagerService) ListAgents(ctx context.Context, orgName string, projName string, limit int32, offset int32) ([]*models.AgentResponse, int32, error) {
	s.logger.Info("Listing agents", "orgName", orgName, "projectName", projName, "limit", limit, "offset", offset)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, 0, translateOrgError(err)
	}

	// Fetch all agent components
	agents, err := s.ocClient.ListComponents(ctx, orgName, projName)
	if err != nil {
		s.logger.Error("Failed to list agents from repository", "orgName", orgName, "projectName", projName, "error", err)
		return nil, 0, fmt.Errorf("failed to list agents: %w", err)
	}

	// Calculate total count
	total := int32(len(agents))

	// Apply pagination
	var paginatedAgents []*models.AgentResponse
	if offset >= total {
		// If offset is beyond available data, return empty slice
		paginatedAgents = []*models.AgentResponse{}
	} else {
		endIndex := offset + limit
		if endIndex > total {
			endIndex = total
		}
		paginatedAgents = agents[offset:endIndex]
	}
	s.logger.Info("Listed agents successfully", "orgName", orgName, "projName", projName, "totalAgents", total, "returnedAgents", len(paginatedAgents))
	return paginatedAgents, total, nil
}

func (s *agentManagerService) CreateAgent(ctx context.Context, orgName string, projectName string, req *spec.CreateAgentRequest) error {
	s.logger.Info("Creating agent", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "provisioningType", req.Provisioning.Type)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return translateOrgError(err)
	}

	// Validate git secret exists if specified
	if req.Provisioning.Repository != nil && req.Provisioning.Repository.SecretRef.Get() != nil {
		if err := s.validateGitSecretExists(ctx, orgName, req.Provisioning.Repository.GetSecretRef()); err != nil {
			return err
		}
	}

	// Get the first/lowest environment for secret path
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get deployment pipeline", "projectName", projectName, "error", err)
		return translatePipelineError(err)
	}
	firstEnv := findLowestEnvironment(pipeline.PromotionPaths)
	if firstEnv == "" {
		s.logger.Error("No environment found in deployment pipeline", "projectName", projectName)
		return fmt.Errorf("no environment found in deployment pipeline")
	}

	// Build secret location for OpenBao KV path
	secretLocation := secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     projectName,
		EnvironmentName: firstEnv,
		EntityName:      req.Name,
	}

	// Check if there are secret env vars that need to be handled
	hasSecrets := false
	if req.Configurations != nil && len(req.Configurations.Env) > 0 {
		for _, env := range req.Configurations.Env {
			if env.GetIsSensitive() {
				hasSecrets = true
				break
			}
		}
	}

	// Create component request
	createAgentReq := s.toCreateAgentRequestWithSecrets(req)
	if err := s.ocClient.CreateComponent(ctx, orgName, projectName, createAgentReq); err != nil {
		s.logger.Error("Failed to create agent component", "agentName", req.Name, "error", err)
		return err
	}

	if hasSecrets {
		if err := s.saveSecretsAndCreateReference(ctx, secretLocation, req.Configurations.Env); err != nil {
			s.logger.Error("Failed to save secrets and create SecretReference for agent", "agentName", req.Name, "error", err)
			// Rollback - delete the created agent and cleanup any partially saved secrets
			s.cleanupSecretsOnRollback(ctx, secretLocation)
			if errDeletion := s.ocClient.DeleteComponent(ctx, orgName, projectName, req.Name); errDeletion != nil {
				s.logger.Error("Failed to rollback agent creation after secret processing failure", "agentName", req.Name, "error", errDeletion)
			}
			return err
		}
	}

	// For internal agents, enable instrumentation (if enabled) and trigger initial build
	if req.Provisioning.Type == string(utils.InternalAgent) {
		s.logger.Debug("Component created successfully", "agentName", req.Name)

		// Build all traits to attach in a single GET-UPDATE cycle to avoid resource version conflicts
		traitRequests, err := s.buildCreateTraitRequests(ctx, orgName, projectName, req)
		if err != nil {
			s.logger.Error("Failed to build trait requests", "agentName", req.Name, "error", err)
			if hasSecrets {
				s.cleanupSecretsOnRollback(ctx, secretLocation)
			}
			if errDeletion := s.ocClient.DeleteComponent(ctx, orgName, projectName, req.Name); errDeletion != nil {
				s.logger.Error("Failed to rollback agent creation after trait build failure", "agentName", req.Name, "error", errDeletion)
			}
			return err
		}

		if len(traitRequests) > 0 {
			if err := s.ocClient.AttachTraits(ctx, orgName, projectName, req.Name, traitRequests); err != nil {
				s.logger.Error("Failed to attach traits", "agentName", req.Name, "error", err)
				if hasSecrets {
					s.cleanupSecretsOnRollback(ctx, secretLocation)
				}
				if errDeletion := s.ocClient.DeleteComponent(ctx, orgName, projectName, req.Name); errDeletion != nil {
					s.logger.Error("Failed to rollback agent creation after trait attachment failure", "agentName", req.Name, "error", errDeletion)
				}
				return err
			}
			s.logger.Info("Attached traits", "agentName", req.Name, "count", len(traitRequests))
		}

		// Trigger initial build (non-fatal - build can be triggered manually later)
		if err := s.triggerInitialBuild(ctx, orgName, projectName, req); err != nil {
			s.logger.Warn("Failed to trigger initial build for agent, build can be triggered manually", "agentName", req.Name, "error", err)
		} else {
			s.logger.Debug("Triggered initial build for agent", "agentName", req.Name)
		}

		// Persist initial instrumentation config to database
		enableAutoInstrumentation := true // Default
		if req.Configurations != nil && req.Configurations.EnableAutoInstrumentation != nil {
			enableAutoInstrumentation = *req.Configurations.EnableAutoInstrumentation
		}
		s.persistInstrumentationConfig(ctx, orgName, projectName, req.Name, enableAutoInstrumentation)
	}

	s.logger.Info("Agent created successfully", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "provisioningType", req.Provisioning.Type)
	return nil
}

func (s *agentManagerService) triggerInitialBuild(ctx context.Context, orgName, projectName string, req *spec.CreateAgentRequest) error {
	// Get the latest commit from the repository
	commitId := ""
	if req.Provisioning.Repository != nil {
		repoURL := req.Provisioning.Repository.Url
		branch := req.Provisioning.Repository.Branch
		owner, repo := utils.ParseGitHubURL(repoURL)
		if owner != "" && repo != "" {
			latestCommit, err := s.gitRepositoryService.GetLatestCommit(ctx, owner, repo, branch)
			if err != nil {
				s.logger.Warn("Failed to get latest commit, will use empty commit", "repoURL", repoURL, "branch", branch, "error", err)
			} else {
				commitId = latestCommit
				s.logger.Debug("Got latest commit for build", "commitId", commitId, "branch", branch)
			}
		}
	}
	// Trigger build in OpenChoreo with the latest commit
	build, err := s.ocClient.TriggerBuild(ctx, orgName, projectName, req.Name, commitId)
	if err != nil {
		return fmt.Errorf("failed to trigger initial build: agentName %s, error: %w", req.Name, err)
	}
	s.logger.Info("Agent component created and build triggered successfully", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "buildName", build.Name, "commitId", commitId)
	return nil
}

// toCreateAgentRequestWithSecrets creates a component request, handling secrets by using secretKeyRef
func (s *agentManagerService) toCreateAgentRequestWithSecrets(req *spec.CreateAgentRequest) client.CreateComponentRequest {
	result := client.CreateComponentRequest{
		Name:             req.Name,
		DisplayName:      req.DisplayName,
		Description:      utils.StrPointerAsStr(req.Description, ""),
		ProvisioningType: client.ProvisioningType(req.Provisioning.Type),
		AgentType: client.AgentTypeConfig{
			Type: req.AgentType.Type,
		},
		Repository:     mapRepository(req.Provisioning.Repository),
		Build:          mapBuildConfig(req.Build),
		InputInterface: mapInputInterface(req.InputInterface),
	}

	result.Configurations = mapConfigurationsWithSecrets(req.Configurations, req.Name)

	if req.Provisioning.Type == string(utils.InternalAgent) {
		result.AgentType.SubType = utils.StrPointerAsStr(req.AgentType.SubType, "")
	}

	return result
}

// saveSecretsAndCreateReference handles storing secrets in OpenBao and creating SecretReference CR
func (s *agentManagerService) saveSecretsAndCreateReference(
	ctx context.Context,
	location secretmanagersvc.SecretLocation,
	envVars []spec.EnvironmentVariable,
) error {
	if s.secretMgmtClient == nil {
		return fmt.Errorf("secret management is not initialized but secret env vars were provided")
	}

	// Collect secret data and keys
	secretData := make(map[string]string)
	secretKeys := make([]string, 0)
	for _, env := range envVars {
		if env.GetIsSensitive() {
			secretData[env.Key] = env.GetValue()
			secretKeys = append(secretKeys, env.Key)
		}
	}

	if len(secretData) == 0 {
		return nil
	}

	// Store secrets in KV via secretmanagersvc client
	kvPath, err := location.KVPath()
	if err != nil {
		return fmt.Errorf("invalid secret location: %w", err)
	}
	s.logger.Debug("Storing secrets in KV", "kvPath", kvPath, "secretCount", len(secretData))
	_, createErr := s.secretMgmtClient.CreateSecret(ctx, location, secretData)
	if createErr != nil {
		if errors.Is(createErr, secretmanagersvc.ErrNotManaged) {
			return fmt.Errorf("secret path %q is already owned by another system and cannot be overwritten; manual cleanup may be required: %w", kvPath, utils.ErrSecretPathConflict)
		}
		return fmt.Errorf("failed to store secrets in OpenBao: %w", createErr)
	}

	// Create SecretReference CR via OpenChoreo /apply API
	secretRefName := utils.BuildSecretRefName(location.EntityName)
	s.logger.Debug("Creating SecretReference CR", "name", secretRefName, "namespace", location.OrgName, "kvPath", kvPath)
	if _, err := s.ocClient.CreateSecretReference(ctx, location.OrgName, client.CreateSecretReferenceRequest{
		Namespace:       location.OrgName,
		Name:            secretRefName,
		ProjectName:     location.ProjectName,
		ComponentName:   location.EntityName,
		KVPath:          kvPath,
		SecretKeys:      secretKeys,
		RefreshInterval: config.GetConfig().SecretManager.RefreshInterval,
	}); err != nil {
		return fmt.Errorf("failed to create SecretReference: %w", err)
	}

	s.logger.Info("Secrets stored and SecretReference created", "kvPath", kvPath, "secretCount", len(secretData))
	return nil
}

// cleanupSecretsOnRollback removes secrets from KV and deletes SecretReference CR during rollback.
// This is a best-effort cleanup - errors are logged but not returned since we're already handling a failure.
func (s *agentManagerService) cleanupSecretsOnRollback(ctx context.Context, location secretmanagersvc.SecretLocation) {
	secretRefName := utils.BuildSecretRefName(location.EntityName)

	// Delete secrets from KV
	if s.secretMgmtClient != nil {
		kvPathForLog, _ := location.KVPath()
		if err := s.secretMgmtClient.DeleteSecret(ctx, location); err != nil {
			s.logger.Warn("Failed to cleanup secrets from KV during rollback", "kvPath", kvPathForLog, "error", err)
		} else {
			s.logger.Debug("Cleaned up secrets from KV during rollback", "kvPath", kvPathForLog)
		}
	}

	// Delete SecretReference CR
	if err := s.ocClient.DeleteSecretReference(ctx, location.OrgName, secretRefName); err != nil {
		s.logger.Warn("Failed to cleanup SecretReference during rollback", "name", secretRefName, "error", err)
	} else {
		s.logger.Debug("Cleaned up SecretReference during rollback", "name", secretRefName)
	}
}

func (s *agentManagerService) UpdateAgentBasicInfo(ctx context.Context, orgName string, projectName string, agentName string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error) {
	s.logger.Info("Updating agent basic info", "agentName", agentName, "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, translateProjectError(err)
	}

	// Fetch existing agent to validate it exists
	_, err = s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch existing agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}
	// Update agent basic info in OpenChoreo
	updateReq := client.UpdateComponentBasicInfoRequest{
		DisplayName: req.DisplayName,
		Description: req.Description,
	}
	if err := s.ocClient.UpdateComponentBasicInfo(ctx, orgName, projectName, agentName, updateReq); err != nil {
		s.logger.Error("Failed to update agent meta data in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to update agent basic info: %w", err)
	}

	// Fetch agent to return current state
	updatedAgent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	s.logger.Info("Agent basic info update called", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	return updatedAgent, nil
}

func (s *agentManagerService) UpdateAgentBuildParameters(ctx context.Context, orgName string, projectName string, agentName string, req *spec.UpdateAgentBuildParametersRequest) (*models.AgentResponse, error) {
	s.logger.Info("Updating agent build parameters", "agentName", agentName, "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, translateProjectError(err)
	}

	// Validate git secret exists if specified
	if req.Provisioning.Repository != nil && req.Provisioning.Repository.SecretRef.Get() != nil {
		if err := s.validateGitSecretExists(ctx, orgName, req.Provisioning.Repository.GetSecretRef()); err != nil {
			return nil, err
		}
	}

	// Fetch existing agent to validate immutable fields
	existingAgent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch existing agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	// Check immutable fields - agentType cannot be changed if provided
	if req.AgentType.Type != existingAgent.Type.Type {
		s.logger.Error("Cannot change agent type", "existingType", existingAgent.Type.Type, "requestedType", req.AgentType.Type)
		return nil, fmt.Errorf("%w: agent type cannot be changed", utils.ErrImmutableFieldChange)
	}

	// Check immutable fields - provisioning type cannot be changed if provided
	if req.Provisioning.Type != existingAgent.Provisioning.Type {
		s.logger.Error("Cannot change provisioning type", "existingType", existingAgent.Provisioning.Type, "requestedType", req.Provisioning.Type)
		return nil, fmt.Errorf("%w: provisioning type cannot be changed", utils.ErrImmutableFieldChange)
	}

	// Update agent build parameters in OpenChoreo
	updateReq := buildUpdateBuildParametersRequest(req)
	if err := s.ocClient.UpdateComponentBuildParameters(ctx, orgName, projectName, agentName, updateReq); err != nil {
		s.logger.Error("Failed to update agent build parameters in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to update agent build parameters: %w", err)
	}

	// Fetch agent to return current state
	updatedAgent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	s.logger.Info("Agent build parameters updated successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	return updatedAgent, nil
}

func (s *agentManagerService) GetAgentResourceConfigs(ctx context.Context, orgName string, projectName string, agentName string, environment string) (*spec.AgentResourceConfigsResponse, error) {
	s.logger.Info("Getting agent resource configurations", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, translateProjectError(err)
	}

	// Validate agent exists
	_, err = s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	_, err = s.ocClient.GetEnvironment(ctx, orgName, environment)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environment, "orgName", orgName, "error", err)
		return nil, translateEnvironmentError(err)
	}

	// Fetch resource configurations from OpenChoreo
	configs, err := s.ocClient.GetEnvResourceConfigs(ctx, orgName, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch agent resource configurations", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment, "error", err)
		return nil, fmt.Errorf("failed to get agent resource configurations: %w", err)
	}

	// Convert client response to spec response
	response := buildResourceConfigsResponse(configs)

	s.logger.Info("Fetched agent resource configurations successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment)
	return response, nil
}

func (s *agentManagerService) UpdateAgentResourceConfigs(ctx context.Context, orgName string, projectName string, agentName string, environment string, req *spec.UpdateAgentResourceConfigsRequest) (*spec.AgentResourceConfigsResponse, error) {
	s.logger.Info("Updating agent resource configurations", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, translateProjectError(err)
	}

	// Fetch existing agent to validate it exists
	_, err = s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch existing agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	// Validate environment (required)
	_, err = s.ocClient.GetEnvironment(ctx, orgName, environment)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environment, "orgName", orgName, "error", err)
		return nil, translateEnvironmentError(err)
	}

	// Update agent resource configurations in OpenChoreo
	updateReq := buildUpdateResourceConfigsRequest(req)
	if err := s.ocClient.UpdateEnvResourceConfigs(ctx, orgName, projectName, agentName, environment, updateReq); err != nil {
		s.logger.Error("Failed to update agent resource configurations in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment, "error", err)
		return nil, fmt.Errorf("failed to update agent resource configurations: %w", err)
	}

	// Fetch updated resource configurations to return
	updatedConfigs, err := s.GetAgentResourceConfigs(ctx, orgName, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch updated resource configurations", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment, "error", err)
		return nil, fmt.Errorf("failed to get agent resource configurations: %w", err)
	}

	s.logger.Info("Agent resource configurations updated successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment)
	return updatedConfigs, nil
}

// buildUpdateResourceConfigsRequest converts spec request to client request
func buildUpdateResourceConfigsRequest(req *spec.UpdateAgentResourceConfigsRequest) client.UpdateComponentResourceConfigsRequest {
	updateReq := client.UpdateComponentResourceConfigsRequest{}

	// Check if autoscaling is enabled
	autoscalingEnabled := req.AutoScaling.Enabled != nil && *req.AutoScaling.Enabled

	// Only set replicas when autoscaling is disabled (static scaling)
	// When autoscaling is enabled, HPA manages replicas
	if !autoscalingEnabled {
		updateReq.Replicas = &req.Replicas
	}

	updateReq.Resources = &client.ResourceConfig{}

	if req.Resources.Requests != nil {
		updateReq.Resources.Requests = &client.ResourceRequests{
			CPU:    utils.StrPointerAsStr(req.Resources.Requests.Cpu, ""),
			Memory: utils.StrPointerAsStr(req.Resources.Requests.Memory, ""),
		}
	}

	if req.Resources.Limits != nil {
		updateReq.Resources.Limits = &client.ResourceLimits{
			CPU:    utils.StrPointerAsStr(req.Resources.Limits.Cpu, ""),
			Memory: utils.StrPointerAsStr(req.Resources.Limits.Memory, ""),
		}
	}

	updateReq.AutoScaling = convertSpecAutoScalingConfigToClient(&req.AutoScaling)

	return updateReq
}

// convertSpecAutoScalingConfigToClient converts spec AutoScalingConfig to client AutoScalingConfig
func convertSpecAutoScalingConfigToClient(specConfig *spec.AutoScalingConfig) *client.AutoScalingConfig {
	if specConfig == nil {
		return nil
	}
	return &client.AutoScalingConfig{
		Enabled:     specConfig.Enabled,
		MinReplicas: specConfig.MinReplicas,
		MaxReplicas: specConfig.MaxReplicas,
	}
}

// buildResourceConfigsResponse converts client response to spec response
func buildResourceConfigsResponse(clientResp *client.ComponentResourceConfigsResponse) *spec.AgentResourceConfigsResponse {
	response := &spec.AgentResourceConfigsResponse{}

	if clientResp.Replicas != nil {
		response.Replicas = clientResp.Replicas
	}

	if clientResp.Resources != nil {
		response.Resources = convertClientResourceConfigToSpec(clientResp.Resources)
	}

	if clientResp.AutoScaling != nil {
		response.AutoScaling = convertClientAutoScalingConfigToSpec(clientResp.AutoScaling)
	}

	return response
}

// convertClientAutoScalingConfigToSpec converts client AutoScalingConfig to spec AutoScalingConfig
func convertClientAutoScalingConfigToSpec(clientConfig *client.AutoScalingConfig) *spec.AutoScalingConfig {
	if clientConfig == nil {
		return nil
	}
	return &spec.AutoScalingConfig{
		Enabled:     clientConfig.Enabled,
		MinReplicas: clientConfig.MinReplicas,
		MaxReplicas: clientConfig.MaxReplicas,
	}
}

// convertClientResourceConfigToSpec converts client ResourceConfig to spec ResourceConfig
func convertClientResourceConfigToSpec(clientConfig *client.ResourceConfig) *spec.ResourceConfig {
	if clientConfig == nil {
		return nil
	}

	specConfig := &spec.ResourceConfig{}

	if clientConfig.Requests != nil {
		requests := &spec.ResourceRequests{}
		if clientConfig.Requests.CPU != "" {
			cpu := clientConfig.Requests.CPU
			requests.Cpu = &cpu
		}
		if clientConfig.Requests.Memory != "" {
			memory := clientConfig.Requests.Memory
			requests.Memory = &memory
		}
		specConfig.Requests = requests
	}

	if clientConfig.Limits != nil {
		limits := &spec.ResourceLimits{}
		if clientConfig.Limits.CPU != "" {
			cpu := clientConfig.Limits.CPU
			limits.Cpu = &cpu
		}
		if clientConfig.Limits.Memory != "" {
			memory := clientConfig.Limits.Memory
			limits.Memory = &memory
		}
		specConfig.Limits = limits
	}

	return specConfig
}

// buildUpdateBuildParametersRequest converts spec request to client request
func buildUpdateBuildParametersRequest(req *spec.UpdateAgentBuildParametersRequest) client.UpdateComponentBuildParametersRequest {
	subType := ""
	if req.AgentType.SubType != nil {
		subType = *req.AgentType.SubType
	}
	return client.UpdateComponentBuildParametersRequest{
		Repository:     mapRepository(req.Provisioning.Repository),
		Build:          mapBuildConfig(&req.Build),
		InputInterface: mapInputInterface(&req.InputInterface),
		AgentType: client.AgentTypeConfig{
			Type:    req.AgentType.Type,
			SubType: subType,
		},
	}
}

func (s *agentManagerService) GenerateName(ctx context.Context, orgName string, payload spec.ResourceNameRequest) (string, error) {
	s.logger.Info("Generating resource name", "resourceType", payload.ResourceType, "displayName", payload.DisplayName, "orgName", orgName)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return "", translateOrgError(err)
	}

	// Generate candidate name from display name
	candidateName := utils.GenerateCandidateName(payload.DisplayName)
	s.logger.Debug("Generated candidate name", "candidateName", candidateName, "displayName", payload.DisplayName)

	if payload.ResourceType == string(utils.ResourceTypeAgent) {
		projectName := utils.StrPointerAsStr(payload.ProjectName, "")
		// Validates the project name by checking its existence
		project, err := s.ocClient.GetProject(ctx, orgName, projectName)
		if err != nil {
			s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
			return "", translateProjectError(err)
		}

		// Check if candidate name is available
		exists, err := s.ocClient.ComponentExists(ctx, org.Name, project.Name, candidateName, false)
		if err != nil {
			return "", fmt.Errorf("failed to check agent existence: %w", err)
		}
		if !exists {
			return candidateName, nil
		}

		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueAgentName(ctx, org.Name, project.Name, candidateName)
		if err != nil {
			s.logger.Error("Failed to generate unique agent name", "baseName", candidateName, "orgName", org.Name, "projectName", project.Name, "error", err)
			return "", fmt.Errorf("failed to generate unique agent name: %w", err)
		}
		s.logger.Info("Generated unique agent name", "agentName", uniqueName, "orgName", orgName, "projectName", projectName)
		return uniqueName, nil
	}
	if payload.ResourceType == string(utils.ResourceTypeProject) {
		// Check if candidate name is available
		_, err = s.ocClient.GetProject(ctx, org.Name, candidateName)
		if err != nil && errors.Is(translateProjectError(err), utils.ErrProjectNotFound) {
			// Name is available, return it
			s.logger.Info("Generated unique project name", "projectName", candidateName, "orgName", orgName)
			return candidateName, nil
		}
		if err != nil {
			s.logger.Error("Failed to check project name availability", "name", candidateName, "orgName", org.Name, "error", err)
			return "", fmt.Errorf("failed to check project name availability: %w", err)
		}
		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueProjectName(ctx, org.Name, candidateName)
		if err != nil {
			s.logger.Error("Failed to generate unique project name", "baseName", candidateName, "orgName", org.Name, "error", err)
			return "", fmt.Errorf("failed to generate unique project name: %w", err)
		}
		s.logger.Info("Generated unique project name", "projectName", uniqueName, "orgName", orgName)
		return uniqueName, nil
	}
	return "", errors.New("invalid resource type for name generation")
}

// generateUniqueProjectName creates a unique name by appending a random suffix
func (s *agentManagerService) generateUniqueProjectName(ctx context.Context, orgName string, baseName string) (string, error) {
	// Create a name availability checker function that uses the project repository
	nameChecker := func(name string) (bool, error) {
		_, err := s.ocClient.GetProject(ctx, orgName, name)
		if err != nil && errors.Is(translateProjectError(err), utils.ErrProjectNotFound) {
			// Name is available
			return true, nil
		}
		if err != nil {
			s.logger.Error("Failed to check project name availability", "name", name, "orgName", orgName, "error", err)
			return false, fmt.Errorf("failed to check project name availability: %w", err)
		}
		// Name is taken
		return false, nil
	}

	// Use the common unique name generation logic from utils
	uniqueName, err := utils.GenerateUniqueNameWithSuffix(baseName, nameChecker)
	if err != nil {
		s.logger.Error("Failed to generate unique project name", "baseName", baseName, "orgName", orgName, "error", err)
		return "", fmt.Errorf("failed to generate unique project name: %w", err)
	}

	return uniqueName, nil
}

// generateUniqueAgentName creates a unique name by appending a random suffix
func (s *agentManagerService) generateUniqueAgentName(ctx context.Context, orgName string, projectName string, baseName string) (string, error) {
	// Create a name availability checker function that uses the agent repository
	nameChecker := func(name string) (bool, error) {
		exists, err := s.ocClient.ComponentExists(ctx, orgName, projectName, name, false)
		if err != nil {
			return false, fmt.Errorf("failed to check agent name availability: %w", err)
		}
		if !exists {
			// Name is available
			return true, nil
		}
		// Name is taken
		return false, nil
	}

	// Use the common unique name generation logic from utils
	uniqueName, err := utils.GenerateUniqueNameWithSuffix(baseName, nameChecker)
	if err != nil {
		return "", fmt.Errorf("failed to generate unique agent name: %w", err)
	}

	return uniqueName, nil
}

func (s *agentManagerService) DeleteAgent(ctx context.Context, orgName string, projectName string, agentName string) error {
	s.logger.Info("Deleting agent", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return translateOrgError(err)
	}
	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgName", orgName, "error", err)
		return translateProjectError(err)
	}

	// Step 1: Fetch workload and check for secret references in env vars
	secretRefNames, err := s.ocClient.GetWorkloadSecretRefNames(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Warn("Failed to get workload secret references", "agentName", agentName, "error", err)
		// Continue with deletion even if we can't get secret refs
	}

	// Step 2-4: For each secret reference, get its details, delete from KV, then delete the CR
	for _, secretRefName := range secretRefNames {
		s.cleanupSecretReference(ctx, orgName, secretRefName)
	}

	// Step 5: Delete agent component in OpenChoreo
	s.logger.Debug("Deleting oc agent", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	err = s.ocClient.DeleteComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		translatedErr := translateAgentError(err)
		if errors.Is(translatedErr, utils.ErrAgentNotFound) {
			s.logger.Warn("Agent not found during deletion, delete is idempotent", "agentName", agentName, "orgName", orgName, "projectName", projectName)
			s.deleteAgentLLMConfigurations(ctx, orgName, projectName, agentName)
			if configErr := s.agentConfigRepo.DeleteAllByAgent(orgName, projectName, agentName); configErr != nil {
				s.logger.Warn("Failed to delete agent configs from database", "agentName", agentName, "error", configErr)
			}
			return nil
		}
		s.logger.Error("Failed to delete oc agent", "agentName", agentName, "error", err)
		return translatedErr
	}

	// Delete agent-level LLM configurations (proxies, API keys, secret references, DB rows).
	s.deleteAgentLLMConfigurations(ctx, orgName, projectName, agentName)

	// Cleanup agent configs from database
	if configErr := s.agentConfigRepo.DeleteAllByAgent(orgName, projectName, agentName); configErr != nil {
		s.logger.Warn("Failed to delete agent configs from database", "agentName", agentName, "error", configErr)
		// Don't fail the deletion - configs will be orphaned but harmless
	}

	s.logger.Debug("Agent deleted from OpenChoreo successfully", "orgName", orgName, "agentName", agentName)
	return nil
}

// deleteAgentLLMConfigurations lists and deletes all agent-level LLM configurations for an agent.
// Each deletion goes through the full AgentConfigurationService.Delete path so external resources
// (proxy API keys, SecretReference CRs, proxy deployments) are cleaned up as well.
// Best-effort: individual failures are logged but do not abort the agent deletion.
func (s *agentManagerService) deleteAgentLLMConfigurations(ctx context.Context, orgName, projectName, agentName string) {
	listResp, err := s.agentConfigurationService.List(ctx, orgName, projectName, agentName, 1000, 0)
	if err != nil {
		s.logger.Warn("Failed to list agent LLM configurations for cleanup", "agentName", agentName, "error", err)
		return
	}
	for _, cfg := range listResp.Configs {
		configUUID, parseErr := uuid.Parse(cfg.UUID)
		if parseErr != nil {
			s.logger.Warn("Failed to parse LLM config UUID during agent deletion", "uuid", cfg.UUID, "error", parseErr)
			continue
		}
		if delErr := s.agentConfigurationService.Delete(ctx, configUUID, orgName, projectName, agentName); delErr != nil {
			s.logger.Warn("Failed to delete LLM configuration during agent deletion", "configUUID", cfg.UUID, "error", delErr)
		}
	}
}

// cleanupSecretReference fetches a secret reference, deletes its secrets from KV, then deletes the CR.
func (s *agentManagerService) cleanupSecretReference(ctx context.Context, orgName, secretRefName string) {
	// Fetch the secret reference to get the KV path and keys
	secretRef, err := s.ocClient.GetSecretReference(ctx, orgName, secretRefName)
	if err != nil {
		s.logger.Warn("Failed to get secret reference during cleanup", "name", secretRefName, "error", err)
		// Continue to delete the CR anyway
	}

	// Delete secrets from KV using the paths from the secret reference
	if s.secretMgmtClient != nil && secretRef != nil && len(secretRef.Data) > 0 {
		// The remote ref key contains the KV path (e.g., "org/project/env/agent")
		kvPaths := make(map[string]struct{})
		for _, data := range secretRef.Data {
			kvPaths[data.RemoteRef.Key] = struct{}{}
		}
		for kvPath := range kvPaths {
			if err := s.secretMgmtClient.DeleteSecretByPath(ctx, kvPath); err != nil {
				s.logger.Warn("Failed to delete secret from KV", "kvPath", kvPath, "error", err)
			} else {
				s.logger.Debug("Deleted secret from KV", "kvPath", kvPath)
			}
		}
	}

	// Delete the SecretReference CR
	if err := s.ocClient.DeleteSecretReference(ctx, orgName, secretRefName); err != nil {
		s.logger.Warn("Failed to delete secret reference CR", "name", secretRefName, "error", err)
	} else {
		s.logger.Debug("Deleted secret reference CR", "name", secretRefName)
	}
}

// BuildAgent triggers a build for an agent.
func (s *agentManagerService) BuildAgent(ctx context.Context, orgName string, projectName string, agentName string, commitId string) (*models.BuildResponse, error) {
	s.logger.Info("Building agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "commitId", commitId)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, translateProjectError(err)
	}

	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return nil, translateAgentError(err)
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("build operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Trigger build in OpenChoreo
	s.logger.Debug("Triggering build in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "commitId", commitId)
	build, err := s.ocClient.TriggerBuild(ctx, orgName, projectName, agentName, commitId)
	if err != nil {
		s.logger.Error("Failed to trigger build in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateBuildError(err)
	}
	s.logger.Info("Build triggered successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "buildName", build.Name)
	return build, nil
}

// DeployAgent deploys an agent.
func (s *agentManagerService) DeployAgent(ctx context.Context, orgName string, projectName string, agentName string, req *spec.DeployAgentRequest) (string, error) {
	s.logger.Info("Deploying agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "imageId", req.ImageId)
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return "", translateOrgError(err)
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return "", translateAgentError(err)
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return "", fmt.Errorf("deploy operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to fetch deployment pipeline", "orgName", orgName, "projectName", projectName, "error", err)
		return "", translatePipelineError(err)
	}
	lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
	if lowestEnv == "" {
		s.logger.Error("No environment found in deployment pipeline", "projectName", projectName)
		return "", fmt.Errorf("no environment found in deployment pipeline")
	}

	// Convert to deploy request with user-provided env vars
	deployReq := client.DeployRequest{
		ImageID: req.ImageId,
	}

	// Process environment variables, handling secrets separately
	// Always call processEnvVars to ensure secrets cleanup happens when all env vars are removed
	envVars, err := s.processEnvVars(ctx, orgName, projectName, lowestEnv, agentName, req.Env)
	if err != nil {
		s.logger.Error("Failed to process environment variables", "agentName", agentName, "error", err)
		return "", fmt.Errorf("failed to process environment variables: %w", err)
	}
	deployReq.Env = envVars

	targetEnv, err := s.ocClient.GetEnvironment(ctx, orgName, lowestEnv)
	if err != nil {
		s.logger.Warn("Failed to get environment details", "environment", lowestEnv, "error", err)
	}

	// Resolve enableAutoInstrumentation value:
	// 1. Use request value if provided
	// 2. Otherwise, read from DB for this environment
	// 3. If not in DB, default to true (first deployment)
	var enableAutoInstrumentation bool
	if req.EnableAutoInstrumentation != nil {
		enableAutoInstrumentation = *req.EnableAutoInstrumentation
		s.logger.Info("Using enableAutoInstrumentation from request", "agentName", agentName, "value", enableAutoInstrumentation)
	} else if targetEnv != nil {
		// Try to read from database
		existingConfig, configErr := s.agentConfigRepo.Get(orgName, projectName, agentName, targetEnv.Name)
		if errors.Is(configErr, repositories.ErrAgentConfigNotFound) {
			// No config in DB - this is first deployment, default to true
			enableAutoInstrumentation = true
			s.logger.Debug("No instrumentation config in database, defaulting to enabled", "agentName", agentName, "environment", targetEnv.Name)
		} else if configErr != nil {
			s.logger.Warn("Failed to read instrumentation config from database", "agentName", agentName, "environment", targetEnv.Name, "error", configErr)
			enableAutoInstrumentation = true // Default to enabled on error
		} else {
			enableAutoInstrumentation = existingConfig.EnableAutoInstrumentation
			s.logger.Debug("Read instrumentation config from database", "agentName", agentName, "environment", targetEnv.Name, "enableAutoInstrumentation", enableAutoInstrumentation)
		}
	} else {
		enableAutoInstrumentation = true // Default if no environment info available
	}

	// Update instrumentation traits before deploy for Python buildpack builds (agent-api only)
	// When auto-instrumentation is enabled: use OTEL instrumentation trait (full instrumentation)
	// When auto-instrumentation is disabled: use env injection trait (just env vars)
	if agent.Type.Type == string(utils.AgentTypeAPI) && agent.Build != nil && agent.Build.Buildpack != nil && agent.Build.Buildpack.Language == string(utils.LanguagePython) {
		hasOTELTrait, otelTraitErr := s.ocClient.HasTrait(ctx, orgName, projectName, agentName, client.TraitOTELInstrumentation)
		hasEnvTrait, envTraitErr := s.ocClient.HasTrait(ctx, orgName, projectName, agentName, client.TraitEnvInjection)

		if otelTraitErr != nil {
			s.logger.Warn("Failed to check OTEL instrumentation trait status", "agentName", agentName, "error", otelTraitErr)
		}
		if envTraitErr != nil {
			s.logger.Warn("Failed to check env injection trait status", "agentName", agentName, "error", envTraitErr)
		}

		if enableAutoInstrumentation {
			// Enable auto-instrumentation: attach OTEL trait, detach env injection trait
			if !hasOTELTrait && otelTraitErr == nil {
				s.logger.Info("Enabling instrumentation (attaching OTEL trait) before deploy", "agentName", agentName)
				if attachErr := s.attachOTELInstrumentationTrait(ctx, orgName, projectName, agentName); attachErr != nil {
					s.logger.Warn("Failed to attach OTEL instrumentation trait before deploy", "agentName", agentName, "error", attachErr)
				}
			}
			if hasEnvTrait && envTraitErr == nil {
				s.logger.Info("Detaching env injection trait (OTEL trait will handle env vars)", "agentName", agentName)
				if detachErr := s.detachEnvInjectionTrait(ctx, orgName, projectName, agentName); detachErr != nil {
					s.logger.Warn("Failed to detach env injection trait", "agentName", agentName, "error", detachErr)
				}
			}
		} else {
			// Disable auto-instrumentation: detach OTEL trait, attach env injection trait
			if hasOTELTrait && otelTraitErr == nil {
				s.logger.Info("Disabling instrumentation (detaching OTEL trait) before deploy", "agentName", agentName)
				if detachErr := s.detachOTELInstrumentationTrait(ctx, orgName, projectName, agentName); detachErr != nil {
					s.logger.Warn("Failed to detach OTEL instrumentation trait before deploy", "agentName", agentName, "error", detachErr)
				}
			}
			if !hasEnvTrait && envTraitErr == nil {
				s.logger.Info("Attaching env injection trait (for env vars without full instrumentation)", "agentName", agentName)
				if attachErr := s.attachEnvInjectionTrait(ctx, orgName, projectName, agentName); attachErr != nil {
					s.logger.Warn("Failed to attach env injection trait", "agentName", agentName, "error", attachErr)
				}
			}
		}
	}

	// Replace Component CR workflow parameters with env vars from deploy request
	// This replaces all existing env vars to ensure the component CR matches the deploy request
	s.logger.Debug("Replacing component workflow parameters with environment variables", "agentName", agentName, "envVarCount", len(deployReq.Env))
	if err := s.ocClient.ReplaceComponentEnvVars(ctx, orgName, projectName, agentName, deployReq.Env); err != nil {
		s.logger.Warn("Failed to replace component workflow parameters with env vars", "agentName", agentName, "error", err)
		// Continue with deploy even if this fails - env vars will still be applied to the workload
	}

	// Deploy agent component in OpenChoreo (after env vars and instrumentation are configured)
	s.logger.Debug("Deploying agent component in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "imageId", req.ImageId)
	if err := s.ocClient.Deploy(ctx, orgName, projectName, agentName, deployReq); err != nil {
		s.logger.Error("Failed to deploy agent component in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return "", err
	}

	// Persist instrumentation config to database
	if targetEnv != nil {
		agentConfig := &models.AgentConfig{
			OrgName:                   orgName,
			ProjectName:               projectName,
			AgentName:                 agentName,
			EnvironmentName:           targetEnv.Name,
			EnableAutoInstrumentation: enableAutoInstrumentation,
		}
		if configErr := s.agentConfigRepo.Upsert(agentConfig); configErr != nil {
			s.logger.Error("Failed to persist instrumentation config to database", "agentName", agentName, "environment", lowestEnv, "error", configErr)
		} else {
			s.logger.Debug("Persisted instrumentation config to database", "agentName", agentName, "environment", lowestEnv, "enableAutoInstrumentation", enableAutoInstrumentation)
		}
	}

	s.logger.Info("Agent deployed successfully to "+lowestEnv, "agentName", agentName, "orgName", org.Name, "projectName", projectName, "environment", lowestEnv)
	return lowestEnv, nil
}

func findLowestEnvironment(promotionPaths []models.PromotionPath) string {
	if len(promotionPaths) == 0 {
		return ""
	}

	// Collect all target environments
	targets := make(map[string]bool)
	for _, path := range promotionPaths {
		for _, target := range path.TargetEnvironmentRefs {
			targets[target.Name] = true
		}
	}

	// Find a source environment that is not a target
	for _, path := range promotionPaths {
		if !targets[path.SourceEnvironmentRef] {
			return path.SourceEnvironmentRef
		}
	}
	return ""
}

// processEnvVars handles environment variables, separating secrets from plain values.
// This function handles configuration updates including:
//   - Adding new secret keys to KV and SecretReference
//   - Updating existing secret values in KV
//   - Preserving existing secrets when secretRef is provided without a new value
//   - Removing keys that are no longer in the request from KV and SecretReference
//
// For sensitive env vars (isSensitive=true):
//   - If secretRef is provided and value is empty: preserves existing secret (no KV update)
//   - If value is provided: stores/updates the secret value in OpenBao
//   - Returns env var with secretKeyRef (Name=K8s Secret name, Key=property)
//
// For plain env vars:
//   - Returns env var with the value directly
func (s *agentManagerService) processEnvVars(
	ctx context.Context,
	orgName, projectName, environmentName, componentName string,
	envVars []spec.EnvironmentVariable,
) ([]client.EnvVar, error) {
	var result []client.EnvVar
	secretData := make(map[string]string)
	var preservedSecretKeys []string

	// Build secret location for OpenBao
	location := secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     projectName,
		EnvironmentName: environmentName,
		EntityName:      componentName,
	}
	defaultSecretRefName := utils.BuildSecretRefName(componentName)

	for _, env := range envVars {
		if env.GetIsSensitive() {
			envSecretRefName := defaultSecretRefName
			// Check if this is an existing secret that should be preserved
			if env.HasSecretRef() && env.GetValue() == "" {
				existingSecretRefName := env.GetSecretRef()
				if strings.EqualFold(existingSecretRefName, defaultSecretRefName) {
					// Preserve existing secret - don't add to secretData (no KV update needed)
					// Just track the key so we include it in the SecretReference
					preservedSecretKeys = append(preservedSecretKeys, env.Key)
					s.logger.Debug("Preserving existing secret", "key", env.Key, "secretRef", env.GetSecretRef())
				} else {
					s.logger.Info(fmt.Sprintf("Skipping existing system-managed secret-ref %s", existingSecretRefName))
				}
				// Preserve per-env existing secretRef for this key only (system-managed secret)
				envSecretRefName = existingSecretRefName
			} else if env.GetValue() != "" {
				// New or updated secret - add to secretData for KV write
				secretData[env.Key] = env.GetValue()
			} else {
				// isSensitive=true but no secretRef and no value - this is an error
				return nil, fmt.Errorf("sensitive environment variable %q requires either a value or secretRef", env.Key)
			}
			// For secret env vars, use secretKeyRef pointing to K8s Secret
			result = append(result, client.EnvVar{
				Key: env.Key,
				ValueFrom: &client.EnvVarValueFrom{
					SecretKeyRef: &client.SecretKeyRef{
						Name: envSecretRefName, // SecretReference name (per env var)
						Key:  env.Key,          // Key within the secret
					},
				},
			})
		} else {
			// Plain env var - use value directly
			result = append(result, client.EnvVar{
				Key:   env.Key,
				Value: env.GetValue(),
			})
		}
	}

	// Handle secrets: update KV store and SecretReference
	if err := s.syncSecrets(ctx, location, secretData, preservedSecretKeys); err != nil {
		return nil, err
	}

	return result, nil
}

// syncSecrets synchronizes secrets between the request and OpenBao/SecretReference.
// It handles:
//   - Creating new secrets when none exist
//   - Updating secrets with new data (adds/updates keys)
//   - Preserving existing secrets (keys in preservedSecretKeys are kept without KV update)
//   - Removing keys that are no longer present
//   - Deleting SecretReference if all secrets are removed
//
// Parameters:
//   - newSecretData: map of secret keys to values that need to be written to KV
//   - preservedSecretKeys: keys of existing secrets to preserve (no KV update, but included in SecretReference)
func (s *agentManagerService) syncSecrets(
	ctx context.Context,
	location secretmanagersvc.SecretLocation,
	newSecretData map[string]string,
	preservedSecretKeys []string,
) error {
	secretRefName := utils.BuildSecretRefName(location.EntityName)
	totalSecretCount := len(newSecretData) + len(preservedSecretKeys)

	// Case 1: No secrets in current request (neither new nor preserved) - cleanup any existing secrets
	if totalSecretCount == 0 {
		// Delete secret from KV directly
		if s.secretMgmtClient != nil {
			kvPath, err := location.KVPath()
			if err != nil {
				return fmt.Errorf("invalid secret location: %w", err)
			}
			if err := s.secretMgmtClient.DeleteSecretByPath(ctx, kvPath); err != nil {
				s.logger.Warn("Failed to delete secret from KV during cleanup", "kvPath", kvPath, "error", err)
			} else {
				s.logger.Debug("Deleted secret from KV", "kvPath", kvPath)
			}
		}

		// Delete the SecretReference CR
		if err := s.ocClient.DeleteSecretReference(ctx, location.OrgName, secretRefName); err != nil {
			s.logger.Warn("Failed to delete SecretReference CR during cleanup", "name", secretRefName, "error", err)
		} else {
			s.logger.Debug("Deleted SecretReference CR", "name", secretRefName)
		}
		return nil
	}

	kvPath, err := location.KVPath()
	if err != nil {
		s.logger.Warn("Failed to construct KV path for secrets sync", "location", location, "error", err)
		return fmt.Errorf("failed to construct KV path for secrets sync: %w", err)
	}

	// Case 2: Have secrets to store/update in KV (either new or preserved)
	// This also handles deletions - by upserting only the desired keys, others are removed
	if len(newSecretData) > 0 || len(preservedSecretKeys) > 0 {
		if s.secretMgmtClient == nil {
			return fmt.Errorf("secret management is not enabled but secret env vars were provided")
		}

		s.logger.Debug("Storing secrets in KV", "kvPath", kvPath, "newSecretCount", len(newSecretData), "preservedCount", len(preservedSecretKeys))

		var dataToWrite map[string]string

		if len(preservedSecretKeys) > 0 {
			// Have preserved secrets - read existing values, merge with new, then upsert
			// This replaces the entire KV content, effectively deleting any keys not in the final set
			s.logger.Debug("Reading existing secrets to merge with preserved keys", "kvPath", kvPath)
			existingData, readErr := s.secretMgmtClient.GetSecret(ctx, kvPath)
			if readErr != nil && !errors.Is(readErr, secretmanagersvc.ErrSecretNotFound) {
				return fmt.Errorf("failed to read existing secrets for merge: %w", readErr)
			}

			// Build merged data: preserved values from KV + new values
			dataToWrite = make(map[string]string)
			for _, key := range preservedSecretKeys {
				if existingData != nil {
					if val, ok := existingData[key]; ok {
						dataToWrite[key] = val
					} else {
						return fmt.Errorf("preserved secret key %q not found in existing secrets at %s", key, kvPath)
					}
				} else {
					return fmt.Errorf("no existing secrets found at %s to preserve keys", kvPath)
				}
			}
			for key, val := range newSecretData {
				dataToWrite[key] = val
			}
		} else {
			// No preserved secrets - just use new data (replaces entire KV, deleting any previous keys)
			dataToWrite = newSecretData
		}

		// Upsert the secrets (creates or replaces - deleted keys are removed)
		_, err := s.secretMgmtClient.CreateSecret(ctx, location, dataToWrite)
		if err != nil {
			if errors.Is(err, secretmanagersvc.ErrNotManaged) {
				return fmt.Errorf("secret path %q is already owned by another system and cannot be overwritten; manual cleanup may be required: %w", kvPath, utils.ErrSecretPathConflict)
			}
			return fmt.Errorf("failed to upsert secrets in OpenBao: %w", err)
		}
	}

	// Build combined list of secret keys (new + preserved) for SecretReference
	secretKeys := make([]string, 0, totalSecretCount)
	for key := range newSecretData {
		secretKeys = append(secretKeys, key)
	}
	secretKeys = append(secretKeys, preservedSecretKeys...)

	// Check if SecretReference already exists
	secretRefReq := client.CreateSecretReferenceRequest{
		Namespace:       location.OrgName,
		Name:            secretRefName,
		ProjectName:     location.ProjectName,
		ComponentName:   location.EntityName,
		KVPath:          kvPath,
		SecretKeys:      secretKeys,
		RefreshInterval: config.GetConfig().SecretManager.RefreshInterval,
	}

	_, err = s.ocClient.GetSecretReference(ctx, location.OrgName, secretRefName)
	if err != nil {
		// Only create if SecretReference doesn't exist (NotFound); other errors should be surfaced
		if !errors.Is(err, utils.ErrNotFound) {
			return fmt.Errorf("failed to check SecretReference existence: %w", err)
		}
		// SecretReference doesn't exist, create it
		s.logger.Debug("Creating SecretReference CR", "name", secretRefName, "namespace", location.OrgName, "kvPath", kvPath, "keyCount", len(secretKeys))
		if _, createErr := s.ocClient.CreateSecretReference(ctx, location.OrgName, secretRefReq); createErr != nil {
			s.logger.Warn("SecretReference creation failed after KV write succeeded - will self-heal on next sync", "kvPath", kvPath, "secretRefName", secretRefName, "error", createErr)
			return fmt.Errorf("failed to create SecretReference: %w", createErr)
		}
	} else {
		// SecretReference exists, update it
		s.logger.Debug("Updating SecretReference CR", "name", secretRefName, "namespace", location.OrgName, "kvPath", kvPath, "keyCount", len(secretKeys), "preservedCount", len(preservedSecretKeys))
		if _, updateErr := s.ocClient.UpdateSecretReference(ctx, location.OrgName, secretRefName, secretRefReq); updateErr != nil {
			s.logger.Warn("SecretReference update failed after KV write succeeded - will self-heal on next sync", "kvPath", kvPath, "secretRefName", secretRefName, "error", updateErr)
			return fmt.Errorf("failed to update SecretReference: %w", updateErr)
		}
	}

	s.logger.Info("Secrets synchronized successfully", "componentName", location.EntityName, "kvPath", kvPath, "newSecretCount", len(newSecretData), "preservedSecretCount", len(preservedSecretKeys))
	return nil
}

func (s *agentManagerService) ListAgentBuilds(ctx context.Context, orgName string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error) {
	s.logger.Info("Listing agent builds", "agentName", agentName, "orgName", orgName, "projectName", projectName, "limit", limit, "offset", offset)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to validate organization", "orgName", orgName, "error", err)
		return nil, 0, translateOrgError(err)
	}

	// Check if component already exists
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch component", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, 0, translateAgentError(err)
	}

	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, 0, fmt.Errorf("build operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}

	// Fetch all builds from OpenChoreo first
	allBuilds, err := s.ocClient.ListBuilds(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to list builds from OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, 0, err
	}

	// Calculate total count
	total := int32(len(allBuilds))

	// Apply pagination
	var paginatedBuilds []*models.BuildResponse
	if offset >= total {
		// If offset is beyond available data, return empty slice
		paginatedBuilds = []*models.BuildResponse{}
	} else {
		endIndex := offset + limit
		if endIndex > total {
			endIndex = total
		}
		paginatedBuilds = allBuilds[offset:endIndex]
	}

	s.logger.Info("Listed builds successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "totalBuilds", total, "returnedBuilds", len(paginatedBuilds))
	return paginatedBuilds, total, nil
}

func (s *agentManagerService) GetBuild(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.BuildDetailsResponse, error) {
	s.logger.Info("Getting build details", "agentName", agentName, "buildName", buildName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return nil, translateAgentError(err)
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("build operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Fetch the build from OpenChoreo
	build, err := s.ocClient.GetBuild(ctx, orgName, projectName, agentName, buildName)
	if err != nil {
		s.logger.Error("Failed to get build from OpenChoreo", "buildName", buildName, "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateBuildError(err)
	}

	s.logger.Info("Fetched build successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "buildName", build.Name)
	return build, nil
}

func (s *agentManagerService) GetAgentDeployments(ctx context.Context, orgName string, projectName string, agentName string) ([]*models.DeploymentResponse, error) {
	s.logger.Info("Getting agent deployments", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}
	project, err := s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, translateProjectError(err)
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, project.Name, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return nil, translateAgentError(err)
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("deployment operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}

	// Get deployment pipeline name from project
	pipelineName := project.DeploymentPipeline
	deployments, err := s.ocClient.GetDeployments(ctx, orgName, pipelineName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to get deployments from OpenChoreo", "agentName", agentName, "pipelineName", pipelineName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get deployments for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched deployments successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "deploymentCount", len(deployments))
	return deployments, nil
}

// UpdateAgentDeploymentState updates the deployment state of an agent in a specific environment
func (s *agentManagerService) UpdateAgentDeploymentState(ctx context.Context, orgName string, projectName string, agentName string, environment string, state string) error {
	s.logger.Info("Updating agent deployment state", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment, "state", state)

	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return translateOrgError(err)
	}

	// Validate agent exists and is an internal agent
	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return translateAgentError(err)
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return fmt.Errorf("deployment state update is not supported for agent type: '%s'", agent.Provisioning.Type)
	}

	// Validate environment exists
	_, err = s.ocClient.GetEnvironment(ctx, orgName, environment)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environment, "orgName", orgName, "error", err)
		return translateEnvironmentError(err)
	}

	// Convert string state to gen.ReleaseBindingSpecState
	var bindingState gen.ReleaseBindingSpecState
	switch state {
	case utils.DeploymentStateActive:
		bindingState = gen.ReleaseBindingSpecStateActive
	case utils.DeploymentStateUndeploy:
		bindingState = gen.ReleaseBindingSpecStateUndeploy
	default:
		return fmt.Errorf("%w: invalid state '%s', must be '%s' or '%s'", utils.ErrBadRequest, state, utils.DeploymentStateActive, utils.DeploymentStateUndeploy)
	}

	// Update the deployment state via OpenChoreo client
	err = s.ocClient.UpdateDeploymentState(ctx, orgName, projectName, agentName, environment, bindingState)
	if err != nil {
		s.logger.Error("Failed to update deployment state", "agentName", agentName, "environment", environment, "state", state, "error", err)
		return fmt.Errorf("failed to update deployment state for agent %s in environment %s: %w", agentName, environment, err)
	}

	s.logger.Info("Updated deployment state successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment, "state", state)
	return nil
}

func (s *agentManagerService) GetAgentEndpoints(ctx context.Context, orgName string, projectName string, agentName string, environmentName string) (map[string]models.EndpointsResponse, error) {
	s.logger.Info("Getting agent endpoints", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environmentName)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}
	project, err := s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, translateProjectError(err)
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, project.Name, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "projectName", projectName, "orgName", orgName, "error", err)
		return nil, translateAgentError(err)
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("endpoints are not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Check if environment exists
	_, err = s.ocClient.GetEnvironment(ctx, orgName, environmentName)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environmentName, "orgName", orgName, "error", err)
		return nil, translateEnvironmentError(err)
	}
	s.logger.Debug("Fetching agent endpoints from OpenChoreo", "agentName", agentName, "environment", environmentName, "orgName", orgName, "projectName", projectName)
	endpoints, err := s.ocClient.GetComponentEndpoints(ctx, orgName, projectName, agentName, environmentName)
	if err != nil {
		s.logger.Error("Failed to fetch endpoints", "agentName", agentName, "environment", environmentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get endpoints for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched endpoints successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environmentName, "endpointCount", len(endpoints))
	return endpoints, nil
}

func (s *agentManagerService) GetAgentConfigurations(ctx context.Context, orgName string, projectName string, agentName string, environment string) ([]models.EnvVars, error) {
	s.logger.Info("Getting agent configurations", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment)
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "projectName", projectName, "orgName", orgName, "error", err)
		return nil, translateAgentError(err)
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		s.logger.Warn("Configuration operation not supported for agent type", "agentName", agentName, "provisioningType", agent.Provisioning.Type, "orgName", orgName, "projectName", projectName)
		return nil, fmt.Errorf("configuration operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Check if environment exists
	_, err = s.ocClient.GetEnvironment(ctx, orgName, environment)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environment, "orgName", orgName, "error", err)
		return nil, translateEnvironmentError(err)
	}

	s.logger.Debug("Fetching agent configurations from OpenChoreo", "agentName", agentName, "environment", environment, "orgName", orgName, "projectName", projectName)
	configurations, err := s.ocClient.GetComponentConfigurations(ctx, orgName, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch configurations", "agentName", agentName, "environment", environment, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get configurations for agent %s: %w", agentName, err)
	}

	// Filter out system-injected environment variables
	filteredConfigurations := make([]models.EnvVars, 0, len(configurations))
	for _, config := range configurations {
		if _, isSystemVar := client.SystemInjectedEnvVars[config.Key]; !isSystemVar {
			filteredConfigurations = append(filteredConfigurations, config)
		}
	}

	s.logger.Info("Fetched configurations successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment, "configCount", len(filteredConfigurations))
	return filteredConfigurations, nil
}

func (s *agentManagerService) GetBuildLogs(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.LogsResponse, error) {
	s.logger.Info("Getting build logs", "agentName", agentName, "buildName", buildName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to validate organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}
	// Validates the project name by checking its existence
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get OpenChoreo project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, translateProjectError(err)
	}

	// Check if component already exists
	_, err = s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to check component existence", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	// Check if build exists
	build, err := s.ocClient.GetBuild(ctx, orgName, projectName, agentName, buildName)
	if err != nil {
		s.logger.Error("Failed to get build", "buildName", buildName, "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateBuildError(err)
	}

	// Fetch the build logs from Observability service
	buildLogsParams := observabilitysvc.BuildLogsParams{
		NamespaceName:      orgName,
		ProjectName:        projectName,
		AgentComponentName: agentName,
		BuildName:          build.Name,
	}
	buildLogs, err := s.observabilitySvcClient.GetBuildLogs(ctx, buildLogsParams)
	if err != nil {
		s.logger.Error("Failed to fetch build logs from observability service", "buildName", build.Name, "error", err)
		return nil, fmt.Errorf("failed to fetch build logs: %w", err)
	}
	s.logger.Info("Fetched build logs successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "buildName", buildName, "logCount", len(buildLogs.Logs))
	return buildLogs, nil
}

func (s *agentManagerService) GetAgentRuntimeLogs(ctx context.Context, orgName string, projectName string, agentName string, payload spec.LogFilterRequest) (*models.LogsResponse, error) {
	s.logger.Info("Getting application logs", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to validate organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}
	// Validates the project name by checking its existence
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get OpenChoreo project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, translateProjectError(err)
	}

	// Check if component already exists
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to check component existence", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("runtime logs are not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Fetch environment from open choreo
	environment, err := s.ocClient.GetEnvironment(ctx, orgName, payload.EnvironmentName)
	if err != nil {
		s.logger.Error("Failed to fetch environment from OpenChoreo", "environmentName", payload.EnvironmentName, "orgName", orgName, "error", err)
		return nil, translateEnvironmentError(err)
	}

	// Fetch the run time logs from Observability service
	componentLogsParams := observabilitysvc.ComponentLogsParams{
		AgentComponentId: agent.UUID,
		EnvId:            environment.UUID,
		NamespaceName:    orgName,
		ComponentName:    agentName,
		ProjectName:      projectName,
		EnvironmentName:  payload.EnvironmentName,
	}
	applicationLogs, err := s.observabilitySvcClient.GetComponentLogs(ctx, componentLogsParams, payload)
	if err != nil {
		s.logger.Error("Failed to fetch application logs from observability service", "agent", agentName, "error", err)
		return nil, fmt.Errorf("failed to fetch application logs: %w", err)
	}
	s.logger.Info("Fetched application logs successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "logCount", len(applicationLogs.Logs))
	return applicationLogs, nil
}

func (s *agentManagerService) GetAgentMetrics(ctx context.Context, orgName string, projectName string, agentName string, payload spec.MetricsFilterRequest) (*spec.MetricsResponse, error) {
	s.logger.Info("Getting agent metrics", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to validate organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}
	// Validates the project name by checking its existence
	project, err := s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get OpenChoreo project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, translateProjectError(err)
	}
	// Fetch environment from open choreo
	environment, err := s.ocClient.GetEnvironment(ctx, orgName, payload.EnvironmentName)
	if err != nil {
		s.logger.Error("Failed to fetch environment from OpenChoreo", "environmentName", payload.EnvironmentName, "orgName", orgName, "error", err)
		return nil, translateEnvironmentError(err)
	}
	// Check if component already exists
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to check component existence", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	// Fetch the metrics from Observability service
	componentMetricsParams := observabilitysvc.ComponentMetricsParams{
		AgentComponentId: agent.UUID,
		EnvId:            environment.UUID,
		ProjectId:        project.UUID,
		NamespaceName:    orgName,
		ProjectName:      projectName,
		ComponentName:    agentName,
		EnvironmentName:  payload.EnvironmentName,
	}
	metrics, err := s.observabilitySvcClient.GetComponentMetrics(ctx, componentMetricsParams, payload)
	if err != nil {
		s.logger.Error("Failed to fetch agent metrics from observability service", "agent", agentName, "error", err)
		return nil, fmt.Errorf("failed to fetch agent metrics: %w", err)
	}
	s.logger.Info("Fetched agent metrics successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	return utils.ConvertToMetricsResponse(metrics), nil
}
