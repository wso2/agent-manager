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
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

type EvaluatorController interface {
	ListEvaluators(w http.ResponseWriter, r *http.Request)
	GetEvaluator(w http.ResponseWriter, r *http.Request)
}

type evaluatorController struct {
	evaluatorService services.EvaluatorManagerService
}

// NewEvaluatorController creates a new evaluator controller instance
func NewEvaluatorController(evaluatorService services.EvaluatorManagerService) EvaluatorController {
	return &evaluatorController{
		evaluatorService: evaluatorService,
	}
}

// ListEvaluators handles GET /orgs/{orgName}/evaluators
func (c *evaluatorController) ListEvaluators(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract org name from path
	orgName := r.PathValue("orgName")
	if orgName == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Organization name is required")
		return
	}

	// Parse query parameters
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32)
	if limit == 0 {
		limit = 20 // default
	}
	if limit > 100 {
		limit = 100 // max cap
	}

	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 32)

	// Parse tags filter (comma-separated)
	var tags []string
	if tagsParam := r.URL.Query().Get("tags"); tagsParam != "" {
		tags = strings.Split(tagsParam, ",")
		// Trim whitespace from each tag
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
	}

	search := r.URL.Query().Get("search")
	provider := r.URL.Query().Get("provider")

	// For now, we'll use a nil orgID to only show builtins
	// In future, when org support is added, parse orgName to orgID
	var orgID *uuid.UUID = nil

	filters := services.EvaluatorFilters{
		Limit:    int32(limit),
		Offset:   int32(offset),
		Tags:     tags,
		Search:   search,
		Provider: provider,
	}

	// Call service
	evaluators, total, err := c.evaluatorService.ListEvaluators(ctx, orgID, filters)
	if err != nil {
		log.Error("Failed to list evaluators", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list evaluators")
		return
	}

	// Build response
	specEvaluators := make([]spec.EvaluatorResponse, len(evaluators))
	for i, evaluator := range evaluators {
		specEvaluators[i] = convertToSpecEvaluatorResponse(evaluator)
	}

	response := spec.EvaluatorListResponse{
		Evaluators: specEvaluators,
		Total:      total,
		Limit:      int32(limit),
		Offset:     int32(offset),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// GetEvaluator handles GET /orgs/{orgName}/evaluators/{evaluatorId}
func (c *evaluatorController) GetEvaluator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract org name from path
	orgName := r.PathValue("orgName")
	if orgName == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Organization name is required")
		return
	}

	// Extract and URL-decode evaluator identifier
	evaluatorID := r.PathValue("evaluatorId")
	if evaluatorID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Evaluator ID is required")
		return
	}

	// URL decode the identifier (handles "deepeval%2Ftool-correctness" -> "deepeval/tool-correctness")
	decodedID, err := url.PathUnescape(evaluatorID)
	if err != nil {
		log.Warn("Failed to decode evaluator ID", "evaluatorId", evaluatorID, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid evaluator ID")
		return
	}

	// For now, we'll use a nil orgID to only show builtins
	var orgID *uuid.UUID = nil

	// Call service
	evaluator, err := c.evaluatorService.GetEvaluator(ctx, orgID, decodedID)
	if err != nil {
		if errors.Is(err, utils.ErrEvaluatorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Evaluator not found")
			return
		}
		log.Error("Failed to get evaluator", "identifier", decodedID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get evaluator")
		return
	}

	// Convert to spec response
	response := convertToSpecEvaluatorResponse(evaluator)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// convertToSpecEvaluatorResponse converts models.EvaluatorResponse to spec.EvaluatorResponse
func convertToSpecEvaluatorResponse(evaluator *models.EvaluatorResponse) spec.EvaluatorResponse {
	configFields := make([]spec.EvaluatorConfigParam, len(evaluator.ConfigSchema))
	for i, param := range evaluator.ConfigSchema {
		field := spec.EvaluatorConfigParam{
			Key:         param.Key,
			Type:        param.Type,
			Description: param.Description,
			Required:    param.Required,
		}

		if param.Default != nil {
			field.Default = param.Default
		}

		if param.Min != nil {
			field.Min = param.Min
		}

		if param.Max != nil {
			field.Max = param.Max
		}

		if len(param.EnumValues) > 0 {
			field.EnumValues = param.EnumValues
		}

		configFields[i] = field
	}

	return spec.EvaluatorResponse{
		Id:           evaluator.ID.String(),
		Identifier:   evaluator.Identifier,
		DisplayName:  evaluator.DisplayName,
		Description:  evaluator.Description,
		Version:      evaluator.Version,
		Provider:     evaluator.Provider,
		Tags:         evaluator.Tags,
		IsBuiltin:    evaluator.IsBuiltin,
		ConfigSchema: configFields,
		CreatedAt:    evaluator.CreatedAt,
		UpdatedAt:    evaluator.UpdatedAt,
	}
}
