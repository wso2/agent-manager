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
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func externalRequest() AgentIdentityPromotionRequest {
	return AgentIdentityPromotionRequest{
		OUID: "org-id", ProjectName: "default", AgentName: "scout", DefaultEnvironment: "development",
		SourceEnvironment: "development", TargetEnvironment: "staging", RequestedBy: "user-1",
		Ownership: models.AgentProvisioningTypeExternal,
	}
}

func booker() AgentIdentityResources {
	return AgentIdentityResources{
		Roles:  []PromotedIdentityResource{{ID: "role-1", Name: "booker"}},
		Groups: []PromotedIdentityResource{{ID: "group-1", Name: "travel"}},
		MCPResourceServers: []MCPResourceServerPromotion{
			{ProxyHandle: "booking", DisplayName: "Booking", Identifier: "https://stage.example.test/booking"},
		},
	}
}

func withBooker(f *promotionFixture) {
	f.svc.SetAgentIdentityResourceResolver(resourceResolverFunc(func(context.Context, AgentIdentityResourceQuery) (AgentIdentityResources, error) {
		return booker(), nil
	}))
}

func TestExternalPromotionCreatesAnExternalBindingAndNeverStoresTheSecret(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	var carried AgentIdentityPromotion
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		carried = p
		return nil
	}))
	withBooker(f)

	result, err := f.svc.PromoteIdentityResources(auditableCtx(t), externalRequest())
	require.NoError(t, err)
	require.True(t, result.Promoted)
	require.False(t, result.NotApplicable)
	require.Equal(t, booker(), result.Resources)

	require.NotEmpty(t, carried.ClientSecret, "the carried client needs a secret value on the target")
	require.NotEmpty(t, carried.TargetClientID)
	require.NotEqual(t, "client-1", carried.TargetClientID, "the target gets its own client ID")
	require.Empty(t, f.store.CreateSecretCalls(), "an external agent's secret is never stored")
	require.Empty(t, f.store.PatchSecretCalls(), "an external agent's secret is never stored")

	require.Len(t, f.upserted, 1)
	binding := f.upserted[0]
	require.Equal(t, models.AgentProvisioningTypeExternal, binding.ProvisioningType)
	require.Equal(t, models.AgentThunderStatusCompleted, binding.Status)
	require.Empty(t, binding.SecretRefPath)
	require.Equal(t, carried.TargetClientID, binding.ThunderClientID)
	require.Equal(t, "staging", binding.EnvironmentName)
	require.Equal(t, "user-1", binding.RequestedBy)
}

func TestExternalPromotionResultAndLogsNeverContainTheSecret(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	var secret string
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		secret = p.ClientSecret
		return &IdentityPromotionError{Kind: IdentityPromotionConflict, Message: "target holds a different application"}
	}))
	var logs bytes.Buffer
	f.svc.logger = slog.New(slog.NewJSONHandler(&logs, nil))

	_, err := f.svc.PromoteIdentityResources(auditableCtx(t), externalRequest())
	require.Error(t, err)
	require.NotEmpty(t, secret)
	require.NotContains(t, logs.String(), secret)
	require.NotContains(t, err.Error(), secret)

	f = newPromotionFixture(t, stagingByPromotion())
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		secret = p.ClientSecret
		return nil
	}))
	result, err := f.svc.PromoteIdentityResources(auditableCtx(t), externalRequest())
	require.NoError(t, err)
	body, err := json.Marshal(result)
	require.NoError(t, err)
	require.NotContains(t, string(body), secret)
}

func TestExternalPromotionRepeatKeepsTheTargetClientIDAndPassesNoSecret(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.target = &models.AgentThunderClient{
		ID: f.source.ID, EnvironmentName: "staging", Status: models.AgentThunderStatusCompleted,
		ProvisioningType: models.AgentProvisioningTypeExternal, ThunderClientID: "staging-client",
	}
	var carried AgentIdentityPromotion
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		carried = p
		return nil
	}))

	result, err := f.svc.PromoteIdentityResources(auditableCtx(t), externalRequest())
	require.NoError(t, err)
	require.True(t, result.Promoted)
	require.Equal(t, "staging-client", carried.TargetClientID)
	require.Empty(t, carried.ClientSecret)
	require.Empty(t, f.upserted)
	require.Empty(t, f.updated)
	require.Empty(t, f.store.CreateSecretCalls())
}

func TestExternalPromotionIntoAnEnvironmentNotCreatedByPromotionIsNotApplicable(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	called := false
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error {
		called = true
		return nil
	}))
	req := externalRequest()
	req.TargetEnvironment = "production"
	result, err := f.svc.PromoteIdentityResources(auditableCtx(t), req)
	require.NoError(t, err)
	require.True(t, result.NotApplicable)
	require.False(t, result.Promoted)
	require.False(t, called)
	require.Empty(t, f.upserted)
}

