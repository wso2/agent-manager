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

import (
	"strings"
	"sync/atomic"
)

// Action names the operation that was performed, in "<resource>:<verb>" form
// ("agent:create", "role:grant-permission"). It is the human- and SIEM-facing
// label.
//
// The route pattern recorded alongside it (Event.RequestPath) is the exact
// machine key. Keeping the two separate is what makes it safe to derive most
// actions from the route's rbac.Permission: if a derived label is imperfect for
// some endpoint, queries on the pattern are still precise.
type Action string

// ActionClass groups actions for coarse filtering and alerting.
type ActionClass string

const (
	// ClassAuthn covers token acceptance and rejection at the edge.
	ClassAuthn ActionClass = "authn"
	// ClassAuthz covers permission decisions, principally denials.
	ClassAuthz ActionClass = "authz"
	// ClassCredential covers API keys, tokens, secrets and OAuth clients —
	// anything whose compromise grants access.
	ClassCredential ActionClass = "credential"
	// ClassIdentity covers users, groups, roles and permission assignment: the
	// privilege-escalation surface.
	ClassIdentity ActionClass = "identity"
	// ClassDeployment covers build, deploy, promote and lifecycle state changes.
	ClassDeployment ActionClass = "deployment"
	// ClassConfig covers all other mutations.
	ClassConfig ActionClass = "config"
	// ClassRead covers the opt-in sensitive reads (see sensitiveReadPatterns).
	ClassRead ActionClass = "read"
	// ClassSystem covers AMS-initiated events: startup posture, reconcilers, purge.
	ClassSystem ActionClass = "system"
)

// Platform-level actions. Unlike resource actions these are not derived from a
// route, so they are named explicitly.
const (
	// ActionAuthnFailure is a rejected token at the edge. Recorded because the
	// service currently logs JWT validation failures without subject, source IP
	// or path, which makes credential stuffing undetectable.
	ActionAuthnFailure Action = "authn:failure"
	// ActionAuthzDeny is a refused authorization check. Denied privilege
	// escalation is the single most important thing a security reviewer looks
	// for, and it is invisible today.
	ActionAuthzDeny Action = "authz:deny"
	// ActionAuthzOrgOverride records a request whose organization was switched
	// from its token's to the one named in X-Impersonate-Org, as permitted by
	// the deployment's OrgOverrideResolver. The handler then reads and writes
	// another tenant's data, so it is recorded even though the request
	// succeeded. The event is filed under the target org; ActorOUID keeps the
	// token's.
	ActionAuthzOrgOverride Action = "authz:org-override"
	// ActionSystemStartup records that the audit trail began, which bounds any
	// gap in the record to a restart.
	ActionSystemStartup Action = "system:startup"
	// ActionSystemAuditDropped records that the buffer overflowed and events
	// were lost. A trail that silently drops records is worse than one that
	// admits it.
	ActionSystemAuditDropped Action = "system:audit-dropped"
)

type actionMeta struct {
	class    ActionClass
	severity Severity
}

// registry holds explicitly declared actions. Actions derived from a route's
// permission are classified heuristically instead (see classify).
var registry = map[Action]actionMeta{
	ActionAuthnFailure:       {class: ClassAuthn, severity: SeverityWarning},
	ActionAuthzDeny:          {class: ClassAuthz, severity: SeverityWarning},
	ActionAuthzOrgOverride:   {class: ClassAuthz, severity: SeverityCritical},
	ActionSystemStartup:      {class: ClassSystem, severity: SeverityNotice},
	ActionSystemAuditDropped: {class: ClassSystem, severity: SeverityCritical},
}

// Register declares an action's class and severity. Semantic emit sites call
// this from an init function so that adding an action forces a decision about
// how it is classified — and so the schema completeness test can enumerate them.
func Register(a Action, class ActionClass, severity Severity) {
	registry[a] = actionMeta{class: class, severity: severity}
}

// RegisteredActions returns every explicitly declared action. Used by tests.
func RegisteredActions() []Action {
	out := make([]Action, 0, len(registry))
	for a := range registry {
		out = append(out, a)
	}
	return out
}

// ModuleAction classifies an audit action an active module declares.
type ModuleAction struct {
	Class    ActionClass
	Severity Severity
	// Details are the detail keys the action's records may carry.
	Details map[string]FieldKind
}

// moduleActions are the actions the active modules declare. The module host
// replaces them whenever the set of active modules changes.
var moduleActions atomic.Pointer[map[Action]ModuleAction]

// SetModuleActions replaces the actions the active modules declare.
func SetModuleActions(m map[Action]ModuleAction) {
	moduleActions.Store(&m)
}

