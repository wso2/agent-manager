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
	"math/rand/v2"
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

const testLease = 2 * time.Minute

// queueEpoch is a moment no other test's row is due at: ClaimDueRenders claims across the whole table, and
// integration packages share the database, so these tests make their rows due in 1990 and claim "then".
func queueEpoch() time.Time {
	return time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(rand.IntN(1_000_000)) * time.Minute)
}

func renderQueue() DeploymentRepository { return NewDeploymentRepo(db.GetDB()) }

func insertPending(t *testing.T, p models.Placement, reason string) bool {
	t.Helper()
	var inserted bool
	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		var err error
		inserted, err = renderQueue().InsertPending(context.Background(), tx, p, reason)
		return err
	}))
	return inserted
}

func dueAt(t *testing.T, p models.Placement, at time.Time) {
	t.Helper()
	require.NoError(t, db.GetDB().Exec(
		`UPDATE deployment_status SET next_attempt_at = ? WHERE artifact_uuid = ? AND ou_id = ? AND gateway_uuid = ?`,
		at, p.ArtifactUUID, p.OUID, p.GatewayUUID,
	).Error)
}

// claimOne claims at now and expects exactly p.
func claimOne(t *testing.T, p models.Placement, now time.Time) models.ClaimedRender {
	t.Helper()
	claims, err := renderQueue().ClaimDueRenders(context.Background(), now, testLease, 10)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	require.Equal(t, p, claims[0].Placement)
	return claims[0]
}

func completeRender(claim models.ClaimedRender) (*models.Deployment, error) {
	deployment := newDeployment(claim.Placement)
	err := db.GetDB().Transaction(func(tx *gorm.DB) error {
		return renderQueue().CompleteRender(context.Background(), tx, claim, deployment)
	})
	return deployment, err
}

func queue(t *testing.T, p models.Placement, gateways []uuid.UUID, scope QueueScope) int64 {
	t.Helper()
	n, err := renderQueue().QueueRender(context.Background(), nil, p.OUID, p.ArtifactUUID, gateways, "inputs changed", scope)
	require.NoError(t, err)
	return n
}

func TestInsertPending(t *testing.T) {
	t.Run("writes a PENDING row, due now", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProxy)
		before := time.Now().Add(-time.Second)

		assert.True(t, insertPending(t, p, "placed"))

		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusPending, row.Status)
		assert.Equal(t, models.DeploymentStatusDeployed, row.StatusDesired)
		assert.Nil(t, row.DeploymentID)
		require.NotNil(t, row.NextAttemptAt)
		assert.WithinRange(t, *row.NextAttemptAt, before, time.Now().Add(time.Second))
		assert.Zero(t, row.AttemptCount)
		assert.Equal(t, "placed", row.RenderReason)
		assert.False(t, row.PerformedAt.IsZero())
	})
	t.Run("never touches an existing row", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProxy)
		deployment := startDeploy(t, p)
		before := statusRow(t, p)

		assert.False(t, insertPending(t, p, "placed"))

		row := statusRow(t, p)
		assert.Equal(t, before, row)
		assert.Equal(t, deployment.DeploymentID, *row.DeploymentID)
	})
	t.Run("an UNDEPLOYED row counts as a row, so an undeploy sticks", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProxy)
		require.True(t, insertPending(t, p, "placed"))
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)

		assert.False(t, insertPending(t, p, "placed"))
		assert.Equal(t, models.DeploymentStatusUndeployed, statusRow(t, p).Status)
	})
}

// The placement pass inserting while a user undeploys: one row whatever the order, never a duplicate, and an
// undeploy that lands after the insert undeploys it.
func TestInsertPendingRacingBeginUndeploy(t *testing.T) {
	for range 10 {
		p := seedPlacement(t, models.KindLLMProxy)
		var wg sync.WaitGroup
		var undeployErr error
		wg.Go(func() { insertPending(t, p, "placed") })
		wg.Go(func() { _, undeployErr = beginUndeploy(t, p) })
		wg.Wait()

		var rows int64
		require.NoError(t, db.GetDB().Model(&models.DeploymentStatusRecord{}).
			Where("artifact_uuid = ? AND ou_id = ?", p.ArtifactUUID, p.OUID).Count(&rows).Error)
		assert.Equal(t, int64(1), rows)
		row := statusRow(t, p)
		if undeployErr == nil {
			assert.Equal(t, models.DeploymentStatusUndeployed, row.Status, "the undeploy saw the PENDING row")
			assertQueueClear(t, row)
		} else {
			assert.ErrorIs(t, undeployErr, utils.ErrDeploymentNotActive)
			assert.Equal(t, models.DeploymentStatusPending, row.Status,
				"the undeploy ran first; the render's record re-check undeploys this row")
		}
	}
}

