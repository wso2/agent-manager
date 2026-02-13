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

// GatewayInternalController defines interface for gateway internal API HTTP handlers
type GatewayInternalController interface {
	GetAPIsByOrganization(w http.ResponseWriter, r *http.Request)
	GetAPI(w http.ResponseWriter, r *http.Request)
	CreateGatewayDeployment(w http.ResponseWriter, r *http.Request)
	GetLLMProvider(w http.ResponseWriter, r *http.Request)
}

type gatewayInternalController struct {
	gatewayService         *services.PlatformGatewayService
	gatewayInternalService *services.GatewayInternalAPIService
}

// NewGatewayInternalController creates a new gateway internal controller
func NewGatewayInternalController(
	gatewayService *services.PlatformGatewayService,
	gatewayInternalService *services.GatewayInternalAPIService,
) GatewayInternalController {
	return &gatewayInternalController{
		gatewayService:         gatewayService,
		gatewayInternalService: gatewayInternalService,
	}
}

// GetAPIsByOrganization handles GET /api/internal/v1/apis
func (c *gatewayInternalController) GetAPIsByOrganization(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract client IP for logging
	clientIP := getClientIP(r)

	// Extract and validate API key from header
	apiKey := r.Header.Get("api-key")
	if apiKey == "" {
		log.Warn("Unauthorized access attempt - Missing API key", "ip", clientIP)
		http.Error(w, "API key is required. Provide 'api-key' header.", http.StatusUnauthorized)
		return
	}

	// Authenticate gateway using API key
	gateway, err := c.gatewayService.VerifyToken(apiKey)
	if err != nil {
		log.Warn("Authentication failed", "ip", clientIP, "error", err)
		http.Error(w, "Invalid or expired API key", http.StatusUnauthorized)
		return
	}

	orgID := gateway.OrganizationUUID.String()
	apis, err := c.gatewayInternalService.GetAPIsByOrganization(orgID)
	if err != nil {
		if errors.Is(err, utils.ErrOrganizationNotFound) {
			http.Error(w, "Organization not found", http.StatusNotFound)
			return
		}
		log.Error("Failed to get APIs", "error", err)
		http.Error(w, "Failed to get APIs", http.StatusInternalServerError)
		return
	}

	// Create ZIP file from API YAML files
	zipData, err := utils.CreateAPIYamlZip(apis)
	if err != nil {
		log.Error("Failed to create ZIP file", "orgID", orgID, "error", err)
		http.Error(w, "Failed to create API package", http.StatusInternalServerError)
		return
	}

	// Set headers for ZIP file download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"apis-org-%s.zip\"", orgID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))

	// Return ZIP file
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(zipData); err != nil {
		log.Error("Failed to write ZIP response", "orgID", orgID, "error", err)
	}
}

// GetAPI handles GET /api/internal/v1/apis/:apiId
func (c *gatewayInternalController) GetAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract client IP for logging
	clientIP := getClientIP(r)

	// Extract and validate API key from header
	apiKey := r.Header.Get("api-key")
	if apiKey == "" {
		log.Warn("Unauthorized access attempt - Missing API key", "ip", clientIP)
		http.Error(w, "API key is required. Provide 'api-key' header.", http.StatusUnauthorized)
		return
	}

	// Authenticate gateway using API key
	gateway, err := c.gatewayService.VerifyToken(apiKey)
	if err != nil {
		log.Warn("Authentication failed", "ip", clientIP, "error", err)
		http.Error(w, "Invalid or expired API key", http.StatusUnauthorized)
		return
	}

	orgID := gateway.OrganizationUUID.String()
	gatewayID := gateway.UUID.String()
	apiID := r.PathValue("apiId")
	if apiID == "" {
		http.Error(w, "API ID is required", http.StatusBadRequest)
		return
	}

	api, err := c.gatewayInternalService.GetActiveDeploymentByGateway(apiID, orgID, gatewayID)
	if err != nil {
		if errors.Is(err, utils.ErrDeploymentNotActive) {
			http.Error(w, "No active deployment found for this API on this gateway", http.StatusNotFound)
			return
		}
		if errors.Is(err, utils.ErrAPINotFound) {
			http.Error(w, "API not found", http.StatusNotFound)
			return
		}
		log.Error("Failed to get API", "error", err)
		http.Error(w, "Failed to get API", http.StatusInternalServerError)
		return
	}

	// Create ZIP file from API YAML file
	zipData, err := utils.CreateAPIYamlZip(api)
	if err != nil {
		log.Error("Failed to create ZIP file", "apiID", apiID, "error", err)
		http.Error(w, "Failed to create API package", http.StatusInternalServerError)
		return
	}

	// Set headers for ZIP file download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"api-%s.zip\"", apiID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))

	// Return ZIP file
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(zipData); err != nil {
		log.Error("Failed to write ZIP response", "apiID", apiID, "error", err)
	}
}

