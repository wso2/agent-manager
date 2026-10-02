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
	"time"

	"github.com/google/uuid"
)

// Alert event types. Alerting reports failures only; successes are never sent.
const (
	AlertEventMonitorThresholdBreached = "monitor.threshold_breached"
	AlertEventMonitorRunFailed         = "monitor.run_failed"
	AlertEventTest                     = "alert.test"
)

// Alert severities carried in the event payload.
const (
	AlertSeverityCritical = "critical"
	AlertSeverityWarning  = "warning"
	AlertSeverityInfo     = "info"
)

// Alert delivery statuses.
const (
	AlertDeliveryStatusPending = "pending"
	AlertDeliveryStatusSent    = "sent"
	AlertDeliveryStatusDead    = "dead"
)

// Threshold operators. A threshold alerts when the score is below (lt) or at
// or below (lte) the configured value.
const (
	AlertOperatorLT  = "lt"
	AlertOperatorLTE = "lte"
)

// Defaults applied when a monitor alert config leaves a field unset.
const (
	DefaultAlertCooldownMinutes     = 60
	DefaultAlertConsecutiveBreaches = 1
)

// AlertEndpoint is the org-level HTTP endpoint that receives alerts.
type AlertEndpoint struct {
	OUID                   string     `gorm:"column:ou_id;primaryKey"`
	URL                    string     `gorm:"column:url;not null"`
	Enabled                bool       `gorm:"column:enabled;not null"`
	HeaderNames            []string   `gorm:"column:header_names;type:jsonb;serializer:json;not null"`
	HeadersEncrypted       []byte     `gorm:"column:headers_encrypted"`
	SigningSecretEncrypted []byte     `gorm:"column:signing_secret_encrypted;not null"`
	ConsecutiveFailures    int        `gorm:"column:consecutive_failures;not null"`
	LastSuccessAt          *time.Time `gorm:"column:last_success_at"`
	LastFailureAt          *time.Time `gorm:"column:last_failure_at"`
	CreatedBy              string     `gorm:"column:created_by;not null"`
	CreatedAt              time.Time  `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt              time.Time  `gorm:"column:updated_at;not null;default:NOW()"`
}

func (AlertEndpoint) TableName() string { return "alert_endpoints" }

// MonitorAlertThreshold is one per-evaluator score rule.
type MonitorAlertThreshold struct {
	Evaluator   string  `json:"evaluator"`
	Aggregation string  `json:"aggregation"`
	Operator    string  `json:"operator"`
	Value       float64 `json:"value"`
}

// Breached reports whether score violates the threshold.
func (t MonitorAlertThreshold) Breached(score float64) bool {
	if t.Operator == AlertOperatorLTE {
		return score <= t.Value
	}
	return score < t.Value
}

// MonitorAlertConfig holds a monitor's alert rules and the state used to
// suppress repeated alerts.
type MonitorAlertConfig struct {
	MonitorID              uuid.UUID               `gorm:"column:monitor_id;primaryKey;type:uuid"`
	Enabled                bool                    `gorm:"column:enabled;not null"`
	AlertOnRunFailure      bool                    `gorm:"column:alert_on_run_failure;not null"`
	Thresholds             []MonitorAlertThreshold `gorm:"column:thresholds;type:jsonb;serializer:json;not null"`
	CooldownMinutes        int                     `gorm:"column:cooldown_minutes;not null"`
	ConsecutiveBreaches    int                     `gorm:"column:consecutive_breaches;not null"`
	ConsecutiveBreachCount int                     `gorm:"column:consecutive_breach_count;not null"`
	LastAlertedAt          *time.Time              `gorm:"column:last_alerted_at"`
	CreatedAt              time.Time               `gorm:"column:created_at;not null;default:NOW()"`
	UpdatedAt              time.Time               `gorm:"column:updated_at;not null;default:NOW()"`
}

func (MonitorAlertConfig) TableName() string { return "monitor_alert_configs" }

// AlertDelivery is one queued or attempted POST of an alert event.
type AlertDelivery struct {
	ID               uuid.UUID  `gorm:"column:id;primaryKey;type:uuid;default:gen_random_uuid()"`
	OUID             string     `gorm:"column:ou_id;not null"`
	EventID          string     `gorm:"column:event_id;not null"`
	EventType        string     `gorm:"column:event_type;not null"`
	MonitorID        *uuid.UUID `gorm:"column:monitor_id;type:uuid"`
	MonitorName      string     `gorm:"column:monitor_name;not null"`
	Payload          string     `gorm:"column:payload;type:jsonb;not null"`
	Status           string     `gorm:"column:status;not null"`
	Attempts         int        `gorm:"column:attempts;not null"`
	NextAttemptAt    *time.Time `gorm:"column:next_attempt_at"`
	LastResponseCode *int       `gorm:"column:last_response_code"`
	LastError        string     `gorm:"column:last_error;not null"`
	DeliveredAt      *time.Time `gorm:"column:delivered_at"`
	CreatedAt        time.Time  `gorm:"column:created_at;not null;default:NOW()"`
}

func (AlertDelivery) TableName() string { return "alert_deliveries" }

// AlertEvent is the JSON body POSTed to an alert endpoint.
type AlertEvent struct {
	ID          string             `json:"id"`
	Type        string             `json:"type"`
	Version     string             `json:"version"`
	OccurredAt  time.Time          `json:"occurredAt"`
	Severity    string             `json:"severity"`
	OrgID       string             `json:"orgId"`
	Project     string             `json:"project,omitempty"`
	Agent       string             `json:"agent,omitempty"`
	Environment string             `json:"environment,omitempty"`
	Monitor     *AlertMonitorInfo  `json:"monitor,omitempty"`
	Breaches    []AlertScoreBreach `json:"breaches,omitempty"`
	Error       string             `json:"error,omitempty"`
	Message     string             `json:"message,omitempty"`
}

// AlertMonitorInfo identifies the monitor and run an alert is about.
type AlertMonitorInfo struct {
	Name        string     `json:"name"`
	DisplayName string     `json:"displayName,omitempty"`
	RunID       string     `json:"runId,omitempty"`
	TraceStart  *time.Time `json:"traceStart,omitempty"`
	TraceEnd    *time.Time `json:"traceEnd,omitempty"`
}

// AlertScoreBreach is one evaluator aggregation that crossed its threshold.
type AlertScoreBreach struct {
	Evaluator   string  `json:"evaluator"`
	Aggregation string  `json:"aggregation"`
	Value       float64 `json:"value"`
	Threshold   float64 `json:"threshold"`
	Operator    string  `json:"operator"`
	SampleCount int     `json:"sampleCount"`
}
