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
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
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
	GetMonitor(ctx context.Context, orgName, projectName, agentName, monitorName string) (*models.MonitorResponse, error)
	ListMonitors(ctx context.Context, orgName, projectName, agentName string) (*models.MonitorListResponse, error)
	UpdateMonitor(ctx context.Context, orgName, projectName, agentName, monitorName string, req *models.UpdateMonitorRequest) (*models.MonitorResponse, error)
	DeleteMonitor(ctx context.Context, orgName, projectName, agentName, monitorName string) error
	StopMonitor(ctx context.Context, orgName, projectName, agentName, monitorName string) (*models.MonitorResponse, error)
	StartMonitor(ctx context.Context, orgName, projectName, agentName, monitorName string) (*models.MonitorResponse, error)
	ListMonitorRuns(ctx context.Context, orgName, projectName, agentName, monitorName string, limit, offset int) (*models.MonitorRunsListResponse, error)
	RerunMonitor(ctx context.Context, orgName, projectName, agentName, monitorName, runID string) (*models.MonitorRunResponse, error)
	GetMonitorRunLogs(ctx context.Context, orgName, projectName, agentName, monitorName, runID string) (*models.LogsResponse, error)
}

type monitorManagerService struct {
	logger                 *slog.Logger
	ocClient               client.OpenChoreoClient
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient
	executor               MonitorExecutor
	evaluatorService       EvaluatorManagerService
	monitorRepo            repositories.MonitorRepository
	encryptionKey          []byte
}

