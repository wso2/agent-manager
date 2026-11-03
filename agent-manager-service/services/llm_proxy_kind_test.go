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
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// proxyFixture is an org with a provider and the LLM proxy services over the real repositories.
type proxyFixture struct {
	*providerFixture
	provider *models.LLMProvider
	kind     *LLMProxyKind
	worker   *reconcile.Worker
	proxies  *LLMProxyDeploymentService
}

func newProxyFixture(t *testing.T) *proxyFixture {
	t.Helper()
	f := &proxyFixture{providerFixture: newProviderFixture(t)}
	gormDB := db.GetDB()
	gateways := repositories.NewGatewayRepo(gormDB)
	f.kind = NewLLMProxyKind(gateways, f.sync.repo)
	core := reconcile.NewCore(gormDB, f.sync.repo, f.hub, reconcile.Kinds{NewLLMProviderKind(), f.kind},
		reconcile.Config{Lease: time.Minute, AttemptBudget: 3}, slog.New(slog.DiscardHandler))
	f.worker = reconcile.NewWorker(core, slog.New(slog.DiscardHandler))
	f.proxies = NewLLMProxyDeploymentService(NewTxRunner(gormDB), f.sync.repo, repositories.NewLLMProxyRepo(gormDB),
		repositories.NewLLMProviderRepo(gormDB), gateways, nil, core, f.kind)
	created, err := f.providers.Create(context.Background(), f.sync.ouID, "me", f.newProvider(t))
	require.NoError(t, err)
	f.provider = created
	return f
}

// mapGateway adds an egress gateway mapped to env.
func (f *proxyFixture) mapGateway(t *testing.T, env uuid.UUID) uuid.UUID {
	t.Helper()
	gateway := f.sync.seedGateway(t, f.sync.ouID)
	require.NoError(t, db.GetDB().Exec(
		`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`, gateway, env).Error)
	return gateway
}

func (f *proxyFixture) newProxy(t *testing.T) *models.LLMProxy {
	t.Helper()
	proxy := goldenProxy()
	proxy.UUID = uuid.Nil
	proxy.Handle = "px-" + uuid.NewString()[:8]
	proxy.ProjectUUID = uuid.New()
	proxy.ProviderUUID = f.provider.UUID
	proxy.Configuration.Provider = f.provider.UUID.String()
	require.NoError(t, repositories.NewLLMProxyRepo(db.GetDB()).Create(context.Background(), proxy, proxy.Handle,
		proxy.Handle, "v1.0", f.sync.ouID))
	t.Cleanup(func() { db.GetDB().Exec(`DELETE FROM artifacts WHERE uuid = ?`, proxy.UUID) })
	return proxy
}

// mapToAgent saves the agent-config record that says proxy should be deployed in env.
func (f *proxyFixture) mapToAgent(t *testing.T, proxy *models.LLMProxy, env uuid.UUID) {
	t.Helper()
	configUUID := uuid.New()
	require.NoError(t, db.GetDB().Exec(
		`INSERT INTO agent_configurations (uuid, name, agent_id, project_name, ou_id) VALUES (?, ?, ?, 'p', ?)`,
		configUUID, "c-"+configUUID.String()[:8], "a-"+configUUID.String()[:8], f.sync.ouID).Error)
	require.NoError(t, db.GetDB().Exec(
		`INSERT INTO env_agent_model_mapping (config_uuid, environment_uuid, llm_proxy_uuid) VALUES (?, ?, ?)`,
		configUUID, env, proxy.UUID).Error)
	t.Cleanup(func() { db.GetDB().Exec(`DELETE FROM agent_configurations WHERE uuid = ?`, configUUID) })
}

func (f *proxyFixture) row(t *testing.T, proxy *models.LLMProxy, gateway uuid.UUID) *models.DeploymentStatusRecord {
	t.Helper()
	row, err := f.sync.repo.GetStatusRecord(context.Background(), models.Placement{
		ArtifactRef: models.ArtifactRef{OUID: f.sync.ouID, ArtifactUUID: proxy.UUID}, GatewayUUID: gateway,
	})
	require.NoError(t, err)
	return row
}

