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
	"log/slog"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

type placementResolver struct {
	*clientmocks.EnvThunderResolverMock
	byPromotion map[string]bool
	err         error
}

func (r placementResolver) IdentityCreatedByPromotion(_ context.Context, _, _, env string) (bool, error) {
	return r.byPromotion[env], r.err
}

// ResolveIdentity answers with the mock's own function when a test set one, and
// otherwise with an identity client on which nobody holds any role, which is all a
// promotion that carries roles asks of it.
func (r placementResolver) ResolveIdentity(ctx context.Context, ouID, namespace, env string) (thundersvc.EnvIdentityClient, error) {
	if r.ResolveIdentityFunc != nil {
		return r.EnvThunderResolverMock.ResolveIdentity(ctx, ouID, namespace, env)
	}
	return &clientmocks.EnvIdentityClientMock{
		GetAgentRoleAssignmentsFunc: func(context.Context, string) (*thundersvc.AgentRoleAssignments, error) {
			return &thundersvc.AgentRoleAssignments{}, nil
		},
	}, nil
}

// promoteIdentity carries an internal agent's identity with sourceEnv as the
// default environment, the way the reconciler's promotion does.
func promoteIdentity(svc *agentThunderProvisioningService, ctx context.Context, ouID, projectName, agentName, sourceEnv, targetEnv, requestedBy string,
) (bool, error) {
	return promoteIdentityInPipeline(svc, ctx, ouID, projectName, agentName, sourceEnv, sourceEnv, targetEnv, requestedBy)
}

func promoteIdentityInPipeline(svc *agentThunderProvisioningService, ctx context.Context, ouID, projectName, agentName, defaultEnv, sourceEnv, targetEnv, requestedBy string,
) (bool, error) {
	result, err := svc.PromoteIdentityResources(ctx, AgentIdentityPromotionRequest{
		OUID: ouID, ProjectName: projectName, AgentName: agentName, DefaultEnvironment: defaultEnv,
		SourceEnvironment: sourceEnv, TargetEnvironment: targetEnv, RequestedBy: requestedBy,
		Ownership: models.AgentProvisioningTypeInternal,
	})
	return result.Promoted, err
}

type promotionManagerFunc func(context.Context, AgentIdentityPromotion) error

func (f promotionManagerFunc) PromoteAgentIdentity(ctx context.Context, p AgentIdentityPromotion) error {
	return f(ctx, p)
}

type promotionFixture struct {
	svc      *agentThunderProvisioningService
	repo     *repomocks.AgentThunderClientRepositoryMock
	store    *clientmocks.SecretManagementClientMock
	upserted []models.AgentThunderClient
	updated  []repositories.AgentThunderAttemptUpdate
	events   []string
	target   *models.AgentThunderClient
	source   *models.AgentThunderClient
	// targetRows are the bindings the target environment holds for every agent.
	targetRows []models.AgentThunderClient
}

func newPromotionFixture(t *testing.T, resolver placementResolver) *promotionFixture {
	t.Helper()
	f := &promotionFixture{
		source: &models.AgentThunderClient{
			ID: uuid.New(), OUID: "org-id", ProjectName: "default", AgentName: "scout", EnvironmentName: "development",
			ProvisioningType: models.AgentProvisioningTypeInternal, ThunderAgentID: "agent-1", ThunderClientID: "client-1",
			Status: models.AgentThunderStatusCompleted,
		},
	}
	f.repo = &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(_ context.Context, _, _, _, env string) (*models.AgentThunderClient, error) {
			if env == "development" {
				return f.source, nil
			}
			if f.target != nil {
				return f.target, nil
			}
			return nil, repositories.ErrAgentThunderClientNotFound
		},
		FindByOuAndEnvironmentFunc: func(context.Context, string, string) ([]models.AgentThunderClient, error) {
			return f.targetRows, nil
		},
		UpsertFunc: func(_ context.Context, c *models.AgentThunderClient) error {
			f.upserted = append(f.upserted, *c)
			return nil
		},
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, u repositories.AgentThunderAttemptUpdate) error {
			f.updated = append(f.updated, u)
			return nil
		},
	}
	f.store = &clientmocks.SecretManagementClientMock{
		PatchSecretFunc: patchSecretNotFound,
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			f.events = append(f.events, "store")
			return "stored-reference", nil
		},
	}
	f.svc = NewAgentThunderProvisioningService(f.repo, resolver, f.store, nil, slog.Default()).(*agentThunderProvisioningService)
	return f
}

