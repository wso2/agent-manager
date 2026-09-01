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
	"net/http"
	"slices"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
)

// publisherImplicitPermissions is the fixed permission set granted to
// amp-publisher-* audience tokens, which carry no amp scopes of their own.
// Publishers may read traces and nothing else — the explicit form of the
// carve-out previously enforced by RejectPublisherAudience.
var publisherImplicitPermissions = []rbac.Permission{rbac.TraceRead}

// ErrMissingClaims and ErrInsufficientPermissions are the two authorization
// failure modes of AuthorizePermission.
var (
	ErrMissingClaims           = errors.New("missing token claims")
	ErrInsufficientPermissions = errors.New("insufficient permissions")
)

// AuthorizePermission is the single scope policy shared by the REST routes
// (via RequirePermission) and the am-obs-mcp per-tool guards: publisher-
// audience tokens are confined to their implicit permission set regardless of
// rbacEnabled, so pre-authz publisher restrictions never regress while the
// kill-switch is off; ordinary tokens need the perm's amp scope only when
// rbacEnabled. Claims must come from a JWTAuth-validated token.
func AuthorizePermission(claims *TokenClaims, perm rbac.Permission, rbacEnabled bool) error {
	if claims != nil && hasPublisherAudience(claims.Audience) {
		if !slices.Contains(publisherImplicitPermissions, perm) {
			return ErrInsufficientPermissions
		}
		return nil
	}
	if !rbacEnabled {
		return nil
	}
	if claims == nil {
		return ErrMissingClaims
	}
	if !slices.Contains(strings.Fields(claims.Scope), perm.Scope()) {
		return ErrInsufficientPermissions
	}
	return nil
}

// RequirePermission returns middleware that applies AuthorizePermission to the
// JWTAuth-validated token claims. Must run inside JWTAuth: it reads claims
// from the request context.
func RequirePermission(rbacEnabled bool, perm rbac.Permission) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := AuthorizePermission(GetTokenClaims(r.Context()), perm, rbacEnabled); err != nil {
				writeAuthError(w, http.StatusForbidden, err.Error())
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
