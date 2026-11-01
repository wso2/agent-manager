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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	gen "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/gen"
	"github.com/wso2/agent-manager/agent-manager-service/db"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// agentService is an agentManagerService over the fixture's database, publishers and kinds, for an internal API
// agent of subType named after key.
func (f *a2aFixture) agentService(key agentExposureKey, subType string) *agentManagerService {
	gdb := db.GetDB()
	agent := &models.AgentResponse{
		UUID: key.AgentUUID.String(), Name: key.AgentName, ProjectName: key.ProjectName,
		Provisioning: models.Provisioning{Type: string(utils.InternalAgent)},
		Type:         models.AgentType{Type: string(utils.AgentTypeAPI), SubType: subType},
	}
	oc := &clientmocks.OpenChoreoClientMock{
		GetOrganizationFunc: func(_ context.Context, name string) (*models.OrganizationResponse, error) {
			return &models.OrganizationResponse{Name: name}, nil
		},
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) { return agent, nil },
		GetEnvironmentFunc: func(_ context.Context, _, name string) (*models.EnvironmentResponse, error) {
			return &models.EnvironmentResponse{Name: name, UUID: f.env.String()}, nil
		},
		UpdateDeploymentStateFunc: func(context.Context, string, string, string, string, gen.ReleaseBindingSpecState) error {
			return nil
		},
	}
	return &agentManagerService{
		db: gdb, ocClient: oc, agentConfigRepo: repositories.NewAgentConfigRepo(gdb),
		artifactRepo: repositories.NewArtifactRepo(gdb), a2aCardRepo: repositories.NewA2AAgentCardRepository(gdb),
		a2aPublisher: f.publisher, exposure: f.publisher.exposure, restAgentKind: f.restKind, logger: testLogger(),
	}
}

func (f *a2aFixture) exposureOf(t *testing.T, artifact models.ArtifactRef) *models.PlatformAgentExposure {
	t.Helper()
	exposure, err := repositories.NewPlatformAgentExposureRepository(db.GetDB()).GetByArtifact(context.Background(),
		artifact.OUID, artifact.ArtifactUUID)
	if err != nil {
		require.ErrorIs(t, err, repositories.ErrPlatformAgentExposureNotFound)
		return nil
	}
	return exposure
}

func (f *a2aFixture) storedArtifact(t *testing.T, artifact models.ArtifactRef) *models.Artifact {
	t.Helper()
	stored, err := repositories.NewArtifactRepo(db.GetDB()).GetByUUID(artifact.ArtifactUUID.String(), artifact.OUID)
	require.NoError(t, err)
	return stored
}

// legacyRestAgent is a REST agent whose artifact predates RestApi: an Agent row, with one API key.
func (f *a2aFixture) legacyRestAgent(t *testing.T) (models.ArtifactRef, agentExposureKey) {
	t.Helper()
	artifact, key := f.platformAgent(t)
	f.seedAPIKey(t, artifact, "kept", "active", nil)
	return artifact, key
}

func TestCreateRestAgentExposes(t *testing.T) {
	t.Run("a chat-api agent waits for its endpoint on the gateway", func(t *testing.T) {
		f := newA2AFixture(t)
		artifact, key := f.restAgent(t)
		svc := f.agentService(key, string(utils.AgentSubTypeChatAPI))

		svc.enqueueCreatedAPIAgent(context.Background(), string(utils.AgentSubTypeChatAPI), key.OUID, key.ProjectName,
			key.AgentName, "dev", f.env.String(), f.storedArtifact(t, artifact))
		require.NoError(t, f.worker.RunOnceFor(context.Background(), artifact.OUID, artifact.ArtifactUUID))

		exposure := f.exposureOf(t, artifact)
		require.NotNil(t, exposure)
		assert.Equal(t, models.AgentExposureProtocolREST, exposure.Protocol)
		row := f.row(t, artifact)
		require.NotNil(t, row)
		assert.Equal(t, models.DeploymentStatusPending, row.Status)
		assert.Equal(t, models.KindRestAPI, f.storedArtifact(t, artifact).Kind)
		assert.Equal(t, waitingForEndpoint, models.UserStatusOf(*row).Reason)
	})
	t.Run("a non-API agent writes none", func(t *testing.T) {
		f := newA2AFixture(t)
		artifact, key := f.restAgent(t)
		svc := f.agentService(key, "")

		svc.enqueueCreatedAPIAgent(context.Background(), "", key.OUID, key.ProjectName, key.AgentName, "dev",
			f.env.String(), f.storedArtifact(t, artifact))

		assert.Nil(t, f.exposureOf(t, artifact))
		assert.Nil(t, f.row(t, artifact))
	})
}

