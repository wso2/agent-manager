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
	promotionTopologyInterval  = 5 * time.Minute
	// promotionReconcilerLockID serializes the project promotion topology pass
	// across replicas; distinct from reconcilerLockID and schedulerLockID.
	promotionReconcilerLockID = int64(739281458)
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
	repo         repositories.AgentThunderClientRepository
	logger       *slog.Logger
	stopCh       chan struct{}
	stopOnce     sync.Once
}

// NewAgentThunderReconcilerService creates a new AgentThunderReconcilerService.
func NewAgentThunderReconcilerService(
	provisioning AgentThunderProvisioningService,
	repo repositories.AgentThunderClientRepository,
	logger *slog.Logger,
) AgentThunderReconcilerService {
	return &agentThunderReconcilerService{
		provisioning: provisioning,
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
	go s.runLoop(ctx)
	if _, ok := s.provisioning.(ProjectAgentIdentityPromotionReconciler); ok {
		go s.runPromotionTopologyLoop(ctx)
	}
	s.logger.Info("Agent thunder provisioning reconciler started")
	return nil
}

func (s *agentThunderReconcilerService) runPromotionTopologyLoop(ctx context.Context) {
	reconciler := s.provisioning.(ProjectAgentIdentityPromotionReconciler)
	s.runPromotionTopologyCycle(ctx, reconciler)
	ticker := time.NewTicker(promotionTopologyInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.runPromotionTopologyCycle(ctx, reconciler)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

// runPromotionTopologyCycle runs one topology pass under a PostgreSQL advisory
// lock so only one replica backfills promotion bindings at a time, the same
// mechanism runCycle uses. The pass is idempotent, so the lock only avoids
// redundant pipeline reads: a replica that does not get it just skips the pass.
func (s *agentThunderReconcilerService) runPromotionTopologyCycle(
	ctx context.Context, reconciler ProjectAgentIdentityPromotionReconciler,
) {
	tx := db.GetDB().WithContext(ctx).Begin()
	if tx.Error != nil {
		s.logger.Error("Failed to begin transaction for promotion topology advisory lock", "error", tx.Error)
		return
	}
	// The transaction-scoped lock is released by the rollback; the pass itself
	// uses its own connections and writes nothing through tx.
	defer tx.Rollback()

	var locked bool
	if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", promotionReconcilerLockID).Scan(&locked).Error; err != nil {
		s.logger.Error("Failed to try promotion topology advisory lock", "error", err)
		return
	}
	if !locked {
		s.logger.Debug("Another instance is reconciling project AgentID promotions, skipping pass")
		return
	}
	reconciler.ReconcileProjectAgentIdentityPromotions(ctx)
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
}
