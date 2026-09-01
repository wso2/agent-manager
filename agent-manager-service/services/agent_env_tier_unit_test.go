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
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	tierOUID    = "ou-tier"
	tierProdEnv = "production"
	tierDevEnv  = "development"
)

// setRBACEnabledForTier flips the process-global RBAC switch for one test and
// restores it on cleanup. Tests using it must not run in parallel.
func setRBACEnabledForTier(t *testing.T, enabled bool) {
	t.Helper()
	cfg := config.GetConfig()
	orig := cfg.RBACEnabled
	cfg.RBACEnabled = enabled
	t.Cleanup(func() { cfg.RBACEnabled = orig })
}

// nonProductionEnvStub is the GetEnvironment stub the deploy and promote
// fixtures need now that both paths resolve their target through requireEnvTier:
// an ordinary non-production environment, under whatever name the path asks for.
// A test that cares about the tier states its own environment instead — see
// tierService below, which is the only place the production flag is interesting.
func nonProductionEnvStub() func(ctx context.Context, ouID, envName string) (*models.EnvironmentResponse, error) {
	return func(_ context.Context, _, envName string) (*models.EnvironmentResponse, error) {
		return &models.EnvironmentResponse{Name: envName}, nil
	}
}

// tierService returns a service whose only live dependency is an OpenChoreo
// client that reports envName's production flag, plus the calls counter so a
// test can assert the lookup happened.
func tierService(envs map[string]bool, lookupErr error) (*agentManagerService, *int) {
	calls := 0
	return &agentManagerService{
		ocClient: &clientmocks.OpenChoreoClientMock{
			GetEnvironmentFunc: func(_ context.Context, _, envName string) (*models.EnvironmentResponse, error) {
				calls++
				if lookupErr != nil {
					return nil, lookupErr
				}
				isProduction, known := envs[envName]
				if !known {
					return nil, utils.ErrNotFound
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
	scope := ""
	for i, perm := range scopes {
		if i > 0 {
			scope += " "
		}
		scope += perm.Scope()
	}
	return jwtassertion.ContextWithTokenClaimsAndScope(auditableCtx(t),
		&jwtassertion.TokenClaims{OuId: tierOUID, Scope: scope})
}

// tierGrantedCtx is the caller the deploy and promote fixtures assume: it holds
// both environment tiers, so requireEnvTier passes and the test reaches whatever
// it is actually about. Tests about the tier check itself use tierCtx.
func tierGrantedCtx(t *testing.T) context.Context {
	t.Helper()
	return jwtassertion.ContextWithTokenClaimsAndScope(auditableCtx(t), &jwtassertion.TokenClaims{
		Scope: audit.ScopesOf([]rbac.Permission{rbac.AgentEnvNonProduction, rbac.AgentEnvProduction}),
	})
}

// TestRequireEnvTier_ProductionEnvNeedsProductionScope is the point of the whole
// change: the floor is not enough for a production environment.
func TestRequireEnvTier_ProductionEnvNeedsProductionScope(t *testing.T) {
	setRBACEnabledForTier(t, true)
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
	setRBACEnabledForTier(t, true)
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
	setRBACEnabledForTier(t, true)
	svc, _ := tierService(map[string]bool{tierProdEnv: true}, nil)

	_, err := svc.requireEnvTier(tierCtx(t, rbac.AgentEnvProduction), tierOUID, tierProdEnv)
	require.ErrorIs(t, err, utils.ErrForbidden)
	require.Contains(t, err.Error(), rbac.AgentEnvNonProduction.Scope(),
		"the missing floor must be the scope the error names")
}

// TestRequireEnvTier_NonProductionEnvAllowedWithFloor covers the ordinary case.
func TestRequireEnvTier_NonProductionEnvAllowedWithFloor(t *testing.T) {
	setRBACEnabledForTier(t, true)
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
	setRBACEnabledForTier(t, true)
	svc, _ := tierService(map[string]bool{tierDevEnv: false}, nil)

	_, err := svc.requireEnvTier(tierCtx(t, rbac.AgentEnvProduction), tierOUID, tierDevEnv)
	require.ErrorIs(t, err, utils.ErrForbidden)
	require.Contains(t, err.Error(), rbac.AgentEnvNonProduction.Scope())
}

// TestRequireEnvTier_NoScopesDenies is the unscoped-token case.
func TestRequireEnvTier_NoScopesDenies(t *testing.T) {
	setRBACEnabledForTier(t, true)
	svc, _ := tierService(map[string]bool{tierDevEnv: false}, nil)

	_, err := svc.requireEnvTier(tierCtx(t), tierOUID, tierDevEnv)
	require.ErrorIs(t, err, utils.ErrForbidden)
}

// TestRequireEnvTier_UnknownEnvIsNotFound keeps the 404 the controller already
// maps, rather than reporting a missing environment as a permission problem.
func TestRequireEnvTier_UnknownEnvIsNotFound(t *testing.T) {
	setRBACEnabledForTier(t, true)
	svc, _ := tierService(map[string]bool{tierDevEnv: false}, nil)

	_, err := svc.requireEnvTier(tierCtx(t, rbac.AgentEnvNonProduction), tierOUID, "nope")
	require.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
}

// TestRequireEnvTier_LookupFailureDenies is the fail-closed guarantee: an
// unreachable OpenChoreo must not become an allow.
func TestRequireEnvTier_LookupFailureDenies(t *testing.T) {
	setRBACEnabledForTier(t, true)
	boom := errors.New("openchoreo unreachable")
	svc, _ := tierService(nil, boom)

	_, err := svc.requireEnvTier(tierCtx(t, rbac.AgentEnvProduction), tierOUID, tierProdEnv)
	require.ErrorIs(t, err, boom)
}

// TestRequireEnvTier_RBACDisabledSkipsCheckButStillReportsTier mirrors
// middleware.RequirePermission's rollout switch. The lookup still runs: the
// production flag is what the deploy and promote records carry, and an
// RBAC-disabled install should not lose it from its audit trail.
func TestRequireEnvTier_RBACDisabledSkipsCheckButStillReportsTier(t *testing.T) {
	setRBACEnabledForTier(t, false)
	svc, calls := tierService(map[string]bool{tierProdEnv: true}, nil)

	env, err := svc.requireEnvTier(tierCtx(t), tierOUID, tierProdEnv)
	require.NoError(t, err)
	require.True(t, env.IsProduction)
	require.Equal(t, 1, *calls, "the environment lookup must still run with RBAC disabled")
}

// TestRequireEnvTier_RBACDisabledStillFailsClosedOnLookupError pins the sharpest
// edge of this change. The lookup runs before the RBAC switch is consulted, so an
// unresolvable environment fails the operation even on an install that enforces
// no scopes at all. That is a deliberate trade: the alternative is a deploy that
// proceeds without knowing where it lands, which is what DeployAgent did before
// (a warning, then a degraded deploy with no config, OAuth issuer or trait work).
// Task 5 Step 3 records this as user-visible and it belongs in the release notes.
func TestRequireEnvTier_RBACDisabledStillFailsClosedOnLookupError(t *testing.T) {
	setRBACEnabledForTier(t, false)
	boom := errors.New("openchoreo unreachable")
	svc, _ := tierService(nil, boom)

	env, err := svc.requireEnvTier(tierCtx(t), tierOUID, tierProdEnv)
	require.ErrorIs(t, err, boom)
	require.Nil(t, env, "a lookup failure is the one case that yields no environment")
}

// TestRequireEnvTier_DenialIsAudited makes the refusal legible to whoever is
// alerting on authorization failures. A service-layer denial reaches the trail by
// a different route than a middleware one, so it needs its own assertion that it
// arrives at all.
func TestRequireEnvTier_DenialIsAudited(t *testing.T) {
	setRBACEnabledForTier(t, true)
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

// The config-update paths reach an environment as directly as a deploy does:
// both rewrite what a running deployment executes with. UpdateAgentConfigurations
// replaces the environment's entire env var and file mount set, and
// UpdateAgentDeploySettings rewrites its trait configs, so a caller who may not
// deploy to production must not be able to edit production either. These two
// tests are that guarantee — the route's static floor is only half of it, since
// the environment arrives in the request body.
func TestUpdateAgentConfigurations_ProductionNeedsProductionScope(t *testing.T) {
	setRBACEnabledForTier(t, true)
	overridesReplaced := false
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Provisioning: models.Provisioning{Type: string(utils.InternalAgent)}}, nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{Name: name, IsProduction: true}, nil
		},
		EnsureReleaseAndBindingFunc: func(context.Context, string, string, string, string, []client.EnvVar, []client.FileVar) error {
			overridesReplaced = true
			return nil
		},
	}
	svc := &agentManagerService{ocClient: ocClient, logger: discardLogger()}

	err := svc.UpdateAgentConfigurations(
		tierCtx(t, rbac.AgentEnvNonProduction, rbac.AgentUpdate), tierOUID, "proj1", "my-agent",
		&spec.UpdateAgentConfigurationsRequest{EnvironmentName: tierProdEnv},
	)

	require.ErrorIs(t, err, utils.ErrForbidden)
	require.Contains(t, err.Error(), rbac.AgentEnvProduction.Scope())
	require.False(t, overridesReplaced,
		"a caller without the production grant must not rewrite a production deployment's env vars, secret refs or file mounts")
}

func TestUpdateAgentDeploySettings_ProductionNeedsProductionScope(t *testing.T) {
	setRBACEnabledForTier(t, true)
	traitConfigsUpdated := false
	ocClient := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(_ context.Context, _, _, _ string) (*models.AgentResponse, error) {
			return &models.AgentResponse{Type: models.AgentType{Type: string(utils.AgentTypeAPI)}}, nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{Name: name, IsProduction: true}, nil
		},
		UpdateReleaseBindingTraitConfigsFunc: func(context.Context, string, string, string, map[string]interface{}, map[string]interface{}) error {
			traitConfigsUpdated = true
			return nil
		},
	}
	svc := &agentManagerService{ocClient: ocClient, logger: discardLogger()}

	err := svc.UpdateAgentDeploySettings(
		tierCtx(t, rbac.AgentEnvNonProduction, rbac.AgentUpdate), tierOUID, "proj1", "my-agent",
		&spec.UpdateAgentDeploySettingsRequest{EnvironmentName: tierProdEnv},
	)

	require.ErrorIs(t, err, utils.ErrForbidden)
	require.Contains(t, err.Error(), rbac.AgentEnvProduction.Scope())
	require.False(t, traitConfigsUpdated,
		"a caller without the production grant must not rewrite a production deployment's trait configs")
}
