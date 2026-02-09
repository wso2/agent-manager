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
	"net/http"
	"strconv"

	"github.com/google/uuid"

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

	// TODO: Get organization UUID from orgName via OpenChoreo client
	// For now, use a placeholder (in real implementation, fetch from DB)
	// This will be integrated with proper organization lookup in Phase 5
	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("CreateEnvironment: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	var req spec.CreateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateEnvironment: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to internal model
	internalReq := &models.CreateEnvironmentRequest{
		Name:        req.Name,
		DisplayName: req.DisplayName,
	}

	if req.Description != nil {
		internalReq.Description = *req.Description
	}

	env, err := c.environmentService.CreateEnvironment(ctx, orgUUID, internalReq)
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

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("GetEnvironment: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	env, err := c.environmentService.GetEnvironment(ctx, orgUUID, envID)
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

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("ListEnvironments: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

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

	envList, err := c.environmentService.ListEnvironments(ctx, orgUUID, int32(limit), int32(offset))
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

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("UpdateEnvironment: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

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

	env, err := c.environmentService.UpdateEnvironment(ctx, orgUUID, envID, internalReq)
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

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("DeleteEnvironment: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	if err := c.environmentService.DeleteEnvironment(ctx, orgUUID, envID); err != nil {
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

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("GetEnvironmentGateways: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	gatewayList, err := c.environmentService.GetEnvironmentGateways(ctx, orgUUID, envID)
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

// getOrgUUIDFromName looks up an organization UUID by name
// TODO: Integrate with OpenChoreo client for proper organization lookup
// Currently returns a placeholder UUID - this will be updated when
// the gateway management feature is fully integrated with the authentication
// and organization management system.
//
// For now, this allows the gateway management APIs to be functional
// for testing with direct UUID access. The full integration will use
// the OpenChoreo client (similar to infraResourceController.GetOrganization).
func getOrgUUIDFromName(ctx context.Context, orgName string) (uuid.UUID, error) {
	// TODO: Phase 6+ - Integrate with OpenChoreo client
	//
	// Integration pattern:
	// 1. Inject OpenChoreo client into controller constructors
	// 2. Use client.GetOrganization(ctx, orgName) to lookup organization
	// 3. Extract and return the organization UUID
	//
	// Example:
	//   org, err := c.ocClient.GetOrganization(ctx, orgName)
	//   if err != nil {
	//       return uuid.UUID{}, err
	//   }
	//   return org.UUID, nil

	if orgName == "" {
		return uuid.UUID{}, errors.New("organization name cannot be empty")
	}

	// Generate deterministic UUID from orgName using SHA-1 in DNS namespace
	// This ensures different orgNames produce different UUIDs, preventing
	// cross-tenant data access until OpenChoreo integration is complete
	return uuid.NewSHA1(uuid.NameSpaceDNS, []byte(orgName)), nil
}

// convertToSpecEnvironmentResponse converts internal environment response to spec response
func convertToSpecEnvironmentResponse(env *models.GatewayEnvironmentResponse) spec.GatewayEnvironmentResponse {
	response := spec.GatewayEnvironmentResponse{
		Id:             env.UUID,
		OrganizationId: env.OrganizationID,
		Name:           env.Name,
		DisplayName:    env.DisplayName,
		CreatedAt:      env.CreatedAt,
		UpdatedAt:      env.UpdatedAt,
	}

	if env.Description != "" {
		response.Description = &env.Description
	}

	return response
}

// convertToSpecGatewayResponse converts internal gateway response to spec response
func convertToSpecGatewayResponse(gw *models.GatewayResponse) spec.GatewayResponse {
	response := spec.GatewayResponse{
		Uuid:           gw.UUID,
		OrganizationId: gw.OrganizationID,
		Name:           gw.Name,
		DisplayName:    gw.DisplayName,
		GatewayType:    spec.GatewayType(gw.GatewayType),
		Vhost:          gw.VHost,
		IsCritical:     gw.IsCritical,
		Status:         spec.GatewayStatus(gw.Status),
		CreatedAt:      gw.CreatedAt,
		UpdatedAt:      gw.UpdatedAt,
	}

	if gw.ControlPlaneURL != "" {
		response.ControlPlaneUrl = &gw.ControlPlaneURL
	}

	if gw.Region != "" {
		response.Region = &gw.Region
	}

	if len(gw.AdapterConfig) > 0 {
		response.AdapterConfig = gw.AdapterConfig
	}

	// Convert environments if present
	if len(gw.Environments) > 0 {
		specEnvs := make([]spec.GatewayEnvironmentResponse, len(gw.Environments))
		for i := range gw.Environments {
			specEnvs[i] = convertToSpecEnvironmentResponse(&gw.Environments[i])
		}
		response.Environments = specEnvs
	}

	return response
}
