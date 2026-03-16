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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/catalog"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

type EvaluatorController interface {
	ListEvaluators(w http.ResponseWriter, r *http.Request)
	GetEvaluator(w http.ResponseWriter, r *http.Request)
	ListLLMProviders(w http.ResponseWriter, r *http.Request)

	// Custom evaluator CRUD
	CreateCustomEvaluator(w http.ResponseWriter, r *http.Request)
	GetCustomEvaluator(w http.ResponseWriter, r *http.Request)
	UpdateCustomEvaluator(w http.ResponseWriter, r *http.Request)
	DeleteCustomEvaluator(w http.ResponseWriter, r *http.Request)
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

	orgName := r.PathValue(utils.PathParamOrgName)

	// Parse query parameters
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 32)
	if limit <= 0 {
		limit = 20 // default
	}
	if limit > 100 {
		limit = 100 // max cap
	}

	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 32)
	if offset < 0 {
		offset = 0
	}

	// Parse tags filter (comma-separated)
	var tags []string
	if tagsParam := r.URL.Query().Get("tags"); tagsParam != "" {
		tags = strings.Split(tagsParam, ",")
		for i := range tags {
			tags[i] = strings.TrimSpace(tags[i])
		}
	}

	search := r.URL.Query().Get("search")
	provider := r.URL.Query().Get("provider")
	source := r.URL.Query().Get("source") // "all", "builtin", "custom"

	filters := services.EvaluatorFilters{
		Limit:    int32(limit),
		Offset:   int32(offset),
		Tags:     tags,
		Search:   search,
		Provider: provider,
		Source:   source,
	}

	// Call service
	evaluators, total, err := c.evaluatorService.ListEvaluators(ctx, orgName, filters)
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

	orgName := r.PathValue(utils.PathParamOrgName)

	// Extract and URL-decode evaluator identifier
	evaluatorID := r.PathValue(utils.PathParamEvaluatorId)
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

	// Call service
	evaluator, err := c.evaluatorService.GetEvaluator(ctx, orgName, decodedID)
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

// ListLLMProviders handles GET /orgs/{orgName}/evaluators/llm-providers
func (c *evaluatorController) ListLLMProviders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	providers := catalog.AllProviders()
	list := make([]spec.EvaluatorLLMProvider, 0, len(providers))
	for _, p := range providers {
		fields := make([]spec.LLMConfigField, 0, len(p.ConfigFields))
		for _, f := range p.ConfigFields {
			fields = append(fields, spec.LLMConfigField{
				Key:       f.Key,
				Label:     f.Label,
				FieldType: f.FieldType,
				Required:  f.Required,
				EnvVar:    f.EnvVar,
			})
		}
		list = append(list, spec.EvaluatorLLMProvider{
			Name:         p.Name,
			DisplayName:  p.DisplayName,
			ConfigFields: fields,
			Models:       p.Models,
		})
	}

	response := spec.EvaluatorLLMProviderListResponse{
		Count: int32(len(list)),
		List:  list,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode LLM providers response", "error", err)
	}
}

