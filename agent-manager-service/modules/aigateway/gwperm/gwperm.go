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

// Package gwperm holds the AI Gateway module's permissions. The module
// declares them in its manifest as <resource>.<action>; the host names them
// ai-gateway:<resource>.<action>, which ThunderID issues as the
// amp:ai-gateway:<resource>.<action> scopes.
package gwperm

import "github.com/wso2/agent-manager/agent-manager-service/rbac"

// Module is the name of the module that owns these permissions.
const Module = "ai-gateway"

// The module's permissions, by their full names.
const (
	Create      rbac.Permission = "ai-gateway:gateways.create"
	Read        rbac.Permission = "ai-gateway:gateways.read"
	Update      rbac.Permission = "ai-gateway:gateways.update"
	Delete      rbac.Permission = "ai-gateway:gateways.delete"
	TokenManage rbac.Permission = "ai-gateway:gateways.token-manage"

	LLMProviderCreate             rbac.Permission = "ai-gateway:llm-providers.create"
	LLMProviderRead               rbac.Permission = "ai-gateway:llm-providers.read"
	LLMProviderUpdate             rbac.Permission = "ai-gateway:llm-providers.update"
	LLMProviderDelete             rbac.Permission = "ai-gateway:llm-providers.delete"
	LLMProviderConfigureGuardrail rbac.Permission = "ai-gateway:llm-providers.configure-guardrail"
	LLMProviderConnect            rbac.Permission = "ai-gateway:llm-providers.connect"
	LLMProviderDeploy             rbac.Permission = "ai-gateway:llm-providers.deploy"
	LLMProviderAPIKeyManage       rbac.Permission = "ai-gateway:llm-providers.api-key-manage"

	LLMProviderTemplateCreate rbac.Permission = "ai-gateway:llm-provider-templates.create"
	LLMProviderTemplateRead   rbac.Permission = "ai-gateway:llm-provider-templates.read"
	LLMProviderTemplateUpdate rbac.Permission = "ai-gateway:llm-provider-templates.update"
	LLMProviderTemplateDelete rbac.Permission = "ai-gateway:llm-provider-templates.delete"

	LLMProxyCreate       rbac.Permission = "ai-gateway:llm-proxies.create"
	LLMProxyRead         rbac.Permission = "ai-gateway:llm-proxies.read"
	LLMProxyUpdate       rbac.Permission = "ai-gateway:llm-proxies.update"
	LLMProxyDelete       rbac.Permission = "ai-gateway:llm-proxies.delete"
	LLMProxyDeploy       rbac.Permission = "ai-gateway:llm-proxies.deploy"
	LLMProxyAPIKeyManage rbac.Permission = "ai-gateway:llm-proxies.api-key-manage"

	MCPServerCreate             rbac.Permission = "ai-gateway:mcp-servers.create"
	MCPServerRead               rbac.Permission = "ai-gateway:mcp-servers.read"
	MCPServerUpdate             rbac.Permission = "ai-gateway:mcp-servers.update"
	MCPServerDelete             rbac.Permission = "ai-gateway:mcp-servers.delete"
	MCPServerConfigureGuardrail rbac.Permission = "ai-gateway:mcp-servers.configure-guardrail"
	MCPServerConnect            rbac.Permission = "ai-gateway:mcp-servers.connect"
	MCPServerAPIKeyManage       rbac.Permission = "ai-gateway:mcp-servers.api-key-manage"

	ScopeCreate rbac.Permission = "ai-gateway:scopes.create"
	ScopeRead   rbac.Permission = "ai-gateway:scopes.read"
	ScopeUpdate rbac.Permission = "ai-gateway:scopes.update"
	ScopeDelete rbac.Permission = "ai-gateway:scopes.delete"

	CatalogRead rbac.Permission = "ai-gateway:catalog.read"

	AgentIdentityCreate rbac.Permission = "ai-gateway:agent-identities.create"
	AgentIdentityRead   rbac.Permission = "ai-gateway:agent-identities.read"
	AgentIdentityUpdate rbac.Permission = "ai-gateway:agent-identities.update"
	AgentIdentityDelete rbac.Permission = "ai-gateway:agent-identities.delete"

	AgentAPIKeyManage rbac.Permission = "ai-gateway:agent-api-keys.manage"

	// The environment tier is an authorization axis of its own, about where
	// an action lands rather than what it is. DeployNonProduction is the
	// floor ("may act on environments at all") and DeployProduction is held
	// in addition to it to reach environments flagged isProduction.
	DeployNonProduction rbac.Permission = "ai-gateway:deploy.non-production"
	DeployProduction    rbac.Permission = "ai-gateway:deploy.production"
)

