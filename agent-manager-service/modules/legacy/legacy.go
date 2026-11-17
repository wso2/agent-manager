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

// Package legacy registers, as one built-in module, the permissions that no
// module owns yet (LLM and MCP proxies, agent identities, evaluators,
// monitors, hosted agents) and the routes of features this version does not
// serve, which answer 501, plus the evaluator catalog. It is always active and
// owns no tables of its own. The AI Gateway's code is the gateway module's;
// the permissions of its LLM and MCP features stay here, under their scope
// names, until that module takes them over.
package legacy

import (
	"context"
	"io/fs"
	"slices"

	"github.com/wso2/agent-manager/agent-manager-service/builtin"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/legacy/evaluators"
	"github.com/wso2/agent-manager/agent-manager-service/modules/legacy/evaluators/evalrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/legacy/legacyapi"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// Name is the module's name.
const Name = "legacy"

type module struct{}

func init() {
	modkit.Register(module{})
	builtin.Register(builtin.Bundle{Name: Name, Build: build})
}

// build wires the evaluator catalog and returns the legacy routes.
func build(_ context.Context, core *wiring.AppParams) (*builtin.Parts, error) {
	evaluatorService := evaluators.NewEvaluatorManagerService(core.Logger,
		evalrepo.NewCustomEvaluatorRepo(core.DB), evalrepo.NewMonitorRepo(core.DB))
	return Parts(evaluators.NewEvaluatorController(evaluatorService)), nil
}

// Parts is what the legacy module adds to the server: its routes.
func Parts(evaluatorCtrl evaluators.EvaluatorController) *builtin.Parts {
	return &builtin.Parts{
		Routes: func(rr *middleware.RouteRegistrar) { legacyapi.RegisterRoutes(rr, evaluatorCtrl) },
	}
}

func (module) Manifest() modkit.Manifest {
	return modkit.Manifest{
		Name:        Name,
		Version:     "1.0.0",
		Description: "Permissions no module owns yet, and the routes of features this version does not serve",
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
