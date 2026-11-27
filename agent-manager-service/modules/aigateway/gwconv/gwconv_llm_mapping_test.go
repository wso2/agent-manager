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

package gwconv

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

func mapPtr[T any](v T) *T { return &v }

func mapLimit(n int32) gwspec.RateLimitingLimitConfig {
	return gwspec.RateLimitingLimitConfig{
		Request: &gwspec.RequestRateLimit{Enabled: true, Count: n, Reset: gwspec.RateLimitResetWindow{Duration: 1, Unit: "minute"}},
		Token:   &gwspec.TokenRateLimit{Enabled: true, Count: n * 10, Reset: gwspec.RateLimitResetWindow{Duration: 2, Unit: "hour"}},
		Cost:    &gwspec.CostRateLimit{Enabled: false, Amount: 1.5, Reset: gwspec.RateLimitResetWindow{Duration: 3, Unit: "day"}},
	}
}

func mapRateLimiting() *gwspec.LLMRateLimitingConfig {
	return &gwspec.LLMRateLimitingConfig{
		ProviderLevel: &gwspec.RateLimitingScopeConfig{Global: mapPtr(mapLimit(5))},
		ConsumerLevel: &gwspec.RateLimitingScopeConfig{ResourceWise: &gwspec.ResourceWiseRateLimitingConfig{
			Default:   mapLimit(7),
			Resources: []gwspec.RateLimitingResourceLimit{{Resource: "/chat", Limit: mapLimit(9)}},
		}},
	}
}

func mapPolicies() []gwspec.LLMPolicy {
	return []gwspec.LLMPolicy{{
		Name: "pii-masking", Version: "v1",
		Paths: []gwspec.LLMPolicyPath{{Path: "/chat", Methods: []string{"POST"}, Params: map[string]interface{}{"mask": true}}},
	}}
}

func mapSecurity() *gwspec.SecurityConfig {
	return &gwspec.SecurityConfig{
		Enabled: mapPtr(true),
		ApiKey:  &gwspec.APIKeySecurity{Enabled: mapPtr(true), Key: mapPtr("x-api-key"), In: mapPtr("header")},
	}
}

// Every field of a create-provider request survives into the response the API returns for it,
// except the upstream credential, which comes back masked.
func TestLLMProviderRequestRoundTripsToResponse(t *testing.T) {
	req := &gwspec.CreateLLMProviderRequest{
		Id: "openai", Name: "OpenAI", Description: mapPtr("desc"), Version: "v1", Context: "/openai",
		Template: "openai-tpl", Openapi: mapPtr("openapi: 3.0.0"),
		Upstream: gwspec.UpstreamConfig{
			Main: &gwspec.UpstreamEndpoint{
				Url: mapPtr("https://api.openai.com"), Ref: mapPtr("main-ref"),
				Auth: &gwspec.UpstreamAuth{Type: "api-key", Header: mapPtr("Authorization"), Value: mapPtr("sk-secret")},
			},
			Sandbox: &gwspec.UpstreamEndpoint{Url: mapPtr("https://sandbox.openai.com")},
		},
		Resilience:     &gwspec.Resilience{Timeout: mapPtr("30s"), IdleTimeout: mapPtr("60s")},
		AccessControl:  &gwspec.LLMAccessControl{Mode: "deny_all", Exceptions: []gwspec.RouteException{{Path: "/chat", Methods: []string{"POST"}}}},
		Policies:       mapPolicies(),
		RateLimiting:   mapRateLimiting(),
		Security:       mapSecurity(),
		ModelProviders: []gwspec.LLMModelProvider{{Id: "openai", Name: mapPtr("OpenAI"), Models: []gwspec.LLMModel{{Id: "gpt-4o", Name: mapPtr("GPT-4o"), Description: mapPtr("omni")}}}},
	}

	model := ConvertSpecToModelLLMProvider(req, "org")

	assert.NotEqual(t, uuid.Nil, model.UUID)
	assert.Equal(t, "openai-tpl", model.TemplateHandle)
	assert.Equal(t, "desc", model.Description)
	assert.Equal(t, "openapi: 3.0.0", model.OpenAPISpec)
	cfg := model.Configuration
	assert.Equal(t, "OpenAI", cfg.Name)
	assert.Equal(t, "openai", cfg.Handle)
	assert.Equal(t, "v1", cfg.Version)
	assert.Equal(t, "/openai", *cfg.Context)
	assert.Nil(t, cfg.VHost)
	require.NotNil(t, cfg.Upstream.Main.Auth)
	assert.Equal(t, "sk-secret", *cfg.Upstream.Main.Auth.Value, "a real credential is stored")
	assert.Equal(t, "https://sandbox.openai.com", cfg.Upstream.Sandbox.URL)
	assert.Nil(t, cfg.Upstream.Sandbox.Auth)
	assert.Equal(t, 50, cfg.RateLimiting.ProviderLevel.Global.Token.Count)
	assert.Equal(t, "/chat", cfg.RateLimiting.ConsumerLevel.ResourceWise.Resources[0].Resource)

	model.Artifact = &gwmodels.Artifact{Handle: "openai", Name: "OpenAI", Version: "v1"}
	model.Status = "CREATED"
	model.CreatedBy = "alice"
	model.InCatalog = true
	resp := ConvertModelToSpecLLMProviderResponse(model)

	assert.Equal(t, model.UUID.String(), resp.Uuid)
	assert.Equal(t, "openai", resp.Id)
	assert.Equal(t, "OpenAI", resp.Name)
	assert.Equal(t, "v1", resp.Version)
	assert.Equal(t, "/openai", resp.Context)
	assert.Equal(t, "openai-tpl", resp.Template)
	assert.Equal(t, "CREATED", resp.Status)
	assert.Equal(t, "alice", *resp.CreatedBy)
	assert.True(t, *resp.InCatalog)
	assert.Equal(t, req.Description, resp.Description)
	assert.Equal(t, req.Openapi, resp.Openapi)
	assert.Equal(t, req.Resilience, resp.Resilience)
	assert.Equal(t, req.AccessControl, resp.AccessControl)
	assert.Equal(t, req.Policies, resp.Policies)
	assert.Equal(t, req.RateLimiting, resp.RateLimiting)
	assert.Equal(t, req.Security, resp.Security)
	assert.Equal(t, req.ModelProviders, resp.ModelProviders)
	assert.Equal(t, req.Upstream.Sandbox, resp.Upstream.Sandbox)
	assert.Equal(t, "https://api.openai.com", *resp.Upstream.Main.Url)
	assert.Equal(t, "main-ref", *resp.Upstream.Main.Ref)
	assert.Equal(t, redactedMarker, *resp.Upstream.Main.Auth.Value, "the credential must never be returned")
}

