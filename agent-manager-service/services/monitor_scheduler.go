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

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	schedulerTickInterval = 1 * time.Minute
	schedulerLockID       = int64(739281456) // PostgreSQL advisory lock ID for scheduler

	// runStuckTimeout bounds how long a run may stay un-terminal before the scheduler
	// gives up on it. A run whose pod can never start — a secret it mounts that never
	// syncs, a quota it never gets — reports Pending forever, and ListPendingOrRunningRuns
	// is capped, so enough of them starve real runs out of status sync entirely.
	// Longer than the workflow's own activeDeadlineSeconds so Argo gets the first chance
	// to fail it and report a real status.
	runStuckTimeout = 45 * time.Minute

	// triggerFailureBackoffCap bounds the retry delay after a failed trigger. Without a
	// delay a permanently broken monitor is retried every tick, and each attempt costs
	// a round of Thunder and OpenChoreo calls.
	triggerFailureBackoffCap = 15 * time.Minute
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
	runObserver MonitorRunObserver
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
	runObserver MonitorRunObserver,
) MonitorSchedulerService {
	return &monitorSchedulerService{
		ocClient:    ocClient,
		provisioner: provisioner,
		logger:      logger,
		executor:    executor,
		monitorRepo: monitorRepo,
		runObserver: runObserver,
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
	tx := db.GetDB().WithContext(ctx).Begin()
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

	// Carry the org the way a request would. The OpenChoreo client reads the resolved
	// org from the context to stamp its impersonation header, and only the HTTP
	// middleware ever sets it — so a scheduled run reaches OpenChoreo with no org at
	// all and endpoints that resolve one from the caller reject the request. The
	// scheduler knows which org it is acting for, so it says so.
	ctx = orgctx.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: monitor.OUID})

	// Get an org-bound OC client in Thunder mode; nil in non-Thunder mode (executor falls back).
	orgOCClient, err := s.orgOCClient(ctx, monitor.OUID)
	if err != nil {
		s.backOff(ctx, monitor, interval)
		return fmt.Errorf("failed to get OC client for org %s: %w", monitor.OUID, err)
	}

	result, err := s.executor.ExecuteMonitorRun(withOCClient(ctx, orgOCClient), ExecuteMonitorRunParams{
		OUID:       monitor.OUID,
		Monitor:    monitor,
		StartTime:  startTime,
		EndTime:    endTime,
		Evaluators: monitor.Evaluators,
	})
	if err != nil {
		// A scheduled run that never started. Recorded with a system actor
		// because no user asked for this one — successful runs are not
		// recorded, since their scores are the record.
		audit.Record(
			ctx, audit.ActionMonitorRunFail,
			audit.Org(monitor.OUID),
			audit.ResourceNamed("monitor", monitor.ID.String(), monitor.Name),
			audit.Actor(audit.ActorSystem, systemActorMonitorScheduler, ""),
			audit.SurfaceOpt(audit.SurfaceSystem),
			audit.Detail("monitorName", monitor.Name),
			audit.Detail("reason", "execute-failed"),
			audit.Result(err),
		)
		s.logger.Error("Failed to execute monitor run", "error", err)
		s.runObserver.OnMonitorTriggerFailed(ctx, monitor, err)
		s.backOff(ctx, monitor, interval)
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

// backOff pushes a failed monitor's next_run_time forward so the next cycle does not
// retry it immediately. Leaving next_run_time in the past makes every tick re-attempt
// a monitor that is broken for a reason a minute will not fix, and each attempt costs
// a round of Thunder and OpenChoreo calls. The delay is the monitor's own interval,
// which is the cadence it asked for, capped so a daily monitor is not stalled a day by
// one transient error. Failing to record the backoff is logged, not returned: the
// caller is already reporting the trigger failure, and the next tick retries either way.
func (s *monitorSchedulerService) backOff(ctx context.Context, monitor *models.Monitor, interval time.Duration) {
	delay := min(interval, triggerFailureBackoffCap)
	if delay < schedulerTickInterval {
		delay = schedulerTickInterval
	}

	retryAt := time.Now().Add(delay)
	if err := s.executor.UpdateNextRunTime(ctx, monitor.ID, retryAt); err != nil {
		s.logger.Error("Failed to back off failed monitor",
			"monitor", monitor.Name, "error", err)
		return
	}

	s.logger.Info("Backed off failed monitor", "monitor", monitor.Name, "retryAt", retryAt)
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

	// Same reason as triggerMonitor: the OpenChoreo call below needs the org this run
	// belongs to, and a background context has none unless we put it there.
	ctx = orgctx.WithResolvedOrg(ctx, orgctx.ResolvedOrg{OUID: monitor.OUID})

	ocClient, err := s.orgOCClient(ctx, monitor.OUID)
	if err != nil {
		return fmt.Errorf("failed to get OC client for org %s: %w", monitor.OUID, err)
	}

	workflowRun, err := ocClient.GetWorkflowRun(ctx, monitor.OUID, run.Name)
	if err != nil {
		// Only a confirmed absence means the run will never reach a terminal status on
		// its own. Transport, authorization and server errors say nothing about the
		// workflow, and treating them the same would mark every long-pending run failed
		// for the duration of an OpenChoreo outage — including ones that then succeed.
		if errors.Is(err, utils.ErrNotFound) {
			s.logger.Warn("WorkflowRun not found", "workflowRunName", run.Name)
			failed, failErr := s.failStaleRun(ctx, monitor, run, "workflow run no longer exists and has exceeded the run timeout")
			if failErr != nil {
				return failErr
			}
			if failed {
				return nil
			}
		}
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
		failed, failErr := s.failStaleRun(ctx, monitor, run, "workflow exceeded the run timeout while running")
		if failErr != nil {
			return failErr
		}
		if failed {
			return nil
		}
		if run.Status != models.RunStatusRunning {
			updates["status"] = models.RunStatusRunning
		}

	case "Pending":
		// A workflow whose pod cannot start — most often a secret it mounts that never
		// syncs — sits here indefinitely, and enough of them fill the capped pending
		// query and starve real runs out of status sync.
		_, failErr := s.failStaleRun(ctx, monitor, run, "workflow never left Pending within the run timeout")
		return failErr

	default:
		s.logger.Warn("Unknown workflow status", "status", workflowRun.Status, "workflowRunName", run.Name)
		_, failErr := s.failStaleRun(ctx, monitor, run, "workflow reported no terminal status within the run timeout")
		return failErr
	}

	if len(updates) > 0 {
		if err := s.monitorRepo.UpdateMonitorRun(run, updates); err != nil {
			return fmt.Errorf("failed to update run status: %w", err)
		}
		s.logger.Info("Updated run status", "runID", run.ID, "status", updates["status"])

		// Report once the new status is stored: a failed write means the run
		// is synced again next tick, and event IDs are per run, so a re-sync
		// cannot publish a duplicate.
		if status, ok := updates["status"].(string); ok && (status == models.RunStatusSuccess || status == models.RunStatusFailed) {
			errMsg, _ := updates["error_message"].(string)
			s.runObserver.OnMonitorRunFinished(ctx, monitor, run, status, errMsg)
		}
	}

	return nil
}

// failStaleRun marks a run failed when it has been un-terminal for longer than
// runStuckTimeout, and reports whether it did. Runs with no StartedAt have no age to
// judge and are left alone, reported as (false, nil). A persistence failure is returned
// rather than folded into false, so callers can tell "this run was fine" apart from
// "the write did not land".
func (s *monitorSchedulerService) failStaleRun(ctx context.Context, monitor *models.Monitor, run *models.MonitorRun, reason string) (bool, error) {
	if run.StartedAt == nil || time.Since(*run.StartedAt) <= runStuckTimeout {
		return false, nil
	}

	updates := map[string]interface{}{
		"status":        models.RunStatusFailed,
		"completed_at":  time.Now(),
		"error_message": reason,
	}
	if err := s.monitorRepo.UpdateMonitorRun(run, updates); err != nil {
		return false, fmt.Errorf("failed to mark stale run %s as failed: %w", run.Name, err)
	}

	s.logger.Warn("Marked stale monitor run as failed",
		"runID", run.ID, "runName", run.Name, "startedAt", run.StartedAt, "reason", reason)
	s.runObserver.OnMonitorRunFinished(ctx, monitor, run, models.RunStatusFailed, reason)
	return true, nil
}

// orgOCClient returns a per-org OC client in Thunder mode, or the system client in non-Thunder mode.
func (s *monitorSchedulerService) orgOCClient(ctx context.Context, ouID string) (client.OpenChoreoClient, error) {
	if !s.provisioner.IsThunderMode() {
		return s.ocClient, nil
	}
	return s.provisioner.GetOCClientForOrg(ctx, ouID)
}

// systemActorMonitorScheduler identifies the monitor scheduler in audit records
// it produces on its own initiative.
const systemActorMonitorScheduler = "system:monitor-scheduler"
