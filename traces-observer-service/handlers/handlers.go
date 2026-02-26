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

package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/wso2/ai-agent-management-platform/traces-observer-service/controllers"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/opensearch"
)

// Handler handles HTTP requests for tracing
type Handler struct {
	controllers *controllers.TracingController
}

// NewHandler creates a new handler
func NewHandler(controllers *controllers.TracingController) *Handler {
	return &Handler{
		controllers: controllers,
	}
}

// TraceRequest represents the request body for getting traces
type TraceRequest struct {
	ComponentUid   string `json:"componentUid"`
	EnvironmentUid string `json:"environmentUid"`
	StartTime      string `json:"startTime"`
	EndTime        string `json:"endTime"`
	Limit          int    `json:"limit,omitempty"`
	SortOrder      string `json:"sortOrder,omitempty"`
}

// TraceByIdAndServiceRequest represents the request body for getting traces by ID and component
type TraceByIdAndServiceRequest struct {
	TraceID        string `json:"traceId"`
	ComponentUid   string `json:"componentUid"`
	EnvironmentUid string `json:"environmentUid"`
	SortOrder      string `json:"sortOrder,omitempty"`
	Limit          int    `json:"limit,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// GetTraceOverviews handles GET /api/traces with query parameters
func (h *Handler) GetTraceOverviews(w http.ResponseWriter, r *http.Request) {
	// Get logger from context
	log := logger.GetLogger(r.Context())

	// Parse query parameters
	query := r.URL.Query()

	componentUid := query.Get("componentUid")
	if componentUid == "" {
		h.writeError(w, http.StatusBadRequest, "componentUid is required")
		return
	}

	environmentUid := query.Get("environmentUid")
	if environmentUid == "" {
		h.writeError(w, http.StatusBadRequest, "environmentUid is required")
		return
	}

	startTime := query.Get("startTime")
	endTime := query.Get("endTime")

	// Parse limit (default: 10)
	limit := 10
	if limitStr := query.Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			h.writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsedLimit
	}

	// Parse offset for pagination (default: 0)
	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err != nil || parsedOffset < 0 {
			h.writeError(w, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}
		offset = parsedOffset
	}

	// Parse sortOrder (default: desc for traces - newest first)
	sortOrder := query.Get("sortOrder")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		h.writeError(w, http.StatusBadRequest, "sortOrder must be 'asc' or 'desc'")
		return
	}

	// Build query parameters
	params := opensearch.TraceQueryParams{
		ComponentUid:   componentUid,
		EnvironmentUid: environmentUid,
		StartTime:      startTime,
		EndTime:        endTime,
		Limit:          limit,
		Offset:         offset,
		SortOrder:      sortOrder,
	}

	// Execute query
	ctx := r.Context()
	result, err := h.controllers.GetTraceOverviews(ctx, params)
	if err != nil {
		log.Error("Failed to get trace overviews", "error", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to retrieve trace overviews")
		return
	}

	// Write response
	h.writeJSON(w, http.StatusOK, result)
}

// GetTraceByIdAndService handles GET /api/trace with query parameters
func (h *Handler) GetTraceByIdAndService(w http.ResponseWriter, r *http.Request) {
	// Get logger from context
	log := logger.GetLogger(r.Context())

	// Parse query parameters
	query := r.URL.Query()

	traceID := query.Get("traceId")
	if traceID == "" {
		h.writeError(w, http.StatusBadRequest, "traceId is required")
		return
	}

	componentUid := query.Get("componentUid")
	if componentUid == "" {
		h.writeError(w, http.StatusBadRequest, "componentUid is required")
		return
	}

	environmentUid := query.Get("environmentUid")
	if environmentUid == "" {
		h.writeError(w, http.StatusBadRequest, "environmentUid is required")
		return
	}

	// Parse sortOrder (default: desc)
	sortOrder := query.Get("sortOrder")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		h.writeError(w, http.StatusBadRequest, "sortOrder must be 'asc' or 'desc'")
		return
	}

	// Parse limit (default: 100 for spans)
	limit := 100
	if limitStr := query.Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			h.writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsedLimit
	}

	// Build query parameters
	params := opensearch.TraceByIdAndServiceParams{
		TraceID:        traceID,
		ComponentUid:   componentUid,
		EnvironmentUid: environmentUid,
		SortOrder:      sortOrder,
		Limit:          limit,
	}

	// Execute query
	ctx := r.Context()
	result, err := h.controllers.GetTraceByIdAndService(ctx, params)
	if err != nil {
		// Check if it's a "not found" error
		if errors.Is(err, controllers.ErrTraceNotFound) {
			h.writeError(w, http.StatusNotFound, "Trace not found")
			return
		}
		// Other errors are internal server errors
		log.Error("Failed to get trace by ID and service", "error", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to retrieve traces")
		return
	}

	// Write response
	h.writeJSON(w, http.StatusOK, result)
}

// ExportTraces handles GET /api/traces/export with query parameters
func (h *Handler) ExportTraces(w http.ResponseWriter, r *http.Request) {
	// Get logger from context
	log := logger.GetLogger(r.Context())

	// Parse query parameters
	query := r.URL.Query()

	componentUid := query.Get("componentUid")
	if componentUid == "" {
		h.writeError(w, http.StatusBadRequest, "componentUid is required")
		return
	}

	environmentUid := query.Get("environmentUid")
	if environmentUid == "" {
		h.writeError(w, http.StatusBadRequest, "environmentUid is required")
		return
	}

	startTime := query.Get("startTime")
	endTime := query.Get("endTime")

	// Parse limit (default: 100 for export)
	limit := 100
	if limitStr := query.Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			h.writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		// Cap at MaxTracesPerRequest to prevent excessive data export
		if parsedLimit > controllers.MaxTracesPerRequest {
			parsedLimit = controllers.MaxTracesPerRequest
		}
		limit = parsedLimit
	}

	// Parse offset for pagination (default: 0)
	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err != nil || parsedOffset < 0 {
			h.writeError(w, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}
		offset = parsedOffset
	}

	// Parse sortOrder (default: desc for traces - newest first)
	sortOrder := query.Get("sortOrder")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		h.writeError(w, http.StatusBadRequest, "sortOrder must be 'asc' or 'desc'")
		return
	}

	// Build query parameters
	params := opensearch.TraceQueryParams{
		ComponentUid:   componentUid,
		EnvironmentUid: environmentUid,
		StartTime:      startTime,
		EndTime:        endTime,
		Limit:          limit,
		Offset:         offset,
		SortOrder:      sortOrder,
	}

	// Execute query
	ctx := r.Context()
	result, err := h.controllers.ExportTraces(ctx, params)
	if err != nil {
		log.Error("Failed to export traces", "error", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to export traces")
		return
	}

	// Set content disposition header to suggest filename
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("traces-export-%s.json", timestamp)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	// Write response
	h.writeJSON(w, http.StatusOK, result)
}

// --- v2 handlers ---

// GetTraceOverviewsV2 handles GET /api/v2/traces with query parameters
// Same interface as v1 but uses OpenSearch aggregations for proper trace-level grouping
func (h *Handler) GetTraceOverviewsV2(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	query := r.URL.Query()

	componentUid := query.Get("componentUid")
	if componentUid == "" {
		h.writeError(w, http.StatusBadRequest, "componentUid is required")
		return
	}

	environmentUid := query.Get("environmentUid")
	if environmentUid == "" {
		h.writeError(w, http.StatusBadRequest, "environmentUid is required")
		return
	}

	startTime := query.Get("startTime")
	endTime := query.Get("endTime")

	limit := 10
	if limitStr := query.Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			h.writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsedLimit
	}

	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err != nil || parsedOffset < 0 {
			h.writeError(w, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}
		offset = parsedOffset
	}

	sortOrder := query.Get("sortOrder")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		h.writeError(w, http.StatusBadRequest, "sortOrder must be 'asc' or 'desc'")
		return
	}

	params := opensearch.TraceQueryParams{
		ComponentUid:   componentUid,
		EnvironmentUid: environmentUid,
		StartTime:      startTime,
		EndTime:        endTime,
		Limit:          limit,
		Offset:         offset,
		SortOrder:      sortOrder,
	}

	ctx := r.Context()
	result, err := h.controllers.GetTraceOverviewsV2(ctx, params)
	if err != nil {
		log.Error("Failed to get trace overviews (v2)", "error", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to retrieve trace overviews")
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// GetTraceByIdAndServiceV2 handles GET /api/v2/trace with query parameters
// Same interface as v1, plus parentSpan filter
func (h *Handler) GetTraceByIdAndServiceV2(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	query := r.URL.Query()

	traceID := query.Get("traceId")
	if traceID == "" {
		h.writeError(w, http.StatusBadRequest, "traceId is required")
		return
	}

	componentUid := query.Get("componentUid")
	if componentUid == "" {
		h.writeError(w, http.StatusBadRequest, "componentUid is required")
		return
	}

	environmentUid := query.Get("environmentUid")
	if environmentUid == "" {
		h.writeError(w, http.StatusBadRequest, "environmentUid is required")
		return
	}

	limit := 1000
	if limitStr := query.Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			h.writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = parsedLimit
	}

	// Parse parentSpan filter (v2 addition)
	parentSpan := false
	if parentSpanStr := query.Get("parentSpan"); parentSpanStr == "true" {
		parentSpan = true
	}

	params := opensearch.V2TraceByIdParams{
		TraceIDs:       []string{traceID},
		ComponentUid:   componentUid,
		EnvironmentUid: environmentUid,
		ParentSpan:     parentSpan,
		Limit:          limit,
	}

	ctx := r.Context()
	result, err := h.controllers.GetTraceByIdV2(ctx, params)
	if err != nil {
		if errors.Is(err, controllers.ErrTraceNotFound) {
			h.writeError(w, http.StatusNotFound, "Trace not found")
			return
		}
		log.Error("Failed to get trace by ID (v2)", "error", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to retrieve traces")
		return
	}

	h.writeJSON(w, http.StatusOK, result)
}

// ExportTracesV2 handles GET /api/v2/traces/export with query parameters
// Same interface as v1 but uses aggregation for proper trace grouping and supports pagination
func (h *Handler) ExportTracesV2(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	query := r.URL.Query()

	componentUid := query.Get("componentUid")
	if componentUid == "" {
		h.writeError(w, http.StatusBadRequest, "componentUid is required")
		return
	}

	environmentUid := query.Get("environmentUid")
	if environmentUid == "" {
		h.writeError(w, http.StatusBadRequest, "environmentUid is required")
		return
	}

	startTime := query.Get("startTime")
	endTime := query.Get("endTime")

	limit := 100
	if limitStr := query.Get("limit"); limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit <= 0 {
			h.writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		if parsedLimit > controllers.MaxTracesPerRequest {
			parsedLimit = controllers.MaxTracesPerRequest
		}
		limit = parsedLimit
	}

	offset := 0
	if offsetStr := query.Get("offset"); offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err != nil || parsedOffset < 0 {
			h.writeError(w, http.StatusBadRequest, "offset must be a non-negative integer")
			return
		}
		offset = parsedOffset
	}

	sortOrder := query.Get("sortOrder")
	if sortOrder == "" {
		sortOrder = "desc"
	}
	if sortOrder != "asc" && sortOrder != "desc" {
		h.writeError(w, http.StatusBadRequest, "sortOrder must be 'asc' or 'desc'")
		return
	}

	params := opensearch.TraceQueryParams{
		ComponentUid:   componentUid,
		EnvironmentUid: environmentUid,
		StartTime:      startTime,
		EndTime:        endTime,
		Limit:          limit,
		Offset:         offset,
		SortOrder:      sortOrder,
	}

	ctx := r.Context()
	result, err := h.controllers.ExportTracesV2(ctx, params)
	if err != nil {
		log.Error("Failed to export traces (v2)", "error", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to export traces")
		return
	}

	// Set content disposition header to suggest filename
	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("traces-export-%s.json", timestamp)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	h.writeJSON(w, http.StatusOK, result)
}

// Health handles GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	// Get logger from context
	log := logger.GetLogger(r.Context())

	ctx := r.Context()
	if err := h.controllers.HealthCheck(ctx); err != nil {
		log.Error("Health check failed", "error", err)
		h.writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"status": "unhealthy",
			"error":  "service unavailable",
		})
		return
	}

	h.writeJSON(w, http.StatusOK, map[string]string{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// Helper functions
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode JSON", "error", err)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error:   "error",
		Message: message,
	})
}
