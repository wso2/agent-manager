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

// Package gwconv converts between the AI Gateway's API types (gwspec) and
// its models.
package gwconv

import (
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

// Helper function to convert *string to string
func ptrToString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// Helper function to convert string to *string
func stringToPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// GetOrDefault returns the value of a pointer or a default value
func GetOrDefault(ptr *string, defaultVal string) string {
	if ptr == nil {
		return defaultVal
	}
	return *ptr
}

// GetOrDefaultProxyConfig returns the value of a proxy config pointer or empty config
func GetOrDefaultProxyConfig(cfg *gwspec.LLMProxyConfig) gwspec.LLMProxyConfig {
	if cfg == nil {
		return gwspec.LLMProxyConfig{}
	}
	return *cfg
}

// ---- LLM Provider Template Conversions ----

// ConvertSpecToModelLLMProviderTemplate converts gwspec.CreateLLMProviderTemplateRequest to gwmodels.LLMProviderTemplate
// Note: The service layer handles Configuration field (JSON marshaling)
func ConvertSpecToModelLLMProviderTemplate(req *gwspec.CreateLLMProviderTemplateRequest, ouID string) *gwmodels.LLMProviderTemplate {
	template := &gwmodels.LLMProviderTemplate{
		UUID:        uuid.New(),
		OUID:        ouID,
		Handle:      req.Id, // ID is the handle
		Name:        req.Name,
		Description: ptrToString(req.Description),
	}

	// Map nested configuration fields
	if req.Metadata != nil {
		template.Metadata = ConvertSpecToModelLLMProviderTemplateMetadata(req.Metadata)
	}
	if req.PromptTokens != nil {
		template.PromptTokens = ConvertSpecToModelExtractionIdentifier(req.PromptTokens)
	}
	if req.CompletionTokens != nil {
		template.CompletionTokens = ConvertSpecToModelExtractionIdentifier(req.CompletionTokens)
	}
	if req.TotalTokens != nil {
		template.TotalTokens = ConvertSpecToModelExtractionIdentifier(req.TotalTokens)
	}
	if req.RemainingTokens != nil {
		template.RemainingTokens = ConvertSpecToModelExtractionIdentifier(req.RemainingTokens)
	}
	if req.RequestModel != nil {
		template.RequestModel = ConvertSpecToModelExtractionIdentifier(req.RequestModel)
	}
	if req.ResponseModel != nil {
		template.ResponseModel = ConvertSpecToModelExtractionIdentifier(req.ResponseModel)
	}

	return template
}

// ConvertModelToSpecLLMProviderTemplateResponse converts gwmodels.LLMProviderTemplate to gwspec.LLMProviderTemplateResponse
func ConvertModelToSpecLLMProviderTemplateResponse(model *gwmodels.LLMProviderTemplate) gwspec.LLMProviderTemplateResponse {
	resp := &gwspec.LLMProviderTemplateResponse{
		Uuid:        model.UUID.String(),
		Id:          model.Handle,
		Name:        model.Name,
		Description: stringToPtr(model.Description),
		CreatedBy:   stringToPtr(model.CreatedBy),
		CreatedAt:   &model.CreatedAt,
		UpdatedAt:   &model.UpdatedAt,
	}

	// Map nested configuration fields
	if model.Metadata != nil {
		resp.Metadata = ConvertModelToSpecLLMProviderTemplateMetadata(model.Metadata)
	}
	if model.PromptTokens != nil {
		resp.PromptTokens = ConvertModelToSpecExtractionIdentifier(model.PromptTokens)
	}
	if model.CompletionTokens != nil {
		resp.CompletionTokens = ConvertModelToSpecExtractionIdentifier(model.CompletionTokens)
	}
	if model.TotalTokens != nil {
		resp.TotalTokens = ConvertModelToSpecExtractionIdentifier(model.TotalTokens)
	}
	if model.RemainingTokens != nil {
		resp.RemainingTokens = ConvertModelToSpecExtractionIdentifier(model.RemainingTokens)
	}
	if model.RequestModel != nil {
		resp.RequestModel = ConvertModelToSpecExtractionIdentifier(model.RequestModel)
	}
	if model.ResponseModel != nil {
		resp.ResponseModel = ConvertModelToSpecExtractionIdentifier(model.ResponseModel)
	}

	return *resp
}

// ConvertSpecToModelExtractionIdentifier converts spec to model ExtractionIdentifier
func ConvertSpecToModelExtractionIdentifier(ei *gwspec.ExtractionIdentifier) *gwmodels.ExtractionIdentifier {
	if ei == nil {
		return nil
	}
	return &gwmodels.ExtractionIdentifier{
		Location:   ei.Location,
		Identifier: ei.Identifier,
	}
}

// ConvertModelToSpecExtractionIdentifier converts model to spec ExtractionIdentifier
func ConvertModelToSpecExtractionIdentifier(ei *gwmodels.ExtractionIdentifier) *gwspec.ExtractionIdentifier {
	if ei == nil {
		return nil
	}
	return &gwspec.ExtractionIdentifier{
		Location:   ei.Location,
		Identifier: ei.Identifier,
	}
}

// ConvertSpecToModelLLMProviderTemplateMetadata converts spec to model LLMProviderTemplateMetadata
func ConvertSpecToModelLLMProviderTemplateMetadata(meta *gwspec.LLMProviderTemplateMetadata) *gwmodels.LLMProviderTemplateMetadata {
	if meta == nil {
		return nil
	}
	metadata := &gwmodels.LLMProviderTemplateMetadata{
		EndpointURL:    ptrToString(meta.EndpointUrl),
		LogoURL:        ptrToString(meta.LogoUrl),
		OpenapiSpecURL: ptrToString(meta.OpenapiSpecUrl),
	}
	if meta.Auth != nil {
		metadata.Auth = &gwmodels.LLMProviderTemplateAuth{
			Type:        ptrToString(meta.Auth.Type),
			Header:      ptrToString(meta.Auth.Header),
			ValuePrefix: ptrToString(meta.Auth.ValuePrefix),
		}
	}
	return metadata
}

// ConvertModelToSpecLLMProviderTemplateMetadata converts model to spec LLMProviderTemplateMetadata
func ConvertModelToSpecLLMProviderTemplateMetadata(meta *gwmodels.LLMProviderTemplateMetadata) *gwspec.LLMProviderTemplateMetadata {
	if meta == nil {
		return nil
	}
	metadata := &gwspec.LLMProviderTemplateMetadata{
		EndpointUrl:    stringToPtr(meta.EndpointURL),
		LogoUrl:        stringToPtr(meta.LogoURL),
		OpenapiSpecUrl: stringToPtr(meta.OpenapiSpecURL),
	}
	if meta.Auth != nil {
		metadata.Auth = &gwspec.LLMProviderTemplateAuth{
			Type:        stringToPtr(meta.Auth.Type),
			Header:      stringToPtr(meta.Auth.Header),
			ValuePrefix: stringToPtr(meta.Auth.ValuePrefix),
		}
	}
	return metadata
}

// ---- LLM Provider Conversions ----

// ConvertSpecToModelLLMProvider converts gwspec.CreateLLMProviderRequest to gwmodels.LLMProvider
func ConvertSpecToModelLLMProvider(req *gwspec.CreateLLMProviderRequest, orgName string) *gwmodels.LLMProvider {
	provider := &gwmodels.LLMProvider{
		UUID:           uuid.New(),
		TemplateHandle: req.Template,
		Description:    ptrToString(req.Description),
		OpenAPISpec:    ptrToString(req.Openapi),
		Configuration:  ConvertSpecToModelLLMProviderConfigFromRequest(req),
	}

	// Convert model providers if present
	if len(req.ModelProviders) > 0 {
		provider.ModelProviders = make([]gwmodels.LLMModelProvider, len(req.ModelProviders))
		for i, mp := range req.ModelProviders {
			provider.ModelProviders[i] = ConvertSpecToModelLLMModelProvider(mp)
		}
	}

	return provider
}

// ConvertModelToSpecLLMProviderResponse converts gwmodels.LLMProvider to gwspec.LLMProviderResponse
func ConvertModelToSpecLLMProviderListItemResponse(model *gwmodels.LLMProvider) gwspec.LLMProviderListItem {
	resp := &gwspec.LLMProviderListItem{
		Uuid:      model.UUID.String(),
		Id:        model.Artifact.Handle,
		Name:      model.Artifact.Name,
		CreatedBy: stringToPtr(model.CreatedBy),
		Template:  model.TemplateHandle,
		Status:    model.Status,
		CreatedAt: &model.Artifact.CreatedAt,
		UpdatedAt: &model.Artifact.UpdatedAt,
	}

	return *resp
}

// ConvertModelToSpecLLMProviderResponse converts gwmodels.LLMProvider to gwspec.LLMProviderResponse
func ConvertModelToSpecLLMProviderResponse(model *gwmodels.LLMProvider) gwspec.LLMProviderResponse {
	var upstream gwspec.UpstreamConfig
	if model.Configuration.Upstream != nil {
		upstream = ConvertModelToSpecUpstreamConfig(*model.Configuration.Upstream)
	}

	resp := &gwspec.LLMProviderResponse{
		Uuid:        model.UUID.String(),
		Id:          model.Artifact.Handle,
		Name:        model.Artifact.Name,
		Description: stringToPtr(model.Description),
		CreatedBy:   stringToPtr(model.CreatedBy),
		Version:     model.Artifact.Version,
		Context:     ptrToString(model.Configuration.Context),
		Template:    model.TemplateHandle,
		Upstream:    upstream,
		Resilience:  ConvertModelToSpecResilience(model.Configuration.Resilience),
		Openapi:     stringToPtr(model.OpenAPISpec),
		Status:      model.Status,
	}

	// Convert optional fields from configuration
	if model.Configuration.AccessControl != nil {
		ac := ConvertModelToSpecLLMAccessControl(*model.Configuration.AccessControl)
		resp.AccessControl = &ac
	}
	if len(model.Configuration.Policies) > 0 {
		resp.Policies = make([]gwspec.LLMPolicy, len(model.Configuration.Policies))
		for i, p := range model.Configuration.Policies {
			resp.Policies[i] = ConvertModelToSpecLLMPolicy(p)
		}
	}
	if model.Configuration.RateLimiting != nil {
		rl := ConvertModelToSpecLLMRateLimitingConfig(*model.Configuration.RateLimiting)
		resp.RateLimiting = &rl
	}
	if model.Configuration.Security != nil {
		sec := ConvertModelToSpecSecurityConfig(*model.Configuration.Security)
		resp.Security = &sec
	}

	// Set catalog status
	resp.InCatalog = &model.InCatalog

	// Convert model providers
	if len(model.ModelProviders) > 0 {
		resp.ModelProviders = make([]gwspec.LLMModelProvider, len(model.ModelProviders))
		for i, mp := range model.ModelProviders {
			resp.ModelProviders[i] = ConvertModelToSpecLLMModelProvider(mp)
		}
	}
	return *resp
}

// ConvertSpecToModelLLMProviderConfigFromRequest converts spec request to model configuration
// This is used for the new flat structure where all fields are at the top level
func ConvertSpecToModelLLMProviderConfigFromRequest(req *gwspec.CreateLLMProviderRequest) gwmodels.LLMProviderConfig {
	upstream := ConvertSpecToModelUpstreamConfig(req.Upstream)

	modelConfig := gwmodels.LLMProviderConfig{
		Name:       req.Name,
		Handle:     req.Id,
		Version:    req.Version,
		Context:    &req.Context,
		VHost:      nil,
		Template:   req.Template,
		Upstream:   &upstream,
		Resilience: ConvertSpecToModelResilience(req.Resilience),
	}

	if req.AccessControl != nil {
		ac := ConvertSpecToModelLLMAccessControl(*req.AccessControl)
		modelConfig.AccessControl = &ac
	}
	if req.RateLimiting != nil {
		rl := ConvertSpecToModelLLMRateLimitingConfig(*req.RateLimiting)
		modelConfig.RateLimiting = &rl
	}
	if len(req.Policies) > 0 {
		modelConfig.Policies = make([]gwmodels.LLMPolicy, len(req.Policies))
		for i, p := range req.Policies {
			modelConfig.Policies[i] = ConvertSpecToModelLLMPolicy(p)
		}
	}
	if req.Security != nil {
		sec := ConvertSpecToModelSecurityConfig(*req.Security)
		modelConfig.Security = &sec
	}

	return modelConfig
}

// ---- LLM Proxy Conversions ----

// ConvertSpecToModelLLMProxy converts gwspec.CreateLLMProxyRequest to gwmodels.LLMProxy
func ConvertSpecToModelLLMProxy(req *gwspec.CreateLLMProxyRequest, projectID string) (*gwmodels.LLMProxy, error) {
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, fmt.Errorf("invalid project UUID: %w", err)
	}

	providerUUID, err := uuid.Parse(req.ProviderUuid)
	if err != nil {
		return nil, fmt.Errorf("invalid provider UUID: %w", err)
	}

	proxy := &gwmodels.LLMProxy{
		UUID:          uuid.New(),
		ProjectUUID:   projectUUID,
		ProviderUUID:  providerUUID,
		Description:   ptrToString(req.Description),
		OpenAPISpec:   ptrToString(req.Openapi),
		Configuration: ConvertSpecToModelLLMProxyConfig(req.Configuration),
	}

	return proxy, nil
}

