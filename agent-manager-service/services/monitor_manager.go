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
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/observabilitysvc"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

const (
	// WorkflowRun CR constants
	resourceKindWorkflowRun  = "WorkflowRun"
	workflowRunAPIVersion    = "openchoreo.dev/v1alpha1"
	monitorLabelResourceType = "amp.wso2.com/resource-type"
	monitorLabelAgentName    = "amp.wso2.com/agent-name"
	monitorResourceTypeValue = "monitor"
)

// MonitorManagerService defines the interface for monitor operations
type MonitorManagerService interface {
	CreateMonitor(ctx context.Context, orgName string, req *models.CreateMonitorRequest) (*models.MonitorResponse, error)
	GetMonitor(ctx context.Context, orgName, monitorName string) (*models.MonitorResponse, error)
	ListMonitors(ctx context.Context, orgName, projectName, agentName string) (*models.MonitorListResponse, error)
	UpdateMonitor(ctx context.Context, orgName, monitorName string, req *models.UpdateMonitorRequest) (*models.MonitorResponse, error)
	DeleteMonitor(ctx context.Context, orgName, monitorName string) error
	StopMonitor(ctx context.Context, orgName, monitorName string) (*models.MonitorResponse, error)
	StartMonitor(ctx context.Context, orgName, monitorName string) (*models.MonitorResponse, error)
	ListMonitorRuns(ctx context.Context, orgName, monitorName string) (*models.MonitorRunsListResponse, error)
	RerunMonitor(ctx context.Context, orgName, monitorName, runID string) (*models.MonitorRunResponse, error)
	GetMonitorRunLogs(ctx context.Context, orgName, monitorName, runID string) (*models.LogsResponse, error)
}

type monitorManagerService struct {
	logger                 *slog.Logger
	ocClient               client.OpenChoreoClient
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient
	executor               MonitorExecutor
}

// NewMonitorManagerService creates a new monitor manager service instance
func NewMonitorManagerService(
	logger *slog.Logger,
	ocClient client.OpenChoreoClient,
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient,
	executor MonitorExecutor,
) MonitorManagerService {
	return &monitorManagerService{
		logger:                 logger,
		ocClient:               ocClient,
		observabilitySvcClient: observabilitySvcClient,
		executor:               executor,
	}
}

// CreateMonitor creates a new evaluation monitor with DB persistence and OpenChoreo CR
func (s *monitorManagerService) CreateMonitor(ctx context.Context, orgName string, req *models.CreateMonitorRequest) (*models.MonitorResponse, error) {
	s.logger.Info("Creating monitor",
		"orgName", orgName,
		"name", req.Name,
		"type", req.Type,
		"agentName", req.AgentName,
		"environmentName", req.EnvironmentName,
		"evaluators", req.Evaluators,
	)

	// Validate type-specific fields
	if err := s.validateCreateRequest(req); err != nil {
		return nil, err
	}

	// Resolve agent ID via OpenChoreo
	agent, err := s.ocClient.GetComponent(ctx, orgName, req.ProjectName, req.AgentName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve agent: %w", err)
	}

	// Resolve environment ID using user-provided environment name
	env, err := s.ocClient.GetEnvironment(ctx, orgName, req.EnvironmentName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve environment: %w", err)
	}

	// Set defaults
	samplingRate := models.DefaultSamplingRate
	if req.SamplingRate != nil {
		samplingRate = *req.SamplingRate
	}

	var intervalMinutes *int
	var nextRunTime *time.Time
	if req.Type == models.MonitorTypeFuture {
		defInterval := models.DefaultIntervalMinutes
		if req.IntervalMinutes != nil {
			defInterval = *req.IntervalMinutes
		}
		intervalMinutes = &defInterval

		// Set next_run_time to NOW() so scheduler triggers within 60 seconds
		now := time.Now()
		nextRunTime = &now
	}

	// Save to DB
	monitor := &models.Monitor{
		ID:              uuid.New(),
		Name:            req.Name,
		DisplayName:     req.DisplayName,
		Type:            req.Type,
		OrgName:         orgName,
		ProjectName:     req.ProjectName,
		AgentName:       req.AgentName,
		AgentID:         agent.UUID,
		EnvironmentName: env.Name,
		EnvironmentID:   env.UUID,
		Evaluators:      req.Evaluators,
		IntervalMinutes: intervalMinutes,
		NextRunTime:     nextRunTime,
		TraceStart:      req.TraceStart,
		TraceEnd:        req.TraceEnd,
		SamplingRate:    samplingRate,
	}

	if err := db.DB(ctx).Create(monitor).Error; err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, utils.ErrMonitorAlreadyExists
		}
		return nil, fmt.Errorf("failed to save monitor: %w", err)
	}

	var latestRun *models.MonitorRunResponse

	// Handle monitor type-specific workflow creation
	if req.Type == models.MonitorTypeFuture {
		// Future monitors: scheduler handles all WorkflowRun creation
		// next_run_time is set to NOW(), so scheduler will trigger within 60 seconds
		s.logger.Info("Future monitor created, scheduler will trigger first run", "name", req.Name)
	} else {
		// Past monitors: create WorkflowRun CR immediately for one-off execution
		result, err := s.executor.ExecuteMonitorRun(ctx, ExecuteMonitorRunParams{
			OrgName:    orgName,
			Monitor:    monitor,
			StartTime:  *req.TraceStart,
			EndTime:    *req.TraceEnd,
			Evaluators: monitor.Evaluators, // Snapshot at creation time
		})
		if err != nil {
			// Rollback DB entry on CR creation failure
			if delErr := db.DB(ctx).Delete(monitor).Error; delErr != nil {
				s.logger.Error("Failed to rollback monitor DB entry", "error", delErr)
			}
			return nil, err
		}

		if result.Run != nil {
			latestRun = result.Run.ToResponse()
		}
	}

	s.logger.Info("Monitor created successfully", "name", req.Name, "id", monitor.ID)
	return monitor.ToResponse(models.MonitorStatusActive, latestRun), nil
}

