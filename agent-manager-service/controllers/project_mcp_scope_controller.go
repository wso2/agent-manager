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

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// A project proxy's scopes: the same scopes an org proxy has (actions on the proxy's resource server, granted to
// agents through roles and enforced per tool on every binding secured with identity), on the project routes.

// writeProjectMCPScopeError writes the response for a project scope route's error.
func writeProjectMCPScopeError(w http.ResponseWriter, r *http.Request, op string, err error) {
	switch {
	case errors.Is(err, utils.ErrInvalidInput):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, err.Error(), "", utils.ErrCodeValidation)
	case errors.Is(err, utils.ErrMCPProxyNotFound), errors.Is(err, utils.ErrScopeNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy or scope not found")
	case errors.Is(err, utils.ErrConflict):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict, err.Error(), "", utils.ErrCodeConflict)
	default:
		logger.GetLogger(r.Context()).Error("project MCP proxy scope: "+op+" failed", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to "+op+" MCP proxy scope")
	}
}

// ListProjectMCPProxyScopes handles GET .../projects/{projName}/mcp-proxies/{proxyId}/scopes.
func (c *projectMCPProxyController) ListProjectMCPProxyScopes(w http.ResponseWriter, r *http.Request) {
	ouID := middleware.OUIDFromRequest(r)
	project, ok := c.resolveProject(w, r, ouID)
	if !ok {
		return
	}
	result, err := c.scopes.ListInProject(r.Context(), ouID, project.UUID, r.PathValue(utils.PathParamProxyId))
	if err != nil {
		writeProjectMCPScopeError(w, r, "list", err)
		return
	}
	items := make([]spec.MCPProxyScopeResponse, 0, len(result.Scopes))
	for i := range result.Scopes {
		items = append(items, toMCPProxyScopeResponse(result.ProxyHandle, result.Scopes[i]))
	}
	utils.WriteSuccessResponse(w, http.StatusOK, spec.MCPProxyScopeListResponse{Scopes: items})
}

// CreateProjectMCPProxyScope handles POST .../projects/{projName}/mcp-proxies/{proxyId}/scopes.
func (c *projectMCPProxyController) CreateProjectMCPProxyScope(w http.ResponseWriter, r *http.Request) {
	ouID := middleware.OUIDFromRequest(r)
	project, ok := c.resolveProject(w, r, ouID)
	if !ok {
		return
	}
	var body spec.MCPProxyScopeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Invalid request body", "", utils.ErrCodeValidation)
		return
	}
	description := ""
	if body.Description != nil {
		description = *body.Description
	}
	result, err := c.scopes.CreateInProject(r.Context(), ouID, project.UUID, r.PathValue(utils.PathParamProxyId),
		models.MCPProxyScopeInput{Action: body.Action, Description: description, Tools: body.Tools})
	if err != nil {
		writeProjectMCPScopeError(w, r, "create", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusCreated, toMCPProxyScopeResponse(result.ProxyHandle, result.Scope))
}

// UpdateProjectMCPProxyScope handles PUT .../projects/{projName}/mcp-proxies/{proxyId}/scopes/{scopeAction}.
func (c *projectMCPProxyController) UpdateProjectMCPProxyScope(w http.ResponseWriter, r *http.Request) {
	ouID := middleware.OUIDFromRequest(r)
	project, ok := c.resolveProject(w, r, ouID)
	if !ok {
		return
	}
	var body spec.MCPProxyScopeUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Invalid request body", "", utils.ErrCodeValidation)
		return
	}
	result, err := c.scopes.UpdateInProject(r.Context(), ouID, project.UUID, r.PathValue(utils.PathParamProxyId),
		r.PathValue(utils.PathParamScopeAction), models.MCPProxyScopeUpdateInput{Description: body.Description, Tools: body.Tools})
	if err != nil {
		writeProjectMCPScopeError(w, r, "update", err)
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, toMCPProxyScopeResponse(result.ProxyHandle, result.Scope))
}

// DeleteProjectMCPProxyScope handles DELETE .../projects/{projName}/mcp-proxies/{proxyId}/scopes/{scopeAction}.
func (c *projectMCPProxyController) DeleteProjectMCPProxyScope(w http.ResponseWriter, r *http.Request) {
	ouID := middleware.OUIDFromRequest(r)
	project, ok := c.resolveProject(w, r, ouID)
	if !ok {
		return
	}
	if err := c.scopes.DeleteInProject(r.Context(), ouID, project.UUID, r.PathValue(utils.PathParamProxyId),
		r.PathValue(utils.PathParamScopeAction)); err != nil {
		writeProjectMCPScopeError(w, r, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
