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

package controllers

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

func storedProvider() *models.LLMProvider {
	enabled := true
	return &models.LLMProvider{
		OpenAPISpec: "openapi: 3.0.0",
		ModelList:   `[{"name":"gpt-4o-mini"}]`,
		Configuration: models.LLMProviderConfig{
			Name: "acme-openai",
			Security: &models.SecurityConfig{
				Enabled: &enabled,
				APIKey:  &models.APIKeySecurity{Enabled: &enabled, Key: "x-api-key", In: "header"},
			},
			AccessControl: &models.LLMAccessControl{},
			RateLimiting:  &models.LLMRateLimitingConfig{},
			Policies:      []models.LLMPolicy{{Name: "guardrail-pii"}},
			Resilience:    &models.Resilience{},
		},
	}
}

// The repository replaces the whole configuration document, so a partial update has to
// carry forward what it did not mention. Dropping security here would leave the provider
// — and every proxy in front of it, once the dependent sync follows — asking callers for
// no credential at all, from an edit that never mentioned security.
func TestPreserveOmittedProviderFields_PartialUpdateKeepsStoredValues(t *testing.T) {
	existing := storedProvider()
	// A caller changing only the description: every other field is absent.
	description := "just a new description"
	req := &spec.UpdateLLMProviderRequest{Description: &description}

	// What conversion produces from that request: the omitted fields are all empty.
	provider := &models.LLMProvider{Configuration: models.LLMProviderConfig{Name: "acme-openai"}}

	preserveOmittedProviderFields(provider, existing, req)

	require.NotNil(t, provider.Configuration.Security, "security must survive an unrelated edit")
	require.True(t, provider.Configuration.Security.RequiresAPIKey(),
		"the provider must still require a credential after an edit that never mentioned security")
	require.Equal(t, "x-api-key", provider.Configuration.Security.APIKey.Key)
	require.NotNil(t, provider.Configuration.AccessControl, "access control must survive")
	require.NotNil(t, provider.Configuration.RateLimiting, "rate limiting must survive")
	require.NotNil(t, provider.Configuration.Resilience, "resilience must survive")
	require.Len(t, provider.Configuration.Policies, 1, "the guardrail policy chain must survive")
	require.Equal(t, "openapi: 3.0.0", provider.OpenAPISpec)
	require.Equal(t, `[{"name":"gpt-4o-mini"}]`, provider.ModelList)
}

// Preserving on omission must not make these fields unremovable: a request that does
// name them still wins, including one that turns api-key auth off.
func TestPreserveOmittedProviderFields_SuppliedValuesWin(t *testing.T) {
	existing := storedProvider()
	disabled := false
	req := &spec.UpdateLLMProviderRequest{
		Security: &spec.SecurityConfig{Enabled: &disabled},
	}

	// Conversion already wrote the request's security onto the model.
	provider := &models.LLMProvider{
		Configuration: models.LLMProviderConfig{
			Security: &models.SecurityConfig{
				Enabled: &disabled,
				APIKey:  &models.APIKeySecurity{Enabled: &disabled},
			},
		},
	}

	preserveOmittedProviderFields(provider, existing, req)

	require.False(t, provider.Configuration.Security.RequiresAPIKey(),
		"an explicit security block must still be able to turn api-key auth off")
	// Fields the request omitted are still carried forward.
	require.Len(t, provider.Configuration.Policies, 1)
}

// nil means "omitted", but an explicitly empty list is a deliberate clear — a caller
// must keep the ability to remove every policy.
func TestPreserveOmittedProviderFields_EmptySliceClears(t *testing.T) {
	existing := storedProvider()
	req := &spec.UpdateLLMProviderRequest{Policies: []spec.LLMPolicy{}}

	provider := &models.LLMProvider{
		Configuration: models.LLMProviderConfig{Policies: []models.LLMPolicy{}},
	}

	preserveOmittedProviderFields(provider, existing, req)

	require.Empty(t, provider.Configuration.Policies,
		"an explicitly empty policy list must clear rather than be restored")
}
