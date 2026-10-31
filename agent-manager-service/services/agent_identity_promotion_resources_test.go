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
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

type resourceResolverFunc func(context.Context, AgentIdentityResourceQuery) (AgentIdentityResources, error)

func (f resourceResolverFunc) ResolveAgentIdentityResources(ctx context.Context, q AgentIdentityResourceQuery) (AgentIdentityResources, error) {
	return f(ctx, q)
}

// previewingManager is a promotion manager with the optional preview capability.
type previewingManager struct {
	promotionManagerFunc
	preview func(context.Context, AgentIdentityPromotion) error
}

func (m previewingManager) PreviewAgentIdentity(ctx context.Context, p AgentIdentityPromotion) error {
	return m.preview(ctx, p)
}

// capturingRecorder keeps every audit event so a test can read the outcome.
type capturingRecorder struct {
	mu     sync.Mutex
	events []audit.Event
}

func (r *capturingRecorder) Record(_ context.Context, e audit.Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.events = append(r.events, e)
}

func (r *capturingRecorder) RecordSync(_ context.Context, e audit.Event) error {
	r.Record(context.Background(), e)
	return nil
}

func (r *capturingRecorder) Close(context.Context) error { return nil }

func (r *capturingRecorder) outcomes() []audit.Outcome {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.Outcome, 0, len(r.events))
	for _, e := range r.events {
		out = append(out, e.Outcome)
	}
	return out
}

func (r *capturingRecorder) last() audit.Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.events[len(r.events)-1]
}

func recordingCtx(t *testing.T) (context.Context, *capturingRecorder) {
	t.Helper()
	rec := &capturingRecorder{}
	return audit.WithRecorder(context.Background(), rec), rec
}

func TestPromoteIdentityCarriesRolesGroupsAndProjectFromTheDefaultEnvironment(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	var carried AgentIdentityPromotion
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		carried = p
		return nil
	}))
	var query AgentIdentityResourceQuery
	f.svc.SetAgentIdentityResourceResolver(resourceResolverFunc(func(_ context.Context, q AgentIdentityResourceQuery) (AgentIdentityResources, error) {
		query = q
		return AgentIdentityResources{
			Roles:  []PromotedIdentityResource{{ID: "role-1", Name: "booker"}},
			Groups: []PromotedIdentityResource{{ID: "group-1", Name: "travel"}},
		}, nil
	}))

	promoted, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.True(t, promoted)
	require.Equal(t, []PromotedIdentityResource{{ID: "role-1", Name: "booker"}}, carried.Roles)
	require.Equal(t, []PromotedIdentityResource{{ID: "group-1", Name: "travel"}}, carried.Groups)
	require.Equal(t, "default", carried.ProjectName)
	require.Equal(t, "agent-1", query.ThunderAgentID)
	require.Equal(t, "development", query.DefaultEnvironment, "the legacy signature treats the source as the default environment")
	require.Equal(t, "staging", query.TargetEnvironment)
	require.Equal(t, "default", query.ProjectName)
}

func TestPromoteIdentityInPipelineReadsRolesAndGroupsFromTheLowestEnvironment(t *testing.T) {
	f := newPromotionFixture(t, placementResolver{
		EnvThunderResolverMock: &clientmocks.EnvThunderResolverMock{},
		byPromotion:            map[string]bool{"production": true},
	})
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error { return nil }))
	var query AgentIdentityResourceQuery
	f.svc.SetAgentIdentityResourceResolver(resourceResolverFunc(func(_ context.Context, q AgentIdentityResourceQuery) (AgentIdentityResources, error) {
		query = q
		return AgentIdentityResources{}, nil
	}))

	_, err := promoteIdentityInPipeline(f.svc, auditableCtx(t), "org-id", "default", "scout", "lowest", "development", "production", "user-1")
	require.NoError(t, err)
	require.Equal(t, "lowest", query.DefaultEnvironment)
	require.Equal(t, "production", query.TargetEnvironment)
}

