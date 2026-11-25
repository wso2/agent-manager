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

package gwrepo

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
)

// seedArtifactOn writes another artifact of kind in ouID and places it on p's gateway, with no status row.
func seedArtifactOn(t *testing.T, p gwmodels.Placement, ouID, kind string) gwmodels.Placement {
	t.Helper()
	gdb := db.GetDB()
	artifactUUID := uuid.New()
	handle := "handle-" + artifactUUID.String()[:8]
	require.NoError(t, NewArtifactRepo(gdb, gwtest.NewSecrets()).Create(gdb, &gwmodels.Artifact{
		UUID: artifactUUID, Handle: handle, Name: handle, Version: "v1.0", Kind: kind, OUID: ouID,
	}))
	t.Cleanup(func() { gdb.Exec(`DELETE FROM mod_ai_gateway_artifacts WHERE id = ?`, artifactUUID) })
	return gwmodels.Placement{
		ArtifactRef: gwmodels.ArtifactRef{OUID: ouID, ArtifactUUID: artifactUUID, Kind: kind, Handle: handle},
		GatewayUUID: p.GatewayUUID,
	}
}

func listForGatewaySync(t *testing.T, p gwmodels.Placement) map[uuid.UUID]gwmodels.GatewaySyncRow {
	t.Helper()
	rows, err := NewDeploymentRepo(db.GetDB()).ListForGatewaySync(context.Background(), p.OUID, p.GatewayUUID.String())
	require.NoError(t, err)
	byArtifact := make(map[uuid.UUID]gwmodels.GatewaySyncRow, len(rows))
	for _, row := range rows {
		require.NotContains(t, byArtifact, row.ArtifactUUID, "one entry per artifact")
		byArtifact[row.ArtifactUUID] = row
	}
	return byArtifact
}

func TestListForGatewaySync(t *testing.T) {
	gateway := seedPlacement(t, gwmodels.KindLLMProvider)
	on := func(kind string) gwmodels.Placement { return seedArtifactOn(t, gateway, gateway.OUID, kind) }

	pending := on(gwmodels.KindLLMProvider)
	insertPendingRow(t, pending)

	undeployedFromPending := on(gwmodels.KindLLMProvider)
	insertPendingRow(t, undeployedFromPending)
	_, err := beginUndeploy(t, undeployedFromPending)
	require.NoError(t, err)

	deploying := on(gwmodels.KindLLMProxy)
	deployingDeployment := startDeploy(t, deploying)

	deployed := gateway
	deployedDeployment := startDeploy(t, deployed)
	applyAck(t, deployed, deployAck(deployedDeployment.DeploymentID, gwmodels.DeploymentAckStatusSuccess, ""), nil)

	failedDeploy := on(gwmodels.KindMCPMapping)
	failedDeployment := startDeploy(t, failedDeploy)
	applyAck(t, failedDeploy, deployAck(failedDeployment.DeploymentID, gwmodels.DeploymentAckStatusFailed, "E"), nil)

	undeploying := on(gwmodels.KindAgent)
	startDeploy(t, undeploying)
	_, err = beginUndeploy(t, undeploying)
	require.NoError(t, err)

	undeployed := on(gwmodels.KindLLMProvider)
	startDeploy(t, undeployed)
	_, err = beginUndeploy(t, undeployed)
	require.NoError(t, err)
	applyAck(t, undeployed, undeployAck(undeployed.ArtifactUUID.String(), gwmodels.DeploymentAckStatusSuccess), nil)

	failedUndeploy := on(gwmodels.KindLLMProvider)
	startDeploy(t, failedUndeploy)
	_, err = beginUndeploy(t, failedUndeploy)
	require.NoError(t, err)
	applyAck(t, failedUndeploy, undeployAck(failedUndeploy.ArtifactUUID.String(), gwmodels.DeploymentAckStatusFailed), nil)

	otherGateway := seedPlacement(t, gwmodels.KindLLMProvider)
	startDeploy(t, otherGateway)
	// Another org's artifact can't be placed on this gateway at all: its status row would break the org key.
	otherOrg := seedArtifactOn(t, gateway, newOrg(t), gwmodels.KindLLMProvider)

	got := listForGatewaySync(t, gateway)

	assert.NotContains(t, got, pending.ArtifactUUID, "the gateway never held a PENDING row")
	assert.NotContains(t, got, undeployedFromPending.ArtifactUUID, "nor one undeployed before it was sent")
	assert.NotContains(t, got, otherGateway.ArtifactUUID)
	assert.NotContains(t, got, otherOrg.ArtifactUUID)
	assert.Len(t, got, 6)

	for _, c := range []struct {
		name       string
		placement  gwmodels.Placement
		deployment *gwmodels.Deployment
	}{
		{"deploying", deploying, deployingDeployment},
		{"deployed", deployed, deployedDeployment},
		{"failed deploy", failedDeploy, failedDeployment},
	} {
		row := got[c.placement.ArtifactUUID]
		assert.Equal(t, gwmodels.GatewaySyncStateDeployed, row.State, c.name)
		assert.Equal(t, c.deployment.DeploymentID.String(), row.DeploymentID, c.name)
	}
	for name, p := range map[string]gwmodels.Placement{
		"undeploying": undeploying, "undeployed": undeployed, "failed undeploy": failedUndeploy,
	} {
		row := got[p.ArtifactUUID]
		assert.Equal(t, gwmodels.GatewaySyncStateUndeployed, row.State, name)
		assert.Empty(t, row.DeploymentID, "%s: the gateway stored the undeploy event's empty id", name)
	}

	for _, p := range []gwmodels.Placement{deploying, deployed, failedDeploy, undeploying, undeployed, failedUndeploy} {
		row := got[p.ArtifactUUID]
		assert.Equal(t, p.ArtifactRef, row.ArtifactRef)
		performedAt := statusRow(t, p).PerformedAt
		assert.True(t, performedAt.Equal(row.DeployedAt), "deployedAt %s is performed_at %s", row.DeployedAt, performedAt)
		assert.Equal(t, gwmodels.NewPerformedAt(performedAt), row.DeployedAt.UTC(), "millisecond precision")
	}
}

