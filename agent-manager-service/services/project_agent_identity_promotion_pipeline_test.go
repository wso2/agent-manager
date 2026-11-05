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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

const notInPipelineMessage = "This environment is no longer a promotion target in the project's pipeline. Add it back to the pipeline, then retry."

const stalePathNote = "Waiting for the identity service to open the promotion path to this environment."

// topologyXA is x -> a. topologyXYA is x -> y -> a: y was inserted in front of a.
func topologyXA() []ProjectIdentityEnvironment {
	return []ProjectIdentityEnvironment{{Name: "x", PromotesTo: []string{"a"}}, {Name: "a"}}
}

func topologyXYA() []ProjectIdentityEnvironment {
	return []ProjectIdentityEnvironment{
		{Name: "x", PromotesTo: []string{"y"}}, {Name: "y", PromotesTo: []string{"a"}}, {Name: "a"},
	}
}

func bindingIn(env string, status models.AgentThunderStatus) *models.AgentThunderClient {
	return &models.AgentThunderClient{
		OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", EnvironmentName: env,
		Status: status, ThunderAgentID: "agent-" + env, ThunderClientID: "client-" + env,
	}
}

// pendingA is the not yet completed binding of environment a that stores source x.
func pendingA(ownership models.AgentProvisioningType) models.AgentThunderClient {
	return models.AgentThunderClient{
		ID: uuid.New(), OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a",
		EnvironmentName: "a", PromotionSource: "x", ProvisioningType: ownership,
	}
}

// promotionAttemptRun records what one promoted attempt did.
type promotionAttemptRun struct {
	requests []ProjectAgentIdentityPromotionRequest
	updates  []repositories.AgentThunderAttemptUpdate
	released []string
	reads    []string
	logs     string
}

// pipelineSourceOf is the first environment that no other environment promotes
// to, or "x" when the list is empty (a pipeline read error case).
func pipelineSourceOf(environments []ProjectIdentityEnvironment) string {
	targets := map[string]bool{}
	for _, environment := range environments {
		for _, target := range environment.PromotesTo {
			targets[target] = true
		}
	}
	for _, environment := range environments {
		if !targets[environment.Name] {
			return environment.Name
		}
	}
	return "x"
}

// runPromotedAttempt runs AttemptProvision for binding against a repository
// holding bindings (environment name to row; missing means not found) and a
// pipeline reader returning environments (or pipelineErr).
func runPromotedAttempt(
	t *testing.T, binding models.AgentThunderClient, environments []ProjectIdentityEnvironment, pipelineErr error,
	bindings map[string]*models.AgentThunderClient, promote func(ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error),
) *promotionAttemptRun {
	t.Helper()
	run := &promotionAttemptRun{}
	var logs bytes.Buffer
	var mu sync.Mutex
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		GetFunc: func(_ context.Context, _, _, _, env string) (*models.AgentThunderClient, error) {
			mu.Lock()
			defer mu.Unlock()
			run.reads = append(run.reads, env)
			if row, ok := bindings[env]; ok {
				return row, nil
			}
			return nil, repositories.ErrAgentThunderClientNotFound
		},
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
			run.updates = append(run.updates, value)
			return nil
		},
		ReleaseClaimFunc: func(_ context.Context, _ uuid.UUID, next time.Time, note string) error {
			require.True(t, next.After(time.Now()))
			run.released = append(run.released, note)
			return nil
		},
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			return "secret-reference", nil
		},
	}
	provider := projectAgentIdentityPromotionProviderStub{
		promote: func(_ context.Context, request ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			run.requests = append(run.requests, request)
			if promote != nil {
				return promote(request)
			}
			return ProjectAgentIdentityResult{
				AgentID: request.ExistingAgentID, ClientID: request.TargetClientID, ClientSecret: request.TargetClientSecret,
			}, nil
		},
	}
	svc := NewAgentThunderProvisioningService(repo, &clientmocks.EnvThunderResolverMock{}, store, nil,
		slog.New(slog.NewTextHandler(&logs, nil)))
	require.NoError(t, svc.(ProjectAgentIdentityProviderSetter).SetProjectAgentIdentityProvider(
		thundersvc.IdentityManagementCPManaged, provider))
	svc.(ProjectPipelineReaderSetter).SetProjectPipelineReader(projectPipelineReaderFunc(
		func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
			return pipelineSourceOf(environments), environments, pipelineErr
		}))

	svc.AttemptProvision(context.Background(), binding)

	run.logs = logs.String()
	return run
}

