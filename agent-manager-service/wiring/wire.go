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

	apiplatformauth "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/apiplatformsvc/auth"
	apiplatformclient "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/apiplatformsvc/client"
	observabilitysvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/observabilitysvc"
	ocauth "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/auth"
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
	ProvideOCAuthProvider,
	ProvideOCClient,
	ProvideAPIPlatformAuthProvider,
	ProvideAPIPlatformConfig,
	ProvideAPIPlatformClient,
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
	services.NewEnvironmentSyncer,
	services.NewOrganizationSyncer,
	services.NewPlatformGatewayService,
	services.NewLLMProviderTemplateService,
	services.NewLLMProviderService,
	services.NewLLMProxyService,
	services.NewLLMProviderDeploymentService,
	services.NewGatewayInternalAPIService,
	ProvideLLMTemplateSeeder,
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
	ProvideWebSocketController,
	controllers.NewGatewayInternalController,
)

var testClientProviderSet = wire.NewSet(
	ProvideTestOpenChoreoClient,
	ProvideTestObservabilitySvcClient,
	ProvideTestTraceObserverClient,
	ProvideTestAPIPlatformClient,
)

// ProvideLogger provides the configured slog.Logger instance
func ProvideLogger() *slog.Logger {
	return slog.Default()
}

// ProvideOCAuthProvider creates the OpenChoreo auth provider using IDP config
func ProvideOCAuthProvider(cfg config.Config) occlient.AuthProvider {
	return ocauth.NewAuthProvider(ocauth.Config{
		TokenURL:     cfg.IDP.TokenURL,
		ClientID:     cfg.IDP.ClientID,
		ClientSecret: cfg.IDP.ClientSecret,
	})
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
	ProvideOrganizationRepository,
	ProvideProjectRepository,
	ProvideGatewayRepository,
	ProvideAPIRepository,
	ProvideLLMProviderTemplateRepository,
	ProvideLLMProviderRepository,
	ProvideLLMProxyRepository,
	ProvideDeploymentRepository,
	ProvideArtifactRepository,
)

var websocketProviderSet = wire.NewSet(
	ProvideWebSocketManager,
)

// ProvideAPIPlatformAuthProvider creates an auth provider for API Platform
func ProvideAPIPlatformAuthProvider(cfg config.Config) apiplatformclient.AuthProvider {
	// Only create auth provider if OAuth2 credentials are configured
	if cfg.IDP.TokenURL != "" && cfg.IDP.ClientID != "" && cfg.IDP.ClientSecret != "" {
		return apiplatformauth.NewAuthProvider(apiplatformauth.Config{
			TokenURL:     cfg.IDP.TokenURL,
			ClientID:     cfg.IDP.ClientID,
			ClientSecret: cfg.IDP.ClientSecret,
		})
	}
	return nil
}

// ProvideAPIPlatformConfig extracts API Platform configuration from config
func ProvideAPIPlatformConfig(cfg config.Config, authProvider apiplatformclient.AuthProvider) *apiplatformclient.Config {
	baseUrl := ""
	if cfg.APIPlatform.Enable {
		baseUrl = cfg.APIPlatform.BaseURL
	}
	return &apiplatformclient.Config{
		BaseURL:      baseUrl,
		AuthProvider: authProvider,
	}
}

// ProvideAPIPlatformClient creates a new API Platform client
// Returns nil if the client cannot be created (will be checked at runtime)
func ProvideAPIPlatformClient(cfg *apiplatformclient.Config) apiplatformclient.APIPlatformClient {
	if cfg.BaseURL == "" || cfg.AuthProvider == nil {
		// Return nil if not configured - services should handle nil client gracefully
		return nil
	}

	apiPlatformClient, err := apiplatformclient.NewAPIPlatformClient(cfg)
	if err != nil {
		slog.Error("Failed to create API Platform client", "error", err)
		return nil
	}
	return apiPlatformClient
}

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

func ProvideTestAPIPlatformClient(testClients TestClients) apiplatformclient.APIPlatformClient {
	return testClients.APIPlatformClient
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

// Repository providers (injecting *gorm.DB)
func ProvideOrganizationRepository(db *gorm.DB) repositories.OrganizationRepository {
	return repositories.NewOrganizationRepo(db)
}

func ProvideProjectRepository(db *gorm.DB) repositories.ProjectRepository {
	return repositories.NewProjectRepo(db)
}

func ProvideGatewayRepository(db *gorm.DB) repositories.GatewayRepository {
	return repositories.NewGatewayRepo(db)
}

func ProvideAPIRepository(db *gorm.DB) repositories.APIRepository {
	return repositories.NewAPIRepo(db)
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

// ProvideLLMTemplateSeeder creates a new LLM template seeder with empty templates
// Templates will be loaded at startup in main.go
func ProvideLLMTemplateSeeder(templateRepo repositories.LLMProviderTemplateRepository) *services.LLMTemplateSeeder {
	// Create seeder with empty templates - actual templates loaded in main.go
	return services.NewLLMTemplateSeeder(templateRepo, nil)
}

// InitializeAppParams wires up all application dependencies
func InitializeAppParams(cfg *config.Config, db *gorm.DB) (*AppParams, error) {
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
