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
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/eventhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// GatewayEventsService handles broadcasting events to connected gateways.
// It publishes events to the EventHub so that any service instance holding
// the gateway's WebSocket connection will deliver the event.
type GatewayEventsService struct {
	hub eventhub.EventHub
}

// NewGatewayEventsService creates a new gateway events service
func NewGatewayEventsService(hub eventhub.EventHub) *GatewayEventsService {
	return &GatewayEventsService{hub: hub}
}

func (s *GatewayEventsService) broadcastEvent(gatewayID string, eventType string, action string, entityID string, payload interface{}) error {
	return s.publish(gatewayID, eventType, action, entityID, payload, func(evt eventhub.Event) error {
		return s.hub.PublishEvent(context.Background(), gatewayID, evt)
	})
}

// publish builds the event the one way gateway events are built (eventhub.NewGatewayEvent) and writes it.
func (s *GatewayEventsService) publish(gatewayID string, eventType string, action string, entityID string, payload interface{}, write func(eventhub.Event) error) error {
	evt := eventhub.NewGatewayEvent(eventType, action, entityID, payload)
	evt.GatewayID = gatewayID
	if err := evt.Err(); err != nil {
		return err
	}
	if err := write(evt); err != nil {
		slog.Error("Failed to publish event to EventHub", "gatewayID", gatewayID, "type", eventType, "error", err)
		return fmt.Errorf("failed to publish %s event: %w", eventType, err)
	}
	slog.Debug("Event published to EventHub", "gatewayID", gatewayID, "type", eventType)
	return nil
}

// API key events use a unique ID per event — they are not deduplicated on replay
// because the gateway bulk-syncs API keys on reconnect via the catalog endpoint.
func (s *GatewayEventsService) BroadcastAPIKeyCreatedEvent(gatewayID string, event *models.APIKeyCreatedEvent) error {
	return s.broadcastEvent(gatewayID, "apikey.created", "PROVISION", event.UUID, event)
}

// PublishAPIKeyCreatedTx writes the event in the caller's transaction, so it is delivered only if that commits.
func (s *GatewayEventsService) PublishAPIKeyCreatedTx(ctx context.Context, tx *gorm.DB, gatewayID string, e *models.APIKeyCreatedEvent) error {
	return s.publish(gatewayID, "apikey.created", "PROVISION", e.UUID, e, func(evt eventhub.Event) error {
		return s.hub.PublishEventTx(ctx, tx, gatewayID, evt)
	})
}

func (s *GatewayEventsService) BroadcastAPIKeyRevokedEvent(gatewayID string, event *models.APIKeyRevokedEvent) error {
	return s.broadcastEvent(gatewayID, "apikey.revoked", "REVOKE", uuid.New().String(), event)
}

func (s *GatewayEventsService) BroadcastAPIKeyUpdatedEvent(gatewayID string, event *models.APIKeyUpdatedEvent) error {
	return s.broadcastEvent(gatewayID, "apikey.updated", "UPDATE", uuid.New().String(), event)
}

func (s *GatewayEventsService) BroadcastApplicationUpdatedEvent(gatewayID string, event *models.ApplicationUpdatedEvent) error {
	return s.broadcastEvent(gatewayID, "application.updated", "UPDATE", event.ApplicationUUID, event)
}