func TestQueueRender(t *testing.T) {
	t.Run("placed scope skips rows heading for UNDEPLOYED and leaves the status alone", func(t *testing.T) {
		placed := seedPlacement(t, models.KindLLMProvider)
		deployment := startDeploy(t, placed)
		applyAck(t, placed, deployAck(deployment.DeploymentID, models.DeploymentAckStatusSuccess, ""), nil)
		undeployed := placed
		undeployed.GatewayUUID = seedPlacement(t, models.KindLLMProvider).GatewayUUID
		startDeploy(t, undeployed)
		_, err := beginUndeploy(t, undeployed)
		require.NoError(t, err)
		require.NoError(t, db.GetDB().Exec(`UPDATE deployment_status SET attempt_count = 4
			WHERE artifact_uuid = ? AND gateway_uuid = ?`, placed.ArtifactUUID, placed.GatewayUUID).Error)

		assert.Equal(t, int64(1), queue(t, placed, nil, QueueScopePlaced))

		row := statusRow(t, placed)
		assert.Equal(t, models.DeploymentStatusDeployed, row.Status, "the gateway still serves the old version")
		assert.Equal(t, deployment.DeploymentID, *row.DeploymentID)
		require.NotNil(t, row.NextAttemptAt)
		assert.Zero(t, row.AttemptCount)
		assert.Equal(t, "inputs changed", row.RenderReason)
		assert.Nil(t, statusRow(t, undeployed).NextAttemptAt)
	})
	t.Run("any scope re-queues an UNDEPLOYED row", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProvider)
		startDeploy(t, p)
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)

		assert.Equal(t, int64(1), queue(t, p, nil, QueueScopeAny))

		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
		assert.NotNil(t, row.NextAttemptAt)
	})
	t.Run("gateways limit the rows", func(t *testing.T) {
		p := seedPlacement(t, models.KindLLMProvider)
		other := p
		other.GatewayUUID = seedPlacement(t, models.KindLLMProvider).GatewayUUID
		startDeploy(t, p)
		startDeploy(t, other)

		assert.Equal(t, int64(1), queue(t, p, []uuid.UUID{other.GatewayUUID}, QueueScopePlaced))
		assert.Nil(t, statusRow(t, p).NextAttemptAt)
		assert.NotNil(t, statusRow(t, other).NextAttemptAt)
	})
}

func TestClaimDueRenders(t *testing.T) {
	epoch := queueEpoch()
	p := seedPlacement(t, models.KindLLMProxy)
	require.True(t, insertPending(t, p, "placed"))
	dueAt(t, p, epoch)
	notDue := seedPlacement(t, models.KindLLMProxy)
	require.True(t, insertPending(t, notDue, "placed"))
	dueAt(t, notDue, epoch.Add(time.Hour))

	claim := claimOne(t, p, epoch)

	assert.Equal(t, models.KindLLMProxy, claim.Kind)
	assert.Equal(t, p.Handle, claim.Handle)
	assert.Zero(t, claim.AttemptCount)
	assert.True(t, claim.Token.Equal(epoch.Add(testLease)), "the token is the leased next_attempt_at")
	assert.True(t, statusRow(t, p).NextAttemptAt.Equal(claim.Token))

	claims, err := renderQueue().ClaimDueRenders(context.Background(), epoch.Add(testLease/2), testLease, 10)
	require.NoError(t, err)
	assert.Empty(t, claims, "a leased row isn't claimed again inside its lease")
}