// An agent-config proxy undeployed from one gateway through the public route keeps its other placement.
func TestAgentConfigProxyUndeployedOnOneGatewayKeepsTheOther(t *testing.T) {
	f := newProxyFixture(t)
	proxy := f.newProxy(t)
	envA, envB := uuid.New(), uuid.New()
	gatewayA, gatewayB := f.mapGateway(t, envA), f.mapGateway(t, envB)
	f.mapToAgent(t, proxy, envA)
	f.mapToAgent(t, proxy, envB)
	deployedA, err := f.proxies.DeployForEnvironment(context.Background(), proxy, f.sync.ouID, envA, gatewayA)
	require.NoError(t, err)
	_, err = f.proxies.DeployForEnvironment(context.Background(), proxy, f.sync.ouID, envB, gatewayB)
	require.NoError(t, err)

	_, err = f.proxies.UndeployLLMProxyDeployment(context.Background(), proxy.Handle,
		deployedA.DeploymentID.String(), gatewayA.String(), f.sync.ouID)
	require.NoError(t, err)

	assert.Equal(t, models.DeploymentStatusUndeploying, f.row(t, proxy, gatewayA).Status)
	assert.Equal(t, models.DeploymentStatusDeploying, f.row(t, proxy, gatewayB).Status)
	require.NoError(t, f.worker.PlaceEnv(context.Background(), f.sync.ouID, envA))
	assert.Equal(t, models.DeploymentStatusUndeployed, f.row(t, proxy, gatewayA).StatusDesired,
		"the placement pass doesn't re-place an undeployed proxy whose record remains")
}

// An agent's proxy whose security config can't render is refused before anything is written, whether or not its
// environment has a gateway.
func TestAgentConfigProxyWithAnInvalidSecurityConfigCommitsNothing(t *testing.T) {
	f := newProxyFixture(t)
	proxy := f.newProxy(t)
	proxy.Configuration.Security.APIKey.Key = ""
	require.NoError(t, repositories.NewLLMProxyRepo(db.GetDB()).Update(proxy, proxy.Handle, f.sync.ouID))

	t.Run("with a gateway", func(t *testing.T) {
		env := uuid.New()
		gateway := f.mapGateway(t, env)
		_, err := f.proxies.DeployForEnvironment(context.Background(), proxy, f.sync.ouID, env, gateway)
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
		assert.Nil(t, f.row(t, proxy, gateway))
	})
	t.Run("without a gateway", func(t *testing.T) {
		record := reconcile.Record{
			ArtifactRef:     models.ArtifactRef{OUID: f.sync.ouID, ArtifactUUID: proxy.UUID, Kind: models.KindLLMProxy, Handle: proxy.Handle},
			EnvironmentUUID: uuid.New(),
		}
		err := NewTxRunner(db.GetDB())(context.Background(), func(tx *gorm.DB) error {
			_, err := f.proxies.core.DeployRecord(context.Background(), tx, f.kind, record)
			return err
		})
		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})
	assert.Empty(t, f.hub.published)
}

// A provider auth-header change re-renders each dependent proxy where it is placed, and nothing else.
func TestRedeployEverywhereRendersOnlyPlacedRows(t *testing.T) {
	f := newProxyFixture(t)
	proxy, other := f.newProxy(t), f.newProxy(t)
	envA, envB := uuid.New(), uuid.New()
	gatewayA, gatewayB := f.mapGateway(t, envA), f.mapGateway(t, envB)
	deployedA, err := f.proxies.DeployForEnvironment(context.Background(), proxy, f.sync.ouID, envA, gatewayA)
	require.NoError(t, err)
	deployedB, err := f.proxies.DeployForEnvironment(context.Background(), proxy, f.sync.ouID, envB, gatewayB)
	require.NoError(t, err)
	otherDeployed, err := f.proxies.DeployForEnvironment(context.Background(), other, f.sync.ouID, envA, gatewayA)
	require.NoError(t, err)
	_, err = f.proxies.UndeployLLMProxyDeployment(context.Background(), proxy.Handle,
		deployedB.DeploymentID.String(), gatewayB.String(), f.sync.ouID)
	require.NoError(t, err)

	require.NoError(t, f.proxies.RedeployEverywhere(context.Background(), proxy, f.sync.ouID, "provider changed"))

	rowA := f.row(t, proxy, gatewayA)
	assert.NotEqual(t, deployedA.DeploymentID, *rowA.DeploymentID, "the placed row re-rendered")
	assert.Equal(t, models.DeploymentStatusDeploying, rowA.Status)
	assert.Equal(t, models.DeploymentStatusUndeploying, f.row(t, proxy, gatewayB).Status, "an undeployed row stays")
	assert.Equal(t, otherDeployed.DeploymentID, *f.row(t, other, gatewayA).DeploymentID, "another proxy is untouched")
}

