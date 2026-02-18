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
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// CatalogController defines the interface for catalog HTTP handlers
type CatalogController interface {
	ListCatalog(w http.ResponseWriter, r *http.Request)
}

type catalogController struct {
	catalogService services.CatalogService
	orgRepo        repositories.OrganizationRepository
}

// NewCatalogController creates a new catalog controller
func NewCatalogController(catalogService services.CatalogService, orgRepo repositories.OrganizationRepository) CatalogController {
	return &catalogController{
		catalogService: catalogService,
		orgRepo:        orgRepo,
	}
}

// ListCatalog handles GET /orgs/{orgName}/catalog
func (c *catalogController) ListCatalog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)

	// Resolve organization UUID
	org, err := c.orgRepo.GetOrganizationByName(orgName)
	if err != nil {
		log.Error("ListCatalog: failed to get organization", "orgName", orgName, "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	// Parse query parameters
	kind := r.URL.Query().Get("kind")
	environmentName := r.URL.Query().Get("environmentName")
	limit := getIntQueryParam(r, "limit", utils.DefaultLimit)
	offset := getIntQueryParam(r, "offset", utils.DefaultOffset)

	// Validate parameters
	if limit < utils.MinLimit || limit > utils.MaxLimit {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid limit parameter")
		return
	}
	if offset < 0 {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid offset parameter")
		return
	}

	// Validate kind parameter if provided
	if kind != "" && !isValidCatalogKind(kind) {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid kind parameter. Must be one of: llmProvider, agent, mcp")
		return
	}

	// For llmProvider kind, use the enhanced service method
	if kind == models.CatalogKindLLMProvider {
		var envFilter *string
		if environmentName != "" {
			envFilter = &environmentName
		}

		llmEntries, total, err := c.catalogService.ListLLMProviders(ctx, org.UUID.String(), envFilter, limit, offset)
		if err != nil {
			log.Error("ListCatalog: failed to list LLM providers", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list catalog entries")
			return
		}

		// Convert to spec response
		response := convertToLLMProviderCatalogResponse(llmEntries, int32(total), int32(limit), int32(offset))
		utils.WriteSuccessResponse(w, http.StatusOK, response)
		return
	}

	// For other kinds or no kind specified, use the basic service method
	entries, total, err := c.catalogService.ListCatalog(ctx, org.UUID.String(), kind, limit, offset)
	if err != nil {
		log.Error("ListCatalog: failed to list catalog", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list catalog entries")
		return
	}

	// Convert to spec response
	response := convertToCatalogListResponse(entries, int32(total), int32(limit), int32(offset))
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// Helper functions

func isValidCatalogKind(kind string) bool {
	validKinds := map[string]bool{
		models.CatalogKindLLMProvider: true,
		models.CatalogKindAgent:       true,
		models.CatalogKindMCP:         true,
	}
	return validKinds[kind]
}

func convertToCatalogListResponse(entries []models.CatalogEntry, total, limit, offset int32) *spec.CatalogListResponse {
	specEntries := make([]spec.CatalogEntry, len(entries))
	for i, entry := range entries {
		specEntries[i] = spec.CatalogEntry{
			Uuid:      entry.UUID.String(),
			Handle:    entry.Handle,
			Name:      entry.Name,
			Version:   entry.Version,
			Kind:      entry.Kind,
			InCatalog: entry.InCatalog,
			CreatedAt: entry.CreatedAt,
		}
	}

	return &spec.CatalogListResponse{
		Entries: specEntries,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}
}

func convertToLLMProviderCatalogResponse(entries []models.CatalogLLMProviderEntry, total, limit, offset int32) any {
	// Convert to comprehensive LLM provider entries
	specEntries := make([]spec.CatalogLLMProviderEntry, len(entries))
	for i, entry := range entries {
		specEntry := spec.CatalogLLMProviderEntry{
			Uuid:      entry.UUID.String(),
			Handle:    entry.Handle,
			Name:      entry.Name,
			Version:   entry.Version,
			Kind:      entry.Kind,
			InCatalog: entry.InCatalog,
			Status:    entry.Status,
			Template:  entry.Template,
			CreatedAt: entry.CreatedAt,
		}

		// Optional fields
		if entry.Description != "" {
			specEntry.Description = &entry.Description
		}
		if entry.CreatedBy != "" {
			specEntry.CreatedBy = &entry.CreatedBy
		}
		if entry.Context != nil {
			specEntry.Context = entry.Context
		}
		if entry.VHost != nil {
			specEntry.Vhost = entry.VHost
		}

		// Model providers
		if len(entry.ModelProviders) > 0 {
			specEntry.ModelProviders = convertModelProviders(entry.ModelProviders)
		}

		// Security summary
		if entry.Security != nil {
			specEntry.Security = convertSecuritySummary(entry.Security)
		}

		// Rate limiting summary
		if entry.RateLimiting != nil {
			specEntry.RateLimiting = convertRateLimitingSummary(entry.RateLimiting)
		}

		// Deployments
		if len(entry.Deployments) > 0 {
			specEntry.Deployments = convertDeploymentSummaries(entry.Deployments)
		}

		specEntries[i] = specEntry
	}

	// Return a custom response with comprehensive entries
	return map[string]any{
		"entries": specEntries,
		"total":   total,
		"limit":   limit,
		"offset":  offset,
	}
}

func convertModelProviders(providers []models.LLMModelProvider) []spec.LLMModelProvider {
	result := make([]spec.LLMModelProvider, len(providers))
	for i, p := range providers {
		result[i] = spec.LLMModelProvider{
			Id:   p.ID,
			Name: &p.Name,
		}
		if len(p.Models) > 0 {
			models := make([]spec.LLMModel, len(p.Models))
			for j, m := range p.Models {
				models[j] = spec.LLMModel{
					Id:   m.ID,
					Name: &m.Name,
				}
				if m.Description != "" {
					models[j].Description = &m.Description
				}
			}
			result[i].Models = models
		}
	}
	return result
}

func convertSecuritySummary(security *models.SecuritySummary) *spec.SecuritySummary {
	return &spec.SecuritySummary{
		Enabled:       &security.Enabled,
		ApiKeyEnabled: &security.APIKeyEnabled,
		ApiKeyIn:      &security.APIKeyIn,
	}
}

func convertRateLimitingSummary(rateLimiting *models.RateLimitingSummary) *spec.RateLimitingSummary {
	result := &spec.RateLimitingSummary{}

	if rateLimiting.ProviderLevel != nil {
		result.ProviderLevel = convertRateLimitingScope(rateLimiting.ProviderLevel)
	}

	if rateLimiting.ConsumerLevel != nil {
		result.ConsumerLevel = convertRateLimitingScope(rateLimiting.ConsumerLevel)
	}

	return result
}

func convertRateLimitingScope(scope *models.RateLimitingScope) *spec.RateLimitingScope {
	result := &spec.RateLimitingScope{
		GlobalEnabled:       &scope.GlobalEnabled,
		ResourceWiseEnabled: &scope.ResourceWiseEnabled,
	}

	if scope.RequestLimitCount != nil {
		count := int32(*scope.RequestLimitCount)
		result.RequestLimitCount = &count
	}

	if scope.TokenLimitCount != nil {
		count := int32(*scope.TokenLimitCount)
		result.TokenLimitCount = &count
	}

	if scope.CostLimitAmount != nil {
		result.CostLimitAmount = scope.CostLimitAmount
	}

	return result
}

func convertDeploymentSummaries(deployments []models.DeploymentSummary) []spec.DeploymentSummary {
	result := make([]spec.DeploymentSummary, len(deployments))
	for i, d := range deployments {
		gatewayDisplayName := d.GatewayDisplayName
		environmentName := d.EnvironmentName

		result[i] = spec.DeploymentSummary{
			GatewayId:          d.GatewayID.String(),
			GatewayName:        d.GatewayName,
			GatewayDisplayName: &gatewayDisplayName,
			EnvironmentName:    &environmentName,
			Status:             string(d.Status),
		}
		if d.DeployedAt != nil {
			result[i].DeployedAt = d.DeployedAt
		}
		if d.VHost != "" {
			result[i].Vhost = &d.VHost
		}
	}
	return result
}
