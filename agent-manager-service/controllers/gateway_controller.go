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
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// GatewayController defines the interface for gateway HTTP handlers
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
}

type gatewayController struct {
	gatewayService services.GatewayService
}

// NewGatewayController creates a new gateway controller
func NewGatewayController(gatewayService services.GatewayService) GatewayController {
	return &gatewayController{
		gatewayService: gatewayService,
	}
}

func handleGatewayErrors(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, utils.ErrGatewayNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
	case errors.Is(err, utils.ErrGatewayAlreadyExists):
		utils.WriteErrorResponse(w, http.StatusConflict, "Gateway already exists")
	case errors.Is(err, utils.ErrGatewayUnreachable):
		utils.WriteErrorResponse(w, http.StatusBadGateway, "Gateway unreachable")
	case errors.Is(err, utils.ErrEnvironmentNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
	case errors.Is(err, utils.ErrInvalidInput):
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
	default:
		utils.WriteErrorResponse(w, http.StatusInternalServerError, fallbackMsg)
	}
}

func (c *gatewayController) RegisterGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)

	var req spec.CreateGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("RegisterGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to internal model
	internalReq := &models.CreateGatewayRequest{
		Name:          req.Name,
		DisplayName:   req.DisplayName,
		GatewayType:   string(req.GatewayType),
		VHost:         req.Vhost,
		IsCritical:    req.GetIsCritical(),
		AdapterConfig: req.AdapterConfig,
	}

	if req.Region != nil {
		internalReq.Region = *req.Region
	}

	if req.Credentials != nil {
		internalReq.Credentials = &models.GatewayCredentials{}
		if req.Credentials.Username != nil {
			internalReq.Credentials.Username = *req.Credentials.Username
		}
		if req.Credentials.Password != nil {
			internalReq.Credentials.Password = *req.Credentials.Password
		}
		if req.Credentials.ApiKey != nil {
			internalReq.Credentials.APIKey = *req.Credentials.ApiKey
		}
		if req.Credentials.Token != nil {
			internalReq.Credentials.Token = *req.Credentials.Token
		}
	}

	gateway, err := c.gatewayService.RegisterGateway(ctx, orgName, internalReq)
	if err != nil {
		log.Error("RegisterGateway: failed to register gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to register gateway")
		return
	}

	response := convertToSpecGatewayResponse(gateway)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *gatewayController) GetGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")

	gateway, err := c.gatewayService.GetGateway(ctx, orgName, gatewayID)
	if err != nil {
		log.Error("GetGateway: failed to get gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway")
		return
	}

	response := convertToSpecGatewayResponse(gateway)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) ListGateways(w http.ResponseWriter, r *http.Request) {
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

	// Parse filter parameters
	filter := services.GatewayFilter{
		Limit:  int32(limit),
		Offset: int32(offset),
	}

	// Query parameter names match OpenAPI spec
	if gatewayType := r.URL.Query().Get("type"); gatewayType != "" {
		filter.GatewayType = &gatewayType
	}

	if status := r.URL.Query().Get("status"); status != "" {
		filter.Status = &status
	}

	// Note: 'region' parameter removed as it's not in OpenAPI spec

	if envID := r.URL.Query().Get("environment"); envID != "" {
		filter.EnvironmentID = &envID
	}

	gatewayList, err := c.gatewayService.ListGateways(ctx, orgName, filter)
	if err != nil {
		log.Error("ListGateways: failed to list gateways", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list gateways")
		return
	}

	// Convert to spec responses
	specGateways := make([]spec.GatewayResponse, len(gatewayList.Gateways))
	for i := range gatewayList.Gateways {
		specGateways[i] = convertToSpecGatewayResponse(&gatewayList.Gateways[i])
	}

	response := spec.GatewayListResponse{
		Gateways: specGateways,
		Total:    gatewayList.Total,
		Limit:    gatewayList.Limit,
		Offset:   gatewayList.Offset,
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) UpdateGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")

	var req spec.UpdateGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to internal model
	internalReq := &models.UpdateGatewayRequest{
		DisplayName:   req.DisplayName,
		IsCritical:    req.IsCritical,
		AdapterConfig: req.AdapterConfig,
	}

	if req.Status != nil {
		status := string(*req.Status)
		internalReq.Status = &status
	}

	if req.Credentials != nil {
		internalReq.Credentials = &models.GatewayCredentials{}
		if req.Credentials.Username != nil {
			internalReq.Credentials.Username = *req.Credentials.Username
		}
		if req.Credentials.Password != nil {
			internalReq.Credentials.Password = *req.Credentials.Password
		}
		if req.Credentials.ApiKey != nil {
			internalReq.Credentials.APIKey = *req.Credentials.ApiKey
		}
		if req.Credentials.Token != nil {
			internalReq.Credentials.Token = *req.Credentials.Token
		}
	}

	gateway, err := c.gatewayService.UpdateGateway(ctx, orgName, gatewayID, internalReq)
	if err != nil {
		log.Error("UpdateGateway: failed to update gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to update gateway")
		return
	}

	response := convertToSpecGatewayResponse(gateway)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) DeleteGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")

	if err := c.gatewayService.DeleteGateway(ctx, orgName, gatewayID); err != nil {
		log.Error("DeleteGateway: failed to delete gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to delete gateway")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, "")
}