// Tearing an agent's or monitor's proxy down undeploys every gateway it is placed on, whatever the row's status:
// a deploy the gateway hasn't acked yet is undeployed too.
func TestUndeployEverywhereUndeploysEveryPlacedRow(t *testing.T) {
	f := newProxyFixture(t)
	proxy := f.newProxy(t)
	envA, envB := uuid.New(), uuid.New()
	gatewayA, gatewayB := f.mapGateway(t, envA), f.mapGateway(t, envB)
	_, err := f.proxies.DeployForEnvironment(context.Background(), proxy, f.sync.ouID, envA, gatewayA)
	require.NoError(t, err)
	_, err = f.proxies.DeployForEnvironment(context.Background(), proxy, f.sync.ouID, envB, gatewayB)
	require.NoError(t, err)
	require.Equal(t, models.DeploymentStatusDeploying, f.row(t, proxy, gatewayA).Status)

	require.NoError(t, f.proxies.UndeployEverywhere(context.Background(), proxy.Handle, f.sync.ouID))

	for _, gateway := range []uuid.UUID{gatewayA, gatewayB} {
		row := f.row(t, proxy, gateway)
		assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
		assert.Equal(t, models.DeploymentStatusUndeployed, row.StatusDesired)
	}
	assert.ErrorIs(t, f.proxies.UndeployEverywhere(context.Background(), "no-such-proxy", f.sync.ouID),
		utils.ErrLLMProxyNotFound)
}

// The placement pass places an agent's proxy from its record alone, on its environment's egress gateway.
func TestPlacementPassPlacesAnAgentConfigProxy(t *testing.T) {
	f := newProxyFixture(t)
	proxy := f.newProxy(t)
	env := uuid.New()
	f.mapToAgent(t, proxy, env)
	gateway := f.mapGateway(t, env)

	require.NoError(t, f.worker.PlaceEnv(context.Background(), f.sync.ouID, env))

	row := f.row(t, proxy, gateway)
	require.NotNil(t, row)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
}

// Deleting the deployment an undeployed agent-config proxy's row points at keeps the row, detached from it, so the
// placement pass doesn't re-place the proxy the user undeployed.
func TestDeletingAnUndeployedAgentConfigProxyDeploymentKeepsItUndeployed(t *testing.T) {
	f := newProxyFixture(t)
	proxy := f.newProxy(t)
	env := uuid.New()
	gateway := f.mapGateway(t, env)
	f.mapToAgent(t, proxy, env)
	ctx := context.Background()
	deployed, err := f.proxies.DeployForEnvironment(ctx, proxy, f.sync.ouID, env, gateway)
	require.NoError(t, err)
	_, err = f.proxies.UndeployLLMProxyDeployment(ctx, proxy.Handle, deployed.DeploymentID.String(), gateway.String(),
		f.sync.ouID)
	require.NoError(t, err)
	applied, err := f.sync.repo.ApplyAck(ctx, gateway.String(), models.DeploymentAck{
		ArtifactID: proxy.Handle, Action: models.DeploymentAckActionUndeploy, Status: models.DeploymentAckStatusSuccess,
	}, nil)
	require.NoError(t, err)
	require.True(t, applied)

	require.NoError(t, f.proxies.DeleteLLMProxyDeployment(proxy.Handle, deployed.DeploymentID.String(), f.sync.ouID))
	published := len(f.hub.published)
	require.NoError(t, f.worker.PlaceAll(ctx))

	row := f.row(t, proxy, gateway)
	require.NotNil(t, row, "the undeployed row is kept")
	assert.Equal(t, models.DeploymentStatusUndeployed, row.Status)
	assert.Equal(t, models.DeploymentStatusUndeployed, row.StatusDesired)
	assert.Nil(t, row.DeploymentID)
	assert.Nil(t, row.NextAttemptAt)
	assert.Len(t, f.hub.published, published, "nothing is re-placed")
}

