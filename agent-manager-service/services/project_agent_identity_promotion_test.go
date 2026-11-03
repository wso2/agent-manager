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

func openChoreoPipelineClient(paths []models.PromotionPath, err error, calls *int) *clientmocks.OpenChoreoClientMock {
	return &clientmocks.OpenChoreoClientMock{
		GetProjectDeploymentPipelineFunc: func(_ context.Context, ouID, projectName string) (*models.DeploymentPipelineResponse, error) {
			if calls != nil {
				*calls++
			}
			if err != nil {
				return nil, err
			}
			return &models.DeploymentPipelineResponse{PromotionPaths: paths}, nil
		},
	}
}

func targets(names ...string) []models.TargetEnvironmentRef {
	refs := make([]models.TargetEnvironmentRef, 0, len(names))
	for _, name := range names {
		refs = append(refs, models.TargetEnvironmentRef{Name: name})
	}
	return refs
}

func TestOpenChoreoPipelineReaderOrdersALinearPipeline(t *testing.T) {
	reader := NewOpenChoreoProjectPipelineReader(openChoreoPipelineClient([]models.PromotionPath{
		{SourceEnvironmentRef: "stage", TargetEnvironmentRefs: targets("production")},
		{SourceEnvironmentRef: "development", TargetEnvironmentRefs: targets("stage")},
	}, nil, nil))

	source, environments, err := reader.ProjectPipeline(context.Background(), "org-a", "project-a")
	require.NoError(t, err)
	require.Equal(t, "development", source)
	steps, err := promotionSteps(source, environments)
	require.NoError(t, err)
	require.Equal(t, []promotionStep{
		{Environment: "development"},
		{Environment: "stage", Source: "development"},
		{Environment: "production", Source: "stage"},
	}, steps)
}

func TestOpenChoreoPipelineReaderOrdersABranchingPipeline(t *testing.T) {
	reader := NewOpenChoreoProjectPipelineReader(openChoreoPipelineClient([]models.PromotionPath{
		{SourceEnvironmentRef: "development", TargetEnvironmentRefs: targets("qa", "stage")},
		{SourceEnvironmentRef: "stage", TargetEnvironmentRefs: targets("production")},
		{SourceEnvironmentRef: "qa", TargetEnvironmentRefs: targets("production")},
	}, nil, nil))

	source, environments, err := reader.ProjectPipeline(context.Background(), "org-a", "project-a")
	require.NoError(t, err)
	require.Equal(t, "development", source)
	steps, err := promotionSteps(source, environments)
	require.NoError(t, err)
	require.Equal(t, []promotionStep{
		{Environment: "development"},
		{Environment: "qa", Source: "development"},
		{Environment: "stage", Source: "development"},
		{Environment: "production", Source: "qa"},
	}, steps)
}

func TestOpenChoreoPipelineReaderSingleEnvironment(t *testing.T) {
	reader := NewOpenChoreoProjectPipelineReader(openChoreoPipelineClient([]models.PromotionPath{
		{SourceEnvironmentRef: "development"},
	}, nil, nil))

	source, environments, err := reader.ProjectPipeline(context.Background(), "org-a", "project-a")
	require.NoError(t, err)
	steps, err := promotionSteps(source, environments)
	require.NoError(t, err)
	require.Equal(t, []promotionStep{{Environment: "development"}}, steps)
}

func TestOpenChoreoPipelineReaderDisconnectedEnvironmentIsUnreachable(t *testing.T) {
	// A second root: both are sources that are not targets, so one of them is
	// not reachable from the first.
	reader := NewOpenChoreoProjectPipelineReader(openChoreoPipelineClient([]models.PromotionPath{
		{SourceEnvironmentRef: "development", TargetEnvironmentRefs: targets("stage")},
		{SourceEnvironmentRef: "sandbox", TargetEnvironmentRefs: targets("stage")},
	}, nil, nil))

	source, environments, err := reader.ProjectPipeline(context.Background(), "org-a", "project-a")
	require.NoError(t, err)
	_, err = promotionSteps(source, environments)
	require.ErrorContains(t, err, "unreachable")
}

func TestOpenChoreoPipelineReaderErrors(t *testing.T) {
	boom := errors.New("openchoreo unavailable")
	_, _, err := NewOpenChoreoProjectPipelineReader(openChoreoPipelineClient(nil, boom, nil)).
		ProjectPipeline(context.Background(), "org-a", "project-a")
	require.ErrorIs(t, err, boom)

	_, _, err = NewOpenChoreoProjectPipelineReader(openChoreoPipelineClient(nil, nil, nil)).
		ProjectPipeline(context.Background(), "org-a", "project-a")
	require.ErrorContains(t, err, "no source environment")
}

func TestEnsureProjectPromotionBindingsWritesSourceFirstAndIsIdempotent(t *testing.T) {
	existing := map[string]bool{}
	var order []string
	repo := &repomocks.AgentThunderClientRepositoryMock{
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
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return nil, repositories.ErrAgentThunderClientNotFound
		},
		// A concurrent writer won the insert: DoNothing leaves the ID unset.
		UpsertFunc: func(context.Context, *models.AgentThunderClient) error { return nil },
	}
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, (&fakeTargetPlatform{}).provider(t),
		sourceOnlyReader("development", nil))
	created, err := svc.(*agentThunderProvisioningService).ensureProjectPromotionBindings(
		context.Background(), repositories.AgentRef{OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a"}, "")
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

	reconciler.ReconcileProjectAgentIdentityPromotions(context.Background())
	for key, count := range calls {
		require.Equal(t, 2, count, "the cache must not outlive a pass: %s", key)
	}
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
			}),
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
		}))

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

func TestPromotionIsNeverCalledBeforeTheSourceBindingIsCompleted(t *testing.T) {
	notCompleted := map[string]struct {
		source *models.AgentThunderClient
		err    error
	}{
		"pending":     {source: &models.AgentThunderClient{Status: models.AgentThunderStatusPending}},
		"in progress": {source: &models.AgentThunderClient{Status: models.AgentThunderStatusInProgress}},
		"failed":      {source: &models.AgentThunderClient{Status: models.AgentThunderStatusFailed}},
		"missing":     {err: repositories.ErrAgentThunderClientNotFound},
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
