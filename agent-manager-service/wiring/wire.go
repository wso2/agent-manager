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
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/wire"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	observersvc "github.com/wso2/agent-manager/agent-manager-service/clients/observersvc"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc/providers/inhouse"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/instrumentation"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations/notify"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations/webhook"
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
	ProvideSecretResolver,
	ProvidePublisherProvisioner,
	ProvideBuildTokenIssuer,
	ProvideIdentityClient,
	ProvideOrgResolver,
	ProvideOUHandleLookup,
	thundersvc.NewProber,
	// EnvThunderResolver is wired for AgentIdentityController's passthrough
	// endpoints. AgentID provisioning itself is injected via
	// app.Options.AgentThunderProvisioning, built with its own reader — see app.Run.
	ProvideEnvThunderSecretReader,
	ProvideEnvThunderURLReader,
	ProvideEnvThunderResolverWithEndpoints,
)

var serviceProviderSet = wire.NewSet(
	services.NewAgentManagerService,
	services.NewAgentKindService,
	services.NewInfraResourceManager,
	services.NewAgentTokenManagerService,
	ProvideGitSecretStore,
	services.NewGitCredentialsService,
	services.NewRepositoryService,
	services.NewMonitorExecutor,
	services.NewMonitorManagerService,
	ProvideThunderConfig,
	services.NewMonitorSchedulerService,
	// Provisioning service is injected (see InitializeAppParams); only the
	// reconciler that consumes it is wired here.
	ProvideAgentIdentityInjectionServiceWithEndpointResolver,
	services.NewAgentThunderReconcilerService,
	services.NewA2ACardFetcher,
	services.NewA2ACardReconcilerService,
	services.NewConfiguredDeploymentTimeoutSweeper,
	services.NewA2AAgentCardService,
	ProvideTxRunner,
	services.NewAgentExposurePublisher,
	services.NewA2APublisher,
	services.NewA2AProxyService,
	ProvideA2AProxyHooks,
	services.NewEvaluatorManagerService,
	services.NewEnvironmentService,
	ProvidePlatformGatewayService,
	services.NewLLMProviderTemplateService,
	services.NewLLMProviderService,
	services.NewLLMProxyService,
	services.NewLLMProviderDeploymentService,
	services.NewLLMProviderAPIKeyService,
	services.NewLLMProxyAPIKeyService,
	services.NewAgentAPIKeyService,
	services.NewLLMProxyDeploymentService,
	services.NewMCPProxyService,
	services.NewProjectMCPProxyService,
	services.NewMCPProxyScopeService,
	// MCPProxyScopeService only needs the narrow redeploy surface; bind it to
	// the concrete service so scope mutations can re-emit gateway policies.
	wire.Bind(new(services.MCPProxyRedeployer), new(*services.MCPProxyService)),
	// The agent-identity controller needs only the identifier-derivation surface.
	wire.Bind(new(controllers.MCPResourceServerIdentifierResolver), new(*services.MCPProxyService)),
	services.NewGatewayInternalAPIService,
	services.NewMonitorScoresService,
	services.NewBuildPublisherService,
	services.NewCatalogService,
	services.NewLLMProxyProvisioner,
	services.NewAgentConfigurationService,
	services.NewLLMTemplateStore,
	services.NewGitSecretService,
	services.NewAIApplicationService,
	ProvideEventBus,
	ProvideEventPublisher,
	repositories.NewEventOutboxRepository,
	services.NewMonitorRunEvents,
	ProvideDestinationDrivers,
	services.NewEventDestinationService,
	services.NewEventSubscriptionService,
	wire.Bind(new(services.EventsCleaner), new(services.EventSubscriptionService)),
	wire.Bind(new(services.DestinationResolver), new(services.EventDestinationService)),
	services.NewEventDispatcherService,
	services.NewMonitorAlertService,
	services.NewMonitorAlertProcessor,
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
	controllers.NewA2AAgentCardController,
	controllers.NewA2AProxyController,
	controllers.NewLLMProxyDeploymentController,
	controllers.NewMCPProxyController,
	controllers.NewProjectMCPProxyController,
	ProvideWebSocketController,
	controllers.NewGatewayInternalController,
	controllers.NewMonitorController,
	controllers.NewMonitorScoresController,
	controllers.NewMonitorScoresPublisherController,
	controllers.NewBuildPublisherController,
	controllers.NewEvaluatorController,
	controllers.NewCatalogController,
	ProvideAgentBuildOptionsController,
	controllers.NewAgentConfigurationController,
	controllers.NewGitSecretController,
	controllers.NewIdentityController,
	controllers.NewMCPProxyScopeController,
	controllers.NewAgentIdentityController,
	controllers.NewEventsController,
	controllers.NewMonitorAlertController,
)

