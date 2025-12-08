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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

type TokenClaims struct {
	Sub   uuid.UUID `json:"sub"`
	Scope string    `json:"scope"`
	Exp   int       `json:"exp"`
}

type tokenClaimsCtxKey struct{}

type Middleware func(http.Handler) http.Handler

var assertionTokenClaimsKey tokenClaimsCtxKey

type jwtTokenCtx struct{}

var jwtToken jwtTokenCtx

type ctxKeyName string

const (
	scopesKey ctxKeyName = "scopes"
)

func JWTAuthMiddleware(header string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := r.Header.Get(header)
			if tokenString == "" {
				utils.WriteErrorResponse(w, http.StatusUnauthorized, fmt.Sprintf("missing header: %s", header))
				return
			}
			// replace "Bearer " prefix
			tokenString = strings.Replace(tokenString, "Bearer ", "", 1)
			// we don't need to validate the token, just extract the claims
			claims, err := extractClaimsFromJWT(tokenString)
			if err != nil {
				utils.WriteErrorResponse(w, http.StatusUnauthorized, fmt.Sprintf("invalid jwt: %v", err))
				return
			}
			ctx := r.Context()
			ctx = context.WithValue(ctx, assertionTokenClaimsKey, claims)
			ctx = context.WithValue(ctx, jwtToken, tokenString)
			ctx = context.WithValue(ctx, scopesKey, claims.Scope)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

func GetTokenClaims(ctx context.Context) *TokenClaims {
	claims, ok := ctx.Value(assertionTokenClaimsKey).(*TokenClaims)
	if !ok {
		return nil
	}
	return claims
}

func GetJWTFromContext(ctx context.Context) string {
	token, ok := ctx.Value(jwtToken).(string)
	if !ok {
		return ""
	}
	return token
}

func HasAllScopes(ctx context.Context, requiredScopes []string) bool {
	scopes, ok := ctx.Value(scopesKey).(string)
	if !ok {
		return false
	}
	scopeSet := make(map[string]struct{})
	for _, s := range strings.Fields(scopes) {
		scopeSet[s] = struct{}{}
	}
	for _, scope := range requiredScopes {
		if _, exists := scopeSet[scope]; !exists {
			// as soon as one is missing return false
			return false
		}
	}
	// all required scopes found
	return true
}

func extractClaimsFromJWT(tokenString string) (*TokenClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid jwt, failed to parse, found %d parts", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid jwt, failed to decode payload: %w", err)
	}

	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("invalid jwt, failed to unmarshal payload: %w", err)
	}
	return &claims, nil
}
