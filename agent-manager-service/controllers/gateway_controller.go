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
	"strings"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	occlient "github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	// Default limit for pagination
	defaultLimit = 100

	// Default offset for pagination
	defaultOffset = 0
)

// GatewayController defines interface for gateway HTTP handlers
type GatewayController interface {
	RegisterGateway(w http.ResponseWriter, r *http.Request)
	GetGateway(w http.ResponseWriter, r *http.Request)
	ListGateways(w http.ResponseWriter, r *http.Request)
	UpdateGateway(w http.ResponseWriter, r *http.Request)
	DeleteGateway(w http.ResponseWriter, r *http.Request)
	AssignGatewayToEnvironment(w http.ResponseWriter, r *http.Request)
	RemoveGatewayFromEnvironment(w http.ResponseWriter, r *http.Request)
	GetGatewayEnvironments(w http.ResponseWriter, r *http.Request)
	CheckGatewayHealth(w http.ResponseWriter, r *http.Request)
	ListGatewayTokens(w http.ResponseWriter, r *http.Request)
	RotateGatewayToken(w http.ResponseWriter, r *http.Request)
	RevokeGatewayToken(w http.ResponseWriter, r *http.Request)
	GetGatewayStatus(w http.ResponseWriter, r *http.Request)

	// Identity provider handlers
	ListIdentityProviders(w http.ResponseWriter, r *http.Request)
	ListGatewayIdentityProviders(w http.ResponseWriter, r *http.Request)
	UpsertGatewayIdentityProvider(w http.ResponseWriter, r *http.Request)
	DeleteGatewayIdentityProvider(w http.ResponseWriter, r *http.Request)
	ListEnvironmentIdentityProviders(w http.ResponseWriter, r *http.Request)
	DiscoverOidcConfiguration(w http.ResponseWriter, r *http.Request)
}

type gatewayController struct {
	gatewayService *services.PlatformGatewayService
	ocClient       occlient.OpenChoreoClient
}

// NewGatewayController creates a new gateway controller
func NewGatewayController(
	gatewayService *services.PlatformGatewayService,
	ocClient occlient.OpenChoreoClient,
) GatewayController {
	return &gatewayController{
		gatewayService: gatewayService,
		ocClient:       ocClient,
	}
}

// resolveEnvironmentUUID resolves an environment name or UUID to the UUID of an
// environment in ouID. A UUID is looked up like any other identifier rather than
// trusted on shape: a well-formed UUID belonging to another organization is as
// unknown to this caller as a typo, and returning it unchecked let a cross-org
// value through as a list filter and as the target of an environment assignment.
func (c *gatewayController) resolveEnvironmentUUID(ctx context.Context, ouID, envIdentifier string) (string, error) {
	environments, err := c.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		return "", fmt.Errorf("failed to list environments: %w", err)
	}

	for _, env := range environments {
		if env.Name == envIdentifier || strings.EqualFold(env.UUID, envIdentifier) {
			return env.UUID, nil
		}
	}

	return "", fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, envIdentifier)
}

func handleGatewayErrors(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, utils.ErrGatewayNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
	case errors.Is(err, utils.ErrGatewayAlreadyExists):
		utils.WriteErrorResponse(w, http.StatusConflict, "Gateway already exists")
	case errors.Is(err, utils.ErrGatewayHasDeployments):
		utils.WriteErrorResponse(w, http.StatusConflict, err.Error())
	case errors.Is(err, utils.ErrGatewayIngressCapExceeded):
		utils.WriteErrorResponse(w, http.StatusConflict, err.Error())
	case errors.Is(err, utils.ErrBadRequest):
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, utils.ErrEnvironmentNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
	case errors.Is(err, utils.ErrInvalidInput):
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
	case errors.Is(err, gorm.ErrRecordNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Resource not found")
	default:
		utils.WriteErrorResponse(w, http.StatusInternalServerError, fallbackMsg)
	}
}

