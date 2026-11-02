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
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/events"
)

// eventOutboxRow stores the complete CloudEvent until publication succeeds.
type eventOutboxRow struct {
	ID          string          `gorm:"primaryKey"`
	Body        json.RawMessage `gorm:"type:jsonb"`
	Lease       string
	AvailableAt time.Time
}

func (eventOutboxRow) TableName() string { return "event_outbox" }

type eventOutboxRepository struct{ db *gorm.DB }

func NewEventOutboxRepository(db *gorm.DB) events.Outbox {
	return &eventOutboxRepository{db: db}
}

func (r *eventOutboxRepository) Put(ctx context.Context, e events.Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	// Times come from the database clock, which Claim compares them with:
	// an application clock ahead of it would hide new rows.
	return r.db.WithContext(ctx).Exec(
		`INSERT INTO event_outbox (id, body, available_at) VALUES (?, ?, NOW()) ON CONFLICT DO NOTHING`,
		e.ID, body).Error
}

func (r *eventOutboxRepository) Claim(ctx context.Context) ([]events.PendingEvent, error) {
	var rows []eventOutboxRow
	// One atomic statement: no lock is held while publishing to NATS.
	err := r.db.WithContext(ctx).Raw(`
  UPDATE event_outbox SET lease = ?, available_at = NOW() + INTERVAL '2 minutes'
  WHERE id IN (SELECT id FROM event_outbox WHERE available_at <= NOW()
   ORDER BY available_at, id FOR UPDATE SKIP LOCKED LIMIT ?)
  RETURNING id, body, lease`, uuid.NewString(), events.OutboxBatchSize).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]events.PendingEvent, len(rows))
	for i, row := range rows {
		result[i] = events.PendingEvent{ID: row.ID, Body: row.Body, Lease: row.Lease}
	}
	return result, nil
}

func (r *eventOutboxRepository) Complete(ctx context.Context, e events.PendingEvent) error {
	return r.db.WithContext(ctx).Where("id = ? AND lease = ?", e.ID, e.Lease).Delete(&eventOutboxRow{}).Error
}

func (r *eventOutboxRepository) Retry(ctx context.Context, e events.PendingEvent) error {
	return r.db.WithContext(ctx).Model(&eventOutboxRow{}).Where("id = ? AND lease = ?", e.ID, e.Lease).
		Updates(map[string]any{"lease": "", "available_at": gorm.Expr("NOW() + INTERVAL '5 seconds'")}).Error
}
