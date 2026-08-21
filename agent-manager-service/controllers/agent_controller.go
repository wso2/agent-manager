// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type AgentController interface {
	ListAgents(w http.ResponseWriter, r *http.Request)
	ListOrgAgents(w http.ResponseWriter, r *http.Request)
	GetAgent(w http.ResponseWriter, r *http.Request)
	CreateAgent(w http.ResponseWriter, r *http.Request)
	UpdateAgentBasicInfo(w http.ResponseWriter, r *http.Request)
	UpdateAgentBuildParameters(w http.ResponseWriter, r *http.Request)
	DeleteAgent(w http.ResponseWriter, r *http.Request)
	BuildAgent(w http.ResponseWriter, r *http.Request)
	DeployAgent(w http.ResponseWriter, r *http.Request)
	ListAgentBuilds(w http.ResponseWriter, r *http.Request)
	GetAgentDeployments(w http.ResponseWriter, r *http.Request)
	UpdateDeploymentState(w http.ResponseWriter, r *http.Request)
	GetAgentEndpoints(w http.ResponseWriter, r *http.Request)
	GetBuild(w http.ResponseWriter, r *http.Request)
	GetAgentConfigurations(w http.ResponseWriter, r *http.Request)
	GenerateName(w http.ResponseWriter, r *http.Request)
	GetAgentResourceConfigs(w http.ResponseWriter, r *http.Request)
	UpdateAgentResourceConfigs(w http.ResponseWriter, r *http.Request)
	PublishKind(w http.ResponseWriter, r *http.Request)
	PromoteAgent(w http.ResponseWriter, r *http.Request)
	UpdateAgentDeploySettings(w http.ResponseWriter, r *http.Request)
	UpdateAgentConfigurations(w http.ResponseWriter, r *http.Request)
	RegenerateTracingToken(w http.ResponseWriter, r *http.Request)
	GetAgentIdentity(w http.ResponseWriter, r *http.Request)
	RegenerateAgentIdentitySecret(w http.ResponseWriter, r *http.Request)
	RevokeAgentIdentitySecret(w http.ResponseWriter, r *http.Request)
	ProvisionAgentIdentity(w http.ResponseWriter, r *http.Request)
	RetryAgentIdentityProvisioning(w http.ResponseWriter, r *http.Request)
	GetAgentRoles(w http.ResponseWriter, r *http.Request)
	GetAgentGroups(w http.ResponseWriter, r *http.Request)
}

type agentController struct {
	agentService     services.AgentManagerService
	agentKindService services.AgentKindService
}

// NewAgentController returns a new AgentController instance.
func NewAgentController(agentService services.AgentManagerService, agentKindService services.AgentKindService) AgentController {
	return &agentController{
		agentService:     agentService,
		agentKindService: agentKindService, // kept for PublishKind
	}
}

