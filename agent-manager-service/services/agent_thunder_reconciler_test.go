//go:build integration

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
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

func newTestReconciler(t *testing.T, provisioning AgentThunderProvisioningService) *agentThunderReconcilerService {
	t.Helper()
	repo := repositories.NewAgentThunderClientRepo(db.GetDB())
	return &agentThunderReconcilerService{
		provisioning: provisioning,
		repo:         repo,
		logger:       slog.Default(),
		stopCh:       make(chan struct{}),
	}
}

func TestAgentThunderReconciler_StartStop(t *testing.T) {
	svc := NewAgentThunderReconcilerService(nil, nil, repositories.NewAgentThunderClientRepo(db.GetDB()), slog.Default())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	require.NoError(t, svc.Start(ctx))
	require.NoError(t, svc.Stop())
}

func TestAgentThunderReconciler_StopIdempotent(t *testing.T) {
	svc := NewAgentThunderReconcilerService(nil, nil, repositories.NewAgentThunderClientRepo(db.GetDB()), slog.Default())
	require.NoError(t, svc.Stop())
	require.NoError(t, svc.Stop())
}

func TestAgentThunderReconciler_StopsOnContextCancel(t *testing.T) {
	s := newTestReconciler(t, nil)
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		s.runLoop(ctx)
		close(done)
	}()

	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reconciler loop did not stop after context cancellation")
	}
}

func TestAgentThunderReconciler_StopsOnStopChannel(t *testing.T) {
	s := newTestReconciler(t, nil)

	done := make(chan struct{})
	go func() {
		s.runLoop(context.Background())
		close(done)
	}()

	close(s.stopCh)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("reconciler loop did not stop after stop channel closed")
	}
}

func TestAgentThunderReconciler_AdvisoryLockBlocksConcurrentCycle(t *testing.T) {
	ctx := context.Background()

	holdTx := db.DB(ctx).Begin()
	require.NoError(t, holdTx.Error)
	defer holdTx.Rollback()

	var locked bool
	require.NoError(t, holdTx.Raw("SELECT pg_try_advisory_xact_lock(?)", reconcilerLockID).Scan(&locked).Error)
	require.True(t, locked)

	var attemptCalled atomic.Int32
	provisioning := &fakeProvisioningService{
		attemptFunc: func(_ context.Context, _ models.AgentThunderClient) { attemptCalled.Add(1) },
	}
	s := newTestReconciler(t, provisioning)

	s.runCycle(ctx)

	assert.Equal(t, int32(0), attemptCalled.Load(), "AttemptProvision must not run when another instance holds the advisory lock")
}

func TestAgentThunderReconciler_RunCycle_RetriesDueBindings(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "reconciler-cycle-agent"
	t.Cleanup(func() { _ = repo.DeleteByAgent(ctx, org, project, agent) })

	due := &models.AgentThunderClient{
		OUID: org, ProjectName: project, AgentName: agent, EnvironmentName: "dev",
		ProvisioningType: models.AgentProvisioningTypeExternal, Status: models.AgentThunderStatusPending,
	}
	require.NoError(t, repo.Upsert(ctx, due))

	notYetDue := &models.AgentThunderClient{
		OUID: org, ProjectName: project, AgentName: agent, EnvironmentName: "staging",
		ProvisioningType: models.AgentProvisioningTypeExternal, Status: models.AgentThunderStatusPending,
	}
	future := time.Now().Add(1 * time.Hour)
	notYetDue.NextRetryAt = &future
	require.NoError(t, repo.Upsert(ctx, notYetDue))

	var mu sync.Mutex
	var attempted []string
	provisioning := &fakeProvisioningService{
		attemptFunc: func(_ context.Context, b models.AgentThunderClient) {
			mu.Lock()
			defer mu.Unlock()
			attempted = append(attempted, b.EnvironmentName)
		},
	}
	s := &agentThunderReconcilerService{provisioning: provisioning, repo: repo, logger: slog.Default(), stopCh: make(chan struct{})}

	s.runCycle(ctx)

	mu.Lock()
	defer mu.Unlock()
	assert.Contains(t, attempted, "dev")
	assert.NotContains(t, attempted, "staging", "a binding whose next_retry_at is still in the future must not be retried early")
}

