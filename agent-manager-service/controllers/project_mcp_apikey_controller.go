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

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// pathParamKeyName is the API key segment of the project MCP proxy binding key routes.
const pathParamKeyName = "keyName"

// projectMCPAPIKeyCreateRequest is the body of POST .../environments/{envName}/api-keys.
type projectMCPAPIKeyCreateRequest struct {
	Name        *string `json:"name,omitempty"`
	DisplayName *string `json:"displayName,omitempty"`
	ExpiresAt   *string `json:"expiresAt,omitempty"`
}

// writeProjectMCPAPIKeyError writes the response for an API key route's error. It returns false for an error it does
// not map, which the caller logs and answers with a 500.
func writeProjectMCPAPIKeyError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, utils.ErrMCPAPIKeyNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound, utils.ErrMCPAPIKeyNotFound.Error(), "", utils.ErrCodeNotFound)
	case errors.Is(err, utils.ErrMCPAPIKeyExists):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict, utils.ErrMCPAPIKeyExists.Error(), err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrAPIKeyChanged):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict, utils.ErrAPIKeyChanged.Error(), err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrMCPAPIKeySecurityDisabled):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			utils.ErrMCPAPIKeySecurityDisabled.Error(), "", utils.ErrCodeAPIKeySecurityDisabled)
	case errors.Is(err, utils.ErrBadRequest):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest, err.Error(), "", utils.ErrCodeValidation)
	default:
		return writeProjectMCPBindingError(w, err)
	}
	return true
}

// bindingKeyParams reads the org, handle and environment of a key route and resolves its project. On failure it has
// written the response and returns ok=false. A deleted project's proxies take no new keys, so it is a 404.
func (c *projectMCPProxyController) bindingKeyParams(
	w http.ResponseWriter, r *http.Request,
) (ouID string, project *models.ProjectResponse, handle, envName string, ok bool) {
	ouID = middleware.OUIDFromRequest(r)
	project, ok = c.resolveProject(w, r, ouID)
	return ouID, project, r.PathValue(utils.PathParamProxyId), r.PathValue(pathParamEnvName), ok
}

// ListProjectMCPProxyEnvironmentAPIKeys handles GET .../mcp-proxies/{proxyId}/environments/{envName}/api-keys.
func (c *projectMCPProxyController) ListProjectMCPProxyEnvironmentAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ouID, project, handle, envName, ok := c.bindingKeyParams(w, r)
	if !ok {
		return
	}
	resp, err := c.service.ListEnvironmentAPIKeys(ctx, ouID, project.UUID, handle, envName)
	if err != nil {
		if !writeProjectMCPAPIKeyError(w, err) {
			logger.GetLogger(ctx).Error("ListProjectMCPProxyEnvironmentAPIKeys: failed",
				"ouID", ouID, "handle", handle, "environment", envName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list the API keys")
		}
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// CreateProjectMCPProxyEnvironmentAPIKey handles POST .../mcp-proxies/{proxyId}/environments/{envName}/api-keys.
func (c *projectMCPProxyController) CreateProjectMCPProxyEnvironmentAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body projectMCPAPIKeyCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	name, displayName, ok := apiKeyCreateNameDisplayName(w, body.Name, body.DisplayName)
	if !ok {
		return
	}
	ouID, project, handle, envName, ok := c.bindingKeyParams(w, r)
	if !ok {
		return
	}
	resp, err := c.service.CreateEnvironmentAPIKey(ctx, ouID, project.Name, project.UUID, handle, envName, &models.CreateAPIKeyRequest{
		Name: name, DisplayName: displayName, ExpiresAt: body.ExpiresAt,
	})
	if err != nil {
		if !writeProjectMCPAPIKeyError(w, err) {
			logger.GetLogger(ctx).Error("CreateProjectMCPProxyEnvironmentAPIKey: failed",
				"ouID", ouID, "handle", handle, "environment", envName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to issue the API key")
		}
		return
	}
	utils.WriteSuccessResponse(w, http.StatusCreated, resp)
}

// RotateProjectMCPProxyEnvironmentAPIKey handles PUT .../environments/{envName}/api-keys/{keyName}.
func (c *projectMCPProxyController) RotateProjectMCPProxyEnvironmentAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var body models.RotateAPIKeyRequest
	// The body is optional: an empty one rotates the value only.
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	ouID, project, handle, envName, ok := c.bindingKeyParams(w, r)
	if !ok {
		return
	}
	keyName := r.PathValue(pathParamKeyName)
	resp, err := c.service.RotateEnvironmentAPIKey(ctx, ouID, project.Name, project.UUID, handle, envName, keyName, &body)
	if err != nil {
		if !writeProjectMCPAPIKeyError(w, err) {
			logger.GetLogger(ctx).Error("RotateProjectMCPProxyEnvironmentAPIKey: failed",
				"ouID", ouID, "handle", handle, "environment", envName, "key", keyName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to rotate the API key")
		}
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// RevokeProjectMCPProxyEnvironmentAPIKey handles DELETE .../environments/{envName}/api-keys/{keyName}.
func (c *projectMCPProxyController) RevokeProjectMCPProxyEnvironmentAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ouID, project, handle, envName, ok := c.bindingKeyParams(w, r)
	if !ok {
		return
	}
	keyName := r.PathValue(pathParamKeyName)
	if err := c.service.RevokeEnvironmentAPIKey(ctx, ouID, project.Name, project.UUID, handle, envName, keyName); err != nil {
		if !writeProjectMCPAPIKeyError(w, err) {
			logger.GetLogger(ctx).Error("RevokeProjectMCPProxyEnvironmentAPIKey: failed",
				"ouID", ouID, "handle", handle, "environment", envName, "key", keyName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to revoke the API key")
		}
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// IssueProjectMCPProxyEnvironmentTestAPIKey handles POST .../environments/{envName}/api-keys/test.
func (c *projectMCPProxyController) IssueProjectMCPProxyEnvironmentTestAPIKey(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
	var body models.IssueBindingTestAPIKeyRequest
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	ouID, project, handle, envName, ok := c.bindingKeyParams(w, r)
	if !ok {
		return
	}
	resp, err := c.service.IssueEnvironmentTestAPIKey(ctx, ouID, project.Name, project.UUID, handle, envName, &body)
	if err != nil {
		if !writeProjectMCPAPIKeyError(w, err) {
			logger.GetLogger(ctx).Error("IssueProjectMCPProxyEnvironmentTestAPIKey: failed",
				"ouID", ouID, "handle", handle, "environment", envName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to issue the test API key")
		}
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}