func (c *gatewayController) RegisterGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	var req spec.CreateGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("RegisterGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Use the OU ID from the request body if provided, otherwise fall back to the one from the request context.
	ouID := middleware.OUIDFromRequest(r)
	if req.OrgId != nil && *req.OrgId != "" {
		ouID = *req.OrgId
		// Re-stash the effective org so downstream context consumers (e.g. the
		// OpenChoreo client's org header) see the org actually operated on
		// rather than the caller's token org.
		org, _ := middleware.GetResolvedOrg(ctx)
		org.OUID = ouID
		ctx = middleware.WithResolvedOrg(ctx, org)
	}

	// Validate environments if present
	if len(req.EnvironmentIds) > 0 {
		envs, err := c.ocClient.ListEnvironments(ctx, ouID)
		if err != nil {
			log.Error("environment validation failed: failed to list environments")
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "environment validation erro")
			return
		}
		if len(envs) == 0 {
			utils.WriteErrorResponse(w, http.StatusBadRequest, "no environments registered")
			return
		}
		envMap := make(map[string]string)
		for _, env := range envs {
			envMap[env.UUID] = env.Name
		}
		for _, envId := range req.EnvironmentIds {
			if _, ok := envMap[envId]; !ok {
				log.Warn("environment validation failed: environment not found", "env_id", envId)
				utils.WriteErrorResponse(w, http.StatusBadRequest, "environment validation failed")
				return
			}
		}
	}

	// Create gateway using local service
	description := "" // Description not in spec, use empty string
	functionalityType := string(req.GatewayType)
	isCritical := false
	if req.IsCritical != nil {
		isCritical = *req.IsCritical
	}
	var properties map[string]interface{}

	runtimeURL := ""
	if req.RuntimeUrl != nil {
		runtimeURL = *req.RuntimeUrl
	}

	gateway, err := c.gatewayService.RegisterGateway(
		ouID,
		req.Name,
		req.DisplayName,
		description,
		req.Vhost,
		runtimeURL,
		isCritical,
		functionalityType,
		properties,
		req.EnvironmentIds,
	)
	audit.Record(
		ctx, audit.ActionGatewayCreate,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceGateway, gatewayResponseID(gateway), req.Name),
		audit.Detail("gatewayName", req.Name),
		audit.Detail("gatewayType", functionalityType),
		audit.Detail("vhost", req.Vhost),
		audit.Result(err),
	)
	if err != nil {
		log.Error("RegisterGateway: failed to create gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to register gateway")
		return
	}

	// Get environments for response
	environments := c.getGatewayEnvironmentsFromDB(ctx, ouID, gateway.ID)

	// Convert to spec response
	response := convertGatewayToSpecResponse(gateway, ouID, environments)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *gatewayController) GetGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	// Get gateway from local service
	gateway, err := c.gatewayService.GetGateway(gatewayID, ouID)
	if err != nil {
		log.Error("GetGateway: failed to get gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway")
		return
	}

	// Key the mapping lookup off the resolved UUID: gatewayID may be a name, and
	// environment mappings are indexed by UUID only.
	environments := c.getGatewayEnvironmentsFromDB(ctx, ouID, gateway.ID)

	response := convertGatewayToSpecResponse(gateway, ouID, environments)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) ListGateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)

	// Parse and validate pagination parameters
	limit := getIntQueryParam(r, "limit", defaultLimit)
	offset := getIntQueryParam(r, "offset", defaultOffset)

	// Validate pagination params to prevent panic
	if limit < 0 {
		limit = defaultLimit
	}
	if offset < 0 {
		offset = 0
	}
	if limit > 1000 {
		limit = 1000 // Cap maximum page size
	}

	// Parse filter parameters
	filters := &services.GatewayListFilters{}

	// Filter by type (functionality type). Alias normalization (REGULAR/AI) and
	// lowercasing happen in the service layer via normalizeGatewayRole.
	if typeParam := r.URL.Query().Get("type"); typeParam != "" {
		filters.FunctionalityType = &typeParam
	}

	// Filter by status
	if statusParam := r.URL.Query().Get("status"); statusParam != "" {
		isActive := statusParam == "ACTIVE"
		filters.Status = &isActive
	}

	// Filter by environment. envParam may be a UUID or a name; an unresolvable one
	// is a client error, not a reason to drop the filter — silently returning the
	// unfiltered list makes a typo look like "every gateway matches".
	if envParam := r.URL.Query().Get("environment"); envParam != "" {
		envUUID, err := c.resolveEnvironmentUUID(ctx, ouID, envParam)
		switch {
		case errors.Is(err, utils.ErrEnvironmentNotFound):
			// A 400 rather than a 404: the collection exists, the filter value does
			// not. listGateways declares 400 but no 404.
			log.Error("ListGateways: unknown environment filter", "environment", envParam)
			utils.WriteErrorResponse(w, http.StatusBadRequest,
				fmt.Sprintf("Unknown environment %q in 'environment' filter", envParam))
			return
		case err != nil:
			log.Error("ListGateways: failed to resolve environment", "environment", envParam, "error", err)
			handleGatewayErrors(w, err, "Failed to resolve environment filter")
			return
		}
		filters.EnvironmentID = &envUUID
	}

	// Get gateways from local service with filters and DB-level pagination
	gatewaysResp, err := c.gatewayService.ListGateways(&ouID, filters, limit, offset)
	if err != nil {
		log.Error("ListGateways: failed to list gateways", "error", err)
		handleGatewayErrors(w, err, "Failed to list gateways")
		return
	}

	// Fetch OpenChoreo environments ONCE for the entire organization (not per-gateway)
	ocEnvironments, err := c.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		log.Warn("ListGateways: failed to list environments from OpenChoreo", "error", err)
		ocEnvironments = nil // Continue with empty environments
	}

	// Bulk fetch environment mappings for all gateways to avoid N+1 query
	gatewayIDs := make([]string, len(gatewaysResp.List))
	for i, gw := range gatewaysResp.List {
		gatewayIDs[i] = gw.ID
	}
	allMappings := c.getGatewayEnvironmentMappingsBulk(ctx, gatewayIDs)

	// Convert to spec responses with pre-fetched environment data
	specGateways := make([]spec.GatewayResponse, 0, len(gatewaysResp.List))
	for _, gw := range gatewaysResp.List {
		// Use pre-fetched mappings and environments (no additional DB/RPC calls)
		environments := c.matchGatewayEnvironments(allMappings[gw.ID], ocEnvironments, ouID)
		specGateways = append(specGateways, convertGatewayToSpecResponse(&gw, ouID, environments))
	}

	response := spec.GatewayListResponse{
		Gateways: specGateways,
		Total:    int32(gatewaysResp.Pagination.Total),
		Limit:    int32(limit),
		Offset:   int32(offset),
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) UpdateGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	var req spec.UpdateGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("UpdateGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Update using local service
	var properties *map[string]interface{}
	var description *string // Description not in spec
	gateway, err := c.gatewayService.UpdateGateway(gatewayID, ouID, description, req.DisplayName, req.IsCritical, properties, req.RuntimeUrl)
	audit.Record(
		ctx, audit.ActionGatewayUpdate,
		audit.Org(ouID),
		audit.Resource(audit.ResourceGateway, gatewayID),
		audit.Result(err),
	)
	if err != nil {
		log.Error("UpdateGateway: failed to update gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to update gateway")
		return
	}

	// Key the mapping lookup off the resolved UUID rather than the raw path value.
	environments := c.getGatewayEnvironmentsFromDB(ctx, ouID, gateway.ID)

	response := convertGatewayToSpecResponse(gateway, ouID, environments)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) DeleteGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	attempt, ok := beginAuditOrFail(
		w, r, "DeleteGateway", "Failed to delete gateway", audit.ActionGatewayDelete,
		audit.Org(ouID),
		audit.Resource(audit.ResourceGateway, gatewayID),
	)
	if !ok {
		return
	}

	if err := c.gatewayService.DeleteGateway(ctx, gatewayID, ouID); err != nil {
		attempt.Complete(ctx, err)
		log.Error("DeleteGateway: failed to delete gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to delete gateway")
		return
	}
	attempt.Complete(ctx, nil)

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