// CreateCustomEvaluator handles POST /orgs/{orgName}/evaluators/custom
func (c *evaluatorController) CreateCustomEvaluator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)

	// Limit request body to 1MB to prevent resource exhaustion
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var specReq spec.CreateCustomEvaluatorRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Trim whitespace from string fields
	specReq.DisplayName = strings.TrimSpace(specReq.DisplayName)
	specReq.Source = strings.TrimSpace(specReq.Source)

	// Basic validation
	if specReq.DisplayName == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Display name is required")
		return
	}
	if specReq.Type != models.CustomEvaluatorTypeCode && specReq.Type != models.CustomEvaluatorTypeLLMJudge {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Type must be 'code' or 'llm_judge'")
		return
	}
	if specReq.Level != "trace" && specReq.Level != "agent" && specReq.Level != "llm" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Level must be 'trace', 'agent', or 'llm'")
		return
	}
	if specReq.Source == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Source is required")
		return
	}

	req := convertSpecCreateRequest(&specReq)
	evaluator, err := c.evaluatorService.CreateCustomEvaluator(ctx, orgName, req)
	if err != nil {
		if errors.Is(err, utils.ErrCustomEvaluatorAlreadyExists) {
			utils.WriteErrorResponse(w, http.StatusConflict, "Custom evaluator with this identifier already exists")
			return
		}
		if errors.Is(err, utils.ErrCustomEvaluatorIdentifierTaken) {
			utils.WriteErrorResponse(w, http.StatusConflict, "Identifier conflicts with a built-in evaluator")
			return
		}
		if errors.Is(err, utils.ErrInvalidInput) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error("Failed to create custom evaluator", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to create custom evaluator")
		return
	}

	response := convertToSpecEvaluatorResponse(evaluator)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// GetCustomEvaluator handles GET /orgs/{orgName}/evaluators/custom/{identifier}
func (c *evaluatorController) GetCustomEvaluator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	identifier := r.PathValue("identifier")
	if identifier == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Identifier is required")
		return
	}

	decodedIdentifier, err := url.PathUnescape(identifier)
	if err != nil {
		log.Warn("Failed to decode identifier", "identifier", identifier, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid identifier")
		return
	}

	identifier = decodedIdentifier

	evaluator, err := c.evaluatorService.GetCustomEvaluator(ctx, orgName, identifier)
	if err != nil {
		if errors.Is(err, utils.ErrCustomEvaluatorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Custom evaluator not found")
			return
		}
		log.Error("Failed to get custom evaluator", "identifier", identifier, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get custom evaluator")
		return
	}

	response := convertToSpecEvaluatorResponse(evaluator)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// UpdateCustomEvaluator handles PUT /orgs/{orgName}/evaluators/custom/{identifier}
func (c *evaluatorController) UpdateCustomEvaluator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	identifier := r.PathValue("identifier")
	if identifier == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Identifier is required")
		return
	}

	decodedIdentifier, err := url.PathUnescape(identifier)
	if err != nil {
		log.Warn("Failed to decode identifier", "identifier", identifier, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid identifier")
		return
	}
	identifier = decodedIdentifier

	// Limit request body to 1MB to prevent resource exhaustion
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)

	var specReq spec.UpdateCustomEvaluatorRequest
	if err := json.NewDecoder(r.Body).Decode(&specReq); err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if specReq.DisplayName != nil && strings.TrimSpace(*specReq.DisplayName) == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Display name cannot be empty")
		return
	}
	if specReq.Source != nil && strings.TrimSpace(*specReq.Source) == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Source cannot be empty")
		return
	}

	req := convertSpecUpdateRequest(&specReq)
	evaluator, err := c.evaluatorService.UpdateCustomEvaluator(ctx, orgName, identifier, req)
	if err != nil {
		if errors.Is(err, utils.ErrCustomEvaluatorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Custom evaluator not found")
			return
		}
		if errors.Is(err, utils.ErrInvalidInput) {
			utils.WriteErrorResponse(w, http.StatusBadRequest, err.Error())
			return
		}
		log.Error("Failed to update custom evaluator", "identifier", identifier, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update custom evaluator")
		return
	}

	response := convertToSpecEvaluatorResponse(evaluator)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// DeleteCustomEvaluator handles DELETE /orgs/{orgName}/evaluators/custom/{identifier}
func (c *evaluatorController) DeleteCustomEvaluator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := r.PathValue(utils.PathParamOrgName)
	identifier := r.PathValue("identifier")
	if identifier == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Identifier is required")
		return
	}

	decodedIdentifier, err := url.PathUnescape(identifier)
	if err != nil {
		log.Warn("Failed to decode identifier", "identifier", identifier, "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid identifier")
		return
	}
	identifier = decodedIdentifier

	if err := c.evaluatorService.DeleteCustomEvaluator(ctx, orgName, identifier); err != nil {
		if errors.Is(err, utils.ErrCustomEvaluatorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Custom evaluator not found")
			return
		}
		if errors.Is(err, utils.ErrCustomEvaluatorInUse) {
			utils.WriteErrorResponse(w, http.StatusConflict, "Custom evaluator is referenced by one or more active monitors and cannot be deleted")
			return
		}
		log.Error("Failed to delete custom evaluator", "identifier", identifier, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete custom evaluator")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// convertSpecCreateRequest converts spec.CreateCustomEvaluatorRequest to models.CreateCustomEvaluatorRequest
func convertSpecCreateRequest(specReq *spec.CreateCustomEvaluatorRequest) *models.CreateCustomEvaluatorRequest {
	var identifier string
	if specReq.Identifier != nil {
		identifier = *specReq.Identifier
	}

	var description string
	if specReq.Description != nil {
		description = *specReq.Description
	}

	return &models.CreateCustomEvaluatorRequest{
		Identifier:   identifier,
		DisplayName:  specReq.DisplayName,
		Description:  description,
		Type:         specReq.Type,
		Level:        specReq.Level,
		Source:       specReq.Source,
		ConfigSchema: convertSpecConfigParams(specReq.ConfigSchema),
		Tags:         specReq.Tags,
	}
}

// convertSpecUpdateRequest converts spec.UpdateCustomEvaluatorRequest to models.UpdateCustomEvaluatorRequest
func convertSpecUpdateRequest(specReq *spec.UpdateCustomEvaluatorRequest) *models.UpdateCustomEvaluatorRequest {
	req := &models.UpdateCustomEvaluatorRequest{
		DisplayName: specReq.DisplayName,
		Description: specReq.Description,
		Source:      specReq.Source,
	}

	if specReq.ConfigSchema != nil {
		converted := convertSpecConfigParams(specReq.ConfigSchema)
		req.ConfigSchema = &converted
	}

	if specReq.Tags != nil {
		tags := specReq.Tags
		req.Tags = &tags
	}

	return req
}

// convertSpecConfigParams converts []spec.EvaluatorConfigParam to []models.EvaluatorConfigParam
func convertSpecConfigParams(specParams []spec.EvaluatorConfigParam) []models.EvaluatorConfigParam {
	if specParams == nil {
		return nil
	}
	result := make([]models.EvaluatorConfigParam, len(specParams))
	for i, p := range specParams {
		result[i] = models.EvaluatorConfigParam{
			Key:         p.Key,
			Type:        p.Type,
			Description: p.Description,
			Required:    p.Required,
			Default:     p.Default,
			Min:         p.Min,
			Max:         p.Max,
			EnumValues:  p.EnumValues,
		}
	}
	return result
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

	resp := spec.EvaluatorResponse{
		Id:           evaluator.ID.String(),
		Identifier:   evaluator.Identifier,
		DisplayName:  evaluator.DisplayName,
		Description:  evaluator.Description,
		Version:      evaluator.Version,
		Provider:     evaluator.Provider,
		Level:        evaluator.Level,
		Tags:         evaluator.Tags,
		IsBuiltin:    evaluator.IsBuiltin,
		ConfigSchema: configFields,
	}

	// Only set custom evaluator fields when present (avoids leaking empty fields for built-ins)
	if evaluator.Type != "" {
		resp.SetType(evaluator.Type)
	}
	if evaluator.Source != "" {
		resp.SetSource(evaluator.Source)
	}
	return resp
}
