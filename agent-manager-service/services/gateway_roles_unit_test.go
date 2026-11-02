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

package services

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// newGateway builds a fully populated Gateway fixture with the given functionality
// role and active status. Every field is set explicitly (exhaustruct covers models.*).
func newGateway(t *testing.T, role string, active bool) *models.Gateway {
	t.Helper()
	id := uuid.New()
	now := time.Now()
	return &models.Gateway{
		UUID:        id,
		OUID:        "org",
		Name:        "gateway-" + id.String(),
		DisplayName: "Gateway " + id.String(),
		Description: "test gateway fixture",
		Properties:  map[string]interface{}{},
		Manifest:    map[string]interface{}{},
		Vhost:       "vhost-" + id.String() + ".example.com",
		// Seeded so internal-URL consumers resolve a real address instead of failing closed.
		RuntimeURL:               "http://gateway-" + id.String() + ".acme-dev:22893",
		IsCritical:               false,
		GatewayFunctionalityType: role,
		IsActive:                 active,
		CreatedAt:                now,
		UpdatedAt:                now,
		DeletedAt:                gorm.DeletedAt{},
	}
}

// gatewayFixtureRepo returns a GatewayRepositoryMock whose ListWithFilters actually
// applies EnvironmentID and FunctionalityTypeIn the way the real SQL does. Returning the
// fixture verbatim would make every no-fallback assertion pass vacuously.
func gatewayFixtureRepo(t *testing.T, envUUID string, gateways []*models.Gateway) *repomocks.GatewayRepositoryMock {
	t.Helper()
	byUUID := map[string]*models.Gateway{}
	for _, gw := range gateways {
		byUUID[gw.UUID.String()] = gw
	}
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(f repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			out := []*models.Gateway{}
			for _, gw := range gateways {
				if f.EnvironmentID != nil && *f.EnvironmentID != envUUID {
					continue
				}
				if f.FunctionalityType != nil && gw.GatewayFunctionalityType != *f.FunctionalityType {
					continue
				}
				if len(f.FunctionalityTypeIn) > 0 && !slices.Contains(f.FunctionalityTypeIn, gw.GatewayFunctionalityType) {
					continue
				}
				if f.Status != nil && gw.IsActive != *f.Status {
					continue
				}
				out = append(out, gw)
			}
			if f.Limit > 0 && len(out) > f.Limit {
				out = out[:f.Limit]
			}
			return out, nil
		},
		GetByUUIDFunc: func(id string) (*models.Gateway, error) {
			gw, ok := byUUID[id]
			if !ok {
				return nil, gorm.ErrRecordNotFound
			}
			return gw, nil
		},
		EnvironmentMappingExistsFunc: func(gatewayID, environmentID string) (bool, error) {
			_, ok := byUUID[gatewayID]
			return ok && environmentID == envUUID, nil
		},
	}
	repo.WithTxFunc = func(*gorm.DB) repositories.GatewayRepository { return repo }
	return repo
}

