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
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/wso2/agent-manager/agent-manager-observer/controllers"
	"github.com/wso2/agent-manager/agent-manager-observer/middleware/logger"
)

const (
	defaultLimit = 10
	maxLimit     = 1000

	// maxLogLimit is the maximum number of log lines that can be requested
	// from /api/v1/logs, matching the observability service's own cap.
	maxLogLimit = 10000
	// maxLogRangeDays is the maximum startTime..endTime span accepted by
	// /api/v1/logs, ported from agent-manager-service/utils/constants.go.
	maxLogRangeDays = 14
)

// validLogLevels are the log levels accepted (case-insensitively) by the
// logLevels query parameter on /api/v1/logs.
var validLogLevels = map[string]bool{
	"INFO":  true,
	"DEBUG": true,
	"WARN":  true,
	"ERROR": true,
}

// ErrorResponse is the standard error body for all endpoints.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

// Handler handles HTTP requests for the tracing and observability APIs.
type Handler struct {
	controller *controllers.TracingController
	obs        *controllers.ObservabilityController
}

// NewHandler creates a new Handler.
func NewHandler(controller *controllers.TracingController, obs *controllers.ObservabilityController) *Handler {
	return &Handler{controller: controller, obs: obs}
}

// GetTraceOverviews handles GET /api/v1/traces
func (h *Handler) GetTraceOverviews(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	query := r.URL.Query()

	organization := query.Get("organization")
	if organization == "" {
		writeError(w, http.StatusBadRequest, "organization is required")
		return
	}

	project := query.Get("project")
	if project == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	agent := query.Get("agent")
	if agent == "" {
		writeError(w, http.StatusBadRequest, "agent is required")
		return
	}

	environment := query.Get("environment")
	if environment == "" {
		writeError(w, http.StatusBadRequest, "environment is required")
		return
	}

	startTime, err := parseRFC3339(query.Get("startTime"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid startTime: %v", err))
		return
	}

	endTime, err := parseRFC3339(query.Get("endTime"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid endTime: %v", err))
		return
	}

	limit, err := parseLimit(query.Get("limit"), defaultLimit, maxLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sortOrder, err := parseSortOrder(query.Get("sortOrder"), "desc")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	include, err := parseInclude(query["include"])
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	filters, err := parseTraceFilters(query)
	if err != nil {
		log.Info("Rejected trace filter", "organization", organization, "error", err)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var cursor *controllers.TraceCursor
	if raw := query.Get("cursor"); raw != "" {
		if cursor, err = controllers.DecodeTraceCursor(raw); err != nil {
			log.Info("Rejected trace list cursor", "organization", organization, "error", err)
			writeError(w, http.StatusBadRequest, "invalid cursor")
			return
		}
	}

	params := controllers.TraceQueryParams{
		Organization: organization,
		Project:      &project,
		Agent:        &agent,
		Environment:  &environment,
		StartTime:    startTime,
		EndTime:      endTime,
		Limit:        limit,
		SortOrder:    sortOrder,
		Include:      include,
		Filters:      filters,
		Cursor:       cursor,
	}

	result, err := h.controller.GetTraceOverviews(r.Context(), params)
	if err != nil {
		log.Error("Failed to get trace overviews", "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to retrieve trace overviews")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetTraceSpans handles GET /api/v1/traces/{traceId}/spans
func (h *Handler) GetTraceSpans(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	traceID := pathSegment(r.URL.Path, "/api/v1/traces/", "/spans")
	if traceID == "" {
		writeError(w, http.StatusBadRequest, "traceId is required")
		return
	}

	query := r.URL.Query()

	organization := query.Get("organization")
	if organization == "" {
		writeError(w, http.StatusBadRequest, "organization is required")
		return
	}

	startTime, err := parseRFC3339(query.Get("startTime"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid startTime: %v", err))
		return
	}

	endTime, err := parseRFC3339(query.Get("endTime"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid endTime: %v", err))
		return
	}

	limit, err := parseLimit(query.Get("limit"), defaultLimit, maxLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sortOrder, err := parseSortOrder(query.Get("sortOrder"), "asc")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	params := controllers.TraceQueryParams{
		Organization: organization,
		Project:      optionalStr(query.Get("project")),
		Agent:        optionalStr(query.Get("agent")),
		Environment:  optionalStr(query.Get("environment")),
		StartTime:    startTime,
		EndTime:      endTime,
		Limit:        limit,
		SortOrder:    sortOrder,
	}

	result, err := h.controller.GetTraceSpans(r.Context(), traceID, params)
	if err != nil {
		log.Error("Failed to get v1 trace spans", "traceId", traceID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to retrieve trace spans")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// ExportTraces handles GET /api/v1/traces/export
// namespace, project, component, and environment are all required to scope the
// export to a specific component — mirroring v1's componentUid + environmentUid.
func (h *Handler) ExportTraces(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())
	query := r.URL.Query()

	organization := query.Get("organization")
	if organization == "" {
		writeError(w, http.StatusBadRequest, "organization is required")
		return
	}

	project := query.Get("project")
	if project == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	agent := query.Get("agent")
	if agent == "" {
		writeError(w, http.StatusBadRequest, "agent is required")
		return
	}

	environment := query.Get("environment")
	if environment == "" {
		writeError(w, http.StatusBadRequest, "environment is required")
		return
	}

	startTime, err := parseRFC3339(query.Get("startTime"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid startTime: %v", err))
		return
	}

	endTime, err := parseRFC3339(query.Get("endTime"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid endTime: %v", err))
		return
	}

	limit, err := parseLimit(query.Get("limit"), 100, maxLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sortOrder, err := parseSortOrder(query.Get("sortOrder"), "desc")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	params := controllers.TraceQueryParams{
		Organization: organization,
		Project:      &project,
		Agent:        &agent,
		Environment:  &environment,
		StartTime:    startTime,
		EndTime:      endTime,
		Limit:        limit,
		SortOrder:    sortOrder,
	}

	result, err := h.controller.ExportTraces(r.Context(), params)
	if err != nil {
		log.Error("Failed to export v1 traces", "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to export traces")
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
	writeJSON(w, http.StatusOK, result)
}

// GetSpanDetail handles GET /api/v1/traces/{traceId}/spans/{spanId}
func (h *Handler) GetSpanDetail(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	traceID, spanID := parseTraceSpanIDs(r.URL.Path)
	if traceID == "" || spanID == "" {
		writeError(w, http.StatusBadRequest, "traceId and spanId are required")
		return
	}

	result, err := h.controller.GetSpanDetail(r.Context(), traceID, spanID)
	if err != nil {
		log.Error("Failed to get v1 span detail", "traceId", traceID, "spanId", spanID, "error", err)
		writeError(w, http.StatusInternalServerError, "Failed to retrieve span detail")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetLogs handles GET /api/v1/logs
func (h *Handler) GetLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	log := logger.GetLogger(r.Context())
	query := r.URL.Query()

	organization := query.Get("organization")
	if organization == "" {
		writeError(w, http.StatusBadRequest, "organization is required")
		return
	}

	project := query.Get("project")
	if project == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	agent := query.Get("agent")
	if agent == "" {
		writeError(w, http.StatusBadRequest, "agent is required")
		return
	}

	environment := query.Get("environment")
	if environment == "" {
		writeError(w, http.StatusBadRequest, "environment is required")
		return
	}

	startTime, err := parseRFC3339(query.Get("startTime"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid startTime: %v", err))
		return
	}

	endTime, err := parseRFC3339(query.Get("endTime"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid endTime: %v", err))
		return
	}

	if err := validateLogTimeRange(startTime, endTime); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	logLevels := parseLogLevels(query.Get("logLevels"))

	limit, err := parseOptionalLimit(query.Get("limit"), maxLogLimit)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sortOrder, err := parseSortOrder(query.Get("sortOrder"), "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	params := controllers.LogsQueryParams{
		Organization: organization,
		Project:      project,
		Agent:        agent,
		Environment:  environment,
		StartTime:    startTime,
		EndTime:      endTime,
		SearchPhrase: query.Get("searchPhrase"),
		LogLevels:    logLevels,
		Limit:        limit,
		SortOrder:    sortOrder,
	}

	result, err := h.obs.GetLogs(r.Context(), params)
	if err != nil {
		log.Error("Failed to get logs", "error", err)
		writeError(w, http.StatusBadGateway, "failed to query upstream observer")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetBuildLogs handles GET /api/v1/build-logs
func (h *Handler) GetBuildLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	log := logger.GetLogger(r.Context())
	query := r.URL.Query()

	organization := query.Get("organization")
	if organization == "" {
		writeError(w, http.StatusBadRequest, "organization is required")
		return
	}

	buildName := query.Get("buildName")
	if buildName == "" {
		writeError(w, http.StatusBadRequest, "buildName is required")
		return
	}

	result, err := h.obs.GetBuildLogs(r.Context(), organization, buildName)
	if err != nil {
		log.Error("Failed to get build logs", "buildName", buildName, "error", err)
		writeError(w, http.StatusBadGateway, "failed to query upstream observer")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// GetMetrics handles GET /api/v1/metrics
func (h *Handler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	log := logger.GetLogger(r.Context())
	query := r.URL.Query()

	organization := query.Get("organization")
	if organization == "" {
		writeError(w, http.StatusBadRequest, "organization is required")
		return
	}

	project := query.Get("project")
	if project == "" {
		writeError(w, http.StatusBadRequest, "project is required")
		return
	}

	agent := query.Get("agent")
	if agent == "" {
		writeError(w, http.StatusBadRequest, "agent is required")
		return
	}

	environment := query.Get("environment")
	if environment == "" {
		writeError(w, http.StatusBadRequest, "environment is required")
		return
	}

	startTime, err := parseRFC3339(query.Get("startTime"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid startTime: %v", err))
		return
	}

	endTime, err := parseRFC3339(query.Get("endTime"))
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid endTime: %v", err))
		return
	}

	params := controllers.MetricsQueryParams{
		Organization: organization,
		Project:      project,
		Agent:        agent,
		Environment:  environment,
		StartTime:    startTime,
		EndTime:      endTime,
	}

	result, err := h.obs.GetMetrics(r.Context(), params)
	if err != nil {
		log.Error("Failed to get metrics", "error", err)
		writeError(w, http.StatusBadGateway, "failed to query upstream observer")
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// parseTraceSpanIDs extracts traceId and spanId from
// /api/v1/traces/{traceId}/spans/{spanId}
func parseTraceSpanIDs(path string) (traceID, spanID string) {
	const prefix = "/api/v1/traces/"
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
	if strings.Contains(traceID, "/") || strings.Contains(spanID, "/") {
		return "", ""
	}
	return traceID, spanID
}

// pathSegment extracts the path segment between prefix and suffix.
// e.g. prefix="/api/v1/traces/", suffix="/spans" from "/api/v1/traces/abc/spans"
func pathSegment(path, prefix, suffix string) string {
	after, ok := strings.CutPrefix(path, prefix)
	if !ok {
		return ""
	}
	idx := strings.Index(after, suffix)
	if idx < 0 {
		return ""
	}
	seg := after[:idx]
	if strings.Contains(seg, "/") {
		return ""
	}
	return seg
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

// parseInclude parses the comma-separated include query parameter of
// GET /api/v1/traces; unknown values are an error.
func parseInclude(raw []string) (controllers.Include, error) {
	var include controllers.Include
	for _, list := range raw {
		for _, item := range strings.Split(list, ",") {
			switch v := strings.TrimSpace(item); v {
			case "":
			case "models":
				include.Models = true
			default:
				return controllers.Include{}, fmt.Errorf("invalid include value %q: must be one of 'models'", v)
			}
		}
	}
	return include, nil
}

// parseTraceFilters parses the optional filter params of GET /api/v1/traces.
func parseTraceFilters(query url.Values) (controllers.TraceFilters, error) {
	var f controllers.TraceFilters

	switch status := controllers.TraceStatusFilter(query.Get("status")); status {
	case controllers.TraceStatusAny, controllers.TraceStatusError, controllers.TraceStatusOK:
		f.Status = status
	default:
		return controllers.TraceFilters{}, fmt.Errorf("status must be 'error' or 'ok'")
	}

	var err error
	if f.MinDurationMs, err = parseMinThreshold("minDurationMs", query.Get("minDurationMs")); err != nil {
		return controllers.TraceFilters{}, err
	}
	if f.MinTokens, err = parseMinThreshold("minTokens", query.Get("minTokens")); err != nil {
		return controllers.TraceFilters{}, err
	}
	if f.MinSpanCount, err = parseMinThreshold("minSpanCount", query.Get("minSpanCount")); err != nil {
		return controllers.TraceFilters{}, err
	}

	f.Model = query.Get("model")
	f.ConversationID = query.Get("conversationId")
	return f, nil
}

func parseMinThreshold(name, s string) (*int64, error) {
	if s == "" {
		return nil, nil
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil || v < 0 {
		return nil, fmt.Errorf("%s must be a non-negative integer", name)
	}
	return &v, nil
}

// validateLogTimeRange ports validateTimes from
// agent-manager-service/utils/utils.go: startTime must not be in the future,
// endTime must not be before startTime, and the range must not exceed
// maxLogRangeDays. Format validation is already handled by parseRFC3339.
func validateLogTimeRange(start, end time.Time) error {
	if start.After(time.Now()) {
		return fmt.Errorf("startTime cannot be in the future")
	}
	if end.Before(start) {
		return fmt.Errorf("endTime must not be before startTime")
	}
	maxDuration := time.Duration(maxLogRangeDays) * 24 * time.Hour
	if end.Sub(start) > maxDuration {
		return fmt.Errorf("time range cannot exceed %d days", maxLogRangeDays)
	}
	return nil
}

// parseLogLevels splits a comma-separated logLevels value, uppercases each
// entry, and keeps the ones in the accepted set. It tolerates malformed input
// the way a filter should: empty tokens (from trailing or duplicate commas)
// are skipped, and unknown levels are ignored rather than failing the whole
// request. An empty string — or a value with no recognized levels — yields a
// nil slice (no filtering).
func parseLogLevels(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	levels := make([]string, 0, len(parts))
	for _, p := range parts {
		level := strings.ToUpper(strings.TrimSpace(p))
		if level == "" {
			continue
		}
		if !validLogLevels[level] {
			slog.Debug("ignoring unknown logLevels value", "value", p)
			continue
		}
		levels = append(levels, level)
	}
	if len(levels) == 0 {
		return nil
	}
	return levels
}

// parseOptionalLimit parses an optional limit query parameter for /api/v1/logs:
// empty input returns a nil pointer (upstream default applies), otherwise the
// value must be a positive integer not exceeding maxVal.
func parseOptionalLimit(s string, maxVal int) (*int, error) {
	if s == "" {
		return nil, nil
	}
	v, err := strconv.Atoi(s)
	if err != nil || v <= 0 {
		return nil, fmt.Errorf("limit must be a positive integer")
	}
	if v > maxVal {
		return nil, fmt.Errorf("limit must not exceed %d", maxVal)
	}
	return &v, nil
}

func optionalStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		slog.Error("Failed to encode JSON", "error", err)
	}
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: "error", Message: message})
}
