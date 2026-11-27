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

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
)

// Org resolution follows the route's surface, not whether its pattern contains {orgName}.

// orgResolutionProbe serves one request through a route registered on rr and reports the status, whether the handler ran, and the resolved org.
func orgResolutionProbe(t *testing.T, rr *RouteRegistrar, pattern, path string, claims *jwtassertion.TokenClaims,
) (status int, ran bool, resolved ResolvedOrg, resolvedOK bool) {
	t.Helper()

	mux := http.NewServeMux()
	rr.mux = mux
	rr.HandleFuncWithValidation(pattern, func(w http.ResponseWriter, r *http.Request) {
		ran = true
		resolved, resolvedOK = GetResolvedOrg(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if claims != nil {
		req = req.WithContext(jwtassertion.ContextWithTokenClaims(req.Context(), claims))
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code, ran, resolved, resolvedOK
}

// An API-surface route without {orgName} still resolves its org from the token.
func TestAPISurfaceResolvesOrgWithoutOrgNameInPattern(t *testing.T) {
	rr := NewRouteRegistrar(http.NewServeMux(), stubResolver{}, nil)
	claims := &jwtassertion.TokenClaims{Sub: "user-a", OuId: "ou-a", OuHandle: "orga"}

	status, ran, resolved, ok := orgResolutionProbe(t, rr, "GET /api/v1/agents", "/api/v1/agents", claims)

	if status != http.StatusOK || !ran {
		t.Fatalf("want 200 and handler run, got %d ran=%v", status, ran)
	}
	if !ok {
		t.Fatal("no org on the handler's context: RequireOrgMatch was not installed on a route without {orgName}")
	}
	if resolved.OUID != "ou-a" || resolved.OuHandle != "orga" {
		t.Fatalf("org must come from the token, got %q/%q", resolved.OUID, resolved.OuHandle)
	}
}

// A token with no org identity is refused on a route without {orgName}.
func TestAPISurfaceRejectsMissingOrgWithoutOrgNameInPattern(t *testing.T) {
	rr := NewRouteRegistrar(http.NewServeMux(), stubResolver{}, nil)
	claims := &jwtassertion.TokenClaims{Sub: "user-a"}

	status, ran, _, _ := orgResolutionProbe(t, rr, "GET /api/v1/agents", "/api/v1/agents", claims)

	if status != http.StatusForbidden {
		t.Fatalf("a token with no org identity must be refused, got %d", status)
	}
	if ran {
		t.Fatal("handler must not run for a token with no org identity")
	}
}

// The internal surface has no JWT and a nil resolver, so it skips org resolution.
func TestInternalSurfaceSkipsOrgResolution(t *testing.T) {
	rr := NewInternalRouteRegistrar(http.NewServeMux(), nil)

	status, ran, _, ok := orgResolutionProbe(t, rr, "GET /api/internal/v1/gateways/config",
		"/api/internal/v1/gateways/config", nil)

	if status != http.StatusOK || !ran {
		t.Fatalf("internal surface must not be org-guarded, got %d ran=%v", status, ran)
	}
	if ok {
		t.Fatal("internal surface must not have an org resolved onto the context")
	}
}
