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
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// ErrRenderSuperseded is a render write-back whose claim token the row no longer holds: a newer request, a restore
// or an undeploy started since the claim. Acks never supersede a render.
var ErrRenderSuperseded = errors.New("render superseded")

// QueueScope selects which of an artifact's rows QueueRender marks due.
type QueueScope int

const (
	// QueueScopePlaced marks only rows heading for DEPLOYED: an input of a placed artifact changed.
	QueueScopePlaced QueueScope = iota
	// QueueScopeAny marks every row, UNDEPLOYED included: the user asked for a deploy, and the placement pass never
	// re-places a row that exists.
	QueueScopeAny
)

// RenderOutcomeKind is what became of a render attempt that wrote no deployment.
type RenderOutcomeKind int

const (
	// RenderWaiting is an input that isn't there yet. It doesn't count against the attempt budget.
	RenderWaiting RenderOutcomeKind = iota
	// RenderRetry is a failed attempt, counted against the budget.
	RenderRetry
	// RenderGiveUp drops the owed deploy and keeps the reason it was given up.
	RenderGiveUp
)

// RenderOutcome is recorded on a claimed row whose render wrote no deployment. NextAttemptAt is unused by GiveUp.
type RenderOutcome struct {
	Outcome       RenderOutcomeKind
	Reason        string
	NextAttemptAt *time.Time
}

const statusKey = "artifact_uuid = ? AND ou_id = ? AND gateway_uuid = ?"

// InsertPending places an artifact on a gateway that has no row for it: PENDING, due now. An existing row, an
// UNDEPLOYED one included, is never touched, so a user's undeploy sticks.
func (r *DeploymentRepo) InsertPending(ctx context.Context, tx *gorm.DB, p models.Placement, reason string) (bool, error) {
	now := time.Now()
	result := r.on(ctx, tx).Exec(`
		INSERT INTO deployment_status (artifact_uuid, ou_id, gateway_uuid, deployment_id, status, status_desired,
			status_reason, performed_at, next_attempt_at, attempt_count, render_reason, updated_at)
		VALUES (?, ?, ?, NULL, ?, ?, '', ?, ?, 0, ?, NOW())
		ON CONFLICT (artifact_uuid, ou_id, gateway_uuid) DO NOTHING`,
		p.ArtifactUUID, p.OUID, p.GatewayUUID, models.DeploymentStatusPending, models.DeploymentStatusDeployed,
		models.NewPerformedAt(now), now, reason)
	if result.Error != nil {
		return false, fmt.Errorf("failed to place artifact on gateway: %w", result.Error)
	}
	return result.RowsAffected > 0, nil
}

// QueueRender marks a deploy owed on the artifact's rows in scope, on gateways or, when empty, on every gateway. It
// changes no status column: the gateway still holds what it was last sent.
func (r *DeploymentRepo) QueueRender(
	ctx context.Context, tx *gorm.DB, ouID string, artifactUUID uuid.UUID, gateways []uuid.UUID, reason string, scope QueueScope,
) (int64, error) {
	query := r.on(ctx, tx).Model(&models.DeploymentStatusRecord{}).
		Where("artifact_uuid = ? AND ou_id = ?", artifactUUID, ouID)
	if len(gateways) > 0 {
		query = query.Where("gateway_uuid IN ?", gateways)
	}
	if scope == QueueScopePlaced {
		query = query.Where("status_desired = ?", models.DeploymentStatusDeployed)
	}
	result := query.Updates(map[string]any{
		"next_attempt_at": time.Now(), "attempt_count": 0, "render_reason": reason, "updated_at": gorm.Expr("NOW()"),
	})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to queue render: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// ClaimDueRenders leases up to limit rows due at now, pushing each one's next_attempt_at to now + lease. That value
// is the claim token. Rows another claimer holds are skipped.
func (r *DeploymentRepo) ClaimDueRenders(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]models.ClaimedRender, error) {
	return r.claimRenders(ctx, now, lease, "TRUE", nil, limit)
}

// ClaimDueRendersOf leases the artifact's due rows on gateways or, when empty, on every gateway: a render owed now
// by the request that queued it.
func (r *DeploymentRepo) ClaimDueRendersOf(
	ctx context.Context, ouID string, artifactUUID uuid.UUID, gateways []uuid.UUID, now time.Time, lease time.Duration,
) ([]models.ClaimedRender, error) {
	condition, args := "artifact_uuid = ? AND ou_id = ?", []any{artifactUUID, ouID}
	if len(gateways) > 0 {
		condition, args = condition+" AND gateway_uuid IN ?", append(args, gateways)
	}
	return r.claimRenders(ctx, now, lease, condition, args, maxRowsPerArtifact)
}