// ConvertModelToSpecLLMProxyResponse converts gwmodels.LLMProxy to gwspec.LLMProxyResponse
func ConvertModelToSpecLLMProxyResponse(model *gwmodels.LLMProxy) gwspec.LLMProxyResponse {
	resp := &gwspec.LLMProxyResponse{
		Uuid:          model.UUID.String(),
		ProjectId:     model.ProjectUUID.String(),
		ProviderUuid:  model.ProviderUUID.String(),
		Status:        model.Status,
		Description:   stringToPtr(model.Description),
		CreatedBy:     stringToPtr(model.CreatedBy),
		Openapi:       stringToPtr(model.OpenAPISpec),
		Configuration: ConvertModelToSpecLLMProxyConfig(model.Configuration),
	}

	return *resp
}

// ConvertSpecToModelLLMProxyConfig converts spec proxy config to model proxy config
func ConvertSpecToModelLLMProxyConfig(config gwspec.LLMProxyConfig) gwmodels.LLMProxyConfig {
	modelConfig := gwmodels.LLMProxyConfig{
		Name:       ptrToString(config.Name),
		Version:    ptrToString(config.Version),
		Context:    config.Context,
		Vhost:      config.Vhost,
		Provider:   ptrToString(config.Provider),
		Resilience: ConvertSpecToModelResilience(config.Resilience),
	}

	// Note: UpstreamAuth is not part of the OpenAPI spec and is handled separately
	// in the preserveUpstreamAuthCredential function during updates

	if len(config.Policies) > 0 {
		modelConfig.Policies = make([]gwmodels.LLMPolicy, len(config.Policies))
		for i, p := range config.Policies {
			modelConfig.Policies[i] = ConvertSpecToModelLLMPolicy(p)
		}
	}
	if config.Security != nil {
		sec := ConvertSpecToModelSecurityConfig(*config.Security)
		modelConfig.Security = &sec
	}

	return modelConfig
}

