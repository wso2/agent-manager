//go:build integration

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
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// seedPlacement writes a gateway and an artifact of kind in a fresh org, with no status row.
func seedPlacement(t *testing.T, kind string) models.Placement {
	t.Helper()
	gdb := db.GetDB()
	ouID := "ou-" + uuid.NewString()[:8]
	gatewayID, artifactUUID := uuid.New(), uuid.New()
	name := "status-gw-" + gatewayID.String()[:8]
	require.NoError(t, gdb.Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, name, name, "gw.example", models.GatewayRoleEgress, ouID).Error)
	handle := "handle-" + artifactUUID.String()[:8]
	require.NoError(t, NewArtifactRepo(gdb).Create(gdb, &models.Artifact{
		UUID: artifactUUID, Handle: handle, Name: handle, Version: "v1.0", Kind: kind, OUID: ouID,
	}))
	t.Cleanup(func() {
		gdb.Exec(`DELETE FROM artifacts WHERE uuid = ?`, artifactUUID)
		gdb.Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID)
	})
	return models.Placement{
		ArtifactRef: models.ArtifactRef{OUID: ouID, ArtifactUUID: artifactUUID, Kind: kind, Handle: handle},
		GatewayUUID: gatewayID,
	}
}

func newDeployment(p models.Placement) *models.Deployment {
	return &models.Deployment{
		Name: "deployment", ArtifactUUID: p.ArtifactUUID, OUID: p.OUID, GatewayUUID: p.GatewayUUID,
		Content: []byte("kind: " + p.Kind + "\n"),
	}
}

func startDeploy(t *testing.T, p models.Placement) *models.Deployment {
	t.Helper()
	deployment := newDeployment(p)
	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		return NewDeploymentRepo(db.GetDB()).StartDeploy(context.Background(), tx, p, deployment)
	}))
	return deployment
}

func beginUndeploy(t *testing.T, p models.Placement) (bool, error) {
	t.Helper()
	var sendEvent bool
	err := db.GetDB().Transaction(func(tx *gorm.DB) error {
		var err error
		sendEvent, err = NewDeploymentRepo(db.GetDB()).BeginUndeploy(context.Background(), tx, p)
		return err
	})
	return sendEvent, err
}

func statusRow(t *testing.T, p models.Placement) models.DeploymentStatusRecord {
	t.Helper()
	row, err := NewDeploymentRepo(db.GetDB()).GetStatusRecord(context.Background(), p)
	require.NoError(t, err)
	require.NotNil(t, row)
	return *row
}

// insertPendingRow writes a deploy owed to a gateway that was never sent anything.
func insertPendingRow(t *testing.T, p models.Placement) {
	t.Helper()
	require.NoError(t, db.GetDB().Exec(
		`INSERT INTO deployment_status (artifact_uuid, ou_id, gateway_uuid, status, status_desired, performed_at,
		 next_attempt_at, render_reason, updated_at)
		 VALUES (?, ?, ?, 'PENDING', 'DEPLOYED', NOW(), NOW(), 'waiting', NOW())`,
		p.ArtifactUUID, p.OUID, p.GatewayUUID).Error)
}

// queueRender marks a deploy owed on the row, as the render queue would.
func queueRender(t *testing.T, p models.Placement) {
	t.Helper()
	require.NoError(t, db.GetDB().Exec(
		`UPDATE deployment_status SET next_attempt_at = NOW(), attempt_count = 3, render_reason = 'waiting'
		 WHERE artifact_uuid = ? AND ou_id = ? AND gateway_uuid = ?`,
		p.ArtifactUUID, p.OUID, p.GatewayUUID).Error)
}

func backdate(t *testing.T, p models.Placement, by time.Duration) {
	t.Helper()
	require.NoError(t, db.GetDB().Exec(
		`UPDATE deployment_status SET performed_at = performed_at - make_interval(secs => ?)
		 WHERE artifact_uuid = ? AND ou_id = ? AND gateway_uuid = ?`,
		by.Seconds(), p.ArtifactUUID, p.OUID, p.GatewayUUID).Error)
}

