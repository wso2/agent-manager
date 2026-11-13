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
	"log/slog"
	"sort"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// deploymentHardLimit is how many deployments an artifact keeps per gateway before the oldest archived ones go.
const deploymentHardLimit = 25

// DeploymentTimeoutReason is the status_reason of a transition the gateway never acked.
const DeploymentTimeoutReason = "DEPLOYMENT_TIMEOUT"

// AfterLiveFunc runs in ApplyAck's transaction when a row moves to DEPLOYED. It must only touch the database.
type AfterLiveFunc func(ctx context.Context, tx *gorm.DB, placement models.Placement, deploymentID uuid.UUID) error

// on returns tx, or the repository's own connection when tx is nil, bound to ctx.
func (r *DeploymentRepo) on(ctx context.Context, tx *gorm.DB) *gorm.DB {
	if tx == nil {
		tx = r.db
	}
	return tx.WithContext(ctx)
}

// StartDeploy records a new deployment of the placement and points its status row at it, DEPLOYING.
func (r *DeploymentRepo) StartDeploy(ctx context.Context, tx *gorm.DB, p models.Placement, deployment *models.Deployment) error {
	deployment.ArtifactUUID, deployment.OUID, deployment.GatewayUUID = p.ArtifactUUID, p.OUID, p.GatewayUUID
	if err := r.WithTx(r.on(ctx, tx)).CreateWithLimitEnforcement(deployment, deploymentHardLimit); err != nil {
		return fmt.Errorf("failed to create deployment: %w", err)
	}
	performedAt, err := r.upsertStatus(ctx, tx, p, deployment.DeploymentID)
	if err != nil {
		return err
	}
	deployment.SetStatusRow(&models.DeploymentStatusRecord{
		Status: models.DeploymentStatusDeploying, StatusDesired: models.DeploymentStatusDeployed,
	})
	deployment.UpdatedAt = &performedAt
	return nil
}

// StartRestore points the placement's status row at an existing deployment, DEPLOYING. Clearing the queue keeps a
// waiting redeploy from overwriting the restore.
func (r *DeploymentRepo) StartRestore(ctx context.Context, tx *gorm.DB, p models.Placement, deploymentID uuid.UUID) error {
	_, err := r.upsertStatus(ctx, tx, p, deploymentID)
	return err
}

// statusKey is the condition naming one placement's status row.
const statusKey = "artifact_id = ? AND org_id = ? AND gateway_id = ?"

// upsertStatus starts a deploy transition. With BeginUndeploy, it is one of the two writers that reset the queue.
func (r *DeploymentRepo) upsertStatus(ctx context.Context, tx *gorm.DB, p models.Placement, deploymentID uuid.UUID) (time.Time, error) {
	performedAt := models.NewPerformedAt(time.Now())
	now := dbx.Now()
	row := &models.DeploymentStatusRecord{
		ArtifactUUID: p.ArtifactUUID, OUID: p.OUID, GatewayUUID: p.GatewayUUID, DeploymentID: &deploymentID,
		Status: models.DeploymentStatusDeploying, StatusDesired: models.DeploymentStatusDeployed,
		PerformedAt: performedAt, UpdatedAt: now,
	}
	_, err := dbx.Upsert(r.on(ctx, tx), row, func(db *gorm.DB) error {
		return db.Model(&models.DeploymentStatusRecord{}).Where(statusKey, p.ArtifactUUID, p.OUID, p.GatewayUUID).
			Updates(map[string]any{
				"deployment_id": deploymentID, "status": models.DeploymentStatusDeploying,
				"status_desired": models.DeploymentStatusDeployed, "status_reason": "", "performed_at": performedAt,
				"next_attempt_at": nil, "attempt_count": 0, "render_reason": "", "claimed_by": nil, "claimed_until": nil,
				"updated_at": now,
			}).Error
	})
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to start deploy: %w", err)
	}
	return performedAt, nil
}

