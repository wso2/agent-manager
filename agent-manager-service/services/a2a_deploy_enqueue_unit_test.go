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

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// a2aPublishingAgentService is an agentManagerService whose only A2A collaborator is the publisher.
func a2aPublishingAgentService(pubs *repomocks.A2APublicationRepositoryMock, proxies *repomocks.A2AProxyRepositoryMock) *agentManagerService {
	publisher := NewA2APublisher(nil, pubs, proxies, nil, nil, nil, nil, nil, testLogger())
	return NewAgentManagerService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		publisher, nil, nil, nil, testLogger()).(*agentManagerService)
}

// A deploy records the intent to publish rather than publishing inline: the
// upstream address is not readable until the release binding reconciles.
func TestEnqueueA2APublicationRecordsTheArtifactUUID(t *testing.T) {
	var enqueued []models.A2APublication
	repo := &repomocks.A2APublicationRepositoryMock{
		EnqueueFunc: func(ctx context.Context, pub *models.A2APublication) error {
			enqueued = append(enqueued, *pub)
			return nil
		},
	}
	svc := a2aPublishingAgentService(repo, &repomocks.A2AProxyRepositoryMock{})

	envUUID := uuid.New()
	artifactUUID := uuid.New()
	agentUUID := uuid.New()
	svc.enqueueA2APublication(context.Background(), a2aPublicationKey{OUID: "org-1", ProjectName: "checkout", AgentName: "trip-planner", EnvironmentName: "dev", EnvironmentUUID: envUUID, AgentUUID: &agentUUID}, models.A2APublicationSourcePlatform, artifactUUID)

	require.Len(t, enqueued, 1)
	assert.Equal(t, "trip-planner", enqueued[0].AgentName)
	assert.Equal(t, "dev", enqueued[0].EnvironmentName)
	assert.Equal(t, envUUID, enqueued[0].EnvironmentUUID)
	assert.Equal(t, artifactUUID, enqueued[0].ArtifactUUID)
	assert.Equal(t, &agentUUID, enqueued[0].AgentUUID)
}

// A queue failure must not fail a deploy that has otherwise succeeded — the
// reconciler is not the deploy's critical path, and the agent is running.
func TestEnqueueA2APublicationSurvivesRepoFailure(t *testing.T) {
	repo := &repomocks.A2APublicationRepositoryMock{
		EnqueueFunc: func(ctx context.Context, pub *models.A2APublication) error {
			return assert.AnError
		},
	}
	svc := a2aPublishingAgentService(repo, &repomocks.A2AProxyRepositoryMock{})
	assert.NotPanics(t, func() {
		svc.enqueueA2APublication(context.Background(), a2aPublicationKey{OUID: "org-1", ProjectName: "checkout", AgentName: "trip-planner", EnvironmentName: "dev", EnvironmentUUID: uuid.New()}, models.A2APublicationSourcePlatform, uuid.New())
	})
}

func TestRepublishA2AAgentQueuesA2AAgents(t *testing.T) {
	var enqueued []models.A2APublication
	repo := &repomocks.A2APublicationRepositoryMock{
		EnqueueFunc: func(ctx context.Context, pub *models.A2APublication) error {
			enqueued = append(enqueued, *pub)
			return nil
		},
	}
	svc := a2aPublishingAgentService(repo, &repomocks.A2AProxyRepositoryMock{})
	agent := &models.AgentResponse{UUID: "6f1c2a7e-4c1b-4e0a-9a51-2b7f0e3d9c11", Type: models.AgentType{Type: "agent-api", SubType: "a2a-agent"}}
	envUUID, artifactUUID := uuid.New(), uuid.New()

	require.NoError(t, svc.republishA2AAgent(context.Background(), agent, "org-1", "checkout", "trip-planner", "staging", envUUID.String(), artifactUUID))

	require.Len(t, enqueued, 1)
	assert.Equal(t, "staging", enqueued[0].EnvironmentName)
	assert.Equal(t, envUUID, enqueued[0].EnvironmentUUID)
	assert.Equal(t, artifactUUID, enqueued[0].ArtifactUUID)
	require.NotNil(t, enqueued[0].AgentUUID)
	assert.Equal(t, agent.UUID, enqueued[0].AgentUUID.String())
}

