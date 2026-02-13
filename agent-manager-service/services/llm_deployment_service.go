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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

const (
	deploymentLimitBuffer = 5
	maxDeploymentsPerAPI  = 20
	apiVersionLLMProvider = "gateway.api-platform.wso2.com/v1alpha1"
	kindLLMProvider       = "LLMProvider"
)

// LLMProviderDeploymentService handles LLM deployment business logic
type LLMProviderDeploymentService struct {
	deploymentRepo       repositories.DeploymentRepository
	providerRepo         repositories.LLMProviderRepository
	templateRepo         repositories.LLMProviderTemplateRepository
	gatewayRepo          repositories.GatewayRepository
	gatewayEventsService *GatewayEventsService
}

// NewLLMProviderDeploymentService creates a new LLM deployment service
func NewLLMProviderDeploymentService(
	deploymentRepo repositories.DeploymentRepository,
	providerRepo repositories.LLMProviderRepository,
	templateRepo repositories.LLMProviderTemplateRepository,
	gatewayRepo repositories.GatewayRepository,
	gatewayEventsService *GatewayEventsService,
) *LLMProviderDeploymentService {
	return &LLMProviderDeploymentService{
		deploymentRepo:       deploymentRepo,
		providerRepo:         providerRepo,
		templateRepo:         templateRepo,
		gatewayRepo:          gatewayRepo,
		gatewayEventsService: gatewayEventsService,
	}
}

