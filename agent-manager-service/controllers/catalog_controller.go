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
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/google/uuid"

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
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to resolve organization")
		return
	}
	if org == nil {
		log.Error("ListCatalog: organization not found", "orgName", orgName)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	// Parse query parameters
	kind := r.URL.Query().Get("kind")
	name := r.URL.Query().Get("name")
	environmentUUID := r.URL.Query().Get("environmentId") // Changed from environmentName to environmentId
	limit := getIntQueryParam(r, "limit", utils.DefaultLimit)
	offset := getIntQueryParam(r, "offset", utils.DefaultOffset)

	// Validate parameters
	if limit < utils.MinLimit || limit > utils.MaxLimit {
		utils.WriteErrorResponse(w, http.StatusBadRequest,
			fmt.Sprintf("Invalid limit parameter. Must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}
	if offset < 0 {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid offset parameter. Must be non-negative")
		return
	}

	// Validate kind parameter if provided
	if kind != "" && !isValidCatalogKind(kind) {
		validKinds := []string{models.CatalogKindLLMProvider, models.CatalogKindAgent, models.CatalogKindMCP}
		log.Error("ListCatalog: invalid kind parameter", "kind", kind)
		utils.WriteErrorResponse(w, http.StatusBadRequest,
			fmt.Sprintf("Invalid kind parameter. Must be one of: %s", strings.Join(validKinds, ", ")))
		return
	}

	// Validate name parameter length if provided
	if name != "" && len(name) > 255 {
		log.Error("ListCatalog: name parameter too long", "length", len(name))
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Name parameter exceeds maximum length of 255 characters")
		return
	}

	// Validate environmentUUID parameter if provided
	if environmentUUID != "" {
		// Prevent DoS by checking length before parsing
		if len(environmentUUID) > 100 {
			log.Error("ListCatalog: environment UUID parameter too long", "length", len(environmentUUID))
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid environment UUID format")
			return
		}
		if _, err := uuid.Parse(environmentUUID); err != nil {
			log.Error("ListCatalog: invalid environment UUID format", "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid environment UUID format")
			return
		}
	}

	// For llmProvider kind, use the enhanced service method with filters
	if kind == models.CatalogKindLLMProvider {
		// Build filter struct
		filters := &models.CatalogListFilters{
			OrganizationUUID: org.UUID.String(),
			Kind:             kind,
			Name:             name,
			EnvironmentUUID:  environmentUUID,
			Limit:            limit,
			Offset:           offset,
		}

		llmEntries, total, err := c.catalogService.ListLLMProviders(ctx, filters)
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

func convertToCatalogListResponse(entries []models.CatalogEntry, total, limit, offset int32) *spec.CatalogListResponse {
	specEntries := make([]spec.CatalogListResponseEntriesInner, len(entries))
	for i, entry := range entries {
		catalogEntry := spec.CatalogEntry{
			Uuid:      entry.UUID.String(),
			Handle:    entry.Handle,
			Name:      entry.Name,
			Version:   entry.Version,
			Kind:      entry.Kind,
			InCatalog: entry.InCatalog,
			CreatedAt: entry.CreatedAt,
		}
		specEntries[i] = spec.CatalogEntryAsCatalogListResponseEntriesInner(&catalogEntry)
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
		Enabled:       security.Enabled,
		ApiKeyEnabled: security.APIKeyEnabled,
		ApiKeyIn:      security.APIKeyIn,
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

	// Model already has int32, no conversion needed
	result.RequestLimitCount = scope.RequestLimitCount
	result.TokenLimitCount = scope.TokenLimitCount

	if scope.CostLimitAmount != nil {
		result.CostLimitAmount = scope.CostLimitAmount
	}

	return result
}

// isValidCatalogKind validates if the provided kind is a valid catalog kind
func isValidCatalogKind(kind string) bool {
	validKinds := []string{
		models.CatalogKindLLMProvider,
		models.CatalogKindAgent,
		models.CatalogKindMCP,
	}
	for _, validKind := range validKinds {
		if kind == validKind {
			return true
		}
	}
	return false
}

// safeIntToInt32 safely converts int to int32 with validation
// Returns (converted value, true) if valid, (0, false) if invalid
// Rejects negative values (rate limits cannot be negative) and values exceeding int32 max
func safeIntToInt32(val int) (int32, bool) {
	// Rate limits must be non-negative
	if val < 0 {
		return 0, false
	}
	// Check if value exceeds int32 max
	if val > math.MaxInt32 {
		return 0, false
	}
	return int32(val), true
}

func convertDeploymentSummaries(deployments []models.DeploymentSummary) []spec.DeploymentSummary {
	result := make([]spec.DeploymentSummary, len(deployments))
	for i, d := range deployments {
		result[i] = spec.DeploymentSummary{
			GatewayId:       d.GatewayID.String(),
			GatewayName:     d.GatewayName,
			EnvironmentName: d.EnvironmentName,
			Status:          string(d.Status),
			DeployedAt:      d.DeployedAt,
		}
		if d.VHost != "" {
			result[i].Vhost = &d.VHost
		}
	}
	return result
}
