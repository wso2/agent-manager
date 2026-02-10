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
