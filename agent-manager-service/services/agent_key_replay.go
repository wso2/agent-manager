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
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// replayAgentKeys re-sends the agent's usable API keys to the gateway that has just started serving it. Each replay
// writes new events: the hub generates the event id, so a repeated ack is not deduplicated.
func replayAgentKeys(
	ctx context.Context, tx *gorm.DB, keys repositories.APIKeyRepository, events *GatewayEventsService,
	p models.Placement, now time.Time,
) error {
	active, err := keys.ListActiveByArtifactTx(ctx, tx, p.ArtifactUUID, now)
	if err != nil {
		return fmt.Errorf("failed to list API keys of artifact %s: %w", p.ArtifactUUID, err)
	}
	for _, key := range active {
		if err := events.PublishAPIKeyCreatedTx(ctx, tx, p.GatewayUUID.String(), apiKeyCreatedEventOf(key)); err != nil {
			return err
		}
	}
	return nil
}

func apiKeyCreatedEventOf(k models.StoredAPIKey) *models.APIKeyCreatedEvent {
	event := &models.APIKeyCreatedEvent{
		UUID:         k.UUID.String(),
		APIID:        k.ArtifactUUID.String(),
		Name:         k.Name,
		DisplayName:  k.DisplayName,
		ApiKeyHashes: apiKeyHashesJSON(k.APIKeyHash),
		MaskedApiKey: k.MaskedAPIKey,
		Operations:   "[\"*\"]",
		CreatedAt:    k.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    k.UpdatedAt.Format(time.RFC3339),
	}
	if k.ExpiresAt != nil {
		expiresAt := k.ExpiresAt.Format(time.RFC3339)
		event.ExpiresAt = &expiresAt
	}
	return event
}
