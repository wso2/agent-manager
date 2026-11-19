//go:build wireinject

package gwwire

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/wire"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/gateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/websocket"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// coreSet is what the gateway takes from the core.
var coreSet = wire.NewSet(
	wire.FieldsOf(
		new(*wiring.AppParams),
		"DB", "Config", "Logger", "PlatformRepository", "InfraResourceManager",
		"EncryptionKey", "SecretRefKeys", "SecretManagementClient", "SecretStore", "EnvThunderResolver",
		"AgentConfigRepository", "AgentThunderClientRepository",
	),
	wiring.ProvideConfigFromPtr,
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
	gwrepo.NewEnvAgentModelMappingRepository,
	gwrepo.NewEnvAgentMCPMappingRepository,
	gwrepo.NewAgentEnvConfigVariableRepository,
	ProvideAIApplicationRepository,
	gwrepo.NewMCPProxyScopeRepository,
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
	gwsvc.NewMCPProxyScopeService,
	// MCPProxyScopeService only needs the narrow redeploy surface; bind it to
	// the concrete service so scope mutations can re-emit gateway policies.
	wire.Bind(new(gwsvc.MCPProxyRedeployer), new(*gwsvc.MCPProxyService)),
	// The agent-identity controller needs only the identifier-derivation surface.
	wire.Bind(new(gwctl.MCPResourceServerIdentifierResolver), new(*gwsvc.MCPProxyService)),
	gwsvc.NewGatewayInternalAPIService,
	gwsvc.NewCatalogService,
	gwsvc.NewLLMProxyProvisioner,
	gwsvc.NewAgentConfigurationService,
	gwsvc.NewLLMTemplateStore,
	gwsvc.NewAIApplicationService,
	gwsvc.NewAgentGatewayService,
	gwsvc.NewEnvironmentGatewayService,
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
)

// Initialize builds the gateway on the core's services. gatewayApplier
// patches the gateway runtime config when an identity provider changes; nil
// keeps the script-driven behaviour.
func Initialize(core *wiring.AppParams, gatewayApplier gwsvc.GatewayConfigApplier) (*Gateway, error) {
	wire.Build(
		coreSet,
		repositoryProviderSet,
		websocketProviderSet,
		reconcileProviderSet,
		serviceProviderSet,
		controllerProviderSet,
		wire.Struct(new(Gateway), "*"),
	)
	return &Gateway{}, nil
}

// ProvideGatewayManifestCacheBackend builds the gateway-manifest cache backend
// selected by config.GatewayManifestCache.Backend. Not part of the wire provider
// sets: nothing constructs a dependency on it (the cache is a services-package-level
// var, not a constructor param — see gwsvc.SetGatewayManifestCacheBackend's doc
// comment for why). Called directly from app.Run at startup, before the rest of the
// dependency graph is built, so every reader/writer in the services package picks up
// the configured backend from their first call.
func ProvideGatewayManifestCacheBackend(cfg config.Config) (gwsvc.GatewayManifestCacheBackend, error) {
	switch cfg.GatewayManifestCache.Backend {
	case "redis":
		return gwsvc.NewRedisGatewayManifestCache(cfg.GatewayManifestCache.Redis), nil
	case "memory", "":
		return gwsvc.NewInMemoryGatewayManifestCache(), nil
	default:
		// Config validation (validateGatewayManifestCacheConfig) already rejects an
		// unrecognized backend at load time; this is an extra guard against a Config
		// built directly (e.g. in tests) bypassing that validation.
		return nil, fmt.Errorf("unknown gateway manifest cache backend %q", cfg.GatewayManifestCache.Backend)
	}
}

var reconcileProviderSet = wire.NewSet(
	gwsvc.NewLLMProviderKind,
	gwsvc.NewLLMProxyKind,
	gwsvc.NewMCPProxyKind,
	gwsvc.NewPlatformAgentExposures,
	gwsvc.NewA2AAgentKind,
	ProvideOtelIngestKind,
	gwsvc.NewOtelIngestPublisher,
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
	gatewayService *gwsvc.PlatformGatewayService,
	ackHandler *gwsvc.DeploymentAckHandler,
	otelIngest *gwsvc.OtelIngestPublisher,
	cfg config.Config,
	logger *slog.Logger,
) gwctl.WebSocketController {
	rateLimitCount := cfg.WebSocket.RateLimitPerMin
	// A connecting gateway gets the trace-ingest API if it does not serve the current one yet.
	onConnect := func(ctx context.Context, gateway *models.Gateway) {
		ctx, cancel := context.WithTimeout(ctx, time.Minute)
		defer cancel()
		if err := otelIngest.EnsureOnGateway(ctx, gateway.OUID, gateway.UUID); err != nil {
			logger.Error("Failed to deploy the trace-ingest API", "gatewayID", gateway.UUID, "error", err)
		}
	}
	return gwctl.NewWebSocketController(manager, hub, gatewayService, ackHandler, rateLimitCount, onConnect)
}

