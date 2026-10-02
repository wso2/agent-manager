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

package repositories

import (
	"context"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// AlertRepository persists org alert endpoints, per-monitor alert rules and the
// outbox of alert deliveries.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/alert_repository_mock.go . AlertRepository:AlertRepositoryMock
type AlertRepository interface {
	// GetEndpoint returns the org's endpoint, or gorm.ErrRecordNotFound.
	GetEndpoint(ctx context.Context, ouID string) (*models.AlertEndpoint, error)
	// UpsertEndpoint creates or replaces the org's endpoint.
	UpsertEndpoint(ctx context.Context, endpoint *models.AlertEndpoint) error
	// DeleteEndpoint removes the org's endpoint and reports whether one existed.
	DeleteEndpoint(ctx context.Context, ouID string) (bool, error)
	// RecordEndpointOutcome updates the endpoint's health counters after a
	// delivery reached a final outcome.
	RecordEndpointOutcome(ctx context.Context, ouID string, success bool, at time.Time) error

	// GetMonitorAlertConfig returns a monitor's alert config, or gorm.ErrRecordNotFound.
	GetMonitorAlertConfig(ctx context.Context, monitorID uuid.UUID) (*models.MonitorAlertConfig, error)
	// UpsertMonitorAlertConfig replaces a monitor's alert rules and resets its
	// breach counter, since the old count was measured against the old rules.
	UpsertMonitorAlertConfig(ctx context.Context, cfg *models.MonitorAlertConfig) error
	// UpdateMonitorAlertState records the breach counter and the last alert time.
	UpdateMonitorAlertState(ctx context.Context, monitorID uuid.UUID, breachCount int, lastAlertedAt *time.Time) error

	// EnqueueDelivery queues a delivery. A delivery whose event ID is already
	// queued is ignored and false is returned, so re-evaluating a run is safe.
	EnqueueDelivery(ctx context.Context, delivery *models.AlertDelivery) (bool, error)
	// ClaimDueDeliveries returns up to limit pending deliveries whose next
	// attempt time has arrived, pushing each one's next attempt out by lease so
	// no other replica claims it mid-attempt.
	ClaimDueDeliveries(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]models.AlertDelivery, error)
	// MarkDeliverySent records a successful attempt.
	MarkDeliverySent(ctx context.Context, id uuid.UUID, attempts int, responseCode int, at time.Time) error
	// MarkDeliveryRetry records a failed attempt and schedules the next one.
	MarkDeliveryRetry(ctx context.Context, id uuid.UUID, attempts int, responseCode *int, lastErr string, nextAttemptAt time.Time) error
	// MarkDeliveryDead records a failed attempt and stops retrying.
	MarkDeliveryDead(ctx context.Context, id uuid.UUID, attempts int, responseCode *int, lastErr string) error
	// ListDeliveries returns the org's most recent deliveries, newest first.
	ListDeliveries(ctx context.Context, ouID string, limit int) ([]models.AlertDelivery, error)
}

type alertRepository struct {
	db *gorm.DB
}

// NewAlertRepository creates an AlertRepository.
func NewAlertRepository(db *gorm.DB) AlertRepository {
	return &alertRepository{db: db}
}

func (r *alertRepository) GetEndpoint(ctx context.Context, ouID string) (*models.AlertEndpoint, error) {
	var endpoint models.AlertEndpoint
	if err := r.db.WithContext(ctx).Where("ou_id = ?", ouID).First(&endpoint).Error; err != nil {
		return nil, err
	}
	return &endpoint, nil
}

func (r *alertRepository) UpsertEndpoint(ctx context.Context, endpoint *models.AlertEndpoint) error {
	endpoint.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "ou_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"url", "enabled", "header_names", "headers_encrypted",
			"signing_secret_encrypted", "updated_at",
		}),
	}).Create(endpoint).Error
}

func (r *alertRepository) DeleteEndpoint(ctx context.Context, ouID string) (bool, error) {
	result := r.db.WithContext(ctx).Where("ou_id = ?", ouID).Delete(&models.AlertEndpoint{})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *alertRepository) RecordEndpointOutcome(ctx context.Context, ouID string, success bool, at time.Time) error {
	updates := map[string]interface{}{
		"consecutive_failures": gorm.Expr("consecutive_failures + 1"),
		"last_failure_at":      at,
	}
	if success {
		updates = map[string]interface{}{
			"consecutive_failures": 0,
			"last_success_at":      at,
		}
	}
	return r.db.WithContext(ctx).Model(&models.AlertEndpoint{}).
		Where("ou_id = ?", ouID).Updates(updates).Error
}

