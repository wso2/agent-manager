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

import "time"

// EventType represents the type of event
type EventType string

const (
	EventTypeLLMProvider EventType = "LLM_PROVIDER"
	EventTypeLLMProxy    EventType = "LLM_PROXY"
	EventTypeAPIKey      EventType = "API_KEY"

	// EmptyEventData is the canonical JSON payload for events that carry no extra data.
	EmptyEventData = "{}"
)

// Event represents a change event in the system
type Event struct {
	GatewayID           string    `json:"gateway_id"`
	ProcessedTimestamp  time.Time `json:"processed_timestamp"`
	OriginatedTimestamp time.Time `json:"originated_timestamp"`
	EventType           EventType `json:"event_type"`
	Action              string    `json:"action"`
	EntityID            string    `json:"entity_id"`
	EventID             string    `json:"event_id"`
	// EventData carries optional event-specific details as a JSON string.
	EventData string `json:"event_data"`
}

// GatewayState tracks the version state of a gateway.
type GatewayState struct {
	GatewayID string    `json:"gateway_id"`
	VersionID string    `json:"version_id"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EventHub defines the interface for event publishing and subscribing
type EventHub interface {
	Initialize() error
	RegisterGateway(gatewayID string) error
	PublishEvent(gatewayID string, event Event) error
	Subscribe(gatewayID string) (<-chan Event, error)
	Unsubscribe(gatewayID string, subscriber <-chan Event) error
	UnsubscribeAll(gatewayID string) error
	CleanUpEvents() error
	Close() error
}

// Config holds configuration for the EventHub
type Config struct {
	PollInterval    time.Duration
	CleanupInterval time.Duration
	RetentionPeriod time.Duration
}

// DefaultConfig returns a Config with sensible defaults
func DefaultConfig() Config {
	return Config{
		PollInterval:    3 * time.Second,
		CleanupInterval: 10 * time.Minute,
		RetentionPeriod: 1 * time.Hour,
	}
}
