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
	"strings"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	apiVersionLLMProxy = "gateway.api-platform.wso2.com/v1"
	kindLLMProxy       = "LlmProxy"
)

// LLMProxyDeploymentService handles LLM proxy deployment business logic
type LLMProxyDeploymentService struct {
	deploymentRepo       repositories.DeploymentRepository
	proxyRepo            repositories.LLMProxyRepository
	providerRepo         repositories.LLMProviderRepository
	gatewayRepo          repositories.GatewayRepository
	gatewayEventsService *GatewayEventsService
}

// NewLLMProxyDeploymentService creates a new LLM proxy deployment service
func NewLLMProxyDeploymentService(
	deploymentRepo repositories.DeploymentRepository,
	proxyRepo repositories.LLMProxyRepository,
	providerRepo repositories.LLMProviderRepository,
	gatewayRepo repositories.GatewayRepository,
	gatewayEventsService *GatewayEventsService,
) *LLMProxyDeploymentService {
	return &LLMProxyDeploymentService{
		deploymentRepo:       deploymentRepo,
		proxyRepo:            proxyRepo,
		providerRepo:         providerRepo,
		gatewayRepo:          gatewayRepo,
		gatewayEventsService: gatewayEventsService,
	}
}

// LLMProxyDeploymentYAML represents the deployment YAML
type LLMProxyDeploymentYAML struct {
	ApiVersion string                 `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                 `yaml:"kind" json:"kind"`
	Metadata   DeploymentMetadata     `yaml:"metadata" json:"metadata"`
	Spec       LLMProxyDeploymentSpec `yaml:"spec" json:"spec"`
}

// LLMProxyDeploymentSpec represents the spec section
type LLMProxyDeploymentSpec struct {
	DisplayName string                     `yaml:"displayName" json:"displayName"`
	Version     string                     `yaml:"version" json:"version"`
	Context     string                     `yaml:"context,omitempty" json:"context,omitempty"`
	VHost       string                     `yaml:"vhost,omitempty" json:"vhost,omitempty"`
	Provider    LLMProxyDeploymentProvider `yaml:"provider" json:"provider"`
	Resilience  *models.Resilience         `yaml:"resilience,omitempty" json:"resilience,omitempty"`
	Policies    []models.LLMPolicy         `yaml:"policies,omitempty" json:"policies,omitempty"`
	Security    *models.SecurityConfig     `yaml:"security,omitempty" json:"security,omitempty"`
}

// LLMProxyDeploymentProvider represents the provider configuration in the spec
type LLMProxyDeploymentProvider struct {
	ID   string               `yaml:"id" json:"id"`
	Auth *models.UpstreamAuth `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// DeployLLMProxy deploys an LLM proxy to a gateway
func (s *LLMProxyDeploymentService) DeployLLMProxy(ctx context.Context, proxyID string, req *models.DeployAPIRequest, ouID string) (*models.Deployment, error) {
	logger.GetLogger(ctx).Info("LLMProxyDeploymentService.DeployLLMProxy: starting", "proxy_id", proxyID, "ou_id", ouID,
		"deployment_name", req.Name, "base", req.Base, "gateway_id", req.GatewayID)

	if req.Base == "" {
		logger.GetLogger(ctx).Error("LLMProxyDeploymentService.DeployLLMProxy: base is required", "proxy_id", proxyID)
		return nil, utils.ErrDeploymentBaseRequired
	}
	if req.GatewayID == "" {
		logger.GetLogger(ctx).Error("LLMProxyDeploymentService.DeployLLMProxy: gateway ID is required", "proxy_id", proxyID)
		return nil, utils.ErrDeploymentGatewayIDRequired
	}
	if req.Name == "" {
		logger.GetLogger(ctx).Error("LLMProxyDeploymentService.DeployLLMProxy: deployment name is required", "proxy_id", proxyID)
		return nil, utils.ErrDeploymentNameRequired
	}

	// Parse UUIDs
	gatewayUUID, err := uuid.Parse(req.GatewayID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.DeployLLMProxy: invalid gateway UUID", "proxy_id", proxyID, "gateway_id", req.GatewayID, "error", err)
		return nil, fmt.Errorf("invalid gateway UUID: %w", err)
	}

	// Validate gateway exists
	logger.GetLogger(ctx).Info("LLMProxyDeploymentService.DeployLLMProxy: validating gateway", "proxy_id", proxyID, "gateway_id", req.GatewayID)
	gateway, err := s.gatewayRepo.GetByUUID(req.GatewayID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.DeployLLMProxy: failed to get gateway", "proxy_id", proxyID, "gateway_id", req.GatewayID, "error", err)
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}
	if gateway == nil || gateway.OUID != ouID {
		logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.DeployLLMProxy: gateway not found or org mismatch", "proxy_id", proxyID, "gateway_id", req.GatewayID, "ou_id", ouID)
		return nil, utils.ErrGatewayNotFound
	}

	// Get LLM proxy
	logger.GetLogger(ctx).Info("LLMProxyDeploymentService.DeployLLMProxy: getting proxy", "proxy_id", proxyID, "ou_id", ouID)
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.DeployLLMProxy: proxy not found", "proxy_id", proxyID, "ou_id", ouID)
			return nil, utils.ErrLLMProxyNotFound
		}
		logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.DeployLLMProxy: failed to get proxy", "proxy_id", proxyID, "ou_id", ouID, "error", err)
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.DeployLLMProxy: proxy not found", "proxy_id", proxyID, "ou_id", ouID)
		return nil, utils.ErrLLMProxyNotFound
	}

	logger.GetLogger(ctx).Info("LLMProxyDeploymentService.DeployLLMProxy: proxy retrieved", "proxy_id", proxyID, "proxy_uuid", proxy.UUID)

	existing, err := s.deploymentRepo.GetDeployedGatewaysByProvider(proxy.UUID, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to list existing proxy deployments: %w", err)
	}
	if err := validateEgressPlacement(s.gatewayRepo, gateway, existing); err != nil {
		return nil, fmt.Errorf("%w: %w", utils.ErrInvalidInput, err)
	}

	var baseDeploymentID *uuid.UUID
	var contentBytes []byte

	// Determine source: "current" or existing deployment
	if req.Base == "current" {
		logger.GetLogger(ctx).Info("LLMProxyDeploymentService.DeployLLMProxy: using current proxy configuration", "proxy_id", proxyID)

		// Generate deployment YAML
		logger.GetLogger(ctx).Info("LLMProxyDeploymentService.DeployLLMProxy: generating deployment YAML", "proxy_id", proxyID)
		deploymentYAML, err := s.generateLLMProxyDeploymentYAML(proxy, ouID)
		if err != nil {
			logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.DeployLLMProxy: failed to generate deployment YAML", "proxy_id", proxyID, "error", err)
			return nil, fmt.Errorf("failed to generate deployment YAML: %w", err)
		}
		contentBytes = []byte(deploymentYAML)
	} else {
		logger.GetLogger(ctx).Info("LLMProxyDeploymentService.DeployLLMProxy: using existing deployment as base", "proxy_id", proxyID, "base_deployment_id", req.Base)

		// Use existing deployment as base
		baseUUID, err := uuid.Parse(req.Base)
		if err != nil {
			logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.DeployLLMProxy: invalid base deployment ID", "proxy_id", proxyID, "base_deployment_id", req.Base, "error", err)
			return nil, fmt.Errorf("invalid base deployment ID: %w", err)
		}

		baseDeployment, err := s.deploymentRepo.GetWithContent(req.Base, proxy.UUID.String(), ouID)
		if err != nil {
			logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.DeployLLMProxy: base deployment not found", "proxy_id", proxyID, "base_deployment_id", req.Base, "error", err)
			return nil, utils.ErrBaseDeploymentNotFound
		}
		contentBytes = baseDeployment.Content
		baseDeploymentID = &baseUUID
		logger.GetLogger(ctx).Info("LLMProxyDeploymentService.DeployLLMProxy: base deployment retrieved", "proxy_id", proxyID, "base_deployment_id", req.Base)
	}

	// Create deployment
	deploymentID := uuid.New()
	deployed := models.DeploymentStatusDeployed

	logger.GetLogger(ctx).Info("LLMProxyDeploymentService.DeployLLMProxy: creating deployment", "proxy_id", proxyID,
		"deployment_id", deploymentID, "deployment_name", req.Name, "gateway_id", req.GatewayID)

	deployment := &models.Deployment{
		DeploymentID:     deploymentID,
		Name:             req.Name,
		ArtifactUUID:     proxy.UUID,
		OUID:             ouID,
		GatewayUUID:      gatewayUUID,
		BaseDeploymentID: baseDeploymentID,
		Content:          contentBytes,
		Metadata:         req.Metadata,
		Status:           &deployed,
	}

	hardLimit := maxDeploymentsPerAPI + deploymentLimitBuffer
	if err := s.deploymentRepo.CreateWithLimitEnforcement(deployment, hardLimit); err != nil {
		logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.DeployLLMProxy: failed to create deployment", "proxy_id", proxyID, "deployment_id", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to create deployment: %w", err)
	}

	logger.GetLogger(ctx).Info("LLMProxyDeploymentService.DeployLLMProxy: deployment created successfully", "proxy_id", proxyID, "deployment_id", deploymentID)

	// Broadcast deployment event to gateway
	vhost := ""
	if proxy.Configuration.Vhost != nil {
		vhost = *proxy.Configuration.Vhost
	}

	deploymentEvent := &models.LLMProxyDeploymentEvent{
		ProxyID:        proxyID,
		DeploymentID:   deploymentID.String(),
		Vhost:          vhost,
		Environment:    "production",
		GatewayID:      req.GatewayID,
		OrganizationID: ouID,
		Status:         string(models.DeploymentStatusDeployed),
	}
	if err := s.gatewayEventsService.BroadcastLLMProxyDeploymentEvent(req.GatewayID, deploymentEvent); err != nil {
		logger.GetLogger(ctx).Error("LLMProxyDeploymentService.DeployLLMProxy: failed to broadcast deployment event",
			"proxy_id", proxyID, "deployment_id", deploymentID, "gateway_id", req.GatewayID, "error", err)
		// Don't fail the deployment if broadcast fails - deployment is already persisted
	} else {
		logger.GetLogger(ctx).Info("LLMProxyDeploymentService.DeployLLMProxy: deployment event broadcast successfully",
			"proxy_id", proxyID, "deployment_id", deploymentID, "gateway_id", req.GatewayID)
	}

	return deployment, nil
}

