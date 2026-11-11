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
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/reconcile"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// a2aFixture is an org with an ingress gateway mapped to env, and the A2A kind over the real repositories with a
// platform repository that reports the external agent.
type a2aFixture struct {
	sync      *syncFixture
	env       uuid.UUID
	gateway   uuid.UUID
	hub       *recordingEventHub
	events    *GatewayEventsService
	core      *reconcile.Core
	worker    *reconcile.Worker
	publisher *A2APublisher
	restKind  *RestAgentKind
	// proxies serves external agents; external is the agent the platform reports, with no subtype (#2184).
	proxies  A2AProxyService
	external *models.AgentResponse
}

func newA2AFixture(t *testing.T) *a2aFixture {
	t.Helper()
	f := &a2aFixture{sync: newSyncFixture(t), env: uuid.New(), hub: &recordingEventHub{}}
	f.external = &models.AgentResponse{
		UUID: uuid.NewString(), Name: "ext-" + uuid.NewString()[:8], ProjectName: "proj",
		Provisioning: models.Provisioning{Type: "external"}, Type: models.AgentType{Type: "external-agent-api"},
	}
	gormDB := db.GetDB()
	f.gateway = f.sync.seedGateway(t, f.sync.ouID)
	require.NoError(t, gormDB.Exec(`UPDATE gateways SET gateway_functionality_type = ? WHERE uuid = ?`,
		models.GatewayRoleIngress, f.gateway).Error)
	require.NoError(t, gormDB.Exec(`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`,
		f.gateway, f.env).Error)
	oc := &repomocks.PlatformRepositoryMock{
		GetAgentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return f.external, nil
		},
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{Name: name, UUID: f.env.String()}, nil
		},
	}
	fetcher := &fakeA2ACardFetcher{
		FetchOriginFunc: func(context.Context, string, *A2ACardFetchHeader) (json.RawMessage, error) {
			return a2aTestCard(t, a2aIface("JSONRPC", "https://agent.example/team/a2a/jsonrpc")), nil
		},
	}
	configs := &repomocks.AgentConfigRepositoryMock{
		GetFunc: func(context.Context, string, string, string, string) (*models.AgentConfig, error) {
			return &models.AgentConfig{}, nil
		},
	}
	f.events = NewGatewayEventsService(f.hub)
	apiKeys := repositories.NewAPIKeyRepo(gormDB)
	tx := NewTxRunner(gormDB)
	exposures := repositories.NewPlatformAgentExposureRepository(gormDB)
	proxies := repositories.NewA2AProxyRepository(gormDB)
	cards := repositories.NewA2AAgentCardRepository(gormDB)
	gateways := repositories.NewGatewayRepo(gormDB)
	kind := NewA2AAgentKind(tx, NewPlatformAgentExposures(exposures, gateways, oc), proxies, cards, f.sync.repo, configs, oc, fetcher, apiKeys, f.events, nil, testLogger())
	f.restKind = NewRestAgentKind(NewPlatformAgentExposures(exposures, gateways, oc), configs, f.sync.repo, apiKeys, f.events, testLogger())
	f.core = reconcile.NewCore(gormDB, f.sync.repo, f.hub, reconcile.Kinds{kind, f.restKind},
		reconcile.Config{Lease: time.Minute, AttemptBudget: 3}, slog.New(slog.DiscardHandler))
	f.worker = reconcile.NewWorker(f.core, slog.New(slog.DiscardHandler))
	artifacts := repositories.NewArtifactRepo(gormDB)
	f.publisher = NewA2APublisher(tx, exposures, proxies, cards, artifacts, f.sync.repo, testLogger(),
		NewAgentExposurePublisher(tx, exposures, artifacts, f.sync.repo, gateways, f.core, testLogger()), kind)
	f.proxies = NewA2AProxyService(tx, proxies, cards, repositories.NewAgentConfigRepo(gormDB),
		repositories.NewArtifactRepo(gormDB), f.publisher, oc, make([]byte, 32), testLogger())
	t.Cleanup(func() {
		for _, table := range []string{"a2a_proxies", "a2a_agent_cards", "agent_configs"} {
			gormDB.Exec(`DELETE FROM `+table+` WHERE ou_id = ?`, f.sync.ouID)
		}
		gormDB.Exec(`DELETE FROM artifacts WHERE ou_id = ? AND kind IN ?`, f.sync.ouID, models.AgentArtifactKinds)
	})
	return f
}

