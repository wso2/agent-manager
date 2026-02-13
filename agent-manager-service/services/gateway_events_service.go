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
	"log"
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

// BroadcastDeploymentEvent sends an API deployment event to target gateway
func (s *GatewayEventsService) BroadcastDeploymentEvent(gatewayID string, deployment *DeploymentEvent) error {
	correlationID := uuid.New().String()

	// Serialize payload
	payloadJSON, err := json.Marshal(deployment)
	if err != nil {
		log.Printf("[ERROR] Failed to serialize deployment event: gatewayID=%s error=%v", gatewayID, err)
		return fmt.Errorf("failed to serialize deployment event: %w", err)
	}

	// Validate payload size
	if len(payloadJSON) > MaxEventPayloadSize {
		err := fmt.Errorf("event payload exceeds maximum size: %d bytes (limit: %d bytes)", len(payloadJSON), MaxEventPayloadSize)
		log.Printf("[ERROR] Payload size validation failed: gatewayID=%s size=%d error=%v", gatewayID, len(payloadJSON), err)
		return err
	}

	// Create gateway event DTO
	eventDTO := GatewayEventDTO{
		Type:          "api.deployed",
		Payload:       deployment,
		Timestamp:     time.Now().Format(time.RFC3339),
		CorrelationID: correlationID,
	}

	// Serialize complete event
	eventJSON, err := json.Marshal(eventDTO)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal event DTO: gatewayID=%s correlationId=%s error=%v", gatewayID, correlationID, err)
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Get all connections for this gateway
	if s.manager == nil {
		log.Printf("[WARN] WebSocket manager not initialized")
		return nil
	}

	connections := s.manager.GetConnections(gatewayID)
	if len(connections) == 0 {
		log.Printf("[WARN] No active connections for gateway: gatewayID=%s correlationId=%s", gatewayID, correlationID)
		return fmt.Errorf("no active connections for gateway: %s", gatewayID)
	}

	// Broadcast to all connections
	successCount := 0
	failureCount := 0
	var lastError error

	for _, conn := range connections {
		err := conn.Send(eventJSON)
		if err != nil {
			failureCount++
			lastError = err
			log.Printf("[ERROR] Failed to send deployment event: gatewayID=%s connectionID=%s correlationId=%s error=%v",
				gatewayID, conn.ConnectionID, correlationID, err)
			conn.DeliveryStats.IncrementFailed(fmt.Sprintf("send error: %v", err))
		} else {
			successCount++
			log.Printf("[INFO] Deployment event sent: gatewayID=%s connectionID=%s correlationId=%s type=%s",
				gatewayID, conn.ConnectionID, correlationID, eventDTO.Type)
			conn.DeliveryStats.IncrementTotalSent()
		}
	}

	log.Printf("[INFO] Broadcast summary: gatewayID=%s correlationId=%s total=%d success=%d failed=%d",
		gatewayID, correlationID, len(connections), successCount, failureCount)

	if successCount == 0 {
		return fmt.Errorf("failed to deliver event to any connection: %w", lastError)
	}

	return nil
}

