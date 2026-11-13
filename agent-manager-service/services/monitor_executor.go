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
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/utils"
	"github.com/wso2/agent-manager/agent-manager-service/workloadpolicy"

	"github.com/google/uuid"

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
	OUID       string
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
	logger                *slog.Logger
	monitorRepo           repositories.MonitorRepository
	custEvalRepo          repositories.CustomEvaluatorRepository
	credRepo              repositories.OrgPublisherCredentialRepository
	provisioner           PublisherCredentialProvisioner
	monitorLLMMappingRepo repositories.MonitorLLMMappingRepository
	gatewayRepo           repositories.GatewayRepository
	llmProviderRepo       repositories.LLMProviderRepository
	deploymentRepo        repositories.DeploymentRepository
}

// NewMonitorExecutor creates a new monitor executor instance
func NewMonitorExecutor(
	logger *slog.Logger,
	monitorRepo repositories.MonitorRepository,
	custEvalRepo repositories.CustomEvaluatorRepository,
	credRepo repositories.OrgPublisherCredentialRepository,
	provisioner PublisherCredentialProvisioner,
	monitorLLMMappingRepo repositories.MonitorLLMMappingRepository,
	gatewayRepo repositories.GatewayRepository,
	llmProviderRepo repositories.LLMProviderRepository,
	deploymentRepo repositories.DeploymentRepository,
) MonitorExecutor {
	return &monitorExecutor{
		logger:                logger,
		monitorRepo:           monitorRepo,
		custEvalRepo:          custEvalRepo,
		credRepo:              credRepo,
		provisioner:           provisioner,
		monitorLLMMappingRepo: monitorLLMMappingRepo,
		gatewayRepo:           gatewayRepo,
		llmProviderRepo:       llmProviderRepo,
		deploymentRepo:        deploymentRepo,
	}
}

// ExecuteMonitorRun creates a WorkflowRun and a MonitorRun DB record
func (e *monitorExecutor) ExecuteMonitorRun(ctx context.Context, params ExecuteMonitorRunParams) (*ExecuteMonitorRunResult, error) {
	if err := workloadpolicy.Check(ctx, params.OUID); err != nil {
		return nil, err
	}
	if len(params.Evaluators) == 0 {
		return nil, fmt.Errorf("evaluators must not be empty for monitor %s", params.Monitor.Name)
	}
	// Monitor runs are evaluation workflows, and no workflow runtime is available.
	return nil, fmt.Errorf("%w: monitor runs need an evaluation workflow runtime", utils.ErrNotImplemented)
}

// UpdateNextRunTime updates the next_run_time for a future monitor
func (e *monitorExecutor) UpdateNextRunTime(ctx context.Context, monitorID uuid.UUID, nextRunTime time.Time) error {
	if err := e.monitorRepo.UpdateNextRunTime(monitorID, &nextRunTime); err != nil {
		return fmt.Errorf("failed to update next_run_time: %w", err)
	}

	e.logger.Debug("Updated next_run_time", "monitorID", monitorID, "nextRunTime", nextRunTime)
	return nil
}
