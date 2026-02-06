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

package gateway

import (
	"context"
)

// IGatewayAdapter defines the contract for gateway management operations.
// Only ONE implementation is active at runtime based on deployment configuration.
type IGatewayAdapter interface {
	// GetAdapterType returns the adapter type identifier (e.g., "on-premise", "cloud")
	GetAdapterType() string

	// Close cleans up adapter resources
	Close() error

	// ========================================================================
	// Gateway Lifecycle Management
	// ========================================================================

	// ValidateGatewayEndpoint checks if a gateway endpoint is reachable
	ValidateGatewayEndpoint(ctx context.Context, controlPlaneURL string) error

	// CheckHealth performs a health check on a gateway
	CheckHealth(ctx context.Context, controlPlaneURL string) (*HealthStatus, error)
}
