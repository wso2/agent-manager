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
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// mcpFixture is an org with an MCP proxy whose one endpoint is bound to env, and the MCP service over the real
// repositories.
type mcpFixture struct {
	*proxyFixture
	svc      *MCPProxyService
	proxy    *models.MCPProxy
	env      uuid.UUID
	artifact uuid.UUID
}

func newMCPFixture(t *testing.T) *mcpFixture {
	t.Helper()
	f := &mcpFixture{proxyFixture: newProxyFixture(t)}
	gormDB := db.GetDB()
	gateways := repositories.NewGatewayRepo(gormDB)
	kind := NewMCPProxyKind(gateways, f.sync.repo)
	core := reconcile.NewCore(gormDB, f.sync.repo, f.hub, reconcile.Kinds{kind},
		reconcile.Config{Lease: time.Minute, AttemptBudget: 3}, slog.New(slog.DiscardHandler))
	f.worker = reconcile.NewWorker(core, slog.New(slog.DiscardHandler))
	f.svc = &MCPProxyService{
		db: gormDB, tx: NewTxRunner(gormDB), repo: repositories.NewMCPProxyRepo(gormDB), deploymentRepo: f.sync.repo,
		gatewayRepo: gateways, artifactRepo: repositories.NewArtifactRepo(gormDB),
		mcpProxyScopeRepo: repositories.NewMCPProxyScopeRepository(gormDB), logger: slog.New(slog.DiscardHandler),
		endpointRepo: repositories.NewMCPProxyEndpointRepository(gormDB), core: core, kind: kind,
		envMCPMappingRepo: repositories.NewEnvAgentMCPMappingRepository(gormDB),
	}

	proxy, _ := goldenMCPProxy()
	proxy.UUID = uuid.New()
	proxy.Handle = "mcp-" + proxy.UUID.String()[:8]
	f.env, f.artifact = uuid.New(), uuid.New()
	proxy.Endpoints[0].UUID = uuid.New()
	proxy.Endpoints[0].Environments[0].EnvironmentUUID = f.env
	proxy.Endpoints[0].Environments[0].ArtifactUUID = f.artifact
	proxy.Endpoints[0].Environments[0].MCPProxyUUID = proxy.UUID
	require.NoError(t, gormDB.Transaction(func(tx *gorm.DB) error {
		handle := mcpProxyEnvArtifactHandle(proxy.Handle, "primary", f.env.String())
		if err := repositories.NewArtifactRepo(tx).Create(tx, &models.Artifact{
			UUID: f.artifact, Handle: handle, Name: handle, Version: "v1.0", Kind: models.KindMCPMapping, OUID: f.sync.ouID,
		}); err != nil {
			return err
		}
		return repositories.NewMCPProxyRepo(gormDB).Create(context.Background(), tx, proxy, proxy.Handle, proxy.Handle,
			"v1.0", f.sync.ouID)
	}))
	t.Cleanup(func() {
		gormDB.Exec(`DELETE FROM artifacts WHERE uuid IN (?, ?)`, proxy.UUID, f.artifact)
	})
	loaded, err := f.svc.repo.GetByUUID(context.Background(), proxy.UUID.String(), f.sync.ouID)
	require.NoError(t, err)
	f.proxy = loaded
	return f
}

// deploy deploys the fixture proxy's bindings as a save does, in one transaction.
func (f *mcpFixture) deploy(t *testing.T) error {
	t.Helper()
	return NewTxRunner(db.GetDB())(context.Background(), func(tx *gorm.DB) error {
		return f.svc.deployMCPProxyEndpoints(context.Background(), tx, f.proxy, f.sync.ouID)
	})
}

func (f *mcpFixture) mcpRow(t *testing.T, gateway uuid.UUID) *models.DeploymentStatusRecord {
	t.Helper()
	return f.row(t, &models.LLMProxy{UUID: f.artifact}, gateway)
}

// A request's explicit gateway is the row's gateway, even with a second egress gateway in the environment.
func TestMCPRequestedGatewayBecomesTheRowsGateway(t *testing.T) {
	f := newMCPFixture(t)
	f.mapGateway(t, f.env)
	requested := f.mapGateway(t, f.env)
	requestedID := requested.String()
	f.proxy.Endpoints[0].Environments[0].RequestedGatewayUUID = &requestedID

	require.NoError(t, f.deploy(t))

	row := f.mcpRow(t, requested)
	require.NotNil(t, row)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
	placed, err := f.sync.repo.PlacedGateways(context.Background(), f.sync.ouID, f.artifact)
	require.NoError(t, err)
	assert.Equal(t, []string{requestedID}, placed)
}