func TestResolveEgressGatewayForEnvironment(t *testing.T) {
	env := uuid.New()
	both := newGateway(t, models.GatewayRoleBoth, true)
	egress := newGateway(t, models.GatewayRoleEgress, true)
	ingress := newGateway(t, models.GatewayRoleIngress, true)
	inactiveEgress := newGateway(t, models.GatewayRoleEgress, false)

	t.Run("exactly one egress-capable is inferred", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{ingress, egress})
		got, err := resolveEgressGatewayForEnvironment(repo, "org", env, nil)
		require.NoError(t, err)
		require.Equal(t, egress.UUID, got.UUID)
	})

	t.Run("an inactive egress gateway is still a candidate", func(t *testing.T) {
		// is_active is WebSocket liveness and flaps; delivery is durable through the
		// event hub, so selection must not depend on it.
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{ingress, inactiveEgress})
		got, err := resolveEgressGatewayForEnvironment(repo, "org", env, nil)
		require.NoError(t, err)
		require.Equal(t, inactiveEgress.UUID, got.UUID)
	})

	t.Run("two egress-capable and none named is ambiguous", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress})
		_, err := resolveEgressGatewayForEnvironment(repo, "org", env, nil)
		require.ErrorIs(t, err, errAmbiguousEgressGateway)
	})

	t.Run("ambiguity fires regardless of is_active", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, inactiveEgress})
		_, err := resolveEgressGatewayForEnvironment(repo, "org", env, nil)
		require.ErrorIs(t, err, errAmbiguousEgressGateway)
	})

	t.Run("ingress-only environment has no egress target", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{ingress})
		_, err := resolveEgressGatewayForEnvironment(repo, "org", env, nil)
		require.ErrorIs(t, err, errNoEgressGatewayForEnvironment)
	})

	t.Run("no gateways mapped at all is the tolerated condition", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), nil)
		_, err := resolveEgressGatewayForEnvironment(repo, "org", env, nil)
		require.ErrorIs(t, err, errNoGatewayForEnvironment)
	})

	t.Run("naming a valid candidate selects it", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress})
		id := egress.UUID.String()
		got, err := resolveEgressGatewayForEnvironment(repo, "org", env, &id)
		require.NoError(t, err)
		require.Equal(t, egress.UUID, got.UUID)
	})

	t.Run("naming an ingress-only gateway is invalid", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{ingress, egress})
		id := ingress.UUID.String()
		_, err := resolveEgressGatewayForEnvironment(repo, "org", env, &id)
		require.ErrorIs(t, err, errInvalidEgressGateway)
	})
}

func TestResolveEgressGatewayForArtifact(t *testing.T) {
	env := uuid.New()
	both := newGateway(t, models.GatewayRoleBoth, true)
	egress := newGateway(t, models.GatewayRoleEgress, true)

	t.Run("anchors to the existing deployment in a two-egress environment", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress})
		got, err := resolveEgressGatewayForArtifact(repo, "org", env, []string{both.UUID.String()}, nil)
		require.NoError(t, err)
		require.Equal(t, both.UUID, got.UUID)
	})

	t.Run("falls through to inference only when the artifact has no deployment", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress})
		_, err := resolveEgressGatewayForArtifact(repo, "org", env, nil, nil)
		require.ErrorIs(t, err, errAmbiguousEgressGateway)
	})

	t.Run("naming the artifact's current gateway is a no-op, not an error", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress})
		id := both.UUID.String()
		got, err := resolveEgressGatewayForArtifact(repo, "org", env, []string{id}, &id)
		require.NoError(t, err)
		require.Equal(t, both.UUID, got.UUID)
	})

	t.Run("naming a different gateway than the existing deployment is placement-fixed", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress})
		other := egress.UUID.String()
		_, err := resolveEgressGatewayForArtifact(repo, "org", env, []string{both.UUID.String()}, &other)
		require.ErrorIs(t, err, errPlacementFixed)
	})

	// A swallowed anchor-lookup error would fall through to environment-only selection,
	// where the caller's conflicting gatewayId validates cleanly and dual-homes the binding
	// errPlacementFixed exists to prevent. Both lookups therefore surface the error.
	errLookupFailed := errors.New("connection refused")

	t.Run("a failed mapping lookup never degrades into environment-only selection", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress})
		repo.EnvironmentMappingExistsFunc = func(string, string) (bool, error) {
			return false, errLookupFailed
		}
		other := egress.UUID.String()
		_, err := resolveEgressGatewayForArtifact(repo, "org", env, []string{both.UUID.String()}, &other)
		require.ErrorIs(t, err, errLookupFailed)
	})

	t.Run("a failed anchor load never degrades into environment-only selection", func(t *testing.T) {
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{both, egress})
		repo.GetByUUIDFunc = func(string) (*models.Gateway, error) {
			return nil, errLookupFailed
		}
		other := egress.UUID.String()
		_, err := resolveEgressGatewayForArtifact(repo, "org", env, []string{both.UUID.String()}, &other)
		require.ErrorIs(t, err, errLookupFailed)
	})

	t.Run("a deleted anchor gateway still falls through to environment-only selection", func(t *testing.T) {
		// Distinguishes an absent anchor from a failed lookup: the deployment row is stale
		// (gateway soft-deleted, so GetByUUID reports not-found) and selection must proceed.
		deleted := uuid.New().String()
		repo := gatewayFixtureRepo(t, env.String(), []*models.Gateway{egress})
		repo.EnvironmentMappingExistsFunc = func(gatewayID, environmentID string) (bool, error) {
			return gatewayID == deleted && environmentID == env.String(), nil
		}
		got, err := resolveEgressGatewayForArtifact(repo, "org", env, []string{deleted}, nil)
		require.NoError(t, err)
		require.Equal(t, egress.UUID, got.UUID)
	})
}

