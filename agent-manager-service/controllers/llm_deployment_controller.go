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

// LLMDeploymentController defines interface for LLM deployment HTTP handlers
type LLMDeploymentController interface {
	DeployLLMProvider(w http.ResponseWriter, r *http.Request)
	UndeployLLMProviderDeployment(w http.ResponseWriter, r *http.Request)
	RestoreLLMProviderDeployment(w http.ResponseWriter, r *http.Request)
	DeleteLLMProviderDeployment(w http.ResponseWriter, r *http.Request)
	GetLLMProviderDeployment(w http.ResponseWriter, r *http.Request)
	GetLLMProviderDeployments(w http.ResponseWriter, r *http.Request)
}

type llmDeploymentController struct {
	deploymentService *services.LLMProviderDeploymentService
	orgRepo           repositories.OrganizationRepository
}

// NewLLMDeploymentController creates a new LLM deployment controller
func NewLLMDeploymentController(
	deploymentService *services.LLMProviderDeploymentService,
	orgRepo repositories.OrganizationRepository,
) LLMDeploymentController {
	return &llmDeploymentController{
		deploymentService: deploymentService,
		orgRepo:           orgRepo,
	}
}

// resolveOrgUUID resolves organization handle to UUID
func (c *llmDeploymentController) resolveOrgUUID(ctx context.Context, orgName string) (string, error) {
	org, err := c.orgRepo.GetOrganizationByHandle(orgName)
	if err != nil {
		return "", utils.ErrOrganizationNotFound
	}
	return org.UUID.String(), nil
}

func (c *llmDeploymentController) DeployLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("DeployLLMProvider: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	if providerID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}

	var req models.DeployAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("DeployLLMProvider: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate required fields
	if req.Name == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Base == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "base is required (use 'current' or a deploymentId)")
		return
	}
	if req.GatewayID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId is required")
		return
	}

	deployment, err := c.deploymentService.DeployLLMProvider(providerID, &req, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrBaseDeploymentNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Base deployment not found")
			return
		case errors.Is(err, utils.ErrDeploymentNameRequired):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment name is required")
			return
		case errors.Is(err, utils.ErrDeploymentBaseRequired):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Base is required (use 'current' or a deploymentId)")
			return
		case errors.Is(err, utils.ErrDeploymentGatewayIDRequired):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Gateway ID is required")
			return
		case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("DeployLLMProvider: failed to deploy", "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to deploy LLM provider")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, deployment)
}

func (c *llmDeploymentController) UndeployLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("UndeployLLMProviderDeployment: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Parse query parameters
	deploymentID := r.URL.Query().Get("deploymentId")
	gatewayID := r.URL.Query().Get("gatewayId")

	if providerID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	if err := c.deploymentService.UndeployLLMProviderDeployment(providerID, orgID, gatewayID, deploymentID); err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotActive):
			utils.WriteErrorResponse(w, http.StatusConflict, "No active deployment found for this LLM provider on the gateway")
			return
		case errors.Is(err, utils.ErrGatewayIDMismatch):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("UndeployLLMProviderDeployment: failed to undeploy", "providerID", providerID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to undeploy deployment")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusOK, map[string]string{"message": "LLM provider undeployed successfully"})
}

func (c *llmDeploymentController) RestoreLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("RestoreLLMProviderDeployment: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Parse query parameters
	deploymentID := r.URL.Query().Get("deploymentId")
	gatewayID := r.URL.Query().Get("gatewayId")

	if providerID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	deployment, err := c.deploymentService.RestoreLLMProviderDeployment(providerID, deploymentID, gatewayID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentAlreadyDeployed):
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot restore currently deployed deployment")
			return
		case errors.Is(err, utils.ErrGatewayIDMismatch):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("RestoreLLMProviderDeployment: failed to restore", "providerID", providerID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to restore deployment")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusOK, deployment)
}

func (c *llmDeploymentController) DeleteLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")
	deploymentID := r.PathValue("deploymentId")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("DeleteLLMProviderDeployment: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	if providerID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	err = c.deploymentService.DeleteLLMProviderDeployment(providerID, deploymentID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrDeploymentIsDeployed):
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot delete an active deployment - undeploy it first")
			return
		default:
			log.Error("DeleteLLMProviderDeployment: failed to delete", "providerID", providerID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete deployment")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

func (c *llmDeploymentController) GetLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")
	deploymentID := r.PathValue("deploymentId")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("GetLLMProviderDeployment: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	if providerID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	deployment, err := c.deploymentService.GetLLMProviderDeployment(providerID, deploymentID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		default:
			log.Error("GetLLMProviderDeployment: failed to get deployment", "providerID", providerID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployment")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusOK, deployment)
}

func (c *llmDeploymentController) GetLLMProviderDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("GetLLMProviderDeployments: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	if providerID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}

	// Parse optional query parameters
	gatewayID := r.URL.Query().Get("gatewayId")

	deployments, err := c.deploymentService.GetLLMProviderDeployments(providerID, orgID, gatewayID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidDeploymentStatus):
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid deployment status")
			return
		default:
			log.Error("GetLLMProviderDeployments: failed to get deployments", "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployments")
			return
		}
	}

	utils.WriteSuccessResponse(w, http.StatusOK, deployments)
}
