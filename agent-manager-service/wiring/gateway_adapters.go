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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/gateway"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/gateway/adapter/mock"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/gateway/adapter/onpremise"
)

// ProvideGatewayAdapter provides a gateway adapter for dependency injection
// TODO: Phase 5 will integrate with configuration to select adapter type based on config
// For now, we use the mock adapter for development/testing
func ProvideGatewayAdapter(logger *slog.Logger) gateway.IGatewayAdapter {
	// Create adapter directly (bypass factory for Phase 4 simplicity)
	// In Phase 5, we'll use the factory with proper config
	adapter, err := mock.NewMockAdapter("mock", false, logger)
	if err != nil {
		// This should never happen with the mock adapter
		// If it does, we'll create a basic mock adapter directly
		return &mock.MockAdapter{
			AdapterType: "mock",
			ShouldFail:  false,
		}
	}
	return adapter
}

// InitGatewayAdapters initializes the gateway factory with built-in adapters
// This function should be called during application initialization
func InitGatewayAdapters(factory *gateway.AdapterFactory, logger *slog.Logger) {
	// Register on-premise adapter
	factory.Register("on-premise", func(config gateway.AdapterConfig, logger *slog.Logger) (gateway.IGatewayAdapter, error) {
		return onpremise.NewOnPremiseAdapter(config, logger)
	})

	// Register mock adapter for testing
	factory.Register("mock", func(config gateway.AdapterConfig, logger *slog.Logger) (gateway.IGatewayAdapter, error) {
		// Create a mock adapter with configurable behavior
		shouldFail := false
		if fail, ok := config.Parameters["shouldFail"].(bool); ok {
			shouldFail = fail
		}
		adapterType := "mock"
		if at, ok := config.Parameters["adapterType"].(string); ok {
			adapterType = at
		}
		return mock.NewMockAdapter(adapterType, shouldFail, logger)
	})

	// Cloud adapter will be registered in future implementations
	// factory.Register("cloud", func(config gateway.AdapterConfig, logger *slog.Logger) (gateway.IGatewayAdapter, error) {
	//     return cloud.NewCloudAdapter(config, logger)
	// })
}
