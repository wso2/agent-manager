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
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MCPProxyScopeController defines HTTP handlers for per-MCP-proxy scopes.
type MCPProxyScopeController interface {
	ListMCPProxyScopes(w http.ResponseWriter, r *http.Request)
	CreateMCPProxyScope(w http.ResponseWriter, r *http.Request)
	UpdateMCPProxyScope(w http.ResponseWriter, r *http.Request)
	DeleteMCPProxyScope(w http.ResponseWriter, r *http.Request)
	// ListAgentIdentityScopes serves the env-filtered aggregate (Task 6).
	ListAgentIdentityScopes(w http.ResponseWriter, r *http.Request)
}

type mcpProxyScopeController struct {
	svc services.MCPProxyScopeService
}

// NewMCPProxyScopeController creates a new MCP proxy scope controller.
func NewMCPProxyScopeController(svc services.MCPProxyScopeService) MCPProxyScopeController {
	return &mcpProxyScopeController{svc: svc}
}

// ListMCPProxyScopes handles GET /orgs/{orgName}/mcp-proxies/{proxyId}/scopes.
func (c *mcpProxyScopeController) ListMCPProxyScopes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	orgName := middleware.OrgHandleFromRequest(r)
	proxyID := r.PathValue(utils.PathParamProxyId)

	log.Info("ListMCPProxyScopes: starting", "ouID", ouID, "orgName", orgName, "proxyID", proxyID)

	result, err := c.svc.List(ctx, ouID, proxyID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			log.Warn("ListMCPProxyScopes: MCP proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		default:
			log.Error("ListMCPProxyScopes: failed", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list MCP proxy scopes")
		}
		return
	}

	items := make([]spec.MCPProxyScopeResponse, 0, len(result.Scopes))
	for i := range result.Scopes {
		items = append(items, toMCPProxyScopeResponse(result.ProxyHandle, result.Scopes[i]))
	}

	log.Info("ListMCPProxyScopes: completed", "ouID", ouID, "proxyID", proxyID, "count", len(items))
	utils.WriteSuccessResponse(w, http.StatusOK, spec.MCPProxyScopeListResponse{Scopes: items})
}

