//go:build wireinject

package gwwire

import (
	"log/slog"
	"time"

	"github.com/google/wire"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconfig"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsecret"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/websocket"
)

var repositoryProviderSet = wire.NewSet(
	ProvideGatewayRepository,
	ProvideLLMProviderTemplateRepository,
	ProvideLLMProviderRepository,
	ProvideLLMProxyRepository,
	ProvideMCPProxyRepository,
	gwrepo.NewMCPProxyEndpointRepository,
	ProvideDeploymentRepository,
	ProvidePlatformAgentExposureRepository,
	ProvideA2AAgentCardRepository,
	ProvideA2AProxyRepository,
	ProvideArtifactRepository,
	ProvideCatalogRepository,
	ProvideAPIKeyRepository,
	gwrepo.NewAgentConfigurationRepository,
	gwrepo.NewAgentConfigRepo,
	gwrepo.NewEnvAgentModelMappingRepository,
	gwrepo.NewEnvAgentMCPMappingRepository,
	gwrepo.NewAgentEnvConfigVariableRepository,
	ProvideAIApplicationRepository,
	gwrepo.NewMCPProxyScopeRepository,
	ProvideBudgetRepository,
	gwrepo.NewBudgetUsageRepository,
	gwrepo.NewCostUsageRepository,
	gwrepo.NewModelPriceRepository,
)

var serviceProviderSet = wire.NewSet(
	gwsvc.NewA2ACardFetcher,
	gwsvc.NewA2ACardReconcilerService,
	gwsvc.NewConfiguredDeploymentTimeoutSweeper,
	gwsvc.NewA2AAgentCardService,
	ProvideTxRunner,
	gwsvc.NewAgentExposurePublisher,
	gwsvc.NewA2APublisher,
	gwsvc.NewA2AProxyService,
	ProvideA2AProxyHooks,
	ProvidePlatformGatewayService,
	gwsvc.NewLLMProviderTemplateService,
	gwsvc.NewLLMProviderService,
	gwsvc.NewLLMProxyService,
	gwsvc.NewLLMProviderDeploymentService,
	gwsvc.NewLLMProviderAPIKeyService,
	gwsvc.NewLLMProxyAPIKeyService,
	gwsvc.NewAgentAPIKeyService,
	gwsvc.NewLLMProxyDeploymentService,
	gwsvc.NewMCPProxyService,
	gwsvc.NewIdentityDirectory,
	gwsvc.NewPermissions,
	gwsecret.NewStore,
	wire.Bind(new(identityv1.Directory), new(*gwsvc.IdentityDirectory)),
	gwsvc.NewNotifications,
	wire.Bind(new(notifyv1.Notifier), new(*gwsvc.Notifications)),
	wire.Bind(new(modkit.Secrets), new(*gwsecret.Store)),
	gwsvc.NewMCPProxyScopeService,
	// MCPProxyScopeService only needs the narrow redeploy surface; bind it to
	// the concrete service so scope mutations can re-emit gateway policies.
	wire.Bind(new(gwsvc.MCPProxyRedeployer), new(*gwsvc.MCPProxyService)),
	// The agent-identity controller needs only the identifier-derivation surface.
	wire.Bind(new(gwctl.MCPResourceServerIdentifierResolver), new(*gwsvc.MCPProxyService)),
	gwsvc.NewGatewayInternalAPIService,
	gwsvc.NewCatalogService,
	gwsvc.NewAgentConfigurationService,
	gwsvc.NewLLMTemplateStore,
	gwsvc.NewAIApplicationService,
	gwsvc.NewAgentGatewayService,
	gwsvc.NewEnvironmentGatewayService,
	gwsvc.NewGatewayResourceSyncService,
	gwsvc.NewBudgetService,
	gwsvc.NewBudgetGateways,
	wire.Bind(new(gwsvc.BudgetGatewaySender), new(*gwsvc.BudgetGateways)),
	gwsvc.NewBudgetWarningService,
	gwsvc.NewCostService,
	wire.Bind(new(gwsvc.CostRecorder), new(gwsvc.CostService)),
	gwsvc.NewPricingService,
)

var controllerProviderSet = wire.NewSet(
	gwctl.NewGatewayController,
	gwctl.NewLLMController,
	gwctl.NewLLMDeploymentController,
	gwctl.NewLLMProviderAPIKeyController,
	gwctl.NewLLMProxyAPIKeyController,
	gwctl.NewAgentAPIKeyController,
	gwctl.NewA2AAgentCardController,
	gwctl.NewA2AProxyController,
	gwctl.NewLLMProxyDeploymentController,
	gwctl.NewMCPProxyController,
	ProvideWebSocketController,
	gwctl.NewGatewayInternalController,
	gwctl.NewCatalogController,
	gwctl.NewAgentConfigurationController,
	gwctl.NewMCPProxyScopeController,
	gwctl.NewAgentIdentityController,
	gwctl.NewAgentGatewayController,
	gwctl.NewEnvironmentGatewayController,
	gwctl.NewBudgetController,
	gwctl.NewCostController,
	gwctl.NewPricingController,
)

