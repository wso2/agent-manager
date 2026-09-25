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

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

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
	svc := &agentManagerService{a2aPublicationRepo: repo, logger: testLogger()}

	envUUID := uuid.New()
	artifactUUID := uuid.New()
	svc.enqueueA2APublication(context.Background(), "org-1", "checkout", "trip-planner", "dev", envUUID, artifactUUID)

	require.Len(t, enqueued, 1)
	assert.Equal(t, "trip-planner", enqueued[0].AgentName)
	assert.Equal(t, "dev", enqueued[0].EnvironmentName)
	assert.Equal(t, envUUID, enqueued[0].EnvironmentUUID)
	assert.Equal(t, artifactUUID, enqueued[0].ArtifactUUID)
}

// A queue failure must not fail a deploy that has otherwise succeeded — the
// reconciler is not the deploy's critical path, and the agent is running.
func TestEnqueueA2APublicationSurvivesRepoFailure(t *testing.T) {
	repo := &repomocks.A2APublicationRepositoryMock{
		EnqueueFunc: func(ctx context.Context, pub *models.A2APublication) error {
			return assert.AnError
		},
	}
	svc := &agentManagerService{a2aPublicationRepo: repo, logger: testLogger()}
	assert.NotPanics(t, func() {
		svc.enqueueA2APublication(context.Background(), "org-1", "checkout", "trip-planner", "dev", uuid.New(), uuid.New())
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
	svc := &agentManagerService{a2aPublicationRepo: repo, logger: testLogger()}
	agent := &models.AgentResponse{Type: models.AgentType{Type: "agent-api", SubType: "a2a-agent"}}
	envUUID, artifactUUID := uuid.New(), uuid.New()

	require.NoError(t, svc.republishA2AAgent(context.Background(), agent, "org-1", "checkout", "trip-planner", "staging", envUUID.String(), artifactUUID))

	require.Len(t, enqueued, 1)
	assert.Equal(t, "staging", enqueued[0].EnvironmentName)
	assert.Equal(t, envUUID, enqueued[0].EnvironmentUUID)
	assert.Equal(t, artifactUUID, enqueued[0].ArtifactUUID)
}

// A REST agent's policies ride on its release binding, which the caller has
// already updated; queueing it would publish an Agent resource for a non-agent.
func TestRepublishA2AAgentIgnoresOtherAgents(t *testing.T) {
	repo := &repomocks.A2APublicationRepositoryMock{}
	svc := &agentManagerService{a2aPublicationRepo: repo, logger: testLogger()}
	agent := &models.AgentResponse{Type: models.AgentType{Type: "agent-api", SubType: "chat-api"}}

	require.NoError(t, svc.republishA2AAgent(context.Background(), agent, "org-1", "checkout", "greeter", "dev", uuid.New().String(), uuid.New()))
	assert.Empty(t, repo.EnqueueCalls())
}

func TestRepublishA2AAgentRejectsUnparseableEnvironmentUUID(t *testing.T) {
	svc := &agentManagerService{a2aPublicationRepo: &repomocks.A2APublicationRepositoryMock{}, logger: testLogger()}
	agent := &models.AgentResponse{Type: models.AgentType{Type: "agent-api", SubType: "a2a-agent"}}
	err := svc.republishA2AAgent(context.Background(), agent, "org-1", "checkout", "trip-planner", "dev", "not-a-uuid", uuid.New())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not-a-uuid")
}
