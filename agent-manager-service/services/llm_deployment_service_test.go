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
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// Test helper functions
func strPtr(s string) *string {
	return &s
}

func boolPtr(b bool) *bool {
	return &b
}

func findPolicy(policies []models.LLMPolicy, name, version string) *models.LLMPolicy {
	for _, p := range policies {
		if p.Name == name && p.Version == version {
			return &p
		}
	}
	return nil
}

// TestFormatRateLimitDuration tests the formatRateLimitDuration helper function
func TestFormatRateLimitDuration(t *testing.T) {
	tests := []struct {
		name        string
		duration    int
		unit        string
		expected    string
		expectError bool
	}{
		{name: "valid minute", duration: 5, unit: "minute", expected: "5m", expectError: false},
		{name: "valid hour", duration: 2, unit: "hour", expected: "2h", expectError: false},
		{name: "valid day", duration: 1, unit: "day", expected: "24h", expectError: false},
		{name: "valid week", duration: 1, unit: "week", expected: "168h", expectError: false},
		{name: "valid month", duration: 1, unit: "month", expected: "720h", expectError: false},
		{name: "invalid duration zero", duration: 0, unit: "minute", expected: "", expectError: true},
		{name: "invalid duration negative", duration: -1, unit: "hour", expected: "", expectError: true},
		{name: "invalid unit", duration: 1, unit: "year", expected: "", expectError: true},
		{name: "case insensitive", duration: 3, unit: "HOUR", expected: "3h", expectError: false},
		{name: "whitespace handling", duration: 4, unit: " minute ", expected: "4m", expectError: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := formatRateLimitDuration(tc.duration, tc.unit)
			if tc.expectError {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				if actual != tc.expected {
					t.Fatalf("expected %q, got %q", tc.expected, actual)
				}
			}
		})
	}
}

// TestNormalizePolicyVersionToMajor tests the normalizePolicyVersionToMajor helper function
func TestNormalizePolicyVersionToMajor(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "semantic version", input: "1.2.3", expected: "v1"},
		{name: "with v prefix", input: "v1.2.3", expected: "v1"},
		{name: "major only", input: "2", expected: "v2"},
		{name: "with v prefix major only", input: "v3", expected: "v3"},
		{name: "zero version", input: "0.1.0", expected: "v0"},
		{name: "prerelease", input: "1.0.0-alpha", expected: "v1"},
		{name: "empty string", input: "", expected: ""},
		{name: "whitespace", input: "  1.2.3  ", expected: "v1"},
		{name: "invalid non-numeric", input: "abc", expected: "abc"},
		{name: "v only", input: "v", expected: "v"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := normalizePolicyVersionToMajor(tc.input)
			if actual != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, actual)
			}
		})
	}
}

