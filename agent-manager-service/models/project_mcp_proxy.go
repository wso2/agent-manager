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

// MCPProxyArtifactName is the artifacts.name of an MCP proxy. An org-level proxy uses its
// display name as is. A project proxy uses its handle, which is unique in the org, so
// uq_artifact_name_version_ou_id (unique across every artifact kind) never rejects a display
// name that another proxy, in this project or another, already uses. Its display name is
// kept in its configuration; MCPProxyGatewayDisplayName is what the gateway shows.
func MCPProxyArtifactName(projectName *string, handle, name string) string {
	if projectName == nil || *projectName == "" {
		return name
	}
	return handle
}

// MCPProxyGatewayDisplayName is the display name a proxy's gateway artifact carries: an
// org-level proxy's display name, or "<project> <name>" for a project proxy.
func MCPProxyGatewayDisplayName(projectName *string, name string) string {
	if projectName == nil || *projectName == "" {
		return name
	}
	return *projectName + " " + name
}

// ProjectMCPProxyRequest is the body of PUT /orgs/{org}/projects/{project}/mcp-proxies/{proxyId}.
type ProjectMCPProxyRequest struct {
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	Version        string `json:"version,omitempty"`
	Context        string `json:"context,omitempty"`
	McpSpecVersion string `json:"mcpSpecVersion,omitempty"`
	// Source and Upstream are exclusive: a proxy fronts either a component endpoint of the project, whose
	// in-cluster address each binding resolves from the component's release binding, or a public URL that
	// every binding uses as it is.
	Source   *MCPProxySource          `json:"source,omitempty"`
	Upstream *ProjectMCPProxyUpstream `json:"upstream,omitempty"`
}

// ProjectMCPProxyUpstream is the public upstream a project proxy registered by URL fronts.
type ProjectMCPProxyUpstream struct {
	URL string `json:"url"`
}

// MCPProxyEnvironmentRequest is the body of PUT .../mcp-proxies/{proxyId}/environments/{envName}.
// It replaces the binding's configuration: an omitted field resets to its default.
type MCPProxyEnvironmentRequest struct {
	GatewayID *string `json:"gatewayId,omitempty"`
	// ReleaseBindingName picks the source component's release binding when it has more than one in the
	// environment, one per deployment track for example.
	ReleaseBindingName *string                          `json:"releaseBindingName,omitempty"`
	Security           *MCPProxyEnvironmentSecurity     `json:"security,omitempty"`
	Policies           []MCPPolicy                      `json:"policies,omitempty"`
	UpstreamAuth       *MCPProxyEnvironmentUpstreamAuth `json:"upstreamAuth,omitempty"`
	// Capabilities is the MCP server's tools, prompts and resources in this environment, stored as given.
	Capabilities *MCPProxyCapabilities `json:"capabilities,omitempty"`
	// Resilience is the binding's request and idle timeouts on the gateway; omitted, the gateway's defaults.
	Resilience *Resilience `json:"resilience,omitempty"`
}

// MCPProxyEnvironmentSecurity is a binding's SecurityConfig: SecurityConfig with a jwt variant.
// Phase 1 accepts only apiKey.
type MCPProxyEnvironmentSecurity struct {
	Enabled  *bool                `json:"enabled,omitempty"`
	APIKey   *APIKeySecurity      `json:"apiKey,omitempty"`
	JWT      *MCPProxyJWTSecurity `json:"jwt,omitempty"`
	Identity *IdentitySecurity    `json:"identity,omitempty"`
}

// MCPProxyJWTSecurity is the jwt security variant (not available yet).
type MCPProxyJWTSecurity struct {
	Enabled   *bool    `json:"enabled,omitempty"`
	Issuers   []string `json:"issuers,omitempty"`
	Audiences []string `json:"audiences,omitempty"`
}

// MCPProxyEnvironmentUpstreamAuth is the credential the gateway adds when it calls the
// upstream. Type is "api-key" (Header and Value required) or "none".
type MCPProxyEnvironmentUpstreamAuth struct {
	Type   string `json:"type"`
	Header string `json:"header,omitempty"`
	Value  string `json:"value,omitempty"`
}

// MCPProxyEnvironmentUpstreamAuthRedacted is the upstream auth as returned: never the value.
type MCPProxyEnvironmentUpstreamAuthRedacted struct {
	Type   string `json:"type"`
	Header string `json:"header,omitempty"`
}

