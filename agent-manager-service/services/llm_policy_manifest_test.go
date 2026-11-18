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
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// gatewayWithLLMPolicyManifest builds a Gateway whose Manifest advertises the given
// policies, in the shape the gateway-controller actually pushes ({"policies": [...]}).
func gatewayWithLLMPolicyManifest(policies ...map[string]interface{}) *models.Gateway {
	items := make([]interface{}, 0, len(policies))
	for _, p := range policies {
		items = append(items, p)
	}
	return &models.Gateway{Manifest: map[string]interface{}{"policies": items}}
}

// -----------------------------------------------------------------------------
// extractLLMPolicyManifestItems — the manifest walk itself.
// -----------------------------------------------------------------------------

func TestExtractLLMPolicyManifestItems_FullFieldExtraction(t *testing.T) {
	manifest := map[string]interface{}{
		"policies": []interface{}{
			map[string]interface{}{
				"name":        "word-count-guardrail",
				"version":     "v1.0.0",
				"displayName": "Word Count Guardrail",
				"description": "Validates word count.",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"min": map[string]interface{}{"type": "integer"},
					},
				},
				"systemParameters": map[string]interface{}{
					"type": "object",
				},
			},
		},
	}

	items := extractLLMPolicyManifestItems(manifest)

	require.Len(t, items, 1)
	item := items[0]
	assert.Equal(t, "word-count-guardrail", item.Name)
	assert.Equal(t, "v1.0.0", item.Version)
	assert.Equal(t, "Word Count Guardrail", item.DisplayName)
	assert.Equal(t, "Validates word count.", item.Description)
	require.NotNil(t, item.Parameters)
	assert.Equal(t, "object", item.Parameters["type"])
	require.NotNil(t, item.SystemParameters)
	assert.Equal(t, "object", item.SystemParameters["type"])
}

func TestExtractLLMPolicyManifestItems_ToleratesKeyAliases(t *testing.T) {
	manifest := map[string]interface{}{
		"policies": []interface{}{
			map[string]interface{}{"policyName": "aliased-by-policyname", "version": "v1"},
			map[string]interface{}{"id": "aliased-by-id", "version": "v1"},
			map[string]interface{}{"name": "aliased-version", "policyVersion": "v2"},
		},
	}

	items := extractLLMPolicyManifestItems(manifest)

	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	assert.ElementsMatch(t, []string{"aliased-by-policyname", "aliased-by-id", "aliased-version"}, names)
}

func TestExtractLLMPolicyManifestItems_ExpandsVersionsArray(t *testing.T) {
	manifest := map[string]interface{}{
		"policies": []interface{}{
			map[string]interface{}{
				"name":     "multi-version-policy",
				"versions": []interface{}{"v1", "v2"},
			},
		},
	}

	items := extractLLMPolicyManifestItems(manifest)

	versions := make([]string, 0, len(items))
	for _, item := range items {
		assert.Equal(t, "multi-version-policy", item.Name)
		versions = append(versions, item.Version)
	}
	assert.ElementsMatch(t, []string{"v1", "v2"}, versions)
}

func TestExtractLLMPolicyManifestItems_IgnoresCoincidentalNameVersionInSchema(t *testing.T) {
	// A real policy whose parameter/system schemas embed nested objects that happen
	// to carry their own name+version string pairs (e.g. a default/example model).
	// Only the top-level policy must surface — the schema leaves must not.
	manifest := map[string]interface{}{
		"policies": []interface{}{
			map[string]interface{}{
				"name":    "model-round-robin",
				"version": "v1.0.2",
				"parameters": map[string]interface{}{
					"type": "object",
					"properties": map[string]interface{}{
						"defaultModel": map[string]interface{}{
							"name":    "gpt-4",
							"version": "1.0",
						},
					},
				},
				"systemParameters": map[string]interface{}{
					"example": map[string]interface{}{
						"name":    "nested-example",
						"version": "9.9",
					},
				},
			},
		},
	}

	items := extractLLMPolicyManifestItems(manifest)

	require.Len(t, items, 1)
	assert.Equal(t, "model-round-robin", items[0].Name)
	assert.Equal(t, "v1.0.2", items[0].Version)
}

