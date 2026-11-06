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
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient},
			want:   buildTestOrg,
		},
		{
			name:   "organization and client with surrounding whitespace",
			claims: &TokenClaims{OuId: " " + buildTestOrg + " ", ClientID: " " + buildTestClient + " "},
			want:   buildTestOrg,
		},
		{
			name:   "issuing client without an organization",
			claims: &TokenClaims{ClientID: buildTestClient},
			want:   "",
		},
		{
			name:   "another client of the same organization",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: "amp-publisher-" + buildTestOrg},
			want:   "",
		},
		{
			name:   "user token of the same organization",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: "test-console-client", Sub: "user-1"},
			want:   "",
		},
		{
			name: "audience naming the issuing client is not enough",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: "OTHER_APP", RegisteredClaims: jwt.RegisteredClaims{
				Audience: jwt.ClaimStrings{buildTestClient},
			}},
			want: "",
		},
		{
			name:   "client ID prefix match is not enough",
			claims: &TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient + "_EXTRA"},
			want:   "",
		},
		{
			name:   "no client ID",
			claims: &TokenClaims{OuId: buildTestOrg},
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

func TestBuildClientOrgRefusesThePlatformAdminOrganization(t *testing.T) {
	const adminOU = "a5000000-0000-0000-0000-adminroot001"
	withIssuingClient(t, buildTestClient)
	cfg := config.GetConfig()
	orig := cfg.PlatformAdminOUID
	t.Cleanup(func() { cfg.PlatformAdminOUID = orig })

	cfg.PlatformAdminOUID = adminOU
	for _, ou := range []string{adminOU, "A5000000-0000-0000-0000-ADMINROOT001"} {
		ctx := ContextWithTokenClaims(context.Background(), &TokenClaims{OuId: ou, ClientID: buildTestClient})
		if got := BuildClientOrg(ctx); got != "" {
			t.Errorf("BuildClientOrg(admin OU %q) = %q, want \"\"", ou, got)
		}
	}
	ctx := ContextWithTokenClaims(context.Background(), &TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient})
	if got := BuildClientOrg(ctx); got != buildTestOrg {
		t.Errorf("BuildClientOrg(customer org) = %q, want %q", got, buildTestOrg)
	}

	// Without a configured admin organization, nothing extra is refused.
	cfg.PlatformAdminOUID = ""
	ctx = ContextWithTokenClaims(context.Background(), &TokenClaims{OuId: adminOU, ClientID: buildTestClient})
	if got := BuildClientOrg(ctx); got != adminOU {
		t.Errorf("BuildClientOrg() without PLATFORM_ADMIN_OU_ID = %q, want %q", got, adminOU)
	}
}

func TestBuildClientOrgWithoutIssuingClient(t *testing.T) {
	// With no client configured, no token can be a build token, not even one
	// that carries no client ID either.
	withIssuingClient(t, "")
	ctx := ContextWithTokenClaims(context.Background(), &TokenClaims{OuId: buildTestOrg})
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
		rec := serve(&TokenClaims{OuId: buildTestOrg, ClientID: "test-console-client"})
		if rec.Code != http.StatusForbidden || reached {
			t.Fatalf("status = %d, reached = %v; want 403 and not reached", rec.Code, reached)
		}
	})

	t.Run("refuses a token of the issuing client that names no organization", func(t *testing.T) {
		rec := serve(&TokenClaims{ClientID: buildTestClient})
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
		rec := serve(&TokenClaims{OuId: buildTestOrg, ClientID: buildTestClient})
		if rec.Code != http.StatusOK || !reached {
			t.Fatalf("status = %d, reached = %v; want 200 and reached", rec.Code, reached)
		}
	})
}
