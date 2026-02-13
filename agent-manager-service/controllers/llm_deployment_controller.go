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
	"fmt"
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
	org, err := c.orgRepo.GetOrganizationByName(orgName)
	if err != nil {
		return "", fmt.Errorf("failed to resolve organization: %w", err)
	}
	if org == nil {
		return "", utils.ErrOrganizationNotFound
	}
	return org.UUID.String(), nil
}

func (c *llmDeploymentController) DeployLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	log.Info("DeployLLMProvider: starting", "orgName", orgName, "providerID", providerID)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Warn("DeployLLMProvider: organization not found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		} else {
			log.Error("DeployLLMProvider: failed to resolve organization", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	log.Info("DeployLLMProvider: organization resolved", "orgName", orgName, "orgID", orgID)

	if providerID == "" {
		log.Error("DeployLLMProvider: provider ID is empty", "orgName", orgName)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}

	var req models.DeployAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("DeployLLMProvider: failed to decode request", "orgName", orgName, "providerID", providerID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Info("DeployLLMProvider: request decoded", "orgName", orgName, "providerID", providerID,
		"deploymentName", req.Name, "base", req.Base, "gatewayID", req.GatewayID)

	// Validate required fields
	if req.Name == "" {
		log.Error("DeployLLMProvider: deployment name is required", "orgName", orgName, "providerID", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Base == "" {
		log.Error("DeployLLMProvider: base is required", "orgName", orgName, "providerID", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "base is required (use 'current' or a deploymentId)")
		return
	}
	if req.GatewayID == "" {
		log.Error("DeployLLMProvider: gateway ID is required", "orgName", orgName, "providerID", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId is required")
		return
	}

	log.Info("DeployLLMProvider: calling service layer", "orgName", orgName, "providerID", providerID,
		"deploymentName", req.Name, "gatewayID", req.GatewayID)

	deployment, err := c.deploymentService.DeployLLMProvider(providerID, &req, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("DeployLLMProvider: provider not found", "orgName", orgName, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("DeployLLMProvider: gateway not found", "orgName", orgName, "providerID", providerID, "gatewayID", req.GatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrBaseDeploymentNotFound):
			log.Warn("DeployLLMProvider: base deployment not found", "orgName", orgName, "providerID", providerID, "base", req.Base)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Base deployment not found")
			return
		case errors.Is(err, utils.ErrDeploymentNameRequired):
			log.Error("DeployLLMProvider: deployment name required", "orgName", orgName, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment name is required")
			return
		case errors.Is(err, utils.ErrDeploymentBaseRequired):
			log.Error("DeployLLMProvider: base required", "orgName", orgName, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Base is required (use 'current' or a deploymentId)")
			return
		case errors.Is(err, utils.ErrDeploymentGatewayIDRequired):
			log.Error("DeployLLMProvider: gateway ID required", "orgName", orgName, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Gateway ID is required")
			return
		case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
			log.Error("DeployLLMProvider: template not found", "orgName", orgName, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("DeployLLMProvider: invalid input", "orgName", orgName, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("DeployLLMProvider: failed to deploy", "orgName", orgName, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to deploy LLM provider")
			return
		}
	}

	log.Info("DeployLLMProvider: deployment created successfully", "orgName", orgName, "providerID", providerID,
		"deploymentID", deployment.DeploymentID, "gatewayID", req.GatewayID)

	utils.WriteSuccessResponse(w, http.StatusCreated, deployment)
}

func (c *llmDeploymentController) UndeployLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	// Parse query parameters
	deploymentID := r.URL.Query().Get("deploymentId")
	gatewayID := r.URL.Query().Get("gatewayId")

	log.Info("UndeployLLMProviderDeployment: starting", "orgName", orgName, "providerID", providerID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Warn("UndeployLLMProviderDeployment: organization not found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		} else {
			log.Error("UndeployLLMProviderDeployment: failed to resolve organization", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	log.Info("UndeployLLMProviderDeployment: organization resolved", "orgName", orgName, "orgID", orgID)

	if providerID == "" {
		log.Error("UndeployLLMProviderDeployment: provider ID is empty", "orgName", orgName)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("UndeployLLMProviderDeployment: deployment ID is empty", "orgName", orgName, "providerID", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		log.Error("UndeployLLMProviderDeployment: gateway ID is empty", "orgName", orgName, "providerID", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	log.Info("UndeployLLMProviderDeployment: calling service layer", "orgName", orgName, "providerID", providerID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	_, err = c.deploymentService.UndeployLLMProviderDeployment(providerID, deploymentID, gatewayID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("UndeployLLMProviderDeployment: provider not found", "orgName", orgName, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("UndeployLLMProviderDeployment: deployment not found", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("UndeployLLMProviderDeployment: gateway not found", "orgName", orgName, "gatewayID", gatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotActive):
			log.Warn("UndeployLLMProviderDeployment: deployment not active", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "No active deployment found for this LLM provider on the gateway")
			return
		case errors.Is(err, utils.ErrGatewayIDMismatch):
			log.Error("UndeployLLMProviderDeployment: gateway ID mismatch", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID, "gatewayID", gatewayID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("UndeployLLMProviderDeployment: failed to undeploy", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to undeploy deployment")
			return
		}
	}

	log.Info("UndeployLLMProviderDeployment: undeployed successfully", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)

	resp := map[string]string{"message": "LLM provider undeployed successfully"}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmDeploymentController) RestoreLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	// Parse query parameters
	deploymentID := r.URL.Query().Get("deploymentId")
	gatewayID := r.URL.Query().Get("gatewayId")

	log.Info("RestoreLLMProviderDeployment: starting", "orgName", orgName, "providerID", providerID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Warn("RestoreLLMProviderDeployment: organization not found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		} else {
			log.Error("RestoreLLMProviderDeployment: failed to resolve organization", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	log.Info("RestoreLLMProviderDeployment: organization resolved", "orgName", orgName, "orgID", orgID)

	if providerID == "" {
		log.Error("RestoreLLMProviderDeployment: provider ID is empty", "orgName", orgName)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("RestoreLLMProviderDeployment: deployment ID is empty", "orgName", orgName, "providerID", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		log.Error("RestoreLLMProviderDeployment: gateway ID is empty", "orgName", orgName, "providerID", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	log.Info("RestoreLLMProviderDeployment: calling service layer", "orgName", orgName, "providerID", providerID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	deployment, err := c.deploymentService.RestoreLLMProviderDeployment(providerID, deploymentID, gatewayID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("RestoreLLMProviderDeployment: provider not found", "orgName", orgName, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("RestoreLLMProviderDeployment: deployment not found", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("RestoreLLMProviderDeployment: gateway not found", "orgName", orgName, "gatewayID", gatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentAlreadyDeployed):
			log.Warn("RestoreLLMProviderDeployment: deployment already deployed", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot restore currently deployed deployment")
			return
		case errors.Is(err, utils.ErrGatewayIDMismatch):
			log.Error("RestoreLLMProviderDeployment: gateway ID mismatch", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID, "gatewayID", gatewayID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("RestoreLLMProviderDeployment: failed to restore", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to restore deployment")
			return
		}
	}

	log.Info("RestoreLLMProviderDeployment: restored successfully", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusOK, deployment)
}

func (c *llmDeploymentController) DeleteLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")
	deploymentID := r.PathValue("deploymentId")

	log.Info("DeleteLLMProviderDeployment: starting", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Warn("DeleteLLMProviderDeployment: organization not found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		} else {
			log.Error("DeleteLLMProviderDeployment: failed to resolve organization", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	log.Info("DeleteLLMProviderDeployment: organization resolved", "orgName", orgName, "orgID", orgID)

	if providerID == "" {
		log.Error("DeleteLLMProviderDeployment: provider ID is empty", "orgName", orgName)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("DeleteLLMProviderDeployment: deployment ID is empty", "orgName", orgName, "providerID", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	log.Info("DeleteLLMProviderDeployment: calling service layer", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)

	err = c.deploymentService.DeleteLLMProviderDeployment(providerID, deploymentID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("DeleteLLMProviderDeployment: provider not found", "orgName", orgName, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("DeleteLLMProviderDeployment: deployment not found", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrDeploymentIsDeployed):
			log.Warn("DeleteLLMProviderDeployment: deployment is active", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot delete an active deployment - undeploy it first")
			return
		default:
			log.Error("DeleteLLMProviderDeployment: failed to delete", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete deployment")
			return
		}
	}

	log.Info("DeleteLLMProviderDeployment: deleted successfully", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

func (c *llmDeploymentController) GetLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")
	deploymentID := r.PathValue("deploymentId")

	log.Info("GetLLMProviderDeployment: starting", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Warn("GetLLMProviderDeployment: organization not found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		} else {
			log.Error("GetLLMProviderDeployment: failed to resolve organization", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	log.Info("GetLLMProviderDeployment: organization resolved", "orgName", orgName, "orgID", orgID)

	if providerID == "" {
		log.Error("GetLLMProviderDeployment: provider ID is empty", "orgName", orgName)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("GetLLMProviderDeployment: deployment ID is empty", "orgName", orgName, "providerID", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	log.Info("GetLLMProviderDeployment: calling service layer", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)

	deployment, err := c.deploymentService.GetLLMProviderDeployment(providerID, deploymentID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("GetLLMProviderDeployment: provider not found", "orgName", orgName, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("GetLLMProviderDeployment: deployment not found", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		default:
			log.Error("GetLLMProviderDeployment: failed to get deployment", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployment")
			return
		}
	}

	log.Info("GetLLMProviderDeployment: retrieved successfully", "orgName", orgName, "providerID", providerID, "deploymentID", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusOK, deployment)
}

func (c *llmDeploymentController) GetLLMProviderDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	providerID := r.PathValue("id")

	// Parse optional query parameters
	gatewayID := r.URL.Query().Get("gatewayId")
	status := r.URL.Query().Get("status")

	log.Info("GetLLMProviderDeployments: starting", "orgName", orgName, "providerID", providerID,
		"gatewayID", gatewayID, "status", status)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Warn("GetLLMProviderDeployments: organization not found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		} else {
			log.Error("GetLLMProviderDeployments: failed to resolve organization", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	log.Info("GetLLMProviderDeployments: organization resolved", "orgName", orgName, "orgID", orgID)

	if providerID == "" {
		log.Error("GetLLMProviderDeployments: provider ID is empty", "orgName", orgName)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}

	var gatewayIDPtr *string
	if gatewayID != "" {
		gatewayIDPtr = &gatewayID
	}
	var statusPtr *string
	if status != "" {
		statusPtr = &status
	}

	log.Info("GetLLMProviderDeployments: calling service layer", "orgName", orgName, "providerID", providerID,
		"hasGatewayFilter", gatewayIDPtr != nil, "hasStatusFilter", statusPtr != nil)

	deployments, err := c.deploymentService.GetLLMProviderDeployments(providerID, orgID, gatewayIDPtr, statusPtr)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("GetLLMProviderDeployments: provider not found", "orgName", orgName, "providerID", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidDeploymentStatus):
			log.Error("GetLLMProviderDeployments: invalid status", "orgName", orgName, "providerID", providerID, "status", status)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid deployment status")
			return
		default:
			log.Error("GetLLMProviderDeployments: failed to get deployments", "orgName", orgName, "providerID", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployments")
			return
		}
	}

	log.Info("GetLLMProviderDeployments: retrieved successfully", "orgName", orgName, "providerID", providerID, "count", len(deployments))

	utils.WriteSuccessResponse(w, http.StatusOK, deployments)
}