func TestExtractLLMPolicyManifestItems_EmptyOrMalformedManifest(t *testing.T) {
	assert.Empty(t, extractLLMPolicyManifestItems(nil))
	assert.Empty(t, extractLLMPolicyManifestItems(map[string]interface{}{}))
	assert.Empty(t, extractLLMPolicyManifestItems("not a map"))
}

// -----------------------------------------------------------------------------
// intersectActiveGatewayLLMPolicies — active-gateway intersection semantics.
// -----------------------------------------------------------------------------

func TestIntersectActiveGatewayLLMPolicies_NilRepoReturnsEmpty(t *testing.T) {
	available, err := intersectActiveGatewayLLMPolicies(nil, "org-uuid")

	require.NoError(t, err)
	assert.NotNil(t, available)
	assert.Empty(t, available)
}

func TestIntersectActiveGatewayLLMPolicies_SingleGateway(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithLLMPolicyManifest(map[string]interface{}{"name": "word-count-guardrail", "version": "v1"}),
			}, nil
		},
	}

	available, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.NoError(t, err)
	require.Len(t, available, 1)
	assert.Contains(t, available, "word-count-guardrail\x00v1")
}

// A gateway that has not reported a manifest yet (no push, empty row) must not empty the
// org's guardrail picker: it is left out until it reports.
func TestIntersectActiveGatewayLLMPolicies_SkipsGatewayThatHasNotReported(t *testing.T) {
	clearGatewayManifestCache(t)
	reported := newGateway(t, models.GatewayRoleBoth, true)
	require.NoError(t, gatewayManifestCache.Set(context.Background(), reported.UUID.String(),
		policyManifest("word-count-guardrail", "v1")))
	silent := newGateway(t, models.GatewayRoleBoth, true)
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{reported, silent}, nil
		},
	}

	available, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.NoError(t, err)
	require.Len(t, available, 1)
	assert.Contains(t, available, "word-count-guardrail\x00v1")
}

// When no gateway has reported, nothing is available (the cold-start behaviour).
func TestIntersectActiveGatewayLLMPolicies_NothingReportedIsEmpty(t *testing.T) {
	clearGatewayManifestCache(t)
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{newGateway(t, models.GatewayRoleBoth, true)}, nil
		},
	}

	available, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.NoError(t, err)
	assert.Empty(t, available)
}

func TestIntersectActiveGatewayLLMPolicies_OverlappingPoliciesSurvive(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "shared-policy", "version": "v1"},
					map[string]interface{}{"name": "only-on-first", "version": "v1"},
				),
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "shared-policy", "version": "v1"},
				),
			}, nil
		},
	}

	available, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.NoError(t, err)
	// Only the policy reported by EVERY active gateway survives the intersection.
	require.Len(t, available, 1)
	assert.Contains(t, available, "shared-policy\x00v1")
	assert.NotContains(t, available, "only-on-first\x00v1")
}

func TestIntersectActiveGatewayLLMPolicies_VersionMismatchFallsBackToLowest(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				// Old gateway, not yet upgraded.
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "pii-filter", "version": "1.0.0"},
				),
				// New gateway, already upgraded — same policy, newer version.
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "pii-filter", "version": "1.1.0"},
				),
			}, nil
		},
	}

	available, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.NoError(t, err)
	// Every gateway has the policy, just at different versions — treated as
	// backward-compatible: falls back to the lowest version instead of vanishing.
	require.Len(t, available, 1)
	assert.Contains(t, available, "pii-filter\x00"+"1.0.0")
	assert.NotContains(t, available, "pii-filter\x00"+"1.1.0")
}

func TestIntersectActiveGatewayLLMPolicies_MissingFromOneGatewayStaysExcluded(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "pii-filter", "version": "1.0.0"},
				),
				// Second gateway doesn't have this policy at all — not a version
				// mismatch, genuinely unsupported here. Must NOT be rescued by the
				// lowest-version fallback.
				gatewayWithLLMPolicyManifest(),
			}, nil
		},
	}

	available, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.NoError(t, err)
	assert.Empty(t, available)
}

