//go:build wireinject
// +build wireinject

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

package wiring

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/google/wire"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	observersvc "github.com/wso2/agent-manager/agent-manager-service/clients/observersvc"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/instrumentation"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/websocket"
)

// Provider sets
var configProviderSet = wire.NewSet(
	ProvideConfigFromPtr,
	ProvideEncryptionKey,
)

var clientProviderSet = wire.NewSet(
	ProvideObserverClient,
	ProvideOCClient,
	ProvideSecretManagementClient,
	ProvidePublisherProvisioner,
	ProvideIdentityClient,
	ProvideOrgResolver,
	thundersvc.NewProber,
	// EnvThunderResolver is wired for AgentIdentityController's passthrough
	// endpoints. AgentID provisioning itself is injected via
	// app.Options.AgentThunderProvisioning, built with its own reader — see app.Run.
	ProvideEnvThunderSecretReader,
	ProvideEnvThunderURLReader,
	ProvideEnvThunderResolver,
)

var serviceProviderSet = wire.NewSet(
	services.NewAgentManagerService,
	services.NewAgentKindService,
	services.NewInfraResourceManager,
	services.NewAgentTokenManagerService,
	ProvideGitCredentialsService,
	services.NewRepositoryService,
	services.NewMonitorExecutor,
	services.NewMonitorManagerService,
	ProvideThunderConfig,
	services.NewMonitorSchedulerService,
	// Provisioning service is injected (see InitializeAppParams); only the
	// reconciler that consumes it is wired here.
	ProvideAgentIdentityInjectionService,
	services.NewAgentThunderReconcilerService,
	services.NewEvaluatorManagerService,
	services.NewEnvironmentService,
	services.NewPlatformGatewayService,
	services.NewLLMProviderTemplateService,
	services.NewLLMProviderService,
	services.NewLLMProxyService,
	services.NewLLMProviderDeploymentService,
	services.NewLLMProviderAPIKeyService,
	services.NewLLMProxyAPIKeyService,
	services.NewAgentAPIKeyService,
	services.NewLLMProxyDeploymentService,
	services.NewMCPProxyService,
	services.NewMCPProxyScopeService,
	// MCPProxyScopeService only needs the narrow redeploy surface; bind it to
	// the concrete service so scope mutations can re-emit gateway policies.
	wire.Bind(new(services.MCPProxyRedeployer), new(*services.MCPProxyService)),
	// The agent-identity controller needs only the identifier-derivation surface.
	wire.Bind(new(controllers.MCPResourceServerIdentifierResolver), new(*services.MCPProxyService)),
	services.NewGatewayInternalAPIService,
	services.NewMonitorScoresService,
	services.NewCatalogService,
	services.NewLLMProxyProvisioner,
	services.NewAgentConfigurationService,
	services.NewLLMTemplateStore,
	services.NewGitSecretService,
	services.NewAIApplicationService,
)

var instrumentationProviderSet = wire.NewSet(
	ProvideInstrumentationCatalog,
	ProvideSupportedPythonVersions,
	ProvideDefaultPythonVersion,
)

var controllerProviderSet = wire.NewSet(
	controllers.NewAgentController,
	controllers.NewAgentKindController,
	controllers.NewInfraResourceController,
	controllers.NewAgentTokenController,
	controllers.NewRepositoryController,
	controllers.NewEnvironmentController,
	controllers.NewGatewayController,
	controllers.NewLLMController,
	controllers.NewLLMDeploymentController,
	controllers.NewLLMProviderAPIKeyController,
	controllers.NewLLMProxyAPIKeyController,
	controllers.NewAgentAPIKeyController,
	controllers.NewLLMProxyDeploymentController,
	controllers.NewMCPProxyController,
	ProvideWebSocketController,
	controllers.NewGatewayInternalController,
	controllers.NewMonitorController,
	controllers.NewMonitorScoresController,
	controllers.NewMonitorScoresPublisherController,
	controllers.NewEvaluatorController,
	controllers.NewCatalogController,
	ProvideAgentBuildOptionsController,
	controllers.NewAgentConfigurationController,
	controllers.NewGitSecretController,
	controllers.NewIdentityController,
	controllers.NewMCPProxyScopeController,
	controllers.NewAgentIdentityController,
)

