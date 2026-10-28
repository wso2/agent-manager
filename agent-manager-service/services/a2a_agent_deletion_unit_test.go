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

// Deletion goes to every gateway the artifact was deployed to AND every active
// gateway in the org — the same union broadcastMCPProxyDeletion uses, because a
// gateway that holds a stale copy is worse than a redundant event.
func TestCollectA2AAgentDeletionTargetsUnionsDeployedAndActiveGateways(t *testing.T) {
	artifactUUID := uuid.New()
	deployedGatewayID := uuid.New()
	otherActiveGateway := &models.Gateway{UUID: uuid.New(), Name: "ingress-staging"}

	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(providerUUID uuid.UUID, orgUUID string) ([]string, error) {
			return []string{deployedGatewayID.String()}, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(opts repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			// Overlaps with the deployed gateway, which is what makes the dedup meaningful.
			return []*models.Gateway{{UUID: deployedGatewayID}, otherActiveGateway}, nil
		},
	}

	got := collectA2AAgentDeletionTargets(deploymentRepo, gatewayRepo, artifactUUID, "org-1", true, testLogger())

	assert.ElementsMatch(t, []string{deployedGatewayID.String(), otherActiveGateway.UUID.String()}, got)
}

// A non-A2A agent with no deployment rows never reached a gateway, so the org's
// gateways are not even listed.
func TestCollectA2AAgentDeletionTargetsSkipsANonA2AAgent(t *testing.T) {
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
			return []string{}, nil
		},
	}
	// ListWithFiltersFunc left nil: listing the org's gateways would panic.
	gatewayRepo := &repomocks.GatewayRepositoryMock{}

	got := collectA2AAgentDeletionTargets(deploymentRepo, gatewayRepo, uuid.New(), "org-1", false, testLogger())

	assert.Empty(t, got)
}

// When the subtype is unknown (the component is already gone), deployment rows
// are the proof the artifact reached a gateway as an A2A Agent.
func TestCollectA2AAgentDeletionTargetsTrustsDeploymentRowsWhenSubtypeUnknown(t *testing.T) {
	deployedGatewayID := uuid.NewString()
	activeGatewayID := uuid.New()
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(uuid.UUID, string) ([]string, error) {
			return []string{deployedGatewayID}, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{{UUID: activeGatewayID}}, nil
		},
	}

	got := collectA2AAgentDeletionTargets(deploymentRepo, gatewayRepo, uuid.New(), "org-1", false, testLogger())

	assert.ElementsMatch(t, []string{deployedGatewayID, activeGatewayID.String()}, got)
}

// A nil-UUID event would ask every gateway to delete an agent identified by all zeroes.
func TestSendA2AAgentDeletionIgnoresAMissingArtifact(t *testing.T) {
	hub := &recordingEventHub{}
	sendA2AAgentWithdrawal(context.Background(), NewGatewayEventsService(hub), uuid.Nil, []string{"gw-1"}, false, testLogger())
	assert.Empty(t, hub.published)
}

// The delete runs on the request context; the broadcast must survive the handler returning.
func TestSendA2AAgentDeletionOutlivesRequestCancellation(t *testing.T) {
	type ctxKey struct{}
	reqCtx, cancel := context.WithCancel(context.WithValue(context.Background(), ctxKey{}, "trace-1"))
	cancel()
	hub := &recordingEventHub{}
	artifactUUID := uuid.New()

	sendA2AAgentWithdrawal(reqCtx, NewGatewayEventsService(hub), artifactUUID, []string{"gw-1", "gw-2"}, false, testLogger())

	require.Len(t, hub.published, 2)
	for i, evt := range hub.published {
		assert.Equal(t, eventhub.EventType("agent.deleted"), evt.EventType)
		assert.Equal(t, artifactUUID.String(), deletedAgentID(t, evt))
		require.NoError(t, hub.contexts[i].Err(), "request cancellation does not reach the hub")
		assert.Equal(t, "trace-1", hub.contexts[i].Value(ctxKey{}), "request values do")
	}
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
}