// UndeployLLMProxyDeployment undeploys a deployment
func (s *LLMProxyDeploymentService) UndeployLLMProxyDeployment(ctx context.Context, proxyID, deploymentID, gatewayID, ouID string) (*models.Deployment, error) {
	logger.GetLogger(ctx).Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: starting", "proxy_id", proxyID,
		"deployment_id", deploymentID, "gateway_id", gatewayID, "ou_id", ouID)

	// Get proxy
	logger.GetLogger(ctx).Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: getting proxy", "proxy_id", proxyID, "ou_id", ouID)
	proxy, err := s.proxyRepo.GetByIDCtx(ctx, proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: proxy not found", "proxy_id", proxyID)
			return nil, utils.ErrLLMProxyNotFound
		}
		logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: failed to get proxy", "proxy_id", proxyID, "error", err)
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: proxy not found", "proxy_id", proxyID)
		return nil, utils.ErrLLMProxyNotFound
	}

	// Get deployment
	logger.GetLogger(ctx).Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: getting deployment", "proxy_id", proxyID, "deployment_id", deploymentID)
	deployment, err := s.deploymentRepo.GetWithStateCtx(ctx, deploymentID, proxy.UUID.String(), ouID)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: failed to get deployment", "proxy_id", proxyID, "deployment_id", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: deployment not found", "proxy_id", proxyID, "deployment_id", deploymentID)
		return nil, utils.ErrDeploymentNotFound
	}
	if deployment.GatewayUUID.String() != gatewayID {
		logger.GetLogger(ctx).Error("LLMProxyDeploymentService.UndeployLLMProxyDeployment: gateway ID mismatch", "proxy_id", proxyID,
			"deployment_id", deploymentID, "expected_gateway_id", gatewayID, "actual_gateway_id", deployment.GatewayUUID.String())
		return nil, utils.ErrGatewayIDMismatch
	}
	if deployment.Status == nil || *deployment.Status != models.DeploymentStatusDeployed {
		logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: deployment not active", "proxy_id", proxyID,
			"deployment_id", deploymentID, "status", deployment.Status)
		return nil, utils.ErrDeploymentNotActive
	}

	// Update status to undeployed
	logger.GetLogger(ctx).Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: setting status to undeployed", "proxy_id", proxyID, "deployment_id", deploymentID)
	updatedAt, err := s.deploymentRepo.SetCurrentCtx(ctx, proxy.UUID.String(), ouID, gatewayID, deploymentID, models.DeploymentStatusUndeployed)
	if err != nil {
		logger.GetLogger(ctx).Warn("LLMProxyDeploymentService.UndeployLLMProxyDeployment: failed to undeploy", "proxy_id", proxyID, "deployment_id", deploymentID, "error", err)
		return nil, fmt.Errorf("failed to undeploy: %w", err)
	}

	undeployed := models.DeploymentStatusUndeployed
	deployment.Status = &undeployed
	deployment.UpdatedAt = &updatedAt

	logger.GetLogger(ctx).Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: undeployed successfully", "proxy_id", proxyID, "deployment_id", deploymentID)

	// Broadcast undeployment event to gateway
	vhost := ""
	if proxy.Configuration.Vhost != nil {
		vhost = *proxy.Configuration.Vhost
	}

	undeploymentEvent := &models.LLMProxyUndeploymentEvent{
		ProxyID:        proxyID,
		Vhost:          vhost,
		Environment:    "production",
		GatewayID:      gatewayID,
		OrganizationID: ouID,
	}
	if err := s.gatewayEventsService.BroadcastLLMProxyUndeploymentEvent(gatewayID, undeploymentEvent); err != nil {
		logger.GetLogger(ctx).Error("LLMProxyDeploymentService.UndeployLLMProxyDeployment: failed to broadcast undeployment event",
			"proxy_id", proxyID, "deployment_id", deploymentID, "gateway_id", gatewayID, "error", err)
		// Don't fail the undeployment if broadcast fails - status is already updated
	} else {
		logger.GetLogger(ctx).Info("LLMProxyDeploymentService.UndeployLLMProxyDeployment: undeployment event broadcast successfully",
			"proxy_id", proxyID, "deployment_id", deploymentID, "gateway_id", gatewayID)
	}

	return deployment, nil
}

