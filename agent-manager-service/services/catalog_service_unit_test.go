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

// UNIT tests for catalogService. Like agent_kind_service_unit_test.go (the
// reference), this file has NO `//go:build integration` tag, so it runs in the
// fast unit tier with all collaborators mocked:
//
//   - repositories.CatalogRepository -> repomocks.CatalogRepositoryMock
//   - repositories.PlatformRepository        -> repomocks.PlatformRepositoryMock
//
// The service has no sentinel errors of its own; it validates input, branches on
// the kind/deployment shape, wraps repository/client errors with %w, and (for
// LLM providers) enriches deployment environment UUIDs with human-readable names
// fetched from OpenChoreo. These tests drive each of those branches.
//
// NOTE: catalogService relies on a package-global env-mapping cache
// (globalEnvCache). To keep tests independent, every test that exercises the
// enrichment path uses a UNIQUE org name so it never reads another test's
// cached entry.
package services

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// newCatalogService wires the service with a discard logger and the two mocked
// dependencies.
func newCatalogService(repo *repomocks.CatalogRepositoryMock, oc *repomocks.PlatformRepositoryMock) CatalogService {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	return NewCatalogService(logger, repo, oc)
}

// -----------------------------------------------------------------------------
// ListCatalog — validation gate, kind branching, and error wrapping.
// -----------------------------------------------------------------------------

func TestCatalogService_ListCatalog(t *testing.T) {
	t.Run("rejects empty ouID", func(t *testing.T) {
		// Both repo methods left nil: hitting either would panic, proving the
		// guard short-circuits before any data access.
		svc := newCatalogService(&repomocks.CatalogRepositoryMock{}, &repomocks.PlatformRepositoryMock{})

		entries, total, err := svc.ListCatalog(context.Background(), "", "agent", 10, 0)

		require.Error(t, err)
		assert.Nil(t, entries)
		assert.Zero(t, total)
	})

	t.Run("no kind filter routes to ListAll", func(t *testing.T) {
		listAllCalled := false
		repo := &repomocks.CatalogRepositoryMock{
			ListAllFunc: func(orgUUID string, limit, offset int) ([]models.CatalogEntry, int64, error) {
				listAllCalled = true
				assert.Equal(t, "acme", orgUUID)
				assert.Equal(t, 10, limit)
				assert.Equal(t, 5, offset)
				return []models.CatalogEntry{{Name: "a"}, {Name: "b"}}, 2, nil
			},
			// ListByKind left nil: must NOT be reached.
		}
		svc := newCatalogService(repo, &repomocks.PlatformRepositoryMock{})

		entries, total, err := svc.ListCatalog(context.Background(), "acme", "", 10, 5)

		require.NoError(t, err)
		assert.True(t, listAllCalled, "expected ListAll to be called")
		assert.Len(t, entries, 2)
		assert.Equal(t, int64(2), total)
	})

	t.Run("kind filter routes to ListByKind", func(t *testing.T) {
		listByKindCalled := false
		repo := &repomocks.CatalogRepositoryMock{
			ListByKindFunc: func(orgUUID, kind string, limit, offset int) ([]models.CatalogEntry, int64, error) {
				listByKindCalled = true
				assert.Equal(t, "acme", orgUUID)
				assert.Equal(t, "agent", kind)
				return []models.CatalogEntry{{Name: "agent-1"}}, 1, nil
			},
			// ListAll left nil: must NOT be reached.
		}
		svc := newCatalogService(repo, &repomocks.PlatformRepositoryMock{})

		entries, total, err := svc.ListCatalog(context.Background(), "acme", "agent", 20, 0)

		require.NoError(t, err)
		assert.True(t, listByKindCalled, "expected ListByKind to be called")
		assert.Len(t, entries, 1)
		assert.Equal(t, int64(1), total)
	})

	t.Run("wraps repository error", func(t *testing.T) {
		boom := errors.New("db down")
		repo := &repomocks.CatalogRepositoryMock{
			ListAllFunc: func(_ string, _, _ int) ([]models.CatalogEntry, int64, error) {
				return nil, 0, boom
			},
		}
		svc := newCatalogService(repo, &repomocks.PlatformRepositoryMock{})

		entries, total, err := svc.ListCatalog(context.Background(), "acme", "", 10, 0)

		require.Error(t, err)
		assert.ErrorIs(t, err, boom) // wrapped with %w, not flattened
		assert.Nil(t, entries)
		assert.Zero(t, total)
	})
}

// -----------------------------------------------------------------------------
// ListLLMProviders — nil/validation guards, repo error wrapping, and the
// environment-enrichment branching (no deployments => no client call; with
// deployments => UUIDs resolved to names, unknown UUIDs left as-is).
// -----------------------------------------------------------------------------

