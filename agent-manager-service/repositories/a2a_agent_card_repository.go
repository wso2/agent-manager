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

// ErrA2AAgentCardSuperseded means the row was re-enqueued after the caller read it.
var ErrA2AAgentCardSuperseded = errors.New("a2a agent card was re-enqueued since it was read")

// ErrA2AAgentCardNotFound means no card row exists for the agent and environment.
var ErrA2AAgentCardNotFound = errors.New("a2a agent card not found")

// A2AAgentCardRepository stores each A2A agent's card per environment and queues its fetches.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/a2a_agent_card_repository_mock.go . A2AAgentCardRepository:A2AAgentCardRepositoryMock
type A2AAgentCardRepository interface {
	// WithTx returns the repository bound to tx, so its writes commit or roll back with the caller's.
	WithTx(tx *gorm.DB) A2AAgentCardRepository

	// Enqueue upserts the environment's row as pending, replacing a same-named row from a deleted environment,
	// and leaves it with a fresh budget, keeping card, card_hash and fetched_at.
	// A different source, or an external row naming a different source_url, replaces them and clears the card state.
	Enqueue(ctx context.Context, card *models.A2AAgentCard) error

	// Requeue resets an existing row to pending without creating one; it returns ErrA2AAgentCardNotFound when absent.
	Requeue(ctx context.Context, ouID, projectName, agentName, environmentName string) error

	// ClearCard drops the stored card and its projection and parks the row (pending, never due) until Requeue; ErrA2AAgentCardNotFound when absent.
	ClearCard(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error
	// FailParked marks a row ClearCard parked as failed with lastErr; any other row is left alone.
	FailParked(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID, lastErr string) error

	// ClaimDue leases up to limit pending rows whose next_attempt_at has passed; NULL is never due.
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]models.A2AAgentCard, error)
	// RequeueFetchedWithoutProjection makes every fetched card that has no fingerprint due again and reports how many;
	// during a rolling upgrade a pod from before the projection can store a card without one.
	RequeueFetchedWithoutProjection(ctx context.Context) (int64, error)

	// The Mark* methods act on the row as ClaimDue returned it and return
	// ErrA2AAgentCardSuperseded when updated_at moved (a newer Enqueue).

	// MarkFetched rewrites card and card_hash only when cardHash differs from read.CardHash, and source_url only for platform rows.
	// It always writes the projection, so a card stored before the projection existed gains one on its next fetch.
	MarkFetched(
		ctx context.Context, read models.A2AAgentCard, card json.RawMessage, cardHash, fetchedURL, releaseName string,
		projection models.A2ACardProjection,
	) error
	MarkAttemptFailed(ctx context.Context, read models.A2AAgentCard, lastErr, releaseName string, nextAttemptAt time.Time) error
	MarkFailed(ctx context.Context, read models.A2AAgentCard, lastErr, releaseName string) error

	// Get returns ErrA2AAgentCardNotFound when no row exists.
	Get(ctx context.Context, ouID, projectName, agentName, environmentName string) (*models.A2AAgentCard, error)
	// ListForAgent returns the agent's rows across environments, ordered by environment name.
	ListForAgent(ctx context.Context, ouID, projectName, agentName string) ([]models.A2AAgentCard, error)
	DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error
	DeleteForAgentEnv(ctx context.Context, ouID, projectName, agentName, environmentName string) error
	// DeletePlatformForAgentEnv drops a platform row and keeps an external one, whose URL is user config.
	DeletePlatformForAgentEnv(ctx context.Context, ouID, projectName, agentName, environmentName string) error
	// DeleteIfUnchanged removes the row as read; ErrA2AAgentCardSuperseded when it changed since.
	DeleteIfUnchanged(ctx context.Context, read models.A2AAgentCard) error
	// DeleteForEnvironment drops every agent's row in a deleted environment.
	DeleteForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) error
	// ListForEnvironment returns every agent's row for the environment, ordered by project and agent name.
	ListForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) ([]models.A2AAgentCard, error)
	// ListEnvironmentUUIDs returns the distinct environment UUIDs with rows recorded under environmentName.
	ListEnvironmentUUIDs(ctx context.Context, ouID, environmentName string) ([]uuid.UUID, error)
}

