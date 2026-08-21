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
	"errors"
	"fmt"
	"log/slog"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// AIApplicationService manages AI application records and broadcasts application-key binding
// events to the gateway so the policy engine can enforce per-consumer rate limits.
//
// One AIApplication is created per agent per environment. All LLM configs (proxies) for the
// same agent+env share the same application; each proxy API key is bound to it.
type AIApplicationService struct {
	appRepo        repositories.AIApplicationRepository
	gatewayRepo    repositories.GatewayRepository
	gatewayService *GatewayEventsService
	logger         *slog.Logger
}

// NewAIApplicationService creates a new AIApplicationService.
func NewAIApplicationService(
	appRepo repositories.AIApplicationRepository,
	gatewayRepo repositories.GatewayRepository,
	gatewayService *GatewayEventsService,
	logger *slog.Logger,
) *AIApplicationService {
	return &AIApplicationService{
		appRepo:        appRepo,
		gatewayRepo:    gatewayRepo,
		gatewayService: gatewayService,
		logger:         logger,
	}
}

// EnsureAndBind upserts the AIApplication for the given agent+environment and binds
// the provided proxy API key to it. Returns the application and a boolean indicating
// whether the application was newly created (true) or already existed (false).
//
// When a new application is created the application.updated event is broadcast to all
// gateways in the org. Broadcast failures are logged but do not fail the call — the
// gateway will pick up the application via the bulk-sync endpoint on reconnect.
func (s *AIApplicationService) EnsureAndBind(
	ctx context.Context,
	ouID, projectName, agentID, envName string,
	appHandle, appName, apiKeyUUID string,
) (*models.AIApplication, bool, error) {
	app := &models.AIApplication{
		Handle:          appHandle,
		Name:            appName,
		AgentID:         agentID,
		ProjectName:     projectName,
		EnvironmentName: envName,
		OUID:            ouID,
	}

	created, err := s.appRepo.Create(ctx, nil, app)
	if err != nil {
		return nil, false, fmt.Errorf("failed to create AI application: %w", err)
	}

	// ON CONFLICT DO NOTHING fired — re-fetch the existing record.
	if !created {
		existing, err := s.appRepo.GetByAgentEnv(ctx, ouID, projectName, agentID, envName)
		if err != nil {
			return nil, false, fmt.Errorf("failed to fetch existing AI application: %w", err)
		}
		app = existing
	}

	s.broadcastToAllGateways(ctx, ouID, app, apiKeyUUID)

	s.logger.Info(
		"Ensured AI application and bound API key",
		"application_uuid", app.UUID,
		"application_handle", appHandle,
		"agent_id", agentID,
		"env_name", envName,
		"ou_id", ouID,
		"newly_created", created,
	)
	return app, created, nil
}

// Delete removes the AIApplication record for a given agent+environment.
// Called when the last LLM config for that agent+env is deleted.
// Broadcasts application.updated with empty mappings so the gateway stops enforcing
// per-consumer rate limits for this application immediately.
func (s *AIApplicationService) Delete(ctx context.Context, ouID, projectName, agentID, envName string) error {
	app, err := s.appRepo.GetByAgentEnv(ctx, ouID, projectName, agentID, envName)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("failed to fetch AI application before delete: %w", err)
	}

	if err := s.appRepo.DeleteByAgentEnv(ctx, nil, ouID, projectName, agentID, envName); err != nil {
		return fmt.Errorf("failed to delete AI application: %w", err)
	}

	s.broadcastDeletionToAllGateways(ctx, ouID, app)

	s.logger.Info(
		"Deleted AI application",
		"application_uuid", app.UUID,
		"application_handle", app.Handle,
		"agent_id", agentID,
		"project_name", projectName,
		"env_name", envName,
		"ou_id", ouID,
	)
	return nil
}

// DeleteAllByAgent removes all AIApplication records for the given org+project+agent and
// broadcasts deletion to all gateways so they revoke the applications immediately without
// waiting for a reconnect-triggered bulk-sync.
func (s *AIApplicationService) DeleteAllByAgent(ctx context.Context, ouID, projectName, agentID string) error {
	apps, err := s.appRepo.ListByAgent(ctx, ouID, projectName, agentID)
	if err != nil {
		return fmt.Errorf("failed to list AI applications for agent %q in org %q: %w", agentID, ouID, err)
	}

	if err := s.appRepo.DeleteByAgent(ctx, ouID, projectName, agentID); err != nil {
		return fmt.Errorf("failed to delete AI applications for agent %q in org %q: %w", agentID, ouID, err)
	}

	for i := range apps {
		s.broadcastDeletionToAllGateways(ctx, ouID, &apps[i])
	}

	s.logger.Info("Deleted all AI applications for agent", "agent_id", agentID, "project_name", projectName, "ou_id", ouID, "count", len(apps))
	return nil
}

// broadcastDeletionToAllGateways sends an application.updated event with empty mappings to every
// gateway in the org. The gateway interprets empty mappings as a full revocation of the application,
// stopping per-consumer rate limit enforcement immediately without requiring a reconnect.
func (s *AIApplicationService) broadcastDeletionToAllGateways(ctx context.Context, ouID string, app *models.AIApplication) {
	gateways, err := s.gatewayRepo.GetByOrganizationID(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to list gateways for application deletion broadcast; gateway will sync on reconnect",
			"ou_id", ouID, "application_uuid", app.UUID, "error", err)
		return
	}

	event := &models.ApplicationUpdatedEvent{
		ApplicationID:   app.Handle,
		ApplicationUUID: app.UUID.String(),
		ApplicationName: app.Name,
		ApplicationType: "ai-agent",
		Mappings:        []models.ApplicationAPIKeyMapping{},
	}

	for _, gw := range gateways {
		if err := s.gatewayService.BroadcastApplicationUpdatedEvent(gw.UUID.String(), event); err != nil {
			s.logger.Warn("Failed to broadcast application deletion to gateway; will sync on reconnect",
				"gateway_id", gw.UUID, "application_uuid", app.UUID, "error", err)
		}
	}
}

// broadcastToAllGateways sends an application.updated event to every gateway in the org.
// Errors are logged but not returned so a broadcast failure never blocks the main flow.
func (s *AIApplicationService) broadcastToAllGateways(ctx context.Context, ouID string, app *models.AIApplication, apiKeyUUID string) {
	gateways, err := s.gatewayRepo.GetByOrganizationID(ctx, ouID)
	if err != nil {
		s.logger.Warn("Failed to list gateways for application broadcast; gateway will sync on reconnect",
			"ou_id", ouID, "application_uuid", app.UUID, "error", err)
		return
	}

	event := &models.ApplicationUpdatedEvent{
		ApplicationID:   app.Handle,
		ApplicationUUID: app.UUID.String(),
		ApplicationName: app.Name,
		ApplicationType: "ai-agent",
		Mappings: []models.ApplicationAPIKeyMapping{
			{APIKeyUUID: apiKeyUUID},
		},
	}

	for _, gw := range gateways {
		if err := s.gatewayService.BroadcastApplicationUpdatedEvent(gw.UUID.String(), event); err != nil {
			s.logger.Warn("Failed to broadcast application.updated to gateway; will sync on reconnect",
				"gateway_id", gw.UUID, "application_uuid", app.UUID, "error", err)
		}
	}
}
