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

package api

import (
	"bufio"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Static authorization invariants over the route table.
//
// These complement the runtime scope-matrix suite in test/e2e/security/authz.
// That suite proves the guards behave; these prove no route escaped one in the
// first place. Statically checking the whole surface is cheap and exhaustive,
// which the runtime matrix cannot be — it would need a live cluster and a
// request per route.
//
// Both tests are deliberately bidirectional: a stale allowlist entry fails just
// as loudly as a missing one. An allowlist that silently keeps passing after
// the thing it excuses is fixed is how these invariants rot.

// authzBearingRegistrars are the RouteRegistrar methods that attach a
// permission check. Any other registration path leaves the route open to any
// authenticated caller.
var authzBearingRegistrars = map[string]bool{
	"HandleFuncWithValidationAndAuthz":            true,
	"HandleFuncWithValidationAndAuthzAllowRootOU": true,
	"HandleFuncWithValidationAndAnyAuthz":         true,
	"HandleFuncWithValidationAndAllAuthz":         true,
	"HandleFuncWithValidationAndDynamicAuthz":     true,
}

type routeRegistration struct {
	function    string
	registrar   string
	pattern     string
	permissions []string
}

type routeKey struct {
	function  string
	registrar string
	pattern   string
}

func (r routeRegistration) key() routeKey {
	return routeKey{function: r.function, registrar: r.registrar, pattern: r.pattern}
}

func (r routeRegistration) String() string {
	return r.function + ": " + r.registrar + "(" + r.pattern + ")"
}

// unguardedRouteAllowlist records every registration that deliberately skips
// a RouteRegistrar permission check. Keys include the exact registration call
// and pattern: replacing an exempt route while keeping the same number of raw
// registrations must not silently inherit the old exemption.
var unguardedRouteAllowlist = map[routeKey]string{
	{"registerAgentBuildOptionsRoutes", "HandleFuncWithValidation", "GET /orgs/{orgName}/agent-build-options"}: "GET /orgs/{orgName}/agent-build-options returns static build-option metadata " +
		"(no org data). Still org-resolved via RequireOrgMatch.",
	{"RegisterMonitorPublisherRoutes", "HandleFuncWithValidation", "POST /publisher/monitors/{monitorId}/runs/{runId}/scores"}: "POST /publisher/.../scores is guarded by jwtassertion.PublisherClientAuthMiddleware, " +
		"which requires an amp-publisher-* audience instead of an amp: scope.",
	{"RegisterWebSocketRoutes", "HandleFunc", "GET /ws/gateways/connect"}: "GET /ws/gateways/connect is on internalMux and authenticates the gateway with an " +
		"api-key header verified in the handler (gatewayService.VerifyToken), not a user JWT.",
	{"registerJWKSRoute", "HandleFunc", "GET /auth/external/jwks.json"}: "publishes public signing keys and must be unauthenticated.",
	{"registerHealthCheck", "HandleFunc", "GET /healthz"}:               "unauthenticated liveness probe.",
	{"registerThunderAskRoute", "HandleFunc", "GET /internal/thunder-ask"}: "public Caddy on-demand TLS ask endpoint; it validates the requested environment handle, " +
		"fails closed on lookup errors, and is rate-limited instead of using user RBAC.",
	{"registerWellKnownRoutes", "HandleFunc", "GET /.well-known/oauth-protected-resource"}: "GET /.well-known/oauth-protected-resource is RFC 9728 resource metadata and must be " +
		"unauthenticated so clients can discover how to authenticate.",
	{"registerWellKnownRoutes", "HandleFunc", "GET /.well-known/oauth-protected-resource/mcp"}: "path-specific RFC 9728 metadata for the MCP resource must be unauthenticated " +
		"so MCP clients can discover how to authenticate.",
	{"registerConfigRoutes", "Handle", "/api/v1/config"}:       "public console/CLI discovery endpoint; exposes only the observer public URL.",
	{"MakeHTTPHandler", "Handle", "/api/v1/"}:                  "mount point for the JWT-authenticated API sub-router, not a leaf endpoint.",
	{"MakeInternalHTTPHandler", "HandleFunc", "/health"}:       "internal-listener liveness probe.",
	{"MakeInternalHTTPHandler", "Handle", "/api/internal/v1/"}: "mount point for the separately exposed internal API sub-router.",
	{"RegisterRoute", "Handle", "/mcp"}:                        "MCP endpoint wrapped by JWT authentication and MCP per-tool scope authorization.",
	{"RegisterRoute", "Handle", "/mcp/"}:                       "MCP endpoint wrapped by JWT authentication and MCP per-tool scope authorization.",
}

var gatewayInternalRoutes = []string{
	"GET /llm-providers/api-keys",
	"GET /llm-proxies/api-keys",
	"GET /apis/api-keys",
	"GET /subscription-plans",
	"GET /deployments",
	"GET /applications",
	"POST /gateways/{gatewayId}/manifest",
	"GET /llm-providers/{providerId}",
	"GET /llm-proxies/{proxyId}",
	"GET /mcp-proxies/{proxyId}",
}

func init() {
	for _, pattern := range gatewayInternalRoutes {
		unguardedRouteAllowlist[routeKey{"RegisterGatewayInternalRoutes", "HandleFuncWithValidation", pattern}] = "gateway-to-control-plane endpoint on the separately exposed internal listener."
	}
}

// TestSecurityInvariantEveryRouteIsAuthorized fails when a route is registered without a
// per-route permission check and is not on the documented allowlist.
//
// This is the invariant that makes the runtime scope matrix tractable: because
// no route can quietly skip authorization, the e2e suite only has to spot-check
// that the wiring behaves, not enumerate all 231 registrations.
func TestSecurityInvariantEveryRouteIsAuthorized(t *testing.T) {
	found := map[routeKey]bool{}

	forEachRouteRegistration(t, func(route routeRegistration) {
		if authzBearingRegistrars[route.registrar] {
			return
		}
		found[route.key()] = true
		if _, ok := unguardedRouteAllowlist[route.key()]; !ok {
			t.Errorf("%s has no RouteRegistrar permission check and is not allowlisted.\n\n"+
				"Use an authz-bearing registrar, or add the exact function/registrar/pattern key to "+
				"unguardedRouteAllowlist with a security rationale.", route)
		}
	})

	for key, reason := range unguardedRouteAllowlist {
		if !found[key] {
			t.Errorf("stale unguarded-route allowlist entry %+v. Remove it.\nIt was allowlisted because: %s",
				key, reason)
		}
	}
}

// declaredButUnenforcedPermissions are rbac.Permission constants that no route
// in this package references.
//
// A permission that exists, is granted to predefined roles, and gates nothing
// is a false sense of least privilege: the role matrix reads as if the boundary
// is enforced when the API has no such check. Each entry below must say where
// the boundary actually lives, or admit that it does not exist.
var declaredButUnenforcedPermissions = map[string]string{
	// --- Enforced by a different service -----------------------------------
	"ObservabilityTraceRead":    "enforced by agent-manager-observer (its own rbac package + RequirePermission)",
	"ObservabilityLogRead":      "enforced by agent-manager-observer",
	"ObservabilityMetricRead":   "enforced by agent-manager-observer",
	"ObservabilityBuildLogRead": "enforced by agent-manager-observer",

	// --- Enforced by a different mechanism ---------------------------------
	"MonitorScorePublish": "the publisher route uses PublisherClientAuthMiddleware (audience check) " +
		"rather than a scope check; the permission is granted to roles but never consulted",

	// --- NOT ENFORCED ANYWHERE ---------------------------------------------
	// These gate nothing today. Each one is a gap between the role matrix in
	// rbac/predefined_roles.go and what the API actually checks. Removing an
	// entry from this map (because the route now enforces it) is the fix; this
	// map exists to stop the list growing silently.
	"AgentEnvProduction": "NOT ENFORCED BY A ROUTE REGISTRAR: the tier a deploy/promote/config-update " +
		"lands in is only known once the request body or pipeline is resolved, so no route can " +
		"declare it statically. services.requireEnvTier checks it dynamically in the service layer " +
		"once the target environment's IsProduction flag is known.",
	"AgentRollback": "NOT ENFORCED: no rollback route consults it",
	"OrgAssignRole": "NOT ENFORCED: role assignment goes through " +
		"POST /identities/roles/{roleID}/assignees/add, which requires RoleUpdate instead",
	"OrgManageIDP":                  "NOT ENFORCED: no route consults it",
	"OrgModifySettings":             "NOT ENFORCED: no route consults it",
	"LLMProviderConfigureGuardrail": "NOT ENFORCED: guardrail config routes require LLMProviderUpdate instead",
	"LLMProviderConnect":            "NOT ENFORCED: no route consults it",
	"MCPServerConfigureGuardrail":   "NOT ENFORCED: no route consults it",
	"MCPServerAPIKeyManage":         "NOT ENFORCED: no route consults it",
}

// TestSecurityInvariantEveryPermissionIsEnforced fails when a declared
// rbac.Permission gates no route and is not accounted for above — and equally
// when an accounted-for permission starts gating a route, so the note gets
// deleted rather than left to mislead.
func TestSecurityInvariantEveryPermissionIsEnforced(t *testing.T) {
	declared := declaredPermissions(t)
	if len(declared) == 0 {
		t.Fatal("parsed zero permissions from rbac/permissions.go — the parser is broken, " +
			"not the code under test")
	}

	enforced := map[string]bool{}
	forEachRouteRegistration(t, func(route routeRegistration) {
		for _, permission := range route.permissions {
			enforced[permission] = true
		}
	})

	for _, perm := range declared {
		_, excused := declaredButUnenforcedPermissions[perm]
		switch {
		case enforced[perm] && excused:
			t.Errorf("rbac.%s is now referenced by a route but is still listed in "+
				"declaredButUnenforcedPermissions. Remove the entry.\nIt said: %s",
				perm, declaredButUnenforcedPermissions[perm])
		case !enforced[perm] && !excused:
			t.Errorf("rbac.%s is declared (and may be granted to predefined roles) but gates no "+
				"route in this package.\n\nEither wire it to the route it is meant to protect, or "+
				"add it to declaredButUnenforcedPermissions explaining where the boundary really is.",
				perm)
		}
	}

	for perm := range declaredButUnenforcedPermissions {
		if !contains(declared, perm) {
			t.Errorf("declaredButUnenforcedPermissions names %q, which is no longer a declared "+
				"rbac.Permission. Remove the entry.", perm)
		}
	}
}

// TestSecurityInvariantPermissionsReachRolesAndIDP verifies the other two
// links in the authorization chain. A route permission is unusable unless a
// predefined role grants it and Thunder advertises the corresponding scope.
func TestSecurityInvariantPermissionsReachRolesAndIDP(t *testing.T) {
	declared := declaredPermissionScopes(t)
	granted := predefinedRolePermissions(t)
	idpScopes := thunderAMPScopeCatalog(t)
	e2eScopes := e2eClientScopeCatalog(t)

	for permission, scope := range declared {
		if !granted[permission] {
			t.Errorf("rbac.%s is declared but no predefined role grants it", permission)
		}
		if !idpScopes[scope] {
			t.Errorf("rbac.%s produces scope %q, but Thunder bootstrap ampScopes does not advertise it",
				permission, scope)
		}
		if !e2eScopes[scope] {
			t.Errorf("rbac.%s produces scope %q, but the E2E client scope catalog does not request it",
				permission, scope)
		}
	}

	for scope := range idpScopes {
		if !containsMapValue(declared, scope) {
			t.Errorf("Thunder bootstrap ampScopes contains stale or unknown AMP scope %q", scope)
		}
	}
	for scope := range e2eScopes {
		if !containsMapValue(declared, scope) {
			t.Errorf("E2E client scope catalog contains stale or unknown scope %q", scope)
		}
	}
}

// predefinedRoleHandles maps the Go policy's role constants to the stable role
// handles created by the Thunder Helm bootstrap. Display names are deliberately
// not used as an implicit join key: RoleAdmin is "Agent Manager Admin" in Go,
// while the deployed Thunder role handle is "admin".
var predefinedRoleHandles = map[string]string{
	"RoleAdmin":            "admin",
	"RoleDeveloper":        "developer",
	"RoleAILead":           "ai-lead",
	"RolePlatformEngineer": "platform-engineer",
}

// TestSecurityInvariantPredefinedRolesMatchThunderBootstrap closes the policy
// source-of-truth gap between the service and the identity provider. The live
// role-persona suite proves Thunder issues and Agent Manager enforces the
// deployed role permissions; this invariant proves those deployed permissions
// are also exactly the ones reviewed in rbac/predefined_roles.go.
func TestSecurityInvariantPredefinedRolesMatchThunderBootstrap(t *testing.T) {
	serviceRoles := predefinedRoleScopesByHandle(t)
	thunderRoles := thunderBootstrapRoleScopes(t)

	for role, serviceScopes := range serviceRoles {
		thunderScopes, ok := thunderRoles[role]
		if !ok {
			t.Errorf("predefined service role %q has no matching Thunder bootstrap role", role)
			continue
		}
		for scope := range serviceScopes {
			if !thunderScopes[scope] {
				t.Errorf("Thunder bootstrap role %q is missing service permission %q", role, scope)
			}
		}
		for scope := range thunderScopes {
			if !serviceScopes[scope] {
				t.Errorf("Thunder bootstrap role %q grants extra permission %q not present in rbac.PredefinedRolePermissions", role, scope)
			}
		}
	}

	for role := range thunderRoles {
		if _, ok := serviceRoles[role]; !ok {
			t.Errorf("Thunder bootstrap defines predefined role %q with no matching service role", role)
		}
	}
}

// securityInvariantPrefix is how `make security-test-static` selects these
// tests: a single -run prefix match rather than a list of names.
//
// A list has to be edited in two places for every new invariant, and getting it
// wrong fails SILENTLY — `go test -run` that matches nothing prints "no tests to
// run" and exits 0, so the target reports success while checking nothing. A
// prefix means a correctly-named new invariant is picked up automatically.
const securityInvariantPrefix = "TestSecurityInvariant"

// TestSecurityInvariantNamingConventionHolds keeps that prefix scheme honest
// from both ends: every Test func in this file must carry the prefix, and the
// root Makefile must still select on it.
func TestSecurityInvariantNamingConventionHolds(t *testing.T) {
	src, err := os.ReadFile("route_authz_invariant_test.go")
	if err != nil {
		t.Fatalf("read own source: %v", err)
	}

	declared := regexp.MustCompile(`(?m)^func (Test\w+)\(`).FindAllStringSubmatch(string(src), -1)
	if len(declared) == 0 {
		t.Fatal("found no Test funcs in this file — the regex is broken, not the code under test")
	}

	for _, m := range declared {
		if !strings.HasPrefix(m[1], securityInvariantPrefix) {
			t.Errorf("%s does not start with %q, so `make security-test-static` will not run it.\n"+
				"Rename it — the target selects these invariants by prefix.", m[1], securityInvariantPrefix)
		}
	}

	const makefile = "../../Makefile"
	makefileSrc, err := os.ReadFile(makefile)
	if err != nil {
		t.Fatalf("read %s: %v", makefile, err)
	}
	if !strings.Contains(string(makefileSrc), securityInvariantPrefix) {
		t.Errorf("the root Makefile no longer mentions %q, so security-test-static is not "+
			"selecting these invariants. A -run regex matching nothing exits 0 and reports "+
			"success — check the SECURITY_STATIC_TESTS variable.", securityInvariantPrefix)
	}
}

// orgScopedPackages are the packages whose handlers must never take the org
// from the URL. Relative to this package's directory.
var orgScopedPackages = []string{".", filepath.Join("..", "controllers")}

// TestSecurityInvariantHandlersNeverReadOrgFromPath enforces the token-trust model: the org a
// request operates on comes from the token's ouId/ouHandle claims, never from
// the {orgName} path segment.
//
// middleware.RequireOrgMatch resolves the org from the token and injects it
// into the request context; {orgName} exists for routing and readability only.
// A handler that reads it instead would let any token act on the org named in
// the URL — the classic cross-tenant object-reference bug.
//
// This matters *now* precisely because AMP on-prem is single-org today. With
// only one org there is no way to observe the difference at runtime, so a
// handler that starts reading the path can sit there indefinitely without any
// test noticing. The day multi-org ships, it becomes a tenancy breach on
// arrival. This test is the thing standing between those two states, and it
// costs one AST walk.
//
// The invariant is currently clean: zero handlers read the path org, and all
// 20 controllers use OUIDFromRequest / OrgHandleFromRequest.
func TestSecurityInvariantHandlersNeverReadOrgFromPath(t *testing.T) {
	var violations []string
	pathValueCalls := 0

	for _, dir := range orgScopedPackages {
		forEachFile(t, dir, func(fset *token.FileSet, file *ast.File) {
			ast.Inspect(file, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "PathValue" || len(call.Args) != 1 {
					return true
				}
				pathValueCalls++

				arg := renderExpr(call.Args[0])
				if arg != "orgName" && !strings.HasSuffix(arg, "PathParamOrgName") {
					return true
				}
				violations = append(violations,
					fset.Position(call.Pos()).String()+": "+renderExpr(sel.X)+".PathValue("+arg+")")
				return true
			})
		})
	}

	// A broken walk would report zero violations and look like a pass. Handlers
	// legitimately read plenty of other path params (projName, agentName, envID),
	// so seeing none of those means the traversal found nothing at all.
	const minExpectedPathValueCalls = 50
	if pathValueCalls < minExpectedPathValueCalls {
		t.Fatalf("only found %d PathValue calls across %v, expected at least %d — the AST walk "+
			"is broken and this invariant is not actually checking anything",
			pathValueCalls, orgScopedPackages, minExpectedPathValueCalls)
	}

	if len(violations) > 0 {
		t.Errorf("handler(s) read the org from the URL path:\n  %s\n\n"+
			"Derive the org from the token instead: middleware.OUIDFromRequest(r) or "+
			"OrgHandleFromRequest(r). The {orgName} path segment is routing only — "+
			"middleware.RequireOrgMatch never reads it, and a token used on another org's path "+
			"operates on its OWN org. Reading the path here would reintroduce that boundary as a "+
			"trusted input. See the token-trust model comment in middleware/authorization.go.",
			strings.Join(violations, "\n  "))
	}
}

