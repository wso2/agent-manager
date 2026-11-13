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
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations/notify"
	"github.com/wso2/agent-manager/agent-manager-service/services/eventdestinations/webhook"
	"github.com/wso2/agent-manager/agent-manager-service/websocket"
)

// Provider sets
var configProviderSet = wire.NewSet(
	ProvideConfigFromPtr,
	ProvideEncryptionKey,
)

var clientProviderSet = wire.NewSet(
	repositories.NewPlatformRepo,
	ProvideSecretManagementClient,
	ProvidePublisherProvisioner,
	ProvideIdentityClient,
	ProvideOrgResolver,
	ProvideOUHandleLookup,
	thundersvc.NewProber,
	// EnvThunderResolver is wired for AgentIdentityController's passthrough
	// endpoints. AgentID provisioning itself is injected via
	// app.Options.AgentThunderProvisioning, built with its own reader — see app.Run.
	services.NewPlatformThunderSecretReader,
	services.NewPlatformThunderURLReader,
	ProvideEnvThunderResolverWithEndpoints,
)

var serviceProviderSet = wire.NewSet(
	services.NewAgentManagerService,
	services.NewInfraResourceManager,
	services.NewAgentTokenManagerService,
	services.NewMonitorExecutor,
	services.NewMonitorManagerService,
	ProvideThunderConfig,
	services.NewMonitorSchedulerService,
	// Provisioning service is injected (see InitializeAppParams); only the
	// reconciler that consumes it is wired here.
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

var controllerProviderSet = wire.NewSet(
	controllers.NewAgentController,
	controllers.NewInfraResourceController,
	controllers.NewAgentTokenController,
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
	ProvideWebSocketController,
	controllers.NewGatewayInternalController,
	controllers.NewMonitorController,
	controllers.NewMonitorScoresController,
	controllers.NewMonitorScoresPublisherController,
	controllers.NewEvaluatorController,
	controllers.NewCatalogController,
	controllers.NewAgentConfigurationController,
	controllers.NewIdentityController,
	controllers.NewMCPProxyScopeController,
	controllers.NewAgentIdentityController,
	controllers.NewEventsController,
	controllers.NewMonitorAlertController,
)

var testClientProviderSet = wire.NewSet(
	ProvideTestPlatformRepository,
	ProvideTestSecretManagementClient,
	ProvidePublisherProvisioner,
	ProvideIdentityClient,
	ProvideOrgResolver,
	ProvideTestOUHandleLookup,
	thundersvc.NewProber,
	services.NewPlatformThunderSecretReader,
	services.NewPlatformThunderURLReader,
	ProvideEnvThunderResolver,
)

// thunderProvisioningTestSet builds the provisioning service for
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

// ProvideSecretManagementClient creates the secret management service client.
// If the provider implements secretmanagersvc.SecretReferenceManager and
// reports that it manages SecretReferences itself, no SecretReference client is
// forwarded — preventing the high-level client from making redundant
// SecretReference CRUD calls.
func ProvideSecretManagementClient(cfg config.Config, secretProvider secretmanagersvc.Provider) (secretmanagersvc.SecretManagementClient, error) {
	return secretmanagersvc.NewSecretManagementClientWithConfig(secretmanagersvc.SecretManagementClientConfig{
		StoreConfig: &secretmanagersvc.StoreConfig{Provider: cfg.SecretManager.Provider},
		Provider:    secretProvider,
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

// ProvidePublisherProvisioner creates the publisher credential provisioner
// for per-org Thunder OAuth app creation and secret storage via SecretManagementClient
func ProvidePublisherProvisioner(cfg config.Config, encryptionKey []byte, logger *slog.Logger, secretClient secretmanagersvc.SecretManagementClient, credRepo repositories.OrgPublisherCredentialRepository, schedulerCredRepo repositories.OrgSchedulerCredentialRepository) (services.PublisherCredentialProvisioner, error) {
	return services.NewPublisherCredentialProvisioner(cfg, encryptionKey, logger, secretClient, credRepo, schedulerCredRepo)
}

var loggerProviderSet = wire.NewSet(
	ProvideLogger,
	ProvideAuditRecorder,
)

var repositoryProviderSet = wire.NewSet(
	ProvideGatewayRepository,
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
	ProvideOtelIngestKind,
	services.NewOtelIngestPublisher,
	ProvideDefaultGatewaySeeder,
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
func ProvideTestPlatformRepository(testClients TestClients) repositories.PlatformRepository {
	return testClients.PlatformRepository
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
	otelIngest *services.OtelIngestPublisher,
	cfg config.Config,
	logger *slog.Logger,
) controllers.WebSocketController {
	rateLimitCount := cfg.WebSocket.RateLimitPerMin
	// A connecting gateway gets the trace-ingest API if it does not serve the current one yet.
	onConnect := func(ctx context.Context, gateway *models.Gateway) {
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		if err := otelIngest.EnsureOnGateway(ctx, gateway.OUID, gateway.UUID); err != nil {
			logger.Error("Failed to deploy the trace-ingest API", "gatewayID", gateway.UUID, "error", err)
		}
	}
	return controllers.NewWebSocketController(manager, hub, gatewayService, ackHandler, rateLimitCount, onConnect)
}

// ProvideOtelIngestKind is the built-in trace-ingest API, accepting tokens Agent Manager signs.
func ProvideOtelIngestKind(cfg config.Config) *services.OtelIngestKind {
	return services.NewOtelIngestKind(cfg.OtelIngest.UpstreamURL, cfg.JWTSigning.Issuer)
}

// ProvideDefaultGatewaySeeder registers the gateway named in configuration.
func ProvideDefaultGatewaySeeder(
	cfg config.Config, gateways *services.PlatformGatewayService, repo repositories.GatewayRepository,
	platform repositories.PlatformRepository, logger *slog.Logger,
) *services.DefaultGatewaySeeder {
	return services.NewDefaultGatewaySeeder(cfg.DefaultGateway, gateways, repo, platform, logger)
}

// ProvideDeploymentAckHandler creates a new deployment ack handler that runs the reconcile kinds' after-live hooks.
func ProvideDeploymentAckHandler(deploymentRepo repositories.DeploymentRepository, core *reconcile.Core) *services.DeploymentAckHandler {
	return services.NewDeploymentAckHandler(deploymentRepo, core.AfterLive())
}

// ProvideReconcileKinds lists the gateway artifact kinds the reconcile core deploys.
func ProvideReconcileKinds(
	llmProvider *services.LLMProviderKind, llmProxy *services.LLMProxyKind, mcp *services.MCPProxyKind,
	agent *services.A2AAgentKind, otelIngest *services.OtelIngestKind,
) reconcile.Kinds {
	return reconcile.Kinds{llmProvider, llmProxy, mcp, agent, otelIngest}
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

func ProvideAIApplicationRepository(db *gorm.DB) repositories.AIApplicationRepository {
	return repositories.NewAIApplicationRepository(db)
}

func ProvideAgentThunderClientRepository(db *gorm.DB) repositories.AgentThunderClientRepository {
	return repositories.NewAgentThunderClientRepo(db)
}

// ProvideEnvThunderResolver maps the default environment to an authenticated
// client for the platform Thunder, read through the injected readers.
func ProvideEnvThunderResolver(readSystemClient thundersvc.ReadSystemClientFunc, readThunderURL thundersvc.ReadThunderURLFunc) thundersvc.EnvThunderResolver {
	return thundersvc.NewEnvThunderResolver(readSystemClient, readThunderURL)
}

func ProvideEnvThunderResolverWithEndpoints(readSystemClient thundersvc.ReadSystemClientFunc, readThunderURL thundersvc.ReadThunderURLFunc, endpointResolver thundersvc.EnvThunderEndpointResolver) thundersvc.EnvThunderResolver {
	return thundersvc.NewEnvThunderResolverWithEndpoints(readSystemClient, readThunderURL, endpointResolver)
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
func InitializeAppParams(cfg *config.Config, db *gorm.DB, secretProvider secretmanagersvc.Provider, gatewayApplier services.GatewayConfigApplier, agentThunderProvisioning services.AgentThunderProvisioningService) (*AppParams, error) {
	return InitializeAppParamsWithEnvThunderEndpoints(cfg, db, secretProvider, gatewayApplier, agentThunderProvisioning, nil)
}

// InitializeAppParamsWithEnvThunderEndpoints wires dependencies with an
// optional deployment-specific environment-Thunder endpoint resolver.
func InitializeAppParamsWithEnvThunderEndpoints(cfg *config.Config, db *gorm.DB, secretProvider secretmanagersvc.Provider, gatewayApplier services.GatewayConfigApplier, agentThunderProvisioning services.AgentThunderProvisioningService, endpointResolver thundersvc.EnvThunderEndpointResolver) (*AppParams, error) {
	wire.Build(
		configProviderSet,
		clientProviderSet,
		loggerProviderSet,
		repositoryProviderSet,
		websocketProviderSet,
		reconcileProviderSet,
		serviceProviderSet,
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
		controllerProviderSet,
		configProviderSet,
		ProvideJWTSigningConfig,
		wire.Struct(new(AppParams), "*"),
	)
	return &AppParams{}, nil
}