// LockStatusRows locks the artifacts' existing status rows in tx, in gateway, then artifact order. A transaction
// deploying several artifacts takes them all before its first event locks a gateway, the order a render takes.
func (r *DeploymentRepo) LockStatusRows(ctx context.Context, tx *gorm.DB, ouID string, artifactUUIDs []uuid.UUID) error {
	if len(artifactUUIDs) == 0 {
		return nil
	}
	db := r.on(ctx, tx)
	var keys []struct {
		ArtifactID uuid.UUID
		GatewayID  uuid.UUID
	}
	for _, chunk := range dbx.Chunks(artifactUUIDs) {
		var part []struct {
			ArtifactID uuid.UUID
			GatewayID  uuid.UUID
		}
		if err := db.Model(&models.DeploymentStatusRecord{}).Select("artifact_id, gateway_id").
			Where("org_id = ? AND artifact_id IN ?", ouID, chunk).Scan(&part).Error; err != nil {
			return fmt.Errorf("failed to lock status rows: %w", err)
		}
		keys = append(keys, part...)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].GatewayID != keys[j].GatewayID {
			return keys[i].GatewayID.String() < keys[j].GatewayID.String()
		}
		return keys[i].ArtifactID.String() < keys[j].ArtifactID.String()
	})
	for _, k := range keys {
		if _, err := dbx.LockRows(db, models.DeploymentStatusRecord{}.TableName(), "status",
			"artifact_id = ? AND gateway_id = ?", k.ArtifactID, k.GatewayID); err != nil {
			return fmt.Errorf("failed to lock status rows: %w", err)
		}
	}
	return nil
}

// LockEnvironmentMapping locks the gateway's mapping to the environment until tx commits; false when there is no
// mapping.
func (r *DeploymentRepo) LockEnvironmentMapping(ctx context.Context, tx *gorm.DB, gatewayUUID, environmentUUID uuid.UUID) (bool, error) {
	n, err := dbx.LockRows(r.on(ctx, tx), "gateway_environments", "gateway_id",
		"gateway_id = ? AND environment_id = ?", gatewayUUID, environmentUUID)
	if err != nil {
		return false, fmt.Errorf("failed to lock gateway %s's mapping to environment %s: %w", gatewayUUID, environmentUUID, err)
	}
	return n > 0, nil
}

// finishedUndeploy is the condition, and its argument, on status rows of an undeploy the user still wants: UNDEPLOYED
// with no deploy owed since.
func finishedUndeploy() (string, models.DeploymentStatus) {
	return "status = ? AND next_attempt_at IS NULL", models.DeploymentStatusUndeployed
}

// BeginUndeploy starts an undeploy transition. A row never sent to the gateway (PENDING) is undeployed at once and
// needs no event; any other row heading for DEPLOYED goes UNDEPLOYING and the caller sends the undeploy event. A failed
// undeploy (FAILED and heading for UNDEPLOYED) is re-sent the same way. A row already heading for UNDEPLOYED with a
// deploy owed only loses its queue, with no event: the undeploy supersedes the owed deploy.
func (r *DeploymentRepo) BeginUndeploy(ctx context.Context, tx *gorm.DB, p models.Placement) (bool, error) {
	db := r.on(ctx, tx)
	now := dbx.Now()
	started := db.Model(&models.DeploymentStatusRecord{}).
		Where(statusKey, p.ArtifactUUID, p.OUID, p.GatewayUUID).
		Where("status_desired = ? OR (status_desired = ? AND status = ?)",
			models.DeploymentStatusDeployed, models.DeploymentStatusUndeployed, models.DeploymentStatusFailed).
		Updates(map[string]any{
			"status": gorm.Expr("CASE WHEN status = ? THEN ? ELSE ? END",
				models.DeploymentStatusPending, models.DeploymentStatusUndeployed, models.DeploymentStatusUndeploying),
			"status_desired": models.DeploymentStatusUndeployed, "status_reason": "",
			"performed_at": models.NewPerformedAt(time.Now()), "next_attempt_at": nil, "attempt_count": 0,
			"render_reason": "", "claimed_by": nil, "claimed_until": nil, "updated_at": now,
		})
	if started.Error != nil {
		return false, fmt.Errorf("failed to begin undeploy: %w", started.Error)
	}
	if started.RowsAffected > 0 {
		var row models.DeploymentStatusRecord
		if err := db.Select("status").Where(statusKey, p.ArtifactUUID, p.OUID, p.GatewayUUID).Take(&row).Error; err != nil {
			return false, fmt.Errorf("failed to read the undeploy status: %w", err)
		}
		return row.Status == models.DeploymentStatusUndeploying, nil
	}
	cancelled := db.Model(&models.DeploymentStatusRecord{}).
		Where(statusKey+" AND status_desired = ? AND next_attempt_at IS NOT NULL",
			p.ArtifactUUID, p.OUID, p.GatewayUUID, models.DeploymentStatusUndeployed).
		Updates(map[string]any{
			"next_attempt_at": nil, "attempt_count": 0, "render_reason": "", "claimed_by": nil, "claimed_until": nil,
			"updated_at": now,
		})
	if cancelled.Error != nil {
		return false, fmt.Errorf("failed to cancel the owed deploy: %w", cancelled.Error)
	}
	if cancelled.RowsAffected == 0 {
		return false, utils.ErrDeploymentNotActive
	}
	return false, nil
}

