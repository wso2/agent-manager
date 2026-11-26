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
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconv"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
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
	svc gwsvc.MCPProxyScopeService
}

// NewMCPProxyScopeController creates a new MCP proxy scope controller.
func NewMCPProxyScopeController(svc gwsvc.MCPProxyScopeService) MCPProxyScopeController {
	return &mcpProxyScopeController{svc: svc}
}

// ListMCPProxyScopes handles GET /m/ai-gateway/mcp-proxies/{proxyId}/scopes.
func (c *mcpProxyScopeController) ListMCPProxyScopes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	proxyID := r.PathValue(gwutil.PathParamProxyId)

	log.Info("ListMCPProxyScopes: starting", "ouID", ouID, "proxyID", proxyID)

	result, err := c.svc.List(ctx, ouID, proxyID)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrMCPProxyNotFound):
			log.Warn("ListMCPProxyScopes: MCP proxy not found", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		default:
			log.Error("ListMCPProxyScopes: failed", "ouID", ouID, "proxyID", proxyID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list MCP proxy scopes")
		}
		return
	}

	out := gwconv.MCPProxyScopeList(result)

	log.Info("ListMCPProxyScopes: completed", "ouID", ouID, "proxyID", proxyID, "count", len(out.List))
	httpx.WriteSuccessResponse(w, http.StatusOK, out)
}

