/*
 *  Copyright (c) 2026, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package services

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
	"gopkg.in/yaml.v3"
)

const (
	deploymentLimitBuffer = 5
	maxDeploymentsPerAPI  = 20
	apiVersionLLMProvider = "gateway.api-platform.wso2.com/v1alpha1"
	kindLLMProvider       = "LLMProvider"
)

// LLMProviderDeploymentService handles LLM deployment business logic
type LLMProviderDeploymentService struct {
	deploymentRepo repositories.DeploymentRepository
	providerRepo   repositories.LLMProviderRepository
	templateRepo   repositories.LLMProviderTemplateRepository
	gatewayRepo    repositories.GatewayRepository
}

// NewLLMProviderDeploymentService creates a new LLM deployment service
func NewLLMProviderDeploymentService(
	deploymentRepo repositories.DeploymentRepository,
	providerRepo repositories.LLMProviderRepository,
	templateRepo repositories.LLMProviderTemplateRepository,
	gatewayRepo repositories.GatewayRepository,
) *LLMProviderDeploymentService {
	return &LLMProviderDeploymentService{
		deploymentRepo: deploymentRepo,
		providerRepo:   providerRepo,
		templateRepo:   templateRepo,
		gatewayRepo:    gatewayRepo,
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
	if req.Base == "" {
		return nil, utils.ErrDeploymentBaseRequired
	}
	if req.GatewayID == "" {
		return nil, utils.ErrDeploymentGatewayIDRequired
	}
	if req.Name == "" {
		return nil, utils.ErrDeploymentNameRequired
	}

	// Parse UUIDs
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}
	gatewayUUID, err := uuid.Parse(req.GatewayID)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway UUID: %w", err)
	}

	// Validate gateway exists
	gateway, err := s.gatewayRepo.GetByUUID(req.GatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}
	if gateway == nil || gateway.OrganizationUUID.String() != orgID {
		return nil, utils.ErrGatewayNotFound
	}

	// Get LLM provider
	provider, err := s.providerRepo.GetByID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}

	var baseDeploymentID *uuid.UUID
	var contentBytes []byte

	// Determine source: "current" or existing deployment
	if req.Base == "current" {
		// Parse model providers from ModelList
		if provider.ModelList != "" {
			if err := json.Unmarshal([]byte(provider.ModelList), &provider.ModelProviders); err != nil {
				return nil, fmt.Errorf("failed to parse model providers: %w", err)
			}
		}

		// Generate deployment YAML
		deploymentYAML, err := s.generateLLMProviderDeploymentYAML(provider, orgID)
		if err != nil {
			return nil, fmt.Errorf("failed to generate deployment YAML: %w", err)
		}
		contentBytes = []byte(deploymentYAML)
	} else {
		// Use existing deployment as base
		baseUUID, err := uuid.Parse(req.Base)
		if err != nil {
			return nil, fmt.Errorf("invalid base deployment ID: %w", err)
		}

		baseDeployment, err := s.deploymentRepo.GetWithContent(req.Base, provider.UUID.String(), orgID)
		if err != nil {
			return nil, utils.ErrBaseDeploymentNotFound
		}
		contentBytes = baseDeployment.Content
		baseDeploymentID = &baseUUID
	}

	// Create deployment
	deploymentID := uuid.New()
	deployed := models.DeploymentStatusDeployed

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
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	return deployment, nil
}

// UndeployLLMProviderDeployment undeploys a deployment
func (s *LLMProviderDeploymentService) UndeployLLMProviderDeployment(providerID, deploymentID, gatewayID, orgID string) (*models.Deployment, error) {
	// Get provider
	provider, err := s.providerRepo.GetByID(providerID, orgID)
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
	if deployment.GatewayUUID.String() != gatewayID {
		return nil, utils.ErrGatewayIDMismatch
	}
	if deployment.Status == nil || *deployment.Status != models.DeploymentStatusDeployed {
		return nil, utils.ErrDeploymentNotActive
	}

	// Update status to undeployed
	updatedAt, err := s.deploymentRepo.SetCurrent(provider.UUID.String(), orgID, gatewayID, deploymentID, models.DeploymentStatusUndeployed)
	if err != nil {
		return nil, fmt.Errorf("failed to undeploy: %w", err)
	}

	undeployed := models.DeploymentStatusUndeployed
	deployment.Status = &undeployed
	deployment.UpdatedAt = &updatedAt

	return deployment, nil
}

// RestoreLLMProviderDeployment restores a previous deployment
func (s *LLMProviderDeploymentService) RestoreLLMProviderDeployment(providerID, deploymentID, gatewayID, orgID string) (*models.Deployment, error) {
	// Get provider
	provider, err := s.providerRepo.GetByID(providerID, orgID)
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

	return deployment, nil
}

// GetLLMProviderDeployments retrieves all deployments for a provider
func (s *LLMProviderDeploymentService) GetLLMProviderDeployments(providerID, orgID string, gatewayID *string, status *string) ([]*models.Deployment, error) {
	// Get provider
	provider, err := s.providerRepo.GetByID(providerID, orgID)
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
	provider, err := s.providerRepo.GetByID(providerID, orgID)
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
	provider, err := s.providerRepo.GetByID(providerID, orgID)
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
