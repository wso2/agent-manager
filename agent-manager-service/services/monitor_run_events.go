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
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// MonitorRunObserver is the narrow surface the monitor scheduler uses to
// prepare run outcomes before terminal persistence and publish them afterwards.
type MonitorRunObserver interface {
	// PrepareMonitorRunFinished loads committed data. On failure the scheduler
	// still persists the terminal status. Call the returned publisher only after persisting status.
	PrepareMonitorRunFinished(ctx context.Context, monitor *models.Monitor, run *models.MonitorRun, status, errorMessage string) (func(), error)
	// OnMonitorTriggerFailed is called when a scheduled run could not be started.
	OnMonitorTriggerFailed(ctx context.Context, monitor *models.Monitor, cause error)
}

// NoopMonitorRunObserver discards run outcomes.
type NoopMonitorRunObserver struct{}

func (NoopMonitorRunObserver) PrepareMonitorRunFinished(context.Context, *models.Monitor, *models.MonitorRun, string, string) (func(), error) {
	return func() {}, nil
}

func (NoopMonitorRunObserver) OnMonitorTriggerFailed(context.Context, *models.Monitor, error) {}

// monitorRunEvents publishes monitor_run.succeeded and monitor_run.failed.
type monitorRunEvents struct {
	publisher   events.Publisher
	scoreRepo   repositories.ScoreRepository
	monitorRepo repositories.MonitorRepository
	logger      *slog.Logger
	now         func() time.Time
}

// NewMonitorRunEvents creates the observer that publishes monitor run events.
func NewMonitorRunEvents(publisher events.Publisher, scoreRepo repositories.ScoreRepository, monitorRepo repositories.MonitorRepository, logger *slog.Logger) MonitorRunObserver {
	if _, disabled := publisher.(events.NoopPublisher); disabled || publisher == nil {
		return NoopMonitorRunObserver{}
	}
	return &monitorRunEvents{publisher: publisher, scoreRepo: scoreRepo, monitorRepo: monitorRepo, logger: logger, now: time.Now}
}

func (m *monitorRunEvents) PrepareMonitorRunFinished(
	ctx context.Context, monitor *models.Monitor, run *models.MonitorRun, status, errorMessage string,
) (func(), error) {
	eventType := events.TypeMonitorRunSucceeded
	switch status {
	case models.RunStatusSuccess:
	case models.RunStatusFailed:
		eventType = events.TypeMonitorRunFailed
	default:
		return func() {}, nil
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
			return nil, fmt.Errorf("load committed scores for monitor run event: %w", err)
		}
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

	// The ID is derived from the run, so a run synced twice publishes one
	// event: JetStream drops the duplicate.
	at := m.now()
	if run.CompletedAt != nil {
		at = *run.CompletedAt
	}
	return func() {
		if err := m.publish(ctx, monitor, "evt_monrun_"+run.ID.String(), eventType, data, at); err != nil {
			m.logger.Error("Monitor event retained for replay", "runId", run.ID, "error", err)
			return
		}
		if err := m.monitorRepo.MarkRunEventQueued(ctx, run.ID); err != nil {
			m.logger.Error("Failed to mark monitor event queued", "runId", run.ID, "error", err)
		}
	}, nil
}

// Reconcile replays terminal runs not yet accepted by the durable outbox.
// A terminal run is its own durable source; a crash between its status update
// and event persistence cannot lose the notification.
func (m *monitorRunEvents) Reconcile(ctx context.Context) error {
	runs, err := m.monitorRepo.ListUnqueuedTerminalRuns(ctx, 100)
	if err != nil {
		return fmt.Errorf("failed to list unpublished monitor runs: %w", err)
	}
	// A run that cannot be replayed yet stays unqueued for the next poll; it
	// must not hold back the runs after it.
	var failed []error
	for i := range runs {
		run := &runs[i]
		monitor, err := m.monitorRepo.GetMonitorByID(run.MonitorID)
		if err != nil {
			failed = append(failed, fmt.Errorf("failed to load monitor for replay of run %s: %w", run.ID, err))
			continue
		}
		reason := ""
		if run.ErrorMessage != nil {
			reason = *run.ErrorMessage
		}
		publish, err := m.PrepareMonitorRunFinished(ctx, monitor, run, run.Status, reason)
		if err != nil {
			failed = append(failed, fmt.Errorf("failed to replay monitor run %s: %w", run.ID, err))
			continue
		}
		publish()
	}
	return errors.Join(failed...)
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
	if err := m.publish(ctx, monitor, fmt.Sprintf("evt_monstart_%s_%d", monitor.ID, now.Unix()/60), events.TypeMonitorRunFailed, data, now); err != nil {
		m.logger.Error("Failed to persist monitor trigger failure event", "monitorId", monitor.ID, "error", err)
	}
}

func (m *monitorRunEvents) publish(ctx context.Context, monitor *models.Monitor, id, eventType string, data map[string]any, at time.Time) error {
	e := events.New(id, eventType, events.ScopeAgent, monitor.OUID, monitor.ProjectName, monitor.AgentName, at)
	e.Environment = monitor.EnvironmentName
	e.Subject = monitor.Name
	e.ActorType = "system"
	e.Data = data
	return m.publisher.Publish(ctx, e)
}

func monitorEventData(monitor *models.Monitor) map[string]any {
	return map[string]any{
		"id":          monitor.ID.String(),
		"name":        monitor.Name,
		"displayName": monitor.DisplayName,
		"type":        monitor.Type,
	}
}
