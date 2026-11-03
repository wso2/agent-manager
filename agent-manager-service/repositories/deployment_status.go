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
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

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

// upsertStatus starts a deploy transition. With BeginUndeploy, it is one of the two writers that reset the queue.
func (r *DeploymentRepo) upsertStatus(ctx context.Context, tx *gorm.DB, p models.Placement, deploymentID uuid.UUID) (time.Time, error) {
	performedAt := models.NewPerformedAt(time.Now())
	err := r.on(ctx, tx).Exec(`
		INSERT INTO deployment_status (artifact_uuid, ou_id, gateway_uuid, deployment_id, status, status_desired,
			status_reason, performed_at, next_attempt_at, attempt_count, render_reason, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, '', ?, NULL, 0, '', NOW())
		ON CONFLICT (artifact_uuid, ou_id, gateway_uuid) DO UPDATE SET
			deployment_id = EXCLUDED.deployment_id, status = EXCLUDED.status, status_desired = EXCLUDED.status_desired,
			status_reason = '', performed_at = EXCLUDED.performed_at,
			next_attempt_at = NULL, attempt_count = 0, render_reason = '', updated_at = NOW()`,
		p.ArtifactUUID, p.OUID, p.GatewayUUID, deploymentID,
		models.DeploymentStatusDeploying, models.DeploymentStatusDeployed, performedAt).Error
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
	var locked []uuid.UUID
	if err := r.on(ctx, tx).Raw(`SELECT artifact_uuid FROM deployment_status
		WHERE ou_id = ? AND artifact_uuid IN ?
		ORDER BY gateway_uuid, artifact_uuid
		FOR UPDATE`, ouID, artifactUUIDs).Scan(&locked).Error; err != nil {
		return fmt.Errorf("failed to lock status rows: %w", err)
	}
	return nil
}