// NewMonitorManagerService creates a new monitor manager service instance
func NewMonitorManagerService(
	logger *slog.Logger,
	ocClient client.OpenChoreoClient,
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient,
	executor MonitorExecutor,
	evaluatorService EvaluatorManagerService,
	monitorRepo repositories.MonitorRepository,
	encryptionKey []byte,
) MonitorManagerService {
	return &monitorManagerService{
		logger:                 logger,
		ocClient:               ocClient,
		observabilitySvcClient: observabilitySvcClient,
		executor:               executor,
		evaluatorService:       evaluatorService,
		monitorRepo:            monitorRepo,
		encryptionKey:          encryptionKey,
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

	// Validate evaluators against catalog schema
	if err := s.validateEvaluators(ctx, req.Evaluators); err != nil {
		return nil, err
	}

	// Validate LLM provider configs against catalog
	if err := s.validateLLMProviderConfigs(ctx, req.LLMProviderConfigs); err != nil {
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

	// Encrypt LLM provider config secrets before persisting
	encryptedConfigs, err := utils.EncryptLLMProviderConfigs(req.LLMProviderConfigs, s.encryptionKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt LLM provider configs: %w", err)
	}

	// Save to DB
	monitor := &models.Monitor{
		ID:                 uuid.New(),
		Name:               req.Name,
		DisplayName:        req.DisplayName,
		Type:               req.Type,
		OrgName:            orgName,
		ProjectName:        req.ProjectName,
		AgentName:          req.AgentName,
		AgentID:            agent.UUID,
		EnvironmentName:    env.Name,
		EnvironmentID:      env.UUID,
		Evaluators:         req.Evaluators,
		LLMProviderConfigs: encryptedConfigs,
		IntervalMinutes:    intervalMinutes,
		NextRunTime:        nextRunTime,
		TraceStart:         req.TraceStart,
		TraceEnd:           req.TraceEnd,
		SamplingRate:       samplingRate,
	}

	if err := s.monitorRepo.CreateMonitor(monitor); err != nil {
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
			if delErr := s.monitorRepo.DeleteMonitor(monitor); delErr != nil {
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
func (s *monitorManagerService) GetMonitor(ctx context.Context, orgName, projectName, agentName, monitorName string) (*models.MonitorResponse, error) {
	s.logger.Debug("Getting monitor", "orgName", orgName, "name", monitorName)

	monitor, err := s.monitorRepo.GetMonitorByName(orgName, projectName, agentName, monitorName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	latestRun := s.getLatestRun(monitor.ID)
	status := s.getMonitorStatus(monitor.ID, monitor.Type, monitor.NextRunTime)

	return monitor.ToResponse(status, latestRun), nil
}

// ListMonitors lists all monitors for an organization with live status enrichment
func (s *monitorManagerService) ListMonitors(ctx context.Context, orgName, projectName, agentName string) (*models.MonitorListResponse, error) {
	s.logger.Debug("Listing monitors", "orgName", orgName, "projectName", projectName, "agentName", agentName)

	monitors, err := s.monitorRepo.ListMonitorsByAgent(orgName, projectName, agentName)
	if err != nil {
		return nil, fmt.Errorf("failed to list monitors: %w", err)
	}

	// Batch-load latest runs for all monitors in one query to avoid N+1
	monitorIDs := make([]uuid.UUID, len(monitors))
	for i := range monitors {
		monitorIDs[i] = monitors[i].ID
	}
	latestRunMap, err := s.monitorRepo.GetLatestMonitorRuns(monitorIDs)
	if err != nil {
		s.logger.Error("Failed to batch-load latest runs", "error", err)
		latestRunMap = make(map[uuid.UUID]models.MonitorRun)
	}

	responses := make([]models.MonitorResponse, 0, len(monitors))
	for i := range monitors {
		var latestRun *models.MonitorRunResponse
		if run, ok := latestRunMap[monitors[i].ID]; ok {
			latestRun = run.ToResponse()
		}
		status := s.deriveMonitorStatus(monitors[i].Type, monitors[i].NextRunTime, latestRun)
		responses = append(responses, *monitors[i].ToResponse(status, latestRun))
	}

	return &models.MonitorListResponse{
		Monitors: responses,
		Total:    len(responses),
	}, nil
}

// UpdateMonitor applies partial updates to a monitor (DB + re-apply CR)
func (s *monitorManagerService) UpdateMonitor(ctx context.Context, orgName, projectName, agentName, monitorName string, req *models.UpdateMonitorRequest) (*models.MonitorResponse, error) {
	s.logger.Info("Updating monitor", "orgName", orgName, "name", monitorName)

	monitor, err := s.monitorRepo.GetMonitorByName(orgName, projectName, agentName, monitorName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	// Validate evaluators against catalog schema if provided
	if req.Evaluators != nil {
		if err := s.validateEvaluators(ctx, *req.Evaluators); err != nil {
			return nil, err
		}
	}

	// Apply partial updates
	if req.DisplayName != nil {
		monitor.DisplayName = *req.DisplayName
	}
	if req.Evaluators != nil {
		monitor.Evaluators = *req.Evaluators
	}
	if req.LLMProviderConfigs != nil {
		if err := s.validateLLMProviderConfigs(ctx, *req.LLMProviderConfigs); err != nil {
			return nil, err
		}
		enc, err := utils.EncryptLLMProviderConfigs(*req.LLMProviderConfigs, s.encryptionKey)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt LLM provider configs: %w", err)
		}
		monitor.LLMProviderConfigs = enc
	}
	if req.IntervalMinutes != nil {
		if *req.IntervalMinutes < models.MinIntervalMinutes {
			return nil, fmt.Errorf("intervalMinutes must be at least %d: %w", models.MinIntervalMinutes, utils.ErrInvalidInput)
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
	if err := s.monitorRepo.UpdateMonitor(monitor); err != nil {
		return nil, fmt.Errorf("failed to update monitor: %w", err)
	}

	// Note: No CR re-application needed
	// - Future monitors: scheduler creates individual WorkflowRun CRs per execution
	// - Past monitors: one-off execution, CR already created
	// Updated evaluators will be used in future runs via the evaluator snapshot mechanism

	latestRun := s.getLatestRun(monitor.ID)
	status := s.getMonitorStatus(monitor.ID, monitor.Type, monitor.NextRunTime)

	s.logger.Info("Monitor updated successfully", "name", monitorName)
	return monitor.ToResponse(status, latestRun), nil
}

// DeleteMonitor removes a monitor from DB and attempts to clean up any WorkflowRun CRs
func (s *monitorManagerService) DeleteMonitor(ctx context.Context, orgName, projectName, agentName, monitorName string) error {
	s.logger.Info("Deleting monitor", "orgName", orgName, "name", monitorName)

	// Get monitor first to check type and get runs
	monitor, err := s.monitorRepo.GetMonitorByName(orgName, projectName, agentName, monitorName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrMonitorNotFound
		}
		return fmt.Errorf("failed to get monitor: %w", err)
	}

	// Get all runs to delete their WorkflowRun CRs
	runs, err := s.monitorRepo.GetMonitorRunsByMonitorID(monitor.ID)
	if err != nil {
		s.logger.Error("Failed to get monitor runs for cleanup", "error", err)
	}

	// Delete from DB (cascade will delete runs)
	if err := s.monitorRepo.DeleteMonitor(monitor); err != nil {
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
func (s *monitorManagerService) StopMonitor(ctx context.Context, orgName, projectName, agentName, monitorName string) (*models.MonitorResponse, error) {
	s.logger.Info("Stopping monitor", "orgName", orgName, "name", monitorName)

	monitor, err := s.monitorRepo.GetMonitorByName(orgName, projectName, agentName, monitorName)
	if err != nil {
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
	if err := s.monitorRepo.UpdateNextRunTime(monitor.ID, nil); err != nil {
		return nil, fmt.Errorf("failed to stop monitor: %w", err)
	}

	// Refresh monitor from DB
	monitor, err = s.monitorRepo.GetMonitorByName(orgName, projectName, agentName, monitorName)
	if err != nil {
		return nil, fmt.Errorf("failed to reload monitor: %w", err)
	}

	latestRun := s.getLatestRun(monitor.ID)
	status := s.getMonitorStatus(monitor.ID, monitor.Type, monitor.NextRunTime)

	s.logger.Info("Monitor stopped successfully", "name", monitorName, "status", status)
	return monitor.ToResponse(status, latestRun), nil
}

// StartMonitor starts a stopped future monitor by setting next_run_time to NOW()
func (s *monitorManagerService) StartMonitor(ctx context.Context, orgName, projectName, agentName, monitorName string) (*models.MonitorResponse, error) {
	s.logger.Info("Starting monitor", "orgName", orgName, "name", monitorName)

	monitor, err := s.monitorRepo.GetMonitorByName(orgName, projectName, agentName, monitorName)
	if err != nil {
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
	if err := s.monitorRepo.UpdateNextRunTime(monitor.ID, &now); err != nil {
		return nil, fmt.Errorf("failed to start monitor: %w", err)
	}

	// Refresh monitor from DB
	monitor, err = s.monitorRepo.GetMonitorByName(orgName, projectName, agentName, monitorName)
	if err != nil {
		return nil, fmt.Errorf("failed to reload monitor: %w", err)
	}

	latestRun := s.getLatestRun(monitor.ID)
	status := s.getMonitorStatus(monitor.ID, monitor.Type, monitor.NextRunTime)

	s.logger.Info("Monitor started successfully", "name", monitorName, "status", status, "nextRunTime", now)
	return monitor.ToResponse(status, latestRun), nil
}

// ListMonitorRuns returns paginated runs for a specific monitor
func (s *monitorManagerService) ListMonitorRuns(ctx context.Context, orgName, projectName, agentName, monitorName string, limit, offset int) (*models.MonitorRunsListResponse, error) {
	s.logger.Debug("Listing monitor runs", "orgName", orgName, "monitorName", monitorName)

	monitor, err := s.monitorRepo.GetMonitorByName(orgName, projectName, agentName, monitorName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	// Get total count
	total, err := s.monitorRepo.CountMonitorRuns(monitor.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to count monitor runs: %w", err)
	}

	runs, err := s.monitorRepo.ListMonitorRuns(monitor.ID, limit, offset)
	if err != nil {
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
		Total: int(total),
	}, nil
}

// RerunMonitor creates a new workflow execution with the same time parameters as an existing run
func (s *monitorManagerService) RerunMonitor(ctx context.Context, orgName, projectName, agentName, monitorName, runID string) (*models.MonitorRunResponse, error) {
	s.logger.Info("Rerunning monitor", "orgName", orgName, "monitorName", monitorName, "runID", runID)

	runUUID, err := uuid.Parse(runID)
	if err != nil {
		return nil, fmt.Errorf("invalid run ID: %w", utils.ErrInvalidInput)
	}

	// Get the monitor
	monitor, err := s.monitorRepo.GetMonitorByName(orgName, projectName, agentName, monitorName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	// Get the original run to extract time parameters
	originalRun, err := s.monitorRepo.GetMonitorRunByID(runUUID, monitor.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorRunNotFound
		}
		return nil, fmt.Errorf("failed to get monitor run: %w", err)
	}

	// Create new WorkflowRun with same time parameters and evaluators from the original run
	result, err := s.executor.ExecuteMonitorRun(ctx, ExecuteMonitorRunParams{
		OrgName:    orgName,
		Monitor:    monitor,
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
func (s *monitorManagerService) GetMonitorRunLogs(ctx context.Context, orgName, projectName, agentName, monitorName, runID string) (*models.LogsResponse, error) {
	s.logger.Info("Getting monitor run logs", "orgName", orgName, "monitorName", monitorName, "runID", runID)

	runUUID, err := uuid.Parse(runID)
	if err != nil {
		return nil, fmt.Errorf("invalid run ID: %w", utils.ErrInvalidInput)
	}

	// Get the monitor
	monitor, err := s.monitorRepo.GetMonitorByName(orgName, projectName, agentName, monitorName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrMonitorNotFound
		}
		return nil, fmt.Errorf("failed to get monitor: %w", err)
	}

	// Get the monitor run
	run, err := s.monitorRepo.GetMonitorRunByID(runUUID, monitor.ID)
	if err != nil {
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
func (s *monitorManagerService) getLatestRun(monitorID uuid.UUID) *models.MonitorRunResponse {
	run, err := s.monitorRepo.GetLatestMonitorRun(monitorID)
	if err != nil {
		return nil
	}
	return run.ToResponse()
}

// getMonitorStatus determines the monitor status based on its type and latest run
func (s *monitorManagerService) getMonitorStatus(monitorID uuid.UUID, monitorType string, nextRunTime *time.Time) models.MonitorStatus {
	if monitorType == models.MonitorTypeFuture {
		// Future monitors: check if scheduled
		if nextRunTime != nil {
			return models.MonitorStatusActive
		}
		return models.MonitorStatusSuspended
	}

	// Past monitors: check latest run status
	run, err := s.monitorRepo.GetLatestMonitorRun(monitorID)
	if err != nil {
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

// deriveMonitorStatus derives status from already-loaded data (no DB call)
func (s *monitorManagerService) deriveMonitorStatus(monitorType string, nextRunTime *time.Time, latestRun *models.MonitorRunResponse) models.MonitorStatus {
	if monitorType == models.MonitorTypeFuture {
		if nextRunTime != nil {
			return models.MonitorStatusActive
		}
		return models.MonitorStatusSuspended
	}

	if latestRun == nil {
		return models.MonitorStatusUnknown
	}

	switch latestRun.Status {
	case models.RunStatusSuccess:
		return models.MonitorStatusActive
	case models.RunStatusFailed:
		return models.MonitorStatusFailed
	case models.RunStatusPending, models.RunStatusRunning:
		return models.MonitorStatusActive
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
		if *req.IntervalMinutes < models.MinIntervalMinutes {
			return fmt.Errorf("intervalMinutes must be at least %d: %w", models.MinIntervalMinutes, utils.ErrInvalidInput)
		}
	}
	if req.SamplingRate != nil {
		if *req.SamplingRate <= 0 || *req.SamplingRate > 1 {
			return fmt.Errorf("samplingRate must be between 0 (exclusive) and 1 (inclusive): %w", utils.ErrInvalidInput)
		}
	}
	return nil
}

// validateLLMProviderConfigs validates each LLM provider config entry against the catalog.
// For each entry, the provider is looked up by name and the env var is checked against
// that provider's config fields.
func (s *monitorManagerService) validateLLMProviderConfigs(ctx context.Context, configs []models.MonitorLLMProviderConfig) error {
	for i, c := range configs {
		prefix := fmt.Sprintf("llmProviderConfigs[%d]", i)

		provider, err := s.evaluatorService.GetLLMProvider(ctx, c.ProviderName)
		if err != nil {
			return fmt.Errorf("%s: %w", prefix, err)
		}

		// Check EnvVar is a valid config field for this provider
		valid := false
		for _, f := range provider.ConfigFields {
			if f.EnvVar == c.EnvVar {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("%s: env var %q is not a valid config field for provider %q: %w",
				prefix, c.EnvVar, c.ProviderName, utils.ErrInvalidInput)
		}
	}
	return nil
}

// validateEvaluators validates evaluators against the catalog schema and populates defaults.
// It mutates evaluator configs in-place to fill in default values from the schema.
func (s *monitorManagerService) validateEvaluators(ctx context.Context, evaluators []models.MonitorEvaluator) error {
	// Check for duplicate displayNames
	displayNames := make(map[string]int) // displayName -> first index
	for i, eval := range evaluators {
		if firstIdx, exists := displayNames[eval.DisplayName]; exists {
			return fmt.Errorf("evaluators[%d]: duplicate displayName %q (also used by evaluators[%d]): %w",
				i, eval.DisplayName, firstIdx, utils.ErrInvalidInput)
		}
		displayNames[eval.DisplayName] = i
	}

	for i := range evaluators {
		eval := &evaluators[i]
		prefix := fmt.Sprintf("evaluators[%d]", i)

		// Check evaluator exists in catalog
		evaluatorResp, err := s.evaluatorService.GetEvaluator(ctx, nil, eval.Identifier)
		if err != nil {
			if errors.Is(err, utils.ErrEvaluatorNotFound) {
				return fmt.Errorf("%s: evaluator %q not found in catalog: %w",
					prefix, eval.Identifier, utils.ErrInvalidInput)
			}
			return fmt.Errorf("%s: failed to look up evaluator %q: %w", prefix, eval.Identifier, err)
		}

		// Validate and apply defaults to config (including level)
		if err := validateAndApplyDefaults(i, eval.Identifier, &eval.Config, evaluatorResp.ConfigSchema); err != nil {
			return err
		}
	}
	return nil
}

// validateAndApplyDefaults validates config values against the evaluator's schema
// and populates default values for missing optional params.
func validateAndApplyDefaults(idx int, identifier string, config *map[string]interface{}, schema []models.EvaluatorConfigParam) error {
	prefix := fmt.Sprintf("evaluators[%d]", idx)

	// Build schema lookup
	schemaMap := make(map[string]models.EvaluatorConfigParam)
	for _, p := range schema {
		schemaMap[p.Key] = p
	}

	// Initialize config map if nil
	if *config == nil {
		*config = make(map[string]interface{})
	}

	// Check for unknown keys
	for key := range *config {
		if _, exists := schemaMap[key]; !exists {
			return fmt.Errorf("%s: config key %q is not defined in evaluator %q schema: %w",
				prefix, key, identifier, utils.ErrInvalidInput)
		}
	}

	// Check required params and populate defaults
	for _, param := range schema {
		_, present := (*config)[param.Key]
		if !present {
			if param.Required && param.Default == nil {
				return fmt.Errorf("%s: required config %q is missing for evaluator %q: %w",
					prefix, param.Key, identifier, utils.ErrInvalidInput)
			}
			// Populate default if available
			if param.Default != nil {
				(*config)[param.Key] = param.Default
			}
		}
	}

	// Validate each value against its schema param
	for key, value := range *config {
		param := schemaMap[key]
		if err := validateConfigValue(prefix, param, value); err != nil {
			return err
		}
	}

	return nil
}

// validateConfigValue validates a single config value against its schema param
func validateConfigValue(prefix string, param models.EvaluatorConfigParam, value interface{}) error {
	switch param.Type {
	case "string":
		if _, ok := value.(string); !ok {
			return fmt.Errorf("%s: config %q must be a string: %w",
				prefix, param.Key, utils.ErrInvalidInput)
		}
		// Check enum values for string type with enum_values
		if len(param.EnumValues) > 0 {
			strVal := value.(string)
			found := false
			for _, ev := range param.EnumValues {
				if ev == strVal {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("%s: config %q must be one of %v: %w",
					prefix, param.Key, param.EnumValues, utils.ErrInvalidInput)
			}
		}

	case "integer":
		num, ok := toFloat64(value)
		if !ok {
			return fmt.Errorf("%s: config %q must be an integer: %w",
				prefix, param.Key, utils.ErrInvalidInput)
		}
		if num != float64(int64(num)) {
			return fmt.Errorf("%s: config %q must be an integer: %w",
				prefix, param.Key, utils.ErrInvalidInput)
		}
		if err := checkMinMax(prefix, param, num); err != nil {
			return err
		}

	case "float":
		num, ok := toFloat64(value)
		if !ok {
			return fmt.Errorf("%s: config %q must be a float: %w",
				prefix, param.Key, utils.ErrInvalidInput)
		}
		if err := checkMinMax(prefix, param, num); err != nil {
			return err
		}

	case "boolean":
		if _, ok := value.(bool); !ok {
			return fmt.Errorf("%s: config %q must be a boolean: %w",
				prefix, param.Key, utils.ErrInvalidInput)
		}

	case "array":
		if _, ok := value.([]interface{}); !ok {
			return fmt.Errorf("%s: config %q must be an array: %w",
				prefix, param.Key, utils.ErrInvalidInput)
		}

	case "enum":
		strVal, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s: config %q must be a string: %w",
				prefix, param.Key, utils.ErrInvalidInput)
		}
		found := false
		for _, ev := range param.EnumValues {
			if ev == strVal {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%s: config %q must be one of %v: %w",
				prefix, param.Key, param.EnumValues, utils.ErrInvalidInput)
		}
	}

	return nil
}

// toFloat64 extracts a float64 from a value (handles JSON number decoding)
func toFloat64(val interface{}) (float64, bool) {
	switch v := val.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	default:
		return 0, false
	}
}

// checkMinMax validates a numeric value against min/max constraints
func checkMinMax(prefix string, param models.EvaluatorConfigParam, num float64) error {
	if param.Min != nil && num < *param.Min {
		return fmt.Errorf("%s: config %q must be >= %v: %w",
			prefix, param.Key, *param.Min, utils.ErrInvalidInput)
	}
	if param.Max != nil && num > *param.Max {
		return fmt.Errorf("%s: config %q must be <= %v: %w",
			prefix, param.Key, *param.Max, utils.ErrInvalidInput)
	}
	return nil
}