func TestEffectivePromotionSourceKeepsTheStoredSourceWhileItIsStillAParent(t *testing.T) {
	run := runPromotedAttempt(t, pendingA(models.AgentProvisioningTypeInternal), topologyXA(), nil,
		map[string]*models.AgentThunderClient{"x": bindingIn("x", models.AgentThunderStatusCompleted)}, nil)

	require.Len(t, run.requests, 1)
	require.Equal(t, "x", run.requests[0].SourceEnvironment)
	require.Equal(t, "a", run.requests[0].TargetEnvironment)
	require.Equal(t, []string{"x"}, run.reads)
}

func TestEffectivePromotionSourceReplacesAStaleStoredSource(t *testing.T) {
	t.Run("current parent is completed", func(t *testing.T) {
		run := runPromotedAttempt(t, pendingA(models.AgentProvisioningTypeInternal), topologyXYA(), nil,
			map[string]*models.AgentThunderClient{
				"x": bindingIn("x", models.AgentThunderStatusCompleted),
				"y": bindingIn("y", models.AgentThunderStatusCompleted),
			}, nil)

		require.Len(t, run.requests, 1)
		require.Equal(t, "y", run.requests[0].SourceEnvironment, "the stored source x is no longer a parent of a")
		require.Equal(t, "agent-y", run.requests[0].ExistingAgentID, "the AgentID comes from the current parent")
		require.Equal(t, "client-y", run.requests[0].ExistingClientID)
	})

	t.Run("current parent is still pending so the target waits for it", func(t *testing.T) {
		run := runPromotedAttempt(t, pendingA(models.AgentProvisioningTypeInternal), topologyXYA(), nil,
			map[string]*models.AgentThunderClient{
				"x": bindingIn("x", models.AgentThunderStatusCompleted),
				"y": bindingIn("y", models.AgentThunderStatusPending),
			}, nil)

		require.Empty(t, run.requests, "x is completed but is not the parent, so it must not be promoted from")
		require.Equal(t, []string{"Waiting for the source environment identity."}, run.released)
		require.Empty(t, run.updates, "waiting does not consume an attempt")
		require.Equal(t, "y", run.reads[len(run.reads)-1])
	})
}

func multiParentTopology() []ProjectIdentityEnvironment {
	return []ProjectIdentityEnvironment{
		{Name: "p2", PromotesTo: []string{"a"}}, {Name: "p1", PromotesTo: []string{"a"}}, {Name: "a"},
	}
}