// TestAddOrAppendPolicyPath tests the addOrAppendPolicyPath helper function
func TestAddOrAppendPolicyPath(t *testing.T) {
	t.Run("adds new policy when not exists", func(t *testing.T) {
		policies := []models.LLMPolicy{}
		path := models.LLMPolicyPath{
			Path:    "/*",
			Methods: []string{"*"},
			Params:  map[string]interface{}{"key": "value"},
		}

		addOrAppendPolicyPath(&policies, "test-policy", "v0", path)

		if len(policies) != 1 {
			t.Fatalf("expected 1 policy, got %d", len(policies))
		}
		if policies[0].Name != "test-policy" {
			t.Fatalf("expected policy name test-policy, got %s", policies[0].Name)
		}
		if policies[0].Version != "v0" {
			t.Fatalf("expected policy version v0, got %s", policies[0].Version)
		}
		if len(policies[0].Paths) != 1 {
			t.Fatalf("expected 1 path, got %d", len(policies[0].Paths))
		}
	})

	t.Run("appends path to existing policy", func(t *testing.T) {
		policies := []models.LLMPolicy{
			{
				Name:    "test-policy",
				Version: "v0",
				Paths: []models.LLMPolicyPath{
					{Path: "/first", Methods: []string{"GET"}},
				},
			},
		}

		newPath := models.LLMPolicyPath{
			Path:    "/second",
			Methods: []string{"POST"},
		}

		addOrAppendPolicyPath(&policies, "test-policy", "v0", newPath)

		if len(policies) != 1 {
			t.Fatalf("expected 1 policy, got %d", len(policies))
		}
		if len(policies[0].Paths) != 2 {
			t.Fatalf("expected 2 paths, got %d", len(policies[0].Paths))
		}
	})

	t.Run("prevents duplicate paths", func(t *testing.T) {
		policies := []models.LLMPolicy{
			{
				Name:    "test-policy",
				Version: "v0",
				Paths: []models.LLMPolicyPath{
					{Path: "/*", Methods: []string{"*"}},
				},
			},
		}

		// Use identical path configuration to test duplicate prevention
		duplicatePath := models.LLMPolicyPath{
			Path:    "/*",
			Methods: []string{"*"}, // Same methods as existing path
		}

		addOrAppendPolicyPath(&policies, "test-policy", "v0", duplicatePath)

		if len(policies) != 1 {
			t.Fatalf("expected 1 policy, got %d", len(policies))
		}
		if len(policies[0].Paths) != 1 {
			t.Fatalf("expected 1 path (duplicate prevented), got %d", len(policies[0].Paths))
		}
	})

	t.Run("allows same path with different methods", func(t *testing.T) {
		policies := []models.LLMPolicy{
			{
				Name:    "test-policy",
				Version: "v0",
				Paths: []models.LLMPolicyPath{
					{Path: "/*", Methods: []string{"*"}},
				},
			},
		}

		// Different methods - should be allowed as a separate path entry
		differentMethodPath := models.LLMPolicyPath{
			Path:    "/*",
			Methods: []string{"GET"}, // Different methods than existing path
		}

		addOrAppendPolicyPath(&policies, "test-policy", "v0", differentMethodPath)

		if len(policies) != 1 {
			t.Fatalf("expected 1 policy, got %d", len(policies))
		}
		if len(policies[0].Paths) != 2 {
			t.Fatalf("expected 2 paths (same URL, different methods allowed), got %d", len(policies[0].Paths))
		}
	})
}

// TestIsBoolTrue tests the isBoolTrue helper function
func TestIsBoolTrue(t *testing.T) {
	trueVal := true
	falseVal := false

	tests := []struct {
		name     string
		input    *bool
		expected bool
	}{
		{name: "nil pointer", input: nil, expected: false},
		{name: "true pointer", input: &trueVal, expected: true},
		{name: "false pointer", input: &falseVal, expected: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := isBoolTrue(tc.input)
			if actual != tc.expected {
				t.Fatalf("expected %v, got %v", tc.expected, actual)
			}
		})
	}
}

// TestGenerateLLMProviderDeploymentYAML_BasicProvider tests basic provider deployment YAML generation
func TestGenerateLLMProviderDeploymentYAML_BasicProvider(t *testing.T) {
	service := &LLMProviderDeploymentService{}

	provider := &models.LLMProvider{
		TemplateHandle: "openai",
		Artifact: &models.Artifact{
			Handle: "openai-provider",
		},
		Configuration: models.LLMProviderConfig{
			Name:    "OpenAI Provider",
			Version: "v1.0",
			Context: strPtr("/"),
			Upstream: &models.UpstreamConfig{
				Main: &models.UpstreamEndpoint{
					URL: "https://api.openai.com",
				},
			},
		},
	}

	yamlStr, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var out LLMProviderDeploymentYAML
	if err := yaml.Unmarshal([]byte(yamlStr), &out); err != nil {
		t.Fatalf("failed to unmarshal generated yaml: %v", err)
	}

	// Verify basic fields
	if out.ApiVersion != apiVersionLLMProvider {
		t.Fatalf("expected apiVersion %s, got: %s", apiVersionLLMProvider, out.ApiVersion)
	}
	if out.Kind != kindLLMProvider {
		t.Fatalf("expected kind %s, got: %s", kindLLMProvider, out.Kind)
	}
	if out.Metadata.Name != "openai-provider" {
		t.Fatalf("expected metadata name openai-provider, got: %s", out.Metadata.Name)
	}
	if out.Spec.DisplayName != "OpenAI Provider" {
		t.Fatalf("expected displayName 'OpenAI Provider', got: %s", out.Spec.DisplayName)
	}
	if out.Spec.Version != "v1.0" {
		t.Fatalf("expected version v1.0, got: %s", out.Spec.Version)
	}
	if out.Spec.Template != "openai" {
		t.Fatalf("expected template openai, got: %s", out.Spec.Template)
	}
	if out.Spec.Upstream.URL != "https://api.openai.com" {
		t.Fatalf("expected upstream url https://api.openai.com, got: %s", out.Spec.Upstream.URL)
	}

	// Verify default access control
	if out.Spec.AccessControl == nil {
		t.Fatalf("expected access control to be set")
	}
	if out.Spec.AccessControl.Mode != "deny_all" {
		t.Fatalf("expected access control mode deny_all, got: %s", out.Spec.AccessControl.Mode)
	}

	// Verify no policies by default
	if len(out.Spec.Policies) != 0 {
		t.Fatalf("expected 0 policies, got: %d", len(out.Spec.Policies))
	}
}