// BroadcastUndeploymentEvent sends an API undeployment event to target gateway
func (s *GatewayEventsService) BroadcastUndeploymentEvent(gatewayID string, undeployment *APIUndeploymentEvent) error {
	correlationID := uuid.New().String()

	payloadJSON, err := json.Marshal(undeployment)
	if err != nil {
		log.Printf("[ERROR] Failed to serialize undeployment event: gatewayID=%s error=%v", gatewayID, err)
		return fmt.Errorf("failed to serialize undeployment event: %w", err)
	}

	if len(payloadJSON) > MaxEventPayloadSize {
		err := fmt.Errorf("event payload exceeds maximum size: %d bytes (limit: %d bytes)", len(payloadJSON), MaxEventPayloadSize)
		log.Printf("[ERROR] Payload size validation failed: gatewayID=%s size=%d error=%v", gatewayID, len(payloadJSON), err)
		return err
	}

	eventDTO := GatewayEventDTO{
		Type:          "api.undeployed",
		Payload:       undeployment,
		Timestamp:     time.Now().Format(time.RFC3339),
		CorrelationID: correlationID,
	}

	eventJSON, err := json.Marshal(eventDTO)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal event DTO: gatewayID=%s correlationId=%s error=%v", gatewayID, correlationID, err)
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if s.manager == nil {
		log.Printf("[WARN] WebSocket manager not initialized")
		return nil
	}

	connections := s.manager.GetConnections(gatewayID)
	if len(connections) == 0 {
		log.Printf("[WARN] No active connections for gateway: gatewayID=%s correlationId=%s", gatewayID, correlationID)
		return fmt.Errorf("no active connections for gateway: %s", gatewayID)
	}

	successCount := 0
	failureCount := 0
	var lastError error

	for _, conn := range connections {
		err := conn.Send(eventJSON)
		if err != nil {
			failureCount++
			lastError = err
			log.Printf("[ERROR] Failed to send undeployment event: gatewayID=%s connectionID=%s correlationId=%s error=%v",
				gatewayID, conn.ConnectionID, correlationID, err)
			conn.DeliveryStats.IncrementFailed(fmt.Sprintf("send error: %v", err))
		} else {
			successCount++
			log.Printf("[INFO] Undeployment event sent: gatewayID=%s connectionID=%s correlationId=%s type=%s",
				gatewayID, conn.ConnectionID, correlationID, eventDTO.Type)
			conn.DeliveryStats.IncrementTotalSent()
		}
	}

	log.Printf("[INFO] Undeployment broadcast summary: gatewayID=%s correlationId=%s total=%d success=%d failed=%d",
		gatewayID, correlationID, len(connections), successCount, failureCount)

	if successCount == 0 {
		return fmt.Errorf("failed to deliver undeployment event to any connection: %w", lastError)
	}

	return nil
}

// BroadcastLLMProviderDeploymentEvent sends an LLM provider deployment event
func (s *GatewayEventsService) BroadcastLLMProviderDeploymentEvent(gatewayID string, deployment *models.LLMProviderDeploymentEvent) error {
	correlationID := uuid.New().String()

	payloadJSON, err := json.Marshal(deployment)
	if err != nil {
		log.Printf("[ERROR] Failed to serialize LLM provider deployment event: gatewayID=%s error=%v", gatewayID, err)
		return fmt.Errorf("failed to serialize LLM provider deployment event: %w", err)
	}

	if len(payloadJSON) > MaxEventPayloadSize {
		err := fmt.Errorf("event payload exceeds maximum size: %d bytes (limit: %d bytes)", len(payloadJSON), MaxEventPayloadSize)
		log.Printf("[ERROR] Payload size validation failed: gatewayID=%s size=%d error=%v", gatewayID, len(payloadJSON), err)
		return err
	}

	eventDTO := GatewayEventDTO{
		Type:          "llmprovider.deployed",
		Payload:       deployment,
		Timestamp:     time.Now().Format(time.RFC3339),
		CorrelationID: correlationID,
	}

	eventJSON, err := json.Marshal(eventDTO)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal event DTO: gatewayID=%s correlationId=%s error=%v", gatewayID, correlationID, err)
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if s.manager == nil {
		log.Printf("[WARN] WebSocket manager not initialized")
		return nil
	}

	connections := s.manager.GetConnections(gatewayID)
	if len(connections) == 0 {
		log.Printf("[WARN] No active connections for gateway: gatewayID=%s correlationId=%s", gatewayID, correlationID)
		return fmt.Errorf("no active connections for gateway: %s", gatewayID)
	}

	successCount := 0
	failureCount := 0
	var lastError error

	for _, conn := range connections {
		err := conn.Send(eventJSON)
		if err != nil {
			failureCount++
			lastError = err
			log.Printf("[ERROR] Failed to send LLM provider deployment event: gatewayID=%s connectionID=%s correlationId=%s error=%v",
				gatewayID, conn.ConnectionID, correlationID, err)
			conn.DeliveryStats.IncrementFailed(fmt.Sprintf("send error: %v", err))
		} else {
			successCount++
			log.Printf("[INFO] LLM provider deployment event sent: gatewayID=%s connectionID=%s correlationId=%s type=%s",
				gatewayID, conn.ConnectionID, correlationID, eventDTO.Type)
			conn.DeliveryStats.IncrementTotalSent()
		}
	}

	log.Printf("[INFO] LLM provider deployment broadcast summary: gatewayID=%s correlationId=%s total=%d success=%d failed=%d",
		gatewayID, correlationID, len(connections), successCount, failureCount)

	if successCount == 0 {
		return fmt.Errorf("failed to deliver LLM provider deployment event to any connection: %w", lastError)
	}

	return nil
}