func TestEffectivePromotionSourceChoosesAmongSeveralParents(t *testing.T) {
	cases := map[string]struct {
		p1, p2         *models.AgentThunderClient
		wantSource     string
		wantRequest    bool
		wantUnavail    bool
		wantDeferredOn string
	}{
		"prefers a completed parent over an earlier pending one": {
			p1: bindingIn("p1", models.AgentThunderStatusPending), p2: bindingIn("p2", models.AgentThunderStatusCompleted),
			wantSource: "p2", wantRequest: true,
		},
		"first completed parent by name wins": {
			p1: bindingIn("p1", models.AgentThunderStatusCompleted), p2: bindingIn("p2", models.AgentThunderStatusCompleted),
			wantSource: "p1", wantRequest: true,
		},
		"no completed parent defers on the first usable parent": {
			p1: bindingIn("p1", models.AgentThunderStatusInProgress), p2: bindingIn("p2", models.AgentThunderStatusPending),
			wantDeferredOn: "p1",
		},
		"a failed parent is skipped": {
			p1: bindingIn("p1", models.AgentThunderStatusFailed), p2: bindingIn("p2", models.AgentThunderStatusPending),
			wantDeferredOn: "p2",
		},
		"a missing parent is skipped": {
			p1: nil, p2: bindingIn("p2", models.AgentThunderStatusCompleted),
			wantSource: "p2", wantRequest: true,
		},
		"every parent failed or missing is source unavailable": {
			p1: bindingIn("p1", models.AgentThunderStatusFailed), p2: nil, wantUnavail: true,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			bindings := map[string]*models.AgentThunderClient{}
			if tc.p1 != nil {
				bindings["p1"] = tc.p1
			}
			if tc.p2 != nil {
				bindings["p2"] = tc.p2
			}
			run := runPromotedAttempt(t, pendingA(models.AgentProvisioningTypeInternal), multiParentTopology(), nil, bindings, nil)

			switch {
			case tc.wantRequest:
				require.Len(t, run.requests, 1)
				require.Equal(t, tc.wantSource, run.requests[0].SourceEnvironment)
			case tc.wantUnavail:
				require.Empty(t, run.requests)
				require.Empty(t, run.released, "a failed parent set must not wait forever")
				require.Len(t, run.updates, 1)
				require.Equal(t, "The source environment's identity is missing or failed. Fix it first, then retry.",
					run.updates[0].LastError)
				require.Contains(t, run.logs, ErrProjectAgentIdentitySourceUnavailable.Error())
			default:
				require.Empty(t, run.requests)
				require.Len(t, run.released, 1)
				require.Empty(t, run.updates)
				require.Equal(t, tc.wantDeferredOn, run.reads[len(run.reads)-1])
			}
		})
	}
}

func TestPromotionFailsWhenTheEnvironmentLeftThePipeline(t *testing.T) {
	// a is no longer a target of any environment.
	environments := []ProjectIdentityEnvironment{{Name: "x", PromotesTo: []string{"y"}}, {Name: "y"}, {Name: "a"}}
	run := runPromotedAttempt(t, pendingA(models.AgentProvisioningTypeInternal), environments, nil,
		map[string]*models.AgentThunderClient{"x": bindingIn("x", models.AgentThunderStatusCompleted)}, nil)

	require.Empty(t, run.requests)
	require.Empty(t, run.released)
	require.Empty(t, run.reads, "no source binding is read for an environment that left the pipeline")
	require.Len(t, run.updates, 1, "the failure goes through the normal attempt accounting")
	require.Equal(t, models.AgentThunderStatusPending, run.updates[0].Status)
	require.Equal(t, notInPipelineMessage, run.updates[0].LastError)
	require.Contains(t, run.logs, ErrProjectAgentIdentityEnvironmentNotInPipeline.Error())

	require.Equal(t, notInPipelineMessage,
		userFacingProvisioningError(fmt.Errorf("%w: a", ErrProjectAgentIdentityEnvironmentNotInPipeline)))
}

func TestPromotionFallsBackToTheStoredSourceWhenThePipelineCannotBeRead(t *testing.T) {
	run := runPromotedAttempt(t, pendingA(models.AgentProvisioningTypeInternal), nil, errors.New("pipeline unavailable"),
		map[string]*models.AgentThunderClient{"x": bindingIn("x", models.AgentThunderStatusCompleted)}, nil)

	require.Len(t, run.requests, 1)
	require.Equal(t, "x", run.requests[0].SourceEnvironment)
	require.Contains(t, run.logs, "level=WARN")
	require.Contains(t, run.logs, "pipeline unavailable")
}

