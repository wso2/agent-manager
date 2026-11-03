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

package eventhub

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewGatewayEventWrapsThePayloadInTheGatewayEnvelope(t *testing.T) {
	payload := map[string]string{"proxyId": "p-1", "deploymentId": ""}

	evt := NewGatewayEvent("agent.undeployed", "DELETE", "p-1", payload)

	require.NoError(t, evt.Err())
	assert.Equal(t, EventType("agent.undeployed"), evt.EventType)
	assert.Equal(t, "DELETE", evt.Action)
	assert.Equal(t, "p-1", evt.EntityID)
	assert.False(t, evt.OriginatedTimestamp.IsZero())
	var envelope struct {
		Type          string            `json:"type"`
		Payload       map[string]string `json:"payload"`
		Timestamp     string            `json:"timestamp"`
		CorrelationID string            `json:"correlationId"`
	}
	require.NoError(t, json.Unmarshal([]byte(evt.EventData), &envelope))
	assert.Equal(t, "agent.undeployed", envelope.Type)
	assert.Equal(t, payload, envelope.Payload, "an empty deploymentId stays on the wire")
	assert.NotEmpty(t, envelope.Timestamp)
	assert.NotEmpty(t, envelope.CorrelationID)
}

func TestNewGatewayEventCarriesItsBuildError(t *testing.T) {
	t.Run("a payload that can't be encoded", func(t *testing.T) {
		assert.Error(t, NewGatewayEvent("x.deployed", "CREATE", "id", make(chan int)).Err())
	})
	t.Run("a payload over the limit", func(t *testing.T) {
		big := strings.Repeat("a", MaxEventPayloadSize)
		assert.ErrorContains(t, NewGatewayEvent("x.deployed", "CREATE", "id", big).Err(), "exceeds maximum size")
	})
}
