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

//go:build wireinject
// +build wireinject

package wiring

import (
	"log/slog"

	"github.com/google/wire"

	observabilitysvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/observabilitysvc"
	clients "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc"
	traceobserversvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/traceobserversvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
)

var configProviderSet = wire.NewSet(
	ProvideConfigFromPtr,
)

var repositoryProviderSet = wire.NewSet(
	repositories.NewOrganizationRepository,
	repositories.NewAgentRepository,
	repositories.NewProjectRepository,
	repositories.NewInternalAgentRepository,
)

var clientProviderSet = wire.NewSet(
	clients.NewOpenChoreoSvcClient,
	observabilitysvc.NewObservabilitySvcClient,
	traceobserversvc.NewTraceObserverClient,
)

var serviceProviderSet = wire.NewSet(
	services.NewAgentManagerService,
	services.NewBuildCIManager,
	services.NewInfraResourceManager,
	services.NewObservabilityManager,
)

var controllerProviderSet = wire.NewSet(
	controllers.NewAgentController,
	controllers.NewBuildCIController,
	controllers.NewInfraResourceController,
	controllers.NewObservabilityController,
)

var testClientProviderSet = wire.NewSet(
	ProvideTestOpenChoreoSvcClient,
	ProvideTestObservabilitySvcClient,
	ProvideTestTraceObserverClient,
)

// ProvideLogger provides the configured slog.Logger instance
func ProvideLogger() *slog.Logger {
	return slog.Default()
}

var loggerProviderSet = wire.NewSet(
	ProvideLogger,
)

// ProvideTestOpenChoreoSvcClient extracts the OpenChoreoSvcClient from TestClients
func ProvideTestOpenChoreoSvcClient(testClients TestClients) clients.OpenChoreoSvcClient {
	return testClients.OpenChoreoSvcClient
}

// ProvideTestObservabilitySvcClient extracts the ObservabilitySvcClient from TestClients
func ProvideTestObservabilitySvcClient(testClients TestClients) observabilitysvc.ObservabilitySvcClient {
	return testClients.ObservabilitySvcClient
}

// ProvideTestTraceObserverClient extracts the TraceObserverClient from TestClients
func ProvideTestTraceObserverClient(testClients TestClients) traceobserversvc.TraceObserverClient {
	return testClients.TraceObserverClient
}

func InitializeAppParams(cfg *config.Config) (*AppParams, error) {
	wire.Build(
		configProviderSet,
		repositoryProviderSet,
		clientProviderSet,
		loggerProviderSet,
		serviceProviderSet,
		controllerProviderSet,
		ProvideAuthMiddleware,
		wire.Struct(new(AppParams), "*"),
	)
	return &AppParams{}, nil
}

func InitializeTestAppParamsWithClientMocks(cfg *config.Config, authMiddleware jwtassertion.Middleware, testClients TestClients) (*AppParams, error) {
	wire.Build(
		repositoryProviderSet,
		testClientProviderSet,
		loggerProviderSet,
		serviceProviderSet,
		controllerProviderSet,
		wire.Struct(new(AppParams), "*"),
	)
	return &AppParams{}, nil
}
