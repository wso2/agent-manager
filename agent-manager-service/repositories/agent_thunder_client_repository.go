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
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// ErrAgentThunderClientNotFound is returned when no binding exists for the given
// agent and environment.
var ErrAgentThunderClientNotFound = errors.New("agent thunder client not found")

// AgentThunderClientRepository persists the AgentID binding described in the
// AgentID architecture doc (Section 7/8): one row per agent per environment,
// tracking provisioning status and retry state.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/agent_thunder_client_repository_mock.go . AgentThunderClientRepository:AgentThunderClientRepositoryMock
type AgentThunderClientRepository interface {
	// Upsert creates or updates a binding row, keyed on (org, project, agent, environment).
	Upsert(ctx context.Context, client *models.AgentThunderClient) error

	// FindByAgent returns every binding for the given agent, across all environments.
	FindByAgent(ctx context.Context, ouID, projectName, agentName string) ([]models.AgentThunderClient, error)

	// FindByOuAndEnvironment returns every binding row for the org (ou_id)+environment,
	// the source for the agent-identity assignment picker (name, status, ThunderAgentID).
	FindByOuAndEnvironment(ctx context.Context, ouID, environmentName string) ([]models.AgentThunderClient, error)

	// Get returns the binding for a specific agent in a specific environment.
	Get(ctx context.Context, ouID, projectName, agentName, environmentName string) (*models.AgentThunderClient, error)

	// FindDue returns up to limit bindings ready for an attempt: PENDING rows
	// whose next retry time has arrived (or was never set — the write-ahead row
	// before its first attempt), plus IN_PROGRESS rows abandoned by a crashed
	// attempt (last_attempted_at older than inProgressStaleThreshold) so a crash
	// mid-attempt cannot strand a binding forever.
	FindDue(ctx context.Context, now time.Time, limit int) ([]models.AgentThunderClient, error)

	// ClaimForAttempt atomically transitions a binding to IN_PROGRESS so the
	// inline fast-path goroutine and the reconciler's sweep can never both run
	// AttemptProvision on the same binding concurrently. Succeeds from PENDING,
	// or from a stale IN_PROGRESS row (a crashed prior attempt). claimed=false
	// means someone else already holds this binding (or it is no longer
	// pending/stale) — the caller must skip the attempt.
	ClaimForAttempt(ctx context.Context, id uuid.UUID) (claimed bool, err error)

	// ResetFailedForRetry atomically transitions a binding from FAILED back to
	// PENDING for a manual retry — clearing attempt_count, last_error, and
	// last_attempted_at so it starts a fresh attempt budget, and setting
	// next_retry_at to now so the reconciler's sweep is a safety net even if
	// the caller's own inline attempt never runs. Conditioned on the row's
	// CURRENT status being FAILED, so two concurrent retry calls (e.g. a
	// double click) can only ever reset it once; reset=false means it already
	// changed state (a concurrent retry won, or the reconciler got there
	// first) and the caller must treat that as a conflict, not a silent no-op.
	ResetFailedForRetry(ctx context.Context, id uuid.UUID) (reset bool, err error)

	// UpdateAfterAttempt records the outcome of a provisioning attempt: the resolved
	// Thunder identity (on success), status, attempt bookkeeping, and the next retry
	// time (nil once the binding is COMPLETED or FAILED).
	UpdateAfterAttempt(ctx context.Context, id uuid.UUID, fields AgentThunderAttemptUpdate) error

	// UpdateSecretRef updates only the stored secret location, without touching
	// status or retry bookkeeping. Used by regenerate (new path, internal
	// agents only) and revoke (cleared to "" — there is no currently valid
	// stored secret).
	UpdateSecretRef(ctx context.Context, id uuid.UUID, secretRefPath string) error

	// DeleteByAgent removes every binding for the given agent. Test-cleanup use
	// only — production deletion goes through DeleteByIDs so a concurrent
	// recreate of the same agent name can't have its fresh rows swept up too.
	DeleteByAgent(ctx context.Context, ouID, projectName, agentName string) error

	// DeleteByIDs removes exactly the given binding rows. No-op if ids is empty.
	DeleteByIDs(ctx context.Context, ids []uuid.UUID) error

	// FindRecentlyCompletedInternal returns up to limit COMPLETED internal
	// bindings created at/after createdAfter that still hold a valid secret —
	// the candidates the injection reconciler sweeps. Bounded on the existing
	// created_at column (no schema change). Used two ways: the periodic
	// per-tick sweep passes a recent cutoff (see identityInjectionReconcileWindow)
	// since steady-state sync is owned by the deploy/promote/rotation/MCP-change
	// paths; the one-time startup backfill passes the zero time to cover every
	// completed internal binding regardless of age (see
	// runInitialIdentityInjectionBackfill).
	//
	// Ordered by (created_at, id) — created_at alone is not a unique key, so id
	// breaks ties deterministically. after is nil for the first page; passing
	// the previous page's last row back in (see ReconcileCursor) continues
	// strictly past it, letting a caller page through every eligible row
	// (not just the oldest limit of them) via keyset pagination.
	FindRecentlyCompletedInternal(ctx context.Context, createdAfter time.Time, after *ReconcileCursor, limit int) ([]models.AgentThunderClient, error)

	// MarkSyncAfter sets sync_after to at on the given bindings that carry an
	// identity by promotion (promotion_source is set) and are completed or in
	// progress. An in progress attempt may have read the carried set before the
	// change, so it is marked too. A pending or failed binding is left alone: its
	// next attempt reads the set as it is then. It overwrites an earlier value: a
	// later request moves the time later, so a burst of changes becomes one
	// promotion.
	MarkSyncAfter(ctx context.Context, ids []uuid.UUID, at time.Time) error

	// FindSyncDue returns up to limit completed bindings whose sync_after has
	// passed, oldest first.
	FindSyncDue(ctx context.Context, now time.Time, limit int) ([]models.AgentThunderClient, error)

	// ClaimSync takes a due binding for one re-promotion by moving its sync_after
	// from observed to leaseUntil, so another replica cannot take it and a crash
	// is retried once the lease passes. claimed is false when sync_after is no
	// longer observed: a newer request moved it, or someone else claimed it.
	ClaimSync(ctx context.Context, id uuid.UUID, observed, leaseUntil time.Time) (claimed bool, err error)

	// FinishSync ends a claimed re-promotion. It only acts while sync_after still
	// equals claimed, so a request that arrived during the promotion (and moved
	// sync_after) is never lost. A nil retryAt clears sync_after; otherwise it is
	// set to retryAt for a later attempt. done is false when the claim was
	// superseded.
	FinishSync(ctx context.Context, id uuid.UUID, claimed time.Time, retryAt *time.Time) (done bool, err error)

	// Defer pushes the next attempt of a PENDING binding to nextRetryAt and records
	// a short user safe note, without consuming an attempt. It never touches a
	// binding another attempt has claimed. Used while a promotion waits for the
	// environment it is promoted from. It returns ErrAgentThunderClientNotFound
	// when no pending binding was updated.
	Defer(ctx context.Context, id uuid.UUID, nextRetryAt time.Time, note string) error

	// ReleaseClaim returns a binding this caller claimed (in progress) to pending,
	// with the next attempt at nextRetryAt and a short user safe note, without
	// consuming an attempt. Used when a promotion needs a user to change
	// something. It returns ErrAgentThunderClientNotFound when the binding is not
	// in progress.
	ReleaseClaim(ctx context.Context, id uuid.UUID, nextRetryAt time.Time, note string) error

	// SetLastError records a short user safe note on a binding without touching
	// its status or attempt bookkeeping. An empty note clears it.
	SetLastError(ctx context.Context, id uuid.UUID, note string) error

	// ListHomeAgents pages through the distinct agents that have a completed
	// binding created directly in an environment (promotion_source is empty),
	// ordered by (org, project, agent). after is nil for the first page.
	ListHomeAgents(ctx context.Context, after *AgentRef, limit int) ([]AgentRef, error)
}