func (c *gatewayController) AssignGatewayToEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))
	envID := strings.TrimSpace(r.PathValue("envID"))

	// Verify gateway exists
	if _, err := c.gatewayService.GetGateway(gatewayID, ouID); err != nil {
		log.Error("AssignGatewayToEnvironment: gateway not found", "error", err)
		handleGatewayErrors(w, err, "Failed to assign gateway")
		return
	}

	resolvedEnvID, err := c.resolveEnvironmentUUID(ctx, ouID, envID)
	if err != nil {
		log.Error("AssignGatewayToEnvironment: failed to resolve environment", "env_id", envID, "error", err)
		handleGatewayErrors(w, err, "Failed to resolve environment")
		return
	}

	// Assign via service
	assignErr := c.gatewayService.AssignGatewayToEnvironment(gatewayID, resolvedEnvID)
	audit.Record(
		ctx, audit.ActionGatewayAssignEnvironment,
		audit.Org(ouID),
		audit.Resource(audit.ResourceGateway, gatewayID),
		audit.Environment(resolvedEnvID),
		audit.Detail("environment", resolvedEnvID),
		audit.Result(assignErr),
	)
	if err := assignErr; err != nil {
		log.Error("AssignGatewayToEnvironment: failed to assign", "error", err)
		handleGatewayErrors(w, err, "Failed to assign gateway to environment")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, map[string]string{"message": "Gateway assigned successfully"})
}

