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
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
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
	ocClient    client.OpenChoreoClient
	logger      *slog.Logger
	executor    MonitorExecutor
	monitorRepo repositories.MonitorRepository
	stopCh      chan struct{}
	stopOnce    sync.Once
}

// NewMonitorSchedulerService creates a new monitor scheduler service
func NewMonitorSchedulerService(
	ocClient client.OpenChoreoClient,
	logger *slog.Logger,
	executor MonitorExecutor,
	monitorRepo repositories.MonitorRepository,
) MonitorSchedulerService {
	return &monitorSchedulerService{
		ocClient:    ocClient,
		logger:      logger,
		executor:    executor,
		monitorRepo: monitorRepo,
		stopCh:      make(chan struct{}),
	}
}

// Start begins the scheduler
func (s *monitorSchedulerService) Start(ctx context.Context) error {
	s.logger.Info("Initializing monitor scheduler")

	// Run scheduler loop in background
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

// runSchedulerLoop runs the main scheduler loop (only when leader)
func (s *monitorSchedulerService) runSchedulerLoop(ctx context.Context) {
	ticker := time.NewTicker(schedulerTickInterval)
	defer ticker.Stop()

	// Run immediately on start
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
	// Use a transaction to pin the advisory lock to a single connection.
	// pg_try_advisory_xact_lock auto-releases when the transaction ends.
	// Note: Advisory lock is a DB-level coordination mechanism, kept as direct DB access.
	// This will be refactored in the future to use an external scheduler
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

	// Trigger pending monitors
	if err := s.triggerPendingMonitors(ctx); err != nil {
		s.logger.Error("Failed to trigger pending monitors", "error", err)
	}

	// Sync run status
	if err := s.syncRunStatus(ctx); err != nil {
		s.logger.Error("Failed to sync run status", "error", err)
	}

	tx.Commit()
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
			// Continue with next monitor
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

	// Calculate safety delta (5% of interval)
	safetyDelta := time.Duration(float64(*monitor.IntervalMinutes)*models.SafetyDeltaPercent) * time.Minute
	interval := time.Duration(*monitor.IntervalMinutes) * time.Minute

	// Calculate time window for this run - ensures continuous coverage with no gaps
	// Look back from next_run_time by one interval to create overlapping windows
	startTime := monitor.NextRunTime.Add(-interval)
	// End at current time minus safety delta (to avoid missing late-arriving traces)
	endTime := time.Now().Add(-safetyDelta)

	// Next run starts interval minutes after this window ends
	nextRunTime := endTime.Add(interval)

	// Execute the monitor run
	result, err := s.executor.ExecuteMonitorRun(ctx, ExecuteMonitorRunParams{
		OrgName:    monitor.OrgName,
		Monitor:    monitor,
		StartTime:  startTime,
		EndTime:    endTime,
		Evaluators: monitor.Evaluators, // Snapshot at execution time
	})
	if err != nil {
		s.logger.Error("Failed to execute monitor run", "error", err)
		return err
	}

	// Update monitor's next_run_time AFTER successful CR creation
	// If workflow execution fails later, that's tracked in monitor_runs status
	if err := s.executor.UpdateNextRunTime(ctx, monitor.ID, nextRunTime); err != nil {
		s.logger.Error("Failed to update next_run_time", "monitor", monitor.Name, "error", err)
		// Don't fail - the workflow is already running
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
			// Continue with next run
		}
	}

	return nil
}

