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

	"gorm.io/gorm"

	observabilitysvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/observabilitysvc"
	occlient "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	traceobserversvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/traceobserversvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/websocket"
)

// AppParams contains all wired application dependencies
type AppParams struct {
	// Middleware
	AuthMiddleware jwtassertion.Middleware
	Logger         *slog.Logger

	// Controllers
	AgentController                  controllers.AgentController
	InfraResourceController          controllers.InfraResourceController
	ObservabilityController          controllers.ObservabilityController
	AgentTokenController             controllers.AgentTokenController
	RepositoryController             controllers.RepositoryController
	EnvironmentController            controllers.EnvironmentController
	GatewayController                controllers.GatewayController
	LLMController                    controllers.LLMController
	LLMDeploymentController          controllers.LLMDeploymentController
	LLMProviderAPIKeyController      controllers.LLMProviderAPIKeyController
	LLMProxyAPIKeyController         controllers.LLMProxyAPIKeyController
	LLMProxyDeploymentController     controllers.LLMProxyDeploymentController
	WebSocketController              controllers.WebSocketController
	GatewayInternalController        controllers.GatewayInternalController
	MonitorController                controllers.MonitorController
	MonitorScoresController          controllers.MonitorScoresController
	MonitorScoresPublisherController controllers.MonitorScoresPublisherController
	EvaluatorController              controllers.EvaluatorController
	CatalogController                controllers.CatalogController
	MonitorScheduler                 services.MonitorSchedulerService

	// Services
	LLMTemplateStore *services.LLMTemplateStore

	// Clients
	OpenChoreoClient occlient.OpenChoreoClient

	// WebSocket
	WebSocketManager *websocket.Manager

	// Database
	DB *gorm.DB
}

// TestClients contains all mock clients needed for testing
type TestClients struct {
	OpenChoreoClient       occlient.OpenChoreoClient
	ObservabilitySvcClient observabilitysvc.ObservabilitySvcClient
	TraceObserverClient    traceobserversvc.TraceObserverClient
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
