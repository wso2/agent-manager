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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

type projectAgentIdentityPromotionProviderStub struct {
	projectAgentIdentityProviderStub
	promote func(context.Context, ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error)
}

func (s projectAgentIdentityPromotionProviderStub) Promote(
	ctx context.Context, request ProjectAgentIdentityPromotionRequest,
) (ProjectAgentIdentityResult, error) {
	return s.promote(ctx, request)
}

// fakeTargetPlatform models the identity platform's target credential. It
// honours the Promote contract: it overwrites the target credential with the
// requested one on every call and echoes it back.
type fakeTargetPlatform struct {
	mu             sync.Mutex
	heldClientID   string
	heldSecret     string
	secretsPassed  []string
	clientIDPassed []string
	requests       []ProjectAgentIdentityPromotionRequest
}

func (p *fakeTargetPlatform) provider(t *testing.T) projectAgentIdentityPromotionProviderStub {
	t.Helper()
	return projectAgentIdentityPromotionProviderStub{
		projectAgentIdentityProviderStub: projectAgentIdentityProviderStub{
			provision: func(context.Context, ProjectAgentIdentityRequest) (ProjectAgentIdentityResult, error) {
				t.Fatal("target environments must use promotion")
				return ProjectAgentIdentityResult{}, nil
			},
			activate: func(context.Context, ProjectAgentIdentityRequest, string) error {
				t.Fatal("a promoted target must not run source activation")
				return nil
			},
		},
		promote: func(_ context.Context, request ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			p.mu.Lock()
			defer p.mu.Unlock()
			p.requests = append(p.requests, request)
			p.secretsPassed = append(p.secretsPassed, request.TargetClientSecret)
			p.clientIDPassed = append(p.clientIDPassed, request.TargetClientID)
			p.heldClientID, p.heldSecret = request.TargetClientID, request.TargetClientSecret
			return ProjectAgentIdentityResult{
				AgentID: request.ExistingAgentID, ClientID: p.heldClientID, ClientSecret: p.heldSecret,
			}, nil
		},
	}
}

func completedSource() *models.AgentThunderClient {
	return &models.AgentThunderClient{
		OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", EnvironmentName: "development",
		Status: models.AgentThunderStatusCompleted, ThunderAgentID: "agent-id", ThunderClientID: "source-client",
	}
}

func promotedBinding(ownership models.AgentProvisioningType) models.AgentThunderClient {
	return models.AgentThunderClient{
		ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a",
		EnvironmentName: "stage", PromotionSource: "development", ProvisioningType: ownership,
	}
}

func newPromotionService(
	t *testing.T, repo *repomocks.AgentThunderClientRepositoryMock, store *clientmocks.SecretManagementClientMock,
	provider ProjectAgentIdentityProvider, reader ProjectPipelineReader,
) AgentThunderProvisioningService {
	t.Helper()
	svc := newTestProvisioningService(repo, &clientmocks.EnvThunderResolverMock{}, store)
	require.NoError(t, svc.(ProjectAgentIdentityProviderSetter).SetProjectAgentIdentityProvider(
		thundersvc.IdentityManagementCPManaged, provider,
	))
	if reader != nil {
		svc.(ProjectPipelineReaderSetter).SetProjectPipelineReader(reader)
	}
	return svc
}

func noExistingBindings(context.Context, string, string, string) ([]models.AgentThunderClient, error) {
	return []models.AgentThunderClient{}, nil
}

// linearTopology is development -> stage -> production.
func linearTopology() []ProjectIdentityEnvironment {
	return []ProjectIdentityEnvironment{
		{Name: "production"},
		{Name: "development", PromotesTo: []string{"stage"}},
		{Name: "stage", PromotesTo: []string{"production"}},
	}
}

func TestPromotionStepsLinearPipeline(t *testing.T) {
	steps, err := promotionSteps("development", linearTopology())
	require.NoError(t, err)
	require.Equal(t, []promotionStep{
		{Environment: "development"},
		{Environment: "stage", Source: "development"},
		{Environment: "production", Source: "stage"},
	}, steps)
}

func TestPromotionStepsSupportsBranchesDeterministically(t *testing.T) {
	steps, err := promotionSteps("development", []ProjectIdentityEnvironment{
		{Name: "development", PromotesTo: []string{"production", "staging"}},
		{Name: "staging"},
		{Name: "production"},
	})
	require.NoError(t, err)
	require.Equal(t, []promotionStep{
		{Environment: "development"},
		{Environment: "production", Source: "development"},
		{Environment: "staging", Source: "development"},
	}, steps)
}

func TestPromotionStepsRejectsUnreachableEnvironment(t *testing.T) {
	_, err := promotionSteps("development", []ProjectIdentityEnvironment{
		{Name: "development"},
		{Name: "production"},
	})
	require.ErrorContains(t, err, "unreachable")
}

func TestDefaultEnvironmentPipelineReaderReportsTheSingleDefaultEnvironment(t *testing.T) {
	source, environments, err := NewDefaultEnvironmentPipelineReader().ProjectPipeline(context.Background(), "org-a", "project-a")
	require.NoError(t, err)
	require.Equal(t, models.DefaultEnvironmentName, source)
	steps, err := promotionSteps(source, environments)
	require.NoError(t, err)
	require.Equal(t, []promotionStep{{Environment: models.DefaultEnvironmentName}}, steps)
}

func TestEnsureProjectPromotionBindingsWritesSourceFirstAndIsIdempotent(t *testing.T) {
	existing := map[string]bool{}
	var order []string
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: noExistingBindings,
		GetFunc: func(_ context.Context, _, _, _, env string) (*models.AgentThunderClient, error) {
			if existing[env] {
				return &models.AgentThunderClient{EnvironmentName: env}, nil
			}
			return nil, repositories.ErrAgentThunderClientNotFound
		},
		UpsertFunc: func(_ context.Context, binding *models.AgentThunderClient) error {
			existing[binding.EnvironmentName] = true
			order = append(order, binding.EnvironmentName+"<-"+binding.PromotionSource)
			binding.ID = uuid.New()
			return nil
		},
	}
	provider := (&fakeTargetPlatform{}).provider(t)
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, provider,
		projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
			return "development", linearTopology(), nil
		}))
	impl := svc.(*agentThunderProvisioningService)
	ref := repositories.AgentRef{OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", ProvisioningType: models.AgentProvisioningTypeInternal}

	created, err := impl.ensureProjectPromotionBindings(context.Background(), ref, "user-1")
	require.NoError(t, err)
	require.Len(t, created, 3)
	require.Equal(t, []string{"development<-", "stage<-development", "production<-stage"}, order)
	for _, binding := range created {
		require.Equal(t, models.AgentThunderStatusPending, binding.Status)
		require.Equal(t, "user-1", binding.RequestedBy)
	}

	again, err := impl.ensureProjectPromotionBindings(context.Background(), ref, "user-1")
	require.NoError(t, err)
	require.Empty(t, again)
	require.Len(t, order, 3, "a second pass must not write anything")
}