// routePackages is the reviewed list of packages that register public or
// internal HTTP endpoints. Add any new routing package here as part of its
// security-policy review.
var routePackages = []string{".", filepath.Join("..", "mcp")}

// forEachRouteRegistration walks every production Go file in routePackages and
// invokes fn for every net/http or RouteRegistrar registration call. Permission
// names are collected only from the authorization registrar's permission
// arguments; an unrelated rbac reference therefore cannot masquerade as route
// enforcement.
func forEachRouteRegistration(t *testing.T, fn func(routeRegistration)) {
	t.Helper()

	seen := 0
	for _, dir := range routePackages {
		forEachFile(t, dir, func(_ *token.FileSet, file *ast.File) {
			for _, decl := range file.Decls {
				funcDecl, ok := decl.(*ast.FuncDecl)
				if !ok || funcDecl.Body == nil {
					continue
				}
				ast.Inspect(funcDecl.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if !ok {
						return true
					}
					sel, ok := call.Fun.(*ast.SelectorExpr)
					if !ok || !isRouteRegistrar(sel.Sel.Name) {
						return true
					}

					route := routeRegistration{
						function:  funcDecl.Name.Name,
						registrar: sel.Sel.Name,
					}
					if len(call.Args) > 0 {
						route.pattern = renderExpr(call.Args[0])
					}
					route.permissions = registrarPermissions(call)
					seen++
					fn(route)
					return true
				})
			}
		})
	}

	// Guard against a silently broken walk: if the AST traversal stops finding
	// registrations, both invariants would pass while checking nothing.
	const minExpectedRoutes = 200
	if seen < minExpectedRoutes {
		t.Fatalf("only found %d route registrations, expected at least %d — the AST walk is "+
			"broken and these invariants are not actually checking anything", seen, minExpectedRoutes)
	}
}

