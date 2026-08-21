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
	"io"
	"net/http"
	"strconv"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// EnvironmentController defines the interface for environment HTTP handlers
type EnvironmentController interface {
	CreateEnvironment(w http.ResponseWriter, r *http.Request)
	GetEnvironment(w http.ResponseWriter, r *http.Request)
	ListEnvironments(w http.ResponseWriter, r *http.Request)
	UpdateEnvironment(w http.ResponseWriter, r *http.Request)
	DeleteEnvironment(w http.ResponseWriter, r *http.Request)
	GetEnvironmentGateways(w http.ResponseWriter, r *http.Request)
	ListThunderInstances(w http.ResponseWriter, r *http.Request)
	SetThunderSystemClient(w http.ResponseWriter, r *http.Request)
	DeleteThunderSystemClient(w http.ResponseWriter, r *http.Request)
	SetThunderURL(w http.ResponseWriter, r *http.Request)
	GetThunderURL(w http.ResponseWriter, r *http.Request)
	DeleteThunderURL(w http.ResponseWriter, r *http.Request)
	CheckThunderURLAvailability(w http.ResponseWriter, r *http.Request)
}

type environmentController struct {
	environmentService services.EnvironmentService
}

// NewEnvironmentController creates a new environment controller
func NewEnvironmentController(environmentService services.EnvironmentService) EnvironmentController {
	return &environmentController{
		environmentService: environmentService,
	}
}

func handleEnvironmentErrors(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, utils.ErrEnvironmentNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
	case errors.Is(err, utils.ErrEnvironmentAlreadyExists) || errors.Is(err, utils.ErrConflict):
		utils.WriteErrorResponse(w, http.StatusConflict, "Environment already exists")
	case errors.Is(err, utils.ErrEnvironmentHasGateways):
		utils.WriteErrorResponse(w, http.StatusConflict, "Environment has associated gateways")
	case errors.Is(err, utils.ErrEnvironmentInUse):
		utils.WriteErrorResponse(w, http.StatusConflict, "Environment is referenced by one or more deployment pipelines")
	case errors.Is(err, utils.ErrInvalidInput):
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
	default:
		utils.WriteErrorResponse(w, http.StatusInternalServerError, fallbackMsg)
	}
}

func (c *environmentController) CreateEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)

	var req spec.CreateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("CreateEnvironment: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to internal model
	internalReq := &models.CreateEnvironmentRequest{
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		DataplaneRef:  utils.GetOrDefault(req.DataplaneRef, ""),
		DNSPrefix:     req.DnsPrefix,
		IsProduction:  false,
		Gateway:       fromSpecGatewaySpec(req.Gateway),
		IsolationTier: utils.GetOrDefault(req.IsolationTier, ""),
	}

	if req.Description != nil {
		internalReq.Description = *req.Description
	}
	if req.IsProduction != nil {
		internalReq.IsProduction = *req.IsProduction
	}

	env, err := c.environmentService.CreateEnvironment(ctx, ouID, internalReq)
	if err != nil {
		log.Error("CreateEnvironment: failed to create environment", "error", err)
		handleEnvironmentErrors(w, err, "Failed to create environment")
		return
	}

	// Convert internal response to spec response
	response := convertToSpecEnvironmentResponse(env)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *environmentController) GetEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	envID := r.PathValue("envID")

	env, err := c.environmentService.GetEnvironment(ctx, ouID, envID)
	if err != nil {
		log.Error("GetEnvironment: failed to get environment", "error", err)
		handleEnvironmentErrors(w, err, "Failed to get environment")
		return
	}

	response := convertToSpecEnvironmentResponse(env)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *environmentController) ListEnvironments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)

	// Parse pagination parameters
	limit := getIntQueryParam(r, "limit", utils.DefaultLimit)
	offset := getIntQueryParam(r, "offset", utils.DefaultOffset)

	// Validate limits
	if limit < utils.MinLimit || limit > utils.MaxLimit {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid limit parameter")
		return
	}
	if offset < 0 {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid offset parameter")
		return
	}

	envList, err := c.environmentService.ListEnvironments(ctx, ouID, int32(limit), int32(offset))
	if err != nil {
		log.Error("ListEnvironments: failed to list environments", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list environments")
		return
	}

	// Convert to spec responses
	specEnvs := make([]spec.GatewayEnvironmentResponse, len(envList.Environments))
	for i, env := range envList.Environments {
		specEnvs[i] = convertToSpecEnvironmentResponse(&env)
	}

	utils.WriteSuccessResponse(w, http.StatusOK, specEnvs)
}