// AgentRef identifies one agent. It doubles as the keyset cursor of ListHomeAgents.
type AgentRef struct {
	OUID             string
	ProjectName      string
	AgentName        string
	ProvisioningType models.AgentProvisioningType
}

// ReconcileCursor identifies the last row returned by a previous
// FindRecentlyCompletedInternal page, so the next call can continue strictly
// after it instead of re-selecting the same oldest page every time.
type ReconcileCursor struct {
	CreatedAt time.Time
	ID        uuid.UUID
}

// AgentThunderAttemptUpdate carries the fields written after one provisioning
// attempt. ThunderAgentID/ThunderClientID/SecretRefPath are pointers so
// "leave the stored value alone" (nil) is explicit and distinct from
// "overwrite it with an empty string" (a non-nil pointer to "").
type AgentThunderAttemptUpdate struct {
	Status          models.AgentThunderStatus
	ThunderAgentID  *string
	ThunderClientID *string
	SecretRefPath   *string
	LastError       string
	NextRetryAt     *time.Time
}

// inProgressStaleThreshold bounds how long a binding may sit in IN_PROGRESS
// before FindDue treats it as an abandoned attempt (e.g. the process crashed
// mid-attempt) and makes it eligible to be claimed again. Comfortably longer
// than a single attempt ever legitimately takes (Thunder's HTTP client timeout
// is 30s), so a live in-flight attempt is never mistakenly reclaimed.
const inProgressStaleThreshold = 5 * time.Minute

