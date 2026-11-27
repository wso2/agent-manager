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
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
)

const (
	// Default limit for pagination
	defaultLimit = 100

	// Default offset for pagination
	defaultOffset = 0

	// mcpBindingReconcileTimeout bounds the MCP binding reconcile after a proxy update, which
	// runs in the background detached from the request context (see
	// UpdateMCPProxy). Detaching removes the caller's cancellation as a stop
	// condition, so the work needs one of its own or a wedged gateway or database call
	// would leak the goroutine indefinitely.
	mcpBindingReconcileTimeout = 2 * time.Minute
)

// reconcileMCPBindingsInBackground runs an MCP binding reconcile off the request path, on a
// context detached from the request and bounded by mcpBindingReconcileTimeout.
//
// The triggering write is already committed and the reconcile writes real resources —
// mapping rows, API keys, injected variables — so a client disconnect must not tear it off
// halfway. It also must not hold the response: across many agents it can outlast the
// server's HTTP write timeout, and a response dropped after the write committed shows the
// client an error for something that succeeded. Its result is only ever logged.
//
// A panic is recovered and logged: the panic-recovery middleware guards only the request
// goroutine, so one escaping here would take the whole service down.
func reconcileMCPBindingsInBackground(
	ctx context.Context, log *slog.Logger, op string, reconcile func(context.Context) error, attrs ...any,
) {
	reconcileCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), mcpBindingReconcileTimeout)
	go func() {
		defer cancel()
		defer func() {
			if rec := recover(); rec != nil {
				log.Error(op+": MCP binding reconcile panicked", append(attrs, "panic", rec)...)
			}
		}()
		if err := reconcile(reconcileCtx); err != nil {
			log.Warn(op+": failed to reconcile agent MCP bindings", append(attrs, "error", err)...)
		}
	}()
}

// GatewayController defines interface for gateway HTTP handlers
type GatewayController interface {
	RegisterGateway(w http.ResponseWriter, r *http.Request)
	GetGateway(w http.ResponseWriter, r *http.Request)
	ListGateways(w http.ResponseWriter, r *http.Request)
	UpdateGateway(w http.ResponseWriter, r *http.Request)
	DeleteGateway(w http.ResponseWriter, r *http.Request)
	CheckGatewayHealth(w http.ResponseWriter, r *http.Request)
	ListGatewayTokens(w http.ResponseWriter, r *http.Request)
	RotateGatewayToken(w http.ResponseWriter, r *http.Request)
	RevokeGatewayToken(w http.ResponseWriter, r *http.Request)
	GetGatewayStatus(w http.ResponseWriter, r *http.Request)

	// Cross-organization, platform-admin only. Not org-scoped like everything
	// else on this interface — see the handler.
	GetPlatformGatewayFailureSummary(w http.ResponseWriter, r *http.Request)

	// Identity provider handlers
	ListIdentityProviders(w http.ResponseWriter, r *http.Request)
	ListGatewayIdentityProviders(w http.ResponseWriter, r *http.Request)
	UpsertGatewayIdentityProvider(w http.ResponseWriter, r *http.Request)
	DeleteGatewayIdentityProvider(w http.ResponseWriter, r *http.Request)
	DiscoverOidcConfiguration(w http.ResponseWriter, r *http.Request)
}

type gatewayController struct {
	gatewayService     *gwsvc.PlatformGatewayService
	ocClient           modkit.CoreAPI
	agentConfigService gwsvc.AgentConfigurationService
}

// NewGatewayController creates a new gateway controller
func NewGatewayController(
	gatewayService *gwsvc.PlatformGatewayService,
	ocClient modkit.CoreAPI,
	agentConfigService gwsvc.AgentConfigurationService,
) GatewayController {
	return &gatewayController{
		gatewayService:     gatewayService,
		ocClient:           ocClient,
		agentConfigService: agentConfigService,
	}
}

