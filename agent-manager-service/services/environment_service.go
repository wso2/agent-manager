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
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	apiplatformclient "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/apiplatformsvc/client"
	occlient "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// EnvironmentService defines the interface for environment operations
type EnvironmentService interface {
	CreateEnvironment(ctx context.Context, orgName string, req *models.CreateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error)
	GetEnvironment(ctx context.Context, orgName string, envID string) (*models.GatewayEnvironmentResponse, error)
	ListEnvironments(ctx context.Context, orgName string, limit, offset int32) (*models.EnvironmentListResponse, error)
	UpdateEnvironment(ctx context.Context, orgName string, envID string, req *models.UpdateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error)
	DeleteEnvironment(ctx context.Context, orgName string, envID string) error
	GetEnvironmentGateways(ctx context.Context, orgName string, envID string) ([]models.GatewayResponse, error)
}

type environmentService struct {
	logger            *slog.Logger
	apiPlatformClient apiplatformclient.APIPlatformClient
	ocClient          occlient.OpenChoreoClient
}

// NewEnvironmentService creates a new environment service
func NewEnvironmentService(logger *slog.Logger, apiPlatformClient apiplatformclient.APIPlatformClient, ocClient occlient.OpenChoreoClient) EnvironmentService {
	return &environmentService{
		logger:            logger,
		apiPlatformClient: apiPlatformClient,
		ocClient:          ocClient,
	}
}

func (s *environmentService) CreateEnvironment(ctx context.Context, orgName string, req *models.CreateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Creating environment", "name", req.Name, "orgName", orgName)

	env := &models.Environment{
		UUID:             uuid.New(),
		OrganizationName: orgName,
		Name:             req.Name,
		DisplayName:      req.DisplayName,
		Description:      req.Description,
		DataplaneRef:     req.DataplaneRef,
		DNSPrefix:        req.DNSPrefix,
		IsProduction:     req.IsProduction,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Wrap in transaction to handle potential race conditions
	err := db.DB(ctx).Transaction(func(tx *gorm.DB) error {
		// Check if environment already exists within the transaction
		var existing models.Environment
		err := tx.Where("organization_name = ? AND name = ?", orgName, req.Name).First(&existing).Error
		if err == nil {
			return utils.ErrEnvironmentAlreadyExists
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("failed to check existing environment: %w", err)
		}

		// Create the environment
		if err := tx.Create(env).Error; err != nil {
			// Handle unique constraint violation from database
			if strings.Contains(strings.ToLower(err.Error()), "unique") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
				return utils.ErrEnvironmentAlreadyExists
			}
			return fmt.Errorf("failed to create environment: %w", err)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, utils.ErrEnvironmentAlreadyExists) {
			return nil, utils.ErrEnvironmentAlreadyExists
		}
		s.logger.Error("Failed to create environment", "error", err)
		return nil, err
	}

	s.logger.Info("Environment created successfully", "uuid", env.UUID)
	return env.ToResponse(), nil
}

func (s *environmentService) GetEnvironment(ctx context.Context, orgName string, envID string) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Getting environment", "envID", envID, "orgName", orgName)

	envUUID, err := uuid.Parse(envID)
	if err != nil {
		return nil, utils.ErrInvalidInput
	}

	var env models.Environment
	err = db.DB(ctx).Where("uuid = ? AND organization_name = ?", envUUID, orgName).First(&env).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	return env.ToResponse(), nil
}

