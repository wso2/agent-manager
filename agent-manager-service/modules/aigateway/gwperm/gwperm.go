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

// Module is the name of the module that owns these permissions.
const Module = "ai-gateway"

// Scope is the ThunderID scope that grants permission p.
func Scope(p string) string { return "amp:" + p }

// The core's permissions the gateway's routes and tools also require.
const (
	AgentRead       = "agent:read"
	AgentUpdate     = "agent:update"
	AgentDelete     = "agent:delete"
	EnvironmentRead = "environment:read"
)

// The core's predefined roles, which RoleGrants gives the module's permissions to.
const (
	RoleAdmin            = "Agent Manager Admin"
	RoleDeveloper        = "Developer"
	RoleAILead           = "AI Lead"
	RolePlatformEngineer = "Platform Engineer"
)

// The module's permissions, by their full names.
const (
	Create      = "ai-gateway:gateways.create"
	Read        = "ai-gateway:gateways.read"
	Update      = "ai-gateway:gateways.update"
	Delete      = "ai-gateway:gateways.delete"
	TokenManage = "ai-gateway:gateways.token-manage"

	LLMProviderCreate             = "ai-gateway:llm-providers.create"
	LLMProviderRead               = "ai-gateway:llm-providers.read"
	LLMProviderUpdate             = "ai-gateway:llm-providers.update"
	LLMProviderDelete             = "ai-gateway:llm-providers.delete"
	LLMProviderConfigureGuardrail = "ai-gateway:llm-providers.configure-guardrail"
	LLMProviderConnect            = "ai-gateway:llm-providers.connect"
	LLMProviderDeploy             = "ai-gateway:llm-providers.deploy"
	LLMProviderAPIKeyManage       = "ai-gateway:llm-providers.api-key-manage"

	LLMProviderTemplateCreate = "ai-gateway:llm-provider-templates.create"
	LLMProviderTemplateRead   = "ai-gateway:llm-provider-templates.read"
	LLMProviderTemplateUpdate = "ai-gateway:llm-provider-templates.update"
	LLMProviderTemplateDelete = "ai-gateway:llm-provider-templates.delete"

	LLMProxyCreate       = "ai-gateway:llm-proxies.create"
	LLMProxyRead         = "ai-gateway:llm-proxies.read"
	LLMProxyUpdate       = "ai-gateway:llm-proxies.update"
	LLMProxyDelete       = "ai-gateway:llm-proxies.delete"
	LLMProxyDeploy       = "ai-gateway:llm-proxies.deploy"
	LLMProxyAPIKeyManage = "ai-gateway:llm-proxies.api-key-manage"

	MCPServerCreate             = "ai-gateway:mcp-servers.create"
	MCPServerRead               = "ai-gateway:mcp-servers.read"
	MCPServerUpdate             = "ai-gateway:mcp-servers.update"
	MCPServerDelete             = "ai-gateway:mcp-servers.delete"
	MCPServerConfigureGuardrail = "ai-gateway:mcp-servers.configure-guardrail"
	MCPServerConnect            = "ai-gateway:mcp-servers.connect"
	MCPServerAPIKeyManage       = "ai-gateway:mcp-servers.api-key-manage"

	ScopeCreate = "ai-gateway:scopes.create"
	ScopeRead   = "ai-gateway:scopes.read"
	ScopeUpdate = "ai-gateway:scopes.update"
	ScopeDelete = "ai-gateway:scopes.delete"

	CatalogRead = "ai-gateway:catalog.read"

	AgentIdentityCreate = "ai-gateway:agent-identities.create"
	AgentIdentityRead   = "ai-gateway:agent-identities.read"
	AgentIdentityUpdate = "ai-gateway:agent-identities.update"
	AgentIdentityDelete = "ai-gateway:agent-identities.delete"

	AgentAPIKeyManage = "ai-gateway:agent-api-keys.manage"

	BudgetCreate = "ai-gateway:budgets.create"
	BudgetRead   = "ai-gateway:budgets.read"
	BudgetUpdate = "ai-gateway:budgets.update"
	BudgetDelete = "ai-gateway:budgets.delete"

	CostRead = "ai-gateway:costs.read"

	// The environment tier is an authorization axis of its own, about where
	// an action lands rather than what it is. DeployNonProduction is the
	// floor ("may act on environments at all") and DeployProduction is held
	// in addition to it to reach environments flagged isProduction.
	DeployNonProduction = "ai-gateway:deploy.non-production"
	DeployProduction    = "ai-gateway:deploy.production"
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
	{"budgets.create", "Create Budgets", "Create and import budgets"},
	{"budgets.read", "Read Budgets", "View budgets and their spend"},
	{"budgets.update", "Update Budgets", "Update, disable and enable a budget"},
	{"budgets.delete", "Delete Budgets", "Delete a budget"},
	{"costs.read", "Read AI Costs", "View costs by project, agent, API key, user, provider and model"},
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
	budgetAll   = []string{"budgets.create", "budgets.read", "budgets.update", "budgets.delete", "costs.read"}
)

func join(lists ...[]string) []string {
	var out []string
	for _, l := range lists {
		out = append(out, l...)
	}
	return out
}

// RoleGrants is what each predefined role holds, by action name: what the
// roles held of these permissions under their former names.
var RoleGrants = map[string][]string{
	RoleAdmin: join(gatewayAll, providerAll, templateAll, proxyAll, mcpAll, scopeAll, identityAll, budgetAll,
		[]string{"catalog.read", "agent-api-keys.manage", "deploy.non-production", "deploy.production"}),
	RoleDeveloper: join([]string{
		"gateways.read", "llm-provider-templates.read",
		"llm-providers.read", "llm-providers.configure-guardrail", "llm-providers.connect",
		"mcp-servers.read", "mcp-servers.configure-guardrail", "mcp-servers.connect", "mcp-servers.api-key-manage",
		"scopes.read",
	}, proxyAll, []string{"agent-identities.read", "catalog.read", "agent-api-keys.manage", "deploy.non-production", "budgets.read", "costs.read"}),
	RoleAILead: join(providerAll, templateAll, mcpAll, scopeAll, budgetAll,
		[]string{"catalog.read", "agent-api-keys.manage", "deploy.non-production"}),
	RolePlatformEngineer: join(gatewayAll, providerAll, templateAll, mcpAll, scopeAll, identityAll, budgetAll,
		[]string{"catalog.read", "agent-api-keys.manage", "deploy.non-production", "deploy.production"}),
}
