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
	"testing"
	"time"

	"github.com/google/uuid"
)

// NewMockMiddleware creates a mock JWT middleware for testing
func NewMockMiddleware(t *testing.T, orgId uuid.UUID, userIdpId uuid.UUID) Middleware {
	t.Helper()

	tokenClaims := &TokenClaims{
		Sub:   userIdpId,
		Scope: "scopes",
		Exp:   int(time.Now().Add(time.Hour).Unix()),
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()

			// Set the context values that GetTokenClaims expects
			ctx = context.WithValue(ctx, assertionTokenClaimsKey, tokenClaims)

			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}
