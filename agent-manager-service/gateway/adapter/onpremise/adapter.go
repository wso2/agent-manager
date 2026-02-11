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

package onpremise

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/gateway"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
)

// OnPremiseAdapter implements IGatewayAdapter for on-premise deployments
// Uses WebSocket events for provider deployment instead of REST API calls
type OnPremiseAdapter struct {
	db            *gorm.DB
	config        gateway.AdapterConfig
	eventsService services.GatewayEventsService
	logger        *slog.Logger
}

// NewOnPremiseAdapter creates a new on-premise adapter instance
func NewOnPremiseAdapter(
	config gateway.AdapterConfig,
	db *gorm.DB,
	eventsService services.GatewayEventsService,
	logger *slog.Logger,
) (gateway.IGatewayAdapter, error) {
	adapter := &OnPremiseAdapter{
		config:        config,
		db:            db,
		eventsService: eventsService,
		logger:        logger,
	}

	return adapter, nil
}

// GetAdapterType returns the adapter type identifier
func (a *OnPremiseAdapter) GetAdapterType() string {
	return "on-premise"
}

// Close cleans up adapter resources
func (a *OnPremiseAdapter) Close() error {
	return nil
}

// ValidateGatewayEndpoint is a no-op for WebSocket-based adapter
// Gateway connectivity is managed through WebSocket connections
func (a *OnPremiseAdapter) ValidateGatewayEndpoint(ctx context.Context, controlPlaneURL string) error {
	// For WebSocket-based communication, gateway connectivity is managed
	// through the WebSocket connection lifecycle
	// This method is kept for interface compatibility
	return nil
}

// CheckHealth returns gateway health status based on WebSocket connection
func (a *OnPremiseAdapter) CheckHealth(ctx context.Context, controlPlaneURL string) (*gateway.HealthStatus, error) {
	// For WebSocket-based communication, health is determined by active connections
	// This is a simplified health check - in production, you would query the WebSocket manager
	return &gateway.HealthStatus{
		Status:       "ACTIVE",
		ResponseTime: 0,
		CheckedAt:    time.Now(),
	}, nil
}

// DeployProvider deploys an LLM provider to a gateway using WebSocket events
func (a *OnPremiseAdapter) DeployProvider(ctx context.Context, gatewayID string, config *gateway.ProviderDeploymentConfig) (*gateway.ProviderDeploymentResult, error) {
	a.logger.Info("Deploying provider to gateway", "gatewayID", gatewayID, "handle", config.Handle)

	gatewayUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway ID: %w", err)
	}

	// 1. Create provider record in database
	providerUUID := uuid.New()
	provider := &models.LLMProvider{
		UUID:          providerUUID,
		Handle:        config.Handle,
		DisplayName:   config.DisplayName,
		Template:      config.Template,
		Configuration: config.Configuration,
		Status:        "APPROVED",
	}

	if err := a.db.WithContext(ctx).Create(provider).Error; err != nil {
		return nil, fmt.Errorf("failed to create provider: %w", err)
	}

	// 2. Create deployment record with PENDING status
	deployment := &models.ProviderGatewayDeployment{
		ProviderUUID:         providerUUID,
		GatewayUUID:          gatewayUUID,
		DeploymentID:         providerUUID.String(),
		Environment:          "production",
		ConfigurationVersion: 1,
		Status:               "PENDING",
	}

	if err := a.db.WithContext(ctx).Create(deployment).Error; err != nil {
		return nil, fmt.Errorf("failed to create deployment record: %w", err)
	}

	// 3. Broadcast api.deployed event (same as api-platform)
	// Using api.deployed allows gateways to process LLM providers without code changes
	event := &models.DeploymentEvent{
		ApiId:        providerUUID.String(), // apiId is the provider UUID
		DeploymentID: providerUUID.String(),
		Vhost:        "", // Not used for LLM providers
		Environment:  "production",
	}

	if err := a.eventsService.BroadcastLLMProviderDeployed(gatewayID, event); err != nil {
		a.logger.Error("Failed to broadcast deployment event", "error", err)
		return nil, fmt.Errorf("failed to broadcast deployment: %w", err)
	}

	return &gateway.ProviderDeploymentResult{
		DeploymentID: providerUUID.String(),
		Status:       "PENDING",
		DeployedAt:   time.Now(),
	}, nil
}

