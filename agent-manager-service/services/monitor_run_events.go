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

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// MonitorRunObserver is the narrow surface the monitor scheduler uses to
// report run outcomes. Implementations must not fail the caller.
type MonitorRunObserver interface {
	// OnMonitorRunFinished is called once a run reaches a terminal status.
	OnMonitorRunFinished(ctx context.Context, monitor *models.Monitor, run *models.MonitorRun, status, errorMessage string)
	// OnMonitorTriggerFailed is called when a scheduled run could not be started.
	OnMonitorTriggerFailed(ctx context.Context, monitor *models.Monitor, cause error)
}

// NoopMonitorRunObserver discards run outcomes.
type NoopMonitorRunObserver struct{}

func (NoopMonitorRunObserver) OnMonitorRunFinished(context.Context, *models.Monitor, *models.MonitorRun, string, string) {
}

func (NoopMonitorRunObserver) OnMonitorTriggerFailed(context.Context, *models.Monitor, error) {}

// monitorRunEvents publishes monitor_run.succeeded and monitor_run.failed.
type monitorRunEvents struct {
	publisher events.Publisher
	scoreRepo repositories.ScoreRepository
	logger    *slog.Logger
	now       func() time.Time
}

// NewMonitorRunEvents creates the observer that publishes monitor run events.
func NewMonitorRunEvents(publisher events.Publisher, scoreRepo repositories.ScoreRepository, logger *slog.Logger) MonitorRunObserver {
	return &monitorRunEvents{publisher: publisher, scoreRepo: scoreRepo, logger: logger, now: time.Now}
}

func (m *monitorRunEvents) OnMonitorRunFinished(
	ctx context.Context, monitor *models.Monitor, run *models.MonitorRun, status, errorMessage string,
) {
	eventType := events.TypeMonitorRunSucceeded
	switch status {
	case models.RunStatusSuccess:
	case models.RunStatusFailed:
		eventType = events.TypeMonitorRunFailed
	default:
		return
	}

	runData := map[string]any{
		"id":         run.ID.String(),
		"name":       run.Name,
		"status":     status,
		"traceStart": run.TraceStart,
		"traceEnd":   run.TraceEnd,
	}
	if run.StartedAt != nil {
		runData["startedAt"] = run.StartedAt
	}
	if run.CompletedAt != nil {
		runData["completedAt"] = run.CompletedAt
	}
	if errorMessage != "" {
		runData["error"] = errorMessage
	}
	data := map[string]any{"monitor": monitorEventData(monitor), "run": runData}

	if status == models.RunStatusSuccess {
		evaluators, err := m.scoreRepo.GetEvaluatorsByMonitorAndRunID(monitor.ID, run.ID)
		if err != nil {
			m.logger.Error("Failed to load run scores for event", "monitor", monitor.Name, "runID", run.ID, "error", err)
		} else {
			scores := make([]map[string]any, 0, len(evaluators))
			for _, e := range evaluators {
				scores = append(scores, map[string]any{
					"evaluator":    e.EvaluatorName,
					"identifier":   e.Identifier,
					"level":        e.Level,
					"count":        e.Count,
					"skippedCount": e.SkippedCount,
					"aggregations": e.Aggregations,
				})
			}
			data["scores"] = scores
		}
	}

	// The ID is derived from the run, so a run synced twice publishes one
	// event: JetStream drops the duplicate.
	m.publish(ctx, monitor, "evt_monrun_"+run.ID.String(), eventType, data)
}

func (m *monitorRunEvents) OnMonitorTriggerFailed(ctx context.Context, monitor *models.Monitor, cause error) {
	reason := "failed to start the evaluation run"
	if cause != nil {
		reason = reason + ": " + cause.Error()
	}
	now := m.now()
	data := map[string]any{
		"monitor": monitorEventData(monitor),
		"run":     map[string]any{"status": models.RunStatusFailed, "error": reason},
	}
	m.publish(ctx, monitor, fmt.Sprintf("evt_monstart_%s_%d", monitor.ID, now.Unix()/60), events.TypeMonitorRunFailed, data)
}

func (m *monitorRunEvents) publish(ctx context.Context, monitor *models.Monitor, id, eventType string, data map[string]any) {
	e := events.New(id, eventType, events.ScopeAgent, monitor.OUID, monitor.ProjectName, monitor.AgentName, m.now())
	e.Environment = monitor.EnvironmentName
	e.Subject = monitor.Name
	e.ActorType = "system"
	e.Data = data
	m.publisher.Publish(ctx, e)
}

func monitorEventData(monitor *models.Monitor) map[string]any {
	return map[string]any{
		"id":          monitor.ID.String(),
		"name":        monitor.Name,
		"displayName": monitor.DisplayName,
		"type":        monitor.Type,
	}
}
