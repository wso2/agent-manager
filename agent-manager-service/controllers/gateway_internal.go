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
	"fmt"
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// GatewayInternalController handles internal API requests from gateways
// Endpoints must match api-platform's internal API exactly
type GatewayInternalController interface {
	// GetAPIsByOrganization handles GET /api/internal/v1/apis
	// Returns ZIP file with all API/LLM provider configurations (same as api-platform)
	GetAPIsByOrganization(w http.ResponseWriter, r *http.Request)

	// GetAPI handles GET /api/internal/v1/apis/:apiId
	// Returns ZIP file with YAML configuration (same as api-platform)
	GetAPI(w http.ResponseWriter, r *http.Request)

	// CreateGatewayDeployment handles POST /api/internal/v1/apis/:apiId/gateway-deployments
	CreateGatewayDeployment(w http.ResponseWriter, r *http.Request)
}

type gatewayInternalController struct {
	internalService services.GatewayInternalService
	gatewayService  services.GatewayService
}

// NewGatewayInternalController creates a new gateway internal controller
func NewGatewayInternalController(
	internalService services.GatewayInternalService,
	gatewayService services.GatewayService,
) GatewayInternalController {
	return &gatewayInternalController{
		internalService: internalService,
		gatewayService:  gatewayService,
	}
}

// GetAPIsByOrganization handles GET /api/internal/v1/apis
// Gateway calls this endpoint to fetch all LLM provider configurations
// Response format must match api-platform exactly (ZIP file with YAML)
func (c *gatewayInternalController) GetAPIsByOrganization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Authenticate gateway using API key
	apiKey := r.Header.Get("api-key")
	if apiKey == "" {
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Missing API key")
		return
	}

	gateway, err := c.gatewayService.VerifyToken(ctx, apiKey)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Invalid API key")
		return
	}

	orgID := gateway.OrganizationName

	// Get all API configurations as YAML map
	apis, err := c.internalService.GetAPIsByOrganization(ctx, orgID)
	if err != nil {
		log.Error("GetAPIsByOrganization: failed to get APIs", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get APIs")
		return
	}

	// Create ZIP file from API YAML files
	zipData, err := utils.CreateAPIYamlZip(apis)
	if err != nil {
		log.Error("GetAPIsByOrganization: failed to create ZIP", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create API package")
		return
	}

	// Set headers for ZIP file download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"apis-org-%s.zip\"", orgID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(zipData)
}

// GetAPI handles GET /api/internal/v1/apis/:apiId
// Gateway calls this endpoint to fetch full API/LLM provider configuration
// Response format must match api-platform exactly (ZIP file with YAML)
func (c *gatewayInternalController) GetAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Authenticate gateway using API key
	apiKey := r.Header.Get("api-key")
	if apiKey == "" {
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Missing API key")
		return
	}

	gateway, err := c.gatewayService.VerifyToken(ctx, apiKey)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Invalid API key")
		return
	}

	// Get API ID from path (this is actually the provider ID)
	apiID := r.PathValue("apiId")
	if apiID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing API ID")
		return
	}

	// Get API configuration as YAML map (for ZIP creation)
	api, err := c.internalService.GetAPI(ctx, apiID, gateway.UUID.String())
	if err != nil {
		log.Error("GetAPI: failed to get API configuration", "error", err)
		if errors.Is(err, utils.ErrGatewayNotFound) || errors.Is(err, utils.ErrProviderNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "API not found")
		} else {
			utils.WriteErrorResponse(w, http.StatusInternalServerError, err.Error())
		}
		return
	}

	// Create ZIP file from API YAML file
	zipData, err := utils.CreateAPIYamlZip(api)
	if err != nil {
		log.Error("GetAPI: failed to create ZIP", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create API package")
		return
	}

	// Set headers for ZIP file download (same as api-platform)
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"api-%s.zip\"", apiID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(zipData)
}

// CreateGatewayDeployment handles POST /api/internal/v1/apis/:apiId/gateway-deployments
// Gateway calls this endpoint to register deployment status
func (c *gatewayInternalController) CreateGatewayDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Authenticate gateway using API key
	apiKey := r.Header.Get("api-key")
	if apiKey == "" {
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Missing API key")
		return
	}

	gateway, err := c.gatewayService.VerifyToken(ctx, apiKey)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Invalid API key")
		return
	}

	// Get API ID from path
	apiID := r.PathValue("apiId")
	if apiID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Missing API ID")
		return
	}

	// Extract optional revision ID from query parameter
	revisionID := r.URL.Query().Get("revisionId")
	var revisionIDPtr *string
	if revisionID != "" {
		revisionIDPtr = &revisionID
	}

	// Parse request body (must match api-platform's DeploymentNotification format)
	var notification models.GatewayDeploymentNotification
	if err := json.NewDecoder(r.Body).Decode(&notification); err != nil {
		log.Error("CreateGatewayDeployment: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Create deployment using the service
	orgID := gateway.OrganizationName
	gatewayID := gateway.UUID.String()

	response, err := c.internalService.CreateGatewayDeployment(ctx, apiID, orgID, gatewayID, &notification, revisionIDPtr)
	if err != nil {
		log.Error("CreateGatewayDeployment: failed to create deployment", "error", err)
		if errors.Is(err, utils.ErrInvalidInput) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input data")
		} else if errors.Is(err, utils.ErrGatewayNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
		} else if errors.Is(err, utils.ErrProviderNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Provider not found")
		} else {
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create deployment")
		}
		return
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, map[string]interface{}{
		"message": response.Message,
	})
}
