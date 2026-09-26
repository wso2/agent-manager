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
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// RouteMeta is everything the route registrar knows about a route at
// registration time. Resolving the audit decision once, at startup, keeps the
// request path free of policy lookups and lets the coverage test inspect it.
type RouteMeta struct {
	// Pattern is the registrar pattern, e.g.
	// "POST /orgs/{orgName}/projects/{projName}/agents".
	Pattern string
	Method  string
	// Path is Pattern with the method stripped.
	Path string
	// Params are the path parameter names declared in the pattern.
	Params []string
	// Perms are the permissions that gate the route. Empty for the handful of
	// routes registered without authorization.
	Perms []rbac.Permission
	// Action is the audit label for this route.
	Action Action
	// Audited reports whether this route emits audit events.
	Audited bool
	// Surface is the entry point this route belongs to. The internal
	// gateway-facing server registers through the same registrar but must not
	// be recorded as ordinary API traffic.
	Surface Surface
	// Coalesce suppresses repeats of this route within the window, per caller.
	// Zero means record every call. Non-zero only for routes a machine polls on
	// a timer, where one record per request would bury everything else.
	Coalesce time.Duration
}

var pathParamPattern = regexp.MustCompile(`\{([^}]+)\}`)

// NewRouteMeta resolves the audit policy for a route.
//
// It panics when an audited route yields no action. That is deliberate and
// mirrors the existing fail-closed discipline in mcp/tools/authz.go, which
// panics on a tool registered without permissions: a route that cannot be
// labelled must fail at startup, in CI, rather than ship as an unattributable
// gap in the trail.
func NewRouteMeta(pattern string, params []string, perms []rbac.Permission) RouteMeta {
	return NewRouteMetaForSurface(pattern, params, perms, SurfaceAPI)
}

// NewRouteMetaForSurface resolves the audit policy for a route on a named
// surface. The internal gateway server uses this so its records are not
// mistaken for user-driven API traffic.
func NewRouteMetaForSurface(pattern string, params []string, perms []rbac.Permission, surface Surface) RouteMeta {
	method, path := splitPattern(pattern)
	meta := RouteMeta{
		Pattern:  pattern,
		Method:   method,
		Path:     path,
		Params:   params,
		Perms:    perms,
		Audited:  shouldAudit(method, path),
		Surface:  surface,
		Coalesce: coalesceWindows[path],
	}
	if !meta.Audited {
		return meta
	}
	meta.Action = deriveAction(method, path, perms)
	if meta.Action == "" {
		panic(fmt.Sprintf("audit: cannot derive an action for audited route %q; "+
			"add an entry to actionOverrides in audit/policy.go", pattern))
	}
	return meta
}

// splitPattern separates "POST /orgs/{orgName}/x" into method and path.
func splitPattern(pattern string) (method, path string) {
	if idx := strings.Index(pattern, " "); idx != -1 {
		return pattern[:idx], strings.TrimSpace(pattern[idx+1:])
	}
	// A pattern with no method matches every method.
	return "", pattern
}

// ExtractPathParams returns the parameter names declared in a route pattern.
func ExtractPathParams(pattern string) []string {
	matches := pathParamPattern.FindAllStringSubmatch(pattern, -1)
	params := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) > 1 {
			params = append(params, strings.TrimSpace(m[1]))
		}
	}
	return params
}

// shouldAudit decides whether a route emits audit events.
//
// Every state-changing route is audited. Reads are not, except for an explicit
// allow-list of routes that return credential material or security
// configuration — auditing every GET would multiply volume several-fold for
// little forensic gain.
func shouldAudit(method, path string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return sensitiveReadPaths[path]
	default:
		return !nonMutatingWritePaths[path]
	}
}

// nonMutatingWritePaths are POST routes that change no state. They are excluded
// to keep the trail signal-dense.
//
// Deliberately narrow: POST /repositories/* and mcp-proxies/fetch-server-info
// are also technically reads, but both make outbound calls with caller-supplied
// credentials, so they stay audited.
var nonMutatingWritePaths = map[string]bool{
	// Suggests an available agent name. Pure function, no persistence.
	"/orgs/{orgName}/utils/generate-name": true,
	// Reports console UI activity (page views, dialogs opened) to the
	// analytics collector. Changes no platform state, and is emitted
	// continuously by every open browser tab — auditing it would bury the
	// trail under telemetry without recording a single thing a user changed.
	"/telemetry/console-actions": true,
}

