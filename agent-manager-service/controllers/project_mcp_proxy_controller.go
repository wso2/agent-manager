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
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// ProjectMCPProxyController defines handlers for project-scoped MCP proxies.
type ProjectMCPProxyController interface {
	UpsertProjectMCPProxy(w http.ResponseWriter, r *http.Request)
	ListProjectMCPProxies(w http.ResponseWriter, r *http.Request)
	GetProjectMCPProxy(w http.ResponseWriter, r *http.Request)
	DeleteProjectMCPProxy(w http.ResponseWriter, r *http.Request)
	PutProjectMCPProxyEnvironment(w http.ResponseWriter, r *http.Request)
	GetProjectMCPProxyEnvironment(w http.ResponseWriter, r *http.Request)
	DeleteProjectMCPProxyEnvironment(w http.ResponseWriter, r *http.Request)
	RefreshProjectMCPProxyEnvironmentCapabilities(w http.ResponseWriter, r *http.Request)
	ListProjectMCPProxyEnvironmentAPIKeys(w http.ResponseWriter, r *http.Request)
	CreateProjectMCPProxyEnvironmentAPIKey(w http.ResponseWriter, r *http.Request)
	IssueProjectMCPProxyEnvironmentTestAPIKey(w http.ResponseWriter, r *http.Request)
	RotateProjectMCPProxyEnvironmentAPIKey(w http.ResponseWriter, r *http.Request)
	RevokeProjectMCPProxyEnvironmentAPIKey(w http.ResponseWriter, r *http.Request)
	ListProjectMCPProxyScopes(w http.ResponseWriter, r *http.Request)
	CreateProjectMCPProxyScope(w http.ResponseWriter, r *http.Request)
	UpdateProjectMCPProxyScope(w http.ResponseWriter, r *http.Request)
	DeleteProjectMCPProxyScope(w http.ResponseWriter, r *http.Request)
}

// pathParamEnvName is the environment segment of the project MCP proxy binding routes.
const pathParamEnvName = "envName"

type projectMCPProxyController struct {
	service  *services.ProjectMCPProxyService
	scopes   services.MCPProxyScopeService
	ocClient client.OpenChoreoClient
}

// NewProjectMCPProxyController creates the project MCP proxy controller.
func NewProjectMCPProxyController(
	service *services.ProjectMCPProxyService,
	scopes services.MCPProxyScopeService,
	ocClient client.OpenChoreoClient,
) ProjectMCPProxyController {
	return &projectMCPProxyController{service: service, scopes: scopes, ocClient: ocClient}
}

// resolveProject maps {projName} to the OpenChoreo project. On failure it has written the
// response and returns ok=false.
func (c *projectMCPProxyController) resolveProject(w http.ResponseWriter, r *http.Request, ouID string) (*models.ProjectResponse, bool) {
	project, deleted, ok := c.resolveProjectOrDeleted(w, r, ouID)
	if ok && deleted {
		writeProjectNotFound(w)
		return nil, false
	}
	return project, ok
}

