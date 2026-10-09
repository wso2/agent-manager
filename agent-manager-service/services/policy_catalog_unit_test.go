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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/clients/clientmocks"
	"github.com/wso2/agent-manager/agent-manager-service/clients/policyhub"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/repositories/repomocks"
)

// hubCatalogFixture mirrors real policy hub categories (policy-hub-public, 2026-10-04)
// for the policies the applicability rules hinge on.
func hubCatalogFixture() map[string]policyhub.Policy {
	return map[string]policyhub.Policy{
		"mcp-ratelimit":        {Name: "mcp-ratelimit", DisplayName: "MCP Rate Limit", Description: "Hub description.", Categories: []string{"MCP", "Security"}},
		"mcp-auth":             {Name: "mcp-auth", DisplayName: "MCP Authentication", Categories: []string{"MCP", "Security"}},
		"cors":                 {Name: "cors", DisplayName: "CORS", Categories: []string{"Security", "AI", "MCP"}},
		"log-message":          {Name: "log-message", DisplayName: "Log Message", Categories: []string{"Logging", "Analytics & Monitoring", "MCP", "WebSub", "WebBroker"}},
		"word-count-guardrail": {Name: "word-count-guardrail", DisplayName: "Word Count Guardrail", Categories: []string{"Guardrails", "AI"}},
		"api-key-auth":         {Name: "api-key-auth", DisplayName: "API Key Auth", Categories: []string{"Security", "AI", "WebSub", "WebBroker"}},
	}
}

func hubMock(policies map[string]policyhub.Policy, err error) *clientmocks.PolicyHubClientMock {
	return &clientmocks.PolicyHubClientMock{
		ListPoliciesFunc: func(context.Context) (map[string]policyhub.Policy, error) {
			if err != nil {
				return nil, err
			}
			return policies, nil
		},
	}
}

func gatewayRepoWith(gateways ...*models.Gateway) *repomocks.GatewayRepositoryMock {
	return &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return gateways, nil
		},
	}
}

func policyNames[T any](items []T, name func(T) string) []string {
	names := make([]string, 0, len(items))
	for _, item := range items {
		names = append(names, name(item))
	}
	return names
}

func TestPolicyAppliesTo(t *testing.T) {
	tests := []struct {
		name       string
		policy     string
		categories []string
		inHub      bool
		wantLLM    bool
		wantMCP    bool
	}{
		{name: "MCP-exclusive policy is MCP only", policy: "mcp-ratelimit", categories: []string{"MCP", "Security"}, inHub: true, wantLLM: false, wantMCP: true},
		{name: "MCP-only tag is MCP only", policy: "mcp-acl-list", categories: []string{"MCP"}, inHub: true, wantLLM: false, wantMCP: true},
		{name: "MCP plus AI tag is in both", policy: "cors", categories: []string{"Security", "AI", "MCP"}, inHub: true, wantLLM: true, wantMCP: true},
		{name: "MCP plus Logging tag is in both", policy: "log-message", categories: []string{"Logging", "MCP"}, inHub: true, wantLLM: true, wantMCP: true},
		{name: "guardrail is LLM only", policy: "word-count-guardrail", categories: []string{"Guardrails", "AI"}, inHub: true, wantLLM: true, wantMCP: false},
		{name: "Security without MCP stays in LLM", policy: "basic-ratelimit", categories: []string{"Security", "AI"}, inHub: true, wantLLM: true, wantMCP: false},
		{name: "unknown mcp- policy is MCP only", policy: "mcp-custom", inHub: false, wantLLM: false, wantMCP: true},
		{name: "unknown custom policy is in both", policy: "acme-custom", inHub: false, wantLLM: true, wantMCP: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantLLM, policyAppliesTo(policyAPIKindLLM, tc.policy, tc.categories, tc.inHub), "LLM")
			assert.Equal(t, tc.wantMCP, policyAppliesTo(policyAPIKindMCP, tc.policy, tc.categories, tc.inHub), "MCP")
		})
	}
}

