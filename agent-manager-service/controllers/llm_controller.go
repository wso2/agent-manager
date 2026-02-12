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
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// LLMController defines interface for LLM provider HTTP handlers
type LLMController interface {
	// Template handlers
	CreateLLMProviderTemplate(w http.ResponseWriter, r *http.Request)
	ListLLMProviderTemplates(w http.ResponseWriter, r *http.Request)
	GetLLMProviderTemplate(w http.ResponseWriter, r *http.Request)
	UpdateLLMProviderTemplate(w http.ResponseWriter, r *http.Request)
	DeleteLLMProviderTemplate(w http.ResponseWriter, r *http.Request)

	// Provider handlers
	CreateLLMProvider(w http.ResponseWriter, r *http.Request)
	ListLLMProviders(w http.ResponseWriter, r *http.Request)
	GetLLMProvider(w http.ResponseWriter, r *http.Request)
	UpdateLLMProvider(w http.ResponseWriter, r *http.Request)
	DeleteLLMProvider(w http.ResponseWriter, r *http.Request)

	// Proxy handlers
	CreateLLMProxy(w http.ResponseWriter, r *http.Request)
	ListLLMProxies(w http.ResponseWriter, r *http.Request)
	ListLLMProxiesByProvider(w http.ResponseWriter, r *http.Request)
	GetLLMProxy(w http.ResponseWriter, r *http.Request)
	UpdateLLMProxy(w http.ResponseWriter, r *http.Request)
	DeleteLLMProxy(w http.ResponseWriter, r *http.Request)
}

type llmController struct {
	templateService *services.LLMProviderTemplateService
	providerService *services.LLMProviderService
	proxyService    *services.LLMProxyService
	orgRepo         repositories.OrganizationRepository
}

// NewLLMController creates a new LLM controller
func NewLLMController(
	templateService *services.LLMProviderTemplateService,
	providerService *services.LLMProviderService,
	proxyService *services.LLMProxyService,
	orgRepo repositories.OrganizationRepository,
) LLMController {
	return &llmController{
		templateService: templateService,
		providerService: providerService,
		proxyService:    proxyService,
		orgRepo:         orgRepo,
	}
}

// resolveOrgUUID resolves organization handle to UUID
func (c *llmController) resolveOrgUUID(ctx context.Context, orgName string) (string, error) {
	org, err := c.orgRepo.GetOrganizationByHandle(orgName)
	if err != nil {
		return "", utils.ErrOrganizationNotFound
	}
	return org.UUID.String(), nil
}

// ---- Template Handlers ----