func deployAck(deploymentID uuid.UUID, status, errorCode string) models.DeploymentAck {
	return models.DeploymentAck{
		DeploymentID: deploymentID.String(), Action: models.DeploymentAckActionDeploy, Status: status,
		ErrorCode: errorCode,
	}
}

func undeployAck(artifactID, status string) models.DeploymentAck {
	return models.DeploymentAck{ArtifactID: artifactID, Action: models.DeploymentAckActionUndeploy, Status: status}
}

func applyAck(t *testing.T, p models.Placement, ack models.DeploymentAck, afterLive AfterLiveFunc) bool {
	t.Helper()
	applied, err := NewDeploymentRepo(db.GetDB()).ApplyAck(context.Background(), p.GatewayUUID.String(), ack, afterLive)
	require.NoError(t, err)
	return applied
}

func assertQueueClear(t *testing.T, row models.DeploymentStatusRecord) {
	t.Helper()
	assert.Nil(t, row.NextAttemptAt)
	assert.Zero(t, row.AttemptCount)
	assert.Empty(t, row.RenderReason)
}

func TestStartDeployStartsADeployTransition(t *testing.T) {
	p := seedPlacement(t, models.KindLLMProvider)
	insertPendingRow(t, p)
	before := models.NewPerformedAt(time.Now())

	deployment := startDeploy(t, p)

	row := statusRow(t, p)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
	assert.Equal(t, models.DeploymentStatusDeployed, row.StatusDesired)
	require.NotNil(t, row.DeploymentID)
	assert.Equal(t, deployment.DeploymentID, *row.DeploymentID)
	assert.Empty(t, row.StatusReason)
	assertQueueClear(t, row)
	assert.Equal(t, models.NewPerformedAt(row.PerformedAt), row.PerformedAt.UTC(), "milliseconds")
	assert.False(t, row.PerformedAt.Before(before))

	require.NotNil(t, deployment.Status)
	assert.Equal(t, models.DeploymentStatusDeploying, *deployment.Status)
	require.NotNil(t, deployment.UpdatedAt)
	assert.True(t, row.PerformedAt.Equal(*deployment.UpdatedAt), "the caller's event carries performed_at exactly")
}

func TestStartDeployOverAFailedRowClearsTheReason(t *testing.T) {
	p := seedPlacement(t, models.KindLLMProvider)
	first := startDeploy(t, p)
	applyAck(t, p, deployAck(first.DeploymentID, models.DeploymentAckStatusFailed, "GATEWAY_PROCESSING_ERROR"), nil)
	queueRender(t, p)

	second := startDeploy(t, p)

	row := statusRow(t, p)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
	assert.Equal(t, second.DeploymentID, *row.DeploymentID)
	assert.Empty(t, row.StatusReason)
	assertQueueClear(t, row)
}

// Retention: past the hard limit of 25 the oldest archived deployments go; the current one never does.
func TestStartDeployPrunesTheOldestArchivedDeployments(t *testing.T) {
	p := seedPlacement(t, models.KindLLMProxy)
	var ids []uuid.UUID
	for range 26 {
		ids = append(ids, startDeploy(t, p).DeploymentID)
	}

	var kept []uuid.UUID
	require.NoError(t, db.GetDB().Raw(`SELECT deployment_id FROM deployments WHERE artifact_uuid = ? AND ou_id = ?`,
		p.ArtifactUUID, p.OUID).Scan(&kept).Error)
	assert.Less(t, len(kept), 26)
	assert.NotContains(t, kept, ids[0], "the oldest archived deployment is pruned")
	assert.Contains(t, kept, ids[25])
	assert.Equal(t, ids[25], *statusRow(t, p).DeploymentID)
}

