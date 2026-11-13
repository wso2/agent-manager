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

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/events"
)

// eventOutboxRow stores the complete CloudEvent until publication succeeds. A row is claimed by writing a lease
// token to claimed_by; claimed_until is when an unfinished claim may be taken again.
type eventOutboxRow struct {
	ID           uuid.UUID       `gorm:"column:id;primaryKey"`
	OrgID        string          `gorm:"column:org_id"`
	Body         json.RawMessage `gorm:"column:body"`
	AvailableAt  time.Time       `gorm:"column:available_at"`
	ClaimedBy    *string         `gorm:"column:claimed_by"`
	ClaimedUntil *time.Time      `gorm:"column:claimed_until"`
}

func (eventOutboxRow) TableName() string { return "event_outbox" }

// Outbox leases: how long a claimed batch is hidden, and how long a failed publication waits.
const (
	outboxClaimLease = 2 * time.Minute
	outboxRetryDelay = 5 * time.Second
)

type eventOutboxRepository struct{ db *gorm.DB }

func NewEventOutboxRepository(db *gorm.DB) events.Outbox {
	return &eventOutboxRepository{db: db}
}

// outboxID is the row id of an event: its own id when that is a UUID, else one derived from it.
func outboxID(eventID string) uuid.UUID {
	if id, err := uuid.Parse(eventID); err == nil {
		return id
	}
	return uuid.NewSHA1(uuid.NameSpaceOID, []byte(eventID))
}

func (r *eventOutboxRepository) Put(ctx context.Context, e events.Event) error {
	body, err := json.Marshal(e)
	if err != nil {
		return err
	}
	row := eventOutboxRow{ID: outboxID(e.ID), OrgID: e.OrgID, Body: body, AvailableAt: dbx.Now()}
	_, err = dbx.InsertIfAbsent(r.db.WithContext(ctx), &row)
	return err
}

// Claim takes up to a batch of available rows. Each row is claimed with its own conditional update, so of two
// concurrent publishers only the first finds it still available; no lock is held while publishing.
func (r *eventOutboxRepository) Claim(ctx context.Context) ([]events.PendingEvent, error) {
	now := dbx.Now()
	lease := uuid.NewString()
	db := r.db.WithContext(ctx)
	var due []eventOutboxRow
	if err := db.Where("available_at <= ?", now).Order("available_at, id").
		Limit(events.OutboxBatchSize).Find(&due).Error; err != nil {
		return nil, err
	}
	result := make([]events.PendingEvent, 0, len(due))
	for _, row := range due {
		res := db.Model(&eventOutboxRow{}).Where("id = ? AND available_at <= ?", row.ID, now).
			Updates(map[string]any{"claimed_by": lease, "claimed_until": now.Add(outboxClaimLease), "available_at": now.Add(outboxClaimLease)})
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected == 1 {
			result = append(result, events.PendingEvent{ID: row.ID.String(), Body: row.Body, Lease: lease})
		}
	}
	return result, nil
}

func (r *eventOutboxRepository) Complete(ctx context.Context, e events.PendingEvent) error {
	return r.db.WithContext(ctx).Where("id = ? AND claimed_by = ?", e.ID, e.Lease).Delete(&eventOutboxRow{}).Error
}

func (r *eventOutboxRepository) Retry(ctx context.Context, e events.PendingEvent) error {
	return r.db.WithContext(ctx).Model(&eventOutboxRow{}).Where("id = ? AND claimed_by = ?", e.ID, e.Lease).
		Updates(map[string]any{"claimed_by": nil, "claimed_until": nil, "available_at": dbx.Now().Add(outboxRetryDelay)}).Error
}
