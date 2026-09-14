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
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// recordingEventHub captures what would be published, so the event's wire shape
// can be asserted without a live hub. Only publication is exercised; the rest of
// the EventHub surface is present to satisfy the interface.
type recordingEventHub struct {
	published []eventhub.Event
}

func (h *recordingEventHub) PublishEvent(gatewayID string, evt eventhub.Event) error {
	h.published = append(h.published, evt)
	return nil
}

func (h *recordingEventHub) Initialize() error                      { return nil }
func (h *recordingEventHub) RegisterGateway(gatewayID string) error { return nil }

func (h *recordingEventHub) Subscribe(gatewayID string) (<-chan eventhub.Event, error) {
	return nil, nil //nolint:nilnil // the recorder never delivers; only PublishEvent is exercised
}

func (h *recordingEventHub) Unsubscribe(gatewayID string, subscriber <-chan eventhub.Event) error {
	return nil
}
func (h *recordingEventHub) UnsubscribeAll(gatewayID string) error { return nil }
func (h *recordingEventHub) CleanUpEvents() error                  { return nil }
func (h *recordingEventHub) Close() error                          { return nil }

// The gateway parses these names and payloads verbatim
// (pkg/controlplane/events.go and client.go's dispatch switch); they are fixed
// by that code, not chosen here.
func TestBroadcastAgentDeploymentEvent(t *testing.T) {
	hub := &recordingEventHub{}
	svc := NewGatewayEventsService(hub)

	performedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	err := svc.BroadcastAgentDeploymentEvent("gw-1", &models.AgentDeploymentEvent{
		AgentID:      "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f",
		DeploymentID: "dep-1",
		PerformedAt:  performedAt,
	})
	require.NoError(t, err)
	require.Len(t, hub.published, 1)

	evt := hub.published[0]
	assert.Equal(t, eventhub.EventType("agent.deployed"), evt.EventType)
	assert.Equal(t, "CREATE", evt.Action)
	assert.Equal(t, "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f", evt.EntityID)

	var envelope struct {
		Type    string `json:"type"`
		Payload struct {
			AgentID      string    `json:"agentId"`
			DeploymentID string    `json:"deploymentId"`
			PerformedAt  time.Time `json:"performedAt"`
		} `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(evt.EventData), &envelope))
	assert.Equal(t, "agent.deployed", envelope.Type)
	assert.Equal(t, "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f", envelope.Payload.AgentID)
	assert.Equal(t, "dep-1", envelope.Payload.DeploymentID)
	assert.True(t, performedAt.Equal(envelope.Payload.PerformedAt))
}

func TestBroadcastAgentDeletionEvent(t *testing.T) {
	hub := &recordingEventHub{}
	svc := NewGatewayEventsService(hub)

	err := svc.BroadcastAgentDeletionEvent("gw-1", &models.AgentDeletionEvent{
		AgentID: "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f",
	})
	require.NoError(t, err)
	require.Len(t, hub.published, 1)

	evt := hub.published[0]
	assert.Equal(t, eventhub.EventType("agent.deleted"), evt.EventType)
	assert.Equal(t, "DELETE", evt.Action)

	var envelope struct {
		Type    string `json:"type"`
		Payload struct {
			AgentID string `json:"agentId"`
		} `json:"payload"`
	}
	require.NoError(t, json.Unmarshal([]byte(evt.EventData), &envelope))
	assert.Equal(t, "agent.deleted", envelope.Type)
	assert.Equal(t, "0192f4c1-9a7d-7c3e-b4f2-1a2b3c4d5e6f", envelope.Payload.AgentID)
}
