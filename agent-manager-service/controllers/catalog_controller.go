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

	// Call service
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