func TestEnsureArtifactSwitchesLegacyKind(t *testing.T) {
	f := newA2AFixture(t)
	legacy, key := f.legacyRestAgent(t)
	gdb := db.GetDB()
	artifacts := repositories.NewArtifactRepo(gdb)

	var switched *models.Artifact
	require.NoError(t, gdb.Transaction(func(tx *gorm.DB) error {
		var err error
		switched, err = ensureExposedAgentEnvAPIArtifact(tx, artifacts, key.OUID, key.ProjectName, key.AgentName,
			f.env.String(), restAgentShape)
		return err
	}))

	assert.Equal(t, legacy.ArtifactUUID, switched.UUID)
	assert.Equal(t, models.KindRestAPI, switched.Kind)
	stored := f.storedArtifact(t, legacy)
	assert.Equal(t, models.KindRestAPI, stored.Kind)
	assert.Equal(t, legacy.Handle, stored.Handle)
	keys, err := repositories.NewAPIKeyRepo(gdb).ListPermanentByArtifactKinds(key.OUID, []string{models.KindRestAPI})
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, legacy.ArtifactUUID, keys[0].ArtifactUUID)

	t.Run("an A2A agent's Agent row stays", func(t *testing.T) {
		a2a, a2aKey := f.platformAgent(t)
		got, err := ensureExposedAgentEnvAPIArtifact(gdb, artifacts, a2aKey.OUID, a2aKey.ProjectName, a2aKey.AgentName,
			f.env.String(), a2aAgentShape)
		require.NoError(t, err)
		assert.Equal(t, a2a.ArtifactUUID, got.UUID)
		assert.Equal(t, models.KindAgent, f.storedArtifact(t, a2a).Kind)
	})
}

// A legacy REST agent's next deploy switches its artifact to RestApi in place and exposes it, in one transaction.
func TestLegacyRESTKindSwitchOnDeploy(t *testing.T) {
	f := newA2AFixture(t)
	legacy, key := f.legacyRestAgent(t)
	svc := f.agentService(key, string(utils.AgentSubTypeChatAPI))
	agent, err := svc.ocClient.GetComponent(context.Background(), key.OUID, key.ProjectName, key.AgentName)
	require.NoError(t, err)
	cfg := &models.AgentConfig{OUID: key.OUID, ProjectName: key.ProjectName, AgentName: key.AgentName, EnvironmentName: "dev"}

	require.NoError(t, svc.recordAgentEnvDeploy(context.Background(), agent, cfg, f.env.String(),
		svc.agentEnvArtifactFunc(agent, key.OUID, key.ProjectName, key.AgentName, f.env.String())))

	stored := f.storedArtifact(t, legacy)
	assert.Equal(t, models.KindRestAPI, stored.Kind)
	exposure := f.exposureOf(t, legacy)
	require.NotNil(t, exposure)
	assert.Equal(t, models.AgentExposureProtocolREST, exposure.Protocol)
	switched := models.ArtifactRef{OUID: key.OUID, ArtifactUUID: legacy.ArtifactUUID, Kind: models.KindRestAPI}
	row := f.row(t, switched)
	require.NotNil(t, row)
	assert.Equal(t, models.DeploymentStatusPending, row.Status)
	keys, err := repositories.NewAPIKeyRepo(db.GetDB()).ListPermanentByArtifactKinds(key.OUID, models.AgentArtifactKinds)
	require.NoError(t, err)
	require.Len(t, keys, 1)
	assert.Equal(t, "kept", keys[0].Name)
	assert.Equal(t, legacy.ArtifactUUID, keys[0].ArtifactUUID)
}