func (c *gatewayController) RemoveGatewayFromEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))
	envID := strings.TrimSpace(r.PathValue("envID"))

	// Verify gateway exists and belongs to the caller's org
	if _, err := c.gatewayService.GetGateway(gatewayID, ouID); err != nil {
		log.Error("RemoveGatewayFromEnvironment: gateway not found", "error", err)
		handleGatewayErrors(w, err, "Failed to remove gateway from environment")
		return
	}

	resolvedEnvID, err := c.resolveEnvironmentUUID(ctx, ouID, envID)
	if err != nil {
		log.Error("RemoveGatewayFromEnvironment: failed to resolve environment", "env_id", envID, "error", err)
		handleGatewayErrors(w, err, "Failed to resolve environment")
		return
	}

	// Remove via service
	removeErr := c.gatewayService.RemoveGatewayFromEnvironment(gatewayID, resolvedEnvID)
	audit.Record(
		ctx, audit.ActionGatewayUnassignEnvironment,
		audit.Org(ouID),
		audit.Resource(audit.ResourceGateway, gatewayID),
		audit.Environment(resolvedEnvID),
		audit.Detail("environment", resolvedEnvID),
		audit.Result(removeErr),
	)
	if err := removeErr; err != nil {
		log.Warn("RemoveGatewayFromEnvironment: failed to remove mapping", "error", err)
		if errors.Is(err, gorm.ErrRecordNotFound) || strings.Contains(err.Error(), "mapping not found") {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway-environment mapping not found")
			return
		}
		if strings.Contains(err.Error(), "invalid") {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to remove gateway from environment")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

func (c *gatewayController) GetGatewayEnvironments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	// Get environments from DB (via OpenChoreo)
	environments := c.getGatewayEnvironmentsFromDB(ctx, ouID, gatewayID)

	// Convert to spec responses
	specEnvs := make([]spec.GatewayEnvironmentResponse, len(environments))
	for i, env := range environments {
		specEnvs[i] = convertGatewayEnvironmentToSpecResponse(&env)
	}

	response := spec.GetGatewayEnvironments200Response{
		Environments: specEnvs,
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) CheckGatewayHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
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

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) ListGatewayTokens(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	log.Info("ListGatewayTokens: starting", "ou_id", ouID, "gateway_id", gatewayID)

	svcResp, err := c.gatewayService.ListTokens(gatewayID, ouID)
	if err != nil {
		log.Error("ListGatewayTokens: failed to list tokens", "error", err)
		handleGatewayErrors(w, err, "Failed to list gateway tokens")
		return
	}

	// Map service DTOs to spec types
	tokenInfos := make([]spec.GatewayTokenInfo, 0, len(svcResp.List))
	for _, t := range svcResp.List {
		info := spec.NewGatewayTokenInfo(t.ID, t.Status, t.CreatedAt)
		if t.RevokedAt != nil {
			info.SetRevokedAt(*t.RevokedAt)
		} else {
			info.SetRevokedAtNil()
		}
		tokenInfos = append(tokenInfos, *info)
	}

	response := spec.NewGatewayTokenListResponse(int32(svcResp.Count), tokenInfos)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) RotateGatewayToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	// The request body is optional; decode it only if one was sent so that
	// callers can still rotate without providing an explicit org.
	if r.Body != nil && r.ContentLength != 0 {
		var req spec.RotateGatewayTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			log.Warn("RotateGatewayToken: failed to decode request", "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
			return
		}

		// Use the OU ID from the request body if provided, otherwise fall back to the one from the request context.
		if req.OrgId != nil && *req.OrgId != "" {
			ouID = *req.OrgId
		}
	}

	// Emitted here rather than in the service because RotateToken takes no
	// context; the audit trail needs the caller identity that only the request
	// context carries. A rotated gateway token is live credential material, so
	// the operation is refused when it cannot be recorded.
	attempt, ok := beginAuditOrFail(
		w, r, "RotateGatewayToken", "Failed to rotate gateway token", audit.ActionGatewayTokenRotate,
		audit.Org(ouID),
		audit.Resource(audit.ResourceGateway, gatewayID),
	)
	if !ok {
		return
	}

	// Call service to rotate the token
	tokenResp, err := c.gatewayService.RotateToken(gatewayID, ouID)
	if err != nil {
		attempt.Complete(ctx, err)
		log.Error("RotateGatewayToken: failed to rotate token", "error", err)
		handleGatewayErrors(w, err, "Failed to rotate gateway token")
		return
	}
	attempt.Complete(ctx, nil, audit.Detail("tokenId", tokenResp.ID))

	// Convert to spec response
	response := spec.GatewayTokenResponse{
		GatewayId: gatewayID,
		Token:     tokenResp.Token,
		TokenId:   tokenResp.ID,
		CreatedAt: tokenResp.CreatedAt,
		ExpiresAt: nil, // Token doesn't have expiry in current implementation
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) RevokeGatewayToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))
	tokenID := strings.TrimSpace(r.PathValue("tokenID"))

	log.Info("RevokeGatewayToken: starting", "ou_id", ouID, "gateway_id", gatewayID, "token_id", tokenID)

	attempt, ok := beginAuditOrFail(
		w, r, "RevokeGatewayToken", "Failed to revoke token", audit.ActionGatewayTokenRevoke,
		audit.Org(ouID),
		audit.Resource(audit.ResourceGateway, gatewayID),
		audit.Detail("tokenId", tokenID),
	)
	if !ok {
		return
	}

	if err := c.gatewayService.RevokeTokenByID(ctx, tokenID, gatewayID, ouID); err != nil {
		attempt.Complete(ctx, err)
		log.Warn("RevokeGatewayToken: failed to revoke token", "error", err)
		switch {
		case errors.Is(err, utils.ErrGatewayNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
		default:
			errMsg := err.Error()
			if strings.Contains(errMsg, "token not found") || strings.Contains(errMsg, "does not belong") {
				utils.WriteErrorResponse(w, http.StatusNotFound, "Token not found")
				return
			}
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to revoke token")
		}
		return
	}
	attempt.Complete(ctx, nil)

	log.Info("RevokeGatewayToken: token revoked successfully", "ou_id", ouID, "gateway_id", gatewayID, "token_id", tokenID)
	w.WriteHeader(http.StatusNoContent)
}

