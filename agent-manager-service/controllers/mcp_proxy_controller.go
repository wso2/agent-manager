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
	"fmt"
	"net/http"
	"strconv"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// MCPProxyController defines handlers for MCP proxy operations.
type MCPProxyController interface {
	CreateMCPProxy(w http.ResponseWriter, r *http.Request)
	ListMCPProxies(w http.ResponseWriter, r *http.Request)
	ListAvailableMCPPolicies(w http.ResponseWriter, r *http.Request)
	GetMCPProxy(w http.ResponseWriter, r *http.Request)
	UpdateMCPProxy(w http.ResponseWriter, r *http.Request)
	DeleteMCPProxy(w http.ResponseWriter, r *http.Request)
	FetchServerInfo(w http.ResponseWriter, r *http.Request)
}

type mcpProxyController struct {
	mcpProxyService    *services.MCPProxyService
	agentConfigService services.AgentConfigurationService
}

// NewMCPProxyController creates a new MCP proxy controller.
//
// agentConfigService is orchestrated here rather than from inside MCPProxyService because
// the agent configuration service already depends on the MCP proxy service; calling back
// the other way would close a dependency cycle.
func NewMCPProxyController(
	mcpProxyService *services.MCPProxyService,
	agentConfigService services.AgentConfigurationService,
) MCPProxyController {
	return &mcpProxyController{
		mcpProxyService:    mcpProxyService,
		agentConfigService: agentConfigService,
	}
}

// CreateMCPProxy handles POST /orgs/{ouID}/mcp-proxies.
func (c *mcpProxyController) CreateMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)

	log.Info("CreateMCPProxy: starting", "ouID", ouID)

	var req models.MCPProxyDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateMCPProxy: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// amp.connections.create-mcp-proxy's probed_first dimension: fetch-server-info
	// returns the same prompts/resources/tools shape carried on an endpoint's
	// Capabilities, so an endpoint arriving with Capabilities already set is the
	// signal that the caller probed the server before registering it.
	growthanalytics.SetDimension(ctx, "probed_first", mcpProxyWasProbedFirst(req.Endpoints))

	resp, err := c.mcpProxyService.Create(ctx, ouID, "system", &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrInvalidInput), errors.Is(err, utils.ErrInvalidURL):
			log.Error("CreateMCPProxy: invalid request", "ouID", ouID, "error", err)
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Bad request", err.Error(), utils.ErrCodeBadRequest)
		case errors.Is(err, utils.ErrMCPProxyExists):
			log.Error("CreateMCPProxy: MCP proxy already exists", "ouID", ouID, "id", req.ID)
			utils.WriteErrorResponse(w, http.StatusConflict, fmt.Sprintf("MCP proxy %q already exists", req.ID))
		case errors.Is(err, utils.ErrArtifactExists):
			log.Error("CreateMCPProxy: handle already in use by another artifact", "ouID", ouID, "id", req.ID)
			utils.WriteErrorResponse(w, http.StatusConflict, fmt.Sprintf("Handle %q is already in use", req.ID))
		case errors.Is(err, utils.ErrMCPEnvAlreadyBound):
			log.Error("CreateMCPProxy: environment already bound to another endpoint", "ouID", ouID, "id", req.ID)
			utils.WriteErrorResponse(w, http.StatusConflict, err.Error())
		default:
			log.Error("CreateMCPProxy: failed", "ouID", ouID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create MCP proxy")
		}
		return
	}

	log.Info("CreateMCPProxy: completed", "ouID", ouID, "id", req.ID)
	utils.WriteSuccessResponse(w, http.StatusCreated, resp)
}

