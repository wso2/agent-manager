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
	"io"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Note: uuid import retained for configUUID parsing in path parameters

// AgentConfigurationController defines interface for agent configuration HTTP handlers
type AgentConfigurationController interface {
	CreateAgentModelConfig(w http.ResponseWriter, r *http.Request)
	GetAgentModelConfig(w http.ResponseWriter, r *http.Request)
	ListAgentModelConfigs(w http.ResponseWriter, r *http.Request)
	UpdateAgentModelConfig(w http.ResponseWriter, r *http.Request)
	DeleteAgentModelConfig(w http.ResponseWriter, r *http.Request)
	CreateAgentMCPConfig(w http.ResponseWriter, r *http.Request)
	GetAgentMCPConfig(w http.ResponseWriter, r *http.Request)
	ListAgentMCPConfigs(w http.ResponseWriter, r *http.Request)
	UpdateAgentMCPConfig(w http.ResponseWriter, r *http.Request)
	DeleteAgentMCPConfig(w http.ResponseWriter, r *http.Request)
	ListMCPConfigAPIKeys(w http.ResponseWriter, r *http.Request)
	CreateMCPConfigAPIKey(w http.ResponseWriter, r *http.Request)
	RotateMCPConfigAPIKey(w http.ResponseWriter, r *http.Request)
	RevokeMCPConfigAPIKey(w http.ResponseWriter, r *http.Request)
	ListLLMConfigAPIKeys(w http.ResponseWriter, r *http.Request)
	CreateLLMConfigAPIKey(w http.ResponseWriter, r *http.Request)
	RotateLLMConfigAPIKey(w http.ResponseWriter, r *http.Request)
	RevokeLLMConfigAPIKey(w http.ResponseWriter, r *http.Request)
}

type agentConfigurationController struct {
	agentConfigService services.AgentConfigurationService
}

// NewAgentConfigurationController creates a new agent configuration controller
func NewAgentConfigurationController(service services.AgentConfigurationService) AgentConfigurationController {
	return &agentConfigurationController{agentConfigService: service}
}

// CreateAgentModelConfig handles POST /orgs/{ouID}/projects/{projName}/agents/{agentName}/model-configs
func (c *agentConfigurationController) CreateAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// The token subject, not a hardcoded literal. This column previously
	// recorded "system" for every config a user created, so it attributed real
	// user actions to the platform.
	createdBy := callerSubject(ctx)

	// Bind request body
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var specReq spec.CreateAgentModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Warn("CreateAgentModelConfig: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.ValidateConfigName(specReq.Name); err != nil {
		log.Warn("CreateAgentModelConfig: invalid name", "error", err)
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	if specReq.Type != models.AgentConfigTypeLLM {
		log.Warn("CreateAgentModelConfig: invalid type", "type", specReq.Type)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Type must be llm")
		return
	}

	// Convert spec request to models request
	req, err := convertCreateAgentModelConfigRequest(specReq)
	if err != nil {
		log.Warn("CreateAgentModelConfig: failed to convert request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	// Call service
	response, err := c.agentConfigService.Create(ctx, ouID, projectName, agentName, req, createdBy)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrAgentConfigAlreadyExists):
			utils.WriteErrorResponse(w, http.StatusConflict, "Agent configuration already exists")
			return
		case errors.Is(err, utils.ErrAgentNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, utils.ErrUnauthorized):
			utils.WriteErrorResponse(w, http.StatusUnauthorized, "Unauthorized access")
			return
		case errors.Is(err, utils.ErrForbidden):
			utils.WriteErrorResponse(w, http.StatusForbidden, "Forbidden")
			return
		case errors.Is(err, utils.ErrLLMProxyExists):
			utils.WriteErrorResponse(w, http.StatusConflict, "LLM proxy name collision: another agent in this org already uses the same model-config name")
			return
		default:
			log.Error("CreateAgentModelConfig: failed to create configuration", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create agent model configuration")
			return
		}
	}

	// Convert response to spec model
	specResponse := convertAgentModelConfigResponse(*response)
	utils.WriteSuccessResponse(w, http.StatusCreated, specResponse)
}