func TestListContentForGateway(t *testing.T) {
	repo := NewDeploymentRepo(db.GetDB())
	gateway := seedPlacement(t, gwmodels.KindLLMProxy)
	current := startDeploy(t, gateway)
	otherGateway := seedPlacement(t, gwmodels.KindLLMProvider)
	elsewhere := startDeploy(t, otherGateway)

	got, err := repo.ListContentForGateway(context.Background(), gateway.OUID, gateway.GatewayUUID.String(),
		[]uuid.UUID{current.DeploymentID, elsewhere.DeploymentID, uuid.New()})
	require.NoError(t, err)

	require.Len(t, got, 1, "a deployment of another gateway or org, or none at all, is skipped")
	assert.Equal(t, gateway.ArtifactRef, got[0].ArtifactRef)
	assert.Equal(t, current.DeploymentID, got[0].DeploymentID)
	assert.Equal(t, current.Content, got[0].Content)

	got, err = repo.ListContentForGateway(context.Background(), uuid.NewString(), gateway.GatewayUUID.String(),
		[]uuid.UUID{current.DeploymentID})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestArtifactListExisting(t *testing.T) {
	repo := NewArtifactRepo(db.GetDB(), gwtest.NewSecrets())
	provider := seedPlacement(t, gwmodels.KindLLMProvider)
	proxy := seedArtifactOn(t, provider, provider.OUID, gwmodels.KindLLMProxy)
	notAProxy := seedArtifactOn(t, provider, provider.OUID, gwmodels.KindMCPMapping)
	otherOrg := seedArtifactOn(t, provider, newOrg(t), gwmodels.KindLLMProxy)

	got, err := repo.ListExisting(context.Background(), provider.OUID,
		[]uuid.UUID{provider.ArtifactUUID, otherOrg.ArtifactUUID, uuid.New()},
		[]string{proxy.Handle, notAProxy.Handle, otherOrg.Handle})
	require.NoError(t, err)

	assert.ElementsMatch(t, []gwmodels.ArtifactRef{provider.ArtifactRef, proxy.ArtifactRef}, got)
}

// A gateway holds an artifact while its row names a deployment and isn't UNDEPLOYED. Rows it never held, or no
// longer holds, don't keep the gateway from being deleted.
func TestHasGatewayDeploymentsCountsOnlyWhatTheGatewayHolds(t *testing.T) {
	hasDeployments := func(p gwmodels.Placement) bool {
		t.Helper()
		has, err := NewGatewayRepo(db.GetDB()).HasGatewayDeployments(p.GatewayUUID.String(), p.OUID)
		require.NoError(t, err)
		return has
	}

	gateway := seedPlacement(t, gwmodels.KindLLMProvider)
	insertPendingRow(t, gateway)
	undeployed := seedArtifactOn(t, gateway, gateway.OUID, gwmodels.KindLLMProvider)
	startDeploy(t, undeployed)
	_, err := beginUndeploy(t, undeployed)
	require.NoError(t, err)
	applyAck(t, undeployed, undeployAck(undeployed.ArtifactUUID.String(), gwmodels.DeploymentAckStatusSuccess), nil)

	assert.False(t, hasDeployments(gateway), "only PENDING and UNDEPLOYED rows")

	for name, hold := range map[string]func(gwmodels.Placement){
		"deploying": func(p gwmodels.Placement) { startDeploy(t, p) },
		"undeploying": func(p gwmodels.Placement) {
			startDeploy(t, p)
			_, err := beginUndeploy(t, p)
			require.NoError(t, err)
		},
		"failed": func(p gwmodels.Placement) {
			deployment := startDeploy(t, p)
			applyAck(t, p, deployAck(deployment.DeploymentID, gwmodels.DeploymentAckStatusFailed, "E"), nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := seedPlacement(t, gwmodels.KindLLMProvider)
			hold(p)
			assert.True(t, hasDeployments(p))
		})
	}

	// An undeploy the gateway never confirmed, as on a gateway that never connected, times out to FAILED. The user
	// asked for the artifact to go, so it no longer keeps the gateway from being deleted.
	unacked := seedPlacement(t, gwmodels.KindLLMProvider)
	startDeploy(t, unacked)
	_, err = beginUndeploy(t, unacked)
	require.NoError(t, err)
	assert.True(t, hasDeployments(unacked), "the gateway gets its timeout to confirm the undeploy")
	backdate(t, unacked, 10*time.Minute)
	_, err = NewDeploymentRepo(db.GetDB()).FailStaleTransitional(context.Background(), time.Now().Add(-5*time.Minute))
	require.NoError(t, err)
	require.Equal(t, gwmodels.DeploymentStatusFailed, statusRow(t, unacked).Status)
	assert.False(t, hasDeployments(unacked), "a timed-out undeploy does not hold the gateway")
}