// A2AAgentCardClaimLease is how long a claimed row stays invisible to other claimers.
const A2AAgentCardClaimLease = 5 * time.Minute

type a2aAgentCardRepository struct {
	db *gorm.DB
}

// NewA2AAgentCardRepository creates an A2AAgentCardRepository.
func NewA2AAgentCardRepository(db *gorm.DB) A2AAgentCardRepository {
	return &a2aAgentCardRepository{db: db}
}

func (r *a2aAgentCardRepository) Enqueue(ctx context.Context, card *models.A2AAgentCard) error {
	// Postgres keeps microseconds; the Mark* guards compare against this value.
	now := time.Now().Truncate(time.Microsecond)
	card.Status = models.A2AAgentCardStatusPending
	card.AttemptCount = 0
	card.LastError = ""
	card.NextAttemptAt = &now
	card.UpdatedAt = now

	changed := a2aCardSourceChanged
	if card.Source == models.A2AAgentCardSourceExternal && card.SourceURL != "" {
		changed += " OR " + a2aCardSourceURLChanged
	}
	updates := clause.AssignmentColumns([]string{"status", "attempt_count", "last_error", "next_attempt_at", "updated_at", "source"})
	updates = append(updates, newSourceURLAssignments(changed)...)
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// A same-named row from a deleted environment must not outlive its replacement.
		err := tx.Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_name = ? AND environment_uuid <> ?",
			card.OUID, card.ProjectName, card.AgentName, card.EnvironmentName, card.EnvironmentUUID).
			Delete(&models.A2AAgentCard{}).Error
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
			DoUpdates: updates,
		}).Create(card).Error
	})
}

func (r *a2aAgentCardRepository) Requeue(ctx context.Context, ouID, projectName, agentName, environmentName string) error {
	now := time.Now().Truncate(time.Microsecond)
	result := r.db.WithContext(ctx).Model(&models.A2AAgentCard{}).
		Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_name = ?",
			ouID, projectName, agentName, environmentName).
		Updates(map[string]interface{}{
			"status":          models.A2AAgentCardStatusPending,
			"attempt_count":   0,
			"last_error":      "",
			"next_attempt_at": now,
			"updated_at":      now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrA2AAgentCardNotFound
	}
	return nil
}

const (
	a2aCardSourceChanged    = "a2a_agent_cards.source IS DISTINCT FROM excluded.source"
	a2aCardSourceURLChanged = "a2a_agent_cards.source_url IS DISTINCT FROM excluded.source_url"
)

// newSourceURLAssignments takes the new source_url and drops the old card when changed holds.
func newSourceURLAssignments(changed string) []clause.Assignment {
	keepUnlessChanged := func(column, replacement string) clause.Assignment {
		return clause.Assignment{
			Column: clause.Column{Name: column},
			Value:  gorm.Expr("CASE WHEN " + changed + " THEN " + replacement + " ELSE a2a_agent_cards." + column + " END"),
		}
	}
	return []clause.Assignment{
		keepUnlessChanged("card", "NULL"),
		keepUnlessChanged("card_hash", "''"),
		keepUnlessChanged("card_fingerprint", "''"),
		keepUnlessChanged("card_search_text", "''"),
		keepUnlessChanged("fetched_at", "NULL"),
		keepUnlessChanged("release_name", "''"),
		keepUnlessChanged("source_url", "excluded.source_url"),
	}
}

// a2aCardClaimColumns omits card: the reconciler compares card_hash and never reads the stored body.
const a2aCardClaimColumns = `id, ou_id, project_name, agent_name, environment_name, environment_uuid, source, source_url, card_hash,
	fetched_at, release_name, attempted_release_name, status, attempt_count, last_error, next_attempt_at, created_at, updated_at`

func (r *a2aAgentCardRepository) WithTx(tx *gorm.DB) A2AAgentCardRepository {
	return &a2aAgentCardRepository{db: tx}
}

