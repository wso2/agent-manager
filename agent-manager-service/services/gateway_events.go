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
	ws "github.com/wso2/ai-agent-management-platform/agent-manager-service/websocket"
)

const (
	// MaxEventPayloadSize is the maximum size of an event payload in bytes
	MaxEventPayloadSize = 1 * 1024 * 1024 // 1MB
)

// GatewayEventsService handles broadcasting events to gateway WebSocket connections
type GatewayEventsService interface {
	BroadcastAgentDeployed(gatewayID string, deployment *models.AgentDeploymentEvent) error
	BroadcastAgentUndeployed(gatewayID string, undeployment *models.AgentUndeploymentEvent) error
	BroadcastConfigUpdated(gatewayID string, config interface{}) error
	// LLM Provider deployment methods (using api-platform compatible event types)
	BroadcastLLMProviderDeployed(gatewayID string, deployment *models.DeploymentEvent) error
	BroadcastLLMProviderUndeployed(gatewayID string, undeployment *models.APIUndeploymentEvent) error
}

type gatewayEventsService struct {
	manager *ws.Manager
	logger  *slog.Logger
}

// NewGatewayEventsService creates a new gateway events service
func NewGatewayEventsService(manager *ws.Manager, logger *slog.Logger) GatewayEventsService {
	return &gatewayEventsService{
		manager: manager,
		logger:  logger,
	}
}

// BroadcastAgentDeployed broadcasts an agent deployment event to a gateway
func (s *gatewayEventsService) BroadcastAgentDeployed(gatewayID string, deployment *models.AgentDeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "agent.deployed", deployment)
}

// BroadcastAgentUndeployed broadcasts an agent undeployment event to a gateway
func (s *gatewayEventsService) BroadcastAgentUndeployed(gatewayID string, undeployment *models.AgentUndeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "agent.undeployed", undeployment)
}

// BroadcastConfigUpdated broadcasts a configuration update event to a gateway
func (s *gatewayEventsService) BroadcastConfigUpdated(gatewayID string, config interface{}) error {
	return s.broadcastEvent(gatewayID, "config.updated", config)
}

// broadcastEvent broadcasts an event to all active connections for a gateway
func (s *gatewayEventsService) broadcastEvent(gatewayID, eventType string, payload interface{}) error {
	// Marshal payload to JSON to check size
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	if len(payloadJSON) > MaxEventPayloadSize {
		return fmt.Errorf("event payload exceeds maximum size of %d bytes", MaxEventPayloadSize)
	}

	// Create the event DTO
	eventDTO := models.GatewayEventDTO{
		Type:          eventType,
		Payload:       payload,
		Timestamp:     time.Now().Format(time.RFC3339),
		CorrelationID: uuid.New().String(),
	}

	// Marshal the complete event
	eventJSON, err := json.Marshal(eventDTO)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Get connections for this gateway
	connections := s.manager.GetConnections(gatewayID)
	if len(connections) == 0 {
		s.logger.Warn("No active connections for gateway",
			"gatewayId", gatewayID,
			"eventType", eventType,
		)
		return nil
	}

	// Send event to all connections for this gateway
	var sentCount, failedCount int
	for _, conn := range connections {
		if err := conn.Send(eventJSON); err != nil {
			failedCount++
			conn.DeliveryStats.IncrementFailed(fmt.Sprintf("send error: %v", err))
			s.logger.Error("Failed to send event to gateway connection",
				"gatewayId", gatewayID,
				"connectionId", conn.ConnectionID,
				"eventType", eventType,
				"error", err,
			)
		} else {
			sentCount++
			conn.DeliveryStats.IncrementTotalSent()
			s.logger.Debug("Event sent successfully",
				"type", eventType,
				"gatewayId", gatewayID,
				"connectionId", conn.ConnectionID,
				"correlationId", eventDTO.CorrelationID,
			)
		}
	}

	s.logger.Info("Event broadcast completed",
		"eventType", eventType,
		"gatewayId", gatewayID,
		"sentCount", sentCount,
		"failedCount", failedCount,
		"correlationId", eventDTO.CorrelationID,
	)

	return nil
}

// BroadcastToAllGateways broadcasts an event to all connected gateways
func (s *gatewayEventsService) BroadcastToAllGateways(eventType string, payload interface{}) error {
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	if len(payloadJSON) > MaxEventPayloadSize {
		return fmt.Errorf("event payload exceeds maximum size of %d bytes", MaxEventPayloadSize)
	}

	eventDTO := models.GatewayEventDTO{
		Type:          eventType,
		Payload:       payload,
		Timestamp:     time.Now().Format(time.RFC3339),
		CorrelationID: uuid.New().String(),
	}

	eventJSON, err := json.Marshal(eventDTO)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	gatewayIDs := s.manager.GetAllGatewayIDs()
	var totalSent, totalFailed int

	for _, gatewayID := range gatewayIDs {
		connections := s.manager.GetConnections(gatewayID)
		for _, conn := range connections {
			if err := conn.Send(eventJSON); err != nil {
				totalFailed++
				conn.DeliveryStats.IncrementFailed(fmt.Sprintf("send error: %v", err))
				s.logger.Error("Failed to send event to gateway connection",
					"gatewayId", gatewayID,
					"connectionId", conn.ConnectionID,
					"eventType", eventType,
					"error", err,
				)
			} else {
				totalSent++
				conn.DeliveryStats.IncrementTotalSent()
			}
		}
	}

	s.logger.Info("Broadcast to all gateways completed",
		"eventType", eventType,
		"totalGateways", len(gatewayIDs),
		"sentCount", totalSent,
		"failedCount", totalFailed,
		"correlationId", eventDTO.CorrelationID,
	)

	return nil
}

// BroadcastLLMProviderDeployed broadcasts an LLM provider deployment event using api.deployed
// This allows gateways to process both APIs and LLM providers with the same handler
func (s *gatewayEventsService) BroadcastLLMProviderDeployed(gatewayID string, deployment *models.DeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "api.deployed", deployment)
}

// BroadcastLLMProviderUndeployed broadcasts an LLM provider undeployment event using api.undeployed
func (s *gatewayEventsService) BroadcastLLMProviderUndeployed(gatewayID string, undeployment *models.APIUndeploymentEvent) error {
	return s.broadcastEvent(gatewayID, "api.undeployed", undeployment)
}
