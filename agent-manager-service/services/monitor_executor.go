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
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/catalog"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// MonitorExecutor handles workflow execution for monitors
// This is the shared component used by both MonitorManagerService and MonitorSchedulerService
type MonitorExecutor interface {
	// ExecuteMonitorRun creates a WorkflowRun CR and a MonitorRun DB record
	ExecuteMonitorRun(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error)

	// UpdateNextRunTime updates the next_run_time for a future monitor
	UpdateNextRunTime(ctx context.Context, monitorID uuid.UUID, nextRunTime time.Time) error
}

// ExecuteMonitorRunParams contains all inputs for executing a monitor run
type ExecuteMonitorRunParams struct {
	OrgName    string
	Monitor    *models.Monitor
	StartTime  time.Time
	EndTime    time.Time
	Evaluators []models.MonitorEvaluator // Snapshot of evaluators to use (for rerun cases, use original evaluators)
}

// ExecuteMonitorRunResult contains the outcome of a monitor run execution
type ExecuteMonitorRunResult struct {
	Run  *models.MonitorRun
	Name string // WorkflowRun CR name
}

type monitorExecutor struct {
	ocClient              client.OpenChoreoClient
	logger                *slog.Logger
	monitorRepo           repositories.MonitorRepository
	custEvalRepo          repositories.CustomEvaluatorRepository
	monitorLLMMappingRepo repositories.MonitorLLMMappingRepository
	gatewayRepo           repositories.GatewayRepository
}

// NewMonitorExecutor creates a new monitor executor instance
func NewMonitorExecutor(
	ocClient client.OpenChoreoClient,
	logger *slog.Logger,
	monitorRepo repositories.MonitorRepository,
	custEvalRepo repositories.CustomEvaluatorRepository,
	monitorLLMMappingRepo repositories.MonitorLLMMappingRepository,
	gatewayRepo repositories.GatewayRepository,
) MonitorExecutor {
	return &monitorExecutor{
		ocClient:              ocClient,
		logger:                logger,
		monitorRepo:           monitorRepo,
		custEvalRepo:          custEvalRepo,
		monitorLLMMappingRepo: monitorLLMMappingRepo,
		gatewayRepo:           gatewayRepo,
	}
}

// ExecuteMonitorRun creates a WorkflowRun and a MonitorRun DB record
func (e *monitorExecutor) ExecuteMonitorRun(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
	// Pre-generate run ID so it can be included in the WorkflowRun for score publishing
	runID := uuid.New()

	evaluators := params.Evaluators
	if len(evaluators) == 0 {
		return nil, fmt.Errorf("evaluators must not be empty for monitor %s", params.Monitor.Name)
	}

	e.logger.Debug("Executing monitor run",
		"monitor", params.Monitor.Name,
		"startTime", params.StartTime,
		"endTime", params.EndTime,
		"evaluators", evaluators)

	// Resolve LLM proxy credentials: secret KV path (for ExternalSecret) and proxy URL (plain param).
	llmProxySecretPath, llmApiBase, err := e.resolveLLMCredentials(ctx, params.Monitor)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve LLM credentials: %w", err)
	}

	// Build WorkflowRun request
	workflowRunReq, err := e.buildWorkflowRunRequest(
		params.Monitor,
		runID,
		params.StartTime,
		params.EndTime,
		evaluators,
		llmProxySecretPath,
		llmApiBase,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build WorkflowRun request: %w", err)
	}

	// Create WorkflowRun via OpenChoreo API
	workflowRunResp, err := e.ocClient.CreateWorkflowRun(ctx, params.OrgName, *workflowRunReq)
	if err != nil {
		return nil, fmt.Errorf("failed to create WorkflowRun: %w", err)
	}

	workflowRunName := workflowRunResp.Name

	// Create monitor_runs entry
	now := time.Now()
	run := &models.MonitorRun{
		ID:         runID,
		MonitorID:  params.Monitor.ID,
		Name:       workflowRunName,
		Evaluators: evaluators,
		TraceStart: params.StartTime,
		TraceEnd:   params.EndTime,
		StartedAt:  &now,
		Status:     models.RunStatusPending,
	}

	if err := e.monitorRepo.CreateMonitorRun(run); err != nil {
		e.logger.Error("Failed to create monitor_runs entry", "error", err, "workflowRunName", workflowRunName)
		// Note: No delete API available for workflow runs
		return nil, fmt.Errorf("failed to create monitor run entry: %w", err)
	}

	e.logger.Info("Monitor run executed successfully",
		"monitor", params.Monitor.Name,
		"runID", run.ID,
		"workflowRunName", workflowRunName)

	return &ExecuteMonitorRunResult{
		Run:  run,
		Name: workflowRunName,
	}, nil
}

