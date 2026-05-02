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

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// AgentAPIKeyController handles API key operations for agent endpoints.
type AgentAPIKeyController interface {
	CreateAPIKey(w http.ResponseWriter, r *http.Request)
	CreateTestAPIKey(w http.ResponseWriter, r *http.Request)
	ListAPIKeys(w http.ResponseWriter, r *http.Request)
	RevokeAPIKey(w http.ResponseWriter, r *http.Request)
}

type agentAPIKeyController struct {
	apiKeyService *services.AgentAPIKeyService
}

// NewAgentAPIKeyController creates a new agent API key controller.
func NewAgentAPIKeyController(apiKeyService *services.AgentAPIKeyService) AgentAPIKeyController {
	return &agentAPIKeyController{
		apiKeyService: apiKeyService,
	}
}

func (c *agentAPIKeyController) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName, projectName, agentName := agentAPIKeyPathParams(r)

	var req models.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateAgentAPIKey: failed to decode request", "orgName", orgName, "projectName", projectName, "agentName", agentName, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Name == "" && req.DisplayName == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "At least one of 'name' or 'displayName' must be provided")
		return
	}

	response, err := c.apiKeyService.CreateAPIKey(ctx, orgName, projectName, agentName, &req)
	if err != nil {
		c.writeAgentAPIKeyError(ctx, w, "CreateAgentAPIKey", err)
		return
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *agentAPIKeyController) CreateTestAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName, projectName, agentName := agentAPIKeyPathParams(r)

	response, err := c.apiKeyService.CreateTestAPIKey(ctx, orgName, projectName, agentName)
	if err != nil {
		c.writeAgentAPIKeyError(ctx, w, "CreateAgentTestAPIKey", err)
		return
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *agentAPIKeyController) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName, projectName, agentName := agentAPIKeyPathParams(r)

	keys, err := c.apiKeyService.ListAPIKeys(ctx, orgName, projectName, agentName)
	if err != nil {
		c.writeAgentAPIKeyError(ctx, w, "ListAgentAPIKeys", err)
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, map[string]interface{}{
		"apiKeys": keys,
		"count":   len(keys),
	})
}

func (c *agentAPIKeyController) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName, projectName, agentName := agentAPIKeyPathParams(r)
	keyName := r.PathValue("keyName")

	if err := c.apiKeyService.RevokeAPIKey(ctx, orgName, projectName, agentName, keyName); err != nil {
		c.writeAgentAPIKeyError(ctx, w, "RevokeAgentAPIKey", err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *agentAPIKeyController) writeAgentAPIKeyError(ctx context.Context, w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, utils.ErrAgentNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
	case errors.Is(err, utils.ErrProjectNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
	case errors.Is(err, utils.ErrEnvironmentNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
	case errors.Is(err, utils.ErrGatewayNotFound):
		utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
	default:
		logger.GetLogger(ctx).Error(operation+": failed", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to manage agent API key")
	}
}

func agentAPIKeyPathParams(r *http.Request) (string, string, string) {
	return r.PathValue(utils.PathParamOrgName), r.PathValue(utils.PathParamProjName), r.PathValue(utils.PathParamAgentName)
}
