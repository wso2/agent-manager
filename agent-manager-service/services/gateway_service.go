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
	"slices"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/gateway"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// GatewayService defines the interface for gateway operations
type GatewayService interface {
	RegisterGateway(ctx context.Context, orgName string, req *models.CreateGatewayRequest) (*models.GatewayResponse, error)
	GetGateway(ctx context.Context, orgName string, gatewayID string) (*models.GatewayResponse, error)
	ListGateways(ctx context.Context, orgName string, filter GatewayFilter) (*models.GatewayListResponse, error)
	UpdateGateway(ctx context.Context, orgName string, gatewayID string, req *models.UpdateGatewayRequest) (*models.GatewayResponse, error)
	DeleteGateway(ctx context.Context, orgName string, gatewayID string) error
	AssignGatewayToEnvironment(ctx context.Context, orgName string, gatewayID, envID string) error
	RemoveGatewayFromEnvironment(ctx context.Context, orgName string, gatewayID, envID string) error
	GetGatewayEnvironments(ctx context.Context, orgName string, gatewayID string) ([]models.GatewayEnvironmentResponse, error)
	CheckGatewayHealth(ctx context.Context, orgName string, gatewayID string) (*models.HealthStatusResponse, error)
	// WebSocket-related methods
	VerifyToken(ctx context.Context, apiKey string) (*models.Gateway, error)
	UpdateGatewayActiveStatus(ctx context.Context, gatewayID string, active bool) error
}

// GatewayFilter defines filter options for listing gateways
type GatewayFilter struct {
	GatewayType   *string
	Status        *string
	EnvironmentID *string
	Region        *string
	Limit         int32
	Offset        int32
}

type gatewayService struct {
	adapter gateway.IGatewayAdapter
	logger  *slog.Logger
}

// isValidGatewayStatus validates if the given status is a valid gateway status
func isValidGatewayStatus(status string) bool {
	validStatuses := []string{"ACTIVE", "INACTIVE", "MAINTENANCE"}
	return slices.Contains(validStatuses, status)
}

// NewGatewayService creates a new gateway service
func NewGatewayService(adapter gateway.IGatewayAdapter, logger *slog.Logger) GatewayService {
	return &gatewayService{
		adapter: adapter,
		logger:  logger,
	}
}

