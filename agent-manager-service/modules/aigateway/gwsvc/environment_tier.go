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

package gwsvc

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// RequireEnvironmentTier authorizes the caller to act on the named environment and
// returns it.
//
// The environment tier is an authorization axis of its own, about *where* an
// action lands rather than what it is: ai-gateway:deploy.non-production is the floor,
// and ai-gateway:deploy.production is held in addition to it to reach the environments
// flagged as production. A production environment requires both — the production
// grant is an extra permission stacked on the floor, not a wider substitute for
// it. That is not a free choice: the REST routes and the two MCP tools declare
// the floor statically and deny before this method is reached, so a rule of
// "production grant alone is sufficient" could never fire and would leave the
// two layers describing the same decision differently.
//
// The check lives here rather than in route middleware because the MCP surface
// declares tool permissions statically and has no
// HTTP request to read a target environment from. This is the one place both
// surfaces pass through, and Runtime.Can reads the same scopes the route or
// tool gate used.
//
// The environment is returned even on a denial, because the caller wants the
// production flag for its audit record and this call has already paid for the
// lookup; it is nil only when the lookup itself failed. The cost is that an
// unresolvable environment fails the operation outright, deliberately.
//
// Nothing here allows on failure. There is deliberately no fallback for an
// installation where no environment carries the flag: every environment is then
// non-production, which is what the model says and what the release notes must
// say too.
func RequireEnvironmentTier(
	ctx context.Context, perms *Permissions, platform modkit.CoreAPI, logger *slog.Logger, ouID, envName string,
) (*modkit.Environment, error) {
	env, err := LookupEnvironment(ctx, platform, ouID, envName)
	if err != nil {
		logger.Error("Failed to resolve environment for the tier check",
			"ouID", ouID, "environment", envName, "error", err)
		return nil, err
	}
	// The floor is always required. Production adds to it rather than replacing
	// it, so the missing scope reported is the first one the caller lacks —
	// naming the production grant to someone who is also missing the floor would
	// send them after the wrong permission.
	required := []rbac.Permission{gwperm.DeployNonProduction}
	if env.IsProduction {
		required = append(required, gwperm.DeployProduction)
	}
	for _, perm := range required {
		if perms.Can(ctx, strings.TrimPrefix(string(perm), gwperm.Module+":")) {
			continue
		}
		// The request's own audit record carries the refusal: the 403 makes
		// its outcome a denial, and these say which scope was missing.
		modkit.AuditDetail(ctx, "reason", "missing-environment-tier-scope")
		modkit.AuditDetail(ctx, "missingScope", perm.Scope())
		return env, fmt.Errorf("%w: %s is required to act on environment %q",
			httpx.ErrForbidden, perm.Scope(), envName)
	}
	return env, nil
}