// ConvertModelToSpecLLMProxyConfig converts model proxy config to spec proxy config
func ConvertModelToSpecLLMProxyConfig(config gwmodels.LLMProxyConfig) gwspec.LLMProxyConfig {
	specConfig := gwspec.LLMProxyConfig{
		Name:       stringToPtr(config.Name),
		Version:    stringToPtr(config.Version),
		Context:    config.Context,
		Vhost:      config.Vhost,
		Provider:   stringToPtr(config.Provider),
		Resilience: ConvertModelToSpecResilience(config.Resilience),
	}

	// Note: UpstreamAuth is intentionally not included in the spec response for security.
	// Credentials should not be exposed via API responses.

	if len(config.Policies) > 0 {
		specConfig.Policies = make([]gwspec.LLMPolicy, len(config.Policies))
		for i, p := range config.Policies {
			specConfig.Policies[i] = ConvertModelToSpecLLMPolicy(p)
		}
	}
	if config.Security != nil {
		sec := ConvertModelToSpecSecurityConfig(*config.Security)
		specConfig.Security = &sec
	}

	return specConfig
}

// ---- Nested Type Conversions ----

// ConvertSpecToModelLLMModelProvider converts spec to model LLMModelProvider
func ConvertSpecToModelLLMModelProvider(specProvider gwspec.LLMModelProvider) gwmodels.LLMModelProvider {
	provider := gwmodels.LLMModelProvider{
		ID:   specProvider.Id,
		Name: ptrToString(specProvider.Name),
	}

	if len(specProvider.Models) > 0 {
		provider.Models = make([]gwmodels.LLMModel, len(specProvider.Models))
		for i, m := range specProvider.Models {
			provider.Models[i] = gwmodels.LLMModel{
				ID:          m.Id,
				Name:        ptrToString(m.Name),
				Description: ptrToString(m.Description),
			}
		}
	}

	return provider
}