// CreateGatewayDeployment handles POST /api/internal/v1/apis/{apiId}/gateway-deployments
func (c *gatewayInternalController) CreateGatewayDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract client IP for logging
	clientIP := getClientIP(r)

	// Extract and validate API key from header
	apiKey := r.Header.Get("api-key")
	if apiKey == "" {
		log.Warn("Unauthorized access attempt - Missing API key", "ip", clientIP)
		http.Error(w, "API key is required. Provide 'api-key' header.", http.StatusUnauthorized)
		return
	}

	// Authenticate gateway using API key
	gateway, err := c.gatewayService.VerifyToken(apiKey)
	if err != nil {
		log.Warn("Authentication failed", "ip", clientIP, "error", err)
		http.Error(w, "Invalid or expired API key", http.StatusUnauthorized)
		return
	}

	// Extract API ID from path parameter
	apiID := r.PathValue("apiId")
	if apiID == "" {
		http.Error(w, "API ID is required", http.StatusBadRequest)
		return
	}

	// Extract optional deployment ID from query parameter
	deploymentID := r.URL.Query().Get("deploymentId")
	var deploymentIDPtr *string
	if deploymentID != "" {
		deploymentIDPtr = &deploymentID
	}

	// Parse and validate request body
	var notificationReq models.DeploymentNotification
	if err := json.NewDecoder(r.Body).Decode(&notificationReq); err != nil {
		log.Warn("Invalid request body", "ip", clientIP, "error", err)
		http.Error(w, "Invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Create API deployment using the service
	orgID := gateway.OrganizationUUID.String()
	gatewayID := gateway.UUID.String()

	// Convert models.DeploymentNotification to services.DeploymentNotification
	notification := services.DeploymentNotification{
		ProjectIdentifier: notificationReq.ProjectIdentifier,
		Configuration: services.APIDeploymentYAML{
			ApiVersion: notificationReq.Configuration.Version,
			Kind:       notificationReq.Configuration.Kind,
			Spec: services.APIDeploymentSpec{
				Name:    notificationReq.Configuration.Spec.Name,
				Version: notificationReq.Configuration.Spec.Version,
				Context: notificationReq.Configuration.Spec.Context,
			},
		},
	}

	response, err := c.gatewayInternalService.CreateGatewayDeployment(
		apiID, orgID, gatewayID, notification, deploymentIDPtr)
	if err != nil {
		if errors.Is(err, utils.ErrInvalidInput) {
			http.Error(w, "Invalid input data", http.StatusBadRequest)
			return
		}
		if errors.Is(err, utils.ErrGatewayNotFound) {
			http.Error(w, "Gateway not found", http.StatusNotFound)
			return
		}
		if errors.Is(err, utils.ErrAPINotFound) {
			http.Error(w, "API not found", http.StatusNotFound)
			return
		}
		log.Error("Failed to create gateway API deployment", "apiID", apiID, "gatewayID", gatewayID, "error", err)
		http.Error(w, "Failed to create API deployment", http.StatusInternalServerError)
		return
	}

	log.Info("Successfully created gateway API deployment", "apiID", apiID, "gatewayID", gatewayID, "created", response.Created)

	// Return success response
	utils.WriteSuccessResponse(w, http.StatusCreated, map[string]interface{}{
		"message": response.Message,
	})
}

// GetLLMProvider handles GET /api/internal/v1/llm-providers/:providerId
func (c *gatewayInternalController) GetLLMProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract client IP for logging
	clientIP := getClientIP(r)

	// Extract and validate API key from header
	apiKey := r.Header.Get("api-key")
	if apiKey == "" {
		log.Warn("Unauthorized access attempt - Missing API key", "ip", clientIP)
		http.Error(w, "API key is required. Provide 'api-key' header.", http.StatusUnauthorized)
		return
	}

	// Authenticate gateway using API key
	gateway, err := c.gatewayService.VerifyToken(apiKey)
	if err != nil {
		log.Warn("Authentication failed", "ip", clientIP, "error", err)
		http.Error(w, "Invalid or expired API key", http.StatusUnauthorized)
		return
	}

	orgID := gateway.OrganizationUUID.String()
	gatewayID := gateway.UUID.String()
	providerID := r.PathValue("providerId")
	if providerID == "" {
		http.Error(w, "Provider ID is required", http.StatusBadRequest)
		return
	}

	provider, err := c.gatewayInternalService.GetActiveLLMProviderDeploymentByGateway(providerID, orgID, gatewayID)
	if err != nil {
		if errors.Is(err, utils.ErrDeploymentNotActive) {
			http.Error(w, "No active deployment found for this LLM provider on this gateway", http.StatusNotFound)
			return
		}
		if errors.Is(err, utils.ErrLLMProviderNotFound) {
			http.Error(w, "LLM provider not found", http.StatusNotFound)
			return
		}
		log.Error("Failed to get LLM provider", "error", err)
		http.Error(w, "Failed to get LLM provider", http.StatusInternalServerError)
		return
	}

	// Create ZIP file from LLM provider YAML file
	zipData, err := utils.CreateLLMProviderYamlZip(provider)
	if err != nil {
		log.Error("Failed to create ZIP file for LLM provider", "providerID", providerID, "error", err)
		http.Error(w, "Failed to create LLM provider package", http.StatusInternalServerError)
		return
	}

	// Set headers for ZIP file download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"llm-provider-%s.zip\"", providerID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))

	// Return ZIP file
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(zipData); err != nil {
		log.Error("Failed to write ZIP response", "providerID", providerID, "error", err)
	}
}
