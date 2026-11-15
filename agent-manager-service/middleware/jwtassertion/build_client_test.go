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

	"github.com/wso2/agent-manager/agent-manager-service/config"
)

const (
	buildTestOrg    = "6f1e2c3d-0000-4000-8000-000000000001"
	buildTestClient = "amp-api-client"
	buildTestScope  = "amp:agent:build"
)

// withIssuingClient sets the service's identity provider client (the client build
// tokens are issued to) for one test.
func withIssuingClient(t *testing.T, clientID string) {
	t.Helper()
	cfg := config.GetConfig()
	orig := cfg.IDP.ClientID
	t.Cleanup(func() { cfg.IDP.ClientID = orig })
	cfg.IDP.ClientID = clientID
}

func TestBuildTokenScopeIsTheAgentBuildPermission(t *testing.T) {
	if got := BuildTokenScope(); got != buildTestScope {
		t.Errorf("BuildTokenScope() = %q, want %q", got, buildTestScope)
	}
}

func TestBuildClientOrg(t *testing.T) {
	withIssuingClient(t, buildTestClient)
	tests := []struct {
		name   string
		claims *TokenClaims
		want   string
	}{
		{name: "no claims", claims: nil, want: ""},
		{
			name:   "build token for an organization",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient, Scope: buildTestScope},
			want:   buildTestOrg,
		},
		{
			name:   "organization and client with surrounding whitespace",
			claims: &TokenClaims{OuId: " " + buildTestOrg + " ", ClientID: " " + buildTestClient + " ", Scope: buildTestScope},
			want:   buildTestOrg,
		},
		{
			name:   "issuing client without an organization",
			claims: &TokenClaims{ClientID: buildTestClient, Scope: buildTestScope},
			want:   "",
		},
		{
			name:   "issuing client without the build scope",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient},
			want:   "",
		},
		{
			name:   "issuing client with the build scope and more",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient, Scope: buildTestScope + " amp:org:view"},
			want:   "",
		},
		{
			name:   "this service's own token, requested with system",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient, Scope: "system"},
			want:   "",
		},
		{
			name:   "a scope merely starting with the build scope",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient, Scope: buildTestScope + "-extra"},
			want:   "",
		},
		{
			name:   "another client of the same organization",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: "amp-publisher-" + buildTestOrg, Scope: buildTestScope},
			want:   "",
		},
		{
			name:   "user token of the same organization holding the build permission",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: "test-console-client", Sub: "user-1", Scope: buildTestScope},
			want:   "",
		},
		{
			name: "audience naming the issuing client is not enough",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: "OTHER_APP", Scope: buildTestScope, RegisteredClaims: jwt.RegisteredClaims{
				Audience: jwt.ClaimStrings{buildTestClient},
			}},
			want: "",
		},
		{
			name:   "client ID prefix match is not enough",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient + "_EXTRA", Scope: buildTestScope},
			want:   "",
		},
		{
			name:   "no client ID",
			claims: &TokenClaims{OuId: buildTestOrg, Scope: buildTestScope},
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

func TestBuildClientOrgAdmitsThePlatformAdminOrganization(t *testing.T) {
	const adminOU = "a5000000-0000-0000-0000-adminroot001"
	withIssuingClient(t, buildTestClient)
	cfg := config.GetConfig()
	orig := cfg.PlatformAdminOUID
	t.Cleanup(func() { cfg.PlatformAdminOUID = orig })
	cfg.PlatformAdminOUID = adminOU

	// The admin organization's build token is a build token like any other.
	ctx := ContextWithTokenClaims(context.Background(), &TokenClaims{OuId: adminOU, ClientID: buildTestClient, Scope: buildTestScope})
	if got := BuildClientOrg(ctx); got != adminOU {
		t.Errorf("BuildClientOrg(admin build token) = %q, want %q", got, adminOU)
	}

	// This service's own token carries the same client and the admin
	// organization, but not the build scope alone, so it is not one.
	for _, scope := range []string{"", "system", buildTestScope + " amp:org:view amp:agent:create"} {
		ctx := ContextWithTokenClaims(context.Background(), &TokenClaims{OuId: adminOU, ClientID: buildTestClient, Scope: scope})
		if got := BuildClientOrg(ctx); got != "" {
			t.Errorf("BuildClientOrg(admin token with scope %q) = %q, want \"\"", scope, got)
		}
	}
}

func TestHasOnlyScope(t *testing.T) {
	tests := []struct {
		name  string
		claim string
		scope string
		want  bool
	}{
		{name: "exactly the scope", claim: buildTestScope, scope: buildTestScope, want: true},
		{name: "the scope with surrounding whitespace", claim: "  " + buildTestScope + " ", scope: buildTestScope, want: true},
		{name: "the scope repeated", claim: buildTestScope + " " + buildTestScope, scope: buildTestScope, want: true},
		{name: "no scopes", claim: "", scope: buildTestScope, want: false},
		{name: "another scope", claim: "amp:org:view", scope: buildTestScope, want: false},
		{name: "the scope and another", claim: buildTestScope + " amp:org:view", scope: buildTestScope, want: false},
		{name: "an empty wanted scope never matches", claim: "", scope: "", want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := HasOnlyScope(tc.claim, tc.scope); got != tc.want {
				t.Errorf("HasOnlyScope(%q, %q) = %v, want %v", tc.claim, tc.scope, got, tc.want)
			}
		})
	}
}

