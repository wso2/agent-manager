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
	"time"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/instrumentation"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"gorm.io/gorm"
)

type AgentManagerService interface {
	ListAgents(ctx context.Context, ouID string, projName string, labelFilter map[string]string, limit int32, offset int32) ([]*models.AgentResponse, int32, error)
	CreateAgent(ctx context.Context, ouID string, projectName string, req *spec.CreateAgentRequest) error
	UpdateAgentBasicInfo(ctx context.Context, ouID string, projectName string, agentName string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error)
	UpdateAgentBuildParameters(ctx context.Context, ouID string, projectName string, agentName string, req *spec.UpdateAgentBuildParametersRequest) (*models.AgentResponse, error)
	BuildAgent(ctx context.Context, ouID string, projectName string, agentName string, commitId string) (*models.BuildResponse, error)
	DeleteAgent(ctx context.Context, ouID string, projectName string, agentName string) error
	DeployAgent(ctx context.Context, ouID string, projectName string, agentName string, req *spec.DeployAgentRequest) (string, error)
	GetAgent(ctx context.Context, ouID string, projectName string, agentName string) (*models.AgentResponse, error)
	ListAgentBuilds(ctx context.Context, ouID string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error)
	GetBuild(ctx context.Context, ouID string, projectName string, agentName string, buildName string) (*models.BuildDetailsResponse, error)
	GetAgentDeployments(ctx context.Context, ouID string, projectName string, agentName string) ([]*models.DeploymentResponse, error)
	UpdateAgentDeploymentState(ctx context.Context, ouID string, projectName string, agentName string, environment string, state string) error
	GetAgentEndpoints(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (map[string]models.EndpointsResponse, error)
	GetAgentConfigurations(ctx context.Context, ouID string, projectName string, agentName string, environment string) ([]models.EnvVars, error)
	GetAgentFileMounts(ctx context.Context, ouID string, projectName string, agentName string, environment string) ([]models.FileMountEntry, error)
	GetAgentEnvConfig(ctx context.Context, ouID string, projectName string, agentName string, environment string) (*models.AgentConfig, error)
	GenerateName(ctx context.Context, ouID string, payload spec.ResourceNameRequest) (string, error)
	GetAgentResourceConfigs(ctx context.Context, ouID string, projectName string, agentName string, environment string) (*spec.AgentResourceConfigsResponse, error)
	UpdateAgentResourceConfigs(ctx context.Context, ouID string, projectName string, agentName string, environment string, req *spec.UpdateAgentResourceConfigsRequest) (*spec.AgentResourceConfigsResponse, error)
	PromoteAgent(ctx context.Context, ouID string, projectName string, agentName string, req *spec.PromoteAgentRequest) error
	UpdateAgentDeploySettings(ctx context.Context, ouID string, projectName string, agentName string, req *spec.UpdateAgentDeploySettingsRequest) error
	UpdateAgentConfigurations(ctx context.Context, ouID string, projectName string, agentName string, req *spec.UpdateAgentConfigurationsRequest) error
	RegenerateAgentTracingToken(ctx context.Context, ouID string, projectName string, agentName string, environmentName string, expiresIn string) (*TracingTokenRotationResult, error)
	GetAgentIdentity(ctx context.Context, ouID string, projectName string, agentName string) ([]models.AgentIdentityEnvironmentView, error)
	RegenerateAgentIdentitySecret(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (*models.AgentRegenerateSecretResponse, error)
	RevokeAgentIdentitySecret(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (models.AgentRevokeSecretResponse, error)
	ProvisionAgentIdentity(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (view models.AgentIdentityEnvironmentView, alreadyExisted bool, err error)
	GetAgentRoles(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) ([]thundersvc.ThunderRole, error)
	GetAgentGroups(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) ([]thundersvc.ThunderGroup, error)
}

type agentManagerService struct {
	db                        *gorm.DB
	ocClient                  client.OpenChoreoClient
	secretMgmtClient          secretmanagersvc.SecretManagementClient
	gitRepositoryService      RepositoryService
	tokenManagerService       AgentTokenManagerService
	agentConfigRepo           repositories.AgentConfigRepository
	agentConfigurationService AgentConfigurationService
	agentKindService          AgentKindService
	artifactRepo              repositories.ArtifactRepository
	aiApplicationService      *AIApplicationService
	gatewayRepo               repositories.GatewayRepository
	agentThunderProvisioning  AgentThunderProvisioningService
	monitorManagerService     MonitorManagerService
	agentIdentityInjection    AgentIdentityInjectionService
	logger                    *slog.Logger
}

func NewAgentManagerService(
	db *gorm.DB,
	OpenChoreoClient client.OpenChoreoClient,
	secretMgmtClient secretmanagersvc.SecretManagementClient,
	gitRepositoryService RepositoryService,
	tokenManagerService AgentTokenManagerService,
	agentConfigRepo repositories.AgentConfigRepository,
	agentConfigurationService AgentConfigurationService,
	agentKindService AgentKindService,
	artifactRepo repositories.ArtifactRepository,
	aiApplicationService *AIApplicationService,
	gatewayRepo repositories.GatewayRepository,
	agentThunderProvisioning AgentThunderProvisioningService,
	monitorManagerService MonitorManagerService,
	agentIdentityInjection AgentIdentityInjectionService,
	logger *slog.Logger,
) AgentManagerService {
	return &agentManagerService{
		db:                        db,
		ocClient:                  OpenChoreoClient,
		secretMgmtClient:          secretMgmtClient,
		gitRepositoryService:      gitRepositoryService,
		tokenManagerService:       tokenManagerService,
		agentConfigRepo:           agentConfigRepo,
		agentConfigurationService: agentConfigurationService,
		agentKindService:          agentKindService,
		agentThunderProvisioning:  agentThunderProvisioning,
		monitorManagerService:     monitorManagerService,
		agentIdentityInjection:    agentIdentityInjection,
		artifactRepo:              artifactRepo,
		aiApplicationService:      aiApplicationService,
		gatewayRepo:               gatewayRepo,
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
func (s *agentManagerService) validateGitSecretExists(ctx context.Context, ouID string, secretRef string) error {
	if secretRef == "" {
		return fmt.Errorf("git secret reference is empty")
	}

	secrets, err := s.ocClient.ListGitSecrets(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to list git secrets for validation", "ouID", ouID, "error", err)
		return fmt.Errorf("failed to validate git secret: %w", err)
	}

	for _, secret := range secrets {
		if secret.Name == secretRef {
			return nil
		}
	}

	s.logger.Error("Git secret not found", "ouID", ouID, "secretRef", secretRef)
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
		config.SchemaPath = utils.StrPointerAsStr(specInterface.Schema.Path, "")
	}

	return config
}
func resolveResilienceTimeoutSeconds(existingConfig *models.AgentConfig, requested *int32, withDefaults bool) int32 {
	resolved := int32(0)
	if withDefaults {
		resolved = client.DefaultResilienceTimeoutSeconds
	}
	if existingConfig != nil && existingConfig.ResilienceTimeoutSeconds != nil {
		resolved = *existingConfig.ResilienceTimeoutSeconds
	}
	if requested != nil && *requested >= client.MinResilienceTimeoutSeconds && *requested <= client.MaxResilienceTimeoutSeconds {
		resolved = *requested
	}
	return resolved
}

// buildCreateTraitRequests collects all traits needed during agent creation into a single
// list so they can be attached in one GET-UPDATE cycle, avoiding resource version conflicts.
// artifactID is the UUID of the agent's artifact record (used for api-configuration trait).
func (s *agentManagerService) buildCreateTraitRequests(ctx context.Context, ouID, projectName, artifactID, envName string, req *spec.CreateAgentRequest) ([]client.TraitRequest, error) {
	var traits []client.TraitRequest

	// Determine instrumentation settings
	autoInstrumentation := req.Configurations == nil || req.Configurations.EnableAutoInstrumentation == nil || *req.Configurations.EnableAutoInstrumentation
	isAPIAgent := req.AgentType != nil && req.AgentType.Type == string(utils.AgentTypeAPI)

	isPythonBuildpack := req.Build != nil && req.Build.BuildpackBuild != nil && req.Build.BuildpackBuild.Buildpack.Language == string(utils.LanguagePython)
	isBallerinaBuildpack := req.Build != nil && req.Build.BuildpackBuild != nil && req.Build.BuildpackBuild.Buildpack.Language == string(utils.LanguageBallerina)
	isDocker := req.Build != nil && req.Build.DockerBuild != nil

	// Attach instrumentation traits at creation.
	// env-injection is always attached for all API agents — it is the sole injector of
	// AMP_OTEL_ENDPOINT and AMP_AGENT_API_KEY (includeWhen on patches is not supported).
	// python-otel-instrumentation-trait is only attached for Python buildpack agents with
	// auto-instrumentation enabled; it handles the init container, SDK volume, and PYTHONPATH.
	if isAPIAgent && isPythonBuildpack {
		apiKey, err := s.generateAgentAPIKey(ctx, ouID, projectName, req.Name, envName)
		if err != nil {
			return nil, fmt.Errorf("failed to generate agent API key: %w", err)
		}
		apiKeySecretRef, apiKeySecretProperty, err := s.storeAgentAPIKey(ctx, ouID, projectName, req.Name, envName, apiKey)
		if err != nil {
			return nil, fmt.Errorf("failed to store agent API key: %w", err)
		}

		// Always attach OTEL trait; patches are gated by instrumentationEnabled via 'where' on target,
		// enabling per-environment control through traitEnvironmentConfigs.
		otelOpts := []client.TraitOption{
			client.WithInstrumentationEnabled(autoInstrumentation),
		}
		lv := req.Build.BuildpackBuild.Buildpack.GetLanguageVersion()
		otelOpts = append(otelOpts, client.WithLanguageVersion(lv))
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

		traits = append(traits, client.TraitRequest{
			TraitKind: client.TraitKindTrait,
			TraitType: client.TraitEnvInjection,
			Opts: []client.TraitOption{
				client.WithAgentApiKeySecretRef(apiKeySecretRef),
				client.WithAgentApiKeySecretProperty(apiKeySecretProperty),
			},
		})
	} else if isAPIAgent && isBallerinaBuildpack {
		apiKey, err := s.generateAgentAPIKey(ctx, ouID, projectName, req.Name, envName)
		if err != nil {
			return nil, fmt.Errorf("failed to generate agent API key: %w", err)
		}
		apiKeySecretRef, apiKeySecretProperty, err := s.storeAgentAPIKey(ctx, ouID, projectName, req.Name, envName, apiKey)
		if err != nil {
			return nil, fmt.Errorf("failed to store agent API key: %w", err)
		}
		// Gate env injection on the same autoInstrumentation flag as the Ballerina
		// OTEL trait, so when instrumentation is disabled the OTEL endpoint and API
		// key env vars are not injected either. Patches are gated by the 'where'
		// clause and per-environment overridable via traitEnvironmentConfigs.
		traits = append(traits, client.TraitRequest{
			TraitKind: client.TraitKindTrait,
			TraitType: client.TraitEnvInjection,
			Opts: []client.TraitOption{
				client.WithAgentApiKeySecretRef(apiKeySecretRef),
				client.WithAgentApiKeySecretProperty(apiKeySecretProperty),
				client.WithOtelEndpointEnvName(client.BalConfigVarOTELEndpoint),
				client.WithApiKeyEnvName(client.BalConfigVarAgentAPIKey),
				client.WithEnvInjectionEnabled(autoInstrumentation),
			},
		})
		traits = append(traits, client.TraitRequest{
			TraitKind: client.TraitKindTrait,
			TraitType: client.TraitBallerinaOTELInstrumentation,
			Opts: []client.TraitOption{
				client.WithInstrumentationEnabled(autoInstrumentation),
			},
		})
	} else if isAPIAgent && isDocker {
		// Docker: attach only env-injection trait (no init container needed)
		apiKey, err := s.generateAgentAPIKey(ctx, ouID, projectName, req.Name, envName)
		if err != nil {
			return nil, fmt.Errorf("failed to generate agent API key: %w", err)
		}
		apiKeySecretRef, apiKeySecretProperty, err := s.storeAgentAPIKey(ctx, ouID, projectName, req.Name, envName, apiKey)
		if err != nil {
			return nil, fmt.Errorf("failed to store agent API key: %w", err)
		}
		traits = append(traits, client.TraitRequest{
			TraitKind: client.TraitKindTrait,
			TraitType: client.TraitEnvInjection,
			Opts: []client.TraitOption{
				client.WithAgentApiKeySecretRef(apiKeySecretRef),
				client.WithAgentApiKeySecretProperty(apiKeySecretProperty),
			},
		})
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
		apiTraitOpts := []client.TraitOption{
			client.WithArtifactID(artifactID),
			client.WithUpstreamPort(port),
			client.WithUpstreamBasePath(basePath),
			client.WithPolicies(createPolicies),
		}
		// backendHost, backendPort, gatewayTarget were previously injected per-environment
		// here; the api-management trait now derives them from the apiGatewayName convention
		// ("api-platform-<org>-<env>") and platform-wide gateway runtime constants.
		traits = append(traits, client.TraitRequest{
			TraitKind: client.TraitKindTrait,
			TraitType: client.TraitAPIManagement,
			Opts:      apiTraitOpts,
		})
	}

	return traits, nil
}

// ErrInstrumentationVersionNotPinned indicates an agent has no pinned AMP
// instrumentation version: no row in agent_configs yet, no project pipeline,
// or the column is NULL. Callers should treat it as "fall back to the platform
// default" rather than a real error. It is intentionally distinct from real
// errors (DB read failures, deployment pipeline lookup failures) so a transient
// failure can't silently swap a customer's pinned version for the default.
var ErrInstrumentationVersionNotPinned = errors.New("agent has no pinned instrumentation version")

// lookupAgentAutoInstrumentation returns the agent's persisted
// EnableAutoInstrumentation setting from agent_configs. Defaults to
// true when there is no row yet (matching the configurations default).
// Errors only on genuine DB failures; missing config is not an error.
func (s *agentManagerService) lookupAgentAutoInstrumentation(ctx context.Context, ouID, projectName, agentName string) (bool, error) {
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if err != nil {
		return true, fmt.Errorf("failed to get deployment pipeline: %w", err)
	}
	if len(pipeline.PromotionPaths) == 0 {
		return true, nil
	}
	lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
	if lowestEnv == "" {
		return true, nil
	}
	cfg, err := s.agentConfigRepo.Get(ouID, projectName, agentName, lowestEnv)
	if errors.Is(err, repositories.ErrAgentConfigNotFound) {
		return true, nil
	}
	if err != nil {
		return true, fmt.Errorf("failed to read agent config: %w", err)
	}
	if cfg == nil {
		return true, nil
	}
	return cfg.EnableAutoInstrumentation, nil
}

// lookupAgentInstrumentationVersion returns the agent's pinned AMP instrumentation
// version (from agent_configs.instrumentation_version). It returns
// ErrInstrumentationVersionNotPinned when there's genuinely no pin to honour,
// and a wrapped real error for transient failures.
func (s *agentManagerService) lookupAgentInstrumentationVersion(ctx context.Context, ouID, projectName, agentName string) (*string, error) {
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
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
	cfg, err := s.agentConfigRepo.Get(ouID, projectName, agentName, lowestEnv)
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

// validateInstrumentationVersion checks the AMP instrumentation version against
// the deployment's effective catalog. Unsupported values return ErrInvalidInput.
func (s *agentManagerService) validateInstrumentationVersion(version string) error {
	cat := instrumentation.GetCatalog()
	if cat.Has(version) {
		return nil
	}
	supported := make([]string, 0, len(cat.All()))
	for _, v := range cat.All() {
		supported = append(supported, v.Version)
	}
	return fmt.Errorf("%w: instrumentationVersion %q is not supported by this deployment; supported: %v", utils.ErrInvalidInput, version, supported)
}

// buildpackPythonVersion returns the buildpack-configured Python version
// normalised to bare-minor ("3.11"), matching the shape stored in the
// instrumentation catalog. Returns "" when the build is not a python
// buildpack build, when LanguageVersion is unset, or when the value
// normalises to empty.
//
// Normalisation:
//   - Language comparison is exact (matches the case-sensitive ==
//     comparison used elsewhere in this file, e.g. isPythonBuildpack at
//     line ~320, so a request with "Python" doesn't take a different
//     branch here than in the trait-attach logic).
//   - LanguageVersion is trimmed, then truncated to the first two
//     dot-separated components so "3.11", "3.11.4", and "3.11.x" all
//     collapse to "3.11" (the form the catalog uses).
func buildpackPythonVersion(b *spec.Build) string {
	if b == nil || b.BuildpackBuild == nil {
		return ""
	}
	bp := b.BuildpackBuild.Buildpack
	if bp.Language != string(utils.LanguagePython) {
		return ""
	}
	if bp.LanguageVersion == nil {
		return ""
	}
	// Single source of the major.minor normalisation rule.
	return normalizePythonMinor(*bp.LanguageVersion)
}

// validateEffectivePythonInstrumentationPair resolves the instrumentation
// version that will actually apply to the agent (the request's explicit
// pin if non-nil, otherwise the platform default from the catalog) and
// pair-checks it against pythonVersion. An empty pythonVersion means the
// agent isn't a python-buildpack build and the check is a no-op.
func (s *agentManagerService) validateEffectivePythonInstrumentationPair(pythonVersion string, requestedVersion *string) error {
	if pythonVersion == "" {
		return nil
	}
	effective := instrumentation.GetCatalog().Default()
	if requestedVersion != nil {
		effective = *requestedVersion
	}
	return s.validatePythonInstrumentationPair(pythonVersion, effective)
}

// validatePythonInstrumentationPair rejects an agent whose instrumentation
// version doesn't cover the chosen Python version. Both values are assumed
// to have passed their individual validations already; this exists because
// each catalog entry's pythonVersions field constrains which Python a
// version supports (the image tag is python-ABI-locked).
func (s *agentManagerService) validatePythonInstrumentationPair(pythonVersion, instrumentationVersion string) error {
	entry, ok := instrumentation.GetCatalog().Get(instrumentationVersion)
	if !ok {
		// Should have been caught by validateInstrumentationVersion;
		// defensive only.
		return fmt.Errorf("%w: instrumentationVersion %q not in catalog", utils.ErrInvalidInput, instrumentationVersion)
	}
	for _, p := range entry.PythonVersions {
		if p == pythonVersion {
			return nil
		}
	}
	return fmt.Errorf("%w: instrumentation %q does not support python %q (supports: %v)",
		utils.ErrInvalidInput, instrumentationVersion, pythonVersion, entry.PythonVersions)
}

// normalizePythonMinor collapses a Python runtime version to its bare
// major.minor form ("3.11.4" -> "3.11"), matching the shape stored in the
// instrumentation catalog. Returns "" when the value has fewer than two
// dot-separated components.
func normalizePythonMinor(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parts := strings.SplitN(raw, ".", 3)
	if len(parts) < 2 {
		return ""
	}
	major, minor := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
	if major == "" || minor == "" {
		return ""
	}
	return major + "." + minor
}

// resolveInstrumentationImageOverride resolves the per-environment
// instrumentationImage override for a Python buildpack agent. Precedence for the
// effective version is: request override -> existing pin. When a version is
// requested it is validated against the deployment catalog and the agent's Python
// version; the existing pin is trusted (it validated at creation) so a redeploy
// never fails on a catalog that has since changed.
//
// It returns the version to persist (nil when nothing is pinned) and the image to
// write into the OTEL trait's per-environment config ("" when no version is
// pinned, meaning the Component's create-time default stands). For non-Python
// agents it is a no-op that echoes the existing pin.
func (s *agentManagerService) resolveInstrumentationImageOverride(isPythonBuildpack bool, languageVersion string, requested, existing *string) (*string, string, error) {
	if !isPythonBuildpack {
		return existing, "", nil
	}
	pyMinor := normalizePythonMinor(languageVersion)
	version := existing
	if requested != nil {
		if err := s.validateInstrumentationVersion(*requested); err != nil {
			return nil, "", err
		}
		if err := s.validateEffectivePythonInstrumentationPair(pyMinor, requested); err != nil {
			return nil, "", err
		}
		version = requested
	}
	if version == nil || *version == "" {
		return version, "", nil
	}
	// For an existing (not freshly-requested) pin, guard against a Python
	// version that changed since the pin was set — build-parameters only
	// re-validates the lowest environment's pin, so a non-lowest env's pin can
	// silently become Python-incompatible. Building the image from an
	// incompatible pair (or an unparseable Python version) would yield an
	// init-container tag that doesn't exist (ImagePullBackOff). Keep the
	// persisted pin but skip the per-env image override so the deployment falls
	// back to the Component's image instead of a broken tag. A freshly-requested
	// version is already strictly validated above.
	if requested == nil {
		if pyMinor == "" {
			s.logger.Warn("Cannot determine Python version for instrumentation image; keeping component default",
				"languageVersion", languageVersion, "instrumentationVersion", *version)
			return version, "", nil
		}
		if err := s.validatePythonInstrumentationPair(pyMinor, *version); err != nil {
			s.logger.Warn("Pinned instrumentation version is not compatible with the current Python version; keeping component default",
				"languageVersion", languageVersion, "instrumentationVersion", *version, "error", err)
			return version, "", nil
		}
	}
	image, err := client.BuildInstrumentationImage(languageVersion, *version)
	if err != nil {
		if requested != nil {
			return nil, "", fmt.Errorf("%w: %s", utils.ErrInvalidInput, err.Error())
		}
		// Defensive: the compatibility guard above should already have caught
		// this; keep the Component default rather than failing a valid redeploy.
		s.logger.Warn("Failed to build instrumentation image from existing pin; keeping component default",
			"languageVersion", languageVersion, "instrumentationVersion", *version, "error", err)
		return version, "", nil
	}
	return version, image, nil
}

// persistInstrumentationConfig saves the instrumentation config to the database.
// instrumentationVersion is nil when the caller did not pin a specific version —
// the column stays NULL and the resolver falls back to the platform default.
func (s *agentManagerService) persistInstrumentationConfig(ctx context.Context, ouID, projectName, agentName string, enableAutoInstrumentation bool, instrumentationVersion *string) {
	// Get the first/lowest environment
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if err != nil {
		s.logger.Warn("Failed to get deployment pipeline for config persistence", "agentName", agentName, "error", err)
		return
	}

	lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
	if lowestEnv == "" {
		s.logger.Warn("No environment found for config persistence", "agentName", agentName)
		return
	}

	targetEnv, err := s.ocClient.GetEnvironment(ctx, ouID, lowestEnv)
	if err != nil {
		s.logger.Warn("Failed to get environment details for config persistence", "agentName", agentName, "environment", lowestEnv, "error", err)
		return
	}

	defaultCORS := config.GetAgentWorkloadConfig().CORS
	agentConfig := &models.AgentConfig{
		OUID:                      ouID,
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
		CORSAllowCredentials:      defaultCORS.AllowCredentials,
		// OAuth off by default; header columns are NOT NULL so set the defaults
		// explicitly (the Select("*") Upsert would otherwise write empty strings).
		EnableOAuthSecurity:   false,
		OAuthHeaderName:       models.DefaultOAuthHeaderName,
		OAuthAuthHeaderPrefix: models.DefaultOAuthAuthHeaderPrefix,
	}

	if err := s.agentConfigRepo.Upsert(agentConfig); err != nil {
		s.logger.Warn("Failed to persist instrumentation config to database", "agentName", agentName, "error", err)
	} else {
		s.logger.Debug("Persisted instrumentation config to database", "agentName", agentName, "environment", lowestEnv, "enableAutoInstrumentation", enableAutoInstrumentation, "instrumentationVersion", instrumentationVersion)
	}
}

// generateAgentAPIKey generates an agent API key (JWT token) for the agent
// This is a common utility used by both buildpack and docker agent instrumentation
func (s *agentManagerService) generateAgentAPIKey(ctx context.Context, ouID, projectName, agentName, envName string) (string, error) {
	// Extract OrgId from the caller's JWT claims
	callerClaims := jwtassertion.GetTokenClaims(ctx)
	if callerClaims == nil || callerClaims.OuId == "" {
		s.logger.Error("GenerateToken: missing organization identity in caller token")
		return "", utils.ErrForbidden
	}
	// Generate agent API key using the token manager service. Leaving ExpiresIn empty
	// routes through the configured default (JWT_SIGNING_DEFAULT_EXPIRY) rather than a
	// hardcoded value, so the deploy/create/promote expiry stays in sync with config.
	tokenReq := GenerateTokenRequest{
		OrgName:     ouID,
		ProjectName: projectName,
		AgentName:   agentName,
		Environment: envName,
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

// agentAPIKeySecretLocation returns the KV store location for an agent's API key in a
// given environment. The key is scoped per environment so each environment materializes
// its own ExternalSecret via the env-injection trait.
func agentAPIKeySecretLocation(ouID, projectName, agentName, envName string) secretmanagersvc.SecretLocation {
	return secretmanagersvc.SecretLocation{
		OrgName:         ouID,
		ProjectName:     projectName,
		EnvironmentName: envName,
		AgentName:       agentName,
		EntityName:      agentName + "-agent-api-key",
	}
}

// storeAgentAPIKey stores the agent API key (JWT) in the secret store and returns the
// remote reference (key/path + optional property) that the env-injection trait uses to
// build its ExternalSecret. This keeps the raw key out of the control plane — only the
// reference is ever passed to the trait.
//
// The reference is resolved from the SecretReference CR rather than computed locally:
// CreateSecret returns the SecretReference name, and only the SecretReference knows the
// provider's real remote reference. (The provider manages the SecretReference and its
// remoteRef internally — location.KVPath() is not the real reference, so we must read
// it back from the SecretReference.)
func (s *agentManagerService) storeAgentAPIKey(ctx context.Context, ouID, projectName, agentName, envName, apiKey string) (key string, property string, err error) {
	if s.secretMgmtClient == nil {
		return "", "", fmt.Errorf("secret management is not initialized; cannot store agent API key")
	}
	location := agentAPIKeySecretLocation(ouID, projectName, agentName, envName)
	secretRefName, err := s.secretMgmtClient.CreateSecret(ctx, location, map[string]string{
		secretmanagersvc.SecretKeyAPIKey: apiKey,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to store agent API key in secret store: %w", err)
	}

	key, property, err = s.resolveAgentAPIKeySecretRef(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		return "", "", err
	}
	s.logger.Debug("Stored agent API key in secret store", "agentName", agentName, "environment", envName,
		"secretRefName", secretRefName, "remoteKey", key)
	return key, property, nil
}

// resolveAgentAPIKeySecretRef returns the remote KV key/property of an environment's agent API key
// WITHOUT minting or storing anything — it derives the deterministic SecretReference name from the
// location and reads the existing reference. Used by the deploy-settings rebuild to re-attach the
// per-env secret ref that would otherwise be dropped when trait configs are replaced.
func (s *agentManagerService) resolveAgentAPIKeySecretRef(ctx context.Context, ouID, projectName, agentName, envName string) (key string, property string, err error) {
	location := agentAPIKeySecretLocation(ouID, projectName, agentName, envName)
	secretRefName := location.SecretRefName()
	secretRef, err := s.ocClient.GetSecretReference(ctx, ouID, secretRefName)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve agent API key secret reference %q: %w", secretRefName, err)
	}
	for _, ds := range secretRef.Data {
		if ds.SecretKey == secretmanagersvc.SecretKeyAPIKey {
			return ds.RemoteRef.Key, ds.RemoteRef.Property, nil
		}
	}
	return "", "", fmt.Errorf("agent API key secret reference %q has no %q data source", secretRefName, secretmanagersvc.SecretKeyAPIKey)
}

// injectAgentAPIKeySecretRef adds the env-injection trait's per-environment agentApiKeySecretRef
// (and property) into a traitEnvConfigs map, preserving any config already set for that trait
// instance. buildTraitEnvConfigs omits this field, and UpdateReleaseBindingTraitConfigs REPLACES
// the binding's trait configs wholesale — so without re-injecting here, a promoted environment
// loses its per-env key ref and the env-injection trait falls back to the base (lowest env's) ref.
func injectAgentAPIKeySecretRef(traitEnvConfigs map[string]interface{}, agentName, secretRef, secretProperty string) {
	if secretRef == "" {
		return
	}
	envInjKey := agentName + "-" + string(client.TraitEnvInjection)
	envInjCfg, _ := traitEnvConfigs[envInjKey].(map[string]interface{})
	if envInjCfg == nil {
		envInjCfg = map[string]interface{}{}
	}
	envInjCfg["agentApiKeySecretRef"] = secretRef
	if secretProperty != "" {
		envInjCfg["agentApiKeySecretProperty"] = secretProperty
	}
	traitEnvConfigs[envInjKey] = envInjCfg
}

// TracingTokenRotationResult is the outcome of a tracing-token regeneration. The raw JWT is never
// returned — it reaches the agent only through the secret store.
type TracingTokenRotationResult struct {
	EnvironmentName string
	ExpiresAt       int64
	RotatedAt       int64
}

// RegenerateAgentTracingToken mints a fresh tracing API key and upserts it into the secret store
// under the agent's stable secret reference. It does NOT restart the workload: the running pod
// picks up the new key on the next rollout, which the caller triggers via the standard Apply
// (UpdateAgentConfigurations) path — the same ExternalSecret + restartedAt mechanism used for every
// other secret. Rotating the key never invalidates previously issued ones; they remain valid until
// their own expiry.
func (s *agentManagerService) RegenerateAgentTracingToken(ctx context.Context, ouID, projectName, agentName, environmentName, expiresIn string) (*TracingTokenRotationResult, error) {
	s.logger.Info("Regenerating agent tracing token", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environmentName)

	if environmentName == "" {
		return nil, fmt.Errorf("%w: environmentName is required", utils.ErrInvalidInput)
	}

	// Org identity comes from the caller's JWT claims, same as generateAgentAPIKey.
	callerClaims := jwtassertion.GetTokenClaims(ctx)
	if callerClaims == nil || callerClaims.OuId == "" {
		s.logger.Error("RegenerateAgentTracingToken: missing organization identity in caller token")
		return nil, utils.ErrForbidden
	}

	// Validate org/agent/env exist. GetComponent resolves the OpenChoreo namespace from the OU id,
	// so pass ouID (matching GenerateToken's GenerateTokenRequest{OrgName: ouID}), not org.Name.
	if _, err := s.ocClient.GetOrganization(ctx, ouID); err != nil {
		return nil, translateOrgError(err)
	}
	if _, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName); err != nil {
		return nil, translateAgentError(err)
	}
	if _, err := s.ocClient.GetEnvironment(ctx, ouID, environmentName); err != nil {
		return nil, translateEnvironmentError(err)
	}

	tokenResp, err := s.tokenManagerService.GenerateToken(ctx, GenerateTokenRequest{
		OrgName:     ouID,
		ProjectName: projectName,
		AgentName:   agentName,
		Environment: environmentName,
		ExpiresIn:   expiresIn, // empty → configured default
		OrgId:       callerClaims.OuId,
	})
	if err != nil {
		s.logger.Error("Failed to generate agent tracing token", "agentName", agentName, "environment", environmentName, "error", err)
		return nil, fmt.Errorf("failed to generate agent tracing token: %w", err)
	}

	if _, _, err := s.storeAgentAPIKey(ctx, ouID, projectName, agentName, environmentName, tokenResp.Token); err != nil {
		s.logger.Error("Failed to store rotated agent tracing token", "agentName", agentName, "environment", environmentName, "error", err)
		return nil, fmt.Errorf("failed to store agent tracing token: %w", err)
	}

	return &TracingTokenRotationResult{
		EnvironmentName: environmentName,
		ExpiresAt:       tokenResp.ExpiresAt,
		RotatedAt:       time.Now().Unix(),
	}, nil
}

func (s *agentManagerService) GetAgent(ctx context.Context, ouID string, projectName string, agentName string) (*models.AgentResponse, error) {
	s.logger.Info("Getting agent", "agentName", agentName, "ouID", ouID, "projectName", projectName)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return nil, translateOrgError(err)
	}
	agent, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	// Populate per-environment agent configuration from database
	// Get the first/lowest environment to read the config
	pipeline, pipelineErr := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if pipelineErr == nil && len(pipeline.PromotionPaths) > 0 {
		lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
		if lowestEnv != "" {
			agentConfig, configErr := s.agentConfigRepo.Get(ouID, projectName, agentName, lowestEnv)
			if errors.Is(configErr, repositories.ErrAgentConfigNotFound) {
				// No config in DB - use defaults for display purposes
				defaultEnabled := true
				defaultCORSEnabled := true
				defaultOAuthDisabled := false
				defCORS := config.GetAgentWorkloadConfig().CORS
				agent.Configurations = &models.Configurations{
					EnableAutoInstrumentation: &defaultEnabled,
					EnableApiKeySecurity:      &defaultEnabled,
					CorsConfig: &models.CorsConfig{
						Enabled:          &defaultCORSEnabled,
						AllowOrigin:      strings.Split(defCORS.AllowOrigin, ","),
						AllowMethods:     strings.Split(defCORS.AllowMethods, ","),
						AllowHeaders:     strings.Split(defCORS.AllowHeaders, ","),
						AllowCredentials: &defCORS.AllowCredentials,
					},
					EnableOAuthSecurity: &defaultOAuthDisabled,
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
					EnableOAuthSecurity:      &agentConfig.EnableOAuthSecurity,
					OAuthConfig:              oauthConfigFromAgentConfig(agentConfig),
					ResilienceTimeoutSeconds: agentConfig.ResilienceTimeoutSeconds,
				}
			}

			// Populate env vars for internal agents (non-fatal if it fails)
			if agent.Provisioning.Type == string(utils.InternalAgent) {
				if envConfigs, envErr := s.ocClient.GetComponentConfigurations(ctx, ouID, projectName, agentName, lowestEnv); envErr == nil {
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

	s.logger.Info("Fetched agent successfully from oc", "agentName", agent.Name, "ouID", ouID, "projectName", projectName, "provisioningType", agent.Provisioning.Type)
	return agent, nil
}

func (s *agentManagerService) ListAgents(ctx context.Context, ouID string, projName string, labelFilter map[string]string, limit int32, offset int32) ([]*models.AgentResponse, int32, error) {
	s.logger.Info("Listing agents", "ouID", ouID, "projectName", projName, "limit", limit, "offset", offset)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return nil, 0, translateOrgError(err)
	}

	// Fetch all agent components
	agents, err := s.ocClient.ListComponents(ctx, ouID, projName)
	if err != nil {
		s.logger.Error("Failed to list agents from repository", "ouID", ouID, "projectName", projName, "error", err)
		return nil, 0, fmt.Errorf("failed to list agents: %w", err)
	}

	// Filter before computing total and paginating so both stay correct.
	// agent.Labels already arrives populated from the component's own
	// metadata (see convertComponentFromTyped), no separate fetch needed.
	if len(labelFilter) > 0 {
		filtered := make([]*models.AgentResponse, 0, len(agents))
		for _, agent := range agents {
			if utils.LabelsMatch(agent.Labels, labelFilter) {
				filtered = append(filtered, agent)
			}
		}
		agents = filtered
	}

	total := int32(len(agents))
	paginatedAgents := paginateSlice(agents, offset, limit)
	s.logger.Info("Listed agents successfully", "ouID", ouID, "projName", projName, "totalAgents", total, "returnedAgents", len(paginatedAgents))
	return paginatedAgents, total, nil
}

// paginateSlice returns items[offset:offset+limit], clamping both bounds defensively so
// negative values or out-of-range offsets never panic the slice expression.
func paginateSlice[T any](items []T, offset, limit int32) []T {
	total := int32(len(items))
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}
	if offset >= total {
		return []T{}
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return items[offset:end]
}

func (s *agentManagerService) CreateAgent(ctx context.Context, ouID string, projectName string, req *spec.CreateAgentRequest) error {
	var requestedVersion *string
	autoInstr := true
	if req.Configurations != nil {
		requestedVersion = req.Configurations.InstrumentationVersion.Get()
		if req.Configurations.EnableAutoInstrumentation != nil {
			autoInstr = *req.Configurations.EnableAutoInstrumentation
		}
		if requestedVersion != nil {
			if err := s.validateInstrumentationVersion(*requestedVersion); err != nil {
				return err
			}
		}
	}

	imageID := ""
	if req.Provisioning.AgentKind != nil {
		kindVersion, err := s.agentKindService.GetKindVersion(ctx, ouID, req.Provisioning.AgentKind.Name, req.Provisioning.AgentKind.Version)
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
		sourceComponent, err := s.ocClient.GetComponent(ctx, ouID, kindVersion.Kind.ProjectName, kindVersion.Kind.AgentName)
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
				req.InputInterface.Schema = &spec.InputInterfaceSchema{Path: &sourceComponent.InputInterface.Schema.Path}
			}
		}
		imageID = kindVersion.ImageId
	}

	// Pair-check the python/instrumentation combo after any kind-based
	// build replacement above, so we validate against the build that
	// will actually be deployed (not the one in the original request,
	// which is empty for kind-based agents). The check runs whenever
	// the deploy will use this version: either the user pinned one
	// (intent must stay consistent), or auto-instrumentation is on and
	// the default will be injected (otherwise the init-container image
	// won't exist).
	if requestedVersion != nil || autoInstr {
		if err := s.validateEffectivePythonInstrumentationPair(buildpackPythonVersion(req.Build), requestedVersion); err != nil {
			return err
		}
	}
	return s.createComponentAgent(ctx, ouID, projectName, req, imageID)
}

// createComponentAgent is the shared agent creation flow for all internal agents.
// For source-based (imageID == ""): CreateComponent (with Workflow) → AttachTraits → TriggerBuild
// For kind-based (imageID != ""): CreateComponent (no Workflow) → AttachTraits → CreateInternalAgentFromKindWorkload
func (s *agentManagerService) createComponentAgent(ctx context.Context, ouID, projectName string, req *spec.CreateAgentRequest, imageID string) error {
	s.logger.Info("Creating agent", "agentName", req.Name, "ouID", ouID, "projectName", projectName, "provisioningType", req.Provisioning.Type)

	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return translateOrgError(err)
	}

	if req.Provisioning.Repository != nil && req.Provisioning.Repository.SecretRef.Get() != nil {
		if err := s.validateGitSecretExists(ctx, ouID, req.Provisioning.Repository.GetSecretRef()); err != nil {
			return err
		}
	}

	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to get deployment pipeline", "projectName", projectName, "error", err)
		return translatePipelineError(err)
	}
	firstEnv := findLowestEnvironment(pipeline.PromotionPaths)
	if firstEnv == "" {
		s.logger.Error("No environment found in deployment pipeline", "projectName", projectName)
		return fmt.Errorf("no environment found in deployment pipeline")
	}

	// Preflight: validate referenced LLM providers before creating any secrets or
	// the component, so a bad provider fails fast with no resources to roll back.
	if len(req.ModelConfig) > 0 {
		handles := make([]string, 0, len(req.ModelConfig))
		for _, mc := range req.ModelConfig {
			handles = append(handles, mc.ProviderName)
		}
		if err := s.agentConfigurationService.ValidateProvidersInCatalog(ctx, ouID, handles); err != nil {
			return err
		}
	}

	// Preflight: validate referenced MCP proxies before creating any secrets or
	// the component, so a bad proxy fails fast with no resources to roll back.
	if len(req.McpConfig) > 0 {
		handles := make([]string, 0, len(req.McpConfig))
		for _, mc := range req.McpConfig {
			handles = append(handles, mc.ProxyName)
		}
		if err := s.agentConfigurationService.ValidateMCPProxiesInCatalog(ctx, ouID, handles); err != nil {
			return err
		}
	}

	secretLocation := secretmanagersvc.SecretLocation{
		OrgName:         ouID,
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
	if err := s.ocClient.CreateComponent(ctx, ouID, projectName, createAgentReq); err != nil {
		s.logger.Error("Failed to create agent component", "agentName", req.Name, "error", err)
		if hasSecrets {
			s.cleanupSecretsOnRollback(ctx, secretLocation)
		}
		return err
	}

	var agentAPIArtifact *models.Artifact
	if req.AgentType.Type == string(utils.AgentTypeAPI) {
		firstEnvDetails, envErr := s.ocClient.GetEnvironment(ctx, ouID, firstEnv)
		if envErr != nil {
			s.logger.Error("Failed to get environment details", "environment", firstEnv, "error", envErr)
			if hasSecrets {
				s.cleanupSecretsOnRollback(ctx, secretLocation)
			}
			if errDeletion := s.ocClient.DeleteComponent(ctx, ouID, projectName, req.Name); errDeletion != nil {
				s.logger.Error("Failed to rollback agent component after environment lookup failure", "agentName", req.Name, "error", errDeletion)
			}
			return translateEnvironmentError(envErr)
		}
		agentAPIArtifact, err = ensureAgentEnvAPIArtifact(s.db, s.artifactRepo, ouID, projectName, req.Name, firstEnvDetails.UUID)
		if err != nil {
			s.logger.Error("Failed to create agent API artifact record", "agentName", req.Name, "environment", firstEnv, "environmentUUID", firstEnvDetails.UUID, "error", err)
			if hasSecrets {
				s.cleanupSecretsOnRollback(ctx, secretLocation)
			}
			if errDeletion := s.ocClient.DeleteComponent(ctx, ouID, projectName, req.Name); errDeletion != nil {
				s.logger.Error("Failed to rollback agent component after API artifact create failure", "agentName", req.Name, "error", errDeletion)
			}
			return fmt.Errorf("failed to create agent API artifact record: %w", err)
		}
	}

	rollbackAgentCreate := func(reason string) {
		// Best-effort cleanup of any LLM/MCP configurations created before the failure.
		// Each per-config create rolls back only its own in-flight resources, so configs
		// completed earlier in the loop (and their proxies/mappings/keys/secrets) would
		// otherwise be orphaned once the component is deleted below. Use a non-cancellable
		// context so cleanup still runs if the request context was cancelled.
		isExternalAgent := req.Provisioning.Type == string(utils.ExternalAgent)
		s.deleteAgentLLMConfigurations(context.WithoutCancel(ctx), ouID, projectName, req.Name, isExternalAgent)

		if hasSecrets {
			s.cleanupSecretsOnRollback(ctx, secretLocation)
		}
		if errDeletion := s.ocClient.DeleteComponent(ctx, ouID, projectName, req.Name); errDeletion != nil {
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
		if err := s.createAgentLLMConfigs(ctx, ouID, projectName, firstEnv, req); err != nil {
			s.logger.Error("Failed to create LLM configurations for agent", "agentName", req.Name, "error", err)
			rollbackAgentCreate("LLM config failure")
			return err
		}
	}

	// Create MCP proxy mapping configurations (applies to both internal and external agents)
	if len(req.McpConfig) > 0 {
		if err := s.createAgentMCPConfigs(ctx, ouID, projectName, firstEnv, req); err != nil {
			s.logger.Error("Failed to create MCP configurations for agent", "agentName", req.Name, "error", err)
			rollbackAgentCreate("MCP config failure")
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
		// Resolve the lowest environment for API key generation
		createPipeline, pipeErr := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
		var lowestEnvName string
		if pipeErr == nil {
			lowestEnvName = findLowestEnvironment(createPipeline.PromotionPaths)
		}
		traitRequests, err := s.buildCreateTraitRequests(ctx, ouID, projectName, artifactID, lowestEnvName, req)
		if err != nil {
			s.logger.Error("Failed to build trait requests", "agentName", req.Name, "error", err)
			rollbackAgentCreate("trait build failure")
			return err
		}

		if len(traitRequests) > 0 {
			if err := s.ocClient.AttachTraits(ctx, ouID, projectName, req.Name, traitRequests); err != nil {
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
			// Kind-sourced agents bypass the build/workflow system, so the LLM env vars
			// written into the Component workflow params by createAgentLLMConfigs never reach
			// the container. The Workload CR is authoritative for these agents, so resolve the
			// system-managed LLM env vars from the persisted config and inject them here.
			kindEnvVars, envErr := s.mergeKindWorkloadLLMEnvVars(ctx, req.Name, ouID, projectName, firstEnv, kindEnvVars, len(req.ModelConfig) > 0)
			if envErr != nil {
				s.logger.Error("Failed to resolve LLM env vars for kind-sourced agent workload", "agentName", req.Name, "environment", firstEnv, "error", envErr)
				rollbackAgentCreate("LLM env var resolution failure")
				return envErr
			}
			kindEndpoints := inputInterfaceToEndpoints(createAgentReq.InputInterface, req.Name)
			if err := s.ocClient.CreateInternalAgentFromKindWorkload(ctx, ouID, projectName, req.Name, client.InternalAgentFromKindWorkloadRequest{
				ImageID:   imageID,
				Endpoints: kindEndpoints,
				Env:       kindEnvVars,
				Files:     kindFileVars,
			}); err != nil {
				s.logger.Error("Failed to create internal-agent-from-kind workload", "agentName", req.Name, "error", err)
				if hasSecrets {
					s.cleanupSecretsOnRollback(ctx, secretLocation)
				}
				if errDeletion := s.ocClient.DeleteComponent(ctx, ouID, projectName, req.Name); errDeletion != nil {
					s.logger.Error("Failed to rollback agent creation after kind-workload failure", "agentName", req.Name, "error", errDeletion)
				}
				return err
			}
			s.logger.Info("Created internal-agent-from-kind workload", "agentName", req.Name)
		} else {
			if err := s.triggerInitialBuild(ctx, ouID, projectName, req); err != nil {
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
		s.persistInstrumentationConfig(ctx, ouID, projectName, req.Name, enableAutoInstrumentation, instrumentationVersion)
	}

	// AgentID provisioning: one Thunder identity per org-level environment (not
	// just this project's deployment pipeline — see the AgentID architecture
	// doc, which project-level UIs filter down to their own pipeline
	// environments when displaying bindings). Runs after every step above has
	// succeeded, so a Thunder hiccup never rolls back an otherwise-successful
	// agent creation; the write-ahead + retry reconciler make it durable
	// without needing a place in the rollback chain above. Skipped when this
	// deployment injects no provisioning implementation.
	if s.agentThunderProvisioning != nil {
		if envs, envErr := s.ocClient.ListEnvironments(ctx, ouID); envErr != nil {
			s.logger.Warn("Failed to list org environments for agent thunder provisioning", "agentName", req.Name, "error", envErr)
		} else if len(envs) > 0 {
			envNames := make([]string, 0, len(envs))
			for _, e := range envs {
				envNames = append(envNames, e.Name)
			}
			ownership := models.AgentProvisioningType(req.Provisioning.Type)
			// requestedBy is captured now, synchronously, because the retry reconciler
			// may not attempt some of these bindings until minutes or hours later, long
			// after this request's own caller identity would otherwise be gone. It is
			// AMS's own audit record only — never sent to Thunder (see
			// models.AgentThunderClient.RequestedBy for why Thunder's own "owner" field
			// cannot carry this).
			var requestedBy string
			if callerClaims := jwtassertion.GetTokenClaims(ctx); callerClaims != nil {
				requestedBy = callerClaims.Sub
			}
			s.agentThunderProvisioning.ProvisionForAgent(ctx, ouID, projectName, req.Name, ownership, envNames, requestedBy)
		}
	}

	s.logger.Info("Agent created successfully", "agentName", req.Name, "ouID", ouID, "projectName", projectName, "provisioningType", req.Provisioning.Type)
	return nil
}

func (s *agentManagerService) triggerInitialBuild(ctx context.Context, ouID, projectName string, req *spec.CreateAgentRequest) error {
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
	build, err := s.ocClient.TriggerBuild(ctx, ouID, projectName, req.Name, commitId)
	if err != nil {
		return fmt.Errorf("failed to trigger initial build: agentName %s, error: %w", req.Name, err)
	}
	s.logger.Info("Agent component created and build triggered successfully", "agentName", req.Name, "ouID", ouID, "projectName", projectName, "buildName", build.Name, "commitId", commitId)
	return nil
}

// mergeKindWorkloadLLMEnvVars appends the system-managed LLM env vars (proxy URL + API key
// secret ref) for the first environment onto the user-supplied env vars of a kind-sourced agent.
// Kind-sourced agents create their Workload CR directly, bypassing the build/workflow system that
// otherwise carries these vars into the container, so they must be injected into the Workload here.
// When the agent has no LLM configuration, userEnvVars is returned unchanged.
func (s *agentManagerService) mergeKindWorkloadLLMEnvVars(
	ctx context.Context, agentName, ouID, projectName, firstEnv string, userEnvVars []client.EnvVar, hasModelConfig bool,
) ([]client.EnvVar, error) {
	if !hasModelConfig {
		return userEnvVars, nil
	}
	llmEnvVars, err := s.agentConfigurationService.BuildSystemManagedEnvVarsFromConfig(ctx, agentName, ouID, projectName, firstEnv)
	if err != nil {
		return nil, fmt.Errorf("failed to build system-managed LLM env vars: agentName %s, ouID %s, projectName %s, env %s, error: %w",
			agentName, ouID, projectName, firstEnv, err)
	}
	return append(userEnvVars, llmEnvVars...), nil
}

func (s *agentManagerService) createAgentLLMConfigs(
	ctx context.Context, ouID, projectName, firstEnv string, req *spec.CreateAgentRequest,
) error {
	for i, mc := range req.ModelConfig {
		configName := fmt.Sprintf("%s-llm-config", req.Name)
		if len(req.ModelConfig) > 1 {
			configName = fmt.Sprintf("%s-llm-config-%d", req.Name, i+1)
		}
		cfg := models.EnvProviderConfiguration{}
		if mc.Configuration != nil {
			cfg = convertConfiguration(*mc.Configuration)
		}
		createReq := models.CreateAgentModelConfigRequest{
			Name: configName,
			Type: "llm",
			EnvMappings: map[string]models.EnvModelConfigRequest{
				firstEnv: {
					ProviderName:  mc.ProviderName,
					Configuration: cfg,
				},
			},
			EnvironmentVariables: convertEnvVars(mc.EnvironmentVariables),
		}
		if _, err := s.agentConfigurationService.Create(ctx, ouID, projectName, req.Name, createReq, "system"); err != nil {
			return fmt.Errorf("failed to create LLM configuration %d: %w", i+1, err)
		}
	}
	return nil
}

func (s *agentManagerService) createAgentMCPConfigs(
	ctx context.Context, ouID, projectName, firstEnv string, req *spec.CreateAgentRequest,
) error {
	for i, mc := range req.McpConfig {
		configName := fmt.Sprintf("%s-mcp-config", req.Name)
		if len(req.McpConfig) > 1 {
			configName = fmt.Sprintf("%s-mcp-config-%d", req.Name, i+1)
		}
		createReq := models.CreateAgentModelConfigRequest{
			Name: configName,
			Type: models.AgentConfigTypeMCP,
			EnvMappings: map[string]models.EnvModelConfigRequest{
				firstEnv: {
					// createMCPConfig reads the MCP proxy handle from ProviderName.
					ProviderName: mc.ProxyName,
				},
			},
			EnvironmentVariables: convertEnvVars(mc.EnvironmentVariables),
		}
		if _, err := s.agentConfigurationService.Create(ctx, ouID, projectName, req.Name, createReq, "system"); err != nil {
			return fmt.Errorf("failed to create MCP configuration %d: %w", i+1, err)
		}
	}
	return nil
}

// convertConfiguration maps a generated provider configuration to the model type,
// preserving guardrail policies. Replaces the per-env conversion that convertEnvMappings did.
func convertConfiguration(cfg spec.EnvProviderConfiguration) models.EnvProviderConfiguration {
	policies := make([]models.LLMPolicy, 0, len(cfg.Policies))
	for _, p := range cfg.Policies {
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
	return models.EnvProviderConfiguration{Policies: policies}
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

	var labels map[string]string
	if req.Labels != nil {
		labels = *req.Labels
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
		Labels:           labels,
	}

	result.Configurations = mapConfigurationsWithSecrets(req.Configurations, secretReferences)

	return result
}

// saveSecretsAndCreateReference stores secrets via the secret management client; the
// provider stores the values and manages the associated SecretReference internally.
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

func (s *agentManagerService) UpdateAgentBasicInfo(ctx context.Context, ouID string, projectName string, agentName string, req *spec.UpdateAgentBasicInfoRequest) (*models.AgentResponse, error) {
	s.logger.Info("Updating agent basic info", "agentName", agentName, "ouID", ouID, "projectName", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return nil, translateOrgError(err)
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", ouID, "error", err)
		return nil, translateProjectError(err)
	}

	// Fetch existing agent to validate it exists
	_, err = s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch existing agent", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}
	// Update agent basic info in OpenChoreo. Labels: nil means "leave
	// unchanged", a non-nil (possibly empty) map replaces the user-label set
	// while system-managed labels are preserved (see UpdateComponentBasicInfo).
	updateReq := client.UpdateComponentBasicInfoRequest{
		DisplayName: req.DisplayName,
		Description: req.Description,
		Labels:      req.Labels,
	}
	if err := s.ocClient.UpdateComponentBasicInfo(ctx, ouID, projectName, agentName, updateReq); err != nil {
		s.logger.Error("Failed to update agent meta data in OpenChoreo", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to update agent basic info: %w", err)
	}

	// Fetch agent to return current state (labels come pre-populated from
	// the component's own metadata).
	updatedAgent, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	s.logger.Info("Agent basic info update called", "agentName", agentName, "ouID", ouID, "projectName", projectName)
	return updatedAgent, nil
}

func (s *agentManagerService) UpdateAgentBuildParameters(ctx context.Context, ouID string, projectName string, agentName string, req *spec.UpdateAgentBuildParametersRequest) (*models.AgentResponse, error) {
	s.logger.Info("Updating agent build parameters", "agentName", agentName, "ouID", ouID, "projectName", projectName)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return nil, translateOrgError(err)
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", ouID, "error", err)
		return nil, translateProjectError(err)
	}

	// Validate git secret exists if specified
	if req.Provisioning.Repository != nil && req.Provisioning.Repository.SecretRef.Get() != nil {
		if err := s.validateGitSecretExists(ctx, ouID, req.Provisioning.Repository.GetSecretRef()); err != nil {
			return nil, err
		}
	}

	// Fetch existing agent to validate immutable fields
	existingAgent, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch existing agent", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
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

	// Re-validate the python/instrumentation pair. The build params
	// payload can flip the agent's Python version, and optionally
	// override its pinned instrumentation, either of which would
	// otherwise leave the deploy pointing at an init-container image
	// tag that doesn't exist. The check is skipped when neither path
	// will inject an init-container: no explicit pin in the request
	// AND auto-instrumentation is off on the agent's effective config.
	// Mirrors the gate in CreateAgent.
	if py := buildpackPythonVersion(&req.Build); py != "" {
		var requestedVersion *string
		if req.Configurations != nil {
			requestedVersion = req.Configurations.InstrumentationVersion.Get()
			if requestedVersion != nil {
				if err := s.validateInstrumentationVersion(*requestedVersion); err != nil {
					return nil, err
				}
			}
		}
		// Resolve effective auto-instrumentation: request override if
		// provided, otherwise the persisted value on the agent.
		var autoInstr bool
		if req.Configurations != nil && req.Configurations.EnableAutoInstrumentation != nil {
			autoInstr = *req.Configurations.EnableAutoInstrumentation
		} else {
			persisted, lookupErr := s.lookupAgentAutoInstrumentation(ctx, ouID, projectName, agentName)
			if lookupErr != nil {
				return nil, lookupErr
			}
			autoInstr = persisted
		}
		if requestedVersion != nil || autoInstr {
			// No new pin: validate against the agent's currently-pinned
			// version (or the platform default if none).
			if requestedVersion == nil {
				pinned, lookupErr := s.lookupAgentInstrumentationVersion(ctx, ouID, projectName, agentName)
				switch {
				case errors.Is(lookupErr, ErrInstrumentationVersionNotPinned):
					// Leave nil; helper resolves to catalog default.
				case lookupErr != nil:
					return nil, lookupErr
				default:
					requestedVersion = pinned
				}
			}
			if err := s.validateEffectivePythonInstrumentationPair(py, requestedVersion); err != nil {
				return nil, err
			}
		}
	}

	// Update agent build parameters in OpenChoreo
	updateReq := buildUpdateBuildParametersRequest(req)
	if err := s.ocClient.UpdateComponentBuildParameters(ctx, ouID, projectName, agentName, updateReq); err != nil {
		s.logger.Error("Failed to update agent build parameters in OpenChoreo", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to update agent build parameters: %w", err)
	}

	// Fetch agent to return current state
	updatedAgent, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	s.logger.Info("Agent build parameters updated successfully", "agentName", agentName, "ouID", ouID, "projectName", projectName)
	return updatedAgent, nil
}

func (s *agentManagerService) GetAgentResourceConfigs(ctx context.Context, ouID string, projectName string, agentName string, environment string) (*spec.AgentResourceConfigsResponse, error) {
	s.logger.Info("Getting agent resource configurations", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return nil, translateOrgError(err)
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", ouID, "error", err)
		return nil, translateProjectError(err)
	}

	// Validate agent exists
	_, err = s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	_, err = s.ocClient.GetEnvironment(ctx, ouID, environment)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environment, "ouID", ouID, "error", err)
		return nil, translateEnvironmentError(err)
	}

	// Fetch resource configurations from OpenChoreo
	configs, err := s.ocClient.GetEnvResourceConfigs(ctx, ouID, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch agent resource configurations", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment, "error", err)
		return nil, fmt.Errorf("failed to get agent resource configurations: %w", err)
	}

	// Convert client response to spec response
	response := buildResourceConfigsResponse(configs)

	s.logger.Info("Fetched agent resource configurations successfully", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment)
	return response, nil
}

func (s *agentManagerService) UpdateAgentResourceConfigs(ctx context.Context, ouID string, projectName string, agentName string, environment string, req *spec.UpdateAgentResourceConfigsRequest) (*spec.AgentResourceConfigsResponse, error) {
	s.logger.Info("Updating agent resource configurations", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment)

	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return nil, translateOrgError(err)
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", ouID, "error", err)
		return nil, translateProjectError(err)
	}

	// Fetch existing agent to validate it exists
	_, err = s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch existing agent", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, translateAgentError(err)
	}

	// Validate environment (required)
	_, err = s.ocClient.GetEnvironment(ctx, ouID, environment)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environment, "ouID", ouID, "error", err)
		return nil, translateEnvironmentError(err)
	}

	// Fetch the agent's current effective resource configuration so a partial update
	// (e.g. requests-only, leaving limits untouched) can be validated against whatever
	// stays in effect for the side it didn't touch.
	currentConfigs, err := s.GetAgentResourceConfigs(ctx, ouID, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch current agent resource configurations", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment, "error", err)
		return nil, fmt.Errorf("failed to get current agent resource configurations: %w", err)
	}
	if err := utils.ValidateResourceRequestsWithinLimits(req.Resources, currentConfigs.Resources); err != nil {
		s.logger.Error("Rejected agent resource configuration update", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment, "error", err)
		return nil, err
	}

	// Update agent resource configurations in OpenChoreo
	updateReq := buildUpdateResourceConfigsRequest(req)
	if err := s.ocClient.UpdateEnvResourceConfigs(ctx, ouID, projectName, agentName, environment, updateReq); err != nil {
		s.logger.Error("Failed to update agent resource configurations in OpenChoreo", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment, "error", err)
		return nil, fmt.Errorf("failed to update agent resource configurations: %w", err)
	}

	// Fetch updated resource configurations to return
	updatedConfigs, err := s.GetAgentResourceConfigs(ctx, ouID, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch updated resource configurations", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment, "error", err)
		return nil, fmt.Errorf("failed to get agent resource configurations: %w", err)
	}

	s.logger.Info("Agent resource configurations updated successfully", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment)
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

func (s *agentManagerService) GenerateName(ctx context.Context, ouID string, payload spec.ResourceNameRequest) (string, error) {
	s.logger.Info("Generating resource name", "resourceType", payload.ResourceType, "displayName", payload.DisplayName, "ouID", ouID)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return "", translateOrgError(err)
	}

	// Generate candidate name from display name
	candidateName := utils.GenerateCandidateName(payload.DisplayName)
	s.logger.Debug("Generated candidate name", "candidateName", candidateName, "displayName", payload.DisplayName)

	if payload.ResourceType == string(utils.ResourceTypeAgent) {
		projectName := utils.StrPointerAsStr(payload.ProjectName, "")
		// Validates the project name by checking its existence
		project, err := s.ocClient.GetProject(ctx, ouID, projectName)
		if err != nil {
			s.logger.Error("Failed to find project", "projectName", projectName, "org", ouID, "error", err)
			return "", translateProjectError(err)
		}

		// Check if candidate name is available
		exists, err := s.ocClient.ComponentExists(ctx, org.Name, project.Name, candidateName)
		if err != nil {
			return "", fmt.Errorf("failed to check agent existence: %w", err)
		}
		if !exists {
			return candidateName, nil
		}

		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueAgentName(ctx, org.Name, project.Name, candidateName)
		if err != nil {
			s.logger.Error("Failed to generate unique agent name", "baseName", candidateName, "ouID", org.Name, "projectName", project.Name, "error", err)
			return "", fmt.Errorf("failed to generate unique agent name: %w", err)
		}
		s.logger.Info("Generated unique agent name", "agentName", uniqueName, "ouID", ouID, "projectName", projectName)
		return uniqueName, nil
	}
	if payload.ResourceType == string(utils.ResourceTypeProject) {
		// Check if candidate name is available
		_, err = s.ocClient.GetProject(ctx, org.Name, candidateName)
		if err != nil && errors.Is(translateProjectError(err), utils.ErrProjectNotFound) {
			// Name is available, return it
			s.logger.Info("Generated unique project name", "projectName", candidateName, "ouID", ouID)
			return candidateName, nil
		}
		if err != nil {
			s.logger.Error("Failed to check project name availability", "name", candidateName, "ouID", org.Name, "error", err)
			return "", fmt.Errorf("failed to check project name availability: %w", err)
		}
		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueProjectName(ctx, org.Name, candidateName)
		if err != nil {
			s.logger.Error("Failed to generate unique project name", "baseName", candidateName, "ouID", org.Name, "error", err)
			return "", fmt.Errorf("failed to generate unique project name: %w", err)
		}
		s.logger.Info("Generated unique project name", "projectName", uniqueName, "ouID", ouID)
		return uniqueName, nil
	}
	if payload.ResourceType == string(utils.ResourceTypeEnvironment) {
		// Env names are bounded tighter than other resources so the gateway runtime
		// Service name fits within k8s's 63-char metadata.name limit.
		maxEnvLen := utils.MaxEnvNameLength(org.Name)
		if len(candidateName) > maxEnvLen {
			candidateName = strings.TrimRight(candidateName[:maxEnvLen], "-")
		}
		// Check if candidate name is available
		_, err = s.ocClient.GetEnvironment(ctx, org.Name, candidateName)
		if err != nil && errors.Is(translateEnvironmentError(err), utils.ErrEnvironmentNotFound) {
			// Name is available, return it
			s.logger.Info("Generated unique env name", "envName", candidateName, "ouID", ouID)
			return candidateName, nil
		}
		if err != nil {
			s.logger.Error("Failed to check env name availability", "name", candidateName, "ouID", org.Name, "error", err)
			return "", fmt.Errorf("failed to check env name availability: %w", err)
		}
		// Name is taken, generate unique name with suffix
		uniqueName, err := s.generateUniqueEnvName(ctx, ouID, org.Name, candidateName)
		if err != nil {
			s.logger.Error("Failed to generate unique env name", "baseName", candidateName, "ouID", org.Name, "error", err)
			return "", fmt.Errorf("failed to generate unique env name: %w", err)
		}
		s.logger.Info("Generated unique env name", "envName", uniqueName, "ouID", ouID)
		return uniqueName, nil

	}
	return "", errors.New("invalid resource type for name generation")
}

// generateUniqueProjectName creates a unique name by appending a random suffix
func (s *agentManagerService) generateUniqueProjectName(ctx context.Context, ouID string, baseName string) (string, error) {
	// Create a name availability checker function that uses the project repository
	nameChecker := func(name string) (bool, error) {
		_, err := s.ocClient.GetProject(ctx, ouID, name)
		if err != nil && errors.Is(translateProjectError(err), utils.ErrProjectNotFound) {
			// Name is available
			return true, nil
		}
		if err != nil {
			s.logger.Error("Failed to check project name availability", "name", name, "ouID", ouID, "error", err)
			return false, fmt.Errorf("failed to check project name availability: %w", err)
		}
		// Name is taken
		return false, nil
	}

	// Use the common unique name generation logic from utils
	uniqueName, err := utils.GenerateUniqueNameWithSuffix(baseName, nameChecker)
	if err != nil {
		s.logger.Error("Failed to generate unique project name", "baseName", baseName, "ouID", ouID, "error", err)
		return "", fmt.Errorf("failed to generate unique project name: %w", err)
	}

	return uniqueName, nil
}

// generateUniqueEnvName creates a unique name by appending a random suffix
func (s *agentManagerService) generateUniqueEnvName(ctx context.Context, ouID string, orgName string, baseName string) (string, error) {
	// Bound the base so the resulting "<base>-XX" stays within the per-org env-name
	// limit (which keeps the gateway runtime Service name ≤ 63 chars).
	maxBaseLen := utils.MaxEnvNameLength(orgName) - utils.RandomSuffixLength - 1 // 1 for hyphen
	if maxBaseLen < 1 {
		maxBaseLen = 1
	}
	if len(baseName) > maxBaseLen {
		baseName = strings.TrimRight(baseName[:maxBaseLen], "-")
	}

	// Create a name availability checker function that uses the project repository
	nameChecker := func(name string) (bool, error) {
		_, err := s.ocClient.GetEnvironment(ctx, ouID, name)
		if err != nil && errors.Is(translateEnvironmentError(err), utils.ErrEnvironmentNotFound) {
			// Name is available
			return true, nil
		}
		if err != nil {
			s.logger.Error("Failed to check env name availability", "name", name, "ouID", ouID, "error", err)
			return false, fmt.Errorf("failed to check env name availability: %w", err)
		}
		// Name is taken
		return false, nil
	}

	// Use the common unique name generation logic from utils
	uniqueName, err := utils.GenerateUniqueNameWithSuffix(baseName, nameChecker)
	if err != nil {
		s.logger.Error("Failed to generate unique env name", "baseName", baseName, "ouID", ouID, "error", err)
		return "", fmt.Errorf("failed to generate unique env name: %w", err)
	}

	return uniqueName, nil
}

// generateUniqueAgentName creates a unique name by appending a random suffix
func (s *agentManagerService) generateUniqueAgentName(ctx context.Context, ouID string, projectName string, baseName string) (string, error) {
	// Create a name availability checker function that uses the agent repository
	nameChecker := func(name string) (bool, error) {
		exists, err := s.ocClient.ComponentExists(ctx, ouID, projectName, name)
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

func (s *agentManagerService) DeleteAgent(ctx context.Context, ouID string, projectName string, agentName string) error {
	s.logger.Info("Deleting agent", "agentName", agentName, "ouID", ouID, "projectName", projectName)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return translateOrgError(err)
	}
	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "ouID", ouID, "error", err)
		return translateProjectError(err)
	}

	// Refuse to delete an agent that is still the source of an agent kind —
	// deleting it would leave the kind unable to create new instances.
	isKindSource, err := s.agentKindService.HasKindsSourcedFrom(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to check agent kinds sourced from agent", "agentName", agentName, "error", err)
		return err
	}
	if isKindSource {
		return utils.ErrAgentIsKindSource
	}

	// Step 1: Fetch workload and check for secret references in env vars
	secretRefNames, err := s.ocClient.GetWorkloadSecretRefNames(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Warn("Failed to get workload secret references", "agentName", agentName, "error", err)
		// Continue with deletion even if we can't get secret refs
	}

	// Step 2-4: For each secret reference, get its details, delete from KV, then delete the CR
	for _, secretRefName := range secretRefNames {
		s.cleanupSecretReference(ctx, ouID, secretRefName)
	}

	// Resolve agent type before component deletion so LLM config cleanup does not need
	// to call GetComponent after the component is gone.
	isExternalAgent := false
	agentComp, compErr := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if compErr != nil {
		s.logger.Warn("Failed to determine agent type before deletion, assuming internal",
			"agentName", agentName, "error", compErr)
	} else {
		isExternalAgent = agentComp.Provisioning.Type == string(utils.ExternalAgent)
	}

	// Recheck immediately before the actual delete call (the commit point) rather
	// than relying solely on the check above. This is a deliberate, narrow
	// mitigation for the gap between that check and this call — not a full fix:
	// a kind could in theory still be published in the moment between this
	// recheck and DeleteComponent below. Closing that completely would mean
	// holding a DB transaction open across the OpenChoreo/secret-manager calls
	// above, which is a worse trade than the (now millisecond-scale, two-admins-
	// racing-the-same-agent) risk it would prevent. publishVersion independently
	// verifies the source agent still exists before ever creating a kind, which
	// is what actually closes the common (non-racing) case of this gap.
	isKindSource, err = s.agentKindService.HasKindsSourcedFrom(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to recheck agent kinds sourced from agent", "agentName", agentName, "error", err)
		return err
	}
	if isKindSource {
		return utils.ErrAgentIsKindSource
	}

	// Step 5: Delete agent component in OpenChoreo — this is the commit point.
	// LLM config cleanup happens after a confirmed DeleteComponent so a transient OC
	// failure leaves the system fully intact and the delete can be retried cleanly.
	s.logger.Debug("Deleting oc agent", "agentName", agentName, "ouID", ouID, "projectName", projectName)
	err = s.ocClient.DeleteComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		translatedErr := translateAgentError(err)
		if errors.Is(translatedErr, utils.ErrAgentNotFound) {
			s.logger.Warn("Agent not found during deletion, delete is idempotent", "agentName", agentName, "ouID", ouID, "projectName", projectName)
			if configErr := s.agentConfigRepo.DeleteAllByAgent(ouID, projectName, agentName); configErr != nil {
				s.logger.Warn("Failed to delete agent configs from database", "agentName", agentName, "error", configErr)
			}
			s.deleteAgentAPIArtifact(ctx, ouID, projectName, agentName)
			if s.agentThunderProvisioning != nil {
				go s.agentThunderProvisioning.DeleteAllBindings(context.WithoutCancel(ctx), ouID, projectName, agentName)
			}
			s.cleanupAgentMonitors(ctx, ouID, projectName, agentName)
			return nil
		}
		s.logger.Error("Failed to delete oc agent", "agentName", agentName, "error", err)
		return translatedErr
	}

	// Component confirmed deleted — clean up LLM proxy resources and DB records in the
	// background. context.WithoutCancel detaches the request deadline so the goroutine
	// is not cancelled when the HTTP handler returns, while still inheriting any values
	// (trace IDs, logger) from the request context.
	go s.deleteAgentLLMConfigurations(context.WithoutCancel(ctx), ouID, projectName, agentName, isExternalAgent)
	if s.agentThunderProvisioning != nil {
		go s.agentThunderProvisioning.DeleteAllBindings(context.WithoutCancel(ctx), ouID, projectName, agentName)
	}

	// Cleanup agent configs from database
	if configErr := s.agentConfigRepo.DeleteAllByAgent(ouID, projectName, agentName); configErr != nil {
		s.logger.Warn("Failed to delete agent configs from database", "agentName", agentName, "error", configErr)
		// Don't fail the deletion - configs will be orphaned but harmless
	}

	// Cleanup env-scoped API artifact record.
	s.deleteAgentAPIArtifact(ctx, ouID, projectName, agentName)

	// Cleanup monitors owned by this agent so they are not orphaned after deletion.
	s.cleanupAgentMonitors(ctx, ouID, projectName, agentName)

	s.logger.Debug("Agent deleted from OpenChoreo successfully", "ouID", ouID, "agentName", agentName)
	return nil
}

// cleanupAgentMonitors removes all monitors owned by an agent. Best-effort: orphaned
// monitors are logged but do not fail the delete, matching the other post-delete cleanups.
func (s *agentManagerService) cleanupAgentMonitors(ctx context.Context, ouID, projectName, agentName string) {
	if err := s.monitorManagerService.DeleteMonitorsByAgent(context.WithoutCancel(ctx), ouID, projectName, agentName); err != nil {
		s.logger.Warn("Failed to delete monitors during agent deletion", "agentName", agentName, "error", err)
	}
}

func (s *agentManagerService) deleteAgentAPIArtifact(ctx context.Context, ouID, projectName, agentName string) {
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if err != nil {
		s.logger.Warn("Failed to get deployment pipeline for agent API artifact cleanup", "agentName", agentName, "error", err)
		return
	}
	environmentName := findLowestEnvironment(pipeline.PromotionPaths)
	if environmentName == "" {
		return
	}
	environment, err := s.ocClient.GetEnvironment(ctx, ouID, environmentName)
	if err != nil {
		s.logger.Warn("Failed to get environment for agent API artifact cleanup", "agentName", agentName, "environment", environmentName, "error", err)
		return
	}
	artifact, err := s.artifactRepo.GetByHandle(agentEnvAPIArtifactHandle(projectName, agentName, environment.UUID), ouID)
	if err != nil {
		return
	}
	if delErr := s.artifactRepo.Delete(s.db, artifact.UUID.String()); delErr != nil {
		s.logger.Warn("Failed to delete agent API artifact record", "agentName", agentName, "environment", environmentName, "environmentUUID", environment.UUID, "error", delErr)
	}
}

// deleteAgentLLMConfigurations removes all LLM and MCP configurations for an agent during agent deletion.
// isExternalAgent must be resolved by the caller before the component is deleted so this function
// requires no OC calls. Calls DeleteForAgentDeletion which skips Component/Workload/ReleaseBinding
// patching and SecretReference CR deletion (handled by component teardown).
// After all configs are deleted, the shared AI application records (one per agent+env) are removed.
// Best-effort: individual failures are logged but do not abort the agent deletion.
func (s *agentManagerService) deleteAgentLLMConfigurations(ctx context.Context, ouID, projectName, agentName string, isExternalAgent bool) {
	listResp, err := s.agentConfigurationService.List(ctx, ouID, projectName, agentName, 1000, 0)
	if err != nil {
		s.logger.Warn("Failed to list agent configurations for cleanup", "agentName", agentName, "error", err)
		return
	}
	for _, cfg := range listResp.Configs {
		configUUID, parseErr := uuid.Parse(cfg.UUID)
		if parseErr != nil {
			s.logger.Warn("Failed to parse config UUID during agent deletion", "uuid", cfg.UUID, "type", cfg.Type, "error", parseErr)
			continue
		}
		if delErr := s.agentConfigurationService.DeleteForAgentDeletion(ctx, configUUID, ouID, projectName, agentName, isExternalAgent); delErr != nil {
			s.logger.Warn("Failed to delete configuration during agent deletion", "configUUID", cfg.UUID, "type", cfg.Type, "error", delErr)
		}
	}

	// Delete all AI application records for this agent (one per environment) now that all configs are gone.
	if delErr := s.aiApplicationService.DeleteAllByAgent(ctx, ouID, projectName, agentName); delErr != nil {
		s.logger.Warn("Failed to delete AI applications during agent deletion", "agentName", agentName, "error", delErr)
	}
}

// cleanupSecretReference deletes an OpenChoreo-managed secret by name (the
// secret name and its SecretReference name are the same); the API removes the
// stored values and the SecretReference together. Only removes secrets owned
// by this service (managed-by label).
func (s *agentManagerService) cleanupSecretReference(ctx context.Context, ouID, secretRefName string) {
	secret, err := s.ocClient.GetSecret(ctx, ouID, secretRefName)
	if err != nil {
		if errors.Is(err, utils.ErrNotFound) {
			s.logger.Debug("Secret not found, skipping cleanup", "secretRefName", secretRefName)
			return
		}
		s.logger.Warn("Failed to get secret, skipping cleanup", "secretRefName", secretRefName, "error", err)
		return
	}

	if secret.Labels[secretmanagersvc.LabelKeyManagedBy] != secretmanagersvc.DefaultManagedBy {
		s.logger.Warn("Secret not managed by this service, skipping cleanup", "secretRefName", secretRefName)
		return
	}

	if err := s.ocClient.DeleteSecret(ctx, ouID, secretRefName); err != nil && !errors.Is(err, utils.ErrNotFound) {
		s.logger.Warn("Failed to delete secret during cleanup", "secretRefName", secretRefName, "error", err)
	} else {
		s.logger.Debug("Deleted secret during cleanup", "secretRefName", secretRefName)
	}
}

// BuildAgent triggers a build for an agent.
func (s *agentManagerService) BuildAgent(ctx context.Context, ouID string, projectName string, agentName string, commitId string) (*models.BuildResponse, error) {
	s.logger.Info("Building agent", "agentName", agentName, "ouID", ouID, "projectName", projectName, "commitId", commitId)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return nil, translateOrgError(err)
	}

	// Validate project exists
	_, err = s.ocClient.GetProject(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "ouID", ouID, "error", err)
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
	s.logger.Debug("Triggering build in OpenChoreo", "agentName", agentName, "ouID", ouID, "projectName", projectName, "commitId", commitId)
	build, err := s.ocClient.TriggerBuild(ctx, ouID, projectName, agentName, commitId)
	if err != nil {
		s.logger.Error("Failed to trigger build in OpenChoreo", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, translateBuildError(err)
	}
	s.logger.Info("Build triggered successfully", "agentName", agentName, "ouID", ouID, "projectName", projectName, "buildName", build.Name)
	return build, nil
}

// DeployAgent deploys an agent.
func (s *agentManagerService) DeployAgent(ctx context.Context, ouID string, projectName string, agentName string, req *spec.DeployAgentRequest) (string, error) {
	s.logger.Info("Deploying agent", "agentName", agentName, "ouID", ouID, "projectName", projectName, "imageId", req.ImageId)
	org, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
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

	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to fetch deployment pipeline", "ouID", ouID, "projectName", projectName, "error", err)
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
	systemManagedEnvVars, systemManagedKeys, sysEnvErr := s.getSystemManagedEnvVars(ctx, ouID, projectName, lowestEnv, agentName)
	if sysEnvErr != nil {
		s.logger.Error("Failed to fetch system-managed env vars, aborting deploy to prevent data loss",
			"agentName", agentName, "ouID", ouID, "projectName", projectName, "error", sysEnvErr)
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

	// AgentID credentials (client ID/secret ref/token endpoint/scopes) are re-derived
	// fresh on every deploy — Deploy() replaces all workload env vars, so anything not
	// re-derived here would be silently wiped. Nil result = identity not provisioned
	// (yet) for this environment, which is a normal state, not an error. A real error
	// aborts the deploy for the same reason getSystemManagedEnvVars failure does: a
	// deploy that silently drops injected credentials is worse than a failed deploy.
	identityEnvVars, idErr := s.agentIdentityInjection.EnvVarsForEnvironment(ctx, ouID, projectName, agentName, lowestEnv)
	if idErr != nil {
		s.logger.Error("Failed to build agent identity env vars, aborting deploy to prevent credential loss",
			"agentName", agentName, "environment", lowestEnv, "error", idErr)
		return "", fmt.Errorf("failed to build agent identity env vars for agent %s: %w", agentName, idErr)
	}
	// Always filter user-echoed copies of the identity keys (the console reads
	// configurations back and may round-trip them), even when there is nothing
	// to re-inject — these keys are system-owned, never user-writable.
	systemManagedKeys = mergeAgentIdentityEnvVarKeys(systemManagedKeys)

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
	envVars, err := s.processEnvVars(ctx, ouID, projectName, lowestEnv, agentName, userEnv, req.Files)
	if err != nil {
		s.logger.Error("Failed to process environment variables", "agentName", agentName, "error", err)
		return "", fmt.Errorf("failed to process environment variables: %w", err)
	}

	s.logger.Debug("Processed user env vars", "agentName", agentName, "count", len(envVars))

	// Combine user-processed env vars with preserved system-managed env vars
	// and freshly-derived AgentID credentials.
	deployReq.Env = append(envVars, systemManagedEnvVars...)
	deployReq.Env = append(deployReq.Env, identityEnvVars...)

	// Ensure Env is always non-nil so Deploy() replaces the Workload env vars rather than
	// skipping the update. A nil slice is a no-op in Deploy(); an empty slice clears all vars.
	if deployReq.Env == nil {
		deployReq.Env = []client.EnvVar{}
	}

	s.logger.Debug("Final deploy env vars", "agentName", agentName, "totalCount", len(deployReq.Env))

	// Process file mounts
	fileVars, err := s.processFileVars(ctx, ouID, projectName, lowestEnv, agentName, req.Files)
	if err != nil {
		s.logger.Error("Failed to process file mounts", "agentName", agentName, "error", err)
		return "", fmt.Errorf("failed to process file mounts: %w", err)
	}
	deployReq.Files = fileVars
	s.logger.Debug("Processed file mounts", "agentName", agentName, "count", len(fileVars))

	targetEnv, err := s.ocClient.GetEnvironment(ctx, ouID, lowestEnv)
	if err != nil {
		s.logger.Warn("Failed to get environment details", "environment", lowestEnv, "error", err)
	}

	// Read the existing agent_configs row once so we can resolve omitted request
	// fields from DB and preserve pinned instrumentation_version during Upsert.
	var existingConfig *models.AgentConfig
	if targetEnv != nil {
		cfg, configErr := s.agentConfigRepo.Get(ouID, projectName, agentName, targetEnv.Name)
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

	// Resolve config values: request > DB > defaults
	tracingCfg := resolveTracingConfig(existingConfig, req.EnableAutoInstrumentation, true)
	apiCfg := resolveAPIConfig(existingConfig, req.EnableApiKeySecurity, req.CorsConfig, req.EnableOAuthSecurity, req.OauthConfig, true)
	resilienceTimeoutSeconds := resolveResilienceTimeoutSeconds(existingConfig, nil, true)
	if err := validateAuthExclusivity(apiCfg); err != nil {
		return "", err
	}
	if err := validateOAuthSecurityConfig(apiCfg); err != nil {
		return "", err
	}
	if targetEnv != nil {
		if err := s.validateOAuthIssuersInEnvironment(targetEnv.UUID, apiCfg); err != nil {
			return "", err
		}
	}
	enableAutoInstrumentation := tracingCfg.EnableAutoInstrumentation
	enableApiKeySecurity := apiCfg.EnableApiKeySecurity
	if apiCfg.CORSAllowCredentials {
		for _, origin := range apiCfg.CORSAllowOrigins {
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
	inProgress, err := s.ocClient.IsDeploymentInProgress(ctx, ouID, agentName, lowestEnv)
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

	// Build trait environment configs for the release binding.
	// Deploy sets the artifactId on the Component CR trait parameters (via AttachTraits),
	// so no per-env override is needed here. The api-management trait now derives
	// gatewayTarget/backendHost/backendPort itself from the apiGatewayName convention,
	// so no per-env gateway lookup is needed for trait configs.
	policies := buildPolicies(apiCfg)
	isPythonBuildpack := agent.Build != nil && agent.Build.Buildpack != nil && agent.Build.Buildpack.Language == string(utils.LanguagePython)
	isBallerinaBuildpack := agent.Build != nil && agent.Build.Buildpack != nil && agent.Build.Buildpack.Language == string(utils.LanguageBallerina)
	// Re-apply the agent's pinned instrumentation version as a per-env image
	// override so a redeploy doesn't drop a version set via deploy-settings/promote
	// back to the Component default. Deploy carries no version override of its own.
	deployLanguageVersion := ""
	if agent.Build != nil && agent.Build.Buildpack != nil {
		deployLanguageVersion = agent.Build.Buildpack.LanguageVersion
	}
	_, deployInstrumentationImage, err := s.resolveInstrumentationImageOverride(isPythonBuildpack, deployLanguageVersion, nil, existingInstrumentationVersion)
	if err != nil {
		return "", err
	}
	deployTraitEnvConfigs := buildTraitEnvConfigs(agentName, policies, "", resilienceTimeoutSeconds, isPythonBuildpack, isBallerinaBuildpack, enableAutoInstrumentation, deployInstrumentationImage)

	// Replace Component CR workflow parameters with env vars and file mounts from deploy request
	// This replaces all existing env vars to ensure the component CR matches the deploy request
	s.logger.Debug("Replacing component workflow parameters with environment variables", "agentName", agentName, "envVarCount", len(deployReq.Env))
	if err := s.ocClient.ReplaceComponentEnvVars(ctx, ouID, projectName, agentName, deployReq.Env); err != nil {
		s.logger.Warn("Failed to replace component workflow parameters with env vars", "agentName", agentName, "error", err)
		// Continue with deploy even if this fails - env vars will still be applied to the workload
	}

	if deployReq.Files != nil {
		s.logger.Debug("Replacing component workflow parameters with file mounts", "agentName", agentName, "fileMountCount", len(deployReq.Files))
		if err := s.ocClient.ReplaceComponentFileMounts(ctx, ouID, projectName, agentName, deployReq.Files); err != nil {
			s.logger.Warn("Failed to replace component workflow parameters with file mounts", "agentName", agentName, "error", err)
		}
	}

	if isAPIAgent {
		if targetEnv == nil {
			return "", fmt.Errorf("cannot deploy API agent without environment details")
		}
		apiArtifact, artifactErr := ensureAgentEnvAPIArtifact(s.db, s.artifactRepo, ouID, projectName, agentName, targetEnv.UUID)
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
		if resilienceTimeoutSeconds > 0 {
			traitOpts = append(traitOpts, client.WithResilienceTimeout(resilienceTimeoutSeconds))
		}
		traitOpts = append(traitOpts, client.WithPolicies(policies))

		componentDeployConfig.TraitsToAttach = append(componentDeployConfig.TraitsToAttach, client.TraitRequest{
			TraitKind: client.TraitKindTrait,
			TraitType: client.TraitAPIManagement,
			Opts:      traitOpts,
		})
		requiresComponentConfig = true

		// OTEL trait is already attached from create. Per-environment instrumentationEnabled is
		// controlled via deployTraitEnvConfigs (written to the ReleaseBinding) so patches are
		// gated by the 'where' clause without touching the Component CR trait attachment.

		s.logger.Info("Updated api-configuration trait", "agentName", agentName, "artifactID", artifactID, "enableApiKeySecurity", enableApiKeySecurity)
	}

	// Apply deploy-time Component CR changes in a single PUT. This replaces workflow env vars
	// and also applies any trait changes needed for this deploy.
	s.logger.Debug("Updating component deployment config", "agentName", agentName, "envVarCount", len(deployReq.Env),
		"traitsToAttach", len(componentDeployConfig.TraitsToAttach), "traitsToDetach", len(componentDeployConfig.TraitsToDetach))
	if err := s.ocClient.UpdateComponentDeploymentConfig(ctx, ouID, projectName, agentName, componentDeployConfig); err != nil {
		if requiresComponentConfig {
			return "", fmt.Errorf("failed to update component deployment config: %w", err)
		}
		s.logger.Warn("Failed to replace component workflow parameters with env vars", "agentName", agentName, "error", err)
		// Continue with deploy even if this fails - env vars will still be applied to the workload.
	}

	// Deploy agent component in OpenChoreo (after env vars and instrumentation are configured)
	s.logger.Debug("Deploying agent component in OpenChoreo", "agentName", agentName, "ouID", ouID, "projectName", projectName, "imageId", req.ImageId)
	if err := s.ocClient.Deploy(ctx, ouID, projectName, agentName, deployReq); err != nil {
		s.logger.Error("Failed to deploy agent component in OpenChoreo", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return "", err
	}

	// Update trait + component-type environment configs (e.g. runtimeClassName) on the release binding after deploy.
	// Component-type configs (runtimeClassName) only apply to sandboxed API agents; external agents have no pod,
	// so gate on isAPIAgent to match PromoteAgent and avoid writing an irrelevant key to their bindings.
	var deployCTConfigs map[string]interface{}
	if isAPIAgent {
		deployCTConfigs = buildComponentTypeEnvConfigs(targetEnv)
	}
	if len(deployTraitEnvConfigs) > 0 || len(deployCTConfigs) > 0 {
		if err := s.ocClient.UpdateReleaseBindingTraitConfigs(ctx, ouID, agentName, lowestEnv, deployTraitEnvConfigs, deployCTConfigs); err != nil {
			s.logger.Warn("Failed to update trait environment configs on release binding", "agentName", agentName, "environment", lowestEnv, "error", err)
		}
	}

	// Persist instrumentation config to database. Passing the pinned
	// instrumentation_version (captured above) preserves it across the
	// Upsert — the repo's DoUpdates map includes that column, so omitting
	// the value would NULL out a customer's pin on every redeploy.
	if targetEnv != nil {
		var deployResilienceTimeoutSeconds *int32
		if resilienceTimeoutSeconds > 0 {
			deployResilienceTimeoutSeconds = &resilienceTimeoutSeconds
		}
		agentConfig := &models.AgentConfig{
			OUID:                      ouID,
			ProjectName:               projectName,
			AgentName:                 agentName,
			EnvironmentName:           targetEnv.Name,
			EnableAutoInstrumentation: enableAutoInstrumentation,
			InstrumentationVersion:    existingInstrumentationVersion,
			EnableApiKeySecurity:      apiCfg.EnableApiKeySecurity,
			CORSEnabled:               apiCfg.CORSEnabled,
			CORSAllowOrigins:          apiCfg.CORSAllowOrigins,
			CORSAllowMethods:          apiCfg.CORSAllowMethods,
			CORSAllowHeaders:          apiCfg.CORSAllowHeaders,
			CORSAllowCredentials:      apiCfg.CORSAllowCredentials,
			EnableOAuthSecurity:       apiCfg.EnableOAuthSecurity,
			OAuthIssuers:              apiCfg.OAuthIssuers,
			OAuthAudiences:            apiCfg.OAuthAudiences,
			OAuthHeaderName:           apiCfg.OAuthHeaderName,
			OAuthAuthHeaderPrefix:     apiCfg.OAuthAuthHeaderPrefix,
			OAuthForwardToken:         apiCfg.OAuthForwardToken,
			ResilienceTimeoutSeconds:  deployResilienceTimeoutSeconds,
		}
		if configErr := s.agentConfigRepo.Upsert(agentConfig); configErr != nil {
			s.logger.Error("Failed to persist instrumentation config to database", "agentName", agentName, "environment", lowestEnv, "error", configErr)
		} else {
			s.logger.Debug("Persisted instrumentation config to database", "agentName", agentName, "environment", lowestEnv, "enableAutoInstrumentation", enableAutoInstrumentation, "instrumentationVersion", existingInstrumentationVersion)
		}
	}

	s.logger.Info("Agent deployed successfully to "+lowestEnv, "agentName", agentName, "ouID", org.Name, "projectName", projectName, "environment", lowestEnv)
	return lowestEnv, nil
}

// resolvedTracingConfig holds resolved instrumentation config values.
type resolvedTracingConfig struct {
	EnableAutoInstrumentation bool
}

// resolvedCORSConfig holds resolved CORS and API key security config values.
type resolvedCORSConfig struct {
	EnableApiKeySecurity bool
	CORSEnabled          bool
	CORSAllowOrigins     []string
	CORSAllowMethods     []string
	CORSAllowHeaders     []string
	CORSAllowCredentials bool
	// OAuth security (mutually exclusive with EnableApiKeySecurity).
	EnableOAuthSecurity   bool
	OAuthIssuers          []string
	OAuthAudiences        []string
	OAuthHeaderName       string
	OAuthAuthHeaderPrefix string
	OAuthForwardToken     bool
}

// resolveTracingConfig resolves instrumentation config: request > DB > default (true if withDefaults, else false).
func resolveTracingConfig(existingConfig *models.AgentConfig, enableAutoInstrumentation *bool, withDefaults bool) resolvedTracingConfig {
	resolved := resolvedTracingConfig{EnableAutoInstrumentation: withDefaults}
	if existingConfig != nil {
		resolved.EnableAutoInstrumentation = existingConfig.EnableAutoInstrumentation
	}
	if enableAutoInstrumentation != nil {
		resolved.EnableAutoInstrumentation = *enableAutoInstrumentation
	}
	return resolved
}

// resolveAPIConfig resolves CORS, API key, and OAuth security config: request > DB > env-var defaults (only if withDefaults).
func resolveAPIConfig(existingConfig *models.AgentConfig, enableApiKeySecurity *bool, corsConfig *spec.CORSConfig, enableOAuthSecurity *bool, oauthConfig *spec.OAuthConfig, withDefaults bool) resolvedCORSConfig {
	var resolved resolvedCORSConfig
	if withDefaults {
		defaultCORS := config.GetAgentWorkloadConfig().CORS
		resolved = resolvedCORSConfig{
			EnableApiKeySecurity: true,
			CORSEnabled:          true,
			CORSAllowOrigins:     strings.Split(defaultCORS.AllowOrigin, ","),
			CORSAllowMethods:     strings.Split(defaultCORS.AllowMethods, ","),
			CORSAllowHeaders:     strings.Split(defaultCORS.AllowHeaders, ","),
			CORSAllowCredentials: defaultCORS.AllowCredentials,
			EnableOAuthSecurity:  false,
			OAuthForwardToken:    models.DefaultOAuthForwardToken,
		}
	}

	if existingConfig != nil {
		resolved.EnableApiKeySecurity = existingConfig.EnableApiKeySecurity
		resolved.CORSEnabled = existingConfig.CORSEnabled
		if len(existingConfig.CORSAllowOrigins) > 0 {
			resolved.CORSAllowOrigins = existingConfig.CORSAllowOrigins
		}
		if len(existingConfig.CORSAllowMethods) > 0 {
			resolved.CORSAllowMethods = existingConfig.CORSAllowMethods
		}
		if len(existingConfig.CORSAllowHeaders) > 0 {
			resolved.CORSAllowHeaders = existingConfig.CORSAllowHeaders
		}
		resolved.CORSAllowCredentials = existingConfig.CORSAllowCredentials
		resolved.EnableOAuthSecurity = existingConfig.EnableOAuthSecurity
		resolved.OAuthIssuers = existingConfig.OAuthIssuers
		resolved.OAuthAudiences = existingConfig.OAuthAudiences
		resolved.OAuthForwardToken = existingConfig.OAuthForwardToken
		if existingConfig.OAuthHeaderName != "" {
			resolved.OAuthHeaderName = existingConfig.OAuthHeaderName
		}
		if existingConfig.OAuthAuthHeaderPrefix != "" {
			resolved.OAuthAuthHeaderPrefix = existingConfig.OAuthAuthHeaderPrefix
		}
	}

	if enableApiKeySecurity != nil {
		resolved.EnableApiKeySecurity = *enableApiKeySecurity
	}
	if corsConfig != nil {
		if corsConfig.Enabled != nil {
			resolved.CORSEnabled = *corsConfig.Enabled
		}
		if len(corsConfig.AllowOrigin) > 0 {
			resolved.CORSAllowOrigins = corsConfig.AllowOrigin
		}
		if len(corsConfig.AllowMethods) > 0 {
			resolved.CORSAllowMethods = corsConfig.AllowMethods
		}
		if len(corsConfig.AllowHeaders) > 0 {
			resolved.CORSAllowHeaders = corsConfig.AllowHeaders
		}
		if corsConfig.AllowCredentials != nil {
			resolved.CORSAllowCredentials = *corsConfig.AllowCredentials
		}
	}

	if enableOAuthSecurity != nil {
		resolved.EnableOAuthSecurity = *enableOAuthSecurity
	}
	if oauthConfig != nil {
		if oauthConfig.Issuers != nil {
			resolved.OAuthIssuers = oauthConfig.Issuers
		}
		if oauthConfig.Audiences != nil {
			resolved.OAuthAudiences = oauthConfig.Audiences
		}
		if oauthConfig.HeaderName != nil && *oauthConfig.HeaderName != "" {
			resolved.OAuthHeaderName = *oauthConfig.HeaderName
		}
		if oauthConfig.AuthHeaderPrefix != nil && *oauthConfig.AuthHeaderPrefix != "" {
			resolved.OAuthAuthHeaderPrefix = *oauthConfig.AuthHeaderPrefix
		}
		if oauthConfig.ForwardToken != nil {
			resolved.OAuthForwardToken = *oauthConfig.ForwardToken
		}
	}

	// Header name/prefix are NOT NULL columns — guarantee non-empty values so the
	// Upsert never writes an empty string over the gateway-compatible defaults.
	if resolved.OAuthHeaderName == "" {
		resolved.OAuthHeaderName = models.DefaultOAuthHeaderName
	}
	if resolved.OAuthAuthHeaderPrefix == "" {
		resolved.OAuthAuthHeaderPrefix = models.DefaultOAuthAuthHeaderPrefix
	}

	return resolved
}

// oauthConfigFromAgentConfig builds the response OAuth config from a persisted
// agent config row. Issuers are returned as persisted (no silent default); empty
// header fields fall back to the gateway-compatible defaults.
func oauthConfigFromAgentConfig(cfg *models.AgentConfig) *models.OAuthConfig {
	issuers := cfg.OAuthIssuers
	headerName := cfg.OAuthHeaderName
	if headerName == "" {
		headerName = models.DefaultOAuthHeaderName
	}
	authHeaderPrefix := cfg.OAuthAuthHeaderPrefix
	if authHeaderPrefix == "" {
		authHeaderPrefix = models.DefaultOAuthAuthHeaderPrefix
	}
	return &models.OAuthConfig{
		Issuers:          issuers,
		Audiences:        cfg.OAuthAudiences,
		HeaderName:       headerName,
		AuthHeaderPrefix: authHeaderPrefix,
		ForwardToken:     cfg.OAuthForwardToken,
	}
}

// validateAuthExclusivity rejects configs that enable both API key and OAuth
// security — only one auth policy may be attached to an agent endpoint.
func validateAuthExclusivity(cfg resolvedCORSConfig) error {
	if cfg.EnableApiKeySecurity && cfg.EnableOAuthSecurity {
		return fmt.Errorf("%w: API key and OAuth security are mutually exclusive — enable only one", utils.ErrInvalidInput)
	}
	return nil
}

// validateOAuthSecurityConfig rejects an OAuth-enabled config with no issuers.
// Issuers reference the environment's identity providers; there is no platform
// default, so an empty list is a configuration error. (Membership of each issuer
// in the environment's identity providers is validated separately where the
// environment context is available.)
func validateOAuthSecurityConfig(cfg resolvedCORSConfig) error {
	if cfg.EnableOAuthSecurity && len(cfg.OAuthIssuers) == 0 {
		return fmt.Errorf("%w: OAuth security requires at least one identity provider issuer", utils.ErrInvalidInput)
	}
	return nil
}

// validateOAuthIssuersInEnvironment rejects any OAuth issuer that is not one of
// the environment's configured identity providers. Issuers are gateway-owned;
// the agent can only reference providers that exist in its target environment.
func (s *agentManagerService) validateOAuthIssuersInEnvironment(envUUID string, cfg resolvedCORSConfig) error {
	if !cfg.EnableOAuthSecurity || envUUID == "" {
		return nil
	}
	providers, err := s.gatewayRepo.ListIdentityProvidersByEnvironment(envUUID)
	if err != nil {
		return err
	}
	valid := make(map[string]bool, len(providers))
	for _, p := range providers {
		valid[p.Name] = true
	}
	for _, issuer := range cfg.OAuthIssuers {
		if !valid[issuer] {
			return fmt.Errorf("%w: identity provider %q is not configured for this environment", utils.ErrInvalidInput, issuer)
		}
	}
	return nil
}

// buildPolicies builds the api-configuration trait policies from resolved config.
// Returns a non-nil slice so the "no authentication" mode (no CORS, no auth)
// marshals to an empty JSON array — the api-configuration trait rejects a null
// policies field ("policies must be of type array").
func buildPolicies(cfg resolvedCORSConfig) []map[string]interface{} {
	policies := make([]map[string]interface{}, 0)
	// CORS must be first so preflight OPTIONS requests are handled before
	// any auth policy runs. Exactly one auth policy (api-key-auth or jwt-auth)
	// is appended after — mutual exclusivity is enforced upstream.
	if cfg.CORSEnabled {
		policies = append(policies, client.CORSPolicy(cfg.CORSAllowOrigins, cfg.CORSAllowMethods, cfg.CORSAllowHeaders, cfg.CORSAllowCredentials))
	}
	if cfg.EnableApiKeySecurity {
		policies = append(policies, client.APIKeyAuthPolicy())
	}
	if cfg.EnableOAuthSecurity {
		// Issuers are passed through as persisted — empty issuers are rejected by
		// validateOAuthSecurityConfig before reaching here, so no default is invented.
		policies = append(policies, client.OAuthPolicy(client.OAuthPolicyParams{
			Issuers:          cfg.OAuthIssuers,
			Audiences:        cfg.OAuthAudiences,
			HeaderName:       cfg.OAuthHeaderName,
			AuthHeaderPrefix: cfg.OAuthAuthHeaderPrefix,
			ForwardToken:     cfg.OAuthForwardToken,
		}))
	}
	return policies
}

// runtimeClassForIsolationTier maps an environment's isolation tier to the Kubernetes
// runtimeClassName the agent-api ComponentType should request. Empty means "omit" (default runc).
func runtimeClassForIsolationTier(tier string) string {
	switch tier {
	case utils.IsolationTierGvisor:
		return utils.RuntimeClassGvisor
	case utils.IsolationTierKata:
		// kata-deploy registers the runtime under the "kata-qemu" RuntimeClass/handler
		// (the QEMU hypervisor variant). The tier name stays "kata" at the API; only the
		// rendered runtimeClassName is "kata-qemu" so it matches what the node installs.
		return utils.RuntimeClassKataQemu
	default:
		return ""
	}
}

// buildComponentTypeEnvConfigs builds the ComponentTypeEnvironmentConfigs overrides for a deployment,
// derived from the target environment's isolation tier. Returns an empty map for the default (runc)
// tier so the rendered SandboxTemplate is unchanged for existing environments.
func buildComponentTypeEnvConfigs(env *models.EnvironmentResponse) map[string]interface{} {
	configs := map[string]interface{}{}
	if env == nil {
		return configs
	}
	if rc := runtimeClassForIsolationTier(env.IsolationTier); rc != "" {
		configs["runtimeClassName"] = rc
	}
	return configs
}

// buildTraitEnvConfigs builds the traitEnvironmentConfigs map for a release binding.
// Keys must be trait instance names (format: "{componentName}-{traitName}") because
// OpenChoreo's rendering engine looks up traitEnvironmentConfigs by instance name.
// artifactID, when non-empty, is injected as a per-environment override for the api-configuration
// trait so that each environment gets a unique artifact UUID in its RestApi resource.
// backendHost / backendPort / gatewayTarget are no longer written here — the
// api-management trait derives them from the apiGatewayName convention
// ("api-platform-<org>-<env>") + platform-wide gateway runtime constants.
// For Python buildpack agents, instrumentationEnabled is set per-environment on the OTEL trait;
// for Ballerina buildpack agents it is set on the ballerina-config-file trait. In both cases the
// 'where' clause on patches enables/disables instrumentation independently per environment.
// instrumentationImage, when non-empty, pins the OTEL init-container image for this environment
// (overriding the Component's create-time default) so the AMP instrumentation version can be
// changed per-environment on deploy/promote without re-attaching the Component trait.
func buildTraitEnvConfigs(agentName string, policies []map[string]interface{}, artifactID string, resilienceTimeoutSeconds int32, isPythonBuildpack, isBallerinaBuildpack bool, autoInstrumentation bool, instrumentationImage string) map[string]interface{} {
	instanceName := func(traitType client.TraitType) string {
		return agentName + "-" + string(traitType)
	}
	apiTraitCfg := map[string]interface{}{
		"policies": policies,
	}
	if artifactID != "" {
		apiTraitCfg["artifactId"] = artifactID
	}
	if resilienceTimeoutSeconds > 0 {
		apiTraitCfg["resilienceTimeout"] = client.FormatResilienceTimeout(resilienceTimeoutSeconds)
	}
	traitEnvConfigs := map[string]interface{}{
		instanceName(client.TraitAPIManagement): apiTraitCfg,
	}
	if isPythonBuildpack {
		otelCfg := map[string]interface{}{
			"instrumentationEnabled": autoInstrumentation,
		}
		if instrumentationImage != "" {
			otelCfg["instrumentationImage"] = instrumentationImage
		}
		traitEnvConfigs[instanceName(client.TraitOTELInstrumentation)] = otelCfg
	}
	if isBallerinaBuildpack {
		traitEnvConfigs[instanceName(client.TraitBallerinaOTELInstrumentation)] = map[string]interface{}{
			"instrumentationEnabled": autoInstrumentation,
		}
		// Gate env injection together with Ballerina instrumentation so the OTEL
		// endpoint and API key env vars are only injected when instrumentation is
		// enabled for this environment.
		traitEnvConfigs[instanceName(client.TraitEnvInjection)] = map[string]interface{}{
			"envInjectionEnabled": autoInstrumentation,
		}
	}
	return traitEnvConfigs
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

// allPipelineEnvironmentNames returns every environment name that appears
// anywhere in a project's deployment pipeline (source or target). Used to
// filter an agent's org-wide AgentID bindings (Section 2.1 of the AgentID
// architecture doc: AgentIDs are provisioned per org-level environment, but a
// project only ever shows the bindings for environments in its own pipeline).
func allPipelineEnvironmentNames(promotionPaths []models.PromotionPath) map[string]bool {
	names := make(map[string]bool)
	for _, path := range promotionPaths {
		names[path.SourceEnvironmentRef] = true
		for _, target := range path.TargetEnvironmentRefs {
			names[target.Name] = true
		}
	}
	return names
}

// GetAgentIdentity returns the agent's AgentID binding for every environment in
// this project's deployment pipeline. AgentIDs are provisioned across every
// org-level environment (Section 2.1 of the AgentID architecture doc), but a
// project only ever shows the bindings for environments in its own pipeline —
// this filters the org-wide result down to that visibility rule. A safe GET:
// it never returns or destroys a secret. Use RegenerateAgentIdentitySecret to
// obtain a secret for an External agent.
func (s *agentManagerService) GetAgentIdentity(ctx context.Context, ouID string, projectName string, agentName string) ([]models.AgentIdentityEnvironmentView, error) {
	// No provisioning implementation: no identities to report.
	if s.agentThunderProvisioning == nil {
		return []models.AgentIdentityEnvironmentView{}, nil
	}
	views, err := s.agentThunderProvisioning.GetIdentityViews(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, err
	}

	visible, err := s.visiblePipelineEnvironments(ctx, ouID, projectName)
	if err != nil {
		return nil, err
	}

	filtered := make([]models.AgentIdentityEnvironmentView, 0, len(views))
	for _, v := range views {
		if visible[v.EnvironmentName] {
			filtered = append(filtered, v)
		}
	}
	return filtered, nil
}

// visiblePipelineEnvironments returns the set of environment names visible to
// projectName's deployment pipeline — AgentIDs are provisioned across every
// org-level environment (Section 2.1 of the AgentID architecture doc), but a
// project only ever sees the ones in its own pipeline. Shared by
// GetAgentIdentity (filtering a list) and requireVisibleEnvironment
// (checking a single name).
func (s *agentManagerService) visiblePipelineEnvironments(ctx context.Context, ouID, projectName string) (map[string]bool, error) {
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to get deployment pipeline for agent identity visibility", "projectName", projectName, "error", err)
		return nil, translatePipelineError(err)
	}
	return allPipelineEnvironmentNames(pipeline.PromotionPaths), nil
}

// requireVisibleEnvironment validates that environmentName is part of
// projectName's deployment pipeline — the same visibility rule GetAgentIdentity
// applies when filtering its org-wide binding list down to project scope.
func (s *agentManagerService) requireVisibleEnvironment(ctx context.Context, ouID, projectName, environmentName string) error {
	visible, err := s.visiblePipelineEnvironments(ctx, ouID, projectName)
	if err != nil {
		return err
	}
	if !visible[environmentName] {
		return fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, environmentName)
	}
	return nil
}

// GetAgentRoles returns the Thunder roles assigned to the agent's AgentID in
// one environment, if that environment is part of this project's deployment
// pipeline.
func (s *agentManagerService) GetAgentRoles(ctx context.Context, ouID, projectName, agentName, environmentName string) ([]thundersvc.ThunderRole, error) {
	if s.agentThunderProvisioning == nil {
		return nil, fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, environmentName)
	}
	if err := s.requireVisibleEnvironment(ctx, ouID, projectName, environmentName); err != nil {
		return nil, err
	}
	return s.agentThunderProvisioning.GetAgentRoles(ctx, ouID, projectName, agentName, environmentName)
}

// GetAgentGroups returns the Thunder groups the agent's AgentID belongs to in
// one environment, if that environment is part of this project's deployment
// pipeline.
func (s *agentManagerService) GetAgentGroups(ctx context.Context, ouID, projectName, agentName, environmentName string) ([]thundersvc.ThunderGroup, error) {
	if s.agentThunderProvisioning == nil {
		return nil, fmt.Errorf("%w: %s in %s", utils.ErrAgentIdentityNotProvisioned, agentName, environmentName)
	}
	if err := s.requireVisibleEnvironment(ctx, ouID, projectName, environmentName); err != nil {
		return nil, err
	}
	return s.agentThunderProvisioning.GetAgentGroups(ctx, ouID, projectName, agentName, environmentName)
}

// RegenerateAgentIdentitySecret rotates the AgentID secret for one environment.
// The new secret is returned in the response for BOTH Internal and External
// agents — the caller already holds agent:update and just explicitly asked
// for a new credential, so withholding the value it produced would only
// force a second call to fetch it separately. ProvisioningType is included
// so the caller can tell which kind of agent this is without a separate lookup.
func (s *agentManagerService) RegenerateAgentIdentitySecret(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (*models.AgentRegenerateSecretResponse, error) {
	if s.agentThunderProvisioning == nil {
		return nil, utils.ErrAgentIdentityNotProvisioned
	}
	ownership, clientID, newSecret, err := s.agentThunderProvisioning.RegenerateSecret(ctx, ouID, projectName, agentName, environmentName)
	if err != nil {
		return nil, err
	}

	// Gateway Binding: an internal agent's running pod still holds the OLD
	// secret via its SecretReference-backed env var. Force an immediate Secret
	// re-sync + pod rollout so it picks up the rotated value. Best-effort —
	// the rotation itself already succeeded, so the response must report it
	// regardless. A failed refresh does NOT self-heal on its own: the pod's
	// env var only updates on an actual rollout, so it keeps serving the now-
	// invalidated old secret until something else redeploys it — surfaced to
	// the caller via WorkloadRefreshWarning instead of only being logged, so
	// this isn't a silent failure from the API consumer's point of view.
	var workloadRefreshWarning string
	if ownership == models.AgentProvisioningTypeInternal {
		if refreshErr := s.agentIdentityInjection.RefreshAfterRotation(ctx, ouID, projectName, agentName, environmentName); refreshErr != nil {
			s.logger.Warn("Failed to refresh agent identity credentials in workload after rotation",
				"agentName", agentName, "environment", environmentName, "error", refreshErr)
			workloadRefreshWarning = "The secret was rotated, but the running workload could not be refreshed automatically; " +
				"it will keep using the previous secret until its next deploy, promote, or rotation."
		}
	}

	return &models.AgentRegenerateSecretResponse{
		EnvironmentName:        environmentName,
		ProvisioningType:       ownership,
		ClientID:               clientID,
		ClientSecret:           newSecret,
		Status:                 models.AgentRegenerateSecretStatus,
		WorkloadRefreshWarning: workloadRefreshWarning,
	}, nil
}

// RevokeAgentIdentitySecret invalidates the AgentID secret for one environment.
// It never returns a usable secret — an explicit regenerate is required
// afterward to restore access.
func (s *agentManagerService) RevokeAgentIdentitySecret(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (models.AgentRevokeSecretResponse, error) {
	if s.agentThunderProvisioning == nil {
		return models.AgentRevokeSecretResponse{}, utils.ErrAgentIdentityNotProvisioned
	}
	clientID, err := s.agentThunderProvisioning.RevokeSecret(ctx, ouID, projectName, agentName, environmentName)
	if err != nil {
		return models.AgentRevokeSecretResponse{}, err
	}

	// Gateway Binding: strip the now-dead credential from the running pod so it
	// doesn't keep presenting a secret that can no longer mint tokens. Only the
	// pipeline's lowest environment carries these vars at the shared Workload
	// level (written there by the deploy flow) — removing workload-level vars
	// while revoking any OTHER environment's credential would break the lowest
	// environment's pod. Best-effort: the revoke itself already succeeded.
	//
	// includeWorkloadLevel is true only for the pipeline's lowest environment
	// (the deploy flow's shared Workload CR only needs clearing there).
	// workloadRefreshWarning is set on the response whenever that cleanup
	// couldn't be confirmed, so a caller knows the running pod may still
	// reference the revoked credential.
	var workloadRefreshWarning string
	includeWorkloadLevel := false
	if pipeline, pipeErr := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName); pipeErr != nil {
		s.logger.Warn("Failed to resolve deployment pipeline during identity revoke cleanup; skipping workload-level env removal",
			"agentName", agentName, "environment", environmentName, "error", pipeErr)
		workloadRefreshWarning = "Could not confirm whether this is the deployment pipeline's lowest environment, " +
			"so its shared workload-level credentials were left untouched as a precaution."
	} else {
		includeWorkloadLevel = findLowestEnvironment(pipeline.PromotionPaths) == environmentName
	}
	if removeErr := s.agentIdentityInjection.RemoveForEnvironment(ctx, ouID, projectName, agentName, environmentName, includeWorkloadLevel); removeErr != nil {
		s.logger.Warn("Failed to remove agent identity credentials from workload after revoke",
			"agentName", agentName, "environment", environmentName, "error", removeErr)
		workloadRefreshWarning = "The secret was revoked, but the running workload could not be refreshed automatically; " +
			"it may keep referencing the revoked credential until this is confirmed or the workload is redeployed."
	}

	return models.AgentRevokeSecretResponse{
		EnvironmentName:        environmentName,
		ClientID:               clientID,
		Status:                 models.AgentRevokeSecretStatus,
		WorkloadRefreshWarning: workloadRefreshWarning,
	}, nil
}

// ProvisionAgentIdentity provisions an AgentID for one environment that doesn't
// have one yet — for an External agent that existed before this environment
// did (or before it entered the project's pipeline). Internal agents get their
// AgentIDs automatically during PromoteAgent instead; this endpoint rejects
// them with a clear pointer to that flow rather than silently doing nothing
// useful (an Internal agent's identity is meaningless without also deploying
// its workload there, which only promotion does).
//
// alreadyExisted is true when a binding for this environment was already
// present (any status) — the caller uses this to choose between 200 (nothing
// new happened, here's the current state) and 202 (provisioning was just
// kicked off in the background).
func (s *agentManagerService) ProvisionAgentIdentity(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (models.AgentIdentityEnvironmentView, bool, error) {
	if s.agentThunderProvisioning == nil {
		return models.AgentIdentityEnvironmentView{}, false, utils.ErrAgentIdentityNotProvisioned
	}
	agent, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent for identity provisioning", "agentName", agentName, "error", err)
		return models.AgentIdentityEnvironmentView{}, false, translateAgentError(err)
	}
	if agent.Provisioning.Type != string(utils.ExternalAgent) {
		return models.AgentIdentityEnvironmentView{}, false, fmt.Errorf(
			"%w: agent %q is an internal agent — internal agents receive an AgentID automatically when promoted to a new environment, not via this endpoint",
			utils.ErrInvalidInput, agentName,
		)
	}

	if _, err := s.ocClient.GetEnvironment(ctx, ouID, environmentName); err != nil {
		s.logger.Error("Failed to fetch environment for identity provisioning", "environmentName", environmentName, "error", err)
		return models.AgentIdentityEnvironmentView{}, false, translateEnvironmentError(err)
	}

	var requestedBy string
	if callerClaims := jwtassertion.GetTokenClaims(ctx); callerClaims != nil {
		requestedBy = callerClaims.Sub
	}

	alreadyExisted, err := s.agentThunderProvisioning.ProvisionForEnvironmentIfMissing(
		ctx, ouID, projectName, agentName, environmentName, models.AgentProvisioningTypeExternal, requestedBy,
	)
	if err != nil {
		return models.AgentIdentityEnvironmentView{}, false, err
	}

	views, err := s.agentThunderProvisioning.GetIdentityViews(ctx, ouID, projectName, agentName)
	if err != nil {
		return models.AgentIdentityEnvironmentView{}, alreadyExisted, err
	}
	for _, v := range views {
		if v.EnvironmentName == environmentName {
			return v, alreadyExisted, nil
		}
	}
	// Upsert inside ProvisionForEnvironmentIfMissing is synchronous, so the row
	// (at minimum PENDING) must already be visible to this read.
	return models.AgentIdentityEnvironmentView{}, alreadyExisted, fmt.Errorf("agent thunder binding for %s/%s vanished immediately after being provisioned", agentName, environmentName)
}

// PromoteAgent promotes an agent from one environment to another.
func (s *agentManagerService) PromoteAgent(ctx context.Context, ouID string, projectName string, agentName string, req *spec.PromoteAgentRequest) error {
	s.logger.Info("Promoting agent", "agentName", agentName, "ouID", ouID, "projectName", projectName,
		"sourceEnvironment", req.SourceEnvironment, "targetEnvironment", req.TargetEnvironment)

	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return translateOrgError(err)
	}

	// Validate agent exists and is an internal agent
	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent from OpenChoreo", "agentName", agentName, "error", err)
		return translateAgentError(err)
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return fmt.Errorf("promote operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}

	// Validate promotion path exists: get deployment pipeline and verify source → target is valid
	pipeline, err := s.ocClient.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to fetch deployment pipeline", "ouID", ouID, "projectName", projectName, "error", err)
		return translatePipelineError(err)
	}

	if !isValidPromotionPath(pipeline.PromotionPaths, req.SourceEnvironment, req.TargetEnvironment) {
		return fmt.Errorf("invalid promotion path: %s → %s is not allowed by the deployment pipeline", req.SourceEnvironment, req.TargetEnvironment)
	}

	// Check if a deployment is already in progress in target environment
	inProgress, err := s.ocClient.IsDeploymentInProgress(ctx, ouID, agentName, req.TargetEnvironment)
	if err != nil {
		s.logger.Warn("Failed to check deployment status in target environment", "agentName", agentName, "environment", req.TargetEnvironment, "error", err)
	} else if inProgress {
		return fmt.Errorf("%w for agent %s in environment %s", utils.ErrDeploymentInProgress, agentName, req.TargetEnvironment)
	}

	// System-managed env vars (LLM provider URL/key, MCP, etc.) live per-environment in
	// agent_env_config_variables_mapping. Promotion must enforce this invariant: if the
	// SOURCE environment has any system-managed vars, the TARGET environment must also
	// have its own — otherwise the agent would silently lose those managed bindings. This check runs in
	// both useConfigFromSourceEnv branches so the rule is uniform.
	srcSystemKeys, err := s.agentConfigurationService.ListSystemManagedEnvVarKeys(ctx, agentName, ouID, projectName, req.SourceEnvironment)
	if err != nil {
		s.logger.Error("Failed to fetch source env system-managed env var keys for promotion", "agentName", agentName, "error", err)
		return fmt.Errorf("failed to fetch source env system-managed keys: %w", err)
	}
	tgtSystemKeys, err := s.agentConfigurationService.ListSystemManagedEnvVarKeys(ctx, agentName, ouID, projectName, req.TargetEnvironment)
	if err != nil {
		s.logger.Error("Failed to fetch target env system-managed env var keys for promotion", "agentName", agentName, "error", err)
		return fmt.Errorf("failed to fetch target env system-managed keys: %w", err)
	}
	if len(srcSystemKeys) > 0 && len(tgtSystemKeys) == 0 {
		return fmt.Errorf("%w: agent %q has LLM/system configuration in source environment %q but none in target environment %q — configure system variables in the target environment before promoting",
			utils.ErrInvalidInput, agentName, req.SourceEnvironment, req.TargetEnvironment)
	}

	// Build the target environment's system-managed env vars from the DB. We always
	// inject these so the target binding has its OWN system credentials, regardless of
	// whether we're cloning from source or taking user overrides.
	var tgtSystemEnvVars []client.EnvVar
	if len(tgtSystemKeys) > 0 {
		tgtSystemEnvVars, err = s.agentConfigurationService.BuildSystemManagedEnvVarsFromConfig(ctx, agentName, ouID, projectName, req.TargetEnvironment)
		if err != nil {
			s.logger.Error("Failed to build target env system-managed vars from config", "agentName", agentName, "error", err)
			return fmt.Errorf("failed to build target env system-managed vars: %w", err)
		}
	}

	// AgentID credentials are strictly per-environment: the source environment's
	// client ID/secret must NEVER travel to the target. Marking the identity keys
	// as system-managed in BOTH sets makes the useSource clone strip them from the
	// copied overrides and the user-driven branch drop echoed copies from req.Env.
	// This runs AFTER the src-vs-tgt validation above, which must only compare
	// DB-backed LLM/system configuration. The target environment's own identity
	// vars are appended below.
	srcSystemKeys = mergeAgentIdentityEnvVarKeys(srcSystemKeys)
	tgtSystemKeys = mergeAgentIdentityEnvVarKeys(tgtSystemKeys)

	// Best-effort kick-off: the target environment may have been added to the
	// org AFTER this agent was created, so its AgentID binding could be
	// missing entirely (agent-creation only provisions into environments that
	// existed at that time). Must happen BEFORE building envOverrides below,
	// since the hard block right after this call needs the binding's state
	// already resolved. Skipped when AgentThunder provisioning is disabled
	// for this deployment — nothing can create a new binding in that case,
	// but the readiness check and hard block below still apply.
	if s.agentThunderProvisioning != nil {
		var requestedBy string
		if callerClaims := jwtassertion.GetTokenClaims(ctx); callerClaims != nil {
			requestedBy = callerClaims.Sub
		}
		if _, err := s.agentThunderProvisioning.ProvisionForEnvironmentIfMissing(
			ctx, ouID, projectName, agentName, req.TargetEnvironment, models.AgentProvisioningTypeInternal, requestedBy,
		); err != nil {
			s.logger.Warn("Failed to ensure AgentID for promotion target environment before promoting", "agentName", agentName, "environment", req.TargetEnvironment, "error", err)
		}
	}

	// Read unconditionally, mirroring the deploy path (DeployAgent):
	// s.agentThunderProvisioning only gates minting a NEW credential (the
	// kick-off above), not reading one that already exists — a real
	// credential can still be sitting in the shared Workload CR even after
	// provisioning is later disabled for this deployment.
	tgtIdentityEnvVars, idErr := s.agentIdentityInjection.EnvVarsForEnvironment(ctx, ouID, projectName, agentName, req.TargetEnvironment)
	if idErr != nil {
		s.logger.Error("Failed to build target env agent identity env vars, aborting promotion to prevent credential loss",
			"agentName", agentName, "environment", req.TargetEnvironment, "error", idErr)
		return fmt.Errorf("failed to build target env agent identity env vars: %w", idErr)
	}
	if len(tgtIdentityEnvVars) == 0 && s.agentThunderProvisioning != nil {
		// The kick-off above only just wrote a PENDING row and kicked off its
		// Thunder call on a detached goroutine — checking readiness this soon
		// virtually always sees it still in flight, so promoting to a brand
		// new environment would otherwise ALWAYS fail once before a manual
		// retry succeeds. A Thunder+OpenBao round trip normally finishes in
		// well under a second, so give it a short, bounded chance to land
		// before falling through to the hard block below. Only meaningful when
		// a kick-off actually happened above — with provisioning disabled
		// there is nothing in flight to wait out.
		tgtIdentityEnvVars = s.pollForTargetIdentityReady(ctx, ouID, projectName, agentName, req.TargetEnvironment)
	}
	if len(tgtIdentityEnvVars) == 0 {
		// HARD BLOCK, not best-effort: the lowest environment's real identity
		// is written into the shared OpenChoreo Workload CR, inherited by
		// every environment unless a ReleaseBinding overrides it. If the
		// target's own identity isn't ready, nothing overrides those keys and
		// the promoted pod would silently start up with the lowest
		// environment's real credentials — a cross-environment leak. Only
		// enforced when the lowest environment actually holds a real
		// credential; a pipeline that has never used AgentID has nothing to
		// leak and must not be blocked from promoting.
		lowestEnv := findLowestEnvironment(pipeline.PromotionPaths)
		lowestEnvVars, lowestErr := s.agentIdentityInjection.EnvVarsForEnvironment(ctx, ouID, projectName, agentName, lowestEnv)
		if lowestErr != nil {
			s.logger.Error("Failed to check lowest env agent identity state for promotion safety, aborting promotion to prevent credential loss",
				"agentName", agentName, "lowestEnvironment", lowestEnv, "error", lowestErr)
			return fmt.Errorf("failed to check lowest environment agent identity state: %w", lowestErr)
		}
		if len(lowestEnvVars) > 0 {
			return s.buildPromotionIdentityBlockedError(ctx, ouID, projectName, agentName, req.TargetEnvironment)
		}
	}

	var envOverrides []client.EnvVar
	var fileOverrides []client.FileVar

	useSource := req.UseConfigFromSourceEnv != nil && *req.UseConfigFromSourceEnv
	if useSource {
		// Clone the source env's workload overrides, but scrub source's system-managed
		// keys — they're env-specific and must be replaced with the target's own.
		srcEnvVars, srcFileVars, err := s.ocClient.GetSourceEnvWorkloadOverrides(ctx, ouID, agentName, req.SourceEnvironment)
		if err != nil {
			s.logger.Error("Failed to fetch source env workload overrides for promotion", "agentName", agentName, "error", err)
			return fmt.Errorf("failed to fetch source env workload overrides: %w", err)
		}
		for _, ev := range srcEnvVars {
			if !srcSystemKeys[ev.Key] {
				envOverrides = append(envOverrides, ev)
			}
		}
		fileOverrides = srcFileVars

		// The cloned overrides reference the SOURCE environment's secret by name.
		// Give the target environment its own copy and re-point the references,
		// so a later secret edit in one environment cannot break the other.
		envOverrides, fileOverrides, err = s.cloneEnvSecretForPromotion(ctx, ouID, projectName, agentName, req.SourceEnvironment, req.TargetEnvironment, envOverrides, fileOverrides)
		if err != nil {
			s.logger.Error("Failed to clone env secret for promotion", "agentName", agentName, "error", err)
			return fmt.Errorf("failed to clone environment secret for promotion: %w", err)
		}
	} else {
		// User-driven overrides: only what the request carries (plus target system vars
		// appended below). Source env's user-managed env/files are NOT inherited.
		userEnv := req.Env
		if len(tgtSystemKeys) > 0 && len(req.Env) > 0 {
			userEnv = make([]spec.EnvironmentVariable, 0, len(req.Env))
			for _, env := range req.Env {
				if tgtSystemKeys[env.Key] {
					s.logger.Debug("Filtering system-managed env var from promote request", "key", env.Key)
					continue
				}
				userEnv = append(userEnv, env)
			}
		}
		if len(userEnv) > 0 {
			processed, err := s.processEnvVars(ctx, ouID, projectName, req.TargetEnvironment, agentName, userEnv, req.Files)
			if err != nil {
				s.logger.Error("Failed to process environment variables for promotion", "agentName", agentName, "error", err)
				return fmt.Errorf("failed to process environment variables: %w", err)
			}
			envOverrides = append(envOverrides, processed...)
		}
		if len(req.Files) > 0 {
			processed, err := s.processFileVars(ctx, ouID, projectName, req.TargetEnvironment, agentName, req.Files)
			if err != nil {
				s.logger.Error("Failed to process file mounts for promotion", "agentName", agentName, "error", err)
				return fmt.Errorf("failed to process file mounts: %w", err)
			}
			fileOverrides = processed
		}
	}

	// Always inject the target env's system-managed vars. In the useSource branch this
	// replaces the source vars we just stripped; in the user-driven branch it ensures
	// the agent has its target-env wiring even if the user supplied no overrides.
	envOverrides = append(envOverrides, tgtSystemEnvVars...)
	// Same for the target env's own AgentID credentials (nil when the target's
	// identity hasn't finished provisioning — the post-provisioning hook injects
	// them once it does).
	envOverrides = append(envOverrides, tgtIdentityEnvVars...)

	// Build trait environment configs for per-environment trait overrides
	var traitEnvConfigs map[string]interface{}
	var promoteCTConfigs map[string]interface{}
	isAPIAgent := agent.Type.Type == string(utils.AgentTypeAPI)
	if isAPIAgent {
		// Resolve config values: request > source env DB > defaults
		var existingConfig *models.AgentConfig
		cfg, configErr := s.agentConfigRepo.Get(ouID, projectName, agentName, req.SourceEnvironment)
		if configErr == nil {
			existingConfig = cfg
		}

		// Deploy-settings precedence (CORS, API key security, auto instrumentation):
		//   request fields → source env's saved AgentConfig → off.
		// When useConfigFromSourceEnv=true, the validator guarantees the request fields
		// are nil, so the resolved settings fall through to the source env's saved
		// AgentConfig. When useConfigFromSourceEnv=false the request takes precedence
		// where set; any unset field falls back to the source env's values.
		tracingCfg := resolveTracingConfig(existingConfig, req.EnableAutoInstrumentation, false)
		apiCfg := resolveAPIConfig(existingConfig, req.EnableApiKeySecurity, req.CorsConfig, req.EnableOAuthSecurity, req.OauthConfig, false)
		resilienceTimeoutSeconds := resolveResilienceTimeoutSeconds(existingConfig, nil, false)
		if err := validateAuthExclusivity(apiCfg); err != nil {
			return err
		}
		if err := validateOAuthSecurityConfig(apiCfg); err != nil {
			return err
		}
		policies := buildPolicies(apiCfg)

		// Each environment must have its own unique artifact UUID so the gateway controller
		// does not confuse two environments' RestApi resources (same UUID = one overwrites the other).
		targetEnv, targetEnvErr := s.ocClient.GetEnvironment(ctx, ouID, req.TargetEnvironment)
		if targetEnvErr != nil {
			return fmt.Errorf("failed to fetch target environment details: %w", targetEnvErr)
		}
		if targetEnv != nil {
			if err := s.validateOAuthIssuersInEnvironment(targetEnv.UUID, apiCfg); err != nil {
				return err
			}
		}

		artifact, artifactErr := ensureAgentEnvAPIArtifact(s.db, s.artifactRepo, ouID, projectName, agentName, targetEnv.UUID)
		if artifactErr != nil {
			return fmt.Errorf("failed to ensure target env API artifact: %w", artifactErr)
		}
		targetArtifactID := artifact.UUID.String()
		promotePythonBuildpack := agent.Build != nil && agent.Build.Buildpack != nil && agent.Build.Buildpack.Language == string(utils.LanguagePython)
		promoteBallerinaBuildpack := agent.Build != nil && agent.Build.Buildpack != nil && agent.Build.Buildpack.Language == string(utils.LanguageBallerina)
		// Resolve the instrumentation version for the target env: request override
		// (validated) -> source env's pinned version. The resolved version is
		// persisted below and its image is written as a per-env override.
		promoteLanguageVersion := ""
		if agent.Build != nil && agent.Build.Buildpack != nil {
			promoteLanguageVersion = agent.Build.Buildpack.LanguageVersion
		}
		var existingInstrumentationVersion *string
		if existingConfig != nil {
			existingInstrumentationVersion = existingConfig.InstrumentationVersion
		}
		resolvedInstrumentationVersion, promoteInstrumentationImage, resolveErr := s.resolveInstrumentationImageOverride(
			promotePythonBuildpack, promoteLanguageVersion, req.InstrumentationVersion.Get(), existingInstrumentationVersion,
		)
		if resolveErr != nil {
			return resolveErr
		}
		traitEnvConfigs = buildTraitEnvConfigs(agentName, policies, targetArtifactID, resilienceTimeoutSeconds, promotePythonBuildpack, promoteBallerinaBuildpack, tracingCfg.EnableAutoInstrumentation, promoteInstrumentationImage)
		promoteCTConfigs = buildComponentTypeEnvConfigs(targetEnv)

		apiKey, apiKeyErr := s.generateAgentAPIKey(ctx, ouID, projectName, agentName, req.TargetEnvironment)
		if apiKeyErr != nil {
			s.logger.Warn("Failed to generate agent API key for promotion", "agentName", agentName, "environment", req.TargetEnvironment, "error", apiKeyErr)
		} else if apiKeySecretRef, apiKeySecretProperty, storeErr := s.storeAgentAPIKey(ctx, ouID, projectName, agentName, req.TargetEnvironment, apiKey); storeErr != nil {
			s.logger.Warn("Failed to store agent API key for promotion", "agentName", agentName, "environment", req.TargetEnvironment, "error", storeErr)
		} else {
			injectAgentAPIKeySecretRef(traitEnvConfigs, agentName, apiKeySecretRef, apiKeySecretProperty)
		}

		var promoteResilienceTimeoutSeconds *int32
		if resilienceTimeoutSeconds > 0 {
			promoteResilienceTimeoutSeconds = &resilienceTimeoutSeconds
		}
		// Persist config for the target environment
		agentConfig := &models.AgentConfig{
			OUID:                      ouID,
			ProjectName:               projectName,
			AgentName:                 agentName,
			EnvironmentName:           req.TargetEnvironment,
			EnableAutoInstrumentation: tracingCfg.EnableAutoInstrumentation,
			InstrumentationVersion:    resolvedInstrumentationVersion,
			EnableApiKeySecurity:      apiCfg.EnableApiKeySecurity,
			CORSEnabled:               apiCfg.CORSEnabled,
			CORSAllowOrigins:          apiCfg.CORSAllowOrigins,
			CORSAllowMethods:          apiCfg.CORSAllowMethods,
			CORSAllowHeaders:          apiCfg.CORSAllowHeaders,
			CORSAllowCredentials:      apiCfg.CORSAllowCredentials,
			EnableOAuthSecurity:       apiCfg.EnableOAuthSecurity,
			OAuthIssuers:              apiCfg.OAuthIssuers,
			OAuthAudiences:            apiCfg.OAuthAudiences,
			OAuthHeaderName:           apiCfg.OAuthHeaderName,
			OAuthAuthHeaderPrefix:     apiCfg.OAuthAuthHeaderPrefix,
			OAuthForwardToken:         apiCfg.OAuthForwardToken,
			ResilienceTimeoutSeconds:  promoteResilienceTimeoutSeconds,
		}
		if upsertErr := s.agentConfigRepo.Upsert(agentConfig); upsertErr != nil {
			s.logger.Error("Failed to persist agent config for target environment", "agentName", agentName, "environment", req.TargetEnvironment, "error", upsertErr)
		}
	}

	// Promote via OC client. The target environment's AgentID is already
	// guaranteed to exist and be COMPLETED at this point — the pre-promote
	// hard block above returns before reaching here otherwise — so there is
	// nothing left to provision for it after a successful promote.
	if err := s.ocClient.PromoteComponent(ctx, ouID, projectName, agentName, req.SourceEnvironment, req.TargetEnvironment, envOverrides, fileOverrides, traitEnvConfigs, promoteCTConfigs); err != nil {
		s.logger.Error("Failed to promote agent", "agentName", agentName, "sourceEnvironment", req.SourceEnvironment, "targetEnvironment", req.TargetEnvironment, "error", err)
		return fmt.Errorf("failed to promote agent: %w", err)
	}

	s.logger.Info("Agent promoted successfully", "agentName", agentName, "sourceEnvironment", req.SourceEnvironment, "targetEnvironment", req.TargetEnvironment)
	return nil
}

// promotionIdentityPollInterval/promotionIdentityPollBudget bound
// pollForTargetIdentityReady. Variables (not consts) so tests can shrink them
// to keep the "still not ready after polling" path fast.
var (
	promotionIdentityPollInterval = 300 * time.Millisecond
	promotionIdentityPollBudget   = 3 * time.Second
)

// pollForTargetIdentityReady gives a just-kicked-off AgentID provisioning
// attempt (ProvisionForEnvironmentIfMissing's Thunder call runs on a detached
// goroutine — see AgentThunderProvisioningService.ProvisionForAgent) a short,
// bounded chance to finish before PromoteAgent's hard block gives up. Without
// this, promoting to a target environment that didn't have a binding yet
// would ALWAYS fail once (the goroutine can't possibly have completed a real
// network round trip in the few microseconds since it was scheduled) before a
// manual retry succeeds — even though a healthy Thunder+OpenBao round trip
// normally finishes in well under a second. A genuinely stuck/slow attempt
// still falls through to the hard block after this budget, unchanged from
// before. Returns nil (unwrapped, not an error) if the budget elapses or ctx
// is done first, in either case leaving the caller to hard-block as usual.
func (s *agentManagerService) pollForTargetIdentityReady(ctx context.Context, ouID, projectName, agentName, envName string) []client.EnvVar {
	deadline := time.Now().Add(promotionIdentityPollBudget)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(promotionIdentityPollInterval):
		}
		vars, err := s.agentIdentityInjection.EnvVarsForEnvironment(ctx, ouID, projectName, agentName, envName)
		if err != nil {
			// A transient error here doesn't warrant retrying the poll loop —
			// the original (pre-poll) call already succeeded moments earlier,
			// so treat this the same as "still not ready" and let the hard
			// block's own error path (which re-reads state) take over.
			return nil
		}
		if len(vars) > 0 {
			return vars
		}
	}
	return nil
}

// buildPromotionIdentityBlockedError builds a state-specific error for
// PromoteAgent's hard block, distinguishing WHY the target environment's
// AgentID isn't ready — EnvVarsForEnvironment collapses several very
// different states into the same "nothing to inject" nil (see
// agentIdentityInjectionService.injectableBinding): no binding, still
// provisioning, permanently failed, and revoked all look identical from
// there. Only "still provisioning" is something a retry actually fixes; the
// other states need a state-specific message so the caller knows what to do
// instead of being told to simply wait and retry.
func (s *agentManagerService) buildPromotionIdentityBlockedError(ctx context.Context, ouID, projectName, agentName, envName string) error {
	if s.agentThunderProvisioning == nil {
		// AgentID provisioning is disabled for this deployment, so nothing can
		// ever mint the target's own credential — unlike the "just triggered"
		// case below, this will never resolve on its own by retrying.
		return fmt.Errorf(
			"%w: agent %q has no AgentID identity for target environment %q, and AgentID provisioning is disabled "+
				"for this deployment — promotion is blocked to prevent the promoted pod from inheriting a "+
				"different environment's real credentials. Enable AgentID provisioning and provision this "+
				"environment before promoting",
			utils.ErrInvalidInput, agentName, envName,
		)
	}

	state, err := s.agentThunderProvisioning.GetBindingState(ctx, ouID, projectName, agentName, envName)
	if err != nil {
		s.logger.Warn("Failed to read agent thunder binding state for promotion error message, falling back to a generic message",
			"agentName", agentName, "environment", envName, "error", err)
		state = nil
	}

	switch {
	case state == nil:
		return fmt.Errorf(
			"%w: agent %q has no AgentID identity for target environment %q yet — provisioning was just triggered; "+
				"check GET .../identities for this agent and retry shortly",
			utils.ErrInvalidInput, agentName, envName,
		)
	case state.Status == models.AgentThunderStatusFailed:
		return fmt.Errorf(
			"%w: agent %q's AgentID provisioning for target environment %q has permanently failed (%s). "+
				"Retrying promotion will not fix this — check GET .../identities for this agent and re-provision the environment",
			utils.ErrInvalidInput, agentName, envName, state.LastError,
		)
	case state.Status == models.AgentThunderStatusCompleted && !state.HasSecret:
		return fmt.Errorf(
			"%w: agent %q's AgentID credential for target environment %q has been revoked. "+
				"Retrying promotion will not fix this — regenerate the credential for %q before promoting",
			utils.ErrInvalidInput, agentName, envName, envName,
		)
	default: // Pending / InProgress, or any other in-flight state
		return fmt.Errorf(
			"%w: agent %q's AgentID identity for target environment %q is not ready yet (still provisioning). "+
				"Promotion is blocked to prevent the promoted pod from inheriting a different environment's credentials — "+
				"check GET .../identities for this agent and retry once it shows status=completed for %q",
			utils.ErrInvalidInput, agentName, envName, envName,
		)
	}
}

// UpdateAgentDeploySettings updates per-environment deploy settings (CORS, API key security,
// auto instrumentation) on an existing release binding without redeploying or promoting the
// agent. Triggers a pod rollout so policy changes take effect immediately. Any field omitted
// from the request keeps its current DB value.
func (s *agentManagerService) UpdateAgentDeploySettings(ctx context.Context, ouID, projectName, agentName string, req *spec.UpdateAgentDeploySettingsRequest) error {
	s.logger.Info("Updating agent deploy settings", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", req.EnvironmentName)

	if req.EnvironmentName == "" {
		return fmt.Errorf("%w: environmentName is required", utils.ErrInvalidInput)
	}

	// Validate org/agent/env exist.
	org, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		return translateOrgError(err)
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName)
	if err != nil {
		return translateAgentError(err)
	}
	if agent.Type.Type != string(utils.AgentTypeAPI) {
		return fmt.Errorf("%w: deploy settings only apply to API-type agents (got %q)", utils.ErrInvalidInput, agent.Type.Type)
	}
	targetEnv, err := s.ocClient.GetEnvironment(ctx, ouID, req.EnvironmentName)
	if err != nil {
		return translateEnvironmentError(err)
	}

	// Resolve final settings: precedence is request → existing DB
	var existingConfig *models.AgentConfig
	if cfg, getErr := s.agentConfigRepo.Get(ouID, projectName, agentName, req.EnvironmentName); getErr == nil {
		existingConfig = cfg
	}
	tracingCfg := resolveTracingConfig(existingConfig, req.EnableAutoInstrumentation, false)
	apiCfg := resolveAPIConfig(existingConfig, req.EnableApiKeySecurity, req.CorsConfig, req.EnableOAuthSecurity, req.OauthConfig, false)
	resilienceTimeoutSeconds := resolveResilienceTimeoutSeconds(existingConfig, req.ResilienceTimeoutSeconds, false)
	if err := validateAuthExclusivity(apiCfg); err != nil {
		return err
	}
	if err := validateOAuthSecurityConfig(apiCfg); err != nil {
		return err
	}
	if targetEnv != nil {
		if err := s.validateOAuthIssuersInEnvironment(targetEnv.UUID, apiCfg); err != nil {
			return err
		}
	}
	policies := buildPolicies(apiCfg)

	// Each environment must keep its own existing API artifact UUID so we don't churn the
	// gateway's RestApi binding. ensureAgentEnvAPIArtifact is idempotent: returns the existing
	// row if one is already allocated for (agent, env).
	artifact, artifactErr := ensureAgentEnvAPIArtifact(s.db, s.artifactRepo, ouID, projectName, agentName, targetEnv.UUID)
	if artifactErr != nil {
		return fmt.Errorf("failed to ensure agent env API artifact: %w", artifactErr)
	}
	isPythonBuildpack := agent.Build != nil && agent.Build.Buildpack != nil && agent.Build.Buildpack.Language == string(utils.LanguagePython)
	isBallerinaBuildpack := agent.Build != nil && agent.Build.Buildpack != nil && agent.Build.Buildpack.Language == string(utils.LanguageBallerina)
	// Resolve the instrumentation version: request override (validated) -> the
	// env's currently-pinned version. The resolved version is persisted below and
	// its image is written as a per-env override on the OTEL trait.
	languageVersion := ""
	if agent.Build != nil && agent.Build.Buildpack != nil {
		languageVersion = agent.Build.Buildpack.LanguageVersion
	}
	var existingInstrumentationVersion *string
	if existingConfig != nil {
		existingInstrumentationVersion = existingConfig.InstrumentationVersion
	}
	resolvedInstrumentationVersion, instrumentationImage, resolveErr := s.resolveInstrumentationImageOverride(
		isPythonBuildpack, languageVersion, req.InstrumentationVersion.Get(), existingInstrumentationVersion,
	)
	if resolveErr != nil {
		return resolveErr
	}
	traitEnvConfigs := buildTraitEnvConfigs(agentName, policies, artifact.UUID.String(), resilienceTimeoutSeconds, isPythonBuildpack, isBallerinaBuildpack, tracingCfg.EnableAutoInstrumentation, instrumentationImage)

	// Re-attach the per-env agent API key secret ref. buildTraitEnvConfigs omits it and the update
	// below REPLACES the binding's trait configs, so without this a non-lowest env falls back to the
	// base (lowest env's) ref and never sees its own key — including after a regenerate. Resolving is
	// read-only (no new key minted); it heals bindings whose ref was previously dropped.
	if apiKeySecretRef, apiKeySecretProperty, resolveErr := s.resolveAgentAPIKeySecretRef(ctx, ouID, projectName, agentName, req.EnvironmentName); resolveErr != nil {
		s.logger.Warn("Failed to resolve agent API key secret ref for deploy settings; env-injection trait will fall back to the base ref",
			"agentName", agentName, "environment", req.EnvironmentName, "error", resolveErr)
	} else {
		injectAgentAPIKeySecretRef(traitEnvConfigs, agentName, apiKeySecretRef, apiKeySecretProperty)
	}

	// Apply to the release binding (atomic: trait configs + component-type configs + restartedAt in a single update).
	settingsCTConfigs := buildComponentTypeEnvConfigs(targetEnv)
	if updateErr := s.ocClient.UpdateReleaseBindingTraitConfigs(ctx, ouID, agentName, req.EnvironmentName, traitEnvConfigs, settingsCTConfigs); updateErr != nil {
		s.logger.Error("Failed to update release binding deploy settings", "agentName", agentName, "environment", req.EnvironmentName, "error", updateErr)
		return fmt.Errorf("failed to update deploy settings: %w", updateErr)
	}

	var settingsResilienceTimeoutSeconds *int32
	if resilienceTimeoutSeconds > 0 {
		settingsResilienceTimeoutSeconds = &resilienceTimeoutSeconds
	}
	// Persist resolved config so subsequent deploy/promote calls see the current values.
	agentConfig := &models.AgentConfig{
		OUID:                      ouID,
		ProjectName:               projectName,
		AgentName:                 agentName,
		EnvironmentName:           req.EnvironmentName,
		EnableAutoInstrumentation: tracingCfg.EnableAutoInstrumentation,
		InstrumentationVersion:    resolvedInstrumentationVersion,
		EnableApiKeySecurity:      apiCfg.EnableApiKeySecurity,
		CORSEnabled:               apiCfg.CORSEnabled,
		CORSAllowOrigins:          apiCfg.CORSAllowOrigins,
		CORSAllowMethods:          apiCfg.CORSAllowMethods,
		CORSAllowHeaders:          apiCfg.CORSAllowHeaders,
		CORSAllowCredentials:      apiCfg.CORSAllowCredentials,
		EnableOAuthSecurity:       apiCfg.EnableOAuthSecurity,
		OAuthIssuers:              apiCfg.OAuthIssuers,
		OAuthAudiences:            apiCfg.OAuthAudiences,
		OAuthHeaderName:           apiCfg.OAuthHeaderName,
		OAuthAuthHeaderPrefix:     apiCfg.OAuthAuthHeaderPrefix,
		OAuthForwardToken:         apiCfg.OAuthForwardToken,
		ResilienceTimeoutSeconds:  settingsResilienceTimeoutSeconds,
	}
	if upsertErr := s.agentConfigRepo.Upsert(agentConfig); upsertErr != nil {
		s.logger.Error("Failed to persist agent deploy settings", "agentName", agentName, "environment", req.EnvironmentName, "error", upsertErr)
		return fmt.Errorf("failed to persist agent deploy settings: %w", upsertErr)
	}

	s.logger.Info("Agent deploy settings updated successfully", "agentName", agentName, "environment", req.EnvironmentName)
	return nil
}

// UpdateAgentConfigurations replaces the per-environment env vars and file mounts on the
// agent's release binding. System-managed env vars (LLM_PROVIDER_*, MCP, OTEL, agent API key)
// are filtered out of req.Env server-side and re-injected from the agent's DB-tracked config,
// so the caller never has to know about them (mirrors the deploy/promote flow).
// Triggers a pod rollout via the same Get→mutate→Update cycle that writes the overrides.
func (s *agentManagerService) UpdateAgentConfigurations(ctx context.Context, ouID, projectName, agentName string, req *spec.UpdateAgentConfigurationsRequest) error {
	s.logger.Info("Updating agent configurations", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", req.EnvironmentName)

	if req.EnvironmentName == "" {
		return fmt.Errorf("%w: environmentName is required", utils.ErrInvalidInput)
	}
	if err := utils.ValidateFileMounts(req.Files); err != nil {
		return fmt.Errorf("%w: %s", utils.ErrInvalidInput, err.Error())
	}

	// Validate org/agent/env exist.
	org, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		return translateOrgError(err)
	}
	if _, err := s.ocClient.GetComponent(ctx, org.Name, projectName, agentName); err != nil {
		return translateAgentError(err)
	}
	if _, err := s.ocClient.GetEnvironment(ctx, ouID, req.EnvironmentName); err != nil {
		return translateEnvironmentError(err)
	}

	// Fetch system-managed env vars + their keys for the target env. We must filter the user's
	// env list to drop these before processEnvVars (which would otherwise mangle their secret
	// key refs), then re-append the canonical system values.
	systemManagedEnvVars, systemManagedKeys, sysEnvErr := s.getSystemManagedEnvVars(ctx, ouID, projectName, req.EnvironmentName, agentName)
	if sysEnvErr != nil {
		s.logger.Warn("Failed to fetch system-managed env vars for configurations update", "agentName", agentName, "error", sysEnvErr)
		systemManagedEnvVars = nil
		systemManagedKeys = nil
	}

	// AgentID credentials must survive the override rewrite below, which replaces
	// the environment's entire env var set. Unlike the lenient LLM-var fallback
	// above, an error here aborts: rewriting overrides without re-appending the
	// identity vars would silently strip the agent's credentials.
	identityEnvVars, idErr := s.agentIdentityInjection.EnvVarsForEnvironment(ctx, ouID, projectName, agentName, req.EnvironmentName)
	if idErr != nil {
		s.logger.Error("Failed to build agent identity env vars, aborting configurations update to prevent credential loss",
			"agentName", agentName, "environment", req.EnvironmentName, "error", idErr)
		return fmt.Errorf("failed to build agent identity env vars: %w", idErr)
	}
	systemManagedKeys = mergeAgentIdentityEnvVarKeys(systemManagedKeys)

	// Build env overrides (nil-vs-empty has meaning at the client layer: nil = leave existing,
	// empty slice = clear).
	var envOverrides []client.EnvVar
	if req.Env != nil {
		userEnv := req.Env
		if len(systemManagedKeys) > 0 {
			userEnv = make([]spec.EnvironmentVariable, 0, len(req.Env))
			for _, env := range req.Env {
				if !systemManagedKeys[env.Key] {
					userEnv = append(userEnv, env)
				} else {
					s.logger.Debug("Filtering system-managed env var from configurations request", "key", env.Key)
				}
			}
		}
		processed, err := s.processEnvVars(ctx, ouID, projectName, req.EnvironmentName, agentName, userEnv, req.Files)
		if err != nil {
			s.logger.Error("Failed to process env vars", "agentName", agentName, "environment", req.EnvironmentName, "error", err)
			return fmt.Errorf("failed to process env vars: %w", err)
		}
		envOverrides = append(processed, systemManagedEnvVars...)
		envOverrides = append(envOverrides, identityEnvVars...)
		if envOverrides == nil {
			// Caller sent an empty list and there are no system-managed vars to inject —
			// preserve the "clear all" intent rather than collapsing to nil.
			envOverrides = []client.EnvVar{}
		}
	}

	// Build file overrides.
	var fileOverrides []client.FileVar
	if req.Files != nil {
		processed, err := s.processFileVars(ctx, ouID, projectName, req.EnvironmentName, agentName, req.Files)
		if err != nil {
			s.logger.Error("Failed to process file mounts", "agentName", agentName, "environment", req.EnvironmentName, "error", err)
			return fmt.Errorf("failed to process file mounts: %w", err)
		}
		fileOverrides = processed
		if fileOverrides == nil {
			fileOverrides = []client.FileVar{}
		}
	}

	if envOverrides == nil && fileOverrides == nil {
		// Nothing requested — surface as a clear error rather than silently no-op'ing.
		return fmt.Errorf("%w: request must include env or files", utils.ErrInvalidInput)
	}

	if err := s.ocClient.ReplaceReleaseBindingWorkloadOverrides(ctx, ouID, agentName, req.EnvironmentName, envOverrides, fileOverrides); err != nil {
		s.logger.Error("Failed to replace release binding workload overrides", "agentName", agentName, "environment", req.EnvironmentName, "error", err)
		return fmt.Errorf("failed to update agent configurations: %w", err)
	}

	s.logger.Info("Agent configurations updated successfully", "agentName", agentName, "environment", req.EnvironmentName)
	return nil
}

// isValidPromotionPath checks if the given source → target promotion is allowed by the pipeline
func isValidPromotionPath(promotionPaths []models.PromotionPath, source, target string) bool {
	for _, path := range promotionPaths {
		if path.SourceEnvironmentRef == source {
			for _, t := range path.TargetEnvironmentRefs {
				if t.Name == target {
					return true
				}
			}
		}
	}
	return false
}

// getSystemManagedEnvVars fetches existing env vars from the Component CR / ReleaseBinding and
// identifies system-managed secret env vars (e.g., LLM provider config API keys).
//
// System-managed env vars are identified by looking up the secretRef in the DB: if it is
// recorded in agent_env_config_variables_mapping for this agent's LLM configurations, it is
// system-managed. This is provider-agnostic — it does not rely on secret reference name
// patterns.
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
	ouID, projectName, environmentName, componentName string,
) ([]client.EnvVar, map[string]bool, error) {
	existingConfigs, err := s.ocClient.GetComponentConfigurations(ctx, ouID, projectName, componentName, environmentName)
	if err != nil {
		return nil, nil, err
	}
	if len(existingConfigs) == 0 {
		s.logger.Debug("No existing env vars found in component configurations", "agentName", componentName)
		return nil, nil, nil
	}

	// Fetch the set of SecretReference names that belong to LLM configurations for this agent
	// and environment from the DB. These are the source of truth — provider-agnostic.
	llmSecretRefs, err := s.agentConfigurationService.ListAgentLLMConfigSecretReferences(ctx, componentName, ouID, environmentName)
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

// cloneEnvSecretForPromotion gives the target environment its own copy of the
// source environment's agent secret. Workload overrides cloned from the source
// environment reference the source secret by name; without a copy, both
// environments would share one secret and a later secret edit in either
// environment could break the other's rendering. Env var and file mount
// references to the source secret are re-pointed at the target copy.
// No-op when the cloned overrides don't reference the source env secret.
func (s *agentManagerService) cloneEnvSecretForPromotion(
	ctx context.Context,
	ouID, projectName, agentName, sourceEnv, targetEnv string,
	envVars []client.EnvVar,
	fileVars []client.FileVar,
) ([]client.EnvVar, []client.FileVar, error) {
	srcLocation := secretmanagersvc.SecretLocation{
		OrgName:         ouID,
		ProjectName:     projectName,
		EnvironmentName: sourceEnv,
		EntityName:      agentName,
	}
	srcSecretName := srcLocation.SecretRefName()

	refersToSrc := func(vf *client.EnvVarValueFrom) bool {
		return vf != nil && vf.SecretKeyRef != nil && vf.SecretKeyRef.Name == srcSecretName
	}
	referenced := false
	for _, ev := range envVars {
		if refersToSrc(ev.ValueFrom) {
			referenced = true
			break
		}
	}
	if !referenced {
		for _, fv := range fileVars {
			if refersToSrc(fv.ValueFrom) {
				referenced = true
				break
			}
		}
	}
	if !referenced {
		return envVars, fileVars, nil
	}

	if s.secretMgmtClient == nil {
		return nil, nil, fmt.Errorf("secret management is not initialized; cannot clone secret for promotion")
	}

	srcSecret, err := s.ocClient.GetSecret(ctx, ouID, srcSecretName)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read source environment secret %q: %w", srcSecretName, err)
	}
	data := make(map[string]string, len(srcSecret.Data))
	for k, v := range srcSecret.Data {
		data[k] = string(v)
	}

	tgtLocation := srcLocation
	tgtLocation.EnvironmentName = targetEnv
	tgtSecretName, err := s.secretMgmtClient.CreateSecret(ctx, tgtLocation, data)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create target environment secret: %w", err)
	}

	for i := range envVars {
		if refersToSrc(envVars[i].ValueFrom) {
			envVars[i].ValueFrom.SecretKeyRef.Name = tgtSecretName
		}
	}
	for i := range fileVars {
		if refersToSrc(fileVars[i].ValueFrom) {
			fileVars[i].ValueFrom.SecretKeyRef.Name = tgtSecretName
		}
	}

	s.logger.Info("Cloned environment secret for promotion",
		"agentName", agentName, "sourceSecret", srcSecretName, "targetSecret", tgtSecretName,
		"sourceEnv", sourceEnv, "targetEnv", targetEnv)
	return envVars, fileVars, nil
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
//   - If value is provided: stores/updates the secret value in the secret store
//   - Returns env var with secretKeyRef (Name=K8s Secret name, Key=property)
//
// For plain env vars:
//   - Returns env var with the value directly
//
// fileMounts are also processed for secrets: sensitive file mount values are stored in the
// same KV path alongside env var secrets.
func (s *agentManagerService) processEnvVars(
	ctx context.Context,
	ouID, projectName, environmentName, componentName string,
	envVars []spec.EnvironmentVariable,
	fileMounts []spec.FileMount,
) ([]client.EnvVar, error) {
	secretData := make(map[string]string)
	var preservedSecretKeys []string

	// Build secret location for the secret store
	location := secretmanagersvc.SecretLocation{
		OrgName:         ouID,
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

// processFileVars converts spec.FileMount entries to client.FileVar entries.
// Sensitive file mounts use secretKeyRef pointing to the K8s Secret (secrets are
// already stored in KV by processEnvVars which handles both env and file secrets).
func (s *agentManagerService) processFileVars(
	ctx context.Context,
	ouID, projectName, environmentName, componentName string,
	fileMounts []spec.FileMount,
) ([]client.FileVar, error) {
	if len(fileMounts) == 0 {
		return make([]client.FileVar, 0), nil
	}

	// Build secret location to derive the secretRefName
	location := secretmanagersvc.SecretLocation{
		OrgName:         ouID,
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

func (s *agentManagerService) ListAgentBuilds(ctx context.Context, ouID string, projectName string, agentName string, limit int32, offset int32) ([]*models.BuildResponse, int32, error) {
	s.logger.Info("Listing agent builds", "agentName", agentName, "ouID", ouID, "projectName", projectName, "limit", limit, "offset", offset)
	// Validate organization exists
	_, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to validate organization", "ouID", ouID, "error", err)
		return nil, 0, translateOrgError(err)
	}

	// Check if component already exists
	agent, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch component", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, 0, translateAgentError(err)
	}

	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, 0, fmt.Errorf("build operation is not supported for agent type: '%s'", agent.Provisioning.Type)
	}

	// Fetch all builds from OpenChoreo first
	allBuilds, err := s.ocClient.ListBuilds(ctx, ouID, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to list builds from OpenChoreo", "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, 0, err
	}

	total := int32(len(allBuilds))
	paginatedBuilds := paginateSlice(allBuilds, offset, limit)

	s.logger.Info("Listed builds successfully", "agentName", agentName, "ouID", ouID, "projectName", projectName, "totalBuilds", total, "returnedBuilds", len(paginatedBuilds))
	return paginatedBuilds, total, nil
}

func (s *agentManagerService) GetBuild(ctx context.Context, ouID string, projectName string, agentName string, buildName string) (*models.BuildDetailsResponse, error) {
	s.logger.Info("Getting build details", "agentName", agentName, "buildName", buildName, "ouID", ouID, "projectName", projectName)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
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
	build, err := s.ocClient.GetBuild(ctx, ouID, projectName, agentName, buildName)
	if err != nil {
		s.logger.Error("Failed to get build from OpenChoreo", "buildName", buildName, "agentName", agentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, translateBuildError(err)
	}

	s.logger.Info("Fetched build successfully", "agentName", agentName, "ouID", ouID, "projectName", projectName, "buildName", build.Name)
	return build, nil
}

func (s *agentManagerService) GetAgentDeployments(ctx context.Context, ouID string, projectName string, agentName string) ([]*models.DeploymentResponse, error) {
	s.logger.Info("Getting agent deployments", "agentName", agentName, "ouID", ouID, "projectName", projectName)
	project, err := s.ocClient.GetProject(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "org", ouID, "error", err)
		return nil, translateProjectError(err)
	}
	// Get deployment pipeline name from project
	pipelineName := project.DeploymentPipeline
	deployments, err := s.ocClient.GetDeployments(ctx, ouID, pipelineName, projectName, agentName)
	if err != nil {
		s.logger.Error("Failed to get deployments from OpenChoreo", "agentName", agentName, "pipelineName", pipelineName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get deployments for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched deployments successfully", "agentName", agentName, "ouID", ouID, "projectName", projectName, "deploymentCount", len(deployments))

	// Reconcile isolation-tier runtimeClassName on out-of-band bindings. OpenChoreo's AutoDeploy
	// creates a release binding when a build completes, WITHOUT the backend's deploy-time config
	// write — so an agent in a gVisor/Kata environment first comes up on the default runtime. The
	// deploy-status poll is the natural point to detect that the binding now exists and correct it.
	// Only platform-hosted API agents carry a SandboxTemplate (and thus a runtimeClassName);
	// external agents have no pod to reconcile, so skip the work — and its per-environment API
	// calls — entirely for them. Best-effort and idempotent: it never fails the read, is a no-op
	// for all-runc setups, and converges in a single write per binding.
	if agent, err := s.ocClient.GetComponent(ctx, ouID, projectName, agentName); err != nil {
		s.logger.Warn("isolation reconcile: failed to fetch agent for type gate", "agentName", agentName, "error", err)
	} else if agent.Type.Type == string(utils.AgentTypeAPI) {
		s.reconcileIsolationRuntimeClass(ctx, ouID, agentName, deployments)
	}

	return deployments, nil
}

// reconcileIsolationRuntimeClass ensures every deployment whose environment has an isolation tier
// carries the matching runtimeClassName on its release binding. See EnsureReleaseBindingRuntimeClass
// for why this is needed (AutoDeploy bypasses the deploy-time config write). Best-effort: failures
// are logged and ignored so they never break the deployments read.
func (s *agentManagerService) reconcileIsolationRuntimeClass(ctx context.Context, ouID, agentName string, deployments []*models.DeploymentResponse) {
	if len(deployments) == 0 {
		return
	}
	envs, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		s.logger.Warn("isolation reconcile: failed to list environments", "agentName", agentName, "error", err)
		return
	}
	// Map only the environments that actually have an isolation tier; for all-runc setups this
	// stays empty and we skip the work entirely.
	rcByEnv := make(map[string]string)
	for _, e := range envs {
		if rc := runtimeClassForIsolationTier(e.IsolationTier); rc != "" {
			rcByEnv[e.Name] = rc
		}
	}
	if len(rcByEnv) == 0 {
		return
	}
	for _, d := range deployments {
		rc, ok := rcByEnv[d.Environment]
		if !ok {
			continue
		}
		if err := s.ocClient.EnsureReleaseBindingRuntimeClass(ctx, ouID, agentName, d.Environment, rc); err != nil {
			s.logger.Warn("isolation reconcile: failed to set runtimeClassName",
				"agentName", agentName, "environment", d.Environment, "runtimeClass", rc, "error", err)
		}
	}
}

// UpdateAgentDeploymentState updates the deployment state of an agent in a specific environment
func (s *agentManagerService) UpdateAgentDeploymentState(ctx context.Context, ouID string, projectName string, agentName string, environment string, state string) error {
	s.logger.Info("Updating agent deployment state", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment, "state", state)

	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
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
	_, err = s.ocClient.GetEnvironment(ctx, ouID, environment)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environment, "ouID", ouID, "error", err)
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
	err = s.ocClient.UpdateDeploymentState(ctx, ouID, projectName, agentName, environment, bindingState)
	if err != nil {
		s.logger.Error("Failed to update deployment state", "agentName", agentName, "environment", environment, "state", state, "error", err)
		return fmt.Errorf("failed to update deployment state for agent %s in environment %s: %w", agentName, environment, err)
	}

	s.logger.Info("Updated deployment state successfully", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment, "state", state)
	return nil
}

func (s *agentManagerService) GetAgentEndpoints(ctx context.Context, ouID string, projectName string, agentName string, environmentName string) (map[string]models.EndpointsResponse, error) {
	s.logger.Info("Getting agent endpoints", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environmentName)
	// Validate organization exists
	org, err := s.ocClient.GetOrganization(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return nil, translateOrgError(err)
	}
	project, err := s.ocClient.GetProject(ctx, ouID, projectName)
	if err != nil {
		s.logger.Error("Failed to find project", "projectName", projectName, "ouID", ouID, "error", err)
		return nil, translateProjectError(err)
	}
	agent, err := s.ocClient.GetComponent(ctx, org.Name, project.Name, agentName)
	if err != nil {
		s.logger.Error("Failed to fetch agent", "agentName", agentName, "projectName", projectName, "ouID", ouID, "error", err)
		return nil, translateAgentError(err)
	}
	if agent.Provisioning.Type != string(utils.InternalAgent) {
		return nil, fmt.Errorf("endpoints are not supported for agent type: '%s'", agent.Provisioning.Type)
	}
	// Check if environment exists
	_, err = s.ocClient.GetEnvironment(ctx, ouID, environmentName)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environmentName, "ouID", ouID, "error", err)
		return nil, translateEnvironmentError(err)
	}
	s.logger.Debug("Fetching agent endpoints from OpenChoreo", "agentName", agentName, "environment", environmentName, "ouID", ouID, "projectName", projectName)
	endpoints, err := s.ocClient.GetComponentEndpoints(ctx, ouID, projectName, agentName, environmentName)
	if err != nil {
		s.logger.Error("Failed to fetch endpoints", "agentName", agentName, "environment", environmentName, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get endpoints for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched endpoints successfully", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environmentName, "endpointCount", len(endpoints))
	return endpoints, nil
}

func (s *agentManagerService) GetAgentConfigurations(ctx context.Context, ouID string, projectName string, agentName string, environment string) ([]models.EnvVars, error) {
	s.logger.Info("Getting agent configurations", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment)
	if _, err := s.ocClient.GetOrganization(ctx, ouID); err != nil {
		s.logger.Error("Failed to find organization", "ouID", ouID, "error", err)
		return nil, translateOrgError(err)
	}
	// Check if environment exists
	_, err := s.ocClient.GetEnvironment(ctx, ouID, environment)
	if err != nil {
		s.logger.Error("Failed to validate environment", "environment", environment, "ouID", ouID, "error", err)
		return nil, translateEnvironmentError(err)
	}

	s.logger.Debug("Fetching agent configurations from OpenChoreo", "agentName", agentName, "environment", environment, "ouID", ouID, "projectName", projectName)
	configurations, err := s.ocClient.GetComponentConfigurations(ctx, ouID, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch configurations", "agentName", agentName, "environment", environment, "ouID", ouID, "projectName", projectName, "error", err)
		return nil, fmt.Errorf("failed to get configurations for agent %s: %w", agentName, err)
	}

	// Build the set of system-managed env var keys for this agent + env. The
	// authoritative source is the env_variables table (populated when the user
	// connects an LLM provider, MCP server, etc.) — not the static
	// SystemInjectedEnvVars allowlist, which only covers platform-injected
	// boot-time vars (OTEL, agent API key).
	systemKeys := map[string]bool{}
	if agent, agentErr := s.ocClient.GetComponent(ctx, ouID, projectName, agentName); agentErr == nil && agent != nil && agent.UUID != "" {
		if dbKeys, listErr := s.agentConfigurationService.ListSystemManagedEnvVarKeys(ctx, agent.Name, ouID, projectName, environment); listErr == nil {
			systemKeys = dbKeys
		} else {
			s.logger.Warn("Failed to list system-managed env var keys; falling back to allowlist only", "agentName", agentName, "environment", environment, "error", listErr)
		}
	}
	// Also mark statically-injected platform vars (OTEL endpoint, agent API key)
	// so they show as read-only regardless of DB state.
	for i := range configurations {
		key := configurations[i].Key
		if systemKeys[key] {
			configurations[i].IsSystem = true
			continue
		}
		if _, ok := client.SystemInjectedEnvVars[key]; ok {
			configurations[i].IsSystem = true
		}
	}

	s.logger.Info("Fetched configurations successfully", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment, "configCount", len(configurations))
	return configurations, nil
}

func (s *agentManagerService) GetAgentFileMounts(ctx context.Context, ouID string, projectName string, agentName string, environment string) ([]models.FileMountEntry, error) {
	s.logger.Info("Getting agent file mounts", "agentName", agentName, "ouID", ouID, "projectName", projectName, "environment", environment)

	fileMounts, err := s.ocClient.GetComponentFileMounts(ctx, ouID, projectName, agentName, environment)
	if err != nil {
		s.logger.Error("Failed to fetch file mounts", "agentName", agentName, "error", err)
		return nil, fmt.Errorf("failed to get file mounts for agent %s: %w", agentName, err)
	}

	s.logger.Info("Fetched file mounts successfully", "agentName", agentName, "count", len(fileMounts))
	return fileMounts, nil
}

// GetAgentEnvConfig returns the full per-environment agent_configs row for (agent, environment)
// — tracing/instrumentation plus CORS and endpoint-authentication settings — or nil when none is
// persisted yet. Unlike GetAgent (which reads only the lowest environment's config), this is scoped
// to the requested environment so the console seeds the correct per-env values.
func (s *agentManagerService) GetAgentEnvConfig(_ context.Context, ouID, projectName, agentName, environment string) (*models.AgentConfig, error) {
	cfg, err := s.agentConfigRepo.Get(ouID, projectName, agentName, environment)
	if errors.Is(err, repositories.ErrAgentConfigNotFound) {
		return nil, nil //nolint:nilnil // "no config yet" is a valid, expected state distinct from an error
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get agent env config: %w", err)
	}
	return cfg, nil
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
