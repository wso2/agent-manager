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

package gwctl

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
)

// CatalogController defines the interface for catalog HTTP handlers
type CatalogController interface {
	ListCatalog(w http.ResponseWriter, r *http.Request)
}

type catalogController struct {
	catalogService gwsvc.CatalogService
}

// NewCatalogController creates a new catalog controller
func NewCatalogController(catalogService gwsvc.CatalogService) CatalogController {
	return &catalogController{
		catalogService: catalogService,
	}
}

// ListCatalog handles GET /orgs/{ouID}/catalog
func (c *catalogController) ListCatalog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)

	// Parse query parameters
	kind := r.URL.Query().Get("kind")
	name := r.URL.Query().Get("name")
	environmentUUID := r.URL.Query().Get("environmentId") // Changed from environmentName to environmentId
	limit := getIntQueryParam(r, "limit", gwutil.DefaultLimit)
	offset := getIntQueryParam(r, "offset", gwutil.DefaultOffset)

	// Validate parameters
	if limit < gwutil.MinLimit || limit > gwutil.MaxLimit {
		httpx.WriteErrorResponse(w, http.StatusBadRequest,
			fmt.Sprintf("Invalid limit parameter. Must be between %d and %d", gwutil.MinLimit, gwutil.MaxLimit))
		return
	}
	if offset < 0 {
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid offset parameter. Must be non-negative")
		return
	}

	// Validate kind parameter if provided
	if kind != "" && !isValidCatalogKind(kind) {
		validKinds := validCatalogKinds()
		log.Error("ListCatalog: invalid kind parameter", "kind", kind)
		httpx.WriteErrorResponse(w, http.StatusBadRequest,
			fmt.Sprintf("Invalid kind parameter. Must be one of: %s", strings.Join(validKinds, ", ")))
		return
	}

	// Validate name parameter length if provided
	if name != "" && len(name) > 255 {
		log.Error("ListCatalog: name parameter too long", "length", len(name))
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Name parameter exceeds maximum length of 255 characters")
		return
	}

	// Validate environmentUUID parameter if provided
	if environmentUUID != "" {
		// Prevent DoS by checking length before parsing
		if len(environmentUUID) > 100 {
			log.Error("ListCatalog: environment UUID parameter too long", "length", len(environmentUUID))
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid environment UUID format")
			return
		}
		if _, err := uuid.Parse(environmentUUID); err != nil {
			log.Error("ListCatalog: invalid environment UUID format", "error", err)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid environment UUID format")
			return
		}
	}

	// The API spells hosting in uppercase; the store in lowercase.
	var gatewayHosting string
	switch hosting := r.URL.Query().Get("gatewayHosting"); hosting {
	case "":
	case string(gwspec.GATEWAYHOSTING_MANAGED):
		gatewayHosting = gwmodels.GatewayHostingManaged
	case string(gwspec.GATEWAYHOSTING_SELF_HOSTED):
		gatewayHosting = gwmodels.GatewayHostingSelfHosted
	default:
		log.Error("ListCatalog: invalid gatewayHosting parameter", "gatewayHosting", hosting)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid gatewayHosting parameter. Must be one of: %s, %s",
			gwspec.GATEWAYHOSTING_MANAGED, gwspec.GATEWAYHOSTING_SELF_HOSTED))
		return
	}

	// For llmProvider kind, use the enhanced service method with filters
	if kind == gwmodels.CatalogKindLLMProvider {
		// Build filter struct
		filters := &gwmodels.CatalogListFilters{
			OrganizationName: ouID,
			Kind:             kind,
			Name:             name,
			EnvironmentUUID:  environmentUUID,
			GatewayHosting:   gatewayHosting,
			Limit:            limit,
			Offset:           offset,
		}

		llmEntries, total, err := c.catalogService.ListLLMProviders(ctx, filters)
		if err != nil {
			log.Error("ListCatalog: failed to list LLM providers", "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list catalog entries")
			return
		}

		// Convert to spec response
		response := convertToLLMProviderCatalogResponse(llmEntries, int32(total), int32(limit), int32(offset))
		httpx.WriteSuccessResponse(w, http.StatusOK, response)
		return
	}

	// For other kinds or no kind specified, use the basic service method
	entries, total, err := c.catalogService.ListCatalog(ctx, ouID, kind, limit, offset)
	if err != nil {
		log.Error("ListCatalog: failed to list catalog", "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list catalog entries")
		return
	}

	// Convert to spec response
	response := convertToCatalogListResponse(entries, int32(total), int32(limit), int32(offset))
	httpx.WriteSuccessResponse(w, http.StatusOK, response)
}