func TestBuildPolicyCatalog_GatewayMetadataWinsAndHubFillsGaps(t *testing.T) {
	available := map[string]gatewayPolicyManifestItem{
		"a": {Name: "mcp-ratelimit", Version: "v1.2.0"},
		"b": {Name: "mcp-auth", Version: "v1.4.0", DisplayName: "Gateway Auth", Description: "Gateway description."},
		"c": {Name: "acme-custom", Version: "v1.0.0"},
	}

	entries := buildPolicyCatalog(context.Background(), hubMock(hubCatalogFixture(), nil), available, policyAPIKindMCP, "org-uuid")

	require.Len(t, entries, 3)
	byName := map[string]policyCatalogEntry{}
	for _, e := range entries {
		byName[e.Name] = e
	}
	assert.Equal(t, "MCP Rate Limit", byName["mcp-ratelimit"].DisplayName, "hub fills a blank display name")
	assert.Equal(t, "Hub description.", byName["mcp-ratelimit"].Description)
	assert.Equal(t, "v1.2.0", byName["mcp-ratelimit"].Version, "the gateway version is never replaced by the hub's")
	assert.Equal(t, []string{"MCP", "Security"}, byName["mcp-ratelimit"].Categories)
	assert.Equal(t, "Gateway Auth", byName["mcp-auth"].DisplayName, "the gateway's own display name wins")
	assert.Equal(t, "Gateway description.", byName["mcp-auth"].Description)
	assert.Equal(t, "acme-custom", byName["acme-custom"].DisplayName, "unknown to both falls back to the name")
	assert.Empty(t, byName["acme-custom"].Categories)
}

func TestBuildPolicyCatalog_HubFailureDegradesToNameRules(t *testing.T) {
	available := map[string]gatewayPolicyManifestItem{
		"a": {Name: "mcp-ratelimit", Version: "v1.2.0"},
		"b": {Name: "word-count-guardrail", Version: "v1.0.0"},
	}
	hub := hubMock(nil, errors.New("hub unreachable"))

	entries := buildPolicyCatalog(context.Background(), hub, available, policyAPIKindLLM, "org-uuid")

	require.Len(t, entries, 1, "with no hub data, mcp- policies are still kept out of the LLM listing")
	assert.Equal(t, "word-count-guardrail", entries[0].Name)
	assert.Equal(t, "word-count-guardrail", entries[0].DisplayName)
	assert.Len(t, hub.ListPoliciesCalls(), 1)
}

func TestMCPProxyService_ListAvailableMCPPolicies_FiltersEnrichesAndUsesMajorVersions(t *testing.T) {
	gateways := gatewayRepoWith(gatewayWithPolicyDefinitions(
		map[string]interface{}{"name": "mcp-ratelimit", "version": "v1.2.0", "parameters": map[string]interface{}{"type": "object"}},
		map[string]interface{}{"name": "cors", "version": "v1.0.2"},
		map[string]interface{}{"name": "word-count-guardrail", "version": "v1.0.0"},
		map[string]interface{}{"name": "log-message", "version": "v1.0.2"},
		map[string]interface{}{"name": "log-message", "version": "v1.0.3"},
	))
	svc := NewMCPProxyService(nil, nil, nil, nil, gateways, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		hubMock(hubCatalogFixture(), nil))

	resp, err := svc.ListAvailableMCPPolicies(context.Background(), "org-uuid")

	require.NoError(t, err)
	names := policyNames(resp.List, func(p models.MCPPolicyAvailableItem) string { return p.Name + "@" + p.Version })
	assert.Equal(t, []string{"cors@v1", "log-message@v1", "mcp-ratelimit@v1"}, names,
		"guardrails are dropped, versions reduced to major, and two builds of one major collapse to one entry")
	assert.Equal(t, int32(3), resp.Count)
	for _, p := range resp.List {
		if p.Name == "mcp-ratelimit" {
			assert.Equal(t, "MCP Rate Limit", p.DisplayName)
			assert.Equal(t, "object", p.Parameters["type"], "parameter schema comes inline from the gateway manifest")
		}
	}
}

func TestMCPProxyService_ListAvailableMCPPolicies_PropagatesGatewayListError(t *testing.T) {
	repoErr := errors.New("db down")
	gateways := &repomocks.GatewayRepositoryMock{
		ListWithFiltersFunc: func(repositories.GatewayFilterOptions) ([]*models.Gateway, error) {
			return nil, repoErr
		},
	}
	// ListPoliciesFunc is nil: reaching the hub after a gateway error would panic.
	svc := NewMCPProxyService(nil, nil, nil, nil, gateways, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		&clientmocks.PolicyHubClientMock{})

	_, err := svc.ListAvailableMCPPolicies(context.Background(), "org-uuid")

	require.ErrorIs(t, err, repoErr)
}