// An agent-config save after the proxy was undeployed from its environment's gateway through the public route has
// nothing placed to refresh there: it saves.
func TestAgentConfigProxyUpdateAfterAPublicUndeploySaves(t *testing.T) {
	f := newProxyFixture(t)
	proxy := f.newProxy(t)
	env := uuid.New()
	gateway := f.mapGateway(t, env)
	f.mapToAgent(t, proxy, env)
	deployed, err := f.proxies.DeployForEnvironment(context.Background(), proxy, f.sync.ouID, env, gateway)
	require.NoError(t, err)
	_, err = f.proxies.UndeployLLMProxyDeployment(context.Background(), proxy.Handle,
		deployed.DeploymentID.String(), gateway.String(), f.sync.ouID)
	require.NoError(t, err)

	gormDB := db.GetDB()
	mappings := repositories.NewEnvAgentModelMappingRepository(gormDB)
	var mapping models.EnvAgentModelMapping
	require.NoError(t, gormDB.Preload("LLMProxy").Where("llm_proxy_uuid = ?", proxy.UUID).First(&mapping).Error)
	mapping.LLMProxy.Handle = proxy.Handle
	svc := &agentConfigurationService{
		db:                        gormDB,
		envMappingRepo:            mappings,
		llmProviderRepo:           repositories.NewLLMProviderRepo(gormDB),
		gatewayRepo:               repositories.NewGatewayRepo(gormDB),
		llmProxyService:           NewLLMProxyService(repositories.NewLLMProxyRepo(gormDB), repositories.NewLLMProviderRepo(gormDB), make([]byte, 32)),
		llmProxyDeploymentService: f.proxies,
		logger:                    slog.New(slog.DiscardHandler),
	}
	config := &models.AgentConfiguration{OUID: f.sync.ouID, AgentID: "a"}
	request := models.EnvModelConfigRequest{ProviderName: f.provider.Configuration.Handle}

	rollback, err := svc.processEnvProxyUpdate(context.Background(), config, nil, env, "dev", request, &mapping, f.sync.ouID)

	require.NoError(t, err)
	assert.Equal(t, uuid.Nil, rollback.deploymentID)
	assert.Equal(t, models.DeploymentStatusUndeployed, f.row(t, proxy, gateway).StatusDesired, "nothing is re-placed")
}

// A public deploy is refused on a second gateway of an environment the artifact is already placed in, before the
// first placement's ack.
func TestPublicDeployRefusesASecondGatewayOfAPlacedEnvironment(t *testing.T) {
	f := newProxyFixture(t)
	env := uuid.New()
	first, second := f.mapGateway(t, env), f.mapGateway(t, env)

	t.Run("LLM proxy", func(t *testing.T) {
		proxy := f.newProxy(t)
		_, err := f.proxies.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{GatewayID: first.String()}, f.sync.ouID)
		require.NoError(t, err)
		require.Equal(t, models.DeploymentStatusDeploying, f.row(t, proxy, first).Status)

		_, err = f.proxies.DeployLLMProxy(proxy.Handle, &models.DeployAPIRequest{GatewayID: second.String()}, f.sync.ouID)

		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})
	t.Run("LLM provider", func(t *testing.T) {
		_, err := f.deployment.DeployLLMProvider(f.provider.UUID.String(),
			&models.DeployAPIRequest{GatewayID: first.String()}, f.sync.ouID)
		require.NoError(t, err)

		_, err = f.deployment.DeployLLMProvider(f.provider.UUID.String(),
			&models.DeployAPIRequest{GatewayID: second.String()}, f.sync.ouID)

		assert.ErrorIs(t, err, utils.ErrInvalidInput)
	})
}