func TestPromotionDefersWithoutConsumingAnAttemptWhenThePathIsUnavailable(t *testing.T) {
	binding := pendingA(models.AgentProvisioningTypeInternal)
	binding.AttemptCount = 2
	run := runPromotedAttempt(t, binding, topologyXA(), nil,
		map[string]*models.AgentThunderClient{"x": bindingIn("x", models.AgentThunderStatusCompleted)},
		func(ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			return ProjectAgentIdentityResult{}, fmt.Errorf("promote x to a: %w", ErrProjectAgentIdentityPromotionPathUnavailable)
		})

	require.Len(t, run.requests, 1)
	require.Equal(t, []string{stalePathNote}, run.released, "released back to pending with the waiting note")
	require.Empty(t, run.updates, "recordFailure would consume an attempt")
	require.Contains(t, run.logs, "level=WARN")
	require.Contains(t, run.logs, "sourceEnvironment=x")
	require.Contains(t, run.logs, "targetEnvironment=a")
	require.NotContains(t, run.logs, run.requests[0].TargetClientSecret)
}

func TestPromotionPathUnavailableUsesTheRetryDelay(t *testing.T) {
	var next time.Time
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return bindingIn("x", models.AgentThunderStatusCompleted), nil
		},
		ReleaseClaimFunc: func(_ context.Context, _ uuid.UUID, at time.Time, _ string) error {
			next = at
			return nil
		},
	}
	provider := projectAgentIdentityPromotionProviderStub{
		promote: func(context.Context, ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			return ProjectAgentIdentityResult{}, ErrProjectAgentIdentityPromotionPathUnavailable
		},
	}
	svc := newPromotionService(t, repo, &clientmocks.SecretManagementClientMock{}, provider, nil)

	before := time.Now()
	svc.AttemptProvision(context.Background(), pendingA(models.AgentProvisioningTypeInternal))

	require.WithinDuration(t, before.Add(promotionPathRetryDelay), next, 5*time.Second)
}

// TestInsertedEnvironmentIsBoundAndPromotedFromTheCurrentParent covers a pipeline
// edited from x -> a to x -> y -> a: y gets a new binding promoted from x and
// a keeps its completed identity untouched. Platform hosted (internal) and
// external agents share the same path; only the secret handling differs.
func TestInsertedEnvironmentIsBoundAndPromotedFromTheCurrentParent(t *testing.T) {
	for _, ownership := range []models.AgentProvisioningType{
		models.AgentProvisioningTypeInternal, models.AgentProvisioningTypeExternal,
	} {
		t.Run(string(ownership), func(t *testing.T) {
			completedA := bindingIn("a", models.AgentThunderStatusCompleted)
			completedA.PromotionSource = "x"
			bindings := map[string]*models.AgentThunderClient{
				"x": bindingIn("x", models.AgentThunderStatusCompleted), "a": completedA,
			}
			var upserted []models.AgentThunderClient
			repo := &repomocks.AgentThunderClientRepositoryMock{
				ListHomeAgentsFunc: func(_ context.Context, after *repositories.AgentRef, _ int) ([]repositories.AgentRef, error) {
					if after != nil {
						return nil, nil
					}
					return []repositories.AgentRef{{
						OUID: "org-a", ProjectName: "project-a", AgentName: "agent-a", EnvironmentName: "x",
						ProvisioningType: ownership, RequestedBy: "user-1",
					}}, nil
				},
				GetFunc: func(_ context.Context, _, _, _, env string) (*models.AgentThunderClient, error) {
					if row, ok := bindings[env]; ok {
						return row, nil
					}
					return nil, repositories.ErrAgentThunderClientNotFound
				},
				UpsertFunc: func(_ context.Context, binding *models.AgentThunderClient) error {
					binding.ID = uuid.New()
					upserted = append(upserted, *binding)
					return nil
				},
				ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
				UpdateAfterAttemptFunc: func(_ context.Context, id uuid.UUID, _ repositories.AgentThunderAttemptUpdate) error {
					require.Equal(t, upserted[0].ID, id, "only the new y binding is attempted")
					return nil
				},
			}
			var created int
			store := &clientmocks.SecretManagementClientMock{
				CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
					created++
					return "secret-reference", nil
				},
			}
			platform := &fakeTargetPlatform{}
			svc := newPromotionService(t, repo, store, platform.provider(t),
				projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
					return "x", topologyXYA(), nil
				}))

			svc.(ProjectAgentIdentityPromotionReconciler).ReconcileProjectAgentIdentityPromotions(context.Background())

			require.Len(t, upserted, 1, "only y is new; a is not touched")
			require.Equal(t, "y", upserted[0].EnvironmentName)
			require.Equal(t, "x", upserted[0].PromotionSource)
			require.Equal(t, ownership, upserted[0].ProvisioningType)
			require.Equal(t, "user-1", upserted[0].RequestedBy)

			svc.AttemptProvision(context.Background(), upserted[0])

			require.Len(t, platform.requests, 1)
			request := platform.requests[0]
			require.Equal(t, "x", request.SourceEnvironment)
			require.Equal(t, "y", request.TargetEnvironment)
			require.Equal(t, ownership, request.ProvisioningType)
			require.Equal(t, ownership == models.AgentProvisioningTypeInternal, request.NeedsSecret)
			if ownership == models.AgentProvisioningTypeInternal {
				require.Equal(t, 1, created)
			} else {
				require.Zero(t, created, "an external agent's secret is never stored")
			}
			require.Len(t, repo.UpsertCalls(), 1)
			require.Len(t, repo.UpdateAfterAttemptCalls(), 1)
			require.Equal(t, models.AgentThunderStatusCompleted, completedA.Status)
		})
	}
}