func (r *a2aAgentCardRepository) ClearCard(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error {
	result := r.db.WithContext(ctx).Model(&models.A2AAgentCard{}).
		Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_uuid = ?",
			ouID, projectName, agentName, environmentUUID).
		Updates(map[string]interface{}{
			"card":             nil,
			"card_hash":        "",
			"card_fingerprint": "",
			"card_search_text": "",
			"fetched_at":       nil,
			"release_name":     "",
			"status":           models.A2AAgentCardStatusPending,
			"attempt_count":    0,
			"last_error":       "",
			"next_attempt_at":  nil,
			"updated_at":       time.Now().Truncate(time.Microsecond),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrA2AAgentCardNotFound
	}
	return nil
}

func (r *a2aAgentCardRepository) FailParked(
	ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID, lastErr string,
) error {
	return r.db.WithContext(ctx).Model(&models.A2AAgentCard{}).
		Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_uuid = ? AND status = ? AND next_attempt_at IS NULL",
			ouID, projectName, agentName, environmentUUID, models.A2AAgentCardStatusPending).
		Updates(map[string]interface{}{
			"status":     models.A2AAgentCardStatusFailed,
			"last_error": lastErr,
			"updated_at": time.Now().Truncate(time.Microsecond),
		}).Error
}

func (r *a2aAgentCardRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]models.A2AAgentCard, error) {
	var claimed []models.A2AAgentCard
	err := r.db.WithContext(ctx).Raw(
		`
		UPDATE a2a_agent_cards SET next_attempt_at = ?
		WHERE id IN (
			SELECT id FROM a2a_agent_cards
			WHERE status = ? AND next_attempt_at <= ?
			ORDER BY next_attempt_at ASC, created_at ASC
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		RETURNING `+a2aCardClaimColumns,
		now.Add(A2AAgentCardClaimLease), models.A2AAgentCardStatusPending, now, limit,
	).Scan(&claimed).Error
	if err != nil {
		return nil, err
	}
	return claimed, nil
}

func (r *a2aAgentCardRepository) RequeueFetchedWithoutProjection(ctx context.Context) (int64, error) {
	res := r.db.WithContext(ctx).Model(&models.A2AAgentCard{}).
		Where("status = ? AND card IS NOT NULL AND card_fingerprint = ''", models.A2AAgentCardStatusFetched).
		Updates(map[string]interface{}{
			"status":          models.A2AAgentCardStatusPending,
			"attempt_count":   0,
			"next_attempt_at": time.Now(),
		})
	return res.RowsAffected, res.Error
}

func (r *a2aAgentCardRepository) MarkFetched(
	ctx context.Context, read models.A2AAgentCard, card json.RawMessage, cardHash, fetchedURL, releaseName string,
	projection models.A2ACardProjection,
) error {
	updates := map[string]interface{}{
		"status":           models.A2AAgentCardStatusFetched,
		"fetched_at":       time.Now(),
		"last_error":       "",
		"next_attempt_at":  nil,
		"release_name":     releaseName,
		"card_fingerprint": projection.Fingerprint,
		"card_search_text": projection.SearchText,
	}
	// An external row's source_url is the registered URL; a fetch through the gateway must not replace it.
	if read.Source == models.A2AAgentCardSourcePlatform {
		updates["source_url"] = fetchedURL
	}
	if cardHash != read.CardHash {
		updates["card"] = card
		updates["card_hash"] = cardHash
	}
	return r.updateIfUnchanged(ctx, read, withAttemptedRelease(updates, releaseName))
}

func (r *a2aAgentCardRepository) MarkAttemptFailed(ctx context.Context, read models.A2AAgentCard, lastErr, releaseName string, nextAttemptAt time.Time) error {
	return r.updateIfUnchanged(ctx, read, withAttemptedRelease(map[string]interface{}{
		"attempt_count":   gorm.Expr("attempt_count + 1"),
		"last_error":      lastErr,
		"next_attempt_at": nextAttemptAt,
	}, releaseName))
}

func (r *a2aAgentCardRepository) MarkFailed(ctx context.Context, read models.A2AAgentCard, lastErr, releaseName string) error {
	return r.updateIfUnchanged(ctx, read, withAttemptedRelease(map[string]interface{}{
		"status":          models.A2AAgentCardStatusFailed,
		"attempt_count":   gorm.Expr("attempt_count + 1"),
		"last_error":      lastErr,
		"next_attempt_at": nil,
	}, releaseName))
}