// MCPProxyEnvironmentResponse is one environment binding of a project proxy.
type MCPProxyEnvironmentResponse struct {
	EnvironmentName string `json:"environmentName"`
	EnvironmentUUID string `json:"environmentUuid"`
	// DeploymentStatus is absent while the binding has no deployment_status row (no gateway).
	DeploymentStatus string `json:"deploymentStatus,omitempty"`
	// DeploymentStatusReason is the ack's error code or the render reason, or NO_GATEWAY when
	// DeploymentStatus is absent.
	DeploymentStatusReason string `json:"deploymentStatusReason,omitempty"`
	GatewayID              string `json:"gatewayId,omitempty"`
	// ReleaseBindingName is the release binding the binding was given, if any.
	ReleaseBindingName string `json:"releaseBindingName,omitempty"`
	// Context reads back as the proxy was given it, without the project prefix.
	Context string `json:"context"`
	// PublicURL is the gateway URL, with the /<project><context> path the gateway serves.
	PublicURL    string                                   `json:"publicUrl,omitempty"`
	Security     *MCPProxyEnvironmentSecurity             `json:"security,omitempty"`
	Policies     []MCPPolicy                              `json:"policies,omitempty"`
	UpstreamAuth *MCPProxyEnvironmentUpstreamAuthRedacted `json:"upstreamAuth,omitempty"`
	Capabilities *MCPProxyCapabilities                    `json:"capabilities,omitempty"`
	Resilience   *Resilience                              `json:"resilience,omitempty"`
	// CapabilitiesSource says where Capabilities came from: "caller" (the binding PUT) or "discovered" (AMS listed
	// them from the MCP server). Absent with no capabilities.
	CapabilitiesSource string `json:"capabilitiesSource,omitempty"`
	// CapabilityDiscovery is AMS's discovery of the binding's capabilities, when one was queued.
	CapabilityDiscovery *MCPCapabilityDiscoveryStatus `json:"capabilityDiscovery,omitempty"`
	UpdatedAt           *time.Time                    `json:"updatedAt,omitempty"`
}

// Where a binding's capabilities came from.
const (
	MCPCapabilitiesSourceCaller     = "caller"
	MCPCapabilitiesSourceDiscovered = "discovered"
)

// MCPDeploymentStatusReasonNoGateway is the reason a binding shows while its environment has no gateway.
const MCPDeploymentStatusReasonNoGateway = "NO_GATEWAY"

// ProjectMCPProxyResponse is the project proxy as returned by upsert and get.
type ProjectMCPProxyResponse struct {
	ID             string                        `json:"id"`
	Name           string                        `json:"name"`
	Description    string                        `json:"description,omitempty"`
	Version        string                        `json:"version"`
	Context        string                        `json:"context"`
	McpSpecVersion string                        `json:"mcpSpecVersion,omitempty"`
	ProjectName    string                        `json:"projectName"`
	Source         *MCPProxySource               `json:"source,omitempty"`
	Upstream       *ProjectMCPProxyUpstream      `json:"upstream,omitempty"`
	Environments   []MCPProxyEnvironmentResponse `json:"environments"`
	CreatedBy      string                        `json:"createdBy,omitempty"`
	CreatedAt      *time.Time                    `json:"createdAt,omitempty"`
	UpdatedAt      *time.Time                    `json:"updatedAt,omitempty"`
}

// ProjectMCPProxyEnvironmentSummary is one binding in a list item.
type ProjectMCPProxyEnvironmentSummary struct {
	EnvironmentName        string `json:"environmentName"`
	DeploymentStatus       string `json:"deploymentStatus,omitempty"`
	DeploymentStatusReason string `json:"deploymentStatusReason,omitempty"`
	PublicURL              string `json:"publicUrl,omitempty"`
}

// ProjectMCPProxyListItem is one proxy in a project list.
type ProjectMCPProxyListItem struct {
	ID           string                              `json:"id"`
	Name         string                              `json:"name"`
	Description  string                              `json:"description,omitempty"`
	Version      string                              `json:"version"`
	Context      string                              `json:"context"`
	Source       *MCPProxySource                     `json:"source,omitempty"`
	Upstream     *ProjectMCPProxyUpstream            `json:"upstream,omitempty"`
	Environments []ProjectMCPProxyEnvironmentSummary `json:"environments"`
	CreatedAt    *time.Time                          `json:"createdAt,omitempty"`
	UpdatedAt    *time.Time                          `json:"updatedAt,omitempty"`
}

// ProjectMCPProxyListResponse is the response of GET .../projects/{project}/mcp-proxies.
type ProjectMCPProxyListResponse struct {
	Count      int                       `json:"count"`
	List       []ProjectMCPProxyListItem `json:"list"`
	Pagination PaginationInfo            `json:"pagination"`
}