func TestMCPProxyService_ListAvailableMCPPolicies_NilGatewayRepoReturnsEmpty(t *testing.T) {
	svc := NewMCPProxyService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		hubMock(hubCatalogFixture(), nil))

	resp, err := svc.ListAvailableMCPPolicies(context.Background(), "org-uuid")

	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.Count)
	assert.NotNil(t, resp.List, "an empty listing must serialize as [], not null")
	assert.Empty(t, resp.List)
}

func TestMCPProxyService_ListAvailableMCPPolicies_HubDownKeepsGatewayPoliciesByName(t *testing.T) {
	gateways := gatewayRepoWith(gatewayWithPolicyDefinitions(
		map[string]interface{}{"name": "mcp-ratelimit", "version": "v1.2.0"},
		map[string]interface{}{"name": "word-count-guardrail", "version": "v1.0.0"},
	))
	svc := NewMCPProxyService(nil, nil, nil, nil, gateways, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		hubMock(nil, errors.New("hub unreachable")))

	resp, err := svc.ListAvailableMCPPolicies(context.Background(), "org-uuid")

	require.NoError(t, err, "the hub is enrichment only; its failure must not fail the listing")
	names := policyNames(resp.List, func(p models.MCPPolicyAvailableItem) string { return p.Name + "@" + p.Version })
	assert.Equal(t, []string{"mcp-ratelimit@v1", "word-count-guardrail@v1"}, names,
		"with no hub categories every gateway policy is kept, with raw names and major versions")
	for _, p := range resp.List {
		assert.Equal(t, p.Name, p.DisplayName)
		assert.Empty(t, p.Categories)
	}
}

func TestLLMProviderService_ListAvailableLLMPolicies_HubDownDropsOnlyMCPNamedPolicies(t *testing.T) {
	gateways := gatewayRepoWith(gatewayWithPolicyDefinitions(
		map[string]interface{}{"name": "mcp-ratelimit", "version": "v1.2.0"},
		map[string]interface{}{"name": "log-message", "version": "v1.0.3"},
	))
	svc := NewLLMProviderService(nil, nil, nil, nil, nil, nil, nil, gateways, nil, nil, nil, nil,
		hubMock(nil, errors.New("hub unreachable")))

	resp, err := svc.ListAvailableLLMPolicies(context.Background(), "org-uuid", "")

	require.NoError(t, err)
	names := policyNames(resp.List, func(p models.LLMPolicyDefinition) string { return p.Name })
	assert.Equal(t, []string{"log-message"}, names)
}

func TestLLMProviderService_ListAvailableLLMPolicies_ExcludesMCPExclusivePolicies(t *testing.T) {
	gateways := gatewayRepoWith(gatewayWithPolicyDefinitions(
		map[string]interface{}{"name": "mcp-ratelimit", "version": "v1.2.0"},
		map[string]interface{}{"name": "mcp-auth", "version": "v1.4.0"},
		map[string]interface{}{"name": "cors", "version": "v1.0.2"},
		map[string]interface{}{"name": "log-message", "version": "v1.0.3"},
		map[string]interface{}{"name": "word-count-guardrail", "version": "v1.0.0"},
		map[string]interface{}{"name": "api-key-auth", "version": "v1.2.0"},
	))
	svc := NewLLMProviderService(nil, nil, nil, nil, nil, nil, nil, gateways, nil, nil, nil, nil,
		hubMock(hubCatalogFixture(), nil))

	resp, err := svc.ListAvailableLLMPolicies(context.Background(), "org-uuid", "")

	require.NoError(t, err)
	names := policyNames(resp.List, func(p models.LLMPolicyDefinition) string { return p.Name + "@" + p.Version })
	assert.Equal(t, []string{"api-key-auth@v1.2.0", "cors@v1.0.2", "log-message@v1.0.3", "word-count-guardrail@v1.0.0"}, names,
		"MCP-exclusive policies are dropped; LLM keeps exact gateway versions; managed-by-tab policies are the console's to hide")
	for _, p := range resp.List {
		if p.Name == "word-count-guardrail" {
			assert.Equal(t, "Word Count Guardrail", p.DisplayName)
			assert.Equal(t, []string{"Guardrails", "AI"}, p.Categories)
		}
	}
}

