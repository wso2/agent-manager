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
	"errors"
	"testing"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// Mock repository for LLM Provider
type mockLLMProviderRepository struct {
	provider *models.LLMProvider
	err      error
}

func (m *mockLLMProviderRepository) Create(tx *gorm.DB, p *models.LLMProvider, handle, name, version string, orgUUID string) error {
	return nil
}

func (m *mockLLMProviderRepository) GetByUUID(uuid, orgName string) (*models.LLMProvider, error) {
	return m.provider, m.err
}

func (m *mockLLMProviderRepository) GetByHandle(handle, orgName string) (*models.LLMProvider, error) {
	return m.provider, m.err
}

func (m *mockLLMProviderRepository) List(orgUUID string, limit, offset int) ([]*models.LLMProvider, error) {
	return nil, nil
}

func (m *mockLLMProviderRepository) Count(orgUUID string) (int, error) {
	return 0, nil
}

func (m *mockLLMProviderRepository) Update(p *models.LLMProvider, providerID string, orgUUID string) error {
	return nil
}

func (m *mockLLMProviderRepository) Delete(providerID, orgUUID string) error {
	return nil
}

func (m *mockLLMProviderRepository) Exists(providerID, orgUUID string) (bool, error) {
	return false, nil
}

// TestGenerateLLMProxyDeploymentYAML_Basic tests basic YAML generation
func TestGenerateLLMProxyDeploymentYAML_Basic(t *testing.T) {
	providerUUID := uuid.New().String()
	providerHandle := "openai-provider"

	mockProvider := &models.LLMProvider{
		UUID: uuid.MustParse(providerUUID),
		Artifact: &models.Artifact{
			Handle: providerHandle,
		},
	}

	service := &LLMProxyDeploymentService{
		providerRepo: &mockLLMProviderRepository{
			provider: mockProvider,
		},
	}

	proxy := &models.LLMProxy{
		Handle: "test-proxy",
		Configuration: models.LLMProxyConfig{
			Name:     "Test Proxy",
			Version:  "1.0.0",
			Provider: providerUUID,
		},
	}

	yamlStr, err := service.generateLLMProxyDeploymentYAML(proxy, "test-org")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if yamlStr == "" {
		t.Fatal("expected non-empty YAML")
	}

	// Parse and validate YAML structure
	var deployment LLMProxyDeploymentYAML
	if err := yaml.Unmarshal([]byte(yamlStr), &deployment); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	// Validate basic fields
	if deployment.ApiVersion != apiVersionLLMProxy {
		t.Errorf("expected ApiVersion %s, got %s", apiVersionLLMProxy, deployment.ApiVersion)
	}
	if deployment.Kind != kindLLMProxy {
		t.Errorf("expected Kind %s, got %s", kindLLMProxy, deployment.Kind)
	}
	if deployment.Metadata.Name != proxy.Handle {
		t.Errorf("expected metadata.name %s, got %s", proxy.Handle, deployment.Metadata.Name)
	}
	if deployment.Spec.DisplayName != proxy.Configuration.Name {
		t.Errorf("expected displayName %s, got %s", proxy.Configuration.Name, deployment.Spec.DisplayName)
	}
	if deployment.Spec.Provider.ID != providerHandle {
		t.Errorf("expected provider.id %s, got %s", providerHandle, deployment.Spec.Provider.ID)
	}
}

// TestGenerateLLMProxyDeploymentYAML_WithContext tests YAML generation with custom context
func TestGenerateLLMProxyDeploymentYAML_WithContext(t *testing.T) {
	providerUUID := uuid.New().String()
	context := "/custom-context"
	vhost := "api.example.com"

	mockProvider := &models.LLMProvider{
		UUID: uuid.MustParse(providerUUID),
		Artifact: &models.Artifact{
			Handle: "provider-handle",
		},
	}

	service := &LLMProxyDeploymentService{
		providerRepo: &mockLLMProviderRepository{
			provider: mockProvider,
		},
	}

	proxy := &models.LLMProxy{
		Handle: "test-proxy",
		Configuration: models.LLMProxyConfig{
			Name:     "Test Proxy",
			Version:  "1.0.0",
			Provider: providerUUID,
			Context:  &context,
			Vhost:    &vhost,
		},
	}

	yamlStr, err := service.generateLLMProxyDeploymentYAML(proxy, "test-org")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var deployment LLMProxyDeploymentYAML
	if err := yaml.Unmarshal([]byte(yamlStr), &deployment); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if deployment.Spec.Context != context {
		t.Errorf("expected context %s, got %s", context, deployment.Spec.Context)
	}
	if deployment.Spec.VHost != vhost {
		t.Errorf("expected vhost %s, got %s", vhost, deployment.Spec.VHost)
	}
}

