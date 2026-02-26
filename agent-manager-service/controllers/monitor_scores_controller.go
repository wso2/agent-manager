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
	"time"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

// MonitorScoresController defines the interface for monitor scores HTTP handlers
type MonitorScoresController interface {
	GetMonitorScores(w http.ResponseWriter, r *http.Request)
	GetMonitorRunScores(w http.ResponseWriter, r *http.Request)
	GetScoresTimeSeries(w http.ResponseWriter, r *http.Request)
	GetTraceScores(w http.ResponseWriter, r *http.Request)
}

type monitorScoresController struct {
	scoresService *services.MonitorScoresService
}

// NewMonitorScoresController creates a new monitor scores controller
func NewMonitorScoresController(scoresService *services.MonitorScoresService) MonitorScoresController {
	return &monitorScoresController{
		scoresService: scoresService,
	}
}

// parseAndValidateTimeRange extracts startTime and endTime query parameters, parses them as
// RFC3339, and validates that endTime is after startTime. On failure it writes the appropriate
// error response and returns false.
func parseAndValidateTimeRange(w http.ResponseWriter, r *http.Request) (startTime, endTime time.Time, ok bool) {
	startTimeStr := r.URL.Query().Get("startTime")
	endTimeStr := r.URL.Query().Get("endTime")

	if startTimeStr == "" || endTimeStr == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Query parameters 'startTime' and 'endTime' are required")
		return time.Time{}, time.Time{}, false
	}

	startTime, err := time.Parse(time.RFC3339, startTimeStr)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid 'startTime' format, expected RFC3339")
		return time.Time{}, time.Time{}, false
	}

	endTime, err = time.Parse(time.RFC3339, endTimeStr)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid 'endTime' format, expected RFC3339")
		return time.Time{}, time.Time{}, false
	}

	if endTime.Before(startTime) {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "endTime must be after startTime")
		return time.Time{}, time.Time{}, false
	}

	return startTime, endTime, true
}

// GetMonitorScores handles GET .../monitors/{monitorName}/scores
// Returns scores and aggregations for a monitor within a time range
func (c *monitorScoresController) GetMonitorScores(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projName := r.PathValue("projName")
	agentName := r.PathValue("agentName")
	monitorName := r.PathValue("monitorName")

	startTime, endTime, ok := parseAndValidateTimeRange(w, r)
	if !ok {
		return
	}

	// Parse optional filter parameters
	filters := repositories.ScoreFilters{
		EvaluatorName: r.URL.Query().Get("evaluator"),
		Level:         r.URL.Query().Get("level"),
	}

	// Validate level if provided
	if filters.Level != "" && filters.Level != "trace" && filters.Level != "agent" && filters.Level != "llm" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid 'level', must be one of: trace, agent, llm")
		return
	}

	// Resolve monitor name to ID
	monitorID, err := c.scoresService.GetMonitorID(orgName, projName, agentName, monitorName)
	if err != nil {
		if errors.Is(err, utils.ErrMonitorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
			return
		}
		log.Error("Failed to resolve monitor", "monitorName", monitorName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to resolve monitor")
		return
	}

	result, err := c.scoresService.GetMonitorScores(monitorID, monitorName, startTime, endTime, filters)
	if err != nil {
		log.Error("Failed to get monitor scores", "monitorName", monitorName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get monitor scores")
		return
	}

	response := utils.ConvertToMonitorScoresResponse(result)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// GetMonitorRunScores handles GET .../monitors/{monitorName}/runs/{runId}/scores
// Returns per-run aggregated scores from the MonitorRunEvaluator records
func (c *monitorScoresController) GetMonitorRunScores(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projName := r.PathValue("projName")
	agentName := r.PathValue("agentName")
	monitorName := r.PathValue("monitorName")
	runIDStr := r.PathValue("runId")

	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid run ID")
		return
	}

	// Resolve monitor name to ID to enforce org/project/agent scoping
	monitorID, err := c.scoresService.GetMonitorID(orgName, projName, agentName, monitorName)
	if err != nil {
		if errors.Is(err, utils.ErrMonitorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
			return
		}
		log.Error("Failed to resolve monitor", "orgName", orgName, "projName", projName, "agentName", agentName, "monitorName", monitorName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to resolve monitor")
		return
	}

	result, err := c.scoresService.GetMonitorRunScores(monitorID, runID, monitorName)
	if err != nil {
		log.Error("Failed to get monitor run scores", "orgName", orgName, "projName", projName, "agentName", agentName, "monitorName", monitorName, "runId", runIDStr, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get monitor run scores")
		return
	}

	response := utils.ConvertToMonitorRunScoresResponse(result)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// GetScoresTimeSeries handles GET .../monitors/{monitorName}/scores/timeseries
// Returns time-bucketed scores for a specific evaluator
func (c *monitorScoresController) GetScoresTimeSeries(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projName := r.PathValue("projName")
	agentName := r.PathValue("agentName")
	monitorName := r.PathValue("monitorName")

	// Parse required parameters
	evaluatorName := r.URL.Query().Get("evaluator")

	startTime, endTime, ok := parseAndValidateTimeRange(w, r)
	if !ok {
		return
	}

	if evaluatorName == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Query parameter 'evaluator' is required")
		return
	}

	duration := endTime.Sub(startTime)
	if duration > 100*24*time.Hour {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Time range cannot exceed 100 days")
		return
	}

	// Resolve monitor name to ID
	monitorID, err := c.scoresService.GetMonitorID(orgName, projName, agentName, monitorName)
	if err != nil {
		if errors.Is(err, utils.ErrMonitorNotFound) {
			utils.WriteErrorResponse(w, http.StatusNotFound, "Monitor not found")
			return
		}
		log.Error("Failed to resolve monitor", "monitorName", monitorName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to resolve monitor")
		return
	}

	// Granularity is determined adaptively by the service based on data density and time range
	result, err := c.scoresService.GetEvaluatorTimeSeries(monitorID, monitorName, evaluatorName, startTime, endTime)
	if err != nil {
		log.Error("Failed to get time series data", "monitorName", monitorName, "evaluator", evaluatorName, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get time series data")
		return
	}

	response := utils.ConvertToTimeSeriesResponse(result)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}

// GetTraceScores handles GET .../traces/{traceId}/scores
// Returns all evaluation scores for a trace across ALL monitors in an agent
func (c *monitorScoresController) GetTraceScores(w http.ResponseWriter, r *http.Request) {
	log := logger.GetLogger(r.Context())

	// Extract path parameters
	orgName := r.PathValue("orgName")
	projName := r.PathValue("projName")
	agentName := r.PathValue("agentName")
	traceID := r.PathValue("traceId")

	if traceID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Trace ID is required")
		return
	}

	result, err := c.scoresService.GetTraceScores(traceID, orgName, projName, agentName)
	if err != nil {
		log.Error("Failed to get trace scores", "traceId", traceID, "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to get trace scores")
		return
	}

	response := utils.ConvertToTraceScoresResponse(result)
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Error("Failed to encode response", "error", err)
	}
}
