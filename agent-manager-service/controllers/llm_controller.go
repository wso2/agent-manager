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
	"fmt"
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
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
	UpdateLLMProviderCatalogStatus(w http.ResponseWriter, r *http.Request)
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
	templateService   *services.LLMProviderTemplateService
	providerService   *services.LLMProviderService
	proxyService      *services.LLMProxyService
	deploymentService *services.LLMProviderDeploymentService
	artifactRepo      repositories.ArtifactRepository
	ocClient          client.OpenChoreoClient
}

// NewLLMController creates a new LLM controller
func NewLLMController(
	templateService *services.LLMProviderTemplateService,
	providerService *services.LLMProviderService,
	proxyService *services.LLMProxyService,
	deploymentService *services.LLMProviderDeploymentService,
	artifactRepo repositories.ArtifactRepository,
	ocClient client.OpenChoreoClient,
) LLMController {
	return &llmController{
		templateService:   templateService,
		providerService:   providerService,
		proxyService:      proxyService,
		deploymentService: deploymentService,
		artifactRepo:      artifactRepo,
		ocClient:          ocClient,
	}
}

// resolveProjectUUID resolves project name to UUID using OpenChoreo client
func (c *llmController) resolveProjectUUID(ctx context.Context, orgName, projectName string) (string, error) {
	project, err := c.ocClient.GetProject(ctx, orgName, projectName)
	if err != nil {
		// Check if it's specifically a not-found error
		if errors.Is(err, utils.ErrProjectNotFound) {
			return "", utils.ErrProjectNotFound
		}
		// Return other errors (network, RPC, backend failures) as-is
		return "", fmt.Errorf("GetProject: %w", err)
	}
	if project == nil {
		return "", utils.ErrProjectNotFound
	}
	return project.UUID, nil
}

// ---- Template Handlers ----

