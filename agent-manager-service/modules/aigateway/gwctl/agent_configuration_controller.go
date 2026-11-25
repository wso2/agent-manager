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

package gwctl

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconv"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
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
	agentConfigService gwsvc.AgentConfigurationService
}

// NewAgentConfigurationController creates a new agent configuration controller
func NewAgentConfigurationController(service gwsvc.AgentConfigurationService) AgentConfigurationController {
	return &agentConfigurationController{agentConfigService: service}
}

// CreateAgentModelConfig handles POST /orgs/{ouID}/projects/{projName}/agents/{agentName}/model-configs
func (c *agentConfigurationController) CreateAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)

	// The token subject, not a hardcoded literal. This column previously
	// recorded "system" for every config a user created, so it attributed real
	// user actions to the platform.
	createdBy := callerSubject(ctx)

	// Bind request body
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var specReq gwspec.CreateAgentModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Error("CreateAgentModelConfig: failed to decode request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := gwutil.ValidateConfigName(specReq.Name); err != nil {
		log.Warn("CreateAgentModelConfig: invalid name", "error", err)
		httpx.WriteValidationErrorResponse(w, err)
		return
	}

	if specReq.Type != gwmodels.AgentConfigTypeLLM {
		log.Warn("CreateAgentModelConfig: invalid type", "type", specReq.Type)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Type must be llm")
		return
	}

	// Convert spec request to models request
	req, err := convertCreateAgentModelConfigRequest(specReq)
	if err != nil {
		log.Error("CreateAgentModelConfig: failed to convert request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	// Call service
	response, err := c.agentConfigService.Create(ctx, ouID, projectName, agentName, req, createdBy)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrAgentConfigAlreadyExists):
			httpx.WriteErrorResponse(w, http.StatusConflict, "Agent configuration already exists")
			return
		case errors.Is(err, gwerr.ErrAgentNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, gwerr.ErrMCPProxyNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, httpx.ErrUnauthorized):
			httpx.WriteErrorResponse(w, http.StatusUnauthorized, "Unauthorized access")
			return
		case errors.Is(err, httpx.ErrForbidden):
			httpx.WriteErrorResponse(w, http.StatusForbidden, "Forbidden")
			return
		case errors.Is(err, gwerr.ErrLLMProxyExists):
			httpx.WriteErrorResponse(w, http.StatusConflict, "LLM proxy name collision: another agent in this org already uses the same model-config name")
			return
		default:
			log.Error("CreateAgentModelConfig: failed to create configuration", "error", err)
			if writeIfNotImplemented(w, err) {
				return
			}
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create agent model configuration")
			return
		}
	}

	// Convert response to spec model
	specResponse := convertAgentModelConfigResponse(*response)
	httpx.WriteSuccessResponse(w, http.StatusCreated, specResponse)
}

