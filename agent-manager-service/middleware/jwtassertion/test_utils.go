// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package jwtassertion

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// MockOUID is the OU ID carried by tokens minted by NewMockMiddleware. It is
// distinct from the path org handle (as in a real token). Handlers scope org
// data by OU ID, so tests assert against this value rather than the path org.
const MockOUID = "mock-org-id"

// mockScopes is the scope set every mocked token carries: the whole permission
// catalog, since the Admin role is pinned to hold all of it. Handler tests are
// about the handler, not the gate — a token short of a scope would turn every
// one of them into a 403. Scope checks are exercised in middleware's own tests.
func mockScopes() string {
	perms := rbac.PredefinedRolePermissions[rbac.RoleAdmin]
	scopes := make([]string, 0, len(perms))
	for _, perm := range perms {
		scopes = append(scopes, perm.Scope())
	}
	return strings.Join(scopes, " ")
}

// NewMockMiddleware creates a mock JWT middleware for testing.
// Automatically extracts org from the request path if it contains /orgs/{orgName}/
func NewMockMiddleware(t *testing.T) Middleware {
	t.Helper()
	return NewMockMiddlewareWithOUID(t, MockOUID)
}

// NewMockMiddlewareWithOUID is like NewMockMiddleware but lets the caller pin
// the token's OU ID. Use it to exercise org-scoped behaviour that depends on a
// specific OU ID — e.g. an organization the OpenChoreo mock reports as missing.
func NewMockMiddlewareWithOUID(t *testing.T, ouID string) Middleware {
	t.Helper()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Extract org from path if present
			orgName := extractOrgFromPath(r.URL.Path)
			if orgName == "" {
				orgName = "mock-org"
			}

			// Create token claims. OuHandle mirrors the path org (the human handle);
			// OuId is a distinct identifier, as in a real token. Handlers scope by
			// OuId, so tests assert against this value, not the path org.
			tokenClaims := &TokenClaims{
				Scope:    mockScopes(),
				OuId:     ouID,
				OuHandle: orgName,
				RegisteredClaims: jwt.RegisteredClaims{
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				},
			}

			// Set the context values that the middleware expects
			ctx = context.WithValue(ctx, assertionTokenClaimsKey, tokenClaims)
			ctx = context.WithValue(ctx, jwtToken, "mock-jwt-token")
			ctx = ContextWithScopes(ctx, tokenClaims.Scope)

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

// extractOrgFromPath extracts the org name from a path like /api/v1/orgs/{org}/...
func extractOrgFromPath(path string) string {
	parts := strings.Split(path, "/")
	for i, part := range parts {
		if part == "orgs" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

// ContextWithTokenClaimsAndScope mirrors what the production auth middleware
// puts on the request context: the parsed claims AND the scope set that
// HasAllScopes reads. ContextWithTokenClaims alone is not enough for tests that
// exercise scope checks, because HasAllScopes uses a separate context key.
func ContextWithTokenClaimsAndScope(ctx context.Context, claims *TokenClaims) context.Context {
	return ContextWithScopes(ContextWithTokenClaims(ctx, claims), claims.Scope)
}
