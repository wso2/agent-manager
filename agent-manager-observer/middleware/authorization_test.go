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
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/golang-jwt/jwt/v5"
	"github.com/wso2/agent-manager/agent-manager-observer/rbac"
)

func doAuthzRequest(t *testing.T, rbacEnabled bool, perm rbac.Permission, claims *TokenClaims) (*httptest.ResponseRecorder, bool) {
	t.Helper()
	called := false
	h := RequirePermission(rbacEnabled, perm)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	if claims != nil {
		req = req.WithContext(ContextWithTokenClaims(req.Context(), claims))
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec, called
}

func userClaims(scope string) *TokenClaims {
	return &TokenClaims{Sub: "user-1", Scope: scope, OuId: "ou-1", OuHandle: "acme"}
}

func publisherClaims() *TokenClaims {
	return &TokenClaims{
		Sub: "amp-publisher-abc",
		RegisteredClaims: jwt.RegisteredClaims{
			Audience: jwt.ClaimStrings{"amp-publisher-abc"},
		},
	}
}

func TestRequirePermission(t *testing.T) {
	cases := []struct {
		name        string
		rbacEnabled bool
		perm        rbac.Permission
		claims      *TokenClaims
		wantStatus  int
		wantCalled  bool
	}{
		{"scope present passes", true, rbac.TraceRead, userClaims("amp:observability:trace-read amp:org:view"), http.StatusOK, true},
		{"scope missing 403", true, rbac.LogRead, userClaims("amp:observability:trace-read"), http.StatusForbidden, false},
		{"empty scope 403", true, rbac.TraceRead, userClaims(""), http.StatusForbidden, false},
		{"nil claims 403", true, rbac.TraceRead, nil, http.StatusForbidden, false},
		{"kill-switch off passes without scope", false, rbac.MetricRead, userClaims(""), http.StatusOK, true},
		{"kill-switch off nil claims passes", false, rbac.MetricRead, nil, http.StatusOK, true},
		{"publisher allowed on traces", true, rbac.TraceRead, publisherClaims(), http.StatusOK, true},
		{"publisher 403 on logs", true, rbac.LogRead, publisherClaims(), http.StatusForbidden, false},
		{"publisher 403 on build-logs", true, rbac.BuildLogRead, publisherClaims(), http.StatusForbidden, false},
		{"publisher 403 on metrics", true, rbac.MetricRead, publisherClaims(), http.StatusForbidden, false},
		{"publisher 403 on logs even with kill-switch off", false, rbac.LogRead, publisherClaims(), http.StatusForbidden, false},
		{"publisher allowed on traces with kill-switch off", false, rbac.TraceRead, publisherClaims(), http.StatusOK, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec, called := doAuthzRequest(t, tc.rbacEnabled, tc.perm, tc.claims)
			if rec.Code != tc.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
			if called != tc.wantCalled {
				t.Errorf("next called = %v, want %v", called, tc.wantCalled)
			}
		})
	}
}
