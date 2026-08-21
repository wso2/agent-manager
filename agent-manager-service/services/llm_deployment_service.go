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
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	deploymentLimitBuffer = 5
	maxDeploymentsPerAPI  = 20
	apiVersionLLMProvider = "gateway.api-platform.wso2.com/v1"
	kindLLMProvider       = "LlmProvider"

	// Policy names and versions
	tokenBasedRateLimitPolicyName = "token-based-ratelimit"
	advancedRateLimitPolicyName   = "advanced-ratelimit"
	costBasedRateLimitPolicyName  = "llm-cost-based-ratelimit"
	llmCostPolicyName             = "llm-cost"
	apiKeyAuthPolicyName          = "api-key-auth"
	rateLimitPolicyVersion        = "v1"
	apiKeyAuthPolicyVersion       = "v1"
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
	Upstream      GatewayUpstream               `yaml:"upstream" json:"upstream"`
	Resilience    *models.Resilience            `yaml:"resilience,omitempty" json:"resilience,omitempty"`
	AccessControl *models.LLMAccessControl      `yaml:"accessControl,omitempty" json:"accessControl,omitempty"`
	RateLimiting  *models.LLMRateLimitingConfig `yaml:"rateLimiting,omitempty" json:"rateLimiting,omitempty"`
	Policies      []models.LLMPolicy            `yaml:"policies,omitempty" json:"policies,omitempty"`
	Security      *models.SecurityConfig        `yaml:"security,omitempty" json:"security,omitempty"`
}

