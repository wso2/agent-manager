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

package audit

// Domain actions for the operations that warrant a semantic record.
//
// The AI Gateway module's own actions are in modules/aigateway/gwaudit.
//
// These deliberately use the same names the coverage tier derives for the
// corresponding routes (see actionOverrides in policy.go), so that "who
// deployed agent X" is one query whether the call arrived over REST, MCP or an
// internal API. Changing a name here without changing it there splits the trail
// in two.
const (

	// ActionAuthLogin covers a file-mode sign-in attempt, successful or not.
	// One action for both outcomes: "show me every attempt on this account"
	// is the question an audit trail exists to answer, and splitting success
	// from failure into two actions makes that query a union of two.
	ActionAuthLogin Action = "authn:login"

	ActionAgentTokenMint Action = "agent-token:mint"

	// A module's workload credentials, and the tokens its workloads exchange
	// them for.
	ActionWorkloadCredentialIssue     Action = "workload-credential:issue"
	ActionWorkloadCredentialRevoke    Action = "workload-credential:revoke"
	ActionWorkloadTokenExchange       Action = "workload-token:exchange"
	ActionAgentTokenRegenerateTracing Action = "agent-token:regenerate-tracing"

	ActionAgentIdentityProvision         Action = "agent-identity:provision"
	ActionAgentIdentityRegenerateSecret  Action = "agent-identity:regenerate-secret"
	ActionAgentIdentityRevokeSecret      Action = "agent-identity:revoke-secret"
	ActionAgentIdentityRetryProvisioning Action = "agent-identity:retry-provisioning"

	ActionServiceAccountConfigure Action = "service-account:configure"
	ActionServiceAccountRemove    Action = "service-account:remove"

	ActionThunderURLSet    Action = "thunder-url:set"
	ActionThunderURLDelete Action = "thunder-url:delete"

	// An event destination decides where event data leaves the platform and
	// may hold a secret (a webhook's signing secret, a service URL's token).
	ActionEventDestinationCreate       Action = "event-destination:create"
	ActionEventDestinationUpdate       Action = "event-destination:update"
	ActionEventDestinationDelete       Action = "event-destination:delete"
	ActionEventDestinationRotateSecret Action = "event-destination:rotate-secret"
	ActionEventDestinationTest         Action = "event-destination:test"

	// An event subscription decides which events are sent to a destination.
	ActionEventSubscriptionCreate Action = "event-subscription:create"
	ActionEventSubscriptionUpdate Action = "event-subscription:update"
	ActionEventSubscriptionDelete Action = "event-subscription:delete"

	// Identity and privilege. These are the escalation path: a record that says
	// only "a role was updated" is useless, so each names what actually changed.
	ActionRoleGrantPermission  Action = "role:grant-permission"
	ActionRoleRevokePermission Action = "role:revoke-permission"
	ActionRoleAssign           Action = "role:assign"
	ActionRoleUnassign         Action = "role:unassign"

	ActionGroupAddMember    Action = "group:add-member"
	ActionGroupRemoveMember Action = "group:remove-member"

	ActionUserInvite Action = "user:invite"
	ActionUserCreate Action = "user:create"
	ActionUserUpdate Action = "user:update"
	ActionUserDelete Action = "user:delete"

	// Read actions. Declared so every MCP tool can name what it does; they are
	// classified as reads and therefore not recorded, matching the REST policy
	// of auditing only credential-disclosing GETs.
	ActionAgentRead       Action = "agent:read"
	ActionProjectRead     Action = "project:read"
	ActionEnvironmentRead Action = "environment:read"

	// Binding a model or MCP config to an agent; the REST routes are gated by
	// agent:update alone, so that is the action they derive.
	ActionAgentUpdate Action = "agent:update"

	// Agent lifecycle.
	ActionAgentCreate                Action = "agent:create"
	ActionProjectCreate              Action = "project:create"
	ActionAgentBuild                 Action = "agent:build"
	ActionAgentDeploy                Action = "agent:deploy"
	ActionAgentPromote               Action = "agent:promote"
	ActionAgentChangeDeploymentState Action = "agent:change-deployment-state"
	ActionAgentDelete                Action = "agent:delete"
	ActionProjectDelete              Action = "project:delete"

	// System-initiated work. Recorded when it changes state, not on every tick.
	ActionSystemAgentIdentityProvisioned Action = "system:agent-identity-provisioned"
	ActionSystemAgentIdentityExhausted   Action = "system:agent-identity-exhausted"
)