// handleCommonErrors checks for common resource errors and writes appropriate responses.
// If no common error matches, writes an internal server error with the provided fallback message.
func handleCommonErrors(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	// A ValidationError raised below the controller (e.g. a stored agent kind
	// version that can't be written as a label) is still a client-side problem —
	// report it as a 400 naming the offending value instead of letting it reach
	// the generic 500 fallback.
	//
	// Keep this FIRST: a ValidationError may also match a classification
	// sentinel (utils.NewInvalidInputError sets ErrInvalidInput), and the
	// sentinel cases below relabel the response with a generic bucket message.
	// Moving this case down silently replaces every such error's own
	// user-facing Message with "Invalid input provided".
	case utils.IsValidationError(err) != nil:
		utils.WriteValidationErrorResponse(w, err)

	// Not found errors
	case errors.Is(err, utils.ErrOrganizationNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Organization not found", err.Error(), utils.ErrCodeOrganizationNotFound)
	case errors.Is(err, utils.ErrProjectNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Project not found", err.Error(), utils.ErrCodeProjectNotFound)
	case errors.Is(err, utils.ErrAgentNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Agent not found", err.Error(), utils.ErrCodeAgentNotFound)
	case errors.Is(err, utils.ErrLLMProviderNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"LLM provider not found", err.Error(), utils.ErrCodeProviderNotFound)
	case errors.Is(err, utils.ErrMCPProxyNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"MCP proxy not found", err.Error(), utils.ErrCodeNotFound)
	case errors.Is(err, utils.ErrBuildNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Build not found", err.Error(), utils.ErrCodeBuildNotFound)
	case errors.Is(err, utils.ErrEnvironmentNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Environment not found", err.Error(), utils.ErrCodeEnvironmentNotFound)
	case errors.Is(err, utils.ErrGitSecretNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Git secret not found", err.Error(), utils.ErrCodeGitSecretNotFound)
	case errors.Is(err, utils.ErrAgentKindNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Agent kind not found", err.Error(), utils.ErrCodeNotFound)
	case errors.Is(err, utils.ErrKindVersionNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Agent kind version not found", err.Error(), utils.ErrCodeNotFound)
	case errors.Is(err, utils.ErrSourceAgentNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Source agent not found", err.Error(), utils.ErrCodeNotFound)

	// Conflict errors
	case errors.Is(err, utils.ErrAgentAlreadyExists):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Agent already exists", err.Error(), utils.ErrCodeAgentAlreadyExists)
	case errors.Is(err, utils.ErrOrphanedAgentConfigsExist):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Configurations from a deleted agent with this name still exist", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrProjectAlreadyExists):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Project already exists", err.Error(), utils.ErrCodeProjectAlreadyExists)
	case errors.Is(err, utils.ErrProjectHasAssociatedAgents):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Project has associated agents", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrDeploymentPipelineInUse):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Deployment pipeline is referenced by one or more projects", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrSecretPathConflict):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Secret path conflict", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrGitSecretAlreadyExists):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Git secret already exists", err.Error(), utils.ErrCodeGitSecretAlreadyExists)
	case errors.Is(err, utils.ErrAgentKindAlreadyExists):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Agent kind already exists", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrKindVersionAlreadyExists):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Agent kind version already exists", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrKindImageAlreadyPublished):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Build image already published", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrAgentKindHasInstances):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Agent kind has active instances", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrAgentIsKindSource):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Agent is the source of an agent kind", err.Error(), utils.ErrCodeConflict)
	// Generic conflict catch-all: any unclassified conflict (e.g. a raw "already exists"
	// from OpenChoreo) is a 409, never a 500. Keep this after the specific conflict cases
	// above so they win; err.Error() carries the detail in the response reason.
	case errors.Is(err, utils.ErrConflict):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Conflict", err.Error(), utils.ErrCodeConflict)

	// Bad request errors
	case errors.Is(err, utils.ErrInvalidInput):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Invalid input provided", err.Error(), utils.ErrCodeValidation)
	case errors.Is(err, utils.ErrImmutableFieldChange):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Cannot modify immutable field", err.Error(), utils.ErrCodeImmutableField)
	case errors.Is(err, utils.ErrBadRequest):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Bad request", err.Error(), utils.ErrCodeBadRequest)
	case errors.Is(err, utils.ErrDeploymentPipelineNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Deployment pipeline not found", err.Error(), utils.ErrCodeBadRequest)
	case errors.Is(err, utils.ErrGitSecretInvalidType):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Invalid git secret type", err.Error(), utils.ErrCodeGitSecretInvalidType)
	case errors.Is(err, utils.ErrBuildNotComplete):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Build not complete", err.Error(), utils.ErrCodeBadRequest)
	case errors.Is(err, utils.ErrMissingKindConfigValue):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Missing required configuration value", err.Error(), utils.ErrCodeValidation)
	case errors.Is(err, utils.ErrDeploymentInProgress):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"A deployment is already in progress", err.Error(), utils.ErrCodeConflict)

	// Authorization errors
	case errors.Is(err, utils.ErrUnauthorized):
		utils.WriteErrorResponseWithReason(w, http.StatusUnauthorized,
			"Unauthorized", err.Error(), utils.ErrCodeUnauthorized)
	case errors.Is(err, utils.ErrForbidden):
		utils.WriteErrorResponseWithReason(w, http.StatusForbidden,
			"Forbidden", err.Error(), utils.ErrCodeForbidden)

	// Service unavailable
	case errors.Is(err, utils.ErrServiceUnavailable):
		utils.WriteErrorResponseWithReason(w, http.StatusServiceUnavailable,
			"Service temporarily unavailable", err.Error(), utils.ErrCodeServiceUnavailable)

	default:
		utils.WriteErrorResponseWithReason(w, http.StatusInternalServerError,
			fallbackMsg, "Internal server error", utils.ErrCodeInternalError)
	}
}

func (c *agentController) GetAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	agent, err := c.agentService.GetAgent(ctx, ouID, projName, agentName)
	if err != nil {
		log.Error("GetAgent: failed to get agent", "error", err)
		handleCommonErrors(w, err, "Failed to get agent")
		return
	}

	agentResponse := utils.ConvertToAgentResponse(agent)
	utils.WriteSuccessResponse(w, http.StatusOK, agentResponse)
}

func (c *agentController) ListAgents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = strconv.Itoa(utils.DefaultLimit)
	}
	offsetStr := r.URL.Query().Get("offset")
	if offsetStr == "" {
		offsetStr = strconv.Itoa(utils.DefaultOffset)
	}

	// Parse and validate pagination parameters
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < utils.MinLimit || limit > utils.MaxLimit {
		log.Warn("ListAgents: invalid limit parameter", "limit", limitStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid limit parameter: must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < utils.MinOffset {
		log.Warn("ListAgents: invalid offset parameter", "offset", offsetStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid offset parameter: must be %d or greater", utils.MinOffset))
		return
	}

	labelFilter, err := utils.ParseLabelSelectors(r.URL.Query()["label"])
	if err != nil {
		log.Debug("ListAgents: invalid label parameter", "error", err)
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	agents, total, err := c.agentService.ListAgents(ctx, ouID, projName, labelFilter, int32(limit), int32(offset))
	if err != nil {
		log.Error("ListAgents: failed to list agents", "error", err)
		handleCommonErrors(w, err, "Failed to list agents")
		return
	}

	agentResponses := utils.ConvertToAgentListResponse(agents)
	response := &spec.AgentListResponse{
		Agents: agentResponses,
		Total:  total,
		Limit:  int32(limit),
		Offset: int32(offset),
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *agentController) ListOrgAgents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)

	agents, err := c.agentService.ListOrgAgents(ctx, ouID)
	if err != nil {
		log.Error("ListOrgAgents: failed to list agents", "error", err)
		handleCommonErrors(w, err, "Failed to list agents")
		return
	}

	response := &spec.AgentSummaryListResponse{
		Agents: utils.ConvertToAgentSummaryListResponse(agents),
	}
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *agentController) CreateAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)

	// Parse and validate request body
	var payload spec.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("CreateAgent: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.ValidateAgentCreatePayload(payload); err != nil {
		log.Warn("CreateAgent: invalid agent payload", "error", err)
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	err := c.agentService.CreateAgent(ctx, ouID, projName, &payload)
	if err != nil {
		log.Error("CreateAgent: failed to create agent", "error", err)
		handleCommonErrors(w, err, "Failed to create agent")
		return
	}
	agentType := spec.AgentType{}
	if payload.AgentType != nil {
		agentType = *payload.AgentType
	}
	response := &spec.AgentResponse{
		Name:           payload.Name,
		DisplayName:    payload.DisplayName,
		Description:    utils.StrPointerAsStr(payload.Description, ""),
		ProjectName:    projName,
		Provisioning:   payload.Provisioning,
		AgentType:      agentType,
		Configurations: utils.RedactSecretConfigValues(payload.Configurations),
		Build:          payload.Build,
		CreatedAt:      time.Now(),
	}

	utils.WriteSuccessResponse(w, http.StatusAccepted, response)
}

func (c *agentController) UpdateAgentBasicInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse and validate request body
	var payload spec.UpdateAgentBasicInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("UpdateAgent: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := utils.ValidateAgentBasicInfoUpdatePayload(payload); err != nil {
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	agent, err := c.agentService.UpdateAgentBasicInfo(ctx, ouID, projName, agentName, &payload)
	if err != nil {
		log.Error("UpdateAgent: failed to update agent", "error", err)
		handleCommonErrors(w, err, "Failed to update agent")
		return
	}

	agentResponse := utils.ConvertToAgentResponse(agent)
	utils.WriteSuccessResponse(w, http.StatusOK, agentResponse)
}

func (c *agentController) UpdateAgentBuildParameters(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse and validate request body
	var payload spec.UpdateAgentBuildParametersRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("UpdateAgentBuildParameters: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := utils.ValidateAgentBuildParametersUpdatePayload(payload); err != nil {
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	agent, err := c.agentService.UpdateAgentBuildParameters(ctx, ouID, projName, agentName, &payload)
	if err != nil {
		log.Error("UpdateAgentBuildParameters: failed to update agent build parameters", "error", err)
		handleCommonErrors(w, err, "Failed to update agent build parameters")
		return
	}

	agentResponse := utils.ConvertToAgentResponse(agent)
	utils.WriteSuccessResponse(w, http.StatusOK, agentResponse)
}

func (c *agentController) GetAgentResourceConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	environment := r.URL.Query().Get("environment")

	if environment == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment query parameter is required")
		return
	}

	configs, err := c.agentService.GetAgentResourceConfigs(ctx, ouID, projName, agentName, environment)
	if err != nil {
		log.Error("GetAgentResourceConfigs: failed to get agent resource configurations", "error", err)
		handleCommonErrors(w, err, "Failed to get agent resource configurations")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, configs)
}

func (c *agentController) UpdateAgentResourceConfigs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	environment := r.URL.Query().Get("environment")

	if environment == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment query parameter is required")
		return
	}

	// Parse and validate request body
	var payload spec.UpdateAgentResourceConfigsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("UpdateAgentResourceConfigs: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := utils.ValidateAgentResourceConfigsPayload(payload, config.GetConfig().PerAgentResourceLimits); err != nil {
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	resourceConfigs, err := c.agentService.UpdateAgentResourceConfigs(ctx, ouID, projName, agentName, environment, &payload)
	if err != nil {
		log.Error("UpdateAgentResourceConfigs: failed to update agent resource configurations", "error", err)
		handleCommonErrors(w, err, "Failed to update agent resource configurations")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, resourceConfigs)
}

