// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// AgentTokenController defines the interface for agent token operations
type AgentTokenController interface {
	// GenerateToken handles the token generation request
	GenerateToken(w http.ResponseWriter, r *http.Request)
	// GetJWKS handles the JWKS endpoint request
	GetJWKS(w http.ResponseWriter, r *http.Request)
}

type agentTokenController struct {
	tokenService services.AgentTokenManagerService
}

// NewAgentTokenController creates a new AgentTokenController instance
func NewAgentTokenController(tokenService services.AgentTokenManagerService) AgentTokenController {
	return &agentTokenController{
		tokenService: tokenService,
	}
}

// GenerateToken handles POST /orgs/{org}/projects/{project}/agents/{agent}/token
func (c *agentTokenController) GenerateToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	orgName := r.PathValue(utils.PathParamOrgName)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	log.Info("GenerateToken request received",
		"orgName", orgName,
		"projName", projName,
		"agentName", agentName,
	)

	// Parse optional query parameters
	environment := r.URL.Query().Get("environment")

	// Parse optional request body
	var tokenRequest models.TokenRequest
	if r.Body != nil && r.ContentLength > 0 {
		if err := json.NewDecoder(r.Body).Decode(&tokenRequest); err != nil {
			log.Error("GenerateToken: failed to parse request body", "error", err)
			utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
			return
		}
	}

	// Build service request
	req := services.GenerateTokenRequest{
		OrgName:     orgName,
		ProjectName: projName,
		AgentName:   agentName,
		Environment: environment,
		ExpiresIn:   tokenRequest.ExpiresIn,
	}

	// Generate token
	tokenResponse, err := c.tokenService.GenerateToken(ctx, req)
	if err != nil {
		log.Error("GenerateToken: failed to generate token", "error", err)
		// Check for specific error types
		if isNotFoundError(err) {
			utils.WriteErrorResponse(w, http.StatusNotFound, err.Error())
			return
		}
		if isValidationError(err) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	log.Info("GenerateToken: token generated successfully",
		"agentName", agentName,
		"expiresAt", tokenResponse.ExpiresAt,
	)

	utils.WriteSuccessResponse(w, http.StatusOK, tokenResponse)
}

// GetJWKS handles GET /external/auth/jwks.json
func (c *agentTokenController) GetJWKS(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	log.Info("GetJWKS request received")

	jwks, err := c.tokenService.GetJWKS(ctx)
	if err != nil {
		log.Error("GetJWKS: failed to get JWKS", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve JWKS")
		return
	}

	log.Info("GetJWKS: JWKS retrieved successfully", "keyCount", len(jwks.Keys))

	utils.WriteSuccessResponse(w, http.StatusOK, jwks)
}

// isNotFoundError checks if the error indicates a resource not found
func isNotFoundError(err error) bool {
	errMsg := err.Error()
	return contains(errMsg, "not found") || contains(errMsg, "not exist")
}

// isValidationError checks if the error indicates a validation failure
func isValidationError(err error) bool {
	errMsg := err.Error()
	return contains(errMsg, "invalid") || contains(errMsg, "must be") || contains(errMsg, "cannot exceed")
}

// contains checks if the string contains the substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsIgnoreCase(s, substr))
}

// containsIgnoreCase is a simple case-insensitive contains check
func containsIgnoreCase(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if equalIgnoreCase(s[i:i+len(substr)], substr) {
			return true
		}
	}
	return false
}

// equalIgnoreCase compares two strings ignoring case
func equalIgnoreCase(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}