func TestEnsureProjectPromotionBindingsTreatsInsertConflictAsSuccess(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc: noExistingBindings,
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
		// A concurrent writer won the insert: DoNothing leaves the ID unset.
		UpsertFunc: func(context.Context, *models.AgentThunderClient) error { return nil },
	}
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, (&fakeTargetPlatform{}).provider(t),
		sourceOnlyReader("development", nil))
	created, err := svc.(*agentThunderProvisioningService).ensureProjectPromotionBindings(
		context.Background(), repositories.AgentRef{OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a"}, "",
	)
	require.NoError(t, err)
	require.Empty(t, created)
}

func TestReconcileReadsEachProjectPipelineOncePerPass(t *testing.T) {
	refs := []repositories.AgentRef{
		{OUID: "org-a", ProjectName: "project-a", AgentName: "agent-1"},
		{OUID: "org-a", ProjectName: "project-a", AgentName: "agent-2"},
		{OUID: "org-a", ProjectName: "project-a", AgentName: "agent-3"},
		{OUID: "org-a", ProjectName: "project-b", AgentName: "agent-1"},
		{OUID: "org-a", ProjectName: "project-bad", AgentName: "agent-1"},
		{OUID: "org-a", ProjectName: "project-bad", AgentName: "agent-2"},
		{OUID: "org-b", ProjectName: "project-a", AgentName: "agent-1"},
	}
	for i := range refs {
		refs[i].EnvironmentName = "development"
		refs[i].RequestedBy = "user-1"
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ListHomeAgentsFunc: func(_ context.Context, after *repositories.AgentRef, _ int) ([]repositories.AgentRef, error) {
			if after != nil {
				return nil, nil
			}
			return refs, nil
		},
		GetFunc: func(_ context.Context, _, _, _, env string) (*models.AgentThunderClient, error) {
			if env == "development" {
				return &models.AgentThunderClient{EnvironmentName: env}, nil
			}
			return nil, repositories.ErrAgentThunderClientNotFound
		},
		UpsertFunc: func(_ context.Context, binding *models.AgentThunderClient) error {
			binding.ID = uuid.New()
			return nil
		},
	}
	var mu sync.Mutex
	calls := map[string]int{}
	reader := projectPipelineReaderFunc(func(_ context.Context, org, project string) (string, []ProjectIdentityEnvironment, error) {
		mu.Lock()
		defer mu.Unlock()
		calls[org+"/"+project]++
		if project == "project-bad" {
			return "", nil, errors.New("pipeline unavailable")
		}
		return "development", linearTopology(), nil
	})
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, (&fakeTargetPlatform{}).provider(t), reader)
	reconciler := svc.(ProjectAgentIdentityPromotionReconciler)

	reconciler.ReconcileProjectAgentIdentityPromotions(context.Background())

	require.Equal(t, map[string]int{
		"org-a/project-a": 1, "org-a/project-b": 1, "org-a/project-bad": 1, "org-b/project-a": 1,
	}, calls, "one pipeline read per project per pass, failures included")
	require.Len(t, repo.UpsertCalls(), 2*5, "stage and production rows for each of the 5 agents of healthy projects")
	for _, call := range repo.UpsertCalls() {
		require.Equal(t, "user-1", call.Client.RequestedBy)
	}

	reconciler.ReconcileProjectAgentIdentityPromotions(context.Background())
	for key, count := range calls {
		require.Equal(t, 2, count, "the cache must not outlive a pass: %s", key)
	}
}

func TestReconcileRefusesToCreateASecondSourceAfterPipelineSourceChanges(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ListHomeAgentsFunc: func(context.Context, *repositories.AgentRef, int) ([]repositories.AgentRef, error) {
			return []repositories.AgentRef{{
				OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a",
				EnvironmentName: "development", ProvisioningType: models.AgentProvisioningTypeInternal,
			}}, nil
		},
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			t.Fatal("source drift must stop before writing bindings")
			return nil, errors.New("unreachable")
		},
		UpsertFunc: func(context.Context, *models.AgentThunderClient) error {
			t.Fatal("source drift must not create a second source")
			return nil
		},
	}
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, (&fakeTargetPlatform{}).provider(t),
		projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
			return "sandbox", []ProjectIdentityEnvironment{{Name: "sandbox", PromotesTo: []string{"development"}}, {Name: "development"}}, nil
		}))
	svc.(ProjectAgentIdentityPromotionReconciler).ReconcileProjectAgentIdentityPromotions(context.Background())
}

func TestReconcileDoesNothingWithoutPromotionProvider(t *testing.T) {
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ListHomeAgentsFunc: func(context.Context, *repositories.AgentRef, int) ([]repositories.AgentRef, error) {
			t.Fatal("a provider that cannot promote has nothing to reconcile")
			return nil, nil
		},
	}
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, projectAgentIdentityProviderStub{}, nil)
	svc.(ProjectAgentIdentityPromotionReconciler).ReconcileProjectAgentIdentityPromotions(context.Background())
}

func provisionRepo(upserted *[]models.AgentThunderClient) *repomocks.AgentThunderClientRepositoryMock {
	return &repomocks.AgentThunderClientRepositoryMock{
		// Never claimed, so no background attempt does real work.
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return false, nil },
		FindByAgentFunc:     noExistingBindings,
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
		UpsertFunc: func(_ context.Context, binding *models.AgentThunderClient) error {
			binding.ID = uuid.New()
			*upserted = append(*upserted, *binding)
			return nil
		},
	}
}

func TestProvisionForAgentWritesOrderedPromotionBindingsFromThePipeline(t *testing.T) {
	var upserted []models.AgentThunderClient
	svc := newPromotionService(t, provisionRepo(&upserted), &clientmocks.SecretManagementClientMock{},
		(&fakeTargetPlatform{}).provider(t),
		projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
			return "development", linearTopology(), nil
		}))

	svc.ProvisionForAgent(context.Background(), "org-a", "project-a", "agent-a", models.AgentProvisioningTypeInternal,
		[]string{"stale-env"}, "user-1", "")

	require.Len(t, upserted, 3)
	require.Equal(t, "development", upserted[0].EnvironmentName)
	require.Empty(t, upserted[0].PromotionSource)
	require.Equal(t, "stage", upserted[1].EnvironmentName)
	require.Equal(t, "development", upserted[1].PromotionSource)
	require.Equal(t, "production", upserted[2].EnvironmentName)
	require.Equal(t, "stage", upserted[2].PromotionSource)
}

func TestProvisionForAgentFallsBackToCandidatesWhenThePipelineFails(t *testing.T) {
	for name, reader := range map[string]ProjectPipelineReader{
		"reader fails":   sourceOnlyReader("", errors.New("pipeline unavailable")),
		"reader not set": nil,
		"unreachable environment": projectPipelineReaderFunc(
			func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
				return "development", []ProjectIdentityEnvironment{{Name: "development"}, {Name: "orphan"}}, nil
			},
		),
	} {
		t.Run(name, func(t *testing.T) {
			var upserted []models.AgentThunderClient
			svc := newPromotionService(t, provisionRepo(&upserted), &clientmocks.SecretManagementClientMock{},
				(&fakeTargetPlatform{}).provider(t), reader)

			svc.ProvisionForAgent(context.Background(), "org-a", "project-a", "agent-a", models.AgentProvisioningTypeInternal,
				[]string{"development", "stage"}, "user-1", "")

			require.Len(t, upserted, 2, "the agent must not be lost: a candidate per known environment")
			require.ElementsMatch(t, []string{"development", "stage"},
				[]string{upserted[0].EnvironmentName, upserted[1].EnvironmentName})
			for _, binding := range upserted {
				require.Empty(t, binding.PromotionSource, "candidates are resolved against the pipeline later")
			}
		})
	}
}