// TestAgentThunderReconciler_RunCycle_AttemptsRunConcurrently guards against a
// slow or unreachable environment starving the rest of a batch: if attempts
// ran sequentially, N bindings each blocking on the barrier below would
// deadlock (each waits for all N to have started, but the (N+1)th can't start
// until an earlier one returns). Only passes if every AttemptProvision call
// starts before any of them returns.
func TestAgentThunderReconciler_RunCycle_AttemptsRunConcurrently(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "reconciler-concurrency-agent"
	t.Cleanup(func() { _ = repo.DeleteByAgent(ctx, org, project, agent) })

	const n = 5
	envs := []string{"env1", "env2", "env3", "env4", "env5"}
	for _, env := range envs {
		require.NoError(t, repo.Upsert(ctx, &models.AgentThunderClient{
			OUID: org, ProjectName: project, AgentName: agent, EnvironmentName: env,
			ProvisioningType: models.AgentProvisioningTypeExternal, Status: models.AgentThunderStatusPending,
		}))
	}

	// Other due bindings may exist in the shared test DB from unrelated tests
	// and would also call attemptFunc — only wait for OUR n known bindings to
	// arrive, not an exact total, so a stray extra can't deadlock this barrier.
	var mine atomic.Int32
	provisioning := &fakeProvisioningService{
		attemptFunc: func(_ context.Context, b models.AgentThunderClient) {
			if b.OUID == org && b.ProjectName == project && b.AgentName == agent {
				mine.Add(1)
			}
			for mine.Load() < n {
				time.Sleep(time.Millisecond)
			}
		},
	}
	s := &agentThunderReconcilerService{provisioning: provisioning, repo: repo, logger: slog.Default(), stopCh: make(chan struct{})}

	done := make(chan struct{})
	go func() {
		s.runCycle(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runCycle did not complete — attempts are not running concurrently")
	}
}

// TestAgentThunderReconciler_RunCycle_ConcurrencyIsCapped verifies that reconciler concurrency
// is strictly limited to reconcilerConcurrencyLimit (10) even when a large number of
// bindings (e.g. 15) are due to be reconciled.
func TestAgentThunderReconciler_RunCycle_ConcurrencyIsCapped(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "reconciler-cap-concurrency-agent"
	t.Cleanup(func() { _ = repo.DeleteByAgent(ctx, org, project, agent) })

	const totalBindings = 15
	for i := range totalBindings {
		env := fmt.Sprintf("env-%d", i)
		require.NoError(t, repo.Upsert(ctx, &models.AgentThunderClient{
			OUID: org, ProjectName: project, AgentName: agent, EnvironmentName: env,
			ProvisioningType: models.AgentProvisioningTypeExternal, Status: models.AgentThunderStatusPending,
		}))
	}

	var active int32
	var maxObservedActive int32
	provisioning := &fakeProvisioningService{
		attemptFunc: func(_ context.Context, b models.AgentThunderClient) {
			if b.OUID == org && b.ProjectName == project && b.AgentName == agent {
				n := atomic.AddInt32(&active, 1)
				for {
					max := atomic.LoadInt32(&maxObservedActive)
					if n <= max || atomic.CompareAndSwapInt32(&maxObservedActive, max, n) {
						break
					}
				}
				time.Sleep(50 * time.Millisecond)
				atomic.AddInt32(&active, -1)
			}
		},
	}
	s := &agentThunderReconcilerService{provisioning: provisioning, repo: repo, logger: slog.Default(), stopCh: make(chan struct{})}

	s.runCycle(ctx)

	assert.EqualValues(t, reconcilerConcurrencyLimit, atomic.LoadInt32(&maxObservedActive),
		"reconciler concurrency must be capped exactly to the reconcilerConcurrencyLimit")
}

// TestAgentThunderReconciler_RunIdentityInjectionReconcile_PagesBeyondSingleBatch
// guards the keyset pagination between pages: FindRecentlyCompletedInternal
// orders by (created_at, id) with a fixed LIMIT, so without pagination a
// single call would only ever see the oldest reconcilerBatchSize eligible
// bindings, re-selecting that same page every tick and starving any binding
// beyond it until enough of the page aged out of the 2-hour window. Seeds
// more than reconcilerBatchSize eligible bindings and asserts every single
// one is reconciled by one call — proving the paging loop keeps fetching
// pages instead of stopping after the first.
func TestAgentThunderReconciler_RunIdentityInjectionReconcile_PagesBeyondSingleBatch(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent = "test-org", "test-proj", "reconcile-page-agent"
	t.Cleanup(func() { _ = repo.DeleteByAgent(ctx, org, project, agent) })

	const total = reconcilerBatchSize + 20
	now := time.Now()
	envNames := make([]string, total)
	for i := range total {
		env := fmt.Sprintf("env-%d", i)
		envNames[i] = env
		require.NoError(t, repo.Upsert(ctx, &models.AgentThunderClient{
			OUID: org, ProjectName: project, AgentName: agent, EnvironmentName: env,
			ProvisioningType: models.AgentProvisioningTypeInternal, Status: models.AgentThunderStatusCompleted,
			ThunderAgentID: "thunder-" + env, ThunderClientID: "client-" + env,
			SecretRefPath: "path/" + env,
			CreatedAt:     now.Add(time.Duration(i) * time.Millisecond),
		}))
	}

	var mu sync.Mutex
	reconciled := map[string]bool{}
	injector := &agentIdentityInjectorStub{
		ReconcileForEnvironmentFunc: func(_ context.Context, ouID, projectName, agentName, envName string) error {
			if ouID != org || projectName != project || agentName != agent {
				return nil // a stray binding from an unrelated concurrent test
			}
			mu.Lock()
			defer mu.Unlock()
			reconciled[envName] = true
			return nil
		},
	}
	s := &agentThunderReconcilerService{injector: injector, repo: repo, logger: slog.Default(), stopCh: make(chan struct{})}

	s.runIdentityInjectionReconcile(ctx)

	mu.Lock()
	defer mu.Unlock()
	for _, env := range envNames {
		assert.True(t, reconciled[env], "environment %q must be reconciled even though more than reconcilerBatchSize eligible bindings existed", env)
	}
}

// TestAgentThunderReconciler_RunInitialIdentityInjectionBackfill_CoversBindingsThePeriodicSweepMisses
// guards the case the periodic sweep's window can't reach: a binding
// completed long enough ago is invisible to it forever — verified here by
// first showing the periodic sweep really does skip it, then that the
// one-time backfill (no window at all) reconciles it.
func TestAgentThunderReconciler_RunInitialIdentityInjectionBackfill_CoversBindingsThePeriodicSweepMisses(t *testing.T) {
	ctx := context.Background()
	repo := repositories.NewAgentThunderClientRepo(db.GetDB())
	const org, project, agent, env = "test-org", "test-proj", "reconcile-backfill-agent", "production"
	t.Cleanup(func() { _ = repo.DeleteByAgent(ctx, org, project, agent) })

	require.NoError(t, repo.Upsert(ctx, &models.AgentThunderClient{
		OUID: org, ProjectName: project, AgentName: agent, EnvironmentName: env,
		ProvisioningType: models.AgentProvisioningTypeInternal, Status: models.AgentThunderStatusCompleted,
		ThunderAgentID: "thunder-old", ThunderClientID: "client-old", SecretRefPath: "path/old",
		CreatedAt: time.Now().Add(-(identityInjectionReconcileWindow + time.Hour)),
	}))

	var mu sync.Mutex
	var reconciled bool
	injector := &agentIdentityInjectorStub{
		ReconcileForEnvironmentFunc: func(_ context.Context, ouID, projectName, agentName, envName string) error {
			if ouID != org || projectName != project || agentName != agent {
				return nil // a stray binding from an unrelated concurrent test
			}
			mu.Lock()
			defer mu.Unlock()
			reconciled = true
			return nil
		},
	}
	s := &agentThunderReconcilerService{injector: injector, provisioning: &fakeProvisioningService{}, repo: repo, logger: slog.Default(), stopCh: make(chan struct{})}

	s.runIdentityInjectionReconcile(ctx)
	mu.Lock()
	assert.False(t, reconciled, "the periodic window-bounded sweep must not reach a binding older than identityInjectionReconcileWindow")
	mu.Unlock()

	s.runInitialIdentityInjectionBackfill(ctx)
	mu.Lock()
	defer mu.Unlock()
	assert.True(t, reconciled, "the one-time backfill must reconcile a binding regardless of age")
}

// fakeProvisioningService is a minimal hand-written test double for
// AgentThunderProvisioningService.
type fakeProvisioningService struct {
	attemptFunc func(ctx context.Context, binding models.AgentThunderClient)
}

func (f *fakeProvisioningService) ProvisionForAgent(context.Context, string, string, string, models.AgentProvisioningType, []string, string) {
}

func (f *fakeProvisioningService) ProvisionForEnvironmentIfMissing(context.Context, string, string, string, string, models.AgentProvisioningType, string) (bool, error) {
	return false, nil
}

func (f *fakeProvisioningService) AttemptProvision(ctx context.Context, binding models.AgentThunderClient) {
	f.attemptFunc(ctx, binding)
}

func (f *fakeProvisioningService) GetCredentials(context.Context, string, string, string, string) (string, string, string, error) {
	return "", "", "", nil
}

func (f *fakeProvisioningService) RegenerateSecret(context.Context, string, string, string, string) (models.AgentProvisioningType, string, string, error) {
	return "", "", "", nil
}

func (f *fakeProvisioningService) RetryProvisioning(context.Context, string, string, string, string) (models.AgentIdentityEnvironmentView, error) {
	return models.AgentIdentityEnvironmentView{}, nil
}

func (f *fakeProvisioningService) RevokeSecret(context.Context, string, string, string, string) (string, error) {
	return "", nil
}
func (f *fakeProvisioningService) DeleteAllBindings(context.Context, string, string, string) {}

func (f *fakeProvisioningService) GetIdentityViews(context.Context, string, string, string) ([]models.AgentIdentityEnvironmentView, error) {
	return nil, nil
}

func (f *fakeProvisioningService) GetBindingState(context.Context, string, string, string, string) (*AgentThunderBindingState, error) {
	return nil, nil
}

func (f *fakeProvisioningService) ClaimSecret(context.Context, string, string, string, string) (string, string, string, error) {
	return "", "", "", nil
}

func (f *fakeProvisioningService) GetAgentRoles(context.Context, string, string, string, string) ([]thundersvc.ThunderRole, error) {
	return nil, nil
}

func (f *fakeProvisioningService) GetAgentGroups(context.Context, string, string, string, string) ([]thundersvc.ThunderGroup, error) {
	return nil, nil
}

// compile-time interface check
var _ AgentThunderProvisioningService = (*fakeProvisioningService)(nil)