// A proxy's deploy event reads the vhost in the deploying transaction, so a vhost saved in it reaches the event.
func TestLLMProxyEventReadsTheVhostInTheDeployingTransaction(t *testing.T) {
	f := newProxyFixture(t)
	proxy := f.newProxy(t)
	env := uuid.New()
	gateway := f.mapGateway(t, env)
	vhost := "saved-" + uuid.NewString()[:8] + ".example.com"

	err := NewTxRunner(db.GetDB())(context.Background(), func(tx *gorm.DB) error {
		proxy.Configuration.Vhost = &vhost
		if err := repositories.NewLLMProxyRepo(tx).Update(proxy, proxy.Handle, f.sync.ouID); err != nil {
			return err
		}
		_, err := f.proxies.core.Deploy(context.Background(), tx, proxyPlacement(proxy, proxy.Handle, f.sync.ouID, gateway))
		return err
	})

	require.NoError(t, err)
	require.NotEmpty(t, f.hub.published)
	assert.Contains(t, f.hub.published[len(f.hub.published)-1].EventData, vhost)
}

// scenarioBRollback deploys a proxy, then changes its description and redeploys, as a Scenario-B update does. It
// returns the service to roll back with, the proxy as it was before the change, and the old and new deployments.
func scenarioBRollback(t *testing.T, f *proxyFixture) (
	svc *agentConfigurationService, proxy, prior *models.LLMProxy, gateway uuid.UUID, old, next *models.Deployment,
) {
	t.Helper()
	ctx := context.Background()
	proxy = f.newProxy(t)
	env := uuid.New()
	gateway = f.mapGateway(t, env)
	f.mapToAgent(t, proxy, env)
	old, err := f.proxies.DeployForEnvironment(ctx, proxy, f.sync.ouID, env, gateway)
	require.NoError(t, err)

	gormDB := db.GetDB()
	proxies := repositories.NewLLMProxyRepo(gormDB)
	prior, err = proxies.GetByID(proxy.Handle, f.sync.ouID)
	require.NoError(t, err)
	prior.Handle = proxy.Handle

	svc = &agentConfigurationService{
		db:                        gormDB,
		llmProxyService:           NewLLMProxyService(proxies, repositories.NewLLMProviderRepo(gormDB), make([]byte, 32)),
		llmProxyDeploymentService: f.proxies,
		logger:                    slog.New(slog.DiscardHandler),
	}
	changed, err := proxies.GetByID(proxy.Handle, f.sync.ouID)
	require.NoError(t, err)
	changed.Description = "changed-" + uuid.NewString()
	updated, err := svc.llmProxyService.Update(proxy.Handle, f.sync.ouID, changed)
	require.NoError(t, err)
	updated.Handle = proxy.Handle
	next, err = f.proxies.Redeploy(ctx, updated, f.sync.ouID, gateway, "scenario B update")
	require.NoError(t, err)
	require.NotEqual(t, old.DeploymentID, next.DeploymentID)
	return svc, proxy, prior, gateway, old, next
}

// deploymentExists reads the deployments table directly: GetLLMProxyDeployment answers an empty deployment, not
// ErrDeploymentNotFound, for an ID that is gone.
func deploymentExists(t *testing.T, id uuid.UUID) bool {
	t.Helper()
	var count int64
	require.NoError(t, db.GetDB().Model(&models.Deployment{}).Where("deployment_id = ?", id).Count(&count).Error)
	return count == 1
}

func proxyEventsSince(f *proxyFixture, before int, proxy *models.LLMProxy) []eventhub.Event {
	var events []eventhub.Event
	for _, evt := range f.hub.published[before:] {
		if evt.EntityID == proxy.Handle {
			events = append(events, evt)
		}
	}
	return events
}

// A failed Scenario-B update points the gateway back at the old deployment and drops the new one, without taking the
// proxy off the gateway.
func TestScenarioBRollbackRestoresTheOldDeploymentWithoutUndeploying(t *testing.T) {
	f := newProxyFixture(t)
	svc, proxy, prior, gateway, old, next := scenarioBRollback(t, f)
	before := len(f.hub.published)

	svc.rollbackProxies(context.Background(), []rollbackResource{{
		proxyHandle: proxy.Handle, gatewayID: gateway.String(), priorProxyConfig: prior,
		restoreDeploymentID: old.DeploymentID, supersededDeploymentID: next.DeploymentID,
	}}, f.sync.ouID)

	events := proxyEventsSince(f, before, proxy)
	require.Len(t, events, 1)
	assert.EqualValues(t, "llmproxy.deployed", events[0].EventType)
	assert.Contains(t, events[0].EventData, old.DeploymentID.String())
	row := f.row(t, proxy, gateway)
	assert.Equal(t, old.DeploymentID, *row.DeploymentID)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
	assert.Equal(t, models.DeploymentStatusDeployed, row.StatusDesired)
	assert.False(t, deploymentExists(t, next.DeploymentID))
	stored, err := repositories.NewLLMProxyRepo(db.GetDB()).GetByID(proxy.Handle, f.sync.ouID)
	require.NoError(t, err)
	assert.Equal(t, prior.Description, stored.Description)
}