// TestGenerateLLMProviderDeploymentYAML_WithSecurityAPIKey tests security transformation
func TestGenerateLLMProviderDeploymentYAML_WithSecurityAPIKey(t *testing.T) {
	service := &LLMProviderDeploymentService{}
	trueValue := true

	provider := &models.LLMProvider{
		TemplateHandle: "openai",
		Artifact: &models.Artifact{
			Handle: "openai-provider",
		},
		Configuration: models.LLMProviderConfig{
			Name:    "OpenAI Provider",
			Version: "v1.0",
			Context: strPtr("/"),
			Upstream: &models.UpstreamConfig{
				Main: &models.UpstreamEndpoint{
					URL: "https://api.openai.com",
				},
			},
			Security: &models.SecurityConfig{
				Enabled: &trueValue,
				APIKey: &models.APIKeySecurity{
					Enabled: &trueValue,
					Key:     "X-API-Key",
					In:      "header",
				},
			},
		},
	}

	yamlStr, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var out LLMProviderDeploymentYAML
	if err := yaml.Unmarshal([]byte(yamlStr), &out); err != nil {
		t.Fatalf("failed to unmarshal generated yaml: %v", err)
	}

	// Verify security is transformed to policy
	if len(out.Spec.Policies) != 1 {
		t.Fatalf("expected 1 policy (api-key-auth), got: %d", len(out.Spec.Policies))
	}

	policy := out.Spec.Policies[0]
	if policy.Name != apiKeyAuthPolicyName {
		t.Fatalf("expected policy name %s, got: %s", apiKeyAuthPolicyName, policy.Name)
	}
	if policy.Version != apiKeyAuthPolicyVersion {
		t.Fatalf("expected policy version %s, got: %s", apiKeyAuthPolicyVersion, policy.Version)
	}
	if len(policy.Paths) != 1 {
		t.Fatalf("expected 1 policy path, got: %d", len(policy.Paths))
	}

	path := policy.Paths[0]
	if path.Path != "/*" {
		t.Fatalf("expected policy path /*, got: %s", path.Path)
	}
	if len(path.Methods) != 1 || path.Methods[0] != "*" {
		t.Fatalf("expected methods [*], got: %#v", path.Methods)
	}
	if path.Params["key"] != "X-API-Key" {
		t.Fatalf("expected params.key X-API-Key, got: %#v", path.Params["key"])
	}
	if path.Params["in"] != "header" {
		t.Fatalf("expected params.in header, got: %#v", path.Params["in"])
	}

	// Verify raw security field is nil
	if out.Spec.Security != nil {
		t.Fatalf("expected spec.security to be nil (transformed to policy)")
	}
}