// resolveProjectOrDeleted is resolveProject for the routes that read or remove what a project
// published. A project OpenChoreo does not have is reported as deleted=true (project nil)
// instead of a 404, so those routes can reach the proxies a deleted project left, stored
// under its name in the caller's org. On failure it has written the response and returns ok=false.
func (c *projectMCPProxyController) resolveProjectOrDeleted(
	w http.ResponseWriter, r *http.Request, ouID string,
) (project *models.ProjectResponse, deleted, ok bool) {
	log := logger.GetLogger(r.Context())
	projName := r.PathValue(utils.PathParamProjName)
	project, err := c.ocClient.GetProject(r.Context(), ouID, projName)
	if err != nil && !errors.Is(err, utils.ErrProjectNotFound) {
		log.Error("project MCP proxy: failed to resolve project", "ouID", ouID, "project", projName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to resolve project")
		return nil, false, false
	}
	if err != nil || project == nil {
		return nil, true, true
	}
	return project, false, true
}

// writeProjectNotFound is the 404 for a project OpenChoreo does not have. A route that falls
// back to a deleted project's stored proxies answers it when nothing is stored for the request.
func writeProjectNotFound(w http.ResponseWriter) {
	utils.WriteErrorResponseWithReason(w, http.StatusNotFound, "Project not found", "", utils.ErrCodeProjectNotFound)
}

// UpsertProjectMCPProxy handles PUT /orgs/{orgName}/projects/{projName}/mcp-proxies/{proxyId}.
func (c *projectMCPProxyController) UpsertProjectMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	handle := r.PathValue(utils.PathParamProxyId)

	var req models.ProjectMCPProxyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpsertProjectMCPProxy: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	project, ok := c.resolveProject(w, r, ouID)
	if !ok {
		return
	}

	// The route gates on update; an insert additionally needs the create permission.
	canCreate := jwtassertion.HoldsAnyScope(ctx, rbac.MCPServerCreate)
	// If-None-Match: * makes the PUT create-only, so a registration cannot replace an existing proxy.
	createOnly := strings.TrimSpace(r.Header.Get("If-None-Match")) == "*"
	resp, created, err := c.service.UpsertWithCreateCheck(ctx, ouID, project.Name, project.UUID, handle, req,
		canCreate, createOnly)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrMCPProxyExists):
			utils.WriteErrorResponseWithReason(w, http.StatusPreconditionFailed,
				"MCP proxy already exists", err.Error(), utils.ErrCodeMCPProxyAlreadyExists)
		case errors.Is(err, utils.ErrMCPProxyCreateNotPermitted):
			utils.WriteErrorResponse(w, http.StatusForbidden, "Creating MCP proxies is not permitted")
		case errors.Is(err, utils.ErrMCPProxyBindingsChanged):
			utils.WriteErrorResponseWithReason(w, http.StatusConflict,
				utils.ErrMCPProxyBindingsChanged.Error(), err.Error(), utils.ErrCodeConflict)
		case errors.Is(err, utils.ErrMCPProxyHandleInUse):
			utils.WriteErrorResponseWithReason(w, http.StatusConflict,
				"MCP proxy id is already in use", err.Error(), utils.ErrCodeHandleInUse)
		case errors.Is(err, utils.ErrImmutableFieldChange):
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
				"source cannot be changed", err.Error(), utils.ErrCodeImmutableField)
		case errors.Is(err, utils.ErrMCPContextInUse):
			utils.WriteErrorResponseWithReason(w, http.StatusConflict,
				utils.ErrMCPContextInUse.Error(), err.Error(), utils.ErrCodeContextInUse)
		case errors.Is(err, utils.ErrMCPSourceComponentNotFound):
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
				"Source component not found", err.Error(), utils.ErrCodeSourceComponentNotFound)
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Bad request", err.Error(), utils.ErrCodeValidation)
		default:
			log.Error("UpsertProjectMCPProxy: failed", "ouID", ouID, "handle", handle, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to save MCP proxy")
		}
		return
	}

	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	utils.WriteSuccessResponse(w, status, resp)
}