// A rollback whose old and new deployment are the same one has nothing to restore or delete.
func TestScenarioBRollbackOfAnUnchangedDeploymentLeavesItLive(t *testing.T) {
	f := newProxyFixture(t)
	proxy := f.newProxy(t)
	env := uuid.New()
	gateway := f.mapGateway(t, env)
	f.mapToAgent(t, proxy, env)
	old, err := f.proxies.DeployForEnvironment(context.Background(), proxy, f.sync.ouID, env, gateway)
	require.NoError(t, err)
	proxies := repositories.NewLLMProxyRepo(db.GetDB())
	prior, err := proxies.GetByID(proxy.Handle, f.sync.ouID)
	require.NoError(t, err)
	prior.Handle = proxy.Handle
	svc := &agentConfigurationService{
		llmProxyService:           NewLLMProxyService(proxies, repositories.NewLLMProviderRepo(db.GetDB()), make([]byte, 32)),
		llmProxyDeploymentService: f.proxies,
		logger:                    slog.New(slog.DiscardHandler),
	}
	before := len(f.hub.published)

	svc.rollbackProxies(context.Background(), []rollbackResource{{
		proxyHandle: proxy.Handle, gatewayID: gateway.String(), priorProxyConfig: prior,
		restoreDeploymentID: old.DeploymentID, supersededDeploymentID: old.DeploymentID,
	}}, f.sync.ouID)

	assert.Empty(t, proxyEventsSince(f, before, proxy))
	row := f.row(t, proxy, gateway)
	assert.Equal(t, old.DeploymentID, *row.DeploymentID)
	assert.Equal(t, models.DeploymentStatusDeployed, row.StatusDesired)
	assert.True(t, deploymentExists(t, old.DeploymentID))
}

// When the old deployment cannot be restored, the new one stays: deleting it would leave the proxy with nothing.
func TestScenarioBRollbackWhoseRestoreFailsKeepsTheNewDeployment(t *testing.T) {
	f := newProxyFixture(t)
	svc, proxy, prior, gateway, _, next := scenarioBRollback(t, f)
	before := len(f.hub.published)

	svc.rollbackProxies(context.Background(), []rollbackResource{{
		proxyHandle: proxy.Handle, gatewayID: gateway.String(), priorProxyConfig: prior,
		restoreDeploymentID: uuid.New(), supersededDeploymentID: next.DeploymentID,
	}}, f.sync.ouID)

	assert.Empty(t, proxyEventsSince(f, before, proxy))
	row := f.row(t, proxy, gateway)
	assert.Equal(t, next.DeploymentID, *row.DeploymentID)
	assert.Equal(t, models.DeploymentStatusDeployed, row.StatusDesired)
	assert.True(t, deploymentExists(t, next.DeploymentID))
}

// A restore that finds the old deployment already current counts as done, so the new deployment is still deleted.
func TestScenarioBRollbackAfterTheOldDeploymentIsBackStillDeletesTheNewOne(t *testing.T) {
	f := newProxyFixture(t)
	svc, proxy, prior, gateway, old, next := scenarioBRollback(t, f)
	_, err := f.proxies.RestoreLLMProxyDeployment(proxy.Handle, old.DeploymentID.String(), gateway.String(), f.sync.ouID)
	require.NoError(t, err)
	before := len(f.hub.published)

	svc.rollbackProxies(context.Background(), []rollbackResource{{
		proxyHandle: proxy.Handle, gatewayID: gateway.String(), priorProxyConfig: prior,
		restoreDeploymentID: old.DeploymentID, supersededDeploymentID: next.DeploymentID,
	}}, f.sync.ouID)

	assert.Empty(t, proxyEventsSince(f, before, proxy))
	row := f.row(t, proxy, gateway)
	assert.Equal(t, old.DeploymentID, *row.DeploymentID)
	assert.Equal(t, models.DeploymentStatusDeployed, row.StatusDesired)
	assert.False(t, deploymentExists(t, next.DeploymentID))
}

