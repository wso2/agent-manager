// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
	observabilitysvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/observabilitysvc"
	clients "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
)

type AppParams struct {
	AuthMiddleware          jwtassertion.Middleware
	AgentController         controllers.AgentController
	InfraResourceController controllers.InfraResourceController
	BuildCIController       controllers.BuildCIController
}

// TestClients contains all mock clients needed for testing
type TestClients struct {
	OpenChoreoSvcClient    clients.OpenChoreoSvcClient
	ObservabilitySvcClient observabilitysvc.ObservabilitySvcClient
}

func ProvideConfigFromPtr(config *config.Config) config.Config {
	return *config
}

func ProvideAuthMiddleware(config config.Config) jwtassertion.Middleware {
	return jwtassertion.JWTAuthMiddleware(config.AuthHeader)
}
