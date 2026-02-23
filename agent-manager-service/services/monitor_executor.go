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
	"time"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
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
	ocClient    client.OpenChoreoClient
	logger      *slog.Logger
	monitorRepo repositories.MonitorRepository
}

// NewMonitorExecutor creates a new monitor executor instance
func NewMonitorExecutor(
	ocClient client.OpenChoreoClient,
	logger *slog.Logger,
	monitorRepo repositories.MonitorRepository,
) MonitorExecutor {
	return &monitorExecutor{
		ocClient:    ocClient,
		logger:      logger,
		monitorRepo: monitorRepo,
	}
}

// ExecuteMonitorRun creates a WorkflowRun CR and a MonitorRun DB record
func (e *monitorExecutor) ExecuteMonitorRun(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
	// Generate unique WorkflowRun name
	workflowRunName := fmt.Sprintf("%s-%d", params.Monitor.Name, time.Now().Unix())

	// Pre-generate run ID so it can be included in the WorkflowRun CR for score publishing
	runID := uuid.New()

	evaluators := params.Evaluators
	if len(evaluators) == 0 {
		return nil, fmt.Errorf("evaluators must not be empty for monitor %s", params.Monitor.Name)
	}

	e.logger.Debug("Executing monitor run",
		"monitor", params.Monitor.Name,
		"workflowRunName", workflowRunName,
		"startTime", params.StartTime,
		"endTime", params.EndTime,
		"evaluators", evaluators)

	// Build WorkflowRun CR
	workflowRunCR, err := e.buildWorkflowRunCR(
		params.OrgName,
		params.Monitor,
		workflowRunName,
		runID,
		params.StartTime,
		params.EndTime,
		evaluators,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build WorkflowRun CR: %w", err)
	}

	// Create WorkflowRun via OpenChoreo API
	if err := e.ocClient.ApplyResource(ctx, workflowRunCR); err != nil {
		return nil, fmt.Errorf("failed to create WorkflowRun CR: %w", err)
	}

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
		// Best-effort cleanup of orphaned WorkflowRun CR
		deleteCR := map[string]interface{}{
			"apiVersion": workflowRunAPIVersion,
			"kind":       resourceKindWorkflowRun,
			"metadata": map[string]interface{}{
				"name":      workflowRunName,
				"namespace": params.OrgName,
			},
		}
		if delErr := e.ocClient.DeleteResource(ctx, deleteCR); delErr != nil {
			e.logger.Error("Failed to cleanup orphaned WorkflowRun CR", "error", delErr, "workflowRunName", workflowRunName)
		}
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

// buildWorkflowRunCR constructs the OpenChoreo WorkflowRun CR for a monitor
func (e *monitorExecutor) buildWorkflowRunCR(
	orgName string,
	monitor *models.Monitor,
	workflowRunName string,
	runID uuid.UUID,
	startTime, endTime time.Time,
	evaluators []models.MonitorEvaluator,
) (map[string]interface{}, error) {
	evaluatorsJSON, err := serializeEvaluators(evaluators)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"apiVersion": workflowRunAPIVersion,
		"kind":       resourceKindWorkflowRun,
		"metadata": map[string]interface{}{
			"name":      workflowRunName,
			"namespace": orgName,
			"labels": map[string]interface{}{
				monitorLabelResourceType: monitorResourceTypeValue,
				monitorLabelAgentName:    monitor.AgentName,
			},
			"annotations": map[string]interface{}{
				"amp.wso2.com/display-name": monitor.DisplayName,
			},
		},
		"spec": map[string]interface{}{
			"workflow": map[string]interface{}{
				"name": models.MonitorWorkflowName,
				"parameters": map[string]interface{}{
					"monitor": map[string]interface{}{
						"name":        monitor.Name,
						"displayName": monitor.DisplayName,
					},
					"agent": map[string]interface{}{
						"id": monitor.AgentID,
					},
					"environment": map[string]interface{}{
						"id": monitor.EnvironmentID,
					},
					"evaluation": map[string]interface{}{
						"evaluators":   evaluatorsJSON,
						"samplingRate": monitor.SamplingRate,
						"traceStart":   startTime.Format(time.RFC3339),
						"traceEnd":     endTime.Format(time.RFC3339),
					},
					"publishing": map[string]interface{}{
						"monitorId": monitor.ID.String(),
						"runId":     runID.String(),
					},
				},
			},
		},
	}, nil
}

// serializeEvaluators converts evaluators to a JSON string for the evaluation job workflow parameter.
func serializeEvaluators(evaluators []models.MonitorEvaluator) (string, error) {
	type evalJobEvaluator struct {
		Identifier  string                 `json:"identifier"`
		DisplayName string                 `json:"displayName"`
		Config      map[string]interface{} `json:"config"`
	}

	jobEvaluators := make([]evalJobEvaluator, len(evaluators))
	for i, eval := range evaluators {
		jobEvaluators[i] = evalJobEvaluator{
			Identifier:  eval.Identifier,
			DisplayName: eval.DisplayName,
			Config:      eval.Config,
		}
	}

	evaluatorsJSON, err := json.Marshal(jobEvaluators)
	if err != nil {
		return "", fmt.Errorf("failed to serialize evaluators: %w", err)
	}
	return string(evaluatorsJSON), nil
}