var testClientProviderSet = wire.NewSet(
	ProvideTestOpenChoreoClient,
	ProvideTestObserverClient,
	ProvideTestSecretManagementClient,
	ProvideTestSecretResolver,
	ProvidePublisherProvisioner,
	ProvideBuildTokenIssuer,
	ProvideIdentityClient,
	ProvideOrgResolver,
	ProvideTestOUHandleLookup,
	thundersvc.NewProber,
	ProvideEnvThunderSecretReader,
	ProvideEnvThunderURLReader,
	ProvideEnvThunderResolver,
)

// ProvideTestSecretResolver provides a resolver that finds no secret, for the
// test wiring, which has no in-house store behind it.
func ProvideTestSecretResolver() secretmanagersvc.SecretResolver {
	return noSecretResolver{}
}

type noSecretResolver struct{}

func (noSecretResolver) Resolve(context.Context, string, string, string) (string, error) {
	return "", secretmanagersvc.ErrSecretNotFound
}

// thunderProvisioningTestSet builds the secret-store-backed provisioning service for
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
		ResourceLabels:   cfg.OpenChoreo.ResourceLabels,
	})
}

func ProvideObserverClient(cfg config.Config, authProvider occlient.AuthProvider) (observersvc.ObserverSvcClient, error) {
	return observersvc.NewObserverClient(&observersvc.Config{
		BaseURL:      cfg.Observer.URL,
		AuthProvider: authProvider,
	})
}

// ProvideSecretManagementClient creates the secret management service client
// backed by the in-house secret store.
func ProvideSecretManagementClient(cfg config.Config, secretProvider secretmanagersvc.Provider, db *gorm.DB) (secretmanagersvc.SecretManagementClient, error) {
	storeConfig, err := inHouseStoreConfig(cfg, db)
	if err != nil {
		return nil, err
	}
	return secretmanagersvc.NewSecretManagementClientWithConfig(secretmanagersvc.SecretManagementClientConfig{
		StoreConfig: storeConfig,
		Provider:    secretProvider,
	})
}

// ProvideSecretResolver creates the resolver the internal secret store endpoint
// reads plaintext values through.
func ProvideSecretResolver(cfg config.Config, db *gorm.DB) (secretmanagersvc.SecretResolver, error) {
	storeConfig, err := inHouseStoreConfig(cfg, db)
	if err != nil {
		return nil, err
	}
	resolver, err := inhouse.NewClient(storeConfig.InHouse)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := resolver.SeedPlatformSecrets(ctx, cfg.SecretManager.PlatformVaultSeed); err != nil {
		return nil, err
	}
	return resolver, nil
}

func inHouseStoreConfig(cfg config.Config, db *gorm.DB) (*secretmanagersvc.StoreConfig, error) {
	key, err := ProvideEncryptionKey(cfg)
	if err != nil {
		return nil, err
	}
	return &secretmanagersvc.StoreConfig{
		Provider: inhouse.ProviderName,
		InHouse: &secretmanagersvc.InHouseConfig{
			DB:             db,
			EncryptionKey:  key,
			MaxSecretBytes: inHouseMaxSecretBytes(cfg),
		},
	}, nil
}