func TestPromoteIdentityMergesRoleReferencedResourceServersWithTheAgentsOwn(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	var carried AgentIdentityPromotion
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		carried = p
		return nil
	}))
	own := MCPResourceServerPromotion{ProxyHandle: "booking", DisplayName: "Booking", Identifier: "https://stage.example.test/booking"}
	f.svc.SetAgentMCPResourceServerResolver(mcpResolverFunc(func(context.Context, string, string, string, string) ([]MCPResourceServerPromotion, error) {
		return []MCPResourceServerPromotion{own}, nil
	}))
	f.svc.SetAgentIdentityResourceResolver(resourceResolverFunc(func(context.Context, AgentIdentityResourceQuery) (AgentIdentityResources, error) {
		return AgentIdentityResources{MCPResourceServers: []MCPResourceServerPromotion{
			{ProxyHandle: "booking", DisplayName: "Booking again", Identifier: "https://other.example.test/booking"},
			{ProxyHandle: "weather", DisplayName: "Weather", Identifier: "https://stage.example.test/weather"},
		}}, nil
	}))

	_, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.Equal(t, []MCPResourceServerPromotion{
		own, {ProxyHandle: "weather", DisplayName: "Weather", Identifier: "https://stage.example.test/weather"},
	}, carried.MCPResourceServers, "each server appears once and the agent's own entry wins")
}

func TestPromoteIdentityBlockedResourcesIssueAndStoreNothing(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	called := false
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error {
		called = true
		return nil
	}))
	f.svc.SetAgentIdentityResourceResolver(resourceResolverFunc(func(context.Context, AgentIdentityResourceQuery) (AgentIdentityResources, error) {
		return AgentIdentityResources{}, &IdentityPromotionError{Kind: IdentityPromotionBlocked, Message: `Role "booker" references an unmanaged server`}
	}))

	// A bare context proves the failure precedes the audit intent.
	_, err := promoteIdentity(f.svc, context.Background(), "org-id", "default", "scout", "development", "staging", "user-1")
	var typed *IdentityPromotionError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, IdentityPromotionBlocked, typed.Kind)
	require.NotErrorIs(t, err, audit.ErrRecorderUnavailable)
	require.False(t, called)
	require.Empty(t, f.store.CreateSecretCalls())
	require.Empty(t, f.upserted)
}

func TestPromoteIdentityPreviewRunsBeforeAnyCredentialExists(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	var previewed, carried AgentIdentityPromotion
	f.svc.SetIdentityPromotionManager(previewingManager{
		promotionManagerFunc: func(_ context.Context, p AgentIdentityPromotion) error {
			f.events = append(f.events, "promote")
			carried = p
			return nil
		},
		preview: func(_ context.Context, p AgentIdentityPromotion) error {
			f.events = append(f.events, "preview")
			previewed = p
			return nil
		},
	})
	f.svc.SetAgentIdentityResourceResolver(resourceResolverFunc(func(context.Context, AgentIdentityResourceQuery) (AgentIdentityResources, error) {
		return AgentIdentityResources{Roles: []PromotedIdentityResource{{ID: "role-1", Name: "booker"}}}, nil
	}))

	_, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.Equal(t, []string{"preview", "promote", "store"}, f.events)
	require.Empty(t, previewed.ClientSecret, "no secret is generated before the preview")
	require.Empty(t, previewed.TargetClientID, "no client ID is generated before the preview")
	require.Equal(t, []PromotedIdentityResource{{ID: "role-1", Name: "booker"}}, previewed.Roles)
	require.NotEmpty(t, carried.ClientSecret)
}