// AgentThunderClientRepo implements AgentThunderClientRepository using GORM.
type AgentThunderClientRepo struct {
	db *gorm.DB
}

// NewAgentThunderClientRepo creates a new agent Thunder client repository.
func NewAgentThunderClientRepo(db *gorm.DB) AgentThunderClientRepository {
	return &AgentThunderClientRepo{db: db}
}

func (r *AgentThunderClientRepo) Upsert(ctx context.Context, c *models.AgentThunderClient) error {
	// DoNothing (not DoUpdates): Upsert is only ever used to write-ahead a brand
	// new binding. If a concurrent caller already wrote one for this
	// (org,project,agent,environment) — e.g. the explicit provision endpoint
	// racing PromoteAgent's hook — silently overwriting the winner's row would
	// clobber an already-completed Thunder identity/secret reference back to
	// blank, orphaning it. On conflict, GORM leaves c's fields (including ID)
	// unpopulated from the DB; callers must treat that as "someone else owns
	// this binding" rather than assume c now reflects a real row.
	if err := r.db.WithContext(ctx).Select("*").Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "ou_id"}, {Name: "project_name"}, {Name: "agent_name"}, {Name: "environment_name"},
		},
		DoNothing: true,
	}).Create(c).Error; err != nil {
		return fmt.Errorf("upsert agent thunder client: %w", err)
	}
	return nil
}

func (r *AgentThunderClientRepo) FindByAgent(ctx context.Context, ouID, projectName, agentName string) ([]models.AgentThunderClient, error) {
	var rows []models.AgentThunderClient
	if err := r.db.WithContext(ctx).Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		Order("environment_name").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("find agent thunder clients by agent: %w", err)
	}
	return rows, nil
}

func (r *AgentThunderClientRepo) FindByOuAndEnvironment(ctx context.Context, ouID, environmentName string) ([]models.AgentThunderClient, error) {
	var rows []models.AgentThunderClient
	if err := r.db.WithContext(ctx).
		Where("ou_id = ? AND environment_name = ?", ouID, environmentName).
		Order("project_name asc, agent_name asc").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to list agent thunder bindings for %s/%s: %w", ouID, environmentName, err)
	}
	return rows, nil
}

func (r *AgentThunderClientRepo) Get(ctx context.Context, ouID, projectName, agentName, environmentName string) (*models.AgentThunderClient, error) {
	var c models.AgentThunderClient
	err := r.db.WithContext(ctx).Where("ou_id = ? AND project_name = ? AND agent_name = ? AND environment_name = ?",
		ouID, projectName, agentName, environmentName).First(&c).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrAgentThunderClientNotFound
		}
		return nil, fmt.Errorf("get agent thunder client: %w", err)
	}
	return &c, nil
}

func (r *AgentThunderClientRepo) FindDue(ctx context.Context, now time.Time, limit int) ([]models.AgentThunderClient, error) {
	staleBefore := now.Add(-inProgressStaleThreshold)
	var rows []models.AgentThunderClient
	if err := r.db.WithContext(ctx).Where(
		"(status = ? AND (next_retry_at IS NULL OR next_retry_at <= ?)) OR (status = ? AND last_attempted_at <= ?)",
		models.AgentThunderStatusPending, now,
		models.AgentThunderStatusInProgress, staleBefore,
	).
		Order("created_at").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("find due agent thunder clients: %w", err)
	}
	return rows, nil
}

