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
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// GatewayInternalService handles internal API operations for gateways
// Must be compatible with api-platform's internal API format
type GatewayInternalService interface {
	// GetAPIsByOrganization retrieves all LLM providers for an organization as YAML map
	// Endpoint: GET /api/internal/v1/apis
	// Returns: map of API ID to YAML content (for ZIP creation)
	GetAPIsByOrganization(ctx context.Context, orgID string) (map[string]string, error)

	// GetAPI returns an LLM provider as an API configuration (for gateway compatibility)
	// Endpoint: GET /api/internal/v1/apis/{apiId}
	// Returns: map of API ID to YAML content (for ZIP creation)
	GetAPI(ctx context.Context, apiID string, gatewayID string) (map[string]string, error)

	// CreateGatewayDeployment creates a deployment notification from gateway
	// Endpoint: POST /api/internal/v1/apis/{apiId}/gateway-deployments
	// Returns: GatewayDeploymentResponse with API ID, deployment ID, message, and created flag
	CreateGatewayDeployment(ctx context.Context, apiID string, orgID string, gatewayID string, notification *models.GatewayDeploymentNotification, revisionID *string) (*models.GatewayDeploymentResponse, error)
}

type gatewayInternalService struct {
	logger *slog.Logger
}

// NewGatewayInternalService creates a new gateway internal service
func NewGatewayInternalService(logger *slog.Logger) GatewayInternalService {
	return &gatewayInternalService{
		logger: logger,
	}
}

// GetAPIsByOrganization retrieves all LLM providers for an organization as YAML map
func (s *gatewayInternalService) GetAPIsByOrganization(ctx context.Context, orgID string) (map[string]string, error) {
	dbInstance := db.DB(ctx)

	// Get all providers for the organization
	var providers []models.LLMProvider
	err := dbInstance.
		Where("organization_name = ?", orgID).
		Find(&providers).Error
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve providers: %w", err)
	}

	apiYamlMap := make(map[string]string)
	for _, provider := range providers {
		// Convert LLM provider to API configuration format
		apiConfig := s.convertLLMProviderToAPIConfig(&provider, nil)

		// Marshal to YAML
		yamlData, err := yaml.Marshal(apiConfig)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal provider %s: %w", provider.UUID, err)
		}

		apiYamlMap[provider.UUID.String()] = string(yamlData)
	}

	return apiYamlMap, nil
}

// GetAPI returns an LLM provider configuration in api-platform compatible format
// The apiID parameter is actually the provider UUID
func (s *gatewayInternalService) GetAPI(ctx context.Context, apiID string, gatewayID string) (map[string]string, error) {
	providerUUID, err := uuid.Parse(apiID)
	if err != nil {
		return nil, fmt.Errorf("invalid API ID: %w", err)
	}

	gatewayUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway ID: %w", err)
	}

	dbInstance := db.DB(ctx)

	// Check if provider is deployed to this gateway
	var deployment models.ProviderGatewayDeployment
	err = dbInstance.
		Where("provider_uuid = ? AND gateway_uuid = ?", providerUUID, gatewayUUID).
		First(&deployment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("API not deployed to this gateway")
		}
		return nil, fmt.Errorf("failed to query deployment: %w", err)
	}

	// Get the provider
	var provider models.LLMProvider
	err = dbInstance.
		Where("uuid = ?", providerUUID).
		First(&provider).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	// Convert LLM provider to API configuration format (compatible with api-platform)
	apiConfig := s.convertLLMProviderToAPIConfig(&provider, &deployment)

	// Marshal to YAML
	yamlData, err := yaml.Marshal(apiConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	// Return as map (for ZIP creation)
	apiYamlMap := map[string]string{
		providerUUID.String(): string(yamlData),
	}

	return apiYamlMap, nil
}

// convertLLMProviderToAPIConfig converts an LLM provider to api-platform's APIConfiguration format
func (s *gatewayInternalService) convertLLMProviderToAPIConfig(provider *models.LLMProvider, deployment *models.ProviderGatewayDeployment) map[string]interface{} {
	// This structure must match api-platform's APIConfiguration format
	return map[string]interface{}{
		"apiVersion": "gateway.agent-manager.wso2.com/v1alpha1",
		"kind":       "LLMProvider",
		"metadata": map[string]interface{}{
			"name":        provider.Handle,
			"displayName": provider.DisplayName,
			"uid":         provider.UUID.String(),
		},
		"spec": map[string]interface{}{
			"template":      provider.Template,
			"configuration": provider.Configuration,
			"environment":   deployment.Environment,
		},
	}
}