// A REST agent's policies ride on its release binding, which the caller has
// already updated; queueing it would publish an Agent resource for a non-agent.
func TestRepublishA2AAgentIgnoresOtherAgents(t *testing.T) {
	repo := &repomocks.A2APublicationRepositoryMock{}
	svc := a2aPublishingAgentService(repo, &repomocks.A2AProxyRepositoryMock{})
	agent := &models.AgentResponse{Type: models.AgentType{Type: "agent-api", SubType: "chat-api"}}

	require.NoError(t, svc.republishA2AAgent(context.Background(), agent, "org-1", "checkout", "greeter", "dev", uuid.New().String(), uuid.New()))
	assert.Empty(t, repo.EnqueueCalls())
}

func TestRepublishA2AAgentRejectsUnparseableEnvironmentUUID(t *testing.T) {
	svc := a2aPublishingAgentService(&repomocks.A2APublicationRepositoryMock{}, &repomocks.A2AProxyRepositoryMock{})
	agent := &models.AgentResponse{Type: models.AgentType{Type: "agent-api", SubType: "a2a-agent"}}
	err := svc.republishA2AAgent(context.Background(), agent, "org-1", "checkout", "trip-planner", "dev", "not-a-uuid", uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-uuid")
}

// A new A2A agent's first binding comes from create, not a deploy, so create
// must queue the first environment's publication itself.
func TestEnqueueCreatedA2AAgentQueuesTheFirstEnvironment(t *testing.T) {
	var enqueued []models.A2APublication
	repo := &repomocks.A2APublicationRepositoryMock{
		EnqueueFunc: func(ctx context.Context, pub *models.A2APublication) error {
			enqueued = append(enqueued, *pub)
			return nil
		},
	}
	agentUUID := uuid.New()
	svc := a2aPublishingAgentService(repo, &repomocks.A2AProxyRepositoryMock{})
	svc.ocClient = &clientmocks.OpenChoreoClientMock{
		GetComponentFunc: func(_ context.Context, _, _, name string) (*models.AgentResponse, error) {
			return &models.AgentResponse{UUID: agentUUID.String(), Name: name}, nil
		},
	}
	envUUID := uuid.New()
	artifact := &models.Artifact{UUID: uuid.New()}

	svc.enqueueCreatedA2AAgent(context.Background(), "a2a-agent", "org-1", "checkout", "trip-planner", "dev", envUUID.String(), artifact)

	require.Len(t, enqueued, 1)
	assert.Equal(t, "dev", enqueued[0].EnvironmentName)
	assert.Equal(t, envUUID, enqueued[0].EnvironmentUUID)
	assert.Equal(t, artifact.UUID, enqueued[0].ArtifactUUID)
	assert.Equal(t, &agentUUID, enqueued[0].AgentUUID)
}

// The UUID only sharpens the reconciler's cleanup, so a failed lookup still queues the row.
func TestEnqueueCreatedA2AAgentQueuesEvenWhenTheAgentLookupFails(t *testing.T) {
	var enqueued []models.A2APublication
	repo := &repomocks.A2APublicationRepositoryMock{
		EnqueueFunc: func(ctx context.Context, pub *models.A2APublication) error {
			enqueued = append(enqueued, *pub)
			return nil
		},
	}
	svc := a2aPublishingAgentService(repo, &repomocks.A2AProxyRepositoryMock{})
	svc.ocClient = &clientmocks.OpenChoreoClientMock{
		GetComponentFunc: func(context.Context, string, string, string) (*models.AgentResponse, error) {
			return nil, assert.AnError
		},
	}

	svc.enqueueCreatedA2AAgent(context.Background(), "a2a-agent", "org-1", "checkout", "trip-planner", "dev", uuid.New().String(), &models.Artifact{UUID: uuid.New()})

	require.Len(t, enqueued, 1)
	assert.Nil(t, enqueued[0].AgentUUID)
}

// Non-A2A agents and agents without an env artifact have nothing to publish; a
// nil EnqueueFunc panics if any of these paths reaches the queue.
func TestEnqueueCreatedA2AAgentSkipsAgentsWithNothingToPublish(t *testing.T) {
	svc := a2aPublishingAgentService(&repomocks.A2APublicationRepositoryMock{}, &repomocks.A2AProxyRepositoryMock{})
	envUUID := uuid.New().String()

	assert.NotPanics(t, func() {
		svc.enqueueCreatedA2AAgent(context.Background(), "chat-api", "org-1", "checkout", "chat", "dev", envUUID, &models.Artifact{UUID: uuid.New()})
		svc.enqueueCreatedA2AAgent(context.Background(), "a2a-agent", "org-1", "checkout", "trip-planner", "dev", envUUID, nil)
		svc.enqueueCreatedA2AAgent(context.Background(), "a2a-agent", "org-1", "checkout", "trip-planner", "dev", "not-a-uuid", &models.Artifact{UUID: uuid.New()})
	})
}

// A proxied external agent publishes its origin, so its row must say so or the worker reads a release binding.
func TestRepublishA2AAgentQueuesAProxiedExternalAgentAsExternal(t *testing.T) {
	var enqueued []models.A2APublication
	pubs := &repomocks.A2APublicationRepositoryMock{
		EnqueueFunc: func(_ context.Context, pub *models.A2APublication) error {
			enqueued = append(enqueued, *pub)
			return nil
		},
	}
	proxies := &repomocks.A2AProxyRepositoryMock{
		GetByEnvUUIDFunc: func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
			return &models.A2AProxy{}, nil
		},
	}
	svc := a2aPublishingAgentService(pubs, proxies)

	require.NoError(t, svc.republishA2AAgent(context.Background(), externalA2AAgent(), "org-1", "checkout", "weather", "dev", uuid.New().String(), uuid.New()))

	require.Len(t, enqueued, 1)
	assert.Equal(t, models.A2APublicationSourceExternal, enqueued[0].Source)
}

