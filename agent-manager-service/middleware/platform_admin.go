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
	"log/slog"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type platformAdminCtxKey int

// platformAdminKey marks a request that RequirePlatformAdminOU has admitted.
const platformAdminKey platformAdminCtxKey = iota

// RequirePlatformAdminOU admits only a token issued to the single organization
// named by config.PlatformAdminOUID, and marks the request context so the
// handler can confirm the check actually ran.
//
// This gate protects cross-organization routes, which read every tenant's rows
// with no ou_id predicate. Three properties are load-bearing, and each of them
// is here because the obvious alternative fails open:
//
//  1. It does not depend on the caller's scopes. The permission a cross-org
//     route declares (gateway:read) is an ordinary org-level grant that every
//     tenant's admin holds, so a route gated on a scope alone would admit any
//     tenant. This check is unconditional and keyed on the organization.
//
//  2. An empty PlatformAdminOUID denies everyone rather than admitting
//     everyone. A deployment with no platform-admin org configured has no
//     caller entitled to cross-tenant data.
//
//  3. It compares claims.OuId, never claims.OuHandle. A handle can be renamed
//     and re-claimed; the OU ID cannot.
//
// The permission a cross-org route declares at registration is a label for the
// audit trail and the route-table invariants, not a second enforcement point —
// see point 1. This gate is the enforcement.
func RequirePlatformAdminOU() func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			// Every denial below carries the correlation id. These are the only
			// record of a refused cross-organization read outside the audit
			// trail, and without it a 403 the caller reports cannot be tied to
			// the request that produced it.
			correlationID := utils.GetCorrelationId(r.Context())

			adminOUID := config.GetConfig().PlatformAdminOUID
			if adminOUID == "" {
				slog.Warn("RequirePlatformAdminOU rejected",
					"reason", "PLATFORM_ADMIN_OU_ID not configured",
					"correlationId", correlationID, "path", r.URL.Path)
				recordAuthzDeny(r, "platform-admin-ou-not-configured")
				utils.WriteErrorResponse(w, http.StatusForbidden, "forbidden")
				return
			}

			claims := jwtassertion.GetTokenClaims(r.Context())
			if claims == nil || claims.OuId == "" {
				// No ouId to log here — that absence is the reason for the denial.
				slog.Warn("RequirePlatformAdminOU rejected",
					"reason", "missing ou identity in token",
					"correlationId", correlationID, "path", r.URL.Path)
				recordAuthzDeny(r, "missing-ou-identity")
				utils.WriteErrorResponse(w, http.StatusForbidden, "forbidden")
				return
			}

			if claims.OuId != adminOUID {
				// The caller's own OU is recorded, not the configured one: the
				// question this record answers later is who tried, and naming
				// the expected value in the trail would spread it further than
				// the config it came from.
				slog.Warn("RequirePlatformAdminOU rejected",
					"reason", "caller is not the platform admin organization",
					"correlationId", correlationID,
					"sub", claims.Sub, "ouId", claims.OuId, "path", r.URL.Path)
				recordAuthzDeny(r, "not-platform-admin-ou", audit.Detail("callerOuId", claims.OuId))
				utils.WriteErrorResponse(w, http.StatusForbidden, "forbidden")
				return
			}

			next(w, r.WithContext(withPlatformAdmin(r.Context())))
		}
	}
}

// withPlatformAdmin marks ctx as having passed the platform-admin OU check.
func withPlatformAdmin(ctx context.Context) context.Context {
	return context.WithValue(ctx, platformAdminKey, true)
}

// IsPlatformAdminRequest reports whether RequirePlatformAdminOU admitted this
// request.
//
// A handler on a cross-organization route calls this and refuses when it is
// false. That looks redundant next to the middleware — it is not. The gate is
// the only thing between an ordinary tenant token and every tenant's rows, and
// a route re-registered through the wrong registrar would drop it silently
// while still compiling, still passing, and still answering requests. Asking
// the context turns that mistake into a 403 instead of a cross-tenant leak.
func IsPlatformAdminRequest(ctx context.Context) bool {
	admitted, ok := ctx.Value(platformAdminKey).(bool)
	return ok && admitted
}
