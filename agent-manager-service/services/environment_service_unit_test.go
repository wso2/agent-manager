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

// UNIT tests for environmentService. Like agent_kind_service_unit_test.go (the
// reference), this file has NO `//go:build integration` tag, so it runs in the
// fast unit tier with the dependencies mocked:
//
//   - repositories.GatewayRepository -> repomocks.GatewayRepositoryMock
//   - repositories.PlatformRepository      -> repomocks.PlatformRepositoryMock
//   - thundersvc.Prober              -> clientmocks.ThunderProberMock
//
// The goal is to exercise the service's OWN logic (error mapping to sentinels,
// validation gates, pagination, fan-out/aggregation, transformation) without a
// database. Unconfigured mock methods panic, so any unexpected call fails loudly.
package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// newEnvService wires the service with a discard logger and the two mocked deps.
// The Thunder prober is left unconfigured (panics if called).
func newEnvService(repo *repomocks.GatewayRepositoryMock, oc *repomocks.PlatformRepositoryMock) EnvironmentService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewEnvironmentService(logger, repo, oc, &clientmocks.ThunderProberMock{})
}

// -----------------------------------------------------------------------------
// CreateEnvironment — wraps client errors; maps client response on success.
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// GetEnvironment — maps not-found to the sentinel; wraps everything else.
// -----------------------------------------------------------------------------

