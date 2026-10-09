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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// gatewayWithPolicyManifest builds a Gateway whose Manifest advertises the given
// policies, in the shape the gateway-controller actually pushes ({"policies": [...]}).
func gatewayWithPolicyDefinitions(policies ...map[string]interface{}) *models.Gateway {
	items := make([]interface{}, 0, len(policies))
	for _, p := range policies {
		items = append(items, p)
	}
	return &models.Gateway{Manifest: map[string]interface{}{"policies": items}}
}

// -----------------------------------------------------------------------------
// extractGatewayPolicyDefinitions — the manifest walk itself.
// -----------------------------------------------------------------------------

func TestExtractGatewayPolicyDefinitions_FullFieldExtraction(t *testing.T) {
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

	items := extractGatewayPolicyDefinitions(manifest)

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

func TestExtractGatewayPolicyDefinitions_ToleratesKeyAliases(t *testing.T) {
	manifest := map[string]interface{}{
		"policies": []interface{}{
			map[string]interface{}{"policyName": "aliased-by-policyname", "version": "v1"},
			map[string]interface{}{"id": "aliased-by-id", "version": "v1"},
			map[string]interface{}{"name": "aliased-version", "policyVersion": "v2"},
		},
	}

	items := extractGatewayPolicyDefinitions(manifest)

	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, item.Name)
	}
	assert.ElementsMatch(t, []string{"aliased-by-policyname", "aliased-by-id", "aliased-version"}, names)
}

func TestExtractGatewayPolicyDefinitions_ExpandsVersionsArray(t *testing.T) {
	manifest := map[string]interface{}{
		"policies": []interface{}{
			map[string]interface{}{
				"name":     "multi-version-policy",
				"versions": []interface{}{"v1", "v2"},
			},
		},
	}

	items := extractGatewayPolicyDefinitions(manifest)

	versions := make([]string, 0, len(items))
	for _, item := range items {
		assert.Equal(t, "multi-version-policy", item.Name)
		versions = append(versions, item.Version)
	}
	assert.ElementsMatch(t, []string{"v1", "v2"}, versions)
}

func TestExtractGatewayPolicyDefinitions_IgnoresCoincidentalNameVersionInSchema(t *testing.T) {
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

	items := extractGatewayPolicyDefinitions(manifest)

	require.Len(t, items, 1)
	assert.Equal(t, "model-round-robin", items[0].Name)
	assert.Equal(t, "v1.0.2", items[0].Version)
}

func TestExtractGatewayPolicyDefinitions_EmptyOrMalformedManifest(t *testing.T) {
	assert.Empty(t, extractGatewayPolicyDefinitions(nil))
	assert.Empty(t, extractGatewayPolicyDefinitions(map[string]interface{}{}))
	assert.Empty(t, extractGatewayPolicyDefinitions("not a map"))
}

// -----------------------------------------------------------------------------
// intersectActiveGatewayPolicies — active-gateway intersection semantics.
// -----------------------------------------------------------------------------

func TestIntersectActiveGatewayPolicies_NilRepoReturnsEmpty(t *testing.T) {
	available, err := intersectActiveGatewayPolicies(nil, "org-uuid")

	require.NoError(t, err)
	assert.NotNil(t, available)
	assert.Empty(t, available)
}

func TestIntersectActiveGatewayPolicies_SingleGateway(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithPolicyDefinitions(map[string]interface{}{"name": "word-count-guardrail", "version": "v1"}),
			}, nil
		},
	}

	available, err := intersectActiveGatewayPolicies(repo, "org-uuid")

	require.NoError(t, err)
	require.Len(t, available, 1)
	assert.Contains(t, available, "word-count-guardrail\x00v1")
}

func TestIntersectActiveGatewayPolicies_OverlappingPoliciesSurvive(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithPolicyDefinitions(
					map[string]interface{}{"name": "shared-policy", "version": "v1"},
					map[string]interface{}{"name": "only-on-first", "version": "v1"},
				),
				gatewayWithPolicyDefinitions(
					map[string]interface{}{"name": "shared-policy", "version": "v1"},
				),
			}, nil
		},
	}

	available, err := intersectActiveGatewayPolicies(repo, "org-uuid")

	require.NoError(t, err)
	// Only the policy reported by EVERY active gateway survives the intersection.
	require.Len(t, available, 1)
	assert.Contains(t, available, "shared-policy\x00v1")
	assert.NotContains(t, available, "only-on-first\x00v1")
}