// ackedRow is a status row an ack changed, joined to its artifact.
type ackedRow struct {
	ArtifactUUID uuid.UUID `gorm:"column:artifact_id"`
	OUID         string    `gorm:"column:org_id"`
	Kind         string    `gorm:"column:kind"`
	Handle       string    `gorm:"column:handle"`
	DeploymentID uuid.UUID `gorm:"column:deployment_id"`
}

// ApplyAck settles a transition from the gateway's ack and reports whether the row changed.
//
// A deploy ack applies to the row that still points at its deployment and still heads for DEPLOYED, whatever its
// status, so a late ack lands on a row the sweeper timed out. An undeploy ack carries no deployment id and applies
// to the artifact on this gateway while its row heads for UNDEPLOYED. afterLive runs in a savepoint when the row
// moves to DEPLOYED: its error is logged and rolled back, and the ack still commits, because the gateway never
// re-sends an ack.
func (r *DeploymentRepo) ApplyAck(ctx context.Context, gatewayID string, ack models.DeploymentAck, afterLive AfterLiveFunc) (bool, error) {
	gatewayUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return false, fmt.Errorf("invalid gateway id %q: %w", gatewayID, err)
	}
	status, ok := ackedStatus(ack)
	if !ok {
		return false, nil
	}
	target, targetArgs, ok := ackTarget(ack)
	if !ok {
		return false, nil
	}
	reason := ""
	if status == models.DeploymentStatusFailed {
		reason = ack.ErrorCode
	}

	var acked []ackedRow
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Find the row the ack is for, then change it only if it still differs, so a repeated ack changes nothing.
		var candidates []ackedRow
		args := append([]any{gatewayUUID}, targetArgs...)
		if err := tx.Table("artifact_gateway_status s").
			Select("s.artifact_id, s.org_id, a.kind, a.handle, s.deployment_id").
			Joins("JOIN artifacts a ON a.id = s.artifact_id").
			Where("s.gateway_id = ? AND "+target, args...).
			Scan(&candidates).Error; err != nil {
			return err
		}
		for _, c := range candidates {
			res := tx.Model(&models.DeploymentStatusRecord{}).
				Where("artifact_id = ? AND gateway_id = ? AND (status <> ? OR status_reason <> ? OR status_reason IS NULL)",
					c.ArtifactUUID, gatewayUUID, status, reason).
				Updates(map[string]any{"status": status, "status_reason": reason, "updated_at": dbx.Now()})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				acked = append(acked, c)
			}
		}
		if len(acked) == 0 || status != models.DeploymentStatusDeployed || afterLive == nil {
			return nil
		}
		return runAfterLive(ctx, tx, gatewayUUID, acked[0], afterLive)
	})
	if err != nil {
		return false, fmt.Errorf("failed to apply deployment ack: %w", err)
	}
	return len(acked) > 0, nil
}