// ListMCPProxies handles GET /orgs/{ouID}/mcp-proxies.
func (c *mcpProxyController) ListMCPProxies(w http.ResponseWriter, r *http.Request) {
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

	log.Info("ListMCPProxies: starting", "ouID", ouID, "limit", limit, "offset", offset)

	resp, err := c.mcpProxyService.List(ctx, ouID, limit, offset)
	if err != nil {
		log.Error("ListMCPProxies: failed", "ouID", ouID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list MCP proxies")
		return
	}

	log.Info("ListMCPProxies: completed", "ouID", ouID, "count", resp.Count)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// ListAvailableMCPPolicies handles GET /orgs/{ouID}/mcp-proxies/policies.
func (c *mcpProxyController) ListAvailableMCPPolicies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)

	log.Info("ListAvailableMCPPolicies: starting", "ouID", ouID)

	resp, err := c.mcpProxyService.ListAvailableMCPPolicies(ctx, ouID)
	if err != nil {
		log.Error("ListAvailableMCPPolicies: failed", "ouID", ouID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list available MCP policies")
		return
	}

	log.Info("ListAvailableMCPPolicies: completed", "ouID", ouID, "count", resp.Count)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// GetMCPProxy handles GET /orgs/{ouID}/mcp-proxies/{proxyId}.
func (c *mcpProxyController) GetMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue(utils.PathParamProxyId)

	log.Info("GetMCPProxy: starting", "ouID", ouID, "proxyID", proxyID)

	resp, err := c.mcpProxyService.Get(ctx, ouID, proxyID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			log.Warn("GetMCPProxy: MCP proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("GetMCPProxy: invalid request", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid MCP proxy id")
		default:
			log.Error("GetMCPProxy: failed", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get MCP proxy")
		}
		return
	}

	log.Info("GetMCPProxy: completed", "ouID", ouID, "proxyID", proxyID)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// UpdateMCPProxy handles PUT /orgs/{ouID}/mcp-proxies/{proxyId}.
func (c *mcpProxyController) UpdateMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue(utils.PathParamProxyId)

	log.Info("UpdateMCPProxy: starting", "ouID", ouID, "proxyID", proxyID)

	var req models.MCPProxyDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateMCPProxy: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := c.mcpProxyService.Update(ctx, ouID, proxyID, &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			log.Warn("UpdateMCPProxy: MCP proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, utils.ErrInvalidInput), errors.Is(err, utils.ErrInvalidURL):
			log.Error("UpdateMCPProxy: invalid request", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Bad request", err.Error(), utils.ErrCodeBadRequest)
		case errors.Is(err, utils.ErrArtifactExists):
			log.Error("UpdateMCPProxy: handle already in use by another artifact", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusConflict, err.Error())
		case errors.Is(err, utils.ErrMCPEnvAlreadyBound):
			log.Error("UpdateMCPProxy: environment already bound to another endpoint", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusConflict, err.Error())
		default:
			log.Error("UpdateMCPProxy: failed", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update MCP proxy")
		}
		return
	}

	// The org-level MCP proxy is deployed directly to the gateway; agents reference it via a
	// DB mapping and read its endpoint at their own deploy time, so an update to an endpoint
	// an agent is already bound to requires no changes to that agent.
	//
	// Binding an endpoint to a NEW environment does, though: agents already promoted into
	// that environment were left with their MCP env vars injected but empty, because no
	// binding could be created for them at the time. Reconcile them now. Best-effort; a
	// failure leaves the proxy updated and is retried on the next update.
	//
	// This covers the proxy-side trigger only. Deployability also flips on gateway topology
	// — PlatformGatewayService.AssignGatewayToEnvironment mapping the first egress gateway to
	// an environment resolves the same dead state for every proxy already bound there — and
	// those paths do not reconcile. Agents stuck that way stay stuck until this proxy is
	// updated again.
	//
	// Run inline rather than detached so the caller reads back a bound agent immediately.
	// Nothing here calls out to the gateway or OpenChoreo until an environment is genuinely
	// bindable, so the steady state is a handful of indexed reads.
	if err := c.agentConfigService.ReconcileMCPBindingsForProxy(ctx, ouID, proxyID); err != nil {
		log.Warn("UpdateMCPProxy: failed to reconcile agent MCP bindings", "ouID", ouID, "proxyID", proxyID, "error", err)
	}

	log.Info("UpdateMCPProxy: completed", "ouID", ouID, "proxyID", proxyID)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// DeleteMCPProxy handles DELETE /orgs/{ouID}/mcp-proxies/{proxyId}.
func (c *mcpProxyController) DeleteMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	orgName := middleware.OrgHandleFromRequest(r)
	proxyID := r.PathValue(utils.PathParamProxyId)

	log.Info("DeleteMCPProxy: starting", "ouID", ouID, "proxyID", proxyID)

	if err := c.mcpProxyService.Delete(ctx, ouID, orgName, proxyID); err != nil {
		switch {
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			log.Warn("DeleteMCPProxy: MCP proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, utils.ErrMCPProxyHasMappings):
			log.Warn("DeleteMCPProxy: MCP proxy has mappings", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusConflict, utils.ErrMCPProxyHasMappings.Error())
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("DeleteMCPProxy: invalid request", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid MCP proxy id")
		default:
			log.Error("DeleteMCPProxy: failed", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete MCP proxy")
		}
		return
	}

	log.Info("DeleteMCPProxy: completed", "ouID", ouID, "proxyID", proxyID)
	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// FetchServerInfo handles POST /orgs/{ouID}/mcp-proxies/fetch-server-info.
func (c *mcpProxyController) FetchServerInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)

	log.Info("FetchMCPProxyServerInfo: starting", "ouID", ouID)

	var req models.MCPServerInfoFetchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("FetchMCPProxyServerInfo: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := c.mcpProxyService.FetchServerInfo(ctx, &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrInvalidInput), errors.Is(err, utils.ErrInvalidURL):
			log.Error("FetchMCPProxyServerInfo: invalid request", "ouID", ouID, "error", err)
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Bad request", err.Error(), utils.ErrCodeBadRequest)
		case errors.Is(err, utils.ErrURLUnreachable):
			log.Error("FetchMCPProxyServerInfo: MCP server URL is unreachable", "ouID", ouID, "error", err)
			utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, "MCP server URL is unreachable", err.Error(), utils.ErrCodeBadRequest)
		case errors.Is(err, utils.ErrMCPServerUnauthorized):
			log.Error("FetchMCPProxyServerInfo: MCP server returned unauthorized", "ouID", ouID, "error", err)
			utils.WriteErrorResponseWithReason(w, http.StatusUnauthorized, "MCP server returned 401 Unauthorized. Check the provided credentials.", err.Error(), utils.ErrCodeUnauthorized)
		default:
			log.Error("FetchMCPProxyServerInfo: failed", "ouID", ouID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to fetch MCP server info")
		}
		return
	}

	log.Info("FetchMCPProxyServerInfo: completed", "ouID", ouID)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// mcpProxyWasProbedFirst reports whether any endpoint in the request already
// carries Capabilities (prompts/resources/tools) — the shape
// FetchServerInfo returns — which is the signal that the caller probed the
// MCP server before registering it, for amp.connections.create-mcp-proxy's
// probed_first dimension.
func mcpProxyWasProbedFirst(endpoints []models.MCPProxyEndpointDTO) bool {
	for _, ep := range endpoints {
		if ep.Capabilities == nil {
			continue
		}
		c := ep.Capabilities
		if (c.Prompts != nil && len(*c.Prompts) > 0) ||
			(c.Resources != nil && len(*c.Resources) > 0) ||
			(c.Tools != nil && len(*c.Tools) > 0) {
			return true
		}
	}
	return false
}

func getMCPProxyIntQueryParam(r *http.Request, key string, defaultValue int) int {
	value := r.URL.Query().Get(key)
	if value == "" {
		return defaultValue
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return parsed
}