// TestGenerateLLMProviderDeploymentYAML_WithGlobalTokenRateLimit tests global token rate limit transformation
func TestGenerateLLMProviderDeploymentYAML_WithGlobalTokenRateLimit(t *testing.T) {
	service := &LLMProviderDeploymentService{}

	provider := &models.LLMProvider{
		TemplateHandle: "openai",
		Artifact: &models.Artifact{
			Handle: "openai-provider",
		},
		Configuration: models.LLMProviderConfig{
			Name:    "OpenAI Provider",
			Version: "v1.0",
			Context: strPtr("/"),
			Upstream: &models.UpstreamConfig{
				Main: &models.UpstreamEndpoint{
					URL: "https://api.openai.com",
				},
			},
			RateLimiting: &models.LLMRateLimitingConfig{
				ProviderLevel: &models.RateLimitingScopeConfig{
					Global: &models.RateLimitingLimitConfig{
						Token: &models.TokenRateLimit{
							Enabled: true,
							Count:   10000,
							Reset: models.RateLimitResetWindow{
								Duration: 1,
								Unit:     "hour",
							},
						},
					},
				},
			},
		},
	}

	yamlStr, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var out LLMProviderDeploymentYAML
	if err := yaml.Unmarshal([]byte(yamlStr), &out); err != nil {
		t.Fatalf("failed to unmarshal generated yaml: %v", err)
	}

	// Verify token rate limit is transformed to policy
	if len(out.Spec.Policies) != 1 {
		t.Fatalf("expected 1 policy (token-based-ratelimit), got: %d", len(out.Spec.Policies))
	}

	policy := findPolicy(out.Spec.Policies, tokenBasedRateLimitPolicyName, rateLimitPolicyVersion)
	if policy == nil {
		t.Fatalf("expected %s policy to exist", tokenBasedRateLimitPolicyName)
	}

	if len(policy.Paths) != 1 {
		t.Fatalf("expected 1 policy path, got: %d", len(policy.Paths))
	}

	path := policy.Paths[0]
	if path.Path != "/*" {
		t.Fatalf("expected policy path /*, got: %s", path.Path)
	}

	// Verify params structure
	if path.Params["totalTokenLimits"] == nil {
		t.Fatalf("expected totalTokenLimits param to exist")
	}

	// Verify raw rate limiting field is nil
	if out.Spec.RateLimiting != nil {
		t.Fatalf("expected spec.rateLimiting to be nil (transformed to policy)")
	}
}