// ackedStatus is the status an ack settles its row in.
func ackedStatus(ack models.DeploymentAck) (models.DeploymentStatus, bool) {
	if ack.Action != models.DeploymentAckActionDeploy && ack.Action != models.DeploymentAckActionUndeploy {
		return "", false
	}
	switch ack.Status {
	case models.DeploymentAckStatusFailed:
		return models.DeploymentStatusFailed, true
	case models.DeploymentAckStatusSuccess:
		if ack.Action == models.DeploymentAckActionDeploy {
			return models.DeploymentStatusDeployed, true
		}
		return models.DeploymentStatusUndeployed, true
	default:
		return "", false
	}
}

// ackTarget is the SQL condition (over artifact_gateway_status s and artifacts a) selecting the row an ack is for. The
// gateway names an LLM proxy by its handle, and every other artifact by its UUID.
func ackTarget(ack models.DeploymentAck) (string, []any, bool) {
	if ack.Action == models.DeploymentAckActionDeploy {
		deploymentID, err := uuid.Parse(ack.DeploymentID)
		if err != nil {
			return "", nil, false
		}
		return "s.deployment_id = ? AND s.status_desired = ?", []any{deploymentID, models.DeploymentStatusDeployed}, true
	}
	if ack.ArtifactID == "" {
		return "", nil, false
	}
	// A row with no deployment was never sent to the gateway, so no ack can be for it.
	undeploying := "s.status_desired = ? AND s.deployment_id IS NOT NULL AND "
	byHandle := "(a.kind = ? AND a.handle = ?)"
	args := []any{models.DeploymentStatusUndeployed}
	if artifactUUID, err := uuid.Parse(ack.ArtifactID); err == nil {
		return undeploying + "(a.id = ? OR " + byHandle + ")",
			append(args, artifactUUID, models.KindLLMProxy, ack.ArtifactID), true
	}
	return undeploying + byHandle, append(args, models.KindLLMProxy, ack.ArtifactID), true
}

func runAfterLive(ctx context.Context, tx *gorm.DB, gatewayUUID uuid.UUID, row ackedRow, afterLive AfterLiveFunc) error {
	const savepoint = "after_live"
	if err := tx.SavePoint(savepoint).Error; err != nil {
		return err
	}
	placement := models.Placement{
		ArtifactRef: models.ArtifactRef{OUID: row.OUID, ArtifactUUID: row.ArtifactUUID, Kind: row.Kind, Handle: row.Handle},
		GatewayUUID: gatewayUUID,
	}
	if err := afterLive(ctx, tx, placement, row.DeploymentID); err != nil {
		slog.Error("Deployment went live but its after-live hook failed; the hook's writes are rolled back",
			"artifactUUID", row.ArtifactUUID, "gatewayUUID", gatewayUUID, "deploymentID", row.DeploymentID, "error", err)
		return tx.RollbackTo(savepoint).Error
	}
	return nil
}

// FailStaleTransitional fails every DEPLOYING or UNDEPLOYING row whose transition started before olderThan. It is
// one guarded UPDATE: a row acked meanwhile no longer matches, so several replicas can sweep at once.
func (r *DeploymentRepo) FailStaleTransitional(ctx context.Context, olderThan time.Time) (int64, error) {
	result := r.db.WithContext(ctx).Model(&models.DeploymentStatusRecord{}).
		Where("status IN ? AND performed_at < ?",
			[]models.DeploymentStatus{models.DeploymentStatusDeploying, models.DeploymentStatusUndeploying}, olderThan.UTC()).
		Updates(map[string]any{"status": models.DeploymentStatusFailed, "status_reason": DeploymentTimeoutReason, "updated_at": dbx.Now()})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to time out stale deployments: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// GetPullable returns the deployment the gateway should hold for the artifact: the one its row points at while the
// row heads for DEPLOYED, whatever its status, because the gateway pulls before it acks. Nil when there is none.
func (r *DeploymentRepo) GetPullable(ctx context.Context, ouID string, artifactUUID uuid.UUID, gatewayID string) (*models.Deployment, error) {
	var deployments []models.Deployment
	err := r.db.WithContext(ctx).Table("artifact_gateway_status s").
		Select("d.id, d.name, d.artifact_id, d.org_id, d.gateway_id, "+
			"d.base_deployment_id, d.content, d.metadata, d.created_at, "+
			"s.status, s.updated_at AS status_updated_at").
		Joins("INNER JOIN artifact_deployments d ON d.id = s.deployment_id").
		Where("s.artifact_id = ? AND s.org_id = ? AND s.gateway_id = ? AND s.status_desired = ?",
			artifactUUID, ouID, gatewayID, models.DeploymentStatusDeployed).
		Scan(&deployments).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get the pullable deployment: %w", err)
	}
	if len(deployments) == 0 {
		//nolint:nilnil // no pullable deployment is not an error; callers answer not-found
		return nil, nil
	}
	return &deployments[0], nil
}

