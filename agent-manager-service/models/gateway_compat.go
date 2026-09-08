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

import (
	"time"

	"github.com/google/uuid"
)

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
	ProviderID     string    `json:"providerId"`
	DeploymentID   string    `json:"deploymentId"`
	PerformedAt    time.Time `json:"performedAt"`
	GatewayID      string    `json:"gatewayId"`
	OrganizationID string    `json:"organizationId"`
	Status         string    `json:"status"`
}

// LLMProviderUndeploymentEvent represents an LLM provider undeployment event
type LLMProviderUndeploymentEvent struct {
	ProviderID     string    `json:"providerId"`
	DeploymentID   string    `json:"deploymentId"`
	PerformedAt    time.Time `json:"performedAt"`
	GatewayID      string    `json:"gatewayId"`
	OrganizationID string    `json:"organizationId"`
}

// LLMProxyDeploymentEvent represents an LLM proxy deployment event
type LLMProxyDeploymentEvent struct {
	ProxyID        string `json:"proxyId"`
	DeploymentID   string `json:"deploymentId"`
	Vhost          string `json:"vhost"`
	Environment    string `json:"environment"`
	GatewayID      string `json:"gatewayId"`
	OrganizationID string `json:"organizationId"`
	Status         string `json:"status"`
}

// MCPProxyDeploymentEvent represents an MCP proxy deployment event.
type MCPProxyDeploymentEvent struct {
	ProxyID      string    `json:"proxyId"`
	DeploymentID string    `json:"deploymentId"`
	PerformedAt  time.Time `json:"performedAt"`
}

// MCPProxyDeletionEvent represents an MCP proxy deletion event.
type MCPProxyDeletionEvent struct {
	ProxyID string `json:"proxyId"`
}

// LLMProviderDeletionEvent represents an LLM provider deletion event.
//
// Distinct from LLMProviderUndeploymentEvent: the gateway treats undeploy as a
// soft state change that deliberately preserves the config, its keys and its
// policies, so only this event removes the config from the gateway's store.
// Without it a deleted provider's config lives on in the gateway's xDS snapshot
// forever. The payload matches the gateway's LLMProviderDeletedEventPayload.
type LLMProviderDeletionEvent struct {
	ProviderID string `json:"providerId"`
}

// LLMProxyDeletionEvent represents an LLM proxy deletion event.
//
// See LLMProviderDeletionEvent: undeploy is soft on the gateway side, so this is
// the only event that reclaims a derived proxy's config. The payload matches the
// gateway's LLMProxyDeletedEventPayload.
type LLMProxyDeletionEvent struct {
	ProxyID string `json:"proxyId"`
}

// LLMProxyUndeploymentEvent represents an LLM proxy undeployment event
type LLMProxyUndeploymentEvent struct {
	ProxyID        string `json:"proxyId"`
	Vhost          string `json:"vhost"`
	Environment    string `json:"environment"`
	GatewayID      string `json:"gatewayId"`
	OrganizationID string `json:"organizationId"`
}

// ApplicationAPIKeyMapping represents a single API key bound to an application.
type ApplicationAPIKeyMapping struct {
	APIKeyUUID string `json:"apiKeyUuid"`
}

// ApplicationUpdatedEvent matches the gateway-controller's ApplicationUpdatedEvent structure.
// Broadcasting this event causes the gateway to bind the listed API keys to the application,
// enabling per-consumer (per-application) rate limiting in the policy engine.
type ApplicationUpdatedEvent struct {
	ApplicationID   string                     `json:"applicationId"`
	ApplicationUUID string                     `json:"applicationUuid"`
	ApplicationName string                     `json:"applicationName"`
	ApplicationType string                     `json:"applicationType"`
	Mappings        []ApplicationAPIKeyMapping `json:"mappings"`
}
