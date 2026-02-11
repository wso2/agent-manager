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

//go:build wireinject
// +build wireinject

package wiring

import (
	"log/slog"

	"github.com/google/wire"

	observabilitysvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/observabilitysvc"
	ocauth "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/auth"
	occlient "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	traceobserversvc "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/traceobserversvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
)

var configProviderSet = wire.NewSet(
	ProvideConfigFromPtr,
)

var clientProviderSet = wire.NewSet(
	ProvideObservabilitySvcClient,
	traceobserversvc.NewTraceObserverClient,
	ProvideOCAuthProvider,
	ProvideOCClient,
)

var serviceProviderSet = wire.NewSet(
	services.NewAgentManagerService,
	services.NewInfraResourceManager,
	services.NewObservabilityManager,
	services.NewAgentTokenManagerService,
	services.NewRepositoryService,
)

var controllerProviderSet = wire.NewSet(
	controllers.NewAgentController,
	controllers.NewInfraResourceController,
	controllers.NewObservabilityController,
	controllers.NewAgentTokenController,
	controllers.NewRepositoryController,
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

// ProvideTestOpenChoreoClient extracts the OpenChoreoClient from TestClients
func ProvideTestOpenChoreoClient(testClients TestClients) occlient.OpenChoreoClient {
	return testClients.OpenChoreoClient
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
		clientProviderSet,
		loggerProviderSet,
		serviceProviderSet,
		controllerProviderSet,
		ProvideAuthMiddleware, ProvideJWTSigningConfig, wire.Struct(new(AppParams), "*"),
	)
	return &AppParams{}, nil
}

func InitializeTestAppParamsWithClientMocks(cfg *config.Config, authMiddleware jwtassertion.Middleware, testClients TestClients) (*AppParams, error) {
	wire.Build(
		testClientProviderSet,
		loggerProviderSet,
		serviceProviderSet,
		controllerProviderSet, configProviderSet,
		ProvideJWTSigningConfig, wire.Struct(new(AppParams), "*"),
	)
	return &AppParams{}, nil
}