// When the policy write of a Scenario-B update fails, processEnvProxyUpdate hands back the new deployment as
// superseded, so the rollback restores the old one over it instead of undeploying the proxy.
func TestScenarioBPolicyWriteFailureRollsBackWithoutUndeploying(t *testing.T) {
	f := newProxyFixture(t)
	proxy := f.newProxy(t)
	env := uuid.New()
	gateway := f.mapGateway(t, env)
	f.mapToAgent(t, proxy, env)
	old, err := f.proxies.DeployForEnvironment(context.Background(), proxy, f.sync.ouID, env, gateway)
	require.NoError(t, err)

	gormDB := db.GetDB()
	mappings := repositories.NewEnvAgentModelMappingRepository(gormDB)
	var mapping models.EnvAgentModelMapping
	require.NoError(t, gormDB.Preload("LLMProxy").Where("llm_proxy_uuid = ?", proxy.UUID).First(&mapping).Error)
	mapping.LLMProxy.Handle = proxy.Handle
	refusePolicyWrites(t, mapping.ID)
	svc := &agentConfigurationService{
		db:                        gormDB,
		envMappingRepo:            mappings,
		llmProviderRepo:           repositories.NewLLMProviderRepo(gormDB),
		gatewayRepo:               repositories.NewGatewayRepo(gormDB),
		llmProxyService:           NewLLMProxyService(repositories.NewLLMProxyRepo(gormDB), repositories.NewLLMProviderRepo(gormDB), make([]byte, 32)),
		llmProxyDeploymentService: f.proxies,
		logger:                    slog.New(slog.DiscardHandler),
	}
	config := &models.AgentConfiguration{OUID: f.sync.ouID, AgentID: "a"}
	request := models.EnvModelConfigRequest{ProviderName: f.provider.Configuration.Handle}

	rollback, err := svc.processEnvProxyUpdate(context.Background(), config, nil, env, "dev", request, &mapping, f.sync.ouID)

	require.ErrorContains(t, err, "failed to update policy configuration")
	assert.Equal(t, uuid.Nil, rollback.deploymentID)
	assert.Equal(t, old.DeploymentID, rollback.restoreDeploymentID)
	assert.NotEqual(t, uuid.Nil, rollback.supersededDeploymentID)
	assert.NotEqual(t, old.DeploymentID, rollback.supersededDeploymentID)

	before := len(f.hub.published)
	svc.rollbackProxies(context.Background(), []rollbackResource{rollback}, f.sync.ouID)

	events := proxyEventsSince(f, before, proxy)
	require.Len(t, events, 1)
	assert.EqualValues(t, "llmproxy.deployed", events[0].EventType)
	assert.Contains(t, events[0].EventData, old.DeploymentID.String())
	row := f.row(t, proxy, gateway)
	assert.Equal(t, old.DeploymentID, *row.DeploymentID)
	assert.Equal(t, models.DeploymentStatusDeployed, row.StatusDesired)
	assert.False(t, deploymentExists(t, rollback.supersededDeploymentID))
}

// refusePolicyWrites makes every update of one env_agent_model_mapping row fail, until the test ends.
func refusePolicyWrites(t *testing.T, mappingID uint) {
	t.Helper()
	name := "refuse_policy_write_" + uuid.NewString()[:8]
	gormDB := db.GetDB()
	require.NoError(t, gormDB.Exec(fmt.Sprintf(
		`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'policy write refused'; END $$`,
		name)).Error)
	t.Cleanup(func() { assert.NoError(t, gormDB.Exec("DROP FUNCTION IF EXISTS "+name+"()").Error) })
	require.NoError(t, gormDB.Exec(fmt.Sprintf(
		`CREATE TRIGGER %s BEFORE UPDATE ON env_agent_model_mapping FOR EACH ROW WHEN (OLD.id = %d) EXECUTE FUNCTION %s()`,
		name, mappingID, name)).Error)
	t.Cleanup(func() {
		assert.NoError(t, gormDB.Exec("DROP TRIGGER IF EXISTS "+name+" ON env_agent_model_mapping").Error)
	})
}
