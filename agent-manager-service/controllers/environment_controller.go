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
	"strconv"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// EnvironmentController defines the interface for environment HTTP handlers
type EnvironmentController interface {
	CreateEnvironment(w http.ResponseWriter, r *http.Request)
	GetEnvironment(w http.ResponseWriter, r *http.Request)
	ListEnvironments(w http.ResponseWriter, r *http.Request)
	UpdateEnvironment(w http.ResponseWriter, r *http.Request)
	DeleteEnvironment(w http.ResponseWriter, r *http.Request)
	GetEnvironmentGateways(w http.ResponseWriter, r *http.Request)
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
	case errors.Is(err, utils.ErrEnvironmentAlreadyExists):
		utils.WriteErrorResponse(w, http.StatusConflict, "Environment already exists")
	case errors.Is(err, utils.ErrEnvironmentHasGateways):
		utils.WriteErrorResponse(w, http.StatusConflict, "Environment has associated gateways")
	case errors.Is(err, utils.ErrInvalidInput):
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
	default:
		utils.WriteErrorResponse(w, http.StatusInternalServerError, fallbackMsg)
	}
}

func (c *environmentController) CreateEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)

	var req spec.CreateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateEnvironment: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to internal model
	internalReq := &models.CreateEnvironmentRequest{
		Name:         req.Name,
		DisplayName:  req.DisplayName,
		DataplaneRef: req.DataplaneRef,
		DNSPrefix:    req.DnsPrefix,
		IsProduction: false,
	}

	if req.Description != nil {
		internalReq.Description = *req.Description
	}
	if req.IsProduction != nil {
		internalReq.IsProduction = *req.IsProduction
	}

	env, err := c.environmentService.CreateEnvironment(ctx, orgName, internalReq)
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

	orgName := r.PathValue(utils.PathParamOrgName)
	envID := r.PathValue("envID")

	env, err := c.environmentService.GetEnvironment(ctx, orgName, envID)
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

	orgName := r.PathValue(utils.PathParamOrgName)

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

	envList, err := c.environmentService.ListEnvironments(ctx, orgName, int32(limit), int32(offset))
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

	orgName := r.PathValue(utils.PathParamOrgName)
	envID := r.PathValue("envID")

	var req spec.UpdateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateEnvironment: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to internal model
	var description *string
	if req.Description.IsSet() {
		description = req.Description.Get()
	}

	internalReq := &models.UpdateEnvironmentRequest{
		DisplayName: req.DisplayName,
		Description: description,
	}

	env, err := c.environmentService.UpdateEnvironment(ctx, orgName, envID, internalReq)
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

	orgName := r.PathValue(utils.PathParamOrgName)
	envID := r.PathValue("envID")

	if err := c.environmentService.DeleteEnvironment(ctx, orgName, envID); err != nil {
		log.Error("DeleteEnvironment: failed to delete environment", "error", err)
		handleEnvironmentErrors(w, err, "Failed to delete environment")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, "")
}

func (c *environmentController) GetEnvironmentGateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	envID := r.PathValue("envID")

	gatewayList, err := c.environmentService.GetEnvironmentGateways(ctx, orgName, envID)
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
		Id:               env.UUID,
		OrganizationName: env.OrganizationName,
		Name:             env.Name,
		DisplayName:      env.DisplayName,
		DataplaneRef:     env.DataplaneRef,
		DnsPrefix:        env.DNSPrefix,
		IsProduction:     env.IsProduction,
		CreatedAt:        env.CreatedAt,
		UpdatedAt:        env.UpdatedAt,
	}

	if env.Description != "" {
		response.Description = &env.Description
	}

	return response
}

// convertToSpecGatewayResponse converts internal gateway response to spec response
func convertToSpecGatewayResponse(gw *models.GatewayResponse) spec.GatewayResponse {
	response := spec.GatewayResponse{
		Uuid:             gw.UUID,
		OrganizationName: gw.OrganizationName,
		Name:             gw.Name,
		DisplayName:      gw.DisplayName,
		GatewayType:      spec.GatewayType(gw.GatewayType),
		Vhost:            gw.VHost,
		IsCritical:       gw.IsCritical,
		Status:           spec.GatewayStatus(gw.Status),
	}

	return response
}