// configParam builds a system parameter the way gateway manifests do: resolved from
// a "${config.<key>}" reference, optionally with a literal default or a minLength.
func configParam(key string, extra map[string]interface{}) map[string]interface{} {
	p := map[string]interface{}{"type": "string", "wso2/defaultValue": "${config." + key + "}"}
	for k, v := range extra {
		p[k] = v
	}
	return p
}

func TestRequiredSystemConfigKeys(t *testing.T) {
	tests := []struct {
		name   string
		schema map[string]interface{}
		want   []string
	}{
		{
			name: "required config refs without defaults (azure-content-safety shape)",
			schema: map[string]interface{}{
				"type":     "object",
				"required": []interface{}{"azureContentSafetyEndpoint", "azureContentSafetyKey"},
				"properties": map[string]interface{}{
					"azureContentSafetyKey":      configParam("azurecontentsafety_key", map[string]interface{}{"minLength": float64(1)}),
					"azureContentSafetyEndpoint": configParam("azurecontentsafety_endpoint", nil),
				},
			},
			want: []string{"azurecontentsafety_endpoint", "azurecontentsafety_key"},
		},
		{
			name: "minLength makes a non-required ref needed (aws-bedrock shape)",
			schema: map[string]interface{}{
				"properties": map[string]interface{}{
					"awsAccessKeyID": configParam("awsbedrock_access_key_id", map[string]interface{}{"minLength": float64(1)}),
					"allowedRegions": map[string]interface{}{"type": "array", "wso2/defaultValue": "${config.awsbedrock_allowed_regions}"},
				},
			},
			want: []string{"awsbedrock_access_key_id"},
		},
		{
			name: "a literal default means no operator setup (jwt-auth headerName shape)",
			schema: map[string]interface{}{
				"required": []interface{}{"headerName"},
				"properties": map[string]interface{}{
					"headerName": configParam("policy_configurations.jwtauth_v1.headername", map[string]interface{}{"default": "Authorization", "minLength": float64(1)}),
				},
			},
			want: nil,
		},
		{
			name: "required parameter with no config ref is the user's, not the operator's",
			schema: map[string]interface{}{
				"required":   []interface{}{"threshold"},
				"properties": map[string]interface{}{"threshold": map[string]interface{}{"type": "number"}},
			},
			want: nil,
		},
		{
			name: "nested objects are walked with their own required list",
			schema: map[string]interface{}{
				"properties": map[string]interface{}{
					"embedding": map[string]interface{}{
						"type":     "object",
						"required": []interface{}{"apiKey"},
						"properties": map[string]interface{}{
							"apiKey": configParam("embedding_provider_api_key", nil),
						},
					},
				},
			},
			want: []string{"embedding_provider_api_key"},
		},
		{
			name:   "no system parameters",
			schema: nil,
			want:   nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, requiredSystemConfigKeys(tc.schema))
		})
	}
}

func TestListAvailablePolicies_ReportRequiredSystemConfig(t *testing.T) {
	azure := map[string]interface{}{
		"name":    "azure-content-safety-content-moderation",
		"version": "v1.0.0",
		"systemParameters": map[string]interface{}{
			"required": []interface{}{"azureContentSafetyKey"},
			"properties": map[string]interface{}{
				"azureContentSafetyKey": configParam("azurecontentsafety_key", nil),
			},
		},
	}
	plain := map[string]interface{}{"name": "regex-guardrail", "version": "v1.0.0"}
	hub := hubMock(map[string]policyhub.Policy{
		"azure-content-safety-content-moderation": {Name: "azure-content-safety-content-moderation", Categories: []string{"Guardrails", "AI"}},
		"regex-guardrail":                         {Name: "regex-guardrail", Categories: []string{"Guardrails", "AI"}},
	}, nil)
	svc := NewLLMProviderService(nil, nil, nil, nil, nil, nil, nil,
		gatewayRepoWith(gatewayWithPolicyDefinitions(azure, plain)), nil, nil, nil, nil, hub)

	resp, err := svc.ListAvailableLLMPolicies(context.Background(), "org-uuid", "")

	require.NoError(t, err)
	require.Len(t, resp.List, 2, "a policy needing gateway configuration is listed, not hidden")
	byName := map[string]models.LLMPolicyDefinition{}
	for _, p := range resp.List {
		byName[p.Name] = p
	}
	assert.Equal(t, []string{"azurecontentsafety_key"}, byName["azure-content-safety-content-moderation"].RequiredSystemConfig)
	assert.Empty(t, byName["regex-guardrail"].RequiredSystemConfig)
}