// GetAgentModelConfig handles GET /orgs/{ouID}/projects/{projName}/agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) GetAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)
	configID := r.PathValue(gwutil.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Error("GetAgentModelConfig: invalid config ID", "configId", configID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	response, err := c.agentConfigService.Get(ctx, configUUID, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, gwerr.ErrAgentConfigNotFound) {
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("GetAgentModelConfig: failed to get configuration", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get configuration")
		return
	}

	if response.Type != gwmodels.AgentConfigTypeLLM {
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
		return
	}

	// Convert response to spec model
	specResponse := convertAgentModelConfigResponse(*response)
	httpx.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// ListAgentModelConfigs handles GET /orgs/{ouID}/projects/{projName}/agents/{agentName}/model-configs
func (c *agentConfigurationController) ListAgentModelConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)
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

	response, err := c.agentConfigService.ListByType(ctx, ouID, projectName, agentName, gwmodels.AgentConfigTypeIDLLM, limit, offset)
	if err != nil {
		log.Error("ListAgentModelConfigs: failed to list configurations", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list configurations")
		return
	}

	// Convert response to spec model
	specResponse := convertAgentModelConfigListResponse(*response)
	httpx.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// UpdateAgentModelConfig handles PUT /orgs/{ouID}/projects/{projName}/agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) UpdateAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)
	configID := r.PathValue(gwutil.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Error("UpdateAgentModelConfig: invalid config ID", "configId", configID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var specReq gwspec.UpdateAgentModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Error("UpdateAgentModelConfig: failed to decode request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if specReq.Name != nil {
		if err := gwutil.ValidateConfigName(*specReq.Name); err != nil {
			log.Warn("UpdateAgentModelConfig: invalid name", "error", err)
			httpx.WriteValidationErrorResponse(w, err)
			return
		}
	}

	// Convert spec request to models request
	req, err := convertUpdateAgentModelConfigRequest(specReq)
	if err != nil {
		log.Error("UpdateAgentModelConfig: failed to convert request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	existing, err := c.agentConfigService.Get(ctx, configUUID, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, gwerr.ErrAgentConfigNotFound) {
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("UpdateAgentModelConfig: failed to get configuration", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update configuration")
		return
	}
	if existing.Type != gwmodels.AgentConfigTypeLLM {
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
		return
	}

	response, err := c.agentConfigService.Update(ctx, configUUID, ouID, projectName, agentName, req)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrAgentConfigNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		case errors.Is(err, gwerr.ErrAgentNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, gwerr.ErrMCPProxyNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, httpx.ErrUnauthorized):
			httpx.WriteErrorResponse(w, http.StatusUnauthorized, "Unauthorized access")
			return
		case errors.Is(err, httpx.ErrForbidden):
			httpx.WriteErrorResponse(w, http.StatusForbidden, "Forbidden")
			return
		case errors.Is(err, gwerr.ErrLLMProxyExists):
			httpx.WriteErrorResponse(w, http.StatusConflict, "LLM proxy name collision: another agent in this org already uses the same model-config name")
			return
		default:
			log.Error("UpdateAgentModelConfig: failed to update configuration", "error", err)
			if writeIfNotImplemented(w, err) {
				return
			}
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update configuration")
			return
		}
	}

	// Convert response to spec model
	specResponse := convertAgentModelConfigResponse(*response)
	httpx.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// DeleteAgentModelConfig handles DELETE /orgs/{ouID}/projects/{projName}/agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) DeleteAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)
	configID := r.PathValue(gwutil.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Error("DeleteAgentModelConfig: invalid config ID", "configId", configID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	existing, err := c.agentConfigService.Get(ctx, configUUID, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, gwerr.ErrAgentConfigNotFound) {
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("DeleteAgentModelConfig: failed to get configuration", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete configuration")
		return
	}
	if existing.Type != gwmodels.AgentConfigTypeLLM {
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
		return
	}

	if err := c.agentConfigService.Delete(ctx, configUUID, ouID, projectName, agentName); err != nil {
		if errors.Is(err, gwerr.ErrAgentConfigNotFound) {
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("DeleteAgentModelConfig: failed to delete configuration", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete configuration")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// CreateAgentMCPConfig handles POST /orgs/{ouID}/projects/{projName}/agents/{agentName}/mcp-configs
func (c *agentConfigurationController) CreateAgentMCPConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)
	createdBy := callerSubject(ctx)

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var specReq gwspec.CreateAgentModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Error("CreateAgentMCPConfig: failed to decode request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	specReq.Type = gwmodels.AgentConfigTypeMCP

	if err := gwutil.ValidateConfigName(specReq.Name); err != nil {
		log.Warn("CreateAgentMCPConfig: invalid name", "error", err)
		httpx.WriteValidationErrorResponse(w, err)
		return
	}

	req, err := convertCreateAgentModelConfigRequest(specReq)
	if err != nil {
		log.Error("CreateAgentMCPConfig: failed to convert request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	response, err := c.agentConfigService.Create(ctx, ouID, projectName, agentName, req, createdBy)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrAgentConfigAlreadyExists):
			httpx.WriteErrorResponse(w, http.StatusConflict, "Agent MCP configuration already exists")
			return
		case errors.Is(err, gwerr.ErrAgentNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		case errors.Is(err, gwerr.ErrMCPProxyNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, httpx.ErrUnauthorized):
			httpx.WriteErrorResponse(w, http.StatusUnauthorized, "Unauthorized access")
			return
		case errors.Is(err, httpx.ErrForbidden):
			httpx.WriteErrorResponse(w, http.StatusForbidden, "Forbidden")
			return
		default:
			log.Error("CreateAgentMCPConfig: failed to create configuration", "error", err)
			if writeIfNotImplemented(w, err) {
				return
			}
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create agent MCP configuration")
			return
		}
	}

	specResponse := convertAgentModelConfigResponse(*response)
	httpx.WriteSuccessResponse(w, http.StatusCreated, specResponse)
}

// GetAgentMCPConfig handles GET /orgs/{ouID}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}
func (c *agentConfigurationController) GetAgentMCPConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)
	configID := r.PathValue(gwutil.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Error("GetAgentMCPConfig: invalid config ID", "configId", configID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	response, err := c.agentConfigService.GetMCP(ctx, configUUID, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, gwerr.ErrAgentConfigNotFound) {
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("GetAgentMCPConfig: failed to get configuration", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get configuration")
		return
	}
	specResponse := convertAgentModelConfigResponse(*response)
	httpx.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// ListAgentMCPConfigs handles GET /orgs/{ouID}/projects/{projName}/agents/{agentName}/mcp-configs
func (c *agentConfigurationController) ListAgentMCPConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)
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
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list configurations")
		return
	}

	specResponse := convertAgentModelConfigListResponse(*response)
	httpx.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// UpdateAgentMCPConfig handles PUT /orgs/{ouID}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}
func (c *agentConfigurationController) UpdateAgentMCPConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)
	configID := r.PathValue(gwutil.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Error("UpdateAgentMCPConfig: invalid config ID", "configId", configID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	_, err = c.agentConfigService.GetMCP(ctx, configUUID, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, gwerr.ErrAgentConfigNotFound) {
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("UpdateAgentMCPConfig: failed to get configuration", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update configuration")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20) // 1MB limit
	var specReq gwspec.UpdateAgentModelConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Error("UpdateAgentMCPConfig: failed to decode request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if specReq.Name != nil {
		if err := gwutil.ValidateConfigName(*specReq.Name); err != nil {
			log.Warn("UpdateAgentMCPConfig: invalid name", "error", err)
			httpx.WriteValidationErrorResponse(w, err)
			return
		}
	}

	req, err := convertUpdateAgentModelConfigRequest(specReq)
	if err != nil {
		log.Error("UpdateAgentMCPConfig: failed to convert request", "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid request: %v", err))
		return
	}

	response, err := c.agentConfigService.UpdateMCP(ctx, configUUID, ouID, projectName, agentName, req)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrAgentConfigNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		case errors.Is(err, gwerr.ErrAgentNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		case errors.Is(err, gwerr.ErrMCPProxyNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "MCP proxy not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, httpx.ErrUnauthorized):
			httpx.WriteErrorResponse(w, http.StatusUnauthorized, "Unauthorized access")
			return
		case errors.Is(err, httpx.ErrForbidden):
			httpx.WriteErrorResponse(w, http.StatusForbidden, "Forbidden")
			return
		default:
			log.Error("UpdateAgentMCPConfig: failed to update configuration", "error", err)
			if writeIfNotImplemented(w, err) {
				return
			}
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update configuration")
			return
		}
	}

	specResponse := convertAgentModelConfigResponse(*response)
	httpx.WriteSuccessResponse(w, http.StatusOK, specResponse)
}

// DeleteAgentMCPConfig handles DELETE /orgs/{ouID}/projects/{projName}/agents/{agentName}/mcp-configs/{configId}
func (c *agentConfigurationController) DeleteAgentMCPConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	projectName := r.PathValue(gwutil.PathParamProjName)
	agentName := r.PathValue(gwutil.PathParamAgentName)
	configID := r.PathValue(gwutil.PathParamConfigId)

	configUUID, err := uuid.Parse(configID)
	if err != nil {
		log.Error("DeleteAgentMCPConfig: invalid config ID", "configId", configID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return
	}

	_, err = c.agentConfigService.GetMCP(ctx, configUUID, ouID, projectName, agentName)
	if err != nil {
		if errors.Is(err, gwerr.ErrAgentConfigNotFound) {
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("DeleteAgentMCPConfig: failed to get configuration", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete configuration")
		return
	}
	if err := c.agentConfigService.DeleteMCP(ctx, configUUID, ouID, projectName, agentName); err != nil {
		if errors.Is(err, gwerr.ErrAgentConfigNotFound) {
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("DeleteAgentMCPConfig: failed to delete configuration", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete configuration")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Converter functions between spec and models

func convertCreateAgentModelConfigRequest(specReq gwspec.CreateAgentModelConfigRequest) (gwmodels.CreateAgentModelConfigRequest, error) {
	envMappings := make(map[string]gwmodels.EnvModelConfigRequest)
	for envName, envConfig := range specReq.EnvMappings {
		proxyName := resolveEnvConfigProxyName(envConfig)
		if proxyName == "" {
			return gwmodels.CreateAgentModelConfigRequest{}, fmt.Errorf("providerName or proxyName is required for environment %s", envName)
		}
		envMappings[envName] = gwmodels.EnvModelConfigRequest{
			ProviderName: proxyName,
			Configuration: gwmodels.EnvProviderConfiguration{
				Policies:   convertToModelPolicies(&envConfig.Configuration),
				Resilience: gwconv.ConvertSpecToModelResilience(envConfig.Configuration.Resilience),
			},
		}
	}

	var envVars []gwmodels.EnvironmentVariableConfig
	if specReq.EnvironmentVariables != nil {
		envVars = make([]gwmodels.EnvironmentVariableConfig, 0, len(specReq.EnvironmentVariables))
		for _, ev := range specReq.EnvironmentVariables {
			envVars = append(envVars, gwmodels.EnvironmentVariableConfig{Key: ev.Key, Name: ev.Name})
		}
	}

	return gwmodels.CreateAgentModelConfigRequest{
		Name:                 specReq.Name,
		Description:          getString(specReq.Description),
		Type:                 specReq.Type,
		EnvMappings:          envMappings,
		EnvironmentVariables: envVars,
	}, nil
}

func convertUpdateAgentModelConfigRequest(specReq gwspec.UpdateAgentModelConfigRequest) (gwmodels.UpdateAgentModelConfigRequest, error) {
	req := gwmodels.UpdateAgentModelConfigRequest{}

	if specReq.Name != nil {
		req.Name = *specReq.Name
	}
	if specReq.Description != nil {
		req.Description = *specReq.Description
	}
	if specReq.EnvMappings != nil {
		envMappings := make(map[string]gwmodels.EnvModelConfigRequest)
		for envName, envConfig := range *specReq.EnvMappings {
			proxyName := resolveEnvConfigProxyName(envConfig)
			if proxyName == "" {
				return gwmodels.UpdateAgentModelConfigRequest{}, fmt.Errorf("providerName or proxyName is required for environment %s", envName)
			}
			envMappings[envName] = gwmodels.EnvModelConfigRequest{
				ProviderName: proxyName,
				Configuration: gwmodels.EnvProviderConfiguration{
					Policies:   convertToModelPolicies(&envConfig.Configuration),
					Resilience: gwconv.ConvertSpecToModelResilience(envConfig.Configuration.Resilience),
				},
			}
		}
		req.EnvMappings = envMappings
	}
	if specReq.EnvironmentVariables != nil {
		req.EnvironmentVariables = make([]gwmodels.EnvironmentVariableConfig, 0, len(specReq.EnvironmentVariables))
		for _, ev := range specReq.EnvironmentVariables {
			req.EnvironmentVariables = append(req.EnvironmentVariables, gwmodels.EnvironmentVariableConfig{Key: ev.Key, Name: ev.Name})
		}
	}

	return req, nil
}

func convertAgentModelConfigResponse(modelResp gwmodels.AgentModelConfigResponse) gwspec.AgentModelConfigResponse {
	envModelConfig := make(map[string]gwspec.EnvProviderConfigMappings)
	for envName, envConfig := range modelResp.EnvModelConfig {
		specEnvConfig := gwspec.EnvProviderConfigMappings{
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
			modelEnvConfig := &gwspec.ProviderConfig{
				ProxyUuid:        proxyUUID,
				ProviderName:     providerName,
				Policies:         convertToSpecPolicies(&envConfig.LLMProxy.Policies),
				ProviderPolicies: convertToSpecPolicies(&envConfig.LLMProxy.ProviderPolicies),
				Resilience:       gwconv.ConvertModelToSpecResilience(envConfig.LLMProxy.Resilience),
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

			// The header name is not a secret, so report it whenever the proxy
			// names one — callers need it to build a request on every read, not
			// just the create that returns the key. Value stays creation-only.
			if envConfig.LLMProxy.AuthHeaderName != nil && *envConfig.LLMProxy.AuthHeaderName != "" {
				// A proxy's api-key security can be configured to read from the query
				// string instead of a header (see gwmodels.LLMProxyInfo.AuthIn) — assuming
				// "header" here would tell the caller to send a working credential the
				// wrong way.
				in := "header"
				if envConfig.LLMProxy.AuthIn != nil && *envConfig.LLMProxy.AuthIn != "" {
					in = *envConfig.LLMProxy.AuthIn
				}
				modelEnvConfig.AuthInfo = &gwspec.AuthInfo{
					Type:  "api-key",
					In:    in,
					Name:  *envConfig.LLMProxy.AuthHeaderName,
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

	envVars := make([]gwspec.EnvironmentVariableConfig, len(modelResp.EnvironmentVariables))
	for i, envVar := range modelResp.EnvironmentVariables {
		envVars[i] = gwspec.EnvironmentVariableConfig{
			Name: envVar.Name,
			Key:  envVar.Key,
		}
	}

	return gwspec.AgentModelConfigResponse{
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

func resolveEnvConfigProxyName(envConfig gwspec.EnvModelConfigRequest) string {
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

func convertAgentModelConfigListResponse(modelResp gwmodels.AgentModelConfigListResponse) gwspec.AgentModelConfigListResponse {
	configs := make([]gwspec.AgentModelConfigListItem, len(modelResp.Configs))
	for i, config := range modelResp.Configs {
		configs[i] = gwspec.AgentModelConfigListItem{
			Uuid:        config.UUID,
			Name:        config.Name,
			Description: getStringPtr(config.Description),
			AgentId:     config.AgentID,
			Type:        config.Type,
			ProjectName: config.ProjectName,
			CreatedAt:   config.CreatedAt,
		}
	}

	return gwspec.AgentModelConfigListResponse{
		Configs: configs,
		Pagination: gwspec.PaginationInfo{
			Count:  int32(modelResp.Pagination.Count),
			Offset: int32(modelResp.Pagination.Offset),
			Limit:  int32(modelResp.Pagination.Limit),
		},
	}
}

// Helper functions

func convertToSpecPolicies(modelPolicies *[]gwmodels.LLMPolicy) []gwspec.LLMPolicy {
	if modelPolicies == nil {
		return nil
	}
	policies := make([]gwspec.LLMPolicy, len(*modelPolicies))
	for i, policy := range *modelPolicies {
		policies[i] = gwspec.LLMPolicy{
			Name:    policy.Name,
			Version: policy.Version,
			Paths:   convertToSpecPolicyPaths(&policy.Paths),
		}
	}
	return policies
}

func convertToSpecPolicyPaths(modelPaths *[]gwmodels.LLMPolicyPath) []gwspec.LLMPolicyPath {
	if modelPaths == nil {
		return nil
	}
	paths := make([]gwspec.LLMPolicyPath, len(*modelPaths))
	for i, path := range *modelPaths {
		paths[i] = gwspec.LLMPolicyPath{
			Path:    path.Path,
			Methods: path.Methods,
			Params:  path.Params,
		}
	}
	return paths
}

func convertToModelPolicies(specConfig *gwspec.EnvProviderConfiguration) []gwmodels.LLMPolicy {
	if specConfig == nil {
		return nil
	}
	policies := make([]gwmodels.LLMPolicy, len(specConfig.Policies))
	for i, policy := range specConfig.Policies {
		policies[i] = gwmodels.LLMPolicy{
			Name:    policy.Name,
			Version: policy.Version,
			Paths:   convertToModelPolicyPaths(&policy.Paths),
		}
	}
	return policies
}

func convertToModelPolicyPaths(specPaths *[]gwspec.LLMPolicyPath) []gwmodels.LLMPolicyPath {
	if specPaths == nil {
		return nil
	}
	paths := make([]gwmodels.LLMPolicyPath, len(*specPaths))
	for i, path := range *specPaths {
		paths[i] = gwmodels.LLMPolicyPath{
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
	ouID = modkit.OrgID(r.Context())
	projName = r.PathValue(gwutil.PathParamProjName)
	agentName = r.PathValue(gwutil.PathParamAgentName)
	envName = r.PathValue("envName")
	configID := r.PathValue(gwutil.PathParamConfigId)
	parsed, err := uuid.Parse(configID)
	if err != nil {
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid configuration ID")
		return "", "", "", "", uuid.Nil, false
	}
	return ouID, projName, agentName, envName, parsed, true
}

// writeConfigAPIKeyError maps the shared per-config API key service errors to HTTP
// responses for the create/rotate/revoke handlers. operation labels the log line.
func (c *agentConfigurationController) writeConfigAPIKeyError(w http.ResponseWriter, log *slog.Logger, operation string, err error) {
	switch {
	case errors.Is(err, httpx.ErrBadRequest):
		httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, gwerr.ErrAgentConfigNotFound):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
	case errors.Is(err, gwerr.ErrAgentConfigNotExternal):
		httpx.WriteErrorResponse(w, http.StatusForbidden, "API key management is only available for external agents")
	case errors.Is(err, gwerr.ErrGatewayNotFound):
		httpx.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
	case errors.Is(err, gwerr.ErrAPIKeyNotFound):
		httpx.WriteErrorResponse(w, http.StatusNotFound, "API key not found")
	default:
		log.Error(operation+": failed", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to manage API key")
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
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "At least one of 'name' or 'displayName' must be provided")
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
		if errors.Is(err, gwerr.ErrAgentConfigNotFound) {
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("ListMCPConfigAPIKeys: failed to list API keys", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list API keys")
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, response)
}

// CreateMCPConfigAPIKey handles POST .../mcp-configs/{configId}/environments/{envName}/api-keys
func (c *agentConfigurationController) CreateMCPConfigAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID, projName, agentName, envName, configUUID, ok := c.configEnvAPIKeyParams(w, r)
	if !ok {
		return
	}

	var specReq gwspec.CreateLLMAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	name, displayName, ok := apiKeyCreateNameDisplayName(w, specReq.Name, specReq.DisplayName)
	if !ok {
		return
	}

	modkit.AuditResource(ctx, gwaudit.ResourceAPIKey, configUUID.String(), name)
	modkit.AuditDetail(ctx, "ownerType", gwaudit.APIKeyOwnerMCPConfig)
	modkit.AuditDetail(ctx, "ownerName", agentName)
	modkit.AuditDetail(ctx, "keyName", name)

	response, err := c.agentConfigService.CreateMCPConfigAPIKey(ctx, ouID, projName, agentName, configUUID, envName, &gwmodels.CreateAPIKeyRequest{
		Name:        name,
		DisplayName: displayName,
		ExpiresAt:   specReq.ExpiresAt,
	})
	if err != nil {
		c.writeConfigAPIKeyError(w, log, "CreateMCPConfigAPIKey", err)
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusCreated, response)
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

	var specReq gwspec.RotateLLMAPIKeyRequest
	// Body is optional for rotation: an empty body (io.EOF) is allowed, but any
	// other decode error indicates a malformed request and must be rejected.
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil && !errors.Is(err, io.EOF) {
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	modkit.AuditResource(ctx, gwaudit.ResourceAPIKey, configUUID.String(), keyName)
	modkit.AuditDetail(ctx, "ownerType", gwaudit.APIKeyOwnerMCPConfig)
	modkit.AuditDetail(ctx, "ownerName", agentName)
	modkit.AuditDetail(ctx, "keyName", keyName)

	response, err := c.agentConfigService.RotateMCPConfigAPIKey(ctx, ouID, projName, agentName, configUUID, envName, keyName, &gwmodels.RotateAPIKeyRequest{
		DisplayName: specReq.DisplayName,
		ExpiresAt:   specReq.ExpiresAt,
	})
	if err != nil {
		c.writeConfigAPIKeyError(w, log, "RotateMCPConfigAPIKey", err)
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, response)
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

	modkit.AuditResource(ctx, gwaudit.ResourceAPIKey, configUUID.String(), keyName)
	modkit.AuditDetail(ctx, "ownerType", gwaudit.APIKeyOwnerMCPConfig)
	modkit.AuditDetail(ctx, "ownerName", agentName)
	modkit.AuditDetail(ctx, "keyName", keyName)

	if err := c.agentConfigService.RevokeMCPConfigAPIKey(ctx, ouID, projName, agentName, configUUID, envName, keyName); err != nil {
		c.writeConfigAPIKeyError(w, log, "RevokeMCPConfigAPIKey", err)
		return
	}
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
		if errors.Is(err, gwerr.ErrAgentConfigNotFound) {
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Configuration not found")
			return
		}
		log.Error("ListLLMConfigAPIKeys: failed to list API keys", "error", err)
		if writeIfNotImplemented(w, err) {
			return
		}
		httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list API keys")
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, response)
}

// CreateLLMConfigAPIKey handles POST .../model-configs/{configId}/environments/{envName}/api-keys
func (c *agentConfigurationController) CreateLLMConfigAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID, projName, agentName, envName, configUUID, ok := c.configEnvAPIKeyParams(w, r)
	if !ok {
		return
	}

	var specReq gwspec.CreateLLMAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	name, displayName, ok := apiKeyCreateNameDisplayName(w, specReq.Name, specReq.DisplayName)
	if !ok {
		return
	}

	modkit.AuditResource(ctx, gwaudit.ResourceAPIKey, configUUID.String(), name)
	modkit.AuditDetail(ctx, "ownerType", gwaudit.APIKeyOwnerModelConfig)
	modkit.AuditDetail(ctx, "ownerName", agentName)
	modkit.AuditDetail(ctx, "keyName", name)

	response, err := c.agentConfigService.CreateLLMConfigAPIKey(ctx, ouID, projName, agentName, configUUID, envName, &gwmodels.CreateAPIKeyRequest{
		Name:        name,
		DisplayName: displayName,
		ExpiresAt:   specReq.ExpiresAt,
	})
	if err != nil {
		c.writeConfigAPIKeyError(w, log, "CreateLLMConfigAPIKey", err)
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusCreated, response)
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

	var specReq gwspec.RotateLLMAPIKeyRequest
	// Body is optional for rotation: an empty body (io.EOF) is allowed, but any
	// other decode error indicates a malformed request and must be rejected.
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil && !errors.Is(err, io.EOF) {
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	modkit.AuditResource(ctx, gwaudit.ResourceAPIKey, configUUID.String(), keyName)
	modkit.AuditDetail(ctx, "ownerType", gwaudit.APIKeyOwnerModelConfig)
	modkit.AuditDetail(ctx, "ownerName", agentName)
	modkit.AuditDetail(ctx, "keyName", keyName)

	response, err := c.agentConfigService.RotateLLMConfigAPIKey(ctx, ouID, projName, agentName, configUUID, envName, keyName, &gwmodels.RotateAPIKeyRequest{
		DisplayName: specReq.DisplayName,
		ExpiresAt:   specReq.ExpiresAt,
	})
	if err != nil {
		c.writeConfigAPIKeyError(w, log, "RotateLLMConfigAPIKey", err)
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, response)
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

	modkit.AuditResource(ctx, gwaudit.ResourceAPIKey, configUUID.String(), keyName)
	modkit.AuditDetail(ctx, "ownerType", gwaudit.APIKeyOwnerModelConfig)
	modkit.AuditDetail(ctx, "ownerName", agentName)
	modkit.AuditDetail(ctx, "keyName", keyName)

	if err := c.agentConfigService.RevokeLLMConfigAPIKey(ctx, ouID, projName, agentName, configUUID, envName, keyName); err != nil {
		c.writeConfigAPIKeyError(w, log, "RevokeLLMConfigAPIKey", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// callerSubject returns the acting user's token subject for created_by
// attribution, falling back to a marker when there is no user behind the call
// (a system client) rather than to a literal that would misattribute the change.
func callerSubject(ctx context.Context) string {
	if sub := modkit.PrincipalFrom(ctx).User; sub != "" {
		return sub
	}
	return "system"
}