var testClientProviderSet = wire.NewSet(
	ProvideTestOpenChoreoClient,
	ProvideTestObserverClient,
	ProvideTestSecretManagementClient,
	ProvidePublisherProvisioner,
	ProvideIdentityClient,
	ProvideOrgResolver,
	thundersvc.NewProber,
	ProvideEnvThunderSecretReader,
	ProvideEnvThunderURLReader,
	ProvideEnvThunderResolver,
)

// thunderProvisioningTestSet builds the OpenBao-backed provisioning service for
// the test wiring only; production injects it via InitializeAppParams.
var thunderProvisioningTestSet = wire.NewSet(
	services.NewAgentThunderProvisioningService,
)

// ProvideLogger provides the configured slog.Logger instance
func ProvideLogger() *slog.Logger {
	return slog.Default()
}

// ProvideAuditRecorder builds the audit recorder.
//
// Records go to stdout as structured JSON, where the platform's log pipeline
// already collects them. That also gives the trail a copy this service cannot
// rewrite, since there is no write path from here back into the log store.
func ProvideAuditRecorder(cfg config.Config, logger *slog.Logger) audit.Recorder {
	if !cfg.Audit.Enabled {
		logger.Warn("audit logging is disabled; no record of privileged operations will be kept",
			"setting", "AUDIT_ENABLED")
		return audit.NewNoopRecorder()
	}
	return audit.NewRecorder(audit.NewStdoutSink(), logger, audit.Config{
		BufferSize:    cfg.Audit.BufferSize,
		BatchSize:     cfg.Audit.BatchSize,
		FlushInterval: time.Duration(cfg.Audit.FlushIntervalMs) * time.Millisecond,
	})
}

// ProvideInstrumentationCatalog loads the instrumentation catalog and
// installs it as the process-wide default so legacy callers via
// instrumentation.GetCatalog get the same instance Wire hands to the new
// controllers.
func ProvideInstrumentationCatalog(cfg config.Config) (*instrumentation.Catalog, error) {
	cat, err := instrumentation.Load(
		cfg.OTEL.InstrumentationExtensionPath,
		cfg.OTEL.DefaultInstrumentationVersion,
	)
	if err != nil {
		return nil, err
	}
	if err := validateDefaultCoversBuildpackPython(cat); err != nil {
		return nil, err
	}
	instrumentation.SetCatalog(cat)
	return cat, nil
}

// validateDefaultCoversBuildpackPython rejects a catalog whose default
// instrumentation entry doesn't cover any Python the buildpack provider
// can build. Without this check a misconfigured override (e.g. an
// extension entry that narrows the default's pythonVersions to a value
// the buildpack can't build) lets the server boot cleanly, then the
// create-agent form is unusable: no Python the user can pick is
// compatible with the platform default. Failing fast here surfaces the
// misconfiguration at helm-upgrade time instead.
func validateDefaultCoversBuildpackPython(cat *instrumentation.Catalog) error {
	entry, ok := cat.Get(cat.Default())
	if !ok {
		return fmt.Errorf("default instrumentation version %q not in effective set", cat.Default())
	}
	bpPython := utils.SupportedPythonVersions()
	for _, p := range entry.PythonVersions {
		for _, bp := range bpPython {
			if p == bp {
				return nil
			}
		}
	}
	return fmt.Errorf(
		"default instrumentation version %q supports python %v but the buildpack provider supports %v; no overlap means the create-agent form would offer no valid combination",
		cat.Default(), entry.PythonVersions, bpPython,
	)
}

// SupportedPythonVersions is a distinct type so Wire can disambiguate
// from other []string providers.
type SupportedPythonVersions []string

// DefaultPythonVersion is a distinct type so Wire can disambiguate from
// other string providers.
type DefaultPythonVersion string

// ProvideSupportedPythonVersions exposes the buildpack-derived Python
// list to the AgentBuildOptions controller.
func ProvideSupportedPythonVersions() SupportedPythonVersions {
	return SupportedPythonVersions(utils.SupportedPythonVersions())
}

// defaultPythonVersion is the platform's preferred Python for new
// agents. Hardcoded today; promote to a chart value if customers need
// to override it per install.
const defaultPythonVersion = "3.11"

