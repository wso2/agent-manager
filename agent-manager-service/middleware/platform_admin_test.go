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

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
)

// setPlatformAdminOUID sets the process-global platform-admin OU for one test
// and restores it on cleanup. Tests using it must not run in parallel.
func setPlatformAdminOUID(t *testing.T, ouID string) {
	t.Helper()
	cfg := config.GetConfig()
	orig := cfg.PlatformAdminOUID
	cfg.PlatformAdminOUID = ouID
	t.Cleanup(func() { cfg.PlatformAdminOUID = orig })
}

// servePlatformAdmin runs RequirePlatformAdminOU around a handler that records
// whether it ran and whether the context was marked, for a token carrying
// tokenOUID.
func servePlatformAdmin(t *testing.T, tokenOUID string) (status int, handlerRan, marked bool) {
	t.Helper()
	next := func(w http.ResponseWriter, r *http.Request) {
		handlerRan = true
		marked = IsPlatformAdminRequest(r.Context())
		w.WriteHeader(http.StatusOK)
	}
	req := httptest.NewRequest(http.MethodGet, "/platform/gateways/failure-summary", nil)
	req = req.WithContext(jwtassertion.ContextWithTokenClaims(req.Context(),
		&jwtassertion.TokenClaims{Sub: "user-a", OuId: tokenOUID, OuHandle: "some-handle"}))
	rec := httptest.NewRecorder()
	RequirePlatformAdminOU()(next)(rec, req)
	return rec.Code, handlerRan, marked
}

// TestRequirePlatformAdminOU_AdmitsConfiguredOU is the passing case, and it also
// pins that admission marks the context: the handler's own guard reads that mark,
// so a gate that admitted without marking would fail closed and 403 in
// production while this test still passed on the status alone.
func TestRequirePlatformAdminOU_AdmitsConfiguredOU(t *testing.T) {
	setPlatformAdminOUID(t, "ou-platform-admin")
	status, ran, marked := servePlatformAdmin(t, "ou-platform-admin")
	if !ran || status != http.StatusOK {
		t.Fatalf("configured OU: want 200 & handler run, got %d ran=%v", status, ran)
	}
	if !marked {
		t.Error("configured OU: context was not marked as platform-admin")
	}
}

// TestRequirePlatformAdminOU_DeniesOtherOU is the isolation guarantee: an
// ordinary tenant token must not reach a route that reads every tenant's rows.
func TestRequirePlatformAdminOU_DeniesOtherOU(t *testing.T) {
	setPlatformAdminOUID(t, "ou-platform-admin")
	status, ran, _ := servePlatformAdmin(t, "ou-some-tenant")
	if ran {
		t.Fatal("other OU: handler must NOT run")
	}
	if status != http.StatusForbidden {
		t.Fatalf("other OU: want 403, got %d", status)
	}
}

// The kill-switch test that used to live here is gone: main removed the RBAC
// kill-switch entirely (config.RBACEnabled and the setRBACEnabled helper no
// longer exist), so there is no "RBAC off" state left for this gate to
// short-circuit through. Scope checks are now unconditional. The invariant it
// guarded — a non-admin OU is refused — is still covered by
// TestRequirePlatformAdminOU_DeniesOtherOU above, which must keep passing
// independently of any scope the caller holds.

// TestRequirePlatformAdminOU_DeniesWhenUnconfigured pins the fail-closed
// reading of an empty PLATFORM_ADMIN_OU_ID. An unset value must not mean "no
// restriction" — a deployment with no platform-admin org has no caller entitled
// to cross-tenant data, and the empty string must not match an empty claim
// either.
func TestRequirePlatformAdminOU_DeniesWhenUnconfigured(t *testing.T) {
	setPlatformAdminOUID(t, "")
	for _, tokenOUID := range []string{"ou-some-tenant", ""} {
		status, ran, _ := servePlatformAdmin(t, tokenOUID)
		if ran {
			t.Errorf("unconfigured (token ouId %q): handler must NOT run", tokenOUID)
		}
		if status != http.StatusForbidden {
			t.Errorf("unconfigured (token ouId %q): want 403, got %d", tokenOUID, status)
		}
	}
}

// TestRequirePlatformAdminOU_DeniesMissingClaims covers a request that reached
// the gate with no token identity at all — a malformed or unauthenticated
// request, which must not be treated as the admin.
func TestRequirePlatformAdminOU_DeniesMissingClaims(t *testing.T) {
	setPlatformAdminOUID(t, "ou-platform-admin")

	ran := false
	next := func(w http.ResponseWriter, _ *http.Request) {
		ran = true
		w.WriteHeader(http.StatusOK)
	}
	req := httptest.NewRequest(http.MethodGet, "/platform/gateways/failure-summary", nil)
	rec := httptest.NewRecorder()
	RequirePlatformAdminOU()(next)(rec, req)

	if ran {
		t.Fatal("missing claims: handler must NOT run")
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("missing claims: want 403, got %d", rec.Code)
	}
}

// TestIsPlatformAdminRequest_FalseByDefault pins the handler guard's default. A
// context that never passed the gate must report false, or the guard would
// admit exactly the misregistration it exists to catch.
func TestIsPlatformAdminRequest_FalseByDefault(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/platform/gateways/failure-summary", nil)
	if IsPlatformAdminRequest(req.Context()) {
		t.Error("a context that never passed RequirePlatformAdminOU must not report platform-admin")
	}
}
