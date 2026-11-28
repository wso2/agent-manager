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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
)

// stubResolver satisfies OrgResolver for the RequireOrgMatch signature. Under
// the token-trust model the resolver is never consulted, so ResolveOUID panics
// if it is ever called.
type stubResolver struct{}

func (stubResolver) ResolveOUID(_ context.Context, _ string) (string, error) {
	panic("ResolveOUID must not be called under the token-trust model")
}

// serve runs RequireOrgMatch around a handler that records whether it ran and
// the org it resolved, for a request whose {orgName} path segment is pathOrg and
// whose token carries the given ouHandle/ouId.
func serve(t *testing.T, pathOrg, tokenHandle, tokenOUID string) (status int, handlerRan bool, resolved ResolvedOrg) {
	t.Helper()
	claims := &jwtassertion.TokenClaims{Sub: "user-a", OuId: tokenOUID, OuHandle: tokenHandle}

	next := func(w http.ResponseWriter, r *http.Request) {
		handlerRan = true
		resolved, _ = GetResolvedOrg(r.Context())
		w.WriteHeader(http.StatusOK)
	}

	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/orgs/"+pathOrg+"/environments/default/agent-identities/roles", nil)
	req.SetPathValue("orgName", pathOrg)
	req = req.WithContext(jwtassertion.ContextWithTokenClaims(req.Context(), claims))

	rec := httptest.NewRecorder()
	RequireOrgMatch(stubResolver{})(next)(rec, req)
	return rec.Code, handlerRan, resolved
}

// TestRequireOrgMatch_TokenTrust_IgnoresPathOrg is the core token-trust guarantee:
// the org is taken from the TOKEN, never the {orgName} path. An orgA token used
// on an /orgs/orgb/... route must still resolve to orgA — so it operates on its
// own tenant and cannot reach orgB's data by changing the path segment.
func TestRequireOrgMatch_TokenTrust_IgnoresPathOrg(t *testing.T) {
	status, ran, resolved := serve(t, "orgb", "orga", "01900000-0000-7000-8000-0000000000a1")
	if status != http.StatusOK || !ran {
		t.Fatalf("token-trust: want 200 & handler run, got %d ran=%v", status, ran)
	}
	if resolved.OUID != "01900000-0000-7000-8000-0000000000a1" || resolved.OuHandle != "orga" {
		t.Fatalf("token-trust: resolved org must come from the token (01900000-0000-7000-8000-0000000000a1/orga), got %q/%q",
			resolved.OUID, resolved.OuHandle)
	}
}

// TestRequireOrgMatch_OwnOrg confirms the normal case: a token on its own org
// route resolves to the token's org.
func TestRequireOrgMatch_OwnOrg(t *testing.T) {
	status, ran, resolved := serve(t, "orga", "orga", "01900000-0000-7000-8000-0000000000a1")
	if status != http.StatusOK || !ran {
		t.Fatalf("own-org: want 200 & handler run, got %d ran=%v", status, ran)
	}
	if resolved.OUID != "01900000-0000-7000-8000-0000000000a1" {
		t.Fatalf("own-org: resolved OUID want 01900000-0000-7000-8000-0000000000a1, got %q", resolved.OUID)
	}
}

// TestRequireOrgMatch_MissingOUIdentity still rejects a token that carries no
// org identity at all — that is a malformed/untrusted token, not a tenant choice.
func TestRequireOrgMatch_MissingOUIdentity(t *testing.T) {
	status, ran, _ := serve(t, "orga", "", "")
	if status != http.StatusForbidden {
		t.Fatalf("missing ou identity: want 403, got %d", status)
	}
	if ran {
		t.Fatal("missing ou identity: handler must NOT run")
	}
}

// TestRequireOrgMatch_NonUUIDOUID rejects a token whose ouId is not a UUID: the
// ouId is the organization's id wherever it is stored, so an identity provider
// that sends anything else is misconfigured, and is told so at the door.
func TestRequireOrgMatch_NonUUIDOUID(t *testing.T) {
	for _, ouID := range []string{"default", "ou-a", "01900000-0000-7000-8000", "x01900000-0000-7000-8000-0000000000a1y"} {
		status, ran, _ := serve(t, "orga", "orga", ouID)
		if status != http.StatusForbidden {
			t.Fatalf("ouId %q: want 403, got %d", ouID, status)
		}
		if ran {
			t.Fatalf("ouId %q: handler must NOT run", ouID)
		}
	}
}
