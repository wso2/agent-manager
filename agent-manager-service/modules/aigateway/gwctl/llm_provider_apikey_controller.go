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
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
)

// LLMProviderAPIKeyController handles API key operations for LLM providers
type LLMProviderAPIKeyController interface {
	ListAPIKeys(w http.ResponseWriter, r *http.Request)
	CreateAPIKey(w http.ResponseWriter, r *http.Request)
	RevokeAPIKey(w http.ResponseWriter, r *http.Request)
	RotateAPIKey(w http.ResponseWriter, r *http.Request)
}

type llmProviderAPIKeyController struct {
	apiKeyService *gwsvc.LLMProviderAPIKeyService
}

// NewLLMProviderAPIKeyController creates a new LLM provider API key controller
func NewLLMProviderAPIKeyController(
	apiKeyService *gwsvc.LLMProviderAPIKeyService,
) LLMProviderAPIKeyController {
	return &llmProviderAPIKeyController{
		apiKeyService: apiKeyService,
	}
}

// ListAPIKeys handles GET /m/ai-gateway/llm-providers/{providerId}/api-keys
func (c *llmProviderAPIKeyController) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	providerID := r.PathValue(gwutil.PathParamProviderId)

	log.Info("ListLLMProviderAPIKeys: starting", "ouID", ouID, "providerID", providerID)

	response, err := c.apiKeyService.ListAPIKeys(ctx, ouID, providerID)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			log.Warn("ListLLMProviderAPIKeys: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		default:
			log.Error("ListLLMProviderAPIKeys: failed to list API keys", "ouID", ouID, "providerID", providerID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list API keys")
			return
		}
	}

	httpx.WriteSuccessResponse(w, http.StatusOK, response)
}

// CreateAPIKey handles POST /m/ai-gateway/llm-providers/{providerId}/api-keys
func (c *llmProviderAPIKeyController) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	providerID := r.PathValue(gwutil.PathParamProviderId)

	log.Info("CreateLLMProviderAPIKey: starting", "ouID", ouID, "providerID", providerID)

	var specReq gwspec.CreateLLMAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Error("CreateLLMProviderAPIKey: failed to decode request", "ouID", ouID, "providerID", providerID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
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
		log.Error("CreateLLMProviderAPIKey: name or displayName required", "ouID", ouID, "providerID", providerID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "At least one of 'name' or 'displayName' must be provided")
		return
	}

	req := &gwmodels.CreateAPIKeyRequest{
		Name:        name,
		DisplayName: displayName,
		ExpiresAt:   specReq.ExpiresAt,
	}

	log.Info("CreateLLMProviderAPIKey: calling service", "ouID", ouID, "providerID", providerID)

	response, err := c.apiKeyService.CreateAPIKey(ctx, ouID, providerID, req)
	if err != nil {
		switch {
		case errors.Is(err, httpx.ErrBadRequest):
			log.Warn("CreateLLMProviderAPIKey: invalid request", "ouID", ouID, "providerID", providerID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			log.Warn("CreateLLMProviderAPIKey: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, gwerr.ErrGatewayNotFound):
			log.Error("CreateLLMProviderAPIKey: no gateways found", "ouID", ouID)
			httpx.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
			return
		default:
			log.Error("CreateLLMProviderAPIKey: failed to create API key", "ouID", ouID, "providerID", providerID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create API key")
			return
		}
	}

	log.Info("CreateLLMProviderAPIKey: API key created successfully", "ouID", ouID, "providerID", providerID, "keyID", response.KeyID)

	httpx.WriteSuccessResponse(w, http.StatusCreated, response)
}

// RevokeAPIKey handles DELETE /m/ai-gateway/llm-providers/{providerId}/api-keys/{keyName}
func (c *llmProviderAPIKeyController) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	providerID := r.PathValue(gwutil.PathParamProviderId)
	keyName := r.PathValue("keyName")

	log.Info("RevokeLLMProviderAPIKey: starting", "ouID", ouID, "providerID", providerID, "keyName", keyName)

	if err := c.apiKeyService.RevokeAPIKey(ctx, ouID, providerID, keyName); err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			log.Warn("RevokeLLMProviderAPIKey: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, gwerr.ErrGatewayNotFound):
			log.Error("RevokeLLMProviderAPIKey: no gateways found", "ouID", ouID)
			httpx.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
			return
		default:
			log.Error("RevokeLLMProviderAPIKey: failed to revoke API key", "ouID", ouID, "providerID", providerID, "keyName", keyName, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to revoke API key")
			return
		}
	}

	log.Info("RevokeLLMProviderAPIKey: API key revoked successfully", "ouID", ouID, "providerID", providerID, "keyName", keyName)

	w.WriteHeader(http.StatusNoContent)
}

// RotateAPIKey handles PUT /m/ai-gateway/llm-providers/{providerId}/api-keys/{keyName}
func (c *llmProviderAPIKeyController) RotateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	providerID := r.PathValue(gwutil.PathParamProviderId)
	keyName := r.PathValue("keyName")

	log.Info("RotateLLMProviderAPIKey: starting", "ouID", ouID, "providerID", providerID, "keyName", keyName)

	var specReq gwspec.RotateLLMAPIKeyRequest
	// Body is optional for rotation; ignore decode errors on empty body
	_ = json.NewDecoder(r.Body).Decode(&specReq)

	req := &gwmodels.RotateAPIKeyRequest{
		DisplayName: specReq.DisplayName,
		ExpiresAt:   specReq.ExpiresAt,
	}

	response, err := c.apiKeyService.RotateAPIKey(ctx, ouID, providerID, keyName, req)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			log.Warn("RotateLLMProviderAPIKey: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, gwerr.ErrAPIKeyNotFound):
			httpx.WriteErrorResponse(w, http.StatusNotFound, "API key not found")
			return
		case errors.Is(err, gwerr.ErrGatewayNotFound):
			log.Error("RotateLLMProviderAPIKey: no gateways found", "ouID", ouID)
			httpx.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
			return
		default:
			log.Error("RotateLLMProviderAPIKey: failed to rotate API key", "ouID", ouID, "providerID", providerID, "keyName", keyName, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to rotate API key")
			return
		}
	}

	log.Info("RotateLLMProviderAPIKey: API key rotated successfully", "ouID", ouID, "providerID", providerID, "keyName", keyName)

	httpx.WriteSuccessResponse(w, http.StatusOK, response)
}
