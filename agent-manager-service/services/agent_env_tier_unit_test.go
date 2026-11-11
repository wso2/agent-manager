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

// UNIT tests for the environment-tier authorization axis.
//
// The tier is checked here rather than in route middleware because the MCP
// surface declares tool permissions statically and has no request body to read a
// target environment from. This is the single implementation both surfaces share,
// so these tests are the whole contract for "may this caller act on this
// environment".
package services

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	tierOUID    = "ou-tier"
	tierProdEnv = "production"
	tierDevEnv  = "development"
)

// tierService returns a service whose only live dependency is a platform
// repository that reports envName's production flag, plus the calls counter so a
// test can assert the lookup happened.
func tierService(envs map[string]bool, lookupErr error) (*agentManagerService, *int) {
	calls := 0
	return &agentManagerService{
		platform: &repomocks.PlatformRepositoryMock{
			GetEnvironmentFunc: func(_ context.Context, _, envName string) (*models.EnvironmentResponse, error) {
				calls++
				if lookupErr != nil {
					return nil, lookupErr
				}
				isProduction, known := envs[envName]
				if !known {
					return nil, utils.ErrEnvironmentNotFound
				}
				return &models.EnvironmentResponse{Name: envName, IsProduction: isProduction}, nil
			},
		},
		logger: discardLogger(),
	}, &calls
}

// tierCtx returns an auditable context carrying a token with exactly the given
// scopes.
func tierCtx(t *testing.T, scopes ...rbac.Permission) context.Context {
	t.Helper()
	return jwtassertion.ContextWithTokenClaimsAndScope(auditableCtx(t),
		&jwtassertion.TokenClaims{OuId: tierOUID, Scope: audit.ScopesOf(scopes)})
}

// tierGrantedCtx is the caller the deploy and promote fixtures assume: it holds
// both environment tiers, so requireEnvTier passes and the test reaches whatever
// it is actually about. Tests about the tier check itself use tierCtx.
func tierGrantedCtx(t *testing.T) context.Context {
	t.Helper()
	return tierGrantedOn(auditableCtx(t))
}

// tierGrantedOn grants both environment tiers on top of base.
func tierGrantedOn(base context.Context) context.Context {
	return jwtassertion.ContextWithTokenClaimsAndScope(base, &jwtassertion.TokenClaims{
		Scope: audit.ScopesOf([]rbac.Permission{rbac.AgentEnvNonProduction, rbac.AgentEnvProduction}),
	})
}

// TestRequireEnvTier_ProductionEnvNeedsProductionScope is the point of the whole
// change: the floor is not enough for a production environment.
func TestRequireEnvTier_ProductionEnvNeedsProductionScope(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierProdEnv: true}, nil)

	env, err := svc.requireEnvTier(tierCtx(t, rbac.AgentEnvNonProduction), tierOUID, tierProdEnv)
	require.ErrorIs(t, err, utils.ErrForbidden)
	require.NotNil(t, env, "the environment is returned even on a denial, so the caller can record it")
	require.True(t, env.IsProduction)
	require.Contains(t, err.Error(), rbac.AgentEnvProduction.Scope(),
		"the error must name the scope the caller is missing")
}

// TestRequireEnvTier_ProductionEnvAllowedWithBothScopes is the same environment
// with the grant that reaches it.
func TestRequireEnvTier_ProductionEnvAllowedWithBothScopes(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierProdEnv: true}, nil)

	env, err := svc.requireEnvTier(
		tierCtx(t, rbac.AgentEnvNonProduction, rbac.AgentEnvProduction), tierOUID, tierProdEnv,
	)
	require.NoError(t, err)
	require.True(t, env.IsProduction)
}

// TestRequireEnvTier_ProductionScopeAloneIsNotEnough pins the half of the rule
// that is easy to get wrong. env-production sits on top of the floor rather than
// replacing it, and this is the layer that has to agree with the static
// declaration the routes and MCP tools already make: both deny a token without
// the floor before this method is ever reached, so accepting the production
// grant alone here would be an unreachable rule that reads as a second, laxer
// model of the same decision.
func TestRequireEnvTier_ProductionScopeAloneIsNotEnough(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierProdEnv: true}, nil)

	_, err := svc.requireEnvTier(tierCtx(t, rbac.AgentEnvProduction), tierOUID, tierProdEnv)
	require.ErrorIs(t, err, utils.ErrForbidden)
	require.Contains(t, err.Error(), rbac.AgentEnvNonProduction.Scope(),
		"the missing floor must be the scope the error names")
}

// TestRequireEnvTier_NonProductionEnvAllowedWithFloor covers the ordinary case.
func TestRequireEnvTier_NonProductionEnvAllowedWithFloor(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierDevEnv: false}, nil)

	env, err := svc.requireEnvTier(tierCtx(t, rbac.AgentEnvNonProduction), tierOUID, tierDevEnv)
	require.NoError(t, err)
	require.False(t, env.IsProduction)
}

