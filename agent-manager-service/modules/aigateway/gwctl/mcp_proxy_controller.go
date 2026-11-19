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
	"net/http"
	"strconv"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtrack"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/ssrf"
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
	mcpProxyService    *gwsvc.MCPProxyService
	agentConfigService gwsvc.AgentConfigurationService
	ocClient           modkit.CoreAPI
}

// NewMCPProxyController creates a new MCP proxy controller.
//
// agentConfigService is orchestrated here rather than from inside MCPProxyService because
// the agent configuration service already depends on the MCP proxy service; calling back
// the other way would close a dependency cycle.
func NewMCPProxyController(
	mcpProxyService *gwsvc.MCPProxyService,
	agentConfigService gwsvc.AgentConfigurationService,
	ocClient modkit.CoreAPI,
) MCPProxyController {
	return &mcpProxyController{
		mcpProxyService:    mcpProxyService,
		agentConfigService: agentConfigService,
		ocClient:           ocClient,
	}
}

// CreateMCPProxy handles POST /orgs/{ouID}/mcp-proxies.
func (c *mcpProxyController) CreateMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())

	log.Info("CreateMCPProxy: starting", "ouID", ouID)

	var req gwmodels.MCPProxyDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateMCPProxy: failed to decode request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// amp.connections.create-mcp-proxy's probed_first dimension: fetch-server-info
	// returns the same prompts/resources/tools shape carried on an endpoint's
	// Capabilities, so an endpoint arriving with Capabilities already set is the
	// signal that the caller probed the server before registering it.
	gwtrack.Set(ctx, "probed_first", mcpProxyWasProbedFirst(req.Endpoints))

	resp, err := c.mcpProxyService.Create(ctx, ouID, "system", &req)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrContextInUse):
			log.Warn("CreateMCPProxy: context in use on the gateway", "ouID", ouID, "error", err)
			httpx.WriteErrorResponseWithReason(w, http.StatusConflict, gwerr.ErrContextInUse.Error(), err.Error(),
				gwerr.ErrCodeContextInUse)
		case errors.Is(err, httpx.ErrInvalidInput), errors.Is(err, ssrf.ErrInvalidURL):
			log.Error("CreateMCPProxy: invalid request", "ouID", ouID, "error", err)
			httpx.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Bad request", err.Error(), httpx.ErrCodeBadRequest)
		case errors.Is(err, gwerr.ErrMCPProxyExists):
			log.Error("CreateMCPProxy: MCP proxy already exists", "ouID", ouID, "id", req.ID)
			httpx.WriteErrorResponse(w, http.StatusConflict, fmt.Sprintf("MCP proxy %q already exists", req.ID))
		case errors.Is(err, gwerr.ErrArtifactExists):
			log.Error("CreateMCPProxy: handle already in use by another artifact", "ouID", ouID, "id", req.ID)
			httpx.WriteErrorResponse(w, http.StatusConflict, fmt.Sprintf("Handle %q is already in use", req.ID))
		case errors.Is(err, gwerr.ErrMCPEnvAlreadyBound):
			log.Error("CreateMCPProxy: environment already bound to another endpoint", "ouID", ouID, "id", req.ID)
			httpx.WriteErrorResponse(w, http.StatusConflict, err.Error())
		default:
			log.Error("CreateMCPProxy: failed", "ouID", ouID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create MCP proxy")
		}
		return
	}

	log.Info("CreateMCPProxy: completed", "ouID", ouID, "id", req.ID)
	httpx.WriteSuccessResponse(w, http.StatusCreated, resp)
}