func handleGatewayErrors(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, gwerr.ErrGatewayNotFound):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
	case errors.Is(err, gwerr.ErrGatewayManaged):
		httpx.WriteErrorResponse(w, http.StatusForbidden, "A managed gateway is read-only")
	case errors.Is(err, gwerr.ErrGatewayAlreadyExists):
		httpx.WriteErrorResponse(w, http.StatusConflict, "A gateway with this name already exists")
	case errors.Is(err, gwerr.ErrGatewayHasDeployments):
		httpx.WriteErrorResponse(w, http.StatusConflict, err.Error())
	case errors.Is(err, gwerr.ErrGatewayIngressCapExceeded):
		httpx.WriteErrorResponse(w, http.StatusConflict, err.Error())
	case errors.Is(err, gwerr.ErrGatewayEnvironmentFixed):
		httpx.WriteErrorResponseWithReason(w, http.StatusConflict, err.Error(), "", gwerr.ErrCodeGatewayEnvironmentFixed)
	case errors.Is(err, gwerr.ErrGatewayTokenLimitReached):
		httpx.WriteErrorResponse(w, http.StatusConflict, err.Error())
	case errors.Is(err, gwerr.ErrGatewayTokenNotFound):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Token not found")
	case errors.Is(err, httpx.ErrBadRequest):
		httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, gwerr.ErrEnvironmentNotFound):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
	case httpx.IsValidationError(err) != nil:
		httpx.WriteValidationErrorResponse(w, err)
	case errors.Is(err, httpx.ErrInvalidInput):
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
	case errors.Is(err, gorm.ErrRecordNotFound):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Resource not found")
	default:
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, fallbackMsg)
	}
}

// refusedForeignBodyOrg writes a 403, which the route's audit record shows as
// a denial with its reason, and reports true when a request body's optional orgId names an org other than
// the caller's. The org always comes from the token; the body field is
// accepted only so that existing clients that send it keep working.
func refusedForeignBodyOrg(w http.ResponseWriter, r *http.Request, bodyOrg *string, callerOUID string) bool {
	if bodyOrg == nil || *bodyOrg == "" || strings.EqualFold(*bodyOrg, callerOUID) {
		return false
	}
	modkit.AuditDetail(r.Context(), "reason", "body-org-mismatch")
	modkit.AuditDetail(r.Context(), "requestedOrgId", *bodyOrg)
	httpx.WriteErrorResponse(w, http.StatusForbidden, "orgId does not match the caller's organization")
	return true
}

// RegisterGateway registers a self-hosted gateway, from its document, in the organization's default environment.
func (c *gatewayController) RegisterGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(ctx)

	var doc gwspec.GatewayDocument
	if err := json.NewDecoder(io.LimitReader(r.Body, maxDocumentBytes)).Decode(&doc); err != nil {
		log.Error("RegisterGateway: failed to decode the document", "ouID", ouID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	input, err := selfHostedFromDocument(doc, "")
	if err != nil {
		writeDocumentError(w, ctx, "RegisterGateway", ouID, err)
		return
	}
	env, err := gwsvc.LookupEnvironment(ctx, c.ocClient, ouID, gwmodels.DefaultEnvironmentName)
	if err != nil {
		log.Error("RegisterGateway: failed to resolve the default environment", "ouID", ouID, "error", err)
		handleGatewayErrors(w, err, "Failed to resolve the default environment")
		return
	}
	input.Environment = env.ID

	gateway, err := c.gatewayService.RegisterSelfHostedGateway(ouID, input)
	vhost := ""
	if gateway != nil {
		vhost = gateway.Vhost
	}
	modkit.AuditResource(ctx, gwaudit.ResourceGateway, gatewayResponseID(gateway), input.DisplayName)
	modkit.AuditDetail(ctx, "gatewayName", input.DisplayName)
	modkit.AuditDetail(ctx, "gatewayType", gwsvc.SelfHostedFunctionalityType(input.FunctionalityType))
	modkit.AuditDetail(ctx, "vhost", vhost)
	if err != nil {
		log.Error("RegisterGateway: failed to register gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to register gateway")
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusCreated, gatewayDocument(gateway))
}

// GetGateway returns a gateway, managed or self-hosted, by name or UUID.
func (c *gatewayController) GetGateway(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	ouID := modkit.OrgID(r.Context())
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	gateway, err := c.gatewayService.GetGateway(gatewayID, ouID)
	if err != nil {
		log.Error("GetGateway: failed to get gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway")
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, gatewayDocument(gateway))
}

// ListGateways lists the organization's gateways, filtered by type, status and hosting.
func (c *gatewayController) ListGateways(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	ouID := modkit.OrgID(r.Context())
	limit, offset := gatewayPagination(r)

	// Alias normalization (REGULAR/AI) and lowercasing happen in the service layer.
	filters := &gwsvc.GatewayListFilters{}
	query := r.URL.Query()
	if typeParam := query.Get("type"); typeParam != "" {
		filters.FunctionalityType = &typeParam
	}
	if statusParam := query.Get("status"); statusParam != "" {
		isActive := statusParam == string(gwspec.GATEWAYSTATUS_ACTIVE)
		filters.Status = &isActive
	}
	switch hosting := query.Get("hosting"); hosting {
	case "":
	case string(gwspec.GATEWAYHOSTING_MANAGED):
		filters.Hosting = gwmodels.GatewayHostingManaged
	case string(gwspec.GATEWAYHOSTING_SELF_HOSTED):
		filters.Hosting = gwmodels.GatewayHostingSelfHosted
	default:
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "hosting must be MANAGED or SELF_HOSTED")
		return
	}

	page, err := c.gatewayService.ListGateways(&ouID, filters, limit, offset)
	if err != nil {
		log.Error("ListGateways: failed to list gateways", "error", err)
		handleGatewayErrors(w, err, "Failed to list gateways")
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, gatewayList(page, limit, offset))
}

