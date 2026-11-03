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

// LLMProxyDeploymentController defines interface for LLM proxy deployment HTTP handlers
type LLMProxyDeploymentController interface {
	DeployLLMProxy(w http.ResponseWriter, r *http.Request)
	UndeployLLMProxyDeployment(w http.ResponseWriter, r *http.Request)
	RestoreLLMProxyDeployment(w http.ResponseWriter, r *http.Request)
	DeleteLLMProxyDeployment(w http.ResponseWriter, r *http.Request)
	GetLLMProxyDeployment(w http.ResponseWriter, r *http.Request)
	GetLLMProxyDeployments(w http.ResponseWriter, r *http.Request)
}

type llmProxyDeploymentController struct {
	deploymentService *services.LLMProxyDeploymentService
}

// NewLLMProxyDeploymentController creates a new LLM proxy deployment controller
func NewLLMProxyDeploymentController(
	deploymentService *services.LLMProxyDeploymentService,
) LLMProxyDeploymentController {
	return &llmProxyDeploymentController{
		deploymentService: deploymentService,
	}
}

func (c *llmProxyDeploymentController) DeployLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")

	log.Info("DeployLLMProxy: starting", "ouID", ouID, "proxyID", proxyID)

	log.Info("DeployLLMProxy: organization resolved", "ouID", ouID)

	if proxyID == "" {
		log.Error("DeployLLMProxy: proxy ID is empty", "ouID", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}

	var req models.DeployAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("DeployLLMProxy: failed to decode request", "ouID", ouID, "proxyID", proxyID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Info("DeployLLMProxy: request decoded", "ouID", ouID, "proxyID", proxyID,
		"base", req.Base, "gatewayID", req.GatewayID)

	if req.GatewayID == "" {
		log.Error("DeployLLMProxy: gateway ID is required", "ouID", ouID, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId is required")
		return
	}

	log.Info("DeployLLMProxy: calling service layer", "ouID", ouID, "proxyID", proxyID,
		"deploymentName", req.Name, "gatewayID", req.GatewayID)

	deployment, err := c.deploymentService.DeployLLMProxy(proxyID, &req, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("DeployLLMProxy: proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("DeployLLMProxy: gateway not found", "ouID", ouID, "proxyID", proxyID, "gatewayID", req.GatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentBaseNotCurrent):
			log.Warn("DeployLLMProxy: base is not current", "ouID", ouID, "base", req.Base)
			utils.WriteErrorResponse(w, http.StatusBadRequest, utils.ErrDeploymentBaseNotCurrent.Error())
			return
		case errors.Is(err, utils.ErrDeploymentGatewayIDRequired):
			log.Error("DeployLLMProxy: gateway ID required", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Gateway ID is required")
			return
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Error("DeployLLMProxy: referenced provider not found", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("DeployLLMProxy: invalid input", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("DeployLLMProxy: failed to deploy", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to deploy LLM proxy")
			return
		}
	}

	log.Info("DeployLLMProxy: deployment created successfully", "ouID", ouID, "proxyID", proxyID,
		"deploymentID", deployment.DeploymentID, "gatewayID", req.GatewayID)

	utils.WriteSuccessResponse(w, http.StatusCreated, utils.ConvertModelToSpecLLMDeploymentResponse(deployment))
}

func (c *llmProxyDeploymentController) UndeployLLMProxyDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")

	// Parse query parameters
	deploymentID := r.URL.Query().Get("deploymentId")
	gatewayID := r.URL.Query().Get("gatewayId")

	log.Info("UndeployLLMProxyDeployment: starting", "ouID", ouID, "proxyID", proxyID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	log.Info("UndeployLLMProxyDeployment: organization resolved", "ouID", ouID)

	if proxyID == "" {
		log.Error("UndeployLLMProxyDeployment: proxy ID is empty", "ouID", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("UndeployLLMProxyDeployment: deployment ID is empty", "ouID", ouID, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		log.Error("UndeployLLMProxyDeployment: gateway ID is empty", "ouID", ouID, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	log.Info("UndeployLLMProxyDeployment: calling service layer", "ouID", ouID, "proxyID", proxyID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	_, err := c.deploymentService.UndeployLLMProxyDeployment(r.Context(), proxyID, deploymentID, gatewayID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("UndeployLLMProxyDeployment: proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("UndeployLLMProxyDeployment: deployment not found", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("UndeployLLMProxyDeployment: gateway not found", "ouID", ouID, "gatewayID", gatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotActive):
			log.Warn("UndeployLLMProxyDeployment: deployment not active", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "The deployment isn't the gateway's current deployment, or it is already undeployed or being undeployed")
			return
		case errors.Is(err, utils.ErrGatewayIDMismatch):
			log.Error("UndeployLLMProxyDeployment: gateway ID mismatch", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID, "gatewayID", gatewayID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("UndeployLLMProxyDeployment: failed to undeploy", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to undeploy deployment")
			return
		}
	}

	log.Info("UndeployLLMProxyDeployment: undeployed successfully", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)

	resp := map[string]string{"message": "LLM proxy undeployed successfully"}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmProxyDeploymentController) RestoreLLMProxyDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")

	// Parse query parameters
	deploymentID := r.URL.Query().Get("deploymentId")
	gatewayID := r.URL.Query().Get("gatewayId")

	log.Info("RestoreLLMProxyDeployment: starting", "ouID", ouID, "proxyID", proxyID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	log.Info("RestoreLLMProxyDeployment: organization resolved", "ouID", ouID)

	if proxyID == "" {
		log.Error("RestoreLLMProxyDeployment: proxy ID is empty", "ouID", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("RestoreLLMProxyDeployment: deployment ID is empty", "ouID", ouID, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		log.Error("RestoreLLMProxyDeployment: gateway ID is empty", "ouID", ouID, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	log.Info("RestoreLLMProxyDeployment: calling service layer", "ouID", ouID, "proxyID", proxyID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	deployment, err := c.deploymentService.RestoreLLMProxyDeployment(proxyID, deploymentID, gatewayID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("RestoreLLMProxyDeployment: proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("RestoreLLMProxyDeployment: deployment not found", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("RestoreLLMProxyDeployment: gateway not found", "ouID", ouID, "gatewayID", gatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentAlreadyDeployed):
			log.Warn("RestoreLLMProxyDeployment: deployment already deployed", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot restore the deployment the gateway is already deploying or serving")
			return
		case errors.Is(err, utils.ErrGatewayIDMismatch):
			log.Error("RestoreLLMProxyDeployment: gateway ID mismatch", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID, "gatewayID", gatewayID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("RestoreLLMProxyDeployment: failed to restore", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to restore deployment")
			return
		}
	}

	log.Info("RestoreLLMProxyDeployment: restored successfully", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusOK, utils.ConvertModelToSpecLLMDeploymentResponse(deployment))
}

func (c *llmProxyDeploymentController) DeleteLLMProxyDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")
	deploymentID := r.PathValue("deploymentId")

	log.Info("DeleteLLMProxyDeployment: starting", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)

	log.Info("DeleteLLMProxyDeployment: organization resolved", "ouID", ouID)

	if proxyID == "" {
		log.Error("DeleteLLMProxyDeployment: proxy ID is empty", "ouID", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("DeleteLLMProxyDeployment: deployment ID is empty", "ouID", ouID, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	log.Info("DeleteLLMProxyDeployment: calling service layer", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)

	err := c.deploymentService.DeleteLLMProxyDeployment(proxyID, deploymentID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("DeleteLLMProxyDeployment: proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("DeleteLLMProxyDeployment: deployment not found", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrDeploymentIsDeployed):
			log.Warn("DeleteLLMProxyDeployment: deployment is active", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot delete the gateway's current deployment - undeploy it and wait until it is UNDEPLOYED")
			return
		default:
			log.Error("DeleteLLMProxyDeployment: failed to delete", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete deployment")
			return
		}
	}

	log.Info("DeleteLLMProxyDeployment: deleted successfully", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

func (c *llmProxyDeploymentController) GetLLMProxyDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")
	deploymentID := r.PathValue("deploymentId")

	log.Info("GetLLMProxyDeployment: starting", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)

	log.Info("GetLLMProxyDeployment: organization resolved", "ouID", ouID)

	if proxyID == "" {
		log.Error("GetLLMProxyDeployment: proxy ID is empty", "ouID", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("GetLLMProxyDeployment: deployment ID is empty", "ouID", ouID, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	log.Info("GetLLMProxyDeployment: calling service layer", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)

	deployment, err := c.deploymentService.GetLLMProxyDeployment(proxyID, deploymentID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("GetLLMProxyDeployment: proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("GetLLMProxyDeployment: deployment not found", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		default:
			log.Error("GetLLMProxyDeployment: failed to get deployment", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployment")
			return
		}
	}

	log.Info("GetLLMProxyDeployment: retrieved successfully", "ouID", ouID, "proxyID", proxyID, "deploymentID", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusOK, utils.ConvertModelToSpecLLMDeploymentResponse(deployment))
}

func (c *llmProxyDeploymentController) GetLLMProxyDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")

	// Parse optional query parameters
	gatewayID := r.URL.Query().Get("gatewayId")
	status := r.URL.Query().Get("status")

	log.Info("GetLLMProxyDeployments: starting", "ouID", ouID, "proxyID", proxyID,
		"gatewayID", gatewayID, "status", status)

	log.Info("GetLLMProxyDeployments: organization resolved", "ouID", ouID)

	if proxyID == "" {
		log.Error("GetLLMProxyDeployments: proxy ID is empty", "ouID", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
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

	log.Info("GetLLMProxyDeployments: calling service layer", "ouID", ouID, "proxyID", proxyID,
		"hasGatewayFilter", gatewayIDPtr != nil, "hasStatusFilter", statusPtr != nil)

	deployments, err := c.deploymentService.GetLLMProxyDeployments(proxyID, ouID, gatewayIDPtr, statusPtr)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("GetLLMProxyDeployments: proxy not found", "ouID", ouID, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrInvalidDeploymentStatus):
			log.Error("GetLLMProxyDeployments: invalid status", "ouID", ouID, "proxyID", proxyID, "status", status)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid deployment status")
			return
		default:
			log.Error("GetLLMProxyDeployments: failed to get deployments", "ouID", ouID, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployments")
			return
		}
	}

	log.Info("GetLLMProxyDeployments: retrieved successfully", "ouID", ouID, "proxyID", proxyID, "count", len(deployments))

	utils.WriteSuccessResponse(w, http.StatusOK, utils.ConvertModelToSpecLLMDeploymentResponses(deployments))
}