func (s *environmentService) ListEnvironments(ctx context.Context, orgName string, limit, offset int32) (*models.EnvironmentListResponse, error) {
	s.logger.Info("Listing environments from OpenChoreo", "orgName", orgName, "limit", limit, "offset", offset)

	// Fetch environments directly from OpenChoreo
	ocEnvironments, err := s.ocClient.ListEnvironments(ctx, orgName)
	if err != nil {
		s.logger.Error("Failed to list environments from OpenChoreo", "orgName", orgName, "error", err)
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	total := int32(len(ocEnvironments))

	// Apply pagination
	start := int(offset)
	end := start + int(limit)

	if start >= len(ocEnvironments) {
		// Offset is beyond available data
		return &models.EnvironmentListResponse{
			Environments: []models.GatewayEnvironmentResponse{},
			Total:        total,
			Limit:        limit,
			Offset:       offset,
		}, nil
	}

	if end > len(ocEnvironments) {
		end = len(ocEnvironments)
	}

	paginatedEnvs := ocEnvironments[start:end]

	// Convert OpenChoreo environment responses to gateway environment responses
	responses := make([]models.GatewayEnvironmentResponse, len(paginatedEnvs))
	for i, env := range paginatedEnvs {
		responses[i] = models.GatewayEnvironmentResponse{
			UUID:             env.UUID,
			OrganizationName: orgName,
			Name:             env.Name,
			DisplayName:      env.DisplayName,
			Description:      "", // OpenChoreo EnvironmentResponse doesn't have description
			DataplaneRef:     env.DataplaneRef,
			DNSPrefix:        env.DNSPrefix,
			IsProduction:     env.IsProduction,
			CreatedAt:        env.CreatedAt,
			UpdatedAt:        env.CreatedAt,
		}
	}

	return &models.EnvironmentListResponse{
		Environments: responses,
		Total:        total,
		Limit:        limit,
		Offset:       offset,
	}, nil
}

func (s *environmentService) UpdateEnvironment(ctx context.Context, orgName string, envID string, req *models.UpdateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Updating environment", "envID", envID, "orgName", orgName)

	envUUID, err := uuid.Parse(envID)
	if err != nil {
		return nil, utils.ErrInvalidInput
	}

	var env models.Environment
	err = db.DB(ctx).Where("uuid = ? AND organization_name = ?", envUUID, orgName).First(&env).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	if req.DisplayName != nil {
		env.DisplayName = *req.DisplayName
	}
	if req.Description != nil {
		env.Description = *req.Description
	}
	env.UpdatedAt = time.Now()

	if err := db.DB(ctx).Save(&env).Error; err != nil {
		return nil, fmt.Errorf("failed to update environment: %w", err)
	}

	return env.ToResponse(), nil
}

func (s *environmentService) DeleteEnvironment(ctx context.Context, orgName string, envID string) error {
	s.logger.Info("Deleting environment", "envID", envID, "orgName", orgName)

	envUUID, err := uuid.Parse(envID)
	if err != nil {
		return utils.ErrInvalidInput
	}

	// Wrap in transaction to handle race conditions with gateway assignments
	err = db.DB(ctx).Transaction(func(tx *gorm.DB) error {
		// Check if environment has associated gateways within the transaction
		var count int64
		if err := tx.Model(&models.GatewayEnvironmentMapping{}).Where("environment_uuid = ?", envUUID).Count(&count).Error; err != nil {
			return fmt.Errorf("failed to check gateway associations: %w", err)
		}
		if count > 0 {
			return utils.ErrEnvironmentHasGateways
		}

		// Delete the environment
		result := tx.Where("uuid = ? AND organization_name = ?", envUUID, orgName).Delete(&models.Environment{})
		if result.Error != nil {
			return fmt.Errorf("failed to delete environment: %w", result.Error)
		}
		if result.RowsAffected == 0 {
			return utils.ErrEnvironmentNotFound
		}

		return nil
	})
	if err != nil {
		if errors.Is(err, utils.ErrEnvironmentHasGateways) || errors.Is(err, utils.ErrEnvironmentNotFound) {
			return err
		}
		s.logger.Error("Failed to delete environment", "error", err)
		return err
	}

	return nil
}

func (s *environmentService) GetEnvironmentGateways(ctx context.Context, orgName string, envID string) ([]models.GatewayResponse, error) {
	s.logger.Info("Getting environment gateways", "envID", envID, "orgName", orgName)

	envUUID, err := uuid.Parse(envID)
	if err != nil {
		return nil, utils.ErrInvalidInput
	}

	// Verify environment exists
	var env models.Environment
	if err := db.DB(ctx).Where("uuid = ? AND organization_name = ?", envUUID, orgName).First(&env).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	// Get gateway-environment mappings from DB
	var mappings []models.GatewayEnvironmentMapping
	err = db.DB(ctx).
		Where("environment_uuid = ?", envUUID).
		Find(&mappings).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway mappings: %w", err)
	}

	// Fetch each gateway from API Platform service
	responses := make([]models.GatewayResponse, 0, len(mappings))
	for _, mapping := range mappings {
		gatewayID := mapping.GatewayUUID.String()

		// Get gateway details from API Platform
		gateway, err := s.apiPlatformClient.GetGateway(ctx, gatewayID)
		if err != nil {
			s.logger.Warn("Failed to get gateway from API Platform", "gatewayID", gatewayID, "error", err)
			// Skip gateways that no longer exist in API Platform
			continue
		}

		// Convert API Platform gateway response to models.GatewayResponse
		responses = append(responses, models.GatewayResponse{
			UUID:             gateway.ID,
			OrganizationName: orgName,
			Name:             gateway.Name,
			DisplayName:      gateway.DisplayName,
			GatewayType:      gateway.FunctionalityType,
			VHost:            gateway.Vhost,
			IsCritical:       gateway.IsCritical,
			Status:           convertAPIPlatformStatusToModelStatus(gateway.IsActive),
			CreatedAt:        gateway.CreatedAt,
			UpdatedAt:        gateway.UpdatedAt,
		})
	}

	return responses, nil
}

// convertAPIPlatformStatusToModelStatus converts API Platform gateway active status to model status
func convertAPIPlatformStatusToModelStatus(isActive bool) string {
	if isActive {
		return string(models.GatewayStatusActive)
	}
	return string(models.GatewayStatusInactive)
}