func (c *gatewayController) GetGatewayStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)

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

	utils.WriteSuccessResponse(w, http.StatusOK, statusResp)
}

// getGatewayEnvironmentsFromDB retrieves environments associated with a gateway
// Fetches environment UUIDs from service layer, then gets environment details from OpenChoreo
func (c *gatewayController) getGatewayEnvironmentsFromDB(ctx context.Context, ouID, gatewayID string) []models.GatewayEnvironmentResponse {
	log := logger.GetLogger(ctx)

	// Get environment mappings via service
	mappings, err := c.gatewayService.GetGatewayEnvironmentMappings(gatewayID)
	if err != nil {
		log.Warn("getGatewayEnvironmentsFromDB: failed to get environment mappings", "error", err)
		return []models.GatewayEnvironmentResponse{}
	}

	if len(mappings) == 0 {
		return []models.GatewayEnvironmentResponse{}
	}

	// Fetch all environments from OpenChoreo for this organization
	ocEnvironments, err := c.ocClient.ListEnvironments(ctx, ouID)
	if err != nil {
		log.Warn("getGatewayEnvironmentsFromDB: failed to list environments from OpenChoreo", "error", err)
		return []models.GatewayEnvironmentResponse{}
	}

	return c.matchGatewayEnvironments(mappings, ocEnvironments, ouID)
}

// getGatewayEnvironmentMappingsBulk retrieves environment mappings for multiple gateways in bulk
// This avoids N+1 DB queries when listing gateways
func (c *gatewayController) getGatewayEnvironmentMappingsBulk(ctx context.Context, gatewayIDs []string) map[string][]models.GatewayEnvironmentMapping {
	if len(gatewayIDs) == 0 {
		return make(map[string][]models.GatewayEnvironmentMapping)
	}

	log := logger.GetLogger(ctx)

	// Bulk fetch all mappings in a single query
	allMappings, err := c.gatewayService.GetGatewayEnvironmentMappingsBulk(gatewayIDs)
	if err != nil {
		log.Warn("getGatewayEnvironmentMappingsBulk: failed to get environment mappings", "error", err)
		return make(map[string][]models.GatewayEnvironmentMapping)
	}

	return allMappings
}

