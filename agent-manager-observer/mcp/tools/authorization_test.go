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

package tools

import (
	"context"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
)

// tokenWithScopes builds a throwaway HS256-signed token carrying aud and a
// space-delimited scope claim. The tool guard re-parses without verifying the
// signature, so an arbitrary key is fine.
func tokenWithScopes(t *testing.T, aud, scope string) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"aud": aud, "scope": scope})
	signed, err := token.SignedString([]byte("k"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}
	return signed
}

// The per-tool guard must mirror the REST RequirePermission policy on the MCP
// transport: ordinary tokens need the tool's amp scope when RBAC is enabled,
// the kill-switch skips that check, and publisher-audience tokens are confined
// to their implicit trace-read permission regardless of the flag.
func TestRequireToolPermission(t *testing.T) {
	tests := []struct {
		name        string
		rbacEnabled bool
		perm        rbac.Permission
		authHeader  string
		wantAllow   bool
	}{
		{
			name:        "rbac on, token with required scope allowed",
			rbacEnabled: true,
			perm:        rbac.LogRead,
			authHeader:  "Bearer " + tokenWithScopes(t, "localhost", "amp:observability:log-read amp:project:read"),
			wantAllow:   true,
		},
		{
			name:        "rbac on, token without required scope rejected",
			rbacEnabled: true,
			perm:        rbac.LogRead,
			authHeader:  "Bearer " + tokenWithScopes(t, "localhost", "amp:project:read"),
			wantAllow:   false,
		},
		{
			name:        "rbac on, token with empty scope rejected",
			rbacEnabled: true,
			perm:        rbac.TraceRead,
			authHeader:  "Bearer " + tokenWithScopes(t, "localhost", ""),
			wantAllow:   false,
		},
		{
			name:        "rbac off, token without scopes allowed",
			rbacEnabled: false,
			perm:        rbac.MetricRead,
			authHeader:  "Bearer " + tokenWithScopes(t, "localhost", ""),
			wantAllow:   true,
		},
		{
			name:        "rbac on, publisher token allowed trace-read without scopes",
			rbacEnabled: true,
			perm:        rbac.TraceRead,
			authHeader:  "Bearer " + tokenWithScopes(t, "amp-publisher-acme", ""),
			wantAllow:   true,
		},
		{
			name:        "rbac off, publisher token still rejected on log-read",
			rbacEnabled: false,
			perm:        rbac.LogRead,
			authHeader:  "Bearer " + tokenWithScopes(t, "amp-publisher-acme", ""),
			wantAllow:   false,
		},
		{
			name:        "rbac on, missing authorization header rejected",
			rbacEnabled: true,
			perm:        rbac.LogRead,
			authHeader:  "",
			wantAllow:   false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			guard := requireToolPermission(tc.rbacEnabled, tc.perm)
			err := guard(requestWithAuth(tc.authHeader))
			if tc.wantAllow && err != nil {
				t.Fatalf("expected allow, got error: %v", err)
			}
			if !tc.wantAllow && err == nil {
				t.Fatal("expected rejection, got nil error")
			}
		})
	}
}

// With RBAC enabled, a tool wired with its permission must not reach the
// upstream client for a token lacking the scope, and must for one carrying it.
func TestObservabilityTools_ScopeEnforced(t *testing.T) {
	input := runtimeLogsInput{
		Organization: testOrgName,
		Project:      testProjectName,
		Agent:        testAgentName,
		Environment:  testEnvName,
	}

	call := func(fake *fakeObserverClient, req *gomcp.CallToolRequest) error {
		handler := getRuntimeLogs(controllers.NewObservabilityController(fake), requireToolPermission(true, rbac.LogRead))
		_, _, err := handler(context.Background(), req, input)
		return err
	}

	t.Run("token without log-read scope rejected", func(t *testing.T) {
		fake := newFakeObserverClient()
		req := requestWithAuth("Bearer " + tokenWithScopes(t, "localhost", "amp:observability:trace-read"))
		if err := call(fake, req); err == nil {
			t.Fatal("expected scope rejection, got nil error")
		}
		if len(fake.calls["QueryLogs"]) != 0 {
			t.Errorf("expected upstream QueryLogs not to be called, got %d calls", len(fake.calls["QueryLogs"]))
		}
	})

	t.Run("token with log-read scope passes", func(t *testing.T) {
		fake := newFakeObserverClient()
		req := requestWithAuth("Bearer " + tokenWithScopes(t, "localhost", "amp:observability:log-read"))
		if err := call(fake, req); err != nil {
			t.Fatalf("expected scoped token to pass, got error: %v", err)
		}
		if len(fake.calls["QueryLogs"]) == 0 {
			t.Error("expected upstream QueryLogs to be called for a scoped token")
		}
	})
}
