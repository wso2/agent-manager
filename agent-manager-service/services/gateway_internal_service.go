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
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// GatewayInternalAPIService handles internal gateway API operations
type GatewayInternalAPIService struct {
	providerRepo     repositories.LLMProviderRepository
	proxyRepo        repositories.LLMProxyRepository
	deploymentRepo   repositories.DeploymentRepository
	gatewayRepo      repositories.GatewayRepository
	infraResourceMgr InfraResourceManager
	encryptionKey    []byte
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
	encryptionKey []byte,
) *GatewayInternalAPIService {
	return &GatewayInternalAPIService{
		providerRepo:     providerRepo,
		proxyRepo:        proxyRepo,
		deploymentRepo:   deploymentRepo,
		gatewayRepo:      gatewayRepo,
		infraResourceMgr: infraResourceMgr,
		encryptionKey:    encryptionKey,
	}
}

// GetActiveMCPProxyDeploymentByGateway retrieves the currently deployed MCP artifact for the
// given UUID on this gateway. The UUID may identify a source MCP proxy (mcp_proxies) or an
// agent-scoped mapping artifact (artifacts); both live in the deployments table keyed by
// artifact_uuid, so a single lookup handles both cases.
func (s *GatewayInternalAPIService) GetActiveMCPProxyDeploymentByGateway(ctx context.Context, proxyID, ouID, gatewayID string) (map[string]string, error) {
	deployment, err := s.deploymentRepo.GetCurrentByGateway(proxyID, gatewayID, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrMCPProxyNotFound
	}

	resolvedYaml, err := s.resolveAllSecretsInYAML(ctx, string(deployment.Content))
	if err != nil {
		logger.GetLogger(ctx).Warn("GatewayInternalAPIService: failed to resolve secrets in MCP proxy YAML",
			"proxy_id", proxyID, "error", err)
		return nil, fmt.Errorf("failed to resolve secrets: %w", err)
	}

	return map[string]string{proxyID: resolvedYaml}, nil
}

// GetActiveDeploymentByGateway retrieves the currently deployed API artifact for a specific gateway
func (s *GatewayInternalAPIService) GetActiveDeploymentByGateway(apiID, ouID, gatewayID string) (map[string]string, error) {
	// Get the active deployment for this API on this gateway
	deployment, err := s.deploymentRepo.GetCurrentByGateway(apiID, gatewayID, ouID)
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
func (s *GatewayInternalAPIService) GetActiveLLMProviderDeploymentByGateway(ctx context.Context, providerID, ouID, gatewayID string) (map[string]string, error) {
	provider, err := s.providerRepo.GetByUUID(providerID, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return nil, fmt.Errorf("LLM provider not found")
	}

	deployment, err := s.deploymentRepo.GetCurrentByGateway(provider.UUID.String(), gatewayID, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrDeploymentNotActive
	}

	providerYaml := string(deployment.Content)

	// Resolve secret references in the YAML
	resolvedYaml, err := s.resolveSecretsInYAML(ctx, providerYaml, "upstream.auth")
	if err != nil {
		logger.GetLogger(ctx).Warn("GatewayInternalAPIService: failed to resolve secrets in provider YAML",
			"provider_id", providerID, "error", err)
		return nil, fmt.Errorf("failed to resolve secrets: %w", err)
	}

	providerYamlMap := map[string]string{
		providerID: resolvedYaml,
	}
	return providerYamlMap, nil
}

// GetActiveLLMProxyDeploymentByGateway retrieves the currently deployed LLM proxy artifact
func (s *GatewayInternalAPIService) GetActiveLLMProxyDeploymentByGateway(ctx context.Context, proxyID, ouID, gatewayID string) (map[string]string, error) {
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get LLM proxy: %w", err)
	}
	if proxy == nil {
		return nil, utils.ErrLLMProxyNotFound
	}

	deployment, err := s.deploymentRepo.GetCurrentByGateway(proxy.UUID.String(), gatewayID, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, utils.ErrDeploymentNotActive
	}

	proxyYaml := string(deployment.Content)

	// Resolve secret references in the YAML
	resolvedYaml, err := s.resolveSecretsInYAML(ctx, proxyYaml, "provider.auth")
	if err != nil {
		logger.GetLogger(ctx).Warn("GatewayInternalAPIService: failed to resolve secrets in proxy YAML",
			"proxy_id", proxyID, "error", err)
		return nil, fmt.Errorf("failed to resolve secrets: %w", err)
	}

	proxyYamlMap := map[string]string{
		proxyID: resolvedYaml,
	}
	return proxyYamlMap, nil
}