func TestEnvironmentService_GetEnvironment(t *testing.T) {
	const org, envID = "acme", "dev"

	t.Run("maps not-found to ErrEnvironmentNotFound", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return nil, utils.ErrEnvironmentNotFound
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.GetEnvironment(context.Background(), org, envID)

		assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("wraps an unexpected client error", func(t *testing.T) {
		boom := errors.New("connection reset")
		oc := &repomocks.PlatformRepositoryMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.GetEnvironment(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
		assert.NotErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("returns mapped response on success", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{
					UUID:        "uuid-1",
					Name:        "dev",
					DisplayName: "Development",
					Description: "from oc",
					DNSPrefix:   "dev-prefix",
				}, nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.GetEnvironment(context.Background(), org, envID)

		require.NoError(t, err)
		assert.Equal(t, "dev", resp.Name)
		assert.Equal(t, org, resp.OrganizationName)
		// Unlike Create, Get carries description + DNS prefix straight from OC.
		assert.Equal(t, "from oc", resp.Description)
		assert.Equal(t, "dev-prefix", resp.DNSPrefix)
	})
}

// -----------------------------------------------------------------------------
// ListEnvironments — exercises pagination (offset/limit) and aggregation.
// -----------------------------------------------------------------------------

func TestEnvironmentService_ListEnvironments(t *testing.T) {
	const org = "acme"

	threeEnvs := func() []*models.EnvironmentResponse {
		return []*models.EnvironmentResponse{
			{UUID: "u1", Name: "env-0"},
			{UUID: "u2", Name: "env-1"},
			{UUID: "u3", Name: "env-2"},
		}
	}

	t.Run("wraps a client error", func(t *testing.T) {
		boom := errors.New("oc down")
		oc := &repomocks.PlatformRepositoryMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.ListEnvironments(context.Background(), org, 10, 0)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("applies limit within the available range", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return threeEnvs(), nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.ListEnvironments(context.Background(), org, 2, 0)

		require.NoError(t, err)
		assert.Equal(t, int32(3), resp.Total) // total reflects all, not the page
		require.Len(t, resp.Environments, 2)
		assert.Equal(t, "env-0", resp.Environments[0].Name)
		assert.Equal(t, "env-1", resp.Environments[1].Name)
		assert.Equal(t, org, resp.Environments[0].OrganizationName)
	})

	t.Run("clamps the page end when limit overruns the slice", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return threeEnvs(), nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.ListEnvironments(context.Background(), org, 10, 1)

		require.NoError(t, err)
		assert.Equal(t, int32(3), resp.Total)
		require.Len(t, resp.Environments, 2) // env-1, env-2
		assert.Equal(t, "env-1", resp.Environments[0].Name)
	})

	t.Run("returns an empty page when offset is past the end", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return threeEnvs(), nil
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		resp, err := svc.ListEnvironments(context.Background(), org, 10, 5)

		require.NoError(t, err)
		assert.Equal(t, int32(3), resp.Total) // total still reports everything
		assert.Empty(t, resp.Environments)
		assert.Equal(t, int32(5), resp.Offset)
	})
}

// -----------------------------------------------------------------------------
// UpdateEnvironment — both ErrNotFound and ErrEnvironmentNotFound collapse to
// the env sentinel; other errors are wrapped. Description comes from the request.
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// pipelineReferencesEnvironment — the pure predicate DeleteEnvironment uses to
// decide whether a pipeline blocks deletion (as source or as a promotion target).
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// DeleteEnvironment — the richest method: lookup, UUID parse, pipeline-reference
// guard, OC delete (idempotent on not-found), then local mapping cleanup.
// -----------------------------------------------------------------------------

// -----------------------------------------------------------------------------
// GetEnvironmentGateways — verify env, resolve mappings, fan-out per gateway,
// skip missing/errored gateways, and map IsActive -> status string.
// -----------------------------------------------------------------------------

func TestEnvironmentService_GetEnvironmentGateways(t *testing.T) {
	const org, envID = "acme", "dev"
	const envUUID = "33333333-3333-3333-3333-333333333333"

	t.Run("maps env not-found to ErrEnvironmentNotFound", func(t *testing.T) {
		oc := &repomocks.PlatformRepositoryMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return nil, utils.ErrEnvironmentNotFound
			},
		}
		svc := newEnvService(&repomocks.GatewayRepositoryMock{}, oc)

		_, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		assert.ErrorIs(t, err, utils.ErrEnvironmentNotFound)
	})

	t.Run("wraps a mapping-lookup error", func(t *testing.T) {
		boom := errors.New("mapping query failed")
		oc := &repomocks.PlatformRepositoryMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			GetEnvironmentMappingsByEnvironmentIDFunc: func(_ string) ([]models.GatewayEnvironmentMapping, error) {
				return nil, boom
			},
		}
		svc := newEnvService(repo, oc)

		_, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("aggregates gateways and skips missing/errored ones", func(t *testing.T) {
		activeGW := uuid.New()
		errorGW := uuid.New()
		missingGW := uuid.New()

		oc := &repomocks.PlatformRepositoryMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			GetEnvironmentMappingsByEnvironmentIDFunc: func(_ string) ([]models.GatewayEnvironmentMapping, error) {
				return []models.GatewayEnvironmentMapping{
					{GatewayUUID: activeGW},
					{GatewayUUID: errorGW},
					{GatewayUUID: missingGW},
				}, nil
			},
			GetByUUIDFunc: func(id string) (*models.Gateway, error) {
				switch id {
				case activeGW.String():
					return &models.Gateway{
						UUID:                     activeGW,
						Name:                     "gw-active",
						DisplayName:              "Active GW",
						GatewayFunctionalityType: "egress",
						Vhost:                    "gw.example.com",
						IsCritical:               true,
						IsActive:                 true,
					}, nil
				case errorGW.String():
					return nil, errors.New("transient") // skipped, not fatal
				case missingGW.String():
					//nolint:nilnil // intentionally exercising the (nil, nil) "missing, skip" input the service must handle
					return nil, nil // skipped
				default:
					return nil, errors.New("unexpected gateway id")
				}
			},
		}
		svc := newEnvService(repo, oc)

		resp, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		require.NoError(t, err)
		require.Len(t, resp, 1) // only the active gateway survives
		assert.Equal(t, "gw-active", resp[0].Name)
		assert.Equal(t, activeGW.String(), resp[0].UUID)
		assert.Equal(t, org, resp[0].OrganizationName)
		assert.True(t, resp[0].IsCritical)
		assert.Equal(t, string(models.GatewayStatusActive), resp[0].Status)
	})

	t.Run("maps an inactive gateway to inactive status", func(t *testing.T) {
		gw := uuid.New()
		oc := &repomocks.PlatformRepositoryMock{
			GetEnvironmentFunc: func(_ context.Context, _, _ string) (*models.EnvironmentResponse, error) {
				return &models.EnvironmentResponse{UUID: envUUID, Name: "dev"}, nil
			},
		}
		repo := &repomocks.GatewayRepositoryMock{
			GetEnvironmentMappingsByEnvironmentIDFunc: func(_ string) ([]models.GatewayEnvironmentMapping, error) {
				return []models.GatewayEnvironmentMapping{{GatewayUUID: gw}}, nil
			},
			GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
				return &models.Gateway{UUID: gw, Name: "gw-idle", IsActive: false}, nil
			},
		}
		svc := newEnvService(repo, oc)

		resp, err := svc.GetEnvironmentGateways(context.Background(), org, envID)

		require.NoError(t, err)
		require.Len(t, resp, 1)
		assert.Equal(t, string(models.GatewayStatusInactive), resp[0].Status)
	})
}