func (c *llmController) CreateLLMProviderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("CreateLLMProviderTemplate: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	var req models.LLMProviderTemplate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateLLMProviderTemplate: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	created, err := c.templateService.Create(orgID, "system", &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderTemplateExists):
			utils.WriteErrorResponse(w, http.StatusConflict, "LLM provider template already exists")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("CreateLLMProviderTemplate: failed to create template", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create LLM provider template")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, created)
}

func (c *llmController) ListLLMProviderTemplates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("ListLLMProviderTemplates: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Parse pagination parameters
	limit := getIntQueryParam(r, "limit", 20)
	offset := getIntQueryParam(r, "offset", 0)

	// Validate and cap limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	templates, totalCount, err := c.templateService.List(orgID, limit, offset)
	if err != nil {
		log.Error("ListLLMProviderTemplates: failed to list templates", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list LLM provider templates")
		return
	}

	resp := map[string]interface{}{
		"templates": templates,
		"total":     totalCount,
		"limit":     limit,
		"offset":    offset,
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) GetLLMProviderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	templateID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("GetLLMProviderTemplate: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	resp, err := c.templateService.Get(orgID, templateID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider template not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid template id")
			return
		default:
			log.Error("GetLLMProviderTemplate: failed to get template", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get LLM provider template")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) UpdateLLMProviderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	templateID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("UpdateLLMProviderTemplate: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	var req models.LLMProviderTemplate
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateLLMProviderTemplate: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := c.templateService.Update(orgID, templateID, &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider template not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("UpdateLLMProviderTemplate: failed to update template", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update LLM provider template")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) DeleteLLMProviderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	templateID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("DeleteLLMProviderTemplate: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	if err := c.templateService.Delete(orgID, templateID); err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider template not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid template id")
			return
		default:
			log.Error("DeleteLLMProviderTemplate: failed to delete template", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete LLM provider template")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// ---- Provider Handlers ----

func (c *llmController) CreateLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("CreateLLMProvider: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	var req models.LLMProvider
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateLLMProvider: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	created, err := c.providerService.Create(orgID, "system", &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderExists):
			utils.WriteErrorResponse(w, http.StatusConflict, "LLM provider already exists")
			return
		case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("CreateLLMProvider: failed to create provider", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create LLM provider")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, created)
}

func (c *llmController) ListLLMProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("ListLLMProviders: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Parse pagination parameters
	limit := getIntQueryParam(r, "limit", 20)
	offset := getIntQueryParam(r, "offset", 0)

	// Validate and cap limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	providers, totalCount, err := c.providerService.List(orgID, limit, offset)
	if err != nil {
		log.Error("ListLLMProviders: failed to list providers", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list LLM providers")
		return
	}

	resp := map[string]interface{}{
		"providers": providers,
		"total":     totalCount,
		"limit":     limit,
		"offset":    offset,
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) GetLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("GetLLMProvider: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	resp, err := c.providerService.Get(orgID, providerID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider id")
			return
		default:
			log.Error("GetLLMProvider: failed to get provider", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get LLM provider")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) UpdateLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("UpdateLLMProvider: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	var req models.LLMProvider
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateLLMProvider: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := c.providerService.Update(orgID, providerID, &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("UpdateLLMProvider: failed to update provider", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update LLM provider")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) DeleteLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("DeleteLLMProvider: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	if err := c.providerService.Delete(orgID, providerID); err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider id")
			return
		default:
			log.Error("DeleteLLMProvider: failed to delete provider", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete LLM provider")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// ---- Proxy Handlers ----

func (c *llmController) CreateLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("CreateLLMProxy: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	var req models.LLMProxy
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateLLMProxy: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// TODO: Validate project ID once LLMProxy model includes it
	// if req.ProjectID == "" {
	// 	utils.WriteErrorResponse(w, http.StatusBadRequest, "Project ID is required")
	// 	return
	// }

	created, err := c.proxyService.Create(orgID, "system", &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyExists):
			utils.WriteErrorResponse(w, http.StatusConflict, "LLM proxy already exists")
			return
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced provider not found")
			return
		case errors.Is(err, utils.ErrProjectNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("CreateLLMProxy: failed to create proxy", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create LLM proxy")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, created)
}

func (c *llmController) ListLLMProxies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("ListLLMProxies: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Parse optional projectId filter
	// Note: projectID filtering not yet supported by service implementation
	// projectID := r.URL.Query().Get("projectId")
	// var projectIDPtr *string
	// if projectID != "" {
	// 	projectIDPtr = &projectID
	// }

	// Parse pagination parameters
	limit := getIntQueryParam(r, "limit", 20)
	offset := getIntQueryParam(r, "offset", 0)

	// Validate and cap limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	// Note: projectIDPtr filtering not supported by current service implementation
	proxies, totalCount, err := c.proxyService.List(orgID, limit, offset)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("ListLLMProxies: failed to list proxies", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list LLM proxies")
		return
	}

	resp := map[string]interface{}{
		"proxies": proxies,
		"total":   totalCount,
		"limit":   limit,
		"offset":  offset,
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) ListLLMProxiesByProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("ListLLMProxiesByProvider: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Parse pagination parameters
	limit := getIntQueryParam(r, "limit", 20)
	offset := getIntQueryParam(r, "offset", 0)

	// Validate and cap limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	proxies, totalCount, err := c.proxyService.ListByProvider(orgID, providerID, limit, offset)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider id")
			return
		default:
			log.Error("ListLLMProxiesByProvider: failed to list proxies", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list LLM proxies")
			return
		}
	}

	resp := map[string]interface{}{
		"proxies": proxies,
		"total":   totalCount,
		"limit":   limit,
		"offset":  offset,
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) GetLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("GetLLMProxy: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	resp, err := c.proxyService.Get(orgID, proxyID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid proxy id")
			return
		default:
			log.Error("GetLLMProxy: failed to get proxy", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get LLM proxy")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) UpdateLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("UpdateLLMProxy: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	var req models.LLMProxy
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateLLMProxy: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := c.proxyService.Update(orgID, proxyID, &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("UpdateLLMProxy: failed to update proxy", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update LLM proxy")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) DeleteLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("DeleteLLMProxy: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	if err := c.proxyService.Delete(orgID, proxyID); err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid proxy id")
			return
		default:
			log.Error("DeleteLLMProxy: failed to delete proxy", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete LLM proxy")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}
