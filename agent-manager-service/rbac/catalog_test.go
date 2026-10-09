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

// The catalog is a wall of `const ... Permission = "..."` declarations with no
// enumerable slice, deliberately: a slice would be one more copy to drift. So
// the invariants below read the declarations out of the source they guard, which
// cannot fall out of step with them.
package rbac

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// declaredPermissions returns every Permission constant declared in
// permissions.go, keyed by Go identifier. Parsing the source is what makes this
// the catalog rather than a second copy of it.
func declaredPermissions(t *testing.T) map[string]Permission {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "permissions.go", nil, 0)
	if err != nil {
		t.Fatalf("parse permissions.go: %v", err)
	}
	out := make(map[string]Permission)
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			ident, ok := vs.Type.(*ast.Ident)
			if !ok || ident.Name != "Permission" {
				continue
			}
			for i, name := range vs.Names {
				lit, ok := vs.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					t.Fatalf("%s is not declared with a string literal", name.Name)
				}
				out[name.Name] = Permission(lit.Value[1 : len(lit.Value)-1])
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no Permission constants found; the parser stopped matching the source")
	}
	return out
}

// TestEnvironmentTierScopesExist pins the two scopes the environment-tier axis
// is built on, and the shape of the strings Thunder composes them from.
func TestEnvironmentTierScopesExist(t *testing.T) {
	if got, want := AgentEnvNonProduction.Scope(), "amp:agent:env-non-production"; got != want {
		t.Errorf("AgentEnvNonProduction.Scope() = %q, want %q", got, want)
	}
	if got, want := AgentEnvProduction.Scope(), "amp:agent:env-production"; got != want {
		t.Errorf("AgentEnvProduction.Scope() = %q, want %q", got, want)
	}
}

// TestCatalogScopeStringsAreUnique catches the copy-paste that gives two
// constants the same scope. Thunder would accept it and one of the two would
// silently grant the other's permission.
func TestCatalogScopeStringsAreUnique(t *testing.T) {
	seen := make(map[Permission]string)
	for name, perm := range declaredPermissions(t) {
		if first, dup := seen[perm]; dup {
			t.Errorf("scope %q is declared twice: %s and %s", perm, first, name)
			continue
		}
		seen[perm] = name
	}
}

// TestAdminHoldsEntireCatalog is the invariant that makes every other role a
// subset question. A scope missing from Admin is a scope nobody can be granted
// through a predefined role.
func TestAdminHoldsEntireCatalog(t *testing.T) {
	held := make(map[Permission]bool, len(PredefinedRolePermissions[RoleAdmin]))
	for _, perm := range PredefinedRolePermissions[RoleAdmin] {
		held[perm] = true
	}
	for name, perm := range declaredPermissions(t) {
		if !held[perm] {
			t.Errorf("%s (%q) is in the catalog but not held by %s", name, perm, RoleAdmin)
		}
	}
}

// catalogScopes returns the catalog as a set, for the parity checks that compare
// it against a role, a chart document or an allowlist.
func catalogScopes(t *testing.T) map[Permission]bool {
	t.Helper()
	out := make(map[Permission]bool)
	for _, perm := range declaredPermissions(t) {
		out[perm] = true
	}
	return out
}

// TestPredefinedRolesHoldOnlyCatalogScopes catches a role granting a scope that
// no longer exists — the state a removed constant leaves behind, and the one the
// Thunder resource-server tree silently drops on import.
func TestPredefinedRolesHoldOnlyCatalogScopes(t *testing.T) {
	catalog := catalogScopes(t)
	for role, perms := range PredefinedRolePermissions {
		seen := make(map[Permission]bool, len(perms))
		for _, perm := range perms {
			if !catalog[perm] {
				t.Errorf("role %q holds %q, which is not in the catalog", role, perm)
			}
			if seen[perm] {
				t.Errorf("role %q holds %q twice", role, perm)
			}
			seen[perm] = true
		}
	}
}

// TestPredefinedRoleSizes pins the size of each role.
//
// A count is a blunt assertion, and that is the point: these numbers are the
// product decision recorded in the design doc, and a role that silently gains or
// loses a scope is exactly the drift this file exists to catch. Changing a number
// here should mean someone decided to.
//
// It is deliberately a speed bump rather than a safety net, and mostly redundant
// once chart_parity_test.go compares the Go map and the chart set-for-set and
// TestAdminHoldsEntireCatalog pins the catalog: between them, the only thing
// these four numbers still catch on their own is a scope dropped from both sides
// at once. The cost is four numbers to hand-edit on every future scope addition.
// Keep it for the friction; do not mistake it for coverage.
func TestPredefinedRoleSizes(t *testing.T) {
	want := map[string]int{
		RoleAdmin:            105,
		RoleDeveloper:        56,
		RoleAILead:           51,
		RolePlatformEngineer: 69,
	}
	if len(PredefinedRolePermissions) != len(want) {
		t.Fatalf("PredefinedRolePermissions has %d roles, want %d", len(PredefinedRolePermissions), len(want))
	}
	for role, size := range want {
		got, ok := PredefinedRolePermissions[role]
		if !ok {
			t.Errorf("role %q is missing", role)
			continue
		}
		if len(got) != size {
			t.Errorf("role %q holds %d scopes, want %d", role, len(got), size)
		}
	}
}

// TestEveryTierGrantSitsAboveTheFloor pins the shape of the axis: the production
// grant sits on top of the floor rather than replacing it, so a role holding it
// without the floor could act on nothing at all — every surface denies a token
// missing the floor before the tier is evaluated, and requireEnvTier requires
// both for a production environment. No predefined role should be in that state.
func TestEveryTierGrantSitsAboveTheFloor(t *testing.T) {
	for role, perms := range PredefinedRolePermissions {
		var floor, production bool
		for _, perm := range perms {
			switch perm {
			case AgentEnvNonProduction:
				floor = true
			case AgentEnvProduction:
				production = true
			}
		}
		if production && !floor {
			t.Errorf("role %q holds %s without %s", role, AgentEnvProduction, AgentEnvNonProduction)
		}
	}
}