func isRouteRegistrar(name string) bool {
	return name == "Handle" || name == "HandleFunc" || strings.HasPrefix(name, "HandleFuncWith")
}

func registrarPermissions(call *ast.CallExpr) []string {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil
	}

	var args []ast.Expr
	switch sel.Sel.Name {
	case "HandleFuncWithValidationAndAuthz", "HandleFuncWithValidationAndAuthzAllowRootOU":
		if len(call.Args) > 1 {
			args = call.Args[1:2]
		}
	case "HandleFuncWithValidationAndAnyAuthz", "HandleFuncWithValidationAndAllAuthz":
		if len(call.Args) > 2 {
			args = call.Args[2:]
		}
	}

	permissions := make([]string, 0, len(args))
	for _, arg := range args {
		sel, ok := arg.(*ast.SelectorExpr)
		if !ok {
			continue
		}
		pkg, ok := sel.X.(*ast.Ident)
		if ok && pkg.Name == "rbac" {
			permissions = append(permissions, sel.Sel.Name)
		}
	}
	return permissions
}

// declaredPermissions parses the rbac package for constants of type Permission.
func declaredPermissions(t *testing.T) []string {
	t.Helper()

	scopes := declaredPermissionScopes(t)
	names := make([]string, 0, len(scopes))
	for name := range scopes {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func declaredPermissionScopes(t *testing.T) map[string]string {
	t.Helper()

	scopes := map[string]string{}
	forEachFile(t, filepath.Join("..", "rbac"), func(_ *token.FileSet, file *ast.File) {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.CONST {
				continue
			}
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				typeIdent, ok := valueSpec.Type.(*ast.Ident)
				if !ok || typeIdent.Name != "Permission" {
					continue
				}
				for i, name := range valueSpec.Names {
					if i >= len(valueSpec.Values) {
						t.Fatalf("rbac permission %s has no explicit value", name.Name)
					}
					literal, ok := valueSpec.Values[i].(*ast.BasicLit)
					if !ok || literal.Kind != token.STRING {
						t.Fatalf("rbac permission %s is not a string literal", name.Name)
					}
					value, err := strconv.Unquote(literal.Value)
					if err != nil {
						t.Fatalf("decode rbac permission %s: %v", name.Name, err)
					}
					scopes[name.Name] = "amp:" + value
				}
			}
		}
	})

	return scopes
}

