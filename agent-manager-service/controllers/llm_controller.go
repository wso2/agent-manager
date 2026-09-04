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

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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
	ListAvailableLLMPolicies(w http.ResponseWriter, r *http.Request)
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

	// Consumer handlers
	ListLLMProviderConsumers(w http.ResponseWriter, r *http.Request)
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
func (c *llmController) resolveProjectUUID(ctx context.Context, ouID, projectName string) (string, error) {
	project, err := c.ocClient.GetProject(ctx, ouID, projectName)
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
	ouID := middleware.OUIDFromRequest(r)

	var req spec.CreateLLMProviderTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateLLMProviderTemplate: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to model
	template := utils.ConvertSpecToModelLLMProviderTemplate(&req, ouID)

	created, err := c.templateService.Create(ouID, "system", template)
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
	ouID := middleware.OUIDFromRequest(r)

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

	templates, totalCount, err := c.templateService.List(ouID, limit, offset)
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
	ouID := middleware.OUIDFromRequest(r)
	templateID := r.PathValue(utils.PathParamTemplateId)

	template, err := c.templateService.Get(ouID, templateID)
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
	ouID := middleware.OUIDFromRequest(r)
	templateID := r.PathValue(utils.PathParamTemplateId)

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
	modelTemplate := utils.ConvertSpecToModelLLMProviderTemplate(template, ouID)

	updated, err := c.templateService.Update(ouID, templateID, modelTemplate)
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
	ouID := middleware.OUIDFromRequest(r)
	templateID := r.PathValue(utils.PathParamTemplateId)

	if err := c.templateService.Delete(ouID, templateID); err != nil {
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

// writeCreateLLMProviderError maps service errors from Create/CreateAndDeploy to HTTP responses.
// Returns true if an error was written (caller should return), false if err is nil.
func writeCreateLLMProviderError(w http.ResponseWriter, r *http.Request, ouID, templateHandle, providerName, providerHandle string, err error) {
	log := logger.GetLogger(r.Context())
	switch {
	case errors.Is(err, utils.ErrLLMProviderExists):
		log.Warn("CreateLLMProvider: provider already exists", "ouID", ouID, "providerName", providerName, "providerHandle", providerHandle)
		// The ID is auto-generated from the name and is what's actually unique (not the
		// display name, which can be reused) — renaming a provider never changes its ID,
		// so this collision can surface even when no provider currently shows this name.
		utils.WriteErrorResponse(w, http.StatusConflict, fmt.Sprintf(
			"A provider with the ID %q already exists (auto-generated from the name). Try a different name, or rename/delete the existing provider first.",
			providerHandle,
		))
	case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
		log.Error("CreateLLMProvider: template not found", "ouID", ouID, "templateHandle", templateHandle, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
	case errors.Is(err, utils.ErrInvalidInput):
		log.Error("CreateLLMProvider: invalid input", "ouID", ouID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
	default:
		log.Error("CreateLLMProvider: failed to create provider", "ouID", ouID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create LLM provider")
	}
}

func (c *llmController) CreateLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)

	log.Info("CreateLLMProvider: starting", "ouID", ouID)

	var req spec.CreateLLMProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateLLMProvider: failed to decode request", "ouID", ouID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	log.Info("CreateLLMProvider: request decoded", "ouID", ouID, "templateHandle", req.Template,
		"Name", req.Name,
		"Version", req.Version,
		"gatewayCount", len(req.Gateways))

	// Convert spec request to model
	provider := utils.ConvertSpecToModelLLMProvider(&req, ouID)
	log.Info("CreateLLMProvider: calling service layer", "ouID", ouID,
		"providerName", provider.Configuration.Name,
		"providerVersion", provider.Configuration.Version,
		"templateHandle", provider.TemplateHandle)

	var created *models.LLMProvider

	// Check if gateways list is present and not empty
	if len(req.Gateways) > 0 {
		log.Info("CreateLLMProvider: creating and deploying provider to gateways", "ouID", ouID, "gatewayCount", len(req.Gateways))
		resp, err := c.providerService.CreateAndDeploy(ctx, ouID, "system", provider, req.Gateways, c.deploymentService)
		if err != nil {
			writeCreateLLMProviderError(w, r, ouID, req.Template, provider.Configuration.Name, provider.Configuration.Handle, err)
			return
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
				log.Warn("CreateLLMProvider: deployment failed for gateway", "ouID", ouID, "gatewayID", result.GatewayID, "error", result.Error)
			}
		}
		log.Info("CreateLLMProvider: deployment results", "ouID", ouID, "successCount", successCount, "failedCount", failedCount, "totalRequested", len(req.Gateways))
	} else {
		log.Info("CreateLLMProvider: creating provider without deployment", "ouID", ouID)
		var err error
		created, err = c.providerService.Create(ctx, ouID, "system", provider)
		if err != nil {
			writeCreateLLMProviderError(w, r, ouID, req.Template, provider.Configuration.Name, provider.Configuration.Handle, err)
			return
		}
	}

	log.Info("CreateLLMProvider: provider created successfully", "ouID", ouID, "providerUUID", created.UUID, "providerName", created.Configuration.Name)

	// Convert model to spec response
	response := utils.ConvertModelToSpecLLMProviderResponse(created)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

// ListAvailableLLMPolicies handles GET /orgs/{orgName}/llm-providers/policies.
func (c *llmController) ListAvailableLLMPolicies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.URL.Query().Get("providerId")

	log.Info("ListAvailableLLMPolicies: starting", "ouID", ouID, "providerID", providerID)

	resp, err := c.providerService.ListAvailableLLMPolicies(ctx, ouID, providerID)
	if err != nil {
		if errors.Is(err, utils.ErrLLMProviderNotFound) {
			log.Warn("ListAvailableLLMPolicies: provider not found", "ouID", ouID, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		}
		log.Error("ListAvailableLLMPolicies: failed", "ouID", ouID, "providerID", providerID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list available LLM policies")
		return
	}

	log.Info("ListAvailableLLMPolicies: completed", "ouID", ouID, "count", resp.Count)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) ListLLMProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)

	log.Info("ListLLMProviders: starting", "ouID", ouID)

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

	log.Info("ListLLMProviders: calling service layer", "ouID", ouID, "limit", limit, "offset", offset)

	providers, totalCount, err := c.providerService.List(ouID, limit, offset)
	if err != nil {
		log.Error("ListLLMProviders: failed to list providers", "ouID", ouID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list LLM providers")
		return
	}

	log.Info("ListLLMProviders: providers retrieved", "ouID", ouID, "count", len(providers), "total", totalCount)

	// Convert models to spec responses
	specProviders := make([]spec.LLMProviderListItem, len(providers))
	for i, p := range providers {
		specProviders[i] = utils.ConvertModelToSpecLLMProviderListItemResponse(p)
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
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(utils.PathParamProviderId)

	log.Info("GetLLMProvider: starting", "ouID", ouID, "providerID", providerID)

	log.Info("GetLLMProvider: calling service layer", "ouID", ouID, "providerID", providerID)

	provider, err := c.providerService.Get(providerID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("GetLLMProvider: provider not found", "ouID", ouID, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("GetLLMProvider: invalid provider id", "ouID", ouID, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider id")
			return
		default:
			log.Error("GetLLMProvider: failed to get provider", "ouID", ouID, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get LLM provider")
			return
		}
	}

	log.Info("GetLLMProvider: provider retrieved", "ouID", ouID, "providerID", providerID, "providerUUID", provider.UUID)

	// Convert model to spec response
	response := utils.ConvertModelToSpecLLMProviderResponse(provider)

	gatewayMappings, err := c.providerService.GetProviderGatewayMapping(provider.UUID, ouID, c.deploymentService)
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
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(utils.PathParamProviderId)

	log.Info("UpdateLLMProvider: starting", "ouID", ouID, "providerID", providerID)

	var req spec.UpdateLLMProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateLLMProvider: failed to decode request", "ouID", ouID, "providerID", providerID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Info("UpdateLLMProvider: request decoded", "ouID", ouID, "providerID", providerID,
		"templateHandle", utils.GetOrDefault(req.Template, ""),
		"name", utils.GetOrDefault(req.Name, ""),
		"version", utils.GetOrDefault(req.Version, ""),
		"gatewayCount", len(req.Gateways))

	// Fetch the existing provider so that fields omitted from the request are preserved
	// (prevents CRIT-1: upstream overwritten with empty struct; CRIT-2: Version/Context reset to defaults).
	existing, err := c.providerService.Get(providerID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("UpdateLLMProvider: provider not found", "ouID", ouID, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		default:
			log.Error("UpdateLLMProvider: failed to fetch existing provider", "ouID", ouID, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update LLM provider")
			return
		}
	}

	// Resolve Version: use request value if provided, otherwise preserve the stored value.
	existingVersion := existing.Configuration.Version
	resolvedVersion := utils.GetOrDefault(req.Version, existingVersion)

	// Resolve Context: use request value if provided, otherwise preserve the stored value.
	existingContext := "/"
	if existing.Configuration.Context != nil {
		existingContext = *existing.Configuration.Context
	}
	resolvedContext := utils.GetOrDefault(req.Context, existingContext)

	// Convert spec request to model - create minimal provider with only updatable fields
	// For update, we need to construct a CreateLLMProviderRequest with the updated fields.
	// Id (the unique handle) is never changed on update — always taken from the existing record.
	providerReq := &spec.CreateLLMProviderRequest{
		Id:             existing.Artifact.Handle,
		Name:           utils.GetOrDefault(req.Name, existing.Configuration.Name),
		Description:    req.Description,
		Version:        resolvedVersion,
		Context:        resolvedContext,
		Template:       utils.GetOrDefault(req.Template, existing.Configuration.Template),
		Openapi:        req.Openapi,
		ModelProviders: req.ModelProviders,
	}

	// Add optional fields
	providerReq.AccessControl = req.AccessControl
	providerReq.Policies = req.Policies
	providerReq.RateLimiting = req.RateLimiting
	providerReq.Security = req.Security
	// Resilience is not part of CreateLLMProviderRequest's field set on the request struct
	// itself (it's applied to the model after conversion) so an omitted value in the update
	// request preserves the existing setting rather than wiping it.
	if req.Resilience != nil {
		providerReq.Resilience = req.Resilience
	}

	provider := utils.ConvertSpecToModelLLMProvider(providerReq, ouID)
	if req.Resilience == nil {
		provider.Configuration.Resilience = existing.Configuration.Resilience
	}

	// Preserve upstream directly from the stored model to avoid the spec converter
	// masking credentials with "***REDACTED***" (H-3). If the request supplies a new
	// upstream, convert that instead.
	if req.Upstream != nil {
		upstream := utils.ConvertSpecToModelUpstreamConfig(*req.Upstream)
		// If the converted upstream has no new auth value (i.e. the client echoed back the
		// redaction marker), carry the existing SecretRef forward so the stored encrypted
		// reference is not lost.
		if upstream.Main != nil && upstream.Main.Auth != nil && upstream.Main.Auth.Value == nil {
			if existing.Configuration.Upstream != nil &&
				existing.Configuration.Upstream.Main != nil &&
				existing.Configuration.Upstream.Main.Auth != nil {
				upstream.Main.Auth.SecretRef = existing.Configuration.Upstream.Main.Auth.SecretRef
			}
		}
		if upstream.Sandbox != nil && upstream.Sandbox.Auth != nil && upstream.Sandbox.Auth.Value == nil {
			if existing.Configuration.Upstream != nil &&
				existing.Configuration.Upstream.Sandbox != nil &&
				existing.Configuration.Upstream.Sandbox.Auth != nil {
				upstream.Sandbox.Auth.SecretRef = existing.Configuration.Upstream.Sandbox.Auth.SecretRef
			}
		}
		provider.Configuration.Upstream = &upstream
	} else if existing.Configuration.Upstream != nil {
		provider.Configuration.Upstream = existing.Configuration.Upstream
	}

	log.Info("UpdateLLMProvider: calling service layer", "ouID", ouID, "providerID", providerID)

	var updated *models.LLMProvider

	// Check if gateways list is present (not nil), if so use UpdateAndSync
	if req.Gateways != nil {
		log.Info("UpdateLLMProvider: updating and syncing deployments to gateways", "ouID", ouID, "gatewayCount", len(req.Gateways))
		resp, err := c.providerService.UpdateAndSync(ctx, providerID, ouID, provider, req.Gateways, c.deploymentService)
		if err != nil {
			switch {
			case errors.Is(err, utils.ErrLLMProviderNotFound):
				log.Warn("UpdateLLMProvider: provider not found", "ouID", ouID, "providerID", providerID)
				utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
				return
			case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
				log.Error("UpdateLLMProvider: template not found", "ouID", ouID, "providerID", providerID, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
				return
			case errors.Is(err, utils.ErrInvalidInput):
				log.Error("UpdateLLMProvider: invalid input", "ouID", ouID, "providerID", providerID, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
				return
			default:
				log.Error("UpdateLLMProvider: failed to update provider", "ouID", ouID, "providerID", providerID, "error", err)
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
				log.Warn("UpdateLLMProvider: deployment failed for gateway", "ouID", ouID, "gatewayID", result.GatewayID, "error", result.Error)
			}
		}
		successUndeployCount := 0
		failedUndeployCount := 0
		for _, result := range resp.Undeployments {
			if result.Success {
				successUndeployCount++
			} else {
				failedUndeployCount++
				log.Warn("UpdateLLMProvider: undeployment failed for gateway", "ouID", ouID, "gatewayID", result.GatewayID, "error", result.Error)
			}
		}
		log.Info("UpdateLLMProvider: sync results",
			"ouID", ouID,
			"successfulDeployments", successDeployCount,
			"failedDeployments", failedDeployCount,
			"successfulUndeployments", successUndeployCount,
			"failedUndeployments", failedUndeployCount)
	} else {
		log.Info("UpdateLLMProvider: updating provider without deployment sync", "ouID", ouID)
		var err error
		updated, err = c.providerService.Update(ctx, providerID, ouID, provider)
		if err != nil {
			switch {
			case errors.Is(err, utils.ErrLLMProviderNotFound):
				log.Warn("UpdateLLMProvider: provider not found", "ouID", ouID, "providerID", providerID)
				utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
				return
			case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
				log.Error("UpdateLLMProvider: template not found", "ouID", ouID, "providerID", providerID, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
				return
			case errors.Is(err, utils.ErrInvalidInput):
				log.Error("UpdateLLMProvider: invalid input", "ouID", ouID, "providerID", providerID, "error", err)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
				return
			default:
				log.Error("UpdateLLMProvider: failed to update provider", "ouID", ouID, "providerID", providerID, "error", err)
				utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update LLM provider")
				return
			}
		}
	}

	log.Info("UpdateLLMProvider: provider updated successfully", "ouID", ouID, "providerID", providerID, "providerUUID", updated.UUID)

	// Convert model to spec response
	response := utils.ConvertModelToSpecLLMProviderResponse(updated)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *llmController) DeleteLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(utils.PathParamProviderId)

	log.Info("DeleteLLMProvider: starting", "ouID", ouID, "providerID", providerID)

	log.Info("DeleteLLMProvider: calling service layer", "ouID", ouID, "providerID", providerID)

	if err := c.providerService.Delete(ctx, providerID, ouID, c.deploymentService); err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("DeleteLLMProvider: provider not found", "ouID", ouID, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("DeleteLLMProvider: invalid provider id", "ouID", ouID, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider id")
			return
		case errors.Is(err, utils.ErrLLMProviderHasProxies):
			log.Warn("DeleteLLMProvider: provider has associated proxies", "ouID", ouID, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusConflict, utils.ErrLLMProviderHasProxies.Error())
			return
		case errors.Is(err, utils.ErrLLMProviderUndeployFailed):
			log.Error("DeleteLLMProvider: undeployment failed", "ouID", ouID, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusConflict, utils.ErrLLMProviderUndeployFailed.Error())
			return
		case errors.Is(err, utils.ErrLLMProviderDeleteInProgress):
			log.Warn("DeleteLLMProvider: delete already in progress", "ouID", ouID, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusConflict, utils.ErrLLMProviderDeleteInProgress.Error())
			return
		default:
			log.Error("DeleteLLMProvider: failed to delete provider", "ouID", ouID, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete LLM provider")
			return
		}
	}

	log.Info("DeleteLLMProvider: provider deleted successfully", "ouID", ouID, "providerID", providerID)

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// ---- Proxy Handlers ----

func (c *llmController) CreateLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)

	// Resolve project name to UUID
	projectUUID, err := c.resolveProjectUUID(ctx, ouID, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			log.Error("CreateLLMProxy: project not found", "ouID", ouID, "projectName", projectName, "error", err)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("CreateLLMProxy: failed to resolve project", "ouID", ouID, "projectName", projectName, "error", err)
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

	created, err := c.proxyService.Create(ctx, ouID, "system", proxy)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyExists):
			utils.WriteErrorResponse(w, http.StatusConflict, "LLM proxy already exists")
			return
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced provider not found")
			return
		case errors.Is(err, utils.ErrLLMProviderBeingDeleted):
			utils.WriteErrorResponse(w, http.StatusConflict, utils.ErrLLMProviderBeingDeleted.Error())
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
	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)

	// Resolve project name to UUID
	projectUUID, err := c.resolveProjectUUID(ctx, ouID, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			log.Error("ListLLMProxies: project not found", "ouID", ouID, "projectName", projectName, "error", err)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("ListLLMProxies: failed to resolve project", "ouID", ouID, "projectName", projectName, "error", err)
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

	proxies, totalCount, err := c.proxyService.List(ouID, &projectUUID, limit, offset)
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
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(utils.PathParamProviderId)

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

	proxies, totalCount, err := c.proxyService.ListByProvider(ouID, providerID, limit, offset)
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
	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	proxyID := r.PathValue(utils.PathParamProxyId)

	// Resolve project name to UUID (validates project exists)
	_, err := c.resolveProjectUUID(ctx, ouID, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			log.Error("GetLLMProxy: project not found", "ouID", ouID, "projectName", projectName, "error", err)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("GetLLMProxy: failed to resolve project", "ouID", ouID, "projectName", projectName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	proxy, err := c.proxyService.Get(proxyID, ouID)
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
	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	proxyID := r.PathValue(utils.PathParamProxyId)

	// Resolve project name to UUID (validates project exists)
	projectUUID, err := c.resolveProjectUUID(ctx, ouID, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			log.Error("UpdateLLMProxy: project not found", "ouID", ouID, "projectName", projectName, "error", err)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("UpdateLLMProxy: failed to resolve project", "ouID", ouID, "projectName", projectName, "error", err)
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

	updated, err := c.proxyService.Update(proxyID, ouID, proxy)
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
	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	proxyID := r.PathValue(utils.PathParamProxyId)

	// Resolve project name to UUID (validates project exists)
	_, err := c.resolveProjectUUID(ctx, ouID, projectName)
	if err != nil {
		if errors.Is(err, utils.ErrProjectNotFound) {
			log.Error("DeleteLLMProxy: project not found", "ouID", ouID, "projectName", projectName, "error", err)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("DeleteLLMProxy: failed to resolve project", "ouID", ouID, "projectName", projectName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if err := c.proxyService.Delete(proxyID, ouID); err != nil {
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

// ListLLMProviderConsumers handles GET /orgs/{ouID}/llm-providers/{providerId}/consumers
func (c *llmController) ListLLMProviderConsumers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(utils.PathParamProviderId)

	consumers, err := c.providerService.ListConsumers(ctx, providerID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider id")
			return
		default:
			log.Error("ListLLMProviderConsumers: failed to list consumers", "ouID", ouID, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list provider consumers")
			return
		}
	}

	items := make([]spec.LLMProviderConsumerItem, len(consumers))
	for i, c := range consumers {
		items[i] = spec.LLMProviderConsumerItem{
			ProxyId:      c.ProxyID,
			ProxyName:    c.ProxyName,
			ProjectName:  c.ProjectName,
			ConsumerType: c.ConsumerType,
			ConsumerName: c.ConsumerName,
		}
	}

	resp := spec.LLMProviderConsumerListResponse{
		Consumers: items,
		Total:     int32(len(items)),
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// UpdateLLMProviderCatalogStatus handles PUT /orgs/{ouID}/llm-providers/{id}/catalog
func (c *llmController) UpdateLLMProviderCatalogStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(utils.PathParamProviderId)

	// Decode request body
	var req spec.UpdateLLMProviderCatalogRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateLLMProviderCatalogStatus: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Update catalog status via service
	provider, err := c.providerService.UpdateCatalogStatus(providerID, ouID, req.InCatalog)
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