// A scope change re-renders the proxy's placed artifacts before the request returns.
func TestMCPScopeChangeReRendersInline(t *testing.T) {
	f := newMCPFixture(t)
	gateway := f.mapGateway(t, f.env)
	require.NoError(t, f.deploy(t))
	before := *f.mcpRow(t, gateway).DeploymentID
	require.NoError(t, db.GetDB().Create(&models.MCPProxyScope{
		MCPProxyUUID: f.proxy.UUID, Action: "read", Tools: []string{"get_forecast"},
	}).Error)
	enabled := true
	f.proxy.Endpoints[0].Configuration.Security = &models.SecurityConfig{
		Enabled:  &enabled,
		Identity: &models.IdentitySecurity{Enabled: &enabled},
	}
	require.NoError(t, db.GetDB().Model(&models.MCPProxyEndpoint{}).Where("uuid = ?", f.proxy.Endpoints[0].UUID).
		Update("configuration", f.proxy.Endpoints[0].Configuration).Error)

	require.NoError(t, f.svc.RedeployMCPProxy(context.Background(), f.proxy, f.sync.ouID))

	row := f.mcpRow(t, gateway)
	assert.NotEqual(t, before, *row.DeploymentID, "the scope change rendered a new deployment")
	assert.Nil(t, row.NextAttemptAt, "rendered inline, nothing left owed")
	pulled, err := f.sync.repo.GetPullable(context.Background(), f.sync.ouID, f.artifact, gateway.String())
	require.NoError(t, err)
	assert.Contains(t, string(pulled.Content), "get_forecast")
}

// A scope change whose configuration doesn't render reaches the caller, and no gateway is sent anything.
func TestMCPScopeChangeThatFailsToRenderReachesTheCaller(t *testing.T) {
	f := newMCPFixture(t)
	gateway := f.mapGateway(t, f.env)
	require.NoError(t, f.deploy(t))
	before := *f.mcpRow(t, gateway).DeploymentID
	published := len(f.hub.published)
	f.proxy.Endpoints[0].Configuration.Upstream = nil
	require.NoError(t, db.GetDB().Model(&models.MCPProxyEndpoint{}).Where("uuid = ?", f.proxy.Endpoints[0].UUID).
		Update("configuration", f.proxy.Endpoints[0].Configuration).Error)

	err := f.svc.RedeployMCPProxy(context.Background(), f.proxy, f.sync.ouID)

	require.ErrorIs(t, err, utils.ErrInvalidInput)
	row := f.mcpRow(t, gateway)
	assert.Equal(t, before, *row.DeploymentID, "no deployment was written")
	assert.Nil(t, row.NextAttemptAt, "no redeploy was queued")
	assert.Len(t, f.hub.published, published, "no event was written")
}

// The placement pass places a binding whose environment gains a gateway, and a deleted binding's artifact sends its
// delete event in the deleting transaction.
func TestMCPPlacementAndDelete(t *testing.T) {
	f := newMCPFixture(t)
	require.NoError(t, f.deploy(t))
	gateway := f.mapGateway(t, f.env)
	assert.Nil(t, f.mcpRow(t, gateway), "no gateway, no row")

	require.NoError(t, f.worker.PlaceEnv(context.Background(), f.sync.ouID, f.env))
	require.NotNil(t, f.mcpRow(t, gateway))
	assert.Equal(t, models.DeploymentStatusDeploying, f.mcpRow(t, gateway).Status)

	require.NoError(t, f.svc.deleteMCPProxyEnvironmentArtifacts(context.Background(), []uuid.UUID{f.artifact}, f.sync.ouID))

	deletedOn := map[string]bool{}
	for _, evt := range f.hub.published {
		if evt.EventType == "mcpproxy.deleted" {
			deletedOn[evt.GatewayID] = true
		}
	}
	assert.True(t, deletedOn[gateway.String()], "the gateway it was placed on is told")
	assert.Nil(t, f.mcpRow(t, gateway))
}

// Tearing an agent's MCP binding down writes its delete events last in the transaction that deletes its artifact.
func TestAgentMCPBindingTeardownDeletesInTheDeletingTransaction(t *testing.T) {
	f := newMCPFixture(t)
	gateway := f.mapGateway(t, f.env)
	require.NoError(t, f.deploy(t))
	published := len(f.hub.published)
	var artifactsSeen []int64
	f.hub.onPublish = func() {
		tx := f.hub.txs[len(f.hub.txs)-1]
		require.NotNil(t, tx)
		var count int64
		require.NoError(t, tx.Raw(`SELECT count(*) FROM artifacts WHERE uuid = ?`, f.artifact).Scan(&count).Error)
		artifactsSeen = append(artifactsSeen, count)
	}
	svc := &agentConfigurationService{db: db.GetDB(), mcpProxyService: f.svc, logger: slog.New(slog.DiscardHandler)}

	svc.cleanupNewMCPMapping(context.Background(), &models.AgentConfiguration{},
		&models.EnvAgentMCPMapping{EnvironmentUUID: f.env, ArtifactUUID: f.artifact}, "", f.sync.ouID, false)

	deletedOn := map[string]bool{}
	for _, evt := range f.hub.published[published:] {
		assert.Equal(t, "mcpproxy.deleted", string(evt.EventType))
		deletedOn[evt.GatewayID] = true
	}
	assert.True(t, deletedOn[gateway.String()], "the gateway it was placed on is told")
	require.NotEmpty(t, artifactsSeen)
	for _, seen := range artifactsSeen {
		assert.Zero(t, seen, "the events are written after the artifact is deleted, in its transaction")
	}
	assert.Nil(t, f.mcpRow(t, gateway))
}