// sentAgent is an agent of subType exposed and sent to the fixture's gateway, with one API key.
func (f *a2aFixture) sentAgent(t *testing.T, subType string) (*agentManagerService, models.ArtifactRef, agentExposureKey) {
	t.Helper()
	newAgent := f.restAgent
	if subType == string(utils.AgentSubTypeA2A) {
		newAgent = f.platformAgent
	}
	artifact, key := newAgent(t)
	svc := f.agentService(key, subType)
	f.seedAPIKey(t, artifact, "kept", "active", nil)
	f.url.Store("http://agent.dev.svc:8080")
	agent, err := svc.ocClient.GetComponent(context.Background(), key.OUID, key.ProjectName, key.AgentName)
	require.NoError(t, err)
	_, err = svc.exposeAgentEnv(context.Background(), agent, key,
		svc.agentEnvArtifactFunc(agent, key.OUID, key.ProjectName, key.AgentName, f.env.String()), nil)
	require.NoError(t, err)
	require.NoError(t, f.worker.RunOnceFor(context.Background(), artifact.OUID, artifact.ArtifactUUID))
	f.hub.published = nil
	return svc, artifact, key
}

func TestUndeployTearsDownAgent(t *testing.T) {
	cases := map[string]string{"REST": string(utils.AgentSubTypeChatAPI), "A2A": string(utils.AgentSubTypeA2A)}
	undeployed := map[string]string{"REST": "api.undeployed", "A2A": "agent.undeployed"}
	for name, subType := range cases {
		t.Run(name, func(t *testing.T) {
			f := newA2AFixture(t)
			svc, artifact, key := f.sentAgent(t, subType)

			require.NoError(t, svc.UpdateAgentDeploymentState(tierGrantedCtx(t), key.OUID, key.ProjectName, key.AgentName,
				"dev", utils.DeploymentStateUndeploy))

			require.Len(t, f.hub.published, 1)
			assert.Equal(t, undeployed[name], string(f.hub.published[0].EventType))
			assert.Equal(t, f.gateway.String(), f.hub.published[0].GatewayID)
			assert.Nil(t, f.exposureOf(t, artifact), "the exposure is gone")
			assert.Equal(t, artifact.ArtifactUUID, f.storedArtifact(t, artifact).UUID, "the artifact is kept")
			keys, err := repositories.NewAPIKeyRepo(db.GetDB()).ListPermanentByArtifactKinds(key.OUID, models.AgentArtifactKinds)
			require.NoError(t, err)
			assert.Len(t, keys, 1, "the keys are kept")
		})
	}
}

func TestReactivateReexposes(t *testing.T) {
	f := newA2AFixture(t)
	svc, artifact, key := f.sentAgent(t, string(utils.AgentSubTypeCustomAPI))
	require.NoError(t, svc.UpdateAgentDeploymentState(tierGrantedCtx(t), key.OUID, key.ProjectName, key.AgentName,
		"dev", utils.DeploymentStateUndeploy))
	require.Nil(t, f.exposureOf(t, artifact))

	require.NoError(t, svc.UpdateAgentDeploymentState(tierGrantedCtx(t), key.OUID, key.ProjectName, key.AgentName,
		"dev", utils.DeploymentStateActive))

	exposure := f.exposureOf(t, artifact)
	require.NotNil(t, exposure)
	assert.Equal(t, models.AgentExposureProtocolREST, exposure.Protocol)
	require.NotNil(t, f.row(t, artifact).NextAttemptAt, "a deploy is owed on the gateway")
	f.hub.published = nil
	require.NoError(t, f.worker.RunOnceFor(context.Background(), artifact.OUID, artifact.ArtifactUUID))
	assert.Equal(t, models.DeploymentStatusDeployed, f.row(t, artifact).StatusDesired)
	require.Len(t, f.hub.published, 1)
	assert.Equal(t, "api.deployed", string(f.hub.published[0].EventType))
}