// ConvertModelToSpecLLMModelProvider converts model to spec LLMModelProvider
func ConvertModelToSpecLLMModelProvider(model gwmodels.LLMModelProvider) gwspec.LLMModelProvider {
	provider := gwspec.LLMModelProvider{
		Id:   model.ID,
		Name: stringToPtr(model.Name),
	}

	if len(model.Models) > 0 {
		provider.Models = make([]gwspec.LLMModel, len(model.Models))
		for i, m := range model.Models {
			provider.Models[i] = gwspec.LLMModel{
				Id:          m.ID,
				Name:        stringToPtr(m.Name),
				Description: stringToPtr(m.Description),
			}
		}
	}

	return provider
}

// ConvertSpecToModelResilience converts spec Resilience to model Resilience.
func ConvertSpecToModelResilience(r *gwspec.Resilience) *gwmodels.Resilience {
	if r == nil {
		return nil
	}
	return &gwmodels.Resilience{
		Timeout:     r.Timeout,
		IdleTimeout: r.IdleTimeout,
	}
}

// ConvertModelToSpecResilience converts model Resilience to spec Resilience.
func ConvertModelToSpecResilience(r *gwmodels.Resilience) *gwspec.Resilience {
	if r == nil {
		return nil
	}
	return &gwspec.Resilience{
		Timeout:     r.Timeout,
		IdleTimeout: r.IdleTimeout,
	}
}

// ConvertSpecToModelUpstreamConfig converts spec to model UpstreamConfig
// redactedMarker is the placeholder returned in API responses to mask credential values.
// When received back in an update request it must be treated as "no change" rather than
// stored as the literal secret value.
const redactedMarker = "***REDACTED***"

// nonRedactedPtr returns the pointer only when the value is not the redaction marker.
// Returns nil (no-op) when the value is the marker or the pointer itself is nil.
func nonRedactedPtr(v *string) *string {
	if v == nil || *v == redactedMarker {
		return nil
	}
	return v
}