// A restore points the row at an earlier deployment and clears the queue, so a waiting redeploy can't overwrite it.
func TestStartRestore(t *testing.T) {
	p := seedPlacement(t, models.KindLLMProvider)
	first := startDeploy(t, p)
	second := startDeploy(t, p)
	applyAck(t, p, deployAck(second.DeploymentID, models.DeploymentAckStatusFailed, "GATEWAY_PROCESSING_ERROR"), nil)
	queueRender(t, p)
	before := statusRow(t, p).PerformedAt

	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		return NewDeploymentRepo(db.GetDB()).StartRestore(context.Background(), tx, p, first.DeploymentID)
	}))

	row := statusRow(t, p)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
	assert.Equal(t, models.DeploymentStatusDeployed, row.StatusDesired)
	assert.Equal(t, first.DeploymentID, *row.DeploymentID)
	assert.Empty(t, row.StatusReason)
	assertQueueClear(t, row)
	assert.False(t, row.PerformedAt.Before(before))
}

func TestBeginUndeploy(t *testing.T) {
	t.Run("a PENDING row is undeployed at once, with no event and no deployment", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProxy)
		insertPendingRow(t, p)

		sendEvent, err := beginUndeploy(t, p)
		require.NoError(t, err)
		assert.False(t, sendEvent)
		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusUndeployed, row.Status)
		assert.Equal(t, models.DeploymentStatusUndeployed, row.StatusDesired)
		assert.Nil(t, row.DeploymentID)
		assertQueueClear(t, row)
	})
	t.Run("a DEPLOYING row starts undeploying, with an event", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProxy)
		deployment := startDeploy(t, p)
		queueRender(t, p)
		before := statusRow(t, p).PerformedAt

		sendEvent, err := beginUndeploy(t, p)
		require.NoError(t, err)
		assert.True(t, sendEvent)
		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
		assert.Equal(t, models.DeploymentStatusUndeployed, row.StatusDesired)
		assert.Equal(t, deployment.DeploymentID, *row.DeploymentID, "the row still names what the gateway may hold")
		assertQueueClear(t, row)
		assert.False(t, row.PerformedAt.Before(before))
	})
	t.Run("a failed deploy can be undeployed", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProxy)
		deployment := startDeploy(t, p)
		applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusFailed, "E"), nil)

		sendEvent, err := beginUndeploy(t, p)
		require.NoError(t, err)
		assert.True(t, sendEvent)
		assert.Equal(t, models.DeploymentStatusUndeploying, statusRow(t, p).Status)
	})
	failUndeploy := func(t *testing.T, p models.Placement) {
		t.Helper()
		startDeploy(t, p)
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)
		applyAck(t, p, undeployAck(p.ArtifactUUID.String(), models.DeploymentAckStatusFailed), nil)
		row := statusRow(t, p)
		require.Equal(t, models.DeploymentStatusFailed, row.Status)
		require.Equal(t, models.DeploymentStatusUndeployed, row.StatusDesired)
	}
	t.Run("a failed undeploy can be retried", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProxy)
		failUndeploy(t, p)

		sendEvent, err := beginUndeploy(t, p)
		require.NoError(t, err)
		assert.True(t, sendEvent)
		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
		assert.Equal(t, models.DeploymentStatusUndeployed, row.StatusDesired)
		assertQueueClear(t, row)
	})
	t.Run("a timed-out undeploy can be retried", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProxy)
		startDeploy(t, p)
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)
		backdate(t, p, 10*time.Minute)
		_, err = NewDeploymentRepo(db.GetDB()).FailStaleTransitional(context.Background(), time.Now().Add(-5*time.Minute))
		require.NoError(t, err)

		sendEvent, err := beginUndeploy(t, p)
		require.NoError(t, err)
		assert.True(t, sendEvent)
		assert.Equal(t, models.DeploymentStatusUndeploying, statusRow(t, p).Status)
	})
	t.Run("a failed undeploy with a redeploy owed is retried and loses its queue", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProxy)
		failUndeploy(t, p)
		_, err := NewDeploymentRepo(db.GetDB()).QueueRender(context.Background(), nil, p.OUID, p.ArtifactUUID, nil,
			"redeploy", QueueScopeAny)
		require.NoError(t, err)

		sendEvent, err := beginUndeploy(t, p)
		require.NoError(t, err)
		assert.True(t, sendEvent)
		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
		assertQueueClear(t, row)
	})
	t.Run("an undeploy already under way is not active", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProxy)
		startDeploy(t, p)
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)

		_, err = beginUndeploy(t, p)
		assert.ErrorIs(t, err, utils.ErrDeploymentNotActive)
	})
	t.Run("an undeployed row with a deploy owed only loses its queue", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProxy)
		startDeploy(t, p)
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)
		_, err = NewDeploymentRepo(db.GetDB()).QueueRender(context.Background(), nil, p.OUID, p.ArtifactUUID, nil,
			"redeploy", QueueScopeAny)
		require.NoError(t, err)

		sendEvent, err := beginUndeploy(t, p)
		require.NoError(t, err)
		assert.False(t, sendEvent)
		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
		assert.Equal(t, models.DeploymentStatusUndeployed, row.StatusDesired)
		assertQueueClear(t, row)

		_, err = beginUndeploy(t, p)
		assert.ErrorIs(t, err, utils.ErrDeploymentNotActive, "with no queue left there is nothing to undeploy")
	})
	t.Run("no row is not active", func(t *testing.T) {
		_, err := beginUndeploy(t, seedPlacement(t, models.KindLLMProxy))
		assert.ErrorIs(t, err, utils.ErrDeploymentNotActive)
	})
}