// GetMonitor retrieves a single monitor with DB config + live CR status
func (s *monitorManagerService) GetMonitor(ctx context.Context, orgName, monitorName string) (*models.MonitorResponse, error) {
	s.logger.Debug("Getting monitor", "orgName", orgName, "name", monitorName)

	var monitor models.Monitor
	if err := db.DB(ctx).Where("name = ? AND org_name = ?", monitorName, orgName).First(&monitor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	latestRun := s.getLatestRun(ctx, monitor.ID)
	status := s.getMonitorStatus(ctx, monitor.ID, monitor.Type, monitor.NextRunTime)

	return monitor.ToResponse(status, latestRun), nil
}

// ListMonitors lists all monitors for an organization with live status enrichment
func (s *monitorManagerService) ListMonitors(ctx context.Context, orgName, projectName, agentName string) (*models.MonitorListResponse, error) {
	s.logger.Debug("Listing monitors", "orgName", orgName, "projectName", projectName, "agentName", agentName)

	var monitors []models.Monitor
	if err := db.DB(ctx).Where("org_name = ? AND project_name = ? AND agent_name = ?", orgName, projectName, agentName).Order("created_at DESC").Find(&monitors).Error; err != nil {
		return nil, fmt.Errorf("failed to list monitors: %w", err)
	}

	responses := make([]models.MonitorResponse, 0, len(monitors))
	for i := range monitors {
		latestRun := s.getLatestRun(ctx, monitors[i].ID)
		status := s.getMonitorStatus(ctx, monitors[i].ID, monitors[i].Type, monitors[i].NextRunTime)
		responses = append(responses, *monitors[i].ToResponse(status, latestRun))
	}

	return &models.MonitorListResponse{
		Monitors: responses,
		Total:    len(responses),
	}, nil
}

// UpdateMonitor applies partial updates to a monitor (DB + re-apply CR)
func (s *monitorManagerService) UpdateMonitor(ctx context.Context, orgName, monitorName string, req *models.UpdateMonitorRequest) (*models.MonitorResponse, error) {
	s.logger.Info("Updating monitor", "orgName", orgName, "name", monitorName)

	var monitor models.Monitor
	if err := db.DB(ctx).Where("name = ? AND org_name = ?", monitorName, orgName).First(&monitor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	// Apply partial updates
	if req.DisplayName != nil {
		monitor.DisplayName = *req.DisplayName
	}
	if req.Evaluators != nil {
		monitor.Evaluators = *req.Evaluators
	}
	if req.IntervalMinutes != nil {
		if *req.IntervalMinutes < 5 {
			return nil, fmt.Errorf("intervalMinutes must be at least 5: %w", utils.ErrInvalidInput)
		}
		monitor.IntervalMinutes = req.IntervalMinutes
	}
	if req.TraceStart != nil {
		monitor.TraceStart = req.TraceStart
	}
	if req.TraceEnd != nil {
		monitor.TraceEnd = req.TraceEnd
	}
	if req.SamplingRate != nil {
		if *req.SamplingRate <= 0 || *req.SamplingRate > 1 {
			return nil, fmt.Errorf("samplingRate must be between 0 (exclusive) and 1 (inclusive): %w", utils.ErrInvalidInput)
		}
		monitor.SamplingRate = *req.SamplingRate
	}
	if req.Suspended != nil {
		// Handle suspended state update
		if *req.Suspended {
			// For future monitors, we could set next_run_time to NULL or far future
			// For now, just log it - the scheduler will skip if needed
			s.logger.Info("Monitor suspended", "name", monitorName)
		}
	}

	if err := db.DB(ctx).Save(&monitor).Error; err != nil {
		return nil, fmt.Errorf("failed to update monitor: %w", err)
	}

	// Note: No CR re-application needed
	// - Future monitors: scheduler creates individual WorkflowRun CRs per execution
	// - Past monitors: one-off execution, CR already created
	// Updated evaluators will be used in future runs via the evaluator snapshot mechanism

	latestRun := s.getLatestRun(ctx, monitor.ID)
	status := s.getMonitorStatus(ctx, monitor.ID, monitor.Type, monitor.NextRunTime)

	s.logger.Info("Monitor updated successfully", "name", monitorName)
	return monitor.ToResponse(status, latestRun), nil
}

// DeleteMonitor removes a monitor from DB and attempts to clean up any WorkflowRun CRs
func (s *monitorManagerService) DeleteMonitor(ctx context.Context, orgName, monitorName string) error {
	s.logger.Info("Deleting monitor", "orgName", orgName, "name", monitorName)

	// Get monitor first to check type and get runs
	var monitor models.Monitor
	if err := db.DB(ctx).Where("name = ? AND org_name = ?", monitorName, orgName).First(&monitor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrMonitorNotFound
		}
		return fmt.Errorf("failed to get monitor: %w", err)
	}

	// Get all runs to delete their WorkflowRun CRs
	var runs []models.MonitorRun
	if err := db.DB(ctx).Where("monitor_id = ?", monitor.ID).Find(&runs).Error; err != nil {
		s.logger.Error("Failed to get monitor runs for cleanup", "error", err)
	}

	// Delete from DB (cascade will delete runs)
	if err := db.DB(ctx).Delete(&monitor).Error; err != nil {
		return fmt.Errorf("failed to delete monitor from DB: %w", err)
	}

	// Clean up WorkflowRun CRs for all runs
	for _, run := range runs {
		deleteCR := map[string]interface{}{
			"apiVersion": workflowRunAPIVersion,
			"kind":       resourceKindWorkflowRun,
			"metadata": map[string]interface{}{
				"name":      run.Name,
				"namespace": orgName,
			},
		}
		if err := s.ocClient.DeleteResource(ctx, deleteCR); err != nil {
			// Log but don't fail — DB is already cleaned up
			s.logger.Debug("Failed to delete WorkflowRun CR (may already be deleted)",
				"workflowRunName", run.Name, "error", err)
		}
	}

	s.logger.Info("Monitor deleted successfully", "name", monitorName)
	return nil
}

// StopMonitor stops a future monitor by setting next_run_time to NULL
func (s *monitorManagerService) StopMonitor(ctx context.Context, orgName, monitorName string) (*models.MonitorResponse, error) {
	s.logger.Info("Stopping monitor", "orgName", orgName, "name", monitorName)

	var monitor models.Monitor
	if err := db.DB(ctx).Where("name = ? AND org_name = ?", monitorName, orgName).First(&monitor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	// Validate: Only future monitors can be stopped
	if monitor.Type != models.MonitorTypeFuture {
		return nil, fmt.Errorf("cannot stop past monitor: %w", utils.ErrInvalidInput)
	}

	// Check if already stopped (idempotency check)
	if monitor.NextRunTime == nil {
		return nil, utils.ErrMonitorAlreadyStopped
	}

	// Set next_run_time to NULL to suspend scheduling
	if err := db.DB(ctx).Model(&monitor).Update("next_run_time", nil).Error; err != nil {
		return nil, fmt.Errorf("failed to stop monitor: %w", err)
	}

	// Refresh monitor from DB
	if err := db.DB(ctx).Where("name = ? AND org_name = ?", monitorName, orgName).First(&monitor).Error; err != nil {
		return nil, fmt.Errorf("failed to reload monitor: %w", err)
	}

	latestRun := s.getLatestRun(ctx, monitor.ID)
	status := s.getMonitorStatus(ctx, monitor.ID, monitor.Type, monitor.NextRunTime)

	s.logger.Info("Monitor stopped successfully", "name", monitorName, "status", status)
	return monitor.ToResponse(status, latestRun), nil
}

// StartMonitor starts a stopped future monitor by setting next_run_time to NOW()
func (s *monitorManagerService) StartMonitor(ctx context.Context, orgName, monitorName string) (*models.MonitorResponse, error) {
	s.logger.Info("Starting monitor", "orgName", orgName, "name", monitorName)

	var monitor models.Monitor
	if err := db.DB(ctx).Where("name = ? AND org_name = ?", monitorName, orgName).First(&monitor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	// Validate: Only future monitors can be started
	if monitor.Type != models.MonitorTypeFuture {
		return nil, fmt.Errorf("cannot start past monitor: %w", utils.ErrInvalidInput)
	}

	// Check if already active (idempotency check)
	if monitor.NextRunTime != nil {
		return nil, utils.ErrMonitorAlreadyActive
	}

	// Set next_run_time to NOW() to schedule immediately
	now := time.Now()
	if err := db.DB(ctx).Model(&monitor).Update("next_run_time", now).Error; err != nil {
		return nil, fmt.Errorf("failed to start monitor: %w", err)
	}

	// Refresh monitor from DB
	if err := db.DB(ctx).Where("name = ? AND org_name = ?", monitorName, orgName).First(&monitor).Error; err != nil {
		return nil, fmt.Errorf("failed to reload monitor: %w", err)
	}

	latestRun := s.getLatestRun(ctx, monitor.ID)
	status := s.getMonitorStatus(ctx, monitor.ID, monitor.Type, monitor.NextRunTime)

	s.logger.Info("Monitor started successfully", "name", monitorName, "status", status, "nextRunTime", now)
	return monitor.ToResponse(status, latestRun), nil
}

// ListMonitorRuns returns all runs for a specific monitor
func (s *monitorManagerService) ListMonitorRuns(ctx context.Context, orgName, monitorName string) (*models.MonitorRunsListResponse, error) {
	s.logger.Debug("Listing monitor runs", "orgName", orgName, "monitorName", monitorName)

	var monitor models.Monitor
	if err := db.DB(ctx).Where("name = ? AND org_name = ?", monitorName, orgName).First(&monitor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	var runs []models.MonitorRun
	if err := db.DB(ctx).Where("monitor_id = ?", monitor.ID).Order("created_at DESC").Find(&runs).Error; err != nil {
		return nil, fmt.Errorf("failed to list monitor runs: %w", err)
	}

	responses := make([]models.MonitorRunResponse, 0, len(runs))
	for i := range runs {
		resp := runs[i].ToResponse()
		resp.MonitorName = monitorName
		responses = append(responses, *resp)
	}

	return &models.MonitorRunsListResponse{
		Runs:  responses,
		Total: len(responses),
	}, nil
}

// RerunMonitor creates a new workflow execution with the same time parameters as an existing run
func (s *monitorManagerService) RerunMonitor(ctx context.Context, orgName, monitorName, runID string) (*models.MonitorRunResponse, error) {
	s.logger.Info("Rerunning monitor", "orgName", orgName, "monitorName", monitorName, "runID", runID)

	runUUID, err := uuid.Parse(runID)
	if err != nil {
		return nil, fmt.Errorf("invalid run ID: %w", utils.ErrInvalidInput)
	}

	// Get the monitor
	var monitor models.Monitor
	if err := db.DB(ctx).Where("name = ? AND org_name = ?", monitorName, orgName).First(&monitor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	// Get the original run to extract time parameters
	var originalRun models.MonitorRun
	if err := db.DB(ctx).Where("id = ? AND monitor_id = ?", runUUID, monitor.ID).First(&originalRun).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorRunNotFound
		}
		return nil, fmt.Errorf("failed to get monitor run: %w", err)
	}

	// Create new WorkflowRun with same time parameters and evaluators from original run
	result, err := s.executor.ExecuteMonitorRun(ctx, ExecuteMonitorRunParams{
		OrgName:    orgName,
		Monitor:    &monitor,
		StartTime:  originalRun.TraceStart,
		EndTime:    originalRun.TraceEnd,
		Evaluators: originalRun.Evaluators, // Use the same evaluators from original run
	})
	if err != nil {
		return nil, err
	}

	s.logger.Info("Monitor rerun created", "runID", result.Run.ID, "workflowRunName", result.Name)

	resp := result.Run.ToResponse()
	resp.MonitorName = monitorName
	return resp, nil
}

// GetMonitorRunLogs retrieves logs for a specific monitor run
func (s *monitorManagerService) GetMonitorRunLogs(ctx context.Context, orgName, monitorName, runID string) (*models.LogsResponse, error) {
	s.logger.Info("Getting monitor run logs", "orgName", orgName, "monitorName", monitorName, "runID", runID)

	runUUID, err := uuid.Parse(runID)
	if err != nil {
		return nil, fmt.Errorf("invalid run ID: %w", utils.ErrInvalidInput)
	}

	// Get the monitor
	var monitor models.Monitor
	if err := db.DB(ctx).Where("name = ? AND org_name = ?", monitorName, orgName).First(&monitor).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	// Get the monitor run
	var run models.MonitorRun
	if err := db.DB(ctx).Where("id = ? AND monitor_id = ?", runUUID, monitor.ID).First(&run).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorRunNotFound
		}
		return nil, fmt.Errorf("failed to get monitor run: %w", err)
	}

	// Fetch logs from observer service using the workflow run name
	logs, err := s.observabilitySvcClient.GetWorkflowRunLogs(ctx, run.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to get workflow run logs: %w", err)
	}

	s.logger.Info("Fetched monitor run logs successfully", "runID", runID, "logCount", len(logs.Logs))
	return logs, nil
}

// getLatestRun fetches the most recent run for a monitor
func (s *monitorManagerService) getLatestRun(ctx context.Context, monitorID uuid.UUID) *models.MonitorRunResponse {
	var run models.MonitorRun
	if err := db.DB(ctx).Where("monitor_id = ?", monitorID).Order("created_at DESC").First(&run).Error; err != nil {
		return nil
	}
	return run.ToResponse()
}

// getMonitorStatus determines the monitor status based on its type and latest run
func (s *monitorManagerService) getMonitorStatus(ctx context.Context, monitorID uuid.UUID, monitorType string, nextRunTime *time.Time) models.MonitorStatus {
	if monitorType == models.MonitorTypeFuture {
		// Future monitors: check if scheduled
		if nextRunTime != nil {
			return models.MonitorStatusActive
		}
		return models.MonitorStatusSuspended
	}

	// Past monitors: check latest run status
	var run models.MonitorRun
	if err := db.DB(ctx).Where("monitor_id = ?", monitorID).Order("created_at DESC").First(&run).Error; err != nil {
		return models.MonitorStatusUnknown
	}

	switch run.Status {
	case models.RunStatusSuccess:
		return models.MonitorStatusActive // Completed successfully
	case models.RunStatusFailed:
		return models.MonitorStatusFailed
	case models.RunStatusPending, models.RunStatusRunning:
		return models.MonitorStatusActive // In progress
	default:
		return models.MonitorStatusUnknown
	}
}

// validateCreateRequest validates the create monitor request based on type
func (s *monitorManagerService) validateCreateRequest(req *models.CreateMonitorRequest) error {
	if req.Type == models.MonitorTypePast {
		if req.TraceStart == nil || req.TraceEnd == nil {
			return fmt.Errorf("traceStart and traceEnd are required for past monitors: %w", utils.ErrInvalidInput)
		}
		if !req.TraceEnd.After(*req.TraceStart) {
			return fmt.Errorf("traceEnd must be after traceStart: %w", utils.ErrInvalidInput)
		}
		if req.TraceEnd.After(time.Now()) {
			return fmt.Errorf("traceEnd must not be in the future: %w", utils.ErrInvalidInput)
		}
	}
	if req.IntervalMinutes != nil {
		if *req.IntervalMinutes < 5 {
			return fmt.Errorf("intervalMinutes must be at least 5: %w", utils.ErrInvalidInput)
		}
	}
	if req.SamplingRate != nil {
		if *req.SamplingRate <= 0 || *req.SamplingRate > 1 {
			return fmt.Errorf("samplingRate must be between 0 (exclusive) and 1 (inclusive): %w", utils.ErrInvalidInput)
		}
	}
	return nil
}

// parseEvaluators parses a comma-separated string of evaluators
func parseEvaluators(evalStr string) []string {
	if evalStr == "" {
		return nil
	}
	parts := strings.Split(evalStr, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