func stagingByPromotion() placementResolver {
	return placementResolver{
		EnvThunderResolverMock: &clientmocks.EnvThunderResolverMock{},
		byPromotion:            map[string]bool{"staging": true},
	}
}

func TestPromoteIdentitySetsTargetCredentialsBeforeStoringThem(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	var carried AgentIdentityPromotion
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		f.events = append(f.events, "promote")
		carried = p
		return nil
	}))

	promoted, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.True(t, promoted)
	require.Equal(t, []string{"promote", "store"}, f.events, "the target must hold the credentials before they are stored")
	require.Equal(t, "agent-1", carried.ThunderAgentID)
	require.Equal(t, "staging", carried.TargetEnvironment)
	require.NotEmpty(t, carried.TargetClientID)
	require.NotEqual(t, "client-1", carried.TargetClientID, "the target gets its own client ID, never the source's")
	require.NotEmpty(t, carried.ClientSecret)

	require.Len(t, f.store.CreateSecretCalls(), 1)
	call := f.store.CreateSecretCalls()[0]
	require.Equal(t, carried.ClientSecret, call.Data[thundersvc.AgentSecretKeyClientSecret], "the stored secret is the one the target was given")
	require.Equal(t, carried.TargetClientID, call.Data[thundersvc.AgentSecretKeyClientID])

	require.Len(t, f.upserted, 1)
	binding := f.upserted[0]
	require.Equal(t, "staging", binding.EnvironmentName)
	require.Equal(t, models.AgentThunderStatusCompleted, binding.Status)
	require.Equal(t, "agent-1", binding.ThunderAgentID)
	require.Equal(t, carried.TargetClientID, binding.ThunderClientID, "the binding records the target's own client ID")
	require.Equal(t, "stored-reference", binding.SecretRefPath)
	require.Equal(t, "user-1", binding.RequestedBy)
}

func TestPromoteIdentityKeepsCredentialsOfAnAlreadyPromotedTarget(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.target = &models.AgentThunderClient{
		ID: uuid.New(), EnvironmentName: "staging", Status: models.AgentThunderStatusCompleted,
		ThunderClientID: "staging-client", SecretRefPath: "existing-reference",
	}
	var carried AgentIdentityPromotion
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		carried = p
		return nil
	}))

	promoted, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.True(t, promoted)
	require.Equal(t, "staging-client", carried.TargetClientID, "a repeated promotion keeps the target's client ID")
	require.Empty(t, carried.ClientSecret, "an empty secret tells the manager to keep the target's own")
	require.Empty(t, f.store.CreateSecretCalls(), "the running agent's stored secret must not change")
	require.Empty(t, f.upserted)
	require.Empty(t, f.updated)
}

func TestPromoteIdentityKeepsARevokedCredentialRevoked(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.target = &models.AgentThunderClient{
		ID: uuid.New(), EnvironmentName: "staging", Status: models.AgentThunderStatusCompleted,
		ThunderClientID: "staging-client", SecretRefPath: "",
	}
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		require.Empty(t, p.ClientSecret)
		return nil
	}))
	_, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.Empty(t, f.store.CreateSecretCalls(), "promotion must not undo a revoke")
}

type mcpResolverFunc func(context.Context, string, string, string, string) ([]MCPResourceServerPromotion, error)

func (f mcpResolverFunc) ResolveAgentMCPResourceServers(ctx context.Context, ouID, project, agent, env string) ([]MCPResourceServerPromotion, error) {
	return f(ctx, ouID, project, agent, env)
}

