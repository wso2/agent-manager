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
)

const shadowPublishTestScope = "amp:shadow-agent:publish"

func TestShadowPublisherOrg(t *testing.T) {
	withIssuingClient(t, buildTestClient)
	tests := []struct {
		name   string
		claims *TokenClaims
		want   string
	}{
		{name: "no claims", claims: nil, want: ""},
		{
			name:   "scan token for an organization",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient, Scope: shadowPublishTestScope},
			want:   buildTestOrg,
		},
		{
			name: "user token of an Admin, whose role holds the publish permission",
			claims: &TokenClaims{
				OuId: buildTestOrg, OuHandle: "acme", ClientID: "test-console-client", Sub: "user-1",
				Scope: shadowPublishTestScope,
			},
			want: "",
		},
		{
			name: "user token holding only the publish scope",
			claims: &TokenClaims{
				OuId: buildTestOrg, ClientID: "test-console-client", Sub: "user-1", Scope: shadowPublishTestScope,
			},
			want: "",
		},
		{
			name: "issuing client with the publish scope and more",
			claims: &TokenClaims{
				OuId: buildTestOrg, ClientID: buildTestClient, Scope: shadowPublishTestScope + " amp:shadow-agent:manage",
			},
			want: "",
		},
		{
			name:   "a build token is not a scan token",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient, Scope: buildTestScope},
			want:   "",
		},
		{
			name:   "issuing client without an organization",
			claims: &TokenClaims{ClientID: buildTestClient, Scope: shadowPublishTestScope},
			want:   "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			if tc.claims != nil {
				ctx = ContextWithTokenClaims(ctx, tc.claims)
			}
			if got := ShadowPublisherOrg(ctx); got != tc.want {
				t.Errorf("ShadowPublisherOrg() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShadowPublisherAuthMiddleware(t *testing.T) {
	withIssuingClient(t, buildTestClient)
	reached := false
	handler := ShadowPublisherAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))
	serve := func(claims *TokenClaims) int {
		reached = false
		req := httptest.NewRequest(http.MethodPost, "/shadow-agent-publisher/scans/s/results", nil)
		if claims != nil {
			req = req.WithContext(ContextWithTokenClaims(req.Context(), claims))
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	refused := map[string]*TokenClaims{
		"user token with the publish scope": {
			OuId: buildTestOrg, OuHandle: "acme", ClientID: "test-console-client", Sub: "user-1", Scope: shadowPublishTestScope,
		},
		"client token with extra scopes": {
			OuId: buildTestOrg, ClientID: buildTestClient, Scope: shadowPublishTestScope + " amp:org:view",
		},
		"no claims": nil,
	}
	for name, claims := range refused {
		t.Run("refuses "+name, func(t *testing.T) {
			if code := serve(claims); code != http.StatusForbidden || reached {
				t.Fatalf("status = %d, reached = %v; want 403 and not reached", code, reached)
			}
		})
	}
	t.Run("admits a scan token", func(t *testing.T) {
		code := serve(&TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient, Scope: shadowPublishTestScope})
		if code != http.StatusOK || !reached {
			t.Fatalf("status = %d, reached = %v; want 200 and reached", code, reached)
		}
	})
}