// APIKeyOwner names the kind of resource an API key belongs to. Every API-key
// route shares one permission and one action, so without this the trail cannot
// tell an agent key from an LLM-provider key.
const (
	APIKeyOwnerAgent       = "agent"
	APIKeyOwnerLLMProvider = "llm-provider"
	APIKeyOwnerLLMProxy    = "llm-proxy"
	APIKeyOwnerModelConfig = "model-config"
	APIKeyOwnerMCPConfig   = "mcp-config"
)

// Resource type names used in records. Kept as constants so a query written
// against one emit site matches every other.
const (
	ResourceAPIKey            = "api-key"
	ResourceGateway           = "gateway"
	ResourceGatewayToken      = "gateway-token"
	ResourceAgent             = "agent"
	ResourceRole              = "role"
	ResourceGroup             = "group"
	ResourceUser              = "user"
	ResourceAgentIdentity     = "agent-identity"
	ResourceEnvironment       = "environment"
	ResourceServiceAccount    = "service-account"
	ResourceThunderURL        = "thunder-url"
	ResourceEventDestination  = "event-destination"
	ResourceEventSubscription = "event-subscription"
	ResourceBuildToken        = "build-token"
)

// class, severity and permitted detail keys next to the action itself keeps the
// three from drifting apart.
//
//nolint:gochecknoinits // The registry is package state; declaring each action's
func init() {
	// Credential lifecycle. All critical: these grant or withdraw access.
	// Credential class: a successful login hands back a bearer token carrying
	// the full scope set of the user's roles.
	registerCredential(ActionAuthLogin, map[string]FieldKind{
		"username": KindName,
		// Which privileges the token went out with. Recorded on success only;
		// a refused attempt was granted none.
		"roles": KindNameList,
		// Why a refusal happened, as a fixed label ("unknown-user",
		// "bad-password") rather than free text, so refusals are countable.
		// Never which of the two it was in the response — only here.
		"reason":    KindEnum,
		"expiresIn": KindCount,
	})

	registerCredential(ActionAgentTokenMint, map[string]FieldKind{
		"agentName": KindName,
		"expiresIn": KindCount,
	})
	registerCredential(ActionWorkloadCredentialIssue, map[string]FieldKind{
		"module":   KindName,
		"workload": KindName,
		"subject":  KindIdentifier,
	})
	registerCredential(ActionWorkloadCredentialRevoke, map[string]FieldKind{
		"module": KindName,
	})
	// A workload heartbeat: the first token of a new instance, or a refusal
	// (reason is the error code). Refreshes of a known instance are not
	// recorded.
	registerCredential(ActionWorkloadTokenExchange, map[string]FieldKind{
		"module":       KindName,
		"workload":     KindName,
		"instance":     KindName,
		"credentialId": KindIdentifier,
		"expiresIn":    KindCount,
		"reason":       KindEnum,
	})
	registerCredential(ActionAgentTokenRegenerateTracing, map[string]FieldKind{
		"agentName": KindName,
	})

	registerCredential(ActionAgentIdentityProvision, map[string]FieldKind{
		"agentName":   KindName,
		"environment": KindName,
		"clientId":    KindIdentifier,
		// True when the binding already existed, so no new credential was issued.
		"alreadyExisted": KindFlag,
	})
	registerCredential(ActionAgentIdentityRegenerateSecret, map[string]FieldKind{
		"agentName":   KindName,
		"environment": KindName,
		"clientId":    KindIdentifier,
	})
	registerCredential(ActionAgentIdentityRevokeSecret, map[string]FieldKind{
		"agentName":   KindName,
		"environment": KindName,
		"clientId":    KindIdentifier,
	})
	registerCredential(ActionAgentIdentityRetryProvisioning, map[string]FieldKind{
		"agentName":   KindName,
		"environment": KindName,
	})

	registerCredential(ActionServiceAccountConfigure, map[string]FieldKind{
		"environment": KindName,
		"clientId":    KindIdentifier,
	})
	registerCredential(ActionServiceAccountRemove, map[string]FieldKind{
		"environment": KindName,
	})

	// A destination is recorded as scheme and host only, since a URL path is
	// often a credential. Secrets never reach the record.
	eventDestinationFields := map[string]FieldKind{
		"kind":         KindEnum,
		"destination":  KindURL,
		"availability": KindEnum,
		"projects":     KindNameList,
	}
	registerCredential(ActionEventDestinationCreate, eventDestinationFields)
	registerCredential(ActionEventDestinationUpdate, eventDestinationFields)
	registerCredential(ActionEventDestinationDelete, eventDestinationFields)
	registerCredential(ActionEventDestinationRotateSecret, eventDestinationFields)
	// A test send changes nothing, but it does send a request to the
	// configured destination, so it is recorded above info.
	Register(ActionEventDestinationTest, ClassConfig, SeverityNotice)
	RegisterDetailSchema(ActionEventDestinationTest, nil)

	// A subscription holds no credential, but it decides which event data
	// reaches a destination outside the platform.
	eventSubscriptionFields := map[string]FieldKind{
		"scope":         KindEnum,
		"destinationId": KindIdentifier,
		"eventCount":    KindCount,
		"allEvents":     KindFlag,
		"environments":  KindNameList,
		"enabled":       KindFlag,
	}
	for _, action := range []Action{
		ActionEventSubscriptionCreate, ActionEventSubscriptionUpdate, ActionEventSubscriptionDelete,
	} {
		Register(action, ClassConfig, SeverityNotice)
		RegisterDetailSchema(action, eventSubscriptionFields)
	}

	// Identity and privilege changes. Critical for the same reason credential
	// changes are: they decide who can do what.
	//
	// The permission and assignee lists are recorded in full. "alice granted
	// SRE the deploy-production scope" is the question an auditor actually
	// asks, and a record naming only the role cannot answer it. Scope strings
	// and principal ids are identifiers, not secrets.
	registerIdentity(ActionRoleGrantPermission, map[string]FieldKind{
		"roleName":         KindName,
		"permissions":      KindNameList,
		"permissionCount":  KindCount,
		"resourceServerId": KindIdentifier,
	})
	registerIdentity(ActionRoleRevokePermission, map[string]FieldKind{
		"roleName":         KindName,
		"permissions":      KindNameList,
		"permissionCount":  KindCount,
		"resourceServerId": KindIdentifier,
	})
	registerIdentity(ActionRoleAssign, map[string]FieldKind{
		"roleName":      KindName,
		"assignees":     KindNameList,
		"assigneeTypes": KindNameList,
		"assigneeCount": KindCount,
	})
	registerIdentity(ActionRoleUnassign, map[string]FieldKind{
		"roleName":      KindName,
		"assignees":     KindNameList,
		"assigneeTypes": KindNameList,
		"assigneeCount": KindCount,
	})

	registerIdentity(ActionGroupAddMember, map[string]FieldKind{
		"groupName":   KindName,
		"members":     KindNameList,
		"memberTypes": KindNameList,
		"memberCount": KindCount,
	})
	registerIdentity(ActionGroupRemoveMember, map[string]FieldKind{
		"groupName":   KindName,
		"members":     KindNameList,
		"memberTypes": KindNameList,
		"memberCount": KindCount,
	})

	// User lifecycle. The attribute map these routes accept is free-form and
	// carries passwords, so only its key names and shape are recordable — see
	// AttributeKeySummary.
	userFields := map[string]FieldKind{
		"username":             KindIdentifier,
		"email":                KindEmail,
		"userType":             KindEnum,
		"groups":               KindNameList,
		"attributeKeys":        KindNameList,
		"attributeCount":       KindCount,
		"containsSensitiveKey": KindFlag,
	}
	registerIdentity(ActionUserInvite, userFields)
	registerIdentity(ActionUserCreate, userFields)
	registerIdentity(ActionUserUpdate, userFields)
	registerIdentity(ActionUserDelete, map[string]FieldKind{
		"username": KindIdentifier,
	})

	// Agent lifecycle. Deployment events carry the target environment and
	// whether it is production, because the permission gating the deploy route
	// says "non-production" regardless of where the agent actually lands — so
	// the record, not the permission, is what establishes what happened.
	Register(ActionAgentRead, ClassRead, SeverityInfo)
	RegisterDetailSchema(ActionAgentRead, nil)
	Register(ActionProjectRead, ClassRead, SeverityInfo)
	RegisterDetailSchema(ActionProjectRead, nil)
	Register(ActionEnvironmentRead, ClassRead, SeverityInfo)
	RegisterDetailSchema(ActionEnvironmentRead, nil)

	Register(ActionAgentCreate, ClassConfig, SeverityNotice)
	RegisterDetailSchema(ActionAgentCreate, map[string]FieldKind{
		"agentName": KindName,
		"agentType": KindEnum,
		"tool":      KindName,
	})
	Register(ActionProjectCreate, ClassConfig, SeverityNotice)
	RegisterDetailSchema(ActionProjectCreate, map[string]FieldKind{
		"projectName": KindName,
		"tool":        KindName,
	})

	Register(ActionAgentBuild, ClassDeployment, SeverityNotice)
	RegisterDetailSchema(ActionAgentBuild, map[string]FieldKind{
		"agentName": KindName,
		"commitId":  KindIdentifier,
		"buildName": KindName,
	})

	deployFields := map[string]FieldKind{
		"agentName":    KindName,
		"environment":  KindName,
		"isProduction": KindFlag,
		"imageId":      KindIdentifier,
	}
	Register(ActionAgentDeploy, ClassDeployment, SeverityNotice)
	RegisterDetailSchema(ActionAgentDeploy, deployFields)
	// A promotion moves an agent up the pipeline, often into production, so it
	// is ranked above an ordinary deploy.
	Register(ActionAgentPromote, ClassDeployment, SeverityWarning)
	RegisterDetailSchema(ActionAgentPromote, map[string]FieldKind{
		"agentName":    KindName,
		"sourceEnv":    KindName,
		"targetEnv":    KindName,
		"isProduction": KindFlag,
		"environment":  KindName,
	})
	Register(ActionAgentChangeDeploymentState, ClassDeployment, SeverityWarning)
	RegisterDetailSchema(ActionAgentChangeDeploymentState, map[string]FieldKind{
		"agentName":   KindName,
		"environment": KindName,
		"toState":     KindEnum,
	})

	// Deletions are irreversible, so they rank above other config changes.
	Register(ActionAgentDelete, ClassConfig, SeverityWarning)
	RegisterDetailSchema(ActionAgentDelete, map[string]FieldKind{
		"agentName": KindName,
	})
	Register(ActionProjectDelete, ClassConfig, SeverityWarning)
	RegisterDetailSchema(ActionProjectDelete, map[string]FieldKind{
		"projectName": KindName,
	})

	// The provisioning reconciler issues real credentials with no request
	// behind it, so its outcomes are recorded with a system actor. The user who
	// originally asked for the agent is carried as OnBehalfOf, which is what the
	// requested_by column on the binding was captured for.
	Register(ActionSystemAgentIdentityProvisioned, ClassCredential, SeverityNotice)
	RegisterDetailSchema(ActionSystemAgentIdentityProvisioned, map[string]FieldKind{
		"agentName":   KindName,
		"environment": KindName,
		"clientId":    KindIdentifier,
	})
	// Retries exhausted: the agent will not get an identity without operator
	// action, so this is a warning rather than routine.
	Register(ActionSystemAgentIdentityExhausted, ClassCredential, SeverityWarning)
	RegisterDetailSchema(ActionSystemAgentIdentityExhausted, map[string]FieldKind{
		"agentName":   KindName,
		"environment": KindName,
		"reason":      KindName,
	})
}

// registerIdentity declares an identity or privilege action. These are always
// critical: they change who holds which permission.
func registerIdentity(action Action, fields map[string]FieldKind) {
	Register(action, ClassIdentity, SeverityCritical)
	if fields == nil {
		fields = map[string]FieldKind{}
	}
	RegisterDetailSchema(action, fields)
}

// registerCredential declares a credential action and the detail keys it may
// carry. Credential changes are always critical, so severity is not a parameter.
func registerCredential(action Action, fields map[string]FieldKind) {
	Register(action, ClassCredential, SeverityCritical)
	if fields == nil {
		fields = map[string]FieldKind{}
	}
	RegisterDetailSchema(action, fields)
}
