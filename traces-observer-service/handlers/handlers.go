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
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/wso2/ai-agent-management-platform/traces-observer-service/controllers"
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
	ComponentUid    string `json:"componentUid"`
	ProjectUid      string `json:"projectUid"`
	EnvironmentUid  string `json:"environmentUid"`
	OrganizationUid string `json:"organizationUid,omitempty"`
	StartTime       string `json:"startTime"`
	EndTime         string `json:"endTime"`
	Limit           int    `json:"limit,omitempty"`
	SortOrder       string `json:"sortOrder,omitempty"`
}

// TraceByIdAndServiceRequest represents the request body for getting traces by ID and component
type TraceByIdAndServiceRequest struct {
	TraceID         string `json:"traceId"`
	ComponentUid    string `json:"componentUid"`
	ProjectUid      string `json:"projectUid"`
	EnvironmentUid  string `json:"environmentUid"`
	OrganizationUid string `json:"organizationUid,omitempty"`
	SortOrder       string `json:"sortOrder,omitempty"`
	Limit           int    `json:"limit,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// GetTraceOverviews handles GET /api/traces with query parameters
func (h *Handler) GetTraceOverviews(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()

	componentUid := query.Get("componentUid")
	if componentUid == "" {
		h.writeError(w, http.StatusBadRequest, "componentUid is required")
		return
	}

	projectUid := query.Get("projectUid")
	if projectUid == "" {
		h.writeError(w, http.StatusBadRequest, "projectUid is required")
		return
	}

	environmentUid := query.Get("environmentUid")
	if environmentUid == "" {
		h.writeError(w, http.StatusBadRequest, "environmentUid is required")
		return
	}

	organizationUid := query.Get("organizationUid") // Optional

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
		ComponentUid:    componentUid,
		ProjectUid:      projectUid,
		EnvironmentUid:  environmentUid,
		OrganizationUid: organizationUid,
		StartTime:       startTime,
		EndTime:         endTime,
		Limit:           limit,
		Offset:          offset,
		SortOrder:       sortOrder,
	}

	// Execute query
	ctx := r.Context()
	result, err := h.controllers.GetTraceOverviews(ctx, params)
	if err != nil {
		log.Printf("Failed to get trace overviews: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to retrieve trace overviews")
		return
	}

	// Write response
	h.writeJSON(w, http.StatusOK, result)
}

// GetTraceByIdAndService handles GET /api/trace with query parameters
func (h *Handler) GetTraceByIdAndService(w http.ResponseWriter, r *http.Request) {
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

	projectUid := query.Get("projectUid")
	if projectUid == "" {
		h.writeError(w, http.StatusBadRequest, "projectUid is required")
		return
	}

	environmentUid := query.Get("environmentUid")
	if environmentUid == "" {
		h.writeError(w, http.StatusBadRequest, "environmentUid is required")
		return
	}

	organizationUid := query.Get("organizationUid") // Optional

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
		TraceID:         traceID,
		ComponentUid:    componentUid,
		ProjectUid:      projectUid,
		EnvironmentUid:  environmentUid,
		OrganizationUid: organizationUid,
		SortOrder:       sortOrder,
		Limit:           limit,
	}

	// Execute query
	ctx := r.Context()
	result, err := h.controllers.GetTraceByIdAndService(ctx, params)
	if err != nil {
		log.Printf("Failed to get trace by ID and service: %v", err)
		h.writeError(w, http.StatusInternalServerError, "Failed to retrieve traces")
		return
	}

	// Write response
	h.writeJSON(w, http.StatusOK, result)
}

// Health handles GET /health
func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if err := h.controllers.HealthCheck(ctx); err != nil {
		log.Printf("Health check failed: %v", err)
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
		log.Printf("Failed to encode JSON: %v", err)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, ErrorResponse{
		Error:   "error",
		Message: message,
	})
}