func TestAttemptProvisionReplacesNonSourceCandidateUsingThePipelineSource(t *testing.T) {
	candidate := models.AgentThunderClient{
		ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a",
		EnvironmentName: "stage", ProvisioningType: models.AgentProvisioningTypeInternal,
	}
	var replacement models.AgentThunderClient
	var deleted []uuid.UUID
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		UpsertFunc: func(_ context.Context, binding *models.AgentThunderClient) error {
			replacement = *binding
			return nil
		},
		DeleteByIDsFunc: func(_ context.Context, ids []uuid.UUID) error {
			deleted = append(deleted, ids...)
			return nil
		},
	}
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, (&fakeTargetPlatform{}).provider(t),
		projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
			return "development", linearTopology(), nil
		}))

	svc.AttemptProvision(context.Background(), candidate)

	require.Equal(t, "development", replacement.EnvironmentName)
	require.Empty(t, replacement.PromotionSource)
	require.Equal(t, []uuid.UUID{candidate.ID}, deleted)
}

func TestAttemptProvisionRefusesDirectNonSourceProvisioning(t *testing.T) {
	// A non-source binding that already carries an identity is never provisioned
	// directly: it fails with the promotion-required conflict.
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a",
		EnvironmentName: "stage", ThunderAgentID: "existing", ProvisioningType: models.AgentProvisioningTypeInternal,
	}
	var update repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
			update = value
			return nil
		},
	}
	provider := projectAgentIdentityProviderStub{
		provision: func(context.Context, ProjectAgentIdentityRequest) (ProjectAgentIdentityResult, error) {
			t.Fatal("must not provision in a non-source environment")
			return ProjectAgentIdentityResult{}, nil
		},
	}
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, provider, sourceOnlyReader("development", nil))

	svc.AttemptProvision(context.Background(), binding)

	require.Equal(t, models.AgentThunderStatusPending, update.Status)
	require.Empty(t, update.ThunderClientID)
}

func TestLegacyDirectIgnoresThePipelineReader(t *testing.T) {
	var upserted []models.AgentThunderClient
	repo := provisionRepo(&upserted)
	svc := newTestProvisioningService(repo, &clientmocks.EnvThunderResolverMock{}, &clientmocks.SecretManagementClientMock{})
	svc.(ProjectPipelineReaderSetter).SetProjectPipelineReader(projectPipelineReaderFunc(
		func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
			t.Fatal("LEGACY_DIRECT must never read the project pipeline")
			return "", nil, nil
		},
	))

	svc.ProvisionForAgent(context.Background(), "org-a", "project-a", "agent-a", models.AgentProvisioningTypeInternal,
		[]string{"development", "stage", "production"}, "user-1", "")

	require.Len(t, upserted, 3)
	for i, env := range []string{"development", "stage", "production"} {
		require.Equal(t, env, upserted[i].EnvironmentName)
		require.Empty(t, upserted[i].PromotionSource)
	}

	alreadyExisted, err := svc.ProvisionForEnvironmentIfMissing(context.Background(),
		"org-a", "project-a", "agent-b", "production", models.AgentProvisioningTypeInternal, "user-1")
	require.NoError(t, err)
	require.False(t, alreadyExisted)
}

func TestAttemptProvisionPromotesOnlyTheTargetAgentID(t *testing.T) {
	binding := promotedBinding(models.AgentProvisioningTypeInternal)
	var update repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		GetFunc: func(_ context.Context, _, _, _, env string) (*models.AgentThunderClient, error) {
			assert.Equal(t, "development", env, "only the source binding is read")
			return completedSource(), nil
		},
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
			update = value
			return nil
		},
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			return "target-secret-reference", nil
		},
	}
	platform := &fakeTargetPlatform{}
	svc := newPromotionService(t, repo, store, platform.provider(t), nil)

	svc.AttemptProvision(context.Background(), binding)

	require.Len(t, platform.requests, 1)
	request := platform.requests[0]
	assert.Equal(t, "development", request.SourceEnvironment)
	assert.Equal(t, "stage", request.TargetEnvironment)
	assert.Equal(t, "agent-id", request.ExistingAgentID)
	assert.Equal(t, "source-client", request.ExistingClientID)
	assert.NotEmpty(t, request.TargetClientID)
	assert.NotEmpty(t, request.TargetClientSecret)
	assert.True(t, request.NeedsSecret)
	require.Equal(t, models.AgentThunderStatusCompleted, update.Status)
	require.NotNil(t, update.ThunderAgentID)
	require.Equal(t, "agent-id", *update.ThunderAgentID)
	require.NotNil(t, update.SecretRefPath)
	require.Equal(t, "target-secret-reference", *update.SecretRefPath)
}

// TestPromotionRetryAfterFailedStoreStoresTheSecretTheTargetHolds covers the
// partial failure where the target credential is set but storing it fails: the
// retry must hand a secret to Promote again and store exactly the value the
// target then holds, never the value of the failed attempt.
func TestPromotionRetryAfterFailedStoreStoresTheSecretTheTargetHolds(t *testing.T) {
	binding := promotedBinding(models.AgentProvisioningTypeInternal)
	var updates []repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return completedSource(), nil
		},
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
			updates = append(updates, value)
			return nil
		},
	}
	var stored []string
	storeCalls := 0
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			storeCalls++
			if storeCalls == 1 {
				return "", errors.New("secret store unavailable")
			}
			stored = append(stored, data[thundersvc.AgentSecretKeyClientSecret])
			assert.Equal(t, data[thundersvc.AgentSecretKeyClientID], *updates[len(updates)-1].ThunderClientID,
				"the stored client ID is the one the failed attempt persisted")
			return "secret-reference", nil
		},
	}
	platform := &fakeTargetPlatform{}
	svc := newPromotionService(t, repo, store, platform.provider(t), nil)

	svc.AttemptProvision(context.Background(), binding)

	require.Len(t, updates, 1)
	require.Equal(t, models.AgentThunderStatusPending, updates[0].Status)
	require.NotNil(t, updates[0].ThunderClientID, "the identity is kept so the retry reuses the client ID")
	require.Nil(t, updates[0].SecretRefPath)

	retry := binding
	retry.ThunderAgentID = *updates[0].ThunderAgentID
	retry.ThunderClientID = *updates[0].ThunderClientID
	retry.AttemptCount = 1
	svc.AttemptProvision(context.Background(), retry)

	require.Len(t, platform.requests, 2)
	require.Len(t, stored, 1)
	require.NotEmpty(t, platform.secretsPassed[1])
	require.NotEqual(t, platform.secretsPassed[0], platform.secretsPassed[1], "a fresh secret per attempt")
	require.Equal(t, platform.heldSecret, stored[0], "the stored secret must be the one the target holds")
	require.Equal(t, platform.clientIDPassed[0], platform.clientIDPassed[1], "the client ID is stable across attempts")
	require.Equal(t, models.AgentThunderStatusCompleted, updates[len(updates)-1].Status)
}