// TestGenerateLLMProviderDeploymentYAML_WithResourceWiseRateLimit tests resource-wise rate limit transformation
func TestGenerateLLMProviderDeploymentYAML_WithResourceWiseRateLimit(t *testing.T) {
	service := &LLMProviderDeploymentService{}

	provider := &models.LLMProvider{
		TemplateHandle: "openai",
		Artifact: &models.Artifact{
			Handle: "openai-provider",
		},
		Configuration: models.LLMProviderConfig{
			Name:    "OpenAI Provider",
			Version: "v1.0",
			Context: strPtr("/"),
			Upstream: &models.UpstreamConfig{
				Main: &models.UpstreamEndpoint{
					URL: "https://api.openai.com",
				},
			},
			RateLimiting: &models.LLMRateLimitingConfig{
				ProviderLevel: &models.RateLimitingScopeConfig{
					ResourceWise: &models.ResourceWiseRateLimitingConfig{
						Default: models.RateLimitingLimitConfig{
							Request: &models.RequestRateLimit{
								Enabled: true,
								Count:   100,
								Reset: models.RateLimitResetWindow{
									Duration: 1,
									Unit:     "minute",
								},
							},
						},
						Resources: []models.RateLimitingResourceLimit{
							{
								Resource: "/chat/completions",
								Limit: models.RateLimitingLimitConfig{
									Request: &models.RequestRateLimit{
										Enabled: true,
										Count:   50,
										Reset: models.RateLimitResetWindow{
											Duration: 1,
											Unit:     "minute",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	yamlStr, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var out LLMProviderDeploymentYAML
	if err := yaml.Unmarshal([]byte(yamlStr), &out); err != nil {
		t.Fatalf("failed to unmarshal generated yaml: %v", err)
	}

	// Verify request rate limit policies
	policy := findPolicy(out.Spec.Policies, advancedRateLimitPolicyName, rateLimitPolicyVersion)
	if policy == nil {
		t.Fatalf("expected %s policy to exist", advancedRateLimitPolicyName)
	}

	// Should have 2 paths: default (/*) and resource-specific (/chat/completions)
	if len(policy.Paths) != 2 {
		t.Fatalf("expected 2 policy paths, got: %d", len(policy.Paths))
	}

	// Find default path
	var defaultPath *models.LLMPolicyPath
	var resourcePath *models.LLMPolicyPath
	for i := range policy.Paths {
		switch policy.Paths[i].Path {
		case "/*":
			defaultPath = &policy.Paths[i]
		case "/chat/completions":
			resourcePath = &policy.Paths[i]
		}
	}

	if defaultPath == nil {
		t.Fatalf("expected default path /* to exist")
	}
	if resourcePath == nil {
		t.Fatalf("expected resource path /chat/completions to exist")
	}
}

// TestGenerateLLMProviderDeploymentYAML_WithUserDefinedPolicies tests user-defined policy handling
func TestGenerateLLMProviderDeploymentYAML_WithUserDefinedPolicies(t *testing.T) {
	service := &LLMProviderDeploymentService{}
	trueValue := true

	provider := &models.LLMProvider{
		TemplateHandle: "openai",
		Artifact: &models.Artifact{
			Handle: "openai-provider",
		},
		Configuration: models.LLMProviderConfig{
			Name:    "OpenAI Provider",
			Version: "v1.0",
			Context: strPtr("/"),
			Upstream: &models.UpstreamConfig{
				Main: &models.UpstreamEndpoint{
					URL: "https://api.openai.com",
				},
			},
			Security: &models.SecurityConfig{
				Enabled: &trueValue,
				APIKey: &models.APIKeySecurity{
					Enabled: &trueValue,
					Key:     "X-API-Key",
					In:      "header",
				},
			},
			Policies: []models.LLMPolicy{
				{
					Name:    "custom-guardrail",
					Version: "1.2.3", // Should be normalized to v1
					Paths: []models.LLMPolicyPath{
						{
							Path:    "/*",
							Methods: []string{"POST"},
							Params: map[string]interface{}{
								"max": 1000,
							},
						},
					},
				},
			},
		},
	}

	yamlStr, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var out LLMProviderDeploymentYAML
	if err := yaml.Unmarshal([]byte(yamlStr), &out); err != nil {
		t.Fatalf("failed to unmarshal generated yaml: %v", err)
	}

	// Should have 2 policies: api-key-auth + custom-guardrail
	if len(out.Spec.Policies) != 2 {
		t.Fatalf("expected 2 policies, got: %d", len(out.Spec.Policies))
	}

	apiKeyPolicy := findPolicy(out.Spec.Policies, apiKeyAuthPolicyName, apiKeyAuthPolicyVersion)
	if apiKeyPolicy == nil {
		t.Fatalf("expected api-key-auth policy to exist")
	}

	customPolicy := findPolicy(out.Spec.Policies, "custom-guardrail", "v1")
	if customPolicy == nil {
		t.Fatalf("expected custom-guardrail policy with normalized version v1 to exist")
	}

	if len(customPolicy.Paths) != 1 {
		t.Fatalf("expected 1 path in custom policy, got: %d", len(customPolicy.Paths))
	}
}

// TestGenerateLLMProviderDeploymentYAML_ValidationErrors tests validation error handling
func TestGenerateLLMProviderDeploymentYAML_ValidationErrors(t *testing.T) {
	service := &LLMProviderDeploymentService{}

	t.Run("nil provider", func(t *testing.T) {
		_, err := service.generateLLMProviderDeploymentYAML(nil, "test-org")
		if err == nil {
			t.Fatalf("expected error for nil provider")
		}
	})

	t.Run("missing template handle", func(t *testing.T) {
		provider := &models.LLMProvider{
			TemplateHandle: "",
			Configuration: models.LLMProviderConfig{
				Upstream: &models.UpstreamConfig{
					Main: &models.UpstreamEndpoint{URL: "https://api.openai.com"},
				},
			},
		}
		_, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
		if err == nil {
			t.Fatalf("expected error for missing template handle")
		}
	})

	t.Run("nil upstream", func(t *testing.T) {
		provider := &models.LLMProvider{
			TemplateHandle: "openai",
			Configuration: models.LLMProviderConfig{
				Upstream: nil,
			},
		}
		_, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
		if err == nil {
			t.Fatalf("expected error for nil upstream")
		}
	})

	t.Run("nil upstream main", func(t *testing.T) {
		provider := &models.LLMProvider{
			TemplateHandle: "openai",
			Configuration: models.LLMProviderConfig{
				Upstream: &models.UpstreamConfig{
					Main: nil,
				},
			},
		}
		_, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
		if err == nil {
			t.Fatalf("expected error for nil upstream main")
		}
	})

	t.Run("missing url and ref", func(t *testing.T) {
		provider := &models.LLMProvider{
			TemplateHandle: "openai",
			Artifact: &models.Artifact{
				Handle: "test",
			},
			Configuration: models.LLMProviderConfig{
				Upstream: &models.UpstreamConfig{
					Main: &models.UpstreamEndpoint{
						URL: "",
						Ref: "",
					},
				},
			},
		}
		_, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
		if err == nil {
			t.Fatalf("expected error for missing URL and Ref")
		}
	})

	t.Run("invalid api key security - empty key", func(t *testing.T) {
		trueValue := true
		provider := &models.LLMProvider{
			TemplateHandle: "openai",
			Artifact: &models.Artifact{
				Handle: "test",
			},
			Configuration: models.LLMProviderConfig{
				Upstream: &models.UpstreamConfig{
					Main: &models.UpstreamEndpoint{URL: "https://api.openai.com"},
				},
				Security: &models.SecurityConfig{
					Enabled: &trueValue,
					APIKey: &models.APIKeySecurity{
						Enabled: &trueValue,
						Key:     "",
						In:      "header",
					},
				},
			},
		}
		_, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
		if err == nil {
			t.Fatalf("expected error for empty api key")
		}
	})

	t.Run("invalid api key security - invalid in", func(t *testing.T) {
		trueValue := true
		provider := &models.LLMProvider{
			TemplateHandle: "openai",
			Artifact: &models.Artifact{
				Handle: "test",
			},
			Configuration: models.LLMProviderConfig{
				Upstream: &models.UpstreamConfig{
					Main: &models.UpstreamEndpoint{URL: "https://api.openai.com"},
				},
				Security: &models.SecurityConfig{
					Enabled: &trueValue,
					APIKey: &models.APIKeySecurity{
						Enabled: &trueValue,
						Key:     "X-API-Key",
						In:      "invalid",
					},
				},
			},
		}
		_, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
		if err == nil {
			t.Fatalf("expected error for invalid 'in' value")
		}
	})

	t.Run("invalid rate limit duration", func(t *testing.T) {
		provider := &models.LLMProvider{
			TemplateHandle: "openai",
			Artifact: &models.Artifact{
				Handle: "test",
			},
			Configuration: models.LLMProviderConfig{
				Upstream: &models.UpstreamConfig{
					Main: &models.UpstreamEndpoint{URL: "https://api.openai.com"},
				},
				RateLimiting: &models.LLMRateLimitingConfig{
					ProviderLevel: &models.RateLimitingScopeConfig{
						Global: &models.RateLimitingLimitConfig{
							Token: &models.TokenRateLimit{
								Enabled: true,
								Count:   1000,
								Reset: models.RateLimitResetWindow{
									Duration: -1,
									Unit:     "hour",
								},
							},
						},
					},
				},
			},
		}
		_, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
		if err == nil {
			t.Fatalf("expected error for invalid duration")
		}
	})
}

// TestGenerateLLMProviderDeploymentYAML_InvalidAccessControlMode tests that invalid access control modes are corrected
func TestGenerateLLMProviderDeploymentYAML_InvalidAccessControlMode(t *testing.T) {
	service := &LLMProviderDeploymentService{}

	provider := &models.LLMProvider{
		TemplateHandle: "openai",
		Artifact: &models.Artifact{
			Handle: "openai-provider",
		},
		Configuration: models.LLMProviderConfig{
			Name:    "OpenAI Provider",
			Version: "v1.0",
			Context: strPtr("/"),
			Upstream: &models.UpstreamConfig{
				Main: &models.UpstreamEndpoint{
					URL: "https://api.openai.com",
				},
			},
			AccessControl: &models.LLMAccessControl{
				Mode: "custom_mode", // Invalid mode
			},
		},
	}

	yamlStr, err := service.generateLLMProviderDeploymentYAML(provider, "test-org")
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	var out LLMProviderDeploymentYAML
	if err := yaml.Unmarshal([]byte(yamlStr), &out); err != nil {
		t.Fatalf("failed to unmarshal generated yaml: %v", err)
	}

	// Should have corrected the mode to "deny_all"
	if out.Spec.AccessControl == nil {
		t.Fatalf("expected AccessControl to be set")
	}
	if out.Spec.AccessControl.Mode != "deny_all" {
		t.Errorf("expected AccessControl.Mode to be 'deny_all', got: %s", out.Spec.AccessControl.Mode)
	}

	// Verify the original provider's AccessControl.Mode was NOT mutated
	if provider.Configuration.AccessControl.Mode != "custom_mode" {
		t.Errorf("original provider AccessControl.Mode was mutated, expected 'custom_mode', got: %s", provider.Configuration.AccessControl.Mode)
	}
}
