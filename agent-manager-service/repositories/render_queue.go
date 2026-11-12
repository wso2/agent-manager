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
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
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

// InsertPending places an artifact on a gateway that has no row for it: PENDING, due now. An existing row, an
// UNDEPLOYED one included, is never touched, so a user's undeploy sticks.
func (r *DeploymentRepo) InsertPending(ctx context.Context, tx *gorm.DB, p models.Placement, reason string) (bool, error) {
	now := dbx.Now()
	inserted, err := dbx.InsertIfAbsent(r.on(ctx, tx), &models.DeploymentStatusRecord{
		ArtifactUUID: p.ArtifactUUID, OUID: p.OUID, GatewayUUID: p.GatewayUUID,
		Status: models.DeploymentStatusPending, StatusDesired: models.DeploymentStatusDeployed,
		PerformedAt: models.NewPerformedAt(now), NextAttemptAt: &now, RenderReason: reason, UpdatedAt: now,
	})
	if err != nil {
		return false, fmt.Errorf("failed to place artifact on gateway: %w", err)
	}
	return inserted, nil
}

// QueueRender marks a deploy owed on the artifact's rows in scope, on gateways or, when empty, on every gateway. It
// changes no status column: the gateway still holds what it was last sent.
func (r *DeploymentRepo) QueueRender(
	ctx context.Context, tx *gorm.DB, ouID string, artifactUUID uuid.UUID, gateways []uuid.UUID, reason string, scope QueueScope,
) (int64, error) {
	query := r.on(ctx, tx).Model(&models.DeploymentStatusRecord{}).
		Where("artifact_id = ? AND org_id = ?", artifactUUID, ouID)
	if len(gateways) > 0 {
		query = query.Where("gateway_id IN ?", gateways)
	}
	if scope == QueueScopePlaced {
		query = query.Where("status_desired = ?", models.DeploymentStatusDeployed)
	}
	now := dbx.Now()
	result := query.Updates(map[string]any{
		"next_attempt_at": now, "attempt_count": 0, "render_reason": reason, "updated_at": now,
	})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to queue render: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// ClaimDueRenders leases up to limit rows due at now, pushing each one's next_attempt_at to now + lease. That value
// is the claim token. Rows another claimer took first are skipped.
func (r *DeploymentRepo) ClaimDueRenders(ctx context.Context, now time.Time, lease time.Duration, limit int) ([]models.ClaimedRender, error) {
	return r.claimRenders(ctx, now, lease, "", nil, limit)
}

// ClaimDueRendersOf leases the artifact's due rows on gateways or, when empty, on every gateway: a render owed now
// by the request that queued it.
func (r *DeploymentRepo) ClaimDueRendersOf(
	ctx context.Context, ouID string, artifactUUID uuid.UUID, gateways []uuid.UUID, now time.Time, lease time.Duration,
) ([]models.ClaimedRender, error) {
	condition, args := "artifact_id = ? AND org_id = ?", []any{artifactUUID, ouID}
	if len(gateways) > 0 {
		condition, args = condition+" AND gateway_id IN ?", append(args, gateways)
	}
	return r.claimRenders(ctx, now, lease, condition, args, maxRowsPerArtifact)
}

// maxRowsPerArtifact bounds a claim of one artifact's rows: one per gateway it is placed on.
const maxRowsPerArtifact = 1000

// claimRenders leases due rows matching condition. Each row is claimed with its own conditional update, so of two
// concurrent claimers only the first finds it still due. A row's last delay runs from its last recorded outcome
// (updated_at, which RecordRenderOutcome sets to the attempt's time) to when it fell due.
func (r *DeploymentRepo) claimRenders(
	ctx context.Context, now time.Time, lease time.Duration, condition string, args []any, limit int,
) ([]models.ClaimedRender, error) {
	now = now.UTC().Truncate(time.Microsecond)
	token := now.Add(lease)
	db := r.db.WithContext(ctx)

	query := db.Model(&models.DeploymentStatusRecord{}).Where("next_attempt_at <= ?", now)
	if condition != "" {
		query = query.Where(condition, args...)
	}
	var due []models.DeploymentStatusRecord
	if err := query.Order("next_attempt_at").Limit(limit).Find(&due).Error; err != nil {
		return nil, fmt.Errorf("failed to claim due renders: %w", err)
	}

	claims := make([]models.ClaimedRender, 0, len(due))
	for _, row := range due {
		res := db.Model(&models.DeploymentStatusRecord{}).
			Where(statusKey+" AND next_attempt_at <= ?", row.ArtifactUUID, row.OUID, row.GatewayUUID, now).
			Update("next_attempt_at", token)
		if res.Error != nil {
			return nil, fmt.Errorf("failed to claim due renders: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			continue
		}
		var artifact models.Artifact
		if err := db.Select("kind, handle").Where("id = ?", row.ArtifactUUID).Take(&artifact).Error; err != nil {
			return nil, fmt.Errorf("failed to read claimed render's artifact: %w", err)
		}
		lastDelay := time.Duration(0)
		if row.NextAttemptAt != nil && row.NextAttemptAt.After(row.UpdatedAt) {
			lastDelay = row.NextAttemptAt.Sub(row.UpdatedAt).Truncate(time.Millisecond)
		}
		claims = append(claims, models.ClaimedRender{
			Placement: models.Placement{
				ArtifactRef: models.ArtifactRef{OUID: row.OUID, ArtifactUUID: row.ArtifactUUID, Kind: artifact.Kind, Handle: artifact.Handle},
				GatewayUUID: row.GatewayUUID,
			},
			Token:        token,
			AttemptCount: row.AttemptCount,
			ClaimedAt:    now,
			LastDelay:    lastDelay,
		})
	}
	return claims, nil
}

// lockClaim locks the claimed row in tx while it still holds the claim's token.
func (r *DeploymentRepo) lockClaim(ctx context.Context, tx *gorm.DB, claim models.ClaimedRender) error {
	held, err := dbx.LockRows(r.on(ctx, tx), models.DeploymentStatusRecord{}.TableName(), "status",
		statusKey+" AND next_attempt_at = ?", claim.ArtifactUUID, claim.OUID, claim.GatewayUUID, claim.Token.UTC())
	if err != nil {
		return fmt.Errorf("failed to lock claimed render: %w", err)
	}
	if held == 0 {
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
	updates := map[string]any{"render_reason": outcome.Reason, "updated_at": claim.ClaimedAt.UTC()}
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
		Where(statusKey+" AND next_attempt_at = ?", claim.ArtifactUUID, claim.OUID, claim.GatewayUUID, claim.Token.UTC()).
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
	query := r.db.WithContext(ctx).Table("artifact_gateway_status s").
		Select("s.artifact_id, s.org_id, s.gateway_id, a.kind, a.handle").
		Joins("INNER JOIN artifacts a ON a.id = s.artifact_id")
	if slices.Contains(models.AgentArtifactKinds, kind) {
		query = query.Joins("INNER JOIN agent_exposures e ON e.artifact_id = s.artifact_id")
	}
	query = query.Where("a.kind = ? AND s.status = ? AND s.next_attempt_at IS NULL", kind, models.DeploymentStatusDeployed)
	if artifact, gateway, ok := strings.Cut(afterKey, "/"); ok {
		query = dbx.After(query, []string{"s.artifact_id", "s.gateway_id"}, []any{artifact, gateway})
	}
	var rows []struct {
		ArtifactUUID uuid.UUID `gorm:"column:artifact_id"`
		OUID         string    `gorm:"column:org_id"`
		GatewayUUID  uuid.UUID `gorm:"column:gateway_id"`
		Kind         string    `gorm:"column:kind"`
		Handle       string    `gorm:"column:handle"`
	}
	err := query.Order("s.artifact_id, s.gateway_id").Limit(limit).Scan(&rows).Error
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

// DriftScanLock names the drift scan's row in the locks table.
const DriftScanLock = "gateway.drift_scan"

// driftScanLease bounds how long a replica that died mid-scan keeps the scan from the others.
const driftScanLease = 10 * time.Minute

// TryLockDriftScan claims this interval's drift scan for owner until UnlockDriftScan or the lease runs out. false
// means another worker holds it.
func TryLockDriftScan(ctx context.Context, db *gorm.DB, owner string) (bool, error) {
	locked, err := dbx.TryLock(ctx, db, DriftScanLock, owner, dbx.Now(), driftScanLease)
	if err != nil {
		return false, fmt.Errorf("failed to try the drift scan lock: %w", err)
	}
	return locked, nil
}

// UnlockDriftScan releases the scan owner claimed.
func UnlockDriftScan(ctx context.Context, db *gorm.DB, owner string) error {
	if err := dbx.Unlock(ctx, db, DriftScanLock, owner); err != nil {
		return fmt.Errorf("failed to release the drift scan lock: %w", err)
	}
	return nil
}
