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
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// -----------------------------------------------------------------------------
// ListAvailableLLMPolicies — surfaces full gateway-reported guardrail definitions
// for the console, replacing the external policy-hub catalog for LLM guardrails.
// -----------------------------------------------------------------------------

func TestLLMProviderService_ListAvailableLLMPolicies_NilGatewayRepoReturnsEmpty(t *testing.T) {
	svc := &LLMProviderService{}

	resp, err := svc.ListAvailableLLMPolicies(context.Background(), "org-uuid", "")

	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Equal(t, int32(0), resp.Count)
	assert.NotNil(t, resp.List)
	assert.Empty(t, resp.List)
}

func TestLLMProviderService_ListAvailableLLMPolicies_SurfacesFullDefinitions(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithLLMPolicyManifest(map[string]interface{}{
					"name":        "word-count-guardrail",
					"version":     "v1.0.0",
					"displayName": "Word Count Guardrail",
					"description": "Validates word count.",
					"parameters": map[string]interface{}{
						"type": "object",
					},
				}),
			}, nil
		},
	}
	svc := &LLMProviderService{gatewayRepo: repo}

	resp, err := svc.ListAvailableLLMPolicies(context.Background(), "org-uuid", "")

	require.NoError(t, err)
	require.Equal(t, int32(1), resp.Count)
	require.Len(t, resp.List, 1)

	def := resp.List[0]
	assert.Equal(t, "word-count-guardrail", def.Name)
	assert.Equal(t, "v1.0.0", def.Version)
	assert.Equal(t, "Word Count Guardrail", def.DisplayName)
	assert.Equal(t, "Validates word count.", def.Description)
	require.NotNil(t, def.Parameters)
	assert.Equal(t, "object", def.Parameters["type"])
}

func TestLLMProviderService_ListAvailableLLMPolicies_IntersectsAcrossActiveGateways(t *testing.T) {
	repo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(_ repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "shared-guardrail", "version": "v1"},
					map[string]interface{}{"name": "only-on-first-gateway", "version": "v1"},
				),
				gatewayWithLLMPolicyManifest(
					map[string]interface{}{"name": "shared-guardrail", "version": "v1"},
				),
			}, nil
		},
	}
	svc := &LLMProviderService{gatewayRepo: repo}

	resp, err := svc.ListAvailableLLMPolicies(context.Background(), "org-uuid", "")

	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "shared-guardrail", resp.List[0].Name)
}

func TestLLMProviderService_ListAvailableLLMPolicies_ScopesToProviderDeployment(t *testing.T) {
	providerUUID := uuid.New()
	provider := &models.LLMProvider{UUID: providerUUID}

	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(_ string, _ string) (*models.LLMProvider, error) {
			return provider, nil
		},
	}
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(artifactUUID uuid.UUID, orgUUID string) ([]string, error) {
			assert.Equal(t, providerUUID, artifactUUID)
			return []string{"gw-1"}, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(_ string) (*models.Gateway, error) {
			gw := gatewayWithLLMPolicyManifest(
				map[string]interface{}{"name": "deployed-policy", "version": "v1"},
			)
			gw.OUID = "org-uuid"
			return gw, nil
		},
	}
	svc := &LLMProviderService{providerRepo: providerRepo, gatewayRepo: gatewayRepo, deploymentRepo: deploymentRepo}

	resp, err := svc.ListAvailableLLMPolicies(context.Background(), "org-uuid", providerUUID.String())

	require.NoError(t, err)
	require.Len(t, resp.List, 1)
	assert.Equal(t, "deployed-policy", resp.List[0].Name)
}

func TestLLMProviderService_ListAvailableLLMPolicies_UnknownProviderReturnsNotFound(t *testing.T) {
	providerRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(_ string, _ string) (*models.LLMProvider, error) {
			return nil, gorm.ErrRecordNotFound
		},
	}
	svc := &LLMProviderService{providerRepo: providerRepo}

	_, err := svc.ListAvailableLLMPolicies(context.Background(), "org-uuid", uuid.New().String())

	assert.ErrorIs(t, err, utils.ErrLLMProviderNotFound)
}

// -----------------------------------------------------------------------------
// CreateAndDeploy — egress placement is validated per requested gateway before the
// provider is created. A placement failure must hard-fail the whole request (not be
// recorded as a per-gateway DeploymentResult and skipped): naming an invalid gateway is
// caller error, and since this check runs before s.Create, failing fast leaves no
// partial state. providerRepo is left nil so that a regression which reaches s.Create
// anyway panics loudly instead of silently succeeding.
// -----------------------------------------------------------------------------

func TestLLMProviderService_CreateAndDeploy_RejectsIngressOnlyGateway(t *testing.T) {
	ingress := newGateway(t, models.GatewayRoleIngress, true)
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(id string) (*models.Gateway, error) {
			require.Equal(t, ingress.UUID.String(), id)
			return ingress, nil
		},
	}
	createCalled := false
	providerRepo := &repomocks.LLMProviderRepositoryMock{
		CreateFunc: func(_ *gorm.DB, _ *models.LLMProvider, _, _, _, _ string) error {
			createCalled = true
			return nil
		},
	}
	svc := &LLMProviderService{gatewayRepo: repo, providerRepo: providerRepo}

	_, err := svc.CreateAndDeploy(context.Background(), "org", "creator",
		&models.LLMProvider{}, []string{ingress.UUID.String()}, nil)

	require.Error(t, err)
	assert.True(t, errors.Is(err, utils.ErrInvalidInput), "expected error to wrap utils.ErrInvalidInput, got: %v", err)
	assert.False(t, createCalled, "CreateAndDeploy must not create the provider when the sole named gateway fails placement")
}

