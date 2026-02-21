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

	observabilitysvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/observabilitysvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
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
	ocClient               client.OpenChoreoClient
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient
	gitRepositoryService   RepositoryService
	tokenManagerService    AgentTokenManagerService
	agentConfigRepo        repositories.AgentConfigRepository
	logger                 *slog.Logger
}

func NewAgentManagerService(
	OpenChoreoClient client.OpenChoreoClient,
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient,
	gitRepositoryService RepositoryService,
	tokenManagerService AgentTokenManagerService,
	agentConfigRepo repositories.AgentConfigRepository,
	logger *slog.Logger,
) AgentManagerService {
	return &agentManagerService{
		ocClient:               OpenChoreoClient,
		observabilitySvcClient: observabilitySvcClient,
		gitRepositoryService:   gitRepositoryService,
		tokenManagerService:    tokenManagerService,
		agentConfigRepo:        agentConfigRepo,
		logger:                 logger,
	}
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

// mapConfigurations converts spec.Configurations to client.Configurations
func mapConfigurations(specConfigs *spec.Configurations) *client.Configurations {
	if specConfigs == nil {
		return nil
	}

	// Check if there's anything to map
	if len(specConfigs.Env) == 0 && specConfigs.EnableAutoInstrumentation == nil {
		return nil
	}

	configs := &client.Configurations{
		EnableAutoInstrumentation: specConfigs.EnableAutoInstrumentation,
	}

	if len(specConfigs.Env) > 0 {
		configs.Env = make([]client.EnvVar, len(specConfigs.Env))
		for i, env := range specConfigs.Env {
			configs.Env[i] = client.EnvVar{Key: env.Key, Value: env.Value}
		}
	}

	return configs
}

// mapRepository converts spec.RepositoryConfig to client.RepositoryConfig
func mapRepository(specRepo *spec.RepositoryConfig) *client.RepositoryConfig {
	if specRepo == nil {
		return nil
	}
	return &client.RepositoryConfig{
		URL:     specRepo.Url,
		Branch:  specRepo.Branch,
		AppPath: specRepo.AppPath,
	}
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

// enableInstrumentation enables observability instrumentation for the agent based on build type.
// For buildpack builds (Python): attaches OTEL instrumentation trait
// For docker builds: injects tracing environment variables
// Note: This function should only be called when instrumentation is enabled (check before calling)
func (s *agentManagerService) enableInstrumentation(ctx context.Context, orgName, projectName string, req *spec.CreateAgentRequest) error {
	if req.AgentType.Type != string(utils.AgentTypeAPI) {
		s.logger.Debug("Skipping instrumentation for non-API agent", "agentName", req.Name, "agentType", req.AgentType.Type)
		return nil
	}

	if req.Build == nil {
		s.logger.Debug("Skipping instrumentation, no build configuration", "agentName", req.Name)
		return nil
	}

	// For buildpack builds, use traits (currently only Python supported)
	if req.Build.BuildpackBuild != nil {
		language := req.Build.BuildpackBuild.Buildpack.Language
		if language == string(utils.LanguagePython) {
			s.logger.Debug("Enabling instrumentation via trait for buildpack build", "agentName", req.Name, "language", language)
			return s.attachOTELInstrumentationTrait(ctx, orgName, projectName, req.Name)
		}
		s.logger.Debug("Instrumentation not supported for buildpack language", "agentName", req.Name, "language", language)
		return nil
	}

	// For docker builds, inject environment variables
	if req.Build.DockerBuild != nil {
		s.logger.Debug("Enabling instrumentation via env vars for docker build", "agentName", req.Name)
		return s.injectTracingEnvVarsForDockerAgents(ctx, orgName, projectName, req)
	}

	return nil
}

// attachOTELInstrumentationTrait attaches OTEL instrumentation trait to the agent
// The trait handles injection of OTEL configuration including the agent API key
func (s *agentManagerService) attachOTELInstrumentationTrait(ctx context.Context, orgName, projectName, agentName string) error {
	// Generate agent API key for the trait parameters
	apiKey, err := s.generateAgentAPIKey(ctx, orgName, projectName, agentName)
	if err != nil {
		return fmt.Errorf("failed to generate agent API key: %w", err)
	}

	if err := s.ocClient.AttachTrait(ctx, orgName, projectName, agentName, client.TraitOTELInstrumentation, apiKey); err != nil {
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

// handleInstrumentationUpdate handles enabling/disabling instrumentation when build parameters are updated
// It always injects tracing env vars and manages the OTEL instrumentation trait for Python buildpack builds
func (s *agentManagerService) handleInstrumentationUpdate(ctx context.Context, orgName, projectName, agentName string, existingAgent *models.AgentResponse, req *spec.UpdateAgentBuildParametersRequest) error {
	// Only handle instrumentation for API agents
	if existingAgent.Type.Type != string(utils.AgentTypeAPI) {
		return nil
	}

	// Check if this is a Python buildpack build
	isPythonBuildpack := req.Build.BuildpackBuild != nil && req.Build.BuildpackBuild.Buildpack.Language == string(utils.LanguagePython)

	// Determine if instrumentation should be enabled
	// Default to true if not specified, but only for Python buildpack builds
	enableInstrumentation := true
	if req.Configurations != nil && req.Configurations.EnableAutoInstrumentation != nil {
		enableInstrumentation = *req.Configurations.EnableAutoInstrumentation
	} else if !isPythonBuildpack {
		enableInstrumentation = false
	} else {
		// No explicit config provided for Python buildpack - preserve current state
		s.logger.Debug("EnableAutoInstrumentation not specified, preserving current state", "agentName", agentName)
		return nil
	}

	// Rule 1: ALWAYS inject tracing env vars (for both Python and Docker builds)
	s.logger.Debug("Injecting tracing env vars", "agentName", agentName, "isPython", isPythonBuildpack)
	if err := s.injectTracingEnvVarsByName(ctx, orgName, projectName, agentName); err != nil {
		return fmt.Errorf("failed to inject tracing env vars: %w", err)
	}

	// Rule 2: Manage the instrumentation trait for Python buildpack only
	// - Add trait if: Python buildpack AND instrumentation enabled
	// - Remove trait otherwise
	if isPythonBuildpack && enableInstrumentation {
		s.logger.Info("Attaching instrumentation trait", "agentName", agentName)
		if err := s.attachOTELInstrumentationTrait(ctx, orgName, projectName, agentName); err != nil {
			return fmt.Errorf("failed to attach instrumentation trait: %w", err)
		}
	} else {
		s.logger.Info("Detaching instrumentation trait", "agentName", agentName, "isPython", isPythonBuildpack, "enabled", enableInstrumentation)
		if err := s.detachOTELInstrumentationTrait(ctx, orgName, projectName, agentName); err != nil {
			return fmt.Errorf("failed to detach instrumentation trait: %w", err)
		}
	}

	// Persist updated instrumentation config to database
	s.persistInstrumentationConfig(ctx, orgName, projectName, agentName, existingAgent.UUID, enableInstrumentation)

	return nil
}

// persistInstrumentationConfig saves the instrumentation config to the database
func (s *agentManagerService) persistInstrumentationConfig(ctx context.Context, orgName, projectName, agentName, agentUUID string, enableAutoInstrumentation bool) {
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
		return "", fmt.Errorf("failed to get deployment pipeline: %w", err)
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

// injectTracingEnvVarsForDockerAgents injects tracing-related environment variables for docker-based agents
func (s *agentManagerService) injectTracingEnvVarsForDockerAgents(ctx context.Context, orgName, projectName string, req *spec.CreateAgentRequest) error {
	s.logger.Debug("Injecting tracing environment variables for docker-based agent", "agentName", req.Name)
	return s.injectTracingEnvVarsByName(ctx, orgName, projectName, req.Name)
}

// updateComponentEnvVars updates the component's workflow parameters with new environment variables
func (s *agentManagerService) updateComponentEnvVars(ctx context.Context, orgName, projectName, componentName string, newEnvVars []client.EnvVar) error {
	s.logger.Debug("Updating component environment variables", "componentName", componentName, "newEnvCount", len(newEnvVars))

	// Use the UpdateComponentEnvironmentVariables method from the OpenChoreo client
	if err := s.ocClient.UpdateComponentEnvironmentVariables(ctx, orgName, projectName, componentName, newEnvVars); err != nil {
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
		return nil, err
	}
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to fetch agent from oc: %w", err)
	}

	// Populate enableAutoInstrumentation from database
	// Get the first/lowest environment to read the config
	pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if pipelineErr == nil && len(pipeline.PromotionPaths) > 0 {
		lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
		if lowestEnv != "" {
			agentConfig, configErr := s.agentConfigRepo.Get(orgName, agentName, lowestEnv)
			if configErr != nil {
				s.logger.Warn("Failed to read agent config from database", "agentName", agentName, "environment", lowestEnv, "error", configErr)
			} else if agentConfig != nil {
				agent.Configurations = &models.Configurations{
					EnableAutoInstrumentation: &agentConfig.EnableAutoInstrumentation,
				}
				s.logger.Debug("Populated enableAutoInstrumentation from database", "agentName", agentName, "environment", lowestEnv, "enableAutoInstrumentation", agentConfig.EnableAutoInstrumentation)
			} else {
				// No config in DB - default to true for display purposes
				defaultEnabled := true
				agent.Configurations = &models.Configurations{
					EnableAutoInstrumentation: &defaultEnabled,
				}
				s.logger.Debug("No agent config in database, defaulting to enabled", "agentName", agentName)
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
		return nil, 0, err
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
	// Log the enableAutoInstrumentation value received
	var enableAutoInstrumentationValue string = "nil (Configurations is nil)"
	if req.Configurations != nil {
		if req.Configurations.EnableAutoInstrumentation != nil {
			enableAutoInstrumentationValue = fmt.Sprintf("%v", *req.Configurations.EnableAutoInstrumentation)
		} else {
			enableAutoInstrumentationValue = "nil (pointer is nil)"
		}
	}
	s.logger.Info("Creating agent", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "provisioningType", req.Provisioning.Type, "enableAutoInstrumentation", enableAutoInstrumentationValue)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return err
	}
	// Create component
	createAgentReq := toCreateAgentRequest(req)
	if err := s.ocClient.CreateComponent(ctx, orgName, projectName, createAgentReq); err != nil {
		s.logger.Error("Failed to create agent component", "agentName", req.Name, "error", err)
		return err
	}

	// For internal agents, enable instrumentation (if enabled) and trigger initial build
	if req.Provisioning.Type == string(utils.InternalAgent) {
		s.logger.Debug("Component created successfully", "agentName", req.Name)

		// Only enable instrumentation if not explicitly disabled
		if req.Configurations == nil || req.Configurations.EnableAutoInstrumentation == nil || *req.Configurations.EnableAutoInstrumentation {
			if err := s.enableInstrumentation(ctx, orgName, projectName, req); err != nil {
				s.logger.Error("Failed to enable instrumentation for agent", "agentName", req.Name, "error", err)
				// Rollback - delete the created agent
				if errDeletion := s.ocClient.DeleteComponent(ctx, orgName, projectName, req.Name); errDeletion != nil {
					s.logger.Error("Failed to rollback agent creation after instrumentation enabling failure", "agentName", req.Name, "error", errDeletion)
				}
				return err
			}
		} else {
			s.logger.Info("Auto instrumentation disabled by user", "agentName", req.Name)
			// For Python buildpack agents, inject tracing env vars even when auto-instrumentation
			// is disabled so the agent can reach the observability backend via the SDK manually,
			// matching the behaviour of Docker-based agents.
			if req.Build != nil && req.Build.BuildpackBuild != nil &&
				req.Build.BuildpackBuild.Buildpack.Language == string(utils.LanguagePython) {
				if err := s.injectTracingEnvVarsByName(ctx, orgName, projectName, req.Name); err != nil {
					s.logger.Error("Failed to inject tracing env vars for disabled Python instrumentation", "agentName", req.Name, "error", err)
					// Rollback - delete the created agent
					if errDeletion := s.ocClient.DeleteComponent(ctx, orgName, projectName, req.Name); errDeletion != nil {
						s.logger.Error("Failed to rollback agent creation after env var injection failure", "agentName", req.Name, "error", errDeletion)
					}
					return err
				}
			}
		}

		// Trigger initial build
		if err := s.triggerInitialBuild(ctx, orgName, projectName, req); err != nil {
			s.logger.Error("Failed to trigger initial build for agent", "agentName", req.Name, "error", err)
			return err
		}
		s.logger.Debug("Triggered initial build for agent", "agentName", req.Name)

		// Persist initial instrumentation config to database
		s.persistInitialAgentConfig(ctx, orgName, projectName, req)
	}

	s.logger.Info("Agent created successfully", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "provisioningType", req.Provisioning.Type)
	return nil
}

// persistInitialAgentConfig saves the initial agent configuration to the database
func (s *agentManagerService) persistInitialAgentConfig(ctx context.Context, orgName, projectName string, req *spec.CreateAgentRequest) {
	// Determine enableAutoInstrumentation value
	enableAutoInstrumentation := true // Default
	if req.Configurations != nil && req.Configurations.EnableAutoInstrumentation != nil {
		enableAutoInstrumentation = *req.Configurations.EnableAutoInstrumentation
		s.logger.Info("persistInitialAgentConfig: Using value from request", "agentName", req.Name, "enableAutoInstrumentation", enableAutoInstrumentation)
	} else {
		s.logger.Info("persistInitialAgentConfig: No value in request, using default true", "agentName", req.Name, "configurations_nil", req.Configurations == nil)
	}

	// Get the first/lowest environment
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		s.logger.Warn("Failed to get deployment pipeline for config persistence", "agentName", req.Name, "error", err)
		return
	}

	lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
	if lowestEnv == "" {
		s.logger.Warn("No environment found for config persistence", "agentName", req.Name)
		return
	}

	targetEnv, err := s.ocClient.GetEnvironment(ctx, orgName, lowestEnv)
	if err != nil {
		s.logger.Warn("Failed to get environment details for config persistence", "agentName", req.Name, "environment", lowestEnv, "error", err)
		return
	}

	agentConfig := &models.AgentConfig{
		OrgName:                   orgName,
		ProjectName:               projectName,
		AgentName:                 req.Name,
		EnvironmentName:           targetEnv.Name,
		EnableAutoInstrumentation: enableAutoInstrumentation,
	}

	if err := s.agentConfigRepo.Upsert(agentConfig); err != nil {
		s.logger.Warn("Failed to persist initial agent config to database", "agentName", req.Name, "error", err)
	} else {
		s.logger.Debug("Persisted initial agent config to database", "agentName", req.Name, "environment", lowestEnv, "enableAutoInstrumentation", enableAutoInstrumentation)
	}
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

func toCreateAgentRequest(req *spec.CreateAgentRequest) client.CreateComponentRequest {
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
		Configurations: mapConfigurations(req.Configurations),
		InputInterface: mapInputInterface(req.InputInterface),
	}

	if req.Provisioning.Type == string(utils.InternalAgent) {
		result.AgentType.SubType = utils.StrPointerAsStr(req.AgentType.SubType, "")
	}

	return result
}

func (s *agentManagerService) UpdateAgentBasicInfo(ctx context.Context, orgName string, projectName string, agentName string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error) {
	s.logger.Info("Updating agent basic info", "agentName", agentName, "orgName", orgName, "projectName", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, err
	}

	// Fetch existing agent to validate it exists
	_, err = s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch existing agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
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
		return nil, err
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
		return nil, err
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, err
	}

	// Fetch existing agent to validate immutable fields
	existingAgent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch existing agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
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

	// Handle instrumentation trait attachment/detachment based on enableAutoInstrumentation change
	if err := s.handleInstrumentationUpdate(ctx, orgName, projectName, agentName, existingAgent, req); err != nil {
		s.logger.Error("Failed to update instrumentation", "agentName", agentName, "error", err)
		return nil, fmt.Errorf("failed to update instrumentation: %w", err)
	}

	// Fetch agent to return current state
	updatedAgent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
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
		return nil, err
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, err
	}

	// Validate agent exists
	_, err = s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	// Validate environment if provided
	if environment != "" {
		_, err = s.ocClient.GetEnvironment(ctx, orgName, environment)
		if err != nil {
			s.logger.Error("Failed to validate environment", "environment", environment, "orgName", orgName, "error", err)
			return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
		}
	}

	// Fetch resource configurations from OpenChoreo
	configs, err := s.ocClient.GetComponentResourceConfigs(ctx, orgName, projectName, agentName, environment)
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
		return nil, err
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, err
	}

	// Fetch existing agent to validate it exists
	_, err = s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch existing agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	// Validate environment if provided (for environment-specific updates)
	if environment != "" {
		_, err = s.ocClient.GetEnvironment(ctx, orgName, environment)
		if err != nil {
			s.logger.Error("Failed to validate environment", "environment", environment, "orgName", orgName, "error", err)
			return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
		}
	}

	// Update agent resource configurations in OpenChoreo
	updateReq := buildUpdateResourceConfigsRequest(req)
	if err := s.ocClient.UpdateComponentResourceConfigs(ctx, orgName, projectName, agentName, environment, updateReq); err != nil {
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

	updateReq.Replicas = &req.Replicas

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

	return updateReq
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

	if clientResp.DefaultReplicas != nil {
		response.DefaultReplicas = clientResp.DefaultReplicas
	}

	if clientResp.DefaultResources != nil {
		response.DefaultResources = convertClientResourceConfigToSpec(clientResp.DefaultResources)
	}

	if clientResp.IsDefaultsOverridden != nil {
		response.IsDefaultsOverridden = clientResp.IsDefaultsOverridden
	}

	return response
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
	return client.UpdateComponentBuildParametersRequest{
		Repository:     mapRepository(req.Provisioning.Repository),
		Build:          mapBuildConfig(&req.Build),
		InputInterface: mapInputInterface(&req.InputInterface),
		Configurations: mapConfigurations(req.Configurations),
	}
}

func (s *agentManagerService) GenerateName(ctx context.Context, orgName string, payload spec.ResourceNameRequest) (string, error) {
	s.logger.Info("Generating resource name", "resourceType", payload.ResourceType, "displayName", payload.DisplayName, "orgName", orgName)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return "", err
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
			return "", err
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
		if err != nil && errors.Is(err, utils.ErrProjectNotFound) {
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
		if err != nil && errors.Is(err, utils.ErrProjectNotFound) {
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
		return err
	}
	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgName", orgName, "error", err)
		return err
	}
	// Delete agent component in OpenChoreo
	s.logger.Debug("Deleting oc agent", "agentName", agentName, "orgName", orgName, "projectName", projectName)
	err = s.ocClient.DeleteComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentNotFound) {
			s.logger.Warn("Agent not found during deletion, delete is idempotent", "agentName", agentName, "orgName", orgName, "projectName", projectName)
			// Still cleanup agent configs from database even if agent not found in OpenChoreo
			if configErr := s.agentConfigRepo.DeleteAllByAgent(orgName, projectName, agentName); configErr != nil {
				s.logger.Warn("Failed to delete agent configs from database", "agentName", agentName, "error", configErr)
			}
			return nil
		}
		s.logger.Error("Failed to delete oc agent", "agentName", agentName, "error", err)
		return err
	}

	// Cleanup agent configs from database
	if configErr := s.agentConfigRepo.DeleteAllByAgent(orgName, projectName, agentName); configErr != nil {
		s.logger.Warn("Failed to delete agent configs from database", "agentName", agentName, "error", configErr)
		// Don't fail the deletion - configs will be orphaned but harmless
	}

	s.logger.Debug("Agent deleted from OpenChoreo successfully", "orgName", orgName, "agentName", agentName)
	return nil
}

// BuildAgent triggers a build for an agent.
func (s *agentManagerService) BuildAgent(ctx context.Context, orgName string, projectName string, agentName string, commitId string) (*models.BuildResponse, error) {
	s.logger.Info("Building agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "commitId", commitId)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}

	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return nil, err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("build operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Trigger build in OpenChoreo
	s.logger.Debug("Triggering build in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "commitId", commitId)
	build, err := s.ocClient.TriggerBuild(ctx, orgName, projectName, agentName, commitId)
	if err != nil {
		s.logger.Error("Failed to trigger build in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}
	s.logger.Info("Build triggered successfully", "agentName", agentName, "orgName", orgName, "projectName", projectName, "buildName", build.Name)
	return build, nil
}

// DeployAgent deploys an agent.
func (s *agentManagerService) DeployAgent(ctx context.Context, orgName string, projectName string, agentName string, req *spec.DeployAgentRequest) (string, error) {
	// Log the enableAutoInstrumentation value received in deploy request
	var enableAutoInstrumentationValue string = "nil (not provided)"
	if req.EnableAutoInstrumentation != nil {
		enableAutoInstrumentationValue = fmt.Sprintf("%v", *req.EnableAutoInstrumentation)
	}
	s.logger.Info("Deploying agent", "agentName", agentName, "orgName", orgName, "projectName", projectName, "imageId", req.ImageId, "enableAutoInstrumentation", enableAutoInstrumentationValue)
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return "", err
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return "", err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return "", fmt.Errorf("deploy operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}

	// Convert to deploy request with user-provided env vars
	deployReq := client.DeployRequest{
		ImageID: req.ImageId,
	}
	if len(req.Env) > 0 {
		deployReq.Env = make([]client.EnvVar, len(req.Env))
		for i, env := range req.Env {
			deployReq.Env[i] = client.EnvVar{
				Key:   env.Key,
				Value: env.Value,
			}
		}
	}

	// Generate and add tracing env vars (AMP_OTEL_ENDPOINT and AMP_AGENT_API_KEY) for both
	// Python buildpack and Docker agents. These are added to deployReq.Env so they get
	// applied to the Workload during deploy.
	if agent.Build != nil && (agent.Build.Buildpack != nil || agent.Build.Docker != nil) {
		s.logger.Debug("Generating tracing env vars for deploy", "agentName", agentName)
		tracingEnvVars, err := s.generateTracingEnvVars(ctx, orgName, projectName, agentName)
		if err != nil {
			s.logger.Warn("Failed to generate tracing env vars for deploy", "agentName", agentName, "error", err)
		} else {
			// Append tracing env vars to deploy request (they will overwrite if duplicates exist)
			deployReq.Env = append(deployReq.Env, tracingEnvVars...)
			// Also persist to Component CR for future reference
			if injectErr := s.updateComponentEnvVars(ctx, orgName, projectName, agentName, tracingEnvVars); injectErr != nil {
				s.logger.Warn("Failed to persist tracing env vars to component CR", "agentName", agentName, "error", injectErr)
			}
		}
	}

	// Get deployment pipeline and environment info early (needed for instrumentation config)
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to fetch deployment pipeline", "orgName", orgName, "projectName", projectName, "error", err)
		return "", fmt.Errorf("failed to fetch deployment pipeline: %w", err)
	}
	lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)

	var targetEnv *models.EnvironmentResponse
	if lowestEnv != "" {
		targetEnv, err = s.ocClient.GetEnvironment(ctx, orgName, lowestEnv)
		if err != nil {
			s.logger.Warn("Failed to get environment details", "environment", lowestEnv, "error", err)
		}
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
		existingConfig, configErr := s.agentConfigRepo.Get(orgName, agentName, targetEnv.Name)
		if configErr != nil {
			s.logger.Warn("Failed to read instrumentation config from database", "agentName", agentName, "environment", targetEnv.Name, "error", configErr)
			enableAutoInstrumentation = true // Default to enabled on error
		} else if existingConfig != nil {
			enableAutoInstrumentation = existingConfig.EnableAutoInstrumentation
			s.logger.Debug("Read instrumentation config from database", "agentName", agentName, "environment", targetEnv.Name, "enableAutoInstrumentation", enableAutoInstrumentation)
		} else {
			// No config in DB - this is first deployment, default to true
			enableAutoInstrumentation = true
			s.logger.Debug("No instrumentation config in database, defaulting to enabled", "agentName", agentName, "environment", targetEnv.Name)
		}
	} else {
		enableAutoInstrumentation = true // Default if no environment info available
	}

	// Update instrumentation trait before deploy for Python buildpack builds
	if agent.Build != nil && agent.Build.Buildpack != nil && agent.Build.Buildpack.Language == string(utils.LanguagePython) {
		hasTrait, traitErr := s.ocClient.HasTrait(ctx, orgName, projectName, agentName, client.TraitOTELInstrumentation)
		if traitErr != nil {
			s.logger.Warn("Failed to check instrumentation trait status before deploy", "agentName", agentName, "error", traitErr)
		} else if enableAutoInstrumentation && !hasTrait {
			s.logger.Info("Enabling instrumentation (attaching trait) before deploy", "agentName", agentName)
			if attachErr := s.attachOTELInstrumentationTrait(ctx, orgName, projectName, agentName); attachErr != nil {
				s.logger.Warn("Failed to attach instrumentation trait before deploy", "agentName", agentName, "error", attachErr)
			}
		} else if !enableAutoInstrumentation && hasTrait {
			s.logger.Info("Disabling instrumentation (detaching trait) before deploy", "agentName", agentName)
			if detachErr := s.detachOTELInstrumentationTrait(ctx, orgName, projectName, agentName); detachErr != nil {
				s.logger.Warn("Failed to detach instrumentation trait before deploy", "agentName", agentName, "error", detachErr)
			}
		}
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

func (s *agentManagerService) ListAgentBuilds(ctx context.Context, orgName string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error) {
	s.logger.Info("Listing agent builds", "agentName", agentName, "orgName", orgName, "projectName", projectName, "limit", limit, "offset", offset)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to validate organization", "orgName", orgName, "error", err)
		return nil, 0, fmt.Errorf("failed to find organization %s: %w", orgName, err)
	}

	// Check if component already exists
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch component", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, 0, err
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
		return nil, err
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return nil, err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("build operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Fetch the build from OpenChoreo
	build, err := s.ocClient.GetBuild(ctx, orgName, projectName, agentName, buildName)
	if err != nil {
		s.logger.Error("Failed to get build from OpenChoreo", "buildName", buildName, "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
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
		return nil, err
	}
	project, err := s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, err
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, project.Name, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return nil, err
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

func (s *agentManagerService) GetAgentEndpoints(ctx context.Context, orgName string, projectName string, agentName string, environmentName string) (map[string]models.EndpointsResponse, error) {
	s.logger.Info("Getting agent endpoints", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environmentName)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, err
	}
	project, err := s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, project.Name, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("endpoints are not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Check if environment exists
	_, err = s.ocClient.GetEnvironment(ctx, orgName, environmentName)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environmentName, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
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
		return nil, err
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		s.logger.Warn("Configuration operation not supported for agent type", "agentName", agentName, "provisioningType", agent.Provisioning.Type, "orgName", orgName, "projectName", projectName)
		return nil, fmt.Errorf("configuration operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Check if environment exists
	_, err = s.ocClient.GetEnvironment(ctx, orgName, environment)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environment, "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to get environments for organization %s: %w", orgName, err)
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
		return nil, err
	}
	// Validates the project name by checking its existence
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get OpenChoreo project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}

	// Check if component already exists
	_, err = s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to check component existence", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}

	// Check if build exists
	build, err := s.ocClient.GetBuild(ctx, orgName, projectName, agentName, buildName)
	if err != nil {
		s.logger.Error("Failed to get build", "buildName", buildName, "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
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
		return nil, err
	}
	// Validates the project name by checking its existence
	_, err = s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get OpenChoreo project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}

	// Check if component already exists
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to check component existence", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("runtime logs are not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Fetch environment from open choreo
	environment, err := s.ocClient.GetEnvironment(ctx, orgName, payload.EnvironmentName)
	if err != nil {
		s.logger.Error("Failed to fetch environment from OpenChoreo", "environmentName", payload.EnvironmentName, "orgName", orgName, "error", err)
		return nil, err
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
		return nil, err
	}
	// Validates the project name by checking its existence
	project, err := s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to get OpenChoreo project", "projectName", projectName, "orgName", orgName, "error", err)
		return nil, err
	}
	// Fetch environment from open choreo
	environment, err := s.ocClient.GetEnvironment(ctx, orgName, payload.EnvironmentName)
	if err != nil {
		s.logger.Error("Failed to fetch environment from OpenChoreo", "environmentName", payload.EnvironmentName, "orgName", orgName, "error", err)
		return nil, err
	}
	// Check if component already exists
	agent, err := s.ocClient.GetComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to check component existence", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return nil, err
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
