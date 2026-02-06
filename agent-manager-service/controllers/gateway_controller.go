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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
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

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("RegisterGateway: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	var req models.CreateGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("RegisterGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	gateway, err := c.gatewayService.RegisterGateway(ctx, orgUUID, &req)
	if err != nil {
		log.Error("RegisterGateway: failed to register gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to register gateway")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, gateway)
}

func (c *gatewayController) GetGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("GetGateway: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	gateway, err := c.gatewayService.GetGateway(ctx, orgUUID, gatewayID)
	if err != nil {
		log.Error("GetGateway: failed to get gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, gateway)
}

func (c *gatewayController) ListGateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("ListGateways: failed to get organization", "error", err)
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

	// Parse filter parameters
	filter := services.GatewayFilter{
		Limit:  int32(limit),
		Offset: int32(offset),
	}

	if gatewayType := r.URL.Query().Get("gatewayType"); gatewayType != "" {
		filter.GatewayType = &gatewayType
	}

	if status := r.URL.Query().Get("status"); status != "" {
		filter.Status = &status
	}

	if region := r.URL.Query().Get("region"); region != "" {
		filter.Region = &region
	}

	if envID := r.URL.Query().Get("environmentId"); envID != "" {
		filter.EnvironmentID = &envID
	}

	gateways, err := c.gatewayService.ListGateways(ctx, orgUUID, filter)
	if err != nil {
		log.Error("ListGateways: failed to list gateways", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list gateways")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, gateways)
}

func (c *gatewayController) UpdateGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("UpdateGateway: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	var req models.UpdateGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	gateway, err := c.gatewayService.UpdateGateway(ctx, orgUUID, gatewayID, &req)
	if err != nil {
		log.Error("UpdateGateway: failed to update gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to update gateway")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, gateway)
}

func (c *gatewayController) DeleteGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("DeleteGateway: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	if err := c.gatewayService.DeleteGateway(ctx, orgUUID, gatewayID); err != nil {
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

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("AssignGatewayToEnvironment: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	if err := c.gatewayService.AssignGatewayToEnvironment(ctx, orgUUID, gatewayID, envID); err != nil {
		log.Error("AssignGatewayToEnvironment: failed to assign gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to assign gateway to environment")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, "")
}

func (c *gatewayController) RemoveGatewayFromEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")
	envID := r.PathValue("envID")

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("RemoveGatewayFromEnvironment: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	if err := c.gatewayService.RemoveGatewayFromEnvironment(ctx, orgUUID, gatewayID, envID); err != nil {
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

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("GetGatewayEnvironments: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	environments, err := c.gatewayService.GetGatewayEnvironments(ctx, orgUUID, gatewayID)
	if err != nil {
		log.Error("GetGatewayEnvironments: failed to get environments", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway environments")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, environments)
}

func (c *gatewayController) CheckGatewayHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")

	orgUUID, err := getOrgUUIDFromName(ctx, orgName)
	if err != nil {
		log.Error("CheckGatewayHealth: failed to get organization", "error", err)
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		return
	}

	health, err := c.gatewayService.CheckGatewayHealth(ctx, orgUUID, gatewayID)
	if err != nil {
		log.Error("CheckGatewayHealth: failed to check health", "error", err)
		handleGatewayErrors(w, err, "Failed to check gateway health")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, health)
}
