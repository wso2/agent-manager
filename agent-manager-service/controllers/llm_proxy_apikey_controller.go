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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// LLMProxyAPIKeyController handles API key operations for LLM proxies
type LLMProxyAPIKeyController interface {
	CreateAPIKey(w http.ResponseWriter, r *http.Request)
}

type llmProxyAPIKeyController struct {
	apiKeyService *services.LLMProxyAPIKeyService
	orgRepo       repositories.OrganizationRepository
}

// NewLLMProxyAPIKeyController creates a new LLM proxy API key controller
func NewLLMProxyAPIKeyController(
	apiKeyService *services.LLMProxyAPIKeyService,
	orgRepo repositories.OrganizationRepository,
) LLMProxyAPIKeyController {
	return &llmProxyAPIKeyController{
		apiKeyService: apiKeyService,
		orgRepo:       orgRepo,
	}
}

// resolveOrgUUID resolves organization handle to UUID
func (c *llmProxyAPIKeyController) resolveOrgUUID(ctx context.Context, orgName string) (string, error) {
	org, err := c.orgRepo.GetOrganizationByName(orgName)
	if err != nil {
		return "", err
	}
	if org == nil {
		return "", utils.ErrOrganizationNotFound
	}
	return org.UUID.String(), nil
}

// CreateAPIKey handles POST /api/v1/orgs/{orgName}/projects/{projName}/llm-proxies/{id}/api-keys
func (c *llmProxyAPIKeyController) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue("id")

	log.Info("CreateLLMProxyAPIKey: starting", "orgName", orgName, "proxyID", proxyID)

	// Resolve organization UUID
	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Error("CreateLLMProxyAPIKey: organization not found", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
			return
		}
		log.Error("CreateLLMProxyAPIKey: failed to resolve organization", "orgName", orgName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Parse request body
	var req models.CreateAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateLLMProxyAPIKey: failed to decode request", "orgName", orgName, "proxyID", proxyID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate that at least one of name or displayName is provided
	if req.Name == "" && req.DisplayName == "" {
		log.Error("CreateLLMProxyAPIKey: name or displayName required", "orgName", orgName, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "At least one of 'name' or 'displayName' must be provided")
		return
	}

	// Get user ID from context or header (optional for logging)
	userID := r.Header.Get("x-user-id")

	log.Info("CreateLLMProxyAPIKey: calling service", "orgName", orgName, "orgID", orgID, "proxyID", proxyID)

	// Call service to create API key
	response, err := c.apiKeyService.CreateAPIKey(ctx, orgID, proxyID, userID, &req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("CreateLLMProxyAPIKey: proxy not found", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Error("CreateLLMProxyAPIKey: no gateways found", "orgName", orgName, "orgID", orgID)
			utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
			return
		default:
			log.Error("CreateLLMProxyAPIKey: failed to create API key", "orgName", orgName, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create API key")
			return
		}
	}

	log.Info("CreateLLMProxyAPIKey: API key created successfully", "orgName", orgName, "proxyID", proxyID, "keyID", response.KeyID)

	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}