// PlacedGateways returns the gateways the artifact is placed on: its row heads for DEPLOYED, in any status.
func (r *DeploymentRepo) PlacedGateways(ctx context.Context, ouID string, artifactUUID uuid.UUID) ([]string, error) {
	return r.gatewaysWhere(ctx, ouID, artifactUUID, "status_desired = ?", models.DeploymentStatusDeployed)
}

// SentGateways returns the gateways the artifact was ever sent, in gateway order: its row names a deployment,
// whatever its status. An undeployed artifact is still on the gateway, so a delete goes to these.
func (r *DeploymentRepo) SentGateways(ctx context.Context, ouID string, artifactUUID uuid.UUID) ([]string, error) {
	return r.gatewaysWhere(ctx, ouID, artifactUUID, "deployment_id IS NOT NULL")
}

func (r *DeploymentRepo) DeleteEventGateways(ctx context.Context, ouID string, artifactUUID uuid.UUID) ([]string, error) {
	sent, err := r.SentGateways(ctx, ouID, artifactUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list the gateways to send the delete to: %w", err)
	}
	var active []uuid.UUID
	if err := r.db.WithContext(ctx).Model(&models.Gateway{}).Where("org_id = ? AND is_active = ?", ouID, true).
		Pluck("id", &active).Error; err != nil {
		return nil, fmt.Errorf("failed to list the gateways to send the delete to: %w", err)
	}
	seen := make(map[string]bool, len(sent)+len(active))
	out := make([]string, 0, len(sent)+len(active))
	for _, id := range sent {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, id := range active {
		if s := id.String(); !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (r *DeploymentRepo) gatewaysWhere(ctx context.Context, ouID string, artifactUUID uuid.UUID, condition string, args ...any) ([]string, error) {
	var gatewayIDs []uuid.UUID
	err := r.db.WithContext(ctx).Model(&models.DeploymentStatusRecord{}).
		Where("artifact_id = ? AND org_id = ?", artifactUUID, ouID).
		Where(condition, args...).
		Order("gateway_id").
		Pluck("gateway_id", &gatewayIDs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list gateways: %w", err)
	}
	out := make([]string, len(gatewayIDs))
	for i, id := range gatewayIDs {
		out[i] = id.String()
	}
	return out, nil
}

func (r *DeploymentRepo) StatusRecords(ctx context.Context, ouID string, artifactUUID uuid.UUID) ([]models.DeploymentStatusRecord, error) {
	var rows []models.DeploymentStatusRecord
	err := r.db.WithContext(ctx).
		Where("artifact_id = ? AND org_id = ?", artifactUUID, ouID).
		Order("gateway_id").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list the artifact's status rows: %w", err)
	}
	return rows, nil
}

// GetStatusRecord returns the placement's status row, or nil when there is none.
func (r *DeploymentRepo) GetStatusRecord(ctx context.Context, p models.Placement) (*models.DeploymentStatusRecord, error) {
	var row models.DeploymentStatusRecord
	err := r.db.WithContext(ctx).
		Where(statusKey, p.ArtifactUUID, p.OUID, p.GatewayUUID).
		First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		//nolint:nilnil // no row is a state of its own: nothing is placed on the gateway
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment status: %w", err)
	}
	return &row, nil
}

// syncRow is a status row the gateway has been sent something for, joined to its artifact.
type syncRow struct {
	ArtifactUUID  uuid.UUID               `gorm:"column:artifact_id"`
	OUID          string                  `gorm:"column:org_id"`
	Kind          string                  `gorm:"column:kind"`
	Handle        string                  `gorm:"column:handle"`
	DeploymentID  uuid.UUID               `gorm:"column:deployment_id"`
	StatusDesired models.DeploymentStatus `gorm:"column:status_desired"`
	PerformedAt   time.Time               `gorm:"column:performed_at"`
}

// ListForGatewaySync lists what the gateway should hold, as it stored it: each row it was ever sent something for,
// in its desired state. A row heading for UNDEPLOYED lists an empty deployment id, because undeploy events carry
// one and the gateway stores it. DeployedAt is the performedAt the gateway was sent. The gateway orders the
// fetches itself, so there is no ORDER BY.
func (r *DeploymentRepo) ListForGatewaySync(ctx context.Context, ouID, gatewayID string) ([]models.GatewaySyncRow, error) {
	var rows []syncRow
	err := r.db.WithContext(ctx).Table("artifact_gateway_status s").
		Select("s.artifact_id, s.org_id, a.kind, a.handle, s.deployment_id, s.status_desired, s.performed_at").
		Joins("INNER JOIN artifacts a ON a.id = s.artifact_id").
		Where("s.gateway_id = ? AND s.org_id = ? AND s.deployment_id IS NOT NULL", gatewayID, ouID).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments for gateway sync: %w", err)
	}
	synced := make([]models.GatewaySyncRow, 0, len(rows))
	for _, row := range rows {
		entry := models.GatewaySyncRow{
			ArtifactRef: models.ArtifactRef{OUID: row.OUID, ArtifactUUID: row.ArtifactUUID, Kind: row.Kind, Handle: row.Handle},
			State:       models.GatewaySyncStateUndeployed,
			DeployedAt:  row.PerformedAt,
		}
		if row.StatusDesired == models.DeploymentStatusDeployed {
			entry.State, entry.DeploymentID = models.GatewaySyncStateDeployed, row.DeploymentID.String()
		}
		synced = append(synced, entry)
	}
	return synced, nil
}

