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
	"io"
	"log/slog"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
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

// DeleteAgentModelConfig handles DELETE /agents/{agentName}/model-configs/{configId}
func (c *agentConfigurationController) DeleteAgentModelConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	projectName := gwutil.Project(r)
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

// DeleteAgentMCPConfig handles DELETE /agents/{agentName}/mcp-configs/{configId}
func (c *agentConfigurationController) DeleteAgentMCPConfig(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := modkit.OrgID(r.Context())
	projectName := gwutil.Project(r)
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
	projName = gwutil.Project(r)
	agentName = r.PathValue(gwutil.PathParamAgentName)
	envName = gwmodels.DefaultEnvironmentName
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