// withAttemptedRelease keeps the last known release when this attempt never learned one.
func withAttemptedRelease(updates map[string]interface{}, releaseName string) map[string]interface{} {
	if releaseName != "" {
		updates["attempted_release_name"] = releaseName
	}
	return updates
}

func (r *a2aAgentCardRepository) updateIfUnchanged(ctx context.Context, read models.A2AAgentCard, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	result := r.db.WithContext(ctx).Model(&models.A2AAgentCard{}).
		Where("id = ? AND updated_at = ?", read.ID, read.UpdatedAt).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrA2AAgentCardSuperseded
	}
	return nil
}

func (r *a2aAgentCardRepository) DeleteIfUnchanged(ctx context.Context, read models.A2AAgentCard) error {
	result := r.db.WithContext(ctx).
		Where("id = ? AND updated_at = ?", read.ID, read.UpdatedAt).
		Delete(&models.A2AAgentCard{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrA2AAgentCardSuperseded
	}
	return nil
}

func (r *a2aAgentCardRepository) Get(ctx context.Context, ouID, projectName, agentName, environmentName string) (*models.A2AAgentCard, error) {
	var row models.A2AAgentCard
	err := r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_name = ?",
			ouID, projectName, agentName, environmentName).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrA2AAgentCardNotFound
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *a2aAgentCardRepository) ListForAgent(ctx context.Context, ouID, projectName, agentName string) ([]models.A2AAgentCard, error) {
	var rows []models.A2AAgentCard
	err := r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		Order("environment_name ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *a2aAgentCardRepository) DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error {
	return r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		Delete(&models.A2AAgentCard{}).Error
}

func (r *a2aAgentCardRepository) DeleteForAgentEnv(ctx context.Context, ouID, projectName, agentName, environmentName string) error {
	return r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_name = ?",
			ouID, projectName, agentName, environmentName).
		Delete(&models.A2AAgentCard{}).Error
}

func (r *a2aAgentCardRepository) DeletePlatformForAgentEnv(ctx context.Context, ouID, projectName, agentName, environmentName string) error {
	return r.db.WithContext(ctx).
		Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_name = ? AND source = ?",
			ouID, projectName, agentName, environmentName, models.A2AAgentCardSourcePlatform).
		Delete(&models.A2AAgentCard{}).Error
}

func (r *a2aAgentCardRepository) DeleteForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) error {
	return deleteForEnvironment[models.A2AAgentCard](ctx, r.db, ouID, environmentUUID)
}

func (r *a2aAgentCardRepository) ListForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) ([]models.A2AAgentCard, error) {
	return listForEnvironment[models.A2AAgentCard](ctx, r.db, ouID, environmentUUID, "project_name ASC, agent_name ASC")
}

func (r *a2aAgentCardRepository) ListEnvironmentUUIDs(ctx context.Context, ouID, environmentName string) ([]uuid.UUID, error) {
	return listEnvironmentUUIDs[models.A2AAgentCard](ctx, r.db, ouID, environmentName)
}

// deleteForEnvironment deletes every T row of the environment.
func deleteForEnvironment[T any](ctx context.Context, db *gorm.DB, ouID string, environmentUUID uuid.UUID) error {
	return db.WithContext(ctx).
		Where("ou_id = ? AND environment_uuid = ?", ouID, environmentUUID).
		Delete(new(T)).Error
}

// listForEnvironment lists the T rows of one environment in the given order.
func listForEnvironment[T any](ctx context.Context, db *gorm.DB, ouID string, environmentUUID uuid.UUID, order string) ([]T, error) {
	var rows []T
	err := db.WithContext(ctx).
		Where("ou_id = ? AND environment_uuid = ?", ouID, environmentUUID).
		Order(order).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// listEnvironmentUUIDs lists the distinct environment UUIDs of the T rows recorded under environmentName.
func listEnvironmentUUIDs[T any](ctx context.Context, db *gorm.DB, ouID, environmentName string) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := db.WithContext(ctx).Model(new(T)).
		Where("ou_id = ? AND environment_name = ?", ouID, environmentName).
		Distinct().Pluck("environment_uuid", &ids).Error
	return ids, err
}
