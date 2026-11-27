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
	"errors"
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwutil"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconv"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
)

// LLMDeploymentController defines interface for LLM deployment HTTP handlers
type LLMDeploymentController interface {
	RestoreLLMProviderDeployment(w http.ResponseWriter, r *http.Request)
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

func (c *llmDeploymentController) RestoreLLMProviderDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
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

func (c *llmDeploymentController) GetLLMProviderDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := modkit.OrgID(r.Context())
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
