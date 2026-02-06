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

package mock

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/gateway"
)

// MockAdapter is a mock implementation of IGatewayAdapter for testing purposes
type MockAdapter struct {
	AdapterType  string
	ShouldFail   bool
	FailMessage  string
	ResponseTime time.Duration
	logger       *slog.Logger
}

// NewMockAdapter creates a new mock adapter instance
func NewMockAdapter(adapterType string, shouldFail bool, logger *slog.Logger) (gateway.IGatewayAdapter, error) {
	return &MockAdapter{
		AdapterType:  adapterType,
		ShouldFail:   shouldFail,
		FailMessage:  "mock adapter failure",
		ResponseTime: 10 * time.Millisecond,
		logger:       logger,
	}, nil
}

// GetAdapterType returns the adapter type identifier
func (m *MockAdapter) GetAdapterType() string {
	if m.AdapterType != "" {
		return m.AdapterType
	}
	return "mock"
}

// Close cleans up adapter resources
func (m *MockAdapter) Close() error {
	m.logger.Debug("mock adapter closed")
	return nil
}

// ValidateGatewayEndpoint checks if a gateway endpoint is reachable
func (m *MockAdapter) ValidateGatewayEndpoint(ctx context.Context, controlPlaneURL string) error {
	if m.ShouldFail {
		return fmt.Errorf("%s: %s", m.FailMessage, controlPlaneURL)
	}
	m.logger.Debug("mock gateway validation successful", "url", controlPlaneURL)
	return nil
}

// CheckHealth performs a health check on a gateway
func (m *MockAdapter) CheckHealth(ctx context.Context, controlPlaneURL string) (*gateway.HealthStatus, error) {
	start := time.Now()

	err := m.ValidateGatewayEndpoint(ctx, controlPlaneURL)

	responseTime := time.Since(start)
	if m.ResponseTime > 0 {
		responseTime = m.ResponseTime
	}

	status := &gateway.HealthStatus{
		ResponseTime: responseTime,
		CheckedAt:    time.Now(),
	}

	if err != nil {
		status.Status = "ERROR"
		status.ErrorMessage = err.Error()
	} else {
		status.Status = "ACTIVE"
	}

	return status, nil
}
