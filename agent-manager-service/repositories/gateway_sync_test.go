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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// seedArtifactOn writes another artifact of kind in ouID and places it on p's gateway, with no status row.
func seedArtifactOn(t *testing.T, p models.Placement, ouID, kind string) models.Placement {
	t.Helper()
	gdb := db.GetDB()
	artifactUUID := uuid.New()
	handle := "handle-" + artifactUUID.String()[:8]
	require.NoError(t, NewArtifactRepo(gdb).Create(gdb, &models.Artifact{
		UUID: artifactUUID, Handle: handle, Name: handle, Version: "v1.0", Kind: kind, OUID: ouID,
	}))
	t.Cleanup(func() { gdb.Exec(`DELETE FROM artifacts WHERE uuid = ?`, artifactUUID) })
	return models.Placement{
		ArtifactRef: models.ArtifactRef{OUID: ouID, ArtifactUUID: artifactUUID, Kind: kind, Handle: handle},
		GatewayUUID: p.GatewayUUID,
	}
}

func listForGatewaySync(t *testing.T, p models.Placement) map[uuid.UUID]models.GatewaySyncRow {
	t.Helper()
	rows, err := NewDeploymentRepo(db.GetDB()).ListForGatewaySync(context.Background(), p.OUID, p.GatewayUUID.String())
	require.NoError(t, err)
	byArtifact := make(map[uuid.UUID]models.GatewaySyncRow, len(rows))
	for _, row := range rows {
		require.NotContains(t, byArtifact, row.ArtifactUUID, "one entry per artifact")
		byArtifact[row.ArtifactUUID] = row
	}
	return byArtifact
}

func TestListForGatewaySync(t *testing.T) {
	gateway := seedPlacement(t, models.KindLLMProvider)
	on := func(kind string) models.Placement { return seedArtifactOn(t, gateway, gateway.OUID, kind) }

	pending := on(models.KindLLMProvider)
	insertPendingRow(t, pending)

	undeployedFromPending := on(models.KindLLMProvider)
	insertPendingRow(t, undeployedFromPending)
	_, err := beginUndeploy(t, undeployedFromPending)
	require.NoError(t, err)

	deploying := on(models.KindLLMProxy)
	deployingDeployment := startDeploy(t, deploying)

	deployed := gateway
	deployedDeployment := startDeploy(t, deployed)
	applyAck(t, deployed, deployAck(deployedDeployment.DeploymentID, models.DeploymentAckStatusSuccess, ""), nil)

	failedDeploy := on(models.KindMCPMapping)
	failedDeployment := startDeploy(t, failedDeploy)
	applyAck(t, failedDeploy, deployAck(failedDeployment.DeploymentID, models.DeploymentAckStatusFailed, "E"), nil)

	undeploying := on(models.KindAgent)
	startDeploy(t, undeploying)
	_, err = beginUndeploy(t, undeploying)
	require.NoError(t, err)

	undeployed := on(models.KindLLMProvider)
	startDeploy(t, undeployed)
	_, err = beginUndeploy(t, undeployed)
	require.NoError(t, err)
	applyAck(t, undeployed, undeployAck(undeployed.ArtifactUUID.String(), models.DeploymentAckStatusSuccess), nil)

	failedUndeploy := on(models.KindLLMProvider)
	startDeploy(t, failedUndeploy)
	_, err = beginUndeploy(t, failedUndeploy)
	require.NoError(t, err)
	applyAck(t, failedUndeploy, undeployAck(failedUndeploy.ArtifactUUID.String(), models.DeploymentAckStatusFailed), nil)

	otherGateway := seedPlacement(t, models.KindLLMProvider)
	startDeploy(t, otherGateway)
	otherOrg := seedArtifactOn(t, gateway, "ou-"+uuid.NewString()[:8], models.KindLLMProvider)
	startDeploy(t, otherOrg)

	got := listForGatewaySync(t, gateway)

	assert.NotContains(t, got, pending.ArtifactUUID, "the gateway never held a PENDING row")
	assert.NotContains(t, got, undeployedFromPending.ArtifactUUID, "nor one undeployed before it was sent")
	assert.NotContains(t, got, otherGateway.ArtifactUUID)
	assert.NotContains(t, got, otherOrg.ArtifactUUID)
	assert.Len(t, got, 6)

	for _, c := range []struct {
		name       string
		placement  models.Placement
		deployment *models.Deployment
	}{
		{"deploying", deploying, deployingDeployment},
		{"deployed", deployed, deployedDeployment},
		{"failed deploy", failedDeploy, failedDeployment},
	} {
		row := got[c.placement.ArtifactUUID]
		assert.Equal(t, models.GatewaySyncStateDeployed, row.State, c.name)
		assert.Equal(t, c.deployment.DeploymentID.String(), row.DeploymentID, c.name)
	}
	for name, p := range map[string]models.Placement{
		"undeploying": undeploying, "undeployed": undeployed, "failed undeploy": failedUndeploy,
	} {
		row := got[p.ArtifactUUID]
		assert.Equal(t, models.GatewaySyncStateUndeployed, row.State, name)
		assert.Empty(t, row.DeploymentID, "%s: the gateway stored the undeploy event's empty id", name)
	}

	for _, p := range []models.Placement{deploying, deployed, failedDeploy, undeploying, undeployed, failedUndeploy} {
		row := got[p.ArtifactUUID]
		assert.Equal(t, p.ArtifactRef, row.ArtifactRef)
		performedAt := statusRow(t, p).PerformedAt
		assert.True(t, performedAt.Equal(row.DeployedAt), "deployedAt %s is performed_at %s", row.DeployedAt, performedAt)
		assert.Equal(t, models.NewPerformedAt(performedAt), row.DeployedAt.UTC(), "millisecond precision")
	}
}