func ConvertSpecToModelUpstreamConfig(config gwspec.UpstreamConfig) gwmodels.UpstreamConfig {
	modelConfig := gwmodels.UpstreamConfig{}

	if config.Main != nil {
		main := gwmodels.UpstreamEndpoint{
			URL: config.Main.GetUrl(),
			Ref: ptrToString(config.Main.Ref),
		}
		if config.Main.Auth != nil {
			main.Auth = &gwmodels.UpstreamAuth{
				Type:   &config.Main.Auth.Type,
				Value:  nonRedactedPtr(config.Main.Auth.Value),
				Header: config.Main.Auth.Header,
			}
		}
		modelConfig.Main = &main
	}

	if config.Sandbox != nil {
		sandbox := gwmodels.UpstreamEndpoint{
			URL: config.Sandbox.GetUrl(),
			Ref: ptrToString(config.Sandbox.Ref),
		}
		if config.Sandbox.Auth != nil {
			sandbox.Auth = &gwmodels.UpstreamAuth{
				Type:   &config.Sandbox.Auth.Type,
				Value:  nonRedactedPtr(config.Sandbox.Auth.Value),
				Header: config.Sandbox.Auth.Header,
			}
		}
		modelConfig.Sandbox = &sandbox
	}

	return modelConfig
}

// ConvertModelToSpecUpstreamConfig converts model to spec UpstreamConfig
func ConvertModelToSpecUpstreamConfig(config gwmodels.UpstreamConfig) gwspec.UpstreamConfig {
	specConfig := gwspec.UpstreamConfig{}

	if config.Main != nil {
		main := gwspec.UpstreamEndpoint{
			Url: &config.Main.URL,
			Ref: stringToPtr(config.Main.Ref),
		}
		if config.Main.Auth != nil {
			main.Auth = maskedSpecUpstreamAuth(config.Main.Auth)
		}
		specConfig.Main = &main
	}

	if config.Sandbox != nil {
		sandbox := gwspec.UpstreamEndpoint{
			Url: &config.Sandbox.URL,
			Ref: stringToPtr(config.Sandbox.Ref),
		}
		if config.Sandbox.Auth != nil {
			sandbox.Auth = maskedSpecUpstreamAuth(config.Sandbox.Auth)
		}
		specConfig.Sandbox = &sandbox
	}

	return specConfig
}

// maskedSpecUpstreamAuth converts an upstream auth block for a response, replacing
// the credential with a fixed mask. Both endpoints share it: the sandbox branch used
// to read its header from config.Main.Auth, which panics for a provider that has a
// sandbox credential and no main one.
func maskedSpecUpstreamAuth(auth *gwmodels.UpstreamAuth) *gwspec.UpstreamAuth {
	masked := redactedMarker
	return &gwspec.UpstreamAuth{
		Type:   gwutil.StrPointerAsStr(auth.Type, ""),
		Header: auth.Header,
		Value:  &masked,
	}
}

// ConvertSpecToModelLLMAccessControl converts spec to model LLMAccessControl
func ConvertSpecToModelLLMAccessControl(ac gwspec.LLMAccessControl) gwmodels.LLMAccessControl {
	modelAC := gwmodels.LLMAccessControl{
		Mode: ac.Mode,
	}

	if len(ac.Exceptions) > 0 {
		modelAC.Exceptions = make([]gwmodels.RouteException, len(ac.Exceptions))
		for i, e := range ac.Exceptions {
			modelAC.Exceptions[i] = gwmodels.RouteException{
				Path:    e.Path,
				Methods: e.Methods,
			}
		}
	}

	return modelAC
}

// ConvertModelToSpecLLMAccessControl converts model to spec LLMAccessControl
func ConvertModelToSpecLLMAccessControl(ac gwmodels.LLMAccessControl) gwspec.LLMAccessControl {
	specAC := gwspec.LLMAccessControl{
		Mode: ac.Mode,
	}

	if len(ac.Exceptions) > 0 {
		specAC.Exceptions = make([]gwspec.RouteException, len(ac.Exceptions))
		for i, e := range ac.Exceptions {
			specAC.Exceptions[i] = gwspec.RouteException{
				Path:    e.Path,
				Methods: e.Methods,
			}
		}
	}

	return specAC
}

// ConvertSpecToModelLLMRateLimitingConfig converts spec to model LLMRateLimitingConfig
func ConvertSpecToModelLLMRateLimitingConfig(rl gwspec.LLMRateLimitingConfig) gwmodels.LLMRateLimitingConfig {
	modelRL := gwmodels.LLMRateLimitingConfig{}

	if rl.ProviderLevel != nil {
		providerLevel := ConvertSpecToModelRateLimitingScopeConfig(*rl.ProviderLevel)
		modelRL.ProviderLevel = &providerLevel
	}
	if rl.ConsumerLevel != nil {
		consumerLevel := ConvertSpecToModelRateLimitingScopeConfig(*rl.ConsumerLevel)
		modelRL.ConsumerLevel = &consumerLevel
	}

	return modelRL
}

