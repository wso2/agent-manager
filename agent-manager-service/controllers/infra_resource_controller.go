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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

type InfraResourceController interface {
	ListOrgEnvironments(w http.ResponseWriter, r *http.Request)
	GetProjectDeploymentPipeline(w http.ResponseWriter, r *http.Request)
	ListOrganizations(w http.ResponseWriter, r *http.Request)
	GetOrganization(w http.ResponseWriter, r *http.Request)
	ListProjects(w http.ResponseWriter, r *http.Request)
	GetProject(w http.ResponseWriter, r *http.Request)
	CreateProject(w http.ResponseWriter, r *http.Request)
	DeleteProject(w http.ResponseWriter, r *http.Request)
	ListOrgDeploymentPipelines(w http.ResponseWriter, r *http.Request)
	GetDataplanes(w http.ResponseWriter, r *http.Request)
}

type infraResourceController struct {
	infraResourceManager services.InfraResourceManager
}

// NewInfraResourceController returns a new InfraResourceController instance.
func NewInfraResourceController(infraResourceManager services.InfraResourceManager) InfraResourceController {
	return &infraResourceController{
		infraResourceManager: infraResourceManager,
	}
}

func (c *infraResourceController) ListOrganizations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract user info from JWT token
	tokenClaims := jwtassertion.GetTokenClaims(ctx)
	userIdpId := tokenClaims.Sub

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
		log.Error("ListOrganizations: invalid limit parameter", "limit", limitStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid limit parameter: must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < utils.MinOffset {
		log.Error("ListOrganizations: invalid offset parameter", "offset", offsetStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid offset parameter: must be %d or greater", utils.MinOffset))
		return
	}

	orgs, total, err := c.infraResourceManager.ListOrganizations(ctx, userIdpId, limit, offset)
	if err != nil {
		log.Error("ListOrganizations: failed to list organizations", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list organizations")
		return
	}

	orgList := utils.ConvertToOrganizationListResponse(orgs)
	orgResponse := &spec.OrganizationListResponse{
		Organizations: orgList,
		Total:         total,
		Limit:         int32(limit),
		Offset:        int32(offset),
	}
	utils.WriteSuccessResponse(w, http.StatusOK, orgResponse)
}

func (c *infraResourceController) GetOrganization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)

	// Extract user info from JWT token
	tokenClaims := jwtassertion.GetTokenClaims(ctx)
	userIdpId := tokenClaims.Sub

	org, err := c.infraResourceManager.GetOrganization(ctx, userIdpId, orgName)
	if err != nil {
		log.Error("GetOrganization: failed to get organization", "error", err)
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get organization")
		return
	}

	orgResponse := utils.ConvertToOrganizationResponse(org)
	utils.WriteSuccessResponse(w, http.StatusOK, orgResponse)
}