func TestApplyAckDeploy(t *testing.T) {
	t.Run("success goes DEPLOYED and runs afterLive once", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProvider)
		deployment := startDeploy(t, p)
		queueRender(t, p)
		var calls []uuid.UUID
		afterLive := func(_ context.Context, tx *gorm.DB, got models.Placement, deploymentID uuid.UUID) error {
			assert.NotNil(t, tx)
			assert.Equal(t, p, got)
			calls = append(calls, deploymentID)
			return nil
		}

		assert.True(t, applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusSuccess, ""), afterLive))
		assert.False(t, applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusSuccess, ""), afterLive),
			"the same ack twice changes nothing")

		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusDeployed, row.Status)
		assert.Empty(t, row.StatusReason)
		assert.Equal(t, []uuid.UUID{deployment.DeploymentID}, calls)
		assert.NotNil(t, row.NextAttemptAt, "acks never touch the queue")
		assert.Equal(t, 3, row.AttemptCount)
	})
	t.Run("failure goes FAILED with the gateway's error code", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProvider)
		deployment := startDeploy(t, p)

		assert.True(t, applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusFailed,
			"GATEWAY_PROCESSING_ERROR"), nil))
		assert.False(t, applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusFailed,
			"GATEWAY_PROCESSING_ERROR"), nil))

		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusFailed, row.Status)
		assert.Equal(t, models.DeploymentStatusDeployed, row.StatusDesired)
		assert.Equal(t, "GATEWAY_PROCESSING_ERROR", row.StatusReason)
	})
	t.Run("an ack for a superseded deployment changes nothing", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProvider)
		old := startDeploy(t, p)
		current := startDeploy(t, p)

		assert.False(t, applyAck(t, p, deployAck(old.DeploymentID, models.DeploymentAckStatusSuccess, ""), nil))
		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
		assert.Equal(t, current.DeploymentID, *row.DeploymentID)
	})
	t.Run("a deploy ack while undeploying changes nothing", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProvider)
		deployment := startDeploy(t, p)
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)

		assert.False(t, applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusSuccess, ""), nil))
		assert.Equal(t, models.DeploymentStatusUndeploying, statusRow(t, p).Status)
	})
	t.Run("a late ack lands on a row the sweeper timed out", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProvider)
		deployment := startDeploy(t, p)
		backdate(t, p, 10*time.Minute)
		_, err := NewDeploymentRepo(db.GetDB()).FailStaleTransitional(context.Background(), time.Now().Add(-5*time.Minute))
		require.NoError(t, err)
		require.Equal(t, "DEPLOYMENT_TIMEOUT", statusRow(t, p).StatusReason)
		var calls int
		afterLive := func(context.Context, *gorm.DB, models.Placement, uuid.UUID) error { calls++; return nil }

		assert.True(t, applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusSuccess, ""), afterLive))
		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusDeployed, row.Status)
		assert.Empty(t, row.StatusReason)
		assert.Equal(t, 1, calls)
	})
	t.Run("another gateway's ack changes nothing", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProvider)
		deployment := startDeploy(t, p)
		other := p
		other.GatewayUUID = uuid.New()

		assert.False(t, applyAck(t, other, deployAck(deployment.DeploymentID, models.DeploymentAckStatusSuccess, ""), nil))
		assert.Equal(t, models.DeploymentStatusDeploying, statusRow(t, p).Status)
	})
	t.Run("an afterLive error is rolled back and the ack still commits", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProvider)
		deployment := startDeploy(t, p)
		afterLive := func(_ context.Context, tx *gorm.DB, _ models.Placement, _ uuid.UUID) error {
			require.NoError(t, tx.Exec(`UPDATE artifacts SET name = 'written-by-hook' WHERE uuid = ?`, p.ArtifactUUID).Error)
			return errors.New("hook failed")
		}

		assert.True(t, applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusSuccess, ""), afterLive))
		assert.Equal(t, models.DeploymentStatusDeployed, statusRow(t, p).Status)
		var name string
		require.NoError(t, db.GetDB().Raw(`SELECT name FROM artifacts WHERE uuid = ?`, p.ArtifactUUID).Scan(&name).Error)
		assert.Equal(t, p.Handle, name, "the hook's writes are rolled back")
	})
}

