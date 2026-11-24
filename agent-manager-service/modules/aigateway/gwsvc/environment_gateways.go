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

package gwsvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
)

// EnvironmentGatewayService lists the gateways that serve an environment.
type EnvironmentGatewayService struct {
	logger      *slog.Logger
	ocClient    modkit.CoreAPI
	gatewayRepo gwrepo.GatewayRepository
}

// NewEnvironmentGatewayService creates the service.
func NewEnvironmentGatewayService(logger *slog.Logger, gatewayRepo gwrepo.GatewayRepository, ocClient modkit.CoreAPI) *EnvironmentGatewayService {
	return &EnvironmentGatewayService{logger: logger, ocClient: ocClient, gatewayRepo: gatewayRepo}
}

// GetEnvironmentGateways lists the gateways mapped to the environment.
func (s *EnvironmentGatewayService) GetEnvironmentGateways(ctx context.Context, ouID string, envID string) ([]gwmodels.GatewayResponse, error) {
	s.logger.Info("Getting environment gateways", "envID", envID, "ouID", ouID)

	// Verify environment exists (envID is environment name)
	env, err := LookupEnvironment(ctx, s.ocClient, ouID, envID)
	if err != nil {
		s.logger.Error("Failed to get environment", "ouID", ouID, "envID", envID, "error", err)
		if errors.Is(err, gwerr.ErrEnvironmentNotFound) {
			return nil, gwerr.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to verify environment: %w", err)
	}

	// Parse environment UUID
	envUUID, err := uuid.Parse(env.ID)
	if err != nil {
		s.logger.Error("Failed to parse environment UUID", "uuid", env.ID, "error", err)
		return nil, fmt.Errorf("invalid environment UUID: %w", err)
	}

	// Get gateway-environment mappings from repository
	mappings, err := s.gatewayRepo.GetEnvironmentMappingsByEnvironmentID(envUUID.String())
	if err != nil {
		s.logger.Error("Failed to get gateway mappings from repository", "environmentID", envUUID.String(), "error", err)
		return nil, fmt.Errorf("failed to get gateway mappings: %w", err)
	}

	// Fetch each gateway from the gateway repository
	responses := make([]gwmodels.GatewayResponse, 0, len(mappings))
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
		status := string(gwmodels.GatewayStatusInactive)
		if gateway.IsActive {
			status = string(gwmodels.GatewayStatusActive)
		}

		responses = append(responses, gwmodels.GatewayResponse{
			UUID:             gateway.UUID.String(),
			OrganizationName: ouID,
			Name:             gateway.Name,
			DisplayName:      gateway.DisplayName,
			GatewayType:      gateway.GatewayFunctionalityType,
			Hosting:          gatewayHostingWire(gateway),
			VHost:            gateway.Vhost,
			IsCritical:       gateway.IsCritical,
			Status:           status,
			CreatedAt:        gateway.CreatedAt,
			UpdatedAt:        gateway.UpdatedAt,
		})
	}

	return responses, nil
}
