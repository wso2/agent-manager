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

package rbac

import (
	"fmt"
	"slices"
	"sync/atomic"
)

// CoreOwner is the owner of the permissions the core declares.
const CoreOwner = "core"

// CorePermissions are the permissions the core declares. Every other
// permission is declared by a module in its manifest; the built-in legacy
// module declares the rest of this package's constants.
var CorePermissions = []Permission{
	OrgView, OrgModifySettings, OrgInviteMember, OrgRemoveMember,
	OrgAssignRole, OrgManageIDP, OrgManageServiceAccount,
	ProjectCreate, ProjectRead, ProjectUpdate, ProjectDelete,
	EnvironmentCreate, EnvironmentRead, EnvironmentUpdate, EnvironmentDelete,
	GitSecretCreate, GitSecretRead, GitSecretDelete,
	AgentCreate, AgentRead, AgentUpdate, AgentDelete, AgentBuild,
	AgentEnvNonProduction, AgentEnvProduction, AgentRollback, AgentSuspend,
	AgentTokenManage, AgentAPIKeyManage,
	EventsRead, EventsManage,
	ObservabilityTraceRead, ObservabilityLogRead, ObservabilityBuildLogRead, ObservabilityMetricRead,
	RoleCreate, RoleRead, RoleUpdate, RoleDelete,
	GroupCreate, GroupRead, GroupUpdate, GroupDelete,
	ProfileRead, ProfileUpdateAttributes,
}

// GrantsOf is PredefinedRolePermissions narrowed to perms: what the predefined
// roles hold of one owner's permissions.
func GrantsOf(perms []Permission) map[string][]Permission {
	out := map[string][]Permission{}
	for role, held := range PredefinedRolePermissions {
		for _, p := range held {
			if slices.Contains(perms, p) {
				out[role] = append(out[role], p)
			}
		}
	}
	return out
}

// Registry is the set of permissions the core and the active modules declare,
// with who declares each, and what the modules grant the predefined roles.
type Registry struct {
	owner  map[Permission]string
	grants map[string][]Permission
}

// NewRegistry returns a registry holding the core's permissions.
func NewRegistry() *Registry {
	r := &Registry{owner: map[Permission]string{}, grants: map[string][]Permission{}}
	for _, p := range CorePermissions {
		r.owner[p] = CoreOwner
	}
	return r
}

// Add declares owner's permissions and its grants to the predefined roles.
// A permission may have one owner, and a grant must name a declared permission
// of the same owner and a predefined role.
func (r *Registry) Add(owner string, perms []Permission, grants map[string][]Permission) error {
	for _, p := range perms {
		if by, ok := r.owner[p]; ok {
			return fmt.Errorf("permission %s is declared by %s and %s", p, by, owner)
		}
	}
	for role, ps := range grants {
		if _, ok := PredefinedRolePermissions[role]; !ok {
			return fmt.Errorf("%s grants permissions to %q, which is not a predefined role", owner, role)
		}
		for _, p := range ps {
			if !slices.Contains(perms, p) {
				return fmt.Errorf("%s grants %s, which it does not declare", owner, p)
			}
		}
	}
	for _, p := range perms {
		r.owner[p] = owner
	}
	for role, ps := range grants {
		r.grants[role] = append(r.grants[role], ps...)
	}
	return nil
}

// Clone returns a copy of r that Add can change without changing r.
func (r *Registry) Clone() *Registry {
	c := &Registry{owner: make(map[Permission]string, len(r.owner)), grants: make(map[string][]Permission, len(r.grants))}
	for p, o := range r.owner {
		c.owner[p] = o
	}
	for role, ps := range r.grants {
		c.grants[role] = slices.Clone(ps)
	}
	return c
}

// Owner reports who declares p.
func (r *Registry) Owner(p Permission) (string, bool) {
	o, ok := r.owner[p]
	return o, ok
}

// Permissions lists every declared permission, sorted.
func (r *Registry) Permissions() []Permission {
	out := make([]Permission, 0, len(r.owner))
	for p := range r.owner {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

var current atomic.Pointer[Registry]

func init() { current.Store(NewRegistry()) }

// Current is the registry of the module set this process runs.
func Current() *Registry { return current.Load() }

// SetCurrent installs the registry of a new module set.
func SetCurrent(r *Registry) { current.Store(r) }

// RoleGrants is everything a predefined role holds: PredefinedRolePermissions
// plus what the active modules grant it.
func RoleGrants(role string) []Permission {
	set := map[Permission]struct{}{}
	for _, p := range PredefinedRolePermissions[role] {
		set[p] = struct{}{}
	}
	for _, p := range Current().grants[role] {
		set[p] = struct{}{}
	}
	out := make([]Permission, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}