func (r *AgentThunderClientRepo) ClaimForAttempt(ctx context.Context, id uuid.UUID) (bool, error) {
	staleBefore := time.Now().Add(-inProgressStaleThreshold)
	result := r.db.WithContext(ctx).Model(&models.AgentThunderClient{}).
		Where("id = ? AND (status = ? OR (status = ? AND last_attempted_at <= ?))",
			id, models.AgentThunderStatusPending, models.AgentThunderStatusInProgress, staleBefore).
		Updates(map[string]interface{}{
			"status":            models.AgentThunderStatusInProgress,
			"last_attempted_at": clause.Expr{SQL: "NOW()"},
			"updated_at":        clause.Expr{SQL: "NOW()"},
		})
	if result.Error != nil {
		return false, fmt.Errorf("claim agent thunder client for attempt: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *AgentThunderClientRepo) ResetFailedForRetry(ctx context.Context, id uuid.UUID) (bool, error) {
	result := r.db.WithContext(ctx).Model(&models.AgentThunderClient{}).
		Where("id = ? AND status = ?", id, models.AgentThunderStatusFailed).
		Updates(map[string]interface{}{
			"status":            models.AgentThunderStatusPending,
			"attempt_count":     0,
			"last_error":        "",
			"last_attempted_at": nil,
			"next_retry_at":     clause.Expr{SQL: "NOW()"},
			"updated_at":        clause.Expr{SQL: "NOW()"},
		})
	if result.Error != nil {
		return false, fmt.Errorf("reset failed agent thunder client for retry: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *AgentThunderClientRepo) UpdateAfterAttempt(ctx context.Context, id uuid.UUID, f AgentThunderAttemptUpdate) error {
	updates := map[string]interface{}{
		"status":            f.Status,
		"last_error":        f.LastError,
		"next_retry_at":     f.NextRetryAt,
		"last_attempted_at": clause.Expr{SQL: "NOW()"},
		"attempt_count":     gorm.Expr("attempt_count + 1"),
		"updated_at":        clause.Expr{SQL: "NOW()"},
	}
	if f.ThunderAgentID != nil {
		updates["thunder_agent_id"] = *f.ThunderAgentID
	}
	if f.ThunderClientID != nil {
		updates["thunder_client_id"] = *f.ThunderClientID
	}
	if f.SecretRefPath != nil {
		updates["secret_ref_path"] = *f.SecretRefPath
	}
	if err := r.db.WithContext(ctx).Model(&models.AgentThunderClient{}).Where("id = ?", id).Updates(updates).Error; err != nil {
		return fmt.Errorf("update agent thunder client after attempt: %w", err)
	}
	return nil
}

func (r *AgentThunderClientRepo) UpdateSecretRef(ctx context.Context, id uuid.UUID, secretRefPath string) error {
	if err := r.db.WithContext(ctx).Model(&models.AgentThunderClient{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"secret_ref_path": secretRefPath,
			"updated_at":      clause.Expr{SQL: "NOW()"},
		}).Error; err != nil {
		return fmt.Errorf("update agent thunder client secret ref: %w", err)
	}
	return nil
}

func (r *AgentThunderClientRepo) DeleteByAgent(ctx context.Context, ouID, projectName, agentName string) error {
	if err := r.db.WithContext(ctx).Where("ou_id = ? AND project_name = ? AND agent_name = ?", ouID, projectName, agentName).
		Delete(&models.AgentThunderClient{}).Error; err != nil {
		return fmt.Errorf("delete agent thunder clients by agent: %w", err)
	}
	return nil
}

func (r *AgentThunderClientRepo) DeleteByIDs(ctx context.Context, ids []uuid.UUID) error {
	if len(ids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Where("id IN ?", ids).Delete(&models.AgentThunderClient{}).Error; err != nil {
		return fmt.Errorf("delete agent thunder clients by ids: %w", err)
	}
	return nil
}

func (r *AgentThunderClientRepo) FindRecentlyCompletedInternal(ctx context.Context, createdAfter time.Time, after *ReconcileCursor, limit int) ([]models.AgentThunderClient, error) {
	q := r.db.WithContext(ctx).Where(
		"status = ? AND provisioning_type = ? AND secret_ref_path != '' AND created_at >= ?",
		models.AgentThunderStatusCompleted, models.AgentProvisioningTypeInternal, createdAfter,
	)
	if after != nil {
		// (created_at, id) > (after.CreatedAt, after.ID), expressed without tuple
		// comparison for portability: strictly later timestamp, OR the same
		// timestamp with a strictly greater id (the deterministic tie-breaker
		// matching the ORDER BY below).
		q = q.Where("(created_at > ?) OR (created_at = ? AND id > ?)", after.CreatedAt, after.CreatedAt, after.ID)
	}

	var rows []models.AgentThunderClient
	if err := q.
		Order("created_at, id").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("find recently completed internal agent thunder clients: %w", err)
	}
	return rows, nil
}

func (r *AgentThunderClientRepo) MarkSyncAfter(ctx context.Context, ids []uuid.UUID, at time.Time) error {
	if len(ids) == 0 {
		return nil
	}
	if err := r.db.WithContext(ctx).Model(&models.AgentThunderClient{}).
		Where("id IN ? AND promotion_source <> '' AND status IN ?", ids, []models.AgentThunderStatus{
			models.AgentThunderStatusCompleted, models.AgentThunderStatusInProgress,
		}).
		Updates(map[string]interface{}{
			"sync_after": at,
			"updated_at": clause.Expr{SQL: "NOW()"},
		}).Error; err != nil {
		return fmt.Errorf("mark agent thunder clients for sync: %w", err)
	}
	return nil
}

func (r *AgentThunderClientRepo) FindSyncDue(ctx context.Context, now time.Time, limit int) ([]models.AgentThunderClient, error) {
	var rows []models.AgentThunderClient
	if err := r.db.WithContext(ctx).
		Where("status = ? AND sync_after IS NOT NULL AND sync_after <= ?", models.AgentThunderStatusCompleted, now).
		Order("sync_after, id").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("find agent thunder clients due for sync: %w", err)
	}
	return rows, nil
}

func (r *AgentThunderClientRepo) ClaimSync(ctx context.Context, id uuid.UUID, observed, leaseUntil time.Time) (bool, error) {
	result := r.db.WithContext(ctx).Model(&models.AgentThunderClient{}).
		Where("id = ? AND status = ? AND sync_after = ?", id, models.AgentThunderStatusCompleted, observed).
		Updates(map[string]interface{}{"sync_after": leaseUntil})
	if result.Error != nil {
		return false, fmt.Errorf("claim agent thunder client for sync: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *AgentThunderClientRepo) FinishSync(ctx context.Context, id uuid.UUID, claimed time.Time, retryAt *time.Time) (bool, error) {
	// A nil *time.Time is stored as NULL, which clears the marker.
	result := r.db.WithContext(ctx).Model(&models.AgentThunderClient{}).
		Where("id = ? AND sync_after = ?", id, claimed).
		Updates(map[string]interface{}{"sync_after": retryAt})
	if result.Error != nil {
		return false, fmt.Errorf("finish agent thunder client sync: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

func (r *AgentThunderClientRepo) Defer(ctx context.Context, id uuid.UUID, nextRetryAt time.Time, note string) error {
	return r.setNextAttempt(ctx, id, models.AgentThunderStatusPending, nextRetryAt, note)
}

func (r *AgentThunderClientRepo) ReleaseClaim(ctx context.Context, id uuid.UUID, nextRetryAt time.Time, note string) error {
	return r.setNextAttempt(ctx, id, models.AgentThunderStatusInProgress, nextRetryAt, note)
}

// setNextAttempt moves a binding that is currently in the given status to pending
// with its next attempt and note, leaving attempt_count alone.
func (r *AgentThunderClientRepo) setNextAttempt(ctx context.Context, id uuid.UUID, from models.AgentThunderStatus,
	nextRetryAt time.Time, note string,
) error {
	result := r.db.WithContext(ctx).Model(&models.AgentThunderClient{}).
		Where("id = ? AND status = ?", id, from).
		Updates(map[string]interface{}{
			"status":        models.AgentThunderStatusPending,
			"next_retry_at": nextRetryAt,
			"last_error":    note,
			"updated_at":    clause.Expr{SQL: "NOW()"},
		})
	if result.Error != nil {
		return fmt.Errorf("set next agent thunder client attempt: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrAgentThunderClientNotFound
	}
	return nil
}

func (r *AgentThunderClientRepo) SetLastError(ctx context.Context, id uuid.UUID, note string) error {
	if err := r.db.WithContext(ctx).Model(&models.AgentThunderClient{}).Where("id = ?", id).
		Updates(map[string]interface{}{
			"last_error": note,
			"updated_at": clause.Expr{SQL: "NOW()"},
		}).Error; err != nil {
		return fmt.Errorf("set agent thunder client note: %w", err)
	}
	return nil
}

func (r *AgentThunderClientRepo) ListHomeAgents(ctx context.Context, after *AgentRef, limit int) ([]AgentRef, error) {
	q := r.db.WithContext(ctx).Model(&models.AgentThunderClient{}).
		Select("DISTINCT ON (ou_id, project_name, agent_name) ou_id, project_name, agent_name, provisioning_type").
		Where("status = ? AND promotion_source = ''", models.AgentThunderStatusCompleted)
	if after != nil {
		q = q.Where("(ou_id, project_name, agent_name) > (?, ?, ?)", after.OUID, after.ProjectName, after.AgentName)
	}
	var rows []models.AgentThunderClient
	if err := q.Order("ou_id, project_name, agent_name").Limit(limit).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list agents with a completed identity: %w", err)
	}
	refs := make([]AgentRef, 0, len(rows))
	for _, row := range rows {
		refs = append(refs, AgentRef{
			OUID: row.OUID, ProjectName: row.ProjectName, AgentName: row.AgentName, ProvisioningType: row.ProvisioningType,
		})
	}
	return refs, nil
}
