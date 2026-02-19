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
	"errors"
	"fmt"
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// GatewayInternalController defines interface for gateway internal API HTTP handlers
type GatewayInternalController interface {
	GetLLMProvider(w http.ResponseWriter, r *http.Request)
	GetLLMProxy(w http.ResponseWriter, r *http.Request)
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

	orgName := gateway.OrganizationName
	gatewayID := gateway.UUID.String()
	providerID := r.PathValue("providerId")
	if providerID == "" {
		http.Error(w, "Provider ID is required", http.StatusBadRequest)
		return
	}

	provider, err := c.gatewayInternalService.GetActiveLLMProviderDeploymentByGateway(providerID, orgName, gatewayID)
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

// GetLLMProxy handles GET /api/internal/v1/llm-proxies/:proxyId
func (c *gatewayInternalController) GetLLMProxy(w http.ResponseWriter, r *http.Request) {
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

	orgName := gateway.OrganizationName
	gatewayID := gateway.UUID.String()
	proxyID := r.PathValue("proxyId")
	if proxyID == "" {
		http.Error(w, "Proxy ID is required", http.StatusBadRequest)
		return
	}

	proxy, err := c.gatewayInternalService.GetActiveLLMProxyDeploymentByGateway(proxyID, orgName, gatewayID)
	if err != nil {
		if errors.Is(err, utils.ErrDeploymentNotActive) {
			http.Error(w, "No active deployment found for this LLM proxy on this gateway", http.StatusNotFound)
			return
		}
		if errors.Is(err, utils.ErrLLMProxyNotFound) {
			http.Error(w, "LLM proxy not found", http.StatusNotFound)
			return
		}
		log.Error("Failed to get LLM proxy", "error", err)
		http.Error(w, "Failed to get LLM proxy", http.StatusInternalServerError)
		return
	}

	// Create ZIP file from LLM proxy YAML file
	zipData, err := utils.CreateLLMProxyYamlZip(proxy)
	if err != nil {
		log.Error("Failed to create ZIP file for LLM proxy", "proxyID", proxyID, "error", err)
		http.Error(w, "Failed to create LLM proxy package", http.StatusInternalServerError)
		return
	}

	// Set headers for ZIP file download
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"llm-proxy-%s.zip\"", proxyID))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(zipData)))

	// Return ZIP file
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(zipData); err != nil {
		log.Error("Failed to write ZIP response", "proxyID", proxyID, "error", err)
	}
}