func (c *agentController) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	err := c.agentService.DeleteAgent(ctx, ouID, projName, agentName)
	if err != nil {
		log.Error("DeleteAgent: failed to delete agent", "error", err)
		handleCommonErrors(w, err, "Failed to delete agent")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusNoContent, "")
}

func (c *agentController) BuildAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse query parameters
	commitId := r.URL.Query().Get("commitId")
	if commitId == "" {
		log.Debug("BuildAgent: commitId not provided, using latest commit")
	}
	build, err := c.agentService.BuildAgent(ctx, ouID, projName, agentName, commitId)
	if err != nil {
		log.Error("BuildAgent: failed to build agent", "error", err)
		handleCommonErrors(w, err, "Failed to build agent")
		return
	}

	buildResponse := utils.ConvertToBuildResponse(build)
	utils.WriteSuccessResponse(w, http.StatusAccepted, buildResponse)
}

func (c *agentController) DeployAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse and validate request body
	var payload spec.DeployAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("DeployAgent: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.ValidateDeployAgentRequest(&payload); err != nil {
		log.Warn("DeployAgent: invalid request", "error", err)
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	deployedEnv, err := c.agentService.DeployAgent(ctx, ouID, projName, agentName, &payload)
	if err != nil {
		log.Error("DeployAgent: failed to deploy agent", "error", err)
		handleCommonErrors(w, err, "Failed to deploy agent")
		return
	}

	response := &spec.DeploymentResponse{
		AgentName:   agentName,
		ProjectName: projName,
		ImageId:     payload.ImageId,
		Environment: deployedEnv,
	}
	utils.WriteSuccessResponse(w, http.StatusAccepted, response)
}