// gatewayRepoWithEnvMappings extends gatewayFixtureRepo with
// GetEnvironmentMappingsByGatewayIDFunc, keyed by gateway UUID string.
func gatewayRepoWithEnvMappings(
	t *testing.T, gateways []*models.Gateway, envMappings map[string][]uuid.UUID,
) *repomocks.GatewayRepositoryMock {
	t.Helper()
	repo := gatewayFixtureRepo(t, "", gateways)
	repo.GetEnvironmentMappingsByGatewayIDFunc = func(gatewayID string) ([]models.GatewayEnvironmentMapping, error) {
		envs := envMappings[gatewayID]
		out := make([]models.GatewayEnvironmentMapping, 0, len(envs))
		for _, envUUID := range envs {
			out = append(out, models.GatewayEnvironmentMapping{
				GatewayUUID:     uuid.MustParse(gatewayID),
				EnvironmentUUID: envUUID,
			})
		}
		return out, nil
	}
	return repo
}

func TestValidateEgressPlacement(t *testing.T) {
	envA := uuid.New()
	envB := uuid.New()

	t.Run("ingress-only gateway is rejected regardless of existing deployments", func(t *testing.T) {
		ingress := newGateway(t, models.GatewayRoleIngress, true)
		repo := gatewayRepoWithEnvMappings(t, []*models.Gateway{ingress}, nil)
		err := validateEgressPlacement(repo, ingress, nil)
		require.ErrorIs(t, err, errInvalidEgressGateway)
	})

	t.Run("no existing deployments short-circuits without consulting env mappings", func(t *testing.T) {
		egress := newGateway(t, models.GatewayRoleEgress, true)
		repo := gatewayRepoWithEnvMappings(t, []*models.Gateway{egress}, nil)
		err := validateEgressPlacement(repo, egress, nil)
		require.NoError(t, err)
	})

	t.Run("idempotent redeploy to the same gateway passes", func(t *testing.T) {
		egress := newGateway(t, models.GatewayRoleEgress, true)
		repo := gatewayRepoWithEnvMappings(t, []*models.Gateway{egress}, map[string][]uuid.UUID{
			egress.UUID.String(): {envA},
		})
		err := validateEgressPlacement(repo, egress, []string{egress.UUID.String()})
		require.NoError(t, err)
	})

	t.Run("same-environment clash with a different gateway is placement-fixed", func(t *testing.T) {
		target := newGateway(t, models.GatewayRoleEgress, true)
		existing := newGateway(t, models.GatewayRoleEgress, true)
		repo := gatewayRepoWithEnvMappings(t, []*models.Gateway{target, existing}, map[string][]uuid.UUID{
			target.UUID.String():   {envA},
			existing.UUID.String(): {envA},
		})
		err := validateEgressPlacement(repo, target, []string{existing.UUID.String()})
		require.ErrorIs(t, err, errPlacementFixed)
	})

	t.Run("disjoint-environment second deployment passes", func(t *testing.T) {
		target := newGateway(t, models.GatewayRoleEgress, true)
		existing := newGateway(t, models.GatewayRoleEgress, true)
		repo := gatewayRepoWithEnvMappings(t, []*models.Gateway{target, existing}, map[string][]uuid.UUID{
			target.UUID.String():   {envB},
			existing.UUID.String(): {envA},
		})
		err := validateEgressPlacement(repo, target, []string{existing.UUID.String()})
		require.NoError(t, err)
	})
}