func (s *gatewayService) RegisterGateway(ctx context.Context, orgName string, req *models.CreateGatewayRequest) (*models.GatewayResponse, error) {
	s.logger.Info("Registering gateway", "name", req.Name, "orgName", orgName)

	// Check if gateway already exists
	var existing models.Gateway
	err := db.DB(ctx).Where("organization_name = ? AND name = ?", orgName, req.Name).First(&existing).Error
	if err == nil {
		return nil, utils.ErrGatewayAlreadyExists
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("failed to check existing gateway: %w", err)
	}

	// Extract control plane URL for on-premise mode
	var controlPlaneURL string
	if url, ok := req.AdapterConfig["controlPlaneUrl"].(string); ok {
		controlPlaneURL = url

		// Validate endpoint is reachable
		if err := s.adapter.ValidateGatewayEndpoint(ctx, controlPlaneURL); err != nil {
			s.logger.Error("Gateway endpoint validation failed", "url", controlPlaneURL, "error", err)
			return nil, fmt.Errorf("gateway endpoint unreachable: %w", err)
		}
	}

	gw := &models.Gateway{
		UUID:             uuid.New(),
		OrganizationName: orgName,
		Name:             req.Name,
		DisplayName:      req.DisplayName,
		GatewayType:      req.GatewayType,
		ControlPlaneURL:  controlPlaneURL,
		VHost:            req.VHost,
		Region:           req.Region,
		IsCritical:       req.IsCritical,
		Status:           string(models.GatewayStatusActive),
		AdapterConfig:    req.AdapterConfig,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Generate and hash API key for WebSocket authentication
	apiKey, err := utils.GenerateAPIKey()
	if err != nil {
		s.logger.Error("Failed to generate API key", "error", err)
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Hash the API key for storage
	apiKeyHash, err := utils.HashAPIKey(apiKey)
	if err != nil {
		s.logger.Error("Failed to hash API key", "error", err)
		return nil, fmt.Errorf("failed to hash API key: %w", err)
	}
	gw.APIKeyHash = apiKeyHash

	if err := db.DB(ctx).Create(gw).Error; err != nil {
		s.logger.Error("Failed to create gateway", "error", err)
		return nil, fmt.Errorf("failed to create gateway: %w", err)
	}

	// Assign gateway to environments if provided
	if len(req.EnvironmentIDs) > 0 {
		s.logger.Info("Assigning gateway to environments", "gatewayUUID", gw.UUID, "environmentCount", len(req.EnvironmentIDs))

		// Validate all environment IDs first
		for _, envID := range req.EnvironmentIDs {
			envUUID, err := uuid.Parse(envID)
			if err != nil {
				return nil, fmt.Errorf("invalid environment UUID %s: %w", envID, utils.ErrInvalidInput)
			}

			// Verify environment exists and belongs to the organization
			var env models.Environment
			if err := db.DB(ctx).Where("uuid = ? AND organization_name = ?", envUUID, orgName).First(&env).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, fmt.Errorf("environment %s not found: %w", envID, utils.ErrEnvironmentNotFound)
				}
				return nil, fmt.Errorf("failed to validate environment %s: %w", envID, err)
			}

			// Create mapping (ignore duplicates)
			mapping := &models.GatewayEnvironmentMapping{
				GatewayUUID:     gw.UUID,
				EnvironmentUUID: envUUID,
				CreatedAt:       time.Now(),
			}

			if err := db.DB(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(mapping).Error; err != nil {
				s.logger.Warn("Failed to create environment mapping", "gatewayUUID", gw.UUID, "envUUID", envUUID, "error", err)
				// Continue with other assignments even if one fails
			} else {
				s.logger.Info("Gateway assigned to environment", "gatewayUUID", gw.UUID, "envUUID", envUUID)
			}
		}

		// Reload gateway with environments
		if err := db.DB(ctx).Preload("Environments").First(gw, "uuid = ?", gw.UUID).Error; err != nil {
			s.logger.Warn("Failed to reload gateway with environments", "error", err)
		}
	}

	s.logger.Info("Gateway registered successfully", "uuid", gw.UUID, "apiKey", apiKey)
	resp := gw.ToResponse()
	resp.APIKey = apiKey // Only returned during registration
	return resp, nil
}

func (s *gatewayService) GetGateway(ctx context.Context, orgName string, gatewayID string) (*models.GatewayResponse, error) {
	s.logger.Info("Getting gateway", "gatewayID", gatewayID, "orgName", orgName)

	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return nil, utils.ErrInvalidInput
	}

	var gw models.Gateway
	err = db.DB(ctx).Preload("Environments").Where("uuid = ? AND organization_name = ?", gwUUID, orgName).First(&gw).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrGatewayNotFound
		}
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}

	return gw.ToResponse(), nil
}

func (s *gatewayService) ListGateways(ctx context.Context, orgName string, filter GatewayFilter) (*models.GatewayListResponse, error) {
	s.logger.Info("Listing gateways", "orgName", orgName, "filter", filter)

	var gateways []models.Gateway
	var total int64

	query := db.DB(ctx).Model(&models.Gateway{}).Where("organization_name = ?", orgName)

	// Apply filters
	if filter.GatewayType != nil {
		query = query.Where("gateway_type = ?", *filter.GatewayType)
	}
	if filter.Status != nil {
		query = query.Where("status = ?", *filter.Status)
	}
	if filter.Region != nil {
		query = query.Where("region = ?", *filter.Region)
	}
	if filter.EnvironmentID != nil {
		envUUID, err := uuid.Parse(*filter.EnvironmentID)
		if err != nil {
			return nil, utils.ErrInvalidInput
		}
		query = query.Joins("JOIN gateway_environment_mappings ON gateway_environment_mappings.gateway_uuid = gateways.uuid").
			Where("gateway_environment_mappings.environment_uuid = ?", envUUID)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count gateways: %w", err)
	}

	if err := query.Preload("Environments").Limit(int(filter.Limit)).Offset(int(filter.Offset)).Order("created_at DESC").Find(&gateways).Error; err != nil {
		return nil, fmt.Errorf("failed to list gateways: %w", err)
	}

	responses := make([]models.GatewayResponse, len(gateways))
	for i, gw := range gateways {
		responses[i] = *gw.ToResponse()
	}

	return &models.GatewayListResponse{
		Gateways: responses,
		Total:    int32(total),
		Limit:    filter.Limit,
		Offset:   filter.Offset,
	}, nil
}

