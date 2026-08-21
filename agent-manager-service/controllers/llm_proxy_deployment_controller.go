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

	log.Info("DeployLLMProxy: starting", "ou_id", ouID, "proxy_id", proxyID)

	log.Info("DeployLLMProxy: organization resolved", "ou_id", ouID)

	if proxyID == "" {
		log.Warn("DeployLLMProxy: proxy ID is empty", "ou_id", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}

	var req models.DeployAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Warn("DeployLLMProxy: failed to decode request", "ou_id", ouID, "proxy_id", proxyID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Info("DeployLLMProxy: request decoded", "ou_id", ouID, "proxy_id", proxyID,
		"deployment_name", req.Name, "base", req.Base, "gateway_id", req.GatewayID)

	// Validate required fields
	if req.Name == "" {
		log.Warn("DeployLLMProxy: deployment name is required", "ou_id", ouID, "proxy_id", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Base == "" {
		log.Warn("DeployLLMProxy: base is required", "ou_id", ouID, "proxy_id", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "base is required (use 'current' or a deploymentId)")
		return
	}
	if req.GatewayID == "" {
		log.Warn("DeployLLMProxy: gateway ID is required", "ou_id", ouID, "proxy_id", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId is required")
		return
	}

	log.Info("DeployLLMProxy: calling service layer", "ou_id", ouID, "proxy_id", proxyID,
		"deployment_name", req.Name, "gateway_id", req.GatewayID)

	deployment, err := c.deploymentService.DeployLLMProxy(ctx, proxyID, &req, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("DeployLLMProxy: proxy not found", "ou_id", ouID, "proxy_id", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("DeployLLMProxy: gateway not found", "ou_id", ouID, "proxy_id", proxyID, "gateway_id", req.GatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrBaseDeploymentNotFound):
			log.Warn("DeployLLMProxy: base deployment not found", "ou_id", ouID, "proxy_id", proxyID, "base", req.Base)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Base deployment not found")
			return
		case errors.Is(err, utils.ErrDeploymentNameRequired):
			log.Warn("DeployLLMProxy: deployment name required", "ou_id", ouID, "proxy_id", proxyID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment name is required")
			return
		case errors.Is(err, utils.ErrDeploymentBaseRequired):
			log.Warn("DeployLLMProxy: base required", "ou_id", ouID, "proxy_id", proxyID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Base is required (use 'current' or a deploymentId)")
			return
		case errors.Is(err, utils.ErrDeploymentGatewayIDRequired):
			log.Warn("DeployLLMProxy: gateway ID required", "ou_id", ouID, "proxy_id", proxyID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Gateway ID is required")
			return
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Warn("DeployLLMProxy: referenced provider not found", "ou_id", ouID, "proxy_id", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			log.Warn("DeployLLMProxy: invalid input", "ou_id", ouID, "proxy_id", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("DeployLLMProxy: failed to deploy", "ou_id", ouID, "proxy_id", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to deploy LLM proxy")
			return
		}
	}

	log.Info("DeployLLMProxy: deployment created successfully", "ou_id", ouID, "proxy_id", proxyID,
		"deployment_id", deployment.DeploymentID, "gateway_id", req.GatewayID)

	utils.WriteSuccessResponse(w, http.StatusCreated, deployment)
}

func (c *llmProxyDeploymentController) UndeployLLMProxyDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")

	// Parse query parameters
	deploymentID := r.URL.Query().Get("deploymentId")
	gatewayID := r.URL.Query().Get("gatewayId")

	log.Info("UndeployLLMProxyDeployment: starting", "ou_id", ouID, "proxy_id", proxyID,
		"deployment_id", deploymentID, "gateway_id", gatewayID)

	log.Info("UndeployLLMProxyDeployment: organization resolved", "ou_id", ouID)

	if proxyID == "" {
		log.Warn("UndeployLLMProxyDeployment: proxy ID is empty", "ou_id", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}
	if deploymentID == "" {
		log.Warn("UndeployLLMProxyDeployment: deployment ID is empty", "ou_id", ouID, "proxy_id", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		log.Warn("UndeployLLMProxyDeployment: gateway ID is empty", "ou_id", ouID, "proxy_id", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	log.Info("UndeployLLMProxyDeployment: calling service layer", "ou_id", ouID, "proxy_id", proxyID,
		"deployment_id", deploymentID, "gateway_id", gatewayID)

	_, err := c.deploymentService.UndeployLLMProxyDeployment(r.Context(), proxyID, deploymentID, gatewayID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("UndeployLLMProxyDeployment: proxy not found", "ou_id", ouID, "proxy_id", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("UndeployLLMProxyDeployment: deployment not found", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("UndeployLLMProxyDeployment: gateway not found", "ou_id", ouID, "gateway_id", gatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotActive):
			log.Warn("UndeployLLMProxyDeployment: deployment not active", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "No active deployment found for this LLM proxy on the gateway")
			return
		case errors.Is(err, utils.ErrGatewayIDMismatch):
			log.Warn("UndeployLLMProxyDeployment: gateway ID mismatch", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID, "gateway_id", gatewayID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("UndeployLLMProxyDeployment: failed to undeploy", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to undeploy deployment")
			return
		}
	}

	log.Info("UndeployLLMProxyDeployment: undeployed successfully", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)

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

	log.Info("RestoreLLMProxyDeployment: starting", "ou_id", ouID, "proxy_id", proxyID,
		"deployment_id", deploymentID, "gateway_id", gatewayID)

	log.Info("RestoreLLMProxyDeployment: organization resolved", "ou_id", ouID)

	if proxyID == "" {
		log.Warn("RestoreLLMProxyDeployment: proxy ID is empty", "ou_id", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}
	if deploymentID == "" {
		log.Warn("RestoreLLMProxyDeployment: deployment ID is empty", "ou_id", ouID, "proxy_id", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		log.Warn("RestoreLLMProxyDeployment: gateway ID is empty", "ou_id", ouID, "proxy_id", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	log.Info("RestoreLLMProxyDeployment: calling service layer", "ou_id", ouID, "proxy_id", proxyID,
		"deployment_id", deploymentID, "gateway_id", gatewayID)

	deployment, err := c.deploymentService.RestoreLLMProxyDeployment(ctx, proxyID, deploymentID, gatewayID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("RestoreLLMProxyDeployment: proxy not found", "ou_id", ouID, "proxy_id", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("RestoreLLMProxyDeployment: deployment not found", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("RestoreLLMProxyDeployment: gateway not found", "ou_id", ouID, "gateway_id", gatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentAlreadyDeployed):
			log.Warn("RestoreLLMProxyDeployment: deployment already deployed", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot restore currently deployed deployment")
			return
		case errors.Is(err, utils.ErrGatewayIDMismatch):
			log.Warn("RestoreLLMProxyDeployment: gateway ID mismatch", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID, "gateway_id", gatewayID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("RestoreLLMProxyDeployment: failed to restore", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to restore deployment")
			return
		}
	}

	log.Info("RestoreLLMProxyDeployment: restored successfully", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusOK, deployment)
}

func (c *llmProxyDeploymentController) DeleteLLMProxyDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")
	deploymentID := r.PathValue("deploymentId")

	log.Info("DeleteLLMProxyDeployment: starting", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)

	log.Info("DeleteLLMProxyDeployment: organization resolved", "ou_id", ouID)

	if proxyID == "" {
		log.Warn("DeleteLLMProxyDeployment: proxy ID is empty", "ou_id", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}
	if deploymentID == "" {
		log.Warn("DeleteLLMProxyDeployment: deployment ID is empty", "ou_id", ouID, "proxy_id", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	log.Info("DeleteLLMProxyDeployment: calling service layer", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)

	err := c.deploymentService.DeleteLLMProxyDeployment(proxyID, deploymentID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("DeleteLLMProxyDeployment: proxy not found", "ou_id", ouID, "proxy_id", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("DeleteLLMProxyDeployment: deployment not found", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrDeploymentIsDeployed):
			log.Warn("DeleteLLMProxyDeployment: deployment is active", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot delete an active deployment - undeploy it first")
			return
		default:
			log.Error("DeleteLLMProxyDeployment: failed to delete", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete deployment")
			return
		}
	}

	log.Info("DeleteLLMProxyDeployment: deleted successfully", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

func (c *llmProxyDeploymentController) GetLLMProxyDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")
	deploymentID := r.PathValue("deploymentId")

	log.Info("GetLLMProxyDeployment: starting", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)

	log.Info("GetLLMProxyDeployment: organization resolved", "ou_id", ouID)

	if proxyID == "" {
		log.Warn("GetLLMProxyDeployment: proxy ID is empty", "ou_id", ouID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}
	if deploymentID == "" {
		log.Warn("GetLLMProxyDeployment: deployment ID is empty", "ou_id", ouID, "proxy_id", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	log.Info("GetLLMProxyDeployment: calling service layer", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)

	deployment, err := c.deploymentService.GetLLMProxyDeployment(proxyID, deploymentID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("GetLLMProxyDeployment: proxy not found", "ou_id", ouID, "proxy_id", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("GetLLMProxyDeployment: deployment not found", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		default:
			log.Error("GetLLMProxyDeployment: failed to get deployment", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployment")
			return
		}
	}

	log.Info("GetLLMProxyDeployment: retrieved successfully", "ou_id", ouID, "proxy_id", proxyID, "deployment_id", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusOK, deployment)
}

func (c *llmProxyDeploymentController) GetLLMProxyDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	proxyID := r.PathValue("id")

	// Parse optional query parameters
	gatewayID := r.URL.Query().Get("gatewayId")
	status := r.URL.Query().Get("status")

	log.Info("GetLLMProxyDeployments: starting", "ou_id", ouID, "proxy_id", proxyID,
		"gateway_id", gatewayID, "status", status)

	log.Info("GetLLMProxyDeployments: organization resolved", "ou_id", ouID)

	if proxyID == "" {
		log.Warn("GetLLMProxyDeployments: proxy ID is empty", "ou_id", ouID)
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

	log.Info("GetLLMProxyDeployments: calling service layer", "ou_id", ouID, "proxy_id", proxyID,
		"has_gateway_filter", gatewayIDPtr != nil, "has_status_filter", statusPtr != nil)

	deployments, err := c.deploymentService.GetLLMProxyDeployments(proxyID, ouID, gatewayIDPtr, statusPtr)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("GetLLMProxyDeployments: proxy not found", "ou_id", ouID, "proxy_id", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrInvalidDeploymentStatus):
			log.Warn("GetLLMProxyDeployments: invalid status", "ou_id", ouID, "proxy_id", proxyID, "status", status)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid deployment status")
			return
		default:
			log.Error("GetLLMProxyDeployments: failed to get deployments", "ou_id", ouID, "proxy_id", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployments")
			return
		}
	}

	log.Info("GetLLMProxyDeployments: retrieved successfully", "ou_id", ouID, "proxy_id", proxyID, "count", len(deployments))

	utils.WriteSuccessResponse(w, http.StatusOK, deployments)
}