// LockEnvironmentMapping holds the gateway's mapping to the environment FOR SHARE until tx commits; false when there
// is no mapping.
func (r *DeploymentRepo) LockEnvironmentMapping(ctx context.Context, tx *gorm.DB, gatewayUUID, environmentUUID uuid.UUID) (bool, error) {
	var mapped []int
	if err := r.on(ctx, tx).Raw(`SELECT 1 FROM gateway_environment_mappings
		WHERE gateway_uuid = ? AND environment_uuid = ?
		FOR SHARE`, gatewayUUID, environmentUUID).Scan(&mapped).Error; err != nil {
		return false, fmt.Errorf("failed to lock gateway %s's mapping to environment %s: %w", gatewayUUID, environmentUUID, err)
	}
	return len(mapped) > 0, nil
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
	var statuses []models.DeploymentStatus
	err := r.on(ctx, tx).Raw(`
		UPDATE deployment_status SET
			status = CASE WHEN status = ? THEN ? ELSE ? END,
			status_desired = ?, status_reason = '', performed_at = ?,
			next_attempt_at = NULL, attempt_count = 0, render_reason = '', updated_at = NOW()
		WHERE artifact_uuid = ? AND ou_id = ? AND gateway_uuid = ?
		  AND (status_desired = ? OR (status_desired = ? AND status = ?))
		RETURNING status`,
		models.DeploymentStatusPending, models.DeploymentStatusUndeployed, models.DeploymentStatusUndeploying,
		models.DeploymentStatusUndeployed, models.NewPerformedAt(time.Now()),
		p.ArtifactUUID, p.OUID, p.GatewayUUID,
		models.DeploymentStatusDeployed, models.DeploymentStatusUndeployed, models.DeploymentStatusFailed).
		Scan(&statuses).Error
	if err != nil {
		return false, fmt.Errorf("failed to begin undeploy: %w", err)
	}
	if len(statuses) > 0 {
		return statuses[0] == models.DeploymentStatusUndeploying, nil
	}
	cancelled := r.on(ctx, tx).Exec(`
		UPDATE deployment_status SET next_attempt_at = NULL, attempt_count = 0, render_reason = '', updated_at = NOW()
		WHERE artifact_uuid = ? AND ou_id = ? AND gateway_uuid = ? AND status_desired = ? AND next_attempt_at IS NOT NULL`,
		p.ArtifactUUID, p.OUID, p.GatewayUUID, models.DeploymentStatusUndeployed)
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
	ArtifactUUID uuid.UUID `gorm:"column:artifact_uuid"`
	OUID         string    `gorm:"column:ou_id"`
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
		args := append([]any{status, reason, gatewayUUID, status, reason}, targetArgs...)
		if err := tx.Raw(`
			UPDATE deployment_status s SET status = ?, status_reason = ?, updated_at = NOW()
			FROM artifacts a
			WHERE a.uuid = s.artifact_uuid AND a.ou_id = s.ou_id AND s.gateway_uuid = ?
			  AND (s.status <> ? OR s.status_reason <> ?)
			  AND `+target+`
			RETURNING s.artifact_uuid, s.ou_id, a.kind, a.handle, s.deployment_id`, args...).
			Scan(&acked).Error; err != nil {
			return err
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

// ackTarget is the SQL condition (over deployment_status s and artifacts a) selecting the row an ack is for. The
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
		return undeploying + "(a.uuid = ? OR " + byHandle + ")",
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
	result := r.db.WithContext(ctx).Exec(`
		UPDATE deployment_status SET status = ?, status_reason = ?, updated_at = NOW()
		WHERE status IN (?, ?) AND performed_at < ?`,
		models.DeploymentStatusFailed, DeploymentTimeoutReason,
		models.DeploymentStatusDeploying, models.DeploymentStatusUndeploying, olderThan)
	if result.Error != nil {
		return 0, fmt.Errorf("failed to time out stale deployments: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// GetPullable returns the deployment the gateway should hold for the artifact: the one its row points at while the
// row heads for DEPLOYED, whatever its status, because the gateway pulls before it acks. Nil when there is none.
func (r *DeploymentRepo) GetPullable(ctx context.Context, ouID string, artifactUUID uuid.UUID, gatewayID string) (*models.Deployment, error) {
	var deployments []models.Deployment
	err := r.db.WithContext(ctx).Table("deployment_status s").
		Select("d.deployment_id, d.name, d.artifact_uuid, d.ou_id, d.gateway_uuid, "+
			"d.base_deployment_id, d.content, d.metadata, d.created_at, "+
			"s.status, s.updated_at AS status_updated_at").
		Joins("INNER JOIN deployments d ON d.deployment_id = s.deployment_id").
		Where("s.artifact_uuid = ? AND s.ou_id = ? AND s.gateway_uuid = ? AND s.status_desired = ?",
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
	var gatewayUUIDs []string
	err := r.db.WithContext(ctx).Raw(`SELECT gateway_uuid::text FROM deployment_status
		WHERE artifact_uuid = ? AND ou_id = ? AND deployment_id IS NOT NULL
		UNION
		SELECT uuid::text FROM gateways WHERE ou_id = ? AND is_active AND deleted_at IS NULL
		ORDER BY 1`, artifactUUID, ouID, ouID).Scan(&gatewayUUIDs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list the gateways to send the delete to: %w", err)
	}
	return gatewayUUIDs, nil
}

func (r *DeploymentRepo) gatewaysWhere(ctx context.Context, ouID string, artifactUUID uuid.UUID, condition string, args ...any) ([]string, error) {
	var gatewayUUIDs []string
	err := r.db.WithContext(ctx).Table("deployment_status").
		Where("artifact_uuid = ? AND ou_id = ?", artifactUUID, ouID).
		Where(condition, args...).
		Order("gateway_uuid").
		Pluck("gateway_uuid", &gatewayUUIDs).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list gateways: %w", err)
	}
	return gatewayUUIDs, nil
}

func (r *DeploymentRepo) StatusRecords(ctx context.Context, ouID string, artifactUUID uuid.UUID) ([]models.DeploymentStatusRecord, error) {
	var rows []models.DeploymentStatusRecord
	err := r.db.WithContext(ctx).
		Where("artifact_uuid = ? AND ou_id = ?", artifactUUID, ouID).
		Order("gateway_uuid").
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
		Where("artifact_uuid = ? AND ou_id = ? AND gateway_uuid = ?", p.ArtifactUUID, p.OUID, p.GatewayUUID).
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

// syncRow is a deployment_status row the gateway has been sent something for, joined to its artifact.
type syncRow struct {
	ArtifactUUID  uuid.UUID               `gorm:"column:artifact_uuid"`
	OUID          string                  `gorm:"column:ou_id"`
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
	err := r.db.WithContext(ctx).Raw(`
		SELECT s.artifact_uuid, s.ou_id, a.kind, a.handle, s.deployment_id, s.status_desired, s.performed_at
		FROM deployment_status s
		INNER JOIN artifacts a ON a.uuid = s.artifact_uuid AND a.ou_id = s.ou_id
		WHERE s.gateway_uuid = ? AND s.ou_id = ? AND s.deployment_id IS NOT NULL`,
		gatewayID, ouID).Scan(&rows).Error
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
	var rows []struct {
		ArtifactUUID uuid.UUID `gorm:"column:artifact_uuid"`
		OUID         string    `gorm:"column:ou_id"`
		Kind         string    `gorm:"column:kind"`
		Handle       string    `gorm:"column:handle"`
		DeploymentID uuid.UUID `gorm:"column:deployment_id"`
		Content      []byte    `gorm:"column:content"`
	}
	err := r.db.WithContext(ctx).Raw(`
		SELECT d.artifact_uuid, d.ou_id, a.kind, a.handle, d.deployment_id, d.content
		FROM deployments d
		INNER JOIN artifacts a ON a.uuid = d.artifact_uuid AND a.ou_id = d.ou_id
		WHERE d.deployment_id IN ? AND d.ou_id = ? AND d.gateway_uuid = ?`,
		deploymentIDs, ouID, gatewayID).Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment content for gateway: %w", err)
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