func (c *environmentController) UpdateEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	envID := r.PathValue("envID")

	var req spec.UpdateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("UpdateEnvironment: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to internal model
	var description *string
	if req.Description.IsSet() {
		description = req.Description.Get()
	}

	internalReq := &models.UpdateEnvironmentRequest{
		DisplayName:  req.DisplayName,
		Description:  description,
		IsProduction: req.IsProduction,
		Gateway:      fromSpecGatewaySpec(req.Gateway),
	}

	env, err := c.environmentService.UpdateEnvironment(ctx, ouID, envID, internalReq)
	if err != nil {
		log.Error("UpdateEnvironment: failed to update environment", "error", err)
		handleEnvironmentErrors(w, err, "Failed to update environment")
		return
	}

	response := convertToSpecEnvironmentResponse(env)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *environmentController) DeleteEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	envID := r.PathValue("envID")

	if err := c.environmentService.DeleteEnvironment(ctx, ouID, envID); err != nil {
		log.Error("DeleteEnvironment: failed to delete environment", "error", err)
		handleEnvironmentErrors(w, err, "Failed to delete environment")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, "")
}

func (c *environmentController) GetEnvironmentGateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	envID := r.PathValue("envID")

	gatewayList, err := c.environmentService.GetEnvironmentGateways(ctx, ouID, envID)
	if err != nil {
		log.Error("GetEnvironmentGateways: failed to get gateways", "error", err)
		handleEnvironmentErrors(w, err, "Failed to get environment gateways")
		return
	}

	// Convert to spec responses
	specGateways := make([]spec.GatewayResponse, len(gatewayList))
	for i, gw := range gatewayList {
		specGateways[i] = convertToSpecGatewayResponse(&gw)
	}

	response := spec.GetEnvironmentGateways200Response{
		Gateways: specGateways,
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// Helper function to get int query param with default value
func getIntQueryParam(r *http.Request, key string, defaultValue int) int {
	if val := r.URL.Query().Get(key); val != "" {
		if intVal, err := strconv.Atoi(val); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// convertToSpecEnvironmentResponse converts internal environment response to spec response
func convertToSpecEnvironmentResponse(env *models.GatewayEnvironmentResponse) spec.GatewayEnvironmentResponse {
	response := spec.GatewayEnvironmentResponse{
		Id:           env.UUID,
		Name:         env.Name,
		DisplayName:  env.DisplayName,
		DataplaneRef: env.DataplaneRef,
		DnsPrefix:    env.DNSPrefix,
		Description:  &env.Description,
		IsProduction: env.IsProduction,
		Gateway:      toSpecGatewaySpec(env.Gateway),
		CreatedAt:    env.CreatedAt,
		UpdatedAt:    env.UpdatedAt,
	}
	if env.IsolationTier != "" {
		response.IsolationTier = &env.IsolationTier
	}

	return response
}

// -----------------------------------------------------------------------------
// Gateway spec translation between the generated OpenAPI types (spec.*) and the
// internal DTO (models.*). The two shapes are field-compatible — the public API
// deliberately omits OC-only fields (Gateway resource name/namespace, listener
// name); the OC client constructs those at request time.
// -----------------------------------------------------------------------------

func fromSpecGatewaySpec(g *spec.GatewaySpec) *models.GatewaySpec {
	if g == nil {
		return nil
	}
	return &models.GatewaySpec{
		Ingress: fromSpecGatewayNetworkSpec(g.Ingress),
		Egress:  fromSpecGatewayNetworkSpec(g.Egress),
	}
}

func fromSpecGatewayNetworkSpec(n *spec.GatewayNetworkSpec) *models.GatewayNetworkSpec {
	if n == nil {
		return nil
	}
	return &models.GatewayNetworkSpec{
		External: fromSpecGatewayEndpointSpec(n.External),
		Internal: fromSpecGatewayEndpointSpec(n.Internal),
	}
}

func fromSpecGatewayEndpointSpec(e *spec.GatewayEndpointSpec) *models.GatewayEndpointSpec {
	if e == nil {
		return nil
	}
	return &models.GatewayEndpointSpec{
		HTTP:  fromSpecGatewayListenerSpec(e.Http),
		HTTPS: fromSpecGatewayListenerSpec(e.Https),
		TLS:   fromSpecGatewayListenerSpec(e.Tls),
	}
}

func fromSpecGatewayListenerSpec(l *spec.GatewayListenerSpec) *models.GatewayListenerSpec {
	if l == nil {
		return nil
	}
	return &models.GatewayListenerSpec{Port: l.Port, Host: l.Host}
}

func toSpecGatewaySpec(g *models.GatewaySpec) *spec.GatewaySpec {
	if g == nil {
		return nil
	}
	return &spec.GatewaySpec{
		Ingress: toSpecGatewayNetworkSpec(g.Ingress),
		Egress:  toSpecGatewayNetworkSpec(g.Egress),
	}
}

func toSpecGatewayNetworkSpec(n *models.GatewayNetworkSpec) *spec.GatewayNetworkSpec {
	if n == nil {
		return nil
	}
	return &spec.GatewayNetworkSpec{
		External: toSpecGatewayEndpointSpec(n.External),
		Internal: toSpecGatewayEndpointSpec(n.Internal),
	}
}

func toSpecGatewayEndpointSpec(e *models.GatewayEndpointSpec) *spec.GatewayEndpointSpec {
	if e == nil {
		return nil
	}
	return &spec.GatewayEndpointSpec{
		Http:  toSpecGatewayListenerSpec(e.HTTP),
		Https: toSpecGatewayListenerSpec(e.HTTPS),
		Tls:   toSpecGatewayListenerSpec(e.TLS),
	}
}

func toSpecGatewayListenerSpec(l *models.GatewayListenerSpec) *spec.GatewayListenerSpec {
	if l == nil {
		return nil
	}
	return &spec.GatewayListenerSpec{Port: l.Port, Host: l.Host}
}

// convertToSpecGatewayResponse converts internal gateway response to spec response
func convertToSpecGatewayResponse(gw *models.GatewayResponse) spec.GatewayResponse {
	response := spec.GatewayResponse{
		Uuid:        gw.UUID,
		Name:        gw.Name,
		DisplayName: gw.DisplayName,
		GatewayType: canonicalGatewayType(gw.GatewayType),
		Vhost:       gw.VHost,
		IsCritical:  gw.IsCritical,
		Status:      spec.GatewayStatus(gw.Status),
	}

	return response
}

func (c *environmentController) ListThunderInstances(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)

	result, err := c.environmentService.ListThunderInstances(ctx, ouID)
	if err != nil {
		log.Error("ListThunderInstances: failed to list thunder instances", "ou_id", ouID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list Thunder instances")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, result)
}

// SetThunderSystemClient stores an environment's env-Thunder system-client
// credential (bootstrap-only; called by add-environment-thunder.sh).
func (c *environmentController) SetThunderSystemClient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	envName := r.PathValue("envID")

	var req spec.ThunderSystemClientRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("SetThunderSystemClient: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.ClientId == "" || req.ClientSecret == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "clientId and clientSecret are required")
		return
	}

	// The clientId identifies the credential; clientSecret is never passed to
	// the recorder. Refused when unrecordable — this credential is what AMS uses
	// to reach the environment's identity provider.
	attempt, ok := beginAuditOrFail(
		w, r, "SetThunderSystemClient", "Failed to store system-client credential", audit.ActionServiceAccountConfigure,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceServiceAccount, envName, envName),
		audit.Environment(envName),
		audit.Detail("environment", envName),
		audit.Detail("clientId", req.ClientId),
	)
	if !ok {
		return
	}

	if err := c.environmentService.SetThunderSystemClientSecret(ctx, ouID, envName, req.ClientId, req.ClientSecret); err != nil {
		attempt.Complete(ctx, err)
		log.Warn("SetThunderSystemClient: failed to store credential", "ou_id", ouID, "env_name", envName, "error", err)
		if errors.Is(err, utils.ErrInvalidInput) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to store system-client credential")
		return
	}
	attempt.Complete(ctx, nil)

	utils.WriteSuccessResponse(w, http.StatusNoContent, "")
}

// DeleteThunderSystemClient removes an environment's env-Thunder system-client
// credential (idempotent; called by remove-environment-thunder.sh).
func (c *environmentController) DeleteThunderSystemClient(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	envName := r.PathValue("envID")

	attempt, ok := beginAuditOrFail(
		w, r, "DeleteThunderSystemClient", "Failed to delete system-client credential", audit.ActionServiceAccountRemove,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceServiceAccount, envName, envName),
		audit.Environment(envName),
		audit.Detail("environment", envName),
	)
	if !ok {
		return
	}

	if err := c.environmentService.DeleteThunderSystemClientSecret(ctx, ouID, envName); err != nil {
		attempt.Complete(ctx, err)
		log.Warn("DeleteThunderSystemClient: failed to delete credential", "ou_id", ouID, "env_name", envName, "error", err)
		if errors.Is(err, utils.ErrInvalidInput) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete system-client credential")
		return
	}
	attempt.Complete(ctx, nil)

	utils.WriteSuccessResponse(w, http.StatusNoContent, "")
}

// SetThunderURL registers an environment's env-Thunder URL handle (bootstrap-only;
// called by add-environment-thunder.sh before it provisions the Thunder instance).
func (c *environmentController) SetThunderURL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	envName := r.PathValue("envID")

	// The request body itself is optional (an empty PUT means "generate one for
	// me"), so a missing/empty body is not a decode error — only malformed JSON is.
	// ContentLength is -1 for chunked requests, so io.EOF checks for an empty body.
	var req spec.ThunderUrlRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			log.Warn("SetThunderURL: failed to decode request", "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}
	var handle string
	if req.Handle != nil {
		handle = *req.Handle
	}

	// The resolved handle (possibly server-generated) is only known once the
	// service call returns, so it's added to the record at Complete rather
	// than here.
	attempt, ok := beginAuditOrFail(
		w, r, "SetThunderURL", "Failed to store thunder url handle", audit.ActionThunderURLSet,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceThunderURL, envName, envName),
		audit.Environment(envName),
	)
	if !ok {
		return
	}

	resolved, err := c.environmentService.SetThunderURL(ctx, ouID, envName, handle)
	if err != nil {
		attempt.Complete(ctx, err)
		log.Warn("SetThunderURL: failed to store handle", "ou_id", ouID, "env_name", envName, "error", err)
		switch {
		case errors.Is(err, utils.ErrThunderHandleTaken):
			utils.WriteErrorResponse(w, http.StatusConflict, "Thunder URL handle is already in use")
		case errors.Is(err, utils.ErrInvalidThunderHandle), errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
		default:
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to store thunder url handle")
		}
		return
	}
	attempt.Complete(ctx, nil, audit.Detail("handle", resolved))

	utils.WriteSuccessResponse(w, http.StatusOK, spec.ThunderUrlResponse{Handle: resolved})
}