func TestPromoteIdentityPreviewRefusalPreventsCredentialsAuditAndStore(t *testing.T) {
	for _, kind := range []IdentityPromotionErrorKind{
		IdentityPromotionUnavailable, IdentityPromotionForbidden, IdentityPromotionConflict,
		IdentityPromotionMissingValues, IdentityPromotionBlocked,
	} {
		t.Run(string(kind), func(t *testing.T) {
			f := newPromotionFixture(t, stagingByPromotion())
			promoteCalled := false
			f.svc.SetIdentityPromotionManager(previewingManager{
				promotionManagerFunc: func(context.Context, AgentIdentityPromotion) error {
					promoteCalled = true
					return nil
				},
				preview: func(context.Context, AgentIdentityPromotion) error {
					return &IdentityPromotionError{Kind: kind, Message: "refused", MissingValues: []string{"CLIENT_URL"}}
				},
			})
			ctx, rec := recordingCtx(t)

			promoted, err := promoteIdentity(f.svc, ctx, "org-id", "default", "scout", "development", "staging", "user-1")
			var typed *IdentityPromotionError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, kind, typed.Kind)
			require.False(t, promoted)
			require.False(t, promoteCalled)
			require.Empty(t, rec.outcomes(), "a refused preview writes no audit record")
			require.Empty(t, f.store.CreateSecretCalls())
			require.Empty(t, f.upserted)
			require.Empty(t, f.updated)
		})
	}
}

func TestPromoteIdentityPreviewNotApplicableChangesNothing(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.svc.SetIdentityPromotionManager(previewingManager{
		promotionManagerFunc: func(context.Context, AgentIdentityPromotion) error { return errors.New("must not be called") },
		preview:              func(context.Context, AgentIdentityPromotion) error { return ErrIdentityPromotionNotApplicable },
	})
	ctx, rec := recordingCtx(t)
	promoted, err := promoteIdentity(f.svc, ctx, "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.False(t, promoted)
	require.Empty(t, rec.outcomes())
}

func TestPromoteIdentityAuditRecordsSuccessOnlyWhenTheIdentityWasCarried(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error { return nil }))
	ctx, rec := recordingCtx(t)
	_, err := promoteIdentity(f.svc, ctx, "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.Equal(t, []audit.Outcome{audit.OutcomeUnknown, audit.OutcomeSuccess}, rec.outcomes())
	require.Equal(t, true, rec.last().Details["carried"])

	f = newPromotionFixture(t, stagingByPromotion())
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error {
		return ErrIdentityPromotionNotApplicable
	}))
	ctx, rec = recordingCtx(t)
	promoted, err := promoteIdentity(f.svc, ctx, "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.False(t, promoted)
	require.Equal(t, []audit.Outcome{audit.OutcomeUnknown, audit.OutcomeFailure}, rec.outcomes(),
		"nothing was issued or carried, so success must not be recorded")
	require.Equal(t, false, rec.last().Details["carried"])
}

func TestPromoteIdentityKeepsTypedManagerErrors(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(context.Context, AgentIdentityPromotion) error {
		return &IdentityPromotionError{Kind: IdentityPromotionConflict, Message: "target holds a different application"}
	}))
	_, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	var typed *IdentityPromotionError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, IdentityPromotionConflict, typed.Kind)
	require.Empty(t, f.store.CreateSecretCalls(), "nothing is stored when the manager refuses")
	require.Empty(t, f.upserted)
}

func TestPromoteIdentityRepeatedPromotionKeepsTheTargetCredentialsAndCarriesAgain(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	var promotions []AgentIdentityPromotion
	f.svc.SetIdentityPromotionManager(promotionManagerFunc(func(_ context.Context, p AgentIdentityPromotion) error {
		promotions = append(promotions, p)
		return nil
	}))
	f.svc.SetAgentIdentityResourceResolver(resourceResolverFunc(func(context.Context, AgentIdentityResourceQuery) (AgentIdentityResources, error) {
		return AgentIdentityResources{Roles: []PromotedIdentityResource{{ID: "role-1", Name: "booker"}}}, nil
	}))

	_, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.Len(t, f.upserted, 1)
	stored := f.upserted[0]
	f.target = &stored

	_, err = promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "staging", "user-1")
	require.NoError(t, err)
	require.Len(t, promotions, 2)
	require.Equal(t, promotions[0].TargetClientID, promotions[1].TargetClientID)
	require.Empty(t, promotions[1].ClientSecret)
	require.Equal(t, promotions[0].Roles, promotions[1].Roles)
	require.Len(t, f.store.CreateSecretCalls(), 1, "the second promotion stores nothing")
	require.Len(t, f.upserted, 1)
}

