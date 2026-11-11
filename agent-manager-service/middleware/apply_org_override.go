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
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// OUHandleLookup maps an organization unit ID to its handle. It returns an
// error wrapping ErrOverrideOrgNotFound when no such organization exists.
type OUHandleLookup func(ctx context.Context, ouID string) (handle string, err error)

// ErrOverrideOrgNotFound is returned (wrapped) by an OUHandleLookup when the
// organization does not exist.
var ErrOverrideOrgNotFound = errors.New("organization not found")

// ApplyOrgOverride lets a trusted caller act on another organization than its
// token's, on the routes it is composed into. It runs after RequireOrgMatch,
// which has put the token's org on the context, and before the permission
// check.
//
// The header decides whether an override is being attempted, and the
// deployment's OrgOverrideResolver decides whether it is allowed:
//
//   - no X-Impersonate-Org header: the request passes through unchanged, and
//     the resolver is not consulted;
//   - header present but no resolver installed, or the resolver refuses (or
//     names no org): 403, recorded as an authz deny. An attempt to impersonate
//     is never silently downgraded to the caller's own org;
//   - the {orgName} path segment must be the target org's handle: an
//     impersonating request names the tenant in its path as a user of that
//     tenant would, and one naming any other org is refused with 403, so the
//     path never says one org while the request acts on another;
//   - resolver allows it: the whole ResolvedOrg — OUID and handle — is
//     rewritten to the target, the handle being looked up because the header
//     carries the UUID only. Handlers and the platform repository then
//     act on the target org. The switch is logged and audited with the
//     caller's client id.
//
// A target the lookup reports as missing is refused with 403 (the caller named
// an org it cannot act on); any other lookup failure is 502.
//
// lookup must not be nil: a route that composes this without one is a wiring
// bug, caught at startup.
func ApplyOrgOverride(lookup OUHandleLookup) func(http.HandlerFunc) http.HandlerFunc {
	if lookup == nil {
		panic("middleware: ApplyOrgOverride requires an OU handle lookup")
	}
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			if strings.TrimSpace(r.Header.Get(HeaderImpersonateOrg)) == "" {
				next(w, r)
				return
			}

			ctx := r.Context()
			tokenOrg, ok := GetResolvedOrg(ctx)
			if !ok {
				// RequireOrgMatch did not run: a route-table bug. Fail closed.
				slog.ErrorContext(ctx, "org override refused: resolved org missing — RequireOrgMatch not applied",
					"path", sanitizeForLog(r.URL.Path))
				RecordAuthzDeny(r, "org-override-not-permitted")
				utils.WriteErrorResponse(w, http.StatusForbidden, "org impersonation not permitted")
				return
			}

			clientID := tokenClientID(ctx)
			targetOUID, allowed := OrgOverride(r)
			targetOUID = strings.TrimSpace(targetOUID)
			if !allowed || targetOUID == "" {
				slog.WarnContext(ctx, "org override refused",
					"clientId", clientID, "tokenOuID", tokenOrg.OUID, "path", sanitizeForLog(r.URL.Path))
				RecordAuthzDeny(r, "org-override-not-permitted")
				utils.WriteErrorResponse(w, http.StatusForbidden, "org impersonation not permitted")
				return
			}
			if targetOUID == tokenOrg.OUID {
				if !pathNamesOrg(w, r, tokenOrg.OuHandle) {
					return
				}
				next(w, r)
				return
			}

			handle, err := lookup(ctx, targetOUID)
			switch {
			case errors.Is(err, ErrOverrideOrgNotFound):
				slog.WarnContext(ctx, "org override refused: target org not found",
					"clientId", clientID, "tokenOuID", tokenOrg.OUID, "targetOuID", targetOUID)
				RecordAuthzDeny(r, "org-override-target-not-found")
				utils.WriteErrorResponse(w, http.StatusForbidden, "org impersonation not permitted")
				return
			case err != nil || strings.TrimSpace(handle) == "":
				slog.ErrorContext(ctx, "org override failed: could not resolve target org handle",
					"clientId", clientID, "tokenOuID", tokenOrg.OUID, "targetOuID", targetOUID, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadGateway, "failed to resolve the impersonated organization")
				return
			}

			if !pathNamesOrg(w, r, handle) {
				return
			}

			ctx = WithResolvedOrg(ctx, ResolvedOrg{OUID: targetOUID, OuHandle: handle})
			slog.Info("org override applied",
				"clientId", clientID, "tokenOuID", tokenOrg.OUID, "targetOuID", targetOUID)
			audit.RecordAncillary(
				ctx, audit.ActionAuthzOrgOverride,
				audit.Detail("clientId", clientID),
				audit.Detail("tokenOuID", tokenOrg.OUID),
				audit.Detail("targetOuID", targetOUID),
			)
			next(w, r.WithContext(ctx))
		}
	}
}

// pathNamesOrg refuses (403) an impersonating request whose {orgName} path segment is not handle, the handle of
// the org it acts on. A route without the segment is refused too: it cannot show which org it acts on.
func pathNamesOrg(w http.ResponseWriter, r *http.Request, handle string) bool {
	pathOrg := r.PathValue("orgName")
	if pathOrg == handle {
		return true
	}
	slog.WarnContext(r.Context(), "org override refused: path org is not the impersonated org",
		"clientId", tokenClientID(r.Context()), "pathOrg", sanitizeForLog(pathOrg), "targetHandle", handle)
	RecordAuthzDeny(r, "org-override-path-mismatch")
	utils.WriteErrorResponse(w, http.StatusForbidden, "the organization in the path is not the impersonated organization")
	return false
}

// tokenClientID returns the OAuth client id the request's token was issued
// to, or "" when the token carries none.
func tokenClientID(ctx context.Context) string {
	if claims := jwtassertion.GetTokenClaims(ctx); claims != nil {
		return claims.ClientID
	}
	return ""
}
