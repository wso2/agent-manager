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

	var req models.CreateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateEnvironment: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	env, err := c.environmentService.CreateEnvironment(ctx, orgUUID, &req)
	if err != nil {
		log.Error("CreateEnvironment: failed to create environment", "error", err)
		handleEnvironmentErrors(w, err, "Failed to create environment")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, env)
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

	utils.WriteSuccessResponse(w, http.StatusOK, env)
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

	envs, err := c.environmentService.ListEnvironments(ctx, orgUUID, int32(limit), int32(offset))
	if err != nil {
		log.Error("ListEnvironments: failed to list environments", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list environments")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, envs)
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

	var req models.UpdateEnvironmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateEnvironment: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	env, err := c.environmentService.UpdateEnvironment(ctx, orgUUID, envID, &req)
	if err != nil {
		log.Error("UpdateEnvironment: failed to update environment", "error", err)
		handleEnvironmentErrors(w, err, "Failed to update environment")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, env)
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

	gateways, err := c.environmentService.GetEnvironmentGateways(ctx, orgUUID, envID)
	if err != nil {
		log.Error("GetEnvironmentGateways: failed to get gateways", "error", err)
		handleEnvironmentErrors(w, err, "Failed to get environment gateways")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, gateways)
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

// Placeholder function for organization lookup
// This will be replaced with proper integration in Phase 5
func getOrgUUIDFromName(ctx context.Context, orgName string) (uuid.UUID, error) {
	// TODO: Integrate with OpenChoreo client or database lookup
	// For now, return a placeholder
	// This is a temporary implementation
	return uuid.Parse("00000000-0000-0000-0000-000000000001")
}