// GetThunderURL returns an environment's registered env-Thunder URL handle.
// Used by add-environment.sh, after Thunder provisioning succeeds, to learn the
// actual handle (possibly server-generated) for wiring the gateway's ThunderKeyManager.
func (c *environmentController) GetThunderURL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	envName := r.PathValue("envID")

	handle, err := c.environmentService.GetThunderURL(ctx, ouID, envName)
	if err != nil {
		if errors.Is(err, utils.ErrThunderHandleNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "No thunder url handle registered for this environment")
			return
		}
		log.Error("GetThunderURL: failed to read handle", "ou_id", ouID, "env_name", envName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to read thunder url handle")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, spec.ThunderUrlResponse{Handle: handle})
}

// DeleteThunderURL removes an environment's env-Thunder URL handle (idempotent;
// called by remove-environment-thunder.sh), freeing it for reuse.
func (c *environmentController) DeleteThunderURL(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	envName := r.PathValue("envID")

	attempt, ok := beginAuditOrFail(
		w, r, "DeleteThunderURL", "Failed to delete thunder url handle", audit.ActionThunderURLDelete,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceThunderURL, envName, envName),
		audit.Environment(envName),
	)
	if !ok {
		return
	}

	// Read the handle being freed so the success record below names it. A
	// genuinely missing handle is not an error — Delete is idempotent — but any
	// other read failure must abort before the delete: silently ignoring it
	// would risk deleting a row the record can no longer identify.
	existing, err := c.environmentService.GetThunderURL(ctx, ouID, envName)
	if err != nil && !errors.Is(err, utils.ErrThunderHandleNotFound) {
		attempt.Complete(ctx, err)
		log.Error("DeleteThunderURL: failed to read handle before delete", "ou_id", ouID, "env_name", envName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete thunder url handle")
		return
	}

	if err := c.environmentService.DeleteThunderURL(ctx, ouID, envName); err != nil {
		attempt.Complete(ctx, err)
		log.Warn("DeleteThunderURL: failed to delete handle", "ou_id", ouID, "env_name", envName, "error", err)
		if errors.Is(err, utils.ErrInvalidInput) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete thunder url handle")
		return
	}
	attempt.Complete(ctx, nil, audit.Detail("handle", existing))

	utils.WriteSuccessResponse(w, http.StatusNoContent, "")
}

// CheckThunderURLAvailability reports whether a candidate env-Thunder URL
// handle passes format validation and is not already registered to any
// environment. Advisory only — used by the console's Create Environment
// drawer to reject an obviously-taken handle before the user ever runs the
// generated add-environment.sh command; SetThunderURL's atomic insert remains
// the real enforcement.
func (c *environmentController) CheckThunderURLAvailability(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	handle := r.URL.Query().Get("handle")

	available, err := c.environmentService.IsThunderHandleAvailable(ctx, handle)
	if err != nil {
		if errors.Is(err, utils.ErrInvalidThunderHandle) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error("CheckThunderURLAvailability: failed to check handle availability", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to check thunder url handle availability")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, spec.ThunderUrlAvailabilityResponse{Available: available})
}
