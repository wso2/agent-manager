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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtrack"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconv"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
)

// LLMController defines interface for LLM provider HTTP handlers
type LLMController interface {
	// Template handlers
	CreateLLMProviderTemplate(w http.ResponseWriter, r *http.Request)
	ListLLMProviderTemplates(w http.ResponseWriter, r *http.Request)
	GetLLMProviderTemplate(w http.ResponseWriter, r *http.Request)
	UpdateLLMProviderTemplate(w http.ResponseWriter, r *http.Request)
	DeleteLLMProviderTemplate(w http.ResponseWriter, r *http.Request)

	// Provider handlers, served as documents
	CreateLLMProvider(w http.ResponseWriter, r *http.Request)
	ListLLMProviders(w http.ResponseWriter, r *http.Request)
	ListAvailableLLMPolicies(w http.ResponseWriter, r *http.Request)
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

	// Consumer handlers
	ListLLMProviderConsumers(w http.ResponseWriter, r *http.Request)
}

type llmController struct {
	templateService   *gwsvc.LLMProviderTemplateService
	providerService   *gwsvc.LLMProviderService
	proxyService      *gwsvc.LLMProxyService
	deploymentService *gwsvc.LLMProviderDeploymentService
	// proxyDeploymentService is needed so a proxy deleted through the public API
	// also has its config reclaimed from the gateways still holding it.
	proxyDeploymentService *gwsvc.LLMProxyDeploymentService
	artifactRepo           gwrepo.ArtifactRepository
	ocClient               modkit.CoreAPI
}

// NewLLMController creates a new LLM controller
func NewLLMController(
	templateService *gwsvc.LLMProviderTemplateService,
	providerService *gwsvc.LLMProviderService,
	proxyService *gwsvc.LLMProxyService,
	deploymentService *gwsvc.LLMProviderDeploymentService,
	proxyDeploymentService *gwsvc.LLMProxyDeploymentService,
	artifactRepo gwrepo.ArtifactRepository,
	ocClient modkit.CoreAPI,
) LLMController {
	return &llmController{
		templateService:        templateService,
		providerService:        providerService,
		proxyService:           proxyService,
		deploymentService:      deploymentService,
		proxyDeploymentService: proxyDeploymentService,
		artifactRepo:           artifactRepo,
		ocClient:               ocClient,
	}
}

// resolveProjectUUID resolves project name to UUID using the platform repository
func (c *llmController) resolveProjectUUID(ctx context.Context, ouID, projectName string) (string, error) {
	project, err := gwsvc.LookupProject(ctx, c.ocClient, ouID, projectName)
	if err != nil {
		// Check if it's specifically a not-found error
		if errors.Is(err, gwerr.ErrProjectNotFound) {
			return "", gwerr.ErrProjectNotFound
		}
		// Return other errors (network, RPC, backend failures) as-is
		return "", fmt.Errorf("GetProject: %w", err)
	}
	if project == nil {
		return "", gwerr.ErrProjectNotFound
	}
	return project.ID, nil
}

// ---- Template Handlers ----