// CreateMCPProxyScope handles POST /orgs/{orgName}/mcp-proxies/{proxyId}/scopes.
func (c *mcpProxyScopeController) CreateMCPProxyScope(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	orgName := middleware.OrgHandleFromRequest(r)
	proxyID := r.PathValue(utils.PathParamProxyId)

	log.Info("CreateMCPProxyScope: starting", "ouID", ouID, "orgName", orgName, "proxyID", proxyID)

	var body spec.MCPProxyScopeRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Error("CreateMCPProxyScope: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	description := ""
	if body.Description != nil {
		description = *body.Description
	}

	result, err := c.svc.Create(ctx, ouID, orgName, proxyID, models.MCPProxyScopeInput{
		Action:      body.Action,
		Description: description,
		Tools:       body.Tools,
	})
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("CreateMCPProxyScope: invalid request", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, utils.ErrMCPProxyProjectOwned):
			log.Warn("CreateMCPProxyScope: project-owned MCP proxy", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponseWithReason(w, http.StatusConflict, "MCP proxy belongs to a project",
				err.Error(), utils.ErrCodeProjectOwned)
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			log.Warn("CreateMCPProxyScope: MCP proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, utils.ErrConflict):
			log.Warn("CreateMCPProxyScope: scope already exists", "ouID", ouID, "proxyID", proxyID, "action", body.Action)
			utils.WriteErrorResponse(w, http.StatusConflict, err.Error())
		default:
			log.Error("CreateMCPProxyScope: failed", "ouID", ouID, "proxyID", proxyID, "action", body.Action, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create MCP proxy scope")
		}
		return
	}

	log.Info("CreateMCPProxyScope: completed", "ouID", ouID, "proxyID", proxyID, "action", body.Action)
	utils.WriteSuccessResponse(w, http.StatusCreated, toMCPProxyScopeResponse(result.ProxyHandle, result.Scope))
}

// UpdateMCPProxyScope handles PUT /orgs/{orgName}/mcp-proxies/{proxyId}/scopes/{scopeAction}.
func (c *mcpProxyScopeController) UpdateMCPProxyScope(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	orgName := middleware.OrgHandleFromRequest(r)
	proxyID := r.PathValue(utils.PathParamProxyId)
	action := r.PathValue(utils.PathParamScopeAction)

	log.Info("UpdateMCPProxyScope: starting", "ouID", ouID, "orgName", orgName, "proxyID", proxyID, "action", action)

	var body spec.MCPProxyScopeUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		log.Error("UpdateMCPProxyScope: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	result, err := c.svc.Update(ctx, ouID, orgName, proxyID, action, models.MCPProxyScopeUpdateInput{
		Description: body.Description,
		Tools:       body.Tools,
	})
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("UpdateMCPProxyScope: invalid request", "ouID", ouID, "proxyID", proxyID, "action", action, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, utils.ErrMCPProxyProjectOwned):
			log.Warn("UpdateMCPProxyScope: project-owned MCP proxy", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponseWithReason(w, http.StatusConflict, "MCP proxy belongs to a project",
				err.Error(), utils.ErrCodeProjectOwned)
		case errors.Is(err, utils.ErrMCPProxyNotFound), errors.Is(err, utils.ErrScopeNotFound):
			log.Warn("UpdateMCPProxyScope: not found", "ouID", ouID, "proxyID", proxyID, "action", action)
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy or scope not found")
		default:
			log.Error("UpdateMCPProxyScope: failed", "ouID", ouID, "proxyID", proxyID, "action", action, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update MCP proxy scope")
		}
		return
	}

	log.Info("UpdateMCPProxyScope: completed", "ouID", ouID, "proxyID", proxyID, "action", action)
	utils.WriteSuccessResponse(w, http.StatusOK, toMCPProxyScopeResponse(result.ProxyHandle, result.Scope))
}

// DeleteMCPProxyScope handles DELETE /orgs/{orgName}/mcp-proxies/{proxyId}/scopes/{scopeAction}.
func (c *mcpProxyScopeController) DeleteMCPProxyScope(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	orgName := middleware.OrgHandleFromRequest(r)
	proxyID := r.PathValue(utils.PathParamProxyId)
	action := r.PathValue(utils.PathParamScopeAction)

	log.Info("DeleteMCPProxyScope: starting", "ouID", ouID, "orgName", orgName, "proxyID", proxyID, "action", action)

	if err := c.svc.Delete(ctx, ouID, orgName, proxyID, action); err != nil {
		switch {
		case errors.Is(err, utils.ErrMCPProxyProjectOwned):
			log.Warn("DeleteMCPProxyScope: project-owned MCP proxy", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponseWithReason(w, http.StatusConflict, "MCP proxy belongs to a project",
				err.Error(), utils.ErrCodeProjectOwned)
		case errors.Is(err, utils.ErrMCPProxyNotFound), errors.Is(err, utils.ErrScopeNotFound):
			log.Warn("DeleteMCPProxyScope: not found", "ouID", ouID, "proxyID", proxyID, "action", action)
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy or scope not found")
		default:
			log.Error("DeleteMCPProxyScope: failed", "ouID", ouID, "proxyID", proxyID, "action", action, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete MCP proxy scope")
		}
		return
	}

	log.Info("DeleteMCPProxyScope: completed", "ouID", ouID, "proxyID", proxyID, "action", action)
	w.WriteHeader(http.StatusNoContent)
}

// ListAgentIdentityScopes handles GET /orgs/{orgName}/environments/{envName}/agent-identities/scopes.
func (c *mcpProxyScopeController) ListAgentIdentityScopes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	orgName := middleware.OrgHandleFromRequest(r)
	envName := r.PathValue("envName")

	log.Info("ListAgentIdentityScopes: starting", "ouID", ouID, "orgName", orgName, "envName", envName)

	entries, err := c.svc.ListEnvironmentScopes(ctx, ouID, envName)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrEnvironmentNotFound):
			log.Warn("ListAgentIdentityScopes: environment not found", "ouID", ouID, "envName", envName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
		default:
			log.Error("ListAgentIdentityScopes: failed", "ouID", ouID, "envName", envName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list agent-identity scopes")
		}
		return
	}

	items := make([]spec.AgentIdentityScopeEntry, 0, len(entries))
	for _, e := range entries {
		items = append(items, toAgentIdentityScopeEntry(e))
	}

	log.Info("ListAgentIdentityScopes: completed", "ouID", ouID, "envName", envName, "count", len(items))
	utils.WriteSuccessResponse(w, http.StatusOK, spec.AgentIdentityScopeListResponse{Scopes: items})
}

// toMCPProxyScopeResponse maps a stored scope to its API representation.
func toMCPProxyScopeResponse(handle string, s models.MCPProxyScope) spec.MCPProxyScopeResponse {
	description := s.Description
	createdAt := s.CreatedAt
	updatedAt := s.UpdatedAt
	return spec.MCPProxyScopeResponse{
		Action:      s.Action,
		Scope:       s.ScopeString(handle),
		Description: &description,
		Tools:       s.Tools,
		CreatedAt:   &createdAt,
		UpdatedAt:   &updatedAt,
	}
}

// toAgentIdentityScopeEntry maps an env-filtered scope aggregate entry to its API representation.
func toAgentIdentityScopeEntry(e models.EnvironmentScopeEntry) spec.AgentIdentityScopeEntry {
	entry := spec.AgentIdentityScopeEntry{
		Scope:      e.Scope,
		McpProxyId: e.MCPProxyID,
	}
	if e.Description != "" {
		entry.Description = &e.Description
	}
	if e.MCPProxyName != "" {
		entry.McpProxyName = &e.MCPProxyName
	}
	return entry
}