// sensitiveReadPaths are GET routes that disclose credential material or the
// security configuration of the org. Reading these is worth recording even
// though nothing changes.
//
// The audit trail is stored outside this service, so there is no read route for
// it here to add to this list.
// coalesceWindows bound how often a route is recorded per caller.
//
// Gateways poll the bulk-sync endpoints on a timer, so recording every call
// would produce millions of near-identical records and bury everything else.
// One record per gateway per window still answers the question that matters —
// which gateway pulled key material, and when it started.
var coalesceWindows = map[string]time.Duration{
	"/llm-providers/api-keys": 5 * time.Minute,
	"/llm-proxies/api-keys":   5 * time.Minute,
	"/apis/api-keys":          5 * time.Minute,
}

var sensitiveReadPaths = map[string]bool{
	// Credential material.
	"/orgs/{orgName}/git-secrets":                                                                                     true,
	"/orgs/{orgName}/llm-providers/{id}/api-keys":                                                                     true,
	"/orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys":                                                   true,
	"/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys":                            true,
	"/orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}/environments/{envName}/api-keys":   true,
	"/orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}/environments/{envName}/api-keys": true,
	"/orgs/{orgName}/gateways/{gatewayID}/tokens":                                                                     true,
	"/orgs/{orgName}/projects/{projName}/agents/{agentName}/identities":                                               true,

	// Internal surface: the gateway bulk-sync endpoints hand real key material
	// to a data-plane gateway. The public equivalents above are audited, and a
	// compromised gateway credential harvesting keys is exactly what these
	// records would show. Coalesced — see coalesceWindows.
	"/llm-providers/api-keys": true,
	"/llm-proxies/api-keys":   true,
	"/apis/api-keys":          true,

	// Security configuration — who holds which privilege.
	"/orgs/{orgName}/identities/permissions":                          true,
	"/orgs/{orgName}/identities/roles/{roleID}/assignments":           true,
	"/orgs/{orgName}/gateways/{gatewayID}/identity-providers":         true,
	"/orgs/{orgName}/environments/{environmentId}/identity-providers": true,
}

