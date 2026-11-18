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

package api

import (
	"slices"
	"strings"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	_ "github.com/wso2/agent-manager/agent-manager-service/modules/aigateway" // declares ai-gateway:*
	"github.com/wso2/agent-manager/agent-manager-service/modules/legacy"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// manifestRegistry is the permission registry of the core plus the built-in
// modules (legacy, gateway), which together declare every permission of the
// full distribution. It also returns the legacy module's manifest.
func manifestRegistry(t *testing.T) (*rbac.Registry, modkit.Manifest) {
	t.Helper()
	var legacyMF modkit.Manifest
	reg := rbac.NewRegistry()
	for _, m := range modkit.Registered() {
		mf := m.Manifest()
		if !mf.Builtin {
			continue
		}
		if mf.Name == legacy.Name {
			legacyMF = mf
		}
		perms := make([]rbac.Permission, 0, len(mf.Permissions))
		for _, p := range mf.Permissions {
			perms = append(perms, rbac.Permission(mf.PermissionName(p.Name)))
		}
		grants := map[string][]rbac.Permission{}
		for role, names := range mf.RoleGrants {
			for _, n := range names {
				grants[role] = append(grants[role], rbac.Permission(mf.PermissionName(n)))
			}
		}
		if err := reg.Add(mf.Name, perms, grants); err != nil {
			t.Fatalf("the core and the built-in modules declare overlapping permissions: %v", err)
		}
	}
	if legacyMF.Name == "" {
		t.Fatal("the legacy module is not registered")
	}
	return reg, legacyMF
}

// Every permission in rbac is declared by exactly one manifest, the core's or
// the legacy module's, under its unchanged name, and every route's permission
// is one of them.
func TestSecurityInvariantEveryPermissionHasAnOwner(t *testing.T) {
	reg, _ := manifestRegistry(t)
	scopes := declaredPermissionScopes(t)
	for ident, scope := range scopes {
		value := strings.TrimPrefix(scope, rbac.ResourceServer+":")
		if _, ok := reg.Owner(rbac.Permission(value)); !ok {
			t.Errorf("rbac.%s (%q) is declared by neither rbac.CorePermissions nor a built-in module", ident, value)
		}
	}
	if got := len(reg.Permissions()); got != len(scopes) {
		t.Errorf("the manifests declare %d permissions, rbac declares %d", got, len(scopes))
	}
	forEachRouteRegistration(t, func(route routeRegistration) {
		for _, ident := range route.permissions {
			if _, ok := scopes[ident]; !ok {
				t.Errorf("%s: permission %s is not an rbac.Permission constant", route, ident)
			}
		}
	})
}

// The predefined roles hold exactly what the core and the legacy module grant.
func TestSecurityInvariantRoleGrantsComeFromManifests(t *testing.T) {
	_, mf := manifestRegistry(t)
	core := rbac.GrantsOf(rbac.CorePermissions)
	for role, want := range rbac.PredefinedRolePermissions {
		var got []rbac.Permission
		got = append(got, core[role]...)
		for _, n := range mf.RoleGrants[role] {
			got = append(got, rbac.Permission(n))
		}
		slices.Sort(got)
		w := slices.Clone(want)
		slices.Sort(w)
		if !slices.Equal(got, w) {
			t.Errorf("role %q: manifests grant %v, PredefinedRolePermissions holds %v", role, got, w)
		}
	}
}