func TestLLMProviderService_CreateAndDeploy_TreatsForeignOrgGatewayAsNotFound(t *testing.T) {
	// GetByUUID is not org-scoped, so a caller naming another org's gateway UUID gets a
	// real row back. It must be treated exactly like not-found — no placement validation,
	// no echoing the foreign gateway's name in any error.
	foreign := newGateway(t, models.GatewayRoleIngress, true)
	foreign.OUID = "other-org"
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(id string) (*models.Gateway, error) {
			require.Equal(t, foreign.UUID.String(), id)
			return foreign, nil
		},
	}
	createCalled := false
	providerRepo := &repomocks.LLMProviderRepositoryMock{
		CreateFunc: func(_ *gorm.DB, _ *models.LLMProvider, _, _, _, _ string) error {
			createCalled = true
			return nil
		},
	}
	svc := &LLMProviderService{gatewayRepo: repo, providerRepo: providerRepo}

	_, err := svc.CreateAndDeploy(context.Background(), "org", "creator",
		&models.LLMProvider{}, []string{foreign.UUID.String()}, nil)

	require.Error(t, err, "sole requested gateway is invalid, so the request must fail")
	assert.NotContains(t, err.Error(), foreign.Name, "foreign gateway name must not leak into the error")
	assert.False(t, createCalled, "CreateAndDeploy must not create the provider when the sole named gateway is foreign")
}

func TestLLMProviderService_CreateAndDeploy_RejectsSameEnvironmentClashWithinRequest(t *testing.T) {
	// Two ingress-only gateways requested in the same call: both fail on the role check
	// alone (independent of each other). The first failure must hard-fail the whole
	// request immediately. The same-environment-clash branch of validateEgressPlacement
	// itself (one gateway succeeding, the next rejected as placement-fixed) is covered
	// directly by TestValidateEgressPlacement in gateway_roles_unit_test.go.
	first := newGateway(t, models.GatewayRoleIngress, true)
	second := newGateway(t, models.GatewayRoleIngress, true)
	repo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(id string) (*models.Gateway, error) {
			switch id {
			case first.UUID.String():
				return first, nil
			case second.UUID.String():
				return second, nil
			default:
				return nil, gorm.ErrRecordNotFound
			}
		},
	}
	createCalled := false
	providerRepo := &repomocks.LLMProviderRepositoryMock{
		CreateFunc: func(_ *gorm.DB, _ *models.LLMProvider, _, _, _, _ string) error {
			createCalled = true
			return nil
		},
	}
	svc := &LLMProviderService{gatewayRepo: repo, providerRepo: providerRepo}

	_, err := svc.CreateAndDeploy(context.Background(), "org", "creator",
		&models.LLMProvider{}, []string{first.UUID.String(), second.UUID.String()}, nil)

	require.Error(t, err)
	assert.True(t, errors.Is(err, utils.ErrInvalidInput), "expected error to wrap utils.ErrInvalidInput, got: %v", err)
	assert.False(t, createCalled, "CreateAndDeploy must not create the provider when a named gateway fails placement")
}

// -----------------------------------------------------------------------------
// UpdateAndSync — same hard-fail requirement, with a sharper failure mode: the
// deploy/undeploy loops derive removals from the requested gateway set, so silently
// dropping a placement-invalid gateway (instead of hard-failing) would shrink that set
// and undeploy every currently-working gateway not re-named in the request. This test
// pins down that naming only a placement-invalid gateway B, while the provider is
// already deployed on gateway A, must fail the whole call and never touch A.
// -----------------------------------------------------------------------------

func TestLLMProviderService_UpdateAndSync_RejectsPlacementInvalidGatewayWithoutUndeployingExisting(t *testing.T) {
	gatewayA := uuid.New()
	gatewayB := newGateway(t, models.GatewayRoleIngress, true) // fails the role check
	providerUUID := uuid.New()

	outerProviderRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(id, _ string) (*models.LLMProvider, error) {
			return &models.LLMProvider{UUID: uuid.MustParse(id)}, nil
		},
		UpdateFunc: func(_ context.Context, _ *models.LLMProvider, _, _ string) error {
			return nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		GetByUUIDFunc: func(id string) (*models.Gateway, error) {
			require.Equal(t, gatewayB.UUID.String(), id)
			return gatewayB, nil
		},
	}
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(_ uuid.UUID, _ string) ([]string, error) {
			return []string{gatewayA.String()}, nil
		},
	}
	// If the undeploy (or deploy) path is reached at all, it resolves the provider again
	// through the deploymentService's own providerRepo — fatal here proves the hard-fail
	// short-circuits before that loop runs, i.e. gateway A is never touched.
	undeployPathReached := false
	deploymentProviderRepo := &repomocks.LLMProviderRepositoryMock{
		GetByUUIDFunc: func(_, _ string) (*models.LLMProvider, error) {
			undeployPathReached = true
			return nil, errors.New("deploy/undeploy path should not be reached")
		},
	}
	deploymentSvc := &LLMProviderDeploymentService{
		deploymentRepo: deploymentRepo,
		providerRepo:   deploymentProviderRepo,
		gatewayRepo:    gatewayRepo,
	}
	svc := &LLMProviderService{providerRepo: outerProviderRepo, gatewayRepo: gatewayRepo}

	_, err := svc.UpdateAndSync(context.Background(), providerUUID.String(), "org",
		&models.LLMProvider{}, []string{gatewayB.UUID.String()}, deploymentSvc)

	require.Error(t, err)
	assert.True(t, errors.Is(err, utils.ErrInvalidInput), "expected error to wrap utils.ErrInvalidInput, got: %v", err)
	assert.False(t, undeployPathReached, "UpdateAndSync must not attempt to deploy/undeploy any gateway once placement validation hard-fails")
}
