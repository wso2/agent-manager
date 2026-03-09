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
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// Note: uuid import retained for configUUID parsing in path parameters

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

	createdBy := "system"

	// Bind request body
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var specReq spec.CreateAgentModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Error("CreateAgentModelConfig: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.ValidateConfigName(specReq.Name); err != nil {
		log.Warn("CreateAgentModelConfig: invalid name", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	validTypes := map[string]bool{"llm": true, "mcp": true, "other": true}
	if !validTypes[specReq.Type] {
		log.Warn("CreateAgentModelConfig: invalid type", "type", specReq.Type)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Type must be one of: llm, mcp, other")
		return
	}

	// Convert spec request to models request
	req, err := convertCreateAgentModelConfigRequest(specReq)
	if err != nil {
		log.Error("CreateAgentModelConfig: failed to convert request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
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
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		case errors.Is(err, utils.ErrUnauthorized):
			utils.WriteErrorResponse(w, http.StatusUnauthorized, "Unauthorized access")
			return
		case errors.Is(err, utils.ErrForbidden):
			utils.WriteErrorResponse(w, http.StatusForbidden, "Forbidden")
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

// GetAgentModelConfig handles GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) GetAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	configID := r.PathValue(utils.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Error("GetAgentModelConfig: invalid config ID", "configId", configID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	response, err := c.agentConfigService.Get(ctx, configUUID, orgName, projectName, agentName)
	if err != nil {
		if errors.Is(err, utils.ErrAgentConfigNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("GetAgentModelConfig: failed to get configuration", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get configuration")
		return
	}

	// Convert response to spec model
	specResponse := convertAgentModelConfigResponse(*response)
	utils.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// ListAgentModelConfigs handles GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs
func (c *agentConfigurationController) ListAgentModelConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
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

	response, err := c.agentConfigService.List(ctx, orgName, projectName, agentName, limit, offset)
	if err != nil {
		log.Error("ListAgentModelConfigs: failed to list configurations", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list configurations")
		return
	}

	// Convert response to spec model
	specResponse := convertAgentModelConfigListResponse(*response)
	utils.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// UpdateAgentModelConfig handles PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) UpdateAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	configID := r.PathValue(utils.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Error("UpdateAgentModelConfig: invalid config ID", "configId", configID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var specReq spec.UpdateAgentModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Error("UpdateAgentModelConfig: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if specReq.Name != nil {
		if err := utils.ValidateConfigName(*specReq.Name); err != nil {
			log.Warn("UpdateAgentModelConfig: invalid name", "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
	}

	// Convert spec request to models request
	req, err := convertUpdateAgentModelConfigRequest(specReq)
	if err != nil {
		log.Error("UpdateAgentModelConfig: failed to convert request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	response, err := c.agentConfigService.Update(ctx, configUUID, orgName, projectName, agentName, req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrAgentConfigNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
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

// DeleteAgentModelConfig handles DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) DeleteAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	projectName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	configID := r.PathValue(utils.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Error("DeleteAgentModelConfig: invalid config ID", "configId", configID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	if err := c.agentConfigService.Delete(ctx, configUUID, orgName, projectName, agentName); err != nil {
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

// Converter functions between spec and models

func convertCreateAgentModelConfigRequest(specReq spec.CreateAgentModelConfigRequest) (models.CreateAgentModelConfigRequest, error) {
	envMappings := make(map[string]models.EnvModelConfigRequest)
	for envName, envConfig := range specReq.EnvMappings {
		if envConfig.ProviderName == "" {
			return models.CreateAgentModelConfigRequest{}, fmt.Errorf("providerName is required for environment %s", envName)
		}
		envMappings[envName] = models.EnvModelConfigRequest{
			ProviderName: envConfig.ProviderName,
			Configuration: models.EnvProviderConfiguration{
				Policies: convertToModelPolicies(&envConfig.Configuration),
			},
		}
	}

	var envVars []models.EnvironmentVariableConfig
	for _, ev := range specReq.EnvironmentVariables {
		envVars = append(envVars, models.EnvironmentVariableConfig{Key: ev.Key, Name: ev.Name})
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
			if envConfig.ProviderName == "" {
				return models.UpdateAgentModelConfigRequest{}, fmt.Errorf("providerName is required for environment %s", envName)
			}
			envMappings[envName] = models.EnvModelConfigRequest{
				ProviderName: envConfig.ProviderName,
				Configuration: models.EnvProviderConfiguration{
					Policies: convertToModelPolicies(&envConfig.Configuration),
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
				ProxyUuid:    proxyUUID,
				ProviderName: providerName,
				Policies:     convertToSpecPolicies(&envConfig.LLMProxy.Policies),
			}

			// Add proxy URL if present
			if envConfig.LLMProxy.URL != nil {
				modelEnvConfig.Url = *envConfig.LLMProxy.URL
			}

			// Build auth info if API key present
			if envConfig.LLMProxy.APIKey != nil {
				authType := "api-key"
				authIn := "header"
				authName := "API-Key"
				modelEnvConfig.AuthInfo = &spec.AuthInfo{
					Type:  authType,
					In:    authIn,
					Name:  authName,
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
		OrganizationName:     modelResp.OrganizationName,
		ProjectName:          modelResp.ProjectName,
		EnvMappings:          &envModelConfig,
		EnvironmentVariables: envVars,
		CreatedAt:            modelResp.CreatedAt,
		UpdatedAt:            modelResp.UpdatedAt,
	}
}

func convertAgentModelConfigListResponse(modelResp models.AgentModelConfigListResponse) spec.AgentModelConfigListResponse {
	configs := make([]spec.AgentModelConfigListItem, len(modelResp.Configs))
	for i, config := range modelResp.Configs {
		configs[i] = spec.AgentModelConfigListItem{
			Uuid:             config.UUID,
			Name:             config.Name,
			Description:      getStringPtr(config.Description),
			AgentId:          config.AgentID,
			Type:             config.Type,
			OrganizationName: config.OrganizationName,
			ProjectName:      config.ProjectName,
			CreatedAt:        config.CreatedAt,
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