// RestoreLLMProxyDeployment restores a previous deployment
func (s *LLMProxyDeploymentService) RestoreLLMProxyDeployment(ctx context.Context, proxyID, deploymentID, gatewayID, ouID string) (*models.Deployment, error) {
	// Get proxy
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}

	// Get target deployment
	deployment, err := s.deploymentRepo.GetWithContent(deploymentID, proxy.UUID.String(), ouID)
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
	currentDeploymentID, status, _, err := s.deploymentRepo.GetStatus(proxy.UUID.String(), ouID, gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment status: %w", err)
	}
	if currentDeploymentID == deploymentID && status == models.DeploymentStatusDeployed {
		return nil, utils.ErrDeploymentAlreadyDeployed
	}

	// Update status to deployed
	updatedAt, err := s.deploymentRepo.SetCurrent(proxy.UUID.String(), ouID, gatewayID, deploymentID, models.DeploymentStatusDeployed)
	if err != nil {
		return nil, fmt.Errorf("failed to restore deployment: %w", err)
	}

	deployed := models.DeploymentStatusDeployed
	deployment.Status = &deployed
	deployment.UpdatedAt = &updatedAt

	// Broadcast deployment event to gateway (restore is treated as a deployment)
	vhost := ""
	if proxy.Configuration.Vhost != nil {
		vhost = *proxy.Configuration.Vhost
	}

	deploymentEvent := &models.LLMProxyDeploymentEvent{
		ProxyID:        proxyID,
		DeploymentID:   deploymentID,
		Vhost:          vhost,
		Environment:    "production",
		GatewayID:      gatewayID,
		OrganizationID: ouID,
		Status:         string(models.DeploymentStatusDeployed),
	}
	if err := s.gatewayEventsService.BroadcastLLMProxyDeploymentEvent(gatewayID, deploymentEvent); err != nil {
		logger.GetLogger(ctx).Error("LLMProxyDeploymentService.RestoreLLMProxyDeployment: failed to broadcast deployment event",
			"proxy_id", proxyID, "deployment_id", deploymentID, "gateway_id", gatewayID, "error", err)
		// Don't fail the restore if broadcast fails - status is already updated
	} else {
		logger.GetLogger(ctx).Info("LLMProxyDeploymentService.RestoreLLMProxyDeployment: deployment event broadcast successfully",
			"proxy_id", proxyID, "deployment_id", deploymentID, "gateway_id", gatewayID)
	}

	return deployment, nil
}