// A minimal provider leaves every optional block unset in both directions.
func TestLLMProviderMinimalRequestLeavesOptionalBlocksUnset(t *testing.T) {
	model := ConvertSpecToModelLLMProvider(&gwspec.CreateLLMProviderRequest{Id: "p", Name: "P", Template: "t"}, "org")

	cfg := model.Configuration
	assert.Empty(t, model.Description)
	assert.Nil(t, model.ModelProviders)
	assert.Nil(t, cfg.Resilience)
	assert.Nil(t, cfg.AccessControl)
	assert.Nil(t, cfg.RateLimiting)
	assert.Nil(t, cfg.Policies)
	assert.Nil(t, cfg.Security)
	assert.Nil(t, cfg.Upstream.Main)
	assert.Nil(t, cfg.Upstream.Sandbox)

	model.Artifact = &gwmodels.Artifact{}
	model.Configuration.Upstream = nil
	resp := ConvertModelToSpecLLMProviderResponse(model)

	assert.Nil(t, resp.Description)
	assert.Nil(t, resp.Openapi)
	assert.Nil(t, resp.Resilience)
	assert.Nil(t, resp.AccessControl)
	assert.Nil(t, resp.RateLimiting)
	assert.Nil(t, resp.Policies)
	assert.Nil(t, resp.Security)
	assert.Nil(t, resp.ModelProviders)
	assert.Equal(t, gwspec.UpstreamConfig{}, resp.Upstream)
	assert.False(t, *resp.InCatalog)
}

