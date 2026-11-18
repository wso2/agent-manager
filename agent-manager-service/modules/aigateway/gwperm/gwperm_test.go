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

package gwperm

import (
	"regexp"
	"slices"
	"testing"

	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// TestEnvironmentTierScopes pins the two scopes the environment-tier axis is
// built on, as ThunderID issues them.
func TestEnvironmentTierScopes(t *testing.T) {
	if got, want := DeployNonProduction.Scope(), "amp:ai-gateway:deploy.non-production"; got != want {
		t.Errorf("DeployNonProduction.Scope() = %q, want %q", got, want)
	}
	if got, want := DeployProduction.Scope(), "amp:ai-gateway:deploy.production"; got != want {
		t.Errorf("DeployProduction.Scope() = %q, want %q", got, want)
	}
}

// TestActions checks the module's 42 permissions: <resource>.<action>, unique,
// and every role grant names one of them.
func TestActions(t *testing.T) {
	if len(Actions) != 42 {
		t.Errorf("%d permissions, want 42", len(Actions))
	}
	name := regexp.MustCompile(`^[a-z][a-z0-9-]*\.[a-z][a-z0-9-]*$`)
	var names []string
	for _, a := range Actions {
		if !name.MatchString(a.Name) {
			t.Errorf("permission %q is not <resource>.<action>", a.Name)
		}
		if slices.Contains(names, a.Name) {
			t.Errorf("permission %q is declared twice", a.Name)
		}
		if a.DisplayName == "" || a.Description == "" {
			t.Errorf("permission %q needs a display name and a description", a.Name)
		}
		names = append(names, a.Name)
	}
	for role, granted := range RoleGrants {
		if _, ok := rbac.PredefinedRolePermissions[role]; !ok {
			t.Errorf("%q is not a predefined role", role)
		}
		for _, g := range granted {
			if !slices.Contains(names, g) {
				t.Errorf("%s is granted %q, which is not declared", role, g)
			}
		}
	}
}

// TestEveryTierGrantSitsAboveTheFloor: the production grant sits on top of
// the floor rather than replacing it, so a role holding it without the floor
// could act on nothing at all.
func TestEveryTierGrantSitsAboveTheFloor(t *testing.T) {
	for role, granted := range RoleGrants {
		if slices.Contains(granted, "deploy.production") && !slices.Contains(granted, "deploy.non-production") {
			t.Errorf("role %q holds deploy.production without deploy.non-production", role)
		}
	}
}