func convertToCatalogListResponse(entries []gwmodels.CatalogEntry, total, limit, offset int32) *gwspec.CatalogListResponse {
	specEntries := make([]gwspec.CatalogListResponseEntriesInner, len(entries))
	for i, entry := range entries {
		catalogEntry := gwspec.CatalogEntry{
			Uuid:      entry.UUID.String(),
			Handle:    entry.Handle,
			Name:      entry.Name,
			Version:   entry.Version,
			Kind:      entry.Kind,
			InCatalog: entry.InCatalog,
			CreatedAt: entry.CreatedAt,
		}
		specEntries[i] = gwspec.CatalogEntryAsCatalogListResponseEntriesInner(&catalogEntry)
	}

	return &gwspec.CatalogListResponse{
		Entries: specEntries,
		Total:   total,
		Limit:   limit,
		Offset:  offset,
	}
}

func convertToLLMProviderCatalogResponse(entries []gwmodels.CatalogLLMProviderEntry, total, limit, offset int32) any {
	// Convert to comprehensive LLM provider entries
	specEntries := make([]gwspec.CatalogLLMProviderEntry, len(entries))
	for i, entry := range entries {
		specEntry := gwspec.CatalogLLMProviderEntry{
			Uuid:      entry.UUID.String(),
			Handle:    entry.Handle,
			Name:      entry.Name,
			Version:   entry.Version,
			Kind:      entry.Kind,
			InCatalog: entry.InCatalog,
			Status:    entry.Status,
			Template:  entry.Template,
			CreatedAt: entry.CreatedAt,
			Policies:  entry.Policies,
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

func convertModelProviders(providers []gwmodels.LLMModelProvider) []gwspec.LLMModelProvider {
	result := make([]gwspec.LLMModelProvider, len(providers))
	for i, p := range providers {
		result[i] = gwspec.LLMModelProvider{
			Id:   p.ID,
			Name: &p.Name,
		}
		if len(p.Models) > 0 {
			models := make([]gwspec.LLMModel, len(p.Models))
			for j, m := range p.Models {
				models[j] = gwspec.LLMModel{
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

func convertSecuritySummary(security *gwmodels.SecuritySummary) *gwspec.SecuritySummary {
	return &gwspec.SecuritySummary{
		Enabled:       security.Enabled,
		ApiKeyEnabled: security.APIKeyEnabled,
		ApiKeyIn:      security.APIKeyIn,
	}
}

func convertRateLimitingSummary(rateLimiting *gwmodels.RateLimitingSummary) *gwspec.RateLimitingSummary {
	result := &gwspec.RateLimitingSummary{}

	if rateLimiting.ProviderLevel != nil {
		result.ProviderLevel = convertRateLimitingScope(rateLimiting.ProviderLevel)
	}

	if rateLimiting.ConsumerLevel != nil {
		result.ConsumerLevel = convertRateLimitingScope(rateLimiting.ConsumerLevel)
	}

	return result
}

// interfaceToFloat32 converts an interface{} holding a numeric value to float32.
// JSON numbers unmarshal as float64; int32 values are also handled.
func interfaceToFloat32(v interface{}) float32 {
	switch val := v.(type) {
	case float64:
		return float32(val)
	case float32:
		return val
	case int:
		return float32(val)
	case int32:
		return float32(val)
	case int64:
		return float32(val)
	}
	return 0
}

func convertRateLimitingScope(scope *gwmodels.RateLimitingScope) *gwspec.RateLimitingScope {
	result := &gwspec.RateLimitingScope{
		GlobalEnabled:       &scope.GlobalEnabled,
		ResourceWiseEnabled: &scope.ResourceWiseEnabled,
	}

	if scope.Request != nil {
		entry := gwspec.RateLimitEntry{
			Limit:         interfaceToFloat32(scope.Request.Limit),
			ResetDuration: scope.Request.ResetDuration,
			ResetUnit:     scope.Request.ResetUnit,
		}
		result.Request = &entry
	}
	if scope.Token != nil {
		entry := gwspec.RateLimitEntry{
			Limit:         interfaceToFloat32(scope.Token.Limit),
			ResetDuration: scope.Token.ResetDuration,
			ResetUnit:     scope.Token.ResetUnit,
		}
		result.Token = &entry
	}
	if scope.Cost != nil {
		entry := gwspec.RateLimitEntry{
			Limit:         interfaceToFloat32(scope.Cost.Limit),
			ResetDuration: scope.Cost.ResetDuration,
			ResetUnit:     scope.Cost.ResetUnit,
		}
		result.Cost = &entry
	}

	return result
}

// isValidCatalogKind validates if the provided kind is a valid catalog kind
func isValidCatalogKind(kind string) bool {
	for _, validKind := range validCatalogKinds() {
		if kind == validKind {
			return true
		}
	}
	return false
}

func validCatalogKinds() []string {
	return []string{
		gwmodels.CatalogKindLLMProvider,
		gwmodels.CatalogKindAgent,
		gwmodels.CatalogKindMCP,
		gwmodels.CatalogKindMCPProxy,
		gwmodels.CatalogKindMCPMapping,
	}
}

func convertDeploymentSummaries(deployments []gwmodels.DeploymentSummary) []gwspec.DeploymentSummary {
	result := make([]gwspec.DeploymentSummary, len(deployments))
	for i, d := range deployments {
		result[i] = gwspec.DeploymentSummary{
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