func TestExternalPromotionRefusesToRunWithoutAnAuditRecorder(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	called := false
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error {
		called = true
		return nil
	}))
	_, err := f.svc.PromoteIdentityResources(context.Background(), externalRequest())
	require.ErrorIs(t, err, audit.ErrRecorderUnavailable)
	require.False(t, called)
	require.Empty(t, f.upserted)
}

func TestExternalPromotionRecordsTheOwnershipInTheAuditIntent(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error { return nil }))
	ctx, rec := recordingCtx(t)
	_, err := f.svc.PromoteIdentityResources(ctx, externalRequest())
	require.NoError(t, err)
	require.Equal(t, string(models.AgentProvisioningTypeExternal), rec.last().Details["provisioningType"])
	require.Equal(t, string(audit.ActionAgentIdentityPromote), string(rec.last().Action))
}

// --- Listing through the control plane ---

type readerManager struct {
	promotionManagerFunc
	list func(context.Context, AgentIdentityListQuery) (AgentIdentityListing, error)
}

func (m readerManager) ListAgentIdentityResources(ctx context.Context, q AgentIdentityListQuery) (AgentIdentityListing, error) {
	return m.list(ctx, q)
}

func noopManager() promotionManagerFunc {
	return func(context.Context, AgentIdentityPromotion) error { return nil }
}

// dpMustNotBeUsed makes any identity client resolution fail the test: an
// environment that receives its identity by promotion is never read directly.
func dpMustNotBeUsed(t *testing.T) placementResolver {
	t.Helper()
	return placementResolver{
		EnvThunderResolverMock: &clientmocks.EnvThunderResolverMock{
			ResolveIdentityFunc: func(_ context.Context, _, _, env string) (thundersvc.EnvIdentityClient, error) {
				// The first environment is where identity is managed and may be read.
				if env != "staging" {
					return &clientmocks.EnvIdentityClientMock{
						GetAgentRoleAssignmentsFunc: func(context.Context, string) (*thundersvc.AgentRoleAssignments, error) {
							return &thundersvc.AgentRoleAssignments{}, nil
						},
					}, nil
				}
				t.Fatal("the data plane must not be read for an environment created by promotion")
				return nil, nil
			},
		},
		byPromotion: map[string]bool{"staging": true},
	}
}

func completedStagingBinding(f *promotionFixture) {
	f.target = &models.AgentThunderClient{
		ID: f.source.ID, OUID: "org-id", ProjectName: "default", AgentName: "scout", EnvironmentName: "staging",
		ProvisioningType: models.AgentProvisioningTypeExternal, ThunderAgentID: "agent-stg", ThunderClientID: "staging-client",
		Status: models.AgentThunderStatusCompleted,
	}
}

func TestGetAgentRolesAndGroupsInAPromotedEnvironmentUseTheReader(t *testing.T) {
	f := newPromotionFixture(t, dpMustNotBeUsed(t))
	completedStagingBinding(f)
	f.svc.SetIdentityPromotionManager(readerManager{
		promotionManagerFunc: noopManager(),
		list: func(_ context.Context, q AgentIdentityListQuery) (AgentIdentityListing, error) {
			require.Equal(t, "agent-stg", q.ThunderAgentID)
			return AgentIdentityListing{
				Roles:  []PromotedIdentityResource{{ID: "role-1", Name: "booker"}},
				Groups: []PromotedIdentityResource{{ID: "group-1", Name: "travel"}},
			}, nil
		},
	})
	roles, err := f.svc.GetAgentRoles(context.Background(), "org-id", "default", "scout", "staging")
	require.NoError(t, err)
	require.Equal(t, []thundersvc.ThunderRole{{ID: "role-1", Name: "booker"}}, roles)
	groups, err := f.svc.GetAgentGroups(context.Background(), "org-id", "default", "scout", "staging")
	require.NoError(t, err)
	require.Equal(t, []thundersvc.ThunderGroup{{ID: "group-1", Name: "travel"}}, groups)

	f.svc.SetIdentityPromotionManager(noopManager())
	_, err = f.svc.GetAgentRoles(context.Background(), "org-id", "default", "scout", "staging")
	require.ErrorIs(t, err, thundersvc.ErrIdentityCapabilityUnavailable)
	_, err = f.svc.GetAgentGroups(context.Background(), "org-id", "default", "scout", "staging")
	require.ErrorIs(t, err, thundersvc.ErrIdentityCapabilityUnavailable)
}