func TestIntersectActiveGatewayLLMPolicies_VersionMismatchUsesSemverComparison(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "pii-filter", "version": "1.9.0"},
				),
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "pii-filter", "version": "1.10.0"},
				),
			}, nil
		},
	}

	available, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.NoError(t, err)
	// A plain string compare would wrongly treat "1.10.0" < "1.9.0". Numeric
	// per-segment comparison must pick 1.9.0 as the lower version.
	require.Len(t, available, 1)
	assert.Contains(t, available, "pii-filter\x00"+"1.9.0")
}

func TestIntersectActiveGatewayLLMPolicies_VersionMismatchStripsVPrefixForSemverComparison(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "pii-filter", "version": "v2"},
				),
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "pii-filter", "version": "v10"},
				),
			}, nil
		},
	}

	available, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.NoError(t, err)
	// A plain string compare of "v10" vs "v2" would wrongly rank "v10" lower.
	// Stripping the leading "v" before numeric comparison must pick v2 as lower.
	require.Len(t, available, 1)
	assert.Contains(t, available, "pii-filter\x00"+"v2")
}

func TestIntersectActiveGatewayLLMPolicies_OnlyActiveGatewaysQueried(t *testing.T) {
	var capturedFilters repositories.GatewayFilterOptions
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(filters repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			capturedFilters = filters
			return []*models.Gateway{}, nil
		},
	}

	_, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.NoError(t, err)
	assert.Equal(t, "org-uuid", capturedFilters.OrganizationID)
	require.NotNil(t, capturedFilters.Status)
	assert.True(t, *capturedFilters.Status)
	assert.Equal(t, models.GatewayHostingManaged, capturedFilters.Hosting)
}

func TestIntersectActiveGatewayLLMPolicies_RepoErrorIsWrapped(t *testing.T) {
	boom := errors.New("db unreachable")
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return nil, boom
		},
	}

	_, err := intersectActiveGatewayLLMPolicies(repo, "org-uuid")

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

// -----------------------------------------------------------------------------
// intersectDeployedGatewayLLMPolicies — provider-scoped intersection, restricted to
// the gateways a specific provider is actually deployed to.
// -----------------------------------------------------------------------------

func TestIntersectDeployedGatewayLLMPolicies_NilRepoReturnsEmpty(t *testing.T) {
	available, err := intersectDeployedGatewayLLMPolicies(context.Background(), nil, nil, uuid.New(), "org-uuid")

	require.NoError(t, err)
	assert.Empty(t, available)
}

func TestIntersectDeployedGatewayLLMPolicies_ScopesToDeployedGatewaysOnly(t *testing.T) {
	providerUUID := uuid.New()
	deployedGatewayID := "gw-1"
	undeployedGateway := gatewayWithLLMPolicyManifest(
		map[string]interface{}{"name": "only-on-undeployed-gateway", "version": "v1"},
	)

	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, orgUUID string, artifactUUID uuid.UUID) ([]string, error) {
			assert.Equal(t, providerUUID, artifactUUID)
			assert.Equal(t, "org-uuid", orgUUID)
			return []string{deployedGatewayID}, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(gatewayId string) (*models.Gateway, error) {
			assert.Equal(t, deployedGatewayID, gatewayId)
			gw := gatewayWithLLMPolicyManifest(
				map[string]interface{}{"name": "deployed-policy", "version": "v1"},
			)
			gw.OUID = "org-uuid"
			return gw, nil
		},
	}
	_ = undeployedGateway // never queried — GetByUUIDFunc only ever asked for the deployed gateway.

	available, err := intersectDeployedGatewayLLMPolicies(context.Background(), gatewayRepo, deploymentRepo, providerUUID, "org-uuid")

	require.NoError(t, err)
	require.Len(t, available, 1)
	assert.Contains(t, available, "deployed-policy\x00v1")
}

func TestIntersectDeployedGatewayLLMPolicies_SkipsGatewayFromAnotherOrg(t *testing.T) {
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, _ uuid.UUID) ([]string, error) {
			return []string{"gw-1"}, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
			gw := gatewayWithLLMPolicyManifest(
				map[string]interface{}{"name": "cross-org-policy", "version": "v1"},
			)
			gw.OUID = "another-org"
			return gw, nil
		},
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{}, nil
		},
	}

	available, err := intersectDeployedGatewayLLMPolicies(context.Background(), gatewayRepo, deploymentRepo, uuid.New(), "org-uuid")

	require.NoError(t, err)
	assert.Empty(t, available)
}

