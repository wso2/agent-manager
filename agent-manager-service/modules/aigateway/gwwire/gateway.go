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

// Package gwwire builds the AI Gateway on the core's services.
package gwwire

import (
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/websocket"
)

// Gateway is the built gateway: its controllers, workers and the services the
// MCP server and the core's capabilities use.
type Gateway struct {
	GatewayController            gwctl.GatewayController
	LLMController                gwctl.LLMController
	LLMDeploymentController      gwctl.LLMDeploymentController
	LLMProviderAPIKeyController  gwctl.LLMProviderAPIKeyController
	LLMProxyAPIKeyController     gwctl.LLMProxyAPIKeyController
	AgentAPIKeyController        gwctl.AgentAPIKeyController
	A2AAgentCardController       gwctl.A2AAgentCardController
	A2AProxyController           gwctl.A2AProxyController
	LLMProxyDeploymentController gwctl.LLMProxyDeploymentController
	MCPProxyController           gwctl.MCPProxyController
	WebSocketController          gwctl.WebSocketController
	GatewayInternalController    gwctl.GatewayInternalController
	CatalogController            gwctl.CatalogController
	AgentConfigurationController gwctl.AgentConfigurationController
	MCPProxyScopeController      gwctl.MCPProxyScopeController
	AgentIdentityController      gwctl.AgentIdentityController
	AgentGatewayController       *gwctl.AgentGatewayController
	EnvironmentGatewayController *gwctl.EnvironmentGatewayController

	A2ACardReconciler        gwsvc.A2ACardReconcilerService
	DeploymentTimeoutSweeper gwsvc.DeploymentTimeoutSweeper
	ReconcileWorker          *reconcile.Worker
	DefaultGatewaySeeder     *gwsvc.DefaultGatewaySeeder
	LLMTemplateStore         *gwsvc.LLMTemplateStore
	WebSocketManager         *websocket.Manager
	EventHub                 eventhub.EventHub

	LLMProviderService           *gwsvc.LLMProviderService
	LLMProviderDeploymentService *gwsvc.LLMProviderDeploymentService
	PlatformGatewayService       *gwsvc.PlatformGatewayService
	MCPProxyService              *gwsvc.MCPProxyService
	AgentConfigurationService    gwsvc.AgentConfigurationService
	AgentGateway                 *gwsvc.AgentGatewayService
}