func (c *agentController) PromoteAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	var payload spec.PromoteAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("PromoteAgent: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := utils.ValidatePromoteAgentRequest(&payload); err != nil {
		log.Warn("PromoteAgent: invalid request", "error", err)
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	if err := c.agentService.PromoteAgent(ctx, ouID, projName, agentName, &payload); err != nil {
		log.Error("PromoteAgent: failed to promote agent", "error", err)
		handleCommonErrors(w, err, "Failed to promote agent")
		return
	}

	response := &spec.PromoteAgentResponse{
		AgentName:         &agentName,
		ProjectName:       &projName,
		SourceEnvironment: &payload.SourceEnvironment,
		TargetEnvironment: &payload.TargetEnvironment,
	}
	utils.WriteSuccessResponse(w, http.StatusAccepted, response)
}

func (c *agentController) UpdateAgentDeploySettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	var payload spec.UpdateAgentDeploySettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("UpdateAgentDeploySettings: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if payload.EnvironmentName == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environmentName is required")
		return
	}

	if err := c.agentService.UpdateAgentDeploySettings(ctx, ouID, projName, agentName, &payload); err != nil {
		log.Error("UpdateAgentDeploySettings: failed to update deploy settings", "error", err)
		handleCommonErrors(w, err, "Failed to update deploy settings")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *agentController) UpdateAgentConfigurations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	var payload spec.UpdateAgentConfigurationsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("UpdateAgentConfigurations: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if payload.EnvironmentName == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environmentName is required")
		return
	}

	if err := c.agentService.UpdateAgentConfigurations(ctx, ouID, projName, agentName, &payload); err != nil {
		log.Error("UpdateAgentConfigurations: failed to update configurations", "error", err)
		handleCommonErrors(w, err, "Failed to update agent configurations")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *agentController) RegenerateTracingToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	var payload spec.TracingTokenRegenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("RegenerateTracingToken: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if payload.EnvironmentName == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environmentName is required")
		return
	}

	expiresIn := ""
	if payload.ExpiresIn != nil {
		expiresIn = *payload.ExpiresIn
	}

	// Rotating the tracing token invalidates the one the deployed agent is
	// using, so it is credential material and is refused when unrecordable.
	attempt, ok := beginAuditOrFail(
		w, r, "RegenerateTracingToken", "Failed to regenerate tracing token", audit.ActionAgentTokenRegenerateTracing,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgent, agentName, agentName),
		audit.Project(projName),
		audit.Environment(payload.EnvironmentName),
		audit.Detail("agentName", agentName),
	)
	if !ok {
		return
	}

	result, err := c.agentService.RegenerateAgentTracingToken(ctx, ouID, projName, agentName, payload.EnvironmentName, expiresIn)
	if err != nil {
		attempt.Complete(ctx, err)
		log.Error("RegenerateTracingToken: failed to regenerate tracing token", "error", err)
		handleCommonErrors(w, err, "Failed to regenerate tracing token")
		return
	}
	attempt.Complete(ctx, nil)

	utils.WriteSuccessResponse(w, http.StatusOK, spec.TracingTokenRegenerateResponse{
		EnvironmentName: result.EnvironmentName,
		ExpiresAt:       result.ExpiresAt,
		RotatedAt:       result.RotatedAt,
	})
}