// ListContentForGateway returns the deployments among deploymentIDs that belong to the gateway in ouID, with their
// artifacts. Any other id is left out.
func (r *DeploymentRepo) ListContentForGateway(
	ctx context.Context, ouID, gatewayID string, deploymentIDs []uuid.UUID,
) ([]models.GatewayDeploymentContent, error) {
	if len(deploymentIDs) == 0 {
		return nil, nil
	}
	type contentRow struct {
		ArtifactUUID uuid.UUID `gorm:"column:artifact_id"`
		OUID         string    `gorm:"column:org_id"`
		Kind         string    `gorm:"column:kind"`
		Handle       string    `gorm:"column:handle"`
		DeploymentID uuid.UUID `gorm:"column:id"`
		Content      []byte    `gorm:"column:content"`
	}
	var rows []contentRow
	for _, chunk := range dbx.Chunks(deploymentIDs) {
		var part []contentRow
		err := r.db.WithContext(ctx).Table("artifact_deployments d").
			Select("d.artifact_id, d.org_id, a.kind, a.handle, d.id, d.content").
			Joins("INNER JOIN artifacts a ON a.id = d.artifact_id").
			Where("d.id IN ? AND d.org_id = ? AND d.gateway_id = ?", chunk, ouID, gatewayID).
			Scan(&part).Error
		if err != nil {
			return nil, fmt.Errorf("failed to get deployment content for gateway: %w", err)
		}
		rows = append(rows, part...)
	}
	contents := make([]models.GatewayDeploymentContent, 0, len(rows))
	for _, row := range rows {
		contents = append(contents, models.GatewayDeploymentContent{
			ArtifactRef:  models.ArtifactRef{OUID: row.OUID, ArtifactUUID: row.ArtifactUUID, Kind: row.Kind, Handle: row.Handle},
			DeploymentID: row.DeploymentID,
			Content:      row.Content,
		})
	}
	return contents, nil
}