// BroadcastLLMProviderUndeploymentEvent sends an LLM provider undeployment event
func (s *GatewayEventsService) BroadcastLLMProviderUndeploymentEvent(gatewayID string, undeployment *models.LLMProviderUndeploymentEvent) error {
	correlationID := uuid.New().String()

	payloadJSON, err := json.Marshal(undeployment)
	if err != nil {
		log.Printf("[ERROR] Failed to serialize LLM provider undeployment event: gatewayID=%s error=%v", gatewayID, err)
		return fmt.Errorf("failed to serialize LLM provider undeployment event: %w", err)
	}

	if len(payloadJSON) > MaxEventPayloadSize {
		err := fmt.Errorf("event payload exceeds maximum size: %d bytes (limit: %d bytes)", len(payloadJSON), MaxEventPayloadSize)
		log.Printf("[ERROR] Payload size validation failed: gatewayID=%s size=%d error=%v", gatewayID, len(payloadJSON), err)
		return err
	}

	eventDTO := GatewayEventDTO{
		Type:          "llmprovider.undeployed",
		Payload:       undeployment,
		Timestamp:     time.Now().Format(time.RFC3339),
		CorrelationID: correlationID,
	}

	eventJSON, err := json.Marshal(eventDTO)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal event DTO: gatewayID=%s correlationId=%s error=%v", gatewayID, correlationID, err)
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if s.manager == nil {
		log.Printf("[WARN] WebSocket manager not initialized")
		return nil
	}

	connections := s.manager.GetConnections(gatewayID)
	if len(connections) == 0 {
		log.Printf("[WARN] No active connections for gateway: gatewayID=%s correlationId=%s", gatewayID, correlationID)
		return fmt.Errorf("no active connections for gateway: %s", gatewayID)
	}

	successCount := 0
	failureCount := 0
	var lastError error

	for _, conn := range connections {
		err := conn.Send(eventJSON)
		if err != nil {
			failureCount++
			lastError = err
			log.Printf("[ERROR] Failed to send LLM provider undeployment event: gatewayID=%s connectionID=%s correlationId=%s error=%v",
				gatewayID, conn.ConnectionID, correlationID, err)
			conn.DeliveryStats.IncrementFailed(fmt.Sprintf("send error: %v", err))
		} else {
			successCount++
			log.Printf("[INFO] LLM provider undeployment event sent: gatewayID=%s connectionID=%s correlationId=%s type=%s",
				gatewayID, conn.ConnectionID, correlationID, eventDTO.Type)
			conn.DeliveryStats.IncrementTotalSent()
		}
	}

	log.Printf("[INFO] LLM provider undeployment broadcast summary: gatewayID=%s correlationId=%s total=%d success=%d failed=%d",
		gatewayID, correlationID, len(connections), successCount, failureCount)

	if successCount == 0 {
		return fmt.Errorf("failed to deliver LLM provider undeployment event to any connection: %w", lastError)
	}

	return nil
}