func TestPromotionRetryWithExistingSecretReferenceStillStoresTheNewSecret(t *testing.T) {
	binding := promotedBinding(models.AgentProvisioningTypeInternal)
	binding.ThunderAgentID, binding.ThunderClientID, binding.SecretRefPath = "agent-id", "target-client", "old-reference"
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return completedSource(), nil
		},
		UpdateAfterAttemptFunc: func(context.Context, uuid.UUID, repositories.AgentThunderAttemptUpdate) error { return nil },
	}
	var stored string
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(_ context.Context, _ secretmanagersvc.SecretLocation, data map[string]string) (string, error) {
			stored = data[thundersvc.AgentSecretKeyClientSecret]
			return "new-reference", nil
		},
	}
	platform := &fakeTargetPlatform{}
	svc := newPromotionService(t, repo, store, platform.provider(t), nil)

	svc.AttemptProvision(context.Background(), binding)

	require.Len(t, platform.requests, 1)
	require.True(t, platform.requests[0].NeedsSecret, "Promote overwrites the target secret, so it is always stored again")
	require.Equal(t, "target-client", platform.requests[0].TargetClientID)
	require.NotEmpty(t, stored)
	require.Equal(t, platform.heldSecret, stored)
}

func TestPromotionRejectsAProviderThatDoesNotApplyTheRequestedCredential(t *testing.T) {
	binding := promotedBinding(models.AgentProvisioningTypeInternal)
	var update repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return completedSource(), nil
		},
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
			update = value
			return nil
		},
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			t.Fatal("a secret that the target does not hold must never be stored")
			return "", nil
		},
	}
	provider := projectAgentIdentityPromotionProviderStub{
		promote: func(_ context.Context, request ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			// Keeps whatever the environment already had instead of the requested secret.
			return ProjectAgentIdentityResult{
				AgentID: request.ExistingAgentID, ClientID: request.TargetClientID, ClientSecret: "pre-existing-secret",
			}, nil
		},
	}
	svc := newPromotionService(t, repo, store, provider, nil)

	svc.AttemptProvision(context.Background(), binding)

	require.Equal(t, models.AgentThunderStatusPending, update.Status)
	require.Nil(t, update.SecretRefPath)
}

func TestPromotionFailurePersistsGeneratedTargetClientID(t *testing.T) {
	binding := promotedBinding(models.AgentProvisioningTypeInternal)
	var update repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return completedSource(), nil
		},
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
			update = value
			return nil
		},
	}
	provider := projectAgentIdentityPromotionProviderStub{
		promote: func(context.Context, ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			return ProjectAgentIdentityResult{}, errors.New("response lost after apply")
		},
	}
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, provider, nil)
	svc.AttemptProvision(context.Background(), binding)

	require.NotNil(t, update.ThunderAgentID)
	require.Equal(t, "agent-id", *update.ThunderAgentID)
	require.NotNil(t, update.ThunderClientID)
	require.NotEmpty(t, *update.ThunderClientID)
}

func TestPromotionNeverStoresAnExternalAgentSecret(t *testing.T) {
	binding := promotedBinding(models.AgentProvisioningTypeExternal)
	var update repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return completedSource(), nil
		},
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
			update = value
			return nil
		},
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			t.Fatal("an external agent's secret must never be stored")
			return "", nil
		},
	}
	platform := &fakeTargetPlatform{}
	svc := newPromotionService(t, repo, store, platform.provider(t), nil)

	svc.AttemptProvision(context.Background(), binding)

	require.Len(t, platform.requests, 1)
	require.False(t, platform.requests[0].NeedsSecret)
	require.Equal(t, models.AgentThunderStatusCompleted, update.Status)
	require.NotNil(t, update.SecretRefPath)
	require.Empty(t, *update.SecretRefPath)
	require.Empty(t, store.CreateSecretCalls())
}

func TestExternalSourceUsesTemporarySecretForActivationWithoutStoringIt(t *testing.T) {
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a",
		EnvironmentName: "development", ProvisioningType: models.AgentProvisioningTypeExternal,
	}
	var update repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc:     noExistingBindings,
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
			update = value
			return nil
		},
	}
	activated := false
	provider := projectAgentIdentityProviderStub{
		provision: func(_ context.Context, request ProjectAgentIdentityRequest) (ProjectAgentIdentityResult, error) {
			require.True(t, request.NeedsSecret)
			return ProjectAgentIdentityResult{AgentID: "agent-id", ClientID: "client-id", ClientSecret: "temporary-secret"}, nil
		},
		activate: func(_ context.Context, request ProjectAgentIdentityRequest, agentID string) error {
			activated = true
			require.Equal(t, "agent-id", agentID)
			require.Equal(t, "temporary-secret", request.ClientSecret)
			require.False(t, request.NeedsSecret, "Activate for an external agent is not asked to store a secret")
			return nil
		},
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			t.Fatal("external source secret must not be stored")
			return "", nil
		},
	}
	svc := newPromotionService(t, repo, store, provider, sourceOnlyReader("development", nil))
	svc.AttemptProvision(context.Background(), binding)

	require.True(t, activated, "the temporary secret reaches Activate")
	require.Empty(t, store.CreateSecretCalls(), "an external agent's secret is never stored")
	require.Equal(t, models.AgentThunderStatusCompleted, update.Status)
	require.NotNil(t, update.SecretRefPath)
	require.Empty(t, *update.SecretRefPath)
}

func TestExternalSourceFailsWhenTheProviderReturnsNoSecret(t *testing.T) {
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a",
		EnvironmentName: "development", ProvisioningType: models.AgentProvisioningTypeExternal,
	}
	var update repositories.AgentThunderAttemptUpdate
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
			update = value
			return nil
		},
	}
	provider := projectAgentIdentityProviderStub{
		provision: func(_ context.Context, request ProjectAgentIdentityRequest) (ProjectAgentIdentityResult, error) {
			require.True(t, request.NeedsSecret)
			return ProjectAgentIdentityResult{AgentID: "agent-id", ClientID: "client-id"}, nil
		},
		activate: func(context.Context, ProjectAgentIdentityRequest, string) error {
			t.Fatal("an environment without a secret value must not be activated")
			return nil
		},
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			t.Fatal("nothing is stored for an external agent")
			return "", nil
		},
	}
	svc := newPromotionService(t, repo, store, provider, sourceOnlyReader("development", nil))

	svc.AttemptProvision(context.Background(), binding)

	require.Equal(t, models.AgentThunderStatusPending, update.Status, "the attempt fails and is retried")
	require.NotEmpty(t, update.LastError)
	require.Nil(t, update.SecretRefPath, "nothing is stored")
	require.Empty(t, store.CreateSecretCalls())
}

