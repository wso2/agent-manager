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

package handlers

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/ai-agent-management-platform/traces-observer-service/controllers"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/middleware/logger"
)

const (
	v2DefaultLimit = 10
	v2MaxLimit     = 1000
)

// V2Handler handles HTTP requests for the v2 observer-backed tracing API.
type V2Handler struct {
	controller *controllers.V2TracingController
}

// NewV2Handler creates a new V2Handler.
func NewV2Handler(controller *controllers.V2TracingController) *V2Handler {
	return &V2Handler{controller: controller}
}

// GetTraceOverviews handles GET /api/v2/traces
func (h *V2Handler) GetTraceOverviews(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	query := r.URL.Query()

	namespace := query.Get("namespace")
	if namespace == "" {
		writeV2Error(w, http.StatusBadRequest, "namespace is required")
		return
	}

	startTime, err := parseRFC3339(query.Get("startTime"))
	if err != nil {
		writeV2Error(w, http.StatusBadRequest, fmt.Sprintf("invalid startTime: %v", err))
		return
	}

	endTime, err := parseRFC3339(query.Get("endTime"))
	if err != nil {
		writeV2Error(w, http.StatusBadRequest, fmt.Sprintf("invalid endTime: %v", err))
		return
	}

	limit, err := parseLimit(query.Get("limit"), v2DefaultLimit, v2MaxLimit)
	if err != nil {
		writeV2Error(w, http.StatusBadRequest, err.Error())
		return
	}

	sortOrder, err := parseSortOrder(query.Get("sortOrder"), "desc")
	if err != nil {
		writeV2Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := controllers.V2TraceQueryParams{
		Namespace:   namespace,
		Project:     optionalStr(query.Get("project")),
		Component:   optionalStr(query.Get("component")),
		Environment: optionalStr(query.Get("environment")),
		StartTime:   startTime,
		EndTime:     endTime,
		Limit:       limit,
		SortOrder:   sortOrder,
	}

	result, err := h.controller.GetTraceOverviews(r.Context(), params)
	if err != nil {
		log.Error("Failed to get v2 trace overviews", "error", err)
		writeV2Error(w, http.StatusInternalServerError, "Failed to retrieve trace overviews")
		return
	}

	writeV2JSON(w, http.StatusOK, result)
}

// GetTraceSpans handles GET /api/v2/traces/{traceId}/spans
func (h *V2Handler) GetTraceSpans(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	traceID := pathSegment(r.URL.Path, "/api/v2/traces/", "/spans")
	if traceID == "" {
		writeV2Error(w, http.StatusBadRequest, "traceId is required")
		return
	}

	query := r.URL.Query()

	namespace := query.Get("namespace")
	if namespace == "" {
		writeV2Error(w, http.StatusBadRequest, "namespace is required")
		return
	}

	startTime, err := parseRFC3339(query.Get("startTime"))
	if err != nil {
		writeV2Error(w, http.StatusBadRequest, fmt.Sprintf("invalid startTime: %v", err))
		return
	}

	endTime, err := parseRFC3339(query.Get("endTime"))
	if err != nil {
		writeV2Error(w, http.StatusBadRequest, fmt.Sprintf("invalid endTime: %v", err))
		return
	}

	limit, err := parseLimit(query.Get("limit"), v2DefaultLimit, v2MaxLimit)
	if err != nil {
		writeV2Error(w, http.StatusBadRequest, err.Error())
		return
	}

	sortOrder, err := parseSortOrder(query.Get("sortOrder"), "asc")
	if err != nil {
		writeV2Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := controllers.V2TraceQueryParams{
		Namespace:   namespace,
		Project:     optionalStr(query.Get("project")),
		Component:   optionalStr(query.Get("component")),
		Environment: optionalStr(query.Get("environment")),
		StartTime:   startTime,
		EndTime:     endTime,
		Limit:       limit,
		SortOrder:   sortOrder,
	}

	result, err := h.controller.GetTraceSpans(r.Context(), traceID, params)
	if err != nil {
		log.Error("Failed to get v2 trace spans", "traceId", traceID, "error", err)
		writeV2Error(w, http.StatusInternalServerError, "Failed to retrieve trace spans")
		return
	}

	writeV2JSON(w, http.StatusOK, result)
}

// ExportTraces handles GET /api/v2/traces/export
// namespace, project, component, and environment are all required to scope the
// export to a specific component — mirroring v1's componentUid + environmentUid.
func (h *V2Handler) ExportTraces(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	query := r.URL.Query()

	namespace := query.Get("namespace")
	if namespace == "" {
		writeV2Error(w, http.StatusBadRequest, "namespace is required")
		return
	}

	project := query.Get("project")
	if project == "" {
		writeV2Error(w, http.StatusBadRequest, "project is required")
		return
	}

	component := query.Get("component")
	if component == "" {
		writeV2Error(w, http.StatusBadRequest, "component is required")
		return
	}

	environment := query.Get("environment")
	if environment == "" {
		writeV2Error(w, http.StatusBadRequest, "environment is required")
		return
	}

	startTime, err := parseRFC3339(query.Get("startTime"))
	if err != nil {
		writeV2Error(w, http.StatusBadRequest, fmt.Sprintf("invalid startTime: %v", err))
		return
	}

	endTime, err := parseRFC3339(query.Get("endTime"))
	if err != nil {
		writeV2Error(w, http.StatusBadRequest, fmt.Sprintf("invalid endTime: %v", err))
		return
	}

	limit, err := parseLimit(query.Get("limit"), 100, v2MaxLimit)
	if err != nil {
		writeV2Error(w, http.StatusBadRequest, err.Error())
		return
	}

	sortOrder, err := parseSortOrder(query.Get("sortOrder"), "desc")
	if err != nil {
		writeV2Error(w, http.StatusBadRequest, err.Error())
		return
	}

	params := controllers.V2TraceQueryParams{
		Namespace:   namespace,
		Project:     &project,
		Component:   &component,
		Environment: &environment,
		StartTime:   startTime,
		EndTime:     endTime,
		Limit:       limit,
		SortOrder:   sortOrder,
	}

	result, err := h.controller.ExportTraces(r.Context(), params)
	if err != nil {
		log.Error("Failed to export v2 traces", "error", err)
		writeV2Error(w, http.StatusInternalServerError, "Failed to export traces")
		return
	}

	timestamp := time.Now().Format("20060102-150405")
	filename := fmt.Sprintf("traces-export-%s.json", timestamp)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Expires", "0")

	writeV2JSON(w, http.StatusOK, result)
}

// GetSpanDetail handles GET /api/v2/traces/{traceId}/spans/{spanId}
func (h *V2Handler) GetSpanDetail(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	traceID, spanID := parseTraceSpanIDs(r.URL.Path)
	if traceID == "" || spanID == "" {
		writeV2Error(w, http.StatusBadRequest, "traceId and spanId are required")
		return
	}

	result, err := h.controller.GetSpanDetail(r.Context(), traceID, spanID)
	if err != nil {
		log.Error("Failed to get v2 span detail", "traceId", traceID, "spanId", spanID, "error", err)
		writeV2Error(w, http.StatusInternalServerError, "Failed to retrieve span detail")
		return
	}

	writeV2JSON(w, http.StatusOK, result)
}

// parseTraceSpanIDs extracts traceId and spanId from
// /api/v2/traces/{traceId}/spans/{spanId}
func parseTraceSpanIDs(path string) (traceID, spanID string) {
	const prefix = "/api/v2/traces/"
	const middle = "/spans/"
	after, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return "", ""
	}
	idx := strings.Index(after, middle)
	if idx < 0 {
		return "", ""
	}
	traceID = after[:idx]
	spanID = after[idx+len(middle):]
	if strings.Contains(spanID, "/") {
		spanID = ""
	}
	return traceID, spanID
}

// pathSegment extracts the path segment between prefix and suffix.
// e.g. prefix="/api/v2/traces/", suffix="/spans" from "/api/v2/traces/abc/spans"
func pathSegment(path, prefix, suffix string) string {
	after, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return ""
	}
	idx := strings.Index(after, suffix)
	if idx < 0 {
		return ""
	}
	return after[:idx]
}

func parseRFC3339(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("value is required")
	}
	return time.Parse(time.RFC3339, s)
}

func parseLimit(s string, defaultVal, maxVal int) (int, error) {
	if s == "" {
		return defaultVal, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return 0, fmt.Errorf("limit must be a positive integer")
	}
	if v > maxVal {
		v = maxVal
	}
	return v, nil
}

func parseSortOrder(s, defaultVal string) (string, error) {
	if s == "" {
		return defaultVal, nil
	}
	if s != "asc" && s != "desc" {
		return "", fmt.Errorf("sortOrder must be 'asc' or 'desc'")
	}
	return s, nil
}

func optionalStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func writeV2JSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode JSON", "error", err)
	}
}

func writeV2Error(w http.ResponseWriter, status int, message string) {
	writeV2JSON(w, status, ErrorResponse{Error: "error", Message: message})
}