// GetLLMProxyDeployments retrieves all deployments for a proxy
// GetDeployedGatewaysByProvider returns the gateway UUIDs where the given provider artifact is currently deployed.
func (s *LLMProxyDeploymentService) GetDeployedGatewaysByProvider(providerUUID uuid.UUID, ouID string) ([]string, error) {
	return s.deploymentRepo.GetDeployedGatewaysByProvider(providerUUID, ouID)
}

func (s *LLMProxyDeploymentService) GetLLMProxyDeployments(proxyID, ouID string, gatewayID *string, status *string) ([]*models.Deployment, error) {
	// Get proxy
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
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
	deployments, err := s.deploymentRepo.GetDeploymentsWithState(proxy.UUID.String(), ouID, gatewayID, status, maxDeploymentsPerAPI)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployments: %w", err)
	}

	return deployments, nil
}

// GetLLMProxyDeployment retrieves a specific deployment
func (s *LLMProxyDeploymentService) GetLLMProxyDeployment(proxyID, deploymentID, ouID string) (*models.Deployment, error) {
	// Get proxy
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}

	// Get deployment
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, proxy.UUID.String(), ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrDeploymentNotFound
	}

	return deployment, nil
}

// DeleteLLMProxyDeployment deletes a deployment
func (s *LLMProxyDeploymentService) DeleteLLMProxyDeployment(proxyID, deploymentID, ouID string) error {
	// Get proxy
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrLLMProxyNotFound
		}
		return fmt.Errorf("failed to get proxy: %w", err)
	}
	if proxy == nil {
		return utils.ErrLLMProxyNotFound
	}

	// Get deployment
	deployment, err := s.deploymentRepo.GetWithState(deploymentID, proxy.UUID.String(), ouID)
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
	if err := s.deploymentRepo.Delete(deploymentID, proxy.UUID.String(), ouID); err != nil {
		return fmt.Errorf("failed to delete deployment: %w", err)
	}

	return nil
}