// env is the fixture's environment as OpenChoreo reports it.
func (f *a2aFixture) envResponse() *models.EnvironmentResponse {
	return &models.EnvironmentResponse{Name: "dev", UUID: f.env.String()}
}

// registerCard registers the external agent's origin card URL while its environment is direct.
func (f *a2aFixture) registerCard(t *testing.T) {
	t.Helper()
	require.NoError(t, f.proxies.StoreCardSource(context.Background(), f.external, f.sync.ouID, "proj", f.envResponse(),
		originCardURL))
}

// proxy proxies the external agent's environment and returns its artifact.
func (f *a2aFixture) proxy(t *testing.T) models.ArtifactRef {
	t.Helper()
	require.NoError(t, f.proxies.SetA2AProxy(tierGrantedCtx(t), f.sync.ouID, "proj", f.external.Name, "dev", nil))
	return f.externalArtifact(t)
}

func (f *a2aFixture) externalArtifact(t *testing.T) models.ArtifactRef {
	t.Helper()
	artifact, err := agentEnvAPIArtifact(repositories.NewArtifactRepo(db.GetDB()), f.sync.ouID, "proj", f.external.Name,
		f.env.String(), models.KindAgent)
	require.NoError(t, err)
	return agentArtifactRef(artifact)
}

func (f *a2aFixture) render(t *testing.T, artifact models.ArtifactRef) {
	t.Helper()
	require.NoError(t, f.worker.RunOnceFor(context.Background(), artifact.OUID, artifact.ArtifactUUID))
}

func (f *a2aFixture) ack(t *testing.T, artifact models.ArtifactRef, action string) {
	t.Helper()
	row := f.row(t, artifact)
	ack := models.DeploymentAck{
		ArtifactID: artifact.ArtifactUUID.String(), Action: action, Status: "success",
		PerformedAt: row.PerformedAt,
	}
	if action == "deploy" {
		ack.DeploymentID = row.DeploymentID.String()
	}
	applied, err := f.sync.repo.ApplyAck(context.Background(), f.gateway.String(), ack, f.core.AfterLive())
	require.NoError(t, err)
	require.True(t, applied)
}

// seedAPIKey stores a key of the artifact; a nil expiry never expires.
func (f *a2aFixture) seedAPIKey(t *testing.T, artifact models.ArtifactRef, name, status string, expiresAt *time.Time) models.StoredAPIKey {
	t.Helper()
	stored := models.StoredAPIKey{
		UUID: uuid.New(), Name: name, DisplayName: name, ArtifactUUID: artifact.ArtifactUUID, OUID: f.sync.ouID,
		APIKeyHash: "hash-" + name, MaskedAPIKey: "****" + name, Status: status,
		CreatedAt: time.Now(), UpdatedAt: time.Now(), ExpiresAt: expiresAt,
	}
	require.NoError(t, repositories.NewAPIKeyRepo(db.GetDB()).Upsert(&stored))
	t.Cleanup(func() { db.GetDB().Exec(`DELETE FROM api_keys WHERE uuid = ?`, stored.UUID) })
	return stored
}

// replayedKeyNames are the keys the hub was told to create, by name.
func (f *a2aFixture) replayedKeyNames(t *testing.T) []string {
	t.Helper()
	var names []string
	for _, evt := range f.hub.published {
		if string(evt.EventType) == "apikey.created" {
			names = append(names, createdPayload(t, evt).Name)
		}
	}
	return names
}

// platformAgent is a platform agent's per-environment artifact and its publication key.
func (f *a2aFixture) platformAgent(t *testing.T) (models.ArtifactRef, agentExposureKey) {
	t.Helper()
	name := "agent-" + uuid.NewString()[:8]
	artifact, err := ensureAgentEnvAPIArtifact(db.GetDB(), repositories.NewArtifactRepo(db.GetDB()), f.sync.ouID, "proj",
		name, f.env.String(), models.KindAgent)
	require.NoError(t, err)
	t.Cleanup(func() { db.GetDB().Exec(`DELETE FROM artifacts WHERE uuid = ?`, artifact.UUID) })
	agentUUID := uuid.New()
	return agentArtifactRef(artifact), agentExposureKey{
		OUID: f.sync.ouID, ProjectName: "proj", AgentName: name, EnvironmentName: "dev", EnvironmentUUID: f.env,
		AgentUUID: &agentUUID,
	}
}