// An update that echoes the masked credential keeps the stored one rather than saving the mask.
func TestConvertSpecToModelUpstreamConfigIgnoresRedactedCredential(t *testing.T) {
	for name, tc := range map[string]struct {
		value *string
		want  *string
	}{
		"redacted marker": {mapPtr(redactedMarker), nil},
		"absent":          {nil, nil},
		"new credential":  {mapPtr("sk-new"), mapPtr("sk-new")},
	} {
		t.Run(name, func(t *testing.T) {
			auth := &gwspec.UpstreamAuth{Type: "api-key", Header: mapPtr("Authorization"), Value: tc.value}
			got := ConvertSpecToModelUpstreamConfig(gwspec.UpstreamConfig{
				Main:    &gwspec.UpstreamEndpoint{Url: mapPtr("https://m"), Auth: auth},
				Sandbox: &gwspec.UpstreamEndpoint{Url: mapPtr("https://s"), Ref: mapPtr("r"), Auth: auth},
			})

			for _, ep := range []*gwmodels.UpstreamEndpoint{got.Main, got.Sandbox} {
				require.NotNil(t, ep.Auth)
				assert.Equal(t, "api-key", *ep.Auth.Type)
				assert.Equal(t, "Authorization", *ep.Auth.Header)
				assert.Equal(t, tc.want, ep.Auth.Value)
			}
			assert.Equal(t, "https://m", got.Main.URL)
			assert.Equal(t, "r", got.Sandbox.Ref)
		})
	}
}

// A proxy request maps to the model (UUIDs parsed) and its config comes back unchanged in the response.
func TestLLMProxyRequestRoundTripsToResponse(t *testing.T) {
	projectID, providerID := uuid.New(), uuid.New()
	req := &gwspec.CreateLLMProxyRequest{
		Description: mapPtr("proxy"), ProviderUuid: providerID.String(), Openapi: mapPtr("spec"),
		Configuration: gwspec.LLMProxyConfig{
			Name: mapPtr("px"), Version: mapPtr("v1"), Context: mapPtr("/px"), Vhost: mapPtr("px.local"),
			Provider:   mapPtr("openai"),
			Resilience: &gwspec.Resilience{Timeout: mapPtr("5s")},
			Policies:   mapPolicies(),
			Security:   mapSecurity(),
		},
	}

	proxy, err := ConvertSpecToModelLLMProxy(req, projectID.String())

	require.NoError(t, err)
	assert.Equal(t, projectID, proxy.ProjectUUID)
	assert.Equal(t, providerID, proxy.ProviderUUID)
	assert.Equal(t, "proxy", proxy.Description)
	assert.Equal(t, "spec", proxy.OpenAPISpec)
	assert.Equal(t, "openai", proxy.Configuration.Provider)
	assert.Nil(t, proxy.Configuration.UpstreamAuth)

	proxy.Status = "CREATED"
	resp := ConvertModelToSpecLLMProxyResponse(proxy)

	assert.Equal(t, proxy.UUID.String(), resp.Uuid)
	assert.Equal(t, projectID.String(), resp.ProjectId)
	assert.Equal(t, providerID.String(), resp.ProviderUuid)
	assert.Equal(t, "CREATED", resp.Status)
	assert.Equal(t, req.Description, resp.Description)
	assert.Nil(t, resp.CreatedBy)
	assert.Equal(t, req.Openapi, resp.Openapi)
	assert.Equal(t, req.Configuration, resp.Configuration)
}

// The proxy response never carries the stored upstream credential.
func TestConvertModelToSpecLLMProxyConfigOmitsUpstreamAuth(t *testing.T) {
	secret := "sk-secret"
	got := ConvertModelToSpecLLMProxyConfig(gwmodels.LLMProxyConfig{
		Name:         "px",
		UpstreamAuth: &gwmodels.UpstreamAuth{Value: &secret},
	})

	assert.Equal(t, gwspec.LLMProxyConfig{Name: mapPtr("px")}, got)
}

// A proxy with a malformed project or provider id is rejected with a message naming which one.
func TestConvertSpecToModelLLMProxyRejectsInvalidUUIDs(t *testing.T) {
	for name, tc := range map[string]struct {
		project, provider, wantErr string
	}{
		"bad project":  {"not-a-uuid", uuid.NewString(), "invalid project UUID"},
		"bad provider": {uuid.NewString(), "nope", "invalid provider UUID"},
	} {
		t.Run(name, func(t *testing.T) {
			proxy, err := ConvertSpecToModelLLMProxy(&gwspec.CreateLLMProxyRequest{ProviderUuid: tc.provider}, tc.project)

			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantErr)
			assert.Nil(t, proxy)
		})
	}
}