func TestPromotionIsNeverCalledBeforeTheSourceBindingIsCompleted(t *testing.T) {
	notCompleted := map[string]struct {
		source *models.AgentThunderClient
		err    error
	}{
		"pending":     {source: &models.AgentThunderClient{Status: models.AgentThunderStatusPending}},
		"in progress": {source: &models.AgentThunderClient{Status: models.AgentThunderStatusInProgress}},
		"read error":  {err: errors.New("database unavailable")},
	}
	for name, tc := range notCompleted {
		t.Run(name, func(t *testing.T) {
			var releasedNote string
			repo := &repomocks.AgentThunderClientRepositoryMock{
				ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
				GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
					return tc.source, tc.err
				},
				ReleaseClaimFunc: func(_ context.Context, _ uuid.UUID, next time.Time, note string) error {
					releasedNote = note
					assert.True(t, next.After(time.Now()))
					return nil
				},
				UpdateAfterAttemptFunc: func(context.Context, uuid.UUID, repositories.AgentThunderAttemptUpdate) error {
					t.Fatal("deferring must not record an attempt, which would consume the retry budget")
					return nil
				},
			}
			provider := projectAgentIdentityPromotionProviderStub{
				promote: func(context.Context, ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
					t.Fatal("Promote must not run before the source binding is completed")
					return ProjectAgentIdentityResult{}, nil
				},
			}
			svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, provider, nil)

			svc.AttemptProvision(context.Background(), promotedBinding(models.AgentProvisioningTypeInternal))

			require.Len(t, repo.ReleaseClaimCalls(), 1)
			require.NotEmpty(t, releasedNote)
		})
	}
}

func TestPromotionTargetFailsWhenItsSourceIsMissingOrFailed(t *testing.T) {
	sources := map[string]struct {
		source *models.AgentThunderClient
		err    error
	}{
		"failed":  {source: &models.AgentThunderClient{Status: models.AgentThunderStatusFailed}},
		"missing": {err: repositories.ErrAgentThunderClientNotFound},
	}
	for name, tc := range sources {
		t.Run(name, func(t *testing.T) {
			var update repositories.AgentThunderAttemptUpdate
			var logs bytes.Buffer
			repo := &repomocks.AgentThunderClientRepositoryMock{
				ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
				GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
					return tc.source, tc.err
				},
				UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
					update = value
					return nil
				},
				ReleaseClaimFunc: func(context.Context, uuid.UUID, time.Time, string) error {
					t.Fatal("a target whose source is gone or failed must not wait")
					return nil
				},
			}
			provider := projectAgentIdentityPromotionProviderStub{
				promote: func(context.Context, ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
					t.Fatal("Promote must not run without a completed source")
					return ProjectAgentIdentityResult{}, nil
				},
			}
			svc := NewAgentThunderProvisioningService(repo, &clientmocks.EnvThunderResolverMock{},
				&clientmocks.SecretManagementClientMock{}, slog.New(slog.NewTextHandler(&logs, nil)))
			require.NoError(t, svc.(ProjectAgentIdentityProviderSetter).SetProjectAgentIdentityProvider(
				thundersvc.IdentityManagementCPManaged, provider,
			))

			svc.AttemptProvision(context.Background(), promotedBinding(models.AgentProvisioningTypeInternal))

			require.Len(t, repo.UpdateAfterAttemptCalls(), 1, "the failure goes through the normal attempt accounting")
			require.Equal(t, models.AgentThunderStatusPending, update.Status)
			require.Equal(t, "The source environment's identity is missing or failed. Fix it first, then retry.", update.LastError)
			require.Contains(t, logs.String(), ErrProjectAgentIdentitySourceUnavailable.Error())
		})
	}

	t.Run("exhausted budget marks the target FAILED", func(t *testing.T) {
		var update repositories.AgentThunderAttemptUpdate
		repo := &repomocks.AgentThunderClientRepositoryMock{
			ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
			GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
				return nil, repositories.ErrAgentThunderClientNotFound
			},
			UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
				update = value
				return nil
			},
		}
		svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{},
			(&fakeTargetPlatform{}).provider(t), nil)
		binding := promotedBinding(models.AgentProvisioningTypeInternal)
		binding.AttemptCount = maxProvisionAttempts - 1

		svc.AttemptProvision(context.Background(), binding)

		require.Equal(t, models.AgentThunderStatusFailed, update.Status)
	})
}

func TestUserFacingProvisioningErrorForAnUnavailableSource(t *testing.T) {
	cause := fmt.Errorf("%w: development", ErrProjectAgentIdentitySourceUnavailable)
	require.Equal(t, "The source environment's identity is missing or failed. Fix it first, then retry.",
		userFacingProvisioningError(cause))
}

func TestPromotionSourceReadErrorIsLoggedAndDefersWithoutConsumingAnAttempt(t *testing.T) {
	var logs bytes.Buffer
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return nil, errors.New("database unavailable")
		},
		ReleaseClaimFunc: func(context.Context, uuid.UUID, time.Time, string) error { return nil },
		UpdateAfterAttemptFunc: func(context.Context, uuid.UUID, repositories.AgentThunderAttemptUpdate) error {
			t.Fatal("a read error must not consume an attempt")
			return nil
		},
	}
	provider := projectAgentIdentityPromotionProviderStub{
		promote: func(context.Context, ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			t.Fatal("Promote must not run when the source cannot be read")
			return ProjectAgentIdentityResult{}, nil
		},
	}
	svc := NewAgentThunderProvisioningService(repo, &clientmocks.EnvThunderResolverMock{},
		&clientmocks.SecretManagementClientMock{}, slog.New(slog.NewTextHandler(&logs, nil)))
	require.NoError(t, svc.(ProjectAgentIdentityProviderSetter).SetProjectAgentIdentityProvider(
		thundersvc.IdentityManagementCPManaged, provider,
	))

	svc.AttemptProvision(context.Background(), promotedBinding(models.AgentProvisioningTypeInternal))

	require.Len(t, repo.ReleaseClaimCalls(), 1)
	require.Contains(t, logs.String(), "level=ERROR")
	require.Contains(t, logs.String(), "database unavailable")
	require.Contains(t, logs.String(), "agentName=agent-a")
	require.Contains(t, logs.String(), "envName=stage")
}

func TestPromotionRetryReusesTheTargetClientIDAfterEveryFailure(t *testing.T) {
	failures := map[string]func(ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error){
		"promote error": func(ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			return ProjectAgentIdentityResult{}, errors.New("response lost after apply")
		},
		"incomplete identity": func(ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			return ProjectAgentIdentityResult{}, nil
		},
		"missing secret": func(r ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			return ProjectAgentIdentityResult{AgentID: r.ExistingAgentID, ClientID: r.TargetClientID}, nil
		},
		"mismatched credential": func(r ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			return ProjectAgentIdentityResult{AgentID: r.ExistingAgentID, ClientID: "other-client", ClientSecret: "other"}, nil
		},
	}
	for name, fail := range failures {
		t.Run(name, func(t *testing.T) {
			var updates []repositories.AgentThunderAttemptUpdate
			repo := &repomocks.AgentThunderClientRepositoryMock{
				ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
				GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
					return completedSource(), nil
				},
				UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
					updates = append(updates, value)
					return nil
				},
			}
			var requests []ProjectAgentIdentityPromotionRequest
			provider := projectAgentIdentityPromotionProviderStub{
				promote: func(_ context.Context, request ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
					requests = append(requests, request)
					if len(requests) == 1 {
						return fail(request)
					}
					return ProjectAgentIdentityResult{
						AgentID: request.ExistingAgentID, ClientID: request.TargetClientID, ClientSecret: request.TargetClientSecret,
					}, nil
				},
			}
			store := &clientmocks.SecretManagementClientMock{
				CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
					return "secret-reference", nil
				},
			}
			svc := newPromotionService(t, repo, store, provider, nil)
			binding := promotedBinding(models.AgentProvisioningTypeInternal)

			svc.AttemptProvision(context.Background(), binding)

			require.Len(t, updates, 1)
			require.Equal(t, models.AgentThunderStatusPending, updates[0].Status)
			require.NotNil(t, updates[0].ThunderClientID, "the generated client ID is persisted by the failure")
			require.Equal(t, requests[0].TargetClientID, *updates[0].ThunderClientID)

			retry := binding
			retry.ThunderClientID = *updates[0].ThunderClientID
			retry.AttemptCount = 1
			svc.AttemptProvision(context.Background(), retry)

			require.Len(t, requests, 2)
			require.Equal(t, requests[0].TargetClientID, requests[1].TargetClientID, "the retry reuses the client ID")
			require.Equal(t, models.AgentThunderStatusCompleted, updates[len(updates)-1].Status)
			require.Equal(t, requests[0].TargetClientID, *updates[len(updates)-1].ThunderClientID)
		})
	}
}