// A replica killed between render and write-back commits nothing; once the lease runs out the row is claimed again.
func TestClaimDueRendersAfterAnAbandonedClaim(t *testing.T) {
	epoch := queueEpoch()
	p := seedPlacement(t, models.KindLLMProxy)
	require.True(t, insertPending(t, p, "placed"))
	dueAt(t, p, epoch)
	first := claimOne(t, p, epoch)

	row := statusRow(t, p)
	assert.Equal(t, models.DeploymentStatusPending, row.Status, "nothing is half-committed")
	assert.Nil(t, row.DeploymentID)

	second := claimOne(t, p, epoch.Add(testLease+time.Second))
	assert.True(t, second.Token.After(first.Token))
	_, err := completeRender(first)
	assert.ErrorIs(t, err, ErrRenderSuperseded, "the abandoned claim can't write back")
	deployment, err := completeRender(second)
	require.NoError(t, err)
	assert.Equal(t, deployment.DeploymentID, *statusRow(t, p).DeploymentID)
}

func TestClaimDueRendersSkipsRowsAnotherClaimerHolds(t *testing.T) {
	epoch := queueEpoch()
	var placements []models.Placement
	for range 6 {
		p := seedPlacement(t, models.KindLLMProxy)
		require.True(t, insertPending(t, p, "placed"))
		dueAt(t, p, epoch)
		placements = append(placements, p)
	}

	var mu sync.Mutex
	claimed := map[models.Placement]int{}
	var wg sync.WaitGroup
	for range 3 {
		wg.Go(func() {
			claims, err := renderQueue().ClaimDueRenders(context.Background(), epoch, testLease, 2)
			assert.NoError(t, err)
			mu.Lock()
			defer mu.Unlock()
			for _, c := range claims {
				claimed[c.Placement]++
			}
		})
	}
	wg.Wait()

	for _, p := range placements {
		assert.LessOrEqual(t, claimed[p], 1, "no row is claimed twice")
	}
}

func TestClaimDueRendersOf(t *testing.T) {
	p := seedPlacement(t, models.KindLLMProvider)
	other := p
	other.GatewayUUID = seedPlacement(t, models.KindLLMProvider).GatewayUUID
	startDeploy(t, p)
	startDeploy(t, other)
	queue(t, p, nil, QueueScopePlaced)
	unrelated := seedPlacement(t, models.KindLLMProvider)
	require.True(t, insertPending(t, unrelated, "placed"))

	claims, err := renderQueue().ClaimDueRendersOf(context.Background(), p.OUID, p.ArtifactUUID,
		[]uuid.UUID{other.GatewayUUID}, time.Now(), testLease)
	require.NoError(t, err)
	require.Len(t, claims, 1)
	assert.Equal(t, other, claims[0].Placement)

	claims, err = renderQueue().ClaimDueRendersOf(context.Background(), p.OUID, p.ArtifactUUID, nil, time.Now(), testLease)
	require.NoError(t, err)
	require.Len(t, claims, 1, "the other gateway's row is leased")
	assert.Equal(t, p, claims[0].Placement)
}

// The claim token: only a request, a restore or an undeploy supersedes a render. An ack doesn't.
func TestCompleteRender(t *testing.T) {
	deployedAndQueued := func(t *testing.T) (models.Placement, *models.Deployment, models.ClaimedRender) {
		epoch := queueEpoch()
		p := seedPlacement(t, models.KindLLMProvider)
		first := startDeploy(t, p)
		queue(t, p, nil, QueueScopePlaced)
		dueAt(t, p, epoch)
		return p, first, claimOne(t, p, epoch)
	}

	t.Run("writes a DEPLOYING deployment and clears the queue", func(t *testing.T) {
		p, _, claim := deployedAndQueued(t)

		deployment, err := completeRender(claim)
		require.NoError(t, err)

		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
		assert.Equal(t, deployment.DeploymentID, *row.DeploymentID)
		assertQueueClear(t, row)
		require.NotNil(t, deployment.UpdatedAt)
		assert.True(t, row.PerformedAt.Equal(*deployment.UpdatedAt))
	})
	t.Run("an ack mid-render doesn't supersede it", func(t *testing.T) {
		p, first, claim := deployedAndQueued(t)
		require.True(t, applyAck(t, p, deployAck(first.DeploymentID, models.DeploymentAckStatusSuccess, ""), nil))

		deployment, err := completeRender(claim)
		require.NoError(t, err)
		assert.Equal(t, deployment.DeploymentID, *statusRow(t, p).DeploymentID)
	})
	t.Run("a newer request supersedes it", func(t *testing.T) {
		p, first, claim := deployedAndQueued(t)
		queue(t, p, nil, QueueScopePlaced)

		_, err := completeRender(claim)
		assert.ErrorIs(t, err, ErrRenderSuperseded)
		assert.Equal(t, first.DeploymentID, *statusRow(t, p).DeploymentID)
	})
	t.Run("a restore supersedes it", func(t *testing.T) {
		p, first, claim := deployedAndQueued(t)
		require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
			return renderQueue().StartRestore(context.Background(), tx, p, first.DeploymentID)
		}))

		_, err := completeRender(claim)
		assert.ErrorIs(t, err, ErrRenderSuperseded)
	})
	t.Run("a user undeploy supersedes it", func(t *testing.T) {
		p, _, claim := deployedAndQueued(t)
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)

		_, err = completeRender(claim)
		assert.ErrorIs(t, err, ErrRenderSuperseded)
		assert.Equal(t, models.DeploymentStatusUndeploying, statusRow(t, p).Status)
	})
}