// Nil nested blocks stay nil rather than becoming empty values.
func TestNestedConvertersKeepNilBlocksNil(t *testing.T) {
	assert.Nil(t, ConvertSpecToModelResilience(nil))
	assert.Equal(t, gwmodels.LLMRateLimitingConfig{}, ConvertSpecToModelLLMRateLimitingConfig(gwspec.LLMRateLimitingConfig{}))
	assert.Equal(t, gwspec.LLMRateLimitingConfig{}, ConvertModelToSpecLLMRateLimitingConfig(gwmodels.LLMRateLimitingConfig{}))
	assert.Equal(t, gwmodels.RateLimitingLimitConfig{}, ConvertSpecToModelRateLimitingLimitConfig(gwspec.RateLimitingLimitConfig{}))
	assert.Nil(t, ConvertSpecToModelLLMAccessControl(gwspec.LLMAccessControl{Mode: "allow_all"}).Exceptions)
	assert.Nil(t, ConvertSpecToModelLLMPolicy(gwspec.LLMPolicy{Name: "p"}).Paths)
	assert.Nil(t, ConvertSpecToModelLLMModelProvider(gwspec.LLMModelProvider{Id: "m"}).Models)
	assert.Nil(t, ConvertModelToSpecLLMModelProvider(gwmodels.LLMModelProvider{ID: "m"}).Name)
	assert.Equal(t, gwmodels.SecurityConfig{}, ConvertSpecToModelSecurityConfig(gwspec.SecurityConfig{}))
	assert.Equal(t, "", ptrToString(nil))
}

// GetOrDefault and GetOrDefaultProxyConfig fall back only when the pointer is nil.
func TestGetOrDefaultHelpers(t *testing.T) {
	assert.Equal(t, "fallback", GetOrDefault(nil, "fallback"))
	assert.Equal(t, "", GetOrDefault(mapPtr(""), "fallback"))
	assert.Equal(t, "set", GetOrDefault(mapPtr("set"), "fallback"))

	assert.Equal(t, gwspec.LLMProxyConfig{}, GetOrDefaultProxyConfig(nil))
	cfg := gwspec.LLMProxyConfig{Name: mapPtr("px")}
	assert.Equal(t, cfg, GetOrDefaultProxyConfig(&cfg))
}

// A deployments list converts each entry in order; an empty list is an empty (not nil) slice.
func TestConvertModelToSpecLLMDeploymentResponses(t *testing.T) {
	assert.Equal(t, []gwspec.LLMDeploymentResponse{}, ConvertModelToSpecLLMDeploymentResponses(nil))

	base := uuid.New()
	rows := make([]*gwmodels.Deployment, 2)
	for i := range rows {
		d := &gwmodels.Deployment{DeploymentID: uuid.New(), Name: "d", OUID: "ou-1", BaseDeploymentID: &base}
		d.SetStatusRow(&gwmodels.DeploymentStatusRecord{
			Status: gwmodels.DeploymentStatusDeployed, StatusDesired: gwmodels.DeploymentStatusDeployed,
		})
		rows[i] = d
	}

	got := ConvertModelToSpecLLMDeploymentResponses(rows)

	require.Len(t, got, 2)
	for i, r := range got {
		assert.Equal(t, rows[i].DeploymentID.String(), r.DeploymentId)
		assert.Equal(t, "ou-1", *r.OrganizationId)
		assert.Equal(t, base.String(), *r.BaseDeploymentId)
	}
}

// Agent deployment details are keyed by environment with endpoints and gateway status mapped through.
func TestConvertToDeploymentDetailsResponse(t *testing.T) {
	assert.Empty(t, ConvertToDeploymentDetailsResponse(nil))

	got := ConvertToDeploymentDetailsResponse([]*gwmodels.DeploymentResponse{
		{
			Environment: "dev", EnvironmentDisplayName: "Development", Status: "ACTIVE",
			Endpoints:     []gwmodels.Endpoint{{Name: "chat", URL: "https://dev/chat", Visibility: "Public"}},
			GatewayStatus: "DEPLOYED", GatewayStatusReason: "ok",
		},
		{Environment: "prod", Status: "NOT_DEPLOYED"},
	})

	require.Len(t, got, 2)
	dev := got["dev"]
	assert.Equal(t, "ACTIVE", dev.Status)
	assert.Equal(t, "Development", *dev.EnvironmentDisplayName)
	assert.Equal(t, []gwspec.DeploymentEndpoint{{Name: "chat", Url: "https://dev/chat", Visibility: "Public"}}, dev.Endpoints)
	assert.Equal(t, "DEPLOYED", *dev.GatewayStatus)
	assert.Equal(t, "ok", *dev.GatewayStatusReason)
	prod := got["prod"]
	assert.Equal(t, "NOT_DEPLOYED", prod.Status)
	assert.Nil(t, prod.EnvironmentDisplayName)
	assert.Nil(t, prod.GatewayStatus)
	assert.Nil(t, prod.GatewayStatusReason)
	assert.Empty(t, prod.Endpoints)
}
