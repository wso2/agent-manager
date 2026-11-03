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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

func runWithScores(scores ...monitorRunScore) monitorRunEvent {
	run := monitorRunEvent{EventID: "evt_1", OrgID: "ou-1", Time: testRunTime, Project: "proj", Agent: "agent", MonitorID: uuid.New()}
	run.Monitor.Name, run.Monitor.DisplayName = "quality", "Quality"
	run.Run.ID = "run-1"
	run.Scores = scores
	return run
}

func score(evaluator, level string, aggregations map[string]any) monitorRunScore {
	return monitorRunScore{Evaluator: evaluator, Level: level, Aggregations: aggregations}
}

func TestEvaluateAlert_RunOutcomes(t *testing.T) {
	failed := runWithScores()
	failed.Failed = true
	failed.Run.Error = "workflow crashed"

	notStarted := runWithScores()
	notStarted.Failed = true
	notStarted.Run.ID = ""

	tests := []struct {
		name      string
		alert     models.MonitorAlert
		run       monitorRunEvent
		wantKinds []string
		wantTitle string
	}{
		{
			name:      "failed run notifies a failure alert",
			alert:     models.MonitorAlert{NotifyOnFailure: true},
			run:       failed,
			wantKinds: []string{models.NotificationMonitorRunFailed},
			wantTitle: "Monitor run failed: Quality",
		},
		{
			name:      "a run that could not start says so",
			alert:     models.MonitorAlert{NotifyOnFailure: true},
			run:       notStarted,
			wantKinds: []string{models.NotificationMonitorRunFailed},
			wantTitle: "Monitor run could not start: Quality",
		},
		{
			name:  "failed run is silent when failures are off",
			alert: models.MonitorAlert{NotifyOnSuccess: true},
			run:   failed,
		},
		{
			name:      "completed run notifies a success alert",
			alert:     models.MonitorAlert{NotifyOnSuccess: true},
			run:       runWithScores(score("Helpfulness", "trace", map[string]any{"mean": 0.9})),
			wantKinds: []string{models.NotificationMonitorRunSucceeded},
			wantTitle: "Monitor run completed: Quality",
		},
		{
			name:  "completed run is silent by default",
			alert: models.MonitorAlert{NotifyOnFailure: true},
			run:   runWithScores(score("Helpfulness", "trace", map[string]any{"mean": 0.9})),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := evaluateAlert(&tt.alert, tt.run)
			kinds := []string{}
			for _, n := range out.Notifications {
				kinds = append(kinds, n.Kind)
			}
			if tt.wantKinds == nil {
				assert.Empty(t, kinds)
				return
			}
			assert.Equal(t, tt.wantKinds, kinds)
			assert.Equal(t, tt.wantTitle, out.Notifications[0].Title)
		})
	}
}

func TestEvaluateAlert_ThresholdsNotifyOnChange(t *testing.T) {
	below := models.ScoreThreshold{Evaluator: "Helpfulness", Aggregation: "mean", Operator: models.ThresholdBelow, Value: 0.7}
	alert := models.MonitorAlert{Thresholds: []models.ScoreThreshold{below}, Breached: []string{}}

	low := runWithScores(score("Helpfulness", "trace", map[string]any{"mean": 0.6}))
	out := evaluateAlert(&alert, low)
	require.True(t, out.Evaluated)
	require.Len(t, out.Notifications, 1)
	assert.Equal(t, models.NotificationScoreBreached, out.Notifications[0].Kind)
	assert.Equal(t, "Score threshold crossed: Quality", out.Notifications[0].Title)
	assert.Equal(t, "Helpfulness mean is 0.60, below 0.70.", out.Notifications[0].Body)
	assert.Equal(t, []string{below.Key()}, out.Breached)

	// Still below on the next run: no new notification.
	alert.Breached = out.Breached
	stillLow := low
	stillLow.Time = testRunTime.Add(time.Hour)
	out = evaluateAlert(&alert, stillLow)
	assert.Empty(t, out.Notifications)
	assert.Equal(t, []string{below.Key()}, out.Breached)

	// Back above: one recovery.
	recovered := runWithScores(score("Helpfulness", "trace", map[string]any{"mean": 0.8}))
	recovered.Time = testRunTime.Add(2 * time.Hour)
	out = evaluateAlert(&alert, recovered)
	require.Len(t, out.Notifications, 1)
	assert.Equal(t, models.NotificationScoreRecovered, out.Notifications[0].Kind)
	assert.Empty(t, out.Breached)
}

func TestEvaluateAlert_ThresholdEdgeCases(t *testing.T) {
	below := models.ScoreThreshold{Evaluator: "Helpfulness", Aggregation: "mean", Operator: models.ThresholdBelow, Value: 0.7}
	above := models.ScoreThreshold{Evaluator: "Toxicity", Aggregation: "pass_rate", Operator: models.ThresholdAbove, Value: 0.2}

	t.Run("a missing score keeps the previous state", func(t *testing.T) {
		alert := models.MonitorAlert{Thresholds: []models.ScoreThreshold{below}, Breached: []string{below.Key()}}
		out := evaluateAlert(&alert, runWithScores(score("Other", "trace", map[string]any{"mean": 0.1})))
		assert.Empty(t, out.Notifications)
		assert.Equal(t, []string{below.Key()}, out.Breached)
	})

	t.Run("several levels use the value most in breach", func(t *testing.T) {
		alert := models.MonitorAlert{Thresholds: []models.ScoreThreshold{below, above}}
		out := evaluateAlert(&alert, runWithScores(
			score("Helpfulness", "trace", map[string]any{"mean": 0.9}),
			score("Helpfulness", "llm", map[string]any{"mean": 0.5}),
			score("Toxicity", "trace", map[string]any{"pass_rate": 0.1}),
			score("Toxicity", "agent", map[string]any{"pass_rate": 0.3}),
		))
		require.Len(t, out.Notifications, 1)
		assert.Equal(t, "2 score thresholds crossed: Quality", out.Notifications[0].Title)
		assert.ElementsMatch(t, []string{below.Key(), above.Key()}, out.Breached)
	})

	t.Run("an older run arriving late is not evaluated", func(t *testing.T) {
		later := testRunTime.Add(time.Hour)
		alert := models.MonitorAlert{Thresholds: []models.ScoreThreshold{below}, LastEvaluatedAt: &later}
		out := evaluateAlert(&alert, runWithScores(score("Helpfulness", "trace", map[string]any{"mean": 0.1})))
		assert.False(t, out.Evaluated)
		assert.Empty(t, out.Notifications)
	})

	t.Run("an edited threshold starts over", func(t *testing.T) {
		edited := below
		edited.Value = 0.5
		alert := models.MonitorAlert{Thresholds: []models.ScoreThreshold{edited}, Breached: []string{below.Key()}}
		out := evaluateAlert(&alert, runWithScores(score("Helpfulness", "trace", map[string]any{"mean": 0.6})))
		assert.Empty(t, out.Notifications, "the old threshold's breach does not become a recovery")
		assert.Empty(t, out.Breached)
	})
}
