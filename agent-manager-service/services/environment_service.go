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
	logger *slog.Logger
}

// NewEnvironmentService creates a new environment service
func NewEnvironmentService(logger *slog.Logger) EnvironmentService {
	return &environmentService{
		logger: logger,
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
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	// Wrap in transaction to handle potential race conditions
	err := db.DB(ctx).Transaction(func(tx *gorm.DB) error {
		// Check if environment already exists within the transaction
		var existing models.Environment
		err := tx.Where("organization_name = ? AND name = ?", req.OrganizationName, req.Name).First(&existing).Error
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
	s.logger.Info("Listing environments", "orgName", orgName, "limit", limit, "offset", offset)

	var environments []models.Environment
	var total int64

	query := db.DB(ctx).Model(&models.Environment{}).Where("organization_name = ?", orgName)

	if err := query.Count(&total).Error; err != nil {
		return nil, fmt.Errorf("failed to count environments: %w", err)
	}

	if err := query.Limit(int(limit)).Offset(int(offset)).Order("created_at DESC").Find(&environments).Error; err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	responses := make([]models.GatewayEnvironmentResponse, len(environments))
	for i, env := range environments {
		responses[i] = *env.ToResponse()
	}

	return &models.EnvironmentListResponse{
		Environments: responses,
		Total:        int32(total),
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

	var gateways []models.Gateway
	err = db.DB(ctx).
		Joins("JOIN gateway_environment_mappings ON gateway_environment_mappings.gateway_uuid = gateways.uuid").
		Where("gateway_environment_mappings.environment_uuid = ?", envUUID).
		Find(&gateways).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get gateways: %w", err)
	}

	responses := make([]models.GatewayResponse, len(gateways))
	for i, gw := range gateways {
		responses[i] = *gw.ToResponse()
	}

	return responses, nil
}
