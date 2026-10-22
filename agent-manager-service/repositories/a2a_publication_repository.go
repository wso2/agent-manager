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
	"errors"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// ErrA2APublicationSuperseded means the row changed after the caller read it —
// a newer Enqueue reset it — so the caller's outcome belongs to a publication
// that is no longer current and was not recorded.
var ErrA2APublicationSuperseded = errors.New("a2a publication was re-enqueued since it was read")

// ErrA2APublicationNotFound means no publication row exists for the agent and environment.
var ErrA2APublicationNotFound = errors.New("a2a publication not found")

// A2APublicationRepository is the queue of outstanding A2A gateway publications.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/a2a_publication_repository_mock.go . A2APublicationRepository:A2APublicationRepositoryMock
type A2APublicationRepository interface {
	// Enqueue records that an agent-environment pair needs publishing, resetting
	// any existing row for that pair to pending with a fresh attempt budget. A
	// redeploy must re-publish, and a pair that previously exhausted its budget
	// must get another chance. A same-named row from a deleted environment is replaced.
	Enqueue(ctx context.Context, pub *models.A2APublication) error

	// WithTx returns the repository bound to tx, so its writes commit or roll back with the caller's.
	WithTx(tx *gorm.DB) A2APublicationRepository

	// Get returns ErrA2APublicationNotFound when no row exists.
	Get(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) (*models.A2APublication, error)

	// ListForEnvironment returns every agent's row in one environment.
	ListForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) ([]models.A2APublication, error)

	// DeleteForAgentEnv removes one environment's row.
	DeleteForAgentEnv(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error

	// DeleteForEnvironment removes every agent's row in a deleted environment.
	DeleteForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) error
	// ListEnvironmentUUIDs returns the distinct environment UUIDs with rows recorded under environmentName.
	ListEnvironmentUUIDs(ctx context.Context, ouID, environmentName string) ([]uuid.UUID, error)

	// ClaimDue returns up to limit pending rows whose next attempt time has
	// arrived, pushing each one's next attempt out by a lease so no other
	// replica claims it mid-attempt. An attempt that dies without recording an
	// outcome is retried once the lease runs out.
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]models.A2APublication, error)

	// The Mark* methods record the outcome of an attempt on the row as ClaimDue
	// returned it. Enqueue always moves updated_at, so a row whose updated_at no
	// longer matches was re-enqueued mid-attempt; it is left pending for the next
	// tick and ErrA2APublicationSuperseded is returned.

	// MarkPublished also records the upstream URL and the JSON transports the gateway was given; nil transports store NULL.
	MarkPublished(ctx context.Context, read models.A2APublication, upstreamURL string, transports json.RawMessage) error

	// MarkAttemptFailed records a retryable failure and schedules the next try.
	MarkAttemptFailed(ctx context.Context, read models.A2APublication, lastErr string, nextAttemptAt time.Time) error

	// MarkWaiting schedules the next try without charging the attempt budget.
	MarkWaiting(ctx context.Context, read models.A2APublication, reason string, nextAttemptAt time.Time) error

	// Requeue resets a published row to pending with a fresh attempt budget.
	Requeue(ctx context.Context, read models.A2APublication) error
	// RequeueFailedExternal makes every failed external row due again and reports how many; a rolling upgrade's
	// old pods fail those rows, as they publish nothing but release bindings.
	RequeueFailedExternal(ctx context.Context) (int64, error)

	// FindPublished pages through published platform rows in id order, starting after afterID. External origins are never drift-checked.
	FindPublished(ctx context.Context, afterID uuid.UUID, limit int) ([]models.A2APublication, error)

	// MarkFailed ends the retry cycle. The row is kept as the record of an agent
	// that never reached its gateway.
	MarkFailed(ctx context.Context, read models.A2APublication, lastErr string) error

	// DeleteForAgent removes every environment's row for a deleted agent.
	DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error
}

// A2APublicationClaimLease comfortably outlasts one publish attempt.
const A2APublicationClaimLease = 5 * time.Minute

type a2aPublicationRepository struct {
	db *gorm.DB
}

// NewA2APublicationRepository creates an A2APublicationRepository.
func NewA2APublicationRepository(db *gorm.DB) A2APublicationRepository {
	return &a2aPublicationRepository{db: db}
}

func (r *a2aPublicationRepository) WithTx(tx *gorm.DB) A2APublicationRepository {
	return &a2aPublicationRepository{db: tx}
}

func (r *a2aPublicationRepository) Enqueue(ctx context.Context, pub *models.A2APublication) error {
	// Postgres keeps microseconds; matching that here keeps pub.UpdatedAt equal
	// to the stored value, which the Mark* methods compare against.
	now := time.Now().Truncate(time.Microsecond)
	pub.Status = models.A2APublicationStatusPending
	pub.AttemptCount = 0
	pub.LastError = ""
	pub.NextAttemptAt = &now
	pub.UpdatedAt = now
	if pub.Source == "" {
		pub.Source = models.A2APublicationSourcePlatform
	}

	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A same-named row from a deleted environment must not outlive its replacement.
		err := tx.Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_name = ? AND environment_uuid <> ?",
			pub.OUID, pub.ProjectName, pub.AgentName, pub.EnvironmentName, pub.EnvironmentUUID).
			Delete(&models.A2APublication{}).Error
		if err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "ou_id"},
				{Name: "project_name"},
				{Name: "agent_name"},
				{Name: "environment_uuid"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"environment_name", "artifact_uuid", "agent_uuid", "source", "status",
				"attempt_count", "last_error", "next_attempt_at", "updated_at",
			}),
		}).Create(pub).Error
	})
}

