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

package rbac

import "sync"

// Permission is a typed string representing an OAuth2 scope (without the resource server prefix).
type Permission string

// ResourceServer is the amp resource server's Thunder resource HANDLE — the prefix Thunder
// composes every scope string from (handle + delimiter, never the resource server's identifier),
// so this must stay "amp" regardless of what ResourceServerIdentifier is set to.
const ResourceServer = "amp"

// ResourceServerIdentifier is the amp resource server's Thunder IDENTIFIER (RFC 8707 resource
// indicator / the token's aud claim) — a SEPARATE field from ResourceServer above, because Thunder
// requires resource identifiers to be absolute URIs while ResourceServer stays the bare handle
// "amp". Must be kept in sync with 60-amp-resource-server.yaml's `identifier` in the Thunder
// extension chart. findResourceServerID must look this value up specifically, or it silently
// finds no match and every permission list comes back empty.
const ResourceServerIdentifier = "urn:wso2:amp"

// Scope returns the OAuth2 scope string for this permission as Thunder issues it (e.g. "amp:org:view").
// Thunder builds permissions as <resource-server-handle>:<resource>:<action>.
func (p Permission) Scope() string {
	return ResourceServer + ":" + string(p)
}

// MainMCPScopes returns exactly the permission scopes required by the tools
// registered on the Agent Manager MCP endpoint: the core's, and those a
// linked built-in module adds with AddMCPScopes.
func MainMCPScopes() []string {
	permissions := []Permission{
		ProjectRead,
		ProjectCreate,
		AgentRead,
		AgentCreate,
		AgentTokenManage,
		AgentBuild,
		AgentSuspend,
		AgentUpdate,
		EnvironmentRead,
	}
	mcpScopesMu.Lock()
	permissions = append(permissions, mcpScopes...)
	mcpScopesMu.Unlock()
	scopes := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		scopes = append(scopes, permission.Scope())
	}
	return scopes
}

var (
	mcpScopesMu sync.Mutex
	mcpScopes   []Permission
)

// AddMCPScopes adds the permissions of the MCP tools a built-in module
// registers on the core's MCP server through its toolsets. It is called from
// the module package's init, so only a binary that links the module
// advertises them.
func AddMCPScopes(perms ...Permission) {
	mcpScopesMu.Lock()
	defer mcpScopesMu.Unlock()
	mcpScopes = append(mcpScopes, perms...)
}

// Org permissions
const (
	OrgView                 Permission = "org:view"
	OrgModifySettings       Permission = "org:modify-settings"
	OrgInviteMember         Permission = "org:invite-member"
	OrgRemoveMember         Permission = "org:remove-member"
	OrgAssignRole           Permission = "org:assign-role"
	OrgManageIDP            Permission = "org:manage-idp"
	OrgManageServiceAccount Permission = "org:manage-service-account"
)

// Project permissions
const (
	ProjectCreate Permission = "project:create"
	ProjectRead   Permission = "project:read"
	ProjectUpdate Permission = "project:update"
	ProjectDelete Permission = "project:delete"
)

// Environment permissions
const (
	EnvironmentCreate Permission = "environment:create"
	EnvironmentRead   Permission = "environment:read"
	EnvironmentUpdate Permission = "environment:update"
	EnvironmentDelete Permission = "environment:delete"
)

// Infrastructure permissions
const (
	DataPlaneRead            Permission = "data-plane:read"
	DeploymentPipelineRead   Permission = "deployment-pipeline:read"
	DeploymentPipelineCreate Permission = "deployment-pipeline:create"
	DeploymentPipelineUpdate Permission = "deployment-pipeline:update"
	DeploymentPipelineDelete Permission = "deployment-pipeline:delete"
)

// Git secret permissions
const (
	GitSecretCreate Permission = "git-secret:create"
	GitSecretRead   Permission = "git-secret:read"
	GitSecretDelete Permission = "git-secret:delete"
)

// Agent permissions
const (
	AgentCreate      Permission = "agent:create"
	AgentRead        Permission = "agent:read"
	AgentUpdate      Permission = "agent:update"
	AgentDelete      Permission = "agent:delete"
	AgentBuild       Permission = "agent:build"
	AgentSuspend     Permission = "agent:suspend"
	AgentTokenManage Permission = "agent:token-manage"
)

// Agent Kind permissions
const (
	AgentKindRead   Permission = "agent-kind:read"
	AgentKindCreate Permission = "agent-kind:create"
	AgentKindUpdate Permission = "agent-kind:update"
	AgentKindDelete Permission = "agent-kind:delete"
)

// Events permissions — event destinations, org subscriptions and the org's
// delivery log, granted to Admin only. Project and agent subscriptions are
// part of the project or agent and use their read and update permissions.
const (
	EventsRead   Permission = "events:read"
	EventsManage Permission = "events:manage"
)

// Role management permissions
const (
	RoleCreate Permission = "role:create"
	RoleRead   Permission = "role:read"
	RoleUpdate Permission = "role:update"
	RoleDelete Permission = "role:delete"
)

// Group management permissions
const (
	GroupCreate Permission = "group:create"
	GroupRead   Permission = "group:read"
	GroupUpdate Permission = "group:update"
	GroupDelete Permission = "group:delete"
)

// Repository permissions
const (
	RepositoryRead Permission = "repository:read"
)

// Profile permissions
const (
	ProfileRead             Permission = "profile:read"
	ProfileUpdateAttributes Permission = "profile:update-attributes"
)
