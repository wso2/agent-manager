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
}

type projectMCPProxyController struct {
	service  *services.ProjectMCPProxyService
	ocClient client.OpenChoreoClient
}

// NewProjectMCPProxyController creates the project MCP proxy controller.
func NewProjectMCPProxyController(
	service *services.ProjectMCPProxyService,
	ocClient client.OpenChoreoClient,
) ProjectMCPProxyController {
	return &projectMCPProxyController{service: service, ocClient: ocClient}
}

// resolveProject maps {projName} to the OpenChoreo project. On failure it has written the
// response and returns ok=false.
func (c *projectMCPProxyController) resolveProject(w http.ResponseWriter, r *http.Request, ouID string) (*models.ProjectResponse, bool) {
	log := logger.GetLogger(r.Context())
	projName := r.PathValue(utils.PathParamProjName)
	project, err := c.ocClient.GetProject(r.Context(), ouID, projName)
	if err != nil && !errors.Is(err, utils.ErrProjectNotFound) {
		log.Error("project MCP proxy: failed to resolve project", "ouID", ouID, "project", projName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to resolve project")
		return nil, false
	}
	if err != nil || project == nil {
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound, "Project not found", "", utils.ErrCodeProjectNotFound)
		return nil, false
	}
	return project, true
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
	resp, created, err := c.service.UpsertWithCreateCheck(ctx, ouID, project.Name, project.UUID, handle, req, canCreate)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrMCPProxyCreateNotPermitted):
			utils.WriteErrorResponse(w, http.StatusForbidden, "Creating MCP proxies is not permitted")
		case errors.Is(err, utils.ErrMCPProxyHandleInUse):
			utils.WriteErrorResponseWithReason(w, http.StatusConflict,
				"MCP proxy id is already in use", err.Error(), utils.ErrCodeHandleInUse)
		case errors.Is(err, utils.ErrImmutableFieldChange):
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
				"source cannot be changed", err.Error(), utils.ErrCodeImmutableField)
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Bad request", err.Error(), utils.ErrCodeBadRequest)
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

	project, ok := c.resolveProject(w, r, ouID)
	if !ok {
		return
	}
	resp, err := c.service.List(ctx, ouID, project.UUID,
		r.URL.Query().Get("component"), r.URL.Query().Get("endpoint"), limit, offset)
	if err != nil {
		log.Error("ListProjectMCPProxies: failed", "ouID", ouID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list MCP proxies")
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

	project, ok := c.resolveProject(w, r, ouID)
	if !ok {
		return
	}
	resp, err := c.service.Get(ctx, ouID, project.UUID, handle)
	if err != nil {
		switch {
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

	project, ok := c.resolveProject(w, r, ouID)
	if !ok {
		return
	}
	if err := c.service.Delete(ctx, ouID, project.UUID, handle); err != nil {
		switch {
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