// newAgentArtifactDeletionFixture stores an Agent artifact per environment UUID in artifacts,
// plus an unrelated agent's artifact the prefix lookup must not pick up.
func newAgentArtifactDeletionFixture(t *testing.T, artifacts map[string]uuid.UUID, deployedTo map[uuid.UUID][]string) *agentArtifactDeletionFixture {
	t.Helper()
	f := &agentArtifactDeletionFixture{hub: &recordingEventHub{}}
	f.hub.onPublish = func() { f.steps = append(f.steps, "broadcast") }

	// OpenChoreo is not consulted: the artifacts themselves say which environments to clear.
	oc := &clientmocks.OpenChoreoClientMock{}
	stored := []*models.Artifact{{UUID: uuid.New(), Handle: agentEnvAPIArtifactHandle("proj", "agent-2", "env-dev"), Kind: models.KindAgent}}
	for envUUID, id := range artifacts {
		stored = append(stored, &models.Artifact{UUID: id, Handle: agentEnvAPIArtifactHandle("proj", "agent", envUUID), Kind: models.KindAgent})
	}
	artifactRepo := &repomocks.ArtifactRepositoryMock{
		ListByHandlePrefixFunc: func(_ context.Context, kind, prefix, _ string) ([]*models.Artifact, error) {
			require.Equal(t, models.KindAgent, kind)
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
	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(id uuid.UUID, _ string) ([]string, error) {
			f.steps = append(f.steps, "collect")
			return append([]string{}, deployedTo[id]...), nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return []*models.Gateway{}, nil
		},
	}
	pubRepo := &repomocks.A2APublicationRepositoryMock{
		DeleteForAgentFunc: func(context.Context, string, string, string) error {
			f.steps = append(f.steps, "delete-queue")
			f.queueDeletes++
			return nil
		},
	}
	cardRepo := &repomocks.A2AAgentCardRepositoryMock{
		DeleteForAgentFunc: func(context.Context, string, string, string) error {
			f.steps = append(f.steps, "delete-cards")
			f.cardDeletes++
			return nil
		},
	}
	proxyRepo := &repomocks.A2AProxyRepositoryMock{
		DeleteForAgentFunc: func(context.Context, string, string, string) error {
			f.steps = append(f.steps, "delete-proxies")
			return nil
		},
	}
	events := NewGatewayEventsService(f.hub)
	publisher := NewA2APublisher(nil, pubRepo, proxyRepo, cardRepo, artifactRepo, deploymentRepo, gatewayRepo, events, testLogger())
	f.svc = NewAgentManagerService(nil, oc, nil, nil, nil, nil, nil, nil, artifactRepo, nil, gatewayRepo, nil, nil, nil, nil, nil,
		publisher, cardRepo, deploymentRepo, events, testLogger()).(*agentManagerService)
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
	assert.Equal(t, map[string]string{"gw-dev": devID.String(), "gw-qa": qaID.String()}, got)
}

// Targets are read before the artifact delete cascades the deployment rows away,
// and the gateway is told only once nothing is left to re-publish or re-serve it.
func TestDeleteAgentAPIArtifactBroadcastsLast(t *testing.T) {
	devID := uuid.New()
	f := newAgentArtifactDeletionFixture(t,
		map[string]uuid.UUID{"env-dev": devID},
		map[uuid.UUID][]string{devID: {"gw-dev"}})

	require.NoError(t, f.svc.deleteAgentAPIArtifact(context.Background(), "org-1", "proj", "agent", true))

	assert.Equal(t, []string{"delete-queue", "delete-proxies", "delete-cards", "collect", "delete-artifact", "broadcast"}, f.steps)
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
	repo.ListByHandlePrefixFunc = func(context.Context, string, string, string) ([]*models.Artifact, error) {
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
}

// Card rows go with the agent, external A2A agents included, so a same-named agent starts clean.
func TestDeleteAgentAPIArtifactClearsCardRowsOnce(t *testing.T) {
	devID := uuid.New()
	f := newAgentArtifactDeletionFixture(t, map[string]uuid.UUID{"env-dev": devID}, nil)
	f.svc.gatewayRepo = &repomocks.GatewayRepositoryMock{}

	require.NoError(t, f.svc.deleteAgentAPIArtifact(context.Background(), "org-1", "proj", "agent", false))

	assert.Equal(t, 1, f.cardDeletes)
}
