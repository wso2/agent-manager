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

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type InfraResourceController interface {
	ListOrgEnvironments(w http.ResponseWriter, r *http.Request)
	GetProjectDeploymentPipeline(w http.ResponseWriter, r *http.Request)
	ListOrganizations(w http.ResponseWriter, r *http.Request)
	GetOrganization(w http.ResponseWriter, r *http.Request)
	ListProjects(w http.ResponseWriter, r *http.Request)
	GetProject(w http.ResponseWriter, r *http.Request)
	CreateProject(w http.ResponseWriter, r *http.Request)
	UpdateProject(w http.ResponseWriter, r *http.Request)
	DeleteProject(w http.ResponseWriter, r *http.Request)
	ListOrgDeploymentPipelines(w http.ResponseWriter, r *http.Request)
	CreateOrgDeploymentPipeline(w http.ResponseWriter, r *http.Request)
	UpdateOrgDeploymentPipeline(w http.ResponseWriter, r *http.Request)
	DeleteOrgDeploymentPipeline(w http.ResponseWriter, r *http.Request)
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
		log.Warn("ListOrganizations: invalid limit parameter", "limit", limitStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid limit parameter: must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < utils.MinOffset {
		log.Warn("ListOrganizations: invalid offset parameter", "offset", offsetStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid offset parameter: must be %d or greater", utils.MinOffset))
		return
	}

	orgs, total, err := c.infraResourceManager.ListOrganizations(ctx, limit, offset)
	if err != nil {
		log.Error("ListOrganizations: failed to list organizations", "error", err)
		handleCommonErrors(w, err, "Failed to list organizations")
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
	ouID := middleware.OUIDFromRequest(r)

	org, err := c.infraResourceManager.GetOrganization(ctx, ouID)
	if err != nil {
		log.Error("GetOrganization: failed to get organization", "error", err)
		handleCommonErrors(w, err, "Failed to get organization")
		return
	}

	orgResponse := utils.ConvertToOrganizationResponse(org)
	utils.WriteSuccessResponse(w, http.StatusOK, orgResponse)
}

func (c *infraResourceController) ListProjects(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)

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
		log.Warn("ListProjects: invalid limit parameter", "limit", limitStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid limit parameter: must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < utils.MinOffset {
		log.Warn("ListProjects: invalid offset parameter", "offset", offsetStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid offset parameter: must be %d or greater", utils.MinOffset))
		return
	}

	projects, total, err := c.infraResourceManager.ListProjects(ctx, ouID, limit, offset)
	if err != nil {
		log.Error("ListProjects: failed to list projects", "error", err)
		handleCommonErrors(w, err, "Failed to list projects")
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
	ouID := middleware.OUIDFromRequest(r)

	// Parse and validate request body
	var payload spec.CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("CreateProject: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.ValidateResourceName(payload.Name, "project"); err != nil {
		log.Warn("CreateProject: invalid project name", "project_name", payload.Name, "error", err)
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	if err := utils.ValidateResourceDisplayName(payload.DisplayName, "project"); err != nil {
		log.Warn("CreateProject: invalid project display name", "project_display_name", payload.DisplayName, "error", err)
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	if payload.DeploymentPipeline == "" {
		log.Warn("CreateProject: missing deployment pipeline in request body")
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing deployment pipeline in request body")
		return
	}

	project, err := c.infraResourceManager.CreateProject(ctx, ouID, payload)
	if err != nil {
		log.Error("CreateProject: failed to create project", "error", err)
		handleCommonErrors(w, err, "Failed to create project")
		return
	}
	projectResponse := spec.ProjectResponse{
		Name:               project.Name,
		DisplayName:        project.DisplayName,
		Description:        project.Description,
		DeploymentPipeline: project.DeploymentPipeline,
		CreatedAt:          project.CreatedAt,
	}

	utils.WriteSuccessResponse(w, http.StatusAccepted, projectResponse)
}

func (c *infraResourceController) UpdateProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)

	// Parse and validate request body
	var payload spec.UpdateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("UpdateProject: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if err := utils.ValidateProjectUpdatePayload(payload); err != nil {
		log.Warn("UpdateProject: invalid project payload", "error", err)
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	project, err := c.infraResourceManager.UpdateProject(ctx, ouID, projectName, payload)
	if err != nil {
		log.Error("UpdateProject: failed to update project", "error", err)
		handleCommonErrors(w, err, "Failed to update project")
		return
	}

	projectResponse := spec.ProjectResponse{
		Name:               project.Name,
		DisplayName:        project.DisplayName,
		Description:        project.Description,
		DeploymentPipeline: project.DeploymentPipeline,
		CreatedAt:          project.CreatedAt,
	}

	utils.WriteSuccessResponse(w, http.StatusOK, projectResponse)
}

func (c *infraResourceController) DeleteProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)

	err := c.infraResourceManager.DeleteProject(ctx, ouID, projectName)
	if err != nil {
		log.Error("DeleteProject: failed to delete project", "error", err)
		handleCommonErrors(w, err, "Failed to delete project")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, "")
}

func (c *infraResourceController) ListOrgDeploymentPipelines(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)

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
		log.Warn("ListOrgDeploymentPipelines: invalid limit parameter", "limit", limitStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid limit parameter: must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < utils.MinOffset {
		log.Warn("ListOrgDeploymentPipelines: invalid offset parameter", "offset", offsetStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid offset parameter: must be %d or greater", utils.MinOffset))
		return
	}

	deploymentPipelines, total, err := c.infraResourceManager.ListOrgDeploymentPipelines(ctx, ouID, limit, offset)
	if err != nil {
		log.Error("ListOrgDeploymentPipelines: failed to get deployment pipelines", "error", err)
		handleCommonErrors(w, err, "Failed to get deployment pipelines")
		return
	}

	deploymentPipelinesResponse := utils.ConvertToDeploymentPipelinesListResponse(deploymentPipelines, int32(total), int32(limit), int32(offset))
	utils.WriteSuccessResponse(w, http.StatusOK, deploymentPipelinesResponse)
}

func (c *infraResourceController) GetProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)

	project, err := c.infraResourceManager.GetProject(ctx, ouID, projectName)
	if err != nil {
		log.Warn("GetProject: failed to get project", "error", err)
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
	ouID := middleware.OUIDFromRequest(r)

	environments, err := c.infraResourceManager.ListOrgEnvironments(ctx, ouID)
	if err != nil {
		log.Error("ListOrgEnvironments: failed to get environments", "error", err)
		handleCommonErrors(w, err, "Failed to get environments")
		return
	}
	environmentsListResponse := utils.ConvertToEnvironmentListResponse(environments)
	utils.WriteSuccessResponse(w, http.StatusOK, environmentsListResponse)
}

func requirePromotionPaths(w http.ResponseWriter, paths []spec.PromotionPath) bool {
	if len(paths) == 0 {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "promotionPaths must contain at least one entry")
		return false
	}
	return true
}

func convertSpecPromotionPaths(specPaths []spec.PromotionPath) []models.PromotionPath {
	modelPaths := make([]models.PromotionPath, len(specPaths))
	for i, p := range specPaths {
		targets := make([]models.TargetEnvironmentRef, len(p.TargetEnvironmentRefs))
		for j, t := range p.TargetEnvironmentRefs {
			targets[j] = models.TargetEnvironmentRef{Name: t.Name}
		}
		modelPaths[i] = models.PromotionPath{
			SourceEnvironmentRef:  p.SourceEnvironmentRef,
			TargetEnvironmentRefs: targets,
		}
	}
	return modelPaths
}

func (c *infraResourceController) CreateOrgDeploymentPipeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)

	var payload spec.CreateDeploymentPipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("CreateOrgDeploymentPipeline: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if payload.DisplayName == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "displayName is required")
		return
	}

	if !requirePromotionPaths(w, payload.PromotionPaths) {
		return
	}

	created, err := c.infraResourceManager.CreateOrgDeploymentPipeline(ctx, ouID, payload.DisplayName, payload.Description, payload.ProjectName, convertSpecPromotionPaths(payload.PromotionPaths))
	if err != nil {
		log.Error("CreateOrgDeploymentPipeline: failed to create deployment pipeline", "error", err)
		handleCommonErrors(w, err, "Failed to create deployment pipeline")
		return
	}

	response := utils.ConvertToDeploymentPipelineResponse(created)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *infraResourceController) UpdateOrgDeploymentPipeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	pipelineName := r.PathValue(utils.PathParamPipelineName)

	var payload spec.UpdateDeploymentPipelineRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Warn("UpdateOrgDeploymentPipeline: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if !requirePromotionPaths(w, payload.PromotionPaths) {
		return
	}

	updated, err := c.infraResourceManager.UpdateOrgDeploymentPipeline(ctx, ouID, pipelineName, payload.DisplayName, payload.Description, convertSpecPromotionPaths(payload.PromotionPaths))
	if err != nil {
		log.Error("UpdateOrgDeploymentPipeline: failed to update deployment pipeline", "error", err)
		handleCommonErrors(w, err, "Failed to update deployment pipeline")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, utils.ConvertToDeploymentPipelineResponse(updated))
}