// TestStaleStoredSourceIsPromotedFromTheInsertedEnvironment covers x -> a edited
// to x -> y -> a while the binding of a (storing source x) is still pending:
// once y completes, a is promoted from y. Platform hosted (internal) and
// external agents share the same binding path; only the secret handling differs.
func TestStaleStoredSourceIsPromotedFromTheInsertedEnvironment(t *testing.T) {
	for _, ownership := range []models.AgentProvisioningType{
		models.AgentProvisioningTypeInternal, models.AgentProvisioningTypeExternal,
	} {
		t.Run(string(ownership), func(t *testing.T) {
			run := runPromotedAttempt(t, pendingA(ownership), topologyXYA(), nil,
				map[string]*models.AgentThunderClient{
					"x": bindingIn("x", models.AgentThunderStatusCompleted),
					"y": bindingIn("y", models.AgentThunderStatusCompleted),
				}, nil)

			require.Len(t, run.requests, 1)
			request := run.requests[0]
			require.Equal(t, "y", request.SourceEnvironment)
			require.Equal(t, "a", request.TargetEnvironment)
			require.Equal(t, "agent-y", request.ExistingAgentID)
			require.Equal(t, ownership, request.ProvisioningType)
			require.Equal(t, ownership == models.AgentProvisioningTypeInternal, request.NeedsSecret)
			require.Len(t, run.updates, 1)
			require.Equal(t, models.AgentThunderStatusCompleted, run.updates[0].Status)
		})
	}
}