// Action is a permission's name within the module, with its ThunderID
// display name and what it lets a caller do.
type Action struct{ Name, DisplayName, Description string }

// Actions are the module's permissions, in the order ThunderID lists them.
var Actions = []Action{
	{"gateways.create", "Create Gateways", "Register a gateway"},
	{"gateways.read", "Read Gateways", "View gateway details and status"},
	{"gateways.update", "Update Gateways", "Update gateway configuration and environments"},
	{"gateways.delete", "Delete Gateways", "Delete a gateway"},
	{"gateways.token-manage", "Manage Gateway Tokens", "Create, list, and delete gateway tokens"},
	{"llm-providers.create", "Create LLM Providers", "Create an LLM provider"},
	{"llm-providers.read", "Read LLM Providers", "View LLM providers and deployments"},
	{"llm-providers.update", "Update LLM Providers", "Update an LLM provider"},
	{"llm-providers.delete", "Delete LLM Providers", "Delete an LLM provider"},
	{"llm-providers.configure-guardrail", "Configure LLM Provider Guardrails", "Configure guardrails, rate limits, and budgets"},
	{"llm-providers.connect", "Connect LLM Providers", "Connect an agent to an LLM provider"},
	{"llm-providers.deploy", "Deploy LLM Providers", "Deploy, undeploy, and restore an LLM provider"},
	{"llm-providers.api-key-manage", "Manage LLM Provider API Keys", "Create, update, and delete LLM provider API keys"},
	{"llm-provider-templates.create", "Create LLM Provider Templates", "Create a provider template"},
	{"llm-provider-templates.read", "Read LLM Provider Templates", "View provider templates"},
	{"llm-provider-templates.update", "Update LLM Provider Templates", "Update a provider template"},
	{"llm-provider-templates.delete", "Delete LLM Provider Templates", "Delete a provider template"},
	{"llm-proxies.create", "Create LLM Proxies", "Create an LLM proxy"},
	{"llm-proxies.read", "Read LLM Proxies", "View LLM proxies and deployments"},
	{"llm-proxies.update", "Update LLM Proxies", "Update an LLM proxy"},
	{"llm-proxies.delete", "Delete LLM Proxies", "Delete an LLM proxy"},
	{"llm-proxies.deploy", "Deploy LLM Proxies", "Deploy, undeploy, and restore an LLM proxy"},
	{"llm-proxies.api-key-manage", "Manage LLM Proxy API Keys", "Create, update, and delete LLM proxy API keys"},
	{"mcp-servers.create", "Create MCP Servers", "Create an MCP server"},
	{"mcp-servers.read", "Read MCP Servers", "View MCP servers"},
	{"mcp-servers.update", "Update MCP Servers", "Update an MCP server"},
	{"mcp-servers.delete", "Delete MCP Servers", "Delete an MCP server"},
	{"mcp-servers.configure-guardrail", "Configure MCP Server Guardrails", "Configure guardrails on an MCP server"},
	{"mcp-servers.connect", "Connect MCP Servers", "Connect an agent to an MCP server"},
	{"mcp-servers.api-key-manage", "Manage MCP Server API Keys", "Create, update, and delete MCP server API keys"},
	{"scopes.create", "Create Scopes", "Create a scope"},
	{"scopes.read", "Read Scopes", "View scopes"},
	{"scopes.update", "Update Scopes", "Update a scope"},
	{"scopes.delete", "Delete Scopes", "Delete a scope"},
	{"catalog.read", "Read Catalog", "View the resource catalog"},
	{"agent-identities.create", "Create Agent Identities", "Create an agent identity"},
	{"agent-identities.read", "Read Agent Identities", "View agent identities"},
	{"agent-identities.update", "Update Agent Identities", "Update an agent identity"},
	{"agent-identities.delete", "Delete Agent Identities", "Delete an agent identity"},
	{"agent-api-keys.manage", "Manage Agent API Keys", "Create, update, and delete agent API keys"},
	{"deploy.non-production", "Deploy to Non-Production Environments", "Deploy, promote, or suspend an agent in a non-production environment"},
	{"deploy.production", "Deploy to Production Environments", "Deploy, promote, or suspend an agent in an environment flagged isProduction (with deploy.non-production)"},
}