func TestApplyAckUndeploy(t *testing.T) {
	t.Run("success goes UNDEPLOYED on artifact and gateway", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProvider)
		deployment := startDeploy(t, p)
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)
		var calls int
		afterLive := func(context.Context, *gorm.DB, models.Placement, uuid.UUID) error { calls++; return nil }

		assert.True(t, applyAck(t, p, undeployAck(p.ArtifactUUID.String(), models.DeploymentAckStatusSuccess), afterLive))
		assert.False(t, applyAck(t, p, undeployAck(p.ArtifactUUID.String(), models.DeploymentAckStatusSuccess), afterLive))

		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusUndeployed, row.Status)
		assert.Equal(t, deployment.DeploymentID, *row.DeploymentID)
		assert.Zero(t, calls)
	})
	t.Run("an LLM proxy is acked by its handle", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProxy)
		startDeploy(t, p)
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)

		assert.True(t, applyAck(t, p, undeployAck(p.Handle, models.DeploymentAckStatusSuccess), nil))
		assert.Equal(t, models.DeploymentStatusUndeployed, statusRow(t, p).Status)
	})
	t.Run("failure goes FAILED", func(t *testing.T) {
		p := seedPlacement(t, models.KindMCPProxy)
		startDeploy(t, p)
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)
		ack := undeployAck(p.ArtifactUUID.String(), models.DeploymentAckStatusFailed)
		ack.ErrorCode = "GATEWAY_PROCESSING_ERROR"

		assert.True(t, applyAck(t, p, ack, nil))
		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusFailed, row.Status)
		assert.Equal(t, models.DeploymentStatusUndeployed, row.StatusDesired)
		assert.Equal(t, "GATEWAY_PROCESSING_ERROR", row.StatusReason)
	})
	t.Run("a row never sent to the gateway is not acked", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProvider)
		insertPendingRow(t, p)
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)

		assert.False(t, applyAck(t, p, undeployAck(p.ArtifactUUID.String(), models.DeploymentAckStatusFailed), nil))
		assert.Equal(t, models.DeploymentStatusUndeployed, statusRow(t, p).Status)
	})
	t.Run("ignored while the row is to be deployed", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProvider)
		startDeploy(t, p)

		assert.False(t, applyAck(t, p, undeployAck(p.ArtifactUUID.String(), models.DeploymentAckStatusSuccess), nil))
		assert.Equal(t, models.DeploymentStatusDeploying, statusRow(t, p).Status)
	})
}