func TestIntersectDeployedGatewayLLMPolicies_SkipsStaleGatewayReference(t *testing.T) {
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, _ uuid.UUID) ([]string, error) {
			return []string{"gw-deleted"}, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
			return nil, gorm.ErrRecordNotFound
		},
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{}, nil
		},
	}

	available, err := intersectDeployedGatewayLLMPolicies(context.Background(), gatewayRepo, deploymentRepo, uuid.New(), "org-uuid")

	require.NoError(t, err)
	assert.Empty(t, available)
}

func TestIntersectDeployedGatewayLLMPolicies_DeploymentRepoErrorIsWrapped(t *testing.T) {
	boom := errors.New("db unreachable")
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, _ uuid.UUID) ([]string, error) {
			return nil, boom
		},
	}

	_, err := intersectDeployedGatewayLLMPolicies(context.Background(), &repomocks.GatewayRepositoryMock{}, deploymentRepo, uuid.New(), "org-uuid")

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

// -----------------------------------------------------------------------------
// managedBy, per-gateway presence and catalog gateway resolution.
// -----------------------------------------------------------------------------

func TestExtractLLMPolicyManifestItems_ReadsManagedBy(t *testing.T) {
	manifest := map[string]interface{}{"policies": []interface{}{
		map[string]interface{}{"name": "my-guard", "version": "v1.0.0", "managedBy": "organization"},
		map[string]interface{}{"name": "hub-one", "version": "v1.0.0", "managedBy": "wso2"},
	}}

	items := extractLLMPolicyManifestItems(manifest)

	require.Len(t, items, 2)
	byName := map[string]llmPolicyManifestItem{}
	for _, item := range items {
		byName[item.Name] = item
	}
	assert.Equal(t, "organization", byName["my-guard"].ManagedBy)
	assert.Equal(t, "wso2", byName["hub-one"].ManagedBy)
}

func catalogGateway(name string, policies ...map[string]interface{}) *models.Gateway {
	gw := gatewayWithLLMPolicyManifest(policies...)
	gw.UUID = uuid.New()
	gw.Name = name
	gw.DisplayName = name + " display"
	return gw
}

func TestCollectInstalledPolicies_CustomFlag(t *testing.T) {
	a := catalogGateway("a",
		map[string]interface{}{"name": "my-guard", "version": "v1.0.0", "managedBy": "organization"},
		map[string]interface{}{"name": "p1", "version": "v1.0.0"},
	)
	b := catalogGateway("b",
		map[string]interface{}{"name": "p1", "version": "v1.0.0"},
	)

	got := collectInstalledPolicies([]*models.Gateway{a, b})

	assert.True(t, got.Custom["my-guard"])
	assert.False(t, got.Custom["p1"])
	assert.Len(t, got.Reporting, 2)
}

func TestCollectInstalledPolicies_MissingOn(t *testing.T) {
	a := catalogGateway("a",
		map[string]interface{}{"name": "p1", "version": "v1.0.0"},
		map[string]interface{}{"name": "p2", "version": "v1.0.0"},
	)
	b := catalogGateway("b",
		map[string]interface{}{"name": "p1", "version": "v1.0.0"},
	)

	got := collectInstalledPolicies([]*models.Gateway{a, b})

	assert.Equal(t, []gatewayRef{{ID: b.UUID.String(), Name: b.DisplayName}}, got.MissingOn["p2"])
	assert.NotContains(t, got.ByName, "p2")
	assert.Contains(t, got.ByName, "p1")
	assert.NotContains(t, got.MissingOn, "p1")
}

func TestCollectInstalledPolicies_NameFallsBackToGatewayName(t *testing.T) {
	a := catalogGateway("a", map[string]interface{}{"name": "p1", "version": "v1.0.0"})
	b := catalogGateway("b")
	b.DisplayName = ""
	b.Manifest = map[string]interface{}{"policies": []interface{}{}, "x": "y"}

	got := collectInstalledPolicies([]*models.Gateway{a, b})

	assert.Equal(t, []gatewayRef{{ID: b.UUID.String(), Name: "b"}}, got.MissingOn["p1"])
}

