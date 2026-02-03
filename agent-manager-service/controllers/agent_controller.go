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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

type AgentController interface {
	ListAgents(w http.ResponseWriter, r *http.Request)
	GetAgent(w http.ResponseWriter, r *http.Request)
	CreateAgent(w http.ResponseWriter, r *http.Request)
	UpdateAgent(w http.ResponseWriter, r *http.Request)
	DeleteAgent(w http.ResponseWriter, r *http.Request)
	BuildAgent(w http.ResponseWriter, r *http.Request)
	DeployAgent(w http.ResponseWriter, r *http.Request)
	ListAgentBuilds(w http.ResponseWriter, r *http.Request)
	GetAgentDeployments(w http.ResponseWriter, r *http.Request)
	GetAgentEndpoints(w http.ResponseWriter, r *http.Request)
	GetBuild(w http.ResponseWriter, r *http.Request)
	GetAgentConfigurations(w http.ResponseWriter, r *http.Request)
	GetBuildLogs(w http.ResponseWriter, r *http.Request)
	GenerateName(w http.ResponseWriter, r *http.Request)
	GetAgentMetrics(w http.ResponseWriter, r *http.Request)
	GetAgentRuntimeLogs(w http.ResponseWriter, r *http.Request)
}

type agentController struct {
	agentService services.AgentManagerService
}

// NewAgentController returns a new AgentController instance.
func NewAgentController(agentService services.AgentManagerService) AgentController {
	return &agentController{
		agentService: agentService,
	}
}