func (s *gatewayService) UpdateGateway(ctx context.Context, orgName string, gatewayID string, req *models.UpdateGatewayRequest) (*models.GatewayResponse, error) {
	s.logger.Info("Updating gateway", "gatewayID", gatewayID, "orgName", orgName)

	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return nil, utils.ErrInvalidInput
	}

	var gw models.Gateway
	err = db.DB(ctx).Where("uuid = ? AND organization_name = ?", gwUUID, orgName).First(&gw).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrGatewayNotFound
		}
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}

	if req.DisplayName != nil {
		gw.DisplayName = *req.DisplayName
	}
	if req.IsCritical != nil {
		gw.IsCritical = *req.IsCritical
	}
	if req.Status != nil {
		if !isValidGatewayStatus(*req.Status) {
			return nil, errors.New("invalid gateway status")
		}
		gw.Status = *req.Status
	}
	if req.AdapterConfig != nil {
		// Merge adapter config
		if gw.AdapterConfig == nil {
			gw.AdapterConfig = make(map[string]interface{})
		}
		for k, v := range req.AdapterConfig {
			gw.AdapterConfig[k] = v
		}
	}

	gw.UpdatedAt = time.Now()

	if err := db.DB(ctx).Save(&gw).Error; err != nil {
		return nil, fmt.Errorf("failed to update gateway: %w", err)
	}

	// Reload with environments
	if err := db.DB(ctx).Preload("Environments").First(&gw, "uuid = ?", gwUUID).Error; err != nil {
		return nil, fmt.Errorf("failed to reload gateway: %w", err)
	}

	return gw.ToResponse(), nil
}

func (s *gatewayService) DeleteGateway(ctx context.Context, orgName string, gatewayID string) error {
	s.logger.Info("Deleting gateway", "gatewayID", gatewayID, "orgName", orgName)

	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return utils.ErrInvalidInput
	}

	result := db.DB(ctx).Where("uuid = ? AND organization_name = ?", gwUUID, orgName).Delete(&models.Gateway{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete gateway: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return utils.ErrGatewayNotFound
	}

	return nil
}

func (s *gatewayService) AssignGatewayToEnvironment(ctx context.Context, orgName string, gatewayID, envID string) error {
	s.logger.Info("Assigning gateway to environment", "gatewayID", gatewayID, "envID", envID)

	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return utils.ErrInvalidInput
	}
	envUUID, err := uuid.Parse(envID)
	if err != nil {
		return utils.ErrInvalidInput
	}

	// Verify gateway exists
	var gw models.Gateway
	if err := db.DB(ctx).Where("uuid = ? AND organization_name = ?", gwUUID, orgName).First(&gw).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrGatewayNotFound
		}
		return fmt.Errorf("failed to get gateway: %w", err)
	}

	// Verify environment exists
	var env models.Environment
	if err := db.DB(ctx).Where("uuid = ? AND organization_name = ?", envUUID, orgName).First(&env).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrEnvironmentNotFound
		}
		return fmt.Errorf("failed to get environment: %w", err)
	}

	// Create mapping (ignore duplicate)
	mapping := &models.GatewayEnvironmentMapping{
		GatewayUUID:     gwUUID,
		EnvironmentUUID: envUUID,
		CreatedAt:       time.Now(),
	}

	// Use GORM's OnConflict clause to handle duplicate assignments gracefully
	// This is DB-dialect-agnostic and avoids brittle string matching
	result := db.DB(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(mapping)
	if result.Error != nil {
		return fmt.Errorf("failed to assign gateway to environment: %w", result.Error)
	}

	return nil
}

func (s *gatewayService) RemoveGatewayFromEnvironment(ctx context.Context, orgName string, gatewayID, envID string) error {
	s.logger.Info("Removing gateway from environment", "gatewayID", gatewayID, "envID", envID)

	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return utils.ErrInvalidInput
	}
	envUUID, err := uuid.Parse(envID)
	if err != nil {
		return utils.ErrInvalidInput
	}

	// Verify gateway exists and belongs to the organization
	var gw models.Gateway
	if err := db.DB(ctx).Where("uuid = ? AND organization_name = ?", gwUUID, orgName).First(&gw).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrGatewayNotFound
		}
		return fmt.Errorf("failed to get gateway: %w", err)
	}

	result := db.DB(ctx).Where("gateway_uuid = ? AND environment_uuid = ?", gwUUID, envUUID).Delete(&models.GatewayEnvironmentMapping{})
	if result.Error != nil {
		return fmt.Errorf("failed to remove gateway from environment: %w", result.Error)
	}

	return nil
}

