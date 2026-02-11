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

// GatewayEventDTO represents events sent to gateways via WebSocket
type GatewayEventDTO struct {
	Type          string      `json:"type"`
	Payload       interface{} `json:"payload"`
	Timestamp     string      `json:"timestamp"`
	CorrelationID string      `json:"correlationId"`
	UserID        string      `json:"userId,omitempty"`
}

// ConnectionAckDTO is the acknowledgment sent when a gateway connects
type ConnectionAckDTO struct {
	Type         string `json:"type"`
	GatewayID    string `json:"gatewayId"`
	ConnectionID string `json:"connectionId"`
	Timestamp    string `json:"timestamp"`
}

// AgentDeployedEventDTO represents an agent deployment notification payload
type AgentDeployedEventDTO struct {
	AgentID     string `json:"agentId"`
	Environment string `json:"environment"`
	RevisionID  string `json:"revisionId"`
}

// AgentUndeployedEventDTO represents an agent undeployment notification payload
type AgentUndeployedEventDTO struct {
	AgentID     string `json:"agentId"`
	Environment string `json:"environment"`
}

// GatewayConfigEventDTO represents gateway configuration update notification payload
type GatewayConfigEventDTO struct {
	ConfigType string `json:"configType"`
	Action     string `json:"action"`
}

// AgentDeploymentEvent represents an agent deployment domain event
type AgentDeploymentEvent struct {
	AgentID      string
	DeploymentID string
	Environment  string
}

// AgentUndeploymentEvent represents an agent undeployment domain event
type AgentUndeploymentEvent struct {
	AgentID     string
	Environment string
}

// DeploymentEvent represents an API/LLM provider deployment event (api-platform compatible)
// Used for both agent deployments and LLM provider deployments
type DeploymentEvent struct {
	ApiId        string `json:"apiId"` // For LLM providers, this is providerId
	DeploymentID string `json:"deploymentId"`
	Vhost        string `json:"vhost"`
	Environment  string `json:"environment"`
}

// APIUndeploymentEvent represents an API/LLM provider undeployment event (api-platform compatible)
// Used for both agent undeployments and LLM provider undeployments
type APIUndeploymentEvent struct {
	ApiId       string `json:"apiId"` // For LLM providers, this is providerId
	Vhost       string `json:"vhost"`
	Environment string `json:"environment"`
}

// GatewayDeploymentNotification represents gateway deployment confirmation
// Must match api-platform's DeploymentNotification format exactly
type GatewayDeploymentNotification struct {
	ID                string           `json:"id" binding:"required"`
	Configuration     APIConfiguration `json:"configuration" binding:"required"`
	Status            string           `json:"status" binding:"required"`
	CreatedAt         string           `json:"createdAt" binding:"required"`
	UpdatedAt         string           `json:"updatedAt" binding:"required"`
	DeployedAt        *string          `json:"deployedAt,omitempty"`
	DeployedVersion   *int             `json:"deployedVersion,omitempty"`
	ProjectIdentifier string           `json:"projectIdentifier" binding:"required"`
}

// APIConfiguration represents the LLM provider configuration (api-platform compatible)
type APIConfiguration struct {
	Version string        `json:"version" yaml:"version" binding:"required"`
	Kind    string        `json:"kind" yaml:"kind" binding:"required"`
	Spec    APIConfigData `json:"spec" yaml:"spec" binding:"required"`
}

// APIConfigData represents the detailed LLM provider configuration
type APIConfigData struct {
	Name        string           `json:"name" yaml:"name" binding:"required"`
	Version     string           `json:"version" yaml:"version" binding:"required"`
	Context     string           `json:"context" yaml:"context" binding:"required"`
	ProjectName string           `json:"projectName,omitempty" yaml:"projectName,omitempty"`
	Upstreams   []Upstream       `json:"upstreams" yaml:"upstream" binding:"required"`
	Operations  []BasicOperation `json:"operations" yaml:"operations" binding:"required"`
}

// Upstream represents backend service configuration
type Upstream struct {
	URL string `json:"url" binding:"required"`
}

// BasicOperation represents API basic operation configuration
type BasicOperation struct {
	Method string `json:"method" binding:"required"`
	Path   string `json:"path" binding:"required"`
}

// GatewayDeploymentResponse represents the response for successful LLM provider deployment registration
type GatewayDeploymentResponse struct {
	APIId        string `json:"apiId"`
	DeploymentId int64  `json:"deploymentId"`
	Message      string `json:"message"`
	Created      bool   `json:"created"`
}