// maxRowsPerArtifact bounds a claim of one artifact's rows: one per gateway it is placed on.
const maxRowsPerArtifact = 1000

// localTimestamp writes t to a TIMESTAMP column (updated_at) as NOW() does: in the session's zone.
func localTimestamp(t time.Time) any {
	return gorm.Expr("(?::timestamptz AT TIME ZONE current_setting('TimeZone'))", t)
}

type claimedRow struct {
	ArtifactUUID uuid.UUID `gorm:"column:artifact_uuid"`
	OUID         string    `gorm:"column:ou_id"`
	GatewayUUID  uuid.UUID `gorm:"column:gateway_uuid"`
	Kind         string    `gorm:"column:kind"`
	Handle       string    `gorm:"column:handle"`
	Token        time.Time `gorm:"column:token"`
	AttemptCount int       `gorm:"column:attempt_count"`
	LastDelayMS  int64     `gorm:"column:last_delay_ms"`
}

// claimRenders leases due rows matching condition. A row's last delay runs from its last recorded outcome
// (updated_at, which RecordRenderOutcome sets to the attempt's time) to when it fell due. updated_at is a TIMESTAMP
// in the session's zone, read back the same way.
func (r *DeploymentRepo) claimRenders(
	ctx context.Context, now time.Time, lease time.Duration, condition string, args []any, limit int,
) ([]models.ClaimedRender, error) {
	var rows []claimedRow
	err := r.db.WithContext(ctx).Raw(`
		WITH due AS (
			SELECT artifact_uuid, ou_id, gateway_uuid,
				next_attempt_at - (updated_at AT TIME ZONE current_setting('TimeZone')) AS last_delay
			FROM deployment_status
			WHERE next_attempt_at <= ? AND `+condition+`
			ORDER BY next_attempt_at
			LIMIT ?
			FOR UPDATE SKIP LOCKED
		)
		UPDATE deployment_status s SET next_attempt_at = ?
		FROM due, artifacts a
		WHERE s.artifact_uuid = due.artifact_uuid AND s.ou_id = due.ou_id AND s.gateway_uuid = due.gateway_uuid
		  AND a.uuid = s.artifact_uuid AND a.ou_id = s.ou_id
		RETURNING s.artifact_uuid, s.ou_id, s.gateway_uuid, a.kind, a.handle, s.next_attempt_at AS token,
			s.attempt_count, (EXTRACT(EPOCH FROM GREATEST(due.last_delay, INTERVAL '0')) * 1000)::BIGINT AS last_delay_ms`,
		append(append([]any{now}, args...), limit, now.Add(lease))...).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to claim due renders: %w", err)
	}
	claims := make([]models.ClaimedRender, 0, len(rows))
	for _, row := range rows {
		claims = append(claims, models.ClaimedRender{
			Placement: models.Placement{
				ArtifactRef: models.ArtifactRef{OUID: row.OUID, ArtifactUUID: row.ArtifactUUID, Kind: row.Kind, Handle: row.Handle},
				GatewayUUID: row.GatewayUUID,
			},
			Token:        row.Token,
			AttemptCount: row.AttemptCount,
			ClaimedAt:    now,
			LastDelay:    time.Duration(row.LastDelayMS) * time.Millisecond,
		})
	}
	return claims, nil
}

// lockClaim locks the claimed row in tx while it still holds the claim's token.
func (r *DeploymentRepo) lockClaim(ctx context.Context, tx *gorm.DB, claim models.ClaimedRender) error {
	var held []int
	err := r.on(ctx, tx).Raw(`SELECT 1 FROM deployment_status WHERE `+statusKey+` AND next_attempt_at = ? FOR UPDATE`,
		claim.ArtifactUUID, claim.OUID, claim.GatewayUUID, claim.Token).Scan(&held).Error
	if err != nil {
		return fmt.Errorf("failed to lock claimed render: %w", err)
	}
	if len(held) == 0 {
		return ErrRenderSuperseded
	}
	return nil
}

// CompleteRender writes the rendered deployment as StartDeploy does, while the row still holds the claim's token.
func (r *DeploymentRepo) CompleteRender(ctx context.Context, tx *gorm.DB, claim models.ClaimedRender, deployment *models.Deployment) error {
	if err := r.lockClaim(ctx, tx, claim); err != nil {
		return err
	}
	return r.StartDeploy(ctx, tx, claim.Placement, deployment)
}

