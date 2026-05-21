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
	"io"
	"net/http"
	"regexp"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// apiKeyNameRegex mirrors the gateway-side validation so we fail fast at the
// API boundary instead of after the WebSocket round-trip. Lowercase alphanumeric
// segments separated by single hyphens or underscores; no leading/trailing or
// consecutive separators.
var apiKeyNameRegex = regexp.MustCompile(`^[a-z0-9]+(?:[-_][a-z0-9]+)*$`)

// AgentAPIKeyController handles API key operations for agents
type AgentAPIKeyController interface {
	CreateAPIKey(w http.ResponseWriter, r *http.Request)
	ListAPIKeys(w http.ResponseWriter, r *http.Request)
	RevokeAPIKey(w http.ResponseWriter, r *http.Request)
	RotateAPIKey(w http.ResponseWriter, r *http.Request)
	IssueTestAPIKey(w http.ResponseWriter, r *http.Request)
}

type agentAPIKeyController struct {
	apiKeyService *services.AgentAPIKeyService
}

// NewAgentAPIKeyController creates a new agent API key controller
func NewAgentAPIKeyController(
	apiKeyService *services.AgentAPIKeyService,
) AgentAPIKeyController {
	return &agentAPIKeyController{
		apiKeyService: apiKeyService,
	}
}