func TestBuildClientOrgWithoutIssuingClient(t *testing.T) {
	// With no client configured, no token can be a build token, not even one
	// that carries no client ID either.
	withIssuingClient(t, "")
	ctx := ContextWithTokenClaims(context.Background(), &TokenClaims{OuId: buildTestOrg, Scope: buildTestScope})
	if got := BuildClientOrg(ctx); got != "" {
		t.Errorf("BuildClientOrg() = %q, want \"\"", got)
	}
}

func TestBuildClientAuthMiddleware(t *testing.T) {
	withIssuingClient(t, buildTestClient)
	reached := false
	handler := BuildClientAuthMiddleware()(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	}))

	serve := func(claims *TokenClaims) *httptest.ResponseRecorder {
		reached = false
		req := httptest.NewRequest(http.MethodPost, "/build-publisher/builds/b/workload", nil)
		if claims != nil {
			req = req.WithContext(ContextWithTokenClaims(req.Context(), claims))
		}
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		return rec
	}

	t.Run("refuses a token issued to another client", func(t *testing.T) {
		rec := serve(&TokenClaims{OuId: buildTestOrg, ClientID: "test-console-client", Scope: buildTestScope})
		if rec.Code != http.StatusForbidden || reached {
			t.Fatalf("status = %d, reached = %v; want 403 and not reached", rec.Code, reached)
		}
	})

	t.Run("refuses a token of the issuing client that names no organization", func(t *testing.T) {
		rec := serve(&TokenClaims{ClientID: buildTestClient, Scope: buildTestScope})
		if rec.Code != http.StatusForbidden || reached {
			t.Fatalf("status = %d, reached = %v; want 403 and not reached", rec.Code, reached)
		}
	})

	t.Run("refuses a token of the issuing client without the build scope", func(t *testing.T) {
		rec := serve(&TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient})
		if rec.Code != http.StatusForbidden || reached {
			t.Fatalf("status = %d, reached = %v; want 403 and not reached", rec.Code, reached)
		}
	})

	t.Run("refuses a request without claims", func(t *testing.T) {
		rec := serve(nil)
		if rec.Code != http.StatusForbidden || reached {
			t.Fatalf("status = %d, reached = %v; want 403 and not reached", rec.Code, reached)
		}
	})

	t.Run("admits a build token", func(t *testing.T) {
		rec := serve(&TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient, Scope: buildTestScope})
		if rec.Code != http.StatusOK || !reached {
			t.Fatalf("status = %d, reached = %v; want 200 and reached", rec.Code, reached)
		}
	})
}