func TestIntersectActiveGatewayPolicies_VersionMismatchFallsBackToLowest(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				// Old gateway, not yet upgraded.
				gatewayWithPolicyDefinitions(
					map[string]interface{}{"name": "pii-filter", "version": "1.0.0"},
				),
				// New gateway, already upgraded — same policy, newer version.
				gatewayWithPolicyDefinitions(
					map[string]interface{}{"name": "pii-filter", "version": "1.1.0"},
				),
			}, nil
		},
	}

	available, err := intersectActiveGatewayPolicies(repo, "org-uuid")

	require.NoError(t, err)
	// Every gateway has the policy, just at different versions — treated as
	// backward-compatible: falls back to the lowest version instead of vanishing.
	require.Len(t, available, 1)
	assert.Contains(t, available, "pii-filter\x00"+"1.0.0")
	assert.NotContains(t, available, "pii-filter\x00"+"1.1.0")
}

func TestIntersectActiveGatewayPolicies_MissingFromOneGatewayStaysExcluded(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithPolicyDefinitions(
					map[string]interface{}{"name": "pii-filter", "version": "1.0.0"},
				),
				// Second gateway doesn't have this policy at all — not a version
				// mismatch, genuinely unsupported here. Must NOT be rescued by the
				// lowest-version fallback.
				gatewayWithPolicyDefinitions(),
			}, nil
		},
	}

	available, err := intersectActiveGatewayPolicies(repo, "org-uuid")

	require.NoError(t, err)
	assert.Empty(t, available)
}

func TestIntersectActiveGatewayPolicies_VersionMismatchUsesSemverComparison(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithPolicyDefinitions(
					map[string]interface{}{"name": "pii-filter", "version": "1.9.0"},
				),
				gatewayWithPolicyDefinitions(
					map[string]interface{}{"name": "pii-filter", "version": "1.10.0"},
				),
			}, nil
		},
	}

	available, err := intersectActiveGatewayPolicies(repo, "org-uuid")

	require.NoError(t, err)
	// A plain string compare would wrongly treat "1.10.0" < "1.9.0". Numeric
	// per-segment comparison must pick 1.9.0 as the lower version.
	require.Len(t, available, 1)
	assert.Contains(t, available, "pii-filter\x00"+"1.9.0")
}

func TestIntersectActiveGatewayPolicies_VersionMismatchStripsVPrefixForSemverComparison(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithPolicyDefinitions(
					map[string]interface{}{"name": "pii-filter", "version": "v2"},
				),
				gatewayWithPolicyDefinitions(
					map[string]interface{}{"name": "pii-filter", "version": "v10"},
				),
			}, nil
		},
	}

	available, err := intersectActiveGatewayPolicies(repo, "org-uuid")

	require.NoError(t, err)
	// A plain string compare of "v10" vs "v2" would wrongly rank "v10" lower.
	// Stripping the leading "v" before numeric comparison must pick v2 as lower.
	require.Len(t, available, 1)
	assert.Contains(t, available, "pii-filter\x00"+"v2")
}

func TestIntersectActiveGatewayPolicies_OnlyActiveGatewaysQueried(t *testing.T) {
	var capturedFilters repositories.GatewayFilterOptions
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(filters repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			capturedFilters = filters
			return []*models.Gateway{}, nil
		},
	}

	_, err := intersectActiveGatewayPolicies(repo, "org-uuid")

	require.NoError(t, err)
	assert.Equal(t, "org-uuid", capturedFilters.OrganizationID)
	require.NotNil(t, capturedFilters.Status)
	assert.True(t, *capturedFilters.Status)
}

func TestIntersectActiveGatewayPolicies_RepoErrorIsWrapped(t *testing.T) {
	boom := errors.New("db unreachable")
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return nil, boom
		},
	}

	_, err := intersectActiveGatewayPolicies(repo, "org-uuid")

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