func TestRepublishA2AAgentSkipsADirectExternalAgent(t *testing.T) {
	pubs := &repomocks.A2APublicationRepositoryMock{} // EnqueueFunc is nil: queueing would panic
	proxies := &repomocks.A2AProxyRepositoryMock{
		GetByEnvUUIDFunc: func(context.Context, string, string, string, uuid.UUID) (*models.A2AProxy, error) {
			return nil, repositories.ErrA2AProxyNotFound
		},
	}
	svc := a2aPublishingAgentService(pubs, proxies)

	require.NoError(t, svc.republishA2AAgent(context.Background(), externalA2AAgent(), "org-1", "checkout", "weather", "dev", uuid.New().String(), uuid.New()))
	assert.Empty(t, pubs.EnqueueCalls())
}

// A same-named agent created later must not inherit a proxy, or its secret.
func TestClearInheritedA2AStateDropsPublicationsAndProxies(t *testing.T) {
	pubs := &repomocks.A2APublicationRepositoryMock{DeleteForAgentFunc: func(context.Context, string, string, string) error { return nil }}
	proxies := &repomocks.A2AProxyRepositoryMock{DeleteForAgentFunc: func(context.Context, string, string, string) error { return nil }}
	svc := a2aPublishingAgentService(pubs, proxies)

	require.NoError(t, svc.clearInheritedA2AState(context.Background(), "org-1", "checkout", "weather"))
	assert.Len(t, pubs.DeleteForAgentCalls(), 1)
	assert.Len(t, proxies.DeleteForAgentCalls(), 1)
}
