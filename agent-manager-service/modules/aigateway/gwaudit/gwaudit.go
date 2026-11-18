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
// manifest declares them with their class and severity, and this package
// registers the detail keys its semantic records may carry.
package gwaudit

import (
	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

// Actions are the module's audit actions, by their manifest names.
var Actions = []modkit.AuditAction{
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
}

// action is the full name of one of Actions.
func action(name string) audit.Action { return audit.Action(gwperm.Module + ":" + name) }

// The actions the gateway's services and MCP tools record themselves. Each
// is also the action of the REST route for the same operation.
var (
	APIKeyCreate    = action("api-keys.create")
	APIKeyRotate    = action("api-keys.rotate")
	APIKeyRevoke    = action("api-keys.revoke")
	APIKeyIssueTest = action("api-keys.issue-test")
	// APIKeySync is a data-plane gateway pulling the key material it is
	// entitled to.
	APIKeySync = action("api-keys.sync")

	GatewayTokenRotate = action("gateway-tokens.rotate")
	GatewayTokenRevoke = action("gateway-tokens.revoke")

	GatewayCreate                 = action("gateways.create")
	GatewayUpdate                 = action("gateways.update")
	GatewayDelete                 = action("gateways.delete")
	GatewayAssignEnvironment      = action("gateways.assign-environment")
	GatewayUnassignEnvironment    = action("gateways.unassign-environment")
	GatewaySetIdentityProvider    = action("gateways.set-identity-provider")
	GatewayRemoveIdentityProvider = action("gateways.remove-identity-provider")
	GatewayPushManifest           = action("gateways.push-manifest")

	// A gateway's startup sync.
	DeploymentSync      = action("deployments.sync")
	ArtifactCheckExists = action("artifacts.check-exists")
	// DeploymentRead is listing an agent's gateway deployments.
	DeploymentRead = action("deployments.read")

	LLMProviderRead   = action("llm-providers.read")
	LLMProviderUpdate = action("llm-providers.update")
	MCPServerRead     = action("mcp-servers.read")

	// Model and MCP configurations attached to an agent.
	AgentConfigUpdate = action("agent-configs.update")
	AgentConfigDelete = action("agent-configs.delete")

	AgentSetCardSource    = action("agents.set-card-source")
	AgentRemoveCardSource = action("agents.remove-card-source")
	AgentSetA2AProxy      = action("agents.set-a2a-proxy")
	AgentRemoveA2AProxy   = action("agents.remove-a2a-proxy")
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

//nolint:gochecknoinits // The detail schemas are the audit package's registry.
func init() {
	keyFields := map[string]audit.FieldKind{
		"ownerType":        audit.KindEnum,
		"ownerName":        audit.KindName,
		"keyName":          audit.KindName,
		"gatewayCount":     audit.KindCount,
		"gatewayConnected": audit.KindFlag,
		"expiresAt":        audit.KindName,
	}
	gatewayFields := map[string]audit.FieldKind{
		"gatewayName": audit.KindName,
		"gatewayType": audit.KindEnum,
		"vhost":       audit.KindName,
		"environment": audit.KindName,
	}
	// jwksUri and skipTlsVerify are the trust inputs themselves.
	idpFields := map[string]audit.FieldKind{
		"gatewayName":          audit.KindName,
		"identityProviderName": audit.KindName,
		"issuer":               audit.KindName,
		"jwksUri":              audit.KindURL,
		"skipTlsVerify":        audit.KindFlag,
	}
	a2aProxyFields := map[string]audit.FieldKind{
		"agentName":           audit.KindName,
		"upstreamAuthHeader":  audit.KindName,
		"upstreamAuthChanged": audit.KindFlag,
	}
	schemas := map[audit.Action]map[string]audit.FieldKind{
		APIKeyCreate: keyFields,
		APIKeyRotate: keyFields,
		APIKeyRevoke: {
			"ownerType": audit.KindEnum, "ownerName": audit.KindName,
			"keyName": audit.KindName, "gatewayCount": audit.KindCount,
		},
		APIKeyIssueTest: {
			"ownerType": audit.KindEnum, "ownerName": audit.KindName, "keyName": audit.KindName,
			"expiresAt": audit.KindName, "rotated": audit.KindFlag,
		},
		APIKeySync:         {"keyCount": audit.KindCount},
		GatewayTokenRotate: {"gatewayName": audit.KindName, "tokenId": audit.KindIdentifier, "expiresAt": audit.KindName},
		GatewayTokenRevoke: {"gatewayName": audit.KindName, "tokenId": audit.KindIdentifier},

		GatewayCreate:                 gatewayFields,
		GatewayUpdate:                 gatewayFields,
		GatewayDelete:                 gatewayFields,
		GatewayAssignEnvironment:      gatewayFields,
		GatewayUnassignEnvironment:    gatewayFields,
		GatewaySetIdentityProvider:    idpFields,
		GatewayRemoveIdentityProvider: idpFields,
		GatewayPushManifest:           {"policyCount": audit.KindCount},

		AgentConfigUpdate: {
			"configName": audit.KindName, "configType": audit.KindEnum,
			"agentName": audit.KindName, "updatedFields": audit.KindNameList,
		},
		AgentConfigDelete: {
			"configName": audit.KindName, "configType": audit.KindEnum,
			"agentName": audit.KindName, "environmentCount": audit.KindCount,
		},
		AgentSetCardSource:    {"agentName": audit.KindName, "sourceUrl": audit.KindURL},
		AgentRemoveCardSource: {"agentName": audit.KindName},
		AgentSetA2AProxy:      a2aProxyFields,
		AgentRemoveA2AProxy:   a2aProxyFields,
	}
	for _, a := range Actions {
		full := action(a.Name)
		class, severity := Classify(a)
		audit.Register(full, class, severity)
		fields := schemas[full]
		if fields == nil {
			// Route-level records carry no detail of their own.
			fields = map[string]audit.FieldKind{}
		}
		audit.RegisterDetailSchema(full, fields)
	}
}

// Classify is an action's audit class and severity.
func Classify(a modkit.AuditAction) (audit.ActionClass, audit.Severity) {
	class := map[string]audit.ActionClass{
		"credential": audit.ClassCredential, "identity": audit.ClassIdentity,
		"deployment": audit.ClassDeployment, "read": audit.ClassRead,
	}[a.Class]
	if class == "" {
		class = audit.ClassConfig
	}
	severity := map[string]audit.Severity{
		"info": audit.SeverityInfo, "notice": audit.SeverityNotice,
		"warning": audit.SeverityWarning, "critical": audit.SeverityCritical,
	}[a.Severity]
	return class, severity
}