func (c *gatewayController) AssignGatewayToEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")
	envID := r.PathValue("envID")

	if err := c.gatewayService.AssignGatewayToEnvironment(ctx, orgName, gatewayID, envID); err != nil {
		log.Error("AssignGatewayToEnvironment: failed to assign gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to assign gateway to environment")
		return
	}

	// Fetch updated gateway to return per OpenAPI spec
	gateway, err := c.gatewayService.GetGateway(ctx, orgName, gatewayID)
	if err != nil {
		log.Error("AssignGatewayToEnvironment: failed to retrieve gateway after assignment", "error", err)
		handleGatewayErrors(w, err, "Failed to retrieve gateway")
		return
	}

	response := convertToSpecGatewayResponse(gateway)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *gatewayController) RemoveGatewayFromEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")
	envID := r.PathValue("envID")

	if err := c.gatewayService.RemoveGatewayFromEnvironment(ctx, orgName, gatewayID, envID); err != nil {
		log.Error("RemoveGatewayFromEnvironment: failed to remove gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to remove gateway from environment")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, "")
}

func (c *gatewayController) GetGatewayEnvironments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")

	environments, err := c.gatewayService.GetGatewayEnvironments(ctx, orgName, gatewayID)
	if err != nil {
		log.Error("GetGatewayEnvironments: failed to get environments", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway environments")
		return
	}

	// Convert to spec responses
	specEnvs := make([]spec.GatewayEnvironmentResponse, len(environments))
	for i := range environments {
		specEnvs[i] = convertToSpecEnvironmentResponse(&environments[i])
	}

	response := spec.GetGatewayEnvironments200Response{
		Environments: specEnvs,
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) CheckGatewayHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")

	health, err := c.gatewayService.CheckGatewayHealth(ctx, orgName, gatewayID)
	if err != nil {
		log.Error("CheckGatewayHealth: failed to check health", "error", err)
		handleGatewayErrors(w, err, "Failed to check gateway health")
		return
	}

	// Convert internal response to spec response
	response := spec.HealthStatusResponse{
		GatewayId: health.GatewayID,
		Status:    health.Status,
		CheckedAt: parseTimeString(health.CheckedAt),
	}

	if health.ResponseTime != "" {
		response.ResponseTime = &health.ResponseTime
	}

	if health.ErrorMessage != "" {
		response.ErrorMessage = &health.ErrorMessage
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// parseTimeString converts a time string to time.Time
// Falls back to current time if parsing fails
func parseTimeString(timeStr string) time.Time {
	t, err := time.Parse(time.RFC3339, timeStr)
	if err != nil {
		return time.Now()
	}
	return t
}
