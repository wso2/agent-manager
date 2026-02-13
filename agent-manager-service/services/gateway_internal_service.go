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
	"fmt"
	"time"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
)

// GatewayInternalAPIService handles internal gateway API operations
type GatewayInternalAPIService struct {
	apiRepo        repositories.APIRepository
	providerRepo   repositories.LLMProviderRepository
	deploymentRepo repositories.DeploymentRepository
	gatewayRepo    repositories.GatewayRepository
	orgRepo        repositories.OrganizationRepository
	projectRepo    repositories.ProjectRepository
}

// DeploymentNotification represents the notification from gateway
type DeploymentNotification struct {
	ProjectIdentifier string
	Configuration     APIDeploymentYAML
}

// GatewayDeploymentResponse represents the response for gateway deployment
type GatewayDeploymentResponse struct {
	APIId        string `json:"apiId"`
	DeploymentId int    `json:"deploymentId"` // Legacy field
	Message      string `json:"message"`
	Created      bool   `json:"created"`
}

// APIDeploymentYAML represents the API deployment YAML structure
type APIDeploymentYAML struct {
	ApiVersion string             `yaml:"apiVersion"`
	Kind       string             `yaml:"kind"`
	Metadata   DeploymentMetadata `yaml:"metadata"`
	Spec       APIDeploymentSpec  `yaml:"spec"`
}

// DeploymentMetadata represents metadata in deployment
type DeploymentMetadata struct {
	Name string `yaml:"name"`
}

// APIDeploymentSpec represents the spec section
type APIDeploymentSpec struct {
	Name       string      `yaml:"name"`
	Version    string      `yaml:"version"`
	Context    string      `yaml:"context"`
	Operations []Operation `yaml:"operations"`
}

// Operation represents an API operation
type Operation struct {
	Method string `yaml:"method"`
	Path   string `yaml:"path"`
}

// NewGatewayInternalAPIService creates a new gateway internal API service
func NewGatewayInternalAPIService(
	apiRepo repositories.APIRepository,
	providerRepo repositories.LLMProviderRepository,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	orgRepo repositories.OrganizationRepository,
	projectRepo repositories.ProjectRepository,
) *GatewayInternalAPIService {
	return &GatewayInternalAPIService{
		apiRepo:        apiRepo,
		providerRepo:   providerRepo,
		deploymentRepo: deploymentRepo,
		gatewayRepo:    gatewayRepo,
		orgRepo:        orgRepo,
		projectRepo:    projectRepo,
	}
}

// GetAPIsByOrganization retrieves all APIs for a specific organization (used by gateways)
func (s *GatewayInternalAPIService) GetAPIsByOrganization(orgID string) (map[string]string, error) {
	// Get all APIs for the organization
	apis, err := s.apiRepo.GetAPIsByOrganizationUUID(orgID, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve APIs: %w", err)
	}

	apiYamlMap := make(map[string]string)
	for _, api := range apis {
		// Generate YAML for each API
		apiYaml, err := generateAPIDeploymentYAML(api)
		if err != nil {
			return nil, fmt.Errorf("failed to generate API YAML: %w", err)
		}
		apiYamlMap[api.ID] = apiYaml
	}
	return apiYamlMap, nil
}

// GetAPIByUUID retrieves an API by its ID
func (s *GatewayInternalAPIService) GetAPIByUUID(apiID, orgID string) (map[string]string, error) {
	apiModel, err := s.apiRepo.GetAPIByUUID(apiID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get api: %w", err)
	}
	if apiModel == nil {
		return nil, fmt.Errorf("API not found")
	}
	if apiModel.OrganizationID != orgID {
		return nil, fmt.Errorf("API not found")
	}

	apiYaml, err := generateAPIDeploymentYAML(apiModel)
	if err != nil {
		return nil, fmt.Errorf("failed to generate API YAML: %w", err)
	}
	apiYamlMap := map[string]string{
		apiModel.Handle: apiYaml,
	}
	return apiYamlMap, nil
}

// GetActiveDeploymentByGateway retrieves the currently deployed API artifact for a specific gateway
func (s *GatewayInternalAPIService) GetActiveDeploymentByGateway(apiID, orgID, gatewayID string) (map[string]string, error) {
	// Get the active deployment for this API on this gateway
	deployment, err := s.deploymentRepo.GetCurrentByGateway(apiID, gatewayID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, fmt.Errorf("deployment not active")
	}

	// Deployment content is already stored as YAML
	apiYaml := string(deployment.Content)

	apiYamlMap := map[string]string{
		apiID: apiYaml,
	}
	return apiYamlMap, nil
}

// GetActiveLLMProviderDeploymentByGateway retrieves the currently deployed LLM provider artifact
func (s *GatewayInternalAPIService) GetActiveLLMProviderDeploymentByGateway(providerID, orgID, gatewayID string) (map[string]string, error) {
	provider, err := s.providerRepo.GetByUUID(providerID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return nil, fmt.Errorf("LLM provider not found")
	}

	deployment, err := s.deploymentRepo.GetCurrentByGateway(provider.UUID.String(), gatewayID, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, fmt.Errorf("deployment not active")
	}

	providerYaml := string(deployment.Content)
	providerYamlMap := map[string]string{
		providerID: providerYaml,
	}
	return providerYamlMap, nil
}

