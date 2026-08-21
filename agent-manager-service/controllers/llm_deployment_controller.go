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
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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
}

// NewLLMDeploymentController creates a new LLM deployment controller
func NewLLMDeploymentController(
	deploymentService *services.LLMProviderDeploymentService,
) LLMDeploymentController {
	return &llmDeploymentController{
		deploymentService: deploymentService,
	}
}

func (c *llmDeploymentController) DeployLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(utils.PathParamProviderId)

	log.Info("DeployLLMProvider: starting", "ou_id", ouID, "provider_id", providerID)

	log.Info("DeployLLMProvider: organization resolved", "ou_id", ouID)

	if providerID == "" {
		log.Warn("DeployLLMProvider: provider ID is empty", "ou_id", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}

	var req models.DeployAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("DeployLLMProvider: failed to decode request", "ou_id", ouID, "provider_id", providerID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Info("DeployLLMProvider: request decoded", "ou_id", ouID, "provider_id", providerID,
		"deployment_name", req.Name, "base", req.Base, "gateway_id", req.GatewayID)

	// Validate required fields
	if req.Name == "" {
		log.Warn("DeployLLMProvider: deployment name is required", "ou_id", ouID, "provider_id", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Base == "" {
		log.Warn("DeployLLMProvider: base is required", "ou_id", ouID, "provider_id", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "base is required (use 'current' or a deploymentId)")
		return
	}
	if req.GatewayID == "" {
		log.Warn("DeployLLMProvider: gateway ID is required", "ou_id", ouID, "provider_id", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId is required")
		return
	}

	log.Info("DeployLLMProvider: calling service layer", "ou_id", ouID, "provider_id", providerID,
		"deployment_name", req.Name, "gateway_id", req.GatewayID)

	deployment, err := c.deploymentService.DeployLLMProvider(ctx, providerID, &req, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("DeployLLMProvider: provider not found", "ou_id", ouID, "provider_id", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("DeployLLMProvider: gateway not found", "ou_id", ouID, "provider_id", providerID, "gateway_id", req.GatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrBaseDeploymentNotFound):
			log.Warn("DeployLLMProvider: base deployment not found", "ou_id", ouID, "provider_id", providerID, "base", req.Base)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Base deployment not found")
			return
		case errors.Is(err, utils.ErrDeploymentNameRequired):
			log.Warn("DeployLLMProvider: deployment name required", "ou_id", ouID, "provider_id", providerID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment name is required")
			return
		case errors.Is(err, utils.ErrDeploymentBaseRequired):
			log.Warn("DeployLLMProvider: base required", "ou_id", ouID, "provider_id", providerID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Base is required (use 'current' or a deploymentId)")
			return
		case errors.Is(err, utils.ErrDeploymentGatewayIDRequired):
			log.Warn("DeployLLMProvider: gateway ID required", "ou_id", ouID, "provider_id", providerID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Gateway ID is required")
			return
		case errors.Is(err, utils.ErrLLMProviderTemplateNotFound):
			log.Warn("DeployLLMProvider: template not found", "ou_id", ouID, "provider_id", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			log.Warn("DeployLLMProvider: invalid input", "ou_id", ouID, "provider_id", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("DeployLLMProvider: failed to deploy", "ou_id", ouID, "provider_id", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to deploy LLM provider")
			return
		}
	}

	log.Info("DeployLLMProvider: deployment created successfully", "ou_id", ouID, "provider_id", providerID,
		"deployment_id", deployment.DeploymentID, "gateway_id", req.GatewayID)

	utils.WriteSuccessResponse(w, http.StatusCreated, deployment)
}

func (c *llmDeploymentController) UndeployLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(utils.PathParamProviderId)

	// Parse query parameters
	deploymentID := r.URL.Query().Get(utils.PathParamDeploymentId)
	gatewayID := r.URL.Query().Get(utils.PathParamGatewayId)

	log.Info("UndeployLLMProviderDeployment: starting", "ou_id", ouID, "provider_id", providerID,
		"deployment_id", deploymentID, "gateway_id", gatewayID)

	log.Info("UndeployLLMProviderDeployment: organization resolved", "ou_id", ouID)

	if providerID == "" {
		log.Warn("UndeployLLMProviderDeployment: provider ID is empty", "ou_id", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		log.Warn("UndeployLLMProviderDeployment: deployment ID is empty", "ou_id", ouID, "provider_id", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		log.Warn("UndeployLLMProviderDeployment: gateway ID is empty", "ou_id", ouID, "provider_id", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	log.Info("UndeployLLMProviderDeployment: calling service layer", "ou_id", ouID, "provider_id", providerID,
		"deployment_id", deploymentID, "gateway_id", gatewayID)

	_, err := c.deploymentService.UndeployLLMProviderDeployment(ctx, providerID, deploymentID, gatewayID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("UndeployLLMProviderDeployment: provider not found", "ou_id", ouID, "provider_id", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("UndeployLLMProviderDeployment: deployment not found", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("UndeployLLMProviderDeployment: gateway not found", "ou_id", ouID, "gateway_id", gatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotActive):
			log.Warn("UndeployLLMProviderDeployment: deployment not active", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "No active deployment found for this LLM provider on the gateway")
			return
		case errors.Is(err, utils.ErrGatewayIDMismatch):
			log.Warn("UndeployLLMProviderDeployment: gateway ID mismatch", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID, "gateway_id", gatewayID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("UndeployLLMProviderDeployment: failed to undeploy", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to undeploy deployment")
			return
		}
	}

	log.Info("UndeployLLMProviderDeployment: undeployed successfully", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)

	resp := map[string]string{"message": "LLM provider undeployed successfully"}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmDeploymentController) RestoreLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(utils.PathParamProviderId)

	// Parse query parameters
	deploymentID := r.URL.Query().Get(utils.PathParamDeploymentId)
	gatewayID := r.URL.Query().Get(utils.PathParamGatewayId)

	log.Info("RestoreLLMProviderDeployment: starting", "ou_id", ouID, "provider_id", providerID,
		"deployment_id", deploymentID, "gateway_id", gatewayID)

	log.Info("RestoreLLMProviderDeployment: organization resolved", "ou_id", ouID)

	if providerID == "" {
		log.Warn("RestoreLLMProviderDeployment: provider ID is empty", "ou_id", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		log.Warn("RestoreLLMProviderDeployment: deployment ID is empty", "ou_id", ouID, "provider_id", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		log.Warn("RestoreLLMProviderDeployment: gateway ID is empty", "ou_id", ouID, "provider_id", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	log.Info("RestoreLLMProviderDeployment: calling service layer", "ou_id", ouID, "provider_id", providerID,
		"deployment_id", deploymentID, "gateway_id", gatewayID)

	deployment, err := c.deploymentService.RestoreLLMProviderDeployment(ctx, providerID, deploymentID, gatewayID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("RestoreLLMProviderDeployment: provider not found", "ou_id", ouID, "provider_id", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("RestoreLLMProviderDeployment: deployment not found", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("RestoreLLMProviderDeployment: gateway not found", "ou_id", ouID, "gateway_id", gatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentAlreadyDeployed):
			log.Warn("RestoreLLMProviderDeployment: deployment already deployed", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot restore currently deployed deployment")
			return
		case errors.Is(err, utils.ErrGatewayIDMismatch):
			log.Warn("RestoreLLMProviderDeployment: gateway ID mismatch", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID, "gateway_id", gatewayID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("RestoreLLMProviderDeployment: failed to restore", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to restore deployment")
			return
		}
	}

	log.Info("RestoreLLMProviderDeployment: restored successfully", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusOK, deployment)
}

func (c *llmDeploymentController) DeleteLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(utils.PathParamProviderId)
	deploymentID := r.PathValue(utils.PathParamDeploymentId)

	log.Info("DeleteLLMProviderDeployment: starting", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)

	log.Info("DeleteLLMProviderDeployment: organization resolved", "ou_id", ouID)

	if providerID == "" {
		log.Warn("DeleteLLMProviderDeployment: provider ID is empty", "ou_id", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		log.Warn("DeleteLLMProviderDeployment: deployment ID is empty", "ou_id", ouID, "provider_id", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	log.Info("DeleteLLMProviderDeployment: calling service layer", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)

	err := c.deploymentService.DeleteLLMProviderDeployment(providerID, deploymentID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("DeleteLLMProviderDeployment: provider not found", "ou_id", ouID, "provider_id", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("DeleteLLMProviderDeployment: deployment not found", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrDeploymentIsDeployed):
			log.Warn("DeleteLLMProviderDeployment: deployment is active", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot delete an active deployment - undeploy it first")
			return
		default:
			log.Error("DeleteLLMProviderDeployment: failed to delete", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete deployment")
			return
		}
	}

	log.Info("DeleteLLMProviderDeployment: deleted successfully", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

func (c *llmDeploymentController) GetLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(utils.PathParamProviderId)
	deploymentID := r.PathValue(utils.PathParamDeploymentId)

	log.Info("GetLLMProviderDeployment: starting", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)

	log.Info("GetLLMProviderDeployment: organization resolved", "ou_id", ouID)

	if providerID == "" {
		log.Warn("GetLLMProviderDeployment: provider ID is empty", "ou_id", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		log.Warn("GetLLMProviderDeployment: deployment ID is empty", "ou_id", ouID, "provider_id", providerID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	log.Info("GetLLMProviderDeployment: calling service layer", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)

	deployment, err := c.deploymentService.GetLLMProviderDeployment(providerID, deploymentID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("GetLLMProviderDeployment: provider not found", "ou_id", ouID, "provider_id", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("GetLLMProviderDeployment: deployment not found", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		default:
			log.Error("GetLLMProviderDeployment: failed to get deployment", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployment")
			return
		}
	}

	log.Info("GetLLMProviderDeployment: retrieved successfully", "ou_id", ouID, "provider_id", providerID, "deployment_id", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusOK, deployment)
}

func (c *llmDeploymentController) GetLLMProviderDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(utils.PathParamProviderId)

	// Parse optional query parameters
	gatewayID := r.URL.Query().Get(utils.PathParamGatewayId)
	status := r.URL.Query().Get("status")

	log.Info("GetLLMProviderDeployments: starting", "ou_id", ouID, "provider_id", providerID,
		"gateway_id", gatewayID, "status", status)

	log.Info("GetLLMProviderDeployments: organization resolved", "ou_id", ouID)

	if providerID == "" {
		log.Warn("GetLLMProviderDeployments: provider ID is empty", "ou_id", ouID)
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

	log.Info("GetLLMProviderDeployments: calling service layer", "ou_id", ouID, "provider_id", providerID,
		"has_gateway_filter", gatewayIDPtr != nil, "has_status_filter", statusPtr != nil)

	deployments, err := c.deploymentService.GetLLMProviderDeployments(providerID, ouID, gatewayIDPtr, statusPtr)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("GetLLMProviderDeployments: provider not found", "ou_id", ouID, "provider_id", providerID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidDeploymentStatus):
			log.Warn("GetLLMProviderDeployments: invalid status", "ou_id", ouID, "provider_id", providerID, "status", status)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid deployment status")
			return
		default:
			log.Error("GetLLMProviderDeployments: failed to get deployments", "ou_id", ouID, "provider_id", providerID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployments")
			return
		}
	}

	log.Info("GetLLMProviderDeployments: retrieved successfully", "ou_id", ouID, "provider_id", providerID, "count", len(deployments))

	utils.WriteSuccessResponse(w, http.StatusOK, deployments)
}