func TestPromoteIdentityLegacyEnvironmentNeverTouchesIdentityResources(t *testing.T) {
	f := newPromotionFixture(t, stagingByPromotion())
	f.svc.SetIdentityPromotionManager(previewingManager{
		promotionManagerFunc: func(context.Context, AgentIdentityPromotion) error { t.Fatal("manager called"); return nil },
		preview:              func(context.Context, AgentIdentityPromotion) error { t.Fatal("preview called"); return nil },
	})
	f.svc.SetAgentIdentityResourceResolver(resourceResolverFunc(func(context.Context, AgentIdentityResourceQuery) (AgentIdentityResources, error) {
		t.Fatal("resolver called")
		return AgentIdentityResources{}, nil
	}))
	promoted, err := promoteIdentity(f.svc, auditableCtx(t), "org-id", "default", "scout", "development", "production", "user-1")
	require.NoError(t, err)
	require.False(t, promoted)
}

func TestRotationCarriesTheProjectName(t *testing.T) {
	r := newRotationFixture(t, models.AgentProvisioningTypeInternal)
	_, _, _, err := r.svc.RegenerateSecret(context.Background(), "org-id", "default", "scout", "staging")
	require.NoError(t, err)
	_, err = r.svc.RevokeSecret(context.Background(), "org-id", "default", "scout", "staging")
	require.NoError(t, err)
	require.Len(t, r.rotations, 2)
	for _, rotation := range r.rotations {
		require.Equal(t, "default", rotation.ProjectName)
	}
}

func TestRegenerateSecretKeepsTheExistingReferenceWhenTheStoreReturnsNone(t *testing.T) {
	r := newRotationFixture(t, models.AgentProvisioningTypeInternal)
	var recorded []string
	r.repo.UpdateSecretRefFunc = func(_ context.Context, _ uuid.UUID, ref string) error {
		recorded = append(recorded, ref)
		return nil
	}
	r.store.PatchSecretFunc = func(context.Context, secretmanagersvc.SecretLocation, map[string]string, []string) (string, error) {
		return "", nil
	}
	_, _, _, err := r.svc.RegenerateSecret(context.Background(), "org-id", "default", "scout", "staging")
	require.NoError(t, err)
	require.Equal(t, []string{"existing-reference"}, recorded, "an empty reference must never replace the binding's own")
}

func TestSecretsAreRedactedFromFormattedAndLoggedPromotions(t *testing.T) {
	const secret = "s3cr3t-value-that-must-not-leak"
	promotion := AgentIdentityPromotion{
		OUID: "org-id", ProjectName: "default", SourceEnvironment: "development", TargetEnvironment: "staging",
		ThunderAgentID: "agent-1", TargetClientID: "client-1", ClientSecret: secret,
		Roles: []PromotedIdentityResource{{ID: "r", Name: "booker"}},
	}
	rotation := AgentSecretRotation{OUID: "org-id", ProjectName: "default", Environment: "staging", ThunderAgentID: "agent-1", ClientSecret: secret}

	for name, value := range map[string]any{"promotion": promotion, "promotionPtr": &promotion, "rotation": rotation, "rotationPtr": &rotation} {
		for _, format := range []string{"%v", "%+v", "%s", "%#v"} {
			require.NotContains(t, fmt.Sprintf(format, value), secret, "%s with %s", name, format)
		}
		var text, structured bytes.Buffer
		slog.New(slog.NewTextHandler(&text, nil)).Info("carry", "value", value)
		slog.New(slog.NewJSONHandler(&structured, nil)).Info("carry", "value", value)
		require.NotContains(t, text.String(), secret, name)
		require.NotContains(t, structured.String(), secret, name)
	}
	require.Contains(t, fmt.Sprintf("%v", promotion), "[REDACTED]")
	require.Contains(t, fmt.Sprintf("%v", AgentIdentityPromotion{}), "<empty>", "an unset secret is not reported as redacted")
	require.Equal(t, secret, promotion.ClientSecret, "redaction must not alter the value the manager receives")
}