func TestRecordRenderOutcome(t *testing.T) {
	claimed := func(t *testing.T) (models.Placement, models.ClaimedRender, time.Time) {
		epoch := queueEpoch()
		p := seedPlacement(t, models.KindAgent)
		require.True(t, insertPending(t, p, "placed"))
		require.NoError(t, db.GetDB().Exec(`UPDATE deployment_status SET attempt_count = 2
			WHERE artifact_uuid = ? AND gateway_uuid = ?`, p.ArtifactUUID, p.GatewayUUID).Error)
		dueAt(t, p, epoch)
		return p, claimOne(t, p, epoch), epoch
	}
	record := func(claim models.ClaimedRender, outcome RenderOutcome) error {
		return renderQueue().RecordRenderOutcome(context.Background(), claim, outcome)
	}

	t.Run("waiting keeps the attempt count", func(t *testing.T) {
		p, claim, epoch := claimed(t)
		next := epoch.Add(10 * time.Second)

		require.NoError(t, record(claim, RenderOutcome{Outcome: RenderWaiting, Reason: "waiting for the endpoint", NextAttemptAt: &next}))

		row := statusRow(t, p)
		assert.True(t, row.NextAttemptAt.Equal(next))
		assert.Equal(t, 2, row.AttemptCount)
		assert.Equal(t, "waiting for the endpoint", row.RenderReason)
		assert.Equal(t, models.DeploymentStatusPending, row.Status)

		again := claimOne(t, p, next)
		assert.InDelta(t, 10*time.Second, again.LastDelay, float64(time.Second),
			"the next claim knows how long the row waited, to double it")
	})
	t.Run("retry counts an attempt", func(t *testing.T) {
		p, claim, epoch := claimed(t)
		next := epoch.Add(time.Minute)

		require.NoError(t, record(claim, RenderOutcome{Outcome: RenderRetry, Reason: "boom", NextAttemptAt: &next}))

		row := statusRow(t, p)
		assert.Equal(t, 3, row.AttemptCount)
		assert.Equal(t, "boom", row.RenderReason)
		assert.True(t, row.NextAttemptAt.Equal(next))
	})
	t.Run("give up clears the queue and keeps the reason", func(t *testing.T) {
		p, claim, _ := claimed(t)

		require.NoError(t, record(claim, RenderOutcome{Outcome: RenderGiveUp, Reason: "invalid policy"}))

		row := statusRow(t, p)
		assert.Nil(t, row.NextAttemptAt)
		assert.Equal(t, "invalid policy", row.RenderReason)
		assert.Equal(t, models.UserStatus{Status: models.DeploymentStatusFailed, Reason: "invalid policy"}, models.UserStatusOf(row))
	})
	t.Run("a superseded claim records nothing", func(t *testing.T) {
		p, claim, _ := claimed(t)
		queue(t, p, nil, QueueScopeAny)

		err := record(claim, RenderOutcome{Outcome: RenderGiveUp, Reason: "invalid policy"})
		assert.ErrorIs(t, err, ErrRenderSuperseded)
		assert.NotNil(t, statusRow(t, p).NextAttemptAt)
	})
}