// ConvertModelToSpecLLMRateLimitingConfig converts model to spec LLMRateLimitingConfig
func ConvertModelToSpecLLMRateLimitingConfig(rl gwmodels.LLMRateLimitingConfig) gwspec.LLMRateLimitingConfig {
	specRL := gwspec.LLMRateLimitingConfig{}

	if rl.ProviderLevel != nil {
		providerLevel := ConvertModelToSpecRateLimitingScopeConfig(*rl.ProviderLevel)
		specRL.ProviderLevel = &providerLevel
	}
	if rl.ConsumerLevel != nil {
		consumerLevel := ConvertModelToSpecRateLimitingScopeConfig(*rl.ConsumerLevel)
		specRL.ConsumerLevel = &consumerLevel
	}

	return specRL
}

// ConvertSpecToModelRateLimitingScopeConfig converts spec to model RateLimitingScopeConfig
func ConvertSpecToModelRateLimitingScopeConfig(scope gwspec.RateLimitingScopeConfig) gwmodels.RateLimitingScopeConfig {
	modelScope := gwmodels.RateLimitingScopeConfig{}

	if scope.Global != nil {
		global := ConvertSpecToModelRateLimitingLimitConfig(*scope.Global)
		modelScope.Global = &global
	}
	if scope.ResourceWise != nil {
		resourceWise := gwmodels.ResourceWiseRateLimitingConfig{
			Default: ConvertSpecToModelRateLimitingLimitConfig(scope.ResourceWise.Default),
		}
		if len(scope.ResourceWise.Resources) > 0 {
			resourceWise.Resources = make([]gwmodels.RateLimitingResourceLimit, len(scope.ResourceWise.Resources))
			for i, r := range scope.ResourceWise.Resources {
				resourceWise.Resources[i] = gwmodels.RateLimitingResourceLimit{
					Resource: r.Resource,
					Limit:    ConvertSpecToModelRateLimitingLimitConfig(r.Limit),
				}
			}
		}
		modelScope.ResourceWise = &resourceWise
	}

	return modelScope
}

// ConvertModelToSpecRateLimitingScopeConfig converts model to spec RateLimitingScopeConfig
func ConvertModelToSpecRateLimitingScopeConfig(scope gwmodels.RateLimitingScopeConfig) gwspec.RateLimitingScopeConfig {
	specScope := gwspec.RateLimitingScopeConfig{}

	if scope.Global != nil {
		global := ConvertModelToSpecRateLimitingLimitConfig(*scope.Global)
		specScope.Global = &global
	}
	if scope.ResourceWise != nil {
		resourceWise := gwspec.ResourceWiseRateLimitingConfig{
			Default: ConvertModelToSpecRateLimitingLimitConfig(scope.ResourceWise.Default),
		}
		if len(scope.ResourceWise.Resources) > 0 {
			resourceWise.Resources = make([]gwspec.RateLimitingResourceLimit, len(scope.ResourceWise.Resources))
			for i, r := range scope.ResourceWise.Resources {
				resourceWise.Resources[i] = gwspec.RateLimitingResourceLimit{
					Resource: r.Resource,
					Limit:    ConvertModelToSpecRateLimitingLimitConfig(r.Limit),
				}
			}
		}
		specScope.ResourceWise = &resourceWise
	}

	return specScope
}

// ConvertSpecToModelRateLimitingLimitConfig converts spec to model RateLimitingLimitConfig
func ConvertSpecToModelRateLimitingLimitConfig(limit gwspec.RateLimitingLimitConfig) gwmodels.RateLimitingLimitConfig {
	modelLimit := gwmodels.RateLimitingLimitConfig{}

	if limit.Request != nil {
		modelLimit.Request = &gwmodels.RequestRateLimit{
			Enabled: limit.Request.Enabled,
			Count:   int(limit.Request.Count),
			Reset: gwmodels.RateLimitResetWindow{
				Duration: int(limit.Request.Reset.Duration),
				Unit:     limit.Request.Reset.Unit,
			},
		}
	}
	if limit.Token != nil {
		modelLimit.Token = &gwmodels.TokenRateLimit{
			Enabled: limit.Token.Enabled,
			Count:   int(limit.Token.Count),
			Reset: gwmodels.RateLimitResetWindow{
				Duration: int(limit.Token.Reset.Duration),
				Unit:     limit.Token.Reset.Unit,
			},
		}
	}
	if limit.Cost != nil {
		modelLimit.Cost = &gwmodels.CostRateLimit{
			Enabled: limit.Cost.Enabled,
			Amount:  limit.Cost.Amount,
			Reset: gwmodels.RateLimitResetWindow{
				Duration: int(limit.Cost.Reset.Duration),
				Unit:     limit.Cost.Reset.Unit,
			},
		}
	}

	return modelLimit
}

