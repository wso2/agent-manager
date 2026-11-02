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

package services

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// syncFixture is an org with one gateway, and the gateway internal service over the real repositories.
type syncFixture struct {
	ouID    string
	gateway uuid.UUID
	svc     *GatewayInternalAPIService
	repo    repositories.DeploymentRepository
}

func newSyncFixture(t *testing.T) *syncFixture {
	t.Helper()
	f := &syncFixture{ouID: "ou-" + uuid.NewString()[:8], repo: repositories.NewDeploymentRepo(db.GetDB())}
	f.gateway = f.seedGateway(t, f.ouID)
	f.svc = NewGatewayInternalAPIService(nil, nil, f.repo, repositories.NewArtifactRepo(db.GetDB()), nil, nil, nil,
		gatewayKinds())
	return f
}

func (f *syncFixture) seedGateway(t *testing.T, ouID string) uuid.UUID {
	t.Helper()
	gatewayID := uuid.New()
	name := "sync-gw-" + gatewayID.String()[:8]
	require.NoError(t, db.GetDB().Exec(
		`INSERT INTO gateways (uuid, name, display_name, vhost, gateway_functionality_type, ou_id, is_active)
		 VALUES (?, ?, ?, ?, ?, ?, TRUE)`,
		gatewayID, name, name, "gw.example", models.GatewayRoleEgress, ouID).Error)
	t.Cleanup(func() { db.GetDB().Exec(`DELETE FROM gateways WHERE uuid = ?`, gatewayID) })
	return gatewayID
}

func (f *syncFixture) seedArtifact(t *testing.T, ouID, kind string) models.ArtifactRef {
	t.Helper()
	artifactUUID := uuid.New()
	handle := "sync-" + artifactUUID.String()[:8]
	require.NoError(t, repositories.NewArtifactRepo(db.GetDB()).Create(db.GetDB(), &models.Artifact{
		UUID: artifactUUID, Handle: handle, Name: handle, Version: "v1.0", Kind: kind, OUID: ouID,
	}))
	t.Cleanup(func() { db.GetDB().Exec(`DELETE FROM artifacts WHERE uuid = ?`, artifactUUID) })
	return models.ArtifactRef{OUID: ouID, ArtifactUUID: artifactUUID, Kind: kind, Handle: handle}
}

func (f *syncFixture) deploy(t *testing.T, ref models.ArtifactRef, gateway uuid.UUID) (models.Placement, *models.Deployment) {
	t.Helper()
	p := models.Placement{ArtifactRef: ref, GatewayUUID: gateway}
	deployment := &models.Deployment{Name: "d", Content: []byte("spec:\n  name: " + ref.Handle + "\n")}
	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		return f.repo.StartDeploy(context.Background(), tx, p, deployment)
	}))
	return p, deployment
}

// statusSnapshot is every column of the gateway's status rows, plus its event hub sequence and event count.
type statusSnapshot struct {
	Rows   []map[string]any
	Seq    int64
	Events int64
}

func (f *syncFixture) snapshot(t *testing.T) statusSnapshot {
	t.Helper()
	var s statusSnapshot
	require.NoError(t, db.GetDB().Raw(
		`SELECT * FROM deployment_status WHERE gateway_uuid = ? ORDER BY artifact_uuid`, f.gateway).
		Scan(&s.Rows).Error)
	require.NoError(t, db.GetDB().Raw(
		`SELECT COALESCE(MAX(seq), 0) FROM eventhub_gateway_states WHERE gateway_id = ?`, f.gateway.String()).
		Scan(&s.Seq).Error)
	require.NoError(t, db.GetDB().Raw(
		`SELECT COUNT(*) FROM eventhub_events WHERE gateway_id = ?`, f.gateway.String()).Scan(&s.Events).Error)
	return s
}

