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
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/gateway"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/gateway/adapter/mock"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/gateway/adapter/onpremise"
)

// ProvideGatewayEncryptionKey provides the encryption key for gateway credentials
func ProvideGatewayEncryptionKey(cfg config.Config) ([]byte, error) {
	if cfg.Gateway.EncryptionKey == "" {
		return nil, fmt.Errorf("GATEWAY_ENCRYPTION_KEY is required")
	}

	// Decode base64-encoded key
	key, err := base64.StdEncoding.DecodeString(cfg.Gateway.EncryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to decode GATEWAY_ENCRYPTION_KEY: %w", err)
	}

	// Validate key size (32 bytes for AES-256)
	if len(key) != 32 {
		return nil, fmt.Errorf("GATEWAY_ENCRYPTION_KEY must be 32 bytes (base64-encoded), got %d bytes", len(key))
	}

	return key, nil
}

// ProvideGatewayAdapter provides a gateway adapter for dependency injection
// Uses configuration to select the appropriate adapter type
func ProvideGatewayAdapter(cfg config.Config, encryptionKey []byte, logger *slog.Logger) (gateway.IGatewayAdapter, error) {
	// Create adapter factory
	factory := gateway.NewAdapterFactory(logger)

	// Initialize adapters
	InitGatewayAdapters(factory, encryptionKey, logger)

	// Create adapter config from environment config
	adapterConfig := gateway.AdapterConfig{
		Type:       cfg.Gateway.AdapterType,
		Parameters: make(map[string]interface{}),
	}

	// Add on-premise specific parameters
	if cfg.Gateway.AdapterType == "on-premise" {
		adapterConfig.Parameters["defaultTimeout"] = time.Duration(cfg.Gateway.DefaultTimeout) * time.Second
		adapterConfig.Parameters["healthCheckTimeout"] = time.Duration(cfg.Gateway.HealthCheckTimeout) * time.Second
		adapterConfig.Parameters["maxRetries"] = cfg.Gateway.MaxRetries
		adapterConfig.Parameters["retryBackoff"] = time.Duration(cfg.Gateway.RetryBackoff) * time.Second
	}

	// Create adapter using factory
	adapter, err := factory.CreateAdapter(adapterConfig)
	if err != nil {
		// Fall back to mock adapter if there's an error
		logger.Error("Failed to create configured gateway adapter, attempting mock fallback",
			"adapterType", cfg.Gateway.AdapterType,
			"error", err)

		mockAdapter, mockErr := mock.NewMockAdapter("mock-fallback", false, logger)
		if mockErr != nil {
			// Both factory and mock adapter creation failed
			logger.Error("Failed to create mock fallback adapter",
				"adapterType", cfg.Gateway.AdapterType,
				"factoryError", err,
				"mockError", mockErr)
			return nil, fmt.Errorf("failed to create gateway adapter (type: %s): %w; mock fallback also failed: %w",
				cfg.Gateway.AdapterType, err, mockErr)
		}

		logger.Warn("Using mock adapter as fallback",
			"requestedType", cfg.Gateway.AdapterType,
			"actualType", mockAdapter.GetAdapterType())
		return mockAdapter, nil
	}

	logger.Info("Gateway adapter initialized", "adapterType", adapter.GetAdapterType())
	return adapter, nil
}

// InitGatewayAdapters initializes the gateway factory with built-in adapters
// This function should be called during application initialization
func InitGatewayAdapters(factory *gateway.AdapterFactory, encryptionKey []byte, logger *slog.Logger) {
	// Register on-premise adapter
	factory.Register("on-premise", func(config gateway.AdapterConfig, logger *slog.Logger) (gateway.IGatewayAdapter, error) {
		// Get DB instance from global db package
		dbInstance := db.DB(context.Background())
		return onpremise.NewOnPremiseAdapter(config, dbInstance, encryptionKey, logger)
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
	// factory.Register("cloud", func(config gateway.AdapterConfig, encryptionKey []byte, logger *slog.Logger) (gateway.IGatewayAdapter, error) {
	//     return cloud.NewCloudAdapter(config, encryptionKey, logger)
	// })
}