func TestFailStaleTransitional(t *testing.T) {
	repo := NewDeploymentRepo(db.GetDB())
	stale, fresh, staleUndeploy, deployed := seedPlacement(t, models.KindLLMProvider),
		seedPlacement(t, models.KindLLMProvider), seedPlacement(t, models.KindLLMProvider),
		seedPlacement(t, models.KindLLMProvider)
	startDeploy(t, stale)
	queueRender(t, stale)
	backdate(t, stale, 10*time.Minute)
	startDeploy(t, fresh)
	startDeploy(t, staleUndeploy)
	_, err := beginUndeploy(t, staleUndeploy)
	require.NoError(t, err)
	backdate(t, staleUndeploy, 10*time.Minute)
	acked := startDeploy(t, deployed)
	applyAck(t, deployed, deployAck(acked.DeploymentID, models.DeploymentAckStatusSuccess, ""), nil)
	backdate(t, deployed, 10*time.Minute)

	_, err = repo.FailStaleTransitional(context.Background(), time.Now().Add(-5*time.Minute))
	require.NoError(t, err)

	for _, p := range []models.Placement{stale, staleUndeploy} {
		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusFailed, row.Status)
		assert.Equal(t, "DEPLOYMENT_TIMEOUT", row.StatusReason)
	}
	assert.Equal(t, models.DeploymentStatusUndeployed, statusRow(t, staleUndeploy).StatusDesired)
	assert.NotNil(t, statusRow(t, stale).NextAttemptAt, "the sweeper never touches the queue")
	assert.Equal(t, models.DeploymentStatusDeploying, statusRow(t, fresh).Status)
	assert.Equal(t, models.DeploymentStatusDeployed, statusRow(t, deployed).Status)
}

// The sweeper's UPDATE re-checks its guard after waiting on the row lock, so an ack that commits first wins.
func TestFailStaleTransitionalDoesNotOverwriteAConcurrentAck(t *testing.T) {
	repo := NewDeploymentRepo(db.GetDB())
	p := seedPlacement(t, models.KindLLMProvider)
	deployment := startDeploy(t, p)
	backdate(t, p, 10*time.Minute)

	tx := db.GetDB().Begin()
	require.NoError(t, tx.Exec(`UPDATE deployment_status SET status = 'DEPLOYED' WHERE deployment_id = ?`,
		deployment.DeploymentID).Error)
	swept := make(chan int64)
	go func() {
		n, err := repo.FailStaleTransitional(context.Background(), time.Now().Add(-5*time.Minute))
		assert.NoError(t, err)
		swept <- n
	}()
	time.Sleep(100 * time.Millisecond)
	require.NoError(t, tx.Commit().Error)

	<-swept
	assert.Equal(t, models.DeploymentStatusDeployed, statusRow(t, p).Status)
}

func TestFailStaleTransitionalWithTwoSweepers(t *testing.T) {
	repo := NewDeploymentRepo(db.GetDB())
	var placements []models.Placement
	for range 5 {
		p := seedPlacement(t, models.KindLLMProvider)
		startDeploy(t, p)
		backdate(t, p, 10*time.Minute)
		placements = append(placements, p)
	}

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := repo.FailStaleTransitional(context.Background(), time.Now().Add(-5*time.Minute))
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	for _, p := range placements {
		assert.Equal(t, models.DeploymentStatusFailed, statusRow(t, p).Status)
	}
}

// The gateway pulls before it acks, so a deploy is served while it is DEPLOYING; an undeploy is never served.
func TestGetPullable(t *testing.T) {
	repo := NewDeploymentRepo(db.GetDB())
	pull := func(p models.Placement) *models.Deployment {
		deployment, err := repo.GetPullable(context.Background(), p.OUID, p.ArtifactUUID, p.GatewayUUID.String())
		require.NoError(t, err)
		return deployment
	}

	deploying := seedPlacement(t, models.KindLLMProvider)
	deployment := startDeploy(t, deploying)
	got := pull(deploying)
	require.NotNil(t, got)
	assert.Equal(t, deployment.DeploymentID, got.DeploymentID)
	assert.Equal(t, deployment.Content, got.Content)

	failed := seedPlacement(t, models.KindLLMProvider)
	failedDeployment := startDeploy(t, failed)
	applyAck(t, failed, deployAck(failedDeployment.DeploymentID, models.DeploymentAckStatusFailed, "E"), nil)
	assert.NotNil(t, pull(failed), "a failed deploy is still the one the gateway should hold")

	undeploying := seedPlacement(t, models.KindLLMProvider)
	startDeploy(t, undeploying)
	_, err := beginUndeploy(t, undeploying)
	require.NoError(t, err)
	assert.Nil(t, pull(undeploying))

	pending := seedPlacement(t, models.KindLLMProvider)
	insertPendingRow(t, pending)
	assert.Nil(t, pull(pending))

	other := deploying
	other.OUID = "another-org"
	assert.Nil(t, pull(other))
}

