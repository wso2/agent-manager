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

package middleware

import (
	"log/slog"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// recordAuthzDeny records a refused authorization decision.
//
// Denied privilege escalation is the first thing a security reviewer looks for,
// and until now every deny site here wrote a 403 and nothing else — two of them
// without even a log line. The event carries the specific missing scope rather
// than the caller's whole scope string, which can run to kilobytes.
//
// It suppresses the coverage tier's envelope event so a single refusal produces
// one record rather than two.
func recordAuthzDeny(r *http.Request, reason string, opts ...audit.Option) {
	ctx := r.Context()

	all := make([]audit.Option, 0, len(opts)+3)
	all = append(
		all,
		audit.OutcomeOpt(audit.OutcomeDeny),
		audit.Status(http.StatusForbidden),
		audit.Detail("reason", reason),
	)
	all = append(all, opts...)

	audit.Record(ctx, audit.ActionAuthzDeny, all...)
	audit.Skip(ctx)
}

// RequireOrgMatch returns a middleware that:
//  1. Validates the token carries ouId and ouHandle (required for both cloud and on-prem).
//  2. Injects ResolvedOrg (from the token) into the request context for handlers.
//
// Token-trust model: the token is the single source of truth for org identity.
// The {orgName} URL path segment is routing only and is NOT read here or used
// for tenant selection — handlers derive the org from the context
// (OUIDFromRequest / OrgHandleFromRequest), never from the path. A token used on
// another org's path therefore operates on its OWN org, not the path's.
//
// The resolver argument is kept for route-registrar signature compatibility but
// is no longer consulted (path-based org resolution has been removed).
func RequireOrgMatch(_ OrgResolver) func(http.HandlerFunc) http.HandlerFunc {
	return resolveOrgFromToken()
}

// RequireOrgMatchAllowRootOU is retained for the route registrar's system-client
// variant. Path-based (root-OU cross-org) resolution has been removed, so it now
// behaves identically to RequireOrgMatch: the org is always taken from the token.
func RequireOrgMatchAllowRootOU(_ OrgResolver) func(http.HandlerFunc) http.HandlerFunc {
	return resolveOrgFromToken()
}

// resolveOrgFromToken validates the token's org identity and injects ResolvedOrg
// (taken from the token) into the request context. It never reads the {orgName}
// path segment.
func resolveOrgFromToken() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			claims := jwtassertion.GetTokenClaims(r.Context())
			if claims == nil {
				slog.Warn("RequireOrgMatch rejected", "reason", "missing token claims", "path", r.URL.Path)
				recordAuthzDeny(r, "missing-token-claims")
				utils.WriteErrorResponse(w, http.StatusForbidden, "missing token claims")
				return
			}
			if claims.OuId == "" || claims.OuHandle == "" {
				slog.Warn("RequireOrgMatch rejected", "reason", "missing ou identity in token", "sub", claims.Sub, "path", r.URL.Path)
				recordAuthzDeny(r, "missing-ou-identity")
				utils.WriteErrorResponse(w, http.StatusForbidden, "missing ou identity in token")
				return
			}

			// Trust the token as the sole source of org identity.
			ctx := WithResolvedOrg(r.Context(), ResolvedOrg{
				OuHandle: claims.OuHandle,
				OUID:     claims.OuId,
			})
			next(w, r.WithContext(ctx))
		}
	}
}

// RequirePermission returns a middleware that checks the request token carries the
// required amp: scope. When RBAC_ENABLED=false the check is skipped entirely,
// allowing zero-downtime rollout.
func RequirePermission(perm rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return requireScopes(allPermissions, false, perm)
}

// RequirePermissionAllowRootOU behaves like RequirePermission but additionally
// allows a client-credentials token issued to the configured root/admin OU
// (config.RootOUHandle) through regardless of scope. Use only for system-client
// endpoints (currently: gateway registration during org bootstrap) — do not
// apply broadly to user-facing routes.
func RequirePermissionAllowRootOU(perm rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return requireScopes(allPermissions, true, perm)
}

