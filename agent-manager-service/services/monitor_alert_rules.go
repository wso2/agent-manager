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
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// notificationBodyLimit caps the text taken from a run's error message.
const notificationBodyLimit = 300

// monitorRunEvent is what the notification center reads from a
// monitor_run.succeeded or monitor_run.failed event.
type monitorRunEvent struct {
	EventID     string
	OrgID       string
	Time        time.Time
	Project     string
	Agent       string
	Environment string
	Failed      bool
	MonitorID   uuid.UUID
	Monitor     struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
	}
	Run struct {
		ID    string `json:"id"`
		Error string `json:"error"`
	}
	Scores []monitorRunScore
}

// monitorRunScore is one evaluator's aggregated scores at one level.
type monitorRunScore struct {
	Evaluator    string         `json:"evaluator"`
	Level        string         `json:"level"`
	Aggregations map[string]any `json:"aggregations"`
}

// parseMonitorRunEvent reads a monitor run event. ok is false for any other
// event, or one without the monitor it is about.
func parseMonitorRunEvent(e events.Event, raw []byte) (monitorRunEvent, bool) {
	name := e.Name()
	if name != events.TypeMonitorRunSucceeded && name != events.TypeMonitorRunFailed {
		return monitorRunEvent{}, false
	}
	var body struct {
		Data struct {
			Monitor struct {
				ID          string `json:"id"`
				Name        string `json:"name"`
				DisplayName string `json:"displayName"`
			} `json:"monitor"`
			Run struct {
				ID    string `json:"id"`
				Error string `json:"error"`
			} `json:"run"`
			Scores []monitorRunScore `json:"scores"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return monitorRunEvent{}, false
	}
	monitorID, err := uuid.Parse(body.Data.Monitor.ID)
	if err != nil {
		return monitorRunEvent{}, false
	}
	run := monitorRunEvent{
		EventID:     e.ID,
		OrgID:       e.OrgID,
		Time:        e.Time,
		Project:     e.Project,
		Agent:       e.Agent,
		Environment: e.Environment,
		Failed:      name == events.TypeMonitorRunFailed,
		MonitorID:   monitorID,
		Scores:      body.Data.Scores,
	}
	run.Monitor.Name = body.Data.Monitor.Name
	run.Monitor.DisplayName = body.Data.Monitor.DisplayName
	if run.Monitor.DisplayName == "" {
		run.Monitor.DisplayName = run.Monitor.Name
	}
	run.Run.ID = body.Data.Run.ID
	run.Run.Error = body.Data.Run.Error
	return run, true
}

// alertOutcome is what one run means for one watch.
type alertOutcome struct {
	Notifications []models.Notification
	// Evaluated is true when the run's scores were checked against the
	// thresholds; Breached is then the new set of breached threshold keys.
	Evaluated bool
	Breached  []string
}

// evaluateAlert decides which notifications a run sends to a watcher, and
// which of their thresholds it breaches.
//
// Thresholds notify on change only: when a score first breaks a threshold,
// and when it is back within it. A run with no score for a threshold (every
// trace skipped, say) leaves that threshold as it was.
func evaluateAlert(w *models.MonitorAlert, run monitorRunEvent) alertOutcome {
	var out alertOutcome
	notify := func(kind, title, body string) {
		out.Notifications = append(out.Notifications, models.Notification{
			OUID:    w.OUID,
			Kind:    kind,
			Title:   title,
			Body:    body,
			EventID: run.EventID,
			Target: models.NotificationTarget{
				ProjectName:        run.Project,
				AgentName:          run.Agent,
				EnvironmentName:    run.Environment,
				MonitorName:        run.Monitor.Name,
				MonitorDisplayName: run.Monitor.DisplayName,
				RunID:              run.Run.ID,
			},
		})
	}
	monitor := run.Monitor.DisplayName

	if run.Failed {
		if w.NotifyOnFailure {
			title := "Monitor run failed: " + monitor
			if run.Run.ID == "" {
				title = "Monitor run could not start: " + monitor
			}
			body := truncate(run.Run.Error, notificationBodyLimit)
			if body == "" {
				body = "The evaluation run failed."
			}
			notify(models.NotificationMonitorRunFailed, title, body)
		}
		return out
	}

	if w.NotifyOnSuccess {
		notify(models.NotificationMonitorRunSucceeded, "Monitor run completed: "+monitor, scoreSummary(run.Scores))
	}

	if len(w.Thresholds) == 0 || (w.LastEvaluatedAt != nil && !run.Time.After(*w.LastEvaluatedAt)) {
		return out
	}
	wasBreached := make(map[string]bool, len(w.Breached))
	for _, k := range w.Breached {
		wasBreached[k] = true
	}
	var breached, recovered []string
	out.Evaluated = true
	out.Breached = []string{}
	for _, t := range w.Thresholds {
		key := t.Key()
		score, ok := thresholdScore(t, run.Scores)
		if !ok {
			if wasBreached[key] {
				out.Breached = append(out.Breached, key)
			}
			continue
		}
		line := fmt.Sprintf("%s %s is %.2f", t.Evaluator, aggregationLabel(t.Aggregation), score)
		switch {
		case t.Breached(score):
			out.Breached = append(out.Breached, key)
			if !wasBreached[key] {
				breached = append(breached, fmt.Sprintf("%s, %s %.2f.", line, t.Operator, t.Value))
			}
		case wasBreached[key]:
			recovered = append(recovered, fmt.Sprintf("%s, no longer %s %.2f.", line, t.Operator, t.Value))
		}
	}
	if len(breached) > 0 {
		title := "Score threshold crossed: " + monitor
		if len(breached) > 1 {
			title = fmt.Sprintf("%d score thresholds crossed: %s", len(breached), monitor)
		}
		notify(models.NotificationScoreBreached, title, strings.Join(breached, "\n"))
	}
	if len(recovered) > 0 {
		title := "Score back within threshold: " + monitor
		if len(recovered) > 1 {
			title = fmt.Sprintf("%d scores back within thresholds: %s", len(recovered), monitor)
		}
		notify(models.NotificationScoreRecovered, title, strings.Join(recovered, "\n"))
	}
	return out
}

// thresholdScore returns the run's score for a threshold. An evaluator scored
// at several levels yields the value most in breach: the lowest for "below",
// the highest for "above".
func thresholdScore(t models.ScoreThreshold, scores []monitorRunScore) (float64, bool) {
	var best float64
	found := false
	for _, s := range scores {
		if s.Evaluator != t.Evaluator {
			continue
		}
		v, ok := s.Aggregations[t.Aggregation].(float64)
		if !ok {
			continue
		}
		if !found || (t.Operator == models.ThresholdAbove && v > best) || (t.Operator != models.ThresholdAbove && v < best) {
			best = v
		}
		found = true
	}
	return best, found
}

// scoreSummary describes a completed run's mean scores in one line.
func scoreSummary(scores []monitorRunScore) string {
	const shown = 3
	parts := make([]string, 0, shown)
	for _, s := range scores {
		v, ok := s.Aggregations[models.ThresholdAggregationMean].(float64)
		if !ok {
			continue
		}
		if len(parts) == shown {
			parts = append(parts, "…")
			break
		}
		parts = append(parts, fmt.Sprintf("%s %.2f", s.Evaluator, v))
	}
	if len(parts) == 0 {
		return "The run completed with no scores."
	}
	return "Mean scores: " + strings.Join(parts, ", ")
}

func aggregationLabel(aggregation string) string {
	return strings.ReplaceAll(aggregation, "_", " ")
}

func truncate(s string, limit int) string {
	r := []rune(strings.TrimSpace(s))
	if len(r) <= limit {
		return string(r)
	}
	return string(r[:limit-1]) + "…"
}