func TestIdentityPromotionErrorBehavesAsATypedError(t *testing.T) {
	cause := errors.New("dial tcp 10.0.0.7:8090: connection refused")
	err := fmt.Errorf("wrapped: %w", &IdentityPromotionError{
		Kind: IdentityPromotionMissingValues, Message: "values are missing", MissingValues: []string{"A", "B"}, Cause: cause,
	})
	var typed *IdentityPromotionError
	require.ErrorAs(t, err, &typed)
	require.ErrorIs(t, err, cause)
	require.Equal(t, "identity promotion missing_values: values are missing (2 required value(s) are missing)", typed.Error())
	require.NotContains(t, typed.Error(), "10.0.0.7", "the cause is for logs, not for the rendered error")
	require.NotContains(t, typed.Error(), "A, B", "value names are not rendered")
}

// --- ResolveAgentIdentityResources ---

type identityResourceFixture struct {
	svc      *MCPProxyService
	identity *clientmocks.EnvIdentityClientMock
	roles    map[string]*thundersvc.ThunderRole
	proxies  map[string]*models.MCPProxy
	envs     []string
}

func newIdentityResourceFixture(t *testing.T) *identityResourceFixture {
	t.Helper()
	stagingUUID := uuid.New()
	gwUUID := uuid.New()
	f := &identityResourceFixture{roles: map[string]*thundersvc.ThunderRole{}, proxies: map[string]*models.MCPProxy{}}
	deployed := func(handle string) *models.MCPProxy {
		ctxPath := "/" + handle
		return &models.MCPProxy{
			UUID: uuid.New(), Handle: handle, Artifact: &models.Artifact{Handle: handle, Name: handle + " server"},
			Configuration: models.MCPProxyConfig{Context: &ctxPath},
			Endpoints: []models.MCPProxyEndpoint{{
				Environments: []models.MCPProxyEndpointEnvironment{{EnvironmentUUID: stagingUUID, ArtifactUUID: uuid.New()}},
			}},
		}
	}
	f.proxies["booking"] = deployed("booking")
	f.proxies["weather"] = deployed("weather")
	f.proxies["offline"] = &models.MCPProxy{UUID: uuid.New(), Handle: "offline", Artifact: &models.Artifact{Handle: "offline"}}

	f.identity = &clientmocks.EnvIdentityClientMock{
		GetDefaultOUIDFunc: func(context.Context) (string, error) { return "thunder-ou", nil },
		GetAgentRolesFunc:  func(context.Context, string) ([]thundersvc.ThunderRole, error) { return nil, nil },
		GetAgentGroupsFunc: func(context.Context, string, string) ([]thundersvc.ThunderGroup, error) { return nil, nil },
		GetGroupRolesFunc:  func(context.Context, string) ([]thundersvc.ThunderRole, error) { return nil, nil },
		GetRoleFunc: func(_ context.Context, id string) (*thundersvc.ThunderRole, error) {
			role, ok := f.roles[id]
			if !ok {
				return nil, fmt.Errorf("role %s not found", id)
			}
			return role, nil
		},
	}
	f.svc = &MCPProxyService{
		repo: &repomocks.MCPProxyRepositoryMock{
			GetByHandleFunc: func(_ context.Context, handle, _ string) (*models.MCPProxy, error) {
				if proxy, ok := f.proxies[handle]; ok {
					return proxy, nil
				}
				return nil, gorm.ErrRecordNotFound
			},
		},
		infraManager: stubInfraManager{listOrgEnvs: func(context.Context, string) ([]*models.EnvironmentResponse, error) {
			return []*models.EnvironmentResponse{{Name: "staging", UUID: stagingUUID.String()}}, nil
		}},
		deploymentRepo: &repomocks.DeploymentRepositoryMock{
			GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) { return []string{gwUUID.String()}, nil },
		},
		gatewayRepo: &repomocks.GatewayRepositoryMock{
			EnvironmentMappingExistsFunc: func(string, string) (bool, error) { return true, nil },
			GetByUUIDFunc: func(string) (*models.Gateway, error) {
				return &models.Gateway{UUID: gwUUID, Vhost: "https://gw.example.com"}, nil
			},
		},
		resolver: &clientmocks.EnvThunderResolverMock{
			ResolveIdentityFunc: func(_ context.Context, _, _, env string) (thundersvc.EnvIdentityClient, error) {
				f.envs = append(f.envs, env)
				return f.identity, nil
			},
		},
		logger: discardLogger(),
	}
	return f
}

