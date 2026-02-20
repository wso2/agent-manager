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

// CreateAgentModelConfigRequest represents the request to create an agent model configuration
type CreateAgentModelConfigRequest struct {
	Name        string                           `json:"name" binding:"required,max=255"`
	Description string                           `json:"description,omitempty"`
	Type        string                           `json:"type" binding:"required,oneof=llm mcp other"`
	EnvMappings map[string]EnvModelConfigRequest `json:"envMappings" binding:"required,min=1"`
}

// EnvModelConfigRequest represents per-environment configuration
type EnvModelConfigRequest struct {
	ProviderUUID  uuid.UUID                `json:"providerUuid" binding:"required"`
	Configuration EnvProviderConfiguration `json:"configuration,omitempty"`
}

// EnvProviderConfiguration contains provider-specific policies
type EnvProviderConfiguration struct {
	Policies []map[string]interface{} `json:"policies,omitempty"`
}

// UpdateAgentModelConfigRequest represents the request to update an agent model configuration
type UpdateAgentModelConfigRequest struct {
	Name        string                           `json:"name,omitempty" binding:"omitempty,max=255"`
	Description string                           `json:"description,omitempty"`
	EnvMappings map[string]EnvModelConfigRequest `json:"envMappings,omitempty"`
}

// AgentModelConfigResponse represents the full configuration response
type AgentModelConfigResponse struct {
	UUID                 string                            `json:"uuid"`
	Name                 string                            `json:"name"`
	Description          string                            `json:"description,omitempty"`
	AgentID              string                            `json:"agentId"`
	Type                 string                            `json:"type"`
	OrganizationName     string                            `json:"organizationName"`
	ProjectName          string                            `json:"projectName"`
	EnvModelConfig       map[string]EnvModelConfigResponse `json:"envModelConfig"`
	EnvironmentVariables []EnvironmentVariableConfig       `json:"environmentVariables"`
	CreatedAt            time.Time                         `json:"createdAt"`
	UpdatedAt            time.Time                         `json:"updatedAt"`
}

// EnvModelConfigResponse represents environment-specific config in response
type EnvModelConfigResponse struct {
	EnvironmentUUID string        `json:"environmentUuid"`
	EnvironmentName string        `json:"environmentName"`
	LLMProxy        *LLMProxyInfo `json:"llmProxy,omitempty"`
}

// LLMProxyInfo contains proxy details exposed in response
type LLMProxyInfo struct {
	ProxyUUID string `json:"proxyUuid"`
	ProxyName string `json:"proxyName"`
	Context   string `json:"context"`
	Status    string `json:"status"`
}

// EnvironmentVariableConfig represents the variable name exposed to agent
type EnvironmentVariableConfig struct {
	Name string `json:"name"` // e.g., "MY_OPENAI_CONFIG_URL"
}

// AgentModelConfigListResponse represents paginated list
type AgentModelConfigListResponse struct {
	Configs    []AgentModelConfigListItem `json:"configs"`
	Pagination PaginationInfo             `json:"pagination"`
}

// AgentModelConfigListItem represents summary in list
type AgentModelConfigListItem struct {
	UUID             string    `json:"uuid"`
	Name             string    `json:"name"`
	Description      string    `json:"description,omitempty"`
	AgentID          string    `json:"agentId"`
	Type             string    `json:"type"`
	OrganizationName string    `json:"organizationName"`
	ProjectName      string    `json:"projectName"`
	CreatedAt        time.Time `json:"createdAt"`
}

// PaginationInfo contains pagination metadata
type PaginationInfo struct {
	Count  int `json:"count"`
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}