func moduleAction(a Action) (ModuleAction, bool) {
	if m := moduleActions.Load(); m != nil {
		meta, ok := (*m)[a]
		return meta, ok
	}
	return ModuleAction{}, false
}

// Class returns the action's class, falling back to a heuristic for actions
// derived from route permissions.
func (a Action) Class() ActionClass {
	if meta, ok := registry[a]; ok {
		return meta.class
	}
	if meta, ok := moduleAction(a); ok {
		return meta.Class
	}
	class, _ := classify(a)
	return class
}

// Severity returns the action's severity, falling back to a heuristic.
func (a Action) Severity() Severity {
	if meta, ok := registry[a]; ok {
		return meta.severity
	}
	if meta, ok := moduleAction(a); ok {
		return meta.Severity
	}
	_, severity := classify(a)
	return severity
}

// Resource returns the leading "<resource>" segment of the action.
func (a Action) Resource() string {
	if idx := strings.Index(string(a), ":"); idx != -1 {
		return string(a)[:idx]
	}
	return string(a)
}

// Verb returns the trailing "<verb>" segment of the action. A module's
// action reads "<module>:<resource>.<verb>"; its verb is the last segment.
func (a Action) Verb() string {
	if idx := strings.Index(string(a), ":"); idx != -1 {
		verb := string(a)[idx+1:]
		if dot := strings.LastIndex(verb, "."); dot != -1 {
			return verb[dot+1:]
		}
		return verb
	}
	return ""
}

// credentialResources are resources whose mutation hands out or withdraws
// access. They are classified as credential events regardless of verb.
var credentialResources = map[string]bool{
	"api-key":        true,
	"agent-identity": true,
	"gateway-token":  true,
	"agent-token":    true,
	"token":          true,
}

// identityResources are the privilege-management surface.
var identityResources = map[string]bool{
	"user":  true,
	"group": true,
	"role":  true,
	"org":   true,
	"scope": true,
}

// deploymentVerbs mark lifecycle transitions wherever they appear.
//
// The environment-tier scopes are deliberately absent: they name where an
// operation lands rather than what it is, deriveAction filters them out before
// they can become an action, and a route left with nothing to derive from
// panics instead. There is no reachable "agent:env-production" to classify.
var deploymentVerbs = map[string]bool{
	"deploy": true, "undeploy": true, "promote": true, "build": true,
	"suspend": true, "resume": true, "restore": true, "rollback": true,
}

// credentialVerbs mark credential handling wherever it appears — an
// "agent:token-manage" is a credential event even though "agent" is not a
// credential resource.
var credentialVerbs = map[string]bool{
	"api-key-manage": true, "token-manage": true, "rotate": true,
	"revoke": true, "regenerate": true, "regenerate-secret": true,
	"revoke-secret": true, "provision": true, "manage-service-account": true,
	"mint": true, "issue-test": true,
}

// readVerbs name actions that disclose rather than change. They are checked
// before the credential and identity rules so that listing API keys is not
// ranked as loudly as rotating one — both are recorded, but only one is a change.
var readVerbs = map[string]bool{
	"read": true, "view": true, "list": true,
	"list-assignments": true, "list-identity-providers": true, "fetch-server-info": true,
}

// classify derives a class and severity for an action that was not explicitly
// registered — which is the normal case for the coverage tier, where the action
// comes from the route's rbac.Permission.
//
// Verb is checked before resource: the verb describes the effect, and the
// effect is what an auditor filters on.
func classify(a Action) (ActionClass, Severity) {
	resource, verb := a.Resource(), a.Verb()

	switch {
	case strings.HasPrefix(string(a), "authn:"):
		return ClassAuthn, SeverityWarning
	case strings.HasPrefix(string(a), "authz:"):
		return ClassAuthz, SeverityWarning
	case strings.HasPrefix(string(a), "system:"):
		return ClassSystem, SeverityNotice

	// Reads are checked before the credential and identity rules: disclosing a
	// credential is worth recording, but it is not the same event as changing
	// one, and ranking them alike would bury the changes in noise.
	case readVerbs[verb]:
		if credentialResources[resource] || identityResources[resource] {
			return ClassRead, SeverityNotice
		}
		return ClassRead, SeverityInfo

	case credentialVerbs[verb] || credentialResources[resource]:
		return ClassCredential, SeverityCritical
	case identityResources[resource]:
		return ClassIdentity, SeverityCritical
	case deploymentVerbs[verb]:
		return ClassDeployment, SeverityNotice
	case verb == "delete":
		return ClassConfig, SeverityNotice
	default:
		return ClassConfig, SeverityInfo
	}
}
