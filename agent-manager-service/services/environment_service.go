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
	"sync"

	"github.com/google/uuid"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// EnvironmentService defines the interface for environment operations
type EnvironmentService interface {
	GetEnvironment(ctx context.Context, ouID string, envID string) (*models.GatewayEnvironmentResponse, error)
	ListEnvironments(ctx context.Context, ouID string, limit, offset int32) (*models.EnvironmentListResponse, error)
	GetEnvironmentGateways(ctx context.Context, ouID string, envID string) ([]models.GatewayResponse, error)
	ListThunderInstances(ctx context.Context, ouID string) (*models.ThunderInstanceListResponse, error)
}

type environmentService struct {
	logger        *slog.Logger
	ocClient      repositories.PlatformRepository
	gatewayRepo   repositories.GatewayRepository
	thunderProber thundersvc.Prober
}

// NewEnvironmentService creates a new environment service
func NewEnvironmentService(logger *slog.Logger, gatewayRepo repositories.GatewayRepository, ocClient repositories.PlatformRepository, thunderProber thundersvc.Prober) EnvironmentService {
	return &environmentService{
		logger:        logger,
		gatewayRepo:   gatewayRepo,
		ocClient:      ocClient,
		thunderProber: thunderProber,
	}
}

func (s *environmentService) GetEnvironment(ctx context.Context, ouID string, envID string) (*models.GatewayEnvironmentResponse, error) {
	s.logger.Info("Getting environment", "envID", envID, "ouID", ouID)

	// There is only the default environment; envID is accepted and ignored.
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		s.logger.Error("Failed to get environment", "ouID", ouID, "envID", envID, "error", err)
		// Check if it's a not-found error
		if errors.Is(err, utils.ErrEnvironmentNotFound) {
			return nil, utils.ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}

	return &models.GatewayEnvironmentResponse{
		UUID:             env.UUID,
		OrganizationName: ouID,
		Name:             env.Name,
		DisplayName:      env.DisplayName,
		Description:      env.Description,
		IsolationTier:    env.IsolationTier,
		DataplaneRef:     env.DataplaneRef,
		DNSPrefix:        env.DNSPrefix,
		IsProduction:     env.IsProduction,
		Gateway:          env.Gateway,
		CreatedAt:        env.CreatedAt,
		UpdatedAt:        env.CreatedAt,
	}, nil
}

func (s *environmentService) ListEnvironments(ctx context.Context, ouID string, limit, offset int32) (*models.EnvironmentListResponse, error) {
	s.logger.Info("Listing environments", "ouID", ouID, "limit", limit, "offset", offset)

	ocEnvironments, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		s.logger.Error("Failed to list environments", "ouID", ouID, "error", err)
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

	responses := make([]models.GatewayEnvironmentResponse, len(paginatedEnvs))
	for i, env := range paginatedEnvs {
		responses[i] = models.GatewayEnvironmentResponse{
			UUID:             env.UUID,
			OrganizationName: ouID,
			Name:             env.Name,
			DisplayName:      env.DisplayName,
			Description:      env.Description,
			IsolationTier:    env.IsolationTier,
			DataplaneRef:     env.DataplaneRef,
			DNSPrefix:        env.DNSPrefix,
			IsProduction:     env.IsProduction,
			Gateway:          env.Gateway,
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

func (s *environmentService) GetEnvironmentGateways(ctx context.Context, ouID string, envID string) ([]models.GatewayResponse, error) {
	s.logger.Info("Getting environment gateways", "envID", envID, "ouID", ouID)

	// Verify environment exists (envID is environment name)
	env, err := s.ocClient.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		s.logger.Error("Failed to get environment", "ouID", ouID, "envID", envID, "error", err)
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

	// Get gateway-environment mappings from repository
	mappings, err := s.gatewayRepo.GetEnvironmentMappingsByEnvironmentID(envUUID.String())
	if err != nil {
		s.logger.Error("Failed to get gateway mappings from repository", "environmentID", envUUID.String(), "error", err)
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
			OrganizationName: ouID,
			Name:             gateway.Name,
			DisplayName:      gateway.DisplayName,
			GatewayType:      gateway.GatewayFunctionalityType,
			Hosting:          gatewayHostingWire(gateway),
			VHost:            gateway.Vhost,
			IsCritical:       gateway.IsCritical,
			Status:           status,
		})
	}

	return responses, nil
}

// ListThunderInstances returns the Thunder OAuth2 identity provider info for every
// environment in the org whose Thunder instance is reachable: the platform
// Thunder for the default environment, when a live probe of its JWKS endpoint
// answers, so the console never advertises dead issuer/token/JWKS URLs.
//
// maxThunderProbeConcurrency bounds how many probes run at once.
const maxThunderProbeConcurrency = 10

func (s *environmentService) ListThunderInstances(ctx context.Context, ouID string) (*models.ThunderInstanceListResponse, error) {
	envs, err := s.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("list environments for org %s: %w", ouID, err)
	}

	// Thunder naming (release name, namespace, host, issuer URL) is keyed by the
	// org NAME the provisioning scripts deployed with (ORG_NAME, e.g. "default"),
	// which is the org namespace — not the OU id from the JWT. Probing or
	// advertising URLs derived from the OU id would point at instances that don't
	// exist.
	orgNamespace := ThunderOrgNamespace()

	// Only the default environment has a Thunder: the platform one. Probes are
	// fanned out (bounded by maxThunderProbeConcurrency) since each can take a
	// couple of seconds in the worst case.
	reachable := make([]bool, len(envs))
	thunderURLs := make([]string, len(envs))
	sem := make(chan struct{}, maxThunderProbeConcurrency)
	var wg sync.WaitGroup
	for i, env := range envs {
		if env == nil || env.Name == "" {
			continue
		}
		thunderURLs[i] = platformThunderURL(env.Name)
		if thunderURLs[i] == "" {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, envName string) {
			defer wg.Done()
			defer func() { <-sem }()
			reachable[idx] = s.thunderProber.Probe(ctx, orgNamespace, envName, thunderURLs[idx], true)
		}(i, env.Name)
	}
	wg.Wait()

	instances := make([]models.ThunderInstanceResponse, 0, len(envs))
	for i, env := range envs {
		if env == nil || env.Name == "" {
			continue
		}

		// Reachability is the only reliable signal: environments created with PROVISION_THUNDER=false,
		// environments whose provisioning failed silently (non-fatal by design), and
		// environments with no registered handle all pass the gateway-mappings check but
		// have no Thunder instance.
		if !reachable[i] {
			s.logger.Debug("env-Thunder not reachable, skipping", "envName", env.Name)
			continue
		}

		// All three URLs must be externally reachable — this response is developer-facing
		// (console Identity page, copy-buttons). The internal svc.cluster.local addresses
		// only resolve inside the cluster and would cause DNS failures for developers.
		instances = append(instances, models.ThunderInstanceResponse{
			EnvName:      env.Name,
			DisplayName:  env.DisplayName,
			IsProduction: env.IsProduction,
			IssuerURL:    thundersvc.ThunderIssuerURL(thunderURLs[i]),
			TokenURL:     thundersvc.ThunderExternalTokenURL(thunderURLs[i]),
			JWKSURL:      thundersvc.ThunderExternalJWKSURL(thunderURLs[i]),
			Namespace:    thundersvc.ThunderNamespace(orgNamespace, env.Name),
		})
	}
	return &models.ThunderInstanceListResponse{ThunderInstances: instances}, nil
}
