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
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

const (
	schedulerTickInterval = 1 * time.Minute
	schedulerLockID       = int64(739281456) // PostgreSQL advisory lock ID for scheduler
)

// MonitorSchedulerService handles scheduled monitor execution
type MonitorSchedulerService interface {
	Start(ctx context.Context) error
	Stop() error
}

type monitorSchedulerService struct {
	// ocClient is the system-level OC client used as fallback in non-Thunder mode.
	ocClient    client.OpenChoreoClient
	provisioner PublisherCredentialProvisioner
	logger      *slog.Logger
	executor    MonitorExecutor
	monitorRepo repositories.MonitorRepository
	stopCh      chan struct{}
	stopOnce    sync.Once
}

// NewMonitorSchedulerService creates a new monitor scheduler service.
// In Thunder mode the provisioner supplies per-org OC clients (org-bound tokens).
// In non-Thunder mode ocClient is used as the system fallback.
func NewMonitorSchedulerService(
	ocClient client.OpenChoreoClient,
	provisioner PublisherCredentialProvisioner,
	logger *slog.Logger,
	executor MonitorExecutor,
	monitorRepo repositories.MonitorRepository,
) MonitorSchedulerService {
	return &monitorSchedulerService{
		ocClient:    ocClient,
		provisioner: provisioner,
		logger:      logger,
		executor:    executor,
		monitorRepo: monitorRepo,
		stopCh:      make(chan struct{}),
	}
}

// Start begins the scheduler
func (s *monitorSchedulerService) Start(ctx context.Context) error {
	s.logger.Info("Initializing monitor scheduler")
	go s.runSchedulerLoop(ctx)
	s.logger.Info("Monitor scheduler started")
	return nil
}

// Stop stops the scheduler
func (s *monitorSchedulerService) Stop() error {
	s.stopOnce.Do(func() {
		close(s.stopCh)
		s.logger.Info("Monitor scheduler stopped")
	})
	return nil
}

// runSchedulerLoop runs the main scheduler loop
func (s *monitorSchedulerService) runSchedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()

	s.runSchedulerCycle(ctx)

	for {
		select {
		case <-ticker.C:
			s.runSchedulerCycle(ctx)
		case <-s.stopCh:
			s.logger.Info("Scheduler loop stopped")
			return
		case <-ctx.Done():
			s.logger.Info("Scheduler loop context cancelled")
			return
		}
	}
}

// runSchedulerCycle executes one cycle of the scheduler
func (s *monitorSchedulerService) runSchedulerCycle(ctx context.Context) {
	tx := db.DB(ctx).Begin()
	if tx.Error != nil {
		s.logger.Error("Failed to begin transaction for advisory lock", "error", tx.Error)
		return
	}
	defer tx.Rollback()

	var locked bool
	if err := tx.Raw("SELECT pg_try_advisory_xact_lock(?)", schedulerLockID).Scan(&locked).Error; err != nil {
		s.logger.Error("Failed to try advisory lock", "error", err)
		return
	}
	if !locked {
		s.logger.Debug("Another instance is running scheduler, skipping cycle")
		return
	}

	s.logger.Debug("Running scheduler cycle")

	if err := s.triggerPendingMonitors(ctx); err != nil {
		s.logger.Error("Failed to trigger pending monitors", "error", err)
	}

	if err := s.syncRunStatus(ctx); err != nil {
		s.logger.Error("Failed to sync run status", "error", err)
	}

	if err := tx.Commit().Error; err != nil {
		s.logger.Error("Failed to commit scheduler advisory lock transaction", "error", err)
	}
}

// triggerPendingMonitors checks for monitors that need to run and creates WorkflowRun CRs
func (s *monitorSchedulerService) triggerPendingMonitors(ctx context.Context) error {
	monitors, err := s.monitorRepo.ListDueMonitors(models.MonitorTypeFuture, time.Now())
	if err != nil {
		return fmt.Errorf("failed to query pending monitors: %w", err)
	}

	if len(monitors) == 0 {
		return nil
	}

	s.logger.Info("Found monitors to trigger", "count", len(monitors))

	for _, monitor := range monitors {
		if err := s.triggerMonitor(ctx, &monitor); err != nil {
			s.logger.Error("Failed to trigger monitor", "monitor", monitor.Name, "error", err)
		}
	}

	return nil
}

