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
	"fmt"
	"log/slog"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/secretmanagersvc"
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
	secretClient     secretmanagersvc.SecretManagementClient
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
	secretClient secretmanagersvc.SecretManagementClient,
) *GatewayInternalAPIService {
	return &GatewayInternalAPIService{
		providerRepo:     providerRepo,
		proxyRepo:        proxyRepo,
		deploymentRepo:   deploymentRepo,
		gatewayRepo:      gatewayRepo,
		infraResourceMgr: infraResourceMgr,
		secretClient:     secretClient,
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

	// Resolve secret references in the YAML
	resolvedYaml, err := s.resolveSecretsInYAML(providerYaml, "upstream.auth")
	if err != nil {
		slog.Error("GatewayInternalAPIService: failed to resolve secrets in provider YAML",
			"providerID", providerID, "error", err)
		return nil, fmt.Errorf("failed to resolve secrets: %w", err)
	}

	providerYamlMap := map[string]string{
		providerID: resolvedYaml,
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

	// Resolve secret references in the YAML
	resolvedYaml, err := s.resolveSecretsInYAML(proxyYaml, "provider.auth")
	if err != nil {
		slog.Error("GatewayInternalAPIService: failed to resolve secrets in proxy YAML",
			"proxyID", proxyID, "error", err)
		return nil, fmt.Errorf("failed to resolve secrets: %w", err)
	}

	proxyYamlMap := map[string]string{
		proxyID: resolvedYaml,
	}
	return proxyYamlMap, nil
}

// resolveSecretsInYAML parses YAML, finds auth.secretRef, resolves from KV,
// and replaces secretRef with the actual value.
// authPath indicates where in the YAML structure the auth block lives
// (e.g., "upstream.auth" for providers, "provider.auth" for proxies).
func (s *GatewayInternalAPIService) resolveSecretsInYAML(yamlContent, authPath string) (string, error) {
	var doc map[string]interface{}
	err := yaml.Unmarshal([]byte(yamlContent), &doc)
	if err != nil {
		return yamlContent, fmt.Errorf("failed to unmarshal YAML: %w", err)
	}
	if doc == nil {
		return yamlContent, nil // Return as-is if not parseable
	}

	spec, ok := doc["spec"].(map[string]interface{})
	if !ok {
		return yamlContent, nil
	}

	// Navigate to the auth block based on authPath
	var auth map[string]interface{}
	parts := strings.Split(authPath, ".")
	current := spec
	for i, part := range parts {
		if i == len(parts)-1 {
			if a, ok := current[part].(map[string]interface{}); ok {
				auth = a
			}
		} else {
			if next, ok := current[part].(map[string]interface{}); ok {
				current = next
			} else {
				return yamlContent, nil // Path doesn't exist, nothing to resolve
			}
		}
	}

	if auth == nil {
		return yamlContent, nil
	}

	secretRef, ok := auth["secretRef"].(string)
	if !ok || secretRef == "" {
		return yamlContent, nil // No secretRef, return as-is (may have legacy value)
	}

	// Resolve from KV
	secretData, err := s.secretClient.GetSecret(context.Background(), secretRef)
	if err != nil {
		return "", fmt.Errorf("failed to resolve secret at %q: %w", secretRef, err)
	}

	// Replace secretRef with resolved value
	if val, exists := secretData["api-key"]; exists {
		auth["value"] = val
		delete(auth, "secretRef")
	}

	// Re-marshal to YAML
	resolved, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("failed to re-marshal YAML after secret resolution: %w", err)
	}

	return string(resolved), nil
}