// RecordRenderOutcome records an attempt that wrote no deployment, while the row still holds the claim's token.
func (r *DeploymentRepo) RecordRenderOutcome(ctx context.Context, claim models.ClaimedRender, outcome RenderOutcome) error {
	updates := map[string]any{"render_reason": outcome.Reason, "updated_at": localTimestamp(claim.ClaimedAt)}
	switch outcome.Outcome {
	case RenderWaiting:
		updates["next_attempt_at"] = outcome.NextAttemptAt
	case RenderRetry:
		updates["next_attempt_at"] = outcome.NextAttemptAt
		updates["attempt_count"] = gorm.Expr("attempt_count + 1")
	case RenderGiveUp:
		updates["next_attempt_at"] = nil
	}
	result := r.db.WithContext(ctx).Model(&models.DeploymentStatusRecord{}).
		Where(statusKey+" AND next_attempt_at = ?", claim.ArtifactUUID, claim.OUID, claim.GatewayUUID, claim.Token).
		Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to record render outcome: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return ErrRenderSuperseded
	}
	return nil
}

// AbandonRender undeploys a claimed row whose per-environment record is gone, as BeginUndeploy does, while the row
// still holds the claim's token. BeginUndeploy documents how a row owed a deploy while UNDEPLOYED is handled.
func (r *DeploymentRepo) AbandonRender(ctx context.Context, tx *gorm.DB, claim models.ClaimedRender) (bool, error) {
	if err := r.lockClaim(ctx, tx, claim); err != nil {
		return false, err
	}
	return r.BeginUndeploy(ctx, tx, claim.Placement)
}

// DriftKey is a placement's position in ListDeployedForDrift's order: pass the last one listed as afterKey.
func DriftKey(p models.Placement) string {
	return p.ArtifactUUID.String() + "/" + p.GatewayUUID.String()
}

// ListDeployedForDrift pages through the DEPLOYED rows of an artifact kind with no render owed, after afterKey
// (DriftKey) in key order. Agent rows are only platform-hosted agents: drift is a moved release binding, which
// external agents don't have.
func (r *DeploymentRepo) ListDeployedForDrift(ctx context.Context, kind string, afterKey string, limit int) ([]models.Placement, error) {
	query := r.db.WithContext(ctx).Table("deployment_status s").
		Select("s.artifact_uuid, s.ou_id, s.gateway_uuid, a.kind, a.handle").
		Joins("INNER JOIN artifacts a ON a.uuid = s.artifact_uuid AND a.ou_id = s.ou_id")
	if slices.Contains(models.AgentArtifactKinds, kind) {
		query = query.Joins("INNER JOIN platform_agent_exposures e ON e.artifact_uuid = s.artifact_uuid")
	}
	var rows []struct {
		ArtifactUUID uuid.UUID `gorm:"column:artifact_uuid"`
		OUID         string    `gorm:"column:ou_id"`
		GatewayUUID  uuid.UUID `gorm:"column:gateway_uuid"`
		Kind         string    `gorm:"column:kind"`
		Handle       string    `gorm:"column:handle"`
	}
	const key = "s.artifact_uuid::text || '/' || s.gateway_uuid::text"
	err := query.
		Where("a.kind = ? AND s.status = ? AND s.next_attempt_at IS NULL AND "+key+" > ?",
			kind, models.DeploymentStatusDeployed, afterKey).
		Order(key).Limit(limit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list deployed rows for drift: %w", err)
	}
	placements := make([]models.Placement, 0, len(rows))
	for _, row := range rows {
		placements = append(placements, models.Placement{
			ArtifactRef: models.ArtifactRef{OUID: row.OUID, ArtifactUUID: row.ArtifactUUID, Kind: row.Kind, Handle: row.Handle},
			GatewayUUID: row.GatewayUUID,
		})
	}
	return placements, nil
}

// DriftScanLockKey is the drift scan's advisory lock key within DriftScanLockNamespace.
const DriftScanLockKey = "drift"

// TryLockDriftScan claims this interval's drift scan on conn's session, until UnlockDriftScan or the
// connection closes. false means another replica holds it. conn must be pinned with gorm.DB.Connection.
func TryLockDriftScan(conn *gorm.DB) (bool, error) {
	var locked bool
	if err := conn.Raw("SELECT pg_try_advisory_lock(?, hashtext(?))", DriftScanLockNamespace, DriftScanLockKey).
		Scan(&locked).Error; err != nil {
		return false, fmt.Errorf("failed to try the drift scan lock: %w", err)
	}
	return locked, nil
}

// UnlockDriftScan releases the lock TryLockDriftScan took, on the same pinned connection. An error means the
// lock may still be held by that connection's session.
func UnlockDriftScan(conn *gorm.DB) error {
	var released bool
	if err := conn.Raw("SELECT pg_advisory_unlock(?, hashtext(?))", DriftScanLockNamespace, DriftScanLockKey).
		Scan(&released).Error; err != nil {
		return fmt.Errorf("failed to release the drift scan lock: %w", err)
	}
	if !released {
		return errors.New("the drift scan lock was not held by this connection")
	}
	return nil
}