// claimSignals returns a repository claim func that reports every claimed
// binding on a channel, so a test can wait for background attempts to start.
func claimSignals(n int) (func(context.Context, uuid.UUID) (bool, error), chan uuid.UUID) {
	started := make(chan uuid.UUID, n)
	return func(_ context.Context, id uuid.UUID) (bool, error) {
		started <- id
		return false, nil
	}, started
}

func waitForClaims(t *testing.T, started chan uuid.UUID, want int) {
	t.Helper()
	for i := 0; i < want; i++ {
		select {
		case <-started:
		case <-time.After(5 * time.Second):
			t.Fatalf("only %d of %d background attempts started", i, want)
		}
	}
}

func TestProvisionForEnvironmentIfMissingPromotesIntoANewEnvironment(t *testing.T) {
	var upserted []models.AgentThunderClient
	repo := provisionRepo(&upserted)
	claim, started := claimSignals(4)
	repo.ClaimForAttemptFunc = claim
	// The agent already lives in development; stage and production are new.
	repo.GetFunc = func(_ context.Context, _, _, _, env string) (*models.AgentThunderClient, error) {
		if env == "development" {
			return &models.AgentThunderClient{EnvironmentName: env, RequestedBy: "user-1"}, nil
		}
		return nil, repositories.ErrAgentThunderClientNotFound
	}
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, (&fakeTargetPlatform{}).provider(t),
		projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
			return "development", linearTopology(), nil
		}))

	alreadyExisted, err := svc.ProvisionForEnvironmentIfMissing(context.Background(),
		"org-a", "project-a", "agent-a", "stage", models.AgentProvisioningTypeInternal, "user-2")

	require.NoError(t, err, "a new environment is promoted into instead of rejected")
	require.False(t, alreadyExisted)
	require.Len(t, upserted, 2)
	require.Equal(t, "stage", upserted[0].EnvironmentName)
	require.Equal(t, "development", upserted[0].PromotionSource)
	require.Equal(t, "production", upserted[1].EnvironmentName)
	require.Equal(t, "stage", upserted[1].PromotionSource)
	require.Equal(t, models.AgentProvisioningTypeInternal, upserted[0].ProvisioningType)
	require.Equal(t, "user-2", upserted[0].RequestedBy)
	waitForClaims(t, started, 2)
}

func TestProvisionForEnvironmentIfMissingStillRejectsANonSourceEnvironmentWithoutPromotion(t *testing.T) {
	cases := map[string]struct {
		provider     ProjectAgentIdentityProvider
		reader       ProjectPipelineReader
		wantConflict bool
	}{
		"provider cannot promote": {
			provider:     projectAgentIdentityProviderStub{},
			reader:       sourceOnlyReader("development", nil),
			wantConflict: true,
		},
		"topology cannot be resolved": {
			provider: (&fakeTargetPlatform{}).provider(t),
			reader:   sourceOnlyReader("", errors.New("pipeline unavailable")),
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var upserted []models.AgentThunderClient
			svc := newPromotionService(t, provisionRepo(&upserted), &clientmocks.SecretManagementClientMock{}, tc.provider, tc.reader)

			_, err := svc.ProvisionForEnvironmentIfMissing(context.Background(),
				"org-a", "project-a", "agent-a", "stage", models.AgentProvisioningTypeInternal, "user-1")

			require.Error(t, err)
			require.Equal(t, tc.wantConflict, errors.Is(err, ErrProjectAgentIdentityRequiresPromotion))
			require.Empty(t, upserted)
		})
	}
}

func TestProvisionForEnvironmentIfMissingStillProvisionsTheSourceEnvironmentDirectly(t *testing.T) {
	var upserted []models.AgentThunderClient
	repo := provisionRepo(&upserted)
	claim, started := claimSignals(2)
	repo.ClaimForAttemptFunc = claim
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, (&fakeTargetPlatform{}).provider(t),
		projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
			return "development", linearTopology(), nil
		}))

	_, err := svc.ProvisionForEnvironmentIfMissing(context.Background(),
		"org-a", "project-a", "agent-a", "development", models.AgentProvisioningTypeInternal, "user-1")

	require.NoError(t, err)
	require.Len(t, upserted, 3, "unchanged: the source environment goes through ProvisionForAgent")
	waitForClaims(t, started, 3)
}

func TestWritePromotionBindingsNeverCreatesASecondSource(t *testing.T) {
	ref := repositories.AgentRef{OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", ProvisioningType: models.AgentProvisioningTypeInternal}
	steps := []promotionStep{{Environment: "sandbox"}, {Environment: "development", Source: "sandbox"}}

	cases := map[string]struct {
		existing    []models.AgentThunderClient
		wantUpserts int
	}{
		"existing home elsewhere": {
			existing: []models.AgentThunderClient{
				{EnvironmentName: "development"},
				{EnvironmentName: "stage", PromotionSource: "development"},
			},
			wantUpserts: 0,
		},
		"only promoted rows exist": {
			existing:    []models.AgentThunderClient{{EnvironmentName: "stage", PromotionSource: "development"}},
			wantUpserts: 2,
		},
		"no rows exist": {wantUpserts: 2},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			repo := &repomocks.AgentThunderClientRepositoryMock{
				FindByAgentFunc: func(context.Context, string, string, string) ([]models.AgentThunderClient, error) {
					return tc.existing, nil
				},
				GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
					return nil, repositories.ErrAgentThunderClientNotFound
				},
				UpsertFunc: func(_ context.Context, binding *models.AgentThunderClient) error {
					binding.ID = uuid.New()
					return nil
				},
			}
			svc := NewAgentThunderProvisioningService(repo, &clientmocks.EnvThunderResolverMock{},
				&clientmocks.SecretManagementClientMock{}, slog.New(slog.NewTextHandler(&logs, nil)))
			impl := svc.(*agentThunderProvisioningService)

			created, err := impl.writePromotionBindings(context.Background(), ref, steps, "user-1")

			require.NoError(t, err)
			require.Len(t, created, tc.wantUpserts)
			require.Len(t, repo.UpsertCalls(), tc.wantUpserts)
			if tc.wantUpserts == 0 {
				require.Equal(t, 1, strings.Count(logs.String(), "level=WARN"), "one warning, not one per step")
				require.Contains(t, logs.String(), "currentSource=development")
				require.Contains(t, logs.String(), "pipelineSource=sandbox")
				require.Contains(t, logs.String(), "agentName=agent-a")
			} else {
				require.NotContains(t, logs.String(), "level=WARN")
			}
		})
	}
}