func (f *identityResourceFixture) query() AgentIdentityResourceQuery {
	return AgentIdentityResourceQuery{
		OUID: "org-id", ProjectName: "default", AgentName: "scout", ThunderAgentID: "agent-1",
		DefaultEnvironment: "development", TargetEnvironment: "staging",
	}
}

func TestResolveAgentIdentityResourcesCarriesRolesGroupsAndInheritedRolesFromTheDefaultEnvironment(t *testing.T) {
	f := newIdentityResourceFixture(t)
	f.roles["role-direct"] = &thundersvc.ThunderRole{ID: "role-direct", Name: "booker", Permissions: []thundersvc.RolePermissionRequest{
		{ResourceServerID: "rs-1", Permissions: []string{"booking:search", "booking:book"}},
	}}
	f.roles["role-group"] = &thundersvc.ThunderRole{ID: "role-group", Name: "forecaster", Permissions: []thundersvc.RolePermissionRequest{
		{ResourceServerID: "rs-2", Permissions: []string{"weather:read"}},
	}}
	f.roles["role-bare"] = &thundersvc.ThunderRole{ID: "role-bare", Name: "observer"}
	f.identity.GetAgentRolesFunc = func(_ context.Context, agentID string) ([]thundersvc.ThunderRole, error) {
		require.Equal(t, "agent-1", agentID)
		return []thundersvc.ThunderRole{{ID: "role-direct", Name: "booker"}}, nil
	}
	f.identity.GetAgentGroupsFunc = func(_ context.Context, ou, agentID string) ([]thundersvc.ThunderGroup, error) {
		require.Equal(t, "thunder-ou", ou)
		require.Equal(t, "agent-1", agentID)
		return []thundersvc.ThunderGroup{{ID: "group-1", Name: "travel"}}, nil
	}
	f.identity.GetGroupRolesFunc = func(_ context.Context, groupID string) ([]thundersvc.ThunderRole, error) {
		require.Equal(t, "group-1", groupID)
		return []thundersvc.ThunderRole{
			{ID: "role-group", Name: "forecaster"},
			{ID: "role-direct", Name: "booker"}, // also assigned directly: carried once
			{ID: "role-bare", Name: "observer"},
			{ID: "role-admin", Name: thundersvc.NativeAdministratorRoleName},
			{ID: "role-system", Name: thundersvc.AMPSystemClientRoleName},
		}, nil
	}

	got, err := f.svc.ResolveAgentIdentityResources(context.Background(), f.query())
	require.NoError(t, err)
	require.Equal(t, []string{"development"}, f.envs, "roles and groups are read from the default environment only")
	require.Equal(t, []PromotedIdentityResource{{ID: "group-1", Name: "travel"}}, got.Groups)
	require.Equal(t, []PromotedIdentityResource{
		{ID: "role-bare", Name: "observer"}, {ID: "role-direct", Name: "booker"}, {ID: "role-group", Name: "forecaster"},
	}, got.Roles, "inherited roles are included, native administrator roles never are")
	require.Equal(t, []MCPResourceServerPromotion{
		{ProxyHandle: "booking", DisplayName: "booking server", Identifier: "https://gw.example.com/booking/mcp"},
		{ProxyHandle: "weather", DisplayName: "weather server", Identifier: "https://gw.example.com/weather/mcp"},
	}, got.MCPResourceServers, "role referenced servers carry the TARGET identifier, once each")
}