// handleCommonErrors checks for common resource errors and writes appropriate responses.
// If no common error matches, writes an internal server error with the provided fallback message.
func handleCommonErrors(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, utils.ErrOrganizationNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
	case errors.Is(err, utils.ErrProjectNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
	case errors.Is(err, utils.ErrAgentNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Agent not found")
	case errors.Is(err, utils.ErrBuildNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Build not found")
	case errors.Is(err, utils.ErrAgentAlreadyExists):
		utils.WriteErrorResponse(w, http.StatusConflict, "Agent already exists")
	case errors.Is(err, utils.ErrImmutableFieldChange):
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
	default:
		utils.WriteErrorResponse(w, http.StatusInternalServerError, fallbackMsg)
	}
}

func (c *agentController) GetAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	agent, err := c.agentService.GetAgent(ctx, orgName, projName, agentName)
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
	orgName := r.PathValue(utils.PathParamOrgName)
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
		log.Error("ListAgents: invalid limit parameter", "limit", limitStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid limit parameter: must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < utils.MinOffset {
		log.Error("ListAgents: invalid offset parameter", "offset", offsetStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid offset parameter: must be %d or greater", utils.MinOffset))
		return
	}

	agents, total, err := c.agentService.ListAgents(ctx, orgName, projName, int32(limit), int32(offset))
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

func (c *agentController) CreateAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)

	// Parse and validate request body
	var payload spec.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("CreateAgent: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.ValidateAgentCreatePayload(payload); err != nil {
		log.Error("CreateAgent: invalid agent payload", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	err := c.agentService.CreateAgent(ctx, orgName, projName, &payload)
	if err != nil {
		log.Error("CreateAgent: failed to create agent", "error", err)
		handleCommonErrors(w, err, "Failed to create agent")
		return
	}
	response := &spec.AgentResponse{
		Name:           payload.Name,
		DisplayName:    payload.DisplayName,
		Description:    utils.StrPointerAsStr(payload.Description, ""),
		ProjectName:    projName,
		Provisioning:   payload.Provisioning,
		AgentType:      payload.AgentType,
		RuntimeConfigs: payload.RuntimeConfigs,
		CreatedAt:      time.Now(),
	}

	utils.WriteSuccessResponse(w, http.StatusAccepted, response)
}

func (c *agentController) UpdateAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse and validate request body
	var payload spec.UpdateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("UpdateAgent: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.ValidateAgentUpdatePayload(payload); err != nil {
		log.Error("UpdateAgent: invalid agent payload", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	agent, err := c.agentService.UpdateAgent(ctx, orgName, projName, agentName, &payload)
	if err != nil {
		log.Error("UpdateAgent: failed to update agent", "error", err)
		handleCommonErrors(w, err, "Failed to update agent")
		return
	}

	agentResponse := utils.ConvertToAgentResponse(agent)
	utils.WriteSuccessResponse(w, http.StatusOK, agentResponse)
}

func (c *agentController) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	err := c.agentService.DeleteAgent(ctx, orgName, projName, agentName)
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
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse query parameters
	commitId := r.URL.Query().Get("commitId")
	if commitId == "" {
		log.Debug("BuildAgent: commitId not provided, using latest commit")
	}

	build, err := c.agentService.BuildAgent(ctx, orgName, projName, agentName, commitId)
	if err != nil {
		log.Error("BuildAgent: failed to build agent", "error", err)
		handleCommonErrors(w, err, "Failed to build agent")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusAccepted, build)
}

func (c *agentController) GetBuildLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	buildName := r.PathValue(utils.PathParamBuildName)

	buildLogs, err := c.agentService.GetBuildLogs(ctx, orgName, projName, agentName, buildName)
	if err != nil {
		log.Error("GetBuildLogs: failed to get build logs", "error", err)
		handleCommonErrors(w, err, "Failed to get build logs")
		return
	}
	buildLogsResponse := utils.ConvertToLogsResponse(*buildLogs)
	utils.WriteSuccessResponse(w, http.StatusOK, buildLogsResponse)
}

func (c *agentController) GetAgentRuntimeLogs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse and validate request body
	var payload spec.LogFilterRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("GetAgentRuntimeLogs: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.ValidateLogFilterRequest(payload); err != nil {
		log.Error("GetAgentRuntimeLogs: invalid request payload", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	applicationLogs, err := c.agentService.GetAgentRuntimeLogs(ctx, orgName, projName, agentName, payload)
	if err != nil {
		log.Error("GetAgentRuntimeLogs: failed to get build logs", "error", err)
		handleCommonErrors(w, err, "Failed to get build logs")
		return
	}
	buildLogsResponse := utils.ConvertToLogsResponse(*applicationLogs)
	utils.WriteSuccessResponse(w, http.StatusOK, buildLogsResponse)
}

func (c *agentController) GetAgentMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse and validate request body
	var payload spec.MetricsFilterRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("GetAgentMetrics: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.ValidateMetricsFilterRequest(payload); err != nil {

		log.Error("GetAgentMetrics: invalid request payload", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	metricsResponse, err := c.agentService.GetAgentMetrics(ctx, orgName, projName, agentName, payload)
	if err != nil {
		log.Error("GetAgentMetrics: failed to get agent metrics", "error", err)
		handleCommonErrors(w, err, "Failed to get agent metrics")
		return
	}
	utils.WriteSuccessResponse(w, http.StatusOK, metricsResponse)
}

func (c *agentController) DeployAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse and validate request body
	var payload spec.DeployAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("DeployAgent: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if payload.ImageId == "" {
		log.Error("DeployAgent: imageId is required in request body")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	deployedEnv, err := c.agentService.DeployAgent(ctx, orgName, projName, agentName, &payload)
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

func (c *agentController) ListAgentBuilds(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
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
		log.Error("ListAgentBuilds: invalid limit parameter", "limit", limitStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid limit parameter: must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < utils.MinOffset {
		log.Error("ListAgentBuilds: invalid offset parameter", "offset", offsetStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid offset parameter: must be %d or greater", utils.MinOffset))
		return
	}

	builds, total, err := c.agentService.ListAgentBuilds(ctx, orgName, projName, agentName, int32(limit), int32(offset))
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
	orgName := r.PathValue(utils.PathParamOrgName)
	// Parse and validate request body
	var payload spec.ResourceNameRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("GenerateName: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	err := utils.ValidateResourceNameRequest(payload)
	if err != nil {
		log.Error("GenerateName: invalid resource name payload", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid resource name payload")
		return
	}

	candidateName, err := c.agentService.GenerateName(ctx, orgName, payload)
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
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	buildName := r.PathValue(utils.PathParamBuildName)

	build, err := c.agentService.GetBuild(ctx, orgName, projName, agentName, buildName)
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
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	deployments, err := c.agentService.GetAgentDeployments(ctx, orgName, projName, agentName)
	if err != nil {
		log.Error("GetAgentDeployments: failed to get deployments", "error", err)
		handleCommonErrors(w, err, "Failed to get deployments")
		return
	}

	deploymentResponses := utils.ConvertToDeploymentDetailsResponse(deployments)
	utils.WriteSuccessResponse(w, http.StatusOK, deploymentResponses)
}

func (c *agentController) GetAgentEndpoints(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	environment := r.URL.Query().Get("environment")
	if environment == "" {
		log.Error("GetAgentEndpoints: missing required query parameter 'environment'")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing required query parameter 'environment'")
		return
	}

	endpoints, err := c.agentService.GetAgentEndpoints(ctx, orgName, projName, agentName, environment)
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
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	environment := r.URL.Query().Get("environment")
	if environment == "" {
		log.Error("GetAgentConfigurations: missing required query parameter 'environment'")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing required query parameter 'environment'")
		return
	}

	configurations, err := c.agentService.GetAgentConfigurations(ctx, orgName, projName, agentName, environment)
	if err != nil {
		log.Error("GetAgentConfigurations: failed to get configurations", "error", err)
		handleCommonErrors(w, err, "Failed to get configurations")
		return
	}

	// Convert configurations to response format
	configurationItems := make([]spec.ConfigurationItem, len(configurations))
	for i, config := range configurations {
		configurationItems[i] = spec.ConfigurationItem{
			Key:   config.Key,
			Value: config.Value,
		}
	}

	configurationsResponse := spec.ConfigurationResponse{
		ProjectName:    projName,
		AgentName:      agentName,
		Environment:    environment,
		Configurations: configurationItems,
	}

	utils.WriteSuccessResponse(w, http.StatusOK, configurationsResponse)
}