func TestWritePromotionBindingsTakesRequestedByFromTheSourceBinding(t *testing.T) {
	ref := repositories.AgentRef{OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", ProvisioningType: models.AgentProvisioningTypeInternal}
	steps := []promotionStep{
		{Environment: "development"},
		{Environment: "stage", Source: "development"},
		{Environment: "production", Source: "stage"},
	}
	newService := func(sourceRead func(call int) (*models.AgentThunderClient, error)) (*agentThunderProvisioningService, *repomocks.AgentThunderClientRepositoryMock) {
		sourceReads := 0
		repo := &repomocks.AgentThunderClientRepositoryMock{
			FindByAgentFunc: noExistingBindings,
			GetFunc: func(_ context.Context, _, _, _, env string) (*models.AgentThunderClient, error) {
				if env == "development" {
					sourceReads++
					return sourceRead(sourceReads)
				}
				return nil, repositories.ErrAgentThunderClientNotFound
			},
			UpsertFunc: func(_ context.Context, binding *models.AgentThunderClient) error {
				binding.ID = uuid.New()
				return nil
			},
		}
		svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, (&fakeTargetPlatform{}).provider(t), nil)
		return svc.(*agentThunderProvisioningService), repo
	}

	t.Run("empty requestedBy is read once from the source", func(t *testing.T) {
		impl, repo := newService(func(call int) (*models.AgentThunderClient, error) {
			if call == 1 {
				return &models.AgentThunderClient{EnvironmentName: "development"}, nil // the existence check
			}
			return &models.AgentThunderClient{EnvironmentName: "development", RequestedBy: "user-1"}, nil
		})

		created, err := impl.writePromotionBindings(context.Background(), ref, steps, "")

		require.NoError(t, err)
		require.Len(t, created, 2)
		for _, call := range repo.UpsertCalls() {
			require.Equal(t, "user-1", call.Client.RequestedBy)
		}
		require.Len(t, repo.GetCalls(), 4, "development (existence), stage, development (requestedBy lookup), production")
	})

	t.Run("a given requestedBy is kept and nothing extra is read", func(t *testing.T) {
		impl, repo := newService(func(call int) (*models.AgentThunderClient, error) {
			return &models.AgentThunderClient{EnvironmentName: "development", RequestedBy: "user-1"}, nil
		})
		_, err := impl.writePromotionBindings(context.Background(), ref, steps, "user-2")
		require.NoError(t, err)
		for _, call := range repo.UpsertCalls() {
			require.Equal(t, "user-2", call.Client.RequestedBy)
		}
		require.Len(t, repo.GetCalls(), 3)
	})

	t.Run("an unreadable source prevents target rows", func(t *testing.T) {
		impl, repo := newService(func(call int) (*models.AgentThunderClient, error) {
			if call == 1 {
				return &models.AgentThunderClient{EnvironmentName: "development"}, nil
			}
			return nil, errors.New("database unavailable")
		})
		created, err := impl.writePromotionBindings(context.Background(), ref, steps, "")
		require.ErrorContains(t, err, "read project AgentID source binding for requestedBy")
		require.Empty(t, created)
		require.Empty(t, repo.UpsertCalls())
	})
}

func provisionRequestedBy(t *testing.T, requestedBy string) string {
	t.Helper()
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a",
		EnvironmentName: "development", ProvisioningType: models.AgentProvisioningTypeInternal,
		RequestedBy: requestedBy,
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		FindByAgentFunc:        noExistingBindings,
		ClaimForAttemptFunc:    func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		UpdateAfterAttemptFunc: func(context.Context, uuid.UUID, repositories.AgentThunderAttemptUpdate) error { return nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			return "secret-reference", nil
		},
	}
	var provisioned, activated *ProjectAgentIdentityRequest
	provider := projectAgentIdentityProviderStub{
		provision: func(_ context.Context, request ProjectAgentIdentityRequest) (ProjectAgentIdentityResult, error) {
			provisioned = &request
			return ProjectAgentIdentityResult{AgentID: "agent-id", ClientID: "client-id", ClientSecret: "secret"}, nil
		},
		activate: func(_ context.Context, request ProjectAgentIdentityRequest, _ string) error {
			activated = &request
			return nil
		},
	}
	svc := newPromotionService(t, repo, store, provider, sourceOnlyReader("development", nil))

	svc.AttemptProvision(context.Background(), binding)

	require.NotNil(t, provisioned)
	require.NotNil(t, activated)
	require.Equal(t, provisioned.RequestedBy, activated.RequestedBy)
	return provisioned.RequestedBy
}

func TestProvisionRequestCarriesTheBindingRequestedBy(t *testing.T) {
	require.Equal(t, "creator-sub", provisionRequestedBy(t, "creator-sub"))
}

func TestProvisionRequestKeepsAnEmptyRequestedByEmpty(t *testing.T) {
	require.Empty(t, provisionRequestedBy(t, ""))
}

func TestPromotedBindingRequestedByIsUnaffected(t *testing.T) {
	binding := promotedBinding(models.AgentProvisioningTypeInternal)
	binding.RequestedBy = "creator-sub"
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			source := completedSource()
			source.RequestedBy = "creator-sub"
			return source, nil
		},
		UpdateAfterAttemptFunc: func(context.Context, uuid.UUID, repositories.AgentThunderAttemptUpdate) error { return nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			return "target-secret-reference", nil
		},
	}
	platform := &fakeTargetPlatform{}
	svc := newPromotionService(t, repo, store, platform.provider(t), nil)

	svc.AttemptProvision(context.Background(), binding)

	require.Len(t, platform.requests, 1, "promotion still runs through Promote, never Provision")
	require.Equal(t, "creator-sub", binding.RequestedBy)
}

// creatorRequest runs a source attempt for a repair binding (requestedBy is the
// repair caller) against an agent whose other bindings are existing, and returns
// the requests the provider saw, the update written, and the logs.
func creatorRequest(
	t *testing.T, repairCaller string, existing []models.AgentThunderClient,
) (provisioned, activated *ProjectAgentIdentityRequest, logs string) {
	t.Helper()
	binding := models.AgentThunderClient{
		ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a",
		EnvironmentName: "development", ProvisioningType: models.AgentProvisioningTypeInternal,
		RequestedBy: repairCaller,
	}
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		FindByAgentFunc: func(context.Context, string, string, string) ([]models.AgentThunderClient, error) {
			return existing, nil
		},
		UpdateAfterAttemptFunc: func(context.Context, uuid.UUID, repositories.AgentThunderAttemptUpdate) error { return nil },
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			return "secret-reference", nil
		},
	}
	provider := projectAgentIdentityProviderStub{
		provision: func(_ context.Context, request ProjectAgentIdentityRequest) (ProjectAgentIdentityResult, error) {
			provisioned = &request
			return ProjectAgentIdentityResult{AgentID: "agent-id", ClientID: "client-id", ClientSecret: "secret"}, nil
		},
		activate: func(_ context.Context, request ProjectAgentIdentityRequest, _ string) error {
			activated = &request
			return nil
		},
	}
	var buf bytes.Buffer
	svc := newPromotionService(t, repo, store, provider, sourceOnlyReader("development", nil))
	svc.(*agentThunderProvisioningService).logger = slog.New(slog.NewTextHandler(&buf, nil))

	svc.AttemptProvision(context.Background(), binding)

	require.NotNil(t, provisioned)
	require.NotNil(t, activated)
	return provisioned, activated, buf.String()
}