func TestListContentForGateway(t *testing.T) {
	repo := NewDeploymentRepo(db.GetDB())
	gateway := seedPlacement(t, models.KindLLMProxy)
	current := startDeploy(t, gateway)
	otherGateway := seedPlacement(t, models.KindLLMProvider)
	elsewhere := startDeploy(t, otherGateway)

	got, err := repo.ListContentForGateway(context.Background(), gateway.OUID, gateway.GatewayUUID.String(),
		[]uuid.UUID{current.DeploymentID, elsewhere.DeploymentID, uuid.New()})
	require.NoError(t, err)

	require.Len(t, got, 1, "a deployment of another gateway or org, or none at all, is skipped")
	assert.Equal(t, gateway.ArtifactRef, got[0].ArtifactRef)
	assert.Equal(t, current.DeploymentID, got[0].DeploymentID)
	assert.Equal(t, current.Content, got[0].Content)

	got, err = repo.ListContentForGateway(context.Background(), "another-org", gateway.GatewayUUID.String(),
		[]uuid.UUID{current.DeploymentID})
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestArtifactListExisting(t *testing.T) {
	repo := NewArtifactRepo(db.GetDB())
	provider := seedPlacement(t, models.KindLLMProvider)
	proxy := seedArtifactOn(t, provider, provider.OUID, models.KindLLMProxy)
	notAProxy := seedArtifactOn(t, provider, provider.OUID, models.KindMCPMapping)
	otherOrg := seedArtifactOn(t, provider, "ou-"+uuid.NewString()[:8], models.KindLLMProxy)

	got, err := repo.ListExisting(context.Background(), provider.OUID,
		[]uuid.UUID{provider.ArtifactUUID, otherOrg.ArtifactUUID, uuid.New()},
		[]string{proxy.Handle, notAProxy.Handle, otherOrg.Handle})
	require.NoError(t, err)

	assert.ElementsMatch(t, []models.ArtifactRef{provider.ArtifactRef, proxy.ArtifactRef}, got)
}

// A gateway holds an artifact while its row names a deployment and isn't UNDEPLOYED. Rows it never held, or no
// longer holds, don't keep the gateway from being deleted.
func TestHasGatewayDeploymentsCountsOnlyWhatTheGatewayHolds(t *testing.T) {
	hasDeployments := func(p models.Placement) bool {
		t.Helper()
		has, err := NewGatewayRepo(db.GetDB()).HasGatewayDeployments(p.GatewayUUID.String(), p.OUID)
		require.NoError(t, err)
		return has
	}

	gateway := seedPlacement(t, models.KindLLMProvider)
	insertPendingRow(t, gateway)
	undeployed := seedArtifactOn(t, gateway, gateway.OUID, models.KindLLMProvider)
	startDeploy(t, undeployed)
	_, err := beginUndeploy(t, undeployed)
	require.NoError(t, err)
	applyAck(t, undeployed, undeployAck(undeployed.ArtifactUUID.String(), models.DeploymentAckStatusSuccess), nil)

	assert.False(t, hasDeployments(gateway), "only PENDING and UNDEPLOYED rows")

	for name, hold := range map[string]func(models.Placement){
		"deploying": func(p models.Placement) { startDeploy(t, p) },
		"undeploying": func(p models.Placement) {
			startDeploy(t, p)
			_, err := beginUndeploy(t, p)
			require.NoError(t, err)
		},
		"failed": func(p models.Placement) {
			deployment := startDeploy(t, p)
			applyAck(t, p, deployAck(deployment.DeploymentID, models.DeploymentAckStatusFailed, "E"), nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			p := seedPlacement(t, models.KindLLMProvider)
			hold(p)
			assert.True(t, hasDeployments(p))
		})
	}
}