// ProvideDefaultPythonVersion returns the platform default Python
// version, panicking at boot if the constant is no longer present in
// the buildpack-supported list. The two values share no compile-time
// link; without this guard a developer pruning utils.Buildpacks could
// ship a chart whose /agent-build-options advertises a default the
// backend then rejects.
func ProvideDefaultPythonVersion() DefaultPythonVersion {
	for _, p := range utils.SupportedPythonVersions() {
		if p == defaultPythonVersion {
			return defaultPythonVersion
		}
	}
	panic(fmt.Sprintf(
		"default python version %q not present in buildpack-supported list %v; "+
			"update defaultPythonVersion in wire.go alongside any buildpack change",
		defaultPythonVersion, utils.SupportedPythonVersions(),
	))
}

// ProvideAgentBuildOptionsController wraps the controller constructor
// so Wire can resolve the typed default + supported list back to the
// plain string / []string the constructor takes.
func ProvideAgentBuildOptionsController(
	cat *instrumentation.Catalog,
	supportedPython SupportedPythonVersions,
	defaultPython DefaultPythonVersion,
) controllers.AgentBuildOptionsController {
	return controllers.NewAgentBuildOptionsController(cat, []string(supportedPython), string(defaultPython))
}

// ProvideOCClient creates the OpenChoreo client
func ProvideOCClient(cfg config.Config, authProvider occlient.AuthProvider) (occlient.OpenChoreoClient, error) {
	return occlient.NewOpenChoreoClient(&occlient.Config{
		BaseURL:          cfg.OpenChoreo.BaseURL,
		DefaultNamespace: cfg.OpenChoreo.DefaultNamespace,
		AuthProvider:     authProvider,
	})
}

func ProvideObserverClient(cfg config.Config, authProvider occlient.AuthProvider) (observersvc.ObserverSvcClient, error) {
	return observersvc.NewObserverClient(&observersvc.Config{
		BaseURL:      cfg.Observer.URL,
		AuthProvider: authProvider,
	})
}

// ProvideSecretManagementClient creates the secret management service client.
// If the provider implements secretmanagersvc.SecretReferenceManager and
// reports that it manages SecretReferences itself, the OpenChoreo client is
// not forwarded — preventing the high-level client from making redundant
// SecretReference CRUD calls.
func ProvideSecretManagementClient(cfg config.Config, secretProvider secretmanagersvc.Provider, ocClient occlient.OpenChoreoClient) (secretmanagersvc.SecretManagementClient, error) {
	return secretmanagersvc.NewSecretManagementClientWithConfig(secretmanagersvc.SecretManagementClientConfig{
		StoreConfig: &secretmanagersvc.StoreConfig{
			Provider: cfg.SecretManager.Provider,
			OpenChoreo: &secretmanagersvc.OpenChoreoConfig{
				Client:          ocClient,
				TargetPlaneKind: cfg.SecretManager.TargetPlaneKind,
				TargetPlaneName: cfg.SecretManager.TargetPlaneName,
			},
		},
		Provider: secretProvider,
	})
}

// ProvideGatewayManifestCacheBackend builds the gateway-manifest cache backend
// selected by config.GatewayManifestCache.Backend. Not part of the wire provider
// sets: nothing constructs a dependency on it (the cache is a services-package-level
// var, not a constructor param — see services.SetGatewayManifestCacheBackend's doc
// comment for why). Called directly from app.Run at startup, before the rest of the
// dependency graph is built, so every reader/writer in the services package picks up
// the configured backend from their first call.
func ProvideGatewayManifestCacheBackend(cfg config.Config) (services.GatewayManifestCacheBackend, error) {
	switch cfg.GatewayManifestCache.Backend {
	case "redis":
		return services.NewRedisGatewayManifestCache(cfg.GatewayManifestCache.Redis), nil
	case "memory", "":
		return services.NewInMemoryGatewayManifestCache(), nil
	default:
		// Config validation (validateGatewayManifestCacheConfig) already rejects an
		// unrecognized backend at load time; this is an extra guard against a Config
		// built directly (e.g. in tests) bypassing that validation.
		return nil, fmt.Errorf("unknown gateway manifest cache backend %q", cfg.GatewayManifestCache.Backend)
	}
}

// ProvideGitCredentialsService creates the git credentials service for fetching
// git credentials from workflow plane OpenBao
func ProvideGitCredentialsService(ocClient occlient.OpenChoreoClient, cfg config.Config) (services.GitCredentialsService, error) {
	return services.NewGitCredentialsService(ocClient, cfg)
}

