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

// restAgent is a REST agent's per-environment RestApi artifact and its exposure key.
func (f *a2aFixture) restAgent(t *testing.T) (models.ArtifactRef, agentExposureKey) {
	t.Helper()
	name := "rest-" + uuid.NewString()[:8]
	artifact, err := ensureAgentEnvAPIArtifact(db.GetDB(), repositories.NewArtifactRepo(db.GetDB()), f.sync.ouID, "proj",
		name, f.env.String(), models.KindRestAPI)
	require.NoError(t, err)
	t.Cleanup(func() { db.GetDB().Exec(`DELETE FROM artifacts WHERE uuid = ?`, artifact.UUID) })
	agentUUID := uuid.New()
	return agentArtifactRef(artifact), agentExposureKey{
		OUID: f.sync.ouID, ProjectName: "proj", AgentName: name, EnvironmentName: "dev", EnvironmentUUID: f.env,
		AgentUUID: &agentUUID,
	}
}

func (f *a2aFixture) publishREST(t *testing.T, key agentExposureKey, artifact models.ArtifactRef) {
	t.Helper()
	require.NoError(t, f.publisher.tx(context.Background(), func(tx *gorm.DB) error {
		return f.publisher.exposure.Publish(context.Background(), tx, f.restKind, key, artifact, models.AgentExposureProtocolREST)
	}))
	require.NoError(t, f.worker.RunOnceFor(context.Background(), artifact.OUID, artifact.ArtifactUUID))
}

// A REST agent deploys once its endpoint appears: PENDING while it waits, DEPLOYING with an api.deployed event once
// the endpoint is there, DEPLOYED on the ack.
func TestRestAgentLifecycle(t *testing.T) {
	f := newA2AFixture(t)
	artifact, key := f.restAgent(t)

	f.publishREST(t, key, artifact)
	row := f.row(t, artifact)
	require.NotNil(t, row)
	assert.Equal(t, models.DeploymentStatusPending, row.Status)
	assert.Equal(t, waitingForEndpoint, models.UserStatusOf(*row).Reason)

	f.url.Store("http://agent.dev.svc:8080/api")
	f.publishREST(t, key, artifact)
	row = f.row(t, artifact)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
	require.Len(t, f.hub.published, 1)
	assert.Equal(t, "api.deployed", string(f.hub.published[0].EventType))
	assert.Equal(t, artifact.ArtifactUUID.String(), f.hub.published[0].EntityID)

	f.seedAPIKey(t, artifact, "kept", "active", nil)
	deleted := f.seedAPIKey(t, artifact, "deleted", "active", nil)
	require.NoError(t, repositories.NewAPIKeyRepo(db.GetDB()).Delete(artifact.ArtifactUUID.String(), deleted.Name))

	applied, err := f.sync.repo.ApplyAck(context.Background(), f.gateway.String(), models.DeploymentAck{
		DeploymentID: row.DeploymentID.String(), ArtifactID: artifact.ArtifactUUID.String(), Action: "deploy",
		Status: "success", PerformedAt: row.PerformedAt,
	}, f.core.AfterLive())
	require.NoError(t, err)
	require.True(t, applied)
	assert.Equal(t, models.DeploymentStatusDeployed, f.row(t, artifact).Status)
	assert.Equal(t, []string{"kept"}, f.replayedKeyNames(t))
	assert.Equal(t, f.gateway.String(), f.hub.published[len(f.hub.published)-1].GatewayID)
}

// Only keys a gateway would still accept are replayed: not revoked, not expired.
func TestRestAgentKeyReplaySkipsInactiveAndExpiredKeys(t *testing.T) {
	f := newA2AFixture(t)
	artifact, key := f.restAgent(t)
	f.url.Store("http://agent.dev.svc:8080")
	f.publishREST(t, key, artifact)
	yesterday, tomorrow := time.Now().Add(-24*time.Hour), time.Now().Add(24*time.Hour)
	f.seedAPIKey(t, artifact, "live", "active", nil)
	f.seedAPIKey(t, artifact, "later", "active", &tomorrow)
	f.seedAPIKey(t, artifact, "expired", "active", &yesterday)
	f.seedAPIKey(t, artifact, "inactive", "revoked", nil)

	f.ack(t, artifact, "deploy")

	assert.ElementsMatch(t, []string{"live", "later"}, f.replayedKeyNames(t))
}

// An exposure with no ingress gateway has no row; mapping a gateway makes the placement pass create it.
func TestRestAgentPlacedAfterGatewayMapped(t *testing.T) {
	f := newA2AFixture(t)
	artifact, key := f.restAgent(t)
	require.NoError(t, db.GetDB().Exec(`DELETE FROM gateway_environment_mappings WHERE gateway_uuid = ?`, f.gateway).Error)
	require.NoError(t, f.publisher.exposure.Expose(context.Background(), nil, key, artifact.ArtifactUUID, models.AgentExposureProtocolREST))
	f.url.Store("http://agent.dev.svc:8080")

	require.NoError(t, f.worker.PlaceEnv(context.Background(), f.sync.ouID, f.env))
	assert.Nil(t, f.row(t, artifact))

	require.NoError(t, db.GetDB().Exec(`INSERT INTO gateway_environment_mappings (gateway_uuid, environment_uuid) VALUES (?, ?)`,
		f.gateway, f.env).Error)
	require.NoError(t, f.worker.PlaceEnv(context.Background(), f.sync.ouID, f.env))

	row := f.row(t, artifact)
	require.NotNil(t, row)
	assert.Equal(t, models.DeploymentStatusDeploying, row.Status)
}