// TestGenerateLLMProxyDeploymentYAML_WithSecurityAPIKey tests security to policy conversion
func TestGenerateLLMProxyDeploymentYAML_WithSecurityAPIKey(t *testing.T) {
	providerUUID := uuid.New().String()

	mockProvider := &models.LLMProvider{
		UUID: uuid.MustParse(providerUUID),
		Artifact: &models.Artifact{
			Handle: "provider-handle",
		},
	}

	service := &LLMProxyDeploymentService{
		providerRepo: &mockLLMProviderRepository{
			provider: mockProvider,
		},
	}

	enabled := true
	proxy := &models.LLMProxy{
		Handle: "test-proxy",
		Configuration: models.LLMProxyConfig{
			Name:     "Test Proxy",
			Version:  "1.0.0",
			Provider: providerUUID,
			Security: &models.SecurityConfig{
				Enabled: &enabled,
				APIKey: &models.APIKeySecurity{
					Enabled: &enabled,
					Key:     "x-api-key",
					In:      "header",
				},
			},
		},
	}

	yamlStr, err := service.generateLLMProxyDeploymentYAML(proxy, "test-org")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var deployment LLMProxyDeploymentYAML
	if err := yaml.Unmarshal([]byte(yamlStr), &deployment); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	// Security should not be in the spec (converted to policy)
	// Note: The spec doesn't include security field anymore

	// Check that API key auth policy was added
	foundPolicy := false
	for _, policy := range deployment.Spec.Policies {
		if policy.Name == apiKeyAuthPolicyName && policy.Version == apiKeyAuthPolicyVersion {
			foundPolicy = true
			if len(policy.Paths) != 1 {
				t.Errorf("expected 1 path, got %d", len(policy.Paths))
			}
			if policy.Paths[0].Path != "/*" {
				t.Errorf("expected path /*, got %s", policy.Paths[0].Path)
			}
			if len(policy.Paths[0].Methods) != 1 || policy.Paths[0].Methods[0] != "*" {
				t.Errorf("expected methods [*], got %v", policy.Paths[0].Methods)
			}
			// Check params
			if policy.Paths[0].Params["key"] != "x-api-key" {
				t.Errorf("expected param key=x-api-key, got %v", policy.Paths[0].Params["key"])
			}
			if policy.Paths[0].Params["in"] != "header" {
				t.Errorf("expected param in=header, got %v", policy.Paths[0].Params["in"])
			}
		}
	}
	if !foundPolicy {
		t.Error("expected to find api-key-auth policy")
	}
}

