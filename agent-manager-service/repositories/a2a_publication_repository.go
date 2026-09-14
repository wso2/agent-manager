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

// A2APublicationRepository is the queue of outstanding A2A gateway publications.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/a2a_publication_repository_mock.go . A2APublicationRepository:A2APublicationRepositoryMock
type A2APublicationRepository interface {
	// Enqueue records that an agent-environment pair needs publishing, resetting
	// any existing row for that pair to pending with a fresh attempt budget. A
	// redeploy must re-publish, and a pair that previously exhausted its budget
	// must get another chance.
	Enqueue(ctx context.Context, pub *models.A2APublication) error

	// FindDue returns pending rows whose next attempt time has arrived, oldest
	// first, capped at limit.
	FindDue(ctx context.Context, now time.Time, limit int) ([]models.A2APublication, error)

	MarkPublished(ctx context.Context, id uuid.UUID) error

	// MarkAttemptFailed records a retryable failure and schedules the next try.
	MarkAttemptFailed(ctx context.Context, id uuid.UUID, lastErr string, nextAttemptAt time.Time) error

	// MarkFailed ends the retry cycle. The row is kept as the record of an agent
	// that never reached its gateway.
	MarkFailed(ctx context.Context, id uuid.UUID, lastErr string) error

	// DeleteForAgent removes every environment's row for a deleted agent.
	DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error
}

type a2aPublicationRepository struct {
	db *gorm.DB
}

// NewA2APublicationRepository creates an A2APublicationRepository.
func NewA2APublicationRepository(db *gorm.DB) A2APublicationRepository {
	return &a2aPublicationRepository{db: db}
}

func (r *a2aPublicationRepository) Enqueue(ctx context.Context, pub *models.A2APublication) error {
	now := time.Now()
	pub.Status = models.A2APublicationStatusPending
	pub.AttemptCount = 0
	pub.LastError = ""
	pub.NextAttemptAt = &now
	pub.UpdatedAt = now

	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "ou_id"},
			{Name: "project_name"},
			{Name: "agent_name"},
			{Name: "environment_name"},
		},
		DoUpdates: clause.AssignmentColumns([]string{
			"environment_uuid", "artifact_uuid", "status",
			"attempt_count", "last_error", "next_attempt_at", "updated_at",
		}),
	}).Create(pub).Error
}

func (r *a2aPublicationRepository) FindDue(ctx context.Context, now time.Time, limit int) ([]models.A2APublication, error) {
	var due []models.A2APublication
	err := r.db.WithContext(ctx).
		Where("status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)",
			models.A2APublicationStatusPending, now).
		Order("next_attempt_at ASC, created_at ASC").
		Limit(limit).
		Find(&due).Error
	if err != nil {
		return nil, err
	}
	return due, nil
}

func (r *a2aPublicationRepository) MarkPublished(ctx context.Context, id uuid.UUID) error {
	return r.db.WithContext(ctx).Model(&models.A2APublication{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          models.A2APublicationStatusPublished,
			"last_error":      "",
			"next_attempt_at": nil,
			"updated_at":      time.Now(),
		}).Error
}

func (r *a2aPublicationRepository) MarkAttemptFailed(ctx context.Context, id uuid.UUID, lastErr string, nextAttemptAt time.Time) error {
	return r.db.WithContext(ctx).Model(&models.A2APublication{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"attempt_count":   gorm.Expr("attempt_count + 1"),
			"last_error":      lastErr,
			"next_attempt_at": nextAttemptAt,
			"updated_at":      time.Now(),
		}).Error
}

func (r *a2aPublicationRepository) MarkFailed(ctx context.Context, id uuid.UUID, lastErr string) error {
	return r.db.WithContext(ctx).Model(&models.A2APublication{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"status":          models.A2APublicationStatusFailed,
			"attempt_count":   gorm.Expr("attempt_count + 1"),
			"last_error":      lastErr,
			"next_attempt_at": nil,
			"updated_at":      time.Now(),
		}).Error
}

func (r *a2aPublicationRepository) DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error {
	return r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		Delete(&models.A2APublication{}).Error
}