// TestPromotionCompletesOnceThePathOpens runs the same binding twice: the first
// attempt finds no promotion path and waits, the second finds it open and
// completes. The wait consumed no attempt, so the binding was never failed.
func TestPromotionCompletesOnceThePathOpens(t *testing.T) {
	var updates []repositories.AgentThunderAttemptUpdate
	var released []string
	repo := &repomocks.AgentThunderClientRepositoryMock{
		ClaimForAttemptFunc: func(context.Context, uuid.UUID) (bool, error) { return true, nil },
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentThunderClient, error) {
			return bindingIn("x", models.AgentThunderStatusCompleted), nil
		},
		UpdateAfterAttemptFunc: func(_ context.Context, _ uuid.UUID, value repositories.AgentThunderAttemptUpdate) error {
			updates = append(updates, value)
			return nil
		},
		ReleaseClaimFunc: func(_ context.Context, _ uuid.UUID, _ time.Time, note string) error {
			released = append(released, note)
			return nil
		},
	}
	store := &clientmocks.SecretManagementClientMock{
		CreateSecretFunc: func(context.Context, secretmanagersvc.SecretLocation, map[string]string) (string, error) {
			return "secret-reference", nil
		},
	}
	pathOpen := false
	var clientIDs []string
	provider := projectAgentIdentityPromotionProviderStub{
		promote: func(_ context.Context, request ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
			clientIDs = append(clientIDs, request.TargetClientID)
			if !pathOpen {
				return ProjectAgentIdentityResult{}, fmt.Errorf("promote x to a: %w", ErrProjectAgentIdentityPromotionPathUnavailable)
			}
			return ProjectAgentIdentityResult{
				AgentID: request.ExistingAgentID, ClientID: request.TargetClientID, ClientSecret: request.TargetClientSecret,
			}, nil
		},
	}
	svc := newPromotionService(t, repo, store, provider, projectPipelineReaderFunc(
		func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
			return "x", topologyXA(), nil
		}))
	binding := pendingA(models.AgentProvisioningTypeInternal)

	svc.AttemptProvision(context.Background(), binding)
	require.Equal(t, []string{stalePathNote}, released)
	require.Empty(t, updates)

	pathOpen = true
	svc.AttemptProvision(context.Background(), binding)

	require.Len(t, clientIDs, 2)
	require.NotEmpty(t, clientIDs[1])
	require.Len(t, updates, 1)
	require.Equal(t, models.AgentThunderStatusCompleted, updates[0].Status)
	require.Equal(t, clientIDs[1], *updates[0].ThunderClientID, "the client ID applied by the successful promote is the one stored")
	require.Len(t, released, 1, "only the first attempt waited")
}

func TestPromotionFailsDistinctlyWhenTheEnvironmentBecameThePipelineSource(t *testing.T) {
	// x was removed from x -> a -> b, so a is now the first environment: it has no
	// parent but is still in the pipeline.
	environments := []ProjectIdentityEnvironment{{Name: "a", PromotesTo: []string{"b"}}, {Name: "b"}}
	run := runPromotedAttempt(t, pendingA(models.AgentProvisioningTypeInternal), environments, nil,
		map[string]*models.AgentThunderClient{"x": bindingIn("x", models.AgentThunderStatusCompleted)}, nil)

	require.Empty(t, run.requests)
	require.Empty(t, run.released)
	require.Len(t, run.updates, 1, "the failure goes through the normal attempt accounting")
	require.Equal(t, firstEnvironmentMessage, run.updates[0].LastError)
	require.Contains(t, run.logs, ErrProjectAgentIdentityEnvironmentIsPipelineSource.Error())
	require.NotContains(t, run.logs, ErrProjectAgentIdentityEnvironmentNotInPipeline.Error())

	require.Equal(t, firstEnvironmentMessage,
		userFacingProvisioningError(fmt.Errorf("%w: a", ErrProjectAgentIdentityEnvironmentIsPipelineSource)))
}

const firstEnvironmentMessage = "This environment is now the first environment of the project's pipeline, so its identity cannot be promoted from another environment."