func (f *a2aFixture) row(t *testing.T, artifact models.ArtifactRef) *models.DeploymentStatusRecord {
	t.Helper()
	row, err := f.sync.repo.GetStatusRecord(context.Background(), models.Placement{ArtifactRef: artifact, GatewayUUID: f.gateway})
	require.NoError(t, err)
	return row
}

// An A2A agent artifact with no rows, switched to direct under an older release that deleted them, still gets its
// delete event on the org's gateways.
func TestAgentDeleteReachesTheOrgsGatewaysForAnArtifactWithNoRows(t *testing.T) {
	f := newA2AFixture(t)
	artifact, key := f.platformAgent(t)
	svc := &agentManagerService{
		db: db.GetDB(), artifactRepo: repositories.NewArtifactRepo(db.GetDB()),
		a2aCardRepo: repositories.NewA2AAgentCardRepository(db.GetDB()), a2aPublisher: f.publisher, logger: testLogger(),
	}

	require.NoError(t, svc.deleteAgentAPIArtifact(context.Background(), key.OUID, key.ProjectName, key.AgentName, true))

	var addressed []string
	for _, evt := range f.hub.published {
		assert.Equal(t, "agent.deleted", string(evt.EventType))
		assert.Equal(t, artifact.ArtifactUUID.String(), evt.EntityID)
		addressed = append(addressed, evt.GatewayID)
	}
	assert.ElementsMatch(t, []string{f.gateway.String(), f.sync.gateway.String()}, addressed)
}

// Set proxy, switch to direct, set proxy again: the undeployed row is owed a deploy again and the worker deploys it.
func TestExternalAgentProxiedAgainDeploysItsUndeployedRow(t *testing.T) {
	f := newA2AFixture(t)
	f.registerCard(t)
	artifact := f.proxy(t)
	f.render(t, artifact)
	first := f.row(t, artifact)
	require.Equal(t, models.DeploymentStatusDeploying, first.Status)
	f.ack(t, artifact, "deploy")

	require.NoError(t, f.proxies.DeleteA2AProxy(tierGrantedCtx(t), f.sync.ouID, "proj", f.external.Name, "dev"))
	require.Equal(t, models.DeploymentStatusUndeploying, f.row(t, artifact).Status)
	f.ack(t, artifact, "undeploy")
	require.Equal(t, models.DeploymentStatusUndeployed, f.row(t, artifact).Status)

	f.proxy(t)
	owed := f.row(t, artifact)
	assert.Equal(t, models.DeploymentStatusUndeployed, owed.Status, "set proxy owes the deploy; the worker does it")
	require.NotNil(t, owed.NextAttemptAt, "the undeployed row is re-queued")

	f.render(t, artifact)

	row := f.row(t, artifact)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
	assert.Equal(t, models.DeploymentStatusDeployed, row.StatusDesired)
	assert.NotEqual(t, *first.DeploymentID, *row.DeploymentID)
	last := f.hub.published[len(f.hub.published)-1]
	assert.Equal(t, "agent.deployed", string(last.EventType))
}