// ListProjectMCPProxies handles GET /orgs/{orgName}/projects/{projName}/mcp-proxies.
func (c *projectMCPProxyController) ListProjectMCPProxies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)

	limit := getMCPProxyIntQueryParam(r, "limit", utils.DefaultLimit)
	offset := getMCPProxyIntQueryParam(r, "offset", utils.DefaultOffset)
	if limit < utils.MinLimit || limit > utils.MaxLimit {
		limit = utils.DefaultLimit
	}
	if offset < utils.MinOffset {
		offset = utils.DefaultOffset
	}

	project, deleted, ok := c.resolveProjectOrDeleted(w, r, ouID)
	if !ok {
		return
	}
	component, endpoint := r.URL.Query().Get("component"), r.URL.Query().Get("endpoint")
	var resp *models.ProjectMCPProxyListResponse
	var err error
	if deleted {
		resp, err = c.service.ListInDeletedProject(ctx, ouID, r.PathValue(utils.PathParamProjName), component, endpoint, limit, offset)
	} else {
		resp, err = c.service.List(ctx, ouID, project.UUID, component, endpoint, limit, offset)
	}
	if err != nil {
		log.Error("ListProjectMCPProxies: failed", "ouID", ouID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list MCP proxies")
		return
	}
	if deleted && resp.Pagination.Count == 0 {
		writeProjectNotFound(w)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// GetProjectMCPProxy handles GET /orgs/{orgName}/projects/{projName}/mcp-proxies/{proxyId}.
func (c *projectMCPProxyController) GetProjectMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	handle := r.PathValue(utils.PathParamProxyId)

	project, deleted, ok := c.resolveProjectOrDeleted(w, r, ouID)
	if !ok {
		return
	}
	var resp *models.ProjectMCPProxyResponse
	var err error
	if deleted {
		resp, err = c.service.GetInDeletedProject(ctx, ouID, r.PathValue(utils.PathParamProjName), handle)
	} else {
		resp, err = c.service.Get(ctx, ouID, project.Name, project.UUID, handle)
	}
	if err != nil {
		switch {
		case deleted && errors.Is(err, utils.ErrMCPProxyNotFound):
			writeProjectNotFound(w)
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid MCP proxy id")
		default:
			log.Error("GetProjectMCPProxy: failed", "ouID", ouID, "handle", handle, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get MCP proxy")
		}
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// DeleteProjectMCPProxy handles DELETE /orgs/{orgName}/projects/{projName}/mcp-proxies/{proxyId}.
func (c *projectMCPProxyController) DeleteProjectMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	handle := r.PathValue(utils.PathParamProxyId)

	project, deleted, ok := c.resolveProjectOrDeleted(w, r, ouID)
	if !ok {
		return
	}
	var err error
	if deleted {
		err = c.service.DeleteInDeletedProject(ctx, ouID, r.PathValue(utils.PathParamProjName), handle)
	} else {
		err = c.service.Delete(ctx, ouID, project.Name, project.UUID, handle)
	}
	if err != nil {
		switch {
		case deleted && errors.Is(err, utils.ErrMCPProxyNotFound):
			writeProjectNotFound(w)
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, utils.ErrMCPProxyHasMappings):
			utils.WriteErrorResponseWithReason(w, http.StatusConflict,
				utils.ErrMCPProxyHasMappings.Error(), "", utils.ErrCodeMCPProxyInUse)
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid MCP proxy id")
		default:
			log.Error("DeleteProjectMCPProxy: failed", "ouID", ouID, "handle", handle, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete MCP proxy")
		}
		return
	}
	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// PutProjectMCPProxyEnvironment handles PUT .../mcp-proxies/{proxyId}/environments/{envName}.
func (c *projectMCPProxyController) PutProjectMCPProxyEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	handle := r.PathValue(utils.PathParamProxyId)
	envName := r.PathValue(pathParamEnvName)

	var req models.MCPProxyEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("PutProjectMCPProxyEnvironment: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	project, ok := c.resolveProject(w, r, ouID)
	if !ok {
		return
	}
	resp, created, err := c.service.PutEnvironment(ctx, ouID, project.Name, project.UUID, handle, envName, req)
	if err != nil {
		if !writeProjectMCPBindingError(w, err) {
			log.Error("PutProjectMCPProxyEnvironment: failed", "ouID", ouID, "handle", handle, "environment", envName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to save the MCP proxy binding")
		}
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	utils.WriteSuccessResponse(w, status, resp)
}

// GetProjectMCPProxyEnvironment handles GET .../mcp-proxies/{proxyId}/environments/{envName}.
func (c *projectMCPProxyController) GetProjectMCPProxyEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	handle := r.PathValue(utils.PathParamProxyId)
	envName := r.PathValue(pathParamEnvName)

	project, deleted, ok := c.resolveProjectOrDeleted(w, r, ouID)
	if !ok {
		return
	}
	var resp *models.MCPProxyEnvironmentResponse
	var err error
	if deleted {
		resp, err = c.service.GetEnvironmentInDeletedProject(ctx, ouID, r.PathValue(utils.PathParamProjName), handle, envName)
	} else {
		resp, err = c.service.GetEnvironment(ctx, ouID, project.Name, project.UUID, handle, envName)
	}
	if err != nil {
		if deleted && errors.Is(err, utils.ErrMCPProxyNotFound) {
			writeProjectNotFound(w)
			return
		}
		if !writeProjectMCPBindingError(w, err) {
			log.Error("GetProjectMCPProxyEnvironment: failed", "ouID", ouID, "handle", handle, "environment", envName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get the MCP proxy binding")
		}
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// DeleteProjectMCPProxyEnvironment handles DELETE .../mcp-proxies/{proxyId}/environments/{envName}.
func (c *projectMCPProxyController) DeleteProjectMCPProxyEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	handle := r.PathValue(utils.PathParamProxyId)
	envName := r.PathValue(pathParamEnvName)

	project, deleted, ok := c.resolveProjectOrDeleted(w, r, ouID)
	if !ok {
		return
	}
	var err error
	if deleted {
		err = c.service.DeleteEnvironmentInDeletedProject(ctx, ouID, r.PathValue(utils.PathParamProjName), handle, envName)
	} else {
		err = c.service.DeleteEnvironment(ctx, ouID, project.Name, project.UUID, handle, envName)
	}
	if err != nil {
		if deleted && errors.Is(err, utils.ErrMCPProxyNotFound) {
			writeProjectNotFound(w)
			return
		}
		if !writeProjectMCPBindingError(w, err) {
			log.Error("DeleteProjectMCPProxyEnvironment: failed", "ouID", ouID, "handle", handle, "environment", envName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete the MCP proxy binding")
		}
		return
	}
	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// writeProjectMCPBindingError writes the response for a binding route's known errors; false for an unexpected one.
func writeProjectMCPBindingError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, utils.ErrMCPProxyNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound, "MCP proxy not found", "", utils.ErrCodeNotFound)
	case errors.Is(err, utils.ErrMCPProxyEnvironmentNotBound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			utils.ErrMCPProxyEnvironmentNotBound.Error(), "", utils.ErrCodeNotFound)
	case errors.Is(err, utils.ErrEnvironmentNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound, "Environment not found", err.Error(), utils.ErrCodeEnvironmentNotFound)
	case errors.Is(err, utils.ErrMCPUpstreamNotReady):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			utils.ErrMCPUpstreamNotReady.Error(), err.Error(), utils.ErrCodeUpstreamNotReady)
	case errors.Is(err, utils.ErrMCPContextInUse):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			utils.ErrMCPContextInUse.Error(), err.Error(), utils.ErrCodeContextInUse)
	case errors.Is(err, utils.ErrMCPProxyURLChanged):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			utils.ErrMCPProxyURLChanged.Error(), "", utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrMCPCapabilitiesGivenByCaller):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			utils.ErrMCPCapabilitiesGivenByCaller.Error(), "", utils.ErrCodeCapabilitiesGivenByCaller)
	case errors.Is(err, utils.ErrMCPProxyHasMappings):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			utils.ErrMCPProxyHasMappings.Error(), "", utils.ErrCodeMCPProxyInUse)
	case errors.Is(err, utils.ErrImmutableFieldChange):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"The binding's gateway cannot change once placed", err.Error(), utils.ErrCodeImmutableField)
	case errors.Is(err, utils.ErrMCPAmbiguousGateway):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			utils.ErrMCPAmbiguousGateway.Error(), err.Error(), utils.ErrCodeAmbiguousGateway)
	case errors.Is(err, utils.ErrInvalidInput):
		message := strings.TrimPrefix(err.Error(), utils.ErrInvalidInput.Error()+": ")
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, message, err.Error(), utils.ErrCodeValidation)
	default:
		return false
	}
	return true
}

// RefreshProjectMCPProxyEnvironmentCapabilities handles POST .../mcp-proxies/{proxyId}/environments/{envName}/capabilities/refresh.
func (c *projectMCPProxyController) RefreshProjectMCPProxyEnvironmentCapabilities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ouID := middleware.OUIDFromRequest(r)
	handle := r.PathValue(utils.PathParamProxyId)
	envName := r.PathValue(pathParamEnvName)
	project, ok := c.resolveProject(w, r, ouID)
	if !ok {
		return
	}
	resp, err := c.service.RefreshEnvironmentCapabilities(ctx, ouID, project.UUID, handle, envName)
	if err != nil {
		if !writeProjectMCPBindingError(w, err) {
			logger.GetLogger(ctx).Error("RefreshProjectMCPProxyEnvironmentCapabilities: failed",
				"ouID", ouID, "handle", handle, "environment", envName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to queue the capability discovery")
		}
		return
	}
	utils.WriteSuccessResponse(w, http.StatusAccepted, resp)
}