func predefinedRolePermissions(t *testing.T) map[string]bool {
	t.Helper()

	granted := map[string]bool{}
	declared := declaredPermissionScopes(t)
	forEachFile(t, filepath.Join("..", "rbac"), func(_ *token.FileSet, file *ast.File) {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.VAR {
				continue
			}
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok || len(valueSpec.Names) != 1 || valueSpec.Names[0].Name != "PredefinedRolePermissions" {
					continue
				}
				for _, value := range valueSpec.Values {
					ast.Inspect(value, func(n ast.Node) bool {
						ident, ok := n.(*ast.Ident)
						if ok {
							if _, isPermission := declared[ident.Name]; isPermission {
								granted[ident.Name] = true
							}
						}
						return true
					})
				}
			}
		}
	})
	return granted
}

func predefinedRoleScopesByHandle(t *testing.T) map[string]map[string]bool {
	t.Helper()

	declared := declaredPermissionScopes(t)
	roles := map[string]map[string]bool{}
	foundPolicy := false
	forEachFile(t, filepath.Join("..", "rbac"), func(_ *token.FileSet, file *ast.File) {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok || genDecl.Tok != token.VAR {
				continue
			}
			for _, spec := range genDecl.Specs {
				valueSpec, ok := spec.(*ast.ValueSpec)
				if !ok || len(valueSpec.Names) != 1 || valueSpec.Names[0].Name != "PredefinedRolePermissions" {
					continue
				}
				foundPolicy = true
				if len(valueSpec.Values) != 1 {
					t.Fatal("rbac.PredefinedRolePermissions must have exactly one map literal value")
				}
				policy, ok := valueSpec.Values[0].(*ast.CompositeLit)
				if !ok {
					t.Fatal("rbac.PredefinedRolePermissions is not a composite map literal")
				}
				for _, element := range policy.Elts {
					entry, ok := element.(*ast.KeyValueExpr)
					if !ok {
						t.Fatal("rbac.PredefinedRolePermissions contains a non-keyed entry")
					}
					roleIdent, ok := entry.Key.(*ast.Ident)
					if !ok {
						t.Fatalf("predefined role key %s is not a role constant", renderExpr(entry.Key))
					}
					handle, ok := predefinedRoleHandles[roleIdent.Name]
					if !ok {
						t.Fatalf("predefined role constant %s has no reviewed Thunder handle mapping", roleIdent.Name)
					}
					permissionList, ok := entry.Value.(*ast.CompositeLit)
					if !ok {
						t.Fatalf("permissions for %s are not a composite literal", roleIdent.Name)
					}
					scopes := map[string]bool{}
					for _, permissionExpr := range permissionList.Elts {
						permission, ok := permissionExpr.(*ast.Ident)
						if !ok {
							t.Fatalf("%s contains non-identifier permission %s", roleIdent.Name, renderExpr(permissionExpr))
						}
						scope, ok := declared[permission.Name]
						if !ok {
							t.Fatalf("%s references unknown permission %s", roleIdent.Name, permission.Name)
						}
						if scopes[scope] {
							t.Errorf("%s grants duplicate permission %q", roleIdent.Name, scope)
						}
						scopes[scope] = true
					}
					roles[handle] = scopes
				}
			}
		}
	})

	if !foundPolicy || len(roles) == 0 {
		t.Fatal("parsed zero predefined roles from rbac/predefined_roles.go")
	}
	for roleConstant, handle := range predefinedRoleHandles {
		if _, ok := roles[handle]; !ok {
			t.Errorf("reviewed role mapping %s -> %s has no entry in rbac.PredefinedRolePermissions", roleConstant, handle)
		}
	}
	return roles
}

