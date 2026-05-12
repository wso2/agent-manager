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
	"fmt"
	"net/http"
	"strconv"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type AgentKindController interface {
	ListKinds(w http.ResponseWriter, r *http.Request)
	GetKind(w http.ResponseWriter, r *http.Request)
	UpdateKind(w http.ResponseWriter, r *http.Request)
	DeleteKind(w http.ResponseWriter, r *http.Request)
	AddVersion(w http.ResponseWriter, r *http.Request)
	ListVersions(w http.ResponseWriter, r *http.Request)
	GetVersion(w http.ResponseWriter, r *http.Request)
	DeleteVersion(w http.ResponseWriter, r *http.Request)
	ListKindAgents(w http.ResponseWriter, r *http.Request)
}

type agentKindController struct {
	kindService services.AgentKindService
}

func NewAgentKindController(kindService services.AgentKindService) AgentKindController {
	return &agentKindController{kindService: kindService}
}

func (c *agentKindController) ListKinds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = strconv.Itoa(utils.DefaultLimit)
	}
	offsetStr := r.URL.Query().Get("offset")
	if offsetStr == "" {
		offsetStr = strconv.Itoa(utils.DefaultOffset)
	}
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < utils.MinLimit || limit > utils.MaxLimit {
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid limit parameter: must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}
	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < utils.MinOffset {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid offset parameter: must be >= 0")
		return
	}

	result, err := c.kindService.ListKinds(ctx, orgName, limit, offset)
	if err != nil {
		log.Error("Failed to list agent kinds", "error", err)
		handleCommonErrors(w, err, "Failed to list agent kinds")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, result)
}

func (c *agentKindController) GetKind(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	kindName := r.PathValue(utils.PathParamKindName)

	result, err := c.kindService.GetKind(ctx, orgName, kindName)
	if err != nil {
		log.Error("Failed to get agent kind", "error", err)
		handleCommonErrors(w, err, "Failed to get agent kind")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, result)
}

func (c *agentKindController) UpdateKind(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	kindName := r.PathValue(utils.PathParamKindName)

	var payload spec.UpdateAgentKindRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	result, err := c.kindService.UpdateKind(ctx, orgName, kindName, &payload)
	if err != nil {
		log.Error("Failed to update agent kind", "error", err)
		handleCommonErrors(w, err, "Failed to update agent kind")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, result)
}

func (c *agentKindController) DeleteKind(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	kindName := r.PathValue(utils.PathParamKindName)

	if err := c.kindService.DeleteKind(ctx, orgName, kindName); err != nil {
		log.Error("Failed to delete agent kind", "error", err)
		handleCommonErrors(w, err, "Failed to delete agent kind")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *agentKindController) AddVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	kindName := r.PathValue(utils.PathParamKindName)

	var payload spec.AddAgentKindVersionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if payload.GetVersion() == "" || payload.GetBuildName() == "" || payload.GetSourceAgentName() == "" || payload.GetSourceProjectName() == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "version, buildName, sourceAgentName, and sourceProjectName are required")
		return
	}

	result, err := c.kindService.AddVersion(ctx, orgName, kindName, &payload)
	if err != nil {
		log.Error("Failed to add agent kind version", "error", err)
		handleCommonErrors(w, err, "Failed to add agent kind version")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusCreated, result)
}

func (c *agentKindController) ListVersions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	kindName := r.PathValue(utils.PathParamKindName)

	result, err := c.kindService.ListVersions(ctx, orgName, kindName)
	if err != nil {
		log.Error("Failed to list agent kind versions", "error", err)
		handleCommonErrors(w, err, "Failed to list agent kind versions")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, result)
}

func (c *agentKindController) GetVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	kindName := r.PathValue(utils.PathParamKindName)
	versionTag := r.PathValue(utils.PathParamVersionTag)

	result, err := c.kindService.GetVersion(ctx, orgName, kindName, versionTag)
	if err != nil {
		log.Error("Failed to get agent kind version", "error", err)
		handleCommonErrors(w, err, "Failed to get agent kind version")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, result)
}

func (c *agentKindController) DeleteVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	kindName := r.PathValue(utils.PathParamKindName)
	versionTag := r.PathValue(utils.PathParamVersionTag)

	if err := c.kindService.DeleteVersion(ctx, orgName, kindName, versionTag); err != nil {
		log.Error("Failed to delete agent kind version", "error", err)
		handleCommonErrors(w, err, "Failed to delete agent kind version")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *agentKindController) ListKindAgents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	kindName := r.PathValue(utils.PathParamKindName)

	result, err := c.kindService.ListKindAgents(ctx, orgName, kindName)
	if err != nil {
		log.Error("Failed to list agents for kind", "error", err)
		handleCommonErrors(w, err, "Failed to list agents for kind")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, result)
}
