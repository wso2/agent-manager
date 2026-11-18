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

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
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
// declares tool permissions statically (mcp/tools/authz.go addTool) and has no
// HTTP request to read a target environment from. This is the one place both
// surfaces pass through, and mcp/tools/authz.go's withEffectiveScopes is what
// guarantees ctx carries the same scopes the tool gate used.
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
	ctx context.Context, platform repositories.PlatformRepository, logger *slog.Logger, ouID, envName string,
) (*models.EnvironmentResponse, error) {
	env, err := platform.GetEnvironment(ctx, ouID, envName)
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
	if perm, short := jwtassertion.FirstMissingScope(ctx, required...); short {
		// A middleware denial reaches the trail through recordAuthzDeny, which
		// uses audit.Record plus audit.Skip: the deny event *replaces* the
		// envelope, because the request never reached a handler. This denial is
		// different — the caller did reach the service layer, and on the
		// change-deployment-state path the controller has already opened an
		// attempt. RecordAncillary is therefore correct here (audit/emit.go:63-71:
		// facts about how a request was handled, envelope preserved), and the
		// consequence is deliberate: a tier refusal writes an authz:deny
		// alongside the envelope rather than in place of it, so "what was
		// attempted" survives. Status is left to the envelope for the same
		// reason. grantedScopes is set because every middleware authz:deny
		// carries it and anything alerting on the action reads it.
		audit.RecordAncillary(
			ctx, audit.ActionAuthzDeny,
			audit.Org(ouID),
			audit.Environment(envName),
			audit.OutcomeOpt(audit.OutcomeDeny),
			audit.RequiredPermissions(required...),
			audit.Detail("reason", "missing-environment-tier-scope"),
			audit.Detail("missingScope", perm.Scope()),
			audit.Detail("grantedScopes", jwtassertion.GrantedScopeCount(ctx)),
		)
		return env, fmt.Errorf("%w: %s is required to act on environment %q",
			httpx.ErrForbidden, perm.Scope(), envName)
	}
	return env, nil
}