func (s *gatewayService) GetGatewayEnvironments(ctx context.Context, orgName string, gatewayID string) ([]models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Getting gateway environments", "gatewayID", gatewayID)

	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return nil, utils.ErrInvalidInput
	}

	// Verify gateway exists
	var gw models.Gateway
	if err := db.DB(ctx).Where("uuid = ? AND organization_name = ?", gwUUID, orgName).First(&gw).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrGatewayNotFound
		}
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}

	var environments []models.Environment
	err = db.DB(ctx).
		Joins("JOIN gateway_environment_mappings ON gateway_environment_mappings.environment_uuid = environments.uuid").
		Where("gateway_environment_mappings.gateway_uuid = ?", gwUUID).
		Find(&environments).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get environments: %w", err)
	}

	responses := make([]models.GatewayEnvironmentResponse, len(environments))
	for i, env := range environments {
		responses[i] = *env.ToResponse()
	}

	return responses, nil
}

func (s *gatewayService) CheckGatewayHealth(ctx context.Context, orgName string, gatewayID string) (*models.HealthStatusResponse, error) {
	s.logger.Info("Checking gateway health", "gatewayID", gatewayID)

	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return nil, utils.ErrInvalidInput
	}

	var gw models.Gateway
	if err := db.DB(ctx).Where("uuid = ? AND organization_name = ?", gwUUID, orgName).First(&gw).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrGatewayNotFound
		}
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}

	if gw.ControlPlaneURL == "" {
		return &models.HealthStatusResponse{
			GatewayID:    gatewayID,
			Status:       "UNKNOWN",
			ErrorMessage: "No control plane URL configured",
			CheckedAt:    time.Now().Format(time.RFC3339),
		}, nil
	}

	health, err := s.adapter.CheckHealth(ctx, gw.ControlPlaneURL)

	// If health struct is returned (even with error status), encode it in response
	if health != nil {
		response := &models.HealthStatusResponse{
			GatewayID:    gatewayID,
			Status:       health.Status,
			ResponseTime: health.ResponseTime.String(),
			ErrorMessage: health.ErrorMessage,
			CheckedAt:    health.CheckedAt.Format(time.RFC3339),
		}
		return response, nil // Return response with nil error
	}

	// Only return error if no health information available
	if err != nil {
		return nil, fmt.Errorf("failed to check gateway health: %w", err)
	}

	// Fallback (should not happen)
	return &models.HealthStatusResponse{
		GatewayID:    gatewayID,
		Status:       "UNKNOWN",
		ErrorMessage: "Health check returned no data",
		CheckedAt:    time.Now().Format(time.RFC3339),
	}, nil
}

// VerifyToken verifies an API key and returns the associated gateway
func (s *gatewayService) VerifyToken(ctx context.Context, apiKey string) (*models.Gateway, error) {
	s.logger.Debug("Verifying API key for gateway connection")

	// Query database for gateway with API key hash
	var gateway models.Gateway
	err := db.DB(ctx).Where("api_key_hash IS NOT NULL").Find(&gateway).Error
	if err != nil {
		s.logger.Error("Failed to query gateways", "error", err)
		return nil, fmt.Errorf("failed to query gateways: %w", err)
	}

	// We need to manually check each gateway since we can't query by hash directly
	// Get all gateways with API keys and verify one by one
	var gateways []models.Gateway
	err = db.DB(ctx).Where("api_key_hash IS NOT NULL").Find(&gateways).Error
	if err != nil {
		s.logger.Error("Failed to query gateways", "error", err)
		return nil, fmt.Errorf("failed to query gateways: %w", err)
	}

	// Check each gateway's API key hash
	for _, gw := range gateways {
		if gw.APIKeyHash == nil {
			continue
		}
		// Verify the API key against the stored hash
		if err := utils.VerifyAPIKey(apiKey, gw.APIKeyHash); err == nil {
			// API key matches - allow connection (status will be updated after connection)
			s.logger.Debug("API key verified successfully", "gatewayId", gw.UUID, "currentStatus", gw.Status)
			return &gw, nil
		}
	}

	s.logger.Warn("No gateway found matching the provided API key")
	return nil, utils.ErrGatewayNotFound
}

// UpdateGatewayActiveStatus updates the active status of a gateway
func (s *gatewayService) UpdateGatewayActiveStatus(ctx context.Context, gatewayID string, active bool) error {
	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return fmt.Errorf("invalid gateway ID: %w", err)
	}

	status := "ACTIVE"
	if !active {
		status = "INACTIVE"
	}

	err = db.DB(ctx).Model(&models.Gateway{}).
		Where("uuid = ?", gwUUID).
		Update("status", status).Error
	if err != nil {
		return fmt.Errorf("failed to update gateway status: %w", err)
	}

	s.logger.Debug("Gateway active status updated", "gatewayId", gatewayID, "active", active)
	return nil
}