// -----------------------------------------------------------------------------
// ListThunderInstances — gates returning Thunder instance info on whether
// the env-Thunder JWKS endpoint is actually reachable (live HTTP probe).
// -----------------------------------------------------------------------------

func TestEnvironmentService_ListThunderInstances(t *testing.T) {
	const org = "acme"
	envs := func(names ...string) *repomocks.PlatformRepositoryMock {
		return &repomocks.PlatformRepositoryMock{
			ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
				out := make([]*models.EnvironmentResponse, 0, len(names))
				for _, name := range names {
					out = append(out, &models.EnvironmentResponse{UUID: "u-" + name, Name: name, DisplayName: name})
				}
				return out, nil
			},
		}
	}

	t.Run("wraps list environments error", func(t *testing.T) {
		boom := errors.New("db down")
		oc := &repomocks.PlatformRepositoryMock{
			ListEnvironmentsFunc: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
				return nil, boom
			},
		}
		_, err := newEnvService(&repomocks.GatewayRepositoryMock{}, oc).ListThunderInstances(context.Background(), org)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("probes only the default environment", func(t *testing.T) {
		withPlatformThunderConfig(t, "http://thunder.example.test", "client", "secret")
		resp, err := newEnvService(&repomocks.GatewayRepositoryMock{}, envs("prod", "")).ListThunderInstances(context.Background(), org)
		require.NoError(t, err)
		assert.Empty(t, resp.ThunderInstances)
	})

	t.Run("skips an unreachable platform Thunder", func(t *testing.T) {
		withPlatformThunderConfig(t, "http://thunder.example.test", "client", "secret")
		prober := &clientmocks.ThunderProberMock{
			ProbeFunc: func(context.Context, string, string, string, bool) bool { return false },
		}
		svc := NewEnvironmentService(discardLogger(), &repomocks.GatewayRepositoryMock{}, envs(models.DefaultEnvironmentName), prober)
		resp, err := svc.ListThunderInstances(context.Background(), org)
		require.NoError(t, err)
		assert.Empty(t, resp.ThunderInstances)
	})

	t.Run("advertises the reachable platform Thunder for the default environment", func(t *testing.T) {
		const thunderURL = "http://thunder.example.test"
		withPlatformThunderConfig(t, thunderURL, "client", "secret")
		var callerSupplied bool
		prober := &clientmocks.ThunderProberMock{
			ProbeFunc: func(_ context.Context, _, _, url string, supplied bool) bool {
				callerSupplied = supplied
				return url == thunderURL
			},
		}
		svc := NewEnvironmentService(discardLogger(), &repomocks.GatewayRepositoryMock{}, envs(models.DefaultEnvironmentName), prober)
		resp, err := svc.ListThunderInstances(context.Background(), org)
		require.NoError(t, err)
		require.Len(t, resp.ThunderInstances, 1)
		assert.True(t, callerSupplied, "the platform URL is probed SSRF-hardened")
		got := resp.ThunderInstances[0]
		assert.Equal(t, models.DefaultEnvironmentName, got.EnvName)
		assert.Equal(t, thunderURL, got.IssuerURL)
		assert.Equal(t, thunderURL+"/oauth2/token", got.TokenURL)
		assert.Equal(t, thunderURL+"/oauth2/jwks", got.JWKSURL)
	})
}