// invalidMCPRequest is a proxy whose one endpoint, bound to env, takes its API key from a cookie: it only fails
// when rendered.
func invalidMCPRequest(handle string, env uuid.UUID) *models.MCPProxyDTO {
	enabled := true
	return &models.MCPProxyDTO{
		ID: handle, Name: handle, Version: "v1.0",
		Endpoints: []models.MCPProxyEndpointDTO{{
			ID:           "primary",
			Upstream:     models.UpstreamConfig{Main: &models.UpstreamEndpoint{URL: "https://1.1.1.1/mcp"}},
			Security:     &models.SecurityConfig{Enabled: &enabled, APIKey: &models.APIKeySecurity{Enabled: &enabled, In: "cookie"}},
			Environments: []models.MCPEndpointEnvironmentDTO{{EnvironmentUUID: env.String()}},
		}},
	}
}

// An MCP save whose artifact can't render is refused with a 4xx and commits nothing, with or without a gateway.
func TestMCPSaveThatCannotRenderCommitsNothing(t *testing.T) {
	for _, withGateway := range []bool{false, true} {
		name := map[bool]string{false: "without a gateway", true: "with a gateway"}[withGateway]
		t.Run("create "+name, func(t *testing.T) {
			f := newMCPFixture(t)
			env := uuid.New()
			if withGateway {
				f.mapGateway(t, env)
			}
			handle := "bad-" + uuid.NewString()[:8]
			t.Cleanup(func() { db.GetDB().Exec(`DELETE FROM artifacts WHERE handle LIKE ?`, handle+"%") })
			published := len(f.hub.published)

			_, err := f.svc.Create(context.Background(), f.sync.ouID, "me", invalidMCPRequest(handle, env))

			assert.ErrorIs(t, err, utils.ErrInvalidInput)
			var artifacts int64
			require.NoError(t, db.GetDB().Table("artifacts").Where("handle LIKE ?", handle+"%").Count(&artifacts).Error)
			assert.Zero(t, artifacts, "neither the proxy nor its environment artifact is committed")
			assert.Len(t, f.hub.published, published, "no event")
		})
		t.Run("update "+name, func(t *testing.T) {
			f := newMCPFixture(t)
			if withGateway {
				f.mapGateway(t, f.env)
			}
			published := len(f.hub.published)

			handle := mcpSourceHandle(f.proxy)
			_, err := f.svc.Update(context.Background(), f.sync.ouID, handle, invalidMCPRequest(handle, f.env))

			assert.ErrorIs(t, err, utils.ErrInvalidInput)
			stored, err := f.svc.repo.GetByUUID(context.Background(), f.proxy.UUID.String(), f.sync.ouID)
			require.NoError(t, err)
			require.Len(t, stored.Endpoints, 1)
			assert.Equal(t, f.proxy.Endpoints[0].UUID, stored.Endpoints[0].UUID, "the endpoint set is not replaced")
			var rows int64
			require.NoError(t, db.GetDB().Model(&models.DeploymentStatusRecord{}).
				Where("artifact_uuid = ?", f.artifact).Count(&rows).Error)
			assert.Zero(t, rows)
			assert.Len(t, f.hub.published, published, "no event")
		})
	}
}

// A binding shows MCP's word for the derived status: a fresh deploy is Deploying until the gateway acks it, and the
// binding names its placed gateway at once.
func TestMCPBindingShowsDeployingUntilTheGatewayAcks(t *testing.T) {
	f := newMCPFixture(t)
	gateway := f.mapGateway(t, f.env)
	binding := func() models.MCPEndpointEnvironmentDTO {
		t.Helper()
		dto, err := f.svc.Get(context.Background(), f.sync.ouID, proxyHandleOf(f.proxy))
		require.NoError(t, err)
		require.Len(t, dto.Endpoints, 1)
		require.Len(t, dto.Endpoints[0].Environments, 1)
		return dto.Endpoints[0].Environments[0]
	}
	assert.Equal(t, models.MCPDeploymentStatusUndeployed, binding().DeploymentStatus, "nothing placed yet")

	require.NoError(t, f.deploy(t))

	got := binding()
	assert.Equal(t, models.MCPDeploymentStatusDeploying, got.DeploymentStatus)
	require.NotNil(t, got.GatewayID)
	assert.Equal(t, gateway.String(), *got.GatewayID)

	row := f.mcpRow(t, gateway)
	_, err := f.sync.repo.ApplyAck(context.Background(), gateway.String(), models.DeploymentAck{
		DeploymentID: row.DeploymentID.String(), Action: models.DeploymentAckActionDeploy,
		Status: models.DeploymentAckStatusSuccess,
	}, nil)
	require.NoError(t, err)
	assert.Equal(t, models.MCPDeploymentStatusDeployed, binding().DeploymentStatus)
}
