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
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/websocket"
)

// AppParams contains all wired application dependencies
type AppParams struct {
	// Middleware
	AuthMiddleware jwtassertion.Middleware
	Logger         *slog.Logger
	OrgResolver    middleware.OrgResolver
	AuditRecorder  audit.Recorder
	// OUHandleLookup maps an OU ID to its handle for middleware.ApplyOrgOverride.
	OUHandleLookup middleware.OUHandleLookup

	// Controllers
	AgentController                  controllers.AgentController
	InfraResourceController          controllers.InfraResourceController
	AgentTokenController             controllers.AgentTokenController
	EnvironmentController            controllers.EnvironmentController
	GatewayController                controllers.GatewayController
	LLMController                    controllers.LLMController
	LLMDeploymentController          controllers.LLMDeploymentController
	LLMProviderAPIKeyController      controllers.LLMProviderAPIKeyController
	LLMProxyAPIKeyController         controllers.LLMProxyAPIKeyController
	AgentAPIKeyController            controllers.AgentAPIKeyController
	A2AAgentCardController           controllers.A2AAgentCardController
	A2AProxyController               controllers.A2AProxyController
	LLMProxyDeploymentController     controllers.LLMProxyDeploymentController
	MCPProxyController               controllers.MCPProxyController
	WebSocketController              controllers.WebSocketController
	GatewayInternalController        controllers.GatewayInternalController
	MonitorController                controllers.MonitorController
	MonitorScoresController          controllers.MonitorScoresController
	MonitorScoresPublisherController controllers.MonitorScoresPublisherController
	EvaluatorController              controllers.EvaluatorController
	CatalogController                controllers.CatalogController
	AgentConfigurationController     controllers.AgentConfigurationController
	IdentityController               controllers.IdentityController
	MCPProxyScopeController          controllers.MCPProxyScopeController
	AgentIdentityController          controllers.AgentIdentityController
	EventsController                 controllers.EventsController
	MonitorAlertController           controllers.MonitorAlertController
	MonitorScheduler                 services.MonitorSchedulerService
	AgentThunderReconciler           services.AgentThunderReconcilerService
	A2ACardReconciler                services.A2ACardReconcilerService
	DeploymentTimeoutSweeper         services.DeploymentTimeoutSweeper
	ReconcileWorker                  *reconcile.Worker
	DefaultGatewaySeeder             *services.DefaultGatewaySeeder

	// Services
	LLMTemplateStore         *services.LLMTemplateStore
	InfraResourceManager     services.InfraResourceManager
	AgentManagerService      services.AgentManagerService
	AgentTokenManagerService services.AgentTokenManagerService
	EnvironmentService       services.EnvironmentService
	EventDispatcher          services.EventDispatcherService
	MonitorAlerts            services.MonitorAlertProcessor
	// Services the MCP connection tools call directly.
	LLMProviderService           *services.LLMProviderService
	LLMProviderDeploymentService *services.LLMProviderDeploymentService
	PlatformGatewayService       *services.PlatformGatewayService
	MCPProxyService              *services.MCPProxyService
	AgentConfigurationService    services.AgentConfigurationService

	// Events
	EventBus       events.Bus
	EventPublisher events.Publisher

	// PlatformRepository stores organizations, projects, environments and agents.
	PlatformRepository repositories.PlatformRepository

	// WebSocket
	WebSocketManager *websocket.Manager

	// EventHub for cross-instance gateway event delivery
	EventHub eventhub.EventHub

	// Database
	DB *gorm.DB
}

// TestClients contains all mock clients needed for testing
type TestClients struct {
	PlatformRepository repositories.PlatformRepository
	SecretMgmtClient   secretmanagersvc.SecretManagementClient
	// OUHandleLookup replaces the Thunder-backed OU ID → handle lookup when set.
	OUHandleLookup middleware.OUHandleLookup
}

func ProvideConfigFromPtr(config *config.Config) config.Config {
	return *config
}

func ProvideAuthMiddleware(config config.Config) jwtassertion.Middleware {
	var apiResourceMetadataURL, mcpResourceMetadataURL string
	if config.ServerPublicURL != "" {
		baseURL := strings.TrimRight(config.ServerPublicURL, "/")
		apiResourceMetadataURL = baseURL + "/.well-known/oauth-protected-resource"
		mcpResourceMetadataURL = apiResourceMetadataURL + "/mcp"
	}
	return jwtassertion.JWTAuthMiddlewareWithResourceMetadataResolver(config.AuthHeader, func(r *http.Request) string {
		if r.URL.Path == "/mcp" || strings.HasPrefix(r.URL.Path, "/mcp/") {
			return mcpResourceMetadataURL
		}
		return apiResourceMetadataURL
	})
}

func ProvideJWTSigningConfig(config config.Config) config.JWTSigningConfig {
	return config.JWTSigning
}

// ProvideEncryptionKey decodes the hex-encoded encryption key from config.
func ProvideEncryptionKey(cfg config.Config) ([]byte, error) {
	key, err := hex.DecodeString(cfg.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ENCRYPTION_KEY: %w", err)
	}
	if len(key) != 32 {
		return nil, fmt.Errorf("ENCRYPTION_KEY must decode to exactly 32 bytes (got %d)", len(key))
	}
	return key, nil
}