func (c *agentController) ListAgentBuilds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = strconv.Itoa(utils.DefaultLimit)
	}
	offsetStr := r.URL.Query().Get("offset")
	if offsetStr == "" {
		offsetStr = strconv.Itoa(utils.DefaultOffset)
	}

	// Parse and validate pagination parameters
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < utils.MinLimit || limit > utils.MaxLimit {
		log.Warn("ListAgentBuilds: invalid limit parameter", "limit", limitStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid limit parameter: must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < utils.MinOffset {
		log.Warn("ListAgentBuilds: invalid offset parameter", "offset", offsetStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid offset parameter: must be %d or greater", utils.MinOffset))
		return
	}

	builds, total, err := c.agentService.ListAgentBuilds(ctx, ouID, projName, agentName, int32(limit), int32(offset))
	if err != nil {
		log.Error("ListAgentBuilds: failed to list agent builds", "error", err)
		handleCommonErrors(w, err, "Failed to list agent builds")
		return
	}

	buildResponses := utils.ConvertToBuildListResponse(builds)
	response := &spec.BuildsListResponse{
		Builds: buildResponses,
		Total:  total,
		Limit:  int32(limit),
		Offset: int32(offset),
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *agentController) GenerateName(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	// Parse and validate request body
	var payload spec.ResourceNameRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("GenerateName: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	err := utils.ValidateResourceNameRequest(payload)
	if err != nil {
		log.Warn("GenerateName: invalid resource name payload", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid resource name payload")
		return
	}

	candidateName, err := c.agentService.GenerateName(ctx, ouID, payload)
	if err != nil {
		log.Error("GenerateAgentName: failed to generate agent name", "error", err)
		handleCommonErrors(w, err, "Failed to check agent name availability")
		return
	}

	response := &spec.ResourceNameResponse{
		Name:         candidateName,
		DisplayName:  payload.DisplayName,
		ResourceType: payload.ResourceType,
	}
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *agentController) GetBuild(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	buildName := r.PathValue(utils.PathParamBuildName)

	build, err := c.agentService.GetBuild(ctx, ouID, projName, agentName, buildName)
	if err != nil {
		log.Error("GetBuild: failed to get build", "error", err)
		handleCommonErrors(w, err, "Failed to get build")
		return
	}

	buildResponse := utils.ConvertToBuildDetailsResponse(build)
	utils.WriteSuccessResponse(w, http.StatusOK, buildResponse)
}

func (c *agentController) GetAgentDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	deployments, err := c.agentService.GetAgentDeployments(ctx, ouID, projName, agentName)
	if err != nil {
		log.Error("GetAgentDeployments: failed to get deployments", "error", err)
		handleCommonErrors(w, err, "Failed to get deployments")
		return
	}

	deploymentResponses := utils.ConvertToDeploymentDetailsResponse(deployments)
	utils.WriteSuccessResponse(w, http.StatusOK, deploymentResponses)
}

func (c *agentController) UpdateDeploymentState(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse and validate request body
	var payload spec.UpdateDeploymentStateRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("UpdateDeploymentState: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if payload.Environment == "" {
		log.Warn("UpdateDeploymentState: missing required field 'environment'")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing required field 'environment'")
		return
	}
	if payload.State == "" {
		log.Warn("UpdateDeploymentState: missing required field 'state'")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing required field 'state'")
		return
	}

	// Validate state value
	if payload.State != utils.DeploymentStateActive && payload.State != utils.DeploymentStateUndeploy {
		log.Warn("UpdateDeploymentState: invalid state value", "state", payload.State)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid state value: must be 'Active' or 'Undeploy'")
		return
	}

	// The gating permission is agent:suspend, but this route also resumes and
	// undeploys, so the record names the state actually requested.
	stateAttempt, ok := beginAuditOrFail(
		w, r, "UpdateDeploymentState", "Failed to update deployment state", audit.ActionAgentChangeDeploymentState,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgent, agentName, agentName),
		audit.Project(projName),
		audit.Environment(payload.Environment),
		audit.Detail("agentName", agentName),
		audit.Detail("environment", payload.Environment),
		audit.Detail("toState", string(payload.State)),
	)
	if !ok {
		return
	}

	err := c.agentService.UpdateAgentDeploymentState(ctx, ouID, projName, agentName, payload.Environment, payload.State)
	if err != nil {
		stateAttempt.Complete(ctx, err)
		log.Error("UpdateDeploymentState: failed to update deployment state", "error", err)
		handleCommonErrors(w, err, "Failed to update deployment state")
		return
	}

	stateAttempt.Complete(ctx, nil)

	response := spec.UpdateDeploymentStateResponse{
		Message:     "Deployment state transition request accepted",
		Environment: payload.Environment,
		State:       payload.State,
	}
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *agentController) GetAgentEndpoints(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	environment := r.URL.Query().Get("environment")
	if environment == "" {
		log.Warn("GetAgentEndpoints: missing required query parameter 'environment'")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing required query parameter 'environment'")
		return
	}

	endpoints, err := c.agentService.GetAgentEndpoints(ctx, ouID, projName, agentName, environment)
	if err != nil {
		log.Error("GetAgentEndpoints: failed to get agent endpoints", "error", err)
		handleCommonErrors(w, err, "Failed to get agent endpoints")
		return
	}

	endpointResponses := utils.ConvertToAgentEndpointResponse(endpoints)
	utils.WriteSuccessResponse(w, http.StatusOK, endpointResponses)
}

func (c *agentController) GetAgentConfigurations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	environment := r.URL.Query().Get("environment")
	if environment == "" {
		log.Warn("GetAgentConfigurations: missing required query parameter 'environment'")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing required query parameter 'environment'")
		return
	}

	configurations, err := c.agentService.GetAgentConfigurations(ctx, ouID, projName, agentName, environment)
	if err != nil {
		log.Error("GetAgentConfigurations: failed to get configurations", "error", err)
		handleCommonErrors(w, err, "Failed to get configurations")
		return
	}

	// Convert configurations to response format
	configurationItems := make([]spec.ConfigurationItem, len(configurations))
	for i, config := range configurations {
		value := config.Value
		var secretRef *string
		if config.IsSensitive {
			value = "" // redact sensitive values in the response for extra layer of security
			secretRef = &config.SecretRef
		}
		configurationItems[i] = spec.ConfigurationItem{
			Key:         config.Key,
			Value:       value,
			IsSensitive: spec.PtrBool(config.IsSensitive),
			SecretRef:   secretRef,
			IsSystem:    spec.PtrBool(config.IsSystem),
		}
	}

	// Fetch file mounts
	fileMounts, err := c.agentService.GetAgentFileMounts(ctx, ouID, projName, agentName, environment)
	if err != nil {
		log.Error("GetAgentConfigurations: failed to get file mounts", "error", err)
		handleCommonErrors(w, err, "Failed to get file mounts")
		return
	}

	// Convert file mounts to response format
	fileMountItems := make([]spec.FileMount, 0)
	for _, fm := range fileMounts {
		value := fm.Value
		var secretRef *string
		isSensitive := fm.IsSensitive
		if isSensitive {
			value = ""
			secretRef = &fm.SecretRef
		}
		fileMountItems = append(fileMountItems, spec.FileMount{
			Key:         fm.Key,
			MountPath:   fm.MountPath,
			Value:       &value,
			IsSensitive: &isSensitive,
			SecretRef:   secretRef,
		})
	}

	configurationsResponse := spec.ConfigurationResponse{
		ProjectName: projName,
		AgentName:   agentName,
		Environment: environment,
		Configurations: spec.ConfigurationResponseConfigurations{
			Env:   configurationItems,
			Files: fileMountItems,
		},
	}

	// Per-environment config: seeds the console's Auto-Instrumentation, CORS, and Endpoint
	// Authentication controls for THIS environment, rather than the lowest-environment values
	// GetAgent returns. nil (no config persisted yet) leaves the fields unset.
	envCfg, err := c.agentService.GetAgentEnvConfig(ctx, ouID, projName, agentName, environment)
	if err != nil {
		log.Error("GetAgentConfigurations: failed to read per-env config", "error", err)
		handleCommonErrors(w, err, "Failed to get configurations")
		return
	}
	utils.PopulateConfigurationResponseFromAgentConfig(&configurationsResponse, envCfg)

	utils.WriteSuccessResponse(w, http.StatusOK, configurationsResponse)
}

func (c *agentController) PublishKind(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	var payload spec.PublishAgentKindRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if payload.GetKindName() == "" || payload.GetVersion() == "" || payload.GetBuildName() == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "kindName, version, and buildName are required")
		return
	}

	// The tag is stamped onto every agent created from this version as a label,
	// so reject an unusable one here rather than at agent-creation time.
	if err := utils.ValidateLabelValue(payload.GetVersion(), "version"); err != nil {
		log.Debug("PublishKind: invalid version tag", "error", err)
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	if payload.KindLabels != nil {
		if err := utils.ValidateLabels(*payload.KindLabels); err != nil {
			log.Debug("PublishKind: invalid kind labels", "error", err)
			utils.WriteValidationErrorResponse(w, err)
			return
		}
	}

	result, err := c.agentKindService.PublishKind(ctx, ouID, projName, agentName, &payload)
	if err != nil {
		log.Error("Failed to publish agent kind", "error", err)
		handleCommonErrors(w, err, "Failed to publish agent kind")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, result)
}

// GetAgentIdentity handles GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities
//
// Returns the agent's AgentID binding for every environment in this project's
// deployment pipeline. A safe, side-effect-free read: it never returns or
// destroys a secret; use RegenerateAgentIdentitySecret to obtain a secret for
// an External agent.
//
// An optional ?environment= query parameter filters the result down to that
// one binding — still returned as an array (0 or 1 elements), so the response
// shape stays consistent whether or not the filter is applied.
func (c *agentController) GetAgentIdentity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	environment := r.URL.Query().Get("environment")

	views, err := c.agentService.GetAgentIdentity(ctx, ouID, projName, agentName)
	if err != nil {
		log.Error("GetAgentIdentity: failed to get agent identity", "org_name", orgName, "agent_name", agentName, "error", err)
		handleCommonErrors(w, err, "Failed to get agent identity")
		return
	}

	if views == nil {
		views = []models.AgentIdentityEnvironmentView{}
	}
	if environment != "" {
		filtered := make([]models.AgentIdentityEnvironmentView, 0, 1)
		for _, v := range views {
			if v.EnvironmentName == environment {
				filtered = append(filtered, v)
				break
			}
		}
		views = filtered
	}
	utils.WriteSuccessResponse(w, http.StatusOK, views)
}