// matchGatewayEnvironments matches gateway environment mappings with OpenChoreo environment details
// This function is used by both single-gateway and bulk-gateway queries
func (c *gatewayController) matchGatewayEnvironments(
	mappings []models.GatewayEnvironmentMapping,
	ocEnvironments []*models.EnvironmentResponse,
	ouID string,
) []models.GatewayEnvironmentResponse {
	if len(mappings) == 0 {
		return []models.GatewayEnvironmentResponse{}
	}

	if ocEnvironments == nil {
		return []models.GatewayEnvironmentResponse{}
	}

	// Create a map of environment UUIDs for quick lookup
	envMap := make(map[string]*models.EnvironmentResponse)
	for _, env := range ocEnvironments {
		envMap[env.UUID] = env
	}

	// Match mapped environments with OpenChoreo data
	var environments []models.GatewayEnvironmentResponse
	for _, mapping := range mappings {
		envUUIDStr := mapping.EnvironmentUUID.String()
		if ocEnv, found := envMap[envUUIDStr]; found {
			environments = append(environments, models.GatewayEnvironmentResponse{
				UUID:             ocEnv.UUID,
				OrganizationName: ouID,
				Name:             ocEnv.Name,
				DisplayName:      ocEnv.DisplayName,
				Description:      "",
				DataplaneRef:     ocEnv.DataplaneRef,
				DNSPrefix:        ocEnv.DNSPrefix,
				IsProduction:     ocEnv.IsProduction,
				CreatedAt:        ocEnv.CreatedAt,
				UpdatedAt:        ocEnv.CreatedAt,
			})
		}
	}

	return environments
}

// Helper conversion functions

// canonicalGatewayType uppercases the stored lowercase role for the wire enum.
func canonicalGatewayType(role string) spec.GatewayType {
	return spec.GatewayType(strings.ToUpper(role))
}

func convertGatewayToSpecResponse(gw *services.GatewayResponse, ouID string, environments []models.GatewayEnvironmentResponse) spec.GatewayResponse {
	response := spec.GatewayResponse{
		Uuid:        gw.ID,
		Name:        gw.Name,
		DisplayName: gw.DisplayName,
		GatewayType: canonicalGatewayType(gw.FunctionalityType),
		Vhost:       gw.Vhost,
		RuntimeUrl:  runtimeURLPtr(gw.RuntimeURL),
		IsCritical:  gw.IsCritical,
		Status:      convertStatusToGatewayStatus(gw.IsActive),
		CreatedAt:   gw.CreatedAt,
		UpdatedAt:   gw.UpdatedAt,
	}

	// Convert environments
	if len(environments) > 0 {
		envs := make([]spec.GatewayEnvironmentResponse, len(environments))
		for i, env := range environments {
			envs[i] = convertGatewayEnvironmentToSpecResponse(&env)
		}
		response.Environments = envs
	}

	return response
}

// runtimeURLPtr omits an unset runtime URL from the response rather than emitting "".
func runtimeURLPtr(runtimeURL string) *string {
	if runtimeURL == "" {
		return nil
	}
	return &runtimeURL
}

func convertStatusToGatewayStatus(isActive bool) spec.GatewayStatus {
	if isActive {
		return "ACTIVE"
	}
	return "INACTIVE"
}

func convertGatewayEnvironmentToSpecResponse(env *models.GatewayEnvironmentResponse) spec.GatewayEnvironmentResponse {
	response := spec.GatewayEnvironmentResponse{
		Id:           env.UUID,
		Name:         env.Name,
		DisplayName:  env.DisplayName,
		DataplaneRef: env.DataplaneRef,
		DnsPrefix:    env.DNSPrefix,
		IsProduction: env.IsProduction,
		CreatedAt:    env.CreatedAt,
		UpdatedAt:    env.UpdatedAt,
	}
	if env.Description != "" {
		response.Description = &env.Description
	}
	return response
}

// gatewayResponseID returns a registered gateway's identifier for an audit
// record, tolerating a nil gateway so a failed registration still records the
// attempt. Named to avoid reading like the many local gatewayUUID variables in
// this package, which hold a different thing.
func gatewayResponseID(gateway *services.GatewayResponse) string {
	if gateway == nil {
		return ""
	}
	return gateway.ID
}