// generateLLMProxyDeploymentYAML generates deployment YAML for an LLM proxy
func (s *LLMProxyDeploymentService) generateLLMProxyDeploymentYAML(proxy *models.LLMProxy, ouID string) (string, error) {
	if proxy == nil {
		return "", errors.New("proxy is required")
	}
	if proxy.Configuration.Provider == "" {
		return "", utils.ErrInvalidInput
	}

	// Get provider to validate it exists
	provider, err := s.providerRepo.GetByUUID(proxy.Configuration.Provider, ouID)
	if err != nil {
		return "", fmt.Errorf("failed to get provider: %w", err)
	}
	if provider == nil {
		return "", utils.ErrLLMProviderNotFound
	}

	// Set default context if not provided
	contextValue := "/"
	if proxy.Configuration.Context != nil && *proxy.Configuration.Context != "" {
		contextValue = *proxy.Configuration.Context
	}

	vhostValue := ""
	if proxy.Configuration.Vhost != nil {
		vhostValue = *proxy.Configuration.Vhost
	}

	// Initialize policies slice
	policies := make([]models.LLMPolicy, 0, len(proxy.Configuration.Policies))

	// Transform security config to policy if enabled
	security := proxy.Configuration.Security
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

			// Add API key auth as a policy
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

	// Process and normalize policies
	for _, p := range proxy.Configuration.Policies {
		// Deep copy paths
		paths := make([]models.LLMPolicyPath, 0, len(p.Paths))
		for _, pp := range p.Paths {
			paths = append(paths, models.LLMPolicyPath{
				Path:    pp.Path,
				Methods: pp.Methods,
				Params:  pp.Params,
			})
		}
		// Add policy with normalized version
		policies = append(policies, models.LLMPolicy{
			Name:    p.Name,
			Version: normalizePolicyVersionToMajor(p.Version),
			Paths:   paths,
		})
	}

	// Build provider reference
	providerRef := LLMProxyDeploymentProvider{
		ID: provider.Artifact.Handle,
	}

	// Add upstream auth if configured
	if proxy.Configuration.UpstreamAuth != nil {
		providerRef.Auth = proxy.Configuration.UpstreamAuth
	}

	// Build deployment YAML
	deploymentYAML := LLMProxyDeploymentYAML{
		ApiVersion: apiVersionLLMProxy,
		Kind:       kindLLMProxy,
		Metadata: DeploymentMetadata{
			Name: proxy.Handle, // Use handle (artifact identifier) for metadata.name
		},
		Spec: LLMProxyDeploymentSpec{
			DisplayName: proxy.Configuration.Name,
			Version:     proxy.Configuration.Version,
			Context:     contextValue,
			VHost:       vhostValue,
			Provider:    providerRef,
			Resilience:  proxy.Configuration.Resilience,
			Policies:    policies,
		},
	}

	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(deploymentYAML)
	if err != nil {
		return "", fmt.Errorf("failed to marshal to YAML: %w", err)
	}

	return string(yamlBytes), nil
}