// syncSingleRunStatus queries OpenChoreo API for a single run and updates DB
func (s *monitorSchedulerService) syncSingleRunStatus(ctx context.Context, run *models.MonitorRun) error {
	// Get monitor to find orgName
	monitor, err := s.monitorRepo.GetMonitorByID(run.MonitorID)
	if err != nil {
		return fmt.Errorf("failed to get monitor: %w", err)
	}

	// Query OpenChoreo API for WorkflowRun status
	cr, err := s.ocClient.GetResource(ctx, monitor.OrgName, "WorkflowRun", run.Name)
	if err != nil {
		// If CR not found, it might have been deleted - mark as error
		s.logger.Warn("WorkflowRun CR not found", "workflowRunName", run.Name)
		return fmt.Errorf("failed to get workflow run: %w", err)
	}

	// Extract status from CR
	status, err := s.extractWorkflowStatus(cr)
	if err != nil {
		return fmt.Errorf("failed to extract workflow status: %w", err)
	}

	s.logger.Debug("WorkflowRun status extracted",
		"runName", run.Name,
		"currentDBStatus", run.Status,
		"extractedStatus", status)

	// Update DB based on status
	updates := make(map[string]interface{})

	switch status {
	case "Succeeded":
		updates["status"] = models.RunStatusSuccess
		now := time.Now()
		updates["completed_at"] = now
		if run.StartedAt == nil {
			updates["started_at"] = now
		}

	case "Failed":
		updates["status"] = models.RunStatusFailed
		now := time.Now()
		updates["completed_at"] = now
		if run.StartedAt == nil {
			updates["started_at"] = now
		}
		if errorMsg := s.extractErrorMessage(cr); errorMsg != "" {
			updates["error_message"] = errorMsg
		} else {
			updates["error_message"] = "workflow completed with unknown failure — no reason or message in WorkflowRun conditions"
		}

	case "Running":
		if run.Status != models.RunStatusRunning {
			updates["status"] = models.RunStatusRunning
			if run.StartedAt == nil {
				now := time.Now()
				updates["started_at"] = now
			}
		}

	case "Pending":
		// Keep as pending
		return nil

	default:
		s.logger.Warn("Unknown workflow status", "status", status, "workflowRunName", run.Name)
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

// extractWorkflowStatus extracts the status from a WorkflowRun CR
func (s *monitorSchedulerService) extractWorkflowStatus(cr map[string]interface{}) (string, error) {
	status, ok := cr["status"].(map[string]interface{})
	if !ok {
		return "Pending", nil
	}

	conditions, ok := status["conditions"].([]interface{})
	if !ok || len(conditions) == 0 {
		return "Pending", nil
	}

	// Look for WorkflowCompleted condition
	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _ := condMap["type"].(string)
		if condType == "WorkflowCompleted" {
			condStatus, _ := condMap["status"].(string)
			reason, _ := condMap["reason"].(string)

			if condStatus == "True" {
				if reason == "WorkflowSucceeded" {
					return "Succeeded", nil
				}
				return "Failed", nil
			}

			// status is "False" or "Unknown"
			switch reason {
			case "WorkflowPending":
				return "Pending", nil
			case "WorkflowRunning":
				return "Running", nil
			default:
				// Unknown reason — could be a controller error (e.g., render failure)
				message, _ := condMap["message"].(string)
				if message != "" {
					s.logger.Error("WorkflowRun has error condition",
						"reason", reason, "message", message)
					return "Failed", nil
				}
				// No message — stay pending, let timeout handle it
				return "Pending", nil
			}
		}
	}

	return "Pending", nil
}

// extractErrorMessage extracts a descriptive error from the WorkflowRun CR.
// It combines the condition's reason and message fields for better diagnostics.
// Examples: "WorkflowFailed: out of memory", "RenderFailed: missing parameter X"
func (s *monitorSchedulerService) extractErrorMessage(cr map[string]interface{}) string {
	status, ok := cr["status"].(map[string]interface{})
	if !ok {
		return ""
	}

	conditions, ok := status["conditions"].([]interface{})
	if !ok {
		return ""
	}

	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _ := condMap["type"].(string)
		if condType == "WorkflowCompleted" {
			reason, _ := condMap["reason"].(string)
			message, _ := condMap["message"].(string)

			switch {
			case reason != "" && message != "":
				return reason + ": " + message
			case reason != "":
				return reason
			case message != "":
				return message
			}
			return ""
		}
	}

	return ""
}