func TestPromoteIdentityCarriesMCPResourceServersWithTargetIdentifiers(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	var carried AgentIdentityPromotion
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		carried = p
		return nil
	}))
	f.svc.SetAgentMCPResourceServerResolver(mcpResolverFunc(func(_ context.Context, _, _, agent, env string) ([]MCPResourceServerPromotion, error) {
		require.Equal(t, "scout", agent)
		require.Equal(t, "staging", env, "identifiers are resolved for the TARGET environment")
		return []MCPResourceServerPromotion{{ProxyHandle: "booking", DisplayName: "Booking", Identifier: "https://stage.example.test/booking"}}, nil
	}))
	_, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.Equal(t, []MCPResourceServerPromotion{{ProxyHandle: "booking", DisplayName: "Booking", Identifier: "https://stage.example.test/booking"}}, carried.MCPResourceServers)
}

func TestPromoteIdentityFailsBeforeIssuingCredentialsWhenResourceServersCannotBeResolved(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	called := false
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error {
		called = true
		return nil
	}))
	f.svc.SetAgentMCPResourceServerResolver(mcpResolverFunc(func(context.Context, string, string, string, string) ([]MCPResourceServerPromotion, error) {
		return nil, errors.New("gateway lookup failed")
	}))
	_, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.ErrorContains(t, err, "gateway lookup failed")
	require.False(t, called)
	require.Empty(t, f.store.CreateSecretCalls())
}

func TestProvisionForEnvironmentIfMissingExplainsPromotionCreatedEnvironments(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	_, err := f.svc.ProvisionForEnvironmentIfMissing(context.Background(), "org-id", "default", "scout", "staging",
		models.AgentProvisioningTypeExternal, "user-1")
	require.ErrorIs(t, err, thundersvc.ErrIdentityEnvironmentUnsupported)
	require.Empty(t, f.upserted, "no binding is written for an environment that receives identity by promotion")
}

func TestPromoteIdentityNeverReusesTheSourceSecret(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	seen := map[string]bool{}
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		require.False(t, seen[p.ClientSecret], "each promotion must use a fresh secret")
		seen[p.ClientSecret] = true
		return nil
	}))
	for range 2 {
		_, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
		require.NoError(t, err)
	}
	require.Len(t, seen, 2)
}

func TestPromoteIdentityUpdatesExistingTargetBinding(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.target = &models.AgentThunderClient{ID: uuid.New(), EnvironmentName: "staging", Status: models.AgentThunderStatusFailed}
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error { return nil }))

	promoted, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.True(t, promoted)
	require.Empty(t, f.upserted, "an existing binding is updated, never replaced")
	require.Len(t, f.updated, 1)
	require.Equal(t, models.AgentThunderStatusCompleted, f.updated[0].Status)
	require.Equal(t, "stored-reference", *f.updated[0].SecretRefPath)
}

func TestPromoteIdentityStoresNothingWhenPromotionFails(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error {
		return errors.New("control plane unavailable")
	}))
	promoted, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.ErrorContains(t, err, "control plane unavailable")
	require.False(t, promoted)
	require.Empty(t, f.store.CreateSecretCalls())
	require.Empty(t, f.upserted)
}

func TestPromoteIdentitySkipsEnvironmentsNotCreatedByPromotion(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	called := false
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error {
		called = true
		return nil
	}))
	promoted, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "production", "user-1")
	require.NoError(t, err)
	require.False(t, promoted)
	require.False(t, called)
	require.Empty(t, f.store.CreateSecretCalls())
}

func TestPromoteIdentityNotApplicableChangesNothing(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error {
		return ErrIdentityPromotionNotApplicable
	}))
	promoted, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.False(t, promoted)
	require.Empty(t, f.store.CreateSecretCalls())
	require.Empty(t, f.upserted)
}

func TestPromoteIdentityRequiresCompletedSourceIdentity(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.source.Status = models.AgentThunderStatusPending
	called := false
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error {
		called = true
		return nil
	}))
	_, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.Error(t, err)
	require.False(t, called)
}

func TestPromoteIdentityFailsClosedWithoutManagerOrAuditRecorder(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	_, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.ErrorIs(t, err, thundersvc.ErrIdentityCapabilityUnavailable)

	called := false
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error {
		called = true
		return nil
	}))
	_, err = promoteIdentity(f.svc, context.Background(), "org-id", "default", "scout", "development", "staging", "user-1")
	require.ErrorIs(t, err, audit.ErrRecorderUnavailable)
	require.False(t, called, "a credential must not be issued when the intent cannot be recorded")
}