func TestAbandonRender(t *testing.T) {
	abandon := func(claim models.ClaimedRender) (bool, error) {
		var sendEvent bool
		err := db.GetDB().Transaction(func(tx *gorm.DB) error {
			var err error
			sendEvent, err = renderQueue().AbandonRender(context.Background(), tx, claim)
			return err
		})
		return sendEvent, err
	}

	t.Run("a PENDING row goes UNDEPLOYED with no event", func(t *testing.T) {
		epoch := queueEpoch()
		p := seedPlacement(t, models.KindAgent)
		require.True(t, insertPending(t, p, "placed"))
		dueAt(t, p, epoch)

		sendEvent, err := abandon(claimOne(t, p, epoch))
		require.NoError(t, err)
		assert.False(t, sendEvent)
		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusUndeployed, row.Status)
		assert.Nil(t, row.DeploymentID)
		assertQueueClear(t, row)
		assert.False(t, insertPending(t, p, "placed"), "the next placement pass doesn't re-place it")
	})
	t.Run("a placed row starts undeploying, with an event", func(t *testing.T) {
		epoch := queueEpoch()
		p := seedPlacement(t, models.KindAgent)
		startDeploy(t, p)
		queue(t, p, nil, QueueScopePlaced)
		dueAt(t, p, epoch)

		sendEvent, err := abandon(claimOne(t, p, epoch))
		require.NoError(t, err)
		assert.True(t, sendEvent)
		assert.Equal(t, models.DeploymentStatusUndeploying, statusRow(t, p).Status)
	})
	t.Run("a row already heading for UNDEPLOYED only loses its queue", func(t *testing.T) {
		epoch := queueEpoch()
		p := seedPlacement(t, models.KindAgent)
		startDeploy(t, p)
		_, err := beginUndeploy(t, p)
		require.NoError(t, err)
		queue(t, p, nil, QueueScopeAny)
		dueAt(t, p, epoch)

		sendEvent, err := abandon(claimOne(t, p, epoch))
		require.NoError(t, err)
		assert.False(t, sendEvent)
		row := statusRow(t, p)
		assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
		assertQueueClear(t, row)
	})
	t.Run("a superseded claim changes nothing", func(t *testing.T) {
		epoch := queueEpoch()
		p := seedPlacement(t, models.KindAgent)
		require.True(t, insertPending(t, p, "placed"))
		dueAt(t, p, epoch)
		claim := claimOne(t, p, epoch)
		queue(t, p, nil, QueueScopeAny)

		_, err := abandon(claim)
		assert.ErrorIs(t, err, ErrRenderSuperseded)
		assert.Equal(t, models.DeploymentStatusPending, statusRow(t, p).Status)
	})
}

