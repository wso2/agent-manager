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
// registered on the Agent Manager MCP endpoint.
func MainMCPScopes() []string {
	permissions := []Permission{
		ProjectRead,
		ProjectCreate,
		AgentRead,
		AgentCreate,
		AgentTokenManage,
		AgentBuild,
		AgentEnvNonProduction,
		AgentSuspend,
		EnvironmentRead,
	}
	scopes := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		scopes = append(scopes, permission.Scope())
	}
	return scopes
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

// Gateway permissions
const (
	GatewayCreate      Permission = "gateway:create"
	GatewayRead        Permission = "gateway:read"
	GatewayUpdate      Permission = "gateway:update"
	GatewayDelete      Permission = "gateway:delete"
	GatewayTokenManage Permission = "gateway:token-manage"
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

// LLM provider template permissions
const (
	LLMProviderTemplateCreate Permission = "llm-provider-template:create"
	LLMProviderTemplateRead   Permission = "llm-provider-template:read"
	LLMProviderTemplateUpdate Permission = "llm-provider-template:update"
	LLMProviderTemplateDelete Permission = "llm-provider-template:delete"
)

// LLM provider permissions
const (
	LLMProviderCreate             Permission = "llm-provider:create"
	LLMProviderRead               Permission = "llm-provider:read"
	LLMProviderUpdate             Permission = "llm-provider:update"
	LLMProviderDelete             Permission = "llm-provider:delete"
	LLMProviderConfigureGuardrail Permission = "llm-provider:configure-guardrail"
	LLMProviderConnect            Permission = "llm-provider:connect"
	LLMProviderDeploy             Permission = "llm-provider:deploy"
	LLMProviderAPIKeyManage       Permission = "llm-provider:api-key-manage"
)

// MCP server permissions
const (
	MCPServerCreate             Permission = "mcp-server:create"
	MCPServerRead               Permission = "mcp-server:read"
	MCPServerUpdate             Permission = "mcp-server:update"
	MCPServerDelete             Permission = "mcp-server:delete"
	MCPServerConfigureGuardrail Permission = "mcp-server:configure-guardrail"
	MCPServerConnect            Permission = "mcp-server:connect"
	MCPServerAPIKeyManage       Permission = "mcp-server:api-key-manage"
)

// Scope catalog permissions
const (
	ScopeCreate Permission = "scope:create"
	ScopeRead   Permission = "scope:read"
	ScopeUpdate Permission = "scope:update"
	ScopeDelete Permission = "scope:delete"
)

// Agent identity (env-Thunder agent groups/roles) permissions
const (
	AgentIdentityRead   Permission = "agent-identity:read"
	AgentIdentityCreate Permission = "agent-identity:create"
	AgentIdentityUpdate Permission = "agent-identity:update"
	AgentIdentityDelete Permission = "agent-identity:delete"
)

// LLM proxy permissions
const (
	LLMProxyCreate       Permission = "llm-proxy:create"
	LLMProxyRead         Permission = "llm-proxy:read"
	LLMProxyUpdate       Permission = "llm-proxy:update"
	LLMProxyDelete       Permission = "llm-proxy:delete"
	LLMProxyDeploy       Permission = "llm-proxy:deploy"
	LLMProxyAPIKeyManage Permission = "llm-proxy:api-key-manage"
)

// Evaluator permissions
const (
	EvaluatorCreate Permission = "evaluator:create"
	EvaluatorRead   Permission = "evaluator:read"
	EvaluatorUpdate Permission = "evaluator:update"
	EvaluatorDelete Permission = "evaluator:delete"
)

// Agent permissions
const (
	AgentCreate Permission = "agent:create"
	AgentRead   Permission = "agent:read"
	AgentUpdate Permission = "agent:update"
	AgentDelete Permission = "agent:delete"
	AgentBuild  Permission = "agent:build"
	// The environment tier is an authorization axis of its own, about where an
	// action lands rather than what it is. AgentEnvNonProduction is the floor —
	// "may act on environments at all" — and AgentEnvProduction is held in
	// addition to it to reach the environments OpenChoreo flags isProduction.
	// The production grant is never sufficient on its own: every surface
	// declares the floor statically and denies before the tier is evaluated.
	AgentEnvNonProduction Permission = "agent:env-non-production"
	AgentEnvProduction    Permission = "agent:env-production"
	AgentRollback         Permission = "agent:rollback"
	AgentSuspend          Permission = "agent:suspend"
	AgentTokenManage      Permission = "agent:token-manage"
	AgentAPIKeyManage     Permission = "agent:api-key-manage"
)

// Agent Kind permissions
const (
	AgentKindRead   Permission = "agent-kind:read"
	AgentKindCreate Permission = "agent-kind:create"
	AgentKindUpdate Permission = "agent-kind:update"
	AgentKindDelete Permission = "agent-kind:delete"
)

// Monitor permissions
const (
	MonitorCreate       Permission = "monitor:create"
	MonitorRead         Permission = "monitor:read"
	MonitorUpdate       Permission = "monitor:update"
	MonitorDelete       Permission = "monitor:delete"
	MonitorExecute      Permission = "monitor:execute"
	MonitorScoreRead    Permission = "monitor:score-read"
	MonitorScorePublish Permission = "monitor:score-publish"
)

// Alerting permissions — org-level webhook endpoints, granted to Admin only.
// Project and agent webhooks are part of the project or agent and use their
// read and update permissions.
const (
	AlertingRead   Permission = "alerting:read"
	AlertingManage Permission = "alerting:manage"
)

// Observability permissions — data-read scopes enforced by agent-manager-observer
const (
	ObservabilityTraceRead    Permission = "observability:trace-read"
	ObservabilityLogRead      Permission = "observability:log-read"
	ObservabilityBuildLogRead Permission = "observability:build-log-read"
	ObservabilityMetricRead   Permission = "observability:metric-read"
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

// Catalog and repository permissions
const (
	CatalogRead    Permission = "catalog:read"
	RepositoryRead Permission = "repository:read"
)

// Profile permissions
const (
	ProfileRead             Permission = "profile:read"
	ProfileUpdateAttributes Permission = "profile:update-attributes"
)