func TestProvisionForAgentSkipsEnvironmentsCreatedByPromotion(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.repo.ClaimForAttemptFunc = func(context.Context, uuid.UUID) (bool, error) { return false, nil }
	f.svc.ProvisionForAgent(context.Background(), "org-id", "default", "scout", models.AgentProvisioningTypeInternal,
		[]string{"development", "staging"}, "user-1", "")
	require.Len(t, f.upserted, 1)
	require.Equal(t, "development", f.upserted[0].EnvironmentName)
}

func TestProvisionForAgentStillWritesBindingWhenPlacementLookupFails(t *testing.T) {
	resolver := stagingByPromotion()
	resolver.err = errors.New("target resolver unavailable")
	f := newPromotionFixture(t, resolver)
	f.repo.ClaimForAttemptFunc = func(context.Context, uuid.UUID) (bool, error) { return false, nil }
	f.svc.ProvisionForAgent(context.Background(), "org-id", "default", "scout", models.AgentProvisioningTypeInternal,
		[]string{"development", "staging"}, "user-1", "")
	require.Len(t, f.upserted, 2, "the row is what the reconciler heals from")
}

// rotatingManager is a promotion manager that can also rotate a secret.
type rotatingManager struct {
	promotionManagerFunc
	rotate func(context.Context, AgentSecretRotation) error
}

func (m rotatingManager) RotateAgentSecret(ctx context.Context, r AgentSecretRotation) error {
	return m.rotate(ctx, r)
}

type rotationFixture struct {
	*promotionFixture
	rotations []AgentSecretRotation
	patched   []map[string]string
	deleted   int
	secretRef string
	binding   *models.AgentThunderClient
}

func newRotationFixture(t *testing.T, provisioningType models.AgentProvisioningType) *rotationFixture {
	t.Helper()
	r := &rotationFixture{promotionFixture: newPromotionFixture(t, stagingByPromotion())}
	r.binding = &models.AgentThunderClient{
		ID: uuid.New(), OUID: "org-id", ProjectName: "default", AgentName: "scout", EnvironmentName: "staging",
		ProvisioningType: provisioningType, ThunderAgentID: "agent-1", ThunderClientID: "staging-client",
		SecretRefPath: "existing-reference", Status: models.AgentThunderStatusCompleted,
	}
	r.repo.GetFunc = func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
		return r.binding, nil
	}
	r.repo.UpdateSecretRefFunc = func(_ context.Context, _ uuid.UUID, ref string) error { r.secretRef = ref; return nil }
	r.store.PatchSecretFunc = func(_ context.Context, _ secretmanagersvc.SecretLocation, data map[string]string, _ []string) (string, error) {
		r.patched = append(r.patched, data)
		return "patched-reference", nil
	}
	r.store.DeleteSecretFunc = func(context.Context, secretmanagersvc.SecretLocation, string) error { r.deleted++; return nil }
	r.svc.SetIdentityPromotionManager(rotatingManager{
		promotionManagerFunc: func(context.Context, AgentIdentityPromotion) error { return nil },
		rotate: func(_ context.Context, rotation AgentSecretRotation) error {
			r.rotations = append(r.rotations, rotation)
			return nil
		},
	})
	return r
}

func TestRegenerateSecretInPromotedEnvironmentRotatesThroughTheManagerAndPatchesTheStoredSecret(t *testing.T) {
	r := newRotationFixture(t, models.AgentProvisioningTypeInternal)

	ownership, clientID, secret, err := r.svc.RegenerateSecret(context.Background(), "org-id", "default", "scout", "staging")
	require.NoError(t, err)
	require.Equal(t, models.AgentProvisioningTypeInternal, ownership)
	require.Equal(t, "staging-client", clientID)
	require.Len(t, r.rotations, 1)
	require.Equal(t, "agent-1", r.rotations[0].ThunderAgentID)
	require.Equal(t, "staging", r.rotations[0].Environment)
	require.Equal(t, secret, r.rotations[0].ClientSecret, "the environment holds the secret that is returned and stored")
	require.Len(t, r.patched, 1)
	require.Equal(t, secret, r.patched[0][thundersvc.AgentSecretKeyClientSecret])
	require.Empty(t, r.store.CreateSecretCalls(), "an existing stored secret is patched, never pushed again")
	require.Equal(t, "patched-reference", r.secretRef)
}

