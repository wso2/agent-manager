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

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// LLMProxyAPIKeyController handles API key operations for LLM proxies
type LLMProxyAPIKeyController interface {
	ListAPIKeys(w http.ResponseWriter, r *http.Request)
	CreateAPIKey(w http.ResponseWriter, r *http.Request)
	RevokeAPIKey(w http.ResponseWriter, r *http.Request)
	RotateAPIKey(w http.ResponseWriter, r *http.Request)
}

type llmProxyAPIKeyController struct {
	apiKeyService *services.LLMProxyAPIKeyService
}

// NewLLMProxyAPIKeyController creates a new LLM proxy API key controller
func NewLLMProxyAPIKeyController(
	apiKeyService *services.LLMProxyAPIKeyService,
) LLMProxyAPIKeyController {
	return &llmProxyAPIKeyController{
		apiKeyService: apiKeyService,
	}
}

// ListAPIKeys handles GET /api/v1/orgs/{ouID}/projects/{projName}/llm-proxies/{id}/api-keys
func (c *llmProxyAPIKeyController) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	proxyID := r.PathValue("id")

	log.Info("ListLLMProxyAPIKeys: starting", "ouID", ouID, "projName", projName, "proxyID", proxyID)

	response, err := c.apiKeyService.ListAPIKeys(ctx, ouID, projName, proxyID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrProjectNotFound):
			log.Warn("ListLLMProxyAPIKeys: project not found", "ouID", ouID, "projName", projName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("ListLLMProxyAPIKeys: proxy not found", "ouID", ouID, "projName", projName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		default:
			log.Error("ListLLMProxyAPIKeys: failed to list API keys", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list API keys")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// CreateAPIKey handles POST /api/v1/orgs/{ouID}/projects/{projName}/llm-proxies/{id}/api-keys
func (c *llmProxyAPIKeyController) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")

	log.Info("CreateLLMProxyAPIKey: starting", "ouID", ouID, "proxyID", proxyID)

	var specReq spec.CreateLLMAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Error("CreateLLMProxyAPIKey: failed to decode request", "ouID", ouID, "proxyID", proxyID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	name := ""
	if specReq.Name != nil {
		name = *specReq.Name
	}
	displayName := ""
	if specReq.DisplayName != nil {
		displayName = *specReq.DisplayName
	}

	if name == "" && displayName == "" {
		log.Error("CreateLLMProxyAPIKey: name or displayName required", "ouID", ouID, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "At least one of 'name' or 'displayName' must be provided")
		return
	}

	req := &models.CreateAPIKeyRequest{
		Name:        name,
		DisplayName: displayName,
		ExpiresAt:   specReq.ExpiresAt,
	}

	log.Info("CreateLLMProxyAPIKey: calling service", "ouID", ouID, "proxyID", proxyID)

	response, err := c.apiKeyService.CreateAPIKey(ctx, ouID, proxyID, req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrBadRequest):
			log.Warn("CreateLLMProxyAPIKey: invalid request", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("CreateLLMProxyAPIKey: proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Error("CreateLLMProxyAPIKey: no gateways found", "ouID", ouID)
			utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
			return
		default:
			log.Error("CreateLLMProxyAPIKey: failed to create API key", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create API key")
			return
		}
	}

	log.Info("CreateLLMProxyAPIKey: API key created successfully", "ouID", ouID, "proxyID", proxyID, "keyID", response.KeyID)

	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

// RevokeAPIKey handles DELETE /api/v1/orgs/{ouID}/projects/{projName}/llm-proxies/{id}/api-keys/{keyName}
func (c *llmProxyAPIKeyController) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")
	keyName := r.PathValue("keyName")

	log.Info("RevokeLLMProxyAPIKey: starting", "ouID", ouID, "proxyID", proxyID, "keyName", keyName)

	if err := c.apiKeyService.RevokeAPIKey(ctx, ouID, proxyID, keyName); err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("RevokeLLMProxyAPIKey: proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Error("RevokeLLMProxyAPIKey: no gateways found", "ouID", ouID)
			utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
			return
		default:
			log.Error("RevokeLLMProxyAPIKey: failed to revoke API key", "ouID", ouID, "proxyID", proxyID, "keyName", keyName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to revoke API key")
			return
		}
	}

	log.Info("RevokeLLMProxyAPIKey: API key revoked successfully", "ouID", ouID, "proxyID", proxyID, "keyName", keyName)

	w.WriteHeader(http.StatusNoContent)
}

// RotateAPIKey handles PUT /api/v1/orgs/{ouID}/projects/{projName}/llm-proxies/{id}/api-keys/{keyName}
func (c *llmProxyAPIKeyController) RotateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")
	keyName := r.PathValue("keyName")

	log.Info("RotateLLMProxyAPIKey: starting", "ouID", ouID, "proxyID", proxyID, "keyName", keyName)

	var specReq spec.RotateLLMAPIKeyRequest
	// Body is optional for rotation; ignore decode errors on empty body
	_ = json.NewDecoder(r.Body).Decode(&specReq)

	req := &models.RotateAPIKeyRequest{
		DisplayName: specReq.DisplayName,
		ExpiresAt:   specReq.ExpiresAt,
	}

	response, err := c.apiKeyService.RotateAPIKey(ctx, ouID, proxyID, keyName, req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("RotateLLMProxyAPIKey: proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Error("RotateLLMProxyAPIKey: no gateways found", "ouID", ouID)
			utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
			return
		default:
			log.Error("RotateLLMProxyAPIKey: failed to rotate API key", "ouID", ouID, "proxyID", proxyID, "keyName", keyName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to rotate API key")
			return
		}
	}

	log.Info("RotateLLMProxyAPIKey: API key rotated successfully", "ouID", ouID, "proxyID", proxyID, "keyName", keyName)

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}
