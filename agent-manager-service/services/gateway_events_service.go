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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/websocket"
)

const (
	// Maximum event payload size (1MB)
	MaxEventPayloadSize = 1024 * 1024
)

// GatewayEventsService handles broadcasting events to connected gateways
type GatewayEventsService struct {
	manager *websocket.Manager
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
func NewGatewayEventsService(manager *websocket.Manager) *GatewayEventsService {
	return &GatewayEventsService{
		manager: manager,
	}
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

func (s *GatewayEventsService) broadcastEvent(gatewayID string, eventType string, payload interface{}) error {
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

	if s.manager == nil {
		slog.Warn("WebSocket manager not initialized")
		return nil
	}

	connections := s.manager.GetConnections(gatewayID)
	if len(connections) == 0 {
		return fmt.Errorf("no active connections for gateway: %s", gatewayID)
	}

	successCount, failureCount := 0, 0
	var lastError error
	for _, conn := range connections {
		if err := conn.Send(eventJSON); err != nil {
			failureCount++
			lastError = err
			slog.Error("Failed to send event", "gatewayID", gatewayID, "connectionID", conn.ConnectionID,
				"correlationID", correlationID, "type", eventType, "error", err)
			conn.DeliveryStats.IncrementFailed(fmt.Sprintf("send error: %v", err))
		} else {
			successCount++
			conn.DeliveryStats.IncrementTotalSent()
		}
	}

	slog.Info("Broadcast summary", "gatewayID", gatewayID, "correlationID", correlationID,
		"type", eventType, "total", len(connections), "success", successCount, "failed", failureCount)

	if successCount == 0 {
		return fmt.Errorf("failed to deliver %s event to any connection: %w", eventType, lastError)
	}
	return nil
}

// Public methods become thin one-liners:
func (s *GatewayEventsService) BroadcastDeploymentEvent(gatewayID string, event *DeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "api.deployed", event)
}

func (s *GatewayEventsService) BroadcastUndeploymentEvent(gatewayID string, event *APIUndeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "api.undeployed", event)
}

func (s *GatewayEventsService) BroadcastLLMProviderDeploymentEvent(gatewayID string, event *models.LLMProviderDeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "llmprovider.deployed", event)
}

func (s *GatewayEventsService) BroadcastLLMProviderUndeploymentEvent(gatewayID string, event *models.LLMProviderUndeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "llmprovider.undeployed", event)
}

func (s *GatewayEventsService) BroadcastLLMProxyDeploymentEvent(gatewayID string, event *models.LLMProxyDeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "llmproxy.deployed", event)
}

func (s *GatewayEventsService) BroadcastLLMProxyUndeploymentEvent(gatewayID string, event *models.LLMProxyUndeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "llmproxy.undeployed", event)
}

func (s *GatewayEventsService) BroadcastAPIKeyCreatedEvent(gatewayID string, event *models.APIKeyCreatedEvent) error {
	return s.broadcastEvent(gatewayID, "apikey.created", event)
}
