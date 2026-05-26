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
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

const (
	// MaxEventPayloadSize is the maximum allowed event payload size (1MB)
	MaxEventPayloadSize = 1024 * 1024
)

// GatewayEventsService handles broadcasting events to connected gateways.
// It publishes events to the EventHub so that any service instance holding
// the gateway's WebSocket connection will deliver the event.
type GatewayEventsService struct {
	hub eventhub.EventHub
}

// GatewayEventDTO represents a gateway event
type GatewayEventDTO struct {
	Type          string      `json:"type"`
	Payload       interface{} `json:"payload"`
	Timestamp     string      `json:"timestamp"`
	CorrelationID string      `json:"correlationId"`
	UserId        string      `json:"userId,omitempty"`
}

// NewGatewayEventsService creates a new gateway events service
func NewGatewayEventsService(hub eventhub.EventHub) *GatewayEventsService {
	return &GatewayEventsService{hub: hub}
}

// DeploymentEvent represents an API deployment event (TODO: move to models package)
type DeploymentEvent struct {
	APIID        string `json:"apiId"`
	DeploymentID string `json:"deploymentId"`
	GatewayID    string `json:"gatewayId"`
}

// APIUndeploymentEvent represents an API undeployment event (TODO: move to models package)
type APIUndeploymentEvent struct {
	APIID        string `json:"apiId"`
	DeploymentID string `json:"deploymentId"`
	GatewayID    string `json:"gatewayId"`
}

func (s *GatewayEventsService) broadcastEvent(gatewayID string, eventType string, action string, entityID string, payload interface{}) error {
	correlationID := uuid.New().String()

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		slog.Error("Failed to serialize event", "gatewayID", gatewayID, "type", eventType, "error", err)
		return fmt.Errorf("failed to serialize %s event: %w", eventType, err)
	}

	if len(payloadJSON) > MaxEventPayloadSize {
		return fmt.Errorf("event payload exceeds maximum size: %d bytes (limit: %d)", len(payloadJSON), MaxEventPayloadSize)
	}

	eventDTO := GatewayEventDTO{
		Type:          eventType,
		Payload:       payload,
		Timestamp:     time.Now().Format(time.RFC3339),
		CorrelationID: correlationID,
	}

	eventJSON, err := json.Marshal(eventDTO)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	evt := eventhub.Event{
		GatewayID:           gatewayID,
		OriginatedTimestamp: time.Now(),
		EventType:           eventhub.EventType(eventType),
		Action:              action,
		EntityID:            entityID,
		EventData:           string(eventJSON),
	}

	if err := s.hub.PublishEvent(gatewayID, evt); err != nil {
		slog.Error("Failed to publish event to EventHub",
			"gatewayID", gatewayID, "type", eventType, "correlationID", correlationID, "error", err)
		return fmt.Errorf("failed to publish %s event: %w", eventType, err)
	}

	slog.Debug("Event published to EventHub",
		"gatewayID", gatewayID, "type", eventType, "correlationID", correlationID)
	return nil
}

// Public methods become thin one-liners:
func (s *GatewayEventsService) BroadcastDeploymentEvent(gatewayID string, event *DeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "api.deployed", "CREATE", event.APIID, event)
}

func (s *GatewayEventsService) BroadcastUndeploymentEvent(gatewayID string, event *APIUndeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "api.undeployed", "DELETE", event.APIID, event)
}

func (s *GatewayEventsService) BroadcastLLMProviderDeploymentEvent(gatewayID string, event *models.LLMProviderDeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "llmprovider.deployed", "CREATE", event.ProviderID, event)
}

func (s *GatewayEventsService) BroadcastLLMProviderUndeploymentEvent(gatewayID string, event *models.LLMProviderUndeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "llmprovider.undeployed", "DELETE", event.ProviderID, event)
}

func (s *GatewayEventsService) BroadcastLLMProxyDeploymentEvent(gatewayID string, event *models.LLMProxyDeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "llmproxy.deployed", "CREATE", event.ProxyID, event)
}

func (s *GatewayEventsService) BroadcastLLMProxyUndeploymentEvent(gatewayID string, event *models.LLMProxyUndeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "llmproxy.undeployed", "DELETE", event.ProxyID, event)
}

// API key events use a unique ID per event — they are not deduplicated on replay
// because the gateway bulk-syncs API keys on reconnect via the catalog endpoint.
func (s *GatewayEventsService) BroadcastAPIKeyCreatedEvent(gatewayID string, event *models.APIKeyCreatedEvent) error {
	return s.broadcastEvent(gatewayID, "apikey.created", "PROVISION", event.UUID, event)
}

func (s *GatewayEventsService) BroadcastAPIKeyRevokedEvent(gatewayID string, event *models.APIKeyRevokedEvent) error {
	return s.broadcastEvent(gatewayID, "apikey.revoked", "REVOKE", uuid.New().String(), event)
}

func (s *GatewayEventsService) BroadcastAPIKeyUpdatedEvent(gatewayID string, event *models.APIKeyUpdatedEvent) error {
	return s.broadcastEvent(gatewayID, "apikey.updated", "UPDATE", uuid.New().String(), event)
}