// GatewayUpstream represents the flat upstream structure expected by the gateway
type GatewayUpstream struct {
	URL  string               `yaml:"url,omitempty" json:"url,omitempty"`
	Ref  string               `yaml:"ref,omitempty" json:"ref,omitempty"`
	Auth *models.UpstreamAuth `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// resolveProvider looks up a provider by UUID or handle.
func (s *LLMProviderDeploymentService) resolveProvider(identifier, ouID string) (*models.LLMProvider, error) {
	if _, err := uuid.Parse(identifier); err == nil {
		return s.providerRepo.GetByUUID(identifier, ouID)
	}
	return s.providerRepo.GetByHandle(identifier, ouID)
}

// DeployLLMProvider deploys an LLM provider to a gateway
func (s *LLMProviderDeploymentService) DeployLLMProvider(ctx context.Context, providerID string, req *models.DeployAPIRequest, ouID string) (*models.Deployment, error) {
	logger.GetLogger(ctx).Info("LLMProviderDeploymentService.DeployLLMProvider: starting", "provider_id", providerID, "ou_id", ouID,
		"deployment_name", req.Name, "base", req.Base, "gateway_id", req.GatewayID)

	if req.Base == "" {
		logger.GetLogger(ctx).Error("LLMProviderDeploymentService.DeployLLMProvider: base is required", "provider_id", providerID)
		return nil, utils.ErrDeploymentBaseRequired
	}
	if req.GatewayID == "" {
		logger.GetLogger(ctx).Error("LLMProviderDeploymentService.DeployLLMProvider: gateway ID is required", "provider_id", providerID)
		return nil, utils.ErrDeploymentGatewayIDRequired
	}
	if req.Name == "" {
		logger.GetLogger(ctx).Error("LLMProviderDeploymentService.DeployLLMProvider: deployment name is required", "provider_id", providerID)
		return nil, utils.ErrDeploymentNameRequired
	}

	gatewayUUID, err := uuid.Parse(req.GatewayID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.DeployLLMProvider: invalid gateway UUID", "provider_id", providerID, "gateway_id", req.GatewayID, "error", err)
		return nil, fmt.Errorf("invalid gateway UUID: %w", err)
	}

	// Validate gateway exists
	logger.GetLogger(ctx).Info("LLMProviderDeploymentService.DeployLLMProvider: validating gateway", "provider_id", providerID, "gateway_id", req.GatewayID)
	gateway, err := s.gatewayRepo.GetByUUID(req.GatewayID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.DeployLLMProvider: failed to get gateway", "provider_id", providerID, "gateway_id", req.GatewayID, "error", err)
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}
	if gateway == nil || gateway.OUID != ouID {
		logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.DeployLLMProvider: gateway not found or org mismatch", "provider_id", providerID, "gateway_id", req.GatewayID, "ou_id", ouID)
		return nil, utils.ErrGatewayNotFound
	}

	// Get LLM provider
	logger.GetLogger(ctx).Info("LLMProviderDeploymentService.DeployLLMProvider: getting provider", "provider_id", providerID, "ou_id", ouID)
	provider, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.DeployLLMProvider: failed to get provider", "provider_id", providerID, "ou_id", ouID, "error", err)
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.DeployLLMProvider: provider not found", "provider_id", providerID, "ou_id", ouID)
		return nil, utils.ErrLLMProviderNotFound
	}

	logger.GetLogger(ctx).Info("LLMProviderDeploymentService.DeployLLMProvider: provider retrieved", "provider_id", providerID, "provider_uuid", provider.UUID)

	existing, err := s.deploymentRepo.GetDeployedGatewaysByProvider(provider.UUID, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing provider deployments: %w", err)
	}
	if err := validateEgressPlacement(s.gatewayRepo, gateway, existing); err != nil {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
	}

	var baseDeploymentID *uuid.UUID
	var contentBytes []byte

	// Determine source: "current" or existing deployment
	if req.Base == "current" {
		logger.GetLogger(ctx).Info("LLMProviderDeploymentService.DeployLLMProvider: using current provider configuration", "provider_id", providerID)

		// Parse model providers from ModelList
		if provider.ModelList != "" {
			logger.GetLogger(ctx).Info("LLMProviderDeploymentService.DeployLLMProvider: parsing model providers", "provider_id", providerID)
			if err := json.Unmarshal([]byte(provider.ModelList), &provider.ModelProviders); err != nil {
				logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.DeployLLMProvider: failed to parse model providers", "provider_id", providerID, "error", err)
				return nil, fmt.Errorf("failed to parse model providers: %w", err)
			}
		}

		// Generate deployment YAML
		logger.GetLogger(ctx).Info("LLMProviderDeploymentService.DeployLLMProvider: generating deployment YAML", "provider_id", providerID)
		deploymentYAML, err := s.generateLLMProviderDeploymentYAML(provider, ouID)
		if err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.DeployLLMProvider: failed to generate deployment YAML", "provider_id", providerID, "error", err)
			return nil, fmt.Errorf("failed to generate deployment YAML: %w", err)
		}
		contentBytes = []byte(deploymentYAML)
	} else {
		logger.GetLogger(ctx).Info("LLMProviderDeploymentService.DeployLLMProvider: using existing deployment as base", "provider_id", providerID, "base_deployment_id", req.Base)

		// Use existing deployment as base
		baseUUID, err := uuid.Parse(req.Base)
		if err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.DeployLLMProvider: invalid base deployment ID", "provider_id", providerID, "base_deployment_id", req.Base, "error", err)
			return nil, fmt.Errorf("invalid base deployment ID: %w", err)
		}

		baseDeployment, err := s.deploymentRepo.GetWithContent(req.Base, provider.UUID.String(), ouID)
		if err != nil {
			logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.DeployLLMProvider: base deployment not found", "provider_id", providerID, "base_deployment_id", req.Base, "error", err)
			return nil, utils.ErrBaseDeploymentNotFound
		}
		contentBytes = baseDeployment.Content
		baseDeploymentID = &baseUUID
		logger.GetLogger(ctx).Info("LLMProviderDeploymentService.DeployLLMProvider: base deployment retrieved", "provider_id", providerID, "base_deployment_id", req.Base)
	}

	// Create deployment
	deploymentID := uuid.New()
	deployed := models.DeploymentStatusDeployed

	logger.GetLogger(ctx).Info("LLMProviderDeploymentService.DeployLLMProvider: creating deployment", "provider_id", providerID,
		"deployment_id", deploymentID, "deployment_name", req.Name, "gateway_id", req.GatewayID)

	deployment := &models.Deployment{
		DeploymentID:     deploymentID,
		Name:             req.Name,
		ArtifactUUID:     provider.UUID,
		OUID:             ouID,
		GatewayUUID:      gatewayUUID,
		BaseDeploymentID: baseDeploymentID,
		Content:          contentBytes,
		Metadata:         req.Metadata,
		Status:           &deployed,
	}

	hardLimit := maxDeploymentsPerAPI + deploymentLimitBuffer
	if err := s.deploymentRepo.CreateWithLimitEnforcement(deployment, hardLimit); err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.DeployLLMProvider: failed to create deployment", "provider_id", providerID, "deployment_id", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	logger.GetLogger(ctx).Info("LLMProviderDeploymentService.DeployLLMProvider: deployment created successfully", "provider_id", providerID, "deployment_id", deploymentID)

	// Broadcast deployment event to gateway
	deploymentEvent := &models.LLMProviderDeploymentEvent{
		ProviderID:     providerID,
		DeploymentID:   deploymentID.String(),
		PerformedAt:    time.Now(),
		GatewayID:      req.GatewayID,
		OrganizationID: ouID,
		Status:         string(models.DeploymentStatusDeployed),
	}
	if err := s.gatewayEventsService.BroadcastLLMProviderDeploymentEvent(req.GatewayID, deploymentEvent); err != nil {
		logger.GetLogger(ctx).Error("LLMProviderDeploymentService.DeployLLMProvider: failed to broadcast deployment event",
			"provider_id", providerID, "deployment_id", deploymentID, "gateway_id", req.GatewayID, "error", err)
		// Don't fail the deployment if broadcast fails - deployment is already persisted
	} else {
		logger.GetLogger(ctx).Info("LLMProviderDeploymentService.DeployLLMProvider: deployment event broadcast successfully",
			"provider_id", providerID, "deployment_id", deploymentID, "gateway_id", req.GatewayID)
	}

	return deployment, nil
}

// UndeployLLMProviderDeployment undeploys a deployment
func (s *LLMProviderDeploymentService) UndeployLLMProviderDeployment(ctx context.Context, providerID, deploymentID, gatewayID, ouID string) (*models.Deployment, error) {
	logger.GetLogger(ctx).Info("LLMProviderDeploymentService.UndeployLLMProviderDeployment: starting", "provider_id", providerID,
		"deployment_id", deploymentID, "gateway_id", gatewayID, "ou_id", ouID)

	// Get provider
	logger.GetLogger(ctx).Info("LLMProviderDeploymentService.UndeployLLMProviderDeployment: getting provider", "provider_id", providerID, "ou_id", ouID)
	provider, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.UndeployLLMProviderDeployment: failed to get provider", "provider_id", providerID, "error", err)
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.UndeployLLMProviderDeployment: provider not found", "provider_id", providerID)
		return nil, utils.ErrLLMProviderNotFound
	}

	// Get deployment
	logger.GetLogger(ctx).Info("LLMProviderDeploymentService.UndeployLLMProviderDeployment: getting deployment", "provider_id", providerID, "deployment_id", deploymentID)
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, provider.UUID.String(), ouID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.UndeployLLMProviderDeployment: failed to get deployment", "provider_id", providerID, "deployment_id", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.UndeployLLMProviderDeployment: deployment not found", "provider_id", providerID, "deployment_id", deploymentID)
		return nil, utils.ErrDeploymentNotFound
	}
	if deployment.GatewayUUID.String() != gatewayID {
		logger.GetLogger(ctx).Error("LLMProviderDeploymentService.UndeployLLMProviderDeployment: gateway ID mismatch", "provider_id", providerID,
			"deployment_id", deploymentID, "expected_gateway_id", gatewayID, "actual_gateway_id", deployment.GatewayUUID.String())
		return nil, utils.ErrGatewayIDMismatch
	}
	if deployment.Status == nil || *deployment.Status != models.DeploymentStatusDeployed {
		logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.UndeployLLMProviderDeployment: deployment not active", "provider_id", providerID,
			"deployment_id", deploymentID, "status", deployment.Status)
		return nil, utils.ErrDeploymentNotActive
	}

	// Update status to undeployed
	logger.GetLogger(ctx).Info("LLMProviderDeploymentService.UndeployLLMProviderDeployment: setting status to undeployed", "provider_id", providerID, "deployment_id", deploymentID)
	updatedAt, err := s.deploymentRepo.SetCurrent(provider.UUID.String(), ouID, gatewayID, deploymentID, models.DeploymentStatusUndeployed)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProviderDeploymentService.UndeployLLMProviderDeployment: failed to undeploy", "provider_id", providerID, "deployment_id", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to undeploy: %w", err)
	}

	undeployed := models.DeploymentStatusUndeployed
	deployment.Status = &undeployed
	deployment.UpdatedAt = &updatedAt

	logger.GetLogger(ctx).Info("LLMProviderDeploymentService.UndeployLLMProviderDeployment: undeployed successfully", "provider_id", providerID, "deployment_id", deploymentID)

	// Broadcast undeployment event to gateway
	undeploymentEvent := &models.LLMProviderUndeploymentEvent{
		ProviderID:     providerID,
		DeploymentID:   deploymentID,
		PerformedAt:    time.Now(),
		GatewayID:      gatewayID,
		OrganizationID: ouID,
	}
	if err := s.gatewayEventsService.BroadcastLLMProviderUndeploymentEvent(gatewayID, undeploymentEvent); err != nil {
		logger.GetLogger(ctx).Error("LLMProviderDeploymentService.UndeployLLMProviderDeployment: failed to broadcast undeployment event",
			"provider_id", providerID, "deployment_id", deploymentID, "gateway_id", gatewayID, "error", err)
		// Don't fail the undeployment if broadcast fails - status is already updated
	} else {
		logger.GetLogger(ctx).Info("LLMProviderDeploymentService.UndeployLLMProviderDeployment: undeployment event broadcast successfully",
			"provider_id", providerID, "deployment_id", deploymentID, "gateway_id", gatewayID)
	}

	return deployment, nil
}

// RestoreLLMProviderDeployment restores a previous deployment
func (s *LLMProviderDeploymentService) RestoreLLMProviderDeployment(ctx context.Context, providerID, deploymentID, gatewayID, ouID string) (*models.Deployment, error) {
	// Get provider
	provider, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}

	// Get target deployment
	deployment, err := s.deploymentRepo.GetWithContent(deploymentID, provider.UUID.String(), ouID)
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
	currentDeploymentID, status, _, err := s.deploymentRepo.GetStatus(provider.UUID.String(), ouID, gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment status: %w", err)
	}
	if currentDeploymentID == deploymentID && status == models.DeploymentStatusDeployed {
		return nil, utils.ErrDeploymentAlreadyDeployed
	}

	// Update status to deployed
	updatedAt, err := s.deploymentRepo.SetCurrent(provider.UUID.String(), ouID, gatewayID, deploymentID, models.DeploymentStatusDeployed)
	if err != nil {
		return nil, fmt.Errorf("failed to restore deployment: %w", err)
	}

	deployed := models.DeploymentStatusDeployed
	deployment.Status = &deployed
	deployment.UpdatedAt = &updatedAt

	// Broadcast deployment event to gateway (restore is treated as a deployment)
	deploymentEvent := &models.LLMProviderDeploymentEvent{
		ProviderID:     providerID,
		DeploymentID:   deploymentID,
		PerformedAt:    time.Now(),
		GatewayID:      gatewayID,
		OrganizationID: ouID,
		Status:         string(models.DeploymentStatusDeployed),
	}
	if err := s.gatewayEventsService.BroadcastLLMProviderDeploymentEvent(gatewayID, deploymentEvent); err != nil {
		logger.GetLogger(ctx).Error("LLMProviderDeploymentService.RestoreLLMProviderDeployment: failed to broadcast deployment event",
			"provider_id", providerID, "deployment_id", deploymentID, "gateway_id", gatewayID, "error", err)
		// Don't fail the restore if broadcast fails - status is already updated
	} else {
		logger.GetLogger(ctx).Info("LLMProviderDeploymentService.RestoreLLMProviderDeployment: deployment event broadcast successfully",
			"provider_id", providerID, "deployment_id", deploymentID, "gateway_id", gatewayID)
	}

	return deployment, nil
}

// GetLLMProviderDeployments retrieves all deployments for a provider
func (s *LLMProviderDeploymentService) GetLLMProviderDeployments(providerID, ouID string, gatewayID *string, status *string) ([]*models.Deployment, error) {
	// Get provider
	provider, err := s.resolveProvider(providerID, ouID)
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
	deployments, err := s.deploymentRepo.GetDeploymentsWithState(provider.UUID.String(), ouID, gatewayID, status, maxDeploymentsPerAPI)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployments: %w", err)
	}

	return deployments, nil
}

// GetLLMProviderDeployment retrieves a specific deployment
func (s *LLMProviderDeploymentService) GetLLMProviderDeployment(providerID, deploymentID, ouID string) (*models.Deployment, error) {
	// Get provider
	provider, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return nil, utils.ErrLLMProviderNotFound
	}

	// Get deployment
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, provider.UUID.String(), ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrDeploymentNotFound
	}

	return deployment, nil
}

// DeleteLLMProviderDeployment deletes a deployment
func (s *LLMProviderDeploymentService) DeleteLLMProviderDeployment(providerID, deploymentID, ouID string) error {
	// Get provider
	provider, err := s.resolveProvider(providerID, ouID)
	if err != nil {
		return fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return utils.ErrLLMProviderNotFound
	}

	// Get deployment
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, provider.UUID.String(), ouID)
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
	if err := s.deploymentRepo.Delete(deploymentID, provider.UUID.String(), ouID); err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	return nil
}

// generateLLMProviderDeploymentYAML generates deployment YAML for an LLM provider
func (s *LLMProviderDeploymentService) generateLLMProviderDeploymentYAML(provider *models.LLMProvider, ouID string) (string, error) {
	// ouID parameter is reserved for future use in multi-tenant scenarios
	_ = ouID
	if provider == nil {
		return "", errors.New("provider is required")
	}

	// Validate template handle exists
	if provider.TemplateHandle == "" {
		return "", utils.ErrLLMProviderTemplateNotFound
	}

	// Enhanced upstream validation (check both Upstream and Main are non-nil)
	if provider.Configuration.Upstream == nil || provider.Configuration.Upstream.Main == nil {
		return "", fmt.Errorf("upstream.main configuration is required: %w", utils.ErrInvalidInput)
	}

	main := provider.Configuration.Upstream.Main
	// Validate that either URL or Ref is provided
	if main.URL == "" && main.Ref == "" {
		return "", fmt.Errorf("upstream.main must have either url or ref: %w", utils.ErrInvalidInput)
	}

	// Template handle is already stored in provider.TemplateHandle
	// No need to fetch the template itself - handle is sufficient for gateway config
	templateHandle := provider.TemplateHandle

	// Set default context if not provided
	contextValue := "/"
	if provider.Configuration.Context != nil && *provider.Configuration.Context != "" {
		contextValue = *provider.Configuration.Context
	}

	vhostValue := ""
	if provider.Configuration.VHost != nil {
		vhostValue = *provider.Configuration.VHost
	}

	// Transform upstream from nested (main/sandbox) to flat structure expected by gateway.
	gatewayUpstream := GatewayUpstream{
		URL:  main.URL,
		Ref:  main.Ref,
		Auth: main.Auth,
	}

	// Ensure access control has a valid mode with default
	// Create a local copy to avoid mutating the provider's configuration
	var accessControl *models.LLMAccessControl
	if provider.Configuration.AccessControl == nil {
		// Set default to deny_all if not provided
		accessControl = &models.LLMAccessControl{
			Mode: "deny_all",
		}
	} else {
		// Create a copy of the access control config
		accessControlCopy := *provider.Configuration.AccessControl
		if accessControlCopy.Mode != "allow_all" && accessControlCopy.Mode != "deny_all" {
			// Fix invalid mode to default
			accessControlCopy.Mode = "deny_all"
		}
		accessControl = &accessControlCopy
	}

	// Transform policies: security, rate limiting, and user-defined policies into unified policy array
	policies := make([]models.LLMPolicy, 0)

	// Step 1: Transform security config to API key auth policy
	security := provider.Configuration.Security
	if security != nil && isBoolTrue(security.Enabled) {
		if security.APIKey != nil && isBoolTrue(security.APIKey.Enabled) {
			key := strings.TrimSpace(security.APIKey.Key)
			if key == "" {
				return "", fmt.Errorf("invalid api key security configuration: key is required")
			}

			in := strings.ToLower(strings.TrimSpace(security.APIKey.In))
			if in != "header" && in != "query" {
				return "", fmt.Errorf("invalid api key security configuration: in must be 'header' or 'query', got %q", security.APIKey.In)
			}

			addOrAppendPolicyPath(&policies, apiKeyAuthPolicyName, apiKeyAuthPolicyVersion, models.LLMPolicyPath{
				Path:    "/*",
				Methods: []string{"*"},
				Params: map[string]interface{}{
					"key": key,
					"in":  in,
				},
			})
		}
	}

	// Step 2: Transform rate limit config to policies
	rateLimit := provider.Configuration.RateLimiting
	// costPaths accumulates the specific paths where llm-cost-based-ratelimit is applied,
	// so llm-cost can be scoped to the same paths rather than applied globally.
	var costPaths []models.LLMPolicyPath
	if rateLimit != nil {
		// Step 2.1: Provider level rate limit
		providerLevel := rateLimit.ProviderLevel
		if providerLevel != nil {
			// Priority to global rate limit configuration if both global and resource-wise are present
			if providerLevel.Global != nil {
				// Step 2.1.1: Handle global rate limiting
				if providerLevel.Global.Token != nil && providerLevel.Global.Token.Enabled {
					tokenLimit := providerLevel.Global.Token
					duration, err := formatRateLimitDuration(tokenLimit.Reset.Duration, tokenLimit.Reset.Unit)
					if err != nil {
						return "", fmt.Errorf("invalid token reset window: %w", err)
					}
					policies = append(policies, models.LLMPolicy{
						Name:    tokenBasedRateLimitPolicyName,
						Version: rateLimitPolicyVersion,
						Paths: []models.LLMPolicyPath{
							{
								Path:    "/*",
								Methods: []string{"*"},
								Params: map[string]interface{}{
									"totalTokenLimits": []map[string]interface{}{
										{
											"count":    tokenLimit.Count,
											"duration": duration,
										},
									},
								},
							},
						},
					})
				}

				if providerLevel.Global.Request != nil && providerLevel.Global.Request.Enabled {
					requestLimit := providerLevel.Global.Request
					duration, err := formatRateLimitDuration(requestLimit.Reset.Duration, requestLimit.Reset.Unit)
					if err != nil {
						return "", fmt.Errorf("invalid request reset window: %w", err)
					}
					policies = append(policies, models.LLMPolicy{
						Name:    advancedRateLimitPolicyName,
						Version: rateLimitPolicyVersion,
						Paths: []models.LLMPolicyPath{
							{
								Path:    "/*",
								Methods: []string{"*"},
								Params: map[string]interface{}{
									"quotas": []map[string]interface{}{
										{
											"name": "request-limit",
											"limits": []map[string]interface{}{
												{
													"limit":    requestLimit.Count,
													"duration": duration,
												},
											},
										},
									},
								},
							},
						},
					})
				}

				if providerLevel.Global.Cost != nil && providerLevel.Global.Cost.Enabled {
					costLimit := providerLevel.Global.Cost
					duration, err := formatRateLimitDuration(costLimit.Reset.Duration, costLimit.Reset.Unit)
					if err != nil {
						return "", fmt.Errorf("invalid cost reset window: %w", err)
					}
					costPath := models.LLMPolicyPath{
						Path:    "/*",
						Methods: []string{"*"},
					}
					policies = append(policies, models.LLMPolicy{
						Name:    costBasedRateLimitPolicyName,
						Version: rateLimitPolicyVersion,
						Paths: []models.LLMPolicyPath{
							{
								Path:    costPath.Path,
								Methods: costPath.Methods,
								Params: map[string]interface{}{
									"budgetLimits": []map[string]interface{}{
										{
											"amount":   costLimit.Amount,
											"duration": duration,
										},
									},
								},
							},
						},
					})
					costPaths = append(costPaths, costPath)
				}
			} else if providerLevel.ResourceWise != nil {
				// Step 2.1.2: Handle resource-wise rate limiting.
				// Resource-specific paths must appear before the catch-all "/*" so that the
				// gateway evaluates specific rules first and falls back to the default last.
				defaultLimit := &providerLevel.ResourceWise.Default

				// Step 2.1.2.1: Resource-specific rate limits (specific paths first)
				for _, r := range providerLevel.ResourceWise.Resources {
					resMethod, resPath := parseResourceKey(r.Resource)
					resMethods := []string{resMethod}

					if r.Limit.Token != nil && r.Limit.Token.Enabled {
						tokenLimit := r.Limit.Token
						duration, err := formatRateLimitDuration(tokenLimit.Reset.Duration, tokenLimit.Reset.Unit)
						if err != nil {
							return "", fmt.Errorf("invalid token reset window for resource %s: %w", r.Resource, err)
						}
						addOrAppendPolicyPath(&policies, tokenBasedRateLimitPolicyName, rateLimitPolicyVersion, models.LLMPolicyPath{
							Path:    resPath,
							Methods: resMethods,
							Params: map[string]interface{}{
								"totalTokenLimits": []map[string]interface{}{
									{
										"count":    tokenLimit.Count,
										"duration": duration,
									},
								},
							},
						})
					}

					if r.Limit.Request != nil && r.Limit.Request.Enabled {
						requestLimit := r.Limit.Request
						duration, err := formatRateLimitDuration(requestLimit.Reset.Duration, requestLimit.Reset.Unit)
						if err != nil {
							return "", fmt.Errorf("invalid request reset window for resource %s: %w", r.Resource, err)
						}
						addOrAppendPolicyPath(&policies, advancedRateLimitPolicyName, rateLimitPolicyVersion, models.LLMPolicyPath{
							Path:    resPath,
							Methods: resMethods,
							Params: map[string]interface{}{
								"quotas": []map[string]interface{}{
									{
										"name": "request-limit",
										"limits": []map[string]interface{}{
											{
												"limit":    requestLimit.Count,
												"duration": duration,
											},
										},
									},
								},
							},
						})
					}

					if r.Limit.Cost != nil && r.Limit.Cost.Enabled {
						costLimit := r.Limit.Cost
						duration, err := formatRateLimitDuration(costLimit.Reset.Duration, costLimit.Reset.Unit)
						if err != nil {
							return "", fmt.Errorf("invalid cost reset window for resource %s: %w", r.Resource, err)
						}
						costPath := models.LLMPolicyPath{Path: resPath, Methods: resMethods}
						addOrAppendPolicyPath(&policies, costBasedRateLimitPolicyName, rateLimitPolicyVersion, models.LLMPolicyPath{
							Path:    resPath,
							Methods: resMethods,
							Params: map[string]interface{}{
								"budgetLimits": []map[string]interface{}{
									{
										"amount":   costLimit.Amount,
										"duration": duration,
									},
								},
							},
						})
						costPaths = append(costPaths, costPath)
					}
				}

				// Step 2.1.2.2: Default catch-all "/*" appended last so specific paths
				// are evaluated before the fallback.
				if defaultLimit.Token != nil && defaultLimit.Token.Enabled {
					tokenLimit := defaultLimit.Token
					duration, err := formatRateLimitDuration(tokenLimit.Reset.Duration, tokenLimit.Reset.Unit)
					if err != nil {
						return "", fmt.Errorf("invalid token reset window: %w", err)
					}
					addOrAppendPolicyPath(&policies, tokenBasedRateLimitPolicyName, rateLimitPolicyVersion, models.LLMPolicyPath{
						Path:    "/*",
						Methods: []string{"*"},
						Params: map[string]interface{}{
							"totalTokenLimits": []map[string]interface{}{
								{
									"count":    tokenLimit.Count,
									"duration": duration,
								},
							},
						},
					})
				}

				if defaultLimit.Request != nil && defaultLimit.Request.Enabled {
					requestLimit := defaultLimit.Request
					duration, err := formatRateLimitDuration(requestLimit.Reset.Duration, requestLimit.Reset.Unit)
					if err != nil {
						return "", fmt.Errorf("invalid request reset window: %w", err)
					}
					addOrAppendPolicyPath(&policies, advancedRateLimitPolicyName, rateLimitPolicyVersion, models.LLMPolicyPath{
						Path:    "/*",
						Methods: []string{"*"},
						Params: map[string]interface{}{
							"quotas": []map[string]interface{}{
								{
									"name": "request-limit",
									"limits": []map[string]interface{}{
										{
											"limit":    requestLimit.Count,
											"duration": duration,
										},
									},
								},
							},
						},
					})
				}

				if defaultLimit.Cost != nil && defaultLimit.Cost.Enabled {
					costLimit := defaultLimit.Cost
					duration, err := formatRateLimitDuration(costLimit.Reset.Duration, costLimit.Reset.Unit)
					if err != nil {
						return "", fmt.Errorf("invalid cost reset window: %w", err)
					}
					costPath := models.LLMPolicyPath{Path: "/*", Methods: []string{"*"}}
					addOrAppendPolicyPath(&policies, costBasedRateLimitPolicyName, rateLimitPolicyVersion, models.LLMPolicyPath{
						Path:    "/*",
						Methods: []string{"*"},
						Params: map[string]interface{}{
							"budgetLimits": []map[string]interface{}{
								{
									"amount":   costLimit.Amount,
									"duration": duration,
								},
							},
						},
					})
					costPaths = append(costPaths, costPath)
				}
			}
		}

		// Step 2.2: Consumer level rate limit (per-application, keyed by x-wso2-application-id)
		consumerLevel := rateLimit.ConsumerLevel
		if consumerLevel != nil && consumerLevel.Global != nil {
			if consumerLevel.Global.Token != nil && consumerLevel.Global.Token.Enabled {
				tokenLimit := consumerLevel.Global.Token
				duration, err := formatRateLimitDuration(tokenLimit.Reset.Duration, tokenLimit.Reset.Unit)
				if err != nil {
					return "", fmt.Errorf("invalid consumer token reset window: %w", err)
				}
				policies = append(policies, models.LLMPolicy{
					Name:    tokenBasedRateLimitPolicyName,
					Version: rateLimitPolicyVersion,
					Paths: []models.LLMPolicyPath{
						{
							Path:    "/*",
							Methods: []string{"*"},
							Params: map[string]interface{}{
								"consumerBased": true,
								"totalTokenLimits": []map[string]interface{}{
									{
										"count":    tokenLimit.Count,
										"duration": duration,
									},
								},
							},
						},
					},
				})
			}

			if consumerLevel.Global.Request != nil && consumerLevel.Global.Request.Enabled {
				requestLimit := consumerLevel.Global.Request
				duration, err := formatRateLimitDuration(requestLimit.Reset.Duration, requestLimit.Reset.Unit)
				if err != nil {
					return "", fmt.Errorf("invalid consumer request reset window: %w", err)
				}
				policies = append(policies, models.LLMPolicy{
					Name:    advancedRateLimitPolicyName,
					Version: rateLimitPolicyVersion,
					Paths: []models.LLMPolicyPath{
						{
							Path:    "/*",
							Methods: []string{"*"},
							Params: map[string]interface{}{
								"consumerBased": true,
								"quotas": []map[string]interface{}{
									{
										"name": "consumer-request-limit",
										"limits": []map[string]interface{}{
											{
												"limit":    requestLimit.Count,
												"duration": duration,
											},
										},
									},
								},
							},
						},
					},
				})
			}

			if consumerLevel.Global.Cost != nil && consumerLevel.Global.Cost.Enabled {
				costLimit := consumerLevel.Global.Cost
				duration, err := formatRateLimitDuration(costLimit.Reset.Duration, costLimit.Reset.Unit)
				if err != nil {
					return "", fmt.Errorf("invalid consumer cost reset window: %w", err)
				}
				costPath := models.LLMPolicyPath{
					Path:    "/*",
					Methods: []string{"*"},
				}
				policies = append(policies, models.LLMPolicy{
					Name:    costBasedRateLimitPolicyName,
					Version: rateLimitPolicyVersion,
					Paths: []models.LLMPolicyPath{
						{
							Path:    costPath.Path,
							Methods: costPath.Methods,
							Params: map[string]interface{}{
								"consumerBased": true,
								"budgetLimits": []map[string]interface{}{
									{
										"amount":   costLimit.Amount,
										"duration": duration,
									},
								},
							},
						},
					},
				})
				costPaths = append(costPaths, costPath)
			}
		}
	}

	// llm-cost must run before llm-cost-based-ratelimit in the response phase.
	// Response-phase policies execute in reverse list order, so llm-cost is appended
	// after llm-cost-based-ratelimit to guarantee it fires first at response time.
	// llm-cost is scoped to the same paths as llm-cost-based-ratelimit, not globally.
	if len(costPaths) > 0 {
		seen := make(map[string]bool, len(costPaths))
		var llmCostPaths []models.LLMPolicyPath
		for _, cp := range costPaths {
			key := cp.Path + "|" + strings.Join(cp.Methods, ",")
			if !seen[key] {
				seen[key] = true
				llmCostPaths = append(llmCostPaths, models.LLMPolicyPath{Path: cp.Path, Methods: cp.Methods})
			}
		}
		policies = append(policies, models.LLMPolicy{
			Name:    llmCostPolicyName,
			Version: rateLimitPolicyVersion,
			Paths:   llmCostPaths,
		})
	}

	// Step 3: Append user-defined policies with version normalization
	for _, p := range provider.Configuration.Policies {
		policies = append(policies, models.LLMPolicy{
			Name:    p.Name,
			Version: normalizePolicyVersionToMajor(p.Version),
			Paths:   p.Paths,
		})
	}

	// Build deployment YAML with transformed policies
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
			Template:      templateHandle,
			Upstream:      gatewayUpstream,
			Resilience:    provider.Configuration.Resilience,
			AccessControl: accessControl,
			Policies:      policies,
		},
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(deploymentYAML)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to YAML: %w", err)
	}

	return string(yamlBytes), nil
}

// parseResourceKey splits a resource key of the form "METHOD-/path" back into its
// constituent method and path. The key is produced by the frontend as "<method>-<path>".
// If the key does not contain "-", the whole string is treated as the path with method "*".
func parseResourceKey(key string) (method, path string) {
	idx := strings.Index(key, "-")
	if idx < 0 {
		return "*", key
	}
	return key[:idx], key[idx+1:]
}

// formatRateLimitDuration converts rate limit duration and unit to a Go duration string
// accepted by gateway policies (ns|us|ms|s|m|h). Day/week/month are converted to hours
// since the Go time package has no native support for calendar units.
func formatRateLimitDuration(duration int, unit string) (string, error) {
	if duration <= 0 {
		return "", fmt.Errorf("duration must be positive, got %d", duration)
	}

	switch strings.ToLower(strings.TrimSpace(unit)) {
	case "minute":
		return fmt.Sprintf("%dm", duration), nil
	case "hour":
		return fmt.Sprintf("%dh", duration), nil
	case "day":
		return fmt.Sprintf("%dh", duration*24), nil
	case "week":
		return fmt.Sprintf("%dh", duration*24*7), nil
	case "month":
		return fmt.Sprintf("%dh", duration*24*30), nil
	default:
		return "", fmt.Errorf("unsupported reset unit %q: must be minute, hour, day, week, or month", unit)
	}
}

// normalizePolicyVersionToMajor normalizes policy version to major version format (e.g., "0.1.0" -> "v0")
func normalizePolicyVersionToMajor(version string) string {
	trimmedVersion := strings.TrimSpace(version)
	if trimmedVersion == "" {
		return trimmedVersion
	}

	versionWithoutPrefix := trimmedVersion
	if strings.HasPrefix(strings.ToLower(versionWithoutPrefix), "v") {
		versionWithoutPrefix = versionWithoutPrefix[1:]
	}
	if versionWithoutPrefix == "" {
		return trimmedVersion
	}

	majorVersion := versionWithoutPrefix
	if idx := strings.Index(majorVersion, "."); idx >= 0 {
		majorVersion = majorVersion[:idx]
	}
	if idx := strings.Index(majorVersion, "-"); idx >= 0 {
		majorVersion = majorVersion[:idx]
	}
	majorVersion = strings.TrimSpace(majorVersion)
	if majorVersion == "" {
		return trimmedVersion
	}

	if _, err := strconv.Atoi(majorVersion); err != nil {
		return trimmedVersion
	}

	return "v" + majorVersion
}

// addOrAppendPolicyPath adds a new policy or appends a path to an existing policy
func addOrAppendPolicyPath(policies *[]models.LLMPolicy, name, version string, path models.LLMPolicyPath) {
	for i := range *policies {
		if (*policies)[i].Name == name && (*policies)[i].Version == version {
			// Check for duplicate paths by comparing full path structure (Path, Methods, Params)
			for _, existingPath := range (*policies)[i].Paths {
				if pathsAreEqual(existingPath, path) {
					// Keep first occurrence and avoid duplicates
					return
				}
			}
			(*policies)[i].Paths = append((*policies)[i].Paths, path)
			return
		}
	}

	*policies = append(*policies, models.LLMPolicy{
		Name:    name,
		Version: version,
		Paths:   []models.LLMPolicyPath{path},
	})
}

// pathsAreEqual compares two policy paths for equality by checking Path, Methods, and Params
func pathsAreEqual(a, b models.LLMPolicyPath) bool {
	// Compare path strings
	if a.Path != b.Path {
		return false
	}

	// Compare methods arrays - use deduplicated sets to avoid false positives with duplicates
	// Build sets from both method arrays
	aMethodsSet := make(map[string]bool)
	for _, m := range a.Methods {
		aMethodsSet[m] = true
	}
	bMethodsSet := make(map[string]bool)
	for _, m := range b.Methods {
		bMethodsSet[m] = true
	}

	// Compare set sizes first
	if len(aMethodsSet) != len(bMethodsSet) {
		return false
	}

	// Ensure all methods in a are in b
	for m := range aMethodsSet {
		if !bMethodsSet[m] {
			return false
		}
	}

	// Compare params maps (shallow comparison of keys and values)
	if len(a.Params) != len(b.Params) {
		return false
	}
	if len(a.Params) == 0 && len(b.Params) == 0 {
		return true
	}
	// For simplicity, do a string-based comparison of the maps
	// This works for most cases but may have edge cases with complex nested structures
	for k, v := range a.Params {
		bv, exists := b.Params[k]
		if !exists {
			return false
		}
		// Simple value comparison - works for primitives and will catch most differences
		if fmt.Sprintf("%v", v) != fmt.Sprintf("%v", bv) {
			return false
		}
	}

	return true
}

// isBoolTrue checks if a boolean pointer is non-nil and true
func isBoolTrue(v *bool) bool {
	return v != nil && *v
}