// -----------------------------------------------------------------------------
// intersectDeployedGatewayPolicies — provider-scoped intersection, restricted to
// the gateways a specific provider is actually deployed to.
// -----------------------------------------------------------------------------

func TestIntersectDeployedGatewayPolicies_NilRepoReturnsEmpty(t *testing.T) {
	available, err := intersectDeployedGatewayPolicies(nil, nil, uuid.New(), "org-uuid")

	require.NoError(t, err)
	assert.Empty(t, available)
}

func TestIntersectDeployedGatewayPolicies_ScopesToDeployedGatewaysOnly(t *testing.T) {
	providerUUID := uuid.New()
	deployedGatewayID := "gw-1"
	undeployedGateway := gatewayWithPolicyDefinitions(
		map[string]interface{}{"name": "only-on-undeployed-gateway", "version": "v1"},
	)

	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(artifactUUID uuid.UUID, orgUUID string) ([]string, error) {
			assert.Equal(t, providerUUID, artifactUUID)
			assert.Equal(t, "org-uuid", orgUUID)
			return []string{deployedGatewayID}, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(gatewayId string) (*models.Gateway, error) {
			assert.Equal(t, deployedGatewayID, gatewayId)
			gw := gatewayWithPolicyDefinitions(
				map[string]interface{}{"name": "deployed-policy", "version": "v1"},
			)
			gw.OUID = "org-uuid"
			return gw, nil
		},
	}
	_ = undeployedGateway // never queried — GetByUUIDFunc only ever asked for the deployed gateway.

	available, err := intersectDeployedGatewayPolicies(gatewayRepo, deploymentRepo, providerUUID, "org-uuid")

	require.NoError(t, err)
	require.Len(t, available, 1)
	assert.Contains(t, available, "deployed-policy\x00v1")
}

func TestIntersectDeployedGatewayPolicies_SkipsGatewayFromAnotherOrg(t *testing.T) {
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(_ uuid.UUID, _ string) ([]string, error) {
			return []string{"gw-1"}, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
			gw := gatewayWithPolicyDefinitions(
				map[string]interface{}{"name": "cross-org-policy", "version": "v1"},
			)
			gw.OUID = "another-org"
			return gw, nil
		},
	}

	available, err := intersectDeployedGatewayPolicies(gatewayRepo, deploymentRepo, uuid.New(), "org-uuid")

	require.NoError(t, err)
	assert.Empty(t, available)
}

func TestIntersectDeployedGatewayPolicies_SkipsStaleGatewayReference(t *testing.T) {
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(_ uuid.UUID, _ string) ([]string, error) {
			return []string{"gw-deleted"}, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}

	available, err := intersectDeployedGatewayPolicies(gatewayRepo, deploymentRepo, uuid.New(), "org-uuid")

	require.NoError(t, err)
	assert.Empty(t, available)
}

func TestIntersectDeployedGatewayPolicies_DeploymentRepoErrorIsWrapped(t *testing.T) {
	boom := errors.New("db unreachable")
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(_ uuid.UUID, _ string) ([]string, error) {
			return nil, boom
		},
	}

	_, err := intersectDeployedGatewayPolicies(&repomocks.GatewayRepositoryMock{}, deploymentRepo, uuid.New(), "org-uuid")

	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
}

// -----------------------------------------------------------------------------
// sortedGatewayPolicyManifestItems — stable output ordering for the API response.
// -----------------------------------------------------------------------------

func TestSortedGatewayPolicyManifestItems_OrdersByNameThenVersion(t *testing.T) {
	available := map[string]gatewayPolicyManifestItem{
		"zebra-policy\x00v1": {Name: "zebra-policy", Version: "v1"},
		"alpha-policy\x00v2": {Name: "alpha-policy", Version: "v2"},
		"alpha-policy\x00v1": {Name: "alpha-policy", Version: "v1"},
	}

	sorted := sortedGatewayPolicyManifestItems(available)

	require.Len(t, sorted, 3)
	assert.Equal(t, "alpha-policy", sorted[0].Name)
	assert.Equal(t, "v1", sorted[0].Version)
	assert.Equal(t, "alpha-policy", sorted[1].Name)
	assert.Equal(t, "v2", sorted[1].Version)
	assert.Equal(t, "zebra-policy", sorted[2].Name)
}