// ListMCPProxies handles GET /orgs/{ouID}/mcp-proxies.
func (c *mcpProxyController) ListMCPProxies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	limit := getMCPProxyIntQueryParam(r, "limit", gwutil.DefaultLimit)
	offset := getMCPProxyIntQueryParam(r, "offset", gwutil.DefaultOffset)

	if limit < gwutil.MinLimit || limit > gwutil.MaxLimit {
		limit = gwutil.DefaultLimit
	}
	if offset < gwutil.MinOffset {
		offset = gwutil.DefaultOffset
	}

	// ?project=<name> adds that project's proxies to the org-level ones (the agent Tools
	// picker). Without it the MCP proxy page keeps seeing org-level proxies only.
	var projectUUID *string
	if projName := r.URL.Query().Get("project"); projName != "" {
		project, err := gwsvc.LookupProject(ctx, c.ocClient, ouID, projName)
		if err != nil && !errors.Is(err, gwerr.ErrProjectNotFound) {
			log.Error("ListMCPProxies: failed to resolve project", "ouID", ouID, "project", projName, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to resolve project")
			return
		}
		if err != nil || project == nil {
			httpx.WriteErrorResponseWithReason(w, http.StatusNotFound, "Project not found", "", gwerr.ErrCodeProjectNotFound)
			return
		}
		projectUUID = &project.ID
	}

	log.Info("ListMCPProxies: starting", "ouID", ouID, "limit", limit, "offset", offset)

	resp, err := c.mcpProxyService.List(ctx, ouID, projectUUID, limit, offset)
	if err != nil {
		log.Error("ListMCPProxies: failed", "ouID", ouID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list MCP proxies")
		return
	}

	log.Info("ListMCPProxies: completed", "ouID", ouID, "count", resp.Count)
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}

// ListAvailableMCPPolicies handles GET /orgs/{ouID}/mcp-proxies/policies.
func (c *mcpProxyController) ListAvailableMCPPolicies(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())

	log.Info("ListAvailableMCPPolicies: starting", "ouID", ouID)

	resp, err := c.mcpProxyService.ListAvailableMCPPolicies(ctx, ouID)
	if err != nil {
		log.Error("ListAvailableMCPPolicies: failed", "ouID", ouID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list available MCP policies")
		return
	}

	log.Info("ListAvailableMCPPolicies: completed", "ouID", ouID, "count", resp.Count)
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}

// GetMCPProxy handles GET /orgs/{ouID}/mcp-proxies/{proxyId}.
func (c *mcpProxyController) GetMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	proxyID := r.PathValue(gwutil.PathParamProxyId)

	log.Info("GetMCPProxy: starting", "ouID", ouID, "proxyID", proxyID)

	resp, err := c.mcpProxyService.Get(ctx, ouID, proxyID)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrMCPProxyNotFound):
			log.Warn("GetMCPProxy: MCP proxy not found", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, httpx.ErrInvalidInput):
			log.Error("GetMCPProxy: invalid request", "ouID", ouID, "proxyID", proxyID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid MCP proxy id")
		default:
			log.Error("GetMCPProxy: failed", "ouID", ouID, "proxyID", proxyID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get MCP proxy")
		}
		return
	}

	log.Info("GetMCPProxy: completed", "ouID", ouID, "proxyID", proxyID)
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}