// Initialize builds the gateway on the module's database handle and logger,
// reading core resources through coreAPI.
func Initialize(db *gorm.DB, logger *slog.Logger, coreAPI modkit.CoreAPI, settings gwconfig.Settings) (*Gateway, error) {
	wire.Build(
		repositoryProviderSet,
		websocketProviderSet,
		reconcileProviderSet,
		serviceProviderSet,
		controllerProviderSet,
		wire.Struct(new(Gateway), "*"),
	)
	return &Gateway{}, nil
}

// ProvideGatewayManifestCacheBackend builds the gateway-manifest cache backend the settings select (gwconfig.Load
// has refused any other). Not part of the wire provider sets: the cache is a package-level backend (see
// gwsvc.SetGatewayManifestCacheBackend), selected before the rest of the gateway is built.
func ProvideGatewayManifestCacheBackend(s gwconfig.Settings) gwsvc.GatewayManifestCacheBackend {
	if s.ManifestCache.Backend == "redis" {
		return gwsvc.NewRedisGatewayManifestCache(s.ManifestCache.Redis)
	}
	return gwsvc.NewInMemoryGatewayManifestCache()
}

var reconcileProviderSet = wire.NewSet(
	gwsvc.NewLLMProviderKind,
	gwsvc.NewLLMProxyKind,
	gwsvc.NewMCPProxyKind,
	gwsvc.NewPlatformAgentExposures,
	gwsvc.NewA2AAgentKind,
	ProvideDefaultGatewaySeeder,
	ProvideReconcileKinds,
	ProvideReconcileCore,
	wire.Bind(new(gwsvc.GatewayArtifacts), new(*reconcile.Core)),
	reconcile.NewWorker,
)

var websocketProviderSet = wire.NewSet(
	ProvideEventHub,
	ProvideWebSocketManager,
	ProvideGatewayConnectionChecker,
	gwsvc.NewGatewayEventsService,
	ProvideDeploymentAckHandler,
)

