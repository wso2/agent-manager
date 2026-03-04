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

package utils

import (
	"fmt"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
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
func GetOrDefaultProxyConfig(cfg *spec.LLMProxyConfig) spec.LLMProxyConfig {
	if cfg == nil {
		return spec.LLMProxyConfig{}
	}
	return *cfg
}

// ---- LLM Provider Template Conversions ----

// ConvertSpecToModelLLMProviderTemplate converts spec.CreateLLMProviderTemplateRequest to models.LLMProviderTemplate
// Note: The service layer handles Configuration field (JSON marshaling)
func ConvertSpecToModelLLMProviderTemplate(req *spec.CreateLLMProviderTemplateRequest, orgName string) *models.LLMProviderTemplate {
	template := &models.LLMProviderTemplate{
		UUID:             uuid.New(),
		OrganizationName: orgName,
		Handle:           req.Id, // ID is the handle
		Name:             req.Name,
		Description:      ptrToString(req.Description),
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

// ConvertModelToSpecLLMProviderTemplateResponse converts models.LLMProviderTemplate to spec.LLMProviderTemplateResponse
func ConvertModelToSpecLLMProviderTemplateResponse(model *models.LLMProviderTemplate) spec.LLMProviderTemplateResponse {
	resp := &spec.LLMProviderTemplateResponse{
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
func ConvertSpecToModelExtractionIdentifier(ei *spec.ExtractionIdentifier) *models.ExtractionIdentifier {
	if ei == nil {
		return nil
	}
	return &models.ExtractionIdentifier{
		Location:   ei.Location,
		Identifier: ei.Identifier,
	}
}

// ConvertModelToSpecExtractionIdentifier converts model to spec ExtractionIdentifier
func ConvertModelToSpecExtractionIdentifier(ei *models.ExtractionIdentifier) *spec.ExtractionIdentifier {
	if ei == nil {
		return nil
	}
	return &spec.ExtractionIdentifier{
		Location:   ei.Location,
		Identifier: ei.Identifier,
	}
}

// ConvertSpecToModelLLMProviderTemplateMetadata converts spec to model LLMProviderTemplateMetadata
func ConvertSpecToModelLLMProviderTemplateMetadata(meta *spec.LLMProviderTemplateMetadata) *models.LLMProviderTemplateMetadata {
	if meta == nil {
		return nil
	}
	metadata := &models.LLMProviderTemplateMetadata{
		EndpointURL:    ptrToString(meta.EndpointUrl),
		LogoURL:        ptrToString(meta.LogoUrl),
		OpenapiSpecURL: ptrToString(meta.OpenapiSpecUrl),
	}
	if meta.Auth != nil {
		metadata.Auth = &models.LLMProviderTemplateAuth{
			Type:        ptrToString(meta.Auth.Type),
			Header:      ptrToString(meta.Auth.Header),
			ValuePrefix: ptrToString(meta.Auth.ValuePrefix),
		}
	}
	return metadata
}

// ConvertModelToSpecLLMProviderTemplateMetadata converts model to spec LLMProviderTemplateMetadata
func ConvertModelToSpecLLMProviderTemplateMetadata(meta *models.LLMProviderTemplateMetadata) *spec.LLMProviderTemplateMetadata {
	if meta == nil {
		return nil
	}
	metadata := &spec.LLMProviderTemplateMetadata{
		EndpointUrl:    stringToPtr(meta.EndpointURL),
		LogoUrl:        stringToPtr(meta.LogoURL),
		OpenapiSpecUrl: stringToPtr(meta.OpenapiSpecURL),
	}
	if meta.Auth != nil {
		metadata.Auth = &spec.LLMProviderTemplateAuth{
			Type:        stringToPtr(meta.Auth.Type),
			Header:      stringToPtr(meta.Auth.Header),
			ValuePrefix: stringToPtr(meta.Auth.ValuePrefix),
		}
	}
	return metadata
}

// ---- LLM Provider Conversions ----

// ConvertSpecToModelLLMProvider converts spec.CreateLLMProviderRequest to models.LLMProvider
func ConvertSpecToModelLLMProvider(req *spec.CreateLLMProviderRequest, orgName string) *models.LLMProvider {
	provider := &models.LLMProvider{
		UUID:           uuid.New(),
		TemplateHandle: req.Template,
		Description:    ptrToString(req.Description),
		OpenAPISpec:    ptrToString(req.Openapi),
		Status:         "ACTIVE",
		Configuration:  ConvertSpecToModelLLMProviderConfigFromRequest(req),
	}

	// Convert model providers if present
	if len(req.ModelProviders) > 0 {
		provider.ModelProviders = make([]models.LLMModelProvider, len(req.ModelProviders))
		for i, mp := range req.ModelProviders {
			provider.ModelProviders[i] = ConvertSpecToModelLLMModelProvider(mp)
		}
	}

	return provider
}

// ConvertModelToSpecLLMProviderResponse converts models.LLMProvider to spec.LLMProviderResponse
func ConvertModelToSpecLLMProviderListItemResponse(model *models.LLMProvider) spec.LLMProviderListItem {
	resp := &spec.LLMProviderListItem{
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

// ConvertModelToSpecLLMProviderResponse converts models.LLMProvider to spec.LLMProviderResponse
func ConvertModelToSpecLLMProviderResponse(model *models.LLMProvider) spec.LLMProviderResponse {
	var upstream spec.UpstreamConfig
	if model.Configuration.Upstream != nil {
		upstream = ConvertModelToSpecUpstreamConfig(*model.Configuration.Upstream)
	}

	resp := &spec.LLMProviderResponse{
		Uuid:        model.UUID.String(),
		Id:          model.Artifact.Handle,
		Name:        model.Artifact.Name,
		Description: stringToPtr(model.Description),
		CreatedBy:   stringToPtr(model.CreatedBy),
		Version:     model.Artifact.Version,
		Context:     ptrToString(model.Configuration.Context),
		Template:    model.TemplateHandle,
		Upstream:    upstream,
		Openapi:     stringToPtr(model.OpenAPISpec),
		Status:      model.Status,
	}

	// Convert optional fields from configuration
	if model.Configuration.AccessControl != nil {
		ac := ConvertModelToSpecLLMAccessControl(*model.Configuration.AccessControl)
		resp.AccessControl = &ac
	}
	if len(model.Configuration.Policies) > 0 {
		resp.Policies = make([]spec.LLMPolicy, len(model.Configuration.Policies))
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

	// Convert model providers
	if len(model.ModelProviders) > 0 {
		resp.ModelProviders = make([]spec.LLMModelProvider, len(model.ModelProviders))
		for i, mp := range model.ModelProviders {
			resp.ModelProviders[i] = ConvertModelToSpecLLMModelProvider(mp)
		}
	}
	return *resp
}

// ConvertSpecToModelLLMProviderConfigFromRequest converts spec request to model configuration
// This is used for the new flat structure where all fields are at the top level
func ConvertSpecToModelLLMProviderConfigFromRequest(req *spec.CreateLLMProviderRequest) models.LLMProviderConfig {
	upstream := ConvertSpecToModelUpstreamConfig(req.Upstream)

	modelConfig := models.LLMProviderConfig{
		Name:     req.Name,
		Handle:   req.Id,
		Version:  req.Version,
		Context:  &req.Context,
		VHost:    nil,
		Template: req.Template,
		Upstream: &upstream,
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
		modelConfig.Policies = make([]models.LLMPolicy, len(req.Policies))
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

// ConvertSpecToModelLLMProxy converts spec.CreateLLMProxyRequest to models.LLMProxy
func ConvertSpecToModelLLMProxy(req *spec.CreateLLMProxyRequest, projectID string) (*models.LLMProxy, error) {
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, fmt.Errorf("invalid project UUID: %w", err)
	}

	providerUUID, err := uuid.Parse(req.ProviderUuid)
	if err != nil {
		return nil, fmt.Errorf("invalid provider UUID: %w", err)
	}

	proxy := &models.LLMProxy{
		UUID:          uuid.New(),
		ProjectUUID:   projectUUID,
		ProviderUUID:  providerUUID,
		Description:   ptrToString(req.Description),
		OpenAPISpec:   ptrToString(req.Openapi),
		Status:        "ACTIVE",
		Configuration: ConvertSpecToModelLLMProxyConfig(req.Configuration),
	}

	return proxy, nil
}

// ConvertModelToSpecLLMProxyResponse converts models.LLMProxy to spec.LLMProxyResponse
func ConvertModelToSpecLLMProxyResponse(model *models.LLMProxy) spec.LLMProxyResponse {
	resp := &spec.LLMProxyResponse{
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
func ConvertSpecToModelLLMProxyConfig(config spec.LLMProxyConfig) models.LLMProxyConfig {
	modelConfig := models.LLMProxyConfig{
		Name:     ptrToString(config.Name),
		Version:  ptrToString(config.Version),
		Context:  config.Context,
		Vhost:    config.Vhost,
		Provider: ptrToString(config.Provider),
	}

	// Note: UpstreamAuth is not part of the OpenAPI spec and is handled separately
	// in the preserveUpstreamAuthCredential function during updates

	if len(config.Policies) > 0 {
		modelConfig.Policies = make([]models.LLMPolicy, len(config.Policies))
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
func ConvertModelToSpecLLMProxyConfig(config models.LLMProxyConfig) spec.LLMProxyConfig {
	specConfig := spec.LLMProxyConfig{
		Name:     stringToPtr(config.Name),
		Version:  stringToPtr(config.Version),
		Context:  config.Context,
		Vhost:    config.Vhost,
		Provider: stringToPtr(config.Provider),
	}

	// Note: UpstreamAuth is intentionally not included in the spec response for security.
	// Credentials should not be exposed via API responses.

	if len(config.Policies) > 0 {
		specConfig.Policies = make([]spec.LLMPolicy, len(config.Policies))
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
func ConvertSpecToModelLLMModelProvider(specProvider spec.LLMModelProvider) models.LLMModelProvider {
	provider := models.LLMModelProvider{
		ID:   specProvider.Id,
		Name: ptrToString(specProvider.Name),
	}

	if len(specProvider.Models) > 0 {
		provider.Models = make([]models.LLMModel, len(specProvider.Models))
		for i, m := range specProvider.Models {
			provider.Models[i] = models.LLMModel{
				ID:          m.Id,
				Name:        ptrToString(m.Name),
				Description: ptrToString(m.Description),
			}
		}
	}

	return provider
}

// ConvertModelToSpecLLMModelProvider converts model to spec LLMModelProvider
func ConvertModelToSpecLLMModelProvider(model models.LLMModelProvider) spec.LLMModelProvider {
	provider := spec.LLMModelProvider{
		Id:   model.ID,
		Name: stringToPtr(model.Name),
	}

	if len(model.Models) > 0 {
		provider.Models = make([]spec.LLMModel, len(model.Models))
		for i, m := range model.Models {
			provider.Models[i] = spec.LLMModel{
				Id:          m.ID,
				Name:        stringToPtr(m.Name),
				Description: stringToPtr(m.Description),
			}
		}
	}

	return provider
}

// ConvertSpecToModelUpstreamConfig converts spec to model UpstreamConfig
func ConvertSpecToModelUpstreamConfig(config spec.UpstreamConfig) models.UpstreamConfig {
	modelConfig := models.UpstreamConfig{}

	if config.Main != nil {
		main := models.UpstreamEndpoint{
			URL: *config.Main.Url,
			Ref: ptrToString(config.Main.Ref),
		}
		if config.Main.Auth != nil {
			main.Auth = &models.UpstreamAuth{
				Type:   &config.Main.Auth.Type,
				Value:  config.Main.Auth.Value,
				Header: config.Main.Auth.Header,
			}
		}
		modelConfig.Main = &main
	}

	if config.Sandbox != nil {
		sandbox := models.UpstreamEndpoint{
			URL: *config.Sandbox.Url,
			Ref: ptrToString(config.Sandbox.Ref),
		}
		if config.Sandbox.Auth != nil {
			sandbox.Auth = &models.UpstreamAuth{
				Type:   &config.Sandbox.Auth.Type,
				Value:  config.Sandbox.Auth.Value,
				Header: config.Sandbox.Auth.Header,
			}
		}
		modelConfig.Sandbox = &sandbox
	}

	return modelConfig
}

// ConvertModelToSpecUpstreamConfig converts model to spec UpstreamConfig
func ConvertModelToSpecUpstreamConfig(config models.UpstreamConfig) spec.UpstreamConfig {
	specConfig := spec.UpstreamConfig{}

	if config.Main != nil {
		main := spec.UpstreamEndpoint{
			Url: &config.Main.URL,
			Ref: stringToPtr(config.Main.Ref),
		}
		if config.Main.Auth != nil {
			// Mask credential value in API responses for security
			maskedValue := "***REDACTED***"
			main.Auth = &spec.UpstreamAuth{
				Type:  *config.Main.Auth.Type,
				Value: &maskedValue,
			}
		}
		specConfig.Main = &main
	}

	if config.Sandbox != nil {
		sandbox := spec.UpstreamEndpoint{
			Url: &config.Sandbox.URL,
			Ref: stringToPtr(config.Sandbox.Ref),
		}
		if config.Sandbox.Auth != nil {
			// Mask credential value in API responses for security
			maskedValue := "***REDACTED***"
			sandbox.Auth = &spec.UpstreamAuth{
				Type:  *config.Sandbox.Auth.Type,
				Value: &maskedValue,
			}
		}
		specConfig.Sandbox = &sandbox
	}

	return specConfig
}

// ConvertSpecToModelLLMAccessControl converts spec to model LLMAccessControl
func ConvertSpecToModelLLMAccessControl(ac spec.LLMAccessControl) models.LLMAccessControl {
	modelAC := models.LLMAccessControl{
		Mode: ac.Mode,
	}

	if len(ac.Exceptions) > 0 {
		modelAC.Exceptions = make([]models.RouteException, len(ac.Exceptions))
		for i, e := range ac.Exceptions {
			modelAC.Exceptions[i] = models.RouteException{
				Path:    e.Path,
				Methods: e.Methods,
			}
		}
	}

	return modelAC
}

// ConvertModelToSpecLLMAccessControl converts model to spec LLMAccessControl
func ConvertModelToSpecLLMAccessControl(ac models.LLMAccessControl) spec.LLMAccessControl {
	specAC := spec.LLMAccessControl{
		Mode: ac.Mode,
	}

	if len(ac.Exceptions) > 0 {
		specAC.Exceptions = make([]spec.RouteException, len(ac.Exceptions))
		for i, e := range ac.Exceptions {
			specAC.Exceptions[i] = spec.RouteException{
				Path:    e.Path,
				Methods: e.Methods,
			}
		}
	}

	return specAC
}

// ConvertSpecToModelLLMRateLimitingConfig converts spec to model LLMRateLimitingConfig
func ConvertSpecToModelLLMRateLimitingConfig(rl spec.LLMRateLimitingConfig) models.LLMRateLimitingConfig {
	modelRL := models.LLMRateLimitingConfig{}

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
func ConvertModelToSpecLLMRateLimitingConfig(rl models.LLMRateLimitingConfig) spec.LLMRateLimitingConfig {
	specRL := spec.LLMRateLimitingConfig{}

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
func ConvertSpecToModelRateLimitingScopeConfig(scope spec.RateLimitingScopeConfig) models.RateLimitingScopeConfig {
	modelScope := models.RateLimitingScopeConfig{}

	if scope.Global != nil {
		global := ConvertSpecToModelRateLimitingLimitConfig(*scope.Global)
		modelScope.Global = &global
	}
	if scope.ResourceWise != nil {
		resourceWise := models.ResourceWiseRateLimitingConfig{
			Default: ConvertSpecToModelRateLimitingLimitConfig(scope.ResourceWise.Default),
		}
		if len(scope.ResourceWise.Resources) > 0 {
			resourceWise.Resources = make([]models.RateLimitingResourceLimit, len(scope.ResourceWise.Resources))
			for i, r := range scope.ResourceWise.Resources {
				resourceWise.Resources[i] = models.RateLimitingResourceLimit{
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
func ConvertModelToSpecRateLimitingScopeConfig(scope models.RateLimitingScopeConfig) spec.RateLimitingScopeConfig {
	specScope := spec.RateLimitingScopeConfig{}

	if scope.Global != nil {
		global := ConvertModelToSpecRateLimitingLimitConfig(*scope.Global)
		specScope.Global = &global
	}
	if scope.ResourceWise != nil {
		resourceWise := spec.ResourceWiseRateLimitingConfig{
			Default: ConvertModelToSpecRateLimitingLimitConfig(scope.ResourceWise.Default),
		}
		if len(scope.ResourceWise.Resources) > 0 {
			resourceWise.Resources = make([]spec.RateLimitingResourceLimit, len(scope.ResourceWise.Resources))
			for i, r := range scope.ResourceWise.Resources {
				resourceWise.Resources[i] = spec.RateLimitingResourceLimit{
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
func ConvertSpecToModelRateLimitingLimitConfig(limit spec.RateLimitingLimitConfig) models.RateLimitingLimitConfig {
	modelLimit := models.RateLimitingLimitConfig{}

	if limit.Request != nil {
		modelLimit.Request = &models.RequestRateLimit{
			Enabled: limit.Request.Enabled,
			Count:   int(limit.Request.Count),
			Reset: models.RateLimitResetWindow{
				Duration: int(limit.Request.Reset.Duration),
				Unit:     limit.Request.Reset.Unit,
			},
		}
	}
	if limit.Token != nil {
		modelLimit.Token = &models.TokenRateLimit{
			Enabled: limit.Token.Enabled,
			Count:   int(limit.Token.Count),
			Reset: models.RateLimitResetWindow{
				Duration: int(limit.Token.Reset.Duration),
				Unit:     limit.Token.Reset.Unit,
			},
		}
	}
	if limit.Cost != nil {
		modelLimit.Cost = &models.CostRateLimit{
			Enabled: limit.Cost.Enabled,
			Amount:  limit.Cost.Amount,
			Reset: models.RateLimitResetWindow{
				Duration: int(limit.Cost.Reset.Duration),
				Unit:     limit.Cost.Reset.Unit,
			},
		}
	}

	return modelLimit
}

// ConvertModelToSpecRateLimitingLimitConfig converts model to spec RateLimitingLimitConfig
func ConvertModelToSpecRateLimitingLimitConfig(limit models.RateLimitingLimitConfig) spec.RateLimitingLimitConfig {
	specLimit := spec.RateLimitingLimitConfig{}

	if limit.Request != nil {
		specLimit.Request = &spec.RequestRateLimit{
			Enabled: limit.Request.Enabled,
			Count:   int32(limit.Request.Count),
			Reset: spec.RateLimitResetWindow{
				Duration: int32(limit.Request.Reset.Duration),
				Unit:     limit.Request.Reset.Unit,
			},
		}
	}
	if limit.Token != nil {
		specLimit.Token = &spec.TokenRateLimit{
			Enabled: limit.Token.Enabled,
			Count:   int32(limit.Token.Count),
			Reset: spec.RateLimitResetWindow{
				Duration: int32(limit.Token.Reset.Duration),
				Unit:     limit.Token.Reset.Unit,
			},
		}
	}
	if limit.Cost != nil {
		specLimit.Cost = &spec.CostRateLimit{
			Enabled: limit.Cost.Enabled,
			Amount:  limit.Cost.Amount,
			Reset: spec.RateLimitResetWindow{
				Duration: int32(limit.Cost.Reset.Duration),
				Unit:     limit.Cost.Reset.Unit,
			},
		}
	}

	return specLimit
}

// ConvertSpecToModelLLMPolicy converts spec to model LLMPolicy
func ConvertSpecToModelLLMPolicy(policy spec.LLMPolicy) models.LLMPolicy {
	modelPolicy := models.LLMPolicy{
		Name:    policy.Name,
		Version: policy.Version,
	}

	if len(policy.Paths) > 0 {
		modelPolicy.Paths = make([]models.LLMPolicyPath, len(policy.Paths))
		for i, p := range policy.Paths {
			modelPolicy.Paths[i] = models.LLMPolicyPath{
				Path:    p.Path,
				Methods: p.Methods,
				Params:  p.Params,
			}
		}
	}

	return modelPolicy
}

// ConvertModelToSpecLLMPolicy converts model to spec LLMPolicy
func ConvertModelToSpecLLMPolicy(policy models.LLMPolicy) spec.LLMPolicy {
	specPolicy := spec.LLMPolicy{
		Name:    policy.Name,
		Version: policy.Version,
	}

	if len(policy.Paths) > 0 {
		specPolicy.Paths = make([]spec.LLMPolicyPath, len(policy.Paths))
		for i, p := range policy.Paths {
			specPolicy.Paths[i] = spec.LLMPolicyPath{
				Path:    p.Path,
				Methods: p.Methods,
				Params:  p.Params,
			}
		}
	}

	return specPolicy
}

// ConvertSpecToModelSecurityConfig converts spec to model SecurityConfig
func ConvertSpecToModelSecurityConfig(sec spec.SecurityConfig) models.SecurityConfig {
	modelSec := models.SecurityConfig{
		Enabled: sec.Enabled,
	}

	if sec.ApiKey != nil {
		modelSec.APIKey = &models.APIKeySecurity{
			Enabled: sec.ApiKey.Enabled,
			In:      ptrToString(sec.ApiKey.In),
			Key:     ptrToString(sec.ApiKey.Key),
		}
	}

	return modelSec
}

// ConvertModelToSpecSecurityConfig converts model to spec SecurityConfig
func ConvertModelToSpecSecurityConfig(sec models.SecurityConfig) spec.SecurityConfig {
	specSec := spec.SecurityConfig{
		Enabled: sec.Enabled,
	}

	if sec.APIKey != nil {
		specSec.ApiKey = &spec.APIKeySecurity{
			Enabled: sec.APIKey.Enabled,
			In:      stringToPtr(sec.APIKey.In),
			Key:     stringToPtr(sec.APIKey.Key),
		}
	}

	return specSec
}