// Removing the card URL of a proxied environment undeploys the agent and deletes its proxy in one transaction: when
// the undeploy can't be written, the proxy and card stay.
func TestRemovingAProxiedEnvironmentsCardURLUndeploysAndDeletesTheProxyTogether(t *testing.T) {
	f := newA2AFixture(t)
	f.registerCard(t)
	artifact := f.proxy(t)
	f.render(t, artifact)
	proxies := repositories.NewA2AProxyRepository(db.GetDB())
	cards := repositories.NewA2AAgentCardRepository(db.GetDB())
	f.hub.fail = func(evt eventhub.Event) error {
		if evt.EventType == "agent.undeployed" {
			return errors.New("event hub is down")
		}
		return nil
	}

	err := f.proxies.RemoveCardSource(tierGrantedCtx(t), f.external, f.sync.ouID, "proj", f.envResponse())

	require.ErrorContains(t, err, "event hub is down")
	_, err = proxies.GetByEnvUUID(context.Background(), f.sync.ouID, "proj", f.external.Name, f.env)
	assert.NoError(t, err, "the proxy stays")
	_, err = cards.Get(context.Background(), f.sync.ouID, "proj", f.external.Name, "dev")
	assert.NoError(t, err, "the card stays")
	assert.Equal(t, models.DeploymentStatusDeployed, f.row(t, artifact).StatusDesired)

	f.hub.fail = nil
	require.NoError(t, f.proxies.RemoveCardSource(tierGrantedCtx(t), f.external, f.sync.ouID, "proj", f.envResponse()))

	_, err = proxies.GetByEnvUUID(context.Background(), f.sync.ouID, "proj", f.external.Name, f.env)
	assert.ErrorIs(t, err, repositories.ErrA2AProxyNotFound)
	row := f.row(t, artifact)
	assert.Equal(t, models.DeploymentStatusUndeploying, row.Status)
	last := f.hub.published[len(f.hub.published)-1]
	assert.Equal(t, "agent.undeployed", string(last.EventType))
}

// The placement pass places an external agent from its proxy row alone.
func TestPlacementPassPlacesAnExternalAgentFromItsProxyAlone(t *testing.T) {
	f := newA2AFixture(t)
	f.registerCard(t)
	require.NoError(t, repositories.NewA2AProxyRepository(db.GetDB()).Upsert(context.Background(), &models.A2AProxy{
		OUID: f.sync.ouID, ProjectName: "proj", AgentName: f.external.Name, EnvironmentName: "dev", EnvironmentUUID: f.env,
	}))
	_, err := ensureAgentEnvAPIArtifact(db.GetDB(), repositories.NewArtifactRepo(db.GetDB()), f.sync.ouID, "proj",
		f.external.Name, f.env.String(), models.KindAgent)
	require.NoError(t, err)
	artifact := f.externalArtifact(t)
	require.Nil(t, f.row(t, artifact))

	require.NoError(t, f.worker.PlaceEnv(context.Background(), f.sync.ouID, f.env))

	row := f.row(t, artifact)
	require.NotNil(t, row)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
}

// #2184: an external agent created with no subtype is placed and deployed through its proxy.
func TestAnExternalAgentWithNoSubtypeIsPlacedThroughItsProxy(t *testing.T) {
	f := newA2AFixture(t)
	require.Empty(t, f.external.Type.SubType)
	f.registerCard(t)

	artifact := f.proxy(t)
	f.render(t, artifact)

	row := f.row(t, artifact)
	require.NotNil(t, row)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
	pulled, err := f.sync.repo.GetPullable(context.Background(), f.sync.ouID, artifact.ArtifactUUID, f.gateway.String())
	require.NoError(t, err)
	assert.Contains(t, string(pulled.Content), "url: https://agent.example/team", "fronts the card's origin")
}

// GET a2a-proxy reports the environment's deployment: DEPLOYING while the deploy is owed and until the gateway acks
// it, DEPLOYED after.
func TestGetA2AProxyFollowsTheDeployment(t *testing.T) {
	f := newA2AFixture(t)
	f.registerCard(t)
	status := func() *models.A2AProxyStatus {
		t.Helper()
		got, err := f.proxies.GetA2AProxy(tierOnlyCtx(), f.sync.ouID, "proj", f.external.Name, "dev")
		require.NoError(t, err)
		return got
	}

	artifact := f.proxy(t)
	assert.Equal(t, models.DeploymentStatusDeploying, status().Status, "the deploy is owed to the worker")

	f.render(t, artifact)
	assert.Equal(t, models.DeploymentStatusDeploying, status().Status, "sent, not acked")

	f.ack(t, artifact, "deploy")
	got := status()
	assert.Equal(t, models.DeploymentStatusDeployed, got.Status)
	assert.Empty(t, got.StatusReason)
	assert.NotEmpty(t, got.GatewayURL)
}