// inHouseMaxSecretBytes sizes one stored secret. An agent's env secrets and
// sensitive file mounts for one environment are stored together, so a secret
// must hold the whole file mount budget the API accepts, plus room for the env
// secrets beside it.
func inHouseMaxSecretBytes(cfg config.Config) int {
	return cfg.FileMountLimits.MaxTotalBytes + inhouse.DefaultMaxSecretBytes
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

// ProvideGitSecretStore creates the tenant-scoped Git credential store.
func ProvideGitSecretStore(cfg config.Config, db *gorm.DB) (services.GitSecretStore, error) {
	storeConfig, err := inHouseStoreConfig(cfg, db)
	if err != nil {
		return nil, err
	}
	return inhouse.NewClient(storeConfig.InHouse)
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

// ProvideBuildTokenIssuer creates the organization-scoped build token issuer, or
// nil when build tokens are disabled (see config.BuildClientConfig).
func ProvideBuildTokenIssuer(cfg config.Config, logger *slog.Logger, secretClient secretmanagersvc.SecretManagementClient) services.BuildTokenIssuer {
	return services.NewBuildTokenIssuer(cfg, logger, secretClient)
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
	ProvidePlatformAgentExposureRepository,
	ProvideA2AAgentCardRepository,
	ProvideA2AProxyRepository,
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
	repositories.NewEventRepository,
	repositories.NewMonitorAlertRepository,
)

var reconcileProviderSet = wire.NewSet(
	services.NewLLMProviderKind,
	services.NewLLMProxyKind,
	services.NewMCPProxyKind,
	services.NewPlatformAgentExposures,
	services.NewA2AAgentKind,
	services.NewRestAgentKind,
	ProvideReconcileKinds,
	ProvideReconcileCore,
	wire.Bind(new(services.GatewayArtifacts), new(*reconcile.Core)),
	reconcile.NewWorker,
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
	hub, err := eventhub.NewSQLBackend(db, logger, eventhub.DefaultSQLBackendConfig())
	if err != nil {
		return nil, err
	}
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

// ProvideDeploymentAckHandler creates a new deployment ack handler that runs the reconcile kinds' after-live hooks.
func ProvideDeploymentAckHandler(deploymentRepo repositories.DeploymentRepository, core *reconcile.Core) *services.DeploymentAckHandler {
	return services.NewDeploymentAckHandler(deploymentRepo, core.AfterLive())
}

// ProvideReconcileKinds lists the gateway artifact kinds the reconcile core deploys.
func ProvideReconcileKinds(
	llmProvider *services.LLMProviderKind, llmProxy *services.LLMProxyKind, mcp *services.MCPProxyKind,
	agent *services.A2AAgentKind, restAgent *services.RestAgentKind,
) reconcile.Kinds {
	return reconcile.Kinds{llmProvider, llmProxy, mcp, agent, restAgent}
}

// ProvideReconcileCore builds the reconcile core with the worker's intervals, lease and attempt budget from cfg, and
// the gateways it locks and checks contexts on.
func ProvideReconcileCore(
	db *gorm.DB, deploymentRepo repositories.DeploymentRepository, gatewayRepo repositories.GatewayRepository,
	hub eventhub.EventHub, kinds reconcile.Kinds, cfg *config.Config, logger *slog.Logger,
) *reconcile.Core {
	seconds := func(n int) time.Duration { return time.Duration(n) * time.Second }
	return reconcile.NewCore(db, deploymentRepo, hub, kinds, reconcile.Config{
		RenderInterval: seconds(cfg.Reconcile.RenderIntervalSeconds),
		PlaceInterval:  seconds(cfg.Reconcile.PlaceIntervalSeconds),
		DriftInterval:  seconds(cfg.Reconcile.DriftIntervalSeconds),
		Lease:          seconds(cfg.Reconcile.RenderLeaseSeconds),
		AttemptBudget:  cfg.Reconcile.RenderAttemptBudget,
	}, logger).WithGateways(gatewayRepo)
}

// ProvidePlatformGatewayService makes each new gateway-environment mapping place the environment's artifacts, and
// refuses to remove a mapping while the gateway serves the environment's artifacts.
func ProvidePlatformGatewayService(
	gatewayRepo repositories.GatewayRepository, gatewayApplier services.GatewayConfigApplier, worker *reconcile.Worker,
	core *reconcile.Core,
) *services.PlatformGatewayService {
	return services.NewPlatformGatewayService(gatewayRepo, gatewayApplier).WithEnvironmentPlacer(worker).
		WithEnvironmentGuard(core)
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

func ProvideA2AAgentCardRepository(db *gorm.DB) repositories.A2AAgentCardRepository {
	return repositories.NewA2AAgentCardRepository(db)
}

func ProvideA2AProxyRepository(db *gorm.DB) repositories.A2AProxyRepository {
	return repositories.NewA2AProxyRepository(db)
}

// ProvideA2AProxyHooks gives the card service the proxy hooks only, not the whole proxy service.
func ProvideA2AProxyHooks(svc services.A2AProxyService) services.A2AProxyHooks {
	return svc
}

// ProvideTxRunner opens database transactions for the services that hold no *gorm.DB.
func ProvideTxRunner(db *gorm.DB) services.TxRunner {
	return services.NewTxRunner(db)
}

func ProvidePlatformAgentExposureRepository(db *gorm.DB) repositories.PlatformAgentExposureRepository {
	return repositories.NewPlatformAgentExposureRepository(db)
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

func ProvideEnvThunderResolverWithEndpoints(readSystemClient thundersvc.ReadSystemClientFunc, readThunderURL thundersvc.ReadThunderURLFunc, endpointResolver thundersvc.EnvThunderEndpointResolver) thundersvc.EnvThunderResolver {
	return thundersvc.NewEnvThunderResolverWithEndpoints(readSystemClient, readThunderURL, endpointResolver)
}

// ProvideNilEnvThunderEndpointResolver supplies the nil endpoint resolver that
// on-premises deployments run with (app.Options leaves it unset), so wiring paths
// that do not inject one (e.g. tests) exercise the same identity-injection path
// as production: the in-cluster token endpoint, with no env-Thunder URL lookup.
func ProvideNilEnvThunderEndpointResolver() thundersvc.EnvThunderEndpointResolver {
	return nil
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
	return services.NewAgentIdentityInjectionService(repo, agentConfigRepo, mcpProxyScopeRepo, ocClient, cfg.SecretManager.AgentIdentityRefreshInterval, logger, services.WithAgentIdentityForceSyncWait(cfg.SecretManager.AgentIdentityForceSyncWait), services.WithAgentIdentityForceSyncApplyTimeout(cfg.SecretManager.AgentIdentityForceSyncApplyTimeout))
}

func ProvideAgentIdentityInjectionServiceWithEndpointResolver(
	repo repositories.AgentThunderClientRepository,
	agentConfigRepo repositories.AgentConfigurationRepository,
	mcpProxyScopeRepo repositories.MCPProxyScopeRepository,
	ocClient occlient.OpenChoreoClient,
	readThunderURL thundersvc.ReadThunderURLFunc,
	endpointResolver thundersvc.EnvThunderEndpointResolver,
	cfg config.Config,
	logger *slog.Logger,
) services.AgentIdentityInjectionService {
	if endpointResolver == nil {
		return services.NewAgentIdentityInjectionService(repo, agentConfigRepo, mcpProxyScopeRepo, ocClient, cfg.SecretManager.AgentIdentityRefreshInterval, logger, services.WithAgentIdentityForceSyncWait(cfg.SecretManager.AgentIdentityForceSyncWait), services.WithAgentIdentityForceSyncApplyTimeout(cfg.SecretManager.AgentIdentityForceSyncApplyTimeout))
	}
	resolveTokenEndpoint := func(ctx context.Context, ouID, orgNamespace, envName string) (string, error) {
		return thundersvc.ResolveEnvThunderTokenEndpoint(ctx, endpointResolver, readThunderURL, ouID, orgNamespace, envName)
	}
	return services.NewAgentIdentityInjectionServiceWithTokenEndpointResolver(repo, agentConfigRepo, mcpProxyScopeRepo, ocClient, cfg.SecretManager.AgentIdentityRefreshInterval, logger, resolveTokenEndpoint, services.WithAgentIdentityForceSyncWait(cfg.SecretManager.AgentIdentityForceSyncWait), services.WithAgentIdentityForceSyncApplyTimeout(cfg.SecretManager.AgentIdentityForceSyncApplyTimeout))
}

// ProvideDestinationDrivers registers the driver of each supported event
// destination kind. Add a kind here once its driver exists.
func ProvideDestinationDrivers(cfg config.Config) *eventdestinations.Registry {
	return eventdestinations.NewRegistry(
		webhook.New(cfg.Events.AllowPrivateDestinations),
		notify.New(cfg.Events.AllowPrivateDestinations),
	)
}

// ProvideEventBus connects to NATS, or starts the embedded in-memory server,
// as EVENTS_MODE says. With events off it returns nil: nothing is published.
func ProvideEventBus(cfg config.Config, logger *slog.Logger) (events.Bus, error) {
	switch cfg.Events.Mode {
	case config.EventsModeEmbedded:
		bus, err := events.StartEmbedded(logger)
		if err != nil {
			return nil, err
		}
		return bus, nil
	case config.EventsModeNATS:
		bus, err := events.Connect(cfg.Events.NATSURL, cfg.Events.NATSToken, "agent-manager-service", logger)
		if err != nil {
			return nil, err
		}
		return bus, nil
	default:
		// A nil interface, not a nil *NATSBus, so callers' nil checks work.
		logger.Info("Events disabled: EVENTS_MODE is off")
		return nil, nil
	}
}

// ProvideEventPublisher returns the publisher routes and the monitor
// scheduler use. Without a bus, events are discarded.
func ProvideEventPublisher(bus events.Bus, outbox events.Outbox, logger *slog.Logger) events.Publisher {
	if bus == nil {
		return events.NoopPublisher{}
	}
	return events.NewAsyncPublisher(bus, outbox, logger)
}

func ProvideThunderConfig(cfg config.Config) config.ThunderConfig {
	return cfg.Thunder
}

// ProvideIdentityClient creates a Thunder identity client using the Thunder system app credentials.
func ProvideIdentityClient(cfg config.ThunderConfig) thundersvc.IdentityClient {
	return thundersvc.NewIdentityClientWithSystemResource(
		cfg.BaseURL,
		cfg.ClientID,
		cfg.ClientSecret,
		cfg.ResolveToHost,
		cfg.ResolvedSystemResourceIdentifier(),
	)
}

// ProvideOUHandleLookup maps an OU ID to its handle through the platform
// Thunder, for the org override on the project MCP routes. It is deliberately
// uncached: a handle can be renamed, and the lookup runs only on impersonated
// requests. A missing OU is reported as middleware.ErrOverrideOrgNotFound.
func ProvideOUHandleLookup(client thundersvc.IdentityClient) middleware.OUHandleLookup {
	return func(ctx context.Context, ouID string) (string, error) {
		handle, err := client.GetOUHandleByID(ctx, ouID)
		if thundersvc.IsNotFound(err) {
			return "", fmt.Errorf("%w: %w", middleware.ErrOverrideOrgNotFound, err)
		}
		return handle, err
	}
}

// ProvideTestOUHandleLookup uses the test's lookup when it supplies one, and
// the Thunder-backed lookup otherwise.
func ProvideTestOUHandleLookup(testClients TestClients, client thundersvc.IdentityClient) middleware.OUHandleLookup {
	if testClients.OUHandleLookup != nil {
		return testClients.OUHandleLookup
	}
	return ProvideOUHandleLookup(client)
}

// ProvideOrgResolver creates the org resolver backed by Thunder, with a per-org OU ID cache.
func ProvideOrgResolver(client thundersvc.IdentityClient) middleware.OrgResolver {
	return middleware.NewOrgResolver(client)
}

// InitializeAppParams preserves the standard on-premises dependency graph.
func InitializeAppParams(cfg *config.Config, db *gorm.DB, authProvider occlient.AuthProvider, secretProvider secretmanagersvc.Provider, gatewayApplier services.GatewayConfigApplier, agentThunderProvisioning services.AgentThunderProvisioningService, buildSecretProvisioner services.BuildSecretProvisioner) (*AppParams, error) {
	return InitializeAppParamsWithEnvThunderEndpoints(cfg, db, authProvider, secretProvider, gatewayApplier, agentThunderProvisioning, buildSecretProvisioner, nil)
}

// InitializeAppParamsWithEnvThunderEndpoints wires dependencies with an
// optional deployment-specific environment-Thunder endpoint resolver.
func InitializeAppParamsWithEnvThunderEndpoints(cfg *config.Config, db *gorm.DB, authProvider occlient.AuthProvider, secretProvider secretmanagersvc.Provider, gatewayApplier services.GatewayConfigApplier, agentThunderProvisioning services.AgentThunderProvisioningService, buildSecretProvisioner services.BuildSecretProvisioner, endpointResolver thundersvc.EnvThunderEndpointResolver) (*AppParams, error) {
	wire.Build(
		configProviderSet,
		clientProviderSet,
		loggerProviderSet,
		repositoryProviderSet,
		websocketProviderSet,
		reconcileProviderSet,
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
		reconcileProviderSet,
		serviceProviderSet,
		instrumentationProviderSet,
		controllerProviderSet,
		configProviderSet,
		ProvideJWTSigningConfig,
		ProvideNilBuildSecretProvisioner,
		ProvideNilEnvThunderEndpointResolver,
		wire.Struct(new(AppParams), "*"),
	)
	return &AppParams{}, nil
}