// LLMProviderDeploymentYAML represents the deployment YAML
type LLMProviderDeploymentYAML struct {
	ApiVersion string                    `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                    `yaml:"kind" json:"kind"`
	Metadata   DeploymentMetadata        `yaml:"metadata" json:"metadata"`
	Spec       LLMProviderDeploymentSpec `yaml:"spec" json:"spec"`
}

// LLMProviderDeploymentSpec represents the spec section
type LLMProviderDeploymentSpec struct {
	DisplayName   string                        `yaml:"displayName" json:"displayName"`
	Version       string                        `yaml:"version" json:"version"`
	Context       string                        `yaml:"context,omitempty" json:"context,omitempty"`
	VHost         string                        `yaml:"vhost,omitempty" json:"vhost,omitempty"`
	Template      string                        `yaml:"template" json:"template"`
	Upstream      models.UpstreamConfig         `yaml:"upstream" json:"upstream"`
	AccessControl *models.LLMAccessControl      `yaml:"accessControl,omitempty" json:"accessControl,omitempty"`
	RateLimiting  *models.LLMRateLimitingConfig `yaml:"rateLimiting,omitempty" json:"rateLimiting,omitempty"`
	Policies      []models.LLMPolicy            `yaml:"policies,omitempty" json:"policies,omitempty"`
	Security      *models.SecurityConfig        `yaml:"security,omitempty" json:"security,omitempty"`
}

// DeployLLMProvider deploys an LLM provider to a gateway
func (s *LLMProviderDeploymentService) DeployLLMProvider(providerID string, req *models.DeployAPIRequest, orgID string) (*models.Deployment, error) {
	slog.Info("LLMProviderDeploymentService.DeployLLMProvider: starting", "providerID", providerID, "orgID", orgID,
		"deploymentName", req.Name, "base", req.Base, "gatewayID", req.GatewayID)

	if req.Base == "" {
		slog.Error("LLMProviderDeploymentService.DeployLLMProvider: base is required", "providerID", providerID)
		return nil, utils.ErrDeploymentBaseRequired
	}
	if req.GatewayID == "" {
		slog.Error("LLMProviderDeploymentService.DeployLLMProvider: gateway ID is required", "providerID", providerID)
		return nil, utils.ErrDeploymentGatewayIDRequired
	}
	if req.Name == "" {
		slog.Error("LLMProviderDeploymentService.DeployLLMProvider: deployment name is required", "providerID", providerID)
		return nil, utils.ErrDeploymentNameRequired
	}

	// Parse UUIDs
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		slog.Error("LLMProviderDeploymentService.DeployLLMProvider: invalid organization UUID", "providerID", providerID, "orgID", orgID, "error", err)
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}
	gatewayUUID, err := uuid.Parse(req.GatewayID)
	if err != nil {
		slog.Error("LLMProviderDeploymentService.DeployLLMProvider: invalid gateway UUID", "providerID", providerID, "gatewayID", req.GatewayID, "error", err)
		return nil, fmt.Errorf("invalid gateway UUID: %w", err)
	}

	// Validate gateway exists
	slog.Info("LLMProviderDeploymentService.DeployLLMProvider: validating gateway", "providerID", providerID, "gatewayID", req.GatewayID)
	gateway, err := s.gatewayRepo.GetByUUID(req.GatewayID)
	if err != nil {
		slog.Error("LLMProviderDeploymentService.DeployLLMProvider: failed to get gateway", "providerID", providerID, "gatewayID", req.GatewayID, "error", err)
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}
	if gateway == nil || gateway.OrganizationUUID.String() != orgID {
		slog.Warn("LLMProviderDeploymentService.DeployLLMProvider: gateway not found or org mismatch", "providerID", providerID, "gatewayID", req.GatewayID, "orgID", orgID)
		return nil, utils.ErrGatewayNotFound
	}

	// Get LLM provider
	slog.Info("LLMProviderDeploymentService.DeployLLMProvider: getting provider", "providerID", providerID, "orgID", orgID)
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		slog.Error("LLMProviderDeploymentService.DeployLLMProvider: failed to get provider", "providerID", providerID, "orgID", orgID, "error", err)
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		slog.Warn("LLMProviderDeploymentService.DeployLLMProvider: provider not found", "providerID", providerID, "orgID", orgID)
		return nil, utils.ErrLLMProviderNotFound
	}

	slog.Info("LLMProviderDeploymentService.DeployLLMProvider: provider retrieved", "providerID", providerID, "providerUUID", provider.UUID)

	var baseDeploymentID *uuid.UUID
	var contentBytes []byte

	// Determine source: "current" or existing deployment
	if req.Base == "current" {
		slog.Info("LLMProviderDeploymentService.DeployLLMProvider: using current provider configuration", "providerID", providerID)

		// Parse model providers from ModelList
		if provider.ModelList != "" {
			slog.Info("LLMProviderDeploymentService.DeployLLMProvider: parsing model providers", "providerID", providerID)
			if err := json.Unmarshal([]byte(provider.ModelList), &provider.ModelProviders); err != nil {
				slog.Error("LLMProviderDeploymentService.DeployLLMProvider: failed to parse model providers", "providerID", providerID, "error", err)
				return nil, fmt.Errorf("failed to parse model providers: %w", err)
			}
		}

		// Generate deployment YAML
		slog.Info("LLMProviderDeploymentService.DeployLLMProvider: generating deployment YAML", "providerID", providerID)
		deploymentYAML, err := s.generateLLMProviderDeploymentYAML(provider, orgID)
		if err != nil {
			slog.Error("LLMProviderDeploymentService.DeployLLMProvider: failed to generate deployment YAML", "providerID", providerID, "error", err)
			return nil, fmt.Errorf("failed to generate deployment YAML: %w", err)
		}
		contentBytes = []byte(deploymentYAML)
	} else {
		slog.Info("LLMProviderDeploymentService.DeployLLMProvider: using existing deployment as base", "providerID", providerID, "baseDeploymentID", req.Base)

		// Use existing deployment as base
		baseUUID, err := uuid.Parse(req.Base)
		if err != nil {
			slog.Error("LLMProviderDeploymentService.DeployLLMProvider: invalid base deployment ID", "providerID", providerID, "baseDeploymentID", req.Base, "error", err)
			return nil, fmt.Errorf("invalid base deployment ID: %w", err)
		}

		baseDeployment, err := s.deploymentRepo.GetWithContent(req.Base, provider.UUID.String(), orgID)
		if err != nil {
			slog.Warn("LLMProviderDeploymentService.DeployLLMProvider: base deployment not found", "providerID", providerID, "baseDeploymentID", req.Base, "error", err)
			return nil, utils.ErrBaseDeploymentNotFound
		}
		contentBytes = baseDeployment.Content
		baseDeploymentID = &baseUUID
		slog.Info("LLMProviderDeploymentService.DeployLLMProvider: base deployment retrieved", "providerID", providerID, "baseDeploymentID", req.Base)
	}

	// Create deployment
	deploymentID := uuid.New()
	deployed := models.DeploymentStatusDeployed

	slog.Info("LLMProviderDeploymentService.DeployLLMProvider: creating deployment", "providerID", providerID,
		"deploymentID", deploymentID, "deploymentName", req.Name, "gatewayID", req.GatewayID)

	deployment := &models.Deployment{
		DeploymentID:     deploymentID,
		Name:             req.Name,
		ArtifactUUID:     provider.UUID,
		OrganizationUUID: orgUUID,
		GatewayUUID:      gatewayUUID,
		BaseDeploymentID: baseDeploymentID,
		Content:          contentBytes,
		Metadata:         req.Metadata,
		Status:           &deployed,
	}

	hardLimit := maxDeploymentsPerAPI + deploymentLimitBuffer
	if err := s.deploymentRepo.CreateWithLimitEnforcement(deployment, hardLimit); err != nil {
		slog.Error("LLMProviderDeploymentService.DeployLLMProvider: failed to create deployment", "providerID", providerID, "deploymentID", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	slog.Info("LLMProviderDeploymentService.DeployLLMProvider: deployment created successfully", "providerID", providerID, "deploymentID", deploymentID)

	// Broadcast deployment event to gateway
	deploymentEvent := &models.LLMProviderDeploymentEvent{
		ProviderID:     providerID,
		GatewayID:      req.GatewayID,
		OrganizationID: orgID,
		Status:         string(models.DeploymentStatusDeployed),
	}
	if err := s.gatewayEventsService.BroadcastLLMProviderDeploymentEvent(req.GatewayID, deploymentEvent); err != nil {
		slog.Error("LLMProviderDeploymentService.DeployLLMProvider: failed to broadcast deployment event",
			"providerID", providerID, "deploymentID", deploymentID, "gatewayID", req.GatewayID, "error", err)
		// Don't fail the deployment if broadcast fails - deployment is already persisted
	} else {
		slog.Info("LLMProviderDeploymentService.DeployLLMProvider: deployment event broadcast successfully",
			"providerID", providerID, "deploymentID", deploymentID, "gatewayID", req.GatewayID)
	}

	return deployment, nil
}

// UndeployLLMProviderDeployment undeploys a deployment
func (s *LLMProviderDeploymentService) UndeployLLMProviderDeployment(providerID, deploymentID, gatewayID, orgID string) (*models.Deployment, error) {
	slog.Info("LLMProviderDeploymentService.UndeployLLMProviderDeployment: starting", "providerID", providerID,
		"deploymentID", deploymentID, "gatewayID", gatewayID, "orgID", orgID)

	// Get provider
	slog.Info("LLMProviderDeploymentService.UndeployLLMProviderDeployment: getting provider", "providerID", providerID, "orgID", orgID)
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		slog.Error("LLMProviderDeploymentService.UndeployLLMProviderDeployment: failed to get provider", "providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		slog.Warn("LLMProviderDeploymentService.UndeployLLMProviderDeployment: provider not found", "providerID", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Get deployment
	slog.Info("LLMProviderDeploymentService.UndeployLLMProviderDeployment: getting deployment", "providerID", providerID, "deploymentID", deploymentID)
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, provider.UUID.String(), orgID)
	if err != nil {
		slog.Error("LLMProviderDeploymentService.UndeployLLMProviderDeployment: failed to get deployment", "providerID", providerID, "deploymentID", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		slog.Warn("LLMProviderDeploymentService.UndeployLLMProviderDeployment: deployment not found", "providerID", providerID, "deploymentID", deploymentID)
		return nil, utils.ErrDeploymentNotFound
	}
	if deployment.GatewayUUID.String() != gatewayID {
		slog.Error("LLMProviderDeploymentService.UndeployLLMProviderDeployment: gateway ID mismatch", "providerID", providerID,
			"deploymentID", deploymentID, "expectedGatewayID", gatewayID, "actualGatewayID", deployment.GatewayUUID.String())
		return nil, utils.ErrGatewayIDMismatch
	}
	if deployment.Status == nil || *deployment.Status != models.DeploymentStatusDeployed {
		slog.Warn("LLMProviderDeploymentService.UndeployLLMProviderDeployment: deployment not active", "providerID", providerID,
			"deploymentID", deploymentID, "status", deployment.Status)
		return nil, utils.ErrDeploymentNotActive
	}

	// Update status to undeployed
	slog.Info("LLMProviderDeploymentService.UndeployLLMProviderDeployment: setting status to undeployed", "providerID", providerID, "deploymentID", deploymentID)
	updatedAt, err := s.deploymentRepo.SetCurrent(provider.UUID.String(), orgID, gatewayID, deploymentID, models.DeploymentStatusUndeployed)
	if err != nil {
		slog.Error("LLMProviderDeploymentService.UndeployLLMProviderDeployment: failed to undeploy", "providerID", providerID, "deploymentID", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to undeploy: %w", err)
	}

	undeployed := models.DeploymentStatusUndeployed
	deployment.Status = &undeployed
	deployment.UpdatedAt = &updatedAt

	slog.Info("LLMProviderDeploymentService.UndeployLLMProviderDeployment: undeployed successfully", "providerID", providerID, "deploymentID", deploymentID)

	// Broadcast undeployment event to gateway
	undeploymentEvent := &models.LLMProviderUndeploymentEvent{
		ProviderID:     providerID,
		GatewayID:      gatewayID,
		OrganizationID: orgID,
	}
	if err := s.gatewayEventsService.BroadcastLLMProviderUndeploymentEvent(gatewayID, undeploymentEvent); err != nil {
		slog.Error("LLMProviderDeploymentService.UndeployLLMProviderDeployment: failed to broadcast undeployment event",
			"providerID", providerID, "deploymentID", deploymentID, "gatewayID", gatewayID, "error", err)
		// Don't fail the undeployment if broadcast fails - status is already updated
	} else {
		slog.Info("LLMProviderDeploymentService.UndeployLLMProviderDeployment: undeployment event broadcast successfully",
			"providerID", providerID, "deploymentID", deploymentID, "gatewayID", gatewayID)
	}

	return deployment, nil
}

// RestoreLLMProviderDeployment restores a previous deployment
func (s *LLMProviderDeploymentService) RestoreLLMProviderDeployment(providerID, deploymentID, gatewayID, orgID string) (*models.Deployment, error) {
	// Get provider
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}

	// Get target deployment
	deployment, err := s.deploymentRepo.GetWithContent(deploymentID, provider.UUID.String(), orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrDeploymentNotFound
	}
	if deployment.GatewayUUID.String() != gatewayID {
		return nil, utils.ErrGatewayIDMismatch
	}

	// Check if already deployed
	currentDeploymentID, status, _, err := s.deploymentRepo.GetStatus(provider.UUID.String(), orgID, gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment status: %w", err)
	}
	if currentDeploymentID == deploymentID && status == models.DeploymentStatusDeployed {
		return nil, utils.ErrDeploymentAlreadyDeployed
	}

	// Update status to deployed
	updatedAt, err := s.deploymentRepo.SetCurrent(provider.UUID.String(), orgID, gatewayID, deploymentID, models.DeploymentStatusDeployed)
	if err != nil {
		return nil, fmt.Errorf("failed to restore deployment: %w", err)
	}

	deployed := models.DeploymentStatusDeployed
	deployment.Status = &deployed
	deployment.UpdatedAt = &updatedAt

	// Broadcast deployment event to gateway (restore is treated as a deployment)
	deploymentEvent := &models.LLMProviderDeploymentEvent{
		ProviderID:     providerID,
		GatewayID:      gatewayID,
		OrganizationID: orgID,
		Status:         string(models.DeploymentStatusDeployed),
	}
	if err := s.gatewayEventsService.BroadcastLLMProviderDeploymentEvent(gatewayID, deploymentEvent); err != nil {
		slog.Error("LLMProviderDeploymentService.RestoreLLMProviderDeployment: failed to broadcast deployment event",
			"providerID", providerID, "deploymentID", deploymentID, "gatewayID", gatewayID, "error", err)
		// Don't fail the restore if broadcast fails - status is already updated
	} else {
		slog.Info("LLMProviderDeploymentService.RestoreLLMProviderDeployment: deployment event broadcast successfully",
			"providerID", providerID, "deploymentID", deploymentID, "gatewayID", gatewayID)
	}

	return deployment, nil
}

// GetLLMProviderDeployments retrieves all deployments for a provider
func (s *LLMProviderDeploymentService) GetLLMProviderDeployments(providerID, orgID string, gatewayID *string, status *string) ([]*models.Deployment, error) {
	// Get provider
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}

	// Validate status if provided
	if status != nil {
		validStatuses := map[string]bool{
			string(models.DeploymentStatusDeployed):   true,
			string(models.DeploymentStatusUndeployed): true,
			string(models.DeploymentStatusArchived):   true,
		}
		if !validStatuses[*status] {
			return nil, utils.ErrInvalidDeploymentStatus
		}
	}

	// Get deployments
	deployments, err := s.deploymentRepo.GetDeploymentsWithState(provider.UUID.String(), orgID, gatewayID, status, maxDeploymentsPerAPI)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployments: %w", err)
	}

	return deployments, nil
}

// GetLLMProviderDeployment retrieves a specific deployment
func (s *LLMProviderDeploymentService) GetLLMProviderDeployment(providerID, deploymentID, orgID string) (*models.Deployment, error) {
	// Get provider
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}

	// Get deployment
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, provider.UUID.String(), orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrDeploymentNotFound
	}

	return deployment, nil
}

// DeleteLLMProviderDeployment deletes a deployment
func (s *LLMProviderDeploymentService) DeleteLLMProviderDeployment(providerID, deploymentID, orgID string) error {
	// Get provider
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return utils.ErrLLMProviderNotFound
	}

	// Get deployment
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, provider.UUID.String(), orgID)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return utils.ErrDeploymentNotFound
	}
	if deployment.Status != nil && *deployment.Status == models.DeploymentStatusDeployed {
		return utils.ErrDeploymentIsDeployed
	}

	// Delete deployment
	if err := s.deploymentRepo.Delete(deploymentID, provider.UUID.String(), orgID); err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	return nil
}

// generateLLMProviderDeploymentYAML generates deployment YAML for an LLM provider
func (s *LLMProviderDeploymentService) generateLLMProviderDeploymentYAML(provider *models.LLMProvider, orgID string) (string, error) {
	if provider == nil {
		return "", errors.New("provider is required")
	}
	if provider.Configuration.Upstream == nil {
		return "", utils.ErrInvalidInput
	}

	// Get template handle
	if provider.TemplateUUID == uuid.Nil {
		return "", utils.ErrLLMProviderTemplateNotFound
	}

	template, err := s.templateRepo.GetByUUID(provider.TemplateUUID.String(), orgID)
	if err != nil {
		return "", fmt.Errorf("failed to get template: %w", err)
	}
	if template == nil {
		return "", utils.ErrLLMProviderTemplateNotFound
	}

	// Set default context if not provided
	contextValue := "/"
	if provider.Configuration.Context != nil && *provider.Configuration.Context != "" {
		contextValue = *provider.Configuration.Context
	}

	vhostValue := ""
	if provider.Configuration.VHost != nil {
		vhostValue = *provider.Configuration.VHost
	}

	// Build deployment YAML
	deploymentYAML := LLMProviderDeploymentYAML{
		ApiVersion: apiVersionLLMProvider,
		Kind:       kindLLMProvider,
		Metadata: DeploymentMetadata{
			Name: provider.Artifact.Handle,
		},
		Spec: LLMProviderDeploymentSpec{
			DisplayName:   provider.Configuration.Name,
			Version:       provider.Configuration.Version,
			Context:       contextValue,
			VHost:         vhostValue,
			Template:      template.Handle,
			Upstream:      *provider.Configuration.Upstream,
			AccessControl: provider.Configuration.AccessControl,
			RateLimiting:  provider.Configuration.RateLimiting,
			Policies:      provider.Configuration.Policies,
			Security:      provider.Configuration.Security,
		},
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(deploymentYAML)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to YAML: %w", err)
	}

	return string(yamlBytes), nil
}