// RegenerateAgentIdentitySecret handles
// POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities
//
// Rotates the AgentID secret for one environment. The target environment is
// passed in the request body (POST parameters live in the body, not the query
// string — unlike the GET/PUT/DELETE identity endpoints, which take
// ?environment= since they have no body). The new secret is included in the
// response for both Internal and External agents.
func (c *agentController) RegenerateAgentIdentitySecret(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	var payload models.AgentIdentityActionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("RegenerateAgentIdentitySecret: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	envID := payload.Environment
	if envID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment is required in the request body")
		return
	}

	log.Info("RegenerateAgentIdentitySecret: starting", "org_name", orgName, "agent_name", agentName, "env_id", envID)

	attempt, ok := beginAuditOrFail(
		w, r, "RegenerateAgentIdentitySecret", "Failed to regenerate agent identity secret", audit.ActionAgentIdentityRegenerateSecret,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgentIdentity, agentName, agentName),
		audit.Project(projName),
		audit.Environment(envID),
		audit.Detail("agentName", agentName),
		audit.Detail("environment", envID),
	)
	if !ok {
		return
	}

	resp, err := c.agentService.RegenerateAgentIdentitySecret(ctx, ouID, projName, agentName, envID)
	if err != nil {
		attempt.Complete(ctx, err)
		if errors.Is(err, utils.ErrAgentIdentityNotProvisioned) {
			log.Warn("RegenerateAgentIdentitySecret: identity not yet provisioned", "org_name", orgName, "agent_name", agentName, "env_id", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent identity not yet provisioned for this environment")
			return
		}
		log.Error("RegenerateAgentIdentitySecret: failed to regenerate secret", "org_name", orgName, "agent_name", agentName, "env_id", envID, "error", err)
		handleCommonErrors(w, err, "Failed to regenerate agent identity secret")
		return
	}

	attempt.Complete(ctx, nil)
	log.Info("RegenerateAgentIdentitySecret: secret regenerated successfully", "org_name", orgName, "agent_name", agentName, "env_id", envID)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// RevokeAgentIdentitySecret handles
// DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities?environment={envID}
//
// Invalidates the AgentID secret for one environment. Never returns a usable
// secret — an explicit regenerate afterward is required to restore access.
func (c *agentController) RevokeAgentIdentitySecret(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.URL.Query().Get("environment")
	if envID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment query parameter is required")
		return
	}

	log.Info("RevokeAgentIdentitySecret: starting", "org_name", orgName, "agent_name", agentName, "env_id", envID)

	attempt, ok := beginAuditOrFail(
		w, r, "RevokeAgentIdentitySecret", "Failed to revoke agent identity secret", audit.ActionAgentIdentityRevokeSecret,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgentIdentity, agentName, agentName),
		audit.Project(projName),
		audit.Environment(envID),
		audit.Detail("agentName", agentName),
		audit.Detail("environment", envID),
	)
	if !ok {
		return
	}

	resp, err := c.agentService.RevokeAgentIdentitySecret(ctx, ouID, projName, agentName, envID)
	if err != nil {
		attempt.Complete(ctx, err)
		if errors.Is(err, utils.ErrAgentIdentityNotProvisioned) {
			log.Warn("RevokeAgentIdentitySecret: identity not yet provisioned", "org_name", orgName, "agent_name", agentName, "env_id", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent identity not yet provisioned for this environment")
			return
		}
		log.Error("RevokeAgentIdentitySecret: failed to revoke secret", "org_name", orgName, "agent_name", agentName, "env_id", envID, "error", err)
		handleCommonErrors(w, err, "Failed to revoke agent identity secret")
		return
	}

	attempt.Complete(ctx, nil)
	log.Info("RevokeAgentIdentitySecret: secret revoked successfully", "org_name", orgName, "agent_name", agentName, "env_id", envID)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// ProvisionAgentIdentity handles
// PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities?environment={envID}
//
// Provisions an AgentID for an External agent in an environment that doesn't
// have one yet — e.g. one created (or added to this project's pipeline) after
// the agent already existed. Internal agents are rejected: they receive their
// AgentID automatically during promotion instead. Idempotent (PUT semantics):
// if a binding already exists, it is left untouched and the current state is
// returned rather than provisioning again.
func (c *agentController) ProvisionAgentIdentity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.URL.Query().Get("environment")
	if envID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment query parameter is required")
		return
	}

	log.Info("ProvisionAgentIdentity: starting", "org_name", orgName, "agent_name", agentName, "env_id", envID)

	attempt, ok := beginAuditOrFail(
		w, r, "ProvisionAgentIdentity", "Failed to provision agent identity", audit.ActionAgentIdentityProvision,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgentIdentity, agentName, agentName),
		audit.Project(projName),
		audit.Environment(envID),
		audit.Detail("agentName", agentName),
		audit.Detail("environment", envID),
	)
	if !ok {
		return
	}

	view, alreadyExisted, err := c.agentService.ProvisionAgentIdentity(ctx, ouID, projName, agentName, envID)
	if err != nil {
		attempt.Complete(ctx, err)
		log.Error("ProvisionAgentIdentity: failed to provision agent identity", "org_name", orgName, "agent_name", agentName, "env_id", envID, "error", err)
		handleCommonErrors(w, err, "Failed to provision agent identity")
		return
	}

	status := http.StatusAccepted
	if alreadyExisted {
		status = http.StatusOK
	}
	// A no-op provision (the binding already existed) issues no new credential,
	// so it is recorded as a read-level notice rather than a credential change.
	if alreadyExisted {
		attempt.Complete(ctx, nil, audit.SeverityOpt(audit.SeverityNotice), audit.Detail("alreadyExisted", true))
	} else {
		attempt.Complete(ctx, nil)
	}
	log.Info("ProvisionAgentIdentity: completed", "org_name", orgName, "agent_name", agentName, "env_id", envID, "already_existed", alreadyExisted)
	utils.WriteSuccessResponse(w, status, view)
}