func TestCollectInstalledPolicies_UnreportedGatewayExcluded(t *testing.T) {
	a := catalogGateway("a", map[string]interface{}{"name": "p1", "version": "v1.0.0"})
	silent := &models.Gateway{UUID: uuid.New(), Name: "silent"}

	got := collectInstalledPolicies([]*models.Gateway{a, silent})

	assert.Equal(t, []gatewayRef{{ID: a.UUID.String(), Name: a.DisplayName}}, got.Reporting)
	assert.Empty(t, got.MissingOn)
	assert.Contains(t, got.ByName, "p1")
}

func TestCollectInstalledPolicies_RollingUpgradeStaysInstalled(t *testing.T) {
	a := catalogGateway("a", map[string]interface{}{"name": "p1", "version": "v1.0.1"})
	b := catalogGateway("b", map[string]interface{}{"name": "p1", "version": "v1.0.2"})

	got := collectInstalledPolicies([]*models.Gateway{a, b})

	require.Contains(t, got.ByName, "p1")
	assert.Equal(t, "v1.0.1", got.ByName["p1"].Version)
	assert.NotContains(t, got.MissingOn, "p1")
}

func TestResolveCatalogGateways_DeployedScope(t *testing.T) {
	artifactUUID := uuid.New()
	selfHosted := catalogGateway("sh")
	selfHosted.OUID = "org-uuid"
	selfHosted.Hosting = models.GatewayHostingSelfHosted
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, id uuid.UUID) ([]string, error) {
			assert.Equal(t, artifactUUID, id)
			return []string{"gw-sh"}, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) { return selfHosted, nil },
	}

	gateways, scope, err := resolveCatalogGateways(context.Background(), gatewayRepo, deploymentRepo, "org-uuid", []uuid.UUID{artifactUUID})

	require.NoError(t, err)
	assert.Equal(t, models.PolicyAvailabilityScopeDeployed, scope)
	assert.Equal(t, []*models.Gateway{selfHosted}, gateways)
}

func TestResolveCatalogGateways_UndeployedFallsBackToOrgManaged(t *testing.T) {
	artifactUUID := uuid.New()
	managed := catalogGateway("m")
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		PlacedGatewaysFunc: func(_ context.Context, _ string, _ uuid.UUID) ([]string, error) {
			return []string{}, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(f repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			assert.Equal(t, models.GatewayHostingManaged, f.Hosting)
			assert.Equal(t, "org-uuid", f.OrganizationID)
			return []*models.Gateway{managed}, nil
		},
	}

	for name, id := range map[string][]uuid.UUID{"placed but empty": {artifactUUID}, "no artifact": nil} {
		t.Run(name, func(t *testing.T) {
			gateways, scope, err := resolveCatalogGateways(context.Background(), gatewayRepo, deploymentRepo, "org-uuid", id)

			require.NoError(t, err)
			assert.Equal(t, models.PolicyAvailabilityScopeOrgManaged, scope)
			assert.Equal(t, []*models.Gateway{managed}, gateways)
		})
	}
}

// -----------------------------------------------------------------------------
// sortedLLMPolicyManifestItems — stable output ordering for the API response.
// -----------------------------------------------------------------------------

func TestSortedLLMPolicyManifestItems_OrdersByNameThenVersion(t *testing.T) {
	available := map[string]llmPolicyManifestItem{
		"zebra-policy\x00v1": {Name: "zebra-policy", Version: "v1"},
		"alpha-policy\x00v2": {Name: "alpha-policy", Version: "v2"},
		"alpha-policy\x00v1": {Name: "alpha-policy", Version: "v1"},
	}

	sorted := sortedLLMPolicyManifestItems(available)

	require.Len(t, sorted, 3)
	assert.Equal(t, "alpha-policy", sorted[0].Name)
	assert.Equal(t, "v1", sorted[0].Version)
	assert.Equal(t, "alpha-policy", sorted[1].Name)
	assert.Equal(t, "v2", sorted[1].Version)
	assert.Equal(t, "zebra-policy", sorted[2].Name)
}
