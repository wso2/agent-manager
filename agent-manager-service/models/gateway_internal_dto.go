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

import "time"

// DeploymentNotification represents the request body for gateway API deployment registration
type DeploymentNotification struct {
	ID                string           `json:"id" binding:"required"`
	Configuration     APIConfiguration `json:"configuration" binding:"required"`
	Status            string           `json:"status" binding:"required"`
	CreatedAt         time.Time        `json:"createdAt" binding:"required"`
	UpdatedAt         time.Time        `json:"updatedAt" binding:"required"`
	DeployedAt        *time.Time       `json:"deployedAt,omitempty"`
	DeployedVersion   *int             `json:"deployedVersion,omitempty"`
	ProjectIdentifier string           `json:"projectIdentifier" binding:"required"`
}

// APIConfiguration represents the API configuration
type APIConfiguration struct {
	Version string        `json:"version" yaml:"version" binding:"required"`
	Kind    string        `json:"kind" yaml:"kind" binding:"required"`
	Spec    APIConfigData `json:"spec" yaml:"spec" binding:"required"`
}

// APIConfigData represents the detailed API configuration
type APIConfigData struct {
	Name        string           `json:"name" yaml:"name" binding:"required"`
	Version     string           `json:"version" yaml:"version" binding:"required"`
	Context     string           `json:"context" yaml:"context" binding:"required"`
	ProjectName string           `json:"projectName,omitempty" yaml:"projectName,omitempty"`
	Upstreams   []UpstreamSimple `json:"upstreams" yaml:"upstream" binding:"required"`
	Operations  []BasicOperation `json:"operations" yaml:"operations" binding:"required"`
}

// UpstreamSimple represents basic backend service configuration for internal DTOs
type UpstreamSimple struct {
	URL string `json:"url" binding:"required"`
}

// BasicOperation represents API basic operation configuration
type BasicOperation struct {
	Method string `json:"method" binding:"required"`
	Path   string `json:"path" binding:"required"`
}

// GatewayDeploymentResponse represents the response for successful API deployment registration
type GatewayDeploymentResponse struct {
	APIId        string `json:"apiId"`
	DeploymentId int64  `json:"deploymentId"`
	Message      string `json:"message"`
	Created      bool   `json:"created"`
}

// DeployAPIRequest represents the request to deploy an API
type DeployAPIRequest struct {
	Name       string                 `json:"name" binding:"required"`
	Base       string                 `json:"base" binding:"required"` // "current" or deployment ID
	GatewayID  string                 `json:"gatewayId" binding:"required"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	Properties map[string]interface{} `json:"properties,omitempty"`
}

// ConnectionAckDTO represents WebSocket connection acknowledgment
type ConnectionAckDTO struct {
	Type      string `json:"type"`
	Message   string `json:"message"`
	GatewayID string `json:"gatewayId"`
}
