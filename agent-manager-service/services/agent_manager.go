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
	"regexp"
	"slices"
	"strings"

	"github.com/google/uuid"
	observabilitysvc "github.com/wso2/agent-manager/agent-manager-service/clients/observabilitysvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
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
	GetAgentFileMounts(ctx context.Context, orgName string, projectName string, agentName string, environment string) ([]models.FileMountEntry, error)
	GetBuildLogs(ctx context.Context, orgName string, projectName string, agentName string, buildName string) (*models.LogsResponse, error)
	GenerateName(ctx context.Context, orgName string, payload spec.ResourceNameRequest) (string, error)
	GetAgentMetrics(ctx context.Context, orgName string, projectName string, agentName string, payload spec.MetricsFilterRequest) (*spec.MetricsResponse, error)
	GetAgentRuntimeLogs(ctx context.Context, orgName string, projectName string, agentName string, payload spec.LogFilterRequest) (*models.LogsResponse, error)
	GetAgentResourceConfigs(ctx context.Context, orgName string, projectName string, agentName string, environment string) (*spec.AgentResourceConfigsResponse, error)
	UpdateAgentResourceConfigs(ctx context.Context, orgName string, projectName string, agentName string, environment string, req *spec.UpdateAgentResourceConfigsRequest) (*spec.AgentResourceConfigsResponse, error)
}

type agentManagerService struct {
	db                        *gorm.DB
	ocClient                  client.OpenChoreoClient
	observabilitySvcClient    observabilitysvc.ObservabilitySvcClient
	secretMgmtClient          secretmanagersvc.SecretManagementClient
	gitRepositoryService      RepositoryService
	tokenManagerService       AgentTokenManagerService
	agentConfigRepo           repositories.AgentConfigRepository
	agentConfigurationService AgentConfigurationService
	agentKindService          AgentKindService
	artifactRepo              repositories.ArtifactRepository
	logger                    *slog.Logger
}