func (c *infraResourceController) GetProjectDeploymentPipeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projectName := r.PathValue(utils.PathParamProjName)

	deploymentPipeline, err := c.infraResourceManager.GetProjectDeploymentPipeline(ctx, ouID, projectName)
	if err != nil {
		log.Error("GetProjectDeploymentPipeline: failed to get deployment pipeline", "error", err)
		handleCommonErrors(w, err, "Failed to get deployment pipeline")
		return
	}

	deploymentPipelineResponse := utils.ConvertToDeploymentPipelineResponse(deploymentPipeline)
	utils.WriteSuccessResponse(w, http.StatusOK, deploymentPipelineResponse)
}

func (c *infraResourceController) DeleteOrgDeploymentPipeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	deploymentPipelineName := r.PathValue(utils.PathParamPipelineName)

	if err := c.infraResourceManager.DeleteOrgDeploymentPipeline(ctx, ouID, deploymentPipelineName); err != nil {
		log.Error("DeleteOrgDeploymentPipeline: failed to delete deployment pipeline", "error", err)
		handleCommonErrors(w, err, "Failed to delete deployment pipeline")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *infraResourceController) GetDataplanes(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)

	dataplanes, err := c.infraResourceManager.GetDataplanes(ctx, ouID)
	if err != nil {
		log.Error("GetDataplanes: failed to get dataplanes", "error", err)
		handleCommonErrors(w, err, "Failed to list dataplanes")
		return
	}
	dataplaneListResponse := utils.ConvertToDataPlaneListResponse(dataplanes)
	utils.WriteSuccessResponse(w, http.StatusOK, dataplaneListResponse)
}