// ProvideNilBuildSecretProvisioner supplies a nil BuildSecretProvisioner for wiring
// paths (e.g. tests) that do not inject one. The real injector receives it as a
// parameter instead (from app.Options); nil is the open-source default and a no-op.
func ProvideNilBuildSecretProvisioner() services.BuildSecretProvisioner {
	return nil
}

// ProvidePublisherProvisioner creates the publisher credential provisioner
// for per-org Thunder OAuth app creation and secret storage via SecretManagementClient
func ProvidePublisherProvisioner(cfg config.Config, encryptionKey []byte, logger *slog.Logger, secretClient secretmanagersvc.SecretManagementClient, ocClient occlient.OpenChoreoClient, credRepo repositories.OrgPublisherCredentialRepository, schedulerCredRepo repositories.OrgSchedulerCredentialRepository) (services.PublisherCredentialProvisioner, error) {
	return services.NewPublisherCredentialProvisioner(cfg, encryptionKey, logger, secretClient, ocClient, credRepo, schedulerCredRepo)
}

var loggerProviderSet = wire.NewSet(
	ProvideLogger,
	ProvideAuditRecorder,
)

var repositoryProviderSet = wire.NewSet(
	ProvideGatewayRepository,
	ProvideAgentKindRepository,
	ProvideLLMProviderTemplateRepository,
	ProvideLLMProviderRepository,
	ProvideLLMProxyRepository,
	ProvideMCPProxyRepository,
	repositories.NewMCPProxyEndpointRepository,
	ProvideDeploymentRepository,
	ProvideArtifactRepository,
	ProvideScoreRepository,
	ProvideCatalogRepository,
	ProvideMonitorRepository,
	ProvideAgentConfigRepository,
	ProvideCustomEvaluatorRepository,
	ProvideAPIKeyRepository,
	repositories.NewAgentConfigurationRepository,
	repositories.NewEnvAgentModelMappingRepository,
	repositories.NewEnvAgentMCPMappingRepository,
	repositories.NewAgentEnvConfigVariableRepository,
	repositories.NewMonitorLLMMappingRepository,
	ProvideOrgPublisherCredentialRepository,
	ProvideOrgSchedulerCredentialRepository,
	ProvideAIApplicationRepository,
	ProvideAgentThunderClientRepository,
	ProvideEnvThunderSystemClientRepository,
	ProvideEnvThunderURLRepository,
	repositories.NewMCPProxyScopeRepository,
)

var websocketProviderSet = wire.NewSet(
	ProvideEventHub,
	ProvideWebSocketManager,
	ProvideGatewayConnectionChecker,
	services.NewGatewayEventsService,
	ProvideDeploymentAckHandler,
)

// ProvideGatewayConnectionChecker exposes the websocket manager through the
// narrow connectivity interface consumed by services.
func ProvideGatewayConnectionChecker(m *websocket.Manager) services.GatewayConnectionChecker {
	return m
}

// Test client providers
func ProvideTestOpenChoreoClient(testClients TestClients) occlient.OpenChoreoClient {
	return testClients.OpenChoreoClient
}

func ProvideTestObserverClient(testClients TestClients) observersvc.ObserverSvcClient {
	return testClients.ObserverSvcClient
}

func ProvideTestSecretManagementClient(testClients TestClients) secretmanagersvc.SecretManagementClient {
	return testClients.SecretMgmtClient
}

// ProvideEventHub creates and initializes the EventHub backed by PostgreSQL.
func ProvideEventHub(db *gorm.DB, logger *slog.Logger) (eventhub.EventHub, error) {
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	cfg := eventhub.DefaultSQLBackendConfig()
	hub := eventhub.NewSQLBackend(sqlDB, logger, cfg)
	if err := hub.Initialize(); err != nil {
		return nil, err
	}
	return hub, nil
}

// ProvideWebSocketManager creates a new WebSocket manager with config
func ProvideWebSocketManager(cfg config.Config, hub eventhub.EventHub) *websocket.Manager {
	wsConfig := websocket.ManagerConfig{
		MaxConnections:    cfg.WebSocket.MaxConnections,
		HeartbeatInterval: 20 * time.Second,
		HeartbeatTimeout:  time.Duration(cfg.WebSocket.ConnectionTimeout) * time.Second,
	}
	return websocket.NewManager(wsConfig, hub)
}

