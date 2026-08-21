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
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
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

// grantedScopeCount reports how many scopes the caller presented. The count is
// recorded instead of the scopes themselves: it distinguishes "token with no
// scopes at all" from "token missing this one scope" without copying a
// potentially huge claim into every record.
func grantedScopeCount(claims *jwtassertion.TokenClaims) int {
	if claims == nil {
		return 0
	}
	return len(strings.Fields(claims.Scope))
}

// ResolverError is returned by a PermissionResolver to signal an expected failure
// with a specific HTTP status code and message. Use NewResolverInputError for bad
// request data (400) and NewResolverForbiddenError for explicit deny (403).
// Any other error type from a resolver is treated as an internal failure (500).
type ResolverError struct {
	StatusCode int
	Message    string
}

func (e *ResolverError) Error() string { return e.Message }

// NewResolverInputError returns a ResolverError that maps to 400 Bad Request.
func NewResolverInputError(msg string) *ResolverError {
	return &ResolverError{StatusCode: http.StatusBadRequest, Message: msg}
}

// NewResolverForbiddenError returns a ResolverError that maps to 403 Forbidden.
func NewResolverForbiddenError(msg string) *ResolverError {
	return &ResolverError{StatusCode: http.StatusForbidden, Message: msg}
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
			log := logger.GetLogger(r.Context())
			claims := jwtassertion.GetTokenClaims(r.Context())
			if claims == nil {
				log.Warn("RequireOrgMatch rejected", "reason", "missing token claims")
				recordAuthzDeny(r, "missing-token-claims")
				utils.WriteErrorResponse(w, http.StatusForbidden, "missing token claims")
				return
			}
			if claims.OuId == "" || claims.OuHandle == "" {
				// `sub` predates this logger and stays: it is the only thing that
				// distinguishes one malformed-token caller from another on a
				// rejection path that carries no org. It is deliberately not
				// promoted onto the context logger — see the Enrich call below.
				log.Warn("RequireOrgMatch rejected", "reason", "missing ou identity in token",
					"sub", utils.SanitizeForLog(claims.Sub))
				recordAuthzDeny(r, "missing-ou-identity")
				utils.WriteErrorResponse(w, http.StatusForbidden, "missing ou identity in token")
				return
			}

			// Trust the token as the sole source of org identity.
			ctx := WithResolvedOrg(r.Context(), ResolvedOrg{
				OuHandle: claims.OuHandle,
				OUID:     claims.OuId,
			})
			// Attach the org once, here, where it first becomes known — every log
			// line the handler and the layers below it write inherits it.
			//
			// The token subject is deliberately not attached: the caller's
			// identity belongs in the audit trail, which records it with the
			// retention and access controls that come with being evidence, not
			// in the application log.
			ctx = logger.Enrich(ctx,
				slog.String("ou_id", claims.OuId),
				slog.String("org_handle", utils.SanitizeForLog(claims.OuHandle)),
			)
			// The access-log record is emitted outside this middleware, where a
			// context value added here is no longer visible.
			setRequestIdentity(ctx, claims.OuId)
			next(w, r.WithContext(ctx))
		}
	}
}

// RequirePermission returns a middleware that checks the request token carries the
// required amp: scope. When RBAC_ENABLED=false the check is skipped entirely,
// allowing zero-downtime rollout.
func RequirePermission(perm rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return requirePermission(perm, false)
}

// RequirePermissionAllowRootOU behaves like RequirePermission but additionally
// allows a client-credentials token issued to the configured root/admin OU
// (config.RootOUHandle) through regardless of scope. Use only for system-client
// endpoints (currently: gateway registration during org bootstrap) — do not
// apply broadly to user-facing routes.
func RequirePermissionAllowRootOU(perm rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return requirePermission(perm, true)
}

func requirePermission(perm rbac.Permission, allowRootOU bool) func(http.HandlerFunc) http.HandlerFunc {
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
						audit.RequiredPermissions(perm),
						audit.Detail("rootOUBypass", true),
					)
					next(w, r)
					return
				}
			}
			if !jwtassertion.HasAllScopes(r.Context(), []string{perm.Scope()}) {
				recordAuthzDeny(
					r, "missing-scope",
					audit.RequiredPermissions(perm),
					audit.Detail("missingScope", perm.Scope()),
					audit.Detail("grantedScopes", grantedScopeCount(jwtassertion.GetTokenClaims(r.Context()))),
				)
				utils.WriteErrorResponse(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next(w, r)
		}
	}
}

// RequireAnyPermission returns a middleware that passes if the token carries at least
// one of the given permissions (OR semantics). Use this for endpoints that are
// legitimately reachable via multiple roles (e.g. environments read needed by both
// the environment manager and the LLM-provider viewer).
func RequireAnyPermission(perms ...rbac.Permission) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !config.GetConfig().RBACEnabled {
				next(w, r)
				return
			}
			for _, perm := range perms {
				if jwtassertion.HasAllScopes(r.Context(), []string{perm.Scope()}) {
					next(w, r)
					return
				}
			}
			// The caller held none of the acceptable permissions, so record all
			// of them rather than singling one out as "the" missing scope.
			recordAuthzDeny(
				r, "missing-any-scope",
				audit.RequiredPermissions(perms...),
				audit.Detail("grantedScopes", grantedScopeCount(jwtassertion.GetTokenClaims(r.Context()))),
			)
			utils.WriteErrorResponse(w, http.StatusForbidden, "insufficient permissions")
		}
	}
}

// PermissionResolver resolves the required permission at request time.
// Return *ResolverError to signal expected failures with a specific status code
// (use NewResolverInputError for 400, NewResolverForbiddenError for 403).
// Any other error is treated as an internal failure and results in a 500 response.
type PermissionResolver func(r *http.Request) (rbac.Permission, error)

// RequireDynamicPermission returns a middleware that resolves the required permission
// at request time via resolver, then checks the token scope. Use this for endpoints
// where the required permission depends on request data (e.g. deploy target environment).
func RequireDynamicPermission(resolver PermissionResolver) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if !config.GetConfig().RBACEnabled {
				next(w, r)
				return
			}
			perm, err := resolver(r)
			if err != nil {
				var re *ResolverError
				if errors.As(err, &re) {
					if re.StatusCode == http.StatusForbidden {
						recordAuthzDeny(r, "resolver-denied")
					}
					utils.WriteErrorResponse(w, re.StatusCode, re.Message)
				} else {
					utils.WriteErrorResponse(w, http.StatusInternalServerError, "internal error resolving permission")
				}
				return
			}
			if !jwtassertion.HasAllScopes(r.Context(), []string{perm.Scope()}) {
				// The route declares no permission at registration time, so this
				// is the only place the required scope becomes known — without
				// recording it here the event would say nothing about what was
				// demanded.
				recordAuthzDeny(
					r, "missing-scope",
					audit.RequiredPermissions(perm),
					audit.Detail("missingScope", perm.Scope()),
					audit.Detail("grantedScopes", grantedScopeCount(jwtassertion.GetTokenClaims(r.Context()))),
				)
				utils.WriteErrorResponse(w, http.StatusForbidden, "insufficient permissions")
				return
			}
			next(w, r)
		}
	}
}
