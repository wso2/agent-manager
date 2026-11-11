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

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// reconcilerOwner names this process as the reconciler lease's owner.
var reconcilerOwner = uuid.NewString()

const (
	reconcilerTickInterval = 1 * time.Minute
	// reconcilerLock is the seeded singleton lease that lets one replica scan for due bindings.
	reconcilerLock             = "agent_identity.reconciler"
	reconcilerBatchSize        = 50
	reconcilerConcurrencyLimit = 10
	promotionTopologyInterval  = 5 * time.Minute
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

// runPromotionTopologyCycle does nothing: Lite environments have no promotion pipeline to backfill bindings for.
func (s *agentThunderReconcilerService) runPromotionTopologyCycle(context.Context, ProjectAgentIdentityPromotionReconciler) {
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

// runCycle sweeps due bindings under a singleton lease so only one replica of this service retries a given binding
// at a time. The lease only covers the FindDue scan: ClaimForAttempt guards each binding, so the slow Thunder calls
// below run unlocked.
func (s *agentThunderReconcilerService) runCycle(ctx context.Context) {
	conn := db.GetDB()
	locked, err := dbx.TryLock(ctx, conn, reconcilerLock, reconcilerOwner, dbx.Now(), reconcilerTickInterval)
	if err != nil {
		s.logger.Error("Failed to take the agent thunder reconciler lease", "error", err)
		return
	}
	if !locked {
		s.logger.Debug("Another instance is running the agent thunder reconciler, skipping cycle")
		return
	}
	due, err := s.repo.FindDue(ctx, time.Now(), reconcilerBatchSize)
	if unlockErr := dbx.Unlock(ctx, conn, reconcilerLock, reconcilerOwner); unlockErr != nil {
		s.logger.Error("Failed to release the agent thunder reconciler lease", "error", unlockErr)
	}
	if err != nil {
		s.logger.Error("Failed to query due agent thunder bindings", "error", err)
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