// RetryAgentIdentityProvisioning handles
// POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities/retry
//
// Resets a FAILED AgentID binding back to PENDING and re-attempts
// provisioning, for both Internal and External agents. The target
// environment is passed in the request body, matching
// RegenerateAgentIdentitySecret's convention for POST identity actions.
func (c *agentController) RetryAgentIdentityProvisioning(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	var payload models.AgentIdentityActionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("RetryAgentIdentityProvisioning: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	envID := payload.Environment
	if envID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment is required in the request body")
		return
	}

	log.Info("RetryAgentIdentityProvisioning: starting", "org_name", orgName, "agent_name", agentName, "env_id", envID)

	attempt, ok := beginAuditOrFail(
		w, r, "RetryAgentIdentityProvisioning", "Failed to retry agent identity provisioning", audit.ActionAgentIdentityRetryProvisioning,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgentIdentity, agentName, agentName),
		audit.Project(projName),
		audit.Environment(envID),
		audit.Detail("agentName", agentName),
		audit.Detail("environment", envID),
	)
	if !ok {
		return
	}

	view, err := c.agentService.RetryAgentIdentityProvisioning(ctx, ouID, projName, agentName, envID)
	if err != nil {
		attempt.Complete(ctx, err)
		if errors.Is(err, utils.ErrAgentIdentityNotProvisioned) {
			log.Warn("RetryAgentIdentityProvisioning: identity not yet provisioned", "org_name", orgName, "agent_name", agentName, "env_id", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent identity not yet provisioned for this environment")
			return
		}
		if errors.Is(err, utils.ErrAgentIdentityRetryNotAllowed) {
			log.Warn("RetryAgentIdentityProvisioning: binding not in a failed state", "org_name", orgName, "agent_name", agentName, "env_id", envID)
			utils.WriteErrorResponse(w, http.StatusConflict, "Agent identity binding is not in a failed state and cannot be retried")
			return
		}
		log.Error("RetryAgentIdentityProvisioning: failed to retry provisioning", "org_name", orgName, "agent_name", agentName, "env_id", envID, "error", err)
		handleCommonErrors(w, err, "Failed to retry agent identity provisioning")
		return
	}

	attempt.Complete(ctx, nil)
	log.Info("RetryAgentIdentityProvisioning: retry scheduled", "org_name", orgName, "agent_name", agentName, "env_id", envID)
	utils.WriteSuccessResponse(w, http.StatusAccepted, view)
}