func TestCatalogService_ListLLMProviders(t *testing.T) {
	t.Run("rejects nil filters", func(t *testing.T) {
		// Repo left nil: must NOT be reached.
		svc := newCatalogService(&repomocks.CatalogRepositoryMock{}, &repomocks.PlatformRepositoryMock{})

		entries, total, err := svc.ListLLMProviders(context.Background(), nil)

		require.Error(t, err)
		assert.Nil(t, entries)
		assert.Zero(t, total)
	})

	t.Run("rejects invalid filters (empty org)", func(t *testing.T) {
		// Validate() fails on empty OrganizationName before any repo access.
		svc := newCatalogService(&repomocks.CatalogRepositoryMock{}, &repomocks.PlatformRepositoryMock{})

		_, _, err := svc.ListLLMProviders(context.Background(), &models.CatalogListFilters{
			OrganizationName: "",
			Limit:            10,
		})

		require.Error(t, err)
	})

	t.Run("wraps repository error", func(t *testing.T) {
		boom := errors.New("query failed")
		repo := &repomocks.CatalogRepositoryMock{
			ListLLMProvidersFunc: func(_ *models.CatalogListFilters) ([]models.CatalogLLMProviderEntry, int64, error) {
				return nil, 0, boom
			},
		}
		svc := newCatalogService(repo, &repomocks.PlatformRepositoryMock{})

		_, _, err := svc.ListLLMProviders(context.Background(), &models.CatalogListFilters{
			OrganizationName: "wrap-org",
			Limit:            10,
		})

		require.Error(t, err)
		assert.ErrorIs(t, err, boom)
	})

	t.Run("skips environment fetch when no deployments exist", func(t *testing.T) {
		repo := &repomocks.CatalogRepositoryMock{
			ListLLMProvidersFunc: func(_ *models.CatalogListFilters) ([]models.CatalogLLMProviderEntry, int64, error) {
				return []models.CatalogLLMProviderEntry{
					{Name: "openai", Deployments: nil},
				}, 1, nil
			},
		}
		// ListEnvironments left nil: hitting it would panic, asserting the
		// service does NOT make the external call when there is nothing to enrich.
		svc := newCatalogService(repo, &repomocks.PlatformRepositoryMock{})

		entries, total, err := svc.ListLLMProviders(context.Background(), &models.CatalogListFilters{
			OrganizationName: "no-deploy-org",
			Limit:            10,
		})

		require.NoError(t, err)
		assert.Len(t, entries, 1)
		assert.Equal(t, int64(1), total)
	})

	t.Run("resolves deployment environment UUIDs to names", func(t *testing.T) {
		const envUUID = "11111111-1111-1111-1111-111111111111"
		const unknownUUID = "22222222-2222-2222-2222-222222222222"

		repo := &repomocks.CatalogRepositoryMock{
			ListLLMProvidersFunc: func(_ *models.CatalogListFilters) ([]models.CatalogLLMProviderEntry, int64, error) {
				known := envUUID
				unknown := unknownUUID
				return []models.CatalogLLMProviderEntry{
					{
						Name: "openai",
						Deployments: []models.DeploymentSummary{
							{GatewayName: "gw-1", EnvironmentName: &known},
							{GatewayName: "gw-2", EnvironmentName: &unknown},
							{GatewayName: "gw-3", EnvironmentName: nil},
						},
					},
				}, 1, nil
			},
		}
		listEnvCalled := false
		oc := &repomocks.PlatformRepositoryMock{
			ListEnvironmentsFunc: func(_ context.Context, namespace string) ([]*models.EnvironmentResponse, error) {
				listEnvCalled = true
				assert.Equal(t, "resolve-org", namespace)
				return []*models.EnvironmentResponse{
					{UUID: envUUID, Name: "production"},
				}, nil
			},
		}
		svc := newCatalogService(repo, oc)

		entries, _, err := svc.ListLLMProviders(context.Background(), &models.CatalogListFilters{
			OrganizationName: "resolve-org",
			Limit:            10,
		})

		require.NoError(t, err)
		assert.True(t, listEnvCalled, "expected ListEnvironments to be called")
		require.Len(t, entries, 1)
		deployments := entries[0].Deployments
		require.Len(t, deployments, 3)

		// Known UUID resolved to its human-readable name.
		require.NotNil(t, deployments[0].EnvironmentName)
		assert.Equal(t, "production", *deployments[0].EnvironmentName)
		// Unknown UUID left untouched (kept as the raw UUID).
		require.NotNil(t, deployments[1].EnvironmentName)
		assert.Equal(t, unknownUUID, *deployments[1].EnvironmentName)
		// Nil environment stays nil.
		assert.Nil(t, deployments[2].EnvironmentName)
	})

	t.Run("continues without names when environment fetch fails", func(t *testing.T) {
		const envUUID = "33333333-3333-3333-3333-333333333333"
		repo := &repomocks.CatalogRepositoryMock{
			ListLLMProvidersFunc: func(_ *models.CatalogListFilters) ([]models.CatalogLLMProviderEntry, int64, error) {
				known := envUUID
				return []models.CatalogLLMProviderEntry{
					{
						Name: "openai",
						Deployments: []models.DeploymentSummary{
							{GatewayName: "gw-1", EnvironmentName: &known},
						},
					},
				}, 1, nil
			},
		}
		oc := &repomocks.PlatformRepositoryMock{
			ListEnvironmentsFunc: func(_ context.Context, _ string) ([]*models.EnvironmentResponse, error) {
				return nil, errors.New("openchoreo unreachable")
			},
		}
		svc := newCatalogService(repo, oc)

		// The enrichment failure is swallowed (logged + empty map); the call
		// still succeeds and the raw UUID is preserved.
		entries, total, err := svc.ListLLMProviders(context.Background(), &models.CatalogListFilters{
			OrganizationName: "env-fail-org",
			Limit:            10,
		})

		require.NoError(t, err)
		assert.Equal(t, int64(1), total)
		require.Len(t, entries, 1)
		require.Len(t, entries[0].Deployments, 1)
		require.NotNil(t, entries[0].Deployments[0].EnvironmentName)
		assert.Equal(t, envUUID, *entries[0].Deployments[0].EnvironmentName)
	})
}
