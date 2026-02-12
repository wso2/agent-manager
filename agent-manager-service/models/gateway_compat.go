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

package models

import "github.com/google/uuid"

// Type aliases and compatibility types for backward compatibility

// PlatformGateway is an alias for Gateway
type PlatformGateway = Gateway

// GatewayResponse represents gateway response DTO (used in environment_service.go)
type GatewayResponse struct {
	UUID             string `json:"id"`
	OrganizationName string `json:"organizationName"`
	Name             string `json:"name"`
	DisplayName      string `json:"displayName"`
	GatewayType      string `json:"gatewayType"`
	VHost            string `json:"vhost"`
	IsCritical       bool   `json:"isCritical"`
	Status           string `json:"status"`
}

// GatewayEnvironmentMapping represents the mapping between gateways and environments
type GatewayEnvironmentMapping struct {
	GatewayUUID     uuid.UUID `json:"gatewayId" gorm:"column:gateway_uuid"`
	EnvironmentUUID uuid.UUID `json:"environmentId" gorm:"column:environment_uuid"`
}

// Gateway status constants
const (
	GatewayStatusActive   = "active"
	GatewayStatusInactive = "inactive"
)

// LLMProviderDeploymentEvent represents an LLM provider deployment event
type LLMProviderDeploymentEvent struct {
	ProviderID     string `json:"providerId"`
	GatewayID      string `json:"gatewayId"`
	OrganizationID string `json:"organizationId"`
	Status         string `json:"status"`
}

// LLMProviderUndeploymentEvent represents an LLM provider undeployment event
type LLMProviderUndeploymentEvent struct {
	ProviderID     string `json:"providerId"`
	GatewayID      string `json:"gatewayId"`
	OrganizationID string `json:"organizationId"`
}
