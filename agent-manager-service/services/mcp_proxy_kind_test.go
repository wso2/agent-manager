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
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
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
	kind := NewMCPProxyKind(gateways, f.sync.repo, nil)
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
	// A public address the placement pass's private-upstream check accepts without a DNS lookup.
	proxy.Endpoints[0].Configuration.Upstream.URL = "https://1.1.1.1/mcp"
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
			"v1.0", f.sync.ouID, nil, nil)
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

func TestMCPWorkerRefreshesExistingToolScopePolicies(t *testing.T) {
	f := newMCPFixture(t)
	ctx := t.Context()
	database := db.GetDB()
	endpoint := &f.proxy.Endpoints[0]
	endpoint.Configuration.Security = identityEnabledSecurity()
	endpoint.Configuration.Upstream.Auth = nil
	require.NoError(t, database.Model(endpoint).Update("configuration", endpoint.Configuration).Error)
	const tool = "get_forecast"
	scopes := []models.MCPProxyScope{
		{MCPProxyUUID: f.proxy.UUID, Action: "read", Tools: []string{tool}},
		{MCPProxyUUID: f.proxy.UUID, Action: "review", Tools: []string{tool}},
	}
	require.NoError(t, database.Create(&scopes).Error)
	read, review := scopes[0].ScopeString(mcpSourceHandle(f.proxy)), scopes[1].ScopeString(mcpSourceHandle(f.proxy))

	ref := models.ArtifactRef{ArtifactUUID: f.artifact, OUID: f.sync.ouID, Kind: models.KindMCPMapping}
	current, err := f.svc.kind.Render(ctx, ref)
	require.NoError(t, err)
	var legacy MCPProxyDeploymentYAML
	require.NoError(t, yaml.Unmarshal(current.Content, &legacy))
	authz := findMCPPolicy(legacy.Spec.Policies, mcpAuthzPolicyName)
	require.NotNil(t, authz)
	authz.Params["tools"] = []map[string]any{{"name": tool, "requiredScopes": []string{read, review}}}
	legacyContent, err := yaml.Marshal(legacy)
	require.NoError(t, err)

	type placement struct {
		models.Placement
		originalID uuid.UUID
	}
	seed := func(content []byte, ack string) placement {
		t.Helper()
		gateway := f.mapGateway(t, f.env)
		t.Cleanup(func() {
			database.Exec("DELETE FROM eventhub_gateway_states WHERE gateway_id = ?", gateway.String())
		})
		p := models.Placement{ArtifactRef: ref, GatewayUUID: gateway}
		deployment := &models.Deployment{Name: current.Name, Content: content}
		require.NoError(t, database.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			return f.sync.repo.StartDeploy(ctx, tx, p, deployment)
		}))
		if ack != "" {
			applied, err := f.sync.repo.ApplyAck(ctx, gateway.String(), models.DeploymentAck{
				DeploymentID: deployment.DeploymentID.String(), Action: models.DeploymentAckActionDeploy, Status: ack,
			}, nil)
			require.NoError(t, err)
			require.True(t, applied)
		}
		return placement{Placement: p, originalID: deployment.DeploymentID}
	}
	deployed := seed(legacyContent, models.DeploymentAckStatusSuccess)
	inProgress := seed(legacyContent, "")
	failed := seed(legacyContent, models.DeploymentAckStatusFailed)
	alreadyCurrent := seed(current.Content, models.DeploymentAckStatusSuccess)
	removed := seed(legacyContent, models.DeploymentAckStatusSuccess)
	_, err = f.sync.repo.BeginUndeploy(ctx, nil, removed.Placement)
	require.NoError(t, err)

	logger := slog.New(slog.DiscardHandler)
	hub, err := eventhub.NewSQLBackend(database, logger, eventhub.DefaultSQLBackendConfig())
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, hub.Close()) })
	core := reconcile.NewCore(database, f.sync.repo, hub, reconcile.Kinds{f.svc.kind}, reconcile.Config{
		RenderInterval: 10 * time.Millisecond, PlaceInterval: time.Hour, DriftInterval: time.Second,
		Lease: time.Minute, AttemptBudget: 3,
	}, logger)
	worker := reconcile.NewWorker(core, logger)
	require.NoError(t, worker.Start(ctx))
	t.Cleanup(func() { assert.NoError(t, worker.Stop()) })

	refresh := []placement{deployed, inProgress, failed}
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		for _, p := range refresh {
			row, err := f.sync.repo.GetStatusRecord(ctx, p.Placement)
			if assert.NoError(c, err) && assert.NotNil(c, row) && assert.NotNil(c, row.DeploymentID) {
				assert.NotEqual(c, p.originalID, *row.DeploymentID)
			}
		}
	}, 8*time.Second, 25*time.Millisecond)
	require.NoError(t, worker.Stop())

	for _, p := range refresh {
		pulled, err := f.sync.repo.GetPullable(ctx, p.OUID, p.ArtifactUUID, p.GatewayUUID.String())
		require.NoError(t, err)
		require.NotNil(t, pulled)
		assert.Equal(t, current.Content, pulled.Content)
		var eventData string
		require.NoError(t, database.Raw(`SELECT event_data FROM eventhub_events
			WHERE gateway_id = ? AND entity_type = 'mcpproxy.deployed' AND entity_id = ?`,
			p.GatewayUUID.String(), p.ArtifactUUID.String()).Scan(&eventData).Error)
		assert.Equal(t, pulled.DeploymentID.String(), eventPayload(t, eventData)["deploymentId"])
		applied, err := f.sync.repo.ApplyAck(ctx, p.GatewayUUID.String(), models.DeploymentAck{
			DeploymentID: pulled.DeploymentID.String(), Action: models.DeploymentAckActionDeploy, Status: models.DeploymentAckStatusSuccess,
		}, nil)
		require.NoError(t, err)
		require.True(t, applied)
	}
	pulled, err := f.sync.svc.GetActiveMCPProxyDeploymentByGateway(ctx, ref.ArtifactUUID.String(), ref.OUID, deployed.GatewayUUID.String())
	require.NoError(t, err)
	var refreshed MCPProxyDeploymentYAML
	require.NoError(t, yaml.Unmarshal([]byte(pulled[ref.ArtifactUUID.String()]), &refreshed))
	refreshedPolicy := findMCPPolicy(refreshed.Spec.Policies, mcpAuthzPolicyName)
	require.NotNil(t, refreshedPolicy)
	assert.Equal(t, []any{
		map[string]any{"name": tool, "requiredScopes": []any{read}},
		map[string]any{"name": tool, "requiredScopes": []any{review}},
	}, refreshedPolicy.Params["tools"])

	_, err = worker.CheckDrift(ctx)
	require.NoError(t, err)
	for _, p := range refresh {
		row := f.mcpRow(t, p.GatewayUUID)
		assert.Equal(t, models.DeploymentStatusDeployed, row.Status)
		assert.Nil(t, row.NextAttemptAt, "a refreshed placement stays settled")
	}
	for _, p := range []placement{alreadyCurrent, removed} {
		row := f.mcpRow(t, p.GatewayUUID)
		assert.Equal(t, p.originalID, *row.DeploymentID)
		assert.Nil(t, row.NextAttemptAt)
	}
	assert.Equal(t, models.DeploymentStatusUndeployed, f.mcpRow(t, removed.GatewayUUID).StatusDesired)
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