func (c *llmController) CreateLLMProviderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	var req spec.CreateLLMProviderTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateLLMProviderTemplate: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to model
	template := utils.ConvertSpecToModelLLMProviderTemplate(&req, orgName)

	created, err := c.templateService.Create(orgName, "system", template)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrSystemTemplateOverride):
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot use handle of built-in template")
			return
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

	// Convert model response to spec
	response := utils.ConvertModelToSpecLLMProviderTemplateResponse(created)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *llmController) ListLLMProviderTemplates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

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

	templates, totalCount, err := c.templateService.List(orgName, limit, offset)
	if err != nil {
		log.Error("ListLLMProviderTemplates: failed to list templates", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list LLM provider templates")
		return
	}

	// Convert models to spec responses
	specTemplates := make([]spec.LLMProviderTemplateResponse, len(templates))
	for i, t := range templates {
		specTemplates[i] = utils.ConvertModelToSpecLLMProviderTemplateResponse(t)
	}

	resp := spec.LLMProviderTemplateListResponse{
		Templates: specTemplates,
		Total:     int32(totalCount),
		Limit:     int32(limit),
		Offset:    int32(offset),
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) GetLLMProviderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	templateID := r.PathValue("id")

	template, err := c.templateService.Get(orgName, templateID)
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

	// Convert model to spec response
	response := utils.ConvertModelToSpecLLMProviderTemplateResponse(template)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *llmController) UpdateLLMProviderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	templateID := r.PathValue("id")

	var req spec.UpdateLLMProviderTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateLLMProviderTemplate: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to model - create minimal template with only updatable fields
	template := &spec.CreateLLMProviderTemplateRequest{
		Id:               templateID,
		Name:             utils.GetOrDefault(req.Name, ""),
		Description:      req.Description,
		Metadata:         req.Metadata,
		PromptTokens:     req.PromptTokens,
		CompletionTokens: req.CompletionTokens,
		TotalTokens:      req.TotalTokens,
		RemainingTokens:  req.RemainingTokens,
		RequestModel:     req.RequestModel,
		ResponseModel:    req.ResponseModel,
	}
	modelTemplate := utils.ConvertSpecToModelLLMProviderTemplate(template, orgName)

	updated, err := c.templateService.Update(orgName, templateID, modelTemplate)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrSystemTemplateImmutable):
			utils.WriteErrorResponse(w, http.StatusForbidden, "System templates cannot be modified")
			return
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

	// Convert model to spec response
	response := utils.ConvertModelToSpecLLMProviderTemplateResponse(updated)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *llmController) DeleteLLMProviderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	templateID := r.PathValue("id")

	if err := c.templateService.Delete(orgName, templateID); err != nil {
		switch {
		case errors.Is(err, utils.ErrSystemTemplateImmutable):
			utils.WriteErrorResponse(w, http.StatusForbidden, "System templates cannot be deleted")
			return
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

	log.Info("CreateLLMProvider: starting", "orgName", orgName)

	log.Info("CreateLLMProvider: organization resolved", "orgName", orgName, "orgName", orgName)

	var req spec.CreateLLMProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateLLMProvider: failed to decode request", "orgName", orgName, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	log.Info("CreateLLMProvider: request decoded", "orgName", orgName, "templateHandle", req.TemplateHandle,
		"configName", ptrToStringLog(req.Configuration.Name),
		"configVersion", ptrToStringLog(req.Configuration.Version),
		"configTemplate", ptrToStringLog(req.Configuration.Template),
		"gatewayCount", len(req.Gateways))

	// Convert spec request to model
	provider := utils.ConvertSpecToModelLLMProvider(&req, orgName)
	log.Info("CreateLLMProvider: calling service layer", "orgName", orgName, "orgName", orgName,
		"providerName", provider.Configuration.Name,
		"providerVersion", provider.Configuration.Version,
		"templateHandle", provider.TemplateHandle)

	var created *models.LLMProvider

	// Check if gateways list is present and not empty
	if len(req.Gateways) > 0 {
		log.Info("CreateLLMProvider: creating and deploying provider to gateways", "orgName", orgName, "gatewayCount", len(req.Gateways))
		resp, err := c.providerService.CreateAndDeploy(orgName, "system", provider, req.Gateways, c.deploymentService)
		if err != nil {
			switch {
			case errors.Is(err, utils.ErrLLMProviderExists):
				log.Warn("CreateLLMProvider: provider already exists", "orgName", orgName, "providerName", provider.Configuration.Name)
				utils.WriteErrorResponse(w, http.StatusConflict, "LLM provider already exists")
				return
			case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
				log.Error("CreateLLMProvider: template not found", "orgName", orgName, "templateHandle", req.TemplateHandle, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
				return
			case errors.Is(err, utils.ErrInvalidInput):
				log.Error("CreateLLMProvider: invalid input", "orgName", orgName, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
				return
			default:
				log.Error("CreateLLMProvider: failed to create provider", "orgName", orgName, "error", err)
				utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create LLM provider")
				return
			}
		}
		created = resp.Provider
		// Log deployment results
		successCount := 0
		failedCount := 0
		for _, result := range resp.Deployments {
			if result.Success {
				successCount++
			} else {
				failedCount++
				log.Warn("CreateLLMProvider: deployment failed for gateway", "orgName", orgName, "gatewayID", result.GatewayID, "error", result.Error)
			}
		}
		log.Info("CreateLLMProvider: deployment results", "orgName", orgName, "successCount", successCount, "failedCount", failedCount, "totalRequested", len(req.Gateways))
	} else {
		log.Info("CreateLLMProvider: creating provider without deployment", "orgName", orgName)
		var err error
		created, err = c.providerService.Create(orgName, "system", provider)
		if err != nil {
			switch {
			case errors.Is(err, utils.ErrLLMProviderExists):
				log.Warn("CreateLLMProvider: provider already exists", "orgName", orgName, "providerName", provider.Configuration.Name)
				utils.WriteErrorResponse(w, http.StatusConflict, "LLM provider already exists")
				return
			case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
				log.Error("CreateLLMProvider: template not found", "orgName", orgName, "templateHandle", req.TemplateHandle, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
				return
			case errors.Is(err, utils.ErrInvalidInput):
				log.Error("CreateLLMProvider: invalid input", "orgName", orgName, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
				return
			default:
				log.Error("CreateLLMProvider: failed to create provider", "orgName", orgName, "error", err)
				utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create LLM provider")
				return
			}
		}
	}

	log.Info("CreateLLMProvider: provider created successfully", "orgName", orgName, "providerUUID", created.UUID, "providerName", created.Configuration.Name)

	// Convert model to spec response
	response := utils.ConvertModelToSpecLLMProviderResponse(created)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

// Helper function to safely convert pointer to string for logging
func ptrToStringLog(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func (c *llmController) ListLLMProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	log.Info("ListLLMProviders: starting", "orgName", orgName)

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

	log.Info("ListLLMProviders: calling service layer", "orgName", orgName, "orgName", orgName, "limit", limit, "offset", offset)

	providers, totalCount, err := c.providerService.List(orgName, limit, offset)
	if err != nil {
		log.Error("ListLLMProviders: failed to list providers", "orgName", orgName, "orgName", orgName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list LLM providers")
		return
	}

	log.Info("ListLLMProviders: providers retrieved", "orgName", orgName, "count", len(providers), "total", totalCount)

	// Convert models to spec responses
	specProviders := make([]spec.LLMProviderResponse, len(providers))
	for i, p := range providers {
		specProviders[i] = utils.ConvertModelToSpecLLMProviderResponse(p)
	}

	resp := spec.LLMProviderListResponse{
		Providers: specProviders,
		Total:     int32(totalCount),
		Limit:     int32(limit),
		Offset:    int32(offset),
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) GetLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	log.Info("GetLLMProvider: starting", "orgName", orgName, "providerID", providerID)

	log.Info("GetLLMProvider: calling service layer", "orgName", orgName, "orgName", orgName, "providerID", providerID)

	provider, err := c.providerService.Get(providerID, orgName)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("GetLLMProvider: provider not found", "orgName", orgName, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("GetLLMProvider: invalid provider id", "orgName", orgName, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider id")
			return
		default:
			log.Error("GetLLMProvider: failed to get provider", "orgName", orgName, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get LLM provider")
			return
		}
	}

	log.Info("GetLLMProvider: provider retrieved", "orgName", orgName, "providerID", providerID, "providerUUID", provider.UUID)

	// Convert model to spec response
	response := utils.ConvertModelToSpecLLMProviderResponse(provider)

	gatewayMappings, err := c.providerService.GetProviderGatewayMapping(provider.UUID, orgName, c.deploymentService)
	if err != nil {
		log.Error("error while fetching deployed gateways for provider", "providerID", providerID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Error fetching deployed gateways")
		return
	}

	response.SetGateways(gatewayMappings)

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *llmController) UpdateLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	log.Info("UpdateLLMProvider: starting", "orgName", orgName, "providerID", providerID)

	var req spec.UpdateLLMProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateLLMProvider: failed to decode request", "orgName", orgName, "providerID", providerID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Info("UpdateLLMProvider: request decoded", "orgName", orgName, "providerID", providerID,
		"templateHandle", ptrToStringLog(req.TemplateHandle),
		"gatewayCount", len(req.Gateways))
	if req.Configuration != nil {
		log.Info("UpdateLLMProvider: config details",
			"configName", ptrToStringLog(req.Configuration.Name),
			"configVersion", ptrToStringLog(req.Configuration.Version))
	}

	// Convert spec request to model - create minimal provider with only updatable fields
	providerReq := &spec.CreateLLMProviderRequest{
		TemplateHandle: utils.GetOrDefault(req.TemplateHandle, ""),
		Description:    req.Description,
		Openapi:        req.Openapi,
		ModelList:      req.ModelList,
		Configuration:  utils.GetOrDefaultConfig(req.Configuration),
	}
	provider := utils.ConvertSpecToModelLLMProvider(providerReq, orgName)

	log.Info("UpdateLLMProvider: calling service layer", "orgName", orgName, "orgName", orgName, "providerID", providerID)

	var updated *models.LLMProvider

	// Check if gateways list is present (not nil), if so use UpdateAndSync
	if req.Gateways != nil {
		log.Info("UpdateLLMProvider: updating and syncing deployments to gateways", "orgName", orgName, "gatewayCount", len(req.Gateways))
		resp, err := c.providerService.UpdateAndSync(providerID, orgName, provider, req.Gateways, c.deploymentService)
		if err != nil {
			switch {
			case errors.Is(err, utils.ErrLLMProviderNotFound):
				log.Warn("UpdateLLMProvider: provider not found", "orgName", orgName, "providerID", providerID)
				utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
				return
			case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
				log.Error("UpdateLLMProvider: template not found", "orgName", orgName, "providerID", providerID, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
				return
			case errors.Is(err, utils.ErrInvalidInput):
				log.Error("UpdateLLMProvider: invalid input", "orgName", orgName, "providerID", providerID, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
				return
			default:
				log.Error("UpdateLLMProvider: failed to update provider", "orgName", orgName, "providerID", providerID, "error", err)
				utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update LLM provider")
				return
			}
		}
		updated = resp.Provider
		// Log deployment/undeployment results
		successDeployCount := 0
		failedDeployCount := 0
		for _, result := range resp.Deployments {
			if result.Success {
				successDeployCount++
			} else {
				failedDeployCount++
				log.Warn("UpdateLLMProvider: deployment failed for gateway", "orgName", orgName, "gatewayID", result.GatewayID, "error", result.Error)
			}
		}
		successUndeployCount := 0
		failedUndeployCount := 0
		for _, result := range resp.Undeployments {
			if result.Success {
				successUndeployCount++
			} else {
				failedUndeployCount++
				log.Warn("UpdateLLMProvider: undeployment failed for gateway", "orgName", orgName, "gatewayID", result.GatewayID, "error", result.Error)
			}
		}
		log.Info("UpdateLLMProvider: sync results",
			"orgName", orgName,
			"successfulDeployments", successDeployCount,
			"failedDeployments", failedDeployCount,
			"successfulUndeployments", successUndeployCount,
			"failedUndeployments", failedUndeployCount)
	} else {
		log.Info("UpdateLLMProvider: updating provider without deployment sync", "orgName", orgName)
		var err error
		updated, err = c.providerService.Update(providerID, orgName, provider)
		if err != nil {
			switch {
			case errors.Is(err, utils.ErrLLMProviderNotFound):
				log.Warn("UpdateLLMProvider: provider not found", "orgName", orgName, "providerID", providerID)
				utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
				return
			case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
				log.Error("UpdateLLMProvider: template not found", "orgName", orgName, "providerID", providerID, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
				return
			case errors.Is(err, utils.ErrInvalidInput):
				log.Error("UpdateLLMProvider: invalid input", "orgName", orgName, "providerID", providerID, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
				return
			default:
				log.Error("UpdateLLMProvider: failed to update provider", "orgName", orgName, "providerID", providerID, "error", err)
				utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update LLM provider")
				return
			}
		}
	}

	log.Info("UpdateLLMProvider: provider updated successfully", "orgName", orgName, "providerID", providerID, "providerUUID", updated.UUID)

	// Convert model to spec response
	response := utils.ConvertModelToSpecLLMProviderResponse(updated)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *llmController) DeleteLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	log.Info("DeleteLLMProvider: starting", "orgName", orgName, "providerID", providerID)

	log.Info("DeleteLLMProvider: calling service layer", "orgName", orgName, "orgName", orgName, "providerID", providerID)

	if err := c.providerService.Delete(providerID, orgName, c.deploymentService); err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("DeleteLLMProvider: provider not found", "orgName", orgName, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("DeleteLLMProvider: invalid provider id", "orgName", orgName, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider id")
			return
		default:
			log.Error("DeleteLLMProvider: failed to delete provider", "orgName", orgName, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete LLM provider")
			return
		}
	}

	log.Info("DeleteLLMProvider: provider deleted successfully", "orgName", orgName, "providerID", providerID)

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// ---- Proxy Handlers ----

func (c *llmController) CreateLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	projectName := r.PathValue(utils.PathParamProjName)

	// Resolve project name to UUID
	projectUUID, err := c.resolveProjectUUID(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			log.Error("CreateLLMProxy: project not found", "orgName", orgName, "projectName", projectName, "error", err)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("CreateLLMProxy: failed to resolve project", "orgName", orgName, "projectName", projectName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	var req spec.CreateLLMProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateLLMProxy: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to model with resolved project UUID
	proxy, err := utils.ConvertSpecToModelLLMProxy(&req, projectUUID)
	if err != nil {
		log.Error("CreateLLMProxy: failed to convert spec to model", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid UUID in request")
		return
	}

	created, err := c.proxyService.Create(orgName, "system", proxy)
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

	// Convert model to spec response
	response := utils.ConvertModelToSpecLLMProxyResponse(created)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *llmController) ListLLMProxies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	projectName := r.PathValue(utils.PathParamProjName)

	// Resolve project name to UUID
	projectUUID, err := c.resolveProjectUUID(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			log.Error("ListLLMProxies: project not found", "orgName", orgName, "projectName", projectName, "error", err)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("ListLLMProxies: failed to resolve project", "orgName", orgName, "projectName", projectName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
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

	proxies, totalCount, err := c.proxyService.List(orgName, &projectUUID, limit, offset)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("ListLLMProxies: failed to list proxies", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list LLM proxies")
		return
	}

	// Convert models to spec responses
	specProxies := make([]spec.LLMProxyResponse, len(proxies))
	for i, p := range proxies {
		specProxies[i] = utils.ConvertModelToSpecLLMProxyResponse(p)
	}

	resp := spec.LLMProxyListResponse{
		Proxies: specProxies,
		Total:   int32(totalCount),
		Limit:   int32(limit),
		Offset:  int32(offset),
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) ListLLMProxiesByProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

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

	proxies, totalCount, err := c.proxyService.ListByProvider(orgName, providerID, limit, offset)
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

	// Convert models to spec responses
	specProxies := make([]spec.LLMProxyResponse, len(proxies))
	for i, p := range proxies {
		specProxies[i] = utils.ConvertModelToSpecLLMProxyResponse(p)
	}

	resp := spec.LLMProxyListResponse{
		Proxies: specProxies,
		Total:   int32(totalCount),
		Limit:   int32(limit),
		Offset:  int32(offset),
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) GetLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	projectName := r.PathValue(utils.PathParamProjName)
	proxyID := r.PathValue("id")

	// Resolve project name to UUID (validates project exists)
	_, err := c.resolveProjectUUID(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			log.Error("GetLLMProxy: project not found", "orgName", orgName, "projectName", projectName, "error", err)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("GetLLMProxy: failed to resolve project", "orgName", orgName, "projectName", projectName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	proxy, err := c.proxyService.Get(orgName, proxyID)
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

	// Convert model to spec response
	response := utils.ConvertModelToSpecLLMProxyResponse(proxy)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *llmController) UpdateLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	projectName := r.PathValue(utils.PathParamProjName)
	proxyID := r.PathValue("id")

	// Resolve project name to UUID (validates project exists)
	projectUUID, err := c.resolveProjectUUID(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			log.Error("UpdateLLMProxy: project not found", "orgName", orgName, "projectName", projectName, "error", err)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("UpdateLLMProxy: failed to resolve project", "orgName", orgName, "projectName", projectName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	var req spec.UpdateLLMProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateLLMProxy: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to model - create minimal proxy with only updatable fields
	proxyReq := &spec.CreateLLMProxyRequest{
		ProviderUuid:  utils.GetOrDefault(req.ProviderUuid, ""),
		Description:   req.Description,
		Openapi:       req.Openapi,
		Configuration: utils.GetOrDefaultProxyConfig(req.Configuration),
	}
	proxy, err := utils.ConvertSpecToModelLLMProxy(proxyReq, projectUUID)
	if err != nil {
		log.Error("UpdateLLMProxy: failed to convert spec to model", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid UUID in request")
		return
	}

	updated, err := c.proxyService.Update(orgName, proxyID, proxy)
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

	// Convert model to spec response
	response := utils.ConvertModelToSpecLLMProxyResponse(updated)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *llmController) DeleteLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	projectName := r.PathValue(utils.PathParamProjName)
	proxyID := r.PathValue("id")

	// Resolve project name to UUID (validates project exists)
	_, err := c.resolveProjectUUID(ctx, orgName, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			log.Error("DeleteLLMProxy: project not found", "orgName", orgName, "projectName", projectName, "error", err)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("DeleteLLMProxy: failed to resolve project", "orgName", orgName, "projectName", projectName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if err := c.proxyService.Delete(orgName, proxyID); err != nil {
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

// UpdateLLMProviderCatalogStatus handles PUT /orgs/{orgName}/llm-providers/{id}/catalog
func (c *llmController) UpdateLLMProviderCatalogStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	// Decode request body
	var req spec.UpdateLLMProviderCatalogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateLLMProviderCatalogStatus: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Update catalog status via service
	provider, err := c.providerService.UpdateCatalogStatus(providerID, orgName, req.InCatalog)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider ID")
			return
		default:
			log.Error("UpdateLLMProviderCatalogStatus: failed to update catalog status", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update catalog status")
			return
		}
	}

	// Convert to response
	response := utils.ConvertModelToSpecLLMProviderResponse(provider)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}
