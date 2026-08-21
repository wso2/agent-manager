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
	"log/slog"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const (
	reconcilerTickInterval = 1 * time.Minute
	// reconcilerLockID is a distinct PostgreSQL advisory lock ID from
	// schedulerLockID (monitor_scheduler.go) — each background loop in this
	// service needs its own ID so they don't block each other.
	reconcilerLockID           = int64(739281457)
	reconcilerBatchSize        = 50
	reconcilerConcurrencyLimit = 10
)

// AgentThunderReconcilerService sweeps PENDING agent Thunder bindings whose
// retry time has arrived and retries them, up to the max-attempts budget
// (Section 6 of the AgentID architecture doc).
type AgentThunderReconcilerService interface {
	Start(ctx context.Context) error
	Stop() error
}

type agentThunderReconcilerService struct {
	provisioning AgentThunderProvisioningService
	// injector pushes an internal agent's credential into its live workload —
	// held directly (not reached via the provisioning service) since this
	// reconciler's own periodic sweep is the only caller within this service;
	// see reconcileWorkloadInjection's doc comment for why that helper is
	// package-level rather than a method needing to live on
	// AgentThunderProvisioningService's interface.
	injector AgentIdentityInjectionService
	repo     repositories.AgentThunderClientRepository
	logger   *slog.Logger
	stopCh   chan struct{}
	stopOnce sync.Once
}

// NewAgentThunderReconcilerService creates a new AgentThunderReconcilerService.
func NewAgentThunderReconcilerService(
	provisioning AgentThunderProvisioningService,
	injector AgentIdentityInjectionService,
	repo repositories.AgentThunderClientRepository,
	logger *slog.Logger,
) AgentThunderReconcilerService {
	return &agentThunderReconcilerService{
		provisioning: provisioning,
		injector:     injector,
		repo:         repo,
		logger:       logger,
		stopCh:       make(chan struct{}),
	}
}

func (s *agentThunderReconcilerService) Start(ctx context.Context) error {
	s.logger.Info("Initializing agent thunder provisioning reconciler")
	// Own goroutine, independent of runLoop's ticker: a large backfill (see its
	// own doc comment) must never delay the time-sensitive PENDING-binding
	// retry cycle below.
	go s.runInitialIdentityInjectionBackfill(ctx)
	go s.runLoop(ctx)
	s.logger.Info("Agent thunder provisioning reconciler started")
	return nil
}

func (s *agentThunderReconcilerService) Stop() error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.logger.Info("Agent thunder provisioning reconciler stopped")
	})
	return nil
}

func (s *agentThunderReconcilerService) runLoop(ctx context.Context) {
	ticker := time.NewTicker(reconcilerTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.runCycle(ctx)
		case <-s.stopCh:
			s.logger.Info("Agent thunder reconciler loop stopped")
			return
		case <-ctx.Done():
			s.logger.Info("Agent thunder reconciler loop context cancelled")
			return
		}
	}
}

// runCycle sweeps due bindings under a PostgreSQL advisory lock so only one
// replica of this service retries a given binding at a time — mirrors
// monitor_scheduler.go's runSchedulerCycle exactly.
func (s *agentThunderReconcilerService) runCycle(ctx context.Context) {
	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		s.logger.Error("Failed to begin transaction for agent thunder reconciler advisory lock", "error", tx.Error)
		return
	}

	var locked bool
	if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", reconcilerLockID).Scan(&locked).Error; err != nil {
		s.logger.Error("Failed to try agent thunder reconciler advisory lock", "error", err)
		tx.Rollback()
		return
	}
	if !locked {
		s.logger.Debug("Another instance is running the agent thunder reconciler, skipping cycle")
		tx.Rollback()
		return
	}

	due, err := s.repo.FindDue(ctx, time.Now(), reconcilerBatchSize)
	if err != nil {
		s.logger.Error("Failed to query due agent thunder bindings", "error", err)
		tx.Rollback()
		return
	}

	// Release the lock/connection now — ClaimForAttempt already guards each
	// binding individually, so the lock only needs to cover the FindDue scan,
	// not the slow Thunder/OpenBao calls below.
	if err := tx.Commit().Error; err != nil {
		s.logger.Error("Failed to commit agent thunder reconciler advisory lock transaction", "error", err)
		return
	}

	if len(due) > 0 {
		s.logger.Info("Retrying due agent thunder bindings", "count", len(due))
	}
	// Run attempts concurrently up to reconcilerConcurrencyLimit: ClaimForAttempt
	// already makes each one safe and independent, so one slow/unreachable
	// environment's bindings (each up to ~8s just in base-URL probing, see naming.go)
	// can't serialize behind the rest of a up-to-reconcilerBatchSize batch and
	// starve everything else's retry schedule. Semaphore limits concurrency to
	// prevent DB connection pool starvation.
	sem := make(chan struct{}, reconcilerConcurrencyLimit)
	var wg sync.WaitGroup
	for _, binding := range due {
		sem <- struct{}{}
		wg.Add(1)
		go func(b models.AgentThunderClient) {
			defer wg.Done()
			defer func() { <-sem }()
			s.provisioning.AttemptProvision(ctx, b)
		}(binding)
	}
	wg.Wait()

	s.runIdentityInjectionReconcile(ctx)
}

