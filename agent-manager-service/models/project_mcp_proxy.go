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
// display name as is. A project proxy is "<project> <name>", which is also the gateway
// display name and keeps uq_artifact_name_version_ou_id from rejecting two projects that
// use the same display name.
func MCPProxyArtifactName(projectName *string, name string) string {
	if projectName == nil || *projectName == "" {
		return name
	}
	return *projectName + " " + name
}

// ProjectMCPProxyRequest is the body of PUT /orgs/{org}/projects/{project}/mcp-proxies/{proxyId}.
type ProjectMCPProxyRequest struct {
	Name           string         `json:"name"`
	Description    string         `json:"description,omitempty"`
	Version        string         `json:"version,omitempty"`
	Context        string         `json:"context,omitempty"`
	McpSpecVersion string         `json:"mcpSpecVersion,omitempty"`
	Source         MCPProxySource `json:"source"`
}

// MCPProxyEnvironmentResponse is one environment binding of a project proxy.
// Populated by the binding routes; empty until a binding exists.
type MCPProxyEnvironmentResponse struct {
	EnvironmentName string `json:"environmentName"`
}

// ProjectMCPProxyResponse is the project proxy as returned by upsert and get.
type ProjectMCPProxyResponse struct {
	ID             string                        `json:"id"`
	Name           string                        `json:"name"`
	Description    string                        `json:"description,omitempty"`
	Version        string                        `json:"version"`
	Context        string                        `json:"context"`
	McpSpecVersion string                        `json:"mcpSpecVersion,omitempty"`
	ProjectName    string                        `json:"projectName"`
	Source         MCPProxySource                `json:"source"`
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
	Source       MCPProxySource                      `json:"source"`
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