// BroadcastLLMProxyDeploymentEvent sends an LLM proxy deployment event
func (s *GatewayEventsService) BroadcastLLMProxyDeploymentEvent(gatewayID string, deployment *models.LLMProxyDeploymentEvent) error {
	correlationID := uuid.New().String()

	payloadJSON, err := json.Marshal(deployment)
	if err != nil {
		log.Printf("[ERROR] Failed to serialize LLM proxy deployment event: gatewayID=%s error=%v", gatewayID, err)
		return fmt.Errorf("failed to serialize LLM proxy deployment event: %w", err)
	}

	if len(payloadJSON) > MaxEventPayloadSize {
		err := fmt.Errorf("event payload exceeds maximum size: %d bytes (limit: %d bytes)", len(payloadJSON), MaxEventPayloadSize)
		log.Printf("[ERROR] Payload size validation failed: gatewayID=%s size=%d error=%v", gatewayID, len(payloadJSON), err)
		return err
	}

	eventDTO := GatewayEventDTO{
		Type:          "llmproxy.deployed",
		Payload:       deployment,
		Timestamp:     time.Now().Format(time.RFC3339),
		CorrelationID: correlationID,
	}

	eventJSON, err := json.Marshal(eventDTO)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal event DTO: gatewayID=%s correlationId=%s error=%v", gatewayID, correlationID, err)
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if s.manager == nil {
		log.Printf("[WARN] WebSocket manager not initialized")
		return nil
	}

	connections := s.manager.GetConnections(gatewayID)
	if len(connections) == 0 {
		log.Printf("[WARN] No active connections for gateway: gatewayID=%s correlationId=%s", gatewayID, correlationID)
		return fmt.Errorf("no active connections for gateway: %s", gatewayID)
	}

	successCount := 0
	failureCount := 0
	var lastError error

	for _, conn := range connections {
		err := conn.Send(eventJSON)
		if err != nil {
			failureCount++
			lastError = err
			log.Printf("[ERROR] Failed to send LLM proxy deployment event: gatewayID=%s connectionID=%s correlationId=%s error=%v",
				gatewayID, conn.ConnectionID, correlationID, err)
			conn.DeliveryStats.IncrementFailed(fmt.Sprintf("send error: %v", err))
		} else {
			successCount++
			log.Printf("[INFO] LLM proxy deployment event sent: gatewayID=%s connectionID=%s correlationId=%s type=%s",
				gatewayID, conn.ConnectionID, correlationID, eventDTO.Type)
			conn.DeliveryStats.IncrementTotalSent()
		}
	}

	log.Printf("[INFO] LLM proxy deployment broadcast summary: gatewayID=%s correlationId=%s total=%d success=%d failed=%d",
		gatewayID, correlationID, len(connections), successCount, failureCount)

	if successCount == 0 {
		return fmt.Errorf("failed to deliver LLM proxy deployment event to any connection: %w", lastError)
	}

	return nil
}

// BroadcastLLMProxyUndeploymentEvent sends an LLM proxy undeployment event
func (s *GatewayEventsService) BroadcastLLMProxyUndeploymentEvent(gatewayID string, undeployment *models.LLMProxyUndeploymentEvent) error {
	correlationID := uuid.New().String()

	payloadJSON, err := json.Marshal(undeployment)
	if err != nil {
		log.Printf("[ERROR] Failed to serialize LLM proxy undeployment event: gatewayID=%s error=%v", gatewayID, err)
		return fmt.Errorf("failed to serialize LLM proxy undeployment event: %w", err)
	}

	if len(payloadJSON) > MaxEventPayloadSize {
		err := fmt.Errorf("event payload exceeds maximum size: %d bytes (limit: %d bytes)", len(payloadJSON), MaxEventPayloadSize)
		log.Printf("[ERROR] Payload size validation failed: gatewayID=%s size=%d error=%v", gatewayID, len(payloadJSON), err)
		return err
	}

	eventDTO := GatewayEventDTO{
		Type:          "llmproxy.undeployed",
		Payload:       undeployment,
		Timestamp:     time.Now().Format(time.RFC3339),
		CorrelationID: correlationID,
	}

	eventJSON, err := json.Marshal(eventDTO)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal event DTO: gatewayID=%s correlationId=%s error=%v", gatewayID, correlationID, err)
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if s.manager == nil {
		log.Printf("[WARN] WebSocket manager not initialized")
		return nil
	}

	connections := s.manager.GetConnections(gatewayID)
	if len(connections) == 0 {
		log.Printf("[WARN] No active connections for gateway: gatewayID=%s correlationId=%s", gatewayID, correlationID)
		return fmt.Errorf("no active connections for gateway: %s", gatewayID)
	}

	successCount := 0
	failureCount := 0
	var lastError error

	for _, conn := range connections {
		err := conn.Send(eventJSON)
		if err != nil {
			failureCount++
			lastError = err
			log.Printf("[ERROR] Failed to send LLM proxy undeployment event: gatewayID=%s connectionID=%s correlationId=%s error=%v",
				gatewayID, conn.ConnectionID, correlationID, err)
			conn.DeliveryStats.IncrementFailed(fmt.Sprintf("send error: %v", err))
		} else {
			successCount++
			log.Printf("[INFO] LLM proxy undeployment event sent: gatewayID=%s connectionID=%s correlationId=%s type=%s",
				gatewayID, conn.ConnectionID, correlationID, eventDTO.Type)
			conn.DeliveryStats.IncrementTotalSent()
		}
	}

	log.Printf("[INFO] LLM proxy undeployment broadcast summary: gatewayID=%s correlationId=%s total=%d success=%d failed=%d",
		gatewayID, correlationID, len(connections), successCount, failureCount)

	if successCount == 0 {
		return fmt.Errorf("failed to deliver LLM proxy undeployment event to any connection: %w", lastError)
	}

	return nil
}