func thunderBootstrapRoleScopes(t *testing.T) map[string]map[string]bool {
	t.Helper()

	const bootstrapPath = "../../deployments/helm-charts/wso2-amp-thunder-extension/templates/amp-thunder-bootstrap.yaml"
	source, err := os.ReadFile(bootstrapPath)
	if err != nil {
		t.Fatalf("read Thunder bootstrap template: %v", err)
	}

	// Each predefined role is a declarative bootstrap resource. Parse complete
	// role file blocks rather than arbitrary scope lines so a formatting or
	// resource-identity change fails loudly instead of producing a partial policy.
	fileBlockPattern := regexp.MustCompile(`(?m)^  \S+\.yaml: \|\n`)
	roleHeaderPattern := regexp.MustCompile(`^  \d+-amp-role-([a-z-]+)\.yaml: \|\n$`)
	scopePattern := regexp.MustCompile(`(?m)^\s+- "(amp:[^"]+)"\s*$`)
	fileBlocks := fileBlockPattern.FindAllStringIndex(string(source), -1)
	roles := map[string]map[string]bool{}
	for index, bounds := range fileBlocks {
		header := string(source[bounds[0]:bounds[1]])
		match := roleHeaderPattern.FindStringSubmatch(header)
		if match == nil {
			continue
		}
		handle := match[1]
		if !containsMapValue(predefinedRoleHandles, handle) {
			continue
		}
		if _, duplicate := roles[handle]; duplicate {
			t.Fatalf("Thunder bootstrap configures predefined role %q more than once", handle)
		}
		blockEnd := len(source)
		if index+1 < len(fileBlocks) {
			blockEnd = fileBlocks[index+1][0]
		}
		block := string(source[bounds[1]:blockEnd])
		if !strings.Contains(block, "    resource_type: role\n") ||
			!strings.Contains(block, "    id: amp-role-"+handle+"\n") ||
			!strings.Contains(block, "      - resourceServerId: amp-resource-server\n") {
			t.Fatalf("Thunder bootstrap role %q no longer has the reviewed declarative role identity", handle)
		}
		// MCP resource servers intentionally repeat a role's applicable scope
		// strings under different resourceServerId values. This invariant compares
		// the service policy with the canonical amp-resource-server block only.
		if mcpBlocks := strings.Index(block, "    {{- range $server := .Values.thunder.bootstrap.mcpResourceServers }}"); mcpBlocks >= 0 {
			block = block[:mcpBlocks]
		}
		roles[handle] = map[string]bool{}
		for _, scopeMatch := range scopePattern.FindAllStringSubmatch(block, -1) {
			scope := scopeMatch[1]
			if roles[handle][scope] {
				t.Errorf("Thunder bootstrap role %q grants duplicate permission %q", handle, scope)
			}
			roles[handle][scope] = true
		}
		if len(roles[handle]) == 0 {
			t.Fatalf("parsed zero AMP permissions for Thunder bootstrap role %q", handle)
		}
	}

	if len(roles) != len(predefinedRoleHandles) {
		t.Fatalf("parsed %d predefined roles from Thunder bootstrap, expected %d; "+
			"found %v — the template format or role inventory changed",
			len(roles), len(predefinedRoleHandles), sortedMapKeys(roles))
	}
	return roles
}