// resolveSecretsInYAML parses YAML, finds auth.secretRef, decrypts it,
// and replaces secretRef with the actual plaintext value.
// authPath indicates where in the YAML structure the auth block lives
// (e.g., "upstream.auth" for providers, "provider.auth" for proxies).
func (s *GatewayInternalAPIService) resolveSecretsInYAML(ctx context.Context, yamlContent, authPath string) (string, error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &doc); err != nil {
		return "", fmt.Errorf("failed to parse YAML: %w", err)
	}
	if doc == nil {
		return "", fmt.Errorf("YAML content is empty or null")
	}

	spec, ok := doc["spec"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("YAML is missing expected \"spec\" section")
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
				// Distinguish: if the parent key exists but has the wrong type, the config is malformed.
				// If the parent key is absent entirely, this is a legacy record with no auth — safe to skip.
				if _, parentExists := current[part]; parentExists {
					return "", fmt.Errorf("unexpected type for YAML path segment %q: expected map", part)
				}
				return yamlContent, nil
			}
		}
	}

	if auth == nil {
		return yamlContent, nil
	}

	secretRef, ok := auth["secretRef"].(string)
	if !ok || secretRef == "" {
		return yamlContent, nil // No secretRef, return as-is
	}

	plaintext, err := s.decryptSecretRef(secretRef)
	if err != nil {
		return "", err
	}

	// Replace secretRef with decrypted value
	auth["value"] = plaintext
	delete(auth, "secretRef")

	// Re-marshal to YAML
	resolved, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("failed to re-marshal YAML after secret resolution: %w", err)
	}

	return string(resolved), nil
}

// resolveAllSecretsInYAML walks the entire YAML and decrypts every `secretRef` field it
// finds, replacing it with `value: <plaintext>`. Use this when secrets may appear at
// multiple or unknown locations in the doc (e.g. MCP proxy YAML, where the upstream auth
// is re-expressed as headers inside a set-headers policy). For YAMLs with a single
// known secret location, prefer resolveSecretsInYAML — it's targeted and won't touch
// any field outside the specified path.
func (s *GatewayInternalAPIService) resolveAllSecretsInYAML(_ context.Context, yamlContent string) (string, error) {
	var doc interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &doc); err != nil {
		return "", fmt.Errorf("failed to parse YAML: %w", err)
	}
	changed, err := s.decryptSecretRefsIn(doc)
	if err != nil {
		return "", err
	}
	if !changed {
		return yamlContent, nil
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("failed to re-marshal YAML after secret resolution: %w", err)
	}
	return string(out), nil
}

// decryptSecretRefsIn recursively visits v. For every map that has a non-empty string
// `secretRef`, it decrypts the value and rewrites the map to carry `value: <plaintext>`
// instead. Returns true if any rewrite happened.
func (s *GatewayInternalAPIService) decryptSecretRefsIn(v interface{}) (bool, error) {
	switch x := v.(type) {
	case map[string]interface{}:
		changed := false
		if ref, ok := x["secretRef"].(string); ok && ref != "" {
			plaintext, err := s.decryptSecretRef(ref)
			if err != nil {
				return false, err
			}
			x["value"] = plaintext
			delete(x, "secretRef")
			changed = true
		}
		for _, child := range x {
			c, err := s.decryptSecretRefsIn(child)
			if err != nil {
				return changed, err
			}
			changed = changed || c
		}
		return changed, nil
	case []interface{}:
		changed := false
		for _, child := range x {
			c, err := s.decryptSecretRefsIn(child)
			if err != nil {
				return changed, err
			}
			changed = changed || c
		}
		return changed, nil
	default:
		return false, nil
	}
}

func (s *GatewayInternalAPIService) decryptSecretRef(secretRef string) (string, error) {
	ciphertext, err := base64.StdEncoding.DecodeString(secretRef)
	if err != nil {
		return "", fmt.Errorf("failed to base64-decode secretRef: %w", err)
	}
	plaintext, err := utils.DecryptBytes(ciphertext, s.encryptionKey)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt secretRef: %w", err)
	}
	return string(plaintext), nil
}
