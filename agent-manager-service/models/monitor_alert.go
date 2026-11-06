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

package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Monitor alert outcome kinds.
const (
	NotificationMonitorRunFailed    = "monitor_run_failed"
	NotificationMonitorRunSucceeded = "monitor_run_succeeded"
	NotificationScoreBreached       = "score_breached"
	NotificationScoreRecovered      = "score_recovered"
)

// Score threshold aggregations and operators.
const (
	ThresholdAggregationMean = "mean"
	ThresholdBelow           = "below"
	ThresholdAbove           = "above"
)

// MaxMonitorThresholds caps the thresholds of one monitor alert configuration.
const MaxMonitorThresholds = 20

// ScoreThreshold is one alert rule: the run's aggregated score of an
// evaluator is breached when it is below (or above) Value.
type ScoreThreshold struct {
	Evaluator   string  `json:"evaluator"`
	Aggregation string  `json:"aggregation"`
	Operator    string  `json:"operator"`
	Value       float64 `json:"value"`
}

// Key identifies the rule. The configuration remembers its breached rules by key, so a
// rule that is edited starts over as not breached.
func (t ScoreThreshold) Key() string {
	return fmt.Sprintf("%s|%s|%s|%g", t.Evaluator, t.Aggregation, t.Operator, t.Value)
}

// Breached reports whether score breaks the rule.
func (t ScoreThreshold) Breached(score float64) bool {
	if t.Operator == ThresholdAbove {
		return score > t.Value
	}
	return score < t.Value
}

// MonitorAlert stores shared alert settings for one monitor.
type MonitorAlert struct {
	ID              uuid.UUID        `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID            string           `gorm:"column:ou_id;not null"`
	MonitorID       uuid.UUID        `gorm:"column:monitor_id;type:uuid;not null"`
	DestinationIDs  []string         `gorm:"column:destination_ids;type:jsonb;serializer:json;not null"`
	NotifyOnFailure bool             `gorm:"column:notify_on_failure;not null"`
	NotifyOnSuccess bool             `gorm:"column:notify_on_success;not null"`
	Thresholds      []ScoreThreshold `gorm:"column:thresholds;type:jsonb;serializer:json;not null"`
	// Breached holds the keys of the thresholds the latest evaluated run
	// breached. A notification is sent when a threshold enters or leaves it.
	Breached []string `gorm:"column:breached;type:jsonb;serializer:json;not null"`
	// LastEvaluatedAt is the time of the latest run event the thresholds were
	// evaluated against. An older event arriving late is not evaluated.
	LastEvaluatedAt *time.Time `gorm:"column:last_evaluated_at"`
	CreatedAt       time.Time  `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt       time.Time  `gorm:"column:updated_at;not null;default:NOW()"`
}

func (MonitorAlert) TableName() string { return "monitor_alerts" }

// BreachedThresholds returns the thresholds the latest evaluated run breached.
func (w *MonitorAlert) BreachedThresholds() []ScoreThreshold {
	breached := make(map[string]bool, len(w.Breached))
	for _, k := range w.Breached {
		breached[k] = true
	}
	out := make([]ScoreThreshold, 0, len(w.Breached))
	for _, t := range w.Thresholds {
		if breached[t.Key()] {
			out = append(out, t)
		}
	}
	return out
}

// NotificationTarget identifies the monitor and run an alert describes.
type NotificationTarget struct {
	ProjectName        string `json:"projectName"`
	AgentName          string `json:"agentName"`
	EnvironmentName    string `json:"environmentName,omitempty"`
	MonitorName        string `json:"monitorName"`
	MonitorDisplayName string `json:"monitorDisplayName"`
	RunID              string `json:"runId,omitempty"`
}

// Notification is a transient alert outcome. It is delivered to organization
// destinations and is never stored in a console inbox.
type Notification struct {
	OUID    string
	Kind    string
	Title   string
	Body    string
	Target  NotificationTarget
	EventID string
}