func TestListDeployedForDrift(t *testing.T) {
	ouID := testOU()
	gateway := seedPlacement(t, models.KindAgent).GatewayUUID
	exposed := func(t *testing.T, kind string, protocol models.AgentExposureProtocol) models.Placement {
		exposure := newTestExposure(t, ouID, "agent-"+uuid.NewString()[:8], uuid.New())
		exposure.Protocol = protocol
		require.NoError(t, db.GetDB().Exec(`UPDATE artifacts SET kind = ? WHERE uuid = ?`, kind, exposure.ArtifactUUID).Error)
		require.NoError(t, NewPlatformAgentExposureRepository(db.GetDB()).Upsert(context.Background(), exposure))
		return models.Placement{
			ArtifactRef: models.ArtifactRef{
				OUID: ouID, ArtifactUUID: exposure.ArtifactUUID, Kind: kind,
				Handle: "checkout/" + exposure.AgentName + "/" + exposure.EnvironmentUUID.String(),
			},
			GatewayUUID: gateway,
		}
	}
	live := func(t *testing.T, p models.Placement) {
		deployment := startDeploy(t, p)
		require.True(t, applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusSuccess, ""), nil))
	}
	listed := func(t *testing.T, kind string) []models.Placement {
		var placements []models.Placement
		afterKey := ""
		for {
			page, err := renderQueue().ListDeployedForDrift(context.Background(), kind, afterKey, 2)
			require.NoError(t, err)
			if len(page) == 0 {
				return placements
			}
			for _, p := range page {
				if p.OUID == ouID {
					placements = append(placements, p)
				}
			}
			afterKey = DriftKey(page[len(page)-1])
		}
	}

	var platform []models.Placement
	for range 3 {
		p := exposed(t, models.KindAgent, models.AgentExposureProtocolA2A)
		live(t, p)
		platform = append(platform, p)
	}
	rest := exposed(t, models.KindRestAPI, models.AgentExposureProtocolREST)
	live(t, rest)
	deploying := exposed(t, models.KindAgent, models.AgentExposureProtocolA2A)
	startDeploy(t, deploying)
	external := seedArtifactOn(t, models.Placement{GatewayUUID: gateway}, ouID, models.KindAgent)
	live(t, external)
	queued := exposed(t, models.KindAgent, models.AgentExposureProtocolA2A)
	live(t, queued)
	queue(t, queued, nil, QueueScopePlaced)

	assert.ElementsMatch(t, platform, listed(t, models.KindAgent),
		"only DEPLOYED platform rows with no render owed; external agents have no exposure")
	assert.Equal(t, []models.Placement{rest}, listed(t, models.KindRestAPI))
}

func TestGatewayDeleteRemovesItsStatusRows(t *testing.T) {
	p := seedPlacement(t, models.KindLLMProxy)
	require.True(t, insertPending(t, p, "placed"))
	undeployed := seedArtifactOn(t, p, p.OUID, models.KindLLMProxy)
	require.True(t, insertPending(t, undeployed, "placed"))
	_, err := beginUndeploy(t, undeployed)
	require.NoError(t, err)

	require.NoError(t, NewGatewayRepo(db.GetDB()).Delete(p.GatewayUUID.String(), p.OUID))

	var kept []models.DeploymentStatusRecord
	require.NoError(t, db.GetDB().Where("gateway_uuid = ?", p.GatewayUUID).Find(&kept).Error)
	require.Len(t, kept, 1, "the PENDING row goes, or it would keep rendering; the user's undeploy stays")
	assert.Equal(t, models.DeploymentStatusUndeployed, kept[0].Status)
	assert.Nil(t, kept[0].NextAttemptAt)
}

// A gateway with nothing held can be deleted (HasGatewayDeployments), but it may still have a deploy owed: a PENDING
// row, or an UNDEPLOYED row re-queued. Its delete takes every status row with it, so nothing is left to render for
// the deleted gateway.
func TestGatewayDeleteLeavesNothingToRender(t *testing.T) {
	pending := seedPlacement(t, models.KindLLMProxy)
	require.True(t, insertPending(t, pending, "placed"))
	owed := seedArtifactOn(t, pending, pending.OUID, models.KindLLMProxy)
	startDeploy(t, owed)
	_, err := beginUndeploy(t, owed)
	require.NoError(t, err)
	applyAck(t, owed, undeployAck(owed.ArtifactUUID.String(), models.DeploymentAckStatusSuccess), nil)
	queue(t, owed, nil, QueueScopeAny)
	has, err := NewGatewayRepo(db.GetDB()).HasGatewayDeployments(pending.GatewayUUID.String(), pending.OUID)
	require.NoError(t, err)
	require.False(t, has, "the gateway holds nothing, so it may be deleted")

	require.NoError(t, NewGatewayRepo(db.GetDB()).Delete(pending.GatewayUUID.String(), pending.OUID))

	for _, p := range []models.Placement{pending, owed} {
		claims, err := renderQueue().ClaimDueRendersOf(context.Background(), p.OUID, p.ArtifactUUID, nil,
			time.Now().Add(time.Hour), testLease)
		require.NoError(t, err)
		assert.Empty(t, claims, "nothing is owed to the deleted gateway")
	}
	var rows int64
	require.NoError(t, db.GetDB().Model(&models.DeploymentStatusRecord{}).
		Where("gateway_uuid = ?", pending.GatewayUUID).Count(&rows).Error)
	assert.Zero(t, rows)
}
