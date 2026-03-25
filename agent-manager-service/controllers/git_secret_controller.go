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
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// GitSecretController defines interface for git secret HTTP handlers
type GitSecretController interface {
	CreateGitSecret(w http.ResponseWriter, r *http.Request)
	ListGitSecrets(w http.ResponseWriter, r *http.Request)
	DeleteGitSecret(w http.ResponseWriter, r *http.Request)
}

type gitSecretController struct {
	gitSecretService *services.GitSecretService
}

// NewGitSecretController creates a new git secret controller
func NewGitSecretController(gitSecretService *services.GitSecretService) GitSecretController {
	return &gitSecretController{
		gitSecretService: gitSecretService,
	}
}

// CreateGitSecret handles POST /orgs/{orgName}/git-secrets
func (c *gitSecretController) CreateGitSecret(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	log.Info("CreateGitSecret: starting", "orgName", orgName)

	var req spec.CreateGitSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("CreateGitSecret: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate request body
	if err := utils.ValidateCreateGitSecretRequest(&req); err != nil {
		log.Error("CreateGitSecret: validation failed", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	created, err := c.gitSecretService.Create(ctx, orgName, &req)
	if err != nil {
		log.Error("CreateGitSecret: failed to create git secret", "error", err)
		handleCommonErrors(w, err, "Failed to create git secret")
		return
	}

	response := spec.GitSecretResponse{
		Name: created.Name,
	}

	log.Info("CreateGitSecret: completed", "orgName", orgName, "name", created.Name)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

// ListGitSecrets handles GET /orgs/{orgName}/git-secrets
func (c *gitSecretController) ListGitSecrets(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	log.Info("ListGitSecrets: starting", "orgName", orgName)

	// Parse pagination parameters
	limit := getIntQueryParam(r, "limit", 20)
	offset := getIntQueryParam(r, "offset", 0)

	// Validate and cap limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}

	secrets, totalCount, err := c.gitSecretService.List(ctx, orgName, limit, offset)
	if err != nil {
		log.Error("ListGitSecrets: failed to list git secrets", "orgName", orgName, "error", err)
		handleCommonErrors(w, err, "Failed to list git secrets")
		return
	}

	// Convert to response format
	secretResponses := make([]spec.GitSecretResponse, len(secrets))
	for i, s := range secrets {
		secretResponses[i] = spec.GitSecretResponse{
			Name: s.Name,
		}
	}

	response := spec.GitSecretListResponse{
		Secrets: secretResponses,
		Total:   int32(totalCount),
		Limit:   int32(limit),
		Offset:  int32(offset),
	}

	log.Info("ListGitSecrets: completed", "orgName", orgName, "count", len(secrets), "total", totalCount)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// DeleteGitSecret handles DELETE /orgs/{orgName}/git-secrets/{gitSecretName}
func (c *gitSecretController) DeleteGitSecret(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gitSecretName := r.PathValue(utils.PathParamSecretName)

	log.Info("DeleteGitSecret: starting", "orgName", orgName, "gitSecretName", gitSecretName)

	if err := c.gitSecretService.Delete(ctx, orgName, gitSecretName); err != nil {
		log.Error("DeleteGitSecret: failed to delete git secret", "error", err)
		handleCommonErrors(w, err, "Failed to delete git secret")
		return
	}

	log.Info("DeleteGitSecret: completed", "orgName", orgName, "gitSecretName", gitSecretName)
	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}