func TestEffectivePromotionSourceSentinels(t *testing.T) {
	svc := newPromotionService(t, &repomocks.AgentThunderClientRepositoryMock{}, &clientmocks.SecretManagementClientMock{},
		projectAgentIdentityPromotionProviderStub{}, nil).(*agentThunderProvisioningService)
	withPipeline := func(environments []ProjectIdentityEnvironment) {
		svc.pipelineReader = projectPipelineReaderFunc(func(context.Context, string, string) (string, []ProjectIdentityEnvironment, error) {
			return pipelineSourceOf(environments), environments, nil
		})
	}
	binding := pendingA(models.AgentProvisioningTypeInternal)

	withPipeline([]ProjectIdentityEnvironment{{Name: "a", PromotesTo: []string{"b"}}, {Name: "b"}})
	_, err := svc.effectivePromotionSource(context.Background(), binding)
	require.ErrorIs(t, err, ErrProjectAgentIdentityEnvironmentIsPipelineSource)
	require.NotErrorIs(t, err, ErrProjectAgentIdentityEnvironmentNotInPipeline)

	withPipeline([]ProjectIdentityEnvironment{{Name: "x", PromotesTo: []string{"y"}}, {Name: "y"}})
	_, err = svc.effectivePromotionSource(context.Background(), binding)
	require.ErrorIs(t, err, ErrProjectAgentIdentityEnvironmentNotInPipeline)
	require.NotErrorIs(t, err, ErrProjectAgentIdentityEnvironmentIsPipelineSource)

	withPipeline(topologyXA())
	source, err := svc.effectivePromotionSource(context.Background(), binding)
	require.NoError(t, err)
	require.Equal(t, "x", source, "a normal target with a parent is unchanged")
}

func promotionPathUnavailable(ProjectAgentIdentityPromotionRequest) (ProjectAgentIdentityResult, error) {
	return ProjectAgentIdentityResult{}, fmt.Errorf("promote x to a: %w", ErrProjectAgentIdentityPromotionPathUnavailable)
}

const pathUnavailableMessage = "The identity service has no promotion path to this environment yet. Retry once it is enabled."

func TestPromotionPathWaitIsBounded(t *testing.T) {
	sources := map[string]*models.AgentThunderClient{"x": bindingIn("x", models.AgentThunderStatusCompleted)}

	t.Run("a young binding keeps waiting without consuming an attempt", func(t *testing.T) {
		binding := pendingA(models.AgentProvisioningTypeInternal)
		binding.CreatedAt = time.Now().Add(-promotionPathWaitLimit / 2)
		run := runPromotedAttempt(t, binding, topologyXA(), nil, sources, promotionPathUnavailable)

		require.Equal(t, []string{stalePathNote}, run.released)
		require.Empty(t, run.updates)
	})

	t.Run("an old binding fails through the normal attempt accounting", func(t *testing.T) {
		binding := pendingA(models.AgentProvisioningTypeInternal)
		binding.CreatedAt = time.Now().Add(-2 * promotionPathWaitLimit)
		run := runPromotedAttempt(t, binding, topologyXA(), nil, sources, promotionPathUnavailable)

		require.Empty(t, run.released, "no deferral past the limit")
		require.Len(t, run.updates, 1, "the attempt is consumed like any other failure")
		require.Equal(t, pathUnavailableMessage, run.updates[0].LastError)
		require.Contains(t, run.logs, ErrProjectAgentIdentityPromotionPathUnavailable.Error())
	})

	t.Run("an old binding on its last attempt ends FAILED", func(t *testing.T) {
		binding := pendingA(models.AgentProvisioningTypeInternal)
		binding.CreatedAt = time.Now().Add(-2 * promotionPathWaitLimit)
		binding.AttemptCount = maxProvisionAttempts - 1
		run := runPromotedAttempt(t, binding, topologyXA(), nil, sources, promotionPathUnavailable)

		require.Len(t, run.updates, 1)
		require.Equal(t, models.AgentThunderStatusFailed, run.updates[0].Status)
		require.Equal(t, pathUnavailableMessage, run.updates[0].LastError)
	})

	t.Run("the message mapping", func(t *testing.T) {
		require.Equal(t, pathUnavailableMessage,
			userFacingProvisioningError(fmt.Errorf("promote project AgentID: %w", ErrProjectAgentIdentityPromotionPathUnavailable)))
	})
}
