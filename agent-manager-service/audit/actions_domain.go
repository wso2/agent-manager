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
// These deliberately use the same names the coverage tier derives for the
// corresponding routes (see actionOverrides in policy.go), so that "who
// deployed agent X" is one query whether the call arrived over REST, MCP or an
// internal API. Changing a name here without changing it there splits the trail
// in two.
const (
	// Credentials.
	ActionAPIKeyCreate    Action = "api-key:create"
	ActionAPIKeyRotate    Action = "api-key:rotate"
	ActionAPIKeyRevoke    Action = "api-key:revoke"
	ActionAPIKeyIssueTest Action = "api-key:issue-test"
	// ActionAPIKeySync is a data-plane gateway pulling the key material it is
	// entitled to. Recorded because a compromised gateway credential harvesting
	// keys looks exactly like a legitimate sync until you can see who pulled
	// what and when it started.
	ActionAPIKeySync Action = "api-key:sync"

	ActionGitSecretCreate Action = "git-secret:create"
	ActionGitSecretDelete Action = "git-secret:delete"

	ActionGatewayTokenRotate Action = "gateway-token:rotate"
	ActionGatewayTokenRevoke Action = "gateway-token:revoke"

	ActionAgentTokenMint              Action = "agent-token:mint"
	ActionAgentTokenRegenerateTracing Action = "agent-token:regenerate-tracing"

	ActionAgentIdentityProvision         Action = "agent-identity:provision"
	ActionAgentIdentityRegenerateSecret  Action = "agent-identity:regenerate-secret"
	ActionAgentIdentityRevokeSecret      Action = "agent-identity:revoke-secret"
	ActionAgentIdentityRetryProvisioning Action = "agent-identity:retry-provisioning"

	ActionServiceAccountConfigure Action = "service-account:configure"
	ActionServiceAccountRemove    Action = "service-account:remove"

	ActionThunderURLSet    Action = "thunder-url:set"
	ActionThunderURLDelete Action = "thunder-url:delete"

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

	// Agent lifecycle.
	ActionAgentCreate                Action = "agent:create"
	ActionProjectCreate              Action = "project:create"
	ActionAgentBuild                 Action = "agent:build"
	ActionAgentCancelBuild           Action = "agent:cancel-build"
	ActionAgentDeploy                Action = "agent:deploy"
	ActionAgentPromote               Action = "agent:promote"
	ActionAgentChangeDeploymentState Action = "agent:change-deployment-state"
	ActionAgentDelete                Action = "agent:delete"
	ActionProjectDelete              Action = "project:delete"

	// Agent configuration (model and MCP configs attached to an agent).
	ActionAgentConfigUpdate Action = "agent-config:update"
	ActionAgentConfigDelete Action = "agent-config:delete"

	// Platform: gateways and their trust configuration.
	ActionGatewayCreate                 Action = "gateway:create"
	ActionGatewayUpdate                 Action = "gateway:update"
	ActionGatewayDelete                 Action = "gateway:delete"
	ActionGatewayAssignEnvironment      Action = "gateway:assign-environment"
	ActionGatewayUnassignEnvironment    Action = "gateway:unassign-environment"
	ActionGatewaySetIdentityProvider    Action = "gateway:set-identity-provider"
	ActionGatewayRemoveIdentityProvider Action = "gateway:remove-identity-provider"
	ActionGatewayPushManifest           Action = "gateway:push-manifest"

	// Score publishing by the evaluation job.
	ActionMonitorScorePublish Action = "monitor-score:publish"

	// Monitors: scheduled evaluations that read agent traces and call an LLM
	// judge, so starting or rerunning one spends money and reads data.
	ActionMonitorCreate  Action = "monitor:create"
	ActionMonitorUpdate  Action = "monitor:update"
	ActionMonitorDelete  Action = "monitor:delete"
	ActionMonitorStart   Action = "monitor:start"
	ActionMonitorStop    Action = "monitor:stop"
	ActionMonitorRerun   Action = "monitor:rerun"
	ActionMonitorRunFail Action = "monitor:run-failed"

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
	ResourceAPIKey         = "api-key"
	ResourceGitSecret      = "git-secret"
	ResourceGateway        = "gateway"
	ResourceGatewayToken   = "gateway-token"
	ResourceAgent          = "agent"
	ResourceRole           = "role"
	ResourceGroup          = "group"
	ResourceUser           = "user"
	ResourceAgentIdentity  = "agent-identity"
	ResourceEnvironment    = "environment"
	ResourceServiceAccount = "service-account"
	ResourceThunderURL     = "thunder-url"
)

