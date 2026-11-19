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

// Package gwerr holds the AI Gateway's error sentinels and error codes.
package gwerr

import "errors"

const (
	// ErrCodeProjectOwned: an org route tried to mutate a project-scoped MCP proxy.
	ErrCodeProjectOwned = "PROJECT_OWNED"
	// ErrCodeContextInUse: an artifact's gateway context overlaps another artifact's on that gateway.
	ErrCodeContextInUse            = "CONTEXT_IN_USE"
	ErrCodeGatewayEnvironmentFixed = "GATEWAY_ENVIRONMENT_FIXED"
)

var (
	ErrAgentCardNotFound             = errors.New("agent card not found")
	ErrAgentNotDeployedInEnvironment = errors.New("agent is not deployed in this environment")
	ErrAgentNotA2A                   = errors.New("agent is not an A2A agent")
	ErrAgentCardSourceNotExternal    = errors.New("a card source can be set only for external agents")
	ErrEnvironmentNotProxied         = errors.New("the environment does not route this agent through its gateway")
	ErrA2AProxyAgentNotExternal      = errors.New("only external agents can be routed through the gateway")
	ErrA2AProxyNeedsCardSource       = errors.New("register the agent's card URL for this environment before routing it through the gateway")
	ErrA2ACardURLNotProxyable        = errors.New("the card URL cannot be routed through the gateway")
	ErrInvalidUpstreamAuth           = errors.New("invalid upstream auth")
	// ErrA2AProxyUpstreamAuthNeedsHTTPS keeps a stored upstream credential off an origin that would receive it in cleartext. Maps to 400.
	ErrA2AProxyUpstreamAuthNeedsHTTPS = errors.New("the card URL must use https before an upstream credential can be stored, as the gateway would otherwise send it in cleartext")
	ErrScopeNotFound                  = errors.New("scope not found")
	// ErrA2AProxyOriginChangeNeedsAuthReset keeps a stored upstream secret from following the card URL to another origin. Maps to 409.
	ErrA2AProxyOriginChangeNeedsAuthReset = errors.New(
		"the stored upstream credential would be sent to the new card URL's origin; clear the upstream auth before moving the card to another origin, then enter it again",
	)
	// Gateway-related errors
	ErrGatewayNotFound          = errors.New("gateway not found")
	ErrGatewayTokenLimitReached = errors.New("maximum 2 active tokens allowed. Revoke old tokens before rotating") // 409: rotating would exceed two active tokens
	ErrGatewayTokenNotFound     = errors.New("gateway token not found")                                            // 404: an unknown token, or a token of another gateway
	ErrGatewayAlreadyExists     = errors.New("gateway already exists")
	// ErrGatewayIngressCapExceeded is returned when assigning an ingress-capable gateway to
	// an environment that already has one. Maps to 409.
	ErrGatewayIngressCapExceeded = errors.New("environment already has an ingress gateway")
	// ErrGatewayEnvironmentFixed is returned when registering or assigning a gateway would map it
	// to a second environment. Maps to 409.
	ErrGatewayEnvironmentFixed = errors.New("gateway already serves another environment")
	// ErrGatewayServesEnvironment is returned when unmapping a gateway from an environment whose
	// agents, MCP proxies or LLM proxies are still placed on it. Maps to 409.
	ErrGatewayServesEnvironment    = errors.New("gateway still serves the environment's artifacts")
	ErrDeploymentNotFound          = errors.New("deployment not found")
	ErrSystemTemplateImmutable     = errors.New("system templates cannot be modified or deleted")
	ErrSystemTemplateOverride      = errors.New("cannot create user template with same handle as system template")
	ErrGatewayHasDeployments       = errors.New("cannot delete gateway: it has active API deployments. Please undeploy all APIs before deleting the gateway")
	ErrAPINotFound                 = errors.New("API not found")
	ErrDeploymentNotActive         = errors.New("deployment not active")
	ErrLLMProviderTemplateNotFound = errors.New("LLM provider template not found")
	ErrLLMProviderTemplateExists   = errors.New("LLM provider template already exists")
	ErrLLMProviderNotFound         = errors.New("LLM provider not found")
	ErrLLMProviderExists           = errors.New("LLM provider already exists")
	ErrLLMProviderHasProxies       = errors.New("cannot delete LLM provider: it has associated LLM proxies. Please delete all proxies before deleting the provider")
	ErrLLMProviderUndeployFailed   = errors.New("cannot delete LLM provider: undeploying it from its gateways failed. Retry, or undeploy manually before deleting")
	ErrLLMProviderDeleteInProgress = errors.New("cannot delete LLM provider: a delete is already in progress for it")
	ErrLLMProviderBeingDeleted     = errors.New("cannot create LLM proxy: the LLM provider is being deleted")
	ErrLLMProxyNotFound            = errors.New("LLM proxy not found")
	ErrLLMProxyExists              = errors.New("LLM proxy already exists")
	ErrLLMProxyManagedByAMS        = errors.New("this LLM proxy is managed by an agent or monitor; change the agent's or monitor's LLM configuration instead")
	ErrMCPProxyNotFound            = errors.New("MCP proxy not found")
	// ErrAgentArtifactNotFound means no active Agent deployment exists for this
	// artifact on this gateway.
	ErrAgentArtifactNotFound     = errors.New("agent artifact not found")
	ErrMCPProxyExists            = errors.New("MCP proxy already exists")
	ErrMCPProxyHasMappings       = errors.New("cannot delete MCP proxy: it has associated MCP proxy mappings. Please delete all mappings before deleting the proxy")
	ErrMCPEnvAlreadyBound        = errors.New("environment is already assigned to another endpoint in this MCP proxy")
	ErrURLUnreachable            = errors.New("URL unreachable")
	ErrMCPServerUnauthorized     = errors.New("MCP server unauthorized")
	ErrMCPResponseTooLarge       = errors.New("MCP response body too large")
	ErrTooManyRequests           = errors.New("too many requests")
	ErrDeploymentIsDeployed      = errors.New("deployment is currently deployed")
	ErrDeploymentAlreadyDeployed = errors.New("deployment already deployed")
	ErrGatewayIDMismatch         = errors.New("gateway ID mismatch")
	ErrDeploymentBaseNotCurrent  = errors.New(`base must be omitted or "current"; ` +
		"redeploy an earlier deployment with POST .../deployments/restore?deploymentId=<id>&gatewayId=<gatewayId>")
	ErrDeploymentGatewayIDRequired = errors.New("deployment gateway ID required")
	ErrInvalidDeploymentStatus     = errors.New("invalid deployment status")
	ErrArtifactNotFound            = errors.New("artifact not found")
	ErrArtifactExists              = errors.New("artifact already exists")
	// Agent Configuration errors
	ErrAgentConfigNotFound      = errors.New("agent configuration not found")
	ErrAgentConfigAlreadyExists = errors.New("agent configuration already exists for this agent")
	// ErrAgentConfigNotExternal is returned when a user-managed API key action
	// (create/rotate/revoke) is attempted against a configuration whose agent is
	// managed/internal. Only external agents own their proxy API keys; managed
	// agents have the platform inject them, so user-managed actions are rejected.
	ErrAgentConfigNotExternal = errors.New("API key management is only available for external agents")
	// ErrMCPProxyHandleInUse means the handle belongs to an org-level proxy, another
	// project's proxy, or another artifact (409 HANDLE_IN_USE on the project routes).
	ErrMCPProxyHandleInUse = errors.New("MCP proxy handle already in use")
	// ErrMCPProxyProjectOwned means an org-level mutation targeted a project-scoped proxy
	// (409 PROJECT_OWNED on the org routes). Project proxies change only via the project routes.
	ErrMCPProxyProjectOwned = errors.New("MCP proxy belongs to a project and can only be changed through the project routes")
	// ErrContextInUse means an artifact's gateway context, of any kind, equals, contains or sits under another
	// artifact's context on the same vhost of the gateway (409 CONTEXT_IN_USE).
	ErrContextInUse = errors.New("the context is already in use on the gateway")
	// ErrMCPContextInUse is ErrContextInUse, under the name the MCP proxy routes have used for it.
	ErrMCPContextInUse = ErrContextInUse
)
