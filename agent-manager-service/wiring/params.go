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

	"gorm.io/gorm"

	apiplatformclient "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/apiplatformsvc/client"
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

// AppParams contains all wired application dependencies
type AppParams struct {
	// Middleware
	AuthMiddleware jwtassertion.Middleware
	Logger         *slog.Logger

	// Controllers
	AgentController           controllers.AgentController
	InfraResourceController   controllers.InfraResourceController
	ObservabilityController   controllers.ObservabilityController
	AgentTokenController      controllers.AgentTokenController
	RepositoryController      controllers.RepositoryController
	EnvironmentController     controllers.EnvironmentController
	GatewayController         controllers.GatewayController
	LLMController             controllers.LLMController
	LLMDeploymentController   controllers.LLMDeploymentController
	WebSocketController       controllers.WebSocketController
	GatewayInternalController controllers.GatewayInternalController

	// Services
	EnvironmentSyncer  services.EnvironmentSynchronizer
	OrganizationSyncer services.OrganizationSynchronizer
	LLMTemplateSeeder  *services.LLMTemplateSeeder

	// Repositories
	OrganizationRepository repositories.OrganizationRepository

	// WebSocket
	WebSocketManager *websocket.Manager

	// Clients
	APIPlatformClient apiplatformclient.APIPlatformClient

	// Database
	DB *gorm.DB
}

// TestClients contains all mock clients needed for testing
type TestClients struct {
	OpenChoreoClient       occlient.OpenChoreoClient
	ObservabilitySvcClient observabilitysvc.ObservabilitySvcClient
	TraceObserverClient    traceobserversvc.TraceObserverClient
	APIPlatformClient      apiplatformclient.APIPlatformClient
}

func ProvideConfigFromPtr(config *config.Config) config.Config {
	return *config
}

func ProvideAuthMiddleware(config config.Config) jwtassertion.Middleware {
	return jwtassertion.JWTAuthMiddleware(config.AuthHeader)
}

func ProvideJWTSigningConfig(config config.Config) config.JWTSigningConfig {
	return config.JWTSigning
}
