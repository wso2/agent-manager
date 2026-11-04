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
	"fmt"
	"time"

	"github.com/google/uuid"
)

// MaxEventPayloadSize is the largest event payload a gateway is sent (1 MiB).
const MaxEventPayloadSize = 1024 * 1024

// GatewayEventEnvelope is what a gateway receives over its WebSocket for every event. EventData holds it whole.
type GatewayEventEnvelope struct {
	Type          string `json:"type"`
	Payload       any    `json:"payload"`
	Timestamp     string `json:"timestamp"`
	CorrelationID string `json:"correlationId"`
	UserId        string `json:"userId,omitempty"`
}

// NewGatewayEvent builds the event that tells a gateway about entityID: payload wrapped in the gateway's envelope.
// It is the one place gateway events are built. A payload that can't be encoded, or is over MaxEventPayloadSize,
// gives an event that fails to publish (Err).
func NewGatewayEvent(eventType, action, entityID string, payload any) Event {
	now := time.Now()
	evt := Event{OriginatedTimestamp: now, EventType: EventType(eventType), Action: action, EntityID: entityID}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		evt.err = fmt.Errorf("failed to serialize %s event: %w", eventType, err)
		return evt
	}
	if len(payloadJSON) > MaxEventPayloadSize {
		evt.err = fmt.Errorf("%s event payload exceeds maximum size: %d bytes (limit: %d)",
			eventType, len(payloadJSON), MaxEventPayloadSize)
		return evt
	}
	envelope, err := json.Marshal(GatewayEventEnvelope{
		Type:          eventType,
		Payload:       json.RawMessage(payloadJSON),
		Timestamp:     now.Format(time.RFC3339),
		CorrelationID: uuid.New().String(),
	})
	if err != nil {
		evt.err = fmt.Errorf("failed to marshal %s event: %w", eventType, err)
		return evt
	}
	evt.EventData = string(envelope)
	return evt
}

// Err is why the event couldn't be built; publishing it returns this error.
func (e Event) Err() error {
	return e.err
}
