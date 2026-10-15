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

// MCPProxy represents an MCP proxy entity.
type MCPProxy struct {
	UUID          uuid.UUID      `gorm:"column:uuid;primaryKey" json:"uuid"`
	Description   string         `gorm:"column:description" json:"description,omitempty"`
	CreatedBy     string         `gorm:"column:created_by" json:"createdBy,omitempty"`
	Status        string         `gorm:"column:status" json:"status"`
	Configuration MCPProxyConfig `gorm:"column:configuration;type:jsonb;serializer:json" json:"configuration"`

	Artifact  *Artifact          `gorm:"foreignKey:UUID;references:UUID" json:"artifact,omitempty"`
	Endpoints []MCPProxyEndpoint `gorm:"foreignKey:MCPProxyUUID;references:UUID;constraint:OnDelete:CASCADE" json:"endpoints,omitempty"`

	OrganizationName string    `gorm:"-" json:"organizationName,omitempty"`
	ID               string    `gorm:"-" json:"id,omitempty"`
	Name             string    `gorm:"-" json:"name,omitempty"`
	Handle           string    `gorm:"-" json:"handle,omitempty"`
	Version          string    `gorm:"-" json:"version,omitempty"`
	CreatedAt        time.Time `gorm:"-" json:"createdAt,omitempty"`
	UpdatedAt        time.Time `gorm:"-" json:"updatedAt,omitempty"`
}

// TableName returns the table name for the MCPProxy model.
func (MCPProxy) TableName() string {
	return "mcp_proxies"
}

// MCPProxyMapping represents an agent-scoped MCP proxy mapping deployment.
type MCPProxyMapping struct {
	UUID               uuid.UUID      `gorm:"column:uuid;primaryKey" json:"uuid"`
	SourceMCPProxyUUID uuid.UUID      `gorm:"column:source_mcp_proxy_uuid" json:"sourceMcpProxyUuid"`
	Description        string         `gorm:"column:description" json:"description,omitempty"`
	Status             string         `gorm:"column:status" json:"status"`
	Configuration      MCPProxyConfig `gorm:"column:configuration;type:jsonb;serializer:json" json:"configuration"`

	Artifact       *Artifact `gorm:"foreignKey:UUID;references:UUID" json:"artifact,omitempty"`
	SourceMCPProxy *MCPProxy `gorm:"foreignKey:SourceMCPProxyUUID;references:UUID" json:"sourceMcpProxy,omitempty"`
}

// TableName returns the table name for the MCPProxyMapping model.
func (MCPProxyMapping) TableName() string {
	return "mcp_proxy_mappings"
}

// MCPProxyConfig represents the MCP proxy configuration stored in JSON.
//
// The config carries two shapes. On the parent org-level MCPProxy it holds shared
// metadata only (Name/Version/Context/Vhost/SpecVersion) — per-endpoint deployable
// config lives on the endpoint rows (MCPProxyEndpoint.Configuration). An agent-scoped
// MCPProxyMapping is an actual gateway-deployable entity: it populates the flat
// root-level fields (flattened from the bound endpoint's config by
// buildAgentMCPConfigProxy). The deployment YAML builder reads only the flat
// root-level fields.
type MCPProxyConfig struct {
	Name         string                `json:"name,omitempty"`
	Version      string                `json:"version,omitempty"`
	Context      *string               `json:"context,omitempty"`
	Vhost        *string               `json:"vhost,omitempty"`
	SpecVersion  string                `json:"specVersion,omitempty"`
	Upstream     UpstreamConfig        `json:"upstream,omitempty"`
	Resilience   *Resilience           `json:"resilience,omitempty"`
	Policies     []MCPPolicy           `json:"policies,omitempty"`
	Capabilities *MCPProxyCapabilities `json:"capabilities,omitempty"`
	Security     *SecurityConfig       `json:"security,omitempty"`
}

// MCPProxyEndpoint is the deployable proxy definition. One endpoint can be deployed
// to 1..N environments through MCPProxyEndpointEnvironment. Within a parent proxy an
// endpoint handle is unique (uq_mcp_endpoint_handle) and an environment maps to at
// most one endpoint (uq_proxy_env_single on the join table).
type MCPProxyEndpoint struct {
	UUID          uuid.UUID         `gorm:"column:uuid;primaryKey" json:"uuid"`
	MCPProxyUUID  uuid.UUID         `gorm:"column:mcp_proxy_uuid" json:"mcpProxyUuid"`
	Handle        string            `gorm:"column:handle" json:"handle"`
	Name          string            `gorm:"column:name" json:"name,omitempty"`
	Status        string            `gorm:"column:status" json:"status"`
	Configuration MCPEndpointConfig `gorm:"column:configuration;type:jsonb;serializer:json" json:"configuration"`
	CreatedAt     time.Time         `gorm:"column:created_at" json:"createdAt,omitempty"`
	UpdatedAt     time.Time         `gorm:"column:updated_at" json:"updatedAt,omitempty"`

	Environments []MCPProxyEndpointEnvironment `gorm:"foreignKey:EndpointUUID;references:UUID;constraint:OnDelete:CASCADE" json:"environments,omitempty"`
}