// BroadcastAPIKeyCreatedEvent sends an API key created event to target gateway
func (s *GatewayEventsService) BroadcastAPIKeyCreatedEvent(gatewayID string, event *models.APIKeyCreatedEvent) error {
	correlationID := uuid.New().String()

	payloadJSON, err := json.Marshal(event)
	if err != nil {
		log.Printf("[ERROR] Failed to serialize API key created event: gatewayID=%s error=%v", gatewayID, err)
		return fmt.Errorf("failed to serialize API key created event: %w", err)
	}

	if len(payloadJSON) > MaxEventPayloadSize {
		err := fmt.Errorf("event payload exceeds maximum size: %d bytes (limit: %d bytes)", len(payloadJSON), MaxEventPayloadSize)
		log.Printf("[ERROR] Payload size validation failed: gatewayID=%s size=%d error=%v", gatewayID, len(payloadJSON), err)
		return err
	}

	eventDTO := GatewayEventDTO{
		Type:          "apikey.created",
		Payload:       event,
		Timestamp:     time.Now().Format(time.RFC3339),
		CorrelationID: correlationID,
	}

	eventJSON, err := json.Marshal(eventDTO)
	if err != nil {
		log.Printf("[ERROR] Failed to marshal event DTO: gatewayID=%s correlationId=%s error=%v", gatewayID, correlationID, err)
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if s.manager == nil {
		log.Printf("[WARN] WebSocket manager not initialized")
		return nil
	}

	connections := s.manager.GetConnections(gatewayID)
	if len(connections) == 0 {
		log.Printf("[WARN] No active connections for gateway: gatewayID=%s correlationId=%s", gatewayID, correlationID)
		return fmt.Errorf("no active connections for gateway: %s", gatewayID)
	}

	successCount := 0
	failureCount := 0
	var lastError error

	for _, conn := range connections {
		err := conn.Send(eventJSON)
		if err != nil {
			failureCount++
			lastError = err
			log.Printf("[ERROR] Failed to send API key created event: gatewayID=%s connectionID=%s correlationId=%s error=%v",
				gatewayID, conn.ConnectionID, correlationID, err)
			conn.DeliveryStats.IncrementFailed(fmt.Sprintf("send error: %v", err))
		} else {
			successCount++
			log.Printf("[INFO] API key created event sent: gatewayID=%s connectionID=%s correlationId=%s type=%s",
				gatewayID, conn.ConnectionID, correlationID, eventDTO.Type)
			conn.DeliveryStats.IncrementTotalSent()
		}
	}

	log.Printf("[INFO] API key created broadcast summary: gatewayID=%s correlationId=%s total=%d success=%d failed=%d",
		gatewayID, correlationID, len(connections), successCount, failureCount)

	if successCount == 0 {
		return fmt.Errorf("failed to deliver API key created event to any connection: %w", lastError)
	}

	return nil
}