// class, severity and permitted detail keys next to the action itself keeps the
// three from drifting apart.
//
//nolint:gochecknoinits // The registry is package state; declaring each action's
func init() {
	// Credential lifecycle. All critical: these grant or withdraw access.
	registerCredential(ActionAPIKeyCreate, map[string]FieldKind{
		"ownerType":        KindEnum,
		"ownerName":        KindName,
		"keyName":          KindName,
		"gatewayCount":     KindCount,
		"gatewayConnected": KindFlag,
		"expiresAt":        KindName,
	})
	registerCredential(ActionAPIKeyRotate, map[string]FieldKind{
		"ownerType":        KindEnum,
		"ownerName":        KindName,
		"keyName":          KindName,
		"gatewayCount":     KindCount,
		"gatewayConnected": KindFlag,
		"expiresAt":        KindName,
	})
	registerCredential(ActionAPIKeyRevoke, map[string]FieldKind{
		"ownerType":    KindEnum,
		"ownerName":    KindName,
		"keyName":      KindName,
		"gatewayCount": KindCount,
	})
	// Test keys are short-lived and scoped to one console session, so they are
	// recorded at a lower severity than a real key issue.
	Register(ActionAPIKeyIssueTest, ClassCredential, SeverityNotice)
	RegisterDetailSchema(ActionAPIKeyIssueTest, map[string]FieldKind{
		"ownerType": KindEnum,
		"ownerName": KindName,
		"keyName":   KindName,
		"expiresAt": KindName,
		"rotated":   KindFlag,
	})

	// A read, not a change, so it is a notice rather than critical.
	Register(ActionAPIKeySync, ClassRead, SeverityNotice)
	RegisterDetailSchema(ActionAPIKeySync, map[string]FieldKind{
		"keyCount": KindCount,
	})

	registerCredential(ActionGitSecretCreate, map[string]FieldKind{
		"secretType": KindEnum,
		// The username is recorded; the password never reaches this package.
		"username": KindName,
	})
	registerCredential(ActionGitSecretDelete, nil)

	registerCredential(ActionGatewayTokenRotate, map[string]FieldKind{
		"gatewayName": KindName,
		"tokenId":     KindIdentifier,
		"expiresAt":   KindName,
	})
	registerCredential(ActionGatewayTokenRevoke, map[string]FieldKind{
		"gatewayName": KindName,
		"tokenId":     KindIdentifier,
	})

	registerCredential(ActionAgentTokenMint, map[string]FieldKind{
		"agentName": KindName,
		"expiresIn": KindCount,
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

	// Cancelling deletes the WorkflowRun and the build's logs with it, so the
	// trail is the only remaining record that the build ever ran.
	Register(ActionAgentCancelBuild, ClassDeployment, SeverityNotice)
	RegisterDetailSchema(ActionAgentCancelBuild, map[string]FieldKind{
		"agentName":   KindName,
		"buildName":   KindName,
		"buildStatus": KindName,
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

	// Agent configuration. These replace two log lines that were labelled as
	// audit records but were only slog output, with no actor and no durability.
	Register(ActionAgentConfigUpdate, ClassConfig, SeverityInfo)
	RegisterDetailSchema(ActionAgentConfigUpdate, map[string]FieldKind{
		"configName":    KindName,
		"configType":    KindEnum,
		"agentName":     KindName,
		"updatedFields": KindNameList,
	})
	Register(ActionAgentConfigDelete, ClassConfig, SeverityNotice)
	RegisterDetailSchema(ActionAgentConfigDelete, map[string]FieldKind{
		"configName":       KindName,
		"configType":       KindEnum,
		"agentName":        KindName,
		"environmentCount": KindCount,
	})

	// Gateway lifecycle and trust configuration.
	gatewayFields := map[string]FieldKind{
		"gatewayName": KindName,
		"gatewayType": KindEnum,
		"vhost":       KindName,
		"environment": KindName,
	}
	Register(ActionGatewayCreate, ClassConfig, SeverityNotice)
	RegisterDetailSchema(ActionGatewayCreate, gatewayFields)
	Register(ActionGatewayUpdate, ClassConfig, SeverityNotice)
	RegisterDetailSchema(ActionGatewayUpdate, gatewayFields)
	Register(ActionGatewayDelete, ClassConfig, SeverityWarning)
	RegisterDetailSchema(ActionGatewayDelete, gatewayFields)
	Register(ActionGatewayAssignEnvironment, ClassConfig, SeverityNotice)
	RegisterDetailSchema(ActionGatewayAssignEnvironment, gatewayFields)
	Register(ActionGatewayUnassignEnvironment, ClassConfig, SeverityNotice)
	RegisterDetailSchema(ActionGatewayUnassignEnvironment, gatewayFields)

	// Changing which identity provider a gateway trusts changes who can mint
	// tokens it will accept, so these rank with credential changes rather than
	// with ordinary gateway configuration.
	// jwksUri and skipTlsVerify are the trust inputs themselves: the first says
	// where the gateway fetches signing keys from, the second lets it fetch them
	// without validating the TLS certificate. An issuer alone does not say
	// whether the keys behind it were obtained safely.
	idpFields := map[string]FieldKind{
		"gatewayName":          KindName,
		"identityProviderName": KindName,
		"issuer":               KindName,
		"jwksUri":              KindURL,
		"skipTlsVerify":        KindFlag,
	}
	Register(ActionGatewaySetIdentityProvider, ClassCredential, SeverityCritical)
	RegisterDetailSchema(ActionGatewaySetIdentityProvider, idpFields)
	Register(ActionGatewayRemoveIdentityProvider, ClassCredential, SeverityCritical)
	RegisterDetailSchema(ActionGatewayRemoveIdentityProvider, idpFields)

	// A gateway reporting the policy manifest it has installed. Routine, but
	// recorded because it is the only mutating route on the unauthenticated
	// internal server.
	Register(ActionGatewayPushManifest, ClassConfig, SeverityInfo)
	RegisterDetailSchema(ActionGatewayPushManifest, map[string]FieldKind{
		"policyCount": KindCount,
	})

	// The evaluation job publishing monitor scores. This route carries no RBAC
	// permission at all, so its record is the only account of who wrote scores.
	Register(ActionMonitorScorePublish, ClassConfig, SeverityInfo)
	RegisterDetailSchema(ActionMonitorScorePublish, map[string]FieldKind{
		"monitorId":  KindIdentifier,
		"runId":      KindIdentifier,
		"scoreCount": KindCount,
	})

	// The provisioning reconciler issues real credentials with no request
	// behind it, so its outcomes are recorded with a system actor. The user who
	// originally asked for the agent is carried as OnBehalfOf, which is what the
	// requested_by column on the binding was captured for.
	monitorFields := map[string]FieldKind{
		"monitorName": KindName,
		"agentName":   KindName,
		"monitorType": KindEnum,
		"evaluators":  KindNameList,
	}
	Register(ActionMonitorCreate, ClassConfig, SeverityInfo)
	RegisterDetailSchema(ActionMonitorCreate, monitorFields)
	Register(ActionMonitorUpdate, ClassConfig, SeverityInfo)
	RegisterDetailSchema(ActionMonitorUpdate, monitorFields)
	Register(ActionMonitorDelete, ClassConfig, SeverityNotice)
	RegisterDetailSchema(ActionMonitorDelete, monitorFields)
	// Start and stop decide whether an evaluation keeps spending against the
	// org's LLM provider, so they rank above ordinary configuration edits.
	Register(ActionMonitorStart, ClassConfig, SeverityNotice)
	RegisterDetailSchema(ActionMonitorStart, monitorFields)
	Register(ActionMonitorStop, ClassConfig, SeverityNotice)
	RegisterDetailSchema(ActionMonitorStop, monitorFields)
	Register(ActionMonitorRerun, ClassConfig, SeverityNotice)
	RegisterDetailSchema(ActionMonitorRerun, map[string]FieldKind{
		"monitorName": KindName,
		"agentName":   KindName,
		"runId":       KindIdentifier,
	})
	// A scheduled run that failed. Recorded by the executor with a system actor;
	// successful runs are not, since their scores are the record.
	Register(ActionMonitorRunFail, ClassSystem, SeverityWarning)
	RegisterDetailSchema(ActionMonitorRunFail, map[string]FieldKind{
		"monitorName": KindName,
		"agentName":   KindName,
		"runId":       KindIdentifier,
		"reason":      KindName,
	})

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