func TestPlacedGateways(t *testing.T) {
	repo := NewDeploymentRepo(db.GetDB())
	p := seedPlacement(t, models.KindLLMProvider)
	deployment := startDeploy(t, p)

	placed, err := repo.PlacedGateways(context.Background(), p.OUID, p.ArtifactUUID)
	require.NoError(t, err)
	assert.Equal(t, []string{p.GatewayUUID.String()}, placed)

	applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusSuccess, ""), nil)
	placed, err = repo.PlacedGateways(context.Background(), p.OUID, p.ArtifactUUID)
	require.NoError(t, err)
	assert.Equal(t, []string{p.GatewayUUID.String()}, placed)

	_, err = beginUndeploy(t, p)
	require.NoError(t, err)
	placed, err = repo.PlacedGateways(context.Background(), p.OUID, p.ArtifactUUID)
	require.NoError(t, err)
	assert.Empty(t, placed)
}

func TestGetStatusRecordWithNoRow(t *testing.T) {
	row, err := NewDeploymentRepo(db.GetDB()).GetStatusRecord(context.Background(), seedPlacement(t, models.KindLLMProvider))
	require.NoError(t, err)
	assert.Nil(t, row)
}

// Delete refuses the deployment a placed or held row points at, under the row's lock, and detaches an UNDEPLOYED
// row instead of cascading it away.
func TestDeleteKeepsTheStatusRow(t *testing.T) {
	repo := NewDeploymentRepo(db.GetDB())
	p := seedPlacement(t, models.KindLLMProvider)
	deployment := startDeploy(t, p)
	id, artifact := deployment.DeploymentID.String(), p.ArtifactUUID.String()

	assert.ErrorIs(t, repo.Delete(id, artifact, p.OUID), utils.ErrDeploymentIsDeployed)
	assert.Equal(t, deployment.DeploymentID, *statusRow(t, p).DeploymentID)

	_, err := beginUndeploy(t, p)
	require.NoError(t, err)
	applyAck(t, p, undeployAck(artifact, models.DeploymentAckStatusSuccess), nil)
	require.NoError(t, repo.Delete(id, artifact, p.OUID))

	row := statusRow(t, p)
	assert.Equal(t, models.DeploymentStatusUndeployed, row.Status)
	assert.Nil(t, row.DeploymentID)
}

// LockStatusRows holds the artifacts' rows until its transaction ends.
func TestLockStatusRows(t *testing.T) {
	p := seedPlacement(t, models.KindLLMProvider)
	startDeploy(t, p)
	other := seedPlacement(t, models.KindLLMProvider)
	startDeploy(t, other)
	gdb := db.GetDB()

	locked := gdb.Begin()
	t.Cleanup(func() { locked.Rollback() })
	require.NoError(t, NewDeploymentRepo(gdb).LockStatusRows(context.Background(), locked, p.OUID,
		[]uuid.UUID{p.ArtifactUUID, uuid.New()}))

	tryLock := func(target models.Placement) error {
		return gdb.Transaction(func(tx *gorm.DB) error {
			return tx.Exec(`SELECT 1 FROM deployment_status WHERE artifact_uuid = ? FOR UPDATE NOWAIT`,
				target.ArtifactUUID).Error
		})
	}
	assert.Error(t, tryLock(p), "the row is held")
	assert.NoError(t, tryLock(other), "another artifact's row is not")
	assert.NoError(t, NewDeploymentRepo(gdb).LockStatusRows(context.Background(), locked, p.OUID, nil))
}