// TableName returns the table name for the MCPProxyEndpoint model.
func (MCPProxyEndpoint) TableName() string {
	return "mcp_proxy_endpoints"
}

// MCPProxyEndpointEnvironment maps one endpoint to one environment and holds the
// stable gateway artifact identity (ArtifactUUID) and deployment status for that
// (endpoint, environment) deployment. MCPProxyUUID is denormalized from the endpoint
// so the DB can enforce uq_proxy_env_single (one endpoint per environment per proxy).
type MCPProxyEndpointEnvironment struct {
	ID              uint      `gorm:"column:id;primaryKey;autoIncrement" json:"id,omitempty"`
	MCPProxyUUID    uuid.UUID `gorm:"column:mcp_proxy_uuid" json:"mcpProxyUuid"`
	EndpointUUID    uuid.UUID `gorm:"column:endpoint_uuid" json:"endpointUuid"`
	EnvironmentUUID uuid.UUID `gorm:"column:environment_uuid" json:"environmentUuid"`
	ArtifactUUID    uuid.UUID `gorm:"column:artifact_uuid" json:"artifactUuid"`
	Status          string    `gorm:"column:status" json:"status"`
	CreatedAt       time.Time `gorm:"column:created_at" json:"createdAt,omitempty"`
	// RequestedGatewayUUID is the caller's explicit egress gateway choice for this
	// binding. Request-scoped only: placement is derived from deployment_status on read,
	// so there is nothing to persist. gorm:"-" keeps it out of every query.
	RequestedGatewayUUID *string `gorm:"-" json:"-"`
}

// TableName returns the table name for the MCPProxyEndpointEnvironment model.
func (MCPProxyEndpointEnvironment) TableName() string {
	return "mcp_proxy_endpoint_environments"
}

// MCPEndpointConfig is the deployable configuration stored on one endpoint's
// configuration JSONB. Upstream holds the single backend endpoint (URL + auth) the
// endpoint proxies. The stable gateway artifact identity for a given (endpoint,
// environment) deployment no longer lives here — it is on the join row
// (MCPProxyEndpointEnvironment.ArtifactUUID). Agent MCP configurations do not deploy
// their own artifacts; they reference the endpoint's per-environment artifact.
type MCPEndpointConfig struct {
	Upstream     *UpstreamEndpoint     `json:"upstream,omitempty"`
	Resilience   *Resilience           `json:"resilience,omitempty"`
	Policies     []MCPPolicy           `json:"policies,omitempty"`
	Capabilities *MCPProxyCapabilities `json:"capabilities,omitempty"`
	Security     *SecurityConfig       `json:"security,omitempty"`
}

// MCP proxy per-environment deployment status values reported on read.
const (
	MCPDeploymentStatusDeployed   = "Deployed"
	MCPDeploymentStatusUndeployed = "Undeployed"
)

// MCPPolicy represents a policy attached to an MCP proxy.
type MCPPolicy struct {
	Name               string                 `json:"name" yaml:"name"`
	Version            string                 `json:"version" yaml:"version"`
	DisplayName        string                 `json:"displayName,omitempty" yaml:"displayName,omitempty"`
	ExecutionCondition *string                `json:"executionCondition,omitempty" yaml:"executionCondition,omitempty"`
	Params             map[string]interface{} `json:"params,omitempty" yaml:"params,omitempty"`
}

// MCPProxyCapabilities contains the MCP server capabilities discovered from the upstream.
type MCPProxyCapabilities struct {
	Prompts   *[]map[string]interface{} `json:"prompts,omitempty"`
	Resources *[]map[string]interface{} `json:"resources,omitempty"`
	Tools     *[]map[string]interface{} `json:"tools,omitempty"`
}