// GetAgentRoles handles
// GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/roles?environment={envID}
//
// Returns the Thunder roles assigned to the agent's AgentID in one
// environment. An agent's AgentID (and its role assignments) is per
// environment, so `environment` is required — there is no single answer
// across every environment the agent is deployed to.
func (c *agentController) GetAgentRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.URL.Query().Get("environment")
	if envID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment query parameter is required")
		return
	}

	log.Info("GetAgentRoles: starting", "org_name", orgName, "agent_name", agentName, "env_id", envID)

	roles, err := c.agentService.GetAgentRoles(ctx, ouID, projName, agentName, envID)
	if err != nil {
		if errors.Is(err, utils.ErrAgentIdentityNotProvisioned) {
			log.Warn("GetAgentRoles: identity not yet provisioned", "org_name", orgName, "agent_name", agentName, "env_id", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent identity not yet provisioned for this environment")
			return
		}
		log.Error("GetAgentRoles: failed to get agent roles", "org_name", orgName, "agent_name", agentName, "env_id", envID, "error", err)
		handleCommonErrors(w, err, "Failed to get agent roles")
		return
	}

	log.Info("GetAgentRoles: completed", "org_name", orgName, "agent_name", agentName, "env_id", envID)
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"roles": roles})
}

// GetAgentGroups handles
// GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/groups?environment={envID}
//
// Returns the Thunder groups the agent's AgentID belongs to in one
// environment. An agent's AgentID (and its group memberships) is per
// environment, so `environment` is required — there is no single answer
// across every environment the agent is deployed to.
func (c *agentController) GetAgentGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.URL.Query().Get("environment")
	if envID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment query parameter is required")
		return
	}

	log.Info("GetAgentGroups: starting", "org_name", orgName, "agent_name", agentName, "env_id", envID)

	groups, err := c.agentService.GetAgentGroups(ctx, ouID, projName, agentName, envID)
	if err != nil {
		if errors.Is(err, utils.ErrAgentIdentityNotProvisioned) {
			log.Warn("GetAgentGroups: identity not yet provisioned", "org_name", orgName, "agent_name", agentName, "env_id", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent identity not yet provisioned for this environment")
			return
		}
		log.Error("GetAgentGroups: failed to get agent groups", "org_name", orgName, "agent_name", agentName, "env_id", envID, "error", err)
		handleCommonErrors(w, err, "Failed to get agent groups")
		return
	}

	log.Info("GetAgentGroups: completed", "org_name", orgName, "agent_name", agentName, "env_id", envID)
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"groups": groups})
}
