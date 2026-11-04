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

package jwtassertion

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func TestBuildClientOrg(t *testing.T) {
	const org = "6f1e2c3d-0000-4000-8000-000000000001"
	tests := []struct {
		name   string
		claims *TokenClaims
		want   string
	}{
		{name: "no claims", claims: nil, want: ""},
		{
			name:   "own build client",
			claims: &TokenClaims{OuId: org, RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{"amp-build-" + org}}},
			want:   org,
		},
		{
			name: "own build client among other audiences",
			claims: &TokenClaims{OuId: org, RegisteredClaims: jwt.RegisteredClaims{
				Audience: jwt.ClaimStrings{"urn:wso2:cloud", "amp-build-" + org},
			}},
			want: org,
		},
		{
			name: "another organization's build client",
			claims: &TokenClaims{OuId: org, RegisteredClaims: jwt.RegisteredClaims{
				Audience: jwt.ClaimStrings{"amp-build-6f1e2c3d-0000-4000-8000-000000000002"},
			}},
			want: "",
		},
		{
			name:   "build client audience without an organization claim",
			claims: &TokenClaims{RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{"amp-build-"}}},
			want:   "",
		},
		{
			name:   "publisher client of the same organization",
			claims: &TokenClaims{OuId: org, RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{"amp-publisher-" + org}}},
			want:   "",
		},
		{
			name:   "prefix match is not enough",
			claims: &TokenClaims{OuId: org, RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{"amp-build-" + org + "-extra"}}},
			want:   "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.claims != nil {
				ctx = ContextWithTokenClaims(ctx, tc.claims)
			}
			if got := BuildClientOrg(ctx); got != tc.want {
				t.Errorf("BuildClientOrg() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildClientAuthMiddleware(t *testing.T) {
	const org = "6f1e2c3d-0000-4000-8000-000000000001"
	reached := false
	handler := BuildClientAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("refuses a token that is not the organization's build client", func(t *testing.T) {
		reached = false
		req := httptest.NewRequest(http.MethodPost, "/build-publisher/builds/b/workload", nil)
		req = req.WithContext(ContextWithTokenClaims(req.Context(), &TokenClaims{
			OuId:             org,
			RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{"AGENT_MANAGER_CONSOLE"}},
		}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden || reached {
			t.Fatalf("status = %d, reached = %v; want 403 and not reached", rec.Code, reached)
		}
	})

	t.Run("admits the organization's own build client", func(t *testing.T) {
		reached = false
		req := httptest.NewRequest(http.MethodPost, "/build-publisher/builds/b/workload", nil)
		req = req.WithContext(ContextWithTokenClaims(req.Context(), &TokenClaims{
			OuId:             org,
			RegisteredClaims: jwt.RegisteredClaims{Audience: jwt.ClaimStrings{"amp-build-" + org}},
		}))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !reached {
			t.Fatalf("status = %d, reached = %v; want 200 and reached", rec.Code, reached)
		}
	})
}