// UpdateNextRunTime updates the next_run_time for a future monitor
func (e *monitorExecutor) UpdateNextRunTime(ctx context.Context, monitorID uuid.UUID, nextRunTime time.Time) error {
	if err := e.monitorRepo.UpdateNextRunTime(monitorID, &nextRunTime); err != nil {
		return fmt.Errorf("failed to update next_run_time: %w", err)
	}

	e.logger.Debug("Updated next_run_time", "monitorID", monitorID, "nextRunTime", nextRunTime)
	return nil
}

// resolveLLMCredentials returns the OpenBao KV path of the composite LLM proxy credentials
// secret and the gateway proxy URL. Returns empty strings if no proxy mapping exists.
// The proxy URL is derived at runtime (not stored) by joining LLMProxy.Configuration.Context
// with the gateway vhost — mirroring the env_agent_model_mapping pattern used by agents.
func (e *monitorExecutor) resolveLLMCredentials(ctx context.Context, monitor *models.Monitor) (secretPath, proxyURL string, err error) {
	mappings, err := e.monitorLLMMappingRepo.ListByMonitorID(ctx, monitor.ID)
	if err != nil {
		return "", "", fmt.Errorf("failed to list monitor LLM mappings: %w", err)
	}

	if len(mappings) == 0 {
		return "", "", nil
	}

	loc := monitorCompositeSecretLocation(monitor.OrgName, monitor.ID)
	kvPath, err := loc.KVPath()
	if err != nil {
		return "", "", fmt.Errorf("failed to compute composite secret path: %w", err)
	}

	resolvedURL, err := e.resolveProxyURL(ctx, monitor.EnvironmentID, mappings[0].LLMProxy)
	if err != nil {
		return "", "", fmt.Errorf("failed to resolve proxy URL: %w", err)
	}

	return kvPath, resolvedURL, nil
}

// resolveProxyURL derives the proxy base URL from the preloaded LLMProxy and the gateway
// associated with the given environment — same logic as agent_configuration_service.go.
func (e *monitorExecutor) resolveProxyURL(ctx context.Context, environmentID string, proxy *models.LLMProxy) (string, error) {
	if proxy == nil {
		return "", fmt.Errorf("LLM proxy not preloaded for mapping")
	}

	envMappings, err := e.gatewayRepo.GetEnvironmentMappingsByEnvironmentID(environmentID)
	if err != nil {
		return "", fmt.Errorf("failed to get gateway mappings for environment %s: %w", environmentID, err)
	}
	if len(envMappings) == 0 {
		return "", fmt.Errorf("no gateway found for environment %s", environmentID)
	}

	gateway, err := e.gatewayRepo.GetByUUID(envMappings[0].GatewayUUID.String())
	if err != nil {
		return "", fmt.Errorf("failed to get gateway %s: %w", envMappings[0].GatewayUUID, err)
	}

	return buildProxyURL(gateway.Vhost, proxy.Configuration.Context), nil
}

// buildWorkflowRunRequest constructs the workflow run request for a monitor.
func (e *monitorExecutor) buildWorkflowRunRequest(
	monitor *models.Monitor,
	runID uuid.UUID,
	startTime, endTime time.Time,
	evaluators []models.MonitorEvaluator,
	llmProxySecretPath string,
	llmApiBase string,
) (*client.CreateWorkflowRunRequest, error) {
	evaluatorsJSON, err := e.serializeEvaluators(monitor.OrgName, evaluators)
	if err != nil {
		return nil, err
	}

	// Generate DNS-1123 compliant WorkflowRun name: <sanitized-monitor-name>-<short-run-id>
	workflowRunName := buildWorkflowRunName(monitor.Name, runID)

	return &client.CreateWorkflowRunRequest{
		Name:         workflowRunName,
		WorkflowName: models.MonitorWorkflowName,
		Parameters: map[string]interface{}{
			"monitor": map[string]interface{}{
				"name":        monitor.Name,
				"displayName": monitor.DisplayName,
			},
			"organization": monitor.OrgName,
			"project":      monitor.ProjectName,
			"agent": map[string]interface{}{
				"id":   monitor.AgentID,
				"name": monitor.AgentName,
			},
			"environment": map[string]interface{}{
				"id":   monitor.EnvironmentID,
				"name": monitor.EnvironmentName,
			},
			"evaluation": map[string]interface{}{
				"evaluators":         evaluatorsJSON,
				"llmProxySecretPath": llmProxySecretPath,
				"llmApiBase":         llmApiBase,
				"samplingRate":       monitor.SamplingRate,
				"traceStart":         startTime.Format(time.RFC3339),
				"traceEnd":           endTime.Format(time.RFC3339),
			},
			"publishing": map[string]interface{}{
				"monitorId": monitor.ID.String(),
				"runId":     runID.String(),
			},
		},
	}, nil
}