func (c *infraResourceController) ListProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)

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
		log.Error("ListProjects: invalid limit parameter", "limit", limitStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid limit parameter: must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < utils.MinOffset {
		log.Error("ListProjects: invalid offset parameter", "offset", offsetStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid offset parameter: must be %d or greater", utils.MinOffset))
		return
	}

	// Extract user info from JWT token
	tokenClaims := jwtassertion.GetTokenClaims(ctx)
	userIdpId := tokenClaims.Sub

	projects, total, err := c.infraResourceManager.ListProjects(ctx, userIdpId, orgName, limit, offset)
	if err != nil {
		log.Error("ListProjects: failed to list projects", "error", err)
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list projects")
		return
	}
	projectList := utils.ConvertToProjectListResponse(projects)
	projectResponse := &spec.ProjectListResponse{
		Projects: projectList,
		Total:    total,
		Limit:    int32(limit),
		Offset:   int32(offset),
	}
	utils.WriteSuccessResponse(w, http.StatusOK, projectResponse)
}

func (c *infraResourceController) CreateProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)

	// Extract user info from JWT token
	tokenClaims := jwtassertion.GetTokenClaims(ctx)
	userIdpId := tokenClaims.Sub

	// Parse and validate request body
	var payload spec.CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("CreateProject: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.ValidateResourceName(payload.Name, "project"); err != nil {
		log.Error("CreateProject: invalid project name", "projectName", payload.Name, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid project name")
		return
	}

	if err := utils.ValidateResourceDisplayName(payload.DisplayName, "project"); err != nil {
		log.Error("CreateProject: invalid project display name", "projectDisplayName", payload.DisplayName, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid project display name")
		return
	}

	if payload.DeploymentPipeline == "" {
		log.Error("CreateProject: missing deployment pipeline in request body")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing deployment pipeline in request body")
		return
	}

	project, err := c.infraResourceManager.CreateProject(ctx, userIdpId, orgName, payload)
	if err != nil {
		log.Error("CreateProject: failed to create project", "error", err)
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
			return
		}
		if errors.Is(err, utils.ErrProjectAlreadyExists) {
			utils.WriteErrorResponse(w, http.StatusConflict, "Project already exists")
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create project")
		return
	}
	projectResponse := spec.ProjectResponse{
		Name:               project.Name,
		DisplayName:        project.DisplayName,
		Description:        project.Description,
		DeploymentPipeline: project.DeploymentPipeline,
		OrgName:            project.OrgName,
		CreatedAt:          project.CreatedAt,
	}

	utils.WriteSuccessResponse(w, http.StatusAccepted, projectResponse)
}

func (c *infraResourceController) DeleteProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projectName := r.PathValue(utils.PathParamProjName)

	// Extract user info from JWT token
	tokenClaims := jwtassertion.GetTokenClaims(ctx)
	userIdpId := tokenClaims.Sub

	err := c.infraResourceManager.DeleteProject(ctx, userIdpId, orgName, projectName)
	if err != nil {
		log.Error("DeleteProject: failed to delete project", "error", err)
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete project")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, "")
}

func (c *infraResourceController) ListOrgDeploymentPipelines(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)

	// Extract user info from JWT token
	tokenClaims := jwtassertion.GetTokenClaims(ctx)
	userIdpId := tokenClaims.Sub

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
		log.Error("ListOrgDeploymentPipelines: invalid limit parameter", "limit", limitStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid limit parameter: must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < utils.MinOffset {
		log.Error("ListOrgDeploymentPipelines: invalid offset parameter", "offset", offsetStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid offset parameter: must be %d or greater", utils.MinOffset))
		return
	}

	deploymentPipelines, total, err := c.infraResourceManager.ListOrgDeploymentPipelines(ctx, userIdpId, orgName, limit, offset)
	if err != nil {
		log.Error("GetDeploymentPipelines: failed to get deployment pipelines", "error", err)
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get deployment pipelines")
		return
	}

	deploymentPipelinesResponse := utils.ConvertToDeploymentPipelinesListResponse(deploymentPipelines, int32(total), int32(limit), int32(offset))
	utils.WriteSuccessResponse(w, http.StatusOK, deploymentPipelinesResponse)
}

func (c *infraResourceController) GetProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projectName := r.PathValue(utils.PathParamProjName)

	// Extract user info from JWT token
	tokenClaims := jwtassertion.GetTokenClaims(ctx)
	userIdpId := tokenClaims.Sub

	project, err := c.infraResourceManager.GetProject(ctx, userIdpId, orgName, projectName)
	if err != nil {
		log.Error("GetProject: failed to get project", "error", err)
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
			return
		}
		if errors.Is(err, utils.ErrProjectNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get project")
		return
	}

	projectResponse := utils.ConvertToProjectResponse(project)

	utils.WriteSuccessResponse(w, http.StatusOK, projectResponse)
}

func (c *infraResourceController) ListOrgEnvironments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)

	// Extract user info from JWT token
	tokenClaims := jwtassertion.GetTokenClaims(ctx)
	userIdpId := tokenClaims.Sub

	environments, err := c.infraResourceManager.ListOrgEnvironments(ctx, userIdpId, orgName)
	if err != nil {
		log.Error("GetOrgEnvironments: failed to get environments", "error", err)
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get environments")
		return
	}
	environmentsListResponse := utils.ConvertToEnvironmentListResponse(environments)
	utils.WriteSuccessResponse(w, http.StatusOK, environmentsListResponse)
}

func (c *infraResourceController) GetProjectDeploymentPipeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projectName := r.PathValue(utils.PathParamProjName)

	// Extract user info from JWT token
	tokenClaims := jwtassertion.GetTokenClaims(ctx)
	userIdpId := tokenClaims.Sub

	deploymentPipeline, err := c.infraResourceManager.GetProjectDeploymentPipeline(ctx, userIdpId, orgName, projectName)
	if err != nil {
		log.Error("GetProjectDeploymentPipeline: failed to get deployment pipeline", "error", err)
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
			return
		}
		if errors.Is(err, utils.ErrProjectNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Project not found")
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get deployment pipeline")
		return
	}

	deploymentPipelineResponse := utils.ConvertToDeploymentPipelineResponse(deploymentPipeline)

	utils.WriteSuccessResponse(w, http.StatusOK, deploymentPipelineResponse)
}

func (c *infraResourceController) GetDataplanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)

	// Extract user info from JWT token
	tokenClaims := jwtassertion.GetTokenClaims(ctx)
	userIdpId := tokenClaims.Sub

	dataplanes, err := c.infraResourceManager.GetDataplanes(ctx, userIdpId, orgName)
	if err != nil {
		log.Error("GetDataplanes: failed to get dataplanes", "error", err)
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get dataplanes")
		return
	}
	dataplaneListResponse := utils.ConvertToDataPlaneListResponse(dataplanes)
	utils.WriteSuccessResponse(w, http.StatusOK, dataplaneListResponse)
}