func (c *llmController) CreateLLMProviderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())

	var req gwspec.CreateLLMProviderTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateLLMProviderTemplate: failed to decode request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to model
	template := gwconv.ConvertSpecToModelLLMProviderTemplate(&req, ouID)

	created, err := c.templateService.Create(ouID, "system", template)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrSystemTemplateOverride):
			httpx.WriteErrorResponse(w, http.StatusConflict, "Cannot use handle of built-in template")
			return
		case errors.Is(err, gwerr.ErrLLMProviderTemplateExists):
			httpx.WriteErrorResponse(w, http.StatusConflict, "LLM provider template already exists")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("CreateLLMProviderTemplate: failed to create template", "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create LLM provider template")
			return
		}
	}

	// Convert model response to spec
	response := gwconv.ConvertModelToSpecLLMProviderTemplateResponse(created)
	httpx.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *llmController) ListLLMProviderTemplates(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())

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
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list LLM provider templates")
		return
	}

	// Convert models to spec responses
	specTemplates := make([]gwspec.LLMProviderTemplateResponse, len(templates))
	for i, t := range templates {
		specTemplates[i] = gwconv.ConvertModelToSpecLLMProviderTemplateResponse(t)
	}

	resp := gwspec.LLMProviderTemplateListResponse{
		Templates: specTemplates,
		Total:     int32(totalCount),
		Limit:     int32(limit),
		Offset:    int32(offset),
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) GetLLMProviderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	templateID := r.PathValue(gwutil.PathParamTemplateId)

	template, err := c.templateService.Get(ouID, templateID)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProviderTemplateNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider template not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid template id")
			return
		default:
			log.Error("GetLLMProviderTemplate: failed to get template", "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get LLM provider template")
			return
		}
	}

	// Convert model to spec response
	response := gwconv.ConvertModelToSpecLLMProviderTemplateResponse(template)
	httpx.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *llmController) UpdateLLMProviderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	templateID := r.PathValue(gwutil.PathParamTemplateId)

	var req gwspec.UpdateLLMProviderTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateLLMProviderTemplate: failed to decode request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to model - create minimal template with only updatable fields
	template := &gwspec.CreateLLMProviderTemplateRequest{
		Id:               templateID,
		Name:             gwconv.GetOrDefault(req.Name, ""),
		Description:      req.Description,
		Metadata:         req.Metadata,
		PromptTokens:     req.PromptTokens,
		CompletionTokens: req.CompletionTokens,
		TotalTokens:      req.TotalTokens,
		RemainingTokens:  req.RemainingTokens,
		RequestModel:     req.RequestModel,
		ResponseModel:    req.ResponseModel,
	}
	modelTemplate := gwconv.ConvertSpecToModelLLMProviderTemplate(template, ouID)

	updated, err := c.templateService.Update(ouID, templateID, modelTemplate)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrSystemTemplateImmutable):
			httpx.WriteErrorResponse(w, http.StatusForbidden, "System templates cannot be modified")
			return
		case errors.Is(err, gwerr.ErrLLMProviderTemplateNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider template not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("UpdateLLMProviderTemplate: failed to update template", "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update LLM provider template")
			return
		}
	}

	// Convert model to spec response
	response := gwconv.ConvertModelToSpecLLMProviderTemplateResponse(updated)
	httpx.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *llmController) DeleteLLMProviderTemplate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	templateID := r.PathValue(gwutil.PathParamTemplateId)

	if err := c.templateService.Delete(ouID, templateID); err != nil {
		switch {
		case errors.Is(err, gwerr.ErrSystemTemplateImmutable):
			httpx.WriteErrorResponse(w, http.StatusForbidden, "System templates cannot be deleted")
			return
		case errors.Is(err, gwerr.ErrLLMProviderTemplateNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider template not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid template id")
			return
		default:
			log.Error("DeleteLLMProviderTemplate: failed to delete template", "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete LLM provider template")
			return
		}
	}

	httpx.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// ---- Provider Handlers ----

// writeCreateLLMProviderError maps service errors from Create/CreateAndDeploy to HTTP responses.
// Returns true if an error was written (caller should return), false if err is nil.
func writeCreateLLMProviderError(w http.ResponseWriter, r *http.Request, ouID, templateHandle, providerName, providerHandle string, err error) {
	log := logger.GetLogger(r.Context())
	switch {
	case errors.Is(err, gwerr.ErrLLMProviderExists):
		log.Warn("CreateLLMProvider: provider already exists", "ouID", ouID, "providerName", providerName, "providerHandle", providerHandle)
		// The ID is auto-generated from the name and is what's actually unique (not the
		// display name, which can be reused) — renaming a provider never changes its ID,
		// so this collision can surface even when no provider currently shows this name.
		httpx.WriteErrorResponse(w, http.StatusConflict, fmt.Sprintf(
			"A provider with the ID %q already exists (auto-generated from the name). Try a different name, or rename/delete the existing provider first.",
			providerHandle,
		))
	case errors.Is(err, gwerr.ErrLLMProviderTemplateNotFound):
		log.Error("CreateLLMProvider: template not found", "ouID", ouID, "templateHandle", templateHandle, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
	case errors.Is(err, httpx.ErrInvalidInput):
		log.Error("CreateLLMProvider: invalid input", "ouID", ouID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
	default:
		log.Error("CreateLLMProvider: failed to create provider", "ouID", ouID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create LLM provider")
	}
}

// ListAvailableLLMPolicies handles GET /orgs/{orgName}/llm-providers/policies.
func (c *llmController) ListAvailableLLMPolicies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	providerID := r.URL.Query().Get("providerId")

	log.Info("ListAvailableLLMPolicies: starting", "ouID", ouID, "providerID", providerID)

	resp, err := c.providerService.ListAvailableLLMPolicies(ctx, ouID, providerID)
	if err != nil {
		if errors.Is(err, gwerr.ErrLLMProviderNotFound) {
			log.Warn("ListAvailableLLMPolicies: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		}
		log.Error("ListAvailableLLMPolicies: failed", "ouID", ouID, "providerID", providerID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list available LLM policies")
		return
	}

	log.Info("ListAvailableLLMPolicies: completed", "ouID", ouID, "count", resp.Count)
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}

// preserveOmittedProviderFields copies stored values into provider for optional fields the update request left out, so a PUT doesn't wipe them.
func preserveOmittedProviderFields(
	provider *gwmodels.LLMProvider,
	existing *gwmodels.LLMProvider,
	req *gwspec.UpdateLLMProviderRequest,
) {
	if provider == nil || existing == nil || req == nil {
		return
	}
	if req.Resilience == nil {
		provider.Configuration.Resilience = existing.Configuration.Resilience
	}
	if req.Security == nil {
		provider.Configuration.Security = existing.Configuration.Security
	}
	if req.AccessControl == nil {
		provider.Configuration.AccessControl = existing.Configuration.AccessControl
	}
	if req.RateLimiting == nil {
		provider.Configuration.RateLimiting = existing.Configuration.RateLimiting
	}
	if req.Policies == nil {
		provider.Configuration.Policies = existing.Configuration.Policies
	}
	if req.ModelProviders == nil {
		provider.ModelProviders = existing.ModelProviders
		provider.ModelList = existing.ModelList
	}
	if req.Openapi == nil {
		provider.OpenAPISpec = existing.OpenAPISpec
	}
	if req.Description == nil {
		provider.Description = existing.Description
	}
}

const (
	defaultListLimit = 20
	maxListLimit     = 100

	// maxDocumentBytes caps a posted document; generous because spec.openapi can be a whole spec.
	maxDocumentBytes = 8 << 20
)

// ListLLMProviders handles GET /m/ai-gateway/llm-providers.
func (c *llmController) ListLLMProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(ctx)

	limit, offset := listBounds(r)

	providers, total, err := c.providerService.List(ouID, limit, offset)
	if err != nil {
		log.Error("ListLLMProviders: failed to list providers", "ouID", ouID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list LLM providers")
		return
	}

	ids := make([]uuid.UUID, 0, len(providers))
	for _, p := range providers {
		ids = append(ids, p.UUID)
	}
	placement, err := c.providerService.ProviderPlacement(ctx, ouID, ids)
	if err != nil {
		log.Error("ListLLMProviders: failed to read the placement", "ouID", ouID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Error fetching deployed gateways")
		return
	}

	resp, err := gwconv.LLMProviderList(providers, placement, limit, offset, total)
	if err != nil {
		log.Error("ListLLMProviders: failed to render the documents", "ouID", ouID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to render LLM providers")
		return
	}

	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}

// GetLLMProvider handles GET /m/ai-gateway/llm-providers/{providerId}.
func (c *llmController) GetLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ouID := modkit.OrgID(ctx)
	providerID := r.PathValue(gwutil.PathParamProviderId)

	provider, err := c.providerService.Get(providerID, ouID)
	if err != nil {
		writeGetLLMProviderError(w, ctx, ouID, providerID, err)
		return
	}

	doc, ok := c.renderLLMProvider(w, r, "GetLLMProvider", provider)
	if !ok {
		return
	}

	modkit.AuditResource(ctx, "llm-provider", provider.UUID.String(), provider.Artifact.Handle)
	httpx.WriteSuccessResponse(w, http.StatusOK, doc)
}

// CreateLLMProvider handles POST /m/ai-gateway/llm-providers.
func (c *llmController) CreateLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(ctx)

	var doc gwspec.LlmProviderDocument
	if err := json.NewDecoder(io.LimitReader(r.Body, maxDocumentBytes)).Decode(&doc); err != nil {
		log.Error("CreateLLMProvider: failed to decode the document", "ouID", ouID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// No runtime schema validation, so the converter checks apiVersion, kind and metadata.
	req, err := gwconv.LLMProviderFromDocument(doc)
	if err != nil {
		writeDocumentError(w, ctx, "CreateLLMProvider", ouID, err)
		return
	}

	gwtrack.Set(ctx, "from_template", req.Template != "")
	if req.Template != "" {
		gwtrack.Set(ctx, "provider_type", req.Template)
	}

	provider := gwconv.ConvertSpecToModelLLMProvider(&req, ouID)

	// spec.gateways deploys the provider as part of the create.
	var created *gwmodels.LLMProvider
	if len(req.Gateways) > 0 {
		resp, err := c.providerService.CreateAndDeploy(ctx, ouID, "system", provider, req.Gateways, c.deploymentService)
		if err != nil {
			writeCreateLLMProviderError(w, r, ouID, req.Template, provider.Configuration.Name, provider.Configuration.Handle, err)
			return
		}
		created = resp.Provider
		for _, result := range resp.Deployments {
			if !result.Success {
				log.Warn("CreateLLMProvider: deployment failed for gateway",
					"ouID", ouID, "gatewayID", result.GatewayID, "error", result.Error)
			}
		}
	} else {
		created, err = c.providerService.Create(ctx, ouID, "system", provider)
		if err != nil {
			writeCreateLLMProviderError(w, r, ouID, req.Template, provider.Configuration.Name, provider.Configuration.Handle, err)
			return
		}
	}

	// spec.inCatalog publishes the provider to the catalog.
	created, err = c.applyCatalog(ctx, "CreateLLMProvider", ouID, created, doc.Spec.InCatalog)
	if err != nil {
		modkit.AuditResource(ctx, "llm-provider", created.UUID.String(), created.Artifact.Handle)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, catalogFailedMessage)
		return
	}

	out, ok := c.renderLLMProvider(w, r, "CreateLLMProvider", created)
	if !ok {
		return
	}

	modkit.AuditResource(ctx, "llm-provider", created.UUID.String(), created.Artifact.Handle)
	httpx.WriteSuccessResponse(w, http.StatusCreated, out)
}

// UpdateLLMProvider handles PUT /m/ai-gateway/llm-providers/{providerId}; omitted spec fields keep their stored value.
func (c *llmController) UpdateLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(ctx)
	providerID := r.PathValue(gwutil.PathParamProviderId)

	body, err := io.ReadAll(io.LimitReader(r.Body, maxDocumentBytes))
	if err != nil {
		log.Error("UpdateLLMProvider: failed to read the body", "ouID", ouID, "providerID", providerID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	doc, update, err := gwconv.LLMProviderUpdateFromDocument(body)
	if err != nil {
		writeDocumentError(w, ctx, "UpdateLLMProvider", ouID, err)
		return
	}
	req := &update

	log.Info("UpdateLLMProvider: starting", "ouID", ouID, "providerID", providerID,
		"templateHandle", gwconv.GetOrDefault(req.Template, ""),
		"name", gwconv.GetOrDefault(req.Name, ""),
		"version", gwconv.GetOrDefault(req.Version, ""),
		"gatewayCount", len(req.Gateways))

	// Fetch the existing provider so that fields omitted from the request are preserved
	// (prevents CRIT-1: upstream overwritten with empty struct; CRIT-2: Version/Context reset to defaults).
	existing, err := c.providerService.Get(providerID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			log.Warn("UpdateLLMProvider: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		default:
			log.Error("UpdateLLMProvider: failed to fetch existing provider", "ouID", ouID, "providerID", providerID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update LLM provider")
			return
		}
	}
	// The document must name the provider it updates; the path may be its handle or UUID.
	if doc.Metadata.Name != existing.Artifact.Handle {
		log.Warn("UpdateLLMProvider: metadata.name does not match the provider", "ouID", ouID,
			"providerID", providerID, "metadataName", doc.Metadata.Name)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "metadata.name does not match the LLM provider in the path")
		return
	}

	// Resolve Version: use request value if provided, otherwise preserve the stored value.
	existingVersion := existing.Configuration.Version
	resolvedVersion := gwconv.GetOrDefault(req.Version, existingVersion)

	// Resolve Context: use request value if provided, otherwise preserve the stored value.
	existingContext := "/"
	if existing.Configuration.Context != nil {
		existingContext = *existing.Configuration.Context
	}
	resolvedContext := gwconv.GetOrDefault(req.Context, existingContext)

	// Convert spec request to model - create minimal provider with only updatable fields
	// For update, we need to construct a CreateLLMProviderRequest with the updated fields.
	// Id (the unique handle) is never changed on update — always taken from the existing record.
	providerReq := &gwspec.CreateLLMProviderRequest{
		Id:             existing.Artifact.Handle,
		Name:           gwconv.GetOrDefault(req.Name, existing.Configuration.Name),
		Description:    req.Description,
		Version:        resolvedVersion,
		Context:        resolvedContext,
		Template:       gwconv.GetOrDefault(req.Template, existing.Configuration.Template),
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

	provider := gwconv.ConvertSpecToModelLLMProvider(providerReq, ouID)
	preserveOmittedProviderFields(provider, existing, req)

	// Preserve upstream directly from the stored model to avoid the spec converter
	// masking credentials with "***REDACTED***" (H-3). If the request supplies a new
	// upstream, convert that instead.
	if req.Upstream != nil {
		upstream := gwconv.ConvertSpecToModelUpstreamConfig(*req.Upstream)
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

	var updated *gwmodels.LLMProvider

	// Check if gateways list is present (not nil), if so use UpdateAndSync
	if req.Gateways != nil {
		log.Info("UpdateLLMProvider: updating and syncing deployments to gateways", "ouID", ouID, "gatewayCount", len(req.Gateways))
		resp, err := c.providerService.UpdateAndSync(ctx, providerID, ouID, provider, req.Gateways, c.deploymentService)
		if err != nil {
			switch {
			case errors.Is(err, gwerr.ErrLLMProviderExists):
				log.Warn("UpdateLLMProvider: provider name and version already exist", "ouID", ouID, "providerID", providerID)
				httpx.WriteErrorResponse(w, http.StatusConflict, "Another LLM provider already uses this name and version")
				return
			case errors.Is(err, gwerr.ErrLLMProviderNotFound):
				log.Warn("UpdateLLMProvider: provider not found", "ouID", ouID, "providerID", providerID)
				httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
				return
			case errors.Is(err, gwerr.ErrLLMProviderTemplateNotFound):
				log.Error("UpdateLLMProvider: template not found", "ouID", ouID, "providerID", providerID, "error", err)
				httpx.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
				return
			case errors.Is(err, httpx.ErrInvalidInput):
				log.Error("UpdateLLMProvider: invalid input", "ouID", ouID, "providerID", providerID, "error", err)
				httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
				return
			default:
				log.Error("UpdateLLMProvider: failed to update provider", "ouID", ouID, "providerID", providerID, "error", err)
				httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update LLM provider")
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
			case errors.Is(err, gwerr.ErrLLMProviderExists):
				log.Warn("UpdateLLMProvider: provider name and version already exist", "ouID", ouID, "providerID", providerID)
				httpx.WriteErrorResponse(w, http.StatusConflict, "Another LLM provider already uses this name and version")
				return
			case errors.Is(err, gwerr.ErrLLMProviderNotFound):
				log.Warn("UpdateLLMProvider: provider not found", "ouID", ouID, "providerID", providerID)
				httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
				return
			case errors.Is(err, gwerr.ErrLLMProviderTemplateNotFound):
				log.Error("UpdateLLMProvider: template not found", "ouID", ouID, "providerID", providerID, "error", err)
				httpx.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
				return
			case errors.Is(err, httpx.ErrInvalidInput):
				log.Error("UpdateLLMProvider: invalid input", "ouID", ouID, "providerID", providerID, "error", err)
				httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
				return
			default:
				log.Error("UpdateLLMProvider: failed to update provider", "ouID", ouID, "providerID", providerID, "error", err)
				httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update LLM provider")
				return
			}
		}
	}

	log.Info("UpdateLLMProvider: provider updated successfully", "ouID", ouID, "providerID", providerID, "providerUUID", updated.UUID)

	// Each dependent proxy holds its own copy of the provider's api-key header, so a
	// security change has to reach them or agents keep authenticating with the old name.
	// Gated on an actual header diff: every proxy provisioned before this feature shipped
	// still carries the old default, so an unconditional sync would redeploy the whole
	// fleet on the next unrelated edit (name, description, policy) to any such provider.
	// Detached and best-effort, the way MCP proxy edits refresh their dependents: the
	// provider update has already succeeded and must not be failed by this.
	if gwsvc.ProviderAuthHeadersChanged(existing, updated) {
		// Detached from the request: the response is about to be written, so the request
		// context is cancelled before the sync gets far. It keeps the request's values
		// (trace and tenant metadata) so the minting it may do is still attributable.
		syncCtx := context.WithoutCancel(r.Context())
		// A copy, since applyCatalog below reassigns updated.
		syncProvider := updated
		go func() {
			if err := c.providerService.SyncDependentProxyAuthHeaders(
				syncCtx, syncProvider, ouID, c.proxyService, c.proxyDeploymentService,
			); err != nil {
				log.Error("UpdateLLMProvider: failed to sync dependent proxy auth headers",
					"ouID", ouID, "providerID", providerID, "error", err)
			}
		}()
	}

	updated, err = c.applyCatalog(ctx, "UpdateLLMProvider", ouID, updated, doc.Spec.InCatalog)
	if err != nil {
		modkit.AuditResource(ctx, "llm-provider", updated.UUID.String(), updated.Artifact.Handle)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, catalogFailedMessage)
		return
	}

	out, ok := c.renderLLMProvider(w, r, "UpdateLLMProvider", updated)
	if !ok {
		return
	}

	modkit.AuditResource(ctx, "llm-provider", updated.UUID.String(), updated.Artifact.Handle)
	httpx.WriteSuccessResponse(w, http.StatusOK, out)
}

// DeleteLLMProvider handles DELETE /m/ai-gateway/llm-providers/{providerId}.
func (c *llmController) DeleteLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(ctx)
	providerID := r.PathValue(gwutil.PathParamProviderId)

	// Read first so the audit record can name what was deleted.
	if provider, err := c.providerService.Get(providerID, ouID); err == nil {
		modkit.AuditResource(ctx, "llm-provider", provider.UUID.String(), provider.Artifact.Handle)
	}

	if err := c.providerService.Delete(ctx, providerID, ouID, c.deploymentService); err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			log.Warn("DeleteLLMProvider: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
		case errors.Is(err, httpx.ErrInvalidInput):
			log.Error("DeleteLLMProvider: invalid provider id", "ouID", ouID, "providerID", providerID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider id")
		case errors.Is(err, gwerr.ErrLLMProviderHasProxies):
			log.Warn("DeleteLLMProvider: provider has associated proxies", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusConflict, gwerr.ErrLLMProviderHasProxies.Error())
		case errors.Is(err, gwerr.ErrLLMProviderUsedByBudgets):
			log.Warn("DeleteLLMProvider: budgets use the provider", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusConflict, err.Error())
		case errors.Is(err, gwerr.ErrLLMProviderUndeployFailed):
			log.Error("DeleteLLMProvider: undeployment failed", "ouID", ouID, "providerID", providerID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusConflict, gwerr.ErrLLMProviderUndeployFailed.Error())
		case errors.Is(err, gwerr.ErrLLMProviderDeleteInProgress):
			log.Warn("DeleteLLMProvider: delete already in progress", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusConflict, gwerr.ErrLLMProviderDeleteInProgress.Error())
		default:
			log.Error("DeleteLLMProvider: failed to delete provider", "ouID", ouID, "providerID", providerID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete LLM provider")
		}
		return
	}

	httpx.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// renderLLMProvider builds the provider's response document, including the gateways it is deployed to; on failure it writes a 500 and returns false.
func (c *llmController) renderLLMProvider(
	w http.ResponseWriter, r *http.Request, op string, provider *gwmodels.LLMProvider,
) (gwspec.LlmProviderDocument, bool) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(ctx)

	placement, err := c.providerService.ProviderPlacement(ctx, ouID, []uuid.UUID{provider.UUID})
	if err != nil {
		log.Error(op+": failed to read the placement", "ouID", ouID, "providerID", provider.UUID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Error fetching deployed gateways")
		return gwspec.LlmProviderDocument{}, false
	}

	doc, err := gwconv.LLMProviderDocument(provider, placement[provider.UUID])
	if err != nil {
		log.Error(op+": failed to render the document", "ouID", ouID, "providerID", provider.UUID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to render LLM provider")
		return gwspec.LlmProviderDocument{}, false
	}
	return doc, true
}

// catalogFailedMessage tells the caller the provider was written, so a retried PUT completes it.
const catalogFailedMessage = "The LLM provider was saved, but setting its catalog status failed; retry the update"

// applyCatalog reconciles spec.inCatalog after the provider is written.
func (c *llmController) applyCatalog(
	ctx context.Context, op, ouID string, provider *gwmodels.LLMProvider, declared *bool,
) (*gwmodels.LLMProvider, error) {
	if declared == nil || provider == nil || provider.Artifact == nil || provider.Artifact.InCatalog == *declared {
		return provider, nil
	}
	if _, err := c.providerService.UpdateCatalogStatus(provider.UUID.String(), ouID, *declared); err != nil {
		logger.GetLogger(ctx).Error(op+": failed to set the catalog status",
			"ouID", ouID, "providerID", provider.UUID, "inCatalog", *declared, "error", err)
		return provider, err
	}
	modkit.AuditDetail(ctx, "inCatalog", *declared)
	// Keep the written provider: the catalog read-back lacks fields such as the parsed model providers.
	provider.InCatalog = *declared
	provider.Artifact.InCatalog = *declared
	return provider, nil
}

// writeDocumentError maps a refused document to 400 and anything else to 500.
func writeDocumentError(w http.ResponseWriter, ctx context.Context, op, ouID string, err error) {
	log := logger.GetLogger(ctx)
	if errors.Is(err, gwconv.ErrDocument) {
		log.Warn(op+": the document was refused", "ouID", ouID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}
	log.Error(op+": failed to read the document", "ouID", ouID, "error", err)
	httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to read the document")
}

// writeGetLLMProviderError maps a provider read failure to its HTTP status.
func writeGetLLMProviderError(w http.ResponseWriter, ctx context.Context, ouID, providerID string, err error) {
	log := logger.GetLogger(ctx)
	switch {
	case errors.Is(err, gwerr.ErrLLMProviderNotFound):
		log.Warn("GetLLMProvider: provider not found", "ouID", ouID, "providerID", providerID)
		httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
	case errors.Is(err, httpx.ErrInvalidInput):
		log.Error("GetLLMProvider: invalid provider id", "ouID", ouID, "providerID", providerID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider id")
	default:
		log.Error("GetLLMProvider: failed to get provider", "ouID", ouID, "providerID", providerID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get LLM provider")
	}
}

// listBounds reads limit and offset, clamped to what the spec declares.
func listBounds(r *http.Request) (limit, offset int) {
	limit = getIntQueryParam(r, "limit", defaultListLimit)
	offset = getIntQueryParam(r, "offset", 0)
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

// ---- Proxy Handlers ----

func (c *llmController) CreateLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)

	// Resolve project name to UUID
	projectUUID, err := c.resolveProjectUUID(ctx, ouID, projectName)
	if err != nil {
		if errors.Is(err, gwerr.ErrProjectNotFound) {
			log.Error("CreateLLMProxy: project not found", "ouID", ouID, "projectName", projectName, "error", err)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("CreateLLMProxy: failed to resolve project", "ouID", ouID, "projectName", projectName, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	var req gwspec.CreateLLMProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateLLMProxy: failed to decode request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to model with resolved project UUID
	proxy, err := gwconv.ConvertSpecToModelLLMProxy(&req, projectUUID)
	if err != nil {
		log.Error("CreateLLMProxy: failed to convert spec to model", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid UUID in request")
		return
	}

	created, err := c.proxyService.Create(ctx, ouID, "system", proxy)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProxyExists):
			httpx.WriteErrorResponse(w, http.StatusConflict, "LLM proxy already exists")
			return
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Referenced provider not found")
			return
		case errors.Is(err, gwerr.ErrLLMProviderBeingDeleted):
			httpx.WriteErrorResponse(w, http.StatusConflict, gwerr.ErrLLMProviderBeingDeleted.Error())
			return
		case errors.Is(err, gwerr.ErrProjectNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("CreateLLMProxy: failed to create proxy", "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create LLM proxy")
			return
		}
	}

	// Convert model to spec response
	response := gwconv.ConvertModelToSpecLLMProxyResponse(created)
	httpx.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *llmController) ListLLMProxies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)

	// Resolve project name to UUID
	projectUUID, err := c.resolveProjectUUID(ctx, ouID, projectName)
	if err != nil {
		if errors.Is(err, gwerr.ErrProjectNotFound) {
			log.Error("ListLLMProxies: project not found", "ouID", ouID, "projectName", projectName, "error", err)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("ListLLMProxies: failed to resolve project", "ouID", ouID, "projectName", projectName, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
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
		if errors.Is(err, gwerr.ErrProjectNotFound) {
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("ListLLMProxies: failed to list proxies", "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list LLM proxies")
		return
	}

	// Convert models to spec responses
	specProxies := make([]gwspec.LLMProxyResponse, len(proxies))
	for i, p := range proxies {
		specProxies[i] = gwconv.ConvertModelToSpecLLMProxyResponse(p)
	}

	resp := gwspec.LLMProxyListResponse{
		Proxies: specProxies,
		Total:   int32(totalCount),
		Limit:   int32(limit),
		Offset:  int32(offset),
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) ListLLMProxiesByProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	providerID := r.PathValue(gwutil.PathParamProviderId)

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
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider id")
			return
		default:
			log.Error("ListLLMProxiesByProvider: failed to list proxies", "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list LLM proxies")
			return
		}
	}

	// Convert models to spec responses
	specProxies := make([]gwspec.LLMProxyResponse, len(proxies))
	for i, p := range proxies {
		specProxies[i] = gwconv.ConvertModelToSpecLLMProxyResponse(p)
	}

	resp := gwspec.LLMProxyListResponse{
		Proxies: specProxies,
		Total:   int32(totalCount),
		Limit:   int32(limit),
		Offset:  int32(offset),
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmController) GetLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	proxyID := r.PathValue(gwutil.PathParamProxyId)

	// Resolve project name to UUID (validates project exists)
	_, err := c.resolveProjectUUID(ctx, ouID, projectName)
	if err != nil {
		if errors.Is(err, gwerr.ErrProjectNotFound) {
			log.Error("GetLLMProxy: project not found", "ouID", ouID, "projectName", projectName, "error", err)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("GetLLMProxy: failed to resolve project", "ouID", ouID, "projectName", projectName, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	proxy, err := c.proxyService.Get(proxyID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProxyNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid proxy id")
			return
		default:
			log.Error("GetLLMProxy: failed to get proxy", "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get LLM proxy")
			return
		}
	}

	// Convert model to spec response
	response := gwconv.ConvertModelToSpecLLMProxyResponse(proxy)
	httpx.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *llmController) UpdateLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	proxyID := r.PathValue(gwutil.PathParamProxyId)

	// Resolve project name to UUID (validates project exists)
	projectUUID, err := c.resolveProjectUUID(ctx, ouID, projectName)
	if err != nil {
		if errors.Is(err, gwerr.ErrProjectNotFound) {
			log.Error("UpdateLLMProxy: project not found", "ouID", ouID, "projectName", projectName, "error", err)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("UpdateLLMProxy: failed to resolve project", "ouID", ouID, "projectName", projectName, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if err := c.proxyDeploymentService.RequireUserManaged(ctx, proxyID, ouID); err != nil {
		writeUserManagedProxyError(w, log, "UpdateLLMProxy", proxyID, err)
		return
	}

	var req gwspec.UpdateLLMProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateLLMProxy: failed to decode request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to model - create minimal proxy with only updatable fields
	proxyReq := &gwspec.CreateLLMProxyRequest{
		ProviderUuid:  gwconv.GetOrDefault(req.ProviderUuid, ""),
		Description:   req.Description,
		Openapi:       req.Openapi,
		Configuration: gwconv.GetOrDefaultProxyConfig(req.Configuration),
	}
	proxy, err := gwconv.ConvertSpecToModelLLMProxy(proxyReq, projectUUID)
	if err != nil {
		log.Error("UpdateLLMProxy: failed to convert spec to model", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid UUID in request")
		return
	}

	updated, err := c.proxyService.Update(proxyID, ouID, proxy)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProxyNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Referenced provider not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("UpdateLLMProxy: failed to update proxy", "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update LLM proxy")
			return
		}
	}

	// Convert model to spec response
	response := gwconv.ConvertModelToSpecLLMProxyResponse(updated)
	httpx.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *llmController) DeleteLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	proxyID := r.PathValue(gwutil.PathParamProxyId)

	// Resolve project name to UUID (validates project exists)
	projectUUID, err := c.resolveProjectUUID(ctx, ouID, projectName)
	if err != nil {
		if errors.Is(err, gwerr.ErrProjectNotFound) {
			log.Error("DeleteLLMProxy: project not found", "ouID", ouID, "projectName", projectName, "error", err)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		log.Error("DeleteLLMProxy: failed to resolve project", "ouID", ouID, "projectName", projectName, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if err := c.proxyDeploymentService.RequireUserManaged(ctx, proxyID, ouID); err != nil {
		writeUserManagedProxyError(w, log, "DeleteLLMProxy", proxyID, err)
		return
	}

	if err := c.proxyService.DeleteInProject(ctx, proxyID, ouID, projectUUID, c.proxyDeploymentService); err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProxyNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid proxy id")
			return
		default:
			log.Error("DeleteLLMProxy: failed to delete proxy", "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete LLM proxy")
			return
		}
	}

	httpx.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// ListLLMProviderConsumers handles GET /orgs/{ouID}/llm-providers/{providerId}/consumers
func (c *llmController) ListLLMProviderConsumers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	providerID := r.PathValue(gwutil.PathParamProviderId)

	consumers, err := c.providerService.ListConsumers(ctx, providerID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid provider id")
			return
		default:
			log.Error("ListLLMProviderConsumers: failed to list consumers", "ouID", ouID, "providerID", providerID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list provider consumers")
			return
		}
	}

	items := make([]gwspec.LLMProviderConsumerItem, len(consumers))
	for i, c := range consumers {
		items[i] = gwspec.LLMProviderConsumerItem{
			ProxyId:      c.ProxyID,
			ProxyName:    c.ProxyName,
			ProjectName:  c.ProjectName,
			ConsumerType: c.ConsumerType,
			ConsumerName: c.ConsumerName,
		}
	}

	resp := gwspec.LLMProviderConsumerListResponse{
		Consumers: items,
		Total:     int32(len(items)),
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}