func TestProviderRequestCarriesTheAgentCreator(t *testing.T) {
	now := time.Now()
	row := func(by string, age time.Duration) models.AgentThunderClient {
		return models.AgentThunderClient{RequestedBy: by, CreatedAt: now.Add(-age)}
	}

	t.Run("a repair binding uses the creator of the older binding for Provision and Activate", func(t *testing.T) {
		provisioned, activated, _ := creatorRequest(t, "repair-caller", []models.AgentThunderClient{
			row("repair-caller", time.Minute), row("creator", time.Hour),
		})
		require.Equal(t, "creator", provisioned.RequestedBy)
		require.Equal(t, "creator", activated.RequestedBy)
	})

	t.Run("the only binding is the agent's own", func(t *testing.T) {
		provisioned, activated, _ := creatorRequest(t, "creator", []models.AgentThunderClient{row("creator", time.Minute)})
		require.Equal(t, "creator", provisioned.RequestedBy)
		require.Equal(t, "creator", activated.RequestedBy)
	})

	t.Run("no bindings at all falls back to the binding's requester", func(t *testing.T) {
		provisioned, _, _ := creatorRequest(t, "only-known", nil)
		require.Equal(t, "only-known", provisioned.RequestedBy)
	})

	t.Run("the oldest binding wins even when its requester is empty", func(t *testing.T) {
		provisioned, activated, _ := creatorRequest(t, "repair-caller", []models.AgentThunderClient{
			row("newer", time.Minute), row("", 3*time.Hour), row("older", time.Hour),
		})
		require.Empty(t, provisioned.RequestedBy, "a system-created agent has no creator")
		require.Empty(t, activated.RequestedBy)
	})

	t.Run("the oldest binding with a requester still wins when it is the oldest", func(t *testing.T) {
		provisioned, activated, _ := creatorRequest(t, "repair-caller", []models.AgentThunderClient{
			row("newer", time.Minute), row("older", time.Hour), row("", 30*time.Minute),
		})
		require.Equal(t, "older", provisioned.RequestedBy)
		require.Equal(t, "older", activated.RequestedBy)
	})

	t.Run("a read error defers the attempt and never sends an unverified subject", func(t *testing.T) {
		binding := models.AgentThunderClient{
			ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a",
			EnvironmentName: "development", ProvisioningType: models.AgentProvisioningTypeInternal,
			RequestedBy: "repair-caller",
		}
		var released int
		repo := &repomocks.AgentThunderClientRepositoryMock{
			ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
			FindByAgentFunc: func(context.Context, string, string, string) ([]models.AgentThunderClient, error) {
				return nil, errors.New("database unavailable")
			},
			ReleaseClaimFunc: func(context.Context, uuid.UUID, time.Time, string) error { released++; return nil },
		}
		store := &clientmocks.SecretManagementClientMock{}
		called := false
		provider := projectAgentIdentityProviderStub{
			provision: func(context.Context, ProjectAgentIdentityRequest) (ProjectAgentIdentityResult, error) {
				called = true
				return ProjectAgentIdentityResult{}, errors.New("must not be called")
			},
		}
		var buf bytes.Buffer
		svc := newPromotionService(t, repo, store, provider, sourceOnlyReader("development", nil))
		svc.(*agentThunderProvisioningService).logger = slog.New(slog.NewTextHandler(&buf, nil))

		svc.AttemptProvision(context.Background(), binding)

		require.False(t, called, "no provider call with an unverified creator")
		require.Equal(t, 1, released, "the claim is released for a later retry")
		require.Len(t, repo.UpdateAfterAttemptCalls(), 0, "no attempt is consumed")
		require.Equal(t, 1, strings.Count(buf.String(), "to find its creator"), "exactly one creator warning")
		require.NotContains(t, buf.String(), "repair-caller")
	})
}

// In CP_MANAGED mode an AgentID attempt takes 10 to 18 seconds, even when it succeeds, so agent
// creation must not hold the request for it: it only made the create request run past the route's
// limit. The attempt still runs, and its outcome is recorded.
func TestProvisionForAgentNeverHoldsTheCreateRequestInCPManagedMode(t *testing.T) {
	release := make(chan struct{})
	claimed := make(chan struct{}, 4)
	var upserted []models.AgentThunderClient
	repo := provisionRepo(&upserted)
	repo.ClaimForAttemptFunc = func(context.Context, uuid.UUID) (bool, error) {
		claimed <- struct{}{}
		<-release // a slow attempt: it stays in flight until the test lets it go
		return false, nil
	}
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, (&fakeTargetPlatform{}).provider(t),
		projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
			return "development", linearTopology(), nil
		}))
	svc.(*agentThunderProvisioningService).awaitTimeout = 30 * time.Second // a wait would show

	start := time.Now()
	svc.ProvisionForAgent(context.Background(), "org-a", "project-a", "agent-a", models.AgentProvisioningTypeInternal,
		[]string{"development"}, "user-1", "development")
	assert.Less(t, time.Since(start), 2*time.Second, "the create request is not held for the AgentID")
	require.Len(t, upserted, 3, "a binding for every environment of the pipeline is written first")

	select {
	case <-claimed:
	case <-time.After(5 * time.Second):
		t.Fatal("the attempt must still start, in the background")
	}
	close(release)
}

// LEGACY_DIRECT keeps the wait: an attempt through an environment's own Thunder is fast, so the
// credentials go into the agent's first ReleaseBinding write.
func TestProvisionForAgentStillWaitsInLegacyDirectMode(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{}, 4)
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		UpsertFunc:          func(context.Context, *models.AgentThunderClient) error { return nil },
		UpdateAfterAttemptFunc: func(context.Context, uuid.UUID, repositories.AgentThunderAttemptUpdate) error {
			return nil
		},
	}
	resolver := &clientmocks.EnvThunderResolverMock{
		ResolveFunc: func(context.Context, string, string, string) (thundersvc.ThunderClient, error) {
			started <- struct{}{}
			<-release
			return nil, errors.New("unused in this test")
		},
	}
	svc := newTestProvisioningService(repo, resolver, &clientmocks.SecretManagementClientMock{})
	svc.(*agentThunderProvisioningService).awaitTimeout = 200 * time.Millisecond

	start := time.Now()
	svc.ProvisionForAgent(context.Background(), "acme", "proj1", "my-agent", models.AgentProvisioningTypeInternal, []string{"dev"}, "", "dev")
	assert.GreaterOrEqual(t, time.Since(start), 150*time.Millisecond, "the awaited environment is still waited for")
	<-started
	close(release)
}