// UpdateGateway replaces the editable fields of a self-hosted gateway from its document.
func (c *gatewayController) UpdateGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(ctx)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	var doc gwspec.GatewayDocument
	if err := json.NewDecoder(io.LimitReader(r.Body, maxDocumentBytes)).Decode(&doc); err != nil {
		log.Error("UpdateGateway: failed to decode the document", "ouID", ouID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	modkit.AuditResource(ctx, gwaudit.ResourceGateway, gatewayID, "")
	existing, err := c.gatewayService.RequireSelfHosted(gatewayID, ouID)
	if err != nil {
		log.Warn("UpdateGateway: gateway cannot be updated", "gatewayID", gatewayID, "error", err)
		handleGatewayErrors(w, err, "Failed to update gateway")
		return
	}
	modkit.AuditResource(ctx, gwaudit.ResourceGateway, existing.UUID.String(), "")
	upd, err := gatewayUpdateFromDocument(doc, existing)
	if err != nil {
		writeDocumentError(w, ctx, "UpdateGateway", ouID, err)
		return
	}

	gateway, err := c.gatewayService.UpdateGateway(existing.UUID.String(), ouID, &upd.Description, &upd.DisplayName,
		&upd.IsCritical, &upd.Properties, nil)
	if err != nil {
		log.Error("UpdateGateway: failed to update gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to update gateway")
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, gatewayDocument(gateway))
}

// DeleteGateway deregisters a self-hosted gateway.
func (c *gatewayController) DeleteGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(ctx)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	modkit.AuditResource(ctx, gwaudit.ResourceGateway, gatewayID, "")
	existing, err := c.gatewayService.RequireSelfHosted(gatewayID, ouID)
	if err != nil {
		log.Warn("DeleteGateway: gateway cannot be deleted", "gatewayID", gatewayID, "error", err)
		handleGatewayErrors(w, err, "Failed to delete gateway")
		return
	}
	modkit.AuditResource(ctx, gwaudit.ResourceGateway, existing.UUID.String(), "")
	if err := c.gatewayService.DeleteGateway(existing.UUID.String(), ouID); err != nil {
		log.Error("DeleteGateway: failed to delete gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to delete gateway")
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

// gatewayPagination reads limit and offset, falling back to the defaults for negative
// values and capping the page size.
func gatewayPagination(r *http.Request) (limit, offset int) {
	limit = getIntQueryParam(r, "limit", defaultLimit)
	offset = getIntQueryParam(r, "offset", defaultOffset)
	if limit < 0 {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}
	if limit > 1000 {
		limit = 1000
	}
	return limit, offset
}

func (c *gatewayController) CheckGatewayHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	// Get gateway to check if it exists
	gateway, err := c.gatewayService.GetGateway(gatewayID, ouID)
	if err != nil {
		log.Error("CheckGatewayHealth: gateway not found", "error", err)
		handleGatewayErrors(w, err, "Failed to check gateway health")
		return
	}

	// Return health based on gateway's active status
	status := "healthy"
	if !gateway.IsActive {
		status = "unhealthy"
	}

	response := map[string]interface{}{
		"gatewayId": gatewayID,
		"status":    status,
		"checkedAt": gateway.UpdatedAt,
	}

	httpx.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) ListGatewayTokens(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	log.Info("ListGatewayTokens: starting", "ouID", ouID, "gatewayID", gatewayID)

	svcResp, err := c.gatewayService.ListTokens(gatewayID, ouID)
	if err != nil {
		log.Error("ListGatewayTokens: failed to list tokens", "error", err)
		handleGatewayErrors(w, err, "Failed to list gateway tokens")
		return
	}

	// Map service DTOs to spec types
	tokenInfos := make([]gwspec.GatewayTokenInfo, 0, len(svcResp.List))
	for _, t := range svcResp.List {
		info := gwspec.NewGatewayTokenInfo(t.ID, t.Status, t.CreatedAt)
		if t.RevokedAt != nil {
			info.SetRevokedAt(*t.RevokedAt)
		} else {
			info.SetRevokedAtNil()
		}
		tokenInfos = append(tokenInfos, *info)
	}

	response := gwspec.NewGatewayTokenListResponse(int32(svcResp.Count), tokenInfos)
	httpx.WriteSuccessResponse(w, http.StatusOK, response)
}

// RotateGatewayToken issues a new token for the gateway.
func (c *gatewayController) RotateGatewayToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	// The request body is optional; its only field, orgId, may name only the
	// caller's own org.
	if r.Body != nil && r.ContentLength != 0 {
		var req gwspec.RotateGatewayTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Error("RotateGatewayToken: failed to decode request", "error", err)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
			return
		}
		if refusedForeignBodyOrg(w, r, req.OrgId, ouID) {
			return
		}
	}

	// Named here rather than in the service because RotateToken takes no
	// context. A rotated gateway token is live credential material, so the
	// route is fail-closed.
	modkit.AuditResource(ctx, gwaudit.ResourceGateway, gatewayID, "")

	// Call service to rotate the token
	tokenResp, err := c.gatewayService.RotateToken(gatewayID, ouID)
	if err != nil {
		log.Error("RotateGatewayToken: failed to rotate token", "error", err)
		handleGatewayErrors(w, err, "Failed to rotate gateway token")
		return
	}
	modkit.AuditDetail(ctx, "tokenId", tokenResp.ID)

	// Convert to spec response
	response := gwspec.GatewayTokenResponse{
		GatewayId: gatewayID,
		Token:     tokenResp.Token,
		TokenId:   tokenResp.ID,
		CreatedAt: tokenResp.CreatedAt,
		ExpiresAt: nil, // Token doesn't have expiry in current implementation
	}

	httpx.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *gatewayController) RevokeGatewayToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))
	tokenID := strings.TrimSpace(r.PathValue("tokenID"))

	log.Info("RevokeGatewayToken: starting", "ouID", ouID, "gatewayID", gatewayID, "tokenID", tokenID)

	modkit.AuditResource(ctx, gwaudit.ResourceGateway, gatewayID, "")
	modkit.AuditDetail(ctx, "tokenId", tokenID)

	if err := c.gatewayService.RevokeTokenByID(tokenID, gatewayID, ouID); err != nil {
		log.Error("RevokeGatewayToken: failed to revoke token", "error", err)
		handleGatewayErrors(w, err, "Failed to revoke token")
		return
	}

	log.Info("RevokeGatewayToken: token revoked successfully", "ouID", ouID, "gatewayID", gatewayID, "tokenID", tokenID)
	w.WriteHeader(http.StatusNoContent)
}

func (c *gatewayController) GetGatewayStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())

	// Parse optional gatewayID query parameter
	gatewayIDParam := r.URL.Query().Get("gatewayId")
	var gatewayIDPtr *string
	if gatewayIDParam != "" {
		gatewayIDPtr = &gatewayIDParam
	}

	statusResp, err := c.gatewayService.GetGatewayStatus(ctx, ouID, gatewayIDPtr)
	if err != nil {
		log.Error("GetGatewayStatus: failed to get status", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway status")
		return
	}

	httpx.WriteSuccessResponse(w, http.StatusOK, statusResp)
}

// Helper conversion functions

// canonicalGatewayType uppercases the stored lowercase role for the wire enum.
func canonicalGatewayType(role string) gwspec.GatewayType {
	return gwspec.GatewayType(strings.ToUpper(role))
}

// gatewayResponseID returns a registered gateway's identifier for an audit
// record, tolerating a nil gateway so a failed registration still records the
// attempt. Named to avoid reading like the many local gatewayUUID variables in
// this package, which hold a different thing.
func gatewayResponseID(gateway *gwsvc.GatewayResponse) string {
	if gateway == nil {
		return ""
	}
	return gateway.ID
}
