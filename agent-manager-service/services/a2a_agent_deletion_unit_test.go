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
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// Deletion goes to every gateway the artifact was deployed to AND every active
// gateway in the org, best effort — the same union broadcastMCPProxyDeletion
// uses, because a gateway that holds a stale copy is worse than a redundant
// event.
func TestBroadcastA2AAgentDeletionReachesEveryCandidateGateway(t *testing.T) {
	artifactUUID := uuid.New()
	hub := &recordingEventHub{}
	events := NewGatewayEventsService(hub)

	deployedGatewayID := uuid.New()
	otherActiveGateway := &models.Gateway{UUID: uuid.New(), Name: "ingress-staging"}

	deploymentRepo := &repomocks.DeploymentRepositoryMock{
		GetDeployedGatewaysByProviderFunc: func(providerUUID uuid.UUID, orgUUID string) ([]string, error) {
			return []string{deployedGatewayID.String()}, nil
		},
	}
	gatewayRepo := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(opts repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			// The gateway the artifact was deployed to is also still active, so
			// the two sources overlap — which is what makes the dedup assertion
			// below meaningful.
			return []*models.Gateway{{UUID: deployedGatewayID}, otherActiveGateway}, nil
		},
	}

	broadcastA2AAgentDeletion(context.Background(), events, deploymentRepo, gatewayRepo, artifactUUID, "org-1", testLogger())

	require.Len(t, hub.published, 2)
	seen := map[string]bool{}
	for _, evt := range hub.published {
		assert.Equal(t, eventhub.EventType("agent.deleted"), evt.EventType)
		var envelope struct {
			Payload struct {
				AgentID string `json:"agentId"`
			} `json:"payload"`
		}
		require.NoError(t, json.Unmarshal([]byte(evt.EventData), &envelope))
		assert.Equal(t, artifactUUID.String(), envelope.Payload.AgentID)
		assert.False(t, seen[evt.GatewayID], "no gateway is told twice")
		seen[evt.GatewayID] = true
	}
	assert.True(t, seen[deployedGatewayID.String()], "the gateway holding the agent is told")
	assert.True(t, seen[otherActiveGateway.UUID.String()], "so is every other active gateway")
}

// An agent that never reached a gateway has no artifact UUID to name, so there
// is nothing to broadcast — and a nil-UUID event would ask every gateway to
// delete an agent identified by all zeroes.
func TestBroadcastA2AAgentDeletionIgnoresAMissingArtifact(t *testing.T) {
	hub := &recordingEventHub{}
	broadcastA2AAgentDeletion(context.Background(), NewGatewayEventsService(hub), nil, nil, uuid.Nil, "org-1", testLogger())
	assert.Empty(t, hub.published)
}
