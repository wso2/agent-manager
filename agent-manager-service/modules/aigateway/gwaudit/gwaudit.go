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

// Package gwaudit holds the AI Gateway module's audit actions and webhook
// event types. Actions are recorded as ai-gateway:<resource>.<verb>; the
// manifest declares them with their class, severity and the detail keys
// their records may carry.
package gwaudit

import (
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

// Action is the full name of one of the module's audit actions.
type Action string

// Resource kinds the module's records name with modkit.AuditResource. Kept as
// constants so a query written against one route matches every other.
const (
	ResourceAPIKey  = "api-key"
	ResourceGateway = "gateway"
	ResourceAgent   = "agent"
)

// APIKeyOwner names the kind of resource an API key belongs to. Every API-key
// route shares one action, so without it the trail cannot tell an agent key
// from an LLM-provider key.
const (
	APIKeyOwnerAgent       = "agent"
	APIKeyOwnerLLMProvider = "llm-provider"
	APIKeyOwnerLLMProxy    = "llm-proxy"
	APIKeyOwnerModelConfig = "model-config"
	APIKeyOwnerMCPConfig   = "mcp-config"
)

// Actions are the module's audit actions, by their manifest names.
var Actions = withDetails([]modkit.AuditAction{
	{Name: "agent-configs.delete", Class: "config", Severity: "notice", Description: "Delete (agent configs)"},
	{Name: "agent-configs.update", Class: "config", Severity: "info", Description: "Update (agent configs)"},
	{Name: "agent-identities.add-group-member", Class: "credential", Severity: "critical", Description: "Add group member (agent identities)"},
	{Name: "agent-identities.assign-role", Class: "credential", Severity: "critical", Description: "Assign role (agent identities)"},
	{Name: "agent-identities.create", Class: "credential", Severity: "critical", Description: "Create (agent identities)"},
	{Name: "agent-identities.delete", Class: "credential", Severity: "critical", Description: "Delete (agent identities)"},
	{Name: "agent-identities.remove-group-member", Class: "credential", Severity: "critical", Description: "Remove group member (agent identities)"},
	{Name: "agent-identities.unassign-role", Class: "credential", Severity: "critical", Description: "Unassign role (agent identities)"},
	{Name: "agent-identities.update", Class: "credential", Severity: "critical", Description: "Update (agent identities)"},
	{Name: "agents.refresh-a2a-proxy", Class: "config", Severity: "info", Description: "Refresh a2a proxy (agents)"},
	{Name: "agents.refresh-card", Class: "config", Severity: "info", Description: "Refresh card (agents)"},
	{Name: "agents.remove-a2a-proxy", Class: "credential", Severity: "critical", Description: "Remove a2a proxy (agents)"},
	{Name: "agents.remove-card-source", Class: "config", Severity: "notice", Description: "Remove card source (agents)"},
	{Name: "agents.set-a2a-proxy", Class: "credential", Severity: "critical", Description: "Set a2a proxy (agents)"},
	{Name: "agents.set-card-source", Class: "config", Severity: "info", Description: "Set card source (agents)"},
	{Name: "agents.update-deploy-settings", Class: "config", Severity: "info", Description: "Update deploy settings (agents)"},
	{Name: "api-keys.create", Class: "credential", Severity: "critical", Description: "Create (api keys)"},
	{Name: "api-keys.issue-test", Class: "credential", Severity: "notice", Description: "Issue test (api keys)"},
	{Name: "api-keys.list", Class: "read", Severity: "notice", Description: "List (api keys)"},
	{Name: "api-keys.revoke", Class: "credential", Severity: "critical", Description: "Revoke (api keys)"},
	{Name: "api-keys.rotate", Class: "credential", Severity: "critical", Description: "Rotate (api keys)"},
	{Name: "api-keys.sync", Class: "read", Severity: "notice", Description: "Sync (api keys)"},
	{Name: "artifacts.check-exists", Class: "read", Severity: "info", Description: "Check exists (artifacts)"},
	{Name: "budgets.create", Class: "config", Severity: "info", Description: "Create (budgets)"},
	{Name: "budgets.delete", Class: "config", Severity: "notice", Description: "Delete (budgets)"},
	{Name: "budgets.import", Class: "config", Severity: "notice", Description: "Import (budgets)"},
	{Name: "budgets.update", Class: "config", Severity: "info", Description: "Update (budgets)"},
	{Name: "deployments.read", Class: "read", Severity: "info", Description: "Read (deployments)"},
	{Name: "deployments.sync", Class: "read", Severity: "notice", Description: "Sync (deployments)"},
	{Name: "environments.list-identity-providers", Class: "read", Severity: "info", Description: "List identity providers (environments)"},
	{Name: "gateway-tokens.list", Class: "read", Severity: "notice", Description: "List (gateway tokens)"},
	{Name: "gateway-tokens.revoke", Class: "credential", Severity: "critical", Description: "Revoke (gateway tokens)"},
	{Name: "gateway-tokens.rotate", Class: "credential", Severity: "critical", Description: "Rotate (gateway tokens)"},
	{Name: "gateways.assign-environment", Class: "config", Severity: "notice", Description: "Assign environment (gateways)"},
	{Name: "gateways.create", Class: "config", Severity: "notice", Description: "Create (gateways)"},
	{Name: "gateways.delete", Class: "config", Severity: "warning", Description: "Delete (gateways)"},
	{Name: "gateways.list-identity-providers", Class: "read", Severity: "info", Description: "List identity providers (gateways)"},
	{Name: "gateways.push-manifest", Class: "config", Severity: "info", Description: "Push manifest (gateways)"},
	{Name: "gateways.sync-resources", Class: "config", Severity: "info", Description: "Sync resources (gateways)"},
	{Name: "gateways.read", Class: "read", Severity: "info", Description: "Read (gateways)"},
	{Name: "gateways.remove-identity-provider", Class: "credential", Severity: "critical", Description: "Remove identity provider (gateways)"},
	{Name: "gateways.set-identity-provider", Class: "credential", Severity: "critical", Description: "Set identity provider (gateways)"},
	{Name: "gateways.unassign-environment", Class: "config", Severity: "notice", Description: "Unassign environment (gateways)"},
	{Name: "gateways.update", Class: "config", Severity: "notice", Description: "Update (gateways)"},
	{Name: "llm-provider-templates.create", Class: "config", Severity: "info", Description: "Create (llm provider templates)"},
	{Name: "llm-provider-templates.delete", Class: "config", Severity: "notice", Description: "Delete (llm provider templates)"},
	{Name: "llm-provider-templates.update", Class: "config", Severity: "info", Description: "Update (llm provider templates)"},
	{Name: "llm-providers.create", Class: "config", Severity: "info", Description: "Create (llm providers)"},
	{Name: "llm-providers.delete", Class: "config", Severity: "notice", Description: "Delete (llm providers)"},
	{Name: "llm-providers.delete-deployment", Class: "config", Severity: "info", Description: "Delete deployment (llm providers)"},
	{Name: "llm-providers.deploy", Class: "deployment", Severity: "notice", Description: "Deploy (llm providers)"},
	{Name: "llm-providers.read", Class: "read", Severity: "info", Description: "Read (llm providers)"},
	{Name: "llm-providers.restore", Class: "deployment", Severity: "notice", Description: "Restore (llm providers)"},
	{Name: "llm-providers.undeploy", Class: "deployment", Severity: "notice", Description: "Undeploy (llm providers)"},
	{Name: "llm-providers.update", Class: "config", Severity: "info", Description: "Update (llm providers)"},
	{Name: "llm-providers.update-catalog", Class: "config", Severity: "info", Description: "Update catalog (llm providers)"},
	{Name: "llm-proxies.create", Class: "config", Severity: "info", Description: "Create (llm proxies)"},
	{Name: "llm-proxies.delete", Class: "config", Severity: "notice", Description: "Delete (llm proxies)"},
	{Name: "llm-proxies.delete-deployment", Class: "config", Severity: "info", Description: "Delete deployment (llm proxies)"},
	{Name: "llm-proxies.deploy", Class: "deployment", Severity: "notice", Description: "Deploy (llm proxies)"},
	{Name: "llm-proxies.restore", Class: "deployment", Severity: "notice", Description: "Restore (llm proxies)"},
	{Name: "llm-proxies.undeploy", Class: "deployment", Severity: "notice", Description: "Undeploy (llm proxies)"},
	{Name: "llm-proxies.update", Class: "config", Severity: "info", Description: "Update (llm proxies)"},
	{Name: "mcp-servers.create", Class: "config", Severity: "info", Description: "Create (mcp servers)"},
	{Name: "mcp-servers.delete", Class: "config", Severity: "notice", Description: "Delete (mcp servers)"},
	{Name: "mcp-servers.fetch-server-info", Class: "read", Severity: "info", Description: "Fetch server info (mcp servers)"},
	{Name: "mcp-servers.read", Class: "read", Severity: "info", Description: "Read (mcp servers)"},
	{Name: "mcp-servers.update", Class: "config", Severity: "info", Description: "Update (mcp servers)"},
	{Name: "scopes.create", Class: "identity", Severity: "critical", Description: "Create (scopes)"},
	{Name: "scopes.delete", Class: "identity", Severity: "critical", Description: "Delete (scopes)"},
	{Name: "scopes.update", Class: "identity", Severity: "critical", Description: "Update (scopes)"},
})

// action is the full name of one of Actions.
func action(name string) Action { return Action(gwperm.Module + ":" + name) }

// The actions the gateway's MCP tools are recorded under. Each is also the
// action of the REST route for the same operation.
var (
	// DeploymentRead is listing an agent's gateway deployments.
	DeploymentRead    = action("deployments.read")
	LLMProviderRead   = action("llm-providers.read")
	LLMProviderUpdate = action("llm-providers.update")
	MCPServerRead     = action("mcp-servers.read")
	// AgentConfigUpdate is a model or MCP configuration attached to an agent.
	AgentConfigUpdate = action("agent-configs.update")
)

// Event is a webhook event type of the module, with the response body left
// out (WithoutData) for routes that return a credential.
type Event struct {
	Type        modkit.EventType
	WithoutData bool
}

// The module's webhook event types, named ai-gateway.<type>.
const (
	MCPServerCreated      = "ai-gateway.mcp_server.created"
	MCPServerUpdated      = "ai-gateway.mcp_server.updated"
	MCPServerDeleted      = "ai-gateway.mcp_server.deleted"
	LLMProviderCreated    = "ai-gateway.llm_provider.created"
	LLMProviderUpdated    = "ai-gateway.llm_provider.updated"
	LLMProviderDeleted    = "ai-gateway.llm_provider.deleted"
	LLMProviderDeployed   = "ai-gateway.llm_provider.deployed"
	LLMProviderUndeployed = "ai-gateway.llm_provider.undeployed"
	GatewayCreated        = "ai-gateway.gateway.created"
	GatewayUpdated        = "ai-gateway.gateway.updated"
	GatewayDeleted        = "ai-gateway.gateway.deleted"
	AgentConfigurationSet = "ai-gateway.agent.configuration_updated"
	AgentAPIKeyCreated    = "ai-gateway.agent.api_key_created"
	AgentAPIKeyRotated    = "ai-gateway.agent.api_key_rotated"
	AgentAPIKeyRevoked    = "ai-gateway.agent.api_key_revoked"
)

// AgentBindingsChanged is published when an agent update changes its
// bindings, for the module's own handler; it is not a webhook type.
const AgentBindingsChanged = "ai-gateway.agent.bindings_changed"

// Events are the module's event types: its webhook types and the ones only
// its own handlers receive.
var Events = []modkit.EventType{
	{Name: AgentBindingsChanged, Description: "An agent's LLM or MCP bindings changed."},
	{Name: MCPServerCreated, Scope: "org", Description: "An MCP server was added."},
	{Name: MCPServerUpdated, Scope: "org", Description: "An MCP server was updated."},
	{Name: MCPServerDeleted, Scope: "org", Description: "An MCP server was deleted."},
	{Name: LLMProviderCreated, Scope: "org", Description: "An LLM provider was added."},
	{Name: LLMProviderUpdated, Scope: "org", Description: "An LLM provider was updated."},
	{Name: LLMProviderDeleted, Scope: "org", Description: "An LLM provider was deleted."},
	{Name: LLMProviderDeployed, Scope: "org", Description: "An LLM provider was deployed to a gateway."},
	{Name: LLMProviderUndeployed, Scope: "org", Description: "An LLM provider was undeployed from a gateway."},
	{Name: GatewayCreated, Scope: "org", Description: "A gateway was registered."},
	{Name: GatewayUpdated, Scope: "org", Description: "A gateway was updated."},
	{Name: GatewayDeleted, Scope: "org", Description: "A gateway was deleted."},
	{Name: AgentConfigurationSet, Scope: "agent", Description: "The agent's gateway deploy settings changed."},
	{Name: AgentAPIKeyCreated, Scope: "agent", Description: "An API key was created for the agent."},
	{Name: AgentAPIKeyRotated, Scope: "agent", Description: "An API key was rotated."},
	{Name: AgentAPIKeyRevoked, Scope: "agent", Description: "An API key was revoked."},
}

// withDetails gives each action the detail keys its records may carry.
func withDetails(actions []modkit.AuditAction) []modkit.AuditAction {
	keyFields := map[string]string{
		"ownerType": "enum", "ownerName": "name", "keyName": "name",
		"gatewayCount": "count", "gatewayConnected": "flag", "expiresAt": "name",
	}
	gatewayFields := map[string]string{
		"gatewayName": "name", "gatewayType": "enum", "vhost": "name", "environment": "name",
		// A refused request whose body named another organization.
		"requestedOrgId": "identifier",
	}
	// jwksUri and skipTlsVerify are the trust inputs themselves.
	idpFields := map[string]string{
		"gatewayName": "name", "identityProviderName": "name", "issuer": "name",
		"jwksUri": "url", "skipTlsVerify": "flag",
	}
	a2aProxyFields := map[string]string{
		"agentName": "name", "upstreamAuthHeader": "name", "upstreamAuthChanged": "flag",
	}
	details := map[string]map[string]string{
		"api-keys.create": keyFields,
		"api-keys.rotate": keyFields,
		"api-keys.revoke": {"ownerType": "enum", "ownerName": "name", "keyName": "name", "gatewayCount": "count"},
		"api-keys.issue-test": {
			"ownerType": "enum", "ownerName": "name", "keyName": "name", "expiresAt": "name", "rotated": "flag",
		},
		"api-keys.sync": {"keyCount": "count"},
		"gateway-tokens.rotate": {
			"gatewayName": "name", "tokenId": "identifier", "expiresAt": "name", "requestedOrgId": "identifier",
		},
		"gateway-tokens.revoke": {"gatewayName": "name", "tokenId": "identifier"},

		"gateways.create":                   gatewayFields,
		"gateways.update":                   gatewayFields,
		"gateways.delete":                   gatewayFields,
		"gateways.assign-environment":       gatewayFields,
		"gateways.unassign-environment":     gatewayFields,
		"gateways.set-identity-provider":    idpFields,
		"gateways.remove-identity-provider": idpFields,
		"gateways.push-manifest":            {"policyCount": "count"},
		// A gateway mirroring the budgets it created through its own API.
		"gateways.sync-resources": {"resourceCount": "count", "failedCount": "count"},
		"budgets.delete":          {"budgetName": "name"},

		"agent-configs.update": {
			"configName": "name", "configType": "enum", "agentName": "name", "updatedFields": "name-list",
		},
		"agent-configs.delete": {
			"configName": "name", "configType": "enum", "agentName": "name", "environmentCount": "count",
		},
		"agents.set-card-source": {"agentName": "name", "sourceUrl": "url"},
		// Removing the source of a proxied environment removes the proxy too.
		"agents.remove-card-source": a2aProxyFields,
		"agents.set-a2a-proxy":      a2aProxyFields,
		"agents.remove-a2a-proxy":   a2aProxyFields,
	}
	for i := range actions {
		actions[i].Details = details[actions[i].Name]
	}
	return actions
}
