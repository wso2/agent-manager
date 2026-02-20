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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// GatewayInternalAPIService handles internal gateway API operations
type GatewayInternalAPIService struct {
	providerRepo     repositories.LLMProviderRepository
	proxyRepo        repositories.LLMProxyRepository
	deploymentRepo   repositories.DeploymentRepository
	gatewayRepo      repositories.GatewayRepository
	infraResourceMgr InfraResourceManager
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
	providerRepo repositories.LLMProviderRepository,
	proxyRepo repositories.LLMProxyRepository,
	deploymentRepo repositories.DeploymentRepository,
	gatewayRepo repositories.GatewayRepository,
	infraResourceMgr InfraResourceManager,
) *GatewayInternalAPIService {
	return &GatewayInternalAPIService{
		providerRepo:     providerRepo,
		proxyRepo:        proxyRepo,
		deploymentRepo:   deploymentRepo,
		gatewayRepo:      gatewayRepo,
		infraResourceMgr: infraResourceMgr,
	}
}

// GetActiveDeploymentByGateway retrieves the currently deployed API artifact for a specific gateway
func (s *GatewayInternalAPIService) GetActiveDeploymentByGateway(apiID, orgName, gatewayID string) (map[string]string, error) {
	// Get the active deployment for this API on this gateway
	deployment, err := s.deploymentRepo.GetCurrentByGateway(apiID, gatewayID, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrDeploymentNotActive
	}

	// Deployment content is already stored as YAML
	apiYaml := string(deployment.Content)

	apiYamlMap := map[string]string{
		apiID: apiYaml,
	}
	return apiYamlMap, nil
}

// GetActiveLLMProviderDeploymentByGateway retrieves the currently deployed LLM provider artifact
func (s *GatewayInternalAPIService) GetActiveLLMProviderDeploymentByGateway(providerID, orgName, gatewayID string) (map[string]string, error) {
	provider, err := s.providerRepo.GetByUUID(providerID, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return nil, fmt.Errorf("LLM provider not found")
	}

	deployment, err := s.deploymentRepo.GetCurrentByGateway(provider.UUID.String(), gatewayID, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrDeploymentNotActive
	}

	providerYaml := string(deployment.Content)
	providerYamlMap := map[string]string{
		providerID: providerYaml,
	}
	return providerYamlMap, nil
}

// GetActiveLLMProxyDeploymentByGateway retrieves the currently deployed LLM proxy artifact
func (s *GatewayInternalAPIService) GetActiveLLMProxyDeploymentByGateway(proxyID, orgName, gatewayID string) (map[string]string, error) {
	proxy, err := s.proxyRepo.GetByID(proxyID, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}

	deployment, err := s.deploymentRepo.GetCurrentByGateway(proxy.UUID.String(), gatewayID, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrDeploymentNotActive
	}

	proxyYaml := string(deployment.Content)
	proxyYamlMap := map[string]string{
		proxyID: proxyYaml,
	}
	return proxyYamlMap, nil
}