func TestResolveAgentIdentityResourcesBlocksARoleReferencingAnUnmanagedResourceServer(t *testing.T) {
	for name, scope := range map[string]string{
		"unknown proxy handle": "ghost:read",
		"not a proxy scope":    "project:create",
		"no action":            "booking:",
	} {
		t.Run(name, func(t *testing.T) {
			f := newIdentityResourceFixture(t)
			f.roles["role-1"] = &thundersvc.ThunderRole{ID: "role-1", Name: "booker", Permissions: []thundersvc.RolePermissionRequest{
				{ResourceServerID: "rs-1", Permissions: []string{scope}},
			}}
			f.identity.GetAgentRolesFunc = func(context.Context, string) ([]thundersvc.ThunderRole, error) {
				return []thundersvc.ThunderRole{{ID: "role-1", Name: "booker"}}, nil
			}
			_, err := f.svc.ResolveAgentIdentityResources(context.Background(), f.query())
			var typed *IdentityPromotionError
			require.ErrorAs(t, err, &typed)
			require.Equal(t, IdentityPromotionBlocked, typed.Kind)
			require.Contains(t, typed.Message, `"booker"`, "the message names the role")
			require.NotContains(t, typed.Message, "https://", "no source environment URL is ever carried or guessed")
		})
	}
}

func TestResolveAgentIdentityResourcesBlocksWhenTheReferencedProxyIsNotDeployedToTheTarget(t *testing.T) {
	f := newIdentityResourceFixture(t)
	f.roles["role-1"] = &thundersvc.ThunderRole{ID: "role-1", Name: "booker", Permissions: []thundersvc.RolePermissionRequest{
		{ResourceServerID: "rs-1", Permissions: []string{"offline:read"}},
	}}
	f.identity.GetAgentRolesFunc = func(context.Context, string) ([]thundersvc.ThunderRole, error) {
		return []thundersvc.ThunderRole{{ID: "role-1", Name: "booker"}}, nil
	}
	_, err := f.svc.ResolveAgentIdentityResources(context.Background(), f.query())
	var typed *IdentityPromotionError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, IdentityPromotionBlocked, typed.Kind)
	require.Contains(t, typed.Message, `"booker"`)
	require.Contains(t, typed.Message, `"offline"`)
	require.Contains(t, typed.Message, `"staging"`)
}

func TestResolveAgentIdentityResourcesReportsAnUnreachableIdentityProviderAsUnavailable(t *testing.T) {
	f := newIdentityResourceFixture(t)
	f.identity.GetAgentRolesFunc = func(context.Context, string) ([]thundersvc.ThunderRole, error) {
		return nil, errors.New("Get https://thunder.internal.svc:8090/roles: connection refused")
	}
	_, err := f.svc.ResolveAgentIdentityResources(context.Background(), f.query())
	var typed *IdentityPromotionError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, IdentityPromotionUnavailable, typed.Kind)
	require.NotContains(t, typed.Message, "thunder.internal")
	require.ErrorContains(t, typed.Cause, "connection refused", "the cause is kept for logs")
}

func TestResolveAgentIdentityResourcesWithoutAResolverIsUnavailable(t *testing.T) {
	svc := &MCPProxyService{logger: discardLogger()}
	_, err := svc.ResolveAgentIdentityResources(context.Background(), AgentIdentityResourceQuery{})
	var typed *IdentityPromotionError
	require.ErrorAs(t, err, &typed)
	require.Equal(t, IdentityPromotionUnavailable, typed.Kind)
}