// triggerMonitor creates a WorkflowRun CR for a single monitor
func (s *monitorSchedulerService) triggerMonitor(ctx context.Context, monitor *models.Monitor) error {
	if monitor.IntervalMinutes == nil {
		return fmt.Errorf("interval_minutes is nil for monitor %s", monitor.Name)
	}
	if monitor.NextRunTime == nil {
		return fmt.Errorf("next_run_time is nil for monitor %s", monitor.Name)
	}

	safetyDelta := time.Duration(float64(*monitor.IntervalMinutes)*models.SafetyDeltaPercent) * time.Minute
	interval := time.Duration(*monitor.IntervalMinutes) * time.Minute
	startTime := monitor.NextRunTime.Add(-interval)
	endTime := time.Now().Add(-safetyDelta)
	nextRunTime := endTime.Add(interval)

	// Get an org-bound OC client in Thunder mode; nil in non-Thunder mode (executor falls back).
	orgOCClient, err := s.orgOCClient(ctx, monitor.OrgName)
	if err != nil {
		return fmt.Errorf("failed to get OC client for org %s: %w", monitor.OrgName, err)
	}

	result, err := s.executor.ExecuteMonitorRun(withOCClient(ctx, orgOCClient), ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  startTime,
		EndTime:    endTime,
		Evaluators: monitor.Evaluators,
	})
	if err != nil {
		s.logger.Error("Failed to execute monitor run", "error", err)
		return err
	}

	if err := s.executor.UpdateNextRunTime(ctx, monitor.ID, nextRunTime); err != nil {
		return fmt.Errorf("failed to update next_run_time for monitor %s: %w", monitor.Name, err)
	}

	s.logger.Info("Monitor triggered successfully",
		"monitor", monitor.Name,
		"workflowRunName", result.Name,
		"nextScheduledRun", nextRunTime)

	return nil
}

// syncRunStatus queries OpenChoreo API for pending/running workflows and updates DB
func (s *monitorSchedulerService) syncRunStatus(ctx context.Context) error {
	runs, err := s.monitorRepo.ListPendingOrRunningRuns(100)
	if err != nil {
		return fmt.Errorf("failed to query pending/running runs: %w", err)
	}

	if len(runs) == 0 {
		return nil
	}

	s.logger.Debug("Syncing run status", "count", len(runs))

	for _, run := range runs {
		if err := s.syncSingleRunStatus(ctx, &run); err != nil {
			s.logger.Error("Failed to sync run status", "runID", run.ID, "error", err)
		}
	}

	return nil
}

// syncSingleRunStatus queries OpenChoreo API for a single run and updates DB
func (s *monitorSchedulerService) syncSingleRunStatus(ctx context.Context, run *models.MonitorRun) error {
	monitor, err := s.monitorRepo.GetMonitorByID(run.MonitorID)
	if err != nil {
		return fmt.Errorf("failed to get monitor: %w", err)
	}
	if monitor == nil {
		s.logger.Warn("Monitor not found for run, skipping status sync", "monitorID", run.MonitorID)
		return nil
	}

	ocClient, err := s.orgOCClient(ctx, monitor.OrgName)
	if err != nil {
		return fmt.Errorf("failed to get OC client for org %s: %w", monitor.OrgName, err)
	}

	workflowRun, err := ocClient.GetWorkflowRun(ctx, monitor.OrgName, run.Name)
	if err != nil {
		s.logger.Warn("WorkflowRun not found", "workflowRunName", run.Name)
		return fmt.Errorf("failed to get workflow run: %w", err)
	}

	s.logger.Debug("WorkflowRun status retrieved",
		"runName", run.Name,
		"currentDBStatus", run.Status,
		"workflowStatus", workflowRun.Status)

	updates := make(map[string]interface{})

	switch workflowRun.Status {
	case "Succeeded":
		updates["status"] = models.RunStatusSuccess
		updates["completed_at"] = time.Now()

	case "Failed", "Error":
		updates["status"] = models.RunStatusFailed
		updates["completed_at"] = time.Now()
		updates["error_message"] = "workflow completed with failure"

	case "Running":
		if run.Status != models.RunStatusRunning {
			updates["status"] = models.RunStatusRunning
		}

	case "Pending":
		return nil

	default:
		s.logger.Warn("Unknown workflow status", "status", workflowRun.Status, "workflowRunName", run.Name)
		return nil
	}

	if len(updates) > 0 {
		if err := s.monitorRepo.UpdateMonitorRun(run, updates); err != nil {
			return fmt.Errorf("failed to update run status: %w", err)
		}
		s.logger.Info("Updated run status", "runID", run.ID, "status", updates["status"])
	}

	return nil
}

// orgOCClient returns a per-org OC client in Thunder mode, or the system client in non-Thunder mode.
func (s *monitorSchedulerService) orgOCClient(ctx context.Context, orgName string) (client.OpenChoreoClient, error) {
	if !s.provisioner.IsThunderMode() {
		return s.ocClient, nil
	}
	orgClient, err := s.provisioner.GetOCClientForOrg(ctx, orgName)
	if errors.Is(err, ErrNotThunderMode) {
		return s.ocClient, nil
	}
	if err != nil {
		return nil, err
	}
	return orgClient, nil
}