// GetAgentModelConfig handles GET /orgs/{ouID}/projects/{projName}/agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) GetAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	configID := r.PathValue(utils.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Warn("GetAgentModelConfig: invalid config ID", "config_id", configID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	response, err := c.agentConfigService.Get(ctx, configUUID, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("GetAgentModelConfig: failed to get configuration", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get configuration")
		return
	}

	if response.Type != models.AgentConfigTypeLLM {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
		return
	}

	// Convert response to spec model
	specResponse := convertAgentModelConfigResponse(*response)
	utils.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// ListAgentModelConfigs handles GET /orgs/{ouID}/projects/{projName}/agents/{agentName}/model-configs
func (c *agentConfigurationController) ListAgentModelConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	limit := getIntQueryParam(r, "limit", 20)
	offset := getIntQueryParam(r, "offset", 0)

	// Validate and clamp pagination parameters
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	response, err := c.agentConfigService.ListByType(ctx, ouID, projectName, agentName, models.AgentConfigTypeIDLLM, limit, offset)
	if err != nil {
		log.Error("ListAgentModelConfigs: failed to list configurations", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list configurations")
		return
	}

	// Convert response to spec model
	specResponse := convertAgentModelConfigListResponse(*response)
	utils.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// UpdateAgentModelConfig handles PUT /orgs/{ouID}/projects/{projName}/agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) UpdateAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	configID := r.PathValue(utils.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Warn("UpdateAgentModelConfig: invalid config ID", "config_id", configID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var specReq spec.UpdateAgentModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Warn("UpdateAgentModelConfig: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if specReq.Name != nil {
		if err := utils.ValidateConfigName(*specReq.Name); err != nil {
			log.Warn("UpdateAgentModelConfig: invalid name", "error", err)
			utils.WriteValidationErrorResponse(w, err)
			return
		}
	}

	// Convert spec request to models request
	req, err := convertUpdateAgentModelConfigRequest(specReq)
	if err != nil {
		log.Warn("UpdateAgentModelConfig: failed to convert request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	existing, err := c.agentConfigService.Get(ctx, configUUID, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("UpdateAgentModelConfig: failed to get configuration", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update configuration")
		return
	}
	if existing.Type != models.AgentConfigTypeLLM {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
		return
	}

	response, err := c.agentConfigService.Update(ctx, configUUID, ouID, projectName, agentName, req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrAgentConfigNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, utils.ErrLLMProxyExists):
			utils.WriteErrorResponse(w, http.StatusConflict, "LLM proxy name collision: another agent in this org already uses the same model-config name")
			return
		default:
			log.Error("UpdateAgentModelConfig: failed to update configuration", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update configuration")
			return
		}
	}

	// Convert response to spec model
	specResponse := convertAgentModelConfigResponse(*response)
	utils.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// DeleteAgentModelConfig handles DELETE /orgs/{ouID}/projects/{projName}/agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) DeleteAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	configID := r.PathValue(utils.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Warn("DeleteAgentModelConfig: invalid config ID", "config_id", configID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	existing, err := c.agentConfigService.Get(ctx, configUUID, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("DeleteAgentModelConfig: failed to get configuration", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete configuration")
		return
	}
	if existing.Type != models.AgentConfigTypeLLM {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
		return
	}

	if err := c.agentConfigService.Delete(ctx, configUUID, ouID, projectName, agentName); err != nil {
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

// CreateAgentMCPConfig handles POST /orgs/{ouID}/projects/{projName}/agents/{agentName}/mcp-configs
func (c *agentConfigurationController) CreateAgentMCPConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	createdBy := callerSubject(ctx)

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var specReq spec.CreateAgentModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Warn("CreateAgentMCPConfig: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	specReq.Type = models.AgentConfigTypeMCP

	if err := utils.ValidateConfigName(specReq.Name); err != nil {
		log.Warn("CreateAgentMCPConfig: invalid name", "error", err)
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	req, err := convertCreateAgentModelConfigRequest(specReq)
	if err != nil {
		log.Warn("CreateAgentMCPConfig: failed to convert request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	response, err := c.agentConfigService.Create(ctx, ouID, projectName, agentName, req, createdBy)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrAgentConfigAlreadyExists):
			utils.WriteErrorResponse(w, http.StatusConflict, "Agent MCP configuration already exists")
			return
		case errors.Is(err, utils.ErrAgentNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, utils.ErrUnauthorized):
			utils.WriteErrorResponse(w, http.StatusUnauthorized, "Unauthorized access")
			return
		case errors.Is(err, utils.ErrForbidden):
			utils.WriteErrorResponse(w, http.StatusForbidden, "Forbidden")
			return
		default:
			log.Error("CreateAgentMCPConfig: failed to create configuration", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create agent MCP configuration")
			return
		}
	}

	specResponse := convertAgentModelConfigResponse(*response)
	utils.WriteSuccessResponse(w, http.StatusCreated, specResponse)
}

// GetAgentMCPConfig handles GET /orgs/{ouID}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}
func (c *agentConfigurationController) GetAgentMCPConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	configID := r.PathValue(utils.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Warn("GetAgentMCPConfig: invalid config ID", "config_id", configID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	response, err := c.agentConfigService.GetMCP(ctx, configUUID, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("GetAgentMCPConfig: failed to get configuration", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get configuration")
		return
	}
	specResponse := convertAgentModelConfigResponse(*response)
	utils.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// ListAgentMCPConfigs handles GET /orgs/{ouID}/projects/{projName}/agents/{agentName}/mcp-configs
func (c *agentConfigurationController) ListAgentMCPConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	limit := getIntQueryParam(r, "limit", 20)
	offset := getIntQueryParam(r, "offset", 0)

	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	response, err := c.agentConfigService.ListMCP(ctx, ouID, projectName, agentName, limit, offset)
	if err != nil {
		log.Error("ListAgentMCPConfigs: failed to list configurations", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list configurations")
		return
	}

	specResponse := convertAgentModelConfigListResponse(*response)
	utils.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// UpdateAgentMCPConfig handles PUT /orgs/{ouID}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}
func (c *agentConfigurationController) UpdateAgentMCPConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	configID := r.PathValue(utils.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Warn("UpdateAgentMCPConfig: invalid config ID", "config_id", configID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	_, err = c.agentConfigService.GetMCP(ctx, configUUID, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("UpdateAgentMCPConfig: failed to get configuration", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update configuration")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var specReq spec.UpdateAgentModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Warn("UpdateAgentMCPConfig: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if specReq.Name != nil {
		if err := utils.ValidateConfigName(*specReq.Name); err != nil {
			log.Warn("UpdateAgentMCPConfig: invalid name", "error", err)
			utils.WriteValidationErrorResponse(w, err)
			return
		}
	}

	req, err := convertUpdateAgentModelConfigRequest(specReq)
	if err != nil {
		log.Warn("UpdateAgentMCPConfig: failed to convert request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	response, err := c.agentConfigService.UpdateMCP(ctx, configUUID, ouID, projectName, agentName, req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrAgentConfigNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		case errors.Is(err, utils.ErrMCPProxyNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		default:
			log.Error("UpdateAgentMCPConfig: failed to update configuration", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update configuration")
			return
		}
	}

	specResponse := convertAgentModelConfigResponse(*response)
	utils.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// DeleteAgentMCPConfig handles DELETE /orgs/{ouID}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}
func (c *agentConfigurationController) DeleteAgentMCPConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	configID := r.PathValue(utils.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Warn("DeleteAgentMCPConfig: invalid config ID", "config_id", configID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	_, err = c.agentConfigService.GetMCP(ctx, configUUID, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("DeleteAgentMCPConfig: failed to get configuration", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete configuration")
		return
	}
	if err := c.agentConfigService.DeleteMCP(ctx, configUUID, ouID, projectName, agentName); err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("DeleteAgentMCPConfig: failed to delete configuration", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete configuration")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Converter functions between spec and models

func convertCreateAgentModelConfigRequest(specReq spec.CreateAgentModelConfigRequest) (models.CreateAgentModelConfigRequest, error) {
	envMappings := make(map[string]models.EnvModelConfigRequest)
	for envName, envConfig := range specReq.EnvMappings {
		proxyName := resolveEnvConfigProxyName(envConfig)
		if proxyName == "" {
			return models.CreateAgentModelConfigRequest{}, fmt.Errorf("providerName or proxyName is required for environment %s", envName)
		}
		envMappings[envName] = models.EnvModelConfigRequest{
			ProviderName: proxyName,
			Configuration: models.EnvProviderConfiguration{
				Policies:   convertToModelPolicies(&envConfig.Configuration),
				Resilience: utils.ConvertSpecToModelResilience(envConfig.Configuration.Resilience),
			},
		}
	}

	var envVars []models.EnvironmentVariableConfig
	if specReq.EnvironmentVariables != nil {
		envVars = make([]models.EnvironmentVariableConfig, 0, len(specReq.EnvironmentVariables))
		for _, ev := range specReq.EnvironmentVariables {
			envVars = append(envVars, models.EnvironmentVariableConfig{Key: ev.Key, Name: ev.Name})
		}
	}

	return models.CreateAgentModelConfigRequest{
		Name:                 specReq.Name,
		Description:          getString(specReq.Description),
		Type:                 specReq.Type,
		EnvMappings:          envMappings,
		EnvironmentVariables: envVars,
	}, nil
}

func convertUpdateAgentModelConfigRequest(specReq spec.UpdateAgentModelConfigRequest) (models.UpdateAgentModelConfigRequest, error) {
	req := models.UpdateAgentModelConfigRequest{}

	if specReq.Name != nil {
		req.Name = *specReq.Name
	}
	if specReq.Description != nil {
		req.Description = *specReq.Description
	}
	if specReq.EnvMappings != nil {
		envMappings := make(map[string]models.EnvModelConfigRequest)
		for envName, envConfig := range *specReq.EnvMappings {
			proxyName := resolveEnvConfigProxyName(envConfig)
			if proxyName == "" {
				return models.UpdateAgentModelConfigRequest{}, fmt.Errorf("providerName or proxyName is required for environment %s", envName)
			}
			envMappings[envName] = models.EnvModelConfigRequest{
				ProviderName: proxyName,
				Configuration: models.EnvProviderConfiguration{
					Policies:   convertToModelPolicies(&envConfig.Configuration),
					Resilience: utils.ConvertSpecToModelResilience(envConfig.Configuration.Resilience),
				},
			}
		}
		req.EnvMappings = envMappings
	}
	if specReq.EnvironmentVariables != nil {
		req.EnvironmentVariables = make([]models.EnvironmentVariableConfig, 0, len(specReq.EnvironmentVariables))
		for _, ev := range specReq.EnvironmentVariables {
			req.EnvironmentVariables = append(req.EnvironmentVariables, models.EnvironmentVariableConfig{Key: ev.Key, Name: ev.Name})
		}
	}

	return req, nil
}

func convertAgentModelConfigResponse(modelResp models.AgentModelConfigResponse) spec.AgentModelConfigResponse {
	envModelConfig := make(map[string]spec.EnvProviderConfigMappings)
	for envName, envConfig := range modelResp.EnvModelConfig {
		specEnvConfig := spec.EnvProviderConfigMappings{
			EnvironmentName: envConfig.EnvironmentName,
		}

		// Build configuration object with proxy URL and auth info
		if envConfig.LLMProxy != nil {
			providerName := ""
			if envConfig.LLMProxy.ProviderName != nil {
				providerName = *envConfig.LLMProxy.ProviderName
			}
			// Guard nil dereference on ProxyUUID (CRIT-4).
			proxyUUID := ""
			if envConfig.LLMProxy.ProxyUUID != nil {
				proxyUUID = *envConfig.LLMProxy.ProxyUUID
			}
			modelEnvConfig := &spec.ProviderConfig{
				ProxyUuid:        proxyUUID,
				ProviderName:     providerName,
				Policies:         convertToSpecPolicies(&envConfig.LLMProxy.Policies),
				ProviderPolicies: convertToSpecPolicies(&envConfig.LLMProxy.ProviderPolicies),
				Resilience:       utils.ConvertModelToSpecResilience(envConfig.LLMProxy.Resilience),
			}
			if envConfig.LLMProxy.ProviderUUID != nil {
				modelEnvConfig.SetProviderUuid(*envConfig.LLMProxy.ProviderUUID)
			}
			if modelResp.Type == "mcp" {
				modelEnvConfig.SetProxyName(providerName)
				modelEnvConfig.SetProxyId(providerName)
				modelEnvConfig.SetMcpProxyName(providerName)
				modelEnvConfig.SetMcpProxyId(providerName)
			}

			// Add proxy URL if present
			if envConfig.LLMProxy.URL != nil {
				modelEnvConfig.Url = *envConfig.LLMProxy.URL
			}

			// Build auth info if API key present
			if envConfig.LLMProxy.APIKey != nil {
				authType := "api-key"
				authIn := "header"
				headerName := "api-key"
				if envConfig.LLMProxy.AuthHeaderName != nil && *envConfig.LLMProxy.AuthHeaderName != "" {
					headerName = *envConfig.LLMProxy.AuthHeaderName
				}
				modelEnvConfig.AuthInfo = &spec.AuthInfo{
					Type:  authType,
					In:    authIn,
					Name:  headerName,
					Value: envConfig.LLMProxy.APIKey,
				}
			}

			// Set status (default to "active" if proxy exists)
			status := "active"
			modelEnvConfig.Status = &status

			specEnvConfig.Configuration = modelEnvConfig
		}
		envModelConfig[envName] = specEnvConfig
	}

	envVars := make([]spec.EnvironmentVariableConfig, len(modelResp.EnvironmentVariables))
	for i, envVar := range modelResp.EnvironmentVariables {
		envVars[i] = spec.EnvironmentVariableConfig{
			Name: envVar.Name,
			Key:  envVar.Key,
		}
	}

	return spec.AgentModelConfigResponse{
		Uuid:                 modelResp.UUID,
		Name:                 modelResp.Name,
		Description:          getStringPtr(modelResp.Description),
		AgentId:              modelResp.AgentID,
		Type:                 modelResp.Type,
		ProjectName:          modelResp.ProjectName,
		EnvMappings:          envModelConfig,
		EnvironmentVariables: envVars,
		CreatedAt:            modelResp.CreatedAt,
		UpdatedAt:            modelResp.UpdatedAt,
	}
}

func resolveEnvConfigProxyName(envConfig spec.EnvModelConfigRequest) string {
	switch {
	case envConfig.GetProviderName() != "":
		return envConfig.GetProviderName()
	case envConfig.GetProxyName() != "":
		return envConfig.GetProxyName()
	case envConfig.GetProxyId() != "":
		return envConfig.GetProxyId()
	case envConfig.GetMcpProxyName() != "":
		return envConfig.GetMcpProxyName()
	case envConfig.GetMcpProxyId() != "":
		return envConfig.GetMcpProxyId()
	default:
		return ""
	}
}

func convertAgentModelConfigListResponse(modelResp models.AgentModelConfigListResponse) spec.AgentModelConfigListResponse {
	configs := make([]spec.AgentModelConfigListItem, len(modelResp.Configs))
	for i, config := range modelResp.Configs {
		configs[i] = spec.AgentModelConfigListItem{
			Uuid:        config.UUID,
			Name:        config.Name,
			Description: getStringPtr(config.Description),
			AgentId:     config.AgentID,
			Type:        config.Type,
			ProjectName: config.ProjectName,
			CreatedAt:   config.CreatedAt,
		}
	}

	return spec.AgentModelConfigListResponse{
		Configs: configs,
		Pagination: spec.PaginationInfo{
			Count:  int32(modelResp.Pagination.Count),
			Offset: int32(modelResp.Pagination.Offset),
			Limit:  int32(modelResp.Pagination.Limit),
		},
	}
}

// Helper functions

func convertToSpecPolicies(modelPolicies *[]models.LLMPolicy) []spec.LLMPolicy {
	if modelPolicies == nil {
		return nil
	}
	policies := make([]spec.LLMPolicy, len(*modelPolicies))
	for i, policy := range *modelPolicies {
		policies[i] = spec.LLMPolicy{
			Name:    policy.Name,
			Version: policy.Version,
			Paths:   convertToSpecPolicyPaths(&policy.Paths),
		}
	}
	return policies
}

func convertToSpecPolicyPaths(modelPaths *[]models.LLMPolicyPath) []spec.LLMPolicyPath {
	if modelPaths == nil {
		return nil
	}
	paths := make([]spec.LLMPolicyPath, len(*modelPaths))
	for i, path := range *modelPaths {
		paths[i] = spec.LLMPolicyPath{
			Path:    path.Path,
			Methods: path.Methods,
			Params:  path.Params,
		}
	}
	return paths
}

func convertToModelPolicies(specConfig *spec.EnvProviderConfiguration) []models.LLMPolicy {
	if specConfig == nil {
		return nil
	}
	policies := make([]models.LLMPolicy, len(specConfig.Policies))
	for i, policy := range specConfig.Policies {
		policies[i] = models.LLMPolicy{
			Name:    policy.Name,
			Version: policy.Version,
			Paths:   convertToModelPolicyPaths(&policy.Paths),
		}
	}
	return policies
}

func convertToModelPolicyPaths(specPaths *[]spec.LLMPolicyPath) []models.LLMPolicyPath {
	if specPaths == nil {
		return nil
	}
	paths := make([]models.LLMPolicyPath, len(*specPaths))
	for i, path := range *specPaths {
		paths[i] = models.LLMPolicyPath{
			Path:    path.Path,
			Methods: path.Methods,
			Params:  path.Params,
		}
	}
	return paths
}

func getString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}

func getStringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// configEnvAPIKeyParams collects the common path parameters for the per-config
// MCP API key handlers and validates the config ID. The org is taken from the
// JWT-resolved OUID in the request context (via OUIDFromRequest), not the raw
// {orgName} path handle — the config repository is keyed by ou_id (a UUID), so
// passing the path handle would never match and every call would 404.
func (c *agentConfigurationController) configEnvAPIKeyParams(w http.ResponseWriter, r *http.Request) (ouID, projName, agentName, envName string, configUUID uuid.UUID, ok bool) {
	ouID = middleware.OUIDFromRequest(r)
	projName = r.PathValue(utils.PathParamProjName)
	agentName = r.PathValue(utils.PathParamAgentName)
	envName = r.PathValue("envName")
	configID := r.PathValue(utils.PathParamConfigId)
	parsed, err := uuid.Parse(configID)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return "", "", "", "", uuid.Nil, false
	}
	return ouID, projName, agentName, envName, parsed, true
}

// writeConfigAPIKeyError maps the shared per-config API key service errors to HTTP
// responses for the create/rotate/revoke handlers. operation labels the log line.
func (c *agentConfigurationController) writeConfigAPIKeyError(w http.ResponseWriter, log *slog.Logger, operation string, err error) {
	switch {
	case errors.Is(err, utils.ErrAgentConfigNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
	case errors.Is(err, utils.ErrAgentConfigNotExternal):
		utils.WriteErrorResponse(w, http.StatusForbidden, "API key management is only available for external agents")
	case errors.Is(err, utils.ErrGatewayNotFound):
		utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
	default:
		log.Error(operation+": failed", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to manage API key")
	}
}

// apiKeyCreateNameDisplayName unwraps the optional name/displayName fields of a
// create-key request and enforces that at least one is provided. On failure it
// writes a 400 and returns ok=false.
func apiKeyCreateNameDisplayName(w http.ResponseWriter, name, displayName *string) (string, string, bool) {
	n := ""
	if name != nil {
		n = *name
	}
	dn := ""
	if displayName != nil {
		dn = *displayName
	}
	if n == "" && dn == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "At least one of 'name' or 'displayName' must be provided")
		return "", "", false
	}
	return n, dn, true
}

// ListMCPConfigAPIKeys handles GET .../mcp-configs/{configId}/environments/{envName}/api-keys
func (c *agentConfigurationController) ListMCPConfigAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID, projName, agentName, envName, configUUID, ok := c.configEnvAPIKeyParams(w, r)
	if !ok {
		return
	}

	response, err := c.agentConfigService.ListMCPConfigAPIKeys(ctx, ouID, projName, agentName, configUUID, envName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("ListMCPConfigAPIKeys: failed to list API keys", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list API keys")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// CreateMCPConfigAPIKey handles POST .../mcp-configs/{configId}/environments/{envName}/api-keys
func (c *agentConfigurationController) CreateMCPConfigAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID, projName, agentName, envName, configUUID, ok := c.configEnvAPIKeyParams(w, r)
	if !ok {
		return
	}

	var specReq spec.CreateLLMAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	name, displayName, ok := apiKeyCreateNameDisplayName(w, specReq.Name, specReq.DisplayName)
	if !ok {
		return
	}

	attempt, ok := beginConfigAPIKeyAudit(w, r, "CreateMCPConfigAPIKey", "Failed to issue API key", audit.ActionAPIKeyCreate, audit.APIKeyOwnerMCPConfig,
		ouID, projName, agentName, envName, configUUID.String(), name)
	if !ok {
		return
	}

	response, err := c.agentConfigService.CreateMCPConfigAPIKey(ctx, ouID, projName, agentName, configUUID, envName, &models.CreateAPIKeyRequest{
		Name:        name,
		DisplayName: displayName,
		ExpiresAt:   specReq.ExpiresAt,
	})
	if err != nil {
		attempt.Complete(ctx, err)
		c.writeConfigAPIKeyError(w, log, "CreateMCPConfigAPIKey", err)
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

// RotateMCPConfigAPIKey handles PUT .../mcp-configs/{configId}/environments/{envName}/api-keys/{keyName}
func (c *agentConfigurationController) RotateMCPConfigAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID, projName, agentName, envName, configUUID, ok := c.configEnvAPIKeyParams(w, r)
	if !ok {
		return
	}
	keyName := r.PathValue("keyName")

	var specReq spec.RotateLLMAPIKeyRequest
	// Body is optional for rotation: an empty body (io.EOF) is allowed, but any
	// other decode error indicates a malformed request and must be rejected.
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil && !errors.Is(err, io.EOF) {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	attempt, ok := beginConfigAPIKeyAudit(w, r, "RotateMCPConfigAPIKey", "Failed to issue API key", audit.ActionAPIKeyRotate, audit.APIKeyOwnerMCPConfig,
		ouID, projName, agentName, envName, configUUID.String(), keyName)
	if !ok {
		return
	}

	response, err := c.agentConfigService.RotateMCPConfigAPIKey(ctx, ouID, projName, agentName, configUUID, envName, keyName, &models.RotateAPIKeyRequest{
		DisplayName: specReq.DisplayName,
		ExpiresAt:   specReq.ExpiresAt,
	})
	if err != nil {
		attempt.Complete(ctx, err)
		c.writeConfigAPIKeyError(w, log, "RotateMCPConfigAPIKey", err)
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// RevokeMCPConfigAPIKey handles DELETE .../mcp-configs/{configId}/environments/{envName}/api-keys/{keyName}
func (c *agentConfigurationController) RevokeMCPConfigAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID, projName, agentName, envName, configUUID, ok := c.configEnvAPIKeyParams(w, r)
	if !ok {
		return
	}
	keyName := r.PathValue("keyName")

	attempt, ok := beginConfigAPIKeyAudit(w, r, "RevokeMCPConfigAPIKey", "Failed to revoke API key", audit.ActionAPIKeyRevoke, audit.APIKeyOwnerMCPConfig,
		ouID, projName, agentName, envName, configUUID.String(), keyName)
	if !ok {
		return
	}

	if err := c.agentConfigService.RevokeMCPConfigAPIKey(ctx, ouID, projName, agentName, configUUID, envName, keyName); err != nil {
		attempt.Complete(ctx, err)
		c.writeConfigAPIKeyError(w, log, "RevokeMCPConfigAPIKey", err)
		return
	}
	attempt.Complete(ctx, nil)
	w.WriteHeader(http.StatusNoContent)
}

// ListLLMConfigAPIKeys handles GET .../model-configs/{configId}/environments/{envName}/api-keys
func (c *agentConfigurationController) ListLLMConfigAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID, projName, agentName, envName, configUUID, ok := c.configEnvAPIKeyParams(w, r)
	if !ok {
		return
	}

	response, err := c.agentConfigService.ListLLMConfigAPIKeys(ctx, ouID, projName, agentName, configUUID, envName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("ListLLMConfigAPIKeys: failed to list API keys", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list API keys")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// CreateLLMConfigAPIKey handles POST .../model-configs/{configId}/environments/{envName}/api-keys
func (c *agentConfigurationController) CreateLLMConfigAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID, projName, agentName, envName, configUUID, ok := c.configEnvAPIKeyParams(w, r)
	if !ok {
		return
	}

	var specReq spec.CreateLLMAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	name, displayName, ok := apiKeyCreateNameDisplayName(w, specReq.Name, specReq.DisplayName)
	if !ok {
		return
	}

	attempt, ok := beginConfigAPIKeyAudit(w, r, "CreateLLMConfigAPIKey", "Failed to issue API key", audit.ActionAPIKeyCreate, audit.APIKeyOwnerModelConfig,
		ouID, projName, agentName, envName, configUUID.String(), name)
	if !ok {
		return
	}

	response, err := c.agentConfigService.CreateLLMConfigAPIKey(ctx, ouID, projName, agentName, configUUID, envName, &models.CreateAPIKeyRequest{
		Name:        name,
		DisplayName: displayName,
		ExpiresAt:   specReq.ExpiresAt,
	})
	if err != nil {
		attempt.Complete(ctx, err)
		c.writeConfigAPIKeyError(w, log, "CreateLLMConfigAPIKey", err)
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

// RotateLLMConfigAPIKey handles PUT .../model-configs/{configId}/environments/{envName}/api-keys/{keyName}
func (c *agentConfigurationController) RotateLLMConfigAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID, projName, agentName, envName, configUUID, ok := c.configEnvAPIKeyParams(w, r)
	if !ok {
		return
	}
	keyName := r.PathValue("keyName")

	var specReq spec.RotateLLMAPIKeyRequest
	// Body is optional for rotation: an empty body (io.EOF) is allowed, but any
	// other decode error indicates a malformed request and must be rejected.
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil && !errors.Is(err, io.EOF) {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	attempt, ok := beginConfigAPIKeyAudit(w, r, "RotateLLMConfigAPIKey", "Failed to issue API key", audit.ActionAPIKeyRotate, audit.APIKeyOwnerModelConfig,
		ouID, projName, agentName, envName, configUUID.String(), keyName)
	if !ok {
		return
	}

	response, err := c.agentConfigService.RotateLLMConfigAPIKey(ctx, ouID, projName, agentName, configUUID, envName, keyName, &models.RotateAPIKeyRequest{
		DisplayName: specReq.DisplayName,
		ExpiresAt:   specReq.ExpiresAt,
	})
	if err != nil {
		attempt.Complete(ctx, err)
		c.writeConfigAPIKeyError(w, log, "RotateLLMConfigAPIKey", err)
		return
	}
	attempt.Complete(ctx, nil)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// RevokeLLMConfigAPIKey handles DELETE .../model-configs/{configId}/environments/{envName}/api-keys/{keyName}
func (c *agentConfigurationController) RevokeLLMConfigAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID, projName, agentName, envName, configUUID, ok := c.configEnvAPIKeyParams(w, r)
	if !ok {
		return
	}
	keyName := r.PathValue("keyName")

	attempt, ok := beginConfigAPIKeyAudit(w, r, "RevokeLLMConfigAPIKey", "Failed to revoke API key", audit.ActionAPIKeyRevoke, audit.APIKeyOwnerModelConfig,
		ouID, projName, agentName, envName, configUUID.String(), keyName)
	if !ok {
		return
	}

	if err := c.agentConfigService.RevokeLLMConfigAPIKey(ctx, ouID, projName, agentName, configUUID, envName, keyName); err != nil {
		attempt.Complete(ctx, err)
		c.writeConfigAPIKeyError(w, log, "RevokeLLMConfigAPIKey", err)
		return
	}
	attempt.Complete(ctx, nil)
	w.WriteHeader(http.StatusNoContent)
}

// callerSubject returns the acting user's token subject for created_by
// attribution, falling back to a marker when there is no user behind the call
// (a system client) rather than to a literal that would misattribute the change.
func callerSubject(ctx context.Context) string {
	if claims := jwtassertion.GetTokenClaims(ctx); claims != nil && claims.Sub != "" {
		return claims.Sub
	}
	return "system"
}
