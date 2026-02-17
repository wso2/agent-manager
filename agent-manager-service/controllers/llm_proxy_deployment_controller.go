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
	orgRepo           repositories.OrganizationRepository
}

// NewLLMProxyDeploymentController creates a new LLM proxy deployment controller
func NewLLMProxyDeploymentController(
	deploymentService *services.LLMProxyDeploymentService,
	orgRepo repositories.OrganizationRepository,
) LLMProxyDeploymentController {
	return &llmProxyDeploymentController{
		deploymentService: deploymentService,
		orgRepo:           orgRepo,
	}
}

// resolveOrgUUID resolves organization handle to UUID
func (c *llmProxyDeploymentController) resolveOrgUUID(ctx context.Context, orgName string) (string, error) {
	org, err := c.orgRepo.GetOrganizationByName(orgName)
	if err != nil {
		return "", fmt.Errorf("failed to resolve organization: %w", err)
	}
	if org == nil {
		return "", utils.ErrOrganizationNotFound
	}
	return org.UUID.String(), nil
}

func (c *llmProxyDeploymentController) DeployLLMProxy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue("id")

	log.Info("DeployLLMProxy: starting", "orgName", orgName, "proxyID", proxyID)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Warn("DeployLLMProxy: organization not found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		} else {
			log.Error("DeployLLMProxy: failed to resolve organization", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	log.Info("DeployLLMProxy: organization resolved", "orgName", orgName, "orgID", orgID)

	if proxyID == "" {
		log.Error("DeployLLMProxy: proxy ID is empty", "orgName", orgName)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}

	var req models.DeployAPIRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("DeployLLMProxy: failed to decode request", "orgName", orgName, "proxyID", proxyID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	log.Info("DeployLLMProxy: request decoded", "orgName", orgName, "proxyID", proxyID,
		"deploymentName", req.Name, "base", req.Base, "gatewayID", req.GatewayID)

	// Validate required fields
	if req.Name == "" {
		log.Error("DeployLLMProxy: deployment name is required", "orgName", orgName, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.Base == "" {
		log.Error("DeployLLMProxy: base is required", "orgName", orgName, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "base is required (use 'current' or a deploymentId)")
		return
	}
	if req.GatewayID == "" {
		log.Error("DeployLLMProxy: gateway ID is required", "orgName", orgName, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId is required")
		return
	}

	log.Info("DeployLLMProxy: calling service layer", "orgName", orgName, "proxyID", proxyID,
		"deploymentName", req.Name, "gatewayID", req.GatewayID)

	deployment, err := c.deploymentService.DeployLLMProxy(proxyID, &req, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("DeployLLMProxy: proxy not found", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("DeployLLMProxy: gateway not found", "orgName", orgName, "proxyID", proxyID, "gatewayID", req.GatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrBaseDeploymentNotFound):
			log.Warn("DeployLLMProxy: base deployment not found", "orgName", orgName, "proxyID", proxyID, "base", req.Base)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Base deployment not found")
			return
		case errors.Is(err, utils.ErrDeploymentNameRequired):
			log.Error("DeployLLMProxy: deployment name required", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment name is required")
			return
		case errors.Is(err, utils.ErrDeploymentBaseRequired):
			log.Error("DeployLLMProxy: base required", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Base is required (use 'current' or a deploymentId)")
			return
		case errors.Is(err, utils.ErrDeploymentGatewayIDRequired):
			log.Error("DeployLLMProxy: gateway ID required", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Gateway ID is required")
			return
		case errors.Is(err, utils.ErrLLMProviderNotFound):
			log.Error("DeployLLMProxy: referenced provider not found", "orgName", orgName, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Referenced LLM provider not found")
			return
		case errors.Is(err, utils.ErrInvalidInput):
			log.Error("DeployLLMProxy: invalid input", "orgName", orgName, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
			return
		default:
			log.Error("DeployLLMProxy: failed to deploy", "orgName", orgName, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to deploy LLM proxy")
			return
		}
	}

	log.Info("DeployLLMProxy: deployment created successfully", "orgName", orgName, "proxyID", proxyID,
		"deploymentID", deployment.DeploymentID, "gatewayID", req.GatewayID)

	utils.WriteSuccessResponse(w, http.StatusCreated, deployment)
}

func (c *llmProxyDeploymentController) UndeployLLMProxyDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue("id")

	// Parse query parameters
	deploymentID := r.URL.Query().Get("deploymentId")
	gatewayID := r.URL.Query().Get("gatewayId")

	log.Info("UndeployLLMProxyDeployment: starting", "orgName", orgName, "proxyID", proxyID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Warn("UndeployLLMProxyDeployment: organization not found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		} else {
			log.Error("UndeployLLMProxyDeployment: failed to resolve organization", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	log.Info("UndeployLLMProxyDeployment: organization resolved", "orgName", orgName, "orgID", orgID)

	if proxyID == "" {
		log.Error("UndeployLLMProxyDeployment: proxy ID is empty", "orgName", orgName)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("UndeployLLMProxyDeployment: deployment ID is empty", "orgName", orgName, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		log.Error("UndeployLLMProxyDeployment: gateway ID is empty", "orgName", orgName, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	log.Info("UndeployLLMProxyDeployment: calling service layer", "orgName", orgName, "proxyID", proxyID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	_, err = c.deploymentService.UndeployLLMProxyDeployment(proxyID, deploymentID, gatewayID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("UndeployLLMProxyDeployment: proxy not found", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("UndeployLLMProxyDeployment: deployment not found", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("UndeployLLMProxyDeployment: gateway not found", "orgName", orgName, "gatewayID", gatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotActive):
			log.Warn("UndeployLLMProxyDeployment: deployment not active", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "No active deployment found for this LLM proxy on the gateway")
			return
		case errors.Is(err, utils.ErrGatewayIDMismatch):
			log.Error("UndeployLLMProxyDeployment: gateway ID mismatch", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID, "gatewayID", gatewayID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("UndeployLLMProxyDeployment: failed to undeploy", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to undeploy deployment")
			return
		}
	}

	log.Info("UndeployLLMProxyDeployment: undeployed successfully", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)

	resp := map[string]string{"message": "LLM proxy undeployed successfully"}
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

func (c *llmProxyDeploymentController) RestoreLLMProxyDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue("id")

	// Parse query parameters
	deploymentID := r.URL.Query().Get("deploymentId")
	gatewayID := r.URL.Query().Get("gatewayId")

	log.Info("RestoreLLMProxyDeployment: starting", "orgName", orgName, "proxyID", proxyID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Warn("RestoreLLMProxyDeployment: organization not found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		} else {
			log.Error("RestoreLLMProxyDeployment: failed to resolve organization", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	log.Info("RestoreLLMProxyDeployment: organization resolved", "orgName", orgName, "orgID", orgID)

	if proxyID == "" {
		log.Error("RestoreLLMProxyDeployment: proxy ID is empty", "orgName", orgName)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("RestoreLLMProxyDeployment: deployment ID is empty", "orgName", orgName, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "deploymentId query parameter is required")
		return
	}
	if gatewayID == "" {
		log.Error("RestoreLLMProxyDeployment: gateway ID is empty", "orgName", orgName, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "gatewayId query parameter is required")
		return
	}

	log.Info("RestoreLLMProxyDeployment: calling service layer", "orgName", orgName, "proxyID", proxyID,
		"deploymentID", deploymentID, "gatewayID", gatewayID)

	deployment, err := c.deploymentService.RestoreLLMProxyDeployment(proxyID, deploymentID, gatewayID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("RestoreLLMProxyDeployment: proxy not found", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("RestoreLLMProxyDeployment: deployment not found", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrGatewayNotFound):
			log.Warn("RestoreLLMProxyDeployment: gateway not found", "orgName", orgName, "gatewayID", gatewayID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
			return
		case errors.Is(err, utils.ErrDeploymentAlreadyDeployed):
			log.Warn("RestoreLLMProxyDeployment: deployment already deployed", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot restore currently deployed deployment")
			return
		case errors.Is(err, utils.ErrGatewayIDMismatch):
			log.Error("RestoreLLMProxyDeployment: gateway ID mismatch", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID, "gatewayID", gatewayID)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment is bound to a different gateway")
			return
		default:
			log.Error("RestoreLLMProxyDeployment: failed to restore", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to restore deployment")
			return
		}
	}

	log.Info("RestoreLLMProxyDeployment: restored successfully", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusOK, deployment)
}

func (c *llmProxyDeploymentController) DeleteLLMProxyDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue("id")
	deploymentID := r.PathValue("deploymentId")

	log.Info("DeleteLLMProxyDeployment: starting", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Warn("DeleteLLMProxyDeployment: organization not found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		} else {
			log.Error("DeleteLLMProxyDeployment: failed to resolve organization", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	log.Info("DeleteLLMProxyDeployment: organization resolved", "orgName", orgName, "orgID", orgID)

	if proxyID == "" {
		log.Error("DeleteLLMProxyDeployment: proxy ID is empty", "orgName", orgName)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("DeleteLLMProxyDeployment: deployment ID is empty", "orgName", orgName, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	log.Info("DeleteLLMProxyDeployment: calling service layer", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)

	err = c.deploymentService.DeleteLLMProxyDeployment(proxyID, deploymentID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("DeleteLLMProxyDeployment: proxy not found", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("DeleteLLMProxyDeployment: deployment not found", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		case errors.Is(err, utils.ErrDeploymentIsDeployed):
			log.Warn("DeleteLLMProxyDeployment: deployment is active", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusConflict, "Cannot delete an active deployment - undeploy it first")
			return
		default:
			log.Error("DeleteLLMProxyDeployment: failed to delete", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete deployment")
			return
		}
	}

	log.Info("DeleteLLMProxyDeployment: deleted successfully", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

func (c *llmProxyDeploymentController) GetLLMProxyDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue("id")
	deploymentID := r.PathValue("deploymentId")

	log.Info("GetLLMProxyDeployment: starting", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Warn("GetLLMProxyDeployment: organization not found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		} else {
			log.Error("GetLLMProxyDeployment: failed to resolve organization", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	log.Info("GetLLMProxyDeployment: organization resolved", "orgName", orgName, "orgID", orgID)

	if proxyID == "" {
		log.Error("GetLLMProxyDeployment: proxy ID is empty", "orgName", orgName)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "LLM proxy ID is required")
		return
	}
	if deploymentID == "" {
		log.Error("GetLLMProxyDeployment: deployment ID is empty", "orgName", orgName, "proxyID", proxyID)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Deployment ID is required")
		return
	}

	log.Info("GetLLMProxyDeployment: calling service layer", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)

	deployment, err := c.deploymentService.GetLLMProxyDeployment(proxyID, deploymentID, orgID)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("GetLLMProxyDeployment: proxy not found", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrDeploymentNotFound):
			log.Warn("GetLLMProxyDeployment: deployment not found", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Deployment not found")
			return
		default:
			log.Error("GetLLMProxyDeployment: failed to get deployment", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployment")
			return
		}
	}

	log.Info("GetLLMProxyDeployment: retrieved successfully", "orgName", orgName, "proxyID", proxyID, "deploymentID", deploymentID)

	utils.WriteSuccessResponse(w, http.StatusOK, deployment)
}

func (c *llmProxyDeploymentController) GetLLMProxyDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	proxyID := r.PathValue("id")

	// Parse optional query parameters
	gatewayID := r.URL.Query().Get("gatewayId")
	status := r.URL.Query().Get("status")

	log.Info("GetLLMProxyDeployments: starting", "orgName", orgName, "proxyID", proxyID,
		"gatewayID", gatewayID, "status", status)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			log.Warn("GetLLMProxyDeployments: organization not found", "orgName", orgName)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Organization not found")
		} else {
			log.Error("GetLLMProxyDeployments: failed to resolve organization", "orgName", orgName, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		}
		return
	}

	log.Info("GetLLMProxyDeployments: organization resolved", "orgName", orgName, "orgID", orgID)

	if proxyID == "" {
		log.Error("GetLLMProxyDeployments: proxy ID is empty", "orgName", orgName)
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

	log.Info("GetLLMProxyDeployments: calling service layer", "orgName", orgName, "proxyID", proxyID,
		"hasGatewayFilter", gatewayIDPtr != nil, "hasStatusFilter", statusPtr != nil)

	deployments, err := c.deploymentService.GetLLMProxyDeployments(proxyID, orgID, gatewayIDPtr, statusPtr)
	if err != nil {
		switch {
		case errors.Is(err, utils.ErrLLMProxyNotFound):
			log.Warn("GetLLMProxyDeployments: proxy not found", "orgName", orgName, "proxyID", proxyID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "LLM proxy not found")
			return
		case errors.Is(err, utils.ErrInvalidDeploymentStatus):
			log.Error("GetLLMProxyDeployments: invalid status", "orgName", orgName, "proxyID", proxyID, "status", status)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid deployment status")
			return
		default:
			log.Error("GetLLMProxyDeployments: failed to get deployments", "orgName", orgName, "proxyID", proxyID, "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve deployments")
			return
		}
	}

	log.Info("GetLLMProxyDeployments: retrieved successfully", "orgName", orgName, "proxyID", proxyID, "count", len(deployments))

	utils.WriteSuccessResponse(w, http.StatusOK, deployments)
}