// CreateMCPProxyScope handles POST /m/ai-gateway/mcp-proxies/{proxyId}/scopes.
func (c *mcpProxyScopeController) CreateMCPProxyScope(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	proxyID := r.PathValue(gwutil.PathParamProxyId)

	log.Info("CreateMCPProxyScope: starting", "ouID", ouID, "proxyID", proxyID)

	var doc gwspec.McpProxyScopeDocument
	if err := json.NewDecoder(io.LimitReader(r.Body, maxDocumentBytes)).Decode(&doc); err != nil {
		log.Error("CreateMCPProxyScope: failed to decode the document", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// No runtime schema validation, so the converter checks apiVersion, kind and metadata.
	in, err := gwconv.MCPProxyScopeFromDocument(doc)
	if err != nil {
		writeDocumentError(w, ctx, "CreateMCPProxyScope", ouID, err)
		return
	}
	action := in.Action

	result, err := c.svc.Create(ctx, ouID, proxyID, in)
	if err != nil {
		switch {
		case errors.Is(err, httpx.ErrInvalidInput):
			log.Error("CreateMCPProxyScope: invalid request", "ouID", ouID, "proxyID", proxyID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, gwerr.ErrMCPProxyProjectOwned):
			log.Warn("CreateMCPProxyScope: project-owned MCP proxy", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponseWithReason(w, http.StatusConflict, "MCP proxy belongs to a project",
				err.Error(), gwerr.ErrCodeProjectOwned)
		case errors.Is(err, gwerr.ErrMCPProxyNotFound):
			log.Warn("CreateMCPProxyScope: MCP proxy not found", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, httpx.ErrConflict):
			log.Warn("CreateMCPProxyScope: scope already exists", "ouID", ouID, "proxyID", proxyID, "action", action)
			httpx.WriteErrorResponse(w, http.StatusConflict, err.Error())
		default:
			log.Error("CreateMCPProxyScope: failed", "ouID", ouID, "proxyID", proxyID, "action", action, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create MCP proxy scope")
		}
		return
	}

	log.Info("CreateMCPProxyScope: completed", "ouID", ouID, "proxyID", proxyID, "action", action)
	httpx.WriteSuccessResponse(w, http.StatusCreated, gwconv.MCPProxyScopeDocument(result.ProxyHandle, result.Scope))
}

// UpdateMCPProxyScope handles PUT /m/ai-gateway/mcp-proxies/{proxyId}/scopes/{scopeAction}.
func (c *mcpProxyScopeController) UpdateMCPProxyScope(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	proxyID := r.PathValue(gwutil.PathParamProxyId)
	action := r.PathValue(gwutil.PathParamScopeAction)

	log.Info("UpdateMCPProxyScope: starting", "ouID", ouID, "proxyID", proxyID, "action", action)

	var doc gwspec.McpProxyScopeDocument
	if err := json.NewDecoder(io.LimitReader(r.Body, maxDocumentBytes)).Decode(&doc); err != nil {
		log.Error("UpdateMCPProxyScope: failed to decode the document", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	in, err := gwconv.MCPProxyScopeUpdateFromDocument(doc)
	if err != nil {
		writeDocumentError(w, ctx, "UpdateMCPProxyScope", ouID, err)
		return
	}
	// The action is immutable, so the document must name the scope the path addresses.
	if doc.Metadata.Name != action {
		log.Warn("UpdateMCPProxyScope: metadata.name does not match the scope", "ouID", ouID,
			"proxyID", proxyID, "action", action, "metadataName", doc.Metadata.Name)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "metadata.name does not match the scope action in the path")
		return
	}

	result, err := c.svc.Update(ctx, ouID, proxyID, action, in)
	if err != nil {
		switch {
		case errors.Is(err, httpx.ErrInvalidInput):
			log.Error("UpdateMCPProxyScope: invalid request", "ouID", ouID, "proxyID", proxyID, "action", action, "error", err)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, gwerr.ErrMCPProxyProjectOwned):
			log.Warn("UpdateMCPProxyScope: project-owned MCP proxy", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponseWithReason(w, http.StatusConflict, "MCP proxy belongs to a project",
				err.Error(), gwerr.ErrCodeProjectOwned)
		case errors.Is(err, gwerr.ErrMCPProxyNotFound), errors.Is(err, gwerr.ErrScopeNotFound):
			log.Warn("UpdateMCPProxyScope: not found", "ouID", ouID, "proxyID", proxyID, "action", action)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy or scope not found")
		default:
			log.Error("UpdateMCPProxyScope: failed", "ouID", ouID, "proxyID", proxyID, "action", action, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update MCP proxy scope")
		}
		return
	}

	log.Info("UpdateMCPProxyScope: completed", "ouID", ouID, "proxyID", proxyID, "action", action)
	httpx.WriteSuccessResponse(w, http.StatusOK, gwconv.MCPProxyScopeDocument(result.ProxyHandle, result.Scope))
}

// DeleteMCPProxyScope handles DELETE /m/ai-gateway/mcp-proxies/{proxyId}/scopes/{scopeAction}.
func (c *mcpProxyScopeController) DeleteMCPProxyScope(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	proxyID := r.PathValue(gwutil.PathParamProxyId)
	action := r.PathValue(gwutil.PathParamScopeAction)

	log.Info("DeleteMCPProxyScope: starting", "ouID", ouID, "proxyID", proxyID, "action", action)

	if err := c.svc.Delete(ctx, ouID, proxyID, action); err != nil {
		switch {
		case errors.Is(err, gwerr.ErrMCPProxyProjectOwned):
			log.Warn("DeleteMCPProxyScope: project-owned MCP proxy", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponseWithReason(w, http.StatusConflict, "MCP proxy belongs to a project",
				err.Error(), gwerr.ErrCodeProjectOwned)
		case errors.Is(err, gwerr.ErrMCPProxyNotFound), errors.Is(err, gwerr.ErrScopeNotFound):
			log.Warn("DeleteMCPProxyScope: not found", "ouID", ouID, "proxyID", proxyID, "action", action)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy or scope not found")
		default:
			log.Error("DeleteMCPProxyScope: failed", "ouID", ouID, "proxyID", proxyID, "action", action, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete MCP proxy scope")
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
	ouID := modkit.OrgID(r.Context())
	envName := r.PathValue("envName")

	log.Info("ListAgentIdentityScopes: starting", "ouID", ouID, "envName", envName)

	entries, err := c.svc.ListEnvironmentScopes(ctx, ouID, envName)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrEnvironmentNotFound):
			log.Warn("ListAgentIdentityScopes: environment not found", "ouID", ouID, "envName", envName)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
		default:
			log.Error("ListAgentIdentityScopes: failed", "ouID", ouID, "envName", envName, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list agent-identity scopes")
		}
		return
	}

	items := make([]gwspec.AgentIdentityScopeEntry, 0, len(entries))
	for _, e := range entries {
		items = append(items, toAgentIdentityScopeEntry(e))
	}

	log.Info("ListAgentIdentityScopes: completed", "ouID", ouID, "envName", envName, "count", len(items))
	httpx.WriteSuccessResponse(w, http.StatusOK, gwspec.AgentIdentityScopeListResponse{Scopes: items})
}

// toAgentIdentityScopeEntry maps an env-filtered scope aggregate entry to its API representation.
func toAgentIdentityScopeEntry(e gwmodels.EnvironmentScopeEntry) gwspec.AgentIdentityScopeEntry {
	entry := gwspec.AgentIdentityScopeEntry{
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