func TestRegenerateSecretInPromotedEnvironmentNeverStoresAnExternalAgentSecret(t *testing.T) {
	r := newRotationFixture(t, models.AgentProvisioningTypeExternal)

	_, _, secret, err := r.svc.RegenerateSecret(context.Background(), "org-id", "default", "scout", "staging")
	require.NoError(t, err)
	require.NotEmpty(t, secret, "the caller receives the new secret once")
	require.Len(t, r.rotations, 1)
	require.Empty(t, r.patched)
	require.Empty(t, r.store.CreateSecretCalls())
	require.Empty(t, r.secretRef)
}

func TestRegenerateSecretInPromotedEnvironmentFailsClosedWithoutARotator(t *testing.T) {
	r := newRotationFixture(t, models.AgentProvisioningTypeInternal)
	r.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error { return nil }))

	_, _, _, err := r.svc.RegenerateSecret(context.Background(), "org-id", "default", "scout", "staging")
	require.ErrorIs(t, err, thundersvc.ErrIdentityCapabilityUnavailable)
	require.Empty(t, r.patched)
}

func TestRegenerateSecretKeepsOldSecretWhenTheEnvironmentRefusesTheRotation(t *testing.T) {
	r := newRotationFixture(t, models.AgentProvisioningTypeInternal)
	r.svc.SetIdentityPromotionManager(rotatingManager{
		promotionManagerFunc: func(context.Context, AgentIdentityPromotion) error { return nil },
		rotate:               func(context.Context, AgentSecretRotation) error { return errors.New("apply refused") },
	})
	_, _, _, err := r.svc.RegenerateSecret(context.Background(), "org-id", "default", "scout", "staging")
	require.ErrorContains(t, err, "apply refused")
	require.Empty(t, r.patched, "the stored secret still matches what the environment holds")
}

func TestRegenerateSecretPatchesAnExistingSecretInTheDefaultEnvironmentToo(t *testing.T) {
	tc := fakeThunderClientMock()
	tc.RegenerateAgentSecretFunc = func(context.Context, string) (string, error) { return "new-secret", nil }
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(context.Context, string, string, string) (thundersvc.ThunderClient, error) { return tc, nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		PatchSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string, []string) (string, error) {
			return "same-reference", nil
		},
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{
				ID: uuid.New(), ThunderAgentID: "agent-1", ThunderClientID: "client-1",
				ProvisioningType: models.AgentProvisioningTypeInternal, SecretRefPath: "same-reference",
			}, nil
		},
		UpdateSecretRefFunc: func(context.Context, uuid.UUID, string) error { return nil },
	}
	svc := newTestProvisioningService(repo, resolver, store)
	_, _, _, err := svc.RegenerateSecret(context.Background(), "org-id", "default", "scout", "development")
	require.NoError(t, err)
	require.Empty(t, store.CreateSecretCalls(), "pushing again would leave the previous secret behind on SaaS")
}

func TestRevokeSecretInPromotedEnvironmentRotatesToAnUnknownSecretAndDeletesTheStoredCopy(t *testing.T) {
	r := newRotationFixture(t, models.AgentProvisioningTypeInternal)

	clientID, err := r.svc.RevokeSecret(context.Background(), "org-id", "default", "scout", "staging")
	require.NoError(t, err)
	require.Equal(t, "staging-client", clientID)
	require.Len(t, r.rotations, 1)
	require.NotEmpty(t, r.rotations[0].ClientSecret)
	require.Empty(t, r.patched, "the throwaway secret is never stored")
	require.Equal(t, 1, r.deleted)
	require.Equal(t, "", r.secretRef)
}
