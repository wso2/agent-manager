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

// Package legacy registers the features that have not moved into their own
// modules yet (AI Gateway, LLM and MCP proxies, A2A, agent identity) as one
// built-in module. It is always active, owns no tables of its own, and its
// routes and workers are still wired by the core directly, so the service
// behaves as it did before modules existed.
package legacy

import (
	"context"
	"io/fs"
	"slices"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// Name is the module's name.
const Name = "legacy"

type module struct{}

func init() { modkit.Register(module{}) }

func (module) Manifest() modkit.Manifest {
	return modkit.Manifest{
		Name:        Name,
		Version:     "1.0.0",
		Description: "Features not yet split into modules: AI Gateway, LLM and MCP proxies, A2A and agent identity",
		CoreAPI:     modkit.CoreAPIVersion,
		Builtin:     true,
		Permissions: permissions,
		RoleGrants:  roleGrants,
	}
}

// The legacy module declares every permission of rbac that is not the core's,
// under the names it had before modules, and grants them as
// rbac.PredefinedRolePermissions always has. The Admin role holds the whole
// catalog, so it is the list to draw from.
var (
	legacyPerms = func() []rbac.Permission {
		var out []rbac.Permission
		for _, p := range rbac.PredefinedRolePermissions[rbac.RoleAdmin] {
			if !slices.Contains(rbac.CorePermissions, p) {
				out = append(out, p)
			}
		}
		return out
	}()
	permissions = func() []modkit.Permission {
		out := make([]modkit.Permission, 0, len(legacyPerms))
		for _, p := range legacyPerms {
			out = append(out, modkit.Permission{Name: string(p)})
		}
		return out
	}()
	roleGrants = func() map[string][]string {
		out := map[string][]string{}
		for role, ps := range rbac.GrantsOf(legacyPerms) {
			for _, p := range ps {
				out[role] = append(out[role], string(p))
			}
		}
		return out
	}()
)

func (module) Migrations() fs.FS { return nil }
func (module) UI() fs.FS         { return nil }

func (module) Activate(context.Context, modkit.Runtime) error { return nil }