// trackBindingState makes the environment's binding follow the deployment state the service sets.
func trackBindingState(svc *agentManagerService) {
	oc := svc.ocClient.(*clientmocks.OpenChoreoClientMock)
	deployed := true
	oc.UpdateDeploymentStateFunc = func(_ context.Context, _, _, _, _ string, state gen.ReleaseBindingSpecState) error {
		deployed = state == gen.ReleaseBindingSpecStateActive
		return nil
	}
	oc.GetReleaseBindingRolloutFunc = func(context.Context, string, string, string) (client.ReleaseBindingRollout, error) {
		return client.ReleaseBindingRollout{Deployed: deployed}, nil
	}
	oc.UpdateReleaseBindingTraitConfigsFunc = func(context.Context, string, string, string, map[string]interface{}, map[string]interface{}) error {
		return nil
	}
}

// A settings save after undeploy keeps the settings for re-activation and exposes nothing until then.
func TestDeploySettingsAfterUndeployWaitsForReactivate(t *testing.T) {
	cases := map[string]string{"REST": string(utils.AgentSubTypeChatAPI), "A2A": string(utils.AgentSubTypeA2A)}
	deployed := map[string]string{"REST": "api.deployed", "A2A": "agent.deployed"}
	for name, subType := range cases {
		t.Run(name, func(t *testing.T) {
			f := newA2AFixture(t)
			svc, artifact, key := f.sentAgent(t, subType)
			trackBindingState(svc)
			require.NoError(t, svc.UpdateAgentDeploymentState(tierGrantedCtx(t), key.OUID, key.ProjectName, key.AgentName,
				"dev", utils.DeploymentStateUndeploy))
			require.NoError(t, f.worker.RunOnceFor(context.Background(), artifact.OUID, artifact.ArtifactUUID))
			undeployedRow := f.row(t, artifact)
			f.hub.published = nil
			timeout := int32(90)

			require.NoError(t, svc.UpdateAgentDeploySettings(tierGrantedCtx(t), key.OUID, key.ProjectName, key.AgentName,
				&spec.UpdateAgentDeploySettingsRequest{EnvironmentName: "dev", ResilienceTimeoutSeconds: &timeout}))
			require.NoError(t, f.worker.RunOnceFor(context.Background(), artifact.OUID, artifact.ArtifactUUID))

			assert.Nil(t, f.exposureOf(t, artifact), "nothing is exposed")
			assert.Equal(t, undeployedRow, f.row(t, artifact), "the gateway row is untouched")
			assert.Empty(t, f.hub.published)
			saved, err := svc.agentConfigRepo.Get(context.Background(), key.OUID, key.ProjectName, key.AgentName, "dev")
			require.NoError(t, err)
			require.NotNil(t, saved.ResilienceTimeoutSeconds)
			assert.Equal(t, timeout, *saved.ResilienceTimeoutSeconds)

			require.NoError(t, svc.UpdateAgentDeploymentState(tierGrantedCtx(t), key.OUID, key.ProjectName, key.AgentName,
				"dev", utils.DeploymentStateActive))
			require.NoError(t, f.worker.RunOnceFor(context.Background(), artifact.OUID, artifact.ArtifactUUID))

			assert.NotNil(t, f.exposureOf(t, artifact), "re-activation exposes the agent")
			require.Len(t, f.hub.published, 1)
			assert.Equal(t, deployed[name], string(f.hub.published[0].EventType))
		})
	}
}