// ProvideOtelIngestKind is the built-in trace-ingest API, accepting tokens Agent Manager signs.
func ProvideOtelIngestKind(cfg config.Config) *gwsvc.OtelIngestKind {
	return gwsvc.NewOtelIngestKind(cfg.OtelIngest.UpstreamURL, cfg.JWTSigning.Issuer)
}

// ProvideDefaultGatewaySeeder registers the gateway named in configuration.
func ProvideDefaultGatewaySeeder(
	cfg config.Config, gateways *gwsvc.PlatformGatewayService, repo gwrepo.GatewayRepository,
	platform repositories.PlatformRepository, logger *slog.Logger,
) *gwsvc.DefaultGatewaySeeder {
	return gwsvc.NewDefaultGatewaySeeder(cfg.DefaultGateway, gateways, repo, platform, logger)
}

// ProvideDeploymentAckHandler creates a new deployment ack handler that runs the reconcile kinds' after-live hooks.
func ProvideDeploymentAckHandler(deploymentRepo gwrepo.DeploymentRepository, core *reconcile.Core) *gwsvc.DeploymentAckHandler {
	return gwsvc.NewDeploymentAckHandler(deploymentRepo, core.AfterLive())
}

// ProvideReconcileKinds lists the gateway artifact kinds the reconcile core deploys.
func ProvideReconcileKinds(
	llmProvider *gwsvc.LLMProviderKind, llmProxy *gwsvc.LLMProxyKind, mcp *gwsvc.MCPProxyKind,
	agent *gwsvc.A2AAgentKind, otelIngest *gwsvc.OtelIngestKind,
) reconcile.Kinds {
	return reconcile.Kinds{llmProvider, llmProxy, mcp, agent, otelIngest}
}

// ProvideReconcileCore builds the reconcile core with the worker's intervals, lease and attempt budget from cfg, and
// the gateways it locks and checks contexts on.
func ProvideReconcileCore(
	db *gorm.DB, deploymentRepo gwrepo.DeploymentRepository, gatewayRepo gwrepo.GatewayRepository,
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
	gatewayRepo gwrepo.GatewayRepository, gatewayApplier gwsvc.GatewayConfigApplier, worker *reconcile.Worker,
	core *reconcile.Core,
) *gwsvc.PlatformGatewayService {
	return gwsvc.NewPlatformGatewayService(gatewayRepo, gatewayApplier).WithEnvironmentPlacer(worker).
		WithEnvironmentGuard(core)
}

func ProvideGatewayRepository(db *gorm.DB) gwrepo.GatewayRepository {
	return gwrepo.NewGatewayRepo(db)
}

func ProvideLLMProviderTemplateRepository(db *gorm.DB) gwrepo.LLMProviderTemplateRepository {
	return gwrepo.NewLLMProviderTemplateRepo(db)
}

func ProvideLLMProviderRepository(db *gorm.DB, secrets *repositories.SecretStore) gwrepo.LLMProviderRepository {
	return gwrepo.NewLLMProviderRepo(db, secrets)
}

func ProvideLLMProxyRepository(db *gorm.DB) gwrepo.LLMProxyRepository {
	return gwrepo.NewLLMProxyRepo(db)
}

func ProvideMCPProxyRepository(db *gorm.DB) gwrepo.MCPProxyRepository {
	return gwrepo.NewMCPProxyRepo(db)
}

func ProvideDeploymentRepository(db *gorm.DB) gwrepo.DeploymentRepository {
	return gwrepo.NewDeploymentRepo(db)
}

func ProvideA2AAgentCardRepository(db *gorm.DB) gwrepo.A2AAgentCardRepository {
	return gwrepo.NewA2AAgentCardRepository(db)
}

func ProvideA2AProxyRepository(db *gorm.DB, secrets *repositories.SecretStore) gwrepo.A2AProxyRepository {
	return gwrepo.NewA2AProxyRepository(db, secrets)
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

func ProvideArtifactRepository(db *gorm.DB) gwrepo.ArtifactRepository {
	return gwrepo.NewArtifactRepo(db)
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