func thunderAMPScopeCatalog(t *testing.T) map[string]bool {
	t.Helper()

	const valuesPath = "../../deployments/helm-charts/wso2-amp-thunder-extension/values.yaml"
	contents, err := os.ReadFile(valuesPath)
	if err != nil {
		t.Fatalf("read Thunder values: %v", err)
	}

	scopes := map[string]bool{}
	inCatalog := false
	scanner := bufio.NewScanner(strings.NewReader(string(contents)))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "    ampScopes:" {
			inCatalog = true
			continue
		}
		if !inCatalog {
			continue
		}
		if strings.HasPrefix(line, "    ") && !strings.HasPrefix(line, "      ") && strings.TrimSpace(line) != "" {
			break
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, `- "amp:`) || !strings.HasSuffix(trimmed, `"`) {
			continue
		}
		scopes[strings.Trim(trimmed[2:], `"`)] = true
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan Thunder values: %v", err)
	}
	if len(scopes) == 0 {
		t.Fatal("parsed zero AMP scopes from Thunder bootstrap ampScopes")
	}
	return scopes
}

func e2eClientScopeCatalog(t *testing.T) map[string]bool {
	t.Helper()

	const authPath = "../../test/e2e/framework/auth.go"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, authPath, nil, 0)
	if err != nil {
		t.Fatalf("parse E2E auth scope catalog: %v", err)
	}

	scopes := map[string]bool{}
	for _, decl := range file.Decls {
		genDecl, ok := decl.(*ast.GenDecl)
		if !ok || genDecl.Tok != token.CONST {
			continue
		}
		for _, spec := range genDecl.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok || len(valueSpec.Names) != 1 || valueSpec.Names[0].Name != "ampScopes" || len(valueSpec.Values) != 1 {
				continue
			}
			for _, scope := range strings.Fields(renderExpr(valueSpec.Values[0])) {
				if strings.HasPrefix(scope, "amp:") {
					scopes[scope] = true
				}
			}
		}
	}
	if len(scopes) == 0 {
		t.Fatal("parsed zero scopes from test/e2e/framework/auth.go ampScopes")
	}
	return scopes
}

// forEachFile parses every non-test .go file in dir and invokes fn on each.
func forEachFile(t *testing.T, dir string, fn func(*token.FileSet, *ast.File)) {
	t.Helper()

	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read %s: %v", dir, err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		fn(fset, file)
	}
}

// renderExpr produces a readable form of a route-pattern argument. Patterns are
// usually string literals but some are built by a route(method, path) helper or
// by concatenation, so this is best-effort and used only in failure messages.
func renderExpr(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return strings.Trim(e.Value, `"`)
	case *ast.BinaryExpr:
		return renderExpr(e.X) + renderExpr(e.Y)
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return renderExpr(e.X) + "." + e.Sel.Name
	case *ast.CallExpr:
		parts := make([]string, 0, len(e.Args))
		for _, arg := range e.Args {
			parts = append(parts, renderExpr(arg))
		}
		return strings.Join(parts, " ")
	default:
		return "<expr>"
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}

func containsMapValue(haystack map[string]string, needle string) bool {
	for _, value := range haystack {
		if value == needle {
			return true
		}
	}
	return false
}

func sortedMapKeys(values map[string]map[string]bool) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