// RequireAnyPermission returns a middleware that passes if the token carries at least
// one of the given permissions (OR semantics). Use this for endpoints that are
// legitimately reachable via multiple roles (e.g. environments read needed by both
// the environment manager and the LLM-provider viewer).
func RequireAnyPermission(perms ...rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return requireScopes(anyPermission, false, perms...)
}

// RequireAllPermissions returns a middleware that passes only if the token carries
// every one of the given permissions (AND semantics). Use it where an operation is
// gated on more than one independent axis — the deployment-state route needs both
// the capability (agent:suspend) and the environment tier
// (agent:env-non-production) — so holding one axis does not admit the other.
func RequireAllPermissions(perms ...rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return requireScopes(allPermissions, false, perms...)
}

// scopeMode says how a route's permissions combine.
type scopeMode int

const (
	// allPermissions admits only a caller holding every one of them.
	allPermissions scopeMode = iota
	// anyPermission admits a caller holding at least one.
	anyPermission
)

// requireScopes is the one gate behind RequirePermission, RequireAnyPermission
// and RequireAllPermissions. They differ only in how the permissions combine and
// in what the refusal names; everything around that — the RBAC_ENABLED
// short-circuit, the root-OU bypass, the audited denial, the 403 — is the same
// gate, and was three copies of it before. A fourth combining rule should be a
// scopeMode, not a fourth copy.
func requireScopes(mode scopeMode, allowRootOU bool, perms ...rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	// An empty list is refused where the route table is built, not where a
	// request arrives, because in allPermissions mode there would be no request
	// to refuse: nothing is missing from a list of nothing, so FirstMissingScope
	// reports the caller short of nothing and the gate opens for everyone. A
	// route gated on no permission is a bug in the route table either way, and
	// the route table is assembled at startup — the only place a bug in it can
	// still be cheap to find.
	if len(perms) == 0 {
		panic("middleware: a route must be gated on at least one permission")
	}
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !config.GetConfig().RBACEnabled {
				// The check is skipped entirely. The event the coverage tier
				// writes for this request carries rbacEnforced=false alongside
				// the permission that would have applied, so the record shows
				// on its face that nothing was enforced.
				next(w, r)
				return
			}
			if allowRootOU {
				if claims := jwtassertion.GetTokenClaims(r.Context()); claims != nil && claims.OuHandle == config.GetConfig().RootOUHandle {
					// A root-OU token is admitted regardless of its scopes. That
					// bypass is a first-class audited fact, not an implementation
					// detail — it is the one path where a caller reaches a
					// protected route without holding its permission.
					//
					// Recorded without suppressing the envelope: this event says
					// how the caller got in, the envelope says what they then did.
					audit.RecordAncillary(
						r.Context(), audit.ActionAuthzRootOUBypass,
						audit.RequiredPermissions(perms...),
						audit.Detail("rootOUBypass", true),
					)
					next(w, r)
					return
				}
			}

			reason := "missing-scope"
			opts := []audit.Option{
				audit.RequiredPermissions(perms...),
				audit.Detail("grantedScopes", jwtassertion.GrantedScopeCount(r.Context())),
			}
			if mode == anyPermission {
				if jwtassertion.HoldsAnyScope(r.Context(), perms...) {
					next(w, r)
					return
				}
				// The caller held none of the acceptable permissions, so record all
				// of them rather than singling one out as "the" missing scope.
				reason = "missing-any-scope"
			} else {
				missing, short := jwtassertion.FirstMissingScope(r.Context(), perms...)
				if !short {
					next(w, r)
					return
				}
				// The record names the first missing permission as the missing scope
				// because that is the one that decided the outcome, and lists all of
				// them as required so the event says what the route actually demands.
				opts = append(opts, audit.Detail("missingScope", missing.Scope()))
			}
			recordAuthzDeny(r, reason, opts...)
			utils.WriteErrorResponse(w, http.StatusForbidden, "insufficient permissions")
		}
	}
}
