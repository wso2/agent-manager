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

package onpremise

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/gateway"
)

const (
	defaultTimeout     = 30 * time.Second
	healthCheckTimeout = 5 * time.Second
)

// OnPremiseAdapter implements IGatewayAdapter for on-premise deployments
type OnPremiseAdapter struct {
	httpClient *http.Client
	config     gateway.AdapterConfig
	logger     *slog.Logger
}

// NewOnPremiseAdapter creates a new on-premise adapter instance
func NewOnPremiseAdapter(config gateway.AdapterConfig, logger *slog.Logger) (gateway.IGatewayAdapter, error) {
	timeout := defaultTimeout
	if params, ok := config.Parameters["defaultTimeout"].(time.Duration); ok {
		timeout = params
	}

	adapter := &OnPremiseAdapter{
		config: config,
		logger: logger,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}

	return adapter, nil
}

// GetAdapterType returns the adapter type identifier
func (a *OnPremiseAdapter) GetAdapterType() string {
	return "on-premise"
}

// Close cleans up adapter resources
func (a *OnPremiseAdapter) Close() error {
	return nil
}

// ValidateGatewayEndpoint checks if a gateway endpoint is reachable
func (a *OnPremiseAdapter) ValidateGatewayEndpoint(ctx context.Context, controlPlaneURL string) error {
	healthURL := fmt.Sprintf("%s/health", controlPlaneURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create health check request: %w", err)
	}

	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("gateway endpoint unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("gateway health check failed with status %d", resp.StatusCode)
	}

	return nil
}

// CheckHealth performs a health check on a gateway
func (a *OnPremiseAdapter) CheckHealth(ctx context.Context, controlPlaneURL string) (*gateway.HealthStatus, error) {
	start := time.Now()

	err := a.ValidateGatewayEndpoint(ctx, controlPlaneURL)
	responseTime := time.Since(start)

	status := &gateway.HealthStatus{
		Status:       "ACTIVE",
		ResponseTime: responseTime,
		CheckedAt:    time.Now(),
	}

	if err != nil {
		status.Status = "ERROR"
		status.ErrorMessage = err.Error()
	}

	return status, nil
}

// init registers the adapter with the factory
func init() {
	// This will be called when the package is imported
	// Registration happens in wiring
}