// MCPProxyDTO is the request/response body for an MCP proxy.
type MCPProxyDTO struct {
	Context        *string               `json:"context,omitempty"`
	CreatedAt      *time.Time            `json:"createdAt,omitempty"`
	CreatedBy      *string               `json:"createdBy,omitempty"`
	Description    *string               `json:"description,omitempty"`
	Gateways       []string              `json:"gateways,omitempty"`
	ID             string                `json:"id"`
	InCatalog      bool                  `json:"inCatalog"`
	McpSpecVersion *string               `json:"mcpSpecVersion,omitempty"`
	Name           string                `json:"name"`
	Endpoints      []MCPProxyEndpointDTO `json:"endpoints,omitempty"`
	UpdatedAt      *time.Time            `json:"updatedAt,omitempty"`
	Version        string                `json:"version"`
	Vhost          *string               `json:"vhost,omitempty"`
}

// MCPProxyEndpointDTO is one endpoint in an MCP proxy request/response body. On write,
// Environments carries target environment UUIDs (DeploymentStatus ignored). On read,
// each entry reports the (endpoint, environment) deployment status.
type MCPProxyEndpointDTO struct {
	ID           string                      `json:"id"`
	Name         string                      `json:"name,omitempty"`
	Upstream     UpstreamConfig              `json:"upstream"`
	Resilience   *Resilience                 `json:"resilience,omitempty"`
	Policies     *[]MCPPolicy                `json:"policies,omitempty"`
	Capabilities *MCPProxyCapabilities       `json:"capabilities,omitempty"`
	Security     *SecurityConfig             `json:"security,omitempty"`
	Environments []MCPEndpointEnvironmentDTO `json:"environments,omitempty"`
}

// MCPEndpointEnvironmentDTO is one endpoint→environment binding. EnvironmentUUID is
// the target environment; DeploymentStatus is response-only (Deployed/Undeployed).
// GatewayID is the caller's explicit egress gateway choice on write (optional: inferred
// when the environment has exactly one egress gateway, required when it has more than
// one; placement is fixed once deployed) and is echoed back on read as the gateway the
// binding is actually deployed to.
type MCPEndpointEnvironmentDTO struct {
	EnvironmentUUID  string  `json:"environmentUuid"`
	DeploymentStatus string  `json:"deploymentStatus,omitempty"`
	GatewayID        *string `json:"gatewayId,omitempty"`
}

// MCPPolicyAvailabilityResponse lists the MCP policies reported by active gateways.
type MCPPolicyAvailabilityResponse struct {
	Count int32                    `json:"count"`
	List  []MCPPolicyAvailableItem `json:"list"`
}

// MCPPolicyAvailableItem is one gateway-installed MCP policy, enriched with policy hub
// metadata. Version is the major version ("v1") MCP proxy policy references use.
type MCPPolicyAvailableItem struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	DisplayName string   `json:"displayName,omitempty"`
	Description string   `json:"description,omitempty"`
	Categories  []string `json:"categories,omitempty"`
	// RequiredSystemConfig lists gateway config keys the policy needs set (no default).
	RequiredSystemConfig []string               `json:"requiredSystemConfig,omitempty"`
	Parameters           map[string]interface{} `json:"parameters,omitempty"`
	SystemParameters     map[string]interface{} `json:"systemParameters,omitempty"`
}

// MCPProxyListItem is the list representation for an MCP proxy.
type MCPProxyListItem struct {
	Context        *string    `json:"context,omitempty"`
	CreatedAt      *time.Time `json:"createdAt,omitempty"`
	CreatedBy      *string    `json:"createdBy,omitempty"`
	Description    *string    `json:"description,omitempty"`
	ID             *string    `json:"id,omitempty"`
	McpSpecVersion *string    `json:"mcpSpecVersion,omitempty"`
	Name           *string    `json:"name,omitempty"`
	Status         *string    `json:"status,omitempty"`
	UpdatedAt      *time.Time `json:"updatedAt,omitempty"`
	Version        *string    `json:"version,omitempty"`
}

// MCPProxyListResponse is the response body for listing MCP proxies.
type MCPProxyListResponse struct {
	Count      int                `json:"count"`
	List       []MCPProxyListItem `json:"list"`
	Pagination PaginationInfo     `json:"pagination"`
}

// MCPServerInfoFetchRequest is the request body for fetching MCP server details.
type MCPServerInfoFetchRequest struct {
	Auth    *UpstreamAuth `json:"auth,omitempty"`
	ProxyID *string       `json:"proxyId,omitempty"`
	URL     *string       `json:"url,omitempty"`
}

// MCPServerInfoFetchResponse contains the MCP server metadata and capabilities.
type MCPServerInfoFetchResponse struct {
	Prompts    *[]map[string]interface{} `json:"prompts,omitempty"`
	Resources  *[]map[string]interface{} `json:"resources,omitempty"`
	ServerInfo *map[string]interface{}   `json:"serverInfo,omitempty"`
	Tools      *[]map[string]interface{} `json:"tools,omitempty"`
}
