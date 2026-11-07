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
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// undeployedAgentID decodes the artifact UUID an agent.undeployed event names.
func undeployedAgentID(t *testing.T, evt eventhub.Event) string {
	t.Helper()
	require.Equal(t, eventhub.EventType("agent.undeployed"), evt.EventType)
	return deletedAgentID(t, evt)
}

// deletedAgentID decodes the artifact UUID an agent.deleted event names.
func deletedAgentID(t *testing.T, evt eventhub.Event) string {
	t.Helper()
	var envelope struct {
		Payload struct {
			AgentID string `json:"proxyId"`
		} `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(evt.EventData), &envelope))
	return envelope.Payload.AgentID
}

type agentArtifactDeletionFixture struct {
	svc          *agentManagerService
	hub          *recordingEventHub
	steps        []string
	deletedUUIDs []string
	queueDeletes int
	cardDeletes  int
	// failDelete makes the artifact delete of that UUID fail with the error.
	failDelete map[string]error
	gateways   map[string]string
	envs       map[string]uuid.UUID
}

// env is the UUID standing in for an environment the test names by label.
func (f *agentArtifactDeletionFixture) env(label string) uuid.UUID {
	if _, ok := f.envs[label]; !ok {
		f.envs[label] = uuid.New()
	}
	return f.envs[label]
}

// gateway is the UUID standing in for a gateway the test names by label.
func (f *agentArtifactDeletionFixture) gateway(label string) string {
	if _, ok := f.gateways[label]; !ok {
		f.gateways[label] = uuid.NewString()
	}
	return f.gateways[label]
}

// newAgentArtifactDeletionFixture stores an Agent artifact per environment UUID in artifacts,
// plus an unrelated agent's artifact the prefix lookup must not pick up.
func newAgentArtifactDeletionFixture(t *testing.T, artifacts map[string]uuid.UUID, deployedTo map[uuid.UUID][]string) *agentArtifactDeletionFixture {
	t.Helper()
	f := &agentArtifactDeletionFixture{hub: &recordingEventHub{}, gateways: map[string]string{}, envs: map[string]uuid.UUID{}}
	f.hub.onPublish = func() { f.steps = append(f.steps, "broadcast") }

	// OpenChoreo is not consulted: the artifacts themselves say which environments to clear.
	oc := &clientmocks.OpenChoreoClientMock{}
	stored := []*models.Artifact{{UUID: uuid.New(), Handle: agentEnvAPIArtifactHandle("proj", "agent-2", "env-dev"), Kind: models.KindAgent}}
	for env, id := range artifacts {
		handle := agentEnvAPIArtifactHandle("proj", "agent", f.env(env).String())
		stored = append(stored, &models.Artifact{UUID: id, Handle: handle, Kind: models.KindAgent})
	}
	artifactRepo := &repomocks.ArtifactRepositoryMock{
		ListByHandlePrefixFunc: func(_ context.Context, kinds []string, prefix, _ string) ([]*models.Artifact, error) {
			require.Equal(t, models.AgentArtifactKinds, kinds)
			var found []*models.Artifact
			for _, a := range stored {
				if strings.HasPrefix(a.Handle, prefix) {
					found = append(found, a)
				}
			}
			return found, nil
		},
		DeleteFunc: func(_ *gorm.DB, id string) error {
			if err := f.failDelete[id]; err != nil {
				return err
			}
			f.steps = append(f.steps, "delete-artifact")
			f.deletedUUIDs = append(f.deletedUUIDs, id)
			return nil
		},
	}
	sentTo := func(id uuid.UUID) []string {
		var sent []string
		for _, label := range deployedTo[id] {
			sent = append(sent, f.gateway(label))
		}
		return sent
	}
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		SentGatewaysFunc: func(_ context.Context, _ string, id uuid.UUID) ([]string, error) {
			return sentTo(id), nil
		},
		DeleteEventGatewaysFunc: func(_ context.Context, _ string, id uuid.UUID) ([]string, error) {
			f.steps = append(f.steps, "collect")
			return sentTo(id), nil
		},
	}
	deploymentRepo.WithTxFunc = func(*gorm.DB) repositories.DeploymentRepository { return deploymentRepo }
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{}, nil
		},
	}
	pubRepo := &repomocks.PlatformAgentExposureRepositoryMock{
		DeleteForAgentFunc: func(context.Context, string, string, string) error {
			f.steps = append(f.steps, "delete-queue")
			f.queueDeletes++
			return nil
		},
		DeleteForAgentEnvFunc: func(context.Context, string, string, string, uuid.UUID) error {
			f.steps = append(f.steps, "delete-exposure")
			return nil
		},
	}
	pubRepo.WithTxFunc = func(tx *gorm.DB) repositories.PlatformAgentExposureRepository {
		assert.Same(t, txSentinel, tx)
		return pubRepo
	}
	cardRepo := &repomocks.A2AAgentCardRepositoryMock{
		DeleteForAgentFunc: func(context.Context, string, string, string) error {
			f.steps = append(f.steps, "delete-cards")
			f.cardDeletes++
			return nil
		},
		DeleteForAgentEnvUUIDFunc: func(context.Context, string, string, string, uuid.UUID) error {
			f.steps = append(f.steps, "delete-card")
			return nil
		},
	}
	cardRepo.WithTxFunc = func(tx *gorm.DB) repositories.A2AAgentCardRepository {
		assert.Same(t, txSentinel, tx)
		return cardRepo
	}
	proxyRepo := &repomocks.A2AProxyRepositoryMock{
		DeleteForAgentFunc: func(context.Context, string, string, string) error {
			f.steps = append(f.steps, "delete-proxies")
			return nil
		},
		DeleteFunc: func(context.Context, string, string, string, uuid.UUID) error {
			f.steps = append(f.steps, "delete-proxy")
			return repositories.ErrA2AProxyNotFound
		},
	}
	proxyRepo.WithTxFunc = func(tx *gorm.DB) repositories.A2AProxyRepository {
		assert.Same(t, txSentinel, tx)
		return proxyRepo
	}
	events := NewGatewayEventsService(f.hub)
	publisher := withA2ACore(NewA2APublisher(sentinelTx, pubRepo, proxyRepo, cardRepo, artifactRepo, deploymentRepo,
		testLogger(), NewAgentExposurePublisher(sentinelTx, pubRepo, artifactRepo, deploymentRepo, gatewayRepo, nil, testLogger()), nil), f.hub)
	f.svc = NewAgentManagerService(nil, oc, nil, nil, nil, nil, nil, nil, artifactRepo, nil, gatewayRepo, nil, nil, nil, nil, nil,
		nil, publisher, cardRepo, deploymentRepo, events, publisher.exposure, nil, testLogger()).(*agentManagerService)
	return f
}

// Promote, deploy and gateway routing create one artifact per environment, so delete must clear
// every one of them, including an environment the project's pipeline does not list.
func TestDeleteAgentAPIArtifactClearsEveryEnvironmentArtifact(t *testing.T) {
	devID, qaID := uuid.New(), uuid.New()
	f := newAgentArtifactDeletionFixture(t,
		map[string]uuid.UUID{"env-dev": devID, "env-qa": qaID},
		map[uuid.UUID][]string{devID: {"gw-dev"}, qaID: {"gw-qa"}})

	require.NoError(t, f.svc.deleteAgentAPIArtifact(context.Background(), "org-1", "proj", "agent", true))

	assert.ElementsMatch(t, []string{devID.String(), qaID.String()}, f.deletedUUIDs)
	assert.Equal(t, 1, f.queueDeletes, "queue rows are cleared once per agent")
	require.Len(t, f.hub.published, 2)
	got := map[string]string{}
	for _, evt := range f.hub.published {
		got[evt.GatewayID] = deletedAgentID(t, evt)
	}
	assert.Equal(t, map[string]string{f.gateway("gw-dev"): devID.String(), f.gateway("gw-qa"): qaID.String()}, got)
}

// Each environment's A2A rows go with its artifact, in its transaction; targets are read before the artifact
// delete cascades the deployment rows away, and the gateway is told last. The agent's rows in environments without
// an artifact go after.
func TestDeleteAgentAPIArtifactBroadcastsLast(t *testing.T) {
	devID := uuid.New()
	f := newAgentArtifactDeletionFixture(t,
		map[string]uuid.UUID{"env-dev": devID},
		map[uuid.UUID][]string{devID: {"gw-dev"}})

	require.NoError(t, f.svc.deleteAgentAPIArtifact(context.Background(), "org-1", "proj", "agent", true))

	assert.Equal(t, []string{
		"delete-exposure", "delete-proxy", "delete-card", "collect", "delete-artifact", "broadcast",
		"delete-queue", "delete-proxies", "delete-cards",
	}, f.steps)
}

// A REST agent's artifacts never reached a gateway as an A2A Agent: no events.
func TestDeleteAgentAPIArtifactSendsNothingForARESTAgent(t *testing.T) {
	devID := uuid.New()
	f := newAgentArtifactDeletionFixture(t, map[string]uuid.UUID{"env-dev": devID}, nil)
	f.svc.gatewayRepo = &repomocks.GatewayRepositoryMock{} // listing gateways would panic

	require.NoError(t, f.svc.deleteAgentAPIArtifact(context.Background(), "org-1", "proj", "agent", false))

	assert.Equal(t, []string{devID.String()}, f.deletedUUIDs)
	assert.Empty(t, f.hub.published)
}

// A failed artifact lookup leaves nothing deleted or broadcast, and the rows already gone.
// A listing failure leaves the artifacts and their gateway Agents behind, so the delete must say so:
// the component is gone by then and only another delete reaches this cleanup again.
func TestDeleteAgentAPIArtifactReportsAListFailure(t *testing.T) {
	devID := uuid.New()
	f := newAgentArtifactDeletionFixture(t, map[string]uuid.UUID{"env-dev": devID}, nil)
	repo, ok := f.svc.artifactRepo.(*repomocks.ArtifactRepositoryMock)
	require.True(t, ok)
	boom := errors.New("db unavailable")
	repo.ListByHandlePrefixFunc = func(context.Context, []string, string, string) ([]*models.Artifact, error) {
		return nil, boom
	}

	err := f.svc.deleteAgentAPIArtifact(context.Background(), "org-1", "proj", "agent", false)

	assert.ErrorIs(t, err, boom)
	assert.Empty(t, f.deletedUUIDs)
	assert.Empty(t, f.hub.published)
}

// One artifact that will not delete is reported, while the others are still removed and withdrawn.
func TestDeleteAgentAPIArtifactReportsADeleteFailureAfterTheRest(t *testing.T) {
	devID, qaID := uuid.New(), uuid.New()
	f := newAgentArtifactDeletionFixture(t, map[string]uuid.UUID{"env-dev": devID, "env-qa": qaID},
		map[uuid.UUID][]string{devID: {"gw-1"}, qaID: {"gw-1"}})
	boom := errors.New("db unavailable")
	f.failDelete = map[string]error{qaID.String(): boom}

	err := f.svc.deleteAgentAPIArtifact(context.Background(), "org-1", "proj", "agent", true)

	assert.ErrorIs(t, err, boom)
	assert.Equal(t, []string{devID.String()}, f.deletedUUIDs)
	require.Len(t, f.hub.published, 1, "only the removed artifact is withdrawn; the other still exists")
	assert.Equal(t, devID.String(), deletedAgentID(t, f.hub.published[0]))
	assert.Zero(t, f.queueDeletes, "the agent-wide sweep waits for the retry")
}

// Card rows go with the agent, external A2A agents included, so a same-named agent starts clean.
func TestDeleteAgentAPIArtifactClearsCardRowsOnce(t *testing.T) {
	devID := uuid.New()
	f := newAgentArtifactDeletionFixture(t, map[string]uuid.UUID{"env-dev": devID}, nil)
	f.svc.gatewayRepo = &repomocks.GatewayRepositoryMock{}

	require.NoError(t, f.svc.deleteAgentAPIArtifact(context.Background(), "org-1", "proj", "agent", false))

	assert.Equal(t, 1, f.cardDeletes)
}