// ConvertModelToSpecRateLimitingLimitConfig converts model to spec RateLimitingLimitConfig
func ConvertModelToSpecRateLimitingLimitConfig(limit gwmodels.RateLimitingLimitConfig) gwspec.RateLimitingLimitConfig {
	specLimit := gwspec.RateLimitingLimitConfig{}

	if limit.Request != nil {
		specLimit.Request = &gwspec.RequestRateLimit{
			Enabled: limit.Request.Enabled,
			Count:   int32(limit.Request.Count),
			Reset: gwspec.RateLimitResetWindow{
				Duration: int32(limit.Request.Reset.Duration),
				Unit:     limit.Request.Reset.Unit,
			},
		}
	}
	if limit.Token != nil {
		specLimit.Token = &gwspec.TokenRateLimit{
			Enabled: limit.Token.Enabled,
			Count:   int32(limit.Token.Count),
			Reset: gwspec.RateLimitResetWindow{
				Duration: int32(limit.Token.Reset.Duration),
				Unit:     limit.Token.Reset.Unit,
			},
		}
	}
	if limit.Cost != nil {
		specLimit.Cost = &gwspec.CostRateLimit{
			Enabled: limit.Cost.Enabled,
			Amount:  limit.Cost.Amount,
			Reset: gwspec.RateLimitResetWindow{
				Duration: int32(limit.Cost.Reset.Duration),
				Unit:     limit.Cost.Reset.Unit,
			},
		}
	}

	return specLimit
}

// ConvertSpecToModelLLMPolicy converts spec to model LLMPolicy
func ConvertSpecToModelLLMPolicy(policy gwspec.LLMPolicy) gwmodels.LLMPolicy {
	modelPolicy := gwmodels.LLMPolicy{
		Name:    policy.Name,
		Version: policy.Version,
	}

	if len(policy.Paths) > 0 {
		modelPolicy.Paths = make([]gwmodels.LLMPolicyPath, len(policy.Paths))
		for i, p := range policy.Paths {
			modelPolicy.Paths[i] = gwmodels.LLMPolicyPath{
				Path:    p.Path,
				Methods: p.Methods,
				Params:  p.Params,
			}
		}
	}

	return modelPolicy
}

// ConvertModelToSpecLLMPolicy converts model to spec LLMPolicy
func ConvertModelToSpecLLMPolicy(policy gwmodels.LLMPolicy) gwspec.LLMPolicy {
	specPolicy := gwspec.LLMPolicy{
		Name:    policy.Name,
		Version: policy.Version,
	}

	if len(policy.Paths) > 0 {
		specPolicy.Paths = make([]gwspec.LLMPolicyPath, len(policy.Paths))
		for i, p := range policy.Paths {
			specPolicy.Paths[i] = gwspec.LLMPolicyPath{
				Path:    p.Path,
				Methods: p.Methods,
				Params:  p.Params,
			}
		}
	}

	return specPolicy
}

// ConvertSpecToModelSecurityConfig converts spec to model SecurityConfig
func ConvertSpecToModelSecurityConfig(sec gwspec.SecurityConfig) gwmodels.SecurityConfig {
	modelSec := gwmodels.SecurityConfig{
		Enabled: sec.Enabled,
	}

	if sec.ApiKey != nil {
		modelSec.APIKey = &gwmodels.APIKeySecurity{
			Enabled: sec.ApiKey.Enabled,
			In:      ptrToString(sec.ApiKey.In),
			Key:     ptrToString(sec.ApiKey.Key),
		}
	}

	return modelSec
}

// ConvertModelToSpecSecurityConfig converts model to spec SecurityConfig
func ConvertModelToSpecSecurityConfig(sec gwmodels.SecurityConfig) gwspec.SecurityConfig {
	specSec := gwspec.SecurityConfig{
		Enabled: sec.Enabled,
	}

	if sec.APIKey != nil {
		specSec.ApiKey = &gwspec.APIKeySecurity{
			Enabled: sec.APIKey.Enabled,
			In:      stringToPtr(sec.APIKey.In),
			Key:     stringToPtr(sec.APIKey.Key),
		}
	}

	return specSec
}

