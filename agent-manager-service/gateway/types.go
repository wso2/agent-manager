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
	"time"
)

// HealthStatus represents the health check result for a gateway
type HealthStatus struct {
	Status       string
	ResponseTime time.Duration
	ErrorMessage string
	CheckedAt    time.Time
}

// ========================================================================
// LLM Provider Types (Phase 7)
// ========================================================================

// ProviderDeploymentConfig holds the configuration for deploying a provider
type ProviderDeploymentConfig struct {
	Handle        string                 `json:"handle"`
	DisplayName   string                 `json:"displayName"`
	Template      string                 `json:"template"`
	Configuration map[string]interface{} `json:"configuration"`
}

// ProviderDeploymentResult holds the result of a provider deployment operation
type ProviderDeploymentResult struct {
	DeploymentID string    `json:"deploymentId"`
	Status       string    `json:"status"`
	DeployedAt   time.Time `json:"deployedAt"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
}

// ProviderStatus holds the current status of a provider on a gateway
type ProviderStatus struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Kind         string                 `json:"kind"`
	Status       string                 `json:"status"`
	Spec         map[string]interface{} `json:"spec,omitempty"`
	CreatedAt    time.Time              `json:"createdAt"`
	DeployedAt   *time.Time             `json:"deployedAt,omitempty"`
	ErrorMessage string                 `json:"errorMessage,omitempty"`
}

// PolicyInfo holds information about an available policy
type PolicyInfo struct {
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters,omitempty"`
}