// CreateAPIKey handles POST /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys
func (c *agentAPIKeyController) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.PathValue(utils.PathParamEnvID)

	log.Info("CreateAgentAPIKey: starting", "orgName", orgName, "projName", projName, "agentName", agentName, "envID", envID)

	var specReq spec.CreateLLMAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		log.Error("CreateAgentAPIKey: failed to decode request", "error", err)
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

	// If a caller supplies an explicit name, validate it against the gateway
	// regex up front. When name is empty, the broadcaster falls back to
	// utils.GenerateHandle(displayName), which always produces a safe handle.
	if name != "" && !apiKeyNameRegex.MatchString(name) {
		log.Warn("CreateAgentAPIKey: invalid name", "name", name)
		utils.WriteErrorResponse(w, http.StatusBadRequest,
			"API key name must be lowercase alphanumeric segments separated by single hyphens or underscores")
		return
	}

	req := &models.CreateAPIKeyRequest{
		Name:        name,
		DisplayName: displayName,
		ExpiresAt:   specReq.ExpiresAt,
	}

	response, err := c.apiKeyService.CreateAPIKey(ctx, orgName, projName, agentName, envID, req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrBadRequest):
			log.Warn("CreateAgentAPIKey: invalid request", "orgName", orgName, "agentName", agentName, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		case errors.Is(err, utils.ErrArtifactNotFound):
			log.Warn("CreateAgentAPIKey: agent not found", "orgName", orgName, "agentName", agentName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		case errors.Is(err, utils.ErrEnvironmentNotFound):
			log.Warn("CreateAgentAPIKey: environment not found", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Error("CreateAgentAPIKey: no gateways found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
			return
		default:
			log.Error("CreateAgentAPIKey: failed to create API key", "orgName", orgName, "agentName", agentName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create API key")
			return
		}
	}

	log.Info("CreateAgentAPIKey: API key created successfully", "orgName", orgName, "agentName", agentName, "keyID", response.KeyID)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

// ListAPIKeys handles GET /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys
func (c *agentAPIKeyController) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.PathValue(utils.PathParamEnvID)

	log.Info("ListAgentAPIKeys: starting", "orgName", orgName, "projName", projName, "agentName", agentName, "envID", envID)

	keys, err := c.apiKeyService.ListAPIKeys(ctx, orgName, projName, agentName, envID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrArtifactNotFound):
			log.Warn("ListAgentAPIKeys: agent not found", "orgName", orgName, "agentName", agentName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		case errors.Is(err, utils.ErrEnvironmentNotFound):
			log.Warn("ListAgentAPIKeys: environment not found", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
			return
		default:
			log.Error("ListAgentAPIKeys: failed to list API keys", "orgName", orgName, "agentName", agentName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list API keys")
			return
		}
	}

	if keys == nil {
		keys = []models.StoredAPIKey{}
	}
	utils.WriteSuccessResponse(w, http.StatusOK, keys)
}

// RevokeAPIKey handles DELETE /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/{keyName}
func (c *agentAPIKeyController) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.PathValue(utils.PathParamEnvID)
	keyName := r.PathValue("keyName")

	log.Info("RevokeAgentAPIKey: starting", "orgName", orgName, "agentName", agentName, "envID", envID, "keyName", keyName)

	if err := c.apiKeyService.RevokeAPIKey(ctx, orgName, projName, agentName, envID, keyName); err != nil {
		switch {
		case errors.Is(err, utils.ErrArtifactNotFound):
			log.Warn("RevokeAgentAPIKey: agent not found", "orgName", orgName, "agentName", agentName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		case errors.Is(err, utils.ErrEnvironmentNotFound):
			log.Warn("RevokeAgentAPIKey: environment not found", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Error("RevokeAgentAPIKey: no gateways found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
			return
		default:
			log.Error("RevokeAgentAPIKey: failed to revoke API key", "orgName", orgName, "agentName", agentName, "keyName", keyName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to revoke API key")
			return
		}
	}

	log.Info("RevokeAgentAPIKey: API key revoked successfully", "orgName", orgName, "agentName", agentName, "keyName", keyName)
	w.WriteHeader(http.StatusNoContent)
}

// RotateAPIKey handles PUT /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/{keyName}
func (c *agentAPIKeyController) RotateAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.PathValue(utils.PathParamEnvID)
	keyName := r.PathValue("keyName")

	log.Info("RotateAgentAPIKey: starting", "orgName", orgName, "agentName", agentName, "envID", envID, "keyName", keyName)

	var specReq spec.RotateLLMAPIKeyRequest
	// Body is optional for rotation; only an empty body is acceptable.
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil && !errors.Is(err, io.EOF) {
		log.Warn("RotateAgentAPIKey: invalid request body", "orgName", orgName, "agentName", agentName, "keyName", keyName, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req := &models.RotateAPIKeyRequest{
		DisplayName: specReq.DisplayName,
		ExpiresAt:   specReq.ExpiresAt,
	}

	response, err := c.apiKeyService.RotateAPIKey(ctx, orgName, projName, agentName, envID, keyName, req)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrArtifactNotFound):
			log.Warn("RotateAgentAPIKey: agent not found", "orgName", orgName, "agentName", agentName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		case errors.Is(err, utils.ErrEnvironmentNotFound):
			log.Warn("RotateAgentAPIKey: environment not found", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Error("RotateAgentAPIKey: no gateways found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
			return
		default:
			log.Error("RotateAgentAPIKey: failed to rotate API key", "orgName", orgName, "agentName", agentName, "keyName", keyName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to rotate API key")
			return
		}
	}

	log.Info("RotateAgentAPIKey: API key rotated successfully", "orgName", orgName, "agentName", agentName, "keyName", keyName)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// IssueTestAPIKey handles POST /api/v1/orgs/{orgName}/projects/{projName}/agents/{agentName}/environments/{envID}/api-keys/test
//
// Issues (or rotates) the single short-lived test API key for the agent.
// Used by the console Try-It flow. The key is test-scoped, scoped to the
// fixed name "console-test", and never appears in the user-facing list.
func (c *agentAPIKeyController) IssueTestAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.PathValue(utils.PathParamEnvID)

	log.Info("IssueTestAPIKey: starting", "orgName", orgName, "projName", projName, "agentName", agentName, "envID", envID)

	response, err := c.apiKeyService.IssueTestAPIKey(ctx, orgName, projName, agentName, envID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrArtifactNotFound):
			log.Warn("IssueTestAPIKey: agent not found", "orgName", orgName, "agentName", agentName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
			return
		case errors.Is(err, utils.ErrEnvironmentNotFound):
			log.Warn("IssueTestAPIKey: environment not found", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Error("IssueTestAPIKey: no gateways found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "No gateway connections available")
			return
		default:
			log.Error("IssueTestAPIKey: failed to issue test API key", "orgName", orgName, "agentName", agentName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to issue test API key")
			return
		}
	}

	log.Info("IssueTestAPIKey: test API key issued successfully", "orgName", orgName, "agentName", agentName, "expiresAt", response.ExpiresAt)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}
