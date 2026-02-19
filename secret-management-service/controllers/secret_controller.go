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
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/wso2/ai-agent-management-platform/secret-management-service/services"
)

// SecretController handles HTTP requests for secret management
type SecretController struct {
	secretService services.SecretService
	logger        *slog.Logger
}

// NewSecretController creates a new secret controller
func NewSecretController(secretService services.SecretService, logger *slog.Logger) *SecretController {
	return &SecretController{
		secretService: secretService,
		logger:        logger,
	}
}

// RegisterRoutes registers the controller's routes
func (c *SecretController) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/organizations/{orgName}/secrets", c.ListSecrets)
	mux.HandleFunc("POST /api/v1/organizations/{orgName}/secrets", c.CreateSecret)
	mux.HandleFunc("GET /api/v1/organizations/{orgName}/secrets/{path...}", c.GetSecret)
	mux.HandleFunc("PUT /api/v1/organizations/{orgName}/secrets/{path...}", c.UpdateSecret)
	mux.HandleFunc("DELETE /api/v1/organizations/{orgName}/secrets/{path...}", c.DeleteSecret)
}

// CreateSecretRequest represents the request body for creating a secret
type CreateSecretRequest struct {
	Path string            `json:"path"`
	Data map[string]string `json:"data"`
}

// UpdateSecretRequest represents the request body for updating a secret
type UpdateSecretRequest struct {
	Data map[string]string `json:"data"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// ListSecrets handles GET /api/v1/organizations/{orgName}/secrets
func (c *SecretController) ListSecrets(w http.ResponseWriter, r *http.Request) {
	orgName := r.PathValue("orgName")
	pathPrefix := r.URL.Query().Get("path")

	secrets, err := c.secretService.ListSecrets(r.Context(), orgName, pathPrefix)
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, "failed_to_list_secrets", err.Error())
		return
	}

	c.writeJSON(w, http.StatusOK, secrets)
}

// CreateSecret handles POST /api/v1/organizations/{orgName}/secrets
func (c *SecretController) CreateSecret(w http.ResponseWriter, r *http.Request) {
	orgName := r.PathValue("orgName")

	var req CreateSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if req.Path == "" {
		c.writeError(w, http.StatusBadRequest, "invalid_request", "Path is required")
		return
	}

	if len(req.Data) == 0 {
		c.writeError(w, http.StatusBadRequest, "invalid_request", "Data is required")
		return
	}

	secret, err := c.secretService.CreateSecret(r.Context(), orgName, services.CreateSecretRequest{
		Path: req.Path,
		Data: req.Data,
	})
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, "failed_to_create_secret", err.Error())
		return
	}

	c.writeJSON(w, http.StatusCreated, secret)
}

// GetSecret handles GET /api/v1/organizations/{orgName}/secrets/{path...}
func (c *SecretController) GetSecret(w http.ResponseWriter, r *http.Request) {
	orgName := r.PathValue("orgName")
	secretPath := r.PathValue("path")

	// Check if this is a versions request
	if strings.HasSuffix(secretPath, "/versions") {
		secretPath = strings.TrimSuffix(secretPath, "/versions")
		c.getSecretVersions(w, r, orgName, secretPath)
		return
	}

	var version *int
	if v := r.URL.Query().Get("version"); v != "" {
		if vInt, err := strconv.Atoi(v); err == nil {
			version = &vInt
		}
	}

	secret, err := c.secretService.GetSecret(r.Context(), orgName, secretPath, version)
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, "failed_to_get_secret", err.Error())
		return
	}

	c.writeJSON(w, http.StatusOK, secret)
}

// getSecretVersions handles GET /api/v1/organizations/{orgName}/secrets/{path}/versions
func (c *SecretController) getSecretVersions(w http.ResponseWriter, r *http.Request, orgName, secretPath string) {
	versions, err := c.secretService.GetSecretVersions(r.Context(), orgName, secretPath)
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, "failed_to_get_versions", err.Error())
		return
	}

	c.writeJSON(w, http.StatusOK, versions)
}

// UpdateSecret handles PUT /api/v1/organizations/{orgName}/secrets/{path...}
func (c *SecretController) UpdateSecret(w http.ResponseWriter, r *http.Request) {
	orgName := r.PathValue("orgName")
	secretPath := r.PathValue("path")

	var req UpdateSecretRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		c.writeError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body")
		return
	}

	if len(req.Data) == 0 {
		c.writeError(w, http.StatusBadRequest, "invalid_request", "Data is required")
		return
	}

	secret, err := c.secretService.UpdateSecret(r.Context(), orgName, secretPath, services.UpdateSecretRequest{
		Data: req.Data,
	})
	if err != nil {
		c.writeError(w, http.StatusInternalServerError, "failed_to_update_secret", err.Error())
		return
	}

	c.writeJSON(w, http.StatusOK, secret)
}

// DeleteSecret handles DELETE /api/v1/organizations/{orgName}/secrets/{path...}
func (c *SecretController) DeleteSecret(w http.ResponseWriter, r *http.Request) {
	orgName := r.PathValue("orgName")
	secretPath := r.PathValue("path")

	if err := c.secretService.DeleteSecret(r.Context(), orgName, secretPath); err != nil {
		c.writeError(w, http.StatusInternalServerError, "failed_to_delete_secret", err.Error())
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (c *SecretController) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if data != nil {
		json.NewEncoder(w).Encode(data)
	}
}

func (c *SecretController) writeError(w http.ResponseWriter, status int, errorCode, message string) {
	c.writeJSON(w, status, ErrorResponse{
		Error:   errorCode,
		Message: message,
	})
}
