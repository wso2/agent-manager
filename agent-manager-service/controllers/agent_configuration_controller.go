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

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// AgentConfigurationController defines interface for agent configuration HTTP handlers
type AgentConfigurationController interface {
	CreateAgentModelConfig(w http.ResponseWriter, r *http.Request)
	GetAgentModelConfig(w http.ResponseWriter, r *http.Request)
	ListAgentModelConfigs(w http.ResponseWriter, r *http.Request)
	UpdateAgentModelConfig(w http.ResponseWriter, r *http.Request)
	DeleteAgentModelConfig(w http.ResponseWriter, r *http.Request)
}

type agentConfigurationController struct {
	agentConfigService services.AgentConfigurationService
}

// NewAgentConfigurationController creates a new agent configuration controller
func NewAgentConfigurationController(service services.AgentConfigurationService) AgentConfigurationController {
	return &agentConfigurationController{agentConfigService: service}
}

// CreateAgentModelConfig handles POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs
func (c *agentConfigurationController) CreateAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// TODO: Extract user from context (createdBy)
	createdBy := "system" // Placeholder

	// Bind request body
	var req models.CreateAgentModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateAgentModelConfig: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Call service
	response, err := c.agentConfigService.Create(ctx, orgName, projectName, agentName, req, createdBy)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrAgentConfigAlreadyExists):
			utils.WriteErrorResponse(w, http.StatusConflict, "Agent configuration already exists")
			return
		case errors.Is(err, utils.ErrAgentNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("CreateAgentModelConfig: failed to create configuration", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create agent model configuration")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

// GetAgentModelConfig handles GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) GetAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	configID := r.PathValue("configId")

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Error("GetAgentModelConfig: invalid config ID", "configId", configID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	response, err := c.agentConfigService.Get(ctx, configUUID, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("GetAgentModelConfig: failed to get configuration", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get configuration")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// ListAgentModelConfigs handles GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs
func (c *agentConfigurationController) ListAgentModelConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	limit := getIntQueryParam(r, "limit", 20)
	offset := getIntQueryParam(r, "offset", 0)

	response, err := c.agentConfigService.List(ctx, orgName, limit, offset)
	if err != nil {
		log.Error("ListAgentModelConfigs: failed to list configurations", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list configurations")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// UpdateAgentModelConfig handles PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) UpdateAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	configID := r.PathValue("configId")

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Error("UpdateAgentModelConfig: invalid config ID", "configId", configID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	var req models.UpdateAgentModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateAgentModelConfig: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	response, err := c.agentConfigService.Update(ctx, configUUID, orgName, req)
	if err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("UpdateAgentModelConfig: failed to update configuration", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update configuration")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// DeleteAgentModelConfig handles DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) DeleteAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	configID := r.PathValue("configId")

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Error("DeleteAgentModelConfig: invalid config ID", "configId", configID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	if err := c.agentConfigService.Delete(ctx, configUUID, orgName); err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("DeleteAgentModelConfig: failed to delete configuration", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete configuration")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
