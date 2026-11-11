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

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
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

	// ClearCard drops the stored card and parks the row (pending, never due) until Requeue; ErrA2AAgentCardNotFound when absent.
	ClearCard(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error
	// FailParked marks a row ClearCard parked as failed with lastErr; any other row is left alone.
	FailParked(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID, lastErr string) error

	// ClaimDue leases up to limit pending rows whose next_attempt_at has passed; NULL is never due.
	ClaimDue(ctx context.Context, now time.Time, limit int) ([]models.A2AAgentCard, error)

	// The Mark* methods act on the row as ClaimDue returned it and return
	// ErrA2AAgentCardSuperseded when updated_at moved (a newer Enqueue).

	// MarkFetched rewrites card and card_hash only when cardHash differs from read.CardHash, and source_url only for platform rows.
	MarkFetched(ctx context.Context, read models.A2AAgentCard, card json.RawMessage, cardHash, fetchedURL, releaseName string) error
	MarkAttemptFailed(ctx context.Context, read models.A2AAgentCard, lastErr, releaseName string, nextAttemptAt time.Time) error
	MarkFailed(ctx context.Context, read models.A2AAgentCard, lastErr, releaseName string) error

	// Get returns ErrA2AAgentCardNotFound when no row exists.
	Get(ctx context.Context, ouID, projectName, agentName, environmentName string) (*models.A2AAgentCard, error)
	// ListForAgent returns the agent's rows across environments, ordered by environment name.
	ListForAgent(ctx context.Context, ouID, projectName, agentName string) ([]models.A2AAgentCard, error)
	DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error
	DeleteForAgentEnv(ctx context.Context, ouID, projectName, agentName, environmentName string) error
	// DeleteForAgentEnvUUID is DeleteForAgentEnv for a caller that knows the environment by its UUID.
	DeleteForAgentEnvUUID(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error
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

// agentCond matches the rows of the named agent.
func agentCond(db *gorm.DB, ouID, projectName, agentName string) (string, []any) {
	return "org_id = ? AND agent_id IN (?)", []any{ouID, models.AgentIDQuery(db, ouID, projectName, agentName)}
}

// agentEnvCond matches the named agent's rows in the named environment.
func agentEnvCond(db *gorm.DB, ouID, projectName, agentName, environmentName string) (string, []any) {
	cond, args := agentCond(db, ouID, projectName, agentName)
	return cond + " AND environment_id IN (?)", append(args, models.EnvironmentIDQuery(db, ouID, environmentName))
}

// agentEnvIDCond matches the named agent's rows in an environment known by id.
func agentEnvIDCond(db *gorm.DB, ouID, projectName, agentName string, environmentUUID uuid.UUID) (string, []any) {
	cond, args := agentCond(db, ouID, projectName, agentName)
	return cond + " AND environment_id = ?", append(args, environmentUUID)
}

func (r *a2aAgentCardRepository) Enqueue(ctx context.Context, card *models.A2AAgentCard) error {
	now := dbx.Now()
	card.Status = models.A2AAgentCardStatusPending
	card.AttemptCount = 0
	card.LastError = ""
	card.NextAttemptAt = &now
	card.UpdatedAt = now
	if card.ID == uuid.Nil {
		card.ID = newID()
	}
	if card.CreatedAt.IsZero() {
		card.CreatedAt = now
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if card.AgentUUID == nil || *card.AgentUUID == uuid.Nil {
			id, err := models.AgentIDByNames(tx, card.OUID, card.ProjectName, card.AgentName)
			if err != nil {
				return err
			}
			card.AgentUUID = &id
		}
		_, err := dbx.Upsert(tx, card, func(tx *gorm.DB) error {
			var current models.A2AAgentCard
			key := "agent_id = ? AND environment_id = ?"
			if err := tx.Where(key, *card.AgentUUID, card.EnvironmentUUID).Take(&current).Error; err != nil {
				return err
			}
			updates := map[string]any{
				"status": card.Status, "attempt_count": 0, "last_error": "", "next_attempt_at": now, "updated_at": now,
				"source": card.Source,
			}
			// A different source, or an external row naming a different URL, makes the stored card stale.
			if current.Source != card.Source ||
				(card.Source == models.A2AAgentCardSourceExternal && card.SourceURL != "" && current.SourceURL != card.SourceURL) {
				updates["card"] = nil
				updates["card_hash"] = ""
				updates["fetched_at"] = nil
				updates["source_url"] = card.SourceURL
			}
			return tx.Model(&models.A2AAgentCard{}).Where("id = ?", current.ID).Updates(updates).Error
		})
		return err
	})
}

func (r *a2aAgentCardRepository) Requeue(ctx context.Context, ouID, projectName, agentName, environmentName string) error {
	now := dbx.Now()
	cond, args := agentEnvCond(r.db, ouID, projectName, agentName, environmentName)
	result := r.db.WithContext(ctx).Model(&models.A2AAgentCard{}).Where(cond, args...).
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

func (r *a2aAgentCardRepository) WithTx(tx *gorm.DB) A2AAgentCardRepository {
	return &a2aAgentCardRepository{db: tx}
}

func (r *a2aAgentCardRepository) ClearCard(ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID) error {
	cond, args := agentEnvIDCond(r.db, ouID, projectName, agentName, environmentUUID)
	result := r.db.WithContext(ctx).Model(&models.A2AAgentCard{}).Where(cond, args...).
		Updates(map[string]interface{}{
			"card":            nil,
			"card_hash":       "",
			"fetched_at":      nil,
			"status":          models.A2AAgentCardStatusPending,
			"attempt_count":   0,
			"last_error":      "",
			"next_attempt_at": nil,
			"updated_at":      dbx.Now(),
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
	cond, args := agentEnvIDCond(r.db, ouID, projectName, agentName, environmentUUID)
	return r.db.WithContext(ctx).Model(&models.A2AAgentCard{}).
		Where(cond+" AND status = ? AND next_attempt_at IS NULL", append(args, models.A2AAgentCardStatusPending)...).
		Updates(map[string]interface{}{
			"status":     models.A2AAgentCardStatusFailed,
			"last_error": lastErr,
			"updated_at": dbx.Now(),
		}).Error
}

// ClaimDue leases each due row with its own conditional update that pushes next_attempt_at past the lease, so of
// two concurrent claimers only the first still finds it due.
func (r *a2aAgentCardRepository) ClaimDue(ctx context.Context, now time.Time, limit int) ([]models.A2AAgentCard, error) {
	now = now.UTC().Truncate(time.Microsecond)
	lease := now.Add(A2AAgentCardClaimLease)
	db := r.db.WithContext(ctx)
	var due []models.A2AAgentCard
	err := db.Omit("card").Where("status = ? AND next_attempt_at <= ?", models.A2AAgentCardStatusPending, now).
		Order("next_attempt_at ASC, created_at ASC").Limit(limit).Find(&due).Error
	if err != nil {
		return nil, err
	}
	claimed := make([]models.A2AAgentCard, 0, len(due))
	for _, row := range due {
		res := db.Model(&models.A2AAgentCard{}).
			Where("id = ? AND status = ? AND next_attempt_at <= ?", row.ID, models.A2AAgentCardStatusPending, now).
			UpdateColumn("next_attempt_at", lease)
		if res.Error != nil {
			return nil, res.Error
		}
		if res.RowsAffected == 0 {
			continue
		}
		row.NextAttemptAt = &lease
		claimed = append(claimed, row)
	}
	return claimed, nil
}

func (r *a2aAgentCardRepository) MarkFetched(ctx context.Context, read models.A2AAgentCard, card json.RawMessage, cardHash, fetchedURL, _ string) error {
	updates := map[string]interface{}{
		"status":          models.A2AAgentCardStatusFetched,
		"fetched_at":      dbx.Now(),
		"last_error":      "",
		"next_attempt_at": nil,
	}
	// An external row's source_url is the registered URL; a fetch through the gateway must not replace it.
	if read.Source == models.A2AAgentCardSourcePlatform {
		updates["source_url"] = fetchedURL
	}
	if cardHash != read.CardHash {
		updates["card"] = card
		updates["card_hash"] = cardHash
	}
	return r.updateIfUnchanged(ctx, read, updates)
}

func (r *a2aAgentCardRepository) MarkAttemptFailed(ctx context.Context, read models.A2AAgentCard, lastErr, _ string, nextAttemptAt time.Time) error {
	return r.updateIfUnchanged(ctx, read, map[string]interface{}{
		"attempt_count":   gorm.Expr("attempt_count + 1"),
		"last_error":      lastErr,
		"next_attempt_at": nextAttemptAt.UTC().Truncate(time.Microsecond),
	})
}

func (r *a2aAgentCardRepository) MarkFailed(ctx context.Context, read models.A2AAgentCard, lastErr, _ string) error {
	return r.updateIfUnchanged(ctx, read, map[string]interface{}{
		"status":          models.A2AAgentCardStatusFailed,
		"attempt_count":   gorm.Expr("attempt_count + 1"),
		"last_error":      lastErr,
		"next_attempt_at": nil,
	})
}

func (r *a2aAgentCardRepository) updateIfUnchanged(ctx context.Context, read models.A2AAgentCard, updates map[string]interface{}) error {
	updates["updated_at"] = dbx.Now()
	result := r.db.WithContext(ctx).Model(&models.A2AAgentCard{}).
		Where("id = ? AND updated_at = ?", read.ID, read.UpdatedAt.UTC()).
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
		Where("id = ? AND updated_at = ?", read.ID, read.UpdatedAt.UTC()).
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
	cond, args := agentEnvCond(r.db, ouID, projectName, agentName, environmentName)
	err := r.db.WithContext(ctx).Where(cond, args...).Take(&row).Error
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
	cond, args := agentCond(r.db, ouID, projectName, agentName)
	err := r.db.WithContext(ctx).Where(cond, args...).Order("environment_id ASC").Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *a2aAgentCardRepository) DeleteForAgent(ctx context.Context, ouID, projectName, agentName string) error {
	cond, args := agentCond(r.db, ouID, projectName, agentName)
	return r.db.WithContext(ctx).Where(cond, args...).Delete(&models.A2AAgentCard{}).Error
}

func (r *a2aAgentCardRepository) DeleteForAgentEnv(ctx context.Context, ouID, projectName, agentName, environmentName string) error {
	cond, args := agentEnvCond(r.db, ouID, projectName, agentName, environmentName)
	return r.db.WithContext(ctx).Where(cond, args...).Delete(&models.A2AAgentCard{}).Error
}

func (r *a2aAgentCardRepository) DeleteForAgentEnvUUID(
	ctx context.Context, ouID, projectName, agentName string, environmentUUID uuid.UUID,
) error {
	cond, args := agentEnvIDCond(r.db, ouID, projectName, agentName, environmentUUID)
	return r.db.WithContext(ctx).Where(cond, args...).Delete(&models.A2AAgentCard{}).Error
}

func (r *a2aAgentCardRepository) DeletePlatformForAgentEnv(ctx context.Context, ouID, projectName, agentName, environmentName string) error {
	cond, args := agentEnvCond(r.db, ouID, projectName, agentName, environmentName)
	return r.db.WithContext(ctx).Where(cond+" AND source = ?", append(args, models.A2AAgentCardSourcePlatform)...).
		Delete(&models.A2AAgentCard{}).Error
}

func (r *a2aAgentCardRepository) DeleteForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) error {
	return deleteForEnvironment[models.A2AAgentCard](ctx, r.db, ouID, environmentUUID)
}

func (r *a2aAgentCardRepository) ListForEnvironment(ctx context.Context, ouID string, environmentUUID uuid.UUID) ([]models.A2AAgentCard, error) {
	return listForEnvironment[models.A2AAgentCard](ctx, r.db, ouID, environmentUUID)
}

func (r *a2aAgentCardRepository) ListEnvironmentUUIDs(ctx context.Context, ouID, environmentName string) ([]uuid.UUID, error) {
	return listEnvironmentUUIDs[models.A2AAgentCard](ctx, r.db, ouID, environmentName)
}

// deleteForEnvironment deletes every T row of the environment.
func deleteForEnvironment[T any](ctx context.Context, db *gorm.DB, ouID string, environmentUUID uuid.UUID) error {
	return db.WithContext(ctx).
		Where("org_id = ? AND environment_id = ?", ouID, environmentUUID).
		Delete(new(T)).Error
}

// listForEnvironment lists the T rows of one environment, in agent order.
func listForEnvironment[T any](ctx context.Context, db *gorm.DB, ouID string, environmentUUID uuid.UUID) ([]T, error) {
	var rows []T
	err := db.WithContext(ctx).
		Where("org_id = ? AND environment_id = ?", ouID, environmentUUID).
		Order("agent_id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	return rows, nil
}

// listEnvironmentUUIDs lists the distinct environment ids of the T rows in the environment named environmentName.
func listEnvironmentUUIDs[T any](ctx context.Context, db *gorm.DB, ouID, environmentName string) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	err := db.WithContext(ctx).Model(new(T)).
		Where("org_id = ? AND environment_id IN (?)", ouID, models.EnvironmentIDQuery(db, ouID, environmentName)).
		Distinct().Pluck("environment_id", &ids).Error
	return ids, err
}