// ProvideWebSocketController creates a new WebSocket controller with rate limiting
func ProvideWebSocketController(
	manager *websocket.Manager,
	hub eventhub.EventHub,
	gatewayService *services.PlatformGatewayService,
	ackHandler *services.DeploymentAckHandler,
	cfg config.Config,
) controllers.WebSocketController {
	rateLimitCount := cfg.WebSocket.RateLimitPerMin
	return controllers.NewWebSocketController(manager, hub, gatewayService, ackHandler, rateLimitCount)
}

// ProvideDeploymentAckHandler creates a new deployment ack handler
func ProvideDeploymentAckHandler(deploymentRepo repositories.DeploymentRepository) *services.DeploymentAckHandler {
	return services.NewDeploymentAckHandler(deploymentRepo)
}

func ProvideGatewayRepository(db *gorm.DB) repositories.GatewayRepository {
	return repositories.NewGatewayRepo(db)
}

func ProvideLLMProviderTemplateRepository(db *gorm.DB) repositories.LLMProviderTemplateRepository {
	return repositories.NewLLMProviderTemplateRepo(db)
}

func ProvideLLMProviderRepository(db *gorm.DB) repositories.LLMProviderRepository {
	return repositories.NewLLMProviderRepo(db)
}

func ProvideLLMProxyRepository(db *gorm.DB) repositories.LLMProxyRepository {
	return repositories.NewLLMProxyRepo(db)
}

func ProvideMCPProxyRepository(db *gorm.DB) repositories.MCPProxyRepository {
	return repositories.NewMCPProxyRepo(db)
}

func ProvideDeploymentRepository(db *gorm.DB) repositories.DeploymentRepository {
	return repositories.NewDeploymentRepo(db)
}

func ProvideArtifactRepository(db *gorm.DB) repositories.ArtifactRepository {
	return repositories.NewArtifactRepo(db)
}

func ProvideAPIKeyRepository(db *gorm.DB) repositories.APIKeyRepository {
	return repositories.NewAPIKeyRepo(db)
}

func ProvideScoreRepository(db *gorm.DB) repositories.ScoreRepository {
	return repositories.NewScoreRepo(db)
}

func ProvideCatalogRepository(db *gorm.DB) repositories.CatalogRepository {
	return repositories.NewCatalogRepo(db)
}

func ProvideMonitorRepository(db *gorm.DB) repositories.MonitorRepository {
	return repositories.NewMonitorRepo(db)
}

func ProvideAgentConfigRepository(db *gorm.DB) repositories.AgentConfigRepository {
	return repositories.NewAgentConfigRepo(db)
}

func ProvideCustomEvaluatorRepository(db *gorm.DB) repositories.CustomEvaluatorRepository {
	return repositories.NewCustomEvaluatorRepo(db)
}

func ProvideOrgPublisherCredentialRepository(db *gorm.DB) repositories.OrgPublisherCredentialRepository {
	return repositories.NewOrgPublisherCredentialRepo(db)
}

func ProvideOrgSchedulerCredentialRepository(db *gorm.DB) repositories.OrgSchedulerCredentialRepository {
	return repositories.NewOrgSchedulerCredentialRepo(db)
}

func ProvideAgentKindRepository(db *gorm.DB) repositories.AgentKindRepository {
	return repositories.NewAgentKindRepo(db)
}

func ProvideAIApplicationRepository(db *gorm.DB) repositories.AIApplicationRepository {
	return repositories.NewAIApplicationRepository(db)
}

func ProvideAgentThunderClientRepository(db *gorm.DB) repositories.AgentThunderClientRepository {
	return repositories.NewAgentThunderClientRepo(db)
}

// ProvideEnvThunderSystemClientRepository provides the repository for
// per-environment env-Thunder system-client credentials.
func ProvideEnvThunderSystemClientRepository(db *gorm.DB) repositories.EnvThunderSystemClientRepository {
	return repositories.NewEnvThunderSystemClientRepo(db)
}

// ProvideEnvThunderURLRepository provides the repository for per-environment
// env-Thunder URL handles.
func ProvideEnvThunderURLRepository(db *gorm.DB) repositories.EnvThunderURLRepository {
	return repositories.NewEnvThunderURLRepo(db)
}

// ProvideEnvThunderSecretReader decrypts the env-Thunder system-client
// credential from AMS's own Postgres — no key-vault read-back.
func ProvideEnvThunderSecretReader(repo repositories.EnvThunderSystemClientRepository, encryptionKey []byte) thundersvc.ReadSystemClientFunc {
	return services.NewEnvThunderSecretReader(repo, encryptionKey)
}

