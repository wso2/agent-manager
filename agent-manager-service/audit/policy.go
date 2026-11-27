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
	// "POST /agents".
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
	// FailClosed writes an intent record before the handler runs and refuses
	// the request when it cannot be written (modkit.AuditFailClosed).
	FailClosed bool
	// ScopeFromPath names the record's project and environment from the
	// request's path and query, for module routes, whose handlers name only
	// what they acted on.
	ScopeFromPath bool
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

// RoutePolicy is what a module route declares about its audit record, in
// place of the core's tables of overrides, sensitive reads and coalesce
// windows, which are keyed by core route patterns.
type RoutePolicy struct {
	// Action, when set, names the route's action instead of its permission.
	Action Action
	// Read records the route even though it is a read.
	Read bool
	// Coalesce records the route at most once per caller per window.
	Coalesce time.Duration
	// FailClosed refuses the request when its intent record cannot be written.
	FailClosed bool
}

// NewRouteMetaWithPolicy resolves the audit policy for a module route. As for
// a core route, every route that changes state is audited, and one that
// yields no action panics.
func NewRouteMetaWithPolicy(pattern string, params []string, perms []rbac.Permission, surface Surface, p RoutePolicy) RouteMeta {
	method, path := splitPattern(pattern)
	meta := RouteMeta{
		Pattern:       pattern,
		Method:        method,
		Path:          path,
		Params:        params,
		Perms:         perms,
		Audited:       shouldAudit(method, path) || p.Read || p.FailClosed,
		Surface:       surface,
		Coalesce:      p.Coalesce,
		FailClosed:    p.FailClosed,
		ScopeFromPath: true,
	}
	if !meta.Audited {
		return meta
	}
	meta.Action = p.Action
	if meta.Action == "" {
		meta.Action = deriveAction(method, path, perms)
	}
	if meta.Action == "" {
		panic(fmt.Sprintf("audit: cannot derive an action for audited route %q; name it with modkit.Audit", pattern))
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
// Deliberately narrow: mcp-proxies/fetch-server-info is also technically a
// read, but it makes outbound calls with caller-supplied credentials, so it
// stays audited.
var nonMutatingWritePaths = map[string]bool{
	// Suggests an available agent name. Pure function, no persistence.
	"/utils/generate-name": true,
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
// The AI Gateway's polled routes declare theirs with modkit.AuditCoalesce.
var coalesceWindows = map[string]time.Duration{}

var sensitiveReadPaths = map[string]bool{
	// Credential material.
	"/agents/{agentName}/identities": true,

	// Security configuration — who holds which privilege.
	"/identities/permissions":                true,
	"/identities/roles/{roleID}/assignments": true,
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
//  2. The permission names a different resource than the operation acts on.
//  3. The permission is narrower or broader than the effect.
//
// Keys are matched against the path with the method prefix stripped, so the
// same path under different methods maps through methodActionOverrides first.
var actionOverrides = map[string]Action{
	// Event destinations and subscriptions are gated by the permission of
	// the scope they belong to (events, project or agent), which says nothing
	// about the operation.
	"POST /events/destinations":                                                            "event-destination:create",
	"PUT /events/destinations/{destinationId}":                                             "event-destination:update",
	"DELETE /events/destinations/{destinationId}":                                          "event-destination:delete",
	"POST /events/destinations/{destinationId}/rotate-secret":                              "event-destination:rotate-secret",
	"POST /events/destinations/{destinationId}/test":                                       "event-destination:test",
	"POST /events/subscriptions":                                                           "event-subscription:create",
	"PUT /events/subscriptions/{subscriptionId}":                                           "event-subscription:update",
	"DELETE /events/subscriptions/{subscriptionId}":                                        "event-subscription:delete",
	"POST /projects/{projName}/events/subscriptions":                                       "event-subscription:create",
	"PUT /projects/{projName}/events/subscriptions/{subscriptionId}":                       "event-subscription:update",
	"DELETE /projects/{projName}/events/subscriptions/{subscriptionId}":                    "event-subscription:delete",
	"POST /projects/{projName}/agents/{agentName}/events/subscriptions":                    "event-subscription:create",
	"PUT /projects/{projName}/agents/{agentName}/events/subscriptions/{subscriptionId}":    "event-subscription:update",
	"DELETE /projects/{projName}/agents/{agentName}/events/subscriptions/{subscriptionId}": "event-subscription:delete",

	// One permission, several operations — agent OAuth identity.
	"PUT /agents/{agentName}/identities":        "agent-identity:provision",
	"POST /agents/{agentName}/identities":       "agent-identity:regenerate-secret",
	"DELETE /agents/{agentName}/identities":     "agent-identity:revoke-secret",
	"POST /agents/{agentName}/identities/retry": "agent-identity:retry-provisioning",

	// One permission, several operations — agent tokens.
	"POST /agents/{agentName}/token": "agent-token:mint",

	// The privilege-escalation path. All four are gated by role:update, and
	// telling them apart is the whole point of an audit trail.
	"POST /identities/roles/{roleID}/permissions/add":    "role:grant-permission",
	"POST /identities/roles/{roleID}/permissions/remove": "role:revoke-permission",
	"POST /identities/roles/{roleID}/assignees/add":      "role:assign",
	"POST /identities/roles/{roleID}/assignees/remove":   "role:unassign",
	"POST /identities/groups/{groupID}/members/add":      "group:add-member",
	"POST /identities/groups/{groupID}/members/remove":   "group:remove-member",

	// Org membership. Gated by org:invite-member / org:remove-member, which
	// describe the grant rather than the operation.
	"POST /identities/users/invite":     "user:invite",
	"POST /identities/users":            "user:create",
	"PUT /identities/users/{userID}":    "user:update",
	"DELETE /identities/users/{userID}": "user:delete",

	// Sensitive reads.
	"GET /agents/{agentName}/identities":         "agent-identity:read",
	"GET /identities/permissions":                "permission:list",
	"GET /identities/roles/{roleID}/assignments": "role:list-assignments",
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
	if len(perms) == 1 {
		return Action(perms[0])
	}
	if len(perms) > 1 {
		verb := methodVerbs[method]
		if verb == "" {
			verb = strings.ToLower(method)
		}
		return Action(Action(perms[0]).Resource() + ":" + verb)
	}
	return ""
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