// actionOverrides maps a route path to the action it actually performs, for
// routes whose gating permission does not describe the effect.
//
// Most routes need no entry: their rbac.Permission is already "<resource>:<verb>"
// and says exactly what happened. Overrides exist for three recurring cases:
//
//  1. One permission gates several distinct operations. Every API-key route is
//     gated by "*:api-key-manage", so create, rotate and revoke would otherwise
//     be indistinguishable — precisely the distinction an auditor needs.
//  2. The permission names a different resource than the operation acts on
//     (publish-kind is gated by agent-kind:create but acts on an agent).
//  3. The permission is narrower or broader than the effect
//     (deployments/state is gated by agent:suspend but also resumes).
//
// Keys are matched against the path with the method prefix stripped, so the
// same path under different methods maps through methodActionOverrides first.
var actionOverrides = map[string]Action{
	// Gating permission names a different resource than the effect.
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/publish-kind": "agent-kind:publish",

	// One permission, several operations — deployment lifecycle.
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments": "agent:deploy",
	// Gated on the environment tier, not on a promote-specific scope, so the
	// route declares no permission that names the operation. Without this
	// override there is nothing to derive from and registration panics.
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/promote":                    "agent:promote",
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/deployments/state":          "agent:change-deployment-state",
	"POST /orgs/{orgName}/llm-providers/{providerId}/deployments":                            "llm-provider:deploy",
	"POST /orgs/{orgName}/llm-providers/{providerId}/deployments/undeploy":                   "llm-provider:undeploy",
	"POST /orgs/{orgName}/llm-providers/{providerId}/deployments/restore":                    "llm-provider:restore",
	"DELETE /orgs/{orgName}/llm-providers/{providerId}/deployments/{deploymentId}":           "llm-provider:delete-deployment",
	"POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments":                  "llm-proxy:deploy",
	"POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments/undeploy":         "llm-proxy:undeploy",
	"POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments/restore":          "llm-proxy:restore",
	"DELETE /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/deployments/{deploymentId}": "llm-proxy:delete-deployment",

	// One permission, several operations — agent OAuth identity.
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities":        "agent-identity:provision",
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities":       "agent-identity:regenerate-secret",
	"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities":     "agent-identity:revoke-secret",
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities/retry": "agent-identity:retry-provisioning",

	// One permission, several operations — agent tokens.
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/token":                    "agent-token:mint",
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/tracing-token/regenerate": "agent-token:regenerate-tracing",

	// One permission, several operations — agent API keys.
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys":             "api-key:create",
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/test":        "api-key:issue-test",
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/{keyName}":    "api-key:rotate",
	"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/{keyName}": "api-key:revoke",

	// One permission, several operations — LLM provider / proxy API keys.
	"POST /orgs/{orgName}/llm-providers/{id}/api-keys":                               "api-key:create",
	"PUT /orgs/{orgName}/llm-providers/{id}/api-keys/{keyName}":                      "api-key:rotate",
	"DELETE /orgs/{orgName}/llm-providers/{id}/api-keys/{keyName}":                   "api-key:revoke",
	"POST /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys":             "api-key:create",
	"PUT /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys/{keyName}":    "api-key:rotate",
	"DELETE /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys/{keyName}": "api-key:revoke",

	// One permission, several operations — model/MCP config API keys.
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}/environments/{envName}/api-keys":             "api-key:create",
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}/environments/{envName}/api-keys/{keyName}":    "api-key:rotate",
	"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}/environments/{envName}/api-keys/{keyName}": "api-key:revoke",
	"POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}/environments/{envName}/api-keys":               "api-key:create",
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}/environments/{envName}/api-keys/{keyName}":      "api-key:rotate",
	"DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}/environments/{envName}/api-keys/{keyName}":   "api-key:revoke",

	// One permission, several operations — gateway tokens and configuration.
	"POST /orgs/{orgName}/gateways/{gatewayID}/tokens":                      "gateway-token:rotate",
	"DELETE /orgs/{orgName}/gateways/{gatewayID}/tokens/{tokenID}":          "gateway-token:revoke",
	"POST /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}":        "gateway:assign-environment",
	"DELETE /orgs/{orgName}/gateways/{gatewayID}/environments/{envID}":      "gateway:unassign-environment",
	"PUT /orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}":    "gateway:set-identity-provider",
	"DELETE /orgs/{orgName}/gateways/{gatewayID}/identity-providers/{name}": "gateway:remove-identity-provider",

	// The privilege-escalation path. All four are gated by role:update, and
	// telling them apart is the whole point of an audit trail.
	"POST /orgs/{orgName}/identities/roles/{roleID}/permissions/add":    "role:grant-permission",
	"POST /orgs/{orgName}/identities/roles/{roleID}/permissions/remove": "role:revoke-permission",
	"POST /orgs/{orgName}/identities/roles/{roleID}/assignees/add":      "role:assign",
	"POST /orgs/{orgName}/identities/roles/{roleID}/assignees/remove":   "role:unassign",
	"POST /orgs/{orgName}/identities/groups/{groupID}/members/add":      "group:add-member",
	"POST /orgs/{orgName}/identities/groups/{groupID}/members/remove":   "group:remove-member",

	// Org membership. Gated by org:invite-member / org:remove-member, which
	// describe the grant rather than the operation.
	"POST /orgs/{orgName}/identities/users/invite":     "user:invite",
	"POST /orgs/{orgName}/identities/users":            "user:create",
	"PUT /orgs/{orgName}/identities/users/{userID}":    "user:update",
	"DELETE /orgs/{orgName}/identities/users/{userID}": "user:delete",

	// Per-environment agent identities, all gated by agent-identity:update.
	"POST /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}/members/add":      "agent-identity:add-group-member",
	"POST /orgs/{orgName}/environments/{envName}/agent-identities/groups/{groupID}/members/remove":   "agent-identity:remove-group-member",
	"POST /orgs/{orgName}/environments/{envName}/agent-identities/roles/{roleID}/assignments/add":    "agent-identity:assign-role",
	"POST /orgs/{orgName}/environments/{envName}/agent-identities/roles/{roleID}/assignments/remove": "agent-identity:unassign-role",

	// The env-Thunder system-client credential, gated by org:manage-service-account.
	"PUT /orgs/{orgName}/environments/{envID}/thunder-system-client":    "service-account:configure",
	"DELETE /orgs/{orgName}/environments/{envID}/thunder-system-client": "service-account:remove",

	// Agent sub-resource updates, all gated by agent:update.
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/build-parameters": "agent:update-build-parameters",
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/configurations":   "agent:update-configurations",
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/deploy-settings":  "agent:update-deploy-settings",
	"PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/resource-configs": "agent:update-resource-configs",

	// Agent-kind versions, gated by the coarse kind update/delete permissions.
	"POST /orgs/{orgName}/agent-kinds/{kindName}/versions":                "agent-kind:add-version",
	"DELETE /orgs/{orgName}/agent-kinds/{kindName}/versions/{versionTag}": "agent-kind:delete-version",

	// Catalog is a distinct sub-resource of an LLM provider.
	"PUT /orgs/{orgName}/llm-providers/{providerId}/catalog": "llm-provider:update-catalog",

	// Internal gateway server. These carry no rbac.Permission — the gateway
	// authenticates with an api-key header checked inside each handler — so
	// each needs an explicit action.
	"POST /gateways/{gatewayId}/manifest": "gateway:push-manifest",
	"GET /llm-providers/api-keys":         "api-key:sync",
	"GET /llm-proxies/api-keys":           "api-key:sync",
	"GET /apis/api-keys":                  "api-key:sync",

	// Reads expressed as POST because they carry a request body.
	// The score-publish route carries no rbac.Permission at all — it is gated
	// only by an audience check — so there is nothing to derive from.
	"POST /publisher/monitors/{monitorId}/runs/{runId}/scores": "monitor-score:publish",

	"POST /orgs/{orgName}/repositories/branches":         "repository:list-branches",
	"POST /orgs/{orgName}/repositories/commits":          "repository:list-commits",
	"POST /orgs/{orgName}/mcp-proxies/fetch-server-info": "mcp-server:fetch-server-info",

	// Sensitive reads.
	"GET /orgs/{orgName}/git-secrets":                                                                                     "git-secret:list",
	"GET /orgs/{orgName}/llm-providers/{id}/api-keys":                                                                     "api-key:list",
	"GET /orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys":                                                   "api-key:list",
	"GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys":                            "api-key:list",
	"GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}/environments/{envName}/api-keys":   "api-key:list",
	"GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}/environments/{envName}/api-keys": "api-key:list",
	"GET /orgs/{orgName}/gateways/{gatewayID}/tokens":                                                                     "gateway-token:list",
	"GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities":                                               "agent-identity:read",
	"GET /orgs/{orgName}/identities/permissions":                                                                          "permission:list",
	"GET /orgs/{orgName}/identities/roles/{roleID}/assignments":                                                           "role:list-assignments",
	"GET /orgs/{orgName}/gateways/{gatewayID}/identity-providers":                                                         "gateway:list-identity-providers",
	"GET /orgs/{orgName}/environments/{environmentId}/identity-providers":                                                 "environment:list-identity-providers",
}

