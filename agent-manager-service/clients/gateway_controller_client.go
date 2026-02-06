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

package clients

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// GatewayControllerClient is a client for communicating with gateway-controller instances
type GatewayControllerClient interface {
	// HealthCheck performs a health check on a gateway controller
	HealthCheck(ctx context.Context, controlPlaneURL string) error
}

type gatewayControllerClient struct {
	httpClient *http.Client
}

// NewGatewayControllerClient creates a new gateway controller client
func NewGatewayControllerClient() GatewayControllerClient {
	return &gatewayControllerClient{
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// HealthCheck performs a health check on a gateway controller
func (c *gatewayControllerClient) HealthCheck(ctx context.Context, controlPlaneURL string) error {
	healthURL := fmt.Sprintf("%s/health", controlPlaneURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check failed with status %d", resp.StatusCode)
	}

	return nil
}