// ProvideEnvThunderURLReader looks up an env-Thunder's registered origin from
// AMS's own Postgres. A missing row means not provisioned — there is no
// fallback to a value computed from (org, env).
func ProvideEnvThunderURLReader(repo repositories.EnvThunderURLRepository) thundersvc.ReadThunderURLFunc {
	return services.NewEnvThunderURLReader(repo)
}

// ProvideEnvThunderResolver maps (org, environment) to an authenticated
// ThunderClient, reading the system-client credential and registered URL via
// the injected readers.
func ProvideEnvThunderResolver(readSystemClient thundersvc.ReadSystemClientFunc, readThunderURL thundersvc.ReadThunderURLFunc) thundersvc.EnvThunderResolver {
	return thundersvc.NewEnvThunderResolver(readSystemClient, readThunderURL)
}

// ProvideAgentIdentityInjectionService creates the Gateway Binding service that
// delivers AgentID credentials into internal agents' workloads. It reuses the
// secret manager's SecretReference refresh interval so AgentID Secrets follow
// the same re-sync cadence as every other injected secret.
func ProvideAgentIdentityInjectionService(
	repo repositories.AgentThunderClientRepository,
	agentConfigRepo repositories.AgentConfigurationRepository,
	mcpProxyScopeRepo repositories.MCPProxyScopeRepository,
	ocClient occlient.OpenChoreoClient,
	cfg config.Config,
	logger *slog.Logger,
) services.AgentIdentityInjectionService {
	return services.NewAgentIdentityInjectionService(repo, agentConfigRepo, mcpProxyScopeRepo, ocClient, cfg.SecretManager.AgentIdentityRefreshInterval, logger)
}

func ProvideThunderConfig(cfg config.Config) config.ThunderConfig {
	return cfg.Thunder
}

// ProvideIdentityClient creates a Thunder identity client using the Thunder system app credentials.
func ProvideIdentityClient(cfg config.ThunderConfig) thundersvc.IdentityClient {
	if cfg.ResolveToHost != "" {
		return thundersvc.NewIdentityClientWithDialOverride(cfg.BaseURL, cfg.ClientID, cfg.ClientSecret, cfg.ResolveToHost)
	}
	return thundersvc.NewIdentityClient(cfg.BaseURL, cfg.ClientID, cfg.ClientSecret)
}

// ProvideOrgResolver creates the org resolver backed by Thunder, with a per-org OU ID cache.
func ProvideOrgResolver(client thundersvc.IdentityClient) middleware.OrgResolver {
	return middleware.NewOrgResolver(client)
}

// InitializeAppParams wires up all application dependencies. agentThunderProvisioning
// is the deployment-injected AgentID provisioning implementation (nil to disable).
func InitializeAppParams(cfg *config.Config, db *gorm.DB, authProvider occlient.AuthProvider, secretProvider secretmanagersvc.Provider, gatewayApplier services.GatewayConfigApplier, agentThunderProvisioning services.AgentThunderProvisioningService, buildSecretProvisioner services.BuildSecretProvisioner) (*AppParams, error) {
	wire.Build(
		configProviderSet,
		clientProviderSet,
		loggerProviderSet,
		repositoryProviderSet,
		websocketProviderSet,
		serviceProviderSet,
		instrumentationProviderSet,
		controllerProviderSet,
		ProvideAuthMiddleware,
		ProvideJWTSigningConfig,
		wire.Struct(new(AppParams), "*"),
	)
	return &AppParams{}, nil
}

// InitializeTestAppParamsWithClientMocks wires up application dependencies with test mocks
func InitializeTestAppParamsWithClientMocks(
	cfg *config.Config,
	db *gorm.DB,
	authMiddleware jwtassertion.Middleware,
	testClients TestClients,
	gatewayApplier services.GatewayConfigApplier,
) (*AppParams, error) {
	wire.Build(
		testClientProviderSet,
		thunderProvisioningTestSet,
		loggerProviderSet,
		repositoryProviderSet,
		websocketProviderSet,
		serviceProviderSet,
		instrumentationProviderSet,
		controllerProviderSet,
		configProviderSet,
		ProvideJWTSigningConfig,
		ProvideNilBuildSecretProvisioner,
		wire.Struct(new(AppParams), "*"),
	)
	return &AppParams{}, nil
}
