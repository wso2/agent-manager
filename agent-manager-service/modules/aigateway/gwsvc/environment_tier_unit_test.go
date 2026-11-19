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
package gwsvc

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

const (
	tierProdEnv = "production"
	tierDevEnv  = "development"
)

// tierService returns a service whose only live dependency is a platform
// repository that reports envName's production flag, plus the calls counter so a
// test can assert the lookup happened.
func tierService(envs map[string]bool, lookupErr error) (*tierChecker, *int) {
	calls := 0
	return &tierChecker{
		platform: &gwtest.Core{
			GetEnvironmentFunc: func(_ context.Context, _, envName string) (*modkit.Environment, error) {
				calls++
				if lookupErr != nil {
					return nil, lookupErr
				}
				isProduction, known := envs[envName]
				if !known {
					return nil, gwerr.ErrEnvironmentNotFound
				}
				return &modkit.Environment{Name: envName, IsProduction: isProduction}, nil
			},
		},
		logger: discardLogger(),
	}, &calls
}

// TestRequireEnvTier_ProductionEnvNeedsProductionScope is the point of the whole
// change: the floor is not enough for a production environment.
func TestRequireEnvTier_ProductionEnvNeedsProductionScope(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierProdEnv: true}, nil)

	env, err := svc.requireEnvTier(tierCtx(t, gwperm.DeployNonProduction), tierOUID, tierProdEnv)
	require.ErrorIs(t, err, httpx.ErrForbidden)
	require.NotNil(t, env, "the environment is returned even on a denial, so the caller can record it")
	require.True(t, env.IsProduction)
	require.Contains(t, err.Error(), gwperm.DeployProduction.Scope(),
		"the error must name the scope the caller is missing")
}

// TestRequireEnvTier_ProductionEnvAllowedWithBothScopes is the same environment
// with the grant that reaches it.
func TestRequireEnvTier_ProductionEnvAllowedWithBothScopes(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierProdEnv: true}, nil)

	env, err := svc.requireEnvTier(
		tierCtx(t, gwperm.DeployNonProduction, gwperm.DeployProduction), tierOUID, tierProdEnv,
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

	_, err := svc.requireEnvTier(tierCtx(t, gwperm.DeployProduction), tierOUID, tierProdEnv)
	require.ErrorIs(t, err, httpx.ErrForbidden)
	require.Contains(t, err.Error(), gwperm.DeployNonProduction.Scope(),
		"the missing floor must be the scope the error names")
}

// TestRequireEnvTier_NonProductionEnvAllowedWithFloor covers the ordinary case.
func TestRequireEnvTier_NonProductionEnvAllowedWithFloor(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierDevEnv: false}, nil)

	env, err := svc.requireEnvTier(tierCtx(t, gwperm.DeployNonProduction), tierOUID, tierDevEnv)
	require.NoError(t, err)
	require.False(t, env.IsProduction)
}

// TestRequireEnvTier_NonProductionEnvDeniedWithoutFloor proves env-production is
// not a superset: a caller holding only the production grant does not get the
// floor by implication. No predefined role is in that state, but a custom role
// can be.
func TestRequireEnvTier_NonProductionEnvDeniedWithoutFloor(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierDevEnv: false}, nil)

	_, err := svc.requireEnvTier(tierCtx(t, gwperm.DeployProduction), tierOUID, tierDevEnv)
	require.ErrorIs(t, err, httpx.ErrForbidden)
	require.Contains(t, err.Error(), gwperm.DeployNonProduction.Scope())
}

// TestRequireEnvTier_NoScopesDenies is the unscoped-token case.
func TestRequireEnvTier_NoScopesDenies(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierDevEnv: false}, nil)

	_, err := svc.requireEnvTier(tierCtx(t), tierOUID, tierDevEnv)
	require.ErrorIs(t, err, httpx.ErrForbidden)
}

// TestRequireEnvTier_UnknownEnvIsNotFound keeps the 404 the controller already
// maps, rather than reporting a missing environment as a permission problem.
func TestRequireEnvTier_UnknownEnvIsNotFound(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierDevEnv: false}, nil)

	_, err := svc.requireEnvTier(tierCtx(t, gwperm.DeployNonProduction), tierOUID, "nope")
	require.ErrorIs(t, err, gwerr.ErrEnvironmentNotFound)
}

// TestRequireEnvTier_LookupFailureDenies is the fail-closed guarantee: an
// unreachable OpenChoreo must not become an allow.
func TestRequireEnvTier_LookupFailureDenies(t *testing.T) {
	boom := errors.New("openchoreo unreachable")
	svc, _ := tierService(nil, boom)

	_, err := svc.requireEnvTier(tierCtx(t, gwperm.DeployProduction), tierOUID, tierProdEnv)
	require.ErrorIs(t, err, boom)
}

// TestRequireEnvTier_ReportsTierOnEveryCall keeps the environment lookup on the
// path regardless of the outcome of the scope check: the production flag is what
// the deploy and promote records carry, and losing it would cost the audit trail
// the difference between a production change and a sandbox one.
func TestRequireEnvTier_ReportsTierOnEveryCall(t *testing.T) {
	svc, calls := tierService(map[string]bool{tierProdEnv: true}, nil)

	env, err := svc.requireEnvTier(
		tierCtx(t, gwperm.DeployNonProduction, gwperm.DeployProduction), tierOUID, tierProdEnv,
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
		tierCtx(t, gwperm.DeployNonProduction, gwperm.DeployProduction), tierOUID, tierProdEnv,
	)
	require.ErrorIs(t, err, boom)
	require.Nil(t, env, "a lookup failure is the one case that yields no environment")
}

// TestRequireEnvTier_DenialIsAudited makes the refusal legible to whoever is
// alerting on authorization failures: the request's own record, whose
// outcome the 403 makes a denial, says which scope was missing.
func TestRequireEnvTier_DenialIsAudited(t *testing.T) {
	svc, _ := tierService(map[string]bool{tierProdEnv: true}, nil)
	ctx, record := scopedCtx(tierCtx(t, gwperm.DeployNonProduction))

	_, err := svc.requireEnvTier(ctx, tierOUID, tierProdEnv)
	require.ErrorIs(t, err, httpx.ErrForbidden)

	event := record("ai-gateway:deploy.non-production")
	require.Equal(t, "missing-environment-tier-scope", event.Details["reason"])
	require.Equal(t, gwperm.DeployProduction.Scope(), event.Details["missingScope"])
}

// tierChecker calls RequireEnvironmentTier on a fixed repository.
type tierChecker struct {
	platform modkit.CoreAPI
	logger   *slog.Logger
}

func (c *tierChecker) requireEnvTier(ctx context.Context, ouID, envName string) (*modkit.Environment, error) {
	return RequireEnvironmentTier(ctx, tierPermissions(), c.platform, c.logger, ouID, envName)
}