func TestUnmapRefusedWhileRestAgentPlaced(t *testing.T) {
	f := newA2AFixture(t)
	artifact, key := f.restAgent(t)
	f.url.Store("http://agent.dev.svc:8080")
	f.publishREST(t, key, artifact)
	require.NotNil(t, f.row(t, artifact))

	serves, err := f.core.ServesEnvironment(context.Background(), db.GetDB(), f.env, f.gateway)

	require.NoError(t, err)
	assert.True(t, serves)
}

func TestGatewaySyncListsRestApi(t *testing.T) {
	f := newA2AFixture(t)
	artifact, key := f.restAgent(t)
	f.url.Store("http://agent.dev.svc:8080")
	f.publishREST(t, key, artifact)

	got, err := f.sync.svc.ListGatewayDeployments(context.Background(), f.sync.ouID, f.gateway.String())

	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, models.KindRestAPI, got[0].Kind)
	assert.Equal(t, artifact.ArtifactUUID.String(), got[0].ArtifactID)
}

// A RestApi sent to a gateway is deleted with api.deleted, never agent.deleted, on every gateway of the org, by agent
// delete and by environment delete. Agent delete writes nothing for one never sent.
func TestRestAgentDeleteEvents(t *testing.T) {
	assertDeletedEverywhere := func(t *testing.T, f *a2aFixture, artifact models.ArtifactRef) {
		t.Helper()
		var addressed []string
		for _, evt := range f.hub.published {
			assert.Equal(t, "api.deleted", string(evt.EventType))
			assert.Equal(t, artifact.ArtifactUUID.String(), evt.EntityID)
			addressed = append(addressed, evt.GatewayID)
		}
		assert.ElementsMatch(t, []string{f.gateway.String(), f.sync.gateway.String()}, addressed)
	}
	sentRestAgent := func(t *testing.T) (*a2aFixture, models.ArtifactRef, agentExposureKey) {
		f := newA2AFixture(t)
		artifact, key := f.restAgent(t)
		f.url.Store("http://agent.dev.svc:8080")
		f.publishREST(t, key, artifact)
		f.hub.published = nil
		return f, artifact, key
	}

	t.Run("agent delete", func(t *testing.T) {
		f, artifact, key := sentRestAgent(t)

		require.NoError(t, f.publisher.tx(context.Background(), func(tx *gorm.DB) error {
			return f.publisher.DropAgentArtifact(context.Background(), tx, key, artifact, false)
		}))

		assertDeletedEverywhere(t, f, artifact)
	})
	t.Run("environment delete", func(t *testing.T) {
		f, artifact, _ := sentRestAgent(t)

		require.NoError(t, f.publisher.TeardownEnvironment(context.Background(), f.sync.ouID, f.env))

		assertDeletedEverywhere(t, f, artifact)
	})
	t.Run("never sent", func(t *testing.T) {
		f := newA2AFixture(t)
		artifact, key := f.restAgent(t)
		require.NoError(t, f.publisher.exposure.Expose(context.Background(), nil, key, artifact.ArtifactUUID, models.AgentExposureProtocolREST))

		require.NoError(t, f.publisher.tx(context.Background(), func(tx *gorm.DB) error {
			return f.publisher.DropAgentArtifact(context.Background(), tx, key, artifact, false)
		}))

		assert.Empty(t, f.hub.published)
	})
}

// Environment delete, unlike agent delete, does not check which gateways held the artifact: Core.PrepareDelete
// addresses every active gateway of the org, and a delete is a no-op on a gateway that never held it.
func TestEnvironmentDeleteOfNeverSentRestAgentArtifactStillWritesDeleteEvents(t *testing.T) {
	f := newA2AFixture(t)
	artifact, key := f.restAgent(t)
	require.NoError(t, f.publisher.exposure.Expose(context.Background(), nil, key, artifact.ArtifactUUID, models.AgentExposureProtocolREST))

	require.NoError(t, f.publisher.TeardownEnvironment(context.Background(), f.sync.ouID, f.env))

	var addressed []string
	for _, evt := range f.hub.published {
		assert.Equal(t, "api.deleted", string(evt.EventType))
		assert.Equal(t, artifact.ArtifactUUID.String(), evt.EntityID)
		addressed = append(addressed, evt.GatewayID)
	}
	assert.ElementsMatch(t, []string{f.gateway.String(), f.sync.gateway.String()}, addressed)
	_, err := repositories.NewArtifactRepo(db.GetDB()).GetByUUID(artifact.ArtifactUUID.String(), key.OUID)
	assert.Error(t, err, "the artifact row is deleted")
}