// ConvertModelToSpecLLMDeploymentResponse is an LLM provider's or proxy's deployment as the API shows it: the status
// a user is shown for it on its gateway, where the transition is heading and why. An ARCHIVED deployment has no
// status row, so it has neither.
func ConvertModelToSpecLLMDeploymentResponse(d *gwmodels.Deployment) gwspec.LLMDeploymentResponse {
	shown := d.UserStatus()
	resp := gwspec.LLMDeploymentResponse{
		DeploymentId: d.DeploymentID.String(),
		Name:         d.Name,
		ArtifactId:   d.ArtifactUUID.String(),
		GatewayId:    d.GatewayUUID.String(),
		Metadata:     d.Metadata,
		CreatedAt:    d.CreatedAt,
		Status:       string(shown.Status),
		UpdatedAt:    d.UpdatedAt,
	}
	if d.OUID != "" {
		resp.OrganizationId = &d.OUID
	}
	if d.BaseDeploymentID != nil {
		base := d.BaseDeploymentID.String()
		resp.BaseDeploymentId = &base
	}
	if shown.Status != gwmodels.DeploymentStatusArchived {
		desired := string(*d.StatusDesired)
		resp.StatusDesired = &desired
	}
	if shown.Reason != "" {
		resp.StatusReason = &shown.Reason
	}
	return resp
}

// ConvertModelToSpecLLMDeploymentResponses converts a deployments list.
func ConvertModelToSpecLLMDeploymentResponses(deployments []*gwmodels.Deployment) []gwspec.LLMDeploymentResponse {
	resp := make([]gwspec.LLMDeploymentResponse, 0, len(deployments))
	for _, d := range deployments {
		resp = append(resp, ConvertModelToSpecLLMDeploymentResponse(d))
	}
	return resp
}

func ConvertToDeploymentDetailsResponse(deploymentDetails []*gwmodels.DeploymentResponse) map[string]gwspec.DeploymentDetailsResponse {
	result := make(map[string]gwspec.DeploymentDetailsResponse)

	if len(deploymentDetails) == 0 {
		return result
	}

	for _, deployment := range deploymentDetails {
		// Convert model endpoints to spec endpoints
		endpoints := make([]gwspec.DeploymentEndpoint, len(deployment.Endpoints))
		for i, endpoint := range deployment.Endpoints {
			endpoints[i] = gwspec.DeploymentEndpoint{
				Name:       endpoint.Name,
				Visibility: endpoint.Visibility,
				Url:        endpoint.URL,
			}
		}

		// Create the deployment details response
		var envDisplayName *string
		if deployment.EnvironmentDisplayName != "" {
			envDisplayName = &deployment.EnvironmentDisplayName
		}

		deploymentResponse := gwspec.DeploymentDetailsResponse{
			Status:                 deployment.Status,
			Endpoints:              endpoints,
			EnvironmentDisplayName: envDisplayName,
			GatewayStatus:          stringToPtr(deployment.GatewayStatus),
			GatewayStatusReason:    stringToPtr(deployment.GatewayStatusReason),
		}

		// Add to result map with environment name as key
		result[deployment.Environment] = deploymentResponse
	}

	return result
}

// PopulateAgentConfiguration fills the per-environment tracing, CORS and endpoint-authentication
// fields of resp from the agent's gateway settings row. No-op when cfg is nil (none stored yet).
func PopulateAgentConfiguration(resp *gwspec.AgentConfigurationResponse, cfg *gwmodels.AgentConfig) {
	if resp == nil || cfg == nil {
		return
	}
	resp.EnableAutoInstrumentation = gwspec.PtrBool(cfg.EnableAutoInstrumentation)
	resp.InstrumentationVersion = cfg.InstrumentationVersion
	resp.EnableApiKeySecurity = gwspec.PtrBool(cfg.EnableApiKeySecurity)
	resp.EnableOAuthSecurity = gwspec.PtrBool(cfg.EnableOAuthSecurity)
	resp.ResilienceTimeoutSeconds = cfg.ResilienceTimeoutSeconds
	resp.CorsConfig = &gwspec.CORSConfig{
		Enabled:          gwspec.PtrBool(cfg.CORSEnabled),
		AllowOrigin:      cfg.CORSAllowOrigins,
		AllowMethods:     cfg.CORSAllowMethods,
		AllowHeaders:     cfg.CORSAllowHeaders,
		AllowCredentials: gwspec.PtrBool(cfg.CORSAllowCredentials),
	}
	resp.OauthConfig = &gwspec.OAuthConfig{
		Issuers:          cfg.OAuthIssuers,
		Audiences:        cfg.OAuthAudiences,
		HeaderName:       &cfg.OAuthHeaderName,
		AuthHeaderPrefix: &cfg.OAuthAuthHeaderPrefix,
		ForwardToken:     &cfg.OAuthForwardToken,
	}
	card := cfg.EffectiveCardCORS()
	resp.AgentCardCorsConfig = &gwspec.AgentCardCORSConfig{
		Inherit:          gwspec.PtrBool(card.Inherited),
		Enabled:          gwspec.PtrBool(card.Enabled),
		AllowOrigin:      card.AllowOrigins,
		AllowHeaders:     card.AllowHeaders,
		AllowCredentials: gwspec.PtrBool(card.AllowCredentials),
	}
}