// UpdateMCPProxy handles PUT /orgs/{ouID}/mcp-proxies/{proxyId}.
func (c *mcpProxyController) UpdateMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	proxyID := r.PathValue(gwutil.PathParamProxyId)

	log.Info("UpdateMCPProxy: starting", "ouID", ouID, "proxyID", proxyID)

	var req gwmodels.MCPProxyDTO
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateMCPProxy: failed to decode request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	resp, err := c.mcpProxyService.Update(ctx, ouID, proxyID, &req)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrContextInUse):
			log.Warn("UpdateMCPProxy: context in use on the gateway", "ouID", ouID, "error", err)
			httpx.WriteErrorResponseWithReason(w, http.StatusConflict, gwerr.ErrContextInUse.Error(), err.Error(),
				gwerr.ErrCodeContextInUse)
		case errors.Is(err, gwerr.ErrMCPProxyNotFound):
			log.Warn("UpdateMCPProxy: MCP proxy not found", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, gwerr.ErrMCPProxyProjectOwned):
			log.Warn("UpdateMCPProxy: project-owned MCP proxy", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponseWithReason(w, http.StatusConflict, "MCP proxy belongs to a project",
				err.Error(), gwerr.ErrCodeProjectOwned)
		case errors.Is(err, httpx.ErrInvalidInput), errors.Is(err, ssrf.ErrInvalidURL):
			log.Error("UpdateMCPProxy: invalid request", "ouID", ouID, "proxyID", proxyID, "error", err)
			httpx.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Bad request", err.Error(), httpx.ErrCodeBadRequest)
		case errors.Is(err, gwerr.ErrArtifactExists):
			log.Error("UpdateMCPProxy: handle already in use by another artifact", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponse(w, http.StatusConflict, err.Error())
		case errors.Is(err, gwerr.ErrMCPEnvAlreadyBound):
			log.Error("UpdateMCPProxy: environment already bound to another endpoint", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponse(w, http.StatusConflict, err.Error())
		default:
			log.Error("UpdateMCPProxy: failed", "ouID", ouID, "proxyID", proxyID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update MCP proxy")
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
	// — mapping the first egress gateway to an environment resolves the same dead state for
	// every proxy already bound there — and GatewayController.AssignGatewayToEnvironment
	// reconciles that case itself. Promotion self-heals per agent either way.
	//
	// Runs in the background, for the same reasons as the gateway-assignment trigger (see
	// reconcileMCPBindingsInBackground): binding many agents mints a key and injects
	// variables per agent, which can outlast the HTTP write timeout and would then report a
	// committed update as a failure. A caller reading agents straight back may briefly see
	// them still unbound.
	reconcileMCPBindingsInBackground(ctx, log, "UpdateMCPProxy",
		func(reconcileCtx context.Context) error {
			return c.agentConfigService.ReconcileMCPBindingsForProxy(reconcileCtx, ouID, proxyID)
		},
		"ouID", ouID, "proxyID", proxyID)

	log.Info("UpdateMCPProxy: completed", "ouID", ouID, "proxyID", proxyID)
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}

// DeleteMCPProxy handles DELETE /orgs/{ouID}/mcp-proxies/{proxyId}.
func (c *mcpProxyController) DeleteMCPProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	proxyID := r.PathValue(gwutil.PathParamProxyId)

	log.Info("DeleteMCPProxy: starting", "ouID", ouID, "proxyID", proxyID)

	if err := c.mcpProxyService.Delete(ctx, ouID, proxyID); err != nil {
		switch {
		case errors.Is(err, gwerr.ErrMCPProxyNotFound):
			log.Warn("DeleteMCPProxy: MCP proxy not found", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
		case errors.Is(err, gwerr.ErrMCPProxyProjectOwned):
			log.Warn("DeleteMCPProxy: project-owned MCP proxy", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponseWithReason(w, http.StatusConflict, "MCP proxy belongs to a project",
				err.Error(), gwerr.ErrCodeProjectOwned)
		case errors.Is(err, gwerr.ErrMCPProxyHasMappings):
			log.Warn("DeleteMCPProxy: MCP proxy has mappings", "ouID", ouID, "proxyID", proxyID)
			httpx.WriteErrorResponse(w, http.StatusConflict, gwerr.ErrMCPProxyHasMappings.Error())
		case errors.Is(err, httpx.ErrInvalidInput):
			log.Error("DeleteMCPProxy: invalid request", "ouID", ouID, "proxyID", proxyID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid MCP proxy id")
		default:
			log.Error("DeleteMCPProxy: failed", "ouID", ouID, "proxyID", proxyID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete MCP proxy")
		}
		return
	}

	log.Info("DeleteMCPProxy: completed", "ouID", ouID, "proxyID", proxyID)
	httpx.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// FetchServerInfo handles POST /orgs/{ouID}/mcp-proxies/fetch-server-info.
func (c *mcpProxyController) FetchServerInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())

	log.Info("FetchMCPProxyServerInfo: starting", "ouID", ouID)

	var req gwmodels.MCPServerInfoFetchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("FetchMCPProxyServerInfo: failed to decode request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	caller := modkit.PrincipalFrom(ctx).User
	if caller == "" {
		log.Warn("FetchMCPProxyServerInfo: missing token claims")
		httpx.WriteErrorResponse(w, http.StatusUnauthorized, "Missing token claims")
		return
	}

	resp, err := c.mcpProxyService.FetchServerInfo(ctx, ouID, caller, &req)
	if err != nil {
		// The console aborts a superseded probe. Whatever error that surfaced as,
		// nobody is waiting for the response, so it is not a failure to log.
		if errors.Is(ctx.Err(), context.Canceled) {
			log.Debug("FetchMCPProxyServerInfo: client cancelled the probe", "ouID", ouID)
			return
		}
		switch {
		case errors.Is(err, httpx.ErrInvalidInput), errors.Is(err, ssrf.ErrInvalidURL):
			log.Error("FetchMCPProxyServerInfo: invalid request", "ouID", ouID, "error", err)
			httpx.WriteErrorResponseWithReason(w, http.StatusBadRequest, "Bad request", err.Error(), httpx.ErrCodeBadRequest)
		case errors.Is(err, gwerr.ErrURLUnreachable):
			log.Error("FetchMCPProxyServerInfo: MCP server URL is unreachable", "ouID", ouID, "error", err)
			httpx.WriteErrorResponseWithReason(w, http.StatusBadRequest, "MCP server URL is unreachable", err.Error(), httpx.ErrCodeBadRequest)
		case errors.Is(err, gwerr.ErrTooManyRequests):
			log.Warn("FetchMCPProxyServerInfo: concurrency limit reached", "ouID", ouID)
			httpx.WriteErrorResponse(w, http.StatusTooManyRequests, "Too many MCP server probes in progress. Please try again shortly")
		case errors.Is(err, gwerr.ErrMCPServerUnauthorized):
			log.Error("FetchMCPProxyServerInfo: MCP server returned unauthorized", "ouID", ouID, "error", err)
			httpx.WriteErrorResponseWithReason(w, http.StatusUnauthorized, "MCP server returned 401 Unauthorized. Check the provided credentials.", err.Error(), httpx.ErrCodeUnauthorized)
		default:
			log.Error("FetchMCPProxyServerInfo: failed", "ouID", ouID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to fetch MCP server info")
		}
		return
	}

	log.Info("FetchMCPProxyServerInfo: completed", "ouID", ouID)
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}

// mcpProxyWasProbedFirst reports whether any endpoint in the request already
// carries Capabilities (prompts/resources/tools) — the shape
// FetchServerInfo returns — which is the signal that the caller probed the
// MCP server before registering it, for amp.connections.create-mcp-proxy's
// probed_first dimension.
func mcpProxyWasProbedFirst(endpoints []gwmodels.MCPProxyEndpointDTO) bool {
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
