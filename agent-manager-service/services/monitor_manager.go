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

	"github.com/wso2/agent-manager/agent-manager-service/clients/observabilitysvc"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/clients/secretmanagersvc"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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
	ListMonitorRuns(ctx context.Context, orgName, projectName, agentName, monitorName string, limit, offset int, includeScores bool) (*models.MonitorRunsListResponse, error)
	RerunMonitor(ctx context.Context, orgName, projectName, agentName, monitorName, runID string) (*models.MonitorRunResponse, error)
	GetMonitorRunLogs(ctx context.Context, orgName, projectName, agentName, monitorName, runID string) (*models.LogsResponse, error)
}

type monitorManagerService struct {
	logger                 *slog.Logger
	db                     *gorm.DB
	ocClient               client.OpenChoreoClient
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient
	executor               MonitorExecutor
	evaluatorService       EvaluatorManagerService
	monitorRepo            repositories.MonitorRepository
	scoreRepo              repositories.ScoreRepository
	llmProvisioner         *LLMProxyProvisioner
	monitorLLMMappingRepo  repositories.MonitorLLMMappingRepository
	provisioner            PublisherCredentialProvisioner
}

// NewMonitorManagerService creates a new monitor manager service instance
func NewMonitorManagerService(
	logger *slog.Logger,
	db *gorm.DB,
	ocClient client.OpenChoreoClient,
	observabilitySvcClient observabilitysvc.ObservabilitySvcClient,
	executor MonitorExecutor,
	evaluatorService EvaluatorManagerService,
	monitorRepo repositories.MonitorRepository,
	scoreRepo repositories.ScoreRepository,
	llmProvisioner *LLMProxyProvisioner,
	monitorLLMMappingRepo repositories.MonitorLLMMappingRepository,
	provisioner PublisherCredentialProvisioner,
) MonitorManagerService {
	return &monitorManagerService{
		logger:                 logger,
		db:                     db,
		ocClient:               ocClient,
		observabilitySvcClient: observabilitySvcClient,
		executor:               executor,
		evaluatorService:       evaluatorService,
		monitorRepo:            monitorRepo,
		scoreRepo:              scoreRepo,
		llmProvisioner:         llmProvisioner,
		monitorLLMMappingRepo:  monitorLLMMappingRepo,
		provisioner:            provisioner,
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
	hasLLMJudge, err := s.validateEvaluators(ctx, orgName, req.Evaluators)
	if err != nil {
		return nil, err
	}
	if hasLLMJudge && req.LLMProvider == nil {
		return nil, fmt.Errorf("llmProvider is required when using llm_judge evaluators: %w", utils.ErrInvalidInput)
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

	// Provision per-org publisher credentials (Thunder OAuth app + secret storage)
	// Extract org UUID from JWT claims if available
	orgUUID := ""
	if claims := jwtassertion.GetTokenClaims(ctx); claims != nil {
		orgUUID = claims.OuId
	}
	if s.provisioner.IsThunderMode() && orgUUID == "" {
		return nil, fmt.Errorf("missing organization unit ID (ouId) in token — required when Thunder is configured")
	}
	if _, err := s.provisioner.EnsureCredentials(ctx, orgName, orgUUID); err != nil {
		return nil, fmt.Errorf("failed to provision publisher credentials: %w", err)
	}

	// Save to DB
	monitor := &models.Monitor{
		ID:              uuid.New(),
		Name:            req.Name,
		DisplayName:     req.DisplayName,
		Description:     req.Description,
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

	if err := s.monitorRepo.CreateMonitor(monitor); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return nil, utils.ErrMonitorAlreadyExists
		}
		return nil, fmt.Errorf("failed to save monitor: %w", err)
	}

	// Provision LLM proxy for the configured org-level provider
	if req.LLMProvider != nil {
		envUUID, err := uuid.Parse(env.UUID)
		if err != nil {
			if delErr := s.monitorRepo.DeleteMonitor(monitor); delErr != nil {
				s.logger.Error("Failed to rollback monitor on error", "error", delErr)
			}
			return nil, fmt.Errorf("invalid environment UUID: %w", err)
		}

		gateway, err := s.llmProvisioner.ResolveGateway(ctx, envUUID, orgName)
		if err != nil {
			if delErr := s.monitorRepo.DeleteMonitor(monitor); delErr != nil {
				s.logger.Error("Failed to rollback monitor on error", "error", delErr)
			}
			return nil, fmt.Errorf("failed to resolve gateway: %w", err)
		}

		project, err := s.ocClient.GetProject(ctx, orgName, req.ProjectName)
		if err != nil {
			if delErr := s.monitorRepo.DeleteMonitor(monitor); delErr != nil {
				s.logger.Error("Failed to rollback monitor on error", "error", delErr)
			}
			return nil, fmt.Errorf("failed to get project: %w", err)
		}
		projectUUID, err := uuid.Parse(project.UUID)
		if err != nil {
			if delErr := s.monitorRepo.DeleteMonitor(monitor); delErr != nil {
				s.logger.Error("Failed to rollback monitor on error", "error", delErr)
			}
			return nil, fmt.Errorf("invalid project UUID: %w", err)
		}

		mapping, rollbackState, proxyAPIKey, err := s.provisionLLMProxy(ctx, orgName, monitor, *req.LLMProvider, gateway, projectUUID)
		if err != nil {
			if delErr := s.monitorRepo.DeleteMonitor(monitor); delErr != nil {
				s.logger.Error("Failed to rollback monitor on error", "error", delErr)
			}
			return nil, fmt.Errorf("failed to provision LLM proxy: %w", err)
		}

		if err := s.monitorLLMMappingRepo.Create(ctx, s.db, mapping); err != nil {
			s.llmProvisioner.RollbackProxy(ctx, rollbackState, orgName)
			if delErr := s.monitorRepo.DeleteMonitor(monitor); delErr != nil {
				s.logger.Error("Failed to rollback monitor on error", "error", delErr)
			}
			return nil, fmt.Errorf("failed to save monitor LLM mapping: %w", err)
		}

		// Write composite secret only after the DB row is committed so that a persist
		// failure above does not corrupt the existing proxy's credentials.
		compositeLoc := monitorCompositeSecretLocation(orgName, monitor.ID)
		if _, err := s.llmProvisioner.SecretClient().CreateSecret(ctx, compositeLoc,
			map[string]string{"LLM_API_KEY": proxyAPIKey}); err != nil {
			s.llmProvisioner.RollbackProxy(ctx, rollbackState, orgName)
			if delErr := s.monitorLLMMappingRepo.DeleteByMonitorIDAndProxyUUID(ctx, s.db, monitor.ID, mapping.LLMProxyUUID); delErr != nil {
				s.logger.Error("Failed to rollback monitor LLM mapping on secret write failure", "error", delErr)
			}
			if delErr := s.monitorRepo.DeleteMonitor(monitor); delErr != nil {
				s.logger.Error("Failed to rollback monitor on error", "error", delErr)
			}
			return nil, fmt.Errorf("failed to write composite LLM proxy secret: %w", err)
		}
	}

	var latestRun *models.MonitorRunResponse

	if monitor.Type == models.MonitorTypePast {
		// Past monitors: trigger evaluation run immediately
		result, err := s.executor.ExecuteMonitorRun(ctx, ExecuteMonitorRunParams{
			OrgName:    orgName,
			Monitor:    monitor,
			StartTime:  *monitor.TraceStart,
			EndTime:    *monitor.TraceEnd,
			Evaluators: monitor.Evaluators,
		})
		if err != nil {
			// Rollback DB entry on run creation failure
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

	resp := monitor.ToResponse(models.MonitorStatusActive, latestRun)

	// Enrich with LLM provider info
	llmProvider, err := s.buildMonitorLLMProviderInfo(ctx, monitor.ID, orgName)
	if err != nil {
		s.logger.Warn("Failed to load LLM provider info for new monitor", "monitor", req.Name, "error", err)
	} else {
		resp.LLMProvider = llmProvider
	}

	return resp, nil
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

	resp := monitor.ToResponse(status, latestRun)

	// Enrich with LLM provider info
	llmProvider, err := s.buildMonitorLLMProviderInfo(ctx, monitor.ID, orgName)
	if err != nil {
		s.logger.Warn("Failed to load LLM provider info for monitor", "monitor", monitorName, "error", err)
	} else {
		resp.LLMProvider = llmProvider
	}

	return resp, nil
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
		resp := monitors[i].ToResponse(status, latestRun)

		// Enrich with LLM provider info
		llmProvider, err := s.buildMonitorLLMProviderInfo(ctx, monitors[i].ID, orgName)
		if err != nil {
			s.logger.Warn("Failed to load LLM provider info", "monitor", monitors[i].Name, "error", err)
		} else {
			resp.LLMProvider = llmProvider
		}

		responses = append(responses, *resp)
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

	// Validate evaluator list and enforce llm_judge ↔ provider invariant.
	// Run when new evaluators are provided (full schema check) OR when the provider is
	// being cleared (must confirm no llm_judge remains in the effective evaluator set).
	if req.Evaluators != nil || req.ClearLLMProvider {
		evalList := monitor.Evaluators
		if req.Evaluators != nil {
			evalList = *req.Evaluators
		}
		hasLLMJudge, err := s.validateEvaluators(ctx, monitor.OrgName, evalList)
		if err != nil {
			return nil, err
		}
		if hasLLMJudge {
			// Provider must remain configured after this update.
			if req.ClearLLMProvider {
				return nil, fmt.Errorf("cannot remove llmProvider while llm_judge evaluators are configured: %w", utils.ErrInvalidInput)
			}
			// If no new provider is being set, the monitor must already have one.
			if req.LLMProvider == nil {
				existing, err := s.buildMonitorLLMProviderInfo(ctx, monitor.ID, monitor.OrgName)
				if err != nil || existing == nil {
					return nil, fmt.Errorf("llmProvider is required when using llm_judge evaluators: %w", utils.ErrInvalidInput)
				}
			}
		}
	}

	// Apply partial updates
	if req.DisplayName != nil {
		monitor.DisplayName = *req.DisplayName
	}
	if req.Evaluators != nil {
		monitor.Evaluators = *req.Evaluators
	}
	// Reconcile org-level LLM provider (proxy-based path)
	if req.ClearLLMProvider {
		if err := s.cleanupLLMProxies(ctx, orgName, monitor.ID); err != nil {
			s.logger.Error("Failed to cleanup LLM proxy during update", "error", err)
		}
	} else if req.LLMProvider != nil {
		// Load old mappings before provisioning so we can clean them up after the new one is saved.
		oldMappings, err := s.monitorLLMMappingRepo.ListByMonitorID(ctx, monitor.ID)
		if err != nil {
			return nil, fmt.Errorf("failed to list existing LLM mappings: %w", err)
		}

		envUUID, err := uuid.Parse(monitor.EnvironmentID)
		if err != nil {
			return nil, fmt.Errorf("invalid environment UUID: %w", err)
		}

		gateway, err := s.llmProvisioner.ResolveGateway(ctx, envUUID, orgName)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve gateway: %w", err)
		}

		project, err := s.ocClient.GetProject(ctx, orgName, monitor.ProjectName)
		if err != nil {
			return nil, fmt.Errorf("failed to get project: %w", err)
		}
		projectUUID, err := uuid.Parse(project.UUID)
		if err != nil {
			return nil, fmt.Errorf("invalid project UUID: %w", err)
		}

		mapping, rollbackState, proxyAPIKey, err := s.provisionLLMProxy(ctx, orgName, monitor, *req.LLMProvider, gateway, projectUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to provision LLM proxy: %w", err)
		}

		if err := s.monitorLLMMappingRepo.Create(ctx, s.db, mapping); err != nil {
			s.llmProvisioner.RollbackProxy(ctx, rollbackState, orgName)
			return nil, fmt.Errorf("failed to save monitor LLM mapping: %w", err)
		}

		// Write composite secret only after the new DB row is committed. Writing it
		// before would corrupt the old proxy's credentials if Create failed above.
		compositeLoc := monitorCompositeSecretLocation(orgName, monitor.ID)
		if _, err := s.llmProvisioner.SecretClient().CreateSecret(ctx, compositeLoc,
			map[string]string{"LLM_API_KEY": proxyAPIKey}); err != nil {
			s.llmProvisioner.RollbackProxy(ctx, rollbackState, orgName)
			if delErr := s.monitorLLMMappingRepo.DeleteByMonitorIDAndProxyUUID(ctx, s.db, monitor.ID, mapping.LLMProxyUUID); delErr != nil {
				s.logger.Error("Failed to rollback monitor LLM mapping on secret write failure", "error", delErr)
			}
			return nil, fmt.Errorf("failed to write composite LLM proxy secret: %w", err)
		}

		// Clean up old proxies now that new mapping is saved.
		// Use per-proxy deletion to avoid removing the newly-created mapping row.
		for _, oldMapping := range oldMappings {
			if err := s.monitorLLMMappingRepo.DeleteByMonitorIDAndProxyUUID(ctx, s.db, monitor.ID, oldMapping.LLMProxyUUID); err != nil {
				s.logger.Error("Failed to delete old LLM mapping row during update", "proxyUUID", oldMapping.LLMProxyUUID, "error", err)
			}
			if oldMapping.LLMProxy == nil {
				continue
			}
			if err := s.llmProvisioner.CleanupProxy(ctx, oldMapping.LLMProxy, orgName, ProxySecretContext{}); err != nil {
				s.logger.Error("Failed to cleanup old LLM proxy during update", "error", err)
			}
		}
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

	var latestRun *models.MonitorRunResponse

	if monitor.Type == models.MonitorTypePast {
		// Past monitors: trigger a new evaluation run with updated config
		result, err := s.executor.ExecuteMonitorRun(ctx, ExecuteMonitorRunParams{
			OrgName:    orgName,
			Monitor:    monitor,
			StartTime:  *monitor.TraceStart,
			EndTime:    *monitor.TraceEnd,
			Evaluators: monitor.Evaluators,
		})
		if err != nil {
			s.logger.Error("Failed to trigger past monitor run after update", "name", monitorName, "error", err)
			return nil, fmt.Errorf("monitor updated but failed to trigger evaluation run: %w", err)
		}
		if result.Run != nil {
			latestRun = result.Run.ToResponse()
		}
	}

	if latestRun == nil {
		latestRun = s.getLatestRun(monitor.ID)
	}
	status := s.getMonitorStatus(monitor.ID, monitor.Type, monitor.NextRunTime)

	s.logger.Info("Monitor updated successfully", "name", monitorName)

	resp := monitor.ToResponse(status, latestRun)

	// Enrich with LLM provider info
	llmProvider, err := s.buildMonitorLLMProviderInfo(ctx, monitor.ID, orgName)
	if err != nil {
		s.logger.Warn("Failed to load LLM provider info for updated monitor", "monitor", monitorName, "error", err)
	} else {
		resp.LLMProvider = llmProvider
	}

	return resp, nil
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

	// Clean up LLM proxies before deleting the monitor
	if err := s.cleanupLLMProxies(ctx, orgName, monitor.ID); err != nil {
		s.logger.Error("Failed to cleanup LLM proxies during monitor deletion", "error", err)
		// Continue with deletion — proxies are best-effort cleanup
	}

	// Get all runs to delete their WorkflowRun CRs
	runs, err := s.monitorRepo.GetMonitorRunsByMonitorID(monitor.ID)
	if err != nil {
		s.logger.Error("Failed to get monitor runs for cleanup", "error", err)
	}

	// Delete from DB (cascade will delete runs and monitor_llm_mapping)
	if err := s.monitorRepo.DeleteMonitor(monitor); err != nil {
		return fmt.Errorf("failed to delete monitor from DB: %w", err)
	}

	// Expire WorkflowRuns by setting a short TTL
	for _, run := range runs {
		// Todo: This would be replaced by deletion once OpenChoreo supports it
		s.logger.Info("Calling ExpireWorkflowRun", "orgName", orgName, "runName", run.Name)
		if err := s.ocClient.ExpireWorkflowRun(ctx, orgName, run.Name); err != nil {
			s.logger.Error("Failed to expire WorkflowRun", "monitorName", monitorName, "runName", run.Name, "error", err)
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
func (s *monitorManagerService) ListMonitorRuns(ctx context.Context, orgName, projectName, agentName, monitorName string, limit, offset int, includeScores bool) (*models.MonitorRunsListResponse, error) {
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

	if includeScores && len(responses) > 0 {
		runIDs := make([]uuid.UUID, len(runs))
		for i := range runs {
			runIDs[i] = runs[i].ID
		}

		evaluators, err := s.scoreRepo.GetEvaluatorsByMonitorAndRunIDs(monitor.ID, runIDs)
		if err != nil {
			s.logger.Error("Failed to fetch run scores", "error", err)
		} else {
			// Group evaluators by run ID
			scoresByRun := make(map[string][]models.EvaluatorScoreSummary)
			for _, eval := range evaluators {
				runID := eval.MonitorRunID.String()
				aggs := eval.Aggregations
				if aggs == nil {
					aggs = make(map[string]interface{})
				}
				// When all evaluations were skipped, clear aggregations so the
				// frontend receives null/empty instead of a misleading 0.
				if eval.Count > 0 && eval.SkippedCount >= eval.Count {
					aggs = make(map[string]interface{})
				}
				scoresByRun[runID] = append(scoresByRun[runID], models.EvaluatorScoreSummary{
					EvaluatorName: eval.EvaluatorName,
					Level:         eval.Level,
					Count:         eval.Count,
					SkippedCount:  eval.SkippedCount,
					Aggregations:  aggs,
				})
			}
			for i := range responses {
				if scores, ok := scoresByRun[responses[i].ID]; ok {
					responses[i].Scores = scores
				}
			}
		}
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
	logs, err := s.observabilitySvcClient.GetWorkflowRunLogs(ctx, run.Name, orgName)
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

// validateEvaluators validates evaluators against the catalog schema and populates defaults.
// It mutates evaluator configs in-place to fill in default values from the schema.
// Returns true if any evaluator requires an LLM provider (i.e. is type "llm_judge").
func (s *monitorManagerService) validateEvaluators(ctx context.Context, orgName string, evaluators []models.MonitorEvaluator) (hasLLMJudge bool, err error) {
	// Check for duplicate displayNames
	displayNames := make(map[string]int) // displayName -> first index
	for i, eval := range evaluators {
		if firstIdx, exists := displayNames[eval.DisplayName]; exists {
			return false, fmt.Errorf("evaluators[%d]: duplicate displayName %q (also used by evaluators[%d]): %w",
				i, eval.DisplayName, firstIdx, utils.ErrInvalidInput)
		}
		displayNames[eval.DisplayName] = i
	}

	for i := range evaluators {
		eval := &evaluators[i]
		prefix := fmt.Sprintf("evaluators[%d]", i)

		// Check evaluator exists in catalog or custom evaluators
		evaluatorResp, err := s.evaluatorService.GetEvaluator(ctx, orgName, eval.Identifier)
		if err != nil {
			if errors.Is(err, utils.ErrEvaluatorNotFound) {
				return false, fmt.Errorf("%s: evaluator %q not found in catalog: %w",
					prefix, eval.Identifier, utils.ErrInvalidInput)
			}
			return false, fmt.Errorf("%s: failed to look up evaluator %q: %w", prefix, eval.Identifier, err)
		}

		if evaluatorResp.Type == models.CustomEvaluatorTypeLLMJudge {
			hasLLMJudge = true
		}

		// Validate and apply defaults to config (including level)
		if err := validateAndApplyDefaults(i, eval.Identifier, &eval.Config, evaluatorResp.ConfigSchema); err != nil {
			return false, err
		}
	}
	return hasLLMJudge, nil
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

// ── LLM Proxy Lifecycle ───────────────────────────────────────────────────────

// monitorCompositeSecretLocation returns the OpenBao SecretLocation for the per-monitor
// composite LLM proxy credentials secret.  The path is deterministic from monitorID so
// the executor can compute it without any extra DB lookup.
func monitorCompositeSecretLocation(orgName string, monitorID uuid.UUID) secretmanagersvc.SecretLocation {
	return secretmanagersvc.SecretLocation{
		OrgName:    orgName,
		EntityName: "monitor-" + monitorID.String(),
		SecretKey:  "llm-proxy-configs",
	}
}

func (s *monitorManagerService) provisionLLMProxy(
	ctx context.Context,
	orgName string,
	monitor *models.Monitor,
	provRef models.MonitorLLMProviderRef,
	gateway *models.Gateway,
	projectUUID uuid.UUID,
) (*models.MonitorLLMMapping, ProxyRollbackState, string, error) {
	// Cap the full proxy name to 52 chars so that appending "-deployment" (11 chars)
	// never exceeds the Kubernetes 63-char name limit.
	rawProxyName := fmt.Sprintf("%s-%s-proxy", sanitizeForK8sName(monitor.Name), sanitizeForK8sName(provRef.ProviderName))
	proxyName := rawProxyName
	if len(proxyName) > 52 {
		proxyName = strings.TrimRight(rawProxyName[:52], "-")
	}

	provisioned, err := s.llmProvisioner.ProvisionProxy(ctx, ProvisionProxyParams{
		OrgName:        orgName,
		ProviderHandle: provRef.ProviderName,
		ProxyName:      proxyName,
		ProjectUUID:    projectUUID,
		Gateway:        gateway,
		Description:    fmt.Sprintf("LLM proxy for monitor %s", monitor.Name),
		SkipKVSecret:   true, // monitors store the key in their own composite secret
	})
	if err != nil {
		return nil, ProxyRollbackState{}, "", fmt.Errorf("failed to provision LLM proxy: %w", err)
	}

	s.logger.Info("Provisioned LLM proxy for monitor",
		"monitor", monitor.Name,
		"provider", provRef.ProviderName,
		"proxyHandle", provisioned.Proxy.Handle,
		"proxyURL", provisioned.ProxyURL,
	)

	// Return the API key to the caller so it can write the composite secret only after
	// the DB mapping row is persisted. Writing the secret before the row is committed
	// means a Create failure would corrupt the existing proxy's credentials on rollback.
	return &models.MonitorLLMMapping{
		MonitorID:    monitor.ID,
		LLMProxyUUID: provisioned.Proxy.UUID,
	}, provisioned.RollbackState, provisioned.ProxyAPIKey, nil
}

// cleanupLLMProxies tears down all LLM proxies associated with a monitor.
func (s *monitorManagerService) cleanupLLMProxies(ctx context.Context, orgName string, monitorID uuid.UUID) error {
	mappings, err := s.monitorLLMMappingRepo.ListByMonitorID(ctx, monitorID)
	if err != nil {
		return fmt.Errorf("failed to list monitor LLM mappings: %w", err)
	}

	for _, mapping := range mappings {
		if mapping.LLMProxy == nil {
			continue
		}
		// Monitors use org-scoped KV paths (empty ProxySecretContext).
		if err := s.llmProvisioner.CleanupProxy(ctx, mapping.LLMProxy, orgName, ProxySecretContext{}); err != nil {
			return err
		}
	}

	// Delete composite LLM proxy credentials secret (only exists if proxies were provisioned).
	if len(mappings) > 0 {
		compositeLoc := monitorCompositeSecretLocation(orgName, monitorID)
		if err := s.llmProvisioner.SecretClient().DeleteSecret(ctx, compositeLoc, ""); err != nil {
			return fmt.Errorf("failed to delete composite LLM proxy secret: %w", err)
		}
	}

	// Delete mapping rows.
	if err := s.monitorLLMMappingRepo.DeleteByMonitorID(ctx, s.db, monitorID); err != nil {
		return fmt.Errorf("failed to delete monitor LLM mappings: %w", err)
	}

	return nil
}

// buildMonitorLLMProviderInfo returns the LLM provider info for API responses (one provider per monitor).
func (s *monitorManagerService) buildMonitorLLMProviderInfo(ctx context.Context, monitorID uuid.UUID, orgName string) (*models.MonitorLLMProviderInfo, error) {
	mappings, err := s.monitorLLMMappingRepo.ListByMonitorID(ctx, monitorID)
	if err != nil {
		return nil, err
	}

	for _, mapping := range mappings {
		if mapping.LLMProxy == nil {
			continue
		}

		providerUUID := mapping.LLMProxy.ProviderUUID.String()
		provider, err := s.llmProvisioner.ProviderRepo().GetByUUID(providerUUID, orgName)
		if err != nil {
			s.logger.Warn("Failed to resolve provider for mapping", "providerUUID", providerUUID, "error", err)
			continue
		}

		if provider.Artifact == nil {
			s.logger.Warn("Provider has no artifact, skipping LLM provider info", "providerUUID", providerUUID)
			continue
		}

		return &models.MonitorLLMProviderInfo{
			ProviderName:   provider.Configuration.Handle,
			DisplayName:    provider.Configuration.Name,
			TemplateHandle: provider.TemplateHandle,
		}, nil
	}

	return nil, nil //nolint:nilnil // nil result means "no provider configured", not an error
}
