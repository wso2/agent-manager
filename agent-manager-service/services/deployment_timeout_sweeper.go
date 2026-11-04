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

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// DeploymentTimeoutSweeper fails gateway deploys and undeploys the gateway never acked. It is the only backstop for
// an unacked event: events are written with their status rows, so a timeout means the gateway didn't answer. Each
// sweep is one guarded UPDATE, so every replica can run one.
type DeploymentTimeoutSweeper interface {
	Start(ctx context.Context) error
	Stop() error
	// RunOnce sweeps once.
	RunOnce(ctx context.Context)
}

type deploymentTimeoutSweeper struct {
	deploymentRepo repositories.DeploymentRepository
	logger         *slog.Logger
	interval       time.Duration
	timeout        time.Duration

	stopCh   chan struct{}
	stopOnce sync.Once
	done     sync.WaitGroup
}

// NewDeploymentTimeoutSweeper sweeps every interval for transitions older than timeout.
func NewDeploymentTimeoutSweeper(
	deploymentRepo repositories.DeploymentRepository, logger *slog.Logger, interval, timeout time.Duration,
) DeploymentTimeoutSweeper {
	return &deploymentTimeoutSweeper{
		deploymentRepo: deploymentRepo,
		logger:         logger,
		interval:       interval,
		timeout:        timeout,
		stopCh:         make(chan struct{}),
	}
}

// NewConfiguredDeploymentTimeoutSweeper reads the interval and timeout from cfg.
func NewConfiguredDeploymentTimeoutSweeper(
	deploymentRepo repositories.DeploymentRepository, logger *slog.Logger, cfg *config.Config,
) DeploymentTimeoutSweeper {
	return NewDeploymentTimeoutSweeper(deploymentRepo, logger,
		time.Duration(cfg.DeploymentTimeout.SweepIntervalSeconds)*time.Second,
		time.Duration(cfg.DeploymentTimeout.TimeoutSeconds)*time.Second)
}

func (s *deploymentTimeoutSweeper) Start(ctx context.Context) error {
	s.done.Add(1)
	go s.runLoop(ctx)
	s.logger.Info("Deployment timeout sweeper started", "interval", s.interval, "timeout", s.timeout)
	return nil
}

// Stop ends the loop and waits for a sweep in progress.
func (s *deploymentTimeoutSweeper) Stop() error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.done.Wait()
		s.logger.Info("Deployment timeout sweeper stopped")
	})
	return nil
}

func (s *deploymentTimeoutSweeper) runLoop(ctx context.Context) {
	defer s.done.Done()
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.RunOnce(ctx)
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		}
	}
}

func (s *deploymentTimeoutSweeper) RunOnce(ctx context.Context) {
	failed, err := s.deploymentRepo.FailStaleTransitional(ctx, time.Now().Add(-s.timeout))
	if err != nil {
		s.logger.Error("Failed to time out unacked gateway deployments", "error", err)
		return
	}
	if failed > 0 {
		s.logger.Warn("Gateway deployments timed out waiting for an ack",
			"count", failed, "reason", repositories.DeploymentTimeoutReason, "timeout", s.timeout)
	}
}