var (
	templateAll = []string{"llm-provider-templates.create", "llm-provider-templates.read", "llm-provider-templates.update", "llm-provider-templates.delete"}
	providerAll = []string{
		"llm-providers.create", "llm-providers.read", "llm-providers.update", "llm-providers.delete",
		"llm-providers.configure-guardrail", "llm-providers.connect", "llm-providers.deploy", "llm-providers.api-key-manage",
	}
	mcpAll = []string{
		"mcp-servers.create", "mcp-servers.read", "mcp-servers.update", "mcp-servers.delete",
		"mcp-servers.configure-guardrail", "mcp-servers.connect", "mcp-servers.api-key-manage",
	}
	scopeAll    = []string{"scopes.create", "scopes.read", "scopes.update", "scopes.delete"}
	proxyAll    = []string{"llm-proxies.create", "llm-proxies.read", "llm-proxies.update", "llm-proxies.delete", "llm-proxies.deploy", "llm-proxies.api-key-manage"}
	identityAll = []string{"agent-identities.create", "agent-identities.read", "agent-identities.update", "agent-identities.delete"}
	gatewayAll  = []string{"gateways.create", "gateways.read", "gateways.update", "gateways.delete", "gateways.token-manage"}
)

func join(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

// FormerNames are the permissions' names before they moved into the
// module. Routes gated by one still record it as their audit action.
var FormerNames = map[rbac.Permission]string{
	Create: "gateway:create", Read: "gateway:read", Update: "gateway:update", Delete: "gateway:delete",
	TokenManage: "gateway:token-manage",

	LLMProviderCreate: "llm-provider:create", LLMProviderRead: "llm-provider:read",
	LLMProviderUpdate: "llm-provider:update", LLMProviderDelete: "llm-provider:delete",
	LLMProviderConfigureGuardrail: "llm-provider:configure-guardrail", LLMProviderConnect: "llm-provider:connect",
	LLMProviderDeploy: "llm-provider:deploy", LLMProviderAPIKeyManage: "llm-provider:api-key-manage",

	LLMProviderTemplateCreate: "llm-provider-template:create", LLMProviderTemplateRead: "llm-provider-template:read",
	LLMProviderTemplateUpdate: "llm-provider-template:update", LLMProviderTemplateDelete: "llm-provider-template:delete",

	LLMProxyCreate: "llm-proxy:create", LLMProxyRead: "llm-proxy:read", LLMProxyUpdate: "llm-proxy:update",
	LLMProxyDelete: "llm-proxy:delete", LLMProxyDeploy: "llm-proxy:deploy", LLMProxyAPIKeyManage: "llm-proxy:api-key-manage",

	MCPServerCreate: "mcp-server:create", MCPServerRead: "mcp-server:read", MCPServerUpdate: "mcp-server:update",
	MCPServerDelete: "mcp-server:delete", MCPServerConfigureGuardrail: "mcp-server:configure-guardrail",
	MCPServerConnect: "mcp-server:connect", MCPServerAPIKeyManage: "mcp-server:api-key-manage",

	ScopeCreate: "scope:create", ScopeRead: "scope:read", ScopeUpdate: "scope:update", ScopeDelete: "scope:delete",

	CatalogRead: "catalog:read",

	AgentIdentityCreate: "agent-identity:create", AgentIdentityRead: "agent-identity:read",
	AgentIdentityUpdate: "agent-identity:update", AgentIdentityDelete: "agent-identity:delete",

	AgentAPIKeyManage: "agent:api-key-manage",
}

// MCPScopes are the permissions of the tools the gateway's toolsets add to
// the core's MCP server.
var MCPScopes = []rbac.Permission{DeployNonProduction, LLMProviderRead, LLMProviderUpdate, MCPServerRead}

// RoleGrants is what each predefined role holds, by action name: what the
// roles held of these permissions under their former names.
var RoleGrants = map[string][]string{
	rbac.RoleAdmin: join(gatewayAll, providerAll, templateAll, proxyAll, mcpAll, scopeAll, identityAll,
		[]string{"catalog.read", "agent-api-keys.manage", "deploy.non-production", "deploy.production"}),
	rbac.RoleDeveloper: join([]string{
		"gateways.read", "llm-provider-templates.read",
		"llm-providers.read", "llm-providers.configure-guardrail", "llm-providers.connect",
		"mcp-servers.read", "mcp-servers.configure-guardrail", "mcp-servers.connect", "mcp-servers.api-key-manage",
		"scopes.read",
	}, proxyAll, []string{"agent-identities.read", "catalog.read", "agent-api-keys.manage", "deploy.non-production"}),
	rbac.RoleAILead: join(providerAll, templateAll, mcpAll, scopeAll,
		[]string{"catalog.read", "agent-api-keys.manage", "deploy.non-production"}),
	rbac.RolePlatformEngineer: join(gatewayAll, providerAll, templateAll, mcpAll, scopeAll, identityAll,
		[]string{"catalog.read", "agent-api-keys.manage", "deploy.non-production", "deploy.production"}),
}