// TestRequireEnvTier_NonProductionEnvDeniedWithoutFloor proves env-production is
// not a superset: a caller holding only the production grant does not get the
// floor by implication. No predefined role is in that state, but a custom role
// can be.
func TestRequireEnvTier_NonProductionEnvDeniedWithoutFloor(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierDevEnv: false}, nil)

	_, err := svc.requireEnvTier(tierCtx(t, rbac.AgentEnvProduction), tierOUID, tierDevEnv)
	require.ErrorIs(t, err, utils.ErrForbidden)
	require.Contains(t, err.Error(), rbac.AgentEnvNonProduction.Scope())
}

// TestRequireEnvTier_NoScopesDenies is the unscoped-token case.
func TestRequireEnvTier_NoScopesDenies(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierDevEnv: false}, nil)

	_, err := svc.requireEnvTier(tierCtx(t), tierOUID, tierDevEnv)
	require.ErrorIs(t, err, utils.ErrForbidden)
}

// TestRequireEnvTier_UnknownEnvIsNotFound keeps the 404 the controller already
// maps, rather than reporting a missing environment as a permission problem.
func TestRequireEnvTier_UnknownEnvIsNotFound(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierDevEnv: false}, nil)

	_, err := svc.requireEnvTier(tierCtx(t, rbac.AgentEnvNonProduction), tierOUID, "nope")
	require.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
}

// TestRequireEnvTier_LookupFailureDenies is the fail-closed guarantee: an
// unreachable OpenChoreo must not become an allow.
func TestRequireEnvTier_LookupFailureDenies(t *testing.T) {
	boom := errors.New("openchoreo unreachable")
	svc, _ := tierService(nil, boom)

	_, err := svc.requireEnvTier(tierCtx(t, rbac.AgentEnvProduction), tierOUID, tierProdEnv)
	require.ErrorIs(t, err, boom)
}

// TestRequireEnvTier_ReportsTierOnEveryCall keeps the environment lookup on the
// path regardless of the outcome of the scope check: the production flag is what
// the deploy and promote records carry, and losing it would cost the audit trail
// the difference between a production change and a sandbox one.
func TestRequireEnvTier_ReportsTierOnEveryCall(t *testing.T) {
	svc, calls := tierService(map[string]bool{tierProdEnv: true}, nil)

	env, err := svc.requireEnvTier(
		tierCtx(t, rbac.AgentEnvNonProduction, rbac.AgentEnvProduction), tierOUID, tierProdEnv,
	)
	require.NoError(t, err)
	require.True(t, env.IsProduction)
	require.Equal(t, 1, *calls, "the environment lookup must run on every tier check")
}

// TestRequireEnvTier_LookupErrorFailsClosedBeforeTheScopeCheck pins the sharpest
// edge of this change. The lookup runs before the scopes are read, so an
// unresolvable environment fails the operation even for a caller holding every
// tier scope. That is a deliberate trade: the alternative is a deploy that
// proceeds without knowing where it lands, which is what DeployAgent did before
// (a warning, then a degraded deploy with no config, OAuth issuer or trait work).
// Task 5 Step 3 records this as user-visible and it belongs in the release notes.
func TestRequireEnvTier_LookupErrorFailsClosedBeforeTheScopeCheck(t *testing.T) {
	boom := errors.New("openchoreo unreachable")
	svc, _ := tierService(nil, boom)

	env, err := svc.requireEnvTier(
		tierCtx(t, rbac.AgentEnvNonProduction, rbac.AgentEnvProduction), tierOUID, tierProdEnv,
	)
	require.ErrorIs(t, err, boom)
	require.Nil(t, env, "a lookup failure is the one case that yields no environment")
}

// TestRequireEnvTier_DenialIsAudited makes the refusal legible to whoever is
// alerting on authorization failures. A service-layer denial reaches the trail by
// a different route than a middleware one, so it needs its own assertion that it
// arrives at all.
func TestRequireEnvTier_DenialIsAudited(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierProdEnv: true}, nil)
	ctx, sink, flush := capturingAuditCtx(t)
	ctx = jwtassertion.ContextWithTokenClaimsAndScope(ctx,
		&jwtassertion.TokenClaims{OuId: tierOUID, Scope: rbac.AgentEnvNonProduction.Scope()})

	_, err := svc.requireEnvTier(ctx, tierOUID, tierProdEnv)
	require.ErrorIs(t, err, utils.ErrForbidden)
	flush()

	event, found := findEvent(sink.captured(), audit.ActionAuthzDeny)
	require.True(t, found, "the tier denial was not recorded")
	require.Equal(t, tierProdEnv, event.Environment)
	require.Equal(t, audit.OutcomeDeny, event.Outcome)
	require.Equal(t, rbac.AgentEnvProduction.Scope(), event.Details["missingScope"])
	// grantedScopes is on every middleware authz:deny. Anything alerting on the
	// action reads the field, so a tier denial that omitted it would look like a
	// token with no scopes at all rather than one missing this scope.
	require.EqualValues(t, 1, event.Details["grantedScopes"])
}