func TestListGatewayDeploymentsOverTheDatabase(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()

	proxy := f.seedArtifact(t, f.ouID, models.KindLLMProxy)
	proxyPlacement, proxyDeployment := f.deploy(t, proxy, f.gateway)
	_, err := f.repo.ApplyAck(ctx, f.gateway.String(), models.DeploymentAck{
		DeploymentID: proxyDeployment.DeploymentID.String(), Action: models.DeploymentAckActionDeploy,
		Status: models.DeploymentAckStatusSuccess,
	}, nil)
	require.NoError(t, err)

	provider := f.seedArtifact(t, f.ouID, models.KindLLMProvider)
	providerPlacement, _ := f.deploy(t, provider, f.gateway)
	require.NoError(t, db.GetDB().Transaction(func(tx *gorm.DB) error {
		_, err := f.repo.BeginUndeploy(ctx, tx, providerPlacement)
		return err
	}))
	// Old enough for the sweeper, which this read must not stand in for.
	require.NoError(t, db.GetDB().Exec(
		`UPDATE deployment_status SET performed_at = performed_at - INTERVAL '1 hour' WHERE gateway_uuid = ?`,
		f.gateway).Error)

	before := f.snapshot(t)
	time.Sleep(10 * time.Millisecond) // so a write's NOW() would differ

	got, err := f.svc.ListGatewayDeployments(ctx, f.ouID, f.gateway.String())
	require.NoError(t, err)

	assert.Equal(t, before, f.snapshot(t), "GET /deployments writes nothing")
	require.Len(t, got, 2)
	byKind := map[string]models.ControlPlaneDeployment{}
	for _, d := range got {
		byKind[d.Kind] = d
	}
	proxyEntry := byKind[models.KindLLMProxy]
	assert.Equal(t, proxy.Handle, proxyEntry.ArtifactID, "the gateway stores an LLM proxy under its handle")
	assert.Equal(t, proxyDeployment.DeploymentID.String(), proxyEntry.DeploymentID)
	assert.Equal(t, models.GatewaySyncStateDeployed, proxyEntry.State)
	row, err := f.repo.GetStatusRecord(ctx, proxyPlacement)
	require.NoError(t, err)
	assert.True(t, row.PerformedAt.Equal(proxyEntry.DeployedAt))

	providerEntry := byKind[models.KindLLMProvider]
	assert.Equal(t, provider.ArtifactUUID.String(), providerEntry.ArtifactID)
	assert.Equal(t, models.GatewaySyncStateUndeployed, providerEntry.State)
	assert.Empty(t, providerEntry.DeploymentID)
}

func TestFetchDeploymentBatchOverTheDatabase(t *testing.T) {
	f := newSyncFixture(t)
	proxy := f.seedArtifact(t, f.ouID, models.KindLLMProxy)
	_, deployment := f.deploy(t, proxy, f.gateway)
	otherGateway := f.seedGateway(t, f.ouID)
	_, elsewhere := f.deploy(t, f.seedArtifact(t, f.ouID, models.KindLLMProvider), otherGateway)

	got, err := f.svc.FetchDeploymentBatch(context.Background(), f.ouID, f.gateway.String(),
		[]string{deployment.DeploymentID.String(), elsewhere.DeploymentID.String(), ""})
	require.NoError(t, err)

	require.Len(t, got, 1, "another gateway's deployment is skipped, not an error")
	assert.Equal(t, deployment.DeploymentID.String(), got[0].DeploymentID)
	assert.Equal(t, proxy.Handle, got[0].ArtifactID)
	assert.Equal(t, models.KindLLMProxy, got[0].Kind)
	assert.Equal(t, deployment.Content, got[0].Content)
}

func TestCheckArtifactsExistOverTheDatabase(t *testing.T) {
	f := newSyncFixture(t)
	ctx := context.Background()

	noStatusRow := f.seedArtifact(t, f.ouID, models.KindLLMProvider)
	onAnotherGateway := f.seedArtifact(t, f.ouID, models.KindMCPMapping)
	f.deploy(t, onAnotherGateway, f.seedGateway(t, f.ouID))
	proxy := f.seedArtifact(t, f.ouID, models.KindLLMProxy)
	deleted := f.seedArtifact(t, f.ouID, models.KindLLMProvider)
	require.NoError(t, repositories.NewArtifactRepo(db.GetDB()).Delete(db.GetDB(), deleted.ArtifactUUID.String()))
	deletedProxy := f.seedArtifact(t, f.ouID, models.KindLLMProxy)
	require.NoError(t, repositories.NewArtifactRepo(db.GetDB()).Delete(db.GetDB(), deletedProxy.ArtifactUUID.String()))
	otherOrgID := "ou-" + uuid.NewString()[:8]
	otherOrg := f.seedArtifact(t, otherOrgID, models.KindLLMProvider)
	otherOrgProxy := f.seedArtifact(t, otherOrgID, models.KindLLMProxy)
	require.NoError(t, db.GetDB().Exec(`UPDATE artifacts SET handle = ? WHERE uuid = ?`,
		deletedProxy.Handle, otherOrgProxy.ArtifactUUID).Error, "another org's proxy holds the deleted proxy's handle")

	got, err := f.svc.CheckArtifactsExist(ctx, f.ouID, []string{
		noStatusRow.ArtifactUUID.String(), onAnotherGateway.ArtifactUUID.String(), proxy.Handle,
		deleted.ArtifactUUID.String(), deletedProxy.Handle, otherOrg.ArtifactUUID.String(),
	})
	require.NoError(t, err)

	assert.Equal(t, []models.ArtifactExistence{
		{ArtifactID: noStatusRow.ArtifactUUID.String(), Exists: true},
		{ArtifactID: onAnotherGateway.ArtifactUUID.String(), Exists: true},
		{ArtifactID: proxy.Handle, Exists: true},
		{ArtifactID: deleted.ArtifactUUID.String(), Exists: false},
		{ArtifactID: deletedProxy.Handle, Exists: false},
		{ArtifactID: otherOrg.ArtifactUUID.String(), Exists: false},
	}, got)
}