// TestGenerateLLMProxyDeploymentYAML_SecurityValidation tests security validation
func TestGenerateLLMProxyDeploymentYAML_SecurityValidation(t *testing.T) {
	providerUUID := uuid.New().String()

	mockProvider := &models.LLMProvider{
		UUID: uuid.MustParse(providerUUID),
		Artifact: &models.Artifact{
			Handle: "provider-handle",
		},
	}

	service := &LLMProxyDeploymentService{
		providerRepo: &mockLLMProviderRepository{
			provider: mockProvider,
		},
	}

	tests := []struct {
		name        string
		security    *models.SecurityConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "empty API key",
			security: &models.SecurityConfig{
				Enabled: boolPtr(true),
				APIKey: &models.APIKeySecurity{
					Enabled: boolPtr(true),
					Key:     "",
					In:      "header",
				},
			},
			expectError: true,
			errorMsg:    "key is required",
		},
		{
			name: "invalid in parameter",
			security: &models.SecurityConfig{
				Enabled: boolPtr(true),
				APIKey: &models.APIKeySecurity{
					Enabled: boolPtr(true),
					Key:     "x-api-key",
					In:      "body",
				},
			},
			expectError: true,
			errorMsg:    "in must be 'header' or 'query'",
		},
		{
			name: "valid header",
			security: &models.SecurityConfig{
				Enabled: boolPtr(true),
				APIKey: &models.APIKeySecurity{
					Enabled: boolPtr(true),
					Key:     "x-api-key",
					In:      "header",
				},
			},
			expectError: false,
		},
		{
			name: "valid query",
			security: &models.SecurityConfig{
				Enabled: boolPtr(true),
				APIKey: &models.APIKeySecurity{
					Enabled: boolPtr(true),
					Key:     "apiKey",
					In:      "query",
				},
			},
			expectError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			proxy := &models.LLMProxy{
				Handle: "test-proxy",
				Configuration: models.LLMProxyConfig{
					Name:     "Test Proxy",
					Version:  "1.0.0",
					Provider: providerUUID,
					Security: tc.security,
				},
			}

			_, err := service.generateLLMProxyDeploymentYAML(proxy, "test-org")
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errorMsg != "" && !contains(err.Error(), tc.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestGenerateLLMProxyDeploymentYAML_WithPolicies tests policy processing and version normalization
func TestGenerateLLMProxyDeploymentYAML_WithPolicies(t *testing.T) {
	providerUUID := uuid.New().String()

	mockProvider := &models.LLMProvider{
		UUID: uuid.MustParse(providerUUID),
		Artifact: &models.Artifact{
			Handle: "provider-handle",
		},
	}

	service := &LLMProxyDeploymentService{
		providerRepo: &mockLLMProviderRepository{
			provider: mockProvider,
		},
	}

	proxy := &models.LLMProxy{
		Handle: "test-proxy",
		Configuration: models.LLMProxyConfig{
			Name:     "Test Proxy",
			Version:  "1.0.0",
			Provider: providerUUID,
			Policies: []models.LLMPolicy{
				{
					Name:    "rate-limit",
					Version: "1.2.3", // Should be normalized to v1
					Paths: []models.LLMPolicyPath{
						{
							Path:    "/api/*",
							Methods: []string{"GET", "POST"},
							Params: map[string]interface{}{
								"limit": 100,
							},
						},
					},
				},
				{
					Name:    "custom-policy",
					Version: "v2.0.0", // Should be normalized to v2
					Paths: []models.LLMPolicyPath{
						{
							Path:    "/*",
							Methods: []string{"*"},
							Params: map[string]interface{}{
								"enabled": true,
							},
						},
					},
				},
			},
		},
	}

	yamlStr, err := service.generateLLMProxyDeploymentYAML(proxy, "test-org")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var deployment LLMProxyDeploymentYAML
	if err := yaml.Unmarshal([]byte(yamlStr), &deployment); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if len(deployment.Spec.Policies) != 2 {
		t.Fatalf("expected 2 policies, got %d", len(deployment.Spec.Policies))
	}

	// Check first policy
	policy1 := deployment.Spec.Policies[0]
	if policy1.Name != "rate-limit" {
		t.Errorf("expected policy name rate-limit, got %s", policy1.Name)
	}
	if policy1.Version != "v1" {
		t.Errorf("expected version v1, got %s", policy1.Version)
	}

	// Check second policy
	policy2 := deployment.Spec.Policies[1]
	if policy2.Name != "custom-policy" {
		t.Errorf("expected policy name custom-policy, got %s", policy2.Name)
	}
	if policy2.Version != "v2" {
		t.Errorf("expected version v2, got %s", policy2.Version)
	}
}

// TestGenerateLLMProxyDeploymentYAML_WithUpstreamAuth tests upstream auth handling
func TestGenerateLLMProxyDeploymentYAML_WithUpstreamAuth(t *testing.T) {
	providerUUID := uuid.New().String()

	mockProvider := &models.LLMProvider{
		UUID: uuid.MustParse(providerUUID),
		Artifact: &models.Artifact{
			Handle: "provider-handle",
		},
	}

	service := &LLMProxyDeploymentService{
		providerRepo: &mockLLMProviderRepository{
			provider: mockProvider,
		},
	}

	authType := "bearer"
	header := "Authorization"
	value := "Bearer token123"

	proxy := &models.LLMProxy{
		Handle: "test-proxy",
		Configuration: models.LLMProxyConfig{
			Name:     "Test Proxy",
			Version:  "1.0.0",
			Provider: providerUUID,
			UpstreamAuth: &models.UpstreamAuth{
				Type:   &authType,
				Header: &header,
				Value:  &value,
			},
		},
	}

	yamlStr, err := service.generateLLMProxyDeploymentYAML(proxy, "test-org")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var deployment LLMProxyDeploymentYAML
	if err := yaml.Unmarshal([]byte(yamlStr), &deployment); err != nil {
		t.Fatalf("failed to unmarshal YAML: %v", err)
	}

	if deployment.Spec.Provider.Auth == nil {
		t.Fatal("expected upstream auth to be set")
	}
	if deployment.Spec.Provider.Auth.Type == nil || *deployment.Spec.Provider.Auth.Type != authType {
		t.Errorf("expected auth type %s, got %v", authType, deployment.Spec.Provider.Auth.Type)
	}
	if deployment.Spec.Provider.Auth.Header == nil || *deployment.Spec.Provider.Auth.Header != header {
		t.Errorf("expected auth header %s, got %v", header, deployment.Spec.Provider.Auth.Header)
	}
	if deployment.Spec.Provider.Auth.Value == nil || *deployment.Spec.Provider.Auth.Value != value {
		t.Errorf("expected auth value %s, got %v", value, deployment.Spec.Provider.Auth.Value)
	}
}

// TestGenerateLLMProxyDeploymentYAML_ValidationErrors tests error cases
func TestGenerateLLMProxyDeploymentYAML_ValidationErrors(t *testing.T) {
	providerUUID := uuid.New().String()

	mockProvider := &models.LLMProvider{
		UUID: uuid.MustParse(providerUUID),
		Artifact: &models.Artifact{
			Handle: "provider-handle",
		},
	}

	tests := []struct {
		name        string
		service     *LLMProxyDeploymentService
		proxy       *models.LLMProxy
		expectError bool
		errorMsg    string
	}{
		{
			name: "nil proxy",
			service: &LLMProxyDeploymentService{
				providerRepo: &mockLLMProviderRepository{provider: mockProvider},
			},
			proxy:       nil,
			expectError: true,
			errorMsg:    "proxy is required",
		},
		{
			name: "empty provider",
			service: &LLMProxyDeploymentService{
				providerRepo: &mockLLMProviderRepository{provider: mockProvider},
			},
			proxy: &models.LLMProxy{
				Handle: "test-proxy",
				Configuration: models.LLMProxyConfig{
					Name:     "Test Proxy",
					Version:  "1.0.0",
					Provider: "",
				},
			},
			expectError: true,
		},
		{
			name: "provider not found",
			service: &LLMProxyDeploymentService{
				providerRepo: &mockLLMProviderRepository{provider: nil},
			},
			proxy: &models.LLMProxy{
				Handle: "test-proxy",
				Configuration: models.LLMProxyConfig{
					Name:     "Test Proxy",
					Version:  "1.0.0",
					Provider: providerUUID,
				},
			},
			expectError: true,
		},
		{
			name: "provider fetch error",
			service: &LLMProxyDeploymentService{
				providerRepo: &mockLLMProviderRepository{err: errors.New("db error")},
			},
			proxy: &models.LLMProxy{
				Handle: "test-proxy",
				Configuration: models.LLMProxyConfig{
					Name:     "Test Proxy",
					Version:  "1.0.0",
					Provider: providerUUID,
				},
			},
			expectError: true,
			errorMsg:    "failed to get provider",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tc.service.generateLLMProxyDeploymentYAML(tc.proxy, "test-org")
			if tc.expectError {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errorMsg != "" && !contains(err.Error(), tc.errorMsg) {
					t.Errorf("expected error containing %q, got %q", tc.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
			}
		})
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
