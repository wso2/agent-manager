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

package gwctl

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconv"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
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
	deploymentService *gwsvc.LLMProviderDeploymentService
}

// NewLLMDeploymentController creates a new LLM deployment controller
func NewLLMDeploymentController(
	deploymentService *gwsvc.LLMProviderDeploymentService,
) LLMDeploymentController {
	return &llmDeploymentController{
		deploymentService: deploymentService,
	}
}

func (c *llmDeploymentController) DeployLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(gwutil.PathParamProviderId)

	log.Info("DeployLLMProvider: starting", "ouID", ouID, "providerID", providerID)

	log.Info("DeployLLMProvider: organization resolved", "ouID", ouID)

	if providerID == "" {
		log.Error("DeployLLMProvider: provider ID is empty", "ouID", ouID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}

	var req gwmodels.DeployAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("DeployLLMProvider: failed to decode request", "ouID", ouID, "providerID", providerID, "error", err)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Info("DeployLLMProvider: request decoded", "ouID", ouID, "providerID", providerID,
		"base", req.Base, "gatewayID", req.GatewayID)

	if req.GatewayID == "" {
		log.Error("DeployLLMProvider: gateway ID is required", "ouID", ouID, "providerID", providerID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId is required")
		return
	}

	log.Info("DeployLLMProvider: calling service layer", "ouID", ouID, "providerID", providerID,
		"gatewayID", req.GatewayID)

	deployment, err := c.deploymentService.DeployLLMProvider(providerID, &req, ouID)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrContextInUse):
			log.Warn("DeployLLMProvider: context in use on the gateway", "ouID", ouID, "error", err)
			httpx.WriteErrorResponseWithReason(w, http.StatusConflict, gwerr.ErrContextInUse.Error(), err.Error(),
				gwerr.ErrCodeContextInUse)
			return
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			log.Warn("DeployLLMProvider: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, gwerr.ErrGatewayNotFound):
			log.Warn("DeployLLMProvider: gateway not found", "ouID", ouID, "providerID", providerID, "gatewayID", req.GatewayID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, gwerr.ErrDeploymentBaseNotCurrent):
			log.Warn("DeployLLMProvider: base is not current", "ouID", ouID, "providerID", providerID, "base", req.Base)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, gwerr.ErrDeploymentBaseNotCurrent.Error())
			return
		case errors.Is(err, gwerr.ErrDeploymentGatewayIDRequired):
			log.Error("DeployLLMProvider: gateway ID required", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Gateway ID is required")
			return
		case errors.Is(err, gwerr.ErrLLMProviderTemplateNotFound):
			log.Error("DeployLLMProvider: template not found", "ouID", ouID, "providerID", providerID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Referenced template not found")
			return
		case errors.Is(err, httpx.ErrInvalidInput):
			log.Error("DeployLLMProvider: invalid input", "ouID", ouID, "providerID", providerID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("DeployLLMProvider: failed to deploy", "ouID", ouID, "providerID", providerID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to deploy LLM provider")
			return
		}
	}

	log.Info("DeployLLMProvider: deployment created successfully", "ouID", ouID, "providerID", providerID,
		"deploymentID", deployment.DeploymentID, "gatewayID", req.GatewayID)

	httpx.WriteSuccessResponse(w, http.StatusCreated, gwconv.ConvertModelToSpecLLMDeploymentResponse(deployment))
}

func (c *llmDeploymentController) UndeployLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(gwutil.PathParamProviderId)

	// Parse query parameters
	deploymentID := r.URL.Query().Get(gwutil.PathParamDeploymentId)
	gatewayID := r.URL.Query().Get(gwutil.PathParamGatewayId)

	log.Info("UndeployLLMProviderDeployment: starting", "ouID", ouID, "providerID", providerID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	log.Info("UndeployLLMProviderDeployment: organization resolved", "ouID", ouID)

	if providerID == "" {
		log.Error("UndeployLLMProviderDeployment: provider ID is empty", "ouID", ouID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("UndeployLLMProviderDeployment: deployment ID is empty", "ouID", ouID, "providerID", providerID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		log.Error("UndeployLLMProviderDeployment: gateway ID is empty", "ouID", ouID, "providerID", providerID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	log.Info("UndeployLLMProviderDeployment: calling service layer", "ouID", ouID, "providerID", providerID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	_, err := c.deploymentService.UndeployLLMProviderDeployment(providerID, deploymentID, gatewayID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			log.Warn("UndeployLLMProviderDeployment: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, gwerr.ErrDeploymentNotFound):
			log.Warn("UndeployLLMProviderDeployment: deployment not found", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, gwerr.ErrGatewayNotFound):
			log.Warn("UndeployLLMProviderDeployment: gateway not found", "ouID", ouID, "gatewayID", gatewayID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, gwerr.ErrDeploymentNotActive):
			log.Warn("UndeployLLMProviderDeployment: deployment not active", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)
			httpx.WriteErrorResponse(w, http.StatusConflict, "The deployment isn't the gateway's current deployment, or it is already undeployed or being undeployed")
			return
		case errors.Is(err, gwerr.ErrGatewayIDMismatch):
			log.Error("UndeployLLMProviderDeployment: gateway ID mismatch", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID, "gatewayID", gatewayID)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("UndeployLLMProviderDeployment: failed to undeploy", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to undeploy deployment")
			return
		}
	}

	log.Info("UndeployLLMProviderDeployment: undeployed successfully", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)

	resp := map[string]string{"message": "LLM provider undeployed successfully"}
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmDeploymentController) RestoreLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(gwutil.PathParamProviderId)

	// Parse query parameters
	deploymentID := r.URL.Query().Get(gwutil.PathParamDeploymentId)
	gatewayID := r.URL.Query().Get(gwutil.PathParamGatewayId)

	log.Info("RestoreLLMProviderDeployment: starting", "ouID", ouID, "providerID", providerID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	log.Info("RestoreLLMProviderDeployment: organization resolved", "ouID", ouID)

	if providerID == "" {
		log.Error("RestoreLLMProviderDeployment: provider ID is empty", "ouID", ouID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("RestoreLLMProviderDeployment: deployment ID is empty", "ouID", ouID, "providerID", providerID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		log.Error("RestoreLLMProviderDeployment: gateway ID is empty", "ouID", ouID, "providerID", providerID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	log.Info("RestoreLLMProviderDeployment: calling service layer", "ouID", ouID, "providerID", providerID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	deployment, err := c.deploymentService.RestoreLLMProviderDeployment(providerID, deploymentID, gatewayID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrContextInUse):
			log.Warn("RestoreLLMProviderDeployment: context in use on the gateway", "ouID", ouID, "error", err)
			httpx.WriteErrorResponseWithReason(w, http.StatusConflict, gwerr.ErrContextInUse.Error(), err.Error(),
				gwerr.ErrCodeContextInUse)
			return
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			log.Warn("RestoreLLMProviderDeployment: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, gwerr.ErrDeploymentNotFound):
			log.Warn("RestoreLLMProviderDeployment: deployment not found", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, gwerr.ErrGatewayNotFound):
			log.Warn("RestoreLLMProviderDeployment: gateway not found", "ouID", ouID, "gatewayID", gatewayID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, gwerr.ErrDeploymentAlreadyDeployed):
			log.Warn("RestoreLLMProviderDeployment: deployment already deployed", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)
			httpx.WriteErrorResponse(w, http.StatusConflict, "Cannot restore the deployment the gateway is already deploying or serving")
			return
		case errors.Is(err, gwerr.ErrGatewayIDMismatch):
			log.Error("RestoreLLMProviderDeployment: gateway ID mismatch", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID, "gatewayID", gatewayID)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("RestoreLLMProviderDeployment: failed to restore", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to restore deployment")
			return
		}
	}

	log.Info("RestoreLLMProviderDeployment: restored successfully", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)

	httpx.WriteSuccessResponse(w, http.StatusOK, gwconv.ConvertModelToSpecLLMDeploymentResponse(deployment))
}

func (c *llmDeploymentController) DeleteLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(gwutil.PathParamProviderId)
	deploymentID := r.PathValue(gwutil.PathParamDeploymentId)

	log.Info("DeleteLLMProviderDeployment: starting", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)

	log.Info("DeleteLLMProviderDeployment: organization resolved", "ouID", ouID)

	if providerID == "" {
		log.Error("DeleteLLMProviderDeployment: provider ID is empty", "ouID", ouID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("DeleteLLMProviderDeployment: deployment ID is empty", "ouID", ouID, "providerID", providerID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	log.Info("DeleteLLMProviderDeployment: calling service layer", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)

	err := c.deploymentService.DeleteLLMProviderDeployment(providerID, deploymentID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			log.Warn("DeleteLLMProviderDeployment: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, gwerr.ErrDeploymentNotFound):
			log.Warn("DeleteLLMProviderDeployment: deployment not found", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, gwerr.ErrDeploymentIsDeployed):
			log.Warn("DeleteLLMProviderDeployment: deployment is active", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)
			httpx.WriteErrorResponse(w, http.StatusConflict, "Cannot delete the gateway's current deployment - undeploy it and wait until it is UNDEPLOYED")
			return
		default:
			log.Error("DeleteLLMProviderDeployment: failed to delete", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete deployment")
			return
		}
	}

	log.Info("DeleteLLMProviderDeployment: deleted successfully", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)

	httpx.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

func (c *llmDeploymentController) GetLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(gwutil.PathParamProviderId)
	deploymentID := r.PathValue(gwutil.PathParamDeploymentId)

	log.Info("GetLLMProviderDeployment: starting", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)

	log.Info("GetLLMProviderDeployment: organization resolved", "ouID", ouID)

	if providerID == "" {
		log.Error("GetLLMProviderDeployment: provider ID is empty", "ouID", ouID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("GetLLMProviderDeployment: deployment ID is empty", "ouID", ouID, "providerID", providerID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	log.Info("GetLLMProviderDeployment: calling service layer", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)

	deployment, err := c.deploymentService.GetLLMProviderDeployment(providerID, deploymentID, ouID)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			log.Warn("GetLLMProviderDeployment: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, gwerr.ErrDeploymentNotFound):
			log.Warn("GetLLMProviderDeployment: deployment not found", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		default:
			log.Error("GetLLMProviderDeployment: failed to get deployment", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployment")
			return
		}
	}

	log.Info("GetLLMProviderDeployment: retrieved successfully", "ouID", ouID, "providerID", providerID, "deploymentID", deploymentID)

	httpx.WriteSuccessResponse(w, http.StatusOK, gwconv.ConvertModelToSpecLLMDeploymentResponse(deployment))
}

func (c *llmDeploymentController) GetLLMProviderDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)
	providerID := r.PathValue(gwutil.PathParamProviderId)

	// Parse optional query parameters
	gatewayID := r.URL.Query().Get(gwutil.PathParamGatewayId)
	status := r.URL.Query().Get("status")

	log.Info("GetLLMProviderDeployments: starting", "ouID", ouID, "providerID", providerID,
		"gatewayID", gatewayID, "status", status)

	log.Info("GetLLMProviderDeployments: organization resolved", "ouID", ouID)

	if providerID == "" {
		log.Error("GetLLMProviderDeployments: provider ID is empty", "ouID", ouID)
		httpx.WriteErrorResponse(w, http.StatusBadRequest, "LLM provider ID is required")
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

	log.Info("GetLLMProviderDeployments: calling service layer", "ouID", ouID, "providerID", providerID,
		"hasGatewayFilter", gatewayIDPtr != nil, "hasStatusFilter", statusPtr != nil)

	deployments, err := c.deploymentService.GetLLMProviderDeployments(providerID, ouID, gatewayIDPtr, statusPtr)
	if err != nil {
		switch {
		case errors.Is(err, gwerr.ErrLLMProviderNotFound):
			log.Warn("GetLLMProviderDeployments: provider not found", "ouID", ouID, "providerID", providerID)
			httpx.WriteErrorResponse(w, http.StatusNotFound, "LLM provider not found")
			return
		case errors.Is(err, gwerr.ErrInvalidDeploymentStatus):
			log.Error("GetLLMProviderDeployments: invalid status", "ouID", ouID, "providerID", providerID, "status", status)
			httpx.WriteErrorResponse(w, http.StatusBadRequest, "Invalid deployment status")
			return
		default:
			log.Error("GetLLMProviderDeployments: failed to get deployments", "ouID", ouID, "providerID", providerID, "error", err)
			httpx.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployments")
			return
		}
	}

	log.Info("GetLLMProviderDeployments: retrieved successfully", "ouID", ouID, "providerID", providerID, "count", len(deployments))

	httpx.WriteSuccessResponse(w, http.StatusOK, gwconv.ConvertModelToSpecLLMDeploymentResponses(deployments))
}