// UpdateProvider updates an existing LLM provider on a gateway using WebSocket events
func (a *OnPremiseAdapter) UpdateProvider(ctx context.Context, gatewayID string, providerID string, config *gateway.ProviderDeploymentConfig) (*gateway.ProviderDeploymentResult, error) {
	a.logger.Info("Updating provider on gateway", "gatewayID", gatewayID, "providerID", providerID)

	providerUUID, err := uuid.Parse(providerID)
	if err != nil {
		return nil, fmt.Errorf("invalid provider ID: %w", err)
	}

	// 1. Update provider in database
	updates := map[string]interface{}{
		"display_name":  config.DisplayName,
		"template":      config.Template,
		"configuration": config.Configuration,
	}

	if err := a.db.WithContext(ctx).
		Model(&models.LLMProvider{}).
		Where("uuid = ?", providerUUID).
		Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update provider: %w", err)
	}

	// 2. Broadcast api.deployed event (same as api-platform for updates)
	event := &models.DeploymentEvent{
		ApiId:        providerID,
		DeploymentID: providerID,
		Vhost:        "",
		Environment:  "production",
	}

	if err := a.eventsService.BroadcastLLMProviderDeployed(gatewayID, event); err != nil {
		return nil, fmt.Errorf("failed to broadcast update: %w", err)
	}

	return &gateway.ProviderDeploymentResult{
		DeploymentID: providerID,
		Status:       "UPDATED",
		DeployedAt:   time.Now(),
	}, nil
}

// UndeployProvider removes an LLM provider from a gateway using WebSocket events
func (a *OnPremiseAdapter) UndeployProvider(ctx context.Context, gatewayID string, providerID string) error {
	a.logger.Info("Undeploying provider from gateway", "gatewayID", gatewayID, "providerID", providerID)

	providerUUID, err := uuid.Parse(providerID)
	if err != nil {
		return fmt.Errorf("invalid provider ID: %w", err)
	}

	gatewayUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return fmt.Errorf("invalid gateway ID: %w", err)
	}

	// 1. Delete deployment record
	if err := a.db.WithContext(ctx).
		Where("provider_uuid = ? AND gateway_uuid = ?", providerUUID, gatewayUUID).
		Delete(&models.ProviderGatewayDeployment{}).Error; err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	// 2. Broadcast api.undeployed event (same as api-platform)
	event := &models.APIUndeploymentEvent{
		ApiId:       providerID,
		Vhost:       "",
		Environment: "production",
	}

	if err := a.eventsService.BroadcastLLMProviderUndeployed(gatewayID, event); err != nil {
		return fmt.Errorf("failed to broadcast undeployment: %w", err)
	}

	return nil
}

// GetProviderStatus retrieves the status of a provider deployment on a gateway
func (a *OnPremiseAdapter) GetProviderStatus(ctx context.Context, gatewayID string, providerID string) (*gateway.ProviderStatus, error) {
	gatewayUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway ID: %w", err)
	}

	providerUUID, err := uuid.Parse(providerID)
	if err != nil {
		return nil, fmt.Errorf("invalid provider ID: %w", err)
	}

	var deployment models.ProviderGatewayDeployment
	err = a.db.WithContext(ctx).
		Preload("Provider").
		Where("provider_uuid = ? AND gateway_uuid = ?", providerUUID, gatewayUUID).
		First(&deployment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("provider deployment not found: %s", providerID)
		}
		return nil, fmt.Errorf("failed to query deployment: %w", err)
	}

	status := &gateway.ProviderStatus{
		ID:     deployment.DeploymentID,
		Status: deployment.Status,
	}

	if deployment.Provider != nil {
		status.Name = deployment.Provider.DisplayName
		status.Kind = deployment.Provider.Template
		status.Spec = deployment.Provider.Configuration
		status.DeployedAt = deployment.DeployedAt
	}

	return status, nil
}

// ListProviders lists all LLM providers deployed on a gateway
func (a *OnPremiseAdapter) ListProviders(ctx context.Context, gatewayID string) ([]*gateway.ProviderStatus, error) {
	gatewayUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway ID: %w", err)
	}

	var deployments []models.ProviderGatewayDeployment
	err = a.db.WithContext(ctx).
		Preload("Provider").
		Where("gateway_uuid = ?", gatewayUUID).
		Find(&deployments).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments: %w", err)
	}

	providers := make([]*gateway.ProviderStatus, 0, len(deployments))
	for _, d := range deployments {
		status := &gateway.ProviderStatus{
			ID:     d.DeploymentID,
			Status: d.Status,
		}
		if d.Provider != nil {
			status.Name = d.Provider.DisplayName
			status.Kind = d.Provider.Template
			status.Spec = d.Provider.Configuration
			status.DeployedAt = d.DeployedAt
		}
		providers = append(providers, status)
	}

	return providers, nil
}

// GetPolicies retrieves available policies from a gateway
// Returns a static list for on-premise deployments
func (a *OnPremiseAdapter) GetPolicies(ctx context.Context, gatewayID string) ([]*gateway.PolicyInfo, error) {
	return []*gateway.PolicyInfo{
		{Name: "rate-limit", Version: "v1", Description: "Rate limiting policy"},
		{Name: "content-filter", Version: "v1", Description: "Content filtering policy"},
		{Name: "token-limit", Version: "v1", Description: "Token usage limiting policy"},
	}, nil
}