// CreateGatewayDeployment handles the registration of an LLM provider deployment from a gateway
// Compatible with api-platform's CreateGatewayDeployment logic
func (s *gatewayInternalService) CreateGatewayDeployment(ctx context.Context, apiID string, orgID string, gatewayID string, notification *models.GatewayDeploymentNotification, revisionID *string) (*models.GatewayDeploymentResponse, error) {
	// Note: revisionID parameter is reserved for future use
	_ = revisionID

	dbInstance := db.DB(ctx)

	// Validate input
	if apiID == "" || orgID == "" || gatewayID == "" {
		return nil, fmt.Errorf("invalid input")
	}

	// Check if the gateway exists and belongs to the organization
	var gateway models.Gateway
	err := dbInstance.
		Where("uuid = ? AND organization_name = ?", gatewayID, orgID).
		First(&gateway).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("gateway not found")
		}
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}

	// Check if provider already exists by getting metadata
	var existingProvider models.LLMProvider
	err = dbInstance.
		Where("handle = ? AND organization_name = ?", apiID, orgID).
		First(&existingProvider).Error

	providerCreated := false
	now := time.Now()
	var providerUUID string

	if errors.Is(err, gorm.ErrRecordNotFound) {
		// Create new provider from notification
		newProvider := &models.LLMProvider{
			UUID:             uuid.New(),
			OrganizationName: orgID,
			Handle:           apiID,
			DisplayName:      notification.Configuration.Spec.Name,
			Template:         notification.Configuration.Kind,
			Configuration:    convertConfigToMap(notification.Configuration),
			Status:           "APPROVED",
			CreatedAt:        now,
			UpdatedAt:        now,
			CreatedBy:        "admin", // Default creator
		}

		err = dbInstance.Create(newProvider).Error
		if err != nil {
			return nil, fmt.Errorf("failed to create provider: %w", err)
		}

		providerUUID = newProvider.UUID.String()
		providerCreated = true
	} else if err != nil {
		return nil, fmt.Errorf("failed to check existing provider: %w", err)
	} else {
		// Provider exists
		providerUUID = existingProvider.UUID.String()
	}

	// Check if deployment already exists
	var existingDeployment models.ProviderGatewayDeployment
	err = dbInstance.
		Where("provider_uuid = ? AND gateway_uuid = ?", providerUUID, gatewayID).
		First(&existingDeployment).Error

	if err == nil {
		// An active deployment already exists for this provider-gateway combination
		if existingDeployment.Status == "DEPLOYED" {
			return nil, fmt.Errorf("provider already deployed to this gateway")
		}
		return nil, fmt.Errorf("a deployment already exists for this provider-gateway combination with status %s", existingDeployment.Status)
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check deployment status: %w", err)
	}

	// Create deployment record
	deployment := &models.ProviderGatewayDeployment{
		ProviderUUID:         uuid.MustParse(providerUUID),
		GatewayUUID:          uuid.MustParse(gatewayID),
		DeploymentID:         notification.ID,
		Environment:          notification.Configuration.Spec.Context, // Use context as environment
		ConfigurationVersion: 1,
		GatewayOverrides:     nil,
		Status:               notification.Status,
		DeployedAt:           nil,
		ErrorMessage:         nil,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	if notification.Status == "DEPLOYED" {
		deployment.DeployedAt = &now
	}

	err = dbInstance.Create(deployment).Error
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment record: %w", err)
	}

	s.logger.Info("Created provider deployment",
		"apiId", apiID,
		"gatewayId", gatewayID,
		"status", notification.Status,
		"created", providerCreated,
	)

	return &models.GatewayDeploymentResponse{
		APIId:        providerUUID,
		DeploymentId: int64(deployment.ID),
		Message:      "LLM provider deployment registered successfully",
		Created:      providerCreated,
	}, nil
}

// convertConfigToMap converts APIConfiguration to map[string]interface{}
func convertConfigToMap(config models.APIConfiguration) map[string]interface{} {
	return map[string]interface{}{
		"version": config.Version,
		"kind":    config.Kind,
		"spec": map[string]interface{}{
			"name":        config.Spec.Name,
			"version":     config.Spec.Version,
			"context":     config.Spec.Context,
			"projectName": config.Spec.ProjectName,
			"upstreams":   config.Spec.Upstreams,
			"operations":  config.Spec.Operations,
		},
	}
}