func NewAgentManagerService(
	db *gorm.DB,
	OpenChoreoClient client.OpenChoreoClient,
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient,
	secretMgmtClient secretmanagersvc.SecretManagementClient,
	gitRepositoryService RepositoryService,
	tokenManagerService AgentTokenManagerService,
	agentConfigRepo repositories.AgentConfigRepository,
	agentConfigurationService AgentConfigurationService,
	agentKindService AgentKindService,
	artifactRepo repositories.ArtifactRepository,
	logger *slog.Logger,
) AgentManagerService {
	return &agentManagerService{
		db:                        db,
		ocClient:                  OpenChoreoClient,
		observabilitySvcClient:    observabilitySvcClient,
		secretMgmtClient:          secretMgmtClient,
		gitRepositoryService:      gitRepositoryService,
		tokenManagerService:       tokenManagerService,
		agentConfigRepo:           agentConfigRepo,
		agentConfigurationService: agentConfigurationService,
		agentKindService:          agentKindService,
		artifactRepo:              artifactRepo,
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
// handling secret env vars and file mounts by using secretKeyRef pointing to the K8s Secret created by SecretReference
func mapConfigurationsWithSecrets(specConfigs *spec.Configurations, secretReference string) *client.Configurations {
	if specConfigs == nil || (len(specConfigs.Env) == 0 && len(specConfigs.Files) == 0) {
		return nil
	}

	configs := &client.Configurations{}

	if len(specConfigs.Env) > 0 {
		configs.Env = make([]client.EnvVar, len(specConfigs.Env))
		for i, env := range specConfigs.Env {
			if env.GetIsSensitive() {
				configs.Env[i] = client.EnvVar{
					Key: env.Key,
					ValueFrom: &client.EnvVarValueFrom{
						SecretKeyRef: &client.SecretKeyRef{
							Name: secretReference,
							Key:  env.Key,
						},
					},
				}
			} else {
				configs.Env[i] = client.EnvVar{Key: env.Key, Value: env.GetValue()}
			}
		}
	}

	if len(specConfigs.Files) > 0 {
		configs.Files = make([]client.FileVar, len(specConfigs.Files))
		for i, f := range specConfigs.Files {
			if f.GetIsSensitive() {
				configs.Files[i] = client.FileVar{
					Key:       f.Key,
					MountPath: f.MountPath,
					ValueFrom: &client.EnvVarValueFrom{
						SecretKeyRef: &client.SecretKeyRef{
							Name: secretReference,
							Key:  f.Key,
						},
					},
				}
			} else {
				configs.Files[i] = client.FileVar{Key: f.Key, MountPath: f.MountPath, Value: f.GetValue()}
			}
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
// artifactID is the UUID of the agent's artifact record (used for api-configuration trait).
func (s *agentManagerService) buildCreateTraitRequests(ctx context.Context, orgName, projectName, artifactID string, req *spec.CreateAgentRequest) ([]client.TraitRequest, error) {
	var traits []client.TraitRequest

	// Determine instrumentation trait
	autoInstrumentation := req.Configurations == nil || req.Configurations.EnableAutoInstrumentation == nil || *req.Configurations.EnableAutoInstrumentation
	isAPIAgent := req.AgentType != nil && req.AgentType.Type == string(utils.AgentTypeAPI)
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
			lv := req.Build.BuildpackBuild.Buildpack.GetLanguageVersion()
			otelOpts := []client.TraitOption{client.WithAgentApiKey(apiKey), client.WithLanguageVersion(lv)}
			if req.Configurations != nil {
				if v := req.Configurations.InstrumentationVersion.Get(); v != nil {
					otelOpts = append(otelOpts, client.WithInstrumentationVersion(v))
				}
			}
			traits = append(traits, client.TraitRequest{
				TraitKind: client.TraitKindTrait,
				TraitType: client.TraitOTELInstrumentation,
				Opts:      otelOpts,
			})
		} else {
			traits = append(traits, client.TraitRequest{TraitKind: client.TraitKindTrait, TraitType: client.TraitEnvInjection, Opts: []client.TraitOption{client.WithAgentApiKey(apiKey)}})
		}
	}

	// Attach api-configuration trait at create time so the RestApi CRD is provisioned immediately.
	// API key security and CORS are enabled by default; deploy time upserts with the actual policy setting.
	if isAPIAgent {
		port := config.GetConfig().DefaultChatAPI.DefaultHTTPPort
		basePath := config.GetConfig().DefaultChatAPI.DefaultBasePath
		if req.InputInterface != nil && req.InputInterface.Port != nil && *req.InputInterface.Port > 0 {
			port = *req.InputInterface.Port
		}
		if req.InputInterface != nil && req.InputInterface.BasePath != nil && *req.InputInterface.BasePath != "" {
			basePath = *req.InputInterface.BasePath
		}
		corsConfig := config.GetAgentWorkloadConfig().CORS
		createPolicies := []map[string]interface{}{
			client.CORSPolicy(
				strings.Split(corsConfig.AllowOrigin, ","),
				strings.Split(corsConfig.AllowMethods, ","),
				strings.Split(corsConfig.AllowHeaders, ","),
				false, // allowCredentials defaults to false at agent creation
			),
			client.APIKeyAuthPolicy(),
		}
		traits = append(traits, client.TraitRequest{
			TraitKind: client.TraitKindTrait,
			TraitType: client.TraitAPIManagement,
			Opts: []client.TraitOption{
				client.WithArtifactID(artifactID),
				client.WithUpstreamPort(port),
				client.WithUpstreamBasePath(basePath),
				client.WithPolicies(createPolicies),
			},
		})
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

	opts := []client.TraitOption{client.WithAgentApiKey(apiKey)}
	// Honor the agent's pinned AMP instrumentation version if one is set;
	// otherwise the trait builder falls back to the platform default. Surface
	// real lookup errors so a transient DB hiccup doesn't silently break the pin.
	v, err := s.lookupAgentInstrumentationVersion(ctx, orgName, projectName, agentName)
	switch {
	case errors.Is(err, ErrInstrumentationVersionNotPinned):
		// no pin → fall through to the platform default in the trait builder
	case err != nil:
		return fmt.Errorf("looking up pinned instrumentation version: %w", err)
	default:
		opts = append(opts, client.WithInstrumentationVersion(v))
	}

	if err := s.ocClient.AttachTraits(ctx, orgName, projectName, agentName, []client.TraitRequest{
		{TraitKind: client.TraitKindTrait, TraitType: client.TraitOTELInstrumentation, Opts: opts},
	}); err != nil {
		return fmt.Errorf("error attaching OTEL instrumentation trait: %w", err)
	}

	s.logger.Info("Enabled instrumentation for buildpack agent", "agentName", agentName)
	return nil
}

// ErrInstrumentationVersionNotPinned indicates an agent has no pinned AMP
// instrumentation version: no row in agent_configs yet, no project pipeline,
// or the column is NULL. Callers should treat it as "fall back to the platform
// default" rather than a real error. It is intentionally distinct from real
// errors (DB read failures, deployment pipeline lookup failures) so a transient
// failure can't silently swap a customer's pinned version for the default.
var ErrInstrumentationVersionNotPinned = errors.New("agent has no pinned instrumentation version")

// lookupAgentInstrumentationVersion returns the agent's pinned AMP instrumentation
// version (from agent_configs.instrumentation_version). It returns
// ErrInstrumentationVersionNotPinned when there's genuinely no pin to honour,
// and a wrapped real error for transient failures.
func (s *agentManagerService) lookupAgentInstrumentationVersion(ctx context.Context, orgName, projectName, agentName string) (*string, error) {
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}
	if len(pipeline.PromotionPaths) == 0 {
		return nil, ErrInstrumentationVersionNotPinned
	}
	lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
	if lowestEnv == "" {
		return nil, ErrInstrumentationVersionNotPinned
	}
	cfg, err := s.agentConfigRepo.Get(orgName, projectName, agentName, lowestEnv)
	if errors.Is(err, repositories.ErrAgentConfigNotFound) {
		return nil, ErrInstrumentationVersionNotPinned
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read agent config: %w", err)
	}
	if cfg == nil || cfg.InstrumentationVersion == nil {
		return nil, ErrInstrumentationVersionNotPinned
	}
	return cfg.InstrumentationVersion, nil
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
		{TraitKind: client.TraitKindTrait, TraitType: client.TraitEnvInjection, Opts: []client.TraitOption{client.WithAgentApiKey(apiKey)}},
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

// validateInstrumentationVersion checks the AMP instrumentation version against
// the deployment's supported set. Empty / unsupported values return ErrInvalidInput.
func (s *agentManagerService) validateInstrumentationVersion(version string) error {
	supported := config.GetConfig().OTEL.SupportedInstrumentationVersions
	if slices.Contains(supported, version) {
		return nil
	}
	return fmt.Errorf("%w: instrumentationVersion %q is not supported by this deployment; supported: %v", utils.ErrInvalidInput, version, supported)
}

// persistInstrumentationConfig saves the instrumentation config to the database.
// instrumentationVersion is nil when the caller did not pin a specific version —
// the column stays NULL and the resolver falls back to the platform default.
func (s *agentManagerService) persistInstrumentationConfig(ctx context.Context, orgName, projectName, agentName string, enableAutoInstrumentation bool, instrumentationVersion *string) {
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

	defaultCORS := config.GetAgentWorkloadConfig().CORS
	agentConfig := &models.AgentConfig{
		OrgName:                   orgName,
		ProjectName:               projectName,
		AgentName:                 agentName,
		EnvironmentName:           targetEnv.Name,
		EnableAutoInstrumentation: enableAutoInstrumentation,
		InstrumentationVersion:    instrumentationVersion,
		EnableApiKeySecurity:      true,
		CORSEnabled:               true,
		CORSAllowOrigins:          strings.Split(defaultCORS.AllowOrigin, ","),
		CORSAllowMethods:          strings.Split(defaultCORS.AllowMethods, ","),
		CORSAllowHeaders:          strings.Split(defaultCORS.AllowHeaders, ","),
		CORSAllowCredentials:      false,
	}

	if err := s.agentConfigRepo.Upsert(agentConfig); err != nil {
		s.logger.Warn("Failed to persist instrumentation config to database", "agentName", agentName, "error", err)
	} else {
		s.logger.Debug("Persisted instrumentation config to database", "agentName", agentName, "environment", lowestEnv, "enableAutoInstrumentation", enableAutoInstrumentation, "instrumentationVersion", instrumentationVersion)
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

	// Extract OrgId from the caller's JWT claims
	callerClaims := jwtassertion.GetTokenClaims(ctx)
	if callerClaims == nil || callerClaims.OuId == "" {
		s.logger.Error("GenerateToken: missing organization identity in caller token")
		return "", utils.ErrForbidden
	}
	// Generate agent API key using token manager service with 1 year expiry
	tokenReq := GenerateTokenRequest{
		OrgName:     orgName,
		ProjectName: projectName,
		AgentName:   agentName,
		Environment: firstEnvName,
		ExpiresIn:   "8760h", // 1 year (365 days * 24 hours)
		OrgId:       callerClaims.OuId,
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

	s.logger.Info(
		"Injected tracing environment variables",
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

	s.logger.Info(
		"Successfully updated component environment variables",
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

	// Populate per-environment agent configuration from database
	// Get the first/lowest environment to read the config
	pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if pipelineErr == nil && len(pipeline.PromotionPaths) > 0 {
		lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
		if lowestEnv != "" {
			agentConfig, configErr := s.agentConfigRepo.Get(orgName, projectName, agentName, lowestEnv)
			if errors.Is(configErr, repositories.ErrAgentConfigNotFound) {
				// No config in DB - use defaults for display purposes
				defaultEnabled := true
				defaultCORSEnabled := true
				defaultAllowCredentials := false
				defCORS := config.GetAgentWorkloadConfig().CORS
				agent.Configurations = &models.Configurations{
					EnableAutoInstrumentation: &defaultEnabled,
					EnableApiKeySecurity:      &defaultEnabled,
					CorsConfig: &models.CorsConfig{
						Enabled:          &defaultCORSEnabled,
						AllowOrigin:      strings.Split(defCORS.AllowOrigin, ","),
						AllowMethods:     strings.Split(defCORS.AllowMethods, ","),
						AllowHeaders:     strings.Split(defCORS.AllowHeaders, ","),
						AllowCredentials: &defaultAllowCredentials,
					},
				}
			} else if configErr != nil {
				s.logger.Warn("Failed to read agent config from database", "agentName", agentName, "environment", lowestEnv, "error", configErr)
			} else {
				agent.Configurations = &models.Configurations{
					EnableAutoInstrumentation: &agentConfig.EnableAutoInstrumentation,
					InstrumentationVersion:    agentConfig.InstrumentationVersion,
					EnableApiKeySecurity:      &agentConfig.EnableApiKeySecurity,
					CorsConfig: &models.CorsConfig{
						Enabled:          &agentConfig.CORSEnabled,
						AllowOrigin:      agentConfig.CORSAllowOrigins,
						AllowMethods:     agentConfig.CORSAllowMethods,
						AllowHeaders:     agentConfig.CORSAllowHeaders,
						AllowCredentials: &agentConfig.CORSAllowCredentials,
					},
				}
			}

			// Populate env vars for internal agents (non-fatal if it fails)
			if agent.Provisioning.Type == string(utils.InternalAgent) {
				if envConfigs, envErr := s.ocClient.GetComponentConfigurations(ctx, orgName, projectName, agentName, lowestEnv); envErr == nil {
					var envVars []models.EnvVars
					for _, ev := range envConfigs {
						if _, isSystem := client.SystemInjectedEnvVars[ev.Key]; !isSystem {
							envVars = append(envVars, ev)
						}
					}
					if agent.Configurations != nil {
						agent.Configurations.Env = envVars
					} else {
						agent.Configurations = &models.Configurations{Env: envVars}
					}
				}
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
	if req.Configurations != nil {
		if v := req.Configurations.InstrumentationVersion.Get(); v != nil {
			if err := s.validateInstrumentationVersion(*v); err != nil {
				return err
			}
		}
	}

	imageID := ""
	if req.Provisioning.AgentKind != nil {
		kindVersion, err := s.agentKindService.GetKindVersion(ctx, orgName, req.Provisioning.AgentKind.Name, req.Provisioning.AgentKind.Version)
		if err != nil {
			return err
		}
		var envVars []spec.EnvironmentVariable
		if req.Configurations != nil {
			envVars = req.Configurations.Env
		}
		if err := ValidateKindConfigValues(kindVersion.ConfigSchema, envVars); err != nil {
			return err
		}
		if kindVersion.ImageId == "" {
			return fmt.Errorf("kind version %q has no stored image; re-publish the kind from a successfully built agent", req.Provisioning.AgentKind.Version)
		}
		sourceComponent, err := s.ocClient.GetComponent(ctx, orgName, kindVersion.Kind.ProjectName, kindVersion.Kind.AgentName)
		if err != nil {
			s.logger.Error("Failed to get source component for kind version", "agentName", kindVersion.Kind.AgentName, "error", err)
			return fmt.Errorf("failed to resolve kind version source: %w", err)
		}
		subType := sourceComponent.Type.SubType
		req.AgentType = &spec.AgentType{
			Type:    sourceComponent.Type.Type,
			SubType: &subType,
		}
		req.Build = modelBuildToSpecBuild(sourceComponent.Build)
		if sourceComponent.InputInterface != nil {
			port := sourceComponent.InputInterface.Port
			basePath := sourceComponent.InputInterface.BasePath
			req.InputInterface = &spec.InputInterface{
				Type:     sourceComponent.InputInterface.Type,
				Port:     &port,
				BasePath: &basePath,
			}
			if sourceComponent.InputInterface.Schema != nil && sourceComponent.InputInterface.Schema.Path != "" {
				req.InputInterface.Schema = &spec.InputInterfaceSchema{Path: sourceComponent.InputInterface.Schema.Path}
			}
		}
		imageID = kindVersion.ImageId
	}
	return s.createComponentAgent(ctx, orgName, projectName, req, imageID)
}

// createComponentAgent is the shared agent creation flow for all internal agents.
// For source-based (imageID == ""): CreateComponent (with Workflow) → AttachTraits → TriggerBuild
// For kind-based (imageID != ""): CreateComponent (no Workflow) → AttachTraits → CreateInternalAgentFromKindWorkload
func (s *agentManagerService) createComponentAgent(ctx context.Context, orgName, projectName string, req *spec.CreateAgentRequest, imageID string) error {
	s.logger.Info("Creating agent", "agentName", req.Name, "orgName", orgName, "projectName", projectName, "provisioningType", req.Provisioning.Type)

	_, err := s.ocClient.GetOrganization(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return translateOrgError(err)
	}

	if req.Provisioning.Repository != nil && req.Provisioning.Repository.SecretRef.Get() != nil {
		if err := s.validateGitSecretExists(ctx, orgName, req.Provisioning.Repository.GetSecretRef()); err != nil {
			return err
		}
	}

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

	secretLocation := secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     projectName,
		EnvironmentName: firstEnv,
		EntityName:      req.Name,
	}

	hasSecrets := false
	if req.Configurations != nil {
		for _, env := range req.Configurations.Env {
			if env.GetIsSensitive() {
				hasSecrets = true
				break
			}
		}
		if !hasSecrets {
			for _, f := range req.Configurations.Files {
				if f.GetIsSensitive() {
					hasSecrets = true
					break
				}
			}
		}
	}

	secretReference := ""
	if hasSecrets {
		// Collect all secret data from both env vars and files
		allSecretVars := req.Configurations.Env
		for _, f := range req.Configurations.Files {
			if f.GetIsSensitive() {
				// Wrap file mount as an EnvironmentVariable for secret storage (same KV path)
				ev := spec.EnvironmentVariable{Key: f.Key}
				ev.SetValue(f.GetValue())
				ev.SetIsSensitive(true)
				allSecretVars = append(allSecretVars, ev)
			}
		}
		secretReference, err = s.saveSecretsAndCreateReference(ctx, secretLocation, allSecretVars)
		if err != nil {
			s.logger.Error("Failed to save secrets and create SecretReference for agent", "agentName", req.Name, "error", err)
			s.cleanupSecretsOnRollback(ctx, secretLocation)
			return err
		}
	}

	createAgentReq := s.toCreateAgentRequestWithSecrets(req, secretReference)
	if err := s.ocClient.CreateComponent(ctx, orgName, projectName, createAgentReq); err != nil {
		s.logger.Error("Failed to create agent component", "agentName", req.Name, "error", err)
		if hasSecrets {
			s.cleanupSecretsOnRollback(ctx, secretLocation)
		}
		return err
	}

	var agentAPIArtifact *models.Artifact
	if req.AgentType.Type == string(utils.AgentTypeAPI) {
		firstEnvDetails, envErr := s.ocClient.GetEnvironment(ctx, orgName, firstEnv)
		if envErr != nil {
			s.logger.Error("Failed to get environment details", "environment", firstEnv, "error", envErr)
			if hasSecrets {
				s.cleanupSecretsOnRollback(ctx, secretLocation)
			}
			if errDeletion := s.ocClient.DeleteComponent(ctx, orgName, projectName, req.Name); errDeletion != nil {
				s.logger.Error("Failed to rollback agent component after environment lookup failure", "agentName", req.Name, "error", errDeletion)
			}
			return translateEnvironmentError(envErr)
		}
		agentAPIArtifact, err = ensureAgentEnvAPIArtifact(s.db, s.artifactRepo, orgName, projectName, req.Name, firstEnvDetails.UUID)
		if err != nil {
			s.logger.Error("Failed to create agent API artifact record", "agentName", req.Name, "environment", firstEnv, "environmentUUID", firstEnvDetails.UUID, "error", err)
			if hasSecrets {
				s.cleanupSecretsOnRollback(ctx, secretLocation)
			}
			if errDeletion := s.ocClient.DeleteComponent(ctx, orgName, projectName, req.Name); errDeletion != nil {
				s.logger.Error("Failed to rollback agent component after API artifact create failure", "agentName", req.Name, "error", errDeletion)
			}
			return fmt.Errorf("failed to create agent API artifact record: %w", err)
		}
	}

	rollbackAgentCreate := func(reason string) {
		if hasSecrets {
			s.cleanupSecretsOnRollback(ctx, secretLocation)
		}
		if errDeletion := s.ocClient.DeleteComponent(ctx, orgName, projectName, req.Name); errDeletion != nil {
			s.logger.Error("Failed to rollback agent component", "agentName", req.Name, "reason", reason, "error", errDeletion)
		}
		if agentAPIArtifact != nil {
			if errDeletion := s.artifactRepo.Delete(s.db, agentAPIArtifact.UUID.String()); errDeletion != nil {
				s.logger.Error("Failed to rollback agent API artifact record", "agentName", req.Name, "reason", reason, "error", errDeletion)
			}
		}
	}

	// Create LLM configurations (applies to both internal and external agents)
	if len(req.ModelConfig) > 0 {
		if err := s.createAgentLLMConfigs(ctx, orgName, projectName, req); err != nil {
			s.logger.Error("Failed to create LLM configurations for agent", "agentName", req.Name, "error", err)
			rollbackAgentCreate("LLM config failure")
			return err
		}
	}

	isFromKind := imageID != ""
	isInternal := req.Provisioning.Type == string(utils.InternalAgent)

	if isInternal {
		s.logger.Debug("Component created successfully", "agentName", req.Name)

		// Build all traits to attach in a single GET-UPDATE cycle to avoid resource version conflicts
		artifactID := ""
		if agentAPIArtifact != nil {
			artifactID = agentAPIArtifact.UUID.String()
		}
		traitRequests, err := s.buildCreateTraitRequests(ctx, orgName, projectName, artifactID, req)
		if err != nil {
			s.logger.Error("Failed to build trait requests", "agentName", req.Name, "error", err)
			rollbackAgentCreate("trait build failure")
			return err
		}

		if len(traitRequests) > 0 {
			if err := s.ocClient.AttachTraits(ctx, orgName, projectName, req.Name, traitRequests); err != nil {
				s.logger.Error("Failed to attach traits", "agentName", req.Name, "error", err)
				rollbackAgentCreate("trait attachment failure")
				return err
			}
			s.logger.Info("Attached traits", "agentName", req.Name, "count", len(traitRequests))
		}

		if isFromKind {
			var kindEnvVars []client.EnvVar
			var kindFileVars []client.FileVar
			if createAgentReq.Configurations != nil {
				kindEnvVars = createAgentReq.Configurations.Env
				kindFileVars = createAgentReq.Configurations.Files
			}
			kindEndpoints := inputInterfaceToEndpoints(createAgentReq.InputInterface, req.Name)
			if err := s.ocClient.CreateInternalAgentFromKindWorkload(ctx, orgName, projectName, req.Name, client.InternalAgentFromKindWorkloadRequest{
				ImageID:   imageID,
				Endpoints: kindEndpoints,
				Env:       kindEnvVars,
				Files:     kindFileVars,
			}); err != nil {
				s.logger.Error("Failed to create internal-agent-from-kind workload", "agentName", req.Name, "error", err)
				if hasSecrets {
					s.cleanupSecretsOnRollback(ctx, secretLocation)
				}
				if errDeletion := s.ocClient.DeleteComponent(ctx, orgName, projectName, req.Name); errDeletion != nil {
					s.logger.Error("Failed to rollback agent creation after kind-workload failure", "agentName", req.Name, "error", errDeletion)
				}
				return err
			}
			s.logger.Info("Created internal-agent-from-kind workload", "agentName", req.Name)
		} else {
			if err := s.triggerInitialBuild(ctx, orgName, projectName, req); err != nil {
				s.logger.Warn("Failed to trigger initial build for agent, build can be triggered manually", "agentName", req.Name, "error", err)
			} else {
				s.logger.Debug("Triggered initial build for agent", "agentName", req.Name)
			}
		}

		enableAutoInstrumentation := true
		var instrumentationVersion *string
		if req.Configurations != nil {
			if req.Configurations.EnableAutoInstrumentation != nil {
				enableAutoInstrumentation = *req.Configurations.EnableAutoInstrumentation
			}
			instrumentationVersion = req.Configurations.InstrumentationVersion.Get()
		}
		s.persistInstrumentationConfig(ctx, orgName, projectName, req.Name, enableAutoInstrumentation, instrumentationVersion)
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

func (s *agentManagerService) createAgentLLMConfigs(
	ctx context.Context, orgName, projectName string, req *spec.CreateAgentRequest,
) error {
	for i, mc := range req.ModelConfig {
		configName := fmt.Sprintf("%s-llm-config", req.Name)
		if len(req.ModelConfig) > 1 {
			configName = fmt.Sprintf("%s-llm-config-%d", req.Name, i+1)
		}
		createReq := models.CreateAgentModelConfigRequest{
			Name:                 configName,
			Type:                 "llm",
			EnvMappings:          convertEnvMappings(mc.EnvMappings),
			EnvironmentVariables: convertEnvVars(mc.EnvironmentVariables),
		}
		if _, err := s.agentConfigurationService.Create(ctx, orgName, projectName, req.Name, createReq, "system"); err != nil {
			return fmt.Errorf("failed to create LLM configuration %d: %w", i+1, err)
		}
	}
	return nil
}

func convertEnvMappings(specMappings map[string]spec.EnvModelConfigRequest) map[string]models.EnvModelConfigRequest {
	result := make(map[string]models.EnvModelConfigRequest, len(specMappings))
	for env, m := range specMappings {
		policies := make([]models.LLMPolicy, 0, len(m.Configuration.Policies))
		for _, p := range m.Configuration.Policies {
			paths := make([]models.LLMPolicyPath, 0, len(p.Paths))
			for _, pp := range p.Paths {
				paths = append(paths, models.LLMPolicyPath{
					Path:    pp.Path,
					Methods: pp.Methods,
					Params:  pp.Params,
				})
			}
			policies = append(policies, models.LLMPolicy{
				Name:    p.Name,
				Version: p.Version,
				Paths:   paths,
			})
		}
		result[env] = models.EnvModelConfigRequest{
			ProviderName:  m.ProviderName,
			Configuration: models.EnvProviderConfiguration{Policies: policies},
		}
	}
	return result
}

func convertEnvVars(specVars []spec.EnvironmentVariableConfig) []models.EnvironmentVariableConfig {
	result := make([]models.EnvironmentVariableConfig, 0, len(specVars))
	for _, v := range specVars {
		result = append(result, models.EnvironmentVariableConfig{Name: v.Name, Key: v.Key})
	}
	return result
}

// toCreateAgentRequestWithSecrets creates a component request, handling secrets by using secretKeyRef
func (s *agentManagerService) toCreateAgentRequestWithSecrets(req *spec.CreateAgentRequest, secretReferences string) client.CreateComponentRequest {
	agentType := client.AgentTypeConfig{}
	if req.AgentType != nil {
		agentType.Type = req.AgentType.Type
		agentType.SubType = utils.StrPointerAsStr(req.AgentType.SubType, "")
	}

	var agentKindRef *client.AgentKindRef
	if req.Provisioning.AgentKind != nil {
		agentKindRef = &client.AgentKindRef{
			Name:    req.Provisioning.AgentKind.Name,
			Version: req.Provisioning.AgentKind.Version,
		}
	}

	result := client.CreateComponentRequest{
		Name:             req.Name,
		DisplayName:      req.DisplayName,
		Description:      utils.StrPointerAsStr(req.Description, ""),
		ProvisioningType: client.ProvisioningType(req.Provisioning.Type),
		AgentType:        agentType,
		Repository:       mapRepository(req.Provisioning.Repository),
		AgentKind:        agentKindRef,
		Build:            mapBuildConfig(req.Build),
		InputInterface:   mapInputInterface(req.InputInterface),
	}

	result.Configurations = mapConfigurationsWithSecrets(req.Configurations, secretReferences)

	return result
}

// saveSecretsAndCreateReference handles storing secrets in OpenBao and creating SecretReference CR
func (s *agentManagerService) saveSecretsAndCreateReference(
	ctx context.Context,
	location secretmanagersvc.SecretLocation,
	envVars []spec.EnvironmentVariable,
) (string, error) {
	if s.secretMgmtClient == nil {
		return "", fmt.Errorf("secret management is not initialized but secret env vars were provided")
	}

	// Collect secret data
	secretData := make(map[string]string)
	for _, env := range envVars {
		if env.GetIsSensitive() {
			secretData[env.Key] = env.GetValue()
		}
	}

	if len(secretData) == 0 {
		return "", nil
	}

	// Store secrets in KV via secretmanagersvc client
	// SecretReference creation is handled internally by the client when ocClient is configured
	kvPath, err := location.KVPath()
	if err != nil {
		return "", fmt.Errorf("invalid secret location: %w", err)
	}
	s.logger.Debug("Storing secrets in KV", "kvPath", kvPath, "secretRefName", location.SecretRefName(), "secretCount", len(secretData))
	secretRef, createErr := s.secretMgmtClient.CreateSecret(ctx, location, secretData)
	if createErr != nil {
		if errors.Is(createErr, secretmanagersvc.ErrNotManaged) {
			return "", fmt.Errorf("secret path %q is already owned by another system and cannot be overwritten; manual cleanup may be required: %w", kvPath, utils.ErrSecretPathConflict)
		}
		return "", fmt.Errorf("failed to store secrets in KV: %w", createErr)
	}

	s.logger.Info("Secrets stored and SecretReference created", "kvPath", kvPath, "secretCount", len(secretData))
	return secretRef, nil
}

// cleanupSecretsOnRollback removes secrets from KV and deletes SecretReference CR during rollback.
// This is a best-effort cleanup - errors are logged but not returned since we're already handling a failure.
func (s *agentManagerService) cleanupSecretsOnRollback(ctx context.Context, location secretmanagersvc.SecretLocation) {
	// Delete secrets from KV and SecretReference
	if s.secretMgmtClient != nil {
		kvPath, _ := location.KVPath()
		if err := s.secretMgmtClient.DeleteSecret(ctx, location, location.SecretRefName()); err != nil {
			s.logger.Warn("Failed to cleanup secrets during rollback", "kvPath", kvPath, "error", err)
		} else {
			s.logger.Debug("Cleaned up secrets during rollback", "kvPath", kvPath)
		}
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
		s.cleanupSecretReference(ctx, orgName, projectName, agentName, secretRefName)
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
			s.deleteAgentAPIArtifact(ctx, orgName, projectName, agentName)
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

	// Cleanup env-scoped API artifact record.
	s.deleteAgentAPIArtifact(ctx, orgName, projectName, agentName)

	s.logger.Debug("Agent deleted from OpenChoreo successfully", "orgName", orgName, "agentName", agentName)
	return nil
}

func (s *agentManagerService) deleteAgentAPIArtifact(ctx context.Context, orgName, projectName, agentName string) {
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, orgName, projectName)
	if err != nil {
		s.logger.Warn("Failed to get deployment pipeline for agent API artifact cleanup", "agentName", agentName, "error", err)
		return
	}
	environmentName := findLowestEnvironment(pipeline.PromotionPaths)
	if environmentName == "" {
		return
	}
	environment, err := s.ocClient.GetEnvironment(ctx, orgName, environmentName)
	if err != nil {
		s.logger.Warn("Failed to get environment for agent API artifact cleanup", "agentName", agentName, "environment", environmentName, "error", err)
		return
	}
	artifact, err := s.artifactRepo.GetByHandle(agentEnvAPIArtifactHandle(projectName, agentName, environment.UUID), orgName)
	if err != nil {
		return
	}
	if delErr := s.artifactRepo.Delete(s.db, artifact.UUID.String()); delErr != nil {
		s.logger.Warn("Failed to delete agent API artifact record", "agentName", agentName, "environment", environmentName, "environmentUUID", environment.UUID, "error", delErr)
	}
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

// cleanupSecretReference deletes secrets from KV and the SecretReference CR.
// It retrieves the SecretReference to get the actual KV path, parses it to a location,
// then calls DeleteSecret which handles both KV and SecretReference deletion.
func (s *agentManagerService) cleanupSecretReference(ctx context.Context, orgName, projectName, agentName, secretRefName string) {
	if s.secretMgmtClient == nil {
		s.logger.Warn("Secret management client not configured, skipping secret cleanup", "secretRefName", secretRefName)
		return
	}

	// Get the SecretReference to find the actual KV path
	secretRefInfo, err := s.ocClient.GetSecretReference(ctx, orgName, secretRefName)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			s.logger.Debug("SecretReference not found, skipping cleanup", "secretRefName", secretRefName)
			return
		}
		s.logger.Warn("Failed to get SecretReference, skipping cleanup", "secretRefName", secretRefName, "error", err)
		return
	}

	if len(secretRefInfo.Data) == 0 {
		s.logger.Warn("SecretReference has no data sources, skipping cleanup", "secretRefName", secretRefName)
		return
	}

	// Parse the KV path to get the correct location
	kvPath := secretRefInfo.Data[0].RemoteRef.Key
	if kvPath == "" {
		s.logger.Warn("SecretReference has empty KV path, skipping cleanup", "secretRefName", secretRefName)
		return
	}

	location, parseErr := secretmanagersvc.ParseKVPath(kvPath)
	if parseErr != nil {
		s.logger.Warn("Failed to parse KV path from SecretReference, skipping cleanup",
			"kvPath", kvPath, "secretRefName", secretRefName, "error", parseErr)
		return
	}

	// DeleteSecret handles both KV deletion and SecretReference CR deletion
	if err := s.secretMgmtClient.DeleteSecret(ctx, location, secretRefName); err != nil {
		s.logger.Warn("Failed to delete secret during cleanup",
			"kvPath", kvPath, "secretRefName", secretRefName, "error", err)
	} else {
		s.logger.Debug("Deleted secret during cleanup", "kvPath", kvPath, "secretRefName", secretRefName)
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
	if agent.KindName != "" {
		return nil, fmt.Errorf("build operation is not supported for kind-sourced agents")
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
		ImageID:     req.ImageId,
		Environment: lowestEnv,
	}

	// Log deploy request env var details for debugging
	s.logger.Debug("Deploy request env vars from client",
		"agentName", agentName, "requestEnvCount", len(req.Env))
	for i, env := range req.Env {
		s.logger.Debug("Deploy request env var",
			"index", i, "key", env.Key,
			"isSensitive", env.GetIsSensitive(),
			"hasValue", env.GetValue() != "",
			"hasSecretRef", env.HasSecretRef(),
			"secretRef", env.GetSecretRef())
	}

	// Fetch system-managed env vars (e.g., LLM provider config) from the existing Component CR /
	// ReleaseBinding. These are managed by the configuration service and must be preserved because
	// both ReplaceComponentEnvVars and Deploy() overwrite all env vars.
	// We fetch these FIRST so we can filter them out of req.Env before processEnvVars, which would
	// otherwise mangle their SecretKeyRef.Key (using env var name instead of the original secret key).
	systemManagedEnvVars, systemManagedKeys, sysEnvErr := s.getSystemManagedEnvVars(ctx, orgName, projectName, lowestEnv, agentName)
	if sysEnvErr != nil {
		s.logger.Error("Failed to fetch system-managed env vars, aborting deploy to prevent data loss",
			"agentName", agentName, "orgName", orgName, "projectName", projectName, "error", sysEnvErr)
		return "", fmt.Errorf("failed to fetch system-managed env vars for agent %s: %w", agentName, sysEnvErr)
	}
	if len(systemManagedEnvVars) > 0 {
		s.logger.Info("Preserving system-managed env vars during deploy", "agentName", agentName, "count", len(systemManagedEnvVars))
		for _, sysEnv := range systemManagedEnvVars {
			if sysEnv.ValueFrom != nil && sysEnv.ValueFrom.SecretKeyRef != nil {
				s.logger.Debug("System-managed secret env var preserved",
					"envKey", sysEnv.Key,
					"secretRefName", sysEnv.ValueFrom.SecretKeyRef.Name,
					"secretKey", sysEnv.ValueFrom.SecretKeyRef.Key)
			} else {
				s.logger.Debug("System-managed plain env var preserved", "envKey", sysEnv.Key)
			}
		}
	} else {
		s.logger.Debug("No system-managed env vars to preserve", "agentName", agentName)
	}

	// Filter out system-managed env vars from the deploy request before processEnvVars.
	// The frontend may include these (e.g., LLM config API key) in req.Env because it reads
	// all configurations. processEnvVars would mangle their SecretKeyRef.Key, so we handle
	// them separately via getSystemManagedEnvVars which preserves the original secret key.
	userEnv := req.Env
	if len(systemManagedKeys) > 0 {
		userEnv = make([]spec.EnvironmentVariable, 0, len(req.Env))
		for _, env := range req.Env {
			if !systemManagedKeys[env.Key] {
				userEnv = append(userEnv, env)
			} else {
				s.logger.Debug("Filtering system-managed env var from deploy request before processEnvVars",
					"key", env.Key)
			}
		}
		s.logger.Debug("Filtered deploy request env vars",
			"originalCount", len(req.Env), "filteredCount", len(userEnv), "removedCount", len(req.Env)-len(userEnv))
	}

	// Process user-provided environment variables, handling secrets separately
	// Always call processEnvVars to ensure secrets cleanup happens when all env vars are removed
	// Include file mount secrets alongside env var secrets so they share the same KV path
	envVars, err := s.processEnvVars(ctx, orgName, projectName, lowestEnv, agentName, userEnv, req.Files)
	if err != nil {
		s.logger.Error("Failed to process environment variables", "agentName", agentName, "error", err)
		return "", fmt.Errorf("failed to process environment variables: %w", err)
	}

	s.logger.Debug("Processed user env vars", "agentName", agentName, "count", len(envVars))

	// Combine user-processed env vars with preserved system-managed env vars
	deployReq.Env = append(envVars, systemManagedEnvVars...)
	s.logger.Debug("Final deploy env vars", "agentName", agentName, "totalCount", len(deployReq.Env))

	// Process file mounts
	fileVars, err := s.processFileVars(ctx, orgName, projectName, lowestEnv, agentName, req.Files)
	if err != nil {
		s.logger.Error("Failed to process file mounts", "agentName", agentName, "error", err)
		return "", fmt.Errorf("failed to process file mounts: %w", err)
	}
	deployReq.Files = fileVars
	s.logger.Debug("Processed file mounts", "agentName", agentName, "count", len(fileVars))

	targetEnv, err := s.ocClient.GetEnvironment(ctx, orgName, lowestEnv)
	if err != nil {
		s.logger.Warn("Failed to get environment details", "environment", lowestEnv, "error", err)
	}

	// Read the existing agent_configs row once so we can resolve omitted request
	// fields from DB and preserve pinned instrumentation_version during Upsert.
	var existingConfig *models.AgentConfig
	if targetEnv != nil {
		cfg, configErr := s.agentConfigRepo.Get(orgName, projectName, agentName, targetEnv.Name)
		switch {
		case errors.Is(configErr, repositories.ErrAgentConfigNotFound):
			s.logger.Debug("No config in database, using defaults", "agentName", agentName, "environment", targetEnv.Name)
		case configErr != nil:
			s.logger.Warn("Failed to read config from database", "agentName", agentName, "environment", targetEnv.Name, "error", configErr)
		default:
			existingConfig = cfg
			s.logger.Debug("Read config from database", "agentName", agentName, "environment", targetEnv.Name,
				"enableAutoInstrumentation", cfg.EnableAutoInstrumentation,
				"enableApiKeySecurity", cfg.EnableApiKeySecurity,
				"instrumentationVersion", cfg.InstrumentationVersion)
		}
	}

	// Resolve config values: request value > DB value > default true.
	enableAutoInstrumentation := true
	if req.EnableAutoInstrumentation != nil {
		enableAutoInstrumentation = *req.EnableAutoInstrumentation
		s.logger.Info("Using enableAutoInstrumentation from request", "agentName", agentName, "value", enableAutoInstrumentation)
	} else if existingConfig != nil {
		enableAutoInstrumentation = existingConfig.EnableAutoInstrumentation
	}

	enableApiKeySecurity := true
	if req.EnableApiKeySecurity != nil {
		enableApiKeySecurity = *req.EnableApiKeySecurity
		s.logger.Info("Using enableApiKeySecurity from request", "agentName", agentName, "value", enableApiKeySecurity)
	} else if existingConfig != nil {
		enableApiKeySecurity = existingConfig.EnableApiKeySecurity
	}

	// Resolve CORS config: request > DB > env-var defaults.
	defaultCORS := config.GetAgentWorkloadConfig().CORS
	corsEnabled := true
	corsAllowOrigins := strings.Split(defaultCORS.AllowOrigin, ",")
	corsAllowMethods := strings.Split(defaultCORS.AllowMethods, ",")
	corsAllowHeaders := strings.Split(defaultCORS.AllowHeaders, ",")
	corsAllowCredentials := false
	if existingConfig != nil {
		corsEnabled = existingConfig.CORSEnabled
		if len(existingConfig.CORSAllowOrigins) > 0 {
			corsAllowOrigins = existingConfig.CORSAllowOrigins
		}
		if len(existingConfig.CORSAllowMethods) > 0 {
			corsAllowMethods = existingConfig.CORSAllowMethods
		}
		if len(existingConfig.CORSAllowHeaders) > 0 {
			corsAllowHeaders = existingConfig.CORSAllowHeaders
		}
		corsAllowCredentials = existingConfig.CORSAllowCredentials
	}
	if req.HasCorsConfig() {
		cc := req.GetCorsConfig()
		if cc.Enabled != nil {
			corsEnabled = *cc.Enabled
		}
		if len(cc.AllowOrigin) > 0 {
			corsAllowOrigins = cc.AllowOrigin
		}
		if len(cc.AllowMethods) > 0 {
			corsAllowMethods = cc.AllowMethods
		}
		if len(cc.AllowHeaders) > 0 {
			corsAllowHeaders = cc.AllowHeaders
		}
		if cc.AllowCredentials != nil {
			corsAllowCredentials = *cc.AllowCredentials
		}
	}
	if corsAllowCredentials {
		for _, origin := range corsAllowOrigins {
			if origin == "*" {
				return "", fmt.Errorf("corsConfig.allowCredentials cannot be true when allowOrigin contains \"*\"")
			}
		}
	}

	var existingInstrumentationVersion *string
	if existingConfig != nil {
		existingInstrumentationVersion = existingConfig.InstrumentationVersion
	}

	// Check if a previous deployment is still in progress BEFORE we make any
	// Component mutations. Doing it after AttachTraits / ReplaceComponentEnvVars
	// would race with our own writes: the controller flips Ready→False/Progressing
	// while reconciling them, the check then misreads that as a real concurrent
	// deploy, and we abort with the Component already half-mutated.
	inProgress, err := s.ocClient.IsDeploymentInProgress(ctx, orgName, agentName, lowestEnv)
	if err != nil {
		s.logger.Warn("Failed to check deployment status", "agentName", agentName, "environment", lowestEnv, "error", err)
		// Continue with deploy even if the check fails
	} else if inProgress {
		s.logger.Warn("Deployment already in progress", "agentName", agentName, "environment", lowestEnv)
		return "", fmt.Errorf("%w for agent %s in environment %s", utils.ErrDeploymentInProgress, agentName, lowestEnv)
	}

	componentDeployConfig := client.ComponentDeploymentConfigRequest{
		Env: deployReq.Env,
	}
	requiresComponentConfig := false
	isAPIAgent := agent.Type.Type == string(utils.AgentTypeAPI)

	// Configure instrumentation traits before deploy for Python buildpack API agents.
	// The actual Component CR update is applied once below together with API config and env vars.
	if isAPIAgent && agent.Build != nil && agent.Build.Buildpack != nil && agent.Build.Buildpack.Language == string(utils.LanguagePython) {
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
				apiKey, keyErr := s.generateAgentAPIKey(ctx, orgName, projectName, agentName)
				if keyErr != nil {
					s.logger.Warn("Failed to generate API key for OTEL instrumentation trait before deploy", "agentName", agentName, "error", keyErr)
				} else {
					componentDeployConfig.TraitsToAttach = append(componentDeployConfig.TraitsToAttach, client.TraitRequest{
						TraitKind: client.TraitKindTrait,
						TraitType: client.TraitOTELInstrumentation,
						Opts:      []client.TraitOption{client.WithAgentApiKey(apiKey)},
					})
					requiresComponentConfig = true
				}
			}
			if hasEnvTrait && envTraitErr == nil {
				s.logger.Info("Detaching env injection trait (OTEL trait will handle env vars)", "agentName", agentName)
				componentDeployConfig.TraitsToDetach = append(componentDeployConfig.TraitsToDetach, client.TraitEnvInjection)
				requiresComponentConfig = true
			}
		} else {
			// Disable auto-instrumentation: detach OTEL trait, attach env injection trait
			if hasOTELTrait && otelTraitErr == nil {
				s.logger.Info("Disabling instrumentation (detaching OTEL trait) before deploy", "agentName", agentName)
				componentDeployConfig.TraitsToDetach = append(componentDeployConfig.TraitsToDetach, client.TraitOTELInstrumentation)
				requiresComponentConfig = true
			}
			if !hasEnvTrait && envTraitErr == nil {
				s.logger.Info("Attaching env injection trait (for env vars without full instrumentation)", "agentName", agentName)
				apiKey, keyErr := s.generateAgentAPIKey(ctx, orgName, projectName, agentName)
				if keyErr != nil {
					s.logger.Warn("Failed to generate API key for env injection trait before deploy", "agentName", agentName, "error", keyErr)
				} else {
					componentDeployConfig.TraitsToAttach = append(componentDeployConfig.TraitsToAttach, client.TraitRequest{
						TraitKind: client.TraitKindTrait,
						TraitType: client.TraitEnvInjection,
						Opts:      []client.TraitOption{client.WithAgentApiKey(apiKey)},
					})
					requiresComponentConfig = true
				}
			}
		}
	}

	// Replace Component CR workflow parameters with env vars and file mounts from deploy request
	// This replaces all existing env vars to ensure the component CR matches the deploy request
	s.logger.Debug("Replacing component workflow parameters with environment variables", "agentName", agentName, "envVarCount", len(deployReq.Env))
	if err := s.ocClient.ReplaceComponentEnvVars(ctx, orgName, projectName, agentName, deployReq.Env); err != nil {
		s.logger.Warn("Failed to replace component workflow parameters with env vars", "agentName", agentName, "error", err)
		// Continue with deploy even if this fails - env vars will still be applied to the workload
	}
	// Manage api-configuration trait for API agents (attach/update with artifact-id and policies)

	if deployReq.Files != nil {
		s.logger.Debug("Replacing component workflow parameters with file mounts", "agentName", agentName, "fileMountCount", len(deployReq.Files))
		if err := s.ocClient.ReplaceComponentFileMounts(ctx, orgName, projectName, agentName, deployReq.Files); err != nil {
			s.logger.Warn("Failed to replace component workflow parameters with file mounts", "agentName", agentName, "error", err)
		}
	}

	if isAPIAgent {
		if targetEnv == nil {
			return "", fmt.Errorf("cannot deploy API agent without environment details")
		}
		apiArtifact, artifactErr := ensureAgentEnvAPIArtifact(s.db, s.artifactRepo, orgName, projectName, agentName, targetEnv.UUID)
		if artifactErr != nil {
			return "", fmt.Errorf("cannot deploy API agent without environment API artifact record: %w", artifactErr)
		}
		artifactID := apiArtifact.UUID.String()

		traitOpts := []client.TraitOption{
			client.WithArtifactID(artifactID),
		}
		if agent.InputInterface != nil && agent.InputInterface.Port > 0 {
			traitOpts = append(traitOpts, client.WithUpstreamPort(agent.InputInterface.Port))
		} else {
			traitOpts = append(traitOpts, client.WithUpstreamPort(config.GetConfig().DefaultChatAPI.DefaultHTTPPort))
		}
		if agent.InputInterface != nil && agent.InputInterface.BasePath != "" {
			traitOpts = append(traitOpts, client.WithUpstreamBasePath(agent.InputInterface.BasePath))
		} else {
			traitOpts = append(traitOpts, client.WithUpstreamBasePath(config.GetConfig().DefaultChatAPI.DefaultBasePath))
		}
		var policies []map[string]interface{}
		if corsEnabled {
			policies = append(policies, client.CORSPolicy(corsAllowOrigins, corsAllowMethods, corsAllowHeaders, corsAllowCredentials))
		}
		if enableApiKeySecurity {
			policies = append(policies, client.APIKeyAuthPolicy())
		}
		traitOpts = append(traitOpts, client.WithPolicies(policies))

		componentDeployConfig.TraitsToAttach = append(componentDeployConfig.TraitsToAttach, client.TraitRequest{
			TraitKind: client.TraitKindTrait,
			TraitType: client.TraitAPIManagement,
			Opts:      traitOpts,
		})
		requiresComponentConfig = true
		s.logger.Info("Updated api-configuration trait", "agentName", agentName, "artifactID", artifactID, "enableApiKeySecurity", enableApiKeySecurity)
	}

	// Apply deploy-time Component CR changes in a single PUT. This replaces workflow env vars
	// and also applies any trait changes needed for this deploy.
	s.logger.Debug("Updating component deployment config", "agentName", agentName, "envVarCount", len(deployReq.Env),
		"traitsToAttach", len(componentDeployConfig.TraitsToAttach), "traitsToDetach", len(componentDeployConfig.TraitsToDetach))
	if err := s.ocClient.UpdateComponentDeploymentConfig(ctx, orgName, projectName, agentName, componentDeployConfig); err != nil {
		if requiresComponentConfig {
			return "", fmt.Errorf("failed to update component deployment config: %w", err)
		}
		s.logger.Warn("Failed to replace component workflow parameters with env vars", "agentName", agentName, "error", err)
		// Continue with deploy even if this fails - env vars will still be applied to the workload.
	}

	// Deploy agent component in OpenChoreo (after env vars and instrumentation are configured)
	s.logger.Debug("Deploying agent component in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "imageId", req.ImageId)
	if err := s.ocClient.Deploy(ctx, orgName, projectName, agentName, deployReq); err != nil {
		s.logger.Error("Failed to deploy agent component in OpenChoreo", "agentName", agentName, "orgName", orgName, "projectName", projectName, "error", err)
		return "", err
	}

	// Persist instrumentation config to database. Passing the pinned
	// instrumentation_version (captured above) preserves it across the
	// Upsert — the repo's DoUpdates map includes that column, so omitting
	// the value would NULL out a customer's pin on every redeploy.
	if targetEnv != nil {
		agentConfig := &models.AgentConfig{
			OrgName:                   orgName,
			ProjectName:               projectName,
			AgentName:                 agentName,
			EnvironmentName:           targetEnv.Name,
			EnableAutoInstrumentation: enableAutoInstrumentation,
			InstrumentationVersion:    existingInstrumentationVersion,
			EnableApiKeySecurity:      enableApiKeySecurity,
			CORSEnabled:               corsEnabled,
			CORSAllowOrigins:          corsAllowOrigins,
			CORSAllowMethods:          corsAllowMethods,
			CORSAllowHeaders:          corsAllowHeaders,
			CORSAllowCredentials:      corsAllowCredentials,
		}
		if configErr := s.agentConfigRepo.Upsert(agentConfig); configErr != nil {
			s.logger.Error("Failed to persist instrumentation config to database", "agentName", agentName, "environment", lowestEnv, "error", configErr)
		} else {
			s.logger.Debug("Persisted instrumentation config to database", "agentName", agentName, "environment", lowestEnv, "enableAutoInstrumentation", enableAutoInstrumentation, "instrumentationVersion", existingInstrumentationVersion)
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

// getSystemManagedEnvVars fetches existing env vars from the Component CR / ReleaseBinding and
// identifies system-managed secret env vars (e.g., LLM provider config API keys).
//
// System-managed env vars are identified by looking up the secretRef in the DB: if it is
// recorded in agent_env_config_variables_mapping for this agent's LLM configurations, it is
// system-managed. This is provider-agnostic — it works for both OpenBao and the Secret Manager
// API without relying on secret reference name patterns.
//
// These must be handled separately from processEnvVars because processEnvVars would use the
// env var name (e.g., "CUSTOM_API_KEY") as the SecretKeyRef.Key, but the actual key in the
// K8s Secret is different (e.g., "api-key").
//
// Returns:
//   - []client.EnvVar: system-managed env vars with correct SecretKeyRef
//   - map[string]bool: set of system-managed env var keys (for filtering from deploy request)
func (s *agentManagerService) getSystemManagedEnvVars(
	ctx context.Context,
	orgName, projectName, environmentName, componentName string,
) ([]client.EnvVar, map[string]bool, error) {
	existingConfigs, err := s.ocClient.GetComponentConfigurations(ctx, orgName, projectName, componentName, environmentName)
	if err != nil {
		return nil, nil, err
	}
	if len(existingConfigs) == 0 {
		s.logger.Debug("No existing env vars found in component configurations", "agentName", componentName)
		return nil, nil, nil
	}

	// Fetch the set of SecretReference names that belong to LLM configurations for this agent
	// and environment from the DB. These are the source of truth — provider-agnostic.
	llmSecretRefs, err := s.agentConfigurationService.ListAgentLLMConfigSecretReferences(ctx, componentName, orgName, environmentName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch LLM config secret references: %w", err)
	}

	s.logger.Debug("Identifying system-managed env vars",
		"agentName", componentName, "existingCount", len(existingConfigs),
		"llmSecretRefCount", len(llmSecretRefs))

	var result []client.EnvVar
	keySet := make(map[string]bool)

	for _, existing := range existingConfigs {
		if !existing.IsSensitive || existing.SecretRef == "" {
			continue
		}
		if _, isLLMRef := llmSecretRefs[existing.SecretRef]; !isLLMRef {
			continue
		}
		secretKey := existing.SecretKey
		if secretKey == "" {
			secretKey = existing.Key
			s.logger.Warn("System-managed secret env var missing SecretKey, falling back to env var name",
				"key", existing.Key, "secretRef", existing.SecretRef)
		}
		result = append(result, client.EnvVar{
			Key: existing.Key,
			ValueFrom: &client.EnvVarValueFrom{
				SecretKeyRef: &client.SecretKeyRef{
					Name: existing.SecretRef,
					Key:  secretKey,
				},
			},
		})
		keySet[existing.Key] = true
		s.logger.Info("Identified system-managed secret env var",
			"key", existing.Key, "secretRef", existing.SecretRef, "secretKey", secretKey)
	}

	return result, keySet, nil
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
//
// fileMounts are also processed for secrets: sensitive file mount values are stored in the
// same KV path alongside env var secrets.
func (s *agentManagerService) processEnvVars(
	ctx context.Context,
	orgName, projectName, environmentName, componentName string,
	envVars []spec.EnvironmentVariable,
	fileMounts []spec.FileMount,
) ([]client.EnvVar, error) {
	secretData := make(map[string]string)
	var preservedSecretKeys []string

	// Build secret location for the secret store
	location := secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     projectName,
		EnvironmentName: environmentName,
		EntityName:      componentName,
	}

	// Track per-env-var secretRef overrides for system-managed secrets (e.g., LLM config keys).
	// Keys not in this map use the agent's own secretRefName.
	secretRefOverrides := make(map[string]string)

	// Fetch existing secret keys upfront so we can correctly classify sensitive env vars
	// that come back with secretRef + empty value as either "ours" (key exists in our
	// agent's secret) or "system-managed" (key lives in another secret like an LLM config).
	var existingInfo *secretmanagersvc.SecretInfo
	existingKeys := make(map[string]struct{})
	if s.secretMgmtClient != nil {
		kvPath, kvErr := location.KVPath()
		if kvErr != nil {
			return nil, fmt.Errorf("failed to construct KV path for secrets lookup: %w", kvErr)
		}
		info, getErr := s.secretMgmtClient.GetSecret(ctx, kvPath)
		if getErr != nil && !errors.Is(getErr, secretmanagersvc.ErrSecretNotFound) {
			return nil, fmt.Errorf("failed to read existing secret metadata: %w", getErr)
		}
		existingInfo = info
		if existingInfo != nil {
			for _, k := range existingInfo.Keys {
				existingKeys[k] = struct{}{}
			}
		}
	}

	// First pass: collect secret data from env vars
	for _, env := range envVars {
		if env.GetIsSensitive() {
			if env.HasSecretRef() && env.GetValue() == "" {
				existingSecretRefName := env.GetSecretRef()
				if _, ours := existingKeys[env.Key]; ours {
					preservedSecretKeys = append(preservedSecretKeys, env.Key)
					s.logger.Debug("Preserving existing secret", "key", env.Key, "secretRef", existingSecretRefName)
				} else {
					s.logger.Info(fmt.Sprintf("Skipping existing system-managed secret-ref %s for key %s", existingSecretRefName, env.Key))
					secretRefOverrides[env.Key] = existingSecretRefName
				}
			} else if env.GetValue() != "" {
				secretData[env.Key] = env.GetValue()
			} else {
				return nil, fmt.Errorf("sensitive environment variable %q requires either a value or secretRef", env.Key)
			}
		}
	}

	// Also collect secret data from file mounts (same KV path)
	for _, f := range fileMounts {
		if f.GetIsSensitive() {
			if f.HasSecretRef() && f.GetValue() == "" {
				if _, ours := existingKeys[f.Key]; ours {
					preservedSecretKeys = append(preservedSecretKeys, f.Key)
					s.logger.Debug("Preserving existing file mount secret", "key", f.Key)
				}
			} else if f.GetValue() != "" {
				secretData[f.Key] = f.GetValue()
			} else {
				return nil, fmt.Errorf("sensitive file mount %q requires either a value or secretRef", f.Key)
			}
		}
	}

	// Sync secrets to KV store and get the secretRefName
	secretRefName, err := s.syncSecrets(ctx, location, secretData, preservedSecretKeys, existingInfo)
	if err != nil {
		return nil, err
	}

	// Second pass: build result for env vars only (file mounts are handled by processFileVars)
	var result []client.EnvVar
	for _, env := range envVars {
		if env.GetIsSensitive() {
			refName := secretRefName
			if override, ok := secretRefOverrides[env.Key]; ok {
				refName = override
			}
			result = append(result, client.EnvVar{
				Key: env.Key,
				ValueFrom: &client.EnvVarValueFrom{
					SecretKeyRef: &client.SecretKeyRef{
						Name: refName,
						Key:  env.Key,
					},
				},
			})
		} else {
			result = append(result, client.EnvVar{
				Key:   env.Key,
				Value: env.GetValue(),
			})
		}
	}

	return result, nil
}

var validMountPathRe = regexp.MustCompile(`^/[A-Za-z0-9._\-/]*$`)

// processFileVars converts spec.FileMount entries to client.FileVar entries.
// Sensitive file mounts use secretKeyRef pointing to the K8s Secret (secrets are
// already stored in KV by processEnvVars which handles both env and file secrets).
func (s *agentManagerService) processFileVars(
	ctx context.Context,
	orgName, projectName, environmentName, componentName string,
	fileMounts []spec.FileMount,
) ([]client.FileVar, error) {
	if len(fileMounts) == 0 {
		return make([]client.FileVar, 0), nil
	}

	for _, f := range fileMounts {
		mp := f.MountPath
		if !strings.HasPrefix(mp, "/") {
			return nil, fmt.Errorf("mount path %q must be an absolute path", mp)
		}
		if strings.Contains(mp, "..") {
			return nil, fmt.Errorf("mount path %q must not contain path traversal (..)", mp)
		}
		if !validMountPathRe.MatchString(mp) {
			return nil, fmt.Errorf("mount path %q contains invalid characters", mp)
		}
	}

	// Build secret location to derive the secretRefName
	location := secretmanagersvc.SecretLocation{
		OrgName:         orgName,
		ProjectName:     projectName,
		EnvironmentName: environmentName,
		EntityName:      componentName,
	}
	secretRefName := location.SecretRefName()

	var result []client.FileVar
	for _, f := range fileMounts {
		if f.GetIsSensitive() {
			result = append(result, client.FileVar{
				Key:       f.Key,
				MountPath: f.MountPath,
				ValueFrom: &client.EnvVarValueFrom{
					SecretKeyRef: &client.SecretKeyRef{
						Name: secretRefName,
						Key:  f.Key,
					},
				},
			})
		} else {
			result = append(result, client.FileVar{
				Key:       f.Key,
				MountPath: f.MountPath,
				Value:     f.GetValue(),
			})
		}
	}

	return result, nil
}

// syncSecrets synchronizes secrets between the request and the secret store / SecretReference.
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
//   - existingInfo: secret metadata pre-fetched by the caller (nil if no secret exists at the location)
//
// Returns the secretRefName on success, empty string if no secrets to sync.
func (s *agentManagerService) syncSecrets(
	ctx context.Context,
	location secretmanagersvc.SecretLocation,
	newSecretData map[string]string,
	preservedSecretKeys []string,
	existingInfo *secretmanagersvc.SecretInfo,
) (string, error) {
	secretRefName := location.SecretRefName()
	totalSecretCount := len(newSecretData) + len(preservedSecretKeys)

	// Case 1: No secrets in current request (neither new nor preserved) - cleanup any existing secrets
	if totalSecretCount == 0 {
		// Delete secret from KV and SecretReference
		if s.secretMgmtClient != nil {
			if err := s.secretMgmtClient.DeleteSecret(ctx, location, secretRefName); err != nil {
				kvPath, _ := location.KVPath()
				s.logger.Warn("Failed to delete secret during cleanup", "kvPath", kvPath, "error", err)
			} else {
				kvPath, _ := location.KVPath()
				s.logger.Debug("Deleted secret", "kvPath", kvPath)
			}
		}
		return "", nil
	}

	kvPath, err := location.KVPath()
	if err != nil {
		s.logger.Warn("Failed to construct KV path for secrets sync", "location", location, "error", err)
		return "", fmt.Errorf("failed to construct KV path for secrets sync: %w", err)
	}

	// Case 2: Have secrets to store/update in KV (either new or preserved)
	// Use PatchSecret for efficient server-side merge instead of read-modify-write
	if len(newSecretData) > 0 || len(preservedSecretKeys) > 0 {
		if s.secretMgmtClient == nil {
			return "", fmt.Errorf("secret management is not enabled but secret env vars were provided")
		}

		s.logger.Debug("Storing secrets in KV", "kvPath", kvPath, "newSecretCount", len(newSecretData), "preservedCount", len(preservedSecretKeys))

		// Build set of keys that should remain (new + preserved)
		keysToKeep := make(map[string]struct{})
		for key := range newSecretData {
			keysToKeep[key] = struct{}{}
		}
		for _, key := range preservedSecretKeys {
			keysToKeep[key] = struct{}{}
		}

		// existingInfo was pre-fetched by the caller (processEnvVars). Use it to compute deletions.
		var keysToDelete []string
		if existingInfo != nil {
			// Validate that preserved keys exist in the secret
			existingKeysSet := make(map[string]struct{})
			for _, key := range existingInfo.Keys {
				existingKeysSet[key] = struct{}{}
			}
			for _, key := range preservedSecretKeys {
				if _, ok := existingKeysSet[key]; !ok {
					return "", fmt.Errorf("preserved secret key %q not found in existing secrets at %s", key, kvPath)
				}
			}
			// Compute keys to delete: existing keys not in keysToKeep
			for _, key := range existingInfo.Keys {
				if _, keep := keysToKeep[key]; !keep {
					keysToDelete = append(keysToDelete, key)
				}
			}
		} else if len(preservedSecretKeys) > 0 {
			// No existing secret but trying to preserve keys - error
			return "", fmt.Errorf("no existing secrets found at %s to preserve keys", kvPath)
		}

		if existingInfo != nil {
			// Secret exists — use PatchSecret for server-side merge
			secretRefName, err = s.secretMgmtClient.PatchSecret(ctx, location, newSecretData, keysToDelete)
			if err != nil {
				if errors.Is(err, secretmanagersvc.ErrNotManaged) {
					return "", fmt.Errorf("secret path %q is already owned by another system and cannot be overwritten; manual cleanup may be required: %w", kvPath, utils.ErrSecretPathConflict)
				}
				return "", fmt.Errorf("failed to patch secrets: %w", err)
			}
		} else {
			// Secret doesn't exist — use CreateSecret
			secretRefName, err = s.secretMgmtClient.CreateSecret(ctx, location, newSecretData)
			if err != nil {
				if errors.Is(err, secretmanagersvc.ErrNotManaged) {
					return "", fmt.Errorf("secret path %q is already owned by another system and cannot be overwritten; manual cleanup may be required: %w", kvPath, utils.ErrSecretPathConflict)
				}
				return "", fmt.Errorf("failed to create secrets: %w", err)
			}
		}
	}

	// SecretReference creation/update is handled internally by secretMgmtClient.PatchSecret
	s.logger.Info("Secrets synchronized successfully", "componentName", location.EntityName, "kvPath", kvPath, "newSecretCount", len(newSecretData), "preservedSecretCount", len(preservedSecretKeys))
	return secretRefName, nil
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
	project, err := s.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", orgName, "error", err)
		return nil, translateProjectError(err)
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
	if _, err := s.ocClient.GetOrganization(ctx, orgName); err != nil {
		s.logger.Error("Failed to find organization", "orgName", orgName, "error", err)
		return nil, translateOrgError(err)
	}
	// Check if environment exists
	_, err := s.ocClient.GetEnvironment(ctx, orgName, environment)
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

func (s *agentManagerService) GetAgentFileMounts(ctx context.Context, orgName string, projectName string, agentName string, environment string) ([]models.FileMountEntry, error) {
	s.logger.Info("Getting agent file mounts", "agentName", agentName, "orgName", orgName, "projectName", projectName, "environment", environment)

	fileMounts, err := s.ocClient.GetComponentFileMounts(ctx, orgName, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch file mounts", "agentName", agentName, "error", err)
		return nil, fmt.Errorf("failed to get file mounts for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched file mounts successfully", "agentName", agentName, "count", len(fileMounts))
	return fileMounts, nil
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

// modelBuildToSpecBuild converts a models.Build (from GetComponent) into a spec.Build for CreateAgent enrichment.
func modelBuildToSpecBuild(b *models.Build) *spec.Build {
	if b == nil {
		return nil
	}
	if b.Buildpack != nil {
		bpCfg := spec.BuildpackConfig{Language: b.Buildpack.Language}
		if b.Buildpack.LanguageVersion != "" {
			bpCfg.LanguageVersion = &b.Buildpack.LanguageVersion
		}
		if b.Buildpack.RunCommand != "" {
			bpCfg.RunCommand = &b.Buildpack.RunCommand
		}
		bp := spec.BuildpackBuildAsBuild(spec.NewBuildpackBuild("buildpack", bpCfg))
		return &bp
	}
	if b.Docker != nil {
		d := spec.DockerBuildAsBuild(spec.NewDockerBuild("docker", spec.DockerConfig{DockerfilePath: b.Docker.DockerfilePath}))
		return &d
	}
	return nil
}

// inputInterfaceToEndpoints converts an InputInterfaceConfig to the slice expected by CreateInternalAgentFromKindWorkload.
// Note: Workload CRs require inline schema content, not a file path. Since the schema path originates
// from the git repository of the source agent, schema is intentionally omitted here — it is already
// configured at the Component level via CreateComponent.
func inputInterfaceToEndpoints(cfg *client.InputInterfaceConfig, componentName string) []client.InputInterfaceEndpoint {
	if cfg == nil {
		return nil
	}
	ep := client.InputInterfaceEndpoint{
		Name:       componentName + "-endpoint",
		Port:       int(cfg.Port),
		Type:       cfg.Type,
		BasePath:   cfg.BasePath,
		Visibility: []string{"external"},
	}
	return []client.InputInterfaceEndpoint{ep}
}
