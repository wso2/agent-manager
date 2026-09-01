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
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// setRBACEnabled flips the process-global RBAC switch for one test and restores
// it on cleanup. Tests using it must not run in parallel.
func setRBACEnabled(t *testing.T, enabled bool) {
	t.Helper()
	cfg := config.GetConfig()
	orig := cfg.RBACEnabled
	cfg.RBACEnabled = enabled
	t.Cleanup(func() { cfg.RBACEnabled = orig })
}

// serveAllPermissions runs RequireAllPermissions around a handler that records
// whether it ran, for a token carrying exactly grantedScopes.
func serveAllPermissions(t *testing.T, grantedScopes string, perms ...rbac.Permission) (status int, handlerRan bool) {
	t.Helper()
	next := func(w http.ResponseWriter, _ *http.Request) {
		handlerRan = true
		w.WriteHeader(http.StatusOK)
	}
	req := httptest.NewRequest(http.MethodPost, "/orgs/acme/projects/p/agents/a/deployments/state", nil)
	req = req.WithContext(jwtassertion.ContextWithTokenClaimsAndScope(req.Context(),
		&jwtassertion.TokenClaims{Sub: "user-a", OuId: "ou-1", Scope: grantedScopes}))
	rec := httptest.NewRecorder()
	RequireAllPermissions(perms...)(next)(rec, req)
	return rec.Code, handlerRan
}

// TestRequireAllPermissions_AllowsWhenEveryScopeHeld is the passing case: the
// caller holds both independent axes the route is gated on.
func TestRequireAllPermissions_AllowsWhenEveryScopeHeld(t *testing.T) {
	setRBACEnabled(t, true)
	granted := rbac.AgentSuspend.Scope() + " " + rbac.AgentEnvNonProduction.Scope()
	status, ran := serveAllPermissions(t, granted, rbac.AgentSuspend, rbac.AgentEnvNonProduction)
	if !ran {
		t.Error("handler did not run despite the token holding every required scope")
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
}

// TestRequireAllPermissions_DeniesOnPartialScopes is the whole reason this
// helper is not RequireAnyPermission: holding one axis must not admit the other.
func TestRequireAllPermissions_DeniesOnPartialScopes(t *testing.T) {
	setRBACEnabled(t, true)
	for _, held := range []rbac.Permission{rbac.AgentSuspend, rbac.AgentEnvNonProduction} {
		status, ran := serveAllPermissions(t, held.Scope(), rbac.AgentSuspend, rbac.AgentEnvNonProduction)
		if ran {
			t.Errorf("handler ran with only %s held", held.Scope())
		}
		if status != http.StatusForbidden {
			t.Errorf("holding only %s: status = %d, want %d", held.Scope(), status, http.StatusForbidden)
		}
	}
}

// TestRequireAllPermissions_SkipsCheckWhenRBACDisabled matches
// RequirePermission's zero-downtime rollout switch.
func TestRequireAllPermissions_SkipsCheckWhenRBACDisabled(t *testing.T) {
	setRBACEnabled(t, false)
	status, ran := serveAllPermissions(t, "", rbac.AgentSuspend, rbac.AgentEnvNonProduction)
	if !ran {
		t.Error("handler did not run with RBAC disabled")
	}
	if status != http.StatusOK {
		t.Errorf("status = %d, want %d", status, http.StatusOK)
	}
}

// TestRequireScopes_PanicsOnEmptyPermissionList covers what the panic is for:
// with no permission in the list, the allPermissions gate finds nothing missing
// and would admit every caller. Both variadic constructors go through the same
// gate, so both must refuse at registration.
func TestRequireScopes_PanicsOnEmptyPermissionList(t *testing.T) {
	constructors := map[string]func(...rbac.Permission) func(http.HandlerFunc) http.HandlerFunc{
		"RequireAllPermissions": RequireAllPermissions,
		"RequireAnyPermission":  RequireAnyPermission,
	}
	for name, construct := range constructors {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Errorf("%s() with no permissions did not panic", name)
				}
			}()
			construct()
		})
	}
}