func (r *alertRepository) GetMonitorAlertConfig(ctx context.Context, monitorID uuid.UUID) (*models.MonitorAlertConfig, error) {
	var cfg models.MonitorAlertConfig
	if err := r.db.WithContext(ctx).Where("monitor_id = ?", monitorID).First(&cfg).Error; err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (r *alertRepository) UpsertMonitorAlertConfig(ctx context.Context, cfg *models.MonitorAlertConfig) error {
	cfg.ConsecutiveBreachCount = 0
	cfg.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "monitor_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"enabled", "alert_on_run_failure", "thresholds", "cooldown_minutes",
			"consecutive_breaches", "consecutive_breach_count", "updated_at",
		}),
	}).Create(cfg).Error
}

func (r *alertRepository) UpdateMonitorAlertState(ctx context.Context, monitorID uuid.UUID, breachCount int, lastAlertedAt *time.Time) error {
	updates := map[string]interface{}{"consecutive_breach_count": breachCount}
	if lastAlertedAt != nil {
		updates["last_alerted_at"] = *lastAlertedAt
	}
	return r.db.WithContext(ctx).Model(&models.MonitorAlertConfig{}).
		Where("monitor_id = ?", monitorID).Updates(updates).Error
}

func (r *alertRepository) EnqueueDelivery(ctx context.Context, delivery *models.AlertDelivery) (bool, error) {
	result := r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "event_id"}},
		DoNothing: true,
	}).Create(delivery)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

func (r *alertRepository) ClaimDueDeliveries(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]models.AlertDelivery, error) {
	var claimed []models.AlertDelivery
	err := r.db.WithContext(ctx).Raw(
		`
		UPDATE alert_deliveries SET next_attempt_at = ?
		WHERE id IN (
			SELECT id FROM alert_deliveries
			WHERE status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
			ORDER BY next_attempt_at ASC NULLS FIRST, created_at ASC
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *`,
		now.Add(lease), models.AlertDeliveryStatusPending, now, limit,
	).Scan(&claimed).Error
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (r *alertRepository) MarkDeliverySent(ctx context.Context, id uuid.UUID, attempts int, responseCode int, at time.Time) error {
	return r.updateDelivery(ctx, id, map[string]interface{}{
		"status":             models.AlertDeliveryStatusSent,
		"attempts":           attempts,
		"last_response_code": responseCode,
		"last_error":         "",
		"next_attempt_at":    nil,
		"delivered_at":       at,
	})
}

func (r *alertRepository) MarkDeliveryRetry(ctx context.Context, id uuid.UUID, attempts int, responseCode *int, lastErr string, nextAttemptAt time.Time) error {
	return r.updateDelivery(ctx, id, map[string]interface{}{
		"attempts":           attempts,
		"last_response_code": responseCode,
		"last_error":         lastErr,
		"next_attempt_at":    nextAttemptAt,
	})
}

func (r *alertRepository) MarkDeliveryDead(ctx context.Context, id uuid.UUID, attempts int, responseCode *int, lastErr string) error {
	return r.updateDelivery(ctx, id, map[string]interface{}{
		"status":             models.AlertDeliveryStatusDead,
		"attempts":           attempts,
		"last_response_code": responseCode,
		"last_error":         lastErr,
		"next_attempt_at":    nil,
	})
}

func (r *alertRepository) updateDelivery(ctx context.Context, id uuid.UUID, updates map[string]interface{}) error {
	return r.db.WithContext(ctx).Model(&models.AlertDelivery{}).Where("id = ?", id).Updates(updates).Error
}

func (r *alertRepository) ListDeliveries(ctx context.Context, ouID string, limit int) ([]models.AlertDelivery, error) {
	var deliveries []models.AlertDelivery
	err := r.db.WithContext(ctx).
		Where("ou_id = ?", ouID).
		Order("created_at DESC").
		Limit(limit).
		Find(&deliveries).Error
	if err != nil {
		return nil, err
	}
	return deliveries, nil
}