// methodVerbs map an HTTP method to a verb, used as the last fallback for
// routes registered without any permission.
var methodVerbs = map[string]string{
	http.MethodPost:   "create",
	http.MethodPut:    "update",
	http.MethodPatch:  "update",
	http.MethodDelete: "delete",
	http.MethodGet:    "read",
}

// deriveAction resolves the audit action for a route, in priority order:
//
//  1. an explicit override, for routes whose permission misdescribes the effect;
//  2. the sole gating permission verbatim — the common case, and why this scales
//     to hundreds of routes without hundreds of map entries;
//  3. for multi-permission routes, the first permission's resource with a verb
//     from the HTTP method, since no single permission is authoritative.
//
// Axis permissions are dropped before any of that: an environment tier says
// where an operation lands, never what it is, so it cannot name the action.
//
// A route left with no naming permission has nothing to derive from and returns
// empty, which makes NewRouteMeta panic. That is deliberate: such a route would
// otherwise be labelled from its path or from its axis, producing an action with
// no class, no severity and no detail schema that nothing would ever flag.
// Declare it in actionOverrides instead.
func deriveAction(method, path string, perms []rbac.Permission) Action {
	if override, ok := actionOverrides[method+" "+path]; ok {
		return override
	}
	named := namingPermissions(perms)
	if len(named) == 1 {
		return Action(named[0])
	}
	if len(named) > 1 {
		verb := methodVerbs[method]
		if verb == "" {
			verb = strings.ToLower(method)
		}
		return Action(Action(named[0]).Resource() + ":" + verb)
	}
	return ""
}

// axisPermissions gate an operation on where it lands rather than on what it is.
// They are an authorization axis of their own, held in addition to the
// capability the route demands, so they never describe the operation.
var axisPermissions = map[rbac.Permission]bool{
	rbac.AgentEnvNonProduction: true,
	rbac.AgentEnvProduction:    true,
}

// namingPermissions returns the permissions that can name an action — every one
// the route declares, less the axes. Filtering here rather than special-casing
// each tier-gated route is what makes the fail-closed guard cover the next one:
// a route gated only on an axis derives nothing and panics at startup, instead
// of quietly registering "agent:env-non-production" as an action.
func namingPermissions(perms []rbac.Permission) []rbac.Permission {
	named := make([]rbac.Permission, 0, len(perms))
	for _, perm := range perms {
		if !axisPermissions[perm] {
			named = append(named, perm)
		}
	}
	return named
}

// OverrideKeys returns every actionOverrides key. Used by the coverage test to
// detect entries that no longer match a registered route.
func OverrideKeys() []string {
	out := make([]string, 0, len(actionOverrides))
	for k := range actionOverrides {
		out = append(out, k)
	}
	return out
}

// SensitiveReadPaths returns every sensitive-read path. Used by the coverage test.
func SensitiveReadPaths() []string {
	out := make([]string, 0, len(sensitiveReadPaths))
	for k := range sensitiveReadPaths {
		out = append(out, k)
	}
	return out
}

// ExemptWritePaths returns the write paths deliberately excluded from the trail.
// Used by the coverage test, which otherwise requires every write to be audited.
func ExemptWritePaths() []string {
	out := make([]string, 0, len(nonMutatingWritePaths))
	for k := range nonMutatingWritePaths {
		out = append(out, k)
	}
	return out
}