// bindingRows counts the fixture proxy's (endpoint, environment) rows and endpoint rows still stored.
func (f *mcpFixture) bindingRows(t *testing.T) (environments, endpoints int64) {
	t.Helper()
	gormDB := db.GetDB()
	require.NoError(t, gormDB.Table("mcp_proxy_endpoint_environments").
		Where("artifact_uuid = ?", f.artifact).Count(&environments).Error)
	require.NoError(t, gormDB.Table("mcp_proxy_endpoints").
		Where("uuid = ?", f.proxy.Endpoints[0].UUID).Count(&endpoints).Error)
	return environments, endpoints
}

// When an environment goes and its gateway artifact can't be deleted yet, a project proxy keeps the binding and its
// endpoint, so the retry still finds the artifact; once the artifact is gone, both go.
func TestRemoveEnvironmentFromEndpoints_ProjectProxyKeepsBindingUntilArtifactGone(t *testing.T) {
	f := newMCPFixture(t)
	project := uuid.NewString()
	f.proxy.ProjectUUID = &project
	tx := f.svc.tx
	f.svc.tx = func(context.Context, func(*gorm.DB) error) error { return errors.New("gateway unreachable") }

	require.Error(t, f.svc.RemoveEnvironmentFromEndpoints(context.Background(), f.proxy, f.env, f.sync.ouID))
	environments, endpoints := f.bindingRows(t)
	assert.EqualValues(t, 1, environments, "the binding stays, so a retry can find the artifact")
	assert.EqualValues(t, 1, endpoints)

	f.svc.tx = tx
	require.NoError(t, f.svc.RemoveEnvironmentFromEndpoints(context.Background(), f.proxy, f.env, f.sync.ouID))
	environments, endpoints = f.bindingRows(t)
	assert.Zero(t, environments, "the retry removes the binding")
	assert.Zero(t, endpoints, "and the project binding's endpoint")
}

// An org proxy removes the binding even when its artifact can't be deleted yet, and keeps the endpoint, as before.
func TestRemoveEnvironmentFromEndpoints_OrgProxyUnchanged(t *testing.T) {
	f := newMCPFixture(t)
	f.svc.tx = func(context.Context, func(*gorm.DB) error) error { return errors.New("gateway unreachable") }

	require.Error(t, f.svc.RemoveEnvironmentFromEndpoints(context.Background(), f.proxy, f.env, f.sync.ouID))
	environments, endpoints := f.bindingRows(t)
	assert.Zero(t, environments)
	assert.EqualValues(t, 1, endpoints)
}