// ProvideGatewayConnectionChecker exposes the websocket manager through the
// narrow connectivity interface consumed by gwsvc.
func ProvideGatewayConnectionChecker(m *websocket.Manager) gwsvc.GatewayConnectionChecker {
	return m
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
func ProvideWebSocketManager(s gwconfig.Settings, hub eventhub.EventHub) *websocket.Manager {
	wsConfig := websocket.ManagerConfig{
		MaxConnections:    s.WebSocket.MaxConnections,
		HeartbeatInterval: 20 * time.Second,
		HeartbeatTimeout:  time.Duration(s.WebSocket.ConnectionTimeoutSeconds) * time.Second,
	}
	return websocket.NewManager(wsConfig, hub)
}

// ProvideWebSocketController creates a new WebSocket controller with rate limiting
func ProvideWebSocketController(
	manager *websocket.Manager,
	hub eventhub.EventHub,
	gatewayService *gwsvc.PlatformGatewayService,
	ackHandler *gwsvc.DeploymentAckHandler,
	s gwconfig.Settings,
) gwctl.WebSocketController {
	return gwctl.NewWebSocketController(manager, hub, gatewayService, ackHandler, s.WebSocket.RateLimitPerMin)
}

// ProvideDefaultGatewaySeeder registers the gateway named in configuration.
func ProvideDefaultGatewaySeeder(
	s gwconfig.Settings, gateways *gwsvc.PlatformGatewayService, repo gwrepo.GatewayRepository,
	platform modkit.CoreAPI, logger *slog.Logger,
) *gwsvc.DefaultGatewaySeeder {
	return gwsvc.NewDefaultGatewaySeeder(s.DefaultGateway, gateways, repo, platform, logger)
}

// ProvideDeploymentAckHandler creates a new deployment ack handler that runs the reconcile kinds' after-live hooks.
// Usage messages go to the budget warning service.
func ProvideDeploymentAckHandler(
	deploymentRepo gwrepo.DeploymentRepository, core *reconcile.Core, warnings *gwsvc.BudgetWarningService,
) *gwsvc.DeploymentAckHandler {
	return gwsvc.NewDeploymentAckHandler(deploymentRepo, core.AfterLive(), warnings)
}

// ProvideBudgetRepository stores budgets, with their owners' handles from the core.
func ProvideBudgetRepository(db *gorm.DB, core modkit.CoreAPI) gwrepo.BudgetRepository {
	return gwrepo.NewBudgetRepository(db, core)
}

// ProvideReconcileKinds lists the gateway artifact kinds the reconcile core deploys.
func ProvideReconcileKinds(
	llmProvider *gwsvc.LLMProviderKind, llmProxy *gwsvc.LLMProxyKind, mcp *gwsvc.MCPProxyKind,
	agent *gwsvc.A2AAgentKind,
) reconcile.Kinds {
	return reconcile.Kinds{llmProvider, llmProxy, mcp, agent}
}

// ProvideReconcileCore builds the reconcile core with the worker's intervals, lease and attempt budget from cfg, and
// the gateways it locks and checks contexts on.
func ProvideReconcileCore(
	db *gorm.DB, deploymentRepo gwrepo.DeploymentRepository, gatewayRepo gwrepo.GatewayRepository,
	hub eventhub.EventHub, kinds reconcile.Kinds, s gwconfig.Settings, logger *slog.Logger,
) *reconcile.Core {
	return reconcile.NewCore(db, deploymentRepo, hub, kinds, reconcile.Config{
		RenderInterval: s.Reconcile.RenderInterval,
		PlaceInterval:  s.Reconcile.PlaceInterval,
		DriftInterval:  s.Reconcile.DriftInterval,
		Lease:          s.Reconcile.RenderLease,
		AttemptBudget:  s.Reconcile.RenderAttemptBudget,
	}, logger).WithGateways(gatewayRepo)
}

// ProvidePlatformGatewayService makes each new gateway-environment mapping place the environment's artifacts, and
// refuses to remove a mapping while the gateway serves the environment's artifacts.
func ProvidePlatformGatewayService(
	gatewayRepo gwrepo.GatewayRepository, worker *reconcile.Worker, core *reconcile.Core, s gwconfig.Settings,
	events *gwsvc.GatewayEventsService,
) *gwsvc.PlatformGatewayService {
	return gwsvc.NewPlatformGatewayService(gatewayRepo).WithTokenEvents(events).WithEnvironmentPlacer(worker).
		WithEnvironmentGuard(core).WithFailureSettings(s.Failure)
}

func ProvideGatewayRepository(db *gorm.DB) gwrepo.GatewayRepository {
	return gwrepo.NewGatewayRepo(db)
}

func ProvideLLMProviderTemplateRepository(db *gorm.DB) gwrepo.LLMProviderTemplateRepository {
	return gwrepo.NewLLMProviderTemplateRepo(db)
}

func ProvideLLMProviderRepository(db *gorm.DB, secrets modkit.Secrets) gwrepo.LLMProviderRepository {
	return gwrepo.NewLLMProviderRepo(db, secrets)
}

func ProvideLLMProxyRepository(db *gorm.DB, secrets modkit.Secrets) gwrepo.LLMProxyRepository {
	return gwrepo.NewLLMProxyRepo(db, secrets)
}

func ProvideMCPProxyRepository(db *gorm.DB, core modkit.CoreAPI, secrets modkit.Secrets) gwrepo.MCPProxyRepository {
	return gwrepo.NewMCPProxyRepo(db, core, secrets)
}

func ProvideDeploymentRepository(db *gorm.DB) gwrepo.DeploymentRepository {
	return gwrepo.NewDeploymentRepo(db)
}

func ProvideA2AAgentCardRepository(db *gorm.DB) gwrepo.A2AAgentCardRepository {
	return gwrepo.NewA2AAgentCardRepository(db)
}

func ProvideA2AProxyRepository(db *gorm.DB, core modkit.CoreAPI, secrets modkit.Secrets) gwrepo.A2AProxyRepository {
	return gwrepo.NewA2AProxyRepository(db, core, secrets)
}

// ProvideA2AProxyHooks gives the card service the proxy hooks only, not the whole proxy service.
func ProvideA2AProxyHooks(svc gwsvc.A2AProxyService) gwsvc.A2AProxyHooks {
	return svc
}

// ProvideTxRunner opens database transactions for the services that hold no *gorm.DB.
func ProvideTxRunner(db *gorm.DB) gwsvc.TxRunner {
	return gwsvc.NewTxRunner(db)
}

func ProvidePlatformAgentExposureRepository(db *gorm.DB) gwrepo.PlatformAgentExposureRepository {
	return gwrepo.NewPlatformAgentExposureRepository(db)
}

func ProvideArtifactRepository(db *gorm.DB, secrets modkit.Secrets) gwrepo.ArtifactRepository {
	return gwrepo.NewArtifactRepo(db, secrets)
}

func ProvideAPIKeyRepository(db *gorm.DB) gwrepo.APIKeyRepository {
	return gwrepo.NewAPIKeyRepo(db)
}

func ProvideCatalogRepository(db *gorm.DB) gwrepo.CatalogRepository {
	return gwrepo.NewCatalogRepo(db)
}

func ProvideAIApplicationRepository(db *gorm.DB) gwrepo.AIApplicationRepository {
	return gwrepo.NewAIApplicationRepository(db)
}