func (r *a2aPublicationRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]models.A2APublication, error) {
	// The claim leaves updated_at alone, so the Mark* check still only trips on a re-enqueue.
	var claimed []models.A2APublication
	err := r.db.WithContext(ctx).Raw(
		`
		UPDATE a2a_publications SET next_attempt_at = ?
		WHERE id IN (
			SELECT id FROM a2a_publications
			WHERE status = ? AND (next_attempt_at IS NULL OR next_attempt_at <= ?)
			ORDER BY next_attempt_at ASC, created_at ASC
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		RETURNING *`,
		now.Add(A2APublicationClaimLease), models.A2APublicationStatusPending, now, limit,
	).Scan(&claimed).Error
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (r *a2aPublicationRepository) MarkPublished(ctx context.Context, read models.A2APublication, upstreamURL string, transports json.RawMessage) error {
	var publishedTransports interface{}
	if len(transports) > 0 {
		publishedTransports = transports
	}
	return r.updateIfUnchanged(ctx, read, map[string]interface{}{
		"status":                 models.A2APublicationStatusPublished,
		"last_error":             "",
		"next_attempt_at":        nil,
		"published_upstream_url": upstreamURL,
		"published_transports":   publishedTransports,
	})
}

func (r *a2aPublicationRepository) MarkWaiting(ctx context.Context, read models.A2APublication, reason string, nextAttemptAt time.Time) error {
	return r.updateIfUnchanged(ctx, read, map[string]interface{}{
		"last_error":      reason,
		"next_attempt_at": nextAttemptAt,
	})
}

func (r *a2aPublicationRepository) Requeue(ctx context.Context, read models.A2APublication) error {
	return r.updateIfUnchanged(ctx, read, dueWithFreshBudget())
}

// dueWithFreshBudget makes a row pending now with its attempts and last error cleared.
func dueWithFreshBudget() map[string]interface{} {
	return map[string]interface{}{
		"status":          models.A2APublicationStatusPending,
		"attempt_count":   0,
		"last_error":      "",
		"next_attempt_at": time.Now(),
	}
}

func (r *a2aPublicationRepository) RequeueFailedExternal(ctx context.Context) (int64, error) {
	res := r.db.WithContext(ctx).Model(&models.A2APublication{}).
		Where("source = ? AND status = ?", models.A2APublicationSourceExternal, models.A2APublicationStatusFailed).
		Updates(dueWithFreshBudget())
	return res.RowsAffected, res.Error
}

func (r *a2aPublicationRepository) FindPublished(ctx context.Context, afterID uuid.UUID, limit int) ([]models.A2APublication, error) {
	var rows []models.A2APublication
	err := r.db.WithContext(ctx).
		Where("status = ? AND source = ? AND id > ?",
			models.A2APublicationStatusPublished, models.A2APublicationSourcePlatform, afterID).
		Order("id ASC").
		Limit(limit).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *a2aPublicationRepository) Get(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) (*models.A2APublication, error) {
	var row models.A2APublication
	err := r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_uuid = ?",
			ouID, projectName, agentName, environmentUUID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrA2APublicationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *a2aPublicationRepository) ListForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) ([]models.A2APublication, error) {
	return listForEnvironment[models.A2APublication](ctx, r.db, ouID, environmentUUID, "agent_name ASC")
}

func (r *a2aPublicationRepository) DeleteForAgentEnv(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error {
	return r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_uuid = ?",
			ouID, projectName, agentName, environmentUUID).
		Delete(&models.A2APublication{}).Error
}

func (r *a2aPublicationRepository) DeleteForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) error {
	return deleteForEnvironment[models.A2APublication](ctx, r.db, ouID, environmentUUID)
}

func (r *a2aPublicationRepository) MarkAttemptFailed(ctx context.Context, read models.A2APublication, lastErr string, nextAttemptAt time.Time) error {
	return r.updateIfUnchanged(ctx, read, map[string]interface{}{
		"attempt_count":   gorm.Expr("attempt_count + 1"),
		"last_error":      lastErr,
		"next_attempt_at": nextAttemptAt,
	})
}

func (r *a2aPublicationRepository) MarkFailed(ctx context.Context, read models.A2APublication, lastErr string) error {
	return r.updateIfUnchanged(ctx, read, map[string]interface{}{
		"status":          models.A2APublicationStatusFailed,
		"attempt_count":   gorm.Expr("attempt_count + 1"),
		"last_error":      lastErr,
		"next_attempt_at": nil,
	})
}

func (r *a2aPublicationRepository) updateIfUnchanged(ctx context.Context, read models.A2APublication, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	result := r.db.WithContext(ctx).Model(&models.A2APublication{}).
		Where("id = ? AND updated_at = ?", read.ID, read.UpdatedAt).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrA2APublicationSuperseded
	}
	return nil
}

func (r *a2aPublicationRepository) DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error {
	return r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		Delete(&models.A2APublication{}).Error
}

func (r *a2aPublicationRepository) ListEnvironmentUUIDs(ctx context.Context, ouID, environmentName string) ([]uuid.UUID, error) {
	return listEnvironmentUUIDs[models.A2APublication](ctx, r.db, ouID, environmentName)
}
