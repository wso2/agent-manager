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
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
)

// GatewayInternalAPIService handles internal gateway API operations
type GatewayInternalAPIService struct {
	providerRepo     gwrepo.LLMProviderRepository
	proxyRepo        gwrepo.LLMProxyRepository
	deploymentRepo   gwrepo.DeploymentRepository
	artifactRepo     gwrepo.ArtifactRepository
	gatewayRepo      gwrepo.GatewayRepository
	infraResourceMgr modkit.CoreAPI
	secrets          modkit.Secrets
	artifacts        GatewayArtifacts
}

// GatewayArtifacts names an artifact as the gateway knows it: the kind, and the id it stores it under; ok is false
// for a kind AMS doesn't deploy to gateways (reconcile.Core, which asks the registered kinds).
type GatewayArtifacts interface {
	GatewayArtifact(ref gwmodels.ArtifactRef) (kind, artifactID string, ok bool)
}

// GatewayDeploymentResponse represents the response for gateway deployment
type GatewayDeploymentResponse struct {
	APIId        string `json:"apiId"`
	DeploymentId int    `json:"deploymentId"` // Legacy field
	Message      string `json:"message"`
	Created      bool   `json:"created"`
}

// DeploymentMetadata represents metadata in deployment
type DeploymentMetadata struct {
	Name        string            `yaml:"name"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
}

// NewGatewayInternalAPIService creates a new gateway internal API service
func NewGatewayInternalAPIService(
	providerRepo gwrepo.LLMProviderRepository,
	proxyRepo gwrepo.LLMProxyRepository,
	deploymentRepo gwrepo.DeploymentRepository,
	artifactRepo gwrepo.ArtifactRepository,
	gatewayRepo gwrepo.GatewayRepository,
	infraResourceMgr modkit.CoreAPI,
	secrets modkit.Secrets,
	artifacts GatewayArtifacts,
) *GatewayInternalAPIService {
	return &GatewayInternalAPIService{
		artifacts:        artifacts,
		providerRepo:     providerRepo,
		proxyRepo:        proxyRepo,
		deploymentRepo:   deploymentRepo,
		artifactRepo:     artifactRepo,
		gatewayRepo:      gatewayRepo,
		infraResourceMgr: infraResourceMgr,
		secrets:          secrets,
	}
}

// GetActiveMCPProxyDeploymentByGateway retrieves the currently deployed MCP artifact for the
// given UUID on this gateway. The UUID may identify a source MCP proxy (mcp_proxies) or an
// agent-scoped mapping artifact (artifacts); both live in the deployments table keyed by
// artifact_uuid, so a single lookup handles both cases.
func (s *GatewayInternalAPIService) GetActiveMCPProxyDeploymentByGateway(ctx context.Context, proxyID, ouID, gatewayID string) (map[string]string, error) {
	return s.getActiveArtifactDeploymentByGateway(ctx, proxyID, ouID, gatewayID, kindMCPProxy, gwerr.ErrMCPProxyNotFound)
}

// GetActiveAgentDeploymentByGateway retrieves the currently deployed A2A Agent
// artifact for the given UUID on this gateway.
//
// The UUID is the per-environment gwmodels.KindAgent artifact row, which is also
// what the agent's API keys are bound to — so the gateway resolves the Agent
// and its keys through the same identity.
func (s *GatewayInternalAPIService) GetActiveAgentDeploymentByGateway(ctx context.Context, agentID, ouID, gatewayID string) (map[string]string, error) {
	return s.getActiveArtifactDeploymentByGateway(ctx, agentID, ouID, gatewayID, gwmodels.KindAgent, gwerr.ErrAgentArtifactNotFound)
}

// getActiveArtifactDeploymentByGateway returns the artifact's deployed YAML with secrets resolved, or notFound.
func (s *GatewayInternalAPIService) getActiveArtifactDeploymentByGateway(
	ctx context.Context, artifactID, ouID, gatewayID, gatewayKind string, notFound error,
) (map[string]string, error) {
	artifactUUID, err := uuid.Parse(artifactID)
	if err != nil {
		return nil, notFound
	}
	deployment, err := s.deploymentRepo.GetPullable(ctx, ouID, artifactUUID, gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, notFound
	}

	resolvedYaml, err := s.resolveSecretsFor(ctx, ouID, gatewayKind, string(deployment.Content))
	if err != nil {
		slog.Error("GatewayInternalAPIService: failed to resolve secrets in "+gatewayKind+" YAML",
			"artifactID", artifactID, "error", err)
		return nil, fmt.Errorf("failed to resolve secrets: %w", err)
	}

	return map[string]string{artifactID: resolvedYaml}, nil
}

// GetActiveRestAPIDeploymentByGateway returns the deployed RestApi YAML of a REST agent's per-environment artifact.
func (s *GatewayInternalAPIService) GetActiveRestAPIDeploymentByGateway(ctx context.Context, apiID, ouID, gatewayID string) (map[string]string, error) {
	return s.getActiveArtifactDeploymentByGateway(ctx, apiID, ouID, gatewayID, gwmodels.KindRestAPI, gwerr.ErrAPINotFound)
}

// GetActiveLLMProviderDeploymentByGateway retrieves the currently deployed LLM provider artifact
func (s *GatewayInternalAPIService) GetActiveLLMProviderDeploymentByGateway(ctx context.Context, providerID, ouID, gatewayID string) (map[string]string, error) {
	if _, err := uuid.Parse(providerID); err != nil {
		return nil, gwerr.ErrLLMProviderNotFound
	}
	provider, err := s.providerRepo.GetByUUID(providerID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gwerr.ErrLLMProviderNotFound
		}
		return nil, fmt.Errorf("failed to get LLM provider: %w", err)
	}
	if provider == nil {
		return nil, gwerr.ErrLLMProviderNotFound
	}

	deployment, err := s.deploymentRepo.GetPullable(ctx, ouID, provider.UUID, gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, gwerr.ErrDeploymentNotActive
	}

	providerYaml := string(deployment.Content)

	// Resolve secret references in the YAML
	resolvedYaml, err := s.resolveSecretsFor(ctx, ouID, gwmodels.KindLLMProvider, providerYaml)
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
func (s *GatewayInternalAPIService) GetActiveLLMProxyDeploymentByGateway(ctx context.Context, proxyID, ouID, gatewayID string) (map[string]string, error) {
	proxy, err := s.proxyRepo.GetByID(proxyID, ouID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, gwerr.ErrLLMProxyNotFound
		}
		return nil, fmt.Errorf("failed to get LLM proxy: %w", err)
	}
	if proxy == nil {
		return nil, gwerr.ErrLLMProxyNotFound
	}

	deployment, err := s.deploymentRepo.GetPullable(ctx, ouID, proxy.UUID, gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to get deployment: %w", err)
	}
	if deployment == nil {
		return nil, gwerr.ErrDeploymentNotActive
	}

	proxyYaml := string(deployment.Content)

	// Resolve secret references in the YAML
	resolvedYaml, err := s.resolveSecretsFor(ctx, ouID, gwmodels.KindLLMProxy, proxyYaml)
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

// resolveSecretsInYAML parses YAML, finds auth.secretRef, reads the secret it names,
// and replaces secretRef with the actual plaintext value.
// authPath indicates where in the YAML structure the auth block lives
// (e.g., "upstream.auth" for providers, "provider.auth" for proxies).
func (s *GatewayInternalAPIService) resolveSecretsInYAML(ctx context.Context, ouID, yamlContent, authPath string) (string, error) {
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

	plaintext, err := s.readSecretRef(ctx, ouID, secretRef)
	if err != nil {
		return "", err
	}

	// Replace secretRef with the secret value
	auth["value"] = plaintext
	delete(auth, "secretRef")

	// Re-marshal to YAML
	resolved, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("failed to re-marshal YAML after secret resolution: %w", err)
	}

	return string(resolved), nil
}

// resolveAllSecretsInYAML walks the entire YAML and reads the secret of every `secretRef` field it
// finds, replacing it with `value: <plaintext>`. Use this when secrets may appear at
// multiple or unknown locations in the doc (e.g. MCP proxy YAML, where the upstream auth
// is re-expressed as headers inside a set-headers policy). For YAMLs with a single
// known secret location, prefer resolveSecretsInYAML — it's targeted and won't touch
// any field outside the specified path.
func (s *GatewayInternalAPIService) resolveAllSecretsInYAML(ctx context.Context, ouID, yamlContent string) (string, error) {
	var doc interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &doc); err != nil {
		return "", fmt.Errorf("failed to parse YAML: %w", err)
	}
	changed, err := s.readSecretRefsIn(ctx, ouID, doc)
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

// readSecretRefsIn recursively visits v. For every map that has a non-empty string
// `secretRef`, it reads the secret and rewrites the map to carry `value: <plaintext>`
// instead. Returns true if any rewrite happened.
func (s *GatewayInternalAPIService) readSecretRefsIn(ctx context.Context, ouID string, v interface{}) (bool, error) {
	switch x := v.(type) {
	case map[string]interface{}:
		changed := false
		if ref, ok := x["secretRef"].(string); ok && ref != "" {
			plaintext, err := s.readSecretRef(ctx, ouID, ref)
			if err != nil {
				return false, err
			}
			x["value"] = plaintext
			delete(x, "secretRef")
			changed = true
		}
		for _, child := range x {
			c, err := s.readSecretRefsIn(ctx, ouID, child)
			if err != nil {
				return changed, err
			}
			changed = changed || c
		}
		return changed, nil
	case []interface{}:
		changed := false
		for _, child := range x {
			c, err := s.readSecretRefsIn(ctx, ouID, child)
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

// readSecretRef reads the value a secretRef names in the organization's secret store. The controller-facing routes
// carry no caller organization, so it is passed explicitly.
func (s *GatewayInternalAPIService) readSecretRef(ctx context.Context, ouID, secretRef string) (string, error) {
	plaintext, err := s.secrets.Get(ctx, ouID, secretRef)
	if err != nil {
		return "", fmt.Errorf("failed to read secretRef: %w", err)
	}
	return plaintext, nil
}