// CreateGatewayDeployment handles the registration of an API deployment from a gateway
func (s *GatewayInternalAPIService) CreateGatewayDeployment(
	apiHandle, orgID, gatewayID string,
	notification DeploymentNotification,
	deploymentID *string,
) (*GatewayDeploymentResponse, error) {
	// Validate input
	if apiHandle == "" || orgID == "" || gatewayID == "" {
		return nil, fmt.Errorf("invalid input")
	}

	// Check if the gateway exists
	gatewayModel, err := s.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get gateway: %w", err)
	}
	if gatewayModel == nil {
		return nil, fmt.Errorf("gateway not found")
	}
	if gatewayModel.OrganizationUUID.String() != orgID {
		return nil, fmt.Errorf("gateway not found")
	}

	// Find the project by name
	projectName := notification.ProjectIdentifier
	project, err := s.projectRepo.GetProjectByNameAndOrgID(projectName, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to get project by name: %w", err)
	}
	if project == nil {
		return nil, fmt.Errorf("project not found: %s", projectName)
	}
	projectID := project.ID

	// Check if API exists
	existingAPI, err := s.apiRepo.GetAPIMetadataByHandle(apiHandle, orgID)
	if err != nil {
		return nil, fmt.Errorf("failed to check existing API: %w", err)
	}

	apiCreated := false
	now := time.Now()
	var apiUUID string

	if existingAPI == nil {
		// Create new API from notification
		newAPI := &models.API{
			Handle:          apiHandle,
			Name:            notification.Configuration.Spec.Name,
			Version:         notification.Configuration.Spec.Version,
			ProjectID:       projectID,
			OrganizationID:  orgID,
			CreatedBy:       "admin",
			LifeCycleStatus: "CREATED",
			Kind:            notification.Configuration.Kind,
			CreatedAt:       now,
			UpdatedAt:       now,
		}

		err = s.apiRepo.CreateAPI(newAPI)
		if err != nil {
			return nil, fmt.Errorf("failed to create API: %w", err)
		}

		apiUUID = newAPI.ID
		apiCreated = true
	} else {
		if existingAPI.OrganizationUUID.String() != orgID {
			return nil, fmt.Errorf("API not found")
		}
		apiUUID = existingAPI.UUID.String()
	}

	// Check if deployment exists
	existingDeploymentID, status, _, err := s.deploymentRepo.GetStatus(apiUUID, orgID, gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to check deployment status: %w", err)
	}

	if existingDeploymentID != "" && (status == models.DeploymentStatusDeployed || status == models.DeploymentStatusUndeployed) {
		return nil, fmt.Errorf("API already deployed to this gateway")
	}

	// Create deployment record
	deploymentName := fmt.Sprintf("deployment-%d", now.Unix())
	deployed := models.DeploymentStatusDeployed

	// Generate deployment content YAML
	deploymentContent, err := yaml.Marshal(notification.Configuration)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize deployment content: %w", err)
	}

	// Parse UUIDs
	artifactUUID, err := uuid.Parse(apiUUID)
	if err != nil {
		return nil, fmt.Errorf("invalid API UUID: %w", err)
	}
	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("invalid gateway UUID: %w", err)
	}
	orgUUID, err := uuid.Parse(orgID)
	if err != nil {
		return nil, fmt.Errorf("invalid organization UUID: %w", err)
	}

	deployment := &models.Deployment{
		Name:             deploymentName,
		ArtifactUUID:     artifactUUID,
		GatewayUUID:      gwUUID,
		OrganizationUUID: orgUUID,
		Content:          deploymentContent,
		Status:           &deployed,
		CreatedAt:        now,
	}

	err = s.deploymentRepo.CreateWithLimitEnforcement(deployment, 100) // Hard limit
	if err != nil {
		return nil, fmt.Errorf("failed to create deployment record: %w", err)
	}

	return &GatewayDeploymentResponse{
		APIId:        apiUUID,
		DeploymentId: 0,
		Message:      "API deployment registered successfully",
		Created:      apiCreated,
	}, nil
}

// generateAPIDeploymentYAML generates the YAML representation of an API
func generateAPIDeploymentYAML(api *models.API) (string, error) {
	if api == nil {
		return "", fmt.Errorf("API is required")
	}

	deployment := APIDeploymentYAML{
		ApiVersion: "gateway.api-platform.wso2.com/v1alpha1",
		Kind:       api.Kind,
		Metadata: DeploymentMetadata{
			Name: api.Handle,
		},
		Spec: APIDeploymentSpec{
			Name:    api.Name,
			Version: api.Version,
			Context: "/api/v1", // Default context
		},
	}

	yamlBytes, err := yaml.Marshal(deployment)
	if err != nil {
		return "", fmt.Errorf("failed to marshal API to YAML: %w", err)
	}

	return string(yamlBytes), nil
}
