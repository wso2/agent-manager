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

	"github.com/google/uuid"

	occlient "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
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
	logger      *slog.Logger
	ocClient    occlient.OpenChoreoClient
	gatewayRepo repositories.GatewayRepository
}

// NewEnvironmentService creates a new environment service
func NewEnvironmentService(logger *slog.Logger, gatewayRepo repositories.GatewayRepository, ocClient occlient.OpenChoreoClient) EnvironmentService {
	return &environmentService{
		logger:      logger,
		gatewayRepo: gatewayRepo,
		ocClient:    ocClient,
	}
}

func (s *environmentService) CreateEnvironment(ctx context.Context, orgName string, req *models.CreateEnvironmentRequest) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Warn("CreateEnvironment: Environments are managed by OpenChoreo and cannot be created via Agent Manager")
	return nil, fmt.Errorf("environments are managed by OpenChoreo platform and cannot be created directly")
}

func (s *environmentService) GetEnvironment(ctx context.Context, orgName string, envID string) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Getting environment from OpenChoreo", "envID", envID, "orgName", orgName)

	// envID in this context is the environment name (not UUID)
	// since OpenChoreo API uses environment name as identifier
	env, err := s.ocClient.GetEnvironment(ctx, orgName, envID)
	if err != nil {
		s.logger.Error("Failed to get environment from OpenChoreo", "orgName", orgName, "envID", envID, "error", err)
		// Check if it's a not-found error
		if errors.Is(err, utils.ErrEnvironmentNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	// Convert OpenChoreo EnvironmentResponse to GatewayEnvironmentResponse
	return &models.GatewayEnvironmentResponse{
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
	}, nil
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
	s.logger.Warn("UpdateEnvironment: Environments are managed by OpenChoreo and cannot be updated via Agent Manager")
	return nil, fmt.Errorf("environments are managed by OpenChoreo platform and cannot be updated directly")
}

func (s *environmentService) DeleteEnvironment(ctx context.Context, orgName string, envID string) error {
	s.logger.Warn("DeleteEnvironment: Environments are managed by OpenChoreo and cannot be deleted via Agent Manager")
	return fmt.Errorf("environments are managed by OpenChoreo platform and cannot be deleted directly")
}

func (s *environmentService) GetEnvironmentGateways(ctx context.Context, orgName string, envID string) ([]models.GatewayResponse, error) {
	s.logger.Info("Getting environment gateways", "envID", envID, "orgName", orgName)

	// Verify environment exists in OpenChoreo (envID is environment name)
	env, err := s.ocClient.GetEnvironment(ctx, orgName, envID)
	if err != nil {
		s.logger.Error("Failed to get environment from OpenChoreo", "orgName", orgName, "envID", envID, "error", err)
		if errors.Is(err, utils.ErrEnvironmentNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to verify environment: %w", err)
	}

	// Parse environment UUID
	envUUID, err := uuid.Parse(env.UUID)
	if err != nil {
		s.logger.Error("Failed to parse environment UUID", "uuid", env.UUID, "error", err)
		return nil, fmt.Errorf("invalid environment UUID: %w", err)
	}

	// Get gateway-environment mappings from DB
	var mappings []models.GatewayEnvironmentMapping
	err = db.DB(ctx).
		Where("environment_uuid = ?", envUUID).
		Find(&mappings).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway mappings: %w", err)
	}

	// Fetch each gateway from the gateway repository
	responses := make([]models.GatewayResponse, 0, len(mappings))
	for _, mapping := range mappings {
		gatewayID := mapping.GatewayUUID.String()

		// Get gateway details from repository
		gateway, err := s.gatewayRepo.GetByUUID(gatewayID)
		if err != nil {
			s.logger.Warn("Failed to get gateway from repository", "gatewayID", gatewayID, "error", err)
			continue
		}
		if gateway == nil {
			s.logger.Warn("Gateway not found", "gatewayID", gatewayID)
			continue
		}

		// Convert gateway model to response
		status := string(models.GatewayStatusInactive)
		if gateway.IsActive {
			status = string(models.GatewayStatusActive)
		}

		responses = append(responses, models.GatewayResponse{
			UUID:             gateway.UUID.String(),
			OrganizationName: orgName,
			Name:             gateway.Name,
			DisplayName:      gateway.DisplayName,
			GatewayType:      gateway.GatewayFunctionalityType,
			VHost:            gateway.Vhost,
			IsCritical:       gateway.IsCritical,
			Status:           status,
		})
	}

	return responses, nil
}