// identityInjectionReconcileWindow bounds how far back the PERIODIC injection
// sweep (runIdentityInjectionReconcile, every tick) looks. It only bridges the
// gap between provisioning completing (seconds after create) and a brand-new
// git agent's first build creating its ReleaseBinding (minutes later);
// steady-state sync is owned by the deploy, promote, rotation, and
// MCP-config-change paths, so the window is finite — every-tick-forever over
// the whole table would be needless, permanent load. Bindings completed
// before this window existed (e.g. an AMS upgrade) are still covered exactly
// once by runInitialIdentityInjectionBackfill instead, without paying that
// cost on every tick.
const identityInjectionReconcileWindow = 2 * time.Hour

// runIdentityInjectionReconcile reconciles recently-completed internal agents'
// live workloads with their desired AgentID env vars, writing only when vars
// are missing or scopes drifted (see ReconcileForEnvironment) so re-running
// each tick never causes a needless pod rollout. Runs outside the advisory
// lock; concurrent instances converge on the same desired state, so the worst
// case is a duplicate content-identical write.
func (s *agentThunderReconcilerService) runIdentityInjectionReconcile(ctx context.Context) {
	s.pageIdentityInjectionReconcile(ctx, time.Now().Add(-identityInjectionReconcileWindow), false)
}

// runInitialIdentityInjectionBackfill runs once per process start (see Start)
// and reconciles EVERY completed internal binding regardless of age — not
// just ones within identityInjectionReconcileWindow. This covers a binding
// whose ReleaseBinding predates this reconcile sweep and so has no AgentID
// overrides at all: the next redeploy to the pipeline's LOWEST environment
// writes that environment's client_id/secret into the shared Workload CR
// (inherited by every environment lacking its own override), and an
// untouched pre-existing staging/prod pod would otherwise silently inherit it
// on its next restart — the same cross-environment credential leak
// PromoteAgent's hard block prevents for new promotions, but that block only
// covers promotions from here on, not bindings already sitting in that state.
//
// Not gated on a persisted "already ran" flag: an AMS process restart already
// guarantees the full sweep runs at least once — rather than once ever, which
// would need new schema state — and self-heals again on any later restart.
// ReconcileForEnvironment is cheap and idempotent for anything already in
// sync, so the real cost this design avoids is running the full-table sweep
// on every one-minute tick forever, not just once (or a few times) per
// restart.
func (s *agentThunderReconcilerService) runInitialIdentityInjectionBackfill(ctx context.Context) {
	s.pageIdentityInjectionReconcile(ctx, time.Time{}, true)
}

// pageIdentityInjectionReconcile reconciles every COMPLETED internal binding
// created at/after createdAfter, paging through ALL of them (not just the
// oldest reconcilerBatchSize): FindRecentlyCompletedInternal orders by
// (created_at, id), so re-querying with the previous page's last row as a
// keyset cursor continues strictly past it. Without this, more than
// reconcilerBatchSize eligible bindings existing at once would starve the
// newest ones — the same oldest page would be reselected every call until
// enough of them aged out of the (periodic caller's) window, and a newer
// binding could miss its entire window without ever being reconciled.
//
// healSecretRefs is true only for the unbounded startup pass
// (runInitialIdentityInjectionBackfill): HealSecretRef is cheap (one bounded
// OpenChoreo read) and idempotent (a no-op once a binding's SecretRefPath
// already matches), but there is no reason to re-run it against every
// already-healed row on every one-minute periodic tick too.
func (s *agentThunderReconcilerService) pageIdentityInjectionReconcile(ctx context.Context, createdAfter time.Time, healSecretRefs bool) {
	var cursor *repositories.ReconcileCursor
	for {
		recent, err := s.repo.FindRecentlyCompletedInternal(ctx, createdAfter, cursor, reconcilerBatchSize)
		if err != nil {
			s.logger.Error("Failed to query recently completed internal agent thunder bindings", "error", err)
			return
		}
		for _, binding := range recent {
			if healSecretRefs {
				if err := s.provisioning.HealSecretRef(ctx, binding); err != nil {
					s.logger.Warn("Failed to heal agent thunder binding secret ref", "ou_id", binding.OUID, "binding_id", binding.ID, "agent_name", binding.AgentName, "env_name", binding.EnvironmentName, "error", err)
				}
			}
			reconcileWorkloadInjection(ctx, s.injector, binding, s.logger)
		}
		if len(recent) < reconcilerBatchSize {
			return
		}
		last := recent[len(recent)-1]
		cursor = &repositories.ReconcileCursor{CreatedAt: last.CreatedAt, ID: last.ID}
	}
}
