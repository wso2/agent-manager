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
	"log/slog"
	"time"

	"github.com/google/wire"
	"gorm.io/gorm"

	observabilitysvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/observabilitysvc"
	occlient "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	traceobserversvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/traceobserversvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/websocket"
)

// Provider sets
var configProviderSet = wire.NewSet(
	ProvideConfigFromPtr,
)

var clientProviderSet = wire.NewSet(
	ProvideObservabilitySvcClient,
	traceobserversvc.NewTraceObserverClient,
	ProvideOCClient,
)

var serviceProviderSet = wire.NewSet(
	services.NewAgentManagerService,
	services.NewInfraResourceManager,
	services.NewObservabilityManager,
	services.NewAgentTokenManagerService,
	services.NewRepositoryService,
	services.NewMonitorExecutor,
	services.NewMonitorManagerService,
	services.NewMonitorSchedulerService,
	services.NewEvaluatorManagerService,
	services.NewEnvironmentService,
	services.NewPlatformGatewayService,
	services.NewLLMProviderTemplateService,
	services.NewLLMProviderService,
	services.NewLLMProxyService,
	services.NewLLMProviderDeploymentService,
	services.NewLLMProviderAPIKeyService,
	services.NewLLMProxyAPIKeyService,
	services.NewLLMProxyDeploymentService,
	services.NewGatewayInternalAPIService,
	services.NewMonitorScoresService,
	services.NewCatalogService,
	services.NewLLMTemplateStore,
)

var controllerProviderSet = wire.NewSet(
	controllers.NewAgentController,
	controllers.NewInfraResourceController,
	controllers.NewObservabilityController,
	controllers.NewAgentTokenController,
	controllers.NewRepositoryController,
	controllers.NewEnvironmentController,
	controllers.NewGatewayController,
	controllers.NewLLMController,
	controllers.NewLLMDeploymentController,
	controllers.NewLLMProviderAPIKeyController,
	controllers.NewLLMProxyAPIKeyController,
	controllers.NewLLMProxyDeploymentController,
	ProvideWebSocketController,
	controllers.NewGatewayInternalController,
	controllers.NewMonitorController,
	controllers.NewMonitorScoresController,
	controllers.NewMonitorScoresPublisherController,
	controllers.NewEvaluatorController,
	controllers.NewCatalogController,
)

var testClientProviderSet = wire.NewSet(
	ProvideTestOpenChoreoClient,
	ProvideTestObservabilitySvcClient,
	ProvideTestTraceObserverClient,
)

// ProvideLogger provides the configured slog.Logger instance
func ProvideLogger() *slog.Logger {
	return slog.Default()
}

// ProvideOCClient creates the OpenChoreo client
func ProvideOCClient(cfg config.Config, authProvider occlient.AuthProvider) (occlient.OpenChoreoClient, error) {
	return occlient.NewOpenChoreoClient(&occlient.Config{
		BaseURL:      cfg.OpenChoreo.BaseURL,
		AuthProvider: authProvider,
	})
}

// ProvideObservabilitySvcClient creates the observability service client
func ProvideObservabilitySvcClient(cfg config.Config, authProvider occlient.AuthProvider) (observabilitysvc.ObservabilitySvcClient, error) {
	return observabilitysvc.NewObservabilitySvcClient(&observabilitysvc.Config{
		BaseURL:      cfg.Observer.URL,
		AuthProvider: authProvider,
	})
}

var loggerProviderSet = wire.NewSet(
	ProvideLogger,
)

var repositoryProviderSet = wire.NewSet(
	ProvideGatewayRepository,
	ProvideLLMProviderTemplateRepository,
	ProvideLLMProviderRepository,
	ProvideLLMProxyRepository,
	ProvideDeploymentRepository,
	ProvideArtifactRepository,
	ProvideScoreRepository,
	ProvideCatalogRepository,
	ProvideMonitorRepository,
	ProvideAgentConfigRepository,
)

var websocketProviderSet = wire.NewSet(
	ProvideWebSocketManager,
	services.NewGatewayEventsService,
)

// Test client providers
func ProvideTestOpenChoreoClient(testClients TestClients) occlient.OpenChoreoClient {
	return testClients.OpenChoreoClient
}

func ProvideTestObservabilitySvcClient(testClients TestClients) observabilitysvc.ObservabilitySvcClient {
	return testClients.ObservabilitySvcClient
}

func ProvideTestTraceObserverClient(testClients TestClients) traceobserversvc.TraceObserverClient {
	return testClients.TraceObserverClient
}

// ProvideWebSocketManager creates a new WebSocket manager with config
func ProvideWebSocketManager(cfg config.Config) *websocket.Manager {
	wsConfig := websocket.ManagerConfig{
		MaxConnections:    cfg.WebSocket.MaxConnections,
		HeartbeatInterval: 20 * time.Second,
		HeartbeatTimeout:  time.Duration(cfg.WebSocket.ConnectionTimeout) * time.Second,
	}
	return websocket.NewManager(wsConfig)
}

// ProvideWebSocketController creates a new WebSocket controller with rate limiting
func ProvideWebSocketController(
	manager *websocket.Manager,
	gatewayService *services.PlatformGatewayService,
	cfg config.Config,
) controllers.WebSocketController {
	rateLimitCount := cfg.WebSocket.RateLimitPerMin
	return controllers.NewWebSocketController(manager, gatewayService, rateLimitCount)
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

func ProvideDeploymentRepository(db *gorm.DB) repositories.DeploymentRepository {
	return repositories.NewDeploymentRepo(db)
}

func ProvideArtifactRepository(db *gorm.DB) repositories.ArtifactRepository {
	return repositories.NewArtifactRepo(db)
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

// InitializeAppParams wires up all application dependencies
func InitializeAppParams(cfg *config.Config, db *gorm.DB, authProvider occlient.AuthProvider) (*AppParams, error) {
	wire.Build(
		configProviderSet,
		clientProviderSet,
		loggerProviderSet,
		repositoryProviderSet,
		websocketProviderSet,
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
) (*AppParams, error) {
	wire.Build(
		testClientProviderSet,
		loggerProviderSet,
		repositoryProviderSet,
		websocketProviderSet,
		serviceProviderSet,
		controllerProviderSet,
		configProviderSet,
		ProvideJWTSigningConfig,
		wire.Struct(new(AppParams), "*"),
	)
	return &AppParams{}, nil
}