// evalJobEvaluator is the JSON structure passed to the evaluation job for each evaluator.
type evalJobEvaluator struct {
	Identifier   string                        `json:"identifier"`
	DisplayName  string                        `json:"displayName"`
	Config       map[string]interface{}        `json:"config"`
	Type         string                        `json:"type,omitempty"`         // "code" or "llm_judge" for custom
	Level        string                        `json:"level,omitempty"`        // "trace", "agent", or "llm"
	Source       string                        `json:"source,omitempty"`       // Python code or prompt template
	ConfigSchema []models.EvaluatorConfigParam `json:"configSchema,omitempty"` // parameter schema for custom evaluators
}

// serializeEvaluators converts evaluators to a JSON string for the evaluation job workflow parameter.
// For custom evaluators, it resolves their full definitions from the DB.
func (e *monitorExecutor) serializeEvaluators(orgName string, evaluators []models.MonitorEvaluator) (string, error) {
	// Identify which evaluators are custom (not in the built-in catalog)
	var customIdentifiers []string
	for _, eval := range evaluators {
		if catalog.Get(eval.Identifier) == nil {
			customIdentifiers = append(customIdentifiers, eval.Identifier)
		}
	}

	// Batch-fetch custom evaluator definitions
	customMap := make(map[string]*models.CustomEvaluator)
	if len(customIdentifiers) > 0 {
		customs, err := e.custEvalRepo.GetByIdentifiers(orgName, customIdentifiers)
		if err != nil {
			return "", fmt.Errorf("failed to resolve custom evaluators: %w", err)
		}
		for i := range customs {
			customMap[customs[i].Identifier] = &customs[i]
		}
	}

	jobEvaluators := make([]evalJobEvaluator, len(evaluators))
	for i, eval := range evaluators {
		je := evalJobEvaluator{
			Identifier:  eval.Identifier,
			DisplayName: eval.DisplayName,
			Config:      eval.Config,
		}

		// Enrich custom evaluators with source code / prompt template.
		// For built-in evaluators, emit Type so the eval job can detect llm_judge evaluators.
		if ce, ok := customMap[eval.Identifier]; ok {
			je.Type = ce.Type
			je.Level = ce.Level
			je.Source = ce.Source
			je.ConfigSchema = ce.ConfigSchema
		} else if entry := catalog.Get(eval.Identifier); entry != nil {
			je.ConfigSchema = entry.ConfigSchema
			// llm_judge builtins: send type+level+source so the eval job routes them
			// through the template path (model prefix transform + _create_custom_llm_judge).
			// code builtins: type intentionally omitted — eval job uses builtin() factory.
			if entry.Type == "llm_judge" {
				if entry.Source == "" {
					return "", fmt.Errorf("builtin LLM-judge evaluator %q has no prompt template in catalog — re-run make gen-evaluators-dev", eval.Identifier)
				}
				je.Type = entry.Type
				je.Level = entry.Level
				je.Source = entry.Source
			}
		} else {
			// Identifier was not in the built-in catalog and was not resolved from the DB.
			// This means the custom evaluator was deleted after the monitor was created.
			return "", fmt.Errorf("custom evaluator %q not found — it may have been deleted", eval.Identifier)
		}

		jobEvaluators[i] = je
	}

	evaluatorsJSON, err := json.Marshal(jobEvaluators)
	if err != nil {
		return "", fmt.Errorf("failed to serialize evaluators: %w", err)
	}
	return string(evaluatorsJSON), nil
}

var nonDNS1123 = regexp.MustCompile(`[^a-z0-9-]+`)

func buildWorkflowRunName(monitorName string, runID uuid.UUID) string {
	const suffixLen = 8
	const maxNameLen = 63

	base := strings.ToLower(monitorName)
	base = nonDNS1123.ReplaceAllString(base, "-")
	base = strings.Trim(base, "-")

	maxBaseLen := maxNameLen - 1 - suffixLen // "-" + suffix
	if len(base) > maxBaseLen {
		base = strings.Trim(base[:maxBaseLen], "-")
	}
	if base == "" {
		base = "monitor"
	}

	return fmt.Sprintf("%s-%s", base, runID.String()[:suffixLen])
}
