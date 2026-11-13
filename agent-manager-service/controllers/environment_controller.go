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
	"errors"
	"net/http"
	"strconv"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// EnvironmentController defines the interface for environment HTTP handlers
type EnvironmentController interface {
	GetEnvironment(w http.ResponseWriter, r *http.Request)
	ListEnvironments(w http.ResponseWriter, r *http.Request)
	GetEnvironmentGateways(w http.ResponseWriter, r *http.Request)
	ListThunderInstances(w http.ResponseWriter, r *http.Request)
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
	case errors.Is(err, utils.ErrResourceBeingDeleted):
		utils.WriteErrorResponse(w, http.StatusConflict, "An environment with this name is still being deleted, try again shortly")
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
		Hosting:     spec.GatewayHosting(gw.Hosting),
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
		log.Error("ListThunderInstances: failed to list thunder instances", "ouID", ouID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list Thunder instances")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, result)
}
